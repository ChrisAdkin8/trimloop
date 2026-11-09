// Package cmdshape turns a shell command into its shape: the program, its
// subcommand and the flags that change how much it prints, whether the
// output was filtered, and the file it views. Shapes group commands that
// produce the same kind of output, so a fix can name the flag or filter to
// use (docs/patterns.md, P04).
//
// A shape is derived on the developer's machine. It never includes argument
// values, so "sed -n '1,50p' main.go" and "sed -n '9,20p' util.go" share the
// shape "sed -n". The viewed file is returned separately, for spotting
// re-reads, and is not part of the shape.
package cmdshape

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// Shape is the shape of one shell command.
type Shape struct {
	Program string   // e.g. "sed", "go", "python3"
	Sub     string   // subcommand, e.g. "test" in "go test"; "inline" for python3 -c
	Flags   []string // output-size flags present, in a fixed order

	// Filtered is true when the output went through a filter (head, tail,
	// grep, jq, ...) or to a file, so the model saw less than the program
	// printed.
	Filtered bool

	// File is the file a file-viewing or editing command names; "" if none,
	// or if which file it is cannot be told (after a cd to a folder that is
	// not written out). A relative name is relative to the folder the
	// command line started in: a cd earlier in it is applied ("cd web &&
	// cat a.ts" views web/a.ts).
	File string
	// Others are the further files a view names after File ("cat a.go
	// b.go"), relative in the same way; a view shows each the same way.
	Others []string
	Edit   bool // the command edits File in place (sed -i)
	// Part is the part of File a view prints: sed's script, or head's or
	// tail's program and count; "" for the whole file. Like File, it is not
	// part of the shape and may hold values.
	Part  string
	Noise event.NoiseKind

	// Writes lists the files any command in cmd writes or changes: the
	// targets of redirections and tee, and the files sed -i edits, relative
	// like File.
	Writes []string
	// WritesUnknown is true when a command may change files it does not
	// name (git checkout, mv, a formatter, perl -i), or writes a relative
	// file after a cd that cannot be followed.
	WritesUnknown bool
}

// String is the shape as shown in reports: program, subcommand, then flags,
// e.g. "go test -v" or "sed -n".
func (s Shape) String() string {
	parts := []string{s.Program}
	if s.Sub != "" {
		parts = append(parts, s.Sub)
	}
	return strings.Join(append(parts, s.Flags...), " ")
}

// Parse returns the shape of cmd. When cmd runs several commands, the shape
// is that of the first one that does real work, skipping set-up such as cd
// and export: in "cd app && go test ./... | tail -5" it is "go test", filtered.
func Parse(cmd string) Shape {
	cmds := split(lex(cmd))
	dirs := folders(cmds)
	i, s := describe(cmds)
	if s.File != "" {
		files := append([]string{s.File}, s.Others...)
		s.File, s.Others = "", nil
		for k, f := range files {
			f, ok := dirs[i].join(f)
			if !ok {
				s.File, s.Others = "", nil // which files is not known
				break
			}
			if k == 0 {
				s.File = f
			} else {
				s.Others = append(s.Others, f)
			}
		}
	}
	s.Writes, s.WritesUnknown = writes(cmds, dirs)
	return s
}

// folder is the folder a command runs in, relative to the one the command
// line started in: path "" is that folder; one under the home folder starts
// with "~/". known is false after a cd the shape cannot follow (cd alone,
// cd -, another user's ~user, a variable, popd).
type folder struct {
	path  string
	known bool
}

// join names file as seen from the command line's starting folder, and
// false when that cannot be told. An absolute file, or one under the home
// folder (~/...), is itself.
func (f folder) join(file string) (string, bool) {
	switch {
	case path.IsAbs(file) || inHome(file):
		return file, true
	case !f.known:
		return "", false
	}
	return path.Join(f.path, file), true
}

