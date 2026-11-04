// Command fakeagent prints agent-like output and cycles through states.
package main

import (
	"os"

	"github.com/sturlabragason/fleet/internal/fake"
)

func main() {
	name := "fake"
	if len(os.Args) > 1 {
		name = os.Args[1]
	}
	fake.Run(name, os.Stdin, os.Stdout)
}
