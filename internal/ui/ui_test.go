package ui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest"
	zone "github.com/lrstanley/bubblezone"

	"github.com/sturlabragason/fleet/internal/board"
)

type fakeBE struct {
	shown, hidden []string
	noWall        bool
	ctx           [2]string
	ctxSets       [][2]string
	resets        int
}

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
func (f *fakeBE) OnWall() map[string]bool  { return map[string]bool{"alpha": true} }
func (f *fakeBE) Show(n string) error      { f.shown = append(f.shown, n); return nil }
func (f *fakeBE) Hide(n string) error      { f.hidden = append(f.hidden, n); return nil }
func (f *fakeBE) Preset(string) error      { return nil }
func (f *fakeBE) WallEnabled() bool        { return !f.noWall }
func (f *fakeBE) SupCtx() (string, string) { return f.ctx[0], f.ctx[1] }
func (f *fakeBE) SetSupCtx(l, c string) error {
	f.ctxSets = append(f.ctxSets, [2]string{l, c})
	return nil
}
func (f *fakeBE) ResetSup() error { f.resets++; return nil }

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

func TestHelpOverlay(t *testing.T) {
	zone.NewGlobal()
	m := New(&fakeBE{}, "")
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	as, _ := (&fakeBE{}).Refresh()
	m.setAgents(as)
	if v := m.View(); !strings.Contains(v, "←↑↓→ select · ⏎ show on wall · x take off wall · Tab next waiting · ? all keys") {
		t.Errorf("header hint:\n%s", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if h := m.header(); !strings.Contains(h, "? all keys") || ansi.StringWidth(h) > 100 {
		t.Errorf("narrow header must keep \"? all keys\": %q", h)
	}
	press := func(r string) tea.Cmd {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)})
		return cmd
	}
	press("?")
	v := m.View()
	if !strings.Contains(v, "hide this pane (the agent keeps running)") || strings.Contains(v, "╭") {
		t.Fatalf("overlay should replace tiles:\n%s", v)
	}
	if cmd := press("q"); cmd != nil {
		t.Fatal("q with the overlay open must not quit")
	}
	if m.help || strings.Contains(m.View(), "hide this pane (the agent keeps running)") {
		t.Fatal("q should close the overlay")
	}
	press("?")
	press("?")
	if m.help {
		t.Fatal("? should toggle the overlay closed")
	}
	press("?")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.help {
		t.Fatal("esc should close the overlay")
	}
	if cmd := press("q"); cmd == nil {
		t.Fatal("q without the overlay quits")
	}
}

// TestHelpOverlayShortPane: the overview pane is ~14 rows, shorter than the
// help list. The overlay switches to two columns and never loses the top.
func TestHelpOverlayShortPane(t *testing.T) {
	zone.NewGlobal()
	m := New(&fakeBE{}, "")
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 14})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	v := ansi.Strip(m.View())
	lines := strings.Split(v, "\n")
	if len(lines) > 14 {
		t.Errorf("view is %d lines, pane is 14", len(lines))
	}
	// columns are half width, so the Alt-z description may wrap after "again to"
	for _, want := range []string{"Panes", "Supervisor", "Alt-z         zoom this pane; press again", "Alt-m         wall: one main pane"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q:\n%s", want, v)
		}
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 120 {
			t.Errorf("line wider than 120: %q", l)
		}
	}
}

func TestSupCtxSetOnlyWhenChanged(t *testing.T) {
	be := &fakeBE{}
	m := New(be, "")
	tick := func() {
		msg := m.poll()().(agentsMsg)
		if cmd := m.syncSupCtx(msg.supCtx); cmd != nil {
			cmd()
		}
	}
	tick() // unknown: unset once
	tick()
	be.ctx = [2]string{"34%", "#a6e3a1"}
	tick()
	tick()
	be.ctx = [2]string{"", ""}
	tick()
	want := [][2]string{{"", ""}, {"34%", "#a6e3a1"}, {"", ""}}
	if len(be.ctxSets) != len(want) {
		t.Fatalf("sets %v", be.ctxSets)
	}
	for i := range want {
		if be.ctxSets[i] != want[i] {
			t.Errorf("set %d: %v", i, be.ctxSets[i])
		}
	}
}

func TestResetKey(t *testing.T) {
	be := &fakeBE{noWall: true}
	m := New(be, "")
	if cmd := m.key("R"); cmd != nil || m.msg != "R needs fleet up" {
		t.Fatalf("no wall: msg=%q", m.msg)
	}
	be.noWall = false
	m.msg = ""
	cmd := m.key("R")
	if cmd == nil {
		t.Fatal("R must ask tmux to confirm")
	}
	cmd()
	if be.resets != 1 || m.msg != "" {
		t.Errorf("resets=%d msg=%q", be.resets, m.msg)
	}
}
