package claudecode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"iter"
	"path/filepath"
	"strings"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
	"github.com/ChrisAdkin8/trimloop/internal/adapters/source/jsonl"
	"github.com/ChrisAdkin8/trimloop/pkg/cmdshape"
	"github.com/ChrisAdkin8/trimloop/pkg/event"
	"github.com/ChrisAdkin8/trimloop/pkg/normalize/anthropic"
)

// record is the part of a transcript line that Read uses.
type record struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"` // system lines: "compact_boundary", ...
	SessionID string `json:"sessionId"`
	UUID      string `json:"uuid"` // the line's own id; copied lines keep it
	Cwd       string `json:"cwd"`

	// Assistant lines: what the harness says caused the turn.
	AttributionSkill     string `json:"attributionSkill"`
	AttributionPlugin    string `json:"attributionPlugin"`
	AttributionMcpServer string `json:"attributionMcpServer"`
	AttributionAgent     string `json:"attributionAgent"`

	// ai-title and custom-title lines. An AI title is written from the
	// conversation, so it is derived from content: shown locally only.
	AITitle     string `json:"aiTitle"`
	CustomTitle string `json:"customTitle"`

	// attachment lines: text the harness adds to the conversation, and
	// snapshots of the system prompt and tool list.
	Attachment *attachment     `json:"attachment"`
	Rendered   json.RawMessage `json:"rendered"` // null when nothing was added to the conversation

	// cost-state lines: the harness's running cost record for one run.
	StartTime    json.RawMessage       `json:"startTime"`
	TotalCostUSD *float64              `json:"totalCostUSD"`
	ModelUsage   map[string]modelUsage `json:"modelUsage"`

	// compact_boundary lines.
	CompactMetadata *struct {
		PreTokens  int    `json:"preTokens"`
		PostTokens int    `json:"postTokens"`
		Trigger    string `json:"trigger"` // "auto" or "manual"
		DurationMs int64  `json:"durationMs"`
	} `json:"compactMetadata"`
	Timestamp json.RawMessage `json:"timestamp"` // RFC 3339 string; parsed apart, see parseLine
	Message   struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"` // a string, or a list of blocks
		Usage   *struct {
			anthropic.Usage
			Speed        string `json:"speed"`         // "fast" in fast mode
			InferenceGeo string `json:"inference_geo"` // "us" for US-only inference
		} `json:"usage"` // nil when the line has no usage
	} `json:"message"`
}

// block is one content block: a tool call on an assistant line, a tool
// result on a user line. Other block types are ignored.
type block struct {
	Type string `json:"type"`

	// tool_use
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`

	// tool_result
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"` // a string, or a list of text and image blocks
}

// input holds the tool-call arguments Read looks at. It is decoded leniently:
// a tool whose arguments have other types leaves these fields empty.
type input struct {
	Command      string `json:"command"`       // Bash
	FilePath     string `json:"file_path"`     // Read, Edit, Write
	NotebookPath string `json:"notebook_path"` // NotebookEdit
}

// attachment is the part of an attachment line Read uses. A prompt snapshot
// keeps its fields either inside "snapshot" or at the top level.
type attachment struct {
	Type     string    `json:"type"`
	Files    []memFile `json:"files"` // instructions: the memory files
	Snapshot *snapshot `json:"snapshot"`
	snapshot           // prompt_snapshot fields at the top level
}

type memFile struct {
	Path    string `json:"path"`
	Type    string `json:"type"` // e.g. "Project", "User"
	Content string `json:"content"`
}

type snapshot struct {
	SystemPrompt []string `json:"systemPrompt"`
	Tools        []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Schema      json.RawMessage `json:"schema"`
	} `json:"tools"`
}

type modelUsage struct {
	InputTokens              int     `json:"inputTokens"`
	OutputTokens             int     `json:"outputTokens"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens"`
	CostUSD                  float64 `json:"costUSD"`
	WebSearchRequests        int     `json:"webSearchRequests"`
}

// persistedTag starts a result whose full output Claude Code saved to disk
// because it was too large; the model sees only a preview.
const persistedTag = "<persisted-output>"

// Read streams the messages in one log. It follows the contract in
// source.Reader. Claude Code writes one line per content block, each
// repeating the same usage, so repeats of a message id are expected.
func (Reader) Read(log *source.Log) iter.Seq2[source.Message, error] {
	return jsonl.Lines(log, parseLine)
}

