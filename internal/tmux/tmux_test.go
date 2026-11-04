package tmux

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// testClient returns a client on a throwaway socket, killed at cleanup.
func testClient(t *testing.T) *Client {
	t.Helper()
	c := &Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer() })
	return c
}

func TestSpawnPasteCapture(t *testing.T) {
	c := testClient(t)
	p, err := c.Spawn("s", "w1", "", nil, "cat")
	if err != nil || !strings.HasPrefix(p, "%") {
		t.Fatalf("spawn: %q %v", p, err)
	}
	if err := c.SetPaneOption(p, "@fleet_agent", "a1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Paste(p, "hello fleet", true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		out, _ := c.Capture(p, 0)
		if strings.Count(out, "hello fleet") >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("capture: %q", out)
		}
		time.Sleep(50 * time.Millisecond)
	}
	ps, err := c.ListPanes("")
	if err != nil || len(ps) != 1 || ps[0].Agent != "a1" || ps[0].Window != "w1" {
		t.Fatalf("list: %+v %v", ps, err)
	}
	p2, _ := c.Spawn("s", "w2", "", nil, "cat")
	if _, err := c.Run("join-pane", "-d", "-s", p2, "-t", p); err != nil {
		t.Fatal(err)
	}
	ps, _ = c.ListPanes(p)
	if len(ps) != 2 {
		t.Fatalf("join: %+v", ps)
	}
}
