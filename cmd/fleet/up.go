package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/tools"
	"github.com/sturlabragason/fleet/internal/wall"
)

// supervisorCmd returns the shell command and dir for the supervisor pane.
func supervisorCmd(f *fleet.Fleet) (string, string, []string, error) {
	prof := os.Getenv("FLEET_SUPERVISOR_PROFILE")
	if prof == "" {
		prof = "copilot"
	}
	p, ok := f.Profiles[prof]
	if !ok {
		return "", "", nil, fmt.Errorf("supervisor profile %q not found", prof)
	}
	dir := os.Getenv("FLEET_SUPERVISOR_DIR")
	if dir == "" {
		dir = filepath.Join(filepath.Dir(filepath.Dir(f.Exe)), "supervisor")
	}
	id := ""
	if len(p.LaunchID) > 0 {
		id = tools.NewUUID()
	}
	argv := p.Argv(tools.Vars{Fleet: f.Exe, Name: "supervisor", ID: id}, false)
	argv = append(argv, supervisorMCPArgs(p)...)
	logx.For("up").Info("supervisor", "profile", prof, "dir", dir, "argv", argv)
	return tools.ShellCommand(argv), dir, p.EnvList(), nil
}

// supervisorMCPArgs makes a copilot supervisor lean: only the fleet MCP tools
// (loaded from supervisor/.mcp.json) are available and pre-approved, and the
// global instructions and skills are skipped. The supervisor rules arrive as
// the fleet MCP server instructions. Measured: ~8k input tokens per turn
// instead of ~160k, and no reading of Go code or shelling out.
func supervisorMCPArgs(p tools.Profile) []string {
	if p.Patterns != "copilot" {
		return nil
	}
	return append([]string{
		"--no-custom-instructions",
		"--allow-all-mcp-server-instructions",
		"--available-tools", "fleet",
		"--allow-tool", "fleet",
		"--disable-builtin-mcps",
	}, disableUserMCPArgs(userMCPConfig())...)
}

func userMCPConfig() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".copilot", "mcp-config.json")
}

// disableUserMCPArgs disables the user's own MCP servers for the supervisor so
// their tools don't load (and don't flood the startup "Disabled tools" notice).
func disableUserMCPArgs(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(b, &cfg) != nil {
		return nil
	}
	names := make([]string, 0, len(cfg.MCPServers))
	for n := range cfg.MCPServers {
		if n != "fleet" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var args []string
	for _, n := range names {
		args = append(args, "--disable-mcp-server", n)
	}
	return args
}

func cmdUp(f *fleet.Fleet, args []string) error {
	t := f.T
	log := logx.For("up")
	noAttach := len(args) > 0 && args[0] == "--no-attach"
	target := fleet.Session + ":main"
	if _, err := t.Run("has-session", "-t", "="+fleet.Session); err != nil || !windowExists(f, "main") {
		cmd, dir, env, err := supervisorCmd(f)
		if err != nil {
			return err
		}
		var a []string
		if t.HasSession(fleet.Session) {
			a = []string{"new-window", "-d", "-t", fleet.Session + ":", "-n", "main"}
		} else {
			a = []string{"new-session", "-d", "-s", fleet.Session, "-n", "main", "-x", "300", "-y", "80"}
		}
		a = append(a, "-P", "-F", "#{pane_id}", "-c", dir)
		for _, e := range env {
			a = append(a, "-e", e)
		}
		sup, err := t.Run(append(a, cmd)...)
		if err != nil {
			return err
		}
		t.SetPaneOption(sup, "@role", "sup")
		t.SetPaneOption(sup, "remain-on-exit", "on")
		log.Info("created main window", "sup", sup)
	}
	ensureOverview(f, target)
	configure(f)
	w := wall.New(t, fleet.Session)
	if err := w.Apply(); err != nil {
		log.Error("apply", "err", err)
	}
	if noAttach || !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Println("fleet is up. attach with: tmux -L fleet attach -t fleet:main")
		return nil
	}
	t.Run("select-window", "-t", target)
	tm, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	env := []string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "TMUX=") {
			env = append(env, e)
		}
	}
	return syscall.Exec(tm, []string{"tmux", "-L", fleet.Socket, "attach-session", "-t", target}, env)
}

func windowExists(f *fleet.Fleet, name string) bool {
	out, _ := f.T.Run("list-windows", "-t", fleet.Session, "-F", "#{window_name}")
	for _, l := range strings.Split(out, "\n") {
		if l == name {
			return true
		}
	}
	return false
}

