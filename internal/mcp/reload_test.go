package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/tmux"
)

// reloadServer is an agent-mode server on a temp home (no tmux server is
// started) watching a fake executable.
func reloadServer(t *testing.T) (*Server, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FLEET_CONFIG", filepath.Join(home, "no-config.toml"))
	f, err := fleet.New(tmux.New("fleet-test-reload-unused"), home, "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "fleet")
	if err := os.WriteFile(exe, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := NewAgent(f, "a1")
	s.ExePath = exe
	s.selfExe = exe
	return s, exe
}

// replaceExe swaps in a new file (new inode) the way `make build` does.
func replaceExe(t *testing.T, exe, content string) {
	t.Helper()
	tmp := exe + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		t.Fatal(err)
	}
}

type pipeConn struct {
	in  *io.PipeWriter
	out *bufio.Scanner
	t   *testing.T
}

func startServe(t *testing.T, s *Server) (*pipeConn, chan error) {
	inR, inW := io.Pipe()
	return startServeOn(t, s, inR, inW)
}

// startServeOn serves r (which may wrap inR) and sends via inW.
func startServeOn(t *testing.T, s *Server, r io.Reader, inW *io.PipeWriter) (*pipeConn, chan error) {
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- s.Serve(r, outW); outW.Close() }()
	t.Cleanup(func() { inW.Close() })
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	return &pipeConn{in: inW, out: sc, t: t}, done
}

func (c *pipeConn) send(lines ...string) {
	c.t.Helper()
	if _, err := c.in.Write([]byte(strings.Join(lines, ""))); err != nil {
		c.t.Fatal(err)
	}
}

func (c *pipeConn) next() map[string]any {
	c.t.Helper()
	if !c.out.Scan() {
		c.t.Fatal("no output")
	}
	var m map[string]any
	if err := json.Unmarshal(c.out.Bytes(), &m); err != nil {
		c.t.Fatalf("bad output %q: %v", c.out.Text(), err)
	}
	return m
}

func ping(id int) string {
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"ping"}`+"\n", id)
}

func TestReloadExecsWithPendingInput(t *testing.T) {
	s, exe := reloadServer(t)
	var probed []string
	type execCall struct {
		path, pendingPath string
		argv, env         []string
		pending           []byte
	}
	execs := make(chan execCall, 1)
	s.Probe = func(p string) error { probed = append(probed, p); return nil }
	s.Exec = func(p string, argv, env []string) error {
		c := execCall{path: p, argv: argv, env: env}
		for _, e := range env {
			if v, ok := strings.CutPrefix(e, envPending+"="); ok {
				c.pendingPath = v
				c.pending, _ = os.ReadFile(v)
			}
		}
		execs <- c
		return nil
	}
	c, done := startServe(t, s)
	c.send(ping(1))
	if r := c.next(); r["id"].(float64) != 1 {
		t.Fatalf("first %v", r)
	}
	if len(probed) != 0 {
		t.Fatal("probed before any change")
	}
	replaceExe(t, exe, "v2")
	// One write: the server's reader takes all three lines in one read, so
	// lines 3 and 4 are read-ahead when line 2 triggers the reload.
	rest := ping(2) + `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" + ping(4)
	c.send(rest)
	got := <-execs
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(probed) != 1 || probed[0] != exe || got.path != exe {
		t.Fatalf("probe %v exec %q", probed, got.path)
	}
	if strings.Join(got.argv, " ") != strings.Join(os.Args, " ") {
		t.Fatalf("argv %v", got.argv)
	}
	env := strings.Join(got.env, "\n")
	if !strings.Contains(env, envReloaded+"=1") || !strings.Contains(env, envPending+"=") {
		t.Fatalf("env lacks reload vars")
	}
	if string(got.pending) != rest {
		t.Fatalf("pending %q, want %q", got.pending, rest)
	}
	fi, err := os.Stat(got.pendingPath)
	if err != nil || fi.Mode().Perm() != 0o600 || filepath.Dir(got.pendingPath) != filepath.Join(s.F.Home, "run") {
		t.Fatalf("pending file %q: %v %v", got.pendingPath, fi, err)
	}
}

