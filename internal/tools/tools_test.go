package tools

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultsAndArgv(t *testing.T) {
	ps, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		profile string
		resume  bool
		id      string
		want    []string
		patt    string
	}{
		{"copilot", false, "U", []string{"copilot", "--session-id", "U"}, "copilot"},
		{"copilot", true, "U", []string{"copilot", "--resume", "U"}, "copilot"},
		{"claude", false, "U", []string{"claude", "--session-id", "U"}, "claude"},
		{"agy", true, "C", []string{"agy", "--conversation", "C"}, "agy"},
		{"agy", false, "C", []string{"agy"}, "agy"},
		{"vibe", true, "V", []string{"vibe", "--resume", "V"}, "vibe"},
		{"fake", false, "", []string{"/bin/fleet", "fakeagent", "a1"}, "fake"},
	}
	for _, c := range cases {
		p := ps[c.profile]
		got := p.Argv(Vars{Fleet: "/bin/fleet", Name: "a1", ID: c.id}, c.resume)
		if !reflect.DeepEqual(got, c.want) || p.Patterns != c.patt {
			t.Errorf("%s resume=%v: %v (%s) want %v", c.profile, c.resume, got, p.Patterns, c.want)
		}
	}
}

func TestExampleFile(t *testing.T) {
	b, err := os.ReadFile("../../examples/tools.toml")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tools.toml"), b, 0o644)
	ps, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := ps["claude-work"].Argv(Vars{ID: "U"}, false); !reflect.DeepEqual(got, []string{"claude", "--model", "claude-sonnet-5", "--session-id", "U"}) {
		t.Errorf("claude-work: %v", got)
	}
	t.Setenv("HOME", "/h")
	if env := ps["claude-personal"].EnvList(); env[0] != "CLAUDE_CONFIG_DIR=/h/.claude-personal" {
		t.Errorf("env not expanded: %v", env)
	}
}

func TestUserOverride(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tools.toml"), []byte("[copilot]\nargs = [\"--x\"]\n[mine]\ncommand = \"foo\"\npatterns = \"claude\"\n"), 0o644)
	ps, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := ps["copilot"].Argv(Vars{}, false); !reflect.DeepEqual(got, []string{"copilot", "--x"}) {
		t.Errorf("override: %v", got)
	}
	if ps["mine"].Command != "foo" || ps["mine"].Patterns != "claude" {
		t.Errorf("new profile: %+v", ps["mine"])
	}
}

func TestShellCommand(t *testing.T) {
	got := ShellCommand([]string{"copilot", "it's"})
	if !strings.HasPrefix(got, "bash -lc ") || !strings.Contains(got, "exec copilot") {
		t.Error(got)
	}
}
