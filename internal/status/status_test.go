package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each testdata/<set>/<state>.txt must classify as <state>.
func TestCaptures(t *testing.T) {
	sets := Load(t.TempDir())
	files, _ := filepath.Glob("testdata/*/*.txt")
	if len(files) == 0 {
		t.Fatal("no testdata")
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		set := filepath.Base(filepath.Dir(f))
		want := Status(strings.TrimSuffix(filepath.Base(f), ".txt"))
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
