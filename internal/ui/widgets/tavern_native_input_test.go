package widgets

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

// Only native event delivery is simulated. Text/byte selection use the pinned
// Field, and both editors use their production constructor body and Update flow.
// Ending a session deliberately does NOT repair the raw character source.
type tavernNativeTestField struct {
	scriptedNativeTextField
	rawStranded *bool
	bounds      image.Rectangle
	pumps       int
	inputError  error
	onBlur      func()
}

func (f *tavernNativeTestField) HandleInputWithBounds(bounds image.Rectangle) (bool, error) {
	if !f.IsFocused() {
		return false, nil
	}
	f.bounds = bounds
	f.pumps++
	*f.rawStranded = true
	if f.inputError != nil {
		return false, f.inputError
	}

	return f.scriptedNativeTextField.HandleInputWithBounds(bounds)
}

func (f *tavernNativeTestField) Blur() {
	if f.onBlur != nil {
		flush := f.onBlur
		f.onBlur = nil
		flush()
	}
	f.preedit = ""
	f.nextInput = nil
	f.Field.Blur()
}

func TestTavernNativeSubjectExternalSuspensionPreservesDraftAndReleaseGate(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	for _, key := range []ebiten.Key{
		ebiten.KeyMax, ebiten.KeyEnter, ebiten.KeyEscape, ebiten.KeyBackspace, ebiten.KeyTab, ebiten.KeyPageDown,
	} {
		name := key.String()
		if key == ebiten.KeyMax {
			name = "no held control"
		}
		t.Run(name, func(t *testing.T) {
			f := newTavernNativeTest(t)
			f.openTavern()
			f.press(ebiten.KeyH)
			f.typeText("é世界")
			f.field.nextInput = func() { f.field.Field.SetTextAndSelection("é世界", len("é"), len("é世")) }
			f.tick()
			f.field.nextInput = func() { f.field.preedit = "候" }
			f.tick()
			state := f.dialog.GetState()
			pumps := f.field.pumps
			f.field.onBlur = func() { f.field.commit(strings.Repeat("x", 40) + "\nlate") }
			if key != ebiten.KeyMax {
				f.held[key] = true
			}
			restorePolling := f.keyboard.SuspendInputPolling()
			f.dialog.SuspendTextInput() // The scene does NOT call Tavern.Update while suspended.
			f.dialog.SuspendTextInput()
			f.dialog.ShowWithoutAnimation() // Masked polling cannot reopen/rearm the native field.
			restorePolling()
			require.True(t, f.dialog.modalFocused)
			require.Same(t, f.dialog, f.stack.PeekTopModalDialog())
			require.False(t, f.field.IsFocused())
			require.False(t, f.dialog.inputKeysReady)
			require.Equal(t, pumps, f.field.pumps)
			require.Equal(t, "é世界", f.dialog.GetTopicInput(), "Blur's pending commit must not alter the saved draft")
			start, end := f.field.Selection()
			require.Equal(t, len("é"), start)
			require.Equal(t, len("é世"), end)
			f.tick()
			if key != ebiten.KeyMax {
				require.False(t, f.field.IsFocused(), "the returning held control still owns its release gate")
				require.Equal(t, pumps, f.field.pumps)
				f.held[key] = false
				f.tick()
			}
			require.True(t, f.field.IsFocused())
			require.False(t, f.dialog.HasPendingTopicIntent())
			require.False(t, f.dialog.HasPendingActionIntent())
			require.Equal(t, state, f.dialog.GetState())
			f.typeText("à")
			require.Equal(t, "éà界", f.dialog.GetTopicInput(), "fresh native input replaces the retained byte selection")
			f.press(ebiten.KeyEnter)
			topic, ok := f.dialog.ConsumePendingTopicIntent()
			require.True(t, ok)
			require.Equal(t, "éà界", topic.Input)
		})
	}
}

