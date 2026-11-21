package main

import (
	"fmt"

	rng "github.com/bradhannah/berad-sdk/modules/berad-rng"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/visual"
)

const (
	wandererDissolveSeed = 0x57414E444552
	avatarDissolveSeed   = 0x415641544152
)

// =============================================================================
// Generic Tile Dissolve System - Enums and Types
// =============================================================================

// TileDissolvePositionMode determines how a dissolve position is interpreted
type TileDissolvePositionMode int

const (
	// PositionModeWorld interprets position relative to player (standard world coordinates)
	// Used for: item spawns, monster summoning, horse materialization
	PositionModeWorld TileDissolvePositionMode = iota

	// PositionModeFixedGrid interprets position on fixed 11x11 centered grid
	// Used for: death scene apparitions, camping wanderer (position fixed regardless of player)
	PositionModeFixedGrid
)

// TileDissolveRenderLayer determines when a dissolve effect is rendered in the draw order
type TileDissolveRenderLayer int

const (
	// RenderLayerDefault renders before fade overlay (standard map entities)
	// Used for: item spawns, monster summoning - visible on the game map
	RenderLayerDefault TileDissolveRenderLayer = iota

	// RenderLayerOverFade renders after fade overlay (on top of black screen)
	// Used for: death scene apparitions that appear on the black background
	RenderLayerOverFade
)

// TileDissolveOptions configures a new tile dissolve animation.
// Used with StartTileDissolveWithOptions() for full control over positioning and rendering.
type TileDissolveOptions struct {
	Position     references.Position      // Position to render at (interpretation depends on PositionMode)
	SpriteIndex  indexes.SpriteIndex      // Sprite to render during dissolve
	Mode         visual.DissolveMode      // DissolveModeReveal or DissolveModeHide
	OutlineColor sprites.OutlineColorType // Outline color (NoOutline, WhiteOutline, etc.)
	PositionMode TileDissolvePositionMode // How to interpret position (world vs fixed grid)
	RenderLayer  TileDissolveRenderLayer  // When to render in draw order (before/after fade)
}

// TileDissolveState represents an active tile dissolve animation.
// Used for generic tile materialization/dematerialization effects
// (e.g., death scene apparitions, camping wanderer, item materialization).
type TileDissolveState struct {
	ID           int                      // Unique identifier for updates/removal
	Effect       DissolveEffectState      // The dissolve effect state (encapsulates GPU effect, buffer, level)
	Position     references.Position      // Position to render at (interpretation depends on PositionMode)
	SpriteIndex  indexes.SpriteIndex      // Sprite to render
	OutlineColor sprites.OutlineColorType // Outline color type (replaces bool WithOutline)
	PositionMode TileDissolvePositionMode // How to interpret position
	RenderLayer  TileDissolveRenderLayer  // When to render in draw order
}

// Teleportation visual effect callbacks for SystemCallbacks integration

func (g *GameScene) setWandererDissolveLevel(level float64) {
	// Update DissolveEffectState (handles both level and effect progress)
	g.wandererDissolve.UpdateProgress(level)

	// Also update the camping scene state for consistency
	if g.gameState.CampingScene != nil {
		g.gameState.CampingScene.SetWandererDissolveLevel(level)
	}

	logging.Debugf(logging.CategoryGameState, "Wanderer dissolve level set to %.2f", level)
}

// initializeWandererDissolveEffect creates a new dissolve effect for the wanderer sprite
// Called when camping starts and wanderer appearance begins
func (g *GameScene) initializeWandererDissolveEffect() {
	// Get wanderer sprite dimensions (using Apparation1/Wizard1 sprite)
	wandererSprite := g.spriteSheet.GetSprite(indexes.Apparation1)
	if wandererSprite == nil {
		logging.Warnf(logging.CategoryGameState, "Failed to get wanderer sprite for dissolve effect initialization")

		return
	}

	spriteWidth := wandererSprite.Bounds().Dx()
	spriteHeight := wandererSprite.Bounds().Dy()

	// Initialize using DissolveEffectState (encapsulates effect, buffer, and level)
	g.wandererDissolve.Initialize(
		spriteWidth, spriteHeight,
		visual.DissolveModeHide, // Hide mode: starts opaque (1.0) → transparent (0.0)
		rng.New(wandererDissolveSeed),
	)

	logging.Debugf(logging.CategoryGameState, "Initialized wanderer dissolve effect (%dx%d)", spriteWidth, spriteHeight)
}

