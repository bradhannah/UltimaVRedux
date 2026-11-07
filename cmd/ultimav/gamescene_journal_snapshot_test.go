package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/test/originaldata"
)

func journalSnapshotTestPresentation() journal.Presentation {
	presentation := journal.EmptyPresentation()
	presentation.History = "Thou dost see a Flame.\n\nRepeated message.\nRepeated message.\nCaf\u00e9  "
	presentation.Visits.Record(journal.ChronicleTab, 1)
	presentation.Visits.Record(journal.VirtuesTab, 0)
	presentation.Visits.Acknowledge(presentation.Visits.Revision, [3]bool{true, false, false})
	presentation.Visits.Record(journal.ShadowlordsTab, 2)
	return presentation
}

func seedSceneJournalPresentation(t *testing.T, scene *GameScene) {
	t.Helper()
	presentation := journalSnapshotTestPresentation()
	scene.output.EnableJournalHistory()
	require.NoError(t, scene.output.RestoreJournalHistory(presentation.History))
	scene.journalVisits = presentation.Visits.Clone()
}

func newJournalSnapshotOriginalScene(t *testing.T) *GameScene {
	t.Helper()
	directory := originaldata.Directory(t, "DATA.OVL", "SAVED.GAM", "SAVED.OOL")
	configuration := snapshotTestGameConfig(directory)
	var err error
	configuration.RawDataOvl, err = os.ReadFile(filepath.Join(directory, "DATA.OVL"))
	require.NoError(t, err)
	scene, err := newGameSceneFromOriginalSave(configuration, filepath.Join(directory, "SAVED.GAM"))
	require.NoError(t, err)
	t.Cleanup(scene.Dispose)
	seedSceneJournalPresentation(t, scene)
	return scene
}

func TestJournalPlayerSlotAndQuickSaveFullSceneRoundtrip(t *testing.T) {
	source := newJournalSnapshotOriginalScene(t)
	source.setPlaythroughMetadata(playthroughMetadata{id: testPlaythroughID})
	repository, err := player_save.NewRepository(t.TempDir(), source.ValidateSaveSnapshotCompatibility)
	require.NoError(t, err)
	game := NewGame(source)
	game.ConfigurePlayerSaveRepository(repository, func(c *config.UltimaVConfiguration, s save_snapshot.SaveSnapshot) (Scene, error) {
		return NewGameSceneFromSaveSnapshot(c, s)
	})
	first, err := game.savePlayerSaveSlot(player_save.Slot01, "", "Journal checkpoint")
	require.NoError(t, err)
	require.Equal(t, journalSnapshotTestPresentation(), first.Envelope.Snapshot.Presentation)
	source.output.AddText("Before Quick Save.")
	source.journalChanged(journal.VirtuesTab, 2)
	quick, err := game.quickSavePlayerSave()
	require.NoError(t, err)
	want, err := source.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.Equal(t, want, quick.Envelope.Snapshot)
	source.output.AddText("Departing scene only.")
	source.journalChanged(journal.ShadowlordsTab, 1)
	departing, err := source.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.Error(t, game.loadPlayerSaveSlot(player_save.Slot01, first.Revision))
	unchanged, err := source.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.Equal(t, departing, unchanged, "failed load preserves the entire departing checkpoint")
	require.NoError(t, game.loadPlayerSaveSlot(player_save.Slot01, quick.Revision))
	restored, ok := game.currentScene.(*GameScene)
	require.True(t, ok)
	t.Cleanup(restored.Dispose)
	after, err := restored.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.Equal(t, want, after, "loaded scene neither drops presentation nor merges departing markers")
	require.Equal(t, journalNotice{}, restored.journalNotice, "hydration must not replay a notice")
}

