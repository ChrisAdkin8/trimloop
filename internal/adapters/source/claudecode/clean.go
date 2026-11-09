package claudecode

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// unsafeRune reports whether r must not reach a report: a control character
// (ESC, BEL, the C1 range), which a terminal can act on, or a format
// character (unicode.Cf: bidi overrides such as U+202E, zero-width
// characters such as U+200B, U+FEFF), which can hide or reorder what is
// shown. Zero-width joiners are format characters too, so an emoji built
// from several (a family) shows as its parts.
func unsafeRune(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

// clean makes a string from a log safe to show: it removes control and
// format characters and invalid UTF-8. Every human-readable string Read
// passes on goes through it once, here at ingestion (titles, model ids,
// effort, attribution, tool and server names, memory-file names); reports
// keep a second guard of their own. Ordinary text in any script, and
// emoji, are unchanged.
func clean(s string) string {
	if isClean(s) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if unsafeRune(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}

// cleanFolder is clean for a project folder, which is compared with other
// folders and named by its last elements (ADR 0032): each unsafe character
// or run of invalid UTF-8 becomes U+FFFD rather than going, so a real
// folder "a\u200bb" and a real folder "ab" stay two projects. Only folders
// that differ in nothing but which unsafe character they hold could merge;
// real folder names hardly ever hold one at all. File identities are
// hashed from the folder as logged (fileKey), not from this.
func cleanFolder(s string) string {
	if isClean(s) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if unsafeRune(r) {
			return unicode.ReplacementChar
		}
		return r
	}, strings.ToValidUTF8(s, string(unicode.ReplacementChar)))
}

// isClean reports whether s needs no cleaning, so the common case of plain
// text allocates nothing.
func isClean(s string) bool {
	return utf8.ValidString(s) && !strings.ContainsFunc(s, unsafeRune)
}
