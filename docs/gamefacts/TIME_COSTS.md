# Time costs and update ownership

These rules distinguish requested game minutes, actual calendar advancement,
player updates, actor dispatches, and presentation updates. They describe the
reviewed DOS behavior within ordinary valid state; they do not certify every
edition or arbitrary large time requests. Original random bounds do not establish
the generator's distribution. Redux comparisons are scoped to the named paths.

### TIME-0001

**Calendar advancement and temporal modifiers**

**Kind:** rule
**Uses:** none
**Original:** Established
**Redux:** Unverified

The calendar has 60 minutes/hour, 24 hours/day, 28 days/month, and 13 months/year.
For a positive request `n`, Quickness changes the request to
`max(1, floor(n / 2))`. The previous-hour comparison byte is set to the current
hour. Unless Time Stop is active, add the effective minutes and subtract them
from both carried-light counters, saturating fuel at zero. Hour rollover also
decrements the camp cooldown. The first month rollover clears the saved
opening-campaign flag and resets daily search markers.

The original minute-to-hour rollover uses one conditional, not a general loop
over arbitrarily many elapsed hours. A zero request skips the advancement block
but can recalculate lighting and conditionally refresh the sky/clock; it is not
equivalent to doing nothing. Spell-duration updates have another owner and are
not universally counted in minutes.

Redux's `advanceGameplayMinutes` preserves the positive-request Quickness and
Time Stop rules, while `advanceElapsedGameplayMinutes` uses general calendar
advancement bounded to 540 effective minutes per call. That accepted interval is
used consistently for calendar, fuel, cooldown and the returned duration. Hour
observations follow PROV-0002 rather than replaying elapsed hours. Nonpositive requests return without
the original presentation refresh. The ordinary small-increment arithmetic is
compatible, but large increments, refresh ownership, and hunger bookkeeping
must not be described as universal equivalence.

**Implementation:** `internal/game_state/turn.go`

### TIME-0002

**Context owns command completion**

**Kind:** rule
**Uses:** TIME-0001
**Original:** Established
**Redux:** Intentional deviation

