package detectors

import (
	"sort"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// Growth shows where spend piles up and how sessions are compacted: the
// concentration behind P19, and P10 (compaction) in reduced fidelity from
// the harness's compaction records (ADR 0031).
type Growth struct {
	Conversations int     `json:"conversations"` // main sessions with their sub-agents
	Cost          float64 `json:"cost"`          // their spend
	MedianCost    float64 `json:"median_cost"`   // spend of the median conversation

	TopShares []TopShare         `json:"top_shares"` // share of spend in the top 1, 5 and 10 conversations
	Top       []ConversationStat `json:"top"`        // the costliest conversations, largest first

	Bands []Band `json:"bands"` // turns by context size, smallest first

	Compactions  int `json:"compactions"`   // compactions recorded
	Manual       int `json:"manual"`        // of them, asked for by the user
	MedianBefore int `json:"median_before"` // median context before a compaction, in tokens
	MedianAfter  int `json:"median_after"`  // median size the harness recorded after: the summary
	// MedianResume is the median context of the first request after a
	// compaction. It is larger than MedianAfter: the harness sends the system
	// prompt and tool definitions again and re-attaches files, skill and tool
	// listings. (2.3–4.8 times on the founder's logs, 2026-11-08.)
	MedianResume int `json:"median_resume"`
	// ResumeLow and ResumeHigh are the lower and upper quartiles of the
	// same: how much the likely compaction saving depends on it.
	ResumeLow  int `json:"resume_low"`
	ResumeHigh int `json:"resume_high"`
	Resumes    int `json:"resumes"` // compactions those were measured on
	// Longest is the longest a compaction took. A median would mislead:
	// Claude Code prepares some compactions in advance, which then take
	// milliseconds, while the rest make the user wait.
	Longest time.Duration `json:"longest_ns"`

	// Sessions (main or sub-agent) whose context passed Limit tokens and
	// that were never compacted, and what they cost.
	Limit           int     `json:"limit"`
	Uncompacted     int     `json:"uncompacted"`
	UncompactedCost float64 `json:"uncompacted_cost"`
}

// TopShare is the share of all spend in the N costliest conversations.
type TopShare struct {
	N     int     `json:"n"`
	Share float64 `json:"share"` // 0 to 1
}

// ConversationStat is one conversation's spend.
type ConversationStat struct {
	ID      string  `json:"id"`
	Project string  `json:"project"` // the project with most of its spend
	Turns   int     `json:"turns"`
	Peak    int     `json:"peak"` // largest context of any turn, in tokens
	Cost    float64 `json:"cost"`
}

// Band is the turns whose context falls in [Min, Max) tokens.
type Band struct {
	Min   int     `json:"min"`
	Max   int     `json:"max"` // 0 means no upper bound
	Turns int     `json:"turns"`
	Cost  float64 `json:"cost"`
}

// MeanCost is the average cost of a turn in the band.
func (b Band) MeanCost() float64 {
	if b.Turns == 0 {
		return 0
	}
	return b.Cost / float64(b.Turns)
}

// bandEdges are the context sizes, in tokens, that split turns into bands.
var bandEdges = []int{50_000, 200_000, 500_000}

// FindGrowth reports how concentrated spend is, the cost of a turn by
// context size, and compactions. limit is the context, in tokens, above which
// a session that was never compacted is reported; topN limits Top.
func FindGrowth(sessions []event.Session, limit, topN int) Growth {
	f := Growth{Limit: limit}
	conv := map[string]*ConversationStat{}
	projects := map[string]map[string]float64{} // conversation -> project -> cost
	var order []string
	f.Bands = make([]Band, len(bandEdges)+1)
	for i := range f.Bands {
		if i > 0 {
			f.Bands[i].Min = bandEdges[i-1]
		}
		if i < len(bandEdges) {
			f.Bands[i].Max = bandEdges[i]
		}
	}
	var before, after, resume []int

	for _, s := range sessions {
		id := s.Conversation()
		c, ok := conv[id]
		if !ok {
			c = &ConversationStat{ID: id}
			conv[id] = c
			projects[id] = map[string]float64{}
			order = append(order, id)
		}

		cost, peak, compacted := 0.0, 0, false
		for _, t := range s.Turns {
			ctx := t.Context()
			cost += t.Cost.Total()
			peak = max(peak, ctx)
			if t.Project != "" {
				projects[id][t.Project] += t.Cost.Total()
			}
			b := &f.Bands[sort.SearchInts(bandEdges, ctx)] // exactly 200k is not over 200k
			b.Turns++
			b.Cost += t.Cost.Total()
			if t.CompactedBefore {
				compacted = true
				f.Compactions++
				if t.Compaction.Manual {
					f.Manual++
				}
				if t.Compaction.Before > 0 {
					before = append(before, t.Compaction.Before)
					after = append(after, t.Compaction.After)
				}
				// What the next request sent, measured on main sessions (those
				// the user can compact), and only when it is smaller than the
				// session was before: a larger one is not a request after it.
				if ctx > 0 && s.Kind != event.SubAgentSession && (t.Compaction.Before == 0 || ctx < t.Compaction.Before) {
					resume = append(resume, ctx)
				}
				f.Longest = max(f.Longest, t.Compaction.Duration)
			}
		}
		for _, tc := range s.Unreplied {
			// No reply followed it: counted, with no reply to measure.
			compacted = true
			f.Compactions++
			if tc.Manual {
				f.Manual++
			}
			if tc.Before > 0 {
				before = append(before, tc.Before)
				after = append(after, tc.After)
			}
			f.Longest = max(f.Longest, tc.Duration)
		}
		c.Turns += len(s.Turns)
		c.Cost += cost
		c.Peak = max(c.Peak, peak)
		f.Cost += cost
		if peak > limit && !compacted {
			f.Uncompacted++
			f.UncompactedCost += cost
		}
	}

	all := make([]ConversationStat, 0, len(order))
	for _, id := range order {
		c := conv[id]
		if names := largest(projects[id], 1); len(names) > 0 {
			c.Project = names[0] // where most of its spend was
		}
		all = append(all, *c)
	}
	sortBy(all, func(c ConversationStat) (float64, string) { return c.Cost, c.ID })
	f.Conversations = len(all)
	if len(all) > 0 {
		f.MedianCost = all[len(all)/2].Cost
	}
	for _, n := range []int{1, 5, 10} {
		if n > len(all) || f.Cost == 0 {
			break
		}
		sum := 0.0
		for _, c := range all[:n] {
			sum += c.Cost
		}
		f.TopShares = append(f.TopShares, TopShare{N: n, Share: sum / f.Cost})
	}
	f.Top = all[:min(topN, len(all))]

	f.MedianBefore, f.MedianAfter, f.MedianResume = median(before), median(after), median(resume)
	f.ResumeLow, f.ResumeHigh, f.Resumes = quantile(resume, 0.25), quantile(resume, 0.75), len(resume)
	return f
}

// median returns the middle value of xs (the upper middle for an even
// count), or 0 if xs is empty. It sorts a copy.
func median(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	return s[len(s)/2]
}

// quantile returns the value at position q·n of the sorted xs, the same
// rule as median (q 0.5), so the median always lies between the quartiles;
// 0 for none.
func quantile(xs []int, q float64) int {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	return s[min(len(s)-1, int(q*float64(len(s))))]
}
