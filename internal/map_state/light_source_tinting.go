// Package map_state provides light source tinting for visual atmosphere.
//
// This implements colored light tinting for torches, braziers, fireplaces, etc.
// Light sources tint nearby tiles with warm or cool colors at night.
//
// Original light-source behavior is documented in docs/gamefacts/LIGHTING.md.
package map_state

import (
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// ============================================================================
// TUNABLE PARAMETERS - Adjust these to change light tinting behavior
// ============================================================================

// Breathing animation parameters (slow, gentle oscillation)
const (
	// BreathingPeriodMs is the time for one complete breathing cycle.
	// Higher = slower breathing. 2000-3000ms recommended for visible effect.
	BreathingPeriodMs = 2250

	// BreathingAmplitude controls how much the light intensity varies.
	// 0.05 = ±5% variation (very subtle), 0.10 = ±10% (more noticeable)
	BreathingAmplitude           = 0.065
	breathingPhaseHashMultiplier = 17
)

// Moongate breathing animation (faster, more intense than fire-based sources)
const (
	// MoongateBreathingPeriodMs is the moongate pulse cycle (matches sprite glow)
	MoongateBreathingPeriodMs = 1500

	// MoongateBreathingAmplitude is the moongate intensity variation (±35%)
	MoongateBreathingAmplitude = 0.35
)

// Color tinting intensity parameters
const (
	// TintStrength controls how strongly the light color affects tiles.
	// 0.0 = no tinting, 1.0 = full color. 0.15-0.25 recommended for subtle warmth.
	TintStrength          = 0.25
	halfDayTintMultiplier = 0.5

	// BaseBrightness is the minimum brightness multiplier at the edge of light.
	// 1.0 = no darkening. Lower values = darker edges.
	// 0.55 = 45% darker at edges for moderate falloff
	BaseBrightness = 0.55

	// CenterBrightness is the brightness at the light source itself.
	// Values > 1.0 make the center slightly brighter.
	CenterBrightness = 1.05
)

// Blending parameters for overlapping light sources
const (
	// FalloffExponent controls how quickly light fades with distance.
	// 1.0 = linear, 2.0 = inverse square (realistic), 1.5 = balanced
	FalloffExponent = 1.5

	// MaxSourcesPerTile limits how many light sources affect a single tile.
	// More sources = smoother blending but more computation.
	MaxSourcesPerTile = 4
)

// Tint arithmetic constants.
const (
	// minTintChannel and maxTintChannel bound each final RGB scale factor.
	minTintChannel = 0.3
	maxTintChannel = 1.5
	// noSourceDistance is the starting "closest" distance before any source is seen.
	noSourceDistance = 1000.0
	// independentPeriodMin and independentPeriodSpread give the +-15% (0.85 to 1.15)
	// per-source period scale for IndependentBreathing, in independentPeriodSteps steps.
	independentPeriodMin    = 0.85
	independentPeriodSpread = 0.30
	independentPeriodSteps  = 100
	// fullCircle is one breathing cycle in radians.
	fullCircle = 2 * math.Pi
)

// Unlit area parameters
const (
	// UnlitDarkeningFactor darkens tiles outside any light source radius.
	// Applied when there are active light sources to create contrast.
	// 1.0 = no darkening, 0.75 = 25% darker, 0.40 = 60% darker.
	// Previous value: 0.55 (45% darker)
	UnlitDarkeningFactor float32 = 0.40
)

// UnlitAmbientBrightness is the colour multiplier for unlit outdoor terrain at a
// cosmetic darkness from 0 (day) to 1 (night): 1 by day, UnlitDarkeningFactor at night.
func UnlitAmbientBrightness(darkness float32) float32 {
	return 1 + (UnlitDarkeningFactor-1)*darkness
}

// ============================================================================
// Ambient Light Configuration
// ============================================================================

// defaultAmbientLightRadius is the night-vision radius, in tiles, used when none
// (or one below 1) is configured. Configured range: 1-5.
const defaultAmbientLightRadius = 2

// resolveAmbientLightRadius applies the fallback to the default for radii below 1.
func resolveAmbientLightRadius(radius int) int {
	if radius < 1 {
		return defaultAmbientLightRadius
	}

	return radius
}

// ============================================================================
// Math helpers
// ============================================================================

func sin(x float64) float64    { return math.Sin(x) }
func sqrt(x float64) float64   { return math.Sqrt(x) }
func pow(x, y float64) float64 { return math.Pow(x, y) }

func clampFloat(value, lower, upper float64) float64 {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}

	return value
}

// tileOffset converts a light-radius offset to a map coordinate delta. Offsets
// are bounded by the light radius, far inside the int16 range of Coordinate.
func tileOffset(delta int) references.Coordinate {
	if delta < math.MinInt16 || delta > math.MaxInt16 {
		return 0
	}

	return references.Coordinate(delta)
}

// ============================================================================
// Player Light Types
// ============================================================================

// PlayerLightType identifies the source of player-carried light
type PlayerLightType int

