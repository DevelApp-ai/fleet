package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/supervisor"
)

func TestAdmiralProfileTool(t *testing.T) {
	s, _, _ := memServer(t)
	var swapped []string
	s.SwapProfile = func(name string) error { swapped = append(swapped, name); return nil }

	if out, isErr := callText(t, s, "fleet_admiral_profile", map[string]any{"profile": "nope"}); !isErr || !strings.Contains(out, `no profile "nope"`) || !strings.Contains(out, "copilot") {
		t.Errorf("unknown: %q", out)
	}
	if len(swapped) != 0 {
		t.Fatalf("unknown name scheduled a swap: %v", swapped)
	}
	out, isErr := callText(t, s, "fleet_admiral_profile", map[string]any{"profile": "copilot", "summary": "user wants claude next"})
	if isErr || out != "switching admiral to copilot in a few seconds. Stop here." {
		t.Errorf("swap: %q", out)
	}
	if len(swapped) != 1 || swapped[0] != "copilot" {
		t.Errorf("SwapProfile calls: %v", swapped)
	}
	if h, _, _ := supervisor.ReadHandoff(s.F.Home); !strings.Contains(h, "user wants claude next") {
		t.Errorf("handoff %q", h)
	}
}

func TestAdmiralHeader(t *testing.T) {
	s, _, _ := memServer(t)
	if st, _ := callText(t, s, "fleet_status", nil); !strings.HasPrefix(st, "admiral: ?\n") {
		t.Errorf("no session: %q", st)
	}
	supervisor.Save(s.F.Home, supervisor.Session{SessionID: "3f2a91c0-aaaa", Profile: "claude-personal", Generation: 3, Started: time.Now()})
	const want = "admiral: claude-personal · session 3f2a91c · gen 3\n"
	if st, _ := callText(t, s, "fleet_status", nil); !strings.HasPrefix(st, want) {
		t.Errorf("status: %q", st)
	}
	br, _ := callText(t, s, "fleet_brief", nil)
	if !strings.HasPrefix(br, want) || strings.Count(br, "admiral: ") != 1 {
		t.Errorf("brief: %q", br)
	}
}
