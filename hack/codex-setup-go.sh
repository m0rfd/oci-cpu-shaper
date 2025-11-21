#!/usr/bin/env bash
set -euo pipefail

# Bootstrap a fresh container for Go development on Ubuntu 24.04.
# This script is intended for Codex Environment setup runs executed
# immediately after cloning the repository.

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

APT_PACKAGES=(build-essential curl git ca-certificates jq make)

run_as_root() {
  if [[ $(id -u) -eq 0 ]]; then
    "$@"
  else
    sudo "$@"
  fi
}

ensure_packages() {
  echo "[setup] Installing base packages: ${APT_PACKAGES[*]}"
  run_as_root apt-get update -y
  run_as_root apt-get install -y --no-install-recommends "${APT_PACKAGES[@]}"
  run_as_root apt-get autoremove -y
}

install_go() {
  local current_version=""
  if command -v go >/dev/null 2>&1; then
    current_version=$(go version | awk '{print $3}' | sed 's/^go//')
  fi
  if [[ "${current_version}" == "${GO_VERSION}" ]]; then
    echo "[setup] Go ${GO_VERSION} already installed"
    return
  fi

  echo "[setup] Installing Go ${GO_VERSION} for ${GO_ARCH}"
  local tarball="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  curl -fsSL "https://go.dev/dl/${tarball}" -o "/tmp/${tarball}"
  run_as_root rm -rf /usr/local/go
  run_as_root tar -C /usr/local -xzf "/tmp/${tarball}"
  rm -f "/tmp/${tarball}"
}

write_env_hints() {
  local profile_snippet="${HOME}/.profile"
  if ! grep -q "OCI_CPU_SHAPER_GO_ENV" "${profile_snippet}" 2>/dev/null; then
    cat <<'SNIPPET' >> "${profile_snippet}"

# OCI CPU Shaper Go environment (OCI_CPU_SHAPER_GO_ENV)
export GOROOT="/usr/local/go"
export GOPATH="${HOME}/go"
export GOBIN="${GOPATH}/bin"
export PATH="${GOROOT}/bin:${GOBIN}:${PATH}"
export XDG_CACHE_HOME="${HOME}/.cache"
export GOCACHE="${XDG_CACHE_HOME}/go-build"
export GOMODCACHE="${XDG_CACHE_HOME}/go-mod"
export GOLANGCI_LINT_CACHE="${XDG_CACHE_HOME}/golangci"
SNIPPET
  fi
}

apply_env_settings() {
  mkdir -p "${HOME}/.cache" "${HOME}/go/bin" "${ROOT_DIR}/.cache/go" "${ROOT_DIR}/.cache/golangci"
  export GOROOT="/usr/local/go"
  export GOPATH="${GOPATH:-${HOME}/go}"
  export GOBIN="${GOBIN:-${GOPATH}/bin}"
  export PATH="${GOROOT}/bin:${GOBIN}:${PATH}"
  export XDG_CACHE_HOME="${HOME}/.cache"
  export GOCACHE="${GOCACHE:-${XDG_CACHE_HOME}/go-build}"
  export GOMODCACHE="${GOMODCACHE:-${XDG_CACHE_HOME}/go-mod}"
  export GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-${ROOT_DIR}/.cache/golangci}"
}

install_tools() {
  echo "[setup] Installing Go tools"
  (cd "${ROOT_DIR}" && make tools)
  go install "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}"
}

prime_modules() {
  echo "[setup] Downloading module dependencies"
  (cd "${ROOT_DIR}" && go mod download)
}

install_git_hook() {
  local hook_path="${ROOT_DIR}/.git/hooks/pre-commit"
  cat <<'HOOK' > "${hook_path}"
#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "${ROOT_DIR}"
mkdir -p "${ROOT_DIR}/.cache/golangci"
GOLANGCI_LINT_CACHE="${ROOT_DIR}/.cache/golangci" make lint
HOOK
  chmod +x "${hook_path}"
  echo "[setup] Installed pre-commit hook to run golangci-lint with autofix"
}

main() {
  ensure_packages
  install_go
  write_env_hints
  apply_env_settings
  install_tools
  prime_modules
  install_git_hook
  echo "[setup] Go environment ready. Recommended env: GOPATH=${GOPATH:-$HOME/go}, GOCACHE=${GOCACHE:-$HOME/.cache/go-build}, GOLANGCI_LINT_CACHE=${GOLANGCI_LINT_CACHE:-$HOME/.cache/golangci}"
}

main "$@"
