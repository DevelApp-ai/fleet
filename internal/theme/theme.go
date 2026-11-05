// Package theme holds the colour palette shared by the TUI and the tmux chrome.
//
// Catppuccin Mocha is the terminal palette; the house brand's Lane blue is a
// print and web accent (1.8:1 on Base, unreadable here) and is deliberately not
// used. What carries over from the brand is the discipline: colour means status,
// never decoration, and at most one thing on screen is the accent. Before adding
// a colour or a status, read docs/style.md §4.
package theme

// Catppuccin Mocha (https://catppuccin.com/palette).
const (
	Rosewater = "#f5e0dc"
	Pink      = "#f5c2e7"
	Mauve     = "#cba6f7"
	Red       = "#f38ba8"
	Maroon    = "#eba0ac"
	Peach     = "#fab387"
	Yellow    = "#f9e2af"
	Green     = "#a6e3a1"
	Teal      = "#94e2d5"
	Sky       = "#89dceb"
	Sapphire  = "#74c7ec"
	Blue      = "#89b4fa"
	Lavender  = "#b4befe"
	Text      = "#cdd6f4"
	Subtext1  = "#bac2de"
	Subtext0  = "#a6adc8"
	Overlay2  = "#9399b2"
	Overlay1  = "#7f849c"
	Overlay0  = "#6c7086"
	Surface2  = "#585b70"
	Surface1  = "#45475a"
	Surface0  = "#313244"
	Base      = "#1e1e2e"
	Mantle    = "#181825"
	Crust     = "#11111b"
)

// Roles used across components. A role is a meaning, not a shade: give a new
// status its own glyph in internal/ui before giving it its own colour.
const (
	Working = Green
	// WorkingDim is the dim step of the running-subagent wave: a darker green,
	// never grey, so it can't pass for a done dot (DESIGN.md §4 Subagents).
	WorkingDim = "#628067"
	Waiting    = Yellow
	WaitingAlt = Peach // second pulse frame for waiting tiles
	Error      = Red
	Stuck      = Maroon
	Idle       = Overlay1
	Gone       = Surface2
	Select     = Lavender
	Supervisor = Mauve
	Muted      = Overlay1
)

// Swatch is a named colour.
type Swatch struct{ Name, Hex string }

// TeamPalette is the ordered team colour palette (DESIGN.md §5c). It leaves out
// the status colours, the selection lavender and rosewater/flamingo.
var TeamPalette = []Swatch{
	{"blue", Blue}, {"pink", Pink}, {"teal", Teal}, {"mauve", Mauve}, {"sky", Sky}, {"sapphire", Sapphire},
}

var status = map[string]string{
	"working": Working,
	"waiting": Waiting,
	"error":   Error,
	"stuck":   Stuck,
	"idle":    Idle,
	"gone":    Gone,
}

// Status returns the colour for an agent status; unknown statuses get Idle.
func Status(st string) string {
	if c, ok := status[st]; ok {
		return c
	}
	return Idle
}
