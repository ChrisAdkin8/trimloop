package report

// The summary at the top of the page answers two questions with two bars:
// where the money went by conversation size (spend against turns), and what
// filled the conversation (the kinds of text sent).

// summaryView is what the page's summary section shows.
type summaryView struct {
	Threshold       int     // the "large" size, P19's threshold
	LargeSpendShare float64 // share of spend in turns over Threshold, 0 to 100
	LargeTurnShare  float64 // share of turns over Threshold, 0 to 100
	SmallCost       string  // cost per turn of turns under Threshold, e.g. "$0.06–$0.08"
	LargeCost       string  // cost per turn of turns over it, e.g. "$0.15–$0.28"
	Bands           []bandShare

	Input  float64 // cost of everything sent to Claude
	Output float64 // cost of Claude's replies
	Fill   []fillPart
}

// bandShare is one conversation-size band.
type bandShare struct {
	Label      string // "Under 50k", "200k–500k"
	Class      string // CSS class that sets its colour
	Cost       float64
	SpendShare float64 // 0 to 100
	TurnShare  float64 // 0 to 100
}

// fillPart is one kind of text in the conversation.
type fillPart struct {
	Label string
	Class string // CSS class that sets its colour
	Cost  float64
	Share float64 // of Input, 0 to 100
}

// bandClasses colour the size bands from small (green) to very large (red),
// as the live status line does. Colours are set by class: html/template does
// not allow var() in a style attribute.
var bandClasses = []string{"size1", "size2", "size3", "size4"}

func newSummary(r Results) summaryView {
	s := summaryView{Threshold: r.Context.Threshold, Input: r.Cost.Input(), Output: r.Cost.Output}
	turns := 0
	for _, b := range r.Growth.Bands {
		turns += b.Turns
	}
	var small, large []float64
	for i, b := range r.Growth.Bands {
		label := approx(b.Min) + "–" + approx(b.Max)
		switch {
		case b.Min == 0:
			label = "Under " + approx(b.Max)
		case b.Max == 0:
			label = "Over " + approx(b.Min)
		}
		bs := bandShare{Label: label, Class: bandClasses[min(i, len(bandClasses)-1)], Cost: b.Cost,
			SpendShare: pct(b.Cost, r.Total), TurnShare: pct(float64(b.Turns), float64(turns))}
		s.Bands = append(s.Bands, bs)
		if b.Turns == 0 {
			continue
		}
		if b.Min >= s.Threshold {
			s.LargeSpendShare += bs.SpendShare
			s.LargeTurnShare += bs.TurnShare
			large = append(large, b.MeanCost())
		} else {
			small = append(small, b.MeanCost())
		}
	}
	s.SmallCost, s.LargeCost = costRange(small), costRange(large)

	tools := r.Tools.Carried
	standing := r.Standing.SystemPrompt + r.Standing.ToolDefinitions
	added := r.Standing.Injected
	rest := max(0, s.Input-tools-standing-added)
	for _, p := range []fillPart{
		{"Your messages and Claude's replies, sent again each turn", "kind1", rest, 0},
		{"Tool output", "kind2", tools, 0},
		{"System prompt and tool definitions", "kind3", standing, 0},
		{"Text Claude Code added", "kind4", added, 0},
	} {
		p.Share = pct(p.Cost, s.Input)
		s.Fill = append(s.Fill, p)
	}
	return s
}

// costRange writes the lowest and highest cost per turn: "$0.06–$0.08", or
// one figure if they round the same.
func costRange(costs []float64) string {
	if len(costs) == 0 {
		return ""
	}
	lo, hi := costs[0], costs[0]
	for _, c := range costs {
		lo, hi = min(lo, c), max(hi, c)
	}
	if money(lo) == money(hi) {
		return money(lo)
	}
	return money(lo) + "–" + money(hi)
}
