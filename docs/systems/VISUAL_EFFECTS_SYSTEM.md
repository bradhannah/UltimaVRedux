# Visual Effects System

**Purpose**: Describe the visual-effect contracts and rendering techniques that exist in the current codebase. This document is the authority for masking, dissolve effects, screen flash, screen shake, inversion, projectiles, and transient impact effects.

## Architecture

Visual effects cross three layers:

| Layer | Responsibility | Primary code |
|---|---|---|
| Core gameplay | Chooses effects and sequences them without importing Ebitengine | `internal/game_state/` |
| Presentation contract | Exposes required rendering mutations through `VisualCallbacks` | `internal/game_state/system_callbacks.go` |
| Ebitengine adapter | Stores effect state and renders it in `GameScene.Draw` | `cmd/ultimav/gamescene_visual_effects.go`, `cmd/ultimav/gamescene_draw_effects.go` |

`NewVisualCallbacks` validates its core callbacks. Code that receives initialized `SystemCallbacks` should call required visual callbacks directly rather than treating them as optional. Projectile callbacks are attached by `GameScene`, and `ShowCombatImpact` is validated by `NewSystemCallbacks`. The required `ShowDungeonInjury` callback is validated at both construction boundaries.

The current visual/workflow callback surface is:

```go
type VisualCallbacks struct {
    SetWandererDissolveLevel         func(float64)
    InitializeWandererDissolveEffect func()
    CleanupWandererDissolveEffect    func()
    SetAvatarDissolveLevel           func(float64)
    InitializeAvatarDissolveEffect   func()
    CleanupAvatarDissolveEffect      func()
    SetMapFadeLevel                  func(float64)
    SetScreenFlashColor              func(r, g, b float64)
    SetScreenFlashIntensity          func(float64)
    SetScreenShakeOffset             func(x, y float64)
    SetScreenInvert                  func(intensity float64, durationMs int)
    BlockAllInput                    func(bool)
    IsAnyKeyPressed                  func() bool
    GetPressedKey                    func() *semantic_input.Key
    ResetTeleportationSourceRenderer func()
    StabilizeTeleportationDestinationRenderer func(game_state.MoongateDestination)
    StartProjectile                  func(from, to references.Position, kind references.MissileType) uint64
    UpdateProjectileProgress         func(float32)
    ClearProjectile                  func()
    PlayKapowAtPosition              func(references.Position)
    ShowCombatImpact                 func(game_state.CombatAttackFeedback)
    ShowDungeonInjury                func(game_state.DungeonInjuryKind)
}
```

Flash and shake callbacks only set current render state. Timing and cleanup belong to the caller's event sequence.

## Event Ownership

Use an event-queue workflow when the sequence mutates gameplay, owns input, or must be awaited. Use feedback for presentation-only effects that must not block or mutate gameplay. `PartyStateBuilder` belongs inside a workflow when it applies damage or status changes.

Every workflow must set priority before `Sequential()`, start with `First()`, and use `Finally()` for state that must be reset. Feedback builders also start with `First()` and use `Finally()` for cleanup.

```go
manager.TriggerFeedback("impact", event_queue.PriorityNormal,
    func(sequence *event_queue.SequentialEventBuilder) {
        sequence.First(func() {
            callbacks.Visual.SetScreenFlashColor(1, 0, 0)
        }).Animate(200, func(progress float32) {
            wave := 1 - math.Abs(float64(progress)*2-1)
            dampen := 1 - float64(progress)
            callbacks.Visual.SetScreenFlashIntensity(wave * 0.7)
            callbacks.Visual.SetScreenShakeOffset(0, math.Sin(float64(progress)*math.Pi*8)*6*dampen)
        }).Finally(func() {
            callbacks.Visual.SetScreenFlashIntensity(0)
            callbacks.Visual.SetScreenShakeOffset(0, 0)
        })
    },
)
```

Do not start an independent workflow inside another workflow callback when the parent must wait for it. Add reusable steps with `ThenSequence`, or use `AwaitFeedback` when a workflow must wait for serialized presentation feedback.

## Transient Status Icons

`UICallbacks.ShowTransientIcon` accepts a semantic `TransientIconKind`; core
gameplay never supplies sprites, colours, or durations. `GameState` publishes
only after an authoritative transaction observes the required state change.
Multi-character transactions aggregate before publishing, and excluded
low-level setters, hydration, debug edits, recurring poison damage,
resurrection, and Regeneration Ring ticks remain silent.

| Kind | Presentation | Duration |
|---|---|---:|
| Eating | Food, white | 1000 ms |
| Starving | Food, light red | 500 ms |
| Healing | Healer sign, light green | 1500 ms |
| Poison onset | Poison field, green | 1500 ms |
| Poison cure | Poison field, light cyan | 1500 ms |
| Level up | Star pattern, yellow | 3000 ms |
| Karma increase | Shrine, light green | 1500 ms |
| Karma decrease | Shrine, light red | 1500 ms |

`ActiveEffectsOverlay` owns the fixed lookup and records deadlines from
`GameState.ElapsedMs`. Registration first prunes expired records. Re-registering
the same kind preserves its stack position and refreshes its deadline; distinct
kinds append in registration order. At most five records are retained, with the
oldest evicted before a sixth distinct kind is added. Opposite poison and karma
kinds are distinct and may coexist.

The overlay is presentation-only: it creates no workflow, owns no input,
consumes no RNG, and mutates no gameplay or save state. Scene replacement and
disposal clear all records idempotently; ordinary same-scene map transitions do
not.

## Time Stop Actor Animation

Time Stop has a body-only presentation clock: affected actors animate at **5%**
of normal speed. `GameState` projects live actor identities and the slowdown flag;
`cmd/ultimav/actor_animation_clock.go` integrates elapsed deltas for each lifetime.
The clock retains fractional twentieths of a millisecond across activation,
expiry, duration-effect replacement, and allegiance changes. Returning to normal
speed continues the accumulated phase rather than jumping to the global frame.

- Combat uses exact effective allegiance: monster-loyal monsters and possessed
  PCs/clones slow down; party-loyal actors retain normal body timing. Clone slots
  distinguish PC copies; stable monster IDs distinguish recycled monster slots
  and retain a surviving monster's phase through storage compaction.
- Surface creature NPCs and monsters use the same treatment on the active floor.
  Vehicles, pirate ships, whirlpools, environmental hazards, and the ordinary
  exploration Avatar keep their normal presentation timing.
- The first-person dungeon wanderer has its own retained body clock. Dungeon
  fields, fountains, stalactites, and atmospheric animation use ordinary elapsed
  time.

Updates and committed-state display refreshes settle the clocks; drawing only
reads them. A surviving offscreen or hidden actor retains its phase without
gaining disclosure. Retired actors, replaced scenes, and disposal release the
associated state. These clocks are transient, consume no gameplay RNG, and do
not rescale turns, duration counters, calendar rules, or actor movement.

