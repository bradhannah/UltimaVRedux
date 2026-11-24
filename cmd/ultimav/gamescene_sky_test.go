package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/shrine"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/bradhannah/UltimaVRedux/test/originaldata"
)

func TestSkyAllPhasesAndHourlyBoundaries(t *testing.T) {
	trammel := []int{0, 1, 1, 2, 2, 3, 3, 4, 5, 5, 6, 6, 7, 7, 0, 1, 1, 2, 2, 3, 3, 4, 5, 5, 6, 6, 7, 7}
	felucca := []int{0, 0, 1, 2, 3, 4, 5, 6, 7, 0, 0, 1, 2, 3, 4, 5, 6, 7, 0, 0, 1, 2, 3, 4, 5, 6, 7, 0}
	for day := 1; day <= 28; day++ {
		for hour := range 24 {
			for _, minute := range []byte{0, 59} {
				date := datetime.UltimaDate{Year: 139, Month: 4, Day: byte(day), Hour: byte(hour), Minute: minute}
				want := skyStrip{visible: true}
				if hour >= 6 && hour <= 17 {
					want.glyphs[17-hour] = 0x2a
				}
				if hour <= 8 || hour >= 21 {
					want.glyphs[(32-hour)%24] = 0x30 + trammel[day-1]
				}
				if hour <= 2 || hour >= 15 {
					want.glyphs[(26-hour)%24] = 0x30 + felucca[day-1]
				}
				assert.Equal(t, want, projectSky(date), "day %d %02d:%02d", day, hour, minute)
			}
		}
	}
	strip := projectSky(datetime.UltimaDate{Day: 8, Hour: 22})
	assert.Equal(t, 0x36, strip.glyphs[4], "day-8 reproduction: Felucca on left")
	assert.Equal(t, 0x34, strip.glyphs[10], "day-8 reproduction: Trammel on right")
}

func newSkyTestScene(t *testing.T, origin references.PlayerLocation) *GameScene {
	t.Helper()
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).WithSystemCallbacks().WithRandomSeed(634).Build()
	state.MapState.PlayerLocation = origin
	state.DateTime = datetime.UltimaDate{Year: 139, Month: 4, Day: 8, Hour: 22}
	scene := &GameScene{gameState: state}
	// These partial scenes own only the presentation resources exercised here.
	// Register cleanup before any assertion or later resize can exit the test.
	t.Cleanup(func() {
		scene.releaseDungeonHUD()
		if scene.borders.skyBacking != nil {
			scene.borders.skyBacking.Deallocate()
			scene.borders.skyBacking = nil
		}
	})

	return scene
}

func TestSkyContextAndInheritedLifecycle(t *testing.T) {
	for _, origin := range []references.PlayerLocation{
		{Location: references.BritanniaUnderworld},
		{Location: references.BritanniaUnderworld, Floor: -1},
		{Location: references.Britain},
		{Location: references.LordBritishsCastle, Floor: 1},
		{Location: references.LordBritishsCastle, Floor: -1},
		{Location: references.Ararat},
		{Location: references.Deceit, Floor: 3},
	} {
		t.Run(fmt.Sprintf("%s/%d", origin.Location, origin.Floor), func(t *testing.T) {
			g := newSkyTestScene(t, origin)
			g.syncSkyPresentation()
			initial := g.sky.strip
			assert.Equal(t, skyEligible(origin), initial.visible)
			// Combat retains the sun only, including across midnight/subsequent hours.
			g.gameState.CombatScene = &game_state.CombatState{PreCombatPlayerLocation: origin}
			g.gameState.MapState.PlayerLocation = references.PlayerLocation{Location: references.CombatLocation}
			g.gameState.DateTime.Advance(180)
			g.syncSkyPresentation()
			assert.Equal(t, initial.withoutMoons(), g.sky.strip)
			g.gameState.DateTime.Advance(60)
			g.syncSkyPresentation()
			assert.Equal(t, initial.withoutMoons(), g.sky.strip)
			g.gameState.CombatScene = nil
			g.gameState.MapState.PlayerLocation = origin
			g.syncSkyPresentation()
			if initial.visible {
				assert.Equal(t, projectSky(g.gameState.DateTime), g.sky.strip)
				assert.NotEqual(t, initial, g.sky.strip, "approved unconditional outdoor/town return refresh")
			}
			// Camping consults the original floor, not its tactical floor zero.
			g.gameState.CampingScene = camping.NewCampingSceneWithCombatRef(2, -1, origin, &references.CombatMapReference{})
			g.gameState.MapState.PlayerLocation = references.PlayerLocation{Location: references.CampingLocation}
			for range 24 {
				g.gameState.DateTime.Advance(5)
				g.syncSkyPresentation()
				assert.Equal(t, initial.visible, g.sky.strip.visible)
				if initial.visible {
					assert.Equal(t, projectSky(g.gameState.DateTime), g.sky.strip)
				}
			}
			// Ambush removes moons from the last live camping display.
			campSky := g.sky.strip
			g.gameState.CampingScene = nil
			g.gameState.CombatScene = &game_state.CombatState{PreCombatPlayerLocation: origin}
			g.gameState.MapState.PlayerLocation.Location = references.CombatLocation
			g.gameState.DateTime.Advance(65)
			g.syncSkyPresentation()
			assert.Equal(t, campSky.withoutMoons(), g.sky.strip)
		})
	}
}

