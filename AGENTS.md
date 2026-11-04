# fleet: agent notes

The spec is DESIGN.md (requirements in §0, decisions in §14). Read it before changing behaviour.

# Commands
- Build: make build
- All tests: make test (non-blocking; output also goes to ~/.fleet/logs/test.log)
- One package: go test ./internal/<pkg>/
- Live dev loop: make dev (tmux -L fleet-dev; rebuild + restart on save)
- End-to-end check with fake agents: fleet spawn --fake a1 && fleet ls --json

# Rules that can't have exceptions
- Only use tmux sockets `fleet`, `fleet-dev`, `fleet-test-*`. Never touch the default tmux server or any running copilot/claude/agy/vibe process.
- No pkill/killall; use a specific PID or `tmux -L <own-socket> kill-server`.
- Never push. Commits are local.
- Don't spawn real claude/agy/vibe agents in tests; they cost tokens. Use the fake agent.

# Conventions
- One package per component (DESIGN.md §15), each with a _test.go. Small files.
- Pane geometry lives only in internal/layout.
- Tool profiles and status patterns are data (~/.fleet/tools.toml, patterns), not code.
- Log with slog and a "component" attribute. Recover and log panics in the TUI loop.
- Dependencies: stdlib + Charm libraries only.

# Gotchas
- Bash aliases don't exist inside tmux spawns; tool variants are profiles.
