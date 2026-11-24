# Architecture Guidelines

This document defines current cross-system boundaries. Executable code is the
authority for current APIs; ADRs record why accepted decisions were made; system
documents describe detailed contracts and entry points.

## Layers

| Layer | Primary locations | Responsibility |
|---|---|---|
| Gameplay domains | `internal/game_state/`, `internal/party_state/`, `internal/map_state/`, `internal/ai/` | Authoritative rules, state, and outcomes |
| Shared references | `internal/references/` | Stable game-data identities and reference types |
| Presentation and application | `cmd/ultimav/` | Ebitengine lifecycle, input adaptation, scene orchestration, and rendering |
| Reusable UI | `internal/ui/` | Presentation widgets and text composition |
| Integration services | `internal/display/`, `internal/audio/`, `internal/config/`, `internal/debugging/` | External-system adapters and application services |

Dependencies point toward gameplay and reference domains. Core gameplay must
not import Ebitengine, input, or UI packages. Presentation may depend on core
domains but does not become authoritative for gameplay state.

`make core-dependencies-check` enforces transitive production and test closures
for GameState, party/map state, AI, references, and config, including internal
and external test variants. It also rejects the standard `testing` package and
`test/` packages in their production closures, so the game links no test code. Plain config values no longer initialize display:
each configuration owns its Viper source, while `GameScene` owns display
adaptation. Core styled feedback uses detached `StyledMessage` segments through
`MessageCallbacks.AppendStyledMessage`, not renderer-bound `text.LineBuilder`.

## Gameplay Authority

- Put gameplay mutations in `GameState` or the focused domain that owns the
  state.
- Keep rules independently testable without graphics initialization.
- Let scene code adapt player input, presentation resources, and renderer-facing
  workflows rather than duplicating domain rules.
- Keep Debug Workbench mutations behind the same authoritative operations used
  by production behavior.
- Account for both terrain passability and object occupancy when movement or
  placement crosses those boundaries.

Prefer the smallest existing authority that can enforce an invariant. Do not
introduce a second state model, service, or compatibility layer merely to avoid
using the established domain API.

## Reference Metadata Ownership

Runtime tile and inventory metadata is compiled from typed Go definitions, not
parallel JSON catalogs. Put new properties beside their consuming domain:

- `internal/references/tile_data.go` owns tile names, movement-feedback factors,
  light emission, light blocking, and windows. Index-based interactions and
  vehicle passability remain in the existing tile methods and passability tables.
- Item-family `item_*_data.go` definitions own player-facing lore, sprites,
  pickup text, and scroll durations. Equipment presentation derives from the
  equipment catalog; spell names and descriptions remain in spell metadata.
- `internal/game_state/duration_effect.go` owns effect identities and expiration
  names. Effect icons remain presentation-owned.
- `internal/debugging/tile_annotations.go` and `effect_annotations.go` own
  inspector-only prose and browsing labels. Tile animation labels describe
  authored sprite groups, not generated visual-effect frame counts. They do not
  control rendering, collision, or enemy behavior.

Inspectors such as `cmd/ref_dump` and the Workbench Prepared Spell Inventory read
runtime properties, then add these annotations. Core packages never import
debugging to obtain game facts. Reference constructors return detached maps and tiles so modifications in
one game state cannot mutate another state's definitions.

`cmd/ref_dump` combines runtime tiles with developer annotations for lookup
scripts. Its `u5dump/json/TileReferences.json` output is generated, git-ignored
inspection data, not an editable game definition. Remove unused properties rather than
keeping a second catalog for potential future consumers.

## External Effects

`SystemCallbacks` is the boundary from gameplay into messages, audio, visual
feedback, screen updates, presentation flow, conversations, UI workflows, and
combat events. Construct mandatory callback families as complete named values,
validate them during initialization, and call them directly afterward. A
missing mandatory callback is a programming error, not an optional feature.

`DisplayManager` centralizes logical draw size, platform window and monitor
dimensions, windowed/maximized/borderless-fullscreen requests, and observed
display-change behavior. `GetCurrentSize` means the published logical render
surface; platform-state callers use the explicit window, monitor, or container
size APIs (DIPs). Display scale and actual final-output pixels are separate
metrics. High-DPI conversion uses Ebitengine's cross-platform APIs at this boundary,
not native OS libraries or per-widget DPI multiplication. Core packages must not
query Ebitengine display state directly.

See [SystemCallbacks ADR](ADR/ADR-003-systemcallbacks-decoupling.md),
[DisplayManager ADR](ADR/ADR-002-displaymanager-pattern.md), and [Game UI
Integration](ui/GAME_UI_INTEGRATION.md).

## Time And Randomness

Gameplay outcomes, ordering, and tests use the `GameState` clock and RNG or an
explicitly injected deterministic provider. Cosmetic wall time is allowed only
when it cannot influence gameplay state, ordering, or randomness.

Random call order is part of behavior when original fidelity or deterministic
replay depends on it. Tests should use fixed seeds and explicit elapsed values.

See [Central GameClock ADR](ADR/ADR-001-central-gameclock.md) and [Time
System](systems/TIME_SYSTEM.md).

## Workflows And Feedback

Use a workflow for multi-stage gameplay orchestration, input ownership, and
cleanup. Use the feedback lane for presentation-only effects. Feedback does not
mutate gameplay or own input.

Reusable sequences compose into the owning workflow. Independently started
work is not implicitly awaited. Every owner cleans up the exact resources it
created, including dialogs and transient effects.

See [Event Queue ADR](ADR/ADR-008-event-queue-execution-lanes.md) and [Event
Queue System](systems/EVENT_QUEUE_SYSTEM.md).

## Errors And Logging

- Return wrapped errors for recoverable input, configuration, save, and I/O
  failures.
- Panic for programming errors, impossible states, and violated initialization
  contracts.
- Use `log.Fatal` only at the application boundary for unrecoverable startup or
  resource failures.
- Use the centralized logging package rather than direct console output.

See [Error Handling](ERROR_HANDLING.md) and [Logging System](systems/LOGGING_SYSTEM.md).

## Interfaces And Abstractions

- Define interfaces at the consuming boundary when they improve testability or
  preserve a real package boundary.
- Prefer named configuration or callback structs over large positional
  constructors.
- Avoid speculative abstractions, duplicate managers, and compatibility code
  without a concrete persisted or external consumer.
- Remove obsolete callbacks, adapters, mocks, and tests together after their
  final production consumer is gone.

## Performance

Measure before optimizing. Reuse persistent buffers in demonstrated hot paths,
but do not retain complexity or mutable global state for unmeasured gains.

## Validation

Architecture changes require focused tests for the affected boundary and the
repository validation required by `AGENTS.md`. Review current entry points in
the owning system document rather than copying API inventories into this guide.

The rationale and trade-offs for accepted decisions are indexed in
[Architecture Decision Records](README.md#architecture-decisions).
