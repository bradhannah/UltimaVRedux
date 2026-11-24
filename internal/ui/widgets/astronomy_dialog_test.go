package widgets

import (
	"image"
	"testing"

	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

func TestAstronomyEdgeColumnsKeepWholePlanetAndCometVisible(t *testing.T) {
	for _, column := range []int{1, 22} {
		buffer := ebiten.NewImage(320, 200)
		t.Cleanup(buffer.Deallocate)
		dialog := &AstronomyDialog{bounds: image.Rect(0, 0, 320, 200), buffer: buffer,
			shadowlordPlanetIndices: []int{7}}
		for index := range dialog.planetXPositions {
			dialog.planetXPositions[index] = column
		}
		dialog.renderPlanets()
		dialog.renderComets()
		cx := astronomyPlanetX(column, 320)
		cy := 9 // Track position 7 has Y offset 1; screen Y increases downward; ref:ASTRO-0005.
		require.GreaterOrEqual(t, cx-6, 0)
		require.Less(t, cx+36, 320)
		require.Equal(t, ucolor.From(ucolor.EGAWhite).RGBA(), buffer.At(cx-6, cy))
		require.Equal(t, ucolor.From(ucolor.EGALightRed).RGBA(), buffer.At(cx+36, cy))
	}
}

func TestAstronomyRenderedOrderMatchesOriginalScreenCoordinates(t *testing.T) {
	buffer := ebiten.NewImage(640, 400)
	t.Cleanup(buffer.Deallocate)
	dialog := &AstronomyDialog{bounds: buffer.Bounds(), buffer: buffer,
		shadowlordPlanetIndices: []int{0, 1, 2, 3, 4, 5, 6, 7}}
	// Distinct columns identify each planet independently of the Y projection.
	for index := range dialog.planetXPositions {
		dialog.planetXPositions[index] = 1 + index*3
	}
	dialog.renderPlanets()
	dialog.renderComets()
	// Original Y-track offsets are passed directly to planet drawing; ref:ASTRO-0005.
	for index, row := range []int{18, 17, 15, 13, 11, 8, 5, 1} {
		x := astronomyPlanetX(1+index*3, 640)
		y := row * 18
		require.Equal(t, ucolor.From(ucolor.EGAWhite).RGBA(), buffer.At(x, y), "planet %d", index)
		require.Equal(t, ucolor.From(ucolor.EGALightRed).RGBA(), buffer.At(x+15, y), "comet %d", index)
	}
}

func TestAstronomyLowerPlanetsClearTelescopeAtSupportedSizes(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		setResponsiveDialogTestResolution(t, size.X, size.Y)
		// Production astronomy placement and a solid telescope make any overlap visible.
		sheet := &sprites.SpriteSheet{SpriteImage: ebiten.NewImage(512, 512)}
		sheet.SpriteImage.Fill(ucolor.EGAGreen)
		t.Cleanup(sheet.SpriteImage.Deallocate)
		dialog := NewAstronomyDialog(AstronomyDialogConfig{
			Placement:   sprites.PercentBasedPlacement{StartPercentX: 0.14, EndPercentX: 0.62, StartPercentY: 0.07, EndPercentY: 0.93},
			SpriteSheet: sheet, ShadowlordPlanetIndices: []int{0, 1},
			GameYear: 39, GameMonth: 4, GameDay: 5,
			RandomIntInRange: func(minimum, _ int) int { return minimum },
		})
		t.Cleanup(dialog.buffer.Deallocate)
		for index, row := range []int{18, 17} {
			// Put the lowest two planets over the telescope's horizontal footprint.
			dialog.planetXPositions[index] = 12
			y := int(float64(row) * float64(dialog.bounds.Dy()) * 0.9 / 20)
			require.Less(t, y+6, dialog.bounds.Dy()-76)
		}
		dialog.renderToBuffer()
		for _, row := range []int{18, 17} {
			x := astronomyPlanetX(12, dialog.bounds.Dx())
			y := int(float64(row) * float64(dialog.bounds.Dy()) * 0.9 / 20)
			require.Equal(t, ucolor.From(ucolor.EGAWhite).RGBA(), dialog.buffer.At(x, y))
			require.Equal(t, ucolor.From(ucolor.EGALightRed).RGBA(), dialog.buffer.At(x+15, y))
		}
	}
}

func TestCalculatePlanetXPositionUsesThirteenMonthYear(t *testing.T) {
	t.Parallel()

	// Planet eight moves once per day, making calendar errors directly visible.
	if got := calculatePlanetXPosition(7, 39, 13, 1); got != 19 {
		t.Fatalf("month 13 position = %d, want 19", got)
	}
	if got := calculatePlanetXPosition(7, 40, 1, 1); got != 13 {
		t.Fatalf("year rollover position = %d, want 13", got)
	}
}

func TestCalculatePlanetXPositionUsesTwoDigitYearAndLeavesEarlierDatesAtStart(t *testing.T) {
	t.Parallel()

	assert.Equal(t, calculatePlanetXPosition(7, 39, 4, 6), calculatePlanetXPosition(7, 139, 4, 6))
	assert.Equal(t, astronomyPlanetStartIndex()[7]+1, calculatePlanetXPosition(7, 38, 13, 28))
	assert.Equal(t, 3, calculatePlanetXPosition(7, 39, 4, 5))
	assert.Equal(t, 2, calculatePlanetXPosition(7, 39, 4, 6))
	assert.Equal(t, 12, calculatePlanetXPosition(0, 39, 4, 6))
}

func TestAstronomyDialogGeneratesFreshDeterministicStars(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	placement := sprites.PercentBasedPlacement{
		StartPercentX: 0.15,
		EndPercentX:   0.85,
		StartPercentY: 0.15,
		EndPercentY:   0.85,
	}
	firstState, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithRandomSeed(443).
		WithSystemCallbacks().
		Build()
	matchingState, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithRandomSeed(443).
		WithSystemCallbacks().
		Build()

	first := NewAstronomyDialog(AstronomyDialogConfig{
		Placement: placement, GameYear: 139, GameMonth: 4, GameDay: 6,
		RandomIntInRange: firstState.RandomIntInRange,
	})
	matching := NewAstronomyDialog(AstronomyDialogConfig{
		Placement: placement, GameYear: 139, GameMonth: 4, GameDay: 6,
		RandomIntInRange: matchingState.RandomIntInRange,
	})
	fresh := NewAstronomyDialog(AstronomyDialogConfig{
		Placement: placement, GameYear: 139, GameMonth: 4, GameDay: 6,
		RandomIntInRange: firstState.RandomIntInRange,
	})

	assert.Equal(t, first.starPositions, matching.starPositions)
	assert.NotEqual(t, first.starPositions, fresh.starPositions)
}
