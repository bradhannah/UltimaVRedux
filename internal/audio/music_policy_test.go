package audio

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

type policyPlayer struct {
	track    string
	playing  bool
	position time.Duration
	volume   float64
	starts   []string
	resumes  []time.Duration
	fail     error
	empty    bool
	events   []string
	overlaps bool
}

func (player *policyPlayer) Play(track string) error {
	player.overlaps = player.overlaps || player.playing
	player.events = append(player.events, "play "+track)
	player.starts = append(player.starts, track)
	player.track, player.position = track, 0
	player.playing = player.fail == nil && !player.empty
	return player.fail
}
func (player *policyPlayer) Stop() error {
	player.events = append(player.events, "stop "+player.track)
	player.playing = false
	player.track = ""
	return nil
}
func (player *policyPlayer) IsPlaying() bool              { return player.playing }
func (player *policyPlayer) Update() error                { return nil }
func (player *policyPlayer) SetVolume(gain float64) error { player.volume = gain; return nil }
func (player *policyPlayer) Pause() time.Duration         { player.playing = false; return player.position }
func (player *policyPlayer) Resume(track string, position time.Duration) error {
	player.track, player.position, player.playing = track, position, true
	player.resumes = append(player.resumes, position)
	return player.fail
}

func newPolicyTest(t *testing.T) (*musicPolicy, *policyPlayer, func(time.Duration)) {
	t.Helper()
	player := &policyPlayer{}
	now := time.Unix(100, 0)
	policy := newMusicPolicy(player, func() time.Time { return now })
	advance := func(delta time.Duration) {
		t.Helper()
		now = now.Add(delta)
		require.NoError(t, policy.update())
	}
	return policy, player, advance
}

func TestSailingMusicPlaysOncePerHoistAndFadesOnFurl(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "overworld"
	policy.setSailing(true, "hornpipe")
	advance(0)
	require.Equal(t, []string{"hornpipe"}, player.starts)
	advance(musicFadeDuration)
	for range 3 {
		policy.setSailing(true, "hornpipe")
		advance(time.Second)
	}
	require.Len(t, player.starts, 1, "movement and frame updates must not restart sailing music")
	player.playing = false
	advance(0)
	advance(10 * time.Minute)
	require.Len(t, player.starts, 1, "natural completion stays silent while sails remain raised")
	policy.setSailing(false, "hornpipe")
	advance(0)
	policy.setSailing(true, "hornpipe")
	advance(musicFadeDuration)
	advance(0)
	require.Equal(t, "hornpipe", player.starts[len(player.starts)-1], "a new hoist bypasses exploration quiet")
	require.Len(t, player.starts, 3, "exploration may briefly resume when sails are furled")
	advance(musicFadeDuration)
	require.Equal(t, policy.sailingEpoch, policy.trackSailEpoch, "current hoist owns the playing score")
	policy.setSailing(false, "hornpipe")
	advance(musicFadeDuration / 2)
	require.True(t, player.playing, "furling fades instead of closing: track=%s epoch=%d/%d envelope=%f",
		policy.track, policy.trackSailEpoch, policy.sailingEpoch, policy.envelope)
	require.Less(t, player.volume, 1.0)
	advance(musicFadeDuration / 2)
	require.False(t, player.playing)
}

func TestSailingMusicYieldToCombatWithoutReplayingAfterward(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.setSailing(true, "hornpipe")
	advance(0)
	policy.setCombatEncounter(1)
	advance(musicFadeDuration)
	advance(0)
	require.Equal(t, []string{"hornpipe", "combat"}, player.starts)
	policy.setCombatEncounter(0)
	advance(musicFadeDuration)
	advance(0)
	require.Equal(t, []string{"hornpipe", "combat"}, player.starts)
	policy.setSailing(false, "hornpipe")
	policy.setSailing(true, "hornpipe")
	advance(0)
	require.Equal(t, []string{"hornpipe", "combat", "hornpipe"}, player.starts)
}

