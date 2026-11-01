package map_state

import (
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/helpers"
)

const (
	// TorchTileDistance is the illumination radius of a carried torch.
	TorchTileDistance = 3
	// DefaultTorchDurationMinutes is the initial fuel time of a lit torch.
	DefaultTorchDurationMinutes = 240
	// BorrowedWallTorchDurationMinutes is the burn a wall torch gives when taken
	// from its sconce; it replaces rather than extends carried fuel. ref:LIGHT-0004
	BorrowedWallTorchDurationMinutes = 100

	// MoongateLightDistance is the radius in tiles of a moongate's mystical light.
	MoongateLightDistance = 4

	// ShardLightDistance is the radius in tiles of the light from a Shard of Evil.
	ShardLightDistance = 3
	// ArtifactLightDistance is the radius in tiles of the stronger light from Lord British's Artifacts.
	ArtifactLightDistance = 4

	// MagicLightTileDistance is the original U5 radius for both In Lor and Vas Lor.
	MagicLightTileDistance = 5
	// InLorTileDistance is the radius in tiles of the In Lor spell light.
	InLorTileDistance = MagicLightTileDistance
	// VasLorTileDistance is the radius in tiles of the Vas Lor spell light.
	VasLorTileDistance = MagicLightTileDistance

	// WispLightDistance is the radius in tiles of a wisp's blue-white glow. Magical NPCs
	// all start at 2 tiles for a subtle glow.
	WispLightDistance = 2
	// SeahorseLightDistance is the radius in tiles of a seahorse's bright blue glow.
	SeahorseLightDistance = 2
	// DaemonLightDistance is the radius in tiles of a daemon's red glow.
	DaemonLightDistance = 2
	// GhostLightDistance is the radius in tiles of a ghost's pale blue glow.
	GhostLightDistance = 2
)

const (
	// lightRadiusPadding extends a light's radius so the lit disc outline stays round;
	// visibility and tinting both use it so edge tiles are lit and tinted together.
	lightRadiusPadding = 0.5
	// screenCentreDivisor halves the visible tile extents to get the distance from screen centre to a corner.
	screenCentreDivisor = 2
	// baselineVisibilityFactor is the lowest light factor (10%); it still lights a radius-1 neighbourhood.
	baselineVisibilityFactor = 0.1
)

// Lighting tracks the carried torch and magic light and builds light distance maps for the visible area.
type Lighting struct {
	torchMinutesRemaining int
	gameDimensions        GameDimensions
	baselineFactor        float32
	baselineRadius        int

	// Magical light is enabled while GameState has remaining magic_light minutes.
	hasMagicLight bool

	// ambientLightRadius is the configured night-vision radius, at least 1.
	ambientLightRadius int
}

// LightSource represents a tile that emits light in the game world
type LightSource struct {
	Tile *references.Tile
	Pos  references.Position
}

// LightSources is a collection of light sources
type LightSources []LightSource

// DistanceMap maps each lit tile to its whole-tile distance from the nearest light source.
type DistanceMap map[references.Position]int

// NewLighting creates an unlit Lighting for the given view dimensions.
func NewLighting(gameDimensions GameDimensions) Lighting {
	return Lighting{
		torchMinutesRemaining: 0,
		gameDimensions:        gameDimensions,
		baselineFactor:        baselineVisibilityFactor,
		baselineRadius:        1,
		hasMagicLight:         false,
		ambientLightRadius:    defaultAmbientLightRadius,
	}
}

// SetAmbientLightRadius sets the night-vision radius of the ambient player light.
// Radii below 1 fall back to the default of 2.
func (l *Lighting) SetAmbientLightRadius(radius int) {
	l.ambientLightRadius = resolveAmbientLightRadius(radius)
}

// AmbientLightRadius returns the effective night-vision radius, at least 1.
func (l *Lighting) AmbientLightRadius() int {
	return resolveAmbientLightRadius(l.ambientLightRadius)
}

// HasTorchLit returns true while the carried torch has fuel left.
func (l *Lighting) HasTorchLit() bool {
	return l.torchMinutesRemaining > 0
}

// TorchMinutesRemaining returns the carried torch's authoritative fuel time.
func (l *Lighting) TorchMinutesRemaining() int {
	return l.torchMinutesRemaining
}

// SetMagicLight enables or disables player-carried magical light.
func (l *Lighting) SetMagicLight(active bool) {
	l.hasMagicLight = active
}

// HasMagicLight returns true if magic light is active.
func (l *Lighting) HasMagicLight() bool {
	return l.hasMagicLight
}

// HasAnyLight returns true if torch OR magic light is providing illumination.
func (l *Lighting) HasAnyLight() bool {
	return l.HasTorchLit() || l.hasMagicLight
}

// LightTorch lights a torch with a full default fuel time.
func (l *Lighting) LightTorch() {
	l.torchMinutesRemaining = DefaultTorchDurationMinutes
}

// LightBorrowedWallTorch replaces carried torch fuel with a borrowed wall
// torch's shorter burn. It never adds to the existing fuel. ref:LIGHT-0004
func (l *Lighting) LightBorrowedWallTorch() {
	l.torchMinutesRemaining = BorrowedWallTorchDurationMinutes
}

