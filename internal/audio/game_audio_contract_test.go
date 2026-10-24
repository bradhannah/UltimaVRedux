package audio

import (
	"io"
	"math"
	"testing"
	"time"

	berad "github.com/bradhannah/berad-sdk/modules/berad-audio"
	ea "github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestReachRecordingPreservesAudibleProductionWindow(t *testing.T) {
	manager, backend, _ := gameAudioLibraryFixture(t, defaultRuntimeLibrary())
	require.NoError(t, manager.PlaySoundEffect(game_state.SoundCombatSwing))
	voice := manager.sfx.Snapshot()[0]
	require.Equal(t, "combat_swing_01", string(voice.Recording))
	pcm, err := io.ReadAll(backend.players[0].source)
	require.NoError(t, err)
	require.Len(t, pcm, (910-350)*EditorSampleRate*decodedPCMBytesPerFrame/1000)
	require.NotEqual(t, make([]byte, len(pcm)), pcm, "reach-weapon onset remains audibly populated")
}

func TestReduxAdapterUsesItsInjectedLibraryAcrossLanes(t *testing.T) {
	manager, _, now := gameAudioDocumentFixture(t, func(document *LibraryDocument) {
		for id, gain := range map[string]float64{"sfx/SoundHeal": 0.17, "music/track/town": 0.23, string(AmbientFire): 0.13} {
			cue := document.Cues[id]
			cue.VolumeModifier = gain
			document.Cues[id] = cue
		}
		document.Routing.Music[locationMusicKey(references.Britain)] = new("music/track/title")
	})
	require.NoError(t, manager.PlaySoundEffect(game_state.SoundHeal))
	require.InDelta(t, 0.17, manager.sfx.Snapshot()[0].Gain, 1e-9)
	require.NoError(t, manager.PlayMusicForLocation(references.Minoc))
	*now = now.Add(musicFadeDuration)
	manager.Update()
	require.InDelta(t, 0.23*math.Pow(10, musicInitialDB/20), manager.music.Snapshot().Voice.Gain, 1e-9)
	definition, exists := manager.planner.definitions[AmbientFire]
	require.True(t, exists)
	require.InDelta(t, 0.13, definition.BaseVolume, 1e-9)
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	require.Equal(t, "title", manager.policy.policy.exploration)
	require.Equal(t, "town", musicForLocation(references.Britain), "injected policy cannot mutate embedded defaults")
}

func TestLivePlaybackDiagnosticsAreDetachedAndDoNotService(t *testing.T) {
	manager, _, now := gameAudioFixture(t)
	require.NoError(t, manager.PlaySound(game_state.SoundPlaybackRequest{Effect: game_state.SoundHeal,
		Mode: game_state.SoundPlaybackLoop, Volume: 0.5, FadeInMs: 1000}))
	*now = now.Add(500 * time.Millisecond)
	manager.Update()
	before := manager.PlaybackSnapshot()
	require.Len(t, before.Voices, 1)
	require.Equal(t, "sfx/SoundHeal", before.Voices[0].CueID)
	require.Equal(t, "healing_01", before.Voices[0].RecordingID)
	require.InDelta(t, 0.5, before.Voices[0].Gain.Request, 1e-9)
	require.InDelta(t, 0.5, before.Voices[0].Gain.Envelope, 1e-9)
	*now = now.Add(time.Hour)
	require.Equal(t, before, manager.PlaybackSnapshot(), "capture cannot sample/service a later presentation time")
	before.Voices[0].RecordingID = "mutated view"
	require.Equal(t, "healing_01", manager.PlaybackSnapshot().Voices[0].RecordingID)
}

