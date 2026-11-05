package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"

	"github.com/sturlabragason/fleet/internal/board"
	"github.com/sturlabragason/fleet/internal/layout"
	"github.com/sturlabragason/fleet/internal/status"
)

func subs(states ...string) []status.Subagent {
	t0 := time.Now().Add(-time.Hour)
	var l []status.Subagent
	for i, s := range states {
		l = append(l, status.Subagent{ID: fmt.Sprint(i), Name: fmt.Sprint(i), State: s, Started: t0.Add(time.Duration(i) * time.Second)})
	}
	return l
}

func TestSubagentDotColours(t *testing.T) {
	if colWorkingDim == colIdle || colWorkingDim == colWorking {
		t.Fatal("the dim running colour must differ from grey (done) and green")
	}
	r, d, f := status.SubRunning, status.SubDone, status.SubFailed
	l := subs(r, r, r, d, f)
	lit := map[int]int{}
	for frame := 0; frame < 9; frame++ {
		cols, hidden := subagentDots(l, 20, frame)
		if len(cols) != 5 || hidden != 0 {
			t.Fatalf("frame %d: %d dots, %d hidden", frame, len(cols), hidden)
		}
		if cols[3] != colIdle || cols[4] != colError {
			t.Errorf("frame %d: done %v failed %v", frame, cols[3], cols[4])
		}
		n := 0
		for i, c := range cols[:3] {
			if c != colWorking && c != colWorkingDim {
				t.Errorf("frame %d dot %d: %v", frame, i, c)
			}
			if c == colWorking {
				n++
				lit[i]++
			}
		}
		if n != 1 {
			t.Errorf("frame %d: %d lit", frame, n)
		}
	}
	if len(lit) != 3 {
		t.Errorf("the wave must reach every running dot: %v", lit)
	}
	// A single running dot still pulses.
	seen := map[lipgloss.Color]bool{}
	for frame := 0; frame < 9; frame++ {
		cols, _ := subagentDots(subs(r), 20, frame)
		seen[cols[0]] = true
	}
	if !seen[colWorking] || !seen[colWorkingDim] {
		t.Errorf("one running dot never pulses: %v", seen)
	}
}

func TestSubagentRowOverflow(t *testing.T) {
	l := subs(strings.Split(strings.Repeat("done ", 12), " ")[:12]...)
	l[11].State = status.SubRunning
	for frame := 0; frame < 9; frame++ {
		row := ansi.Strip(subagentRow(l, 20, frame))
		// 12 dots need 23 columns: the newest 9 fit, then "+3".
		if row != "• • • • • • • • • +3" {
			t.Errorf("frame %d: %q", frame, row)
		}
	}
	cols, hidden := subagentDots(l, 20, 0)
	if hidden != 3 || cols[len(cols)-1] == colIdle {
		t.Errorf("the newest (running) dot must be shown: hidden %d", hidden)
	}
	if subagentRow(nil, 20, 0) != "" {
		t.Error("row without subagents")
	}
}

func TestTileSubagentRow(t *testing.T) {
	zone.NewGlobal()
	now := time.Now()
	m := New(&fakeBE{}, "")
	m.now = func() time.Time { return now }
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 9})
	r, d, f := status.SubRunning, status.SubDone, status.SubFailed
	a := &board.Agent{Name: "orch", Profile: "copilot", Status: "working", LastLine: "running go test", StatusSince: now.Add(-time.Minute)}
	plain := ansi.Strip(m.tile(a, m.cfg.TileW))
	if strings.Contains(plain, "•") {
		t.Errorf("dots without subagents:\n%s", plain)
	}
	a.Subagents = subs(r, r, r, d, f)
	var row string
	for frame := 0; frame < 9; frame++ {
		m.frame = frame
		tile := m.tile(a, m.cfg.TileW)
		lines := strings.Split(ansi.Strip(tile), "\n")
		if len(lines) != layout.Default.TileH {
			t.Fatalf("tile has %d lines:\n%s", len(lines), strings.Join(lines, "\n"))
		}
		if !strings.Contains(lines[4], "• • • • •") {
			t.Errorf("frame %d: line 4 %q", frame, lines[4])
		}
		if frame == 0 {
			row = lines[4]
			t.Logf("tile:\n%s", strings.Join(lines, "\n"))
		} else if lines[4] != row || ansi.StringWidth(lines[4]) != ansi.StringWidth(lines[0]) {
			t.Errorf("frame %d: row %q changed (was %q)", frame, lines[4], row)
		}
	}
}
