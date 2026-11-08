package main

import (
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

// between is what happened in a log since its last new turn, waiting to be
// handed to the next one. It is a small state machine with two phases:
//
//   - open (compaction == nil): text the harness added and the user's
//     messages collect in after, for the next turn.
//   - compacted (compaction != nil): a compaction marker was seen. What came
//     before the marker moved to before; after starts empty and collects what
//     follows the marker.
//
// Events and transitions:
//
//	inject / prompt      either phase: add to after
//	compact              open → compacted; compacted → compacted, and the
//	                     earlier compaction is returned as never replied to
//	take, late trigger   compacted → compacted: a reply logged after the
//	                     marker but timed before it caused the compaction; it
//	                     takes before, and the compaction waits for the next
//	take, otherwise      either phase → open: the turn takes before and after,
//	                     and the compaction if there is one
//	end                  a compaction still pending was never replied to
//
// See docs/measurements.md ("Compaction") and ADR 0045 rule 5.
type between struct {
	before, after carry
	compaction    *pendingCompaction // nil when open
}

// carry is what a turn inherits: added text, and whether the user acted.
type carry struct {
	injected []event.Injected
	prompted bool
}

// pendingCompaction is a compaction no turn has taken yet, and when it was
// logged as happening (zero if not known).
type pendingCompaction struct {
	c  event.Compaction
	at time.Time
}

// handover is what a new turn takes from between.
type handover struct {
	injected   []event.Injected
	prompted   bool
	compaction *event.Compaction // the compaction before this turn; nil if none
}

func (b *between) inject(x []event.Injected) { b.after.injected = append(b.after.injected, x...) }
func (b *between) prompt()                   { b.after.prompted = true }

// compact records a compaction marker logged at at. If one was already
// pending, no reply followed it: it is returned, with its time, to be kept
// as unreplied.
func (b *between) compact(c event.Compaction, at time.Time) (unreplied *event.Compaction) {
	if p := b.compaction; p != nil {
		u := p.c
		u.At = p.at
		unreplied = &u
	}
	b.compaction = &pendingCompaction{c: c, at: at}
	b.before = carry{
		injected: append(b.before.injected, b.after.injected...),
		prompted: b.before.prompted || b.after.prompted,
	}
	b.after = carry{}
	return unreplied
}

// lateTrigger reports whether a reply timed at t, logged after the pending
// compaction's marker, came before it: the reply that triggered it.
func (b *between) lateTrigger(t time.Time) bool {
	p := b.compaction
	return p != nil && !p.at.IsZero() && !t.IsZero() && t.Before(p.at)
}

// take hands a new turn timed at t what it inherits, and moves on.
func (b *between) take(t time.Time) handover {
	if b.lateTrigger(t) {
		h := handover{injected: b.before.injected, prompted: b.before.prompted}
		b.before = carry{}
		return h // still compacted: the next turn takes the compaction
	}
	h := handover{
		injected: append(b.before.injected, b.after.injected...),
		prompted: b.before.prompted || b.after.prompted,
	}
	if p := b.compaction; p != nil {
		c := p.c
		h.compaction = &c
	}
	*b = between{}
	return h
}

// end returns the compaction still pending when the log ends, with its time:
// no reply followed it. It returns nil if none is pending.
func (b *between) end() *event.Compaction {
	if p := b.compaction; p != nil {
		c := p.c
		c.At = p.at
		return &c
	}
	return nil
}
