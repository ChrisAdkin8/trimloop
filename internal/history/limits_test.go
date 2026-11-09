package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

func TestAppendAndLoadLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "limits.jsonl")
	t0 := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	reset := t0.Add(3 * time.Hour).Unix() // within the plausible bound of each reading
	w := func(used float64) *detectors.LimitWindow {
		return &detectors.LimitWindow{Used: used, ResetsAt: reset}
	}
	next := &detectors.LimitWindow{Used: 1, ResetsAt: reset + 18_000} // the next 5-hour window
	for i, r := range []detectors.LimitReading{
		{At: t0, FiveHour: w(10), SevenDay: w(30)},
		{At: t0.Add(time.Minute), FiveHour: w(10), SevenDay: w(30)},     // the same: not kept
		{At: t0.Add(2 * time.Minute), FiveHour: w(11), SevenDay: w(30)}, // 5-hour use rose: kept
		{At: t0.Add(3 * time.Minute)},                                   // no limits: not kept
		{At: t0.Add(4 * time.Minute), FiveHour: w(9), SevenDay: w(29)},  // an idle session's older reading: not kept
		{At: t0.Add(5 * time.Minute), FiveHour: w(11)},                  // weekly limit missing, nothing new: not kept
		{At: t0.Add(6 * time.Minute), FiveHour: next, SevenDay: w(30)},  // a new 5-hour window: kept
	} {
		if err := AppendLimits(path, r); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	got, err := LoadLimits(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1].FiveHour.Used != 11 || got[2].FiveHour.Used != 1 {
		t.Errorf("readings: got %d %+v, want 3", len(got), got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file mode: %v, %v; want 0600", info.Mode().Perm(), err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "session") || strings.Count(string(b), "\n") != 3 {
		t.Errorf("file: %s", b)
	}
	if r, err := LoadLimits(filepath.Join(t.TempDir(), "missing.jsonl")); err != nil || r != nil {
		t.Errorf("missing file: got %v, %v", r, err)
	}
}

func TestPathFor(t *testing.T) {
	home := "/home/u"
	if got := LimitsPath(home, ""); got != "/home/u/.trimloop/limits.jsonl" {
		t.Errorf("default: got %s", got)
	}
	if got := LimitsPath(home, "/home/u/.claude"); got != "/home/u/.trimloop/limits.jsonl" {
		t.Errorf("default folder named: got %s", got)
	}
	other := LimitsPath(home, "/work/claude")
	if !strings.HasPrefix(other, "/home/u/.trimloop/limits-") || !strings.HasSuffix(other, ".jsonl") || other == LimitsPath(home, "/work/other") {
		t.Errorf("other folder: got %s", other)
	}
}

func TestLoadLimitsSurvivesALongDamagedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "limits.jsonl")
	good := `{"time":"2026-11-08T09:00:00Z","seven_day":{"used":5,"resets_at":1800000000}}`
	content := good + "\n" + strings.Repeat("x", 200_000) + "\n" + strings.Replace(good, ":5,", ":6,", 1) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLimits(path)
	if err != nil || len(got) != 2 {
		t.Errorf("readings around a 200 KB damaged line: got %d, %v; want 2", len(got), err)
	}
}

// TestAppendLimitsPastAFarFutureLine checks that a last line stored with an
// implausible reset time (version 0.1.0 did not check) does not make every
// later genuine reading look older, and that the line is left in place.
func TestAppendLimitsPastAFarFutureLine(t *testing.T) {
	t0 := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	reset := t0.Add(3 * time.Hour).Unix()
	for _, bad := range []int64{
		1_790_000_000_000, // milliseconds
		time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), // year 9999
		t0.Add(9 * 24 * time.Hour).Unix(),                  // just past the 8-day bound
	} {
		path := filepath.Join(t.TempDir(), "limits.jsonl")
		stored := detectors.LimitReading{
			At:       t0.Add(-time.Hour),
			FiveHour: &detectors.LimitWindow{Used: 50, ResetsAt: bad},
			SevenDay: &detectors.LimitWindow{Used: 50, ResetsAt: bad},
		}
		b, _ := json.Marshal(stored)
		if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		for i, used := range []float64{10, 11, 12} {
			r := detectors.LimitReading{
				At:       t0.Add(time.Duration(i) * time.Minute),
				FiveHour: &detectors.LimitWindow{Used: used, ResetsAt: reset},
			}
			if err := AppendLimits(path, r); err != nil {
				t.Fatal(err)
			}
		}
		got, err := LoadLimits(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 4 || got[0].FiveHour.ResetsAt != bad || got[3].FiveHour.Used != 12 {
			t.Errorf("reset %d: got %d readings %+v; want the stored line and all three", bad, len(got), got)
		}
	}
}

// TestAppendLimitsKeepsPlausibleLastLine checks that a stored reset time
// within the bound still holds back a reading that is not ahead of it.
func TestAppendLimitsKeepsPlausibleLastLine(t *testing.T) {
	t0 := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "limits.jsonl")
	weekly := t0.Add(7 * 24 * time.Hour).Unix() // late in the bound, still believed
	for _, r := range []detectors.LimitReading{
		{At: t0, SevenDay: &detectors.LimitWindow{Used: 30, ResetsAt: weekly}},
		{At: t0.Add(time.Minute), SevenDay: &detectors.LimitWindow{Used: 30, ResetsAt: weekly}},            // the same: not kept
		{At: t0.Add(2 * time.Minute), SevenDay: &detectors.LimitWindow{Used: 29, ResetsAt: weekly - 3600}}, // earlier window: not kept
	} {
		if err := AppendLimits(path, r); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := LoadLimits(path); err != nil || len(got) != 1 {
		t.Errorf("got %d readings, %v; want 1", len(got), err)
	}
}
