package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// updateWindAndEffects handles wind updates and visual effects using real time
func (g *GameScene) updateWindAndEffects() {
	currentMs := g.getElapsedMs()
	g.gameState.ElapsedMs = currentMs
	g.updateActorAnimationClocks()
	g.gameState.ProcessWindThroughElapsed(currentMs)
	g.gameState.UpdateMoongateTransition()

	// Update lighthouse rotation at same ~18Hz rate (55ms intervals)
	// Lighthouse beams rotate through 16 angles, 3 segments lit at a time
	// Disabled during full daylight (sight >= 50 in original)
	// Visibility changes are automatically applied during next draw cycle
	// Note: Lighthouse tinting is handled in UpdateLightSourceTinting() which runs every frame
	if g.gameState.LighthouseState != nil && g.gameState.LighthouseState.HasLighthouses() {
		isFullDaylight := g.gameState.DateTime.IsFullDaylight()
		g.gameState.LighthouseState.Update(currentMs, isFullDaylight)
	}

	// Update event queue manager for earthquake and other event sequences
	if g.gameState.EventQueueManager == nil {
		panic("GameScene.updateWindAndEffects: EventQueueManager is nil - critical system component missing")
	}

	g.gameState.EventQueueManager.Update()

	g.gameState.EventQueueManager.UpdateFeedback()
	g.moongateVisualSystem.Update()
	g.updateAmbientSounds()

	// Publish scene-owned clocks and lifecycle transitions before the stack's
	// single non-blocking widget update advances their visual state.
	g.updateTransientActionFeedback()
	g.updateNonBlockingDialogs()
	g.updateDungeonSkeletonCrumble()
	g.updateDungeonPitReveals()
	g.updateDungeonFieldDispels()
	g.updateDungeonInjury()

	// Update timed overlay auto-dismissal (checked independently of event queue)
	g.updateTimedOverlay()
	g.updateCombatReportAttention()

	// Intentionally left minimal; hook up AI, schedules, etc., later.

	// Update visual effects
	g.updateVisualEffects()

	// Update wind display overlay (animation if needed)
	g.windDisplay.Update()

	// MessageDialog updates are now handled via dialog stack (no persistent messageDialog)
	// All dialogs in the stack are automatically updated via drawDialogs()
}

func (g *GameScene) updateNonBlockingDialogs() {
	g.dialogStack.UpdateNonBlockingDialogs()
}

// updateVisualEffects updates and removes expired visual effects
func (g *GameScene) updateVisualEffects() {
	// Noncombat cosmetics retain a frame-delta clock. Combat effects use their
	// own admission/arrival timestamps, including work queued earlier this frame.
	now := g.sceneNow()
	deltaTime := now.Sub(g.lastEffectsUpdate).Seconds()
	g.lastEffectsUpdate = now
	g.updateCombatImpactEffects(deltaTime)
	g.updateCombatDivisionEffects(deltaTime)
	g.updateCombatSceptreEffects()
	g.updateCombatVanishEffects(deltaTime)
	g.updateCombatSignatureEffects()
	g.updateCombatAllegiance()

	// Update existing effects in-place and compact the slice (no allocation)
	writeIndex := 0
	for i := range g.visualEffects {
		if g.visualEffects[i].combatClock {
			g.visualEffects[i].Elapsed = float64(max(g.gameState.ElapsedMs-g.visualEffects[i].combatStartedMs, 0)) / 1000
		} else {
			g.visualEffects[i].Elapsed += deltaTime
		}
		if g.visualEffects[i].Elapsed < g.visualEffects[i].Duration {
			// Keep this effect by moving it to the write position
			if writeIndex != i {
				g.visualEffects[writeIndex] = g.visualEffects[i]
			}
			writeIndex++
		}
	}
	// Truncate the slice to remove expired effects
	g.visualEffects = g.visualEffects[:writeIndex]
}

// updateAmbientSounds updates proximity-based ambient sounds based on player position and nearby tiles
func (g *GameScene) updateAmbientSounds() {
	if g.audioManager == nil {
		return
	}

	// Skip ambient sound updates when hidden (e.g., during camping where campfire sound is managed separately)
	if g.hideAmbientSounds {
		return
	}

	// Get current player position
	playerPos := g.gameState.MapState.PlayerLocation.Position

	// Gather nearby tiles within ambient sound range (approx 10x10 area around player)
	nearbyTiles := g.getNearbyTilesForAmbientSound(playerPos, g.audioManager.AmbientScanRadius())

	// Get dynamic ambient sources from various systems
	moongateSources := g.getMoongateAmbientSources()
	bardSources := g.getBardAmbientSources()

	// Merge all dynamic sources
	dynamicSources := make([]audio.AmbientSoundSource, 0, len(moongateSources)+len(bardSources))
	dynamicSources = append(dynamicSources, moongateSources...)
	dynamicSources = append(dynamicSources, bardSources...)
	if source, exists := g.audioManager.DungeonAmbientSource(g.dungeonAudioLocation(), playerPos); exists {
		dynamicSources = append(dynamicSources, source)
	}
	vessel, _ := g.navalAudioSituation()
	if source, exists := g.audioManager.NavalAmbientSource(vessel, playerPos); exists {
		dynamicSources = append(dynamicSources, source)
	}
	if source, exists := g.audioManager.LighthouseAmbientSource(g.gameState.MapState.PlayerLocation.Location, playerPos); exists {
		dynamicSources = append(dynamicSources, source)
	}

	// Update ambient sounds through audio manager with dynamic sources
	g.projectAmbientSources(playerPos, dynamicSources)
	g.audioManager.UpdateAmbientSoundsWithDynamicSources(playerPos, nearbyTiles, dynamicSources)
}

