package main

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/character_creation"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestCharacterCreationDiscardCardUsesScreenTypographyAndClassicFrame(t *testing.T) {
	fixture := newCharacterCreationSceneFixture(t, CharacterCreationCallbacks{
		OnComplete: func(character_creation.Result) {},
		OnCancel:   func() {},
	})
	fixture.completeIdentityAndIntroduction()
	fixture.press(ebiten.KeyEscape)
	scene := fixture.scene
	for _, size := range []image.Point{image.Pt(640, 480), image.Pt(1280, 720), image.Pt(1920, 1080), image.Pt(3840, 2160)} {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			logical := image.Rectangle{Max: size}
			pixelUnit, padding := scene.prepareDiscardConfirmation(logical)
			output, bounds := scene.discardOutput, scene.discardBounds
			require.NotNil(t, output)
			assert.InDelta(t, text.GetScaledNumberToWidth(size.X, 20), output.Font.TextFace.Size, 0.001)
			lines := strings.Split(output.GetOutputStr(false), "\n")
			for index := range lines {
				lines[index] = strings.TrimSpace(lines[index])
			}
			assert.Equal(t, []string{"Discard this character?", "[Y] Discard   [N/Esc] Keep creating"}, lines)
			require.True(t, bounds.In(logical))
			assert.InDelta(t, float64(size.X)/2, float64(bounds.Min.X+bounds.Max.X)/2, 1)
			assert.InDelta(t, float64(size.Y)/2, float64(bounds.Min.Y+bounds.Max.Y)/2, 1)
			assert.Equal(t, widgets.ClassicUIPixelUnit(size.X, size.Y), pixelUnit)
			assert.Equal(t, max(pixelUnit*4, int(math.Ceil(output.GetFontHeight()))), padding)
			interior := bounds.Inset(padding)
			assert.GreaterOrEqual(t, output.GetLineCapacity(interior.Dy()), 2)
			for _, line := range []string{"Discard this character?", "[Y] Discard   [N/Esc] Keep creating"} {
				width, _ := output.Font.MeasureString(line)
				assert.LessOrEqual(t, width, float64(interior.Dx()))
			}
			// All resolutions retain exact measured geometry. Rasterize only two
			// distinct pixel units, in a card-sized target with unchanged coordinates.
			if size.X != 640 && size.X != 1280 {
				return
			}
			screen := ebiten.NewImageWithOptions(bounds.Inset(-pixelUnit), &ebiten.NewImageOptions{Unmanaged: true})
			t.Cleanup(screen.Deallocate)
			scene.drawPreparedDiscardConfirmation(screen, pixelUnit, padding)
			assertCharacterCreationPixel(t, screen, size.X/2, bounds.Min.Y, ucolor.EGAWhite)
			assertCharacterCreationPixel(t, screen, bounds.Min.X, size.Y/2, ucolor.EGAWhite)
			assertCharacterCreationPixel(t, screen, size.X/2, bounds.Max.Y-1, ucolor.EGAWhite)
			assertCharacterCreationPixel(t, screen, bounds.Max.X-1, size.Y/2, ucolor.EGAWhite)
			screen.Fill(ucolor.EGAMagenta)
			scene.drawPreparedDiscardConfirmation(screen, pixelUnit, padding)
			assertCharacterCreationPixel(t, screen, bounds.Min.X, bounds.Min.Y, ucolor.EGAMagenta)
			assertCharacterCreationPixel(t, screen, bounds.Min.X+padding/2, size.Y/2, ucolor.EGABlack)
			_, _, _, alpha := screen.At(bounds.Min.X+padding/2, size.Y/2).RGBA()
			assert.EqualValues(t, 0xffff, alpha)
			assert.Same(t, output, scene.discardOutput, "stable resolution reuses Output")
		})
	}
}

