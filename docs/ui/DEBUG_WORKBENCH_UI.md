# Debug Workbench UI

## Purpose

This document defines the browser workbench layout and primary workflows. The interface prioritizes rapid keyboard-driven debugging, generated controls, clear safety consequences, and repeatable save operations.

The visual language should feel related to Ultima V without imitating an in-game medieval interface. Use a dense instrument-panel layout, restrained EGA-inspired semantic colors, thin blue borders, and monospaced values. Avoid generic dashboard cards and excessive whitespace.

Related documents:

- [Debug Workbench System](../systems/DEBUG_WORKBENCH.md)
- [Save Snapshot Schema](../details/SAVE_SNAPSHOT_SCHEMA.md)

## Current Implementation

The responsive query/mutation shell and first intent-based workspaces are implemented. They include:

- Home, Game, System, Saves, and Tools navigation while preserving the authoritative command catalog underneath.
- A visually dominant workspace tab rail with clear selected-state treatment.
- Every browser launch opens Home. The selected instrument, collapsed Global History state, bottom/right history dock, and navigation recents remain browser-local across refreshes; stale instrument and recent-surface identities fall back through catalog validation.
- Visible-only silent refresh for Logging, Wind, Calendar, World Navigator, Moongates, Monster Ecosystem, Active Combat, Vehicle, Inventory Locker, Prepared Spells, Search, Quest Inventory, and Shrine progression plus revisioned party-editor metadata refresh, without flooding global history.
- Named virtue navigation instead of numeric Shrine entry.
- Palette, Home, and command-rail navigation automatically load the selected read-only reference. Failed reference loads render an explicit error instead of an indefinite loading state.
- Connection, scene-generation, retry, and session-resume indicators.
- A masthead Britannian calendar showing the live formatted game date/time, moon-phase label, and authentic runic phase glyph from the shared `runic_font.png` sprite sheet.
- Correlated accepted/completed/failed command handling with replay deduplication.
- Typed wind, calendar, world-navigation, moongate, ecosystem, vehicle, inventory-locker, encounter-composer, search/item-stack, quest-inventory, logging, shrine, and party result views, plus schema-generated fallback controls.
- A Game Shrine Progress instrument for current quest/ruined state and explicit alterations, including shrine progress presets.
- A specialized Wind console with automatic telemetry, pause/resume, age, explicit direction selection, and Apply.
- A Calendar chronometer with staged practical presets, bounded exact controls, and derived lighting/moon context.
- A five-cell Search cross with complete stack inspection, stable provision placement, and confirmed one-time-progress reset.
- A fixed Quest Inventory ledger with all 14 item identities, canonical placements, and confirmed per-item removal.
- Three independent Game party instruments: Character Editor for one active character's attributes/resources/condition/order, Equipment Loadouts for reviewed six-slot atomic changes, and Party Roster for active/inactive composition plus reviewed party-wide operations. User-facing active positions are one-based; zero-based roster indices remain secondary diagnostic identities.
- A Tools Test Setups instrument for reviewed camping and death/resurrection utilities; these are built-in utilities, not saved scripts.
- A Saves workspace grouping Project / Shared, Personal / Local, and Rollback / Recovery records, with detached compact detail, stable-scene capture, reviewed load/delete, and automatic rollback disclosure.
- A World Navigator that preserves staged destination drafts while live status silently refreshes their reviewed source preconditions. It submits one atomic exact or authoritative route placement across large, small, and dungeon maps. Dungeon routes expose outside entrance, canonical inside start, and the non-Doom Underworld-exit ladder; unavailable route targets remain visible with a reason. Physical shrine setup remains a separate lane; Spirituality and waterfall transitions remain separate workflows.
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
- A System Debug Options instrument for retry-safe renderer flags, including the lighting overlay. Display settings and persisted Redux gameplay preferences are no longer Workbench instruments; they remain in the game's own Configure dialog.
- A Tools Observation pane that owns Telescope, Spyglass, and Crystal Ball production workflows, presents their distinct requirements and branches, and shows the latest queued context without polling or automatic execution.
- A Tools Merchant Tester with a static 46-merchant directory, town filtering,
  family-grouped merchant selection, direct concrete Preview for all eight
  families, and reviewed Live launch. All 46 merchants are ready: nine Arms,
  nine Tavern, three Guild, seven Healer, five Reagent, three Horse, four Ship,
  and six Inn merchants.
