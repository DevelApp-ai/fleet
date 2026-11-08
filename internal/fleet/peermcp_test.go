package fleet

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sturlabragason/fleet/internal/tmux"
	"github.com/sturlabragason/fleet/internal/tools"
)

// peerFleet is a fleet whose one profile wires its peer MCP server into
// a project config file (ADR-64), with two agents in one shared dir.
func peerFleet(t *testing.T, dir string) *Fleet {
	t.Helper()
	home := t.TempDir()
	tc := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { tc.KillServer() })
	os.WriteFile(filepath.Join(home, "tools.toml"),
		[]byte("[cat]\ncommand = \"sh\"\nargs = [\"-c\", \"cat\"]\npatterns = \"fake\"\npeer_project = \".vibe/config.toml\"\n"), 0o644)
	t.Setenv("FLEET_CONFIG", filepath.Join(home, "no-config.toml"))
	f, err := New(tc, home, "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func peerFile(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".vibe", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestPeerProjectWiresOnSpawn: spawn merges the agent's server into the
// project config, one table per agent, and close takes each back out,
// the file going only when fleet wrote it.
func TestPeerProjectWiresOnSpawn(t *testing.T) {
	dir := t.TempDir()
	f := peerFleet(t, dir)
	for _, n := range []string{"a", "b"} {
		if _, err := f.Spawn(n, "cat", dir, ""); err != nil {
			t.Fatal(err)
		}
	}
	got := peerFile(t, dir)
	if !strings.Contains(got, "name = \"fleet-peer-a\"") || !strings.Contains(got, "name = \"fleet-peer-b\"") {
		t.Fatalf("no per-agent servers: %q", got)
	}
	if err := f.Close("a"); err != nil {
		t.Fatal(err)
	}
	got = peerFile(t, dir)
	if strings.Contains(got, "fleet-peer-a") || !strings.Contains(got, "fleet-peer-b") {
		t.Fatalf("close did not take only a's server out: %q", got)
	}
	if err := f.Close("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".vibe", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("fleet's own file left behind: %v", err)
	}
}

// TestPeerProjectMergesUserFile: an authored project config keeps every
// other byte; fleet's table comes and goes around it.
func TestPeerProjectMergesUserFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vibe"), 0o755); err != nil {
		t.Fatal(err)
	}
	user := "# my config\nmodel = \"mistral\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".vibe", "config.toml"), []byte(user), 0o600); err != nil {
		t.Fatal(err)
	}
	f := peerFleet(t, dir)
	if _, err := f.Spawn("a", "cat", dir, ""); err != nil {
		t.Fatal(err)
	}
	if got := peerFile(t, dir); !strings.HasPrefix(got, user) || !strings.Contains(got, tools.PeerMCPName("a")) {
		t.Fatalf("user content not preserved: %q", got)
	}
	if err := f.Close("a"); err != nil {
		t.Fatal(err)
	}
	if got := peerFile(t, dir); got != user {
		t.Fatalf("file after close = %q; want the user's %q", got, user)
	}
}
