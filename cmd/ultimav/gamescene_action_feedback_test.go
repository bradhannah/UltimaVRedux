package main

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSolveTransientFeedbackTopsNewestFirstAndBottomClamped(t *testing.T) {
	available := image.Rect(10, 20, 500, 300)

	assert.Equal(t, []int{80}, solveTransientFeedbackTops(available, 100, 8, []int{40}))
	assert.Equal(t, []int{100, 148, 226},
		solveTransientFeedbackTops(available, 120, 8, []int{40, 70, 30}),
		"unequal cards retain complete newest-first gaps")
	assert.Equal(t, []int{84, 172, 250},
		solveTransientFeedbackTops(available, 260, 8, []int{80, 70, 50}),
		"bottom overflow shifts the complete group upward once")
	assert.Equal(t, []int{20}, solveTransientFeedbackTops(available, 260, 8, []int{400}),
		"an oversized singleton keeps its beginning visible")
}

func TestActionFeedbackMovementProtectsConsumedLifeAsOneUnion(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Britain)
	scene.ShowActionFeedback("First", false)
	first := scene.transientActionFeedback[0]
	state.ElapsedMs = 100
	scene.updateTransientActionFeedback()
	scene.updateNonBlockingDialogs()
	level := first.dialog.DissolveLevel()

	scene.ShowActionFeedback("Second", false)
	state.ElapsedMs = 190
	scene.ShowSearchFeedback("Third")
	require.Equal(t, int64(370), first.protectedUntil, "mid-slide retarget extends the same protected union")
	state.ElapsedMs = 370
	scene.updateTransientActionFeedback()
	scene.updateNonBlockingDialogs()
	assert.Equal(t, int64(100), first.consumedMs)
	assert.InDelta(t, level, first.dialog.DissolveLevel(), 0.000001, "reveal progress freezes while moving")

	state.ElapsedMs++
	scene.updateTransientActionFeedback()
	scene.updateNonBlockingDialogs()
	assert.Equal(t, int64(101), first.consumedMs)
	assert.Less(t, first.dialog.DissolveLevel(), level)
}

func TestActionFeedbackMovementProtectsReadableHold(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Britain)
	scene.ShowActionFeedback("First", false)
	first := scene.transientActionFeedback[0]
	state.ElapsedMs = 1000
	scene.updateTransientActionFeedback()
	scene.updateNonBlockingDialogs()
	require.Equal(t, transientFeedbackHolding, first.phase)

	scene.ShowActionFeedback("Second", false)
	state.ElapsedMs += transientFeedbackMoveDurationMs
	scene.updateTransientActionFeedback()
	assert.Equal(t, int64(1000), first.consumedMs)
	assert.Equal(t, transientFeedbackHolding, first.phase)

	state.ElapsedMs++
	scene.updateTransientActionFeedback()
	assert.Equal(t, int64(1001), first.consumedMs)
}

func TestActionFeedbackMovementFreezesPartialFadeOut(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Britain)
	scene.ShowActionFeedback("Nearly gone", false)
	first := scene.transientActionFeedback[0]
	state.ElapsedMs = 2100
	scene.updateTransientActionFeedback()
	scene.updateNonBlockingDialogs()
	require.Equal(t, transientFeedbackFadingOut, first.phase)
	level := first.dialog.DissolveLevel()
	require.Greater(t, level, 0.0)

	scene.ShowActionFeedback("New arrival", false)
	state.ElapsedMs = 2280
	scene.updateTransientActionFeedback()
	scene.updateNonBlockingDialogs()
	assert.Equal(t, int64(2100), first.consumedMs)
	assert.InDelta(t, level, first.dialog.DissolveLevel(), 0.000001)
	assert.True(t, first.dialog.IsVisible(), "a moving half-dissolved card cannot disappear")

	state.ElapsedMs = 2479
	scene.updateTransientActionFeedback()
	assert.Same(t, first, scene.transientActionFeedback[0], "the fade resumes from its remaining budget")
	assert.Equal(t, int64(2299), first.consumedMs)

	state.ElapsedMs = 2480
	scene.updateTransientActionFeedback()
	assert.False(t, scene.dialogStack.ContainsWidgetInstance(first.dialog))
	assert.NotContains(t, scene.transientActionFeedback, first, "the preserved final millisecond expires exactly once")
}

