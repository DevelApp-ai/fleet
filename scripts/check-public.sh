#!/usr/bin/env bash
# Check commits before they leave this machine.
# Usage: scripts/check-public.sh <rev-list args>   e.g.  origin/main..HEAD   or   HEAD
#
# Fails when a commit in the range:
#   - has an author or committer email not listed in .git/info/public-emails
#   - leaves a tree containing a home path, a pattern from .git/info/public-deny,
#     or an email address that is not allowed
#   - is flagged by gitleaks (if installed)
# Both files live in the git dir, so they are never committed. One pattern or email per line.
set -euo pipefail

[[ $# -gt 0 ]] || { echo "usage: $0 <rev-list args>" >&2; exit 2; }
info="$(git rev-parse --git-common-dir)/info"
emails_file="$info/public-emails"
deny_file="$info/public-deny"
fail=0
err() { echo "check-public: $*" >&2; fail=1; }

[[ -s $emails_file ]] || { echo "check-public: missing $emails_file (allowed commit emails, one per line)" >&2; exit 1; }
mapfile -t allowed < <(grep -v -e '^#' -e '^$' "$emails_file")
ok_args=()
for a in "${allowed[@]}"; do ok_args+=(-e "$a"); done
deny='/home/[a-z]'
if [[ -s $deny_file ]]; then
  while IFS= read -r p; do [[ -z $p || $p == \#* ]] || deny+="|$p"; done <"$deny_file"
fi

commits=$(git rev-list "$@")
[[ -n $commits ]] || exit 0

for c in $commits; do
  for e in $(git log -1 --format='%ae %ce' "$c"); do
    printf '%s\n' "${allowed[@]}" | grep -qxF -- "$e" || err "$(git log -1 --format=%h "$c"): email $e is not in public-emails"
  done
done

tip=$(head -1 <<<"$commits")
if git grep -n -I -i -E "$deny" "$tip" -- . ':!scripts/check-public.sh' >&2; then
  err "personal or denied strings in $(git log -1 --format=%h "$tip") (above)"
fi
if git grep -I -h -o -E '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}' "$tip" \
  | sort -u | grep -v -F -e noreply -e example. -e git@github.com "${ok_args[@]}" >&2; then
  err "unexpected email addresses in tree (above)"
fi

gl=$(command -v gitleaks || true)
[[ -n $gl || ! -x $HOME/go/bin/gitleaks ]] || gl=$HOME/go/bin/gitleaks
if [[ -n $gl ]]; then
  "$gl" git --no-banner --log-level error --log-opts="$*" . || err "gitleaks found something"
else
  echo "check-public: gitleaks not installed, secret scan skipped" >&2
fi

exit $fail
