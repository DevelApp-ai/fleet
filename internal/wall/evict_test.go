package wall

import (
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/tmux"
)

func pane(id, st string, used int64) tmux.PaneInfo {
	return tmux.PaneInfo{ID: id, Status: st, Used: used}
}

func TestEvictIdleBeforeWorking(t *testing.T) {
	ps := []tmux.PaneInfo{pane("%1", "working", 1), pane("%2", "idle", 9)}
	if got, ok := EvictCandidate(ps); !ok || got != "%2" {
		t.Errorf("got %q %v, want %%2", got, ok)
	}
}

func TestEvictWorkingOnlyWhenNoIdleLeft(t *testing.T) {
	ps := []tmux.PaneInfo{pane("%1", "working", 5), pane("%2", "working", 2), pane("%3", "waiting", 1)}
	if got, ok := EvictCandidate(ps); !ok || got != "%2" {
		t.Errorf("got %q %v, want the LRU working pane %%2", got, ok)
	}
}

func TestEvictNeverTakesAttentionOrPinned(t *testing.T) {
	for _, st := range []string{"waiting", "error", "stuck"} {
		ps := []tmux.PaneInfo{pane("%1", st, 1), pane("%2", "idle", 9)}
		if got, _ := EvictCandidate(ps); got != "%2" {
			t.Errorf("status %s: evicted %q", st, got)
		}
	}
	ps := []tmux.PaneInfo{{ID: "%1", Status: "idle", Used: 1, Pin: true}, pane("%2", "working", 9)}
	if got, ok := EvictCandidate(ps); !ok || got != "%2" {
		t.Errorf("pinned pane evicted: %q %v", got, ok)
	}
}

func TestEvictLRUWithinClass(t *testing.T) {
	ps := []tmux.PaneInfo{pane("%1", "", 30), pane("%2", "unknown", 10), pane("%3", "idle", 20)}
	if got, _ := EvictCandidate(ps); got != "%2" {
		t.Errorf("got %q, want the least recently used %%2", got)
	}
}

func TestEvictAllProtected(t *testing.T) {
	ps := []tmux.PaneInfo{pane("%1", "waiting", 1), pane("%2", "error", 2), {ID: "%3", Pin: true}}
	if got, ok := EvictCandidate(ps); ok {
		t.Errorf("want ok=false, got %q", got)
	}
	if _, ok := EvictCandidate(nil); ok {
		t.Error("empty wall must not be evictable")
	}
}

func TestReplaceSlotPicksTheLastFocusedWallPane(t *testing.T) {
	ps := []tmux.PaneInfo{
		{ID: "%1", Role: "wall", Status: "idle"},
		{ID: "%2", Role: "wall", Status: "waiting", Last: true},
		{ID: "%3", Role: "wall", Status: "idle"},
	}
	// An explicit Enter beats class-1 protection.
	if got, ok := ReplaceSlot(ps); !ok || got != "%2" {
		t.Errorf("got %q %v, want %%2", got, ok)
	}
	// Pinned panes are never replaced, not even explicitly.
	ps[1].Pin = true
	if got, ok := ReplaceSlot(ps); ok {
		t.Errorf("pinned slot reused: %q", got)
	}
	// Focus came from the supervisor, or never left the overview.
	ps[1].Pin, ps[1].Last = false, false
	if got, ok := ReplaceSlot(ps); ok {
		t.Errorf("no last pane, got %q", got)
	}
	// The last pane is not on the wall (supervisor/overview).
	if got, ok := ReplaceSlot([]tmux.PaneInfo{{ID: "%9", Role: "ov", Last: true}}); ok {
		t.Errorf("non-wall slot reused: %q", got)
	}
}

// --- emphasis hysteresis -------------------------------------------------

// choose is chooseEmphasis for the common case: the cached choice was not an
// explicit focus, and the test only cares about the pane it picks.
func choose(ps []tmux.PaneInfo, active, prev, cur string, at, now time.Time) string {
	pick, _ := chooseEmphasis(ps, active, prev, cur, false, at, now)
	return pick
}

func waiting(id string, since time.Time) tmux.PaneInfo {
	return tmux.PaneInfo{ID: id, Status: "waiting", StatusSince: since.UnixNano()}
}

