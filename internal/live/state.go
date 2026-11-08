package live

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// State remembers, per session, which nudges were shown, so each appears
// once. It holds numbers and times only.
type State struct {
	Crossed   map[int]bool `json:"crossed"`    // after-reply thresholds already shown
	PausedFor time.Time    `json:"paused_for"` // the reply a pause before sending was shown after
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
		return nil
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, b, 0o600); err != nil {
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
