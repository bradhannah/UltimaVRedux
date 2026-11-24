package game_state

import (
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	dungeonInteractionRollSides = 30
	dungeonLootCategoryCount    = 7
)

// DungeonLookResult preserves the reviewed fountain target while presentation
// obtains the player's yes/no response.
type DungeonLookResult struct {
	Fountain bool
	Floor    references.FloorNumber
	Position references.Position
	Cell     references.DungeonCell
}

// DungeonInteractionCharacterIndices returns active characters eligible for
// original dungeon commands: Good and Poisoned, but not Sleeping or Dead.
func (g *GameState) DungeonInteractionCharacterIndices() []int {
	indices := make([]int, 0, party_state.MaxActiveParty)
	for index := range g.PartyState.GetCharacterCount() {
		character := g.PartyState.GetCharacter(index)
		if character != nil && character.PartyMap == party_state.PartyMapActive &&
			(character.IsGood() || character.IsPoisoned()) {
			indices = append(indices, index)
		}
	}

	return indices
}

// IsDungeonInteractionCharacterEligible validates a retained UI selection.
func (g *GameState) IsDungeonInteractionCharacterEligible(index int) bool {
	if index < 0 || index >= g.PartyState.GetCharacterCount() {
		return false
	}
	character := g.PartyState.GetCharacter(index)

	return character != nil && character.PartyMap == party_state.PartyMapActive &&
		(character.IsGood() || character.IsPoisoned())
}

// DungeonOpenRequiresCharacter reports whether current-cell Open must ask Who.
func (g *GameState) DungeonOpenRequiresCharacter() bool {
	cell := g.currentDungeonCell()

	return cell.Kind() == references.DungeonClosedChest
}

func (g *GameState) actionOpenDungeonMap(characterIndex int) bool {
	g.requireActiveDungeon()
	g.claimDungeonCommandAction(DungeonFullEndPlayer)
	cell := g.currentDungeonCell()
	//nolint:exhaustive // Only closed/open chest kinds have distinct Open behavior.
	switch cell.Kind() {
	case references.DungeonClosedChest:
		g.requireDungeonInteractionCharacter(characterIndex)
		if cell.Raw()&0x07 != 0 {
			g.applyDungeonChestTrap(characterIndex)
		}
		g.transformCurrentDungeonCell(func(before references.DungeonCell) references.DungeonCell {
			if before.Kind() != references.DungeonClosedChest {
				panic("dungeon chest changed during Open")
			}

			return references.DungeonCell(0x70 | (before.Raw() & 0x08))
		})
		g.SystemCallbacks.Message.AddRowStr("Chest opened")
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundOpenDoor)
		g.SystemCallbacks.Screen.UpdateDisplay()

		return true
	case references.DungeonOpenChest:
		g.SystemCallbacks.Message.AddRowStr("Already Open!")
	default:
		g.SystemCallbacks.Message.AddRowStr("What?")
	}

	return false
}

func (g *GameState) actionJimmyDungeonMap(characterIndex int) bool {
	g.requireActiveDungeon()
	g.claimDungeonCommandAction(DungeonFullEndPlayer)
	character := g.requireDungeonInteractionCharacter(characterIndex)
	cell := g.currentDungeonCell()
	if cell.Kind() == references.DungeonOpenChest {
		g.SystemCallbacks.Message.AddRowStr("Already open!")

		return false
	}
	if cell.Kind() != references.DungeonClosedChest {
		g.SystemCallbacks.Message.AddRowStr("What?")

		return false
	}
	// REDUX DEVIATION: dungeon chest disarming breaks a key on safe 0x40/0x48
	// chests. Known safety now refuses harmlessly; uncertain and uninspected
	// attempts still pay the original key cost without learning hidden state.
	// See ref:DNG-0004.
	if g.requireActiveDungeon().ChestAssessment(g.MapState.PlayerLocation.Floor, g.MapState.PlayerLocation.Position) == references.ChestSafe {
		g.SystemCallbacks.UI.ShowActionFeedback(chestAlreadySafeMessage, true)

		return false
	}
	if !g.PartyState.Inventory.Provisions.Keys.HasSome() {
		g.SystemCallbacks.UI.ShowActionFeedback("No keys!", true)

		return false
	}

	// Original treats 0x40 and 0x48 as physically untrapped but still lets the key break.
	if cell.Raw()&0xf7 == 0x40 {
		g.PartyState.Inventory.Provisions.Keys.DecrementByOne()
		g.SystemCallbacks.UI.ShowActionFeedback("Key broke!", true)

		return false
	}
	difficulty := dungeonInteractionDifficulty(g.MapState.PlayerLocation.Floor, character.Dexterity)
	if g.RandomIntInRange(1, dungeonInteractionRollSides) <= difficulty {
		g.PartyState.Inventory.Provisions.Keys.DecrementByOne()
		g.SystemCallbacks.UI.ShowActionFeedback("Key broke!", true)

		return false
	}

	g.transformCurrentDungeonCell(func(before references.DungeonCell) references.DungeonCell {
		if before.Kind() != references.DungeonClosedChest {
			panic("dungeon chest changed during Jimmy")
		}

		return references.DungeonCell(0x40 | (before.Raw() & 0x08))
	})
	g.requireActiveDungeon().SetChestAssessment(g.MapState.PlayerLocation.Floor, g.MapState.PlayerLocation.Position, references.ChestSafe)
	g.SystemCallbacks.UI.ShowActionFeedback(chestDisarmedMessage, true)
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundUnlock)
	g.SystemCallbacks.Screen.UpdateDisplay()

	return true
}

