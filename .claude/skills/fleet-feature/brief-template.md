# Brief: <feature>

- Spec: DESIGN.md §<n> (<title>)
- Slug / branch: <slug>

## Goal
<one or two sentences, user-visible>

## Behaviour
- CLI: `fleet <cmd> ...` → <what happens, output>
- TUI/keys: <if any>
- MCP: <tool name + args, if the supervisor should use it>
- Errors: <what the user sees when it fails>

## Where
- Packages/files likely touched: <internal/x, cmd/fleet/y.go>
- New package? Only if it is a new component (DESIGN.md §15).

## Tests
- <unit cases that would fail without the change>
- End-to-end with the fake agent on a `fleet-test-*` socket, if it touches tmux.

## Done-check (must exit 0)
make build && go vet ./... && go test ./...
<feature-specific command, if any>

## Out of scope
- <things not to do now>
