package main

import (
	"errors"
	"os"
	"testing"
)

// Errors exit 2, never 0 or 1, so a failed check can't read as all-clear or
// as an overlap.
func TestTouchingErrorsExit2(t *testing.T) {
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	notRepo := t.TempDir()
	for name, args := range map[string][]string{
		"not a repo": {"-C", notRepo, "a.go"},
		"bad flag":   {"--nope"},
	} {
		var code exitCode
		if err := cmdTouching(args); !errors.As(err, &code) || code != 2 {
			t.Errorf("%s: err %v, want exit 2", name, err)
		}
	}
}
