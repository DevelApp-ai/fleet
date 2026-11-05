package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/sessions"
	"github.com/sturlabragason/fleet/internal/supervisor"
	"github.com/sturlabragason/fleet/internal/tools"
	"github.com/sturlabragason/fleet/internal/wall"
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
	{"fleet_sessions", "Find copilot/claude sessions the user started outside fleet, newest first: id | age | profile | dir | title | flags. Use before fleet_adopt.", obj(nil, map[string]any{
		"query": str("optional words; every word must appear in title, dir, branch or id"), "limit": map[string]any{"type": "integer", "description": "max sessions (default 15)"}})},
	{"fleet_adopt", "Resume an existing session the user started outside fleet as a fleet agent. Pass session_id (full id or unique 8+ char prefix) or query (words that match exactly one session), not both. Refuses if the session is missing or still open elsewhere.", obj(nil, map[string]any{
		"session_id": str("session id from fleet_sessions"), "query": str("words matching exactly one session"), "profile": str("optional tool profile; default the suggested one"),
		"dir": str("optional working directory (absolute); default from the session"), "name": str("optional agent name; default base name of dir")})},
	{"fleet_profiles", "List tool profiles that fleet_spawn accepts.", obj(nil, map[string]any{})},
	{"fleet_close", "Stop an agent and remove it from the board. Ask the user first; set confirmed=true only after an explicit yes.", obj([]string{"name", "confirmed"}, map[string]any{
		"name": str("agent name"), "confirmed": confirmedProp})},
	{"fleet_wall", "Change the live wall: show or hide an agent pane, or set the layout preset (1,2,4,6,9,m).", obj([]string{"action"}, map[string]any{
		"action": map[string]any{"type": "string", "enum": []string{"show", "hide", "preset"}}, "name": str("agent name for show/hide"), "preset": str("1, 2, 4, 6, 9 or m")})},
	{"fleet_note", "Save one line to the decisions log: a user decision, preference or promise. Survives a supervisor reset.", obj([]string{"text"}, map[string]any{
		"text": str("the note, one line"), "agent": str("optional agent the note is about")})},
	{"fleet_brief", "Start-of-session brief: the last handoff, recent notes and the agent table. Call first in a fresh session.", obj(nil, map[string]any{})},
	{"fleet_handoff", "Write a handoff and get replaced by a fresh supervisor in a few seconds. Include open questions, promises and plans in flight, not agent statuses.", obj([]string{"summary"}, map[string]any{
		"summary": str("what the next supervisor must know")})},
}

type args struct {
	Name, Text, Answer, Profile, Dir, Prompt, Action, Preset, Agent, Summary, Query string
	SessionID                                                                       string `json:"session_id"`
	Lines, Limit                                                                    int
	Confirmed                                                                       bool
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
	case "fleet_sessions":
		return s.sessions(a.Query, a.Limit)
	case "fleet_adopt":
		o := fleet.AdoptOpts{Profile: a.Profile, Dir: a.Dir, Name: a.Name}
		id, q := strings.TrimSpace(a.SessionID), strings.TrimSpace(a.Query)
		var res sessions.Session
		switch {
		case (id == "") == (q == ""):
			return "", fmt.Errorf("pass exactly one of session_id or query")
		case q != "":
			r, err := s.F.ResolveWords(a.Profile, strings.Fields(q))
			if err != nil {
				return "", err
			}
			res = r
		default:
			r, ok, err := s.F.ResolveID(a.Profile, id)
			if err != nil {
				return "", err
			}
			if !ok && a.Profile == "" {
				return "", fmt.Errorf("no session %s; try fleet_sessions", id)
			}
			res = r
			if !ok { // not in the store: Adopt reports it
				res = sessions.Session{ID: id, Profile: a.Profile}
			}
		}
		ag, err := s.F.AdoptResolved(o, res)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("adopted %s (%s) session %s in %s", ag.Name, ag.Profile, ag.SessionID, ag.Dir), nil
	case "fleet_profiles":
		var b strings.Builder
		for _, n := range tools.Names(s.F.Profiles) {
			p := s.F.Profiles[n]
			argv, _ := p.Argv(tools.Vars{Fleet: "fleet", Name: "<name>"}, tools.Launch{})
			fmt.Fprintf(&b, "%s: %s\n", n, strings.Join(argv, " "))
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
				ok, err := s.W.Show(p)
				if err != nil || !ok {
					return wall.FullMsg, err
				}
				return a.Name + " on wall", nil
			}
			return a.Name + " off wall", s.W.Hide(p)
		}
		return "", fmt.Errorf("action must be show, hide or preset")
	case "fleet_note":
		if err := need(strings.TrimSpace(a.Text), "text"); err != nil {
			return "", err
		}
		return "noted", supervisor.AppendNote(s.F.Home, a.Agent, a.Text)
	case "fleet_brief":
		return s.brief()
	case "fleet_handoff":
		if err := need(strings.TrimSpace(a.Summary), "summary"); err != nil {
			return "", err
		}
		if err := supervisor.WriteHandoff(s.F.Home, a.Summary); err != nil {
			return "", err
		}
		if err := s.Reset(); err != nil {
			return "", fmt.Errorf("handoff saved, but reset failed: %v", err)
		}
		return "handoff saved; a fresh supervisor takes over in a few seconds. Stop here.", nil
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

// sessions lists sessions to adopt, one line each.
func (s *Server) sessions(query string, limit int) (string, error) {
	if limit <= 0 {
		limit = 15
	}
	ss, err := s.F.Sessions(sessions.Opts{Words: strings.Fields(query), Limit: limit})
	if err != nil {
		return "", err
	}
	if len(ss) == 0 {
		return "no sessions match", nil
	}
	var b strings.Builder
	b.WriteString("id | age | profile | dir | title | flags\n")
	for _, x := range ss {
		var flags []string
		if x.InUse {
			flags = append(flags, "in use")
		}
		if x.OnBoard {
			flags = append(flags, "on board")
		}
		fmt.Fprintf(&b, "%s | %s | %s | %s | %s | %s\n", x.ID, sessions.Age(time.Since(x.Updated)), x.Profile, x.Dir, sessions.OneLine(x.Title, 80), strings.Join(flags, ", "))
	}
	return b.String(), nil
}

// brief is the handoff, the last 30 notes and the status table.
func (s *Server) brief() (string, error) {
	var b strings.Builder
	if h, at, ok := supervisor.ReadHandoff(s.F.Home); ok {
		fmt.Fprintf(&b, "## Handoff (%s)\n%s\n\n", at.Format("2006-01-02 15:04"), h)
	} else {
		b.WriteString("## Handoff\nnone\n\n")
	}
	b.WriteString("## Notes\n")
	if notes := supervisor.TailNotes(s.F.Home, 30); len(notes) > 0 {
		b.WriteString(strings.Join(notes, "\n") + "\n\n")
	} else {
		b.WriteString("none\n\n")
	}
	st, err := s.status()
	if err != nil {
		return "", err
	}
	b.WriteString("## Agents\n" + st)
	return b.String(), nil
}
