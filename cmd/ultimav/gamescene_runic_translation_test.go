package main

import (
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func openTranslationTestSign(t *testing.T, scene *GameScene, update func()) *widgets.SignDialog {
	t.Helper()
	scene.ShowSignDialog(nil)
	for range 20 {
		update()
		if sign, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SignDialog); ok {
			update() // Release the opening transition before shortcut tests.
			return sign
		}
	}
	t.Fatal("sign did not open")
	return nil
}

func TestRunicTranslationProductionChordLifetimeAndNoGameplayMutation(t *testing.T) {
	for _, control := range []ebiten.Key{ebiten.KeyControlLeft, ebiten.KeyControlRight} {
		for _, ctrlFirst := range []bool{false, true} {
			name := control.String() + "/T-first"
			if ctrlFirst {
				name = control.String() + "/Ctrl-first"
			}
			t.Run(name, func(t *testing.T) {
				scene, held, advance, callbacks := newRunicReferenceTestScene(t, references.Britain)
				sign := openTranslationTestSign(t, scene, advance)
				// Chord frames share a clock: independently scheduled wind ticks
				// must not be attributed to this presentation-only shortcut.
				update := func() { require.NoError(t, scene.Update(nil)) }
				state := scene.gameState
				beforeDate := state.DateTime
				beforeFood := state.PartyState.Inventory.Provisions.Food.Get()
				beforeGold := state.PartyState.Inventory.Gold.Get()
				beforeKnowledge := state.KnowledgeSnapshot()
				beforeJournal := state.QuestJournalEvents()
				transcript := scene.output.GetOutputStr(true)
				calls := state.GetRNG().Calls()
				callbacks.Reset()
				held[control], held[ebiten.KeyR] = true, true
				update()
				require.True(t, scene.runicReference.open)
				// Ctrl stays physically down when moving directly from R to T.
				delete(held, ebiten.KeyR)
				held[ebiten.KeyT] = true
				require.False(t, scene.IsAnyKeyPressed(), "raw workflow polling captures chord before Update")
				update()
				require.True(t, sign.IsTranslated())
				for range 3 {
					update()
				}
				require.True(t, sign.IsTranslated(), "held chord toggles once")
				if ctrlFirst {
					delete(held, control)
				} else {
					delete(held, ebiten.KeyT)
				}
				update()
				require.False(t, sign.IsDismissed())
				clear(held)
				update()
				held[control], held[ebiten.KeyT] = true, true
				update()
				require.False(t, sign.IsTranslated())
				delete(held, ebiten.KeyT)
				update()
				held[ebiten.KeyT] = true
				update()
				require.True(t, sign.IsTranslated())
				delete(held, ebiten.KeyT)
				update()
				held[ebiten.KeyR] = true
				update()
				require.False(t, scene.runicReference.open)
				require.False(t, sign.IsTranslated())
				delete(held, ebiten.KeyR)
				update()
				held[ebiten.KeyR] = true
				update()
				require.True(t, scene.runicReference.open)
				require.False(t, sign.IsTranslated(), "reference reopening does not restore translation")
				require.Equal(t, transcript, scene.output.GetOutputStr(true))
				require.NotContains(t, transcript, "Ctrl+T")
				require.Equal(t, calls, state.GetRNG().Calls())
				require.Empty(t, callbacks.TimeAdvanced())
				require.Equal(t, beforeDate, state.DateTime)
				require.Equal(t, beforeFood, state.PartyState.Inventory.Provisions.Food.Get())
				require.Equal(t, beforeGold, state.PartyState.Inventory.Gold.Get())
				require.Equal(t, beforeKnowledge, state.KnowledgeSnapshot())
				require.Equal(t, beforeJournal, state.QuestJournalEvents())
				clear(held)
				update()
				held[ebiten.KeyT] = true
				update()
				require.True(t, sign.IsDismissed(), "plain T retains ordinary dismissal")
				clear(held)
				for range 20 {
					advance()
				}
				require.False(t, scene.runicReference.open)
				require.Nil(t, scene.runicReference.owner)
				require.False(t, openTranslationTestSign(t, scene, advance).IsTranslated())
			})
		}
	}
}

