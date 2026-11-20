# Semantic Conversation System

## Journal excerpts and last-conversation retention

Talk records exchanges at actual output insertion boundaries. Dormant
construction does not replace Last Conversation until mounting. Submitted
Avatar input starts a new exchange; timed and any-key continuations remain in
the current exchange. Runic passages retain their font identity. Cancellation
discards unseen speech. Tab keeps the current or history-selected exchange
without acknowledging waits or submitting input. Last Conversation remains
after Talk closes and supports K capture from Journal.

Journal attribution assumes the NPC's semantic name when recording a conversation,
so Last Conversation, kept excerpts, and disclosed Chronicle clues identify their
source without requiring the player to ask NAME. This presentation policy remains
independent of NPC recognition flags and retained notes: deleting all excerpts
does not make subsequent captures anonymous. It does not execute the NAME body,
advance dialogue, capture unseen speech, or grant unpresented discoveries.
Last Conversation is a continuous transcript with selectable exchange boundaries
for capture. Met-NPC flags describe the NPC recognizing the Avatar; journal
attribution never changes those flags. See the
[save contract](../details/SAVE_SNAPSHOT_SCHEMA.md#snapshot-17-journal-extensions)
for bounded retention and the [save-owned excerpt contract](../details/SAVE_SNAPSHOT_SCHEMA.md#snapshot-25-save-owned-excerpts).
Keeping or managing an excerpt changes only the active game; saving persists it
inside that checkpoint. Loading restores its text and archive choices without
merging later notes. Older native saves have a temporary read-only notebook importer.

Normal NPC conversations execute validated semantic definitions. Original
`DATA.OVL` and the four TLK families are
imported in memory at startup, exact project patches are applied, and the
resulting catalog is executed by `SemanticConversationEngine`.

Original TLK records, opcodes, keywords, and flow are documented separately in
[Talk System Structure](../gamefacts/TALK_SYSTEM_STRUCTURE.md).

Gameplay and offline lookup both use the semantic catalog.
Original binary decoding belongs to the importer; no compatibility AST or
second interpreter is loaded. Shared response/action contracts live in
`internal/conversation/protocol.go`. Talk callbacks expose NPC dialog creation
and mounting; guards, wells, and merchants use their dedicated production workflows.

## Runtime Path

1. `NewGameReferences` calls `catalog.LoadOriginal` with the configured original
   data directory and embedded `data/conversation-patches` filesystem.
2. `extractor.Import` validates the original files and converts all 135 records
   directly into canonical semantic documents without publishing copyrighted
   YAML to disk.
3. The catalog applies metadata-bound unified diffs exactly, reloads every
   patched document through the strict semantic loader, validates identity, and
   indexes original conversations by TLK family plus one-based dialog number.
4. The Talk workflow passes the authoritative `NPCFriendly` to
   `NewLinearTalkDialog`. The dialog resolves the semantic program from the
   current location family and NPC dialog number.
5. A detached definition and the NPC's real map-unit slot initialize
   `SemanticConversationEngine` and `GameActionCallbacks`.
6. `LinearTalkDialog` presents the engine's `ConversationResponse` values and
   owns modal input, pauses, key waits, runic rendering, and dismissal.
7. The dialog records completion only when the semantic engine reaches a
   terminal response. The owning Talk workflow distinguishes that state from a
   dialog removed during cancellation or scene cleanup.

The production dialog currently renders as one opaque black classic card with a
rounded white outer frame and no internal divider. Speech history, active
prompts, and responses share one bottom-aligned stream. A prompt occupies one
retained row; the next row starts with the classic light-green command marker,
and live keyboard input renders immediately after it before being committed to
that same row. The joined-pane and original-colon presentations remain local
compile-time alternatives while this visual treatment is evaluated.

Talk uses genuine free-text input: spaces survive submission, Tab does not print
an empty autocomplete list, and blank farewell and fresh-key waits keep their
existing semantics. The current dialog retains already-inserted logical output,
bounded to 256 insertions and 256 KiB of text, not 256 wrapped rows. Eviction or
an oversized insertion shows an explicit earlier-text truncation notice.
Page Up or Home enters history; Page Up/Page Down page, Home goes to the oldest
retained text, and End/Escape returns to live Talk. Review neither resumes engine
actions nor flushes future speech, advances puzzle/calendar time, or consumes
gameplay RNG. It preserves partial input, Runic segments, key waits, and the
remaining pause duration. Same-instance resize reflows typography without
discarding speech or input. History is transient to this conversation, not a
save field, journal expansion, or transcript of future/unvisited topics.

Conversation identity and actor identity are separate. TLK family plus dialog
number selects a program; location plus map-unit slot owns persistent met state.

## Semantic Format

The closed v1 grammar is implemented in `internal/conversation/semantic`:

- Required fixed bodies: `name`, `description`, `greeting`, `job`, and
  `farewell`.
- Ordered topic keyword groups and named `question_01` through `question_15`
  question blocks.
- `say` and `narrate` rich text with ordinary text, runic text, and line-break
  parts. Ordinary text supports validated `{avatar_name}` and `{npc_name}`
  substitutions.
- Branches for NPC name knowledge and karma thresholds.
- Gold, item, karma, knowledge discovery, pause, key wait, AskName, recruitment,
  guards, jail, circle study, return-to-topics, and end actions.

The loader rejects unknown fields and variants, duplicate keys, aliases,
invalid substitutions, missing question targets, duplicate normalized
keywords, malformed numeric values, and misplaced terminal actions.

## Protocol Flow

An engine instance is stateful for one conversation. `Start(actorID)` renders
the description, runs any description-triggered question flow, and shows the
known-NPC greeting where applicable. `ProcessInput` resolves state in this
order:

1. Resume `wait_for_key` without inserting quote or whitespace boundaries.
2. Resume `ask_name`, update session knowledge, and persist the real actor slot.
3. Match the active question's ordered answers or default body.
4. Handle empty input and standard NAME, JOB/WORK, and BYE/THANK interests.
5. Match ordered semantic topics.
6. Return the standard no-help response.

Original substring matching requires the match to begin at input offset zero or
after a space. Semantic topic and answer order is preserved.

`REDUX DEVIATION:` The original randomly introduces about
half of unmet NPCs by name. Redux retains its established first-meeting behavior
and waits for the player to ask NAME, making the player's inquiry the discovery
boundary. See [TALK-0004](../gamefacts/TALK_SYSTEM_STRUCTURE.md#talk-0004).

## Action Boundary

The engine owns conversation state and output structure. Shared legacy actions
are delegated through `ActionCallbacks`; semantic-only discovery selects its
immediate or deferred callback capability at initialization:

- Recruitment uses the semantic NPC name supplied when callbacks are created;
  it no longer reaches back through the dialog stack.
- AskName uses `SemanticCallbacks.GetActivePartyNames`, separately from Avatar
  name substitution. It matches the first four source bytes (or shorter name up
  to NUL) at input start or after a literal space, not arbitrary shorter prefixes.
  Met state uses the actor's map-unit slot, not its TLK dialog number.
- Verified semantic item names map to original talk item IDs before inventory
  callbacks execute.
- Karma deltas and item quantities execute as repeated authoritative callback
  operations.
- Scalar `discover_knowledge` keeps its authored grammar and order. Production
  uses `NewDeferredSemanticConversationEngine`, which requires
  `DeferredKnowledgeCallbacks.QueueKnowledge` at initialization rather than
  silently falling back to an early commit. The typed `DiscoveryBoundary`
  identifies a response serial and source-byte offset in semantic output,
  including pause markers. Explicit headless callbacks may retain immediate
  `DiscoverKnowledge(KnowledgeID) error` execution.

`GameActionCallbacks` captures each pending discovery's journal attribution label
at execution, using `Speaker` only when no label was supplied by its caller.
`LinearTalkDialog` commits only through the source boundary actually
inserted into its mounted live output. Dormant initial responses wait for mount;
normal/Runic chunks, timed pauses, key waits, and terminal responses retain their
boundaries. Forced removal/cancellation discards unseen tails, never flushes them.
History review and resize do not advance the presented cursor. This boundary is
not inferred from the callbacks' accumulated-output mirror or by parsing rendered
words. See `cmd/ultimav/linear_talk_dialog_discovery.go` and its Gruman/Sutek regressions in
`cmd/ultimav/linear_talk_dialog_discovery_test.go`.

Actual curated changes publish through mandatory `UI.JournalChanged`; the scene
stamps finite visit metadata and coalesces one notice per newly informative
response, not per fact/tab. Repeated disclosures and hydration are quiet. The
ordinary main Output `Journal updated.` notice and separately styled
`[Shift+J] View journal` hint share one scene-owned pending queue with map updates.
Notices wait for conversation, secondary prompts, readers, arrival modal, fade
and workflow ownership to clear at both frame entry and exit, then flush into
normal Output/History. Pending map updates precede the coalesced journal notice.
There is no timed card, countdown or fade. The queue remains outside the modal
stack and save restrictions, with no input, sound, acknowledgement or automatic
reader opening.

Semantic `give_item: sextant` resolves original item ID 72 through production
`GameActionCallbacks.GiveItem` to authoritative `GameState.GrantSextant`. That
operation commits inventory and, only on genuine first receipt inside valid
Greyhaven with existing entry/name evidence, its local exterior survey. Quantity
loops cannot duplicate or refresh the survey. The existing receipt text stays
conversation-owned; `UI.AtlasUpdated` queues one coordinate update after commit,
appended when the conversation releases output ownership. Setup, debug
inventory changes and hydration do not call this live grant operation.

A conversation gift that would exceed its carrying limit grants nothing and prints
"Thou canst carry no more <item>!" instead of the usual "Received:" line.

`ErrInsufficientPayment` is an ordinary refusal, not a resumable callback error.
It abandons all paid continuation frames, the active question, and pending
name/key waits, then returns to topics. A subsequent No, NAME, BYE, or new topic
cannot execute an unpaid reward tail. Other callback errors still surface as
errors. `TestKristiProductionPaymentRefusal` covers 0/99/100 gold through real
catalog/dialog callbacks, including exactly-once paid success.

The closed knowledge inventory contains 35 scalar IDs: three spell formulas,
six separate Shadowlord names/associations, eight shrine mantras, seven journal
domain/Flame/method clues, the Resistance and Oppression passwords, the Stones
melody, and eight dungeon Words of Power. The constants in
`internal/conversation/semantic/model.go` are the live inventory authority.

Journal discovery patches record the actual revealing branch, not a typed
keyword or an engine-known pairing:

| Source | Discovery boundary |
|---|---|
| Shalineth (`keep/012`) | MERC/SIMP reveals a standalone Falsehood domain before the oath; the Yes continuation reveals Faulinei's name/association. Refusal preserves only the earlier note. Protocol NAME intercepts the NAME alias. |
| Michael (`keep/016`) | His Hatred disclosures record a nameless domain, not Astaroth's identity. |
| Sin'Vraal (`dwelling/011`) | JOB/WORK records Astaroth's name only; ASTA explicitly associates him with Hatred. JOB plus SHAR does not infer that association. |
| Malone (`keep/022`) | EVIL/COWA/SHAD records Nosfentor's name only; NOSF explicitly associates him with Cowardice. |
| Sir Sean, Barbra, Gardner (`keep/026`, `keep/019`, `keep/025`) | Their keeper JOB/WORK disclosures, plus Barbra's KEEP response, record the corresponding Flame and keep, not coordinates or a destruction method. |
| Sutek (`dwelling/010`) | Record the method only after the complete paragraph ending in the Principle each Shadowlord opposes, not the initial Yes or partial instructions. |
| Chamfort (`towne/017`) | `RESI` → `TERR` → `Y` → a recognized party name reaches `question_03`. Record `journal.resistance.password` after the Runic `DAWN` speech, beyond its preceding pause. Wrong referral, refusal, an unrecognized party name, and cancellation before that speech reveal no password. His `MANT` → `JERE` mantra is independent. |
| Flain (`towne/038`) | `OPPR` → fresh key → `JUDG`/`DRYD` → `Y` → `MALI`/`ANNO`/`GOET`/`FELE`/`FION`/`SIND` reaches the accepted `question_04` response. Its pause and acceptance speech lead to a separate fresh-key wait. Record `journal.oppression.password` only after the final paragraph tells the player to give Elistaria the password `Impera`. `HASS`, wrong answers, refusals and cancellation before that paragraph reveal nothing. Elistaria (`keep/009`) accepts the password but does not disclose it; a guessed answer there does not grant the Chronicle fact. |
| Lord Kenneth (`dwelling/014`) | JOB signposts MUSIC; COUR/COMP/STONES/MUSIC/HARPSICHORD offer teaching. Agree to learn, then either answer to whether the tune is known reaches the same friendly lesson. The fresh-key continuation presents north-of-harpsichord placement, numeric controls and the complete phrase before `journal.stones.melody`. The mounted output boundary commits it automatically; repeat lessons preserve first provenance. |

Arbuthnot (`dwelling/015`) accepts Y/YES at both Avatar questions. The repaired
acknowledgment requests a party name unless already recognized. A new recognized
name sets the real actor slot; rejection returns cleanly to topics. Recognition
continues through the original coin, Codex symbol, and secret commissioning
disclosures, separated by fresh-key acknowledgments, then returns to topics.
Existing negative/default responses and topic routes remain intact. This patch
contains no automatic discovery action. Original evidence and the deliberate
story-flow departure are recorded in
[TALK-0006](../gamefacts/TALK_SYSTEM_STRUCTURE.md#talk-0006).

Lady Janell (`keep/011`) is the one patch that adds a Redux service rather than
a correction or a discovery. QUEE/MAG/BRIT keeps her original answer, then after a
fresh key adds one sentence naming the Great Council. COUN/GREA and STRO/DEFE
are new answers, the second leading through two fresh keys to `question_03`. Y
speaks one closing line and runs `begin_circle_study`, which ends the talk; a
terminal action with no speech before it would show an empty quotation. Any
other answer gives her existing `I see.` and returns to topics. Her original topics, questions and
farewell are unchanged. The lore boundary and the service are recorded in
[PROGRESS-0005](../gamefacts/CHARACTER_PROGRESSION.md#progress-0005).

Standalone domain notes remain nameless Chronicle entries even after a name is
learned separately. The three fixed Shadowlord rows expose Embodies and derived
Opposes only after explicit association; a prior association-only save remains
valid. Association implies the domain is known without fabricating another
standalone event. Flame/place and method discoveries remain independent.
Knowledge and current shrine/life flags retain gameplay authority; Chronicle
records first occurrences with authentic date/source and never replays into
progression. Correct blind gameplay input remains valid.

Outside Talk, the existing blue-Flame-plus-keep Look identification records an
idempotent `observation` discovery. Red braziers, covered tiles/top NPCs, and
non-keep Look retain their existing precedence and time cost. Codex answers use
their actual commit with `revelation` provenance. These paths are implemented in
`internal/game_state/action_look.go`, `internal/game_state/player_knowledge.go`,
`internal/game_state/quest_journal.go`, and `internal/game_state/shrine_service.go`;
they do not pretend observation is NPC dialogue.

Seven NPC patches add scalar `discover_knowledge: dungeon.<name>.word_of_power`
after their actual word-bearing responses: Malifora (`towne/002`), Annon
(`towne/012`), Goeth (`towne/015`), Felespar (`towne/022`), Fiona (`towne/026`),
Sindar (`towne/032`), and Hassad (`castle/017`). They use the same mounted
response-boundary commit and captured dialogue provenance, without rewriting
speech or Runic text. Word discoveries create permanent searchable Chronicle
notes only; they neither unseal dungeons nor change fixed virtue/Shadowlord rows.

The display projection must preserve the clue actually disclosed. Goeth's
`question_01/drat` branch requires the player to identify Destard before hearing
`AIPONI`, so Chronicle and Dungeons can name Destard. The reversed clue is not
corrected to `INOPIA` until successful entrance use. Sindar's
WORD response reveals `INFAMA` but not Shame, which he names separately under
WHIC; the word note does not automatically join those responses. The other five
NPC revealing responses explicitly name their target dungeon, so those notes
may include it. Stable dungeon keys are never indexed as hidden search text.

The eighth word uses the Codex workflow rather than an NPC patch.
`GameScene.addCodexRevelationSequence` commits Doom knowledge only after
`CodexRevelationMessages[3]` is presented and acknowledged, before message `[4]`,
with `revelation` source `Codex of Ultimate Wisdom`. The existing five source
messages remain unchanged. Neither completing all shrine quests nor queuing the
revelation grants knowledge. A completed checkpoint may learn it on a later
revisit; repeats preserve the first date/source and produce no new notice.
The projected note records `VERAMOCOR` to unlock the gate, not the unspoken
engine dungeon label Doom. See
[WORD-0004](../gamefacts/WORDS_OF_POWER.md#word-0004).

`REDUX DEVIATION:` Original conversations reveal these facts through dialogue
without a general persisted semantic-knowledge registry. Redux records explicit
semantic discoveries for dossier and journal use.

The UI uses `Output` as the plain mirror of `TextSegment` values. Pause markers
remain inline, while `WaitingForKeypress` represents an indefinite fresh-key
acknowledgement. Source `line_break` parts and protocol-routine separators are
rendered exactly; adjacent text actions do not acquire heuristic spacing.
The engine imports no Ebitengine or UI package.

After a completed normal conversation, the Talk workflow owns any contextual
post-conversation consequence. Automatic Faulinei presence invokes the original
theft mutation described by [SHADOW-0003](../gamefacts/SHADOWLORDS.md#shadow-0003) and then presents
`Something was stolen!` once. This occurs after terminal conversation output,
not merely after dialog removal. Cancelled conversations and alternate Talk
handlers such as merchants, Guards, sleeping NPCs, and automatic Astaroth or
Nosfentor responses do not invoke it. Yell-summoned Faulinei is also excluded
because it does not establish automatic town presence.

A completed conversation that ended with `begin_circle_study` then opens the
circle study list. The action only records the request on the host callbacks;
the Talk workflow reads it after the dialog has closed and owns the list until
the player leaves it. The list only records a choice; the workflow asks the
yes-or-no question before any gold is spent. A dialog removed before its terminal response opens
nothing.

Ordinary input responses continue from the submitted prompt row. This makes the
first of the original two protocol newlines terminate that occupied row and the
second produce one blank separator row. The same physical-row contract applies
to plain and runic output. The response field remains inline on the final
transcript row; it does not acquire a separate input box.

Keyboard hints occupy a connected footer below the transcript, separated by one
classic frame rule. The footer uses the conversation font and line spacing,
and shared white key/grey description styling. It keeps one row across input,
waits and capture confirmation, so inline input never moves when a hint changes;
history review and capture failures take a second row from the transcript.
The card spans the map's width and starts below the Avatar and the tile beneath
it, so the speaker stays visible; it ends just inside the map frame. The footer
takes its rows from the bottom of the card.
`Tab` keeps an excerpt and `Shift+J` opens Journal. The footer does not list
History, which keeps the hints to one row; command help lists Page Up. During a
wait, the footer advertises Space to continue or close. Successful capture
briefly changes the Tab label to `Excerpt kept`; this expires after three
seconds of scene elapsed time without entering speech/history or changing
inline input. Failures retain a bounded readable status and Retry hint. The
interest placeholder lists topic suggestions (`name, job, bye`); controls
belong to the footer.

`REDUX DEVIATION:` Original wait control `0x8F` waits silently
([TALK-0003](../gamefacts/TALK_SYSTEM_STRUCTURE.md#talk-0003)). Redux shows
`[Space] Continue` in the footer for usability, without appending it to speech,
the response field, or history, and without creating a speech boundary. The
underlying fresh-key acknowledgement behavior remains unchanged.

## Authoring Workflow

Original extracted catalogs remain private. The tracked source of corrections is
the cumulative patch tree:

```bash
make conversations-init
make conversations-diff
make conversations-check
make conversations-patches
make conversations-verify
```

The default private workspace is
`$HOME/.ultima_v_redux/conversations/{base,edited}`. Patch generation and
application are deterministic and fail closed on source hash, schema, extractor,
path, or hunk mismatches.

## Inspection And Tests

Semantic dialog identities are one-based and are not NPC map-unit slots. For
offline lookup, `go run ./cmd/ref_dump --output u5dump/json` exports
`ConversationCatalog.json` alongside NPC, tile, and location snapshots.
Conversation values are the exact post-patch semantic documents converted to
JSON, not another script format. `scripts/npc-lookup.py` uses them for name and
topic/question-keyword searches; location/schedule lookup uses `NPCReferences.json`.
These generated snapshots contain original game data, are private, and are
git-ignored. Regenerate them after source data or project patches change.

Focused verification:

```bash
go test ./internal/conversation/...
go test ./internal/conversation/catalog/...
go test ./internal/conversation/extractor/...
```

Synthetic tests cover every runtime action family and protocol state. Optional
real-data tests discover the configured original-data directory, import all 135
definitions, and run each through a bounded smoke conversation. Balinor at
`keep/010` has source-backed extraction, staged execution, and production
catalog-to-dialog physical-row fixtures covering its description-to-question
boundary, four successive key waits, and final question prompt. Annon at
`towne/012` covers both karma outcomes, every imported trigger alias, and a
successful question answer through the semantic runtime.

`semantic_engine_regressions_original_test.go` retains source-backed behavioral
regressions for Ava's name/offering/donation flow and repeat recognition,
Chuckles' sequential waits and entertainment retry, Saduj/Jaana recruitment and
capacity refusal, and Delwyn/Greymarch greeting paths. Synthetic tests additionally
cover default-only questions entered through JOB and all protocol/action families.
Offline lookup projections and source/dialog-to-NPC identity joins are tested by
`python3 -B scripts/test_npc_lookup.py`.

Tracked source-backed patches add knowledge discovery without adding dialogue
output, including the independent domain, keeper, staged Sutek, and seven NPC
Words of Power paths above. The Lighthouse corrections and Lady Janell's circle
study are the patches that add spoken lines.
Runtime patch coverage is enumerated by
`internal/conversation/catalog/catalog_original_test.go` and
`internal/conversation/semantic_engine_original_test.go`. Original-data tests
require locally supplied assets; their presence is not evidence that private
catalog generation or production acceptance has been completed.

Asset-independent word regressions exercise all eight core discoveries and the
production scalar-loader/engine/mounted-Talk path for Goeth, plus the real Codex
workflow over synthetic map tiles. They cover unseen-tail cancellation,
acknowledged-page timing, completed-save revisits, unchanged Codex text,
provenance/idempotency, and projection redaction:

```bash
go test -race ./internal/game_state -run '^TestWordOfPower'
go test -race ./cmd/ultimav -run '^Test(GoethSerializedWordOfPower|CodexWordOfPower)'
go test ./internal/save_snapshot -run '^Test(DungeonKnowledgeV16|JournalV16)'
```

These synthetic runs do not verify the seven patches against original files.
That gate uses `TestOriginalWordOfPowerDiscoveryBranches`,
`TestOriginalWordOfPowerNonDiscoveryBranches`, and
`TestOriginalCatalogKnowledgePatches` with provisioned original data, plus the
existing patch-verification workflow. An optional skip for unavailable data is
not a pass; final run evidence belongs in the owning issue or pull request.

## Key Files

- `internal/conversation/semantic/`
- `internal/conversation/extractor/`
- `internal/conversation/catalog/`
- `internal/conversation/semantic_engine.go`
- `internal/conversation/protocol.go`
- `internal/conversation/patch/`
- `internal/conversation/workspace/`
- `data/conversation-patches/`
- `cmd/ultimav/linear_talk_dialog_*.go`
- `cmd/ultimav/game_action_callbacks.go`

Merchant text expansion still uses `internal/references/tlk_compressed.go`;
that dictionary is independent of NPC conversation import and execution.
