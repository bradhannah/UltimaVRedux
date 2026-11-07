# Camping, beds, and inn rest

Rest modes have different clocks, player-update boundaries, and rewards. The
rules below describe reviewed DOS paths and separately identify Redux behavior.
They are not retail-build certification. Random requests specify bounds and
selection multiplicities; exact generator distributions and wall-time sequences
remain unverified. Formation bytes, Wanderer internals, and all combat wake paths
are outside this rest review.

### CAMP-0001

**Entry, terrain, and the watch**

**Kind:** rule
**Uses:** TIME-0002
**Original:** Established
**Redux:** Intentional deviation

Choose 1–9 hours; zero or Space cancels setup. Surface camping rejects water and
requires the ordinary on-foot party shape; ships route to repair. Small-map Hole
Up instead requires bed terrain. A watch is offered only with more than one
Good-or-Poisoned member, but the selected guard must be Good. Cancelled or non-Good
selection becomes no guard. Setup cancellation does not by itself prove the
enclosing command has no time cost.

**REDUX DEVIATION: CAMP-0001:** Redux permits horses and carpets on eligible land.
Horses remain mounted; at actual camping commitment a ridden carpet is transferred
to inventory exactly once and the party dismounts. This protects possession through
ambush/death and leaves it carried afterward. No ground carpet is created.
Cancelling before commitment leaves it mounted; full carpet stock refuses the
transfer without loss. Water/skiff rejection and frigate repair stay separate.

Confirming a non-Good member, such as a Poisoned warning row, camps without a
watch, as in the original. The warning row says so before confirmation. The
explicit No One row also starts an unguarded camp.

**REDUX DEVIATION: CAMP-0001 — cancelling the watch prompt:** the original treats
a cancelled guard prompt like No One and still camps. Redux Escape backs out of
camp setup instead, because Escape never commits a time-consuming action
elsewhere in Redux; No One gives the original's unguarded outcome. Surface
cancellation has no ordinary completion cycle; dungeon cancellation retains its
accepted precharge and exact full end-player cleanup without camp setup RNG.

