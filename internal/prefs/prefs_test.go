package prefs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLEET_CONFIG", p)
	return p
}

func TestMissingFileIsNeutral(t *testing.T) {
	t.Setenv("FLEET_CONFIG", filepath.Join(t.TempDir(), "nope.toml"))
	p, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.For("copilot"); !reflect.DeepEqual(got, Neutral()) {
		t.Errorf("%+v want %+v", got, Neutral())
	}
	if p.ToolsDoc() == nil || len(p.ToolsDoc()) != 0 {
		t.Errorf("tools doc %v", p.ToolsDoc())
	}
}

func TestPathPrecedence(t *testing.T) {
	t.Setenv("FLEET_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := Path(); got != "/xdg/fleet/config.toml" {
		t.Errorf("xdg path %q", got)
	}
	t.Setenv("FLEET_CONFIG", "/tmp/x.toml")
	if got := Path(); got != "/tmp/x.toml" {
		t.Errorf("env path %q", got)
	}
}

func TestDefaultsAndProfileOverride(t *testing.T) {
	write(t, `
[defaults]
mode = "autopilot"
auto_submit = false
ready_timeout = "5s"

[profile.copilot]
mode = "plan"
extra_args = ["--reasoning-effort", "high"]

[profile.my-copilot]
command = "copilot"
patterns = "copilot"
`)
	p, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	// [defaults] reaches a profile with no section of its own.
	got := p.For("claude")
	if got.Mode != "autopilot" || got.AutoSubmit || got.ReadyTimeout != 5*time.Second {
		t.Errorf("defaults: %+v", got)
	}
	// [profile.x] wins over [defaults], and inherits what it doesn't set.
	got = p.For("copilot")
	if got.Mode != "plan" || got.AutoSubmit || got.ReadyTimeout != 5*time.Second {
		t.Errorf("profile override: %+v", got)
	}
	if !reflect.DeepEqual(got.ExtraArgs, []string{"--reasoning-effort", "high"}) {
		t.Errorf("extra_args %v", got.ExtraArgs)
	}
	// Profile sections are also a tools override layer.
	d := p.ToolsDoc()
	if _, ok := d["my-copilot"]; !ok {
		t.Errorf("tools doc missing my-copilot: %v", d)
	}
	if _, ok := d["defaults"]; ok {
		t.Error("[defaults] leaked into the tools doc")
	}
}

func TestBadTOMLNamesTheFile(t *testing.T) {
	path := write(t, "[defaults]\nthis is not toml\n")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %v, want it to name %s", err, path)
	}
}

func TestNilPrefsIsNeutral(t *testing.T) {
	var p *Prefs
	if got := p.For("copilot"); !reflect.DeepEqual(got, Neutral()) {
		t.Errorf("%+v", got)
	}
	if p.ToolsDoc() != nil {
		t.Error("nil ToolsDoc")
	}
}

func TestEnvMergesAndMaxWorking(t *testing.T) {
	write(t, "[defaults]\nmax_working = 2\nenv = { A = \"1\", B = \"2\" }\n[profile.copilot]\nenv = { B = \"3\" }\n")
	p, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.For("copilot").Env; !reflect.DeepEqual(got, map[string]string{"A": "1", "B": "3"}) {
		t.Errorf("copilot env %v", got)
	}
	if got := p.For("claude").Env; !reflect.DeepEqual(got, map[string]string{"A": "1", "B": "2"}) {
		t.Errorf("claude env %v", got)
	}
	if p.MaxWorking() != 2 {
		t.Errorf("max_working %d", p.MaxWorking())
	}
	write(t, "[defaults]\nmode = \"\"\n")
	if p, _ := Load(); p.MaxWorking() != DefaultMaxWorking {
		t.Errorf("default max_working %d", p.MaxWorking())
	}
	write(t, "[defaults]\nmax_working = -1\n")
	if _, err := Load(); err == nil {
		t.Error("negative max_working accepted")
	}
}

func TestPlaybookPref(t *testing.T) {
	if !Neutral().Playbook {
		t.Fatal("playbook should default to on")
	}
	write(t, "[defaults]\nplaybook = false\n")
	p, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if p.For("copilot").Playbook {
		t.Error("[defaults] playbook = false ignored")
	}
	write(t, "[profile.x]\nplaybook = false\n")
	if p, err = Load(); err != nil {
		t.Fatal(err)
	}
	if p.For("x").Playbook {
		t.Error("[profile.x] playbook = false ignored")
	}
	if !p.For("y").Playbook {
		t.Error("other profile lost the default")
	}
}

func TestDeadPaneGrace(t *testing.T) {
	write(t, `
[defaults]
dead_pane_grace = "2m"

[profile.zero]
dead_pane_grace = "0"

[profile.off]
dead_pane_grace = " Off "

[profile.bad]
dead_pane_grace = "-1m"
`)
	p, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if Neutral().DeadPaneGrace != 5*time.Minute {
		t.Errorf("default %v", Neutral().DeadPaneGrace)
	}
	for prof, want := range map[string]time.Duration{"copilot": 2 * time.Minute, "zero": 0, "off": GraceOff, "bad": 2 * time.Minute} {
		if got := p.For(prof).DeadPaneGrace; got != want {
			t.Errorf("%s: %v want %v", prof, got, want)
		}
	}
}
