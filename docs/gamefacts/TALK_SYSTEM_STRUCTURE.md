# TLK records and conversation behavior

This is a semantic format specification for the inspected DOS conversation model,
not a reproduction of dialogue payloads or its compression dictionary. Original
source version labels do not certify a retail executable or the installed TLK
edition. Redux uses a validated import and semantic runtime.

### TALK-0001

**Family lookup, container, and record grammar**

**Kind:** rule
**Uses:** none
**Original:** Established
**Redux:** Matches

| Small-map IDs | Container |
| --- | --- |
| 1–8 | `TOWNE.TLK` |
| 9–16 | `DWELLING.TLK` |
| 17–24 | `CASTLE.TLK` |
| 25–32 | `KEEP.TLK` |

Normal conversation numbers are below 128; zero means no response. Higher values
use specialized merchant, tribute, or influence handling. Each container begins
with a little-endian 16-bit count, followed by that many pairs of 16-bit
conversation number and absolute record offset. Records start at
`2 + 4 * count`; compression requires rewriting their offsets. The original
reader examines a 512-byte header and reads up to 1,024 bytes at a found offset.

Each record starts with five NUL-terminated fields in this order: name,
description, previously-met greeting, job response, farewell. Ordered
keyword/response NUL-terminated pairs follow. A question section begins with
`0x90`, then its ID, question string, catch-all response, and ordered
answer-keyword/answer-response pairs. The next `0x90` begins another section.
Question IDs range `0x91..0x9F`; `90 9F C0` is the end-of-record sentinel, not a
general executable three-byte opcode. NUL separates fields without printing.
Control operands immediately follow their control in the same string stream.

Redux validates index bounds, ordering, IDs, field boundaries, and terminators
before semantic execution. A scoped match for supported containers is not a
claim to reproduce the original reader's behavior on malformed bytes.

**Implementation:** `internal/conversation/extractor/raw.go`
**Tests:** `internal/conversation/extractor/raw_test.go`

### TALK-0002

**Dictionary expansion and character modes**

**Kind:** rule
**Uses:** TALK-0001
**Original:** Established
**Redux:** Known discrepancy

Values below `0x81` address a 129-entry dictionary with 118 populated words.
The absent indices are `00,08,1C,32,41,43,46,48,49,4A,4B` hexadecimal.
NUL is a field delimiter, not a word. Valid dictionary entries expand as text;
the dictionary's literal word payload is intentionally not redistributed here.

Unhandled high-half text produces its low-seven-bit glyph. In particular raw
`0xE1` is `a`, and raw `0xFD` is `}`, not a jump. Runic mode toggles a high-bit
mask between `0xFF` and `0x7F`; the latter selects Runic glyphs, persisting until
the next toggle. Dictionary expansion follows the same output-mode pipeline.
The original word-wrap buffer is 16 bytes; spaces/newlines flush against the
current message-window cursor. If already full when another byte arrives, it
flushes and drops that arriving byte. Repeated quote output is suppressed.

Redux expands tokens and rich-text mode changes during import, then uses semantic
text layout. It rejects literal braces that collide with semantic substitution,
including `0xFD`, rather than representing every possible original text byte.
It also validates unsupported token/control combinations. This is a supported
input limitation, not evidence that those original bytes meant something else.

**Implementation:** `internal/conversation/extractor/decode.go`
**Tests:** `internal/conversation/extractor/decode_test.go`

### TALK-0003

**Control effects and operand precedence**

**Kind:** rule
**Uses:** KARMA-0007, KARMA-0011, KARMA-0018, CRIME-0002, INV-0004
**Original:** Established
**Redux:** Unverified

An outstanding operand consumes the next byte before opcode dispatch. The
following table describes behavior independently of interpreter implementation.