func TestNewMusicRequestCancelsPriorAndSupersededRequestNeverStarts(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.setSailing(true, "hornpipe")
	advance(0)
	advance(musicFadeDuration)
	policy.cue = "victory"
	advance(musicFadeDuration / 2)
	require.Equal(t, []string{"hornpipe"}, player.starts, "new score waits for previous voice to stop")
	policy.cue = "reunion" // Victory is replaced before it ever starts.
	advance(musicFadeDuration / 2)
	advance(0)
	require.Equal(t, []string{"hornpipe", "reunion"}, player.starts)
	require.Equal(t, []string{"play hornpipe", "stop hornpipe", "play reunion"}, player.events)
	require.False(t, player.overlaps)
	policy.setSailing(false, "hornpipe")
	policy.setSailing(true, "hornpipe") // A new hoist replaces the active special cue.
	advance(0)
	advance(0)
	require.Equal(t, []string{"hornpipe", "reunion", "hornpipe"}, player.starts)
	require.Equal(t, []string{"play hornpipe", "stop hornpipe", "play reunion", "stop reunion", "play hornpipe"}, player.events)
	require.False(t, player.overlaps)
}

func TestMusicPolicyFiniteQuietAndStableSubduedRepeat(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	advance(0)
	require.Zero(t, player.volume, "new playback fades in from silence")
	advance(musicFadeDuration)
	initial := math.Pow(10, musicInitialDB/20)
	repeat := math.Pow(10, musicRepeatDB/20)
	require.InDelta(t, initial, player.volume, 1e-9)
	require.Less(t, initial, 0.4, "even the first presentation is subdued")
	for range 4 {
		player.playing = false // Device reports natural EOF, not a wall-time estimate.
		advance(time.Second)
		count := len(player.starts)
		advance(musicQuietInterval - time.Nanosecond)
		require.Len(t, player.starts, count)
		advance(time.Nanosecond)
		require.Len(t, player.starts, count+1)
		advance(musicFadeDuration)
		require.InDelta(t, repeat, player.volume, 1e-9, "repeat attenuation never accumulates")
	}
}

func TestMusicPolicySameThemeAndRapidReentry(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	advance(0)
	advance(musicFadeDuration)
	player.position = 45 * time.Second
	for range 6 {
		policy.exploration = "town" // Floors and other locations resolving to this theme.
		advance(time.Second)
	}
	require.Equal(t, []string{"town"}, player.starts)
	require.Equal(t, 45*time.Second, player.position)
	policy.exploration = "overworld"
	advance(musicFadeDuration / 2)
	policy.exploration = "town" // Return before fade-out finishes: reverse it, retain voice.
	advance(musicFadeDuration)
	require.Equal(t, []string{"town"}, player.starts)
	policy.exploration = "overworld"
	advance(musicFadeDuration)
	require.False(t, player.playing)
	advance(time.Minute)
	policy.exploration = "town"
	advance(time.Minute)
	policy.exploration = "dungeon"
	advance(time.Minute)
	policy.exploration = "town"
	advance(2 * time.Minute)
	advance(musicFadeDuration)
	require.Equal(t, []string{"town", "town"}, player.starts)
	require.InDelta(t, math.Pow(10, musicRepeatDB/20), player.volume, 1e-9)
}

func TestMusicPolicyIndependentOwnersFreezeQuietAndResumePosition(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	advance(0)
	advance(musicFadeDuration)
	player.position = 37 * time.Second
	policy.setSuppressed(musicWorkflow, true)
	policy.setSuppressed(MusicMenu, true)
	advance(time.Hour)
	policy.setSuppressed(musicWorkflow, false)
	advance(time.Hour)
	require.Empty(t, player.resumes)
	policy.setSuppressed(MusicMenu, false)
	advance(0)
	require.Equal(t, []time.Duration{37 * time.Second}, player.resumes)
	require.Zero(t, player.volume)
	advance(musicFadeDuration)
	player.playing = false
	advance(0)
	advance(time.Minute)
	policy.setSuppressed(MusicJournal, true)
	advance(time.Hour)
	policy.setSuppressed(MusicJournal, false)
	advance(4*time.Minute - time.Nanosecond)
	require.Len(t, player.starts, 1)
	advance(time.Nanosecond)
	require.Len(t, player.starts, 2)
}

