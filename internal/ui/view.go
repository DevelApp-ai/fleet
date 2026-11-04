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
	"github.com/sturlabragason/fleet/internal/logx"
)

// View renders header, tiles (as many rows as fit), then chips for the rest.
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
	cols := m.cols()
	rows := (m.h - 2) / m.cfg.TileH
	if rows < 0 {
		rows = 0
	}
	nt := cols * rows
	if nt > len(m.agents) {
		nt = len(m.agents)
	}
	var lines []string
	for r := 0; r*cols < nt; r++ {
		var row []string
		for c := 0; c < cols && r*cols+c < nt; c++ {
			row = append(row, m.tile(m.agents[r*cols+c]))
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, row...))
	}
	if len(lines) > 0 {
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteByte('\n')
	}
	b.WriteString(m.chips(m.agents[nt:]))
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

func (m *Model) tile(a *board.Agent) string {
	col := statusColor(a.Status, m.frame)
	w := m.cfg.TileW - 2
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
	l1 := lipgloss.NewStyle().Foreground(col).Bold(true).Render(glyph(a.Status, m.frame) + " " + ansi.Truncate(a.Name, inner-4, "…") + wall)
	l2 := lipgloss.NewStyle().Foreground(colIdle).Render(ansi.Truncate(a.Profile+" · "+a.Status+" "+since(a.StatusSince, m.now()), inner, "…"))
	l3 := lipgloss.NewStyle().Foreground(colText).Render(ansi.Truncate(strings.TrimSpace(a.LastLine), inner, "…"))
	body := lipgloss.NewStyle().Width(inner+2).Padding(0, 1).Render(l1 + "\n" + l2 + "\n" + l3)
	t := lipgloss.NewStyle().Border(border).BorderForeground(bc).Width(w).Height(m.cfg.TileH - 2).Render(body)
	return zone.Mark("tile-"+a.Name, t)
}

func (m *Model) chips(rest []*board.Agent) string {
	if len(rest) == 0 {
		return ""
	}
	var cs []string
	for _, a := range rest {
		st := lipgloss.NewStyle().Foreground(statusColor(a.Status, m.frame))
		if a.Name == m.sel {
			st = st.Reverse(true)
		}
		cs = append(cs, zone.Mark("tile-"+a.Name, st.Render(glyph(a.Status, m.frame)+" "+a.Name)))
	}
	return ansi.Truncate(" "+strings.Join(cs, "  "), m.w, "…")
}

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
