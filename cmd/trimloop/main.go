// Command trimloop analyses coding-agent session logs for token waste.
package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
	"github.com/ChrisAdkin8/trimloop/internal/adapters/source/claudecode"
	"github.com/ChrisAdkin8/trimloop/internal/history"
	"github.com/ChrisAdkin8/trimloop/internal/live"
	"github.com/ChrisAdkin8/trimloop/internal/report"
	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "statusline", "hook", "setup":
			os.Exit(runLive(os.Args[1], os.Args[2:]))
		case "analyse", "analyze":
			os.Args = append(os.Args[:1], os.Args[2:]...)
		}
	}
	analyse()
}

// runLive runs the commands Claude Code calls while a session is open, and
// the setup that tells Claude Code about them (ADR 0034).
func runLive(cmd string, args []string) int {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0 // the nudges must never get in the way
	}
	env := live.DefaultEnv(home)
	switch cmd {
	case "statusline":
		live.StatusLine(os.Stdin, os.Stdout, env)
	case "hook":
		if len(args) == 1 {
			live.Hook(args[0], os.Stdin, os.Stdout, env)
		}
	case "setup":
		return setup(args, home)
	}
	return 0
}

// setup prints the Claude Code settings for the nudges, or adds them to a
// settings file with --write.
func setup(args []string, home string) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	write := fs.String("write", "", "add the settings to this Claude Code settings file (a backup is made first), e.g. "+
		filepath.Join(home, ".claude", "settings.json"))
	replace := fs.Bool("replace-statusline", false, "replace an existing status line")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if strings.Contains(exe, "go-build") {
		fmt.Fprintln(os.Stderr, "This trimloop was started by 'go run' from a temporary folder. Build it first, for example:\n"+
			"  go build -o ~/.local/bin/trimloop ./cmd/trimloop\nthen run that binary's setup.")
		return 1
	}
	if *write == "" {
		fmt.Println("Add this to ~/.claude/settings.json (or a project's .claude/settings.json), or run")
		fmt.Println("'trimloop setup --write <file>' to add it for you:")
		fmt.Println()
		fmt.Println(live.SetupSnippet(exe))
		return 0
	}
	backup, err := live.WriteSettings(*write, exe, *replace)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("Added trimloop's status line and hooks to %s.\n", *write)
	if backup != "" {
		fmt.Printf("The previous file is saved as %s.\n", backup)
	}
	fmt.Println("They take effect in new Claude Code sessions.")
	return 0
}

// analyse reads every session log and writes the report.
func analyse() {
	format := flag.String("format", "summary", "report format: summary (a few lines), text (the full report), html or json")
	out := flag.String("out", "", "write the report to this file (default: standard output, or "+defaultHTML+" for html)")
	noTitles := flag.Bool("no-titles", false, "leave out session titles, which summarise what was asked")
	historyPath := flag.String("history", "", "file that keeps weekly figures between runs (default ~/.trimloop/history.json; with CLAUDE_CONFIG_DIR, one file per folder)")
	noHistory := flag.Bool("no-history", false, "do not read or update the history file")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: trimloop [analyse] [--format summary|text|html|json] [--out FILE] [--no-titles] [--history FILE | --no-history]\n"+
			"       trimloop setup [--write SETTINGS_FILE] [--replace-statusline]\n"+
			"       trimloop statusline | trimloop hook stop|prompt   (run by Claude Code)\n\n"+
			"Analyses Claude Code session logs on this machine for token waste. 'setup' adds live\n"+
			"nudges to Claude Code: a status line and two hooks.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 || !validFormat[*format] {
		flag.Usage()
		os.Exit(2)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	// The harnesses to read. Adding a harness is one line here.
	readers := []source.Reader{
		claudecode.Reader{Home: home, ConfigDir: os.Getenv("CLAUDE_CONFIG_DIR")},
	}

	// 1. Find the logs, remembering which reader found each one.
	var logs []source.Log
	byName := map[string]source.Reader{}
	for _, r := range readers {
		found, err := r.Discover()
		if err != nil {
			log.Fatalf("%s: %v", r.Name(), err)
		}
		logs = append(logs, found...)
		byName[r.Name()] = r
	}

	// 2. Read every log, then order them oldest first.
	var st stats
	data := make([]logData, 0, len(logs))
	for _, l := range logs {
		d := readLog(byName[l.Harness], l)
		st.badLines += d.bad
		if d.err != nil {
			log.Printf("%s: %v", l.Path, d.err) // report and carry on with the next log
			st.failed++
		}
		// Keep what was read even if the log failed partway.
		data = append(data, d)
		if info, err := os.Stat(l.Path); err == nil && (st.oldest.IsZero() || info.ModTime().Before(st.oldest)) {
			st.oldest = info.ModTime()
		}
	}
	st.logs = len(logs)
	oldestFirst(data)

	// 3. Build sessions. seen is shared by all logs, so a message copied into
	// several logs (for example, when a session is resumed) is counted only
	// once: in the oldest log that has it (ADR 0026).
	seen := map[string]bool{}
	var sessions []event.Session
	for _, d := range data {
		if s := buildSession(d, seen); len(s.Turns) > 0 {
			sessions = append(sessions, s)
		}
	}
	inheritProjects(sessions)

	// 4. Run the findings and write the report.
	settings := report.DefaultSettings()
	settings.Titles = !*noTitles
	r := report.Analyse(sessions, report.Read{Logs: st.logs, Failed: st.failed, BadLines: st.badLines},
		settings, time.Now())
	if !*noHistory {
		path := *historyPath
		if path == "" {
			path = defaultHistory(home, os.Getenv("CLAUDE_CONFIG_DIR"))
		}
		keepHistory(&r, path, st.oldest)
	}
	if err := write(r, *format, *out); err != nil {
		log.Fatal(err)
	}
}

