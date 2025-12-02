SHELL := /bin/bash
.DEFAULT_GOAL := help
MAKEFLAGS += --warn-undefined-variables --no-builtin-rules

GO_MACHINE_ARCH := $(shell uname -m)
GO_DL_ARCH := $(if $(filter x86_64,$(GO_MACHINE_ARCH)),amd64,$(if $(filter aarch64,$(GO_MACHINE_ARCH)),arm64,$(GO_MACHINE_ARCH)))
GO_SHA256_linux_amd64 ?= 9fa5ffeda4170de60f67f3aa0f824e426421ba724c21e133c1e35d6159ca1bec
GO_SHA256_linux_arm64 ?= a68e86d4b72c2c2fecf7dfed667680b6c2a071221bbdb6913cf83ce3f80d9ff0

GO ?= go
GO_REQUIRED_VERSION ?= 1.25.4
MIN_COVERAGE ?= 96.0
COVERAGE_PROFILE ?= coverage.out
COVERAGE_SUMMARY ?= coverage.txt
INTEGRATION_COVERAGE_PROFILE ?=
REUSE_INTEGRATION_COVERAGE ?= 0
RUN_E2E_TESTS ?= 0
PYTHON ?= python3

MODULE := $(shell $(GO) list -m 2>/dev/null)
PKGS := $(shell $(GO) list ./... 2>/dev/null)
COVERAGE_EXCLUDES ?= $(if $(MODULE),$(MODULE)/cmd/agentscheck% $(MODULE)/hack/%,)
PROD_PATTERNS := ./cmd/... ./internal/... ./pkg/...
PROD_PKGS := $(shell $(GO) list $(PROD_PATTERNS) 2>/dev/null)
COVERAGE_PKGS := $(filter-out $(COVERAGE_EXCLUDES),$(PROD_PKGS))
INTEGRATION_PKGS_RAW := $(shell $(GO) list ./tests/integration/... ./cmd/shaper 2>/dev/null)
INTEGRATION_PKGS := $(filter-out $(COVERAGE_EXCLUDES),$(INTEGRATION_PKGS_RAW))
E2E_PKGS_RAW := $(shell $(GO) list ./tests/e2e/... ./cmd/shaper 2>/dev/null)
UNIT_TEST_PKGS := $(filter-out $(INTEGRATION_PKGS_RAW) $(E2E_PKGS_RAW),$(PKGS))
COVERAGE_TAGS ?=
E2E_PKGS := $(filter-out $(COVERAGE_EXCLUDES),$(E2E_PKGS_RAW))
COVERAGE_TAG_ARGS := $(if $(strip $(COVERAGE_TAGS)),-tags "$(strip $(COVERAGE_TAGS))",)

GOLANGCI_LINT_VERSION ?= v2.6.1
GOLANGCI_LINT_OS ?= linux
GOLANGCI_LINT_VERSION_STRIPPED := $(patsubst v%,%,$(GOLANGCI_LINT_VERSION))
GOLANGCI_LINT_ARCH := $(GO_DL_ARCH)
GOLANGCI_LINT_TARBALL := golangci-lint-$(GOLANGCI_LINT_VERSION_STRIPPED)-$(GOLANGCI_LINT_OS)-$(GOLANGCI_LINT_ARCH).tar.gz
GOLANGCI_LINT_DOWNLOAD_URL := https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_LINT_VERSION)/$(GOLANGCI_LINT_TARBALL)
GOLANGCI_LINT_SHA256_linux_amd64 := c22e188e46aff9b140588abe6828ba271b600ae82b2d6a4f452196a639c17ec0
GOLANGCI_LINT_SHA256_linux_arm64 := 1c22b899f2dd84f9638e0e0352a319a2867b0bb082c5323ad50d8713b65bb793
GOVULNCHECK_VERSION ?= v1.1.4
ACTIONLINT_VERSION ?= v1.7.9
MBAKE_VERSION ?= 1.4.3
TIDY_VERIFY ?= 1