// dungeonAudioLocation retains the dungeon owner through tactical room/encounter
// presentation, while the spatial sources still come from the active map.
func (g *GameScene) dungeonAudioLocation() references.Location {
	if combat := g.gameState.CombatScene; combat != nil {
		return combat.PreCombatPlayerLocation.Location
	}

	return g.gameState.MapState.PlayerLocation.Location
}

// getMoongateAmbientSources extracts ambient sound sources from active moongates
func (g *GameScene) getMoongateAmbientSources() []audio.AmbientSoundSource {
	if g.moongateVisualSystem == nil {
		panic("GameScene.getMoongateAmbientSources: moongateVisualSystem is nil - critical system component missing")
	}

	activeRenderers := g.moongateVisualSystem.GetActiveRenderers()
	sources := make([]audio.AmbientSoundSource, 0, len(activeRenderers))
	for _, renderer := range activeRenderers {
		sources = append(sources, g.audioManager.EmitterAmbientSource("moongate", renderer.GetPosition(),
			moongateVolumeForStage(g.gameState.MoongateStage())))
	}

	return sources
}

func moongateVolumeForStage(stage game_state.MoongateStage) float64 {
	return float64(stage) / float64(game_state.MoongateStageFull)
}

// getBardAmbientSources extracts ambient sound sources from bards playing music
// Play music near a visible banjo-playing NPC.
//
//nolint:godox // The user intentionally deferred the tavern recording.
func (g *GameScene) getBardAmbientSources() []audio.AmbientSoundSource {
	if g.gameState.IsInCombat() {
		return nil // World NPCs are not sound sources on the tactical combat map.
	}
	// Get positions of all bards who are seated and playing music
	bardPositions := g.gameState.GetSeatedBardPlayingPositions()
	if len(bardPositions) == 0 {
		return nil
	}

	// TODO(audio): Supply tavern_lute_01; existing seated-bard proximity/fade lifecycle owns its loop.
	sources := make([]audio.AmbientSoundSource, 0, len(bardPositions))
	for _, pos := range bardPositions {
		sources = append(sources, g.audioManager.EmitterAmbientSource("bard", pos, 1)) // Full volume when playing
	}

	return sources
}

// getNearbyTilesForAmbientSound gathers tiles around the player for ambient sound processing
func (g *GameScene) getNearbyTilesForAmbientSound(playerPos references.Position, radius int) []audio.TileWithPosition {
	clear(g.ambientTiles)
	nearbyTiles := g.ambientTiles[:0]

	// Get current layered map
	currentMap := g.gameState.GetLayeredMapByCurrentLocation()
	if currentMap == nil {
		g.ambientTiles = nearbyTiles

		return nearbyTiles
	}

	// Visit each canonical tile at most once, even when a dungeon's authored
	// hearing radius exceeds its grid. Keep the nearest image for attenuation.
	startX, endX, startY, endY := -radius, radius, -radius, radius
	if currentMap.IsWrappedMap() {
		startX, endX = max(startX, -int(currentMap.XMaxTilesPerMap)/2), min(endX, (int(currentMap.XMaxTilesPerMap)-1)/2)
		startY, endY = max(startY, -int(currentMap.YMaxTilesPerMap)/2), min(endY, (int(currentMap.YMaxTilesPerMap)-1)/2)
	}
	for dx := startX; dx <= endX; dx++ {
		for dy := startY; dy <= endY; dy++ {
			//nolint:gosec // Listener coordinates and validated audio radius (<=32) fit Coordinate.
			pos := references.Position{X: playerPos.X + references.Coordinate(dx), Y: playerPos.Y + references.Coordinate(dy)}
			if currentMap.IsWrappedMap() {
				pos.X = canonicalAudioCoordinate(pos.X, currentMap.XMaxTilesPerMap)
				pos.Y = canonicalAudioCoordinate(pos.Y, currentMap.YMaxTilesPerMap)
			} else if pos.X < 0 || pos.Y < 0 || pos.X >= currentMap.XMaxTilesPerMap || pos.Y >= currentMap.YMaxTilesPerMap {
				continue
			}

			// Get the top tile at this position
			tile := currentMap.GetTileTopMapOnlyTile(&pos)
			if g.gameState.MapState.PlayerLocation.Location.GetMapType() == references.SmallMapType {
				tile = currentMap.GetSmallMapTerrainTile(&pos, g.gameState.DateTime.Hour)
			}
			if tile != nil {
				projected := pos
				if currentMap.IsWrappedMap() {
					projected.X = projectAudioCoordinate(playerPos.X, pos.X, currentMap.XMaxTilesPerMap)
					projected.Y = projectAudioCoordinate(playerPos.Y, pos.Y, currentMap.YMaxTilesPerMap)
				}
				nearbyTiles = append(nearbyTiles, audio.TileWithPosition{
					Tile:     tile,
					Position: projected,
				})
			}
		}
	}

	g.ambientTiles = nearbyTiles

	return nearbyTiles
}