func TestMusicPolicyInterruptionsRestoreActualContextNotPreview(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "same location", true: "changed location"}[changed], func(t *testing.T) {
			policy, player, advance := newPolicyTest(t)
			policy.exploration = "town"
			advance(0)
			player.position = time.Minute
			policy.setSuppressed(MusicMenu, true)
			require.NoError(t, player.Play("preview"))
			advance(time.Hour)
			require.Equal(t, "preview", player.track)
			if changed {
				policy.exploration = "dungeon"
			}
			require.NoError(t, player.Stop())
			policy.setSuppressed(MusicMenu, false)
			advance(0)
			if !changed {
				require.Equal(t, "town", player.track)
				require.Equal(t, time.Minute, player.position)
				return
			}
			require.Empty(t, player.resumes, "never resume the stale town track")
			advance(musicQuietInterval)
			require.Equal(t, "dungeon", player.track)
		})
	}
}

func TestMusicPolicyLocalMusicHoldsQuietAndAvoidsBoundaryChatter(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	advance(0)
	player.position = time.Minute
	for range 10 {
		policy.localMusic()
		advance(time.Second)
	}
	require.False(t, player.playing)
	require.Empty(t, player.resumes)
	advance(musicLocalRelease)
	advance(0)
	require.Equal(t, []time.Duration{time.Minute}, player.resumes)
	policy.localMusic()
	policy.exploration = "dungeon"
	advance(musicLocalRelease)
	advance(0)
	advance(musicQuietInterval)
	require.Equal(t, "dungeon", player.track)
}

func TestMusicPolicyDisableWhileQuietCannotBeUndoneByTravel(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	policy.history.quietRemaining = time.Minute
	policy.history.disabled = true
	advance(time.Hour)
	policy.exploration = "dungeon"
	advance(time.Hour)
	require.Empty(t, player.starts)
	policy.history.disabled = false
	advance(time.Minute - time.Nanosecond)
	require.Empty(t, player.starts)
	advance(time.Nanosecond)
	require.Equal(t, []string{"dungeon"}, player.starts)
}

func TestMusicPolicySessionHistorySurvivesLoadWithoutPlaybackOwners(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	advance(0)
	policy.setSuppressed(musicWorkflow, true)
	history := policy.exportHistory()
	loaded, loadedPlayer, loadedAdvance := newPolicyTest(t)
	loaded.restoreHistory(history)
	loaded.exploration = "town"
	loadedAdvance(0)
	require.Empty(t, loadedPlayer.starts)
	loadedAdvance(musicQuietInterval)
	loadedAdvance(musicFadeDuration)
	require.Equal(t, []string{"town"}, loadedPlayer.starts)
	require.InDelta(t, math.Pow(10, musicRepeatDB/20), loadedPlayer.volume, 1e-9)
	require.Empty(t, loadedPlayer.resumes)
	require.Empty(t, loaded.holds)
	loaded.history.heard["dungeon"] = true
	require.False(t, policy.history.heard["dungeon"], "history must be detached for failed scene candidates")
	require.False(t, history.heard["dungeon"])
	require.False(t, player.playing)
}

func TestMusicPolicySpecialCueCompletesOnceAndRestoresCurrentExploration(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	policy.history.quietRemaining = musicQuietInterval
	policy.cue = "victory"
	advance(0)
	require.Equal(t, []string{"victory"}, player.starts, "special cue may bypass exploration quiet")
	policy.exploration = "dungeon"
	player.playing = false
	advance(0)
	advance(musicQuietInterval)
	require.Equal(t, []string{"victory", "dungeon"}, player.starts)
	require.False(t, policy.history.heard["victory"])
}

func TestMusicPolicyMissingAndFailedTracksDoNotRetryEveryFrame(t *testing.T) {
	for _, failure := range []error{nil, errors.New("device failure")} {
		policy, player, _ := newPolicyTest(t)
		player.empty, player.fail = true, failure
		policy.exploration = "town"
		err := policy.update()
		require.ErrorIs(t, err, failure)
		for range 60 {
			require.NoError(t, policy.update())
		}
		require.Equal(t, []string{"town"}, player.starts)
		require.False(t, policy.history.heard["town"])
	}
}

