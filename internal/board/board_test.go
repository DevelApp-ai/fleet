package board

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestUpdateLoad(t *testing.T) {
	s := NewStore(t.TempDir())
	if err := s.Update(func(b *Board) error { b.Agents["a"] = &Agent{Name: "a", Status: "idle"}; return nil }); err != nil {
		t.Fatal(err)
	}
	if s.Load().Agents["a"].Status != "idle" {
		t.Fatal("not saved")
	}
}

func TestConcurrent(t *testing.T) {
	s := NewStore(t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Update(func(b *Board) error { b.Agents[string(rune('a'+i))] = &Agent{}; return nil })
		}(i)
	}
	wg.Wait()
	if n := len(s.Load().Agents); n != 20 {
		t.Fatalf("lost updates: %d", n)
	}
}

func TestBrokenJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "board.json"), []byte("{nope"), 0o644)
	s := NewStore(dir)
	if len(s.Load().Agents) != 0 {
		t.Fatal("want empty")
	}
	if _, err := os.Stat(filepath.Join(dir, "board.json.broken")); err != nil {
		t.Fatal("no backup")
	}
}
