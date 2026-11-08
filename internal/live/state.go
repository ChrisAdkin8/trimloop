package live

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// State remembers, per session, which nudges were shown, so each appears
// once. It holds numbers and times only. Its two parts change only through
// afterReply and beforeSend (see docs/state-machines.md §4):
//
//   - Crossed is the size thresholds already announced. A reply under a
//     threshold re-arms it (the session was compacted or cleared); a reply
//     past an armed one announces the highest, once.
//   - PausedFor is the reply a pause before sending was shown after, so a
//     long break pauses once.
type State struct {
	Crossed   map[int]bool `json:"crossed"`    // after-reply thresholds already shown
	PausedFor time.Time    `json:"paused_for"` // the reply a pause before sending was shown after
}

// afterReply moves the size nudges on after reply r. It returns the message
// to show and the threshold it announces ("" and 0 if none is due), and
// whether any threshold was re-armed (the state changed even with no
// message).
func (st *State) afterReply(s Settings, r Reply) (msg string, threshold int, rearmed bool) {
	for t := range st.Crossed {
		if r.Context <= t {
			delete(st.Crossed, t)
			rearmed = true
		}
	}
	if rearmed && r.Context > s.Large {
		st.Crossed[s.Large] = true // re-armed but still past it: not news
	}
	msg, threshold = s.AfterReply(r, st.Crossed)
	if msg != "" {
		st.Crossed[threshold] = true
	}
	return msg, threshold, rearmed
}

// beforeSend decides whether to pause the message the user just sent, and
// remembers the pause so the same break pauses once. It returns the reason
// to show, or "".
func (st *State) beforeSend(s Settings, r Reply, now time.Time) string {
	reason := s.BeforeSend(r, now, st.PausedFor)
	if reason != "" {
		st.PausedFor = r.Time
	}
	return reason
}

// Store keeps State files in Dir, one per session.
type Store struct {
	Dir string
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func (s Store) path(session string) (string, bool) {
	if !safeID.MatchString(session) {
		return "", false
	}
	return filepath.Join(s.Dir, session+".json"), true
}

// Load returns the session's state; a missing or unreadable file is a fresh
// state.
func (s Store) Load(session string) State {
	st := State{Crossed: map[int]bool{}}
	p, ok := s.path(session)
	if !ok {
		return st
	}
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, &st) // a damaged file starts fresh
	}
	if st.Crossed == nil {
		st.Crossed = map[int]bool{}
	}
	return st
}

// Save writes the session's state, and removes states untouched for 30 days.
func (s Store) Save(session string, st State) error {
	p, ok := s.path(session)
	if !ok {
		return fmt.Errorf("session id %q cannot be used as a file name", session)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, ".state-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return err
	}
	s.prune(30 * 24 * time.Hour)
	return nil
}

func (s Store) prune(age time.Duration) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > age {
			_ = os.Remove(filepath.Join(s.Dir, e.Name()))
		}
	}
}
