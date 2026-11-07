# Karma

**Scope:** Original DOS-format party karma and the reviewed Redux development
implementation. Normal karma is 0-99. Correspondence across every distributed DOS
version, every authored dialogue record, and malformed saves is not established.
Sections explicitly narrow uncertain or unusual cases. Code and test paths below
identify the implementation in this application source tree.

Arithmetic in the pseudocode uses nonnegative valid inputs and a sufficiently
wide intermediate unless stated otherwise. `increase(k, n)` and `decrease(k, n)`
refer to [KARMA-0001](#karma-0001), not language-specific byte arithmetic.

See [conventions](CONVENTIONS.md) for statuses. **Matches** is scoped to the stated
case; **Unverified** can coexist with tested normal behavior when broader
equivalence is not established. Report offsets identify records, not permission
to reproduce the original report prose.

### KARMA-0001

**Party storage and adjustment bounds**

**Kind:** rule
**Uses:** none
**Original:** Established
**Redux:** Matches

Karma is party-wide. In `SAVED.GAM`, the value occupies one unsigned byte at
offset `0x02E2`. Normal adjustment operations maintain 0-99:

```text
increase(karma, amount) = min(99, karma + amount)
decrease(karma, amount) = max(0, karma - amount)
```

These equations describe the normal adjustment operations with karma initially
in 0-99 and nonnegative amounts. Byte storage alone is not a universal validator.
Some other operations add and then cap explicitly; do not infer corrupted-input
equivalence from the valid-state equations.

Redux clamps signed adjustments to 0-99 and reports the actual change. Its raw
byte constructor preserves its input; the exact-value gameplay setter instead
rejects values outside 0-99. At 98, adding 3 yields 99 and an actual gain of 1;
at 1, subtracting 5 yields zero and an actual loss of 1.

**Implementation:** `internal/party_state/karma.go`, `internal/game_state/party_karma.go`
**Tests:** `internal/party_state/karma_test.go`, `internal/game_state/party_karma_test.go`

### KARMA-0002

**Initial template and imported games**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Incomplete
**Redux:** Matches

The reviewed initial-template generation path assigns karma 75 and a beggar
reward counter of zero before writing `INIT.GAM`. New-character initialization
loads that template. This does not prove that every distributed template has
identical contents; other save-generation paths and imported saves can differ.

Redux's tested new-game fixture expects 75 and counter zero. Imported state is
read from its own saved fields rather than universally reset to the new-game
values. The starting state of every historical distribution remains unverified.

**Implementation:** `internal/new_game/initial_state.go`, `internal/legacy_save/conversion.go`
**Tests:** `internal/new_game/initial_state_original_test.go`, `internal/new_game/initial_state_test.go`

### KARMA-0003

**Town attacks and defenseless targets**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Established
**Redux:** Unverified

In the ordinary town attack route (map IDs 1-32), an intact mirror is handled
before object attacks and carries no attack-karma deduction. A particular
water/vehicle restriction and direction selection also precede the victim route.

For an accepted target in the character/guard tile range below `0x80`, subtract
5 before activating guards. The attackable-object filter requires tile >= `0x40`,
tile < `0xE8` or >= `0xF0`, and a four-frame tile group other than `0xB4`.

The defenseless branch tests the underlying terrain: broken mirror, bed, stocks,
or manacles. An exact Blackthorn tile `0x78` misses and avoids the second penalty;
otherwise subtract another 5 before the murder/removal effects. The initial
penalty is not refunded by that miss. NPC identity resolution is not the original
gate for every deduction.

An ordinary qualifying victim therefore normally costs 5 plus another 5, each
floored separately. This is not a general test of innocence, health, or sleep.
Redux tests normal victims, Blackthorn immunity, ordering, and empty terrain;
equivalence for every raw tile/object-identity combination is not established.

**Implementation:** `internal/game_state/action_attack.go`, `internal/game_state/combat_small_map_encounter.go`
**Tests:** `internal/game_state/action_attack_murder_integration_test.go`, `internal/game_state/action_attack_mirror_test.go`

### KARMA-0004

**Small-map cannon removal**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Incomplete
**Redux:** Intentional deviation

After the adjacent-cannon route selects a nonzero object-list target, removal
precedes a 5-point loss. Only afterward is that object mapped to an NPC for any
additional persistent removal handling. A successful NPC mapping is not required
for the penalty. A target at the party's zero slot follows a separate party-damage
branch without this loss. Guard activation occurs on discharge, earlier still.

This route is distinct from ship cannons on map 0 and rejects dungeon maps 33-40.
Adjacent cannons are considered north, east, south, west; targeting extends four
positions beyond the selected cannon. Exact target exclusions and intervening
impact-failure behavior are not certified for every executable variant.

Redux's small-map workflow subtracts 5 on its selected map-unit removal path.
Its target representation differs; universal selection equivalence is unverified.

**REDUX DEVIATION: KARMA-0004:** The inspected original town-cannon path can remove
a selected target despite blocked projectile travel and has an ineffective
Shadowlord exclusion. Redux stops at blocking terrain and prevents ordinary
cannon removal of Shadowlords, preserving physical obstruction and quest defeat
authority. The original Shadowlord outcome is local removal, not ritual destruction;
specific retail-executable reachability remains unverified. Successful removal is
floor-aware. The target search covers four tiles beyond the muzzle.

**Implementation:** `internal/game_state/action_fire.go`
**Tests:** `internal/game_state/action_fire_integration_test.go`

### KARMA-0005

**Town chest opening**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Established
**Redux:** Matches

For the surface chest route, cancellation of character selection occurs before
the chest is consumed or karma changes. For a valid selected chest:

```text
remember contents and trap state
remove chest
if 1 <= map_id <= 32:
    karma = decrease(karma, 2)
resolve trap, if present
determine contents
```

There is no ownership exemption in this penalty. An empty chest, a trap, or an
inventory unable to accept all loot does not postpone the loss until successful
looting. Dungeon chest opening uses a separate route. Redux's reviewed town
chest path preserves this ordering; its other admission/loot rules are outside
this karma comparison.

**Implementation:** `internal/game_state/action_open.go`
**Tests:** `internal/game_state/basement_chests_test.go`

### KARMA-0006

**Crops and reachable table food**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Established
**Redux:** Matches

When terrain pickup is reached after object-pickup precedence, crops become
plowed ground. An upper plate is reachable when targeting south, a lower plate
when targeting north; a two-plate table rejects east/west and leaves the other
plate after a valid north/south pickup.

```text
if crop or reachable plate is selected:
    consume/change the terrain item
    food = min(9999, food + 1)
    karma = decrease(karma, 1)
```

The loss follows the food-add operation, not a positive-inventory-delta check.
At food 9999, food remains capped but the item is consumed and karma still falls
if above zero. An unreachable plate does not enter this mutation branch. There
is no additional ownership or map-1-32 gate inside these terrain branches.

Redux follows the reviewed mutation order and gives explicit theft feedback.
The same branch serves tactical maps through COMBAT-0010; a dungeon-room plate
is still not the party's. This scope does not establish every object-precedence
or corrupted-map case.

**Implementation:** `internal/game_state/action_get.go`
**Tests:** `internal/game_state/combat_terrain_commands_test.go`

### KARMA-0007

**Dialogue karma adjustments**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Established
**Redux:** Matches

In the normal TLK instruction stream, opcode `0x89` increases karma by one with
the upper cap; `0x8A` decreases it by one with the lower floor. A byte being
collected as an operand of another instruction is not reinterpreted as a fresh
karma opcode. Multiple instructions apply in their executed order.

Redux imports these operations as semantic karma adjustments and applies the
signed change through the party mutation boundary.

**Implementation:** `internal/conversation/semantic_engine.go`, `cmd/ultimav/game_action_callbacks.go`
**Tests:** `internal/conversation/semantic_engine_test.go`, `internal/conversation/extractor/decode_test.go`

### KARMA-0008

**Freeing a prisoner**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Incomplete
**Redux:** Intentional deviation

The stocks/manacles Jimmy path requires a key and appropriate target before the
attempt. Failure when dexterity <= a draw requested with bounds 0..29
consumes one key and awards no karma. The bounds alone do not establish the
original generator's uniformity. Success does not consume a key.

Where the target resolves to an eligible NPC, clear its dialogue association
before testing its persistent absence/death marker. If that marker is not set,
make it follow and add 2 capped at 99; then mark persistent absence. For ordinary
prisoners this prevents repeated rewards.

The original marker's read/write eligibility depends on actor shape; there is no
universal freed-prisoner flag or general prisoner-type test. The target-presence
and reward branches also use different map-number boundaries (<128 and <127).
Arbitrary unusual occupants or unsupported map encodings are not certified here.

Redux explicitly requires a persistent friendly town NPC, checks its floor and
identity, and tests first reward, repeat attempts, failed keys, and lookup
rejection. This is a scoped normal-prisoner match, not proof for every actor type.
**REDUX DEVIATION: KARMA-0008:** that explicit identity restriction is retained
to prevent arbitrary occupants and repeated rescues from generating rewards.

**Implementation:** `internal/game_state/action_jimmy.go`
**Tests:** `internal/game_state/action_jimmy_prisoner_test.go`

### KARMA-0009

**Shrine quest completion**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Established
**Redux:** Intentional deviation

After accepted virtue/mantra interaction, the quest flags select the result:

| Done flag | On-quest flag | Karma result |
| --- | --- | --- |
| Clear | Either | Assign/reassign quest; no completion award here |
| Set | Clear | Offering route, not another quest reward |
| Set | Set | Completion reward |

On completion, clear the on-quest flag, add 3 with a cap of 99, then apply the
virtue's stat bonuses capped at 30. Humility has no stat bonus and applies another
separately capped +3. The Codex establishes completion eligibility; the reward
occurs on the qualifying return to the shrine.

With valid starting karma, ordinary completion gains at most 3 and Humility at
most 6. At karma 98, either ends at 99. Redux yields these final values but applies
stat changes before one combined karma adjustment. **REDUX DEVIATION: KARMA-0009:**
that ordering is deliberately retained for a single coherent reward transaction;
equal final values do not imply identical intermediate feedback.

**Implementation:** `internal/game_state/shrine_service.go`, `internal/game_state/shrine_data.go`
**Tests:** `internal/game_state/world_service_authority_test.go`

### KARMA-0010

**Shrine offerings**

**Kind:** rule
**Uses:** KARMA-0001, KARMA-0009
**Original:** Established
**Redux:** Matches

The offering route requires done set and on-quest clear after the accepted shrine
interaction. A chosen digit 1-9 costs that digit times 100 gold. Zero exits without
payment/reward; insufficient gold causes another prompt. On an affordable choice,
deduct gold before increasing karma by the digit and capping at 99.

At karma 99, a completed offering still costs gold even though the gain is zero.
Redux reviews and revalidates the offering, rejects invalid digits/unaffordable
payment, and applies payment followed by the capped adjustment. This comparison
covers committed offerings rather than every presentation interaction.

**Implementation:** `internal/game_state/shrine_service.go`
**Tests:** none

### KARMA-0011

**Beggar payment reward**

**Kind:** rule
**Uses:** KARMA-0001, KARMA-0012
**Original:** Established
**Redux:** Matches

The eligibility threshold of **100 refers to the reward counter**, not the amount
of gold donated. For a script-requested amount the party can afford:

```text
gold = gold - requested_amount
if recipient belongs to the beggar tile group and reward_counter >= 100:
    reward_counter = 0
    karma = increase(karma, 1)
    if gold == 0:
        karma = increase(karma, 2)
```

The all-gold bonus is nested inside the beggar/cooldown condition and examines
post-payment gold. Insufficient funds do not consume the cooldown. The cooldown
is consumed even at karma 99. There is no helper-level positive-amount or
100-gold-minimum check; individual scripts choose their requested amounts.

Example: counter 100, gold 50, karma 50, payment 1 -> gold 49 and karma 51.
Paying all 50 in that state instead yields karma 53. Counter 99 yields no reward
even when all gold is paid. A zero-valued script payment has a defined consequence
in this logic, but reachability in original authored dialogue remains unverified.

Redux tests these cases, including zero payment and capped karma. It combines the
eligible +1/+2 into a single adjustment, preserving final values over valid
0-99 karma; it emits feedback for the actual committed change.

**Implementation:** `internal/game_state/party_karma.go`, `cmd/ultimav/game_action_callbacks.go`
**Tests:** `internal/game_state/party_karma_test.go`

### KARMA-0012

**Beggar reward counter timing**

**Kind:** rule
**Uses:** none
**Original:** Established
**Redux:** Matches

The counter is an unsigned saved byte at `SAVED.GAM` offset `0x02E5`. A qualifying
player-update boundary increments it once with saturation at 255. It is not
incremented once per character or once per elapsed minute.

During bed rest, each full attempt requests a ten-minute time increment and then
performs a player update before the bed-occupancy interruption test. Quickness and
Time Stop can alter actual clock movement while the update still runs. The
ordinary command-end path can add an update after returning from rest. Do not
translate counter 100 into a universal fixed number of minutes or rest hours.

Redux explicitly advances the counter in surface, dungeon-cycle, and bed-rest
update paths and preserves it through saves. Tests check those update boundaries,
the saturation limit, and magic-altered bed timing. This is a per-update match;
all historical bed-loop termination and time-boundary cases are not certified.

**Implementation:** `internal/game_state/turn.go`, `internal/game_state/action_hole_up_bed.go`, `internal/game_state/dungeon_command_cycle.go`
**Tests:** `internal/game_state/turn_test.go`, `internal/game_state/save_snapshot_test.go`

### KARMA-0013

**Giving Blackthorn the requested mantra**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Established
**Redux:** Matches

The demanded shrine is selected from those with a zero closure byte. The answer
is accepted when it contains the requested mantra, case-insensitively; whole-line
equality is not required. On an accepted response, mark the shrine closed with
`0xFF`, subtract 5 with a zero floor, then perform the remaining interrogation
consequences. The incorrect-answer branches do not apply this specific loss.

Closed-state interpretation (>127) is broader than the zero-byte eligibility test
for selecting a demanded shrine. Redux's reviewed answer path preserves substring
matching and closure-before-penalty ordering. Other torture/martyrdom rules are
outside this karma rule.

**Implementation:** `internal/game_state/blackthorn_interrogation.go`
**Tests:** `internal/game_state/blackthorn_interrogation_test.go`

### KARMA-0014

**Continuing to drink after the warning**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Established
**Redux:** Intentional deviation

The original warning occurs at a drunkenness count of exactly 3. Agreeing to
stop returns before buying another drink. Refusing sets the intoxication
consequence counter to 25 and subtracts 1 before the later purchase attempt.
A subsequent affordability failure or wine-selection cancellation does not undo
those warning consequences.

**REDUX DEVIATION: KARMA-0014**

- **What:** A cancelled or unaffordable warned purchase has no new karma loss or
  warning-state mutation in Redux.
- **How:** Redux validates the reviewed request and affordability, commits payment,
  then applies warning effects, theft consequences, and the drink counter change.
- **Why:** Cancellation and failed payment remain atomic rather than partially
  changing the party during an uncommitted transaction.

Successful warned purchases still set the consequence counter and apply the
capped loss. This is an existing implementation decision, not a change made by
the documentation pilot.

**Implementation:** `internal/game_state/tavern_shop.go`
**Tests:** `internal/game_state/tavern_shop_test.go`

### KARMA-0015

**Individual resurrection experience adjustment**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Established
**Redux:** Matches

The resurrection mutation requires an existing dead character. It changes the
status to good, sets current HP to 1, and restores class-dependent magic points.
For valid karma and nonnegative experience:

```text
if karma < 98:
    experience = floor((experience * karma) / 100)
recompute level and maximum HP from experience
```

Multiplication must use a sufficiently wide intermediate before division. Karma
98 and 99 preserve experience; level/maximum-HP recalculation still occurs.
A living character does not enter this resurrection mutation. Redux tests the
individual resurrection gate, class/level boundaries, and the experience rule.

**Implementation:** `internal/game_state/effect_spells.go`
**Tests:** `internal/game_state/effect_spells_test.go`

### KARMA-0016

**Party recovery uses pre-recovery karma**

**Kind:** rule
**Uses:** KARMA-0001, KARMA-0015, KARMA-0017
**Original:** Established
**Redux:** Unverified

Select and display the karma report using the pre-recovery value, then wait for
acknowledgement. Visit active roster entries in ascending order: invoke individual
resurrection and restore current HP to the resulting maximum. Only afterward
raise karma to at least 75.

Thus qualifying dead members lose experience using the old karma, not the later
minimum. Individual resurrection still requires dead status; merely calling it
for every active entry does not establish that every entry is mutated. Original
recovery-entry eligibility is broader than a universal assertion that all statuses
are dead.

Redux's ordinary wipe gate requires the party dead, but its shared recovery
penalty routine processes every active member without a dead-status gate. Tests
verify pre-recovery karma and active/inactive membership. Equivalence across all
alternative recovery entry states is not established and is not newly designated
an intentional deviation.

**Implementation:** `internal/game_state/death_resurrection.go`
**Tests:** `internal/game_state/death_resurrection_test.go`

### KARMA-0017

**Karma report selection**

**Kind:** rule
**Uses:** KARMA-0001
**Original:** Established
**Redux:** Matches

For normal karma, compute `band = floor(karma / 20)`. The report offsets within
`KARMA.DAT` are:

| Karma | Death/recovery report offset | Wanderer report offset |
| --- | ---: | ---: |
| 0-19 | 0 | 0 |
| 20-39 | 132 | 132 |
| 40-59 | 269 | 269 |
| 60-79 | 410 | 410 |
| 80-99 | 546 | 671 |

Both contexts require acknowledgement after displaying the selected report.
Different offsets are established; the exact prose is external data and is not
reproduced here. The valid-state death lookup assumes karma 0-99; it does not
first sanitize arbitrary byte values.

Redux loads six records from the user's file, selects the separate top-band
record by context, and rejects out-of-range karma. Its file parser also validates
record boundaries. That parser's extra validation is not a claim that the original
accepted/rejected all malformed files identically.

**Implementation:** `internal/references/karma_reports.go`
**Tests:** `internal/references/karma_reports_test.go`

### KARMA-0018

**Dialogue karma threshold branch**

**Kind:** rule
**Uses:** KARMA-0001, KARMA-0007
**Original:** Established
**Redux:** Unverified

TLK opcode `0xFE` consumes a raw unsigned threshold byte and a question-label byte.
Those bytes are operands, even if their numeric values resemble another opcode.
Once both have been collected, branch to the specified question if
`karma >= threshold`; otherwise continue without that branch. This is a question
label, not an arbitrary byte-offset jump. The threshold is not ASCII-decoded or
reduced to seven bits.

Redux's semantic condition uses the same inclusive comparison and has boundary
tests, but its validator permits thresholds only in 0-99. The original raw-byte
operation can represent 100-255. Whether a shipped dialogue exercises that wider
range is unverified; no compatibility claim or intended-deviation rationale is
invented for it here.

**Implementation:** `internal/conversation/semantic_engine.go`, `internal/conversation/semantic/validate.go`
**Tests:** `internal/conversation/semantic_engine_original_test.go`, `internal/conversation/extractor/decode_test.go`

### KARMA-0099

**Recovery overview**

**Kind:** group
**Uses:** KARMA-0015, KARMA-0016, KARMA-0017

Read the individual resurrection rule, party-recovery ordering, and report
selection together when investigating recovery. This group supplies navigation;
its members own the algorithms and their independent verification limits.
