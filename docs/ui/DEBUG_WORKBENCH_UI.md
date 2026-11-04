# Debug Workbench UI

## Purpose

This document defines the browser workbench layout and primary workflows. The interface prioritizes rapid keyboard-driven debugging, generated controls, clear safety consequences, and repeatable save/scenario operations.

The visual language should feel related to Ultima V without imitating an in-game medieval interface. Use a dense instrument-panel layout, restrained EGA-inspired semantic colors, thin blue borders, and monospaced values. Avoid generic dashboard cards and excessive whitespace.

Related documents:

- [Debug Workbench System](../systems/DEBUG_WORKBENCH.md)
- [Save Snapshot Schema](../details/SAVE_SNAPSHOT_SCHEMA.md)

## Current Implementation

The responsive query/mutation shell and first intent-based workspaces are implemented. They include:

- Home, Game, Codex, System, Tools, Scenarios, and Saves navigation while preserving the authoritative command catalog underneath.
- A visually dominant workspace tab rail with clear selected-state treatment.
- Every browser launch opens Home. The selected instrument, collapsed Global History state, bottom/right history dock, and navigation recents remain browser-local across refreshes; stale instrument and recent-surface identities fall back through catalog validation.
- Visible-only silent refresh for Logging, Wind, Calendar, World Navigator, Moongates, Monster Ecosystem, Active Combat, Vehicle, Inventory Locker, Prepared Spells, Search, Quest Inventory, and Shrine progression plus revisioned party-editor metadata refresh, without flooding global history.
- Automatically loaded Combat Map reference tables in Codex and named virtue navigation instead of numeric Shrine entry.
- Palette, Home, and command-rail navigation automatically load the selected read-only reference. Failed reference loads render an explicit error instead of an indefinite loading state.
- Connection, scene-generation, retry, and session-resume indicators.
- A masthead Britannian calendar showing the live formatted game date/time, moon-phase label, and authentic runic phase glyph from the shared `runic_font.png` sprite sheet.
- Correlated accepted/completed/failed command handling with replay deduplication.
- Typed wind, calendar, world-navigation, moongate, ecosystem, vehicle, inventory-locker, encounter-composer, search/item-stack, quest-inventory, logging, combat-map, shrine, and party result views, plus schema-generated fallback controls.
- Strict Shrine ownership: Codex renders immutable mantra/word/location/reward reference, while Game renders current quest/ruined state and explicit alterations. Cross-navigation preserves the selected shrine.
- A specialized Wind console with automatic telemetry, pause/resume, age, explicit direction selection, and Apply.
- A specialized Combat Map catalog that loads and filters in place without exposing a manual query form.
- A Calendar chronometer with staged practical presets, bounded exact controls, and derived lighting/moon context.
- A five-cell Search cross with complete stack inspection, stable provision placement, and confirmed one-time-progress reset.
- A fixed Quest Inventory ledger with all 14 item identities, canonical placements, and confirmed per-item removal.
- Three independent Game party instruments: Character Editor for one active character's attributes/resources/condition/order, Equipment Loadouts for reviewed six-slot atomic changes, and Party Roster for active/inactive composition plus reviewed party-wide operations. User-facing active positions are one-based; zero-based roster indices remain secondary diagnostic identities.
- A top-level Scenarios workspace that defaults to Library, groups Project / Shared and Personal / Local definitions, and opens a selected definition in Details.
- Revision-safe create, update, delete, and duplicate flows with explicit project/destructive review, stale-revision rejection, and no-overwrite destinations.
- Edit Scenario controls for schema-v2 `current`/`init`/named-snapshot baselines, typed launch parameters, ordered command/note steps, direct scenario-safe catalog command addition, and catalog-driven typed literal or launch-parameter argument controls.
- Immutable Quick Setups for reviewed camping and death/resurrection utilities; these are built-in utilities, not saved scenario scripts.
- Revision- and scene-bound browser Run Review with typed launch values and an execution plan, plus bounded durable Run History for queued/running current status and completed per-step results.
- A Saves workspace grouping Project / Shared, Personal / Local, and Rollback / Recovery records, with detached compact detail, stable-scene capture, reviewed load/delete, automatic rollback disclosure, and reviewed named-baseline assignment to the active scenario draft.
- Global History capture of server-normalized typed invocations, with a persistent masthead active-scenario target, per-result Add to Scenario, and target-bound RECORD ON/OFF controls for eligible successful mutations.
- A World Navigator that preserves staged destination drafts while live status silently refreshes their reviewed source preconditions. It submits one atomic exact or authoritative route placement across large, small, and dungeon maps. Dungeon routes expose outside entrance, canonical inside start, and the non-Doom Underworld-exit ladder; unavailable route targets remain visible with a reason. Physical shrine/Codex setup remains a separate lane; Spirituality and waterfall transitions remain separate workflows.
- A Moongate phase atlas with all moonstone records, reviewed production travel, coordinate-destination handoff into the Navigator, and a coordinate-free Shrine of Spirituality route during 00:00-00:09.
- The Monster Ecosystem roster uses bounded keyboard-accessible scrolling with sticky headings; short reference tables remain unbounded.
- A reviewed, map-first Combat Encounter Composer. A searchable battlefield browser lists Britannia maps and dungeon rooms grouped by level with their map cell. An 11x11 tile preview shows party and monster slots, concealing squares, and push triggers; the party's entry side is chosen on the map's edge. A placed dungeon room can load its **authored contents** exactly as walking in would, always live and without changing the save's room progress; otherwise **custom monsters** select one family with Auto/Max/Exact count, friends, and an optional deterministic seed. A one-line summary is reviewed before Start.
- A comprehensive Logging Control instrument with master/stdout switches, typed minimum level, explicit category channels, and a handoff to the read-only log window.
- A separate read-only log window with level/category/text filters, pause/follow controls, overwrite reporting, and support for multiple tabs without consuming the single controller connection.
- An Active Combat Console with an 11x11 tactical map, party and monster condition telemetry, DEX forecast, reviewed stable-ID mutations, and a fixed-room-only Clear Room review that defeats hostile enemies and declares victory without exiting combat. Mutations are unavailable while combat animation or dialogs are active.
- A Monster Ecosystem Console with map capacity, enemy telemetry, explicit generation settings, confirmed enemy-only clear, and special spawn attempts.
- A dedicated Ships instrument with current PartyVehicle details, hull preview, reviewed non-sinking damage, full repair, and exact Redux skiff quantities from 0 through 12.
- A Game Prepared Spell Inventory grouped by circle, with exact staged quantities, a reviewed all-99 preset, and active-duration controls for exact remaining turns and authentic Light/Time Stop activation.
- A persistent top-bar Free Move switch backed by the same explicit setter and status query as the Debug Options instrument.
- A combat-only masthead **Escape Combat** action that submits `combat.exit-debug` with the latest encounter ID and reviewed snapshot fingerprint. It disappears outside combat and disables during transient combat work; the server rejects fixed dungeon rooms because their exploration continuation requires a normal room exit.
- System Debug Options and Display instruments for retry-safe renderer flags, including the lighting overlay, and exact runtime game-window state. Display mode and preset-size requests are presentation-only and do not persist player Configuration choices.
- A System Redux Modifications instrument for deliberate, locally persisted gameplay choices. It groups explicit typed toggles by gameplay domain, including persistent hidden-dungeon discoveries and subdued unlit dungeon visibility, labels Redux and original modes, refreshes after mutation without continuous polling, and keeps persisted preferences out of Scenario Lab. Combat Speed (a select of the listed 50–2000 percent choices, shown as 0.5x–20x), Confirm Quick Save, Show Combat HP Bars, and Maximum Action Popups (0–3, zero Off) share player Configuration's canonical preferences. Numeric controls derive constraints from catalog metadata, preserve drafts across refreshes, and reject fractional/out-of-step values; an integer parameter with catalog choices renders as a select of those choices. Active Combat's speed control is a compatibility view of that same master speed, not a separate debug override. An open Configuration dialog disables Redux preference controls with an Apply/Cancel explanation; server-side conflict checks also reject stale browser attempts.
  Its Navigation panel offers **Flexible sextant use**, off by default (original nighttime world-only policy). The opt-in permits daytime and settlement use on all floors, including cellars, while retaining dungeon/Underworld exclusions and sextant ownership. The switch persists `Gameplay.Navigation.FlexibleSextantUse` and refreshes the owning typed status after mutation.
