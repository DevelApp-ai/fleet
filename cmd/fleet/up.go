package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/keys"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/supervisor"
	"github.com/sturlabragason/fleet/internal/theme"
	"github.com/sturlabragason/fleet/internal/tools"
	"github.com/sturlabragason/fleet/internal/wall"
)

// supLaunch is how to start the supervisor pane.
type supLaunch struct {
	Cmd, Dir string
	Env      []string
	ID       string // session id, "" if the profile can't set one
	Profile  string
}

// supervisorCmd builds the supervisor command for profile ("": the
// FLEET_SUPERVISOR_PROFILE env, else copilot). A non-empty boot prompt is
// passed with -i to copilot-pattern profiles (used by supervisor reset).
func supervisorCmd(f *fleet.Fleet, prof, boot string) (supLaunch, error) {
	if prof == "" {
		prof = os.Getenv("FLEET_SUPERVISOR_PROFILE")
	}
	if prof == "" {
		prof = "copilot"
	}
	p, ok := f.Profiles[prof]
	if !ok {
		return supLaunch{}, fmt.Errorf("supervisor profile %q not found", prof)
	}
	dir := os.Getenv("FLEET_SUPERVISOR_DIR")
	if dir == "" {
		dir = filepath.Join(filepath.Dir(filepath.Dir(f.Exe)), "supervisor")
	}
	id := ""
	if len(p.LaunchID) > 0 {
		id = tools.NewUUID()
	}
	argv, _ := p.Argv(tools.Vars{Fleet: f.Exe, Name: "supervisor", ID: id}, tools.Launch{})
	argv = append(argv, supervisorMCPArgs(p)...)
	if boot != "" && p.Patterns == "copilot" {
		argv = append(argv, "-i", boot)
	}
	logx.For("up").Info("supervisor", "profile", prof, "dir", dir, "argv", argv)
	return supLaunch{Cmd: tools.ShellCommand(argv), Dir: dir, Env: p.EnvList(), ID: id, Profile: prof}, nil
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
		sl, err := supervisorCmd(f, "", "")
		if err != nil {
			return err
		}
		var a []string
		if t.HasSession(fleet.Session) {
			a = []string{"new-window", "-d", "-t", fleet.Session + ":", "-n", "main"}
		} else {
			a = []string{"new-session", "-d", "-s", fleet.Session, "-n", "main", "-x", "300", "-y", "80"}
		}
		a = append(a, "-P", "-F", "#{pane_id}", "-c", sl.Dir)
		for _, e := range sl.Env {
			a = append(a, "-e", e)
		}
		sup, err := t.Run(append(a, sl.Cmd)...)
		if err != nil {
			return err
		}
		t.SetPaneOption(sup, "@role", "sup")
		t.SetPaneOption(sup, "remain-on-exit", "on")
		if err := supervisor.Save(f.Home, supervisor.Session{SessionID: sl.ID, Profile: sl.Profile, PaneID: sup, Started: time.Now(), Generation: 1}); err != nil {
			log.Error("save supervisor session", "err", err)
		}
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
	for _, a := range tmuxCommands(f) {
		f.T.Run(a...)
	}
}

