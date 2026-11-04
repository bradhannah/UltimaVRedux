package map_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/helpers"
)

const (
	totalLayers   = 6
	overflowTiles = references.Coordinate(10)
	// Coordinate offset to handle negative positions (overflow extends from -10 to +10)
	coordinateOffset = overflowTiles + 1
)

// LayerType selects one of the stacked tile layers of a LayeredMap.
type LayerType int

const (
	// MapLayer is the base terrain layer.
	MapLayer LayerType = iota
	// MapOverrideLayer holds runtime changes to terrain, such as opened doors.
	MapOverrideLayer
	// MapUnitLayer holds NPCs, monsters and other units drawn above the terrain.
	MapUnitLayer
)

// Layer represents a 2D array of sprite indices for performance optimization
// Using triple slice [layer][y][x] instead of maps for 5-8x speedup
type Layer [][]indexes.SpriteIndex

// GameDimensions describes the visible game screen and the map extents behind it.
type GameDimensions interface {
	GetTilesVisibleOnScreen() (int, int)
	GetTopLeftExtent() references.Position
	GetBottomRightExtent() references.Position
	GetBottomRightWithoutOverflow() references.Position
	IsWrappedMap() bool
}

// LayeredMap represents a map broken into separate layers. These layers are essentially "floors"
type LayeredMap struct {
	layers           [totalLayers]Layer
	exposure         MapExposure
	daylightOpenings []daylightOpening
	doorOccupied     func(references.Position) bool

	visibleFlags            VisibilityCoords
	visiblePositions        []references.Position // Raw positions for this recalculation, including seam halos.
	testForVisibilityMap    VisibilityCoords
	primaryDistanceMaskMap  DistanceMap
	lightSourcesDistanceMap DistanceMap

	tileRefs *references.Tiles

	xVisibleTiles, yVisibleTiles     int
	TopLeft, BottomRight             references.Position
	XMaxTilesPerMap, YMaxTilesPerMap references.Coordinate

	bWrappingMap bool

	// Light source calculation cache for performance optimization
	cachedLightSources      LightSources
	cachedAvatarPos         references.Position
	cachedMoongatePositions []references.Position
	lightSourceCacheValid   bool
}

func newLayeredMap(xMax references.Coordinate,
	yMax references.Coordinate,
	tileRefs *references.Tiles,
	xVisibleTiles int,
	yVisibleTiles int,
	bWrappingMap bool,
) *LayeredMap {
	layeredMap := LayeredMap{ //nolint:exhaustruct_v5
		xVisibleTiles:   xVisibleTiles,
		yVisibleTiles:   yVisibleTiles,
		XMaxTilesPerMap: xMax,
		YMaxTilesPerMap: yMax,
		bWrappingMap:    bWrappingMap,
		tileRefs:        tileRefs,
	}

	// Calculate array dimensions including overflow
	arrayHeight := int(yMax + 2*overflowTiles + 1)
	arrayWidth := int(xMax + 2*overflowTiles + 1)

	// Initialize visibility maps (keep as maps for now)
	layeredMap.visibleFlags = make(map[references.Coordinate]map[references.Coordinate]bool)
	layeredMap.testForVisibilityMap = make(map[references.Coordinate]map[references.Coordinate]bool)
	for yRow := -overflowTiles - 1; yRow < yMax+overflowTiles; yRow++ {
		layeredMap.visibleFlags[yRow] = make(map[references.Coordinate]bool)
		layeredMap.testForVisibilityMap[yRow] = make(map[references.Coordinate]bool)
	}

	// Initialize layers as 2D arrays for performance
	for mapLayer := range layeredMap.layers {
		layeredMap.layers[mapLayer] = make([][]indexes.SpriteIndex, arrayHeight)
		for y := range arrayHeight {
			layeredMap.layers[mapLayer][y] = make([]indexes.SpriteIndex, arrayWidth)
		}
	}

	return &layeredMap
}

// ExposureAt returns immutable source-floor exposure, independently of runtime tiles and lighting.
func (l *LayeredMap) ExposureAt(pos references.Position) ExposureKind {
	return l.exposure.At(int(pos.X), int(pos.Y))
}

