# Player command dispatch and interaction boundaries

This reference describes the inspected DOS source's command routes and scoped
Redux comparisons. Source labels do not identify an immutable retail build.
Dispatch permission is not proof that every target, resource, actor, or return
path succeeds. Minute requests follow the temporal rules in TIME-0001; valueless
original returns are not assigned invented timing outcomes.

### CMD-0001

**Exploration and tactical command matrix**

**Kind:** rule
**Uses:** TIME-0002, INV-0003, INV-0006, EQUIP-0010, DNG-0003
**Original:** Established
**Redux:** Unverified

World exploration uses map ID 0 with level selecting Britannia/Underworld;
small maps use 1–32; dungeon exploration 33–40; tactical combat uses IDs above
127 and a separate command switch. Dir below means a cardinal direction prompt;
Current means the packed dungeon cell under the party. PC-only tactical routes
require a player-controlled party actor.

| Key | Command | Britannia / Underworld | Small map | Dungeon exploration | Tactical combat |
| --- | --- | --- | --- | --- | --- |
| A | Attack | Dir outdoor attack | Dir town attack | Wanderer directly ahead | Attack/aim each readied damage item |
| B | Board | Vehicle underfoot, no Dir | Same | Not here | Board what? |
| C | Cast | Outdoor-legal spells | Town-legal spells | Dungeon-legal spells | PC only, combat-legal spells |
| D | Unassigned | Reject | Reject | Reject | Reject |
| E | Enter | Location/dungeon underfoot | Enter what? | Enter what? | Not here |
| F | Fire | Valid frigate broadside | Adjacent cardinal cannon | What? | Not here |
| G | Get | Dir object pickup | Dir object pickup | Open chest in Current | PC-only tactical Get |
| H | Hole up | Camp on foot or repair ship | Underlying bed required | Dungeon rest | Not here |
| I | Ignite | Surface torch | Surface torch | Dungeon light | Not here |
| J | Jimmy | Dir lock attempt | Dir lock attempt | Current chest disarm | PC-only tactical Jimmy |
| K | Klimb | Mountain attempt | Stairs/ladder or adjacent climb | Current ladder/pit/grapple route | Ladder or small rocks |
| L | Look | Dir | Dir | Relative Look | Not here |
| M | Mix | Prepare spells | Same | Same | Not here |
| N | New Order | Reorder party | Same | Same | Not here |
| O | Open | Dir | Dir | Current chest | PC-only tactical Open |
| P | Push | Dir push/pull | Dir push/pull | Not here | Dir after trap check |
| Q | Quit | Save decision | Save decision | Save decision | Not here |
| R | Ready | Select actor/equipment | Same | Same | Acting PC; body armor locked until victory |
| S | Search | Dir | Dir | Relative Search | PC-only tactical Search |
| T | Talk | Dir, no response | Dir NPC interaction | No response | No response |
| U | Use | Usable-item projection | Same | Same | PC-only context-sensitive effects |
| V | View | Spend gem, area view | Same | Spend gem, dungeon view | Not here |
| W | Unassigned | Reject | Reject | Reject | Reject |
| X | X-it | Leave vehicle when legal | Same | X-it what? | X-it what? |
| Y | Yell | Sail toggle or input | Input | Input, ordinarily no effect | Tactical Yell |
| Z | Z-stats | Party stats | Same | Same | Party stats |
| Space | Pass | Pass; sailing first loses sail state | Pass | Pass | End actor action |
| Arrows | Movement | Absolute cardinal | Absolute cardinal | Advance/right/back/left relative to facing | Absolute; legal edge exits |
| Return / period | Context input | Both reject; Return completes, period free | Both reject without completion | Turn around (Return in ordinary non-debug build) | Not movement |
| 0–9 | Selection/music | Active player | Active player unless at harpsichord | Active player | 0 clears; 1–6 select combat PC |
| Escape | Context input | Not Quit | Not Quit | Not Quit | Attempt battlefield escape |

Combat rejects B,D,E,F,H,I,L,M,N,Q,T,V,W,X and unknown keys, returning to
the same actor prompt. Accepted commands normally reach end-action; active-player
digits are an explicit exception. Combat Cast/Get/Jimmy/Open/Search/Use have PC
gates. Exploration command availability must not be inferred from this tactical
loop or vice versa. Redux routes through separate map-specific input handlers;
full raw-key/actor/target equivalence is not claimed by this matrix.

