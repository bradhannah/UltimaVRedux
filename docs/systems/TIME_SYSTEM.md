# Time System

The game has two separate time domains. They must not be conflated: elapsed presentation time schedules updates and effects, while Britannian calendar time advances only through gameplay actions.

## Central Elapsed Time

`GameState.ElapsedMs` is the shared elapsed-millisecond value consumed by event queues, gameplay-adjacent animations, wind cadence, overlays, and other time-based systems. `GameScene` is the platform adapter that samples elapsed wall time and copies it into `GameState` during update.

Core systems consume the supplied value rather than sampling wall time themselves. Tests can therefore set `ElapsedMs` directly and advance event queues deterministically. Elapsed milliseconds do not advance the Britannian date or charge an action cost.

Purely cosmetic presentation and resource-lifecycle code may sample wall time directly when it cannot affect gameplay state, ordering, or randomness. Current examples include dialog fades and cursor blink, audio fades and cleanup, transient render-only effects, and position-offset sprite overlays. Gameplay timing must not use those clocks.

### Shared RNG Ownership

`GameState` owns one shared stream using a directly imported `Source` from
[Berad RNG](https://github.com/bradhannah/berad-sdk/blob/891cf44f456a/modules/berad-rng/README.md).
The SDK owns PCG mechanics, `Intn`/`Shuffle` consumption, pointer-stable reset, and
logarithmic-time restore. Redux owns seed policy, dice/inclusive-range helpers,
`AsRandomProvider`, ordering across consumers, and update-thread serialization.
The SDK introduces no clock, global RNG, implicit seed, or game policy.

Native saves retain `RandomSnapshot.Seed` and `SourceCalls` unchanged. The latter
counts raw source draws, including rejected bounded samples, rather than API
invocations. Capture observes the pair without consuming it; hydration restores
the next value before wiring environmental consumers. `SetRandomSeed` resets the
existing pointer so borrowers remain attached. The original x/exp/rand stream
and shuffle callback order remain compatible. SDK tests own algorithm vectors
and jump correctness; game bridge tests own provider and save continuation.

### Elapsed Time And Shared RNG Scope

[ADR-001](../ADR/ADR-001-central-gameclock.md) specifies centralized time inputs,
not fixed-step or seed-only replay. `GameScene.updateWindAndEffects` publishes
`ElapsedMs`, advances wind, runs queues, then updates effects. Wind's
`AdvanceThrough` processes every due 55 ms tick using the **shared gameplay RNG**,
including during combat presentation waits. There is no isolated wind stream.
Longer actual waits can therefore admit more wind ticks and change later rolls,
although waiting alone adds no Britannian calendar minutes.

Combat signature effects record `StartedMs` at event admission. Gate phases
each measure their own admission against `GameState.ElapsedMs`; a hitch cannot
skip a phase or let the workflow expire it before effects update. They never inherit the preceding
frame's age. Their no-extra-RNG contract is evaluated at the same supplied
elapsed/action timeline, accounting for due wind ticks and authoritative action
draws; it does not promise duration-independent replay.
`cmd/ultimav/gamescene_combat_signature_timing_test.go` exercises the production
wind/queue/effect update ordering after a 700 ms hitch, retaining a newly admitted
gate's authored-at-1x 125/600/125 ms phases and original turn token for Dragon
and Daemon summoners, including captured-rate cases. Its oracle includes
`WindSystem.AdvanceThrough`, gate creation draws and next-RNG equality.

### Combat Presentation Speed

Combat uses one canonical preference, `GetCombatSpeedFactor()`: 0.5x, 1x, 1.5x,
2x, 3x, 4x, 6x, 8x, 12x, 16x, 20x or Ludicrous, default 1x. `combatPresentationTiming` captures a validated
value for the committed action. Included authored millisecond durations are
divided **once**, rounded to the nearest millisecond (positive halves upward),
with zero preserved and a 1 ms technical minimum for positive work. A zero-valued
policy or detached fixture deliberately defaults to 1x. Other invalid nonzero
factors are programming errors at this boundary; configuration owns validation
of explicit preferences. No old debug factor is multiplied in.

| Stage | Authored at 1x | At 4x | At 20x |
| --- | ---: | ---: | ---: |
| Ordinary / targeted enemy anticipation | 250 / 1000 ms | 63 / 250 ms | 13 / 50 ms |
| Scheduled turn hand-off (same-frame review) | 16 ms | 4 ms | 1 ms |
| Ordinary enemy movement | 650 ms | 163 ms | 33 ms |
| Player or controlled-monster movement | 333 ms | 83 ms | 17 ms |
| AI bat/mongbat anticipation / movement | 75 / 175 ms | 19 / 44 ms | 4 / 9 ms |
| Condition-driven party auto-pass | 250 ms | 63 ms | 13 ms |
| Tab non-projectile source hold | 1250 ms | 313 ms | 63 ms |
| Floating result label (300 ms floor) | 1250 ms | 313 ms | 300 ms |
| Minimum projectile flight | 100 ms | 25 ms | 5 ms |
| Chest reveal interval / victory departure per member | 500 / 180 ms | 125 / 45 ms | 25 / 9 ms |

**Ludicrous** is stored as `config.CombatSpeedLudicrous` (100, percent 10000)
and is the only choice that removes presentation. Every enemy anticipation,
scheduled hand-off, auto-pass pause and Tab source hold is zero; enemy, player
and controlled-monster movement glides are not started, and missiles resolve
their impact without flight (`animatesMotion` and `pauseMs`). Everything else,
including spells, signatures, impact bursts, labels, chest reveal and victorious
departure, keeps its 20x duration (`config.CombatSpeedFastestAnimated`). A
scheduled enemy turn then completes in the queue update after it is reviewed,
so consecutive enemy turns start one 60 TPS frame apart; at 20x they take about
five frames. The skipped stages are presentation only: they draw no random
numbers and change no rule, though, as at every speed, a shorter elapsed
timeline can shift the wind's shared draws.

Enemy turns and condition-driven auto-passes release their input block in the
same queue update that completes them when no unprotected key is held. The next
scheduled actor reviewed in that frame adds the scaled 16 ms hand-off, replacing
the one 60 TPS frame the release formerly took: 1x pacing is unchanged and faster
rates shorten the gap between actors. A held key still defers the release, and a
later review adds no hand-off because it has already spent its frames.

The same snapshot covers deferred anticipation, every source of a multi-source
attack, outbound/impact/return travel, and all phases of a committed Cast/Use or
victory departure. Speed changes affect the next action. Capture occurs before
synchronous resolution can publish effect events. Deferred closures retain the
value explicitly; a narrowly scoped, restored synchronous resolution context
lets event consumers capture it without a mutable workflow-wide current rate.
Manual attack snapshots are guarded by encounter and targeting identity.

Combat spells, finite signatures, lunge/jolt/strike motion, division, vanish,
field-dispel and combat impact bursts use the same policy. Renderer phase
boundaries and workflow animation progress use the admitted effective durations.
Concurrent impacts, division, disappearance and combat Kapow bursts timestamp
admission with `GameState.ElapsedMs`; age is the elapsed time since that timestamp,
not the preceding frame's delta. Queue publication before the same frame's effect
update therefore starts at age zero even after a hitch. Repeated updates at the
same timestamp cannot age these effects.
Shared world projectile and spell helpers retain 1x unless explicitly in the
combat presentation path. The projectile's authored 100 ms minimum is applied
before scaling, with no second perceptibility floor.

Floating result text divides its **1250 ms reading lifetime** by the admitted
rate but never shortens below **300 ms**, independently of the Tab source hold.
Movement-bound fire still admits at 50% progress and keeps its post-arrival
450 ms tint with the same scaled label budget. Reports, action-card
reveal/hold/fade, status/selection/idle pulses, targeting, held-key protection,
player acknowledgements, polling and audio keep their own clocks. Camping,
death/resurrection narratives, bridge-troll narration and dungeon-owned
pre-entry contact are outside this tactical policy.

Neither `ElapsedMs` nor the event queue is globally rescaled. Admission, hitches,
polling and human input mean total wall-clock action length need not have an exact
speed ratio. Wind continues every due shared-RNG tick as described above; shorter
waits can change subsequent gameplay rolls without changing rules or action costs.
Queue-only timing tests are not cross-speed production replay proof.

The field-spell continuation retains its existing lone dynamically inserted
`Wait(200)`. That leading-only delay has no action node to anchor it and is
ineffective; the focused boundary regression preserves the 1x baseline rather
than silently introducing an additional hold.

### Journal Pause

`GameScene` owns one Journal session independently of top-widget focus. Opening
latches the last published `GameState.ElapsedMs`; `getElapsedMs` returns that
frozen value until the owner closes. `GameScene.Update` branches before finale,
wind, queues, automatic doors, and gameplay routing, updating only safe modal/
audio presentation and the cosmetic notice. Skipping every gameplay queue is
necessary: freezing deadlines alone would still allow ready callbacks to run.

Tab/search changes never release the pause. Dismissal rebases the scene elapsed
epoch to the frozen value, removes the exact dialog, and returns for that whole
frame, so neither wall-time catch-up nor closing-key gameplay occurs. It does
not alter wind phase, RNG, queue deadlines, or reactivate the scene. Disposal
cleans session ownership without acknowledging departing checkpoint markers.
The Draw FPS limiter uses independent cosmetic time so the frozen gameplay
clock cannot freeze Journal rendering. This policy is Journal-specific, not a
change to every modal or a restriction on deliberate developer Workbench actions.

Implementation: `cmd/ultimav/gamescene_quest_journal.go`,
`cmd/ultimav/gamescene_input.go`, `cmd/ultimav/gamescene_initialization.go`, and
`cmd/ultimav/gamescene_draw_core.go`. Injected-clock
assertions are in `cmd/ultimav/gamescene_quest_journal_test.go`.

## Britannian Calendar

`datetime.UltimaDate` stores:

```go
type UltimaDate struct {
    Year   uint16
    Month  byte
    Day    byte
    Hour   byte
    Minute byte
    Turn   uint32
}
```

The Britannian calendar uses 13 months, 28 days per month (364 days per year), 24 hours, and 60 minutes. Month 12 advances to month 13; month 13 advances to month 1 of the next year. `Advance(minutes)` increments `Turn` once per invocation, applies the calendar rollover logic, and clamps a single request to nine hours.

Calendar consumers include NPC schedules, food checks, visibility, lighthouse behavior, moon phases, moongates, and date/time display.

GameState applies temporal modifiers before normalizing to the calendar's
maximum interval. Calendar, carried-light fuel, camp cooldown and returned elapsed
minutes all use that same accepted duration. Positive requests replace the prior
hour observation even under Time Stop; zero requests leave it alone. Player updates
observe one current/previous-hour pair, never replay a backlog of elapsed hours.
Rest can overwrite an unprocessed crossing; sailing interruption returns to command
input without an extra update. See [PROV-0002](../gamefacts/PROVISIONS_CONSUMPTION.md#prov-0002).

## Action Costs

Authentic action cost is a gameplay rule, not a property of `ElapsedMs`.

- `GameState.AdvanceGameplayMinutes(minutes)` applies an explicit command or workflow cost through the authoritative calendar domain.
- Normal surface routing calls `CompleteSurfaceCommandCycle` or `CompleteSurfaceMovementCycle`: ordinary completion advances the calendar, runs player-turn effects, then runs the world-actor phase. `FinishTurn` delegates surface work to that seam when needed and remains a compatibility/combat boundary elsewhere.
- Small-map movement costs one minute.
- Large-map completion applies terrain or vehicle movement cost. Raised-sail wind and HMS Cape timing belongs to the automatic sailing wait cadence; the eventual accepted movement receives ordinary outdoor completion.
- Combat advances one calendar minute at each tenth selected actor action through the DEX turn queue.
- Camping advances five game minutes per camping cycle; its normal 200 ms or
  one-way accelerated 50 ms presentation delay is independent of that cost.
  Space changes only subsequent inter-cycle scheduling for the current sleeping
  phase. A previously scheduled interval may finish, and a hitch never skips or
  catches up logical cycles. Setup/Bard and results/Wanderer timing is separate.
  Production wind still consumes the shared RNG before queues, so differently
  paced rests do not promise unconditional seed-only result equality.
- A complete merchant Talk owns one minute, including repeated family actions
  or cancellation. Inn Rest additionally owns its overnight steps; the normal
  Talk minute is still applied during final workflow cleanup after the Inn
  result is acknowledged. An accepted Redux harbour escort is a separate
  verified transition that adds three minutes, for four total; unavailable or
  cancelled travel adds nothing and never affects the vessel purchase.
- `ProcessTurnEffects` owns a complete non-calendar player turn. Ship repair
  instead calls its narrow world-actor phase before each explicit five-minute
  advance so ordinary hazards and durations are not repeated. Those special
  advances also skip meal processing; the one ordinary completion owns the
  resulting food/starvation check.

Individual action methods may charge time directly and therefore must not also receive an unconditional `FinishTurn` from the input layer. The authoritative original-game cost matrix and known deviations belong in [TIME_COSTS.md](../gamefacts/TIME_COSTS.md); this document describes how costs are applied.

### Automatic Sailing Boundaries

`CompleteAutomaticSailingWait` is distinct from ordinary movement completion:
an eligible wait requests two gameplay minutes and its outdoor actor phase, or
one minute with the HMS Cape's alternating actor gate. Normal temporal modifiers
still apply. The scene's 55 ms cadence determines when that phase may run, not
an additional calendar cost. Each wait/movement
boundary revalidates an outdoor, raised-sail frigate and live party. Invalidated
travel stops without another wait, movement, actor phase or gameplay RNG draw.

Furling stops authority before feedback. Validated tactical entry stops and furls
while preserving facing, including stationary and player-initiated encounters;
normal return leaves sails down. A successful outdoor enemy ranged hit instead
stops active automatic sailing with the surviving frigate's sail state/facing
intact. Sinking stops before replacing the vessel or applying drowning. Warning,
rotation, obstruction and failed firing opportunities do not create ranged-hit
interruption. The original comparisons and deliberate differences remain in
[SHIP-0001](../gamefacts/SHIP_MECHANICS.md#ship-0001),
[SHIP-0003](../gamefacts/SHIP_MECHANICS.md#ship-0003) and
[SHIP-0005](../gamefacts/SHIP_MECHANICS.md#ship-0005).

Stopping is not a new command cycle and does not repeat the player update.
Already-spent waits remain spent; a separately dispatched command keeps its own
completion cost. The current descending actor phase still resolves every owed
lower-slot hit once, including later sinking or fatal damage. Its retained
projectile results drain without another sailing phase or damage roll. Terminal
outcomes take precedence; a surviving fallback is not moved by the old sailing
loop. Fresh movement after physical release is required to resume travel.
Rough-seas damage has its own [workflow owner](EVENT_QUEUE_SYSTEM.md#rough-seas-and-travel-ownership),
not a sailing wait or an extra ordinary command completion.

### Inn Calendar Maintenance And Overnight Time

Every calendar advance through `advanceElapsedGameplayMinutes` compares the
before and after Britannian month ordinal. For each crossed 28-day month
boundary, all 16 roster records with `MonthsAtInn < 25` increment once; values
25 and above remain unchanged. This global maintenance applies to ordinary
gameplay time and Inn time, including year rollover. It deliberately mirrors
the original all-roster update rather than filtering to companions currently
stored at an Inn.

A successful `RestAtInn` uses `advanceInnRestMinutes`, a dedicated elapsed-time
path with this exact order:

```text
successful payment and Faulinei resolution
-> move to the Inn sleep position and mark Good members Sleeping
-> clear the active duration spell
-> advance 5 minutes twelve times
-> while hour != 6: process eligible regeneration rings, then advance 9 minutes
-> restore morning HP/MP/status and move to the wake position
-> later, complete the owning Talk workflow's one-minute cleanup
```

Each 5- or 9-minute invocation advances the calendar/turn counter, global Inn
month counters, lighting, magic-light minutes, and hourly `LastCamped`
maintenance. It intentionally skips ordinary hourly food consumption and does
not apply Quickness reduction or Time Stop suppression. Cancellation, refusal,
Leave, Pick Up, and Preview never use this overnight path and preserve the
active duration spell. If Rest leaves the whole active party Dead, the workflow
hands off to resurrection after Inn cleanup and skips the separate Talk minute.

### Dungeon Command Cycles

Dungeon exploration does not use `FinishTurn` directly. `GameState` issues an
opaque command-cycle token when a recognized dungeon command is accepted.
Ordinary commands apply their precharge immediately. Cast and Use reserve a
deferred token and charge it only when a final valid attempt commits, so their
rejection and cancellation are free. Every token is completed exactly once with
one of two modes:

- command-only completion retains the precharge without full dungeon
  end-player processing, as used by facing changes;
- full completion runs the implemented common dungeon end-player phase without
  charging another minute, as used by advance and back-up attempts even when
  blocked. Wandering actors, fields, rooms, and hazards extend that phase in
  their owning dungeon children.

Quickness alternates charged/actor-active and free/actor-inactive cycles. Time
Stop suppresses both the precharge and dungeon actors, while a full completion
still decrements its duration. Tokens are transient and stable save capture is
permitted only between command cycles. Native snapshots retain
`quicknessSkipNext`, so loading resumes the exact next charged/free phase.
They do not store the command token or sequence. Supported versions and promotion
rules belong in the [snapshot contract](../details/SAVE_SNAPSHOT_SCHEMA.md#snapshot-version-compatibility).

Redux begins the cycle when an accepted command key is received rather than
before idle input polling. This avoids retaining a token while the player is
idle. Most accepted commands preserve source-style rejection/cancellation
costs; Cast and Use deliberately defer charging until final confirmation.

## Turn Effects

`ProcessTurnEffects` handles the non-calendar work associated with a completed
turn, including a separable map-specific NPC/enemy phase, environmental hazards,
wake checks, duration effects, toggle items, and party-wipe detection. Standing
hazard mutation completes synchronously; its flash is presentation-only feedback
and may coalesce without changing gameplay.

Normal surface completion advances the owning command's calendar minutes,
runs one ordered player update, then the actor phase. That player update owns
town wake checks before standing hazards, the Codex guardian and random Underworld
earthquake on large maps, poison, observed-hour food/starvation, durations,
toggle items, the noncombat Regeneration Ring sweep, and party-wipe detection.
Completed, rejected, and cancelled surface results enter this seam by default
when `ConsumesNormalCycle` is true. Explicitly free, deferred, still-active, and
map-transition results bypass it. Camping's internal five-minute steps
deliberately bypass it and its owner invokes one final player update. Waterfall
admission retains the initiating actor phase until the
[first complete fall](#waterfall-completion-and-resumption).

Slow outdoor terrain is a distinct owner: its extra actor phases run before its
extra calendar advance, followed by ordinary completion. Each extra phase's
direct contact resolves before the next phase. Tactical combat or a gameplay
workflow retains the remaining phases and time costs until the existing
deferred-contact input boundary resumes them. This movement continuation is
transient, rejects native capture while pending, and is cleared on shutdown;
neither the interrupted phase nor its costs are replayed. Living physical retreat
reported as `CombatDefeat` resumes lower outdoor actors and the owed command
tail after restoration and presentation ownership release; actual party death,
terminal handoff, cancellation or invalid origin clears it. The production
keyboard/retreat/summary-dismissal regression in
`cmd/ultimav/gamescene_outdoor_retreat_continuation_test.go` verifies a very-slow
step completes four extra minutes plus two ordinary minutes exactly once on the
subsequent scene update. Blocked outdoor movement has no completion; blocked town
movement still does. Door timers run
before town actor gates even under Time Stop. An expired automatic door timer
remains pending while the tracked doorway is occupied and retries before later
actor phases; an NPC that vacates during a phase permits closure on the next
update. Automatic sailing waits, repair,
camping, and transition suppression retain their separate completion contracts.

Horse/carpet cadence skips alternate actor phases without skipping ordinary
calendar/status work. Town gates riding before Time Stop and Quickness; Space
Pass bypasses only the town riding gate. Outdoors, Time Stop and Quickness gate
before riding, including Pass. Town riding and Quickness phases reset on a new
town visit, not a floor change; outdoor phases survive town visits and foot
travel does not reset riding phase. Native saves persist these independent phases;
see [snapshot schema](../details/SAVE_SNAPSHOT_SCHEMA.md).

Outdoor actor execution follows [ENEMY-0006](../gamefacts/ENEMIES.md#enemy-0006): generation first,
then descending NPC slots with an attack check immediately before each actor's
movement, then cleanup. An attack (including an obstructed fireball) suppresses
all later movement in that phase; later actors still evaluate attacks. An actor
that moves into contact waits for its next eligible phase to attack. Wrapped
nearby actors survive cleanup at map seams.

Sea Serpent/Dragon fireball rolls and adjacent carpet/skiff damage belong to
this same gated phase, including Underworld Pass. Direct contact and ranged
party/hull damage resolve authoritatively in actor order. Detached ranged
results share the pirate projectile workflow's FIFO presentation lane, without
additional actor phases or damage in animation callbacks. Tactical contact
retains the existing pending-encounter handoff at the completion boundary.
On-foot whirlpool contact commits damage in actor order and presents a captured
result. Vehicle contact suspends the descending scan while its owning workflow
performs the hit/transport at fade-out. After fade-in, the same phase resumes
below the saved slot on the current world's controller, without repeating
generation, player updates, or cadence gates. No old-map actor pointers cross
the handoff. Prior attacks continue to suppress movement, but lower-slot
attacks use the resolved position, vehicle and HP. The continuation is owned
by the live workflow and is discarded on cancellation, never run by cleanup.
Reusable transport steps in `internal/game_state/whirlpool.go` install no
finalizer. The outdoor owner composes vortex-audio/fade cleanup and phase
cancellation in one `Finally`, so adding transport steps cannot replace the
continuation's cleanup.
See [outdoor attack rules](../gamefacts/ENEMIES.md#enemy-0006).

Time stop deliberately separates these concerns:

- Turn-owned durations continue to process.
- Calendar advancement is suppressed if Time Stop was active at completion entry.
- Actors check Time Stop after the player update, so its final duration decrement
  permits that completion's actor phase (subject to mounted/Quickness gates).
- The time-stop duration itself still counts down.

Combat field effects and DEX scheduling are handled by combat-specific paths rather than by the small/large-map branch.

### Waterfall Completion And Resumption

At consumed surface completion, including stationary Pass, Redux first applies
the initiating command's calendar charge and player update. A qualifying waterfall
then retains the one owed actor tail. Blocked outdoor movement and explicitly free
commands do not create this checkpoint. Repeated completion requests while the
logical cascade is retained cannot charge that command again.

The ordering is:

```text
initiating calendar charge and player update
-> any already admitted movement-owned rough-sea work
-> first complete fall: two south steps, presentation, damage, optional plane travel
-> initiating actor tail once, including its retained contacts/presentation
-> review the current landing for further automatic falls
-> ordinary surface input after the cascade ends
```

`completeFirstWaterfallActorTail` marks completion before callbacks can suspend
the phase. The ordinary actor cadence gates still apply at that boundary. If the
phase owes a projectile, tactical encounter or whirlpool, the physical waterfall
queue releases its input block while `activeWaterfall` retains the logical
cascade. Existing outdoor cursors finish lower actors without repeating generation
or cadence. Living retreat from an owned encounter can resume; actual party death,
terminal takeover or invalid ownership cancels the continuation. A validated owned
whirlpool may change its retained landing. The pre-input checkpoint resumes only
after those owners finish, before an ordinary movement or Pass can intervene.

Later falls perform only their own two steps, damage and optional plane travel:
no new ordinary minute charge, food/status sweep or actor phase. Elapsed waits
schedule presentation and do not themselves advance the calendar. Wind retains
the [shared-RNG timing scope](#elapsed-time-and-shared-rng-scope). If eligibility
disappears before the first step without a takeover, the already consumed command
still runs its owed actor tail once.
The two forced steps retain their 255 ms spacing, but no additional 2.5-second
post-step dwell. Damage feedback and required transitions still complete under the
cascade owner. Its notice expires independently, allowing fresh-key movement after
the cascade and owed actors finish without waiting for the card to disappear.

Both forced southward steps call `MovePlayerOneStep` separately, so each updates
the runtime outdoor window; the special plane transition initializes it at the
destination. Neither operation supplies another ordinary turn. The window and
quadrant-local Push rules remain in [SPELL-0002](../gamefacts/MAGIC_SPELLS.md#spell-0002).

Redux checks the wrapped raw-map tile immediately south at consumed admission and
landing rechecks. This is not the original standing-tile consumed checkpoint, and
equivalence with its visible-buffer pre-input south-cell test is unverified.
A visited `(location, floor, position)` trigger stops a wrapped cycle at the
already committed landing with one notice. This bound does not establish original
predicate parity. [ENV-0003](../gamefacts/ENVIRONMENTAL_HAZARDS.md#env-0003) retains
those comparison limits; [cascade ownership](EVENT_QUEUE_SYSTEM.md#retained-waterfall-cascade)
defines release, cancellation and shutdown cleanup.

## Daylight And Lunar State

- `IsDayLight` is true from 05:00 through 19:59.
- `IsFullDaylight` is true from 06:00 through 19:59.
- `GetVisibilityFactorWithoutTorch` interpolates at hour 5 and hour 20 and returns the supplied nighttime baseline outside daylight.
- `GetCurrentMoonPhase` is the compatibility display helper and returns Trammel's phase; `GetTrammelMoonPhase` and `GetFeluccaMoonPhase` expose each moon explicitly.
- `GetMoongateMoonPhase` follows routing rules by selecting Trammel before noon and Felucca from noon onward.

## Key Files

- `internal/datetime/ultima_date.go`
- `internal/game_state/turn.go`
- `internal/game_state/inn_shop.go`
- `internal/game_state/dungeon_command_cycle.go`
- `internal/game_state/game_state.go`
- [Berad RNG source contract](https://github.com/bradhannah/berad-sdk/blob/891cf44f456a/modules/berad-rng/docs/API.md)
- `internal/game_state/rng_sdk_bridge_test.go`
- `cmd/ultimav/gamescene_update.go`
- `cmd/ultimav/gamescene_callbacks.go`
- `docs/gamefacts/TIME_COSTS.md`
