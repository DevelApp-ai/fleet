// Package wall manages the main fleet window: supervisor, overview and the
// live wall of agent panes moved in with join-pane / out with break-pane.
// All state lives in tmux (window options and pane @role/@fleet_used tags),
// so the TUI and `fleet wall ...` key bindings can both drive it.
package wall

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sturlabragason/fleet/internal/layout"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/tmux"
)

// Wall drives one main window.
type Wall struct {
	T      *tmux.Client
	Target string // e.g. "fleet:main"
	Cfg    layout.Config
}

// New returns a wall for session:main.
func New(t *tmux.Client, session string) *Wall {
	return &Wall{T: t, Target: session + ":main", Cfg: layout.Default}
}

// State is the parsed main window.
type State struct {
	W, H      int
	Sup, Ov   string
	Wall      []tmux.PaneInfo // in window order
	Preset    string
	Cfg       layout.Config
	SupHidden string // pane id of hidden supervisor, if any
}

func (w *Wall) opt(name string) string {
	s, _ := w.T.Run("show-options", "-wqv", "-t", w.Target, name)
	return strings.TrimSpace(s)
}

func (w *Wall) setOpt(name, val string) {
	w.T.Run("set-option", "-w", "-t", w.Target, name, val)
}

// Read loads the current state.
func (w *Wall) Read() (*State, error) {
	dims, err := w.T.Run("display-message", "-p", "-t", w.Target, "#{window_width} #{window_height}")
	if err != nil {
		return nil, err
	}
	s := &State{Cfg: w.Cfg, Preset: w.opt("@fleet_wall")}
	fmt.Sscan(dims, &s.W, &s.H)
	if s.Preset == "" {
		s.Preset = "4"
	}
	if v := w.opt("@fleet_sup_side"); v != "" {
		s.Cfg.SupSide = v
	}
	s.Cfg.ChipsOnly = w.opt("@fleet_chips") == "1"
	panes, err := w.T.ListPanes(w.Target)
	if err != nil {
		return nil, err
	}
	for _, p := range panes {
		switch p.Role {
		case "sup":
			s.Sup = p.ID
		case "ov":
			s.Ov = p.ID
		default:
			s.Wall = append(s.Wall, p)
		}
	}
	return s, nil
}

// Apply recomputes and applies the full layout (never `tiled`).
func (w *Wall) Apply() error {
	s, err := w.Read()
	if err != nil {
		return err
	}
	var ids []string
	for _, p := range s.Wall {
		ids = append(ids, p.ID)
	}
	lay, leaves := s.Cfg.Render(layout.Input{W: s.W, H: s.H, Sup: s.Sup, Ov: s.Ov, Wall: ids, Preset: s.Preset})
	w.reorder(leaves)
	if _, err := w.T.Run("select-layout", "-t", w.Target, lay); err != nil {
		logx.For("wall").Error("select-layout", "layout", lay, "err", err)
		return err
	}
	if s.Sup != "" {
		w.T.Run("resize-pane", "-t", s.Sup, "-x", strconv.Itoa(s.Cfg.SupCols(s.W)))
	}
	return nil
}

// reorder swaps panes so window order matches layout leaf order.
func (w *Wall) reorder(want []string) {
	panes, _ := w.T.ListPanes(w.Target)
	cur := make([]string, len(panes))
	for i, p := range panes {
		cur[i] = p.ID
	}
	for i, id := range want {
		if i >= len(cur) || cur[i] == id {
			continue
		}
		j := index(cur, id)
		if j < 0 {
			continue
		}
		w.T.Run("swap-pane", "-d", "-s", cur[i], "-t", id)
		cur[i], cur[j] = cur[j], cur[i]
	}
}

func index(l []string, s string) int {
	for i, v := range l {
		if v == s {
			return i
		}
	}
	return -1
}

// Show brings an agent pane onto the wall (evicting the least recently used).
func (w *Wall) Show(pane string) error {
	log := logx.For("wall")
	s, err := w.Read()
	if err != nil {
		return err
	}
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	for _, p := range s.Wall {
		if p.ID == pane {
			w.T.SetPaneOption(pane, "@fleet_used", now)
			_, err := w.T.Run("select-pane", "-t", pane)
			return err
		}
	}
	slots := s.Cfg.Slots(s.Preset, s.W)
	for len(s.Wall) >= slots {
		lru := LRU(s.Wall)
		w.breakOut(lru)
		s.Wall = remove(s.Wall, lru)
	}
	target := s.Ov
	if len(s.Wall) > 0 {
		target = s.Wall[len(s.Wall)-1].ID
	}
	if _, err := w.T.Run("join-pane", "-d", "-s", pane, "-t", target); err != nil {
		if _, err2 := w.T.Run("join-pane", "-d", "-s", pane, "-t", s.Sup); err2 != nil {
			return err
		}
	}
	w.T.SetPaneOption(pane, "@role", "wall")
	w.T.SetPaneOption(pane, "@fleet_used", now)
	log.Info("on wall", "pane", pane, "preset", s.Preset)
	if err := w.Apply(); err != nil {
		return err
	}
	_, err = w.T.Run("select-pane", "-t", pane)
	return err
}

