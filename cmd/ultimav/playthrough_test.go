package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

const otherTestPlaythroughID = "fedcba9876543210fedcba9876543210"

func TestPlaythroughIdentityGenerationAndValidation(t *testing.T) {
	first, err := newPlaythroughID()
	require.NoError(t, err)
	second, err := newPlaythroughID()
	require.NoError(t, err)
	assert.Regexp(t, `^[0-9a-f]{32}$`, first)
	assert.NotEqual(t, first, second)
	for _, id := range []string{"", "short", strings.ToUpper(testPlaythroughID), strings.Repeat("g", 32)} {
		_, err = freshPlaythroughMetadata(func() (string, error) { return id, nil })
		require.Error(t, err)
	}
	entropyErr := errors.New("entropy unavailable")
	_, err = freshPlaythroughMetadata(func() (string, error) { return "", entropyErr })
	require.ErrorIs(t, err, entropyErr)
}

func TestPlayerLoadRestoresMetadataWithoutEntropyBeforeActivation(t *testing.T) {
	record := playerSaveTestRecord(player_save.Slot03, "reviewed", validCommandTestSnapshot())
	record.Envelope.Metadata.UserDescription = "Before Doom"
	source := &playerSaveTestScene{playthrough: playthroughMetadata{id: otherTestPlaythroughID, userDescription: "Departing"}}
	game := NewGame(source)
	game.playthroughIDProvider = func() (string, error) { t.Fatal("load requested entropy"); return "", nil }
	for range 2 {
		candidate := &playerSaveTestScene{}
		candidate.onActivate = func() {
			assert.Equal(t, playthroughMetadata{id: testPlaythroughID, userDescription: "Before Doom"}, candidate.playthrough)
			assert.Equal(t, player_save.Slot03, candidate.slot)
			assert.Equal(t, "reviewed", candidate.revision)
		}
		game.ConfigurePlayerSaveRepository(&playerSaveTestRepository{}, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
			return candidate, nil
		})
		require.NoError(t, game.loadPlayerSaveRecord(record))
		assert.Equal(t, playthroughMetadata{id: testPlaythroughID, userDescription: "Before Doom"}, candidate.playthrough)
		assert.Equal(t, player_save.Slot03, candidate.slot)
		assert.Equal(t, "reviewed", candidate.revision)
	}
	for _, metadata := range []playthroughMetadata{
		{}, {id: "bad"}, {id: testPlaythroughID, userDescription: "bad\n"},
		{id: testPlaythroughID, userDescription: strings.Repeat("a", 81)},
	} {
		record.Envelope.Metadata.PlaythroughID = metadata.id
		record.Envelope.Metadata.UserDescription = metadata.userDescription
		active := game.currentScene
		require.Error(t, game.loadPlayerSaveRecord(record))
		assert.Same(t, active, game.currentScene)
	}
}

