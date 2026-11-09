#!/usr/bin/env bash
#
# Run every code check Trimloop expects before a commit:
#
#   gofmt          formatting
#   go vet         common mistakes
#   golangci-lint  linters in .golangci.yml
#   go test -race  tests, with the data-race detector
#   claude plugin validate  the marketplace and the plugin (skipped without claude)
#   govulncheck    known vulnerabilities in dependencies (needs network)
#
# Usage: scripts/check.sh [--offline] [--help]
#   --offline  skip govulncheck, which downloads the vulnerability database

set -euo pipefail

OFFLINE=0
for arg in "$@"; do
  case "$arg" in
    --offline) OFFLINE=1 ;;
    -h|--help) sed -n '3,13p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "Unknown option: $arg" >&2; exit 2 ;;
  esac
done

cd "$(dirname "$0")/.."

# Tools installed by scripts/bootstrap.sh live in Go's bin directory.
if command -v go >/dev/null 2>&1; then
  gobin="$(go env GOBIN)"
  [[ -n $gobin ]] || gobin="$(go env GOPATH)/bin"
  export PATH="$gobin:$PATH"
fi

if [[ -t 1 ]]; then
  GREEN=$'\033[32m'; RED=$'\033[31m'; BOLD=$'\033[1m'; RESET=$'\033[0m'
else
  GREEN=""; RED=""; BOLD=""; RESET=""
fi

failed=()

# check NAME COMMAND...: runs the command, records a failure, and carries on,
# so one run reports every problem.
check() {
  local name=$1; shift
  printf '%s==> %s%s\n' "$BOLD" "$name" "$RESET"
  if "$@"; then
    printf '  %s✓%s %s\n' "$GREEN" "$RESET" "$name"
  else
    printf '  %s✗%s %s\n' "$RED" "$RESET" "$name"
    failed+=("$name")
  fi
}

for tool in go golangci-lint; do
  command -v "$tool" >/dev/null 2>&1 \
    || { echo "$tool not found; run scripts/bootstrap.sh" >&2; exit 2; }
done

gofmt_clean() {
  local out
  out="$(gofmt -l .)"
  [[ -z $out ]] && return 0
  echo "  not formatted:"
  while IFS= read -r f; do printf '    %s\n' "$f"; done <<<"$out"
  return 1
}

check "gofmt" gofmt_clean
check "go vet" go vet ./...
check "golangci-lint" golangci-lint run ./...
check "go test -race" go test -race ./...
# The Claude Code plugin and marketplace (ADR 0049); CI runs it too.
if command -v claude >/dev/null 2>&1; then
  check "claude plugin validate (marketplace)" claude plugin validate . --strict
  check "claude plugin validate (plugin)" claude plugin validate plugins/trimloop --strict
else
  printf '%s==> claude plugin validate%s\n  skipped (claude not installed; CI runs it)\n' "$BOLD" "$RESET"
fi
if ((OFFLINE)); then
  printf '%s==> govulncheck%s\n  skipped (--offline)\n' "$BOLD" "$RESET"
else
  command -v govulncheck >/dev/null 2>&1 \
    || { echo "govulncheck not found; run scripts/bootstrap.sh" >&2; exit 2; }
  check "govulncheck" govulncheck ./...
fi

echo
if ((${#failed[@]})); then
  printf '%sFailed:%s %s\n' "$RED" "$RESET" "${failed[*]}"
  exit 1
fi
printf '%sAll checks passed.%s\n' "$GREEN" "$RESET"