- A Tools Observation pane that owns Telescope, Spyglass, and Crystal Ball production workflows, presents their distinct requirements and branches, and shows the latest queued context without polling or automatic execution.
- A Tools Merchant Tester with a static 46-merchant directory, town filtering,
  family-grouped merchant selection, direct concrete Preview for all eight
  families, and reviewed Live launch. All 46 merchants are ready: nine Arms,
  nine Tavern, three Guild, seven Healer, five Reagent, three Horse, four Ship,
  and six Inn merchants.
- Game World Diagnostics consolidating otherwise hidden Guard hostility, Shadowlord presence/assignments, and death/resurrection consequences.
- A Game Inventory Locker with persistent provisions, reagents, potions, scrolls, and armory tabs; staged stale-safe exact carried quantities, explicit scenario presets, additive consumable grants, and read-only all-roster loadout telemetry.
- Character Editor uses a compact live party selector, prefilled attributes, segmented condition/order controls, dirty-only submission, and effective-change preview. Equipment Loadouts independently stages inventory, manufacture, or unequip sources per slot for one member or atomically for all active members, showing common or mixed current slots and aggregate inventory requirements. Party Roster shows active and inactive members together, supports arbitrary occupied-position selection for reviewed status changes, and stages composition/progression reviews before submission.
- A compact last-operation banner inside the active instrument instead of a separate Latest Result column.
- Compact masthead, workspace tabs, and Global History chrome that preserve touch-sized narrow-layout controls while maximizing desktop instrument space; execution-mode badges sit beside instrument titles instead of consuming a separate row.
- Collapsible bounded history with follow controls, switchable bottom/right docking, and orientation-aware resizing. Narrow layouts render it at the bottom without overwriting the saved dock preference.
- A five-step typography scale: 11px metadata, 12px supporting text, 14px standard UI text, 18px titles, and one responsive display size.
- Content-based sizing: a 1600px shell cap, 760-1100px view caps, intrinsic workspace tabs, 72-112px numeric controls, 176-280px selectors, fixed-size Wind controls, and vertical stacking before horizontal scrolling.
- Global catalog search across all workspaces, catalog-owned aliases and search terms, destination-labelled results that switch to the owning workspace, category navigation, one active generated or specialized instrument, F5 execution, and narrow desktop-browser stacking. Phone-specific design is not a goal.