func TestGameAudioConfigurationAndBorrowedHost(t *testing.T) {
	configuration, err := NewAudioConfiguration()
	require.NoError(t, err)
	require.Equal(t, 0.43, configuration.SoundEffectVolume)
	require.Equal(t, 0.30, configuration.MusicVolume)
	require.Equal(t, 0.18, configuration.AmbientVolume)
	host := ea.CurrentContext()
	if host == nil {
		host = ea.NewContext(48000)
	}
	require.Same(t, host, getSharedAudioContext())
	manager, err := NewGameAudioManagerWithRuntime(configuration, host, func() time.Time { return time.Unix(0, 0) })
	require.NoError(t, err)
	require.NoError(t, manager.Close())
	require.Same(t, host, ea.CurrentContext(), "closing the game does not replace/close the host device")
	library := defaultRuntimeLibrary()
	manager, err = newGameAudioManager(library, configuration, new(gamePlaybackBackend), nil, time.Now)
	require.Error(t, err, "missing decoder is rejected at construction")
}

func TestGameAudioRequestValidationAndAdmissionOutcomes(t *testing.T) {
	manager, backend, _ := gameAudioFixture(t)
	for _, request := range []game_state.SoundPlaybackRequest{
		{Effect: game_state.SoundEffect(-1), Mode: game_state.SoundPlaybackOneShot, Volume: 1},
		{Effect: game_state.SoundHeal, Mode: game_state.SoundPlaybackOneShot, Volume: -1},
		{Effect: game_state.SoundHeal, Mode: game_state.SoundPlaybackOneShot, Volume: 1, FadeInMs: -1},
		{Effect: game_state.SoundHeal, Mode: game_state.SoundPlaybackMode(99), Volume: 1},
	} {
		require.ErrorIs(t, manager.PlaySound(request), berad.ErrInvalidRequest)
	}
	require.Empty(t, backend.players)
	for range 8 {
		require.NoError(t, manager.PlaySound(game_state.SoundPlaybackRequest{Effect: game_state.SoundEarthquake,
			Mode: game_state.SoundPlaybackLoop, Volume: 1}))
	}
	require.ErrorIs(t, manager.PlaySound(game_state.SoundPlaybackRequest{Effect: game_state.SoundEarthquake,
		Mode: game_state.SoundPlaybackLoop, Volume: 1}), berad.ErrVoiceLimit)
	require.NoError(t, manager.PlaySoundEffect(game_state.SoundHeal))
	require.Len(t, manager.sfx.Snapshot(), 8, "dropped finite request is supported silence at the game boundary")
}

func TestRuntimeLanesShareMonotonicPresentationTime(t *testing.T) {
	manager, _, now := gameAudioDocumentFixture(t, func(document *LibraryDocument) {
		hum := document.Routing.Ambient[document.Routing.Emitters["moongate"]]
		hum.FadeInMs = 1000 // Match the SFX fade so both lanes reach half together.
		document.Routing.Ambient[document.Routing.Emitters["moongate"]] = hum
	})
	start := *now
	require.NoError(t, manager.PlaySound(game_state.SoundPlaybackRequest{Effect: game_state.SoundEarthquake,
		Mode: game_state.SoundPlaybackLoop, Volume: 1, FadeInMs: 1000}))
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	source := manager.EmitterAmbientSource("moongate", references.Position{}, 1)
	source.ID = "gate"
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, []AmbientSoundSource{source})
	*now = now.Add(500 * time.Millisecond)
	manager.Update()
	for range 2 {
		require.InDelta(t, 0.5, manager.sfx.Snapshot()[0].Envelope, 1e-9)
		require.InDelta(t, 0.5, manager.ambience.Snapshot().Sources[0].Voice.Envelope, 1e-9)
		require.InDelta(t, 0.25, manager.policy.policy.envelope, 1e-9)
		*now = now.Add(-time.Hour)
		manager.Update()
	}
	*now = start.Add(time.Hour)
	manager.Update()
	require.Equal(t, 1.0, manager.sfx.Snapshot()[0].Envelope)
	manager.StopSound(game_state.SoundStopRequest{Effect: game_state.SoundEarthquake, FadeOutMs: 1000})
	*now = now.Add(24 * time.Hour)
	manager.Update()
	require.Empty(t, manager.sfx.Snapshot())
}

