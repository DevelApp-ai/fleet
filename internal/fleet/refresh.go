package fleet

import (
	"hash/fnv"
	"strconv"
	"time"

	"github.com/sturlabragason/fleet/internal/board"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/status"
	"github.com/sturlabragason/fleet/internal/theme"
)

// Refresh captures every agent pane, updates statuses on the board, logs
// transitions and returns the agents sorted by name.
func (f *Fleet) Refresh() ([]*board.Agent, error) {
	panes, _ := f.T.ListPanes("")
	byAgent := map[string]bool{}
	dead := map[string]bool{}
	ids := map[string]string{}
	tagged := map[string]string{}
	for _, p := range panes {
		if p.Agent != "" {
			byAgent[p.Agent] = true
			dead[p.Agent] = p.Dead
			ids[p.Agent] = p.ID
			tagged[p.Agent] = p.Status
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
				f.tagPane(ids[a.Name], next, now)
			} else if id := ids[a.Name]; id != "" && tagged[a.Name] != next {
				// The pane title lags the board (pane re-created, tag lost, or
				// set by an older fleet): re-sync without resetting the age.
				f.tagPane(id, next, a.StatusSince)
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
	st, line, ok := f.Patterns[a.Patterns].Match(text)
	if ok && (st == status.Waiting || st == status.Error) && line != "" {
		// waiting_for: the prompt, not the footer under it.
		a.LastLine = line
	}
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

var paneGlyphs = map[string]string{"working": "⠹", "waiting": "●", "error": "✖", "stuck": "⧗", "idle": "○"}

// tagPane stores the status on the pane so tmux border formats can show it and
// the wall can evict and emphasise by status. since is the time the agent
// entered this status (now on a transition, the board's StatusSince on a re-sync).
func (f *Fleet) tagPane(pane, st string, since time.Time) {
	if pane == "" {
		return
	}
	c := theme.Status(st)
	g, ok := paneGlyphs[st]
	if !ok {
		g = "?"
	}
	f.T.SetPaneOption(pane, "@fleet_status", st)
	f.T.SetPaneOption(pane, "@fleet_status_since", strconv.FormatInt(since.UnixNano(), 10))
	f.T.SetPaneOption(pane, "@fleet_color", c)
	f.T.SetPaneOption(pane, "@fleet_glyph", g)
}
