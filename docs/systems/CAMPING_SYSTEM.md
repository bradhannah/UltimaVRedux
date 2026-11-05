# Camping System

Camping is a blocking gameplay workflow that temporarily presents either the
outdoor camp from `BRIT.CBT` or a generated dungeon arena, advances Britannian
time, processes contextual rest events, and restores the original exploration
location. Ambushes transfer into tactical combat only after camping releases
map and input ownership.

Original timing, rest, food, healing, and encounter evidence belongs in
[Camping Mechanics](../gamefacts/CAMPING_MECHANICS.md). Original frigate repair
rules and Redux differences belong in [Ship Mechanics](../gamefacts/SHIP_MECHANICS.md).

## Entry

`ActionHoleUpLargeMap` handles the large-map command:

- A frigate routes to ship repair instead of camping.
- Outdoor camping permits foot, horse and carpet parties on eligible land.
  Horses retain their mounted state. `AddCampingSequenceToBuilder` revalidates
  the location, terrain and vehicle at runtime after duration/watch selection,
  then atomically stows a ridden carpet before starting camp. Full inventory
  refuses without loss; cancellation before commitment leaves the carpet
  mounted. A stowed carpet stays carried after rest or ambush, including death
  and resurrection. Water remains forbidden, skiffs remain rejected, and frigates
  retain repair. This convenience deviation is documented in
  [Camping Mechanics](../gamefacts/CAMPING_MECHANICS.md).
- Normal play asks for duration and guard selection through flow callbacks.
- Guard selection distinguishes **No One** from **Escape**. Explicitly accepting
  No One commits an unguarded camp; Escape abandons setup before any camp map,
  rest cycle, healing, food, or camp RNG work. On the surface, that cancellation
  also skips ordinary command completion. A duration cancellation is likewise
  uncommitted. This does not undo the dungeon command's existing precharge.
- `GameScene.thenExecuteCampingSequence` captures the original `PlayerLocation`, loads map 0 from `BRIT.CBT`, and creates a `CampingScene`.

The sequence is inserted into its owning workflow with `ThenSequence`; it does not start a nested camping workflow.

Ship repair has separate ownership: its blocking workflow performs five narrow
large-map actor phases and five-minute advances, waiting for any owed pirate
attack before checking that the party remains on an anchored frigate. It then
repairs the hull. An adjacent encounter releases workflow ownership, retains a
transient repair continuation through combat, and resumes before that cycle's
anchored-frigate check. The retained outdoor actor cursor finishes lower-slot
contacts and projectile presentation first, even if a second battle interrupts
the same phase. It never repeats generation or mount/Quickness gates on return.
Living retreat reported as `CombatDefeat` resumes the owed repair tail. Actual
party death, terminal handoff, cancellation or invalid origin discards this
transient work. Repair-only calendar advances skip meal processing.
Deferred surface completion supplies the one ordinary hazard/status/duration
pass and final two minutes.

Dungeon `H` claims the accepted `DungeonCommandCycle`, then requests 1-9 hours
with Space/`0` cancellation. Escape at the subsequent guard selector also cancels
setup rather than choosing No One. `PrepareDungeonCampingPlan` snapshots the active
dungeon, builds the generated arena, and consumes setup RNG before
`AddDungeonCampingSequenceToBuilder` enters camping. Cancellation and normal
rest complete `DungeonFullEndPlayer`, retaining the precharged dungeon-minute
policy; an ambush retains the same token through
combat and completes it only after dungeon restoration.

### Guard comparison and confirmation

Both outdoor and dungeon guard prompts explicitly enable the character selector's
`GuardResources` layout. Its detached opening snapshot shows Name, current/max HP,
current/max MP and Condition for all active party members, including unavailable
rows. All six party rows plus **No One** fit together; No One has blank resource
and condition cells. A measured explanation below the roster and the control
footer retain their space when inspecting another row or resizing. These are
current resources, not predicted rest gains. Reopening takes a fresh snapshot.
The content-sized card is centred inside the map area. Measured columns stay
grouped on large/ultrawide windows; extra workspace stays outside the card. Width
precedes explanation wrapping, selection never resizes it, and narrow screens keep
the existing two-line fallback.

Arrows and number keys inspect without confirmation; Enter accepts an actionable
or warning row, and Escape cancels. No resource-based guard recommendation or
automatic choice is made. Good members can stand watch; their explanation notes
that guarding forfeits ordinary rest HP/MP recovery. Poisoned members remain
warning rows saying **A poisoned member cannot keep watch.** The scene rechecks
Good status when accepting a character; a non-Good selection camps without a
watch, as the original does. Other unavailable rows stay inspectable but cannot
confirm. No One also starts an unguarded camp.

