package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"

	"github.com/sturlabragason/fleet/internal/fleet"
	"github.com/sturlabragason/fleet/internal/team"
	"github.com/sturlabragason/fleet/internal/wall"
)

const teamUsage = `usage: fleet team ls [--json] | create <team> [colour] | delete <team> | add <team> <agent>... |
       rm <team> <agent>... | color <team> <colour> | send <team> "text" | show <team> | hide <team> |
       close <team> [--yes]`

func cmdTeam(f *fleet.Fleet, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s", teamUsage)
	}
	sub, rest := args[0], args[1:]
	need := func(n int) error {
		if len(rest) < n {
			return fmt.Errorf("%s", teamUsage)
		}
		return nil
	}
	report := func(verb string, rs []fleet.Result, err error) error {
		if err != nil {
			return err
		}
		fmt.Println(fleet.FormatResults(verb, rs))
		return nil
	}
	switch sub {
	case "ls":
		return teamLs(f, os.Stdout, len(rest) > 0 && rest[0] == "--json")
	case "create":
		if err := need(1); err != nil {
			return err
		}
		color := ""
		if len(rest) > 1 {
			color = rest[1]
		}
		return f.Teams.Create(rest[0], color)
	case "delete":
		if err := need(1); err != nil {
			return err
		}
		return f.Teams.Delete(rest[0])
	case "add":
		if err := need(2); err != nil {
			return err
		}
		moved, err := f.Teams.Add(rest[0], rest[1:]...)
		if err != nil {
			return err
		}
		fmt.Printf("added %s to %s%s\n", strings.Join(rest[1:], ", "), rest[0], fleet.MovedNote(moved))
		return nil
	case "rm":
		if err := need(2); err != nil {
			return err
		}
		return f.Teams.Remove(rest[0], rest[1:]...)
	case "color", "colour":
		if err := need(2); err != nil {
			return err
		}
		return f.Teams.SetColor(rest[0], rest[1])
	case "send":
		if err := need(2); err != nil {
			return err
		}
		rs, err := f.TeamSend(rest[0], strings.Join(rest[1:], " "), true) // the user typed it
		return report("sent", rs, err)
	case "show", "hide":
		if err := need(1); err != nil {
			return err
		}
		w := wall.New(f.T, fleet.Session)
		if sub == "show" {
			rs, err := f.TeamShow(w, rest[0])
			return report("on wall", rs, err)
		}
		rs, err := f.TeamHide(w, rest[0])
		return report("off wall", rs, err)
	case "close":
		fs := flag.NewFlagSet("team close", flag.ContinueOnError)
		yes := fs.Bool("yes", false, "don't ask")
		pos, err := parseInterleaved(fs, rest)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return fmt.Errorf("usage: fleet team close <team> [--yes]")
		}
		ms, err := f.Teams.Members(pos[0])
		if err != nil {
			return err
		}
		if !*yes && !confirmClose(os.Stdin, os.Stdout, pos[0], ms) {
			return fmt.Errorf("aborted")
		}
		rs, err := f.TeamClose(pos[0])
		return report("closed", rs, err)
	}
	return fmt.Errorf("unknown team command %q\n%s", sub, teamUsage)
}

// confirmClose lists the members and asks y/N; only y or yes confirms.
func confirmClose(in io.Reader, out io.Writer, name string, members []string) bool {
	fmt.Fprintf(out, "members: %s\nclose %d agents in %s? [y/N] ", strings.Join(members, ", "), len(members), name)
	line, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

func teamLs(f *fleet.Fleet, out io.Writer, asJSON bool) error {
	rows := f.TeamList()
	if asJSON {
		e := json.NewEncoder(out)
		e.SetIndent("", "  ")
		return e.Encode(rows)
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "no teams")
		return nil
	}
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TEAM\tCOLOUR\tMEMBERS")
	colors := []string{""}
	for _, r := range rows {
		var ms []string
		for _, m := range r.Members {
			if m.Running {
				ms = append(ms, m.Name)
			} else {
				ms = append(ms, m.Name+" (not running)")
			}
		}
		fmt.Fprintf(w, "%s %s\t%s\t%s\n", teamMark, r.Name, team.ColorName(r.Color), strings.Join(ms, ", "))
		colors = append(colors, r.Color)
	}
	w.Flush()
	_, err := io.WriteString(out, colourMarks(buf.String(), 0, colors))
	return err
}

const teamMark = "◆"

// colourMarks colours the team mark at byte offset off of each line in the
// matching colour ("" leaves the line alone). tabwriter counts escape codes as
// width, so the table is laid out plain first and coloured afterwards.
func colourMarks(table string, off int, colors []string) string {
	lines := strings.Split(table, "\n")
	for i, c := range colors {
		if c == "" || i >= len(lines) || !strings.HasPrefix(lines[i][min(off, len(lines[i])):], teamMark) {
			continue
		}
		l := lines[i]
		lines[i] = l[:off] + lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render(teamMark) + l[off+len(teamMark):]
	}
	return strings.Join(lines, "\n")
}
