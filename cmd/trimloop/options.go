package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// buildVersion is set at release time with
// -ldflags "-X main.buildVersion=v0.1.0"; empty in a local build.
var buildVersion string

// version names this build: the release version, else the module version
// that 'go install' recorded, else the source revision a local build was
// made from ("dev-1a2b3c4d", with "-modified" if it had uncommitted changes).
func version() string {
	if buildVersion != "" {
		return buildVersion
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	// A local build gets a version Go makes up from the commit
	// (v0.1.1-0.20261109101144-c0daee340df5): not a release, so show it as
	// a dev build of that commit instead.
	if v := info.Main.Version; v != "" && v != "(devel)" && !pseudoVersion.MatchString(v) {
		return v
	}
	v, modified := "dev", false
	if m := pseudoVersion.FindStringSubmatch(info.Main.Version); m != nil && !hasRevision(info) {
		// 'go install module@commit' records no revision, only the
		// version Go made up, which ends with the commit.
		v += "-" + m[1][:8]
	}
	for _, s := range info.Settings {
		switch {
		case s.Key == "vcs.revision" && len(s.Value) >= 8:
			v += "-" + s.Value[:8]
		case s.Key == "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if modified {
		v += "-modified"
	}
	return v
}

// pseudoVersion matches the versions Go makes up for unreleased commits
// (v0.0.0-20261109101144-c0daee340df5, v0.1.1-0.20261109101144-c0daee340df5,
// v0.2.0-rc.1.0.20261109101144-c0daee340df5): a 14-digit time and a
// 12-letter commit hash, with "+dirty" if the working tree had changes.
// The first group is the hash.
var pseudoVersion = regexp.MustCompile(`^v\d+\.(?:0\.0-|\d+\.\d+-(?:[^+]*\.)?0\.)\d{14}-([0-9a-f]{12})(?:\+dirty)?$`)

// hasRevision says whether the build recorded the commit it was made from.
func hasRevision(info *debug.BuildInfo) bool {
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return true
		}
	}
	return false
}

// shellArg quotes s for a POSIX shell when it holds anything but letters,
// digits and / . _ ~ - (a space, say), so a command shown with it can be
// copied as it is.
func shellArg(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._~-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// parseSince reads --since: a date (2026-11-01, local time) or a span back
// from now in days or weeks (7d, 2w), which starts at the beginning of that
// day.
func parseSince(s string, now time.Time) (time.Time, error) {
	if t, err := time.ParseInLocation("2006-01-02", s, now.Location()); err == nil {
		return t, nil
	}
	if n := len(s); n >= 2 {
		count, err := strconv.Atoi(s[:n-1])
		if err == nil && count >= 0 {
			days := map[byte]int{'d': 1, 'w': 7}[s[n-1]]
			if days > 0 {
				day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
				return day.AddDate(0, 0, -count*days), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("--since %q: give a date such as 2026-11-01, or a number of days or weeks such as 7d or 2w", s)
}

// startedSince keeps the main sessions that started on or after since, with
// their sub-agents, so a report can show only work done after a change. A
// sub-agent whose main session was not read is kept by its own start.
func startedSince(sessions []event.Session, since time.Time) []event.Session {
	kept := map[string]bool{}
	mains := map[string]bool{}
	for _, s := range sessions {
		if s.Kind != event.SubAgentSession {
			mains[s.ID] = true
			kept[s.ID] = !began(s).Before(since)
		}
	}
	var out []event.Session
	for _, s := range sessions {
		keep := kept[s.ID]
		if s.Kind == event.SubAgentSession {
			if mains[s.ParentID] {
				keep = kept[s.ParentID]
			} else {
				keep = !began(s).Before(since)
			}
		}
		if keep {
			out = append(out, s)
		}
	}
	return out
}

// began is when a session started: its first turn, or, for one with no
// turns (refused at its first message), its first limit hit or failure.
func began(s event.Session) time.Time {
	t := s.Start()
	if !t.IsZero() {
		return t
	}
	for _, h := range s.LimitHits {
		if !h.At.IsZero() && (t.IsZero() || h.At.Before(t)) {
			t = h.At
		}
	}
	for _, f := range s.Failures {
		if !f.At.IsZero() && (t.IsZero() || f.At.Before(t)) {
			t = f.At
		}
	}
	return t
}

// openInBrowser opens a file with the system's default program for it.
func openInBrowser(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not open %s: %w", path, err)
	}
	return cmd.Process.Release()
}

// noLogsMessage explains an empty run.
func noLogsMessage(dir string) string {
	return strings.Join([]string{
		"No Claude Code session logs found in " + dir + ".",
		"Claude Code writes them there as you use it. If yours are elsewhere, set",
		"CLAUDE_CONFIG_DIR to the folder that holds 'projects'.",
	}, "\n")
}
