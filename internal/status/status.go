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

// DefaultPatterns, per tool. Detect reads the last Window non-empty lines:
//
//  1. waiting: any match in the window wins. An approval box or question form
//     can sit above a footer that still says "Working", and its key hints are
//     the most reliable sign the tool is blocked on me. List the most
//     descriptive pattern first: the line it matches becomes waiting_for.
//     Lines matching `transcript` (the scrolling conversation) only count
//     when they are its last content line, so prompt text an agent merely
//     quoted earlier in its work doesn't make it look blocked.
//  2. otherwise the bottom-most line that matches error, working or idle
//     decides (within one line: error > working > idle). That is normally the
//     tool's footer, so text quoted in the transcript above it doesn't count.
//
// Copilot CLI (newer UI): busy = "● Working · … esc esc interrupt" in the
// footer; idle = the mode footer ("Interactive · Allow All · / commands ·
// space hold to record") with no "Working". Picker hints like "esc to cancel"
// mean waiting, never working.
const DefaultPatterns = `
[fake]
waiting = ['\[y/n\]\s*$']
error = ['^✖ Error']
working = ['\(esc to interrupt\)', '^● Task:']
idle = ['^>\s*$', '^✔ Done']

[copilot]
transcript = ['^[^┃].*┃\s*$']
waiting = ['needs information\.', 'Do you want to', '\[y/n\]', 'Allow .* to run', '❯ 1\. Yes', '○ Asking user', '↑/↓ (to )?(select|navigate) · enter']
error = ['^✖', '(?i)^error:']
working = ['(?i)esc (esc )?(to )?interrupt', '(?i)^\s*(\S+ · )*\S? ?working · ', '[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]', '(?i)^\s*\S? ?thinking']
idle = ['^\s*[>❯]\s*$', '(?i)type @ to mention', '(?i)hold to record', '(?i)^\s*(← open sidebar · )?(interactive|autopilot|plan) · ']

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
	Transcript                    []*regexp.Regexp // lines of the scrolling conversation
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
			for k, dst := range map[string]*[]*regexp.Regexp{"waiting": &s.Waiting, "error": &s.Error, "working": &s.Working, "idle": &s.Idle, "transcript": &s.Transcript} {
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

// Window is how many non-empty bottom lines Detect reads: enough for a
// Copilot question form or approval box plus the footer under it.
const Window = 14

// deadPane is tmux's remain-on-exit banner; it means exited whatever the tool.
var deadPane = regexp.MustCompile(`^Pane is dead \(`)

// Detect classifies pane text. ok=false when nothing matched.
func (s Set) Detect(text string) (Status, bool) {
	st, _, ok := s.Match(text)
	return st, ok
}

// Match is Detect plus the line that explains the status: for waiting, the
// prompt (e.g. "Do you want to run this command?"), else the matching line.
func (s Set) Match(text string) (Status, string, bool) {
	lines := Tail(text, Window)
	for _, l := range lines {
		if deadPane.MatchString(l) {
			return Error, strings.TrimSpace(l), true
		}
	}
	last := -1 // last transcript line with content
	skip := make([]bool, len(lines))
	for i, l := range lines {
		if matchAny(s.Transcript, l) {
			skip[i] = true
			if clean(l) != "" {
				last = i
			}
		}
	}
	if last >= 0 {
		skip[last] = false
	}
	for _, r := range s.Waiting {
		for i, l := range lines {
			if !skip[i] && r.MatchString(l) {
				return Waiting, describe(lines, i, s.Waiting), true
			}
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		for _, c := range []struct {
			st Status
			rs []*regexp.Regexp
		}{{Error, s.Error}, {Working, s.Working}, {Idle, s.Idle}} {
			for _, r := range c.rs {
				if r.MatchString(l) {
					return c.st, clean(l), true
				}
			}
		}
	}
	return Unknown, "", false
}

func matchAny(rs []*regexp.Regexp, l string) bool {
	for _, r := range rs {
		if r.MatchString(l) {
			return true
		}
	}
	return false
}

// describe returns lines[i] cleaned of box borders, plus the next line when
// that is plain text (the question under "Copilot needs information.").
func describe(lines []string, i int, waiting []*regexp.Regexp) string {
	d := clean(lines[i])
	if i+1 >= len(lines) {
		return d
	}
	next := clean(lines[i+1])
	if next == "" || strings.Trim(next, "─━-╌ ") == "" || strings.HasSuffix(d, "?") {
		return d
	}
	if matchAny(waiting, lines[i+1]) {
		return d
	}
	return d + " " + next
}

// clean trims whitespace and box-drawing borders from a pane line.
func clean(l string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "│┃║"))
}