Saves and Scenarios use capped work surfaces and explicit reviewed operations.
The Workbench does not expose an arbitrary live-state or snapshot-field editor.

### Specialized Instrument Ownership

A specialized instrument owns its status command and every companion mutation as one UI surface. Owned commands remain in the authoritative catalog but are hidden from duplicate raw/generated navigation. Adding a companion command requires updating ownership routing, pending-state aggregation, post-operation refresh, and frontend tests together.

Observation uses `command-test.telescope` as its stable navigation entry and hides `command-test.spyglass` and `command-test.crystal-ball` as companion operations. Its three mode buttons always submit the original command IDs with empty arguments; shared pending state disables all modes, and F5 is a prevented no-op so keyboard execution cannot accidentally choose Telescope.

Merchant Tester uses `merchant.tester.catalog` as its Tools entry and hides
`merchant.tester.preview.open` and `merchant.tester.live.open` from generic
Tools navigation. Its static catalog loads on selection or F5 and never polls.
The town filter and merchant rows make all canonical merchants discoverable.
Preview opens the concrete production Arms, Guild, Healer, Reagent, Horse, Ship,
Inn, or Tavern UI
from any otherwise supported gameplay location and stops before commit, so it
cannot mutate gameplay, invoke gameplay callbacks, advance time, or consume RNG.
The Preview UI is visibly labeled before transaction selection. Remote Horse
Preview reports that availability will be checked in Live instead of exposing
delivery, terrain, position, or floor internals. Ship Preview uses the production
Showroom and Manifest and stops a confirmed purchase or harbour escort before
vehicle mutation or transition. The workflow still invokes its
normal presentation input block/unblock callbacks. Inn Preview uses the
production Rest card, Leave/Pick Up lists, and action approval; each confirmation
returns to the same mode with an unchanged-state receipt before any payment,
overnight time, spell clear, roster transfer, or guard activation. Tavern
Preview uses the exact menu, Wine picker, visible topic input, and
authority-projected provision/action approval while stopping before payment, food, intoxication, karma,
customer-session, table-overlay, guard, time, callback, or RNG effects. Live first
stages a review of merchant, current location/floor/coordinates, effective mode,
and source fingerprint; the second action submits those reviewed preconditions.
Same-town Live queues the real production workflow, while remote Live performs a
coordinated town alteration before resolving the real NPC/session. Production
transactions remain active afterward, the whole Talk owns one minute, and there
is no automatic rollback. Families share the tester command IDs; concrete
production workflows retain their own transaction semantics.