func TestReloadProbeFailureKeepsServing(t *testing.T) {
	s, exe := reloadServer(t)
	now := time.Unix(1000, 0)
	s.Now = func() time.Time { return now }
	probes := 0
	s.Probe = func(string) error { probes++; return errors.New("half-written") }
	s.Exec = func(string, []string, []string) error { t.Error("exec after failed probe"); return nil }
	c, _ := startServe(t, s)
	c.send(ping(1))
	c.next()
	replaceExe(t, exe, "broken")
	c.send(ping(2))
	if r := c.next(); r["id"].(float64) != 2 || r["error"] != nil {
		t.Fatalf("not answered by old code: %v", r)
	}
	c.send(ping(3))
	c.next()
	if probes != 1 {
		t.Fatalf("probes = %d within 10 s, want 1", probes)
	}
	now = now.Add(11 * time.Second)
	c.send(ping(4))
	c.next()
	if probes != 2 {
		t.Fatalf("probes = %d after 10 s, want 2", probes)
	}
}

func TestReloadExecFailureKeepsServing(t *testing.T) {
	s, exe := reloadServer(t)
	s.Probe = func(string) error { return nil }
	s.Exec = func(string, []string, []string) error { return errors.New("exec format error") }
	c, _ := startServe(t, s)
	c.send(ping(1))
	c.next()
	replaceExe(t, exe, "v2")
	c.send(ping(2))
	if r := c.next(); r["id"].(float64) != 2 || r["error"] != nil {
		t.Fatalf("not answered: %v", r)
	}
	left, _ := filepath.Glob(filepath.Join(s.F.Home, "run", "*"))
	if len(left) != 0 {
		t.Fatalf("pending files left: %v", left)
	}
}

