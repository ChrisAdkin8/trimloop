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
	"strings"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
	"github.com/ChrisAdkin8/trimloop/internal/adapters/source/claudecode"
	"github.com/ChrisAdkin8/trimloop/internal/demo"
	"github.com/ChrisAdkin8/trimloop/internal/history"
	"github.com/ChrisAdkin8/trimloop/internal/live"
	"github.com/ChrisAdkin8/trimloop/internal/report"
	"github.com/ChrisAdkin8/trimloop/internal/safefile"
	"github.com/ChrisAdkin8/trimloop/internal/sessions"
	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "statusline", "hook", "setup":
			os.Exit(runLive(args[0], args[1:]))
		case "help":
			_, code, _ := parseArgs(false, []string{"--help"}, time.Now(), os.Stdout, os.Stderr)
			os.Exit(code)
		case "demo":
			os.Exit(runDemo(args[1:]))
		case "analyse", "analyze":
			args = args[1:]
		}
	}
	os.Exit(runAnalyse(args))
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

// resolved is the file path names once every link is followed, the same
// way setup --write follows them (live.LinkTarget), even to a file not made
// yet; then the links to its folder. When that folder is missing too, it is
// the cleaned absolute path.
func resolved(path string) string {
	path = live.LinkTarget(path)
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

// parseArgs reads the analyse command line, or the demo's (forDemo): args
// without the program name or the word analyse or demo. A --since span
// counts back from now. It returns false, with the exit code, when the run
// stops here: after --help or --version, printed to stdout (0), or after a
// mistake, explained in one line on stderr (2).
func parseArgs(forDemo bool, args []string, now time.Time, stdout, stderr io.Writer) (analyseOptions, int, bool) {
	var o analyseOptions
	fs := flag.NewFlagSet("trimloop", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // a mistake is explained below, in one line
	fs.Usage = func() {}
	fs.StringVar(&o.format, "format", "summary", "report format: summary (a few lines), text (the full report), html, json, or share (a few lines with no names, safe to paste elsewhere)")
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
	formatSet, historyFlag := false, ""
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "format":
			formatSet = true
		case "history", "no-history":
			historyFlag = f.Name
		}
	})
	if o.open && !formatSet {
		o.format = "html"
	}
	if err := checkOptions(cliOptions{
		args:        fs.Args(),
		first:       !forDemo && fs.NArg() == len(args), // nothing came before the first word
		format:      o.format,
		open:        o.open,
		out:         o.out,
		demo:        forDemo,
		historyFlag: historyFlag,
	}); err != nil {
		fmt.Fprintf(stderr, "%v.\n%s", err, helpHint)
		return o, 2, false
	}
	if *sinceArg != "" {
		if o.since, err = parseSince(*sinceArg, now); err != nil {
			fmt.Fprintln(stderr, err)
			return o, 2, false
		}
	}
	return o, 0, true
}

// usage prints how to run trimloop and every analyse option.
func usage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprint(w, "Usage: trimloop [analyse] [--format summary|text|html|json|share] [--out FILE] [--open] [--since DATE]\n"+
		"                         [--no-titles] [--history FILE | --no-history]\n"+
		"       trimloop demo [--format summary|text|html|json|share] [--out FILE] [--open] [--since DATE] [--no-titles]\n"+
		"       trimloop --version\n"+
		"       trimloop setup [--write SETTINGS_FILE] [--replace-statusline]\n"+
		"       trimloop setup --check [--settings SETTINGS_FILE]\n"+
		"       trimloop statusline | trimloop hook stop|prompt   (run by Claude Code)\n\n"+
		"Analyses Claude Code session logs on this machine for token waste. 'demo' shows the\n"+
		"report on made-up logs. 'setup' adds live nudges to Claude Code: a status line and\n"+
		"two hooks.\n\n")
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

// runAnalyse reads every session log on this machine and writes the
// report. It returns the exit code.
func runAnalyse(args []string) int {
	now := time.Now()
	o, code, ok := parseArgs(false, args, now, os.Stdout, os.Stderr)
	if !ok {
		return code
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Print(err)
		return 1
	}
	cfgDir := os.Getenv("CLAUDE_CONFIG_DIR")
	claude := claudecode.Reader{Home: home, ConfigDir: cfgDir}

	// What the report needs from outside the logs: this build, the plan,
	// the status line's usage-limit readings (ADR 0048) and how long Claude
	// Code keeps logs (ADR 0050).
	readings, err := history.LoadLimits(history.LimitsPath(home, cfgDir))
	if err != nil {
		log.Printf("usage-limit readings not used: %v", err)
	}
	plan := claude.Plan()
	days, set := claude.Retention()
	a := analysis{
		claude: claude,
		local: report.Local{
			Version:   version(),
			Plan:      report.Plan{Name: plan.Name, Subscription: plan.Subscription},
			Readings:  readings,
			Retention: report.Retention{Days: days, Set: set},
		},
		now: now,
	}
	if !o.noHistory {
		a.history = o.history
		if a.history == "" {
			a.history = defaultHistory(home, cfgDir)
		}
	}
	return analyse(o, a)
}