World Navigator is owned by `world.position.status`; it submits
`world.position.set` and `movement.landmark.travel`. The older focused
`movement.teleport`, `movement.enter-location`, and `movement.floor.set`
commands remain catalogued but are hidden from duplicate Tools navigation. Its
location control opens the complete destination list on click and filters ranked
matches by location name, map type, or ID as the operator types.

### Status-Backed Refresh

Status-backed instruments query when selected, generally poll silently only while visible, and issue an immediate silent status query after each successful companion mutation. World Navigator preserves dirty destination drafts while polling advances only the reviewed live-source preconditions. A `stale_world_position` failure triggers one silent status refresh and never automatically retries the mutation. The masthead calendar and combat-only quick exit are explicit global consumers, so their calendar and active-combat status queries continue silently across workspaces. Refreshed status, not optimistic UI state, is authoritative. Refreshes otherwise reconcile selections while preserving unrelated dirty drafts, and routine polling does not enter global history.

Active Combat also refreshes when it is the selected instrument, with duplicate
pending queries suppressed. Hidden browser pages stop routine polling. Workspace
selection is retained per workspace; session-owned instrument drafts/tabs survive
detours and reconnects. These are desired values, not durable browser-reload
storage or review authority. Navigation, connection/session, scene, and visibility
changes disarm reviews; later telemetry does not silently overwrite dirty values.
Local/pre-send errors are displayed on the owning instrument instead of leaving
an older success banner visible. Blank numeric input is not coerced to zero.

## Visual Semantics

| Treatment | Meaning |
|---|---|
| Blue border | Structural panel or modal boundary |
| Yellow focus | Current keyboard focus or pending review |
| Green indicator | Connected, valid, live-safe, or successful |
| Red indicator | Destructive, failed, or disconnected |
| Magenta indicator | Seeded scenario or synthetic test operation |
| Gray text | Secondary metadata or unavailable action |

Use the Ultima display font only for the product title and selected headings. Use a readable monospaced developer font for forms, values, diffs, and events.

## Home Workspace

Home is the first workspace on every browser launch. It replaces the former decorative `WORKSPACES` label with a real accessible tab and provides orientation without duplicating command controls.

The page contains:

- A prominent Go To launcher for the navigation-only command palette.
- Up to eight browser-local recent surfaces, resolved against the current authoritative catalog. Selecting one navigates and auto-loads its owning read-only query without executing a mutation.
- The newest three current or durable scenario runs, including textual state, progress, baseline, and local completion time.
- An explicit handoff to Scenarios / Run History by stable run ID, or to the scenario Library when no run is retained.
- Review Again opens a fresh review with recorded launch values; old history
  lacking those values is visibly unsupported rather than replayed with defaults.
- Empty states that direct new users toward Go To or the workspace tabs.

Home reuses the existing silent startup `scenario.run.status` query. Receiving durable history does not pull the user away from Home. Disconnected Home content is labelled `LAST RECEIVED` rather than live. Recent surfaces are recorded by navigation through the command palette or Home itself; ordinary command-rail selection does not create a second recency model.

Home owns no catalog commands and exposes no mutation controls. The previously selected instrument and its draft remain intact underneath Home so entering its workspace can continue that work.

