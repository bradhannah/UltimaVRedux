# DOS graphics formats

Graphics-format facts distinguish archive structure, builder-defined output,
Redux decoder policy, and unverified retail/driver behavior. Original artwork,
fonts, narrative assets, and generated extracts remain user-supplied data rather
than payloads embedded in this reference.

### GFX-0001

**Compression catalogue and decoding model**

**Kind:** rule
**Uses:** none
**Original:** Incomplete
**Redux:** Unverified

The compressed catalogue contains `PROPORT.PCS`, `BRITISH.BIT`, `TITLE.BIT`,
`TILES.16`, `ITEMS.16`, `DNG1.16`-`DNG3.16`, `MON0.16`-`MON7.16`, `CREATE.16`,
`TEXT.16`, `STORY1.16`-`STORY6.16`, `END1.16`, `END2.16`, `ULTIMA.16`,
`STARTSC.16`, and `ENDSC.16`. `WD.BIT`, `IBM.CH`, `RUNES.CH`, and `BRITISH.PTH`
are direct files in the reviewed loading arrangement. Applicable CGA/Hercules
picture names use `.4`; Hercules fixed fonts use `.HCS`.

Redux's reviewed decoding model is a four-byte little-endian expanded length,
then LSB-first variable-width LZW codes. It requires an initial clear code 256,
uses end code 257, first generated code 258, and widths 9 through 12. Clear resets
width, next-entry index, and the previous sequence. After outputting a sequence,
append previous-sequence plus its first new byte to the dictionary; increment
the next-entry index, and increase width when that index reaches `2^width`.
The standard next-entry self-reference case appends the previous sequence's first
byte to itself. Once full, the 4096-entry dictionary stops adding entries until
clear. Redux validates declared output length and zero terminal padding.

