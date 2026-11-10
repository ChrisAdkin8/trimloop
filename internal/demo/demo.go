// Package demo holds the made-up Claude Code logs that 'trimloop demo'
// reports on, so someone with few or no logs of their own can see what a
// report looks like (ADR 0055). scripts/gen-demo.py writes them into data;
// edit the script, not the files, and regenerate.
package demo

import (
	"embed"
	"io/fs"
	"time"
)

//go:embed data
var data embed.FS

// Logs is a Claude Code folder, laid out as CLAUDE_CONFIG_DIR is, holding
// five weeks of an invented developer's logs.
func Logs() fs.FS {
	sub, err := fs.Sub(data, "data")
	if err != nil {
		panic(err) // "data" is a valid path, so fs.Sub cannot fail
	}
	return sub
}

// Now is the demo's clock: the morning after its last reply, in UTC, so
// the report and a --since span come out the same on every machine and on
// every date.
var Now = time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
