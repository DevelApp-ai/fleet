// Package playbook holds the fleet worker playbook: the standard process rules
// prepended to an agent's first prompt at spawn (DESIGN §4, ADR-22). The
// built-in text can be extended or replaced per user (~/.fleet/worker.md) and
// per repo (.fleet/worker.md).
package playbook

import (
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sturlabragason/fleet/internal/logx"
)

// ReplaceMarker as the first line of a layer replaces everything before it
// instead of appending. A layer with nothing after the marker turns the
// playbook off.
const ReplaceMarker = "<!-- fleet:replace -->"

// BuiltIn is the source name of the embedded default.
const BuiltIn = "built-in"

//go:embed worker.md
var worker string

// Default returns the built-in playbook.
func Default() string { return worker }

// Resolve returns the effective playbook for an agent working in dir: the
// built-in text, then <home>/worker.md, then the nearest .fleet/worker.md at
// or above dir within the repo. sources lists what was used, in order.
func Resolve(home, dir string) (text string, sources []string) {
	text, sources = Default(), []string{BuiltIn}
	files := []string{filepath.Join(home, "worker.md")}
	if p := repoFile(dir); p != "" {
		files = append(files, p)
	}
	for _, p := range files {
		layer, ok := read(p)
		if !ok {
			continue
		}
		text = apply(text, layer)
		sources = append(sources, p)
	}
	return text, sources
}

// apply layers one file's text onto the playbook so far.
func apply(text, layer string) string {
	first, rest, _ := strings.Cut(layer, "\n")
	if strings.TrimSpace(first) == ReplaceMarker {
		return strings.TrimSpace(rest)
	}
	if strings.TrimSpace(text) == "" {
		return layer
	}
	return strings.TrimRight(text, "\n") + "\n\n" + layer
}

// read returns a layer file's text; ok is false when it is missing, empty or
// unreadable.
func read(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logx.For("playbook").Warn("unreadable; skipped", "path", path, "err", err)
		}
		return "", false
	}
	s := strings.TrimSpace(string(b))
	return s, s != ""
}

// repoFile walks up from dir to the first directory holding a .git entry (a
// worktree has a .git file) or the filesystem root, and returns the first
// .fleet/worker.md it finds on the way, or "".
func repoFile(dir string) string {
	if dir == "" {
		return ""
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	for {
		p := filepath.Join(dir, ".fleet", "worker.md")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Compose prepends the playbook to a first prompt. An empty prompt or an
// empty playbook leaves the prompt unchanged.
func Compose(playbook, prompt string) string {
	if prompt == "" || playbook == "" {
		return prompt
	}
	return strings.TrimSpace(playbook) + "\n\n## Your task\n\n" + prompt
}
