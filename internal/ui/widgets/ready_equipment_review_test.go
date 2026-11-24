package widgets

import (
	"image"
	"strconv"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func newReadyReviewFixture(t *testing.T) (*game_state.GameState, *ReadyEquipmentDialog, map[ebiten.Key]bool) {
	t.Helper()
	setResponsiveDialogTestResolution(t, 1280, 720)
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	character := state.PartyState.GetAvatar()
	character.Strength = 20
	for slot := references.SlotHelm; slot <= references.SlotAmulet; slot++ {
		character.SetEquippedItem(slot, references.NoEquipment)
	}
	character.SetEquippedItem(references.SlotArmour, references.LeatherArmour)
	character.SetEquippedItem(references.SlotWeapon, references.SilverSword)
	character.SetEquippedItem(references.SlotShield, references.SmallShield)
	state.PartyState.Inventory.Equipment.Set(references.Halberd, 1)
	state.PartyState.Inventory.Equipment.Set(references.LongSword, 1)
	state.PartyState.Inventory.Equipment.Set(references.SilverSword, 0)
	state.PartyState.Inventory.Equipment.Set(references.SmallShield, 0)
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	dialog := NewReadyEquipmentDialog(keyboard, &sprites.SpriteSheet{}, state, game_state.ReadyEquipmentRequest{})
	return state, dialog, held
}

func selectReadyReviewItem(t *testing.T, dialog *ReadyEquipmentDialog, equipment references.Equipment) {
	t.Helper()
	for index, item := range dialog.availableItems {
		if item.Equipment == equipment {
			dialog.itemListNav.SetIndex(index)
			return
		}
	}
	t.Fatalf("missing %s", equipment)
}

func TestReadyReviewRequiresReleaseAndCancelPreservesLoadout(t *testing.T) {
	state, dialog, held := newReadyReviewFixture(t)
	selectReadyReviewItem(t, dialog, references.Halberd)
	before := state.PartyState.Inventory.LockerSnapshot()
	held[ebiten.KeyEnter] = true
	dialog.Update()
	require.NotNil(t, dialog.confirmation)
	for range 3 {
		dialog.Update()
		_, submitted := dialog.TakeTransaction()
		require.False(t, submitted, "held opening Enter cannot confirm")
	}
	text := dialog.confirmation.output.GetOutputStr(true)
	for _, expected := range []string{"Silver Sword", "Small Shield", "Both hands", "12 -> 30 (+18)", "4 -> 2 (-2)", "20 of 20 Strength"} {
		require.Contains(t, text, expected)
	}
	delete(held, ebiten.KeyEnter)
	dialog.keyboard.ClearFrameConsumption()
	held[ebiten.KeyEscape] = true
	dialog.Update()
	require.Nil(t, dialog.confirmation)
	require.False(t, dialog.IsDismissed())
	dialog.Update()
	require.False(t, dialog.IsDismissed(), "held cancel cannot close parent")
	require.Equal(t, before, state.PartyState.Inventory.LockerSnapshot())
	require.Equal(t, references.SilverSword, state.PartyState.GetAvatar().GetEquippedItem(references.SlotWeapon))
}

func TestReadyReviewEmitsOneIntentAndShowsComparisonBudget(t *testing.T) {
	state, dialog, held := newReadyReviewFixture(t)
	selectReadyReviewItem(t, dialog, references.LongSword)
	dialog.populateDetailOutput()
	detail := dialog.detailOutput.GetOutputStr(true)
	content := strings.Join(strings.Fields(detail), " ")
	for _, expected := range []string{"Replaces: Silver Sword", "Attack: 15 (+3)", "Strength budget", "Other equipment -4", "Long Sword -9", "Remaining 7"} {
		require.Contains(t, content, expected)
	}
	beforeBudget, budget, found := strings.Cut(detail, "Strength budget \n")
	require.True(t, found, "the budget heading must remain present")
	require.True(t, strings.HasSuffix(beforeBudget, "  \n"), "retain the blank separator before the budget")
	require.Equal(t, dialog.getSelectedCharacter().GetNameAsString()+"'s Strength 20 Other equipment -4 Long Sword -9 --- Remaining 7",
		strings.Join(strings.Fields(budget), " "), "wrapped labels must retain every complete label and its value in order")
	for _, row := range strings.Split(budget, " \n") {
		trimmed := strings.TrimSpace(row)
		if trimmed == "" || trimmed == "---" {
			continue
		}
		width, _ := dialog.font.MeasureString(row)
		require.LessOrEqual(t, width, float64(dialog.detailContentRect().Dx()), row)
		require.True(t, strings.HasPrefix(row, " "), "budget row %q retains its inset", row)
		if _, err := strconv.Atoi(trimmed); err != nil {
			require.False(t, strings.HasPrefix(row, "  "), "label continuation %q needs exactly one leading space", row)
		}
	}
	require.NotContains(t, content, "Defense:")
	dialog.switchComparisonHand()
	require.Equal(t, references.SlotShield, dialog.getSelectedItem().Preview.Slot)
	dialog.switchComparisonHand()
	held[ebiten.KeyEnter] = true
	dialog.Update()
	delete(held, ebiten.KeyEnter)
	dialog.keyboard.ClearFrameConsumption()
	held[ebiten.KeyEnter] = true
	dialog.Update()
	require.Equal(t, references.SilverSword, state.PartyState.GetAvatar().GetEquippedItem(references.SlotWeapon), "widget emits only")
	commitReadyDialogIntent(t, dialog, state)
	dialog.Update()
	_, submitted := dialog.TakeTransaction()
	require.False(t, submitted)
	require.Equal(t, references.LongSword, state.PartyState.GetAvatar().GetEquippedItem(references.SlotWeapon))
}

func TestReadyReviewRetainsInvalidBudgetAndResponsiveCards(t *testing.T) {
	state, dialog, _ := newReadyReviewFixture(t)
	state.PartyState.GetAvatar().Strength = 18
	dialog.updateAvailableItems()
	selectReadyReviewItem(t, dialog, references.Halberd)
	dialog.populateDetailOutput()
	content := strings.Join(strings.Fields(dialog.detailOutput.GetOutputStr(true)), " ")
	require.Contains(t, content, "Other equipment -2")
	require.Contains(t, content, "Halberd -18")
	require.Contains(t, content, "Short by 2")
	dialog.toggleEquipSelected()
	require.Nil(t, dialog.confirmation)
	state.PartyState.GetAvatar().Strength = 20
	dialog.updateAvailableItems()
	for _, size := range []image.Point{{1280, 720}, {1920, 1080}, {3840, 2160}} {
		setResponsiveDialogTestResolution(t, size.X, size.Y)
		dialog.ensureLayout()
		selectReadyReviewItem(t, dialog, references.Halberd)
		dialog.toggleEquipSelected()
		require.NotNil(t, dialog.confirmation)
		dialog.confirmation.resolvePopupLayout(image.Rectangle{Max: size})
		dialog.populateCharacterOutput(dialog.characterHeaderLayout())
		require.True(t, dialog.confirmation.frameBounds.In(image.Rectangle{Max: size}))
		require.Contains(t, dialog.charOutput.GetOutputStr(true), "Strength in use: 12 of 20")
		require.NotContains(t, dialog.charOutput.GetOutputStr(true), "|")
		require.LessOrEqual(t, dialog.charOutput.GetTotalHeightForLines(dialog.charOutput.GetPanelContentLineCount())+
			dialog.titleOutput.GetTotalHeightForLines(1)+itemDialogTextPadding*3, float64(dialog.charBorder.GetInteriorBounds().Dy()))
		dialog.confirmation, dialog.review = nil, nil
	}
	dialog.showStrengthBreakdown()
	require.Contains(t, dialog.confirmation.output.GetOutputStr(true), "Silver Sword (8)")
}

func TestReadyStrengthRefusalAppearsOnceBeforeAndAfterAttempt(t *testing.T) {
	state, dialog, _ := newReadyReviewFixture(t)
	character := state.PartyState.GetAvatar()
	character.SetEquippedItem(references.SlotArmour, references.ScaleMail)
	dialog.updateAvailableItems()
	selectReadyReviewItem(t, dialog, references.Halberd)
	before := state.PartyState.Inventory.LockerSnapshot()
	for _, attempted := range []bool{false, true} {
		if attempted {
			dialog.toggleEquipSelected()
		}
		dialog.populateDetailOutput()
		dialog.populateStatusOutput(dialog.statusBorder.GetInteriorBounds().Dx() - (itemDialogTextPadding+4)*2)
		details := strings.Join(strings.Fields(dialog.detailOutput.GetOutputStr(true)), " ")
		footer := strings.Join(strings.Fields(dialog.statusOutput.GetOutputStr(true)), " ")
		require.Equal(t, 1, strings.Count(details+footer, "Short by 4"))
		require.NotContains(t, details, "Needs 4 more Strength.")
		require.NotContains(t, details, "Cannot equip:")
		require.Contains(t, details, "Other equipment -6")
		require.Contains(t, details, "Halberd -18")
		require.Contains(t, footer, "Too heavy: the Halberd needs 18 Strength; Avatar has 14 to spare.")
		require.NotContains(t, footer, "Needs 4")
		require.Nil(t, dialog.confirmation)
		_, submitted := dialog.TakeTransaction()
		require.False(t, submitted)
	}
	require.Equal(t, before, state.PartyState.Inventory.LockerSnapshot())
	require.Equal(t, references.SilverSword, character.GetEquippedItem(references.SlotWeapon))
}

func TestReadyEquippedInspectionShowsContributionsWithoutRemovalModifiers(t *testing.T) {
	for _, test := range []struct {
		equipment references.Equipment
		slot      references.EquipmentSlot
		placement string
		rating    string
		cost      string
	}{
		{references.ScaleMail, references.SlotArmour, "Armour", "Defense: 4", "Uses 6 Strength"},
		{references.SilverSword, references.SlotWeapon, "Hand 1", "Attack: 12", "Uses 8 Strength"},
		{references.Halberd, references.SlotWeapon, "Both hands", "Attack: 30", "Uses 18 Strength"},
	} {
		t.Run(test.equipment.String(), func(t *testing.T) {
			state, dialog, _ := newReadyReviewFixture(t)
			character := state.PartyState.GetAvatar()
			character.SetEquippedItem(test.slot, test.equipment)
			if test.equipment.IsTwoHanded() {
				character.SetEquippedItem(references.SlotShield, references.NoEquipment)
			}
			dialog.updateAvailableItems()
			selectReadyReviewItem(t, dialog, test.equipment)
			dialog.populateDetailOutput()
			content := strings.Join(strings.Fields(dialog.detailOutput.GetOutputStr(true)), " ")
			require.Contains(t, content, "Equipped: "+test.placement)
			require.Contains(t, content, test.rating)
			require.Contains(t, content, test.cost)
			for _, absent := range []string{"(-", "(+", "(0)", "After removal", "Removes:", "Strength budget", "Strength used:"} {
				require.NotContains(t, content, absent)
			}
			dialog.toggleEquipSelected()
			require.Nil(t, dialog.confirmation)
			commitReadyDialogIntent(t, dialog, state)
			require.Equal(t, references.NoEquipment, character.GetEquippedItem(test.slot), "inspection retains unready behavior")
		})
	}
}

func TestReadyAmmoRefusalHasOneExplanation(t *testing.T) {
	state, dialog, _ := newReadyReviewFixture(t)
	state.PartyState.Inventory.Equipment.Set(references.Bow, 1)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 0)
	dialog.updateAvailableItems()
	selectReadyReviewItem(t, dialog, references.Bow)
	dialog.populateDetailOutput()
	dialog.populateStatusOutput(dialog.statusBorder.GetInteriorBounds().Dx() - (itemDialogTextPadding+4)*2)
	content := strings.Join(strings.Fields(dialog.detailOutput.GetOutputStr(true)+" "+dialog.statusOutput.GetOutputStr(true)), " ")
	require.Equal(t, 1, strings.Count(content, "Thou hast no ammunition for that weapon!"))
	require.NotContains(t, content, "No Arrows!")
	require.Contains(t, content, "Ammo: Arrows (0)")
}