func TestResumeAfterReload(t *testing.T) {
	s, _ := reloadServer(t)
	pending := filepath.Join(s.F.Home, "run", "mcp-pending-x")
	os.MkdirAll(filepath.Dir(pending), 0o700)
	call := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fleet_inbox","arguments":{}}}` + "\n"
	os.WriteFile(pending, []byte(ping(1)+call), 0o600)
	t.Setenv(envReloaded, "1")
	t.Setenv(envPending, pending)

	stdinR, stdinW := io.Pipe()
	in, reloaded := ResumeInput(stdinR)
	if !reloaded {
		t.Fatal("not reloaded")
	}
	if os.Getenv(envReloaded) != "" || os.Getenv(envPending) != "" {
		t.Fatal("reload env vars still set")
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatal("pending file not deleted")
	}
	s.Reloaded = true
	outR, outW := io.Pipe()
	go s.Serve(in, outW)
	t.Cleanup(func() { stdinW.Close() })
	c := &pipeConn{in: stdinW, out: bufio.NewScanner(outR), t: t}
	if r := c.next(); r["method"] != "notifications/tools/list_changed" || r["id"] != nil {
		t.Fatalf("first line %v", r)
	}
	if r := c.next(); r["id"].(float64) != 1 {
		t.Fatalf("second line %v", r)
	}
	text := func(r map[string]any) string {
		return r["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	}
	r := c.next()
	if r["id"].(float64) != 2 || !strings.HasSuffix(text(r), "[fleet updated: tools reloaded]") {
		t.Fatalf("first tool result %v", r)
	}
	c.send(strings.Replace(call, `"id":2`, `"id":3`, 1))
	r = c.next()
	if r["id"].(float64) != 3 || strings.Contains(text(r), "reloaded") {
		t.Fatalf("note repeated: %v", r)
	}
}

func TestResumeInputNoop(t *testing.T) {
	t.Setenv(envReloaded, "")
	r := strings.NewReader("x")
	if in, ok := ResumeInput(r); ok || in != r {
		t.Fatal("resumed without reload env")
	}
}

func TestInitializeAdvertisesListChanged(t *testing.T) {
	s, _ := reloadServer(t)
	c, _ := startServe(t, s)
	c.send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	r := c.next()
	caps := r["result"].(map[string]any)["capabilities"].(map[string]any)["tools"].(map[string]any)
	if caps["listChanged"] != true {
		t.Fatalf("capabilities %v", caps)
	}
}

// TestReloadIntegration builds fleet, serves MCP from it, swaps the binary and
// checks the next call runs in a re-exec'd process with the same pid. It uses
// an isolated home, config and tmux dir and only ping/initialize/tools/list,
// which never touch tmux.
func TestReloadIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the fleet binary")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "fleet")
	build := exec.Command("go", "build", "-o", exe, "github.com/sturlabragason/fleet/cmd/fleet")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := filepath.Join(dir, "home")
	os.MkdirAll(home, 0o700)
	var env []string
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if k == "TMUX" || k == "TMUX_PANE" || k == "HOME" || k == "XDG_CONFIG_HOME" || k == "TMUX_TMPDIR" || strings.HasPrefix(k, "FLEET_") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"FLEET_HOME="+filepath.Join(home, ".fleet"), "FLEET_CONFIG="+filepath.Join(home, "no-config.toml"),
		"TMUX_TMPDIR="+filepath.Join(dir, "tmux"))

	cmd := exec.Command(exe, "mcp")
	cmd.Env = env
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	stop := func() { once.Do(func() { stdin.Close(); cmd.Wait() }) }
	t.Cleanup(func() {
		stop()
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
		}
	})
	pid := cmd.Process.Pid
	lines := make(chan map[string]any, 16)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			var m map[string]any
			json.Unmarshal(sc.Bytes(), &m)
			lines <- m
		}
		close(lines)
	}()
	next := func() map[string]any {
		t.Helper()
		select {
		case m, ok := <-lines:
			if !ok {
				t.Fatal("server exited")
			}
			return m
		case <-time.After(15 * time.Second):
			t.Fatal("timeout")
		}
		return nil
	}
	io.WriteString(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"`+ProtocolVersion+`"}}`+"\n")
	if r := next(); r["id"].(float64) != 1 {
		t.Fatalf("initialize %v", r)
	}
	io.WriteString(stdin, ping(2))
	if r := next(); r["id"].(float64) != 2 {
		t.Fatalf("ping %v", r)
	}

	// Swap in a copy (new inode) the way `make build` does.
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe+".tmp", data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(exe+".tmp", exe); err != nil {
		t.Fatal(err)
	}
	io.WriteString(stdin, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`+"\n")
	if r := next(); r["method"] != "notifications/tools/list_changed" {
		t.Fatalf("want list_changed first, got %v", r)
	}
	r := next()
	if r["id"].(float64) != 3 || len(r["result"].(map[string]any)["tools"].([]any)) == 0 {
		t.Fatalf("tools/list %v", r)
	}
	if cmd.Process.Pid != pid || syscall.Kill(pid, 0) != nil {
		t.Fatal("server process is gone")
	}
	// /proc shows the process now runs the new file, not the deleted old one.
	if got, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil && got != exe {
		t.Fatalf("pid %d runs %q, want %q", pid, got, exe)
	}
	left, _ := filepath.Glob(filepath.Join(home, ".fleet", "run", "*"))
	if len(left) != 0 {
		t.Fatalf("pending files left: %v", left)
	}
	stop()
}

// execRecorder is an Exec stub that keeps the pending file's bytes.
func execRecorder(execs chan []byte) func(string, []string, []string) error {
	return func(_ string, _, env []string) error {
		for _, e := range env {
			if v, ok := strings.CutPrefix(e, envPending+"="); ok {
				b, _ := os.ReadFile(v)
				execs <- b
			}
		}
		return nil
	}
}

