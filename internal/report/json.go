package report

import (
	"encoding/json"
	"io"
)

// WriteJSON writes r as indented JSON. Field names are stable within a
// FormatVersion.
func WriteJSON(out io.Writer, r Results) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