func TestManualSaveAndQuickSaveCommitOnlySuccessfulCheckpointNotes(t *testing.T) {
	source := &playerSaveTestScene{snapshot: validCommandTestSnapshot(), playthrough: playthroughMetadata{id: testPlaythroughID, userDescription: "Old note"}}
	game := NewGame(source)
	repository := &playerSaveTestRepository{saveErr: errors.New("disk full")}
	game.ConfigurePlayerSaveRepository(repository, nil)
	_, err := game.savePlayerSaveSlot(player_save.Slot02, "", "New note")
	require.Error(t, err)
	assert.Equal(t, "Old note", source.playthrough.userDescription)
	assert.Empty(t, source.slot)
	repository.saveErr = nil
	record, err := game.savePlayerSaveSlot(player_save.Slot02, "", "")
	require.NoError(t, err)
	assert.Empty(t, source.playthrough.userDescription)
	assert.Equal(t, testPlaythroughID, record.Envelope.Metadata.PlaythroughID)
	assert.Equal(t, playerSaveGameplayDescription, record.Envelope.Metadata.Description)
	quick, err := game.quickSavePlayerSave()
	require.NoError(t, err)
	assert.Equal(t, record.Revision, repository.saves[2].expectedRevision)
	assert.Empty(t, quick.Envelope.Metadata.UserDescription, "an intentional clear cannot fall back to an old note")
	assert.Equal(t, record.Slot, quick.Slot)
	assert.Equal(t, record.Envelope.Snapshot, quick.Envelope.Snapshot)
	for _, note := range []string{"\tbad", "bad\n", strings.Repeat("a", 81)} {
		captures := source.captures
		_, err = game.savePlayerSaveSlot(quick.Slot, quick.Revision, note)
		require.Error(t, err)
		assert.Equal(t, captures, source.captures)
		assert.Equal(t, quick.Revision, source.revision)
	}
	unrelated := playerSaveTestRecord(player_save.Slot05, "another", source.snapshot)
	unrelated.Envelope.Metadata.PlaythroughID = otherTestPlaythroughID
	unrelated.Envelope.Metadata.UserDescription = "Another playthrough's checkpoint"
	repository.records[player_save.Slot05] = unrelated
	overwritten, err := game.savePlayerSaveSlot(player_save.Slot05, unrelated.Revision, "Save As")
	require.NoError(t, err)
	assert.Equal(t, testPlaythroughID, overwritten.Envelope.Metadata.PlaythroughID)
	assert.Equal(t, "Save As", overwritten.Envelope.Metadata.UserDescription)
	assert.Equal(t, testPlaythroughID, source.playthrough.id)
}

func TestPlayerSaveMissingIdentityRejectsBeforeCaptureOrPersistence(t *testing.T) {
	for _, test := range []struct {
		name       string
		quick      bool
		provenance bool
	}{
		{name: "manual without provenance"},
		{name: "manual with provenance", provenance: true},
		{name: "quick with provenance", quick: true, provenance: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := validCommandTestSnapshot()
			metadata := playthroughMetadata{userDescription: "Identity-free checkpoint note"}
			scene := &playerSaveTestScene{snapshot: snapshot, playthrough: metadata}
			existing := playerSaveTestRecord(player_save.Slot03, "reviewed", snapshot)
			if test.provenance {
				scene.setPlayerSaveProvenance(existing.Slot, existing.Revision)
			}
			game := NewGame(scene)
			require.Equal(t, metadata, scene.playthrough, "activation must not synthesize an identity")
			entropyCalls := 0
			game.playthroughIDProvider = func() (string, error) {
				entropyCalls++
				return testPlaythroughID, nil
			}
			repository := &playerSaveTestRepository{
				records: map[player_save.SlotID]player_save.Record{existing.Slot: existing},
			}
			game.ConfigurePlayerSaveRepository(repository, nil)
			beforeSlot, beforeRevision, beforeActive := scene.playerSaveProvenance()
			var record player_save.Record
			var err error
			if test.quick {
				record, err = game.quickSavePlayerSave()
			} else {
				record, err = game.savePlayerSaveSlot(existing.Slot, existing.Revision, "Submitted note")
			}

			require.ErrorIs(t, err, save_snapshot.ErrInvalidEnvelope)
			assert.Equal(t, player_save.Record{}, record)
			assert.Zero(t, entropyCalls)
			assert.Zero(t, scene.captures)
			assert.Empty(t, repository.saves)
			assert.Empty(t, repository.loads, "destination metadata cannot supply missing scene identity")
			assert.Empty(t, repository.restores)
			assert.Empty(t, repository.deletes)
			assert.Equal(t, existing, repository.records[existing.Slot])
			afterSlot, afterRevision, afterActive := scene.playerSaveProvenance()
			assert.Equal(t, beforeSlot, afterSlot)
			assert.Equal(t, beforeRevision, afterRevision)
			assert.Equal(t, beforeActive, afterActive)
			assert.Equal(t, metadata, scene.playthrough)
			assert.Equal(t, snapshot, scene.snapshot)
			assert.Same(t, scene, game.currentScene)
		})
	}
}