// folders works out the folder each command in cmds runs in, following cd
// and pushd from command to command. A subshell's cd ends with it; a cd
// inside a group ({ }) does not.
func folders(cmds [][]pipePart) []folder {
	dirs := make([]folder, len(cmds))
	cur := folder{known: true}
	var outer []folder // the folder outside each open subshell
	for i, c := range cmds {
		dirs[i] = cur
		if len(c) == 0 {
			continue
		}
		switch c[0].mark {
		case "(":
			outer = append(outer, cur)
			continue
		case ")":
			if n := len(outer); n > 0 {
				cur, outer = outer[n-1], outer[:n-1]
			}
			continue
		}
		if len(c[0].words) == 0 {
			continue
		}
		switch path.Base(c[0].words[0]) {
		case "cd", "pushd":
			cur = cd(cur, skipOptions(c[0].words[1:], nil))
		case "popd":
			cur = folder{}
		}
	}
	return dirs
}

// inHome reports whether a path is written from the home folder: ~ or ~/...
// (not ~user, another user's).
func inHome(p string) bool { return p == "~" || strings.HasPrefix(p, "~/") }

// cd is the folder after cd with args, from cur.
func cd(cur folder, args []string) folder {
	if len(args) != 1 || args[0] == "-" || strings.HasPrefix(args[0], "~") && !inHome(args[0]) ||
		strings.ContainsAny(args[0], "$`*?") {
		return folder{} // the previous folder, another user's, or one only the shell knows
	}
	to, ok := cur.join(args[0])
	if !ok {
		return folder{}
	}
	return folder{path: path.Clean(to), known: true}
}

// describe returns the shape of the first command in cmds that does real
// work, and its index.
func describe(cmds [][]pipePart) (int, Shape) {
	first := -1
	for i, c := range cmds {
		if len(c) == 0 || len(c[0].words) == 0 {
			continue
		}
		if first < 0 {
			first = i
		}
		if !setup[path.Base(c[0].words[0])] {
			return i, shapeOf(c)
		}
	}
	if first < 0 {
		return 0, Shape{}
	}
	return first, shapeOf(cmds[first])
}

// writes lists the files the commands write or change, and whether any
// command may change files it does not name. Every command counts, not
// only the one the shape describes: in "cat > a.go <<EOF ... EOF && go
// build", the shape is go build's, but a.go has changed.
func writes(cmds [][]pipePart, dirs []folder) (files []string, unknown bool) {
	for i, c := range cmds {
		add := func(names ...string) {
			for _, n := range names {
				if f, ok := dirs[i].join(n); ok {
					files = append(files, f)
				} else {
					unknown = true // a file in a folder that is not known
				}
			}
		}
		for _, p := range c {
			add(p.writes...)
			if len(p.words) == 0 {
				continue
			}
			_, prog, args := resolve(p.words)
			switch {
			case prog == "tee":
				for _, a := range args {
					if !strings.HasPrefix(a, "-") {
						add(a)
					}
				}
			case prog == "sed" && inPlace(args):
				named, _ := fileArgs(prog, args)
				add(named...)
			case changesUnnamed(prog, args):
				unknown = true
			}
		}
	}
	return files, unknown
}

// inPlace reports whether sed's arguments ask it to edit files in place:
// -i, -i.bak, a bundle such as -ni, or --in-place. No other short flag of
// sed has an i, and only -i's suffix follows it in the same word.
func inPlace(args []string) bool {
	for _, a := range args {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "--in-place") ||
			strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "i") {
			return true
		}
	}
	return false
}

// changesUnnamed reports whether a program, run with args, may change files
// without naming them as a plain operand: moving, copying or deleting
// files, switching git's working tree, formatting in place, or adding a
// dependency (which rewrites the project's manifest).
func changesUnnamed(prog string, args []string) bool {
	switch {
	case unnamedWriters[prog]:
		return true
	case prog == "perl" || prog == "ruby":
		for _, a := range args {
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "i") {
				return true // perl -pi -e, ruby -i.bak
			}
		}
	case writeFlag[prog] != nil:
		for _, a := range args {
			name, _, _ := strings.Cut(a, "=")
			if slices.Contains(writeFlag[prog], name) {
				return true
			}
		}
	case writingSubs[prog] != nil:
		return slices.Contains(writingSubs[prog], subcommand(prog, args))
	}
	return false
}

