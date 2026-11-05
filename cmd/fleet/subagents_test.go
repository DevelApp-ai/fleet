package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/board"
	"github.com/sturlabragason/fleet/internal/status"
	"github.com/sturlabragason/fleet/internal/team"
)

// fleet ls --json carries the board's subagents, with and without teams.
func TestLsJSONSubagents(t *testing.T) {
	now := time.Now()
	as := []*board.Agent{
		{Name: "a1", Profile: "copilot", Status: "working", StatusSince: now, Subagents: []status.Subagent{{ID: "t1", Name: "fleet-builder", State: status.SubRunning, Started: now}}},
		{Name: "a2", Profile: "fake", Status: "idle", StatusSince: now},
	}
	for _, teams := range []map[string]team.Info{nil, {"a1": {Team: "alpha", Color: "#89b4fa"}}} {
		var js bytes.Buffer
		writeLs(&js, as, teams, true)
		if strings.Count(js.String(), `"subagents": [`) != 1 || !strings.Contains(js.String(), `"name": "fleet-builder"`) || !strings.Contains(js.String(), `"state": "running"`) {
			t.Errorf("json:\n%s", js.String())
		}
	}
}
