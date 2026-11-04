# Save Snapshot Schema

## Status

This document describes native envelope version 2 and canonical `SaveSnapshot` version 25 in
`internal/save_snapshot`, its strict native envelope, and the runtime
capture/hydration boundary.

The architecture decision is recorded in [ADR-009](../ADR/ADR-009-canonical-save-snapshot.md).
Original-save conversion dispositions are recorded in the
[legacy crosswalk](LEGACY_TO_NATIVE_SAVE_CROSSWALK.md).
For headless command-line conversion and loading either a standalone snapshot or
a player slot, follow the [save converter procedure](SAVE_CONVERTER.md).
The executable Draft 2020-12 contract is
[`native-save-e2-s25.schema.json`](../schemas/native-save-e2-s25.schema.json), with
frozen V16–V24 sibling resources loaded locally. Assert its version-aware
`ultima5-world-journal` format as well as structural validation. Go DTOs and the
strict codec in `internal/save_snapshot` own the current wire contract.
Version 24 (first shipped in `v0.1.0-preview.1`) is the only frozen strict
promotion source, described by
[`native-save-e2-s24.schema.json`](../schemas/native-save-e2-s24.schema.json).
Frozen V16–V23 schemas remain solely because later schemas reuse their
definitions; they are not decode paths. See
[Snapshot Version Compatibility](#snapshot-version-compatibility).

Schema `$id` values use this repository's canonical namespace; cross-file `$ref`
values use sibling filenames and unchanged JSON pointers. Register those local
resources when compiling schemas rather than fetching them from the network.
The schema files are available under `docs/schemas/` in the public source on
`main`; `$id` remains a schema identifier rather than a requirement to fetch it.
Public availability does not change accepted fields, validation constraints,
envelope or snapshot versions, or promotion.

## Principles

### Snapshot 25 Save-Owned Excerpts

The required `excerpts` member contains the complete kept-exchange collection:
stable `id`, `speaker`, `location`, Avatar `input`, ordered `passages` (`text`,
`runic`), and optional `archived` (missing means false). Explicit null or `[]`
means no excerpts. Missing the member is invalid in V25. Duplicate IDs, malformed
text and retention-limit violations are rejected. Last Conversation still rejects
archive metadata. Assert `ultima5-world-journal` with the version-appropriate Go
validator to enforce excerpt identity, UTF-8 byte and aggregate bounds as well as
the structural schema.

`GameScene` owns `journal.Excerpts`, captures a detached copy and restores it
without merging the departing scene. Keep, archive, restore, delete and attribution
changes are unsaved state until a normal save. This also applies to Quick Save,
named snapshots, rollback and pre-Doom recovery. Two saves
with the same playthrough ID are independent. Current-format loads and Journal
browsing never access external notebooks, including when the saved list is empty.
An unsaved game can keep excerpts; a portable save needs no companion notebook.

**Temporary old-save compatibility:** strict V24 decode records transient
envelope provenance through `NeedsExcerptMigration()`. Pure schema migration
does no I/O. Before constructing a replacement scene, the application imports
the matching `<player-save-dir>/excerpts/<playthrough-id>.json` through the
read-only legacy repository. Each old save imports independently. Missing files
produce an empty collection; missing identity/configuration produces an empty
collection with a diagnostic rather than guessing another notebook. Invalid,
unreadable or oversized files fail preparation without replacing the scene or
rewriting either input. Resave normally to embed the result. Until resaved, an
untouched old file can import again; no global migration ledger is used.

The old notebook contains the latest shared notes, **not historical per-save
lists**. Initial import may include later discoveries; it cannot recover deleted
notes or reconstruct earlier archive choices. Independence starts at import and
is durable after resaving. Original-game conversions and new games start empty.
Legacy files remain untouched for other old saves. The importer is explicitly
marked temporary in code and can be removed in a future iteration when support
for external-notebook saves is retired; embedded persistence remains independent.

### Snapshot 24 Personal State And Chest Lifetimes

These additions share **one snapshot version, 24**, within unchanged envelope 2.
They are checkpoint state, distinct from transient readers; excerpts now also
follow their checkpoint under V25. Root names below are defined in `snapshot.go`;
`validation_v24.go` in `internal/save_snapshot` owns cross-field validation.

| Current wire field | Durable meaning |
|---|---|
| `basementChestHistory` | Exactly three records in `references.LordBritishBasementChests()` order: authored NPCs 23, 24, 25. Each required `lastOpened` is null or a full Britannian `{year, month, day}` date. Null means no known opening. History is independent of active map/object presence. |
| `mapUnits[].surfaceChestAuthored` | Explicit authored chest lifetime; reference compatibility is checked against installed NPC data. |
| `mapUnits[].surfaceChestTransient` | Explicit generated child-chest lifetime, independent of authored NPC/daily identity. The two flags are mutually exclusive, require `kind: chest` when true, and are false on non-chests. Physical `surfaceChestNumber` and learned assessment remain separate. |
| `atlasNotes` | `journal.AtlasNotesState`: allocation watermark `lastId` plus `records`. Each record has `id`, `position`, `label`, `description`, and `recordedAt`. |
| `atlasFleet` | `journal.AtlasFleetState`, containing only the allocation watermark `lastId`, never a second position/history cache. |
| `partyVehicle.vesselId`, `mapUnits[].vehicle.vesselId` | Stable registered frigate/skiff identity across boarded and parked representations. Zero means unregistered; other vehicle types must use zero. |
| `codexRevelationComplete` | Boolean recording acknowledgement of the final Codex revelation page. It is independent of earlier Word of Power acquisition. |

Note IDs are positive, strictly increasing in record order, unique and no greater
than `lastId`; allocation watermarks are bounded by `2^53-1` and survive deletion.
`records` is required, with null or `[]` representing an empty collection.
There are at most 256 records and one note per exact position. Positions use
`SiteLocation` (`map`, `floor`, `x`, `y`) and must be Britannia surface map 0,
floor 0, coordinates 0–255. A note has a valid recorded `GameTime`, a trimmed
nonempty label of at most 32 printable ASCII characters, and a description of
at most 500 characters using printable ASCII plus LF. Editing preserves its
point/date; drafts and pending deletion confirmations are never serialized.
Restore invalidates stale drafts instead of accepting a token from another
checkpoint.

Live source-gate Look uses the existing `moonstoneJournal` records and `look`
source enum, without new fields or a snapshot-version bump. V24 accepts
`firstSource: "look"` as legitimate first-discovery evidence, with the usual
required discovery/observation dates, private site and same-map/floor observer
context. A measured Look stores the gate target in `survey` and records held
sextant evidence in `surveyContext`; strict 19:00–05:59 or recorded flexible
daytime eligibility still applies. An unmeasured record has zero survey and survey
context; an ineligible revisit at the same placement retains an existing survey.
Current-placement and sticky origin-retirement rules are unchanged.
Capture/hydration preserves the evidence without deriving observations or surveys
from live placement, renderer state or newly acquired tools. Codec/schema coverage
is in `internal/save_snapshot/moongate_look_test.go`; the shared validation is in
`internal/save_snapshot/validation_v20.go`.

Fleet validation checks parked and boarded vehicles together: nonzero IDs must
be unique, no greater than `atlasFleet.lastId`, and belong only to frigates or
skiffs. A nonzero `mapUnits[].vehicle.vesselId` additionally requires the enclosing
map unit to have `kind: vehicle`; vehicle details on another unit kind cannot
grant registration. Pending deliveries cannot already be registered. Boat-use
boundaries with sextant possession allocate identity; possession alone, readers,
promotion and hydration do not register boats or backfill pre-sextant history.
Atlas positions are derived from actual eligible parked surface units; boarded,
removed, destroyed and pending-delivery boats do not leave stale map entries.

The Codex word records immediately after acknowledged page `[3]`, before the
closing page `[4]`. The completion flag is reserved for final-page acknowledgement.
A true flag requires the existing word knowledge and matching Chronicle event;
their already-valid first-discovery provenance may be `dialogue`, `observation`,
or `revelation`. Completion does not impose revelation-only provenance or rewrite
the original date/source/identity. It authorizes the complete four-paragraph clue,
not another event or disclosure of Doom's unspoken name. Shrine completion and
word knowledge alone do not prove full revelation.

Capture/hydration in `internal/game_state/save_snapshot_personal.go` copies the
three optional dates, note records and fleet watermark, restores chest lifetime
identity before physical state/assessment, and preserves Codex completion.
`NewGameStateFromSaveSnapshot` installs daily history through
`restoreBasementChestHistory` **before any controller population**, including
legacy-import off-floor reconstruction. Saved surviving objects are restored
afterward; reconstruction cannot recreate a consumed daily definition first.
Hydration is continuation, **not basement entry or boat use**: it must retain
saved chest absence even after midnight, plus actual surviving chests and loot.
Replenishment occurs only on a later eligible real basement entry. Static atlas
artwork, overview/list selection, current fixes, notices and rendering caches
remain derived or transient. See [daily chest behavior](../gamefacts/COMMANDS_REFERENCE.md#cmd-0009).

### Snapshot 23 Timed Doors And Atlas Authority

Envelope 2 is unchanged. Snapshot 23 adds the required nullable `openDoor`
member and expands the finite atlas domain without changing survey member shapes.

- `openDoor: null` means no timed opening. A non-null value contains required
  `position`, `remainingTurns`, and `closedTile` fields. Position uses location
  1–32, X/Y 0–31, and floor -1–3, and must belong to the active small-map
  **location**, not necessarily the party's current floor. The countdown is 0–2;
  zero is a valid stable boundary even without an occupant. The exact remembered
  unlocked variant is tile 184 (regular) or 186 (view door).
- A persisted `setObject` override at that position must retain the same closed
  identity, never the temporary floor tile. Runtime compatibility additionally
  verifies the installed map/floor and effective persistent terrain after both
  mutation layers. Capture/hydration details are in
  [Small-Map Door And Gate Continuation](#small-map-door-and-gate-continuation).
- `atlasLandmarks` now permits at most 39 unique records: the existing 31
  ordinary exterior sites and seven physical shrines, plus Guardians key 108.
  Guardians requires kind `guardians`, surface anchor `(233,235)`, source `look`,
  and no name or naming provenance. Its optional survey also requires `look`
  and the ordinary measurement policy; recognition alone does not grant a plot.
- `sextant-receipt` is a narrowly scoped acquisition/survey source for Greyhaven
  key 11, kind `lighthouse`, surface anchor `(104,216)`. It requires a measured
  matching survey, an existing nonempty name with `nameSource: "enter"`, and
  genuine observation/measurement dates. A known naming date must not follow
  the receipt; a previously undated legitimate Enter name stays undated.
  The survey date must not follow the latest observation. When the latest
  `source` is itself `sextant-receipt`, those two dates must be equal; later Look
  or Enter observations may retain the receipt survey.
- This receipt alone permits strict daytime measurement with `flexible: false`.
  The flag still truthfully records the acquisition preference, including at
  night; it is not rewritten to justify the exception. Ordinary surveys in the
  same checkpoint retain their own nighttime/flexible rules. A receipt does not
  create naming evidence, authorize an interior coordinate, or act as a general
  source of daylight survey authority.

The V23 schema loads frozen V16–V22 siblings locally. Assert the version-aware
`ultima5-world-journal` format as well as the structural schema: the registration
uses `validateWorldJournal`, `validateChestAssessments`, and `validateOpenDoor`.
Atlas source/site/name authority is checked by `journal.ValidateAtlasLandmarks`;
native survey validation allows the daytime exception only for receipt survey
provenance. Codec/schema regressions are in
`internal/save_snapshot/migration_v23_test.go`. Installed-terrain compatibility
and next-turn restoration are tested separately in
`internal/game_state/save_snapshot_doors_test.go`.

### Snapshot 22 Measurement Policy Provenance

Snapshot 22 introduced a required concrete boolean `flexible` on every persisted
`SurveyedLocation`: the two sites in each of eight dungeon observations, each of
eight moonstone observations, and each of its at most 38 atlas landmarks (39 in
V23). The flag records the policy accepted **at acquisition**, including nighttime
acquisition with the flexible option enabled. It is never inferred from the
current configuration or hour.

- `measured: false` requires the entire survey to be zero, including
  `flexible: false`, location, and optional timestamp.
- Every measured survey requires a valid Britannian date and world map identity
  `map: 0`, `floor: 0`, with surface coordinates in 0–255. Flexible provenance
  does not authorize interior, Underworld, dungeon, or invalid-hour coordinates.
- `flexible: false` retains the original 19:00–05:59 measurement gate, except
  for V23's explicitly sourced Greyhaven receipt above.
  `flexible: true` allows any valid game hour 0–23. Date and chronology validation
  remain independent of user settings.
- The opt-in `GameplayConfiguration.FlexibleSextantUse` preference is outside the
  save authority. Disabling it before capture or hydration does not invalidate
  measurements previously earned under that policy.
- An interior sextant fix may measure only an already legitimately entered atlas
  landmark's canonical Britannia exterior anchor. It cannot create a landmark,
  reveal a name, replace naming provenance, or persist the interior fix itself.
  A later accepted local measurement must update the last observation date so
  `survey.recordedAt` is not later than `observedAt`.
- Moonstone private site/observer evidence may identify an interior or Underworld
  map, but those sites remain unmeasured. `MoonstoneContext` is unchanged: survey
  context must still prove a held sextant and a valid observer on the same plane.

The V22 schema loads frozen V16–V21 sibling schemas locally and replaces all
three nested survey families with the four-member survey shape. Fixed array
sizes, atlas limits, finite domains, and all non-survey authorities are unchanged.
Strict codec/schema tests reject missing, null, nonboolean, aliased, and unknown
policy members, and verify all 24 hours.

### Snapshot 21 Current Moonstone Placements

Version 21 adds the required boolean `originalSiteRetired` to each private
`moonstoneJournal` observation. It is sticky after a successful pickup or burial,
including burial at the original tile, or an actual observation of a different
site. Unknown observations remain entirely zero, including retirement `false`.
Capture and hydration preserve the field by value. Immutable original-site
reference metadata remains outside save state.

A survey belongs only to the current personally observed placement. Pickup, burial,
and site changes clear `survey` and `surveyContext` before any eligible measurement;
pickup never measures a destination. An ineligible revisit of the same placement
retains its existing survey. Current validation rejects surveys of carried stones,
unplaced stones, and previous sites. The detached Journal projection exposes
`MoonstoneEntry.OriginalSiteRetired`, leaves `ChartedOrigin` empty after retirement,
and never emits a historical survey. Atlas projects one stable neutral-key row per
phase: carried or reburied-unmeasured stones have no plot; a newly measured site
has only its current plot. Returning to original coordinates or reloading does
not resurrect the original marker or note.

The v21 schema loads frozen v20 and its v16-v19 dependencies locally, references
their unchanged finite domains, and overrides only the moonstone observation
shape. Chronicle, visit, atlas-landmark, dungeon, and chest limits are unchanged.
`AtlasEntry.Discovered` is a nonpersisted projection of personal evidence, separate
from supplied symbols, learned names, and optional genuine survey coordinates.

### Snapshot 20 World journal evidence

Version 20 adds required concrete evidence fields while preserving envelope 2
metadata, including playthrough and Doom-run identities:

- Every map unit has `surfaceChestAssessment` (`0` uninspected, `1` uncertain,
  `2` safe, `3` trapped). Only semantic surface chests may carry nonzero knowledge;
  safe/trapped certainty must agree with the physical chest number. Hydration
  restores the number before the assessment because assigning a number resets it.
- An active dungeon has exactly 512 `chestAssessments`, ordered floor/Y/X like
  `cells`. Non-chest cells must be uninspected; assessments never replace cells.
- Each of eight `dungeonJournal` observations has two required `sites`, surface
  then Underworld. Private recognition coordinates are separate from optional
  nighttime surface surveys and observation dates. Surveys identify canonical
  ordinary surface entrances; Doom and Underworld sites cannot be surveyed.
- Required `moonstoneJournal` contains eight `journal.MoonstoneObservation`
  records: first successful evidence, private last site and action context,
  placement status, dates, and optional survey with instrument context. Chart
  origins remain immutable references, separate from the v21 handling/relocation
  retirement flag.
- `oppression.password` and `journal.oppression.password` extend the knowledge
  and Chronicle catalogs. Current limits are 51 Chronicle entries and 62 visit
  changes.
- Required `atlasLandmarks` holds at most 38 unique
  `journal.AtlasLandmarkObservation` records. As with other optional record
  slices, `null` or `[]` means no earned observations; omission is invalid.
  Site keys are ordinary exterior locations 1–32 except Ararat (25), or physical
  shrine keys 101–107. Private surface site, observed category/source, separately
  learned name/provenance, optional dates, and survey/provenance remain distinct.
  Native validation checks finite identities, duplicate keys, source/naming
  consistency, dates, survey eligibility, and shrine anchors. Runtime compatibility
  checks ordinary exterior anchors against installed immutable world references.
  Capture and hydration clone this value-only slice. Legacy import starts empty; no visits, inventory, chart facts, or hidden locations grant
  earned atlas history. The static 34-point supplied map, current Avatar fix, terrain
  cache, and reader preferences are not persisted in this field.

Legacy import backfills only actually carried moonstones as `known` with
`firstSource: "possession"` and `originalSiteRetired: true`; raw world placements
never disclose private sites, dates, or surveys. Password knowledge is never
backfilled.

Load frozen sibling v16-v20 schemas locally. In addition to the existing Journal
formats, assert `ultima5-world-journal` using `validateWorldJournal` and
`validateChestAssessments`: these enforce evidence consistency, location/time
bounds, chronological ordering, physical chest state, and survey eligibility.
Runtime compatibility additionally checks moonstone sites and observers against
installed map/floor references. The opt-in `U5_NATIVE_SAVE_TEST_PATHS` platform
path list exercises read-only local saves through decode and real-reference
GameState hydration/recapture, compares original persisted base state, and writes
only temporary copies. No private save bytes or local paths belong in fixtures.

### Snapshot 19 Stones discovery

Version 19 adds the finite `journal` knowledge key `stones.melody` and the
`knowledge-learned` Chronicle subject `journal.stones.melody`. Version 19 bounds
are 50 Chronicle entries and 61 visit-change keys. The persisted identity is
independent of NPC names, rendered instructions, and melody progress. Normal
capture/hydration preserves the first disclosure's date/source and deduplicates
later teaching. Knowing the tune does not open the harpsichord passage.

There is **no Stones-specific data migration, converter, or backfill**. The
existing optional knowledge-list entry is enough: absent means **not learned**,
with no Stones Chronicle entry. Kenneth can teach it normally after loading.
Neither met-Kenneth flags nor an open passage grants missing knowledge.

### Snapshot 18 Resistance discovery

Version 18 adds the finite `journal` knowledge key `resistance.password` and
its `knowledge-learned` Chronicle subject `journal.resistance.password`.
Version 18 bounds are 49 Chronicle entries and 60 visit-change keys. All member
wire shapes are unchanged; loading never invents password knowledge.
The v18 schema loads both frozen sibling v16 and v17 resources locally.

### Snapshot 17 Journal extensions

`lastConversation` stores bounded presented exchanges (identity, known
speaker/location, Avatar input, normal/Runic passages) and a truncation flag.
It is scene-owned: core-only capture emits an empty conversation and scene
capture enriches it. No widget, engine, pending speech, or input is serialized.

`dungeonJournal` contains eight observation records in canonical dungeon order:
known identity, encountered world coordinates/floor, last observed open/sealed
state, and successful word use. GameState owns these observations; actual seal
authority remains separate. Hydration never infers discoveries from inventory
or map flags.

The Chronicle adds first-acquisition events for Crown, Sceptre, Amulet, three
Shards, Sandalwood Box, and Minoc odd keys. Version 17 bounds are 48 events and 59
change keys.

Load the v17 schema and its frozen sibling v16 resource locally. Assert the
existing Journal formats plus `ultima5-journal-conversation`, implemented by
`journal.Conversation.Validate`, for UTF-8 byte bounds and unique identities.

#### Playthrough-wide excerpts

This historical V17–V24 ownership model is superseded by
[save-owned excerpts](#snapshot-25-save-owned-excerpts). The external version-1
JSON format survives solely as a read-only compatibility input. Its optional
`archived` boolean defaults false and is preserved during import.

Archived entries retain their source and count against the same limits. Keeping
an archived identity restores it while preserving longest-speech and known-speaker
deduplication **within the active game**. Loading a checkpoint replaces the whole
collection instead of extending it. Show Archived is Journal-session state, not
persisted in checkpoints. Archive changes never modify Last Conversation or learned
Chronicle facts.

Last Conversation retains 256 exchanges/256 KiB including attribution, evicting
oldest complete exchanges with an indication. An individually oversized exchange
stops recording further text with feedback. Notebooks permit 512 excerpts/4 MiB;
overflow refuses capture rather than evicting kept passages. Later captures may
extend an exchange, but older copies cannot shorten it. Random conversation IDs
are presentation-only and do not consume gameplay RNG.

- Store semantic durable values, not runtime object layouts.
- Use stable IDs instead of pointers and display names.
- Rebuild derived state, services, controllers, maps, and caches.
- Make unsupported or omitted state explicit.
- Validate cross-field invariants before constructing runtime state.
- Keep native format versioning independent from legacy `SAVED.GAM` offsets.

## Current Shape

```go
type SaveSnapshot struct {
	Version      int
	Party        PartySnapshot
	Inventory    InventorySnapshot
	World        WorldSnapshot
	Progression  ProgressionSnapshot
	Effects      EffectsSnapshot
	Wind         WindSnapshot
	Random       RandomSnapshot
	Continuation ContinuationSnapshot
	Redux        ReduxSnapshot
	Presentation journal.Presentation
	LastConversation journal.Conversation
	DungeonJournal [8]journal.DungeonObservation
	MoonstoneJournal [8]journal.MoonstoneObservation
	AtlasLandmarks []journal.AtlasLandmarkObservation
	Dungeon      *DungeonSnapshot
	PartyVehicle *VehicleSnapshot
	MapUnits     []MapUnitSnapshot
	ItemStacks   []ItemStackSnapshot
	MapMutations []MapMutationSnapshot
	OpenDoor     *OpenDoorSnapshot
	BasementChestHistory [3]BasementChestDailyRecordSnapshot
	AtlasNotes   journal.AtlasNotesState
	AtlasFleet   journal.AtlasFleetState
	CodexRevelationComplete bool
}
```

Fixed collection sizes are 16 roster slots, 48 equipment entries, 48 spells, 8 scrolls/potions/reagents, 14 quest items, 8 moonstones, 8 shrines, 8 dungeons, 14 cleared dungeon-room bytes, 3 Shadowlords, 24 search-found bytes, and 512 raw cells for an active dungeon.

### Exact JSON Fields

`internal/save_snapshot/snapshot.go` owns field names/types, including the
checkpoint-local `presentation`, `lastConversation`, `dungeonJournal`,
`moonstoneJournal`, `atlasLandmarks`, `openDoor`, `basementChestHistory`,
`atlasNotes`, `atlasFleet`, and `codexRevelationComplete` values.
The current [JSON schema](../schemas/native-save-e2-s25.schema.json) and `codec.go`
own required/null/unknown-member rules;
`validation.go` and the version-specific validators own semantic
constraints. Use those executable definitions when changing the wire shape.
The sections below explain ownership, restoration, and non-obvious field
semantics rather than maintaining another exhaustive field list.

## Durable State

### Party

- All 16 roster slots with `occupied`, `name`, `gender`, `class`, `status`, `strength`, `dexterity`, `intelligence`, `currentMp`, `currentHp`, `maximumHp`, `experience`, `level`, `monthsAtInn`, `armorClass`, `helmet`, `armor`, `weapon`, `shield`, `ring`, `amulet`, and `partyMap`.
- Version 11 and later occupied records accept gender `0x0b` or `0x0c`, class `A`, `F`, `B`, or `M`, and status `G`, `P`, `C`, `S`, or `D`.
- `partyMap` preserves the original semantic byte: `0` is the packed active-party prefix, map IDs `1..40` identify companions staying at inns, `0x7f` identifies Blackthorn's martyrs, and `0xff` identifies unrecruited companions.
- Roster slot zero is occupied by an active Avatar-class character. The Avatar name remains player-defined. Active characters form a packed prefix of one to six records.
- Karma.
- Source-qualified NPC met/dead flags as location/NPC pairs.

### Inventory

- Food and gold.
- Keys, skull keys, gems, and torches.
- Equipment quantities.
- Spell, scroll, potion, and reagent quantities.
- Quest-item and moonstone quantities in authoritative catalog order.

### Position And Calendar

- Current and last-large-map positions as location, X, Y, and floor.
- Calendar year, month, day, hour, minute, and turn count.
- Elapsed milliseconds, last processed hour, last camp counter, last wanderer month, combat encounter sequence, and sailing state.

### Continuation

- Active-player selection as `-1` for none/all or an eligible active-party index
  from `0` through `5`. Selection is cleared if that character dies, sleeps, or
  reaches zero HP.
- Criminal status, which preserves hostile-town continuation across a native
  save even though the original game derived hostility differently.
- The HMS Cape automatic-sailing wait toggle, preserving which next wait phase
  skips world actors.
- The original byte-sized beggar reward cooldown, from `0` through `255`.
- The original opening-campaign encounter reduction until the first
  month rollover.
- The original modulo-ten combat action counter, preserving the next
  combat-time minute boundary across encounters and saves.

### Surface Actor Cadence

Version 15 introduced four required boolean phase bits in `effects`, retained
unchanged in version 16:
`townMountedActorSkip`, `townQuicknessActorSkip`, `outdoorMountedActorSkip`,
and the existing `quicknessWorldActorSkip`, now exclusively outdoor Quickness.
Each qualifying gate toggles its bit before testing it; `false` therefore means
the next qualifying gate skips actors. Earlier gates can prevent later gates
from being reached. Town Pass bypasses riding without toggling it; outdoor Pass
does not. Horse and carpet share riding phase. Board/Exit, inactive spells, and
calendar advancement do not reset the phases. A new town visit resets only the
town bits; outdoor bits survive map visits. Hydration restores all four after
map/controller initialization without running an ordinary entry transition.

The original keeps town-local mounted/Quickness phases and process-lifetime
outdoor mounted/Quickness phases; neither is in the original saved record.
See [TIME-0002](../gamefacts/TIME_COSTS.md#time-0002) for context ownership and
[SAVE-0005](../gamefacts/SAVED_GAM_STRUCTURE.md#save-0005) for saved allocation.
Fresh original imports therefore initialize all four native bits
to `false`. Native saves preserve them for deterministic continuation, including
dormant context and spell phases; the bits need not be false merely because the
party is currently on foot or Quickness is inactive.

### Progression

- Search-found bytes, skull-key found day, and three reagent-cache found days.
- Eight moonstones with X, Y, map, level, state, and found flag.
- Eight shrine active/completed/ruined records, eight dungeon entrance states, and the 14-byte cleared dungeon-room bitset.
- Three Shadowlord alive flags and persistent town assignments, last called identity, summoned state, and automatic-presence identity.
- Forty-eight known-recipe records with reagent masks.
- HMS Cape activation.

`searchFound` is the authoritative 24-byte Search state. Bits 0-112 are the
one-time flags for the 113 fixed Search records ([CMD-0007](../gamefacts/COMMANDS_REFERENCE.md#cmd-0007)). Starting at bit 113, the
remaining space stores active surface-corpse discovery order in four-bit slots;
zero terminates the list and nonzero values are one plus the corpse record's
source ordinal. There is no separate surface-corpse snapshot field. Hydration
restores these bytes and reconstructs the active corpses for only the current
map and floor. See [Search locations](search_locations.md#persistence).

### World

- Independent magical-light and carried-torch remaining minutes, the Rel Tym next-actor phase, active duration-spell type/turns, plus amulet, crown, and badge active/turn counters.
- Tavern intoxication continuation: uncapped nonnegative `drunk` and byte-sized `smashed` (`0..255`).
- Wind direction, update counter, and next elapsed-time tick.
- Deterministic RNG seed and source-call count.
- Optional party vehicle details: type, current/previous direction, skiffs, ownership, hull, sail state, HMS Cape, and required false `pendingDelivery` state.
- Persistent large-map and current-small-map units with controller world and processing order, kind, reference/NPC IDs, position, visibility, turn/autotalk/AI state, optional concrete NPC schedule state, semantic current path, optional vehicle details, instance-owned enemy HP, variant, facing, Shadowlord identity, and semantic surface chests.
- `aiOverrideProvenance` records why scheduled NPC AI was replaced: `none`,
  `automatic-astaroth`, `automatic-nosfentor`, `guard-hostility`, or
  `generic-flee`. Automatic causes are valid only on NPCs in the active location
  with matching automatic presence. Astaroth permits hostile AI or its own
  resulting flee state; Nosfentor and generic flee require flee AI; guard
  hostility requires hostile AI. Generic law/flee transitions replace an old
  cause, while Astaroth's own post-Talk flee explicitly retains its cause.
- A map-unit frigate or skiff may have `pendingDelivery: true` only while it is the single visible, party-owned delivery reserved at one of the four canonical Britannia main-floor berths. It retains the delivered defaults: 99 hull, furled sail, current and previous East direction, no HMS Cape, plus 2-12 carried skiffs for a frigate or zero for a standalone skiff. The marker activates in place when the party reaches a stable overworld boundary; friendly large-map vehicles are not distance-culled.
- `arrivedScheduleSlot` is `0`, `1`, or `2` and records the last schedule destination the loaded NPC physically reached; it is distinct from the slot currently desired by the clock. New captures always emit it, including an explicit `0`. Historical version-4 records may omit it (or contain `null`); hydration infers a slot only when saved position and floor exactly match a loaded schedule destination, otherwise it restores conservative closed slot `0`.
- `consumedTalk3ScheduleSlot` is omitted (or `null`) until the loaded NPC successfully consumes one `TALK3` action. It then identifies the exact slot `0`, `1`, or `2` changed to `WANDER3`. Hydration reapplies that mutation to the fresh loaded schedule copy without mutating shared reference data. Schedule resets and teleports change arrival state but do not clear this one-shot state.
- `surfaceChestNumber` preserves the original object-number byte (`0..255`),
  including its lock bit. Version 14 adds `kind: chest` for semantic chests that
  have no authored small-map NPC reference; historical `kind: npc` chest records
  remain readable. Explicit JSON `null` is invalid.
- Current-map item stacks with typed item identity and quantity.
- Both large-world maps and active small-map mutations for the map and override layers (`setTile` and `setObject`); inactive retained small-map caches are excluded. Lord British's solved passage uses its dedicated Redux field and is not duplicated as a generic mutation. Overworld Push records its loaded-buffer tile swaps as narrowly tracked transient writes: capture substitutes each write's pre-Push value while it remains unchanged, preserving actor coordinates and unrelated mutations without persisting the pushed arrangement.

### Small-Map Door And Gate Continuation

`GameState.captureMapMutations` scans every loaded floor of the active
small-map location, not just the current floor. Positive door overrides use
existing `setObject` mutations with location/floor/X/Y identity. Hydration builds
fresh floors and restores those overrides; subsequent same-visit reconstruction
retains the latest regular or magical closed-door variants. This also supports
unlocked secret-door overrides without requiring an ordinary source lock.
See [door visit semantics](../gamefacts/COMMANDS_REFERENCE.md#cmd-0006).

A physical departure discards the retained small-map cache; an outdoor checkpoint
cannot revive the previous visit. Existing saves benefit only from door overrides
actually captured; already-lost unlocks cannot be recovered. Legacy conversion
continues to start without native map mutations and does not infer unlock history.

Snapshot 23 makes a single timed opening saveable at an otherwise stable
boundary. `openDoor` stores its originating position, 0–2 remaining turns, and
exact regular/view closed variant. Capture substitutes that variant for the
temporary floor override in a detached mutation list without closing the live
door or advancing the timer. A matching closed override is valid; a conflicting
closed variant or raw timed-floor override is rejected.

Runtime compatibility resolves both persisted map layers against installed
terrain and requires the resulting closed tile to match `openDoor.closedTile`.
Hydration restores actors and persistent mutations before reinstating the timed
opening. A zero countdown is valid even when the final actor phase has left the
tile unoccupied; the next door-processing boundary performs the appropriate
close or occupancy deferral. Occupancy-only openness is derived from final
actors and does not create another timer or persisted floor mutation.

Scheduled gates remain derived from terrain and the restored clock via
`GetSmallMapTerrainTile`, not serialized gate toggles. Explicit stored terrain
overrides still take precedence. Earlier native snapshots and legacy imports
default the new timer to null rather than infer an opening from actors or tiles.
Continuation coverage is in `internal/game_state/save_snapshot_door_visit_test.go`
and `internal/game_state/save_snapshot_doors_test.go`; the codec/schema boundary
is covered by `internal/save_snapshot/migration_v23_test.go`.

### Tavern Intoxication And Presentation

Version 6 adds required `effects.drunk` and `effects.smashed`. `drunk` is an
uncapped nonnegative counter used by the Tavern fourth-drink warning;
`smashed` is the `0..255` forced-command counter used by town input. Capture and
hydration preserve both values together with RNG seed/source-call state, so the
next Tavern warning or smashed-command check resumes deterministically.

**REDUX DEVIATION:** The original keeps its Tavern drink counter only in process memory
and does not serialize it. Native v6 deliberately persists it for deterministic
Redux continuation. The original forced-command counter is durable at legacy `SAVED.GAM` offset
`0x03B1`, so its native persistence is not the same deviation. See
[MERCH-0013](../gamefacts/MERCHANT_SERVICES.md#merch-0013).

Tavern customer eligibility is scoped to one live merchant session and is not
durable. The meal table-food display is a transient `GameScene` overlay rather
than a map mutation: it is excluded from `mapMutations` and snapshots and clears
when the location, floor, or loaded map instance changes. Tavern dialogs,
visible topic input, paid-information transcript, approvals, result modals, and
receipts are also transient presentation.

### Active Dungeon

The required JSON member `dungeon` is null on surface maps and non-null exactly
while `world.position.location` is a dungeon. Its DTO stores the exact dungeon
identity, all 512 packed cells in floor/Y/X order, facing, cardinal or vertical
last direction, the optional wanderer's monster/floor/current and previous X/Y
plus ceiling state, and the next Quickness skip phase. The active command token,
command sequence, semantic revision, layered-map projection, renderer state, and
debug caches are not durable.

### Redux Extensions

The required, strict `redux` object was introduced in version 3 as the extension
point for Redux-specific native state. Version 7 added the required non-null
`playerKnowledge` array, version 8 added persistent Shadowlord assignments
outside Redux, version 9 added the required non-null `questJournal` array and
required map-unit `aiOverrideProvenance`, version 10 adds required boolean
`lordBritishPassageOpen`, and version 11 adds optional NPC-chest
`surfaceChestNumber` authority plus strict character identity and
Avatar-leading roster rules. Version 12 retains the same JSON shape and expands
the validated player-knowledge domain with discovery-gated Shadowlord and
shrine lore.
Version 13 removes `reconstructMapUnits` from JSON, retains the required
non-null `dungeonDiscoveries` array, and adds the required top-level
`continuation` object.
Version 14 adds the required opening-campaign and combat-action counters to
`continuation` and adds semantic `chest` map units.
Version 16 expands the finite lore/Chronicle families and requires event
`sourceName`; scene-owned History and tab visits live in the separate root
`presentation` object, not in gameplay `redux` authority.
Future durable Redux fields must be added explicitly under a later snapshot
version; unknown fields remain invalid.

`lordBritishPassageOpen` records the permanently solved Lord British castle
harpsichord passage independently from box ownership and generic map mutations.
Original-save import infers it from Wooden Box ownership or castle NPC-dead slot 31.

Legacy conversion may attach a private, non-JSON map-unit reconstruction hint
while also carrying supported semantic object overlays. Hydration retains authored small-map NPCs, applies
the overlays, and consumes the hint. `EncodeCanonical` rejects snapshots that
still carry it.

`dungeonDiscoveries` is a sparse cross-visit journal for source-backed hidden
doors and hidden pits. Each entry uses dungeon location, zero-based floor/X/Y,
and kind `hiddenDoor` or `hiddenPit`. Entries are unique by coordinate and
sorted by location, floor, Y, X, then kind for deterministic encoding. Runtime
compatibility validation verifies each identity against immutable dungeon data.
The active 512-cell snapshot remains exact and independent: hydration never
replays the journal into an active visit, while fresh dungeon preparation may
replay it when the persisted gameplay preference is enabled.

`playerKnowledge` is the durable generic category/key journal used by the
Ztats dossier and other authoritative discovery paths. Each entry contains:

- string `category` and `key` identity;
- a required `source` object with string `kind` and `displayName`;
- nullable `discoveredOn`, whose non-null Britannian date has integer `year`,
  `month`, and `day` fields;
- boolean `unread` read state; and
- nullable `spellFormula`, whose non-null form has string `state`, uint16
  `confirmedMask`, and a non-null uint16 `candidateMasks` array.

Version 7 introduced durable player knowledge, and versions 8 through 11 retain
frozen validation for hidden-spell knowledge only: category `spell` and
keys `an-ylem`, `in-xen-mani`, or `rel-xen-bet`. Source kind is `dialogue` with
a nonblank display name, `experimentation` with display name
`Experimentation`, or `migration` with display name
`Previously learned; date unknown`. Formula state is `uncertain` with no
confirmed mask and at least two unique nonzero eight-bit candidates, or
`confirmed` with the hidden spell's canonical nonzero eight-bit mask and an
empty non-null candidate array; the whole formula may instead be null. Entries
must form a non-null array, be unique by category/key, and be sorted by category
then key. A non-null date uses year 1-65535, month 1-13, and day 1-28.

Version 12 retains those spell rules and additionally accepts only these lore
identities:

- category `shadowlord` with key `faulinei.name`, `faulinei.association`,
  `astaroth.name`, `astaroth.association`, `nosfentor.name`, or
  `nosfentor.association`; and
- category `shrine` with key `honesty.mantra`, `compassion.mantra`,
  `valour.mantra`, `justice.mantra`, `sacrifice.mantra`, `honor.mantra`,
  `spirituality.mantra`, or `humility.mantra`.

In versions 12-15 each lore entry is independent and must use a named `dialogue`
source, a non-null authentic discovery date, `unread: false`, and a null `spellFormula`.
Snapshot validation does not infer one fact from another or require paired name
and association entries.

Version 16 additionally accepts category `journal` with exactly
`domain.falsehood`, `domain.hatred`, `domain.cowardice`, `flame.truth`,
`flame.love`, `flame.courage`, and `banishment.method`, and category `dungeon`
with exactly `deceit.word_of_power`, `despise.word_of_power`,
`destard.word_of_power`, `wrong.word_of_power`, `covetous.word_of_power`,
`shame.word_of_power`, `hythloth.word_of_power`, and `doom.word_of_power`.
The dungeon keys are persistence identities, not permission to display an
unestablished dungeon name or solve a clue. Goeth's projected note keeps
`AIPONI`, but associates it with Destard because the player identifies the dungeon
before his revelation. Sindar's `INFAMA` and the Codex's `VERAMOCOR` remain
unassociated until demonstrated use. This is projection behavior, not a change
to historical save formats. See
[Words of Power discoveries](../gamefacts/WORDS_OF_POWER.md#word-0004).
Current non-spell lore accepts named `dialogue`, `observation`, or `revelation`
provenance; the required
authentic discovery date, `unread: false`, and null formula rules remain.
Hidden-spell validation is unchanged. Standalone domains do not bind a named
Shadowlord row; explicit associations and Flame/method discoveries remain
separate. See [discovery boundaries](../systems/LINEAR_CONVERSATION_SYSTEM.md#action-boundary).

`questJournal` is an insertion-ordered semantic Chronicle, not quest-progress
authority. Version 9 initially supported kind `shadowlord-defeated` with subject
`faulinei`, `astaroth`, or `nosfentor`. Each event stores its one-based
contiguous `sequence`, nullable `occurredOn` Britannian date, stable source
`gameplay` or `migration`, and `acknowledged` state. It never stores rendered
prose or an arbitrary payload. Kind/subject pairs are unique. Live events are
date-stamped and initially unacknowledged; migration events are acknowledged
with a null date. Current virtue and Shadowlord dossier state remains projected
from shrine flags and `ShadowlordState.Alive`, never from Chronicle metadata.

Version 16 adds `knowledge-learned` with a finite non-spell `category.key` subject
and `codex-answer-learned` with one of the eight lowercase virtue identities
(`valour`, not `valor`). Discoveries require a date, `dialogue`, `observation`,
or `revelation` source, and nonblank `sourceName`. Defeat events retain their
earlier validation and require empty `sourceName`; the member itself is required
on every v16 event. At most 40 unique first-occurrence events exist in this scope:
29 lore facts (including eight Words of Power), eight Codex answers, and three
defeats. Word discoveries use kind `knowledge-learned` and subject
`dungeon.<name>.word_of_power`, not another event family. Repeated observations
preserve the first sequence/date/source. There is no retention eviction, authored
prose, general payload, or gameplay replay. Current hydration does not fill in
missing events, and V24 promotion adds none.

### Presentation

Version 16 requires a strict, renderer-independent root `presentation` value,
defined in `internal/journal`. It belongs to the scene, not live `GameState`.
The canonical quiet default for core capture, fresh games, and legacy conversion
is:

```json
{"history":"","visits":{"revision":0,"changes":[],"seen":[0,0,0]}}
```

`history` is canonical plain main-output text: LF between entries, double LF
for one explicit blank separator, no edge padding, and no blank whitespace-only
entries. It permits at most 64 nonblank entries, 4,096 UTF-8 bytes per logical
line, and 65,536 bytes total including separators. Valid UTF-8 is required;
controls other than structural LF and Unicode line/paragraph separators are
invalid. These are byte limits, not JSON-escaped size or character counts.
Saved values are rejected rather than normalized. The live collector's rune-safe
shortening and assembly rules are documented in
[Message Output](../systems/MESSAGE_OUTPUT_SYSTEM.md#journal-history).

Journal no longer offers a History reader. This field and its strict validation
remain for live main-output rail restoration and compatibility with supported
older saves; the UI removal changes neither the snapshot version nor retained text.

`visits` stores only finite change keys/revisions and three independent seen
baselines. Tab identities are Chronicle `0`, Virtues `1`, and Shadowlords `2`;
History has no visit metadata. Entry identities are Chronicle sequence `1..48`,
virtue index `0..7`, and fixed Shadowlord row `0..2`. `changes` is a required
non-null array of at most 59 keys, sorted and unique by `(tab, entry)`.
`seen` is a required three-element array. Revisions are uint64; each change is
positive and no greater than `revision`, and every seen baseline is no greater
than `revision`. Changes replace a key's
stamp, not its gameplay value. Text/date comparisons do not determine newness.

The scene freezes entry stars and the opening revision for each Journal session.
Dismissal acknowledges only visited tabs through that revision; new changes
after opening and unvisited tabs remain new. This is independent of existing
spell `Unread` and Chronicle `Acknowledged` semantics. Loading restores the
selected checkpoint's values, never merges departing-scene visits, and does not
replay a notification.

Core `CaptureSaveSnapshot` emits presentation defaults and core hydration does
not take ownership of live History or visits. `GameScene.CaptureSaveSnapshot`
enriches the authoritative capture and validates the complete value.
`NewGameSceneFromSaveSnapshot` restores History and cloned visits on the dormant
candidate. `RestoreJournalHistory` reflows the main rail without callback
mirroring, re-recording, or a second normalization; canonical source recaptures
exactly. Codecs/repositories preserve presentation without a renderer. A headless
tool that hydrates and recaptures a whole checkpoint must explicitly carry the
loaded `Presentation` forward instead of replacing it with core defaults.

Whole-scene equality remains strict, including `presentation`, for player slots/
Quick Save, named captures and rollback, and pre-Doom/
Return Before Doom checkpoints. Do not strip this field from full
`reflect.DeepEqual` comparisons. Core-only equality has the explicit default
semantics above. The active dialog, selection/search/reading cursor, notice,
pause owner, and rendering resources remain transient and are not serialized.

## Context-Dependent State

The following are not captured and make the active scene ineligible:

- Combat encounter state.
- Camping progression.
- Shrine and Codex subscenes.
- Pending encounters, transient command/town state, and combat-map or mini-map locations.
- An active dungeon command token. Stable dungeon command boundaries are eligible when matching active dungeon authority exists.
- Automatic sailing, whose orchestration belongs to an event-queue workflow; vehicle sail state remains durable independently.
- An in-progress moongate growth or shrink workflow. The authoritative 0-16
  stage is transient and is initialized from the loaded hour rather than
  serialized.
- Open dialogs, pending secondary input, blocked input, feedback, projectiles, visual/combat animations, dissolves, fades, flashes, shakes, debug presentation overrides, and mini-map scenes.
- Tavern customer-session eligibility, visible topic input/transcript, approvals, receipts, and the meal table-food overlay.

Initial save capture is rejected while unsupported transient contexts are active.
Deferred provision observations are processed before self-contained time-cost
workflows finish and when combat returns to exploration. Ordinary surface and
dungeon cycles observe them in their player-update phase; capture rejects an
outstanding observation defensively. There is no elapsed-hour backlog. The existing
`LastProcessedHour` runtime/snapshot field now records the hour observation replaced
by each positive time request and by a completed player update. Its wire shape and
accepted range are unchanged. A sailing interruption may leave that observation
for the next command; it is not silently discarded just to permit a save.

### Exact Capture Eligibility

`GameState.ValidateSaveSnapshotEligibility` requires initialized game references/configuration, RNG source, wind system, and search manager, and rejects:

- Active combat, camping, or shrine scenes.
- Active event-queue workflows.
- A retained waterfall continuation (`activeWaterfall`), including actor/contact
  handoffs between physical waterfall workflows. Capture remains ineligible even
  when no waterfall workflow is currently live.
- Pending overworld encounters or direct monster contact.
- Pending pirate attack presentation or an active surface command cycle.
- Active automatic sailing.
- A combat-map or mini-map player location.
- An active dungeon command cycle, or a dungeon location without matching active semantic authority.

`GameScene.ValidateSaveSnapshotEligibility` additionally rejects:

- Open or timed overlay dialogs, pending secondary input, or blocked scene input.
- Transient action feedback, visual/combat animations, automatic door animation, dissolves, fades, flashes, shakes, inversion, or X-ray vision.
- Active projectiles or a mini-map scene.
- Debug presentation overrides that hide units, moongates, quest items, item stacks, or ambient sounds.

The same eligibility is required for named capture and for capturing the automatic rollback before load. Ineligible loads fail before candidate construction or scene replacement.

An open Journal or atlas (including a note editor) retains the existing open-modal
capture restriction. Pending ordinary Output discovery notices are not modals or
save-restricted feedback effects and add no restriction to an otherwise eligible
scene; their transient queue is not saved.

## Transient And Derived State

Never include:

- `GameConfig` and `GameReferences` pointers.
- `SystemCallbacks` or functions.
- RNG implementation pointers.
- Event queue managers and queue contents.
- Search and environmental manager instances.
- AI controllers and path caches.
- Full loaded NPC schedule copies remain derived runtime state. Native saves
  persist only the concrete `consumedTalk3ScheduleSlot` identity needed to
  reapply the one-shot `TALK3` to `WANDER3` mutation to a fresh loaded copy.
- `LayeredMaps` base tiles.
- Dungeon command tokens/sequences, semantic revision, and retained top-down projections.
- Lighting caches and tint calculations.
- Dialog stack, keyboard state, or live text output buffers. The bounded plain
  History value and finite Journal visits above are the narrow durable exception,
  not serialized Output/widget instances or Talk transcripts.
- Audio players.
- Ebitengine images and rendering resources.
- Active projectiles, screen effects, fades, and dissolves.
- Real-time animation epochs.
- Debug Workbench session state and browser preferences.
- Tavern customer-session identity and all Tavern presentation, including the transient table-food overlay and paid-information transcript.

Deterministic continuation stores the runtime RNG seed and source-call count, never a `*rand.Rand` pointer.

The stream implementation is imported directly from
`github.com/bradhannah/berad-sdk/modules/berad-rng`. The existing algorithm, source-draw
counting (including rejection sampling), and two persisted fields are unchanged;
the extraction does not require a snapshot schema version change. Restore jumps
to the recorded source state without replaying gameplay or consuming extra draws.

Outdoor Blink window origins and quadrant retention are runtime-only gameplay
state, not serialized camera state. Every actual hydration initializes them from
the final loaded outdoor position and plane, following normal original startup.
This includes legacy import, player loads and Workbench loads/rollback. Saving alone
preserves live history. Thus the next Blink may differ after a load
at unchanged coordinates. Failed candidate construction/activation preserves the
old scene's window. No schema change is required for this explicit reset contract.
Temporary outdoor Push writes are already excluded from capture; initialization
expires only writes still owned by Push, never unrelated durable map mutations,
NPCs, objects, or progression. See [SPELL-0002](../gamefacts/MAGIC_SPELLS.md#spell-0002).

## Validation

### Implemented Invariants

- Current snapshot version must equal 25; V24 decode validates its frozen source version before promotion. Vacant roster slots must be zero-valued, occupied names are 1-9 bytes, and character numeric fields fit their runtime ranges. HP cannot exceed maximum HP and MP cannot exceed Intelligence. Occupied records accept only gender `0x0b`/`0x0c`, class `A`/`F`/`B`/`M`, status `G`/`P`/`C`/`S`/`D`, and `partyMap` `0`, `1..40`, `0x7f`, or `0xff`. Slot zero must be occupied, active, and Avatar class without requiring a literal Avatar name. Active characters form a packed prefix of at most six slots.
- Karma is 0-99. NPC flag pairs must be in range and unique within each met/dead collection. Native snapshots retain the original zero-based town index (`0..31`); hydration translates it to canonical runtime location IDs (`1..32`) and capture translates it back. See [SAVE-0005](../gamefacts/SAVED_GAM_STRUCTURE.md#save-0005).
- `continuation.activePlayerIndex` is `-1` or `0..5` and must identify an eligible active-party character when selected. `continuation.beggarKarmaCounter` is `0..255`; `continuation.combatActionTimeCounter` is `0..9`; all continuation booleans are required.
- Food/gold are at most 9999; gems, torches, keys, skull keys, equipment, spells, potions, reagents, quest items, and moonstones use their runtime maxima; scrolls are at most 9999.
- Calendar fields use year 1-65535, month 1-13, day 1-28, hour 0-23, and minute 0-59. Runtime counters cannot be negative where applicable.
- Moonstone state is `0`, `1`, or `0xff`; recipe masks fit eight reagents; Shadowlord identities are `-1` or 0-2. Each Shadowlord town assignment is unassigned `0` or town `1..8`; nonzero assignments are unique and defeated Shadowlords must be unassigned.
- Wind, duration/toggle, vehicle, position, map-unit, item, and mutation fields are range-checked. `effects.drunk` is nonnegative without an upper cap, while `effects.smashed` is `0..255`. Every serialized vehicle requires the boolean `pendingDelivery` member; a boarded party vehicle cannot be pending, at most one map-unit vehicle may be pending, and it must be a visible party-owned frigate or skiff in the Britannia overworld controller on the main floor with the canonical delivered defaults described above. Runtime compatibility additionally requires one of the four canonical berths. Pending state cannot persist once the party is already at a stable active-overworld boundary. Present map-unit `arrivedScheduleSlot` and `consumedTalk3ScheduleSlot` values are 0-2. Magical-light and carried-torch minutes cannot be negative, and automatic sailing must be false. The active-spell slot accepts only `P`, `N`, `T`, `Q`, or `C` with positive turns; an empty slot requires zero turns, and `L` is never valid in the active-spell slot. Unit controller identities and contiguous processing-order indexes, item-stack positions, and mutation position/layer keys must be unique.
- Unit kinds are `enemy`, `npc`, `vehicle`, or `chest`; vehicle units require vehicle details and chest units require a byte-range `surfaceChestNumber`. Version 9 and later units require a supported `aiOverrideProvenance`, and non-`none` values must match unit kind, overridden AI, active location, and automatic Shadowlord presence as described above. Historical NPC-backed chest state remains valid. Item stacks cannot be empty and quantities must be positive. Mutation kinds are `setTile` or `setObject`.
- The private reconstruction hint may accompany supported semantic overlays but
  cannot be canonically encoded.
- `redux.dungeonDiscoveries` is required and non-null. Entries must be sorted,
  coordinate-unique, in dungeon/floor/X/Y range, and use `hiddenDoor` or
  `hiddenPit`; runtime compatibility also verifies immutable source-cell kind.
- `redux.playerKnowledge` is required and non-null. Entries must be sorted and
  unique by category/key. Hidden-spell entries use the three supported keys,
  source values, optional authentic date, boolean unread state, and null,
  uncertain, or canonical confirmed formula detail described above. Shadowlord,
  shrine, journal, and dungeon lore use only their listed keys, named dialogue/observation/
  revelation provenance, a required authentic date, `unread: false`, and a null formula.
- `redux.questJournal` is required and non-null. Events use contiguous one-based
  insertion sequence, unique supported kind/subject pairs, supported provenance,
  and kind-specific authentic dates/source names. Migrated defeat events require
  null dates and acknowledged state; discovery events require dated named sources.
- `presentation`, its nested members, the canonical History byte/text rules,
  and finite visit keys/revisions are required and validated as described above.
- `redux.lordBritishPassageOpen` is required and boolean.
- `openDoor` is required and nullable; non-null values obey the active-small-map,
  countdown, exact closed-variant, and mutation-consistency rules above. Actor
  occupancy is not a condition of validity, including at zero remaining turns.
- Atlas observations have at most 39 unique finite site identities. Guardians and
  Greyhaven receipt authority obey the source/site/name/date restrictions above.
- `dungeon` is non-null exactly for dungeon world positions, its identity must match, and its cell array is exactly 512 bytes. Dungeon floor/X/Y, facing, last direction, optional wanderer monster/current/previous coordinates, party non-occupancy, and on-foot state are validated. Regular map units and map mutations cannot substitute for dungeon wanderer authority or derived projections.

Pure schema validation intentionally does not claim complete semantic validation of equipment compatibility, quest coherence, or a game-data fingerprint. Workbench list/get/load additionally validate locations, floors, map bounds, enemies, NPCs, items, and loaded-map mutation constraints against the active runtime references. A record that cannot be verified is reported incompatible with a reason and cannot be loaded.

### Required Schema Formats

The v16 standalone schema requires registered, **asserted** custom formats:
`ultima5-journal-history` delegates to `journal.ValidateHistory`,
`ultima5-journal-presentation` delegates to `journal.ValidatePresentation`, and
`ultima5-journal-events` enforces the finite event sequence/identity/provenance
rules. Merely loading the schema or treating `format` as annotation is not
equivalent validation. JSON Schema `maxLength` counts Unicode characters, not
UTF-8 bytes, and ordinary keywords cannot compare visit revisions. The Go/schema
parity fixtures and registration example are in
`internal/save_snapshot/journal_v16_test.go`; complete snapshot validation still
owns cross-authority consistency such as defeat versus current life state.
`internal/save_snapshot/dungeon_knowledge_v16_test.go` covers all eight word keys,
source/date preservation with or without existing Chronicle events, strict
Go/schema rejection of malformed lore. `internal/journal/presentation_test.go` asserts the complete
40-event/51-key bounds.

## Native Save Envelope

```go
type Envelope struct {
	Format          string
	EnvelopeVersion int
	Metadata        Metadata
	Snapshot        SaveSnapshot
}
```

The implemented metadata fields are `name`, `description`, UTC `createdAt`,
`gameVersion`, and optional `doomRunId`, `playthroughId`, and `userDescription`.
The format is `ultima5redux-native-snapshot`; only envelope version 2 is accepted.
Envelope-1 files are intentionally unsupported: no upgrade, identity backfill,
or name-based association is performed. This also invalidates prior named
Workbench captures, rollback records, and (retired) scenario snapshot baselines.
Reading or rejecting an old file never rewrites or deletes it. New original-save
imports remain supported and create fresh playthroughs.

Decoding rejects unknown or duplicate members, null/type violations, malformed
UTF-8 and unpaired Unicode surrogate escapes, and trailing values. Encoding uses
deterministic indented JSON plus a final newline, and the canonical bytes produce
the record's SHA-256 revision.

### Playthrough Metadata

`playthroughId` is 16 cryptographically random bytes encoded as 32 lowercase
hexadecimal characters. Generic snapshots can omit it; player repository writes
and ready/load eligibility require a valid nonempty identity. An otherwise valid
envelope-2 record without one remains inspectable as incompatible, preserving its
reviewed revision for explicit overwrite or deletion. Neither the codec nor
repository generates an identity.

The application generates one ID for a new game or accepted original-save import,
retains it through prepared-save retries, and preserves it when loading or saving
to other slots. Separate games with the same Avatar name remain distinct; Avatar
renames and loading an earlier checkpoint do not create a new playthrough. The
generator is injected independently of gameplay RNG and time.

`userDescription` is an optional, per-checkpoint player note of at most 80 Unicode
runes. Valid UTF-8, single-line text is required; controls and Unicode line and
paragraph separators are rejected. An absent or empty note means no description.
The in-game editor accepts at most 22 runes so a new note fits whole on its
Save/Load row; longer stored notes remain valid.
The editor trims surrounding whitespace on submission, but decoding does not
coerce saved text. Existing `description` remains independent system/developer
capture-purpose text with its own contract.

`GameScene` owns live playthrough metadata separately from active slot/revision
and Doom-run association. Application load paths explicitly install checkpoint
metadata on dormant candidates, since `GameState` hydration receives only the
gameplay snapshot. Failed writes retain the prior committed note and slot
authority; successful writes update both. A playthrough ID alone never grants
Quick Save authority over a player slot.

### Snapshot Version Compatibility

Within a valid envelope 2, strict decode accepts snapshot version 25
(`CurrentVersion`) and promotes version 24 (`MinimumVersion`, first shipped in
`v0.1.0-preview.1`). V24 retains all state; it only receives empty excerpts plus
the application-level notebook import described
[above](#snapshot-25-save-owned-excerpts). Promotion consumes no time or RNG and
never rewrites the source record. Named-record and slot revisions use the
promoted canonical current-version bytes.

Versions 23 and older, plus future versions, return a typed
`UnsupportedVersionError`. Player slots holding them list as incompatible with a
reviewed revision, so they can be overwritten or deleted. Original `SAVED.GAM`
import is unaffected and always produces the current version. New encoding
always emits version 25 and rejects the private reconstruction hint.

Metadata is not part of gameplay equality comparisons unless explicitly requested.

## Named Repositories

Records use `project:<id>`, `personal:<id>`, or `rollback:<id>` references. IDs contain lowercase letters, digits, and interior hyphens. Project and personal records are user-named immutable captures; rollback records are generated automatically before snapshot loads. Creation is atomic and no-overwrite, deletion is revision-conditional, and cooperating snapshot processes use one stable-order advisory lock set while establishing or removing snapshot references. Reads use descriptor-based no-follow opens on Unix. Personal and rollback roots/files use `0700`/`0600`; project roots/files remain collaborative at `0755`/`0644`. The Workbench does not edit or patch snapshot JSON in place.

Named captures and automatic rollbacks copy the source scene's playthrough ID
and player note independently of their developer/system description. Loading a
named or rollback checkpoint restores that metadata without granting player-slot
provenance. Explicit fresh
generic or INIT gameplay starts without identity create a fresh playthrough and
blank note at the application boundary, not as an old-player-save upgrade.

## Player Slots

The player-facing repository uses eight stable IDs, `slot-01` through
`slot-08`. Each slot stores one canonical native JSON envelope and is replaced
atomically under a reviewed SHA-256 revision. Directories and files use private
permissions. Listing classifies each slot independently as empty, ready,
incompatible, corrupt, or unavailable; a bad record never prevents healthy
slots from being selected. Ready, incompatible, and corrupt regular files can be
explicitly overwritten. Corrupt files remain unloadable: inspection exposes only
their raw-byte SHA-256 revision and private rollback bytes, never fabricated
gameplay metadata. Replacement rechecks those bytes under the transaction lock;
changed files require fresh review. Failed import activation restores the exact
previous bytes, including an empty corrupt file. This is deliberate replacement,
not old-save migration. Unavailable/nonregular files remain disabled. Delete
remains a repository capability but is not a player-facing browser action.

Journey Onward and Load choose the newest ready slot overall. Gameplay Save
initially selects the newest ready slot belonging to the current playthrough,
not merely the same Avatar name. Save falls back to the first empty slot; if
none exists and no matching ready save exists, the player must deliberately
select a destination. Equal timestamps use lowest slot number. A stable `*`
marks the applicable newest save, with its meaning in the footer; navigation
does not move that marker or reorder the eight slots.

Manual saving uses one combined optional-description/overwrite confirmation.
The same-playthrough destination's note is prefilled; empty and unrelated slots
start blank. Cross-playthrough overwrites are explicitly warned even if names
match. Corrupt destinations warn that unreadable data will be permanently
replaced and start with a blank description. Ctrl+U clears the line, Enter saves (including blank), and Escape returns
to the browser. Held Backspace deletes immediately, then after 21 update ticks
repeats every 3 ticks (approximately 350/50 ms at 60 TPS). Native composition owns
its editing; physical repeat, Unicode selection replacement, and focus cleanup
are tested separately. Native input failure retains the editor/text and displays
an error until explicit Escape rather than masquerading as cancellation.
Drafts survive failed submissions and stale-revision
refreshes, but fresh overwrite review remains mandatory.

Failed import commits additionally show a centered, user-dismissed error dialog
identifying the destination and underlying failure, including rollback errors.
The submitting Enter must be released before acknowledgement. Dismissal returns
to the selected slot with its description draft intact, without automatically
retrying. Source-file validation failures return to active-folder recovery with
Check Again and Back actions.

The workflow removes its exact editor and browser before snapshot capture so
the scene is representable. Failed capture/write reopens the browser with the
reviewed slot, draft, and error notice; success leaves it closed. Transient UI
never enters durable state. Quick Save preserves the last committed note,
including an intentionally cleared one. Escape-menu Quick Save remains
prompt-free and opens selection without slot provenance.

The **Q** command always uses the active player slot/revision established by the
last successful player-slot save/load in this scene. **Confirm Quick Save** defaults
On: Q asks `Save game?` (Y/N/Escape) and acknowledges success. Off performs the save
immediately and reports success in normal output without another key press. Both
settings show acknowledged errors; absent provenance explains that a player slot
must first be saved or loaded. Neither path changes the description or chooses a
slot automatically. Surface capture runs before surface command routing owns a
cycle. Dungeon Q completes its accepted precharged command-only token exactly
once before capture, retaining the Quickness phase without a full end-player
pass. Existing snapshot eligibility and optimistic revision protection remain
mandatory; combat Q cannot save.

**Ctrl+Q** directly reloads that same active slot and stored reviewed revision,
independent of the Q preference, without confirmation, slot selection, outgoing
save, or rollback capture. This Redux shortcut is available at normal exploration
input and player-controlled combat turns, including controlled monsters. Input
owners such as enemy actions, targeting, readers, modals and workflows block it;
incidental damage labels/action cards do not. Blocked/held chords are suppressed
through release, never deferred or interpreted as Q. Loading itself charges no
turn/time/resource/RNG. The canonical native loader constructs, preflights and
activates a dormant candidate before retiring the old scene, then publishes the
replacement and reports success there. Old finalizers run before publication.
Failures report in normal output and retain the old scene/generation, source input
and active provenance; changed, missing,
corrupt or incompatible checkpoints require ordinary reviewed Load instead of
silently adopting a new revision. No snapshot schema/version change is involved.

Rows use `Slot N`, Avatar, and a secondary note when measured space permits;
Details retains the full wrapped note. Main title and DETAILS use the Z-Stats
title-font role. Inset rectangular selection fills stay inside connected pane
interiors. Up/Down selects, Enter acts, Escape cancels; Page Up/Page Down have no
slot action. Left/Right detail guidance appears only for overflowing detail
content. Saved timestamps display in the computer's local timezone while storage
and newest ordering remain UTC-based. Building floors display basement/main/
ordinals, dungeon floors display one-based levels, and world-map names retain
Britannia/Underworld without an artificial floor label.

Returning to the main menu or exiting the application first presents Quick
Save, explicit Save, without-save, and cancel choices. Departure prompts and
slot browsers are removed before capture. Successful persistence continues to
the reviewed destination; cancellation returns to the departure prompt, and a
failure never discards the active gameplay scene.

Import from Legacy reads the exact `SAVED.GAM`/`SAVED.OOL` pair only from the
active game-data directory. Pending configuration changes, previous paths, and
provider-discovered installations are not fallback sources. There is no path
entry field. Missing or unusable pairs offer Check Again and Back, preserving
explicit recovery ownership. The detached importer requires the 512-byte `SAVED.OOL`
sibling, validates and decodes both, then converts once before native destination
selection. A dormant candidate is preflighted before the selected slot is
written. Source files are read-only. Active dungeon imports restore all 512
cells and stable semantic wanderer state; Redux never exports the original
binary formats.
Legacy bundle validation rejects unknown occupied-character gender, class,
status, and party/location marker values before conversion. Conversion copies gender, class,
and status unchanged, applies the original packed active-party prefix to
those markers ([SAVE-0002](../gamefacts/SAVED_GAM_STRUCTURE.md#save-0002)), and must satisfy the same current Avatar-leading identity contract
before returning a snapshot.
Conversion also reads original active-player selection and beggar reward cooldown; unavailable,
dead, sleeping, or zero-HP selections normalize to `-1` with a warning. The
converted snapshot carries transient reconstruction metadata only in memory.
Candidate preparation hydrates and immediately recaptures it so the first slot
write contains current canonical JSON with concrete authored NPCs and supported
semantic overlays.

`cmd/save-convert` provides a separate headless entry point with explicit
original-data, source-pair, and output selection. It uses real references and the
same hydrate/recapture boundary; it neither reads HOME configuration nor changes
the schema. Standalone output and player-slot output have separate destination
contracts, documented in the [CLI procedure](SAVE_CONVERTER.md). Converter success
does not establish an interactive load test.

Outdoor object conversion excludes records whose level disagrees with the
authoritative Britannia/Underworld list. Original non-combat lookup and drawing
ignore those records; moving them to another world or changing their level
would create incorrect occupancy. Each exclusion produces a source-path warning.
Small-map basement/upper-floor objects and the boarded party vehicle retain their
own state. The exact source rule is recorded in the
[legacy crosswalk](LEGACY_TO_NATIVE_SAVE_CROSSWALK.md).

Legacy effect conversion reads the durable active-effect identity and duration
fields ([SAVE-0007](../gamefacts/SAVED_GAM_STRUCTURE.md#save-0007)). Supported duration-spell codes become the native single duration slot,
and the Amulet, Crown, or Black Badge code becomes the matching toggle state.
The original spell/magic current-action flags are omitted with a stable
conversion warning because they are transient scratch state despite residing in
the original raw save block.

Legacy conversion reads the forced-command counter from `SAVED.GAM` offset `0x03B1` and writes it
to `effects.smashed`. No original save field exists for the Tavern drink counter, so direct
legacy startup initializes the runtime counter to zero and paired-file
conversion writes `effects.drunk: 0`, without a data-loss warning.

## Pre-Doom Recovery

The player-save root also owns one private `recovery/pre-doom.json` envelope,
outside the eight player slots. An accepted Doom transition atomically replaces
that record with the canonical snapshot captured immediately before entry. The
record and live Doom scene share an opaque `doomRunId` and playthrough identity;
ordinary saves made
inside that Doom run retain the metadata so a later load can claim only its
matching checkpoint. Saves outside Doom omit it, and absent or mismatched
metadata cannot authorize recovery. The recovery envelope also retains the
player note independently of its automatic checkpoint description.

The recovery repository uses the same private permissions, canonical encoding,
temporary-file publication, directory synchronization, and rollback guarantees
as player saves. Failure to prepare or replace this optional checkpoint does not
block Doom entry. Successful recovery hydrates and validates a fresh scene,
compares its recaptured canonical snapshot with the checkpoint, restores the
checkpoint's playthrough ID/player note, clears Doom-run and player-slot
provenance, and only then replaces the terminal finale scene.

## Capture And Hydration

### Capture

```text
verify save-safe boundary
        |
        v
read durable authoritative values on update thread
        |
        v
construct SaveSnapshot --> validate --> persist or return
```

### Hydration

```text
strict decode --> validate SaveSnapshot
        |
        v
create blank GameState with dependencies
        |
        v
apply durable values
        |
        v
rebuild maps, AI, search, lighting, environment, and callbacks
        |
        v
construct GameScene and atomically swap
```

Hydration uses a neutral reconstruction path and does not replay ordinary location-entry actions, dialogs, transitions, or progression effects. It rebuilds maps, controllers, deterministic systems, callbacks, and actual magical-light and carried-torch state around the validated values. For an active dungeon it restores world position, constructs `DungeonState` directly from the saved cells, installs the authoritative projection, restores facing/last direction/wanderer/Quickness phase, and leaves the current NPC controller nil. It restores the sparse discovery journal and cleared-room bitset separately and does not replay either into those exact active cells. A later fresh visit reads source `DUNGEON.DAT`, converts live room cells selected by the 14-byte progression bitset to cleared cells, and may apply the discovery journal according to the gameplay preference. Hydration itself never runs entry, transition, generation, time, RNG, or progression effects. The restored RNG is installed before environmental systems and controllers receive it; their reconstruction must not consume the saved continuation stream.

The display-only status rail reads detached
`GetWornArtifactStatuses` and `GetPersistentEffectStatuses` projections from
restored authority on its next draw. There are no persistent overlay
registrations or presentation hydration callbacks to replay. Crown and duration
spells are mutually exclusive: donning Crown clears the duration slot, activating
a duration spell removes Crown, and removing/expiring the replacement does not
resume the displaced state. Amulet and Badge remain independent; magical light
is independent of the Crown/duration slot. A worn Crown contributes both an
authoritative worn state and one steady Crown-sourced suppression effect, but the
rail renders the Crown once rather than duplicating that inherent benefit as an
Ankh. Normal artifact duration 255 is a permanent sentinel, not a countdown.
Restoration does not replay activation messages, sounds, flashes, or transient
VFX and adds no save fields.

The one-time legacy map-unit bridge is consumed during this reconstruction. It
is never copied into runtime state, so immediate recapture produces ordinary
current-version semantic map units with no reconstruction property.

## Current Limitations

- Active combat, camping, shrine scenes, workflows, dialogs, presentation effects, and other transient contexts are rejected rather than serialized.
- Item stacks are restorable only for the current map/floor because the runtime stack store is current-map scoped.
- Map mutations must target maps loaded by neutral reconstruction; arbitrary unloaded-map mutations are not supported.
- Combat encounter state, event queues, dialogs, input ownership, audio playback, rendering state, and transient presentation are not restored.
- Neither repository has an arbitrary snapshot editor, comparison engine, or
  patch format. Legacy import is available through the title menu and headless
  converter; there is no Workbench legacy-import command or legacy export.

## Deterministic Continuation

Snapshots preserve exact active-dungeon and pending ship-delivery continuation,
`drunk`/`smashed`, durable player knowledge, Shadowlord assignments, semantic
Journal events, AI override provenance, Lord British passage state, and surface
chest authority. Continuation also includes active-player selection, criminal
status, HMS Cape automatic-wait phase, beggar cooldown, opening-campaign encounter
reduction, modulo-ten combat action count, independent surface cadence phases,
finite discoveries, and checkpoint-owned Journal presentation. They preserve
carried-torch fuel, the Rel Tym actor phase, elapsed-time and wind tick phases,
enemy instance state, exact Shadowlord identity, controller processing order,
dungeon cells, facing, vertical arrival, optional occupancy, the dungeon
Quickness phase, the single reserved overworld delivery marker, the next
intoxication boundary, and deterministic midnight Shadowlord movement. Scene activation retains the hydrated elapsed-time base and wind system,
map/floor transitions clear the current-scope item-stack store, and Lord British
treasure generation uses the captured `GameState` random stream. Automatic
sailing is rejected rather than partially restored.

Coverage includes immediate capture/hydrate/recapture equality and fixed-seed
next-action parity across actor processing, enemy movement, wind advancement,
RNG source-call state, both dungeon Quickness phases, and dungeon Ignite.
Legacy-import omissions remain separate because they occur before native capture.