## Command Palette

`Ctrl+K` or `Command+K` opens the fastest path to any Workbench surface. Selecting a result opens its owning workspace and instrument, automatically loads the owning read-only query, and never executes a mutation.

```text
              +====================================================+
              | COMMAND PALETTE                               Esc x |
              +====================================================+
              | > shrine_                                         |
              +----------------------------------------------------+
              | MATCHES                                            |
              | > Shrine Reference                                 |
              |   Codex / Immutable virtue reference               |
              |                                              Enter |
              +----------------------------------------------------+
              |   Shrine Progress                                   |
              |   Game / Live shrine quest state                    |
              +----------------------------------------------------+
              | READ-ONLY REFERENCES AUTO-LOAD · NO MUTATIONS RUN   |
              +====================================================+
```

Search matches:

- Workspace and instrument names.
- Surface descriptions and owning workspaces.
- Stable command IDs and catalog-owned search terms.
- Command names, descriptions, categories, and aliases owned by each surface.
- Recent surfaces when the query is empty, capped at eight.

Ranking favors exact surface or workspace-qualified matches, then prefixes, then deterministic all-token matches. Ties sort by workspace, surface label, and stable surface ID.

Enter navigates to the selected surface and closes the palette. Command execution remains inside the destination instrument with its normal validation and review requirements.

## Generated Command Form

```text
+============================================================================+
| COMMANDS / CHARACTER                 CHARACTER RESOURCES                [*] |
+============================================================================+
| Target       [All active party                                      v]      |
| Resource     [Hit Points                                            v]      |
| Operation    [Set                                                   v]      |
| Value        [100____________________________________________________]      |
|                                                                            |
| Safety       LIVE-SAFE                                                     |
| Preview      Six active characters will be set to their clamped maximum.   |
|                                                                            |
| Logical operation                                                          |
| party.resource.change(target=all, resource=hp, operation=set, value=100)   |
|                                                                            |
| [F5 Refresh] [Reset]                                                     |
+============================================================================+
```

Controls are selected from parameter schemas:

| Parameter kind | Default control |
|---|---|
| Enum | Searchable select |
| Bounded integer | Number input with min/max and optional slider |
| Boolean | Checkbox or explicit two-choice control |
| Pattern string | Text input with live constraint message |
| Character | Searchable party/roster selector with current state |
| Location | Grouped searchable location selector |
| Monster | Searchable selector with relevant statistics |
| Equipment/item | Searchable selector with quantity and compatibility |
| Dynamic option | Server-supplied revisioned select |

Dangerous defaults such as `all`, `clear`, or deletion are not preselected unless the logical action is explicitly a bulk preset.

## Saves Workspace

Saves is a top-level workspace for immutable canonical snapshot records. It does not import legacy `SAVED.GAM`, expose full snapshot JSON, compare records, or offer arbitrary field editing.

```text
+============================================================================+
| SAVES                                            [REFRESH LIBRARY]          |
+============================================================================+
| PROJECT / SHARED | SELECTED SNAPSHOT DETAIL       | CAPTURE ACTIVE SCENE  |
| Before Dungeon   | project:before-dungeon         | Source [Personal v]   |
|                  | Revision / Compatibility       | Name [____________]   |
| PERSONAL / LOCAL | Location / Party / Date / Time | Slug [____________]   |
| Guard Setup      | Moon [authentic runic glyph]   | Description [...]     |
|                  |                                | [CAPTURE ACTIVE SCENE]|
| ROLLBACK/RECOVERY| [REVIEW & LOAD]                |                        |
| rollback-...     | [USE AS SCENARIO BASELINE]     |                        |
|                  | [DELETE]                       |                        |
+------------------+--------------------------------+------------------------+
```

Capture collects source, name, slug, and description. The slug follows the repository ID grammar and defaults from the name. Capture remains visible but disabled with the server eligibility reason when the scene has combat, dialogs, workflows, pending input, animations, debug presentation overrides, or other unsupported transient state.

Selecting a record fetches detached detail: source-qualified reference, canonical revision, compatibility, location/coordinates/floor, active-party count, date/time, and moon phase rendered with the authentic runic glyph. Full snapshot data is not returned to the editor.

