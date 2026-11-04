package fake

import "testing"

func TestNext(t *testing.T) {
	cases := []struct {
		s    State
		ev   Event
		r    float64
		want State
	}{
		{Working, Timer, 0.1, Waiting},
		{Working, Timer, 0.6, Error},
		{Working, Timer, 0.9, Idle},
		{Working, Input, 0.1, Working},
		{Waiting, Yes, 0, Working},
		{Waiting, No, 0, Error},
		{Waiting, Input, 0, Waiting},
		{Error, Timer, 0, Idle},
		{Idle, Timer, 0, Working},
		{Idle, Input, 0, Working},
	}
	for _, c := range cases {
		if got := Next(c.s, c.ev, c.r); got != c.want {
			t.Errorf("Next(%s,%s,%v)=%s want %s", c.s, c.ev, c.r, got, c.want)
		}
	}
}

func TestCycleVisitsAllStates(t *testing.T) {
	seen := map[State]bool{}
	s := Working
	for _, step := range []struct {
		ev Event
		r  float64
	}{{Timer, 0.1}, {Yes, 0}, {Timer, 0.6}, {Timer, 0}, {Timer, 0}} {
		seen[s] = true
		s = Next(s, step.ev, step.r)
	}
	for _, st := range []State{Working, Waiting, Error, Idle} {
		if !seen[st] {
			t.Errorf("never visited %s", st)
		}
	}
}