func TestReadyZeroStrengthItemDistinguishesCurrentFromProposedLoadout(t *testing.T) {
	state, dialog, _ := newReadyReviewFixture(t)
	state.PartyState.Inventory.Equipment.Set(references.MagicAxe, 1)
	dialog.updateAvailableItems()
	selectReadyReviewItem(t, dialog, references.MagicAxe)
	dialog.populateDetailOutput()
	dialog.populateCharacterOutput(dialog.characterHeaderLayout())
	content := strings.Join(strings.Fields(dialog.detailOutput.GetOutputStr(true)), " ")
	require.Contains(t, content, "Magic Axe None")
	require.Contains(t, content, "Other equipment -4")
	require.Contains(t, content, "Remaining 16")
	require.NotContains(t, content, "Strength cost")
	require.NotContains(t, content, "Strength used")
	require.NotContains(t, content, "(-8)")
	require.Contains(t, dialog.charOutput.GetOutputStr(true), "Strength in use: 12 of 20")
	dialog.toggleEquipSelected()
	dialog.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	commitReadyDialogIntent(t, dialog, state)
	selectReadyReviewItem(t, dialog, references.MagicAxe)
	dialog.populateDetailOutput()
	content = strings.Join(strings.Fields(dialog.detailOutput.GetOutputStr(true)), " ")
	require.Contains(t, content, "No Strength required")
	require.NotContains(t, content, "Strength budget")
}

