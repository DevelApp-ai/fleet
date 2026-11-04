package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

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
