# Startup, introductions, and acknowledgments

Original startup/title, illustrated Introduction, and Return to the View are
separate workflows. Source/generator structure is not verification of every
graphic, narrative byte, timing driver, or retail version.

### INTRO-0001

**Startup controller and six-choice menu**

**Kind:** rule
**Uses:** CREATE-0001
**Original:** Established
**Redux:** Intentional deviation

Original first-polled J bypasses production/title/automatic attract to Journey.
Other first-polled input is discarded and interruption reset. Production-stage
polls can suppress automatic attract; input caught inside title-frame processing
changes a local flag and does not necessarily suppress the caller's attract call.
Journey checks nonempty Avatar name, not comprehensive save integrity.

Menu direct-key order: J Journey, C Create, T Transfer from Ultima IV, U Introduction,
A Acknowledgments, R Return to the View. North/West selects previous, South/East
next with wrap; Space/Enter activates; direct letter needs no second confirmation.
New pass selects Journey. Idle expiry is 200 scan iterations with temporary speed
275; a ten-second comment is not independently measured timing. Creation/Transfer/
Introduction/credits return to menu; attract repeats until input.

**REDUX DEVIATION: INTRO-0001:** Redux shows seven top-level choices: Journey
Onward, Select Save Game, New Game, Configure, Ultima V Introduction,
Acknowledgments and Return to the View. New Game opens a sub-menu holding Create
New Character and Import from Legacy; Configure opens Display, Audio and Set Data
Directory. Enter or Right opens a sub-menu panel beside the column, Escape or Left
closes it, and the parent stays lit while its panel is open. The selected row is
drawn in yellow between two cursor triangles rather than on an inverted bar, and
Acknowledgments and Return to the View rest in light grey. Navigation clamps
instead of wrapping and title interruption requires Enter. V shows or hides the
build version on the menu frame's lower border; it is hidden by default and
selects nothing. These support current setup/save workflows and bug reports.
Unavailable flip/fizz driver operations use deterministic substitutes rather than
claimed original pixels.

**Implementation:** `cmd/ultimav/intro_menu_scene.go`

### INTRO-0002

**Asset roles and curated credits**

**Kind:** rule
**Uses:** GFX-0001
**Original:** Incomplete
**Redux:** Intentional deviation

TITLE.BIT supplies production artwork; BRITISH.BIT/PTH signature; ULTIMA.16 title/
four flame frames; WD.BIT transition; STARTSC.16 credit center/side scroll.
Credits draw STARTSC shape 1 at (16,63), reveal sides, wait for input. The later
title shape is not the roster. Panel pixels and manual transcription were not
freshly verified in this pass; retain attribution rather than infer recording authorship.

Redux's curated credits retain Origin Systems; Produced/Designed by Lord British;
IBM version John Fachini, Herman Miller, Ed Nelson, Cheryl Chen, Steve Meuse;
Apple II version Lord British, John Miles, Mark Hamner, Toshi Morita, Steve Meuse,
Dallas Snell; artwork Denis Loubet/Doug Wike; the Book of Lore acknowledgment of
Kenneth Arnold's music; project creator/developer Brad Hannah. Original-game music
credit does not establish authorship of separately bundled recordings. Recording
credits come from their authored manifest with existing notices preserved.

**REDUX DEVIATION: INTRO-0002:** one scene-owned scrolling reader combines original,
project, music/effects, and special thanks. After a two-second reading pause it
advances about a line per two seconds. Space pauses; arrows move lines; Page keys
or Shift-arrows page; Home/End jump; Escape closes; Enter does nothing. Manual
navigation pauses. Complete rests visibly; Home then Space replays; reopen starts
fresh. Fixed header/footer surround clipped content. Resize preserves logical
reading fraction/state; prior menu selection and idle baseline are retained.
Existing title music keeps its owner; the reader requires no gameplay save,
decoder/catalog scan, new audio player, or network request.

**Implementation:** `internal/text/credits.go`, `internal/ui/widgets/credits_dialog.go`

### INTRO-0003

**Twenty-one-frame illustrated Introduction**

**Kind:** rule
**Uses:** INTRO-0001
**Original:** Established
**Redux:** Unverified

Use PROPORT.PCS, TEXT.16, STORY.DAT and STORY1.16–STORY6.16. Archive numbers below
mean STORYn.16; shape indexes are zero-based; offsets are STORY.DAT bytes.

