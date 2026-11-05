# ai-fleet: one place to see and steer every agent session

Status: written 2026-11-04 from a planning conversation, before any code. Slices 0–3 and the MCP server are built; see the README for what works today. Everything else here is planned.
Section 0 lists every requirement I stated. Sections 1–7 and 5a are the design we discussed. Section 8 lists additions that came up while writing. Section 14 records the decisions and why.

---

## 0. Requirements

What I asked for, in my own terms. Each one is tagged so the design and decisions can refer to it.

| ID | Requirement | Where it's handled |
|---|---|---|
| R1 | One central place to see and steer up to 40 sessions of Copilot, Claude Code, agy and vibe | §3, §4 |
| R2 | A central LLM session that can reach into the other sessions, spawn new ones and run agent workflows | §6 |
| R3 | A TUI, not a web UI | §5 |
| R4 | Visual: a grid of tiles coloured green, yellow and red, with animation | §5 |
| R5 | Keyboard driven, with arrow keys | §5 |
| R6 | Mouse: click into session windows | §5a |
| R7 | Tiling inside the TUI, with splits, to use a large monitor | §5a |
| R8 | Live session windows in the grid, not only summaries | §5a |
| R9 | Fast and efficient | §5a, §7, ADR-3, ADR-5 |
| R10 | Solve the RAM problem of running many agents | §7 |
| R11 | The central session must not get overloaded by every agent's context, and must be easy to clear or manage | §6 |
| R12 | Very short dev feedback loop: keep it open and add or remove features live while we talk | §10 |
| R13 | As simple as possible to build | §9, ADR-1 |
| R14 | Better than agent-deck, which only helps me switch between sessions | §1 |
| R15 | Low maintenance: not yet another tool that keeps breaking and needs constant fixing | §16, ADR-10 |
| R16 | Every component has a test, but tests never block progress | §15 |
| R17 | Go fast: no gates that slow me down; logs capture issues instead | §9, §15 |
| R18 | Modular: features can be added or removed without touching the rest | §10, §15 |
| R19 | Ready to change direction at short notice | §9 |
| R20 | Works with every tool variant I use: copilot, claude-copilot, claude, claude-personal, claude-work, agy, vibe. Profiles live in a data file, not hard-coded | §4 |
| R21 | Copilot is the main supervisor session for now | §6 |

---

## 1. The problem

I often run up to 40 agent sessions at once (GitHub Copilot CLI, Claude Code, agy, vibe), each in its own terminal. The terminals themselves are not the hard part. My attention is.

1. **I can't see the fleet's state.** I can't tell which agents are working, stuck, waiting on me or done without visiting every terminal.
2. **I'm the only router.** Every answer, approval and follow-up has to come from me, typed into the right window.
3. **Agents don't share context.** I carry the overall plan in my head and pass it between agents by hand.
4. **Starting work takes manual setup.** Splitting a task across agents means opening terminals, picking directories and pasting prompts myself.
5. **RAM runs out.** Measured on 2026-11-04 (30 GB machine): a Copilot session uses 400–540 MB and a Claude or agy session 250–280 MB. 19 sessions used about 4.6 GB, so 40 would use about 15–20 GB.
6. **A central agent would get flooded.** If one supervisor session reads raw output from 40 agents, its context fills with unrelated material and its judgement gets worse.

What I want is one place to talk to, which can see every agent and act on them for me: report state, answer or nudge agents, start new ones and coordinate multi-step work. That is a **control plane with an LLM in it**, not just a session switcher.

### Is this a known pattern?

Yes. It is usually called the supervisor (or orchestrator) and worker pattern. Existing tools such as agent-deck, claude-squad, Vibe Kanban and Crystal are mostly session managers: they make switching between agents faster, but no LLM supervises the others. agent-deck felt wrong for exactly that reason. It improves navigation but leaves all the supervising to me.

---

## 2. Design principles

1. **tmux does the work underneath.** Every agent runs in a named tmux window. The tmux server survives if the TUI or the supervisor crashes. No custom terminal handling.
2. **One CLI, two front ends.** The `fleet` CLI is the only thing that touches tmux and state. The TUI uses it, and so does the supervisor LLM. Anything I can do, the supervisor can do, and the reverse.
3. **State lives on disk, not in any chat.** If something isn't written to the board, it doesn't exist.
4. **The supervisor is stateless and can be thrown away.** Resetting it costs seconds and loses nothing.
5. **Raw output never reaches the supervisor.** Agent output is summarised by a cheap model first.
6. **Parked is the normal state, not live.** Idle agents give up their process and resume on demand.
7. **Visual first, keyboard and mouse driven.** Colour and motion tell me where to look. Arrow keys or a click get me there.
8. **Let native code do the heavy lifting.** tmux (written in C) and the terminal emulator draw the live sessions. The fleet binary only draws the overview, and only redraws when something changes.

---

## 3. Architecture

```
┌───────────────────────────── fleet TUI (Go) ──────────────────────────────────┐
│  grid of tiles  ·  attention queue  ·  supervisor pane  ·  status bar          │
└───────────────┬───────────────────────────────────────────────┬────────────────┘
                │ calls                                         │ calls
                ▼                                               ▼
        ┌──────────────┐        reads/writes          ┌──────────────────┐
        │  fleet CLI   │ ───────────────────────────▶ │  ~/.fleet/ state │
        └──────┬───────┘                              └──────────────────┘
               │ tmux new-window / capture-pane / send-keys / kill
               ▼
   tmux session "fleet": window per agent ── copilot | claude | agy | vibe
                                                   ▲
   supervisor = one more agent (copilot), uses fleet CLI via bash
```

### State layout (`~/.fleet/`)

| Path | What it holds |
|---|---|
| `board.json` | One entry per agent: name, tool, repo/dir, goal, status, blocker, session ID, live/parked, last activity, RAM |
| `agents/<name>.md` | Agent card: goal, what's done, current step, next step, open questions. 10–30 lines, rewritten (not appended). |
| `decisions.log` | Append-only, one line each: timestamp, who decided (me or the supervisor), what, for which agent |
| `events.jsonl` | Status transitions (for the TUI animation, notifications and later analysis) |
| `peek-cache/<name>.txt` | Last summary of each agent and the hash of the pane it was made from |

### Resume support (checked against `--help` on 2026-11-04)

| Tool | Set the ID at launch | Resume by ID |
|---|---|---|
| Copilot CLI | `copilot --session-id <id>` | `copilot --resume <id>` |
| Claude Code | `claude --session-id <uuid>` | `claude --resume <id>` |
| agy | not seen | `agy --conversation <id>` |
| vibe | not seen | `vibe --resume <id>` |

Where the ID can't be set at launch, `fleet` has to find it after the start. It looks for the newest session folder for that tool and directory (for Copilot, `~/.copilot/session-state/`). For agy and vibe, the location of session storage is still to be confirmed.

---

## 4. The `fleet` CLI

| Command | What it does |
|---|---|
| `fleet spawn <name> <tool> <dir> ["prompt"]` | New tmux window, starts the tool, records the session ID, creates the card |
| `fleet ls [--json]` | All agents with status, live/parked and RAM |
| `fleet peek <name> [--raw] [-n 200]` | Default: a summary of about 5 lines from a cheap model. `--raw`: the actual pane text (for me, not the supervisor). |
| `fleet send <name> "text"` | Paste into the agent and press Enter |
| `fleet park <name>` | Ask the agent to update its card, wait, stop the process, mark it parked |
| `fleet wake <name>` | Restart with the tool's resume flag in the same tmux window |
| `fleet close <name>` | Stop it and archive the card (`kill` is a hidden alias) |
| `fleet note <name> "text"` | Add an entry to `decisions.log` |
| `fleet adopt [--name N] [--force] <profile> <session-id> [dir]` | Bring an existing session (started outside fleet) under management by resuming it in tmux (see below) |

### `fleet adopt`

Moves a session that was started in a plain terminal into fleet, one at a time (§8 item 6).

