# Debug Workbench UI

## Purpose

This document defines the browser workbench layout and primary workflows. The interface prioritizes rapid keyboard-driven debugging, generated controls, clear safety consequences, and repeatable save operations.

The visual language should feel related to Ultima V without imitating an in-game medieval interface. Use a dense instrument-panel layout, restrained EGA-inspired semantic colors, thin blue borders, and monospaced values. Avoid generic dashboard cards and excessive whitespace.

Related documents:

- [Debug Workbench System](../systems/DEBUG_WORKBENCH.md)
- [Save Snapshot Schema](../details/SAVE_SNAPSHOT_SCHEMA.md)

## Task Pages

Each of the six task tabs is one page with one pending-changes bar ("N changes · Discard · Apply N changes") where edits are staged. The bar sends its changes one command at a time, in order, refreshing whatever the next command was reviewed against (the inventory snapshot, the dynamic party options) before sending it. It stops at the first failure and says how many changes were applied; a stale-state failure refreshes the page and says to review and apply again. Every command still carries the fingerprints it requires, read from the page's latest status. A page's anchor command (`world.position.status`, `party.character.inspect`, `inventory.locker.status`, `calendar.status`, `combat.status`, `save.snapshot.list`) stands for the page in the command palette, polling and the last-operation banner; the page's other commands are companions found through the same palette entry. Pages read their status commands on open and while visible.

