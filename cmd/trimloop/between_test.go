package main

import (
	"testing"
	"time"

	"github.com/ChrisAdkin8/trimloop/pkg/event"
)

func inj(name string) []event.Injected { return []event.Injected{{Name: name}} }

func names(x []event.Injected) []string {
	var out []string
	for _, i := range x {
		out = append(out, i.Name)
	}
	return out
}

func TestBetweenOpen(t *testing.T) {
	var b between
	b.inject(inj("a"))
	b.prompt()
	h := b.take(t0)
	if got := names(h.injected); len(got) != 1 || got[0] != "a" || !h.prompted || h.compaction != nil {
		t.Errorf("open: got %+v", h)
	}
	if h := b.take(t0); h.injected != nil || h.prompted || h.compaction != nil {
		t.Errorf("after a take, nothing is left: got %+v", h)
	}
	if b.end() != nil {
		t.Error("open at the end: nothing unreplied")
	}
}

func TestBetweenCompactedSplitsBeforeAndAfter(t *testing.T) {
	var b between
	b.inject(inj("before"))
	b.prompt()
	if u := b.compact(event.Compaction{Before: 900}, t0.Add(time.Minute)); u != nil {
		t.Fatalf("first compaction: got unreplied %+v", u)
	}
	b.inject(inj("after"))

	// A reply logged after the marker but timed before it triggered the
	// compaction: it takes only what came before.
	h := b.take(t0)
	if got := names(h.injected); len(got) != 1 || got[0] != "before" || !h.prompted || h.compaction != nil {
		t.Errorf("late trigger: got %+v", h)
	}
	// The next reply takes the compaction and what followed the marker.
	h = b.take(t0.Add(2 * time.Minute))
	if got := names(h.injected); len(got) != 1 || got[0] != "after" || h.prompted || h.compaction == nil || h.compaction.Before != 900 {
		t.Errorf("first reply after: got %+v", h)
	}
	if b.compaction != nil {
		t.Error("open again after the first reply")
	}
}

func TestBetweenWithoutTimesNoLateTrigger(t *testing.T) {
	var b between
	b.inject(inj("x"))
	b.compact(event.Compaction{}, time.Time{}) // marker time unknown
	if b.lateTrigger(t0) {
		t.Error("an unknown marker time cannot place a reply before it")
	}
	if h := b.take(time.Time{}); h.compaction == nil || len(h.injected) != 1 {
		t.Errorf("reply with no time: got %+v, want the compaction and everything carried", h)
	}
}

func TestBetweenTwoCompactionsInARow(t *testing.T) {
	var b between
	b.inject(inj("1"))
	b.compact(event.Compaction{Before: 1}, t0)
	b.inject(inj("2"))
	u := b.compact(event.Compaction{Before: 2}, t0.Add(time.Minute))
	if u == nil || u.Before != 1 || !u.At.Equal(t0) {
		t.Fatalf("second compaction: the first is unreplied, with its time; got %+v", u)
	}
	// Both stretches of text came before the pending (second) marker.
	h := b.take(t0.Add(30 * time.Second)) // before the second marker: its trigger
	if got := names(h.injected); len(got) != 2 || got[0] != "1" || got[1] != "2" || h.compaction != nil {
		t.Errorf("trigger of the second: got %+v", h)
	}
	if e := b.end(); e == nil || e.Before != 2 || !e.At.Equal(t0.Add(time.Minute)) {
		t.Errorf("end: the second is unreplied; got %+v", e)
	}
}