func TestTavernNativeSubjectBlurCannotLeakIntoSubmitOrCancel(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	for _, submit := range []bool{false, true} {
		t.Run(fmt.Sprintf("submitted=%t", submit), func(t *testing.T) {
			f := newTavernNativeTest(t)
			f.openTavern()
			f.press(ebiten.KeyH)
			f.typeText("honesty")
			f.field.onBlur = func() { f.field.commit("late") }
			key := ebiten.KeyEscape
			if submit {
				key = ebiten.KeyEnter
			}
			f.press(key)
			require.False(t, f.field.IsFocused())
			if submit {
				topic, ok := f.dialog.ConsumePendingTopicIntent()
				require.True(t, ok)
				require.Equal(t, "honesty", topic.Input)
				require.Equal(t, topic.Input, f.dialog.GetTopicInput())
			} else {
				require.False(t, f.dialog.HasPendingTopicIntent())
				require.Empty(t, f.dialog.GetTopicInput())
			}
		})
	}
}

type tavernNativeTest struct {
	dialog        *TavernShopDialog
	field         *tavernNativeTestField
	keyboard      *input.Keyboard
	held          map[ebiten.Key]bool
	stack         DialogStack
	windowFocused bool
	rawStranded   bool
	elapsed       int64
}

func newTavernNativeTest(t *testing.T) *tavernNativeTest {
	t.Helper()
	f := &tavernNativeTest{held: map[ebiten.Key]bool{}, windowFocused: true}
	f.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return f.held[key] })
	f.keyboard.SetElapsedMillisecondsSource(func() int64 { return f.elapsed })
	t.Cleanup(func() {
		if f.dialog != nil {
			f.dialog.Dismiss()
		}
	})

	return f
}

func (f *tavernNativeTest) openTavern() {
	state := testTavernState(references.TavernMenuRumBoar)
	state.CustomerEligible = true
	config := testTavernShopConfig(f.keyboard, state, nil)
	config.AppendInputChars = nil // Production native path, never a raw fallback.
	config.IsKeyPressed = f.keyboard.IsKeyHeld
	f.field = &tavernNativeTestField{rawStranded: &f.rawStranded}
	f.dialog = newTavernShopDialog(config, f.field)
	f.dialog.isWindowFocused = func() bool { return f.windowFocused }
	f.stack.PushModalDialog(f.dialog)
	f.tick()
}

func (f *tavernNativeTest) tick() {
	f.elapsed += 16
	f.keyboard.ClearFrameConsumption()
	if top := f.stack.PeekTopModalDialog(); top != nil {
		top.Update()
	}
}

func (f *tavernNativeTest) press(key ebiten.Key) {
	f.held[key] = true
	f.tick()
	f.held[key] = false
	f.tick()
}

func (f *tavernNativeTest) typeText(value string) {
	f.field.nextInput = func() { f.field.commit(value) }
	f.tick()
}

func TestTavernNativeSubjectProductionConstructorSelectsNativeField(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	config := testTavernShopConfig(newWidgetTestKeyboard(0), testTavernState(references.TavernMenuRumBoar), nil)
	config.AppendInputChars = nil
	dialog := NewTavernShopDialog(config)
	t.Cleanup(dialog.Dismiss)
	require.NotNil(t, dialog.topicField)
	require.Nil(t, dialog.appendInputChars)
	require.False(t, dialog.topicField.IsFocused(), "construction does not claim native focus")
}