func TestMusicManagerSameThemeAndNestedWorkflowOwnership(t *testing.T) {
	manager, _, _ := gameAudioFixture(t)
	policy, player, advance := newPolicyTest(t)
	manager.policy.policy = policy
	for _, location := range []references.Location{references.Britain, references.Britain, references.Minoc} {
		require.NoError(t, manager.PlayMusicForLocation(location))
	}
	require.Equal(t, []string{"town"}, player.starts)
	manager.PauseBackgroundMusic()
	manager.PauseBackgroundMusic()
	require.NoError(t, manager.PlayMusicForLocation(references.Deceit))
	manager.ResumeBackgroundMusic()
	advance(time.Hour)
	require.Empty(t, player.resumes)
	manager.ResumeBackgroundMusic()
	advance(0)
	advance(musicQuietInterval)
	require.Equal(t, []string{"town", "dungeon"}, player.starts)
}

func TestMusicPolicyMutePreservesContextGain(t *testing.T) {
	manager, _, _ := gameAudioFixture(t)
	policy, player, advance := newPolicyTest(t)
	manager.policy.policy = policy
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	advance(musicFadeDuration)
	require.NoError(t, manager.SetMusicVolume(0))
	require.False(t, player.playing)
	advance(time.Hour)
	require.NoError(t, manager.SetMusicVolume(0.5))
	advance(0)
	advance(musicFadeDuration)
	require.Len(t, player.starts, 1)
	require.InDelta(t, math.Pow(10, musicInitialDB/20), player.volume, 1e-9)
}

func TestMusicPolicyInstrumentOwnershipUsesPlayingVoices(t *testing.T) {
	manager, _, _ := gameAudioFixture(t)
	policy, player, advance := newPolicyTest(t)
	manager.policy.policy = policy
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	require.NoError(t, manager.PlaySoundEffect(game_state.SoundHarpsichord0))
	require.False(t, player.playing)
	require.True(t, manager.hasLocalMusic())
	manager.StopSound(game_state.SoundStopRequest{Effect: game_state.SoundHarpsichord0})
	require.False(t, manager.hasLocalMusic())
	advance(musicLocalRelease)
	advance(0)
	require.True(t, player.playing)
}

func TestMusicPolicyRepeatedOwnerRefreshDoesNotFreezeClock(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	advance(0)
	policy.setSuppressed(MusicCinematic, false)
	advance(musicFadeDuration)
	require.InDelta(t, math.Pow(10, musicInitialDB/20), player.volume, 1e-9)
	player.playing = false
	advance(0)
	for range 5 {
		policy.setSuppressed(MusicCinematic, false)
		advance(time.Minute)
	}
	require.Len(t, player.starts, 2)
}

func TestMusicRequestWhileDisabledLoadsWhenEnabled(t *testing.T) {
	manager, backend, _ := gameAudioFixture(t)
	manager.SetEnabled(false)
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	require.Empty(t, backend.players)
	manager.SetEnabled(true)
	manager.Update()
	require.Len(t, backend.players, 1)
	require.True(t, manager.music.IsPlaying())
	require.Equal(t, "town", manager.policy.GetCurrentTrack())
}

func TestMusicPlaybackResumesPositionAfterSameThemePreview(t *testing.T) {
	manager, backend, _ := gameAudioFixture(t)
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	require.NoError(t, backend.players[0].SetPosition(20*time.Millisecond))
	manager.policy.SetSuppressed(MusicMenu, true)
	position := manager.policy.policy.position
	require.Equal(t, 20*time.Millisecond, position)
	require.NoError(t, manager.PlayMusicPreview("town"))
	manager.SetEnabled(false) // A paused preview can have exactly the same track ID.
	manager.SetEnabled(true)
	manager.policy.SetSuppressed(MusicMenu, false)
	manager.Update()
	require.Equal(t, position, manager.music.Snapshot().Position)
	require.Equal(t, "town", manager.policy.GetCurrentTrack())
}

func TestMusicPlaybackResumesPositionAfterPreviewIsClosed(t *testing.T) {
	manager, backend, _ := gameAudioFixture(t)
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	require.NoError(t, backend.players[0].SetPosition(20*time.Millisecond))
	manager.policy.SetSuppressed(MusicMenu, true)
	position := manager.policy.policy.position
	require.NoError(t, manager.PlayMusicPreview("dungeon"))
	require.NoError(t, manager.StopBackgroundMusic())
	manager.policy.SetSuppressed(MusicMenu, false)
	manager.Update()
	require.Equal(t, position, manager.music.Snapshot().Position)
	require.Equal(t, "town", manager.policy.GetCurrentTrack())
}