func TestSkyShrineAndExceptionalScenes(t *testing.T) {
	origin := references.PlayerLocation{Location: references.BritanniaUnderworld}
	g := newSkyTestScene(t, origin)
	g.syncSkyPresentation()
	want := g.sky.strip
	g.gameState.ShrineScene = shrine.NewShrineScene(0, origin)
	g.gameState.MapState.PlayerLocation.Location = references.ShrineLocation
	g.miniMapScene = minimap.NewMiniMapScene()
	g.gameState.DateTime.Advance(180)
	g.syncSkyPresentation()
	assert.Equal(t, want, g.sky.strip)
	g.gameState.ShrineScene = nil
	g.miniMapScene = nil
	g.gameState.MapState.PlayerLocation = origin
	g.syncSkyPresentation()
	assert.Equal(t, projectSky(g.gameState.DateTime), g.sky.strip)

	for _, location := range []references.Location{references.CodexLocation, references.BlackthornPrison, references.FinaleLocation} {
		g.gameState.MapState.PlayerLocation.Location = location
		// A handoff must follow the active map, even while old tactical state is
		// still being cleaned up and before the cinematic presenter is installed.
		g.gameState.CombatScene = &game_state.CombatState{PreCombatPlayerLocation: origin}
		g.miniMapScene = nil
		g.syncSkyPresentation()
		assert.False(t, g.sky.strip.visible, "%s before presenter", location)
		g.miniMapScene = minimap.NewMiniMapScene()
		g.syncSkyPresentation()
		assert.False(t, g.sky.strip.visible, "%s", location)
	}
	g.gameState.CombatScene = nil
	g.gameState.MapState.PlayerLocation = origin
	g.miniMapScene = nil
	g.gameState.EventQueueManager.Workflow(deathResurrectionWorkflowName).Priority(event_queue.PriorityNormal).Sequential().First(func() {}).Wait(1000).Start()
	g.syncSkyPresentation()
	assert.False(t, g.sky.strip.visible, "death sequence can still own a surface map")
}

func TestSkyDateEditsRolloverLoadsAndNoGameplayMutation(t *testing.T) {
	g := newSkyTestScene(t, references.PlayerLocation{Location: references.Britain})
	for _, date := range []datetime.UltimaDate{
		{Year: 139, Month: 4, Day: 8, Hour: 22},
		{Year: 139, Month: 4, Day: 28, Hour: 23, Minute: 59},
		{Year: 139, Month: 13, Day: 28, Hour: 23, Minute: 59},
	} {
		g.gameState.DateTime = date // Same authority updated by debug time commands.
		for range 2 {
			g.gameState.UpdateMoongateTransition()
			for range 30 {
				g.gameState.ElapsedMs += 200
				g.gameState.EventQueueManager.Update()
			}
			g.gameState.ProcessWindThroughElapsed(g.gameState.ElapsedMs)
			before, err := g.gameState.CaptureSaveSnapshot()
			require.NoError(t, err)
			for range 5 {
				g.syncSkyPresentation()
				assert.Equal(t, projectSky(g.gameState.DateTime), g.sky.strip)
			}
			after, err := g.gameState.CaptureSaveSnapshot()
			require.NoError(t, err)
			assert.Equal(t, before, after, "no clock, RNG, gate, inventory or travel mutations")
			g.gameState.DateTime.Advance(1)
		}
	}
	// Hydration and replacing a scene's authority must not retain its old sky.
	snapshot, err := g.gameState.CaptureSaveSnapshot()
	require.NoError(t, err)
	loaded, err := game_state.NewGameStateFromSaveSnapshot(snapshot, g.gameState.GameConfig, g.gameState.GameReferences,
		g.gameState.MapState.XTilesVisibleOnGameScreen, g.gameState.MapState.YTilesVisibleOnGameScreen)
	require.NoError(t, err)
	g.sky.strip = projectSky(datetime.UltimaDate{Day: 8, Hour: 22})
	g.gameState = loaded
	g.syncSkyPresentation()
	assert.Equal(t, projectSky(loaded.DateTime), g.sky.strip)
	recaptured, err := loaded.CaptureSaveSnapshot()
	require.NoError(t, err)
	assert.Equal(t, snapshot, recaptured)
}