// pipePart is one command in a pipeline: its words, with prefixes such as
// "sudo" and "FOO=bar" removed, and where its output goes.
type pipePart struct {
	mark     string // "(" or ")" for a subshell's start or end, which has no words
	words    []string
	toFile   bool     // stdout is redirected to a file (including /dev/null)
	heredoc  bool     // stdin is a here-document or here-string
	fromFile bool     // stdin is redirected from a file
	writes   []string // files a redirection writes, stdout's or another's
}

// split groups tokens into commands (separated by ;, &&, ||, & and newlines),
// each a pipeline of parts.
func split(toks []token) [][]pipePart {
	var cmds [][]pipePart
	var pipe []pipePart
	var cur pipePart
	endPart := func() {
		cur.words = strip(cur.words)
		pipe = append(pipe, cur)
		cur = pipePart{}
	}
	endCmd := func() {
		endPart()
		cmds = append(cmds, pipe)
		pipe = nil
	}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.op {
		case "":
			cur.words = append(cur.words, t.word)
		case "|", "|&":
			endPart()
		case "&&", "||", ";", ";;", "&":
			endCmd()
		case ";(", ";)":
			endCmd()
			cmds = append(cmds, []pipePart{{mark: t.op[1:]}})
		default: // a redirection; its target is the next word
			target := ""
			if i+1 < len(toks) && toks[i+1].op == "" {
				target = toks[i+1].word
				i++
			}
			switch {
			case t.op == "<<" || t.op == "<<-" || t.op == "<<<":
				cur.heredoc = true
			case t.op == "<":
				cur.fromFile = true
			case strings.HasPrefix(t.op, "2>"), t.op == ">&" && target != "1" && target != "-":
				// stderr, or stdout duplicated onto stderr: not stdout to a file
			case strings.HasSuffix(t.op, ">"):
				cur.toFile = target != "/dev/stderr" && target != "/dev/stdout"
			}
			if writesFile(t.op, target) {
				cur.writes = append(cur.writes, target)
			}
		}
	}
	endCmd()
	return cmds
}

// writesFile reports whether a redirection writes to the file target: any
// output redirection (>, >>, 2>, &>), except one that duplicates a
// descriptor (2>&1, >&-) or names a device.
func writesFile(op, target string) bool {
	if !strings.Contains(op, ">") || target == "" || strings.HasPrefix(target, "/dev/") {
		return false
	}
	if strings.HasSuffix(op, ">&") && (target == "-" || isNumber(target)) {
		return false
	}
	return true
}

// strip removes words in front of the program that do not change what it
// prints: variable assignments, wrappers such as sudo and timeout, and shell
// keywords such as "do" and "then".
func strip(w []string) []string {
	for len(w) > 0 {
		head := w[0]
		switch {
		case isAssignment(head):
			w = w[1:]
		case prefixes[head]:
			// Drop the wrapper's own options and their values, and the
			// duration timeout takes.
			w = skipOptions(w[1:], wrapperValueOpts[head])
			if head == "timeout" && len(w) > 0 {
				w = w[1:]
			}
		default:
			return w
		}
	}
	return w
}

// skipOptions drops leading options, and the value of each one listed in
// valued, so a value (a user, a package, a variable name) is never taken for
// the program. "--" ends the options.
func skipOptions(w []string, valued []string) []string {
	for len(w) > 0 && strings.HasPrefix(w[0], "-") && w[0] != "-" {
		opt := w[0]
		w = w[1:]
		if opt == "--" {
			break
		}
		if !strings.Contains(opt, "=") && slices.Contains(valued, opt) && len(w) > 0 {
			w = w[1:]
		}
	}
	return w
}

func isAssignment(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	for i, c := range w[:eq] {
		letter := c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		digit := i > 0 && c >= '0' && c <= '9'
		if !letter && !digit {
			return false
		}
	}
	return true
}

