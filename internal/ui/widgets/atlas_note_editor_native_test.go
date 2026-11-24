package widgets

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

type atlasNativeTest struct {
	editor        *AtlasNoteEditor
	fields        [2]*tavernNativeTestField
	keyboard      *input.Keyboard
	held          map[ebiten.Key]bool
	stack         DialogStack
	windowFocused bool
	rawStranded   bool
	elapsed       int64
	saves         int
	cancels       int
	label, body   string
	submitError   error
}

func newAtlasNativeTest(t *testing.T) *atlasNativeTest {
	t.Helper()
	f := &atlasNativeTest{held: map[ebiten.Key]bool{}, windowFocused: true}
	f.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return f.held[key] })
	f.keyboard.SetElapsedMillisecondsSource(func() int64 { return f.elapsed })
	t.Cleanup(func() {
		if f.editor != nil {
			f.editor.Dismiss()
		}
	})

	return f
}

func (f *atlasNativeTest) open(record journal.AtlasNoteRecord) {
	for index := range f.fields {
		f.fields[index] = &tavernNativeTestField{rawStranded: &f.rawStranded}
	}
	f.editor = newAtlasNoteEditor(AtlasNoteEditorConfig{
		Record: record, Keyboard: f.keyboard,
		Submit: func(label, body string) error {
			if f.submitError != nil {
				return f.submitError
			}
			f.saves++
			f.label, f.body = label, body
			f.stack.MustRemoveWidgetInstance(f.editor)

			return nil
		},
		Cancel: func() {
			f.cancels++
			f.stack.MustRemoveWidgetInstance(f.editor)
		},
	}, [2]nativeTextField{f.fields[0], f.fields[1]})
	f.editor.isWindowFocused = func() bool { return f.windowFocused }
	f.stack.PushModalDialog(f.editor)
	f.tick()
}

func (f *atlasNativeTest) tick() {
	f.elapsed += 16
	f.keyboard.ClearFrameConsumption()
	if top := f.stack.PeekTopModalDialog(); top != nil {
		top.Update()
	}
}

func (f *atlasNativeTest) press(key ebiten.Key) {
	f.held[key] = true
	f.tick()
	f.held[key] = false
	f.tick()
}

func (f *atlasNativeTest) typeText(value string) {
	field := f.fields[f.editor.focus]
	field.nextInput = func() { field.commit(value) }
	f.tick()
}

func TestAtlasNoteEditorNativeProductionConstructor(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	editor := NewAtlasNoteEditor(AtlasNoteEditorConfig{
		Keyboard: newWidgetTestKeyboard(0), Submit: func(string, string) error { return nil }, Cancel: func() {},
	})
	t.Cleanup(editor.Dismiss)
	require.Nil(t, editor.appendChars, "production must not use raw character delivery")
	editor.isWindowFocused = func() bool { return true }
	editor.Update()
	for _, field := range editor.nativeFields {
		require.NotNil(t, field)
		require.False(t, field.IsFocused(), "detached construction/update must not steal native focus")
	}
}