// demoBanner says, on standard error so that JSON on standard output stays
// valid, whose logs a demo report is about.
const demoBanner = "trimloop demo: these are made-up logs of an invented developer, not yours; run 'trimloop' to analyse your own."

// runDemo writes the report on the made-up logs in internal/demo, so that
// someone with few logs of their own can see one (ADR 0055). Nothing of the
// user's is read or written: not their logs, plan, usage-limit readings or
// log retention setting, nor the history file. It returns the exit code.
func runDemo(args []string) int {
	o, code, ok := parseArgs(true, args, demo.Now, os.Stdout, os.Stderr)
	if !ok {
		return code
	}
	// The log reader reads files, so the logs are copied out for the run.
	dir, err := os.MkdirTemp("", "trimloop-demo-")
	if err != nil {
		log.Print(err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			log.Print(err)
		}
	}()
	if err := os.CopyFS(dir, demo.Logs()); err != nil {
		log.Print(err)
		return 1
	}
	fmt.Fprintln(os.Stderr, demoBanner)
	return analyse(o, analysis{
		claude: claudecode.Reader{ConfigDir: dir},
		local:  report.Local{Version: version()},
		now:    demo.Now,
	})
}

// analysis is what a report is made from besides the command line: the
// logs, what is known from outside them, the clock, and the history file
// to keep ("" for none).
type analysis struct {
	claude  claudecode.Reader
	local   report.Local
	now     time.Time
	history string
}

// analyse reads the logs a names and writes the report o asks for. It
// returns the exit code.
func analyse(o analyseOptions, a analysis) int {
	// The harnesses to read. Adding a harness is one line here.
	readers := []source.Reader{a.claude}

	// 1. Find and read every log, and build their sessions.
	all, st, err := sessions.Load(readers)
	if err != nil {
		log.Print(err)
		return 1
	}
	for _, p := range st.Problems {
		log.Print(p) // a log that failed partway: what was read is kept
	}
	if st.Logs == 0 {
		fmt.Fprintln(os.Stderr, noLogsMessage(a.claude.Projects()))
		return 1
	}
	read := report.Read{Logs: st.Logs, Failed: len(st.Problems), BadLines: st.BadLines}
	if !o.since.IsZero() {
		// Weekly figures from part of the logs would replace fuller stored
		// weeks, so a --since run leaves the history file alone.
		all = startedSince(all, o.since)
		read.Since = o.since.Format("2006-01-02")
		a.history = ""
		if len(all) == 0 {
			fmt.Fprintf(os.Stderr, "No sessions started on or after %s.\n", o.since.Format("2 Jan 2006"))
			return 1
		}
	}

	// 2. Run the findings and write the report.
	settings := report.DefaultSettings()
	settings.Titles = !o.noTitles
	settings.Location = a.now.Location() // weeks in the clock's zone: local time, or the demo's
	r := report.Analyse(all, read, a.local, settings, a.now)
	if a.history != "" {
		keepHistory(&r, a.history, st.Oldest)
	}
	path, err := write(r, o.format, o.out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not write the report: %v\n", err)
		return 1
	}
	if o.open {
		if err := openInBrowser(path); err != nil {
			log.Print(err) // the report is written; say why it did not open
		}
	}
	return 0
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

var validFormat = map[string]bool{"summary": true, "text": true, "html": true, "json": true, "share": true}

// summary prints the short summary, pointing to the full formats.
func summary(out io.Writer, r report.Results) error { return report.WriteSummary(out, r, "", 3) }

// share prints the few lines that are safe to paste where others read them.
func share(out io.Writer, r report.Results) error { return report.WriteShare(out, r, 3) }

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
		"share":   share,
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
	if format == "summary" || format == "share" {
		return path, nil // short already: no summary of it
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
	return safefile.Write(path, 0o600, render)
}