- Game World Diagnostics consolidating otherwise hidden Guard hostility, Shadowlord presence/assignments, and death/resurrection consequences.
- A Game Inventory Locker with persistent provisions, reagents, potions, scrolls, and armory tabs; staged stale-safe exact carried quantities, explicit presets, additive consumable grants, and read-only all-roster loadout telemetry.
- Character Editor uses a compact live party selector, prefilled attributes, segmented condition/order controls, dirty-only submission, and effective-change preview. Equipment Loadouts independently stages inventory, manufacture, or unequip sources per slot for one member or atomically for all active members, showing common or mixed current slots and aggregate inventory requirements. Party Roster shows active and inactive members together, supports arbitrary occupied-position selection for reviewed status changes, and stages composition/progression reviews before submission.
- A compact last-operation banner inside the active instrument instead of a separate Latest Result column.
- Compact masthead, workspace tabs, and Global History chrome that preserve touch-sized narrow-layout controls while maximizing desktop instrument space; execution-mode badges sit beside instrument titles instead of consuming a separate row.
- Collapsible bounded history with follow controls, switchable bottom/right docking, and orientation-aware resizing. Narrow layouts render it at the bottom without overwriting the saved dock preference.
- A five-step typography scale: 11px metadata, 12px supporting text, 14px standard UI text, 18px titles, and one responsive display size.
- Content-based sizing: a 1600px shell cap, 760-1100px view caps, intrinsic workspace tabs, 72-112px numeric controls, 176-280px selectors, fixed-size Wind controls, and vertical stacking before horizontal scrolling.
- Global catalog search across all workspaces, catalog-owned aliases and search terms, destination-labelled results that switch to the owning workspace, category navigation, one active generated or specialized instrument, F5 execution, and narrow desktop-browser stacking. Phone-specific design is not a goal.

Saves uses capped work surfaces and explicit reviewed operations.
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
`world.position.set` and `movement.landmark.travel`; the older focused
teleport, enter-location and floor commands were removed in its favour. Its
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
| Magenta indicator | Seeded or synthetic test operation |
| Gray text | Secondary metadata or unavailable action |

Use the Ultima display font only for the product title and selected headings. Use a readable monospaced developer font for forms, values, diffs, and events.

## Home Workspace

Home is the first workspace on every browser launch. It replaces the former decorative `WORKSPACES` label with a real accessible tab and provides orientation without duplicating command controls.

The page contains:

- A prominent Go To launcher for the navigation-only command palette.
- Up to eight browser-local recent surfaces, resolved against the current authoritative catalog. Selecting one navigates and auto-loads its owning read-only query without executing a mutation.
- Empty states that direct new users toward Go To or the workspace tabs.

Home disconnected content is labelled `LAST RECEIVED` rather than live. Recent surfaces are recorded by navigation through the command palette or Home itself; ordinary command-rail selection does not create a second recency model.

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
              | > Shrine Progress                                  |
              |   Game / Live shrine quest state                   |
              |                                              Enter |
              +----------------------------------------------------+
              | READ-ONLY QUERIES AUTO-LOAD · NO MUTATIONS RUN   |
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
| rollback-...     | [DELETE]                       |                        |
+------------------+--------------------------------+------------------------+
```

Capture collects source, name, slug, and description. The slug follows the repository ID grammar and defaults from the name. Capture remains visible but disabled with the server eligibility reason when the scene has combat, dialogs, workflows, pending input, animations, debug presentation overrides, or other unsupported transient state.

Selecting a record fetches detached detail: source-qualified reference, canonical revision, compatibility, location/coordinates/floor, active-party count, date/time, and moon phase rendered with the authentic runic glyph. Full snapshot data is not returned to the editor.

**Review & Load** compares the current scene summary with the candidate and states that rollback will be captured first. Confirmation sends the selected reference and reviewed revision. The server persists rollback, constructs a dormant candidate, and commits scene replacement only on success; a failed candidate leaves the current scene active and reports the persisted rollback reference. Rollback records can themselves be reviewed and loaded.

Project and personal records can be deleted only at the reviewed revision. The user must type the snapshot display name. Rollback records are not exposed for user deletion.


## Test Setups

Test Setups is a Tools instrument (`test-setup.status`, category Test Setups)
holding immutable, reviewed production-workflow utilities: camping
(`command-test.hole-up`) and death/resurrection (`death.kill-party`,
`death.test-sequence`). They are built-in utilities, not saved scripts, and cannot
be edited or duplicated. Death reviews display the reviewed party fingerprint;
a party change, task switch, Escape, or Cancel disarms the review.

Scripted scenarios, scenario libraries, run review and run history were removed
from the Workbench; see [Debug Workbench System](../systems/DEBUG_WORKBENCH.md).

## Global History

```text
+============================================================================+
| GLOBAL HISTORY      [DOCK RIGHT] [FOLLOW ON] [CLEAR]                       |
+============================================================================+
| 12:38:04 Set Free Move                                      SUCCESS         |
| Free Move enabled.                                                          |
|                                                    REQ ... · SCENE 42      |
+============================================================================+
```

Every result produced after catalog normalization may include a transport-neutral invocation containing only the stable command ID and normalized typed arguments. Global History retains that invocation with the structured result; it does not reconstruct arguments from labels, summaries, drafts, or formatted output.

**Dock Right / Dock Bottom** changes the desktop panel between a vertically resizable bottom row and a horizontally resizable right column. The choice and collapsed state are browser-local preferences. Below the narrow-layout breakpoint, history is rendered at the bottom for usable width while retaining the selected desktop dock for the next wider viewport.

History is a log of results only. It does not capture steps into saved scripts.

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
desktop zoom and exhaustive workflow matrices are outside browser acceptance.
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

The command palette automatically executes only the read-only query that owns the selected surface. It never executes a mutation; dangerous operations retain their populated review forms. There is no `F9` binding or run-selected-step operation.

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
