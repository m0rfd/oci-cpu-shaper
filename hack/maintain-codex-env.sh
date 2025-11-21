#!/usr/bin/env bash
set -euo pipefail

GO_VERSION="${GO_VERSION:-1.25.4}"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v2.6.1}"
GOFUMPT_VERSION="${GOFUMPT_VERSION:-v0.9.2}"
ACTIONLINT_VERSION="${ACTIONLINT_VERSION:-v1.7.8}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_DIR="${HOME}/.config/oci-cpu-shaper"
ENV_FILE="${ENV_DIR}/env.sh"

log() {
    printf '[maintain-codex-env] %s\n' "$*"
}

ensure_env_file() {
    if [[ ! -f "${ENV_FILE}" ]]; then
        log "Environment file missing; running setup script first"
        "${REPO_ROOT}/hack/setup-codex-env.sh"
    fi

    # shellcheck disable=SC1090
    . "${ENV_FILE}"
}

refresh_go() {
    if command -v go >/dev/null 2>&1 && go version | grep -q "go${GO_VERSION} "; then
        log "Go ${GO_VERSION} already present"
        return
    fi

    log "Detected mismatched Go toolchain; reinstalling via setup script"
    "${REPO_ROOT}/hack/setup-codex-env.sh"
    # shellcheck disable=SC1090
    . "${ENV_FILE}"
}

sync_tools_and_modules() {
    mkdir -p "${GOBIN}" "${GOCACHE}" "${GOLANGCI_LINT_CACHE}" "${GOMODCACHE}"
    log "Updating golangci-lint (${GOLANGCI_LINT_VERSION}) and gofumpt (${GOFUMPT_VERSION})"
    go install "github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
    go install "mvdan.cc/gofumpt@v${GOFUMPT_VERSION}"
    go install "github.com/rhysd/actionlint/cmd/actionlint@${ACTIONLINT_VERSION}"

    log "Refreshing module/download caches"
    GOMODCACHE="${GOMODCACHE}" GOCACHE="${GOCACHE}" go mod download
    make -C "${REPO_ROOT}" tools
}

ensure_hooks() {
    if git -C "${REPO_ROOT}" rev-parse --git-dir >/dev/null 2>&1; then
        log "Ensuring linting git hooks remain enabled"
        git -C "${REPO_ROOT}" config core.hooksPath .githooks
        find "${REPO_ROOT}/.githooks" -maxdepth 1 -type f -exec chmod +x {} +
    fi
}

main() {
    ensure_env_file
    refresh_go
    sync_tools_and_modules
    ensure_hooks
    log "Maintenance complete; tooling refreshed and caches hydrated."
}

main "$@"
