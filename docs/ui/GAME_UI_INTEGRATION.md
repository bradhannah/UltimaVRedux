# Game UI Integration

**Purpose**: Document how `GameScene` wires widgets, callbacks, update ownership, rendering order, transient feedback, and resolution changes. Widget internals and dialog selection are documented separately.

## Owned Presentation State

`GameScene` owns:

- `widgets.DialogStack` for modal and non-blocking dialogs
- `text.Output` for the right-side game record
- `input.Keyboard` and the primary/secondary command state machine
- borders, character summary, compact provision summary, display-only status rail, wind display, and transient icon overlay
- visual-effect state such as map fade, flash intensity, shake offset, and inversion
- optional `audio.GameAudioManager` over Berad Audio when sound is available
- feature UI such as the combat HUD, detached Z-Stats and Quest Journal modals,
  and transient action-feedback records

Core gameplay reaches this layer through `SystemCallbacks`; it does not construct Ebitengine widgets.

The surface sky and Winds readouts share the rasterized outer map-frame center
and blue/white inward bookend geometry. The sky's glyph cells are spread across
a strip about 50% wider where the frame has room, capped on narrow screens to
preserve the rounded corners while retaining the glyphs' original font scale.
Its black backing spans the full upper border band beneath the decorated strip,
covering the frame's otherwise exposed lower white rule. Both readouts are
presentation-only; frame resizing rebuilds their graphics without advancing
the calendar or consuming gameplay randomness.

The bottom frame uses `mapBorderLabel`: large maps retain Winds, while ordinary
small maps show the current friendly location name through the same centred
blue/white bookends. It does not append a floor suffix or change wind authority.
Combat, camping, shrine/Codex and other non-surface-frame scenes suppress the
label. `WindDisplay.DrawLabel` measures and rebuilds its Output/chrome when the
label or layout changes.

First-person dungeon exploration uses the same inward-pointing blue/white
bookends around `L <level>` in the upper frame band and `Dir: <cardinal>` in the
lower band. Each complete label is centred on the rasterized map frame with an
opaque black backing and scaled clearance around its `text.Output` text. Level
numbering remains one-based; the four facing names retain their existing meaning.
The cached label graphics rebuild when text, font, frame or band geometry changes,
releasing the replaced image. Resizing and disposal share that cache lifecycle.
The labels are static chrome: camera shake and dungeon world effects do not move
them or cover the surrounding status panels.

Dungeon-origin camping and tactical combat reuse the same decorated level header
where the inherited header is already visible. They do not gain an exploration
direction readout or surface sky. Existing cinematic/header suppression and
diagnostic bounds remain separate from this label presentation.

## Initialization

`wireSystemCallbacks` constructs the required message, visual, audio, screen, flow, talk, UI, and combat callback groups. Constructors validate mandatory functions before `NewSystemCallbacks` accepts the aggregate. Code running after successful initialization should use those callbacks directly.

Supply projectile/impact callbacks inside `NewVisualCallbacks` and MessageDialog
callbacks inside `NewScreenCallbacks`. `cmd/ultimav/gamescene_callbacks.go`
shows complete named-family construction; do not attach mandatory members later.

Audio is the exception at the concrete-resource level: sound-disabled mode or a recoverable audio initialization error leaves `GameScene.audioManager` nil, while the `AudioCallbacks` functions remain present and safely perform no playback.

The adapter owns Redux JSON/world interpretation and music policy; its SFX,
music, ambient and preview transports come from the pinned Berad Audio module.
Application exit disposes the current scene once, including early errors after
Game allocation. Queue finalizers retain their scene ownership until disposal
completes, then module voices retire without closing the shared audio device.

## Update Order

