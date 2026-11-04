// Package fleet implements the core operations behind the CLI and TUI:
// spawn, refresh (status), send, peek and kill. It is the only place that
// combines tmux, tools, status and board.
package fleet

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/sturlabragason/fleet/internal/board"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/status"
	"github.com/sturlabragason/fleet/internal/tmux"
	"github.com/sturlabragason/fleet/internal/tools"
)

const (
	// Socket is the dedicated tmux server for agents.
	Socket = "fleet"
	// Session holds every agent window and the main window.
	Session = "fleet"
	// WindowPrefix names hidden per-agent windows.
	WindowPrefix = "ag-"
)

// StuckAfter is how long a working pane may stay unchanged before it is stuck.
var StuckAfter = 10 * time.Minute

// Fleet bundles the dependencies.
type Fleet struct {
	T        *tmux.Client
	Store    *board.Store
	Home     string
	Exe      string
	Profiles map[string]tools.Profile
	Patterns status.Sets
}

// Open builds a Fleet on the default socket and ~/.fleet.
func Open() (*Fleet, error) {
	home := logx.Home()
	exe, _ := os.Executable()
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return New(tmux.New(Socket), home, exe)
}

// New builds a Fleet from parts (used by tests).
func New(t *tmux.Client, home, exe string) (*Fleet, error) {
	ps, err := tools.Load(home)
	if err != nil {
		return nil, err
	}
	return &Fleet{T: t, Store: board.NewStore(home), Home: home, Exe: exe, Profiles: ps, Patterns: status.Load(home)}, nil
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$`)

// Spawn starts an agent in its own hidden window.
func (f *Fleet) Spawn(name, profile, dir, prompt string) (*board.Agent, error) {
	log := logx.For("fleet")
	if !nameRe.MatchString(name) {
		return nil, fmt.Errorf("bad name %q", name)
	}
	p, ok := f.Profiles[profile]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q (have %v)", profile, tools.Names(f.Profiles))
	}
	if _, exists := f.Store.Load().Agents[name]; exists {
		if f.paneOf(name) != "" {
			return nil, fmt.Errorf("agent %q already exists", name)
		}
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	id := ""
	if len(p.LaunchID) > 0 {
		id = tools.NewUUID()
	}
	argv := p.Argv(tools.Vars{Fleet: f.Exe, Name: name, ID: id}, false)
	pane, err := f.T.Spawn(Session, WindowPrefix+name, dir, p.EnvList(), tools.ShellCommand(argv))
	if err != nil {
		log.Error("spawn failed", "agent", name, "err", err)
		return nil, err
	}
	f.T.SetPaneOption(pane, "@fleet_agent", name)
	f.T.SetPaneOption(pane, "@role", "agent")
	f.T.SetPaneOption(pane, "remain-on-exit", "on")
	f.T.Run("set-option", "-s", "-t", Session, "escape-time", "10")
	f.T.Run("set-option", "-g", "mouse", "on")
	a := &board.Agent{Name: name, Profile: profile, Patterns: p.Patterns, Dir: dir, Goal: prompt, SessionID: id,
		PaneID: pane, Status: string(status.Unknown), Created: time.Now(), StatusSince: time.Now(), LastChange: time.Now()}
	err = f.Store.Update(func(b *board.Board) error { b.Agents[name] = a; return nil })
	log.Info("spawned", "agent", name, "profile", profile, "dir", dir, "pane", pane, "session_id", id, "argv", argv)
	if prompt != "" {
		time.Sleep(1500 * time.Millisecond)
		f.T.Paste(pane, prompt, true)
	}
	return a, err
}

// paneOf finds the live pane for an agent by its @fleet_agent tag.
func (f *Fleet) paneOf(name string) string {
	ps, _ := f.T.ListPanes("")
	for _, p := range ps {
		if p.Agent == name {
			return p.ID
		}
	}
	return ""
}

// Pane returns the pane id of an agent or an error.
func (f *Fleet) Pane(name string) (string, error) {
	if p := f.paneOf(name); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("agent %q not running", name)
}

// Send pastes text into the agent and presses Enter.
func (f *Fleet) Send(name, text string) error {
	p, err := f.Pane(name)
	if err != nil {
		return err
	}
	logx.For("fleet").Info("send", "agent", name, "bytes", len(text))
	return f.T.Paste(p, text, true)
}

// Peek returns raw pane text (last n lines).
func (f *Fleet) Peek(name string, n int) (string, error) {
	p, err := f.Pane(name)
	if err != nil {
		return "", err
	}
	return f.T.Capture(p, n)
}

// Close stops the agent and drops it from the board.
func (f *Fleet) Close(name string) error {
	if p := f.paneOf(name); p != "" {
		if err := f.T.KillPane(p); err != nil {
			return err
		}
	}
	logx.For("fleet").Info("closed", "agent", name)
	return f.Store.Update(func(b *board.Board) error {
		if _, ok := b.Agents[name]; !ok {
			return fmt.Errorf("no agent %q", name)
		}
		delete(b.Agents, name)
		return nil
	})
}