func TestReadyBudgetUsesRetainedEquipmentForBothHandTargets(t *testing.T) {
	_, dialog, _ := newReadyReviewFixture(t)
	selectReadyReviewItem(t, dialog, references.LongSword)
	dialog.switchComparisonHand() // Replace the shield; retain the sword and armour.
	dialog.populateDetailOutput()
	content := strings.Join(strings.Fields(dialog.detailOutput.GetOutputStr(true)), " ")
	require.Contains(t, content, "Other equipment -10")
	require.Contains(t, content, "Long Sword -9")
	require.Contains(t, content, "Remaining 1")
	selectReadyReviewItem(t, dialog, references.Halberd) // Both hand occupants leave.
	dialog.populateDetailOutput()
	content = strings.Join(strings.Fields(dialog.detailOutput.GetOutputStr(true)), " ")
	require.Contains(t, content, "Other equipment -2")
	require.Contains(t, content, "Halberd -18")
	require.Contains(t, content, "Remaining 0")
}

func TestReadyBudgetDoesNotHideDifferentRefusalWhenAlsoOverStrength(t *testing.T) {
	state, dialog, _ := newReadyReviewFixture(t)
	state.PartyState.GetAvatar().Strength = 5
	state.PartyState.Inventory.Equipment.Set(references.Bow, 1)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 0)
	dialog.updateAvailableItems()
	selectReadyReviewItem(t, dialog, references.Bow)
	dialog.populateDetailOutput()
	content := strings.Join(strings.Fields(dialog.detailOutput.GetOutputStr(true)), " ")
	require.Contains(t, content, "Short by 5")
	dialog.populateStatusOutput(dialog.statusBorder.GetInteriorBounds().Dx() - (itemDialogTextPadding+4)*2)
	status := strings.Join(strings.Fields(dialog.statusOutput.GetOutputStr(true)), " ")
	require.Contains(t, status, "Thou hast no ammunition for that weapon!")
	require.NotContains(t, status, "Too heavy")
	require.NotContains(t, content, "ammunition")
}
