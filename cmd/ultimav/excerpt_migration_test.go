package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/stretchr/testify/require"
)

func migrationTestExcerpts() journal.Excerpts {
	return journal.Excerpts{{ID: "kept-1", Speaker: "Iolo", Location: "Britain", Input: "WORD", Archived: true,
		Passages: []journal.Passage{{Text: "Complete speech\n\n  "}, {Text: "SUMM", Runic: true}}}}
}

func writeMigrationNotebook(t *testing.T, root string, excerpts journal.Excerpts) string {
	t.Helper()
	data, err := json.Marshal(journal.Notebook{Version: 1, Excerpts: excerpts})
	require.NoError(t, err)
	path := filepath.Join(root, testPlaythroughID+".json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func oldExcerptSaveData(t *testing.T, envelope save_snapshot.Envelope) []byte {
	t.Helper()
	data, err := save_snapshot.EncodeCanonical(envelope)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(data, &document))
	body := document["snapshot"].(map[string]any)
	body["version"] = 24
	delete(body, "excerpts")
	delete(body, "notes")
	data, err = json.Marshal(document)
	require.NoError(t, err)
	return data
}

func TestExcerptMigrationVersionBoundaryAndFailures(t *testing.T) {
	root := t.TempDir()
	path := writeMigrationNotebook(t, root, migrationTestExcerpts())
	game := &Game{notebookRoot: root}
	record := snapshotCommandTestRecord(t, "personal:legacy", "revision")
	record.Envelope.Metadata.PlaythroughID = testPlaythroughID
	old, err := save_snapshot.Decode(oldExcerptSaveData(t, record.Envelope))
	require.NoError(t, err)
	require.True(t, old.NeedsExcerptMigration())
	for range 2 {
		prepared, migrationErr := game.prepareExcerptMigration(old)
		require.NoError(t, migrationErr)
		require.Equal(t, migrationTestExcerpts(), prepared.Excerpts)
		prepared.Excerpts[0].Passages[0].Text = "Detached"
		require.Empty(t, old.Snapshot.Excerpts)
	}
	other := old
	other.Metadata.PlaythroughID = otherTestPlaythroughID
	prepared, err := game.prepareExcerptMigration(other)
	require.NoError(t, err)
	require.Empty(t, prepared.Excerpts)
	other.Metadata.PlaythroughID = ""
	prepared, err = game.prepareExcerptMigration(other)
	require.NoError(t, err)
	require.Empty(t, prepared.Excerpts, "missing identity cannot borrow another notebook")
	for _, data := range []string{"broken", `{"version":2,"excerpts":[]}`} {
		require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
		_, err = game.prepareExcerptMigration(old)
		require.Error(t, err)
		// Empty current saves remain authoritative even beside corrupt old data.
		prepared, err = game.prepareExcerptMigration(record.Envelope)
		require.NoError(t, err)
		require.Empty(t, prepared.Excerpts)
		after, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		require.Equal(t, data, string(after))
	}
	tooMany := make(journal.Excerpts, journal.MaxExcerpts+1)
	for i := range tooMany {
		tooMany[i] = migrationTestExcerpts()[0]
		tooMany[i].ID = fmt.Sprint(i)
	}
	writeMigrationNotebook(t, root, tooMany)
	_, err = game.prepareExcerptMigration(old)
	require.ErrorIs(t, err, journal.ErrNotebookFull)
	tooLong := migrationTestExcerpts()
	tooLong[0].Passages[0].Text = strings.Repeat("a", journal.MaxConversationBytes)
	writeMigrationNotebook(t, root, tooLong)
	_, err = game.prepareExcerptMigration(old)
	require.Error(t, err)
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Mkdir(path, 0o700))
	_, err = game.prepareExcerptMigration(old)
	require.Error(t, err, "unreadable/nonregular input is not an absent notebook")
	require.NoError(t, os.Remove(path))
	prepared, err = game.prepareExcerptMigration(old)
	require.NoError(t, err)
	require.Empty(t, prepared.Excerpts)
	game.notebookRoot = ""
	prepared, err = game.prepareExcerptMigration(old)
	require.NoError(t, err)
	require.Empty(t, prepared.Excerpts)
}

