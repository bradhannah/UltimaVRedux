# ADR-009: Canonical Save Snapshot

**Status**: Accepted

## Context

`GameState` mixes durable values with callbacks, services, controllers, caches,
closures, event queues, and transient presentation. It cannot serve as a wire
model. `GameState.RawSave` is populated during original-save import and is not
synchronized as play continues, so it cannot be save authority either.

Player saves, Workbench checkpoints, rollback, and test
fixtures all need the same restoration semantics. A separate debug-state format
would duplicate party, inventory, progression, and world rules and eventually
diverge from gameplay.

## Decision

Use one versioned renderer-independent production DTO, `SaveSnapshot`, as the
canonical durable-state representation. Original `SAVED.GAM`/`SAVED.OOL` import
is a read-only one-way conversion into that representation. Native JSON becomes
authoritative after import; there is no original-format export path.

### Separation Of Responsibilities

```text
original save pair --decode/convert--+
native envelope --------decode------+--> SaveSnapshot --> validate --> hydrate
current runtime --------capture-----+
```

- `internal/save_snapshot` owns DTOs, strict validation, native codec, and named
  snapshot storage. Codecs do not construct maps, controllers, or dialogs.
- `internal/legacy_save` owns original-pair decoding and conversion dispositions.
- `internal/game_state` owns capture/hydration with explicit runtime dependencies.
- Scene adapters enrich captures with bounded presentation and construct dormant
  candidates; `internal/player_save` owns revision-checked fixed player slots.

Runtime services are rebuilt from semantic values, not serialized. Private
original-import reconstruction hints must be consumed through hydration and
recapture before canonical native persistence. Do not introduce a parallel
`DebugState` or gameplay `ProfileState`.

### Stable Boundaries And Scene Replacement

Capture rejects contexts whose continuation cannot be restored semantically,
including active combat and in-flight input/workflows. Active dungeon exploration
is eligible only between exact-once command cycles. Mutable dungeon cells and
semantic occupants are durable; command tokens and renderers are not.

A timed small-map door is representable at a stable boundary by its originating
position, remaining turns, and exact closed variant. Persist that semantic
continuation separately from the temporary floor replacement: canonical map
mutations retain closed terrain, and hydration restores the opening after actors
and mutations. Occupancy-only openness is derived from those actors. Zero turns
does not require an occupant, because the actor phase can vacate the door before
the next close-processing boundary. This permits faithful next-turn continuation
without serializing sprite layers or relaxing in-flight workflow restrictions.

Workbench loads persist a rollback before
constructing a dormant candidate. Player-slot loads construct from the reviewed
slot revision. Candidate construction/activation failure leaves the old scene
active and disposes the failed candidate. The application-owned debug control
plane survives scene replacement. The Workbench's former scripted scenarios and
scenario baselines were retired; nothing patches arbitrary snapshot fields.

### Checkpoint Metadata And Player Reference Material

Playthrough identity groups checkpoints independently of slot, content revision,
Avatar name, and Doom run. It is envelope metadata, generated at explicit fresh-
playthrough boundaries independently of gameplay RNG. Player description is
separate from automatic/Workbench capture descriptions. Saving commits metadata
only on success; scene replacement transfers it explicitly.

Bounded plain Journal History and tab-visit revisions follow the checkpoint
without becoming quest authority or serializing widgets. Core capture supplies
quiet defaults; scene capture enriches and validates the whole snapshot. Scene
hydration restores it without replaying notices or recording the transcript
again. A headless rewrite must carry loaded presentation forward explicitly.
The Journal History reader has since been removed. Retaining the bounded source
is still justified by live output-rail restoration and supported-save compatibility;
no snapshot field/version changes are needed for that presentation simplification.

Outdoor Blink's retained map window is deliberately runtime-only gameplay state.
Normal original startup rebuilds it from loaded coordinates despite saving offsets.
All actual native hydration therefore initializes the window from the final loaded
position and plane, including player loads and Workbench load/rollback. Saving
alone preserves it. A failed candidate leaves the old scene's history untouched. This
bounded original-style reset can change the next Blink at unchanged coordinates;
it is not a promise of byte-for-byte runtime continuation. It needs no new DTO or
schema version and does not reset unrelated durable terrain, actors, or progression.

Last Conversation, dungeon observations and deliberately kept excerpts follow
the checkpoint. The original excerpt design used a playthrough-keyed external
notebook to retain notes across older loads. That decision is superseded: carrying
later clues and archived/deleted choices into earlier saves violates independent
saved games. Full text, attribution and archive flags now rewind together, even
when two saves share an identity. Excerpts remain reference material, never
gameplay knowledge. Active Talk and input/search/selection state remain transient.

A temporary read-only importer preserves available notes for old native saves
before scene activation. It cannot reconstruct historical per-save lists. The
next normal save embeds the imported collection; current saves never consult or
update the old notebook. Retaining the source allows each old checkpoint to import
independently, without a shared migration ledger. The compatibility entry point is
marked for removal when support for external-notebook saves is retired.

Player-authored atlas notes likewise follow their checkpoint: their recorded
position, creation date, text, stable ID and allocation watermark rewind together.
They are distinct from learned landmark surveys and captured conversation excerpts. A
captured note draft belongs to one authoritative state and cannot survive restore.
Player notes in My Notes follow their checkpoint in the same way: a reload rewinds
them with the rest of the save, and older saves start with a fresh notebook.
Fleet identity likewise follows each actual vehicle across parked/boarded forms,
with one checkpoint allocation watermark and no duplicate position/history store.
Registration requires a real eligible boat-use boundary; acquiring a sextant or
loading cannot manufacture prior boat use.

