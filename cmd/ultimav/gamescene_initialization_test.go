package main

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/legacy_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/test/originaldata"
)

func TestNewGameSceneFromLegacySaveFileRejectsInvalidSizeRecoverably(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	filePath := filepath.Join(directory, "SAVED.GAM")
	if err := os.WriteFile(filePath, make([]byte, 4191), 0o600); err != nil {
		t.Fatalf("writing legacy save fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SAVED.OOL"), make([]byte, legacy_save.SavedObjectsSize), 0o600); err != nil {
		t.Fatalf("writing legacy object fixture: %v", err)
	}

	gameScene, err := NewGameSceneFromLegacySaveFile(nil, filePath)
	if gameScene != nil {
		t.Fatal("NewGameSceneFromLegacySaveFile() returned a scene for an invalid save")
	}
	if !errors.Is(err, legacy_save.ErrInvalidBundle) {
		t.Fatalf("NewGameSceneFromLegacySaveFile() error = %v, want ErrInvalidBundle", err)
	}
}

func TestNewGameSceneFromSaveSnapshotWithRealData(t *testing.T) {
	gameDataPath := originaldata.Directory(t, "DATA.OVL", "SAVED.GAM")
	legacySavePath := filepath.Join(gameDataPath, "SAVED.GAM")
	gameConfig := snapshotTestGameConfig(gameDataPath)
	rawDataOvl, err := os.ReadFile(filepath.Join(gameDataPath, "DATA.OVL"))
	requireNoError(t, err)
	gameConfig.RawDataOvl = rawDataOvl
	source, err := NewGameSceneFromLegacySaveFile(gameConfig, legacySavePath)
	requireNoError(t, err)
	t.Cleanup(source.Dispose)
	seedSceneJournalPresentation(t, source)
	snapshot, err := source.CaptureSaveSnapshot()
	requireNoError(t, err)
	snapshot.World.ElapsedMs = 9_876
	snapshot.Wind.NextTickMs = 9_900
	snapshot.Effects.MagicLightMinutesRemaining = 83
	snapshot.Effects.DurationType = byte(game_state.DurationEffectProtection)
	snapshot.Effects.DurationTurns = 37
	snapshot.Effects.Smashed = 25
	snapshot.Effects.Amulet = save_snapshot.ToggleSnapshot{Active: true, Turns: game_state.ToggleItemDuration}
	snapshot.Inventory.QuestItems[references.Crown] = 1
	snapshot.Inventory.QuestItems[references.Amulet] = 1
	snapshot.Inventory.QuestItems[references.BlackBadge] = 1

	candidate, err := NewGameSceneFromSaveSnapshot(gameConfig, snapshot)
	requireNoError(t, err)
	t.Cleanup(candidate.Dispose)
	if source.activePlaythrough.id != "" || candidate.activePlaythrough.id != "" {
		t.Fatal("generic scene constructors must not create playthrough identities")
	}
	if candidate.active {
		t.Fatal("NewGameSceneFromSaveSnapshot() activated a dormant candidate")
	}
	if candidate.dungeonCanvas == nil || candidate.dungeonClipLayer == nil {
		t.Fatal("NewGameSceneFromSaveSnapshot() did not allocate reusable dungeon render layers")
	}
	after, err := candidate.CaptureSaveSnapshot()
	requireNoError(t, err)
	if !reflect.DeepEqual(after, snapshot) {
		t.Fatal("snapshot changed while constructing a dormant GameScene")
	}
	if !candidate.gameState.MapState.Lighting.HasMagicLight() {
		t.Fatal("snapshot scene did not restore actual magical lighting")
	}
	effects := candidate.gameState.GetPersistentEffectStatuses()
	foundIntoxication := false
	for _, effect := range effects {
		foundIntoxication = foundIntoxication || effect.ID == game_state.PersistentEffectIntoxication
	}
	if !foundIntoxication {
		t.Fatalf("restored persistent effects = %#v, want intoxication", effects)
	}
	candidate.activeEffectsPanel.refreshLayout()
	expectedWorn := make([]game_state.WornArtifactStatus, 0, 3)
	for _, artifact := range candidate.gameState.GetWornArtifactStatuses() {
		if artifact.Worn {
			expectedWorn = append(expectedWorn, artifact)
		}
	}
	assert.Equal(t, expectedWorn, candidate.activeEffectsPanel.worn)
	assert.Equal(t, candidate.gameState.GetPersistentEffectStatuses(), candidate.activeEffectsPanel.visibleEffects)
	assert.Equal(t, candidate.rightRail.activeBorder, candidate.activeEffectsPanel.bounds)
	require.Len(t, candidate.activeEffectsPanel.worn, 1)
	assert.True(t, candidate.activeEffectsPanel.worn[0].Owned)
	assert.True(t, candidate.activeEffectsPanel.worn[0].Worn)
	if candidate.dialogStack.Count() != 0 {
		t.Fatal("effect overlay restoration replayed activation feedback")
	}
	if candidate.journalNotice != (journalNotice{}) {
		t.Fatal("journal hydration replayed discovery feedback")
	}
	game := NewGame(candidate)
	requireNoError(t, candidate.Update(game))
	if candidate.gameState.ElapsedMs < snapshot.World.ElapsedMs {
		t.Fatalf("elapsed time after activation = %d, want at least hydrated base %d", candidate.gameState.ElapsedMs, snapshot.World.ElapsedMs)
	}
	_, err = candidate.CaptureSaveSnapshot()
	requireNoError(t, err)

	crownSnapshot := snapshot
	crownSnapshot.Effects.DurationType = byte(game_state.DurationEffectNone)
	crownSnapshot.Effects.DurationTurns = 0
	crownSnapshot.Effects.Crown = save_snapshot.ToggleSnapshot{Active: true, Turns: game_state.ToggleItemDuration}
	crownCandidate, err := NewGameSceneFromSaveSnapshot(gameConfig, crownSnapshot)
	requireNoError(t, err)
	t.Cleanup(crownCandidate.Dispose)
	crownCandidate.activeEffectsPanel.refreshLayout()
	assert.Contains(t, crownCandidate.gameState.GetPersistentEffectStatuses(), game_state.PersistentEffectStatus{
		ID:         game_state.PersistentEffectCrownSuppression,
		Atmosphere: game_state.EffectAtmosphereSteady,
	})
	assert.NotContains(t, crownCandidate.activeEffectsPanel.visibleEffects, game_state.PersistentEffectStatus{
		ID:         game_state.PersistentEffectCrownSuppression,
		Atmosphere: game_state.EffectAtmosphereSteady,
	})
	require.Len(t, crownCandidate.activeEffectsPanel.worn, 2)
	assert.Equal(t, game_state.ToggleItemCrown, crownCandidate.activeEffectsPanel.worn[0].ID)
	assert.Equal(t, game_state.ToggleItemAmulet, crownCandidate.activeEffectsPanel.worn[1].ID)
}

