package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestStatusHelpUsesVisibleRailIdentitiesAndSpendsNothing(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
		WithRandomSeed(42).WithSystemCallbacks().Build()
	state.MagicLightMinutesRemaining = 40
	state.CrownState.IsActive = true
	scene := newSurfaceCompletionTestScene(state)
	initializeDungeonUpdateTestScene(scene, state)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	before, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	view := scene.currentHelpStatus()
	require.Len(t, view.Worn, 1)
	require.Equal(t, "Crown", view.Worn[0].Name)
	require.NotNil(t, view.Worn[0].Icon)
	for _, effect := range view.Effects {
		require.NotEqual(t, "Crown", effect.Name, "the worn Crown must not be duplicated")
		require.NotNil(t, effect.Icon)
	}
	scene.showCommandHelp()
	dialog := scene.dialogStack.PeekTopModalDialog().(*widgets.CommandHelpDialog)
	for _, key := range []ebiten.Key{ebiten.KeyTab, ebiten.KeyPageDown, ebiten.KeyTab, ebiten.KeyEscape} {
		scene.keyboard.SimulateKeyPress(key)
		dialog.Update()
	}
	after, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.Equal(t, before.Random, after.Random)
	require.Equal(t, before.Party, after.Party)
	require.Equal(t, before.Effects, after.Effects)
	require.Empty(t, callbacks.TimeAdvanced())
	require.Zero(t, scene.dialogStack.Count())
	name, description := effectHelpDescription(game_state.PersistentEffectShadowlord)
	require.Equal(t, "Ominous influence", name)
	require.NotContains(t, description, "Faulinei")
}
