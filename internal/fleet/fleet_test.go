package fleet

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/board"
	"github.com/sturlabragason/fleet/internal/tmux"
)

func TestSpawnLsSendClose(t *testing.T) {
	home := t.TempDir()
	tc := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { tc.KillServer() })
	// A profile whose pane prints a y/n prompt, using cat to echo input.
	os.WriteFile(filepath.Join(home, "tools.toml"), []byte(`[yn]
command = "sh"
args = ["-c", "printf 'Approve? [y/n] '; cat"]
patterns = "fake"
`), 0o644)
	// Keep the user's real ~/.config/fleet/config.toml out of the test.
	t.Setenv("FLEET_CONFIG", filepath.Join(home, "no-config.toml"))
	f, err := New(tc, home, "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Spawn("a1", "yn", home, ""); err != nil {
		t.Fatal(err)
	}
	var st string
	for i := 0; i < 40; i++ {
		as, _ := f.Refresh()
		if len(as) == 1 && as[0].Status == "waiting" {
			st = as[0].Status
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if st != "waiting" {
		t.Fatalf("status never waiting")
	}
	if err := f.Send("a1", "hello"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if out, _ := f.Peek("a1", 0); !strings.Contains(out, "hello") {
		t.Errorf("peek: %q", out)
	}
	if err := f.Close("a1"); err != nil {
		t.Fatal(err)
	}
	if len(f.Store.Load().Agents) != 0 {
		t.Error("close left board entry")
	}
}

// TestSpawnPromptWaitsForReady covers the bug ADR-13 fixes: the pane only
// accepts input well after the old fixed 1500 ms sleep, so a timed paste lost
// the prompt. Spawn must wait for the pane to look ready instead.
//
// Note on the assertion: merely finding the prompt somewhere in the pane does
// NOT guard the bug. tmux buffers input written before the program reaches the
// terminal, so an early paste is still echoed by the pty — it just lands as a
// bare line *above* the input box, and the program then reads it back out of
// the buffer. The test therefore pins *where* the prompt first appears:
// directly after the "> " marker, which only happens when the paste waited.
func TestSpawnPromptWaitsForReady(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLEET_CONFIG", filepath.Join(home, "no-config.toml"))
	tc := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { tc.KillServer() })
	// No prompt_args, and the input box appears only after 2.5s.
	os.WriteFile(filepath.Join(home, "tools.toml"), []byte(`[late]
command = "sh"
args = ["-c", "sleep 2.5; printf '> '; cat"]
patterns = "fake"
`), 0o644)
	f, err := New(tc, home, "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Spawn("late1", "late", home, "hello fleet"); err != nil {
		t.Fatal(err)
	}
	out, _ := f.Peek("late1", 0)
	i := strings.Index(out, "hello fleet")
	if i < 0 {
		t.Fatalf("prompt not delivered; pane:\n%s", out)
	}
	if i < 2 || out[i-2:i] != "> " {
		t.Fatalf("prompt did not land at the input box (pasted before ready); pane:\n%s", out)
	}
	if err := f.Close("late1"); err != nil {
		t.Fatal(err)
	}
}

// TestSpawnFakeAgentEndToEnd is the end-to-end check from AGENTS.md: the real
// `fake` profile (which deliberately has no prompt_args, so it exercises the
// ready-then-paste path) on a throwaway tmux server. Needs `make build`.
//
// As in TestSpawnPromptWaitsForReady, "the prompt is somewhere in the pane"
// proves nothing: tmux buffers an early paste and the pty echoes it anyway.
// The guard is that the echoed prompt appears *after* the fake agent's ready
// banner, which can only happen if the paste waited for the pane to be ready.
func TestSpawnFakeAgentEndToEnd(t *testing.T) {
	exe, err := filepath.Abs("../../bin/fleet")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Skip("bin/fleet not built; run make build")
	}
	home := t.TempDir()
	t.Setenv("FLEET_CONFIG", filepath.Join(home, "no-config.toml"))
	tc := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { tc.KillServer() })
	f, err := New(tc, home, exe)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Spawn("a1", "fake", home, "hello fleet"); err != nil {
		t.Fatal(err)
	}
	var out string
	for i := 0; i < 30; i++ {
		out, _ = f.Peek("a1", 0)
		if strings.Contains(out, "queued: hello fleet") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(out, "queued: hello fleet") {
		t.Fatalf("fake agent never received the prompt; pane:\n%s", out)
	}
	banner := strings.Index(out, "ready (tool: fake)")
	first := strings.Index(out, "hello fleet")
	if banner < 0 || first < banner {
		t.Fatalf("prompt was typed before the agent was ready (banner=%d prompt=%d); pane:\n%s", banner, first, out)
	}
}

// A pane whose process exited is error, and the pane tag follows the board
// even when it drifted (e.g. left at "working" by an older fleet).
func TestDeadPaneIsErrorAndTagResyncs(t *testing.T) {
	home := t.TempDir()
	tc := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { tc.KillServer() })
	os.WriteFile(filepath.Join(home, "tools.toml"), []byte(`[short]
command = "sh"
args = ["-c", "echo bye; sleep 0.5"]
patterns = "fake"
`), 0o644)
	t.Setenv("FLEET_CONFIG", filepath.Join(home, "no-config.toml"))
	f, err := New(tc, home, "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Spawn("d1", "short", home, ""); err != nil {
		t.Fatal(err)
	}
	paneStatus := func() string {
		ps, _ := tc.ListPanes("")
		for _, p := range ps {
			if p.Agent == "d1" {
				return p.Status
			}
		}
		return ""
	}
	var as []*board.Agent
	for i := 0; i < 50; i++ {
		as, _ = f.Refresh()
		if len(as) == 1 && as[0].Status == "error" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(as) != 1 || as[0].Status != "error" {
		t.Fatalf("dead pane not error: %+v", as)
	}
	if got := paneStatus(); got != "error" {
		t.Fatalf("pane tag %q, want error", got)
	}
	ps, _ := tc.ListPanes("")
	for _, p := range ps {
		if p.Agent == "d1" {
			tc.SetPaneOption(p.ID, "@fleet_status", "working")
		}
	}
	f.Refresh()
	if got := paneStatus(); got != "error" {
		t.Errorf("pane tag not re-synced: %q", got)
	}
}
