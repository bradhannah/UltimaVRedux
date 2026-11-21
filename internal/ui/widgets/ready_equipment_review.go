package widgets

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const readyBothHands = "Both hands"

func (d *ReadyEquipmentDialog) transactionFor(equipment references.Equipment) game_state.ReadyEquipmentTransaction {
	hand := d.targetHand
	if hand == references.SlotNone {
		hand = d.defaultComparisonHand(equipment)
	}

	return game_state.ReadyEquipmentTransaction{
		Request: d.request, CharacterIndex: d.selectedCharIdx,
		ExpectedOccupant: d.getSelectedCharacter().GetNameAsString(), Equipment: equipment,
		TargetHand: hand, Confirmed: nil,
	}
}

func (d *ReadyEquipmentDialog) defaultComparisonHand(equipment references.Equipment) references.EquipmentSlot {
	if !equipment.IsWeapon() && !equipment.IsShield() {
		return references.SlotNone
	}
	character := d.getSelectedCharacter()
	hands := []references.EquipmentSlot{references.SlotWeapon, references.SlotShield}
	for _, slot := range hands {
		current := character.GetEquippedItem(slot)
		if current != references.NoEquipment && current.IsShield() == equipment.IsShield() {
			return slot
		}
	}
	for _, slot := range hands {
		if character.IsSlotEmpty(slot) {
			return slot
		}
	}

	return equipment.GetSlotType()
}

func (d *ReadyEquipmentDialog) switchComparisonHand() {
	item := d.getSelectedItem()
	if item == nil || item.Equipped || item.Equipment.IsTwoHanded() ||
		(!item.Equipment.IsWeapon() && !item.Equipment.IsShield()) {
		return
	}
	d.targetHand = references.SlotWeapon
	if item.Preview.Slot == references.SlotWeapon {
		d.targetHand = references.SlotShield
	}
	d.updateAvailableItems()
	d.warningMessage = ""
}

func (d *ReadyEquipmentDialog) appendReplacementTargets(item *availableEquipment) {
	if item.Equipped {
		for slot := references.SlotHelm; slot <= references.SlotAmulet; slot++ {
			if d.getSelectedCharacter().GetEquippedItem(slot) != item.Equipment {
				continue
			}
			placement := slot.DisplayName()
			if item.Equipment.IsTwoHanded() {
				placement = readyBothHands
			}
			d.detailOutput.AppendLine().Text("Equipped: ", DialogLabelColor).
				Text(placement, DialogValueColor).WordWrap().Done()
		}

		return
	}
	preview := item.Preview
	if preview.Selection == game_state.ReadyEquipmentNoChange {
		return
	}
	for slot := references.SlotHelm; slot <= references.SlotAmulet; slot++ {
		if preview.Removes(slot) {
			d.detailOutput.AppendLine().Text("Replaces: ", DialogLabelColor).
				Text(preview.Before[slot].String()+" - "+slot.DisplayName(), DialogValueColor).WordWrap().Done()
		}
	}
	affected := preview.Slot.DisplayName()
	if item.Equipment.IsTwoHanded() {
		affected = readyBothHands
	}
	d.detailOutput.AppendLine().Text("Affects: ", DialogLabelColor).Text(affected, DialogValueColor).WordWrap().Done()
	if item.Equipment.IsWeapon() || item.Equipment.IsShield() {
		for _, slot := range []references.EquipmentSlot{references.SlotWeapon, references.SlotShield} {
			if slot != preview.Slot && preview.After[slot] != references.NoEquipment && !preview.Removes(slot) {
				d.detailOutput.AppendLine().Text("Keeps: ", DialogLabelColor).
					Text(preview.After[slot].String(), DialogValueColor).WordWrap().Done()
			}
		}
	}
}

func (d *ReadyEquipmentDialog) appendEquippedRatings(equipment references.Equipment) {
	for _, rating := range []struct {
		label string
		value int
	}{
		{ratingAttack, equipment.GetAttackValue()},
		{ratingDefense, equipment.GetDefenseValue()},
	} {
		if rating.value != 0 {
			d.detailOutput.AppendLine().Text(rating.label+": ", DialogLabelColor).
				Textf("%d", DialogValueColor, rating.value).WordWrap().Done()
		}
	}
	d.appendItemStrength(equipment)
}

func (d *ReadyEquipmentDialog) appendItemStrength(equipment references.Equipment) {
	if equipment.GetStrengthRequired() == 0 {
		d.detailOutput.AppendLine().Text("No Strength required", DialogValueColor).WordWrap().Done()

		return
	}
	d.detailOutput.AppendLine().Text("Uses ", DialogLabelColor).
		Textf("%d", DialogValueColor, equipment.GetStrengthRequired()).
		Text(" Strength", DialogLabelColor).WordWrap().Done()
}

func (d *ReadyEquipmentDialog) appendComparisonRating(label string, value, delta int) {
	if value == 0 && delta == 0 {
		return
	}
	deltaColor := d.deltaColor(delta)
	d.detailOutput.AppendLine().Text(label+": ", DialogLabelColor).
		Textf("%d ", DialogValueColor, value).Text(d.formatDelta(delta), deltaColor).WordWrap().Done()
}

func (d *ReadyEquipmentDialog) submitTransaction(transaction game_state.ReadyEquipmentTransaction) {
	d.pending = &transaction
	d.waitingForResult = true
	d.keyboard.PrepareForTransition(0)
}