- Starts the profile's `resume` args (`{id}` = the session ID) in a new hidden agent window, exactly like `spawn` but with no launch ID and no prompt. The board entry records the session ID, so `wake` and `park` work on it later.
- **dir**: given, or for Copilot the `cwd:` from `~/.copilot/session-state/<id>/workspace.yaml`, or the current directory.
- **name**: `--name`, or the base name of dir (made safe for names), with `-2`, `-3`… if taken.
- **Session must exist**: profiles may set `session_path`, a glob with `{id}` and `~` (Copilot: `~/.copilot/session-state/{id}`, Claude: `<config dir>/projects/*/{id}.jsonl`). If it is set and nothing matches, adopt fails. Profiles without it (agy, vibe, fake) skip the check.
- **Session must not be running**: two processes on one session corrupt it. Adopt refuses when a live process holds it: a Copilot `inuse.<pid>.lock` in the session folder with a live pid, or any process whose command line contains the session ID. The error names the pid and says to close that terminal first. `--force` skips both checks.
- MCP: `fleet_adopt` (profile, session_id, optional dir and name), same checks.
- Limit: a Claude session started fresh (no `--resume <id>` on its command line) has no lock file, so the in-use check can't see it. Close it before adopting.
- Out of scope: discovering sessions to adopt (a picker), closing the outside terminal for you.

### Spawn startup: autopilot and the first prompt (ADR-13)

Measured on 2026-11-04 against Copilot CLI, the two reasons a freshly spawned agent needed hand-holding:

1. **The first prompt was swallowed.** `spawn` slept a fixed 1500 ms and then pasted. The Copilot TUI is not in raw mode yet at that point, so both the pasted text *and* the Enter went nowhere — the input box stayed empty. Reproduced on `tmux -L fleet-test-autostart`: paste at 1.5 s → empty box; at 15 s → submitted.
2. **The session started in Interactive / Manual Approval.** Nothing asked for autopilot, so every tool call stopped for approval.

The fix is in two layers, both data-driven (profiles are data, ADR-5):

- **Launch-time delivery, preferred.** A profile may declare `prompt_args` (args carrying the initial prompt, `{prompt}` substituted) and `mode_args` per mode. The tool then starts already in the right mode with the prompt submitted by the tool itself — no keystrokes, no race. Copilot: `prompt_args = ["-i", "{prompt}"]`, `mode_args = { autopilot = "--allow-all --autopilot" }`. `--autopilot` on its own opens a blocking "Enable autopilot mode" permission dialog; `--allow-all` is what makes it unattended, so it is part of the mode args. Claude Code's equivalent is `--permission-mode`.

  `mode_args` and `mode_keys` are **inline tables of space-separated strings**, not of arrays: the `internal/conf` TOML subset types an inline table as `map[string]string`, and fleet splits each value on whitespace. A dotted key such as `mode_args.autopilot = [...]` is not part of the subset and is silently dropped. Limitation: an argument that itself contains a space cannot be expressed this way — a tool needing one has to get it from `args` or local `extra_args`.
- **Paste fallback, for tools without those flags.** Wait until the pane is actually ready — poll `capture-pane` until the profile's status patterns classify it as **idle or waiting** (those two describe a live input box), or an explicit `ready` regex matches — then paste, then press Enter. `working` and `error` deliberately do not count: a bare spinner glyph matches the `working` patterns and can render during startup before the TUI is in raw mode, which is the very condition that lost the prompt. Tools whose banner arrives before any input box (the `fake` agent) set an explicit `ready` regex instead. A profile may also declare `mode_keys` (keys to send once ready) for tools that only offer a keystroke. Readiness replaces the fixed sleep in both paths.

### Local spawn preferences (`$XDG_CONFIG_HOME/fleet/config.toml`)

Repo defaults stay neutral: fleet out of the box spawns in whatever mode the tool defaults to. Autopilot is a **local, per-user** choice and lives in a file that is never committed — `$XDG_CONFIG_HOME/fleet/config.toml`, default `~/.config/fleet/config.toml` (`FLEET_CONFIG` overrides the path, for tests). It is the same TOML subset as `tools.toml`.

```toml
[defaults]                      # applies to every profile
mode = "autopilot"              # "" = leave the tool's default
auto_submit = true              # press Enter after a pasted prompt
ready_timeout = "15s"           # how long to wait for the TUI before giving up

[profile.copilot]               # per-profile override of the above
mode = "autopilot"
extra_args = ["--reasoning-effort", "high"]

[profile.claude]
mode = "accept-edits"

[profile.my-copilot]            # defines a new profile, or overrides a built-in
command = "copilot"
args = ["--model", "claude-opus-5"]
patterns = "copilot"
```

- `[profile.<name>]` is a third override layer for tool profiles, after the built-ins and `~/.fleet/tools.toml`; it accepts every `tools.toml` key, so a local file can define or redefine a profile without touching the repo.
- `mode` names a key in the profile's `mode_args` / `mode_keys`. An unknown mode for a profile is logged and ignored rather than failing the spawn.
- `extra_args` are appended to the launch argv.
- Precedence: `[profile.<name>]` > `[defaults]` > built-in (no mode, `auto_submit = true`, `ready_timeout = 15s`). `spawn` blocks for up to `ready_timeout`, including the `fleet_spawn` MCP call, so the default stays well under a typical MCP client tool timeout.
- `auto_submit = false` also disables launch-time prompt delivery: `copilot -i <prompt>` submits the prompt itself, so honouring "paste but don't press Enter" means falling back to the paste path.
- A `[profile.<name>]` section that neither names an existing profile nor sets `command` is ignored with a warning, so a typo like `[profile.coplot]` cannot create a command-less profile.
- Out of scope: changing the mode of an already running agent, and a CLI flag per spawn.
### Finding sessions to adopt: `fleet sessions`

Nobody remembers session IDs. Fleet finds them from the tools' own session stores, so adopt works with words.

- `fleet sessions [--all] [--json] [-n 20] [words...]` lists sessions newest first: age, suggested profile, dir, title, short ID, and `in use` / `on board` markers. Title = the tool's own name for the session (Copilot `workspace.yaml` `name`/`summary`, Claude `custom-title`/`summary`), else the first user prompt, cut to one line.
- Sources are the profiles' `session_path` globs (no new config). Several profiles sharing one store (copilot, claude-copilot) give one source; the suggested profile is the one whose `--model` matches the session's model family (claude vs gpt), else the one with no args.
- Hidden unless `--all`: sessions with no user prompt (hook and probe runs) and sessions already on the board.
- `words`: case-insensitive, every word must appear in title, dir, branch or ID.
- `fleet adopt [--name N] [--dir D] [--force] [profile] <id | words...>`: the profile is optional (suggested one is used). An exact ID or a unique ID prefix of at least 8 characters adopts that session; otherwise the words search, leaving out sessions in use or on the board. One match is adopted, none or several is an error that lists up to 5 candidates. A positional dir after an exact ID still works.
- MCP: `fleet_sessions` (optional `query`, `limit`) returns one line per session; `fleet_adopt` takes `session_id` or `query`, `profile` optional. Supervisor rule: on "adopt my X session" call `fleet_sessions` with the user's words, ask which one if more than one, then `fleet_adopt` with the ID.
- Reading is bounded: only the newest sessions are opened, and at most the first 256 KB of a transcript is read.
- In use = a live Copilot `inuse.<pid>.lock`, or a process with the ID as a whole argument (tmux and screen servers are ignored: they keep the command they were started with).
- Built 2026-11-04.
- Out of scope: a TUI picker, agy and vibe stores.

### Status detection

Status is read from the bottom of the pane each poll (every 1–2 seconds):

| Status | Signal |
|---|---|
| working | pane text changed since the last poll, or the tool's spinner or "thinking" marker is visible |
| waiting | approval prompt, y/n prompt or question form (one pattern set per tool) |
| error | error banner, crashed process or non-zero exit |
| stuck | working, but the pane hash hasn't changed for N minutes (default 10) |
| idle | turn finished: empty input box and the tool's idle footer, nothing asked |
| parked | no process; resumes on demand |

How the patterns are applied (`internal/status`, per-tool sets in `patterns.toml` with keys `waiting`, `error`, `working`, `idle`, `transcript`):

- Only the last 14 non-empty lines are read: enough for a question form or approval box plus the footer under it.
- **waiting wins anywhere in that window.** A Copilot approval box ("Do you want to run this command?", "↑/↓ to navigate · enter to select · esc to cancel") or question form ("Copilot needs information.", "↑/↓ select · enter accept …") can sit above a footer that still says "Working". The line matched by the first `waiting` pattern in list order becomes `waiting_for` (with the question under "needs information." appended).
- `transcript` marks lines of the scrolling conversation (Copilot: lines ending in the right-hand scrollbar `┃`). A waiting pattern there only counts on the transcript's last content line, so prompt text an agent quoted earlier in its work doesn't make it look blocked.
- Otherwise the **bottom-most** line matching `error`, `working` or `idle` decides — normally the tool's footer. Copilot: busy is "● Working · … esc esc interrupt"; idle is the mode footer ("Interactive · Allow All · / commands · space hold to record") without "Working". Picker hints like "esc to cancel" mean waiting, never working.
- tmux's "Pane is dead (status …)" banner, or a dead pane, is always `error`.
- `fleet refresh` tags the pane (`@fleet_status`, shown in the pane title) on every transition, and re-syncs the tag whenever it differs from the board.

