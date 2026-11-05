// Package layout owns ALL pane geometry of the main fleet window: supervisor
// side and width, overview strip height and the wall presets. Change the
// numbers here; nothing else hard-codes sizes.
package layout

// Config holds every geometry knob.
type Config struct {
	SupSide       string // "left" or "right"
	SupWidth      int    // columns when window >= NarrowBelow
	SupWidthNarr  int    // columns when window < NarrowBelow
	NarrowBelow   int    // window width threshold
	SupHidden     bool
	OverviewRows  int // full overview: 2 tile rows + header + chips
	ChipsOnlyRows int // compact overview
	ChipsOnly     bool
	MainPercent   int // M preset: main pane share of wall width
	MinPaneW      int // smallest readable wall pane width
	MinPaneH      int // smallest readable wall pane height (incl. the border-status row)
	FocusPercent  int // emphasis pane share of the wall
	TileW, TileH  int // overview tile size (used by the TUI)
}

// Default is the designer-reviewed layout.
var Default = Config{
	SupSide:       "left",
	SupWidth:      90,
	SupWidthNarr:  64,
	NarrowBelow:   220,
	OverviewRows:  14,
	ChipsOnlyRows: 3,
	MainPercent:   60,
	MinPaneW:      64,
	MinPaneH:      17,
	FocusPercent:  60,
	TileW:         26,
	TileH:         6,
}

// SupCols returns the supervisor width for a window width.
func (c Config) SupCols(w int) int {
	if w < c.NarrowBelow {
		return c.SupWidthNarr
	}
	return c.SupWidth
}

// OvRows returns the overview height.
func (c Config) OvRows() int {
	if c.ChipsOnly {
		return c.ChipsOnlyRows
	}
	return c.OverviewRows
}

// Presets are the wall layouts: name -> slots.
var Presets = map[string]int{"1": 1, "2": 2, "4": 4, "6": 6, "9": 9, "m": 4}

// Slots returns the slot cap for a preset. Presets cap how many panes may be
// on the wall; the grid (see grid.go) decides the shape.
func (c Config) Slots(preset string) int {
	n, ok := Presets[preset]
	if !ok {
		n = 4
	}
	return n
}