GO_BIN_PATH := $(strip $(GOBIN))
ifeq ($(GO_BIN_PATH),)
GO_BIN_PATH := $(shell \
        if command -v $(GO) >/dev/null 2>&1; then \
                GOBIN_VALUE="$$($(GO) env GOBIN)"; \
                if [ -n "$$GOBIN_VALUE" ]; then \
                        echo "$$GOBIN_VALUE"; \
                else \
                        echo "$$($(GO) env GOPATH)/bin"; \
                fi; \
        fi)
endif
ifeq ($(GO_BIN_PATH),)
GO_BIN_PATH := $(HOME)/go/bin
endif

ROOT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
GOVULNCHECK_CACHE_DIR ?= $(ROOT_DIR)/.cache/govulncheck
GOCACHE_DIR ?= $(ROOT_DIR)/.cache/go
GOMODCACHE_DIR ?= $(ROOT_DIR)/.cache/gomod
GOLANGCI_LINT_CACHE_DIR ?= $(ROOT_DIR)/.cache/golangci

GOLANGCI_LINT_BIN ?= $(GO_BIN_PATH)/golangci-lint
GOLANGCI_LINT ?= $(GOLANGCI_LINT_BIN)
ACTIONLINT_BIN ?= $(GO_BIN_PATH)/actionlint
ACTIONLINT ?= $(ACTIONLINT_BIN)
ACTIONLINT_FLAGS ?=
ACTIONLINT_PATHS ?=
MBAKE_BIN ?= $(HOME)/.local/bin/mbake
MBAKE ?= $(MBAKE_BIN)
MBAKE_FORMAT_PATHS ?= Makefile

.PHONY: actionlint agents bench build check clean coverage e2e ensure-actionlint ensure-dev-deps ensure-go ensure-golangci-lint ensure-mbake format go-mod-download govulncheck help install-git-hooks integration lint lint-autofix lint-fix lint-makefile lint-workflows maintenance mbake print-golangci-lint-version setup test tidy tools verify-git-hooks verify-go-version
HELP_TARGETS := lint lint-makefile lint-workflows test coverage build check govulncheck integration e2e agents actionlint help clean verify-git-hooks

tools: verify-go-version ensure-golangci-lint ensure-actionlint ensure-mbake

print-golangci-lint-version:
	@printf "%s\n" "$(GOLANGCI_LINT_VERSION)"

ensure-golangci-lint:
	@set -euo pipefail; \
	mkdir -p "$(GO_BIN_PATH)"; \
	BIN="$(GOLANGCI_LINT_BIN)"; \
	CURRENT_VERSION=""; \
	if [ -x "$$BIN" ]; then \
		CURRENT_VERSION="v$$($$BIN version --short 2>/dev/null || true)"; \
	fi; \
	if [ "$$CURRENT_VERSION" = "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "golangci-lint $$CURRENT_VERSION already installed at $$BIN"; \
		exit 0; \
	fi; \
	if [ "$(GOLANGCI_LINT_OS)" != "linux" ]; then \
		echo "Unsupported golangci-lint OS: $(GOLANGCI_LINT_OS)"; \
		exit 1; \
	fi; \
	case "$(GOLANGCI_LINT_ARCH)" in \
		amd64) CHECKSUM="$(GOLANGCI_LINT_SHA256_linux_amd64)" ;; \
		arm64) CHECKSUM="$(GOLANGCI_LINT_SHA256_linux_arm64)" ;; \
		*) echo "Unsupported golangci-lint arch: $(GOLANGCI_LINT_ARCH)"; exit 1 ;; \
	esac; \
	TMP_DIR="$$(mktemp -d)"; \
	trap "rm -rf \"$$TMP_DIR\"" EXIT; \
	echo "Downloading golangci-lint $(GOLANGCI_LINT_VERSION) from $(GOLANGCI_LINT_DOWNLOAD_URL)"; \
	attempts=3; \
	for i in $$(seq 1 $$attempts); do \
		if curl -fsSL "$(GOLANGCI_LINT_DOWNLOAD_URL)" -o "$$TMP_DIR/$(GOLANGCI_LINT_TARBALL)"; then \
			break; \
		fi; \
		if [ $$i -lt $$attempts ]; then \
			sleep $$i; \
		else \
			echo "Failed to download golangci-lint after $$attempts attempts"; \
			exit 1; \
		fi; \
	done; \
	printf "%s  %s\n" "$$CHECKSUM" "$$TMP_DIR/$(GOLANGCI_LINT_TARBALL)" | sha256sum -c -; \
	tar -xzf "$$TMP_DIR/$(GOLANGCI_LINT_TARBALL)" -C "$$TMP_DIR"; \
	install -m 0755 "$$TMP_DIR/golangci-lint-$(GOLANGCI_LINT_VERSION_STRIPPED)-$(GOLANGCI_LINT_OS)-$(GOLANGCI_LINT_ARCH)/golangci-lint" "$(GOLANGCI_LINT_BIN)"; \
	echo "Installed golangci-lint $(GOLANGCI_LINT_VERSION) to $(GOLANGCI_LINT_BIN)"

lint: go-mod-download ensure-golangci-lint
	@mkdir -p "$(GOLANGCI_LINT_CACHE_DIR)"
	@echo "Running golangci-lint..."
	@GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE_DIR)" $(GOLANGCI_LINT) run

