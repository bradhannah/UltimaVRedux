package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestSceneMusicHistorySurvivesMenuRoundTripWithoutRevivingPlayback(t *testing.T) {
	oldAudio, err := audio.NewGameAudioManagerWithConfig(nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, oldAudio.Close()) })
	oldScene := &GameScene{audioManager: oldAudio}
	game := &Game{currentScene: oldScene}
	require.NoError(t, oldAudio.PlayMusicForLocation(references.Britain))
	oldAudio.GetMusicManager().SetSuppressed(audio.MusicMenu, true)
	game.musicHistory = game.musicHistoryForReplacement()
	game.currentScene = &IntroMenuScene{}
	history := game.musicHistoryForReplacement()
	require.Equal(t, game.musicHistory, history)

	loadedAudio, err := audio.NewGameAudioManagerWithConfig(nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, loadedAudio.Close()) })
	loadedAudio.GetMusicManager().RestoreHistory(history)
	require.NoError(t, loadedAudio.PlayMusicForLocation(references.Britain))
	require.Empty(t, loadedAudio.GetMusicManager().GetCurrentTrack(), "a load respects the retained quiet interval")
	loadedAudio.GetMusicManager().SetEnabled(false)
	require.True(t, oldAudio.GetMusicManager().IsEnabled(), "candidate state must be detached from the old scene")
	require.Equal(t, history, game.musicHistory, "candidate mutation cannot alter application history")
}

func TestMusicToggleDuringQuietIntervalStaysOffAcrossLocationChanges(t *testing.T) {
	manager, err := audio.NewGameAudioManagerWithConfig(nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	scene := newEscapeMenuTestScene()
	scene.audioManager = manager
	require.Empty(t, manager.GetMusicManager().GetCurrentTrack())
	scene.toggleMusic()
	require.False(t, manager.GetMusicManager().IsEnabled())
	require.Contains(t, scene.output.GetOutputStr(false), "Music Off")
	require.NoError(t, manager.PlayMusicForLocation(references.Deceit))
	require.Empty(t, manager.GetMusicManager().GetCurrentTrack())
	scene.toggleMusic()
	require.True(t, manager.GetMusicManager().IsEnabled())
	require.Contains(t, scene.output.GetOutputStr(false), "Music On")
}

func TestSceneCombatSelectsEngagementAndMaleeDuringExplorationQuiet(t *testing.T) {
	manager, err := audio.NewGameAudioManagerWithConfig(nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	history := manager.GetMusicManager().History()
	loaded, err := audio.NewGameAudioManagerWithConfig(nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, loaded.Close()) })
	loaded.GetMusicManager().RestoreHistory(history)
	scene := newEscapeMenuTestScene()
	scene.audioManager = loaded
	scene.gameState.MapState.PlayerLocation.Location = references.Britain
	scene.updateSceneMusic()
	require.Empty(t, loaded.GetMusicManager().GetCurrentTrack())

	scene.gameState.CombatScene = &game_state.CombatState{EncounterID: 1}
	scene.gameState.MapState.PlayerLocation.Location = references.CombatLocation
	scene.updateSceneMusic()
	require.Equal(t, "combat", loaded.GetMusicManager().GetCurrentTrack())
	require.Empty(t, scene.getBardAmbientSources(), "world bards cannot suppress the combat score")
	scene.gameState.CombatScene.Victory = true
	scene.updateSceneMusic()
	require.Empty(t, loaded.GetMusicManager().GetCurrentTrack(), "victory silences the combat score while looting")
	loaded.GetMusicManager().SetSuppressed(audio.MusicMenu, true)
	scene.updateSceneMusic()
	loaded.GetMusicManager().SetSuppressed(audio.MusicMenu, false)
	scene.updateSceneMusic()
	require.Empty(t, loaded.GetMusicManager().GetCurrentTrack(), "closing a menu cannot resume a won encounter")

	scene.gameState.CombatScene = nil
	scene.gameState.MapState.PlayerLocation.Location = references.Deceit
	scene.updateSceneMusic()
	require.Equal(t, references.Deceit, loaded.GetMusicManager().GetCurrentLocation())
	require.Empty(t, loaded.GetMusicManager().GetCurrentTrack(), "return to exploration starts quiet")
}