Redux's Q confirmation preference and Ctrl+Q active-slot reload extension are
specified separately in [CMD-0002](#cmd-0002); the table above describes the
original single-key command routes.

**Implementation:** `cmd/ultimav/gamescene_input_smallmap.go`, `cmd/ultimav/gamescene_input_largemap.go`, `cmd/ultimav/gamescene_input_dungeon.go`, `cmd/ultimav/gamescene_input_combat.go`

### CMD-0002

**Completion exceptions, Quit, and undefined Yell results**

**Kind:** rule
**Uses:** TIME-0002, DNG-0003
**Original:** Incomplete
**Redux:** Intentional deviation

TIME-0002 owns ordinary context completion. Outdoor Return is a particular
exception: it prints rejection but leaves completion enabled. Town Return
disables it. Period reaches the common rejection branch and is free in both.
Town Fire searches cannon terrain north, east, south, west; finding none still
owns ordinary town completion. Dungeon facing turns retain precharge without
the ordinary end-player/hazard pass; advance/back-up may run that pass.

Original Q asks only an uppercase Y/N save decision. N returns to play; Y reads
the 256-byte `UNDER.OOL` and `BRIT.OOL` lists, writes `SAVED.GAM`'s saved region
and combined 512-byte `SAVED.OOL`, then returns to play. It is quick-save, not
termination to DOS. Outdoor/town ordinary completion is disabled. Dungeon time
was already charged before reading Q, but no additional end-player pass follows.
Combat Q rejects and re-prompts without saving or consuming the actor action.

The original Yell routine returns no value on both its ship sail-toggle and
empty-input paths despite its integer return type being consumed by dispatch.
The resulting completion value is undefined in C, not reliably free or charged.
Town-specific Yell failures have additional unestablished returns; do not turn
the successful summon contract into a guarantee for every input. Debug/exit
control keys vary by build and are not the ordinary A–Z command set.

**REDUX DEVIATION:** Primary Escape outside combat opens the application/menu
workflow; it is not an original DOS Quit key. Redux gives commands explicit
completion ownership rather than emulating undefined returned register values.
Its current native-save workflow is separate from the original file-write facts.

**REDUX DEVIATION:** Q uses the active native player slot and reviewed revision
from this scene's last successful player-slot save/load, preserving its committed
description. The **Confirm Quick Save** preference defaults On: the existing
`Save game?` Y/N/Escape question and success acknowledgement remain. Off saves
immediately with slot-specific output and no success acknowledgement. Both paths
show acknowledged errors, retain provenance on failure, and explain missing slot
authority rather than choosing a destination. Surface capture occurs outside
surface command-cycle ownership; dungeon Q completes its accepted precharged
command-only token before capture, exactly once, retaining Quickness phase and
avoiding full end-player processing. Ordinary combat Q remains rejected. This
preference reduces repeated save keystrokes without relaxing save eligibility or
optimistic revision checks.

**REDUX DEVIATION:** Exact **Ctrl+Q** reloads the same active slot directly, with
no confirmation, new preference, outgoing save, or rollback capture. Either
physical Control key is accepted; Shift/Alt/Meta are excluded and Caps Lock is
neutral. It supports normal exploration and player-controlled combat input,
including controlled-monster turns, for quick retries. Enemy/action owners,
targeting, modals, readers and workflows block the shortcut; harmless floating
labels and nonblocking action cards do not. Blocked chords are suppressed through
release and never queued or converted to plain Q. Loading charges no action,
minute, resource or gameplay RNG. Canonical native candidate activation and
replacement retain the old scene/provenance on failure, dispose its transient
owners safely on success, and report success on the replacement. Stale revisions
are refused rather than refreshed; use ordinary reviewed Load for a changed or
unusable checkpoint. The preference affects Q only, not Ctrl+Q or menu/departure
confirmations. These are Redux native-save/input extensions, not claims about
original keyboard or file-write behavior.

**Implementation:** `cmd/ultimav/gamescene_input.go`, `internal/game_state/dungeon_command_cycle.go`, `cmd/ultimav/gamescene_input_quit.go`, `cmd/ultimav/gamescene_quickload.go`, `cmd/ultimav/gamescene.go`, `cmd/ultimav/gamescene_transient_command_input.go`, `cmd/ultimav/player_save.go`, `cmd/ultimav/player_save_ui.go`, `cmd/ultimav/scene.go`, `internal/config/gameplay_preferences.go`
**Tests:** `cmd/ultimav/gamescene_quicksave_test.go`, `cmd/ultimav/gamescene_quickload_test.go`, `cmd/ultimav/gamescene_quickload_release_test.go`, `cmd/ultimav/player_save_quickload_test.go`, `internal/config/gameplay_preferences_test.go`

### CMD-0003

**Ordinary direction input, Pass, and free precommit Escape**

**Kind:** rule
**Uses:** TIME-0002
**Original:** Established
**Redux:** Intentional deviation

Original ordinary direction reading accepts only Space and four cardinal codes,
producing one-step offsets. Space reports Pass and returns no direction. An
Escape case exists after the input loop but cannot be reached through that loop.
Other keys keep waiting. Combat missile/spell crosshair paths are separate and
may support diagonals; that does not enlarge exploration direction input.

**REDUX DEVIATION:** Before commitment, surface Get, Push, Open, Jimmy, Klimb,
Look, Talk, Search, Attack and outdoor Fire permit Escape with no time, actor
phase, resource, or gameplay RNG. It restores primary input, reports cancellation,
and consumes Escape through release so the same press cannot open the menu.
Combat Klimb, Open, Jimmy, Push, Get, and Search ordinary direction prompts also
permit free Escape before commitment, retaining the exact actor and action. This
includes controlled-monster Push and an ordinary Klimb retry. Consuming Escape
through release prevents the held press from becoming a battlefield-exit request.
Single-route underfoot exits, the dual-ladder U/D selector, tactical attack/spell
selectors, and item workflows retain their own contracts.

This lets accidental commands be abandoned. Space remains Pass: ordinary surface
commands complete their map cycle, except outdoor Attack Pass is free. Invalid
keys/keypad diagonals leave the prompt active. Tactical aiming and dungeon
retained cycles keep their own rules. Current Use is an item workflow, not the
declared but unreachable ordinary Use-direction state.

Surface prompts use short command text without an inline key legend; contextual
Help explains cardinal input, Escape and Space's command-specific cost. The six
ordinary combat prompts advertise `Arrows; Space: Pass; Esc: Cancel`; their Pass
spends exactly one action. Hole Up guard selection has a
separate boundary: Escape cancels setup, while explicitly choosing No One starts
unguarded camping. Surface cancellation is free; dungeon cancellation retains
its precharged cycle. Neither cancellation runs camp setup, stowing, rest, or
encounters.

**Implementation:** `cmd/ultimav/gamescene_surface_direction_cancel.go`, `cmd/ultimav/gamescene_combat_direction_cancel.go`
**Tests:** `cmd/ultimav/gamescene_surface_direction_cancel_test.go`, `cmd/ultimav/gamescene_combat_direction_cancel_test.go`

### CMD-0004

**Crop feedback and named exterior Look targets**

**Kind:** rule
**Uses:** KARMA-0006, WORD-0001, TIME-0002
**Original:** Established
**Redux:** Intentional deviation

Crop mutation and theft cost belong to KARMA-0006. Original feedback reports
the crop pickup without a theft warning. **REDUX DEVIATION:** Redux adds an
explicit ownership warning in the message log alongside pickup feedback,
clarifying the unchanged mechanics.

Exterior Look gives the top object precedence over terrain. Terrain inspection
wraps at world seams; the original dynamic-object comparison retains the raw
signed target and does not reliably find an opposite-edge object. Only sealed
dungeon entrances gain a name, chosen by target X; open entrances keep generic
tile descriptions.

**REDUX DEVIATION:** Redux names both open and sealed exterior portals, including
Underworld exits, using full wrapped coordinate, permitted world plane, and
effective portal tile. Objects/overlays still win. Successful inspection records
name and open/sealed state in the Journal without revealing a Word of Power.
Interior dungeon ladders retain generic descriptions. This improves geographic
identification without guessing identity from a cave sprite or revealing a spell.
Look still owns its ordinary context completion.

**Implementation:** `internal/game_state/action_get.go`, `internal/game_state/action_look.go`

### CMD-0005

**Town beds and Klimb entry conditions**

**Kind:** rule
**Uses:** TIME-0002, NPC-0005
**Original:** Established
**Redux:** Unverified

Town Hole Up starts only on underlying bed tile `0xAB`, not paired `0xAC` or
an occupied-bed object shape. Before sleep, sixteen NPC-driver/display passes
occur, aborting only when a pass's final pending command is attack. Each sleep
cycle requests ten minutes, updates players/duration effects, rebuilds live
NPC placement and checks bed occupancy until the target hour. Return from the
special loop still has ordinary town completion and a fresh NPC pass.

Town Klimb refuses riders. Underfoot ladder/grate routes use their floor change;
otherwise adjacent Small Rocks and either fence orientation are valid. Opening
the direction prompt does not itself emit a preliminary failure. Current Redux
bed and climb authorities are separately implemented; this conversion does not
certify every temporal-effect/NPC-interruption combination.

**Implementation:** `internal/game_state/action_hole_up_bed.go`, `internal/game_state/action_klimb.go`

### CMD-0006

**Small-map door identity, physical openness, and a visit's lifetime**

**Kind:** rule
**Uses:** PROV-0004, PROV-0006
**Original:** Established
**Redux:** Intentional deviation

Original physical unlocking changes only current town terrain. Open remembers
the effective closed variant; ordinary closure restores that exact variant.
Floor reconstruction and combat return reload source locks, and edited town
terrain is outside the original saved region; `SAVED.OOL` contains outdoor lists.
Original drawing clears a terrain display cell under an actor but does not
physically open/unlock a door. Timed closure has no occupancy deferral.

**REDUX DEVIATION:** Redux preserves the latest regular/magical closed-door
override throughout a physical small-map visit, including view variants and
discovered secret doors once unlocked. Jimmy, An Sanct, In Ex Por, and Skull
Key share that policy; deliberate relock supersedes unlock and vice versa.
Reconstruction retains only `RegularDoor`, `RegularDoorView`, `MagicLockDoor`,
and `MagicLockDoorWithView` overrides on rebuilt floors. Other mutations and
searched-but-never-unlocked secret doors do not gain this policy. Same-location
floor/full/debug rebuild, combat return and native continuation retain it, to
avoid source-reload soft locks.

Two independent sources produce current physical openness:

* One player-open door remembers exact closed identity and floor. Its countdown
  is 2 → 1 → 0 → close, deferred at zero while occupied by party or visible,
  nonempty same-floor actor. Zero may be unoccupied at a completed boundary if
  the occupant leaves after that turn's door phase.
* Every occupied doorway, including locked/magical and view variants, is held
  open as live floor for rendering, sight, daylight and collision, without
  persistent floor mutation or unlocking. Multiple holds can coexist. Hidden/
  empty actors, stale sprites and actors on another floor do not hold it open.

Closing/replacing the timer restores its closed identity but does not cancel an
independent occupant hold. Opening B ends A's timer without restoring A's older
source lock/secret wall or closing live terrain under its occupant. Failed Open
does not replace the timer. Lock operations and NPC traversal use persistent
terrain; occupancy openness creates no NPC traversal privilege and cannot let
the Avatar walk through another actor. Hydration derives holds from final actors.

Leaving for a different physical location discards the visit. Immediate re-entry
starts source locks. A matching destination preserves it; temporary combat/prison
scenes suspend it. Native saves carry closed-door mutations and the timer
separately; actor holds are derived anew. Key/spell costs and completion remain
with their command authorities.

Stonegate's destructive flood discards superseded door overrides and its current-
floor timer without restoring a door. Other floors and unrelated overrides remain;
[ENV-0002](ENVIRONMENTAL_HAZARDS.md#env-0002) owns the intentional original-timer
edge-case difference and unchanged trap consequences.

**Implementation:** `internal/game_state/small_map.go`, `internal/map_state/action_open_door.go`, `internal/game_state/action_fall_through.go`

### CMD-0007

**Search actor, target precedence, and one honest chest assessment**

**Kind:** rule
**Uses:** DNG-0004, RNG-0001
**Original:** Established
**Redux:** Intentional deviation

Original surface object-chest Search rerolls Intelligence each time and can
report a nonexistent trap or false safety. Dungeon Search has its distinct
Dexterity/depth test and fabricated severity fallback (DNG-0004).

**REDUX DEVIATION:** Redux gives each chest lifetime one party-shared honest
assessment: trapped, safe, or uncertain with an explicit danger warning.
Changing inspectors and repeat Search cannot reroll or erase it. Search never
opens, disarms, or triggers the chest. The first eligible inspection makes one
Redux uniform `1..30` draw. For surface/combat objects, difficulty is
`trunc((30 - INT)/2)` when physically safe, or
`trunc((30 + treasure_value - INT)/2)` when armed; a draw at least difficulty
is conclusive. Dungeon uses its PROV-0004 Dexterity/internal-floor difficulty,
with strictly greater comparison. Normal command completion can still draw RNG.

Conclusive results disclose actual state; otherwise uncertainty persists.
Surface/combat trap descriptions are simple below value 10, complex above 20;
dungeon descriptions are simple below internal floor 4, complex at least 7.
Dungeon safety tests actual low three bits, so both `0x40` and `0x48` are safe;
original Search recognizes only `0x40`. This removes false/fabricated reports
and repeat-search fishing, not physical danger.

Redux surface Search uses an eligible selected active character directly,
otherwise runs eligible selection. That actor owns trap inspection and moldy
corpse plague effects. For a valid target, authority handles chest, corpse,
secret door, moonstone, reagent cache, and fixed-source record in priority order.
It composes one complete terrain-aware message for popup and log, then completes
Search once. Fixed reward decoding and transfer are distinct from assessment.

Knowledge follows the physical chest or active dungeon visit through normal
refresh, floor change and combat return. Removal/replacement, nested children,
reused combat slots, fresh dungeon visits and town object reconstruction begin
new lifetimes; combat knowledge is encounter-local. Native persistence carries
knowledge with physical state; older saves/imports begin uninspected.

**Implementation:** `internal/game_state/action_search.go`, `internal/game_state/chest_assessment.go`
**Tests:** `internal/game_state/combat_chest_assessment_test.go`, `internal/game_state/dungeon_chest_assessment_test.go`

### CMD-0008

**Blind Jimmy, actual trap state, and exact pile selection**

**Kind:** rule
**Uses:** CMD-0007, PROV-0004, DNG-0008
**Original:** Established
**Redux:** Intentional deviation

Original chest Jimmy permits blind attempts: safe chests waste a Key; failed
armed attempts waste a Key without triggering/removing the trap; successful
armed attempts retain the Key and disarm. PROV-0004 owns the physical checks.

| Redux selected chest | Outcome | Key cost / assessment |
| --- | --- | --- |
| Known safe/disarmed | Harmless already-safe refusal | None, no disarm roll; stays safe |
| Uncertain/uninspected and physically safe | Broken key | One; knowledge unchanged |
| Armed, failed check | Broken key | One; knowledge unchanged |
| Armed, successful check | Disarmed | None; certify safe |
| No Keys and not known safe | Refusal | None, no disarm roll; unchanged |

**REDUX DEVIATION:** Known-safe refusal avoids original pointless key loss but
retains command time. Broken-key results deliberately disclose nothing; an
uncertain safe chest can waste Keys indefinitely. No pity result, rising certainty,
or consumed future first Search is added. Armed checks retain strict greater-than
success and the original difficulty from PROV-0004.

Open always tests the actual trap bit; no separate chest lock exists. Surface/
combat magical disarm clears authoritative object state and certifies that exact
chest safe. Dungeon An Sanct opens/disarms current-or-ahead closed chest and
retires closed-chest knowledge. In overlapping combat piles, Search/Jimmy/
An Sanct select the last matching slot; Open selects the first and checks that
slot's actual state. This is not a redesigned pile selection policy.
Redux's successful combat Jimmy result and Open result carry the selected chest's
detached lifetime identity (COMBAT-0010). Open retains it after removal so its
contents summary can finalize the correct chest lifetime without confusing another
chest at the same square or an identical replacement using the same original slot.
Actual contents-modal admission clears all lingering scene-owned action/Search/Look
cards under the shared UI policy. Earlier cancellation and later finalization remain
chest-scoped; they do not purge unrelated newer feedback. Popup capacity zero can
leave a valid successful Jimmy result with no card handles. History and the current
opening's trap notice remain independently owned.

**Implementation:** `internal/game_state/action_jimmy.go`, `internal/game_state/action_open.go`
**Tests:** `internal/game_state/combat_chest_assessment_test.go`

### CMD-0009

**Lord British's basement daily identities and ground loot**

**Kind:** rule
**Uses:** KARMA-0005, CMD-0007, EQUIP-0001, RNG-0001
**Original:** Incomplete
**Redux:** Intentional deviation

The baseline local-data audit identifies `CASTLE.NPC` records 23–25, floor -1,
at `(16,21)`, `(17,22)`, `(13,23)`. Record 25 schedule begins at offset `0x190`;
X bytes `0x193..0x195` are 13. This is not a Redux relocation. Those authored
binary bytes were not independently reopened here. Original town initialization
recreates authored chests at value 30 on floor return/fresh entry: immediate
replenishment, not daily. Open consumes runtime chest and generates ground loot,
with KARMA-0005's town cost; save/resume of existing town objects does not itself
refill the chest.

**REDUX DEVIATION:** Only those three verified authored identities replenish at
most once per calendar day, on real basement entry, discouraging stair-loop
farming. Identity includes source reference/schedule, not merely sprite, number
or current position. Each records an optional full year/month/day of committed
Open before command time advances, even for empty results or uncollected loot.
Same-day return does not replenish; later dates permit one absent replacement,
without accumulating missed days. Reversed dates cannot bypass the allowance.
Midnight in place, ordinary refresh, bed/NPC reset, combat return and hydration
are not entry triggers.

Survivors retain physical state and assessment. Replacement checks terrain,
Avatar, occupants, loose piles and capacity; obstruction waits for another eligible
entry without consuming allowance or RNG. A new lifetime is safe, value 30 and
uninspected. Children have independent identities/value/knowledge, never the
parent's daily history. Town Open still charges its ordinary karma, not Get again.

Every chest object opened on a town, surface or combat map, including a monster's
([COMBAT-0009](COMBAT.md#combat-0009)), generates loot from its actual
low-seven-bit value; first-person dungeon chests use
[DNG-0005](DUNGEON_SYSTEM.md#dng-0005) instead. Miscellaneous categories are considered
in reverse order of the following table. Skip without a probability draw when
threshold exceeds value; otherwise draw `1..30` and accept when it is at least
threshold. Accepted categories draw their quantity/type; chest and gold make
replacement draws after their initial quantity draw.

| Category | Threshold | Result / initial draw |
| --- | ---: | --- |
| Child chest | 25 | Initial 1–10 discarded; child value 1–parent value |
| Gold | 3 | Initial 1–90 discarded; replacement 1–3×value |
| Potion | 17 | Type 1–8, one item |
| Scroll | 17 | Type 1–8, one item |
| Keys | 9 | 1–2 |
| Gems | 15 | 1–2 |
| Torches | 7 | 1–2 |
| Food | 7 | 1–2 |

Armaments make `floor(value/2)+1` attempts (16 at value 30), each selecting an
EQUIP-0001 ID from `0..47` then applying that item's value/threshold/`1..30`
tests. The seven `0xFF` exclusions never pass.

| Threshold | Equipment |
| ---: | --- |
| 5 | Dagger |
| 10 | Leather Helm, Chain Coif, Small Shield, Sling, Club, Flaming Oil, Main Gauche, Spear, Throwing Axe, Short Sword, Arrows, Quarrels |
| 15 | Iron Helm, Large Shield, Cloth Armour, Leather Armour, Mace, Morning Star, Bow, Crossbow, Spiked Collar |
| 20 | Spiked Helm, Spiked Shield, Ring Mail, Scale Mail, Chain Mail, Long Sword, Two-Handed Hammer, Two-Handed Axe, Two-Handed Sword, Halberd |
| 23 | Magic Bow, Silver Sword, Magic Axe, Ring of Invisibility, Ring of Protection, Ring of Regeneration, Amulet of Turning |
| 24 | Plate Mail |
| 28 | Magic Shield |
| 0xFF | Jewel Shield, Mystic Armour, Sword of Chaos, Glass Sword, Jeweled Sword, Mystic Sword, Ankh |

Empty loot is valid, with no minimum
or retry-until-eligible. Children are unopened safe world objects requiring Open,
not carried items. Source short-circuit and replacement-draw order remains even
when later object admission lacks space; bounds do not prove original uniformity.

**REDUX DEVIATION:** A bounded adapter counts active-floor visible units and
unmerged loose entries against 31 usable slots across existing storage authorities,
rather than the original shared object array. It preserves pickup order. Open
reveals ground contents; Get transfers subject to inventory limits. Children
and loose loot are discarded on real floor/location departure; active saved
objects continue. Daily history survives outside the castle. Other chest, fixed
Search and skull-key policies are not implicitly made daily.

**Implementation:** `internal/game_state/basement_chests.go`, `internal/references/basement_chests.go`, `internal/references/chest_loot.go`, `internal/map_units/transient_surface_chest.go`
**Tests:** `internal/game_state/basement_chests_test.go`