lint-makefile: ensure-mbake
	@echo "Running mbake validate..."
	@$(MBAKE) validate $(MBAKE_FORMAT_PATHS)
	@echo "Running mbake check..."
	@$(MBAKE) format --check $(MBAKE_FORMAT_PATHS)

lint-fix: go-mod-download ensure-golangci-lint mbake
	@mkdir -p "$(GOLANGCI_LINT_CACHE_DIR)"
	@echo "Running mbake format..."
	@$(MBAKE) format $(MBAKE_FORMAT_PATHS)
	@echo "Running golangci-lint with fix..."
	@GOLANGCI_LINT_CACHE="$(GOLANGCI_LINT_CACHE_DIR)" $(GOLANGCI_LINT) run --fix

help:
	@printf "Available targets:\n"
	@for target in $(HELP_TARGETS); do \
		case $$target in \
			lint) desc="Run golangci-lint";; \
			lint-makefile) desc="Run mbake validate and check";; \
			lint-workflows) desc="Run actionlint against GitHub workflows";; \
			lint-fix) desc="Run golangci-lint with autofix";; \
			verify-git-hooks) desc="Verify pre-commit hook matches repository template";; \
			test) desc="Run unit tests (excludes integration/e2e)";; \
			coverage) desc="Run coverage with minimum threshold enforcement";; \
			build) desc="Compile all modules with cache isolation";; \
			check) desc="Run lint, coverage, tests, and agent checks";; \
			govulncheck) desc="Scan dependencies with govulncheck";; \
			integration) desc="Execute integration suite (requires Docker + cgroup v2)";; \
			e2e) desc="Execute end-to-end suite";; \
			agents) desc="Validate agent instructions";; \
			actionlint) desc="Lint GitHub Actions workflows";; \
			clean) desc="Remove build caches and coverage artifacts";; \
			help) desc="Show this help";; \
			*) desc="";; \
		esac; \
		printf "  %-14s %s\n" "$$target" "$$desc"; \
	done

ensure-actionlint: verify-go-version
	@set -euo pipefail; \
	mkdir -p "$(GO_BIN_PATH)"; \
	BIN="$(ACTIONLINT_BIN)"; \
	CURRENT_VERSION=""; \
	if [ -x "$$BIN" ]; then \
		CURRENT_VERSION="$$($$BIN -version 2>/dev/null | awk 'NR==1 {print "v"$$2}')"; \
	fi; \
	if [ "$$CURRENT_VERSION" != "$(ACTIONLINT_VERSION)" ]; then \
		echo "Installing actionlint $(ACTIONLINT_VERSION)"; \
		GOBIN="$(GO_BIN_PATH)" $(GO) install github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION); \
	fi

ensure-mbake:
	@set -euo pipefail; \
	BIN="$(MBAKE_BIN)"; \
	CURRENT_VERSION=""; \
	if [ -x "$$BIN" ]; then \
		CURRENT_VERSION="$$($$BIN --version 2>/dev/null | awk '{print $$NF}')"; \
	fi; \
	if [ "$$CURRENT_VERSION" != "$(MBAKE_VERSION)" ]; then \
		echo "Installing mbake $(MBAKE_VERSION)"; \
		$(PYTHON) -m pip install --user --upgrade "mbake==$(MBAKE_VERSION)"; \
	fi