**Review & Load** compares the current scene summary with the candidate and states that rollback will be captured first. Confirmation sends the selected reference and reviewed revision. The server persists rollback, constructs a dormant candidate, and commits scene replacement only on success; a failed candidate leaves the current scene active and reports the persisted rollback reference. Rollback records can themselves be reviewed and loaded.

Project and personal records can be deleted only at the reviewed revision. The user must type the snapshot display name. Deletion is blocked while any scenario references the record. Rollback records are not exposed for user deletion.

**Use as Scenario Baseline** is available only for a named record and an active scenario draft. It changes the draft only and still requires saving. A project scenario can use only a project snapshot; a personal scenario can use either project or personal. Rollback records cannot be baselines.

## Scenarios Workspace

Scenarios is a top-level workspace with three primary tasks:

- **Library** is the default and groups definitions under Project / Shared and Personal / Local, with search, source filtering, and separate new-project/new-personal actions.
- **Quick Setups** contains immutable camping and death/resurrection production-workflow utilities. They are not saved scenario definitions and cannot be edited or duplicated.
- **Run History** lists the active run plus up to 20 newest successful or failed runs from private local disk, with stable run-ID selection and complete step detail.

Selecting a Library entry opens **Details**, which shows its source-qualified reference, revision, baseline, launch-parameter count, and step count. Details is the gateway to Review & Run, Edit, Duplicate, and Delete. Create refuses an existing destination; update and delete require the loaded revision; duplicate fixes the reviewed source revision and requires an absent explicit destination. Project / Shared writes and destructive actions use explicit review.

### Edit Scenario

```text
+============================================================================+
| SCENARIOS                         [LIBRARY] [QUICK SETUPS] [RUN HISTORY]    |
+============================================================================+
| EDIT SCENARIO      | DEFINITION                                            |
|                    | ID [guard-test____]  BASELINE [Current state v]       |
|                    | NAME [Guard test______________________________]       |
|                    | DESTINATION [Project / Shared v]                      |
|                    +-------------------------------------------------------+
|                    | LAUNCH PARAMETERS                         [ADD]        |
|                    | seed | Integer | required | min 0 | default 12345      |
|                    +-------------------------------------------------------+
|                    | ORDERED STEPS                                         |
|                    | 01 movement.free-move.set       [UP] [DOWN] [REMOVE]  |
|                    |    enabled = Parameter: enabled                       |
|                    | 02 Note: Continue through the gate                    |
+--------------------+-------------------------------------------------------+
| UNSAVED CHANGES                          [VALIDATE & CREATE IN PROJECT]     |
+============================================================================+
```

Edit Scenario edits schema version 2. Definitions use `project:<id>` or `personal:<id>` references, select a `current`, `init`, or named `snapshot` baseline object, and contain typed launch parameters plus ordered command/note steps. Transient arguments marked `scenarioRuntime`, such as reviewed party fingerprints, are hidden and omitted from persisted steps because the runner resolves them from live state. Named choices are filtered for compatibility and portability: project definitions can select only project snapshots, while personal definitions can also select personal snapshots. Schema-v1 files are rejected without migration and may be recreated in the editor. Workspace and task navigation preserves the current draft. Home, Library open/play/create, and Global History capture handoffs share one replacement review: a dirty draft requires explicit discard, and a delayed definition load is ignored if the draft or navigation intent changed after dispatch. Closing or reloading the browser also uses the standard unsaved-change warning while the draft is dirty.

The editor searches scenario-safe commands by catalog name, ID, category, description, aliases, and search terms, with matching commands grouped by category. Selecting a command opens a pre-insertion staging panel: each argument renders from catalog metadata as the appropriate enum, bounded integer, boolean, string, or revisioned dynamic-option literal control, or can bind to a type-compatible launch parameter. The command cannot enter the definition until required arguments are valid. An insertion-position control places commands and notes at the beginning, after an existing step, or at the end; terminal-step rules clamp placement before the final workflow. Commands can still be replaced, duplicated, reordered with UP/DOWN controls, or removed, and incompatible argument resets require review. The masthead identifies the active scenario, source, ordered-step count, and saved/unsaved state from every workspace.

