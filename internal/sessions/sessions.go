// Package sessions turns harness logs into sessions: it finds and reads
// every log, then builds one session per log, oldest log first, so that a
// message copied into several logs is counted once (ADR 0026), and credits
// and names projects by conversation (ADR 0053).
package sessions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

// Read is what reading the logs found.
type Read struct {
	Logs     int       // logs found
	BadLines int       // lines that could not be parsed, skipped
	Oldest   time.Time // the oldest log's last change: older logs may have been deleted
	Problems []error   // one per log whose reading stopped early (what was read is kept), naming its path, in reading order
}

// Load finds every reader's logs, reads them all and builds their sessions.
// A log that fails partway keeps what was read and is named in Problems; a
// reader that cannot look for logs is an error. Sessions with no turns and
// no usage-limit refusals are left out.
func Load(readers []source.Reader) ([]event.Session, Read, error) {
	// 1. Find the logs, remembering which reader found each one.
	var logs []source.Log
	byName := map[string]source.Reader{}
	for _, r := range readers {
		found, err := r.Discover()
		if err != nil {
			return nil, Read{}, fmt.Errorf("%s: %w", r.Name(), err)
		}
		logs = append(logs, found...)
		byName[r.Name()] = r
	}

	// 2. Read every log.
	read := Read{Logs: len(logs)}
	data := make([]logData, 0, len(logs))
	for _, l := range logs {
		d := readLog(byName[l.Harness], l)
		read.BadLines += d.bad
		if d.err != nil {
			read.Problems = append(read.Problems, fmt.Errorf("%s: %w", l.Path, d.err))
		}
		// Keep what was read even if the log failed partway.
		data = append(data, d)
		if info, err := os.Stat(l.Path); err == nil && (read.Oldest.IsZero() || info.ModTime().Before(read.Oldest)) {
			read.Oldest = info.ModTime()
		}
	}

	// 3. Build sessions, oldest log first.
	return buildSessions(data), read, nil
}

// logData is everything read from one log.
type logData struct {
	log   source.Log
	msgs  []source.Message // in file order, repeats included
	start time.Time        // earliest message time; zero if none has a time
	end   time.Time        // latest message time
	bad   int              // lines that could not be parsed
	err   error            // set if reading stopped early
}

// readLog reads one log with r. If reading fails partway, it keeps what was
// read so far and records the error.
func readLog(r source.Reader, l source.Log) logData {
	d := logData{log: l}
	for m, err := range r.Read(&d.log) {
		if errors.Is(err, source.ErrBadLine) {
			d.bad++ // count it, skip it, keep going
			continue
		}
		if err != nil {
			d.err = err
			break
		}
		d.msgs = append(d.msgs, m)
		if m.Kind == source.KindTurn && m.Time.After(d.end) {
			d.end = m.Time
		}
		if m.Kind == source.KindTurn && !m.Time.IsZero() && (d.start.IsZero() || m.Time.Before(d.start)) {
			d.start = m.Time
		}
	}
	return d
}

// oldestFirst sorts logs by their earliest message, so a message copied into
// a later log is credited to the log it was first written in. A resumed log
// copies the original's messages with their times, so the two can start at
// the same moment: the one that ended first is then the original. Logs with
// no times go last; remaining ties keep path order, so every run agrees.
func oldestFirst(data []logData) {
	sort.SliceStable(data, func(i, j int) bool {
		a, b := data[i].start, data[j].start
		switch {
		case a.IsZero() != b.IsZero():
			return !a.IsZero() // a log with a time comes before one without
		case !a.Equal(b):
			return a.Before(b)
		case !data[i].end.Equal(data[j].end):
			return data[i].end.Before(data[j].end)
		}
		return data[i].log.Path < data[j].log.Path
	})
}

