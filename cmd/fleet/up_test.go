package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/keys"
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

// TestTmuxBindingsMatchRegistry: every root-table Alt key and every wall
// chord key fleet up binds is in the keys registry, and vice versa.
func TestTmuxBindingsMatchRegistry(t *testing.T) {
	bound := map[string]bool{}
	var statusRight, popup []string
	for _, c := range tmuxCommands(&fleet.Fleet{Exe: "/opt/my fleet/fleet"}) {
		if len(c) > 3 && c[0] == "bind-key" && c[1] == "-n" && strings.HasPrefix(c[2], "M-") {
			bound[c[2]] = true
			if c[2] == keys.HelpTmuxKey {
				popup = c
			}
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
