// Package status classifies an agent from the bottom of its pane.
// Patterns are data (built-in TOML + ~/.fleet/patterns.toml), one set per tool.
package status

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sturlabragason/fleet/internal/conf"
	"github.com/sturlabragason/fleet/internal/logx"
)

// Status of an agent.
type Status string

const (
	Working Status = "working"
	Waiting Status = "waiting"
	Error   Status = "error"
	Stuck   Status = "stuck"
	Idle    Status = "idle"
	Unknown Status = "unknown"
	Gone    Status = "gone"
)

// DefaultPatterns: checked against the last `tail` non-empty lines, bottom-most
// match wins in order waiting > error > working > idle.
const DefaultPatterns = `
[fake]
waiting = ['\[y/n\]\s*$']
error = ['^✖ Error']
working = ['\(esc to interrupt\)', '^● Task:']
idle = ['^>\s*$', '^✔ Done']

[copilot]
waiting = ['\[y/n\]', 'Do you want to', '│ ❯ 1\. Yes', 'Allow .* to run', 'Approve']
error = ['^✖', '(?i)^error:']
working = ['(?i)esc to (interrupt|cancel)', '[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]', '(?i)thinking']
idle = ['^\s*[>❯]\s*$', '(?i)type @ to mention']

[claude]
waiting = ['Do you want to', '❯ 1\. Yes', '\[y/n\]']
error = ['(?i)^\s*(⎿\s*)?error', 'API Error']
working = ['(?i)esc to interrupt', '[✻✽✶✳✢·] \w+…']
idle = ['^\s*[>❯]\s*$', '\? for shortcuts']

[agy]
waiting = ['\[y/n\]', '(?i)approve', '(?i)allow']
error = ['(?i)^error']
working = ['(?i)esc to (interrupt|cancel)', '[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]']
idle = ['^\s*[>❯]\s*$']

[vibe]
waiting = ['\[y/n\]', '(?i)approve', '(?i)allow']
error = ['(?i)^error']
working = ['(?i)esc to (interrupt|cancel)', '[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]']
idle = ['^\s*[>❯]\s*$']
`

// Set holds compiled patterns for one tool.
type Set struct {
	Waiting, Error, Working, Idle []*regexp.Regexp
}

// Sets maps pattern-set name to Set.
type Sets map[string]Set

// Load parses built-ins then overrides from <home>/patterns.toml.
func Load(home string) Sets {
	log := logx.For("status")
	out := Sets{}
	def, _ := conf.Parse(DefaultPatterns)
	user, err := conf.ParseFile(filepath.Join(home, "patterns.toml"))
	if err != nil {
		log.Error("patterns.toml", "err", err)
	}
	for _, d := range []conf.Doc{def, user} {
		for name, sec := range d {
			if name == "" {
				continue
			}
			s := out[name]
			for k, dst := range map[string]*[]*regexp.Regexp{"waiting": &s.Waiting, "error": &s.Error, "working": &s.Working, "idle": &s.Idle} {
				if l, ok := sec[k].([]string); ok {
					*dst = compile(name, l)
				}
			}
			out[name] = s
		}
	}
	return out
}

func compile(set string, l []string) []*regexp.Regexp {
	var rs []*regexp.Regexp
	for _, p := range l {
		r, err := regexp.Compile(p)
		if err != nil {
			logx.For("status").Error("bad pattern", "set", set, "pattern", p, "err", err)
			continue
		}
		rs = append(rs, r)
	}
	return rs
}

// Tail returns the last n non-empty lines of text.
func Tail(text string, n int) []string {
	lines := strings.Split(strings.TrimRight(text, "\n "), "\n")
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			out = append([]string{strings.TrimRight(lines[i], " ")}, out...)
		}
	}
	return out
}

// Detect classifies pane text. ok=false when nothing matched.
// The bottom-most matching line decides; within one line, waiting > error > working > idle.
func (s Set) Detect(text string) (Status, bool) {
	lines := Tail(text, 6)
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		for _, c := range []struct {
			st Status
			rs []*regexp.Regexp
		}{{Waiting, s.Waiting}, {Error, s.Error}, {Working, s.Working}, {Idle, s.Idle}} {
			for _, r := range c.rs {
				if r.MatchString(l) {
					return c.st, true
				}
			}
		}
	}
	return Unknown, false
}