func TestSkyDebugCalendarEdits(t *testing.T) {
	g := newSkyTestScene(t, references.PlayerLocation{Location: references.BritanniaUnderworld})
	g.syncSkyPresentation()
	for _, request := range []debugging.Request{
		{SessionID: "sky", ID: "day", CommandID: debugging.CommandCalendarSetDay,
			Arguments: map[string]json.RawMessage{"day": json.RawMessage(`1`)}},
		{SessionID: "sky", ID: "time", CommandID: debugging.CommandCalendarSetTime,
			Arguments: map[string]json.RawMessage{"hour": json.RawMessage(`12`), "minute": json.RawMessage(`30`)}},
	} {
		result := executeDebugCommand(g.gameState, request)
		require.True(t, result.Success)
		g.syncSkyPresentation()
		assert.Equal(t, projectSky(g.gameState.DateTime), g.sky.strip)
	}
}

func TestSkyProductionTransitions(t *testing.T) {
	for _, location := range []references.Location{references.BritanniaUnderworld, references.Britain} {
		g := newSkyTestScene(t, references.PlayerLocation{Location: location, Position: references.Position{X: 15, Y: 15}})
		g.syncSkyPresentation()
		inherited := g.sky.strip
		require.NoError(t, g.gameState.TransitionToCombatMap(int(references.GladeCombatMap), references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
		g.gameState.DateTime.Advance(181)
		g.syncSkyPresentation()
		assert.Equal(t, inherited.withoutMoons(), g.sky.strip)
		g.gameState.EndCombat(game_state.CombatEvacuated)
		g.syncSkyPresentation()
		assert.Equal(t, projectSky(g.gameState.DateTime), g.sky.strip)

		inherited = g.sky.strip
		require.NoError(t, g.gameState.TransitionToShrineMap(0))
		g.gameState.DateTime.Advance(120)
		g.syncSkyPresentation()
		assert.Equal(t, inherited, g.sky.strip)
		g.gameState.TransitionFromShrineMap()
		g.syncSkyPresentation()
		assert.Equal(t, projectSky(g.gameState.DateTime), g.sky.strip)
	}
}

func TestSkySuppressedPixelsAndInheritedDungeonHeader(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	// Snapshot restoration preserves logicalDirty; PublishLogicalSize in
	// cleanup would manufacture a pending resize for another scene's Update.
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(1001, 701)
	g := newSkyTestScene(t, references.PlayerLocation{Location: references.Britain})
	g.initializeDungeonHUDOutputs()
	outer, aperture := surfaceMapBorderGeometry()
	g.borders.surfaceAperture = aperture
	g.borders.outsideBorder, _ = sprites.NewBorderSprites().VeryPixelatedRoundedBlueWhite.CreateSizedAndScaledBorderSprite(borderWidthScaling, gameScreenPercents)
	g.initializeSkyBorder()
	screen := newPresentationRaster(outer.Dx(), outer.Dy())
	defer screen.Deallocate()
	for _, origin := range []references.PlayerLocation{
		{Location: references.BritanniaUnderworld, Floor: -1},
		{Location: references.Britain, Floor: -1},
		{Location: references.Ararat},
		{Location: references.Deceit, Floor: 3},
	} {
		g.gameState.MapState.PlayerLocation = origin
		screen.Clear()
		g.drawSkyStrip(screen)
		assert.Equal(t, make([]byte, outer.Dx()*outer.Dy()*4), dissolveMaskPixels(screen), "suppression must not paint over existing chrome or dungeon HUD")
	}
	origin := references.PlayerLocation{Location: references.Deceit, Floor: 3}
	for _, combat := range []bool{false, true} {
		g.gameState.CampingScene = camping.NewDungeonCampingScene(2, -1, origin, &references.CombatMapReference{})
		g.gameState.MapState.PlayerLocation.Location = references.CampingLocation
		if combat {
			g.gameState.CampingScene = nil
			g.gameState.CombatScene = &game_state.CombatState{PreCombatPlayerLocation: origin}
			g.gameState.MapState.PlayerLocation.Location = references.CombatLocation
		}
		screen.Clear()
		g.drawSkyStrip(screen)
		pixels := dissolveMaskPixels(screen)
		assert.NotEqual(t, make([]byte, len(pixels)), pixels, "dungeon level retained on tactical view")
		assert.False(t, g.sky.strip.visible)
		// Compare the entire decorated band, including the lower frame rule and
		// cap tips that a text-height slice would miss.
		baseline := newPresentationRaster(outer.Dx(), outer.Dy())
		g.drawDungeonHUD(baseline, newDungeonHUDStatus(origin, references.DungeonFacingNorth))
		height := aperture.Min.Y - outer.Min.Y
		want := dissolveMaskPixels(baseline)
		assert.Equal(t, want[:outer.Dx()*height*4], pixels[:outer.Dx()*height*4])
		assert.Equal(t, make([]byte, len(pixels)-outer.Dx()*height*4), pixels[outer.Dx()*height*4:],
			"inherited dungeon presentation adds neither a direction readout nor surface sky")
		baseline.Deallocate()
	}
	g.gameState.CombatScene = nil
	g.gameState.MapState.PlayerLocation.Location = references.Britain
	for _, mode := range []finalePresentationMode{finaleRescueMap, finaleEndingArt, finaleSuccessCertificate, finaleFailureScreen} {
		g.finalePresentation = &lordBritishFinalePresentation{mode: mode}
		screen.Clear()
		g.drawSkyStrip(screen)
		assert.Equal(t, make([]byte, outer.Dx()*outer.Dy()*4), dissolveMaskPixels(screen), "finale never leaks sky")
	}
}

func TestInheritedDungeonHeaderMatchesAfterResizeAndLevelChanges(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	g := newSkyTestScene(t, references.PlayerLocation{Location: references.Deceit})
	for _, size := range []image.Point{{X: 640, Y: 360}, {X: 1280, Y: 720}, {X: 1367, Y: 769}, {X: 1001, Y: 701}} {
		manager.PublishLogicalSize(size.X, size.Y)
		setDungeonBorderHUDTestSize(g)
		outer := g.borders.outsideBorder.Bounds()
		// Include two aperture rows and the whole right rail to catch leaks.
		region := image.Rect(0, 0, size.X, g.borders.surfaceAperture.Min.Y+2)
		ordinary := ebiten.NewImageWithOptions(region, &ebiten.NewImageOptions{Unmanaged: true})
		t.Cleanup(ordinary.Deallocate)
		inherited := ebiten.NewImageWithOptions(region, &ebiten.NewImageOptions{Unmanaged: true})
		t.Cleanup(inherited.Deallocate)
		for floor := range 8 {
			origin := references.PlayerLocation{Location: references.Deceit, Floor: references.FloorNumber(floor)}
			ordinary.Clear()
			ordinary.DrawImage(g.borders.outsideBorder, g.borders.outsideBorderOp)
			g.drawDungeonHUD(ordinary, newDungeonHUDStatus(origin, references.DungeonFacingWest))
			want := dissolveMaskPixels(ordinary)
			for _, combat := range []bool{false, true} {
				g.gameState.CombatScene = nil
				g.gameState.CampingScene = camping.NewDungeonCampingScene(2, -1, origin, &references.CombatMapReference{})
				g.gameState.MapState.PlayerLocation.Location = references.CampingLocation
				if combat {
					g.gameState.CampingScene = nil
					g.gameState.CombatScene = &game_state.CombatState{PreCombatPlayerLocation: origin}
					g.gameState.MapState.PlayerLocation.Location = references.CombatLocation
				}
				inherited.Clear()
				inherited.DrawImage(g.borders.outsideBorder, g.borders.outsideBorderOp)
				g.drawSkyStrip(inherited)
				assert.Equal(t, want, dissolveMaskPixels(inherited), "size=%v level=%d combat=%t entire header", size, floor+1, combat)
				assert.False(t, g.sky.strip.visible)
				assert.Equal(t, fmt.Sprintf("L %d", floor+1), g.dungeonHUD.level.GetOutputStr(false))
				assertDungeonBorderLabelLayout(t, &g.dungeonHUD.levelLabel,
					image.Rect(outer.Min.X, outer.Min.Y, outer.Max.X, g.borders.surfaceAperture.Min.Y), size.X)
			}
		}
		ordinary.Deallocate()
		inherited.Deallocate()
	}
}

func TestDungeonHUDSceneDisposalReleasesLabels(t *testing.T) {
	g := newSkyTestScene(t, references.PlayerLocation{Location: references.Deceit})
	setDungeonBorderHUDTestSize(g)
	outer := g.borders.outsideBorder.Bounds()
	screen := newPresentationRaster(outer.Dx(), outer.Dy())
	defer screen.Deallocate()
	g.drawDungeonHUD(screen, dungeonHUDStatus{floor: 0, facing: references.DungeonFacingNorth})
	level, direction := g.dungeonHUD.levelLabel.backing, g.dungeonHUD.directionLabel.backing
	g.Dispose()
	assert.Zero(t, g.dungeonHUD)
	assertDungeonBackingReleased(t, level)
	assertDungeonBackingReleased(t, direction)
	assert.NotPanics(t, g.Dispose)
}

func TestSkyGlyphPixelsMatchOriginalRunes(t *testing.T) {
	path := originaldata.Directory(t, "RUNES.CH")
	data, err := os.ReadFile(filepath.Join(path, "RUNES.CH"))
	require.NoError(t, err)
	require.Len(t, data, 1024)
	for _, glyph := range []int{0x2a, 0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37} {
		img := text.GetBitmapFonts().Runic.GetCharImage(glyph)
		pixels := make([]byte, 8*8*4)
		img.ReadPixels(pixels)
		for y := range 8 {
			for x := range 8 {
				want := []byte{0, 0, 0, 0}
				if data[glyph*8+y]&(1<<(7-x)) != 0 {
					want = []byte{255, 255, 255, 255}
				}
				require.Equal(t, want, pixels[(y*8+x)*4:(y*8+x+1)*4], "glyph %02x pixel %d,%d", glyph, x, y)
			}
		}
	}
}

func TestSkyPixelsScaleInsideStaticTopBorder(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	g := newSkyTestScene(t, references.PlayerLocation{Location: references.Britain})
	lowerWhiteObserved := false
	for _, size := range []image.Point{{X: 640, Y: 360}, {X: 1001, Y: 701}, {X: 1280, Y: 720}, {X: 1366, Y: 768}, {X: 1367, Y: 769}, {X: 1920, Y: 1080}, {X: 1921, Y: 1081}, {X: 2560, Y: 1440}, {X: 3840, Y: 2160}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			manager.PublishLogicalSize(size.X, size.Y)
			raster := size.X == 1001 || size.X == 1280 || size.X == 1366 || size.X == 1367
			// Large-size geometry cases use an explicit synthetic border band.
			// Calling surfaceMapBorderGeometry would itself allocate and read a
			// full raster frame, even if we later skipped drawing the sky.
			outer := image.Rect(0, 0, int(float64(size.X)*gameScreenPercents.EndPercentX), size.Y)
			aperture := outer.Inset(max(1, size.X/128))
			if raster {
				outer, aperture = surfaceMapBorderGeometry()
			}
			g.borders.surfaceAperture = aperture
			bounds := skyStripBounds(outer, aperture)
			require.False(t, bounds.Empty())
			require.True(t, bounds.In(image.Rect(0, 0, outer.Max.X, aperture.Min.Y)))
			backing := image.Rect(bounds.Min.X, outer.Min.Y, bounds.Max.X, aperture.Min.Y)
			oldCellSize := min(aperture.Min.Y-outer.Min.Y, outer.Dx()/(skyCells+2))
			if oldCellSize >= text.BitmapCharHeight {
				oldCellSize -= oldCellSize % text.BitmapCharHeight
			}
			oldWidth := (skyCells + 2) * oldCellSize
			maxWidth := max(oldWidth, outer.Dx()-2*max(1, (aperture.Min.Y-outer.Min.Y)/2))
			assert.Equal(t, min(maxWidth, oldWidth*skyWidthNumerator/skyWidthDenominator), bounds.Dx())
			assert.Greater(t, bounds.Min.X, outer.Min.X, "the sky must not erase the left rounded frame corner")
			assert.Less(t, bounds.Max.X, outer.Max.X, "the sky must not erase the right rounded frame corner")
			assert.InDelta(t, float64(outer.Dx())/2, float64(bounds.Min.X+bounds.Max.X)/2, 0.5)
			for cell := range skyCells - 1 {
				assert.GreaterOrEqual(t, skyGlyphX(bounds, cell+1)-skyGlyphX(bounds, cell), float64(bounds.Dy()),
					"wider cells cannot overlap glyphs")
			}
			// All nine resolutions retain the geometry checks above. GPU glyph,
			// cap, border-rule and world-motion oracles need only representative
			// fractional and adjacent odd/even layouts, not full 4K canvases.
			if !raster {
				return
			}
			g.borders.outsideBorder, g.borders.outsideBorderOp = sprites.NewBorderSprites().VeryPixelatedRoundedBlueWhite.CreateSizedAndScaledBorderSprite(borderWidthScaling, gameScreenPercents)
			g.initializeSkyBorder()
			assert.Equal(t, bounds, g.borders.skyBounds)
			assert.Equal(t, backing, g.borders.skyBackingBounds)
			wind := NewWindDisplay()
			wind.ensureText(environment.North)
			defer wind.background.Deallocate()
			assert.InDelta(t, float64(wind.backgroundBounds.Min.X+wind.backgroundBounds.Max.X)/2,
				float64(backing.Min.X+backing.Max.X)/2, 0.5, "sky and wind share the rasterized map-frame center")
			screen := newPresentationRaster(size.X, size.Y)
			defer screen.Deallocate()
			g.surfaceCamera = surfaceCameraProjection{active: true, x: 8.75, y: -15.5}
			g.screenShakeOffsetX, g.screenShakeOffsetY = -27, 21
			g.drawSkyStrip(screen)
			date, rngCalls, stage := g.gameState.DateTime, g.gameState.GetRNG().Calls(), g.gameState.MoongateStage()
			pixels := make([]byte, size.X*size.Y*4)
			screen.ReadPixels(pixels)
			for y := range size.Y {
				for x := range size.X {
					if !image.Pt(x, y).In(backing) {
						require.Zero(t, pixels[(y*size.X+x)*4+3], "HUD leak at %d,%d", x, y)
					}
				}
			}
			// Verify every glyph foreground/background pixel after actual GPU drawing.
			for _, sample := range []struct{ day, hour byte }{{8, 12}, {8, 22}, {1, 22}} {
				g.gameState.DateTime.Day, g.gameState.DateTime.Hour = sample.day, sample.hour
				beforeDate := g.gameState.DateTime
				g.drawSkyStrip(screen)
				assertSkyRenderedCells(t, screen, bounds, g.sky.strip)
				assert.Equal(t, beforeDate, g.gameState.DateTime, "drawing cannot advance time")
			}
			g.gameState.DateTime = date
			g.drawSkyStrip(screen)
			baseline := make([]byte, len(pixels))
			screen.ReadPixels(baseline)
			g.surfaceCamera = surfaceCameraProjection{}
			g.screenShakeOffsetX, g.screenShakeOffsetY = 0, 0
			g.drawSkyStrip(screen)
			screen.ReadPixels(pixels)
			assert.Equal(t, baseline, pixels, "world transforms never move the static strip")
			assert.Equal(t, date.Day, g.gameState.DateTime.Day)
			assert.Equal(t, rngCalls, g.gameState.GetRNG().Calls())
			assert.Equal(t, stage, g.gameState.MoongateStage())
			// Both end caps are mirrored, opaque blue/white arrows around black sky.
			for y := range backing.Dy() {
				for x := range max(1, backing.Dy()/2) {
					assert.Equal(t, g.borders.skyBacking.At(x, y), g.borders.skyBacking.At(backing.Dx()-x-1, y))
				}
			}
			capWidth := max(1, backing.Dy()/2)
			slope := float64(2*(capWidth-1)) / float64(max(1, backing.Dy()-1))
			wantWhiteSpan := max(1, int(math.Round(float64(wind.borderStroke)*math.Hypot(1, slope))))
			whiteSpan := 0
			for x := range capWidth {
				if g.borders.skyBacking.At(x, backing.Dy()/2) == ucolor.EGAWhite.ToRGBA() {
					whiteSpan++
				}
			}
			assert.Equal(t, wantWhiteSpan, whiteSpan, "sky endcaps must have the same border-relative stroke as Winds")
			lowerWhiteObserved = assertSkyCoversLowerBorderRule(t, g, outer, aperture) || lowerWhiteObserved
		})
	}
	require.True(t, lowerWhiteObserved, "test must exercise a white rule formerly visible below the sky strip")
}

