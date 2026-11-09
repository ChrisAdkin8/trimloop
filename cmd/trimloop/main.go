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
		case "help":
			_, code, _ := parseArgs([]string{"--help"}, os.Stdout, os.Stderr)
			os.Exit(code)
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
	if cmd == "setup" {
		return setup(args, home)
	}
	runNudge(cmd, args, os.Stdin, os.Stdout, home)
	return 0
}

// The status line and the hooks, swapped by tests.
var (
	statusLine = live.StatusLine
	hook       = live.Hook
)

// runNudge runs the status line or a hook. They must never get in the way,
// so a panic is swallowed and prints nothing: Go would exit with code 2,
// which for a UserPromptSubmit hook blocks the user's prompt and shows the
// error. The output is held until the command finishes, so a panic part way
// leaves none.
func runNudge(cmd string, args []string, in io.Reader, out io.Writer, home string) {
	var buf strings.Builder
	defer func() {
		if recover() != nil {
			return
		}
		_, _ = io.WriteString(out, buf.String())
	}()
	env := live.DefaultEnv(home)
	switch cmd {
	case "statusline":
		statusLine(in, &buf, env)
	case "hook":
		if len(args) == 1 {
			hook(args[0], in, &buf, env)
		}
	}
}

// setup prints the Claude Code settings for the nudges, or adds them to a
// settings file with --write.
func setup(args []string, home string) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	cfgDir := os.Getenv("CLAUDE_CONFIG_DIR")
	defaultSettings := userSettings(home, cfgDir)
	write := fs.String("write", "", "add the settings to this Claude Code settings file (a backup is made first), e.g. "+
		shellArg(defaultSettings))
	replace := fs.Bool("replace-statusline", false, "replace an existing status line")
	check := fs.Bool("check", false, "say whether the status line, hooks and usage-limit readings are working; change nothing")
	settingsFile := fs.String("settings", defaultSettings, "the Claude Code settings file --check reads and plain setup names")
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return 0 // asked for: the options are printed
	} else if err != nil {
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
	if *check {
		return checkSetup(os.Stdout, *settingsFile, exe, home, cfgDir)
	}
	if *write == "" {
		fmt.Printf("Add this to %s (or a project's .claude/settings.json), or run\n", *settingsFile)
		fmt.Printf("'trimloop setup --write %s' to add it for you:\n", shellArg(*settingsFile))
		fmt.Println()
		fmt.Println(live.SetupSnippet(exe))
		return 0
	}
	return writeSetup(os.Stdout, os.Stderr, *write, exe, *replace, home, cfgDir)
}

