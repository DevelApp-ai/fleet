package supervisor

import "testing"

func TestChoiceRoundTrip(t *testing.T) {
	home := t.TempDir()
	if got := LoadChoice(home); got != "" {
		t.Fatalf("no file: %q", got)
	}
	if err := SaveChoice(home, "claude-personal"); err != nil {
		t.Fatal(err)
	}
	if got := LoadChoice(home); got != "claude-personal" {
		t.Errorf("got %q", got)
	}
	SaveChoice(home, " copilot ")
	if got := LoadChoice(home); got != "copilot" {
		t.Errorf("overwrite: %q", got)
	}
}