When a stored scenario contains a recognized legacy character identity token, Details and Edit Scenario show the in-memory before/after migration and mark the definition dirty. The warning explains that the one-based target follows the character occupying that party position at run time. Saving is explicit and revision-checked; Duplicate remains disabled until the migrated revision is saved, after which the normal successful-update state is clean.

### Run Review And Run History

Every browser launch passes through Run Review. The review binds the saved definition revision, active scene generation, and named-baseline revision while presenting baseline consequences, typed launch values, readiness, the retained-mutation warning, and the ordered execution plan. Start requires the same reviewed revisions and scene generation; stale state rejects the launch before mutation. Direct `--scenario` startup has no browser review page; it loads and pins current revisions before submitting the same runner request.

Library RUN and historical Home Review Again open this review even for
parameterless definitions. Missing required launch values do not block entry to
the screen containing their inputs; they block final Start. Parameter ID edits
retain the input node/caret and migrate bindings using stable session row keys.
Quick Setup death reviews display the reviewed party fingerprint; a party change,
task switch, Escape, or Cancel disarms the review. Catalog validation errors stay visible
on Scenarios. Project-write/delete/duplicate modals own focus, trap Tab, make the
background inert, and restore invoking focus on Escape. Capture's focus loop
includes insertion position; inventory quantity controls have operation/item
accessible names.

Run History reports the baseline, named-baseline revision when applicable, automatic rollback reference/revision for non-current baselines, timestamps, scene generation, next and total step counts, actual command IDs, summaries, semantic state changes, skipped notes, and the first failure. Command identities are populated when the run is queued, so pending steps never appear as a generic `COMMAND`. The current run is merged ahead of newest-first durable terminal records and selected by stable run ID; repeated runs of one scenario remain distinct. The runner continues on the application update thread if the browser disconnects, and completed runs survive process restarts.

History v2 additionally shows actual accepted launch values, whether the baseline
was applied, and optional exact decimal-string RNG seed/source-call provenance.
Review Again prefills those values but does not reapply the diagnostic RNG or
promise deterministic replay. Definition revision changes or unavailable recorded
named-baseline revisions block this shortcut. Legacy history without launch
values is labelled unknown; current defaults are not historical evidence.
The separate RUN AGAIN route opens the current definition's review, not an exact
historical replay.

Cancel Remaining Steps targets the current process run ID and takes effect when
processed; the runner may dispatch before that request reaches the bridge. It
retains completed effects, applied baseline, pending step records, and production
workflows, releasing only scenario-level gating. Review Rollback opens Saves with
the recorded reference/revision pinned; stale or missing records cannot be
substituted and no load occurs before explicit confirmation. Neither cancellation
nor failure restores automatically. `rng.seed.set` remains combat-only.

The supported step types are:

- Execute logical command.
- Add a note.

The runner preflights all command steps, then executes them at update-thread boundaries when the gameplay scene is ready. A `current` baseline starts from the active state. An `init` or named-snapshot baseline first persists automatic rollback, then atomically replaces the scene from external `INIT.GAM` or the reviewed named revision. The first failed command stops the run. Earlier mutations and the failure-time state remain active for inspection; rollback is not automatically restored.