// shapeOf works out the shape of one pipeline.
func shapeOf(pipe []pipePart) Shape {
	p := pipe[0]
	s := Shape{Filtered: p.toFile}
	for _, later := range pipe[1:] {
		if len(later.words) > 0 && filters[path.Base(later.words[0])] {
			s.Filtered = true
		}
		if later.toFile {
			s.Filtered = true
		}
	}

	raw, prog, args := resolve(p.words)
	if strings.ContainsAny(raw, "$`(") || !progName.MatchString(prog) {
		prog = "(computed command)" // never show what a substitution or odd word contains
	}
	s.Program = prog

	switch {
	case interpreters[prog]:
		s.Sub = interpreterSub(args, p.heredoc)
	case subcommands[prog] > 0:
		s.Sub = subcommand(prog, args)
	}
	s.Flags = sizeFlags(prog, args)
	s.Noise = noise(s)
	if fileViewers[prog] || prog == "sed" {
		s.Edit = prog == "sed" && inPlace(args)
		if !p.toFile || prog == "sed" { // cat > file <<EOF writes a file
			if files, part := fileArgs(prog, args); len(files) > 0 {
				s.File = files[0]
				if !s.Edit {
					s.Others = files[1:]
				}
				if !s.Edit {
					s.Part = part
				}
			}
		}
		if s.File == "" && s.Noise == event.NoiseFileView {
			s.Noise = event.NoiseOther // reads stdin: not a file view
		}
	}
	return s
}

// resolve returns the program a command's words run, and its arguments.
// For a runner (npx, uv run, xargs, python -m) it is the program the runner
// runs. raw is the word the program name came from.
func resolve(words []string) (raw, prog string, args []string) {
	prog = path.Base(words[0])
	args = words[1:]
	raw = words[0]
	for {
		if subs, ok := runners[prog]; ok {
			rest := args
			if len(subs) > 0 { // e.g. "uv run pytest", but not "uv sync"
				if len(rest) == 0 || !slices.Contains(subs, rest[0]) {
					break
				}
				rest = rest[1:]
			}
			rest = skipOptions(rest, runnerValueOpts[prog])
			if len(rest) == 0 {
				break
			}
			raw = rest[0]
			prog, args = path.Base(rest[0]), rest[1:]
			continue
		}
		// python -m pytest runs pytest.
		if interpreters[prog] && len(args) >= 2 && args[0] == "-m" {
			raw = args[1]
			prog, args = args[1], args[2:]
			continue
		}
		break
	}
	return raw, prog, args
}

// interpreterSub tells inline code (python3 -c, node -e, a here-document)
// from running a script file.
func interpreterSub(args []string, heredoc bool) string {
	for _, a := range args {
		switch {
		case a == "-c" || a == "-e" || a == "-":
			return "inline"
		case strings.HasPrefix(a, "-"):
			continue
		default:
			return "script"
		}
	}
	if heredoc {
		return "inline"
	}
	return ""
}

// subcommand returns the first one or two subcommand words. A shape must
// never carry a value (a token, a profile, a path), so it keeps only words
// that look like subcommands: an option before the subcommand is taken to
// have a value, unless it is a known on/off flag or written --opt=value, and
// a word that is not a plain lower-case word ends the subcommand.
func subcommand(prog string, args []string) string {
	var sub []string
	for i := 0; i < len(args) && len(sub) < subcommands[prog]; i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if len(sub) > 0 {
				break // options after the subcommand end it
			}
			if !strings.Contains(a, "=") && !slices.Contains(globalFlags[prog], a) {
				i++ // skip its value
			}
			continue
		}
		if !subWord.MatchString(a) {
			break
		}
		sub = append(sub, a)
	}
	return strings.Join(sub, " ")
}

// progName is what a program name looks like: "go", "python3.12", "check.sh".
var progName = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)

// subWord is what a subcommand looks like: "test", "pr", "ls-files".
var subWord = regexp.MustCompile(`^[a-z][a-z0-9-]{0,23}$`)