Hooks are better where they exist. Claude Code and Copilot can run hooks on events such as "turn finished" and "needs permission". Those hooks can write status straight to the board, which is more reliable than scraping. Use scraping as the fallback that works with every tool.

---

## 5. The TUI

Built in **Go with Bubble Tea and Lip Gloss** (see ADR-10). It's one binary, the same one as the CLI. Go 1.26 is already installed.

### Layout

```
┌ fleet ─ 31 agents · 9 live · 22 parked · RAM 4.1/30 GB · ⚠ 3 waiting ───────────────┐
│ ┌─ api-auth ───────┐ ┌─ web-nav ────────┐ ┌─ infra-dns ──────┐ ┌─ db-migrate ─────┐ │
│ │ 🟢 ⠹ copilot      │ │ 🟡 ● claude      │ │ 🔴 ✖ agy          │ │ 💤 vibe          │ │
│ │ api              │ │ web              │ │ infra            │ │ db               │ │
│ │ running go test  │ │ approve: npm test│ │ plan failed      │ │ parked 2h        │ │
│ │ 512 MB · 3m      │ │ 280 MB · wait 4m │ │ 260 MB · 12m     │ │ —                │ │
│ └──────────────────┘ └──────────────────┘ └──────────────────┘ └──────────────────┘ │
│  … more rows, grouped by repo …                                                      │
├ supervisor ─ ctx 34% 🟢 ─────────────────────────────────────────────────────────────┤
│ > 3 agents are waiting. web-nav wants to run `npm test` — approve?                  │
└ ←↑↓→ move · ⏎ attach · Tab next waiting · s send · p peek · z park · ? help ─────────┘
```

### Tile states

| State | Colour | Motion |
|---|---|---|
| working | green | spinner turning, last line of output scrolling |
| waiting for me | yellow | slow pulse of the border; tiles that have waited longer pulse brighter |
| error / stuck | red | still border, ✖ icon; stuck also shows "no output 12m" |
| idle / done | grey | dimmed, no motion |
| parked | dark grey | 💤, no process, no RAM shown |
| supervisor | own tile | context fill: green < 40%, yellow 40–69%, red ≥ 70% (built as the supervisor pane border label, §6.5) |

Motion stays rare on purpose. If everything moves, nothing stands out. Only working tiles spin and only waiting tiles pulse.

### Keyboard (yes, arrow driven)

| Key | Action |
|---|---|
| `←` `↑` `↓` `→` (also `h` `j` `k` `l`) | Move between tiles |
| `Enter` | Attach to that agent's tmux window. Return to the grid with a tmux key (for example `prefix + g`). |
| `Tab` / `Shift+Tab` | Jump to the next/previous waiting tile, longest wait first |
| `Space` | Peek: overlay with the 5-line summary. Press `r` in the overlay for raw output. |
| `s` | Send a prompt to the selected agent (one-line input) |
| `a` / `x` | Approve / reject when the agent is at a y/n prompt |
| `n` | Spawn wizard: name, tool, directory, first prompt |
| `z` / `w` | Park / wake |
| `f` | Focus session on this agent (see 6.4) |
| `c` | Move the cursor to the supervisor input |
| `R` | Reset the supervisor, after a y/n confirm (see 6.3, 6.5; tmux `M-R` does the same) |
| `/` | Filter tiles by name, repo or status |
| `1`–`9` | Jump to a repo group |
| `?` | Help overlay |

### Hotkey hints and help (built)

The bottom tmux status line and the overview header show a short hint in plain words, written with `Alt-` instead of tmux's `M-`. The full list is one keypress away:

- **`Alt-?`** anywhere in the fleet tmux server opens a popup (`tmux display-popup`) with every hotkey, grouped by area (panes, wall, supervisor, overview), one line each. Any key closes it. `fleet keys` prints the same list in a terminal.
- **`?`** in the overview opens the same list as an overlay; `q`, `Esc` or `?` closes it.
- One registry (`internal/keys`) is the single source of truth: the status hint, the overview hint, the popup, the overlay, `fleet keys` and the generated table below come from it. Tests fail if a tmux `M-*` or `fleetwall` chord binding in `fleet up` is missing from the registry (or the reverse), or if the table below is out of date (`fleet keys --markdown` regenerates it).

<!-- keys:begin -->
| Group | Key | Description |
|---|---|---|
| Panes | `Alt-←↑↓→` | move to the pane in that direction |
| Panes | `Alt-z` | zoom this pane; press again to restore |
| Panes | `Alt-x` | hide this pane (the agent keeps running) |
| Panes | `Alt-o` | jump to the marked pane (right-click → Mark) |
| Wall | `Alt-w 1/2/4/6/9/m` | same as Alt-1…9/m: Alt-w, let go, then the key |
| Wall | `Alt-1/2/4/6/9` | wall: show up to N panes |
| Wall | `Alt-m` | wall: one main pane + stack |
| Supervisor | `Alt-s` | show/hide the supervisor |
| Supervisor | `Alt-S` | move the supervisor left/right |
| Supervisor | `Alt-R` | reset the supervisor to a fresh chat (asks y/n) |
| Overview | `Alt-c` | overview: tiles ↔ compact chips |
| Overview | `←↑↓→ / hjkl` | select a tile |
| Overview | `⏎ / click` | show the selected agent on the wall |
| Overview | `x` | take the selected agent off the wall |
| Overview | `Tab` | jump to the next waiting agent |
| Overview | `1/2/4/6/9/m` | same as Alt-1…9 / Alt-m |
| Overview | `R` | same as Alt-R: reset the supervisor (asks y/n) |
| Overview | `q` | quit the overview |

`Alt-?` (tmux) · `?` (overview): show this list.
<!-- keys:end -->

Out of scope: rebinding keys from config, mouse-clickable hints.

---

## 5a. Screen layout: a tiled wall of live sessions (R6, R7, R8, R9)

I want live session windows on a grid that I can click into, tiled across a large monitor. Drawing 40 live terminals inside the app itself would be slow. So the design splits the work between two layers:

- **Overview (fleet binary):** small tiles for *all* agents, showing summaries only. Cheap to draw.
- **Live wall (tmux panes):** real, interactive terminals for the 1–9 agents I'm working with right now. tmux and the terminal emulator draw these, both written in native code.

Everything sits in one tmux window, so one terminal window fills the monitor:

```
┌ overview (fleet) ─────────┬ live wall: real tmux panes ────────────────────────────┐
│ 🟢 api-auth    🟡 web-nav │ ┌ web-nav (claude) ───────────┬ infra-dns (agy) ───────┐ │
│ 🔴 infra-dns   💤 db-mig  │ │ Approve `npm test`? [y/n]   │ Error: plan failed in  │ │
│ 🟢 ci-cache    ⚪ docs    │ │ █                           │ modules/dns            │ │
│ … 34 more, by repo …      │ ├─────────────────────────────┼────────────────────────┤ │
│                           │ │ api-auth (copilot)          │ ci-cache (claude)      │ │
│                           │ │ ⠹ running go test ./...     │ ⠹ writing tests        │ │
├ supervisor ───────────────┤ └─────────────────────────────┴────────────────────────┘ │
│ > 3 waiting. Approve…     │                                                         │
└───────────────────────────┴─────────────────────────────────────────────────────────┘
```

**How it works**

- Each agent's pane lives in a hidden tmux window when it isn't on the wall.
- Clicking a tile in the overview (or pressing `Enter`) moves that agent's pane onto the wall with `tmux join-pane`/`swap-pane`. If the wall is full, the slot used least recently goes back to its hidden window. The process keeps running throughout. Nothing restarts.
- **Mouse:** `set -g mouse on`. Click a live pane to type into it. Drag a border to resize. Scroll to read history. Click an overview tile to bring that agent onto the wall.
- **Tiling layouts**, chosen from the overview (`1 2 4 6 9 m`), globally with `Alt+w` then the digit, or `Alt+<digit>` in terminals that don't use Alt+digit for tabs (Ptyxis and GNOME Terminal do): `1` one large pane; `2` side by side; `4` a 2×2; `6` a 3×2; `9` a 3×3; `M` main + stack (one large pane plus a column of small ones). Built from tmux's own layouts (`main-vertical`, `tiled`) plus saved custom layout strings.
- **Pin** (`P`) keeps a pane on the wall so it is never swapped out.
- **Zoom** (`Z`, or tmux `prefix + z`) makes one pane fill the whole wall for a moment.
- Wall panes show a coloured border in the same colour as their tile, so status is visible in both places.
- Named wall setups ("web work", "infra incident") save and restore which agents are on the wall and in which layout.