func TestFreshImportsUseDistinctRetryStableIdentities(t *testing.T) {
	path := writePlayerSaveLegacyBundle(t, false)
	calls := 0
	game := NewGame(&playerSaveTestScene{})
	game.playthroughIDProvider = func() (string, error) {
		calls++
		if calls == 1 {
			return testPlaythroughID, nil
		}
		return otherTestPlaythroughID, nil
	}
	first, err := game.prepareLegacyPlayerSave(path)
	require.NoError(t, err)
	second, err := game.prepareLegacyPlayerSave(path)
	require.NoError(t, err)
	assert.Equal(t, first.snapshot, second.snapshot)
	assert.NotEqual(t, first.playthrough.id, second.playthrough.id)
	repository := &playerSaveTestRepository{saveErr: errors.New("retry")}
	game.ConfigurePlayerSaveRepository(repository, func(_ *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
		return &playerSaveTestScene{snapshot: canonicalPlayerSaveTestSnapshotFrom(snapshot)}, nil
	})
	for range 2 {
		require.Error(t, game.importPreparedLegacyPlayerSave(player_save.Slot01, "", first))
	}
	repository.saveErr = nil
	require.NoError(t, game.importPreparedLegacyPlayerSave(player_save.Slot01, "", first))
	assert.Equal(t, 2, calls)
	for _, save := range repository.saves {
		assert.Equal(t, first.playthrough.id, save.envelope.Metadata.PlaythroughID)
	}
}

func TestBrowserPlaythroughSelectionAndMarkerAreIndependentOfNamesAndCursor(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	summaries := []player_save.SlotSummary{
		{Slot: player_save.Slot01, Status: player_save.SlotReady, Metadata: player_save.MetadataSummary{PlaythroughID: otherTestPlaythroughID, AvatarName: "Derp", CreatedAt: now.Add(time.Hour)}},
		{Slot: player_save.Slot02, Status: player_save.SlotReady, Metadata: player_save.MetadataSummary{PlaythroughID: testPlaythroughID, AvatarName: "Derp", CreatedAt: now}},
		{Slot: player_save.Slot03, Status: player_save.SlotReady, Metadata: player_save.MetadataSummary{PlaythroughID: testPlaythroughID, AvatarName: "Renamed", CreatedAt: now}},
		{Slot: player_save.Slot04, Status: player_save.SlotEmpty},
	}
	write := buildPlayerSaveBrowserModel(summaries, playerSaveBrowserWrite, testPlaythroughID)
	assert.Equal(t, string(player_save.Slot02), write.initialSelectionID)
	assert.False(t, write.views[0].Newest)
	assert.True(t, write.views[1].Newest)
	assert.False(t, write.views[2].Newest)
	assert.Equal(t, "* Newest save for this playthrough", write.newestLegend)
	load := buildPlayerSaveBrowserModel(summaries, playerSaveBrowserLoad, testPlaythroughID)
	assert.Equal(t, string(player_save.Slot01), load.initialSelectionID)
	assert.True(t, load.views[0].Newest)
	assert.Equal(t, "* Newest save overall", load.newestLegend)
	noMatch := buildPlayerSaveBrowserModel(summaries, playerSaveBrowserWrite, "")
	assert.Equal(t, string(player_save.Slot04), noMatch.initialSelectionID)
	for _, view := range noMatch.views {
		assert.False(t, view.Newest)
	}
	full := buildPlayerSaveBrowserModel(summaries[:3], playerSaveBrowserWrite, "")
	assert.Empty(t, full.initialSelectionID)
	assert.True(t, full.requireExplicitSelection)
	summaries[1].Status, summaries[2].Status = player_save.SlotIncompatible, player_save.SlotCorrupt
	assert.Equal(t, string(player_save.Slot04), buildPlayerSaveBrowserModel(summaries, playerSaveBrowserWrite, testPlaythroughID).initialSelectionID)
}