const (
	// PlayerLightNone means the avatar carries no light.
	PlayerLightNone PlayerLightType = iota
	// PlayerLightAmbient is minimal night vision: neutral gray, configurable radius (default 2 tiles).
	PlayerLightAmbient
	// PlayerLightTorch is a lit torch: orange light, 3 tile radius.
	PlayerLightTorch
	// PlayerLightInLor is the In Lor spell: white light, 5 tile radius.
	PlayerLightInLor
	// PlayerLightVasLor is the Vas Lor spell: white light, 5 tile radius.
	PlayerLightVasLor
)

// ============================================================================
// Light Source Properties
// ============================================================================

// TintMode controls when a light source applies color tinting to nearby tiles.
type TintMode int

const (
	// TintNightOnly applies tinting only at night (default for fire-based sources)
	TintNightOnly TintMode = iota
	// TintHalfDuringDay applies full tint at night, 50% tint during day (subtle magical glow)
	TintHalfDuringDay
	// TintAlways applies full tint at all times (strong magical sources)
	TintAlways
)

// LightSourceProperties defines visual characteristics of a light source.
// These properties control how nearby tiles are tinted at night.
type LightSourceProperties struct {
	// TintColor is the color this light adds to tiles (as RGB scale factors > 1.0)
	// Values > 1.0 brighten that channel, < 1.0 darken it
	TintColor LightTintColor

	// HasBreathing enables slow breathing animation for fire-based sources
	HasBreathing bool

	// BreathingPeriodMs is the custom breathing period in milliseconds.
	// 0 = use default BreathingPeriodMs constant (2250ms)
	BreathingPeriodMs int

	// BreathingAmplitude is the custom breathing intensity variation for emitted light.
	// 0 = use default BreathingAmplitude constant (0.065)
	BreathingAmplitude float64

	// IndependentBreathing varies the breathing period per-source based on position.
	// When true, each instance breathes at a slightly different rate (±15% variation).
	// When false (default), all sources of this type breathe at the same rate.
	// Use for fire sources to create organic flickering; leave false for magical sources.
	IndependentBreathing bool

	// Radius overrides the tile's LightEmission if non-zero
	Radius int

	// TintMode controls when this light source applies color tinting.
	// TintNightOnly (default): tint only at night
	// TintHalfDuringDay: full tint at night, 50% during day
	// TintAlways: full tint at all times
	TintMode TintMode
}

// LightTintColor represents RGB scale factors for light tinting.
// Values around 1.0 are neutral. > 1.0 brightens, < 1.0 darkens.
// These are NOT raw colors but multipliers applied to existing tile colors.
type LightTintColor struct {
	R, G, B float32
}

// LighthouseMarkerIndex is a special sprite index marker for lighthouse light sources.
// Used to identify lighthouse contributions in the lit tiles map.
const LighthouseMarkerIndex indexes.SpriteIndex = 0xFFFE

// MoongateMarkerIndex is a special sprite index marker for moongate light sources.
// Used to identify moongate contributions in the lit tiles map for purple/blue tinting.
const MoongateMarkerIndex indexes.SpriteIndex = 0xFFFD

// ============================================================================
// NPC Light Source Marker Indexes
// ============================================================================
// These marker indexes identify magical NPCs that emit light.
// Used for tint color lookup in LightSourcePropertiesMap.
// Allocation: 0xFFFC and below (0xFFFE=Lighthouse, 0xFFFD=Moongate)

const (
	// WispMarkerIndex marks wisps, ethereal energy beings.
	WispMarkerIndex indexes.SpriteIndex = 0xFFFC
	// SeahorseMarkerIndex marks seahorses, magical sea creatures.
	SeahorseMarkerIndex indexes.SpriteIndex = 0xFFFB
	// DaemonMarkerIndex marks daemons, demonic fire creatures.
	DaemonMarkerIndex indexes.SpriteIndex = 0xFFFA
	// GhostMarkerIndex marks ghosts, spectral beings.
	GhostMarkerIndex indexes.SpriteIndex = 0xFFF9
)

// ============================================================================
// Quest Item Light Source Marker Indexes
// ============================================================================
// These marker indexes identify quest items that emit light.
// Used for tint color lookup in LightSourcePropertiesMap.
// Allocation: 0xFFF8 and below

const (
	// ShardMarkerIndex marks the Shards of Evil (corrupted virtue gems).
	ShardMarkerIndex indexes.SpriteIndex = 0xFFF8
	// ArtifactMarkerIndex marks Lord British's Artifacts.
	ArtifactMarkerIndex indexes.SpriteIndex = 0xFFF7
)