func assertSkyCoversLowerBorderRule(t *testing.T, g *GameScene, outer, aperture image.Rectangle) bool {
	t.Helper()
	band := image.Rect(outer.Min.X, outer.Min.Y, outer.Max.X, aperture.Min.Y)
	frame := ebiten.NewImageWithOptions(band, &ebiten.NewImageOptions{Unmanaged: true})
	t.Cleanup(frame.Deallocate)
	frame.DrawImage(g.borders.outsideBorder, g.borders.outsideBorderOp)
	center := outer.Min.X + outer.Dx()/2
	coveredWhite := 0
	lowerWhite := 0
	for y := outer.Min.Y; y < aperture.Min.Y; y++ {
		if frame.At(center, y) == ucolor.EGAWhite.ToRGBA() {
			coveredWhite++
			if y >= g.borders.skyBounds.Max.Y {
				lowerWhite++
			}
		}
	}
	require.Positive(t, coveredWhite, "the border contains a real white rule to cover")
	composed := ebiten.NewImageWithOptions(band, &ebiten.NewImageOptions{Unmanaged: true})
	t.Cleanup(composed.Deallocate)
	composed.DrawImage(g.borders.outsideBorder, g.borders.outsideBorderOp)
	g.drawSkyStrip(composed)
	// Sample the lower rule away from glyphs (their cells are vertically centered).
	for y := g.borders.skyBounds.Max.Y; y < aperture.Min.Y; y++ {
		assert.Equal(t, ucolor.EGABlack.ToRGBA(), color.RGBAModel.Convert(composed.At(center, y)),
			"the lower white border rule must not peek out at %d", y)
	}
	for _, x := range []int{g.borders.skyBackingBounds.Min.X - 1, g.borders.skyBackingBounds.Max.X} {
		if x < outer.Min.X || x >= outer.Max.X {
			continue
		}
		for y := outer.Min.Y; y < aperture.Min.Y; y++ {
			assert.Equal(t, color.RGBAModel.Convert(frame.At(x, y)), color.RGBAModel.Convert(composed.At(x, y)),
				"backing must not erase frame outside its bookends")
		}
	}

	return lowerWhite > 0
}

