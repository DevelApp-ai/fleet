package fleet

import (
	"hash/fnv"
	"time"

	"github.com/sturlabragason/fleet/internal/board"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/status"
)

// Refresh captures every agent pane, updates statuses on the board, logs
// transitions and returns the agents sorted by name.
func (f *Fleet) Refresh() ([]*board.Agent, error) {
	panes, _ := f.T.ListPanes("")
	byAgent := map[string]bool{}
	dead := map[string]bool{}
	ids := map[string]string{}
	for _, p := range panes {
		if p.Agent != "" {
			byAgent[p.Agent] = true
			dead[p.Agent] = p.Dead
			ids[p.Agent] = p.ID
		}
	}
	now := time.Now()
	var out []*board.Agent
	err := f.Store.Update(func(b *board.Board) error {
		for _, a := range b.Agents {
			prev := a.Status
			next := f.classify(a, byAgent[a.Name], dead[a.Name], ids[a.Name], now)
			if next != prev {
				a.Status, a.StatusSince = next, now
				logx.For("status").Info("transition", "agent", a.Name, "from", prev, "to", next, "line", a.LastLine)
				f.Store.AppendEvent(board.Event{Time: now, Agent: a.Name, From: prev, To: next})
				f.tagPane(ids[a.Name], next)
			}
		}
		out = b.Sorted()
		return nil
	})
	return out, err
}

func (f *Fleet) classify(a *board.Agent, alive, dead bool, pane string, now time.Time) string {
	if !alive {
		return string(status.Gone)
	}
	a.PaneID = pane
	text, err := f.T.Capture(pane, 0)
	if err != nil {
		return string(status.Unknown)
	}
	if dead {
		a.LastLine = "process exited"
		return string(status.Error)
	}
	h := fnv.New64a()
	h.Write([]byte(text))
	sum := h.Sum64()
	changed := sum != a.Hash
	if changed {
		a.Hash, a.LastChange = sum, now
	}
	if t := status.Tail(text, 1); len(t) > 0 {
		a.LastLine = t[0]
	}
	st, ok := f.Patterns[a.Patterns].Detect(text)
	switch {
	case !ok && changed:
		st = status.Working
	case !ok:
		st = status.Status(a.Status)
		if st == "" || st == status.Gone {
			st = status.Unknown
		}
	}
	if (st == status.Working || st == status.Stuck) && now.Sub(a.LastChange) > StuckAfter {
		st = status.Stuck
	} else if st == status.Stuck {
		st = status.Working
	}
	return string(st)
}

var paneColors = map[string]string{"working": "#3fb950", "waiting": "#d29922", "error": "#f85149", "stuck": "#f85149", "idle": "#8b949e"}
var paneGlyphs = map[string]string{"working": "⠹", "waiting": "●", "error": "✖", "stuck": "⧗", "idle": "○"}

// tagPane stores the status on the pane so tmux border formats can show it.
func (f *Fleet) tagPane(pane, st string) {
	if pane == "" {
		return
	}
	c, ok := paneColors[st]
	if !ok {
		c = "#8b949e"
	}
	g, ok := paneGlyphs[st]
	if !ok {
		g = "?"
	}
	f.T.SetPaneOption(pane, "@fleet_status", st)
	f.T.SetPaneOption(pane, "@fleet_color", c)
	f.T.SetPaneOption(pane, "@fleet_glyph", g)
}
