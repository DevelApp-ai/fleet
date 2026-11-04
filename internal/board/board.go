// Package board stores agent state in ~/.fleet/board.json (flock + atomic rename)
// and appends status transitions to events.jsonl.
package board

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/sturlabragason/fleet/internal/logx"
)

// Agent is one board entry.
type Agent struct {
	Name        string    `json:"name"`
	Profile     string    `json:"profile"`
	Patterns    string    `json:"patterns"`
	Dir         string    `json:"dir"`
	Goal        string    `json:"goal,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	PaneID      string    `json:"pane_id"`
	Status      string    `json:"status"`
	StatusSince time.Time `json:"status_since"`
	LastLine    string    `json:"last_line"`
	Hash        uint64    `json:"hash"`
	LastChange  time.Time `json:"last_change"`
	Created     time.Time `json:"created"`
}

// Board is the whole file.
type Board struct {
	Agents map[string]*Agent `json:"agents"`
}

// Store reads and writes the board under a directory.
type Store struct{ Dir string }

// NewStore returns a store rooted at dir.
func NewStore(dir string) *Store { return &Store{Dir: dir} }

func (s *Store) path() string { return filepath.Join(s.Dir, "board.json") }

// Load reads the board. Broken JSON is logged, backed up and treated as empty.
func (s *Store) Load() *Board {
	b := &Board{Agents: map[string]*Agent{}}
	data, err := os.ReadFile(s.path())
	if err != nil {
		return b
	}
	if err := json.Unmarshal(data, b); err != nil {
		logx.For("board").Error("broken board.json, starting empty", "err", err)
		_ = os.WriteFile(s.path()+".broken", data, 0o644)
		return &Board{Agents: map[string]*Agent{}}
	}
	if b.Agents == nil {
		b.Agents = map[string]*Agent{}
	}
	return b
}

// Update runs fn on the board under an exclusive lock and saves the result.
func (s *Store) Update(fn func(*Board) error) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	lf, err := os.OpenFile(filepath.Join(s.Dir, "board.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
	b := s.Load()
	if err := fn(b); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(b, "", "  ")
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path())
}

// Sorted returns agents sorted by name.
func (b *Board) Sorted() []*Agent {
	var out []*Agent
	for _, a := range b.Agents {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Event is one line of events.jsonl.
type Event struct {
	Time  time.Time `json:"t"`
	Agent string    `json:"agent"`
	From  string    `json:"from"`
	To    string    `json:"to"`
}

// AppendEvent appends a transition to events.jsonl.
func (s *Store) AppendEvent(e Event) {
	f, err := os.OpenFile(filepath.Join(s.Dir, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	data, _ := json.Marshal(e)
	f.Write(append(data, '\n'))
}
