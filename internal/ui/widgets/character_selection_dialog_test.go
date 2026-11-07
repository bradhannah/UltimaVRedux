package widgets

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestCharacterSelectionDialogInspectsEveryRowAndConfirmsByState(t *testing.T) {
	selected := 99
	dialog := newCharacterSelectionTestDialog(characterSelectionTestOptions(), func(index int) { selected = index })

	require.Equal(t, 0, dialog.GetCurrentSelection())
	dialog.HandleKeyDown()
	assert.Equal(t, 1, dialog.GetCurrentSelection(), "unavailable row remains inspectable")
	dialog.HandleEnter()
	assert.Equal(t, 99, selected, "unavailable row cannot confirm")
	dialog.HandleKeyDown()
	assert.Equal(t, 2, dialog.GetCurrentSelection())
	dialog.HandleEnter()
	assert.Equal(t, 2, selected, "warning row remains confirmable")
}

func TestCharacterSelectionDialogArrowAndNumberInspectionWrapsWithoutConfirming(t *testing.T) {
	selected := 99
	options := append(characterSelectionTestOptions(), CharacterSelectionOption{
		CharacterIndex: -1, PartyPosition: 0, Name: "No One", PortraitSprite: indexes.NoSprites,
		State: CharacterSelectionAvailable,
	})
	dialog := newCharacterSelectionTestDialog(options, func(index int) { selected = index })

	dialog.HandleKeyUp()
	assert.Equal(t, -1, dialog.GetCurrentSelection())
	dialog.HandleNumber(2)
	assert.Equal(t, 1, dialog.GetCurrentSelection())
	dialog.HandleNumber(0)
	assert.Equal(t, -1, dialog.GetCurrentSelection())
	assert.Equal(t, 99, selected)
	dialog.HandleEnter()
	assert.Equal(t, -1, selected)
}

func TestCharacterSelectionDialogCancellationAndDetachedOptions(t *testing.T) {
	cancelled := false
	options := characterSelectionTestOptions()
	dialog := NewCharacterSelectionDialog(CharacterSelectionConfig{
		Title:       "Choose",
		Options:     options,
		OnSelect:    func(int) {},
		OnCancel:    func() { cancelled = true },
		Keyboard:    newWidgetTestKeyboard(0),
		SpriteSheet: &sprites.SpriteSheet{},
		Placement:   *createMockPlacement(),
	})
	options[0].Name = "Changed"
	options[0].Facts[0].Value = "Changed"
	options[0].StatusKind = party_state.Poisoned

	assert.Equal(t, "Avatar", dialog.GetOptions()[0].Name)
	assert.Equal(t, "10/20", dialog.GetOptions()[0].Facts[0].Value)
	assert.Equal(t, party_state.Good, dialog.GetOptions()[0].StatusKind)
	dialog.HandleEscape()
	assert.True(t, cancelled)
}

func TestCharacterSelectionDialogUpdateHandlesNumberAndEnterSeparately(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	selected := -1
	dialog := NewCharacterSelectionDialog(CharacterSelectionConfig{
		Title:       "Choose",
		Options:     characterSelectionTestOptions(),
		OnSelect:    func(index int) { selected = index },
		OnCancel:    func() {},
		Keyboard:    keyboard,
		SpriteSheet: &sprites.SpriteSheet{},
		Placement:   *createMockPlacement(),
	})
	keyboard.SetForceWaitAnyKey(0)
	keyboard.SimulateKeyPress(ebiten.Key3)
	dialog.Update()
	assert.Equal(t, 2, dialog.GetCurrentSelection())
	assert.Equal(t, -1, selected)
	keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	assert.Equal(t, 2, selected)
}

func TestCharacterSelectionDialogLayoutStaysWithinPlacementAndPreservesInspection(t *testing.T) {
	setResponsiveDialogTestResolution(t, 320, 200)
	dialog := newCharacterSelectionTestDialog(characterSelectionTestOptions(), func(int) {})
	dialog.HandleNumber(2)
	dialog.shell.ensureLayout(320, 200, len(dialog.GetOptions()), dialog.footer())
	workspace := sprites.GetRectangleFromPercents(*createMockPlacement())
	panel := dialog.shell.layout.panelRect
	assert.True(t, panel.In(*workspace))
	assert.Equal(t, 1, dialog.GetCurrentSelection())

	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog.shell.ensureLayout(1280, 720, len(dialog.GetOptions()), dialog.footer())
	assert.Equal(t, 1, dialog.GetCurrentSelection())
	assert.NotEqual(t, panel, dialog.shell.layout.panelRect)
}

func newCharacterSelectionTestDialog(options []CharacterSelectionOption, onSelect func(int)) *CharacterSelectionDialog {
	return NewCharacterSelectionDialog(CharacterSelectionConfig{
		Title:       "Choose",
		Options:     options,
		OnSelect:    onSelect,
		OnCancel:    func() {},
		Keyboard:    newWidgetTestKeyboard(0),
		SpriteSheet: &sprites.SpriteSheet{},
		Placement:   *createMockPlacement(),
	})
}

func characterSelectionTestOptions() []CharacterSelectionOption {
	return []CharacterSelectionOption{
		{
			CharacterIndex: 0, PartyPosition: 1, Name: "Avatar", Status: "Good", StatusKind: party_state.Good,
			PortraitSprite: indexes.AvatarKeyIndex, State: CharacterSelectionAvailable,
			Facts: []CharacterSelectionFact{{Label: "HP", Value: "10/20"}},
		},
		{
			CharacterIndex: 1, PartyPosition: 2, Name: "Iolo", Status: "Dead", StatusKind: party_state.Dead,
			PortraitSprite: indexes.BardKeyIndex, State: CharacterSelectionUnavailable,
			Details: []string{"Dead characters cannot act."},
		},
		{
			CharacterIndex: 2, PartyPosition: 3, Name: "Shamino", Status: "Good", StatusKind: party_state.Good,
			PortraitSprite: indexes.FighterKeyIndex, State: CharacterSelectionWarning,
			Details: []string{"This choice has no effect."},
		},
	}
}

func createMockPartyState() *party_state.PartyState {
	state := party_state.NewBlankPartyState()
	characters := []struct {
		name string
		role party_state.CharacterClass
	}{
		{name: "Avatar", role: party_state.Avatar},
		{name: "Iolo", role: party_state.Bard},
		{name: "Shamino", role: party_state.Fighter},
	}
	for index, fixture := range characters {
		character := state.GetCharacter(index)
		copy(character.Name[:], fixture.name)
		character.Class = fixture.role
		character.SetStatus(party_state.Good)
		character.PartyMap = party_state.PartyMapActive
	}

	return state
}

func createMockPlacement() *sprites.PercentBasedPlacement {
	return &sprites.PercentBasedPlacement{
		StartPercentX: 0,
		EndPercentX:   0.76,
		StartPercentY: 0,
		EndPercentY:   1,
	}
}