Codex Word of Power knowledge and acknowledgement of the complete revelation
are separate facts. Retain a completion boolean for the final acknowledged page,
enriching the existing Chronicle identity without moving its earlier word-learning
boundary or changing original provenance. Word/shrine flags do not prove that
the player saw the final paragraph, so completion stays false until an actual
revisit earns it.

Daily basement-chest history is independent of whether those objects are loaded.
Persist three optional full dates and explicit authored/generated chest lifetime
identity, rather than overloading permanent NPC-death flags or recognizing an
authored chest by a reused runtime slot. Hydration restores actual objects and
absence without becoming a replenishment trigger.

### Versioning

Envelope and snapshot formats are independent contracts. Their exact
supported versions, strict field requirements, migration behavior, and limits
belong in [Save Snapshot Schema](../details/SAVE_SNAPSHOT_SCHEMA.md), not a second
version ledger in this ADR. Unsupported versions are rejected rather than
guessed, and schema changes require deliberate compatibility decisions. Native
compatibility starts at the first publicly shipped snapshot version; earlier
pre-release snapshots are unsupported rather than carried as promotion code.

An extension to a closed knowledge or Chronicle catalog also requires a snapshot
version, even when JSON member shapes are unchanged: older validators cannot
interpret the newly learned fact. Promotion and import preserve prior knowledge
rather than inferring discoveries from reference data or completed dialogue flags.
This also applies to taught melodies: the stable knowledge identity and first
disclosure provenance persist, while playable instructions are projected by the
current presentation and performance progress remains separate gameplay state.

A frozen promotion DTO must also freeze nested types whose current runtime shape
evolves. A new required journal field must not retroactively become required in
an old save. Private observed sites, public measured coordinates, physical chest
state, and learned chest assessments are distinct authorities. Import preserves old
evidence without manufacturing surveys or inferring inspection from hidden trap
bits. Carried moonstones can establish possession-only recognition; immutable
charted origins and raw world placement cannot establish earned observations.
Atlas landmark history follows the same boundary: clone its bounded value slice
at capture and hydration, preserve independent naming and survey provenance, and
keep the immutable supplied chart and ephemeral current fix outside save state.

Moonstone handling permanently retires the personal original-site authority and
regional note for that phase in the player's checkpoint. Static supplied origin
artwork remains immutable decoration, not an earned destination. Persist retirement evidence separately
from immutable chart metadata: comparing current coordinates cannot distinguish
an untouched origin from a stone returned there after pickup. Surveys authorize
only the current observed placement, so pickup and relocation erase previous-site
measurement authority rather than maintaining a historical destination trail.
Legacy import proves retirement only from carried inventory; possession alone
never fabricates a personal visit or date.
Personal atlas discovery is a derived reader flag, independent of name knowledge,
supplied symbols, and survey eligibility, and therefore does not add save state.

Persist measurement-policy provenance with each earned survey, independently of
the user's current navigation preference. A flexible measurement remains earned
after the option is disabled or a checkpoint is reloaded. Validation uses only
the saved policy, date, location, and action evidence: ordinary original-policy
surveys remain nighttime-only, and both policies authorize only Britannia surface
coordinates. Neither live settings nor import can fabricate daytime authority.
Canonical settlement fixes do not create atlas visits or names.

Narrow acquisition exceptions require their own finite evidence, not a falsified
policy flag. A live Greyhaven sextant receipt can authorize only its canonical
exterior survey while preserving the prior Enter name and truthful measurement
preference. Saved source/site/date constraints distinguish that receipt from an
ordinary daytime observation; ownership cannot infer it.
Guardians recognition similarly uses a bounded unnamed Look identity rather than
granting arbitrary landmark names.

## Consequences

- All native consumers share validation and restoration semantics; developer
  tooling cannot silently weaken normal save rules.
- Detached candidates permit validation before touching live state.
- Canonical revisions make stale review rejection explicit. Named records are
  immutable; player slots are replaced atomically under reviewed revisions.
- Durable fields require deliberate classification, capture, and hydration.
- Eligibility is narrower than arbitrary runtime capture, and missing faithful
  continuation values can require an incompatible format change.
- Presentation and playthrough reference material need explicit ownership to
  avoid replay, false knowledge, or loss during checkpoint restoration.

## Alternatives Rejected

- **Serialize GameState:** services, pointers, callbacks, and closures are not
  safe durable values.
- **Use RawSave as authority:** it becomes stale after gameplay mutation and
  cannot represent native-only state reliably.
- **Separate debug format:** duplicates production semantics and creates
  synchronization problems.
- **Require legacy binary saves for all persistence:** the original layout cannot
  express every durable field, validation requirement, or metadata identity.

## References

- [Snapshot contract and capture/hydration](../details/SAVE_SNAPSHOT_SCHEMA.md)
- [Original-save conversion dispositions](../details/LEGACY_TO_NATIVE_SAVE_CROSSWALK.md)
- [Workbench snapshot replacement](../systems/DEBUG_WORKBENCH.md#save-snapshots)
