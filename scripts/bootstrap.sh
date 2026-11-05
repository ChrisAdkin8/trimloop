#!/usr/bin/env bash
#
# Install the SDKs and tools needed to develop Trimloop.
#
#   Go          core: analyzer, sidecar, proxy, CLI         (ADR 0002)
#   C toolchain cgo, needed by the DuckDB Go driver         (ADR 0003)
#   buf + protoc-gen-go(-grpc)  wire schema in proto/       (ADR 0001)
#   golangci-lint, govulncheck  Go linting and vuln checks
#   uv + Python LiteLLM plugin                              (ADR 0005)
#   Node.js     web UI (TypeScript)
#   DuckDB CLI  writing and testing SQL detectors
#   Docker      optional: local ClickHouse/Postgres for the team collector
#
# Supported: macOS (Homebrew) and Debian/Ubuntu (apt).
# Safe to re-run: anything already present at the minimum version is skipped.
#
# Usage: scripts/bootstrap.sh [--check] [--yes] [--skip-node] [--skip-python]
#                             [--with-collector] [--help]

set -euo pipefail

ORIG_PATH="$PATH"
GO_MIN="1.25"
PYTHON_VERSION="3.12"   # pinned for the LiteLLM plugin
NODE_MIN="22"

CHECK_ONLY=0
ASSUME_YES=0
SKIP_NODE=0
SKIP_PYTHON=0
WITH_COLLECTOR=0

usage() {
  sed -n '3,19p' "$0" | sed 's/^# \{0,1\}//'
  cat <<EOF
Options:
  --check           Report what is installed or missing; change nothing.
                    Exits 1 if anything is missing (usable as a CI gate).
  --yes             Do not prompt before installing.
  --skip-node       Skip Node.js (web UI).
  --skip-python     Skip uv and Python (LiteLLM plugin).
  --with-collector  Also check Docker for the team-collector stack.
  -h, --help        Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check) CHECK_ONLY=1 ;;
    --yes|-y) ASSUME_YES=1 ;;
    --skip-node) SKIP_NODE=1 ;;
    --skip-python) SKIP_PYTHON=1 ;;
    --with-collector) WITH_COLLECTOR=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

# ---------- output helpers ----------

