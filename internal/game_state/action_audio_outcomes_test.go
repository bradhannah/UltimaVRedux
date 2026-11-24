package game_state

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/stretchr/testify/require"
)

func TestJimmyAudioFollowsPhysicalUnlockAndBrokenKey(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).WithSystemCallbacks().Build()
	target := findDoorVisitSourceAtLocation(t, state, references.LordBritishsCastle, indexes.LockedDoor)
	state.TransitionToLocation(target.Location, target.Floor, target.Position.Down(), TransitionFullLocation, false)
	character := state.PartyState.GetCharacter(0)
	state.PartyState.Inventory.Provisions.Keys.Set(2)
	setJimmyOutcome(state, false)
	require.False(t, state.ActionJimmySmallMapWithCharacter(references.Up, character))
	require.Equal(t, []SoundEffect{SoundKeyBreak}, callbacks.SoundEffectsPlayed)
	require.Equal(t, uint16(1), state.PartyState.Inventory.Provisions.Keys.Get())

	setJimmyOutcome(state, true)
	require.True(t, state.ActionJimmySmallMapWithCharacter(references.Up, character))
	require.Equal(t, []SoundEffect{SoundKeyBreak, SoundUnlock}, callbacks.SoundEffectsPlayed)
	require.Equal(t, uint16(1), state.PartyState.Inventory.Provisions.Keys.Get())
}