// defaultHistory is where weekly figures are kept: one file per set of
// logs, so runs over another Claude Code folder (CLAUDE_CONFIG_DIR) do not
// replace each other's weeks. The folder is named by a hash of its path.
func defaultHistory(home, configDir string) string {
	dir := filepath.Join(home, ".trimloop")
	if abs, err := filepath.Abs(configDir); configDir != "" && err == nil {
		configDir = abs
	}
	if configDir == "" || filepath.Clean(configDir) == filepath.Join(home, ".claude") {
		return filepath.Join(dir, "history.json") // the default logs, however named
	}
	sum := sha256.Sum256([]byte(filepath.Clean(configDir)))
	return filepath.Join(dir, fmt.Sprintf("history-%x.json", sum[:4]))
}

// partialWeek is the last week whose logs Claude Code may have partly
// deleted: the week of the oldest log still on disk, by its last change.
// It returns "" if no log was found.
func partialWeek(oldest time.Time) string {
	if oldest.IsZero() {
		return ""
	}
	return detectors.WeekOf(oldest.Local())
}

// keepHistory merges this run's weekly figures into the history file at path
// and gives the report every week kept there. A history that cannot be read
// or written is reported and skipped: the report still runs.
func keepHistory(r *report.Results, path string, oldest time.Time) {
	h, err := history.Load(path)
	if err != nil {
		log.Printf("history not used: %v", err)
		return
	}
	r.Weeks = history.Merge(&h, r.Weeks, r.Prices, partialWeek(oldest))
	if err := history.Save(path, h); err != nil {
		log.Printf("history not saved: %v", err)
	}
}

// defaultHTML is where an HTML report goes when no --out is given.
const defaultHTML = "trimloop-report.html"

var validFormat = map[string]bool{"summary": true, "text": true, "html": true, "json": true}

// summary prints the short summary, pointing to the full formats.
func summary(out io.Writer, r report.Results) error { return report.WriteSummary(out, r, "", 3) }

// write renders r in format to path, or to standard output if path is empty
// (HTML always goes to a file). When the report goes to a file, a short
// summary is printed to the terminal.
func write(r report.Results, format, path string) error {
	if format == "html" && path == "" {
		path = defaultHTML
	}
	render := map[string]func(io.Writer, report.Results) error{
		"summary": summary,
		"text":    report.WriteText,
		"json":    report.WriteJSON,
		"html":    report.WriteHTML,
	}[format]
	if path == "" {
		return render(os.Stdout, r)
	}
	// Readable by the owner only: a report names projects and shows session
	// titles, which summarise what was asked.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil { // an existing file keeps its mode otherwise
		_ = f.Close()
		return err
	}
	if err := render(f, r); err != nil {
		_ = f.Close() // the write error is the one to report
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if format == "summary" {
		return nil
	}
	return report.WriteSummary(os.Stdout, r, path, 3)
}