func TestCharacterCreationDiscardPreservesPreviousCanvasAndArtwork(t *testing.T) {
	fixture := newCharacterCreationSceneFixture(t, CharacterCreationCallbacks{
		OnComplete: func(character_creation.Result) {},
		OnCancel:   func() { t.Fatal("unexpected cancellation") },
	})
	fixture.completeIdentityAndIntroduction()
	scene := fixture.scene
	setCharacterCreationTestArtwork(t, scene)
	scene.assets.Oracle.Fill(ucolor.EGABlue)
	scene.assets.Incense.Fill(ucolor.EGACyan)
	scene.assets.Shrine.Fill(ucolor.EGAMagenta)
	for _, symbol := range scene.assets.VirtueSymbols {
		symbol.Fill(ucolor.EGARed)
	}
	screen := newPresentationRaster(1280, 720)
	t.Cleanup(screen.Deallocate)
	for _, state := range []characterCreationState{
		characterCreationIntroduction, characterCreationFirstRound, characterCreationSecondRound,
		characterCreationFinalRound, characterCreationConclusion,
	} {
		scene.state = state
		scene.Draw(screen)
		before := make([]byte, introLogicalWidth*introLogicalHeight*4)
		scene.canvas.ReadPixels(before)
		beforeScreen := make([]byte, 1280*720*4)
		screen.ReadPixels(beforeScreen)
		fixture.press(ebiten.KeyEscape)
		require.Equal(t, state, scene.stateBeforeDiscard)
		scene.Draw(screen)
		after := make([]byte, len(before))
		scene.canvas.ReadPixels(after)
		require.True(t, bytes.Equal(before, after), "prior canvas changed for state %d", state)
		afterScreen := make([]byte, len(beforeScreen))
		screen.ReadPixels(afterScreen)
		unchangedOutsideCard := true
		for y := range 720 {
			for x := range 1280 {
				if !image.Pt(x, y).In(scene.discardBounds) {
					i := (y*1280 + x) * 4
					unchangedOutsideCard = unchangedOutsideCard && bytes.Equal(beforeScreen[i:i+4], afterScreen[i:i+4])
				}
			}
		}
		assert.True(t, unchangedOutsideCard, "card must not replace or dim prior artwork")
		fixture.press(ebiten.KeyN)
		assert.Equal(t, state, scene.state)
	}
}

func TestCharacterCreationDiscardChoicesWaitForHeldKeyRelease(t *testing.T) {
	for _, choice := range []ebiten.Key{ebiten.KeyY, ebiten.KeyN, ebiten.KeyEscape} {
		t.Run(choice.String(), func(t *testing.T) {
			cancelled := 0
			fixture := newCharacterCreationSceneFixture(t, CharacterCreationCallbacks{
				OnComplete: func(character_creation.Result) { t.Fatal("unexpected completion") },
				OnCancel:   func() { cancelled++ },
			})
			fixture.completeIdentityAndIntroduction()
			scene := fixture.scene
			questionnaire, calls := scene.questionnaire, scene.source.Calls()
			held := make(map[ebiten.Key]bool)
			scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
			held[ebiten.KeyEscape] = true
			fixture.update()
			require.Equal(t, characterCreationDiscardConfirmation, scene.state)
			assert.Nil(t, scene.dialogStack.PeekTopModalDialog(), "scene must retain choice input ownership")
			fixture.queueRunes([]rune("y"))
			fixture.update()
			require.Equal(t, characterCreationDiscardConfirmation, scene.state)
			assert.Zero(t, cancelled)
			held[ebiten.KeyEscape] = false
			fixture.update()
			require.Equal(t, characterCreationDiscardConfirmation, scene.state)
			// Avoid the same-millisecond repeat throttle without bypassing physical release checks.
			scene.keyboard.SetAllowKeyPressImmediately()
			held[choice] = true
			fixture.update()
			want := characterCreationFirstRound
			if choice == ebiten.KeyY {
				want = characterCreationCancelled
				assert.Equal(t, 1, cancelled, "Y discards immediately without Enter")
			} else {
				assert.Zero(t, cancelled)
			}
			require.Equal(t, want, scene.state)
			fixture.queueRunes([]rune("a"))
			fixture.update()
			held[choice] = false
			fixture.update()
			assert.Equal(t, want, scene.state, "held Escape must not reopen confirmation")
			assert.Zero(t, scene.questionNumber)
			assert.Same(t, questionnaire, scene.questionnaire)
			assert.Equal(t, calls, scene.source.Calls())
			assert.False(t, scene.hasResult)
			if choice == ebiten.KeyY {
				assert.Equal(t, 1, cancelled, "cancellation callback runs once")
			}
		})
	}
}