The normal `GameScene.Update` path is listed below. An active Journal session
is intercepted before queues and gameplay updates, including on its dismissal
frame; see [Journal pause](../systems/TIME_SYSTEM.md#journal-pause) and
[input routing](../systems/INPUT_HANDLING.md#routing-order). Ordinary modals do
not imply that full pause.

1. Sample physical key edges and reconcile focus/input ownership before queues
   can change it. Register deferred audio servicing and actor-clock reconciliation.
   Native editors release their sessions before unfocused reader returns.
2. Give retained readers and their reserved shortcuts precedence. Atlas/Journal
   keep their gameplay pause, including dismissal; ordinary scene presentation
   and shortcut handling otherwise continue in the established input order.
3. Admit an eligible fresh top-row 0 all-party reset before pending-combat verbs,
   then capture eligible pending-combat and sleeping-Space intentions before
   queue advancement, retaining the owner and blocked/ready state of that sample.
   Reset changes future solo selection, never the currently presented turn token.
4. In `updateWindAndEffects`, publish elapsed time, settle actor-body clocks,
   process wind/moongate/lighthouse state, and update workflow, background and
   feedback queues. Then update moongate presentation, ambient source discovery,
   transient/non-blocking presentation, dungeon effects and the wind display.
5. Revalidate transient input after queues, update status icons, and advance
   any committed automatic-door animation before accepting ordinary input.
6. Update only the highest modal and return while it owns input. Unfocused
   frames advance the permitted passive dialog animations without polling an
   input-owning selector; native editor suspension remains explicit.
7. Revalidate the narrowly admitted camping acceleration request, retain workflow
    input blocks, and resolve pending encounter, combat-scheduler and auto-talk
    ownership before ordinary commands. The request itself adds no command cost.
8. Call `ResumeWaterfallContinuation` after those owners, before secondary input
   or ordinary movement/Pass. A retained surface cascade consumes the frame while
   awaiting/resuming its next fall; an owned tactical encounter keeps normal input.
9. Accept an eligible pending solo verb through its normal direction-prompt route
   before movement, then route secondary command input before primary input.

Pending solo verbs have no status popup, queue overlay or acknowledgement. Capture
and replacement are silent; cancellation/refusal can append one ordinary Output
line. The buffer retains its exact opportunity and release guards, and acceptance
opens the ordinary prompt. See [pending solo input](../systems/INPUT_HANDLING.md#one-pending-solo-combat-verb)
and [early all-party reset](../systems/INPUT_HANDLING.md#early-all-party-combat-reset).

Audio fades/retirement are serviced on exit even when a reader or modal returns
early; source discovery and transport servicing are separate. Reader pauses keep
gameplay elapsed time fixed. Actor-body clock reconciliation consumes that fixed
time rather than advancing gameplay. See the [audio lifecycle](../systems/AUDIO_SYSTEM.md)
and [input ownership](../systems/INPUT_HANDLING.md) for their detailed contracts.

This order lets modal dissolve and dismissal state advance while a workflow owns the global input block. Do not update a modal from both the feature and `GameScene`; the stack is its frame owner.

The waterfall checkpoint observes a core-owned logical continuation, not just
whether `waterfall_hazard` is live. After the first complete fall, the initiating
actor tail runs once and can yield the physical waterfall queue to projectile,
combat or whirlpool ownership. `activeWaterfall` survives that explicit handoff;
scene routing resumes it after the owed phase and before another surface command.
No scene callback or physical finalizer manufactures another turn. See
[cascade ownership](../systems/EVENT_QUEUE_SYSTEM.md#retained-waterfall-cascade)
and [waterfall timing](../systems/TIME_SYSTEM.md#waterfall-completion-and-resumption).

## Dialog Integration

Push dialogs directly by role:

```go
g.dialogStack.PushModalDialog(dialog)
g.dialogStack.PushNonBlockingDialog(feedback)
```

Retain exact instances for cleanup:

```go
g.dialogStack.RemoveWidgetInstance(dialog)
```

Avoid type-based removal and nonexistent pop APIs. Multiple overlays and message dialogs may coexist.

Common event-queue helpers in `gamescene_dialog_helpers.go` return sequence modifiers for confirmation, yes/no, direction, text input, sleep duration, and party selection. Add them with `ThenSequence` so the parent workflow owns and waits for their steps.

### Dungeon View Integration

Gem View, prepared In Quas Wis and its scroll use the same `DungeonViewDialog`
through `thenShowDungeonViewDialog`. The widget consumes a detached current-floor
observation; its north-up 8×8 solid-wall map and matching feature key share one
symbol renderer. Not-revealed hatching and known paired same-level wrap labels
describe the supplied observation, not exploration history or safe routes.
The [Dungeon View widget contract](WIDGET_COMPONENTS.md#dungeon-view) owns the
artwork, dynamic key, responsive layout and documented inspection alternative.

The invoking workflow owns costs and completion. Gem View retains its exact
`DungeonCommandCycle` through fresh Enter/keypad-Enter dismissal and full
end-player completion; prepared and scroll routes keep their own commitment
owners. The scene pushes one exact modal and removes it after dismissal or in
the owner's finalizer. Resize changes presentation resources without changing
the observation, facing, dismissal state or release guard.

## Action Feedback

`SystemCallbacks.UI.ShowActionFeedback` routes to `GameScene.ShowActionFeedback`:

1. If `alsoAddToOutput` is true, append the message once to the main output before checking popup admission.
2. Read the successfully applied **Maximum Action Popups** (`Gameplay.Presentation.MaxVisibleActionPopups`): Off (0) creates no card; 1–3 permit that many, with default 3. Create `widgets.NewActionFeedbackDialog` only when enabled.
3. Attach its scene-owned animation clock, show it, and push it as non-blocking.
4. Measure complete wrapped content and admit it to the shared newest-first action/Search stack, removing oldest exact instances above the configured limit.
5. Place the stack within the surface, dungeon-HUD, or combat-battlefield safe area, evicting oldest cards when complete cards do not fit.
6. Animate rearrangement from current positions while pausing only the affected cards' remaining reveal, hold, or hide budget.
7. Remove the exact instance after its 400 ms reveal, 1600 ms readable hold, and 300 ms hide complete.

Rapid feedback calls remain independently owned and never block gameplay. Fitting cards retain complete content and visible gaps; an oversized newest singleton keeps its beginning visible and clips at the lower feedback boundary. Resize and map-mode changes remeasure and retarget the live stack without refreshing lifetime. Scene replacement and disposal remove every retained exact instance.

The ordinary transient update reconciles a lower applied cap even without another
message or geometry change. This retirement works during reveal, hold and hide,
and while a covering modal pauses the remaining lifetime. Retired cards hide
immediately, leave the dialog stack, and release scene/chest references; Off keeps
no invisible card or pending admission. Empty stacks reset their layout state.
Increasing the cap never resurrects retired cards. Configuration staging, Cancel
and failed Apply do not change live capacity or evict cards.

`ShowSearchFeedback` uses the same admission and layout policy. Search and calls
with `alsoAddToOutput == false` keep their caller-owned narration; the adapter
never conditionally adds text when a popup is suppressed. Calls with `true`
retain history at every limit, including location entry/exit and telescope or
spyglass results/refusals. Identical messages from separate actions remain separate
history entries. See [message ownership](../systems/MESSAGE_OUTPUT_SYSTEM.md#action-and-search-feedback-history).

Chest contents admission retires all scene-owned action-feedback cards. A
successful Jimmy card also retires if its chest lifetime ends without a pending
reveal. [Combat loot feedback](#combat-loot-feedback) distinguishes scoped cleanup
from admission's broader retirement; persistent history survives.
At Off, successful Jimmy capture may have no admitted handles. Modal contents,
current-opening trap notices, summaries, confirmations and their acknowledgement
workflows remain outside this cap. Chest finalization cannot remove later cards,
and closing contents cannot revive cards retired at admission or by a lower cap.

## Smooth Surface Movement

`Gameplay.Presentation.SmoothSurfaceMovement` changes only the camera projection after authoritative movement and synchronous completion. Walking, horses, carpets, manual skiff movement, and furled-frigate rowing use the configured ordinary duration and curve; only horses and carpets apply the 25-percent faster presentation factor.

This camera projection is independent of the runtime outdoor window used by
Blink and temporary Push terrain. Rendering does not scroll or initialize that
gameplay state; [SPELL-0002](../gamefacts/MAGIC_SPELLS.md#spell-0002) owns its
movement, transition and normal-load reset boundaries. Saving and continuing keeps
the live window; actual hydration initializes from the final loaded coordinates
and plane. Window origins and quadrant retention are runtime-only, not serialized
camera state. The two forced waterfall steps use the same step-owned window update.

Automatic raised-sail frigate translations use the selected ordinary curve but finish within the existing 55 ms sailing phase. Heading changes, wind or calm waits, blocked attempts, contacts, hazards, and transitions do not fabricate movement. The automatic-sailing workflow is the only workflow admitted by that specialized projection, and every world layer shares the same wrapped camera offset. Presentation never delays movement completion, clock advancement, world actors, contact, or RNG.

An actual sailing stop increments `AutomaticSailingStopEpoch`. The scene observes
it before input and after queues to clear the camera projection and reject held
arrow/cardinal-keypad movement through physical release, even while projectile
presentation preempts sailing. This does not cancel owed attacks or suppress fresh
Fire/Yell commands at their normal input boundary. The old sailing finalizer also
clears its projection; a sunk frigate's fallback cannot inherit an automatic glide.
See [sailing input](../systems/INPUT_HANDLING.md#automatic-sailing-interruption).

`ShowTimedOverlayMessage` has separate GameScene ownership for one modal timed overlay. `installTimedOverlay` first removes the previously owned exact instance, shows and pushes the replacement, and derives one deadline from `GameState.ElapsedMs`. Expiry, scene replacement, and disposal converge on `clearTimedOverlay`, which cannot remove an unrelated or covering modal. These overlays use `WithExternallyManagedDuration`; their widget `Update` method does not run a competing frame-count timer. New feature code should use the established callback or these scene helpers rather than writing the timed-overlay slot directly.

`ShowTransientIcon` is the non-blocking semantic status channel. `GameState`
emits one of eight fixed kinds after an actual eating, starvation, explicit
healing, poison transition, Wanderer level-up, or clamped moral-karma change.
`GameScene` maps that meaning to presentation and registers it with
`ActiveEffectsOverlay`; gameplay code does not choose the sprite, tint, or
duration. The overlay uses gameplay elapsed time, refreshes repeated kinds,
stacks distinct kinds up to five records, and is cleared only by scene
replacement or disposal. It never enters the dialog stack or event queue and
never owns input.

## Provision Change Feedback

### Ammunition Summary And Shortages

The provisions panel has **four normal-font rows**: compact label/quantity pairs
centered in thirds on the first two rows, followed by ammunition and calendar
rows centered in matching equal halves:

| Row | Left | Middle | Right |
|---|---|---|---|
| 1 | `F` — Food | `K` — Keys | `G` — Gems |
| 2 | `$` — Gold | `T` — Torches | `Ka` — Karma |
| 3 | `Ar` centered in left half | | `Qr` centered in right half |
| 4 | Date centered in left half | | Time centered in right half |

Each resource column reserves a fixed label slot wide enough for both rows and
a fixed quantity slot wide enough for its maximum value. That compact block is
centered in its third. Labels are right-aligned; quantities are left-aligned
after a small scaled gap, giving both rows the same numeric starting X coordinate.
Neither changing digit counts nor `G` versus `Ka` shifts the quantity anchor.
Ammunition blocks reserve
two-digit quantity slots and fixed `LOW`/`OUT` slots, including while healthy,
with a full normal-font space between the quantity slot and the warning;
changing the count or warning cannot shift the label, count, or neighboring block.
Each complete ammunition block is centered in the same half as the date or time
below it. Labels, quantities, status words, date, and time all use the **same
normal font size**, with no smaller fallbacks. The calendar centers each string
within its half, giving the date the extra pixel when the row width is odd; the
largest supported numeric date fits at normal size. The panel retains its
existing height and frame clearance at every resolution.

`Ar` and `Qr` labels stay cyan, and their quantities always stay white. Only the
status word is coloured: yellow `LOW` at 1–20 and red `OUT` at zero, with no
warning above 20. Status words use the same font size as quantities. Other
resource quantities stay white. Ammunition counters are read-only
snapshots of party equipment stock, independent of readied weapons.

`ProvisionSummaryHeight` reserves the four scaled rows. The supplies area grows
upward into the message panel, retaining the bottom inset and the map, party and
status-band geometry. The shortened transcript clip fits **11 visible message
rows**, with 16-character wrapping and the same 64-row buffer and saved plain
output history through resize. History is not a Journal tab.

Scene before/after observation emits one nonblocking action-feedback
notice, also appended to ordinary Output, when stock crosses from above 20 into
1–20 or from positive to zero. It compares
final balances, not gross transaction activity; low-stock increases do not rearm
LOW, while any positive refill allows a later OUT. Activation/hydration silently
baseline stock. These notices use `ShowActionFeedback(notice, true)`, not a modal
or the recent-provisions strip.
Sources: `internal/ui/mainscreen/ammunition.go`, `provision_summary.go` in that
package, and `cmd/ultimav/gamescene_provision_changes.go`.

### Provision Observation

`GameScene` observes detached provision snapshots at entry to and exit from
`Update`, including early returns for merchant modals and workflow input locks.
`internal/ui/mainscreen/provision_changes.go` owns the presentation model;
`provision_change_panel.go` in that package owns its measured `text.Output`
buffers. Observation never changes prices, time, RNG, inventory, or input.

Each quantity stack retains runtime-only gross gained/lost counters in
`internal/party_state/quantity_activity.go`. They count **actual post-limit
changes**, including `Set`, and exclude failed decrements and unchanged writes.
This small amount-level observation seam preserves opposing mutations inside one
command or queue update, which plain before/after balance subtraction cannot do.
Transaction call sites and mutation ordering are unchanged. The counters are not
part of native or legacy saves and must not be used for gameplay decisions.

### Presentation And Coalescing

- The strip is centered in the current **map viewport**, not the entire screen.
  Normal gameplay anchors it near lower-map feedback. Merchant/modal interaction
  or active action feedback moves it to map-top, keeping the lower region
  available. Geometry and modal overlap are re-evaluated on resize.
- Typography scales against the logical **screen width**, independently of the
  narrower map/battlefield placement bounds. Ordinary rows use main-screen body
  text; the food-loss heading retains its larger title size. Fresh feedback uses
  the same typography in exploration and combat.
- A single opaque **Recent provisions** strip is drawn above dialogs only when
  its measured bounds do not overlap the active modal's bounds (merchant parents
  expose their shell; transaction modals expose their own bounds). With unknown
  or overlapping modal bounds it draws underneath the dialog stack. This protects
  tight layouts such as Dungeon View, including its heading near the screen top.
  Small ordinary grids and a simple food-loss notice fit above the merchant
  shell's 10% top edge at supported logical resolutions, including 1280×720.
  A larger combined food/resource card follows the same measured overlap rule,
  preserving choices, receipts, and keyboard hints. With no modal, the strip
  draws above the stack on surface, dungeon, and combat scenes.
- Resource order is Food, Gold, Keys, Gems, Torches, Skull keys. Only changed
  resources appear, ordinarily as `Food +20 -6  Now 218`. Actual food losses use
  a featured normal-title-size `FOOD -6` heading, with remaining stock and any
  simultaneous gross gain on separate body-size lines. Other resources retain
  their compact grid beside it, or below it when width requires stacking. This
  is recent actual loss, not a last-meal record or consumption-rate prediction.
  Labels, signed gains, signed losses, and current values use existing distinct
  semantic dialog colours.
  The signs and names carry the meaning independently of colour.
- Up to six resources appear. The compact resource grid uses at most three
  measured columns, ordinarily one or two rows plus a heading; featured food
  occupies its own block beside or above the remaining grid. Long cumulative numbers
  reduce the column count instead of clipping, abbreviating, or shrinking text;
  that exceptional taller strip yields to any overlapping active modal.
- Each resource has a **six-second idle timeout** on `GameState.ElapsedMs`.
  Time obscured by an overlapping or unmeasurable modal still counts toward expiry;
  dismissing the dialog does not restore an expired receipt.
  A further actual change refreshes only that resource and adds to its separate
  gain/loss accumulators. Food gained and eaten in the same update therefore
  remain `+N -M`, even when the final balance is unchanged. Repeated eating uses
  one entry instead of producing a notification backlog. `Now`/`Remaining` report
  the latest observed balance, not the balance when the entry first appeared.
- One transaction's changed resources appear together on the next observation.
  This is a bounded recent summary, not an ordered transaction history. There
  are no animations, sounds, acknowledgement keys, queue ownership, or focus
  changes. Draw and resize do not refresh lifetime.
- Activation silently baselines loaded/initialized quantities. State replacement,
  a rewound clock, and reset activity counters discard prior feedback; scene
  replacement and disposal clear it. Native hydration creates a new `GameState`
  and never replays loaded stock as a reward. Live developer inventory changes
  are observable like other live mutations, while candidate-scene setup is
  absorbed by activation's baseline.

### Provision Feedback Context Boundaries

Old receipts belong to the interaction that produced them. Cleanup is
presentation-only and never resets ammunition-shortage observation or inventory.

- A change of location, floor, or combat instance silently adopts current
  provisions as the new baseline. This covers town/dungeon entry and exit,
  stairs, camping-map setup/return, combat entry (including ambushes), and combat
  exit before its summary. Ordinary movement within the same map preserves feedback.
- Moongate/teleport source-renderer cleanup clears receipts even for a destination
  on the same map and floor.
- Opening the atlas or quest journal immediately clears receipts. Their frozen
  scene clocks cannot preserve old receipts for redisplay after reading.
- Entering the first modal clears already-displayed rows while preserving fresh,
  unobserved activity generated by that interaction. This includes NPC/merchant
  conversations, rest setup, and Dungeon View. Nested approval/result dialogs
  preserve fresh transaction feedback; they do not restart its timeout.
- `HideAllOverlays` clears and suppresses provision feedback until
  `ShowAllOverlays` silently baselines it again. Map fades and the finale also
  suppress feedback, keeping bed rest, death/resurrection, and other faded
  scripted presentations free of stale receipts. Suppressed changes are not
  replayed afterward. Fresh changes after the boundary remain eligible.

`cmd/ultimav/gamescene_provision_lifecycle_test.go` covers transitions, ordinary
movement, nested modals, frozen readers, teleport cleanup, and scripted suppression.
The production tavern purchase tests continue to verify fresh merchant receipts.

### Combat Loot Feedback

Combat chest discovery uses `CombatLootDialog`: immutable admitted icon/text
pairs accumulate at 500 ms intervals, with measured keyboard scrolling for longer
lists. Each row centres the icon and measured wrapped text block on a shared
vertical centre. A fresh Enter/Escape after the reveal acknowledges the list; the workflow
cleans its exact dialog and completes its deferred token once. Discovery leaves
the items on the ground. Persistent display is stable in original slot order:
the lowest ordinary-object slot wins; when only fields/dead markers remain, the
highest slot wins. Fields share that selection despite their separate overlay
pass, and are suppressed at the temporary reveal position. Chest priority and
continuous icon cycling are absent. Removing the displayed object exposes the
next winner without changing object slots, chest knowledge, or command targeting
and Get order. Terrain concealment applies to both the pile and preview.
Trap consequences appear above the discovery list and in the battle report.
The report records each discovery at the same reveal cadence, explicitly as an
item on the ground; an empty chest is reported without implying an acquisition.

Successful combat chest Jimmy captures every newly admitted, still-surviving
action-feedback handle from that synchronous action. The association retains the
exact combat instance, encounter ID, selected chest position and non-reused object
lifetime ID; it is not inferred from message text, widget type or a reusable object
slot. Jimmy selects the last matching chest while Open selects the first, so
co-located chests retain independent lifetime associations. Immediately before
pushing contents, the workflow clears all scene-owned action cards, including
Search, Look, failures and other chest cards. They cannot reappear afterward.
Unrelated workflow-owned dialogs, the current trap notice and transcript/report
history survive. Open returns the removed chest's detached
identity. A pending reveal retains that exact snapshot until modal admission;
object absence alone must not retire its card during this handoff. Empty Open
results create no contents modal and leave no pending reveal: feedback for the
removed owner retires on reconciliation. Door Jimmy, failed disarms and
known-safe refusals do not acquire this successful-chest association.

Expiry, capacity eviction, external removal and scene cleanup forget the exact
handle. Removed objects without a pending matching reveal and ended/replaced
encounters retire stale associations even while another modal pauses ordinary
feedback lifetime. Readmission assigns a fresh lifetime ID even for identical
contents at the same position and original slot; repeated cleanup cannot remove
the replacement's card. Pending-reveal cancellation also retires its captured
cards; it does not purge unrelated cards before admission or newer cards during
finalization. This changes presentation only, leaving
keys, trap/loot RNG, action completion and contents acknowledgement with their
existing owners. The scope and production Jimmy-to-Open handoff are exercised in
`cmd/ultimav/gamescene_combat_chest_feedback_test.go`.

Combat Get publishes one `CombatEventPickup` with a detached actor name, target
position, item identity, and actual post-cap quantity. The full-sized **Battle
Report** renders labelled `LOOT`, `PICKUP` (a map change that adds no stock, such
as a borrowed wall torch) or `PICKUP FAILED` entries instead of map-overlay
receipts. Ordinary player/enemy movement is discarded before admission to the
64-event report history; teleportation, fleeing, and other consequences remain.
Pickup gains already reported there are absorbed from the provision observer's
gain baseline, preventing a second map-overlay receipt. Unrelated gains and all
losses in that same update remain observable.

Pickup, discovery, and trap entries receive a 2400 ms scene-clock accent. Only
time with their complete entry exposed in the report counts; modals, party-spell
targeting, and browsing elsewhere preserve it. Rendering and resizing do not
consume the budget. `F3` jumps to the oldest pending result (or the latest result
when none are pending), acknowledges its pending marker, and renews its accent.
New events preserve a browsed endpoint. `End` returns live; neither key consumes
an action, gameplay time, or RNG.

`CombatState` separately accumulates actual collections by item identity in
first-acquired order. Revealed treasure, failed/capped-out acquisitions, later
consumption, and report eviction do not inflate or erase those totals. Victory
leaves the encounter open for looting; actual departure snapshots the totals into
`CombatConclusion` before clearing combat state.

GameScene queues ordinary conclusions until the exit/recovery workflow and other
modals finish, then automatically presents `NewCombatSummaryDialog`, a bounded
scrollable readout using `CombatLootDialog`. Fresh Enter/keypad Enter/Escape
dismisses the exact instance. `F4` at primary input reopens the last presented
summary without a gameplay action. Summary retention is scene/session-local;
scene replacement and disposal clear pending, visible, and retained summaries.
The special Doom-mirror finale keeps its existing handoff instead of opening a
collection modal.

The completed-battle summary is a lower, map-centred card: its reference width
is 540 logical pixels at 1280 screen width, scaled with the UI and capped to the
available viewport. Its bottom edge is fixed at 88% of screen height, clamped
inside the viewport. Measured content grows upward to half the viewport height,
then scrolls within a clipped body while the heading and acknowledgement stay
visible. Quantities are right-aligned, long names wrap without shrinking the
body font, and scroll hints appear only for overflow. Empty totals use a compact
`No items collected.` state without the inventory notice. Chest discovery
retains its own reveal layout. The summary's stepped border uses the standard
dialog pixel unit independently of its larger content padding, so its frame
does not become heavier when the map pane expands.

Implementation entry points are `internal/game_state/combat_collection.go`,
`cmd/ultimav/gamescene_combat_report.go`, and
`cmd/ultimav/gamescene_combat_summary.go`; their `_test.go` files cover accounting,
history/attention, and summary ownership respectively.

### Combat Interruption Feedback

`GameState` publishes `CombatEventInterruption` when remembered melee interference
actually rejects a physical source or Cast. It preserves the classic
`"<creature> interferes!"` output and records the exact interrupter, interrupted
actor/clone, and weapon or casting context. Manual Attack, its confirmation
recheck, and Tab's source loop publish through the same boundary; previews never
record attempts. Rejection keeps the existing remaining-source and turn rules.

`recordCombatEvent` admits one Battle Report entry and requests
`SoundCombatInterrupted` once. Both styled and plain report text explain who
interrupted whose action. History uses admission-time names, allegiance and
disclosure; invisible interrupters remain `Unknown`, including concealed detail
coordinates, even after later reveal or slot reuse. Rendering, wrapping and
history navigation never request another sound or mutate gameplay. The manifest's
`combat_interrupted_01` is a named `file: null` slot; assigning its future recording
enables playback through the existing routing after a rebuild.

### Finite Mutation Coverage Matrix

All rows reach the same quantity-stack observation seam, including aliases to
the quantities. Production entry points below are in `internal/game_state/`
unless otherwise qualified.

| Resource | Gain paths | Loss paths |
|---|---|---|
| Food | `tavern_shop.go` meals, rations, and actual charity; `action_get.go` food/stack pickups; `dungeon_interactions.go` chest food; `effect_spells.go` CreateFood; `death_resurrection.go` minimum food restoration | `internal/party_state/inventory.go` meal consumption (surface, dungeon, combat, camping, bed/inn rest); `combat_monster_hit.go` gremlin theft |
| Gold | `action_get.go` stack/loot pickups; `dungeon_interactions.go` chests; `arms_shop.go` sales; `action_bridge_trolls.go` signed transfers | Arms, guild, reagent, horse, ship, healer, inn, and tavern `*_shop.go` payments; `shadowlord_spawn.go` shop/street theft; `guard_extortion.go` tribute/half-gold outcomes; `party_karma.go` donations/conversation payments; `shrine_service.go`; `wishing_well_service.go`; `action_bridge_trolls.go` |
| Keys | `guild_shop.go`; `action_get.go` stacks; `dungeon_interactions.go` chests | `action_jimmy.go` and `dungeon_interactions.go` committed key use; `guard_arrest.go` and `blackthorn_interrogation.go` confiscation; `shadowlord_spawn.go` theft |
| Gems | `guild_shop.go`; `action_get.go` stacks; `dungeon_interactions.go` chests | `action_view.go` surface/dungeon View; `shadowlord_spawn.go` theft |
| Torches | `guild_shop.go`; `action_get.go` stacks/wall torches; `dungeon_interactions.go` chests | `action_ignite.go`; `shadowlord_spawn.go` theft |
| Skull keys | `action_get.go` stacks; conversation grants | `action_use_commits.go` committed Skull Key use |
| All six, common boundaries | `internal/party_state/inventory.go` provision insertion; `cmd/ultimav/game_action_callbacks.go` conversation grants; live `debug_inventory_locker.go` edits | Quantity `Set`/decrement methods, including live `world_setup.go`/`starvation_scenario.go` setup actions |
| Silent/no-change boundaries | Legacy/native hydration, candidate setup, cancellation, rejected/stale transactions, saturated no-op grants | Failed decrements, empty-resource attempts, unchanged `Set`, zero-quantity changes |

A failed **action** may still commit a real cost (for example a used Jimmy key),
and a refused ration order may grant actual charity. Feedback follows the
quantities, not the action's success flag; it must show those real changes.

Regression coverage lives in `internal/party_state/quantity_activity_test.go`,
`internal/ui/mainscreen/provision_changes_test.go`,
`internal/ui/mainscreen/provision_change_panel_test.go`, and
`cmd/ultimav/gamescene_provision_changes_test.go`. These cover gross movements,
caps/no-ops, all six real pickups, real consumption, Britain ration approval and
results, modal update ordering, save hydration, expiry/reset/disposal, rapid
opposing changes, and supported layouts. Rendered composition regressions in
`cmd/ultimav/gamescene_provision_composition_test.go` cover four ordinary entries
over Dungeon View's heading, tight measured/unmeasured modals above a merchant,
and unobstructed merchant feedback. Visual gameplay review should include a
Britain purchase with its result dialog still open, a pickup, consumption, and
rapid repeated changes at minimum resolution.

## Message Output And MessageDialog

`MessageCallbacks.AddRowStr` always writes to the right-side output. If the highest modal is a visible, non-static `MessageDialog`, `GameScene.addRowStr` also mirrors the text into that dialog.

`appendLineBuilderDualOutput` returns a `LineBuilder` that writes styled content to the top `MessageDialog` and plain text to the persistent game output. When no `MessageDialog` is active, it returns the main output builder.

Map and Journal updates use ordinary main Output notices with normal History
retention. Their scene-local queue coalesces pending same-site changes and places
map updates before the coalesced journal notice, flushing only after both
frame-entry and frame-exit command/modal/workflow blockers clear. Reader hints
style keys and descriptions separately. No timed top-centre discovery overlay or
extra rendering layer remains; see [shared notices](WIDGET_COMPONENTS.md#shared-map-and-journal-notices).

Camping uses the current screen callback bridge:

- `CreateAndPushMessageDialog`, a non-returning notification that creates and pushes the dialog
- `HideMessageDialog`
- `PopMessageDialog`
- content and prompt mutation callbacks
- fade-duration query and configuration callbacks

These callbacks operate on the top modal and exist for the current camping integration. Direct feature code should retain an exact `MessageDialog` pointer when it can do so, avoiding ambiguity if another modal is pushed.

## SystemCallbacks UI Boundary

Current UI-facing callback groups include:

- `MessageCallbacks` for persistent text output and prompts
- `UICallbacks` for timed overlays, action feedback, signs, and transient icons
- `ScreenCallbacks` for display refresh, overlay visibility, and the camping MessageDialog bridge
- `FlowCallbacks` for presentation workflows grouped by consequence, rest, world interaction, observation, Shadowlord, item, and sacred-site concerns
- `TalkCallbacks` for creating and pushing the concrete linear conversation dialog
- `CombatCallbacks.PublishEvent` for presentation-neutral combat history

`UICallbacks.ShowQuestJournalDossier` is mandatory. `ActionQuestJournal`
passes it a detached projection; `GameScene.ShowQuestJournalDossier` prepares
release debouncing, pushes one `QuestJournalDialog`, and removes that exact
instance on dismissal. Browsing never calls time, dungeon-cycle, turn, or
combat-action completion.

`UICallbacks.ShowZStatsDossier` follows the same detached, read-only ownership
boundary across exploration, dungeon, and combat contexts. Its eight-section
header uses measured label widths, responsive typography, and explicit spacing
so static and dynamic labels remain separated at every supported resolution.
Supply details come from the embedded inventory catalog and explain each
provision's source-backed gameplay purpose without changing quantities or
mechanics.

Counted catalog rows include parenthetical quantities, including zero for visible
catalog entries. Spells show prepared counts. Armory and character equipped-slot
rows show full equipment names at compact typography; available/readied counts
belong in armory details rather than the uncounted equipment rows. Measured wrapping
reserves glyphs, first-line quantity suffixes and the scroll gutter before laying
out the complete name. Special possessions retain their owned-only filter and
selection follows typed identity across refresh/reordering.

Moonstone entries carry explicit optional typed phase data, with phase zero a
present New Moon rather than absence. The Items list uses a small phase glyph;
details use a compact framed phase icon beside the full heading and category/
quantity summary, replacing the ordinary large catalog portrait allocation only
for moonstones. Integer nearest-neighbour scaling follows bounded header/pixel
units, not spare pane space. New Moon retains its outline. The fixed compact header
stays visible while long lore pages below it; other item portraits remain intact.

Spell details keep a compact, content-measured summary beside the portrait.
Circle, MP, and prepared count share a summary row; the formula state remains
visible. The detail body begins below both the portrait and summary. Canonical
spell details fit one page at supported resolutions; genuinely oversized bodies
retain Space paging without paging the summary. Single-page details omit the
Space hint and page indicator. Coverage is in
`internal/ui/widgets/ztats_catalog_regression_test.go`.

Mandatory callbacks are supplied as complete named values and checked during initialization. Optional indicators explicitly documented as optional may be omitted; do not generalize that policy to required callbacks or install mandatory callbacks later by mutating the callback set.

## Render Order

`GameScene.Draw` currently composes these major layers:

1. Map image with camera shake.
2. Moongates, item stacks, world quest items, map units, and Avatar. In combat,
   threatened-target underlays precede actors and actor-local effects; direction
   cues and HP/allegiance frames follow. The complete active-turn indicator then
   draws above HP frames, followed by eligibility brackets and the selected aiming
    cursor. The current party entry on ARCH gets only its hollow contour and active
    frame in that foreground pass; body and HP remain concealed. See
    [combat aim and active selection](../systems/VISUAL_EFFECTS_SYSTEM.md#combat-aim-and-active-selection).
3. Default tile dissolves.
4. Map fade.
5. Over-fade tile dissolves.
6. Screen flash and screen inversion.
7. Debug overlays and combat HUD.
8. Static borders, the right-rail status panel, wind, transient map icons, clipped game output, party summary, and compact provisions.
9. The provision-change strip when the active modal's bounds overlap or are unknown;
   all dialog-stack entries from bottom to top; then the strip
   when the active modal has verified free space for it (or there is no modal).
10. Kapow/combat impact effects and the active projectile on the non-surface, non-dungeon path.
11. Debug text.

On ordinary top-down surface maps, world effects and projectiles instead render
before the terrain visibility mask, debug overlays, transient icons, static UI,
and dialogs. See [final world composition](../systems/VISUAL_EFFECTS_SYSTEM.md#final-composition).
Only the non-surface, non-dungeon path retains impact/projectile layers above
dialogs. Change either order only as an intentional presentation decision backed
by tests.

Screen shake is applied to world presentation, not the static HUD and dialog layers.
Within combat, fields, cone footprints, utility target cues and aiming markers
use the square transform; actor artwork and actor-local cues use real exact-entry
motion. A field/list ordinal must never stand in for a monster identity. See
[actor and square transforms](../systems/VISUAL_EFFECTS_SYSTEM.md#combat-actor-and-square-transforms).

## Combat HUD

`CombatHUD` is a non-modal GameScene component and never enters `DialogStack`. Core combat publishes `CombatEvent` records through required `CombatCallbacks.PublishEvent`. GameScene keeps encounter-local history and renders it beside a non-mutating turn forecast.

The forecast must not advance combat turns or consume gameplay RNG. The HUD remains static during battlefield shake while battlefield-local impact labels follow world transforms.

Show Combat HP Bars defaults On. Off hides battlefield HP artwork and its attached
allegiance outline while preserving actor pixels, independent Ally/Enemy text,
status/selection/target cues, insect member pips and actual-fleeing `<>` markers.
Numeric report damage and floating numbers remain in both modes. Off additionally
shows the recorded [original monster injury band](../gamefacts/COMBAT.md#combat-0008)
for disclosed positive surviving monster damage; PCs/clones, fatal, zero-loss and
status-only results have no extra injury sentence. Core events capture severity,
identity and disclosure at resolution. History never consults later HP or a reused
slot. Toggling the preference changes suffix visibility, not historical content;
styled/plain measurement and paging share the same suffix policy. Target-only
field reports do not treat an absent attacker as monster slot zero.

Combat terrain admission is a presentation filter, not a queue filter. A monster
on authored `BlackSquare`/`BrickWallArchway` terrain retains its turn forecast
slot, scheduling, and actions, but the HUD omits its portrait and identifying
detail. By default, `Gameplay.Combat.ShowConcealedTurnOrderSlots` preserves that
blank forecast slot. Players may disable the preference to omit and compact
concealed entries within the same four-action forecast; this does not inspect
beyond the forecast or mutate scheduling. A concealed player-controlled
monster's command prompt uses the neutral `Unknown` label. The same admission
rule suppresses ordinary actor-local battlefield decorations. Only the current
party entry's [ARCH contour and active frame](../systems/VISUAL_EFFECTS_SYSTEM.md#active-party-entry-on-arch)
have a narrow presentation exception; it grants no HUD identity, HP or targeting
disclosure. A terrain trigger immediately restores the ordinary identity
presentation without recreating the actor.

The combat sidebar uses a fixed **45% Turn Order / 55% Events** split via
`combatHUDTurnHeightPercent`, retaining the existing border overlap. This is
separate from the gameplay right rail's unified status row. Current PC names use
`combatEntityName` in both normal and auto-pass presentation, including the full
`CLONE` identity. The compressed NEXT queue retains `combatEntityLabel`
abbreviations. Current actor placement measures wrapped text as well as sprite
height before placing equipment readouts and NEXT.

`appendCombatAttackReadout` consumes the detached core preview through
`GameScene.combatAttackPreview`. Before aiming it lists each attack source and
its own effective range as `(RN)`; during physical aiming it lists only the
current source, the target name with distance `(DN)`, and a concise
presentation-only shot status. The origin says `Choose target`; an unnamed
square says `No one seen`, preserving disclosure rather than revealing occupancy.
Complete equipment and target labels wrap to the measured pane width at the
normal HUD font size. `prepareCombatAttackReadout` reserves their full physical
height before placing NEXT. The target itself labels the aiming row without an
additional `Aim:` prefix. The core `CombatShotStatus` values remain unchanged.
It does not collapse mixed sources to their longest range or predict hit chance.
Input-blocked workflows, enemy/auto-pass turns, and unrelated targeting suppress
the assistance readout. The battlefield uses the same authoritative preview for
[eligibility brackets](../systems/VISUAL_EFFECTS_SYSTEM.md#combat-attack-eligibility).
Eligibility brackets use the approved compact within-cell foreground foot-corner
placement, above monster artwork but below the reticle and clear of HP/status
cues, avoiding neighboring sprites' occlusion of an underlay. Composed-render
and exhaustive supported-resolution row/layout/name assertions are in
`cmd/ultimav/gamescene_combat_marker_composition_test.go` and
`cmd/ultimav/gamescene_combat_attack_preview_test.go`. Manual acceptance should
also exercise long names, multiple sources, resize, HiDPI, and crowded fights.

## Unified Status Rail

`ActiveEffectsPanel` owns the existing status band's outer bounds. Construction,
`initializeResizeableVisualElements`, and `syncRightRailLayout` all pass
`rightRail.activeBorder`; `calculateStatusBandLayout` preserves the established
interior padding as one shared row. Actually worn artifacts compact from the
left, while ongoing effects compact from the right. Both groups use the same
measured cell size and retain a clear empty gap, with no separator, section
captions, inactive wells or midpoint rule. Band height, party summary and map
viewport stay fixed. The provisions
footer's [four-row layout](#ammunition-summary-and-shortages) takes its additional
height from the message panel, whose transcript has 11 visible lines,
16-character wrapping, clipping, and the same history buffer through resize.

`GameState.GetWornArtifactStatuses` returns a detached fixed array in **Crown,
Amulet of Lord British, Black Badge** order. These are party-level Use toggles,
not ordinary character equipment. The renderer selects only entries whose
authoritative `Worn` flag is true and compacts them in that order. Unowned and
owned-but-unworn artifacts have no slot, subdued preview, or corner marker. An
active toggle still renders when ownership is inconsistent: ownership and worn
state remain separate facts, and presentation neither infers wearing from
quantity nor repairs gameplay state.

`wornArtifactImage` in `cmd/ultimav/active_effects_panel_worn.go` uses Crown sprite
437, royal Amulet sprite 439, and the existing cached
`text.GetBitmapFonts().IBM.GetCharImage(0x1D)` for the Badge. The Badge is the
original 8x8 worn-status font symbol, not a guessed world tile, generic star, or
custom badge image. Character-sheet compatibility is described by
[GFX-0005](../gamefacts/GRAPHICS_FORMATS.md#gfx-0005); active Badge state remains
distinct from ownership as in [CRIME-0001](../gamefacts/CRIME_PRISON.md#crime-0001).
The panel adds no original-data asset. Wrapped sprites are cropped to their
16x16 artwork; glyphs/artwork retain aspect ratio and nearest-neighbor sampling.
Worn and effect entries share the compact Effects scale. Their artwork is
unboxed and does not expand to fill unused space; only the outer rail owns a
complete frame.

The Effects group derives `GetPersistentEffectStatuses` on each draw, in
category order: torch, magical light, **Crown suppression OR one duration
spell**, intoxication, automatic Shadowlord influence, starvation, and active
frigate HMS Cape. Its presentation omits `PersistentEffectCrownSuppression`
because the worn Crown at
the left already communicates that permanent inherent benefit; gameplay still
projects the semantic effect. Other effects retain deterministic order and
compact from the right with stable cell size and spacing. No entry is silently
truncated. Seven is a conservative projection bound, not proof that all seven
categories are reachable together through ordinary gameplay. Tiny defensive
bounds retain projection entries but cannot promise readable artwork below the
configured minimum.

The effect row reserves an additional right gutter, scaled from the content
height, before both groups calculate their shared cell size. Worn artifacts
retain their left anchor. Lone and full effect rows use the same inset edge.

A worn Crown therefore appears exactly once in the rail. This presentation-only
deduplication does not assert that Crown mechanics are identical to In An or
remove `PersistentEffectCrownSuppression` from authority. Donning Crown clears
the duration spell; activating any duration spell removes Crown. Removing or
expiring a replacement never resumes its predecessor. Magical light remains
independent, as do Amulet and Badge; all three artifacts may be worn together.
Normal artifact sentinel 255 is permanent. Wearing does not imply missing
Amulet gameplay protection or add
speculative Amulet/Badge effect entries.

The rail is display-only: no focus, input, popup, numeric timer, callback
registration, gameplay mutation, clock advance, or RNG draw. Effect artwork
retains its tint and animation but has no under-icon accent bar or surrounding
box. Atmosphere remains part of the detached gameplay projection; this panel
does not render its former pulsing underline. Active
`smashed` intoxication uses a magenta potion with danger atmosphere after the
Crown/duration category; decrements retain one entry, zero/fresh-town reset
removes it, and native continuation restores it. The Tavern's separate `drunk`
purchase counter is not presented.

The panel draws in the static rail wherever that rail is present, including
combat, dark first-person dungeons, vehicles, camping, and shrine/Codex views.
It is independent of Avatar visibility, smooth movement, world shake, and the
terrain mask, while intentional whole-rail suppression and modal covering remain.
The Avatar-attached artifact renderer and its sprite-list projection are removed,
not retained as a compatibility option. Activation/removal flashes and feedback,
world quest-item rendering, and the separate transient `ActiveEffectsOverlay`
remain live. Native hydration supplies authority, not persistent UI registrations.

Projection, active-only compaction, pixel sources, absent under-icon accents, shared-cell geometry,
seven-entry containment, context, read-only, and resize assertions are in
`internal/game_state/persistent_effects_test.go`,
`internal/game_state/toggle_item_state_test.go`,
`cmd/ultimav/active_effects_panel_test.go`,
`cmd/ultimav/gamescene_draw_core_test.go`,
`cmd/ultimav/gamescene_output_layout_test.go`, and
`cmd/ultimav/gamescene_initialization_test.go`. Manual acceptance should cover
supported resolutions, HiDPI, aspect-fit containers, darkness, movement/shake,
and modal coverage; execution results belong in the relevant issue.

## Resolution Changes

`Game.LayoutF` preserves fractional outside dimensions in DIPs, multiplies by
the current Ebitengine display scale, and chooses an aspect-safe 16:9 drawing
surface. The canvas is at least 1280×720 for layout/readability and at most
3840×2160 to bound rendering buffers; smaller/larger outputs scale that canvas.
It publishes the dimensions through `DisplayManager`. `DrawFinalScreen` records
the actual drawable pixels (zero before the first observed frame),
clears the physical target to black and uniformly centers that logical surface;
non-16:9 windows and monitors therefore produce stable bars rather than stretch
or crop game content.

`Game.Update` observes platform display state once per frame and invalidates the
active scene when mode, window, monitor, container, or logical dimensions
change, including scale-only changes. Native monitor movement does not itself
recenter the window or save a new pixel-size preference. `Draw` compares the logical size with `lastCheckedResolution` and calls
`initializeResizeableVisualElements` when required. That method rebuilds
GameScene-owned borders and scaled fonts while preserving the main transcript
buffer. It also recalculates one bounds-driven right-rail layout shared by the
status rail, clipped output, and provisions. Long-lived cached-layout
dialogs call their dimension-remembering `ensureLayout()` from update and draw,
rebuilding only presentation-derived resources while retaining workflow state.
Other dialogs that survive a resolution change may expose `Refresh`; ownership
code is responsible for invoking component-specific refresh behavior where
needed.

Use the canonical `display.GetManager()` identity, percentage placement, shared
typography helpers, and actual panel bounds. `GetCurrentSize` is the logical
layout size; use explicit manager APIs when platform window, monitor, or
container dimensions are required. The game loop updates that same manager so
registered callbacks observe changes. Do not call Ebitengine window-size APIs
from core gameplay.

## Integration Rules

- Keep gameplay mutation in `GameState` or workflow steps, not widget drawing code.
- Use required callbacks directly after validated initialization.
- Push every interactive dialog through `DialogStack`.
- Remove exact instances; hidden does not mean unowned.
- Let GameScene update modal and non-blocking lanes exactly once.
- Use `text.Output` for all text rendering.
- Use `ThenSequence` when the parent workflow must await a dialog helper.
- Use `Finally` for guaranteed dialog, audio, input, and visual cleanup.

See [UI Widget System Architecture](UI_WIDGET_SYSTEM.md),
[Widget Components](WIDGET_COMPONENTS.md), and [Modal Dialog System](MODAL_DIALOG_SYSTEM.md).
