package mcp

// resetShell is the run-shell command fleet_handoff uses to replace the
// admiral (qexe: the shell-quoted fleet binary); the delay lets the tool
// result reach the admiral first.
func resetShell(qexe string) string {
	return "sleep 2; " + qexe + " admiral reset --handoff >/dev/null 2>&1"
}