func TestExcerptPlayerSaveMigrationAndIndependentBranches(t *testing.T) {
	source := newJournalSnapshotOriginalScene(t)
	source.setPlaythroughMetadata(playthroughMetadata{id: testPlaythroughID})
	root, notebookRoot := t.TempDir(), t.TempDir()
	notebookPath := writeMigrationNotebook(t, notebookRoot, migrationTestExcerpts())
	repository, err := player_save.NewRepository(root, source.ValidateSaveSnapshotCompatibility)
	require.NoError(t, err)
	game := NewGame(source)
	game.notebookRoot = notebookRoot
	game.ConfigurePlayerSaveRepository(repository, func(c *config.UltimaVConfiguration, s save_snapshot.SaveSnapshot) (Scene, error) {
		return NewGameSceneFromSaveSnapshot(c, s)
	})
	initial, err := game.savePlayerSaveSlot(player_save.Slot01, "", "Old checkpoint")
	require.NoError(t, err)
	oldBytes := oldExcerptSaveData(t, initial.Envelope)
	for _, slot := range []player_save.SlotID{player_save.Slot01, player_save.Slot02} {
		require.NoError(t, os.WriteFile(filepath.Join(root, string(slot)+".json"), oldBytes, 0o600))
	}
	first, err := repository.Load(player_save.Slot01)
	require.NoError(t, err)
	require.NoError(t, game.loadPlayerSaveSlot(first.Slot, first.Revision))
	active := game.currentScene.(*GameScene)
	require.Equal(t, migrationTestExcerpts(), active.excerpts)
	savedA, err := game.quickSavePlayerSave()
	require.NoError(t, err)
	_, err = active.archiveExcerpt("kept-1", false)
	require.NoError(t, err)
	extended := active.excerpts[0].Clone()
	extended.Passages = append(extended.Passages, journal.Passage{Text: "Later continuation"})
	_, err = active.keepExcerpt(extended)
	require.NoError(t, err)
	added := extended.Clone()
	added.ID = "new-2"
	_, err = active.keepExcerpt(added)
	require.NoError(t, err)
	savedB, err := game.savePlayerSaveSlot(player_save.Slot03, "", "Later branch")
	require.NoError(t, err)
	second, err := repository.Load(player_save.Slot02)
	require.NoError(t, err)
	require.NoError(t, game.loadPlayerSaveSlot(second.Slot, second.Revision))
	require.Equal(t, migrationTestExcerpts(), game.currentScene.(*GameScene).excerpts, "first import did not consume shared legacy source")
	before := game.currentScene
	require.NoError(t, os.WriteFile(notebookPath, []byte("corrupt"), 0o600))
	require.Error(t, game.loadPlayerSaveSlot(second.Slot, second.Revision))
	require.Same(t, before, game.currentScene, "migration failure preserves the departing scene")
	for _, record := range []player_save.Record{savedA, savedB, savedA, savedB} {
		require.NoError(t, game.loadPlayerSaveSlot(record.Slot, record.Revision))
		got, captureErr := game.currentScene.(*GameScene).CaptureSaveSnapshot()
		require.NoError(t, captureErr)
		require.Equal(t, record.Envelope.Snapshot, got)
	}
	active = game.currentScene.(*GameScene)
	for _, excerpt := range active.excerpts.Clone() {
		_, err = active.deleteExcerpt(excerpt.ID)
		require.NoError(t, err)
	}
	empty, err := game.quickSavePlayerSave()
	require.NoError(t, err)
	require.Error(t, game.loadPlayerSaveSlot(savedB.Slot, savedB.Revision), "stale revisions cannot overwrite newer state")
	require.NoError(t, game.loadPlayerSaveSlot(empty.Slot, empty.Revision))
	require.Empty(t, game.currentScene.(*GameScene).excerpts)
	// Portability: move only the save bytes into another repository with no notebook.
	portableRoot := t.TempDir()
	data, err := save_snapshot.EncodeCanonical(savedA.Envelope)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(portableRoot, "slot-01.json"), data, 0o600))
	portable, err := player_save.NewRepository(portableRoot, source.ValidateSaveSnapshotCompatibility)
	require.NoError(t, err)
	game.playerSaveRepository, game.notebookRoot = portable, ""
	record, err := portable.Load(player_save.Slot01)
	require.NoError(t, err)
	require.NoError(t, game.loadPlayerSaveSlot(record.Slot, record.Revision))
	require.Equal(t, savedA.Envelope.Snapshot.Excerpts, game.currentScene.(*GameScene).excerpts)
	game.currentScene.(*GameScene).Dispose()
}

func TestExcerptNamedSnapshotMigrationPreservesRollback(t *testing.T) {
	source := newJournalSnapshotOriginalScene(t)
	source.setPlaythroughMetadata(playthroughMetadata{id: testPlaythroughID})
	root := t.TempDir()
	writeMigrationNotebook(t, root, migrationTestExcerpts())
	target := snapshotCommandTestRecord(t, "personal:legacy-excerpts", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	target.Envelope.Metadata.PlaythroughID = testPlaythroughID
	var err error
	target.Envelope.Snapshot, err = source.CaptureSaveSnapshot()
	require.NoError(t, err)
	target.Envelope, err = save_snapshot.Decode(oldExcerptSaveData(t, target.Envelope))
	require.NoError(t, err)
	departing := migrationTestExcerpts()[0]
	departing.ID, departing.Archived = "departing-only", false
	_, err = source.keepExcerpt(departing)
	require.NoError(t, err)
	repository := &snapshotCommandTestRepository{records: map[string]save_snapshot.Record{target.Reference.String(): target}}
	game := NewGame(source)
	game.notebookRoot = root
	game.ConfigureSnapshotRepository(repository, func(c *config.UltimaVConfiguration, s save_snapshot.SaveSnapshot) (Scene, error) {
		return NewGameSceneFromSaveSnapshot(c, s)
	})
	result := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{
		"reference": target.Reference.String(), "expectedRevision": target.Revision,
	})
	require.True(t, result.Success, result.ErrorMessage)
	var loaded debugging.SaveSnapshotLoadData
	require.NoError(t, json.Unmarshal(result.Data, &loaded))
	require.Equal(t, journal.Excerpts{departing}, repository.records[loaded.RollbackReference].Envelope.Snapshot.Excerpts)
	restored := game.currentScene.(*GameScene)
	t.Cleanup(restored.Dispose)
	require.Equal(t, migrationTestExcerpts(), restored.excerpts)
}