func TestSkyFullDrawWorldEffectsDialogsAndResize(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(1280, 720)
	g := newJournalSnapshotOriginalScene(t)
	// The graphics test runner deliberately starts unfocused. This detached
	// fixture exercises presentation updates, independently of desktop focus.
	g.transientCommandInput.focused = func() bool { return true }
	g.gameConfig.DisplayConfiguration = &config.DisplayConfiguration{}
	g.gameConfig.GameplayConfiguration = &config.GameplayConfiguration{}
	require.True(t, skyEligible(g.gameState.MapState.PlayerLocation))
	g.gameState.DateTime = datetime.UltimaDate{Year: 139, Month: 4, Day: 8, Hour: 16}
	require.NoError(t, g.Update(nil))
	assert.Equal(t, projectSky(g.gameState.DateTime), g.sky.strip, "Update captures inheritance even before the first Draw")
	width, height := display.GetManager().GetCurrentSize()
	screen := newPresentationRaster(width, height)
	defer screen.Deallocate()
	g.Draw(screen)
	assertSkyRenderedCells(t, screen, g.borders.skyBounds, g.sky.strip)
	// These deliberately oversized world effects would cover the top border if
	// composed after the HUD (including the non-surface tactical render path).
	g.screenFlashIntensity = 1
	g.screenShakeOffsetX, g.screenShakeOffsetY = 31, -29
	g.screenInvertIntensity = 1
	pos := g.gameState.MapState.PlayerLocation.Position
	g.visualEffects = []VisualEffect{{WorldPos: &pos, SpriteIndex: indexes.Grass, Type: "kapow", Duration: 1, Elapsed: 0.5, ScaleMin: 100, ScaleMax: 100}}
	g.Draw(screen)
	assertSkyRenderedCells(t, screen, g.borders.skyBounds, g.sky.strip)
	require.NoError(t, g.gameState.TransitionToCombatMap(int(references.GladeCombatMap), references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
	pos = references.Position{X: 5, Y: 0}
	g.Draw(screen)
	assertSkyRenderedCells(t, screen, g.borders.skyBounds, g.sky.strip)
	inherited := g.sky.strip
	g.gameState.DateTime.Advance(181)
	manager.PublishLogicalSize(1920, 1080)
	resized := ebiten.NewImage(1920, 1080)
	defer resized.Deallocate()
	g.Draw(resized)
	assert.Equal(t, inherited, g.sky.strip, "resize preserves only the inherited sun")
	assertSkyRenderedCells(t, resized, g.borders.skyBounds, g.sky.strip)
	screen = resized
	g.dialogStack.PushModalDialog(&provisionOpaqueModal{bounds: g.borders.skyBounds})
	g.Draw(screen)
	for y := g.borders.skyBounds.Min.Y; y < g.borders.skyBounds.Max.Y; y++ {
		for x := g.borders.skyBounds.Min.X; x < g.borders.skyBounds.Max.X; x++ {
			require.Equal(t, ucolor.EGABlue.ToRGBA(), color.RGBAModel.Convert(screen.At(x, y)), "dialog must cover HUD")
		}
	}
}

func TestCombatMoonSuppressionPixelsAndColdEntry(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	origin := references.PlayerLocation{Location: references.Britain}
	g := newSkyTestScene(t, origin)
	for _, size := range []image.Point{{X: 640, Y: 360}, {X: 1280, Y: 720}} {
		manager.PublishLogicalSize(size.X, size.Y)
		_, aperture := surfaceMapBorderGeometry()
		g.borders.surfaceAperture = aperture
		g.borders.outsideBorder, _ = sprites.NewBorderSprites().VeryPixelatedRoundedBlueWhite.CreateSizedAndScaledBorderSprite(borderWidthScaling, gameScreenPercents)
		g.initializeSkyBorder()
		for _, hour := range []byte{5, 16, 22} { // one moon, sun plus moon, two moons
			for _, warm := range []bool{false, true} {
				g.gameState.CombatScene = nil
				g.gameState.MapState.PlayerLocation = origin
				g.gameState.DateTime = datetime.UltimaDate{Year: 139, Month: 4, Day: 8, Hour: hour}
				g.sky = skyPresentation{}
				if warm {
					g.syncSkyPresentation()
				}
				g.gameState.CombatScene = &game_state.CombatState{PreCombatPlayerLocation: origin}
				g.gameState.MapState.PlayerLocation.Location = references.CombatLocation
				date, rng, stage := g.gameState.DateTime, g.gameState.GetRNG().Calls(), g.gameState.MoongateStage()
				for range 3 {
					g.syncSkyPresentation()
				}
				want := skyStrip{}
				if hour == 16 {
					want.visible = true
					want.glyphs[1] = skySunGlyph
				}
				require.Equal(t, want, g.sky.strip)
				screen := newPresentationRaster(size.X, size.Y)
				g.drawSkyStrip(screen)
				if hour == 16 {
					assertSkyRenderedCells(t, screen, g.borders.skyBounds, want)
				} else {
					require.Equal(t, make([]byte, size.X*size.Y*4), dissolveMaskPixels(screen), "no glyph or orphan backing")
				}
				screen.Deallocate()
				require.Equal(t, date, g.gameState.DateTime)
				require.Equal(t, rng, g.gameState.GetRNG().Calls())
				require.Equal(t, stage, g.gameState.MoongateStage())
				g.gameState.CombatScene = nil
				g.gameState.MapState.PlayerLocation = origin
				g.gameState.DateTime.Advance(65)
				g.syncSkyPresentation()
				require.Equal(t, projectSky(g.gameState.DateTime), g.sky.strip, "exit recomputes actual phases and positions")
			}
		}
	}
}

func TestSkyDisposalReleasesPresentation(t *testing.T) {
	g := newSkyTestScene(t, references.PlayerLocation{Location: references.Britain})
	g.borders.skyBacking = ebiten.NewImage(8, 8)
	g.syncSkyPresentation()
	g.Dispose()
	assert.Nil(t, g.borders.skyBacking)
	assert.Zero(t, g.sky)
	assert.NotPanics(t, g.Dispose)
}

func assertSkyRenderedCells(t *testing.T, screen *ebiten.Image, bounds image.Rectangle, strip skyStrip) {
	t.Helper()
	cellSize := bounds.Dy()
	scale := float64(cellSize) / 8
	for cell, glyph := range strip.glyphs {
		glyphX := skyGlyphX(bounds, cell)
		startX := int(math.Floor(glyphX))
		var img *ebiten.Image
		if glyph != 0 {
			img = text.GetBitmapFonts().Runic.GetCharImage(glyph)
		}
		for y := range cellSize {
			for x := range cellSize {
				got := color.RGBAModel.Convert(screen.At(startX+x, bounds.Min.Y+y))
				require.Contains(t, skyNearestPixelColours(img, glyph, float64(startX+x)-glyphX, float64(y), scale), got,
					"cell %d glyph %02x pixel %d,%d", cell, glyph, x, y)
			}
		}
	}
}

func skyNearestPixelColours(img *ebiten.Image, glyph int, x, y, scale float64) []color.RGBA {
	if img == nil {
		return []color.RGBA{ucolor.EGABlack.ToRGBA()}
	}
	// With fractional reduction, exact texel-edge ties can select either neighbour
	// depending on GPU atlas coordinates. Integral magnification has no such ties.
	// All non-tie pixels must match the original glyph exactly, at every scale.
	var colours []color.RGBA
	for _, epsilonX := range []float64{-1e-6, 1e-6} {
		for _, epsilonY := range []float64{-1e-6, 1e-6} {
			sourceX := int((x+0.5)/scale + epsilonX)
			sourceY := int((y+0.5)/scale + epsilonY)
			_, _, _, alpha := img.At(img.Bounds().Min.X+sourceX, img.Bounds().Min.Y+sourceY).RGBA()
			colour := ucolor.EGABlack.ToRGBA()
			if alpha != 0 {
				colour = ucolor.EGALightGray.ToRGBA()
				if glyph == skySunGlyph {
					colour = ucolor.EGAYellow.ToRGBA()
				}
			}
			colours = append(colours, colour)
		}
	}
	return colours
}
