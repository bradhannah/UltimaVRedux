package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestExperienceAwardWordingSaysWhoWasPaidAndWhy(t *testing.T) {
	party := game_state.DeedExperienceAward{Each: 150, Recipients: 6, Deeds: 1}
	assert.Equal(t, "+150 experience each", experienceAwardText(party))
	assert.Equal(t, "Experience +150 each", experienceDialogText(party))

	alone := game_state.DeedExperienceAward{Each: 150, Recipients: 1, Deeds: 1}
	assert.Equal(t, "+150 experience", experienceAwardText(alone), "a lone Avatar has no one to share with")
	assert.Equal(t, "Experience +150", experienceDialogText(alone))

	backPay := game_state.DeedExperienceAward{Each: 2645, Recipients: 6, Deeds: 34, BackPay: true}
	assert.Equal(t, "+2645 experience each for past deeds", experienceAwardText(backPay))
}

func TestExperienceDialogLinesColourTheAwardAndNameAWaitingLevel(t *testing.T) {
	assert.Empty(t, experienceDialogLines(game_state.DeedExperienceAward{}), "no award adds no line")

	lines := experienceDialogLines(game_state.DeedExperienceAward{
		Each: 20, Recipients: 3, Deeds: 1,
		Ready: []game_state.ExperienceLevelReady{{Name: "Jaana", Level: 5}},
	})
	if assert.Len(t, lines, 2) {
		assert.Equal(t, "Experience +20 each", lines[0].Text)
		assert.Equal(t, ucolor.ExperienceGain, lines[0].Color)
		assert.Equal(t, "Jaana is ready for level 5.", lines[1].Text)
		assert.Equal(t, ucolor.ExperienceLevelReady, lines[1].Color)
	}
}