// Light tint channel multipliers (see LightTintColor). Fire sources boost red and green
// and cut blue for an orange glow; magical sources do the reverse.
const (
	// torchTint is shared by torches, sconces, braziers, fireplaces, candles and campfires.
	torchTintR, torchTintG, torchTintB = 1.47, 0.93, 0.17
	// cookStoveTint is a redder tint for cooking fires.
	cookStoveTintR, cookStoveTintG, cookStoveTintB = 1.55, 0.75, 0.15
	// lampPostTint is the whiter, cooler tint of street lamps.
	lampPostTintR, lampPostTintG, lampPostTintB = 1.25, 1.15, 0.90
	// blueFlameTint reduces red and green and boosts blue for a clear magical blue glow.
	blueFlameTintR, blueFlameTintG, blueFlameTintB = 0.70, 0.85, 1.45
	// magicWhiteTint is near neutral with a slight blue shift for the In Lor and Vas Lor spells.
	magicWhiteTintR, magicWhiteTintG, magicWhiteTintB = 1.00, 1.00, 1.02
	// moongateTint is a bright, white-shifted blue for moongates.
	moongateTintR, moongateTintG, moongateTintB = 0.82, 0.90, 1.50
	// lighthouseTint is the warm red of the rotating lighthouse beam.
	lighthouseTintR, lighthouseTintG, lighthouseTintB = 1.15, 0.80, 0.80
	// wispTint is an ethereal blue-white glow for energy beings.
	wispTintR, wispTintG, wispTintB = 0.90, 0.95, 1.40
	// seahorseTint is a bright white-cyan glow that stands out against blue water.
	seahorseTintR, seahorseTintG, seahorseTintB = 1.30, 1.45, 1.50
	// daemonTint is a red demonic glow for fire creatures.
	daemonTintR, daemonTintG, daemonTintB = 1.40, 0.85, 0.75
	// ghostTint is a pale blue spectral glow for undead spirits.
	ghostTintR, ghostTintG, ghostTintB = 0.95, 0.98, 1.25
	// ambientSightTint is the neutral gray of the avatar's minimal night vision.
	ambientSightTintR, ambientSightTintG, ambientSightTintB = 0.55, 0.55, 0.55
	// shardTint is a sinister red-purple glow for the corrupted Shards of Evil.
	shardTintR, shardTintG, shardTintB = 1.30, 0.80, 1.20
	// artifactTint is a noble golden-white glow for Lord British's Artifacts.
	artifactTintR, artifactTintG, artifactTintB = 1.22, 1.18, 0.85
)

// Per-source breathing overrides. Fire sources use the default period and
// amplitude except where noted; magical sources pulse at their own rate.
const (
	sconceBreathingAmplitude    = 0.6
	blueFlameBreathingAmplitude = 0.4

	wispBreathingPeriodMs      = 1200 // Fast flicker for energy beings
	wispBreathingAmplitude     = 0.25
	seahorseBreathingPeriodMs  = 1800 // Gentle magical pulse
	seahorseBreathingAmplitude = 0.45
	daemonBreathingPeriodMs    = 1400 // Fire flicker
	daemonBreathingAmplitude   = 0.50
	ghostBreathingPeriodMs     = 2000 // Slow spectral waver
	ghostBreathingAmplitude    = 0.15
	shardBreathingPeriodMs     = 2500 // Slow, ominous pulse
	shardBreathingAmplitude    = 0.20
	artifactBreathingPeriodMs  = 2500 // Slow, majestic pulse
	artifactBreathingAmplitude = 0.35
)

func torchWarm() LightTintColor { return LightTintColor{R: torchTintR, G: torchTintG, B: torchTintB} }
func cookStoveRed() LightTintColor {
	return LightTintColor{R: cookStoveTintR, G: cookStoveTintG, B: cookStoveTintB}
}
func lampPostWhite() LightTintColor {
	return LightTintColor{R: lampPostTintR, G: lampPostTintG, B: lampPostTintB}
}
func blueFlameCool() LightTintColor {
	return LightTintColor{R: blueFlameTintR, G: blueFlameTintG, B: blueFlameTintB}
}
func magicWhite() LightTintColor {
	return LightTintColor{R: magicWhiteTintR, G: magicWhiteTintG, B: magicWhiteTintB}
}
func moongatePurpleTint() LightTintColor {
	return LightTintColor{R: moongateTintR, G: moongateTintG, B: moongateTintB}
}
func lighthouseRed() LightTintColor {
	return LightTintColor{R: lighthouseTintR, G: lighthouseTintG, B: lighthouseTintB}
}
func wispGlow() LightTintColor { return LightTintColor{R: wispTintR, G: wispTintG, B: wispTintB} }
func seahorseGlow() LightTintColor {
	return LightTintColor{R: seahorseTintR, G: seahorseTintG, B: seahorseTintB}
}
func daemonGlow() LightTintColor {
	return LightTintColor{R: daemonTintR, G: daemonTintG, B: daemonTintB}
}
func ghostGlow() LightTintColor { return LightTintColor{R: ghostTintR, G: ghostTintG, B: ghostTintB} }
func ambientSightDim() LightTintColor {
	return LightTintColor{R: ambientSightTintR, G: ambientSightTintG, B: ambientSightTintB}
}
func shardGlow() LightTintColor { return LightTintColor{R: shardTintR, G: shardTintG, B: shardTintB} }
func artifactGlow() LightTintColor {
	return LightTintColor{R: artifactTintR, G: artifactTintG, B: artifactTintB}
}

