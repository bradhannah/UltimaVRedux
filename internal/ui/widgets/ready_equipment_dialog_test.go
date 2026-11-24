package widgets

import (
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestReadyDecisionDetailsUseInventoryAndAuthoritativeHands(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	character := state.PartyState.GetAvatar()
	character.Strength = 30
	for slot := references.SlotHelm; slot <= references.SlotAmulet; slot++ {
		character.SetEquippedItem(slot, references.NoEquipment)
	}
	state.PartyState.Inventory.Equipment.Set(references.Bow, 1)
	state.PartyState.Inventory.Equipment.Set(references.MagicAxe, 1)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 38)
	dialog := NewReadyEquipmentDialog(newWidgetTestKeyboard(-1), &sprites.SpriteSheet{}, state, game_state.ReadyEquipmentRequest{})
	for _, equipment := range []references.Equipment{references.Bow, references.MagicAxe} {
		for index, item := range dialog.availableItems {
			if item.Equipment == equipment {
				dialog.itemListNav.SetIndex(index)
			}
		}
		dialog.populateDetailOutput()
		content := strings.Join(strings.Fields(dialog.detailOutput.GetOutputStr(true)), " ")
		require.Contains(t, content, "Range:")
		if equipment == references.Bow {
			require.Contains(t, content, "Arrows (38)")
			require.Contains(t, content, "Affects: Both hands")
		} else {
			require.Contains(t, content, "Affects: Hand 1")
			require.Contains(t, content, "Returns")
		}
	}
	// Overflow is read without triggering the existing Space/Enter equip action.
	dialog.detailOutput.Reset()
	dialog.detailOutput.AppendLine().White(strings.Repeat("Long equipment property ", 100)).WordWrap().Done()
	require.Greater(t, dialog.detailPageCount(), 1)
	require.True(t, dialog.detailContentRect().In(image.Rect(0, 0, 1280, 720)))
	require.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotWeapon))
}

func TestExplorationReadyDialogRetainsCharacterKeyboardNavigation(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	second := state.PartyState.GetCharacter(1)
	require.NotNil(t, second)
	second.PartyMap = party_state.PartyMapActive
	second.SetStatus(party_state.Good)
	second.SetHp(max(second.GetCurrentHp(), 1))
	state.PartyState.Inventory.Equipment.Set(references.Dagger, 1)
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewReadyEquipmentDialog(
		keyboard,
		&sprites.SpriteSheet{},
		state,
		game_state.ReadyEquipmentRequest{},
	)

	keyboard.SimulateKeyPress(ebiten.KeyRight)
	dialog.Update()

	assert.Equal(t, 1, dialog.GetSelectedCharacterIndex())
}

func TestReadyDialogListsAmmoAsNoOpAndKeepsDuplicateReadiedMarker(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	character := state.PartyState.GetCharacter(0)
	character.Strength = 30
	for equipment := references.LeatherHelm; equipment <= references.Ankh; equipment++ {
		state.PartyState.Inventory.Equipment.Set(equipment, 0)
	}
	for slot := references.SlotHelm; slot <= references.SlotAmulet; slot++ {
		character.SetEquippedItem(slot, references.NoEquipment)
	}
	character.SetEquippedItem(references.SlotWeapon, references.Dagger)
	character.SetEquippedItem(references.SlotShield, references.Dagger)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 3)
	dialog := NewReadyEquipmentDialog(
		newWidgetTestKeyboard(0), &sprites.SpriteSheet{}, state,
		game_state.ReadyEquipmentRequest{CharacterIndex: 0},
	)

	assertReadyDialogItem(t, dialog, references.Dagger, 0, true, game_state.ReadyEquipmentUnready)
	assertReadyDialogItem(t, dialog, references.Arrows, 3, false, game_state.ReadyEquipmentNoChange)

	dialog.itemListNav.SetIndex(0)
	dialog.keyboard.SimulateKeyPress(ebiten.KeyNumpadEnter)
	dialog.Update()
	commitReadyDialogIntent(t, dialog, state)
	assert.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, references.Dagger, character.GetEquippedItem(references.SlotShield))
	assertReadyDialogItem(t, dialog, references.Dagger, 1, true, game_state.ReadyEquipmentUnready)

	for index := range dialog.availableItems {
		if dialog.availableItems[index].Equipment == references.Arrows {
			dialog.itemListNav.SetIndex(index)
			break
		}
	}
	dialog.toggleEquipSelected()
	commitReadyDialogIntent(t, dialog, state)
	assert.Equal(t, uint16(3), state.PartyState.Inventory.Equipment.Get(references.Arrows))
	assert.False(t, dialog.IsDismissed())
}

func TestReadyDialogPresentsAuthorityErrorAndRetainsModal(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	character := state.PartyState.GetCharacter(0)
	character.Strength = 30
	character.SetEquippedItem(references.SlotHelm, references.LeatherHelm)
	state.PartyState.Inventory.Equipment.Set(references.ChainCoif, 1)
	dialog := NewReadyEquipmentDialog(
		newWidgetTestKeyboard(0), &sprites.SpriteSheet{}, state,
		game_state.ReadyEquipmentRequest{CharacterIndex: 0},
	)
	for index := range dialog.availableItems {
		if dialog.availableItems[index].Equipment == references.ChainCoif {
			dialog.itemListNav.SetIndex(index)
			break
		}
	}

	dialog.toggleEquipSelected()

	require.NotNil(t, dialog.confirmation)
	state.PartyState.Inventory.Equipment.Set(references.ChainCoif, 0)
	dialog.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	transaction, submitted := dialog.TakeTransaction()
	require.True(t, submitted)
	result, err := state.ApplyReadyEquipment(transaction)
	require.Error(t, err)
	dialog.ApplyTransactionResult(result, err)
	assert.Contains(t, dialog.warningMessage, "Thou hast no")
	assert.False(t, dialog.IsDismissed())
	assert.Equal(t, references.LeatherHelm, character.GetEquippedItem(references.SlotHelm))
	assert.Zero(t, state.PartyState.Inventory.Equipment.Get(references.ChainCoif))
}

func commitReadyDialogIntent(t *testing.T, dialog *ReadyEquipmentDialog, state *game_state.GameState) {
	t.Helper()
	transaction, submitted := dialog.TakeTransaction()
	require.True(t, submitted)
	result, err := state.ApplyReadyEquipment(transaction)
	require.NoError(t, err)
	dialog.ApplyTransactionResult(result, err)
}

func assertReadyDialogItem(
	t *testing.T,
	dialog *ReadyEquipmentDialog,
	equipment references.Equipment,
	quantity uint16,
	equipped bool,
	selection game_state.ReadyEquipmentSelection,
) {
	t.Helper()
	for _, item := range dialog.availableItems {
		if item.Equipment != equipment {
			continue
		}
		assert.Equal(t, quantity, item.Quantity)
		assert.Equal(t, equipped, item.Equipped)
		assert.Equal(t, selection, item.Preview.Selection)

		return
	}
	t.Fatalf("%s absent from Ready list", equipment)
}