// buildSession turns one log's messages into a session and prices each turn.
// Message IDs already in seen are skipped, and new ones are added to it.
// Skipped repeats still count as activity, so the idle time before the next
// turn is measured from the last thing that happened in this log.
//
// Tool calls are collected from every line of a message counted in this log,
// and each result is attached to its call. Calls of a message counted in an
// earlier log (copied into a resumed session) are not: their output is
// credited to the log it first appeared in.
func buildSession(d logData, seen map[string]bool) event.Session {
	s := sessionFor(d.log)
	var last time.Time            // latest response time so far in this log
	replied := false              // a real reply (not one the harness made up) came earlier in this log
	turnOf := map[string]int{}    // message id -> index in s.Turns, for ids counted here
	callAt := map[string][2]int{} // tool-use id -> turn index, call index
	// active marks activity at time t: idle time before a turn runs from the
	// latest activity, which includes replies the harness made up itself.
	active := func(t time.Time) {
		if t.After(last) {
			last = t
		}
	}
	var pending between                      // what the next new turn inherits (between.go)
	customTitle := false                     // the user named the session: generated titles no longer replace it
	var standing *event.Standing             // the latest system prompt and tool list
	runs := map[string]event.HarnessRecord{} // the harness's latest record per run
	for _, m := range d.msgs {
		switch m.Kind {
		case source.KindToolResults:
			answered := m.UserDecided // the user allowed or refused a call
			for _, r := range m.Results {
				if at, ok := callAt[r.CallID]; ok {
					c := &s.Turns[at[0]].ToolCalls[at[1]]
					c.Result, c.HasResult = r.ToolResult, true
					answered = answered || c.Tool == "AskUserQuestion" // the user answered Claude's question
				}
			}
			if answered && !copiedLine(m, seen) {
				pending.prompt()
			}
			continue
		case source.KindCompaction:
			if copiedLine(m, seen) {
				continue
			}
			// Another compaction with no reply in between: the earlier one
			// is kept as unreplied.
			if u := pending.compact(m.Compaction, m.Time); u != nil {
				s.Unreplied = append(s.Unreplied, *u)
			}
			continue
		case source.KindInjected:
			if copiedLine(m, seen) {
				continue
			}
			pending.inject(m.Injected)
			continue
		case source.KindStanding:
			st := m.Standing
			if len(st.Tools) == 0 && standing != nil {
				// A record without the tool list leaves it out rather than
				// saying there are no tools: keep the last one seen.
				st = &event.Standing{SystemBytes: st.SystemBytes, Tools: standing.Tools}
			}
			standing = st
			continue
		case source.KindCostRecord:
			rec := m.Record
			if rec.At.IsZero() {
				rec.At = last // the record covers the turns logged before it
			}
			if prev, ok := runs[m.RunID]; !ok || rec.Cost >= prev.Cost {
				runs[m.RunID] = rec // a run's records are running totals
			}
			continue
		case source.KindPrompt:
			if !copiedLine(m, seen) {
				pending.prompt()
			}
			continue
		case source.KindLimitHit:
			active(m.Time) // a refused request is activity, like any reply the harness made up
			if !copiedLine(m, seen) {
				s.LimitHits = append(s.LimitHits, event.LimitHit{At: m.Time, Limit: m.Limit})
			}
			continue
		case source.KindFailure:
			active(m.Time) // not a turn, but activity, like any reply the harness made up
			if !copiedLine(m, seen) {
				f := event.Failure{At: m.Time, Kind: m.Failure}
				if n := len(s.Turns); n > 0 {
					f.Context, f.Model = s.Turns[n-1].Context(), s.Turns[n-1].Model
				}
				s.Failures = append(s.Failures, f)
			}
			continue
		case source.KindTitle:
			if m.CustomTitle || !customTitle {
				s.Title, customTitle = m.Title, m.CustomTitle
			}
			continue
		}

		if m.Synthetic {
			// A reply the harness made up (after an API error): no usage,
			// not a turn, but still activity.
			active(m.Time)
			continue
		}
		i, here := turnOf[m.ID]
		if here && fuller(m.Usage, s.Turns[i].Usage) {
			// Claude Code logs one line per content block; an early line can
			// carry a partial count. Keep the fullest one and price it again.
			t := &s.Turns[i]
			t.Usage = m.Usage
			t.Cost, t.Priced = price.Cost(t.Model, t.Usage, price.Options{Fast: t.Fast, USOnly: t.USOnly})
		}
		if m.ID == "" || !seen[m.ID] { // no id: nothing to match repeats on
			if m.ID != "" {
				seen[m.ID] = true
			}
			// The reply that triggered a compaction can be logged after it,
			// with an earlier time: it still belongs before the compaction.
			h := pending.take(m.Time)
			t := event.Turn{
				ID:              m.ID,
				Seq:             len(s.Turns),
				Model:           m.Model,
				Time:            m.Time,
				First:           !replied, // limit hits and failures before it wrote no cache
				Usage:           m.Usage,
				Fast:            m.Fast,
				Effort:          m.Effort,
				USOnly:          m.USOnly,
				Project:         turnProject(s, m.Project),
				Attribution:     m.Attribution,
				CompactedBefore: h.compaction != nil,
				Prompted:        h.prompted,
				Injected:        h.injected,
				Standing:        standing,
			}
			if h.compaction != nil {
				t.Compaction = *h.compaction
			}
			if s.Project == "" {
				s.Project, t.Project = m.Project, m.Project
			}
			if s.Kind == event.SubAgentSession && s.AgentType == "" {
				s.AgentType = m.Attribution.Agent
			}
			if !last.IsZero() && m.Time.After(last) {
				t.IdleBefore = m.Time.Sub(last)
			}
			t.Cost, t.Priced = price.Cost(m.Model, m.Usage, price.Options{Fast: m.Fast, USOnly: m.USOnly})
			s.Turns = append(s.Turns, t)
			i, here = len(s.Turns)-1, true
			if m.ID != "" {
				turnOf[m.ID] = i
			}
		}
		if here {
			for _, c := range m.ToolCalls {
				if c.ID != "" {
					if _, dup := callAt[c.ID]; dup {
						continue
					}
					callAt[c.ID] = [2]int{i, len(s.Turns[i].ToolCalls)}
				}
				s.Turns[i].ToolCalls = append(s.Turns[i].ToolCalls, c)
			}
		}
		active(m.Time)
		replied = true
	}
	s.Harness = sumRuns(runs)
	if s.Kind == event.SubAgentSession && s.AgentType == "" {
		s.AgentType = d.log.AgentType // the type recorded beside the log, when no line named it
	}
	if u := pending.end(); u != nil {
		s.Unreplied = append(s.Unreplied, *u)
	}
	return s
}