The compression catalogue is established, but this pass did not independently
verify all stream transitions, full-dictionary cases, padding, or corruption
acceptance against original binary samples. A functioning Redux decoder is not
independent proof of every original decoder rule. One complete file has since been
checked: expanding the shipped `TILES.16` with this model reproduces its source
sheets byte for byte ([GFX-0008](#gfx-0008)).

**Implementation:** `pkg/graphics/legacy/lzw.go`
**Tests:** `pkg/graphics/legacy/lzw_test.go`, `pkg/graphics/legacy/integration_test.go`

### GFX-0002

**Four-bit picture and dungeon-panel archives**

**Kind:** rule
**Uses:** GFX-0001
**Original:** Established
**Redux:** Matches

Expanded ordinary archives begin with a little-endian 16-bit count, followed by
four bytes per entry: low offset word and a high word that is zero in the reviewed
builder outputs. Each nonmissing record begins with width and height words, then
4bpp pixels with the left pixel in the high nibble.

Ordinary picture output rounds storage width upward to a multiple of eight:

```text
stored_width = 8 * ceil(declared_width / 8)
payload_bytes = stored_width * height / 2
```

Builder-established counts are CREATE 11; STORY1-6 respectively 3,4,2,2,2,8;
TEXT 6; END1/2 three each; ULTIMA 5; STARTSC 3; ENDSC 1. Dungeon-panel archives
declare 28 slots, with 8 and 24 absent and zero-offset. The ordinary first payload
offset is `2 + 4*count`; for the dungeon panels it is `0x72`.

Do not generalize the ordinary builder's rounding operation to every builder:
the dungeon/item/monster builder uses input dimensions directly. Redux validates
payload dimensions and offsets for its supported archives. Exact correspondence
of every shipped file or nonzero high word is not claimed.

**Implementation:** `pkg/graphics/legacy/decode.go`, `pkg/graphics/legacy/catalog.go`
**Tests:** `pkg/graphics/legacy/decode_test.go`

### GFX-0003

**Paired images and masks**

**Kind:** rule
**Uses:** GFX-0001
**Original:** Incomplete
**Redux:** Unverified

`ITEMS.16` and the monster archives use a logical count followed by twice that
many 16-bit offsets: image, mask, image, mask. Each physical record has its own
width/height prefix. Images are 4bpp; masks are 1bpp with the earliest pixel in
the most-significant bit. Builder counts are 20 logical items/40 records and
six logical monster images/12 records per archive; first offsets are `0x52` and
`0x1A` respectively.

Mask bit order is established. The original compositing polarity and treatment
of opaque black were not independently verified in the drawing implementation
or authored masks in this pass. Redux's image/mask interpretation must not be
promoted into a universal original corruption or compositing specification.

**Implementation:** `pkg/graphics/legacy/decode.go`
**Tests:** `pkg/graphics/legacy/decode_test.go`

### GFX-0004

**One-bit archives and proportional font records**

**Kind:** rule
**Uses:** GFX-0001
**Original:** Established
**Redux:** Matches

One-bit bitmap archives use a 16-bit count and 16-bit offsets, then width/height
and packed rows. TITLE has ten builder records and first payload `0x16`; BRITISH
and WD have one and first payload `0x04`. Their builder-defined expanded totals
are 2,116 and 1,772 bytes respectively. These are not measured compressed-file sizes.

The proportional font covers characters `0x20` through `0x7A`: 91 glyphs. A count
and 91 offset words precede 12-byte records containing width, height 8, and eight
row bytes. First payload is `0xB8`, total expanded size `0x4FC` (1,276 bytes).
Space has width zero but still owns a stored record. Glyph width differs from
text advance: spacing and nine-pixel line advance are separate rendering rules.

**Implementation:** `pkg/graphics/legacy/decode.go`
**Tests:** `pkg/graphics/legacy/decode_test.go`

### GFX-0005

**Fixed character-set boundary**

**Kind:** rule
**Uses:** none
**Original:** Incomplete
**Redux:** Unverified

The DOS loading path selects `IBM.CH` as charset 0 and `RUNES.CH` as charset 1,
with seven-bit glyph indexing in their consumers; Hercules selects `.HCS` instead.
The precise fixed-font binary packing was not independently verified here.

Redux expects 128 consecutive 8x8 glyphs, one MSB-first byte per row, requiring
1,024 bytes. Its derived PNG sheets are 16 by 8 glyphs and are not the original
font files. The proportional font's working format is not proof of every fixed
font's layout.

**Implementation:** `pkg/graphics/legacy/decode.go`
**Tests:** `pkg/graphics/legacy/decode_test.go`

### GFX-0006

**Tile-color transformation boundary**

**Kind:** rule
**Uses:** none
**Original:** Incomplete
**Redux:** Unverified

Dungeon tactical setup and the ending request separate tile-color operations.
The exact binary driver tables, affected tile lists, and palette transformations
were not independently byte-decoded in this conversion pass.

The current rendering model uses a selective dungeon permutation that exchanges
palette-index bits 1 and 2, and a separate lossy ending permutation. Redux applies
its selected mappings at draw time before lighting/tinting, preserving gameplay
tile identity instead of modifying the shared sheet. This describes the remake's
strategy; it does not certify every driver's pixels or original timing.

### GFX-0007

**Signature path and asset families**

**Kind:** rule
**Uses:** GFX-0001
**Original:** Established
**Redux:** Matches

`BRITISH.PTH` uses command bytes: bits 4-6 encode X magnitude, bit 7 its sign;
bits 0-2 encode Y magnitude, bit 3 its sign. Plot the current point according to
the preceding command's pen state before applying movement. A magnitude above
two leaves the next iteration pen-up. Zero terminates a segment. Startup consumes
four segments at (44,68), (64,94), (143,78), and (167,105); this is not proof that
every file contains exactly four segments without trailing bytes.

Dungeon assets use DNG/ITEMS/MON families; creation uses CREATE/PROPORT; illustrated
introduction uses TEXT/STORY; startup uses TITLE/BRITISH/WD/ULTIMA/STARTSC; ending
uses END/ENDSC. `TILES.16` shares the compression wrapper but not these archive
layouts; its layout is given in [GFX-0008](#gfx-0008). Intermediate build inputs
are not automatically shipped runtime formats.

**Implementation:** `pkg/graphics/legacy/decode.go`, `pkg/graphics/legacy/catalog.go`
**Tests:** `pkg/graphics/legacy/decode_test.go`

### GFX-0008

**Tile sheet**

**Kind:** rule
**Uses:** GFX-0001
**Original:** Established
**Redux:** Not implemented

`TILES.16` expands to 65,536 bytes with no header or offset table: 512 tiles of
16 x 16 pixels, 128 bytes each, in tile-number order. Each tile is 16 rows of 8
bytes, top to bottom, at 4 bits per pixel. The expanded data is exactly the four
16,384-byte sheets `EGA0.T` to `EGA3.T` from the original source tree, joined in
order; the tile editor worked on 128 tiles at a time.

The left pixel is in the high nibble, as in [GFX-0002](#gfx-0002) pictures. This
order is inferred from agreement with Redux's derived tile sheet; the display
driver's own reading was not examined.

`TILES.4`, used for CGA, expands to 32,768 bytes: the four 8,192-byte sheets
`CGA0.TC` to `CGA3.TC` joined, 64 bytes per tile, consistent with 16 x 16 pixels at
2 bits per pixel. Its pixel order within a byte was not verified.

Redux does not decode `TILES.16`. It draws tiles from a bundled 512 x 256 sheet,
32 tiles across, with tile n at column n mod 32 and row n / 32 (integer division).

**Implementation:** `internal/sprites/spritesheet.go`
**Tests:** `internal/sprites/spritesheet_pixels_test.go`