func TestTavernNativeSubjectAfterSaveDescriptionHandoff(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	for _, submit := range []bool{false, true} {
		t.Run(fmt.Sprintf("saveSubmitted=%t", submit), func(t *testing.T) {
			f := newTavernNativeTest(t)
			saveField := &tavernNativeTestField{rawStranded: &f.rawStranded}
			var result SaveDescriptionEntryResult
			save := newSaveDescriptionEntryModal(SaveDescriptionEntryModalConfig{
				Keyboard: f.keyboard, IsKeyPressed: f.keyboard.IsKeyHeld, MaxRunes: 80,
				OnResult: func(value string, submitted bool) {
					result = SaveDescriptionEntryResult{Text: value, Submitted: submitted}
				},
			}, saveField).AutoCloseAndPop(&f.stack)
			save.isWindowFocused = func() bool { return true }
			f.stack.PushModalDialog(save)
			f.tick() // Actually start the native session, rather than only calling Focus.
			saveField.nextInput = func() { saveField.commit("Before Yew") }
			f.tick()
			key := ebiten.KeyEscape
			if submit {
				key = ebiten.KeyEnter
			}
			f.press(key)
			require.Equal(t, SaveDescriptionEntryResult{Text: "Before Yew", Submitted: submit}, result)
			require.False(t, saveField.IsFocused())
			require.Zero(t, f.stack.Count())
			require.True(t, f.rawStranded)
			rawChars := func(dst []rune) []rune {
				if f.rawStranded {
					return dst
				}
				return append(dst, []rune("honesty")...)
			}
			require.Empty(t, rawChars(nil), "Blur must not pretend to restore Cocoa raw character delivery")

			f.openTavern()
			f.press(ebiten.KeyB)
			meal, ok := f.dialog.ConsumePendingActionIntent()
			require.True(t, ok)
			require.Equal(t, TavernActionMeal, meal.Action)
			require.Equal(t, "Boar", meal.Meal.Meal.Name)
			f.dialog.CancelActionApproval()
			f.press(ebiten.KeyH)
			require.True(t, f.field.IsFocused())
			f.typeText("honesty")
			require.Empty(t, rawChars(nil))
			require.Equal(t, "honesty", f.dialog.GetTopicInput())
			f.press(ebiten.KeyEnter)
			topic, ok := f.dialog.ConsumePendingTopicIntent()
			require.True(t, ok)
			require.Equal(t, TavernTopicIntent{Input: "honesty"}, topic)
			require.False(t, f.field.IsFocused(), "quotation releases native focus immediately")
			require.Nil(t, f.dialog.topicInput, "native field is the only text authority")
		})
	}
}

func TestTavernNativeSubjectModalPhaseAndWindowFocus(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	f := newTavernNativeTest(t)
	f.openTavern()
	require.False(t, f.field.IsFocused(), "Order owns physical controls only")
	f.press(ebiten.KeyH)
	f.typeText("honesty")
	f.press(ebiten.KeyEnter)
	topic, ok := f.dialog.ConsumePendingTopicIntent()
	require.True(t, ok)
	f.dialog.ApplyInformationQuote(topic, testTavernInformationQuote(f.dialog.GetState(), "hone", 50), "I may know someone.")
	intent, ok := f.dialog.ConsumePendingActionIntent()
	require.True(t, ok)
	action := NewTavernActionDialog(TavernActionDialogConfig{
		Keyboard: f.keyboard, Intent: intent, IsKeyPressed: f.keyboard.IsKeyHeld,
	})
	f.stack.PushModalDialog(action)
	f.press(ebiten.KeyEscape)
	require.True(t, action.IsCancelled())
	f.dialog.CancelActionApproval()
	require.False(t, f.field.IsFocused(), "Browse under approval is not top-modal focus")
	f.stack.MustRemoveWidgetInstance(action)
	f.tick()
	require.True(t, f.field.IsFocused())
	require.Equal(t, "honesty", f.dialog.GetTopicInput())

	notice := NewTavernAdviceNotice(f.keyboard, f.dialog.GetWorkspaceBounds, "A merchant response.")
	f.stack.PushNonBlockingDialog(notice)
	require.True(t, f.field.IsFocused(), "nonblocking presentation does not take native focus")
	f.stack.MustRemoveWidgetInstance(notice)
	f.stack.PushModalDialog(notice)
	require.False(t, f.field.IsFocused())
	pumps := f.field.pumps
	f.dialog.Update() // Even a mistaken detached parent Update cannot steal focus.
	require.Equal(t, pumps, f.field.pumps)
	f.stack.MustRemoveWidgetInstance(notice)
	f.tick()
	require.True(t, f.field.IsFocused())

	f.windowFocused = false
	f.tick()
	require.False(t, f.field.IsFocused())
	f.held[ebiten.KeyEnter] = true
	f.windowFocused = true
	f.tick()
	require.False(t, f.field.IsFocused(), "held return-to-window keys must release first")
	require.False(t, f.dialog.HasPendingTopicIntent())
	f.held[ebiten.KeyEnter] = false
	f.tick()
	require.True(t, f.field.IsFocused())
	require.Equal(t, "honesty", f.dialog.GetTopicInput())

	f.dialog.Hide()
	require.False(t, f.field.IsFocused())
	f.dialog.ShowWithoutAnimation()
	f.tick()
	require.True(t, f.field.IsFocused())
	state := f.dialog.GetState()
	state.CustomerEligible = false
	f.dialog.ApplyState(state)
	require.False(t, f.field.IsFocused())
	state.CustomerEligible = true
	f.dialog.ApplyState(state)
	f.tick()
	require.True(t, f.field.IsFocused())
	f.dialog.ApplySessionError("Session ended.")
	require.False(t, f.field.IsFocused())
	f.press(ebiten.KeyEscape)
	require.True(t, f.dialog.IsDismissed())
}

