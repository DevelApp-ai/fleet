package supervisor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	keepHandoffs = 20
	maxHandoff   = 4 << 10
)

func handoffPath(home string) string { return filepath.Join(Dir(home), "handoff.md") }

// WriteHandoff writes supervisor/handoff.md, moving the previous one to
// supervisor/handoffs/<timestamp>.md and keeping the last 20 of those.
func WriteHandoff(home, text string) error {
	old := filepath.Join(Dir(home), "handoffs")
	if err := os.MkdirAll(old, 0o755); err != nil {
		return err
	}
	if st, err := os.Stat(handoffPath(home)); err == nil {
		// <mtime>-<n>.md: lexical order is time order, also on collisions.
		base := st.ModTime().Format("20060102T150405.000000000")
		dst := ""
		for i := 0; dst == "" || exists(dst); i++ {
			dst = filepath.Join(old, fmt.Sprintf("%s-%03d.md", base, i))
		}
		if err := os.Rename(handoffPath(home), dst); err != nil {
			return err
		}
		prune(old)
	}
	return os.WriteFile(handoffPath(home), []byte(strings.TrimRight(text, "\n")+"\n"), 0o644)
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func prune(dir string) {
	es, _ := os.ReadDir(dir)
	var names []string
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keepHandoffs {
		os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}

// ReadHandoff returns the latest handoff (capped at 4 KB) and its time.
func ReadHandoff(home string) (string, time.Time, bool) {
	st, err := os.Stat(handoffPath(home))
	if err != nil {
		return "", time.Time{}, false
	}
	b, err := os.ReadFile(handoffPath(home))
	if err != nil {
		return "", time.Time{}, false
	}
	s := string(b)
	if len(s) > maxHandoff {
		s = s[:maxHandoff] + "\n…[truncated]"
	}
	return strings.TrimRight(s, "\n"), st.ModTime(), true
}
