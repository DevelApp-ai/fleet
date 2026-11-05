// Command fleet is the CLI and TUI for the agent fleet.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/sturlabragason/fleet/internal/logx"
)

func main() {
	logx.Init()
	if err := run(os.Args[1:]); err != nil {
		var code exitCode
		if errors.As(err, &code) {
			os.Exit(int(code))
		}
		fmt.Fprintln(os.Stderr, "fleet:", err)
		os.Exit(1)
	}
}
