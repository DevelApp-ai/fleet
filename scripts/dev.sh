#!/usr/bin/env bash
# make dev: tmux "fleet-dev" on its own socket: TUI (auto-rebuild) | shell, log pane below.
set -u
cd "$(dirname "$0")/.."
ROOT=$PWD
T="tmux -L fleet-dev"
mkdir -p "$HOME/.fleet/logs"
touch "$HOME/.fleet/logs/fleet.log" "$HOME/.fleet/logs/dev.log"
if ! $T has-session -t fleet-dev 2>/dev/null; then
  $T new-session -d -s fleet-dev -n dev -x 220 -y 60 -c "$ROOT" \
    "$HOME/.local/bin/watchfiles --filter all --ignore-paths bin,.git,.dev --sigint-timeout 2 '$ROOT/scripts/dev-cycle.sh' $ROOT/cmd $ROOT/internal"
  $T split-window -d -v -l 15 -t fleet-dev:dev -c "$ROOT" \
    "tail -n 50 -F $HOME/.fleet/logs/dev.log $HOME/.fleet/logs/fleet.log"
  $T split-window -d -h -l 70 -t fleet-dev:dev.0 -c "$ROOT"
  $T set-option -t fleet-dev mouse on
fi
if [ -t 0 ]; then
  exec env -u TMUX $T attach -t fleet-dev
fi
echo "fleet-dev running: tmux -L fleet-dev attach -t fleet-dev"
