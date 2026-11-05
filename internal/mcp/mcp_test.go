package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/tmux"
	"github.com/sturlabragason/fleet/internal/tools"
	"github.com/sturlabragason/fleet/internal/wall"
)

func TestRoundTrip(t *testing.T) {
	home := t.TempDir()
	tc := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { tc.KillServer() })
	os.WriteFile(filepath.Join(home, "tools.toml"), []byte("[yn]\ncommand = \"sh\"\nargs = [\"-c\", \"printf 'Approve? [y/n] '; cat\"]\npatterns = \"fake\"\n"), 0o644)
	// Keep the user's real ~/.config/fleet/config.toml out of the test.
	t.Setenv("FLEET_CONFIG", filepath.Join(home, "no-config.toml"))
	f, err := fleet.New(tc, home, "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Spawn("a1", "yn", home, ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	go New(f, wall.New(tc, fleet.Session)).Serve(inR, outW)
	dec := bufio.NewScanner(outR)
	call := func(id int, method string, params any) map[string]any {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		inW.Write(append(b, '\n'))
		if !dec.Scan() {
			t.Fatal("no response")
		}
		var r map[string]any
		json.Unmarshal(dec.Bytes(), &r)
		return r
	}
	init := call(1, "initialize", map[string]any{"protocolVersion": ProtocolVersion})
	if init["result"].(map[string]any)["protocolVersion"] != ProtocolVersion {
		t.Fatalf("init %v", init)
	}
	inW.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"))
	list := call(2, "tools/list", nil)
	names := ""
	for _, tl := range list["result"].(map[string]any)["tools"].([]any) {
		names += tl.(map[string]any)["name"].(string) + " "
	}
	for _, want := range []string{"fleet_status", "fleet_peek", "fleet_send", "fleet_answer", "fleet_spawn", "fleet_sessions", "fleet_adopt", "fleet_profiles", "fleet_close", "fleet_wall", "fleet_note", "fleet_brief", "fleet_handoff"} {
		if !strings.Contains(names, want) {
			t.Errorf("missing tool %s", want)
		}
	}
	text := func(r map[string]any) (string, bool) {
		res := r["result"].(map[string]any)
		return res["content"].([]any)[0].(map[string]any)["text"].(string), res["isError"].(bool)
	}
	// The prompt can take a while to appear on slow machines (CI).
	var st string
	var isErr bool
	for i := 0; i < 50; i++ {
		st, isErr = text(call(100+i, "tools/call", map[string]any{"name": "fleet_status", "arguments": map[string]any{}}))
		if strings.Contains(st, "a1 | yn | waiting") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if isErr || !strings.Contains(st, "a1 | yn | waiting") {
		t.Errorf("status: %q", st)
	}
	if _, isErr := text(call(4, "tools/call", map[string]any{"name": "fleet_answer", "arguments": map[string]any{"name": "a1", "answer": "y"}})); !isErr {
		t.Error("unconfirmed answer must be refused")
	}
	if _, isErr := text(call(5, "tools/call", map[string]any{"name": "fleet_answer", "arguments": map[string]any{"name": "a1", "answer": "y", "confirmed": true}})); isErr {
		t.Error("confirmed answer failed")
	}
	if _, isErr := text(call(6, "tools/call", map[string]any{"name": "fleet_close", "arguments": map[string]any{"name": "a1"}})); !isErr {
		t.Error("unconfirmed close must be refused")
	}
	if msg, isErr := text(call(8, "tools/call", map[string]any{"name": "fleet_adopt", "arguments": map[string]any{"profile": "fake", "session_id": "abcdefgh"}})); !isErr || !strings.Contains(msg, "can't resume") {
		t.Errorf("adopt: %v %q", isErr, msg)
	}
	for _, args := range []map[string]any{{}, {"session_id": "abcdefgh", "query": "x"}} {
		if msg, isErr := text(call(9, "tools/call", map[string]any{"name": "fleet_adopt", "arguments": args})); !isErr || !strings.Contains(msg, "exactly one of session_id or query") {
			t.Errorf("adopt %v: %v %q", args, isErr, msg)
		}
	}
	// fleet_sessions on a temp store only (never the real ~/.copilot or ~/.claude*).
	sess := filepath.Join(home, "sessions")
	for n, p := range f.Profiles {
		p.SessionPath = ""
		f.Profiles[n] = p
	}
	f.Profiles["rs"] = tools.Profile{Name: "rs", Command: "sh", Resume: []string{"{id}"}, SessionPath: sess + "/{id}"}
	sd := filepath.Join(sess, "mcpsess-0001")
	os.MkdirAll(sd, 0o755)
	os.WriteFile(filepath.Join(sd, "workspace.yaml"), []byte("cwd: "+home+"\nname: Portal nav\n"), 0o644)
	os.WriteFile(filepath.Join(sd, "events.jsonl"), []byte(`{"type":"user.message","data":{"content":"go"}}`+"\n"), 0o644)
	if msg, isErr := text(call(10, "tools/call", map[string]any{"name": "fleet_sessions", "arguments": map[string]any{"query": "nav"}})); isErr || !strings.Contains(msg, "mcpsess-0001 | ") || !strings.Contains(msg, "| rs | ") || !strings.Contains(msg, "Portal nav") {
		t.Errorf("sessions: %v %q", isErr, msg)
	}
	if msg, isErr := text(call(11, "tools/call", map[string]any{"name": "fleet_adopt", "arguments": map[string]any{"query": "nothing-here"}})); !isErr || !strings.Contains(msg, "no session matches") {
		t.Errorf("adopt query: %v %q", isErr, msg)
	}
	// Claude-style store: adopt by padded query and by padded id lands in the transcript's cwd.
	cwd := t.TempDir()
	pd := filepath.Join(home, "projects", "-p")
	os.MkdirAll(pd, 0o755)
	f.Profiles["cl"] = tools.Profile{Name: "cl", Command: "sh", Args: []string{"-c", "cat"}, Resume: []string{"{id}"}, SessionPath: home + "/projects/*/{id}.jsonl"}
	for i, id := range []string{"ffff6666-0000-0000-0000-000000000001", "ffff6666-0000-0000-0000-000000000002"} {
		os.WriteFile(filepath.Join(pd, id+".jsonl"), []byte(`{"type":"user","cwd":"`+cwd+`","message":{"content":"grafana `+[]string{"alpha", "beta"}[i]+`"}}`+"\n"), 0o644)
	}
	for i, args := range []map[string]any{{"query": "  grafana   alpha ", "session_id": "  "}, {"session_id": " ffff6666-0000-0000-0000-000000000002 ", "query": ""}} {
		args["name"] = "cl" + strconv.Itoa(i)
		if msg, isErr := text(call(12+i, "tools/call", map[string]any{"name": "fleet_adopt", "arguments": args})); isErr || !strings.Contains(msg, "in "+cwd) {
			t.Errorf("adopt claude %v: %v %q", args, isErr, msg)
		}
	}
	if r := call(7, "nope", nil); r["error"] == nil {
		t.Error("want method not found")
	}
	inW.Close()
}