// neutralTint applies no color change.
func neutralTint() LightTintColor { return LightTintColor{R: 1.0, G: 1.0, B: 1.0} }

// fireLight describes a fire source: half tint by day and independent breathing for organic flicker.
// amplitude 0 uses the default BreathingAmplitude.
func fireLight(tint LightTintColor, amplitude float64) LightSourceProperties {
	return LightSourceProperties{
		TintColor:            tint,
		HasBreathing:         true,
		BreathingPeriodMs:    0,
		BreathingAmplitude:   amplitude,
		IndependentBreathing: true,
		Radius:               0,
		TintMode:             TintHalfDuringDay,
	}
}

// steadyLight describes a non-breathing source that is half tinted by day.
func steadyLight(tint LightTintColor) LightSourceProperties {
	return LightSourceProperties{
		TintColor:            tint,
		HasBreathing:         false,
		BreathingPeriodMs:    0,
		BreathingAmplitude:   0,
		IndependentBreathing: false,
		Radius:               0,
		TintMode:             TintHalfDuringDay,
	}
}

// pulsingLight describes a magical source whose breathing is synchronized across instances.
// periodMs 0 uses the default BreathingPeriodMs.
func pulsingLight(tint LightTintColor, mode TintMode, periodMs int, amplitude float64) LightSourceProperties {
	return LightSourceProperties{
		TintColor:            tint,
		HasBreathing:         true,
		BreathingPeriodMs:    periodMs,
		BreathingAmplitude:   amplitude,
		IndependentBreathing: false,
		Radius:               0,
		TintMode:             mode,
	}
}

// LightSourcePropertiesMap maps sprite indexes to their light properties.
//
//nolint:gochecknoglobals // Read-only light table looked up per tile while drawing.
var LightSourcePropertiesMap = map[indexes.SpriteIndex]LightSourceProperties{
	indexes.RightSconce:   fireLight(torchWarm(), sconceBreathingAmplitude),
	indexes.LeftSconce:    fireLight(torchWarm(), sconceBreathingAmplitude),
	indexes.Brazier:       fireLight(torchWarm(), 0),
	indexes.Fireplace:     fireLight(torchWarm(), 0),
	indexes.FireLogs:      fireLight(torchWarm(), 0),
	indexes.CampFire:      fireLight(torchWarm(), 0),
	indexes.CandleOnTable: fireLight(torchWarm(), 0),
	indexes.FireField:     fireLight(torchWarm(), 0),
	indexes.ItemTorch:     fireLight(torchWarm(), 0),

	// Street lamps and the lighthouse beam are steady; the cook stove burns a redder, steady flame.
	indexes.LampPost:      steadyLight(lampPostWhite()),
	indexes.CookStove:     steadyLight(cookStoveRed()),
	LighthouseMarkerIndex: steadyLight(lighthouseRed()),

	indexes.BlueFlame: pulsingLight(blueFlameCool(), TintAlways, 0, blueFlameBreathingAmplitude),

	// Moongate mystical glow: a faster, stronger pulse than fire.
	MoongateMarkerIndex: pulsingLight(
		moongatePurpleTint(), TintAlways, MoongateBreathingPeriodMs, MoongateBreathingAmplitude,
	),

	// Magical NPCs.
	WispMarkerIndex: pulsingLight(wispGlow(), TintAlways, wispBreathingPeriodMs, wispBreathingAmplitude),
	SeahorseMarkerIndex: pulsingLight(
		seahorseGlow(), TintHalfDuringDay, seahorseBreathingPeriodMs, seahorseBreathingAmplitude,
	),
	DaemonMarkerIndex: pulsingLight(daemonGlow(), TintAlways, daemonBreathingPeriodMs, daemonBreathingAmplitude),
	GhostMarkerIndex:  pulsingLight(ghostGlow(), TintHalfDuringDay, ghostBreathingPeriodMs, ghostBreathingAmplitude),

	// Quest items.
	ShardMarkerIndex: pulsingLight(shardGlow(), TintAlways, shardBreathingPeriodMs, shardBreathingAmplitude),
	ArtifactMarkerIndex: pulsingLight(
		artifactGlow(), TintHalfDuringDay, artifactBreathingPeriodMs, artifactBreathingAmplitude,
	),
}

// PlayerLightProperties returns light properties for player-carried light sources.
// ambientRadius is the configured night-vision radius used for PlayerLightAmbient.
func PlayerLightProperties(lightType PlayerLightType, ambientRadius int) LightSourceProperties {
	switch lightType {
	case PlayerLightAmbient:
		return playerLight(ambientSightDim(), false, resolveAmbientLightRadius(ambientRadius))
	case PlayerLightTorch:
		return playerLight(torchWarm(), true, TorchTileDistance)
	case PlayerLightInLor:
		return playerLight(magicWhite(), false, InLorTileDistance)
	case PlayerLightVasLor:
		return playerLight(magicWhite(), false, VasLorTileDistance)
	case PlayerLightNone:
	}

	return playerLight(LightTintColor{R: 0, G: 0, B: 0}, false, 0)
}