| Byte | Operand bytes | Meaning |
| --- | ---: | --- |
| `81` | 0 | Insert Avatar name. |
| `82` | 0 | Abort/end the conversation through its callers. |
| `83` | 0 | Up to 28 update/tick cycles; a key shortens the pause. |
| `84` | 0 | Attempt recruitment; success ends Talk. |
| `85` | 3 | Read high-bit-insensitive decimal digits and attempt that gold payment. |
| `86` | 1 | Give the low-seven-bit item code described below. |
| `87` | 0 | Print the next response body, preserving the current stream position unless it aborts. |
| `88` | 0 | Ask party name; recognized name marks the actual actor met. |
| `89`, `8A` | 0 | Apply the increase/decrease rules in KARMA-0007. |
| `8B` | 0 | Activate guards. |
| `8C` | 1 | If met, enter target question; target `FF` returns to interests. |
| `8D` | 0 | Authored carriage return, converted to output newline. |
| `8E` | 0 | Toggle Runic mode. |
| `8F` | 0 | Wait for a key without introducing an output boundary. |
| `90` | structural | Delimit/search for a question, not itself a question ID. |
| `91..9F` | 0 | Enter that question when executed in output. |
| `A2` | text | Quote, subject to repeated-quote suppression. |
| `FD` | text | Literal closing-brace glyph. |
| `FE` | 2 | Compare karma with threshold byte, then conditionally enter question operand. |
| `FF` | 0 | Flush output and return to the interests prompt. |

For Give, codes below `0x40` increment carried arms. Codes `0x41..0x4B`, in
order, grant food, gold, keys, gems, torches, grapples, carpets, sextant, spyglass,
badge, skull keys. `0x40` and values above `0x4B` have no original item case.
Inventory bounds and special ownership representations belong to INV-0004.
The low-code test is not a safe capacity check: `0x30..0x3F` exceeds the
48-entry equipment array, so such malformed selectors have no established
in-bounds item result. This must not become a fictitious extra equipment range.

For AskName, search the first four characters of each active party member's name
inside input, accepting only offset zero or a preceding space. A match records
met state by actor slot, not TLK program number. Failed/full recruitment returns
to interests after feedback and a key wait; it does not start another Talk action.

