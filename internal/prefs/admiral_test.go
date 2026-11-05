package prefs

import (
	"path/filepath"
	"testing"
)

func TestAdmiralProfile(t *testing.T) {
	var nilp *Prefs
	if got := nilp.AdmiralProfile(); got != "" {
		t.Errorf("nil prefs: %q", got)
	}
	t.Setenv("FLEET_CONFIG", filepath.Join(t.TempDir(), "nope.toml"))
	if p, _ := Load(); p.AdmiralProfile() != "" {
		t.Errorf("missing file: %q", p.AdmiralProfile())
	}
	write(t, "[admiral]\nprofile = \"claude-personal\"\n")
	p, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.AdmiralProfile(); got != "claude-personal" {
		t.Errorf("got %q", got)
	}
}
