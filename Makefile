PROJECT_NAME := UltimaVRedux
BIN_DIR := bin
BINARY := $(BIN_DIR)/ultimav
MAIN_PACKAGE := ./cmd/ultimav

GO ?= go
PYTHON_BOOTSTRAP ?= python3
ifeq ($(OS),Windows_NT)
PYTHON := $(CURDIR)/.cache/python-quality/Scripts/python.exe
else
PYTHON := $(CURDIR)/.cache/python-quality/bin/python
endif
PYTHON_QUALITY := "$(PYTHON)" -I -B scripts/python_quality.py
TOOLS_BIN := $(CURDIR)/$(BIN_DIR)/tools
GOIMPORTS ?= $(TOOLS_BIN)/goimports
GOLANGCI_LINT ?= $(TOOLS_BIN)/golangci-lint
LINT_PACKAGES ?= ./...
TEST_PROFILE ?= host
TEST_GATE_FLAGS = -profile=$(TEST_PROFILE) $(if $(filter 1,$(ULTIMA5_REQUIRE_DATA)),-strict,)
TEST_PROFILE_CPUS = $(if $(filter throughput,$(TEST_PROFILE)),4,2)
TEST_PROFILE_MEMORY = $(if $(filter throughput,$(TEST_PROFILE)),2GiB,256MiB)
TEST_PROFILE_TARGETS := test test-render test-all test-race test-provisioned test-race-provisioned lint lint-all check validate quality-gate

# Capture original Windows environment names before target exports can mask them.
ifeq ($(OS),Windows_NT)
TEST_PROFILE_CASE_KEYS := $(shell env | awk -F= 'toupper($$1) ~ /^(GOMAXPROCS|GOMEMLIMIT)$$/ && $$1 != toupper($$1) { print $$1 }')
endif

# Keep in step with workbench.DefaultPort; make debug prints the address it opens.
WORKBENCH_PORT ?= 8585

CONVERSATION_DATA_DIR ?= ./OLD
CONVERSATION_WORKSPACE ?= $(HOME)/.ultima_v_redux/conversations
CONVERSATION_PATCH_DIR ?= data/conversation-patches

.DEFAULT_GOAL := help

# Serialize Make stages even with inherited -j, without limiting host package
# compilation or the non-graphics lane. Separate Make processes need one owner.
.NOTPARALLEL:

$(TEST_PROFILE_TARGETS): export TEST_PROFILE := $(TEST_PROFILE)
ifneq ($(TEST_PROFILE),host)
$(TEST_PROFILE_TARGETS): export GOMAXPROCS ?= $(TEST_PROFILE_CPUS)
$(TEST_PROFILE_TARGETS): export GOMEMLIMIT ?= $(TEST_PROFILE_MEMORY)
$(TEST_PROFILE_TARGETS): export GOGC ?= 50
$(TEST_PROFILE_TARGETS): export TEST_GO_BUILD_FLAGS := -p=1
endif
# Host leaves Go resource defaults and explicit caller settings intact. Keep
# GOFLAGS untouched, including settings persisted in the user's GOENV file.
$(TEST_PROFILE_TARGETS): test-profile-check

