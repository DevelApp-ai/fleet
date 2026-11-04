// Package fake implements a scripted fake agent for development (ADR-8).
package fake

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"strings"
	"time"
)

// State is a fake agent state.
type State string

const (
	Working State = "working"
	Waiting State = "waiting"
	Error   State = "error"
	Idle    State = "idle"
)

// Event drives transitions.
type Event string

const (
	Timer Event = "timer"
	Yes   Event = "yes"
	No    Event = "no"
	Input Event = "input"
)

// Next is the pure transition function. r in [0,1) picks branches after work.
func Next(s State, ev Event, r float64) State {
	switch s {
	case Working:
		if ev != Timer {
			return Working
		}
		switch {
		case r < 0.5:
			return Waiting
		case r < 0.7:
			return Error
		default:
			return Idle
		}
	case Waiting:
		switch ev {
		case Yes, Timer:
			return Working
		case No:
			return Error
		}
		return Waiting
	case Error:
		if ev == Input {
			return Working
		}
		return Idle
	case Idle:
		return Working
	}
	return Idle
}

// Durations for each state (min, spread). Waiting times out after a long while.
var durations = map[State][2]time.Duration{
	Working: {6 * time.Second, 8 * time.Second},
	Waiting: {90 * time.Second, 30 * time.Second},
	Error:   {6 * time.Second, 4 * time.Second},
	Idle:    {8 * time.Second, 7 * time.Second},
}

var tasks = []string{"refactor auth middleware", "add tests for board store", "fix flaky e2e", "update deploy config", "add search endpoint", "bump dependencies", "write migration", "tidy README"}
var spin = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Run runs the agent loop until in closes.
func Run(name string, in io.Reader, out io.Writer) {
	h := fnv.New64a()
	h.Write([]byte(name))
	rng := rand.New(rand.NewSource(int64(h.Sum64()) ^ time.Now().UnixNano()))
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(in)
		for sc.Scan() {
			lines <- strings.TrimSpace(sc.Text())
		}
		close(lines)
	}()
	fmt.Fprintf(out, "fakeagent %s ready (tool: fake)\n", name)
	state, task, step := Working, tasks[rng.Intn(len(tasks))], 0
	enter := func(s State) <-chan time.Time {
		d := durations[s]
		switch s {
		case Working:
			fmt.Fprintf(out, "\n● Task: %s\n", task)
		case Waiting:
			fmt.Fprintf(out, "\nI want to run `make test` in ./%s\nApprove? [y/n] ", name)
		case Error:
			fmt.Fprintf(out, "\n✖ Error: build failed: exit status 1 (%s)\n", task)
		case Idle:
			fmt.Fprintf(out, "\n✔ Done: %s\n> ", task)
		}
		return time.After(d[0] + time.Duration(rng.Int63n(int64(d[1]))))
	}
	timer := enter(state)
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		ev := Event("")
		var line string
		select {
		case <-tick.C:
			if state == Working {
				step++
				fmt.Fprintf(out, "%s Thinking… step %d of %s (esc to interrupt)\n", spin[step%len(spin)], step, task)
			}
			continue
		case <-timer:
			ev = Timer
		case l, ok := <-lines:
			if !ok {
				return
			}
			line = l
			switch strings.ToLower(l) {
			case "y", "yes":
				ev = Yes
			case "n", "no":
				ev = No
			default:
				ev = Input
			}
		}
		next := Next(state, ev, rng.Float64())
		if state == Waiting && ev == Input {
			fmt.Fprintf(out, "Please answer y or n.\nApprove? [y/n] ")
			continue
		}
		if ev == Input && line != "" && next == Working {
			task = line
		}
		if next == Working && state != Working && ev == Timer && state != Waiting {
			task = tasks[rng.Intn(len(tasks))]
		}
		if state == Waiting {
			fmt.Fprintf(out, "%s\n", map[Event]string{Yes: "approved", No: "rejected", Timer: "no answer, continuing"}[ev])
		}
		if next == state && ev == Input {
			fmt.Fprintf(out, "(queued: %s)\n", line)
			continue
		}
		state, step = next, 0
		timer = enter(state)
	}
}
