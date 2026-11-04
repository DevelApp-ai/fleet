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
	Wall9MinCols  int // 9-slot wall allowed only at this width
	MainPercent   int // M preset: main pane share of wall width
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
	Wall9MinCols:  280,
	MainPercent:   60,
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

// Slots returns the slot count for a preset at a window width (9 capped to 4 when narrow).
func (c Config) Slots(preset string, w int) int {
	n, ok := Presets[preset]
	if !ok {
		n = 4
	}
	if n > 4 && preset == "9" && w < c.Wall9MinCols {
		n = 4
	}
	return n
}

// gridCols returns columns for a preset.
func gridCols(preset string) int {
	switch preset {
	case "1":
		return 1
	case "2", "4":
		return 2
	case "6", "9":
		return 3
	}
	return 2
}
