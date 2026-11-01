# Provisions and consumption

Shared quantities and legacy offsets are cataloged in the
[inventory](INVENTORY_SYSTEM.md) and [save format](SAVED_GAM_STRUCTURE.md).
These DOS rules concern particular mutation paths, not every grant, theft, monster
effect, or malformed loaded state. Random intervals below state requested bounds;
the original generator's exact distribution is not independently established.

### PROV-0001

**Poison, diners, meals, and starvation ordering**

**Kind:** rule
**Uses:** KARMA-0012
**Original:** Established
**Redux:** Matches

At an original player update, visit party slots in order and sample each status.
Clear the active designation when its member is Dead. A sampled member who is
neither Dead nor Sleeping counts as a diner; first apply one HP of poison damage
if Poisoned. Even if that point kills the member, the sampled diner still counts.

On an observed hour mismatch:

```text
if food is already zero:
    damage each non-dead member in slots below min(party_size, 6)
    with a separate requested roll from 1 through 8
else if the observed hour is 6, 12, or 18:
    food = max(0, food - diner_count)
record current hour as the comparison baseline
```

Sleeping members do not eat but can starve. A meal exhausting positive food does
not immediately starve anyone in the same branch. With food already zero,
starvation occurs at any observed changed hour, not only mealtimes. For example,
one food and three diners at 06:00 leave zero food without starvation; a later
observed 07:00 mismatch then invokes damage.

Afterward the charity counter advances, eligible finite spell duration decrements,
and Regeneration Rings are checked. Each eligible non-dead wearer has a requested
one-of-eight healing opportunity for one HP capped at maximum. These are separate
steps, not effects of the meal itself.

Redux caches diner eligibility before poison and preserves the reviewed meal/
starvation ordering for a single processed hour. Surface, dungeon, and bed updates
use that ordering. This match is scoped to consequences, not **which hours** are
processed; that differs as described below.

**Implementation:** `internal/game_state/environmental_integration.go`, `internal/game_state/turn.go`, `internal/game_state/action_hole_up_bed.go`
**Tests:** `internal/game_state/turn_test.go`

### PROV-0002

**Observed hour versus a backlog of elapsed hours**

**Kind:** rule
**Uses:** TIME-0001, PROV-0001
**Original:** Established
**Redux:** Matches

The original tests one pair of hour bytes. It does not enumerate all crossed
hours. A nonzero time request overwrites the previous-hour baseline even when no
player update follows, so later time advancement may erase an earlier crossing.
Time advancement alone is not a food/starvation event.

No ordinary player update occurs inside camping. An uninterrupted original camp
ends at minute 0–4. Outdoor completion normally adds two minutes first, replacing
the terminal-hour mismatch without crossing another hour; dungeon completion
does not have that extra post-camp charge and may still observe the terminal
crossing. Neither path replays the intermediate camp hours.

Redux replaces the hour observation on every positive request, including rest and
requests suppressed by Time Stop. A player-update boundary compares it with the
current hour and processes at most one consequence. A later same-hour advance
erases an earlier crossing. Its transient deferred marker supports workflow/save
eligibility; it is not an elapsed-hour backlog. Zero requests do not replace the
observation. Sailing interruption returns to input without an extra player update.
Bed's local rest-hour processing remains distinct.

For example, 121 minutes from 04:59 reaches 07:00 and can produce one starvation
consequence if observed immediately. Another one-minute request before the update
replaces the baseline with 07:00 and produces none. Modern full-duration camping
remains the independently declared CAMP-0005 deviation.

**Implementation:** `internal/game_state/turn.go`, `internal/game_state/action_hole_up_camping.go`, `internal/game_state/action_hole_up_dungeon.go`
**Tests:** `internal/game_state/turn_test.go`, `internal/game_state/action_hole_up_dungeon_test.go`, `internal/game_state/findings_closeout_test.go`

### PROV-0003

**Torch commitment and carried fuel**

**Kind:** rule
**Uses:** TIME-0001, TIME-0002
**Original:** Established
**Redux:** Matches

With no torches, Ignite changes neither quantity nor fuel. Otherwise consume one:

| Context | Assignment |
| --- | --- |
| Dungeon map IDs 33–40 | Add `112 + roll(0,15)` to existing torch fuel, capped at 255. |
| Other contexts | Replace torch fuel with 240. |

Torch and magical-light counters are independent of the active-duration spell
slot. Positive effective time subtracts from both with saturation; Time Stop
freezes them and Quickness changes the requested decrement through TIME-0001.
Surface completion can immediately reduce a newly assigned value; dungeon timing
has its pre-command owner. Thus 240 is an assignment, not a guarantee of 240
future commands. An empty-inventory refusal is not universally free of caller time.

Redux matches the reviewed consumption and additive/replacement fuel rules through
`ActionIgnite`. Its dungeon roll uses the controlled game RNG. This comparison
does not assert identical lighting radii, cosmetic feedback, or cancellation costs.

**Implementation:** `internal/game_state/action_ignite.go`, `internal/game_state/turn.go`

### PROV-0004

**Regular Keys and physical Jimmy attempts**