func TestEmphasisPicksLongestWaiting(t *testing.T) {
	now := time.Unix(1000, 0)
	ps := []tmux.PaneInfo{
		{ID: "%1", Status: "working"},
		waiting("%2", now.Add(-20*time.Second)),
		waiting("%3", now.Add(-60*time.Second)),
	}
	if got := choose(ps, "%1", "%1", "", time.Time{}, now); got != "%3" {
		t.Errorf("got %q, want the longest-waiting %%3", got)
	}
}

func TestEmphasisFallsBackToActivePane(t *testing.T) {
	now := time.Unix(1000, 0)
	ps := []tmux.PaneInfo{{ID: "%1", Status: "working"}, {ID: "%2", Status: "idle"}}
	if got := choose(ps, "%2", "%2", "", time.Time{}, now); got != "%2" {
		t.Errorf("got %q, want the active pane %%2", got)
	}
	if got := choose(ps, "", "", "", time.Time{}, now); got != "" {
		t.Errorf("got %q, want no emphasis", got)
	}
}

func TestEmphasisHoldsAgainstANewWaitingPane(t *testing.T) {
	now := time.Unix(1000, 0)
	// %2 holds emphasis because it failed; %3 then starts waiting, which ranks
	// higher — but only once the current choice has held long enough.
	cur := tmux.PaneInfo{ID: "%2", Status: "error", StatusSince: now.Add(-5 * time.Minute).UnixNano()}
	fresh := waiting("%3", now.Add(-EmphasisDwell-time.Second))
	ps := []tmux.PaneInfo{{ID: "%1", Status: "working"}, cur, fresh}
	at := now.Add(-EmphasisHold + 5*time.Second)
	if got := choose(ps, "%1", "%1", "%2", at, now); got != "%2" {
		t.Errorf("stole emphasis before the hold: %q", got)
	}
	at = now.Add(-EmphasisHold - time.Second)
	if got := choose(ps, "%1", "%1", "%2", at, now); got != "%3" {
		t.Errorf("after the hold the waiting pane wins, got %q", got)
	}
}

func TestEmphasisDwellMustPass(t *testing.T) {
	now := time.Unix(1000, 0)
	ps := []tmux.PaneInfo{
		{ID: "%1", Status: "working"},
		waiting("%2", now.Add(-5*time.Minute)),
		waiting("%3", now.Add(-time.Second)), // waiting, but not yet for Dwell
	}
	at := now.Add(-time.Hour)
	if got := choose(ps, "%1", "%1", "%2", at, now); got != "%2" {
		t.Errorf("a pane waiting < dwell must not win: %q", got)
	}
}

func TestEmphasisExplicitFocusWinsImmediately(t *testing.T) {
	now := time.Unix(1000, 0)
	ps := []tmux.PaneInfo{{ID: "%1", Status: "working"}, waiting("%2", now.Add(-time.Hour))}
	if got := choose(ps, "%1", "%2", "%2", now, now); got != "%1" {
		t.Errorf("got %q, want the just-focused pane %%1", got)
	}
}

func TestEmphasisFocusHeldAgainstCandidates(t *testing.T) {
	now := time.Unix(1000, 0)
	ps := []tmux.PaneInfo{{ID: "%1", Status: "working"}, waiting("%2", now.Add(-time.Hour))}
	at := now.Add(-EmphasisHold - time.Second) // past the normal hold...
	if got := choose(ps, "%1", "%1", "%1", at, now); got != "%1" {
		t.Errorf("focused pane must hold for %s, got %q", EmphasisFocusHold, got)
	}
	at = now.Add(-EmphasisFocusHold - time.Second)
	if got := choose(ps, "%1", "%1", "%1", at, now); got != "%2" {
		t.Errorf("after the focus hold the waiting pane wins, got %q", got)
	}
	// ... and the hold survives looking away: I focus %1, then click the
	// supervisor, which is not a wall pane at all. %1 must keep emphasis.
	at = now.Add(-5 * time.Second)
	pick, byFocus := chooseEmphasis(ps, "%sup", "%sup", "%1", true, at, now)
	if pick != "%1" || !byFocus {
		t.Errorf("looking away dropped the focus hold: got %q focus=%v, want %%1 true", pick, byFocus)
	}
	// Same on the first call after the active pane moved (prev was still %1).
	if pick, _ := chooseEmphasis(ps, "%sup", "%1", "%1", true, at, now); pick != "%1" {
		t.Errorf("got %q, want %%1 to hold", pick)
	}
	// Once the focus hold expires the waiting pane takes over anyway.
	at = now.Add(-EmphasisFocusHold - time.Second)
	if pick, byFocus := chooseEmphasis(ps, "%sup", "%sup", "%1", true, at, now); pick != "%2" || byFocus {
		t.Errorf("after the focus hold: got %q focus=%v, want %%2 false", pick, byFocus)
	}
}

