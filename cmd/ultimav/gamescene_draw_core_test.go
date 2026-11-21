package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestCinematicMapLayoutFitsCenteredElevenTileRoomInRightPane(t *testing.T) {
	const cinematicTiles = 11
	for _, size := range []struct{ width, height int }{{320, 200}, {1280, 720}} {
		panePlacement := cinematicMapPanePlacement()
		transcriptPlacement := cinematicTranscriptPlacement()
		layout := cinematicMapLayoutForScreen(size.width, size.height)
		require.Equal(t, layout.viewport.Dx(), layout.viewport.Dy())
		require.GreaterOrEqual(t, layout.viewport.Min.X, int(math.Round(float64(size.width)*panePlacement.StartPercentX)))
		require.LessOrEqual(t, layout.viewport.Max.X, int(math.Round(float64(size.width)*panePlacement.EndPercentX)))
		require.Equal(t, int(math.Round(float64(size.width)*panePlacement.StartPercentX)), layout.viewport.Min.X)
		require.Equal(t, int(math.Round(float64(size.height)*transcriptPlacement.StartPercentY)), layout.viewport.Min.Y)
		require.Equal(t, int(math.Round(float64(size.height)*transcriptPlacement.EndPercentY)), layout.viewport.Max.Y)
		transcriptCenter := float64(size.width) *
			(transcriptPlacement.StartPercentX + transcriptPlacement.EndPercentX) / 2
		require.InDelta(t, float64(layout.viewport.Min.X)/2, transcriptCenter, 0.51)

		fullLeft := layout.placement.StartPercentX * float64(size.width)
		fullTop := layout.placement.StartPercentY * float64(size.height)
		scaleX := (layout.placement.EndPercentX - layout.placement.StartPercentX) *
			float64(size.width) / float64(xTilesVisibleOnGameScreen*sprites.TileSize)
		scaleY := (layout.placement.EndPercentY - layout.placement.StartPercentY) *
			float64(size.height) / float64(yTilesVisibleOnGameScreen*sprites.TileSize)
		require.InDelta(t, scaleX, scaleY, 0.0001)
		xOffset := (xTilesVisibleOnGameScreen - cinematicTiles) / 2
		yOffset := (yTilesVisibleOnGameScreen - cinematicTiles) / 2
		require.InDelta(t, float64(layout.viewport.Min.X), fullLeft+float64(xOffset*sprites.TileSize)*scaleX, 0.51)
		require.InDelta(t, float64(layout.viewport.Min.Y), fullTop+float64(yOffset*sprites.TileSize)*scaleY, 0.51)
		require.InDelta(t, float64(layout.viewport.Dx()), float64(cinematicTiles*sprites.TileSize)*scaleX, 0.51)
	}
}

func TestMapUnitRenderSelectionUsesFloorOnlyOnSmallMaps(t *testing.T) {
	position := references.Position{X: 12, Y: 34}
	otherFloorVehicle := map_units.NewNPCFriendlyVehiceNewRef(references.FrigateVehicle, position, 0)
	currentFloorHorse := map_units.NewNPCFriendlyVehiceNewRef(references.HorseVehicle, position, 1)
	otherFloorVehicle.SetPos(position)
	currentFloorHorse.SetPos(position)
	mapUnits := map_units.MapUnits{otherFloorVehicle, currentFloorHorse}

	smallMapUnit := mapUnitAtPositionForRender(&mapUnits, references.SmallMapType, position, 1)
	require.NotNil(t, smallMapUnit)
	assert.Same(t, currentFloorHorse, (*smallMapUnit).(*map_units.NPCFriendly))

	largeMapUnit := mapUnitAtPositionForRender(&mapUnits, references.LargeMapType, position, 1)
	require.NotNil(t, largeMapUnit)
	assert.Same(t, otherFloorVehicle, (*largeMapUnit).(*map_units.NPCFriendly),
		"large-map rendering must preserve position-only selection")
}