// playerLight describes a carried light of the given radius.
func playerLight(tint LightTintColor, breathes bool, radius int) LightSourceProperties {
	return LightSourceProperties{
		TintColor:            tint,
		HasBreathing:         breathes,
		BreathingPeriodMs:    0,
		BreathingAmplitude:   0,
		IndependentBreathing: false,
		Radius:               radius,
		TintMode:             TintNightOnly,
	}
}

// GetLightSourceProperties returns the light properties for a sprite index.
func GetLightSourceProperties(spriteIndex indexes.SpriteIndex) LightSourceProperties {
	if props, ok := LightSourcePropertiesMap[spriteIndex]; ok {
		return props
	}

	return playerLight(neutralTint(), false, 0)
}

// ============================================================================
// Light Contribution - tracks a single light source's effect on a tile
// ============================================================================

// LightContribution represents one light source's contribution to a tile.
type LightContribution struct {
	SourceIndex indexes.SpriteIndex
	SourcePos   references.Position
	Distance    float64 // Actual distance (not squared)
	Radius      int     // Max radius of this source
	Weight      float64 // Computed weight based on distance
}

// ============================================================================
// Light Source Tinting State
// ============================================================================

// LightSourceTintingState tracks which tiles are lit by light sources for tinting.
// Supports blending multiple overlapping light sources smoothly.
type LightSourceTintingState struct {
	// litTilesMap maps positions to all light sources affecting that tile
	litTilesMap map[references.Position][]LightContribution

	// isLargeMap indicates if operating on wrapping 256x256 map
	isLargeMap bool

	// playerLightType tracks the current player light source
	playerLightType PlayerLightType

	// playerPos tracks the player's current position
	playerPos references.Position

	// ambientLightRadius is the configured night-vision radius, at least 1.
	ambientLightRadius int
}

// SetAmbientLightRadius sets the night-vision radius used for the ambient player light.
// Radii below 1 fall back to the default of 2.
func (ls *LightSourceTintingState) SetAmbientLightRadius(radius int) {
	ls.ambientLightRadius = resolveAmbientLightRadius(radius)
}

// NewLightSourceTintingState creates a new light source tinting state.
func NewLightSourceTintingState() *LightSourceTintingState {
	return &LightSourceTintingState{
		litTilesMap:     make(map[references.Position][]LightContribution),
		isLargeMap:      false,
		playerLightType: PlayerLightNone,
		playerPos:       references.Position{X: 0, Y: 0},

		ambientLightRadius: defaultAmbientLightRadius,
	}
}

// SetLargeMapMode sets whether the tinting operates on a wrapping large map.
func (ls *LightSourceTintingState) SetLargeMapMode(isLargeMap bool) {
	ls.isLargeMap = isLargeMap
}

// BuildLitTilesMap recalculates which tiles are lit by light sources.
// Only light sources visible to the player (determined by visibleFlags) will contribute.
// This ensures that light from behind walls doesn't bleed through.
// Pass nil for visibleFlags to include all light sources (useful for tests).
func (ls *LightSourceTintingState) BuildLitTilesMap(
	lightSources LightSources,
	visibleFlags VisibilityCoords,
) {
	ls.litTilesMap = make(map[references.Position][]LightContribution)

	for _, source := range lightSources {
		// Skip light sources that aren't visible to the player
		// This prevents light from bleeding through walls
		if visibleFlags != nil && !visibleFlags[source.Pos.X][source.Pos.Y] {
			continue
		}

		spriteIndex := source.Tile.Index
		radius := source.Tile.GetLightEmission()
		if radius <= 0 {
			continue
		}

		ls.addLitTilesForSource(source.Pos, spriteIndex, radius)
	}
}

// addLitTilesForSource adds light contributions for a single source.
func (ls *LightSourceTintingState) addLitTilesForSource(
	sourcePos references.Position,
	spriteIndex indexes.SpriteIndex,
	radius int,
) {
	// Use radius + 0.5 to match lighting.go's applyLightSource calculation.
	// This ensures tinting covers the exact same tiles that visibility does.
	// Without this, edge tiles would be lit but not tinted.
	maxDist := float64(radius) + lightRadiusPadding

	for deltaY := -radius; deltaY <= radius; deltaY++ {
		for deltaX := -radius; deltaX <= radius; deltaX++ {
			dist := sqrt(float64(deltaX*deltaX + deltaY*deltaY))
			if dist > maxDist {
				continue // Outside circle
			}

			pos := references.Position{
				X: sourcePos.X + tileOffset(deltaX),
				Y: sourcePos.Y + tileOffset(deltaY),
			}

			if ls.isLargeMap {
				pos = *pos.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
			}

			// Calculate weight using inverse distance with falloff exponent
			// Weight = 1 / (distance + 1)^exponent
			weight := 1.0 / pow(dist+1.0, FalloffExponent)

			ls.addContribution(pos, LightContribution{
				SourceIndex: spriteIndex,
				SourcePos:   sourcePos,
				Distance:    dist,
				Radius:      radius,
				Weight:      weight,
			})
		}
	}
}

