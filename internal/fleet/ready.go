package fleet

import (
	"regexp"
	"time"

	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/prefs"
	"github.com/sturlabragason/fleet/internal/status"
	"github.com/sturlabragason/fleet/internal/tools"
)

// readyPoll is how often the pane is captured while waiting for the TUI;
// readySettle is a short pause after the first match so the input box has
// finished drawing (ADR-13).
var (
	readyPoll   = 200 * time.Millisecond
	readySettle = 300 * time.Millisecond
)

// maxCaptureErrs is how many consecutive capture failures mean the pane is
// gone (the command died instantly). Without this, waitReady would poll a
// corpse for the whole ready_timeout and then paste into it.
const maxCaptureErrs = 3

// paneIO is the slice of tmux that autostart needs; *tmux.Client satisfies it
// and tests supply a fake, so no test waits on real time or a real agent.
type paneIO interface {
	Capture(pane string, n int) (string, error)
	Paste(pane, text string, enter bool) error
	SendKeys(pane string, keys ...string) error
}

// waitReady polls capture until ready reports the pane is accepting input.
// It never uses the wall clock: elapsed time is the sum of the sleeps it asks
// for, so a fake sleep makes it instant. ok=false means it timed out; a
// non-nil err means capture failed maxCaptureErrs times in a row, i.e. the
// pane is dead and there is nothing to wait for.
func waitReady(capture func() (string, error), ready func(string) bool, sleep func(time.Duration), timeout time.Duration) (waited time.Duration, ok bool, err error) {
	errs := 0
	for waited = 0; waited < timeout; waited += readyPoll {
		s, cerr := capture()
		switch {
		case cerr != nil:
			if errs++; errs >= maxCaptureErrs {
				return waited, false, cerr
			}
		case ready(s):
			sleep(readySettle)
			return waited, true, nil
		default:
			errs = 0
		}
		sleep(readyPoll)
	}
	return waited, false, nil
}

// readyFunc returns the "TUI is accepting input" test for a profile: its own
// `ready` regex, or else "the status patterns say idle or waiting" — those two
// describe a live input box. Working and Error are deliberately not enough: a
// bare spinner glyph matches the copilot/agy/vibe `working` patterns and can
// render during startup, before the TUI is in raw mode, which is exactly the
// pre-condition of the lost-prompt bug in ADR-13.
func (f *Fleet) readyFunc(p tools.Profile) func(string) bool {
	if p.Ready != "" {
		if re, err := regexp.Compile(p.Ready); err == nil {
			return re.MatchString
		}
		logx.For("fleet").Warn("bad ready regex", "profile", p.Name, "ready", p.Ready)
	}
	set := f.Patterns[p.Patterns]
	return func(s string) bool {
		st, ok := set.Detect(s)
		return ok && (st == status.Idle || st == status.Waiting)
	}
}

// autostart finishes a spawn: wait for the pane to be ready, send any
// mode_keys, and paste the prompt when the profile could not carry it in argv.
// It returns without touching the pane when there is nothing to do.
func (f *Fleet) autostart(io paneIO, sleep func(time.Duration), pane string, p tools.Profile, pr prefs.Spawn, prompt string, embedded bool) {
	log := logx.For("fleet")
	var keys []string
	if len(p.ModeArgs[pr.Mode]) == 0 {
		keys = p.ModeKeys[pr.Mode]
	}
	paste := prompt != "" && !embedded
	if !paste && len(keys) == 0 {
		return
	}
	waited, ok, err := waitReady(func() (string, error) { return io.Capture(pane, 0) }, f.readyFunc(p), sleep, pr.ReadyTimeout)
	switch {
	case err != nil:
		log.Warn("pane capture keeps failing; giving up on the first prompt",
			"pane", pane, "profile", p.Name, "waited", waited, "err", err)
		return
	case !ok:
		log.Warn("pane never looked ready; continuing anyway", "pane", pane, "profile", p.Name, "ready_timeout", pr.ReadyTimeout)
	default:
		log.Info("pane ready", "pane", pane, "profile", p.Name, "waited", waited)
	}
	if len(keys) > 0 {
		if err := io.SendKeys(pane, keys...); err != nil {
			log.Warn("mode keys failed", "pane", pane, "mode", pr.Mode, "err", err)
		}
	}
	if paste {
		if err := io.Paste(pane, prompt, pr.AutoSubmit); err != nil {
			log.Warn("prompt paste failed", "pane", pane, "err", err)
		}
	}
}

// PastePrompt waits up to timeout for pane to accept input, then pastes
// prompt and presses Enter. Used for the admiral's boot prompt when its
// profile has no prompt_args (DESIGN §6.10).
func (f *Fleet) PastePrompt(pane string, p tools.Profile, prompt string, timeout time.Duration) {
	pr := prefs.Neutral()
	pr.ReadyTimeout = timeout
	f.autostart(f.T, time.Sleep, pane, p, pr, prompt, false)
}
