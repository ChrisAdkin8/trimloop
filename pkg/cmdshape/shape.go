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

	File  string // the file a file-viewing or editing command names; "" if none
	Edit  bool   // the command edits File in place (sed -i)
	Noise event.NoiseKind
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
	var first []pipePart
	for _, c := range cmds {
		if len(c) == 0 || len(c[0].words) == 0 {
			continue
		}
		if first == nil {
			first = c
		}
		if !setup[path.Base(c[0].words[0])] {
			return shapeOf(c)
		}
	}
	if first == nil {
		return Shape{}
	}
	return shapeOf(first)
}

// pipePart is one command in a pipeline: its words, with prefixes such as
// "sudo" and "FOO=bar" removed, and where its output goes.
type pipePart struct {
	words    []string
	toFile   bool // stdout is redirected to a file (including /dev/null)
	heredoc  bool // stdin is a here-document or here-string
	fromFile bool // stdin is redirected from a file
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
				// stderr, or stdout duplicated onto stderr: not a file
			case strings.HasSuffix(t.op, ">"):
				cur.toFile = target != "/dev/stderr" && target != "/dev/stdout"
			}
		}
	}
	endCmd()
	return cmds
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
			w = w[1:]
			// Drop the wrapper's own options, and the duration timeout takes.
			for len(w) > 0 && strings.HasPrefix(w[0], "-") {
				w = w[1:]
			}
			if head == "timeout" && len(w) > 0 {
				w = w[1:]
			}
		default:
			return w
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

	words := p.words
	prog := path.Base(words[0])
	args := words[1:]

	// Runners: the program that matters is the one they run.
	for {
		if subs, ok := runners[prog]; ok {
			rest := args
			if len(subs) > 0 { // e.g. "uv run pytest", but not "uv sync"
				if len(rest) == 0 || !slices.Contains(subs, rest[0]) {
					break
				}
				rest = rest[1:]
			}
			for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
				rest = rest[1:]
			}
			if len(rest) == 0 {
				break
			}
			prog, args = path.Base(rest[0]), rest[1:]
			continue
		}
		// python -m pytest runs pytest.
		if interpreters[prog] && len(args) >= 2 && args[0] == "-m" {
			prog, args = args[1], args[2:]
			continue
		}
		break
	}
	if strings.ContainsAny(prog[:min(1, len(prog))], "$`(") {
		prog = "(computed command)" // never show what a substitution contains
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
		s.File = fileArg(prog, args, p)
		s.Edit = prog == "sed" && slices.Contains(s.Flags, "-i")
		if s.File == "" && s.Noise == event.NoiseFileView {
			s.Noise = event.NoiseOther // reads stdin: not a file view
		}
	}
	return s
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

// fileArg returns the file a file-viewing command (cat, head, sed ...) reads,
// or "" if it reads stdin, writes a file, or names none.
func fileArg(prog string, args []string, p pipePart) string {
	if p.toFile && prog != "sed" { // cat > file <<EOF writes a file
		return ""
	}
	var operands []string
	script := prog == "sed"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-e" || a == "--expression":
			script = false // the script is the option's value
			i++
		case a == "-n" && (prog == "head" || prog == "tail"), a == "-c" && (prog == "head" || prog == "tail"):
			i++ // the line or byte count
		case strings.HasPrefix(a, "-") && a != "-":
			continue
		case a == "" || isNumber(a): // '' is macOS sed -i's empty suffix
			continue
		default:
			operands = append(operands, a)
		}
	}
	if script && len(operands) > 0 {
		operands = operands[1:] // sed's first operand is its script
	}
	if len(operands) == 0 || operands[0] == "-" {
		return ""
	}
	return operands[0]
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