| Context | Original calendar owner and ordering |
| --- | --- |
| Town/small map | Accepted completion requests one minute; party-status work follows, then eligible actor work. |
| Overworld/Underworld | Accepted completion requests two minutes; terrain/events and player update precede the final monster phase. |
| Dungeon exploration | Normally requests one minute before input; completion separately controls end-player work. |
| Combat | Requests one minute before every tenth eligible actor dispatch; see [cadence](#time-0003). |
| Camping | Internal five-minute requests; see [camp loops](CAMPING_MECHANICS.md#camp-0003). |
| Bed | Internal ten-minute requests followed by player updates. |
| Inn | Separate initial and overnight loops. |

The generic dispatcher defaults to completion: cancellation inside a helper does
not automatically make a command free. Town completion requires a remaining
nonzero map and an eligible party result; that test distinguishes Good/Poisoned
members and Sleeping members, rather than simply checking any non-dead status.
Some helper return paths do not define a caller-visible completion value; no
guaranteed zero-cost rule can be assigned to those paths.

Dungeon Quickness alternates the pre-command advance; Time Stop suppresses it.
This cadence gate is separate from the central minute arithmetic.

Redux surface routing retains ordinary one/two-minute completion until an
asynchronous command finishes. It advances time, then player effects, then
actors. **REDUX DEVIATION: TIME-0002:** dungeon cycles begin on an accepted key
rather than before waiting for input, avoiding an idle in-flight command. Cast/Use
reserve a token but defer charge until valid confirmation, so cancellation and
rejection remain free. Other accepted dungeon commands retain their cycle cost.
Exact-once tokens keep presentation dismissal from charging another turn.

**Implementation:** `internal/game_state/turn.go`, `internal/game_state/dungeon_command_cycle.go`

### TIME-0003

**Persistent combat actor cadence**

**Kind:** rule
**Uses:** TIME-0001
**Original:** Established
**Redux:** Unverified

An actor must pass preceding queue/type/death/restraint gates and have its
countdown expire. Reset that countdown, increment the shared counter, and at ten
reset the counter and request one minute **before dispatching the actor**.
Players and monsters both count. This is neither ten player inputs nor ten
successful attacks nor ten rounds.

The counter is saved at `SAVED.GAM` offset `0x02DC`; the inspected initialization
paths do not establish an encounter-start reset. Redux separates queue selection
from action completion and tests tenth-actor timing, including sleeping actors.
An exhaustive comparison of every actor exclusion and save/encounter boundary is
outside this timing conversion.

**Implementation:** `internal/game_state/turn.go`, `internal/game_state/combat_turn_queue.go`
**Tests:** `internal/game_state/combat_status_test.go`, `internal/game_state/combat_turn_queue_test.go`

### TIME-0004

**Accepted slow steps and extra enemy phases**

**Kind:** rule
**Uses:** TIME-0001, TIME-0002
**Original:** Established
**Redux:** Matches

Without modifiers, intervening combat time, or interruption:

| Accepted destination terrain | Extra minutes | Ordinary total | Monster-phase opportunities |
| --- | ---: | ---: | ---: |
| Grass/otherwise unpenalized | 0 | 2 | 1 |
| Swamp, brush, desert, thick brush, either waste tile | 2 | 4 | 2 |
| Terrain values 9–15: forests and hill/mountain variants | 4 | 6 | 3 |

This classification follows movement acceptance; it does not make normally
impassable terrain walkable. Change position, run the extra one/two monster
phases with intervening update checks, add extra minutes, then perform ordinary
outdoor completion. The final phase occurs after that completion's player update.
Slow-progress feedback is suppressed when the extra phases reported attacks.

Stationary Pass does not use the slow-step routine. Water values 1–3 have no
penalty here. Phase opportunities are not guaranteed attacks: cadence and contact
gates still apply. Adjacent contact precedes pirate fire; otherwise an aligned
pirate within three tiles may attempt fire during an eligible phase. This can
produce repeated shoreline fire, but does not identify the cause of any particular
unreproduced incident.

Redux's `calculateMovementTimeCost`, `CompleteSurfaceMovementCycle`, and
`resumeSlowMovement` preserve this reviewed classification/extra-phase ordering.
The continuation survives blocking projectile presentation or combat without new
input. This scoped match does not certify every message or attack eligibility gate.

**Implementation:** `internal/game_state/turn.go`
**Tests:** `internal/game_state/slow_movement_completion_test.go`

### TIME-0005

**Pocket Watch has no independent time charge**

**Kind:** rule
**Uses:** TIME-0002, TIME-0003
**Original:** Established
**Redux:** Intentional deviation

The original item formats the current time without advancing it. Town/outdoor
display precedes ordinary one/two-minute completion. Dungeon input has already
received its pre-command charge. Combat Use occupies an actor dispatch, with
calendar advancement governed by the shared counter.

Redux likewise delegates surface completion until the display closes.
**REDUX DEVIATION: TIME-0005:** dungeon Use charges its deferred token when the
watch is selected, keeps ownership through dismissal, and adds no second minute
or surface completion. Cancelling the menu before selection remains free under
the shared confirmed-use policy.

In combat, Redux exposes the owned watch in Use and reads the time when that
selection commits, retaining the watch quantity. The same Use workflow owns the
dismissible display and completes exactly one action for its retained
encounter/actor/dispatch owner after dismissal. It adds no surface completion,
provision pass, or independent calendar charge; TIME-0003's combat scheduler
still owns minute advancement. Menu cancellation and stale precommit selection
are free. A replaced owner cannot inherit the deferred completion, and workflow
finalization/disposal removes the owned dialog.

**Implementation:** `internal/game_state/action_use.go`, `internal/game_state/dungeon_command_cycle.go`, `internal/game_state/combat_watch.go`, `internal/game_state/combat_use_owner.go`, `cmd/ultimav/gamescene_use.go`, `cmd/ultimav/gamescene_use_item.go`
**Tests:** `cmd/ultimav/gamescene_dungeon_use_test.go`, `cmd/ultimav/gamescene_surface_use_ownership_test.go`, `internal/game_state/combat_key_watch_test.go`, `cmd/ultimav/gamescene_combat_key_watch_test.go`

### TIME-0006

**Repair, sailing, and rest are separate loops**

**Kind:** rule
**Uses:** TIME-0001, TIME-0002, PROV-0002
**Original:** Established
**Redux:** Matches

Repair performs five iterations of update/monster work followed by a five-minute
request, and can abort after losing anchored-ship state. Outdoor completion is
caller-owned. Twenty-seven minutes describes only the uninterrupted, unmodified
five-cycle case plus ordinary completion, not every repair attempt.

With wind, automatic sailing can wait zero, one, or two times before movement
dispatch according to its cadence. Each wait requests two minutes, or one with
enhanced rigging, which also has a separate actor-phase toggle. There is no
ordinary player update inside the wait loop. Calm removes the wind-dependent
cadence break and permits continued waiting until interrupted.

Redux keeps separate `CompleteAutomaticSailingWait` and repair workflows.
`CompleteAutomaticSailingInterruption` returns to command input without adding
time or a player-effects update. A later time request can replace an earlier
hour crossing before the next player update, as described in
[provision timing](PROVISIONS_CONSUMPTION.md#prov-0002). This match covers the
wait/interruption ownership, not exact presentation timing or every rest policy.

Camping, bed, and inn loops retain their own [rest rules](CAMPING_MECHANICS.md).
Presentation delays and guard-performance iterations are not game minutes.

**Implementation:** `internal/game_state/sailing.go`, `internal/game_state/action_hole_up_repair.go`
**Tests:** `internal/game_state/sailing_state_test.go`, `internal/game_state/action_hole_up_repair_test.go`

### TIME-0007

**Opening-campaign reduction is saved state**

**Kind:** rule
**Uses:** TIME-0001
**Original:** Established
**Redux:** Unverified

In the variable monster-count branch, an enabled opening flag causes a second
count draw bounded by the first. Special count categories 1, 8, and 16 bypass
that branch. The first month rollover clears the flag; it is not continuously
derived from the date. It is independently saved at `SAVED.GAM` offset `0x03B3`.
Neither the requested bounds nor this reduction establish exact probabilities
without the original generator.

Redux retains a separate opening-campaign flag, imports its legacy byte, and
persists `continuation.openingCampaignEncounterReduction` in every supported
native version. Tests distinguish the live flag from calendar date. Full monster-count
distribution and special-category equivalence remain outside this review.

**Implementation:** `internal/game_state/turn.go`, `internal/legacy_save/conversion.go`
**Tests:** `internal/game_state/combat_encounter_test.go`
