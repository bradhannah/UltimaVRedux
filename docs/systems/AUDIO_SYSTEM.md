# Audio System

**Purpose:** Redux integration, game policy, authored media and acceptance over
Berad Audio. [ADR-007](../ADR/ADR-007-audio-system-architecture.md) records callback
decoupling; [ADR-010](../ADR/ADR-010-reusable-audio-module.md) records SDK adoption.

## Current Architecture

Title/game scenes construct `internal/audio.GameAudioManager`, a Redux adapter over
published Berad Audio dependencies. Redux pins runtime/authoring
`v0.0.0-20261019225412-564225ae5d47` and tools
`v0.0.0-20261020115307-20020a7917b6`, without replacements or a sibling checkout.
These revisions trim unused experimental APIs while retaining the gameplay and
editor/catalog/probe workflows. Runtime voices expose Stop/Status; music and
audition retain their separate production transport controls. The SDK's own full-source gate covers dependency code;
Redux's gate covers the consuming adapters and application behavior.

| Owner | Responsibility |
| --- | --- |
| Redux `GameAudioManager` | Gameplay callbacks, preferences, world-source projection and diagnostics |
| SDK `Runtime` | SFX/preview transport, owners/voices, admission, gain, fades and retirement |
| SDK `MusicPlayer` | One finite transport, zero-gain onset, pause/seek/resume and replacement |
| SDK `AmbientController` | Desired-source reconciliation, budgets, releases, retries and completion |
| Redux `MusicManager` | Location/combat/sailing scheduling, interruptions and session history |
| Redux `AmbientPlanner` | JSON/world interpretation, grouping, source identities and gain projection |
| SDK `authoring` and tools | Document/history, analysis/calibration, source-bound transactions, persistence, terminal editor, catalog and probes |
| Redux `internal/audio/tooling` | Project validation, world labels, eligibility, calibration-family targets, sliders, import destinations and Help |

Reusable contracts and generic operation are maintained upstream:

