package ui

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"

	"github.com/sturlabragason/fleet/internal/board"
	"github.com/sturlabragason/fleet/internal/keys"
	"github.com/sturlabragason/fleet/internal/layout"
	"github.com/sturlabragason/fleet/internal/logx"
)

// View renders the header, a card per agent wrapped to the pane, and, only
// when the cards don't all fit, one line of chips for the agents left out.
func (m *Model) View() (out string) {
	defer func() {
		if r := recover(); r != nil {
			logx.For("ui").Error("panic in View", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			out = fmt.Sprintf("render error: %v (see log)", r)
		}
	}()
	if m.w == 0 {
		return "loading…"
	}
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteByte('\n')
	if m.help {
		b.WriteString(m.helpOverlay())
		return zone.Scan(b.String())
	}
	g := m.grid()
	first, last := m.window(g)
	var lines []string
	for i := first; i < last; i += g.Cols {
		var row []string
		for j := i; j < min(i+g.Cols, last); j++ {
			row = append(row, m.tile(m.agents[j], g.TileW))
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, row...))
	}
	hidden := append(append([]*board.Agent{}, m.agents[:first]...), m.agents[last:]...)
	if c := m.chips(hidden, first == last); c != "" {
		lines = append(lines, c)
	}
	// no trailing newline: a full pane would push the header off the top
	b.WriteString(strings.Join(lines, "\n"))
	return zone.Scan(b.String())
}

// helpOverlay is the hotkey list centred below the header. Too tall for the
// pane: two columns; still too tall: the bottom is cut, never the top.
func (m *Model) helpOverlay() string {
	avail := max(m.h-1, 1)
	h := keys.HelpStyled(min(m.w, keys.PopupWidth()-2*keys.PopupMargin-2))
	if strings.Count(h, "\n")+1 > avail {
		h = keys.HelpColumns(m.w)
	}
	lines := strings.Split(h, "\n")
	if len(lines) > avail {
		lines = lines[:avail]
	}
	bw := 0
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.w, "")
		bw = max(bw, ansi.StringWidth(lines[i]))
	}
	for i, l := range lines { // one block width, so Place centres the block, not each line
		lines[i] = l + strings.Repeat(" ", bw-ansi.StringWidth(l))
	}
	return lipgloss.Place(m.w, avail, lipgloss.Center, lipgloss.Center, strings.Join(lines, "\n"))
}

func (m *Model) header() string {
	counts := map[string]int{}
	for _, a := range m.agents {
		counts[a.Status]++
	}
	h := fmt.Sprintf(" fleet · %d agents · %d working · %d waiting · %d error", len(m.agents), counts["working"], counts["waiting"], counts["error"]+counts["stuck"])
	hint := "  " + keys.Hint(keys.ScopeOverview, m.w-ansi.StringWidth(h)-2)
	if m.msg != "" {
		hint = "  " + m.msg
	}
	return ansi.Truncate(lipgloss.NewStyle().Bold(true).Render(h)+lipgloss.NewStyle().Foreground(colIdle).Render(hint), m.w, "…")
}

// line3 is the tile's last line: the latest report while the agent is not
// working (DESIGN §6.7), else the last screen line.
func line3(a *board.Agent, w int) string {
	if r := a.Report; r != nil && a.Status != "working" {
		g, c := reportStyle(r.State)
		return lipgloss.NewStyle().Foreground(c).Render(ansi.Truncate(g+" "+r.State+": "+r.Summary, w, "…"))
	}
	return lipgloss.NewStyle().Foreground(colText).Render(shorten(a.LastLine, w))
}

// grid is the card layout for the current pane and agent count.
func (m *Model) grid() layout.TileGrid {
	return m.cfg.Tiles(m.w, m.h-1, len(m.agents))
}

// window is the range of agents drawn as cards. On overflow it scrolls so
// the selected agent keeps its card, and keeps the window full at the end.
func (m *Model) window(g layout.TileGrid) (first, last int) {
	if g.Cards == 0 {
		return 0, 0
	}
	if i := m.index(); i >= g.Cards {
		first = min((i/g.Cols-g.Rows+1)*g.Cols, len(m.agents)-g.Cards)
	}
	return first, min(first+g.Cards, len(m.agents))
}