// tmuxCommands returns the tmux commands configure runs. Pure, so tests can
// check the key bindings against the keys registry without tmux.
func tmuxCommands(f *fleet.Fleet) [][]string {
	exe := tools.Quote(f.Exe)
	wallCmd := func(a string) string { return "run-shell -b '" + f.Exe + " wall " + a + "'" }
	set := [][]string{
		{"set-option", "-g", "mouse", "on"},
		{"set-option", "-s", "escape-time", "10"},
		{"set-option", "-g", "pane-border-status", "top"},
		{"set-option", "-g", "pane-border-lines", "heavy"},
		{"set-option", "-g", "pane-border-style", "fg=" + theme.Surface1},
		{"set-option", "-g", "pane-active-border-style", "fg=" + theme.Select},
		{"set-option", "-g", "pane-border-format",
			" #{?#{==:#{@role},sup},#[fg=" + theme.Supervisor + "]#[bold]supervisor#[default]#{?@fleet_ctx,#[fg=#{@fleet_ctx_color}] · ctx #{@fleet_ctx}#[default],},#{?#{==:#{@role},ov},#[fg=" + theme.Text + "]#[bold]overview#[default],#[fg=#{?@fleet_color,#{@fleet_color},default}]#{@fleet_glyph} #{@fleet_agent} #[fg=" + theme.Muted + "]#{@fleet_status}#[default]}} "},
		{"set-option", "-g", "window-style", "fg=" + theme.Subtext0 + ",bg=" + theme.Base},
		{"set-option", "-g", "window-active-style", "fg=" + theme.Text + ",bg=" + theme.Base},
		{"set-option", "-g", "status-style", "fg=" + theme.Text + ",bg=" + theme.Mantle},
		{"set-option", "-g", "message-style", "fg=" + theme.Text + ",bg=" + theme.Surface0},
		{"set-option", "-g", "message-command-style", "fg=" + theme.Text + ",bg=" + theme.Surface0},
		{"set-option", "-g", "mode-style", "fg=" + theme.Base + ",bg=" + theme.Lavender},
		{"set-option", "-g", "copy-mode-match-style", "fg=" + theme.Base + ",bg=" + theme.Yellow},
		{"set-option", "-g", "copy-mode-current-match-style", "fg=" + theme.Base + ",bg=" + theme.Peach},
		{"set-option", "-g", "display-panes-colour", theme.Overlay0},
		{"set-option", "-g", "display-panes-active-colour", theme.Select},
		{"set-option", "-g", "clock-mode-colour", theme.Mauve},
		{"set-option", "-g", "status-left", "#[fg=" + theme.Crust + ",bg=" + theme.Mauve + ",bold] fleet #[default] "},
		{"set-option", "-g", "status-left-length", "20"},
		{"set-option", "-g", "window-status-separator", ""},
		{"set-option", "-g", "window-status-format", "#[fg=" + theme.Overlay1 + "] #I #W "},
		{"set-option", "-g", "window-status-current-format", "#[fg=" + theme.Crust + ",bg=" + theme.Lavender + ",bold] #I #W #[default]"},
		{"set-option", "-g", "status-right", "#[fg=" + theme.Overlay1 + "]" + keys.Hint(keys.ScopeTmux, statusRightLen-1) + " "},
		{"set-option", "-g", "status-right-length", strconv.Itoa(statusRightLen)},
		{"set-hook", "-g", "client-resized", wallCmd("apply")},
		{"set-hook", "-g", "client-attached", wallCmd("apply")},
		// Focusing a wall pane is an explicit choice: re-tile so it gets
		// emphasis. This fires on every pane switch, so it takes the cached
		// path: an unchanged layout string costs one tmux round trip.
		{"set-hook", "-g", "pane-focus-in", wallCmd("refresh")},
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
		append([]string{"bind-key", "-n", "M-R"}, confirmResetArgs(f.Exe)...),
		{"bind-key", "-n", keys.HelpTmuxKey, "display-popup", "-E", "-b", "rounded",
			"-w", strconv.Itoa(keys.PopupWidth()), "-h", strconv.Itoa(keys.PopupHeight()),
			"-T", " fleet keys · any key closes ", exe + " keys --popup"},
	}
	for _, p := range []string{"1", "2", "4", "6", "9", "m"} {
		set = append(set, []string{"bind-key", "-n", "M-" + p, wallCmd("preset " + p)})
		// Chord for terminals that grab Alt+digit (Ptyxis/GNOME use it for tabs).
		set = append(set, []string{"bind-key", "-T", "fleetwall", p, wallCmd("preset " + p)})
	}
	set = append(set, []string{"bind-key", "-n", "M-w", "switch-client", "-T", "fleetwall", "\\;", "display-message", "wall layout: press 1 2 4 6 9 or m (release Alt first)"})
	return set
}

const statusRightLen = 110

func cmdWall(f *fleet.Fleet, args []string) error {
	w := wall.New(f.T, fleet.Session)
	if len(args) == 0 {
		return fmt.Errorf("usage: fleet wall apply|refresh|preset N|sup|side|chips|emphasis|show NAME|hide NAME")
	}
	switch args[0] {
	case "apply":
		return w.Apply()
	case "refresh":
		// Cached: used by the pane-focus-in hook, which fires constantly.
		return w.ApplyCached()
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
	case "emphasis":
		return w.ToggleEmphasis()
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
		ok, err := w.Show(p)
		if err == nil && !ok {
			// display-message goes nowhere from a plain shell, so say it here too.
			fmt.Fprintln(os.Stderr, wall.FullMsg)
		}
		return err
	}
	return fmt.Errorf("unknown wall command %q", args[0])
}