// addContribution records a contribution on a tile. A tile keeps at most MaxSourcesPerTile
// contributions; beyond that the weakest is replaced when the new one is stronger.
func (ls *LightSourceTintingState) addContribution(pos references.Position, contribution LightContribution) {
	existing := ls.litTilesMap[pos]
	if len(existing) < MaxSourcesPerTile {
		ls.litTilesMap[pos] = append(existing, contribution)

		return
	}

	minIdx := 0
	minWeight := existing[0].Weight
	for i, c := range existing {
		if c.Weight < minWeight {
			minWeight = c.Weight
			minIdx = i
		}
	}
	if contribution.Weight > minWeight {
		existing[minIdx] = contribution
	}
}

// UpdatePlayerLight updates the lit tiles map with the player's current light source.
func (ls *LightSourceTintingState) UpdatePlayerLight(
	playerPos references.Position,
	lightType PlayerLightType,
) {
	// Clear previous player light contribution if position or type changed
	if ls.playerLightType != PlayerLightNone && (lightType != ls.playerLightType || playerPos != ls.playerPos) {
		ls.clearPlayerLight()
	}

	ls.playerLightType = lightType
	ls.playerPos = playerPos

	if lightType == PlayerLightNone {
		return
	}

	props := PlayerLightProperties(lightType, ls.ambientLightRadius)
	if props.Radius <= 0 {
		return
	}

	// Ambient sight and the magic light spells have no sprite; sprite 0 marks them.
	var spriteIndex indexes.SpriteIndex
	if lightType == PlayerLightTorch {
		spriteIndex = indexes.ItemTorch
	}

	ls.addLitTilesForSource(playerPos, spriteIndex, props.Radius)
}

// clearPlayerLight removes the player's light contribution from tiles.
func (ls *LightSourceTintingState) clearPlayerLight() {
	if ls.playerLightType == PlayerLightNone {
		return
	}

	props := PlayerLightProperties(ls.playerLightType, ls.ambientLightRadius)
	radius := props.Radius

	for deltaY := -radius; deltaY <= radius; deltaY++ {
		for deltaX := -radius; deltaX <= radius; deltaX++ {
			pos := references.Position{
				X: ls.playerPos.X + tileOffset(deltaX),
				Y: ls.playerPos.Y + tileOffset(deltaY),
			}

			if ls.isLargeMap {
				pos = *pos.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
			}

			// Remove contributions from this player position
			if contributions, ok := ls.litTilesMap[pos]; ok {
				filtered := contributions[:0]
				for _, c := range contributions {
					if c.SourcePos != ls.playerPos {
						filtered = append(filtered, c)
					}
				}
				if len(filtered) > 0 {
					ls.litTilesMap[pos] = filtered
				} else {
					delete(ls.litTilesMap, pos)
				}
			}
		}
	}
}

// GetTintScaleFactors returns the RGB scale factors for a tile at the given position.
// Returns nil if the tile is not lit by any light source (or all sources are filtered out).
// The elapsedMs parameter is used for breathing animation.
// The ambientDarkness parameter controls TintMode filtering and smoothly ranges
// from 0 during full daylight to 1 at night:
//   - TintNightOnly sources scale with ambient darkness
//   - TintHalfDuringDay sources scale from 50% intensity to full intensity
//   - TintAlways sources apply at full intensity always
//
// Color tint and brightness falloff are handled separately:
// - Color tint (warm/cool) stays consistent across the lit area
// - Brightness falls off with distance (darker at edges)
// This creates the effect of "same color, just dimmer" at edges.
func (ls *LightSourceTintingState) GetTintScaleFactors(
	pos references.Position,
	elapsedMs int64,
	ambientDarkness float32,
) *LightTintColor {
	return ls.getTintScaleFactors(pos, elapsedMs, ambientDarkness, 1)
}

// GetTintScaleFactorsWithDaylight composes source tint with a local daylight target.
// daylightTarget must be finite and in [0.3, 1]; invalid targets panic.
// ambientDarkness remains global clock darkness for TintMode and color intensity.
// Only sources eligible in full daylight can raise daytime brightness above the
// target, up to 1, before adding color deltas. Target 1 and full night preserve
// GetTintScaleFactors exactly.
// Unlit tiles and fully filtered contributions return nil for caller ambient fallback.
func (ls *LightSourceTintingState) GetTintScaleFactorsWithDaylight(
	pos references.Position,
	elapsedMs int64,
	ambientDarkness float32,
	daylightTarget float32,
) *LightTintColor {
	const minimumDaylightTarget float32 = 0.3
	if !(daylightTarget >= minimumDaylightTarget && daylightTarget <= 1) {
		panic("daylight target must be finite and in [0.3, 1]")
	}

	return ls.getTintScaleFactors(pos, elapsedMs, ambientDarkness, daylightTarget)
}