func TestRunicTranslationUnavailableObscuredAndLostOwner(t *testing.T) {
	scene, held, update, callbacks := newRunicReferenceTestScene(t, references.Britain)
	held[ebiten.KeyControlRight], held[ebiten.KeyT] = true, true
	update()
	delete(held, ebiten.KeyControlRight)
	update()
	require.Equal(t, PrimaryInput, scene.secondaryKeyState, "unsupported chord cannot become Talk")
	require.Empty(t, callbacks.TimeAdvanced())
	clear(held)
	update()
	sign := openTranslationTestSign(t, scene, update)
	held[ebiten.KeyControlRight], held[ebiten.KeyT] = true, true
	update()
	require.False(t, sign.IsTranslated(), "a sign alone is insufficient")
	require.False(t, sign.IsDismissed())
	clear(held)
	update()
	held[ebiten.KeyControlRight], held[ebiten.KeyR] = true, true
	update()
	clear(held)
	update()
	scene.showCommandHelp()
	help := scene.dialogStack.PeekTopModalDialog()
	held[ebiten.KeyControlRight], held[ebiten.KeyT] = true, true
	update()
	require.False(t, sign.IsTranslated(), "a background sign is not the current owner")
	require.Same(t, help, scene.dialogStack.PeekTopModalDialog())
	scene.dialogStack.MustRemoveWidgetInstance(help)
	update()
	require.False(t, sign.IsTranslated(), "eligibility gained while T is held cannot toggle")
	clear(held)
	update()
	held[ebiten.KeyControlRight], held[ebiten.KeyT] = true, true
	update()
	require.True(t, sign.IsTranslated())
	scene.dialogStack.MustRemoveWidgetInstance(sign)
	delete(held, ebiten.KeyControlRight)
	update()
	require.False(t, sign.IsTranslated())
	require.False(t, scene.runicReference.open)
	require.Equal(t, PrimaryInput, scene.secondaryKeyState)
	require.False(t, scene.IsAnyKeyPressed(), "lost eligibility still consumes captured T")
	clear(held)
	for range 20 {
		update()
	}
	require.False(t, scene.gameState.EventQueueManager.HasWorkflow("sign_dialog_workflow"))
	require.False(t, scene.inputBlocked)
}

func TestRunicTranslationProtectsRetainedRawAnyKeyWait(t *testing.T) {
	scene, held, update, _ := newRunicReferenceTestScene(t, references.Britain)
	scene.gameState.SystemCallbacks.Visual.IsAnyKeyPressed = scene.IsAnyKeyPressed
	continued := false
	scene.gameState.EventQueueManager.Workflow("translation_any_key_test").Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {}).WaitForAnyKey().Then(func() { continued = true }).Start()
	update()
	held[ebiten.KeyControlLeft], held[ebiten.KeyT] = true, true
	// The manager can poll raw key state before GameScene.Update owns the chord.
	for range 3 {
		scene.gameState.EventQueueManager.Update()
	}
	update()
	delete(held, ebiten.KeyControlLeft)
	for range 3 {
		scene.gameState.EventQueueManager.Update()
	}
	require.False(t, continued)
	clear(held)
	update()
	held[ebiten.KeySpace] = true
	for range 10 {
		update()
	}
	require.True(t, continued, "the next ordinary key must still release the wait")
}

func TestSignTranslationGeneratedWantedPosterPreservesNames(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Yew).
		WithPlayerAt(0x11, 0x16).WithSystemCallbacks().Build()
	names := []string{"George", "[Iolo]", "Shamino"}
	for index, name := range names {
		clear(state.PartyState.GetCharacter(index).Name[:])
		copy(state.PartyState.GetCharacter(index).Name[:], name)
	}
	require.True(t, state.ActionLookSmallMap(references.Up))
	require.NotNil(t, callbacks.SignDialogSign)
	result := text.NewSignRenderer(1).RenderTranslatedSign(callbacks.SignDialogSign.RawText)
	t.Cleanup(result.Image.Deallocate)
	var readable strings.Builder
	for _, row := range result.TranslationRows {
		for _, segment := range row {
			readable.WriteString(segment.Text)
		}
		readable.WriteByte('\n')
	}
	for _, name := range names {
		require.Contains(t, readable.String(), name)
	}
	require.Contains(t, readable.String(), "Wanted:")
	require.Contains(t, readable.String(), "Dead or Alive")
}