func (m *Model) tile(a *board.Agent, tw int) string {
	col := statusColor(a.Status, m.frame)
	w := tw - 2
	border := lipgloss.RoundedBorder()
	if a.Name == m.sel {
		border = lipgloss.ThickBorder()
	}
	bc := col
	if a.Name == m.sel {
		bc = colSelect
	}
	wall := ""
	if m.onWall[a.Name] {
		wall = " ▣"
	}
	inner := w - 4
	st1 := lipgloss.NewStyle().Foreground(col).Bold(true)
	l1 := st1.Render(glyph(a.Status, m.frame) + " " + ansi.Truncate(a.Name, inner-4, "…") + wall)
	state := strings.TrimSpace(a.Status + m.queueMark(a.Name) + " " + since(a.StatusSince, m.now()))
	l2 := lipgloss.NewStyle().Foreground(colIdle).Render(metaLine(a.Profile, state, inner))
	if ti, ok := m.teams[a.Name]; ok && ti.Team != "" {
		ts := lipgloss.NewStyle().Foreground(lipgloss.Color(ti.Color))
		l1 = st1.Render(glyph(a.Status, m.frame)+" "+ansi.Truncate(a.Name, inner-4-2, "…")) + " " + ts.Render(teamMark)
		if wall != "" {
			l1 += st1.Render(wall)
		}
		tn := ansi.Truncate(ti.Team, 10, "…")
		rest := lipgloss.NewStyle().Foreground(colIdle).Render(" · " + metaLine(a.Profile, state, inner-ansi.StringWidth(tn)-3))
		l2 = ansi.Truncate(ts.Render(tn)+rest, inner, "…")
	}
	l3 := line3(a, inner)
	if a.Status == "queued" { // no pane yet: the start of the goal
		l3 = lipgloss.NewStyle().Foreground(colIdle).Render(shorten(a.Goal, inner))
	}
	text := l1 + "\n" + l2 + "\n" + l3
	if l4 := subagentRow(a.Subagents, inner, m.frame); l4 != "" {
		text += "\n" + l4
	}
	body := lipgloss.NewStyle().Width(inner+2).Padding(0, 1).Render(text)
	t := lipgloss.NewStyle().Border(border).BorderForeground(bc).Width(w).Height(m.cfg.TileH - 2).Render(body)
	return zone.Mark("tile-"+a.Name, t)
}

// chips is the overflow line: how many agents have no card, then a chip for
// each that fits and "+k" for the rest. all is true when no card fits at
// all; the line then holds every agent and only counts what it cuts.
func (m *Model) chips(rest []*board.Agent, all bool) string {
	if len(rest) == 0 {
		return ""
	}
	dim := lipgloss.NewStyle().Foreground(colIdle)
	line := " "
	if !all {
		line += dim.Render(fmt.Sprintf("+%d hidden:", len(rest))) + "  "
	}
	for i, a := range rest {
		st := lipgloss.NewStyle().Foreground(statusColor(a.Status, m.frame))
		if a.Name == m.sel {
			st = st.Reverse(true)
		}
		c := st.Render(glyph(a.Status, m.frame) + " " + a.Name + m.queueMark(a.Name))
		if ti, ok := m.teams[a.Name]; ok && ti.Team != "" {
			c += " " + lipgloss.NewStyle().Foreground(lipgloss.Color(ti.Color)).Render(teamMark)
		}
		more := ""
		if left := len(rest) - i - 1; left > 0 {
			more = fmt.Sprintf("  +%d", left)
		}
		if ansi.StringWidth(line+c+more) > m.w {
			return ansi.Truncate(line+dim.Render(fmt.Sprintf("+%d", len(rest)-i)), m.w, "")
		}
		line += zone.Mark("tile-"+a.Name, c) + "  "
	}
	return strings.TrimRight(line, " ")
}

// queueMark is " #n" for a queued agent (1 = starts next), else "".
func (m *Model) queueMark(name string) string {
	if n := m.queuePos[name]; n > 0 {
		return fmt.Sprintf(" #%d", n)
	}
	return ""
}

// metaLine is "profile · state" in w cells. The state (status and age) is
// kept whole; the profile is cut first.
func metaLine(profile, state string, w int) string {
	full := profile + " · " + state
	if ansi.StringWidth(full) <= w {
		return full
	}
	if room := w - ansi.StringWidth(" · "+state); room >= 4 {
		return ansi.Truncate(profile, room, "…") + " · " + state
	}
	return ansi.Truncate(state, w, "…")
}

// shorten fits a pane's last line into w cells: runs of spaces collapse,
// then brackets go ("53% (105k/200k)" -> "53% 105k/200k"), then it is cut.
func shorten(s string, w int) string {
	s = strings.Join(strings.Fields(s), " ")
	if ansi.StringWidth(s) > w {
		s = strings.Join(strings.Fields(strings.NewReplacer("(", " ", ")", " ", "[", " ", "]", " ").Replace(s)), " ")
	}
	return ansi.Truncate(s, w, "…")
}

// teamMark follows the name of an agent in a team (● is the waiting glyph).
const teamMark = "◆"

func since(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t).Round(time.Second)
	if d >= time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
