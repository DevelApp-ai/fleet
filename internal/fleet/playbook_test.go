package fleet

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/playbook"
	"github.com/sturlabragason/fleet/internal/tmux"
	"github.com/sturlabragason/fleet/internal/tools"
)

// argvFleet builds a Fleet on a fleet-test-* socket with a profile "pa" whose
// pane writes its prompt argument to <home>/argv.txt and then idles. config is
// the FLEET_CONFIG body ("" = no file).
func argvFleet(t *testing.T, config string) (*Fleet, string) {
	t.Helper()
	home := t.TempDir()
	tc := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { tc.KillServer() })
	out := filepath.Join(home, "argv.txt")
	os.WriteFile(filepath.Join(home, "tools.toml"), []byte(`[pa]
command = "sh"
args = ["-c", "printf '%s' \"$1\" > `+out+`; cat", "sh"]
prompt_args = ["{prompt}"]
patterns = "fake"
`), 0o644)
	cfg := filepath.Join(home, "no-config.toml")
	if config != "" {
		cfg = filepath.Join(home, "config.toml")
		os.WriteFile(cfg, []byte(config), 0o644)
	}
	t.Setenv("FLEET_CONFIG", cfg)
	f, err := New(tc, home, "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	return f, out
}

func readArg(t *testing.T, path string) string {
	t.Helper()
	for i := 0; i < 50; i++ {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return string(b)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("pane never wrote %s", path)
	return ""
}

func TestSpawnPrependsPlaybook(t *testing.T) {
	f, out := argvFleet(t, "")
	dir := t.TempDir() // no .fleet/worker.md here, and f.Home has no worker.md
	if _, err := f.Spawn("pb1", "pa", dir, "fix the bug"); err != nil {
		t.Fatal(err)
	}
	got := readArg(t, out)
	want := playbook.Compose(playbook.Default(), "fix the bug")
	if got != want {
		t.Fatalf("launch prompt:\n%.200q\nwant\n%.200q", got, want)
	}
	if !strings.HasPrefix(got, "# Fleet worker playbook") || !strings.HasSuffix(got, "## Your task\n\nfix the bug") {
		t.Fatalf("playbook not before the task: %.80q", got)
	}
	if g := f.Store.Load().Agents["pb1"].Goal; g != "fix the bug" {
		t.Errorf("board goal %q, want the user's prompt", g)
	}
}

func TestSpawnPlaybookOff(t *testing.T) {
	f, out := argvFleet(t, "[defaults]\nplaybook = false\n")
	if _, err := f.Spawn("pb2", "pa", t.TempDir(), "fix the bug"); err != nil {
		t.Fatal(err)
	}
	if got := readArg(t, out); got != "fix the bug" {
		t.Fatalf("launch prompt %q, want only the prompt", got)
	}
}

// The logged argv hides the whole composed prompt, and the byte count is the
// composed length (not the user's prompt length).
func TestSpawnPlaybookLogRedacted(t *testing.T) {
	logHome := t.TempDir()
	t.Setenv("FLEET_HOME", logHome)
	logx.Init() // first Init in this test binary, so the log lands in logHome
	logFile := filepath.Join(logHome, "logs", "fleet.log")
	if _, err := os.Stat(logFile); err != nil {
		t.Skip("logx already initialised elsewhere in this test binary")
	}
	f, out := argvFleet(t, "")
	if _, err := f.Spawn("pb3", "pa", t.TempDir(), "fix the bug"); err != nil {
		t.Fatal(err)
	}
	full := readArg(t, out)
	b, _ := os.ReadFile(logFile)
	var line string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "msg=started") && strings.Contains(l, "agent=pb3") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("no started line in log:\n%s", b)
	}
	if strings.Contains(line, "worker playbook") || strings.Contains(line, "fix the bug") {
		t.Fatalf("prompt leaked into the log: %s", line)
	}
	if want := fmt.Sprintf("<prompt:%d bytes>", len(full)); !strings.Contains(line, want) {
		t.Fatalf("log line lacks %s: %s", want, line)
	}
}

// A prompt too long for tmux new-window falls back to the paste path.
func TestLaunchArgvLongPromptPastes(t *testing.T) {
	f, _ := argvFleet(t, "")
	p := f.Profiles["pa"]
	v := tools.AgentVars("/nonexistent", "x", "")
	argv, embedded := launchArgv("x", p, v, tools.Launch{Prompt: "short"})
	if !embedded || argv[len(argv)-1] != "short" {
		t.Fatalf("short prompt not embedded: %q", argv)
	}
	long := strings.Repeat("y", MaxLaunchCmd+1)
	argv, embedded = launchArgv("x", p, v, tools.Launch{Prompt: long})
	if embedded {
		t.Fatal("long prompt still embedded")
	}
	if n := len(tools.ShellCommand(argv)); n > MaxLaunchCmd {
		t.Fatalf("launch command %d bytes", n)
	}
}

// End to end: the long prompt reaches the pane via paste and spawn succeeds.
func TestSpawnLongPromptViaPaste(t *testing.T) {
	f, _ := argvFleet(t, "[defaults]\nplaybook = false\nready_timeout = \"3s\"\n")
	long := strings.Repeat("z", 20000)
	if _, err := f.Spawn("pb4", "pa", t.TempDir(), long); err != nil {
		t.Fatalf("spawn with a 20KB prompt: %v", err)
	}
}