mbake: ensure-mbake
	@$(MBAKE) format $(MBAKE_FORMAT_PATHS)

format: mbake

test: go-mod-download
	@if [ -z "$(strip $(UNIT_TEST_PKGS))" ]; then \
		echo "No Go packages found; skipping tests."; \
	else \
		mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"; \
		GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) test -race $(UNIT_TEST_PKGS); \
	fi
	@e2e_pkgs="$(strip $(E2E_PKGS))"; \
	if [ "$(strip $(RUN_E2E_TESTS))" = "1" ] && [ -n "$$e2e_pkgs" ]; then \
		mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"; \
		GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) test -tags=e2e $$e2e_pkgs; \
	elif [ -n "$$e2e_pkgs" ]; then \
		echo "Skipping e2e tests; set RUN_E2E_TESTS=1 to enable."; \
	fi

coverage: go-mod-download
	@set -euo pipefail; \
	if [ -z "$(strip $(PKGS))" ]; then \
		echo "No Go packages found; skipping coverage."; \
	elif [ -z "$(strip $(COVERAGE_PKGS))" ]; then \
		echo "No Go packages selected for coverage after exclusions; adjust COVERAGE_EXCLUDES."; \
		exit 1; \
	else \
		excluded="$(strip $(COVERAGE_EXCLUDES))"; \
		if [ -n "$$excluded" ]; then \
			echo "Excluding packages from coverage: $$excluded"; \
		fi; \
	coverage_pkgs="$(strip $(COVERAGE_PKGS))"; \
	coverage_csv=$$(printf '%s' "$$coverage_pkgs" | tr ' \n' ',' | sed 's/,,*/,/g; s/^,//; s/,$$//'); \
	mkdir -p "$(dir $(COVERAGE_PROFILE))" "$(dir $(COVERAGE_SUMMARY))" "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"; \
	rm -f $(COVERAGE_PROFILE) $(COVERAGE_SUMMARY); \
	unit_profile="coverage-unit.out"; \
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) test -race -covermode=atomic $(COVERAGE_TAG_ARGS) -coverpkg="$$coverage_csv" -coverprofile="$$unit_profile" $(COVERAGE_PKGS); \
	cat "$$unit_profile" > $(COVERAGE_PROFILE); \
	rm -f "$$unit_profile"; \
	if [ -n "$(strip $(INTEGRATION_PKGS))" ]; then \
		integration_profile="$(strip $(INTEGRATION_COVERAGE_PROFILE))"; \
		if [ -z "$$integration_profile" ]; then \
			integration_profile="coverage-integration.out"; \
		fi; \
	reuse_integration="$(strip $(REUSE_INTEGRATION_COVERAGE))"; \
	if [ "$$reuse_integration" = "1" ]; then \
		if [ ! -f "$$integration_profile" ]; then \
			echo "Integration coverage profile '$$integration_profile' not found."; \
			exit 1; \
		fi; \
	else \
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) test -race -covermode=atomic -tags=integration $(COVERAGE_TAG_ARGS) -coverpkg="$$coverage_csv" -coverprofile="$$integration_profile" $(INTEGRATION_PKGS); \
	fi; \
	tail -n +2 "$$integration_profile" >> $(COVERAGE_PROFILE); \
	if [ "$$reuse_integration" != "1" ]; then \
		rm -f "$$integration_profile"; \
	fi; \
	fi; \
	e2e_pkgs="$(strip $(E2E_PKGS))"; \
	if [ -n "$$e2e_pkgs" ]; then \
		if [ "$(strip $(RUN_E2E_TESTS))" = "1" ]; then \
			e2e_profile="coverage-e2e.out"; \
			if GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) test -race -covermode=atomic -tags=e2e $(COVERAGE_TAG_ARGS) -coverpkg="$$coverage_csv" -coverprofile="$$e2e_profile" $$e2e_pkgs; then \
				tail -n +2 "$$e2e_profile" >> $(COVERAGE_PROFILE); \
			else \
				echo "Skipping e2e coverage due to test failures"; \
			fi; \
	rm -f "$$e2e_profile"; \
	else \
	echo "Skipping e2e coverage; set RUN_E2E_TESTS=1 to enable."; \
	fi; \
	fi; \
	$(GO) tool cover -func=$(COVERAGE_PROFILE) | tee $(COVERAGE_SUMMARY); \
	"$(ROOT_DIR)/hack/coverage_summary_check.sh" "$(COVERAGE_SUMMARY)" "$(MIN_COVERAGE)"; \
	fi