// stats counts what was read.
type stats struct {
	logs, failed, badLines int
	oldest                 time.Time // the oldest log's last change: older logs may have been deleted
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
	s := sessionFor(d.log.Path)
	var last time.Time               // latest response time so far in this log
	turnOf := map[string]int{}       // message id -> index in s.Turns, for ids counted here
	callAt := map[string][2]int{}    // tool-use id -> turn index, call index
	compacted := false               // a compaction since the last new turn
	prompted := false                // a message from the user since the last new turn
	var preInjected []event.Injected // added before a pending compaction's marker
	prePrompted := false             // a message from the user before it
	var compaction event.Compaction
	var compactedAt time.Time                // when that compaction was logged as happening
	customTitle := false                     // the user named the session: generated titles no longer replace it
	var injected []event.Injected            // added since the last new turn
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
				prompted = true
			}
			continue
		case source.KindCompaction:
			if copiedLine(m, seen) {
				continue
			}
			compacted, compaction, compactedAt = true, m.Compaction, m.Time
			// What came before the marker belongs to the request before the
			// compaction; what follows it, to the first one after.
			preInjected, prePrompted = injected, prePrompted || prompted
			injected, prompted = nil, false
			continue
		case source.KindInjected:
			if copiedLine(m, seen) {
				continue
			}
			injected = append(injected, m.Injected...)
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
				prompted = true
			}
			continue
		case source.KindTitle:
			if m.CustomTitle || !customTitle {
				s.Title, customTitle = m.Title, m.CustomTitle
			}
			continue
		}

		if m.Model == claudecode.Synthetic {
			// A reply the harness made up (after an API error): no usage,
			// not a turn, but still activity.
			if m.Time.After(last) {
				last = m.Time
			}
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
			before := compacted && !compactedAt.IsZero() && !m.Time.IsZero() && m.Time.Before(compactedAt)
			t := event.Turn{
				ID:              m.ID,
				Seq:             len(s.Turns),
				Model:           m.Model,
				Time:            m.Time,
				First:           last.IsZero(),
				Usage:           m.Usage,
				Fast:            m.Fast,
				USOnly:          m.USOnly,
				Project:         m.Project,
				Attribution:     m.Attribution,
				CompactedBefore: compacted && !before,
				Prompted:        prompted || prePrompted,
				Injected:        injected,
				Standing:        standing,
			}
			if before {
				// The triggering reply takes what came before the marker and
				// leaves what the compaction re-attached for the next turn.
				t.Injected, t.Prompted = preInjected, prePrompted
				preInjected, prePrompted = nil, false
			} else {
				injected, prompted, preInjected, prePrompted = nil, false, nil, false
			}
			if compacted && !before {
				t.Compaction = compaction
			}
			if s.Project == "" {
				s.Project = m.Project
			}
			if s.Kind == "subagent" && s.AgentType == "" {
				s.AgentType = m.Attribution.Agent
			}
			if !last.IsZero() && m.Time.After(last) {
				t.IdleBefore = m.Time.Sub(last)
			}
			t.Cost, t.Priced = price.Cost(m.Model, m.Usage, price.Options{Fast: m.Fast, USOnly: m.USOnly})
			s.Turns = append(s.Turns, t)
			if !before {
				compacted, compaction = false, event.Compaction{}
			}
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
		if m.Time.After(last) {
			last = m.Time
		}
	}
	s.Harness = sumRuns(runs)
	if s.Kind == "subagent" && s.AgentType == "" {
		s.AgentType = claudecode.AgentType(d.log.Path)
	}
	if compacted {
		c := compaction
		s.Trailing = &c
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

// inheritProjects gives a subagent that logged no project, and its turns,
// its main session's project.
func inheritProjects(sessions []event.Session) {
	project := map[string]string{}
	for _, s := range sessions {
		if s.Kind == "main" && s.Project != "" {
			project[s.ID] = s.Project
		}
	}
	for i, s := range sessions {
		p, ok := project[s.ParentID]
		if !ok || s.Kind != "subagent" {
			continue
		}
		if s.Project == "" {
			sessions[i].Project = p
		}
		for j := range s.Turns {
			if s.Turns[j].Project == "" {
				s.Turns[j].Project = p
			}
		}
	}
}

// sessionFor names the session a log holds. A main session log is named after
// its session ID. A subagent log is shown with its session ID in front, so it
// can be traced back to the session that started it.
func sessionFor(path string) event.Session {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dir := filepath.Dir(path)
	if filepath.Base(dir) == "subagents" {
		parent := filepath.Base(filepath.Dir(dir))
		return event.Session{ID: parent + "/" + name, Kind: "subagent", ParentID: parent}
	}
	return event.Session{ID: name, Kind: "main"}
}
