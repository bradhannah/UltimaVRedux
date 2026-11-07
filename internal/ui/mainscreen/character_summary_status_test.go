package mainscreen

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCharacterSummary_StatusColorMapping(t *testing.T) {
	summary := &CharacterSummary{}
	for _, test := range []struct {
		status party_state.CharacterStatus
		want   color.Color
	}{
		{party_state.Poisoned, ucolor.From(ucolor.EGAGreen).RGBA()},
		{party_state.Sleep, ucolor.From(ucolor.EGAMagenta).RGBA()},
		{party_state.Charmed, ucolor.From(ucolor.EGABlue).RGBA()},
		{party_state.Dead, ucolor.From(ucolor.EGARed).RGBA()},
		{party_state.Good, nil},
	} {
		assert.Equal(t, test.want, summary.getStatusColor(test.status))
	}
}

func TestCharacterSummaryStatusNameUsesCompactPoisonLabel(t *testing.T) {
	character := party_state.NewBlankPartyState().GetCharacter(0)
	character.SetStatus(party_state.Poisoned)
	assert.Equal(t, "Pois.", characterSummaryStatusName(character))
}

func TestCharacterSummarySelectionUsesBrightnessWithoutStatusColorCollision(t *testing.T) {
	summary := &CharacterSummary{}
	subduedWhite := ucolor.From(ucolor.White).Intensity(inactivePlayerIntensity).RGBA()
	subduedPoison := ucolor.From(ucolor.EGAGreen).Intensity(inactivePlayerIntensity).RGBA()
	tests := []struct {
		name                                   string
		status                                 party_state.CharacterStatus
		active, hasActive, solo, soloCharacter bool
		want                                   color.Color
	}{
		{name: "round robin healthy", status: party_state.Good, want: ucolor.White},
		{name: "selected active healthy", status: party_state.Good, active: true, hasActive: true, want: ucolor.White},
		{name: "non-selected active healthy", status: party_state.Good, hasActive: true, want: subduedWhite},
		{name: "selected solo healthy", status: party_state.Good, solo: true, soloCharacter: true, want: ucolor.White},
		{name: "non-solo healthy", status: party_state.Good, solo: true, want: subduedWhite},
		{name: "selected solo poisoned", status: party_state.Poisoned, solo: true, soloCharacter: true, want: statusColors[party_state.Poisoned]},
		{name: "non-solo poisoned", status: party_state.Poisoned, solo: true, want: subduedPoison},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := summary.getEffectiveTextColor(test.status, test.active, test.hasActive, test.solo, test.soloCharacter)
			assert.Equal(t, test.want, got)
			if test.name == "non-solo healthy" {
				assert.NotEqual(t, ucolor.EGAGreen, got, "green remains reserved for poison")
			}
		})
	}
}

func TestCharacterSummary_StatusEffectOutlines(t *testing.T) {
	summary := &CharacterSummary{}
	for _, status := range []party_state.CharacterStatus{party_state.Poisoned, party_state.Sleep, party_state.Charmed, party_state.Dead} {
		assert.True(t, summary.shouldApplyStatusOutline(status))
	}
	assert.False(t, summary.shouldApplyStatusOutline(party_state.Good))
}

func TestCharacterSummary_StatusEffectsErrorHandling(t *testing.T) {
	summary := &CharacterSummary{}
	screen := ebiten.NewImage(1, 1)
	t.Cleanup(screen.Dispose)
	t.Run("InvalidStatusHandling", func(t *testing.T) {
		party := party_state.NewBlankPartyState()
		character := party.GetCharacter(0)
		character.PartyMap = party_state.PartyMapActive
		character.SetStatus(party_state.CharacterStatus(255))
		assert.Nil(t, summary.getStatusColor(character.GetStatus()))
		assert.Equal(t, ucolor.White, summary.getEffectiveTextColor(character.GetStatus(), false, false, false, false))
		assert.NotPanics(t, func() { summary.Draw(party, screen, -1, false, -1) })
	})
	t.Run("NilInputHandling", func(t *testing.T) {
		assert.NotPanics(t, func() { summary.Draw(nil, screen, -1, false, -1) })
		assert.NotPanics(t, func() { summary.Draw(party_state.NewBlankPartyState(), nil, -1, false, -1) })
	})
}
