package ui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	zone "github.com/lrstanley/bubblezone"

	"github.com/sturlabragason/fleet/internal/board"
)

type fakeBE struct{ shown, hidden []string }

func (f *fakeBE) Refresh() ([]*board.Agent, error) {
	t0 := time.Now().Add(-time.Minute)
	return []*board.Agent{
		{Name: "alpha", Profile: "fake", Status: "working", LastLine: "thinking", StatusSince: t0},
		{Name: "bravo", Profile: "fake", Status: "waiting", LastLine: "Approve? [y/n]", StatusSince: t0},
		{Name: "charlie", Profile: "claude", Status: "error", LastLine: "✖ Error", StatusSince: t0},
		{Name: "delta", Profile: "fake", Status: "waiting", LastLine: "Approve? [y/n]", StatusSince: t0.Add(-time.Hour)},
		{Name: "echo", Profile: "fake", Status: "idle", StatusSince: t0},
	}, nil
}
func (f *fakeBE) OnWall() map[string]bool { return map[string]bool{"alpha": true} }
func (f *fakeBE) Show(n string) error     { f.shown = append(f.shown, n); return nil }
func (f *fakeBE) Hide(n string) error     { f.hidden = append(f.hidden, n); return nil }
func (f *fakeBE) Preset(string) error     { return nil }
func (f *fakeBE) WallEnabled() bool       { return true }

func TestOrderAndKeys(t *testing.T) {
	zone.NewGlobal()
	be := &fakeBE{}
	m := New(be, "")
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(130, 14))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return strings.Contains(string(b), "charlie") },
		teatest.WithDuration(3*time.Second))
	// attention order: delta (longest wait), bravo, charlie, alpha, echo
	var names []string
	for _, a := range m.agents {
		names = append(names, a.Name)
	}
	if got := strings.Join(names, ","); got != "delta,bravo,charlie,alpha,echo" {
		t.Errorf("order %s", got)
	}
	tm.Send(tea.KeyMsg{Type: tea.KeyTab})   // next waiting after delta -> bravo
	tm.Send(tea.KeyMsg{Type: tea.KeyRight}) // -> charlie
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
	if len(be.shown) != 1 || be.shown[0] != "charlie" {
		t.Errorf("shown %v", be.shown)
	}
	out, _ := io.ReadAll(tm.FinalOutput(t))
	_ = out
}

func TestViewTilesThenChips(t *testing.T) {
	zone.NewGlobal()
	m := New(&fakeBE{}, "")
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 9}) // 2 cols x 1 row of tiles
	as, _ := (&fakeBE{}).Refresh()
	m.setAgents(as)
	v := m.View()
	for _, want := range []string{"delta", "bravo", "charlie", "alpha", "echo", "5 agents"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	if strings.Count(v, "╭")+strings.Count(v, "┏") != 2 {
		t.Errorf("want 2 tiles then chips:\n%s", v)
	}
}

func TestMouseClickSelectsAndShows(t *testing.T) {
	zone.NewGlobal()
	be := &fakeBE{}
	m := New(be, "")
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 14})
	as, _ := be.Refresh()
	m.setAgents(as)
	m.View()
	time.Sleep(50 * time.Millisecond) // zones register asynchronously
	// second tile (bravo) spans columns 26..51, rows 1..6
	_, cmd := m.Update(tea.MouseMsg{X: 30, Y: 3, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if m.sel != "bravo" || cmd == nil {
		t.Fatalf("sel=%s cmd=%v", m.sel, cmd)
	}
	cmd()
	if len(be.shown) != 1 || be.shown[0] != "bravo" {
		t.Errorf("shown %v", be.shown)
	}
}

func TestXHidesOnlyWallTiles(t *testing.T) {
	be := &fakeBE{}
	m := New(be, "")
	as, _ := be.Refresh()
	m.setAgents(as)
	m.onWall = be.OnWall()
	m.sel = "bravo"
	if cmd := m.key("x"); cmd != nil {
		t.Fatal("bravo is not on the wall")
	}
	m.sel = "alpha"
	m.key("x")()
	if len(be.hidden) != 1 || be.hidden[0] != "alpha" {
		t.Errorf("hidden %v", be.hidden)
	}
}
