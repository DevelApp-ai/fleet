// Command fleet is the CLI and TUI for the agent fleet.
package main

import (
	"fmt"
	"os"

	"github.com/sturlabragason/fleet/internal/logx"
)

func main() {
	logx.Init()
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fleet:", err)
		os.Exit(1)
	}
}
