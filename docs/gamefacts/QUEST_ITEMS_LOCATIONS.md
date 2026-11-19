# Quest and special items

Original quantities, singleton bytes, operational bits, and discovery flags are
not one contiguous fourteen-item collection. Generator starts and direct placements
must remain distinct from uninspected authored map/NPC/TLK acquisition.

### QUEST-0001

**Storage and source-defined starting state**

**Kind:** rule
**Uses:** SAVE-0003, INV-0008
**Original:** Established
**Redux:** Unverified

Amulet, Crown, Sceptre, three named shards, Spyglass, Watch, Badge, Box, and Sextant
use singleton-style fields; ordinary grants store FF hex when held. Carpets and
Grapples are quantities. Plans mix presence and operational high-bit state. The
generator starts Watch held and these other listed items zero, not verified shipped
INIT.GAM bytes. Four shard bytes are allocated despite three named identities.
Redux IDs 0–13 order Amulet, Crown, Sceptre, three shards, Carpet, Grapple, Spyglass,
plans, Watch, Badge, Box, Sextant; that model is not the original save structure.

Identified Redux labels retain Amulet/Crown/Sceptre **of Lord British**, **Shard of
Falsehood/Hatred/Cowardice**, **Magic Carpet**, **Grapple**, **Spyglass**, **H.M.S.
Cape Plans**, **Pocket Watch**, **Black Badge**, **Sandalwood Box** and **Sextant**.
Magic Carpet and Sandalwood Box are supported by original longer labels. The
plans' punctuation is editorial. These labels do not rename stable typed IDs or
saved fields. Descriptive pickup prose may still say “a grappling hook” or “a
sandalwood box”; labels and authored discovery sentences are separate under INV-0003.

### QUEST-0002

**Fixed placements and acquisition limits**

**Kind:** rule
**Uses:** QUEST-0001, SHADOW-0001
**Original:** Incomplete
**Redux:** Unverified

On nonzero world level, original fixed objects are written at encoded level FF:

| Item | X,Y | Slot/object number, hex | Presence |
| --- | --- | --- | --- |
| Amulet of Lord British | 105,225 | 1C/F3 | Not owned |
| Shard of Falsehood | 192,80 | 1D/F0 | Not owned and identity 0 state below 128 |
| Shard of Hatred | 130,65 | 1E/F1 | Not owned and identity 1 state below 128 |
| Shard of Cowardice | 176,184 | 1F/F2 | Not owned and identity 2 state below 128 |

Pickup identifies Amulet by B7 hex and shards by B4 plus low-two object-number bits.
Other evidence is narrower: held Sceptre removes Stonegate NPC 9; Castle Britannia
carpet pickup removes NPC 22; Sandalwood Box pickup sets castle NPC-31 persistence. Crown
pickup records/removes its source NPC but does not identify its exact placement.
Talk grants quantities of Grapple/Carpet capped 99 or held Sextant/Spyglass/Badge;
special plans pickup writes FF. Granting conversations and exact remaining sites
require authored data.

Redux places Crown at Palace (15,13),floor 3 and Sceptre at Stonegate (15,15),floor
0. Its Amulet/shard coordinates match the table at Underworld floor -1. Do not
infer original Crown/Sceptre coordinates from that implementation table.

**Implementation:** `internal/game_state/quest_item_placement.go`

### QUEST-0003

**Artifact competition and Sceptre safety**

**Kind:** rule
**Uses:** QUEST-0001, SHADOW-0005, VEH-0004, SHIP-0002, ASTRO-0003, TIME-0005
**Original:** Established
**Redux:** Intentional deviation

Original Amulet/Crown/Badge share the active-effect slot with duration spells.
Reusing the active artifact clears slot/duration; another replaces both with
duration 255. No replaced effect resumes. Sceptre outside dungeon IDs scans the
centered 3×3 in X-major order, changing barrier tiles 70–7F hex to grass. If no
replacement occurs, it unconditionally calls field removal; that routine's
noncombat branch can index unrelated/out-of-range dungeon storage with surface
coordinates. Not every such coordinate necessarily lies outside storage.

**REDUX DEVIATION: QUEST-0003:** Redux keeps Badge/Amulet independent for concurrent
artifact use; Palace guards use Badge state. Crown competes with Protection,
Quickness, Confusion, Negate Magic, and Time Stop; recast refreshes, removal/expiry
never resumes a replaced effect. The original replaces silently; Redux reports
**Crown removed!** when a spell displaces the Crown and **<Spell> ended!** when
donning the Crown ends a spell. Independent native artifact fields remain; the
Amulet's own effect is QUEST-0004. Redux suppresses unsafe surface
Sceptre fallback and reports no effect; dungeon/combat removal stays context-owned.
Shard consumption is SHADOW-0005; Carpet, plans, instruments, Watch use their owning
rules. Box has no operative Use effect in the inspected original handler.

Owned Crown, Amulet, and Black Badge are actionable through combat Use: confirmed
wear/remove retains the artifact and completes exactly one current actor action.
Selecting the Crown preserves its duration-effect replacement rules and its
protection against enemy magical abilities, including acquisition of possession.
This is prevention, not a cure of an already active possession episode. Inventory,
context, and exact encounter/actor ownership are revalidated before activation;
stale or unavailable selections are free under CONSUME-0001.

**Implementation:** `internal/game_state/duration_effect.go`, `internal/game_state/action_use.go`, `cmd/ultimav/gamescene_use.go`
**Tests:** `internal/game_state/combat_artifact_use_test.go`, `cmd/ultimav/gamescene_combat_artifact_use_test.go`

### QUEST-0004

**Darkness and the Amulet of Lord British**

**Kind:** rule
**Uses:** QUEST-0003
**Original:** Established
**Redux:** Intentional deviation

The Underworld holds 106 darkness squares (terrain FF hex): a disc eleven squares
across, centred on the cave entrance at 128,128. The surface has none. At the start
of each outdoor turn the original tests the terrain under the party. On a darkness
square, unless the Amulet holds the active-effect slot, sight becomes zero and the
party is lost. With zero sight the view fill leaves every cell at the darkness
value, and no object standing on such a cell is drawn, the party's own icon
included, so the view is blank.

A direction command while lost still prints its direction and tests the target.
Darkness is legal ground on foot and the blank view makes every neighbour darkness,
so only an object can block, with the usual free "Blocked!". Otherwise the command
completes its turn, but the step itself is skipped: the party stays where it is.
No other command is gated. Once the Amulet is worn the test fails on the next turn,
and sight and movement return. A party without the Amulet cannot walk out.

Redux derives the same state from the party's large-map terrain and its independent
Amulet state (QUEST-0003). While lost, the map view is blank and a direction command
completes its turn without moving the party. Small maps, dungeons, combat and camping
are unaffected.

**REDUX DEVIATION: QUEST-0004:** the original gives no reason; Redux adds "Lost in
darkness!" so a blank view and a party that will not move do not read as a fault.
Redux charges the turn for every direction while lost, without the free "Blocked!"
for an object in the way, so bumping cannot be used to feel for neighbours. With the
Amulet worn, Redux keeps drawing the party on a darkness square, where the original
hides any object standing on one, so the player can see where the party stands. The
original's redraw timing between turns is not reproduced.

**Implementation:** `internal/game_state/toggle_item_state.go`, `cmd/ultimav/gamescene_movement.go`, `cmd/ultimav/gamescene_draw_effects.go`
**Tests:** `internal/game_state/toggle_item_state_test.go`, `cmd/ultimav/gamescene_surface_completion_test.go`
