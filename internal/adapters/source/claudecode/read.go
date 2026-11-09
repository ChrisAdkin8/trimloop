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
	IsMeta    bool   `json:"isMeta"` // user lines the harness wrote, not the user

	// User lines: who sent it ("human", "peer", "task-notification", ...;
	// missing in older versions), whether it is a compaction's summary, and
	// for tool results, who allowed the call ("user_..." when the user did).
	Origin             *origin `json:"origin"`
	IsCompactSummary   bool    `json:"isCompactSummary"`
	PermissionDecision *struct {
		Source string `json:"source"`
	} `json:"permissionDecision"`

	// Assistant lines: what the harness says caused the turn.
	AttributionSkill     string `json:"attributionSkill"`
	AttributionPlugin    string `json:"attributionPlugin"`
	AttributionMcpServer string `json:"attributionMcpServer"`
	AttributionAgent     string `json:"attributionAgent"`
	Effort               string `json:"effort"` // the effort level the request asked for

	// Assistant lines the harness wrote itself to report a failed request:
	// error is "rate_limit" when a usage limit was reached.
	Error             json.RawMessage `json:"error"`
	IsAPIErrorMessage bool            `json:"isApiErrorMessage"`

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
	UnknownCost  bool                  `json:"hasUnknownModelCost"`
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
			// Iterations holds the usage of each step when the harness
			// split one reply into several; the top level can then be zero.
			Iterations []anthropic.Usage `json:"iterations"`
		} `json:"usage"` // nil when the line has no usage
	} `json:"message"`
}