func (l *LayeredMap) getAllLightSourcesInRange(centerPosition references.Position) LightSources {
	lightSources := make(LightSources, 0)

	for dX := -l.XMaxTilesPerMap; dX < l.XMaxTilesPerMap; dX++ {
		for dY := -l.YMaxTilesPerMap; dY < l.YMaxTilesPerMap; dY++ {
			pos := references.Position{X: centerPosition.X + dX, Y: centerPosition.Y + dY}

			tile := l.GetTileTopMapOnlyTile(&pos)
			if tile == nil {
				continue
			}

			if tile.GetLightEmission() > 0 {
				lightSources = append(lightSources, LightSource{
					Tile: tile,
					Pos:  pos,
				})
			}
		}
	}

	return lightSources
}

// addMoongateLightSources appends moongate light sources to the provided slice.
// Moongates emit mystical blue light with MoongateLightDistance radius.
// Uses MoongateMarkerIndex for tint color lookup in LightSourcePropertiesMap.
func (l *LayeredMap) addMoongateLightSources(lightSources *LightSources, moongatePositions []references.Position) {
	for _, moongatePos := range moongatePositions {
		baseTile := l.GetTileTopMapOnlyTile(&moongatePos)
		if baseTile == nil {
			continue
		}

		// Create a modified tile with moongate light emission
		moongateLight := *baseTile                          // Copy the base tile
		moongateLight.LightEmission = MoongateLightDistance // Mystical light radius
		moongateLight.Index = MoongateMarkerIndex           // Marker for blue tinting lookup

		*lightSources = append(*lightSources, LightSource{
			Tile: &moongateLight,
			Pos:  moongatePos,
		})
	}
}

// InvalidateLightSourceCache invalidates the light source cache, forcing recalculation on next access
func (l *LayeredMap) InvalidateLightSourceCache() {
	l.lightSourceCacheValid = false
}

func (l *LayeredMap) floodFillIfInside(pos *references.Position, bForce bool) {
	if l.bWrappingMap {
		pos = pos.GetWrapped(l.XMaxTilesPerMap, l.YMaxTilesPerMap)
	}

	if !bForce && !l.testForVisibilityMap[pos.X][pos.Y] {
		// don't reprocess and existing tile if it has been set
		return
	}

	l.SetVisible(true, pos)
	l.testForVisibilityMap[pos.X][pos.Y] = false

	tile := l.GetLiveMapOnlyTile(pos)

	// basically - if they are next to a window, then they can see through - otherwise we treat
	// the window as opaque, and nothing passes through
	if tile != nil {
		seeThroughWindow := tile.IsWindow() && bForce
		if !seeThroughWindow && (tile.BlocksLight || tile.IsWindow()) {
			return
		}
	}

	l.setTilesAroundPositionVisible(pos)

	l.floodFillIfInside(pos.GetPositionToLeft(), false)
	l.floodFillIfInside(pos.GetPositionToRight(), false)
	l.floodFillIfInside(pos.GetPositionDown(), false)
	l.floodFillIfInside(pos.GetPositionUp(), false)
}

func (l *LayeredMap) setTilesAroundPositionVisible(pos *references.Position) {
	l.SetVisible(true, pos.GetPositionToLeft())
	l.SetVisible(true, pos.GetPositionToRight())
	l.SetVisible(true, pos.GetPositionDown())
	l.SetVisible(true, pos.GetPositionUp())

	// do the diagonal on the initial call
	l.SetVisible(true, pos.GetPositionUp().GetPositionToLeft())
	l.SetVisible(true, pos.GetPositionUp().GetPositionToRight())
	l.SetVisible(true, pos.GetPositionDown().GetPositionToLeft())
	l.SetVisible(true, pos.GetPositionDown().GetPositionToRight())
}

// SetVisible sets the visibility of a tile to true or false.
func (l *LayeredMap) SetVisible(bVisible bool, pos *references.Position) {
	if bVisible && !l.visibleFlags[pos.X][pos.Y] {
		l.visiblePositions = append(l.visiblePositions, *pos)
	}
	l.visibleFlags[pos.X][pos.Y] = bVisible
}

