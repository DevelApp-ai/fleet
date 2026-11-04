// Package tools holds data-driven tool profiles (built-in defaults merged with
// ~/.fleet/tools.toml) and builds the spawn/resume command lines.
package tools

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sturlabragason/fleet/internal/conf"
)

// Profile describes how to launch one tool variant.
type Profile struct {
	Name     string
	Command  string
	Args     []string
	Env      map[string]string
	LaunchID []string // args to set a session id at launch, with {id}; empty = not supported
	Resume   []string // args to resume, with {id}
	Patterns string   // status pattern set name
}

// Defaults are the built-in profiles. {fleet} is the fleet executable, {name} the agent name.
const Defaults = `
[copilot]
command = "copilot"
launch_id = ["--session-id", "{id}"]
resume = ["--resume", "{id}"]
patterns = "copilot"

[claude]
command = "claude"
launch_id = ["--session-id", "{id}"]
resume = ["--resume", "{id}"]
patterns = "claude"

[agy]
command = "agy"
resume = ["--conversation", "{id}"]
patterns = "agy"

[vibe]
command = "vibe"
resume = ["--resume", "{id}"]
patterns = "vibe"

[fake]
command = "{fleet}"
args = ["fakeagent", "{name}"]
patterns = "fake"
`

// Path is the user override file.
func Path(home string) string { return filepath.Join(home, "tools.toml") }

// Load returns built-in profiles overridden field-by-field by the user file.
func Load(home string) (map[string]Profile, error) {
	def, err := conf.Parse(Defaults)
	if err != nil {
		return nil, err
	}
	user, err := conf.ParseFile(Path(home))
	if err != nil {
		return nil, fmt.Errorf("tools.toml: %w", err)
	}
	out := map[string]Profile{}
	for _, d := range []conf.Doc{def, user} {
		for name, sec := range d {
			if name == "" {
				continue
			}
			p := out[name]
			p.Name = name
			if v := conf.Str(sec, "command"); v != "" {
				p.Command = v
			}
			if v, ok := sec["args"]; ok {
				p.Args, _ = v.([]string)
			}
			if v := conf.Map(sec, "env"); v != nil {
				p.Env = v
			}
			if v, ok := sec["launch_id"]; ok {
				p.LaunchID, _ = v.([]string)
			}
			if v, ok := sec["resume"]; ok {
				p.Resume, _ = v.([]string)
			}
			if v := conf.Str(sec, "patterns"); v != "" {
				p.Patterns = v
			}
			if p.Patterns == "" {
				p.Patterns = name
			}
			out[name] = p
		}
	}
	return out, nil
}

// Names returns sorted profile names.
func Names(ps map[string]Profile) []string {
	var n []string
	for k := range ps {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// Vars are substituted into command and args.
type Vars struct{ Fleet, Name, ID string }

func (v Vars) expand(s string) string {
	return strings.NewReplacer("{fleet}", v.Fleet, "{name}", v.Name, "{id}", v.ID).Replace(s)
}

// Argv builds the launch argv. If resume, uses Resume args; else LaunchID (when id != "").
func (p Profile) Argv(v Vars, resume bool, extra ...string) []string {
	argv := []string{v.expand(p.Command)}
	for _, a := range p.Args {
		argv = append(argv, v.expand(a))
	}
	switch {
	case resume:
		for _, a := range p.Resume {
			argv = append(argv, v.expand(a))
		}
	case v.ID != "":
		for _, a := range p.LaunchID {
			argv = append(argv, v.expand(a))
		}
	}
	return append(argv, extra...)
}

// ShellCommand wraps argv into `bash -lc 'exec ...'` so login PATH applies
// (aliases don't work in non-interactive spawns; profiles replace them).
func ShellCommand(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = Quote(a)
	}
	return "bash -lc " + Quote("exec "+strings.Join(q, " "))
}

// EnvList returns KEY=VAL pairs ($VAR expanded) including a PATH with ~/.local/bin first.
func (p Profile) EnvList() []string {
	home, _ := os.UserHomeDir()
	var out []string
	keys := make([]string, 0, len(p.Env))
	for k := range p.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+os.ExpandEnv(p.Env[k]))
	}
	return append(out, "PATH="+filepath.Join(home, ".local/bin")+":"+os.Getenv("PATH"))
}

// Quote single-quotes s for sh.
func Quote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r == '-' || r == '_' || r == '.' || r == '/' || r == '=' || r == ':' || r == '{' || r == '}' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// NewUUID returns a random v4 UUID.
func NewUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
