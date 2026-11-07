package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
)

// Jimmy uses the first party member in Good status; with everyone dead nobody
// can try.
func TestSelectCharacterForJimmy(t *testing.T) {
	allDead := map[int]party_state.CharacterStatus{}
	for i := range 6 {
		allDead[i] = party_state.Dead
	}
	for _, test := range []struct {
		name     string
		statuses map[int]party_state.CharacterStatus
		want     int
	}{
		{name: "first good", statuses: map[int]party_state.CharacterStatus{0: party_state.Good, 1: party_state.Dead}, want: 0},
		{name: "first dead, second good", statuses: map[int]party_state.CharacterStatus{0: party_state.Dead, 1: party_state.Good}, want: 1},
		{name: "all dead", statuses: allDead, want: -1},
	} {
		gs := &GameState{}
		gs.PartyState = *party_state.NewBlankPartyState()
		for index, status := range test.statuses {
			gs.PartyState.GetCharacter(index).SetStatus(status)
		}

		got := gs.SelectCharacterForJimmy()
		if test.want < 0 {
			assert.Nil(t, got, test.name)

			continue
		}
		assert.Same(t, gs.PartyState.GetCharacter(test.want), got, test.name)
	}
}