func TestStatusBandBothResizePathsPreservePanelAndTranscript(t *testing.T) {
	state := &game_state.GameState{CrownState: game_state.ToggleItemState{IsActive: true, TurnsRemaining: 255}}
	panel := NewActiveEffectsPanel(state, syntheticStatusSpriteSheet(t), image.Rect(0, 0, 318, 65))
	scene := &GameScene{gameState: state, spriteSheet: panel.spriteSheet, activeEffectsPanel: panel}
	scene.initializeResizeableVisualElements()
	output := scene.output
	output.AddText("First history.")
	output.AddText("nothing of note.")
	history := output.GetOutputStr(false)
	initialBand := scene.rightRail.activeBorder
	for _, size := range []image.Point{{1280, 720}, {1366, 768}, {1920, 1080}, {2560, 1440}, {3840, 2160}, {1280, 720}} {
		bounds := logicalGameBounds(size.X, size.Y)
		scene.syncRightRailLayout(bounds)
		assert.Same(t, panel, scene.activeEffectsPanel)
		assert.Equal(t, scene.rightRail.activeBorder, panel.bounds)
		assert.Equal(t, calculateStatusBandLayout(panel.bounds), panel.layout)
		panel.refreshLayout()
		require.True(t, panel.worn[0].Worn)
		assert.Equal(t, history, scene.output.GetOutputStr(false))
		assert.Same(t, output, scene.output)
		assert.Equal(t, scene.rightRail.outputContent.Size(), scene.outputClipImage.Bounds().Size())
		_, scaledOutput := newMainOutput(bounds.Dx())
		assert.Equal(t, visibleLinesForOutput, scaledOutput.GetLineCapacity(scene.rightRail.outputContent.Dy()), "%v", size)
	}
	scene.initializeResizeableVisualElements()
	assert.Equal(t, initialBand, panel.bounds, "DisplayManager resize path reapplies outer status bounds")
	assert.Equal(t, history, scene.output.GetOutputStr(false))
	assert.Same(t, output, scene.output)
	assert.Equal(t, visibleLinesForOutput, scene.output.GetLineCapacity(scene.rightRail.outputContent.Dy()))
}

