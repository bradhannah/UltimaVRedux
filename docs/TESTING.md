# Testing Strategy

This document owns test strategy, fixture selection, and verification procedures.
The Makefile owns executable commands; [ADR-004](ADR/ADR-004-integration-testing-real-data.md)
explains the rationale for real-data integration coverage.

## Principles

- Use standard Go `*_test.go` tests close to the package under test.
- **A test is not a test without at least one valuable assertion**: one that fails
  when the behaviour it names regresses. Logging, "did not panic", restating a
  constant or a test that cannot fail does not count; delete such tests.
- Add a regression test for every bug fix and explain the behavior it protects.
- Keep unit tests independent of copyrighted Ultima V assets.
- Use real game data when parsing, map layout, save state, or cross-system behavior is part of the assertion.
- Drive gameplay randomness through `GameState` and use fixed seeds.
- Drive gameplay timing through game-state clocks and elapsed time, not wall time.
- Validate UI-facing behavior through `SystemCallbacks` mocks rather than presentation packages.
- Test the smallest scope that proves the behavior; do not use full game setup for pure logic.
- SDK tests establish shared mechanisms; Redux tests establish authored policy,
  adaptation, wiring, and owner lifetimes. Do not retain an unused production API
  solely because a test calls it, or copy an SDK implementation into a test helper
  to manufacture expected output. Port missing mechanism assertions upstream
  before retiring duplicated coverage; keep independent game-specific oracles.

## Test Scopes

### Unit Tests

Unit tests cover deterministic logic with local fixtures or small value objects. They should be fast, asset-independent, and suitable for `go test -short`.

Use table tests for related inputs and subtests for cases with distinct setup. Keep package-local helpers in `_test.go` files unless production packages need them.

### Integration Tests

Integration tests cover behavior that depends on real Ultima V files or multiple initialized systems. The primary game-state helper is `NewIntegrationTestBuilder` in `internal/game_state/test_builder_helpers.go`.

The builder can configure location, floor, player position, callbacks, and a deterministic random seed. It loads real references and `SAVED.GAM`, then initializes the selected map. Only genuinely absent optional data skips. Explicitly selected missing files, malformed data/configuration, and loader failures fail with their original diagnostic. `FailIfNoGameData` or `ULTIMA5_REQUIRE_DATA=1` also makes optional absence fatal.

Because `make test-integration` filters test names with `-run Integration`, an integration test intended for that target must include `Integration` in its Go test function name. A filename ending in `_integration_test.go` is useful organization but does not affect Go test selection.

### Adapter and UI Tests

Tests under `cmd/ultimav` may require Ebitengine display initialization. Linux CI runs these tests through Xvfb. Keep core gameplay tests out of the adapter package when rendering or input is not part of the behavior.

Ebitengine 2.10 initializes native window services lazily. `internal/display`
initializes the process display manager in `TestMain`, before test goroutines
can query monitors, as required by Cocoa. Main-screen rendering tests run inside
the native loop through `internal/ui/mainscreen/main_test.go`; application
rendering tests use `cmd/ultimav/graphics_test_main_test.go`. These harnesses,
the widget harness, and the sprite harness launch `m.Run` from the first Update
in a goroutine while the native loop continues to finish frames. Running the
entire suite synchronously inside one Update retains frame-local native upload
buffers. The channel transfers the exit code after completion; `RunGame` stays
on the main thread and initializes native services before tests start. Ebitengine
2.10's `Monitor` API requires the main thread before `RunGame` and is concurrent-safe
afterward. A missing display/backend fails the required render gate; it is never
a reason to skip the package.

Advancing frames also advances live input. Native fixtures must supply detached
held-key and text sources rather than read the live keyboard from test goroutines.
Synthetic application fixtures use `NewKeyboardWithKeyStateSource` with the
test-only `noSceneTestKeyHeld` source, or an explicitly owned held-key map when
testing physical chord/release semantics. `SimulateKeyPress` alone does not isolate
modifier or transition-release polling: a host Ctrl/Shift can alter simulated
dialog input. Fixture regressions cover menu navigation, save selection and guard
answers with controlled modifier contamination. This isolates automated tests;
it does not establish physical-keyboard or native usability acceptance.

### VM Keyboard/Settings Proof

```bash
ULTIMA5_DATA_PATH="/path/to/ultima5" make vm-settings-proof
```

This opt-in Ebitengine v2.10.4 `exp/vmhost` proof builds a host and a tagged
application test binary into `bin/`. The guest uses production `Game`, title
scene, keyboard handling, configuration dialog, and rendering. Its dedicated
`settingsproof,ebitenginevmguest` test entry point adds read-only observations;
it does not replace the normal native test suite.

The host sends Enter to skip the title presentation, navigates to Configure,
opens its sub-menu on Display, tabs from Display to Gameplay, toggles Repeated Monster Division, cancels,
reopens, and returns to Gameplay. Each press and release receives a completed
guest tick. Assertions check the scene, menu choice, dialog identity, staged
and live values, and unchanged configuration-file bytes. Real host-rendered
pixels from the selected row must change with the toggle and exactly return
to the original pixels after reopening; animated background pixels are excluded.
No golden images or original assets are written to the repository.

