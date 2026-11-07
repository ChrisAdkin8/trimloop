package claudecode

import (
	"encoding/json"
	"iter"
	"time"

	"github.com/ChrisAdkin8/trimloop/internal/adapters/source"
	"github.com/ChrisAdkin8/trimloop/internal/adapters/source/jsonl"
	"github.com/ChrisAdkin8/trimloop/pkg/normalize/anthropic"
)

// record is the part of a transcript line that Read uses.
type record struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	Timestamp json.RawMessage `json:"timestamp"` // RFC 3339 string; parsed apart, see parseLine
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			anthropic.Usage
			Speed        string `json:"speed"`         // "fast" in fast mode
			InferenceGeo string `json:"inference_geo"` // "us" for US-only inference
		} `json:"usage"` // nil when the line has no usage
	} `json:"message"`
}

// Read streams the assistant messages with usage in one log. It follows the
// contract in source.Reader. Claude Code writes one line per content block,
// each repeating the same usage, so repeats of a message id are expected.
func (Reader) Read(log *source.Log) iter.Seq2[source.Message, error] {
	return jsonl.Lines(log, parseLine)
}

// parseLine turns one transcript line into a Message. Lines that are not
// assistant messages with usage are skipped.
func parseLine(line []byte) (source.Message, bool, error) {
	var rec record
	if err := json.Unmarshal(line, &rec); err != nil {
		return source.Message{}, false, err
	}
	u := rec.Message.Usage
	if rec.Type != "assistant" || u == nil {
		return source.Message{}, false, nil
	}
	// A timestamp that is missing or in an unexpected format leaves Time
	// zero rather than failing the line: the usage is still worth counting.
	var ts time.Time
	var s string
	if json.Unmarshal(rec.Timestamp, &s) == nil {
		ts, _ = time.Parse(time.RFC3339Nano, s)
	}
	return source.Message{
		ID:        rec.Message.ID,
		SessionID: rec.SessionID,
		Time:      ts,
		Model:     rec.Message.Model,
		Usage:     anthropic.NormalizeUsage(u.Usage),
		Fast:      u.Speed == "fast",
		USOnly:    u.InferenceGeo == "us",
	}, true, nil
}