agents: verify-go-version
	@set -euo pipefail; \
	mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"; \
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) run ./cmd/agentscheck

verify-go-version:
	@set -euo pipefail; \
	if ! command -v $(GO) >/dev/null 2>&1; then \
		echo "Go not found in PATH; expected version $(GO_REQUIRED_VERSION)."; \
		exit 1; \
	fi; \
	CURRENT_VERSION="$$( $(GO) version | awk '{print $$3}' | sed 's/^go//' )"; \
	if [ "$$CURRENT_VERSION" != "$(GO_REQUIRED_VERSION)" ]; then \
		echo "Go version $$CURRENT_VERSION detected, but $(GO_REQUIRED_VERSION) is required."; \
		exit 1; \
	fi

govulncheck: verify-go-version
	@set -euo pipefail; \
	mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)" "$(GOVULNCHECK_CACHE_DIR)"; \
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" GOVULNCHECK_CACHE="$(GOVULNCHECK_CACHE_DIR)" \
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

check: go-mod-download verify-git-hooks tidy lint lint-makefile lint-workflows test coverage agents

actionlint: ensure-actionlint
	@set -euo pipefail; \
	if [ ! -d ".github/workflows" ]; then \
		echo "No workflows directory found; skipping workflow lint."; \
	else \
		if [ -n "$(strip $(ACTIONLINT_PATHS))" ]; then \
			$(ACTIONLINT) $(strip $(ACTIONLINT_FLAGS)) $(ACTIONLINT_PATHS); \
		else \
			$(ACTIONLINT) $(strip $(ACTIONLINT_FLAGS)); \
		fi; \
	fi

lint-workflows: actionlint

bench:
	@set -euo pipefail; \
	./hack/check_benchmarks.sh

build: verify-go-version
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) build ./...

integration: verify-go-version
	@set -euo pipefail; \
	if [ "$$(uname -s)" != "Linux" ]; then \
		echo "integration suite requires Linux (detected $$(uname -s))"; \
		exit 1; \
	fi; \
	if ! command -v docker >/dev/null 2>&1; then \
		echo "integration suite requires the docker CLI"; \
		exit 1; \
	fi; \
	if ! docker info >/dev/null 2>&1; then \
		echo "failed to communicate with the Docker daemon"; \
		exit 1; \
	fi; \
	cgroup_version="$$(docker info --format '{{.CgroupVersion}}' 2>/dev/null || true)"; \
	if [ "$$cgroup_version" != "2" ]; then \
		echo "integration suite requires cgroup v2 (detected $${cgroup_version:-unknown})"; \
		exit 1; \
	fi; \
	echo "Docker cgroup version: $$cgroup_version"; \
	controllers_file="/sys/fs/cgroup/cgroup.controllers"; \
	if [ ! -r "$$controllers_file" ]; then \
		echo "cgroup controllers file $$controllers_file is not readable"; \
		exit 1; \
	fi; \
	controllers=$$(tr '\n' ' ' < "$$controllers_file"); \
	if ! grep -qw cpu "$$controllers_file"; then \
	echo "integration suite requires the cgroup v2 cpu controller (controllers: $$controllers)"; \
	exit 1; \
	fi; \
	echo "cgroup v2 controllers: $$controllers"; \
	artifacts_dir="$(ROOT_DIR)/artifacts"; \
	log_file="$$artifacts_dir/integration.log"; \
	mkdir -p "$$artifacts_dir" "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"; \
	coverage_profile="$(strip $(INTEGRATION_COVERAGE_PROFILE))"; \
	coverage_enabled=0; \
	coverage_pkgs=""; \
	coverage_csv=""; \
	if [ -n "$$coverage_profile" ]; then \
		coverage_enabled=1; \
		coverage_pkgs="$(strip $(COVERAGE_PKGS))"; \
		if [ -z "$$coverage_pkgs" ]; then \
			echo "No Go packages selected for coverage after exclusions; adjust COVERAGE_EXCLUDES."; \
			exit 1; \
		fi; \
		coverage_csv=$$(printf '%s' "$$coverage_pkgs" | tr ' \n' ',' | sed 's/,,*/,/g; s/^,//; s/,$$//'); \
		profile_dir=$$(dirname "$$coverage_profile"); \
		mkdir -p "$$profile_dir"; \
	fi; \
	keep_logs="$${INTEGRATION_KEEP_LOGS:-0}"; \
	cleanup() { \
		status="$$?"; \
		if [ "$$status" -eq 0 ] && [ "$$keep_logs" != "1" ]; then \
			rm -f "$$log_file"; \
			rmdir "$$artifacts_dir" 2>/dev/null || true; \
		else \
			echo "Integration logs captured at $$log_file"; \
		fi; \
		exit "$$status"; \
	}; \
	trap 'cleanup' EXIT; \
	touch "$$log_file"; \
	if [ "$$coverage_enabled" -eq 1 ]; then \
		GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) test -tags=integration -covermode=atomic -coverpkg="$$coverage_csv" -coverprofile="$$coverage_profile" -v ./tests/integration/... | tee "$$log_file"; \
	else \
		GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) test -tags=integration -v ./tests/integration/... | tee "$$log_file"; \
	fi