func TestAtlasNoteEditorNativeAfterSaveHandoff(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	for _, submitted := range []bool{false, true} {
		t.Run(fmt.Sprintf("saveSubmitted=%t", submitted), func(t *testing.T) {
			f := newAtlasNativeTest(t)
			saveField := &tavernNativeTestField{rawStranded: &f.rawStranded}
			var result SaveDescriptionEntryResult
			save := newSaveDescriptionEntryModal(SaveDescriptionEntryModalConfig{
				Keyboard: f.keyboard, IsKeyPressed: f.keyboard.IsKeyHeld, MaxRunes: 80,
				OnResult: func(value string, accepted bool) {
					result = SaveDescriptionEntryResult{Text: value, Submitted: accepted}
				},
			}, saveField).AutoCloseAndPop(&f.stack)
			t.Cleanup(save.Dismiss)
			save.isWindowFocused = func() bool { return true }
			f.stack.PushModalDialog(save)
			f.tick()
			saveField.nextInput = func() { saveField.commit("Before Yew") }
			f.tick()
			key := ebiten.KeyEscape
			if submitted {
				key = ebiten.KeyEnter
			}
			f.press(key)
			require.Equal(t, SaveDescriptionEntryResult{Text: "Before Yew", Submitted: submitted}, result)
			require.False(t, saveField.IsFocused())
			require.Zero(t, f.stack.Count())
			rawChars := func(dst []rune) []rune {
				if f.rawStranded {
					return dst
				}

				return append(dst, []rune("Landing")...)
			}
			require.Empty(t, rawChars(nil), "ending native ownership does not repair the raw path")
			f.open(journal.AtlasNoteRecord{})
			require.Nil(t, f.editor.appendChars)
			f.typeText("Landing")
			f.press(ebiten.KeyTab)
			require.False(t, f.fields[0].IsFocused())
			require.True(t, f.fields[1].IsFocused())
			f.typeText("Walk north.")
			f.press(ebiten.KeyEnter)
			f.typeText("Bring food.")
			f.held[ebiten.KeyShiftLeft] = true
			f.press(ebiten.KeyTab)
			f.held[ebiten.KeyShiftLeft] = false
			require.True(t, f.fields[0].IsFocused())
			require.False(t, f.fields[1].IsFocused())
			f.typeText(" site")
			require.Equal(t, "Walk north.\nBring food.", string(f.editor.fields[1]))
			require.Empty(t, rawChars(nil))
			f.fields[0].onBlur = func() { f.fields[0].commit("late") }
			f.held[ebiten.KeyControlLeft] = true
			f.press(ebiten.KeyEnter)
			require.Equal(t, 1, f.saves)
			require.Equal(t, "Landing site", f.label)
			require.Equal(t, f.label, f.fields[0].Text(), "Blur cannot change a submitted draft")
			require.Equal(t, "Walk north.\nBring food.", f.body)
			require.Zero(t, f.cancels)
			require.False(t, f.fields[0].IsFocused())
			require.Zero(t, f.stack.Count())
		})
	}
}

func TestAtlasNoteEditorNativeReplacementPreservesSuffixAndPolicy(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	for _, index := range []int{0, 1} {
		t.Run(fmt.Sprintf("field=%d", index), func(t *testing.T) {
			f := newAtlasNativeTest(t)
			limit := journal.MaxAtlasNoteLabelRunes
			if index == 1 {
				limit = journal.MaxAtlasNoteDescriptionRunes
			}
			value := "AB" + strings.Repeat("x", limit-3) + "Z"
			f.open(journal.AtlasNoteRecord{Label: "Title"})
			if index == 1 {
				f.press(ebiten.KeyTab)
			}
			field := f.fields[index]
			field.nextInput = func() { field.Field.SetTextAndSelection(value, 1, 1) }
			f.tick()
			f.typeText("Q")
			require.Equal(t, value, field.Text(), "overflow must not truncate untouched suffix")
			require.Contains(t, f.editor.message, fmt.Sprint(limit))
			start, end := field.Selection()
			require.Equal(t, 1, start)
			require.Equal(t, start, end)
			field.nextInput = func() { field.Field.SetTextAndSelection(value, 1, 2) }
			f.tick()
			f.typeText("Q")
			want := "AQ" + value[2:]
			require.Equal(t, want, field.Text())
			require.Equal(t, want, string(f.editor.fields[index]))
			for _, rejected := range []string{"é", "\t", "\r", "\x7f"} {
				f.typeText(rejected)
				require.Equal(t, want, field.Text())
				require.Contains(t, f.editor.message, "printable")
			}
			edits := field.externalEdits
			f.tick()
			f.tick()
			require.Equal(t, edits, field.externalEdits, "idle frames must not reset native sessions")
			f.held[ebiten.KeyShiftLeft] = true
			f.press(ebiten.KeyHome)
			f.held[ebiten.KeyShiftLeft] = false
			require.Greater(t, f.editor.anchors[index], f.editor.cursors[index])
			f.typeText("R")
			require.Equal(t, "R"+want[2:], field.Text(), "typed input replaces reversed selection")
			f.held[ebiten.KeyControlLeft] = true
			f.press(ebiten.KeyA)
			f.held[ebiten.KeyControlLeft] = false
			f.typeText("First\n\nThird")
			if index == 0 {
				require.Equal(t, "R"+want[2:], field.Text(), "title remains single-line ASCII")
			} else {
				require.Equal(t, "First\n\nThird", field.Text())
				f.press(ebiten.KeyHome)
				f.press(ebiten.KeyUp)
				require.Equal(t, 6, f.editor.cursors[1])
				f.press(ebiten.KeyBackspace)
				require.Equal(t, "First\nThird", field.Text())
			}
		})
	}
}