func TestStatusBandInitializationUsesPublishedLogicalSize(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	state := &game_state.GameState{}
	panel := NewActiveEffectsPanel(state, syntheticStatusSpriteSheet(t), image.Rectangle{})
	scene := &GameScene{gameState: state, spriteSheet: panel.spriteSheet, activeEffectsPanel: panel}
	game := &Game{currentScene: scene}
	for _, size := range []image.Point{{1788, 1033}, {1500, 1000}, {1366, 768}, {1280, 720}} {
		width, height := game.Layout(size.X, size.Y)
		bounds := image.Rect(0, 0, width, height)
		scene.initializeResizeableVisualElements()
		assert.Equal(t, calculateRightRailLayout(bounds), scene.rightRail, "%v", size)
		assert.InDelta(t, float64(defaultOutputFontSize)*float64(width)/referenceGameWidth,
			scene.ultimaFont.TextFace.Size, 0.001, "%v", size)
		clip := scene.outputClipImage
		scene.syncRightRailLayout(bounds)
		assert.Same(t, clip, scene.outputClipImage, "published logical bounds must already match the render target")
	}
}

func TestNewGameSceneFromActiveDungeonSnapshotPermitsRollbackCapture(t *testing.T) {
	gameDataPath := originaldata.Directory(t, "DATA.OVL", "SAVED.GAM")
	legacySavePath := filepath.Join(gameDataPath, "SAVED.GAM")
	gameConfig := snapshotTestGameConfig(gameDataPath)
	rawDataOvl, err := os.ReadFile(filepath.Join(gameDataPath, "DATA.OVL"))
	requireNoError(t, err)
	gameConfig.RawDataOvl = rawDataOvl
	source, err := NewGameSceneFromLegacySaveFile(gameConfig, legacySavePath)
	requireNoError(t, err)
	t.Cleanup(source.Dispose)
	_, err = source.gameState.DebugSetupDungeon(
		references.Deceit,
		6,
		references.Position{X: 2, Y: 3},
		references.DungeonFacingWest,
		true,
	)
	requireNoError(t, err)
	_, err = source.gameState.TransformDungeonCell(7, references.Position{X: 7, Y: 7}, func(cell references.DungeonCell) references.DungeonCell {
		return cell ^ 0xff
	})
	requireNoError(t, err)
	seedSceneJournalPresentation(t, source)

	snapshot, err := source.CaptureSaveSnapshot()
	requireNoError(t, err)
	candidate, err := NewGameSceneFromSaveSnapshot(gameConfig, snapshot)
	requireNoError(t, err)
	t.Cleanup(candidate.Dispose)
	rollback, err := candidate.CaptureSaveSnapshot()
	requireNoError(t, err)
	if !reflect.DeepEqual(rollback, snapshot) {
		t.Fatal("active dungeon snapshot changed before rollback capture")
	}
	if candidate.dialogStack.Count() != 0 {
		t.Fatal("active dungeon hydration replayed presentation feedback")
	}
}

func TestNewGameSceneFromSaveSnapshotRejectsInvalidSnapshot(t *testing.T) {
	invalid := save_snapshot.SaveSnapshot{Version: save_snapshot.CurrentVersion}
	invalid.World.Calendar.Day = 29

	scene, err := NewGameSceneFromSaveSnapshot(nil, invalid)
	if scene != nil {
		t.Fatal("NewGameSceneFromSaveSnapshot() returned a scene for an invalid snapshot")
	}
	if !errors.Is(err, save_snapshot.ErrInvalidSnapshot) {
		t.Fatalf("NewGameSceneFromSaveSnapshot() error = %v, want ErrInvalidSnapshot", err)
	}
}

func snapshotTestGameConfig(gameDataPath string) *config.UltimaVConfiguration {
	return &config.UltimaVConfiguration{
		SavedConfigData: &config.UltimaVConfigurationFlags{
			DataFilePath: gameDataPath,
		},
		DebugConfiguration: &config.DebugConfiguration{},
		SoundDisabled:      true,
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
