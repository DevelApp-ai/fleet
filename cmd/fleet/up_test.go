package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/keys"
	"github.com/sturlabragason/fleet/internal/tmux"
	"github.com/sturlabragason/fleet/internal/tools"
)

func TestSupervisorMCPArgs(t *testing.T) {
	got := supervisorMCPArgs(tools.Profile{Patterns: "copilot"})
	want := []string{"--no-custom-instructions", "--allow-all-mcp-server-instructions", "--available-tools", "fleet", "--allow-tool", "fleet", "--disable-builtin-mcps"}
	if !reflect.DeepEqual(got[:len(want)], want) {
		t.Fatalf("copilot: %v", got)
	}
	if got := supervisorMCPArgs(tools.Profile{Patterns: "claude"}); got != nil {
		t.Fatalf("claude should get no copilot flags: %v", got)
	}
}

func TestDisableUserMCPArgs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mcp-config.json")
	os.WriteFile(p, []byte(`{"mcpServers":{"zeta":{},"fleet":{},"alpha":{}}}`), 0o600)
	want := []string{"--disable-mcp-server", "alpha", "--disable-mcp-server", "zeta"}
	if got := disableUserMCPArgs(p); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if got := disableUserMCPArgs(filepath.Join(t.TempDir(), "missing.json")); got != nil {
		t.Fatalf("missing file: %v", got)
	}
}

// Finding 1: focusing a wall pane is what makes emphasis move to it, so the
// hooks must include pane-focus-in next to client-resized/client-attached.
func TestConfigureSetsPaneFocusHook(t *testing.T) {
	c := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { c.KillServer() })
	if _, err := c.Run("new-session", "-d", "-s", "f", "-n", "main", "cat"); err != nil {
		t.Skip("tmux unavailable")
	}
	configure(&fleet.Fleet{T: c, Exe: "/usr/bin/fleet"})
	// pane-focus-in is a window-scoped hook, so it only shows with -gw.
	// client-resized/client-attached force a real apply; pane-focus-in fires on
	// every pane switch, so it takes the cached path instead.
	for _, h := range []struct{ scope, name, want string }{
		{"-g", "client-resized", "wall apply"},
		{"-g", "client-attached", "wall apply"},
		{"-gw", "pane-focus-in", "wall refresh"},
	} {
		hooks, err := c.Run("show-hooks", h.scope)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, line := range strings.Split(hooks, "\n") {
			if strings.HasPrefix(line, h.name+"[") {
				found = true
				if !strings.Contains(line, h.want) {
					t.Errorf("%s must run `%s`, got %q", h.name, h.want, line)
				}
			}
		}
		if !found {
			t.Errorf("hook %s is not set", h.name)
		}
	}
}

// Outside the fleet server there is no wall, so the refresh-driven apply must
// be a silent no-op rather than an error (and a log line) every second.
func TestApplyWallDisabledIsANoop(t *testing.T) {
	b := &backend{enabled: false} // no wall client at all: must not be touched
	if err := b.ApplyWall(); err != nil {
		t.Errorf("disabled wall returned %v", err)
	}
}

// TestTmuxBindingsMatchRegistry: every root-table Alt key and every wall
// chord key fleet up binds is in the keys registry, and vice versa.
func TestTmuxBindingsMatchRegistry(t *testing.T) {
	bound, chord := map[string]bool{}, map[string]bool{}
	var statusRight, popup []string
	for _, c := range tmuxCommands(&fleet.Fleet{Exe: "/opt/my fleet/fleet"}) {
		if len(c) > 3 && c[0] == "bind-key" && c[1] == "-n" && strings.HasPrefix(c[2], "M-") {
			bound[c[2]] = true
			if c[2] == keys.HelpTmuxKey {
				popup = c
			}
		}
		if len(c) > 4 && c[0] == "bind-key" && c[1] == "-T" && c[2] == keys.ChordTable {
			chord[c[3]] = true
		}
		if len(c) == 4 && c[0] == "set-option" && c[2] == "status-right" {
			statusRight = c
		}
	}
	reg := keys.TmuxKeys()
	for k := range bound {
		if !reg[k] {
			t.Errorf("tmux binds %s but the keys registry doesn't list it", k)
		}
	}
	for k := range reg {
		if !bound[k] {
			t.Errorf("keys registry lists %s but fleet up doesn't bind it", k)
		}
	}
	regChord := keys.ChordKeys()
	if !reflect.DeepEqual(chord, regChord) {
		t.Errorf("-T %s binds %v, keys registry claims %v", keys.ChordTable, chord, regChord)
	}
	if statusRight == nil || popup == nil {
		t.Fatal("tmuxCommands must set status-right and bind the help popup")
	}
	if !strings.Contains(statusRight[3], "Alt-? all keys") || strings.Contains(statusRight[3], "M-") {
		t.Errorf("status-right %q", statusRight[3])
	}
	if got := popup[len(popup)-1]; got != "'/opt/my fleet/fleet' keys --popup" {
		t.Errorf("popup command %q", got)
	}
}