func TestAtlasNoteEditorNativeCompositionOwnsControlsAndRendering(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	for _, key := range atlasEditorControlKeys() {
		t.Run(key.String(), func(t *testing.T) {
			f := newAtlasNativeTest(t)
			f.open(journal.AtlasNoteRecord{Label: "ABCD"})
			field := f.fields[0]
			field.nextInput = func() { field.Field.SetTextAndSelection("ABCD", 1, 3) }
			f.tick()
			edits := field.externalEdits
			field.nextInput = func() { field.preedit = "候界" }
			f.held[key] = true
			f.held[ebiten.KeyControlLeft] = true
			f.tick()
			content, anchor, caret := f.editor.fieldPresentation(0)
			require.Equal(t, "A候界D", string(content))
			require.Equal(t, 3, anchor, "native composition bytes become rune offsets")
			require.Equal(t, 3, caret)
			require.Equal(t, "ABCD", string(f.editor.fields[0]))
			f.tick()
			require.Equal(t, edits, field.externalEdits, "preedit must not be reset by synchronization")
			require.False(t, field.bounds.Empty())
			require.True(t, field.bounds.In(f.editor.fieldLayout(0).content))
			screen := ebiten.NewImage(1280, 720)
			t.Cleanup(screen.Deallocate)
			f.editor.Draw(screen)
			field.nextInput = func() { field.commit("é") }
			f.tick()
			f.tick()
			require.Equal(t, "ABCD", field.Text(), "committed Unicode is rejected without broadening note policy")
			require.Zero(t, f.editor.focus)
			require.Zero(t, f.saves)
			require.Zero(t, f.cancels)
			clear(f.held)
			f.tick()
			f.typeText("X")
			require.Equal(t, "AXD", field.Text(), "rejected commit restores the replacement selection")
			f.held[ebiten.KeyControlLeft] = true
			f.press(ebiten.KeyEnter)
			require.Equal(t, 1, f.saves)
		})
	}
}

func TestAtlasNoteEditorNativeFocusSuspensionAndDismissal(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	for _, transition := range []string{"window", "modal", "suspend", "remove", "dismiss"} {
		t.Run(transition, func(t *testing.T) {
			f := newAtlasNativeTest(t)
			f.open(journal.AtlasNoteRecord{Label: "Landing", Description: "Draft"})
			f.press(ebiten.KeyTab)
			field := f.fields[1]
			field.nextInput = func() { field.Field.SetTextAndSelection("Draft", 1, 4) }
			f.tick()
			field.nextInput = func() { field.preedit = "候" }
			f.tick()
			field.onBlur = func() { field.commit("late") }
			pumps := field.pumps
			f.held[ebiten.KeyEnter] = true
			f.held[ebiten.KeyControlLeft] = true
			switch transition {
			case "window":
				f.windowFocused = false
				f.tick()
				f.windowFocused = true
			case "modal":
				cover := NewTavernAdviceNotice(f.keyboard, func() image.Rectangle { return f.editor.pane }, "Covered")
				f.stack.PushModalDialog(cover)
				f.editor.Update()
				f.stack.MustRemoveWidgetInstance(cover)
			case "suspend":
				resume := f.keyboard.SuspendInputPolling()
				f.editor.SuspendTextInput()
				f.editor.SuspendTextInput()
				resume()
			case "remove":
				f.stack.MustRemoveWidgetInstance(f.editor)
			case "dismiss":
				f.editor.Dismiss()
				f.editor.Dismiss()
			}
			require.False(t, field.IsFocused())
			require.Equal(t, "Draft", field.Text(), "Blur must not change the accepted draft")
			start, end := field.Selection()
			require.Equal(t, 1, start)
			require.Equal(t, 4, end)
			require.Empty(t, field.preedit)
			f.tick()
			require.Equal(t, pumps, field.pumps)
			require.Zero(t, f.saves)
			require.Zero(t, f.cancels)
			clear(f.held)
			f.tick()
			if transition == "remove" || transition == "dismiss" {
				require.False(t, field.IsFocused())
				return
			}
			require.True(t, field.IsFocused())
			f.typeText("X")
			require.Equal(t, "DXt", field.Text())
			f.press(ebiten.KeyEscape)
			require.Equal(t, 1, f.cancels)
			require.False(t, field.IsFocused())
		})
	}
}

