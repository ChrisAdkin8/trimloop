package cmdshape

import "strings"

// token is one word or operator of a shell command.
type token struct {
	op   string // operator: "|", "&&", ";", ">", "<<", ...; empty for a word
	word string // the word, with quotes removed
}

// lex splits a shell command into words and operators. It is a simple
// tokenizer, not a full shell parser: it handles quotes, escapes, $(...) and
// backquotes, comments, line continuations and here-documents, which covers
// the commands coding agents write. Anything it does not understand ends up
// inside a word, which at worst gives a less specific shape.
func lex(s string) []token {
	var toks []token
	var word strings.Builder
	inWord := false
	var heredocs []heredoc // here-documents whose bodies start after the next newline

	flush := func() {
		if inWord {
			toks = append(toks, token{word: word.String()})
			word.Reset()
			inWord = false
		}
	}
	emit := func(op string) {
		flush()
		toks = append(toks, token{op: op})
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s):
			if s[i+1] == '\n' { // line continuation
				i++
				continue
			}
			word.WriteByte(s[i+1])
			inWord = true
			i++
		case c == '\'':
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				end = len(s) - i - 1
			}
			word.WriteString(s[i+1 : i+1+end])
			inWord = true
			i += end + 1
		case c == '"':
			j := i + 1
			for ; j < len(s) && s[j] != '"'; j++ {
				if s[j] == '\\' && j+1 < len(s) {
					j++
				}
				word.WriteByte(s[j])
			}
			inWord = true
			i = j
		case c == '$' && i+1 < len(s) && s[i+1] == '(':
			end := closing(s, i+1)
			word.WriteString(s[i:end])
			inWord = true
			i = end - 1
		case c == '`':
			end := strings.IndexByte(s[i+1:], '`')
			inWord = true
			if end < 0 { // never closed: the rest is one word
				word.WriteString(s[i:])
				i = len(s)
				continue
			}
			word.WriteString(s[i : i+2+end])
			i += end + 1
		case c == '#' && !inWord:
			for i+1 < len(s) && s[i+1] != '\n' {
				i++
			}
		case c == ' ' || c == '\t':
			flush()
		case c == '\n':
			emit(";")
			if len(heredocs) > 0 {
				i = skipHeredocs(s, i+1, heredocs) - 1
				heredocs = nil
			}
		case c == '|' || c == '&' || c == ';' || c == '<' || c == '>':
			// A digit just before a redirection is its file descriptor: 2>.
			fd := ""
			if (c == '>' || c == '<') && inWord && word.Len() == 1 && word.String()[0] >= '0' && word.String()[0] <= '9' {
				fd = word.String()
				word.Reset()
				inWord = false
			}
			op := operator(s[i:])
			if op == "<<" || op == "<<-" {
				flush()
				toks = append(toks, token{op: op})
				i += len(op)
				delim, n := heredocDelim(s[i:])
				heredocs = append(heredocs, heredoc{delim: delim, strip: op == "<<-"})
				toks = append(toks, token{word: delim})
				i += n - 1
				continue
			}
			emit(fd + op)
			i += len(op) - 1
		case c == '(' || c == ')' || c == '{' || c == '}':
			if inWord && c != '(' && c != ')' {
				word.WriteByte(c) // part of a word such as ${x} or a{b,c}
				continue
			}
			emit(";") // a subshell or group: treat as a command boundary
		default:
			word.WriteByte(c)
			inWord = true
		}
	}
	flush()
	return toks
}

// operator returns the shell operator at the start of s.
func operator(s string) string {
	for _, op := range []string{"&&", "||", "|&", ";;", "<<<", "<<-", "<<", ">>", ">&", "&>>", "&>", "<&", "|", "&", ";", "<", ">"} {
		if strings.HasPrefix(s, op) {
			return op
		}
	}
	return s[:1]
}

// closing returns the index just past the parenthesis that closes the one
// at s[open], allowing for nesting and quotes, or len(s) if there is none.
func closing(s string, open int) int {
	depth := 0
	var quote byte
	for i := open; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(s)
}

type heredoc struct {
	delim string
	strip bool // <<- allows the closing line to be indented with tabs
}

// heredocDelim reads a here-document's delimiter word, quoted or not, at the
// start of s (after optional spaces). It returns the delimiter and how many
// bytes it used.
func heredocDelim(s string) (string, int) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	start := i
	for i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != ';' && s[i] != '|' && s[i] != '&' && s[i] != ')' {
		i++
	}
	delim := strings.Trim(s[start:i], `'"`)
	delim = strings.TrimPrefix(delim, `\`)
	return delim, i
}

// skipHeredocs skips the bodies of here-documents starting at s[i], one after
// another, and returns the index of the first byte after the last closing
// line (or len(s) if a body is never closed).
func skipHeredocs(s string, i int, docs []heredoc) int {
	for _, d := range docs {
		for i < len(s) {
			end := strings.IndexByte(s[i:], '\n')
			line := s[i:]
			if end >= 0 {
				line = s[i : i+end]
			}
			if d.strip {
				line = strings.TrimLeft(line, "\t")
			}
			if end < 0 {
				i = len(s)
			} else {
				i += end + 1
			}
			if line == d.delim {
				break
			}
		}
	}
	return i
}
