package event

// BytesPerToken is the estimate used to turn a tool result's size into
// tokens. Logs give a result's bytes, not its tokens; four bytes per token is
// a common rule of thumb for English text and code.
const BytesPerToken = 4

// ToolCall is one tool call a turn made, and its result. It holds sizes and
// derived labels only, never the command text or the output.
type ToolCall struct {
	ID   string // the harness's tool-use id, which links the result to the call
	Tool string // tool name as the harness reports it, e.g. "Bash", "Read", "mcp__github__get_issue"

	// Shape is a shell command's shape (see package cmdshape), e.g. "sed -n"
	// or "go test -v". Empty for tools that are not shells.
	Shape string
	// Noise is the kind of output the call produces (see NoiseKind).
	Noise NoiseKind
	// Filtered is true when a shell command's output was already piped
	// through a filter (head, tail, grep, jq, ...) or sent to a file.
	Filtered bool

	// File identifies the file the call viewed or edited, for spotting
	// re-reads within a session. It is a hash of the path, never the path,
	// and is used in memory only. Empty if the call touched no single file.
	File   string
	FileOp FileOp

	Result    ToolResult
	HasResult bool // false if the log has no result for the call

	// InputBytes is the size of the call's arguments as logged: the command,
	// script or file content Claude wrote. It stays in the conversation like
	// any reply (ADR 0050).
	InputBytes int
}

// FileOp says what a call did to File.
type FileOp int

// What a call can do to a file.
const (
	FileNone FileOp = iota
	FileView        // read the file: Read, cat, sed -n, head, tail
	FileEdit        // changed the file: Edit, Write
)

// ToolResult is the size and outcome of a tool call's result, as the model
// received it.
type ToolResult struct {
	Bytes int  // size of the text the model received
	Error bool // the harness marked the result as an error

	// Persisted is true when the output was too large for the harness, which
	// saved it to disk and sent the model a preview instead. Bytes is then
	// the preview's size.
	Persisted bool
}

// Tokens is the result's estimated size in tokens, rounded up.
func (r ToolResult) Tokens() int { return (r.Bytes + BytesPerToken - 1) / BytesPerToken }

// InputTokens is the call's input size in estimated tokens.
func (c ToolCall) InputTokens() int { return (c.InputBytes + BytesPerToken - 1) / BytesPerToken }

// NoiseKind sorts tool output by where it comes from, which decides the fix
// (docs/patterns.md, P04 "Kinds of noise").
type NoiseKind string

// The kinds of noise.
const (
	NoiseOther     NoiseKind = ""
	NoiseFileView  NoiseKind = "file-view"  // whole or partial file views
	NoiseSearch    NoiseKind = "search"     // grep, find, ls -R, search tools
	NoiseBuildTest NoiseKind = "build-test" // build, test and install logs
	NoiseData      NoiseKind = "data"       // inline scripts, API and JSON dumps
	NoiseWeb       NoiseKind = "web"        // web search and fetch
	NoiseMCP       NoiseKind = "mcp"        // MCP server tools
	NoiseSubAgent  NoiseKind = "sub-agent"  // a sub-agent's final report
)
