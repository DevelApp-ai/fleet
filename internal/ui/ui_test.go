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
	"github.com/sturlabragason/fleet/internal/layout"
	"github.com/sturlabragason/fleet/internal/team"
	"github.com/sturlabragason/fleet/internal/wall"
)

type fakeBE struct {
	shown, hidden []string
	applied       int
	refuse        bool
	noWall        bool
	ctx           [2]string
	ctxSets       [][2]string
	resets        int
	picks         int
	changes       int
	teams         map[string]team.Info
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
func (f *fakeBE) OnWall() map[string]bool     { return map[string]bool{"alpha": true} }
func (f *fakeBE) Teams() map[string]team.Info { return f.teams }
func (f *fakeBE) Show(n string) (bool, error) {
	f.shown = append(f.shown, n)
	return !f.refuse, nil
}
func (f *fakeBE) Hide(n string) error      { f.hidden = append(f.hidden, n); return nil }
func (f *fakeBE) Preset(string) error      { return nil }
func (f *fakeBE) WallEnabled() bool        { return !f.noWall }
func (f *fakeBE) ApplyWall() error         { f.applied++; return nil }
func (f *fakeBE) SupCtx() (string, string) { return f.ctx[0], f.ctx[1] }
func (f *fakeBE) SetSupCtx(l, c string) error {
	f.ctxSets = append(f.ctxSets, [2]string{l, c})
	return nil
}
func (f *fakeBE) ResetSup() error { f.resets++; return nil }
func (f *fakeBE) Changes() error  { f.changes++; return nil }
func (f *fakeBE) PickProfile() error {
	f.picks++
	return nil
}

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
	// 5 cards wrap 3+2 at 43 columns: the second (bravo) spans columns 43..85, rows 1..6
	_, cmd := m.Update(tea.MouseMsg{X: 50, Y: 3, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
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

// Finding 1: nothing re-tiles the wall when a pane's status flips, so the
// refresh loop must apply the layout after every poll (cheap: cached).
func TestPollAppliesTheWall(t *testing.T) {
	be := &fakeBE{}
	m := New(be, "")
	msg := m.poll()()
	if _, ok := msg.(agentsMsg); !ok {
		t.Fatalf("poll returned %T", msg)
	}
	if be.applied != 1 {
		t.Errorf("wall applied %d times per poll, want 1", be.applied)
	}
}

// Finding 4: a refused Show (wall full, nothing evictable) must not be
// reported to the user as "on wall".
func TestShowRefusalIsReported(t *testing.T) {
	be := &fakeBE{refuse: true}
	m := New(be, "")
	m.sel = "alpha"
	got, ok := m.show()().(infoMsg)
	if !ok || string(got) != wall.FullMsg {
		t.Errorf("got %q, want %q", got, wall.FullMsg)
	}
	be.refuse = false
	if got := m.show()().(infoMsg); string(got) != "alpha on wall" {
		t.Errorf("got %q, want the success message", got)
	}
}

func TestHelpOverlay(t *testing.T) {
	zone.NewGlobal()
	m := New(&fakeBE{}, "")
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	as, _ := (&fakeBE{}).Refresh()
	m.setAgents(as)
	if v := m.View(); !strings.Contains(v, "←↑↓→ select · ⏎ show on wall · x take off wall · Tab next waiting · g what's new · ? all keys") {
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
	if !strings.Contains(v, "hide this pane (the agent keeps") || strings.Contains(v, "╭") {
		t.Fatalf("overlay should replace tiles:\n%s", v)
	}
	if cmd := press("q"); cmd != nil {
		t.Fatal("q with the overlay open must not quit")
	}
	if m.help || strings.Contains(m.View(), "hide this pane (the agent keeps") {
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
	for _, want := range []string{"Panes", "Admiral", "Alt-z         zoom this pane; press again", "Alt-m         wall: one main pane"} {
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

func TestChangesKey(t *testing.T) {
	be := &fakeBE{noWall: true}
	m := New(be, "")
	if cmd := m.key("g"); cmd != nil || m.msg != "g needs fleet up" {
		t.Fatalf("no wall: msg=%q", m.msg)
	}
	be.noWall = false
	m.msg = ""
	cmd := m.key("g")
	if cmd == nil {
		t.Fatal("g must open the what's new popup")
	}
	cmd()
	if be.changes != 1 || m.msg != "" {
		t.Errorf("changes=%d msg=%q", be.changes, m.msg)
	}
}

func TestTileShowsReportUnlessWorking(t *testing.T) {
	a := &board.Agent{Name: "x", Status: "idle", LastLine: "screen line", Report: &board.Report{State: "done", Summary: "merged feat/x"}}
	if got := ansi.Strip(line3(a, 40)); got != "✓ done: merged feat/x" {
		t.Fatalf("idle with report: %q", got)
	}
	a.Report.State, a.Status = "blocked", "waiting"
	if got := ansi.Strip(line3(a, 40)); got != "✗ blocked: merged feat/x" {
		t.Fatalf("waiting with report: %q", got)
	}
	a.Status = "working"
	if got := ansi.Strip(line3(a, 40)); got != "screen line" {
		t.Fatalf("working: %q", got)
	}
	a.Report, a.Status = nil, "idle"
	if got := ansi.Strip(line3(a, 40)); got != "screen line" {
		t.Fatalf("no report: %q", got)
	}
}

func TestTeamMark(t *testing.T) {
	zone.NewGlobal()
	now := time.Now()
	render := func(teams map[string]team.Info, a *board.Agent) (tile, chip string) {
		m := New(&fakeBE{}, "")
		m.now = func() time.Time { return now }
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 9})
		m.teams = teams
		m.onWall = map[string]bool{a.Name: true}
		return m.tile(a, m.cfg.TileW), m.chips([]*board.Agent{a}, true)
	}
	a := &board.Agent{Name: "a-rather-long-agent-name", Profile: "copilot", Status: "idle", StatusSince: now.Add(-3 * time.Minute)}
	tile, chip := render(map[string]team.Info{a.Name: {Team: "frontend-squad", Color: "#89b4fa"}}, a)
	plainTile := ansi.Strip(tile)
	for _, want := range []string{"◆ ▣", "○ a-rather-long… ◆ ▣", "frontend-… · idle 3m"} {
		if !strings.Contains(plainTile, want) {
			t.Errorf("tile missing %q:\n%s", want, plainTile)
		}
	}
	if !strings.Contains(ansi.Strip(chip), "○ a-rather-long-agent-name ◆") {
		t.Errorf("chip %q", ansi.Strip(chip))
	}
	// Line 1 never wraps: the tile keeps its height.
	if got := strings.Count(tile, "\n"); got != layout.Default.TileH-1 {
		t.Errorf("tile has %d lines:\n%s", got+1, plainTile)
	}
	// No team: identical to an empty teams map (and to nil).
	t1, c1 := render(nil, a)
	t2, c2 := render(map[string]team.Info{"other": {Team: "x", Color: "#f5c2e7"}}, a)
	if t1 != t2 || c1 != c2 || strings.Contains(t1, "◆") || strings.Contains(c1, "◆") {
		t.Error("an agent without a team must render as before")
	}
}

func TestPickProfileKey(t *testing.T) {
	be := &fakeBE{noWall: true}
	m := New(be, "")
	if cmd := m.key("P"); cmd != nil || m.msg != "P needs fleet up" {
		t.Fatalf("no wall: msg=%q", m.msg)
	}
	be.noWall = false
	m.msg = ""
	cmd := m.key("P")
	if cmd == nil {
		t.Fatal("P must open the profile picker")
	}
	cmd()
	if be.picks != 1 || m.msg != "" {
		t.Errorf("picks=%d msg=%q", be.picks, m.msg)
	}
}