func (g *GameState) actionGetDungeonMap() bool {
	g.requireActiveDungeon()
	g.claimDungeonCommandAction(DungeonFullEndPlayer)
	cell := g.currentDungeonCell()
	//nolint:exhaustive // Only closed/open chest kinds have distinct Get behavior.
	switch cell.Kind() {
	case references.DungeonClosedChest:
		g.SystemCallbacks.Message.AddRowStr("Must open first!")

		return false
	case references.DungeonOpenChest:
		g.transformCurrentDungeonCell(func(before references.DungeonCell) references.DungeonCell {
			if before.Kind() != references.DungeonOpenChest {
				panic("dungeon chest changed during Get")
			}

			return references.DungeonCell(before.Raw() & 0x08)
		})
		g.generateDungeonChestLoot()
		g.SystemCallbacks.Screen.UpdateDisplay()

		return true
	default:
		g.SystemCallbacks.Message.AddRowStr("Not here!")

		return false
	}
}

func (g *GameState) generateDungeonChestLoot() {
	thresholds := [dungeonLootCategoryCount]int{2, 4, 5, 10, 20, 25, 25}
	potionNames := [references.NumberOfPotions]string{
		"Blue", "Yellow", "Red", "Green", "Orange", "Purple", "Black", "White",
	}
	scrollNames := [references.NumberOfScrolls]string{
		"Vas Lor", "Rel Hur", "In Sanct", "In An",
		"In Quas Wis", "Kal Xen Corp", "In Mani Corp", "An Tym",
	}
	floor := int(g.MapState.PlayerLocation.Floor)
	eligibilityMaximum := (floor + 1) * 4
	g.SystemCallbacks.Message.AddRowStr("Chest contains:")
	for category, threshold := range thresholds {
		if g.RandomIntInRange(1, eligibilityMaximum) < threshold {
			continue
		}
		switch category {
		case 0:
			quantity := g.RandomIntInRange(1, 31)
			g.PartyState.Inventory.Provisions.Food.IncrementBy(uint16(quantity))
			g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("%d food!", quantity))
		case 1:
			// REDUX DEVIATION: Original Search/Get/Open/Jimmy rules call random(1,0) for eligible
			// floor-zero gold. Its unavailable RNG implementation leaves the
			// result undefined; Redux reports zero without an invalid extra roll.
			// See ref:DNG-0004.
			quantity := 0
			if floor > 0 {
				quantity = g.RandomIntInRange(1, floor*8)
				g.PartyState.Inventory.Gold.IncrementBy(uint16(quantity))
			}
			g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("%d gold!", quantity))
		case 2:
			quantity := g.RandomIntInRange(1, 3)
			g.PartyState.Inventory.Provisions.Keys.IncrementBy(uint16(quantity))
			g.SystemCallbacks.Message.AddRowStr(dungeonLootCount(quantity, "key", "keys"))
		case 3:
			quantity := g.RandomIntInRange(1, 3)
			g.PartyState.Inventory.Provisions.Gems.IncrementBy(uint16(quantity))
			g.SystemCallbacks.Message.AddRowStr(dungeonLootCount(quantity, "gem", "gems"))
		case 4:
			quantity := g.RandomIntInRange(1, 3)
			g.PartyState.Inventory.Provisions.Torches.IncrementBy(uint16(quantity))
			g.SystemCallbacks.Message.AddRowStr(dungeonLootCount(quantity, "torch", "torches"))
		case 5:
			potion := references.Potion(g.RandomIntInRange(0, references.NumberOfPotions-1))
			g.PartyState.Inventory.Potions.IncrementByOne(potion)
			article := "A "
			if strings.ContainsAny(potionNames[potion][:1], "AEIOU") {
				article = "An "
			}
			g.SystemCallbacks.Message.AddRowStr(article + potionNames[potion] + " potion!")
		case 6:
			scroll := references.Scroll(g.RandomIntInRange(0, references.NumberOfScrolls-1))
			g.PartyState.Inventory.Scrolls.IncrementByOne(scroll)
			g.SystemCallbacks.Message.AddRowStr("A scroll: " + scrollNames[scroll] + "!")
		default:
			panic("unknown dungeon loot category")
		}
	}
}