// LRU returns the wall pane used least recently.
func LRU(ps []tmux.PaneInfo) string {
	c := append([]tmux.PaneInfo(nil), ps...)
	sort.SliceStable(c, func(i, j int) bool { return c[i].Used < c[j].Used })
	return c[0].ID
}

func remove(ps []tmux.PaneInfo, id string) []tmux.PaneInfo {
	var out []tmux.PaneInfo
	for _, p := range ps {
		if p.ID != id {
			out = append(out, p)
		}
	}
	return out
}

func (w *Wall) breakOut(pane string) {
	name, _ := w.T.Run("display-message", "-p", "-t", pane, "#{@fleet_agent}")
	win := "ag-" + name
	if name == "" {
		win = "loose"
	}
	session := strings.SplitN(w.Target, ":", 2)[0]
	if _, err := w.T.Run("break-pane", "-d", "-s", pane, "-t", session+":", "-n", win); err == nil {
		w.T.SetPaneOption(pane, "@role", "agent")
		logx.For("wall").Info("off wall", "pane", pane, "agent", name)
	}
}

// HideFocused hides the active pane of the main window if it is a wall pane.
// Otherwise it shows a tmux message and does nothing.
func (w *Wall) HideFocused() error {
	out, err := w.T.Run("display-message", "-p", "-t", w.Target, "#{pane_id}\t#{@role}")
	if err != nil {
		return err
	}
	id, role, _ := strings.Cut(out, "\t")
	if role != "wall" {
		w.T.Run("display-message", "M-x: focus a wall pane first (supervisor/overview stay)")
		return nil
	}
	return w.Hide(id)
}

// Hide returns a pane to its hidden window.
func (w *Wall) Hide(pane string) error { w.breakOut(pane); return w.Apply() }

// SetPreset changes the wall layout, evicting panes that no longer fit.
func (w *Wall) SetPreset(p string) error {
	p = strings.ToLower(p)
	if _, ok := layout.Presets[p]; !ok {
		return fmt.Errorf("unknown preset %q (1 2 4 6 9 m)", p)
	}
	w.setOpt("@fleet_wall", p)
	s, err := w.Read()
	if err != nil {
		return err
	}
	for slots := s.Cfg.Slots(p, s.W); len(s.Wall) > slots; {
		lru := LRU(s.Wall)
		w.breakOut(lru)
		s.Wall = remove(s.Wall, lru)
	}
	return w.Apply()
}

// ToggleSup hides or shows the supervisor pane.
func (w *Wall) ToggleSup() error {
	s, err := w.Read()
	if err != nil {
		return err
	}
	session := strings.SplitN(w.Target, ":", 2)[0]
	if s.Sup != "" {
		if _, err := w.T.Run("break-pane", "-d", "-s", s.Sup, "-t", session+":", "-n", "sup-hidden"); err != nil {
			return err
		}
		return w.Apply()
	}
	panes, _ := w.T.ListPanes("")
	for _, p := range panes {
		if p.Role == "sup" {
			flag := "-b"
			if s.Cfg.SupSide == "right" {
				flag = ""
			}
			args := []string{"join-pane", "-d", "-h", "-l", strconv.Itoa(s.Cfg.SupCols(s.W)), "-s", p.ID, "-t", s.Ov}
			if flag != "" {
				args = append(args, flag)
			}
			if _, err := w.T.Run(args...); err != nil {
				return err
			}
			return w.Apply()
		}
	}
	return fmt.Errorf("no supervisor pane")
}

// SwapSide moves the supervisor to the other side.
func (w *Wall) SwapSide() error {
	s, err := w.Read()
	if err != nil {
		return err
	}
	side := "right"
	if s.Cfg.SupSide == "right" {
		side = "left"
	}
	w.setOpt("@fleet_sup_side", side)
	return w.Apply()
}

// ToggleChips switches the overview between tiles and chips-only.
func (w *Wall) ToggleChips() error {
	v := "1"
	if w.opt("@fleet_chips") == "1" {
		v = "0"
	}
	w.setOpt("@fleet_chips", v)
	return w.Apply()
}

// OnWall returns agent names currently on the wall.
func (w *Wall) OnWall() map[string]bool {
	out := map[string]bool{}
	panes, err := w.T.ListPanes(w.Target)
	if err != nil {
		return out
	}
	for _, p := range panes {
		if p.Agent != "" {
			out[p.Agent] = true
		}
	}
	return out
}
