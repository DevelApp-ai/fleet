package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/sturlabragason/fleet/internal/board"
	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/supervisor"
	"github.com/sturlabragason/fleet/internal/ui"
	"github.com/sturlabragason/fleet/internal/wall"
)

// backend adapts fleet + wall to ui.Backend.
type backend struct {
	f       *fleet.Fleet
	w       *wall.Wall
	enabled bool
}

func (b *backend) Refresh() ([]*board.Agent, error) { return b.f.Refresh() }

// ApplyWall re-tiles after every refresh, so emphasis follows status changes
// and the dwell/hold clock actually ticks. It is the cached apply: an
// unchanged layout costs a handful of read-only tmux calls and never touches
// a pane. It is a no-op (and never logs) when the TUI is not running inside
// the fleet server.
func (b *backend) ApplyWall() error {
	if !b.enabled {
		return nil
	}
	return b.w.ApplyCached()
}
func (b *backend) WallEnabled() bool { return b.enabled }
func (b *backend) OnWall() map[string]bool {
	if !b.enabled {
		return map[string]bool{}
	}
	return b.w.OnWall()
}
func (b *backend) Show(name string) (bool, error) {
	p, err := b.f.Pane(name)
	if err != nil {
		return false, err
	}
	return b.w.Show(p)
}
func (b *backend) Hide(name string) error {
	p, err := b.f.Pane(name)
	if err != nil {
		return err
	}
	return b.w.Hide(p)
}
func (b *backend) Preset(p string) error {
	if !b.enabled {
		return nil
	}
	return b.w.SetPreset(p)
}

func (b *backend) SupCtx() (string, string) {
	if !b.enabled {
		return "", ""
	}
	used, max, ok := supervisor.CurrentFill(b.f.Home, b.f.T, mainTarget())
	if !ok {
		return "", ""
	}
	pct := supervisor.Pct(used, max)
	return fmt.Sprintf("%d%%", pct), supervisor.Band(pct)
}

func (b *backend) SetSupCtx(label, color string) error {
	if !b.enabled {
		return nil
	}
	pane, err := supervisor.FindPane(b.f.T, mainTarget())
	if err != nil {
		return nil // no supervisor, nothing to colour
	}
	if label == "" {
		b.f.T.Run("set-option", "-p", "-u", "-t", pane, "@fleet_ctx")
		_, err = b.f.T.Run("set-option", "-p", "-u", "-t", pane, "@fleet_ctx_color")
		return err
	}
	b.f.T.SetPaneOption(pane, "@fleet_ctx_color", color)
	return b.f.T.SetPaneOption(pane, "@fleet_ctx", label)
}

func (b *backend) ResetSup() error {
	_, err := b.f.T.Run(confirmResetArgs(b.f.Exe)...)
	return err
}

// insideFleet reports whether this process runs in a pane of the fleet server.
func insideFleet(f *fleet.Fleet) bool {
	tm := os.Getenv("TMUX")
	if tm == "" || os.Getenv("TMUX_PANE") == "" {
		return false
	}
	sock := strings.SplitN(tm, ",", 2)[0]
	return filepath.Base(sock) == fleet.Socket
}

func cmdUI(f *fleet.Fleet) error {
	zone.NewGlobal()
	be := &backend{f: f, w: wall.New(f.T, fleet.Session), enabled: insideFleet(f)}
	logx.For("ui").Info("start", "wall", be.enabled)
	p := tea.NewProgram(ui.New(be, f.Home), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	if err != nil {
		logx.For("ui").Error("exit", "err", err)
	}
	return err
}
