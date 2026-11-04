package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestPopupClosesOnAnyKey(t *testing.T) {
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("a")},
		{Type: tea.KeyRunes, Runes: []rune("?"), Alt: true},
		{Type: tea.KeyEsc},
	} {
		_, cmd := popup{}.Update(k)
		if cmd == nil {
			t.Fatalf("%s did not close the popup", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%s: not a quit", k)
		}
	}
}

func TestPopupSizing(t *testing.T) {
	if v := (popup{}).View(); v != "" {
		t.Fatalf("before the first WindowSizeMsg the popup should draw nothing, got %q", v)
	}
	m, _ := popup{}.Update(tea.WindowSizeMsg{Width: 30, Height: 5})
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if len(lines) != 5 || lines[0] != " Panes" {
		t.Fatalf("small client must cut the bottom, keep the top: %q", lines)
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 30 {
			t.Errorf("line wider than client: %q", l)
		}
	}
}