e2e:
	@set -euo pipefail; \
	e2e_pkgs="$(strip $(E2E_PKGS))"; \
	if [ -z "$$e2e_pkgs" ]; then \
		echo "e2e suite not available"; \
		exit 0; \
	fi; \
	mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"; \
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) test -tags=e2e -v $$e2e_pkgs

setup: install-git-hooks ensure-dev-deps ensure-go maintenance
	@set -euo pipefail; \
	if ! command -v go >/dev/null 2>&1; then \
	echo "Go installation failed; check logs above"; \
	exit 1; \
	fi; \
	echo "PATH hints: export PATH=/usr/local/go/bin:\"$${PATH}\" and ensure \"$(GO_BIN_PATH)\" is in PATH for Go tools"; \
	echo "Optional: export GOPATH=$${GOPATH:-$${HOME}/go} and GOBIN=$${GOBIN:-$${GOPATH:-$${HOME}/go}/bin} to keep binaries isolated"; \
	echo "Setup complete; caches live in $(GOCACHE_DIR), $(GOMODCACHE_DIR), and $(GOLANGCI_LINT_CACHE_DIR)";

maintenance: ensure-go go-mod-download tools
	@set -euo pipefail; \
	mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)" "$(GOLANGCI_LINT_CACHE_DIR)"; \
	echo "Dependencies refreshed; Go cache at $(GOCACHE_DIR), module cache at $(GOMODCACHE_DIR), golangci-lint cache at $(GOLANGCI_LINT_CACHE_DIR)";

ensure-dev-deps:
	@set -euo pipefail; \
	if [ ! -r /etc/os-release ]; then \
		echo "/etc/os-release not readable; cannot verify platform"; \
		exit 1; \
	fi; \
	. /etc/os-release; \
	if [ "$$ID" != "ubuntu" ]; then \
		echo "System package install only supported on Ubuntu (detected $$ID)"; \
		exit 1; \
	fi; \
	APT_GET_CMD="apt-get"; \
	if [ "$$EUID" -ne 0 ]; then \
		if command -v sudo >/dev/null 2>&1; then \
			APT_GET_CMD="sudo -n apt-get"; \
		else \
			echo "Root privileges or passwordless sudo required to install system packages"; \
			exit 1; \
		fi; \
	fi; \
	DEBIAN_FRONTEND=noninteractive $$APT_GET_CMD update -y; \
	DEBIAN_FRONTEND=noninteractive $$APT_GET_CMD install -y --no-install-recommends ca-certificates curl git tar gzip build-essential;