The host fixes its render surface at 1280×960, disables guest sound, isolates
HOME/configuration/save locations and working directory, and removes the
temporary home after reaping the guest. Connection/idle waits are bounded to
10 seconds and the runtime to 30 seconds; builds are outside that runtime
deadline. Use the resource protection described below for the complete command.
Success requires both host completion and a successful guest assertion exit.

The guest needs no display, but the host still requires native graphics; use
`xvfb-run -a make vm-settings-proof` on headless Linux. This verifies injected
engine keyboard handling and rendered settings rollback. Physical keyboard,
IME/native text-service behavior, platform-window behavior and gameplay replay
require their own acceptance. In particular, the application's existing wall-time input/debounce
and scene timing mean VM tick counts alone do not prove gameplay determinism.

### Regression Tests

Display DPI regression checks distinguish logical window DIPs, scale, canvas
pixels, and observed final output. Cover 100%, 125%, 150%, 175%, and 200%, odd
pixel rounding, 90%-monitor preset eligibility, saved geometry/legacy indices,
scale-only changes, and apply/Keep/Revert. Review Display at title and in-game
placements, including custom sizes, long mode labels, errors, and confirmation.
Native acceptance separately requires Windows 2560×1440 at 150%, macOS Retina
and scaled modes, mixed-DPI movement, resize/maximize/fullscreen/restore, and
restart. Fake-backend, render-harness and VM success are not native DPI acceptance.

A regression test should state the user-visible failure or violated invariant, the root cause when it is not obvious, and why the assertion prevents recurrence. An issue link is useful when one exists. Avoid dates and historical counts in names unless the date is part of the domain behavior.

## Real Game Data

The complete original Ultima V installation is externally provisioned. Retained
bundled assets and fixtures carry their existing notices and do not replace it.
For game-state integration tests, set `ULTIMA5_DATA_PATH` to a directory containing
the required files, including `SAVED.GAM`, `LOOK2.DAT`, `CASTLE.TLK`, and `MISCMAPS.DAT`:

```bash
export ULTIMA5_DATA_PATH="/path/to/ultima5"
```

`test/originaldata.ResolveDirectory` selects `ULTIMA5_DATA_PATH`, then
`ULTIMA5_DATA_DIR`, then repository `OLD`. It never reads HOME or falls back
after a broken explicit selection. Required files depend on the test; selected
data must be complete for those assertions.

To reproduce a legacy import with a save pair separate from the runtime data,
set `ULTIMA5_IMPORT_SOURCE` for the production import integration test. The
source pair is read-only and the selected native slot is written to temporary
storage; the test verifies Slot 7 creation, description/identity persistence,
canonical capture, and gameplay activation. Without this optional source
override, the test uses `SAVED.GAM` from the selected runtime data directory.
This test-only override exercises the conversion/activation transaction; the
title-menu player flow reads only the active data directory and has no path field.

```bash
ULTIMA5_DATA_PATH="/path/to/original-data" \
ULTIMA5_IMPORT_SOURCE="/path/to/save/SAVED.GAM" \
go test ./cmd/ultimav -run '^TestIntegrationLegacyImportCreatesSelectedSlotAndStartsGame$' -count=1
```

### Preview setup and save conversion acceptance

Use isolated HOME/configuration and output directories for setup/conversion
checks. Preserve original input bytes and never write test outputs into an
original installation. The [converter procedure](details/SAVE_CONVERTER.md) owns
human CLI invocation and result loading.

- Setup: unset configuration must ask for confirmation even with one candidate;
  a valid configured path, including historical `./OLD`, must remain usable.
  Exercise GOG/Heroic and resolved home/UltimaV discovery, external editor failure,
  real-file settings recheck, malformed-settings preservation, and next-launch
  activation of title-menu data changes.
- Title import: use only the active folder's exact pair, even with a different
  pending or historical location. Verify Check Again/Back recovery, cancellation,
  stale asynchronous completion, destination selection, description retention,
  explicit overwrite review, persistence failure, and activation rollback.
- Description editor: preserve native text/IME selection, repeat, validation,
  resize, and top-modal focus lifecycle. Redux clipboard adapters/paste hints are
  absent; that is not proof that OS text services cannot paste.
- Converter: cover required flags and exclusive output modes; full-data
  hydration/recapture; preserved input bytes; stderr warnings and the stdout
  receipt (destination, playthrough identity, format/versions, warning count);
  file and slot collision,
  approved overwrite and stale-revision refusal; protected input trees; missing
  parents; and post-publication uncertainty. Focused package coverage belongs in
  `cmd/save-convert`, alongside native repository and conversion tests.
- Load proof: separately load a converted slot using `--player-save-dir`, and a
  lowercase-slug standalone snapshot through the Workbench's selected personal
  directory. Establish stable gameplay first so the latter can persist rollback.
  A successful converter exit is not this interactive acceptance.