func TestTavernNativeSubjectCompositionOwnsControlsBeforePaging(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyEscape, ebiten.KeyBackspace, ebiten.KeyPageUp} {
		t.Run(key.String(), func(t *testing.T) {
			f := newTavernNativeTest(t)
			f.openTavern()
			f.press(ebiten.KeyH)
			for index := range 12 {
				f.dialog.appendPaidTranscript(TavernTranscriptEntry{
					Question: fmt.Sprintf("Question %d", index), Answer: strings.Repeat("An answer. ", 20),
				})
			}
			f.dialog.populateBodyOutput()
			scroll := f.dialog.GetTranscriptScroll()
			require.Positive(t, scroll)
			f.typeText("a")
			edits := f.field.externalEdits
			f.field.nextInput = func() { f.field.preedit = "界" }
			f.held[key] = true
			f.tick() // Composition may begin only inside this native pump.
			require.Contains(t, f.dialog.GetWorkspaceText(), "a界")
			require.Equal(t, "a", f.dialog.GetTopicInput())
			f.field.nextInput = func() { f.field.commit("界") }
			f.tick()
			for range 3 {
				f.tick()
			}
			require.Equal(t, edits, f.field.externalEdits, "preedit/commit must not restart the session")
			require.Equal(t, "a界", f.dialog.GetTopicInput())
			require.Equal(t, TavernShopConversation, f.dialog.GetMode())
			require.False(t, f.dialog.HasPendingTopicIntent())
			require.Equal(t, scroll, f.dialog.GetTranscriptScroll(), "candidate controls cannot page the transcript")
			f.held[key] = false
			f.tick()
			f.press(ebiten.KeyEnter)
			topic, ok := f.dialog.ConsumePendingTopicIntent()
			require.True(t, ok)
			require.Equal(t, "a界", topic.Input)
		})
	}
}

func TestTavernNativeSubjectCompositionCancellationAndImmediateCommit(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	for _, key := range []ebiten.Key{ebiten.KeyEscape, ebiten.KeyBackspace, ebiten.KeyEnter} {
		t.Run(key.String(), func(t *testing.T) {
			f := newTavernNativeTest(t)
			f.openTavern()
			f.press(ebiten.KeyH)
			f.typeText("é")
			if key != ebiten.KeyEnter {
				f.field.nextInput = func() { f.field.preedit = "候" }
				f.tick()
			}
			f.held[key] = true
			f.field.nextInput = func() {
				f.field.preedit = ""
				if key == ebiten.KeyEnter {
					f.field.commit("界") // Preedit can start and finish inside a single native pump.
				}
			}
			f.tick()
			f.tick()
			require.False(t, f.dialog.HasPendingTopicIntent())
			require.Equal(t, TavernShopConversation, f.dialog.GetMode())
			if key == ebiten.KeyEnter {
				require.Equal(t, "é界", f.dialog.GetTopicInput())
			} else {
				require.Equal(t, "é", f.dialog.GetTopicInput(), "deleting/cancelling the last preedit rune cannot delete the draft")
			}
			f.held[key] = false
			f.tick()
			f.press(ebiten.KeyEscape)
			require.Equal(t, TavernShopOrder, f.dialog.GetMode())
			require.False(t, f.field.IsFocused())
		})
	}
}

