# Vibe parity

What works for a `vibe` agent today, what does not, and the plan for closing the four gaps: sessions, spend, agent mail and context sizing. After reading you can pick up any workstream knowing the constraints it must keep, and check the evidence that the built-in `vibe` profile is Mistral's Vibe CLI.

Checked 2026-11-07 against `main` at 739ed7c and the Vibe CLI docs at docs.mistral.ai/vibe/code. Everything here is built unless marked planned.

## Why this profile is Mistral's Vibe CLI

The built-in `vibe` profile in `internal/tools` is data, and every line of it matches the Vibe CLI (package `mistral-vibe`, command `vibe`):

| Profile field | Vibe CLI behaviour |
|---|---|
| `command = "vibe"` | the command the installer provides |
| `auto = "--auto-approve"` | the auto-approve agent, `--auto-approve` (alias `--yolo`) |
| `smart-approve = "--smart-approve"` | the smart-approve agent |
| `ask = "--agent ask"` | the ask agent |
| `resume = ["--resume", "{id}"]` | `vibe --resume SESSION_ID`, partial ids accepted |
| trust store `$VIBE_HOME/trusted_folders.toml`, default `~/.vibe` | trusted folders remembered in `~/.vibe/trusted_folders.toml`; `VIBE_HOME` moves all state |

No other agent CLI has these three agents or this trust file. Links to the doc page for each row are in Sources, below.

## What works, and what does not

Works today: spawn, the `vibe` status patterns, starting modes, trust-store pre-approval, sessions and adopt (`fleet sessions`, `fleet adopt`), spend (`fleet spend`), resume arguments (stored for park/wake, which is planned), one worktree per agent, and `fleet snap` fixtures that keep the pane readable.

| Capability | copilot | claude | vibe | Where it lives |
|---|---|---|---|---|
| `fleet sessions` and `fleet adopt` | yes | yes | yes | `internal/sessions`, driven by the profile's `session_path` (ADR-62) |
| `fleet spend` | credits | tokens | tokens | `internal/spend` (ADR-46, ADR-63) |
| agent mail through peer MCP tools | yes | yes | no | `peer_args` on the profile, `internal/mail` |
| context sized by effort level | yes | yes | no | `context_args` (ADR-60) |

The `vibe` profile points `session_path` at `~/.vibe/logs/session/unified/{id}` and `parse()` reads `meta.json` (ADR-62), so adopt works on sessions started outside fleet. The store follows `VIBE_HOME` and `[session_logging] save_dir`: if yours is elsewhere, override `session_path` in `~/.fleet/tools.toml`. Still open: the post-spawn session id lookup and later park/wake for vibe agents.

## Constraints every change keeps

- Data, not code. Tool knowledge goes in profiles and patterns; Go only where it is generic: one parser per store, the mail wiring.
- Scrape first, hooks later (ADR-7). A misread vibe screen is fixed with a `fleet snap` fixture, not a special case.
- Spend is read from the tool's own session files, never from a model (ADR-46).
- State on disk and in tmux (ADR-4). The board stays the only cross-tool record.
- One reviewable commit per change, a CHANGELOG line in the same commit, an ADR claimed with `fleet adr claim` when the change decides something (DESIGN.md §14).
- Tests use the fake agent. No real vibe sessions in tests.
- No private data in fixtures or docs. `scripts/check-public.sh` is the gate.

## Workstreams

### 1. Find the session store

The spike everything else needs. Run real `vibe` sessions locally (interactive, with `--output-dir`, resumed by id), with and without `VIBE_HOME`, and record: where session files land, how the session id appears in the name, the format, how the workdir is associated, where title, model and token usage live, and how "in use" can be detected. Anonymize the fixtures.

Known already: sessions are saved by default, controlled by `[session_logging]` in `~/.vibe/config.toml`, `--resume` needs them, and all state sits under `VIBE_HOME`, default `~/.vibe`.

Done when the `session_path` glob and the parser can be written from the findings alone.

Done by source inspection (2026-11-07); the live-run confirmations stay on fleet#3: a real `meta.json`, the has_prompt rule against a live session, and a real anonymized fixture through `scripts/check-public.sh`.

### 2. Sessions and adopt

Add `session_path` to the built-in `vibe` profile from the spike, then teach `parse()` in `internal/sessions` the store format, the same way the Copilot and Claude parsers work: id, title, dir, model, has_prompt, in use. The glob should respect `VIBE_HOME` when it is set. Also confirm the spawn flow records the agent's real vibe session id on the board after start.

