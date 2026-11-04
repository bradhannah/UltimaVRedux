# DATA.OVL layout

`DATA.OVL` is the original program's initialized data, stored as a separate file.
These rules give its conventions and an index of identified regions, so tables can
be found. Each region's meaning is defined by the topic named beside it. Names and
other text stored in the file are not reproduced here.

### OVL-0001

**File identity and conventions**

**Kind:** rule
**Uses:** none
**Original:** Established
**Redux:** Matches

The shipped `DATA.OVL` is 48,464 bytes. It is the main program's initialized data
segment, linked into a separate overlay file; the program's uninitialized data is
not stored. Copies of `DATA.OVL` that survive in the original source tree match
the shipped file only up to offset `28BE` hex, so offsets must be checked against
the shipped file.

- Offsets in these rules are file offsets, in hex.
- Multibyte values are little-endian.
- Strings are zero-terminated 8-bit ASCII with no length prefix.
- Coordinates are unsigned bytes held in separate X and Y tables.
- Two-dimensional tables are stored row by row.
- String pointer tables hold 16-bit data-segment offsets. Add `10` hex to a
  pointer to get its file offset; a zero pointer means no string.

Redux checks the shipped size and reads its tables at these file offsets.

**Implementation:** `internal/references/data_ovl.go`, `internal/originaldata/validation.go`
**Tests:** `internal/originaldata/validation_test.go`

### OVL-0002

**Identified regions**

**Kind:** rule
**Uses:** OVL-0001
**Original:** Incomplete
**Redux:** Unverified

These regions were located by matching the original source's initialized tables
against the shipped file:

| Offset | Bytes | Contents | Meaning |
| --- | ---: | --- | --- |
| `0010` | 66 | Runtime library notice (text) | none |
| `0052` | 4,964 | Name and label strings: weapons, creatures (singular and plural), items, armour, potions, reagents, spells, statuses, spell syllables, places, virtues, mantras, shops, and merchants. Place names start at `0A4D` (towns to Iolo's Hut), `0AC1` (castles and keeps), and `0B5E` (dungeons). | [EQUIP-0001](EQUIPMENT_ORIGINAL_DATA.md#equip-0001), [ENEMY-0001](ENEMIES.md#enemy-0001), [MAP-0003](MAP_FILE_FORMATS.md#map-0003) |
| `104C` | 590 | Conversation word list: 118 words in token order | [TALK-0002](TALK_SYSTEM_STRUCTURE.md#talk-0002) |
| `129A` | 284 | Names of the compressed files | [GFX-0001](GRAPHICS_FORMATS.md#gfx-0001) |
| `13CC` | 384 | Creature attributes, 48 x 8 bytes in ENEMY-0001's column order: STR, DEX, INT, armour, damage, HP, number appearing, treasure | [ENEMY-0001](ENEMIES.md#enemy-0001) |
| `154C` | 96 | Creature ability words, 48 x 16-bit | [ENEMY-0001](ENEMIES.md#enemy-0001) |
| `15AC`, `15DC` | 48 each | Creature range and missile type | [ENEMY-0001](ENEMIES.md#enemy-0001) |
| `160C`, `1644`, `1674`, `16AC` | 55, 48, 55, 55 | Equipment attack, defense, range, and attack type | [EQUIP-0001](EQUIPMENT_ORIGINAL_DATA.md#equip-0001), [EQUIP-0002](EQUIPMENT_ORIGINAL_DATA.md#equip-0002) |
| `1806` onward | various | String pointer tables (see OVL-0001): weapons `1806`, creatures `1866`, creature plurals `18C6`, items `1926`, armour `1972`, potions `19D2`, reagents `19E2`, spells `19F2`, places `1E4A`, virtues `1F5E`, mantras `1F6E`, shops `21DA`, merchants `22DA`, conversation words `24F8` (129 entries), file names `25FA` | as above |
| `1E2A` | 32 | Settlement ground-floor records | [MAP-0002](MAP_FILE_FORMATS.md#map-0002) |
| `1E9A`, `1EC2` | 40 each | Entrance X and Y | [MAP-0004](MAP_FILE_FORMATS.md#map-0004) |
| `1EEA` | 56 | Moon phases, 28 days x 2 | [MOON-0002](MOONSTONE_LOCATIONS.md#moon-0002) |
| `1F7E`, `1F86` | 8 each | Shrine X and Y | [SHRINE-0001](SHRINE_SYSTEM.md#shrine-0001) |
| `3886` | 256 | Britannia chunk index | [MAP-0001](MAP_FILE_FORMATS.md#map-0001) |
| `3A42`, `3A6A` | 40 each | Reagent prices and quantities | [MERCH-0005](MERCHANT_SERVICES.md#merch-0005) |
| `3A92` | 96 | Arms base prices, 48 x 16-bit | [EQUIP-0003](EQUIPMENT_ORIGINAL_DATA.md#equip-0003), [MERCH-0004](MERCHANT_SERVICES.md#merch-0004) |
| `3E88` | 6 x 113 | Hidden Search objects: tile, quantity or item, location, level, X, Y | [CMD-0007](COMMANDS_REFERENCE.md#cmd-0007) |
| `4D76`, `4D7E` | 8 each | Frigate and skiff prices, 4 x 16-bit | [MERCH-0008](MERCHANT_SERVICES.md#merch-0008) |
| `4D86`, `4D8A` | 4 each | Ship berth X and Y: Jhelom, Minoc, East Britanny, Buccaneer's Den | [MERCH-0008](MERCHANT_SERVICES.md#merch-0008) |
| `4D8E`, `4DD4`, `4E8A`, `4E90` | 6 each | Inn prices, rooms, and bed X and Y | [MERCH-0009](MERCHANT_SERVICES.md#merch-0009) |
| `6A1A` | 15,129 | Text used by the program's overlays | none |

Not yet identified: the first 12 bytes; two zero-filled ranges, `55B6`-`6A19` and
`A533`-`BD4F`; and gaps between tables, the largest roughly `2636`-`2756`,
`2777`-`2BD0`, `30FF`-`3666`, `3AF2`-`3D96`, `41E3`-`4B06`, and `4E96`-`54E4`. Some
of these hold data from libraries whose source is not in the reviewed tree.

Redux reads the place names, word list, file names, entrances, chunk index, and
ship berths at these offsets. It compiles many other tables into its own code;
those copies have not been compared byte for byte with this file.

**Implementation:** `internal/references/data_ovl.go`, `internal/references/world_locations.go`, `internal/references/docks.go`, `internal/references/large_map.go`
**Tests:** `internal/references/docks_test.go`, `internal/references/large_map_test.go`