Status overlays, lighting, terrain, environmental animation, projectiles, hit
feedback, spell effects, and UI animation retain their ordinary clocks. The
gameplay rules and scope of this Redux presentation choice are
[SPELL-0004](../gamefacts/MAGIC_SPELLS.md#spell-0004); the scroll shares it under
[CONSUME-0003](../gamefacts/POTIONS_SCROLLS.md#consume-0003).

## Screen Flash

`SetScreenFlashColor` selects RGB components in the `0.0` to `1.0` range. `SetScreenFlashIntensity` supplies the current alpha. `drawScreenFlashEffect` draws the color over the map area only; the right-side UI and dialogs remain unaffected. Flash rendering is skipped in a dungeon while no light source is active.

The caller owns the intensity curve. Current effects commonly use a triangle or sine wave and explicitly restore intensity to zero. `PartyStateBuilder` provides `FlashRed`, `FlashWater`, `FlashGray`, `FlashGreen`, `FlashOrange`, `FlashPurple`, `FlashWhite`, and `FlashInverted` presets.

### Dungeon Injury Feedback

Dungeon injury uses `ShowDungeonInjury`, a separate map-viewport tint rather than
the ordinary flash state. Core code aggregates actual clamped HP loss across the
party before admitting one result for each resolved source:

| Source | Tint | Audio request |
|---|---|---|
| Fire-field HP loss | Light red | `SoundFireBurn` |
| Electrical-contact HP loss | Light blue | `SoundElectricalDamage` |
| Dungeon poison HP tick | Light green | `SoundPoison` |

Zero loss admits nothing. Poison onset alone and sleep-field contact do not own
this injury pulse. Each pulse lasts **200 ms** on `GameState.ElapsedMs`: alpha
starts at 100/255, holds for 40 ms, then fades to zero. It covers the already
composed viewport, including black or subdued darkness, without drawing or
illuminating terrain. It precedes chrome, transcript, and dialogs and neither
borrows nor clears ordinary screen-flash state.

The three stable feedback names coalesce repeats while live, bounding this family
to three active/pending requests. Distinct kinds serialize through the feedback
lane, alongside other feedback, with no input ownership or gameplay delay. Audio
is requested once per resolved injury source at admission, even when its visual
repeat coalesces; it is not replayed at each animation frame. Electrical injury
and poison currently have empty recording slots, so cue requests do not establish
audible feedback.

Admission and drawing retain the game, dungeon visit, and floor. Queued or active
feedback cannot cross visit/floor/combat replacement; expiry, cancellation,
finalization, and scene disposal clear the pulse. Fatal HP loss can still present
while that dungeon owner remains current. Damage, death, status, command
completion, and RNG remain in the authoritative hazard path. See
[DNG-0009](../gamefacts/DUNGEON_SYSTEM.md#dng-0009).

### Dungeon Field Removal

Successful dungeon Sceptre or `An Grav` removal supplies the committed
`DungeonCellChange` to `presentDungeonFieldDispel`. The field is already gone for
collision and hazards. A **600 ms**, cell-anchored pixel fizzle retains only the
removed field's colour/pixels at its admission phase, not its background or
opening-bit geometry. It reprojects at the cell's current visible depth during
the normal far-to-near first-person traversal, before live cell contents. Turning
or moving does not attach the remnant to the camera. Walls, clipping, black
darkness, and optional subdued-light rendering keep their normal authority.

At most **four** remnants coexist; a replacement for the same owner retires its
predecessor, and a fifth evicts the oldest. Admission validates the committed
revision, while later checks retain the dungeon visit, floor, position, and
expected post-removal cell. Unrelated cell edits do not cancel it. A mismatching
cell, floor/visit change, combat entry, elapsed expiry or rewind, or disposal
releases its depth-specific masks and buffers. Once retired, it cannot revive
when a cell later happens to match.

The effect is nonblocking: it adds no wait, command completion, resource payment,
or gameplay RNG draw. Its dissolve uses a presentation-owned seed. Each admitted
success requests `SoundDispel` once; cancellation/no effect does not. That cue's
recording is intentionally empty and shared with combat `An Grav`. It denotes
successful removal, not spell failure. Combat retains its separate collapsing
field cue and target kapow; the first-person fizzle does not replace those paths.
See [DNG-0009](../gamefacts/DUNGEON_SYSTEM.md#dng-0009) and
[Audio System](AUDIO_SYSTEM.md).

## Screen Shake

Screen shake is a camera offset, not a self-timed subsystem:

```go
callbacks.Visual.SetScreenShakeOffset(xPixels, yPixels)
```

The event sequence computes each offset and must restore `(0, 0)`. `PartyStateBuilder.ShakeModerate` currently uses 200 ms and a 6-pixel range; `ShakeStrong` uses 300 ms and a 10-pixel range. Its deterministic oscillation is damped toward zero without consuming gameplay RNG.

The offset is applied to world presentation, including map rendering, world entities, projectiles, kapow effects, and combat impact effects. Static UI, the combat HUD, output panes, and modal dialogs are drawn without the camera offset.

Audio is coordinated explicitly in the same workflow or feedback sequence:

```go
manager.Workflow("earthquake").
    Priority(event_queue.PriorityHigh).
    Sequential().
    BlockInput().
    First(func() {
        callbacks.Audio.Play(game_state.SoundEarthquake).FadeIn(500).Start()
    }).
    ThenSequence(game_state.NewPartyStateBuilder(gs).
        ToAllParty().
        WithShake(4000, 9.6).
        Build()).
    Finally(func() {
        callbacks.Visual.SetScreenShakeOffset(0, 0)
        callbacks.Audio.Stop(game_state.SoundEarthquake).FadeOut(500).Now()
    }).
    Start()
```

## Screen Inversion

`SetScreenInvert(intensity, durationMs)` drives a Kage shader over the composed screen. A positive duration uses `GameScene` elapsed time for automatic expiration. A duration of zero means an external event animation owns the intensity and must restore it to zero. `PartyStateBuilder.FlashInverted` uses the externally controlled form. Inversion shares the screen-flash guard and is skipped in a dungeon while no light source is active.

## Pixel Dissolve And Masking

`internal/visual.PixelDissolveEffect` adapts SDK `graphics.Dissolve` and a native
`Surface` to game-owned target sizes and highlight coordinates. It supports:

- `DissolveModeReveal`: transparent to opaque
- `DissolveModeHide`: opaque to transparent
- forward and reverse progress updates in the `0.0` to `1.0` range
- a persistent CPU pixel buffer and Ebitengine mask image
- an optional persistent scaled mask rendered with nearest-neighbor filtering
- deterministic shuffle order from a presentation-owned source

Create the effect once, update progress, and reuse its mask:

```go
maskWidth, maskHeight := visual.CalculateMaskSizeForDialog(width, height)
effect := visual.NewPixelDissolveEffect(
    width, height,
    maskWidth, maskHeight,
    visual.DissolveModeReveal,
    presentationSource,
)

effect.UpdateProgress(progress)

maskedBuffer.Clear()
maskedBuffer.DrawImage(contentBuffer, nil)
maskOptions := &ebiten.DrawImageOptions{}
maskOptions.Blend = ebiten.BlendDestinationIn
maskedBuffer.DrawImage(effect.GetMask(), maskOptions)
```

`BlendDestinationIn` is required here because the destination already contains the sprite or dialog colors. The mask's alpha controls destination visibility while preserving those colors. Using `BlendSourceIn` in this order selects the mask's colors instead and produces a silhouette rather than a color-preserving dissolve.

`CalculateMaskSizeForDialog` derives mask dimensions from the display scale and clamps each dimension to the implementation's safety range. Do not duplicate that calculation at call sites.

`MessageDialog` uses this path with persistent content and masked buffers. Tile dissolves use a one-to-one sprite mask through `DissolveEffectState`. Presentation effects must construct their own stable source; they must never receive the persisted gameplay RNG source.

The SDK consumes the supplied RNG only while constructing the permutation; reset
and reverse progress reuse that order. Progress counts mask cells, not opaque
sprite pixels. Only changed cells update the CPU buffer, and an unchanged mask
avoids another upload. The optional scaled target is reused. `GetMask` returns a
borrow valid until this effect's `Dispose`, which closes its surface and scaled
image, never the caller's content/target. A disposed effect cannot be revived.
`MessageDialog` disposes its old dissolve owner before Show/Hide replacement,
instant visibility changes, and completion of either fade direction. Its reusable
content/masked buffers and unrelated image borrowers survive those transitions.
Resize separately retires dimension-specific masks. Completing a fade does not
retain a native mask until another visibility or resize operation happens.
The SDK's removed allocating `Pixels` and non-buffer `Update` conveniences are not
used: initialization writes the persistent buffer, and progress updates it
in place. Frame arrays, geometry and ordinary native draw opacity stay with Redux.
Leading highlights retain the game's integer cell-to-target projection; these are
recently revealed permutation cells, not a spatial wavefront or a promise of exact
fractional nearest-sampling coverage.

SDK tests own dissolve progression, repeated-query immutability, reset ordering,
and completion/hide-mode highlight suppression. Game tests retain supplied-source
wiring, uneven integer target projection and actual dialog/scene owner retirement.
The game facade has no separate public reset operation. `MessageDialog` retains
its existing fixed-seed cosmetic source construction, so removing its unused RNG
setter does not change the dissolve order or consume gameplay randomness.

For rejection surfaces, game tests cover reusable Dispose/Resize ownership and
the selected nearest/SourceOver draw policy; generic CPU-clear/explicit-upload
behavior is tested in the SDK. Quarter-mask tests retain exhaustive neighborhood
geometry and locality, plus threshold-adjacent floating-to-Q12 conversion checks.
The exhaustive Bayer phase/value matrix belongs to the SDK.

## Terrain Visibility Dithering

Ordinary top-down small and large maps use a world-anchored visibility-edge
mask, including basement and underworld floors. This is a presentation-only
bonus-terrain treatment: it deliberately exposes a narrow portion of actual
neighboring terrain without changing gameplay visibility, light radii, source
collection, interaction, or entity visibility. Outdoor terrain clues can be
revealed. Indoor rooms and structural boundary tiles retain whole-tile coverage:
visible tiles stay intact, and hidden tiles stay black. There is no indoor
inward fade, rounded erosion, or continuous edge darkening.

- `internal/sprites/quarter_mask.go` contains game-owned neighborhood coverage
  geometry and delegates Bayer4/Q12 thresholding to Berad Graphics. Four pieces assemble
  each tile's coverage; reflections reuse the small piece set. Geometry is
  independent of `GameState`, sprite identities, clocks, and Ebitengine.
- `internal/sprites/dither_mask_image.go` adapts an SDK-owned reusable CPU buffer
  and standalone GPU surface. Transparent mask pixels keep the composed world;
  opaque black pixels reject it. Standalone storage avoids atlas-dependent
  nearest-sampling differences at fractional camera offsets.
- `cmd/ultimav/gamescene_terrain_dither.go` owns the Ultima adapter, sampling,
  trust bounds, mode gate, projection, and final placement. Its constants are
  the outdoor tuning surface: six native pixels outward, one inward, and a
  two-pixel convex-corner radius. The pattern remains fixed at 4x4 world-pixel
  phase. A quarter touching indoor space or a structural boundary keeps its
  original visibility coverage rather than letting an outdoor neighbor bleed
  through that room boundary.
- `GameState.GetBonusTerrainTile` resolves effective map terrain without item
  stacks, corpses, or Avatar reflections. Legacy artifact sprites use their
  known floor underlay; ordinary `GetEffectiveTile` behavior is preserved.

Sampling follows existing frame visibility, not a new LOS calculation. A
missing or untrusted neighbor sample preserves the original hard edge rather
than creating bonus coverage. Small-map exterior tiles retain their existing
boundary treatment. Bonus cells cannot seed additional bonus cells.

### Shared Map Exposure

`internal/map_state/map_exposure.go` supplies an immutable, four-neighbor
classification independent of player position, visibility, lighting, and
Ebitengine. `ClassifyMapExposure` consumes a plain boundary grid and optional
outdoor seed positions; `LayeredMap.ExposureAt` exposes `ExposureIndoor`,
`ExposureOutdoor`, `ExposureBoundary`, or out-of-domain `ExposureUnknown`.
Floors without an exposure classification also return `ExposureUnknown`; callers
must select their map-context policy rather than interpreting unknown as indoor
or outdoor. Large-map dithering retains its existing outdoor treatment.

Small-map floors build this result once in `ReloadSmallMapFloor` from immutable
source architecture. Walls, windows, doors, embedded fireplaces, and source
`Bookcase`/`SignWarning` wall fixtures are separators. Fences, vegetation, water,
other furniture, archways, and portcullises do
not partition exposure. Eligible border cells and explicit outdoor seeds flood
outward-connected space; other non-boundary cells are enclosed indoors.
Door opening, clock changes, visibility changes, and runtime map overrides do
not reclassify the building. Recreating the source floor recreates its exposure.

`smallMapOutdoorSeeds` in `internal/map_state/layered_maps.go` marks Yew and
Minoc's enclosed ground-floor exteriors plus three disconnected upper wall
walks at Lord British's castle as outdoors.
Serpent's Hold's hall and courtyard already reach
the border and need no seed. Its floor-0 room at `(10,25)` is a separate 30-cell
indoor component, covered by original-data and scene regression tests.
Britain rooms `(22,1)` on floor 0 and `(12,1)` on floor 1 exposed missing wall
fixtures: a bookcase at `(22,0)` and a wall-mounted warning sign at `(11,0)`.
Those fixtures are structural barriers, not openings to the map border.

The [sunlight-aware tinting path](../gamefacts/LIGHTING.md#light-0007) consumes this
same classification and seed catalog, without duplicating the flood fill.
Its daytime baselines and cosmetic opening targets do not alter the dither
mask or gameplay visibility. Structural boundary cells remain distinct so that
lighting can shade wall and opening surfaces independently of the dither
renderer's whole-tile protection policy. No roof-inference system is involved.

### Final Composition

The mask is drawn after world actors, glows, ordinary effects, and projectiles,
but before debug diagnostics, transient feedback icons, static UI, and dialogs.
World effects lacking an existing visibility gate check their normal visible
anchor before composition. The final mask cannot make hidden entity admission
safe by itself. Mask and terrain share camera, layout, and shake transforms;
uncovered aperture strips are explicitly blackened using GPU vertex precision.

X-ray, fully opaque map fades, active over-fade dissolves, combat/camping,
first-person dungeons, mini-map presentations, and finale ownership bypass both
bonus admission and masking. Partial fades and ordinary flash/inversion remain
below rejection. No new workflows, callbacks, save fields, or gameplay RNG
sources are involved. Light-pool perimeter dithering is not implemented.

Automated coverage lives in `internal/sprites/quarter_mask_test.go`,
`internal/sprites/dither_mask_image_test.go`,
`internal/game_state/bonus_terrain_test.go`,
`cmd/ultimav/gamescene_terrain_dither_test.go`, and
`cmd/ultimav/gamescene_terrain_dither_rendering_test.go`. Whole-tile indoor
coverage is tested in `cmd/ultimav/gamescene_terrain_dither_indoor_test.go`;
classification is tested in `internal/map_state/map_exposure_test.go` and
`internal/map_state/map_exposure_original_test.go`. These assertions establish
composition and classification, not final visual acceptance.

## Sprite-Shaped Masks

For glows, poison, sleep, and combat overlays that must follow a sprite silhouette, use:

```go
mask := sprites.GetSpriteMask(sprite)
pipeline.ApplyOverlayWithMask(screen, drawOptions, elapsedMs, opacity, mask)
```

`GetSpriteMask` asks the SDK for binary white occupancy from nonzero source alpha.
Its application-lifetime processor bounds source and derived stores separately
(500 entries and 32 MiB each). Inputs and returned images are read-only borrows;
callers must not mutate or deallocate them. Clearing/eviction drops cache references
without invalidating retained results. Sprites are immutable at runtime.

`WithAdditiveAlphaPulse` and `WithAdditiveColour` name the existing additive
channel semantics accurately: RGB is scaled independently of alpha and does not
fade with the alpha pulse under `BlendLighter`. `WithPulsingAlpha` instead returns
the multiplier for the caller's ordinary base-sprite `ScaleAlpha`. Renaming the
additive presets preserves their current appearance; it does not introduce bloom,
halo generation or a correction to their brightness. Explicit-time calls are
deterministic; the position conveniences retain presentation-only wall time.

## Generic Tile Dissolves

`GameScene` owns multiple simultaneous tile dissolves:

```go
id := g.StartTileDissolve(pos, spriteIndex, visual.DissolveModeReveal, true)
g.UpdateTileDissolveProgress(id, progress)
g.StopTileDissolve(id)
```

Use `StartTileDissolveWithOptions` when the effect needs a fixed-grid position, a non-default outline, or `RenderLayerOverFade`. Store the returned ID and remove that exact instance. `StopAllTileDissolves` is reserved for scene-wide cleanup.

```go
var dissolveID int
manager.Workflow("materialize_item").
    Priority(event_queue.PriorityNormal).
    Sequential().
    First(func() {
        dissolveID = g.StartTileDissolve(pos, spriteIndex, visual.DissolveModeReveal, true)
    }).
    Animate(500, func(progress float32) {
        g.UpdateTileDissolveProgress(dissolveID, float64(progress))
    }).
    Finally(func() {
        g.StopTileDissolve(dissolveID)
    }).
    Start()
```

## Combat Actor And Square Transforms

Choose the transform by the visual's owner:

- `combatEntityDrawOptions(position, isPartyMember, entityIndex, slot)` follows
  the exact actor's movement interpolation, attack lunge and damage jolt, then
  the shared battlefield placement/scale and camera shake. Actor-local cues keep
  real identities, including distinct PC clone slots and teleport casters.
- `combatSquareMarkerDrawOptions(position)` applies only battlefield
  placement/scale and camera shake. Persistent fields, cone preview/cast
  footprints, adjacent-square previews, utility target cues, and aim/wake markers
  remain on their authoritative squares through any actor's motion.

Collection ordinals, field indexes and coordinate hashes are not actor identities.
Do not pass them to the actor transform to position terrain or spell footprints.
Field artwork still uses its position-based animation phase and elapsed clock;
square anchoring removes borrowed actor offsets, not animation. Existing ground
display precedence, terrain concealment, cone geometry and effect lifetimes remain
with their owners. The renderer introduces no gameplay mutation or RNG draw.
`cmd/ultimav/gamescene_combat_square_visuals_test.go` covers these distinctions,
including square effects during unrelated actor motion and actor-following
teleport cues.

Combat Splat artwork is alpha-composited over the effective map-only terrain at
the same logical square, below actors and markers. The pile/preview winner is
selected first; losing corpses never appear beneath a field or temporary chest
reveal. Underlays retain overrides, animation and their own dungeon palette
treatment; blood keeps its own palette. The shipped Splat has transparent
background pixels, so no atlas-wide black-to-transparent conversion is needed.
`gamescene_corpse_composition_test.go` exercises actual tile rendering and atlas
alpha over contrasting and animated terrain in both combat palettes without
changing objects, time or RNG. Native visual acceptance remains separate.

## Combat Aim And Active Selection

The selected physical/spell cursor retains its familiar full-size geometry,
with the **entire centre cross and all four corners steady opaque white**.
The face follows the inset 16-source-pixel cell from `(1,1)` to `(17,17)`, with
corner arms spanning 32% of each transformed dimension, two-source-pixel strokes,
and a centred six-source-pixel cross at `(9,9)`. Stroke width and centre arms use
the smaller transformed pixel scale, preserving the original unequal-scale
appearance. Its original one-pixel down/right black shadow supplies contrast.
It is not a compact inset or HP-reserved replacement. Friendly/enemy/empty and
party-recipient meaning remain available in the existing semantic target and
availability readouts. When `An Zu` would awaken a different copy, that secondary
square gets only a small light-magenta outlined cross, without white corners.

Both markers are square-anchored and include camera shake. Actor lunge/movement
offsets cannot drag either onto another square. The secondary cross uses rounded
source-rectangle edges; the selected cursor retains the original stroked-line
raster geometry. Target legality, Tab cycling, cancellation, and resource
commitment remain core/input contracts.

The active-turn actor's **complete pulsing square plus exactly two upper-corner
accents** draws once, above HP/allegiance bars. The accents have four-pixel arms,
two-pixel faces and a full black rim; they follow the square's 450 ms two-phase
expansion and exact presented actor transform. Both use active-PC green,
active-monster/possessed-PC yellow, or auto-pass gray. Threatened-target frames
stay beneath actors, remain steady, and receive no raised accents.

The combat order is threatened-target underlays, actor artwork and local effects,
direction cues, HP/allegiance, the full active indicator, eligibility brackets,
then selected aim. Active and selected indicators intentionally win where they
overlap HP bars; selected aim has final priority. Hidden/retired actors, teleport
hidden phases, and combat exit suppress the active indicator through live exact
identity/disclosure checks, including clones and insect-stack presentation. The
active-party ARCH exception below is limited to its contour and selection frame.
Hands-on feedback chose the original-size all-white cursor and the active frame
above HP; the corrected rendering still requires in-game visual confirmation.

Entry points are `gamescene_draw_entities.go`,
`gamescene_combat_active_indicator.go`, and `gamescene_combat_attack_preview.go`
under `cmd/ultimav`. [COMBAT-0012](../gamefacts/COMBAT.md#combat-0012) owns the public
combat presentation comparison.

### Active Party Entry On ARCH

When the exact current party entry stands on live `BrickWallArchway` terrain,
Redux draws a **hollow silhouette contour plus the existing active square and
upper accents** to make the acting character locatable. It does not reveal the
body or HP/allegiance/status artwork, change targeting assistance, or reveal other
occupants. `BlackSquare` remains fully concealing; active monsters, inactive party
entries, underground/captive actors, retired identities and hidden teleport phases
receive no exception. A possessed current PC keeps its existing yellow selection
colour; an auto-pass keeps gray, and ordinary active PCs keep green.

The contour uses the same unoutlined, current-condition sprite and settled body
clock as ordinary party rendering, including sleeping, invisible-player and rat
forms and exact clone state. `ApplyDynamicOutlineOnly` retains only the coloured
edge with black backing and clears all source-body pixels, including partial
alpha, leaving the arch visible through the interior. Its bounded sprite/colour cache holds artwork,
not actor ownership or position. Live identity is resolved on every draw. The
contour follows the exact actor transform and presented signature endpoint;
signature fade/dissolve masks still apply without mutating cached artwork.

The foreground selection pass draws the contour before the existing square,
then eligibility and selected aim retain their normal priority. Ordinary floor
rendering gets no extra contour or duplicate frame. This narrow presentation
deviation is recorded in [COMBAT-0001](../gamefacts/COMBAT.md#combat-0001) and
[COMBAT-0012](../gamefacts/COMBAT.md#combat-0012). Source and regression entry points
are `gamescene_combat_active_indicator.go`, `gamescene_combat_arch_indicator_test.go`
under `cmd/ultimav`, and `tile_outline_only.go` / `tile_outline_only_test.go` under
`internal/sprites`; rendered assertions do not establish in-game visual acceptance.

### Door And Disarm Selection

Combat An Sanct, An Ex Por and In Ex Por reuse this exact selected cursor in the
final aim layer. It occupies one of four cardinal neighbours, with a geometric
in-bounds default rather than a target-dependent choice. These spells suppress
both the older adjacent-target coloured underlay and the generic direction
arrows, so one white cursor owns selection. Cursor geometry/colour and prompt text
do not expose a concealed door's eligibility or an unassessed chest's trap state.
The active actor indicator retains its foreground placement above HP. Other
adjacent and cone spells keep their existing previews. Commitment and cleanup
belong to [combat door/disarm input](INPUT_HANDLING.md#combat-door-and-disarm-spells),
with composed cursor coverage in `cmd/ultimav/gamescene_combat_door_spell_test.go`.

## Combat Attack Eligibility

`cmd/ultimav/gamescene_combat_attack_preview.go` draws two disconnected,
compact foot-corner brackets above monster artwork and below the aiming reticle.
Foreground placement keeps neighboring sprites from hiding every eligibility
pixel in a dense group.

The face uses semantic `CombatAttackEligibleFace` (EGA light blue);
`CombatAttackEligibleBorder` is black. Each arm is two source pixels long and
one pixel thick, surrounded on all sides by a full one-pixel black border, not
an offset shadow. Both complete brackets fit inside the 16-pixel cell, inward
of flee/status cues. Their faces occupy source rows Y=9..10, so the
backing ends at Y=12 exclusive, before the allegiance outline beginning at Y=12.
The original HP strip still begins at Y=13. Faces and
expanded backing rectangles follow `combatEntityDrawOptions`, including
battlefield shake, and round transformed edges to crisp pixel boundaries.

The brackets are opaque and steady: no pulse, alpha fade, glow, adaptive colour,
sprite tint, full tile box, or dimming of unavailable enemies. In
`drawAvatarOverlay`, the eligibility pass follows all actor artwork and direction
indicators, immediately before `drawCombatAttackCrosshair`. The existing aiming
reticle has priority; the aimed square's brackets are suppressed while other
eligible enemies retain theirs. The white aiming cursor, semantic target text,
and HP/status cues keep their separate meanings.

Eligibility comes only from `GameState.PreviewCombatAttack`, shared with Tab.
Before physical aiming, a target needs at least one currently available source;
during aiming, markers and text use only the actual current source. The core
owns range, ammo/interference, visibility, live combat-terrain admission, and
intended-shot obstruction/bypass facts. Occupants on authored
`BlackSquare`/`BrickWallArchway` terrain receive no bracket, name, remembered
target snap, or Tab attack/approach selection. The renderer adds neither a LOS
formula nor guaranteed-hit claims.
Queries and drawing consume no gameplay RNG, resources, time, or transcript
output.

No marker cache or persistent registration exists. Each read reflects actor,
movement, readied equipment/ammo, source, target life/visibility, and targeting
state. Enemy/auto-pass turns, pending attacks, input-blocked workflows, and
unrelated targeting suppress assistance. Cancelling Aim removes its readout and
restores normal any-source markers when eligible; combat exit yields no preview.
Projectile finalizers retain their separate cleanup ownership; see
[physical attack ownership](INPUT_HANDLING.md#combat-physical-attack-ownership).

Core eligibility/purity and scene bracket pixel geometry, steady rendering,
reticle priority, readout, and cleanup assertions are in
`internal/game_state/combat_attack_preview_test.go` and
`cmd/ultimav/gamescene_combat_attack_preview_test.go`.
`cmd/ultimav/gamescene_combat_marker_composition_test.go` adds the real outer
actor pass plus dense all-nine, center-only, and checkerboard marker layouts at
native, 720p, and 768p scales with subpixel origins. Assertions preserve both
marked faces and complete outlines, HP/status raster footprints, unmarked and
unrelated pixels, and reticle priority.

Composed-render assertions do not replace live swarm keyboard, HiDPI, or
gameplay acceptance, which remain separate validation work.

## Persistent Combat Allegiance

`GameState.ProjectCombatActor` is the read-only exact-entry side/disclosure
projection shared by live health-frame outlines and target/current-actor text. It delegates
side to `combatIdentityLoyalty`, including intrinsic Saduj hostility and
clone-local possession. Its monster `CombatantID` guard rejects recycled slots;
PC clones retain their distinct `Slot`. Encounter-only charm and summon flags
are recorded at successful producers, never inferred from loyalty. A dividing
monster creates a fresh hostile child without inheriting those flags.

With Show Combat HP Bars enabled (the default), every disclosed battlefield actor
uses a steady one-source-pixel **cyan Ally /
red Enemy exterior outline around its black HP frame**, paired with explicit
Ally/Enemy text. The exterior is `(x=1, y=12, width=15, height=6)` inside the
18-pixel replacement sprite. The existing black HP frame remains exactly
`(x=2, y=13, width=13, height=4)`, including its separator, fill dimensions,
health-threshold colours and opacity. The outline never recolours or covers the
HP interior. Non-combat world/vehicle callers of `drawCombatHealthBar` receive
no allegiance outline.

Disabling Show Combat HP Bars removes the health fill/frame and attached outline,
not the complete foreground pass. Signature health-region images can contain
underlying actor pixels and must still receive their matching mask/opacity and
position exactly once. Insect member/control pips, independent Ally/Enemy text,
statuses, targeting, selection and actual-fleeing `<>` remain. Noncombat health
consumers keep their existing rendering. Numeric damage remains readable; Battle
Report adds a detached post-hit monster condition under the
[conditional reporting contract](../gamefacts/COMBAT.md#combat-0012).

HP and allegiance render together in a foreground pass after actor/status
artwork, protecting both from overlapping 18-pixel neighbors. Teleport/capture
frames partition body and health into full-size temporary images before scaling;
their health region receives the same mask, opacity and presented position exactly
once in that foreground pass. Gate health frames stay steady after the hidden
prelude. The possession cap remains upper-right. Insect stacks emit one aggregate
frame with their unchanged health segments and member order; pips sit on source
row 2 so a northern neighbor's outline cannot cover them. Hidden members do not
contribute pips or segments. The full active selection square and its two upper
accents draw above the health frame, followed by attack brackets and the aiming
reticle. Threatened-target selection alone remains an underlay; selected aim
retains final foreground priority.

An encounter-local presentation observer compares only continuously disclosed
exact identities. A visible side change immediately updates the outline and adds
concise transcript feedback. The exterior stays cyan/red without a white flash
or pulse replacing the side meaning. Newly created or
newly revealed entries establish silent baselines. Hidden actors, temporary
hidden signature phases, recycled slots and combat exit cannot produce stale
transition feedback. No gameplay mutation, time advancement, RNG draw or input
owner is introduced. Historical event names and colours remain immutable.

Actual-size render fixtures can be captured by setting `ULTIMA5_REVIEW_IMAGES`
to an existing directory outside the repository and running the focused
`TestCombatHealthFramesActualSizeLookalikes` and
`TestCombatSignatureAdmissionAfterHitchRetainsFullScheduledLifetime` tests.
These are inspectable 1280x720 compositions, not live or joint visual acceptance.
`TestCombatFullHUDCombinedComposition` additionally draws the battlefield, full
combat HUD, roster, transcript and provisions together at 1280x720 and 3840x2160.
Set `ULTIMA5_REVIEW_IMAGES` to an existing external directory to export those
frames. The assertions include reverse spatial draw order with adjacent poisoned
actors, preserving complete HP fills, separators and exterior outlines through
the final HUD composition. Exports include `combat-health-frames-full-hud-1280.png`,
`combat-health-frames-full-hud-3840.png`, and grass/water comparisons with paired
full, quarter and one-hit-point lookalikes. Native and noninteger-scale HP byte
preservation, insect-stack attribution, signature opacity/masking, temporary
concealment and source/clone identity are covered by
`cmd/ultimav/gamescene_combat_health_frame_test.go` and the combat polish tests.

## Combat Signature Feedback And Conditions

All finite timings in this section are **authored at 1x**. The immutable
[combat timing policy](TIME_SYSTEM.md#combat-presentation-speed) divides each
included duration once using the committed action's master rate. The snapshot
is captured before synchronous event publication and retained by every later
phase; changing settings does not retime an admitted effect. Gate and monster
teleport snapshots store their effective phase durations, shared by body,
selection, health and actor-condition composition.

`GameScene.recordCombatEvent` consumes already-committed `CombatEvent` results.
`cmd/ultimav/gamescene_combat_signature_effect.go` owns concurrent, short-lived
snapshots for these successful outcomes:

| Event | Presentation | Audio cue request |
|---|---|---|
| `CombatEventGating` | 125 ms arrival disturbance, 600 ms scattered-pixel materialization, 125 ms settling of the **created** Daemon at its committed position. | `SoundMonsterGate`, once at materialization onset |
| `CombatEventPossession` | 600 ms stepped blue caster-to-victim link and onset corners; a blue marker remains on the possessed entry afterward. | `SoundMonsterPossession` |
| `CombatEventTeleport` | 180/80/200 ms departure, hidden interval and arrival at the published endpoints. | `SoundMonsterTeleport` |
| `CombatConditionDraggedUnder` | 420 ms Corpser victim hide/dissolve, with an Underground cue. | `SoundCorpserCapture` |
| `CombatConditionRegurgitated` | The released exact victim reveals again over 420 ms. | `SoundCorpserRelease` |

Routine underground turns do not replay capture. These snapshots never summon,
move, charm, release, apply damage or complete a turn. They retain the encounter,
exact PC/clone identity or immutable monster `CombatantID`, and original event
positions. A reused slot cannot hide or animate its replacement. Gate/capture
dissolve seeds are presentation-owned; effect updates consume no extra gameplay
RNG. `StartedMs` is captured from `GameState.ElapsedMs` at admission. Gates
advance at most one named phase per update and anchor the new phase to that
update's clock; a hitch cannot consume an unpresented phase. Their live predicate
remains true until effect update completes settling, including when the workflow
polls before visual effects. Nominal duration is 850 ms at 1x; the three
individually scaled phases determine other rates, and hitches extend it.
Repeated updates at the same elapsed value do not age it. Wind continues on the shared main RNG during these
waits: reproducibility requires the same elapsed/action timeline, not just the
same seed. See [timing/RNG scope](TIME_SYSTEM.md#elapsed-time-and-shared-rng-scope).

Live authored `NOTHING`/`ARCH` underlays suppress actor-local disclosure, and
invisible monsters do not gain visible signature sprites. Possession snapshots
also retain admission-time origin disclosure: an invisible caster gets no
origin cue or linking mote, while the visible victim cue remains. A disclosed
Daemon origin survives its subsequent retirement or slot reuse.

The effects themselves create no workflow and acquire no input. The existing
scheduled monster/auto-pass workflow awaits live signature effects before its
exact-once turn completion, and clears them in `Finally`. Expiry, invalidated
identity or position, possession recovery, combat exit/replacement and scene
disposal also release the dissolve, composed frame, body/health layers and
highlight images. Player summon
and `In Por` retain their own workflow finalizers; cancelling those presentations
must not summon or teleport again.
Active signature effects are transient and reject scene snapshot capture.

### Conditions, Selection And History

`cmd/ultimav/gamescene_combat_conditions.go` projects Possessed, Underground and
Sleep from each active exact combat identity. `internal/ui/mainscreen/combat_conditions.go`
formats those detached conditions for the combat turn HUD, which shows the
disclosed acting entry's own combination. Concealed/underground actors use
`Unknown` rather than leaking their names or clone identity in live readouts.
Source and clone conditions remain independent.
Enemy-possession conditions additionally carry a detached episode ID and completed
own-turn progress, shown as `Possessed - 8/12 turns`. Chaos-only control has no
timer. Onset effects require the same encounter and episode, so a cure followed
by reacquisition cannot revive an old effect. Typed resistance/release events
use the report's text.Output paths and freeze the exact victim name without
disclosing its controller. Their allegiance observation prevents duplicate side
announcements. Ordinary release is distinct from emergency pass-out recovery;
the authoritative policy is [COMBAT-0005](../gamefacts/COMBAT.md#combat-0005).
Persistent conditions such as Poisoned can coexist; this projection never
mutates the party roster to force a colour or label.

The top-right party panel uses the same portraits, two-line name/HP/status/MP
layout, XLarge font, and line spacing in combat and exploration. In particular,
`Pois.` remains in its ordinary status column with its ordinary typography;
entering combat does not shrink portraits or move status text into a compact
block. A detached text overlay supplies each original party entry's exact combat
conditions without inheriting its clones' conditions or changing persistent roster
status. Text precedence is Dead, Possessed/Charmed, Poisoned, Sleep, then Good.
Possession uses semantic light blue and the full `Possessed` label; the status
label alone is measured to fit its existing cell when necessary, leaving MP,
names, portraits, and row geometry unchanged. Selection keeps its existing
brightness distinction. The text cache includes effective label, colour, and cell
width, so acquisition, recovery, resizing, and combat exit update immediately.
Clone-specific conditions and resistance progress remain in the exact-actor HUD
and battlefield indicators, not a second roster layout.

The ongoing Possessed marker is a blue cap inside the sprite's upper edge and
is paired with explicit `Possessed` text. It remains distinct from active
selection, target indicators and HP colour, including when an unpossessed source
and possessed clone coexist. Underground and Sleep are independent labels;
rendering a short onset effect does not own their lifetime.

`cmd/ultimav/gamescene_combat_actor_conditions.go` shares exact-entry composition
between ordinary sprites and signature frames. Teleport and Corpser conditions,
health outlines and HP are composed before applying the frame's dissolve/alpha. Gating
first highlights newly revealed **sprite-only** pixels, then composes conditions
and masks the silhouette; the persistent health outline and untinted HP follow. Highlights
never recolour semantic marks. The entire gate replacement suppresses ordinary
actor rendering, including the sprite-free prelude. The white leading pixels
and staged pacing are approved Redux presentation embellishments to the original
fizzle, not changes to gating mechanics. All masks, frames and highlight buffers
are released on completion and every invalidation/cancellation exit. Teleport selection
follows the departure position, disappears during the hidden interval, and
returns at arrival rather than exposing the committed destination early.
Targeting remains in the foreground; HP colour remains independent of status.

`cmd/ultimav/gamescene_combat_report_identity.go` captures actor names,
allegiance colours and disclosure permissions when an event enters report
history. Known historical names remain stable through retirement or slot reuse.
An actor concealed at admission
stays `Unknown` in that historical event even if later revealed. Both compact
and detailed player reports suppress concealed coordinates, diagnostics and
creature types, including typed gate/vanish/summon/repel payloads; later reveal
or slot reuse cannot disclose them retroactively. Workbench raw diagnostics
retain authoritative data independently of this player-facing policy. Live turn
HUD identity is queried independently: invisible monsters have no sprite, name or
DEX detail; the optional concealed forecast slot remains anonymous (`???`).
This is a Redux information-display policy, not a reconstruction of the DOS HUD.

Original gate creation includes a fizzle reveal;
possession and Corpser capture also have original feedback calls. The modern
blue link, marker, text composition and timing are Redux presentation choices.
Gameplay/presentation boundaries are documented in [COMBAT-0012](../gamefacts/COMBAT.md#combat-0012)
and the [species mapping](../gamefacts/ENEMIES.md#enemy-0002).

### Sound Availability

The five named signature cues above are wiring placeholders awaiting recordings,
not listening acceptance. The authoritative recording assignments are in
`internal/audio/assets/library.json`; a `file: null` slot is silent. Existing
empty magic, poison, sleep, miss/graze/flee and projectile/impact slots likewise
do not become audible because a caller requests them. `SoundMagic` is not proof
of an audible fallback. Callback tests establish which cue is requested;
recording work remains incomplete. See
[Audio System](AUDIO_SYSTEM.md) for the manifest and playback boundary.

### Practical Keyboard And Workbench Walkthrough

This is a repeatable inspection procedure, not a record of manual acceptance.

1. Launch the [Debug Workbench](../DEVELOPMENT_GUIDE.md#starting-the-debug-workbench)
   with local original data. From stable exploration, use Encounter Options and
   Start Encounter (`combat.encounter.options`, `combat.encounter.start`) to
   choose a map, exact count, friends off and an explicit seed. Retain the seed,
   party/loadout, map and keyboard action sequence for comparison; exact replay
   also requires the supplied elapsed timeline. Use Active
   Combat (`combat.status`) to inspect IDs, positions, conditions and queue state.
2. Start with Dragons to observe gating without the caster's possession branch;
   separately use Daemons/Shadowlords for possession, Wisps for teleport, Ghosts
   for invisibility, and Corpsers on a map with usable grass. Use keyboard Space
   to pass and arrows to position the party until the relevant committed event
   occurs. Keep at least two living PCs for possession. Probability gates mean
   an arbitrary seed does not guarantee a special on its first turn.
3. Compare the event report with the battlefield and Active Combat state. A gate
   reveals one newly created Daemon; teleport shows both endpoints; successful
   possession leaves blue marker/text after its link expires. A Corpser's
   damaging surviving capture hides the victim and labels it Underground;
   release reveals it once. Selection follows the visible teleport phase, and
   compound status/HP cues remain legible independently.
   Check at native and scaled window sizes with adjacent actors.
4. For clone composition, prepare the required spells/resources through existing
   Workbench controls, then use keyboard `C` and the spell dialog for
   `In Quas Xen`. Compare source/clone IDs and simultaneous conditions; one
   entry's possession/sleep/underground state must not colour every copy. Browse
   report history with its displayed keyboard hints after invisibility changes
   or retirement: history retains admission-time identity/disclosure, including
   detailed coordinates and typed creature names, while live invisible turn
    slots remain anonymous. Combat entry, condition changes and exit must retain
    the same party portraits and name/HP/status/MP typography as exploration.
5. Let effects expire and leave combat through ordinary victory/evacuation;
   confirm no stale marker, hidden actor, dissolve or blocked input survives.
   At an eligible quiescent boundary, Workbench Discard Active Encounter can
   separately inspect teardown. It is not a way to interrupt an input-owning
   effect mid-animation. Mid-phase shutdown and replacement are covered by the
   automated lifecycle tests below. Audio listening requires populated manifest recordings.
6. Open a multi-item combat chest and compare each temporary icon with its
   discovery description. Acknowledge with a fresh Enter/Escape, wait several
   seconds, then pick up items one at a time: persistent pile display must stay
   stable until its contents change. Inspect overlapping fields/body markers
   where available; a field must not cover the winning ground object or reveal
   preview. A nested unopened chest remains closed. Search/Jimmy/An Sanct retain
   their exact selected chest's knowledge independently of display order. Check
   uncertain-safe key loss, armed failure/success, and known-safe refusal; use
   surface/first-person dungeon chests for save/load and fresh-visit lifetime
   checks because combat saves are unsupported. For two co-located combat chests,
   Jimmy's last-selected chest card must survive the first-selected chest's Open
   summary and retire only at its own summary. Association uses encounter plus
   non-reused object lifetime identity, not square or original slot. A pending
   reveal retains the opened chest's detached identity between removal and modal
   admission; other removed-owner cards retire even under a modal, and identical
   replacements cannot inherit them. See [combat loot feedback](../ui/GAME_UI_INTEGRATION.md#combat-loot-feedback).
7. Ready a Bow and a melee off-hand, then let an adjacent hostile creature make
   a melee attack against that PC. While the remembered attacker remains eligible,
   use keyboard `A`: one classic interference message and one Battle Report entry
   identify the creature, PC, and Bow, and the remaining source is still offered.
   Repeat from a fresh melee engagement for `Tab` and `C`; Cast rejection does
   not enter the spell dialog or spend MP/prepared spells. Browse/redraw the
   report and check that no message/cue repeats. The interrupted-action recording
   is intentionally empty; this walkthrough can inspect report/command behavior,
   while listening acceptance requires a future assigned recording.

Real encounter events above exercise authoritative producer-to-presentation
routing. Reading an automated assertion does not constitute a live
keyboard/audiovisual acceptance pass.

## Area-Spell Outcome Feedback

Earthquake, Sleep, Poison Wind, Flame Wind, and Energy Vortex publish typed
`CombatAreaSpellTargetResult` records for exact resolved entries, separately from
processed positions. `showCombatAreaSpellImpact` consumes those records rather
than treating every processed square as a successful hit. Local labels distinguish
`AVOIDED`, `RESIST`, `IMMUNE`, and `NO DAMAGE`; actual damage/fatality and successful
poison/sleep retain their corresponding result labels. Only successful cone
damage or status application produces its arrival kapow, and Flame Wind's fire
tint requires actual HP loss. Poison Wind avoidance produces one local result.

The combat report consumes the same outcomes. `presentCombatAreaSpellSummary`
says `No targets reached.` for an empty resolved target set. With no disclosed
successful effect, it summarizes avoidance, resistance, immunity, and no damage;
concealed results instead use `No visible effect.` without revealing counts or
identities. Local effects require admission-time disclosure and remain subject to
live terrain/hidden-actor checks. A disclosed successful HP or status effect
already explains the cast, so it suppresses the aggregate no-effect message.

Feedback never rerolls saves or damage, changes cone candidate collection, or
repeats commitment. Confirmed ineffective casts retain their audio request,
animation, payment, and one action completion. Entry points are
`gamescene_combat_cone.go`, `gamescene_combat_spell_effects.go`,
`gamescene_combat_impact.go`, and `gamescene_combat_hud.go` under `cmd/ultimav`;
[SPELL-0003](../gamefacts/MAGIC_SPELLS.md#spell-0003) owns the public comparison.

## Combat Fire Injury Feedback

Fireplace, lava and magical fire-field injuries publish a `CombatFireInjury`
alongside the resolved field event. It captures the exact PC/clone or monster
identity and disclosure before damage can retire the entry, then records actual
clamped HP loss and fatality. This durable result contains only value data:
target/position, encounter ID, queue-entry lifetime ID, monster birth ID,
disclosure, HP loss and fatality. It retains no combat, queue or provider pointers,
so later gameplay or presentation-mode differences cannot alter historical tape
equality. Entry lifetime IDs are assigned deterministically on registration and
replacement, and remain unchanged by dispatch or death. Flame Wind adapts its
existing typed
`CombatAreaSpellTargetResult` into the same presentation family. Report admission
and the effect each retain their own value copy; later mutation of the producer's
record cannot rewrite an admitted result. Zero HP loss admits no burn tint or
damage label. Flame Wind's avoidance/resistance/no-damage labels remain part of
its separate typed outcome contract.

One resolved injury owns one local result. Direct fire-field damage to monsters
retains authoritative damage, death/XP and surviving-hit processing but no longer
also emits the generic attack-impact callback. The field event supplies its burn
feedback once and retains the direct spell's attack/armor rolls, pre-protection
damage, final damage, undead/immortal flags and magic/spell identity. Presentation
never infers damage from touching a tile, repeats a standing-hazard check, or
consumes gameplay RNG. Terrain/field precedence and
direct-versus-standing formulas remain
[COMBAT-0007](../gamefacts/COMBAT.md#combat-0007); the presentation comparison is
[COMBAT-0012](../gamefacts/COMBAT.md#combat-0012).

### Contact and lifetime

Fire effects timestamp admission from `GameState.ElapsedMs`; the preceding frame's
delta cannot age a newly admitted effect after a hitch. A stationary injury starts
its lifetime immediately. When movement has already resolved damage before its
interpolation is installed, only the fresh injury for that exact actor/clone and
destination binds to the move. A new authoritative move closes older injuries'
binding eligibility, including moves in the same millisecond or with zero damage.

For a bound move, burn tint and label become visible at **50% movement progress**,
when the smooth interpolation reaches the destination tile's near edge. Their
age stays zero through the remaining movement. Completed arrival starts a full
**450 ms tint fade** and the floating-result lifetime (1250 ms at 1x, divided by
the captured combat rate with a 300 ms floor); movement speed does
not consume that post-arrival budget. The tint begins at alpha 0.72 and fades
linearly. It covers only the sprite silhouette, after persistent condition
artwork, while the normal HP/allegiance and selection passes keep their ownership.
Fire adds no attack lunge or damage jolt. Living labels follow the exact disclosed
actor and its movement offset, including camera shake; fatal labels retain the
resolved square after sprite retirement (`FALLEN` for a PC, `SLAIN` for a monster).

The effect owns neither input nor a new workflow. Movement's existing finalizer
starts the arrival lifetime or discards only the injury bound to a cancelled move.
The later tint/label never extends movement's input block or completes another
action. Unrelated impacts retain their own lifetimes.

### Disclosure and cleanup

The scene binds a separate opaque `CombatFireInjuryOwner` synchronously when
consuming the result. Only that transient effect owner retains the live combat
and queue references; the report and event payload never retain it. Projection
checks that owner together with the result's scalar lifetime IDs, so same-slot
replacement is rejected even if it occurs before effect admission or reuses the
same provider. Reconstructing a combat/queue with equal numeric IDs cannot keep
an already admitted effect alive.

Admission-time concealment is permanent for that result; later revealing an actor
cannot reveal a previously hidden burn. Live checks also reject changed encounters,
replaced queue-entry lifetimes, reused monster identities, invisible/underground
actors and concealed terrain. PC clones remain separate even though HP is shared.
A disclosed fatal result can survive retirement at its recorded square, but not a
replacement occupant or revival. Once update removes an invalidated effect it
cannot reappear on a later reveal.

At most 64 fire impacts coexist; admitting another evicts the oldest fire impact
without evicting unrelated effect families. Expiry, invalidated ownership or
disclosure, movement cancellation, combat exit/replacement and scene cleanup
release retained fire state. These records are transient and add no save fields.

Entry points are `internal/game_state/combat_fire_injury.go`,
`internal/game_state/combat_field_effects.go`, and
`cmd/ultimav/gamescene_combat_fire.go`, alongside combat impact/movement handling.
`combat_fire_injury_test.go` and `gamescene_combat_fire_test.go` cover actual-loss
admission, direct-field single delivery, clone/fatal identity, concealment, contact,
speed/hitch timing, bounds and cleanup. Their controlled RNG and rendered-mask
assertions do not establish live visual acceptance or equal-seed equivalence under
different production wind timelines; see [elapsed-time RNG scope](TIME_SYSTEM.md#elapsed-time-and-shared-rng-scope).

## Projectile Travel

`gamescene_projectile.go` renders the source, resolved endpoint, and missile
type supplied by authoritative gameplay. Tactical attacks use the scheduled
monster workflow and `CombatProjectileLaunch`; surface fireballs and pirate
broadsides share the outdoor FIFO workflow. The renderer never retraces a shot
or rolls gameplay RNG. Large-map coordinates take the shortest wrapped path.

`ProjectileAnimationSequence` starts at the source, animates to the resolved
endpoint, clears the projectile, then invokes impact. The owning workflow's
finalizer also clears transient state. Scheduled monster impact and completion
are idempotent: shutdown resolves an already-prepared attack once; encounter
teardown discards the old encounter's pending impact. Outdoor damage is already
resolved before presentation and must never be replayed by flight or cleanup.

Queue admission gives a new flight progress zero rather than charging the
previous update's pre-launch interval (see
[animation admission](EVENT_QUEUE_SYSTEM.md#animation-admission-and-time)).
Timing uses native pixels, independent of display scaling: ordinary missiles
travel 13 pixels per 40 ms, rocks 8, and town missiles 6. The Euclidean path and
100 ms authored minimum are deliberate Redux presentation choices; original evidence is
recorded in [COMBAT-0012](../gamefacts/COMBAT.md#combat-0012).

The combat sequence scales the calculated base duration **after** that floor.
At 4x a minimum flight is 25 ms. One action snapshot covers every source and
outbound/impact/return leg. `StartProjectile` itself continues to return an
unscaled duration so shared surface cannon, pirate and world-missile workflows
retain their own timing. Impact still follows arrival and clearing; presentation
never spends a later source or reapplies a resolved hit.

### Rendering Families And Automated Probes

| Rendered family | Tactical enemy references exercised by the scheduled-flight test |
|---|---|
| Rock outline | Gargoyle, Ettin |
| Directional arrow | Bard |
| Directional axe | Guard, Troll |
| Blue magic | Wanderer, Lord British, Sea Horse, Squid, Reaper, Mage |
| Red magic | Blackthorn, Sea Serpent, Devil, Dragon, Shadowlord |
| Green magic | Mimic, Snake |
| Violet magic | Gazer |
| Cannonball silhouette | Pirate ships use the outdoor workflow rather than the creature-reference inventory |

`gamescene_projectile_rendering_test.go` exercises all 19 ranged references
through normal scheduled turns and reads Ebitengine pixels at origin and during
flight. The shared-family raster probes cover grass, brick, water and black
backgrounds at 960×540, 1280×720 and 1920×1080 logical sizes, including shake.
Surface and underworld visibility probes reject hidden centers under the normal
terrain-dither path, including fractional camera offsets. Combat projectiles
remain event feedback, following the existing combat admission rules below;
they do not disclose an actor sprite under concealed terrain.

`gamescene_combat_scheduler_projectile_test.go` covers the ordinary Ettin's
250 ms seek-delay boundary, intermediate frames, hit/scatter/blocked endpoints,
short legal shots, exact-once impact/completion, shutdown and encounter exit.
`gamescene_projectile_world_test.go` exercises actual outdoor Pass completion
for Dragon/Sea Serpent fireballs and pirate cannonballs, including wrapped
origins, obstruction, cancellation, and presentation-only RNG accounting.
Melee/reach exclusion remains covered by `gamescene_reach_weapon_test.go`.

These probes establish raster contribution and bounded position, not perceptual
recognition or live acceptance. A live pass must still record the build and
display settings, allow a normal Ettin turn, observe recognisable stone travel,
and repeat representative physical, coloured and cannonball flights at supported
window/fullscreen and display scales. Neither these pixel assertions nor a VFX
primitive preview substitutes for a recorded gameplay observation.

## Finite Combat Timing And Readability

Cone propagation/hold (450/150 ms), party spell flashes (500 ms healing/cure
inversion; 150 ms awakening flash), combat-scoped shared pulses (180 ms, with a
60 ms gap where authored), reveal (600 ms), earthquake
(1000 ms), utility approach/result (180/220 ms), and player teleport
(180/80/200 ms) use the same captured policy. Summon phases retain their
120/140/300/180 ms profile; Daemon invocation retains 150/180/420/280 ms;
repel/fear retain 240/640 ms caster/target phases. Workflow animation progress
drives the same phase state consumed by rendering. A phase still must be admitted
before its time can be spent; faster rates do not skip phases after hitches.

`PartyStateBuilder.BuildWithPresentationTiming` clones the authored visual layers
and maps their durations together, preserving agreement between the longest
animation and individual flash/inversion/shake progress. It retains existing
gameplay mutation and audio ownership; ordinary `Build()` stays at its authored
timing. Combat scroll inversion uses the same scaled duration for its expiry and
owning wait. Confirmations remain player-dismissed.

Concurrent motion is independent of result reading: impact snapshots store
effective 300 ms lunge, 320 ms jolt and 180 ms strike durations, while floating
labels divide their 1250 ms reading budget by the same rate but never shorten
below 300 ms. Division's 280 ms tether / 720 ms total and 110 ms
outline interval, 750 ms disappearance, 320 ms Sceptre / 420 ms field-dispel cues,
and 500 ms combat Kapow bursts scale without acquiring input or a new completion
dependency. Their noncombat callers and the movement-bound fire readability
budget remain independent. Report emphasis, action cards, persistent conditions,
selection/idle animation and audio are not accelerated.

Concurrent impact (including area-spell labels), division, disappearance and
combat Kapow snapshots record their admission timestamp. Updates calculate age
from `GameState.ElapsedMs` rather than adding the previous frame's delta, so a
50 ms or 200 ms frame cannot erase a newly admitted 4x strike, division or vanish
before its first draw. Normal aging and expiry still use their captured durations;
the label reading budget uses its captured rate above the 300 ms floor. Movement-bound fire retains its
separate contact/arrival clock. Sceptre, field-dispel and signature effects already
use admission clocks; queued spell animations retain their animation-admission
ownership. Noncombat Kapow effects retain their independent cosmetic delta clock.

Chest contents reveal every authored 500 ms and victory departure waits an
authored 180 ms per member, each with one sequence snapshot. Loot acknowledgement,
turn completion, cancellation cleanup and input release retain their existing
owners. The leading-only dynamic field-spell wait remains ineffective as
documented in the [time contract](TIME_SYSTEM.md#combat-presentation-speed).
Automated boundary, snapshot, return, hitch and cleanup tests do not substitute
for native pacing and text-readability acceptance.

## Other Current Effects

- Projectiles are started with a world source, target, and missile type; `Animate` updates progress and `Finally` clears the active projectile.
- Kapow effects use the explosion sprite and a scale triangle over their short lifetime.
- Combat impact effects render local hit feedback and floating results from `CombatAttackFeedback`.
  Its typed presentation metadata separates melee, projectile, spell, and
  environmental delivery while retaining source equipment for future presets.
  Melee feedback draws a short concurrent EGA strike above battlefield actors;
  projectile, spell, and environmental feedback never reuses the melee strike
  or attacker lunge. Projectiles and cone footprints remain event feedback rather
  than actor disclosure. Generic impacts retain their existing admission path;
  typed area-spell and fire-injury effects apply the disclosure gates described above.
  Player reports separately apply admission-time disclosure rules.
- Combat actor-local silhouettes, selection frames, division tethers, field
  removal cues, and summon/repel/teleport identity cues consult the actor's live
  map-only underlay. `BlackSquare` and `BrickWallArchway` suppress those direct
  location disclosures without cancelling or delaying the underlying effect,
  except for the [current party entry's ARCH contour and active frame](#active-party-entry-on-arch).
- Committed combat Sceptre use records a short actor cue with the exact combat
  identity and position before barrier or direction resolution. Successful
  `CombatEventFieldDispelled` events independently record a collapsing cue at
  the removed field position, so Sceptre and `An Grav` share producer-neutral
  result feedback. Both concurrent effect slices use `GameState.ElapsedMs`,
  consume no gameplay RNG, add no workflow delay, and clear outside combat.
- Map fade is a black map-area overlay controlled by `SetMapFadeLevel`.
- Moongate renderers project the authoritative `GameState` stage; callbacks
  refresh presentation resources but do not own an independent growth state.

## Verification

Focused tests live beside the owning packages:

- `internal/visual/` for dissolve progress and scaling
- `internal/sprites/tile_overlays_test.go` for sprite-mask caching
- `internal/game_state/party_state_builder_test.go` for flash and shake callbacks
- `cmd/ultimav/gamescene_draw_effects_test.go` for rendering calculations
- `internal/game_state/actor_animation_presentation_test.go` and
  `cmd/ultimav/actor_animation_clock_test.go` for actor classification, retained
  phase, rate boundaries, and read-only draw clocks
- `cmd/ultimav/dungeon_field_dispel_test.go` for committed-removal projection,
  lighting, bounded remnants, and owner cleanup
- `internal/game_state/dungeon_injury_feedback_test.go` and
  `cmd/ultimav/gamescene_dungeon_injury_test.go` for actual-loss admission,
  coalescing, serialized presentation, darkness, and cleanup
- `cmd/ultimav/gamescene_combat_colors_test.go` and
  `cmd/ultimav/gamescene_combat_active_indicator_test.go` for aim/wake geometry,
  two-corner active accents, and marker identity
- `internal/game_state/combat_area_spell_outcome_test.go` and
  `cmd/ultimav/gamescene_combat_area_spell_outcome_test.go` for exact target results,
  disclosure-safe summaries, and ineffective-cast presentation
- `cmd/ultimav/gamescene_input_combat_test.go` for combat effect integration
- `cmd/ultimav/gamescene_combat_sceptre_effect_test.go` for Sceptre actor and
  producer-neutral field-dispel cue lifecycle
- `cmd/ultimav/gamescene_combat_signature_effect_test.go` for production event
  routing, cue requests, unchanged gameplay state/RNG, concurrent expiry,
  recycled-slot rejection, terrain/invisibility and exit/replacement/disposal
- `cmd/ultimav/gamescene_combat_signature_lifecycle_test.go` for scheduler-owned
  waiting/completion, queue shutdown, and cancellation at every player Daemon
  scroll summon or `In Por` presentation phase
- `cmd/ultimav/gamescene_combat_signature_timing_test.go` for hitch-time admission,
  full effect lifetime, exact turn-token ownership and the shared-wind RNG oracle
- `cmd/ultimav/gamescene_combat_effect_admission_test.go` for production-order
  concurrent effect publication after hitches, the first 16 ms strike draw with
  an explicitly published logical display size,
  exact expiry boundaries, same-time updates and scaled label reading time with its floor
- `cmd/ultimav/gamescene_combat_signature_disclosure_test.go` for concealed versus
  retired possession origins and immutable report disclosure after slot reuse
- `cmd/ultimav/gamescene_combat_signature_composition_test.go` for teleport
  selection phases, compound actor conditions/HP, targeting priority and cleanup
- `cmd/ultimav/gamescene_combat_conditions_test.go` and
  `internal/ui/mainscreen/combat_conditions_test.go` for exact source/clone
  conditions, combined HUD labels, marker/selection/HP independence, recovery and
  invisible live HUD versus admission-time report history
- `internal/ui/mainscreen/character_summary_layout_test.go` for original-size
  portraits and identical name/resource/status rendering across combat entry,
  conditions, recovery and exploration, including font-metric preservation
