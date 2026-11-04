// Package keys is the single registry of fleet hotkeys. The tmux status hint,
// the overview header hint, the Alt-? popup, the overview ? overlay,
// `fleet keys` and the DESIGN.md table are all rendered from it.
package keys

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Group is the area a binding belongs to.
type Group string

// Groups, in display order.
const (
	Panes      Group = "Panes"
	Wall       Group = "Wall"
	Supervisor Group = "Supervisor"
	Overview   Group = "Overview"
)

// Groups lists the groups in display order.
var Groups = []Group{Panes, Wall, Supervisor, Overview}

// Scope is where a short hint line is shown.
type Scope int

// Hint scopes.
const (
	ScopeTmux     Scope = iota // tmux status-right
	ScopeOverview              // overview header
)

// Binding is one row of the registry.
type Binding struct {
	Group Group
	Key   string           // display key, e.g. "Alt-s"
	Tmux  []string         // tmux root-table keys it binds ("M-s"); none for overview-only keys
	Desc  string           // one-line explanation
	Hints map[Scope]string // optional short hint item per scope, e.g. "Alt-z zoom"
}

// Bindings is the ordered registry.
var Bindings = []Binding{
	{Group: Panes, Key: "Alt-←↑↓→", Tmux: []string{"M-Left", "M-Right", "M-Up", "M-Down"},
		Desc: "move to the pane in that direction", Hints: map[Scope]string{ScopeTmux: "Alt-←↑↓→ switch pane"}},
	{Group: Panes, Key: "Alt-z", Tmux: []string{"M-z"},
		Desc: "zoom this pane; press again to restore", Hints: map[Scope]string{ScopeTmux: "Alt-z zoom"}},
	{Group: Panes, Key: "Alt-x", Tmux: []string{"M-x"},
		Desc: "hide this pane (the agent keeps running)", Hints: map[Scope]string{ScopeTmux: "Alt-x hide"}},
	{Group: Panes, Key: "Alt-o", Tmux: []string{"M-o"},
		Desc: "jump to the marked pane (right-click → Mark)"},
	{Group: Wall, Key: "Alt-1/2/4/6/9", Tmux: []string{"M-1", "M-2", "M-4", "M-6", "M-9"},
		Desc: "wall: show up to N panes"},
	{Group: Wall, Key: "Alt-m", Tmux: []string{"M-m"},
		Desc: "wall: one main pane + stack"},
	{Group: Supervisor, Key: "Alt-s", Tmux: []string{"M-s"},
		Desc: "show/hide the supervisor", Hints: map[Scope]string{ScopeTmux: "Alt-s supervisor"}},
	{Group: Supervisor, Key: "Alt-S", Tmux: []string{"M-S"},
		Desc: "move the supervisor left/right"},
	{Group: Overview, Key: "Alt-c", Tmux: []string{"M-c"},
		Desc: "overview: tiles ↔ compact chips"},
	{Group: Overview, Key: "←↑↓→ / hjkl",
		Desc: "select a tile", Hints: map[Scope]string{ScopeOverview: "←↑↓→ select"}},
	{Group: Overview, Key: "⏎ / click",
		Desc: "show the selected agent on the wall", Hints: map[Scope]string{ScopeOverview: "⏎ show on wall"}},
	{Group: Overview, Key: "x",
		Desc: "take the selected agent off the wall", Hints: map[Scope]string{ScopeOverview: "x take off wall"}},
	{Group: Overview, Key: "Tab",
		Desc: "jump to the next waiting agent", Hints: map[Scope]string{ScopeOverview: "Tab next waiting"}},
	{Group: Overview, Key: "1/2/4/6/9/m",
		Desc: "same as Alt-1…9 / Alt-m"},
	{Group: Overview, Key: "q",
		Desc: "quit the overview"},
}

// HelpTmuxKey is the tmux key that opens the help popup. It is shown as the
// footer of the help, not as a row.
const HelpTmuxKey = "M-?"

// Footer is the last line of the help list.
const Footer = "Alt-? (tmux) · ? (overview)  show this list"

// allKeys is the final hint item per scope; Hint never drops it.
var allKeys = map[Scope]string{ScopeTmux: "Alt-? all keys", ScopeOverview: "? all keys"}

const sep = " · "

// KeyCol is the width of the key column in the help list.
const KeyCol = 14

const indent = "  "

// PopupMargin is the blank column on each side of the help inside the popup.
const PopupMargin = 1

// PopupWidth is the tmux popup width: the longest unwrapped help line plus
// the border and a 1-col margin on each side.
func PopupWidth() int {
	w := 0
	for _, l := range strings.Split(Help(0), "\n") {
		w = max(w, ansi.StringWidth(l))
	}
	return w + 2 + 2*PopupMargin
}

// PopupHeight is the tmux popup height: the help lines plus the border.
func PopupHeight() int { return strings.Count(Help(0), "\n") + 1 + 2 }