func ensureOverview(f *fleet.Fleet, target string) {
	t := f.T
	panes, _ := t.ListPanes(target)
	uiCmd := tools.ShellCommand([]string{f.Exe, "ui"})
	for _, p := range panes {
		if p.Role == "ov" {
			if p.Dead {
				t.Run("respawn-pane", "-k", "-t", p.ID, uiCmd)
			}
			return
		}
	}
	for _, p := range panes {
		if p.Role == "sup" && p.Dead {
			t.Run("respawn-pane", "-t", p.ID)
		}
	}
	ov, err := t.Run("split-window", "-d", "-h", "-t", target, "-P", "-F", "#{pane_id}", uiCmd)
	if err != nil {
		return
	}
	t.SetPaneOption(ov, "@role", "ov")
	t.SetPaneOption(ov, "remain-on-exit", "on")
}

// configure sets server options, key bindings, hooks and border polish.
func configure(f *fleet.Fleet) {
	t := f.T
	exe := tools.Quote(f.Exe)
	wallCmd := func(a string) string { return "run-shell -b '" + f.Exe + " wall " + a + "'" }
	_ = exe
	set := [][]string{
		{"set-option", "-g", "mouse", "on"},
		{"set-option", "-s", "escape-time", "10"},
		{"set-option", "-g", "pane-border-status", "top"},
		{"set-option", "-g", "pane-border-lines", "heavy"},
		{"set-option", "-g", "pane-active-border-style", "fg=#58a6ff"},
		{"set-option", "-g", "pane-border-format",
			" #{?#{==:#{@role},sup},#[fg=#a371f7]#[bold]supervisor#[default],#{?#{==:#{@role},ov},#[bold]overview#[default],#[fg=#{?@fleet_color,#{@fleet_color},default}]#{@fleet_glyph} #{@fleet_agent} #{@fleet_status}#[default]}} "},
		{"set-option", "-g", "window-style", "fg=#a8a8a8"},
		{"set-option", "-g", "window-active-style", "fg=default"},
		{"set-option", "-g", "status-left", "#[bold] fleet #[default]"},
		{"set-option", "-g", "status-right", "M-arrows panes · M-x hide pane · M-z zoom · M-s sup · M-S side · M-1/2/4/6/9/m wall "},
		{"set-option", "-g", "status-right-length", "110"},
		{"set-hook", "-g", "client-resized", wallCmd("apply")},
		{"set-hook", "-g", "client-attached", wallCmd("apply")},
		{"bind-key", "-n", "M-Left", "select-pane", "-L"},
		{"bind-key", "-n", "M-Right", "select-pane", "-R"},
		{"bind-key", "-n", "M-Up", "select-pane", "-U"},
		{"bind-key", "-n", "M-Down", "select-pane", "-D"},
		{"bind-key", "-n", "M-z", "resize-pane", "-Z"},
		{"bind-key", "-n", "M-x", wallCmd("hide-focused")},
		{"bind-key", "-n", "M-s", wallCmd("sup")},
		{"bind-key", "-n", "M-S", wallCmd("side")},
		{"bind-key", "-n", "M-c", wallCmd("chips")},
		{"bind-key", "-n", "M-o", "select-pane", "-t", "{marked}"},
	}
	for _, p := range []string{"1", "2", "4", "6", "9", "m"} {
		set = append(set, []string{"bind-key", "-n", "M-" + p, wallCmd("preset " + p)})
	}
	for _, a := range set {
		t.Run(a...)
	}
}

func cmdWall(f *fleet.Fleet, args []string) error {
	w := wall.New(f.T, fleet.Session)
	if len(args) == 0 {
		return fmt.Errorf("usage: fleet wall apply|preset N|sup|side|chips|show NAME|hide NAME")
	}
	switch args[0] {
	case "apply":
		return w.Apply()
	case "preset":
		if len(args) < 2 {
			return fmt.Errorf("usage: fleet wall preset 1|2|4|6|9|m")
		}
		return w.SetPreset(args[1])
	case "sup":
		return w.ToggleSup()
	case "side":
		return w.SwapSide()
	case "chips":
		return w.ToggleChips()
	case "hide-focused":
		return w.HideFocused()
	case "show", "hide":
		if len(args) < 2 {
			return fmt.Errorf("usage: fleet wall %s NAME", args[0])
		}
		p, err := f.Pane(args[1])
		if err != nil {
			return err
		}
		if args[0] == "hide" {
			return w.Hide(p)
		}
		return w.Show(p)
	}
	return fmt.Errorf("unknown wall command %q", args[0])
}
