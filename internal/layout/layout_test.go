package layout

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestChecksumKnown(t *testing.T) {
	// From a real tmux 3.x session: "bb62,159x48,0,0{79x48,0,0,79x48,80,0}"
	if got := Checksum("159x48,0,0{79x48,0,0,79x48,80,0}"); got != "bb62" {
		t.Errorf("checksum %s", got)
	}
}

func TestBuildShapes(t *testing.T) {
	c := Default
	cases := []struct {
		name   string
		in     Input
		leaves []string
		want   string
	}{
		{"no wall", Input{W: 300, H: 80, Sup: "%1", Ov: "%2"}, []string{"%1", "%2"},
			"300x80,0,0{90x80,0,0,1,209x80,91,0,2}"},
		{"narrow sup", Input{W: 200, H: 50, Sup: "%1", Ov: "%2"}, []string{"%1", "%2"},
			"200x50,0,0{64x50,0,0,1,135x50,65,0,2}"},
		{"4 wall", Input{W: 300, H: 80, Sup: "%1", Ov: "%2", Wall: []string{"%3", "%4", "%5", "%6"}, Preset: "4"},
			[]string{"%1", "%2", "%3", "%4", "%5", "%6"}, ""},
		{"no sup", Input{W: 300, H: 80, Ov: "%2", Wall: []string{"%3"}, Preset: "1"}, []string{"%2", "%3"},
			"300x80,0,0[300x14,0,0,2,300x65,0,15,3]"},
	}
	for _, tc := range cases {
		s, leaves := c.Render(tc.in)
		if !reflect.DeepEqual(leaves, tc.leaves) {
			t.Errorf("%s leaves %v", tc.name, leaves)
		}
		if tc.want != "" && !strings.HasSuffix(s, ","+tc.want) {
			t.Errorf("%s: %s", tc.name, s)
		}
	}
	right := Default
	right.SupSide = "right"
	_, leaves := right.Render(Input{W: 300, H: 80, Sup: "%1", Ov: "%2"})
	if leaves[1] != "%1" {
		t.Errorf("right side leaves %v", leaves)
	}
}

// Applies generated layouts to a real tmux window on a throwaway socket.
func TestApplyInTmux(t *testing.T) {
	sock := fmt.Sprintf("fleet-test-%d", rand.Int63())
	tm := func(a ...string) (string, error) {
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", "/dev/null"}, a...)...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	t.Cleanup(func() { p, _ := tm("display", "-p", "#{socket_path}"); tm("kill-server"); os.Remove(p) })
	if _, err := tm("new-session", "-d", "-s", "t", "-x", "300", "-y", "80", "cat"); err != nil {
		t.Skip("no tmux")
	}
	for i := 0; i < 6; i++ {
		tm("split-window", "-t", "t", "cat")
		tm("select-layout", "-t", "t", "even-vertical")
	}
	ids, _ := tm("list-panes", "-t", "t", "-F", "#{pane_id}")
	p := strings.Split(ids, "\n")
	for _, preset := range []string{"1", "2", "4", "6", "9", "m"} {
		wall := p[2:]
		s, _ := Default.Render(Input{W: 300, H: 80, Sup: p[0], Ov: p[1], Wall: wall, Preset: preset})
		if out, err := tm("select-layout", "-t", "t", s); err != nil {
			t.Errorf("preset %s: %v %s (%s)", preset, err, out, s)
		}
		w, _ := tm("display", "-p", "-t", p[0], "#{pane_width}")
		if w != "90" {
			t.Errorf("preset %s: sup width %s", preset, w)
		}
	}
}
