package main

import (
	"fmt"
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func TestCombatSearchFeedbackRenderedPosition(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })

	for _, size := range []image.Point{{X: 1280, Y: 720}, {X: 1365, Y: 768}, {X: 1920, Y: 1080}, {X: 2560, Y: 1440}, {X: 3840, Y: 2160}} {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			manager.PublishLogicalSize(size.X, size.Y)
			state, mock := newBritainCombatTestState(t, 42)
			scene := newProductionCastTestScene(state)
			previous := manager.SetMapPopupViewportProvider(scene.mapPopupViewport)
			defer manager.SetMapPopupViewportProvider(previous)
			state.SystemCallbacks.UI.ShowActionFeedback = scene.ShowActionFeedback
			activePosition := state.CombatScene.GetPartyMemberPosition(state.CombatScene.GetActiveCombatantIndex())
			target := references.Down.GetNewPositionInDirection(&activePosition)
			require.NotNil(t, target)
			state.CombatScene.AddObject(game_state.NewCombatCorpse(*target))
			// Isolate the corpse outcome from combat initialization's RNG consumption.
			state.SetRandomSeed(42)
			mock.Reset()

			require.True(t, state.ActionSearchCombatMap(references.Down))
			const message = "Thou dost find nothing!"
			require.Contains(t, mock.Messages, message)
			require.Len(t, scene.transientActionFeedback, 1)
			action := scene.transientActionFeedback[0].dialog
			assert.Equal(t, 1, scene.dialogStack.Count())
			assert.Nil(t, scene.dialogStack.PeekTopModalDialog(), "Search feedback must not own input")
			assert.NotContains(t, scene.output.GetOutputStr(false), message, "Search already publishes its narration")

			// Resolve the cosmetic fade without sleeping; scene expiry still uses ElapsedMs.
			action.ShowWithoutAnimation()
			scene.ShowSearchFeedback(message)
			require.Len(t, scene.transientActionFeedback, 2)
			search := scene.transientActionFeedback[1].dialog
			search.ShowWithoutAnimation()
			state.ElapsedMs += transientFeedbackMoveDurationMs
			scene.updateTransientActionFeedback()
			searchBounds, actionBounds := search.PresentationBounds(), action.PresentationBounds()
			if size.X == 1280 || size.X == 1365 {
				searchBounds = renderedSpellFeedbackBounds(t, search, size)
				actionBounds = renderedSpellFeedbackBounds(t, action, size)
			}
			battlefield := combatLayout{}.battlefieldRectangle(image.Rectangle{Max: size})
			for _, bounds := range []image.Rectangle{actionBounds, searchBounds} {
				assert.GreaterOrEqual(t, bounds.Min.X, battlefield.Min.X-1)
				assert.LessOrEqual(t, bounds.Max.X, battlefield.Max.X+1)
				assert.InDelta(t, float64(battlefield.Min.X+battlefield.Max.X)/2,
					float64(bounds.Min.X+bounds.Max.X)/2, 1.5)
			}
			assert.Less(t, searchBounds.Max.Y, actionBounds.Min.Y, "newest Search card is fully above the older action card")
			assert.InDelta(t, widgets.PositionMapCenterLower.GetVerticalPercent()*float64(size.Y),
				float64(searchBounds.Min.Y+searchBounds.Max.Y)/2, 1.5)

			other := &countingDialog{}
			scene.dialogStack.PushModalDialog(other)
			state.ElapsedMs = 10_000
			scene.updateTransientActionFeedback()
			assert.Len(t, scene.transientActionFeedback, 2, "later modal preserves obscured reading time")
			assert.Equal(t, 3, scene.dialogStack.Count())
			assert.Same(t, other, scene.dialogStack.PeekTopModalDialog())
			scene.dialogStack.MustRemoveWidgetInstance(other)
			state.ElapsedMs += 10_000
			scene.updateTransientActionFeedback()
			assert.Empty(t, scene.transientActionFeedback)
			assert.Zero(t, scene.dialogStack.Count())
		})
	}
}

func TestCombatTimedCastFailureRenderedPosition(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	state, _ := newProductionCombatCastState(t)
	scene := newProductionCastTestScene(state)
	previous := manager.SetMapPopupViewportProvider(scene.mapPopupViewport)
	defer manager.SetMapPopupViewportProvider(previous)
	for _, size := range []image.Point{{X: 1280, Y: 720}, {X: 1920, Y: 1080}, {X: 2560, Y: 1440}} {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			manager.PublishLogicalSize(size.X, size.Y)
			const message = "No one in the party has magical ability!"
			scene.showTemporaryCastFailure(message)
			overlay, ok := scene.timedOverlayDialog.(*widgets.OverlayDialog)
			require.True(t, ok)
			assert.False(t, overlay.IsUserDismissed())
			assert.True(t, overlay.IsDurationExternallyManaged())
			assert.Equal(t, message, overlay.GetMessage())
			bounds := spellFeedbackPresentationBounds(overlay)
			if size.X == 1280 {
				bounds = renderedSpellFeedbackBounds(t, overlay, size)
			}
			battlefield := combatLayout{}.battlefieldRectangle(image.Rectangle{Max: size})
			assert.GreaterOrEqual(t, bounds.Min.X, battlefield.Min.X-1)
			assert.LessOrEqual(t, bounds.Max.X, battlefield.Max.X+1)
			assert.InDelta(t, float64(battlefield.Min.X+battlefield.Max.X)/2,
				float64(bounds.Min.X+bounds.Max.X)/2, 1.5)
			assert.InDelta(t, widgets.PositionMapCenterLower.GetVerticalPercent()*float64(size.Y),
				float64(bounds.Min.Y+bounds.Max.Y)/2, 1.5)
			state.ElapsedMs += castFailureDialogDurationMs - 1
			scene.updateTimedOverlay()
			assert.Same(t, overlay, scene.timedOverlayDialog)
			other := &countingDialog{}
			scene.dialogStack.PushModalDialog(other)
			state.ElapsedMs++
			scene.updateTimedOverlay()
			assert.Nil(t, scene.timedOverlayDialog)
			assert.False(t, overlay.IsVisible())
			assert.Equal(t, 1, scene.dialogStack.Count())
			assert.Same(t, other, scene.dialogStack.PeekTopModalDialog())
			scene.dialogStack.MustRemoveWidgetInstance(other)
		})
	}
}
