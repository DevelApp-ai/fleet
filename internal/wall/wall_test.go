package wall

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/tmux"
)

// fromOutsideTheWall makes the last-focused pane a non-wall pane (I came from
// the supervisor), so Show takes the class/LRU fallback instead of reusing the
// slot I just left (DESIGN §5b.1).
func fromOutsideTheWall(t *testing.T, c *tmux.Client, w *Wall) {
	t.Helper()
	s, err := w.Read()
	if err != nil {
		t.Fatal(err)
	}
	c.Run("select-pane", "-t", s.Sup)
	c.Run("select-pane", "-t", s.Ov)
}

func TestFiveOntoFourSlots(t *testing.T) {
	c := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { c.KillServer() })
	sup, err := c.Run("new-session", "-d", "-s", "f", "-n", "main", "-x", "300", "-y", "80", "-P", "-F", "#{pane_id}", "cat")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	ov, _ := c.Run("split-window", "-d", "-h", "-t", "f:main", "-P", "-F", "#{pane_id}", "cat")
	c.SetPaneOption(sup, "@role", "sup")
	c.SetPaneOption(ov, "@role", "ov")
	var agents []string
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("a%d", i)
		p, _ := c.Spawn("f", "ag-"+name, "", nil, "cat")
		c.SetPaneOption(p, "@fleet_agent", name)
		agents = append(agents, p)
	}
	w := New(c, "f")
	for _, p := range agents {
		fromOutsideTheWall(t, c, w)
		if _, err := w.Show(p); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	on := w.OnWall()
	if len(on) != 4 || on["a0"] {
		t.Fatalf("on wall: %v (a0 should be evicted)", on)
	}
	s, _ := w.Read()
	if s.Sup == "" || s.Ov == "" || len(s.Wall) != 4 {
		t.Fatalf("state %+v", s)
	}
	if w, _ := c.Run("display", "-p", "-t", sup, "#{pane_width}"); w != "90" {
		t.Errorf("sup width %s", w)
	}
	if err := w.SetPreset("1"); err != nil {
		t.Fatal(err)
	}
	if on := w.OnWall(); len(on) != 1 || !on["a4"] {
		t.Errorf("preset 1 kept %v, want most recent a4", on)
	}
	if err := w.SetPreset("4"); err != nil {
		t.Fatal(err)
	}
	if on := w.OnWall(); len(on) != 4 || !on["a4"] || !on["a3"] || on["a0"] {
		t.Errorf("preset 4 after 1 = %v, want a4 plus 3 most recent hidden (not a0)", on)
	}
	if err := w.ToggleSup(); err != nil {
		t.Fatal(err)
	}
	if s, _ := w.Read(); s.Sup != "" {
		t.Error("sup still visible")
	}
	if err := w.ToggleSup(); err != nil {
		t.Fatal(err)
	}
	if s, _ := w.Read(); s.Sup == "" {
		t.Error("sup not restored")
	}
}

func TestHideFocused(t *testing.T) {
	c := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { c.KillServer() })
	ov, err := c.Run("new-session", "-d", "-s", "f", "-n", "main", "-x", "200", "-y", "50", "-P", "-F", "#{pane_id}", "cat")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	c.SetPaneOption(ov, "@role", "ov")
	p, _ := c.Spawn("f", "ag-a", "", nil, "cat")
	c.SetPaneOption(p, "@fleet_agent", "a")
	w := New(c, "f")
	w.Show(p) // focuses p
	if err := w.HideFocused(); err != nil || len(w.OnWall()) != 0 {
		t.Fatalf("not hidden: %v %v", err, w.OnWall())
	}
	c.Run("select-pane", "-t", ov)
	if err := w.HideFocused(); err != nil {
		t.Fatal(err)
	}
	if s, _ := w.Read(); s.Ov != ov {
		t.Error("overview must stay")
	}
}

