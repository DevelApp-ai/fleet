package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/playbook"
	"github.com/sturlabragason/fleet/internal/worktree"
)

// exitCode makes run end the process with this status and no error text.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit %d", int(e)) }

// cmdPlaybook prints the worker playbook an agent spawned in dir would get.
func cmdPlaybook(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: fleet playbook [dir]")
	}
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}
	text, src := playbook.Resolve(logx.Home(), dir)
	fmt.Fprintf(os.Stderr, "# sources: %s\n", strings.Join(src, ", "))
	fmt.Print(text)
	if text != "" && !strings.HasSuffix(text, "\n") {
		fmt.Println()
	}
	return nil
}

// cmdTouching lists the files other worktrees of this repo change; with file
// arguments it reports only overlaps. Exit 0: no overlap, 1: overlap, 2: error
// (so a failed check never reads as all-clear).
func cmdTouching(args []string) error {
	code, err := touching(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fleet touching:", err)
		return exitCode(2)
	}
	if code != 0 {
		return exitCode(code)
	}
	return nil
}

func touching(args []string) (int, error) {
	fs := flag.NewFlagSet("touching", flag.ContinueOnError)
	dir := fs.String("C", ".", "directory inside the repo")
	asJSON := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return 0, err
	}
	ws, err := worktree.Others(*dir)
	if err != nil {
		return 0, err
	}
	files := fs.Args()
	if len(files) > 0 {
		top, err := worktree.TopLevel(*dir)
		if err != nil {
			return 0, err
		}
		cwd, err := os.Getwd()
		if err != nil {
			return 0, err
		}
		for i, f := range files {
			files[i] = worktree.RepoRel(top, cwd, f)
		}
		ws = worktree.Overlap(ws, files)
	}
	if *asJSON {
		if ws == nil {
			ws = []worktree.Worktree{}
		}
		b, _ := json.MarshalIndent(ws, "", "  ")
		fmt.Println(string(b))
	} else if len(files) > 0 && len(ws) == 0 {
		fmt.Println("no other worktree touches these files")
	} else {
		for _, w := range ws {
			fmt.Printf("%s  %s  %d files\n", w.Path, w.Branch, len(w.Files))
			for _, f := range w.Files {
				fmt.Printf("    %s\n", f)
			}
		}
	}
	if len(files) > 0 && len(ws) > 0 {
		return 1, nil
	}
	return 0, nil
}