Escape backs out of setup with the surface/dungeon cancellation costs above. The
original instead camps unguarded on a cancelled guard prompt; this is a recorded
deviation. See [CAMP-0001](../gamefacts/CAMPING_MECHANICS.md#camp-0001)
and [detached selectors](../ui/WIDGET_COMPONENTS.md#detached-selectors).

Projection, detached snapshots, measured full-party layout and input/workflow
ownership are covered by `cmd/ultimav/gamescene_guard_resources_test.go`,
`cmd/ultimav/gamescene_camping_cancellation_test.go` and
`internal/ui/widgets/character_guard_resources_test.go`.

## Map And Scene Model

`TransitionToCampingMap` creates an 11 by 11 `LayeredMap` through
`ResetAndCreateCombatMap`, changes the virtual location to `CampingLocation`,
and centers the player at `(5,5)`. Dungeon camping supplies a generated map with
wrapped current/neighbor-cell geometry, source camp formations, a contextual
center campfire/feature, and no fixed-room trigger slots.

Responsibilities remain separate:

- `LayeredMap` owns terrain loaded from `BRIT.CBT` and uses the normal tile rendering pipeline.
- `camping.Map` owns party positions, the moving guard position, campfire position, and monster entry points.
- `CampingScene` owns requested hours, five-minute cycles, interruption state, original location, and wanderer presentation state.

Both outdoor and dungeon scenes use exact five-minute cycles and 12 cycles per
hour. They skip intermediate ordinary player updates and food checks; the
owning surface or dungeon command performs one final player update.

`CampingLocation` maps to `CombatMapType` for rendering, but entering it does not create `CombatScene`, a DEX turn queue, or combatants.

## Workflow

The parent workflow runs at normal priority with input blocked and guaranteed cleanup. The current sequence:

1. Enters the camping map, clears active duration magic and active-player selection, puts eligible active party members except the guard to sleep, hides exploration overlays, pauses map music, and starts wind. Fire crackle starts only when the presented center tile is a campfire.
2. Starts the selected Bard guard's temporary lute performance and sprite override when applicable, then awaits `BardMusicDurationMs` and calls `finishBardGuardPerformance` before entering any logical cycle.
3. Opens the logical sleeping phase and repeats cycles with a 200 ms presentation
   delay. A fresh Space press latches 50 ms intervals for the remaining sleep.
4. In each cycle, applies the noncombat Regeneration Ring sweep and checks the preceding nonterminal hour crossing for ambush. If uninterrupted, advances five Britannian minutes and processes guard wandering. It does not run intermediate ordinary player or provision updates.
5. After uninterrupted camping, resolves eligible ordinary healing in party order. For outdoor camping longer than five hours and with `LastCamped < 1`, then checks the one-in-four Wanderer appearance roll without a once-per-month gate. A forced debug request bypasses the hour and cooldown gate.
6. If selected, applies Wanderer restoration/level handling after those preceding rest rolls.
7. Restores sleep status and the saved exploration location, then lets the owning command perform one ordinary final player update.
8. In `Finally`, stops camping audio, restores the exact exploration location, resumes map music, restores overlays, clears sprite overrides, and releases `CampingScene` and input ownership.

Gameplay randomness uses `GameState` RNG helpers, and presentation timing uses `GameState.ElapsedMs` plus event-queue delays.

### On-demand sleeping acceleration

Each outdoor or dungeon camp begins normally. During logical sleeping, a fresh
**Space** press silently accelerates the remaining cycles. There is no hint box,
status box, or acknowledgement. Releasing Space leaves that camp fast; further presses are
idempotent. Setup/fades, Bard music, results, ambush combat, and Wanderer
presentation retain their existing timing and ownership. This is a Redux
presentation control, not an original-game command or a saved preference.

`CampingScene` owns the sleeping phase and one-way acceleration latch.
`RequestCampingAcceleration` changes intent only. Both workflows use
`RepeatWithRuntimeDelay`, evaluating the delay at admission and between cycles.
An already scheduled 200 ms interval may finish; following intervals use 50 ms.
Deadlines advance from execution time, so a slow frame never catches up by
draining multiple sleep cycles. All required five-minute cycles, ring sweeps,
guard rolls/steps, hour-crossing checks and final completion remain authoritative.

The scene admits only a fresh Space edge while the exact sleeping workflow owns
input, after readers and top modals. A Space held through setup cannot accelerate;
an accelerating press is suppressed through release, including at the final
cycle or ambush handoff. Raw result/Wanderer acknowledgement still requires a
release and fresh press. Acceleration adds no dialog, message, or sound.
Completion, interruption, finalization and scene teardown discard pacing state; the next camp
starts normally. Audio playback rates and cleanup owners are unchanged.

Controlled camping RNG inputs yield identical ordered camping decisions at
normal and fast speed. Production wind uses the shared gameplay RNG before
queues, so shorter actual sleep durations admit fewer wind ticks and can change
later random results. Acceleration does not reseed, rewind, discard draws, or
change either global clock. See [elapsed-time RNG scope](TIME_SYSTEM.md#elapsed-time-and-shared-rng-scope).

The Bard wait belongs to the camping workflow, not an independent feedback lane
or a timer checked inside `processSingleCampingCycle`. During the wait, logical
cycles, calendar, regeneration, guard and ambush RNG are stationary. Isolated
workflow tests preserve camping-local draw order across update increments;
production wind still shares the main RNG during waits, as described in
[elapsed-time RNG scope](TIME_SYSTEM.md#elapsed-time-and-shared-rng-scope).
[CAMP-0004](../gamefacts/CAMPING_MECHANICS.md#camp-0004) defines the original
before-loop ordering; Redux's duration is a presentation choice, not a
seconds-equivalent claim for the original 52 update/delay iterations.
Incidental original update RNG is not ported into this wait. Cancellation runs
the same finalizer, stopping the lute and removing the guard override once,
without advancing a cycle or queuing an ambush.

Ordinary `No effect...` and `Party rested!` outcomes use the compact, centred
`CreateAndPushCampingResultDialog` screen callback, after authoritative healing
and the Wanderer check. The same workflow owns acknowledgement and cleanup.
Wanderer conversations retain the larger six-line message dialog. Dungeon
camping shares the compact result presentation without changing its turn token.

## Combat Integration Boundary

Camping already consumes combat assets:

- `BRIT.CBT` supplies map tiles, authentic party positions, and monster entry positions.
- `CampingLocation` renders through `CombatMapType`.
- `references.GetCampingAmbusher` and the eight-slot camping table provide the weighted encounter source data.
- `QueueOutdoorCampingAmbush` prepares a complete `CombatState` while camping still owns input, including source-order group, friend, position-shuffle, and DEX rolls.
- The pending resolver commits that state only after the camping `Finally` has restored exploration and released workflow ownership.
- `CampingCombatOrigin` lets `EndCombat` restore the pre-camp active character and run ordinary party-wipe handling after the map is authoritative again.
- Dungeon origins additionally retain the opaque dungeon command token and complete `DungeonFullEndPlayer` after every combat outcome restores the exact dungeon square and exploration state.

The camping workflow never starts combat directly because `StartCombatEncounter`
rejects active gameplay workflows. A successful roll queues the prepared state,
marks camping interrupted, suppresses healing/Wanderer/completion presentation,
and lets guaranteed cleanup run. The normal depth-zero encounter resolver then
commits camp-map combat. All tactical conclusions return to the same exploration
position; only an actually dead persistent party starts the death sequence.

No gameplay continuation is owned by a presentation callback. Save snapshots
and debug combat exit reject both pending and active camping continuations.

## Rendering And Audio

The standard map renderer draws camping terrain. Camping-specific overlays draw sleeping party members, the active guard, campfire, and wanderer state.

Audio currently includes camping start, looped fire crackle, wind, bard lute, guard footsteps, ambush alert, rest/no-effect feedback, healing-related feedback, and wanderer/level effects. Cleanup restores the exploration audio and overlay state even when camping is interrupted.

Campfire audio follows the actual center terrain after the camping-map transition.
Outdoor camps and generated dungeon camps with a campfire retain the loop;
dungeon center features such as ladders and chests replace the fire and remain
silent. The shared workflow finalizer always stops fire crackle while restoring
exploration and before handing off to combat, including cancellation. Ambient
exploration audio is suppressed separately while camping owns the scene.
Regression coverage is in `internal/game_state/camping_audio_test.go`.

## Key Files

- `internal/game_state/action_hole_up_camping.go`
- `internal/game_state/action_hole_up_dungeon.go`
- `internal/game_state/action_hole_up_repair.go`
- `internal/game_state/action_camping_ambush.go`
- `internal/game_state/combat_camping_encounter.go`
- `internal/game_state/combat_dungeon_camping.go`
- `internal/game_state/action_camping_wanderer.go`
- `internal/game_state/game_state.go`
- `internal/camping/camping_scene.go`
- `internal/camping/camping_map.go`
- `internal/map_state/layered_maps.go`
- `internal/references/combat_map_reference.go`
- `internal/references/combat_map_dungeon_generated.go`
- `cmd/ultimav/gamescene_hole_up_dungeon.go`
- `cmd/ultimav/gamescene.go`
- `cmd/ultimav/gamescene_tiles.go`

Map-transition coverage is in `camping_transition_test.go` and
`layered_maps_combat_test.go`; deterministic no-ambush, guarded/unguarded
handoff, RNG order, save/debug rejection, and every combat return are covered in
`action_camping_ambush_test.go`.

`internal/game_state/action_hole_up_camping_determinism_test.go` compares
per-cycle calendar, guard, HP and RNG traces under different elapsed-time
increments, including hour-boundary/ambush behavior and cancellation during the
Bard introduction. `internal/game_state/outdoor_actor_continuation_test.go`
covers repair spanning consecutive battles and lower-slot projectile feedback.