func TestMusicPolicyBardOwnershipIncludesReleaseTailAndExcludesMutedBard(t *testing.T) {
	manager, _, now := gameAudioFixture(t)
	policy, player, advance := newPolicyTest(t)
	manager.policy.policy = policy
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	definition := manager.planner.definitions[AmbientBard]
	source := manager.EmitterAmbientSource("bard", references.Position{}, 1)
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, []AmbientSoundSource{source})
	require.True(t, manager.hasLocalMusic(), "initial fade-in already owns local music")
	manager.Update()
	require.False(t, player.playing)
	*now = now.Add(time.Second)
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, []AmbientSoundSource{source})
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, nil)
	require.True(t, manager.hasLocalMusic(), "release tail still owns music")
	require.NoError(t, manager.SetAmbientVolume(0))
	require.False(t, manager.hasLocalMusic())
	*now = now.Add(definition.FadeOutTime)
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, nil)
	manager.Update()
	require.False(t, manager.hasLocalMusic())
	advance(musicLocalRelease)
	advance(0)
	require.True(t, player.playing)
}

func TestMusicSchedulingConcurrentCategoryChangesAndDisable(t *testing.T) {
	manager, _, _ := gameAudioFixture(t)
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	workers := []func() error{
		func() error {
			for range 100 {
				manager.Update()
				if err := manager.PlayMusicForLocation(references.Minoc); err != nil {
					return err
				}
			}
			return nil
		},
		func() error {
			for index := range 100 {
				if err := manager.SetMusicVolume(float64(index%3) / 4); err != nil {
					return err
				}
				manager.SetEnabled(index%2 == 0)
			}
			return nil
		},
		func() error {
			for range 10 {
				if err := manager.PlayMusicPreview("town"); err != nil {
					return err
				}
			}
			return nil
		},
	}
	results := make(chan error, len(workers))
	var group sync.WaitGroup
	for _, worker := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- worker()
		}()
	}
	group.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
}

func TestCombatMusicBypassesExplorationQuietAndRepeatHistory(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	policy.history.quietRemaining = musicQuietInterval
	policy.history.heard[musicCombatTrack] = true // Previously heard in Jhelom.
	policy.setCombatEncounter(1)
	advance(0)
	require.Equal(t, []string{musicCombatTrack}, player.starts)
	advance(musicFadeDuration)
	require.InDelta(t, math.Pow(10, musicCombatDB/20), player.volume, 1e-9)
	for range 20 {
		policy.setCombatEncounter(1) // New turns/frames are not new encounters.
		advance(time.Second)
	}
	require.Len(t, player.starts, 1)
	player.playing = false
	advance(0)
	advance(time.Hour)
	require.Len(t, player.starts, 1, "neither combat opening nor town theme restarts during battle")
	policy.setCombatEncounter(2)
	advance(0)
	require.Equal(t, []string{musicCombatTrack, musicCombatTrack}, player.starts)
}

func TestCombatMusicReplacesScoreAndRestoresCurrentExploration(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	advance(0)
	advance(musicFadeDuration)
	policy.setCombatEncounter(1)
	advance(musicFadeDuration / 2)
	require.Equal(t, "town", player.track, "old score fades instead of stacking")
	advance(musicFadeDuration / 2)
	advance(0)
	require.Equal(t, musicCombatTrack, player.track, "combat bypasses the transition quiet interval")
	advance(musicFadeDuration)
	policy.setCombatEncounter(0)
	policy.exploration = "dungeon" // Exit need not return to the original location.
	advance(musicFadeDuration)
	require.False(t, player.playing)
	advance(musicQuietInterval - time.Nanosecond)
	require.Len(t, player.starts, 2)
	advance(time.Nanosecond)
	require.Equal(t, []string{"town", musicCombatTrack, "dungeon"}, player.starts)
	require.False(t, policy.history.heard[musicCombatTrack], "combat does not spend Jhelom exploration history")
}

