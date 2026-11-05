package main

import (
	"fmt"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

func TestIntroNarrativeFrameTableIsCompleteAndOrdered(t *testing.T) {
	require.Len(t, introNarrativeFrames, 21)
	for index, frame := range introNarrativeFrames {
		assert.Equal(t, minimap.FrameID(fmt.Sprintf("intro-%02d", index+1)), frame.id)
		assert.NotEmpty(t, frame.shapes)
		assert.NotEmpty(t, frame.texts)
	}
	assert.NotNil(t, introNarrativeFrames[1].fizz)
	assert.Nil(t, introNarrativeFrames[0].fizz)
}

func TestNarrativeCanvasTransformReservesTwoPercentScreenPadding(t *testing.T) {
	for _, size := range []struct {
		width  int
		height int
	}{
		{width: 320, height: 200},
		{width: 1280, height: 720},
	} {
		scale, offsetX, offsetY := narrativeCanvasTransform(image.Rect(0, 0, size.width, size.height))
		minimumX := float64(size.width) * narrativeScreenPadding
		minimumY := float64(size.height) * narrativeScreenPadding

		assert.GreaterOrEqual(t, offsetX, minimumX)
		assert.GreaterOrEqual(t, offsetY, minimumY)
		assert.LessOrEqual(t, offsetX+introLogicalWidth*scale, float64(size.width)-minimumX)
		assert.LessOrEqual(t, offsetY+introLogicalHeight*scale, float64(size.height)-minimumY)
	}

	assert.Equal(t, sprites.PercentBasedPlacement{
		StartPercentX: 0.02,
		EndPercentX:   0.98,
		StartPercentY: 0.02,
		EndPercentY:   0.98,
	}, narrativeScreenPlacement())
}

func TestIntroNarrativeUsesMiniMapScriptAdvanceGates(t *testing.T) {
	presentation := newIntroNarrativePresentation(new(sprites.NarrativeIntroAssets))

	assert.Equal(t, minimap.FrameID("intro-01"), presentation.scene.GetCurrentFrame())
	for frame := 1; frame <= 21; frame++ {
		assert.True(t, presentation.executor.IsWaitingForAdvance())
		assert.True(t, presentation.executor.Advance())
		if frame < 21 {
			assert.Equal(t, minimap.FrameID(fmt.Sprintf("intro-%02d", frame+1)), presentation.scene.GetCurrentFrame())
		}
	}
	assert.True(t, presentation.executor.IsComplete())
}

func TestIntroNarrativeCharacterWidthHandlesMarkup(t *testing.T) {
	widths := make([]int, 91)
	widths[int('-'-' ')] = 3
	widths[int('A'-' ')] = 6

	assert.Equal(t, 5, introNarrativeCharacterWidth(' ', widths, 5))
	assert.Equal(t, 4, introNarrativeCharacterWidth(' ', widths, 4))
	assert.Equal(t, 15, introNarrativeCharacterWidth('{', widths, 5))
	assert.Zero(t, introNarrativeCharacterWidth('_', widths, 5))
	assert.Equal(t, 7, introNarrativeCharacterWidth('A', widths, 5))
}

func TestIntroNarrativeLineBacksUpToNearestSpaceOrFittingSoftHyphen(t *testing.T) {
	widths := make([]int, 91)
	for index := range widths {
		widths[index] = 4
	}

	for _, test := range []struct {
		text, line string
		maxWidth   int
		hyphen     bool
	}{
		{text: "ONE TWO THREE", line: "ONE TWO", maxWidth: 42, hyphen: false},
		{text: "ONE TWO_THREE", line: "ONE TWO", maxWidth: 42, hyphen: true},
		{text: "ONE TWO_THREE", line: "ONE", maxWidth: 40, hyphen: false},
	} {
		line := introNarrativeLine(test.text, 0, 0, test.maxWidth, widths, 5)

		assert.Equal(t, test.line, test.text[:line.end], test.text)
		assert.Equal(t, line.end+1, line.next, test.text)
		assert.Equal(t, test.hyphen, line.drawHyphen, test.text)
		assert.True(t, line.justify, test.text)
	}
}

func TestIntroNarrativeMenuWorkflowAdvancesOnAnyFreshKey(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.narrativeAssets = fakeNarrativeIntroAssets(t)
	scene.eventQueueManager = event_queue.NewEventQueueManager(func() int64 { return scene.elapsedMs }, nil)
	scene.nCurrentSelection = introChoiceNarrative
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	require.NoError(t, scene.Update(nil))
	require.NotNil(t, scene.narrative)

	for updates := 0; updates < 1000 && !scene.menuVisible; updates++ {
		if scene.narrative.executor.IsWaitingForAdvance() && scene.narrative.canAdvance(scene.elapsedMs) {
			scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
		}
		require.NoError(t, scene.Update(nil))
	}

	assert.True(t, scene.menuVisible)
	assert.Nil(t, scene.narrative)
	assert.False(t, scene.eventQueueManager.HasActiveWorkflows())
}

func fakeNarrativeIntroAssets(t *testing.T) *sprites.NarrativeIntroAssets {
	t.Helper()
	placeholder := ebiten.NewImage(1, 1)
	t.Cleanup(placeholder.Deallocate)
	counts := map[string]int{
		"TEXT.16": 6, "STORY1.16": 3, "STORY2.16": 4, "STORY3.16": 2,
		"STORY4.16": 2, "STORY5.16": 2, "STORY6.16": 8,
	}
	shapes := make(map[string][]*ebiten.Image, len(counts))
	for name, count := range counts {
		images := make([]*ebiten.Image, count)
		for index := range images {
			images[index] = placeholder
		}
		shapes[name] = images
	}

	return &sprites.NarrativeIntroAssets{
		Shapes: shapes, Glyphs: make([]*ebiten.Image, 91), GlyphWidths: make([]int, 91),
		StoryBlocks: make([]string, 20),
	}
}