if [[ -t 1 ]]; then
  BOLD=$'\033[1m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; RED=$'\033[31m'; RESET=$'\033[0m'
else
  BOLD=""; GREEN=""; YELLOW=""; RED=""; RESET=""
fi
step() { printf '\n%s==> %s%s\n' "$BOLD" "$1" "$RESET"; }
ok()   { printf '  %s✓%s %s\n' "$GREEN" "$RESET" "$1"; }
todo() { printf '  %s•%s %s\n' "$YELLOW" "$RESET" "$1"; }
fail() { printf '  %s✗%s %s\n' "$RED" "$RESET" "$1" >&2; }
die()  { fail "$1"; exit 1; }

MISSING=()
note_missing() { MISSING+=("$1"); todo "$1: missing or too old"; }

# version_ge A B  ->  true if version A >= version B (numeric, dot-separated)
version_ge() {
  local -a a b
  IFS=. read -r -a a <<<"$1"
  IFS=. read -r -a b <<<"$2"
  local i
  for ((i = 0; i < ${#b[@]}; i++)); do
    local x=${a[i]:-0} y=${b[i]:-0}
    x=${x%%[^0-9]*}; y=${y%%[^0-9]*}
    ((10#${x:-0} > 10#${y:-0})) && return 0
    ((10#${x:-0} < 10#${y:-0})) && return 1
  done
  return 0
}

have() { command -v "$1" >/dev/null 2>&1; }

confirm() {
  ((ASSUME_YES)) && return 0
  local reply
  read -r -p "$1 [y/N] " reply
  [[ $reply == [yY]* ]]
}

# Run a command, or just describe it in --check mode.
run() {
  if ((CHECK_ONLY)); then
    printf '    would run: %s\n' "$*"
  else
    "$@"
  fi
}

# ---------- platform ----------

OS="$(uname -s)"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  arm64|aarch64) GOARCH=arm64 ;;
  *) die "Unsupported CPU architecture: $ARCH" ;;
esac

case "$OS" in
  Darwin)
    PLATFORM=macos
    have brew || die "Homebrew is required on macOS. Install it from https://brew.sh, then re-run."
    ;;
  Linux)
    have apt-get || die "Only Debian/Ubuntu (apt) is supported on Linux. Install manually: Go >= $GO_MIN, a C toolchain, uv, Node.js >= $NODE_MIN."
    PLATFORM=linux
    SUDO=""; [[ $EUID -ne 0 ]] && SUDO="sudo"
    ;;
  *) die "Unsupported OS: $OS (macOS and Debian/Ubuntu only)." ;;
esac

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

printf '%sTrimloop development setup%s (%s, %s)%s\n' "$BOLD" "$RESET" "$PLATFORM" "$GOARCH" \
  "$( ((CHECK_ONLY)) && echo ' — check only, no changes' )"

if ((!CHECK_ONLY)) && ! confirm "Install missing SDKs and tools now?"; then
  echo "Nothing changed. Re-run with --check to see what is missing."
  exit 0
fi

# ---------- base packages / C toolchain ----------

step "C toolchain and base tools (cgo for DuckDB)"
if [[ $PLATFORM == macos ]]; then
  if xcode-select -p >/dev/null 2>&1; then
    ok "Xcode Command Line Tools ($(xcode-select -p))"
  else
    note_missing "Xcode Command Line Tools"
    run xcode-select --install
    ((CHECK_ONLY)) || die "Finish the Command Line Tools installer that just opened, then re-run this script."
  fi
  for t in git curl jq; do
    if have "$t"; then ok "$t"; else note_missing "$t"; run brew install "$t"; fi
  done
else
  pkgs=(build-essential pkg-config git curl ca-certificates unzip jq python3)
  missing_pkgs=()
  for p in "${pkgs[@]}"; do
    dpkg -s "$p" >/dev/null 2>&1 || missing_pkgs+=("$p")
  done
  if ((${#missing_pkgs[@]} == 0)); then
    ok "apt packages: ${pkgs[*]}"
  else
    note_missing "apt packages: ${missing_pkgs[*]}"
    run $SUDO apt-get update
    run $SUDO apt-get install -y "${missing_pkgs[@]}"
  fi
fi

# ---------- Go ----------

step "Go >= $GO_MIN"
[[ -x /usr/local/go/bin/go ]] && export PATH="/usr/local/go/bin:$PATH"
go_current=""
have go && go_current="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
if [[ -n $go_current ]] && version_ge "$go_current" "$GO_MIN"; then
  ok "Go $go_current"
else
  note_missing "Go"
  if [[ $PLATFORM == macos ]]; then
    run brew install go
  else
    # Latest stable release from go.dev, verified against its published SHA-256.
    if ((CHECK_ONLY)); then
      echo "    would download the latest stable Go for linux-$GOARCH from go.dev and install to /usr/local/go"
    else
      go_meta="$(curl -fsSL 'https://go.dev/dl/?mode=json' | python3 -c '
import json, sys
arch = sys.argv[1]
rel = next(r for r in json.load(sys.stdin) if r["stable"])
f = next(f for f in rel["files"] if f["os"] == "linux" and f["arch"] == arch and f["kind"] == "archive")
print(f["filename"], f["sha256"])' "$GOARCH")" || die "Could not look up the latest Go release on go.dev."
      read -r go_file go_sha <<<"$go_meta"
      [[ -n ${go_file:-} && -n ${go_sha:-} ]] || die "Unexpected response from go.dev; install Go >= $GO_MIN manually."
      curl -fsSL -o "$TMP/$go_file" "https://go.dev/dl/$go_file"
      echo "$go_sha  $TMP/$go_file" | sha256sum -c - >/dev/null || die "Go download failed checksum verification."
      $SUDO rm -rf /usr/local/go
      $SUDO tar -C /usr/local -xzf "$TMP/$go_file"
      export PATH="/usr/local/go/bin:$PATH"
      ok "Installed $(go version)"
    fi
  fi
fi

# ---------- Go tools ----------

step "Go tools (buf, protoc plugins, linters)"
GOBIN_DIR=""
have go && GOBIN_DIR="$(go env GOPATH)/bin" && export PATH="$GOBIN_DIR:$PATH"
go_tools=(
  "buf|github.com/bufbuild/buf/cmd/buf@latest"
  "protoc-gen-go|google.golang.org/protobuf/cmd/protoc-gen-go@latest"
  "protoc-gen-go-grpc|google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
  "golangci-lint|github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"
  "govulncheck|golang.org/x/vuln/cmd/govulncheck@latest"
)
for entry in "${go_tools[@]}"; do
  bin="${entry%%|*}"; pkg="${entry#*|}"
  if have "$bin"; then
    ok "$bin"
  else
    note_missing "$bin"
    if have go; then run go install "$pkg"; else todo "  (needs Go first) go install $pkg"; fi
  fi
done

# ---------- Python (LiteLLM plugin) ----------

if ((!SKIP_PYTHON)); then
  step "uv and Python $PYTHON_VERSION (LiteLLM plugin)"
  [[ -x "$HOME/.local/bin/uv" ]] && export PATH="$HOME/.local/bin:$PATH"
  if have uv; then
    ok "uv $(uv --version | awk '{print $2}')"
  else
    note_missing "uv"
    if [[ $PLATFORM == macos ]]; then
      run brew install uv
    elif ((CHECK_ONLY)); then
      echo "    would download the latest uv release from GitHub, verify its SHA-256 and install to ~/.local/bin"
    else
      # Release archive plus its published .sha256; no installer script, no shell rc edits.
      uv_triple=$([[ $GOARCH == amd64 ]] && echo x86_64-unknown-linux-gnu || echo aarch64-unknown-linux-gnu)
      uv_file="uv-${uv_triple}.tar.gz"
      uv_url="https://github.com/astral-sh/uv/releases/latest/download/${uv_file}"
      curl -fsSL -o "$TMP/$uv_file" "$uv_url" || die "Could not download $uv_file."
      curl -fsSL -o "$TMP/$uv_file.sha256" "$uv_url.sha256" || die "Could not download the checksum for $uv_file."
      [[ "$(awk '{print $1}' "$TMP/$uv_file.sha256")" == "$(sha256sum "$TMP/$uv_file" | awk '{print $1}')" ]] \
        || die "uv download failed checksum verification."
      tar -C "$TMP" -xzf "$TMP/$uv_file"
      mkdir -p "$HOME/.local/bin"
      install -m 0755 "$TMP/uv-${uv_triple}/uv" "$TMP/uv-${uv_triple}/uvx" "$HOME/.local/bin/"
      export PATH="$HOME/.local/bin:$PATH"
      ok "Installed uv $(uv --version | awk '{print $2}')"
    fi
  fi
  if have uv && uv python find "$PYTHON_VERSION" >/dev/null 2>&1; then
    ok "Python $PYTHON_VERSION (managed by uv)"
  else
    note_missing "Python $PYTHON_VERSION"
    run uv python install "$PYTHON_VERSION"
  fi
fi

# ---------- Node.js (web UI) ----------

if ((!SKIP_NODE)); then
  step "Node.js >= $NODE_MIN (web UI)"
  node_current=""
  have node && node_current="$(node --version | sed 's/^v//')"
  if [[ -n $node_current ]] && version_ge "$node_current" "$NODE_MIN"; then
    ok "Node.js $node_current"
  else
    note_missing "Node.js"
    if [[ $PLATFORM == macos ]]; then
      run brew install node
    elif ((CHECK_ONLY)); then
      echo "    would download the latest Node.js LTS for linux-$GOARCH from nodejs.org into /usr/local/lib/nodejs"
    else
      # Latest LTS from nodejs.org, verified against SHASUMS256.txt.
      node_ver="$(curl -fsSL https://nodejs.org/dist/index.json | python3 -c '
import json, sys
print(next(r["version"] for r in json.load(sys.stdin) if r["lts"]))')" || die "Could not look up the latest Node.js LTS."
      node_arch=$([[ $GOARCH == amd64 ]] && echo x64 || echo arm64)
      node_file="node-${node_ver}-linux-${node_arch}.tar.xz"
      curl -fsSL -o "$TMP/$node_file" "https://nodejs.org/dist/${node_ver}/${node_file}"
      curl -fsSL -o "$TMP/SHASUMS256.txt" "https://nodejs.org/dist/${node_ver}/SHASUMS256.txt"
      (cd "$TMP" && grep " ${node_file}\$" SHASUMS256.txt | sha256sum -c - >/dev/null) \
        || die "Node.js download failed checksum verification."
      # Unpack into its own directory and link the commands, rather than
      # scattering the tarball's files across /usr/local.
      node_dir="/usr/local/lib/nodejs/${node_file%.tar.xz}"
      $SUDO mkdir -p /usr/local/lib/nodejs
      $SUDO rm -rf "$node_dir"
      $SUDO tar -C /usr/local/lib/nodejs -xJf "$TMP/$node_file"
      for b in "$node_dir"/bin/*; do
        $SUDO ln -sf "$b" "/usr/local/bin/$(basename "$b")"
      done
      ok "Installed Node.js $(node --version)"
    fi
  fi
  # The web UI's package manager is not chosen yet; corepack is only a convenience.
  if have corepack; then
    if ((CHECK_ONLY)); then
      ok "corepack available"
    elif corepack enable >/dev/null 2>&1; then
      ok "corepack enabled"
    else
      todo "corepack enable failed (it may need sudo); only needed for pnpm or yarn"
    fi
  elif have node; then
    todo "corepack is not bundled with this Node.js (25+); install pnpm separately if the web UI needs it"
  fi
fi

# ---------- DuckDB CLI ----------

step "DuckDB CLI (aggregate SQL development)"
if have duckdb; then
  ok "duckdb $(duckdb --version 2>/dev/null | awk '{print $1}')"
else
  note_missing "duckdb CLI"
  if [[ $PLATFORM == macos ]]; then
    run brew install duckdb
  elif ((CHECK_ONLY)); then
    echo "    would download the latest DuckDB CLI release from GitHub to /usr/local/bin"
  else
    # Only assets for this machine's architecture: amd64 never falls back to ARM.
    # No checksum file is used; the binary is run once after install to prove it works here.
    if [[ $GOARCH == amd64 ]]; then duck_assets=(duckdb_cli-linux-amd64.zip)
    else duck_assets=(duckdb_cli-linux-arm64.zip duckdb_cli-linux-aarch64.zip); fi
    installed=0
    for asset in "${duck_assets[@]}"; do
      if curl -fsSL -o "$TMP/duckdb.zip" "https://github.com/duckdb/duckdb/releases/latest/download/${asset}" \
         && unzip -o -q "$TMP/duckdb.zip" -d "$TMP" && "$TMP/duckdb" --version >/dev/null 2>&1; then
        $SUDO install -m 0755 "$TMP/duckdb" /usr/local/bin/duckdb
        installed=1; break
      fi
    done
    if ((installed)); then
      ok "Installed duckdb $(duckdb --version 2>/dev/null | awk '{print $1}')"
    else
      todo "Could not download the DuckDB CLI; install it manually (optional — the Go driver bundles DuckDB)."
    fi
  fi
fi

# ---------- Docker (optional) ----------

if ((WITH_COLLECTOR)); then
  step "Docker (team collector: ClickHouse, Postgres)"
  if have docker && docker info >/dev/null 2>&1; then
    ok "Docker is running ($(docker --version | awk '{print $3}' | tr -d ,))"
  elif have docker; then
    todo "Docker is installed but not running — start Docker Desktop, Colima or the daemon."
  else
    note_missing "Docker"
    todo "Install Docker Desktop, Colima or Docker Engine yourself; this script does not install it."
  fi
fi

# ---------- summary ----------

step "Summary"
if ((CHECK_ONLY)); then
  if ((${#MISSING[@]} == 0)); then
    ok "Everything needed is installed."
  else
    todo "Missing: ${MISSING[*]}"
    echo "  Run scripts/bootstrap.sh (without --check) to install them."
  fi
else
  have go && ok "$(go version)"
  ((SKIP_PYTHON)) || { have uv && ok "uv $(uv --version | awk '{print $2}')"; }
  ((SKIP_NODE)) || { have node && ok "Node.js $(node --version)"; }
  have buf && ok "buf $(buf --version)"
fi

path_hint=()
[[ $PLATFORM == linux && -d /usr/local/go/bin ]] && path_hint+=("/usr/local/go/bin")
[[ -n $GOBIN_DIR ]] && path_hint+=("$GOBIN_DIR")
[[ -d $HOME/.local/bin ]] && path_hint+=("$HOME/.local/bin")
not_on_path=()
for d in ${path_hint[@]+"${path_hint[@]}"}; do
  case ":$ORIG_PATH:" in *":$d:"*) ;; *) not_on_path+=("$d") ;; esac
done
if ((${#not_on_path[@]})); then
  joined="$(IFS=:; echo "${not_on_path[*]}")"
  echo
  echo "Add these to your PATH (for example in ~/.zshrc or ~/.bashrc):"
  echo "  export PATH=\"$joined:\$PATH\""
fi

# --check exits non-zero when anything is missing, so CI can use it as a gate.
if ((CHECK_ONLY && ${#MISSING[@]} > 0)); then
  exit 1
fi
