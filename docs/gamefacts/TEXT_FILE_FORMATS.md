# Text files

These rules describe how three text files are stored and selected: Look
descriptions, merchant phrases, and the ending. They cover containers, encodings,
and selection, not the text itself, which remains user-supplied data.

### TEXT-0001

**Look descriptions**

**Kind:** rule
**Uses:** TILE-0001
**Original:** Established
**Redux:** Matches

`LOOK2.DAT` begins with 512 little-endian 16-bit offsets, one for each tile number
0-511. Each offset points into the same file. Entries 0-255 describe map terrain;
entries 256-511 describe object and creature shapes. Many entries share one
string, so animation frames and variants reuse a description. Strings are plain
ASCII terminated by zero, with no word tokens or control codes, and the file ends
with a `1A` hex end-of-file mark. Each string is a noun phrase that the game
places after its own Look wording. A one-character `*` entry marks a tile that is
never described through this file.

Look reads the top object on the square first and uses entry 256 plus its shape.
With no object, it uses the terrain entry. Signs bypass this file
([SIGN-0002](SIGNS_AND_FONTS.md#sign-0002)); telescopes, wells, and fountains have
their own handlers, and some features add context, such as the time on a clock.

In the shipped file the strings begin at offset 1,024, and the 512 entries use 216
distinct strings.

Redux reads the same offsets and looks at objects before terrain. Whether its
object index equals 256 plus the original object shape for every object has not
been checked separately.

**Implementation:** `internal/references/look.go`, `internal/game_state/action_look.go`

### TEXT-0002

**Merchant phrases**

**Kind:** rule
**Uses:** TALK-0002, MERCH-0001
**Original:** Established
**Redux:** Matches

`SHOPPE.DAT` is a sequence of zero-terminated strings with no index. The program
holds each phrase's byte offset in its own tables, generated from labels when the
file was built. Greetings and farewells are chosen from a table by shop type (arms,
tavern, horses, ships, reagents, guild, healer, inn), one of four variants at
random; other phrases are addressed directly by the shop code. The shipped file
holds 194 strings, and the last is followed by one extra zero.

Common words are stored as single bytes from `80` to `FE` hex that select words
from the list conversations use ([TALK-0002](TALK_SYSTEM_STRUCTURE.md#talk-0002)).
An expanded word gets a space on each side unless the neighbouring byte is another
word. Newline (`0A` hex) is the only other control byte. These characters are
replaced when a phrase is shown:

| Character | Replaced by |
| --- | --- |
| `#` | Shop name |
| `$` | Merchant's name |
| `@` | Morning before 12:00, afternoon before 18:00, otherwise evening |
| `%` | Price in gold |
| `^` | Quantity |
| `&` | Item name; in paid information, the informant's name |
| `*` | Town name, in paid information |

Redux reads the same offsets, word list, and replacement characters. It rebuilds
the spacing around expanded words rather than porting the original rule;
byte-exact spacing has not been compared.

**Implementation:** `internal/references/merchant_text.go`, `internal/references/tlk_compressed.go`
**Tests:** `internal/references/merchant_text_test.go`

### TEXT-0003

**Ending text**

**Kind:** rule
**Uses:** GFX-0004, DNG-0015, INTRO-0003
**Original:** Established
**Redux:** Matches

`END.DAT` holds six zero-terminated text blocks, one for each illustrated ending
screen, at offsets compiled into the program: 0, 424, 956, 1,530, 2,280, and 2,932.
Each block ends just before the next begins, and the last is followed by one extra
zero. The text is plain ASCII with no word tokens. It is set in the proportional
font ([GFX-0004](GRAPHICS_FORMATS.md#gfx-0004)) with a nine-pixel line height and
two marks: `{` leaves a fixed 15-pixel gap, and `_` marks a point where a word may
break, printing a hyphen only if the line breaks there. Newline forces a break.

| Screen | Offset | Picture |
| ---: | ---: | --- |
| 1 | 0 | `END1.16` record 0, with a title from `TEXT.16` |
| 2 | 424 | `END1.16` record 1 |
| 3 | 956 | `END1.16` record 2 |
| 4 | 1,530 | `END2.16` record 0, with a title from `TEXT.16` |
| 5 | 2,280 | `END2.16` record 1 |
| 6 | 2,932 | `END2.16` record 2 |

`ENDSC.16` follows the sixth screen. A separate file, `ENDMSG.DAT`, holds eleven
short zero-terminated strings used earlier in the finale
([DNG-0015](DUNGEON_SYSTEM.md#dng-0015)).

Redux reads the same six blocks and sets them with the story-text wrapper it
shares with the Introduction ([INTRO-0003](STARTUP_MENU_AND_INTRODUCTIONS.md#intro-0003)).
It applies the `{` gap, and when a line fills it backs up to the nearest space or
nearest `_` whose hyphen still fits, drawing `-` at a `_` break.

**Implementation:** `internal/sprites/endgame.go`, `cmd/ultimav/gamescene_finale.go`, `cmd/ultimav/intro_narrative.go`
**Tests:** `internal/sprites/endgame_test.go`, `internal/sprites/endgame_original_test.go`, `cmd/ultimav/intro_narrative_test.go`
