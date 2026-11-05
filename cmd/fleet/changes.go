package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	aifleet "github.com/sturlabragason/fleet"
	"github.com/sturlabragason/fleet/internal/keys"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/whatsnew"
)

// changesTitle is the popup border title.
const changesTitle = " what's new in fleet · q closes "

// changesPopupArgs is the tmux command that opens the what's new popup,
// shared by Alt-g, the status line click and the overview's g. exe is
// already shell-quoted.
func changesPopupArgs(exe string) []string {
	return []string{"display-popup", "-E", "-b", "rounded", "-w", "90%", "-h", "85%",
		"-T", changesTitle, exe + " changes --popup"}
}

// tmuxDefaultStatusClick is tmux's own MouseDown1Status binding (3.8), kept
// for clicks outside the "changes" range.
const tmuxDefaultStatusClick = "switch-client -t ="

// changesStatusRange is the status-line range name around the hint.
const changesStatusRange = "changes"

// tmuxCmdString joins args into one tmux command string, each argument
// quoted for tmux's parser (for if-shell's command arguments).
func tmuxCmdString(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		if !strings.Contains(a, "'") {
			q[i] = "'" + a + "'"
			continue
		}
		q[i] = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`).Replace(a) + `"`
	}
	return strings.Join(q, " ")
}

// changesStatusClick binds a left click on the status line: on the
// "changes" range it opens the popup, anywhere else it does what tmux does.
func changesStatusClick(exe string) []string {
	return []string{"bind-key", "-n", "MouseDown1Status", "if-shell", "-F",
		"#{==:#{mouse_status_range}," + changesStatusRange + "}",
		tmuxCmdString(changesPopupArgs(exe)), tmuxDefaultStatusClick}
}

// clickableChanges wraps the what's new hint in a status-line range so a
// click on it can be told apart. A hint dropped for width gets no range.
func clickableChanges(hint string) string {
	return strings.Replace(hint, keys.ChangesHint,
		"#[range=user|"+changesStatusRange+"]"+keys.ChangesHint+"#[norange]", 1)
}

// newIDs is the set of ids of es that are not in the seen file.
func newIDs(es []whatsnew.Entry) map[string]bool {
	seen, err := whatsnew.LoadSeen()
	if err != nil {
		logx.For("whatsnew").Warn("load seen", "err", err)
	}
	isNew := map[string]bool{}
	for _, e := range es {
		if !seen[e.ID()] {
			isNew[e.ID()] = true
		}
	}
	return isNew
}

// cmdChanges shows what's new in fleet (DESIGN.md §5d).
func cmdChanges(args []string) error {
	fs := flag.NewFlagSet("changes", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	popupMode := fs.Bool("popup", false, "run the popup TUI (used by Alt-g)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("usage: fleet changes [--json]")
	}
	es := whatsnew.List(aifleet.Changelog, whatsnew.Repo())
	isNew := newIDs(es)
	version := whatsnew.Version()
	if *popupMode {
		p := newChangesPopup(version, es, isNew, whatsnew.MarkSeen)
		_, err := tea.NewProgram(p, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
		p.markAll()
		return err
	}
	if *asJSON {
		type row struct {
			Date   string   `json:"date"`
			Text   string   `json:"text"`
			Refs   []string `json:"refs"`
			Commit bool     `json:"commit"`
			New    bool     `json:"new"`
			ID     string   `json:"id"`
		}
		out := struct {
			Version string `json:"version"`
			Entries []row  `json:"entries"`
		}{Version: version, Entries: []row{}}
		for _, e := range es {
			refs := e.Refs
			if refs == nil {
				refs = []string{}
			}
			out.Entries = append(out.Entries, row{e.Date, whatsnew.Plain(e.Text), refs, e.Commit, isNew[e.ID()], e.ID()})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	fmt.Print(changesText(version, es, isNew))
	return nil
}

// changesText is the terminal list: the version line, then
// "date  ✦  text  (refs)" with the date on a day's first line only.
func changesText(version string, es []whatsnew.Entry, isNew map[string]bool) string {
	var b strings.Builder
	b.WriteString(headerLine(version, es, isNew) + "\n")
	if len(es) == 0 {
		b.WriteString("no entries\n")
	}
	day := ""
	for _, e := range es {
		date := strings.Repeat(" ", len(whatsnew.DateLayout))
		if e.Date != day {
			date, day = e.Date, e.Date
		}
		mark := " "
		if isNew[e.ID()] {
			mark = newMark
		}
		line := date + "  " + mark + "  " + whatsnew.Plain(e.Text)
		if len(e.Refs) > 0 {
			line += "  (" + strings.Join(e.Refs, ", ") + ")"
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// newMark flags an unseen entry. Not Peach ●, which means waiting.
const newMark = "✦"

// headerLine is the version line, with "· N new" when something is new.
func headerLine(version string, es []whatsnew.Entry, isNew map[string]bool) string {
	n := 0
	for _, e := range es {
		if isNew[e.ID()] {
			n++
		}
	}
	if n > 0 {
		return fmt.Sprintf("%s · %d new", version, n)
	}
	return version
}