// Hint returns the short hint line for scope, at most width cells wide. Items
// are dropped from the end of the middle when it is too long; the final
// "all keys" item is always kept.
func Hint(scope Scope, width int) string {
	var items []string
	for _, b := range Bindings {
		if h := b.Hints[scope]; h != "" {
			items = append(items, h)
		}
	}
	last := allKeys[scope]
	for {
		s := strings.Join(append(append([]string{}, items...), last), sep)
		if ansi.StringWidth(s) <= width || len(items) == 0 {
			return s
		}
		items = items[:len(items)-1]
	}
}

// TmuxKeys returns the set of tmux root-table keys the registry claims.
func TmuxKeys() map[string]bool {
	m := map[string]bool{HelpTmuxKey: true}
	for _, b := range Bindings {
		for _, k := range b.Tmux {
			m[k] = true
		}
	}
	return m
}

// Help returns the grouped list as plain text, wrapped to width cells
// (width <= 0 means no wrapping).
func Help(width int) string {
	id := func(s string) string { return s }
	return help(width, id, id, id)
}

// HelpStyled is Help with theme colours: bold Mauve group headings, a bold
// key column and Subtext0 descriptions.
func HelpStyled(width int) string {
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#cba6f7"))
	key := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#cdd6f4"))
	desc := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6adc8"))
	r := func(st lipgloss.Style) func(string) string { return func(s string) string { return st.Render(s) } }
	return help(width, r(head), r(key), r(desc))
}

// HelpColumns is HelpStyled laid out in two columns side by side (Panes and
// Wall | Supervisor and Overview) with the footer below, for places too short
// for the single list. width is the total width; each column wraps to half.
func HelpColumns(width int) string {
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#cba6f7"))
	key := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#cdd6f4"))
	desc := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6adc8"))
	r := func(st lipgloss.Style) func(string) string { return func(s string) string { return st.Render(s) } }
	const gap = 2
	cw := max(minCol, (width-gap)/2)
	left := groupLines(Groups[:2], cw, r(head), r(key), r(desc))
	right := groupLines(Groups[2:], cw, r(head), r(key), r(desc))
	var lines []string
	for i := 0; i < max(len(left), len(right)); i++ {
		l, rt := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			rt = right[i]
		}
		lines = append(lines, l+strings.Repeat(" ", max(0, cw+gap-ansi.StringWidth(l)))+rt)
	}
	return strings.Join(append(lines, desc.Render(Footer)), "\n")
}

const minCol = len(indent) + KeyCol + 8 // narrowest useful help column

func help(width int, head, key, desc func(string) string) string {
	lines := groupLines(Groups, width, head, key, desc)
	lines = append(lines, "", desc(Footer))
	return strings.Join(lines, "\n")
}

// groupLines renders groups as heading + rows, wrapping descriptions to
// width (<= 0: no wrap). A key wider than the key column gets its own line.
func groupLines(groups []Group, width int, head, key, desc func(string) string) []string {
	var lines []string
	for gi, g := range groups {
		if gi > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, head(string(g)))
		for _, b := range Bindings {
			if b.Group != g {
				continue
			}
			pad := indent + strings.Repeat(" ", KeyCol)
			dw := 0
			if width > 0 {
				dw = max(1, width-ansi.StringWidth(pad))
			}
			own := ansi.StringWidth(b.Key) >= KeyCol // too wide: key on its own line
			if own {
				lines = append(lines, indent+key(b.Key))
			}
			k := b.Key + strings.Repeat(" ", max(0, KeyCol-ansi.StringWidth(b.Key)))
			for i, l := range wrap(b.Desc, dw) {
				if i == 0 && !own {
					lines = append(lines, indent+key(k)+desc(l))
				} else {
					lines = append(lines, pad+desc(l))
				}
			}
		}
	}
	return lines
}

// wrap splits s into lines of at most w cells on word boundaries (w <= 0: no wrap).
func wrap(s string, w int) []string {
	if w <= 0 || ansi.StringWidth(s) <= w {
		return []string{s}
	}
	var out []string
	cur := ""
	for _, word := range strings.Fields(s) {
		switch {
		case cur == "":
			cur = word
		case ansi.StringWidth(cur+" "+word) > w:
			out = append(out, cur)
			cur = word
		default:
			cur += " " + word
		}
	}
	return append(out, cur)
}

// Markdown returns the registry as a Markdown table plus the footer line:
// the content of the DESIGN.md keys block.
func Markdown() string {
	var b strings.Builder
	b.WriteString("| Group | Key | Description |\n|---|---|---|\n")
	for _, g := range Groups {
		for _, r := range Bindings {
			if r.Group == g {
				b.WriteString("| " + string(g) + " | `" + r.Key + "` | " + r.Desc + " |\n")
			}
		}
	}
	b.WriteString("\n`Alt-?` (tmux) · `?` (overview): show this list.\n")
	return b.String()
}
