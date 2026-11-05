package wall

import (
	"sort"

	"github.com/sturlabragason/fleet/internal/tmux"
)

// Eviction classes, best eviction candidate last (DESIGN.md §5b.1, ADR-14).
const (
	classPinned    = 0 // @fleet_pin = 1: never evicted
	classAttention = 1 // waiting/error/stuck: never evicted
	classWorking   = 2 // evicted only when nothing idle is left
	classIdle      = 3 // idle/gone/unknown/untagged: goes first
)

// needsAttention reports whether a pane is asking for me or has failed.
func needsAttention(st string) bool {
	switch st {
	case "waiting", "error", "stuck":
		return true
	}
	return false
}

// evictClass classifies one wall pane.
func evictClass(p tmux.PaneInfo) int {
	switch {
	case p.Pin:
		return classPinned
	case needsAttention(p.Status):
		return classAttention
	case p.Status == "working":
		return classWorking
	}
	return classIdle
}

// ReplaceSlot returns the wall pane whose slot an incoming pane should take:
// the pane the user was last in (#{pane_last}), because the usual path is
// wall pane -> overview -> Enter, so that slot is the one they meant to reuse
// (DESIGN.md §5b.1). Status does not matter — an explicit Enter beats class-1
// protection — but a pinned pane is never replaced, and there is nothing to
// reuse when focus came from the supervisor or never left the overview.
func ReplaceSlot(ps []tmux.PaneInfo) (string, bool) {
	for _, p := range ps {
		if p.Last && p.Role == "wall" && !p.Pin {
			return p.ID, true
		}
	}
	return "", false
}

// EvictCandidate returns the wall pane that should leave: the least recently
// used pane (@fleet_used) of the worst class present. ok is false when every
// pane is protected (pinned, waiting, error or stuck).
func EvictCandidate(ps []tmux.PaneInfo) (string, bool) {
	c := append([]tmux.PaneInfo(nil), ps...)
	sort.SliceStable(c, func(i, j int) bool {
		ci, cj := evictClass(c[i]), evictClass(c[j])
		if ci != cj {
			return ci > cj // worst class first
		}
		return c[i].Used < c[j].Used
	})
	if len(c) == 0 || evictClass(c[0]) <= classAttention {
		return "", false
	}
	return c[0].ID, true
}
