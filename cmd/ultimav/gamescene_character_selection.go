package main

import (
	"fmt"
	"strconv"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

type characterSelectionProfile int

const (
	characterSelectionCaster characterSelectionProfile = iota
	characterSelectionSpellTarget
	characterSelectionPotionTarget
	characterSelectionResurrectionScroll
	characterSelectionOpen
	characterSelectionJimmy
	characterSelectionSearch
	characterSelectionLook
	characterSelectionFountain
	characterSelectionGuard
	characterSelectionCrystalBall
	characterSelectionBluePotionName   = "Blue"
	characterSelectionYellowPotionName = "Yellow"
	characterSelectionRedPotionName    = "Red"
	characterSelectionGreenPotionName  = "Green"
	characterSelectionOrangePotionName = "Orange"
	characterSelectionPurplePotionName = "Purple"
	characterSelectionBlackPotionName  = "Black"
	characterSelectionWhitePotionName  = "White"
)

type characterSelectionContext struct {
	Profile characterSelectionProfile
	Spell   references.Spell
	Potion  references.Potion
}

func (g *GameScene) characterSelectionOptions(context characterSelectionContext) []widgets.CharacterSelectionOption {
	options := make([]widgets.CharacterSelectionOption, 0, party_state.MaxActiveParty)
	g.gameState.PartyState.ForEachCharacter(func(index int, character *party_state.PlayerCharacter) {
		if character == nil || character.PartyMap != party_state.PartyMapActive || character.GetNameAsString() == "" {
			return
		}
		option := widgets.CharacterSelectionOption{
			CharacterIndex: index,
			PartyPosition:  index + 1,
			Name:           character.GetNameAsString(),
			Status:         characterSelectionStatus(character.GetStatus()),
			StatusKind:     character.GetStatus(),
			PortraitSprite: character.GetKeySpriteIndex(),
			State:          widgets.CharacterSelectionAvailable,
		}
		g.populateCharacterSelectionOption(&option, character, context)
		options = append(options, option)
	})

	return options
}

func (g *GameScene) populateCharacterSelectionOption(
	option *widgets.CharacterSelectionOption,
	character *party_state.PlayerCharacter,
	context characterSelectionContext,
) {
	switch context.Profile {
	case characterSelectionCaster:
		g.populateCasterOption(option, character)
	case characterSelectionSpellTarget:
		populateSpellTargetOption(option, character, context.Spell)
	case characterSelectionPotionTarget:
		populatePotionTargetOption(option, character, context.Potion,
			g.gameState.GetCurrentSpellContext() == references.ContextDungeon)
	case characterSelectionResurrectionScroll:
		populateResurrectionOption(option, character)
	case characterSelectionOpen:
		populateActorOption(option, character)
		option.Facts = append(option.Facts, hpSelectionFact(character))
		if option.State == widgets.CharacterSelectionAvailable {
			option.Details = []string{"A trapped chest may harm this character or the whole party."}
		}
	case characterSelectionJimmy:
		populateActorOption(option, character)
		option.Facts = append(option.Facts, widgets.CharacterSelectionFact{
			Label: "Dexterity", Value: strconv.FormatUint(uint64(character.Dexterity), 10),
		})
		if option.State == widgets.CharacterSelectionAvailable {
			option.Details = []string{"Dexterity decides it. Fail, and the key breaks!"}
		}
	case characterSelectionSearch:
		populateActorOption(option, character)
		option.Facts = append(option.Facts, widgets.CharacterSelectionFact{
			Label: "Intelligence", Value: strconv.FormatUint(uint64(character.Intelligence), 10),
		})
		if option.State == widgets.CharacterSelectionAvailable {
			option.Details = []string{"Intelligence decides it."}
		}
	case characterSelectionLook:
		populateActorOption(option, character)
		if option.State == widgets.CharacterSelectionAvailable {
			option.Details = []string{"Choose who will inspect the dungeon."}
		}
	case characterSelectionFountain:
		populateActorOption(option, character)
		if option.State == widgets.CharacterSelectionAvailable {
			option.Details = []string{"Refreshing... and nothing more."}
		}
	case characterSelectionGuard:
		populateGuardOption(option, character)
	case characterSelectionCrystalBall:
		populateCrystalBallOption(option, character)
	default:
		panic("unknown character selection profile")
	}
}

func (g *GameScene) populateCasterOption(
	option *widgets.CharacterSelectionOption,
	character *party_state.PlayerCharacter,
) {
	option.Facts = []widgets.CharacterSelectionFact{
		{Label: "Class", Value: character.Class.String()},
		{Label: "MP", Value: fmt.Sprintf("%d/%d", character.GetCurrentMp(), character.GetMaxMp())},
		{Label: "Level", Value: strconv.FormatUint(uint64(character.Level), 10)},
	}
	switch {
	case !isCharacterActorEligible(character):
		markCharacterUnavailable(option, characterActorUnavailableReason(character))
	case !character.CanCastSpells():
		markCharacterUnavailable(option, "No magical ability!")
	case character.GetCurrentMp() == 0 && !g.gameState.DebugOptions.UnlimitedCastingResources:
		markCharacterUnavailable(option, "MP too low!")
	default:
		option.Details = []string{"Ready to choose a spell."}
	}
}

func populateSpellTargetOption(
	option *widgets.CharacterSelectionOption,
	character *party_state.PlayerCharacter,
	spell references.Spell,
) {
	option.Facts = []widgets.CharacterSelectionFact{hpSelectionFact(character)}
	spellName := references.GetSpellName(spell)
	resourceWarning := "The spell and MP are spent even with no effect!"
	//nolint:exhaustive // Only party-target spells with distinct consequences need specialized guidance.
	switch spell {
	case references.AnNox:
		if character.IsPoisoned() {
			option.Details = []string{spellName + " cures poison.", "Ready to cure."}
		} else {
			markCharacterWarning(option, spellName+" will have no effect on this target.", resourceWarning)
		}
	case references.AnZu:
		if character.IsAsleep() {
			option.Details = []string{spellName + " awakens a sleeping character.", "Ready to awaken."}
		} else {
			markCharacterWarning(option, spellName+" will have no effect on this target.", resourceWarning)
		}
	case references.Mani, references.VasMani:
		option.Health = characterSelectionHealth(character)
		if character.IsDead() {
			markCharacterWarning(option, spellName+" cannot heal a dead character.", resourceWarning)
		} else {
			option.Details = []string{spellName + " heals living characters.", "Healing at full health still succeeds and remains capped."}
		}
	case references.InManiCorp:
		option.Health = characterSelectionHealth(character)
		if character.IsDead() {
			option.Details = []string{spellName + " resurrects a dead character at one HP.", "Ready to resurrect."}
		} else {
			markCharacterWarning(option, spellName+" will fail because this character is living.", resourceWarning)
		}
	default:
		option.Details = []string{"Choose this spell's target."}
	}
}

func populatePotionTargetOption(
	option *widgets.CharacterSelectionOption,
	character *party_state.PlayerCharacter,
	potion references.Potion,
	dungeonUse bool,
) {
	option.Facts = []widgets.CharacterSelectionFact{hpSelectionFact(character)}
	if potion == references.Yellow {
		option.Health = characterSelectionHealth(character)
	}
	// The Use list has already shown what the potion is for. This window adds
	// only what depends on who drinks it; the pane is too small for more.
	spent := "The potion is spent either way."
	switch {
	case !potionTakesEffect(character, potion, dungeonUse):
		markCharacterWarning(option, "No noticeable effect on "+character.GetNameAsString()+" now!", spent)
	case potion == references.White:
		option.Details = []string{"It does not matter who drinks it.", spent}
	default:
		option.Details = []string{spent}
	}
}

// potionTakesEffect reports whether the potion would do anything for this member now.
func potionTakesEffect(
	character *party_state.PlayerCharacter,
	potion references.Potion,
	dungeonUse bool,
) bool {
	switch potion {
	case references.Blue:
		return character.IsAsleep()
	case references.Yellow:
		return !character.IsDead()
	case references.Red:
		return character.IsPoisoned()
	case references.Green, references.Orange:
		return character.IsGood()
	case references.Purple, references.Black:
		return false
	case references.White:
		return !dungeonUse
	default:
		panic("unknown potion")
	}
}

func populateResurrectionOption(
	option *widgets.CharacterSelectionOption,
	character *party_state.PlayerCharacter,
) {
	option.Facts = []widgets.CharacterSelectionFact{hpSelectionFact(character)}
	option.Health = characterSelectionHealth(character)
	consumption := "The scroll is spent once chosen."
	if character.IsDead() {
		option.Details = []string{
			"The scroll resurrects this character at one HP.",
			consumption,
		}

		return
	}
	markCharacterWarning(
		option,
		"Resurrection will fail because this character is living.",
		consumption,
	)
}

func characterSelectionHealth(character *party_state.PlayerCharacter) *widgets.CharacterSelectionHealth {
	current, maximum := character.GetCurrentHp(), character.GetMaxHp()

	return &widgets.CharacterSelectionHealth{
		Current: current, Maximum: maximum, Missing: maximum - min(current, maximum), Dead: character.IsDead(),
	}
}

func populateActorOption(option *widgets.CharacterSelectionOption, character *party_state.PlayerCharacter) {
	if !isCharacterActorEligible(character) {
		markCharacterUnavailable(option, characterActorUnavailableReason(character))
	}
}

func populateGuardOption(option *widgets.CharacterSelectionOption, character *party_state.PlayerCharacter) {
	option.Resources = &widgets.CharacterSelectionResources{
		CurrentHP: character.GetCurrentHp(), MaximumHP: character.GetMaxHp(),
		CurrentMP: character.GetCurrentMp(), MaximumMP: character.GetMaxMp(),
	}
	option.Facts = []widgets.CharacterSelectionFact{
		{Label: "HP", Value: fmt.Sprintf("%d/%d", option.Resources.CurrentHP, option.Resources.MaximumHP)},
		{Label: "MP", Value: fmt.Sprintf("%d/%d", option.Resources.CurrentMP, option.Resources.MaximumMP)},
	}
	switch {
	case character.IsGood():
		option.Details = []string{"Whoever stands guard gets no rest, and recovers nothing."}
	case character.IsPoisoned():
		markCharacterWarning(option, "A poisoned member cannot keep watch.", "Choosing them camps without a watch.")
	default:
		markCharacterUnavailable(option, characterActorUnavailableReason(character))
	}
}

func populateCrystalBallOption(option *widgets.CharacterSelectionOption, character *party_state.PlayerCharacter) {
	option.Facts = []widgets.CharacterSelectionFact{
		{Label: "Intelligence", Value: strconv.FormatUint(uint64(character.Intelligence), 10)},
		hpSelectionFact(character),
	}
	if !isCharacterActorEligible(character) {
		markCharacterUnavailable(option, characterActorUnavailableReason(character))

		return
	}
	option.Details = []string{"Intelligence governs the vision check.", "Failure causes one damage."}
	if character.GetCurrentHp() <= 1 {
		markCharacterWarning(option, "Failure causes one damage.", "Warning: this risk is lethal.")
	}
}

func characterSelectionNoOneOption() widgets.CharacterSelectionOption {
	return widgets.CharacterSelectionOption{
		CharacterIndex: gameNoCharacterSelection,
		PartyPosition:  0,
		Name:           "No One",
		PortraitSprite: indexes.NoSprites,
		State:          widgets.CharacterSelectionAvailable,
		Details: []string{
			"The party camps without a watch.",
			"With none posted, an ambush may find all asleep!",
		},
	}
}

const gameNoCharacterSelection = -1

func isCharacterActorEligible(character *party_state.PlayerCharacter) bool {
	return character != nil && (character.IsGood() || character.IsPoisoned())
}

func characterActorUnavailableReason(character *party_state.PlayerCharacter) string {
	if isCharacterActorEligible(character) {
		return character.GetNameAsString() + " cannot do this!"
	}

	return character.GetNameAsString() + " is incapacitated!"
}

func characterSelectionStatus(status party_state.CharacterStatus) string {
	switch status {
	case party_state.Good:
		return "Good"
	case party_state.Poisoned:
		return "Poisoned"
	case party_state.Sleep:
		return "Asleep"
	case party_state.Dead:
		return "Dead"
	case party_state.Charmed:
		return "Charmed"
	default:
		return "Unknown"
	}
}

func hpSelectionFact(character *party_state.PlayerCharacter) widgets.CharacterSelectionFact {
	return widgets.CharacterSelectionFact{
		Label: "HP",
		Value: fmt.Sprintf("%d/%d", character.GetCurrentHp(), character.GetMaxHp()),
	}
}

func markCharacterUnavailable(option *widgets.CharacterSelectionOption, details ...string) {
	option.State = widgets.CharacterSelectionUnavailable
	option.Details = details
}

func markCharacterWarning(option *widgets.CharacterSelectionOption, details ...string) {
	option.State = widgets.CharacterSelectionWarning
	option.Details = details
}

func characterSelectionPotionName(potion references.Potion) string {
	names := [...]string{
		characterSelectionBluePotionName,
		characterSelectionYellowPotionName,
		characterSelectionRedPotionName,
		characterSelectionGreenPotionName,
		characterSelectionOrangePotionName,
		characterSelectionPurplePotionName,
		characterSelectionBlackPotionName,
		characterSelectionWhitePotionName,
	}
	if potion < references.Blue || potion > references.White {
		panic("unknown potion")
	}

	return names[potion]
}