// dungeonLootCount words a counted find as the original does: "1 key!", "3 keys!".
func dungeonLootCount(quantity int, singular, plural string) string {
	if quantity == 1 {
		return "1 " + singular + "!"
	}

	return fmt.Sprintf("%d %s!", quantity, plural)
}

func (g *GameState) applyDungeonChestTrap(characterIndex int) {
	g.applyChestTrap(characterIndex)
}

func (g *GameState) applyChestTrap(characterIndex int) {
	trap := g.ExecuteBoomForTarget(characterIndex)
	if trap == nil {
		panic("dungeon chest trap has no eligible target")
	}
	g.SystemCallbacks.Message.AddRowStr("Trapped!")
	g.SystemCallbacks.Message.AddRowStr(trap.Message)
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundTrapTrigger)
	changes := transientStatusChanges{}
	switch trap.TrapType {
	case TrapAcid:
		damage := g.RollOriginalD30() // Failed-experiment trap selection calls transformed d30 for acid damage.
		// See ref:RNG-0001.
		g.PartyState.GetCharacter(characterIndex).SubtractHp(uint16(damage))
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundWhiteNoiseDamage)
	case TrapPoison:
		character := g.PartyState.GetCharacter(characterIndex)
		statusBefore := character.GetStatus()
		character.SetStatus(party_state.Poisoned)
		changes.observePoison(statusBefore, character)
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundPoison)
	case TrapBomb:
		for _, index := range trap.TargetIndices {
			damage := g.RandomIntInRange(trap.DamageMin, trap.DamageMax)
			g.PartyState.GetCharacter(index).SubtractHp(uint16(damage))
			g.SystemCallbacks.Audio.PlaySoundEffect(SoundWhiteNoiseDamage)
		}
	case TrapGas:
		for _, index := range trap.TargetIndices {
			character := g.PartyState.GetCharacter(index)
			statusBefore := character.GetStatus()
			character.SetStatus(party_state.Poisoned)
			changes.observePoison(statusBefore, character)
		}
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundPoison)
	case TrapNone:
		panic("dungeon chest selected no trap")
	default:
		panic("dungeon chest selected unknown trap")
	}
	g.publishTransientStatusChanges(changes)
}

func (g *GameState) processDungeonCurrentCellBomb() {
	cell := g.currentDungeonCell()
	if cell.Raw() != 0x62 && cell.Raw() != 0x6a {
		return
	}
	g.SystemCallbacks.Message.AddRowStr("Bomb Trap!")
	g.SystemCallbacks.Message.AddRowStr("KABOOM!!")
	g.transformCurrentDungeonCell(func(before references.DungeonCell) references.DungeonCell {
		if before.Raw() != 0x62 && before.Raw() != 0x6a {
			panic("dungeon floor bomb changed during resolution")
		}

		return references.DungeonCell(before.Raw() & 0x08)
	})
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundTrapTrigger)
	for _, index := range g.getAllLivingPartyIndices() {
		damage := g.RollDice(8)
		g.PartyState.GetCharacter(index).SubtractHp(uint16(damage))
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundWhiteNoiseDamage)
	}
	g.SystemCallbacks.Screen.UpdateDisplay()
}

