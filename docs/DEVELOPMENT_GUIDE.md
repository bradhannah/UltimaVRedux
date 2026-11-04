# Development Guide

This guide covers local source development. Players should use
[Installation](INSTALL.md). Discuss changes through the
[issue-first contribution process](../CONTRIBUTING.md) before preparing a PR.
Architecture and system documents describe durable contracts independently of any
particular editor or assistant.

## Setup

The application source is public. For local development, provision:

- Go: use Go 1.27's latest patch or newer; `go.mod` declares `go 1.27.0` and
  `toolchain go1.27.1`. Provision the selected toolchain before offline validation.
  Ebitengine is pinned to v2.10.4.
- Git and Make for the contributor workflows below.
- Python **3.12+** for developer scripts and their quality tools.
- Repository-pinned Go tools, installed by `make tools` into `bin/tools`.
- Optional Node/npm when changing or validating the Debug Workbench frontend;
  provision its dependencies separately before offline checks.

Player installation needs neither Node, Make, an AI tool, nor a C compiler;
see [platform status](INSTALL.md#platform-status) for what has been verified on
each target.
Race-instrumented developer tests have separate native compiler requirements.
The text editor adapter uses the supported, deprecated `exp/textinput.Field`
for IME composition and selection; replacing it requires preserving those
contracts and native input acceptance. Cross-builds do not prove native acceptance.

```sh
make tools
make python-tools PYTHON_BOOTSTRAP=python3.12
make validate
```

`make tools` is networked provisioning: it downloads Go modules and installs
pinned goimports and golangci-lint. Select an installed Python 3.12+ interpreter
for `PYTHON_BOOTSTRAP` (a command name or absolute path); install Python and
optional browser tools separately. `make validate` is local/offline and fails
rather than downloading a missing prerequisite. See [Testing](TESTING.md) for the public asset-independent
lane, frontend checks, and explicitly provisioned original-data tests.

The experimental [Berad Audio module](https://github.com/bradhannah/berad-sdk/tree/564225ae5d47/modules/berad-audio)
lives in the separate public SDK repository. `make tools` provisions the game's
pinned dependencies, including audio. `make audio-module-check` exercises and
lints the game-owned bridge against that published SDK version. SDK unit/race and
independent-consumer checks belong to the SDK; game checks need no SDK checkout
and do not rerun its independent suites. Publish an available SDK revision before
changing the game pin. No application workspace or local replacement is required;
the throwaway bridge substitutes only the candidate game, never the SDK.

Reusable audio development guidance and the optional OpenCode domain skill are
[SDK-owned](https://github.com/bradhannah/berad-sdk/blob/main/README.md#converting-an-existing-subsystem).
The [Redux audio guide](systems/AUDIO_SYSTEM.md) owns this application's callbacks,
world/music policy, assets and listening acceptance; generic editor/authoring
contracts are linked upstream rather than maintained twice. No assistant or
private tooling is required to use these public contracts.

The terminal editor and report implementation now come from the separately pinned
[audio tools module](https://github.com/bradhannah/berad-sdk/tree/20020a7917b6/tools/berad-audio).
Use `make audio-editor`, `make audio-catalog`, and `make audio-catalog-export`
for Redux-configured launchers/reports. Existing editor probe flags remain supported.
For a single build-and-open command, run `make audio-editor-run` in this checkout.
It selects the Redux manifest explicitly. `make audio-editor` remains build-only.
`make audio-probe ARGS='...'` invokes them in a standalone process. `make clean`
removes known executables; `make clean-audio-reports` removes derived reports.
Neither deletes authored JSON/media, player settings or `bin/tools` validators.

Text rendering similarly consumes the pinned
[Berad Text module](https://github.com/bradhannah/berad-sdk/tree/564225ae5d47/modules/berad-text).
`internal/text` is the game's asset/semantic/history adapter; its fonts, glyph
rendering, retained geometry and image cache use the SDK. SDK source/public-consumer
and native-render regressions belong upstream. Game `internal/text`, widget and
scene tests verify compatibility against the published pin without a sibling SDK
checkout or replacement. See [message output](systems/MESSAGE_OUTPUT_SYSTEM.md)
and [ADR-011](ADR/ADR-011-berad-text.md). The module is experimental; this does not
claim a stable SDK release or new Windows/Linux native acceptance.

Graphics integration consumes the published
[Berad Graphics module](https://github.com/bradhannah/berad-sdk/tree/20020a7917b6/modules/berad-graphics).
Sprite and VFX facades retain game artwork/palette/phase policy while the SDK owns
pixel operations and resource reuse. Sprite, visual, portrait and scene render
tests exercise the actual pin; independent synthetic/native SDK tests stay upstream.
See [ADR-012](ADR/ADR-012-berad-graphics.md) and
[sprite integration](systems/SPRITE_OUTLINE_SYSTEM.md). Publish an available SDK
revision before advancing the dependency; no workspace replacement is required.

[Berad RNG](https://github.com/bradhannah/berad-sdk/tree/891cf44f456a/modules/berad-rng)
and [Berad Input](https://github.com/bradhannah/berad-sdk/tree/891cf44f456a/modules/berad-input)
provide engine-free deterministic streams and keyboard mechanisms. The game imports
the RNG source directly and keeps its `GameState` RNG policy and `pkg/input` adapter;
saved seed/source-call state and command/repeat policy remain local. SDK checks use `make rng-check` and
`make input-check`; game save, keyboard, widget and scene regressions exercise the
published pins. See [ADR-013](ADR/ADR-013-berad-rng-input.md),
[RNG ownership](systems/TIME_SYSTEM.md#shared-rng-ownership) and
[keyboard integration](systems/INPUT_HANDLING.md#keyboard-mechanism-boundary).

[Berad Path](https://github.com/bradhannah/berad-sdk/tree/bea6c1522e86/modules/berad-path)
is the A* route search. `references.FindRoute` adapts it to tile positions; the
game keeps terrain prices, occupancy, the overworld search window and its edge
wrapping, and what a unit does without a route. SDK checks use `make path-check`;
game AI and combat-assistance tests exercise the published pin.

The consumed Audio/Text runtimes and audio tools have passed the SDK's
[strict formatting/lint policy](https://github.com/bradhannah/berad-sdk/blob/81526f97e7c9/docs/LINTING.md):
the same pinned Go style tools and applicable strict rules, with full-source
zero-findings enforcement across registered SDK modules. Game lint covers the
game's adapters; it does not lint dependency source or replace that SDK gate.

Use [original-data setup](INSTALL.md#locate-the-original-game-data) to discover an
existing GOG/Heroic installation, use the displayed home/UltimaV folder,
or edit the settings file externally and recheck. Fresh setup requires explicit
confirmation even for one result; valid configured locations are retained.
There are no in-game data or legacy-save path fields. A complete original
installation is external even though the application retains bundled assets and
test fixtures with their notices. Tests needing original files should receive an
explicit data path; public asset-independent validation must not silently rely
on a developer's local files.

## Python quality workflow

`make python-tools PYTHON_BOOTSTRAP=python3.12` creates the ignored
`.cache/python-quality` virtual environment and installs `requirements-quality.lock`
using pip's required hashes and binary wheels. This is the only networked Python
setup step; it changes neither global Python nor Git configuration. If `python3`
is older (for example the Apple system interpreter), select a modern bootstrap
explicitly. An old bootstrap reports the required version and command. To refresh
an existing environment, deliberately remove that environment and reprovision;
setup refuses to mutate an existing environment in place.

`requirements-quality.in` pins Black **26.10.1** and Pylint **4.0.4**. The lock
pins and hashes their transitive dependencies for Python 3.12+ across platforms.
`mypy-extensions` is a Black dependency, not the Mypy checker. To deliberately
refresh the lock with separately installed `uv`:

```sh
uv pip compile --python-version 3.12 --universal --generate-hashes --output-file requirements-quality.lock requirements-quality.in
```

Review the lock and re-create the environment before checking. Normal Make checks
use its interpreter directly (`bin/python`, or `Scripts/python.exe` on Windows),
so an old ambient `python3` cannot select the validation runtime. Missing tools
or mismatched installed versions fail with setup guidance. Operational scripts
can also run directly with any supported Python and the standard library; they
do not import Black or Pylint.

| Command | Effect |
| --- | --- |
| `make python-tools` | Provision the separate environment; defaults to `PYTHON_BOOTSTRAP=python3`. |
| `make python-format` | Apply stable Black formatting, 88 columns. |
| `make python-format-check` | Check formatting and display differences without changing files. |
| `make python-lint` | Run Pylint with its bundled typing plugin and zero enabled diagnostics. |
| `make python-check` | Run the format check and lint once each. |
| `make tooling-test` | Run existing script tests and the real-tool/inventory/hook regressions. |

The repo-owned `scripts/python_quality.py` inventories all tracked and nonignored
new `.py` files, including tests and tracked files that later match an ignore rule.
Deleted files are absent from the working inventory. Symlinked Python/config
inputs and directory symlinks that could hide source are rejected. There is no
fixed file allowlist or new-code cutoff. Black caches and bytecode are disabled
during checks; Pylint persistence is disabled. Configuration lives in
`pyproject.toml`. Pylint scoring is off and effective `exit-zero` is false. In
Pylint 4.0.4, writing even `exit-zero=false` activates its flag-style option, so
the key is intentionally omitted; a real undefined-name fixture verifies failure.
Enabled informational diagnostics (including stale suppressions) also fail.
The runner also requires an empty JSON diagnostic list and a zero exit status:
Pylint configuration errors can emit diagnostics while returning zero. Missing or
malformed diagnostic output fails validation.

`make format` includes Python formatting. `make format-check` and `make lint`
include their Python checks, so `check`, `validate`, and the existing quality-gate
graph execute each check once. The optional native `.githooks/pre-commit` delegates
to `scripts/pre-commit-checks.sh`, which owns the full check-only route. Before and
after checking, it requires exact staged/worktree bytes and executable modes for
Python, quality configuration/locks, Makefile, ignore/attribute rules, hooks,
workflow files and shell runners. Partially staged or nonignored new relevant
files fail with staging guidance. Hooks never format, stage, provision or repair
the candidate. `--policy-only` retains the aggregate's nonduplicating policy route.

### Python tool identity

For an external validation receipt owner, run:

```sh
.cache/python-quality/bin/python -I -B scripts/python_quality.py identity
```

Success emits one JSON object to stdout; failure emits diagnostics to stderr with
a nonzero exit. `schema_version: 1` contains `python_version`, `interpreter`,
`environment`, `stdlib`, `dependencies`, `distributions`, `configuration`, `lock`,
`helper`, and `sha256`.
The implementation components have `path` and `sha256` fields. The
`helper` component hashes the actual `scripts/python_quality.py` implementation.
The distribution map provides names/versions for interpretation, not proof by itself.
The environment component binds `pyvenv.cfg`; installed distributions outside the
dedicated environment are rejected. The interpreter component hashes its
executable, available shared Python library and importable stdlib zip. Stdlib and
installed-package trees hash names, modes,
symlink targets and actual file bytes, including dependency metadata/configuration;
bytecode and cache directories are excluded. Directory symlinks and unreadable
implementation files fail. `configuration` binds the complete relevant repository
inventory's names, modes and bytes, and `lock` separately identifies the hashed
lock. Top-level `sha256` hashes the canonical JSON object without that field (sorted keys, compact
separators, UTF-8). Paths are local diagnostics; do not publish identity output
without reviewing them. The command supplies identity, not a validation receipt
or an assertion that checks passed.

## Go lint policy

All configured linters, thresholds, and exclusions are defined in `.golangci.yml`.
`make lint` requires zero findings across every linted file; there is no revision
cutoff, and the config verifier rejects one. `make lint-all` runs the same analysis
without output caps, so every finding is listed at once.

Resolve all configured findings in each file you change. Repository-wide autofix
is a separate, reviewed pass; files changed only by autofix are exempt from further
manual cleanup. Reject automatic rewrites that change domain identities, callback
ordering, or dynamically growing loop semantics. Formatting and tests remain
required after reviewing the automatic diff. This policy does not relax enabled checks.

Canonical lint checks enumerate the requested packages with `go list` and run
serial batches of four through the same pinned linter/configuration. Each package
receives the complete configured analysis once; an earlier failure does not omit later batches. This
bounds retained analyzer graphs without changing the configured rules. Duplicate-code detection additionally runs over the full inventory so
cross-batch duplication remains visible. `lint-all` retains uncapped
output in every batch and the whole-inventory duplicate pass.

## Starting The Debug Workbench

```sh
make dev ARGS="-debug-workbench"
```

Open `http://127.0.0.1:8585`. For another loopback port:

```sh
make dev ARGS="-debug-workbench -debug-workbench-port 8686"
```

The server binds only to `127.0.0.1`; browser requests execute through the game
update thread. Use the navigation-only Command Palette (`Ctrl+K` / `Command+K`)
to find the owning instrument. Selecting a surface may load a read-only query
but never executes a mutation. The live catalog supplies current commands and
availability reasons. See [Workbench architecture](systems/DEBUG_WORKBENCH.md)
for review, revision, and rollback contracts, and
[Workbench UI](ui/DEBUG_WORKBENCH_UI.md) for interaction ownership.

## Common Commands

| Command | Purpose |
|---|---|
| `go build -o bin/ultimav ./cmd/ultimav` | Build local production source without Make. |
| `go run ./cmd/save-convert --help` | Inspect the headless converter; follow the [save converter procedure](details/SAVE_CONVERTER.md) for explicit inputs and output modes. |
| `make dev` | Build and run the game for development. |
| `make test` | Run the Go test suite. |
| `make test-integration` | Run tests whose names match `Integration`, not the complete real-data suite. |
| `make test-provisioned` / `make test-race-provisioned` | Run every package with explicit original data and zero skips. |
| `make vm-settings-proof` | Run the opt-in keyboard/settings rollback proof with original data and a native graphics context; see [Testing](TESTING.md#vm-keyboardsettings-proof). |
| `make format` | Apply repository formatting. |
| `make lint` | Run the repository-pinned Go and Python linters. |
| `make web-build` | Rebuild Debug Workbench browser assets after frontend changes. |
| `make generate` | Regenerate the committed Windows executable icon resources (`cmd/ultimav/rsrc_windows_*.syso`) after changing the icon tile or sprite atlas; `internal/appicon` tests fail while they are stale. |
| `make validate` | Run standard local validation after final edits. |
| `make quality-gate` | Run the final pre-PR validation gate when required. |

Run focused tests while developing. Use deterministic seeds and callback mocks
for gameplay regressions. Review contracts, failure paths, cancellation, resource
commitment, and save behavior as well as happy paths. Humans review and merge PRs.

## Local release packaging

Release archives are built locally; no GitHub Actions runner or credit is needed.
Use Python 3.12+, Git, and the exact toolchain selected by `go.mod`. Provision that
toolchain and `go mod download` separately while network access is available.
The packaging command is offline and fails if a toolchain, module, or dependency
notice is missing. It uses shipped assets without a frontend rebuild.

From a **clean checkout** of the intended commit, after local validation:

```sh
python3 scripts/package_release.py --revision HEAD --version v0.1.0-preview.1 --output /existing/output-parent/ultimav-v0.1.0-preview.1
```

The version is an example, not an announced release. Supply an explicit version
and revision; the revision must resolve to current HEAD. An existing local tag of
that name must identify the same commit. No tag is required or created. Untracked
and modified files block packaging. The output parent must exist, the output
directory must be new and outside the checkout, and concurrent use of the same
output is rejected. An interrupted process may leave a sibling `.lock` directory;
remove it only after confirming its packaging process has ended.

The script exports the commit to a temporary source tree, builds sequentially with
`CGO_ENABLED=0`, two Go scheduler threads, `-p=1`, and a 256 MiB Go soft memory
limit. These settings reduce pressure; they are not a hard process-tree RSS cap.
Ambient Go flags, workspaces, and architecture tuning are disabled. It produces:

- `ultimav_<version>_darwin_arm64.tar.gz`
- `ultimav_<version>_darwin_amd64.tar.gz`
- `ultimav_<version>_windows_amd64.zip`
- `ultimav_<version>_linux_amd64.tar.gz`
- `ultimav_<version>_linux_arm64.tar.gz`
- `SHA256SUMS`

Only the game executable is included; the optional save converter remains a
separate source installation. Each archive includes an explicit-path launch guide,
installation guidance, version/commit/toolchain metadata and binary checksum,
project/media credits, font companion notices, the audio attribution manifest,
and top-level Go/dependency license notices. Review notice coverage when changing
dependencies, particularly licenses in nested directories. Source game data,
settings, saves, and unrelated checkout files are not copied into the payload.
Go build metadata is trimmed of local source paths; `release.json` records the
commit because the exported source has no Git directory. The build stamps the
version into the binary (`-X main.version`), which `ultimav --version`, the title
screen (V toggles it on the menu frame's lower border), crash logs and saves report. Archive timestamps are
not normalized, so byte-for-byte reproducibility is not claimed.

The final output directory appears only when every build and checksum finishes.
Failures clean up staged output and do not replace an earlier release. Test the
packaging logic with `python3 -B -m unittest discover -s scripts -p test_package_release.py -v`.
Run `make validate` for implementation changes; packaging does not replace the
project's validation gates or platform acceptance.

Before distribution, verify checksums and archive membership, extract into a clean
directory outside the checkout, and invoke the extracted macOS executable by its
explicit path with `--help` and bounded startup. Record interactive original-data
setup, keyboard, graphics, audio, and save/load acceptance separately per platform.
Linux and Windows cross-builds alone are not native acceptance. Initial archives
are unsigned and require the [download guidance](INSTALL.md#downloaded-binaries).

Uploading is a separate, explicitly authorized operation. With a reviewed and
pushed tag, a maintainer can create a **draft prerelease** using `gh release create
<tag> <archive-paths> <checksum-path> --repo bradhannah/UltimaVRedux --verify-tag
--draft --prerelease --title <title> --notes-file <public-release-notes>`.
Enumerate only reviewed artifacts. Existing drafts can receive reviewed files via
`gh release upload <tag> <paths> --repo bradhannah/UltimaVRedux`; avoid overwriting
published artifacts. Tag creation/push, uploading, and publishing are separate
decisions. Release notes must identify the source and honest platform acceptance,
original-data requirements, save compatibility, and relevant known limitations.

## Finding The Right Guidance

| Work | Durable reference |
|---|---|
| Architecture or package boundaries | [Architecture Guidelines](ARCHITECTURE_GUIDELINES.md) |
| Player commands and input | [Input Handling](systems/INPUT_HANDLING.md) |
| Deterministic time or randomness | [Time System](systems/TIME_SYSTEM.md) |
| Event workflows and feedback | [Event Queue System](systems/EVENT_QUEUE_SYSTEM.md) |
| Dialogs and modal feedback | [Modal Dialog System](ui/MODAL_DIALOG_SYSTEM.md) |
| Reusable UI widgets | [UI Widget System](ui/UI_WIDGET_SYSTEM.md) |
| Debug Workbench | [Debug Workbench System](systems/DEBUG_WORKBENCH.md) |
| Original mechanics or formats | [Game Facts](gamefacts/README.md) and [rule conventions](gamefacts/CONVENTIONS.md) |
| Testing and review | [Testing](TESTING.md) and [repository invariants](../AGENTS.md) |
| Documentation maintenance | [Documentation Catalog](README.md) |

For documentation changes, preserve non-obvious ordering, ownership, cleanup,
failure and compatibility contracts. Use canonical rule IDs for original behavior;
do not replace detailed explanations with code-link stubs. Current Redux code and
tests establish implementation, ADRs explain decisions, and Game Facts distinguish
original rules from intentional differences and unresolved behavior.

See [Coding Conventions](CODING_CONVENTIONS.md), [Error Handling](ERROR_HANDLING.md),
and the [Documentation Catalog](README.md) for the complete retained policy and
reference set.
