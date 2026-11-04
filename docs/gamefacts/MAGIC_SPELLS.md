# Magic spells

The 48 prepared spells use indexes 0–47; non-player Nox is excluded. Recipes are
canonical in [Reagent mixing](REAGENT_MIXING.md#mix-0001); potions and scrolls have
[separate Use rules](POTIONS_SCROLLS.md). Original assessments concern inspected
material labelled 1.16, not an immutable source release or verified retail binary.
R(a,b) records the requested random bounds; neither original-generator uniformity
nor ordering of multiple calls inside an expression is established. D30 denotes
[the transformed helper](ENVIRONMENTAL_HAZARDS.md#rng-0001).

### SPELL-0001

**Prepared cast gates, build branches, and Redux commitment**

**Kind:** rule
**Uses:** INV-0006
**Original:** Incomplete
**Redux:** Intentional deviation

Original entry selects a caster and a spell. Empty/invalid spell input returns
without payment. Up to four syllable initials are sorted for lookup, making
syllable order irrelevant. Lookup skips adjacent duplicate initials after sorting;
repeating an initial does not itself reject an otherwise matching abbreviation.
The four-input limit still applies before duplicates are skipped. Mix uses this
same spell-name parser. Circle is floor(index/6)+1. Outdoor, town, dungeon,
and combat contexts have separate legality masks. Direct Blackthorn's Castle
casting is absorbed without ownership of the Crown, and Stonegate absorbs direct
noncombat casting; combat has its own outer absorption/interference gates.

The **ordinary resource-enforcing branch** then:

1. Rejects zero prepared stock for free; otherwise consumes one mixture.
2. Rejects insufficient MP after mixture loss, leaving MP unchanged.
3. Otherwise consumes circle MP, then rejects insufficient caster level.
4. Resolves spell-specific targeting and effect after payment. Cancellation,
   resistance, obstruction, ineffective targets, and failed placement do not refund.

The exposed include chain enables a **CHEAT build branch** instead. It reports
zero mixture stock but continues; nonzero stock loses one mixture. It separately
reports insufficient MP without deduction, or deducts circle MP when sufficient.
Thus an available mixture is consumed even with insufficient MP, and sufficient
MP is consumed even with no mixture. It then dispatches regardless of those two
shortages and bypasses the level gate. Consequently the ordinary branch is a
visible conditional algorithm, not an established description of the exposed
build or a particular retail release. Context and earlier absorption gates remain.

Redux intentionally validates known prerequisites and obtains the final required
selection before atomic payment, so cancellation/rejection is not a penalty.
Final confirmation consumes one mixture and circle MP once; later resistance,
obstruction, no effect, or random placement failure remains paid. Precommit
rejection consumes no gameplay time, action, or RNG. Resource/level debug overrides
are explicit Redux settings, distinct from proving an original build branch.
Precommit rejections include every prepared combat Cast while Negate Magic is
active (**Absorbed!**, In An included), combat Vas Mani before victory, and An Tym
while a Shadowlord would absorb it.

Metadata owns legal contexts and separate selection/effect scope. The support
table owns implemented contexts. Crosshair, adjacent-tile, and cone workflows
validate first, revalidate on confirmation, retain authoritative pending data,
and resolve exactly once; presentation never commits again. Dungeon casting keeps
its dungeon command-cycle token until cleanup and charges it only on commitment.
It must not complete a surface cycle.

Combat An Sanct, An Ex Por, and In Ex Por retain the exact encounter, scheduled
PC/clone, origin, dispatch and map instances from selection through delayed
resolution and completion. Confirmation consumes one prepared spell and its MP
once. The paid cast keeps ownership until cleanup; duplicate confirmation or
effect callbacks cannot pay, mutate, or finish again. Presentation shutdown settles
an already-paid cast only while that original owner is still current. A changed
owner/map never mutates or finishes a replacement action. A changed target after
payment produces no success event; it still completes the original paid action.
For a chest, An Sanct retains both the selected lifetime ID and its value snapshot.
Removing and readmitting identical contents at the same position and original slot
invalidates the paid effect. Moving the original value within object storage does
not. Encounter-local lifetime allocation is described in COMBAT-0010.

**Implementation:** `internal/references/spell_metadata.go`, `internal/game_state/action_cast.go`, `internal/game_state/combat_spell_targeting.go`, `internal/game_state/combat_adjacent_spell_targeting.go`, `internal/game_state/combat_utility_spell_commit.go`, `internal/game_state/combat_cone_targeting.go`, `cmd/ultimav/gamescene_combat_utility_spell_effect.go`, `cmd/ultimav/gamescene_input_dungeon.go`
**Tests:** `internal/game_state/cast_use_commit_test.go`, `internal/game_state/combat_spell_targeting_test.go`, `internal/game_state/combat_door_spell_test.go`, `cmd/ultimav/gamescene_combat_door_spell_test.go`

### SPELL-0002

**Complete prepared-spell context and principal-effect catalog**

**Kind:** rule
**Uses:** MIX-0001, RNG-0001
**Original:** Established
**Redux:** Unverified

O = outdoors; T = town/surface small map; D = first-person dungeon; C = tactical
combat. Context admission alone does not guarantee effective targeting or success.
The table describes original principal effects; the remaining rules specify
shared mechanics and Redux differences. Redux's support table currently admits
all listed contexts, but that fact alone does not establish full effect parity.

| Index | Circle | Spell | Context | Principal original effect |
|---:|---:|---|---|---|
| 0 | 1 | In Lor | O/T/D | Set separate magical-light counter to 100. |
| 1 | 1 | Grav Por | C | Range-15 automatically hitting delivered missile; attack rating 16, then armor. |
| 2 | 1 | An Zu | O/T/D/C | Select member; Sleeping becomes Good, with matching combat sleep-entry handling. |
| 3 | 1 | An Nox | O/T/D/C | Select member; Poisoned becomes Good. |
| 4 | 1 | Mani | O/T/D/C | Non-Dead target gains D30 HP capped at maximum; full-health living target still succeeds. |
| 5 | 1 | An Ylem | T/C | Adjacent direction; eligible furniture/pushables, including mirrors, become floor. |
| 6 | 2 | An Sanct | O/T/D/C | Adjacent surface/combat locked door unlocks or chest trap clears. Dungeon opens current then facing eligible chest cell, preserving bit 0x08. |
| 7 | 2 | An Xen Corp | C | Scan eligible undead monsters regardless of loyalty; non-immune failed-INT-save targets become 1 HP and flee. |
| 8 | 2 | Rel Hur | O | Choose wind direction; passed direction changes nothing after payment. |
| 9 | 2 | In Wis | O | Report world Y then X using two Runic A–P nibbles each. |
| 10 | 2 | Kal Xen | C | Up to eight placement attempts; loyal Rat/Spider/Bat/Python, selector weights 6/5/3/2 of sixteen. |
| 11 | 2 | In Xen Mani | O/T/D/C | Add R(1,3) food, capped at 9,999. |
| 12 | 3 | Vas Lor | O/T/D | Set magical light to 255. |
| 13 | 3 | Vas Flam | C | Range-15 automatically hitting delivered fire missile; attack rating 30, then armor. |
| 14 | 3 | In Flam Grav | D/C | Facing dungeon fire field 0x82/0x8A. Combat range-15 field, direct rating 21, standing damage R(0,10). |
| 15 | 3 | In Nox Grav | D/C | Facing dungeon poison field 0x81/0x89. Combat field poisons a directly hit Good PC; other direct targets take R(0,20). |
| 16 | 3 | In Zu Grav | D/C | Facing dungeon sleep field 0x80/0x88. Combat direct/standing contact sleeps without a save. |
| 17 | 3 | In Por | O/C | Outdoors reaches last exact Grass on selected ray in retained 32×32 window, with numeric clipping and initial-only coordinate wrapping. Combat tries up to seven random legal destinations; dungeon-combat fails. |
| 18 | 4 | An Grav | D/C | Dungeon removes current field before testing facing field. Combat removes first adjacent field object in selected direction. |
| 19 | 4 | In Sanct | O/T/D/C | Shared Protection duration 20; apparent +3 armor intent is subject to effective-armor uncertainty. |
| 20 | 4 | In Sanct Grav | D/C | Facing dungeon electric field 0x83/0x8B; exact 0x83 contact damages party and pushes back. Combat range-15 movement-blocking protection field. |
| 21 | 4 | Uus Por | D | One floor upward; boundary exits dungeon. Fails in Doom. |
| 22 | 4 | Des Por | D | One floor downward; boundary exits dungeon. Fails in Doom. |
| 23 | 4 | Wis Quas | C | Reveal active invisible non-PC entries. |
| 24 | 5 | In Bet Xen | C | Find one legal random square in up to eight tries; create up to four loyal Insect Swarms there, capacity permitting. |
| 25 | 5 | An Ex Por | T/C | Adjacent ordinary/locked door becomes magically locked, preserving solid/window family. |
| 26 | 5 | In Ex Por | T/C | Adjacent magic door becomes ordinarily unlocked, preserving family. |
| 27 | 5 | Vas Mani | O/T/D/C | Non-Dead member reaches maximum HP; combat fails until victory. |
| 28 | 5 | In Zu | C | Directional cone sleeps non-immune targets that fail INT saves. |
| 29 | 5 | Rel Tym | O/T/D/C | Shared Quickness duration 30; context-specific time/activity gates. |
| 30 | 6 | In Vas Por Ylem | C | Hostile entries with effective DEX ≤ D30 take R(1,20) magical damage; caster receives attributed kill XP. |
| 31 | 6 | Quas An Wis | C | Shared Confusion duration 20; a monster-loyal actor's target search temporarily changes source loyalty if D30 > INT, including possessed/charmed PCs. |
| 32 | 6 | In An | O/T/D/C | Shared Negate Magic duration 10; suppress relevant monster magic/teleport/specials and prepared combat casting. |
| 33 | 6 | Wis An Ylem | O/T | Local X-ray for 20 update ticks. |
| 34 | 6 | An Xen Ex | C | Range-15 occupied target; reject stocks, friendly loyalty, immunity, or successful INT save; otherwise toggle Charm. Charmed PC roster status becomes Good. |
| 35 | 6 | Rel Xen Bet | C | Range-15 occupied target; failed INT save and no immunity deletes it and creates a Rat on that square. |
| 36 | 7 | Sanct Lor | C | Acting PC entry becomes invisible. |
| 37 | 7 | Xen Corp | C | Range-15 death missile; failed INT save delivers special damage 99, still subject to immortality. |
| 38 | 7 | In Quas Xen | C | Range-15 occupied target; copy complete entry/object into free slots, then repeat random placement until legal. No save; no attempt bound. |
| 39 | 7 | In Quas Wis | O/T/D | Surface local map view or dungeon symbolic view. |
| 40 | 7 | In Nox Hur | C | Cone toxifies targets with D30 ≥ effective DEX; Good PCs become Poisoned, other targets take R(0,20). No explicit immunity-family gate. |
| 41 | 7 | In Quas Corp | C | Scan monsters; non-immune failed-INT-save targets become 1 HP and flee. |
| 42 | 8 | In Mani Corp | O/T/D | Resurrect Dead member at 1 HP, restore class MP, apply karma XP reduction, recalculate level and maximum HP. |
| 43 | 8 | Kal Xen Corp | C | Up to eight placement attempts for Daemon; D30 ≥ caster INT leaves it hostile, otherwise loyal. |
| 44 | 8 | In Vas Grav Corp | C | Cone; non-immune failed-INT-save targets receive special damage 99. |
| 45 | 8 | In Flam Hur | C | Cone; each encountered target takes R(0,30) magical damage without an armor roll. |
| 46 | 8 | Vas Rel Por | O/T/D | Reject ship-shape family, including skiff; choose phase 1–8 and attempt that moongate destination. Invalid/unavailable travel fails after payment. |
| 47 | 8 | An Tym | O/T/D/C | An object with exact base Shadowlord tile 0xFC absorbs it; the other three family frames do not satisfy this test. Otherwise shared Time Stop duration 10 suppresses hostile activity and supplies effective monster DEX 1 without rewriting stored countdown DEX. |

In Redux combat, An Sanct mundane unlocking, An Ex Por magical locking, and In Ex
Por magical unlocking update the encounter reference and layered terrain together,
retiring any older terrain override at that square. Plain and windowed doors keep
their family. A successful unlock event therefore corresponds to the effective
unlocked door seen by rendering and ordinary Open. Open changes both terrain views
to floor; successful Jimmy changes both to the matching regular door, retaining
its existing physical-unlock roll and key-consumption rules. An Ex Por cannot
re-lock an already-open floor square. Chest objects and Skull Key resource rules
remain separate from these terrain transitions.

Redux In Quas Corp scans every active non-PC monster entry regardless of loyalty,
so party-loyal summons and charmed monsters can also become 1 HP and flee; whether
the original filters by loyalty is not established. Successful An Xen Ex, Rel Xen
Bet and In Quas Xen currently publish the generic zero-damage spell event, so the
Combat Event line reads "... but causes no harm" instead of naming the charm or
release, transformation or clone. This is a known text defect, not a deviation.

#### Outdoor Blink window and lifecycle

On both Britannia (floor 0) and the top-down Underworld (floor -1), Blink uses
retained gameplay window origins, not the camera viewport or a fresh range derived
at casting time. Each axis initializes to `(coordinate - 8) & 0xf0`. After **each
actual cardinal step**, wrap the position to 0..255, then compute both relative
coordinates modulo 32 against the old origins. If either is below 5 or above 26,
shift the origins **once** by `(dx * 16, dy * 16)`, modulo 256. Blocked steps,
turn cadence and rendering do not scroll this window.

Blink starts at the adjacent square, masking that first candidate to 0..255.
Both axes must lie in `[origin, min(origin + 32, 256))`. Remember the last exact
Grass on the ray and advance by plain integer addition, without wrapping subsequent
candidates. Mountains, water, and objects neither stop the scan nor disqualify an
otherwise exact-Grass endpoint. No range or placement RNG is drawn outdoors.

For example, initialization at `(119,120)` gives origins `(96,112)`. Walking east
to `(120,120)` keeps them. With only `(100,120)`, `(124,120)`, and `(140,120)` grassy,
alternative west/east casts land at 100/124. Fresh initialization at `(120,120)`
instead gives X origin 112: west fails and east reaches 140. At the world seam,
fresh X=254 east searches only 255; X=255 east searches nothing; X=0 west searches
255 through 240. Fresh X=1 has no candidates in any direction. At `(0,0)` all
directions fail because the perpendicular coordinate also lies outside the numeric
bounds. These asymmetries are part of the inspected original behavior.

| Boundary | Window behavior |
|---|---|
| Outdoor startup, town/dungeon exit, Gate/moongate, whirlpool, debug relocation | Initialize from final coordinates and plane, even for same-plane travel. |
| Walking, riding, sailing, outdoor Klimb | Scroll after each actual step. |
| Waterfall | Scroll after each of its two south steps; initialize after special plane travel. |
| Successful outdoor Blink | Search the old window, relocate, then initialize. |
| Failed/cancelled Blink, waiting, redraw, View | Preserve. |
| Combat, camping, shrine/Codex return | Preserve across temporary scene coordinates. |
| Outdoor Push/Pull | Direct displacement preserves origins. |
| Guardian pushback | Preserve the attempted step's window, including any completed shift. |
| Save and continue | Preserve the live window. |
| Actual load/import/rollback hydration | Initialize from loaded coordinates. |

Logical quadrants are NW/NE/SW/SE. East scrolling copies old NE→NW and SE→SW;
west copies NW→NE and SW→SE; south copies SW→NW and SE→NE; north copies NW→SW
and NE→SE. The outgoing half's temporary Push terrain expires; the copied half
retains it. Re-entry reads fresh persistent terrain. Initialization reloads all
four quadrants even if the origins are unchanged. Redux models this with owned
before/after Push writes on its full maps: expiration restores only values still
matching the owned write, preserving unrelated durable mutations, actors and flags.

Redux implements these outdoor rules with runtime-only plane-tagged window state.
Normal original loading rebuilds the outdoor window despite serialized offsets;
native consumers share that reset contract rather than persisting movement history.
See the [snapshot restoration contract](../details/SAVE_SNAPSHOT_SCHEMA.md#capture-and-hydration).
First-person dungeon Blink remains unavailable; combat uses its separate placement
rules in SPELL-0005.

**Implementation:** `internal/references/spell_metadata.go`, `internal/game_state/action_cast.go`, `internal/game_state/action_cast_direction.go`, `internal/game_state/outdoor_window.go`, `internal/game_state/action_push.go`, `internal/game_state/combat_utility_spells.go`, `internal/game_state/combat_utility_spell_commit.go`, `internal/game_state/combat_special_death.go`
**Tests:** `internal/game_state/action_cast_direction_test.go`, `internal/game_state/outdoor_window_test.go`, `internal/game_state/outdoor_window_push_test.go`, `internal/game_state/outdoor_window_lifecycle_test.go`, `cmd/ultimav/gamescene_outdoor_window_test.go`, `cmd/ultimav/gamescene_outdoor_window_snapshot_test.go`, `internal/game_state/combat_door_spell_test.go`, `cmd/ultimav/gamescene_combat_door_spell_test.go`

### SPELL-0003

**Saves, occupied targets, missiles, and cone collection**

**Kind:** rule
**Uses:** RNG-0001, EQUIP-0013
**Original:** Incomplete
**Redux:** Matches

An intelligence save succeeds when D30 < trunc((30 + victim INT − attacker INT)/2).
The offensive effect succeeds at or above that threshold. Grav Por, Vas Flam,
and field delivery bypass their ordinary save/hit roll. Families with explicit
immunity checks exclude Shadowlords, Blackthorn, and Lord British; do not apply
that exclusion to a routine lacking the check. Effective DEX uses the sleep,
Mimic, and Time Stop override, not always species base DEX.

Grav Por, Vas Flam, Xen Corp, and field deliveries use the range-15 weapon pipeline;
terrain may block arrival. Charm, Rat, and Clone are occupied-target operations,
not obstruction-tested missiles merely because Redux animates a flight. Magical
damage is not halved for undead; immortality still zeroes it. Rating-based attack
damage and armor subtraction reuse EQUIP-0013; direct Flame Wind does not.

Cone collection uses 21 pixel rays. From an outer ray inward, the ten divisors
are 10, 12, 14, 16, 20, 25, 35, 50, 80, 190; the center divisor is 2000 and the
other half mirrors the first. A committed pass randomly advances each live ray
by R(0,15), plots before collision, obeys pixel bounds, admits only rendered-marker
squares, and retains at most 63 distinct candidates. For each candidate square,
scan queue slots 31 down to 0 and process only the first eligible entry not already
processed by this cast. Mark that entry processed before its save/effect, then
stop the square's scan even if it resists. Do not apply the effect to every actor
overlapping that square. Each entry is processed at most once per cast. Both
loyalties are eligible; individual saves and immunity gates decide effects.
In Zu consumes its save before testing immunity.

Redux preserves these scoped thresholds, ordering, all-loyalty cone eligibility,
and candidate collection. Preview is a pure possible footprint, not a promise
that every preview cell will enter the randomized committed list. Poison Wind
saves before toxification and credits attributed caster kills; Good PCs do not
consume a fallback damage roll. Flame Wind requests 0..30 and toxify 0..20 under
the source endpoint convention, not verified external-PRNG sequence equivalence.
Original calls in a single expression do not establish argument evaluation order.

Redux area-spell feedback retains authoritative, exact-entry outcomes separately
from the processed positions. Earthquake reports eligible hostile entries that
avoid their DEX roll; Sleep distinguishes a successful INT save from immunity
reached after a failed save. Energy Vortex retains its save/immunity result and
shows it locally as well as in the combat report. Flame Wind and Poison Wind
report a resolved zero-damage roll as **no damage**, not a damaging impact.
Poison Wind avoidance has one local result, and successful poison/sleep counts
as an effect without HP loss. Cone candidates and both-side eligibility are
unchanged; no feedback path rerolls a save or damage.

A confirmed cone with no eligible targets says **No targets reached**. A wholly
unaffected area cast summarizes avoidance, resistance, immunity, or no damage;
casts with disclosed successful effects rely on their per-target results.
Undisclosed targets do not acquire local markers, names, or diagnostic disclosure
through this feedback, and aggregate wording does not reveal hidden outcomes.
These are Redux presentation additions: confirmed casts retain their sound,
animation, mixture/MP cost, and single action completion even when ineffective.

**Implementation:** `internal/game_state/combat_cone.go`, `internal/game_state/combat_cone_spells.go`, `internal/game_state/combat_field_effects.go`, `internal/game_state/combat_remaining_spells.go`, `internal/game_state/combat_spell_targeting.go`
**Tests:** `internal/game_state/combat_cone_test.go`, `internal/game_state/combat_spell_parity_test.go`, `internal/game_state/combat_area_spell_outcome_test.go`, `cmd/ultimav/gamescene_combat_area_spell_outcome_test.go`

### SPELL-0004

**Duration effects, light, artifacts, and action ownership**

**Kind:** rule
**Uses:** TIME-0003, EQUIP-0014
**Original:** Incomplete
**Redux:** Intentional deviation

Original Protection, Quickness, Confusion, Negate Magic, and Time Stop share one
active effect and duration with Crown, Amulet, and Badge. Recasting the same spell
refreshes its full duration; a different effect replaces it. Reusing the active
artifact removes it. Crown and Negate Magic therefore cannot coexist. Magical
light remains independent: prepared In Lor/Vas Lor set 100/255, Light scroll 240.

The normal combat player-controlled action tail decrements duration, including
controlled monsters. Hostile/possessed dispatch does not; numeric actor selection
is free. Undefined exceptional player tails are qualified in COMBAT-0002. Fields
expire separately and standing hazards follow dispatch. Outside combat, normal
context turn boundaries own decrement/gates. Quickness halves ordinary exploration
minutes with minimum one and alternates applicable town/dungeon/world activity;
combat uses a fresh R(0,1) permission gate at each monster-loyal due action. Time
Stop suppresses those actions before sleep, specials, or attacks, while countdowns
still use stored DEX. Forecasts must not consume RNG.

Prepared An Tym's original absorption scan tests each of the 32 object tiles for
exact base Shadowlord tile 0xFC, not the complete four-frame family 0xFC–0xFF and
not a separate living-species test. Redux instead checks authoritative Shadowlord
presence and active living Shadowlord combat species. **REDUX DEVIATION: SPELL-0004:**
identity-based absorption is deliberately retained so presentation variants cannot
bypass a living Shadowlord's absorption; frame-sensitive equivalence is not claimed.

Redux keeps Crown competing with the five duration spells, but Badge and Amulet
are independent. Applying a spell removes Crown; donning Crown clears the spell.
Removal/expiry never resumes a replaced effect. This hybrid preserves independent
artifact usability while retaining one competing combat-duration slot; native
fields remain separate without legacy normalization. Redux also applies effective
Protection +3 from live equipment defense; the original stored-armor synchronization
gap remains EQUIP-0014, not proof of retail +3 behavior.

Redux additionally presents Time Stop with **5% normal-speed actor-body animation**.
In combat the flag follows exact effective allegiance: monster-loyal monsters and
possessed PCs/clones slow down, while party-loyal actors continue normally. Surface
creature NPCs/monsters and the first-person dungeon wanderer also use slowed body
clocks; vehicles, environmental hazards, and the exploration Avatar do not. Each
live actor retains its animation phase, including fractional elapsed time, across
activation, expiry, effect replacement, and allegiance changes. Leaving the
viewport does not reset a surviving actor; retirement or scene replacement ends
that presentation lifetime.

This near-still rate is a Redux visual choice, not an established original
animation rate. It adds no gameplay RNG draws and does not rescale duration
counters, stored DEX, calendar rules, or action suppression. Fields, terrain,
lighting, status overlays, projectiles, injury/impact feedback, and UI retain
their ordinary timing. The An Tym scroll shares this presentation without changing
its separate 20-unit duration under CONSUME-0003.

**Implementation:** `internal/game_state/combat_duration_spells.go`, `internal/game_state/combat_protection.go`, `internal/game_state/duration_effect.go`, `internal/game_state/toggle_item_state.go`, `internal/game_state/actor_animation_presentation.go`, `cmd/ultimav/actor_animation_clock.go`
**Tests:** `internal/game_state/combat_duration_spells_test.go`, `internal/game_state/combat_protection_test.go`

### SPELL-0005

**Placement, dungeon cells, resurrection, and unsafe limits**

**Kind:** rule
**Uses:** DNG-0008, PROGRESS-0001, PROGRESS-0002, RNG-0001
**Original:** Incomplete
**Redux:** Known discrepancy

Summon placement tries up to eight random coordinates, subject to terrain,
occupancy, and free source/object slots. Kal Xen tests Rat terrain even when the
chosen animal is another species; Daemon placement tests Daemon terrain. In Bet
Xen puts up to four independently created insects on the one admitted square.
Prepared Daemon binding draws only after successful placement; the scroll bypasses
the binding draw. Redux preserves these placement-shape and binding distinctions.

Outdoor Blink placement follows SPELL-0002. No eligible Grass reports **Failed!**,
preserving both position and window; it does not mean an intervening obstacle
stopped the ray. Under SPELL-0001's atomic prepared-Cast policy, a confirmed legal
direction pays one mixture and 3 MP exactly once even on this failure. Precommit
cancellation is free. Surface-combat Blink independently tries up to seven random
legal unoccupied destinations. Dungeon rooms/encounters retain precommit rejection;
outdoor occupancy and deterministic scanning do not alter combat legality.

Original Clone reserves free slots and then has an unbounded random destination
loop. No legal destination can mean nontermination. Clone tests Player terrain,
not the copied monster's shape. Its complete-copy semantics differ from division's
fresh hostile child. Do not replace the original unbounded loop with an imagined
safe failure or infer arbitrary original RNG argument order from Redux's order.
Redux deliberately bounds destination sampling to 256 attempts, returning a paid
failure rather than risking nontermination or permitting free placement rerolls;
capacity/target validation remains free before commitment. A Redux PC clone is an
extra battlefield entry backed by the same roster character, so HP and status are
shared rather than copied.

Dungeon fields accept only a facing packed byte whose value after clearing bit
0x08 is zero, preserving that bit. Electric field 0x8B is creatable, but original
contact damage tests exact 0x83; the two must not be silently normalized. An Sanct
opens a high-nibble-0x40 chest and An Grav removes a field at the current cell
before trying wrapped facing coordinates. Creation only tests facing coordinates.
Floor changes reject Doom; original boundary/failure reporting need not agree
with the physical transition. Redux validates implicit dungeon targets and floor
changes before payment and reports failure atomically.

Vas Rel Por's original ship-shaped high-nibble gate includes skiffs. Redux rejects
both frigates and skiffs before destination selection, retaining the independently
documented atomic prerequisite/resource commitment policy. Rejection spends no
prepared spell, mana, or time through the prerequisite check.

Resurrection applies the shared sequence in CONSUME-0004, including karma-scaled
XP before level/max-HP recalculation. Original effect routines have differing
success conventions: an empty target scan can still report success. No normalized
effect-count contract is inferred from their returns.

**Implementation:** `internal/game_state/combat_summon_spells.go`, `internal/game_state/combat_remaining_spells.go`, `internal/game_state/dungeon_spells.go`, `internal/game_state/action_cast.go`
**Tests:** `internal/game_state/combat_summon_spells_test.go`, `internal/game_state/dungeon_spells_test.go`, `internal/game_state/vas_rel_por_test.go`

### SPELL-0006

**Redux character targeting, chest authority, and directed awakening**

**Kind:** rule
**Uses:** COMBAT-0006
**Original:** Established
**Redux:** Intentional deviation

Original character spells select roster members. Redux combat Mani, An Nox,
An Zu, and post-victory Vas Mani select visible living battlefield PC entries,
including sleeping, shimmered, possessed members and clones. Fled, underground,
terrain-concealed, and dead entries are unavailable. This makes the selected
battlefield identity explicit while HP/poison remain shared roster state.
Noncombat selection remains roster-based. An Zu still requires shared Sleep and
scans for the first eligible sleeping copy in source-slot order; it can wake a
copy other than the highlighted one, irrespective of selection visibility.

An Sanct resolves and disarms a live authoritative chest, not merely a rendered
chest tile. Validation and mutation retain the same object slot, including piles.
Redux also records party-shared safe knowledge; dungeon opening retires closed-
chest assessment without an inspection roll. This knowledge is Redux assistance.

Combat An Sanct, An Ex Por, and In Ex Por use the familiar full-size, uniformly
white attack cursor on one of the caster's four cardinal neighbours. Arrow keys
and keypad 8/2/4/6 select that neighbour directly; repeated input never extends
range, diagonals are ignored, and map edges do not wrap. The initial neighbour is
chosen geometrically (north, then east/south/west if clipped), without inspecting
terrain or objects. Enter, keypad Enter, C, or Space confirms; Escape cancels
without spending MP, a prepared spell, RNG or the combat action. Invalid targets
remain in selection without cost. Cursor colour and aim text do not disclose
concealed terrain, lock eligibility, or unassessed chest traps. The active actor
indicator remains above its HP bar; these spells do not also draw directional
arrows or the older coloured adjacent underlay. Other adjacent and cone selectors
retain their existing previews.

Directed offensive magic shares the intuitive attack-awakening policy of
COMBAT-0006, including resisted/blocked/absorbed attacks. Direct fire/poison field
delivery qualifies; standing hazards and nearby cone victims do not. Newly applied
sleep fields remain asleep; protection and cloning are not attacks. Formula
knowledge and Chronicle ownership are maintained in MIX-0004 rather than inferred
from the prepared inventory.

**Implementation:** `internal/game_state/combat_party_spell_targeting.go`, `internal/game_state/combat_field_effects.go`, `internal/game_state/action_cast_direction.go`
**Tests:** `internal/game_state/combat_party_spell_targeting_test.go`, `internal/game_state/combat_attack_wake_test.go`