func TestWalkingSurfaceSelectionUsesLiveAdapterAndUnboundedSequence(t *testing.T) {
	manager, _, _ := gameAudioFixture(t)
	for _, test := range []struct {
		tile    indexes.SpriteIndex
		effects []game_state.SoundEffect
	}{
		{indexes.Forest1, []game_state.SoundEffect{game_state.SoundWalkGrassLeft, game_state.SoundWalkGrassRight}},
		{indexes.BrickFloor, []game_state.SoundEffect{game_state.SoundWalkStone1, game_state.SoundWalkStone2, game_state.SoundWalkStone3, game_state.SoundWalkStone4}},
		{indexes.WoodFloor1, []game_state.SoundEffect{game_state.SoundWalkWood1, game_state.SoundWalkWood2, game_state.SoundWalkWood3}},
	} {
		manager.steps = 0
		for index := range len(test.effects) * 2 {
			require.NoError(t, manager.PlayWalkingSound(&references.Tile{Index: test.tile}))
			voices := manager.sfx.Snapshot()
			require.Equal(t, "sfx/"+catalogEffectNames()[test.effects[index%len(test.effects)]], string(voices[len(voices)-1].Cue))
			manager.StopSound(game_state.SoundStopRequest{Effect: test.effects[index%len(test.effects)]})
		}
	}
	manager.steps = 999
	require.NoError(t, manager.PlayWalkingSound(&references.Tile{Index: indexes.WoodFloor1}))
	require.EqualValues(t, 1000, manager.steps)
	require.NoError(t, manager.PlayWalkingSound(nil))
}

func TestFireBurnRetriggersAndReleasesWithoutStoppingAmbientFire(t *testing.T) {
	manager, _, now := gameAudioFixture(t)
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, []TileWithPosition{{Tile: &references.Tile{Index: indexes.FireLogs}}}, nil)
	*now = now.Add(time.Second)
	manager.Update()
	ambient := manager.ambience.Snapshot().Sources[0].ID
	require.NoError(t, manager.PlaySound(game_state.SoundPlaybackRequest{Effect: game_state.SoundFireCrackle,
		Mode: game_state.SoundPlaybackLoop, Volume: 1}))
	for range 12 {
		require.NoError(t, manager.PlaySoundEffect(game_state.SoundFireBurn))
	}
	require.Len(t, manager.sfx.Snapshot(), 2)
	*now = now.Add(900 * time.Millisecond)
	manager.Update()
	for _, voice := range manager.sfx.Snapshot() {
		if voice.Cue == "sfx/SoundFireBurn" {
			require.InDelta(t, 0.5, voice.Envelope, 1e-9)
		}
	}
	*now = now.Add(100 * time.Millisecond)
	manager.Update()
	require.Len(t, manager.sfx.Snapshot(), 1)
	require.Equal(t, berad.CueID("sfx/SoundFireCrackle"), manager.sfx.Snapshot()[0].Cue)
	require.Equal(t, ambient, manager.ambience.Snapshot().Sources[0].ID)
	_, options, err := gamePlaybackOptions(game_state.SoundPlaybackRequest{Effect: game_state.SoundFireBurn,
		Mode: game_state.SoundPlaybackOneShot, Volume: 1, FadeInMs: 1600})
	require.NoError(t, err)
	require.Equal(t, 1600*time.Millisecond, options.FadeIn)
	require.Equal(t, 200*time.Millisecond, options.ReleaseTail)
	library := productionLibrary()
	burn := library.Recordings[library.Cues["sfx/SoundFireBurn"].RecordingID]
	bed := library.Recordings[library.Cues[string(AmbientFire)].RecordingID]
	require.Equal(t, bed.File, burn.File)
	require.Nil(t, bed.PlaybackWindow)
	require.Equal(t, &PlaybackWindow{StartMs: 0, EndMs: 1000}, burn.PlaybackWindow)
}
