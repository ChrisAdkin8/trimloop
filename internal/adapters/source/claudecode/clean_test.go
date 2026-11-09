package claudecode

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
)

// dirty holds what a crafted log can put in a name: terminal escapes (OSC
// with BEL, CSI, the 8-bit CSI), a bidi override and a zero-width space.
const dirty = "\x1b]0;PWNED\x07\x1b[31mRED\x1b[0m\u009b2J\u202eTXT\u200b"

// ordinary is text in other scripts, with emoji, that must come through.
const ordinary = "café 日本 🎉"

func unsafeIn(s string) bool { return strings.ContainsFunc(s, unsafeRune) }

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"":                         "",
		"claude-opus-5-5":          "claude-opus-5-5",
		ordinary:                   ordinary,
		"a" + dirty + "b":          "a]0;PWNED[31mRED[0m2JTXTb",
		"bad\xff\xfeutf8":          "badutf8",
		"zero\u200bwidth\ufeff":    "zerowidth",
		"tab\tnew\nline\rdel\x7f.": "tabnewlinedel.",
	} {
		if got := clean(in); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
	// A folder keeps a mark where something was taken out, so it cannot
	// become the same folder as one that never had it.
	if a, b := cleanFolder("/r/a\u200bb"), cleanFolder("/r/ab"); a == b || unsafeIn(a) {
		t.Errorf("cleanFolder: %q and %q", a, b)
	}
	if got := cleanFolder("/r/" + ordinary); got != "/r/"+ordinary {
		t.Errorf("cleanFolder(ordinary) = %q", got)
	}
	if got := cleanFolder("/r/x\xff"); got != "/r/x�" {
		t.Errorf("cleanFolder(invalid UTF-8) = %q", got)
	}
}

// Every human-readable string Read passes on is cleaned at ingestion, and
// ordinary non-ASCII text survives.
func TestParseLineCleansText(t *testing.T) {
	tag := func(s string) string { return s + " " + ordinary + dirty }
	line := func(v map[string]any) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	parse := func(v map[string]any) source.Message {
		t.Helper()
		m, ok, err := parseLine(line(v))
		if err != nil || !ok {
			t.Fatalf("parseLine: %v, %v", ok, err)
		}
		return m
	}
	rendered := []map[string]any{{"content": "added text"}}

	var got []string
	for _, title := range []map[string]any{
		{"type": "ai-title", "aiTitle": tag("title")},
		{"type": "custom-title", "customTitle": tag("title")},
	} {
		got = append(got, parse(title).Title)
	}

	turn := parse(map[string]any{
		"type": "assistant", "cwd": "/r/" + tag("proj"), "effort": tag("max"),
		"attributionSkill": tag("skill"), "attributionPlugin": tag("plugin"),
		"attributionMcpServer": tag("mcp"), "attributionAgent": tag("agent"),
		"message": map[string]any{"id": "m1", "model": tag("claude-model"),
			"usage": map[string]any{"output_tokens": 1},
			"content": []any{
				map[string]any{"type": "tool_use", "id": "t1", "name": tag("Tool"), "input": map[string]any{}},
				map[string]any{"type": "tool_use", "id": "t2", "name": "Bash", "input": map[string]any{"command": tag("cat") + " x"}},
			}},
	})
	got = append(got, turn.Model, turn.Effort, turn.Project, turn.Attribution.Skill, turn.Attribution.Plugin,
		turn.Attribution.MCPServer, turn.Attribution.Agent)
	got = append(got, turn.ToolCalls[0].Tool, turn.ToolCalls[1].Shape)

	mem := parse(map[string]any{"type": "attachment", "rendered": rendered, "attachment": map[string]any{
		"type": "instructions", "files": []any{map[string]any{"path": "/r/" + tag("CLAUDE.md"), "type": tag("Project"), "content": "y"}}}})
	mcp := parse(map[string]any{"type": "attachment", "rendered": rendered, "attachment": map[string]any{
		"type": "mcp_instructions_delta", "addedNames": []string{tag("server")}, "addedBlocks": []string{"z"}}})
	other := parse(map[string]any{"type": "attachment", "rendered": rendered, "attachment": map[string]any{"type": tag("kind")}})
	got = append(got, mem.Injected[0].Name, mcp.Injected[0].Name, other.Injected[0].Kind)

	snap := parse(map[string]any{"type": "attachment", "attachment": map[string]any{"type": "prompt_snapshot",
		"snapshot": map[string]any{"systemPrompt": []string{"p"}, "tools": []any{map[string]any{"name": "mcp__" + tag("srv") + "__t"}}}}})
	for _, d := range snap.Standing.Tools {
		got = append(got, d.Name)
	}

	cost := parse(map[string]any{"type": "cost-state", "totalCostUSD": 3.0, "modelUsage": map[string]any{
		tag("claude-model"): map[string]any{"costUSD": 1.0}, tag("claude-model") + "\x07": map[string]any{"costUSD": 2.0}}})
	if len(cost.Record.ByModel) != 1 {
		t.Errorf("model ids that clean alike should be added together: %v", cost.Record.ByModel)
	}
	for model, r := range cost.Record.ByModel {
		got = append(got, model)
		if r.Cost != 3 {
			t.Errorf("cost for %q: got %v, want 3", model, r.Cost)
		}
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-a1.meta.json"), string(line(map[string]any{"agentType": tag("type")})))
	got = append(got, agentType(filepath.Join(dir, "agent-a1.jsonl")))

	if len(got) != 17 {
		t.Errorf("checked %d strings, want 17", len(got))
	}
	for _, s := range got {
		if unsafeIn(s) || strings.ContainsRune(s, '\x1b') {
			t.Errorf("not cleaned: %q", s)
		}
		if !strings.Contains(s, ordinary) && !strings.Contains(s, "cat") {
			t.Errorf("ordinary text lost: %q", s)
		}
	}
}

// A title made only of characters that are removed is no title.
func TestParseLineEmptyAfterCleaning(t *testing.T) {
	if _, ok, err := parseLine([]byte(`{"type":"ai-title","aiTitle":"\u202e\u200b\u001b"}`)); ok || err != nil {
		t.Errorf("got %v, %v; want the line skipped", ok, err)
	}
}