func TestRunicTranslationSceneDisposalRestoresExactOwner(t *testing.T) {
	scene, held, update, _ := newRunicReferenceTestScene(t, references.Britain)
	sign := openTranslationTestSign(t, scene, update)
	held[ebiten.KeyControlLeft], held[ebiten.KeyR] = true, true
	update()
	delete(held, ebiten.KeyR)
	held[ebiten.KeyT] = true
	update()
	require.True(t, sign.IsTranslated())
	scene.Dispose()
	require.False(t, sign.IsTranslated())
	require.False(t, scene.runicReference.open)
	require.Nil(t, scene.runicReference.owner)
}

func TestRunicTranslationFreshPressBypassesProductionRepeatInterval(t *testing.T) {
	for _, control := range []ebiten.Key{ebiten.KeyControlLeft, ebiten.KeyControlRight} {
		t.Run(control.String(), func(t *testing.T) {
			scene, held, advance, _ := newRunicReferenceTestScene(t, references.Britain)
			// Use the actual production repeat delay, with physical state injected
			// before constructing the sign so the dialog uses this keyboard too.
			scene.keyboard = input.NewKeyboardWithKeyStateSource(keyPressDelay, func(key ebiten.Key) bool { return held[key] })
			sign := openTranslationTestSign(t, scene, advance)
			beforeDate, beforeRNG := scene.gameState.DateTime, scene.gameState.GetRNG().Calls()
			update := func() { require.NoError(t, scene.Update(nil)) }
			held[control], held[ebiten.KeyR] = true, true
			update()
			require.True(t, scene.runicReference.open)
			delete(held, ebiten.KeyR)
			held[ebiten.KeyT] = true
			update()
			require.True(t, sign.IsTranslated())
			for range 3 {
				update()
			}
			require.True(t, sign.IsTranslated(), "a held chord toggles only once")
			delete(held, ebiten.KeyT)
			update() // Ctrl remains held; only T's physical release rearms the chord.
			// Keyboard has no injected clock. Pin its last-press timestamp into
			// the future to guarantee an unexpired repeat interval without sleeps.
			scene.keyboard.SetLastKeyPressedNowPlusMs(60_000)
			held[ebiten.KeyT] = true
			require.False(t, scene.keyboard.TryToRegisterKeyPress(ebiten.KeyT), "ordinary production repeat gating rejects this press")
			require.False(t, scene.IsAnyKeyPressed(), "raw polling must not consume the fresh chord")
			update()
			require.False(t, sign.IsTranslated(), "a genuine fresh T toggles immediately despite the repeat interval")
			for range 3 {
				update()
			}
			require.False(t, sign.IsTranslated())
			delete(held, control)
			update()
			require.False(t, sign.IsDismissed(), "Ctrl-first release cannot leak T into dismissal")
			delete(held, ebiten.KeyT)
			update()
			require.Equal(t, beforeDate, scene.gameState.DateTime)
			require.Equal(t, beforeRNG, scene.gameState.GetRNG().Calls())
			held[ebiten.KeyT] = true
			update()
			require.True(t, sign.IsDismissed(), "unmodified T retains ordinary dismissal after the captured press is released")
		})
	}
}

func TestRunicTranslationRetriesRegistrationAfterTransitionDelay(t *testing.T) {
	scene, held, advance, _ := newRunicReferenceTestScene(t, references.Britain)
	scene.keyboard = input.NewKeyboardWithKeyStateSource(keyPressDelay, func(key ebiten.Key) bool { return held[key] })
	sign := openTranslationTestSign(t, scene, advance)
	update := func() { require.NoError(t, scene.Update(nil)) }
	held[ebiten.KeyControlLeft], held[ebiten.KeyR] = true, true
	update()
	require.True(t, scene.runicReference.open)
	delete(held, ebiten.KeyR)
	update()
	scene.keyboard.SetForceWaitAnyKey(60_000)
	held[ebiten.KeyT] = true
	update()
	require.False(t, sign.IsTranslated(), "the shortcut must respect an explicit transition delay")
	require.False(t, scene.runicReference.translationHandled, "failed registration is not a completed toggle")
	require.False(t, sign.IsDismissed())
	scene.keyboard.SetForceWaitAnyKey(-1) // Expire the explicit barrier without sleeping.
	update()
	require.True(t, sign.IsTranslated(), "the same captured press may register once its barrier clears")
	for range 3 {
		update()
	}
	require.True(t, sign.IsTranslated(), "successful retry marks this physical press handled exactly once")
}