//nolint:cyclop,funlen // Keep source filtering and daylight composition together to preserve legacy arithmetic.
func (ls *LightSourceTintingState) getTintScaleFactors(
	pos references.Position,
	elapsedMs int64,
	ambientDarkness float32,
	daylightTarget float32,
) *LightTintColor {
	contributions, ok := ls.litTilesMap[pos]
	if !ok || len(contributions) == 0 {
		return nil
	}
	darkness := clampFloat(float64(ambientDarkness), 0, 1)

	// Blend all contributions weighted by inverse distance
	// We accumulate color DELTAS (deviation from neutral 1.0), not absolute values
	var totalDeltaR, totalDeltaG, totalDeltaB float64
	var totalWeight float64
	closestDist := noSourceDistance
	var closestRadius int
	closestDayDist := math.Inf(1)
	var closestDayRadius int

	for _, contribution := range contributions {
		props := GetLightSourceProperties(contribution.SourceIndex)

		// Determine tint multiplier based on TintMode and ambient darkness.
		var tintMultiplier = 1.0
		switch props.TintMode {
		case TintNightOnly:
			if darkness == 0 {
				continue
			}
			tintMultiplier = darkness
		case TintHalfDuringDay:
			tintMultiplier = halfDayTintMultiplier + halfDayTintMultiplier*darkness
		case TintAlways:
			tintMultiplier = 1.0
		}

		// Get base tint color deltas (how much each channel deviates from 1.0)
		deltaR := (float64(props.TintColor.R) - 1.0) * tintMultiplier
		deltaG := (float64(props.TintColor.G) - 1.0) * tintMultiplier
		deltaB := (float64(props.TintColor.B) - 1.0) * tintMultiplier

		// Apply breathing modulation if enabled (affects color intensity)
		if props.HasBreathing {
			// Use custom breathing parameters if specified, otherwise use defaults
			periodMs := props.BreathingPeriodMs
			if periodMs == 0 {
				periodMs = BreathingPeriodMs
			}
			amplitude := props.BreathingAmplitude
			if amplitude == 0 {
				amplitude = BreathingAmplitude
			}

			breathMod := ls.calculateBreathingModulationWithParams(
				contribution.SourcePos, elapsedMs, periodMs, amplitude, props.IndependentBreathing,
			)
			deltaR *= breathMod
			deltaG *= breathMod
			deltaB *= breathMod
		}

		// Apply tint strength to dampen the color effect
		deltaR *= TintStrength
		deltaG *= TintStrength
		deltaB *= TintStrength

		// Accumulate weighted color deltas
		totalDeltaR += deltaR * contribution.Weight
		totalDeltaG += deltaG * contribution.Weight
		totalDeltaB += deltaB * contribution.Weight
		totalWeight += contribution.Weight

		// Track closest source for brightness falloff
		if contribution.Distance < closestDist {
			closestDist = contribution.Distance
			closestRadius = contribution.Radius
		}
		if props.TintMode != TintNightOnly && contribution.Distance < closestDayDist {
			closestDayDist = contribution.Distance
			closestDayRadius = contribution.Radius
		}
	}

	if totalWeight == 0 {
		return nil
	}

	// Calculate weighted average of color deltas
	avgDeltaR := totalDeltaR / totalWeight
	avgDeltaG := totalDeltaG / totalWeight
	avgDeltaB := totalDeltaB / totalWeight

	// Preserve the legacy nighttime falloff and outdoor interpolation arithmetic.
	var brightnessMult = 1.0
	nightBrightness := 1.0
	if darkness > 0 && closestRadius > 0 {
		// Use radius + 0.5 to match the extended lit area (same as addLitTilesForSource)
		maxDist := float64(closestRadius) + lightRadiusPadding
		distRatio := clampFloat(closestDist/maxDist, 0, 1)
		nightBrightness = CenterBrightness + (BaseBrightness-CenterBrightness)*distRatio
		brightnessMult = 1 + (nightBrightness-1)*darkness
	}
	if daylightTarget < 1 && darkness < 1 {
		// Night-only sources must not lift the daytime endpoint when dawn/dusk starts.
		dayBrightness := float64(daylightTarget)
		if closestDayRadius > 0 {
			maxDist := float64(closestDayRadius) + lightRadiusPadding
			distRatio := clampFloat(closestDayDist/maxDist, 0, 1)
			daySourceBrightness := CenterBrightness + (BaseBrightness-CenterBrightness)*distRatio
			dayBrightness = max(dayBrightness, min(1.0, daySourceBrightness))
		} else if !math.IsInf(closestDayDist, 1) {
			// An eligible source without a positive radius retains the default scalar of 1.
			dayBrightness = 1
		}
		brightnessMult = dayBrightness + (nightBrightness-dayBrightness)*darkness
	}

	// Combine brightness falloff with color tint:
	// - Full daylight: source brightness has a daylightTarget floor and a ceiling of 1
	// - Dawn/dusk: brightness and night-specific color deltas fade smoothly
	// - Nighttime: brightnessMult varies (1.05 at center, 0.55 at edge), plus color deltas
	finalR := float32(clampFloat(brightnessMult+avgDeltaR, minTintChannel, maxTintChannel))
	finalG := float32(clampFloat(brightnessMult+avgDeltaG, minTintChannel, maxTintChannel))
	finalB := float32(clampFloat(brightnessMult+avgDeltaB, minTintChannel, maxTintChannel))

	return &LightTintColor{R: finalR, G: finalG, B: finalB}
}

