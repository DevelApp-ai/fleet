package supervisor

import (
	"fmt"

	"github.com/sturlabragason/fleet/internal/tmux"
)

// ErrNoPane is returned when the main window has no @role=sup pane.
var ErrNoPane = fmt.Errorf("no admiral pane; run fleet up")

// FindPane returns the @role=sup pane in target (e.g. "fleet:main").
func FindPane(t *tmux.Client, target string) (string, error) {
	ps, err := t.ListPanes(target)
	if err != nil {
		return "", ErrNoPane
	}
	for _, p := range ps {
		if p.Role == "sup" {
			return p.ID, nil
		}
	}
	return "", ErrNoPane
}

// Current returns session.json, or a record with only the session ID taken
// from the admiral pane's start command when session.json is missing.
func Current(home string, t *tmux.Client, target string) Session {
	if s, ok := Load(home); ok {
		return s
	}
	pane, err := FindPane(t, target)
	if err != nil {
		return Session{}
	}
	cmd, _ := t.Run("display-message", "-p", "-t", pane, "#{pane_start_command}")
	return Session{SessionID: SessionIDFromCommand(cmd), PaneID: pane}
}

// CurrentFill is Fill for the current admiral.
func CurrentFill(home string, t *tmux.Client, target string) (used, max int, ok bool) {
	return Fill(CopilotHome(), Current(home, t, target).SessionID)
}