func TestJournalNamedSnapshotRollbackFullSceneRoundtrip(t *testing.T) {
	source := newJournalSnapshotOriginalScene(t)
	source.setPlaythroughMetadata(playthroughMetadata{id: testPlaythroughID})
	exchange := journal.Exchange{
		ID: "rollback-conversation-1", Speaker: journal.UnknownSpeaker, Location: "Britain", Input: "JOB",
		Passages: []journal.Passage{{Text: "Original clue", Runic: false}},
	}
	_, err := source.keepExcerpt(exchange)
	require.NoError(t, err)
	target := snapshotCommandTestRecord(t, "personal:journal", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	target.Envelope.Metadata.PlaythroughID = testPlaythroughID
	target.Envelope.Snapshot, err = source.CaptureSaveSnapshot()
	require.NoError(t, err)
	extended := exchange.Clone()
	extended.Passages = append(extended.Passages, journal.Passage{Text: "Later continuation", Runic: false})
	_, err = source.keepExcerpt(extended)
	require.NoError(t, err)
	source.output.AddText("Rollback-only message.")
	source.journalChanged(journal.VirtuesTab, 3)
	rollback, err := source.CaptureSaveSnapshot()
	require.NoError(t, err)
	repository := &snapshotCommandTestRepository{records: map[string]save_snapshot.Record{target.Reference.String(): target}}
	game := NewGame(source)
	game.ConfigureSnapshotRepository(repository, func(c *config.UltimaVConfiguration, s save_snapshot.SaveSnapshot) (Scene, error) {
		return NewGameSceneFromSaveSnapshot(c, s)
	})
	result := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{
		"reference": target.Reference.String(), "expectedRevision": target.Revision,
	})
	require.True(t, result.Success, result.ErrorMessage)
	var loaded debugging.SaveSnapshotLoadData
	require.NoError(t, json.Unmarshal(result.Data, &loaded))
	require.Equal(t, rollback, repository.records[loaded.RollbackReference].Envelope.Snapshot)
	restored, ok := game.currentScene.(*GameScene)
	require.True(t, ok)
	t.Cleanup(restored.Dispose)
	after, err := restored.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.Equal(t, target.Envelope.Snapshot, after)
	require.Equal(t, journalNotice{}, restored.journalNotice)
	excerpts, err := restored.loadExcerpts()
	require.NoError(t, err)
	displayOriginal := exchange.Clone()
	displayOriginal.Location = restored.journalLocationLabel(displayOriginal.Location)
	require.Equal(t, []journal.Exchange{displayOriginal}, excerpts, "older checkpoint restores its own speech")
	restored.setPlaythroughMetadata(playthroughMetadata{id: otherTestPlaythroughID})
	excerpts, err = restored.loadExcerpts()
	require.NoError(t, err)
	require.Equal(t, []journal.Exchange{displayOriginal}, excerpts, "identity metadata does not select external notes")
	restored.setPlaythroughMetadata(playthroughMetadata{id: testPlaythroughID})
	excerpts, err = restored.loadExcerpts()
	require.NoError(t, err)
	require.Equal(t, []journal.Exchange{displayOriginal}, excerpts)
}

func TestJournalFPSLimitedFullDrawUsesIndependentClock(t *testing.T) {
	scene := newJournalSnapshotOriginalScene(t)
	scene.gameConfig.DisplayConfiguration = &config.DisplayConfiguration{EnableFpsLimiting: true, TargetFPS: 30}
	now := time.Unix(100, 0)
	scene.now = func() time.Time { return now }
	scene.Activate()
	scene.ShowQuestJournalDossier(scene.gameState.CurrentQuestJournalDossier())
	frozen := scene.getElapsedMs()
	width, height := display.GetManager().GetCurrentSize()
	screen := newPresentationRaster(width, height)
	defer screen.Deallocate()
	scene.Draw(screen)
	first := scene.lastDrawTime
	now = now.Add(time.Millisecond)
	scene.Draw(screen)
	require.Equal(t, first, scene.lastDrawTime, "frame limiter still limits rendering")
	scene.keyboard.SimulateKeyPress(ebiten.KeyTab)
	now = now.Add(time.Second)
	require.NoError(t, scene.Update(nil))
	scene.Draw(screen)
	require.Greater(t, scene.lastDrawTime, first, "Journal navigation renders while simulation time is frozen")
	require.Equal(t, frozen, scene.getElapsedMs())
	require.Equal(t, frozen, scene.gameState.ElapsedMs)
}
