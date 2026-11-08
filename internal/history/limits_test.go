package history

import (
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
	w := func(used float64) *detectors.LimitWindow {
		return &detectors.LimitWindow{Used: used, ResetsAt: 1_800_000_000}
	}
	next := &detectors.LimitWindow{Used: 1, ResetsAt: 1_800_018_000} // the next 5-hour window
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
