# Debug Workbench System

## Purpose

The debug workbench is a browser-based development tool for rapidly discovering commands, applying validated game alterations, managing save snapshots, and setting up focused test situations.

The Go catalog and its exact browser fixture define the current command set. No
text-command compatibility grammar remains; the browser Workbench is the sole
debug interface.

Related documents:

- [Canonical Save Snapshot ADR](../ADR/ADR-009-canonical-save-snapshot.md)
- [Save Snapshot Schema](../details/SAVE_SNAPSHOT_SCHEMA.md)
- [Development Guide](../DEVELOPMENT_GUIDE.md)
- [Debug Workbench UI](../ui/DEBUG_WORKBENCH_UI.md)

## Goals

- Make commands easy to find by intent, category, stable ID, and parameter value.
- Generate friendly controls from typed command schemas.
- Keep all gameplay mutation on the Ebitengine update thread.
- Replace adapter-layer field mutation with authoritative core operations.
- Support immediate, synchronized, workflow, coordinated, and restart-required operations.
- Use canonical `SaveSnapshot` data for immutable checkpoints and rollback.
- Preserve deterministic seeds and structured results for repeatability.
- Retain recent navigation surfaces for fast iteration without bypassing command review.
- Keep the browser connected while `GameScene` is replaced.

## Non-Goals

- A remotely accessible administration server.
- Arbitrary reflection-based editing of `GameState`.
- Serializing transient runtime objects.
- Exact mid-combat save restoration.
- Preserving known defects in legacy debug handlers.
- Supporting multiple controlling clients.

## Technology

### Game Process

- Standard `net/http` server bound to `127.0.0.1` only.
- `github.com/coder/websocket` for the WebSocket transport.
- Embedded browser assets.
- Bounded inbound and outbound queues.
- Top-level control plane owned by `Game`, not `GameScene`.
- `ebiten.SetRunnableOnUnfocused(true)` so browser interaction does not suspend updates.

### Browser Client

- Vue 3.
- TypeScript.
- Vite.
- Native CSS without a component framework or utility-CSS dependency.
- Vue composables and reactive state without Pinia or Vue Router.

## Control Plane

### Persisted player preferences

`combat.presentation.speed.set` (category Combat) is used by the Active Combat
console. Its integer `percent` transport is limited to listed integer `choices`
(50, 100, 150, 200, 300, 400, 600, 800, 1200, 1600 and 2000) within 50–2000 in
steps of 50. Integer choices are validated in the catalog and request path, and
in the browser form. It controls master combat presentation, including
animations and action pauses, through `Gameplay.Combat.SpeedFactor`, using the
same typed `UltimaVConfiguration` getter and atomic setter as player
Configuration; Debug configuration is not a speed writer. An open staged
Configuration dialog rejects the mutation with `configuration_editing` and an
Apply/Cancel explanation. Failed persistence keeps the live model and source
unchanged.

The other persisted Redux preferences (hidden-dungeon discoveries, subdued unlit
visibility, smooth surface movement, flexible sextant use, quick-save
confirmation, combat health bars, action popups and repeated-monster division)
are available only in the game's own Configure dialog; the Workbench no longer
exposes a Redux Modifications console or setters for them.

Numeric parameter metadata includes optional `step`, anchored at `minimum`.
Go validates positive steps and grid membership before dispatch, and TypeScript
forms and specialized controls use those same catalog constraints. Out-of-grid
speed requests fail validation rather than panicking.

```text
Browser/Vue
    |
    | JSON WebSocket messages
    v
Debug server read loop
    |
    | bounded immutable requests
    v
Application debug bridge
    |
    | drained at an update boundary
    v
Game / GameScene command executor
    |
    +--> authoritative GameState or PartyState operation
    +--> event-queue workflow
    +--> snapshot candidate and scene replacement
    |
    | structured results and events
    v
Debug server write loop --> Browser/Vue
```

Network goroutines never read or mutate live gameplay objects directly.

### Execution Ownership

Command ownership follows the lifetime of the resource being inspected or changed:

| Owner | Appropriate commands | Scene replacement behavior |
|---|---|---|
| Application (`Game`) | Process-wide logging, static catalogs, and services that survive gameplay scenes | Execute on the update thread without requiring a gameplay scene |
| Scene (`GameScene`) | Gameplay state, renderer state, dialogs, workflows, and scene configuration | Require the expected scene generation and reject stale requests |

Do not route process-wide commands through `GameScene` merely because one exists. Renderer and gameplay operations must not move to application scope to avoid scene validation.