.PHONY: test-profile-check
test-profile-check:
	@case "$(TEST_PROFILE)" in host|constrained|throughput) ;; *) \
		printf 'Unknown TEST_PROFILE: %s; use host, constrained or throughput\n' "$(TEST_PROFILE)" >&2; exit 1;; esac; \
	if [ "$(TEST_PROFILE)" = host ]; then \
		printf 'TEST_PROFILE=host GOMAXPROCS=%s GOMEMLIMIT=%s GOGC=%s (caller settings or Go defaults; logic uses available Go CPUs, native -p=1 -parallel=1)\n' \
			"$${GOMAXPROCS:-auto}" "$${GOMEMLIMIT:-off}" "$${GOGC:-100}"; exit 0; \
	fi; \
	if [ -n "$(TEST_PROFILE_CASE_KEYS)" ]; then \
		printf 'Windows resource variables must use uppercase names before Make: %s\n' "$(TEST_PROFILE_CASE_KEYS)" >&2; exit 1; \
	fi; \
	for setting in "GOMAXPROCS=$$GOMAXPROCS:$(TEST_PROFILE_CPUS)" "GOMEMLIMIT=$$GOMEMLIMIT:$(TEST_PROFILE_MEMORY)"; do \
		name=$${setting%%=*}; values=$${setting#*=}; actual=$${values%:*}; expected=$${values##*:}; \
		if [ "$$actual" != "$$expected" ]; then \
			printf 'TEST_PROFILE=%s requires %s=%s, inherited %s; unset it or select matching settings\n' \
				"$(TEST_PROFILE)" "$$name" "$$expected" "$$actual" >&2; exit 1; \
		fi; \
	done

.PHONY: help all dev debug run tools format format-check lint lint-all lint-config-check core-dependencies-check vet test test-short test-race test-provisioned test-race-provisioned test-integration generate web-install web-build web-test-browser build audio-editor build-check check pre-commit policy-check docs-check gamefacts-check tooling-test site site-publish validate quality-gate conversations-init conversations-diff conversations-check conversations-patches conversations-verify clean

help: ## Show available targets
	@printf "$(PROJECT_NAME) development targets:\n\n"
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

all: check build ## Run checks and build the game

dev: build ## Build and play the game (pass options with ARGS="...")
	@./$(BINARY) $(ARGS)

debug: build ## Build and play with the Debug Workbench on (WORKBENCH_PORT=8585, ARGS="...")
	@printf 'Debug Workbench: http://127.0.0.1:%s\n' "$(WORKBENCH_PORT)"
	@./$(BINARY) -debug-workbench -debug-workbench-port $(WORKBENCH_PORT) $(ARGS)

run: ## Run the existing game binary (pass options with ARGS="...")
	@./$(BINARY) $(ARGS)

tools: ## Provision pinned Go validation tools and module dependencies (networked setup)
	@mkdir -p "$(TOOLS_BIN)"
	@$(GO) mod download
	@GOBIN="$(TOOLS_BIN)" $(GO) install golang.org/x/tools/cmd/goimports@v0.37.0
	@GOBIN="$(TOOLS_BIN)" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

.PHONY: python-tools python-env-check python-format python-format-check python-lint python-check python-staged-check
python-tools: ## Provision the hashed Python quality environment (networked setup, Python 3.12+)
	@"$(PYTHON_BOOTSTRAP)" -I -B scripts/python_quality.py provision

python-env-check:
	@test -x "$(PYTHON)" || { printf '%s\n' 'Missing Python tools: run make python-tools PYTHON_BOOTSTRAP=python3.12 (or another Python 3.12+ interpreter)' >&2; exit 1; }
	@$(PYTHON_QUALITY) ready

python-format: python-env-check ## Apply stable Black formatting to the full Python inventory
	@$(PYTHON_QUALITY) format

python-format-check: python-env-check ## Check stable Black formatting without changing files
	@$(PYTHON_QUALITY) format-check

python-lint: python-env-check ## Require zero enabled Pylint diagnostics across all Python
	@$(PYTHON_QUALITY) lint

python-check: python-format-check python-lint ## Check Python formatting and lint once

python-staged-check: python-env-check ## Require staged/worktree equality for Python quality inputs
	@$(PYTHON_QUALITY) staged-check

format: python-format ## Format Go and Python source and organize Go imports
	@$(GOIMPORTS) -w .

format-check: python-format-check ## Report Go and Python files requiring formatting
	@files="$$($(GOIMPORTS) -l .)" || exit $$?; \
	test -z "$$files" || { \
		printf "Files require formatting; run 'make format':\n"; \
		printf '%s\n' "$$files"; \
		exit 1; \
	}

lint-config-check: ## Verify the pinned linter schema and executable errcheck settings
	@$(GO) run $(TEST_GO_BUILD_FLAGS) ./scripts/check-lint-config "$(GOLANGCI_LINT)"

lint: python-lint lint-config-check ## Run pinned Go and Python linters
	@"$(PYTHON)" -B scripts/lint_gate.py --profile "$(TEST_PROFILE)" --linter "$(GOLANGCI_LINT)" --go "$(GO)" $(LINT_PACKAGES)
	@"$(PYTHON)" -B scripts/audio_module_gate.py --lint

lint-all: python-lint lint-config-check ## Report all configured lint findings without revision filtering or output caps
	@"$(PYTHON)" -B scripts/lint_gate.py --profile "$(TEST_PROFILE)" --linter "$(GOLANGCI_LINT)" --go "$(GO)" --all $(LINT_PACKAGES)
	@"$(PYTHON)" -B scripts/audio_module_gate.py --lint

core-dependencies-check: ## Reject transitive graphics/input dependencies in core code and tests
	@$(GO) run $(TEST_GO_BUILD_FLAGS) ./scripts/check-core-dependencies

vet: ## Run Go static analysis
	@$(GO) vet $(TEST_GO_BUILD_FLAGS) ./...

.PHONY: test-render test-all
test: ## Run uncached logic tests without a display
	@$(GO) run $(TEST_GO_BUILD_FLAGS) ./scripts/test-gate $(TEST_GATE_FLAGS) -suite=logic

test-render: ## Run required native/Ebitengine-linked tests (use Xvfb on headless Linux)
	@$(GO) run $(TEST_GO_BUILD_FLAGS) ./scripts/test-gate $(TEST_GATE_FLAGS) -suite=render

test-all: ## Run aggregate logic and native tests
	@$(GO) run $(TEST_GO_BUILD_FLAGS) ./scripts/test-gate $(TEST_GATE_FLAGS) -suite=all

test-short: ## Run unit tests without integration tests
	@$(GO) test -short ./... -timeout 90s

test-race: ## Run all tests with the race detector
	@$(GO) run $(TEST_GO_BUILD_FLAGS) ./scripts/test-gate $(TEST_GATE_FLAGS) -suite=all -race

test-provisioned: ## Require explicit original data and zero skipped tests
	@$(GO) run $(TEST_GO_BUILD_FLAGS) ./scripts/test-gate $(TEST_GATE_FLAGS) -suite=all -strict

test-race-provisioned: ## Require explicit original data and zero skips with the race detector
	@$(GO) run $(TEST_GO_BUILD_FLAGS) ./scripts/test-gate $(TEST_GATE_FLAGS) -suite=all -strict -race

test-integration: ## Run integration tests
	@$(GO) test -run Integration ./...

generate: ## Run Go code generation
	@$(GO) generate ./...

web-install: ## Install reproducible debug workbench frontend dependencies
	@npm --prefix internal/debugging/webui/client ci

web-build: web-install ## Type-check and build embedded debug workbench assets
	@npm --prefix internal/debugging/webui/client test
	@npm --prefix internal/debugging/webui/client run build

web-test-browser: ## Run Workbench Chromium regressions (browser must be preinstalled)
	@npm --prefix internal/debugging/webui/client run test:browser

build: ## Build the game into bin/ultimav using shipped assets
	@mkdir -p $(BIN_DIR)
	@$(GO) build -o $(BINARY) $(MAIN_PACKAGE)

audio-editor: ## Build the Redux project launcher for the SDK terminal audio editor
	@mkdir -p $(BIN_DIR)
	@$(GO) build -o $(BIN_DIR)/audio-editor ./cmd/audio-editor

.PHONY: audio-editor-run
audio-editor-run: audio-editor ## Build and open the editor with Redux project settings
	@./$(BIN_DIR)/audio-editor -library "$(CURDIR)/internal/audio/assets/library.json" $(ARGS)

.PHONY: audio-catalog audio-catalog-export audio-probe clean-audio-reports
audio-catalog: ## Build the game-specific SDK audio catalog exporter
	@mkdir -p $(BIN_DIR)
	@$(GO) build -o $(BIN_DIR)/audio-catalog ./cmd/audio-catalog

audio-catalog-export: audio-catalog ## Export the derived game inventory and guide into bin/
	@./$(BIN_DIR)/audio-catalog -source . -output $(BIN_DIR)/audio-catalog.json -guide $(BIN_DIR)/audio-generation-guide.md $(ARGS)

audio-probe: audio-editor ## Run a standalone or Ebitengine probe through the Redux project (ARGS required)
	@./$(BIN_DIR)/audio-editor $(ARGS)

clean-audio-reports: ## Remove derived audio reports only, preserving authored assets and settings
	@rm -f $(BIN_DIR)/audio-catalog.json $(BIN_DIR)/audio-generation-guide.md

.PHONY: audio-module-check
audio-module-check: python-env-check ## Check the game-owned Berad Audio bridge against the pinned SDK revision
	@$(GO) test $(TEST_GO_BUILD_FLAGS) -count=1 -run '^TestBeradAudioModule$$' ./scripts/check-audio-module
	@"$(PYTHON)" -B scripts/audio_module_gate.py --lint

.PHONY: vm-settings-proof
vm-settings-proof: ## Run the opt-in Ebitengine VM keyboard/settings rollback proof (requires original data and graphics)
	@mkdir -p $(BIN_DIR)
	@$(GO) test -p=1 -c -tags=ebitenginevmguest,settingsproof -o $(BIN_DIR)/settings-vm-guest ./cmd/ultimav
	@$(GO) build -p=1 -o $(BIN_DIR)/vm-settings-proof ./scripts/vm-settings-proof
	@./$(BIN_DIR)/vm-settings-proof

check: format-check lint vet test ## Run formatting, lint, vet, and logic tests

pre-commit: ## Run the repository pre-commit validation script
	@./scripts/pre-commit-checks.sh

policy-check: ## Run policy checks without repeating formatting, vet, build, or tests
	@./scripts/pre-commit-checks.sh --policy-only

build-check: ## Compile every package without generating application assets
	@$(GO) build $(TEST_GO_BUILD_FLAGS) ./...

docs-check: ## Validate retained documentation links, catalog, ownership, and authority
	@$(GO) run $(TEST_GO_BUILD_FLAGS) ./scripts/check-documentation

gamefacts-check: python-env-check ## Validate public stable references, graph, catalog, and code locators
	@"$(PYTHON)" -B scripts/check-gamefacts.py

tooling-test: python-env-check ## Run Python developer-tool and real quality-tool regressions
	@"$(PYTHON)" -I -B -m unittest discover -s scripts -p 'test_*.py'

site: python-env-check ## Build the public GitHub Pages site into _site (see docs/SITE.md)
	@"$(PYTHON)" -B scripts/build_site.py _site

site-publish: python-env-check ## Build the site and push it to the gh-pages branch GitHub Pages serves (networked)
	@"$(PYTHON)" -B scripts/publish_site.py

validate: check policy-check build-check docs-check gamefacts-check tooling-test ## Run logic tests and canonical static checks once

# Provision with make tools and make python-tools first; validation never installs.
validate: export GOPROXY = off
validate: export GOSUMDB = off
validate: export GOTOOLCHAIN = local

quality-gate: ## Run the local checks required before creating or merging a PR
	@./scripts/quality-gate.sh

conversations-init: ## Create the private original-conversation editing workspace
	@$(GO) run ./cmd/conversation-tool workspace init --data-dir "$(CONVERSATION_DATA_DIR)" --workspace "$(CONVERSATION_WORKSPACE)" --patch-dir "$(CONVERSATION_PATCH_DIR)"

conversations-diff: ## Print private base-to-edited conversation diffs
	@$(GO) run ./cmd/conversation-tool workspace diff --workspace "$(CONVERSATION_WORKSPACE)"

conversations-check: ## Validate the private conversation workspace
	@$(GO) run ./cmd/conversation-tool workspace check --workspace "$(CONVERSATION_WORKSPACE)"

conversations-patches: ## Regenerate cumulative tracked conversation patches
	@$(GO) run ./cmd/conversation-tool workspace patches --workspace "$(CONVERSATION_WORKSPACE)" --patch-dir "$(CONVERSATION_PATCH_DIR)"

conversations-verify: ## Verify tracked patches reproduce all private edits
	@$(GO) run ./cmd/conversation-tool workspace verify --workspace "$(CONVERSATION_WORKSPACE)" --patch-dir "$(CONVERSATION_PATCH_DIR)"

clean: ## Remove generated build artifacts
	@rm -f $(BINARY)
	@rm -f $(BIN_DIR)/ultimav.exe $(BIN_DIR)/audio-editor $(BIN_DIR)/audio-editor.exe $(BIN_DIR)/audio-catalog $(BIN_DIR)/audio-catalog.exe
