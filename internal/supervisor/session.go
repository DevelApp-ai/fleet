// Package supervisor keeps the supervisor's memory on disk (DESIGN.md §6.5):
// the current session record, notes in decisions.log, handoffs, and the
// context meter read from Copilot's own usage records.
package supervisor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// BootPrompt starts a fresh supervisor after a reset.
const BootPrompt = "You are taking over as fleet supervisor. Call fleet_brief, tell the user in 1–3 lines what needs them, then wait."

// Session is the current supervisor, stored in <home>/supervisor/session.json.
type Session struct {
	SessionID  string    `json:"session_id"`
	Profile    string    `json:"profile"`
	PaneID     string    `json:"pane_id"`
	Started    time.Time `json:"started"`
	Generation int       `json:"generation"`
	Previous   string    `json:"previous,omitempty"`
}

// Dir is <home>/supervisor.
func Dir(home string) string { return filepath.Join(home, "supervisor") }

func sessionPath(home string) string { return filepath.Join(Dir(home), "session.json") }

// Load reads session.json; ok is false when it is missing or broken.
func Load(home string) (Session, bool) {
	var s Session
	b, err := os.ReadFile(sessionPath(home))
	if err != nil || json.Unmarshal(b, &s) != nil {
		return Session{}, false
	}
	return s, true
}

// Save writes session.json atomically.
func Save(home string, s Session) error {
	if err := os.MkdirAll(Dir(home), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := sessionPath(home) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, sessionPath(home))
}