ensure-go:
	@set -euo pipefail; \
	if command -v $(GO) >/dev/null 2>&1; then \
	CURRENT_VERSION="$$( $(GO) version | awk '{print $$3}' | sed 's/^go//' )"; \
	if [ "$$CURRENT_VERSION" = "$(GO_REQUIRED_VERSION)" ]; then \
		echo "Go already available: $$($(GO) version)"; \
		exit 0; \
	else \
		echo "Go $$CURRENT_VERSION detected, reinstalling $(GO_REQUIRED_VERSION)"; \
	fi; \
	fi; \
	if [ ! -r /etc/os-release ]; then \
		echo "/etc/os-release not readable; cannot install Go"; \
		exit 1; \
	fi; \
	. /etc/os-release; \
	if [ "$$ID" != "ubuntu" ]; then \
		echo "Go not found and platform ($$ID) is not Ubuntu; aborting install"; \
		exit 1; \
	fi; \
        TARBALL="go$(GO_REQUIRED_VERSION).linux-$(GO_DL_ARCH).tar.gz"; \
        case "$(GO_DL_ARCH)" in \
                amd64) CHECKSUM="$(GO_SHA256_linux_amd64)" ;; \
                arm64) CHECKSUM="$(GO_SHA256_linux_arm64)" ;; \
                *) echo "Unsupported Go arch: $(GO_DL_ARCH)"; exit 1 ;; \
        esac; \
        URL="https://go.dev/dl/$$TARBALL"; \
        echo "Installing Go $(GO_REQUIRED_VERSION) from $$URL"; \
        TMP_DIR="$$(mktemp -d)"; \
        trap "rm -rf \"$$TMP_DIR\"" EXIT; \
	TMP_TARBALL="$$TMP_DIR/$$TARBALL"; \
	attempts=3; \
	for i in $$(seq 1 $$attempts); do \
		if curl -fsSL "$$URL" -o "$$TMP_TARBALL"; then \
			break; \
		fi; \
		if [ $$i -lt $$attempts ]; then \
			sleep $$i; \
		else \
			echo "Failed to download Go after $$attempts attempts"; \
			exit 1; \
		fi; \
	done; \
	printf "%s  %s\n" "$$CHECKSUM" "$$TMP_TARBALL" | sha256sum -c -; \
        rm -rf /usr/local/go; \
        tar -C /usr/local -xzf "$$TMP_TARBALL"; \
        echo "Go $(GO_REQUIRED_VERSION) installed at /usr/local/go";

go-mod-download: verify-go-version
	@set -euo pipefail; \
	if [ ! -f go.mod ]; then \
		echo "go.mod not found; skipping module download."; \
		exit 0; \
	fi; \
	mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"; \
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) mod download; \
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) mod verify

tidy: go-mod-download
	@set -euo pipefail; \
	mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"; \
	GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" $(GO) mod tidy; \
	if [ "$(strip $(TIDY_VERIFY))" = "1" ]; then \
		if ! git diff --quiet -- go.mod go.sum; then \
			echo "go.mod or go.sum changed after tidy; commit or reset the updates."; \
			exit 1; \
		fi; \
	fi

clean:
	@set -euo pipefail; \
	rm -rf "$(COVERAGE_PROFILE)" "$(COVERAGE_SUMMARY)" coverage-*.out "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)" "$(GOLANGCI_LINT_CACHE_DIR)" "$(GOVULNCHECK_CACHE_DIR)" artifacts

lint-autofix: lint-fix

