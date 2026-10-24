package audio

import (
	"fmt"
	"io"
	"math"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	berad "github.com/bradhannah/berad-sdk/modules/berad-audio"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestLiveAmbientProjectionAppliesCalibrationAndWindowExactlyOnce(t *testing.T) {
	// Start from the normal synthetic project, then preserve the authored
	// calibration/window through the actual manager's library and planner.
	base, _, _ := gameAudioFixture(t)
	document := base.library.Document()
	cue := document.Cues[string(AmbientFire)]
	cue.VolumeModifier = 0.5
	document.Cues[string(AmbientFire)] = cue
	recording := document.Recordings[cue.RecordingID]
	recording.CalibrationGainDB = new(6.0)
	recording.PlaybackWindow = &PlaybackWindow{StartMs: 20, EndMs: 40}
	document.Recordings[cue.RecordingID] = recording
	document.Routing.Ambient[string(AmbientFire)] = AmbientProfile{RadiusTiles: 8, FadeInMs: 1000, FadeOutMs: 1000}
	key := tileAudioKey(indexes.FireLogs)
	assignment := document.Routing.Tiles[key]
	assignment.AmbientCue, assignment.Gain = string(AmbientFire), new(2.0)
	document.Routing.Tiles[key] = assignment
	pcm := editorTestWAV()
	library, err := NewLibrary(encodeAudioValue(document), fstest.MapFS{*recording.File: {Data: pcm}},
		LibraryOptions{Validate: ReduxLibraryValidator})
	require.NoError(t, err)
	manager, backend, now := gameAudioLibraryFixture(t, library)
	tiles := []TileWithPosition{{Tile: &references.Tile{Index: indexes.FireLogs}},
		{Tile: &references.Tile{Index: indexes.FireLogs}, Position: references.Position{X: 4}}}
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, tiles, nil)
	*now = now.Add(time.Second)
	manager.Update()
	require.Len(t, backend.players, 1, "strongest static fire owns one calibrated voice")
	want := math.Pow(10, 6.0/20) // 0.5 cue * 2 source; calibration applies exactly once.
	require.InDelta(t, want, manager.ambience.Snapshot().Sources[0].Voice.Gain, 1e-9)
	windowBytes := EditorSampleRate * decodedPCMBytesPerFrame * 20 / 1000
	window, err := io.ReadAll(io.LimitReader(backend.players[0].source, int64(windowBytes*2)))
	require.NoError(t, err)
	require.Len(t, window, windowBytes*2)
	require.NotEqual(t, make([]byte, windowBytes), window[:windowBytes], "the selected source window is audible")
	require.Equal(t, window[:windowBytes], window[windowBytes:], "ambient loop uses the authored excerpt")
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{X: 4}, tiles[1:], nil)
	require.Len(t, backend.players, 1, "a new strongest tile retains the source identity")
	require.NoError(t, manager.SetAmbientVolume(0.5))
	require.InDelta(t, want*0.5, manager.ambience.Snapshot().Sources[0].Voice.Gain, 1e-9)
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, nil)
	*now = now.Add(time.Second)
	manager.Update()
	require.False(t, backend.players[0].IsPlaying())
	require.Empty(t, manager.ambience.Snapshot().Sources)
}

func dynamicFixtureSource(kind AmbientSoundType, id string) AmbientSoundSource {
	return AmbientSoundSource{ID: id, SoundType: kind, VolumeModifier: 1}
}

func TestLiveAmbientAdapterPreservesStableIDsAndBudget(t *testing.T) {
	manager, backend, now := gameAudioFixture(t)
	sources := make([]AmbientSoundSource, 32)
	for index := range sources {
		sources[index] = dynamicFixtureSource(AmbientFire, fmt.Sprintf("source/%03d", index))
	}
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, sources)
	require.Len(t, backend.players, AmbientVoiceLimit)
	slices.Reverse(sources)
	*now = now.Add(time.Second)
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, sources)
	manager.Update()
	require.Len(t, backend.players, AmbientVoiceLimit, "ordering alone cannot replace admitted sources")
	require.NoError(t, manager.SetAmbientVolume(0))
	manager.Update()
	require.Len(t, backend.players, AmbientVoiceLimit)
	for _, source := range manager.ambience.Snapshot().Sources {
		require.Zero(t, source.Voice.Gain)
	}
	manager.SetAmbientSuppressed(true)
	for _, player := range backend.players {
		require.False(t, player.IsPlaying())
	}
	manager.SetAmbientSuppressed(false)
	manager.Update()
	require.Empty(t, manager.ambience.Snapshot().Sources, "restoration requires fresh world discovery")
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, sources[:1])
	initial := len(backend.players)
	sources[0].Position.X = 255
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{X: 255}, nil, sources[:1])
	require.Len(t, backend.players, initial, "projected seam coordinates do not change source identity")
}