func TestActionFeedbackStationaryUnobscuredReflowGetsNoMovementCredit(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Deceit)
	scene.ShowActionFeedback("same height", false)
	first := scene.transientActionFeedback[0]
	scene.ShowActionFeedback("same height", false)
	state.ElapsedMs = transientFeedbackMoveDurationMs
	scene.updateTransientActionFeedback()
	settledY := first.y
	consumed := first.consumedMs

	state.ElapsedMs = 500
	scene.updateTransientActionFeedback()
	consumed = first.consumedMs
	scene.layoutTransientActionFeedback(state.ElapsedMs, scene.transientActionFeedbackLayout)
	assert.Empty(t, first.motion, "an unchanged settled target does not restart interpolation")
	assert.Equal(t, settledY, first.y)
	assert.LessOrEqual(t, first.protectedUntil, state.ElapsedMs)

	state.ElapsedMs += transientFeedbackMoveDurationMs
	scene.updateTransientActionFeedback()
	assert.Equal(t, consumed+transientFeedbackMoveDurationMs, first.consumedMs)
}

func TestActionFeedbackRepeatedTimestampIsIdempotent(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Britain)
	scene.ShowActionFeedback("older", false)
	scene.ShowActionFeedback("newer", false)
	state.ElapsedMs = 90
	scene.updateTransientActionFeedback()
	feedback := scene.transientActionFeedback[0]
	consumed, motion, protectedUntil := feedback.consumedMs, feedback.motion, feedback.protectedUntil
	x, y := feedback.x, feedback.y

	scene.updateTransientActionFeedback()
	assert.Equal(t, consumed, feedback.consumedMs)
	assert.Equal(t, motion, feedback.motion)
	assert.Equal(t, protectedUntil, feedback.protectedUntil)
	assert.Equal(t, x, feedback.x)
	assert.Equal(t, y, feedback.y)
}

func TestActionFeedbackCapacityEvictsOldestExactInstance(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Britain)
	dialogs := make([]*widgets.MessageDialog, 0, 4)
	scene.ShowActionFeedback("one", false)
	first := scene.transientActionFeedback[0]
	dialogs = append(dialogs, first.dialog)
	state.ElapsedMs = 2100
	scene.updateTransientActionFeedback()
	scene.updateNonBlockingDialogs()
	require.Equal(t, transientFeedbackFadingOut, first.phase)

	for _, message := range []string{"two", "three"} {
		scene.ShowActionFeedback(message, false)
		dialogs = append(dialogs, scene.transientActionFeedback[len(scene.transientActionFeedback)-1].dialog)
	}
	require.Greater(t, first.protectedUntil, state.ElapsedMs, "the oldest card is both fading and movement-protected")
	scene.ShowActionFeedback("four", false)
	dialogs = append(dialogs, scene.transientActionFeedback[len(scene.transientActionFeedback)-1].dialog)

	require.Len(t, scene.transientActionFeedback, 3)
	assert.Equal(t, 3, scene.dialogStack.Count())
	assert.False(t, scene.dialogStack.ContainsWidgetInstance(dialogs[0]))
	for index := 1; index < len(dialogs); index++ {
		assert.True(t, scene.dialogStack.ContainsWidgetInstance(dialogs[index]))
	}
}

func TestActionFeedbackOversizedNewestSingletonClipsAndExpires(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Britain)
	scene.ShowSearchFeedback(strings.Repeat("A complete synthetic row that must wrap.\n", 80))
	require.Len(t, scene.transientActionFeedback, 1)
	feedback := scene.transientActionFeedback[0]
	context := scene.transientActionFeedbackLayout
	require.Greater(t, feedback.cardHeight, context.available.Dy())
	assert.Equal(t, float64(context.available.Min.Y), feedback.y)
	feedback.dialog.ShowWithoutAnimation()
	rendered := renderedSpellFeedbackBounds(t, feedback.dialog, context.screen.Max)
	assert.Equal(t, context.available.Min.Y, rendered.Min.Y)
	assert.LessOrEqual(t, rendered.Max.Y, context.available.Max.Y)
	assert.Greater(t, feedback.dialog.PresentationBounds().Max.Y, context.available.Max.Y)

	state.ElapsedMs = 10_000
	scene.updateTransientActionFeedback()
	assert.Empty(t, scene.transientActionFeedback, "static clipping does not protect lifetime")
	assert.Zero(t, scene.dialogStack.Count())
}

func TestActionFeedbackHeightOverflowEvictsOlderBeforeOversizedNewest(t *testing.T) {
	scene, _ := newTransientFeedbackTestScene(t, references.Britain)
	scene.ShowActionFeedback("older", false)
	old := scene.transientActionFeedback[0].dialog
	scene.ShowSearchFeedback(strings.Repeat("oversized newest row\n", 80))

	require.Len(t, scene.transientActionFeedback, 1)
	assert.NotSame(t, old, scene.transientActionFeedback[0].dialog)
	assert.False(t, scene.dialogStack.ContainsWidgetInstance(old))
	assert.Greater(t, scene.transientActionFeedback[0].cardHeight, scene.transientActionFeedbackLayout.available.Dy())
}

