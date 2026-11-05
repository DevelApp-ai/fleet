package fleet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sturlabragason/fleet/internal/board"
	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/mail"
	"github.com/sturlabragason/fleet/internal/status"
	"github.com/sturlabragason/fleet/internal/supervisor"
	"github.com/sturlabragason/fleet/internal/tools"
)

// Report records an agent's report to the admiral (DESIGN §6.7): a
// "report" message in the admiral's mailbox and the latest report on the
// board. done, blocked and question wake an idle admiral. On done with
// auto_close_done, note is "auto-closing" or "not auto-closed: <reason>".
func (f *Fleet) Report(me, state, summary string) (note string, err error) {
	if err := f.onBoard(me); err != nil {
		return "", err
	}
	m, err := f.Mail().Report(me, state, summary)
	if err != nil {
		return "", err
	}
	sum := mail.FoldSummary(strings.TrimPrefix(m.Text, state+": "))
	err = f.Store.Update(func(b *board.Board) error {
		if a, ok := b.Agents[me]; ok {
			a.Report = &board.Report{State: state, Summary: sum, At: time.Now()}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if state == "done" {
		note = f.autoClose(me)
	}
	if state == "done" || state == "blocked" || state == "question" {
		line := ReportLine(me, state, sum)
		if note != "" {
			line += " (" + note + ")"
		}
		f.wakeSupervisor(me, line)
	}
	return note, nil
}

// wakeHeadline caps the summary in the line typed into the admiral pane,
// so the line fits on screen; the full text stays in the mailbox.
const wakeHeadline = 100

// ReportLine is the line typed into an idle admiral pane: a short headline
// and where the full text is (fleet_reports keeps it after the agent closes).
func ReportLine(from, state, summary string) string {
	if r := []rune(summary); len(r) > wakeHeadline {
		summary = string(r[:wakeHeadline-1]) + "…"
	}
	return fmt.Sprintf("[fleet] report from %s: %s: %s. Full text: call fleet_reports.", from, state, summary)
}

// wakeSupervisor queues line for the admiral pane and delivers it now if
// the pane looks idle. A busy or missing admiral keeps it queued until the
// fleet UI's poll sees the pane idle (DeliverWake), so a report that lands
// mid-turn is not lost. One line per unread batch.
func (f *Fleet) wakeSupervisor(from, line string) {
	p := f.wakePath()
	os.MkdirAll(filepath.Dir(p), 0o755)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(line), 0o644); err == nil {
		os.Rename(tmp, p)
	}
	f.DeliverWake()
}

func (f *Fleet) wakePath() string {
	return filepath.Join(f.Home, "mail", mail.Supervisor, "wake-pending")
}

// DeliverWake types the queued report line into the admiral pane when it
// looks idle. The line is dropped once the admiral has read its reports or
// was already woken for this unread batch.
func (f *Fleet) DeliverWake() {
	p := f.wakePath()
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	pane, idle := f.supPane()
	if pane == "" || !idle {
		return
	}
	_, ok, err := f.Mail().ClaimNudge(mail.Supervisor)
	if err != nil {
		return
	}
	os.Remove(p)
	if !ok {
		return
	}
	log := logx.For("mail")
	if err := f.paste(pane, string(b)); err != nil {
		log.Warn("admiral wake failed", "err", err)
		return
	}
	log.Info("woke admiral", "line", string(b))
}

// supPane finds the @role=sup pane in fleet:main and classifies it with the
// admiral profile's status patterns (default copilot).
func (f *Fleet) supPane() (string, bool) {
	if f.SupPane != nil {
		return f.SupPane()
	}
	pane, err := supervisor.FindPane(f.T, Session+":main")
	if err != nil {
		return "", false
	}
	prof := supervisor.Current(f.Home, f.T, Session+":main").Profile
	if prof == "" {
		prof = "copilot"
	}
	set := prof
	if p, ok := f.Profiles[prof]; ok && p.Patterns != "" {
		set = p.Patterns
	}
	text, err := f.T.Capture(pane, 0)
	if err != nil {
		return pane, false
	}
	st, ok := f.Patterns[set].Detect(text)
	return pane, ok && st == status.Idle
}

// autoClose closes a done agent when its profile opts in and it left no
// work behind (AgentBlocker). It returns the note for the admiral ("" = off).
func (f *Fleet) autoClose(name string) string {
	a, ok := f.Store.Load().Agents[name]
	if !ok || !f.Prefs.For(a.Profile).AutoCloseDone {
		return ""
	}
	if reason := AgentBlocker(a); reason != "" {
		return "not auto-closed: " + reason
	}
	if err := f.closeLater(name); err != nil {
		logx.For("fleet").Warn("auto-close failed", "agent", name, "err", err)
		return "not auto-closed: " + err.Error()
	}
	logx.For("fleet").Info("auto-closing", "agent", name)
	return "auto-closing"
}

// closeLater closes name through tmux after 2 s, so the reporting MCP call
// (which runs inside the agent) returns first.
func (f *Fleet) closeLater(name string) error {
	if f.CloseLater != nil {
		return f.CloseLater(name)
	}
	_, err := f.T.Run("run-shell", "-b", "sleep 2; "+tools.Quote(f.Exe)+" close "+tools.Quote(name)+" >/dev/null 2>&1")
	return err
}

// CloseBlocker returns why dir can't be auto-closed ("" = it can): it must
// be a git work tree with a clean status whose HEAD is merged into the
// local main (or master when there is no main).
func CloseBlocker(dir string) string {
	g := GitInfo(dir)
	switch {
	case !g.Repo:
		return "not a git work tree"
	case g.Dirty != 0:
		return "uncommitted changes"
	case g.Base == "":
		return "no main or master branch"
	case !g.Merged:
		return "not merged into " + g.Base
	}
	return ""
}