func TestMapUnitOverlayIsSuppressedDuringCombat(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{gameState: state, xRayVisionActive: true}

	assert.True(t, scene.shouldDrawMapUnitsOverlay())
	state.MapState.PlayerLocation.Location = references.CombatLocation
	assert.False(t, scene.shouldDrawMapUnitsOverlay(), "town NPCs must not leak into combat presentation")

	state.MapState.PlayerLocation.Location = references.Britain
	scene.hideMapUnits = true
	assert.False(t, scene.shouldDrawMapUnitsOverlay())

	scene.hideMapUnits = false
	state.MapState.PlayerLocation.Location = references.Deceit
	assert.False(t, scene.shouldDrawMapUnitsOverlay(), "dungeon rendering must not expose projected map units")
}

func TestPerformingBardSuppressesChairOnlyWhileRenderable(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	position := references.Position{X: 11, Y: 10}
	layer := state.GetCurrentLayeredMap()
	layer.SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.ChairBackBack)
	layer.SetVisible(true, &position)
	npcReference := references.NPCReference{}
	npcReference.SetKeyIndex(indexes.BardPlaying1)
	bard := map_units.NewNPCFriendly(npcReference, 7)
	bard.SetPos(position)
	bard.SetFloor(state.MapState.PlayerLocation.Floor)
	bard.SetVisible(true)
	state.CurrentNPCAIController.GetNpcs().ReplaceWith([]map_units.MapUnit{bard})
	scene := &GameScene{gameState: state, xRayVisionActive: true}
	require.True(t, bard.IsVisible())
	require.Equal(t, indexes.SpriteIndex(indexes.BardPlaying1), bard.GetKeySpriteIndex())
	require.True(t, state.GetNPCSpriteReplacement(indexes.ChairBackBack, bard.GetKeySpriteIndex(), position).SuppressTerrainUnderlay)
	require.True(t, scene.shouldDrawMapUnitsOverlay())
	require.True(t, scene.shouldRenderMapUnitAtPosition(layer, &position))

	assert.True(t, scene.suppressTerrainUnderlayAtPosition(layer, references.SmallMapType, position))
	assert.Equal(t, indexes.SpriteIndex(indexes.ChairBackBack), layer.GetTileTopMapOnlyTile(&position).Index,
		"presentation must not mutate chair terrain")

	bard.SetVisible(false)
	assert.False(t, scene.suppressTerrainUnderlayAtPosition(layer, references.SmallMapType, position),
		"a hidden bard must not remove the chair")
	bard.SetVisible(true)
	scene.hideMapUnits = true
	assert.False(t, scene.suppressTerrainUnderlayAtPosition(layer, references.SmallMapType, position),
		"a suppressed actor layer must leave terrain visible")
	scene.hideMapUnits = false
	layer.SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.BrickFloor)
	assert.False(t, scene.suppressTerrainUnderlayAtPosition(layer, references.SmallMapType, position),
		"an off-chair bard uses ordinary terrain composition")
}

