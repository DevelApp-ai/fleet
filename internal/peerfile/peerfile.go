// Package peerfile keeps the agent-scoped fleet MCP server inside a
// tool's project-local config file (ADR-64). Vibe has no CLI flag for
// extra servers: it reads them from .vibe/config.toml in the project
// root, a file the user may have authored, so fleet merges its
// [[mcp_servers]] table into the text and leaves every other byte alone.
package peerfile

import (
	"fmt"
	"os"
	"strings"

	"github.com/sturlabragason/fleet/internal/trust"
)

// Add merges an [[mcp_servers]] table into the file at path. The table is
// identified by its name line (Name): an existing table of the same name
// is replaced in place, so the server's command stays current, and an
// absent one is appended at the end. The rewrite is atomic (lock, temp
// file, rename) and safe against a concurrent write by the user or the
// tool. A missing parent directory is created; a file that cannot be
// written is an error the caller can log and skip (ADR-64).
func Add(path, block string) (changed bool, err error) {
	name := Name(block)
	if name == "" {
		return false, fmt.Errorf("peer server block has no name")
	}
	return trust.EditFile(path, func(old []byte) ([]byte, bool, error) {
		text := merge(string(old), name, block)
		if text == string(old) {
			return nil, false, nil
		}
		return []byte(text), true, nil
	})
}

// Remove deletes the [[mcp_servers]] table named name from the file at
// path. The file itself is deleted when nothing but whitespace remains,
// so one fleet wrote leaves no trace and one the user wrote keeps its
// other content. A missing file is a no-op.
func Remove(path, name string) (changed bool, err error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return trust.EditFile(path, func(old []byte) ([]byte, bool, error) {
		text := cut(string(old), name)
		if text == string(old) {
			return nil, false, nil
		}
		if strings.TrimSpace(text) == "" {
			return nil, true, nil // fleet's own file: nothing else in it
		}
		return []byte(text), true, nil
	})
}

// merge returns text with its [[mcp_servers]] table named name replaced
// by block, appended when absent. A second table of the same name is
// dropped, not duplicated.
func merge(text, name, block string) string {
	var b strings.Builder
	pos, found := 0, false
	for _, sp := range spans(text) {
		seg := text[sp[0]:sp[1]]
		if !isMcpServers(seg) || Name(seg) != name {
			continue
		}
		if found { // a leftover duplicate: drop it, keep one
			pos = sp[1]
			continue
		}
		b.WriteString(text[pos:sp[0]])
		b.WriteString(block)
		pos, found = sp[1], true
	}
	if !found {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return text + block
	}
	b.WriteString(text[pos:])
	return b.String()
}

// cut returns text without its [[mcp_servers]] tables named name.
func cut(text, name string) string {
	var b strings.Builder
	pos := 0
	for _, sp := range spans(text) {
		seg := text[sp[0]:sp[1]]
		if !isMcpServers(seg) || Name(seg) != name {
			continue
		}
		b.WriteString(text[pos:sp[0]])
		pos = sp[1]
	}
	b.WriteString(text[pos:])
	return b.String()
}

// line is one line of a config file, newline included.
type line struct{ lo, hi int }

// spans returns the [start, end) byte spans of the TOML tables in text:
// each runs from a [[...]] or [...] header line to just before the next
// header line, newlines included, minus the trailing blank lines, which
// belong to no table and must survive a replace untouched. Content
// before the first header belongs to no table. A line inside a
// multi-line string that happens to look like a header is mistaken for
// one; a project config holding that is beyond what fleet reads.
func spans(text string) [][2]int {
	var ls []line
	for off := 0; off < len(text); {
		hi := len(text)
		if i := strings.IndexByte(text[off:], '\n'); i >= 0 {
			hi = off + i + 1
		}
		ls = append(ls, line{off, hi})
		off = hi
	}
	var out [][2]int
	start := -1
	for i, l := range ls {
		if !isHeader(text[l.lo:l.hi]) {
			continue
		}
		if start >= 0 {
			out = append(out, spanEnd(text, ls, start, i))
		}
		start = i
	}
	if start >= 0 {
		out = append(out, spanEnd(text, ls, start, len(ls)))
	}
	return out
}

// spanEnd returns the [lo, hi) span of the table that starts at lines[s]
// and runs to lines[e) exclusive, without its trailing blank lines.
func spanEnd(text string, ls []line, s, e int) [2]int {
	blank := func(i int) bool { return strings.TrimSpace(text[ls[i].lo:ls[i].hi]) == "" }
	for e > s+1 && blank(e-1) {
		e--
	}
	return [2]int{ls[s].lo, ls[e-1].hi}
}

// isHeader reports whether line, which may carry its newline, opens a
// TOML table ([[array]] or [table]) with an optional trailing comment.
func isHeader(line string) bool {
	t := strings.TrimSpace(line)
	if i := strings.IndexByte(t, '#'); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	return strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") && len(t) > 2
}

// isMcpServers reports whether a table span opens with [[mcp_servers]].
func isMcpServers(span string) bool {
	h := span
	if i := strings.IndexByte(h, '\n'); i >= 0 {
		h = h[:i]
	}
	if !isHeader(h) || !strings.HasPrefix(strings.TrimSpace(h), "[[") {
		return false
	}
	h = strings.TrimSpace(h)
	if i := strings.IndexByte(h, '#'); i >= 0 {
		h = strings.TrimSpace(h[:i])
	}
	return strings.TrimSpace(h[2:len(h)-2]) == "mcp_servers"
}

// Name returns the server name of an [[mcp_servers]] table: the value of
// its `name = "..."` line, single or double quoted. "" when it has none.
func Name(block string) string {
	for _, line := range strings.Split(block, "\n") {
		t := strings.TrimSpace(line)
		if i := strings.IndexByte(t, '#'); i >= 0 {
			t = strings.TrimSpace(t[:i])
		}
		if !strings.HasPrefix(t, "name") {
			continue
		}
		v := strings.TrimLeft(t[len("name"):], " \t")
		if !strings.HasPrefix(v, "=") {
			continue
		}
		v = strings.TrimLeft(v[1:], " \t")
		if len(v) < 2 || (v[0] != '"' && v[0] != '\'') {
			continue
		}
		if i := strings.IndexByte(v[1:], v[0]); i >= 0 {
			return v[1 : 1+i]
		}
	}
	return ""
}