func TestSaveDescriptionDefaultsAndInputContract(t *testing.T) {
	summary := player_save.SlotSummary{Slot: player_save.Slot01, Status: player_save.SlotReady,
		Metadata: player_save.MetadataSummary{PlaythroughID: testPlaythroughID, AvatarName: "Derp", UserDescription: "Destination note"}}
	draft := playerSaveDescriptionDraft{}
	assert.Equal(t, "Destination note", playerSaveDescriptionInitial(summary, testPlaythroughID, &draft))
	assert.Empty(t, playerSaveDescriptionInitial(summary, otherTestPlaythroughID, &draft))
	assert.Contains(t, playerSaveDescriptionNotice(summary, otherTestPlaythroughID), "This slot belongs to another playthrough.")
	assert.NotContains(t, playerSaveDescriptionNotice(summary, testPlaythroughID), "another playthrough")
	draft = playerSaveDescriptionDraft{slot: summary.Slot, text: "", set: true}
	assert.Empty(t, playerSaveDescriptionInitial(summary, testPlaythroughID, &draft))
	// An older, longer note is offered cut to the box's limit, never as an empty box.
	draft = playerSaveDescriptionDraft{}
	summary.Metadata.UserDescription = "Britain, day 12 - bought the crossbows"
	assert.Equal(t, "Britain, day 12 - boug", playerSaveDescriptionInitial(summary, testPlaythroughID, &draft))
	summary.Metadata.UserDescription = strings.Repeat("界", widgets.SaveSlotRowDescriptionMaxRunes+5)
	assert.Equal(t, strings.Repeat("界", widgets.SaveSlotRowDescriptionMaxRunes),
		playerSaveDescriptionInitial(summary, testPlaythroughID, &draft))
	for _, invalid := range []string{"\ntext", "text\r", "text\t", "bad\x00", strings.Repeat("a", 81)} {
		assert.NotEmpty(t, validatePlayerSaveDescriptionInput(invalid))
	}
	assert.Empty(t, validatePlayerSaveDescriptionInput("  About to go to Doom  "))
	assert.Empty(t, validatePlayerSaveDescriptionInput(strings.Repeat("\u754c", 80)))
}

func TestManualDescriptionNestedRetryRemovesAllDialogsAndDoesNotAdvanceGameplay(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).WithRandomSeed(519).WithSystemCallbacks().Build()
	scene := newEscapeMenuTestScene()
	scene.gameState = state
	game := NewGame(scene)
	game.playthroughIDProvider = func() (string, error) { t.Fatal("save requested entropy"); return "", nil }
	repository := &playerSaveTestRepository{saveErr: errors.New("disk full"), summaries: []player_save.SlotSummary{{Slot: player_save.Slot03, Status: player_save.SlotEmpty}}}
	game.ConfigurePlayerSaveRepository(repository, nil)
	before, err := scene.CaptureSaveSnapshot()
	require.NoError(t, err)
	scene.openPlayerSaveBrowser(game, playerSaveBrowserWrite)
	for attempt := range 3 {
		browser := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, scene)
		view, selected := browser.GetSelectedView()
		require.True(t, selected)
		assert.Equal(t, string(player_save.Slot03), view.ID)
		pressEscapeMenuWidgetKey(scene, browser, ebiten.KeyEnter)
		editor := requireEscapeMenuDialog[*widgets.SaveDescriptionEntryModal](t, scene)
		if attempt == 0 {
			editor.SetText(" About to go to Doom ")
		} else {
			assert.Equal(t, "About to go to Doom", editor.GetText())
		}
		if attempt == 2 {
			repository.saveErr = nil
		}
		editor.Submit()
		editor.Submit()
		assert.Len(t, repository.saves, attempt+1)
		if attempt < 2 {
			assert.Empty(t, scene.activePlaythrough.userDescription)
		}
	}
	assert.Zero(t, scene.dialogStack.Count())
	assert.Equal(t, "About to go to Doom", scene.activePlaythrough.userDescription)
	after, err := scene.CaptureSaveSnapshot()
	require.NoError(t, err)
	assert.Equal(t, before, after)
	for _, save := range repository.saves {
		assert.Equal(t, before, save.envelope.Snapshot)
	}
}