// setupWall makes a main window with a supervisor, an overview and n agent
// panes in hidden windows, on a throwaway socket.
func setupWall(t *testing.T, n int) (*tmux.Client, *Wall, []string) {
	t.Helper()
	c := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { c.KillServer() })
	sup, err := c.Run("new-session", "-d", "-s", "f", "-n", "main", "-x", "300", "-y", "80", "-P", "-F", "#{pane_id}", "cat")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	ov, _ := c.Run("split-window", "-d", "-h", "-t", "f:main", "-P", "-F", "#{pane_id}", "cat")
	c.SetPaneOption(sup, "@role", "sup")
	c.SetPaneOption(ov, "@role", "ov")
	var agents []string
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("a%d", i)
		p, _ := c.Spawn("f", "ag-"+name, "", nil, "cat")
		c.SetPaneOption(p, "@fleet_agent", name)
		c.SetPaneOption(p, "@fleet_status", "idle")
		agents = append(agents, p)
	}
	return c, New(c, "f"), agents
}

// The waiting pane stays on the wall; an idle one is evicted instead.
func TestEvictionKeepsWaitingPaneOnTheWall(t *testing.T) {
	c, w, agents := setupWall(t, 5)
	c.SetPaneOption(agents[0], "@fleet_status", "waiting")
	c.SetPaneOption(agents[0], "@fleet_status_since", "1")
	for _, p := range agents {
		fromOutsideTheWall(t, c, w)
		if _, err := w.Show(p); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	on := w.OnWall()
	if len(on) != 4 || !on["a0"] || on["a1"] {
		t.Fatalf("on wall: %v (a0 waiting must stay, idle a1 must go)", on)
	}
}

// Nothing is evictable: Show must leave the wall alone and return nil.
func TestShowRefusesWhenEveryPaneNeedsAttention(t *testing.T) {
	c, w, agents := setupWall(t, 5)
	for _, p := range agents[:4] {
		if _, err := w.Show(p); err != nil {
			t.Fatal(err)
		}
		c.SetPaneOption(p, "@fleet_status", "waiting")
		time.Sleep(2 * time.Millisecond)
	}
	fromOutsideTheWall(t, c, w)
	before := w.OnWall()
	ok, err := w.Show(agents[4])
	if err != nil {
		t.Fatalf("a full, all-waiting wall is a normal outcome: %v", err)
	}
	// Finding 4: a refusal must be visible to the caller, not reported as success.
	if ok {
		t.Error("Show reported success for a pane it refused to join")
	}
	after := w.OnWall()
	if len(after) != 4 || after["a4"] {
		t.Fatalf("wall changed: %v -> %v", before, after)
	}
}

// Finding 3: `mouse on` lets the user drag borders, so the real layout drifts
// from the cached @fleet_layout. An explicit apply must force a real
// select-layout (and the supervisor resize that follows it); only the
// refresh-driven ApplyCached may trust the cache.
func TestApplyForcesThroughADriftedCache(t *testing.T) {
	c, w, agents := setupWall(t, 3)
	for _, p := range agents {
		if _, err := w.Show(p); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if err := w.Apply(); err != nil {
		t.Fatal(err)
	}
	if w.opt("@fleet_layout") == "" {
		t.Fatal("no cached layout after apply")
	}
	s, _ := w.Read()
	supW := func() string {
		out, _ := c.Run("display-message", "-p", "-t", s.Sup, "#{pane_width}")
		return strings.TrimSpace(out)
	}
	want := supW()
	// The user drags the supervisor border; the rendered layout is unchanged,
	// so @fleet_layout still matches and a cached apply is a no-op.
	c.Run("resize-pane", "-t", s.Sup, "-x", "60")
	if supW() == want {
		t.Skip("tmux did not drift the layout")
	}
	if err := w.ApplyCached(); err != nil {
		t.Fatal(err)
	}
	if supW() == want {
		t.Error("ApplyCached re-applied despite an unchanged cached layout")
	}
	// The explicit apply forces through and un-wedges the wall.
	if err := w.Apply(); err != nil {
		t.Fatal(err)
	}
	if got := supW(); got != want {
		t.Errorf("Apply did not force through the drifted cache: sup width %s, want %s", got, want)
	}
}

// slotOf returns the wall panes of the main window as "id WxH" in order.
func slots(t *testing.T, c *tmux.Client) []string {
	t.Helper()
	ps, err := c.ListPanes("f:main")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range ps {
		if p.Role == "wall" {
			out = append(out, fmt.Sprintf("%s %dx%d", p.ID, p.Width, p.Height))
		}
	}
	return out
}

// fillWall shows the first four agents and leaves the wall full.
func fillWall(t *testing.T, w *Wall, agents []string) {
	t.Helper()
	for _, p := range agents[:4] {
		if ok, err := w.Show(p); err != nil || !ok {
			t.Fatalf("show %s: %v %v", p, ok, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// DESIGN §5b.1: the wall pane I was last in is the slot the incoming pane
// takes, in place — same position, same size, nothing else moves.
func TestShowTakesTheSlotJustLeft(t *testing.T) {
	c, w, agents := setupWall(t, 5)
	fillWall(t, w, agents)
	s, _ := w.Read()
	x := agents[1]
	c.Run("select-pane", "-t", x)    // I was in a1 ...
	c.Run("select-pane", "-t", s.Ov) // ... then went to the overview and pressed Enter
	before := slots(t, c)
	idx := -1
	for i, sl := range before {
		if strings.HasPrefix(sl, x+" ") {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("%s not on the wall: %v", x, before)
	}
	ok, err := w.Show(agents[4])
	if err != nil || !ok {
		t.Fatalf("show: %v %v", ok, err)
	}
	after := slots(t, c)
	if len(after) != len(before) {
		t.Fatalf("wall size changed: %v -> %v", before, after)
	}
	// The incoming pane sits in exactly the slot a1 had.
	wantSlot := strings.Replace(before[idx], x+" ", agents[4]+" ", 1)
	if after[idx] != wantSlot {
		t.Errorf("slot %d is %q, want %q", idx, after[idx], wantSlot)
	}
	// Nothing else moved or resized.
	for i := range before {
		if i != idx && before[i] != after[i] {
			t.Errorf("slot %d moved: %q -> %q", i, before[i], after[i])
		}
	}
	// a1 went back to its own window, named after the agent.
	if on := w.OnWall(); on["a1"] || !on["a4"] {
		t.Errorf("on wall: %v", on)
	}
	win, _ := c.Run("display-message", "-p", "-t", x, "#{window_name}")
	if strings.TrimSpace(win) != "ag-a1" {
		t.Errorf("replaced pane landed in window %q, want ag-a1", win)
	}
	if strings.TrimSpace(win) == "main" {
		t.Error("replaced pane is stranded in the main window")
	}
	ps, _ := c.ListPanes("f:main")
	for _, p := range ps {
		if p.ID == x {
			t.Error("replaced pane is still in the main window")
		}
	}
}

// An explicit Enter beats class-1 protection: a waiting pane I just left is
// still the slot that gets reused.
func TestShowTakesTheSlotEvenWhenItIsWaiting(t *testing.T) {
	c, w, agents := setupWall(t, 5)
	fillWall(t, w, agents)
	s, _ := w.Read()
	x := agents[1]
	c.SetPaneOption(x, "@fleet_status", "waiting")
	c.SetPaneOption(x, "@fleet_status_since", "1")
	c.Run("select-pane", "-t", x)
	c.Run("select-pane", "-t", s.Ov)
	if ok, err := w.Show(agents[4]); err != nil || !ok {
		t.Fatalf("show: %v %v", ok, err)
	}
	if on := w.OnWall(); on["a1"] || !on["a4"] {
		t.Errorf("a waiting pane I just left must still be replaced: %v", on)
	}
}

// A pinned last pane is never replaced: class 0 wins and the class/LRU
// fallback picks an evictable pane instead.
func TestShowDoesNotTakeAPinnedSlot(t *testing.T) {
	c, w, agents := setupWall(t, 5)
	fillWall(t, w, agents)
	s, _ := w.Read()
	x := agents[1]
	c.SetPaneOption(x, "@fleet_pin", "1")
	c.Run("select-pane", "-t", x)
	c.Run("select-pane", "-t", s.Ov)
	if ok, err := w.Show(agents[4]); err != nil || !ok {
		t.Fatalf("show: %v %v", ok, err)
	}
	on := w.OnWall()
	if !on["a1"] {
		t.Error("pinned pane was replaced")
	}
	if !on["a4"] || len(on) != 4 {
		t.Errorf("fallback eviction did not run: %v", on)
	}
	if !on["a2"] || !on["a3"] {
		t.Errorf("fallback took the wrong pane (want LRU a0): %v", on)
	}
}

// A re-layout must move neither the user nor #{pane_last}: emphasis falls
// back to the active pane and "take the slot I just left" (§5b.1) reads
// pane_last, so an apply that rewrote either would change what the next Enter
// replaces.
func TestApplyLeavesFocusAndLastPaneAlone(t *testing.T) {
	c, w, agents := setupWall(t, 3)
	fromOutsideTheWall(t, c, w)
	for _, p := range agents {
		if _, err := w.Show(p); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	s, _ := w.Read()
	// I was in a wall pane, then went to the overview to pick the next agent.
	c.Run("select-pane", "-t", s.Wall[0].ID)
	c.Run("select-pane", "-t", s.Ov)
	active := func() string {
		out, _ := c.Run("display-message", "-p", "-t", "f:main", "#{pane_id}")
		return strings.TrimSpace(out)
	}
	last := func() string {
		ps, _ := c.ListPanes("f:main")
		for _, p := range ps {
			if p.Last {
				return p.ID
			}
		}
		return ""
	}
	wantActive, wantLast := active(), last()
	if wantLast != s.Wall[0].ID {
		t.Fatalf("setup: pane_last is %s, want the wall pane %s", wantLast, s.Wall[0].ID)
	}
	for _, apply := range []func() error{w.Apply, w.ApplyCached, w.Apply} {
		if err := apply(); err != nil {
			t.Fatal(err)
		}
		if got := active(); got != wantActive {
			t.Fatalf("apply moved the user: active %s, want %s", got, wantActive)
		}
		if got := last(); got != wantLast {
			t.Fatalf("apply rewrote pane_last: %s, want %s", got, wantLast)
		}
	}
}

// Two applies racing from independent snapshots would scramble pane order, so
// the second one backs off while the first holds the lock.
func TestApplyLockKeepsConcurrentAppliesOut(t *testing.T) {
	c, w, agents := setupWall(t, 3)
	for _, p := range agents {
		if _, err := w.Show(p); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if err := w.Apply(); err != nil {
		t.Fatal(err)
	}
	s, _ := w.Read()
	// Another process is mid-apply.
	w.setOpt("@fleet_applying", strconv.FormatInt(time.Now().Add(applyLockTTL).UnixNano(), 10))
	c.Run("resize-pane", "-t", s.Sup, "-x", "60")
	supW := func() string {
		out, _ := c.Run("display-message", "-p", "-t", s.Sup, "#{pane_width}")
		return strings.TrimSpace(out)
	}
	if supW() != "60" {
		t.Skip("tmux refused the drag")
	}
	if err := w.Apply(); err != nil {
		t.Fatal(err)
	}
	if supW() != "60" {
		t.Error("apply ran while another process held the lock")
	}
	// A stale lock (a crashed apply) must not block the wall forever.
	w.setOpt("@fleet_applying", strconv.FormatInt(time.Now().Add(-time.Minute).UnixNano(), 10))
	if err := w.Apply(); err != nil {
		t.Fatal(err)
	}
	if supW() == "60" {
		t.Error("a stale lock wedged the wall")
	}
}
