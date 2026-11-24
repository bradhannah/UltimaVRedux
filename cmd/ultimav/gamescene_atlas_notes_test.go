package main

import (
	"testing"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestAtlasNoteChildOwnsInputAndContinuousPause(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).WithSystemCallbacks().WithRandomSeed(42).Build()
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour = 19
	state.ElapsedMs = 321
	draft, err := state.BeginAtlasNoteDraft(0)
	require.NoError(t, err)
	note, err := state.CommitAtlasNote(draft, "Landing", "Walk north.\n\nBring food.")
	require.NoError(t, err)
	keys := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return keys[key] })
	now := time.Unix(100, 0)
	scene := &GameScene{
		gameState: state, gameConfig: state.GameConfig, keyboard: keyboard,
		now: func() time.Time { return now }, startTime: now,
		// This detached control/pause fixture must not poll a live native text session.
		appendInputChars: func(chars []rune) []rune { return chars },
	}
	scene.openAtlas(nil)
	step := func(held ...ebiten.Key) {
		clear(keys)
		for _, key := range held {
			keys[key] = true
		}
		keyboard.ClearFrameConsumption()
		scene.updateAtlasSession()
	}
	step()
	scene.atlasSession.dialog.NoteCommitted(state.CurrentAtlasDossier(), note.ID)
	beforeDate, beforeCalls := state.DateTime, state.GetRNG().Calls()
	scene.handleAtlasNoteIntent(widgets.AtlasNoteIntent{Action: widgets.AtlasNoteEdit, ID: note.ID})
	require.Equal(t, 2, scene.dialogStack.Count())
	require.Equal(t, scene.atlasSession.child, scene.dialogStack.PeekTopModalDialog())
	step()
	step(ebiten.KeyShiftLeft, ebiten.KeyM)
	require.NotNil(t, scene.atlasSession.child)
	require.False(t, scene.atlasSession.closing, "Shift+M belongs to the editor")
	step()
	step(ebiten.KeyEscape)
	require.Nil(t, scene.atlasSession.child)
	require.False(t, scene.atlasSession.closing)
	require.Equal(t, 1, scene.dialogStack.Count())
	step(ebiten.KeyEscape)
	require.False(t, scene.atlasSession.closing, "held child cancel cannot close parent")
	step()
	scene.handleAtlasNoteIntent(widgets.AtlasNoteIntent{Action: widgets.AtlasNoteEdit, ID: note.ID})
	step()
	step(ebiten.KeyControlLeft, ebiten.KeyEnter)
	require.Nil(t, scene.atlasSession.child)
	require.Equal(t, note, state.CaptureAtlasNotes().Records[0])
	require.Equal(t, journal.AtlasNoteKey(note.ID), scene.atlasView.SelectedKey)
	step()
	scene.handleAtlasNoteIntent(widgets.AtlasNoteIntent{Action: widgets.AtlasNoteDelete, ID: note.ID})
	step()
	step(ebiten.KeyEscape)
	step()
	require.Len(t, state.CaptureAtlasNotes().Records, 1)
	scene.handleAtlasNoteIntent(widgets.AtlasNoteIntent{Action: widgets.AtlasNoteDelete, ID: note.ID})
	step()
	step(ebiten.KeyEnter)
	require.Empty(t, state.CaptureAtlasNotes().Records)
	require.Equal(t, 1, scene.dialogStack.Count())
	step()
	scene.handleAtlasNoteIntent(widgets.AtlasNoteIntent{Action: widgets.AtlasNoteMark})
	require.Equal(t, 2, scene.dialogStack.Count())
	now = now.Add(time.Hour)
	scene.closeAtlasSession(true)
	require.Nil(t, scene.atlasSession)
	require.Zero(t, scene.dialogStack.Count())
	require.Equal(t, beforeDate, state.DateTime)
	require.Equal(t, beforeCalls, state.GetRNG().Calls())
	require.EqualValues(t, 321, scene.getElapsedMs())
}

func TestAtlasGameReplacementRemovesExactChild(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).WithSystemCallbacks().Build()
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour = 19
	scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld), now: func() time.Time { return time.Unix(100, 0) }}
	other := &countingDialog{}
	scene.dialogStack.PushNonBlockingDialog(other)
	scene.openAtlas(nil)
	scene.handleAtlasNoteIntent(widgets.AtlasNoteIntent{Action: widgets.AtlasNoteMark})
	require.Equal(t, 3, scene.dialogStack.Count())
	replacement, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).WithSystemCallbacks().Build()
	scene.gameState = replacement
	scene.updateAtlasSession()
	require.Nil(t, scene.atlasSession)
	require.Equal(t, 1, scene.dialogStack.Count())
	require.Equal(t, other, scene.dialogStack.Dialogs()[0])
	require.Empty(t, state.CaptureAtlasNotes().Records)
	require.Empty(t, replacement.CaptureAtlasNotes().Records)
}
