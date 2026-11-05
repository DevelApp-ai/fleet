package layout

import "testing"

func TestTiles(t *testing.T) {
	c := Default
	for _, tc := range []struct {
		name    string
		w, h, n int
		want    TileGrid
	}{
		{"one agent caps width", 150, 13, 1, TileGrid{Cols: 1, Rows: 1, TileW: 48, Cards: 1}},
		{"five in one row", 150, 6, 5, TileGrid{Cols: 5, Rows: 1, TileW: 30, Cards: 5}},
		{"five wrap to comfort width", 150, 12, 5, TileGrid{Cols: 3, Rows: 2, TileW: 48, Cards: 5}},
		{"bug: nine in the 13-row overview", 150, 12, 9, TileGrid{Cols: 5, Rows: 2, TileW: 30, Cards: 9}},
		{"nine, tall pane", 150, 30, 9, TileGrid{Cols: 3, Rows: 3, TileW: 48, Cards: 9}},
		{"six, tall pane", 150, 30, 6, TileGrid{Cols: 3, Rows: 2, TileW: 48, Cards: 6}},
		{"twelve overflow keeps a line", 150, 12, 12, TileGrid{Cols: 5, Rows: 1, TileW: 30, Cards: 5}},
		{"twenty fit tall", 150, 30, 20, TileGrid{Cols: 4, Rows: 5, TileW: 37, Cards: 20}},
		{"too short for a card", 150, 2, 20, TileGrid{Cols: 5, Rows: 0, TileW: 30, Cards: 0}},
		{"narrower than a card", 20, 12, 3, TileGrid{Cols: 1, Rows: 1, TileW: 20, Cards: 1}},
	} {
		if got := c.Tiles(tc.w, tc.h, tc.n); got != tc.want {
			t.Errorf("%s: Tiles(%d,%d,%d) = %+v, want %+v", tc.name, tc.w, tc.h, tc.n, got, tc.want)
		}
	}
}