// fuller reports whether usage a counts more tokens than b, as a later line
// of the same reply does when an earlier one held a partial count.
func fuller(a, b event.Usage) bool {
	return a.InputTotal()+a.Output > b.InputTotal()+b.Output
}

// copiedLine reports whether a line with this LineID was already counted in
// an earlier log (it was copied into a resumed or forked session), and marks
// it counted otherwise.
func copiedLine(m source.Message, seen map[string]bool) bool {
	if m.LineID == "" {
		return false
	}
	key := "line:" + m.LineID
	if seen[key] {
		return true
	}
	seen[key] = true
	return false
}

// sumRuns adds up the harness's records for each run of a session, or
// returns nil if there are none.
func sumRuns(runs map[string]event.HarnessRecord) *event.HarnessRecord {
	if len(runs) == 0 {
		return nil
	}
	total := &event.HarnessRecord{ByModel: map[string]event.ModelRecord{}}
	for _, r := range runs {
		total.Cost += r.Cost
		total.UnknownCost = total.UnknownCost || r.UnknownCost
		if r.At.After(total.At) {
			total.At = r.At
		}
		if !r.Start.IsZero() && (total.Start.IsZero() || r.Start.Before(total.Start)) {
			total.Start = r.Start
		}
		for model, m := range r.ByModel {
			total.ByModel[model] = total.ByModel[model].Add(m)
		}
	}
	return total
}

// buildSessions orders the logs oldest first and builds their sessions,
// then credits and names projects across all of them. seen is shared by all
// logs, so a message copied into several logs (for example, when a session
// is resumed) is counted only once: in the oldest log that has it (ADR
// 0026). The end-to-end test (golden_test.go) runs this same function.
func buildSessions(data []logData) []event.Session {
	oldestFirst(data)
	seen := map[string]bool{}
	var sessions []event.Session
	for _, d := range data {
		if s := buildSession(d, seen); len(s.Turns) > 0 || len(s.LimitHits) > 0 {
			sessions = append(sessions, s)
		}
	}
	inheritProjects(sessions)
	nameProjects(sessions)
	return sessions
}