**Why this is fast**

- Agents off the wall have no on-screen terminal. Their output costs nothing to display.
- Status changes arrive as events from **tmux control mode** (`tmux -C`, `%output` messages per pane) instead of reading 40 panes every second. Panes are only captured when their output actually changed, and only the last lines.
- The overview redraws only the tiles that changed. Animation runs on a 10 fps timer, and only for tiles that are spinning or pulsing.
- Agents run on a separate tmux server (`tmux -L fleet`) so my own tmux sessions and the dev loop never get mixed up with the fleet.
- Terminal emulator: today it's a VTE-based terminal. If drawing feels slow on the big monitor with 9 busy panes, try a GPU-accelerated terminal (kitty, WezTerm, Ghostty or foot). Nothing in the design depends on which one.

---

## 5b. Smarter wall tiling (R4, R7, R9)

The first wall was dumb in three ways: it evicted by last use alone, so a pane waiting for my answer could be thrown off the wall by a pane I had just opened; the grid shape was a fixed preset, so five panes on a 300-column monitor still drew a 2×2 and hid one; and every pane got the same size whether it was asking me a question or quietly running tests. Three rules fix that. All three are pure geometry plus pane status, so they live in `internal/layout` (shape) and `internal/wall` (which pane, which status).

### 5b.1 Evict by status, not just last use

When the wall is full and another pane has to come on, the pane that leaves is chosen by **class first, last use second**:

| Class | Panes | Evicted |
|---|---|---|
| 0 | pinned (`@fleet_pin` = `1`) | never |
| 1 | `waiting`, `error`, `stuck` | never |
| 2 | `working` | only if no class-3 pane is left |
| 3 | `idle`, `gone`, `unknown`, untagged | first |

Within a class the least recently used pane (`@fleet_used`) goes. A pane that needs me (`waiting`) or has failed (`error`, `stuck`) is never swapped out automatically — those are exactly the panes I opened the wall to see. If every pane on the wall is protected, the new pane is **not** forced on: tmux shows `wall full: every pane is waiting or failing (M-x to free one)` and nothing moves. Pressing `M-x` (explicit hide) always works, whatever the status.

Status comes from the pane itself: `fleet refresh` already tags `@fleet_status` on every transition, and now also `@fleet_status_since` (unix nanoseconds). No board read from the wall code, so state stays in tmux (ADR-1, ADR-4).

Pinning (`P` in §5a) is **specified here but not built**: class 0 exists in the ordering and `@fleet_pin` is read, so the key binding is the only missing piece.

**Take the slot I just left.** Eviction order is the fallback, not the first rule. When the wall is full and I bring an agent on from the overview, the pane that goes is the wall pane I was **last in** — tmux `#{pane_last}` = 1 in the main window. My focus path is almost always *wall pane → overview → `Enter`*, so the slot I just left is the slot I meant to reuse, whatever its status: an explicit `Enter` is my decision and it beats class-1 protection. The incoming pane takes that slot with `swap-pane`, so it keeps the position and size I was already looking at, nothing else on the wall moves, and the layout string doesn't change. The pane that leaves goes back to a window renamed `ag-<agent>`, as with any eviction.

Two exceptions fall back to the class order above: a **pinned** last-focused pane (class 0 is never replaced, even explicitly), and a last pane that isn't a wall pane at all — if I came from the supervisor, or focus never left the overview, there is no slot to reuse.

### 5b.2 A grid sized to the screen, not to the preset

Presets (`M-1/2/4/6/9`) stop being fixed shapes and become **caps on the number of slots**. The shape is computed from the space actually available — the wall rectangle, not the window. On the reference 300×80 terminal that rectangle is 209×64: 300×79 after the tmux status line, minus the 90-column supervisor and its border, minus the 14-row overview and its border.