// parseLine turns one transcript line into a Message: an assistant message
// with usage, a user line carrying tool results, or a compaction boundary.
// Other lines are skipped.
func parseLine(line []byte) (source.Message, bool, error) {
	var rec record
	if err := json.Unmarshal(line, &rec); err != nil {
		return source.Message{}, false, err
	}
	// A timestamp that is missing or in an unexpected format leaves Time
	// zero rather than failing the line: the rest is still worth counting.
	var ts time.Time
	var s string
	if json.Unmarshal(rec.Timestamp, &s) == nil {
		ts, _ = time.Parse(time.RFC3339Nano, s)
	}

	switch {
	case rec.Type == "assistant" && rec.Message.Usage != nil:
		u := rec.Message.Usage
		return source.Message{
			Kind:      source.KindTurn,
			ID:        rec.Message.ID,
			SessionID: rec.SessionID,
			Time:      ts,
			Project:   projectOf(rec.Cwd),
			Model:     rec.Message.Model,
			Usage:     anthropic.NormalizeUsage(u.Usage),
			Fast:      u.Speed == "fast",
			USOnly:    u.InferenceGeo == "us",
			ToolCalls: toolCalls(blocks(rec.Message.Content), rec.Cwd),
			Attribution: event.Attribution{
				Skill:     rec.AttributionSkill,
				Plugin:    rec.AttributionPlugin,
				MCPServer: rec.AttributionMcpServer,
				Agent:     rec.AttributionAgent,
			},
		}, true, nil

	case rec.Type == "user":
		var results []source.Result
		for _, b := range blocks(rec.Message.Content) {
			if b.Type == "tool_result" {
				results = append(results, toolResult(b))
			}
		}
		if len(results) == 0 {
			return source.Message{}, false, nil
		}
		return source.Message{Kind: source.KindToolResults, SessionID: rec.SessionID, Time: ts, Results: results}, true, nil

	case rec.Type == "ai-title" && rec.AITitle != "":
		return source.Message{Kind: source.KindTitle, SessionID: rec.SessionID, Title: rec.AITitle}, true, nil
	case rec.Type == "custom-title" && rec.CustomTitle != "":
		return source.Message{Kind: source.KindTitle, SessionID: rec.SessionID, Title: rec.CustomTitle, CustomTitle: true}, true, nil

	case rec.Type == "attachment" && rec.Attachment != nil:
		return parseAttachment(rec, ts)

	case rec.Type == "cost-state" && rec.TotalCostUSD != nil:
		m := source.Message{Kind: source.KindCostRecord, SessionID: rec.SessionID, Time: ts,
			RunID: string(rec.StartTime), Record: event.HarnessRecord{Cost: *rec.TotalCostUSD, ByModel: map[string]event.ModelRecord{}, At: ts}}
		for model, u := range rec.ModelUsage {
			m.Record.ByModel[model] = event.ModelRecord{
				Cost: u.CostUSD, InputUncached: u.InputTokens, CacheRead: u.CacheReadInputTokens,
				CacheWrite: u.CacheCreationInputTokens, Output: u.OutputTokens, WebSearches: u.WebSearchRequests,
			}
		}
		return m, true, nil

	case rec.Type == "system" && rec.Subtype == "compact_boundary":
		m := source.Message{Kind: source.KindCompaction, SessionID: rec.SessionID, Time: ts, LineID: rec.UUID}
		if c := rec.CompactMetadata; c != nil {
			m.Compaction = event.Compaction{
				Before:   c.PreTokens,
				After:    c.PostTokens,
				Manual:   c.Trigger == "manual",
				Duration: time.Duration(c.DurationMs) * time.Millisecond,
			}
		}
		return m, true, nil
	}
	return source.Message{}, false, nil
}

// blocks decodes a content list. Content that is a plain string, or a list in
// an unexpected shape, has no blocks.
func blocks(content json.RawMessage) []block {
	if len(content) == 0 || content[0] != '[' {
		return nil
	}
	var bs []block
	if json.Unmarshal(content, &bs) != nil {
		return nil
	}
	return bs
}

// toolCalls describes the tool_use blocks in bs. cwd, the session's working
// directory, makes relative file paths comparable with absolute ones.
func toolCalls(bs []block, cwd string) []event.ToolCall {
	var calls []event.ToolCall
	for _, b := range bs {
		if b.Type != "tool_use" {
			continue
		}
		var in input
		_ = json.Unmarshal(b.Input, &in) // lenient: other argument types leave in empty
		c := event.ToolCall{ID: b.ID, Tool: b.Name}
		switch {
		case b.Name == "Bash":
			s := cmdshape.Parse(in.Command)
			c.Shape, c.Noise, c.Filtered = s.String(), s.Noise, s.Filtered
			if s.File != "" {
				c.File, c.FileOp = fileKey(s.File, cwd), event.FileView
				if s.Edit {
					c.FileOp = event.FileEdit
				}
			}
		case b.Name == "Read" || b.Name == "NotebookRead":
			c.Noise = event.NoiseFileView
			c.File, c.FileOp = fileKey(firstOf(in.FilePath, in.NotebookPath), cwd), event.FileView
		case b.Name == "Edit" || b.Name == "MultiEdit" || b.Name == "Write" || b.Name == "NotebookEdit":
			c.File, c.FileOp = fileKey(firstOf(in.FilePath, in.NotebookPath), cwd), event.FileEdit
		case b.Name == "Grep" || b.Name == "Glob" || b.Name == "LS":
			c.Noise = event.NoiseSearch
		case b.Name == "WebFetch" || b.Name == "WebSearch":
			c.Noise = event.NoiseWeb
		case b.Name == "Task" || b.Name == "Agent":
			c.Noise = event.NoiseSubAgent
		case strings.HasPrefix(b.Name, "mcp__"):
			c.Noise = event.NoiseMCP
		}
		if c.File == "" {
			c.FileOp = event.FileNone
		}
		calls = append(calls, c)
	}
	return calls
}