Redux translates these controls into semantic actions, expands shared responses,
and replaces numeric questions with validated identities. Its custom in-memory
GoToJail value `0xE1` is not a raw TLK opcode; knowledge discovery has no original
byte assignment, and neither has the action that ends a talk to open circle study
([PROGRESS-0005](CHARACTER_PROGRESSION.md#progress-0005)). Arbitrary operand equivalence is unverified: the importer masks
the karma threshold and limits it to 99, while the original compares an unmasked
unsigned byte (see KARMA-0018). Zero-gold Take and
no-effect Give codes are also not a universal semantic-import contract.

**Implementation:** `internal/conversation/extractor/decode.go`, `internal/conversation/semantic_engine.go`, `internal/conversation/protocol.go`

### TALK-0004

**Ordered prompts, matching, and actor state**

**Kind:** rule
**Uses:** TALK-0003, RNG-0001
**Original:** Established
**Redux:** Intentional deviation

Introduction displays description followed by two newlines. A met actor gives
its greeting; a first meeting can randomly disclose its name. Standard NAME,
JOB/WORK, and BYE/THANK handling precedes authored interests. Matching accepts a
substring only at input offset zero or after a literal space, without requiring
a trailing word boundary. Question answers use the same ordered-search idea,
falling back to that question's catch-all when unmatched. Completed responses
normally add two newlines before returning to interests.

Both interests and scripted-question input check reachable global profanity
entries after protocol words. A match gives admonishment and a pause, then
repeats the same prompt. The unreachable index-34 entry is excluded. The list's
literal payload is not reproduced here. Original prompts, speech and input all
share permanent lower-right text window 2, with no portrait, title, modal frame,
or speech/input divider.

**REDUX DEVIATION:** Redux presents a modal conversation card for readable,
keyboard-owned interaction. It retains semantic paragraph/wait boundaries rather
than treating NUL delimiters as output. Program identity is family plus one-based
dialog number; met state uses the actual location and actor slot. Validated
definitions, not a second raw-byte interpreter, own runtime action order.

**Implementation:** `internal/conversation/semantic_engine.go`, `cmd/ultimav/linear_talk_dialog_core.go`
**Tests:** `internal/conversation/semantic_engine_original_test.go`

### TALK-0005

**Two-tile targeting, specialized dispatch, and Talk completion**

**Kind:** rule
**Uses:** TIME-0002, MERCH-0001, CRIME-0001
**Original:** Established
**Redux:** Intentional deviation

Inspect the adjacent cardinal tile. Only if it contains no object and its
terrain is talk-through, inspect exactly one more tile in that direction.
There is no longer scan. Talk-through terrain comprises three table pieces,
magically locked doors, windowed regular/locked/magical doors, portcullis,
three plated-table forms, desk, end table, both fences, candelabrum, and crystal.
A target standing on bed terrain sleeps; an intact mirror gives no response.

On listening, action 4 in the arrived slot changes to action 1. Otherwise a Guard
with even arrived slot or zero Talk gives no response. Talk zero means silence;
normal numbers select TLK; `FD` is fearful response, `FE` Astaroth influence,
`FF` tribute; merchants then apply their arrived-and-desired-slot gate.

The original explicit town Talk completes even after Space direction pass or
no conversation, normally adding one map minute afterward. Conversation routines
do not add another generic minute; UI pauses are not gameplay minutes. Redux
retains workflow-owned completion and no second charge for NPC-initiated Talk.
Its free uncommitted surface Escape cancellation is the deliberate command
policy in CMD-0003, not an original reachable direction-reader branch.

**Implementation:** `cmd/ultimav/gamescene_talk_workflow.go`, `internal/game_state/action_talk.go`

### TALK-0006

**Lighthouse corrections and provisioned-data boundaries**

**Kind:** rule
**Uses:** TALK-0001, TALK-0004
**Original:** Incomplete
**Redux:** Intentional deviation

The baseline local-data audit of `DWELLING.TLK` record 14 reports a
manual-dependent music quiz: staff interpretation, DCB then 987, leading to
numeric phrase `678 987 8767653`. This installed-record observation is not proof
for every TLK edition. The unpatched importer preserves that dependency.

**REDUX DEVIATION:** The tracked Lord Kenneth correction provides repeatable
teaching and a JOB-to-MUSIC signpost. Both claiming and lacking knowledge lead
to the lesson. It explains standing immediately north and using top-row/numpad
numbers; the Journal learns the clue only after the mounted dialogue discloses
the complete phrase. It never performs the tune for the player.

The local audit of record 15 reports a stranded 26-byte final-answer key at
`0x1F0D`, with terminator at `0x1F28`, leaving Y/YES to the catch-all instead of
its intended acknowledgment/name request. The importer audits the orphan rather
than silently inventing an affirmative answer. Binary payloads were not reopened
for this conversion; the source-defined matching/fallback and installed-data
regressions are separate evidence layers.

**REDUX DEVIATION:** Sir Arbuthnot's tracked correction repairs the missing Y
answer and deliberately links successful party-name recognition to existing
disclosures about minting the magical coin, its Codex symbol, and the secret
commission to summon the Avatar. That story connection is a usability choice
in addition to the defect repair. Recognized visitors may repeat it; COIN keeps
its earlier ordered match, and MAGI and SYMB/CODE remain independently available.
No item gate, invented lore, or automatic Chronicle fact is added.

Tracked corrections: `data/conversation-patches/dwelling/014.patch` and
`data/conversation-patches/dwelling/015.patch`. The one other patch that adds
spoken lines, `data/conversation-patches/keep/011.patch`, is not a correction; it
belongs to [PROGRESS-0005](CHARACTER_PROGRESSION.md#progress-0005).

**Implementation:** `internal/conversation/semantic_engine.go`, `cmd/ultimav/linear_talk_dialog_discovery.go`
**Tests:** `internal/conversation/extractor/arbuthnot_original_test.go`, `internal/conversation/semantic_engine_original_test.go`