- [Runtime contract](https://github.com/bradhannah/berad-sdk/blob/564225ae5d47/modules/berad-audio/README.md).
- [Authoring, analysis and publication](https://github.com/bradhannah/berad-sdk/blob/564225ae5d47/modules/berad-audio/docs/AUTHORING.md).
- [Editor controls and workflows](https://github.com/bradhannah/berad-sdk/blob/20020a7917b6/tools/berad-audio/docs/EDITOR.md).
- [Catalog contract](https://github.com/bradhannah/berad-sdk/blob/20020a7917b6/tools/berad-audio/docs/CATALOG.md)
  and [neutral quickstart](https://github.com/bradhannah/berad-sdk/blob/564225ae5d47/modules/berad-audio/docs/QUICKSTART.md).

These are current compatible guidance; `go.mod` remains the authority for the
application's consumed revisions. SDK owns reusable mechanism changes and tests;
this guide owns Redux's obligations and policy. There is no second runtime or
authoring implementation in production or test-only compatibility engines.

`Library.PlaybackLibrary()` caches one immutable playback projection per game
library. Authoring and playback currently retain separate encoded copies; managers
share the projection across lanes/scenes. `EncodedBytes` describes one projection's
unique payload, not total application heap. Manifest/media stay in
`internal/audio/assets`. They are embedded at build time, not hot-reloaded.

The adapter borrows one Ebitengine context. Its initialization validates library,
configuration, clock and required SDK capabilities. `Update` services presentation
even during modal/reader frames. Scene disposal is idempotent; workflow shutdown
precedes audio retirement. Closing lanes never closes the host context. Application
window persistence and Workbench shutdown finish before ordinary disposal, with
deferred disposal also covering early configuration failures.

## Integration Boundary

Core gameplay uses `GameState.SystemCallbacks.Audio` and semantic `SoundEffect`
identities, never imports `internal/audio` or a device backend. The callbacks expose:

```go
type AudioCallbacks struct {
    Play func(SoundEffect) *SoundBuilder
    Stop func(SoundEffect) *StopBuilder
    PlaySoundEffect func(SoundEffect)
    RequestPlayback func(SoundPlaybackRequest)
    RequestVoice func(SoundPlaybackRequest) SoundVoice
    RequestStop func(SoundStopRequest)
    UpdateLocationMusic func()
    PauseBackgroundMusic func()
    ResumeBackgroundMusic func()
}
```

`NewAudioCallbacks` requires ordinary playback, effect-wide stop, owned playback
and three music lifecycle functions, then supplies the builders/convenience call.
It and `NewSystemCallbacks` validate all nine capabilities without invoking or
rebuilding them. Missing wiring panics at construction, even with sound disabled.
Disabled sound or recoverable initialization failure may leave a nil scene audio
manager, but the adapter still wires every required callback. Gameplay does not
hide missing functions behind nil checks.

## Supported Usage

```go
gs.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundUnlock)
gs.SystemCallbacks.Audio.Play(game_state.SoundWind).Loop().Start()
gs.SystemCallbacks.Audio.Stop(game_state.SoundWind).FadeOut(500).Now()
```

For overlapping workflows, retain only their cleanup capability:

```go
var voice game_state.SoundVoice = game_state.SilentSoundVoice{}
sequence.First(func() {
    voice = gs.SystemCallbacks.Audio.Play(game_state.SoundEarthquake).
        FadeIn(500).StartOwned()
}).Finally(func() {
    voice.Stop(500)
})
```

The silent initial capability handles cancellation before the first step.
`StartOwned` cannot expose completion to gameplay or determine costs, time, RNG or
outcomes. Voice capabilities are transient and excluded from saves. Ordinary
`Start`/`Stop(effect)` retains effect-wide semantics; an owned voice cannot replace
an unrelated workflow or the application's ordinary burn/footfall slot.

`Start` emits exactly one request with effect, explicit one-shot/loop mode, volume
and fade-in milliseconds. The mode is authoritative, not another metadata flag.
Builder volume is clamped to [0,1]. Workflows own duration and dispatch Stop from
normal/Finally cleanup; timed playback and playback-side fade-out are not builder
features. Fade-out begins at the currently applied envelope.

## Sequencing With Effects

Audio and visuals are independent callbacks composed by the owning workflow.
There is no shake-with-sound callback. A blocking earthquake sequence may start
audio, run its party shake, then stop audio/reset offsets in `Finally`. Use
`TriggerFeedback` when the entire sequence is presentation-only and nonblocking;
feedback callbacks never own gameplay mutations.

## Sound Effects

`assets/library.json` owns recordings, exact paths, cue labels, modifiers and
intended audition triggers. `soundEffectMetadataMap` is an immutable projection,
not another authored catalog. A known `file: null` slot is intentional silence;
unknown IDs, broken references, malformed/duplicate JSON or missing non-null files
are errors. Invalid embedded data is a programming/build failure; external project
construction returns errors. Recordings and cue identities are distinct.

Redux schema-3 policy requires exhaustive taxonomy and routing. Recording acoustic
family describes content (or intended content of a silent slot); cue profiles
describe intent, phase/outcome, mix role, emitters and contexts. SFX/Music/Ambience
categories remain derived from game cue identities. Family mismatch is a review
finding, not proof of invalid audio. New empty editor slots begin `unreviewed`.
Do not relabel stand-ins to hide genuine intent/content mismatches.

### Committed semantic feedback

- Ordinary Get, Ready, Board/Exit and Ignite have no verified direct original cue.
  Redux reserves silent semantic slots at committed results. Spell activation,
  potion consumption and the actual potion effect are separate events. Visual-only
  `PartyStateBuilder` calls request no audio; explicit `WithSound(SoundNoEffect)`
  requests the dud cue.
- Lava/fireplace injury uses `SoundFireBurn`; mountain/trapdoor falls use
  `SoundFallingMinor`/`SoundFallingSevere`. The severity distinction is Redux
  presentation policy, rather than an original sound distinction. Availability
  comes from the current manifest; an empty slot remains silent until filled.
- Resolved dungeon fire/poison injuries use `SoundFireBurn`/`SoundPoison`.
  `SoundElectricalDamage` (**Electrical Injury**, `electrical_injury_01`) is a
  dedicated, intentionally empty environmental-injury slot, not sustained field
  ambience. Its provisional cue gain is 0.8; merely viewing/entering a field is not
  the committed injury trigger.
- `SoundDispel` (**Dispel Field**, `field_dispel_01`) is a dedicated empty magic
  slot. Successful dungeon Sceptre/An Grav removal requests it once; cancelled or
  ineffective attempts do not. Combat An Grav shares it. `SoundAbsorb` retains
  `healing_02` and its provenance/calibration. The intended future sound is a short
  field-collapse fizzle with restrained tail, separate from spell failure; 0.8 gain
  is provisional. Search by Sceptre/An Grav, field removal or dungeon.
- Morning Star/Halberd commitment uses `SoundCombatSwing` for hits, misses and empty
  attacks at range 1/2. Bows retain `SoundCombatRangedLaunch`. Missile type still
  governs resolution, not a travelling reach-weapon graphic. Redux uses physical
  swing foley instead of the original synthesized glide; see
  [COMBAT-0004](../gamefacts/COMBAT.md#combat-0004). `combat_swing_01` retains
  XxChr0nosxX's CC0 [Swing.mp3](https://freesound.org/people/XxChr0nosxX/sounds/268227/)
  and its 350–910 ms window. Correct dispatch/decoding is not listening acceptance.
- Creature interference requests `SoundCombatInterrupted` once when its committed
  `CombatEventInterruption` enters the Battle Report. Preview/render/history do not
  request audio. `combat_interrupted_01` is intentionally empty, combat/failure
  classified, and requires no later gameplay rewiring to fill. The original prints
  interruption text without audio; see [COMBAT-0012](../gamefacts/COMBAT.md#combat-0012).

New silent slots retain truthful unknown provenance, empty processing, no window
or calibration. Their how-to-hear text describes intent, not audible acceptance.

### Spell cast cues

Prepared spells request `SoundMagic` with their success presentation unless
listed below. Noncombat **No effect!**, failed Blink/dispel/door spells and
no-room summons request `SoundSpellFailure`; a confirmed but ineffective combat
party spell keeps `SoundMagic`.

| Spell | Cue |
|---|---|
| Mani / Vas Mani / An Nox / An Zu | `SoundHeal` / `SoundGreatHeal` / `SoundCure` / `SoundAwakenSpell` with the target flash |
| In Mani Corp | `SoundResurrect`; `SoundMagic` for **Not dead** |
| In Xen Mani | `SoundCreateFood` with the party flash after the food is added |
| In Wis | `SoundLocate` before the Runic coordinate overlay |
| Wis Quas, Wis An Ylem, In Quas Wis | `SoundXRay` |
| An Sanct / An Ex Por / In Ex Por | `SoundUnlock` / `SoundMagicLock` / `SoundMagicUnlock` at the mutation |
| An Grav | `SoundDispel` (intentionally empty slot) on successful removal |
| In Vas Por Ylem | `SoundEarthquake` with the one-second shake |
| In Zu, In Nox Hur, In Vas Grav Corp, In Flam Hur | `SoundWhiteNoiseDamage` when the cone starts, even if it later proves ineffective |
| Combat missiles (Grav Por, Vas Flam, Xen Corp, An Xen Ex, Rel Xen Bet, In Quas Xen) | `SoundMagic` at launch, then `SoundSpellImpact` on an unblocked non-field impact |

Kal Xen Corp binding failure is shown with **Oops...** feedback and a hostile
allegiance cue; it has no distinct failure sound.

### Admission, footsteps and injury lifetimes

Redux configures eight SFX voices. SDK prepares replacements before admission;
unknown/failed/dropped/muted finite requests do not advance variant selection.
Finite pressure may evict the oldest finite voice; protected loops remain. A new
loop reports capacity failure when all slots are loops; ordinary dropped effects
are observational diagnostics. Explicit Stop reaches all owners for its effect.
Loops preserve continuity under category mute. `Update` owns servicing, not a
cleanup goroutine or gameplay timer.

Metal-grate movement requests `SoundWalkMetal1`, which cycles four metadata clips
sharing `sounds/footsteps/stone_steps_01.ogg`: `stone_steps_01` 40–260 ms,
`_02` 560–790 ms, `_03` 1060–1270 ms, `_04` 1570–1780 ms. Each step replaces only
the preceding ordinary grate voice. Brick/stone retains its four concrete-file
assignments; grass alternates and wood cycles using Redux's shared step sequence.
No gameplay RNG is consumed. A new recording without a window inherits no old trim.
The measured clips isolate four bursts, not perceptual approval as metal sounds.

`SoundFireBurn` shares credited source bytes with sustained fire but has a separate
0–1000 ms recording window and 200 ms release tail. Retrigger replaces its ordinary
slot, including while fading; owned burn voices, campfire loops and proximity fire
remain independent. Requested fade-in is preserved. Ambient fire keeps its full
source window. This is game injury-feedback policy over SDK window/tail mechanisms.

## Music

`MusicManager` owns scheduling over SDK finite transport using a presentation clock.
It never reads/advances gameplay time or consumes gameplay randomness.

| Provisional treatment | Value |
| --- | --- |
| First exploration presentation of a theme per application session | −10 dB relative to calibrated track/category |
| Later presentations | −16 dB, noncumulative |
| Quiet interval after completion/switching away | 5 minutes of unsuppressed presentation time |
| Score onset/location-switch release | 2 seconds; one voice, no crossfade |
| Local-music release grace | 3 seconds |

These values are code-authored, not listening-accepted preferences. Floors and
locations sharing a theme retain player, position, gain and quiet interval. A new
theme fades the old one out and observes global quiet time. Returning before fade
completion reverses the envelope without restart. Rapid travel cannot chain
introductions; absence does not reset heard-theme history.

Menu, journal, mute/disable, cinematic and nested workflow owners independently
suppress score and freeze scheduling. Cinematic owners include isolated overlay
scenes, Shadowlord presentations and finale; routine action workflows do not duck
music. Audio continues servicing modal/atlas/reader frames without advancing game
queues. Title services its own manager.

Bard proximity voices, including release tails, and playing harpsichord/lute voices
take precedence. Missing/muted local recordings claim no ownership. The release
grace avoids boundary chatter. Restoration fades from silence at saved position;
changed locations discard stale score and return through quiet time. Releasing one
hold never releases another. Settings preview auditions independently under the
menu hold, restores score position even after a same-track preview, and never
advances exploration history. Named special cues may bypass quiet, play once at
the subdued initial level and restore current exploration through quiet time.

Combat selects `routing.music["context/combat"]` (initially **Engagement and Malee**),
once per authoritative encounter, bypassing exploration quiet/repeat history. Gain
follows encounter role, not track name. Surface/dungeon/camping fights share the
policy. Per-frame/turn updates never restart it; completion leaves the battlefield
quiet. Victory stops score promptly and uses the short `SoundCombatVictory`, not
the music victory track. Menu/mute/workflow owners still apply; exit restores the
actual destination via quiet time. Tactical maps do not discover world bards.

Frigate sail hoists select `routing.naval.sailing` (initially Hornpipe) once per
hoist. Movement/turning/interrupted sailing never restart it. Completion stays
silent until furl/new hoist. Furling/leaving fades out; a new hoist bypasses quiet.
Combat takes precedence and a consumed hoist does not replay afterward. Missing or
failed starts consume that hoist, avoiding frame retries. Skiffs never sail. Pirate
locations no longer choose Hornpipe merely by entry. Onboard ambience is separate.

Detached `MusicHistory` survives load/new-game/menu scene replacement: heard themes,
remaining quiet and explicit on/off preference transfer; live transports, pending
cues and interruption owners do not. Replacing active score starts full quiet time.
Failed scene candidates cannot mutate retained history. History is not saved to
disk and resets on application restart. A quiet interval is not music being off.
Exploration failures defer retry for a full quiet interval; combat/hoist failures
consume their presentation. Actual device completion, not wall-time estimates,
ends finite music. Controlled policy/adapter tests are not native listening proof.

## Ambient And Positional Audio

`AmbientPlanner` derives definitions from the supplied game library. It coalesces
static sources to the strongest audible source per cue after source range/level
overrides; ties prefer nearer sources. Switching winner changes gain/range without
restart. Authored cue/calibration are removed from projected source multipliers so
SDK applies them exactly once. Relative level up to 200% is preserved. Plans carry
game emitter roles, not engine semantics.

`GameScene.updateAmbientSounds` gathers terrain plus visible moongates/seated bards.
Occupants do not hide terrain. Scan radius includes authored overrides. A dynamic
source names only its cue, position, emitter role, intensity and whether it is a
background; the planner takes the recording, radius, level and fades from the
cue. `EmitterAmbientSource` resolves the bard and moongate cues. Dynamic IDs
bind map, floor, emitter and canonical position before projection. Missing explicit
IDs retain emitter/cue/position fallback. Crossing a seam changes attenuation without
duplicating voices; bard precedence follows role, not ID spelling.

World/dungeon discovery wraps actual map dimensions; towns/combat stay bounded.
Each physical tile is visited once even if radius exceeds a dungeon grid. Projection
uses the nearest image and negative displacement for an exact half-map tie.
Audio calculations remain planar. Radius-zero sources ignore coordinates and use
stable cue-owned background identities.

Explicit empty source snapshots mean departure; omitted discovery retains the last
desired snapshot while `Update` services releases/failures/completion/retries.
Disable/suppression clears desired/retry state; restoration requires fresh discovery.
Re-entry reverses release continuously. Finite sources complete once per presence
or asset epoch. SDK retries at 1,2,4,8,16 then 30 seconds; repeated snapshots do not
reset backoff and long gaps allow one attempt. Failed replacement retains its old
voice; intentional silence retires without retries. Diagnostics do not drive policy.

Redux configures **16 active-or-releasing ambient voices**, separate from eight SFX,
finite music and one settings ambience preview. SDK ranks source/calibration/distance
mix, retained voices on exact ties, then stable ID. Category mute preserves ranking
and loop continuity. Release tails occupy capacity; backoff/completed sources do not.
Historical old-controller benchmarks do not characterize current SDK performance.

### Authored Routing And Animation Families

| Routing field | Redux meaning |
| --- | --- |
| `ambient` | Cue → radius (0 background, positive proximity), fades in milliseconds; radius ≤32, fades ≤60000 |
| `tiles` | Canonical tile → ambience/surface and optional positive radius ≤32 / relative gain 0–2; omissions inherit |
| `surfaces`, `defaultSurface` | Ordered footstep cues and fallback; empty sequence is silent |
| `music` | Location/world/context → music or null silence; missing target inherits required default |
| `dungeon` | Dungeon visit → radius-zero ambience or null; missing target inherits required default |
| `naval` | Required frigate/skiff onboard background and sailing music situations; null is silence |
| `emitters` | Bard/moongate → spatial ambience; code owns activity |

These meanings are consumer policy over retained schema-3 wire names. Redux
validates category, vocabulary, target completeness and eligibility. SDK structural
validation does not invent world targets. Missing/inherit and null/silence stay distinct.

Atlas animation families come from `internal/sprites/indexes/animations.go`.
Fountain 216–219 shares canonical 216; member overrides are rejected. Frame changes
do not change the map tile or restart audio. Torch/sconce/brazier may independently
override range/level while sharing Fire bytes. Removing ambience removes obsolete
overrides. Pixel effects, directional sprites, streams and magical fields are not
automatically atlas-frame families. Fountain ambience (`fountain_gurgle_01`) and
drinking (`fountain_drink_01`) remain separate cues.

Overworld/Underworld have distinct music targets despite a shared gameplay location
ID. Dungeon beds belong to the visit, including tactical rooms whose pre-combat
location is that dungeon. Movement/turns/floors/combat retain one cue-keyed loop.
Nearby tile effects and encounter score coexist; surface/town/Underworld exploration
do not inherit it. Default is silent; cave/mine Despise, Destard, Doom, Shame,
Hythloth select `ambient/dungeon_cave` / `dungeon_cave_01` (0.22 provisional gain);
brick Deceit, Wrong, Covetous select `ambient/dungeon_brick` / `dungeon_brick_01`
(0.16). These prepared slots are empty, with 1500 ms onset/1200 ms release when
filled. Select the corresponding Moment/Recording, import truthful media, calibrate,
Save and rebuild; no gameplay rewiring is needed.

Fogsbane, Stormcrow, Greyhaven and Waveguide use `AmbientLighthouse`/`seagulls_01`
throughout small-map floors at full spatial gain. Interior and static coastal tile
share one voice. Exit resumes ordinary tile proximity if audible, otherwise releases.
The overworld radius remains 10; Ambience, mute and suppression apply normally.

All seated tavern bards share `tavern_lute_01` through `ambient/AmbientBard` and
`SoundTavernBardMusic`. Its intentionally empty file claims no score ownership until
filled. Camping retains `SoundBardLute` / `bard_lute_01`. Future tavern material needs
loop-seam/extended listening acceptance. Frigate/skiff onboard Moments also have
prepared recording slots and loop through movement/sail toggles until departure or
vessel change. Sailing music and per-step movement effects remain independent.

Spatial attenuation belongs to the ambient source planner's authored radius and
distance profile. The profile is Berad Audio's `QuadraticDistanceGain`: full gain
within one tile, `1 - (distance / radius)^2` beyond it, silence at the radius, and
a radius of 0 is a non-spatial background bed. Redux keeps source selection: the
strongest tile per cue, bard and moongate sources and the lighthouse. Semantic SFX
requests supply gain directly; neither lane applies stereo positioning or occlusion.

## Volume Settings

Player sliders 0–10 use `ConvertSliderToVolume` once; 10 currently caps at 0.75.
Request, contextual, positional, cue and source factors remain continuous linear
amplitude. Recording calibration uses `10^(dB/20)` in [-48,+12] dB; missing is
unreviewed unity, not accepted zero. Zero-amplitude mute is separate from finite dB.
There is no master-volume preference. SDK saturates boosted PCM, not a multi-voice
limiter. Category changes preserve transport, request/context factors and envelopes;
configuration Cancel restores prior preferences without rewriting authored gains.

## Standalone Audio Editor

The Redux launcher reads the real disk manifest independently of the game/Workbench:

```sh
make audio-editor-run
# Build only, then launch explicitly:
make audio-editor
./bin/audio-editor
# Or:
go run ./cmd/audio-editor
```

`-library /path/to/assets/library.json` selects another Redux-valid project. `-check`
validates without playback/UI/writes. Generic SDK commands instead require an explicit
library/project and supply neutral policies; use the Redux launcher for this preset.
The SDK editor guide linked above owns common keys, drafts, variants, waveform,
Soundscape, source verification and safe quit. The browser Workbench is separate.

### Redux authoring policy

Tiles shows canonical atlas families; Locations exposes game music, dungeon, emitter
and Naval targets. Dungeon/onboard targets accept radius-zero ambience; tiles/emitters
require positive radius; Sailing accepts Music. Game source discovery, passability,
combat and sail eligibility do not move into the editor. Usages describe configured
types/assignments, not placed-map counts or proven trigger reachability.

For onboard sounds, fill **Frigate Ambience**/**Skiff Ambience** in Moments, then
review Naval assignments in Locations. Sailing initially selects Hornpipe, once per
hoist; editor audition does not simulate that scheduling. Shared recording edits
affect every use; change one cue's assignment to replace only that event.

### Game-Matched Preview Volumes

The default **Game Mix** label uses temporary Music/SFX/Ambience monitoring sliders,
initially 7 each, and the game's curve (7 ×0.55, 10 ×0.75, 0 mute). These are comparison
defaults, not player defaults. Cue audition composes calibration × cue × category at
unity request/context/distance; it does not simulate runtime fades or suppression.
Bare Recordings use linked-use category, falling back for mixed uses to SFX;
unassigned music paths use Music, ambience families use Ambience, otherwise SFX.
Explicit recording-bus override never changes a Moment's category. Reference bypasses
cue/category while retaining calibration and temporary mix-layer controls. Preview
never reads/writes player settings or saves its monitoring state.

### Auto-Gain And Manual Overrides

Redux supplies sustained-estimator targets −23 for score/instrument and −26 for
ambience/nature/fire; other families use active-RMS −20. These editable starting
targets are estimated, not a universal LUFS standard or listening-approved mix.
SDK owns estimator limits and source/revision-bound approval. Manual stays locked
across region/source changes until explicit AUTO confirmation. Calibration is shared
recording treatment; use cue gain for a quieter particular event, not the category
preference. Neither saved AUTO nor Manual constitutes listening acceptance.

External imports go to `sounds/imported/` or `music/imported/` for music replacements,
with content-qualified names. Unknown provenance stays explicit. Undo/Quit retain
copied files for Git review. SDK owns publication locking, external-change conflicts,
retained-file receipts and retryable durability state. Do not remove publisher-lock
inodes while editors are active. Review pending durability independently of authored
dirty state. Save changes disk/preview immediately; rebuild/restart to embed edits.

## In-Game Configuration

`ConfigurationDialog` owns detached Sound/Display/Gameplay settings. Sound previews
through the scene adapter; Apply persists the transaction and Cancel reverts preview.
The scene removes the exact modal and restores its parent policy. Game route starts
on Sound over paused Escape menu; title starts on Display with title music held.
Tab changes sections, arrows select/adjust, Enter applies, Escape reverts. Dimensions
stay stable and player input remains keyboard-only. Display confirmation is covered
by [Modal Dialog System](../ui/MODAL_DIALOG_SYSTEM.md).

Configuration → Sound → **D** captures a detached source/gain diagnostic report.
Arrows/PgUp/PgDown/Home/End scroll, R recaptures and D/Esc returns. Capture never
services audio or advances selection/time; playback continues. Slider/section actions
cannot mutate configuration behind the report. State is labelled captured, not live.

## Failure Policy

Recoverable initialization/preparation/device-stream failures return wrapped causes
and are logged by the presentation adapter. SDK owns decoder cleanup/read barriers,
finite failure reporting and transactional replacement. Empty recordings/disabled
sound are supported silence; missing non-null embedded assets are broken data.
Repair requires a newly validated library, not retargeting frozen streams. Music
failures retain quiet/encounter/hoist policy rather than frame retries. No infinite
silent stream substitutes for failed decoding. Ebitengine context/device errors can
still surface through its engine hook after construction and terminate `RunGame`;
successful construction is not device-readiness or fail-soft acceptance.

### Native probes and measured acceptance scope

```sh
go run ./cmd/audio-editor --probe healing_01
go run ./cmd/audio-editor --probe-runtime sfx/SoundHeal
go run ./cmd/audio-editor --probe-runtime-soak sfx/SoundHeal
```

Standalone audition owns Oto in its separate process; runtime probe owns a real
Ebitengine loop. Oto permits one context per process; do not combine standalone
audition and the game's independently created device context. Check/probe modes
are mutually exclusive. Probes measure progress/release, not complete listening.

Historical macOS healing-cue probes passed and the maintainer reported clean sound.
A one-minute muted lifecycle run completed 281 cycles/1,124 transports, zero active
voices, retained heap 65,055,424→63,493,600 bytes, about 355 MiB peak tracked RSS and
no surviving children. Those observations remain bound to their original revision,
cue/device/backend; extraction and controlled tests do not renew them for every pin.
The accelerated synthetic lifecycle/heap regression now lives in SDK and exercises
the SDK public API. Old-engine benchmark/soak measurements are not current acceptance.

Editor PTY render/quit, raw terminal-decoder tests and controlled transport tests
are bounded evidence. Physical terminal-key acceptance remains owner-deferred:
modified/ordinary arrows, held repeat, pause/crop/resume, crossed bounds, cancelled
imports, Save/Undo/Redo and safe Quit need hands-on observation. Broader native
Windows/Linux playback, long sessions, device failures, loop seams and multi-voice
headroom require their own proof. Record identities, sliders, listening setup,
revision/fingerprints, accepted values and limitations in the owning review.

## Source And Tests

### Asset And Moment Catalog

Redux enriches SDK inventory/measurement/reporting with semantic enums, source
evidence, proposed moments and game review notes:

```sh
make audio-catalog-export
# Optional whole-recording sample-peak/RMS pass:
go run ./cmd/audio-catalog -measure
```

Reports default to `bin/audio-catalog.json` and `bin/audio-generation-guide.md`.
`-source`, `-output`, `-guide` select source-evidence root/destinations; output parents
must exist. This tool uses its embedded manifest/assets plus separately fingerprinted
source evidence, not a disk-manifest loader. Run it against its building checkout.
No original-game data is needed. `make clean-audio-reports` removes derived reports;
ordinary clean removes known executables, never authored media/settings/tools.

Recording identity is independent of filename; semantic SFX uses stable symbol
`sfx/SoundPushObject`, never display name/iota ordinal. Music track/location/context,
ambient symbols and `proposed/<category>/<name>` remain distinct; proposed moments
are not live triggers. Catalog dimensions (availability, assignment, listening/loop
review, trigger coverage) stay independent. `AttachSourceEvidence` scans non-test
function bodies under `cmd/ultimav`/`internal`, ignoring comments/strings/declarations
and catalog code as evidence. Literal playback arguments are stronger than mere
references, but scanning proves neither branch reachability nor audibility. Scanned
source and embedded library have separate fingerprints. Reports never accept audio
or own a second backlog; GitHub remains the work tracker.

### Provenance And Filename Conventions

Production filenames follow recording IDs plus format extension. SFX live under
`sounds/ambience`, `effects`, `footsteps`, `instruments`, `interactions`, `magic`;
music under `music`, relative to assets. Imports use the destinations above. Keep
experiments outside embedded assets. File moves preserve recording IDs, update `file`
and preserve provenance; compare hashes for pure renames. Replacing bytes invalidates
listening evidence for the former bytes. Do not overwrite a shared recording to
change only one cue's assignment.

Each recording, including null slots, supplies original filename, source type/URL,
creator, license, import lineage and processing steps (description/tool/performedBy).
Unknown text is literal `unknown`; empty processing means undocumented, not unedited.
Source URLs alone do not establish licensing; filenames do not establish authorship
or a generation platform. Music's confirmed Audacity MIDI conversion by Brad Hannah
is processing, not MIDI authorship: original online sources/creators/licenses remain
unknown. `catalog_notes.go` owns recording-bound observations/briefs, not provenance
or assignments; old-recording diagnoses do not follow replacement candidates.

### Player-Facing Attribution

Main-menu Acknowledgments uses detached, stable-ID `RecordingCredits()` from validated
metadata, without decoding/catalog/hash scans/players or enabled sound. Each non-null
recording appears once regardless of cue reuse. Processing slices are cloned.
Music/effects are separated; retained original filename falls back to basename.
Only identical attribution combinations group, preserving every work/license/processing
statement verbatim. URLs display as plain text; import URLs, hashes, cue IDs and
calibration are not creator attribution. Missing facts are labelled not recorded.

Retained unassigned `legacy_water_01`, `legacy_healing_01`, `legacy_healing_02` are
media identities, not obsolete engine code. Preserve their source credits/unknowns
and import lineage. They do not inherit newer replacements' licenses. The retired
Credit.txt's nine source links remain covered by manifest/provenance regressions;
every bundled file has a recording entry. Source MIT does not relicense media.

### Calibration And Recording Acceptance

Keep recording calibration, contextual cue gain and player preference separate.
Unset calibration is not accepted zero; configuredGain is cue amplitude, not final
output level. SDK measurements are diagnostics, not certified perceptual approval.

For reference listening, record fixed output device, category/OS/hardware levels,
revision and byte fingerprint. Choose provisional family references (grass steps,
ordinary door, healing, forest/stream, exploration score), compare variants alone
and in environmental mix, then check indoor interaction, repeated grate walking,
bard/score, magic/spell layering and extended exploration on headphones/speakers.
Keep routine effects restrained. Record exact dB/context values and approval separately
from saved calibration; distinguish missing triggers/metadata from missing material.
Generated/sourced candidates need provenance, editing notes, tail/seam/click/clipping
and repetition review before integration. A brief should specify event/sequence,
duration, material/family, variants, looping and exclusions; generation is not acceptance.

### Implementation map and regression ownership

- `audio_manager.go`, `game_audio_*.go`: live facade, settings/world adaptation and diagnostics.
- `sound_effects.go`, `voice_ownership.go`, `walking_sounds.go`: semantic requests/cleanup and step policy.
- `music_manager.go`, `music_policy.go`, `music_ownership.go`: score selection/history/holds.
- `ambient_projection.go`, `ambient_sources.go`, `routing*.go`: game grouping/world interpretation.
- `configuration.go`, `calibration_policy.go`: preference curve and family targets.
- `library*.go`, `assets/library.json`: immutable game metadata/assets and Redux validation.
- `catalog*.go`, `cmd/audio-catalog`: game enrichment and report launcher.
- `tooling/`, `cmd/audio-editor`: Redux authoring preset and tool launcher.
- `game_audio*_test.go`, `game_ambient_test.go`, metadata/policy tests: live adapter and game contracts.
- `testdata/berad-audio-redux`: isolated public JSON/planning projections and tool-policy
  integration against the published SDK pin; no alternate playback adapter.
- SDK runtime/authoring/PCM/tool tests: mechanism, publication, analysis, editor/session and backend behavior.
- `scripts/check-audio-module`: consumed-pin/production boundary, including rejection of retired test engines.

Legacy playback/authoring implementations and their duplicate suites are retired.
Game-policy assertions exercise the live facade, pure projections/policy or the
isolated tool preset. Calibrated gain and source-window playback use the same live
manager as the game. Neutral lifecycle/measurement and report-format regressions
live with SDK mechanisms; game report tests retain authored IDs, instructions and
recording-brief cases. Tests must not add a second engine or copied renderer to
make old private-field assertions compile.
Use [Workbench Logging Control](LOGGING_SYSTEM.md#runtime-control) with audio for diagnostics.