func TestStatusBandWornAndEffectsRemainReadOnlyAcrossGameplayContexts(t *testing.T) {
	state := &game_state.GameState{}
	state.SetRandomSeed(536)
	state.ElapsedMs = 4567
	state.CrownState = game_state.ToggleItemState{IsActive: true, TurnsRemaining: 255}
	state.AmuletState = state.CrownState
	state.BadgeState = state.CrownState
	state.PartyState.Inventory.Provisions.Food.Set(0)
	band := calculateRightRailLayout(image.Rect(0, 0, 1280, 720)).activeBorder
	panel := NewActiveEffectsPanel(state, syntheticStatusSpriteSheet(t), band)
	scene := &GameScene{gameState: state, activeEffectsPanel: panel, screenShakeOffsetX: 31, screenShakeOffsetY: -17}
	screen := newPresentationRaster(1280, 720)
	t.Cleanup(screen.Dispose)
	worn := []game_state.WornArtifactStatus{
		{ID: game_state.ToggleItemCrown, Worn: true},
		{ID: game_state.ToggleItemAmulet, Worn: true},
		{ID: game_state.ToggleItemBadge, Worn: true},
	}
	var firstPixels []byte
	bandImage := screen.SubImage(band).(*ebiten.Image)
	pixels := make([]byte, band.Dx()*band.Dy()*4)
	for _, location := range []references.Location{
		references.Britain,
		references.BritanniaUnderworld,
		references.CombatLocation,
		references.CampingLocation,
		references.ShrineLocation,
		references.CodexLocation,
		references.Deceit,
	} {
		state.MapState.PlayerLocation.Location = location
		before := *state
		for range 3 {
			screen.Clear()
			scene.activeEffectsPanel.Draw(screen)
			assert.Equal(t, worn, panel.worn, "location %d", location)
			require.Len(t, panel.visibleEffects, 1, "location %d", location)
			assert.Equal(t, game_state.PersistentEffectStarvation, panel.visibleEffects[0].ID,
				"Crown suppression is omitted without hiding ordinary effects")
			assert.Equal(t, before, *state, "draw is presentation-only even without callbacks")
			assert.Zero(t, state.GetRNG().Calls())
			assert.False(t, state.MapState.Lighting.HasAnyLight(), "darkness never hides status")
			bandImage.ReadPixels(pixels)
			if firstPixels == nil {
				firstPixels = append([]byte(nil), pixels...)
			}
			assert.True(t, bytes.Equal(firstPixels, pixels), "contexts and world shake never change static status pixels")
		}
		assert.False(t, scene.usesCinematicSplitLayout(), "ordinary gameplay keeps the rail")
	}
	scene.finalePresentation = &lordBritishFinalePresentation{mode: finaleRescueMap}
	assert.True(t, scene.usesCinematicSplitLayout(), "intentional cinematic rail suppression remains")
}

func TestStatusBandReplacesLegacyAvatarPathAndStaysInStaticDrawBlock(t *testing.T) {
	for _, path := range []string{"gamescene_draw_core.go", "gamescene_draw_entities.go", "../../internal/game_state/toggle_item_state.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		require.NoError(t, err)
		ast.Inspect(file, func(node ast.Node) bool {
			if id, ok := node.(*ast.Ident); ok {
				assert.NotContains(t, []string{"drawActiveToggleItems", "shouldDrawAvatarAttachedToggleItems", "GetActiveToggleItemSprites"}, id.Name)
			}
			return true
		})
	}
	file, err := parser.ParseFile(token.NewFileSet(), "gamescene_draw_core.go", nil, 0)
	require.NoError(t, err)
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		block, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		condition, ok := block.Cond.(*ast.UnaryExpr)
		if !ok || condition.Op != token.NOT {
			return true
		}
		name, ok := condition.X.(*ast.Ident)
		if !ok || name.Name != "usesCinematicSplit" {
			return true
		}
		ast.Inspect(block.Body, func(child ast.Node) bool {
			call, ok := child.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Draw" || len(call.Args) != 1 {
				return true
			}
			owner, ok := selector.X.(*ast.SelectorExpr)
			if ok && owner.Sel.Name == "activeEffectsPanel" {
				argument, ok := call.Args[0].(*ast.Ident)
				require.True(t, ok)
				assert.Equal(t, "screen", argument.Name, "never worldRenderTarget or terrain mask")
				found = true
			}
			return true
		})
		return true
	})
	assert.True(t, found)
}

// The cinematic map's frame is part of the scene. Drawn after the dialogs it cut
// through any window opened over the map, such as the ending's recovery choice.
func TestCinematicMapFrameIsDrawnUnderTheDialogs(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "gamescene_draw_core.go", nil, 0)
	require.NoError(t, err)
	calls := map[string]token.Pos{}
	ast.Inspect(file, func(node ast.Node) bool {
		function, ok := node.(*ast.FuncDecl)
		if !ok || function.Name.Name != "Draw" {
			return true
		}
		ast.Inspect(function.Body, func(child ast.Node) bool {
			call, ok := child.(*ast.CallExpr)
			if !ok {
				return true
			}
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
				calls[selector.Sel.Name] = call.Pos()
			}

			return true
		})

		return false
	})
	require.Contains(t, calls, "drawCinematicMapBorder")
	require.Contains(t, calls, "drawDialogs")
	assert.Less(t, calls["drawCinematicMapBorder"], calls["drawDialogs"])
}
