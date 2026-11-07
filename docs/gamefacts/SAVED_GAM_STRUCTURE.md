# Legacy save format

This describes the DOS `SAVED.GAM` block and companion `SAVED.OOL` object lists.
Offsets are hexadecimal and sizes decimal. Multibyte quantities are little-endian.
Allocation boundaries are substantially established; not every saved working field
has a complete portable interpretation or a verified malformed-input contract.
Redux native snapshots use a separate schema and must not use these offsets.

### SAVE-0001

**File boundary and roster addressing**

**Kind:** rule
**Uses:** none
**Original:** Established
**Redux:** Matches

The standard saved block is 4,192 bytes (`0x1060`), covering offsets `0x0000`
through `0x105F`. The initial two bytes are an allocated boundary field, not a
verified magic signature. The excluded ending boundary begins at `0x1060`.
This establishes the written block length, not every historical loader's response
to truncation or trailing bytes.

Sixteen character records begin at `0x0002`, each 32 bytes:

```text
character_offset(index) = 0x0002 + index * 32       # index 0..15
```

Redux's decoder requires the standard block length. Normal numeric values are
interpreted by field; little-endian storage does not make every field unsigned.

**Implementation:** `internal/legacy_save/types.go`, `internal/legacy_save/decode.go`
**Tests:** `internal/legacy_save/decode_test.go`

### SAVE-0002

**Character record and party membership**

**Kind:** rule
**Uses:** SAVE-0001
**Original:** Established
**Redux:** Matches

| Relative offset | Bytes | Meaning |
| --- | ---: | --- |
| 00 | 9 | Name storage |
| 09 | 1 | Sex |
| 0A | 1 | Class |
| 0B | 1 | Status |
| 0C, 0D, 0E | 1 each | Strength, Dexterity, Intelligence |
| 0F | 1 | Magic points |
| 10, 12, 14 | 2 each | Current HP, maximum HP, experience |
| 16 | 1 | Level |
| 17 | 1 | Inn time |
| 18 | 1 | Stored armor value |
| 19, 1A | 1 each | Helmet, body armor |
| 1B, 1C | 1 each | First and second hand |
| 1D, 1E | 1 each | Ring and neck accessory |
| 1F | 1 | Party/location marker |

The three two-byte character quantities are signed 16-bit fields in the reviewed
layout; normal HP and experience are nonnegative. Negative/corrupt interpretation
is not inferred here. Empty equipment is `0xFF`; item zero remains valid.

Active members occupy a packed prefix. Recruitment marks a companion active and
swaps its record into that prefix. Inn separation records the location and moves
the character out of the prefix. Common marker values are zero for active,
a small-map location for inn residence, `0x7F` for execution, and `0xFF` for
unrecruited. The stored armor byte is real data, not padding; its complete
relationship to effective protection is a separate equipment question.

**Implementation:** `internal/legacy_save/decode.go`, `internal/legacy_save/types.go`
**Tests:** `internal/legacy_save/decode_test.go`

### SAVE-0003

**Inventory and party allocation**

**Kind:** rule
**Uses:** SAVE-0001, SAVE-0002
**Original:** Established
**Redux:** Matches

| Offset | Bytes | Meaning |
| --- | ---: | --- |
| 0202, 0204 | 2 each | Food, gold |
| 0206 | 1 | Keys |
| 0207 | 1 | Gems |
| 0208 | 1 | Torches |
| 0209 | 1 | Grapples |
| 020A | 1 | Carpets |
| 020B | 1 | Skull Keys |
| 020C | 1 | Skull Key search/reveal day |
| 020D, 020E, 020F | 1 each | Amulet, Crown, Sceptre possession |
| 0210 | 4 | Shard allocation; only three named counters established |
| 0214, 0215, 0216 | 1 each | Spyglass, plans, Sextant |
| 0217, 0218, 0219 | 1 each | Watch, Badge, wooden box |
| 021A | 48 | Unequipped equipment quantities |
| 024A | 48 | Prepared spell quantities |
| 027A, 0282 | 8 each | Scrolls, potions |
| 028A, 0292, 029A, 02A2 | 8 each | Moonstone X, Y, map, level |
| 02AA | 8 | Reagent quantities |
| 02B2 | 3 | Reagent collection days |
| 02B5 | 1 | Active party size |
| 02B6 | 24 | Hidden-object discovery bits |

