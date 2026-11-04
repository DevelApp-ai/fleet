package supervisor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// now is replaceable in tests.
var now = time.Now

func notesPath(home string) string { return filepath.Join(home, "decisions.log") }

// AppendNote adds one line to decisions.log: `2026-11-04T21:40 [agent] text`.
func AppendNote(home, agent, text string) error {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return fmt.Errorf("note text is empty")
	}
	line := now().Format("2006-01-02T15:04") + " "
	if agent = strings.TrimSpace(agent); agent != "" {
		line += "[" + agent + "] "
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(notesPath(home), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + text + "\n")
	return err
}

// TailNotes returns the last n lines of decisions.log (nil if none).
func TailNotes(home string, n int) []string {
	b, err := os.ReadFile(notesPath(home))
	if err != nil {
		return nil
	}
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
