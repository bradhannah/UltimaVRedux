# Map files and locations

These rules describe how the MS-DOS release stores its maps: the two 256 x 256
worlds, the settlement floor files, the location numbers that tie them together,
entrance coordinates, and four fixed scene maps. Tile values are logical tile
indexes ([TILE-0001](TILES.md#tile-0001)). Map contents remain user-supplied data
and are not reproduced here.

### MAP-0001

**World maps and chunk index**

**Kind:** rule
**Uses:** TILE-0001
**Original:** Established
**Redux:** Matches

`BRIT.DAT` (Britannia) and `UNDER.DAT` (the Underworld) each describe a 256 x 256
grid with one logical tile byte per position. The grid is divided into 256 chunks
of 16 x 16 tiles. Chunks are numbered row by row across the world, and tiles row by
row within a chunk:

```text
chunk = (y >> 4) * 16 + (x >> 4)
tile  = (y & 15) * 16 + (x & 15)
```

`UNDER.DAT` stores all 256 chunks in chunk order: 65,536 bytes. `BRIT.DAT` omits
the 51 chunks that are entirely deep water and stores the other 205 in ascending
chunk order: 52,480 bytes. A 256-byte chunk index gives, for each chunk number,
the chunk's record in `BRIT.DAT`, or `FF` hex for an omitted chunk. The loader
fills an omitted chunk with Deep Water, tile 1. The index is compiled into the
program and also stored in `DATA.OVL` at `3886` hex
([OVL-0002](DATA_OVL_LAYOUT.md#ovl-0002)). Its present entries ascend from 0 to
204, so reading the present chunks in order is equivalent to following the index.

Both worlds wrap at their edges. Closed dungeon entrances and destroyed shrines
are substituted while chunks load; neither file stores those states.

Redux reads the index from `DATA.OVL`, fills omitted chunks with tile 1, and reads
`BRIT.DAT` in chunk order. This match covers file decoding; the runtime
substitutions belong to the dungeon and shrine rules.

**Implementation:** `internal/references/large_map.go`
**Tests:** `internal/references/large_map_test.go`

### MAP-0002

**Settlement floor files**

**Kind:** rule
**Uses:** MAP-0003
**Original:** Established
**Redux:** Matches

`TOWNE.DAT`, `DWELLING.DAT`, `CASTLE.DAT`, and `KEEP.DAT` are each 16,384 bytes:
sixteen floor records of 1,024 bytes. A floor is a 32 x 32 grid of logical tile
bytes, stored row by row. Location numbers 1-32 select the file in groups of eight
([MAP-0003](#map-0003)).

A 32-entry table gives the record of each location's ground floor (level 0). The
record loaded is that entry plus the signed level, so a basement (level -1) is the
record before the ground floor and upper floors follow it. Floor counts are not
stored; they follow from the gap to the next location's entry. The table is
compiled into the program and also stored in `DATA.OVL` at `1E2A` hex.

| Locations | File | Ground-floor records | Floors |
| --- | --- | --- | --- |
| 1-8: Moonglow, Britain, Jhelom, Yew, Minoc, Trinsic, Skara Brae, New Magincia | `TOWNE.DAT` | 0, 2, 4, 7, 8, 10, 12, 14 | Two each. Yew has a basement and a ground floor; the others have a ground and a first floor. |
| 9-12: Fogsbane, Stormcrow, Greyhaven, Waveguide | `DWELLING.DAT` | 0, 3, 6, 9 | Three each |
| 13-16: Iolo's Hut, Sutek's Hut, Sin Vraal's Hut, Grendel's Hut | `DWELLING.DAT` | 12, 13, 14, 15 | One each |
| 17-18: Lord British's Castle, Palace of Blackthorn | `CASTLE.DAT` | 1, 6 | Five each: a basement and ground to third floors |
| 19-24: West Britanny, North Britanny, East Britanny, Paws, Cove, Buccaneer's Den | `CASTLE.DAT` | 10, 11, 12, 13, 14, 15 | One each |
| 25-26: Ararat, Bordermarch | `KEEP.DAT` | 0, 2 | Two each |
| 27-29: Farthing, Windemere, Stonegate | `KEEP.DAT` | 4, 5, 6 | One each |
| 30-31: The Lycaeum, Empath Abbey | `KEEP.DAT` | 7, 10 | Three each |
| 32: Serpent's Hold | `KEEP.DAT` | 14 | Three: a basement, a ground and a first floor |

The ordinary entry path places the party on level 0 at (15,30). Stairs and ladders
change the level by one.

Redux builds its floor tables from the same records.

**Implementation:** `internal/references/small_map.go`, `internal/references/location_references.go`, `internal/references/small_location_reference.go`

### MAP-0003

**Location numbers**

**Kind:** rule
**Uses:** none
**Original:** Established
**Redux:** Matches

The program identifies every map by a location number:

| Number | Locations | Stored in |
| --- | --- | --- |
| 0 | Britannia at level 0, or the Underworld at level `FF` | `BRIT.DAT`, `UNDER.DAT` |
| 1-8 | Moonglow, Britain, Jhelom, Yew, Minoc, Trinsic, Skara Brae, New Magincia | `TOWNE.DAT` |
| 9-16 | Fogsbane, Stormcrow, Greyhaven, Waveguide, Iolo's Hut, Sutek's Hut, Sin Vraal's Hut, Grendel's Hut | `DWELLING.DAT` |
| 17-24 | Lord British's Castle, Palace of Blackthorn, West Britanny, North Britanny, East Britanny, Paws, Cove, Buccaneer's Den | `CASTLE.DAT` |
| 25-32 | Ararat, Bordermarch, Farthing, Windemere, Stonegate, The Lycaeum, Empath Abbey, Serpent's Hold | `KEEP.DAT` |
| 33-40 | Deceit, Despise, Destard, Wrong, Covetous, Shame, Hythloth, Doom | `DUNGEON.DAT`, record = number - 33 |
| `FF` | Combat and other special screens | none |

Saved state uses the same numbers, for example the party's current map
([SAVE-0004](SAVED_GAM_STRUCTURE.md#save-0004)) and a companion's inn
([SAVE-0002](SAVED_GAM_STRUCTURE.md#save-0002)). The values `41` and `42` hex
appear while Return to the View runs; their full meaning is not established.

Redux uses the same numbers for 0-40. It reserves further values for its own
modes, such as camping, combat, and the finale; those are Redux identifiers, not
original data.

**Implementation:** `internal/references/locations.go`

### MAP-0004

**World entrances**

**Kind:** rule
**Uses:** MAP-0003
**Original:** Established
**Redux:** Matches

Two 40-byte tables give the world X and Y of every entrance, indexed by location
number minus one: entries 0-31 are settlements and 32-39 are dungeons. They are
compiled into the program and also stored in `DATA.OVL` at `1E9A` hex (X) and
`1EC2` hex (Y). Entering matches the party's coordinates against the tables; the
tile at each coordinate is the visible entrance. Leaving places the party back at
the same coordinate. For a dungeon, the floor it is left by picks the world: the
top floor returns to Britannia and the bottom floor to the Underworld
([DNG-0003](DUNGEON_SYSTEM.md#dng-0003)).

| Number | Location | X,Y | Number | Location | X,Y |
| ---: | --- | --- | ---: | --- | --- |
| 1 | Moonglow | 232,135 | 17 | Lord British's Castle | 86,107 |
| 2 | Britain | 81,106 | 18 | Palace of Blackthorn | 196,245 |
| 3 | Jhelom | 36,222 | 19 | West Britanny | 84,106 |
| 4 | Yew | 58,43 | 20 | North Britanny | 86,105 |
| 5 | Minoc | 159,20 | 21 | East Britanny | 88,106 |
| 6 | Trinsic | 106,184 | 22 | Paws | 98,145 |
| 7 | Skara Brae | 22,128 | 23 | Cove | 136,90 |
| 8 | New Magincia | 187,169 | 24 | Buccaneer's Den | 136,158 |
| 9 | Fogsbane | 88,120 | 25 | Ararat | 49,58 |
| 10 | Stormcrow | 152,24 | 26 | Bordermarch | 15,160 |
| 11 | Greyhaven | 104,216 | 27 | Farthing | 64,240 |
| 12 | Waveguide | 216,120 | 28 | Windemere | 248,8 |
| 13 | Iolo's Hut | 45,62 | 29 | Stonegate | 148,74 |
| 14 | Sutek's Hut | 176,208 | 30 | The Lycaeum | 218,107 |
| 15 | Sin Vraal's Hut | 201,59 | 31 | Empath Abbey | 28,50 |
| 16 | Grendel's Hut | 153,91 | 32 | Serpent's Hold | 146,241 |

Dungeon entrances are listed with their Words of Power in
[WORD-0001](WORDS_OF_POWER.md#word-0001); entries 32-39 agree with it.

Ararat is the only settlement whose entrance is in the Underworld. Doom's entrance
exists only in the Underworld. Each of the other seven dungeons has an entrance at
the same coordinate in both worlds.

Redux reads both tables from `DATA.OVL`.

**Implementation:** `internal/references/world_locations.go`

### MAP-0005

**Fixed scene maps**

**Kind:** rule
**Uses:** INTRO-0004
**Original:** Established
**Redux:** Unverified

The first 704 bytes of `MISCMAPS.DAT` hold four fixed scenes of 176 bytes each,
ahead of the Return to the View data in
[INTRO-0004](STARTUP_MENU_AND_INTRODUCTIONS.md#intro-0004). Each record is 11 rows
of 16 bytes. The first 11 bytes of a row are logical tiles and the last five are
zero, giving an 11 x 11 scene.

| Offset | Scene |
| ---: | --- |
| 0 | Blackthorn's interrogation chamber |
| 176 | A shrine interior |
| 352 | The Codex chamber |
| 528 | The endgame room |

Each scene is shown through the combat-screen path, location `FF`. How Redux
presents these scenes has not been compared with this layout.