// A second reload while the first reload's pending input (> 4 KB, more than
// the read buffer) is still being served must hand on every unread byte.
func TestSecondReloadKeepsUnreadPendingInput(t *testing.T) {
	s, exe := reloadServer(t)
	var data strings.Builder
	starts := []int{}
	for i := 1; data.Len() < 12<<10; i++ {
		starts = append(starts, data.Len())
		fmt.Fprintf(&data, `{"jsonrpc":"2.0","id":%d,"method":"ping","params":{"pad":"%s"}}`+"\n", i, strings.Repeat("x", 40))
	}
	pending := filepath.Join(s.F.Home, "run", "mcp-pending-first")
	os.MkdirAll(filepath.Dir(pending), 0o700)
	os.WriteFile(pending, []byte(data.String()), 0o600)
	t.Setenv(envReloaded, "1")
	t.Setenv(envPending, pending)
	stdinR, stdinW := io.Pipe()
	in, _ := ResumeInput(stdinR)

	// The binary changed; probes fail for requests 1-4 (each served by the
	// current code) and pass on request 5, which reloads mid-way through
	// the pending input.
	replaceExe(t, exe, "v3")
	s.selfExe = filepath.Join(t.TempDir(), "running")
	os.WriteFile(s.selfExe, []byte("v2"), 0o755)
	now := time.Unix(1000, 0)
	s.Now = func() time.Time { now = now.Add(11 * time.Second); return now }
	probes := 0
	s.Probe = func(string) error {
		if probes++; probes <= 4 {
			return errors.New("not yet")
		}
		return nil
	}
	execs := make(chan []byte, 1)
	s.Exec = execRecorder(execs)
	c, done := startServeOn(t, s, in, stdinW)
	t.Cleanup(func() { stdinW.Close() })
	for i := 1; i <= 4; i++ {
		if r := c.next(); r["id"].(float64) != float64(i) {
			t.Fatalf("response %v", r)
		}
	}
	got := <-execs
	<-done
	if want := data.String()[starts[4]:]; string(got) != want {
		t.Fatalf("pending after second reload: %d bytes, want %d (first 80: %q)", len(got), len(want), got[:min(80, len(got))])
	}
}

// A build that lands between the probe and the exec must not be exec'd
// unprobed; the next request retries.
func TestReloadSkipsFileChangedAfterProbe(t *testing.T) {
	s, exe := reloadServer(t)
	probes := 0
	s.Probe = func(string) error {
		probes++
		if probes == 1 {
			replaceExe(t, exe, "v3")
		}
		return nil
	}
	execs := make(chan []byte, 1)
	s.Exec = execRecorder(execs)
	c, done := startServe(t, s)
	c.send(ping(1))
	c.next()
	replaceExe(t, exe, "v2")
	c.send(ping(2))
	if r := c.next(); r["id"].(float64) != 2 {
		t.Fatalf("not answered by old code: %v", r)
	}
	if left, _ := filepath.Glob(filepath.Join(s.F.Home, "run", "*")); len(left) != 0 {
		t.Fatalf("pending files left: %v", left)
	}
	c.send(ping(3))
	if got := <-execs; string(got) != ping(3) {
		t.Fatalf("pending %q", got)
	}
	<-done
	if probes != 2 {
		t.Fatalf("probes = %d, want 2", probes)
	}
}

// The starting identity is the running binary, so a file replaced before
// Serve started still counts as a change.
func TestReloadIdentityFromRunningBinary(t *testing.T) {
	s, _ := reloadServer(t)
	old := filepath.Join(t.TempDir(), "running")
	os.WriteFile(old, []byte("v0"), 0o755)
	s.selfExe = old
	s.Probe = func(string) error { return nil }
	execs := make(chan []byte, 1)
	s.Exec = execRecorder(execs)
	c, done := startServe(t, s)
	c.send(ping(1))
	select {
	case got := <-execs:
		if string(got) != ping(1) {
			t.Fatalf("pending %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no reload: binary replaced before Serve was not seen as a change")
	}
	<-done
}

func TestProbeErrorIncludesStderr(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "fleet")
	os.WriteFile(bin, []byte("#!/bin/sh\necho 'flag provided but not defined: -probe' >&2\nexit 2\n"), 0o755)
	err := probe(bin)
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("probe error %v", err)
	}
}
