package widgets

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

func TestNewOrderAvatarAndLockedRowsRemainInspectableButCannotConfirm(t *testing.T) {
	dialog := createNewOrderTestDialog(createMockPartyState(), 1, func(int, int) {}, func() {})
	dialog.HandleNumber(1)
	require.Equal(t, 0, dialog.GetCurrentSelectionIndex())
	dialog.HandleEnter()
	assert.Equal(t, Stage1SelectFirst, dialog.GetStage())

	dialog.HandleNumber(2)
	dialog.HandleEnter()
	require.Equal(t, Stage2SelectSecond, dialog.GetStage())
	dialog.HandleNumber(2)
	assert.Equal(t, 1, dialog.GetCurrentSelectionIndex())
	option := newOrderOptionByIndex(t, dialog.GetOptions(), 1)
	assert.Equal(t, CharacterSelectionLocked, option.State)
	dialog.HandleEnter()
	assert.False(t, dialog.IsComplete())
}

func TestNewOrderAllCompanionStatusesRemainSwappable(t *testing.T) {
	state := createMockPartyState()
	state.GetCharacter(1).SetStatus(party_state.Dead)
	state.GetCharacter(2).SetStatus(party_state.Sleep)
	dialog := createNewOrderTestDialog(state, -1, func(int, int) {}, func() {})

	assert.Equal(t, CharacterSelectionAvailable, newOrderOptionByIndex(t, dialog.GetOptions(), 1).State)
	assert.Equal(t, CharacterSelectionAvailable, newOrderOptionByIndex(t, dialog.GetOptions(), 2).State)
	assert.Equal(t, party_state.Dead, newOrderOptionByIndex(t, dialog.GetOptions(), 1).StatusKind)
	assert.Equal(t, party_state.Sleep, newOrderOptionByIndex(t, dialog.GetOptions(), 2).StatusKind)
	dialog.HandleEnter()
	assert.Equal(t, 1, dialog.GetFirstSelectedIndex())
	assert.Equal(t, party_state.Dead, newOrderOptionByIndex(t, dialog.GetOptions(), 1).StatusKind,
		"locking a row preserves its status cue")
}

func TestNewOrderShowsExactSwapAndActiveSlotPreview(t *testing.T) {
	state := createMockPartyState()
	dialog := createNewOrderTestDialog(state, 1, func(int, int) {}, func() {})
	assert.True(t, newOrderOptionByIndex(t, dialog.GetOptions(), 1).Active)
	dialog.HandleNumber(2)
	dialog.HandleEnter()
	dialog.HandleNumber(3)

	option := newOrderOptionByIndex(t, dialog.GetOptions(), 2)
	assert.Equal(t, []CharacterSelectionFact{
		{Label: "Iolo", Value: "2 -> 3"},
		{Label: "Shamino", Value: "3 -> 2"},
	}, option.Facts)
	assert.Contains(t, option.Details[0], "Shamino is now the active player")
}

func TestNewOrderSwapSpaceAndEscapeBehavior(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	var first, second int
	cancelled := false
	dialog := NewNewOrderDialog(
		createMockPartyState(),
		-1,
		func(a, b int) { first, second = a, b },
		func() { cancelled = true },
		keyboard,
		&sprites.SpriteSheet{},
		*createMockPlacement(),
	)
	keyboard.SetForceWaitAnyKey(0)
	keyboard.SimulateKeyPress(ebiten.KeySpace)
	dialog.Update()
	require.Equal(t, Stage2SelectSecond, dialog.GetStage())
	dialog.HandleEscape()
	assert.Equal(t, Stage1SelectFirst, dialog.GetStage())
	assert.False(t, cancelled)

	dialog.HandleEnter()
	dialog.HandleEnter()
	assert.True(t, dialog.IsComplete())
	assert.ElementsMatch(t, []int{1, 2}, []int{first, second})

	cancelDialog := createNewOrderTestDialog(createMockPartyState(), -1, func(int, int) {}, func() { cancelled = true })
	cancelDialog.HandleEscape()
	assert.True(t, cancelled)
	assert.True(t, cancelDialog.IsComplete())
}

func TestNewOrderArrowNavigationIncludesAvatar(t *testing.T) {
	dialog := createNewOrderTestDialog(createMockPartyState(), -1, func(int, int) {}, func() {})
	require.Equal(t, 1, dialog.GetCurrentSelectionIndex())
	dialog.HandleKeyUp()
	assert.Equal(t, 0, dialog.GetCurrentSelectionIndex())
	dialog.HandleKeyDown()
	assert.Equal(t, 1, dialog.GetCurrentSelectionIndex())
}

func createNewOrderTestDialog(
	partyState *party_state.PartyState,
	activeSlot int,
	swapCallback func(first, second int),
	cancelCallback func(),
) *NewOrderDialog {
	return NewNewOrderDialog(
		partyState,
		activeSlot,
		swapCallback,
		cancelCallback,
		newWidgetTestKeyboard(0),
		&sprites.SpriteSheet{},
		*createMockPlacement(),
	)
}

func newOrderOptionByIndex(
	t *testing.T,
	options []CharacterSelectionOption,
	characterIndex int,
) CharacterSelectionOption {
	t.Helper()
	for _, option := range options {
		if option.CharacterIndex == characterIndex {
			return option
		}
	}
	t.Fatalf("character %d not found", characterIndex)

	return CharacterSelectionOption{}
}
