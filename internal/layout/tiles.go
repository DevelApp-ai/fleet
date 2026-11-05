package layout

// TileGrid is how the overview lays out its agent cards.
type TileGrid struct {
	Cols, Rows int // card grid on screen
	TileW      int // card width including the border
	Cards      int // cards on screen; the other agents overflow
}

// Tiles fits n agent cards into a w×h area (the overview below its header).
// When every card fits, it takes the fewest rows that give cards at least
// TileComfortW columns, then stretches them to share the width (up to
// TileMaxW). When they don't, one row is kept for the overflow line and the
// grid uses the narrowest cards; Cards is 0 when not even one row fits.
func (c Config) Tiles(w, h, n int) TileGrid {
	if n <= 0 || w <= 0 {
		return TileGrid{Cols: 1}
	}
	maxCols := max(w/c.TileW, 1)
	width := func(cols int) int { return max(min(w/cols, c.TileMaxW), min(w, c.TileW)) }
	need := (n + maxCols - 1) / maxCols
	if fit := h / c.TileH; need <= fit {
		rows := need
		for r := need; r <= min(fit, n); r++ {
			if w/((n+r-1)/r) >= c.TileComfortW {
				rows = r
				break
			}
		}
		cols := (n + rows - 1) / rows
		rows = (n + cols - 1) / cols
		return TileGrid{Cols: cols, Rows: rows, TileW: width(cols), Cards: n}
	}
	rows := max((h-1)/c.TileH, 0)
	return TileGrid{Cols: maxCols, Rows: rows, TileW: width(maxCols), Cards: rows * maxCols}
}
