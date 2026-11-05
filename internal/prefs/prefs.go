// Package prefs reads the local, never-committed spawn preferences from
// $XDG_CONFIG_HOME/fleet/config.toml (default ~/.config/fleet/config.toml,
// FLEET_CONFIG overrides the whole path). Repo defaults stay neutral: without
// the file fleet spawns in whatever mode the tool itself defaults to.
package prefs

import (
	"fmt"
	"strings"
	"time"

	"github.com/sturlabragason/fleet/internal/conf"
)

// DefaultReadyTimeout is how long spawn waits for a TUI to accept input.
// Spawn is synchronous for up to this long, including the MCP `fleet_spawn`
// tool call, so it is kept comfortably under a typical MCP client tool
// timeout rather than at the 30s mark.
const DefaultReadyTimeout = 15 * time.Second

// Spawn holds the effective preferences for one profile.
type Spawn struct {
	Mode         string        // "" = leave the tool's default
	AutoSubmit   bool          // press Enter after a pasted prompt
	ReadyTimeout time.Duration // how long to wait for the TUI
	ExtraArgs    []string      // appended to the launch argv
}

// Neutral is the built-in default: no mode, auto-submit, DefaultReadyTimeout.
func Neutral() Spawn { return Spawn{AutoSubmit: true, ReadyTimeout: DefaultReadyTimeout} }

// Prefs is a loaded config file.
type Prefs struct {
	Path     string // file it came from (may not exist)
	defaults Spawn
	profiles map[string]map[string]conf.Value
}

// Load reads the config file. A missing file is not an error.
func Load() (*Prefs, error) {
	path := Path()
	d, err := conf.ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	p := &Prefs{Path: path, defaults: Neutral(), profiles: map[string]map[string]conf.Value{}}
	if sec, ok := d["defaults"]; ok {
		p.defaults = applySpawn(p.defaults, sec)
	}
	for name, sec := range d {
		if n, ok := strings.CutPrefix(name, "profile."); ok && n != "" {
			p.profiles[n] = sec
		}
	}
	return p, nil
}

// For returns the effective preferences for a profile:
// [profile.<name>] > [defaults] > built-in.
func (p *Prefs) For(profile string) Spawn {
	if p == nil {
		return Neutral()
	}
	s := p.defaults
	if sec, ok := p.profiles[profile]; ok {
		s = applySpawn(s, sec)
	}
	return s
}

// ToolsDoc returns the [profile.<name>] sections re-keyed to profile names, so
// tools.LoadWith can use them as a third profile override layer.
func (p *Prefs) ToolsDoc() conf.Doc {
	if p == nil {
		return nil
	}
	d := conf.Doc{}
	for name, sec := range p.profiles {
		d[name] = sec
	}
	return d
}

func applySpawn(s Spawn, sec map[string]conf.Value) Spawn {
	if v, ok := sec["mode"]; ok {
		s.Mode, _ = v.(string)
	}
	if v, ok := conf.Bool(sec, "auto_submit"); ok {
		s.AutoSubmit = v
	}
	if v, ok := conf.Dur(sec, "ready_timeout"); ok && v > 0 {
		s.ReadyTimeout = v
	}
	if v, ok := sec["extra_args"]; ok {
		s.ExtraArgs, _ = v.([]string)
	}
	return s
}
