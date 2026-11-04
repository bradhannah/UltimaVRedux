# Tile identity and procedural properties

Tile bytes have contextual meanings. Terrain, objects, combat descriptors, and
graphics selectors must not be collapsed into one universal property record.
The following is a DOS behavior reference; unknown art and unlabelled variants
remain explicit rather than acquiring invented names.

### TILE-0001

**Logical indexes and contextual families**

**Kind:** rule
**Uses:** none
**Original:** Established
**Redux:** Unverified

Terrain and object identities are byte-sized, with different contextual
namespaces. Logical terrain frames have a 256-entry identity mapping initially;
normal tactical spacing/highlights use 16 x 16 pixels. Some graphics operations
add 256 to select an object image, not another byte-sized terrain namespace.
FF hex is a gameplay sentinel, while zero also has display-control uses.

Creature families use `0x40 + 4*creature_index`; low bits select frames/directions.
Object identities include chest 01, gold 02, potion 03, scroll 04, equipment
categories 05-0C, torch 0D, box 0E, food 0F, horses 10-13, carpet rider 14,
party 1C, ships 20-27, skiff 28, pirate ship 2C, and humanoids from 40.
Fields E8-EB in object/room descriptors are distinct from terrain E8's hourglass.

The physical layout of `TILES.16` is given in
[GFX-0008](GRAPHICS_FORMATS.md#gfx-0008); this logical space alone does not
establish it. Tile zero's tactical hit-effect manipulation is not fully specified
by its map-legend meaning.

### TILE-0002

**Terrain-family reference**

**Kind:** rule
**Uses:** TILE-0001
**Original:** Established
**Redux:** Unverified

All values in this table are hexadecimal; gaps remain unnamed.

| Range | Meaning |
| --- | --- |
| 01-03 | Deep water, sea, shoals |
| 04-0F | Swamp, grass, brush, desert, thick brush, forest, tropical vegetation, hills/mountains/peaks/split hills |
| 10-1C | Hut, Codex, keep, village, town, castle, cave, mine, dungeon, shrine, ruined shrine, lighthouse, oasis |
| 1D-1F | Red bridge and waste halves |
| 20-26 | Road encounter family |
| 27 | Roof |
| 29-2F | Crystal, beacon, stump, plowed field, crops, fruit tree, cactus |
| 30-37 | Beaches and grass-water coasts |
| 38-3E | Gargoyle, evil entrance, large-castle pieces, castle entrance |
| 40-4F | Planks, gangplank, mast, rail, floors, pillar, pier, bridges, slit/window, rocks, secret door, wall |
| 50-5F | Castle walls and furnishings/guardians |
| 60-6F | River family, including bridge 6A-6B and headwaters 6C |
| 80-8F | Pendulum, restraints, grate, arch, grave/torture/instrument pieces, lava |
| 90-9F | Chairs, tables, magic doors, portcullis, food, mirrors |
| A0-AF | Signs, well, furniture, carpet, bed, storage furniture |
| B0-BF | Torches/flame/meat, cannons, doors, fireplace/lamp/candelabrum/stove |
| C4-CB | Stairs, ladders, fences |
| D4-D7, D8-DB | Waterfall and fountain frames |
| DC-DF | Moongate, fake desert, blue flame, sealed dungeon |
| E0-E7 | Sign/flagpole and water-corner families |
| E8 | Hourglass |
| EC-EF | Flag frames |
| F0-F9 | Merchant/service/wall/ship signs |
| FA-FD | Clock and bellows pairs |
| FE, FF | White wall, nothing sentinel |

### TILE-0003

**Walking bitmap and movement dispatch**

**Kind:** rule
**Uses:** TILE-0001, TILE-0002
**Original:** Established
**Redux:** Unverified

Walking uses a separate 256-bit prohibited-terrain table, MSB first within each
byte. Its blocked indexes, represented as ranges rather than program data, are:

```text
01-03, 0C-0D, 1A, 1C, 27-2B, 2E-2F, 38, 3A-3D, 3F,
41-43, 46, 4A-69, 6C-85, 88-8B, 8D-8E, 94-A9, AD-BB,
BD-C3, CA-DB, DE-F8, FA-FE
```

Chairs have an additional mover-identity exception: only mover values 1C-1D and
40-4F bypass the chair restriction, not every loosely defined NPC/vehicle.

Movement classes select foot; water; water-or-foot flying; horse foot excluding
lava/swamp; non-water ether movement; directional skiff; deep/sea frigate; or
exact swamp, grass, deep-water, and desert specialists. Water means a terrain
value below swamp or any river-family value. This numerically includes zero;
do not silently replace it with a narrower named set.

Skiff coast masks for the four grass-water variants are 6,3,9,12. River masks
in variant order are 10,5,3,9,12,6,11,13,14,7,10,5,8,4,2,1. Test the mask against
`8 >> facing`. Frigates require terrain at or below Sea; they do not inherit the
skiff river rule. Force fields reject movement. Wisp's ordinary terrain class is
on-foot despite its separate teleport ability.

### TILE-0004

**Projectile passage is a separate table**

**Kind:** rule
**Uses:** TILE-0001
**Original:** Established
**Redux:** Unverified

The projectile table is MSB-first, but a set bit means passage is allowed.
Blocked indexes are:

```text
0C-0D, 12-15, 19-1B, 3A-3F, 42, 46, 4D-55, 5A, 70-7F, B8-B9, DF
```

This test belongs to its projectile consumer and composed display context;
walking legality and generic line of sight are not substitutes for it.

### TILE-0005

**Occupancy and mutable door terrain**

**Kind:** rule
**Uses:** TILE-0003, CBT-0004
**Original:** Established
**Redux:** Unverified

Terrain admission is only one collision layer. Outdoor/town movement also checks
objects and vehicle exceptions. Tactical checks combine terrain, objects, and
combatants with ordered branches: Protection fields block, other ordinary fields
and corpses have exceptions, and off-screen movement can be admitted for escape.
Do not generalize malformed object/queue behavior into independent perfect scans.

Opening an ordinary or windowed door stores its old type/coordinates, replaces
terrain with floor, and sets a four-turn counter. The reviewed closing routine
restores it in small-map contexts without an occupant-safety gate. These are
mutations of effective terrain, not an immutable abstract door property.

### TILE-0006

**Scheduled portcullises**

**Kind:** rule
**Uses:** TILE-0005
**Original:** Established
**Redux:** Intentional deviation

The original selects an arch and XOR-toggles the cell immediately south. Ordinary
town turns trigger this on entering hour 5 or 20; floor loading applies the
nighttime transform below 5 or above 19. Standalone portcullises are not selected
merely because of their tile. The loop has no occupancy-safety check and does not
inherit the separate drawbridge safeguards. Different sleep loops can miss or
repeat transitions.

**REDUX DEVIATION: TILE-0006** — Redux derives the approved 05:00-20:00 open state
when reading an eligible arch/floor pair, including after sleep and time edits.
This gives predictable gates without replaying inconsistent toggles. Overrides
remain authoritative; malformed/out-of-bounds pairs are left alone. Terrain and
actor occupancy stay distinct, so a closing gate does not displace its occupant.

**Implementation:** `internal/map_state/small_map_terrain.go`
**Tests:** `internal/game_state/portcullis_actions_test.go`

### TILE-0007

**Logical and object animation**

**Kind:** rule
**Uses:** TILE-0001
**Original:** Incomplete
**Redux:** Unverified

Logical animation changes display mappings rather than terrain. Waterfalls and
fountains advance each call; pendulum/flag changes occur on odd counter values;
clock/bellows changes occur when both low counter bits are set. The counter
advances once per call. Moongate is the exclusive end of the fountain family,
not another fountain frame. Call cadence is not a millisecond specification.

Object animation packs a program index in the high nibble and skip state in the
low nibble. Values 1-14 count down; 15 does not. Frame commands select one of four
family frames and end that update; reset/jump can continue interpretation in the
same call. Conditional base-frame reset gives ordinary objects six skipped updates
but advances the banjo-player program. Another command conditionally resets the
program; delay commands install a low-nibble skip value.

Most eligible actors have a preliminary random skip test; banjo player and Mimic
bypass it. Eleven programs are defined despite a conflicting ten-row declaration.
Exact generator distribution, some excluded/unnamed families, and driver-side
animation remain unverified. Original animation can consume gameplay RNG.

### TILE-0008

**Presentation substitutions and property limits**

**Kind:** rule
**Uses:** TILE-0001, TILE-0002
**Original:** Incomplete
**Redux:** Unverified

Oasis/fake-desert display substitution compares squared viewport distance with
5, not a five-tile radius. The reviewed original gate is noncombat, not an explicit
surface-level test. Arch cells suppress normal actor composition; this does not
establish a modern foreground-mask algorithm.

Redux's presentation metadata, decorative ground variants, tints, animation
caches, and foreground drawing remain separate from collision and persistence.
Specific authored placements and complete light/sight/speed/building properties
were not re-certified in this conversion. There is no established universal
on-disk per-tile Boolean property structure; each property needs its own consumer
evidence, and typed Redux fields are an interpretation of those contracts.
