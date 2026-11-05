package wall

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sturlabragason/fleet/internal/tmux"
)

// Hysteresis for the emphasis pane (DESIGN.md §5b.3, ADR-16): resizing a pane
// makes an agent TUI reflow, so emphasis moves reluctantly.
const (
	EmphasisDwell     = 10 * time.Second // a candidate must have waited this long
	EmphasisHold      = 30 * time.Second // ... and the current choice held this long
	EmphasisFocusHold = 60 * time.Second // ... or this long, if I focused it myself
)

// chooseEmphasis picks the emphasis pane. ps are the wall panes, active is the
// window's active pane and prev the active pane at the last call (so a change
// is an explicit focus by the user), cur/at are the cached choice and when it
// was made, and curFocus records that cur was chosen by an explicit focus. It
// returns the choice and whether that choice is an explicit focus. It is pure
// so the hysteresis can be tested without tmux.
func chooseEmphasis(ps []tmux.PaneInfo, active, prev, cur string, curFocus bool, at, now time.Time) (string, bool) {
	onWall := func(id string) bool {
		for _, p := range ps {
			if p.ID == id {
				return true
			}
		}
		return false
	}
	fallback := func() (string, bool) {
		if best := bestCandidate(ps); best != nil {
			return best.ID, false
		}
		if onWall(active) {
			return active, false
		}
		return "", false
	}
	if active != prev && active != cur && onWall(active) {
		return active, true // the user just focused a wall pane: wins immediately
	}
	if cur == "" || !onWall(cur) {
		return fallback()
	}
	// A pane I chose myself keeps emphasis for EmphasisFocusHold wherever the
	// active pane wanders to afterwards (clicking the supervisor must not
	// re-tile the pane I just picked).
	if curFocus && now.Sub(at) < EmphasisFocusHold {
		return cur, true
	}
	curPane := paneByID(ps, cur)
	if !needsAttention(curPane.Status) && cur != active {
		return fallback() // it stopped needing attention
	}
	best := bestCandidate(ps)
	if best == nil || best.ID == cur {
		return cur, curFocus
	}
	hold := EmphasisHold
	if cur == active || curFocus {
		hold = EmphasisFocusHold
	}
	// StatusSince 0 means "untagged by an older build": the dwell is unknown,
	// so it is not satisfied and the pane cannot steal emphasis yet.
	if best.StatusSince == 0 {
		return cur, curFocus
	}
	waited := now.Sub(time.Unix(0, best.StatusSince))
	if now.Sub(at) >= hold && waited >= EmphasisDwell {
		return best.ID, false
	}
	return cur, curFocus
}

// bestCandidate is the pane that most needs attention: waiting before
// error/stuck, oldest @fleet_status_since first. nil when nothing needs me.
func bestCandidate(ps []tmux.PaneInfo) *tmux.PaneInfo {
	var c []tmux.PaneInfo
	for _, p := range ps {
		if needsAttention(p.Status) {
			c = append(c, p)
		}
	}
	if len(c) == 0 {
		return nil
	}
	rank := func(p tmux.PaneInfo) int {
		if p.Status == "waiting" {
			return 0
		}
		return 1
	}
	// StatusSince 0 means "untagged by an older build": the wait is unknown, so
	// it sorts last. Sorting it first would let one untagged pane sit at the
	// head of the queue and (via the dwell guard) block emphasis forever.
	since := func(p tmux.PaneInfo) int64 {
		if p.StatusSince == 0 {
			return math.MaxInt64
		}
		return p.StatusSince
	}
	sort.SliceStable(c, func(i, j int) bool {
		if ri, rj := rank(c[i]), rank(c[j]); ri != rj {
			return ri < rj
		}
		return since(c[i]) < since(c[j])
	})
	return &c[0]
}

func paneByID(ps []tmux.PaneInfo, id string) tmux.PaneInfo {
	for _, p := range ps {
		if p.ID == id {
			return p
		}
	}
	return tmux.PaneInfo{}
}

// emphasis returns the pane to emphasise for this state, updating the cached
// choice in window options. "" means an even grid.
func (w *Wall) emphasis(s *State) string {
	if w.opt("@fleet_emphasis") == "0" || len(s.Wall) < 2 {
		return ""
	}
	cur := w.opt("@fleet_emphasis_pane")
	curFocus := w.opt("@fleet_emphasis_focus") == "1"
	at, _ := strconv.ParseInt(w.opt("@fleet_emphasis_at"), 10, 64)
	out, _ := w.T.Run("display-message", "-p", "-t", w.Target, "#{pane_id}")
	active := strings.TrimSpace(out)
	prev := w.opt("@fleet_emphasis_active")
	now := w.now()
	pick, byFocus := chooseEmphasis(s.Wall, active, prev, cur, curFocus, time.Unix(0, at), now)
	if active != prev {
		w.setOpt("@fleet_emphasis_active", active)
	}
	if pick != cur {
		w.setOpt("@fleet_emphasis_pane", pick)
		w.setOpt("@fleet_emphasis_at", strconv.FormatInt(now.UnixNano(), 10))
	}
	if byFocus != curFocus {
		w.setOpt("@fleet_emphasis_focus", map[bool]string{true: "1", false: "0"}[byFocus])
	}
	return pick
}

// ToggleEmphasis switches the emphasis pane off and back on (`fleet wall emphasis`).
func (w *Wall) ToggleEmphasis() error {
	v := "0"
	if w.opt("@fleet_emphasis") == "0" {
		v = "1"
	}
	w.setOpt("@fleet_emphasis", v)
	return w.Apply()
}

func (w *Wall) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}
