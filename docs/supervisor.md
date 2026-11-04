# Supervisor

The supervisor is an ordinary agent in the left pane of `fleet up`. It steers the fleet through MCP tools served by `fleet mcp`. It is not special inside fleet: it runs from a profile like any other agent, and you can close it and start it again without losing anything, because fleet's state is in tmux and `~/.fleet/`.

## How it starts

- Profile: `copilot`, or `FLEET_SUPERVISOR_PROFILE` (any profile, including `fake`).
- Directory: `supervisor/` next to the directory holding the `fleet` binary (works for `make build`), or `FLEET_SUPERVISOR_DIR`.
- That directory holds `.mcp.json`, which registers the `fleet` MCP server as `fleet mcp`. It expects `fleet` on `PATH`; `make install` puts it in `~/.local/bin`.
- The supervisor's rules (tools only, short replies, the confirmation rule) are in `internal/mcp/instructions.md`, built into the binary and sent as the MCP server instructions.
- With a `copilot` profile, `fleet up` starts it lean: `--no-custom-instructions --allow-all-mcp-server-instructions --available-tools fleet --allow-tool fleet --disable-builtin-mcps`, plus `--disable-mcp-server` for each server in `~/.copilot/mcp-config.json`. It sees only the fleet tools: no shell, no file reading, and about 8k input tokens per turn instead of ~160k.

If you use a different tool as supervisor, register `fleet mcp` as a stdio MCP server in that tool's own config.

## MCP tools

`fleet mcp` speaks MCP (protocol `2025-06-18`) over stdin/stdout.

| Tool | Arguments | What it does |
|---|---|---|
| `fleet_status` | none | Table of agents: name, profile, status, on wall, time in status, and the last line for waiting/error/stuck agents |
| `fleet_peek` | `name`, `lines` (default 20, max 80) | Last lines of the agent's screen |
| `fleet_send` | `name`, `text` | Paste text into the agent and press Enter |
| `fleet_answer` | `name`, `answer` (`y`/`n`), `confirmed` | Answer a y/n prompt |
| `fleet_spawn` | `name`, `profile`, `dir`, `prompt` (optional) | Start an agent |
| `fleet_profiles` | none | Profiles and their command lines |
| `fleet_close` | `name`, `confirmed` | Stop the agent and remove it from the board |
| `fleet_wall` | `action` (`show`/`hide`/`preset`), `name` or `preset` | Change the wall |
| `fleet_note` | `text`, `agent` (optional) | Append a line to `~/.fleet/decisions.log` |
| `fleet_brief` | none | Latest handoff, last 30 notes, then the `fleet_status` table. A fresh supervisor calls this first. |
| `fleet_handoff` | `summary` | Write `~/.fleet/supervisor/handoff.md` and replace the supervisor with a fresh session |

## Confirmation rule

`fleet_answer` and `fleet_close` refuse to run unless `confirmed=true`. The supervisor's instructions tell it to ask you in one line first, for example "Answer y to web-nav (run `make test`)?", and to set `confirmed=true` only after an explicit yes.

This is enforced in code only to the extent that the flag has to be set. The model sets the flag, so the actual safeguard is the instruction plus your reply. Ordinary prompts through `fleet_send` need no confirmation. They also tell the supervisor to ask before telling any agent to merge, push to a shared branch or apply infrastructure.

Planned ([DESIGN.md §8, ADR-9](../DESIGN.md#14-decisions-adrs)): risky actions shown as approval cards in the TUI, and a blue flash on tiles the supervisor acts on.

## Keeping its context small

`fleet_peek` returns at most 80 lines, and the instructions ask the supervisor to peek only when needed. Planned: `peek` returns a ~5-line summary from a cheap model instead of raw text.

Fleet doesn't compact the supervisor's chat; it replaces it ([DESIGN.md §6.5, ADR-11](../DESIGN.md#65-supervisor-memory-notes-handoff-context-meter-r11)):

- The supervisor records decisions, your preferences and promises with `fleet_note`.
- Its context fill is read from Copilot's `events.jsonl` usage records and shown on the pane border (`supervisor · ctx 34%`, green/yellow/red) and by `fleet supervisor`.
- At 70% (`FLEET_SUP_HANDOFF_PCT`) every fleet tool result ends with a warning. The supervisor then calls `fleet_handoff` with open questions, promises and plans in flight. Fleet respawns the pane with a new session whose first prompt is to call `fleet_brief`.
- `Alt-R` (tmux), `R` (overview) or `fleet supervisor reset` does the same by hand, after a y/n prompt. The old session isn't deleted: `copilot --resume <id>` still opens it.

Agents are not affected: they live on the board and in tmux, not in the supervisor's chat.

## Try it without tokens

`FLEET_SUPERVISOR_PROFILE=fake fleet up` puts a fake agent in the supervisor pane. To exercise the MCP server directly:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fleet_status","arguments":{}}}' \
  | fleet mcp
```
