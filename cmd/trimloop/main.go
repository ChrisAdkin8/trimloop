// Command trimloop analyses coding-agent session logs for token waste.
package main

import (
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
	"github.com/ChrisAdkin8/trimloop/internal/report"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/price"
)

func main() {
	format := flag.String("format", "text", "report format: text, html or json")
	out := flag.String("out", "", "write the report to this file (default: standard output for text and json, "+defaultHTML+" for html)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: trimloop [--format text|html|json] [--out FILE]\n\n"+
			"Analyses Claude Code session logs on this machine for token waste.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 || (*format != "text" && *format != "html" && *format != "json") {
		flag.Usage()
		os.Exit(2)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	// The harnesses to read. Adding a harness is one line here.
	readers := []source.Reader{
		claudecode.Reader{Home: home},
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
	r := report.Analyse(sessions, report.Read{Logs: st.logs, Failed: st.failed, BadLines: st.badLines},
		report.DefaultSettings(), time.Now())
	if err := write(r, *format, *out); err != nil {
		log.Fatal(err)
	}
}

// defaultHTML is where an HTML report goes when no --out is given.
const defaultHTML = "trimloop-report.html"

// write renders r in format to path, or to standard output if path is empty
// (HTML always goes to a file). When the report goes to a file, a short
// summary is printed to the terminal.
func write(r report.Results, format, path string) error {
	if format == "html" && path == "" {
		path = defaultHTML
	}
	render := map[string]func(io.Writer, report.Results) error{
		"text": report.WriteText,
		"json": report.WriteJSON,
		"html": report.WriteHTML,
	}[format]
	if path == "" {
		return render(os.Stdout, r)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := render(f, r); err != nil {
		_ = f.Close() // the write error is the one to report
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return report.WriteSummary(os.Stdout, r, path, 3)
}

// stats counts what was read.
type stats struct {
	logs, failed, badLines int
}

// logData is everything read from one log.
type logData struct {
	log   source.Log
	msgs  []source.Message // in file order, repeats included
	start time.Time        // earliest message time; zero if none has a time
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
		if m.Kind == source.KindTurn && !m.Time.IsZero() && (d.start.IsZero() || m.Time.Before(d.start)) {
			d.start = m.Time
		}
	}
	return d
}

// oldestFirst sorts logs by their earliest message, so a message copied into
// a later log is credited to the log it was first written in. Logs with no
// times go last; ties keep path order, so the result is the same every run.
func oldestFirst(data []logData) {
	sort.SliceStable(data, func(i, j int) bool {
		a, b := data[i].start, data[j].start
		switch {
		case a.IsZero() != b.IsZero():
			return !a.IsZero() // a log with a time comes before one without
		case !a.Equal(b):
			return a.Before(b)
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
	var last time.Time            // latest response time so far in this log
	turnOf := map[string]int{}    // message id -> index in s.Turns, for ids counted here
	callAt := map[string][2]int{} // tool-use id -> turn index, call index
	compacted := false            // a compaction since the last new turn
	var compaction event.Compaction
	for _, m := range d.msgs {
		switch m.Kind {
		case source.KindToolResults:
			for _, r := range m.Results {
				if at, ok := callAt[r.CallID]; ok {
					c := &s.Turns[at[0]].ToolCalls[at[1]]
					c.Result, c.HasResult = r.ToolResult, true
				}
			}
			continue
		case source.KindCompaction:
			compacted, compaction = true, m.Compaction
			continue
		}

		i, here := turnOf[m.ID]
		if m.ID == "" || !seen[m.ID] { // no id: nothing to match repeats on
			if m.ID != "" {
				seen[m.ID] = true
			}
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
				CompactedBefore: compacted,
			}
			if compacted {
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
			compacted, compaction = false, event.Compaction{}
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
	return s
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