For unsupported step types and persistence boundaries, see
[Scenario Limitations](../systems/DEBUG_WORKBENCH.md#scenario-limitations).

## Global History

```text
+============================================================================+
| GLOBAL HISTORY      [DOCK RIGHT] [RECORD ON] [FOLLOW ON] [CLEAR]           |
+============================================================================+
| 12:38:04 Set Free Move                                      SUCCESS         |
| Free Move enabled.                                                          |
| [ADD TO SCENARIO]                                  REQ ... · SCENE 42      |
+============================================================================+
```

Every result produced after catalog normalization may include a transport-neutral invocation containing only the stable command ID and normalized typed arguments. Global History retains that invocation with the structured result; it does not reconstruct arguments from labels, summaries, drafts, or formatted output.

**Dock Right / Dock Bottom** changes the desktop panel between a vertically resizable bottom row and a horizontally resizable right column. The choice and collapsed state are browser-local preferences. Below the narrow-layout breakpoint, history is rendered at the bottom for usable width while retaining the selected desktop dock for the next wider viewport.

Successful scenario-safe invocations are eligible for capture, except scenario-management commands. When the masthead shows an active scenario, **Add to Scenario** appends directly without leaving the current workspace; the active-target EDIT action opens the ordered editor for review and reordering. Without an active target, Add opens a searchable application-level chooser for an existing Personal or Project definition, followed by an insertion-position choice. The Workbench loads that scenario, inserts the server-normalized invocation, and opens the editor with the new step focused. The chooser can instead create a name-only Personal draft. Project choices remain unsaved drafts and retain the stronger shared-project save review. Dirty drafts cannot be replaced silently. **RECORD ON** requires and continuously identifies one active target, then appends each subsequent eligible non-silent successful result. Request IDs prevent duplicate capture during replay, and clearing Global History does not clear the draft. Failed results and pre-normalization failures are not recordable as command steps; attaching failures as notebook notes remains future work.

## Laptop Layout

Shell tracks allow shrinking, workspace/history actions wrap, and instrument
grids respond to available container width rather than assuming an undocked
viewport. Narrow layouts move history to the bottom without changing the saved
desktop dock preference and reduce command-rail height so controls remain
reachable. Editable values use larger form typography within the existing visual
language; this does not add phone/touch controls to the player-facing game.

`browser/workbench.spec.ts` provides four smoke tests for this secondary tool:
desktop startup/status/navigation at 1280px, reachable inventory controls at 800px,
confirmation/cancellation and visible server rejection for one inventory mutation,
and keyboard palette navigation/dismissal/focus restoration. It uses freshly built
source UI in real Chromium with synthetic transport. Dock/width matrices, actual
desktop zoom and exhaustive scenario workflows are outside browser acceptance.
Live-game mutations and physical keyboard/window acceptance remain manually unverified.

The browser harness builds before launching Chromium and serves the fresh assets
with synthetic transport. Its bounded process profile and cleanup contract are
specified in [Testing](../TESTING.md#canonical-commands).

## Keyboard Model

| Key | Action |
|---|---|
| `Ctrl+K` / `Command+K` | Open the navigation-only command palette |
| `Ctrl+Backquote` | Focus the event stream |
| `F5` | Refresh the focused instrument's telemetry; never apply a mutation |
| `Shift+F5` | Report that staging is unavailable for the current instrument |
| `Tab` / `Shift+Tab` | Move through controls |
| Arrow keys | Navigate lists, trees, and choices |
| `Enter` | Open or select |
| `Escape` | Close top overlay or cancel current operation |

The command palette automatically executes only the read-only query that owns the selected surface. It never executes a mutation; dangerous operations retain their populated review forms. Browser scenario launches use Run Review; direct `--scenario` startup pins current revisions and enters the same runner without browser interaction. There is no `F9` binding or run-selected-step operation.

## Core Workflows

### Discover And Run

```text
Ctrl+K / Command+K
  |
  v
type workspace, instrument, intent, or stable command ID
  |
  v
navigate to owning Workbench surface
  |
  v
review and execute inside the destination instrument
  |
  v
server validation and update-thread execution
  |
  v
structured result/event
```

### Named Snapshot Load

```text
select and inspect immutable snapshot revision
        |
        v
review current scene, candidate summary, and rollback consequence
        |
        v
submit reference + expected revision
        |
        v
capture and persist automatic rollback snapshot
        |
        v
validate snapshot and construct dormant scene
        |
        +--> failure: current scene remains active
        |
        v
atomic scene replacement --> refresh Workbench state
```

### Scenario Run

```text
load definition --> bind launch parameters --> preflight every command
        |
        v
current: retain active scene
init/snapshot: persist rollback, build reviewed baseline, atomically replace scene
        |
        v
advance from Game.Update when the scene reports command readiness
        |
        +--> command
        +--> note (recorded as skipped)
        |
        +--> success: retain state and continue
        |
        +--> failure: stop and retain failure-time state
```
