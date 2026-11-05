package keys

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHintExact(t *testing.T) {
	cases := map[Scope]string{
		ScopeTmux:     "Alt-←↑↓→ switch pane · Alt-z zoom · Alt-x hide · Alt-s admiral · Alt-g what's new · Alt-? all keys",
		ScopeOverview: "←↑↓→ select · ⏎ show on wall · x take off wall · Tab next waiting · g what's new · ? all keys",
	}
	for sc, want := range cases {
		if got := Hint(sc, 200); got != want {
			t.Errorf("scope %d:\n got %q\nwant %q", sc, got, want)
		}
		if strings.Contains(Hint(sc, 200), "M-") {
			t.Errorf("scope %d hint uses tmux M- notation", sc)
		}
	}
}

func TestHintFitsAndKeepsAllKeys(t *testing.T) {
	for _, sc := range []Scope{ScopeTmux, ScopeOverview} {
		for w := 20; w <= 100; w += 7 {
			h := Hint(sc, w)
			if ansi.StringWidth(h) > w {
				t.Errorf("scope %d width %d: %q is %d wide", sc, w, h, ansi.StringWidth(h))
			}
			if !strings.HasSuffix(h, "all keys") {
				t.Errorf("scope %d width %d: dropped the all-keys item: %q", sc, w, h)
			}
		}
		if h := Hint(sc, 1); !strings.HasSuffix(h, "all keys") {
			t.Errorf("tiny width must still return the all-keys item: %q", h)
		}
	}
	// drops from the end of the middle, keeps the start
	if h := Hint(ScopeTmux, 40); h != "Alt-←↑↓→ switch pane · Alt-? all keys" {
		t.Errorf("got %q", h)
	}
}

func TestHelpGroupsAndRows(t *testing.T) {
	for _, w := range []int{0, PopupWidth() - 4} {
		h := Help(w)
		var heads []string
		for _, l := range strings.Split(h, "\n") {
			for _, g := range Groups {
				if l == string(g) {
					heads = append(heads, l)
				}
			}
		}
		if got := strings.Join(heads, ","); got != "Panes,Wall,Review,Admiral,Overview" {
			t.Errorf("width %d: groups %s", w, got)
		}
		flat := strings.Join(strings.Fields(h), " ")
		for _, b := range Bindings {
			if !strings.Contains(h, b.Key) || !strings.Contains(flat, b.Desc) {
				t.Errorf("width %d: missing %s %q", w, b.Key, b.Desc)
			}
		}
		if !strings.HasSuffix(h, Footer) {
			t.Errorf("width %d: footer missing", w)
		}
		if w > 0 {
			for _, l := range strings.Split(h, "\n") {
				if ansi.StringWidth(l) > w {
					t.Errorf("line wider than %d: %q", w, l)
				}
			}
		}
		if strings.Contains(h, "\x1b") {
			t.Error("plain Help contains ANSI")
		}
	}
	if PopupHeight() != strings.Count(Help(0), "\n")+3 {
		t.Error("popup height")
	}
}

func TestTmuxKeys(t *testing.T) {
	k := TmuxKeys()
	for _, want := range []string{"M-?", "M-Left", "M-z", "M-x", "M-o", "M-1", "M-9", "M-m", "M-s", "M-S", "M-c", "M-w", "M-g"} {
		if !k[want] {
			t.Errorf("missing %s", want)
		}
	}
	if k["M-h"] {
		t.Error("M-h must not be claimed")
	}
}

// TestPopupFitsHelp: at the popup's inner width (border and margins off) the
// help wraps nothing, so the popup shows every line as written.
func TestPopupFitsHelp(t *testing.T) {
	inner := PopupWidth() - 2 - 2*PopupMargin
	if got, want := Help(inner), Help(0); got != want {
		t.Errorf("Help wraps at popup inner width %d:\n%s", inner, got)
	}
	if Help(inner-1) == Help(0) {
		t.Errorf("popup width %d is wider than the longest line needs", PopupWidth())
	}
	if PopupHeight() != len(strings.Split(Help(inner), "\n"))+2 {
		t.Error("popup height must be the help lines plus the border")
	}
}

func TestChordRowFirstInWall(t *testing.T) {
	for _, b := range Bindings {
		if b.Group == Wall {
			if b.Key != "Alt-w 1/2/4/6/9/m" || len(b.Tmux) != 1 || b.Tmux[0] != "M-w" {
				t.Errorf("first Wall row %+v", b)
			}
			break
		}
	}
	c := ChordKeys()
	for _, k := range []string{"1", "2", "4", "6", "9", "m"} {
		if !c[k] {
			t.Errorf("chord key %s missing", k)
		}
	}
	h := Help(0)
	if !strings.Contains(h, "\n  Alt-w 1/2/4/6/9/m\n                same as Alt-1") {
		t.Errorf("long key should sit on its own line above its description:\n%s", h)
	}
}

func TestHelpColumns(t *testing.T) {
	h := ansi.Strip(HelpColumns(120))
	lines := strings.Split(h, "\n")
	if !strings.HasPrefix(lines[0], "Panes") || !strings.Contains(lines[0], "Admiral") {
		t.Errorf("first line should head both columns: %q", lines[0])
	}
	if lines[len(lines)-1] != Footer {
		t.Errorf("footer last: %q", lines[len(lines)-1])
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 120 {
			t.Errorf("line wider than 120: %q", l)
		}
	}
}

// TestDesignTableInSync fails when the DESIGN.md keys block drifts from the
// registry. Regenerate with `fleet keys --markdown`.
func TestDesignTableInSync(t *testing.T) {
	b, err := os.ReadFile("../../DESIGN.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	const begin, end = "<!-- keys:begin -->\n", "<!-- keys:end -->"
	i, j := strings.Index(s, begin), strings.Index(s, end)
	if i < 0 || j < i {
		t.Fatal("DESIGN.md keys markers missing")
	}
	if got := s[i+len(begin) : j]; got != Markdown() {
		t.Errorf("DESIGN.md keys block is stale; replace it with `fleet keys --markdown` output.\n got:\n%s\nwant:\n%s", got, Markdown())
	}
}