// TakeTransaction transfers one intent to the owning scene; polling cannot replay it.
func (d *ReadyEquipmentDialog) TakeTransaction() (game_state.ReadyEquipmentTransaction, bool) {
	if d.pending == nil || d.isDismissed {
		var noTransaction game_state.ReadyEquipmentTransaction

		return noTransaction, false
	}
	transaction := *d.pending
	d.pending = nil

	return transaction, true
}

// ApplyTransactionResult updates presentation after the scene commits or rejects an intent.
func (d *ReadyEquipmentDialog) ApplyTransactionResult(result game_state.ReadyEquipmentResult, err error) {
	d.waitingForResult = false
	d.updateAvailableItems()
	d.warningMessage = ""
	if err != nil {
		d.warningMessage = err.Error()
	} else if result.Dismiss {
		d.dismiss()
	}
}

type readyReviewLine struct {
	label string
	value string
}

func (d *ReadyEquipmentDialog) showReplacementConfirmation(transaction game_state.ReadyEquipmentTransaction) {
	preview := *transaction.Confirmed
	lines := []readyReviewLine{{label: "", value: "Replace equipment?"}, {label: "", value: transaction.ExpectedOccupant}}
	for slot := references.SlotHelm; slot <= references.SlotAmulet; slot++ {
		if preview.Removes(slot) {
			lines = append(lines, readyReviewLine{"Remove: ", preview.Before[slot].String() + " (" + slot.DisplayName() + ")"})
		}
	}
	target := preview.Slot.DisplayName()
	if transaction.Equipment.IsTwoHanded() {
		target = readyBothHands
	}
	lines = append(lines, readyReviewLine{"Equip: ", transaction.Equipment.String() + " (" + target + ")"})
	if preview.Current.Attack != preview.Resulting.Attack {
		lines = append(lines, readyReviewLine{"Attack: ", fmt.Sprintf("%d -> %d (%+d)", preview.Current.Attack,
			preview.Resulting.Attack, preview.Resulting.Attack-preview.Current.Attack)})
	}
	if preview.Current.Defense != preview.Resulting.Defense {
		lines = append(lines, readyReviewLine{"Defense: ", fmt.Sprintf("%d -> %d (%+d)", preview.Current.Defense,
			preview.Resulting.Defense, preview.Resulting.Defense-preview.Current.Defense)})
	}
	lines = append(lines, readyReviewLine{"With this equipped: ", fmt.Sprintf("%d of %d Strength",
		preview.Resulting.Strength, preview.Strength)},
		readyReviewLine{label: "", value: fmt.Sprintf("%d remaining after replacement.",
			preview.Strength-preview.Resulting.Strength)},
		readyReviewLine{label: "", value: "Removed items return to party inventory."})
	d.showReviewCard(lines, "[Enter] Replace   [Esc] Cancel")
}

func (d *ReadyEquipmentDialog) showStrengthBreakdown() {
	character := d.getSelectedCharacter()
	if character == nil {
		return
	}
	lines := []readyReviewLine{{label: "", value: character.GetNameAsString() + " - Equipment Strength"}}
	for slot := references.SlotHelm; slot <= references.SlotAmulet; slot++ {
		item := character.GetEquippedItem(slot)
		if item != references.NoEquipment {
			lines = append(lines, readyReviewLine{slot.DisplayName() + ": ",
				fmt.Sprintf("%s (%d)", item, item.GetStrengthRequired())})
		}
	}
	used := character.GetTotalEquippedStrRequirement()
	lines = append(lines,
		readyReviewLine{"Current equipment: ", fmt.Sprintf("%d of %d Strength", used, character.Strength)},
		readyReviewLine{"Remaining: ", strconv.Itoa(int(character.Strength) - used)})
	d.showReviewCard(lines, "[Enter/Esc] Back")
}

// The Ready modal owns this child card and its input. Journal suspension preserves
// the parent and child together; no second stack entry or gameplay workflow is needed.
func (d *ReadyEquipmentDialog) showReviewCard(lines []readyReviewLine, controls string) {
	const center, maximumExtent = 0.5, 0.85
	plain := make([]string, 0, len(lines))
	for _, line := range lines {
		plain = append(plain, line.label+line.value)
	}
	d.confirmation = NewMessageDialog(strings.Join(plain, "\n")).WithEffects(false).
		WithStaticContent().WithSeparatePromptArea().WithContentTextAlignment(ContentAlignTop).
		WithHorizontalPosition(DialogHorizontalWholeScreen).WithVerticalPosition(center).
		WithAutoSize(maximumExtent, maximumExtent).Build()
	d.confirmation.output.Reset()
	for _, line := range lines {
		d.confirmation.AppendLineBuilder().Text(line.label, DialogLabelColor).
			Text(line.value, DialogValueColor).WordWrap().Done()
	}
	d.confirmation.promptOutput.Reset()
	AppendControlHints(d.confirmation.promptOutput.AppendLine(), controls).WordWrap().Done()
	d.confirmation.ShowWithoutAnimation()
	d.keyboard.PrepareForTransition(0)
}

func (d *ReadyEquipmentDialog) updateConfirmation() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.confirmation, d.review = nil, nil
		d.keyboard.PrepareForTransition(0)

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		if d.review != nil {
			d.submitTransaction(*d.review)
		}
		d.confirmation, d.review = nil, nil
		d.keyboard.PrepareForTransition(0)
	}
}