Done when `fleet sessions` lists vibe sessions (words-filtered, in use skipped) and `fleet adopt` resumes one as an agent, with copilot and claude output unchanged.

Built (2026-11-07, fleet#9, ADR-62): `session_path = "~/.vibe/logs/session/unified/{id}"`, and `parse()` reads `meta.json` — id cross-checked against the folder, title, dir, branch, model, end time — with has_prompt as the presence of a `core_input` record in the newest `journal/*.jsonl` (the text is not extracted), and in use read from the store's `active/<id>.lock.json` lease. Copilot and claude output unchanged. Not done: the glob is a static path, so a custom `VIBE_HOME` or `save_dir` is a `session_path` override in `~/.fleet/tools.toml`, not automatic; and the post-spawn id lookup for a spawned vibe agent is unconfirmed.

### 3. Spend

A vibe reader in `internal/spend`: calls and token usage from the session files, wired through `fleet spend` and its JSON output. The fleet total must stay honest across mixed units (Copilot credits, Claude and vibe tokens), and a missing store still renders "no usage recorded", not an error.

Done when `fleet spend` shows a vibe agent's tokens and includes them in the total.

Built (2026-11-07, fleet#8, ADR-63): a `vibe` reader in `internal/spend` keyed by the profile's pattern set, parsing `completion_succeeded` journal records (pinned by a fixture from the 3.1.0 source; a line that does not match is skipped). A session's usage is summed over its journal chunk files, each through the size-and-mtime cache; no credits, so vibe tokens land in the uncredited side of a mixed total; a missing or unreadable store still renders "no usage recorded". Still open: confirmation against a real journal, riding with the #3 live-run items.

### 4. Agent mail through peer MCP tools

Claude agents get the agent-scoped fleet MCP server through `peer_args = ["--mcp-config", "{peer_mcp_claude}"]` (DESIGN.md §6.6). The Vibe CLI configures MCP servers in `config.toml` (`[[mcp_servers]]`, stdio or http), not on the command line, so the mechanism needs its own spike: a flag for one session, or a config written per agent at spawn under the same lock-and-rename discipline as the trust store. A per-agent config must not disturb the user's credentials, and the user's own config file is never written. Access control does not change: free inside a team or repo, one agent asks otherwise.

Done when two vibe agents in one team exchange mail through peer MCP tools and a cross-team message still asks.

### 5. Context sized by effort

Copilot profiles drop built-in tools the fleet does not use (ADR-60, a light agent starts about 20k tokens lighter). The Vibe CLI restricts tools in programmatic mode (`--enabled-tools`, `--disabled-tools`) and per agent in config, so the mechanism must be checked against an interactive session first, not assumed.

Done when the `vibe` profile has `context_args` for `light`, `standard`, `pro` and `leader`, and a light agent's context fill is measurably below a pro one, numbers recorded with the date and method.

### 6. Docs and upstream packaging

Each workstream lands as one upstreamable branch, with no fork-only deltas in shared code. Update docs/profiles.md and the README where behaviour changed, resolve DESIGN.md's "still to be confirmed" notes as facts land, and re-check this page against reality when the work is done.

Done when every workstream maps to one branch that could be offered to sturlabragason/fleet as is.

## Non-goals

- Hosted Vibe (the browser and mobile app) is out of scope. Fleet supervises terminal processes.
- `--prompt` for the first prompt is wrong on purpose: it is programmatic mode, one task and exit. Fleet pastes instead.
- No `launch_id`: the CLI does not take a session id at launch; the id is found on disk after start.
- Windows is out of scope here. Fleet is Linux-only (tmux, bash, Linux the only platform tested). The Vibe CLI itself runs on Windows, but running fleet under WSL is a separate investigation.
- No new architecture, no fork-only features.

## Sources

Checked 2026-11-07:

- Vibe CLI docs: [install and setup](https://docs.mistral.ai/vibe/code/cli/install-setup) (install, `~/.vibe`, `VIBE_HOME`), [safety, approvals, and permissions](https://docs.mistral.ai/vibe/code/safety-approvals-permissions) (agents, `--auto-approve`), [work with the CLI](https://docs.mistral.ai/vibe/code/cli/work-with-cli) (`--resume SESSION_ID`, `trusted_folders.toml`), [configuration reference](https://docs.mistral.ai/vibe/code/cli/configuration-reference) (`[session_logging]`, `[[mcp_servers]]`).
- fleet: DESIGN.md (session discovery, §6.6 peer MCP, §14 ADRs), docs/architecture.md, internal/tools (built-in profiles), internal/sessions.
