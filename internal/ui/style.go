package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/sturlabragason/fleet/internal/theme"
)

// Colours per status; pulse alternates for waiting tiles.
var (
	colWorking = lipgloss.Color(theme.Working)
	colWaitA   = lipgloss.Color(theme.Waiting)
	colWaitB   = lipgloss.Color(theme.WaitingAlt)
	colError   = lipgloss.Color(theme.Error)
	colStuck   = lipgloss.Color(theme.Stuck)
	colIdle    = lipgloss.Color(theme.Idle)
	colGone    = lipgloss.Color(theme.Gone)
	colSelect  = lipgloss.Color(theme.Select)
	colText    = lipgloss.Color(theme.Text)
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
	case "error":
		return colError
	case "stuck":
		return colStuck
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