func TestAtlasNoteEditorNativeErrorsAndFreshReopen(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	f := newAtlasNativeTest(t)
	f.open(journal.AtlasNoteRecord{Label: "Landing", Description: "Keep this"})
	f.submitError = errors.New("stale fix")
	f.held[ebiten.KeyControlLeft] = true
	f.press(ebiten.KeyEnter)
	f.held[ebiten.KeyControlLeft] = false
	require.Equal(t, "stale fix", f.editor.message)
	require.False(t, f.editor.finished)
	require.True(t, f.fields[0].IsFocused())
	f.typeText(" site")
	require.Equal(t, "Landing site", string(f.editor.fields[0]))
	f.fields[0].inputError = errors.New("service unavailable")
	f.held[ebiten.KeyEscape] = true
	f.tick()
	f.tick()
	require.Zero(t, f.cancels, "failed native delivery cannot accidentally cancel")
	require.False(t, f.fields[0].IsFocused())
	require.Contains(t, f.editor.message, "Typing stopped working")
	f.held[ebiten.KeyEscape] = false
	f.tick()
	f.press(ebiten.KeyEscape)
	require.Equal(t, 1, f.cancels)
	f.submitError = nil
	f.open(journal.AtlasNoteRecord{})
	require.Empty(t, f.fields[0].Text())
	require.Empty(t, f.fields[1].Text())
	require.Empty(t, f.editor.message)
	f.typeText("New")
	f.held[ebiten.KeyControlLeft] = true
	f.press(ebiten.KeyEnter)
	require.Equal(t, "New", f.label)
	require.Empty(t, f.body)
}

func TestAtlasNoteEditorNativeCommitAndNextPreeditPreserveAcceptedDraft(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	for _, committed := range []string{"B", "é", strings.Repeat("x", 33)} {
		t.Run(fmt.Sprintf("commit=%q", committed), func(t *testing.T) {
			f := newAtlasNativeTest(t)
			f.open(journal.AtlasNoteRecord{Label: "A"})
			field := f.fields[0]
			field.nextInput = func() {
				field.commit(committed)
				field.preedit = "候"
			}
			f.tick()
			edits := field.externalEdits
			f.tick()
			require.Equal(t, edits, field.externalEdits, "new preedit survives even a rejected preceding commit")
			want := "A"
			if committed == "B" {
				want = "AB"
			}
			require.Equal(t, want, string(f.editor.fields[0]))
			f.editor.SuspendTextInput()
			require.Equal(t, want, field.Text(), "already delivered valid commits survive suspension")
			require.Empty(t, field.preedit)
			f.tick()
			f.held[ebiten.KeyControlLeft] = true
			f.press(ebiten.KeyEnter)
			require.Equal(t, want, f.label)
		})
	}
}

func TestAtlasNoteEditorNativeSameFrameCommitCannotSubmitOrCancel(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyEscape, ebiten.KeyTab} {
		t.Run(key.String(), func(t *testing.T) {
			f := newAtlasNativeTest(t)
			f.open(journal.AtlasNoteRecord{Label: "A"})
			f.held[ebiten.KeyControlLeft], f.held[key] = true, true
			f.typeText("B") // A complete native commit with no visible preedit frame.
			f.tick()
			require.Equal(t, "AB", string(f.editor.fields[0]))
			require.Zero(t, f.editor.focus)
			require.Zero(t, f.saves)
			require.Zero(t, f.cancels)
			clear(f.held)
			f.tick()
			f.fields[0].onBlur = func() { f.fields[0].commit("late") }
			f.press(ebiten.KeyEscape)
			require.Equal(t, 1, f.cancels)
			require.Equal(t, "AB", f.fields[0].Text())
			require.False(t, f.fields[0].IsFocused())
		})
	}
}