- A pane should be at least `MinPaneW`×`MinPaneH` = **64×17** cells. 16 usable rows plus the one row `pane-border-status top` takes for the title. 64 columns is what an agent TUI needs before it starts wrapping badly; demanding 80 would cap the reference wall at a 2×2 and make this rule a no-op exactly where it matters.
- Given the wall rectangle and `n` panes, every row count `r` from 1 to `n` is tried. `cols = ceil(n/r)`, and the shape scores `min(cellW/MinPaneW, cellH/MinPaneH)` — not raw area, which ignores that cells are about twice as tall as they are wide and would put five panes in one row of 41 columns. **The score is clamped at 1**: once a cell clears the minimum, extra slack is worth nothing, so a shape that fits is never beaten by a roomier one with worse proportions (without the clamp, 3 panes would tile 1+2 instead of a single row of three). Shapes that divide evenly (`n % r == 0`) get a 10% bonus *after* the clamp, so 6 panes stay 3+3 instead of going ragged. Highest score wins; ties go to fewer rows, because height is the scarce axis.
- A score below 1 means nothing fits the minimum; the best-scoring shape is used anyway. A cramped pane beats a hidden agent. (Refusing the join below some hard floor is a later question — see §18 #15.)
- Panes spread over the rows as evenly as possible with the remainder on the **bottom** rows, and each row fills the full width. New panes join at the end, so the ragged row is where growth happens and the rows I am already reading keep their shape. 5 panes become **2 + 3**, 7 become 2 + 2 + 3, 3 sit in a single row.

`M-1` still means one pane — plus any pane that refused to leave, since a waiting or failing pane is never evicted (§5b.1) and the leftovers auto-grid. `M-m` still means main + stack. `M-9` no longer needs the `Wall9MinCols` special case: a narrow window simply cannot fit three 64-column panes and the grid falls back on its own.

### 5b.3 The pane that needs attention gets the space

Equal tiles are wrong when one agent is asking a question and the rest are grinding. The wall picks one **emphasis pane** and gives it `FocusPercent` = 60% of the wall; the others share the rest in an auto-grid (§5b.2):

- Emphasis pane = the **longest-waiting** pane (`waiting`, then `error`/`stuck`, by `@fleet_status_since`); if none needs attention, the **active** wall pane; if neither, no emphasis and the grid is even.
- Stacked or side by side is decided by scoring both remainders with the grid rule above, tie to stacked — on a 209×64 wall stacked gives the emphasis pane the full 209 columns and the others better shapes, while side by side leaves one cramped column.
- The emphasis pane takes the first slot. The other panes keep their relative order: reordering them to follow focus would mean `swap-pane`, and every agent TUI would repaint and lose its scroll position.
- Emphasis is skipped when it would push the remaining panes under the minimum pane size, or when there are fewer than two panes. With the default 14-row overview that means emphasis is off above about four wall panes — intended, not a bug.
- **Hysteresis**, because resizing a pane makes an agent TUI reflow: emphasis moves only when its pane leaves the wall or stops needing attention, when I explicitly focus a wall pane (immediate, then held 60s), or when a candidate has waited 10s *and* the current emphasis has held 30s. The choice lives in `@fleet_emphasis_pane` / `@fleet_emphasis_at`, and a re-layout that produces the same layout string as `@fleet_layout` is skipped entirely.
- `fleet wall emphasis` toggles it off and back on (window option `@fleet_emphasis`) for when I want a plain even grid. No key binding: the hint bar is already full, and the remaining key budget belongs to pinning.

A pane that is merely `working` therefore shrinks on its own as soon as another pane starts waiting, and grows back when I answer it.

### Out of scope here

Adaptive overview height, auto-slotting waiting agents onto the wall (§18 #13), keeping hand-dragged pane sizes across a re-layout, and saved named walls (§5a) are all still unbuilt and unchanged by this.

---

## 6. The supervisor without context overload

The supervisor is a normal Copilot session in the bottom pane. Its `AGENTS.md` tells it how to use the `fleet` CLI and the rules below.

### 6.1 State on disk

The supervisor reads `board.json`, agent cards and `decisions.log`. Anything it learns that matters goes into a card or the log before it moves on. Its conversation is a scratchpad.

### 6.2 Summaries, not raw panes

`fleet peek` runs the pane through a cheap model (Haiku, or local Ollama for anything sensitive). It returns roughly: status, what the agent is doing, what it needs, in about 5 lines. The supervisor is not allowed `--raw`. If the pane hash hasn't changed, the cached summary is reused, so polling stays cheap.

### 6.3 One-key reset (`R`)

Kill the supervisor process and start a fresh one with a boot prompt:
"Read `~/.fleet/board.json` and the last 50 lines of `decisions.log`. Report what needs me, then wait." This loses nothing because state lives on disk.

Automatic version: when the supervisor's context goes red, it writes anything pending to the board and resets itself. Built as described in §6.5.

### 6.4 Focus sessions (`f`)

For a hard problem in one agent, open a short-lived supervisor that loads only that agent's card and raw pane. It can go deep, then updates the card and exits. The main supervisor only sees the updated card.

### 6.5 Supervisor memory: notes, handoff, context meter (R11, §18 #14)

The supervisor's chat fills up over a long day. Fleet does not compact it (ADR-4). The supervisor saves what it must not forget to disk as it goes. When its context gets full it writes a handoff and fleet replaces it with a fresh session that starts from the handoff and the board. Agents aren't affected: they live on the board and in tmux, not in the supervisor's chat.

**On disk** (`~/.fleet/`)

| Path | What |
|---|---|
| `decisions.log` | Append-only notes, one line each: `2026-11-04T21:40 [agent] text`. Decisions, user preferences, promises ("tell me when bs-nav is done"). |
| `supervisor/session.json` | Current supervisor: `session_id`, `profile`, `pane_id`, `started`, `generation`, `previous` (last session ID). |
| `supervisor/handoff.md` | Latest handoff written by the outgoing supervisor (max 4 KB is shown to the next one). Earlier ones are moved to `supervisor/handoffs/<timestamp>.md`; the last 20 are kept. |

**Context meter.** For a Copilot supervisor, fill = `inputTokens / maxPromptTokens` of the last `session.usage_record` in `~/.copilot/session-state/<session_id>/events.jsonl`. Only the end of the file is read. The session ID comes from `session.json`, or from `--session-id <uuid>` in the supervisor pane's start command if the file is missing (supervisors started before this feature). No ID, a non-Copilot supervisor or no usage yet means the fill is unknown, and nothing is shown or warned.

- Supervisor pane border: `supervisor · ctx 34%`, green < 40%, yellow 40–69%, red ≥ 70% (the handoff threshold; same bands as §5). The overview's refresh loop sets the `@fleet_ctx` pane option when the value changes.
- `fleet supervisor` prints one line: short session ID, fill (`ctx 34% (68k/200k)`), generation, uptime. `--json` gives the same fields.

**MCP tools** (supervisor)

- `fleet_note(text, agent?)` appends a line to `decisions.log`.
- `fleet_brief()` returns: the handoff (if any, with its time), the last 30 lines of `decisions.log`, then the same table as `fleet_status`. A fresh supervisor calls this first.
- `fleet_handoff(summary)` writes `supervisor/handoff.md` (rotating the old one) and asks tmux to run `fleet supervisor reset --handoff` in the background after about 2 seconds. It returns at once, because the reset kills the supervisor and the MCP server with it. The summary covers things the board doesn't hold: open questions to the user, what it promised to watch, plans in flight. It doesn't restate agent statuses.
- When the fill is at or above the threshold (default 70, env `FLEET_SUP_HANDOFF_PCT`), every fleet tool result ends with one line: `[supervisor context 74%: save anything pending with fleet_note, then call fleet_handoff]`.

**Reset** (`fleet supervisor reset [--handoff]`, tmux `M-R`, overview `R`)

1. Find the `@role=sup` pane in `fleet:main`. If there is none, fail with "no supervisor pane; run fleet up".
2. If no handoff is newer than the current session's start (a manual reset), write a stub handoff: "Manual reset at <time>; previous supervisor wrote no handoff. Previous session: <id>."
3. Build the same command as `fleet up` with a new session ID, plus for Copilot `-i "<boot prompt>"`. Boot prompt: "You are taking over as fleet supervisor. Call fleet_brief, tell the user in 1–3 lines what needs them, then wait."
4. `respawn-pane -k` the supervisor pane with that command, then update `session.json` (generation + 1, `previous` = old ID). The old Copilot session isn't deleted; its ID is in the handoff, so `copilot --resume <id>` still opens it.

`M-R` and overview `R` go through tmux `confirm-before` ("reset supervisor? (y/n)"), because a reset by mistake loses the chat. `--handoff` (called from `fleet_handoff`) doesn't ask.

**Out of scope:** Copilot `/compact` or any in-place summarising; context meters for non-Copilot supervisors; deleting old supervisor sessions; agent cards (`agents/<name>.md`); focus sessions (§6.4).

---

## 7. Memory (RAM): parking

- **Cap on live agents** (default 12, configurable). Starting or waking one more parks the least recently used idle agent first.
- **Park** = ask the agent to update its card, give it a moment, stop the process and keep the tmux window with a "💤 parked" banner and the session ID.
- **Wake** = the tool's resume command in the same window. The conversation history comes back from the tool's own session store.
- **Memory pressure guard**: if available RAM falls below a threshold (default 4 GB), park idle agents until it's back above it, and flash the status bar.
- Target: 40 tiles visible, about 12 processes running, about 5 GB.

---

## 8. My additions

These came up while writing this down and weren't in the conversation.

1. **An attention queue, not just a grid.** With 40 tiles, colour alone doesn't tell me what to do next. `Tab` walks the waiting agents in order of how long they've waited, so clearing the queue means pressing Tab, then `a`, `x` or `s`, until nothing is yellow.
2. **Rules for the supervisor.** It can read anything and send ordinary prompts. It must not approve, merge, push to a shared branch, `apply`, or kill agents without me confirming in the TUI. Any action that needs approval appears as a yellow card in the supervisor pane, and I approve it there with `a`.
3. **Visible supervisor actions.** Every `fleet send` or `fleet spawn` the supervisor makes briefly flashes the target tile blue and is written to `decisions.log`. I should never wonder why an agent suddenly changed direction.
4. **Pasting, not typing.** Use `tmux load-buffer` + `paste-buffer -p` (bracketed paste) instead of `send-keys` for prompts. Multi-line prompts don't get sent half-way, and tool input boxes handle pasted text better.
5. **A worktree per agent in the same repo.** Two agents editing one checkout will collide. `fleet spawn --worktree` creates `git worktree add ../<repo>-<name>` and points the agent there.
6. **Adopting the sessions I already have.** The current terminals aren't in tmux. `fleet adopt` resumes an existing session by ID inside fleet, so I can move over one at a time instead of starting from scratch.
7. **Desktop notification when an agent goes yellow** (`notify-send`), with a minimum interval so 10 agents finishing together isn't 10 pop-ups. Optional sound.
8. **Group by repo.** Rows are repos, and `1`–`9` jumps to one. Seeing all agents working on the same API repo together makes it easier to spot two agents doing the same work.
9. **Recording status changes.** `events.jsonl` costs almost nothing now and later answers questions like "how long do agents sit waiting on me".
10. **Restart safety.** The TUI only reads state and tmux. Killing and restarting it changes nothing. Agents keep running because tmux owns them.

---

## 9. How I'll build and iterate quickly

The approach is thin vertical slices. Each slice is 1–2 hours of work, ends with something I can see and use on screen, and gets a git commit. Fake agents come first, so nothing depends on real agents, tokens or RAM until slice 5.

| Slice | What gets built | I can see/do this afterwards |
|---|---|---|
| 0 | `git init`, Go module, fake agent, `fleet-dev` tmux layout with the watcher, `make test` running | Run `make dev` and an empty TUI appears; saving a file runs the tests and restarts it |
| 1 | `fleet spawn --fake` and `fleet ls` on a separate tmux server; status read by pane scraping | 20 fake agents running; `fleet ls` shows correct status for each |
| 2 | Overview grid: coloured tiles, arrow keys, mouse click selects | 20 tiles in the right colours; arrows and clicks move the selection |
| 3 | Live wall: `Enter`/click brings a pane onto the wall; layouts 1/2/4/9; zoom | Clicking 4 tiles gives a 2×2 wall of live fake agents I can type into |
| 4 | Motion and attention: spinner, pulse, Tab queue, peek overlay, send, approve | Clearing 3 waiting fake agents takes only Tab + `a` |
| 5 | Real tools: spawn and resume for copilot and claude, then agy and vibe; capture real "waiting" patterns | A real Claude and Copilot agent on the wall with correct colours |
| 6 | Park and wake, live cap, RAM guard, `adopt` | 40 tiles, at most 12 live processes, RAM under about 6 GB |
| 7 | Supervisor pane, its `AGENTS.md`, summarising `peek`, reset key, approval cards | The supervisor reports what's waiting; `R` resets it without losing anything |
| 8 | Control mode events instead of polling, hooks, notifications, worktrees, named walls | CPU use is low with 40 agents; desktop alert when one goes yellow |

Slices 0–3 give a usable tiled wall of fake agents and are about a day of work. Slice 5 makes it useful for real work.

**Each change during a live session:**

1. I say what I want ("make waiting tiles pulse faster", "drop the RAM line").
2. The coding agent in the right-hand pane edits one feature file and its test.
3. The watcher runs that package's tests, rebuilds and restarts the TUI on the left within about 1–2 seconds, on the same tile.
4. If I like it and the tests pass, it gets a commit. If not, `git checkout .` puts back the previous version.
5. Removing a feature: turn it off in `config.toml`, and delete the file once I'm sure.

**Keeping the loop fast**

- Dev runs on the fake agents. Real agents are never restarted by the dev loop, because they run on their own tmux server (`tmux -L fleet`) and the TUI only reads from it.
- Every component ships with its test (§15). Tests run on every save, but they **don't block**: if the code builds, the TUI restarts with it, and test failures go to the log pane and `~/.fleet/logs/fleet.log`.
- Logging is the main way issues get found: every tmux error, status change, spawn/kill and caught panic is logged with the component name.
- One rule for every slice: if a slice takes longer than about 2 hours, it is split, not pushed through.

## 10. Dev loop: change it live while it runs

Goal: the TUI stays open in one pane while I talk to a coding agent in another, and each change shows up within a second or two.

```
┌──────────── tmux "fleet-dev" ────────────┐
│  fleet TUI (auto-restarting)  │  copilot │
│                               │  editing │
│                               │  the code│
├───────────────────────────────┴──────────┤
│  test output + build errors + app log    │
└──────────────────────────────────────────┘
```

- **Save → test → rebuild → restart in about 1–2 seconds.** `watchfiles` (already installed) runs `go test` for the changed package, then `go build`, then restarts the TUI. A small Go program builds in well under a second. A restart costs nothing because all state is on disk and in tmux. The cursor position is saved to `~/.fleet/ui.json` so a restart lands on the same tile.
- **Colours and sizes in a theme file.** `~/.fleet/theme.toml` is reread on change without a rebuild, so visual tweaks take effect immediately.
- **Errors don't kill the loop.** A failing test or build shows in the bottom pane, and the last working binary keeps running. The next save tries again.
- **Fake agents.** `fleet spawn --fake <name> <script>` starts a small script that prints output like a real agent and moves through working, waiting, error and idle on a timer. The grid can be built and tested with 40 tiles without spending tokens or RAM on real agents.
- **Features as small modules.** Each feature (tile animation, Tab queue, notifications, parking) is one file plus its `_test.go` file in `internal/features/`, registered in a list. `~/.fleet/config.toml` turns each one on or off, and the setting is read at startup. Adding a feature = one new file; removing it = one line in the config.
- **Git checkpoint after every working change.** If a live change breaks something, `git checkout .` gets back the last good state in seconds.
- **Screen snapshot tests from day one.** `teatest` stores the rendered grid as a golden text file. A layout change shows up as a readable diff, and `make golden` accepts it.

## 11. What still needs doing before the first line of code

| Item | Why | Default if I don't decide |
|---|---|---|
| `git init` in `ai-fleet` | Checkpoints for the live loop | Do it |
| Go module with Bubble Tea, Lip Gloss, bubblezone (mouse), teatest | One binary, few dependencies | Go 1.26, already installed |
| Fake agent script | Build the grid without real agents | First thing to build |
| One real pane capture per tool, in each state | Waiting/error patterns for status detection | Collect during step 1 |
| Name of the command | `fleet` might clash with something | `fleet` |
| Summariser model | Cost vs keeping text local | Ollama for now, Haiku as an option |

## 15. Tests per component (R16)

Each component lives in its own Go package with its own `_test.go` file. `make test` runs all of them in under about 10 seconds. Tests that need tmux use a throwaway server (`tmux -L fleet-test-<random>`) that is removed afterwards, so they never touch real agents.

| Component | Package | Test |
|---|---|---|
| tmux adapter (new window, capture, send/paste, join/swap pane, kill) | `internal/tmux` | Integration test against a throwaway tmux server: spawn `cat`, paste text, capture it back, move panes |
| Status detector | `internal/status` | Table test over real pane captures stored in `testdata/<tool>/<state>.txt`. One file per tool per state; a tool UI change shows up as a failing case |
| Board and cards store | `internal/board` | Unit test in a temp directory: write, read, concurrent update, broken JSON recovery |
| Session discovery (list and search Copilot/Claude session stores for adopt) | `internal/sessions` | Unit test over fake session folders and transcripts in a temp dir: titles, filters, word search, suggested profile |
| Tool adapters (spawn and resume command per tool, finding the session ID) | `internal/tools` | Unit test of the exact command lines; finding the session ID against a fake session folder |
| Local spawn preferences (mode, auto-submit, extra args, local profile layer) | `internal/prefs` | Unit test in a temp directory via `FLEET_CONFIG`: missing file gives neutral defaults, `[profile.x]` beats `[defaults]`, broken TOML names the file |
| Parking scheduler (cap, least recently used, RAM guard) | `internal/park` | Unit test with a fake clock and fake RAM readings |
| Supervisor memory (§6.5) | `internal/supervisor` | Fill reads the last usage record from the tail of `events.jsonl`; handoff rotation keeps 20; notes format; colour band edges; reset respawns a fake supervisor on a `fleet-test-*` socket |
| Summariser (`peek`) | `internal/summary` | Unit test with a fake model: cache hit when the pane hash is unchanged, at most 5 lines out, raw text never returned in supervisor mode |
| Approval gate (ADR-9) | `internal/gate` | Unit test: risky actions are held for approval, ordinary sends pass |
| Wall manager (layouts, pin, swap out least recently used) | `internal/wall` | Integration test on a throwaway tmux server: bring 5 agents onto a 4-slot wall and check which one went back |
| Overview TUI (tiles, colours, arrows, mouse, Tab queue) | `internal/ui` | `teatest`: send key and mouse events, compare the screen with a golden file |
| Fake agent | `cmd/fakeagent` | Unit test that its script goes through each state in order |
| Whole system | `e2e/` | Start 10 fake agents, run `fleet ls --json`, check every status; then park one, wake it and check it again |

Rules:
- Every component has a test, but a failing test doesn't block a build, a restart or a commit. Failures are logged and fixed when they matter.
- When a bug is found, the fix comes with a test that fails without it.
- Real tools (copilot, claude, agy, vibe) are tested only by an opt-in smoke test (`make smoke`) because it costs tokens. Everything else runs on fakes and recorded captures.

## 16. Keeping maintenance low (R15)

The biggest maintenance risk is not the language. It's the four agent tools changing their screens every week, which breaks status detection. The design limits this:

1. **Small on purpose.** The target is roughly 2,000–3,000 lines of Go. Anything tmux already does (layouts, mouse, scrollback, resizing) is not rebuilt.
2. **Tool-specific details in one place.** Screen patterns per tool live in `~/.fleet/patterns.toml`, not in code. When a tool changes its UI, it takes one line in that file and one new capture in `testdata/`.
3. **Hooks over scraping where the tool supports them** (Claude Code, Copilot). Hooks don't break when a tool changes how it looks.
4. **Fail soft.** An unknown screen shows a grey "unknown" tile. It never crashes the TUI or blocks other agents.
5. **`fleet doctor`** checks tmux version, each tool's version and resume flag, and runs the status detector against a fresh capture of each tool. It says exactly what broke.
6. **Few dependencies, pinned.** Only the Charm libraries (Bubble Tea, Lip Gloss, bubblezone) and the standard library.
7. **No server, no database, no daemon.** tmux, files in `~/.fleet/` and one binary. Removing it is `rm`.

## 12. Open questions

- Where agy and vibe store session IDs, so `fleet` can find them after launch.
- The exact pane patterns for "waiting" in each tool (collect real screenshots of each).
- Which Copilot and Claude hook events give "needs permission" and "turn finished" reliably.
- Whether the cheap summariser should default to Haiku or local Ollama (Ollama keeps client code on the machine; Haiku is faster).
- Default live cap (12?) and stuck timeout (10 min?).

## 13. Not in scope (for now)

- Agents talking directly to each other. All coordination goes through the board and the supervisor.
- A web UI.
- Running agents on other machines.

---

## 14. Decisions (ADRs)

Short records: what was decided, why, and what was rejected. Change a decision by adding a new ADR that replaces the old one, not by editing it.

**ADR-1: tmux underneath, not a custom terminal manager** (R1, R13)
Decision: every agent runs in a tmux pane on its own server (`tmux -L fleet`).
Why: it survives crashes of the TUI, supports mouse, resizing and layouts already, and agents keep running while the TUI restarts during dev.
Rejected: a custom pty manager (a lot of work for the same result); agent-deck (switches sessions but doesn't supervise, R14).

**ADR-2: Textual (Python) for the overview** (R3, R4, R5, R12). *Replaced by ADR-10.*
Decision: the overview, supervisor pane and status bar are a Textual app.
Why: CSS-like styling that reloads live, mouse and keyboard support built in, timers for animation, quick to change while talking.
Rejected: Go + Bubble Tea (faster but slower to iterate on and no live styling); a Zellij WASM plugin (tied to Zellij, rebuild needed for every change).

**ADR-3: live sessions drawn by tmux, summaries drawn by the fleet TUI** (R7, R8, R9)
Decision: the wall is made of real tmux panes; the overview never embeds live terminals.
Why: a terminal emulator inside the app for 9+ busy panes would be slow and a lot of code. tmux is native code and already does this well.
Rejected: a terminal widget inside the TUI for every tile.

**ADR-4: state on disk; supervisor stateless** (R2, R11)
Decision: `board.json`, cards and `decisions.log` hold all state. The supervisor can be reset at any time.
Why: a long-running supervisor fills up with every agent's details and gets worse at its job.
Rejected: one long supervisor conversation with occasional compaction.

**ADR-5: park idle agents, resume by session ID** (R9, R10)
Decision: cap live processes (default 12); park the least recently used idle agent; resume with each tool's resume flag.
Why: the measured RAM per agent makes 40 live sessions about 15–20 GB on a 30 GB machine. All four tools can resume by ID (checked 2026-11-04).
Rejected: keeping everything live and buying RAM; killing sessions and starting fresh (loses the conversation).

**ADR-6: summaries for the supervisor, never raw panes** (R11)
Decision: `fleet peek` sends pane text through a cheap model and returns about 5 lines; raw output is only for me.
Why: keeps the supervisor's context small and focused.
Rejected: giving the supervisor the last N lines of raw output.

**ADR-7: scrape panes first, events and hooks later** (R9, R13)
Decision: start by reading the bottom of each pane; move to tmux control mode events and tool hooks in slice 8.
Why: scraping works with all four tools on day one. Events and hooks are faster and more reliable but differ per tool.

**ADR-8: fake agents and live reload for development** (R12)
Decision: build slices 0–4 against scripted fake agents, with `watchfiles` running test, build and restart on every save.
Why: changes show up in 1–2 seconds, and the dev loop costs no tokens or RAM.

**ADR-9: the supervisor needs my approval for anything risky**
Decision: the supervisor can read and send ordinary prompts. Approving, merging, pushing to shared branches, applying infrastructure or killing agents needs my `a` in the TUI.
Why: a supervisor acting on 40 agents can do a lot of damage quickly.

**ADR-11: hand off to a fresh supervisor instead of compacting** (R11, R21). *Builds on ADR-4.*
Decision: the supervisor writes notes to `decisions.log` as it goes. At 70% context it writes a handoff and fleet respawns it with a new session that starts from `fleet_brief`. The fill is read from Copilot's own `events.jsonl` usage records. Warnings are added to MCP tool results, so the rule doesn't depend on the model remembering a prompt.
Why: a fresh session with a brief of under 2k tokens is cheaper and more predictable than a compacted long chat. Fleet controls what survives: the board, notes and handoff, not a model-written summary of everything. The old session can still be resumed.
Rejected: Copilot `/compact` (opaque, keeps growing, can't be triggered from outside); a supervisor summary written by another model (costs tokens; the supervisor knows best what is pending); restarting without a handoff (loses open questions to the user).

**ADR-10: Go, one static binary** (R9, R12, R15, R16). *Replaces ADR-2.*
Decision: write the CLI and the TUI in Go (Bubble Tea, Lip Gloss, bubblezone for mouse clicks) as a single `fleet` binary.
Why:
- Nothing to install or break: no Python environment, no virtualenv and no `node_modules`. Copy one file and it runs. Python version or package updates can't break it.
- Go keeps old code compiling with new compiler versions, and the few dependencies are pinned in `go.sum`. Code that works now will still build in two years.
- Fast: low memory and CPU use, and it handles tmux control-mode events well.
- Testing is built in (`go test`), including screen snapshots with `teatest`.
- The coding agents write Go well, and a small Go program builds in well under a second, so the live loop stays fast.
Cost: no live CSS reload as Textual has. The theme file and a 1–2 second rebuild cover most of that.
Rejected: Python + Textual (quicker first draft, but brings a Python environment to maintain); Rust + Ratatui (fastest, but slower to write and compile); a bash script (fine for a prototype, breaks at this size).

**ADR-12: one key registry; plain words; `Alt-?` for help**
Decision: every hotkey lives once in `internal/keys`. The tmux status hint, overview header, `Alt-?` popup, `?` overlay, `fleet keys` and the §5 keys table are generated from it, and tests fail when tmux bindings or DESIGN.md drift from it. Hints say `Alt-`, not `M-`, and name what the key does.
Why: the old hint ("M-s sup · M-S side · M-1/2/4/6/9/m wall") was cryptic, and three hand-written key lists had already drifted apart.
Rejected: `Alt-h` for help (it did not reach tmux in a live test; `Alt-?` did, and matches the overview's `?`); keeping hints hand-written next to each binding.

**ADR-13: a spawned agent starts ready to work; the mode is a local preference** (R12, R15)
Decision: `spawn` delivers the first prompt through the tool's own launch flag where one exists (`copilot -i`), and otherwise waits for a *detected-ready* pane before pasting — never a fixed sleep. Starting mode (autopilot, plan, accept-edits) is `mode_args`/`mode_keys` data on the profile, selected by a per-user file at `$XDG_CONFIG_HOME/fleet/config.toml` that the repo neither ships nor commits. Repo defaults are neutral.
Why: the fixed 1500 ms sleep lost the first prompt outright (the TUI was not in raw mode yet), and hand-switching every new agent to autopilot was the single most repeated manual step. Keeping it in a local file keeps an opinionated, blast-radius-heavy default (`--allow-all`) out of the repo while still being one line to turn on. Keeping it as profile data means Claude Code's `--permission-mode` needs no new code.
Rejected: raising the sleep (still a race, just a slower one); auto-answering the autopilot permission dialog with keystrokes (ADR-9 — and `--allow-all` at launch makes the dialog not appear at all); committing `autopilot = true` as a repo default.
**ADR-14: the wall evicts by status class, and never evicts a pane that needs me** (R4, R7)
Decision: when the wall is full, the incoming pane takes the slot of the wall pane the user was last in (`#{pane_last}`) via `swap-pane`, unless that pane is pinned or isn't a wall pane. Otherwise eviction orders panes pinned → waiting/error/stuck → working → idle/unknown and takes the least recently used pane of the worst class present. If every pane is protected, the new pane stays off the wall and tmux says so.
Why: last-use order threw away exactly the pane I had come to answer. Status is already on the pane (`@fleet_status`), so this costs one tmux field, not a board read.
Why the last-focused pane first: pressing `Enter` on a tile is an explicit choice, and the slot just left is the one the user is already looking at — replacing it in place avoids a re-tile and never surprises them by taking a different pane.
Rejected: a hard LRU with a manual pin only (needs me to pin before it helps); forcing the new pane on anyway (silently loses a waiting agent); class order even for an explicit `Enter` (it would evict some other pane while the slot the user just left sits there).

**ADR-15: the wall grid is computed from the window, presets are caps** (R7, R9)
Decision: `internal/layout` picks rows and columns so every pane keeps at least 64×17 cells of the wall rectangle and the space is filled; shapes score `min(cellW/min, cellH/min)` rather than area; `M-1/2/4/6/9` only cap how many panes may be on the wall. 5 panes become 2+3.
Why: a fixed 2×2 or 3×3 either wastes a wide monitor or crushes panes below a readable width. One rule covers every window size, and the `Wall9MinCols` special case disappears.
Rejected: more presets (more keys, same problem); tmux's own `tiled` layout (can't keep the supervisor and overview sized, ADR-3's wall must stay one computed layout string); refusing to place a pane when the grid falls below the minimum (contradicts "cramped beats hidden" in the same rule; parked as §18 #15).

**ADR-16: one emphasis pane at 60%, chosen by attention then focus** (R4, R9)
Decision: the longest-waiting pane — or the active pane when nothing waits — gets about 60% of the wall; the rest auto-grid in the remainder. Orientation is whichever remainder grids better, tie to stacked. Skipped when it would push panes under the minimum size. Moves are damped (10s dwell, 30s hold, explicit focus wins for 60s) and an unchanged layout string is never re-applied, so agent TUIs don't reflow every time a status flips. Toggled by `fleet wall emphasis`, no key binding.
Why: tiles of equal size spend the monitor evenly on work that needs me and work that doesn't; without the damping, resizing panes on every status change costs more than it gives.
Rejected: zoom only (all or nothing, and hides the others); sizing every pane by status (layout churns on every status change); an `M-e` binding (the hint bar is full; the next key belongs to pinning).

---

## 16a. Lean supervisor (R11)

The supervisor gets fleet as MCP tools (`fleet mcp`, wired in `supervisor/.mcp.json`) and nothing else. `fleet up` starts Copilot with `--no-custom-instructions --allow-all-mcp-server-instructions --available-tools fleet --allow-tool fleet`. Its rules live in `internal/mcp/instructions.md` and arrive as the MCP server instructions.

Measured on 2026-11-04 with the same one-line question: about 160k input tokens per turn with the default setup (global instructions, skills, every tool, and the fleet tools deferred behind a search, so the model fell back to bash) versus about 8–9k with the lean setup, and a direct `fleet_status` call in 4–7 seconds. The supervisor can't read Go code or run shell commands, because those tools aren't available to it.

## 17. Development process (based on Anthropic's Claude Code best practices)

Source: https://code.claude.com/docs/en/best-practices (fetched 2026-11-04).

| Guide's point | How it applies here |
|---|---|
| Tell the agent what it can't see; keep CLAUDE.md short | `AGENTS.md` holds only commands, hard rules, conventions and gotchas. `CLAUDE.md` just points to it. Anything corrected twice gets a line there. |
| Write the spec first, then build in a fresh session | This file is the spec. Builder agents start fresh and read it, instead of inheriting a long chat. |
| Give the agent a check to run | `make test` per package, the fake-agent end-to-end check, and a screen snapshot of the TUI. Agents verify by running the tool, not by saying it's done. |
| Rules that can't have exceptions belong in deterministic checks, not prompts | Socket isolation (`fleet*` only) and "never push" go into the code (socket name is a constant, no push target) and later into a hook. Preferences stay in `AGENTS.md`. |
| One agent writes, another reviews | A separate reviewer agent reads each slice's diff against this spec. The builder doesn't review its own work. |
| Run independent work in parallel with subagents | Builder (code), designer (layout), researcher (tool sessions and hooks) run at the same time; each gets a narrow brief and reports back briefly. |
| Course-correct early | Small commits every working piece; I watch `git log` and the running TUI and redirect the builder by message, not by restarting it. |

---

## 18. Review after the prototype (2026-11-04): what would make it a real agentic fleet manager

State at review: about 3,900 lines of Go. Built: tmux substrate, tool profiles, scraped status, overview grid, live wall (incl. smarter tiling, §5b: status-aware eviction, screen-sized grid, emphasis pane), lean Copilot supervisor over MCP (`fleet_status/peek/send/answer/spawn/profiles/close/wall`). Missing: park/wake, summaries, workflows, worktrees, real status patterns for Claude, agy and vibe.

The gap: fleet today manages **sessions**. Agentic work is about **tasks** (a goal, a place to do it, a definition of done, a result, dependencies). The supervisor can poke panes, but it has nothing to plan against and no way to know when work is actually finished.

### Priority 1: needed for live testing now

| # | Change | Why |
|---|---|---|
| 1 | ✅ built (2026-11-04, see §4) **`fleet adopt`**: bring an already-running session (copilot/claude ID + dir) under fleet by resuming it in a fleet pane | I already have many sessions open. Without this, live testing means starting over. |
| 2 | **`fleet snap <name> <state>`**: save the current pane to `testdata/<tool>/<state>.txt` when a tile shows the wrong colour | Turns every misdetection during live testing into a test case in seconds. Fixes the "patterns are guesses" gap with real data. |
| 3 | **Claude hooks for status** (`Stop` = turn finished, `Notification`/`PreToolUse` = needs me), writing to `~/.fleet/status/<name>.json`; scraping stays as fallback | Claude variants are most of the fleet; hooks don't break when the UI changes. |
| 4 | **Worktree per agent by default** when the dir is a git repo: `spawn` creates `git worktree add` on branch `fleet/<name>` | Agentic work runs several agents on one repo; shared checkouts collide. |

### Priority 2: tasks, not sessions

| # | Change | Why |
|---|---|---|
| 5 | **Task card on spawn**: goal, repo, **done-check** (command + expected exit code), result file. Stored in the board. | Same rule as my loop guide: no machine-checkable done condition, no loop. Lets fleet mark a tile ✅ done only when the check passes, not when the agent says so. |
| 6 | **Result file**: every spawned agent gets told to write `~/.fleet/agents/<name>/result.md` (what changed, branch, open questions) when it finishes | The supervisor reads results instead of screens: cheaper, and nothing gets lost when an agent is closed. |
| 7 | **`fleet run plan.yaml`**: a small workflow file: tasks with profile, dir, prompt, done-check and `after:` dependencies. Fleet starts tasks as their dependencies finish, up to the live cap. The supervisor can write these plans via an MCP tool (`fleet_plan`). | This is the actual agentic workflow: fan out, wait, fan in. Keeps the plan on disk (ADR-4), so a supervisor reset loses nothing. |
| 8 | **Review step as a task type**: after an implement task passes its check, fleet can spawn a reviewer agent (different profile/model) on the same worktree | Writer and reviewer separated, as in the Claude Code guide. |
| 9 | **Recipes**: named spawn presets (`review-pr`, `implement-story`, `fix-ci`) = profile + prompt template + done-check | One keypress or one supervisor call instead of re-typing prompts. |

### Priority 3: running 40 at once

| # | Change | Why |
|---|---|---|
| 10 | **Park/wake + live cap + RAM guard** (§7) | Still the RAM fix; needed beyond about 15 real agents. |
| 11 | **Spend per agent**: read tokens/credits from Copilot `events.jsonl` and Claude transcripts; show on the tile; optional per-task cap | Agentic workflows can burn credits silently. |
| 12 | **Stuck rule**: same error twice or no progress for N minutes → tile red, agent paused, supervisor told | Mirrors the "same failure twice, stop" rule; stops grinding. |
| 13 | **Waiting agents come to me**: optional auto-placement of the longest-waiting agent on the wall, plus a desktop notification | Less hunting; the wall shows what needs me. |
| 14 | **Supervisor reset key + context tile** (§6.3, §6.5). **Built 2026-11-04:** notes, handoff, context meter, `M-R`/`R` reset (ADR-11). | The lean supervisor is cheap now, but long days still fill it. |
| 15 | **A hard floor under the wall grid**: refuse to place one more pane (and say so) once the grid would drop every pane below a readable size, instead of tiling ever smaller | §5b.2 currently prefers a cramped pane to a hidden agent; past some point a 41×12 pane is just wrapped garbage. Needs a floor number from live use. |
| 16 | **Pin a pane to the wall** (`M-p` / `@fleet_pin`): eviction class 0 already exists and is read, nothing sets it | §5b.1 is built around it; it is the one key binding worth spending (§5b.3 deliberately skipped `M-e`). |

### Not recommended

- Auto-answering approval prompts. It saves clicks but removes the gate (ADR-9). At most, an allowlist per recipe, later.
- Agents talking to each other directly. Keep coordination through task cards and result files.

### Suggested order for the next builder slices

1 → 2 → 4 → 5 + 6 → 3 → 7 → 10, then the rest as live testing shows the need.