// calculateBreathingModulationWithParams returns breathing modulation with custom parameters.
// periodMs controls how long one complete breathing cycle takes.
// amplitude controls how much the intensity varies (0.10 = ±10% variation).
// independentBreathing varies the period by ±15% based on position for organic fire effects.
// Uses position-based phase offset so different sources breathe at different phases.
func (ls *LightSourceTintingState) calculateBreathingModulationWithParams(
	sourcePos references.Position,
	elapsedMs int64,
	periodMs int,
	amplitude float64,
	independentBreathing bool,
) float64 {
	// Apply period variation for independent breathing (organic fire effect)
	// Each source gets a slightly different period based on its position
	posHash := sourcePos.AnimationHash()
	if independentBreathing {
		// Hash to get 0.85 to 1.15 multiplier (±15% variation)
		step := float64(posHash % independentPeriodSteps)
		variation := independentPeriodMin + independentPeriodSpread*step/independentPeriodSteps
		periodMs = int(float64(periodMs) * variation)
	}

	// Position-based phase offset (different sources out of sync)
	phaseOffset := int64(posHash) * breathingPhaseHashMultiplier
	adjustedTime := elapsedMs + phaseOffset

	// Calculate breathing phase (0 to 2π)
	phase := float64(adjustedTime%int64(periodMs)) / float64(periodMs) * fullCircle

	// Sine wave oscillation: returns -1 to 1, then scaled to (1-amp) to (1+amp)
	// When result is 1.0, we get full tint. When less, tint is reduced.
	breathValue := 1.0 + sin(phase)*amplitude

	return breathValue
}

// Clear removes all lit tiles (used when leaving a map).
func (ls *LightSourceTintingState) Clear() {
	ls.litTilesMap = make(map[references.Position][]LightContribution)
	ls.playerLightType = PlayerLightNone
}

// ============================================================================
// Lighthouse Integration
// ============================================================================

// InjectLighthouseTiles adds lighthouse-lit tiles to the tinting state.
// This integrates the lighthouse's rotating beam with the unified light source system.
// Each lit tile gets distance-based brightness falloff from the nearest beacon.
func (ls *LightSourceTintingState) InjectLighthouseTiles(lighthouseState *LighthouseState) {
	if lighthouseState == nil || !lighthouseState.HasLighthouses() {
		return
	}

	litTiles := lighthouseState.GetLitTiles()
	beaconPositions := lighthouseState.GetPositions()

	if len(litTiles) == 0 || len(beaconPositions) == 0 {
		return
	}

	// Estimate max distance for lighthouse beams (based on cone shapes, ~7 tiles max)
	const lighthouseMaxRadius = 7

	for tilePos := range litTiles {
		// Find distance to nearest beacon for brightness falloff
		minDist := noSourceDistance
		for _, beaconPos := range beaconPositions {
			dx := float64(tilePos.X - beaconPos.X)
			dy := float64(tilePos.Y - beaconPos.Y)
			dist := sqrt(dx*dx + dy*dy)
			if dist < minDist {
				minDist = dist
			}
		}

		// Calculate weight based on distance (same formula as other lights)
		weight := 1.0 / pow(minDist+1.0, FalloffExponent)

		contribution := LightContribution{
			SourceIndex: LighthouseMarkerIndex,
			SourcePos:   beaconPositions[0], // Use first beacon as source pos
			Distance:    minDist,
			Radius:      lighthouseMaxRadius,
			Weight:      weight,
		}

		ls.addContribution(tilePos, contribution)
	}

	// Also add beacon positions themselves (they glow too)
	for _, beaconPos := range beaconPositions {
		contribution := LightContribution{
			SourceIndex: LighthouseMarkerIndex,
			SourcePos:   beaconPos,
			Distance:    0, // Beacon is at distance 0 from itself
			Radius:      lighthouseMaxRadius,
			Weight:      1.0, // Full weight at source
		}

		existing := ls.litTilesMap[beaconPos]
		if len(existing) < MaxSourcesPerTile {
			ls.litTilesMap[beaconPos] = append(existing, contribution)
		}
	}
}