The guard comparison presents detached current/max HP and MP for every party row,
with blank resource cells for No One. Inspection does not choose a guard or predict
healing. Layout and confirmation ownership are described in the
[camping system](../systems/CAMPING_SYSTEM.md#guard-comparison-and-confirmation).

**Implementation:** `internal/game_state/action_hole_up_camping.go`, `internal/game_state/action_hole_up_dungeon.go`, `cmd/ultimav/gamescene_dialog_helpers.go`, `cmd/ultimav/gamescene_character_selection.go`, `internal/ui/widgets/character_guard_resources.go`
**Tests:** `internal/game_state/camping_carpet_stow_test.go`, `cmd/ultimav/gamescene_camping_cancellation_test.go`, `cmd/ultimav/gamescene_guard_resources_test.go`, `internal/ui/widgets/character_guard_resources_test.go`

### CAMP-0002

**Arena and preselected dungeon ambushers**

**Kind:** rule
**Uses:** CBT-0001
**Original:** Established
**Redux:** Unverified

Outdoor camping loads `BRIT.CBT` record 0. Dungeon setup builds floor/walls,
places roasting meat, clears traps, installs camp formations, shuffles monster
slots, and selects the dungeon monster and quantity **before resting**.

| Monster | Outdoor selection entries out of 8 | Dungeon selection entries out of 8 |
| --- | ---: | ---: |
| Rats | 2 | 1 |
| Troll | 1 | 0 |
| Bats | 1 | 1 |
| Slime | 1 | 1 |
| Spiders | 1 | 1 |
| Gremlins | 1 | 1 |
| Headless | 1 | 0 |
| Ghost | 0 | 1 |
| Gazers | 0 | 1 |
| Reaper | 0 | 1 |

On ambush the shared camp routine still reseeds and consumes an outdoor selection.
Dungeon combat then uses its prepared roster; this is not a post-ambush replacement
selection. Other dungeon initialization draws exist, so the unused outdoor draw
is not a complete RNG trace.

Redux's `PrepareDungeonCampingPlan` likewise retains its pre-rest selection for
`QueueDungeonCampingAmbush`; the latter consumes an unused outdoor draw without
wall-time reseeding. Exact authored arena bytes, complete population RNG order,
and all formation coordinates were not freshly certified in this conversion.

Redux places outdoor ambushers only on shuffled monster slots that no party
member occupies, so a guard who wandered onto one keeps it, and caps the number
of ambushers at the free slots. How the original resolves that overlap is not
established.

**Implementation:** `internal/game_state/combat_dungeon_camping.go`, `internal/game_state/combat_camping_encounter.go`

### CAMP-0003

**Five-minute loop, ambush, and guard movement**

**Kind:** rule
**Uses:** TIME-0001, PROV-0002
**Original:** Established
**Redux:** Intentional deviation

For each entered iteration, in order:

1. Update display, check Regeneration Rings, and present status.
2. If the hour changed since the preceding cycle, test `roll(0,63) == 0` for ambush.
3. On ambush, reseed from wall time, consume an outdoor monster selection, wake
   applicable party members only if guarded, initialize the appropriate combat,
   and leave the loop.
4. Otherwise store the current hour and request five minutes.
5. Perform delay and eligible sky refresh.
6. If guarded, test `roll(0,3) == 2`; on success select a cardinal direction and
   move the guard only to an on-screen legal destination.

The target-hour condition is checked before entering another iteration. The
terminal hour crossing therefore has no subsequent ambush test: ordinary
uninterrupted original `H`-hour rest receives `H - 1` tests.

No ordinary player update runs inside the loop: no ordinary poison, meal, or
starvation tick. Ring healing still occurs. The owning command later completes
according to its context; that does not replay intermediate hours or universally
preserve the final hunger comparison.

**REDUX DEVIATION: CAMP-0003:** all camping randomness remains on the controlled
game RNG rather than reseeding from wall time. Each logical cycle retains its
guard/ring/ambush ordering. Full requested-duration behavior is separately
described below and can change how many hour crossings are tested.

Outdoor and dungeon camping schedule each sleep-cycle presentation interval at
200 ms. A fresh unmodified **Space** press during the sleeping loop latches 50 ms
intervals for the rest of that camp. It takes effect when the next interval is
scheduled; an already-scheduled deadline is not shortened. A hitch runs the due
cycle and schedules from its execution time, rather than catching up multiple
cycles. The press does not advance a cycle or consume RNG, and a held key does not
repeat or become an acknowledgement of the later result. Setup, Bard performance,
ambush, Wanderer presentation, and results are outside this acceleration window.
Completion/interruption and final cleanup reset the latch.
Acceleration is silent: Space changes the sleeping cadence without adding a hint
box, status box, message, sound, or acknowledgement.

Acceleration changes presentation pacing, not five-minute cycle size, requested
rest length, healing/cooldown rules, or the order of camp decisions. However,
elapsed wind processing shares the gameplay RNG: normal and accelerated runs
can interleave different wind draws. Reproducibility requires the same elapsed,
input, and action timeline as well as seed/state; equal seeds alone do not promise
equal guard, ambush, healing, or later gameplay outcomes.

**Implementation:** `internal/game_state/action_hole_up_camping.go`, `internal/game_state/action_hole_up_dungeon.go`, `internal/game_state/combat_dungeon_camping.go`, `internal/game_state/camping_pacing.go`, `internal/camping/camping_scene.go`, `internal/event_queue/event_queue_builder.go`, `cmd/ultimav/gamescene_transient_command_input.go`
**Tests:** `internal/game_state/action_hole_up_camping_determinism_test.go`

### CAMP-0004

**Sleep statuses and Bard preparation**

**Kind:** rule
**Uses:** CAMP-0001
**Original:** Established
**Redux:** Unverified

Camp entry clears active duration magic and duration. Poisoned members are
remembered, but the sleep helper leaves them awake; do not describe all nonguards
as sleeping. A Good Bard guard with sound enabled performs before the normal loop
for 52 update/delay iterations. That is not an independently verified number of
seconds or game minutes. Original updates may consume incidental presentation RNG.

Guarded ambush preparation wakes applicable entries and restores remembered
Poisoned status. Unguarded ambush does not perform that wake sweep. The ordinary
successful return normalizes remaining sleepers; an ambush returns before that
cleanup.

Redux puts active Good members to sleep, preserves a Good guard, and leaves
Poisoned members awake. Its outdoor/dungeon workflows await the Bard presentation
before logical cycles: no camp minutes, ring sweep, guard roll, or ambush roll runs
during that wait. `BardMusicDurationMs` is a Redux presentation choice, not a
conversion of 52 original iterations. Performance overrides are cleaned on exit.

Redux also deliberately wakes a living PC who was already sleeping when a directed
attack resolves, including a miss/zero damage, with no bonus turn or wake RNG.
The exact tactical copy is affected; fresh sleep from that attack and ongoing
hazards are separate. This existing threat-response policy is preserved here as
current behavior; its full original combat comparison belongs to the combat
review, not a blanket camping Match claim.

**Implementation:** `internal/game_state/action_hole_up_camping.go`, `internal/game_state/combat_status.go`
**Tests:** `internal/game_state/action_hole_up_camping_determinism_test.go`, `internal/game_state/combat_attack_wake_test.go`

### CAMP-0005

**Duration, healing, Wanderer ordering, and cooldown**

**Kind:** rule
**Uses:** CAMP-0003, CAMP-0004
**Original:** Established
**Redux:** Intentional deviation

The original target is an hour bucket, not `H * 60` elapsed minutes. For ordinary
uninterrupted camping starting at minute `m`, requesting `H` hours:

```text
cycles = 12 * H - floor(m / 5)
elapsed_minutes = 60 * H - 5 * floor(m / 5)
ending_minute = m modulo 5
```

Caller completion and combat are excluded. Twelve cycles/hour is exact only for
starting minutes 0–4. For six hours from 22:37, original rest advances 325 minutes
to 04:02, before any command completion.

At completion, reward requires remaining cooldown zero and **requested** hours
greater than five. Eligibility is evaluated after the rest's hour-rollover
decrements. In party order, exclude remembered Poisoned members, Dead members,
and the guard. Other eligible members receive a `roll(1,63)` HP increase capped
at maximum. Mage/Avatar MP becomes Intelligence; Bard MP becomes integer half
Intelligence; other classes receive no MP assignment. Actual sleeping is not an
additional reward predicate. Independent ring healing can benefit excluded guards
or poisoned members.

After party reward rolls, qualifying outdoor rest requests a one-of-four Wanderer
appearance check; dungeon scenarios exclude it. The last-Wanderer month is written
but not read as a camping eligibility gate. Qualifying rest sets cooldown 14,
decremented at hour rollovers rather than measured as a timestamp. Ambush bypasses
reward, Wanderer, normal wake cleanup, and this cooldown assignment.

**REDUX DEVIATION: CAMP-0005:** Redux advances the full requested `H * 60` minutes
using twelve five-minute cycles per requested hour, retaining the starting-minute
offset so duration selection means the requested elapsed rest. Six hours from
22:37 ends at 04:37 before caller completion. This can also expose an additional
hour-crossing ambush opportunity compared with the original shortened loop.

Redux's reviewed reward path keeps party-order healing before the outdoor Wanderer
roll and does not impose a once-per-month gate. Its cooldown assignment occurs
inside healing before the later Wanderer decision; final-value agreement does
not establish identical intermediate state. Wanderer restoration/advancement is
outside this rule.

**REDUX DEVIATION: CAMP-0005:** optional `AnkhWandererBonus`, off by default,
adds five percentage points to the one-of-four Wanderer check for each active,
non-Dead member wearing an Ankh: 25% to 55% with six wearers, on the same single
roll. Players have long told a folk tale that the Ankh draws the Wanderer; no
evidence shows the original doing so. Redux offers it for players who enjoy the
idea, and the default keeps the original chance.

**Implementation:** `internal/game_state/action_hole_up_camping.go`, `internal/game_state/action_hole_up_dungeon.go`, `internal/game_state/action_camping_wanderer.go`
**Tests:** `internal/game_state/action_hole_up_dungeon_test.go`, `internal/game_state/action_camping_wanderer_test.go`

### CAMP-0006

**Bed rest retains magic and player updates**

**Kind:** rule
**Uses:** TIME-0001, TIME-0002, PROV-0001
**Original:** Established
**Redux:** Intentional deviation

Beds do not clear temporal magic at entry. Only Good members become Sleeping.
Preliminary NPC processing can abort. Each entered cycle requests ten minutes,
then performs player update, status/NPC work, and bed-occupancy/eviction checking.
Normal return wakes Sleeping members and moves the party one cell right. There
is no camp-style HP reward or MP restoration; ring healing remains possible.

Quickness turns a ten-minute request into five while active. Time Stop prevents
clock progress, but the following player update may decrement its finite duration.
Duration 0 or 255 is not decremented by that branch. If it never expires, clock
progress cannot reach the target, although NPC occupancy can still interrupt;
unconditional nontermination would overstate the evidence.

The target is current hour plus requested hours, subtracting **23 once** if above
23. Without modifiers or eviction, let `K = H` without that wrap, otherwise
`K = H + 1`. Elapsed rest is `60 * K - 10 * floor(start_minute / 10)`.
For two hours at 23:00, the literal original target is 02:00, three hours later.
The ordinary town return minute occurs only if its context/party gates permit it.

**REDUX DEVIATION: CAMP-0006:** Redux uses modulo 24 for the target, preserving
hour-bucket timing without the extra wrapped hour. It also wakes immediately in
the abnormal Time Stop duration-0/255 state so the rest workflow cannot deadlock
waiting for time. Ordinary finite Time Stop and Quickness retain modifier-sensitive
attempts. Redux restores only statuses changed by this rest and moves east only
when the destination is passable and unoccupied; unusual-state equivalence is not
implied by its ordinary timing tests.

**Implementation:** `internal/game_state/action_hole_up_bed.go`, `internal/game_state/turn.go`
**Tests:** `internal/game_state/turn_test.go`, `internal/game_state/action_hole_up_test.go`

### CAMP-0007

**Paid inn rest and morning outcomes**

**Kind:** rule
**Uses:** TIME-0001, PROV-0001, PROV-0002
**Original:** Established
**Redux:** Intentional deviation

Original entry into inn service clears active duration magic and duration. After
vacancy/agreement/payment, Good members become Sleeping. Advance twelve five-minute
steps, then repeat presentation/ring checks and nine-minute steps until the hour
is 6. No ordinary player update occurs inside either loop, and the final minute
need not be exactly 00. Bed's retained temporal modifiers do not apply here.

For each non-dead member in morning processing: restore maximum HP, restore
Mage/Avatar MP to Intelligence or Bard MP to integer half Intelligence, then kill
a still-Poisoned member and zero HP; otherwise turn Sleeping into Good. Other
classes have no new MP assignment. Intermediate poison, meals, and starvation are
not replayed. Normal caller completion remains separately gated.

Redux's `RestAtInn` retains the reviewed 12-by-5 then 9-minute loop and morning
ordering. **REDUX DEVIATION: CAMP-0007:** it clears duration magic only after paid
Rest commits, so Leave, Pick Up, refusal, and cancellation preserve an existing
effect. This makes uncommitted merchant operations non-destructive. Pricing,
vacancy, theft, roster transfers, and full merchant-session behavior belong to
the service-family review rather than this rest rule.

**Implementation:** `internal/game_state/inn_shop.go`, `internal/game_state/turn.go`
**Tests:** `internal/game_state/inn_shop_test.go`