// ExtendTorchMinutes adds carried fuel and saturates at maximum.
func (l *Lighting) ExtendTorchMinutes(minutes, maximum int) int {
	if minutes < 0 {
		panic("torch extension minutes cannot be negative")
	}
	if maximum < 0 {
		panic("torch maximum cannot be negative")
	}
	l.torchMinutesRemaining = min(l.torchMinutesRemaining+minutes, maximum)

	return l.torchMinutesRemaining
}

// RestoreTorchMinutes restores carried fuel without consuming another torch.
func (l *Lighting) RestoreTorchMinutes(minutes int) {
	if minutes < 0 {
		panic("carried torch minutes cannot be negative")
	}
	l.torchMinutesRemaining = minutes
}

// ExtinguishTorch immediately extinguishes the torch (used by death resurrection).
func (l *Lighting) ExtinguishTorch() {
	l.torchMinutesRemaining = 0
}

// AdvanceMinutes burns a carried torch by actual elapsed gameplay minutes.
func (l *Lighting) AdvanceMinutes(minutes int) {
	if minutes <= 0 {
		return
	}
	l.torchMinutesRemaining = helpers.Max(l.torchMinutesRemaining-minutes, 0)
}

// BuildGameScreenDistanceMap builds the distance map for a light at centrePos sized to the whole game screen.
func (l *Lighting) BuildGameScreenDistanceMap(centrePos references.Position) DistanceMap {
	dmm := make(DistanceMap)
	l.applyLightSource(dmm, centrePos, 0, 1)

	return dmm
}

// BuildLightSourceDistanceMap builds the distance map from the player-carried light and every visible light source.
func (l *Lighting) BuildLightSourceDistanceMap(
	lightSources LightSources,
	visibleFlags VisibilityCoords,
	playerLightType PlayerLightType,
	centerPos references.Position,
) DistanceMap {
	distanceMaskMap := make(DistanceMap)

	// Apply player-carried light based on light type (torch, magic light, ambient)
	// This uses the same PlayerLightProperties() as the tinting system for consistency
	if playerLightType != PlayerLightNone {
		props := PlayerLightProperties(playerLightType, l.ambientLightRadius)
		if props.Radius > 0 {
			l.applyLightSource(distanceMaskMap, centerPos, props.Radius, 1)
		}
	}

	for _, ls := range lightSources {
		if visibleFlags[ls.Pos.X][ls.Pos.Y] {
			l.applyLightSource(distanceMaskMap, ls.Pos, ls.Tile.GetLightEmission(), 1)
		}
	}

	return distanceMaskMap
}

func (l *Lighting) applyLightSource(
	distanceMap DistanceMap,
	centrePos references.Position,
	nRadius int, // 0 means “use factor”
	fFactor float32, // ignored when radius > 0
) {
	// Decide radius
	if nRadius <= 0 {
		nRadius = l.radiusForFactor(fFactor)
	}
	if nRadius <= 0 {
		return // nothing to do
	}

	// (r + 0.5)² → keeps outline round
	edge := float64(nRadius) + lightRadiusPadding
	maxDist2 := edge * edge

	bottomRight := l.gameDimensions.GetBottomRightWithoutOverflow()
	isWrapped := l.gameDimensions.IsWrappedMap()

	// Reuse single Position struct to avoid allocations in nested loops
	var position references.Position

	for deltaY := -nRadius; deltaY <= nRadius; deltaY++ {
		yy := deltaY * deltaY
		for deltaX := -nRadius; deltaX <= nRadius; deltaX++ {
			dist2 := float64(deltaX*deltaX + yy)
			if dist2 > maxDist2 { // outside disc
				continue
			}

			// Update position in-place instead of creating new struct
			position.X = centrePos.X + tileOffset(deltaX)
			position.Y = centrePos.Y + tileOffset(deltaY)

			if isWrapped {
				position = *position.GetWrapped(bottomRight.X, bottomRight.Y)
			}

			d := int(math.Sqrt(dist2)) // Euclidean distance 0‥radius
			if cur := distanceMap[position]; cur == 0 || d < cur {
				distanceMap[position] = d
			}
		}
	}
}

func (l *Lighting) radiusForFactor(factor float32) int {
	switch {
	case factor <= l.baselineFactor:
		return l.baselineRadius
	case factor >= 1:
		return l.maxRadius()
	default:
		norm := (factor - l.baselineFactor) / (1 - l.baselineFactor) // 0‥1

		return l.baselineRadius +
			int(math.Round(float64(norm)*float64(l.maxRadius()-l.baselineRadius)))
	}
}

func (l *Lighting) maxRadius() int {
	xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen := l.gameDimensions.GetTilesVisibleOnScreen()

	return int(math.Ceil(
		math.Hypot(
			float64(xTilesVisibleOnGameScreen)/screenCentreDivisor,
			float64(yTilesVisibleOnGameScreen)/screenCentreDivisor,
		),
	))
}
