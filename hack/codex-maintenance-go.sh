#!/usr/bin/env bash
set -euo pipefail

# Refresh a cached container so tooling stays current after checkout.
# Intended for Codex Environment maintenance runs in Ubuntu 24.04.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_VERSION="${GO_VERSION:-1.25.4}"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v2.6.1}"
GOFUMPT_VERSION="${GOFUMPT_VERSION:-0.9.2}"
ACTIONLINT_VERSION="${ACTIONLINT_VERSION:-v1.7.8}"
GOVULNCHECK_VERSION="${GOVULNCHECK_VERSION:-v1.1.4}"
ARCH="$(uname -m)"

if [[ "${ARCH}" == "aarch64" ]]; then
  GO_ARCH=arm64
else
  GO_ARCH=amd64
fi

run_as_root() {
  if [[ $(id -u) -eq 0 ]]; then
    "$@"
  else
    sudo "$@"
  fi
}

refresh_packages() {
  echo "[maint] Refreshing apt metadata"
  run_as_root apt-get update -y
}

ensure_go() {
  local current_version=""
  if command -v go >/dev/null 2>&1; then
    current_version=$(go version | awk '{print $3}' | sed 's/^go//')
  fi
  if [[ "${current_version}" == "${GO_VERSION}" ]]; then
    echo "[maint] Go ${GO_VERSION} already present"
    return
  fi

  echo "[maint] Updating Go toolchain to ${GO_VERSION}"
  local tarball="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  curl -fsSL "https://go.dev/dl/${tarball}" -o "/tmp/${tarball}"
  run_as_root rm -rf /usr/local/go
  run_as_root tar -C /usr/local -xzf "/tmp/${tarball}"
  rm -f "/tmp/${tarball}"
}

refresh_env() {
  mkdir -p "${HOME}/.cache" "${HOME}/go/bin" "${ROOT_DIR}/.cache/go" "${ROOT_DIR}/.cache/golangci"
  export GOROOT="/usr/local/go"
  export GOPATH="${GOPATH:-${HOME}/go}"
  export PATH="${GOROOT}/bin:${GOPATH}/bin:${PATH}"
  export XDG_CACHE_HOME="${HOME}/.cache"
  export GOCACHE="${GOCACHE:-${XDG_CACHE_HOME}/go-build}"
  export GOMODCACHE="${GOMODCACHE:-${XDG_CACHE_HOME}/go-mod}"
  export GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-${ROOT_DIR}/.cache/golangci}"
}

refresh_tools() {
  echo "[maint] Ensuring Go tools match repository expectations"
  (cd "${ROOT_DIR}" && make tools)
  go install "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}"
}

hydrate_modules() {
  echo "[maint] Syncing module and tool caches"
  (cd "${ROOT_DIR}" && go mod download)
}

refresh_git_hook() {
  local hook_path="${ROOT_DIR}/.git/hooks/pre-commit"
  if [[ ! -f "${hook_path}" ]]; then
    echo "[maint] Installing missing pre-commit hook"
    cat <<'HOOK' > "${hook_path}"
#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "${ROOT_DIR}"
mkdir -p "${ROOT_DIR}/.cache/golangci"
GOLANGCI_LINT_CACHE="${ROOT_DIR}/.cache/golangci" make lint
HOOK
    chmod +x "${hook_path}"
  fi
}

main() {
  refresh_packages
  ensure_go
  refresh_env
  refresh_tools
  hydrate_modules
  refresh_git_hook
  echo "[maint] Maintenance complete. Key env vars: GOPATH=${GOPATH:-$HOME/go}, GOCACHE=${GOCACHE:-$HOME/.cache/go-build}, GOLANGCI_LINT_CACHE=${GOLANGCI_LINT_CACHE:-$ROOT_DIR/.cache/golangci}"
}

main "$@"
