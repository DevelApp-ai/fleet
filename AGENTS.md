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
- Push `main` only, and only as a fast-forward; the `.githooks` pre-push check is the gate. Never force-push, never `--no-verify`, never `git push --tags`.
- Don't spawn real claude/agy/vibe agents in tests; they cost tokens. Use the fake agent.

# Conventions
- One package per component (DESIGN.md §15), each with a _test.go. Small files.
- Pane geometry lives only in internal/layout.
- Tool profiles and status patterns are data (~/.fleet/tools.toml, patterns), not code.
- Log with slog and a "component" attribute. Recover and log panics in the TUI loop.
- Dependencies: stdlib + Charm libraries only.

# Gotchas
- Bash aliases don't exist inside tmux spawns; tool variants are profiles.
- Spawn behaviour (starting mode, auto-submit, extra args, local profile overrides) comes from the user's uncommitted `$XDG_CONFIG_HOME/fleet/config.toml`. Tests must pin `FLEET_CONFIG` to a nonexistent path so the real file can't change results. See `examples/config.toml.example`.

# Building features
"Implement <X>" runs the `fleet-feature` skill (.claude/skills/fleet-feature): spec in DESIGN.md → brief in .work/briefs/ → fleet-builder → fleet-reviewer → done-check → docs → local commit → live rollout. Agents live in .claude/agents/ (symlinked into .github/agents/ for Copilot).
