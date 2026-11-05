package theme

import "testing"

func TestStatus(t *testing.T) {
	cases := map[string]string{"working": Green, "waiting": Yellow, "error": Red, "stuck": Maroon, "bogus": Idle}
	for st, want := range cases {
		if got := Status(st); got != want {
			t.Errorf("Status(%q) = %s, want %s", st, got, want)
		}
	}
}