func TestSaveLocationAndLocalTimePresentation(t *testing.T) {
	if zone := os.Getenv(saveTimeFixtureEnvironment); zone != "" {
		if zone == "named" {
			assert.Equal(t, "Sep 6, 2026, 6:00 PM PDT", playerSaveLocalTime(time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC)))
			assert.Equal(t, "Mar 8, 2026, 1:59 AM PST", playerSaveLocalTime(time.Date(2026, 3, 8, 9, 59, 0, 0, time.UTC)))
			assert.Equal(t, "Mar 8, 2026, 3:00 AM PDT", playerSaveLocalTime(time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)))
		} else {
			assert.Contains(t, playerSaveLocalTime(time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC)), "+0530")
		}
		return
	}
	for floor, want := range map[int]string{-2: "basement 2", -1: "basement", 0: "main", 1: "1st", 2: "2nd", 3: "3rd", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd", 23: "23rd"} {
		assert.Equal(t, "Britain, "+want, playerSaveLocationLabel(int(references.Britain), floor))
	}
	assert.Equal(t, "Doom, level 1", playerSaveLocationLabel(int(references.Doom), 0))
	assert.Equal(t, "Deceit, level 8", playerSaveLocationLabel(int(references.Deceit), 7))
	assert.Equal(t, "Britannia", playerSaveLocationLabel(int(references.BritanniaUnderworld), 0))
	assert.Equal(t, "Underworld", playerSaveLocationLabel(int(references.BritanniaUnderworld), int(references.UNDERWORLD)))
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, zone := range []string{"named", "unnamed"} {
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		command := exec.CommandContext(ctx, executable, "-test.run=^TestSaveLocationAndLocalTimePresentation$", "-test.count=1")
		command.Env = append(os.Environ(), saveTimeFixtureEnvironment+"="+zone)
		output, runErr := command.CombinedOutput()
		cancel()
		require.NoError(t, runErr, "isolated %s timezone fixture: %s", zone, output)
	}
}

const saveTimeFixtureEnvironment = "ULTIMAV_TEST_SAVE_TIME_ZONE"

// Initialize timezone fixtures before any native loop is running. The child
// executes only the formatting test: the parent continues its native rendering
// normally and never mutates the process-wide location read by time.Now.
func initializeSaveTimeFixture() bool {
	switch os.Getenv(saveTimeFixtureEnvironment) {
	case "named":
		zone, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			panic(err)
		}
		time.Local = zone
	case "unnamed":
		time.Local = time.FixedZone("", 5*60*60+30*60)
	default:
		return false
	}
	return true
}

func TestDoomRecoveryRequiresPlaythroughAndRestoresIndependentNote(t *testing.T) {
	source, snapshot := newDoomRecoverySource(t, "run")
	source.activePlaythrough.userDescription = "In Doom"
	record := doomRecoveryRecord(snapshot, "run")
	record.Envelope.Metadata.UserDescription = "Before Doom"
	repository := &doomRecoveryTestRepository{record: record}
	game := NewGame(source)
	game.doomRecoveryRepository = repository
	game.playthroughIDProvider = func() (string, error) { t.Fatal("recovery requested entropy"); return "", nil }
	candidate := &playerSaveTestScene{snapshot: snapshot}
	game.doomRecoverySceneBuilder = func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) { return candidate, nil }
	source.activePlaythrough.id = otherTestPlaythroughID
	require.ErrorIs(t, game.returnBeforeDoom(source), ErrDoomRecoveryAssociation)
	source.activePlaythrough.id = testPlaythroughID
	require.NoError(t, game.returnBeforeDoom(source))
	assert.Equal(t, "Before Doom", candidate.playthrough.userDescription)
	assert.Empty(t, candidate.slot)
	assert.Empty(t, candidate.doomRunID)
	assert.Equal(t, doomRecoveryDescription, repository.record.Envelope.Metadata.Description)
}