// IsPositionVisible returns true if the position is visible based on the time of day.
func (l *LayeredMap) IsPositionVisible(pos *references.Position, timeOfDay datetime.UltimaDate, bIsBasement bool) bool {
	// note: some of this may feel like overkill - but it is setting up for an eventual alpha- or gradient-
	// based lighting that degrades as it gets further away

	// first check to see if the line of sight allows it to be seen
	if !l.visibleFlags[pos.X][pos.Y] {
		return false
	}

	// next focus on checking the primary lighting
	xUp := l.xVisibleTiles/screenCentreDivisor + 1

	var nToShow int

	if bIsBasement {
		nToShow = 1 // helpers.RoundUp(float32(xUp) * timeOfDay.GetVisibilityFactorWithoutTorch(baselineVisibilityFactor))
	} else {
		nToShow = helpers.RoundUp(float32(xUp) * timeOfDay.GetVisibilityFactorWithoutTorch(baselineVisibilityFactor))
	}

	// nToShow is small because of the Avatar, so this check alone is not enough;
	// the second, light-source distance map is the more factual of the two.
	bShow := l.primaryDistanceMaskMap[*pos] <= nToShow

	if bShow {
		return true
	}

	if _, ok := l.lightSourcesDistanceMap[*pos]; !ok {
		return false
	}

	return true
}

// GetTopTile returns the highest non-empty tile across all layers, or nil for a nil position.
func (l *LayeredMap) GetTopTile(position *references.Position) *references.Tile {
	if position == nil {
		// Gracefully handle nil position to prevent crashes in tests
		return nil
	}
	for i := MapUnitLayer; i >= MapLayer; i-- {
		tile := l.GetTileByLayer(i, position)
		if tile.Index <= 0 {
			continue
		}

		return tile
	}

	return nil
}

// GetTileTopMapOnlyTile returns the highest non-empty terrain tile, ignoring the unit layer.
func (l *LayeredMap) GetTileTopMapOnlyTile(position *references.Position) *references.Tile {
	for i := MapOverrideLayer; i >= MapLayer; i-- {
		tile := l.GetTileByLayer(i, position)
		if tile.Index <= 0 {
			continue
		}

		return tile
	}

	return nil
}

// GetLiveMapOnlyTile resolves occupancy-held doors for visibility. Persistent
// terrain/lock consumers continue to use GetTileTopMapOnlyTile.
func (l *LayeredMap) GetLiveMapOnlyTile(position *references.Position) *references.Tile {
	return l.resolveOccupiedDoor(position, l.GetTileTopMapOnlyTile(position))
}

func (l *LayeredMap) resolveOccupiedDoor(position *references.Position, terrain *references.Tile) *references.Tile {
	if l.isOccupiedDoor(position, terrain) {
		// REDUX DEVIATION: Original object display conceals terrain under an actor (ref:TILE-0008)
		// without opening it. Redux makes occupied doorways live floor for LOS,
		// daylight and presentation, retaining the underlying lock unchanged.
		return l.tileRefs.GetTile(indexes.BrickFloor)
	}

	return terrain
}

func (l *LayeredMap) isOccupiedDoor(position *references.Position, terrain *references.Tile) bool {
	return terrain != nil && terrain.Index.IsDoor() && l.doorOccupied != nil && l.doorOccupied(*position)
}

// SetTileByLayer writes a sprite index into one layer; writes outside the map are ignored.
func (l *LayeredMap) SetTileByLayer(layer LayerType, position *references.Position, nIndex indexes.SpriteIndex) {
	// Convert world coordinates to array indices with overflow offset
	arrayX := int(position.X + coordinateOffset)
	arrayY := int(position.Y + coordinateOffset)

	// Bounds checking - ignore writes outside array bounds
	if arrayY < 0 || arrayY >= len(l.layers[layer]) ||
		arrayX < 0 || arrayX >= len(l.layers[layer][arrayY]) {
		return
	}

	l.layers[layer][arrayY][arrayX] = nIndex
}

// UnSetTileByLayer clears one layer at position.
func (l *LayeredMap) UnSetTileByLayer(layer LayerType, position *references.Position) {
	l.SetTileByLayer(layer, position, indexes.NoSprites)
}

// GetTileByLayer returns the tile at position in one layer, or the empty tile outside the map.
func (l *LayeredMap) GetTileByLayer(layer LayerType, position *references.Position) *references.Tile {
	// Convert world coordinates to array indices with overflow offset
	arrayX := int(position.X + coordinateOffset)
	arrayY := int(position.Y + coordinateOffset)

	// Bounds checking - return nil tile if outside array bounds
	if arrayY < 0 || arrayY >= len(l.layers[layer]) ||
		arrayX < 0 || arrayX >= len(l.layers[layer][arrayY]) {
		return l.tileRefs.GetTile(indexes.NoSprites)
	}

	return l.tileRefs.GetTile(l.layers[layer][arrayY][arrayX])
}