- **Go to** (`GoToPage.vue`): a searchable places list with kind filters (Towns and castles, Dungeons, Shrines and landmarks, Recent) beside an Arrive panel. Landmarks and shrines travel with `movement.landmark.travel`. Towns offer the starting spot, the outside entrance and an exact position. Dungeons offer outside the entrance, level start, **outside a room**, an exact cell and the exit ladder, with level buttons. Outside a room lists the level's rooms from `rooms` on the destination; unavailable rooms stay listed with their reason. Applying sends `world.position.set` with placement `dungeon_outside_room` and the `room` number, which stands the party on the approach cell facing the room so one step forward enters it. A plain sentence states where you will stand. "Start this room's fight instead" opens Fight with that room preselected in the composer. The free-move toggle lives here. There is no map-first picker.
- **Party** (`PartyPage.vue`): whole-party actions (`party.restore` Heal and cure everyone, `party.maximize-levels`, `party.fill`) plus karma; member cards with HP bar and condition; "+ Add a member" from inactive companions (`party.member.add`); the selected member's attributes, level and experience, HP and MP with Full buttons, condition chips, and the six equipment slots. Edits are kept per member and applied together as `party.character.update` and `party.equipment.loadout.update` calls. There is no "Best gear" action because the game has no rule for which item is better.
- **Items & spells** (`ItemsPage.vue`): one search box over everything carried, kind tabs (Supplies, Reagents, Potions and scrolls, Equipment stock, Quest items, Prepared spells), a presets row per kind, and "-/+ with Max" rows that stage absolute targets. Potions and scrolls can only be added. Quest items grant and remove one at a time. A preset replaces staged row edits and is dropped if the inventory changes after it was chosen. Light and duration-spell controls sit under Prepared spells. Quest-item shard use and the Item Stacks search tool stay under More.
- **Time & sky** (`TimePage.vue`): time-of-day tiles with the reason each matters (from the game's daylight boundaries), hour, minute and day inputs, the live moon phase read-only (no command sets it; it follows the calendar day), moongate status with travel and the moonstone-marks overlay, and wind buttons. Month and year are shown read-only.
- **Fight** (`FightPage.vue`): in combat, the battlefield, per-monster -10 HP, Sleep or Wake, and Defeat, Win now (`combat.monster.kill-all`), Put all to sleep (`combat.monster.sleep-all`), Leave combat, "+ Add monsters", and under Advanced the repeatable-dice seed, combat speed, fixed-room clear and outcome-free removal. Actions carry the encounter snapshot and are held back with a plain reason while a message or monster is acting. Healing the party is not offered here because `party.restore` is refused during combat. Outside combat the page shows the encounter composer.
- **Checkpoints** (`SaveLibrary.vue`): "Save this moment" (name, Just me or Shared with the project), Mine and Shared lists, and "Load..." which opens a Now versus After loading review before sending the reviewed revision. Delete asks for the checkpoint name. Automatic recovery checkpoints are not listed.

## Current Implementation

The responsive query/mutation shell and task-hub navigation are implemented. They include:

- A top bar on every view: connection, scene and retry state; **Where** (location and floor), **When** (game date and time), **Party** (size and plain-language health); an Exploring or In combat chip; and Search (`Ctrl/⌘K`). It shows only telemetry the Workbench already receives.
- Task tabs with a clear selected state: Home, Go to, Party, Items & spells, Time & sky, Fight (marked while in combat), Checkpoints, and More. Go to, Party, Items & spells, Time & sky, Fight and Checkpoints are each one page that shows the whole job at once (see Task Pages); only More keeps a tool picker. The authoritative command catalog stays underneath.
- Every browser launch opens Home. The selected instrument, collapsed Global History state, bottom/right history dock, and navigation recents remain browser-local across refreshes; stale instrument and recent-surface identities fall back through catalog validation.
- Named virtue navigation instead of numeric Shrine entry.
- Palette, Home, and tool-picker navigation automatically load the selected read-only reference. Failed reference loads render an explicit error instead of an indefinite loading state.
- Connection, scene-generation, retry, and session-resume indicators.
- A top-bar Britannian calendar showing the live formatted game date/time, moon-phase label, and authentic runic phase glyph from the shared `runic_font.png` sprite sheet.
- Correlated accepted/completed/failed command handling with replay deduplication.
- Typed wind, calendar, world-navigation, moongate, ecosystem, vehicle, inventory-locker, encounter-composer, search/item-stack, quest-inventory, logging, shrine, and party result views, plus schema-generated fallback controls.
- A Shrine Progress instrument (More) for current quest/ruined state and explicit alterations, including shrine progress presets.
- A five-cell Search cross with complete stack inspection, stable provision placement, and confirmed one-time-progress reset.
- A fixed Quest Inventory ledger with all 14 item identities, canonical placements, and confirmed per-item removal.
- A Test Setups instrument (More) for reviewed camping and death/resurrection utilities; these are built-in utilities, not saved scripts.
- The Monster Ecosystem roster uses bounded keyboard-accessible scrolling with sticky headings; short reference tables remain unbounded.
- A comprehensive Logging Control instrument with master/stdout switches, typed minimum level, explicit category channels, and a handoff to the read-only log window.
- A separate read-only log window with level/category/text filters, pause/follow controls, overwrite reporting, and support for multiple tabs without consuming the single controller connection.
- A Monster Ecosystem Console with map capacity, enemy telemetry, explicit generation settings, confirmed enemy-only clear, and special spawn attempts.
- A dedicated Ships instrument with current PartyVehicle details, hull preview, reviewed non-sinking damage, full repair, and exact Redux skiff quantities from 0 through 12.
- A Prepared Spell Inventory (Items & spells) grouped by circle, with exact staged quantities, a reviewed all-99 preset, and active-duration controls for exact remaining turns and authentic Light/Time Stop activation.
- A Free Move switch in the Go to page header, backed by the same explicit setter and status query as the Debug Options instrument.
- A combat-only top-bar **Escape Combat** action that submits `combat.exit-debug` with the latest encounter ID and reviewed snapshot fingerprint. It disappears outside combat and disables during transient combat work; the server rejects fixed dungeon rooms because their exploration continuation requires a normal room exit.
- A Debug Options instrument (More) for retry-safe renderer flags, including the lighting overlay. Display settings and persisted Redux gameplay preferences are no longer Workbench instruments; they remain in the game's own Configure dialog.
- An Observation pane (More) that owns Telescope, Spyglass, and Crystal Ball production workflows, presents their distinct requirements and branches, and shows the latest queued context without polling or automatic execution.
- A Merchant Tester (More) with a static 46-merchant directory, town filtering,
  family-grouped merchant selection, direct concrete Preview for all eight
  families, and reviewed Live launch. All 46 merchants are ready: nine Arms,
  nine Tavern, three Guild, seven Healer, five Reagent, three Horse, four Ship,
  and six Inn merchants.
- World Diagnostics (More) consolidating otherwise hidden Guard hostility, Shadowlord presence/assignments, and death/resurrection consequences.
- A compact last-operation banner inside the active instrument instead of a separate Latest Result column.
- Compact top bar, task tabs, and Global History chrome that preserve touch-sized narrow-layout controls while maximizing desktop instrument space; execution-mode badges sit beside instrument titles instead of consuming a separate row.
- Collapsible bounded history with follow controls, switchable bottom/right docking, and orientation-aware resizing. Narrow layouts render it at the bottom without overwriting the saved dock preference.
- A five-step typography scale: 11px metadata, 12px supporting text, 14px standard UI text, 18px titles, and one responsive display size.
- Content-based sizing: a 1600px shell cap, 760-1100px view caps, intrinsic task tabs, 72-112px numeric controls, 176-280px selectors, and vertical stacking before horizontal scrolling.
- Global catalog search across all tasks, catalog-owned aliases and search terms, destination-labelled results that switch to the owning task, one active generated or specialized instrument, F5 execution, and narrow desktop-browser stacking. Phone-specific design is not a goal.

Checkpoints uses capped work surfaces and explicit reviewed operations.
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

The Go to page is owned by `world.position.status`; it submits
`world.position.set` and `movement.landmark.travel`; the older focused
teleport, enter-location and floor commands were removed in its favour. Its
places list shows every destination and landmark and filters by name, kind and
recent use as the operator types.

### Status-Backed Refresh

Status-backed instruments query when selected, generally poll silently only while visible, and issue an immediate silent status query after each successful companion mutation. Page drafts (a chosen place, level, room, staged edits) survive polling, which advances only the reviewed live-source preconditions. A `stale_world_position` failure triggers one silent status refresh and never automatically retries the mutation. The top bar's calendar, location and party summary, and the combat-only quick exit, are explicit global consumers, so their calendar, test-setup, and active-combat status queries continue silently across tasks. Refreshed status, not optimistic UI state, is authoritative. Refreshes otherwise reconcile selections while preserving unrelated dirty drafts, and routine polling does not enter global history.

A task page also refreshes its status commands while it is the selected page, with duplicate
pending queries suppressed. Hidden browser pages stop routine polling. Tool
selection is retained per task; session-owned instrument drafts/tabs survive
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

## Home

Home opens on every browser launch and asks "What do you want to set up?". It
holds one card for each common task (Go to, Fight, Party, Items & spells, Time &
sky, Checkpoints), each with an icon, a one-line purpose, a few quick actions and
an Open button, plus a Recent list.

Quick actions only call commands that need no separate review:

- Go to: `movement.landmark.travel` for a few landmarks from the live catalog.
- Time & sky: `calendar.set-time` presets for 05:00, 12:00 and 20:00.
- Fight: Escape combat (`combat.exit-debug`), shown only while in combat.
- Party: Heal and cure everyone (`party.restore`), which reads the roster first so its reviewed fingerprint is fresh; unavailable during combat.

Commands that require a fingerprint review inside their tool, such as party
status, inventory presets and checkpoint load, are not offered on Home. The last
quick action's outcome shows in a status line.

Recent lists up to eight browser-local surfaces, resolved against the current
authoritative catalog. Selecting one navigates and auto-loads its owning read-only
query without executing a mutation. Recents are recorded by palette or Home
navigation; ordinary tool-picker selection does not create a second recency model.
Surface IDs are `<task>:<commandId>`; recents saved under the former workspace IDs
are dropped.

The previously selected instrument and its draft remain intact underneath Home,
so opening its task continues that work.

## Command Palette

`Ctrl+K` or `Command+K` opens the fastest path to any Workbench surface. Selecting a result opens its owning task and instrument, automatically loads the owning read-only query, and never executes a mutation.

```text
              +====================================================+
              | COMMAND PALETTE                               Esc x |
              +====================================================+
              | > shrine_                                         |
              +----------------------------------------------------+
              | MATCHES                                            |
              | > Shrine Progress                                  |
              |   More / Live shrine quest state                   |
              |                                              Enter |
              +----------------------------------------------------+
              | READ-ONLY QUERIES AUTO-LOAD · NO MUTATIONS RUN   |
              +====================================================+
```

Search matches:

- Task and instrument names.
- Surface descriptions and owning tasks.
- Stable command IDs and catalog-owned search terms.
- Command names, descriptions, categories, and aliases owned by each surface.
- Recent surfaces when the query is empty, capped at eight.

Ranking favors exact surface or task-qualified matches, then prefixes, then deterministic all-token matches. Ties sort by task, surface label, and stable surface ID.

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

## Checkpoints

Checkpoints is a top-level page for immutable canonical snapshot records. It does not import legacy `SAVED.GAM`, expose full snapshot JSON, compare records, or offer arbitrary field editing. See Task Pages for the layout.

Saving takes a name (the slug is derived from it), an optional note and an audience: Just me (personal) or Shared with the project. The button stays visible but disabled, with the server's reason, when the scene has combat, dialogs, workflows, pending input, animations, debug presentation overrides, or other unsupported transient state.

**Load...** fetches the checkpoint's detached detail and compares it with the current scene summary (where, when, party). Confirmation sends the reviewed reference and revision; the review is pinned to what was fetched and disarms if the revision changes. The server persists a recovery record, constructs a dormant candidate, and commits scene replacement only on success. Project and personal records can be deleted only at the reviewed revision, after typing the display name. Recovery records are not listed or deleted here.

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

Shell tracks allow shrinking, task tabs and history actions wrap, and instrument
grids respond to available container width rather than assuming an undocked
viewport. Narrow layouts move history to the bottom without changing the saved
desktop dock preference and keep tool pickers compact so controls remain
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
type task, instrument, intent, or stable command ID
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
review current scene and candidate summary
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
