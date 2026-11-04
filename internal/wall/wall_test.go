package wall

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/tmux"
)

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
		if err := w.Show(p); err != nil {
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
