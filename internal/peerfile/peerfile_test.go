package peerfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sturlabragason/fleet/internal/tools"
)

// block is the peer server fleet writes for agent a1. tmpHome points the
// lock directory at a temp home so a test run inside an agent pane cannot
// touch the user's real one.
func block(exe, name string) string { return tools.PeerMCPVibe(exe, name) }

func tmpHome(t *testing.T) {
	t.Helper()
	t.Setenv("FLEET_HOME", filepath.Join(t.TempDir(), ".fleet"))
}

// TestAddCreatesFile: a missing file gets the server, its parent made.
func TestAddCreatesFile(t *testing.T) {
	tmpHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, ".vibe", "config.toml")
	changed, err := Add(path, block("/bin/fleet", "a1"))
	if err != nil || !changed {
		t.Fatalf("Add = %v, %v; want true, nil", changed, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != block("/bin/fleet", "a1") {
		t.Fatalf("file = %q; want the block verbatim", got)
	}
}

// TestAddMergesUserFile: an authored file keeps every other byte.
func TestAddMergesUserFile(t *testing.T) {
	tmpHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	user := "# my config\nmodel = \"mistral\"\n\n[[mcp_servers]]\nname = 'own'\ncommand = \"uvx\"\nargs = [\"own\"]\n"
	if err := os.WriteFile(path, []byte(user), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(path, block("/bin/fleet", "a1")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := user + block("/bin/fleet", "a1")
	if string(got) != want {
		t.Fatalf("file = %q; want user content untouched with the block appended", got)
	}
	if _, err := Add(path, block("/bin/fleet", "a1")); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != want {
		t.Fatalf("second Add rewrote the file: %q", got)
	}
}

// TestAddReplacesInPlace: an older fleet block is swapped for the current
// one, without touching anything around it.
func TestAddReplacesInPlace(t *testing.T) {
	tmpHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	pre := "# top\n"
	post := "\n[[mcp_servers]]\nname = \"other\"\ncommand = \"other\"\n"
	old := pre + block("/old/fleet", "a1") + post
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := Add(path, block("/bin/fleet", "a1"))
	if err != nil || !changed {
		t.Fatalf("Add = %v, %v; want true, nil", changed, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := pre + block("/bin/fleet", "a1") + post; string(got) != want {
		t.Fatalf("file = %q; want %q", got, want)
	}
}

// TestRemoveKeepsUserFile: close takes only fleet's table out; a second
// agent's table and authored content stay.
func TestRemoveKeepsUserFile(t *testing.T) {
	tmpHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	user := "# my config\nmodel = \"mistral\"\n\n"
	other := block("/bin/fleet", "a2")
	if err := os.WriteFile(path, []byte(user+block("/bin/fleet", "a1")+other), 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, err := Remove(path, tools.PeerMCPName("a1")); err != nil || !changed {
		t.Fatalf("Remove = %v, %v; want true, nil", changed, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != user+other {
		t.Fatalf("file = %q; want %q", got, user+other)
	}
	if changed, err := Remove(path, tools.PeerMCPName("a1")); err != nil || changed {
		t.Fatalf("second Remove = %v, %v; want false, nil", changed, err)
	}
}

// TestRemoveDeletesFleetFile: a file fleet wrote disappears on close; a
// missing file is a no-op, not an error.
func TestRemoveDeletesFleetFile(t *testing.T) {
	tmpHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, ".vibe", "config.toml")
	if _, err := Add(path, block("/bin/fleet", "a1")); err != nil {
		t.Fatal(err)
	}
	changed, err := Remove(path, tools.PeerMCPName("a1"))
	if err != nil || !changed {
		t.Fatalf("Remove = %v, %v; want true, nil", changed, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
	if changed, err := Remove(path, tools.PeerMCPName("a1")); err != nil || changed {
		t.Fatalf("Remove of a missing file = %v, %v; want false, nil", changed, err)
	}
}

// TestName: quoted, commented and absent name lines.
func TestName(t *testing.T) {
	for _, tt := range []struct {
		block, want string
	}{
		{"[[mcp_servers]]\nname = \"x\"\n", "x"},
		{"[[mcp_servers]]\nname = 'x' # mine\n", "x"},
		{"[[mcp_servers]]\nname = \"a b\"\n", "a b"},
		{"[[mcp_servers]]\nnamed = \"no\"\n", ""},
		{"[[mcp_servers]]\n", ""},
	} {
		if got := Name(tt.block); got != tt.want {
			t.Errorf("Name(%q) = %q; want %q", tt.block, got, tt.want)
		}
	}
}