func TestWorkbenchCaptureLoadAndRollbackPreserveCheckpointMetadata(t *testing.T) {
	source := &snapshotCommandTestScene{snapshot: validCommandTestSnapshot(), playthrough: playthroughMetadata{id: testPlaythroughID, userDescription: "Player note"}}
	game := NewGame(source)
	game.playthroughIDProvider = func() (string, error) { t.Fatal("checkpoint load requested entropy"); return "", nil }
	repository := &snapshotCommandTestRepository{records: make(map[string]save_snapshot.Record)}
	game.ConfigureSnapshotRepository(repository, func(_ *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
		return &snapshotCommandTestScene{snapshot: snapshot}, nil
	})
	result := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotCapture, map[string]any{
		"source": "project", "id": "checkpoint", "name": "Checkpoint", "description": "Developer purpose",
	})
	require.True(t, result.Success, result.ErrorMessage)
	record := repository.records["project:checkpoint"]
	assert.Equal(t, testPlaythroughID, record.Envelope.Metadata.PlaythroughID)
	assert.Equal(t, "Player note", record.Envelope.Metadata.UserDescription)
	assert.Equal(t, "Developer purpose", record.Envelope.Metadata.Description)
	source.playthrough = playthroughMetadata{id: otherTestPlaythroughID, userDescription: "Rollback note"}
	result = executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{"reference": record.Reference.String(), "expectedRevision": record.Revision})
	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, playthroughMetadataFromEnvelope(record.Envelope.Metadata), game.currentScene.(playthroughMetadataOwner).playthroughMetadata())
	var load debugging.SaveSnapshotLoadData
	require.NoError(t, json.Unmarshal(result.Data, &load))
	rollback := repository.records[load.RollbackReference]
	assert.Equal(t, otherTestPlaythroughID, rollback.Envelope.Metadata.PlaythroughID)
	assert.Equal(t, "Rollback note", rollback.Envelope.Metadata.UserDescription)
	assert.Contains(t, rollback.Envelope.Metadata.Description, "Captured before loading")
	result = executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{"reference": load.RollbackReference, "expectedRevision": load.RollbackRevision})
	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, source.playthrough, game.currentScene.(playthroughMetadataOwner).playthroughMetadata())
}

func TestNewGameIdentityFailureAndNestedSaveRetriesRetainPreparedPayload(t *testing.T) {
	game, scene, _ := newCompletedCharacterCreationFlow(t)
	snapshot := canonicalPlayerSaveTestSnapshot()
	calls := 0
	game.playthroughIDProvider = func() (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("no entropy")
		}
		return testPlaythroughID, nil
	}
	repository := newCharacterCreationFlowRepository(player_save.Slot04, player_save.SlotEmpty, "")
	repository.saveErr = errors.New("retry write")
	game.ConfigurePlayerSaveRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
		assert.Zero(t, scene.dialogStack.Count(), "prepared candidate capture must not retain save UI")
		return &playerSaveTestScene{snapshot: snapshot}, nil
	})
	game.configureCharacterCreationSaveSelection(scene, snapshot)
	assert.Zero(t, scene.dialogStack.Count())
	assert.Equal(t, 1, calls)
	pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)
	for attempt := range 3 {
		pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)
		editor, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveDescriptionEntryModal)
		require.True(t, ok)
		if attempt == 0 {
			editor.SetText("First checkpoint")
		} else {
			assert.Equal(t, "First checkpoint", editor.GetText())
		}
		if attempt == 2 {
			repository.saveErr = nil
		}
		editor.Submit()
	}
	assert.Equal(t, 2, calls, "one failed entropy request, then one retained prepared identity")
	require.Len(t, repository.saves, 3)
	for _, save := range repository.saves {
		assert.Equal(t, testPlaythroughID, save.envelope.Metadata.PlaythroughID)
		assert.Equal(t, "First checkpoint", save.envelope.Metadata.UserDescription)
	}
}