Food and gold are signed 16-bit storage fields in this layout. Normal mutation
limits are gameplay rules, not consequences of their storage widths. The fourth
shard byte's meaning remains unresolved; preserve it rather than inventing a
fourth named shard or assuming it can never be addressed.

This is the canonical inventory offset table; other topics should refer here.
The day marker is committed by successful reveal, not later pickup; see
[Skull Key commitment](PROVISIONS_CONSUMPTION.md#prov-0006).

**Implementation:** `internal/legacy_save/decode.go`, `internal/legacy_save/types.go`
**Tests:** `internal/legacy_save/decode_test.go`

### SAVE-0004

**Calendar, action, and position allocation**

**Kind:** rule
**Uses:** SAVE-0001, KARMA-0001, KARMA-0012
**Original:** Incomplete
**Redux:** Unverified

| Offset | Bytes | Meaning or retained working-state role |
| --- | ---: | --- |
| 02CE | 2 | Year |
| 02D0, 02D2 | 2 each | Temporary X, Y |
| 02D4 | 1 | Shared global active effect/item slot |
| 02D5, 02D6 | 1 each | Active player, party display shape |
| 02D7, 02D8, 02D9 | 1 each | Month, day, hour |
| 02DA, 02DB | 1 each | Previous hour, minute |
| 02DC, 02DD, 02DE | 1 each | Combat cadence, update counter, gong state |
| 02DF, 02E0, 02E1 | 1 each | Moon states and gate stage |
| 02E2 | 1 | Karma |
| 02E3, 02E4 | 1 each | Eating/food-update state |
| 02E5 | 1 | Beggar reward counter |
| 02E6, 02E7 | 1 each | Last-camping and Wanderer markers |
| 02E8 | 1 | Global effect duration |
| 02E9, 02EA | 1 each | Current-action spell and magic flags |
| 02EB, 02EC | 1 each | Motion and wind state |
| 02ED, 02EE, 02EF | 1 each | Current map, prior map, level |
| 02F0, 02F1 | 1 each | Party X, Y |
| 02F2, 02F3, 02F4 | 1 each | Crosshair enable/X/Y |
| 02F5, 02F6 | 1 each | Display offsets |
| 02F7 | 1 | Current weapon |
| 02F8, 02F9, 02FA | 1 each | Turn, turn-light, exit direction |
| 02FB, 02FC, 02FD | 1 each | Scenario, wound type, victory |
| 02FE, 02FF | 1 each | Update flag, sight |
| 0300, 0301 | 1 each | Magic-light and torch-light counters |

The offsets are established. Their complete reload semantics are not: a contiguous
memory save can contain temporary process state as well as durable gameplay.
Do not infer native snapshot fields from these names or silently discard unknown
bytes when preservation is required.

**Implementation:** `internal/legacy_save/decode.go`, `internal/legacy_save/types.go`
**Tests:** `internal/legacy_save/decode_test.go`

### SAVE-0005

**World, dungeon, and NPC allocation**

**Kind:** rule
**Uses:** SAVE-0001
**Original:** Incomplete
**Redux:** Unverified

| Offset | Bytes | Meaning or retained region |
| --- | ---: | --- |
| 0302 | 32 | Enemy state |
| 0322, 0325 | 3, 1 | Shadowlord state and last-call marker |
| 0326, 0328 | 2 each | Active and completed shrine quest bits |
| 032A, 0332 | 8 each | Dungeon entrances, shrine closure state |
| 033A | 14 | Dungeon room state |
| 0348, 0368, 0388 | 32 each | Drawbridge X, Y, saved terrain |
| 03A8 | 1 | Drawbridge count/state |
| 03A9, 03AA, 03AB, 03AC | 1 each | Door type, X, Y, closing counter |
| 03AD, 03AE, 03AF | 1 each | Ship X, Y, sailing state |
| 03B0, 03B1, 03B2, 03B3 | 1 each | Prompt, intoxication, Shadowlord, opening-encounter flag |
| 03B4 | 512 | Dungeon cells: 8 x 8 x 8 bytes |
| 05B4, 0634 | 128 each | NPC absence and met-party data: 32 four-byte entries each |
| 06B4 | 256 | Active object list: 32 eight-byte entries |
| 07B4, 07B6 | 2 each | Retained coordinate working values |
| 07B8 | 512 | 32 schedules, 16 bytes each |
| 09B8 | 512 | 32 NPC statuses, 16 bytes each |
| 0BB8 | 1024 | 32 paths, 32 bytes each |
| 0FB8 | 64 | 32 signed 16-bit path indexes |
| 0FF8 | 32 | NPC shape bytes |
| 1018, 1019 | 1 each | Pending NPC command and actor |
| 101A | 2 | Current level working value |
| 101C | 64 | 32 signed 16-bit pause values |
| 105C, 105D, 105E, 105F | 1 each | Last direction, facing, dungeon type, skiff count |

Each schedule has three action bytes, three X, three Y, three level, and four
time bytes. Each NPC status has eight 16-bit values, including an explicit
object-list association. Path indexes are signed integer indexes, not saved
machine pointers; the pause region is 64 bytes, not 64 integer elements.
The complete interpretation of every bit and transient status remains incomplete.

**Implementation:** `internal/legacy_save/decode.go`, `internal/legacy_save/types.go`
**Tests:** `internal/legacy_save/decode_test.go`

### SAVE-0006

**Object records and the companion object-list file**

**Kind:** rule
**Uses:** SAVE-0005
**Original:** Established
**Redux:** Matches

An object record's eight bytes are base identity, displayed shape, X, Y, level,
contextual number, animation state, and miscellaneous state. Identity and display
frame are distinct. The contextual number can represent contents, quantity,
durability, or another category-specific value; it is not universally HP.
Outside combat, slot zero represents the party, including a boarded vehicle.
Ordinary NPC association is stored separately in NPC status data.

`SAVED.OOL` contains two 256-byte lists: Britannia, then Underworld. When one large
world is active, its current list in `SAVED.GAM` supersedes the potentially stale
matching companion half. The other half remains the inactive world's list. Town
and dungeon saves retain both halves. Redux requires the exact 512-byte companion
for player-facing legacy import and explicitly resolves this ownership boundary.

**Implementation:** `internal/legacy_save/read.go`, `internal/legacy_save/decode.go`, `internal/legacy_save/types.go`
**Tests:** `internal/legacy_save/read_test.go`, `internal/legacy_save/decode_test.go`

### SAVE-0007

**Effect slots and initialization boundaries**

**Kind:** rule
**Uses:** SAVE-0004, KARMA-0002
**Original:** Established
**Redux:** Unverified

The shared global effect/item slot can hold Protection, Negate Magic, Time Stop,
Quickness, charm, or the Amulet/Crown/Badge item codes. Duration 255 is exempt from
ordinary countdown. Magic-light and torch-light counters are separate. Action
spell/magic flags are not extra global active-effect slots. This is not a claim
that every persistent magical condition uses one slot.

Character creation loads `INIT.GAM`, changes selected Avatar fields, and writes
the standard block. Allocation alone does not establish a default value for every
field or every shipped template. Redux's native state deliberately has its own
semantic schema; only explicitly converted legacy fields become authoritative,
with preservation/default warnings handled by the import boundary.

**Implementation:** `internal/legacy_save/decode.go`, `internal/legacy_save/conversion.go`
**Tests:** `internal/legacy_save/decode_test.go`, `internal/legacy_save/conversion_test.go`
