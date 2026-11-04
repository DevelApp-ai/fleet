package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sturlabragason/fleet/internal/tools"
)

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func obj(required []string, props map[string]any) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

var confirmedProp = map[string]any{"type": "boolean", "description": "true only after the user explicitly said yes in chat"}

var toolDefs = []tool{
	{"fleet_status", "List all agents: name, profile, status, on_wall, what it waits for, age. Start here.", obj(nil, map[string]any{})},
	{"fleet_peek", "Show the last N lines (default 20, max 80) of one agent's screen.", obj([]string{"name"}, map[string]any{
		"name": str("agent name"), "lines": map[string]any{"type": "integer", "description": "lines to return (default 20, max 80)"}})},
	{"fleet_send", "Send an ordinary prompt or follow-up text to an agent (presses Enter).", obj([]string{"name", "text"}, map[string]any{
		"name": str("agent name"), "text": str("text to send")})},
	{"fleet_answer", "Answer an agent's y/n approval prompt. Ask the user first; set confirmed=true only after an explicit yes.", obj([]string{"name", "answer", "confirmed"}, map[string]any{
		"name": str("agent name"), "answer": map[string]any{"type": "string", "enum": []string{"y", "n"}}, "confirmed": confirmedProp})},
	{"fleet_spawn", "Start a new agent with a tool profile in a directory, optionally with a first prompt.", obj([]string{"name", "profile", "dir"}, map[string]any{
		"name": str("short unique agent name"), "profile": str("tool profile, see fleet_profiles"), "dir": str("working directory (absolute)"), "prompt": str("optional first prompt")})},
	{"fleet_profiles", "List tool profiles that fleet_spawn accepts.", obj(nil, map[string]any{})},
	{"fleet_close", "Stop an agent and remove it from the board. Ask the user first; set confirmed=true only after an explicit yes.", obj([]string{"name", "confirmed"}, map[string]any{
		"name": str("agent name"), "confirmed": confirmedProp})},
	{"fleet_wall", "Change the live wall: show or hide an agent pane, or set the layout preset (1,2,4,6,9,m).", obj([]string{"action"}, map[string]any{
		"action": map[string]any{"type": "string", "enum": []string{"show", "hide", "preset"}}, "name": str("agent name for show/hide"), "preset": str("1, 2, 4, 6, 9 or m")})},
}

type args struct {
	Name, Text, Answer, Profile, Dir, Prompt, Action, Preset string
	Lines                                                    int
	Confirmed                                                bool
}

func (s *Server) call(name string, raw json.RawMessage) (string, error) {
	var a args
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return "", fmt.Errorf("bad arguments: %v", err)
		}
	}
	need := func(v, field string) error {
		if v == "" {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
	switch name {
	case "fleet_status":
		return s.status()
	case "fleet_peek":
		if err := need(a.Name, "name"); err != nil {
			return "", err
		}
		n := a.Lines
		if n <= 0 {
			n = 20
		}
		if n > 80 {
			n = 80
		}
		out, err := s.F.Peek(a.Name, 0)
		if err != nil {
			return "", err
		}
		lines := strings.Split(strings.TrimRight(out, "\n "), "\n")
		if len(lines) > n {
			lines = lines[len(lines)-n:]
		}
		return strings.Join(lines, "\n"), nil
	case "fleet_send":
		if err := need(a.Name, "name"); err != nil {
			return "", err
		}
		if err := need(a.Text, "text"); err != nil {
			return "", err
		}
		return "sent to " + a.Name, s.F.Send(a.Name, a.Text)
	case "fleet_answer":
		if a.Answer != "y" && a.Answer != "n" {
			return "", fmt.Errorf(`answer must be "y" or "n"`)
		}
		if !a.Confirmed {
			return "", fmt.Errorf("not confirmed: ask the user, then call again with confirmed=true")
		}
		return "answered " + a.Answer + " to " + a.Name, s.F.Send(a.Name, a.Answer)
	case "fleet_spawn":
		for f, v := range map[string]string{"name": a.Name, "profile": a.Profile, "dir": a.Dir} {
			if err := need(v, f); err != nil {
				return "", err
			}
		}
		ag, err := s.F.Spawn(a.Name, a.Profile, a.Dir, a.Prompt)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("spawned %s (%s) in %s", ag.Name, ag.Profile, ag.Dir), nil
	case "fleet_profiles":
		var b strings.Builder
		for _, n := range tools.Names(s.F.Profiles) {
			p := s.F.Profiles[n]
			fmt.Fprintf(&b, "%s: %s\n", n, strings.Join(p.Argv(tools.Vars{Fleet: "fleet", Name: "<name>"}, false), " "))
		}
		return b.String(), nil
	case "fleet_close":
		if err := need(a.Name, "name"); err != nil {
			return "", err
		}
		if !a.Confirmed {
			return "", fmt.Errorf("not confirmed: ask the user, then call again with confirmed=true")
		}
		return "closed " + a.Name, s.F.Close(a.Name)
	case "fleet_wall":
		switch a.Action {
		case "preset":
			return "wall preset " + a.Preset, s.W.SetPreset(a.Preset)
		case "show", "hide":
			p, err := s.F.Pane(a.Name)
			if err != nil {
				return "", err
			}
			if a.Action == "show" {
				return a.Name + " on wall", s.W.Show(p)
			}
			return a.Name + " off wall", s.W.Hide(p)
		}
		return "", fmt.Errorf("action must be show, hide or preset")
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

func (s *Server) status() (string, error) {
	as, err := s.F.Refresh()
	if err != nil {
		return "", err
	}
	if len(as) == 0 {
		return "no agents", nil
	}
	on := s.W.OnWall()
	var b strings.Builder
	b.WriteString("name | profile | status | wall | age | waiting_for\n")
	for _, a := range as {
		wf := ""
		if a.Status == "waiting" || a.Status == "error" || a.Status == "stuck" {
			wf = a.LastLine
			if len(wf) > 70 {
				wf = wf[:70] + "…"
			}
		}
		w := ""
		if on[a.Name] {
			w = "yes"
		}
		fmt.Fprintf(&b, "%s | %s | %s | %s | %s | %s\n", a.Name, a.Profile, a.Status, w, time.Since(a.StatusSince).Round(time.Second), wf)
	}
	return b.String(), nil
}
