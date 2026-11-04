package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sturlabragason/fleet/internal/fake"
	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/mcp"
	"github.com/sturlabragason/fleet/internal/tools"
	"github.com/sturlabragason/fleet/internal/wall"
)

const usage = `usage:
  fleet up                                  start/attach the fleet (supervisor + overview + wall)
  fleet ui                                  run the overview TUI in this terminal
  fleet spawn <name> <profile> [dir] ["prompt"]
  fleet spawn --fake <name> [dir]           shorthand for profile "fake"
  fleet ls [--json]
  fleet send <name> "text"
  fleet peek <name> [--raw] [-n 200]
  fleet close <name>                        stop an agent and remove it from the board
  fleet mcp                                 stdio MCP server for the supervisor
  fleet supervisor [--json]                 supervisor session, context fill, generation, uptime
  fleet supervisor reset [--handoff]        replace the supervisor with a fresh session (M-R)
  fleet note [--agent NAME] "text"          append a note to ~/.fleet/decisions.log
  fleet profiles                            list tool profiles (~/.fleet/tools.toml)
  fleet wall <layout|show NAME>             wall control (used by tmux key bindings)
  fleet keys [--markdown]                   list every hotkey (Alt-? in tmux, ? in the overview)`

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s", usage)
	}
	cmd, rest := args[0], args[1:]
	if cmd == "fakeagent" {
		name := "fake"
		if len(rest) > 0 {
			name = rest[0]
		}
		fake.Run(name, os.Stdin, os.Stdout)
		return nil
	}
	if cmd == "keys" { // needs no fleet state
		return cmdKeys(rest)
	}
	f, err := fleet.Open()
	if err != nil {
		return err
	}
	switch cmd {
	case "spawn":
		return cmdSpawn(f, rest)
	case "ls":
		return cmdLs(f, rest)
	case "send":
		if len(rest) < 2 {
			return fmt.Errorf("usage: fleet send <name> \"text\"")
		}
		return f.Send(rest[0], strings.Join(rest[1:], " "))
	case "peek":
		return cmdPeek(f, rest)
	case "close", "kill": // "kill" kept as a hidden alias
		if len(rest) != 1 {
			return fmt.Errorf("usage: fleet close <name>")
		}
		return f.Close(rest[0])
	case "mcp":
		return mcp.New(f, wall.New(f.T, fleet.Session)).Serve(os.Stdin, os.Stdout)
	case "profiles":
		for _, n := range tools.Names(f.Profiles) {
			p := f.Profiles[n]
			fmt.Printf("%-16s %s  (patterns=%s)\n", n, strings.Join(p.Argv(tools.Vars{Fleet: "fleet", Name: "<name>", ID: "<id>"}, false), " "), p.Patterns)
		}
		return nil
	case "supervisor":
		return cmdSupervisor(f, rest)
	case "note":
		return cmdNote(f, rest)
	case "ui":
		return cmdUI(f)
	case "up":
		return cmdUp(f, rest)
	case "wall":
		return cmdWall(f, rest)
	case "help", "-h", "--help":
		fmt.Println(usage)
		return nil
	}
	return fmt.Errorf("unknown command %q\n%s", cmd, usage)
}

func cmdSpawn(f *fleet.Fleet, args []string) error {
	fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
	isFake := fs.Bool("fake", false, "use the fake agent profile")
	if err := fs.Parse(args); err != nil {
		return err
	}
	a := fs.Args()
	var name, profile, dir, prompt string
	if *isFake {
		if len(a) < 1 {
			return fmt.Errorf("usage: fleet spawn --fake <name> [dir]")
		}
		name, profile = a[0], "fake"
		if len(a) > 1 {
			dir = a[1]
		}
	} else {
		if len(a) < 2 {
			return fmt.Errorf("usage: fleet spawn <name> <profile> [dir] [\"prompt\"]")
		}
		name, profile = a[0], a[1]
		if len(a) > 2 {
			dir = a[2]
		}
		if len(a) > 3 {
			prompt = strings.Join(a[3:], " ")
		}
	}
	ag, err := f.Spawn(name, profile, dir, prompt)
	if err != nil {
		return err
	}
	fmt.Printf("spawned %s (%s) in %s pane %s\n", ag.Name, ag.Profile, ag.Dir, ag.PaneID)
	return nil
}

func cmdLs(f *fleet.Fleet, args []string) error {
	as, err := f.Refresh()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "--json" {
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		return e.Encode(as)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPROFILE\tSTATUS\tFOR\tPANE\tLAST LINE")
	for _, a := range as {
		ll := a.LastLine
		if len(ll) > 60 {
			ll = ll[:60]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", a.Name, a.Profile, a.Status, time.Since(a.StatusSince).Round(time.Second), a.PaneID, ll)
	}
	return w.Flush()
}

func cmdPeek(f *fleet.Fleet, args []string) error {
	fs := flag.NewFlagSet("peek", flag.ContinueOnError)
	fs.Bool("raw", true, "raw pane text (summaries come in slice 7)")
	n := fs.Int("n", 0, "lines of history")
	var name string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		return fmt.Errorf("usage: fleet peek <name> [--raw] [-n N]")
	}
	out, err := f.Peek(name, *n)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}
