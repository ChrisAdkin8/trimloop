package history

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

func TestLoadMissingIsEmpty(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || f.Version != Version || len(f.Weeks) != 0 {
		t.Errorf("got %+v, %v", f, err)
	}
}

func TestMergeAndSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "history.json")
	f, _ := Load(path)
	weeks := Merge(&f, []detectors.Week{{Week: "2026-10-05", Turns: 100, Spend: 10}, {Week: "2026-10-12", Turns: 50}}, "p1", "")
	if len(weeks) != 2 || weeks[0].Kept || weeks[0].Prices != "p1" {
		t.Fatalf("first run: got %+v", weeks)
	}
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}

	// A later run: the first week's logs are partly deleted, the second
	// grew, and a third is new.
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	weeks = Merge(&f, []detectors.Week{{Week: "2026-10-05", Turns: 40, Spend: 4}, {Week: "2026-10-12", Turns: 70}, {Week: "2026-10-19", Turns: 5}}, "p1", "2026-10-05")
	if len(weeks) != 3 {
		t.Fatalf("got %d weeks, want 3", len(weeks))
	}
	if w := weeks[0]; w.Turns != 100 || w.Spend != 10 || !w.Kept {
		t.Errorf("partly deleted week: got %+v, want the fuller stored copy, marked kept", w)
	}
	if weeks[1].Turns != 70 || weeks[1].Prices != "p1" {
		t.Errorf("grown week: got %+v", weeks[1])
	}

	// A run after the first week's logs are gone entirely.
	weeks = Merge(&f, []detectors.Week{{Week: "2026-10-19", Turns: 9}}, "p1", "2026-10-19")
	if !weeks[0].Kept || weeks[2].Kept {
		t.Errorf("got %+v, want the old weeks marked kept", weeks)
	}
}

func TestLoadRejectsOtherVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"weeks":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("got no error for an unknown version")
	}
	if err := os.WriteFile(path, []byte(`not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("got no error for a damaged file")
	}
}

// A partly deleted week keeps its fuller stored copy even when an older
// price list or analysis worked it out; a later, complete week is replaced
// even when the stored copy counted more turns (an older analysis counted
// some lines twice).
func TestMergeOlderFigures(t *testing.T) {
	f := File{Version: Version, Weeks: map[string]detectors.Week{
		"2026-10-05": {Week: "2026-10-05", Turns: 120, Spend: 50, Prices: "old", Analysis: detectors.AnalysisVersion},
		"2026-10-12": {Week: "2026-10-12", Turns: 300, Spend: 70}, // no analysis version: older
	}}
	weeks := Merge(&f, []detectors.Week{{Week: "2026-10-05", Turns: 100, Spend: 45}, {Week: "2026-10-12", Turns: 250, Spend: 60}}, "new", "2026-10-05")
	if w := weeks[0]; w.Turns != 120 || w.Prices != "old" || !w.Kept {
		t.Errorf("partly deleted week: got %+v, want the stored copy, marked kept", w)
	}
	if w := weeks[1]; w.Turns != 250 || w.Prices != "new" || w.Kept {
		t.Errorf("complete week: got %+v, want this run's figures", w)
	}
}
