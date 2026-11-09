package report

import (
	"bytes"
	"strings"
	"testing"
	"unicode"

	"github.com/ChrisAdkin8/trimloop/pkg/detectors"
)

// guardDirty is log text that tries to drive the terminal (OSC with BEL,
// CSI) or hide what is shown (a bidi override, a zero-width space).
const guardDirty = "x\x1b]0;PWNED\x07\x1b[31m\u202eRED\u200b"

func TestPrintableFormatCharacters(t *testing.T) {
	for in, want := range map[string]string{
		"a\u202eb":    "a?b",
		"zero\u200bw": "zero?w",
		"bom\ufeff":   "bom?",
		"日本 🎉 café":   "日本 🎉 café",
	} {
		if got := printable(in); got != want {
			t.Errorf("printable(%q) = %q, want %q", in, got, want)
		}
	}
}

// Log text the reader would have cleaned is still made safe where the text
// report prints it: tool names in the tool-input line, model names in P13
// and the reconciliation, and effort levels.
func TestTextGuardsLogText(t *testing.T) {
	r := Analyse(sample(), Read{Logs: 1}, Local{}, DefaultSettings(), t0)
	r.ToolInputs = detectors.ToolInputs{Carried: 5, ByTool: []detectors.Group{{Name: "tool" + guardDirty, Carried: 5}}}
	r.MainModels.ByModel = []detectors.ModelSpend{{Model: "model" + guardDirty, Turns: 1, Cost: 1}}
	r.Effort.ByLevel = []detectors.EffortLevel{{Level: "effort" + guardDirty, Turns: 1}}
	r.Reconciliation.Sessions = 1
	r.Reconciliation.ByModel = []detectors.ModelGap{{Model: "recmodel" + guardDirty, Harness: 2, Ours: 1, Gap: 1}}
	var out bytes.Buffer
	if err := WriteText(&out, r); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, name := range []string{"tool", "model", "effort", "recmodel"} {
		if !strings.Contains(text, name+"x?]0;PWNED??[31m?RED?") {
			t.Errorf("no guarded %s in the text report", name)
		}
	}
	if strings.ContainsFunc(text, func(r rune) bool {
		return r != '\n' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r))
	}) {
		t.Errorf("control or format characters in the text report:\n%q", text)
	}
}