func TestTavernNativeSubjectResultStackAndBrowseReturn(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	f := newTavernNativeTest(t)
	f.openTavern()
	f.press(ebiten.KeyH)
	f.typeText("honesty")
	f.press(ebiten.KeyEnter)
	topic, ok := f.dialog.ConsumePendingTopicIntent()
	require.True(t, ok)
	f.dialog.ApplyInformationQuote(topic, testTavernInformationQuote(f.dialog.GetState(), "hone", 50), "I may know someone.")
	intent, ok := f.dialog.ConsumePendingActionIntent()
	require.True(t, ok)
	f.dialog.AcceptActionApproval(intent.Request)
	f.tick()
	require.False(t, f.field.IsFocused())
	result := NewTavernResultDialog(TavernResultDialogConfig{
		Keyboard: f.keyboard, IsKeyPressed: f.keyboard.IsKeyHeld,
		View: TavernResultView{Title: "Information", Item: "Honesty", Tone: TavernResultSuccess},
	})
	f.stack.PushModalDialog(result)
	state := f.dialog.GetState()
	f.dialog.ApplyResult(intent.Request, state, TavernActionResultDisplay{
		Action: TavernActionInformation, MerchantExchange: "Ask the informant.",
		PaidInformation: &TavernTranscriptEntry{Question: "honesty", Answer: "Ask the informant."},
	})
	require.False(t, f.field.IsFocused(), "Browse cannot steal focus from a result acknowledgement")
	f.press(ebiten.KeyEnter)
	require.True(t, result.IsDismissed())
	f.stack.MustRemoveWidgetInstance(result)
	f.tick()
	require.True(t, f.field.IsFocused())
	require.Empty(t, f.dialog.GetTopicInput())
	f.typeText("   ")
	f.press(ebiten.KeyEnter)
	spaces, ok := f.dialog.ConsumePendingTopicIntent()
	require.True(t, ok)
	require.Equal(t, "   ", spaces.Input)
	f.dialog.RejectInformationTopic(spaces)
	f.tick()
	f.press(ebiten.KeyEnter)
	require.Equal(t, TavernShopOrder, f.dialog.GetMode(), "blank native subject returns to Order")
	require.False(t, f.field.IsFocused())
	require.Equal(t, state, f.dialog.GetState(), "widgets only emit intents and consume detached state")
}

func TestTavernNativeSubjectRuneLimitSelectionAndCaretBounds(t *testing.T) {
	for _, width := range []int{320, 1280} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			setMerchantShellTestResolution(t, width, width*9/16)
			f := newTavernNativeTest(t)
			f.openTavern()
			f.press(ebiten.KeyH)
			f.typeText(strings.Repeat("界", tavernTopicRuneLimit) + "\nextra")
			require.Equal(t, strings.Repeat("界", tavernTopicRuneLimit), f.dialog.GetTopicInput())
			edits := f.field.externalEdits
			f.field.nextInput = func() { f.field.preedit = "候" }
			f.tick()
			f.tick()
			require.Equal(t, edits, f.field.externalEdits, "full committed capacity must not reset live preedit")
			f.field.nextInput = func() { f.field.preedit = "" }
			f.press(ebiten.KeyBackspace)
			require.Equal(t, tavernTopicRuneLimit, utf8.RuneCountInString(f.dialog.GetTopicInput()))
			f.press(ebiten.KeyBackspace)
			require.Equal(t, tavernTopicRuneLimit-1, utf8.RuneCountInString(f.dialog.GetTopicInput()))
			f.typeText("é")
			f.held[ebiten.KeyControl] = true
			f.press(ebiten.KeyA)
			f.held[ebiten.KeyControl] = false
			require.False(t, f.dialog.topicSelectionBounds.Empty())
			f.typeText(strings.Repeat("é", tavernTopicRuneLimit))
			require.Equal(t, strings.Repeat("é", tavernTopicRuneLimit), f.dialog.GetTopicInput())
			f.held[ebiten.KeyShift] = true
			f.press(ebiten.KeyHome)
			f.held[ebiten.KeyShift] = false
			start, end := f.field.Selection()
			require.Zero(t, start)
			require.Equal(t, len(strings.Repeat("é", tavernTopicRuneLimit)), end)
			require.True(t, f.dialog.selectionReversed)
			f.typeText("é世界  ")
			require.Equal(t, "é世界  ", f.dialog.GetTopicInput(), "typed Unicode replaces the selected byte range")
			f.field.nextInput = func() { f.field.preedit = strings.Repeat("候", 30) }
			f.tick()
			f.tick() // Candidate placement now reflects the rendered preedit caret.
			require.True(t, f.field.bounds.In(f.dialog.bodyText))
			require.False(t, f.field.bounds.Empty())
			require.Equal(t, "é世界  ", f.dialog.GetTopicInput())
			screen := ebiten.NewImage(width, width*9/16)
			t.Cleanup(screen.Deallocate)
			f.dialog.Draw(screen)
			f.field.nextInput = func() { f.field.preedit = "" }
			f.tick()
			f.press(ebiten.KeyEnter)
			topic, ok := f.dialog.ConsumePendingTopicIntent()
			require.True(t, ok)
			require.Equal(t, "é世界  ", topic.Input, "spaces are preserved verbatim")
		})
	}
}

