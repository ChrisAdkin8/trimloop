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
	fromLogs := map[string]bool{}
	for _, w := range current {
		w.Prices = prices
		w.Analysis = detectors.AnalysisVersion
		w.Kept = false
		old, ok := f.Weeks[w.Week]
		if ok && w.Week <= partialUpTo && old.Turns > w.Turns && old.Analysis >= detectors.TurnsCountedSince {
			continue // partly deleted: the stored copy is fuller
		}
		f.Weeks[w.Week] = w
		fromLogs[w.Week] = true
	}
	out := make([]detectors.Week, 0, len(f.Weeks))
	for k, w := range f.Weeks {
		w.Kept = !fromLogs[k]
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Week < out[j].Week })
	return out
}

// Save writes the history to path, creating its folder. It writes a new file
// and renames it into place, so a crash cannot leave half a file.
func Save(path string, f File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".history-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
