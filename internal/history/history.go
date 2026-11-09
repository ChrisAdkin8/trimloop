// Package history keeps each week's figures between runs, so a fix can be
// compared before and after even once the harness has deleted the logs it
// was measured from (ADR 0033). The file holds numbers only: no names,
// titles or paths.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/ChrisAdkin8/trimloop/internal/safefile"
	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

// Version identifies the file layout.
const Version = 1

// File is the history file's content.
type File struct {
	Version int                       `json:"version"`
	Weeks   map[string]detectors.Week `json:"weeks"` // by Week
}

// Load reads the history file at path. A missing file is an empty history.
func Load(path string) (File, error) {
	f := File{Version: Version, Weeks: map[string]detectors.Week{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("%s: %w", path, err)
	}
	if f.Version != Version {
		return f, fmt.Errorf("%s: history version %d, want %d", path, f.Version, Version)
	}
	if f.Weeks == nil {
		f.Weeks = map[string]detectors.Week{}
	}
	return f, nil
}

// Merge adds this run's weeks to the history and returns every week, oldest
// first. The harness deletes old logs, oldest first, so only weeks up to
// partialUpTo (the week of the oldest log still on disk) can be partly
// deleted. For those, the stored copy stays when it has more turns and
// counted them the same way (detectors.TurnsCountedSince), even if another
// price list worked it out; every later week in the logs is complete there
// and replaces the stored copy. partialUpTo "" means no week is treated as
// partial. Weeks whose figures come from the history file are marked Kept.
func Merge(f *File, current []detectors.Week, prices, partialUpTo string) []detectors.Week {
	outcomes := map[string]weekOutcome{}
	for _, w := range current {
		w.Prices = prices
		w.Analysis = detectors.AnalysisVersion
		w.Kept, w.KeptWhy = false, ""
		old, stored := f.Weeks[w.Week]
		o := decide(old, stored, w, partialUpTo)
		outcomes[w.Week] = o
		if o == fromLogs {
			f.Weeks[w.Week] = w
		}
	}
	out := make([]detectors.Week, 0, len(f.Weeks))
	for k, w := range f.Weeks {
		o, inLogs := outcomes[k]
		if !inLogs {
			o = logsGone
		}
		w.Kept, w.KeptWhy = o != fromLogs, o.why()
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Week < out[j].Week })
	return out
}

// weekOutcome is where a week's figures come from after a merge
// (docs/state-machines.md §6).
type weekOutcome int

const (
	fromLogs   weekOutcome = iota // this run's figures, from the logs
	keptFuller                    // the logs were partly deleted; the stored copy counted more turns the same way
	logsGone                      // the logs no longer hold the week; the stored copy
)

func (o weekOutcome) why() string {
	return [...]string{"", "logs partly deleted, stored copy fuller", "logs deleted"}[o]
}

// decide picks between this run's week w and the stored copy old (stored
// reports whether there is one). Only weeks up to partialUpTo can be partly
// deleted, and a stored copy wins only if it counted turns as this version
// does (detectors.TurnsCountedSince) and counted more of them.
func decide(old detectors.Week, stored bool, w detectors.Week, partialUpTo string) weekOutcome {
	if stored && w.Week <= partialUpTo && old.Turns > w.Turns && old.Analysis >= detectors.TurnsCountedSince {
		return keptFuller
	}
	return fromLogs
}

// Save writes the history to path, readable by the owner only, creating its
// folder. The file is replaced whole or not at all (safefile), so a crash
// cannot leave half a file.
func Save(path string, f File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return safefile.WriteBytes(path, 0o600, b)
}
