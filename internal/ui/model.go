package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/sturlabragason/fleet/internal/board"
	"github.com/sturlabragason/fleet/internal/layout"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/wall"
)

// Model is the overview state.
type Model struct {
	be       Backend
	agents   []*board.Agent // attention-sorted
	onWall   map[string]bool
	sel      string
	w, h     int
	frame    int
	animate  bool
	msg      string
	help     bool // ? overlay with every hotkey replaces the tiles
	stateDir string
	cfg      layout.Config
	now      func() time.Time
	supCtx   [2]string // last label/colour set on the supervisor border
	ctxSet   bool
}

type agentsMsg struct {
	agents []*board.Agent
	onWall map[string]bool
	supCtx [2]string
	err    error
}
type pollMsg struct{}
type animMsg struct{}
type infoMsg string

// New builds a model. stateDir holds ui.json (cursor survives restarts).
func New(be Backend, stateDir string) *Model {
	m := &Model{be: be, onWall: map[string]bool{}, stateDir: stateDir, cfg: layout.Default, now: time.Now}
	if b, err := os.ReadFile(filepath.Join(stateDir, "ui.json")); err == nil {
		var s struct{ Selected string }
		if json.Unmarshal(b, &s) == nil {
			m.sel = s.Selected
		}
	}
	return m
}

func (m *Model) saveCursor() {
	if m.stateDir == "" {
		return
	}
	b, _ := json.Marshal(struct{ Selected string }{m.sel})
	os.WriteFile(filepath.Join(m.stateDir, "ui.json"), b, 0o644)
}

func (m *Model) poll() tea.Cmd {
	be := m.be
	return func() tea.Msg {
		as, err := be.Refresh()
		// Re-tile on every poll: nothing else notices a status flip, so this
		// is what makes emphasis move when a pane starts waiting (§5b.3).
		_ = be.ApplyWall()
		l, c := be.SupCtx()
		return agentsMsg{agents: as, onWall: be.OnWall(), supCtx: [2]string{l, c}, err: err}
	}
}

// Init starts polling.
func (m *Model) Init() tea.Cmd { return tea.Batch(m.poll(), anim()) }

func anim() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return animMsg{} })
}

func pollLater() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return pollMsg{} })
}

// Update handles messages; panics are logged and swallowed.
func (m *Model) Update(msg tea.Msg) (_ tea.Model, cmd tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			logx.For("ui").Error("panic in Update", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			m.msg = fmt.Sprintf("internal error: %v (logged)", r)
			cmd = nil
		}
	}()
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case agentsMsg:
		if msg.err != nil {
			m.msg = msg.err.Error()
		}
		m.setAgents(msg.agents)
		m.onWall = msg.onWall
		return m, tea.Batch(pollLater(), m.syncSupCtx(msg.supCtx))
	case pollMsg:
		return m, m.poll()
	case animMsg:
		m.frame++
		return m, anim()
	case infoMsg:
		m.msg = string(msg)
	case tea.KeyMsg:
		return m, m.key(msg.String())
	case tea.MouseMsg:
		if !m.help && msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			for _, a := range m.agents {
				if zone.Get("tile-" + a.Name).InBounds(msg) {
					m.sel = a.Name
					m.saveCursor()
					return m, m.show()
				}
			}
		}
	}
	return m, nil
}

func (m *Model) setAgents(as []*board.Agent) {
	sort.SliceStable(as, func(i, j int) bool {
		ri, rj := rank(as[i].Status), rank(as[j].Status)
		if ri != rj {
			return ri < rj
		}
		if as[i].Status == "waiting" && !as[i].StatusSince.Equal(as[j].StatusSince) {
			return as[i].StatusSince.Before(as[j].StatusSince) // longest wait first
		}
		return as[i].Name < as[j].Name
	})
	m.agents = as
	if m.index() < 0 && len(as) > 0 {
		m.sel = as[0].Name
	}
}

func (m *Model) index() int {
	for i, a := range m.agents {
		if a.Name == m.sel {
			return i
		}
	}
	return -1
}

func (m *Model) cols() int {
	c := m.w / m.cfg.TileW
	if c < 1 {
		c = 1
	}
	return c
}

func (m *Model) key(k string) tea.Cmd {
	i := m.index()
	n := len(m.agents)
	move := func(d int) {
		if n == 0 {
			return
		}
		j := i + d
		if j < 0 || j >= n {
			return
		}
		m.sel = m.agents[j].Name
		m.saveCursor()
	}
	if m.help {
		switch k {
		case "q", "esc", "?": // q closes the overlay, it doesn't quit
			m.help = false
		case "ctrl+c":
			return tea.Quit
		}
		return nil
	}
	switch k {
	case "q", "ctrl+c":
		return tea.Quit
	case "?":
		m.help = true
	case "left", "h":
		move(-1)
	case "right", "l":
		move(1)
	case "up", "k":
		move(-m.cols())
	case "down", "j":
		move(m.cols())
	case "tab":
		// waiting tiles sort first, longest wait first: cycle through them
		var waiting []string
		for _, a := range m.agents {
			if a.Status == "waiting" {
				waiting = append(waiting, a.Name)
			}
		}
		if len(waiting) == 0 {
			m.msg = "nothing waiting"
			return nil
		}
		next := waiting[0]
		for j, nm := range waiting {
			if nm == m.sel && j+1 < len(waiting) {
				next = waiting[j+1]
			}
		}
		m.sel = next
		m.saveCursor()
	case "enter":
		return m.show()
	case "R":
		if !m.be.WallEnabled() {
			m.msg = "R needs fleet up"
			return nil
		}
		be := m.be
		return func() tea.Msg {
			if err := be.ResetSup(); err != nil {
				return infoMsg("reset: " + err.Error())
			}
			return nil
		}
	case "x":
		name, be := m.sel, m.be
		if name == "" || !m.onWall[name] {
			m.msg = "x: " + name + " is not on the wall"
			return nil
		}
		return func() tea.Msg {
			if err := be.Hide(name); err != nil {
				logx.For("ui").Error("hide failed", "agent", name, "err", err)
				return infoMsg("hide " + name + ": " + err.Error())
			}
			return infoMsg(name + " off wall")
		}
	case "1", "2", "4", "6", "9", "m":
		be := m.be
		return func() tea.Msg {
			if err := be.Preset(k); err != nil {
				return infoMsg(err.Error())
			}
			return infoMsg("wall layout " + k)
		}
	}
	return nil
}

func (m *Model) show() tea.Cmd {
	name, be := m.sel, m.be
	if name == "" {
		return nil
	}
	if !be.WallEnabled() {
		m.msg = "live wall needs `fleet up` (TUI must run inside the fleet tmux server)"
		return nil
	}
	return func() tea.Msg {
		ok, err := be.Show(name)
		if err != nil {
			logx.For("ui").Error("show failed", "agent", name, "err", err)
			return infoMsg("show " + name + ": " + err.Error())
		}
		if !ok {
			return infoMsg(wall.FullMsg)
		}
		return infoMsg(name + " on wall")
	}
}

// syncSupCtx updates the supervisor border meter only when it changed.
func (m *Model) syncSupCtx(v [2]string) tea.Cmd {
	if m.ctxSet && v == m.supCtx {
		return nil
	}
	m.supCtx, m.ctxSet = v, true
	be := m.be
	return func() tea.Msg {
		if err := be.SetSupCtx(v[0], v[1]); err != nil {
			logx.For("ui").Warn("set supervisor ctx", "err", err)
		}
		return nil
	}
}
