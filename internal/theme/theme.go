// Package theme holds the colour palette shared by the TUI and the tmux chrome.
package theme

// Catppuccin Mocha (https://catppuccin.com/palette).
const (
	Rosewater = "#f5e0dc"
	Mauve     = "#cba6f7"
	Red       = "#f38ba8"
	Maroon    = "#eba0ac"
	Peach     = "#fab387"
	Yellow    = "#f9e2af"
	Green     = "#a6e3a1"
	Teal      = "#94e2d5"
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

// Roles used across components.
const (
	Working    = Green
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
