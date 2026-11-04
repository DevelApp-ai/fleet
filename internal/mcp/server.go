// Package mcp is a minimal stdio MCP server (JSON-RPC 2.0, one message per
// line) exposing fleet operations as tools for the supervisor. Only protocol
// messages go to stdout; everything else goes to fleet.log.
package mcp

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"sync"

	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/supervisor"
	"github.com/sturlabragason/fleet/internal/tools"
	"github.com/sturlabragason/fleet/internal/wall"
)

// Instructions are the supervisor rules, sent as MCP server instructions.
//
//go:embed instructions.md
var Instructions string

// ProtocolVersion is the MCP revision we speak.
const ProtocolVersion = "2025-06-18"

// Server serves fleet tools.
type Server struct {
	F *fleet.Fleet
	W *wall.Wall
	// Fill reports the supervisor's context use (tests replace it).
	Fill func() (used, max int, ok bool)
	// Reset asks tmux to replace the supervisor after a handoff (tests stub it).
	Reset func() error
	mu    sync.Mutex
}

// New returns a server.
func New(f *fleet.Fleet, w *wall.Wall) *Server {
	s := &Server{F: f, W: w}
	s.Fill = func() (int, int, bool) { return supervisor.CurrentFill(f.Home, f.T, fleet.Session+":main") }
	s.Reset = func() error {
		// tmux runs the reset, so it survives this MCP server being killed with the supervisor.
		_, err := f.T.Run("run-shell", "-b", "sleep 2; "+tools.Quote(f.Exe)+" supervisor reset --handoff")
		return err
	}
	return s
}

// contextWarning is appended to tool results once the supervisor's context
// is at or above the handoff threshold ("" otherwise).
func (s *Server) contextWarning() string {
	if s.Fill == nil {
		return ""
	}
	used, max, ok := s.Fill()
	if !ok {
		return ""
	}
	pct := supervisor.Pct(used, max)
	if pct < supervisor.Threshold() {
		return ""
	}
	return fmt.Sprintf("\n\n[supervisor context %d%%: save anything pending with fleet_note, then call fleet_handoff]", pct)
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads requests from r and writes responses to w until EOF.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	log := logx.For("mcp")
	log.Info("start")
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	enc := json.NewEncoder(w)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			log.Warn("bad json", "err", err)
			enc.Encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			continue
		}
		res, rerr := s.handle(req)
		if len(req.ID) == 0 { // notification
			continue
		}
		resp := response{JSONRPC: "2.0", ID: req.ID, Result: res, Error: rerr}
		if rerr == nil && res == nil {
			resp.Result = struct{}{}
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	log.Info("stdin closed")
	return sc.Err()
}

func (s *Server) handle(req request) (res any, rerr *rpcError) {
	log := logx.For("mcp")
	defer func() {
		if r := recover(); r != nil {
			log.Error("panic", "method", req.Method, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			rerr = &rpcError{-32603, fmt.Sprint(r)}
		}
	}()
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "fleet", "version": "0.1.0"},
			"instructions":    Instructions,
		}, nil
	case "notifications/initialized", "notifications/cancelled":
		return nil, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolDefs}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		s.mu.Lock()
		text, err := s.call(p.Name, p.Arguments)
		s.mu.Unlock()
		log.Info("call", "tool", p.Name, "err", err)
		isErr := err != nil
		if isErr {
			text = err.Error()
		}
		if p.Name != "fleet_handoff" {
			text += s.contextWarning()
		}
		return toolResult(text, isErr), nil
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}

func toolResult(text string, isErr bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isErr}
}