## Command Contract

Each logical command has a stable definition independent of text grammar:

### Debug Tool Eligibility

Workbench operations are reserved for exceptional state, data that normal
gameplay cannot inspect, or setup that materially shortens development and test
iterations. Routine interactions that are immediately available in game do not
qualify. For additions, define a stable typed command, its owner and availability
reason, revision/review requirements, and detached result before wiring the UI.
Keep Go contracts and TypeScript protocol/forms synchronized. Add catalog/wire
coverage and rejection tests, then rebuild embedded assets with `make web-build`.
Browser controls must not bypass update-thread execution or confirmation.

```go
type CommandDefinition struct {
	ID               string
	Category         string
	Name             string
	Description      string
	Aliases          []string
	SearchTerms      []string
	Parameters       []ParameterDefinition
	ExecutionModes   []ExecutionMode
	Destructive      bool
}
```

`ExecutionModes` advertises the possible modes for discovery and UI planning. The authoritative executor resolves one effective mode from the command arguments and current context. For example, changing equipment may be synchronized live during combat, while changing party membership requires leaving combat and may require scene restart.

Parameter definitions require stable IDs and may represent:

- Enumerated choices.
- Bounded integers.
- Integer lists.
- Booleans.
- Pattern-constrained strings.
- Free text.
- Player characters.
- Locations.
- Monsters.
- Equipment and inventory items.
- Dynamic options provided from an update-thread snapshot.

The schema distinguishes serialized values from display labels and supports required values, defaults, descriptions, context availability, and control hints.

### Structured Invocation

Handlers accept typed arguments. Browser forms invoke stable command IDs directly; no text-console grammar is registered or translated.

### Retry-Safe State Changes

Remote mutations set an explicit desired state instead of toggling server state. Boolean commands accept values such as `enabled=true`, return the resulting state, and remain safe under retries and replay. UI labels may say “toggle,” but the emitted command must be an idempotent setter.

### Results

Commands return structured results rather than writing only to `text.Output`:

```go
type Result struct {
	SessionID              string
	RequestID              string
	CommandID              string
	SceneGeneration        uint64
	Invocation             *CommandInvocation
	EffectiveExecutionMode ExecutionMode
	Success                bool
	Summary                string
	ErrorCode              string
	ErrorMessage           string
	Changes                []StateChange
	Data                   json.RawMessage
}
```

Presentation adapters may render the summary, changes, and typed data differently. The browser must not scrape formatted console output.

## Execution Modes

| Mode | Purpose | Example |
|---|---|---|
| Query | Read an update-thread snapshot | Combat status |
| Live-safe | Apply one focused operation next update | Set gold |
| Synchronized live | Update related runtime structures atomically | Equip a weapon during combat |
| Workflow | Run gameplay orchestration and presentation | Moongate teleport |
| Coordinated alteration | Validate and apply several durable domains | Replace party inventory and statuses |
| Scene restart | Hydrate a detached complete state | Load or broadly edit a save snapshot |
| Presentation | Change game-window/browser presentation without gameplay mutation | Toggle an overlay |

The server resolves the effective execution mode and availability for each request. Client metadata is advisory only.

## Specialized Instruments

Purpose-built instruments handle interactions that are poorly served by
generated forms, but they remain clients of the typed catalog. Companion
queries and mutations use the same stable command IDs, request validation,
update-thread bridge, stale-state checks, structured results, and session
reconciliation as generated controls. Immutable reference catalogs may be
application-owned so they remain available across gameplay scene replacement;
gameplay and renderer operations retain their normal scene ownership.

The current instrument inventory is maintained in [Debug Workbench
UI](../ui/DEBUG_WORKBENCH_UI.md). The Go catalog definitions under
`internal/debugging/` and their exact browser fixture define the current command
inventory; do not maintain a parallel Markdown ledger.