// cleanupWandererDissolveEffect releases the wanderer dissolve effect resources
// Called when camping ends or wanderer disappears
func (g *GameScene) cleanupWandererDissolveEffect() {
	g.wandererDissolve.Cleanup()
	logging.Debugf(logging.CategoryGameState, "Cleaned up wanderer dissolve effect")
}

func (g *GameScene) setAvatarDissolveLevel(level float64) {
	// Update DissolveEffectState (handles both level and effect progress)
	g.avatarDissolve.UpdateProgress(level)
	logging.Debugf(logging.CategoryGameState, "Avatar dissolve level set to %.2f", level)
}

// initializeAvatarDissolveEffect creates a new dissolve effect for the avatar sprite
// Called when teleportation starts and avatar dissolves out
func (g *GameScene) initializeAvatarDissolveEffect() {
	// Get avatar sprite dimensions
	avatarSprite := g.spriteSheet.GetSprite(indexes.AvatarKeyIndex)
	if avatarSprite == nil {
		logging.Warnf(logging.CategoryGameState, "Failed to get avatar sprite for dissolve effect initialization")

		return
	}

	spriteWidth := avatarSprite.Bounds().Dx()
	spriteHeight := avatarSprite.Bounds().Dy()

	// Initialize using DissolveEffectState (encapsulates effect, buffer, and level)
	g.avatarDissolve.Initialize(
		spriteWidth, spriteHeight,
		visual.DissolveModeHide, // Hide mode: starts opaque (1.0) → transparent (0.0)
		rng.New(avatarDissolveSeed),
	)

	logging.Debugf(logging.CategoryGameState, "Initialized avatar dissolve effect (%dx%d)", spriteWidth, spriteHeight)
}

// cleanupAvatarDissolveEffect releases the avatar dissolve effect resources
// Called when teleportation ends or avatar rematerializes
func (g *GameScene) cleanupAvatarDissolveEffect() {
	g.avatarDissolve.Cleanup()
	logging.Debugf(logging.CategoryGameState, "Cleaned up avatar dissolve effect")
}

// Note: Death Scene Apparition Dissolve Effects removed - now uses generic tile dissolve system
// See: StartTileDissolveWithOptions(), UpdateTileDissolveProgress(), StopTileDissolve()
// The death sequence (gamescene_death_sequence.go) directly uses the generic system
// with TileDissolveOptions for GuardianLeft (sprite 94), GuardianRight (sprite 95), Wanderer (sprite 372)

func (g *GameScene) setMapFadeLevel(level float64) {
	g.mapFadeLevel = level
	logging.Debugf(logging.CategoryGameState, "Map fade level set to %.2f", level)
}

// =============================================================================
// Generic Tile Dissolve System - API
// =============================================================================

// StartTileDissolveWithOptions creates and starts a new tile dissolve animation with full control.
// Returns the dissolve ID for later reference (progress updates, cleanup).
//
// This is the primary API for complex dissolve effects like death scene apparitions.
//
// Usage in event queue:
//
//	var guardianID int
//	q.Then(func() {
//	    guardianID = g.StartTileDissolveWithOptions(TileDissolveOptions{
//	        Position:     references.Position{X: 2, Y: 6},
//	        SpriteIndex:  indexes.CodexAngelLeft,
//	        Mode:         visual.DissolveModeReveal,
//	        OutlineColor: sprites.WhiteOutline,
//	        PositionMode: PositionModeFixedGrid,
//	        RenderLayer:  RenderLayerOverFade,
//	    })
//	}).
//	Animate(1500, func(progress float32) {
//	    g.UpdateTileDissolveProgress(guardianID, float64(progress))
//	}).
//	Then(func() {
//	    g.StopTileDissolve(guardianID)
//	})
func (g *GameScene) StartTileDissolveWithOptions(opts TileDissolveOptions) int {
	return g.startTileDissolveWithOptions(opts, rng.New(tileDissolveSeed(opts)))
}

func tileDissolveSeed(opts TileDissolveOptions) uint64 {
	positionX := uint64(uint16(opts.Position.X))     //nolint:gosec // Coordinates retain their signed bit pattern in the seed.
	positionY := uint64(uint16(opts.Position.Y))     //nolint:gosec // Coordinates retain their signed bit pattern in the seed.
	spriteIndex := uint64(uint16(opts.SpriteIndex))  //nolint:gosec // Ultima V sprite indices fit in 16 bits.
	mode := uint64(uint8(opts.Mode))                 //nolint:gosec // Dissolve modes are a bounded enum.
	outline := uint64(uint8(opts.OutlineColor))      //nolint:gosec // Outline colors are a bounded enum.
	positionMode := uint64(uint8(opts.PositionMode)) //nolint:gosec // Position modes are a bounded enum.
	renderLayer := uint64(uint8(opts.RenderLayer))   //nolint:gosec // Render layers are a bounded enum.
	value := positionX | positionY<<16 | spriteIndex<<32 | mode<<48 |
		outline<<50 | positionMode<<54 | renderLayer<<56

	return presentationMix64(value)
}

