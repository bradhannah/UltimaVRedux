package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestProductionDungeonAudioFollowsStyleFloorsAndCombatOwner(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).WithSystemCallbacks().WithRandomSeed(42).Build()
	scene := &GameScene{gameState: state}
	manager, err := audio.NewGameAudioManagerWithConfig(nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	for location := references.Deceit; location <= references.Doom; location++ {
		wallSet, valid := dungeonWallSetForLocation(location)
		require.True(t, valid)
		expected := audio.AmbientSoundType("ambient/dungeon_cave")
		if wallSet == 2 {
			expected = "ambient/dungeon_brick"
		}
		for floor := references.FloorNumber(0); floor < references.DungeonFloorCount; floor++ {
			state.MapState.PlayerLocation.Location = location
			state.MapState.PlayerLocation.Floor = floor
			state.MapState.PlayerLocation.Position = references.Position{X: 3, Y: 4}
			exploration, exists := manager.DungeonAmbientSource(scene.dungeonAudioLocation(), state.MapState.PlayerLocation.Position)
			require.True(t, exists)
			require.Equal(t, expected, exploration.SoundType)
			state.CombatScene = &game_state.CombatState{PreCombatPlayerLocation: state.MapState.PlayerLocation}
			state.MapState.PlayerLocation.Location = references.CombatLocation
			combat, exists := manager.DungeonAmbientSource(scene.dungeonAudioLocation(), references.Position{X: 10, Y: 10})
			require.True(t, exists)
			require.Equal(t, exploration.SoundType, combat.SoundType, "combat retains the persistent playback key")
			state.MapState.PlayerLocation = state.CombatScene.PreCombatPlayerLocation
			state.CombatScene = nil
			returned, exists := manager.DungeonAmbientSource(scene.dungeonAudioLocation(), state.MapState.PlayerLocation.Position)
			require.True(t, exists)
			require.Equal(t, exploration, returned)
		}
	}
	for _, exit := range []references.Location{references.BritanniaUnderworld, references.Britain} {
		state.MapState.PlayerLocation.Location = exit
		_, exists := manager.DungeonAmbientSource(scene.dungeonAudioLocation(), state.MapState.PlayerLocation.Position)
		require.False(t, exists)
	}
}
