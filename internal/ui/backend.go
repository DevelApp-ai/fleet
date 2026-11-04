// Package ui is the overview TUI: a grid of agent tiles sorted by attention.
package ui

import "github.com/sturlabragason/fleet/internal/board"

// Backend is what the TUI needs from the rest of fleet (fakeable in tests).
type Backend interface {
	Refresh() ([]*board.Agent, error)
	OnWall() map[string]bool
	Show(name string) error // bring onto the live wall
	Preset(p string) error  // wall layout 1/2/4/6/9/m
	Hide(name string) error // send back to its hidden window
	WallEnabled() bool
	// SupCtx is the supervisor context meter: label ("34%") and colour,
	// both "" when the fill is unknown.
	SupCtx() (label, color string)
	SetSupCtx(label, color string) error // "" unsets the border option
	ResetSup() error                     // ask tmux to confirm, then reset the supervisor
}
