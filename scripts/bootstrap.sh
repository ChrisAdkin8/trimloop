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
#   DuckDB CLI  writing and testing aggregate SQL
#   Docker      optional: local ClickHouse/Postgres for the team collector
#
# Supported: macOS (Homebrew) and Debian/Ubuntu (apt).
# Safe to re-run: anything already present at the minimum version is skipped.
#
# Usage: scripts/bootstrap.sh [--check] [--yes] [--skip-node] [--skip-python]
#                             [--with-collector] [--help]

set -euo pipefail

ORIG_PATH="$PATH"
GO_MIN="1.27.3"
PYTHON_VERSION="3.12"   # pinned for the LiteLLM plugin
NODE_MIN="22"
# Pinned tool versions. DUCKDB_VERSION should match the DuckDB bundled by the Go
# driver in go.mod, so aggregate SQL is written against the engine that ships.
# The DuckDB CLI is a convenience: on Linux the pinned release is installed; on
# macOS Homebrew installs its current release and a mismatch is reported but
# does not fail --check.
DUCKDB_VERSION="v2.0.0"

# On Linux, Go, uv and Node.js are pinned to releases at least two weeks old,
# each with the SHA-256 of its download recorded here, so a download is
# checked against this file rather than against a checksum from the same
# place (ADR 0051). Update the version and both checksums together.
GO_PIN="1.27.3"
GO_SHA256_amd64="65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40"
GO_SHA256_arm64="a06e99309dbcbdca1a8ccd2282c3ee146e55930886e26b4f1193269587165f49"
UV_PIN="0.13.5"
UV_SHA256_amd64="f68d291c75783122dad58ab77def0e13f3233f358731db5d6d7e8715603b928f"
UV_SHA256_arm64="4bb69cf95ed13f23713bb7bc09f241896ea0c75b7c557e615a20604b82f9c403"
NODE_PIN="v24.22.1"
NODE_SHA256_amd64="6c172a9ab6718f7b801fb5a7bd486af2ee5e32758086a04b84fcce2aa77095e6"
NODE_SHA256_arm64="dbabf7a1cbf947e6059ffad2c27b80041db97dea1e553b62afdee7f13fca336e"

# pinned_sha prints the pinned checksum for a tool on this machine's architecture.
pinned_sha() { local v="${1}_SHA256_${GOARCH}"; echo "${!v}"; }
BUF_VERSION="v1.75.0"
PROTOC_GEN_GO_VERSION="v1.36.12"
PROTOC_GEN_GO_GRPC_VERSION="v1.6.2"
GOLANGCI_LINT_VERSION="v2.15.0"
GOVULNCHECK_VERSION="v1.9.0"

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
CONFIRMED=0
# Records a missing item. The first time something is missing in install mode,
# asks once whether to go ahead, so nothing is asked when all is present.
note_missing() {
  MISSING+=("$1"); todo "$1: missing or too old"
  if ((!CHECK_ONLY && !CONFIRMED)); then
    if ! confirm "Install missing SDKs and tools now?"; then
      echo "Nothing changed. Re-run with --check to see what is missing."
      exit 0
    fi
    CONFIRMED=1
  fi
}

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
  if [[ ! -t 0 ]]; then
    fail "No terminal to ask for confirmation; re-run with --yes (or --check)."
    exit 2
  fi
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
  pkgs=(build-essential pkg-config git curl ca-certificates xz-utils jq python3)
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
[[ $PLATFORM == linux && -x /usr/local/go/bin/go ]] && export PATH="/usr/local/go/bin:$PATH"
go_current=""
have go && go_current="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
if [[ -n $go_current ]] && version_ge "$go_current" "$GO_MIN"; then
  ok "Go $go_current"
else
  note_missing "Go"
  if [[ $PLATFORM == macos ]]; then
    run brew install go
  else
    # The pinned release from go.dev, verified against the SHA-256 recorded above.
    if ((CHECK_ONLY)); then
      echo "    would download Go $GO_PIN for linux-$GOARCH from go.dev and install to /usr/local/go"
    else
      go_file="go${GO_PIN}.linux-${GOARCH}.tar.gz"
      go_sha="$(pinned_sha GO)"
      curl -fsSL -o "$TMP/$go_file" "https://go.dev/dl/$go_file"
      echo "$go_sha  $TMP/$go_file" | sha256sum -c - >/dev/null || die "Go download failed checksum verification."
      $SUDO rm -rf /usr/local/go
      $SUDO tar -C /usr/local -xzf "$TMP/$go_file"
      export PATH="/usr/local/go/bin:$PATH"
      ok "Installed $(go version)"
    fi
  fi
fi

if ((!CHECK_ONLY)) && have go; then
  go_now="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
  version_ge "$go_now" "$GO_MIN" \
    || die "Go $go_now is first on PATH ($(command -v go)) but $GO_MIN or newer is needed. Remove or reorder the older Go and re-run."
fi

