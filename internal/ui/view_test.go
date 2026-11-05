package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"

	"github.com/sturlabragason/fleet/internal/board"
)

// overview renders n agents in a w×h pane with the last one selected.
func overview(w, h, n int) (*Model, string) {
	now := time.Now()
	m := New(&fakeBE{}, "")
	m.now = func() time.Time { return now }
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	var as []*board.Agent
	for i := 0; i < n; i++ {
		as = append(as, &board.Agent{Name: fmt.Sprintf("agent-%02d", i), Profile: "claude-copilot",
			Status: "working", LastLine: "53% (105k/200k)", StatusSince: now.Add(-12 * time.Minute)})
	}
	m.setAgents(as)
	m.sel = as[n-1].Name
	return m, ansi.Strip(m.View())
}

func cards(v string) int { return strings.Count(v, "╭") + strings.Count(v, "┏") }

func TestOverviewFitsEveryPane(t *testing.T) {
	zone.NewGlobal()
	for _, sz := range [][2]int{{150, 13}, {150, 40}, {80, 13}, {60, 9}, {200, 20}, {150, 4}} {
		for _, n := range []int{1, 5, 6, 9, 12, 20} {
			w, h := sz[0], sz[1]
			m, v := overview(w, h, n)
			lines := strings.Split(v, "\n") // as bubbletea counts: a trailing \n is a line
			if len(lines) > h {
				t.Errorf("%dx%d n=%d: %d lines", w, h, n, len(lines))
			}
			if !strings.HasPrefix(lines[0], " fleet · ") {
				t.Errorf("%dx%d n=%d: header missing: %q", w, h, n, lines[0])
			}
			for _, l := range lines {
				if ansi.StringWidth(l) > w {
					t.Errorf("%dx%d n=%d: line wider than pane: %q", w, h, n, l)
				}
			}
			g := m.grid()
			if got := cards(v); got != g.Cards {
				t.Errorf("%dx%d n=%d: %d cards, grid says %d:\n%s", w, h, n, got, g.Cards, v)
			}
			hidden := n - g.Cards
			switch {
			case hidden == 0:
				if strings.Contains(v, "hidden") {
					t.Errorf("%dx%d n=%d: all fit but says hidden:\n%s", w, h, n, v)
				}
			case g.Cards > 0:
				if !strings.Contains(v, fmt.Sprintf("+%d hidden:", hidden)) {
					t.Errorf("%dx%d n=%d: want +%d hidden:\n%s", w, h, n, hidden, v)
				}
				if !strings.Contains(v, "┃ ⠋ "+m.sel) {
					t.Errorf("%dx%d n=%d: selected %s has no card:\n%s", w, h, n, m.sel, v)
				}
			}
		}
	}
}

// The reported bug: 9 agents in a ~150-column, 13-row overview drew 5 cards
// and a line of bare chips. All 9 must get a card.
func TestOverviewNineAgentsAllCards(t *testing.T) {
	zone.NewGlobal()
	_, v := overview(150, 13, 9)
	if got := cards(v); got != 9 {
		t.Fatalf("want 9 cards, got %d:\n%s", got, v)
	}
	if strings.Contains(v, "hidden") || strings.Contains(v, "wo…") {
		t.Errorf("no overflow line and no cut status expected:\n%s", v)
	}
	for _, want := range []string{"agent-00", "agent-08", "working 12m", "53% (105k/200k)"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q:\n%s", want, v)
		}
	}
}

func TestOverviewScrollsToSelection(t *testing.T) {
	zone.NewGlobal()
	m, v := overview(80, 13, 20) // 3 cards across, one row, the rest overflow
	if !strings.Contains(v, "┃ ⠋ agent-19") || !strings.Contains(v, "agent-17") || cards(v) != 3 {
		t.Errorf("the window should end on the selected agent and stay full:\n%s", v)
	}
	if !strings.Contains(v, "+17 hidden:") || !strings.Contains(v, "agent-00") {
		t.Errorf("overflow line:\n%s", v)
	}
	m.sel = "agent-00"
	if v := ansi.Strip(m.View()); !strings.Contains(v, "┃ ⠋ agent-00") {
		t.Errorf("selecting the first agent scrolls back:\n%s", v)
	}
}

func TestChipsCountWhatTheyCut(t *testing.T) {
	zone.NewGlobal()
	_, v := overview(60, 3, 20) // no room for a card
	last := strings.Split(strings.TrimRight(v, "\n"), "\n")[1]
	shown := strings.Count(last, "agent-")
	if want := fmt.Sprintf("+%d", 20-shown); !strings.HasSuffix(last, want) {
		t.Errorf("chips line %q should end with %s", last, want)
	}
}

func TestShortenAndMeta(t *testing.T) {
	for _, tc := range []struct {
		in   string
		w    int
		want string
	}{
		{"53% (105k/200k)", 20, "53% (105k/200k)"},
		{"53% (105k/200k)", 13, "53% 105k/200k"},
		{"53% (105k/200k)", 8, "53% 105…"},
		{"  a   b  ", 10, "a b"},
	} {
		if got := shorten(tc.in, tc.w); got != tc.want {
			t.Errorf("shorten(%q,%d) = %q, want %q", tc.in, tc.w, got, tc.want)
		}
	}
	for _, tc := range []struct {
		w    int
		want string
	}{
		{40, "claude-copilot · working 12m"},
		{26, "claude-copi… · working 12m"},
		{16, "working 12m"},
		{8, "working…"},
	} {
		if got := metaLine("claude-copilot", "working 12m", tc.w); got != tc.want {
			t.Errorf("metaLine(w=%d) = %q, want %q", tc.w, got, tc.want)
		}
	}
}

func TestQueuedTileShowsPositionAndGoal(t *testing.T) {
	zone.NewGlobal()
	now := time.Now()
	m := New(&fakeBE{}, "")
	m.now = func() time.Time { return now }
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 20})
	m.setAgents([]*board.Agent{
		{Name: "w1", Profile: "fake", Status: "working", StatusSince: now},
		{Name: "q2", Profile: "fake", Status: "queued", Goal: "second job", Created: now, StatusSince: now.Add(-3 * time.Minute)},
		{Name: "q1", Profile: "fake", Status: "queued", Goal: "first job", Created: now.Add(-time.Minute), StatusSince: now.Add(-3 * time.Minute)},
		{Name: "s1", Profile: "fake", Status: "starting", StatusSince: now.Add(-8 * time.Second)},
	})
	var order []string
	for _, a := range m.agents {
		order = append(order, a.Name)
	}
	if strings.Join(order, ",") != "s1,w1,q1,q2" { // starting ranks with working, queued last
		t.Errorf("sort: %v", order)
	}
	tw := m.grid().TileW
	q1 := ansi.Strip(m.tile(m.agents[2], tw))
	for _, want := range []string{"‖ q1", "fake · queued #1 3m", "first job"} {
		if !strings.Contains(q1, want) {
			t.Errorf("queued tile missing %q:\n%s", want, q1)
		}
	}
	for _, l := range strings.Split(q1, "\n") {
		if ansi.StringWidth(l) != tw {
			t.Errorf("row width %d, want %d: %q", ansi.StringWidth(l), tw, l)
		}
	}
	if s := ansi.Strip(m.tile(m.agents[0], tw)); !strings.Contains(s, "▸ s1") || !strings.Contains(s, "starting 8s") {
		t.Errorf("starting tile:\n%s", s)
	}
	if c := ansi.Strip(m.chips(m.agents[2:3], false)); !strings.Contains(c, "‖ q1 #1") {
		t.Errorf("chip: %q", c)
	}
}