func TestCombatVictoryStopsScoreAndKeepsBattlefieldQuiet(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	policy.setCombatEncounter(1)
	advance(0)
	advance(musicFadeDuration)
	require.Equal(t, []string{musicCombatTrack}, player.starts)

	policy.setCombatVictory(true)
	advance(0)
	require.Equal(t, []string{"play combat", "stop combat"}, player.events,
		"victory stops the score on the next update, without waiting for a fade")
	require.False(t, player.playing)

	policy.setSuppressed(MusicMenu, true)
	policy.cue = "victory" // A later special request cannot revive the score on the battlefield.
	for range 3 {
		policy.setCombatEncounter(1)
		policy.setCombatVictory(true)
		advance(musicQuietInterval)
	}
	policy.setSuppressed(MusicMenu, false)
	advance(musicQuietInterval)
	require.Equal(t, []string{musicCombatTrack}, player.starts, "looting and menus stay quiet")

	policy.setCombatEncounter(0)
	advance(musicQuietInterval)
	require.Equal(t, []string{musicCombatTrack, "town"}, player.starts)
	policy.setCombatEncounter(2)
	advance(musicFadeDuration)
	advance(0)
	require.Equal(t, musicCombatTrack, player.track, "a new encounter can play its own score")
}

func TestCombatVictoryUsesShortSoundInsteadOfMusicRecording(t *testing.T) {
	cue := productionLibrary().Cues["sfx/SoundCombatVictory"]
	require.Equal(t, "combat_victory_01", cue.RecordingID)
	require.NotEmpty(t, productionLibrary().recordingPath(cue.RecordingID))
	require.Equal(t, "fanfare_for_the_virtuous", productionLibrary().Cues["music/track/victory"].RecordingID)
}

func TestCombatVictoryBeforeScoreStartsOrAfterItEnds(t *testing.T) {
	for _, finished := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "completed"}[finished], func(t *testing.T) {
			policy, player, advance := newPolicyTest(t)
			policy.exploration = "town"
			policy.setCombatEncounter(1)
			if finished {
				advance(0)
				player.playing = false
				advance(0)
			}
			policy.setCombatVictory(true)
			advance(0)
			advance(time.Hour)
			require.False(t, player.playing)
			if finished {
				require.Equal(t, []string{musicCombatTrack}, player.starts)
			} else {
				require.Empty(t, player.starts)
			}
		})
	}
}

func TestCombatVictoryStopsScoreWhileMenuOwnsInterruption(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.setCombatEncounter(1)
	advance(0)
	policy.setSuppressed(MusicMenu, true)
	policy.setCombatVictory(true)
	advance(0)
	policy.setSuppressed(MusicMenu, false)
	advance(0)
	require.Equal(t, []string{"play combat", "stop combat"}, player.events)
	require.Empty(t, player.resumes)
}

func TestCombatMusicPendingAndActiveRespectInterruptionOwners(t *testing.T) {
	policy, player, advance := newPolicyTest(t)
	policy.exploration = "town"
	policy.history.quietRemaining = musicQuietInterval
	policy.setSuppressed(musicWorkflow, true) // Camping presentation before ambush handoff.
	policy.setCombatEncounter(1)
	advance(time.Minute)
	require.Empty(t, player.starts)
	policy.setSuppressed(musicWorkflow, false)
	advance(0)
	player.position = 30 * time.Second
	policy.setSuppressed(MusicMenu, true)
	advance(time.Hour)
	policy.setSuppressed(MusicMenu, false)
	advance(0)
	require.Equal(t, []string{musicCombatTrack}, player.starts)
	require.Equal(t, []time.Duration{30 * time.Second}, player.resumes)
	policy.setSuppressed(musicMuted, true)
	policy.setCombatEncounter(0)
	policy.exploration = "dungeon"
	advance(time.Minute)
	policy.setSuppressed(musicMuted, false)
	advance(0)
	require.Len(t, player.resumes, 1, "combat cannot resume after exit while muted")
	advance(musicQuietInterval)
	require.Equal(t, "dungeon", player.track)
}

func TestCombatMusicMissingOrFailedDoesNotRetryEachFrame(t *testing.T) {
	for _, failure := range []error{nil, errors.New("combat playback failed")} {
		policy, player, _ := newPolicyTest(t)
		player.empty, player.fail = true, failure
		policy.exploration = "town"
		policy.setCombatEncounter(1)
		require.ErrorIs(t, policy.update(), failure)
		for range 60 {
			policy.setCombatEncounter(1)
			require.NoError(t, policy.update())
		}
		require.Equal(t, []string{musicCombatTrack}, player.starts)
	}
}
