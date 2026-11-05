#!/usr/bin/env bash
# Regenerate docs/img/hero.png and docs/img/demo.gif from fake agents.
# Runs a separate tmux server (own TMUX_TMPDIR and FLEET_HOME), so a running
# fleet is never touched. The admiral pane runs the fake profile: no tokens.
# Needs: tmux, python3 with Pillow + fontTools, ImageMagick (magick).
set -eu
cd "$(dirname "$0")/.."
ROOT=$PWD
make build >/dev/null
D=$(mktemp -d)
mkdir -m 700 "$D/tmux"
export TMUX_TMPDIR="$D/tmux" FLEET_HOME="$D/home" FLEET_ADMIRAL_PROFILE=fake
unset TMUX
B=$ROOT/bin/fleet
cleanup() {
  tmux -S "$D/tmux/tmux-$(id -u)/fleet-test-shot" kill-server 2>/dev/null || true
  tmux -S "$D/tmux/tmux-$(id -u)/fleet" kill-server 2>/dev/null || true
  rm -r "$D"
}
trap cleanup EXIT
"$B" up --no-attach
for n in api-auth web-nav infra-dns db-migrate ci-cache docs search-idx billing-fix; do
  "$B" spawn --fake "$n" /tmp >/dev/null
done
sleep 2
for n in web-nav api-auth infra-dns ci-cache; do "$B" wall show "$n"; done
tmux -L fleet-test-shot new-session -d -x 200 -y 52 -s shot \
  "env TMUX_TMPDIR=$D/tmux TERM=xterm-256color tmux -L fleet attach -t fleet:main"
sleep 20
for n in ci-cache docs db-migrate; do "$B" send "$n" y; done
sleep 3
mkdir -p docs/img "$D/frames"
tmux -L fleet-test-shot capture-pane -e -p -t shot > "$D/hero.ans"
python3 scripts/ansi2png.py "$D/hero.ans" docs/img/hero.png 14
for i in $(seq -w 1 24); do
  tmux -L fleet-test-shot capture-pane -e -p -t shot > "$D/frames/$i.ans"
  [ "$i" = 08 ] && "$B" send web-nav y
  [ "$i" = 14 ] && "$B" send infra-dns y
  sleep 0.5
done
for f in "$D"/frames/*.ans; do python3 scripts/ansi2png.py "$f" "${f%.ans}.png" 11; done
magick -delay 50 -loop 0 "$D"/frames/*.png -layers Optimize -colors 64 docs/img/demo.gif
echo "wrote docs/img/hero.png docs/img/demo.gif"