func (g *GameState) actionLookDungeonMap(
	direction references.Direction,
	characterIndex int,
) (DungeonLookResult, bool) {
	dungeon := g.requireActiveDungeon()
	g.claimDungeonCommandAction(DungeonFullEndPlayer)
	g.requireDungeonInteractionCharacter(characterIndex)
	if !g.CanLookDungeonMap() {
		return DungeonLookResult{}, false
	}
	target, ok := dungeonRelativeTarget(g.MapState.PlayerLocation.Position, dungeon.facing, direction)
	if !ok {
		g.SystemCallbacks.Message.AddRowStr("Look-What?")

		return DungeonLookResult{}, false
	}
	cell, ok := dungeon.Cell(g.MapState.PlayerLocation.Floor, target)
	if !ok {
		panic("wrapped dungeon Look produced invalid target")
	}
	descriptionCell := cell
	if cell.Raw() == 0x61 {
		descriptionCell = 0x00
	}
	description := g.dungeonLookDescription(descriptionCell)
	g.SystemCallbacks.Message.AddRowStr("Look-You see " + description)
	g.SystemCallbacks.UI.ShowActionFeedback("You see "+description, false)

	return DungeonLookResult{
		Fountain: cell.Kind() == references.DungeonFountain,
		Floor:    g.MapState.PlayerLocation.Floor,
		Position: target,
		Cell:     cell,
	}, true
}

// CanLookDungeonMap resolves the original light precondition before direction selection.
func (g *GameState) CanLookDungeonMap() bool {
	g.requireActiveDungeon()
	if g.MapState.Lighting.HasAnyLight() {
		return true
	}
	g.SystemCallbacks.Message.AddRowStr("You see darkness.")
	g.SystemCallbacks.UI.ShowActionFeedback("You see darkness.", false)

	return false
}

// DungeonFountainChoiceResult reports the authoritative presentation outcome.
type DungeonFountainChoiceResult struct {
	Drank   bool
	Outcome string
}

// ApplyDungeonFountainChoice resolves the retained Look target after a yes/no prompt.
func (g *GameState) ApplyDungeonFountainChoice(
	characterIndex int,
	review DungeonLookResult,
	drink bool,
) DungeonFountainChoiceResult {
	character := g.requireDungeonInteractionCharacter(characterIndex)
	cell, ok := g.requireActiveDungeon().Cell(review.Floor, review.Position)
	if !ok || !review.Fountain || cell != review.Cell || cell.Kind() != references.DungeonFountain {
		panic("dungeon fountain changed after review")
	}
	if !drink {
		return DungeonFountainChoiceResult{Drank: false, Outcome: ""}
	}

	g.SystemCallbacks.Message.AddRowStr("Yes. Gulp!")
	result := DungeonFountainChoiceResult{Drank: true, Outcome: ""}
	changes := transientStatusChanges{}
	switch cell.Raw() {
	case 0x50:
		statusBefore := character.GetStatus()
		character.SetStatus(party_state.Good)
		changes.observePoison(statusBefore, character)
		result.Outcome = "Cured!"
	case 0x51:
		hpBefore := character.GetCurrentHp()
		character.SetHp(character.GetMaxHp())
		changes.observeHealing(hpBefore, character)
		result.Outcome = "Healed!"
	case 0x52:
		statusBefore := character.GetStatus()
		character.SetStatus(party_state.Poisoned)
		changes.observePoison(statusBefore, character)
		result.Outcome = "Poisoned!"
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundPoison)
	default:
		result.Outcome = "Bad taste."
		damage := g.RandomIntInRange(0, 7)
		character.SubtractHp(uint16(damage))
		if damage > 0 {
			g.SystemCallbacks.Audio.PlaySoundEffect(SoundWhiteNoiseDamage)
		}
	}
	g.publishTransientStatusChanges(changes)
	g.SystemCallbacks.Message.AddRowStr(result.Outcome)

	return result
}