func TestFountainFramesAndStrongestFireRetainLiveVoices(t *testing.T) {
	manager, backend, now := gameAudioDocumentFixture(t, func(document *LibraryDocument) {
		for _, tile := range []struct {
			index        indexes.SpriteIndex
			radius, gain float64
		}{
			{indexes.ItemTorch, 2, 0.25}, {indexes.Brazier, 8, 0.8},
		} {
			assignment := document.Routing.Tiles[tileAudioKey(tile.index)]
			assignment.AmbientCue = string(AmbientFire)
			assignment.RadiusTiles, assignment.Gain = new(tile.radius), new(tile.gain)
			document.Routing.Tiles[tileAudioKey(tile.index)] = assignment
		}
	})
	for frame := indexes.Fountain1; frame <= indexes.Fountain4; frame++ {
		manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, []TileWithPosition{
			{Tile: &references.Tile{Index: indexes.SpriteIndex(frame)}},
			{Tile: &references.Tile{Index: indexes.Fountain1}, Position: references.Position{X: 2}},
		}, nil)
		*now = now.Add(200 * time.Millisecond)
		manager.Update()
		require.Len(t, backend.players, 1, "canonical animation family owns one voice")
	}
	manager.SetAmbientSuppressed(true)
	manager.SetAmbientSuppressed(false)
	tiles := []TileWithPosition{
		{Tile: &references.Tile{Index: indexes.ItemTorch}, Position: references.Position{X: 1}},
		{Tile: &references.Tile{Index: indexes.Brazier}, Position: references.Position{X: 4}},
	}
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, tiles, nil)
	plans, err := manager.planner.Sources(references.Position{}, tiles, nil)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	require.InDelta(t, 0.8*0.75, plans[0].Gain, 1e-9, "4 of 8 tiles: 1 - (4/8)^2")
	count := len(backend.players)
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, tiles[:1], nil)
	require.Len(t, backend.players, count, "strongest static source changes gain, not voice identity")
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{X: 4}, tiles[:1], nil)
	*now = now.Add(time.Minute)
	manager.Update()
	require.False(t, backend.players[count-1].IsPlaying())
}

func TestLighthouseSharesTileVoiceAcrossMovementFloorAndExit(t *testing.T) {
	manager, backend, now := gameAudioFixture(t)
	definition, exists := manager.planner.definitions[AmbientLighthouse]
	require.True(t, exists)
	require.Equal(t, 10.0, definition.MaxDistance)
	require.Equal(t, "seagulls_01", productionLibrary().Cues[string(AmbientLighthouse)].RecordingID)
	tiles := []TileWithPosition{{Tile: &references.Tile{Index: indexes.SpriteIndex(27)}}}
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, tiles, nil)
	*now = now.Add(definition.FadeInTime)
	manager.Update()
	for _, location := range []references.Location{references.Fogsbane, references.Stormcrow, references.Greyhaven, references.Waveguide} {
		for floor := range 3 {
			listener := references.Position{X: references.Coordinate(floor * 7), Y: references.Coordinate(floor * 5)}
			tiles[0].Position = listener
			source, active := manager.LighthouseAmbientSource(location, listener)
			require.True(t, active)
			require.True(t, source.Background)
			manager.UpdateAmbientSoundsWithDynamicSources(listener, tiles, []AmbientSoundSource{source})
			manager.Update()
			require.Len(t, backend.players, 1)
			require.InDelta(t, definition.BaseVolume, manager.ambience.Snapshot().Sources[0].Voice.Gain, 1e-9)
		}
	}
	outside := tiles[0].Position
	outside.X += 5
	manager.UpdateAmbientSoundsWithDynamicSources(outside, tiles, nil)
	manager.Update()
	require.Len(t, backend.players, 1)
	require.InDelta(t, definition.BaseVolume*0.75, manager.ambience.Snapshot().Sources[0].Voice.Gain, 1e-9,
		"5 of 10 tiles: 1 - (5/10)^2")
	outside.X += 6
	manager.UpdateAmbientSoundsWithDynamicSources(outside, tiles, nil)
	*now = now.Add(definition.FadeOutTime)
	manager.Update()
	require.False(t, backend.players[0].IsPlaying())
	for _, location := range []references.Location{references.Britain, references.Deceit, references.BritanniaUnderworld} {
		_, active := manager.LighthouseAmbientSource(location, references.Position{})
		require.False(t, active)
	}
}

