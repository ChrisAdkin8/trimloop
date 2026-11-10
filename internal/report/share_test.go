package report

import (
	"strings"
	"testing"
)

// A subscriber's share says the plan is a flat fee without naming it, and a
// short report with no period, findings or version still reads as sentences.
func TestWriteShare(t *testing.T) {
	for _, c := range []struct {
		name string
		r    Results
		want string
	}{
		{"nothing but a total", Results{Total: 12.5, Read: Read{Sessions: 1, Turns: 3}},
			"Claude Code usage: $13 at list prices (1 session, 3 turns).\n\n" +
				"Worked out by trimloop from local logs: https://github.com/ChrisAdkin8/trimloop\n"},
		{"a subscriber", Results{Total: 300, Version: "v1.2.3", Plan: Plan{Name: "Max 5×", Subscription: true},
			Period: Period{Days: 14, PerWeek: 150}, Read: Read{Sessions: 20, Turns: 400}},
			"Claude Code usage over 14 days: $300 at list prices, about $150 a week (20 sessions, 400 turns).\n" +
				"On a flat-fee plan, so list prices measure how much of the usage limits the work takes.\n\n" +
				"Worked out by trimloop v1.2.3 from local logs: https://github.com/ChrisAdkin8/trimloop\n"},
	} {
		var b strings.Builder
		if err := WriteShare(&b, c.r, 3); err != nil {
			t.Fatal(err)
		}
		if b.String() != c.want {
			t.Errorf("%s: got\n%s\nwant\n%s", c.name, b.String(), c.want)
		}
	}
}