func TestActionFeedbackExactExpiryWinsSameTimeAdmission(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Britain)
	scene.ShowActionFeedback("old", false)
	old := scene.transientActionFeedback[0].dialog
	state.ElapsedMs = transientFeedbackLifetimeMs
	scene.ShowActionFeedback("new", false)

	require.Len(t, scene.transientActionFeedback, 1)
	assert.NotSame(t, old, scene.transientActionFeedback[0].dialog)
	assert.False(t, scene.dialogStack.ContainsWidgetInstance(old))
	assert.Equal(t, int64(0), scene.transientActionFeedback[0].consumedMs)
	assert.Equal(t, 1, scene.dialogStack.Count())
}

func TestActionFeedbackLargeJumpMatchesBoundaryUpdates(t *testing.T) {
	large, largeState := newTransientFeedbackTestScene(t, references.Britain)
	stepped, steppedState := newTransientFeedbackTestScene(t, references.Britain)
	for _, fixture := range []struct {
		scene *GameScene
		state *game_state.GameState
	}{{large, largeState}, {stepped, steppedState}} {
		fixture.scene.ShowActionFeedback("older", false)
		fixture.state.ElapsedMs = 100
		fixture.scene.ShowActionFeedback("newer", false)
	}

	largeState.ElapsedMs = 2400
	large.updateTransientActionFeedback()
	for _, at := range []int64{280, 400, 2000, 2300, 2400} {
		steppedState.ElapsedMs = at
		stepped.updateTransientActionFeedback()
	}

	require.Len(t, large.transientActionFeedback, 1)
	require.Len(t, stepped.transientActionFeedback, 1)
	a, b := large.transientActionFeedback[0], stepped.transientActionFeedback[0]
	assert.Equal(t, a.consumedMs, b.consumedMs)
	assert.Equal(t, a.phase, b.phase)
	assert.Equal(t, a.protectedUntil, b.protectedUntil)
	assert.Equal(t, a.motion, b.motion)
	assert.InDelta(t, a.x, b.x, 0.000001)
	assert.InDelta(t, a.y, b.y, 0.000001)
}

func TestActionFeedbackExternalRemovalAndSceneCleanupAreIdempotent(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Britain)
	modal := &countingDialog{}
	scene.dialogStack.PushModalDialog(modal)
	scene.ShowActionFeedback("one", false)
	scene.ShowActionFeedback("two", false)
	removed := scene.transientActionFeedback[0].dialog
	require.True(t, scene.dialogStack.RemoveWidgetInstance(removed))
	scene.updateTransientActionFeedback()
	require.Len(t, scene.transientActionFeedback, 1)
	assert.Same(t, modal, scene.dialogStack.PeekTopModalDialog())

	scene.PrepareForSceneReplacement()
	scene.PrepareForSceneReplacement()
	assert.Empty(t, scene.transientActionFeedback)
	assert.Equal(t, 1, scene.dialogStack.Count())
	assert.Same(t, modal, scene.dialogStack.PeekTopModalDialog())
	state.ElapsedMs++
}

func TestActionFeedbackModeReflowPreservesLifeAndMovesToDungeonAnchor(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Britain)
	scene.ShowActionFeedback("Reflow", false)
	feedback := scene.transientActionFeedback[0]
	surfaceY := feedback.y
	state.ElapsedMs = 500
	scene.updateTransientActionFeedback()
	consumed := feedback.consumedMs

	state.MapState.PlayerLocation.Location = references.Deceit
	scene.updateTransientActionFeedback()
	assert.Equal(t, consumed, feedback.consumedMs)
	assert.True(t, feedback.motion.activeAt(state.ElapsedMs))
	assert.Greater(t, feedback.motion.toY, surfaceY)
	assert.Equal(t, transientFeedbackDungeon, scene.transientActionFeedbackLayout.mode)
	state.ElapsedMs += transientFeedbackMoveDurationMs
	scene.updateTransientActionFeedback()
	assert.True(t, feedback.dialog.PresentationBounds().In(scene.transientActionFeedbackLayout.available))
}

