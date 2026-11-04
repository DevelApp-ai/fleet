package ui

import "github.com/charmbracelet/lipgloss"

// Colours per status; pulse alternates for waiting tiles.
var (
	colWorking = lipgloss.Color("#3fb950")
	colWaitA   = lipgloss.Color("#d29922")
	colWaitB   = lipgloss.Color("#ffd75f")
	colError   = lipgloss.Color("#f85149")
	colIdle    = lipgloss.Color("#8b949e")
	colGone    = lipgloss.Color("#484f58")
	colSelect  = lipgloss.Color("#58a6ff")
	colText    = lipgloss.Color("#c9d1d9")
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func statusColor(st string, frame int) lipgloss.Color {
	switch st {
	case "working":
		return colWorking
	case "waiting":
		if (frame/6)%2 == 0 {
			return colWaitA
		}
		return colWaitB
	case "error", "stuck":
		return colError
	case "gone":
		return colGone
	}
	return colIdle
}

func glyph(st string, frame int) string {
	switch st {
	case "working":
		return spinner[frame%len(spinner)]
	case "waiting":
		return "●"
	case "error":
		return "✖"
	case "stuck":
		return "⧗"
	case "idle":
		return "○"
	case "gone":
		return "·"
	}
	return "?"
}

// attention rank: lower sorts first.
func rank(st string) int {
	switch st {
	case "waiting":
		return 0
	case "error":
		return 1
	case "stuck":
		return 2
	case "working":
		return 3
	case "idle":
		return 4
	case "unknown":
		return 5
	}
	return 6
}