func TestTavernNativeSubjectInputErrorRetainsDraftAndReleasesSession(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	f := newTavernNativeTest(t)
	f.openTavern()
	f.press(ebiten.KeyH)
	f.typeText("honesty")
	f.field.inputError = errors.New("native service unavailable")
	f.tick()
	require.Equal(t, TavernShopSessionError, f.dialog.GetPhase())
	require.Contains(t, f.dialog.GetInlineFeedback(), "Subject text input failed")
	require.Contains(t, f.dialog.GetInlineFeedback(), "native service unavailable")
	require.Equal(t, "honesty", f.dialog.GetTopicInput())
	require.False(t, f.field.IsFocused())
	require.False(t, f.dialog.HasPendingTopicIntent())
	f.press(ebiten.KeyEscape)
	require.True(t, f.dialog.IsDismissed())
}

func TestTavernNativeSubjectCapacityPreservesUntouchedSuffix(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	for _, test := range []struct {
		name        string
		initial     string
		start, end  int
		replacement string
		want        string
		caret       int
	}{
		{
			name: "full insertion at start", initial: strings.Repeat("界", 15), start: 0, end: 0,
			replacement: "é", want: strings.Repeat("界", 15), caret: 0,
		},
		{
			name: "full middle replacement", initial: "ab界defghijklmno", start: len("ab"), end: len("ab界"),
			replacement: "éΩ", want: "abédefghijklmno", caret: len("abé"),
		},
		{
			name: "partially fitting printable replacement", initial: "ab界def", start: len("ab"), end: len("ab界"),
			replacement: " \n" + strings.Repeat("é", 12), want: "ab " + strings.Repeat("é", 9) + "def",
			caret: len("ab " + strings.Repeat("é", 9)),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newTavernNativeTest(t)
			f.openTavern()
			f.press(ebiten.KeyH)
			f.typeText(test.initial)
			f.field.nextInput = func() { f.field.Field.SetTextAndSelection(test.initial, test.start, test.end) }
			f.tick() // Native selection delivery, followed by its typed replacement.
			f.typeText(test.replacement)
			require.Equal(t, test.want, f.dialog.GetTopicInput())
			require.Equal(t, tavernTopicRuneLimit, utf8.RuneCountInString(f.dialog.GetTopicInput()))
			start, end := f.field.Selection()
			require.Equal(t, test.caret, start)
			require.Equal(t, start, end)
			edits := f.field.externalEdits
			f.tick()
			require.Equal(t, edits, f.field.externalEdits, "idle at capacity must not reset the session")
			f.press(ebiten.KeyEnd)
			f.press(ebiten.KeyBackspace)
			require.Equal(t, tavernTopicRuneLimit-1, utf8.RuneCountInString(f.dialog.GetTopicInput()))
		})
	}
}

func TestTavernNativeSubjectBoundsCommitWhenAnotherCompositionEnds(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	for _, committed := range []string{"é\n", strings.Repeat("界", 17)} {
		t.Run(fmt.Sprintf("committed=%q", committed), func(t *testing.T) {
			f := newTavernNativeTest(t)
			f.openTavern()
			f.press(ebiten.KeyH)
			f.field.nextInput = func() {
				f.field.commit(committed)
				f.field.preedit = "候" // A single pump can commit text and start the next composition.
			}
			f.tick()
			edits := f.field.externalEdits
			f.tick()
			require.Equal(t, edits, f.field.externalEdits, "do not interrupt the live composition to enforce capacity")
			f.field.nextInput = func() { f.field.preedit = "" }
			f.tick() // Committed text did not change this frame, but still needs validation.
			want := "é"
			if strings.HasPrefix(committed, "界") {
				want = strings.Repeat("界", tavernTopicRuneLimit)
			}
			require.Equal(t, want, f.dialog.GetTopicInput())
			f.press(ebiten.KeyEnter)
			topic, ok := f.dialog.ConsumePendingTopicIntent()
			require.True(t, ok)
			require.Equal(t, want, topic.Input)
		})
	}
}