Audio auditioning and direct library editing are separate from the Workbench.
The standalone terminal editor is described in
[Audio System](AUDIO_SYSTEM.md#standalone-audio-editor); it exposes
no browser mixer or game-audio mute commands.

## Safety Rules

### All Mutations

- Validate all expected failures before the first mutation.
- Execute only at an update-thread boundary.
- Use stable identities rather than display names or mutable indexes where possible.
- Emit actual resulting values after clamping or normalization.
- Use centralized RNG and record explicit seeds for repeatable operations.
- Reject stale revision-based edits rather than silently overwriting newer state.

Dynamic provider choices are published as detached immutable discovery snapshots from `Game.Update`. Catalog requests read only the published snapshot, and commands using a provider include its expected options revision. Execution rejects stale revisions and revalidates the selected target before mutation.

Continuous discovery refresh is owned by the running Workbench server, including
when it is listening without a browser. Ordinary gameplay without that server
does not build developer options each tick. The bridge and catalog remain
initialized: dynamic command admission refreshes current options on the update
thread before checking the expected revision. Startup publishes fresh choices
before accepting HTTP readers. Shutdown/server failure retires reviewed options
across the dormant interval; scene replacement still invalidates generations and
options immediately. Network goroutines change only atomic publication demand,
never read option providers or live gameplay state.

Static command definitions are immutable and retained internally per `Game`.
Dynamic provider data is detached at publication, unchanged snapshots are reused,
and external discovery readers still receive deep copies of both definitions and
choices. This sharing does not permit modifying published or borrowed metadata.

### Combat

Selected equipment, HP, MP, and supported status edits may execute live when their authoritative operation synchronizes combat-derived state.

Party ordering, membership, Dexterity, combat location, turn ownership, and broad state replacement are not live-safe. They require combat exit or a scene restart from a safe baseline.

Because snapshot capture is unavailable during combat, restart-required alterations are unavailable until combat exits. The workbench must not offer a restart path that cannot first capture its required rollback snapshot.

#### Combat Target Identity

Each tactical encounter receives a new identity when combat is created or replaced. Each monster receives an immutable ID that is never reused within that encounter. Manager indexes and display names are telemetry only and must not target mutations.

Reviewed combat mutations carry the encounter ID, combatant ID, and expected snapshot or condition. Execution revalidates all three so a delayed request cannot affect a replacement encounter or a combatant whose state changed after review.

#### Reinforcement Admission

`GameState.SpawnDebugCombatMonsters` checks exact manager/queue identities for
both active and historical monsters; matching population counts alone is not
enough. Retired, fled and replaced records can remain in history without staying
in active rotation. Capacity comes from free source slots `6..31`, free slots
in the shared 32-object table, and distinct unoccupied authored spawn positions,
with the existing 16-monster per-request bound. Removing a corpse does not free
its retained dead source slot; released flight/retirement slots can be reused.

Preflight validates the complete batch before changing the seed, consuming RNG
or allocating IDs. An over-capacity exact request fails atomically; automatic
and maximum counts respect available capacity. Occupancy uses exact live source
and clone entries, so a remaining clone still blocks after its source departs.
Underground PCs do not block; protection fields block, while harmful fields may
share a spawn square but still consume object capacity. Loose objects and
corpses also occupy spawn positions. These are authored-position admission
rules, not an additional creature-terrain movement test.

Authority and regression coverage are in
`internal/game_state/active_combat_debug.go` and
`internal/game_state/active_combat_debug_spawn_test.go`.

### Snapshot Loads

- Capture a rollback `SaveSnapshot` automatically.
- Validate the complete stored snapshot and reviewed revision.
- Construct the replacement scene before discarding the current scene.
- Commit scene replacement only after rollback persistence and dormant candidate construction succeed.
- Keep the application-owned WebSocket connected across scene replacement.

## Equipment Transaction

`GameState.AlterEquipmentLoadouts` owns the atomic inventory/equipment
orchestration of the reviewed `party.equipment.loadout.update` Workbench action,
for one character or the whole party. The normal Ready dialog commits through
`GameState.ApplyReadyEquipment` instead. Its current command contract
is defined in `internal/debugging/party_equipment_catalog.go`; authoritative
transaction behavior remains in `GameState` and its tests.

## WebSocket Protocol

Messages use a versioned JSON envelope:

```json
{
  "version": 1,
  "id": "request-42",
  "type": "command.execute",
  "payload": {}
}
```

Client-to-server message types:

- `session.hello`
- `catalog.request`
- `command.execute`
- `command.ack`
- `operation.status`

Server-to-client message types:

- `session.ready`
- `catalog.snapshot`
- `command.accepted`
- `command.completed`
- `command.failed`
- `protocol.error`

Use request IDs to reconcile reconnects and avoid duplicate mutations. Before
WebSocket acceptance, the server validates the request Host, Origin, and
process token and enforces single-controller ownership. Session creation or
resumption occurs afterward through `session.hello`.

Completed command results remain retained by session until `command.ack` arrives. A reconnect that supplies its previous session ID receives pending acceptance and completion messages again. The browser acknowledges every completed delivery, but a request ID already completed returns before changing pending state, scene generation, result history, instrument state, or navigation. A repeated acceptance received before completion is ignored rather than acknowledged because `command.ack` releases a completed server record.

## Backpressure

- The game update never waits for a browser write.
- Each connection has one reader loop and one writer loop.
- Outbound queues are bounded.
- Submitted operations and completed or failed results remain retained by session until acknowledgment.
- Outbound saturation disconnects the slow client; retained operations replay after session resumption.
- Client result retention is bounded, and the UI reports trimmed results.

## Read-Only Log Transport

`/logs` is separate from the single-controller command WebSocket. It receives a distinct read-only token that cannot authorize commands and never acquires a controller session.

Accepted structured entries are retained in one bounded process-wide ring with monotonically increasing sequence numbers. Each reader supplies an independent cursor; reads never advance another reader. Responses report the next cursor, remaining backlog, and records overwritten before the requested cursor. Multiple tabs may poll concurrently, while filters, pause/follow state, and additional client retention remain local to each window.

## Save Snapshots

There is no separate debug gameplay state. The Workbench operates on canonical `SaveSnapshot` values, but it does not expose arbitrary field editing or a general patch protocol.

```text
stable GameScene --capture--> validated SaveSnapshot --> immutable named record
named/rollback record --revision check--> persist rollback --> dormant candidate
                                             |
                                             v
                                  atomic scene replacement
```

Initial snapshot capture is disallowed in transient contexts that are not semantically restorable. The workbench reports the reason and suggests exiting the context first.

The five catalog commands are:

| ID | Behavior |
|---|---|
| `save.snapshot.list` | List detached metadata and current capture/load/delete eligibility. |
| `save.snapshot.get` | Return metadata plus compact location, calendar, moon phase, and party detail; it does not return editable full JSON. |
| `save.snapshot.capture` | Create a new immutable `project:<id>` or `personal:<id>` record from a stable active scene. |
| `save.snapshot.load` | Require the reviewed SHA-256 revision, persist an automatic rollback, build a dormant candidate, and atomically replace the scene. |
| `save.snapshot.delete` | Revision-conditionally delete an unreferenced project or personal record; rollback records are not user-deletable. |

The default roots are `debug/snapshots/`, `~/.ultima_v_redux/snapshots/`, and `~/.ultima_v_redux/snapshots/rollback/`. Override them with `--snapshot-project-dir`, `--snapshot-personal-dir`, and `--snapshot-rollback-dir`. Records use the strict native format described in [Save Snapshot Schema](../details/SAVE_SNAPSHOT_SCHEMA.md), which owns supported versions and migration semantics. They are immutable after creation, canonically hashed, and protected with a shared stable-order advisory lock set (the snapshot reference lock, `.scenario-snapshot-references.lock`, a historical file name that is retained). Personal and rollback storage is private (`0700` directories and `0600` files); project storage remains collaborative. Snapshot compatibility is checked against active runtime references, and unverified or incompatible records cannot be loaded.

Scripted scenarios, durable scenario definitions, scenario run history and the `--scenario*` command-line flags were retired from the Workbench. Snapshots are now used only as checkpoints and as rollback records produced by loads.

## Running The Workbench

The server is disabled by default and never binds outside `127.0.0.1`. See the
[development quick start](../DEVELOPMENT_GUIDE.md#starting-the-debug-workbench) for launch flags and the
default browser endpoint. Frontend source is under
`internal/debugging/webui/client`; `make web-build` type-checks it and refreshes
the committed embedded assets.

## Validation Gates

- Exact zero-registration and Go/browser catalog parity tests.
- Schema and protocol decoding tests.
- Update-thread ownership tests.
- Queue saturation and reconnect tests.
- Fixed-seed command tests.
- Snapshot capture/hydration round-trip tests.
- Snapshot promotion fixture tests.
- Scene replacement cleanup tests.
- `go test -race` coverage for the bridge and server.
- Browser type-check and production build.
- Four Playwright Chromium smoke tests with synthetic transport cover desktop
  startup/navigation/status, one compact layout, mutation confirmation/cancellation
  and visible rejection, and keyboard palette focus/dismissal. Detailed component
  and protocol behavior remains in DOM/Go tests. Browser dock matrices, actual
  desktop zoom and exhaustive workflow matrices are outside this secondary tool's
  acceptance scope. These tests are not live-game mutation or usability evidence.
- Full repository validation for command-catalog, transport, and browser-interface changes.