[Platform status](INSTALL.md#platform-status) records what has been verified on
each target. Keep build/install results, including `CGO_ENABLED=0`, separate from
native graphics, audio, keyboard/IME, setup, save/load, and update acceptance.

Use default skipping for public asset-independent runs. Private original-data
validation is a mandatory local gate for fidelity changes: set an explicit data
directory and run `make test-provisioned` and `make test-race-provisioned`.
`ULTIMA5_REQUIRE_DATA=1 make validate` makes its logic test scope strict;
use `ULTIMA5_REQUIRE_DATA=1 make quality-gate` for both required scopes, or
`make test-provisioned` for the aggregate zero-skip test gate.
`ULTIMA5_REQUIRE_DATA=1 make test` and
`ULTIMA5_REQUIRE_DATA=1 make test-race` select the same strict test policy.
Strict gates reject every test/subtest skip, including non-data
skips; use a supported, fully provisioned local platform. No hosted private
asset runner is configured. Do not commit the externally supplied original
installation or publish it as a test artifact; retain notices on bundled fixtures.

Configuration tests use instance-owned sources: `NewUltimaVConfigurationWithPath`
uses in-memory preferences and explicit game data without reading HOME or
enabling persistence. `LoadUltimaVConfiguration(configFilePath, dataFilePath)`
reads only the selected YAML file without rewriting it. Tests that exercise
startup's default-file creation isolate HOME with a temporary directory.

## Deterministic Gameplay Tests

Configure the seed before invoking behavior that can consume randomness:

```go
gs, callbacks := NewIntegrationTestBuilder(t).
	WithLocation(references.Britain).
	WithSystemCallbacks().
	WithRandomSeed(12345).
	Build()
```

For lower-level tests, call `GameState.SetRandomSeed` before the first random operation. Advance `GameState.ElapsedMs`, `DateTime`, or the relevant deterministic system explicitly when testing time-dependent behavior.

Call `GameState.FinishTurn` only when the assertion includes end-of-turn processing such as NPC movement, hazards, enemy maintenance, or other turn effects. Do not add it to callback-only or isolated unit tests: unrelated turn processing can obscure the behavior being tested. In multi-command workflows, finish each turn at the same boundary used by gameplay.

## System Callback Tests

`MockSystemCallbacks` in `internal/game_state/mock_callbacks_helpers.go` installs the mandatory callback groups and records messages, audio, visuals, flow, dialogs, and related interactions. Prefer its assertion methods, such as `AssertMessageContains` and `AssertTimeAdvanced`, when they express the contract clearly. Call `Reset` between independent actions in one test.

Assert both state mutation and externally visible feedback when both are part of the command contract. Do not import Ebitengine into core tests just to observe presentation behavior.

## Writing Tests

Keep setup local until several tests need the same valid fixture; extracted
helpers call `t.Helper()`. Diagnostic logging belongs behind `t.Log`/`t.Logf`;
retain a diagnostic test only when assertions enforce a useful verdict. Calling
the code without checking its outcome is not regression coverage.

Assert authoritative before/after state, rejected and cancelled paths, resource
effects, and required callbacks. Persisted changes need round-trip coverage and,
when the supported contract includes it, migration coverage. Prefer detached
structural snapshots (tile IDs, frames, positions) over brittle pixel snapshots;
use pixel assertions when the rendering operation itself is the contract.

### Fixture Entry Points

| Scope | Start here | Important boundary |
|---|---|---|
| Real-data gameplay | `internal/game_state/test_builder_helpers.go`: `NewIntegrationTestBuilder`, `WithLocation`, `WithFloor`, `WithPlayerAt`, `WithRandomSeed`, `WithSystemCallbacks` | Configure before the action; original-data failures are not passing early returns. |
| Isolated callback contract | `internal/game_state/mock_callbacks_helpers.go`: `NewMockSystemCallbacks`, `ToSystemCallbacks`, assertion methods, `Reset` | Use the builder's callbacks when it already owns setup. |
| Semantic conversation | `internal/conversation/semantic_engine_test.go`: `semanticTestConversation`, `semanticTestCallbacks`, `TestSemanticConversationEngineProtocolFlow` | All gameplay conversations and inspection use the semantic catalog. |
| Authentic semantic import | `internal/conversation/extractor/` and `internal/conversation/catalog/` tests | Test extraction and the patched runtime catalog when original content matters. |
| Command dispatch | `cmd/ultimav/gamescene_command_context_routing_test.go` and map-specific input tests | Prove primary routing and completion ownership separately from terminal authoritative effects. |

An integration test normally follows this shape, using the action and assertion
appropriate to its fixture:

```go
gs, callbacks := NewIntegrationTestBuilder(t).
    WithLocation(references.Britain).
    WithPlayerAt(15, 15).
    WithSystemCallbacks().
    WithRandomSeed(12345).
    Build()

result := gs.ActionLookSmallMap(references.Up)
assert.True(t, result)
assert.NotEmpty(t, callbacks.Messages)
```

### Semantic And Real-Data Conversation Fixtures

Use `semanticTestConversation` and `semanticTestCallbacks` for deterministic
protocol/action coverage. For original content, import with `extractor.Import`
or load the production patched catalog with `catalog.LoadOriginal`, resolve the
one-based source/dialog identity, and initialize the semantic engine with a
distinct actor slot. Do not infer actor-state identity from a dialog number.

`semantic_engine_original_test.go` covers all 135 imported conversations with a
bounded smoke path; `semantic_engine_regressions_original_test.go` covers complex
name, donation, wait, and recruitment paths. Only absent optional installations
skip; selected-data or decoding failures must fail with the original cause.
Use provisioned mode to require this evidence rather than counting optional
skips as coverage. See the [conversation system](systems/LINEAR_CONVERSATION_SYSTEM.md).

The developer NPC lookup script's semantic text projection and identity joins
have asset-independent tests: `python3 -B scripts/test_npc_lookup.py`.

### Automated Versus Manual Evidence

Injected primary-handler tests prove routing, retained input, and time/action
ownership. Focused core/workflow tests prove terminal effects. Neither proves
physical keyboard behavior, readable presentation, or audible cue ordering.
Record those observations in the relevant issue after a real game-window run;
do not keep a second completion ledger in a system guide.

## Commands

### Resource-Sensitive Runs

Do not launch concurrent validation commands or overlapping agent test jobs on a
memory-constrained workstation. Start with one relevant package, an explicit
timeout, and serial execution. For example:

```bash
GOMAXPROCS=1 GOFLAGS=-p=1 GOMEMLIMIT=512MiB \
go test ./internal/map_state -run '^TestDaylight' -count=1 -parallel=1 -timeout=60s
```

`GOMEMLIMIT` is a soft Go-runtime target, not an operating-system memory cap. It
does not bound all native/GPU allocations or the combined footprint of child
processes. After an unexplained memory incident, use a separate process-tree
watchdog with native-memory accounting and termination on budget overrun or
launcher cancellation before attempting larger runs. Stop if that protection is
unavailable or triggered; do not retry unbounded or report an interrupted gate
as passing. The canonical gate and its required checks remain unchanged.

Keep failure diagnostics bounded too. Avoid handing live `GameState` graphs or
back-reference-bearing values such as `Lighting` to an automatic structural
diff formatter. Compare scalar values or binding identity directly; for large
detached snapshots, retain the complete equality check but report a bounded
failure message rather than formatting the entire snapshot. Use fail-fast
assertions for invariant failures before continuing expensive loops.

### Canonical Commands

The `Makefile` is the command authority:

| Command | Purpose |
| --- | --- |
| `make tools` | Networked provisioning: download modules and install pinned Go tools into `bin/tools`. |
| `make python-tools PYTHON_BOOTSTRAP=python3.12` | Separately provision the hashed Black/Pylint environment using an installed Python 3.12+ interpreter. |
| `make python-format` / `make python-format-check` | Apply stable Black formatting or check the full Python inventory without mutation. |
| `make python-lint` / `make python-check` | Require zero enabled Pylint diagnostics, or run both Python format and lint checks. |
| `make test` | Run uncached logic JSON tests without a display, with the complete package inventory and other required scope assignments. |
| `make test-render` | Run required native/Ebitengine-linked tests; use Xvfb on headless Linux and a native backend on macOS/Windows. |
| `make test-all` | Run the aggregate logic and render gate sequentially. |
| `make test-short` | Run `go test -short ./... -timeout 90s`. Only tests that honor `testing.Short()` are skipped. |
| `make test-integration` | Run tests whose names match `Integration` across all packages. |
| `make vm-settings-proof` | Run the opt-in native-host/VM-guest keyboard and rendered settings rollback proof. |
| `make test-race` | Run all Go tests with the race detector. |
| `make test-provisioned` / `make test-race-provisioned` | Require explicit original data and zero test/subtest skips, without/with the race detector. |
| `make lint-config-check` | Verify pinned v2 schema and detect both intentional errcheck fixture violations. |
| `make lint-all` | Run the pinned configured linters across all packages without per-linter caps, same-message caps, or line deduplication. Existing path exclusions and production-only test handling still apply. |
| `make core-dependencies-check` | Check transitive production and internal/external-test dependency closures for all six core roots. |
| `make web-build` | Install locked frontend dependencies, run frontend tests, and build Debug Workbench assets. |
| `make web-test-browser` | Build current source UI, then run four real Chromium smoke tests with mock transport; headless shell must already be installed. |
| `make check` | Run formatting checks, lint, vet, and logic Go tests. |
| `make pre-commit` | Run `scripts/pre-commit-checks.sh`. |
| `make policy-check` | Run the script's architecture/input/logging policy checks without repeating Go formatting, vet, build, or tests. |
| `make docs-check` | Validate retained Markdown links and anchors, catalog coverage and ownership, and documentation authority boundaries. |
| `make gamefacts-check` | Validate the public Game Facts catalog, canonical rule IDs, relationships, and public references with provisioned Python 3.12+. |
| `make tooling-test` | Run developer-tool regressions, including real failing Black/Pylint fixtures and isolated Git/hook checks. |
| `make validate` | Run `check`, `policy-check`, `build-check`, documentation, public Game Facts, and developer-tool validation locally with `GOPROXY=off`, `GOSUMDB=off`, and `GOTOOLCHAIN=local`. Missing prerequisites fail, not download. |
| `make quality-gate` | Run validation, render tests, frontend build/browser tests, and serial debug race tests once each, with named stage timings. |

Useful focused Go commands:

```bash
go test ./internal/game_state -run '^TestSpecificBehavior$' -count=1
go test ./internal/game_state -run 'Integration' -v
go test ./internal/debugging/... -race
```

On a headless Linux host, wrap Ebitengine-dependent tests with Xvfb:

```bash
xvfb-run -a go test ./cmd/ultimav
```

Provision separately before offline validation: `make tools` installs
`goimports@v0.37.0` and `golangci-lint@v2.12.2`. Python 3.12+ is required for scripts
and tests; `make python-tools` installs Black 26.10.1 and Pylint 4.0.4 from the
hashed lock into `.cache/python-quality`. Set `PYTHON_BOOTSTRAP` explicitly when
ambient Python is older. All Make Python checks use that environment and never
install tools during validation.
The Go-driven audio-module bridge test also selects the platform-specific
`.cache/python-quality` interpreter and checks its readiness before execution;
it never selects ambient `python3`.

Node/npm is needed for frontend builds and frontend-specific validation; it is not
a player prerequisite. Provision the module's selected Go
toolchain (`go 1.27.0`, `toolchain go1.27.1`) before offline validation.
Race-instrumented tests require a native C toolchain, separate from the
C-compiler-free player installation. Browser provisioning is separate from testing
(`npx playwright install --with-deps --only-shell chromium` from the frontend directory after
dependency installation). `make validate` does not install frontend dependencies
or browsers.

Python checks include every tracked and nonignored new source/test file, with no
debt baseline. `make python-check tooling-test gamefacts-check` is the focused
Python lane. Regression fixtures invoke the actual pinned tools and require
formatting errors and lint errors to fail without source mutation. Isolated Git
fixtures cover new/ignored/tracked files, deletions, symlinks, partial staging,
executable modes and the native hook's single check route. No hook is installed
by these tests. Existing graph, lookup, packaging and Go-lint-routing assertions
remain in the same unittest discovery suite.

Aggregate formatting/lint reaches the Python checks once through Make's shared
prerequisites; the quality gate retains its single `make validate` invocation.
The native hook's full route additionally checks Python/config/runner index
parity before and after checks, and fails without formatting or staging anything.
The policy-only aggregate path omits those repeated checks. See
[Python quality workflow](DEVELOPMENT_GUIDE.md#python-quality-workflow) for the
effective Pylint exit policy, provisioning and tool-identity contract.

Frontend DOM tests run one worker without parallel test files; Chromium tests
also use one worker. This bounds concurrent fixture/browser lifetimes in the
local quality gate without excluding tests. These settings do not limit runtime
Workbench behavior or replace host memory supervision.

Frontend type checking runs four serial `vue-tsc` processes: application sources,
root-component tests, other DOM tests, and browser tests/configuration. The
canonical `tsconfig.json` remains the complete source inventory and compiler-option
authority. `typecheck.mjs` includes Vue roots in that inventory, rejects missing,
duplicate, unexpected or empty partitions, and requires identical compiler options
in every partition. Each process checks its full imported source/declaration
closure; `skipLibCheck` is not enabled. The 512 MiB old-space and 8 MiB semi-space
settings encourage bounded V8 collection, but are not process RSS caps. `npm test`
also runs the partition-accounting regressions before the existing DOM suite.

The browser command first builds the current source into ignored `browser/.build`,
then starts Playwright. Its global setup serves those fresh assets on loopback
inside the runner process; it never reuses an existing server or the checked-in
embedded bundle. Teardown closes the server and removes the temporary build.
This avoids retaining Vite's compiler/transform graph beside Chromium. Tests use
the same application production build as the embedded Workbench, with synthetic
transport; Vite development-server/HMR behavior is not covered by this suite.

Playwright's runner/workers use 64 MiB old-space and 2 MiB semi-space settings.
The secondary Debug Workbench has a deliberately small browser acceptance scope:
four smoke tests in `browser/workbench.spec.ts` cover desktop startup/status and
navigation at 1280px, reachable inventory controls at 800px, explicit mutation
confirmation/cancellation with visible server rejection, and keyboard palette
navigation/Escape/focus restoration. They use bundled real Chromium headless shell
with Playwright's default browser launch settings and screenshots only on failure.
Full Chrome, custom Chromium feature overrides, extensions, persistent profiles,
actual desktop zoom, dock matrices and exhaustive scenario browser workflows are
not required. This is an intentional coverage reduction, not equivalent acceptance
of those removed cases. Existing DOM/Go tests retain detailed behavior checks;
two focused App DOM regressions protect stale scenario Start and delayed rollback
review revocation. The synthetic transport never performs live game mutations or
certifies the interface's live usability. These execution settings still require
aggregate process-tree supervision on constrained hosts.

`lint-config-check` verifies the actual configuration, rejects any revision
cutoff (`issues.new*`), then runs it against a fixture. It requires two errcheck
findings (unchecked type assertion and blank-assigned error) and clean checked
controls. `core-dependencies-check` uses `go list -deps -test`
including import remapping for GameState, party/map state, AI, references, and
config; missing/incomplete metadata fails closed.

`test-gate -suite=logic|render|all` accepts only those closed scopes (default: all).
It inventories every `./...` package, uses `partitionPackages` to classify the
production and generated test-binary dependency closures, then compares the
selected package inventory with uncached `go test -json -count=1` events. It prevents
inherited selection flags from hiding assertions, and reports each skipped
test/subtest and reason. Raw JSON goes to stdout and the inventory to stderr;
the gate creates no test-output artifact. A process success without the expected
package/assertion inventory is not sufficient. Unselected packages are reported
as assigned to the other required scope, not missing or passed. Selected but
unexecuted packages remain missing and fail. Summary totals cover only selected
packages, and optional original-data skips remain observations rather than
fidelity acceptance. Inherited GOFLAGS build settings remain intact; explicit
`-run`, `-skip`, `-list`, `-bench`, `-fuzz=`, count, short, race, and concurrency
flags prevent inherited selection or fuzz campaigns from changing the gate.

The next link identifies the **initial SDK cutover checkpoint**. For current
runtime/tools pins and maintained guidance, see the [Redux audio guide](systems/AUDIO_SYSTEM.md).

Berad Audio lives in the [separate SDK repository](https://github.com/bradhannah/berad-sdk/tree/9e7e3ca45b4616f5f7b1e8c177a756f3f224e1e0/modules/berad-audio).
`scripts/check-audio-module` runs `scripts/audio_module_gate.py` from the logic
lane. It copies the game-owned `testdata/berad-audio-redux` bridge into a temporary
directory, disables workspaces/network access and substitutes only the candidate
game. Both the fixture requirement and selected SDK build-list version must match
the application's published pin exactly; SDK replacements are rejected. It tests
real Redux JSON routing with synthetic audio, checks package completion and zero
skips, and is distinct from the SDK's unrelated-consumer proof.
The bridge also exercises Redux's SDK authoring project, exact slider/target policy,
source-bound edits, Save/Undo/reopen and read-only no-op formatting. It verifies
both declared and actually selected runtime and tools pins, rejecting unintended
Go module-selection upgrades. The transferred TUI/session and catalog regressions
are SDK-owned; the game does not rerun them. Old low-level playback/authoring
implementations and duplicate suites have been retired after mapping their
contracts to SDK mechanism tests and live Redux adapter/policy tests.

Root lint checks this bridge with unfiltered pinned rules. Only this throwaway
fixture omits the blanket local-replacement prohibition, after checking its sole
game substitution. Analyzer caches are scoped to the temporary directory.
`make audio-module-check` combines these focused game-adapter checks. SDK tests,
vet/build and independent-consumer/race lanes run in SDK with its selective result
reuse; they are not repeated by game gates or dependent on a sibling checkout.

The live scene adapter uses the real pinned module revision.
`TestGameBuildCannotContainLegacyPlaybackEngines` rejects retired implementations
in production **and test files**, including `legacy_*` copies. Live facade, world
grouping, semantic silent-slot, footstep, burn-tail and scene tests cover Redux
policy. Pure music-policy regressions retain fake transport/clock injection;
integration assertions now use `GameAudioManager`, never a copied old facade.
Publication/history/analysis regressions live with SDK authoring, terminal/session
and backend contracts with SDK tools, and PCM contracts with SDK `internal/pcm`.
The accelerated lifecycle/heap and signed-measurement tests were transferred to
SDK public-runtime/catalog tests rather than discarded with the old implementation.
Race-instrumenting the harness instruments the game bridge and its dependencies;
the complete quality gate includes it in the race stage. These synthetic transport
and WAV tests do not initialize a native device or establish listening acceptance.

Berad Text's independent font/layout/cache and external-consumer proof is SDK-owned.
Its declared native-render lanes use a running Ebitengine test harness and synthetic
bitmap/Go-font fixtures. The game pins the published module and keeps adapter tests
in `internal/text` plus widget/scene regression coverage. Its text TestMain also
starts Ebitengine, permitting actual pixel comparisons to the former game raster
canvas, scaled-text reuse and bitmap/sign behaviour. Generic cache tests were
replaced by SDK coverage, not retained as a second implementation. Focused game
checks include `go test ./internal/text` and the applicable native widget tests;
the normal test gate classifies these by their engine dependency. A renderer/input
package cannot enter core gameplay through the new SDK path. Game checks require
no sibling SDK checkout and do not rerun its independent suite.

Native pixel tests must use bounded offscreen targets, explicit owned-image
cleanup, and a positive visual oracle. Keep broad state/layout/resolution matrices
in non-raster assertions, with only representative native cases for clipping,
composition, shaders, and fractional sampling. In application tests,
`newPresentationRaster` creates standalone offscreen storage: a temporary large
canvas must not share an atlas that persistent tiny sprites could keep alive.
Do not enlarge a fixture merely to test pointer identity, borrowed ownership, or
source-file dimensions; those assertions have smaller or CPU-only counterparts.

### Resource-Bounded Runs

Conversation test callbacks simulate elapsed pause time instead of sleeping.
Their timing assertions inspect the simulated clock, while production adapter
tests remain responsible for real pause scheduling. `make validate` executes one
uncached logic test pass; it does not repeat the suite through standalone
pre-commit. The standalone `make pre-commit` command retains its full checks.

Do not run broad suites concurrently. Prefer a focused package/test selection
with bounded build and test concurrency, especially for asset-heavy adapter tests:

```bash
GOMAXPROCS=2 GOMEMLIMIT=256MiB go test -p=1 -parallel=1 ./internal/game_state -run '^TestSpecificBehavior$' -count=1
```

For a deliberately constrained canonical run, use
`make validate TEST_PROFILE=constrained`. Normal `make validate` uses the host
profile described below. `GOMEMLIMIT` is a per-process soft Go-runtime memory
target, not a hard system-memory cap; graphics/native allocations are not covered.
Full asset-backed race runs need separate resource planning and should not run
beside other test suites. An aborted or timed-out run is not validation evidence
for the tests it did not finish.

### Explicit Resource Profiles

Canonical test/validation Make targets default to `TEST_PROFILE=host`.
`constrained` and `throughput` remain explicit opt-ins. The direct inventory
command accepts the corresponding `-profile` flag and also defaults to `host`.
Host leaves the outer `go run` bootstrap, vet, and build commands at normal Go
package-compilation defaults; the bounded profiles retain bootstrap `-p=1`.
Make stages remain serial, independently of concurrency inside a Go command.

| Profile | Go CPUs / soft memory per process | Logic scope | Native/Ebitengine-linked scope |
|---|---|---|---|
| `host` (default) | Go automatic CPU selection / no injected memory target | `-p=N -parallel=N`, where N is available Go CPUs | `-p=1 -parallel=1` |
| `constrained` | 2 / 256MiB | `-p=1 -parallel=1` | `-p=1 -parallel=1` |
| `throughput` | 4 / 2GiB | `-p=2 -parallel=2` | `-p=1 -parallel=1` |

```bash
make test
make test-provisioned TEST_PROFILE=host
ULTIMA5_REQUIRE_DATA=1 make validate
make quality-gate TEST_PROFILE=constrained
```

Host injects neither `GOMAXPROCS`, `GOMEMLIMIT`, nor `GOGC`. It uses Go's current
`runtime.GOMAXPROCS(0)` automatic CPU selection (including runtime-supported CPU
quota/affinity limits), or a positive explicitly selected `GOMAXPROCS`. Explicit
inherited or Make command-line resource settings remain effective and are reported;
host does not silently replace them with the old two-CPU/256MiB/50 settings.
Absent overrides, memory and GC retain normal Go defaults (`GOMEMLIMIT=off`,
`GOGC=100`). This policy enables a host-resource performance experiment; it is not
evidence of a speedup or an aggregate native-memory bound.

For constrained/throughput, absent CPU/memory settings receive the selected
defaults. Explicit inherited or command-line `GOMAXPROCS`/`GOMEMLIMIT` must match
that profile (canonical `256MiB` or `2GiB` spelling); conflicts fail rather than
silently overriding a budget. Unset conflicting values before selecting another
bounded profile. Both bounded Make profiles default `GOGC` to 50 and preserve an
explicit caller value. These exports apply to validation, not game execution.
Caller GOFLAGS build settings, including persisted GOENV settings, are retained;
explicit test-gate arguments enforce lane concurrency and the existing
anti-filter/uncached/race/short rules. A caller-supplied compilation limit remains
effective outside the test lanes.

Make reports selected resource settings before canonical work. The test gate
reports the effective profile, CPU selection, logic/native package and test
parallelism, memory target, and GC settings to stderr; stdout retains raw test
JSON. Only resource settings are included in these diagnostics, not the complete
environment or original-data paths.

The aggregate gate first runs logic packages, waits for completion, then runs the
render scope serially. Host and throughput raise test concurrency only in the logic scope.
Classification uses Go's production and generated test-binary dependency
closures, including internal/external tests and
transitive Ebitengine subpackage imports. It is not a hardcoded list of three
UI packages. All Ebitengine subpackages, including audio, conservatively assign
the package to render: this is an honest linked-dependency scope, not a claim
that every assertion in that package needs native rendering.
Missing/incomplete dependency metadata fails closed. The original
repository inventory, including no-test packages, remains authoritative; a failed
lane leaves unexecuted work visibly incomplete. Test-level concurrency applies
only to tests that explicitly opt into `t.Parallel`; changing the profile does
not make other tests parallel.

Host lint resolves the complete requested package inventory once, then runs the
pinned configured linters in one invocation over that exact inventory. Linter
concurrency uses its available-CPU/quota-aware automatic default, or explicit
positive `GOMAXPROCS`. Cross-package duplicate detection runs once within that
same full invocation. Constrained and throughput lint retain serial four-package
batches with `--concurrency=1`; those batches defer duplicate detection to one
full-inventory `dupl` pass so cross-batch duplicates cannot disappear. Ordinary
findings do not stop later batches; cancellation does. These scheduling choices
do not change configured linters, package selection, or exclusions, or the uncapped `lint-all` policy. The lint planner's
`--profile=host|constrained|throughput` defaults to `TEST_PROFILE`, then `host`;
Make passes its selected profile explicitly.

Large native application packages (main packages with at least 128 internal test
files) compile once and execute in at most seven serial feature contexts: combat,
dungeon, surface rendering, astronomy, title/creation, dialogs, and remaining workflows.
Each context is one native process for the whole feature group, not one per test.
The gate discovers every top-level test, example, and fuzz seed workload from the
compiled binary; every discovered name is assigned exactly once. It verifies all
top-level terminals, retains subtest failures/skips in the aggregate inventory,
and rejects omissions, unexpected assertions, duplicate execution, and nonzero
process exits. No discovery run is counted as validation. All contexts use the
package's original working directory and preserve the selected race/short/data
policy. Temporary test binaries are removed after the pass.

Direct constrained native contexts default `GOGC` to 50 when the caller has not selected
a value. This makes unreachable Go owners eligible for native resource cleanup
sooner; it neither forces GC after each test nor caps driver memory. Explicit
caller settings remain visible and unchanged. The 256 MiB soft Go memory target
is still separate from measured process and native-memory budgets.

Host native contexts do not add a GC or heap target; they keep serial execution
and preserve ordinary Go defaults or explicit caller settings.

Make recipes are serialized even under inherited `-j`; this does not prevent two
separate validation commands from overlapping. The final quality-gate debugging
race pass retains conservative package/test concurrency. Do not run multiple
gates or agent test jobs together. A 2GiB target is per process, not an aggregate
native-memory cap. Use external supervision and host headroom assessment; the
profile is not a memory watchdog. Interrupted or unexecuted lanes are not
validation evidence.

### Quality-Gate Stage Progress

`scripts/quality-gate.sh` emits `STAGE START <name>` and
`STAGE END <name> status=<exit> DURATION=<seconds>s` to stderr for these ordered
stages:

1. `validate`: exactly one canonical `make validate`, including format, lint
   schema/fixtures and package lint, vet, logic tests, policy/dependency checks,
   build, documentation, Game Facts, and tooling regressions.
2. `native`: one `make test-render`, under Xvfb on Linux; existing native cohort
   names and assertion/package inventories remain visible within the stage.
3. `frontend`: one `make web-build`, including its install, test, and build steps.
4. `browser`: one `make web-test-browser`.
5. `race`: the uncached, serial race pass over `./internal/debugging/...` and the
   independent Berad Audio harness, under Xvfb on Linux.

Durations measure each actual command invocation in whole seconds. The validate
duration includes its subtargets; the script does not claim separate subtarget
timings or rerun them to collect timings. A failing stage reports its nonzero
status and stops subsequent stages. Interrupt/termination handling exits nonzero
(130/143); incomplete stages or missing END records are not passing evidence.
Stage markers contain fixed stage names, timing and status, not command arguments
or arbitrary environment values.

## Continuous Integration

`.github/workflows/test.yml` currently defines the CI scope:

- The Debug Workbench frontend runs `make web-build`, checks embedded asset parity, provisions Chromium, then runs `make web-test-browser`.
- Debug control-plane race tests run on Ubuntu under Xvfb for `./internal/debugging/...`.
- The bounded logic matrix runs every logic-assigned package on Linux, macOS,
  and Windows, with DISPLAY/WAYLAND_DISPLAY cleared and core dependency checks.
- The required render matrix runs every Ebitengine-linked package on Linux under
  Xvfb and macOS/Windows using native backends. Backend failures fail the job.
- Production builds target Linux, macOS, and Windows with `CGO_ENABLED=0`, using
  shipped embedded/generated assets without a generation step. A successful
  build is not native graphics, audio, or input acceptance.
- Ubuntu quality checks provision tools and Workbench dependencies separately,
  require the Go/TypeScript scenario wire assertion with
  `ULTIMA5_REQUIRE_WEB_CONTRACT=1`, then run `make validate` without a display.
  This runs logic and static checks, not a second Linux native/full-suite pass.
- Go versions come from `go.mod`; the workflow does not duplicate a version literal.

These are public asset-independent jobs; optional-data skips remain visible.
They do not replace mandatory local strict private-data validation. Browser
tests exercise real DOM/input/layout with synthetic transport at desktop and
compact-laptop widths, not live game mutations. The four-test smoke scope above
does not establish full Workbench usability, desktop zoom, or physical-keyboard/
game-window acceptance.

## Test Review Checklist

- The test name describes behavior and includes `Integration` when target selection requires it.
- The test fails for the bug or missing behavior before the fix.
- Randomness and gameplay time are controlled.
- Real data is used only when the behavior depends on it.
- Missing data skips or fails intentionally.
- Callback assertions cover user-visible behavior without UI coupling.
- `FinishTurn` is called only at authentic turn boundaries.
- Assertions target stable behavior rather than incidental positions, logs, or implementation details.
- The focused test and the appropriate Make target pass.