func TestImportDescriptionNestedRetriesKeepIdentityAndDraft(t *testing.T) {
	scene := newIntroInputTestScene()
	game := NewGame(scene)
	calls := 0
	game.playthroughIDProvider = func() (string, error) { calls++; return testPlaythroughID, nil }
	repository := newCharacterCreationFlowRepository(player_save.Slot04, player_save.SlotEmpty, "")
	repository.saveErr = errors.New("retry import")
	game.ConfigurePlayerSaveRepository(repository, func(_ *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
		assert.Zero(t, scene.dialogStack.Count())
		return &playerSaveTestScene{snapshot: canonicalPlayerSaveTestSnapshotFrom(snapshot)}, nil
	})
	openIntroImportBrowser(t, game, scene, writePlayerSaveLegacyBundle(t, false))
	for attempt := range 3 {
		scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
		require.NoError(t, game.Update())
		editor, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveDescriptionEntryModal)
		require.True(t, ok)
		if attempt == 0 {
			editor.SetText("Imported checkpoint")
		} else {
			assert.Equal(t, "Imported checkpoint", editor.GetText())
		}
		if attempt == 2 {
			repository.saveErr = nil
		}
		editor.Submit()
		if attempt < 2 {
			overlay, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
			require.True(t, ok)
			assert.Contains(t, overlay.GetMessage(), "retry import")
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			require.NoError(t, game.Update())
			assert.IsType(t, &widgets.SaveSlotBrowser{}, scene.dialogStack.PeekTopModalDialog())
			assert.Len(t, repository.saves, attempt+1)
		}
	}
	assert.Equal(t, 1, calls)
	assert.Zero(t, scene.dialogStack.Count())
	require.Len(t, repository.saves, 3)
	for _, save := range repository.saves {
		assert.Equal(t, testPlaythroughID, save.envelope.Metadata.PlaythroughID)
		assert.Equal(t, "Imported checkpoint", save.envelope.Metadata.UserDescription)
	}
}

func TestGenericWorkbenchLoadStartsFreshAndRejectsCandidateSlotAuthority(t *testing.T) {
	target := snapshotCommandTestRecord(t, "project:generic", strings.Repeat("a", 64))
	source := &snapshotCommandTestScene{snapshot: validCommandTestSnapshot(), playthrough: playthroughMetadata{id: testPlaythroughID, userDescription: "Departing note"}}
	game := NewGame(source)
	calls := 0
	game.playthroughIDProvider = func() (string, error) { calls++; return otherTestPlaythroughID, nil }
	repository := &snapshotCommandTestRepository{records: map[string]save_snapshot.Record{target.Reference.String(): target}}
	candidate := &playerSaveTestScene{slot: player_save.Slot01, revision: "unrelated"}
	game.ConfigureSnapshotRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) { return candidate, nil })
	result := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{"reference": target.Reference.String(), "expectedRevision": target.Revision})
	require.False(t, result.Success)
	assert.Same(t, source, game.currentScene)
	assert.Equal(t, 1, candidate.disposals)
	candidate = &playerSaveTestScene{}
	result = executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{"reference": target.Reference.String(), "expectedRevision": target.Revision})
	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, playthroughMetadata{id: otherTestPlaythroughID}, candidate.playthrough)
	assert.Empty(t, candidate.slot)
	assert.Equal(t, 2, calls)
}
