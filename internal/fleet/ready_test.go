package fleet

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sturlabragason/fleet/internal/prefs"
	"github.com/sturlabragason/fleet/internal/status"
	"github.com/sturlabragason/fleet/internal/tools"
)

// fakePane records what autostart does instead of touching tmux, and never sleeps.
type fakePane struct {
	screens  []string // returned one per Capture call; the last one repeats
	captures int
	capErr   error // when set, every Capture fails
	pasted   []string
	enter    bool
	keys     []string
	slept    time.Duration
}

func (f *fakePane) Capture(pane string, n int) (string, error) {
	f.captures++
	if f.capErr != nil {
		return "", f.capErr
	}
	return f.screens[min(f.captures-1, len(f.screens)-1)], nil
}

func (f *fakePane) Paste(pane, text string, enter bool) error {
	f.pasted = append(f.pasted, text)
	f.enter = enter
	return nil
}

func (f *fakePane) SendKeys(pane string, keys ...string) error {
	f.keys = append(f.keys, keys...)
	return nil
}

func (f *fakePane) sleep(d time.Duration) { f.slept += d }

func testFleet(t *testing.T) *Fleet {
	t.Helper()
	return &Fleet{Patterns: status.Load(t.TempDir())}
}

func TestWaitReadyMatchesAndTimesOut(t *testing.T) {
	f := testFleet(t)
	ready := f.readyFunc(tools.Profile{Patterns: "copilot"})

	// Blank screens until the input box appears on the third capture.
	p := &fakePane{screens: []string{"", "starting up", "> "}}
	waited, ok, err := waitReady(func() (string, error) { return p.Capture("%1", 0) }, ready, p.sleep, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || p.captures != 3 {
		t.Fatalf("ok=%v captures=%d waited=%v", ok, p.captures, waited)
	}
	if waited != 2*readyPoll {
		t.Errorf("waited %v want %v", waited, 2*readyPoll)
	}
	if p.slept != 2*readyPoll+readySettle {
		t.Errorf("settle delay missing: slept %v", p.slept)
	}

	// Never ready -> timeout, bounded by ready_timeout, with no real waiting.
	p = &fakePane{screens: []string{"...nothing here..."}}
	if _, ok, _ := waitReady(func() (string, error) { return p.Capture("%1", 0) }, ready, p.sleep, time.Second); ok {
		t.Fatal("expected timeout")
	}
	if p.slept < time.Second {
		t.Errorf("gave up early: slept %v", p.slept)
	}
}

func TestReadyFuncUsesProfileRegex(t *testing.T) {
	f := testFleet(t)
	ready := f.readyFunc(tools.Profile{Patterns: "copilot", Ready: `READY NOW`})
	if ready("> ") {
		t.Error("regex profile should ignore the status patterns")
	}
	if !ready("all set: READY NOW") {
		t.Error("regex did not match")
	}
}

func TestAutostartPastesOnlyWhenNotEmbedded(t *testing.T) {
	f := testFleet(t)
	prof := tools.Profile{Name: "fake", Patterns: "fake"}
	pr := prefs.Neutral()

	// Prompt not carried in argv: wait for ready, then paste and press Enter.
	p := &fakePane{screens: []string{"", "> "}}
	f.autostart(p, p.sleep, "%1", prof, pr, "do the thing", false)
	if !reflect.DeepEqual(p.pasted, []string{"do the thing"}) || !p.enter {
		t.Errorf("pasted %v enter=%v", p.pasted, p.enter)
	}

	// Prompt already in argv: the pane is left completely alone.
	p = &fakePane{screens: []string{"> "}}
	f.autostart(p, p.sleep, "%1", prof, pr, "do the thing", true)
	if len(p.pasted) != 0 || p.captures != 0 {
		t.Errorf("embedded prompt touched the pane: pasted=%v captures=%d", p.pasted, p.captures)
	}

	// No prompt at all: nothing to do either.
	p = &fakePane{screens: []string{"> "}}
	f.autostart(p, p.sleep, "%1", prof, pr, "", false)
	if len(p.pasted) != 0 || p.captures != 0 {
		t.Errorf("empty prompt touched the pane: %+v", p)
	}

	// auto_submit = false pastes without Enter.
	noSubmit := pr
	noSubmit.AutoSubmit = false
	p = &fakePane{screens: []string{"> "}}
	f.autostart(p, p.sleep, "%1", prof, noSubmit, "draft", false)
	if p.enter {
		t.Error("auto_submit=false still pressed Enter")
	}
}

func TestAutostartModeKeys(t *testing.T) {
	f := testFleet(t)
	pr := prefs.Neutral()
	pr.Mode = "autopilot"

	// mode_keys are sent after ready and before the prompt.
	prof := tools.Profile{Name: "k", Patterns: "fake", ModeKeys: map[string][]string{"autopilot": {"C-a", "p"}}}
	p := &fakePane{screens: []string{"", "> "}}
	f.autostart(p, p.sleep, "%1", prof, pr, "go", false)
	if !reflect.DeepEqual(p.keys, []string{"C-a", "p"}) || len(p.pasted) != 1 {
		t.Errorf("keys=%v pasted=%v", p.keys, p.pasted)
	}

	// When mode_args already selected the mode, the keys are not sent.
	prof.ModeArgs = map[string][]string{"autopilot": {"--allow-all"}}
	p = &fakePane{screens: []string{"> "}}
	f.autostart(p, p.sleep, "%1", prof, pr, "", true)
	if len(p.keys) != 0 || p.captures != 0 {
		t.Errorf("mode_keys sent despite mode_args: %v", p.keys)
	}
}

// Timeout still pastes: best effort beats silently losing the prompt.
func TestAutostartPastesAfterTimeout(t *testing.T) {
	f := testFleet(t)
	p := &fakePane{screens: []string{"nothing readable"}}
	pr := prefs.Neutral()
	pr.ReadyTimeout = 400 * time.Millisecond
	f.autostart(p, p.sleep, "%1", tools.Profile{Name: "fake", Patterns: "fake"}, pr, "anyway", false)
	if !reflect.DeepEqual(p.pasted, []string{"anyway"}) {
		t.Errorf("pasted %v", p.pasted)
	}
}

// A pane whose command died instantly must not be polled for the whole
// ready_timeout and then pasted into.
func TestWaitReadyGivesUpOnCaptureErrors(t *testing.T) {
	f := testFleet(t)
	p := &fakePane{screens: []string{""}, capErr: errors.New("no such pane")}
	waited, ok, err := waitReady(func() (string, error) { return p.Capture("%1", 0) },
		f.readyFunc(tools.Profile{Patterns: "fake"}), p.sleep, time.Minute)
	if ok || err == nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if p.captures != maxCaptureErrs {
		t.Errorf("captured %d times, want %d", p.captures, maxCaptureErrs)
	}
	if waited > time.Second {
		t.Errorf("waited %v; should have bailed out early", waited)
	}

	// ...and autostart then leaves the dead pane alone.
	p = &fakePane{screens: []string{""}, capErr: errors.New("no such pane")}
	f.autostart(p, p.sleep, "%1", tools.Profile{Name: "fake", Patterns: "fake"}, prefs.Neutral(), "gone", false)
	if len(p.pasted) != 0 {
		t.Errorf("pasted into a dead pane: %v", p.pasted)
	}
}

// Readiness must mean "accepting input". A bare spinner glyph classifies as
// working and can render before the TUI enters raw mode — the exact
// pre-condition of the lost-prompt bug — so it is not ready.
func TestReadyFuncRejectsWorkingAndError(t *testing.T) {
	f := testFleet(t)
	ready := f.readyFunc(tools.Profile{Patterns: "copilot"})
	for _, s := range []string{"⠹", "⠙ thinking", "esc to interrupt", "error: boom", "total gibberish"} {
		if ready(s) {
			t.Errorf("%q should not count as ready", s)
		}
	}
	for _, s := range []string{"> ", "Do you want to continue?"} {
		if !ready(s) {
			t.Errorf("%q should count as ready", s)
		}
	}
}
