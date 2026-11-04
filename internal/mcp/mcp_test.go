package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/tmux"
	"github.com/sturlabragason/fleet/internal/wall"
)

func TestRoundTrip(t *testing.T) {
	home := t.TempDir()
	tc := &tmux.Client{Socket: fmt.Sprintf("fleet-test-%d", rand.Int63()), Config: "/dev/null"}
	t.Cleanup(func() { tc.KillServer() })
	os.WriteFile(filepath.Join(home, "tools.toml"), []byte("[yn]\ncommand = \"sh\"\nargs = [\"-c\", \"printf 'Approve? [y/n] '; cat\"]\npatterns = \"fake\"\n"), 0o644)
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
	for _, want := range []string{"fleet_status", "fleet_peek", "fleet_send", "fleet_answer", "fleet_spawn", "fleet_profiles", "fleet_close", "fleet_wall", "fleet_note", "fleet_brief", "fleet_handoff"} {
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
	if r := call(7, "nope", nil); r["error"] == nil {
		t.Error("want method not found")
	}
	inW.Close()
}