// getSpriteIndexByLayer returns the raw sprite index from a layer without tileRefs lookup.
// Returns indexes.NoSprites if position is out of bounds.
// Use this for performance-critical scanning operations like lighthouse detection.
func (l *LayeredMap) getSpriteIndexByLayer(layer LayerType, position *references.Position) indexes.SpriteIndex {
	arrayX := int(position.X + coordinateOffset)
	arrayY := int(position.Y + coordinateOffset)

	if arrayY < 0 || arrayY >= len(l.layers[layer]) ||
		arrayX < 0 || arrayX >= len(l.layers[layer][arrayY]) {
		return indexes.NoSprites
	}

	return l.layers[layer][arrayY][arrayX]
}

// SpriteIndexByLayer returns the raw persistent layer value at position.
func (l *LayeredMap) SpriteIndexByLayer(layer LayerType, position references.Position) indexes.SpriteIndex {
	return l.getSpriteIndexByLayer(layer, &position)
}

// SwapTiles exchanges the top terrain tiles of two positions on the base layer.
func (l *LayeredMap) SwapTiles(pos1, pos2 *references.Position) {
	tile1 := l.GetTileTopMapOnlyTile(pos1)
	tile2 := l.GetTileTopMapOnlyTile(pos2)
	l.SetTileByLayer(MapLayer, pos1, tile2.Index)
	l.SetTileByLayer(MapLayer, pos2, tile1.Index)
}

// ClearMapUnitTiles empties the whole unit layer.
func (l *LayeredMap) ClearMapUnitTiles() {
	// Clear the entire MapUnitLayer array
	for y := range l.layers[MapUnitLayer] {
		for x := range l.layers[MapUnitLayer][y] {
			l.layers[MapUnitLayer][y][x] = 0
		}
	}
}

// GetTilesVisibleOnScreen returns the visible screen size in tiles (x, y).
func (l *LayeredMap) GetTilesVisibleOnScreen() (int, int) {
	return l.xVisibleTiles, l.yVisibleTiles
}

// GetTopLeftExtent returns the top-left corner of the loaded area, including overflow tiles.
func (l *LayeredMap) GetTopLeftExtent() references.Position {
	return l.TopLeft
}

// GetBottomRightExtent returns the bottom-right corner of the loaded area, including overflow tiles.
func (l *LayeredMap) GetBottomRightExtent() references.Position {
	return l.BottomRight
}

// IsWrappedMap returns true for maps whose edges wrap, such as the overworld.
func (l *LayeredMap) IsWrappedMap() bool {
	return l.bWrappingMap
}

// GetBottomRightWithoutOverflow returns the map size in tiles, without overflow tiles.
func (l *LayeredMap) GetBottomRightWithoutOverflow() references.Position {
	return references.Position{X: l.XMaxTilesPerMap, Y: l.YMaxTilesPerMap}
}

// GetVisibilityFlags returns the visibility flags map for overlay rendering
// This is used by the overlay system to check which tiles are visible
func (l *LayeredMap) GetVisibilityFlags() VisibilityCoords {
	return l.visibleFlags
}

// GetPrimaryDistanceMap returns the primary distance mask map
// Used by overlay system for lighting calculations
func (l *LayeredMap) GetPrimaryDistanceMap() DistanceMap {
	return l.primaryDistanceMaskMap
}

// GetLightSourcesDistanceMap returns the light sources distance map
// Used by overlay system for secondary light source rendering
func (l *LayeredMap) GetLightSourcesDistanceMap() DistanceMap {
	return l.lightSourcesDistanceMap
}

// GetLastComputedLightSources returns the light sources from the most recent visibility calculation.
// Used by LightSourceTintingState to build the lit tiles map for colored light tinting.
// The custom-only path excludes invisible static emitters; dynamic inputs retain
// their supplied order/duplicates and are visibility-filtered by the consumers.
func (l *LayeredMap) GetLastComputedLightSources() LightSources {
	return l.cachedLightSources
}

// GetVisibleFlags returns the current visibility flags map.
// Used by LightSourceTintingState to filter light sources by visibility,
// ensuring that light from behind walls doesn't bleed through.
func (l *LayeredMap) GetVisibleFlags() VisibilityCoords {
	return l.visibleFlags
}