func (g *GameScene) startTileDissolveWithOptions(
	opts TileDissolveOptions,
	rng *rng.Source,
) int {
	// Get sprite dimensions
	sprite := g.spriteSheet.GetSprite(opts.SpriteIndex)
	if sprite == nil {
		panic(fmt.Sprintf("Failed to get sprite %d for tile dissolve effect - sprite must exist", opts.SpriteIndex))
	}

	spriteWidth := sprite.Bounds().Dx()
	spriteHeight := sprite.Bounds().Dy()

	// Generate unique ID
	g.nextDissolveID++
	id := g.nextDissolveID

	// Create new dissolve state with full options
	state := TileDissolveState{
		ID:           id,
		Position:     opts.Position,
		SpriteIndex:  opts.SpriteIndex,
		OutlineColor: opts.OutlineColor,
		PositionMode: opts.PositionMode,
		RenderLayer:  opts.RenderLayer,
	}

	// Initialize the dissolve effect
	state.Effect.Initialize(
		spriteWidth, spriteHeight,
		opts.Mode,
		rng,
	)

	// Add to active dissolves
	g.activeTileDissolves = append(g.activeTileDissolves, state)

	modeStr := "Reveal"
	if opts.Mode == visual.DissolveModeHide {
		modeStr = "Hide"
	}

	posStr := "World"
	if opts.PositionMode == PositionModeFixedGrid {
		posStr = "FixedGrid"
	}

	layerStr := "Default"
	if opts.RenderLayer == RenderLayerOverFade {
		layerStr = "OverFade"
	}

	logging.Debugf(logging.CategoryGameState,
		"Started tile dissolve ID=%d sprite=%d at (%d,%d) mode=%s outline=%d posMode=%s layer=%s",
		id, opts.SpriteIndex, opts.Position.X, opts.Position.Y, modeStr, opts.OutlineColor, posStr, layerStr)

	return id
}

// UpdateTileDissolveProgress updates the progress of a dissolve animation by ID.
// Progress should be 0.0 to 1.0:
//   - For Reveal mode: 0.0 = fully transparent, 1.0 = fully opaque
//   - For Hide mode: 0.0 = fully opaque, 1.0 = fully transparent
func (g *GameScene) UpdateTileDissolveProgress(id int, progress float64) {
	for i := range g.activeTileDissolves {
		if g.activeTileDissolves[i].ID == id {
			g.activeTileDissolves[i].Effect.UpdateProgress(progress)

			return
		}
	}
	// ID not found - may have been stopped already, which is fine
}

// StopTileDissolve removes a dissolve animation by ID and cleans up its resources.
func (g *GameScene) StopTileDissolve(id int) {
	for i := range g.activeTileDissolves {
		if g.activeTileDissolves[i].ID == id {
			// Cleanup effect resources
			g.activeTileDissolves[i].Effect.Cleanup()

			// Remove from slice by swapping with last element and truncating
			lastIdx := len(g.activeTileDissolves) - 1
			if i != lastIdx {
				g.activeTileDissolves[i] = g.activeTileDissolves[lastIdx]
			}

			g.activeTileDissolves = g.activeTileDissolves[:lastIdx]

			logging.Debugf(logging.CategoryGameState, "Stopped tile dissolve ID=%d", id)

			return
		}
	}
	// ID not found - may have been stopped already, which is fine
}

// StopAllTileDissolves clears all active tile dissolve animations.
// Useful for guaranteed cleanup in event queue Finally() callbacks.
func (g *GameScene) StopAllTileDissolves() {
	for i := range g.activeTileDissolves {
		g.activeTileDissolves[i].Effect.Cleanup()
	}

	count := len(g.activeTileDissolves)
	g.activeTileDissolves = g.activeTileDissolves[:0] // Clear slice but keep capacity

	if count > 0 {
		logging.Debugf(logging.CategoryGameState, "Stopped all tile dissolves (count=%d)", count)
	}
}
