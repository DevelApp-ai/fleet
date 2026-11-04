#!/usr/bin/env bash
# One dev cycle: build (keep last good binary on failure), run tests in the
# background (non-blocking, results to dev.log), then run the TUI.
cd "$(dirname "$0")/.."
LOG="$HOME/.fleet/logs/dev.log"
mkdir -p bin .dev
ts() { date +%H:%M:%S; }
if go build -o bin/fleet.tmp ./cmd/fleet 2>.dev/build.err; then
  mv bin/fleet.tmp bin/fleet
  echo "$(ts) build ok" >>"$LOG"
else
  echo "$(ts) BUILD FAILED (running last good binary):" >>"$LOG"
  cat .dev/build.err >>"$LOG"
fi
(
  if out=$(go test ./... 2>&1); then
    echo "$(ts) tests ok" >>"$LOG"
  else
    echo "$(ts) TESTS FAILED:" >>"$LOG"
    echo "$out" | grep -v '^ok ' >>"$LOG"
  fi
) </dev/null &
exec ./bin/fleet ui
