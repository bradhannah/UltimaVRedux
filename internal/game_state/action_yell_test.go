package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActionYellSmallMap_WordsOfPower(t *testing.T) {
	tests := []struct {
		name      string
		word      string
		expectMsg string
	}{
		{
			name:      "Word of Power FALLAX",
			word:      "FALLAX",
			expectMsg: "No effect!",
		},
		{
			name:      "Word of Power VERAMOCOR",
			word:      "VERAMOCOR",
			expectMsg: "No effect!",
		},
		{
			name:      "Sailing command while walking",
			word:      "HOIST",
			expectMsg: "Not on a ship!",
		},
		{
			name:      "Unknown word",
			word:      "RANDOM",
			expectMsg: "No effect!",
		},
		{
			name:      "Empty word",
			word:      "",
			expectMsg: "No effect!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := createTestGameState()
			mockCallbacks := NewMockSystemCallbacks(t)
			gs.SystemCallbacks = mockCallbacks.ToSystemCallbacks()

			result := gs.ActionYellSmallMap(tt.word)

			assert.False(t, result) // Words of Power don't work on small maps, sailing fails without ship
			assert.Equal(t, tt.expectMsg, mockCallbacks.Messages[0])
			assert.Empty(t, mockCallbacks.TimeAdvanced(), "the dialog workflow owns failed-Yell turn completion")
		})
	}
}

func TestActionYellWithoutEffect(t *testing.T) {
	gs := createTestGameState()
	mockCallbacks := NewMockSystemCallbacks(t)
	gs.SystemCallbacks = mockCallbacks.ToSystemCallbacks()

	for _, test := range []struct {
		word    string
		message string
	}{
		{message: "Nothing"},
		{word: "ANYTHING", message: "No effect!"},
	} {
		mockCallbacks.Reset()

		assert.True(t, gs.ActionYellWithoutEffect(test.word))
		assert.Equal(t, []string{test.message}, mockCallbacks.Messages)
		assert.Empty(t, mockCallbacks.TimeAdvanced())
	}
}

func TestActionYellMap_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name     string
		word     string
		testFunc func(*GameState, string) bool
		result   bool
	}{
		{
			name:     "Small map lowercase",
			word:     "fallax",
			testFunc: (*GameState).ActionYellSmallMap,
			result:   false,
		},
		{
			name:     "Combat map mixed case",
			word:     "FaLlAx",
			testFunc: (*GameState).ActionYellWithoutEffect,
			result:   true,
		},
		{
			name:     "Dungeon map with spaces",
			word:     " FALLAX ",
			testFunc: (*GameState).ActionYellWithoutEffect,
			result:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := createTestGameState()
			mockCallbacks := NewMockSystemCallbacks(t)
			gs.SystemCallbacks = mockCallbacks.ToSystemCallbacks()

			result := tt.testFunc(gs, tt.word)

			// All should handle the word processing correctly.
			assert.Equal(t, tt.result, result)
			assert.True(t, len(mockCallbacks.Messages) > 0)
		})
	}
}