func TestActionFeedbackActiveResizeReflowsWithoutRefreshingLife(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Britain)
	scene.ShowActionFeedback("Resize in place", false)
	feedback := scene.transientActionFeedback[0]
	state.ElapsedMs = 500
	scene.updateTransientActionFeedback()
	consumed := feedback.consumedMs

	display.GetManager().PublishLogicalSize(1920, 1080)
	scene.updateTransientActionFeedback()
	assert.Equal(t, consumed, feedback.consumedMs)
	assert.Equal(t, image.Pt(1920, 1080), scene.transientActionFeedbackLayout.screen.Max)
	assert.True(t, feedback.motion.activeAt(state.ElapsedMs))
	assert.True(t, feedback.dialog.PresentationBounds().In(scene.transientActionFeedbackLayout.available),
		"abrupt resize rebases the current fitting card into valid bounds")

	state.ElapsedMs += transientFeedbackMoveDurationMs
	scene.updateTransientActionFeedback()
	assert.Equal(t, consumed, feedback.consumedMs, "resize motion is protected without refreshing prior life")
}

func TestActionFeedbackResponsiveSurfaceAndDungeonSafeBounds(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	for _, size := range []image.Point{{1280, 720}, {1365, 768}, {1920, 1080}, {2560, 1440}, {3840, 2160}} {
		for _, location := range []references.Location{references.Britain, references.Deceit} {
			name := fmt.Sprintf("%dx%d/location-%d", size.X, size.Y, location)
			t.Run(name, func(t *testing.T) {
				manager.PublishLogicalSize(size.X, size.Y)
				state := &game_state.GameState{}
				state.MapState.PlayerLocation.Location = location
				scene := &GameScene{gameState: state}
				scene.ShowActionFeedback("one", false)
				first := scene.transientActionFeedback[0]
				preferred := transientFeedbackNormalCenter
				if location.GetMapType() == references.DungeonMapType {
					preferred = transientFeedbackDungeonCenter
				}
				assert.InDelta(t, preferred*float64(size.Y),
					float64(first.dialog.PresentationBounds().Min.Y+first.dialog.PresentationBounds().Max.Y)/2, 1.5)

				scene.ShowSearchFeedback("two wraps across a representative complete line")
				scene.ShowActionFeedback("three", false)
				state.ElapsedMs = transientFeedbackMoveDurationMs
				scene.updateTransientActionFeedback()
				context := scene.transientActionFeedbackLayout
				require.Len(t, scene.transientActionFeedback, 3)
				for _, feedback := range scene.transientActionFeedback {
					assert.True(t, feedback.dialog.PresentationBounds().In(context.available))
				}
				newest := scene.transientActionFeedback[2].dialog.PresentationBounds()
				middle := scene.transientActionFeedback[1].dialog.PresentationBounds()
				oldest := scene.transientActionFeedback[0].dialog.PresentationBounds()
				assert.LessOrEqual(t, newest.Max.Y+context.gap, middle.Min.Y)
				assert.LessOrEqual(t, middle.Max.Y+context.gap, oldest.Min.Y)
			})
		}
	}
}

func TestActionFeedbackOutputMirroringRemainsExactlyOnce(t *testing.T) {
	scene, _ := newTransientFeedbackTestScene(t, references.Britain)
	_, scene.output = newMainOutput(1280)
	scene.ShowActionFeedback("recorded", true)
	scene.ShowActionFeedback("popup only", false)
	scene.ShowSearchFeedback("search already recorded elsewhere")

	output := scene.output.GetOutputStr(false)
	assert.Equal(t, 1, strings.Count(output, "recorded"))
	assert.NotContains(t, output, "popup only")
	assert.NotContains(t, output, "search already recorded elsewhere")
}

func TestActionFeedbackDisposeClearsShutdownFinalizerAdmission(t *testing.T) {
	for limit := range 4 {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithSystemCallbacks().
				Build()
			scene := newProductionCastTestScene(state)
			setActionPopupLimitForTest(scene, limit)
			scene.ShowActionFeedback("existing", false)
			state.EventQueueManager.Workflow("feedback_on_shutdown").
				Priority(event_queue.PriorityNormal).
				Sequential().
				First(func() {}).
				Wait(1000).
				Finally(func() { scene.ShowActionFeedback("from finalizer", false) }).
				Start()

			assert.NotPanics(t, scene.Dispose)
			assert.NotPanics(t, scene.Dispose)
			assert.Empty(t, scene.transientActionFeedback)
			assert.Zero(t, scene.dialogStack.Count())
			assert.Empty(t, scene.combatChestFeedback)
			assert.False(t, scene.transientActionFeedbackLayoutSet)
		})
	}
}

func newTransientFeedbackTestScene(t *testing.T, location references.Location) (*GameScene, *game_state.GameState) {
	t.Helper()
	retirePresentationFixtureCaches(t)
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(1280, 720)
	state := &game_state.GameState{}
	state.MapState.PlayerLocation.Location = location
	scene := &GameScene{gameState: state}
	t.Cleanup(scene.clearTransientActionFeedback)

	return scene, state
}
