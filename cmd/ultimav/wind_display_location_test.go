package main

import (
	"image"
	"math"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestMapBorderLabelUsesFriendlyNamesAcrossLocationsFloorsAndContexts(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
		WithSystemCallbacks().WithRandomSeed(1).Build()
	scene := &GameScene{gameState: state}
	rngCalls := state.GetRNG().Calls()
	calendar := state.DateTime
	for _, location := range []references.Location{references.Britain, references.LordBritishsCastle, references.PalaceOfBlackthorn, references.SerpentsHold} {
		state.MapState.PlayerLocation.Location = location
		expected := state.GameReferences.LocationReferences.GetSmallLocationReference(location.String()).FriendlyLocationName
		require.NotEmpty(t, expected)
		for _, floor := range []references.FloorNumber{0, 1, -1} {
			state.MapState.PlayerLocation.Floor = floor
			require.Equal(t, expected, scene.mapBorderLabel())
		}
	}
	for _, location := range []references.Location{references.Deceit, references.Doom, references.CombatLocation,
		references.CampingLocation, references.ShrineLocation, references.CodexLocation, references.BlackthornPrison, references.FinaleLocation} {
		state.MapState.PlayerLocation.Location = location
		require.Empty(t, scene.mapBorderLabel(), "%v must not inherit the previous location name", location)
	}
	state.MapState.PlayerLocation.Location = references.BritanniaUnderworld
	require.Equal(t, state.GetCurrentWind().GetUIString(), scene.mapBorderLabel())
	state.MapState.PlayerLocation.Location = references.Britain
	scene.finalePresentation = &lordBritishFinalePresentation{}
	require.Empty(t, scene.mapBorderLabel())
	scene.finalePresentation = nil
	require.Equal(t, rngCalls, state.GetRNG().Calls())
	require.Equal(t, calendar, state.DateTime)
}

func TestMapBorderLabelAllRealNamesFitAndCacheAtSupportedResolutions(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	for _, size := range []image.Point{{1280, 720}, {1366, 768}, {1920, 1080}, {2560, 1440}, {3840, 2160}} {
		t.Run(size.String(), func(t *testing.T) {
			setTalkHistoryTestResolution(t, size.X, size.Y)
			label := NewWindDisplay()
			defer func() {
				if label.background != nil {
					label.background.Deallocate()
				}
			}()
			outer, aperture := surfaceMapBorderGeometry()
			longestWidth := 0.0
			longestName := ""
			for _, location := range references.GetListOfAllSmallMaps() {
				name := state.GameReferences.LocationReferences.GetSmallLocationReference(location).FriendlyLocationName
				width, _ := label.font.MeasureString(name)
				if width > longestWidth {
					longestWidth, longestName = width, name
				}
				// All real names must fit without clipping; measurement needs no raster.
				height := label.textBounds.Dy() + 2*label.verticalPadding
				capWidth := max(1, height/windCenterDivisor)
				require.LessOrEqual(t, math.Ceil(width)+float64(2*(label.padding+capWidth)), float64(label.textBounds.Dx()), "%s at %v", name, size)
			}
			require.NotEmpty(t, longestName)
			t.Logf("longest friendly label: %s (%.1fpx)", longestName, longestWidth)
			label.ensureLabel(longestName)
			require.Equal(t, longestName, label.lastText)
			require.Contains(t, label.output.GetOutputStr(false), longestName)
			require.Equal(t, 1, label.output.GetPanelContentLineCount())
			require.True(t, label.backgroundBounds.In(outer))
			require.GreaterOrEqual(t, label.backgroundBounds.Min.Y, aperture.Max.Y)
			require.InDelta(t, float64(outer.Min.X+outer.Max.X)/2, float64(label.backgroundBounds.Min.X+label.backgroundBounds.Max.X)/2, 0.5)
			cached := label.background
			label.ensureLabel(longestName)
			require.Same(t, cached, label.background)
			if size == (image.Point{1280, 720}) {
				// Retain one positive, label-sized production render rather than five
				// unasserted full-screen targets. Subimages preserve screen coordinates.
				frame := ebiten.NewImageWithOptions(label.backgroundBounds, nil)
				defer frame.Deallocate()
				label.DrawLabel(frame, longestName)
				require.Positive(t, countExactColor(frame, ucolor.EGAWhite))
			}
		})
	}
}

func TestMapBorderLocationCacheRefreshesOnTransitionAndResize(t *testing.T) {
	setTalkHistoryTestResolution(t, 1280, 720)
	label := NewWindDisplay()
	label.ensureLabel("Britain")
	first := label.background
	label.ensureLabel("Britain")
	require.Same(t, first, label.background, "floor changes keep the same name and raster")
	label.ensureLabel("Castle Britannia")
	require.NotSame(t, first, label.background)
	second := label.background
	setTalkHistoryTestResolution(t, 3840, 2160)
	label.Update()
	label.ensureLabel("Castle Britannia")
	require.NotSame(t, second, label.background)
	require.Equal(t, "Castle Britannia", label.lastText)
	label.background.Deallocate()
}