func TestDungeonAndNavalBedsUseGameRoutingAndLiveLifecycle(t *testing.T) {
	production, devices, _ := gameAudioLibraryFixture(t, defaultRuntimeLibrary())
	for _, test := range []struct {
		cue       string
		locations []references.Location
	}{
		{"ambient/dungeon_cave", []references.Location{references.Despise, references.Destard, references.Doom, references.Shame, references.Hythloth}},
		{"ambient/dungeon_brick", []references.Location{references.Deceit, references.Wrong, references.Covetous}},
	} {
		for _, location := range test.locations {
			source, exists := production.DungeonAmbientSource(location, references.Position{})
			require.True(t, exists)
			require.Equal(t, AmbientSoundType(test.cue), source.SoundType)
			require.Empty(t, production.planner.definitions[source.SoundType].FileName)
			production.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, []AmbientSoundSource{source})
		}
	}
	require.Empty(t, devices.players, "unfilled dungeon slots never open transports")
	for _, location := range []references.Location{references.BritanniaUnderworld, references.Britain, references.CombatLocation} {
		_, active := production.DungeonAmbientSource(location, references.Position{})
		require.False(t, active)
	}
	manager, backend, now := gameAudioDocumentFixture(t, func(document *LibraryDocument) {
		for _, id := range []string{"dungeon_cave_01", "dungeon_brick_01", "frigate_onboard_01", "skiff_onboard_01"} {
			recording := document.Recordings[id]
			recording.File = new("sounds/generated.wav")
			document.Recordings[id] = recording
		}
	})
	for _, source := range []struct {
		name    string
		resolve func(references.Position) (AmbientSoundSource, bool)
	}{
		{"cave", func(p references.Position) (AmbientSoundSource, bool) {
			return manager.DungeonAmbientSource(references.Despise, p)
		}},
		{"brick", func(p references.Position) (AmbientSoundSource, bool) {
			return manager.DungeonAmbientSource(references.Deceit, p)
		}},
		{"frigate", func(p references.Position) (AmbientSoundSource, bool) {
			return manager.NavalAmbientSource(NavalFrigate, p)
		}},
		{"skiff", func(p references.Position) (AmbientSoundSource, bool) {
			return manager.NavalAmbientSource(NavalSkiff, p)
		}},
	} {
		t.Run(source.name, func(t *testing.T) {
			bed, exists := source.resolve(references.Position{})
			require.True(t, exists)
			require.True(t, bed.Background)
			definition := manager.planner.definitions[bed.SoundType]
			manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, []AmbientSoundSource{bed})
			count := len(backend.players)
			*now = now.Add(definition.FadeInTime)
			manager.Update()
			for _, point := range []references.Position{{X: 7, Y: 7}, {X: 200, Y: 100}} {
				bed.Position = point
				manager.UpdateAmbientSoundsWithDynamicSources(point, nil, []AmbientSoundSource{bed})
				require.Len(t, backend.players, count)
			}
			require.NoError(t, manager.SetAmbientVolume(0))
			manager.Update()
			require.Zero(t, manager.ambience.Snapshot().Sources[0].Voice.Gain)
			require.NoError(t, manager.SetAmbientVolume(0.5))
			require.NoError(t, manager.SetSoundEffectVolume(0))
			manager.Update()
			require.InDelta(t, definition.BaseVolume*0.5, manager.ambience.Snapshot().Sources[0].Voice.Gain, 1e-9)
			manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, nil)
			require.Equal(t, berad.VoiceReleasing, manager.ambience.Snapshot().Sources[0].Voice.State)
			*now = now.Add(definition.FadeOutTime)
			manager.Update()
			require.False(t, backend.players[count-1].IsPlaying())
		})
	}
	_, active := manager.NavalAmbientSource("", references.Position{})
	require.False(t, active)
}
