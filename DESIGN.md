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
| `fleet adopt <tool> <session-id> <dir>` | Bring an existing session (started outside fleet) under management by resuming it in tmux |

### Status detection

Status is read from the bottom of the pane each poll (every 1–2 seconds):

| Status | Signal |
|---|---|
| working | pane text changed since the last poll, or the tool's spinner or "thinking" marker is visible |
| waiting | approval prompt, y/n prompt or an empty input box after a finished turn (one pattern per tool) |
| error | error banner, crashed process or non-zero exit |
| stuck | working, but the pane hash hasn't changed for N minutes (default 10) |
| idle | turn finished and nothing new asked for, longer than M minutes |
| parked | no process; resumes on demand |

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
| supervisor | own tile | context fill: green < 40%, yellow 40–70%, red > 70% |

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
| `R` | Reset the supervisor (see 6.3) |
| `/` | Filter tiles by name, repo or status |
| `1`–`9` | Jump to a repo group |
| `?` | Help overlay |

### Hotkey hints and help (built)

The bottom tmux status line and the overview header show a short hint in plain words, written with `Alt-` instead of tmux's `M-`. The full list is one keypress away:

- **`Alt-?`** anywhere in the fleet tmux server opens a popup (`tmux display-popup`) with every hotkey, grouped by area (panes, wall, supervisor, overview), one line each. Any key closes it. `fleet keys` prints the same list in a terminal.
- **`?`** in the overview opens the same list as an overlay; `q`, `Esc` or `?` closes it.
- One registry (`internal/keys`) is the single source of truth: the status hint, the overview hint, the popup, the overlay, `fleet keys` and the generated table below come from it. Tests fail if a tmux `M-*` binding in `fleet up` is missing from the registry (or the reverse), or if the table below is out of date (`fleet keys --markdown` regenerates it).

<!-- keys:begin -->
| Group | Key | Description |
|---|---|---|
| Panes | `Alt-←↑↓→` | move to the pane in that direction |
| Panes | `Alt-z` | zoom this pane; press again to restore |
| Panes | `Alt-x` | hide this pane (the agent keeps running) |
| Panes | `Alt-o` | jump to the marked pane (right-click → Mark) |
| Wall | `Alt-1/2/4/6/9` | wall: show up to N panes |
| Wall | `Alt-m` | wall: one main pane + stack |
| Supervisor | `Alt-s` | show/hide the supervisor |
| Supervisor | `Alt-S` | move the supervisor left/right |
| Overview | `Alt-c` | overview: tiles ↔ compact chips |
| Overview | `←↑↓→ / hjkl` | select a tile |
| Overview | `⏎ / click` | show the selected agent on the wall |
| Overview | `x` | take the selected agent off the wall |
| Overview | `Tab` | jump to the next waiting agent |
| Overview | `1/2/4/6/9/m` | same as Alt-1…9 / Alt-m |
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
- **Tiling layouts**, chosen from the keyboard or the status bar: `1` one large pane; `2` side by side; `4` a 2×2; `6` a 3×2; `9` a 3×3; `M` main + stack (one large pane plus a column of small ones). Built from tmux's own layouts (`main-vertical`, `tiled`) plus saved custom layout strings.
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

## 6. The supervisor without context overload

The supervisor is a normal Copilot session in the bottom pane. Its `AGENTS.md` tells it how to use the `fleet` CLI and the rules below.

### 6.1 State on disk

The supervisor reads `board.json`, agent cards and `decisions.log`. Anything it learns that matters goes into a card or the log before it moves on. Its conversation is a scratchpad.

### 6.2 Summaries, not raw panes

`fleet peek` runs the pane through a cheap model (Haiku, or local Ollama for anything sensitive). It returns roughly: status, what the agent is doing, what it needs, in about 5 lines. The supervisor is not allowed `--raw`. If the pane hash hasn't changed, the cached summary is reused, so polling stays cheap.

### 6.3 One-key reset (`R`)

Kill the supervisor process and start a fresh one with a boot prompt:
"Read `~/.fleet/board.json` and the last 50 lines of `decisions.log`. Report what needs me, then wait." This loses nothing because state lives on disk.

Automatic version: when the supervisor's context goes red, it writes anything pending to the board and resets itself.

### 6.4 Focus sessions (`f`)

For a hard problem in one agent, open a short-lived supervisor that loads only that agent's card and raw pane. It can go deep, then updates the card and exits. The main supervisor only sees the updated card.

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
| Tool adapters (spawn and resume command per tool, finding the session ID) | `internal/tools` | Unit test of the exact command lines; finding the session ID against a fake session folder |
| Parking scheduler (cap, least recently used, RAM guard) | `internal/park` | Unit test with a fake clock and fake RAM readings |
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