// sizeFlags returns the flags in args that change how much prog prints, in
// the order of the program's flag list.
func sizeFlags(prog string, args []string) []string {
	known, ok := flagsByProgram[prog]
	if !ok {
		known = defaultFlags
	}
	present := map[string]bool{}
	var value string // the value of a flag listed with "=", e.g. "-o yaml"
	for i, a := range args {
		if a == "--" {
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			continue
		}
		name, val, hasVal := strings.Cut(a, "=")
		switch {
		case strings.HasPrefix(name, "--"):
			present[name] = true
		case bundles[prog] && len(name) > 2:
			for _, c := range name[1:] {
				if c >= '0' && c <= '9' {
					break // -m5: the value follows
				}
				present["-"+string(c)] = true
			}
		default:
			present[name] = true
		}
		if slices.Contains(known, name+"=") {
			if !hasVal && i+1 < len(args) {
				val = args[i+1]
			}
			value = name
			if subWord.MatchString(val) { // "-o yaml", but not a template or path
				value += " " + val
			}
		}
	}
	var out []string
	for _, f := range known {
		if strings.HasSuffix(f, "=") {
			if name := strings.TrimSuffix(f, "="); present[name] && value != "" {
				out = append(out, value)
			}
			continue
		}
		if present[f] {
			out = append(out, f)
		}
	}
	return out
}

// fileArgs returns the files a file-viewing command (cat, head, sed ...)
// names, none if it reads stdin, and the part of a file it prints: sed's
// script, or head's or tail's program and count ("" for the whole file).
// Paging through a file with sed -n or head and tail views different parts,
// which are not re-reads of each other.
func fileArgs(prog string, args []string) (files []string, part string) {
	var operands, scripts []string
	script := prog == "sed"
	counted := prog == "head" || prog == "tail"
	count := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-e" || a == "--expression":
			script = false // the script is the option's value
			if i+1 < len(args) {
				scripts = append(scripts, args[i+1])
			}
			i++
		case strings.HasPrefix(a, "--expression="):
			script = false
			scripts = append(scripts, strings.TrimPrefix(a, "--expression="))
		case counted && (a == "-n" || a == "-c"):
			if i+1 < len(args) {
				count = a + " " + args[i+1]
			}
			i++ // the line or byte count
		case counted && (strings.HasPrefix(a, "-n") || strings.HasPrefix(a, "-c") || strings.HasPrefix(a, "--lines") ||
			strings.HasPrefix(a, "--bytes") || isNumber(a)):
			count = a // -n50, --lines=50, -50, +10
		case strings.HasPrefix(a, "-") && a != "-":
			continue
		case a == "" || isNumber(a): // '' is macOS sed -i's empty suffix
			continue
		default:
			operands = append(operands, a)
		}
	}
	if script && len(operands) > 0 {
		scripts = append(scripts, operands[0]) // sed's first operand is its script
		operands = operands[1:]
	}
	switch {
	case prog == "sed":
		part = strings.Join(scripts, "\n")
	case counted:
		part = strings.TrimSpace(prog + " " + count)
	}
	if len(operands) == 0 || operands[0] == "-" {
		return nil, part
	}
	return operands, part
}

func isNumber(s string) bool {
	s = strings.TrimLeft(s, "+-")
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// noise sorts a shape into a kind of noise.
func noise(s Shape) event.NoiseKind {
	switch {
	case s.Program == "sed":
		return event.NoiseFileView // sed -n 'X,Yp' file, or sed 's/../../' file printing it all
	case fileViewers[s.Program]:
		return event.NoiseFileView
	case searchers[s.Program], s.Program == "git" && (s.Sub == "grep" || s.Sub == "ls-files"):
		return event.NoiseSearch
	case builders[s.Program]:
		return event.NoiseBuildTest
	case dataDumpers[s.Program], scriptLangs[s.Program] && s.Sub != "",
		s.Program == "git" && (s.Sub == "diff" || s.Sub == "show" || s.Sub == "log"),
		s.Program == "gh" && strings.HasPrefix(s.Sub, "api"):
		return event.NoiseData
	}
	return event.NoiseOther
}