// QuestItemLightInfo holds information about a quest item that emits light
type QuestItemLightInfo struct {
	Position    references.Position
	MarkerIndex indexes.SpriteIndex // For tint color lookup in LightSourcePropertiesMap
	LightRadius int
}

// NPCLightInfo holds information about a magical NPC that emits light.
// Used for creatures like wisps, seahorses, daemons, and ghosts.
type NPCLightInfo struct {
	Position    references.Position
	MarkerIndex indexes.SpriteIndex // For tint color lookup in LightSourcePropertiesMap
	LightRadius int
}

// RecalculateVisibleTilesWithCustomLights recalculates visible tiles including moongate,
// quest item, and NPC light sources.
func (l *LayeredMap) RecalculateVisibleTilesWithCustomLights(
	avatarPos references.Position,
	lighting *Lighting,
	moongatePositions []references.Position,
	questItemLights []QuestItemLightInfo,
	npcLights []NPCLightInfo,
	playerLightType PlayerLightType,
) {
	l.TopLeft = references.Position{
		X: avatarPos.X - overflowTiles,
		Y: avatarPos.Y - overflowTiles,
	}
	l.BottomRight = references.Position{
		X: avatarPos.X + overflowTiles,
		Y: avatarPos.Y + overflowTiles,
	}

	// Reset and rectangle marking could be done in one pass.
	l.testForVisibilityMap.ResetVisibilityCoords(false)
	l.testForVisibilityMap.setVisibilityCoordsRectangle(
		&l.TopLeft,
		&l.BottomRight,
		l.XMaxTilesPerMap,
		l.YMaxTilesPerMap,
		l.bWrappingMap,
	)
	l.visibleFlags.ResetVisibilityCoords(false)
	l.visiblePositions = l.visiblePositions[:0]

	l.SetVisible(true, &avatarPos)

	l.setTilesAroundPositionVisible(&avatarPos)

	l.floodFillIfInside(&avatarPos, true)
	l.floodFillIfInside(avatarPos.GetPositionToLeft(), true)
	l.floodFillIfInside(avatarPos.GetPositionToRight(), true)
	l.floodFillIfInside(avatarPos.GetPositionDown(), true)
	l.floodFillIfInside(avatarPos.GetPositionUp(), true)

	// get a full game screen lighting map
	l.primaryDistanceMaskMap = lighting.BuildGameScreenDistanceMap(avatarPos)

	// get lighting sources including moongates, quest items, and NPCs and overlay them of a light map
	lightSources := l.getAllLightSourcesWithCustomLights(avatarPos, moongatePositions, questItemLights, npcLights)
	l.cachedLightSources = lightSources // Store for LightSourceTintingState access
	l.lightSourcesDistanceMap = lighting.BuildLightSourceDistanceMap(lightSources,
		l.visibleFlags,
		playerLightType,
		avatarPos,
	)
}

// getAllLightSourcesWithCustomLights gets all light sources including custom light sources
// (moongates, quest items, and NPCs).
func (l *LayeredMap) getAllLightSourcesWithCustomLights(
	centerPosition references.Position,
	moongatePositions []references.Position,
	questItemLights []QuestItemLightInfo,
	npcLights []NPCLightInfo,
) LightSources {
	// Both distance and tint consumers reject invisible static emitters. Collect
	// only current visible positions, preserving the exhaustive scan's raw order.
	// A primed older cache shares cachedLightSources; retain its full-list behavior.
	var lightSources LightSources
	if l.lightSourceCacheValid {
		lightSources = l.getAllLightSourcesInRange(centerPosition)
	} else {
		lightSources = l.getVisibleStaticLightSources(centerPosition)
	}

	// Add moongate light sources
	l.addMoongateLightSources(&lightSources, moongatePositions)

	// Add quest items as light sources
	l.addQuestItemLightSources(&lightSources, questItemLights)

	// Add magical NPC light sources
	l.addNPCLightSources(&lightSources, npcLights)

	return lightSources
}