# ---------- Go tools ----------

step "Go tools (buf, protoc plugins, linters)"
GOBIN_DIR=""
if have go; then
  GOBIN_DIR="$(go env GOBIN)"
  [[ -n $GOBIN_DIR ]] || GOBIN_DIR="$(go env GOPATH)/bin"
  export PATH="$GOBIN_DIR:$PATH"
fi
# Module version a Go binary was built from ("" if unknown).
go_bin_version() { have go && go version -m "$(command -v "$1")" 2>/dev/null | awk '$1=="mod"{print $3; exit}'; }
go_tools=(
  "buf|github.com/bufbuild/buf/cmd/buf@${BUF_VERSION}"
  "protoc-gen-go|google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}"
  "protoc-gen-go-grpc|google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}"
  "golangci-lint|github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
  "govulncheck|golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}"
)
for entry in "${go_tools[@]}"; do
  bin="${entry%%|*}"; pkg="${entry#*|}"; want="${pkg##*@}"
  have_ver=""; have "$bin" && have_ver="$(go_bin_version "$bin")"
  if have "$bin" && [[ $have_ver == "$want" ]]; then
    ok "$bin $have_ver"
  elif have "$bin" && [[ -z $have_ver ]]; then
    todo "$bin present, version unknown (pinned $want); reinstall with: go install $pkg"
  else
    [[ -n $have_ver ]] && todo "$bin $have_ver installed; $want pinned"
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
      echo "    would download uv $UV_PIN from GitHub, verify its SHA-256 and install to ~/.local/bin"
    else
      # The pinned release archive, checked against the SHA-256 recorded
      # above; no installer script, no shell rc edits.
      uv_triple=$([[ $GOARCH == amd64 ]] && echo x86_64-unknown-linux-gnu || echo aarch64-unknown-linux-gnu)
      uv_file="uv-${uv_triple}.tar.gz"
      uv_url="https://github.com/astral-sh/uv/releases/download/${UV_PIN}/${uv_file}"
      curl -fsSL -o "$TMP/$uv_file" "$uv_url" || die "Could not download $uv_file."
      [[ "$(pinned_sha UV)" == "$(sha256sum "$TMP/$uv_file" | awk '{print $1}')" ]] \
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
      echo "    would download Node.js $NODE_PIN for linux-$GOARCH from nodejs.org into /usr/local/lib/nodejs"
    else
      # The pinned LTS release, verified against the SHA-256 recorded above.
      node_ver="$NODE_PIN"
      node_arch=$([[ $GOARCH == amd64 ]] && echo x64 || echo arm64)
      node_file="node-${node_ver}-linux-${node_arch}.tar.xz"
      curl -fsSL -o "$TMP/$node_file" "https://nodejs.org/dist/${node_ver}/${node_file}"
      echo "$(pinned_sha NODE)  $TMP/$node_file" | sha256sum -c - >/dev/null \
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
  duck_now="$(duckdb --version 2>/dev/null | awk '{print $1}')"
  if [[ $duck_now == "$DUCKDB_VERSION" ]]; then
    ok "duckdb $duck_now"
  else
    todo "duckdb $duck_now installed; $DUCKDB_VERSION expected (the version the Go driver bundles)"
  fi
else
  note_missing "duckdb CLI"
  if [[ $PLATFORM == macos ]]; then
    run brew install duckdb
  elif ((CHECK_ONLY)); then
    echo "    would download DuckDB CLI $DUCKDB_VERSION from GitHub, verify its SHA-256 and install to /usr/local/bin"
  else
    # Pinned release for this machine's architecture only, verified against the
    # SHA-256 digest GitHub publishes for the asset, then run once to prove it works.
    duck_asset="duckdb-cli-linux-${GOARCH}.tar.gz"
    duck_sha="$(curl -fsSL "https://api.github.com/repos/duckdb/duckdb/releases/tags/${DUCKDB_VERSION}" | python3 -c '
import json, sys
name = sys.argv[1]
a = next((x for x in json.load(sys.stdin)["assets"] if x["name"] == name), None)
print((a or {}).get("digest", "").removeprefix("sha256:"))' "$duck_asset")" || duck_sha=""
    installed=0
    if [[ -n $duck_sha ]] \
       && curl -fsSL -o "$TMP/$duck_asset" "https://github.com/duckdb/duckdb/releases/download/${DUCKDB_VERSION}/${duck_asset}" \
       && [[ "$(sha256sum "$TMP/$duck_asset" | awk '{print $1}')" == "$duck_sha" ]] \
       && mkdir -p "$TMP/duckdb-cli" && tar -C "$TMP/duckdb-cli" -xzf "$TMP/$duck_asset" \
       && "$TMP/duckdb-cli/duckdb" --version >/dev/null 2>&1; then
      $SUDO install -m 0755 "$TMP/duckdb-cli/duckdb" /usr/local/bin/duckdb
      installed=1
    fi
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
