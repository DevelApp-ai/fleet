package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each testdata/<set>/<state>[-<variant>].txt must classify as <state>.
func TestCaptures(t *testing.T) {
	sets := Load(t.TempDir())
	files, _ := filepath.Glob("testdata/*/*.txt")
	if len(files) == 0 {
		t.Fatal("no testdata")
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		set := filepath.Base(filepath.Dir(f))
		want := Status(strings.SplitN(strings.TrimSuffix(filepath.Base(f), ".txt"), "-", 2)[0])
		got, _ := sets[set].Detect(string(b))
		if got != want {
			t.Errorf("%s: got %s want %s", f, got, want)
		}
	}
}

func TestUnknownAndOverride(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "patterns.toml"), []byte("[fake]\nidle = ['zzz']\n"), 0o644)
	s := Load(dir)["fake"]
	if st, ok := s.Detect("hello"); ok || st != Unknown {
		t.Errorf("got %s", st)
	}
	if st, _ := s.Detect("zzz"); st != Idle {
		t.Errorf("override: %s", st)
	}
	if len(s.Waiting) == 0 {
		t.Error("override should keep other keys")
	}
}

// waiting_for comes from the prompt line, not the footer under it.
func TestWaitingFor(t *testing.T) {
	cp := Load(t.TempDir())["copilot"]
	for f, want := range map[string]string{
		"waiting-question.txt":             "Copilot needs information. Should agents be allowed to message each other without approval?",
		"waiting-question-idle-footer.txt": "Copilot needs information. Which branch should I merge into?",
		"waiting-approval.txt":             "Do you want to run this command?",
		"error-dead.txt":                   "Pane is dead (status 0, Thu Nov  5 06:58:12 2026)",
	} {
		b, _ := os.ReadFile(filepath.Join("testdata/copilot", f))
		if _, got, _ := cp.Match(string(b)); got != want {
			t.Errorf("%s: got %q want %q", f, got, want)
		}
	}
}