install-git-hooks:
	@set -euo pipefail; \
	git_common_dir="$$(git rev-parse --git-common-dir 2>/dev/null || true)"; \
	if [ -z "$$git_common_dir" ]; then \
		git_common_dir=".git"; \
	fi; \
	if [ -f "$$git_common_dir" ]; then \
		git_common_dir="$$(sed -n 's/^gitdir: //p' "$$git_common_dir")"; \
	fi; \
	if [ -z "$$git_common_dir" ] || [ ! -d "$$git_common_dir" ]; then \
		echo "No git metadata found; skipping hook installation."; \
		exit 0; \
	fi; \
	hooks_path="$$(git config core.hooksPath 2>/dev/null || true)"; \
	repo_root="$$(git rev-parse --show-toplevel 2>/dev/null || pwd)"; \
	case "$$hooks_path" in \
		/*) hook_dir="$$hooks_path" ;; \
		"" ) hook_dir="$$git_common_dir/hooks" ;; \
		*) hook_dir="$$repo_root/$$hooks_path" ;; \
	esac; \
	script_path="hack/githooks/pre-commit"; \
	hook_path="$$hook_dir/pre-commit"; \
	checksum_path="$$hook_path.sha256"; \
	if [ ! -f "$$script_path" ]; then \
		echo "Hook template $$script_path not found" >&2; \
		exit 1; \
	fi; \
	script_checksum="$$(sha256sum "$$script_path" | awk '{print $$1}')"; \
	mkdir -p "$$(dirname "$$hook_path")"; \
	hook_checksum=""; \
	if [ -f "$$hook_path" ]; then \
		hook_checksum="$$(sha256sum "$$hook_path" | awk '{print $$1}')"; \
	fi; \
	stored_checksum=""; \
	if [ -f "$$checksum_path" ]; then \
		stored_checksum="$$(awk 'NR==1 {print $$1}' "$$checksum_path")"; \
	fi; \
	if [ "$$hook_checksum" = "$$script_checksum" ] && [ "$$stored_checksum" = "$$script_checksum" ]; then \
		echo "Pre-commit hook already up to date."; \
		exit 0; \
	fi; \
	install -m 0755 "$$script_path" "$$hook_path"; \
	printf "%s\n" "$$script_checksum" > "$$checksum_path"; \
	if [ -f "$$hook_path" ]; then \
		echo "Refreshed pre-commit hook from $$script_path."; \
	else \
		echo "Installed pre-commit hook with auto-staging autofix."; \
	fi

verify-git-hooks:
	@set -euo pipefail; \
	if [ "$${CI:-}" = "true" ]; then \
		echo "Running in CI; skipping hook verification."; \
		exit 0; \
	fi; \
	git_common_dir="$$(git rev-parse --git-common-dir 2>/dev/null || true)"; \
	if [ -z "$$git_common_dir" ]; then \
		git_common_dir=".git"; \
	fi; \
	if [ -f "$$git_common_dir" ]; then \
		git_common_dir="$$(sed -n 's/^gitdir: //p' "$$git_common_dir")"; \
	fi; \
	if [ -z "$$git_common_dir" ] || [ ! -d "$$git_common_dir" ]; then \
		echo "No git metadata found; skipping hook verification."; \
		exit 0; \
	fi; \
	hooks_path="$$(git config core.hooksPath 2>/dev/null || true)"; \
	repo_root="$$(git rev-parse --show-toplevel 2>/dev/null || pwd)"; \
	case "$$hooks_path" in \
		/*) hook_dir="$$hooks_path" ;; \
		"" ) hook_dir="$$git_common_dir/hooks" ;; \
		*) hook_dir="$$repo_root/$$hooks_path" ;; \
	esac; \
	script_path="hack/githooks/pre-commit"; \
	hook_path="$$hook_dir/pre-commit"; \
	checksum_path="$$hook_path.sha256"; \
	if [ ! -f "$$script_path" ]; then \
		echo "Hook template $$script_path not found" >&2; \
		exit 1; \
	fi; \
	if [ ! -f "$$hook_path" ]; then \
		echo "Pre-commit hook missing at $$hook_path. Run 'make install-git-hooks'."; \
		exit 1; \
	fi; \
	if [ ! -f "$$checksum_path" ]; then \
		echo "Checksum file $$checksum_path missing. Run 'make install-git-hooks'."; \
		exit 1; \
	fi; \
	expected_checksum="$$(sha256sum "$$script_path" | awk '{print $$1}')"; \
	stored_checksum="$$(awk 'NR==1 {print $$1}' "$$checksum_path")"; \
	hook_checksum="$$(sha256sum "$$hook_path" | awk '{print $$1}')"; \
	if [ "$$stored_checksum" != "$$expected_checksum" ]; then \
		echo "Pre-commit hook checksum out of date. Run 'make install-git-hooks'."; \
		exit 1; \
	fi; \
	if [ "$$hook_checksum" != "$$expected_checksum" ]; then \
		echo "Pre-commit hook at $$hook_path differs from template. Run 'make install-git-hooks'."; \
		exit 1; \
	fi; \
	echo "Pre-commit hook verified at $$hook_path."