// inheritProjects credits a subagent, and its turns, by its conversation:
// each turn belongs to its main session's project, as the main session's
// own turns do, when it ran there, in a subfolder or in a folder above
// (projectFor), and to the folder it ran in otherwise, whatever folder the
// subagent started in (ADR 0053). One that logged no folder takes the main
// session's; one whose main session was not read is matched against its
// own first folder.
func inheritProjects(sessions []event.Session) {
	project := map[string]string{}
	for _, s := range sessions {
		if s.Kind == event.MainSession && s.Project != "" {
			project[s.ID] = s.Project
		}
	}
	for i, s := range sessions {
		if s.Kind != event.SubAgentSession {
			continue
		}
		p, ok := project[s.ParentID]
		if !ok {
			p = s.Project // its main session was not read: its own folder is the conversation's
		}
		sessions[i].Project = projectFor(s.Project, p)
		for j := range s.Turns {
			s.Turns[j].Project = projectFor(s.Turns[j].Project, p)
		}
	}
}

// turnProject is the folder a new turn of s is credited to while its log is
// read: for a main session, by the session's folder (projectFor); for a
// sub-agent, the folder as logged, since it belongs to its conversation's
// folder, which inheritProjects applies once every log is read (ADR 0053).
func turnProject(s event.Session, dir string) string {
	if s.Kind == event.SubAgentSession {
		return dir
	}
	return projectFor(dir, s.Project)
}

// projectFor returns the project folder to credit a turn that ran in dir to,
// in a session whose project folder is home: home when dir is home, a
// folder inside it, or a folder above it (a cd into a subfolder or up to
// the parent is still the same work); dir when it is somewhere else.
func projectFor(dir, home string) string {
	if dir == "" || home == "" || within(dir, home) || within(home, dir) {
		return home
	}
	return dir
}

// within reports whether path is dir or inside it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// nameProjects replaces the project folders of sessions and turns with short
// names: the folder's own name, or, where two folders share a name, as many
// of their parent folders' names as tell them apart ("work/api" and
// "home/api"). Reports show these names, never the paths (ADR 0032).
func nameProjects(sessions []event.Session) {
	dirs := map[string]bool{}
	for _, s := range sessions {
		dirs[s.Project] = true
		for _, t := range s.Turns {
			dirs[t.Project] = true
		}
	}
	delete(dirs, "")
	names := shortNames(dirs)
	for i := range sessions {
		sessions[i].Project = names[sessions[i].Project]
		for j := range sessions[i].Turns {
			sessions[i].Turns[j].Project = names[sessions[i].Turns[j].Project]
		}
	}
}

// shortNames gives each folder the fewest trailing path elements that no
// other folder in dirs shares. A folder with no parent left to add keeps
// the name it has ("/app" stays "app" beside "/Users/c/app", which becomes
// "c/app"); names of different lengths cannot clash. The result does not
// depend on map order.
func shortNames(dirs map[string]bool) map[string]string {
	names := map[string]string{"": ""}
	for depth := 1; len(dirs) > 0; depth++ {
		byName := map[string][]string{}
		for d := range dirs {
			byName[tail(d, depth)] = append(byName[tail(d, depth)], d)
		}
		for name, ds := range byName {
			for _, d := range ds {
				if len(ds) == 1 || tail(d, depth+1) == name { // unique, or nothing left to add
					names[d] = name
					delete(dirs, d)
				}
			}
		}
	}
	return names
}

// tail returns the last n elements of a path, split at either slash, joined
// with "/".
func tail(path string, n int) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return path
	}
	return strings.Join(parts[max(0, len(parts)-n):], "/")
}

// sessionFor starts the session a log holds, named as its reader found it
// (source.Log): a sub-agent's log names the main session that started it.
// A log with no session id is named after its file.
func sessionFor(l source.Log) event.Session {
	id := l.Session
	if id == "" {
		id = strings.TrimSuffix(filepath.Base(l.Path), filepath.Ext(l.Path))
	}
	if l.Parent != "" {
		return event.Session{ID: id, Kind: event.SubAgentSession, ParentID: l.Parent}
	}
	return event.Session{ID: id, Kind: event.MainSession}
}
