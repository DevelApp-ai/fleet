package fleet

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/tmux"
)

func TestSpawnLsSendClose(t *testing.T) {
	home := t.TempDir()
	tc := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { tc.KillServer() })
	// A profile whose pane prints a y/n prompt, using cat to echo input.
	os.WriteFile(filepath.Join(home, "tools.toml"), []byte(`[yn]
command = "sh"
args = ["-c", "printf 'Approve? [y/n] '; cat"]
patterns = "fake"
`), 0o644)
	f, err := New(tc, home, "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Spawn("a1", "yn", home, ""); err != nil {
		t.Fatal(err)
	}
	var st string
	for i := 0; i < 40; i++ {
		as, _ := f.Refresh()
		if len(as) == 1 && as[0].Status == "waiting" {
			st = as[0].Status
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if st != "waiting" {
		t.Fatalf("status never waiting")
	}
	if err := f.Send("a1", "hello"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if out, _ := f.Peek("a1", 0); !strings.Contains(out, "hello") {
		t.Errorf("peek: %q", out)
	}
	if err := f.Close("a1"); err != nil {
		t.Fatal(err)
	}
	if len(f.Store.Load().Agents) != 0 {
		t.Error("close left board entry")
	}
}