// toolResult measures one tool_result block: the size of the text the model
// received, and whether it was an error or a preview of saved output. Images
// are not counted.
func toolResult(b block) source.Result {
	r := source.Result{CallID: b.ToolUseID}
	r.Error = b.IsError
	var text string
	if json.Unmarshal(b.Content, &text) == nil {
		r.Bytes = len(text)
		r.Persisted = strings.HasPrefix(text, persistedTag)
		return r
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(b.Content, &parts) == nil {
		for i, p := range parts {
			if p.Type == "text" {
				r.Bytes += len(p.Text)
				if i == 0 {
					r.Persisted = strings.HasPrefix(p.Text, persistedTag)
				}
			}
		}
	}
	return r
}

// fileKey identifies a file by a hash of its cleaned, absolute path, so
// re-reads can be spotted without keeping the path. It is unkeyed and used
// in memory only; stored identifiers use keyed fingerprints (ADR 0011).
func fileKey(p, cwd string) string {
	if p == "" || strings.ContainsAny(p, "$`*?") {
		return "" // a variable or a glob: not one known file
	}
	if !filepath.IsAbs(p) && cwd != "" {
		p = filepath.Join(cwd, p)
	}
	sum := sha256.Sum256([]byte(filepath.Clean(p)))
	return hex.EncodeToString(sum[:8])
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// worktrees is where Claude Code puts temporary copies of a repo, for example
// for a sub-agent: <project>/.claude/worktrees/<name>.
var worktrees = string(filepath.Separator) + filepath.Join(".claude", "worktrees") + string(filepath.Separator)

// projectOf names the project a working folder belongs to: the folder's own
// name, or for a worktree, the name of the project it was copied from.
func projectOf(cwd string) string {
	if cwd == "" {
		return ""
	}
	if i := strings.Index(cwd, worktrees); i > 0 {
		cwd = cwd[:i]
	}
	return filepath.Base(cwd)
}

// parseAttachment reads an attachment line: a snapshot of the system prompt
// and tool list, or text the harness added to the conversation. Only sizes
// and memory-file names are kept, never the text.
func parseAttachment(rec record, ts time.Time) (source.Message, bool, error) {
	a := rec.Attachment
	if a.Type == "prompt_snapshot" {
		snap := a.snapshot
		if a.Snapshot != nil {
			snap = *a.Snapshot
		}
		st := &event.Standing{}
		for _, p := range snap.SystemPrompt {
			st.SystemBytes += len(p)
		}
		for _, t := range snap.Tools {
			st.Tools = append(st.Tools, event.ToolDef{Name: t.Name, Bytes: len(t.Name) + len(t.Description) + len(t.Schema)})
		}
		if st.Bytes() == 0 {
			return source.Message{}, false, nil
		}
		return source.Message{Kind: source.KindStanding, SessionID: rec.SessionID, Time: ts, Standing: st}, true, nil
	}

	size := renderedSize(rec.Rendered)
	if size == 0 {
		return source.Message{}, false, nil // not added to the conversation
	}
	var pieces []event.Injected
	if a.Type == "instructions" && len(a.Files) > 0 {
		for _, f := range a.Files {
			pieces = append(pieces, event.Injected{Kind: "memory_file", Name: memoryName(f), Bytes: len(f.Content)})
		}
	} else {
		pieces = []event.Injected{{Kind: a.Type, Bytes: size}}
	}
	return source.Message{Kind: source.KindInjected, SessionID: rec.SessionID, Time: ts, Injected: pieces, LineID: rec.UUID}, true, nil
}

// renderedSize is the size of the text an attachment added: the content of
// each rendered block, or 0 if nothing was rendered.
func renderedSize(raw json.RawMessage) int {
	var blocks []struct {
		Content json.RawMessage `json:"content"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &blocks) != nil {
		return 0
	}
	n := 0
	for _, b := range blocks {
		var s string
		if json.Unmarshal(b.Content, &s) == nil {
			n += len(s)
		} else {
			n += len(b.Content)
		}
	}
	return n
}

// memoryName names a memory file by its kind and file name, e.g.
// "Project CLAUDE.md", without the folders above it.
func memoryName(f memFile) string {
	name := filepath.Base(f.Path)
	if f.Type != "" {
		return f.Type + " " + name
	}
	return name
}