| Frame | Archive/shape at X,Y | Extra composition | Text offset |
| ---: | --- | --- | ---: |
| 1 | 1/0 at 0,0 | Section title | 0 |
| 2 | 1/1 at 0,74 | Shape 2 fizz at 40,86 | 273 |
| 3 | 2/0 at 136,0 | — | 971 |
| 4 | 2/1 at 0,38 | — | 1424 |
| 5 | 2/2 at 152,76 | — | 2051 |
| 6 | 2/2 at 0,0 | — | 2756 |
| 7 | 2/2 at 72,38 | Gate shape 3 at 96,39 | Inline text, no data block |
| 8 | 3/0 at 0,0 | Section title | 3437 |
| 9 | 3/1 at 0,82 | — | 3619 |
| 10 | 4/0 at 0,82 | — | 4022 |
| 11 | 4/1 at 0,82 | — | 4467 |
| 12 | 5/0 at 0,82 | — | 4906 |
| 13 | 5/1 at 0,82 | — | 5310 |
| 14 | 6/0 at 176,0 | — | 5678 |
| 15 | 6/1 at 0,0 | Section title | 6443 |
| 16 | 6/2 at 176,0 | Shape 3 at 176,55 | 7066 |
| 17 | 6/6 at 0,46 | Shape 5 at 0,101 | 7803 |
| 18 | 6/4 at 176,78 | Shape 7 at 176,133 | 8562 |
| 19 | 6/2 at 0,0 | Shape 5 at 0,55 | 9291 |
| 20 | 6/6 at 176,55 | Shape 7 at 176,110 | 10112 |
| 21 | 6/4 at 0,87 | Shape 3 at 0,142 | 10903 |

First frame has no initial wait; every later frame clears pending input and waits
before display. One final clear/wait follows frame 21. No whole-sequence skip or
explicit audio call appears in the inspected routine. Redux retains sequence/data
structure with modern presentation; complete pixel/driver cadence parity is unverified.

### INTRO-0004

**Return to the View format and opcodes**

**Kind:** rule
**Uses:** INTRO-0001
**Original:** Established
**Redux:** Unverified

Read 2,000 bytes from MISCMAPS.DAT offset 704. Four maps occupy file 704–1215;
actions start at 1216/local 512. Each map stores four 32-byte rows but displays
19×4 tiles (16-pixel tiles), revealing from column 9 outward. A stale alternate
filename comment is not runtime authority. Final reset restarts the action stream;
the old claimed 655-byte length was not independently measured.

| Opcode | Operation |
| ---: | --- |
| 0 | Set object shape/tile/position |
| 1 | Remove object and restore underlying terrain |
| 2 | Move one step without timed walking refresh |
| 3 | Requested refresh cycles |
| 4 / 5 | Raise / lower current gate |
| 6 | Select map |
| 7 | Fizz object into view |
| 8 | Visual fade toward terrain, not object deletion |
| 9 | Restart action stream |
| 10 | Replace terrain and master terrain cell |
| 11 | Attack visual, not independent object replacement |
| 12 | Clear object list |
| 13 | One step plus seven refreshes |
| 14 / 15 | Begin / repeat-end counted loop |

### INTRO-0005

**Generated attract scenes, input, and display variants**

**Kind:** rule
**Uses:** INTRO-0004
**Original:** Established
**Redux:** Unverified

Summoning: sleeper rises after appearing/disappearing Codex, traverses equipment,
and exits northward—does not return to bed. Journey crosses the strip and enters
a raised/lowered gate. Arrival includes three Shadowlord-tile actors; attack
visual precedes a separate delete/add body replacement. Welcoming puts Shamino
in bed, seats figures, pauses 140 refreshes, resets. This is generated sequence
structure, not a retrospective of earlier Ultima games or verified shipped bytes.

Refresh and opcode boundaries poll input; caught key exits and is not reused by
menu. Gates have 15 rise/fall phases; flames/objects animate. Arrival requests
noise parameters 20,60,10000; Welcoming tones 3000/3 and 2000/3; attack noise
1,1200,4000. Exact wall time/waveforms depend on drivers. Redux does not reproduce
these Return-to-the-View PC-speaker effects.

CGA/Hercules rewrite applicable .16 archives to .4; Hercules uses .HCS charsets,
others .CH. Tandy 16-color falls back below 368 KiB checked available memory.
These do not certify non-DOS ports or every retail revision.