// addNPCLightSources appends magical NPC light sources to the provided slice.
// NPCs like wisps, seahorses, daemons, and ghosts emit colored light.
func (l *LayeredMap) addNPCLightSources(lightSources *LightSources, npcLights []NPCLightInfo) {
	for _, npcLight := range npcLights {
		baseTile := l.GetTileTopMapOnlyTile(&npcLight.Position)
		if baseTile == nil {
			continue
		}

		// Create a modified tile with NPC light emission
		npcLightTile := *baseTile // Copy the base tile
		npcLightTile.LightEmission = npcLight.LightRadius
		npcLightTile.Index = npcLight.MarkerIndex // Use marker for tint color lookup

		*lightSources = append(*lightSources, LightSource{
			Tile: &npcLightTile,
			Pos:  npcLight.Position,
		})
	}
}

// addQuestItemLightSources appends quest item light sources to the provided slice.
// Quest items (shards, artifacts) emit light with their configured radius and tint color.
func (l *LayeredMap) addQuestItemLightSources(lightSources *LightSources, questItemLights []QuestItemLightInfo) {
	for _, questLight := range questItemLights {
		baseTile := l.GetTileTopMapOnlyTile(&questLight.Position)
		if baseTile == nil {
			continue
		}

		// Create a modified tile with quest item light emission
		questItemTile := *baseTile                           // Copy the base tile
		questItemTile.LightEmission = questLight.LightRadius // Use quest item specific radius
		questItemTile.Index = questLight.MarkerIndex         // Use marker for tint color lookup

		*lightSources = append(*lightSources, LightSource{
			Tile: &questItemTile,
			Pos:  questLight.Position,
		})
	}
}

// FindLighthousePositions scans the map for LighthouseLight tiles (beacon, sprite 0x2a).
// Returns up to MaxLighthousesPerMap positions (per original game limit).
// This is used when entering a small map to detect lighthouse locations.
func (l *LayeredMap) FindLighthousePositions() []references.Position {
	positions := make([]references.Position, 0, MaxLighthousesPerMap)

	for tileY := references.Coordinate(0); tileY < l.YMaxTilesPerMap; tileY++ {
		for tileX := references.Coordinate(0); tileX < l.XMaxTilesPerMap; tileX++ {
			pos := references.Position{X: tileX, Y: tileY}
			spriteIndex := l.getSpriteIndexByLayer(MapLayer, &pos)

			if spriteIndex == indexes.LighthouseLight {
				logging.Debugf(logging.CategoryLighthouse,
					"FindLighthousePositions: Found beacon at (%d, %d), tile index %d",
					tileX, tileY, spriteIndex)
				positions = append(positions, pos)
				if len(positions) >= MaxLighthousesPerMap {
					return positions
				}
			}
		}
	}

	logging.Debugf(logging.CategoryLighthouse,
		"FindLighthousePositions: Scanned %dx%d map, found %d lighthouse(s)",
		l.XMaxTilesPerMap, l.YMaxTilesPerMap, len(positions))

	return positions
}

// FindLighthousePositionsInViewport scans a viewport around the player for lighthouse building tiles.
// Used for large maps (overworld) where lighthouse buildings need to be detected in visible area.
// Scans the loaded 32x32 region (four 16x16 quadrants) for lighthouse terrain.
//
// Parameters:
//   - playerPos: Center of viewport (player position)
//   - viewRadius: Scan radius in tiles (16 for 32x32 area matching original)
//   - tileToFind: Sprite index to search for (indexes.Lighthouse for overworld)
//
// Returns up to MaxLighthousesPerMap positions.
func (l *LayeredMap) FindLighthousePositionsInViewport(
	playerPos references.Position,
	viewRadius int,
	tileToFind indexes.SpriteIndex,
) []references.Position {
	positions := make([]references.Position, 0, MaxLighthousesPerMap)

	for deltaY := -viewRadius; deltaY <= viewRadius; deltaY++ {
		for deltaX := -viewRadius; deltaX <= viewRadius; deltaX++ {
			checkPos := references.Position{
				X: playerPos.X + tileOffset(deltaX),
				Y: playerPos.Y + tileOffset(deltaY),
			}

			// Handle coordinate wrapping for large maps
			if l.bWrappingMap {
				checkPos = *checkPos.GetWrapped(l.XMaxTilesPerMap, l.YMaxTilesPerMap)
			}

			// Get raw sprite index from layer array (avoids tileRefs dependency)
			spriteIndex := l.getSpriteIndexByLayer(MapLayer, &checkPos)
			if spriteIndex == tileToFind {
				positions = append(positions, checkPos)
				if len(positions) >= MaxLighthousesPerMap {
					return positions
				}
			}
		}
	}

	return positions
}