// block is one content block: a tool call on an assistant line, a tool
// result on a user line, and text. Other block types are ignored.
type block struct {
	Type string `json:"type"`

	// tool_use
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`

	// text: read only to tell a typed message from a harness notice
	Text string `json:"text"`

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

	// Read: the part of the file read. Kept raw, as a number or a string.
	Offset json.RawMessage `json:"offset"`
	Limit  json.RawMessage `json:"limit"`
	Pages  string          `json:"pages"` // a PDF's pages
}

// attachment is the part of an attachment line Read uses. A prompt snapshot
// keeps its fields either inside "snapshot" or at the top level.
// origin says who sent a user line or queued message.
type origin struct {
	Kind string `json:"kind"`
}

type attachment struct {
	Type        string  `json:"type"`
	CommandMode string  `json:"commandMode"` // queued_command: "prompt" for a message
	Origin      *origin `json:"origin"`      // queued_command: who sent it

	// mcp_instructions_delta: each server's name and its instructions.
	AddedNames  []string  `json:"addedNames"`
	AddedBlocks []string  `json:"addedBlocks"`
	Files       []memFile `json:"files"` // instructions: the memory files
	Snapshot    *snapshot `json:"snapshot"`
	snapshot              // prompt_snapshot fields at the top level
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
	rec.clean()
	// A timestamp that is missing or in an unexpected format leaves Time
	// zero rather than failing the line: the rest is still worth counting.
	var ts time.Time
	var s string
	if json.Unmarshal(rec.Timestamp, &s) == nil {
		ts, _ = time.Parse(time.RFC3339Nano, s)
	}

	switch {
	case rec.Type == "assistant" && rec.IsAPIErrorMessage && limitError(rec.Error) && limitKind(blocks(rec.Message.Content)) != "":
		// A subscription usage limit, in Claude Code's own words. Other rate
		// limits (an API key's per-minute limit) are failures, below.
		return source.Message{Kind: source.KindLimitHit, SessionID: rec.SessionID, Time: ts, LineID: rec.UUID,
			Limit: limitKind(blocks(rec.Message.Content))}, true, nil

	case rec.Type == "assistant" && rec.IsAPIErrorMessage && (rec.Message.Model == synthetic || limitError(rec.Error)):
		return source.Message{Kind: source.KindFailure, SessionID: rec.SessionID, Time: ts, LineID: rec.UUID,
			Failure: failureKind(errorText(rec.Error), blocks(rec.Message.Content))}, true, nil

	case rec.Type == "assistant" && rec.Message.Usage != nil:
		u := rec.Message.Usage
		return source.Message{
			Kind:      source.KindTurn,
			ID:        rec.Message.ID,
			SessionID: rec.SessionID,
			Time:      ts,
			Project:   cleanFolder(projectOf(rec.Cwd)),
			Model:     rec.Message.Model,
			Synthetic: rec.Message.Model == synthetic,
			Usage:     replyUsage(u.Usage, u.Iterations),
			Fast:      u.Speed == "fast",
			USOnly:    u.InferenceGeo == "us",
			Effort:    rec.Effort,
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
			if typedPrompt(rec) {
				return source.Message{Kind: source.KindPrompt, SessionID: rec.SessionID, Time: ts, LineID: rec.UUID}, true, nil
			}
			return source.Message{}, false, nil
		}
		m := source.Message{Kind: source.KindToolResults, SessionID: rec.SessionID, Time: ts, Results: results, LineID: rec.UUID}
		if d := rec.PermissionDecision; d != nil && strings.HasPrefix(d.Source, "user") {
			m.UserDecided = true // the user allowed or refused the call
		}
		for _, r := range results {
			m.UserDecided = m.UserDecided || r.Refused
		}
		return m, true, nil

	case rec.Type == "ai-title" && rec.AITitle != "":
		return source.Message{Kind: source.KindTitle, SessionID: rec.SessionID, Title: rec.AITitle}, true, nil
	case rec.Type == "custom-title" && rec.CustomTitle != "":
		return source.Message{Kind: source.KindTitle, SessionID: rec.SessionID, Title: rec.CustomTitle, CustomTitle: true}, true, nil

	case rec.Type == "attachment" && rec.Attachment != nil:
		return parseAttachment(rec, ts)

	case rec.Type == "cost-state" && rec.TotalCostUSD != nil:
		m := source.Message{Kind: source.KindCostRecord, SessionID: rec.SessionID, Time: ts,
			RunID: string(rec.StartTime), Record: event.HarnessRecord{Cost: *rec.TotalCostUSD, ByModel: map[string]event.ModelRecord{}, At: ts, UnknownCost: rec.UnknownCost}}
		var startMs int64
		if json.Unmarshal(rec.StartTime, &startMs) == nil && startMs > 0 {
			m.Record.Start = time.UnixMilli(startMs).UTC()
		}
		for model, u := range rec.ModelUsage {
			// Two ids that clean to the same one are added together.
			model = clean(model)
			m.Record.ByModel[model] = m.Record.ByModel[model].Add(event.ModelRecord{
				Cost: u.CostUSD, InputUncached: u.InputTokens, CacheRead: u.CacheReadInputTokens,
				CacheWrite: u.CacheCreationInputTokens, Output: u.OutputTokens, WebSearches: u.WebSearchRequests,
			})
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

// clean removes control and format characters from the record's
// human-readable fields (see clean). The working folder is left as logged:
// file identities are hashed from it, and the project folder is cleaned
// apart (cleanFolder).
func (rec *record) clean() {
	rec.AITitle, rec.CustomTitle = clean(rec.AITitle), clean(rec.CustomTitle)
	rec.Effort, rec.Message.Model = clean(rec.Effort), clean(rec.Message.Model)
	rec.AttributionSkill, rec.AttributionPlugin = clean(rec.AttributionSkill), clean(rec.AttributionPlugin)
	rec.AttributionMcpServer, rec.AttributionAgent = clean(rec.AttributionMcpServer), clean(rec.AttributionAgent)
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
		b.Name = clean(b.Name)
		c := event.ToolCall{ID: b.ID, Tool: b.Name, InputBytes: len(b.Input)}
		switch {
		case b.Name == "Bash":
			s := cmdshape.Parse(in.Command)
			c.Shape, c.Noise, c.Filtered = clean(s.String()), s.Noise, s.Filtered
			if s.File != "" {
				c.File, c.FileOp = fileKey(s.File, cwd), event.FileView
				c.Part = partKey(s.Part)
				if s.Edit {
					c.FileOp = event.FileEdit
				}
			}
			c.WritesUnknown = s.WritesUnknown
			for _, w := range s.Writes {
				if k := fileKey(w, cwd); k != "" {
					c.Writes = append(c.Writes, k)
				} else {
					c.WritesUnknown = true // a variable or a glob: which file is not known
				}
			}
		case b.Name == "Read" || b.Name == "NotebookRead":
			c.Noise = event.NoiseFileView
			c.File, c.FileOp = fileKey(firstOf(in.FilePath, in.NotebookPath), cwd), event.FileView
			if o, l := rawArg(in.Offset), rawArg(in.Limit); o != "" || l != "" || in.Pages != "" {
				c.Part = partKey("read offset=" + o + " limit=" + l + " pages=" + in.Pages)
			}
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
// refusedTag starts the result Claude Code sends when the user refuses a
// tool call; such a result carries no permissionDecision.
const refusedTag = "The user doesn't want to proceed"

func toolResult(b block) source.Result {
	r := source.Result{CallID: b.ToolUseID}
	r.Error = b.IsError
	var text string
	if json.Unmarshal(b.Content, &text) == nil {
		r.Bytes = len(text)
		r.Persisted = strings.HasPrefix(text, persistedTag)
		r.Refused = strings.HasPrefix(text, refusedTag)
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
					r.Refused = strings.HasPrefix(p.Text, refusedTag)
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

// partKey identifies the part of a file a view printed by a hash, so a
// script or count is never kept; "" stays "" (the whole file).
func partKey(part string) string {
	if part == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(part))
	return hex.EncodeToString(sum[:8])
}

// rawArg is an argument as logged, "" if absent or null. A number written
// as a string ("100") reads the same as the number (100).
func rawArg(r json.RawMessage) string {
	var s string
	if json.Unmarshal(r, &s) == nil {
		return strings.TrimSpace(s)
	}
	if string(r) == "null" {
		return ""
	}
	return string(r)
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

// projectOf returns the project folder a working folder belongs to: the
// folder itself, or for a worktree, the folder of the project it was copied
// from. The caller turns folders into short names (ADR 0032 shows project
// names, not paths).
func projectOf(cwd string) string {
	if cwd == "" {
		return ""
	}
	if i := strings.Index(cwd, worktrees); i > 0 {
		cwd = cwd[:i]
	}
	return filepath.Clean(cwd)
}

// parseAttachment reads an attachment line: a snapshot of the system prompt
// and tool list, or text the harness added to the conversation. Only sizes
// and memory-file names are kept, never the text.
func parseAttachment(rec record, ts time.Time) (source.Message, bool, error) {
	a := rec.Attachment
	if a.Type == "queued_command" && a.CommandMode == "prompt" && a.Origin != nil && a.Origin.Kind == "human" {
		// A message the user typed while Claude was working. It reaches the
		// model as the user's message, not as text the harness added.
		return source.Message{Kind: source.KindPrompt, SessionID: rec.SessionID, Time: ts, LineID: rec.UUID}, true, nil
	}
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
			st.Tools = append(st.Tools, event.ToolDef{Name: clean(t.Name), Bytes: len(t.Name) + len(t.Description) + len(t.Schema)})
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
	switch {
	case a.Type == "instructions" && len(a.Files) > 0:
		for _, f := range a.Files {
			pieces = append(pieces, event.Injected{Kind: "memory_file", Name: memoryName(f), Bytes: len(f.Content)})
		}
	case a.Type == "mcp_instructions_delta" && len(a.AddedNames) > 0 && len(a.AddedNames) == len(a.AddedBlocks):
		// One piece per server, so each can be sorted by who can change it.
		for i, name := range a.AddedNames {
			pieces = append(pieces, event.Injected{Kind: a.Type, Name: clean(name), Bytes: len(a.AddedBlocks[i])})
		}
	default:
		pieces = []event.Injected{{Kind: clean(a.Type), Bytes: size}}
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
		name = f.Type + " " + name
	}
	return clean(name)
}

// replyUsage normalises a reply's usage. When the top level is all zero but
// the reply was logged as steps (iterations), their usage is added up
// instead, so the reply is not priced as free.
func replyUsage(top anthropic.Usage, steps []anthropic.Usage) event.Usage {
	u := anthropic.NormalizeUsage(top)
	// The top-level counts can be zero with only a stray cache-write split
	// left (a reply that triggered a compaction, logged after it): the steps
	// then hold the real figures.
	empty := top.InputTokens+top.CacheReadInputTokens+top.CacheCreationInputTokens+top.OutputTokens == 0
	if !empty || len(steps) == 0 {
		return u
	}
	var sum event.Usage
	for _, s := range steps {
		sum = sum.Add(anthropic.NormalizeUsage(s))
	}
	return sum
}

// typedPrompt reports whether a user line with no tool results is a message
// the user sent, rather than one the harness or another agent wrote. Newer
// lines say who sent them; for older ones, lines marked isMeta, compaction
// summaries and notices that a background task finished are not the user's.
// Only the answer is kept; the text is not.
func typedPrompt(rec record) bool {
	if rec.Origin != nil {
		return rec.Origin.Kind == "human"
	}
	if rec.IsMeta || rec.IsCompactSummary {
		return false
	}
	text := ""
	c := rec.Message.Content
	if len(c) > 0 && c[0] == '"' {
		if json.Unmarshal(c, &text) != nil {
			return false
		}
	} else {
		for _, b := range blocks(c) {
			if b.Type == "text" {
				text = b.Text
				break
			}
		}
	}
	text = strings.TrimSpace(text)
	return text != "" && !strings.HasPrefix(text, "<task-notification")
}

// limitError reports whether an assistant line's error says a usage limit
// was reached.
func limitError(raw json.RawMessage) bool {
	var s string
	return json.Unmarshal(raw, &s) == nil && s == "rate_limit"
}

// limitKind tells which subscription limit a harness limit message names,
// from its text ("You've hit your session limit · resets 11:30pm", "...
// weekly limit ...", "... Opus limit ...", or wordings such as "5-hour limit
// reached" and "Usage limit reached"): "session", "weekly", "other", or ""
// when the text names no usage limit (an API key's rate limit).
func limitKind(bs []block) string {
	for _, b := range bs {
		t := strings.ToLower(b.Text)
		named := strings.Contains(t, "hit your") && strings.Contains(t, "limit") ||
			strings.Contains(t, "usage limit") ||
			strings.Contains(t, "limit reached")
		if !named || strings.Contains(t, "rate limit") { // an API key's rate limit, in any wording
			continue
		}
		switch {
		case strings.Contains(t, "weekly"):
			return "weekly"
		case strings.Contains(t, "session limit"), strings.Contains(t, "5-hour"), strings.Contains(t, "five-hour"):
			return "session"
		}
		return "other"
	}
	return ""
}

// errorText is an error line's error field when it is a string.
func errorText(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// failureKind sorts an error reply the harness wrote, by its text and error
// field: "sleep" ("API Error: Your computer went to sleep mid-response"),
// "not-sent" (refused before any work was done, so nothing was charged: an
// expired login, a rate limit, an overloaded service, a server that could
// not be reached), "network" (a connection lost or a server error during
// the request), or "other".
func failureKind(errField string, bs []block) string {
	// The text comes first: Claude Code marks a reply cut off by sleep, and
	// a server it could not reach, with a server error.
	for _, b := range bs {
		t := strings.ToLower(b.Text)
		switch {
		case strings.Contains(t, "went to sleep"):
			return "sleep"
		case strings.Contains(t, "/login"), strings.Contains(t, "login expired"),
			strings.Contains(t, "enotfound"), strings.Contains(t, "econnrefused"),
			strings.Contains(t, "connection refused"), strings.Contains(t, "reach the api server"):
			return "not-sent"
		}
	}
	switch errField {
	case "authentication_failed", "invalid_request", "billing_error", "rate_limit", "overloaded":
		return "not-sent"
	case "server_error":
		return "network"
	}
	for _, b := range bs {
		t := strings.ToLower(b.Text)
		switch {
		case strings.Contains(t, "rate limit"), strings.Contains(t, "overloaded"):
			return "not-sent"
		case strings.Contains(t, "connection"), strings.Contains(t, "network"),
			strings.Contains(t, "timed out"), strings.Contains(t, "server error"):
			return "network"
		}
	}
	return "other"
}