**Kind:** rule
**Uses:** INV-0004
**Original:** Established
**Redux:** Intentional deviation

Verified pickup, NPC, and Guild grants cap Keys at 99. Surface Jimmy requires a
key before direction. For an ordinary locked door, choose a character and succeed
when `DEX > roll(0,29)`: success preserves the key; failure consumes one.
Applicable restraints use the same comparison after their occupancy/NPC gates.
A magical door always consumes one and remains closed. Earlier direction or
character cancellation consumes none.

Chest paths have their own rules:

| Chest state | Original Jimmy result |
| --- | --- |
| Surface chest encoding below 128 | Consume one key. |
| Surface high-bit chest | Success if `roll(1,30) > (30 + low_seven_bits - DEX) / 2`; clear high bit and preserve key. Failure consumes one. |
| Dungeon ordinary closed chest | Consume one key. |
| Dungeon trapped chest | Success if `roll(1,30) > (30 + 2 * dungeon_level - DEX) / 2`; change to ordinary closed chest and preserve key. Failure consumes one. |

Division is integer arithmetic. Dungeon Jimmy examines the current cell, not a
forward adjacent door. Blackthorn's renewable key Search requires zero regular
keys and no object at the search position; this is not a global duplicate-copy
scan over every loaded object.

Redux retains success-preserves/failure-consumes physical unlocking. On a
tactical map, stocks and manacles hold no prisoner: a physical Jimmy with the
door comparison clears the restraint to floor on success and consumes one key
on failure, as the original's combat branch does.
**REDUX DEVIATION: PROV-0004:** a chest the party already knows to be safe may
refuse Jimmy harmlessly, avoiding a knowingly wasted key without revealing hidden
trap state. Blind attempts keep the original cost. Direct character selection
also uses an established Good/Poisoned eligibility policy rather than trusting
every existing designation. Exhaustive search-renewal and all chest-context
equivalence remain unverified.

**Implementation:** `internal/game_state/action_jimmy.go`
**Tests:** `internal/game_state/action_jimmy_prisoner_test.go`, `internal/game_state/combat_chest_assessment_test.go`, `internal/game_state/combat_terrain_commands_test.go`

### PROV-0005

**Gems are committed before exploration View**

**Kind:** rule
**Uses:** INV-0004, TIME-0002
**Original:** Established
**Redux:** Unverified

Gems are shared stock. Reviewed pickup, NPC, and Guild grants cap it at 99.
Exploration View decrements one before entering its surface or dungeon viewer;
zero stock prevents the viewer. Tactical combat rejects View before this path.
This is a resource-commitment rule, not a universal time cost or a specification
of which map information becomes visible. Redux's full View workflow and knowledge
policy require their owning topic's review.

### PROV-0006

**Skull Key discovery and Use commitment**

**Kind:** rule
**Uses:** INV-0004, TIME-0002
**Original:** Established
**Redux:** Intentional deviation

Skull Keys have a separate quantity from regular Keys. Encoded pickup with bit 7
set adds the low-seven-bit quantity, capped at 99. The renewable Search is allowed
when current day differs from its saved marker. Successful **search/reveal**, not
later pickup, records that day; month rollover resets the marker.

Original Use consumes one after item selection but before context/direction/effect
validation. Surface and tactical contexts invoke the two-variant magical-door
unlock effect; dungeon exploration rejects it. In tactical combat, including a
dungeon room, the adjacent cardinal target is measured from the acting PC entry,
not the party's exploration/entry position. Each magical-door variant becomes
its corresponding ordinary **closed** door, retaining whether it has a view.
The effect does not open that door. Later cancellation or an
ineffective target does not refund the key. Cancelling the item menu before
selection consumes nothing. This is not a universal unlock for ordinary locks.

**REDUX DEVIATION: PROV-0006:** Redux validates context and supports free
pre-confirmation cancellation before its committed use attempt. Invalid dungeon
use preserves the key. Once committed, consumption precedes the unlock operation
and no-effect/stale targeting does not manufacture a refund. This follows the
existing validated-attempt policy so uncommitted rejection is atomic. Discovery
marker equivalence is not claimed by this narrower Use review.

Redux makes owned Skull Keys selectable in tactical combat while retaining the
first-person dungeon and Underworld exploration restrictions. A direction prompt
retains the exact encounter, PC/clone entry, character, dispatch, actor origin,
map identity, and inventory quantity. Changed ownership, movement, map, or stock
before confirmation rejects the attempt for free. A confirmed cardinal direction
spends one key and one combat action, including an ordinary physical lock, a
non-door square, or a direction beyond the battlefield edge; edges do not wrap.
Both authoritative combat terrain and its displayed map receive the unlock.
The owning Use workflow completes only its own original actor action; neither
key consumption nor unlocking adds a separate surface time/provision cycle.

**Implementation:** `internal/game_state/action_use_commits.go`, `internal/game_state/combat_skull_key.go`, `internal/game_state/combat_use_owner.go`, `cmd/ultimav/gamescene_skull_key.go`
**Tests:** `internal/game_state/action_use_commits_test.go`, `internal/game_state/combat_key_watch_test.go`, `cmd/ultimav/gamescene_combat_key_watch_test.go`