func TestEmphasisMovesWhenItsPaneLeavesTheWall(t *testing.T) {
	now := time.Unix(1000, 0)
	ps := []tmux.PaneInfo{{ID: "%1", Status: "working"}, waiting("%3", now.Add(-time.Second))}
	if got := choose(ps, "%1", "%1", "%9", now, now); got != "%3" {
		t.Errorf("got %q, want an immediate move to %%3", got)
	}
}

func TestEmphasisMovesWhenItStopsNeedingAttention(t *testing.T) {
	now := time.Unix(1000, 0)
	ps := []tmux.PaneInfo{{ID: "%1", Status: "idle"}, {ID: "%2", Status: "working"}, waiting("%3", now)}
	if got := choose(ps, "%2", "%2", "%1", now, now); got != "%3" {
		t.Errorf("got %q, want %%3 once %%1 stopped waiting", got)
	}
}

// An explicit focus is reported so the caller can persist @fleet_emphasis_focus.
func TestEmphasisReportsExplicitFocus(t *testing.T) {
	now := time.Unix(1000, 0)
	ps := []tmux.PaneInfo{{ID: "%1", Status: "working"}, waiting("%2", now.Add(-time.Hour))}
	if pick, byFocus := chooseEmphasis(ps, "%1", "%2", "%2", false, now, now); pick != "%1" || !byFocus {
		t.Errorf("got %q focus=%v, want %%1 true", pick, byFocus)
	}
	if pick, byFocus := chooseEmphasis(ps, "%2", "%2", "", false, time.Time{}, now); pick != "%2" || byFocus {
		t.Errorf("a candidate pick is not an explicit focus: %q %v", pick, byFocus)
	}
}

// Finding 5: a pane tagged by an older build has no @fleet_status_since, so its
// dwell is unknown and must count as not satisfied rather than ~55 years.
func TestEmphasisUntaggedStatusSinceFailsDwell(t *testing.T) {
	now := time.Unix(1000, 0)
	ps := []tmux.PaneInfo{
		{ID: "%1", Status: "error", StatusSince: now.Add(-5 * time.Minute).UnixNano()},
		{ID: "%2", Status: "waiting"}, // StatusSince 0: never tagged
	}
	at := now.Add(-time.Hour) // the hold passed long ago
	if got := choose(ps, "%3", "%3", "%1", at, now); got != "%1" {
		t.Errorf("a pane with no status_since must not steal emphasis, got %q", got)
	}
}

// An untagged pane (an older build set @fleet_status but not _since) must not
// sit at the head of the queue and block every properly dwelled candidate.
func TestEmphasisUntaggedPaneDoesNotBlockTheQueue(t *testing.T) {
	now := time.Unix(1000, 0)
	ps := []tmux.PaneInfo{
		{ID: "%1", Status: "working"},
		{ID: "%2", Status: "waiting"}, // StatusSince 0: never tagged
		waiting("%3", now.Add(-5*time.Minute)),
	}
	at := now.Add(-time.Hour)
	if got := choose(ps, "%1", "%1", "%1", at, now); got != "%3" {
		t.Errorf("got %q, want the dwelled %%3 (the untagged %%2 must not block it)", got)
	}
	if got := choose(ps, "%1", "%1", "", time.Time{}, now); got != "%3" {
		t.Errorf("first choice got %q, want %%3", got)
	}
}
