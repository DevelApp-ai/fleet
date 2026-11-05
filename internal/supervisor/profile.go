package supervisor

import (
	"os"
	"path/filepath"
	"strings"
)

func choicePath(home string) string { return filepath.Join(Dir(home), "profile") }

// LoadChoice returns the admiral profile last chosen (DESIGN §6.10), "" if none.
func LoadChoice(home string) string {
	b, err := os.ReadFile(choicePath(home))
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return strings.TrimSpace(line)
}

// SaveChoice persists the admiral profile choice as one line.
func SaveChoice(home, name string) error {
	if err := os.MkdirAll(Dir(home), 0o755); err != nil {
		return err
	}
	tmp := choicePath(home) + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.TrimSpace(name)+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, choicePath(home))
}
