package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/supervisor"
	"github.com/sturlabragason/fleet/internal/tools"
)

// confirmResetArgs is the tmux command behind M-R and the overview's R.
func confirmResetArgs(exe string) []string {
	// Output is discarded: run-shell would show it in view mode over the pane;
	// the result reaches the user via display-message.
	// tmux parses the run-shell argument again: double-quote it for tmux and
	// shell-quote the exe for sh.
	sh := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`).Replace(tools.Quote(exe) + " supervisor reset >/dev/null 2>&1")
	return []string{"confirm-before", "-p", "reset supervisor? (y/n)", `run-shell -b "` + sh + `"`}
}

// display shows msg literally on the fleet client (no format/strftime expansion).
func display(f *fleet.Fleet, msg string) { f.T.Run("display-message", "-l", msg) }

func mainTarget() string { return fleet.Session + ":main" }

func cmdSupervisor(f *fleet.Fleet, args []string) error {
	if len(args) > 0 && args[0] == "reset" {
		fs := flag.NewFlagSet("supervisor reset", flag.ContinueOnError)
		handoff := fs.Bool("handoff", false, "called after fleet_handoff wrote a handoff (no stub)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		gen, err := resetSupervisor(f, *handoff)
		if err != nil {
			display(f, err.Error())
			return err
		}
		msg := fmt.Sprintf("supervisor reset · gen %d", gen)
		display(f, msg)
		fmt.Println(msg)
		return nil
	}
	asJSON := len(args) > 0 && args[0] == "--json"
	if len(args) > 0 && !asJSON {
		return fmt.Errorf("usage: fleet supervisor [--json] | fleet supervisor reset [--handoff]")
	}
	s := supervisor.Current(f.Home, f.T, mainTarget())
	used, max, ok := supervisor.Fill(supervisor.CopilotHome(), s.SessionID)
	if asJSON {
		out := map[string]any{"session_id": s.SessionID, "profile": s.Profile, "pane_id": s.PaneID,
			"generation": s.Generation, "previous": s.Previous, "ctx_pct": nil}
		if !s.Started.IsZero() {
			out["started"] = s.Started
			out["uptime_s"] = int(time.Since(s.Started).Seconds())
		}
		if ok {
			out["ctx_pct"], out["ctx_used"], out["ctx_max"] = supervisor.Pct(used, max), used, max
		}
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		return e.Encode(out)
	}
	fmt.Println(supLine(s, used, max, ok, time.Now()))
	return nil
}

// supLine is `sup 3f2a91c · ctx 34% (68k/200k) · gen 2 · up 2h13m`.
func supLine(s supervisor.Session, used, max int, ok bool, now time.Time) string {
	id := s.SessionID
	if len(id) > 7 {
		id = id[:7]
	}
	if id == "" {
		id = "?"
	}
	ctx := "ctx ?"
	if ok {
		ctx = fmt.Sprintf("ctx %d%% (%s/%s)", supervisor.Pct(used, max), kTok(used), kTok(max))
	}
	parts := []string{"sup " + id, ctx}
	if s.Generation > 0 {
		parts = append(parts, fmt.Sprintf("gen %d", s.Generation))
	}
	if !s.Started.IsZero() {
		d := now.Sub(s.Started)
		up := fmt.Sprintf("%dm", int(d.Minutes()))
		if h := int(d.Hours()); h > 0 {
			up = fmt.Sprintf("%dh%dm", h, int(d.Minutes())%60)
		}
		parts = append(parts, "up "+up)
	}
	return strings.Join(parts, " · ")
}

func kTok(n int) string { return fmt.Sprintf("%dk", (n+500)/1000) }

// resetSupervisor replaces the supervisor with a fresh session (DESIGN.md §6.5
// Reset steps 1–4) and returns the new generation.
func resetSupervisor(f *fleet.Fleet, handoff bool) (int, error) {
	log := logx.For("supervisor")
	unlock, err := supervisor.Lock(f.Home)
	if err != nil {
		return 0, err
	}
	defer unlock()
	pane, err := supervisor.FindPane(f.T, mainTarget())
	if err != nil {
		return 0, err
	}
	old := supervisor.Current(f.Home, f.T, mainTarget())
	_, hTime, hOK := supervisor.ReadHandoff(f.Home)
	stale := !hOK || (!old.Started.IsZero() && hTime.Before(old.Started)) || (old.Started.IsZero() && !handoff)
	if stale {
		prev := old.SessionID
		if prev == "" {
			prev = "unknown"
		}
		stub := fmt.Sprintf("Manual reset at %s; previous supervisor wrote no handoff. Previous session: %s.",
			time.Now().Format("2006-01-02 15:04"), prev)
		if err := supervisor.WriteHandoff(f.Home, stub); err != nil {
			return 0, err
		}
	}
	prof := old.Profile
	if _, ok := f.Profiles[prof]; !ok {
		prof = "" // unknown or unset: FLEET_SUPERVISOR_PROFILE / default
	}
	sl, err := supervisorCmd(f, prof, supervisor.BootPrompt)
	if err != nil {
		return 0, err
	}
	a := []string{"respawn-pane", "-k", "-c", sl.Dir}
	for _, e := range sl.Env {
		a = append(a, "-e", e)
	}
	if _, err := f.T.Run(append(a, "-t", pane, sl.Cmd)...); err != nil {
		return 0, err
	}
	f.T.SetPaneOption(pane, "@role", "sup")
	f.T.SetPaneOption(pane, "remain-on-exit", "on")
	for _, o := range []string{"@fleet_ctx", "@fleet_ctx_color"} {
		f.T.Run("set-option", "-p", "-u", "-t", pane, o)
	}
	gen := old.Generation + 1
	if gen < 2 {
		gen = 2 // a supervisor existed before this one
	}
	ns := supervisor.Session{SessionID: sl.ID, Profile: sl.Profile, PaneID: pane, Started: time.Now(), Generation: gen, Previous: old.SessionID}
	if err := supervisor.Save(f.Home, ns); err != nil {
		return 0, err
	}
	log.Info("reset", "pane", pane, "generation", gen, "session_id", sl.ID, "previous", old.SessionID, "stub_handoff", stale, "handoff", handoff)
	return gen, nil
}

func cmdNote(f *fleet.Fleet, args []string) error {
	fs := flag.NewFlagSet("note", flag.ContinueOnError)
	agent := fs.String("agent", "", "agent the note is about")
	if err := fs.Parse(args); err != nil {
		return err
	}
	text := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(text) == "" {
		return errors.New(`usage: fleet note [--agent NAME] "text"`)
	}
	return supervisor.AppendNote(f.Home, *agent, text)
}
