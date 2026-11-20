package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestMixConfirmationActualReceipt(t *testing.T) {
	scene := &GameScene{}
	for _, test := range []struct {
		name  string
		added int
		stock uint16
		want  string
	}{
		{"room", 10, 10, "Done!\n\nAdded 10 In Lor. Stock: 10."},
		{"98", 1, 99, "Done!\n\nAdded 1 In Lor. Stock: 99.\n9 excess mixtures discarded."},
		{"99", 0, 99, "Done!\n\nAdded 0 In Lor. Stock: 99.\n10 excess mixtures discarded."},
	} {
		t.Run(test.name, func(t *testing.T) {
			outcome := game_state.MixOutcome{
				Success: true, Message: "Done!", SpellName: "In Lor",
				Quantity: 10, Added: test.added, Stock: test.stock,
			}
			assert.Equal(t, test.want, scene.buildMixConfirmationMessage(outcome))
			outcome.IsNewRecipe = true
			assert.Equal(t, test.want+"\n\nNew recipe memorized!", scene.buildMixConfirmationMessage(outcome))
		})
	}
	assert.Equal(t, "POISON!", scene.buildMixConfirmationMessage(game_state.MixOutcome{Message: "POISON!"}))
}

func TestMixFailureWorkflowShowsActualTrapWithoutFizzle(t *testing.T) {
	scene, state, _ := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	for reagent := references.SulfurAsh; reagent <= references.MandrakeRoot; reagent++ {
		state.PartyState.Inventory.Reagent.Set(reagent, 10)
	}
	var feedback []string
	state.SystemCallbacks.UI.ShowActionFeedback = func(message string, _ bool) {
		feedback = append(feedback, message)
	}
	pressDungeonInputKey(t, scene, ebiten.KeyM)
	waitForDungeonInputDialog(t, scene, state)
	// An Nox, then the Experiment tab, where Sulfurous Ash is added to spoil the recipe.
	for _, key := range []ebiten.Key{ebiten.KeyA, ebiten.KeyN, ebiten.KeyEnter, ebiten.KeyTab, ebiten.KeySpace,
		ebiten.Key1, ebiten.KeyEnter} {
		pressDungeonInputKey(t, scene, key)
	}
	drainDungeonInputWorkflowThroughUpdate(t, scene, state, "reagent_mixing_workflow")
	require.NotEmpty(t, feedback)
	assert.Regexp(t, `^(ACID!|POISON!|BOMB!|GAS!)\n`, feedback[0])
	for _, message := range feedback {
		assert.NotContains(t, message, "Fizzle!")
	}
}