func (g *GameState) dungeonLookDescription(cell references.DungeonCell) string {
	if cell.Raw() >= 0x80 && cell.Raw() <= 0x8f {
		switch cell.Raw() {
		case 0x80:
			return "a sleep field."
		case 0x81:
			return "a poison gas field."
		case 0x82:
			return "a wall of fire."
		case 0x83:
			return "an electric field."
		default:
			return "an energy field."
		}
	}
	if cell.Kind() == references.DungeonSpecialWall {
		switch dungeonPresentationKind(g.MapState.PlayerLocation.Location) {
		case dungeonPresentationStalactite:
			return "a dripping stalactite."
		case dungeonPresentationCaveIn:
			return "a caved in passage."
		case dungeonPresentationSkeleton:
			if g.RandomIntInRange(1, 255) == 255 {
				return "an unfortunate software pirate."
			}

			return "a less fortunate adventurer."
		default:
			panic("unknown dungeon presentation kind")
		}
	}

	switch cell.Kind() {
	case references.DungeonPassage:
		return "a passage."
	case references.DungeonLadderUp:
		return "an up ladder."
	case references.DungeonLadderDown:
		return "a down ladder."
	case references.DungeonLadderBoth:
		return "a ladder."
	case references.DungeonClosedChest:
		return "a wooden chest."
	case references.DungeonFountain:
		return "a fountain."
	case references.DungeonPitOrTrap:
		return "a pit."
	case references.DungeonOpenChest:
		return "an open chest."
	case references.DungeonEnergyField:
		return "an energy field."
	case references.DungeonReserved:
		return "nothing of note."
	case references.DungeonClearedRoom:
		return "a heavy door."
	case references.DungeonWall, references.DungeonHiddenDoor:
		return "a wall."
	case references.DungeonRevealedDoor, references.DungeonLiveRoom:
		return "a heavy door."
	case references.DungeonSpecialWall:
		panic("special dungeon wall was not described")
	default:
		panic("unknown dungeon cell kind")
	}
}

type dungeonPresentation byte

const (
	dungeonPresentationStalactite dungeonPresentation = iota
	dungeonPresentationCaveIn
	dungeonPresentationSkeleton
)

func dungeonPresentationKind(location references.Location) dungeonPresentation {
	//nolint:exhaustive // Non-dungeon locations are rejected by the default programming-error panic.
	switch location {
	case references.Despise, references.Destard, references.Doom:
		return dungeonPresentationStalactite
	case references.Shame, references.Hythloth:
		return dungeonPresentationCaveIn
	case references.Deceit, references.Wrong, references.Covetous:
		return dungeonPresentationSkeleton
	default:
		panic("non-dungeon location has no presentation kind")
	}
}

func dungeonInteractionDifficulty(floor references.FloorNumber, dexterity byte) int {
	return (dungeonInteractionRollSides + 2*int(floor) - int(dexterity)) / 2
}

func (g *GameState) requireDungeonInteractionCharacter(index int) *party_state.PlayerCharacter {
	if !g.IsDungeonInteractionCharacterEligible(index) {
		panic("invalid dungeon interaction character")
	}

	return g.PartyState.GetCharacter(index)
}

func (g *GameState) currentDungeonCell() references.DungeonCell {
	cell, ok := g.requireActiveDungeon().Cell(
		g.MapState.PlayerLocation.Floor,
		g.MapState.PlayerLocation.Position,
	)
	if !ok {
		panic("current dungeon cell is invalid")
	}

	return cell
}

func (g *GameState) transformCurrentDungeonCell(
	transform func(references.DungeonCell) references.DungeonCell,
) DungeonCellChange {
	change, err := g.TransformDungeonCell(
		g.MapState.PlayerLocation.Floor,
		g.MapState.PlayerLocation.Position,
		transform,
	)
	if err != nil {
		panic(err)
	}

	return change
}

func dungeonRelativeTarget(
	position references.Position,
	facing references.DungeonFacing,
	direction references.Direction,
) (references.Position, bool) {
	if direction == references.Down {
		return position, true
	}
	targetFacing := facing
	switch direction {
	case references.Up:
	case references.Left:
		targetFacing = facing.TurnLeft()
	case references.Right:
		targetFacing = facing.TurnRight()
	case references.Down, references.NoneDirection:
		return references.Position{}, false
	default:
		return references.Position{}, false
	}
	target := targetFacing.Direction().GetNewPositionInDirection(&position)
	if target == nil {
		return references.Position{}, false
	}

	return *target.GetWrapped(references.XDungeonMapTiles, references.YDungeonMapTiles), true
}
