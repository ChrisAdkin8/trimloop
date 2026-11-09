package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ChrisAdkin8/trimloop/internal/live"
)

// TestMain points HOME at a scratch folder for every test in the package,
// so nothing a test runs (setup's backups in ~/.trimloop/backups, say) can
// write into the real home folder.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "trimloop-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, v := range []string{"HOME", "USERPROFILE"} { // USERPROFILE on Windows
		if err := os.Setenv(v, home); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

// TestRunNudgeSwallowsPanics checks that a panic in the status line or a
// hook prints nothing and does not escape: Go would exit with code 2, which
// blocks the user's prompt for a UserPromptSubmit hook.
func TestRunNudgeSwallowsPanics(t *testing.T) {
	s, h := statusLine, hook
	t.Cleanup(func() { statusLine, hook = s, h })
	statusLine = func(_ io.Reader, out io.Writer, _ live.Env) {
		_, _ = io.WriteString(out, "half a line")
		panic("status line")
	}
	hook = func(_ string, _ io.Reader, out io.Writer, _ live.Env) {
		_, _ = io.WriteString(out, `{"decision":`)
		panic("hook")
	}
	for _, c := range []struct {
		cmd  string
		args []string
	}{{"statusline", nil}, {"hook", []string{"prompt"}}, {"hook", []string{"stop"}}} {
		var out strings.Builder
		runNudge(c.cmd, c.args, strings.NewReader("{}"), &out, t.TempDir())
		if out.Len() != 0 {
			t.Errorf("%s %v: got %q, want nothing", c.cmd, c.args, out.String())
		}
	}
}

// TestRunNudgeBadInput checks that input that is not JSON prints nothing
// and that a normal run's output still reaches standard output.
func TestRunNudgeBadInput(t *testing.T) {
	for _, c := range []struct {
		cmd  string
		args []string
	}{{"statusline", nil}, {"hook", []string{"prompt"}}, {"hook", []string{"stop"}}, {"hook", nil}, {"other", nil}} {
		var out strings.Builder
		runNudge(c.cmd, c.args, strings.NewReader("not json {"), &out, t.TempDir())
		if out.Len() != 0 {
			t.Errorf("%s %v: got %q, want nothing", c.cmd, c.args, out.String())
		}
	}
	var out strings.Builder
	runNudge("statusline", nil, strings.NewReader(`{"cost":{"total_cost_usd":1.5}}`), &out, t.TempDir())
	if strings.TrimSpace(out.String()) != "$1.50" {
		t.Errorf("status line: got %q, want the cost", out.String())
	}
}
