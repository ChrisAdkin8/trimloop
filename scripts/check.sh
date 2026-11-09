#!/usr/bin/env bash
#
# Run every code check Trimloop expects before a commit:
#
#   gofmt          formatting
#   go vet         common mistakes
#   golangci-lint  linters in .golangci.yml
#   go test -race  tests, with the data-race detector
#   golden drift   testdata/golden is what its scripts make (skipped without
#                  python3, except in CI, where it fails)
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
    -h|--help) sed -n '3,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
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

# The end-to-end test's logs and expected answers must be what
# scripts/gen-golden.py and scripts/recount.py make today: regenerate both
# into a scratch folder and compare every file git tracks under
# testdata/golden, and every file the scripts make, but the READMEs
# (testdata/golden/README.md). So a tracked file the scripts no longer make,
# or a made file missing from git, fails, but untracked junk such as
# .DS_Store does not.
# It runs in a subshell, so its EXIT trap removes the scratch folder however
# it ends.
golden_fresh() (
  local tmp status=0
  local hint="  regenerate testdata/golden with its scripts (testdata/golden/README.md, \"Regenerating\")"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  if ! python3 -I scripts/gen-golden.py --out "$tmp" >/dev/null; then
    echo "  scripts/gen-golden.py failed"
    echo "$hint"
    return 1
  fi
  if ! python3 -I scripts/recount.py "$tmp" --json "$tmp/expected.json"; then
    echo "  scripts/recount.py failed"
    echo "$hint"
    return 1
  fi
  local tracked made f differ=()
  if ! tracked="$(git ls-files -- testdata/golden)"; then
    echo "  git ls-files failed: the golden check needs a git checkout"
    return 1
  fi
  tracked="$(sed 's|^testdata/golden/||' <<<"$tracked" | not_readme | LC_ALL=C sort)"
  made="$(cd "$tmp" && find . -type f | sed 's|^\./||' | not_readme | LC_ALL=C sort)"
  while IFS= read -r f; do
    if [[ -n $f ]]; then
      differ+=("Only in testdata/golden (tracked, no longer made): $f")
    fi
  done < <(LC_ALL=C comm -23 <(printf '%s\n' "$tracked") <(printf '%s\n' "$made"))
  while IFS= read -r f; do
    if [[ -n $f ]]; then
      differ+=("Only in what the scripts make (not tracked by git): $f")
    fi
  done < <(LC_ALL=C comm -13 <(printf '%s\n' "$tracked") <(printf '%s\n' "$made"))
  while IFS= read -r f; do
    if [[ -n $f ]] && ! cmp -s "testdata/golden/$f" "$tmp/$f"; then
      differ+=("Files testdata/golden/$f and the made copy differ")
    fi
  done < <(LC_ALL=C comm -12 <(printf '%s\n' "$tracked") <(printf '%s\n' "$made"))
  if ((${#differ[@]})); then
    printf '  %s\n' "${differ[@]}"
    echo "  testdata/golden differs from what its scripts make"
    echo "$hint"
    status=1
  fi
  return "$status"
)

# not_readme drops README.md files from a list of paths, one per line.
not_readme() { grep -v -e '^README\.md$' -e '/README\.md$' || true; }

if command -v python3 >/dev/null 2>&1; then
  check "golden drift" golden_fresh
elif [[ -n ${CI:-} ]]; then
  # Without this, a runner image that dropped Python would drop the guard
  # silently.
  printf '%s==> golden drift%s\n' "$BOLD" "$RESET"
  printf '  %s✗%s golden drift: python3 not found, and CI needs it for this step (install Python in the workflow)\n' "$RED" "$RESET"
  failed+=("golden drift")
else
  printf '%s==> golden drift%s\n  skipped (python3 not installed; CI runs it)\n' "$BOLD" "$RESET"
fi
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
