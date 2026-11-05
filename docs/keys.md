# Keys

fleet has two sets of keys: the overview TUI's own keys, and tmux bindings that `fleet up` sets on the `fleet` tmux server. The bindings are set with `bind-key -n` on that server only, so your own tmux setup is not touched.

You don't need to remember them: `Alt+?` opens a popup with every key (any key closes it), `?` in the overview shows the same list, and `fleet keys` prints it. All of these, the status-line hint and the table in [DESIGN.md §5](../DESIGN.md#hotkey-hints-and-help-built) come from one registry, [`internal/keys`](../internal/keys/keys.go).

## Overview TUI

Active when the overview pane has focus (click it, or `Alt+arrows`).

| Key | Action |
|---|---|
| `←` `→` / `h` `l` | Previous / next tile |
| `↑` `↓` / `k` `j` | Tile one row up / down |
| `Tab` | Next waiting agent. Waiting agents sort first, longest wait first. |
| `Enter` | Put the selected agent on the wall and focus its pane |
| Left click on a tile or chip | Select it and put it on the wall |
| `x` | Take the selected agent off the wall |
| `1` `2` `4` `6` `9` `m` | Wall layout (see below) |
| `R` | Reset the supervisor (y/n first) |
| `?` | Show all keys; `q`, `Esc` or `?` closes the list (`q` doesn't quit while it is open) |
| `q`, `Ctrl+C` | Quit the overview. Run `fleet up` again to restart it. |

The wall only works when the overview runs inside the fleet tmux server (`fleet up`). Run on its own (`fleet ui`), the overview shows tiles but `Enter` prints a hint instead.

Tile order: waiting, error, stuck, working, idle, then the rest; within waiting, the longest wait first. A `▣` after the name means the agent is on the wall. The selected tile has a thick blue border. The cursor position is saved in `~/.fleet/ui.json`, so a restart keeps the same tile selected.

## tmux bindings (no prefix)

| Key | Action |
|---|---|
| `Alt+←` `Alt+↑` `Alt+↓` `Alt+→` | Move focus between panes |
| `Alt+z` | Zoom the focused pane to the whole window; again to unzoom |
| `Alt+x` | Take the focused wall pane off the wall (no-op on other panes) |
| `Alt+1` `Alt+2` `Alt+4` `Alt+6` `Alt+9` `Alt+m` | Wall layout |
| `Alt+s` | Hide or show the supervisor pane |
| `Alt+Shift+s` | Move the supervisor to the other side |
| `Alt+Shift+r` | Reset the supervisor: replace it with a fresh session, after a y/n prompt |
| `Alt+c` | Overview: tiles, or one line of chips |
| `Alt+o` | Jump to the marked pane (`prefix m` marks one) |
| `Alt+?` | Popup with every key; any key closes it |

Mouse is on: click a pane to type in it, drag a border to resize, scroll to read history.

## Wall layouts

A layout is a cap on how many panes the wall shows, not a fixed shape. Within that cap the grid is sized to the window: fleet picks the rows and columns that fill the space while keeping every pane at least about 64×17, so five panes become 3+2 rather than a 2×2 with one left over, and a narrow window gets fewer, larger panes.

| Layout | Panes |
|---|---|
| `1` | One large pane |
| `2` | Up to two |
| `4` | Up to four |
| `6` | Up to six |
| `9` | Up to nine, as many as fit at a readable size |
| `m` | Main + stack: one large pane (60% of the wall width) and up to three small ones |

One pane gets the large slot: the pane you just focused, or else the one that has been waiting longest. A pane that is only working shrinks. The wall follows your focus on its own; `fleet wall emphasis` turns the large slot off and back on when you want a plain even grid instead.

When the wall is full, putting another agent on it replaces the wall pane you were last in, in the same slot, so nothing else moves. If that pane is pinned or you were not in a wall pane, fleet evicts by status instead: idle panes first, then working ones, least recently used first. Waiting and failing panes are never evicted that way. Choosing a smaller layout sends the panes that no longer fit back to their hidden windows. The agent process keeps running either way.

## Planned

From [DESIGN.md §5](../DESIGN.md#5-the-tui); not built yet:

| Key | Planned action |
|---|---|
| `Space` | Peek overlay with a short summary (`r` for raw) |
| `s` | Send a one-line prompt to the selected agent |
| `a` / `x` | Approve / reject at a y/n prompt |
| `n` | Spawn wizard |
| `z` / `w` | Park / wake |
| `f` | Focus session on one agent |
| `/` | Filter tiles |
| `P` | Pin a wall pane |