// userSettings is the user settings file Claude Code reads: in
// CLAUDE_CONFIG_DIR when it is set, else in ~/.claude.
func userSettings(home, cfgDir string) string {
	if cfgDir != "" {
		return filepath.Join(cfgDir, "settings.json")
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// unreadSettings returns the settings file Claude Code reads instead of
// path, when path is in ~/.claude but CLAUDE_CONFIG_DIR points Claude Code
// at another folder; "" otherwise. A project's .claude/settings.json is
// read wherever CLAUDE_CONFIG_DIR points, so it is not warned about, and
// neither is a path that links to the file Claude Code reads.
func unreadSettings(path, home, cfgDir string) string {
	userDir := filepath.Join(home, ".claude")
	if cfgDir == "" || !sameDir(filepath.Dir(path), userDir) || sameDir(cfgDir, userDir) {
		return ""
	}
	reads := userSettings(home, cfgDir)
	if resolved(path) == resolved(reads) {
		return ""
	}
	return reads
}

// resolved is the file path names once every link is followed. For a file
// not made yet (or a link to one), it follows the links to its folder; when
// that folder is missing too, it is the cleaned absolute path.
func resolved(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	if target, err := os.Readlink(path); err == nil { // a link to a file not made yet
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = target
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		return filepath.Join(dir, filepath.Base(path))
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// sameDir reports whether a and b name the same folder, following links
// when both exist.
func sameDir(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	if errA == nil && errB == nil {
		return os.SameFile(ia, ib)
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	return errA == nil && errB == nil && absA == absB
}

// writeSetup adds the nudges to a settings file for 'trimloop setup --write'
// and says what it did, and how to undo it.
func writeSetup(stdout, stderr io.Writer, path, exe string, replace bool, home, cfgDir string) int {
	if reads := unreadSettings(path, home, cfgDir); reads != "" {
		fmt.Fprintf(stderr, "Warning: CLAUDE_CONFIG_DIR is set, so Claude Code reads %s, not %s.\n"+
			"  To add the nudges where Claude Code will see them, run: trimloop setup --write %s\n", reads, path, shellArg(reads))
	}
	backup, changed, err := live.WriteSettings(path, exe, replace)
	if errors.Is(err, live.ErrStatusLineTaken) {
		fmt.Fprintf(stderr, "Nothing was written, hooks included: %s already has another program's status line.\n", path)
		fmt.Fprint(stderr, otherStatusLineChoices(path, exe, false))
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !changed {
		fmt.Fprintf(stdout, "trimloop's status line and hooks in %s are already up to date; nothing changed.\n", path)
		return 0
	}
	fmt.Fprintf(stdout, "Added trimloop's status line and hooks to %s.\n", path)
	fmt.Fprintln(stdout, "They take effect in new Claude Code sessions.")
	if backup != "" {
		fmt.Fprintf(stdout, "To undo this, put the previous file back: cp %s %s\n", shellArg(backup), shellArg(path))
	} else {
		fmt.Fprintf(stdout, "There was no settings file before; to undo this, delete %s (or its statusLine and hooks entries).\n", path)
	}
	return 0
}

// otherStatusLineChoices says what to do when a settings file already has
// another program's status line, which 'setup --write' does not replace
// unless asked: replace it, or keep it and add only the hooks by hand.
// When the hooks are already in place (hooksDone), only the first choice
// is left.
func otherStatusLineChoices(path, exe string, hooksDone bool) string {
	if hooksDone {
		return fmt.Sprintf("trimloop's hooks are in place. To use trimloop's status line instead (a backup is made first), run:\n"+
			"  trimloop setup --write %s --replace-statusline\n", shellArg(path))
	}
	return fmt.Sprintf("You can either:\n"+
		"  - use trimloop's status line instead (a backup is made first):\n"+
		"      trimloop setup --write %s --replace-statusline\n"+
		"  - or keep your status line and add only trimloop's hooks, by merging this into %s:\n\n%s\n",
		shellArg(path), path, live.HooksSnippet(exe))
}

// analyseOptions is what the analyse command line asks for.
type analyseOptions struct {
	format, out, history string
	noTitles, noHistory  bool
	open                 bool
	since                time.Time // zero for every session
}

// helpHint ends the one line that explains a mistake on the command line.
const helpHint = "Run 'trimloop --help' to see every option.\n"

// parseArgs reads the analyse command line: args without the program name
// or the word analyse. It returns false, with the exit code, when the run
// stops here: after --help or --version, printed to stdout (0), or after a
// mistake, explained in one line on stderr (2).
func parseArgs(args []string, stdout, stderr io.Writer) (analyseOptions, int, bool) {
	var o analyseOptions
	fs := flag.NewFlagSet("trimloop", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // a mistake is explained below, in one line
	fs.Usage = func() {}
	fs.StringVar(&o.format, "format", "summary", "report format: summary (a few lines), text (the full report), html or json")
	fs.StringVar(&o.out, "out", "", "write the report to this file (default: standard output, or "+defaultHTML+" for html; - for standard output)")
	fs.BoolVar(&o.noTitles, "no-titles", false, "leave out session titles, which summarise what was asked")
	fs.StringVar(&o.history, "history", "", "file that keeps weekly figures between runs (default ~/.trimloop/history.json; with CLAUDE_CONFIG_DIR, one file per folder)")
	fs.BoolVar(&o.noHistory, "no-history", false, "do not read or update the history file")
	sinceArg := fs.String("since", "", "only sessions started on or after this date (2026-11-01) or this long ago (7d, 2w), with their sub-agents; the history file is not used")
	fs.BoolVar(&o.open, "open", false, "write the web page and open it in the browser (implies --format html)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	err := fs.Parse(args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		usage(stdout, fs)
		return o, 0, false
	case err != nil:
		fmt.Fprintf(stderr, "%s.\n%s", optionError(err), helpHint)
		return o, 2, false
	case *showVersion:
		fmt.Fprintln(stdout, "trimloop", version())
		return o, 0, false
	}
	formatSet := false
	fs.Visit(func(f *flag.Flag) { formatSet = formatSet || f.Name == "format" })
	if o.open && !formatSet {
		o.format = "html"
	}
	if err := checkOptions(cliOptions{
		args:   fs.Args(),
		first:  fs.NArg() == len(args), // nothing came before the first word
		format: o.format,
		open:   o.open,
		out:    o.out,
	}); err != nil {
		fmt.Fprintf(stderr, "%v.\n%s", err, helpHint)
		return o, 2, false
	}
	if *sinceArg != "" {
		if o.since, err = parseSince(*sinceArg, time.Now()); err != nil {
			fmt.Fprintln(stderr, err)
			return o, 2, false
		}
	}
	return o, 0, true
}

// usage prints how to run trimloop and every analyse option.
func usage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprint(w, "Usage: trimloop [analyse] [--format summary|text|html|json] [--out FILE] [--open] [--since DATE]\n"+
		"                         [--no-titles] [--history FILE | --no-history]\n"+
		"       trimloop --version\n"+
		"       trimloop setup [--write SETTINGS_FILE] [--replace-statusline]\n"+
		"       trimloop setup --check [--settings SETTINGS_FILE]\n"+
		"       trimloop statusline | trimloop hook stop|prompt   (run by Claude Code)\n\n"+
		"Analyses Claude Code session logs on this machine for token waste. 'setup' adds live\n"+
		"nudges to Claude Code: a status line and two hooks.\n\n")
	fs.SetOutput(w)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
}

// optionError puts the flag package's errors in the words the other
// mistakes use: "unknown option --foo", not Go's own wording.
func optionError(err error) string {
	msg := err.Error()
	if name, ok := strings.CutPrefix(msg, "flag provided but not defined: "); ok {
		return "unknown option --" + strings.TrimLeft(name, "-")
	}
	if name, ok := strings.CutPrefix(msg, "flag needs an argument: "); ok {
		return "--" + strings.TrimLeft(name, "-") + " needs a value"
	}
	msg = strings.Replace(msg, " for flag -", " for --", 1)
	return strings.Replace(msg, " for -", " for --", 1)
}

// analyse reads every session log and writes the report.
func analyse() {
	o, code, ok := parseArgs(os.Args[1:], os.Stdout, os.Stderr)
	if !ok {
		os.Exit(code)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	// The harnesses to read. Adding a harness is one line here.
	claude := claudecode.Reader{Home: home, ConfigDir: os.Getenv("CLAUDE_CONFIG_DIR")}
	readers := []source.Reader{claude}

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
	if st.logs == 0 {
		fmt.Fprintln(os.Stderr, noLogsMessage(claude.Projects()))
		os.Exit(1)
	}

	// 3. Build sessions.
	sessions := buildSessions(data)
	read := report.Read{Logs: st.logs, Failed: st.failed, BadLines: st.badLines}
	if !o.since.IsZero() {
		// Weekly figures from part of the logs would replace fuller stored
		// weeks, so a --since run leaves the history file alone.
		sessions = startedSince(sessions, o.since)
		read.Since = o.since.Format("2006-01-02")
		o.noHistory = true
		if len(sessions) == 0 {
			fmt.Fprintf(os.Stderr, "No sessions started on or after %s.\n", o.since.Format("2 Jan 2006"))
			os.Exit(1)
		}
	}

	// 4. Run the findings and write the report.
	settings := report.DefaultSettings()
	settings.Titles = !o.noTitles
	r := report.Analyse(sessions, read, settings, time.Now())
	// What the report needs from outside the logs: this build, the plan,
	// the status line's usage-limit readings (ADR 0048) and how long Claude
	// Code keeps logs (ADR 0050).
	readings, err := history.LoadLimits(history.LimitsPath(home, os.Getenv("CLAUDE_CONFIG_DIR")))
	if err != nil {
		log.Printf("usage-limit readings not used: %v", err)
	}
	plan := claude.Plan()
	days, set := claude.Retention()
	report.AddLocal(&r, sessions, report.Local{
		Version:   version(),
		Plan:      report.Plan{Name: plan.Name, Subscription: plan.Subscription},
		Readings:  readings,
		Retention: report.Retention{Days: days, Set: set},
	})
	if !o.noHistory {
		path := o.history
		if path == "" {
			path = defaultHistory(home, os.Getenv("CLAUDE_CONFIG_DIR"))
		}
		keepHistory(&r, path, st.oldest)
	}
	path, err := write(r, o.format, o.out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not write the report: %v\n", err)
		os.Exit(1)
	}
	if o.open {
		if err := openInBrowser(path); err != nil {
			log.Print(err) // the report is written; say why it did not open
		}
	}
}

// defaultHistory is where weekly figures are kept: one file per set of
// logs, so runs over another Claude Code folder (CLAUDE_CONFIG_DIR) do not
// replace each other's weeks.
func defaultHistory(home, configDir string) string {
	return history.PathFor(home, configDir, "history", ".json")
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

// reportName is the report's file name without its ending.
const reportName = "trimloop-report"

// defaultHTML is where an HTML report goes when no --out is given.
const defaultHTML = reportName + ".html"

// discard is the --out that throws the report away.
const discard = "/dev/null"

// toStdout reports whether --out path means standard output.
func toStdout(path string) bool { return path == "-" || path == "/dev/stdout" }

var validFormat = map[string]bool{"summary": true, "text": true, "html": true, "json": true}

// summary prints the short summary, pointing to the full formats.
func summary(out io.Writer, r report.Results) error { return report.WriteSummary(out, r, "", 3) }

// write renders r in format to path, or to standard output if path is empty,
// "-" or /dev/stdout (HTML goes to a file when path is empty); /dev/null
// throws it away. When the report goes to a file, a short summary is
// printed to the terminal.
// It returns the path written, "" when no file was written.
func write(r report.Results, format, path string) (string, error) {
	if format == "html" && path == "" {
		path = defaultHTML
	}
	render := map[string]func(io.Writer, report.Results) error{
		"summary": summary,
		"text":    report.WriteText,
		"json":    report.WriteJSON,
		"html":    report.WriteHTML,
	}[format]
	switch {
	case path == "" || toStdout(path):
		return "", render(os.Stdout, r)
	case path == discard:
		return "", render(io.Discard, r)
	}
	if err := writeFile(path, func(w io.Writer) error { return render(w, r) }); err != nil {
		return "", err
	}
	if format == "summary" {
		return path, nil
	}
	return path, report.WriteSummary(os.Stdout, r, path, 3)
}

// writeFile writes a file at path with render, readable by the owner only:
// a report names projects and shows session titles, which summarise what
// was asked. It writes a new file in the same folder and renames it over
// path, so a link at path is replaced, not followed: in a folder others can
// write to, a link planted there cannot make trimloop overwrite another
// file. Anything at path but a file or a link is refused.
func writeFile(path string, render func(io.Writer) error) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s exists and is not a file: choose another name", path)
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp") // made 0600
	if err != nil {
		return err
	}
	tmp := f.Name()
	done := false
	defer func() {
		if !done {
			_ = os.Remove(tmp)
		}
	}()
	if err := render(f); err != nil {
		_ = f.Close() // the write error is the one to report
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	done = true
	return nil
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

		if m.Model == claudecode.Synthetic {
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
		s.AgentType = claudecode.AgentType(d.log.Path)
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

// sessionFor names the session a log holds. A main session log is named after
// its session ID. A subagent log is shown with its session ID in front, so it
// can be traced back to the session that started it.
func sessionFor(path string) event.Session {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dir := filepath.Dir(path)
	if filepath.Base(dir) == "subagents" {
		parent := filepath.Base(filepath.Dir(dir))
		return event.Session{ID: parent + "/" + name, Kind: event.SubAgentSession, ParentID: parent}
	}
	return event.Session{ID: name, Kind: event.MainSession}
}
