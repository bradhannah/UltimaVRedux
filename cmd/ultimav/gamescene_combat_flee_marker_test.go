package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestCombatFleeChevronUsesWholeSourcePixelBlocks(t *testing.T) {
	screen := ebiten.NewImage(96, 96)
	t.Cleanup(screen.Deallocate)
	options := &ebiten.DrawImageOptions{}
	options.GeoM.Scale(3, 3)
	options.GeoM.Translate(12, 12)
	drawCombatFleeChevrons(screen, options)
	for y := 0; y < 72; y += 3 {
		for x := 0; x < 84; x += 3 {
			want := screen.At(x, y)
			for dy := range 3 {
				for dx := range 3 {
					require.Equal(t, want, screen.At(x+dx, y+dy), "source block at (%d,%d) contains a fine diagonal", x, y)
				}
			}
		}
	}
	require.Positive(t, opaquePixelCount(screen))
}

func TestCombatFleeChevronPreservesActorTransformOptionsAndAlpha(t *testing.T) {
	for _, scale := range [][2]float64{{3, 3}, {3.2, 720.0 / 208}, {4.8, 1080.0 / 208}} {
		screen, expected := ebiten.NewImage(128, 128), ebiten.NewImage(128, 128)
		sourceFrame := ebiten.NewImage(18, 18)
		t.Cleanup(screen.Deallocate)
		t.Cleanup(expected.Deallocate)
		t.Cleanup(sourceFrame.Deallocate)
		// A full actor-sized source frame is the registration oracle, including
		// both tips on its padded edges. No separately rounded screen geometry.
		for y, x := range []int{3, 2, 1, 0, 1, 2, 3} {
			sourceFrame.Set(x, y+5, ucolor.EGAYellow)
			sourceFrame.Set(17-x, y+5, ucolor.EGAYellow)
		}
		options := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		options.GeoM.Scale(scale[0], scale[1])
		options.GeoM.Translate(13.35, 11.65)
		options.ColorScale.ScaleAlpha(0.5)
		before := *options
		drawCombatFleeChevrons(screen, options)
		require.Equal(t, before, *options)
		referenceOptions := before
		referenceOptions.Filter = ebiten.FilterNearest
		expected.DrawImage(sourceFrame, &referenceOptions)
		require.Equal(t, signaturePixels(expected), signaturePixels(screen))
	}
	require.Same(t, combatFleeGlyph(), combatFleeGlyph(), "immutable glyph is reused")
}

func TestCombatFleeChevronSignatureBoundsAndState(t *testing.T) {
	scene, screen := newCombatConcealmentTestScene(t)
	index, monster := signatureMonster(t, scene, references.MonsterDevil, references.Position{X: 5, Y: 5})
	monster.SetCurrentHP(monster.MaxHP() - 1) // The battlefield marks only the injured.
	monster.SetFleeing(true)
	scene.recordCombatEvent(game_state.CombatEvent{Kind: game_state.CombatEventTeleport,
		From: monster.Position(), To: monster.Position(),
		Attacker: game_state.CombatFeedbackEntity{Index: index, Position: monster.Position()}})
	effect := scene.combatSignatureEffects[0]
	frame := scene.composeCombatSignatureFrame(effect)
	for y, x := range []int{3, 2, 1, 0, 1, 2, 3} {
		require.Equal(t, ucolor.From(ucolor.EGAYellow).RGBA(), frame.At(x, y+5))
		require.Equal(t, ucolor.From(ucolor.EGAYellow).RGBA(), frame.At(17-x, y+5))
	}
	assertSignatureHP(t, frame, monster.CurrentHP(), monster.MaxHP())
	// Status removal is observed next draw; no cached fleeing state.
	monster.SetFleeing(false)
	screen.Clear()
	scene.drawCombatMonsterStatusMarks(screen, &ebiten.DrawImageOptions{}, monster)
	require.Zero(t, opaquePixelCount(screen))
	monster.SetFleeing(true)
	scene.gameState.ElapsedMs = effect.StartedMs + combatTeleportDepartureMs
	scene.drawCombatSignatureEffects(screen)
	require.Zero(t, opaquePixelCount(screen), "blink concealment includes chevrons")
	monster.SetInvisible(true)
	scene.gameState.ElapsedMs = effect.StartedMs + combatTeleportDepartureMs + combatTeleportHiddenMs + 1
	scene.drawCombatSignatureEffects(screen)
	require.Zero(t, opaquePixelCount(screen), "invisible actors do not disclose a fleeing cue")
	scene.updateCombatSignatureEffects()
	require.Empty(t, scene.combatSignatureEffects)
}

func TestCombatFleeChevronSharesGateMask(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	index, monster := signatureMonster(t, scene, references.MonsterDevil, references.Position{X: 5, Y: 5})
	monster.SetFleeing(true)
	scene.recordCombatEvent(signatureGateEvent(index, monster))
	effect := scene.combatSignatureEffects[0]
	effect.Dissolve.UpdateProgress(0)
	frame := scene.composeCombatSignatureFrame(effect)
	require.NotEqual(t, ucolor.From(ucolor.EGAYellow).RGBA(), frame.At(0, 8), "unrevealed tip must be masked")
	effect.Dissolve.UpdateProgress(1)
	frame = scene.composeCombatSignatureFrame(effect)
	require.Equal(t, ucolor.From(ucolor.EGAYellow).RGBA(), frame.At(0, 8))
	require.Equal(t, ucolor.From(ucolor.EGAYellow).RGBA(), frame.At(17, 8))
}
