package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

const unknownSpellFormulaState = "unknown"

// ZStatsDossier is a complete detached projection of the information available
// through Z-Stats. It contains no live GameState or PartyState references.
type ZStatsDossier struct {
	InitialCharacterIndex  int
	Characters             []ZStatsCharacter
	Provisions             []ZStatsProvision
	Armory                 []ZStatsArmoryItem
	Magic                  ZStatsMagicDossier
	Items                  []ZStatsOwnedItem
	UnreadHiddenSpellCount int
}

// ZStatsProvision describes one canonical party provision quantity.
type ZStatsProvision struct {
	ID          int
	SpriteIndex indexes.SpriteIndex
	Name        string
	Quantity    uint16
	Description string
}

// ZStatsCharacter describes one active roster member.
type ZStatsCharacter struct {
	RosterIndex              int
	SpriteIndex              indexes.SpriteIndex
	Name                     string
	Gender                   string
	Class                    string
	Status                   string
	Strength                 int
	Dexterity                int
	Intelligence             int
	CurrentHP                int
	MaximumHP                int
	CurrentMP                int
	MaximumMP                int
	Experience               int
	Level                    int
	NextLevelExperience      int
	ExperienceIntoLevel      int
	ExperienceForNextLevel   int
	ExperienceToNextLevel    int
	NextLevelProgress        float64
	ReadyToAdvance           bool
	HighestUsableSpellCircle int
	// CircleStudied reports the once-only study that lifts the circle above level.
	CircleStudied    bool
	CanCast          bool
	EquipmentAttack  int
	EquipmentDefense int
	Equipped         []ZStatsEquippedItem
}

// ZStatsEquippedItem describes one of the six ordered character equipment slots.
type ZStatsEquippedItem struct {
	Slot             string
	ItemID           int
	Name             string
	Category         string
	Attack           int
	Defense          int
	StrengthRequired int
	Range            int
	TwoHanded        bool
	RequiresAmmo     bool
	AmmoID           int
	AmmoName         string
	Abilities        []string
}

// ZStatsArmoryItem describes one original equipment identity.
type ZStatsArmoryItem struct {
	ID               int
	SpriteIndex      indexes.SpriteIndex
	Name             string
	Category         string
	Available        uint16
	Readied          uint16
	Attack           int
	Defense          int
	StrengthRequired int
	Range            int
	TwoHanded        bool
	RequiresAmmo     bool
	AmmoID           int
	AmmoName         string
	Abilities        []string
}

// ZStatsMagicDossier describes the visible spellbook and all magic consumables.
type ZStatsMagicDossier struct {
	Spells   []ZStatsSpell
	Reagents []ZStatsInventoryItem
	Potions  []ZStatsInventoryItem
	Scrolls  []ZStatsInventoryItem
}

// ZStatsSpell describes one visible Book-of-Lore or learned hidden spell.
type ZStatsSpell struct {
	ID          int
	SpriteIndex indexes.SpriteIndex
	Syllables   string
	Name        string
	Circle      int
	MPCost      int
	Prepared    uint16
	Targeting   string
	// Unaimed marks a spell cast here with no aiming step. Its Targeting already
	// says who or what it falls on, so the details show no separate Affects line.
	Unaimed            bool
	Scope              references.SpellEffectScope
	Contexts           string
	Description        string
	EffectLabel        string
	RecipeMask         uint16
	CanMix             int
	RecipeInventory    []SpellReagentStock
	MixMissing         []string
	FormulaState       string
	Reagents           []string
	ReagentCandidates  [][]string
	Hidden             bool
	LearningSourceKind string
	LearningSource     string
	LearningDate       string
	Unread             bool
}

// SpellReagentStock describes the owned amount of one known recipe ingredient.
type SpellReagentStock struct {
	Name     string
	Quantity uint16
}

// ZStatsInventoryItem describes one reagent, potion, or scroll identity.
type ZStatsInventoryItem struct {
	ID          int
	SpriteIndex indexes.SpriteIndex
	Name        string
	Quantity    uint16
	Description string
}

// ZStatsOwnedItem describes one carried quest item or moonstone.
type ZStatsOwnedItem struct {
	ID          int
	SpriteIndex indexes.SpriteIndex
	Category    string
	Name        string
	Quantity    uint16
	Description string
	MoonPhase   ZStatsMoonPhase
}

// ZStatsMoonPhase distinguishes a carried stone's phase from an ordinary item.
// Phase zero is New Moon, so absence must be represented separately.
type ZStatsMoonPhase struct {
	Present bool
	Phase   datetime.MoonPhase
}

// CurrentZStatsDossier returns a fully detached, read-only Z-Stats projection.
func (g *GameState) CurrentZStatsDossier() ZStatsDossier {
	inventoryReferences := g.GameReferences.InventoryItemReferences
	locker := g.PartyState.Inventory.LockerSnapshot()
	preparedSpells := g.PartyState.Inventory.SpellSnapshot()
	activeRosterIndices := g.activeZStatsRosterIndices()
	initialCharacterIndex := g.initialZStatsCharacterIndex(activeRosterIndices)

	characters := make([]ZStatsCharacter, 0, len(activeRosterIndices))
	readied := make(map[references.Equipment]uint16, references.Ankh-references.LeatherHelm+1)
	for _, rosterIndex := range activeRosterIndices {
		character := g.PartyState.GetCharacter(rosterIndex)
		characters = append(characters, projectZStatsCharacter(rosterIndex, character))
		for _, slot := range references.EquipmentSlots() {
			equipment := character.GetEquippedItem(slot)
			if equipment >= references.LeatherHelm && equipment <= references.Ankh {
				readied[equipment]++
			}
		}
	}

	armory := make([]ZStatsArmoryItem, 0, references.Ankh-references.LeatherHelm+1)
	for equipment := references.LeatherHelm; equipment <= references.Ankh; equipment++ {
		ammo := equipment.GetRequiredAmmo()
		inventoryReference := inventoryReferences.GetReferenceByItem(equipment)
		armory = append(armory, ZStatsArmoryItem{
			ID:               equipment.ID(),
			SpriteIndex:      inventoryReference.ItemSprite,
			Name:             equipment.String(),
			Category:         equipment.GetCategory().String(),
			Available:        locker.Equipment[equipment],
			Readied:          readied[equipment],
			Attack:           equipment.GetAttackValue(),
			Defense:          equipment.GetDefenseValue(),
			StrengthRequired: equipment.GetStrengthRequired(),
			Range:            equipment.GetAttackRange(),
			TwoHanded:        equipment.IsTwoHanded(),
			RequiresAmmo:     ammo != references.NoEquipment,
			AmmoID:           ammo.ID(),
			AmmoName:         zStatsEquipmentName(ammo),
			Abilities:        zStatsEquipmentAbilities(equipment),
		})
	}

	return ZStatsDossier{
		InitialCharacterIndex:  initialCharacterIndex,
		Characters:             characters,
		Provisions:             zStatsProvisions(inventoryReferences, locker),
		Armory:                 armory,
		Magic:                  g.projectZStatsMagic(inventoryReferences, locker, preparedSpells),
		Items:                  g.projectZStatsOwnedItems(inventoryReferences),
		UnreadHiddenSpellCount: g.UnreadKnowledgeCount(KnowledgeCategorySpell),
	}
}

func zStatsProvisions(
	inventoryReferences *references.InventoryItemReferences,
	locker party_state.InventoryLockerSnapshot,
) []ZStatsProvision {
	provisions := [...]references.Provision{
		references.Food,
		references.Gold,
		references.Key,
		references.Gem,
		references.Torches,
		references.SkullKeys,
	}
	result := make([]ZStatsProvision, 0, len(provisions))
	for _, provision := range provisions {
		reference := inventoryReferences.GetReferenceByItem(provision)
		result = append(result, ZStatsProvision{
			ID:          provision.ID(),
			SpriteIndex: reference.ItemSprite,
			Name:        zStatsInventoryItemName(reference),
			Quantity:    locker.Provisions[provision],
			Description: reference.ItemDescription,
		})
	}

	return result
}

func (g *GameState) activeZStatsRosterIndices() []int {
	indices := make([]int, 0, party_state.MaxActiveParty)
	for index := range party_state.NRosterSlots {
		if g.PartyState.HasCharacter(index) && g.PartyState.GetCharacter(index).IsInParty() {
			indices = append(indices, index)
		}
	}

	return indices
}

func (g *GameState) initialZStatsCharacterIndex(activeRosterIndices []int) int {
	isActive := func(index int) bool {
		return index >= 0 && index < party_state.NRosterSlots &&
			g.PartyState.HasCharacter(index) && g.PartyState.GetCharacter(index).IsInParty()
	}
	if g.CombatScene != nil {
		identity := g.CombatScene.GetCurrentTurnIdentity()
		if identity.IsPartyMember && isActive(identity.EntityIndex) {
			return identity.EntityIndex
		}
	}
	if activePlayerIndex := g.GetActivePlayerIndex(); isActive(activePlayerIndex) {
		return activePlayerIndex
	}
	if len(activeRosterIndices) > 0 {
		return activeRosterIndices[0]
	}

	return -1
}

func projectZStatsCharacter(rosterIndex int, character *party_state.PlayerCharacter) ZStatsCharacter {
	nextExperience, intoLevel, forNextLevel, toNextLevel, progress, ready := zStatsLevelProgress(character)
	canCast := character.CanCastSpells()
	highestCircle := 0
	if canCast {
		highestCircle = character.SpellCircleLimit()
	}

	equipped := make([]ZStatsEquippedItem, 0, len(references.EquipmentSlots()))
	for _, slot := range references.EquipmentSlots() {
		equipment := character.GetEquippedItem(slot)
		ammo := equipment.GetRequiredAmmo()
		equipped = append(equipped, ZStatsEquippedItem{
			Slot:             slot.DisplayName(),
			ItemID:           equipment.ID(),
			Name:             zStatsEquipmentName(equipment),
			Category:         equipment.GetCategory().String(),
			Attack:           equipment.GetAttackValue(),
			Defense:          equipment.GetDefenseValue(),
			StrengthRequired: equipment.GetStrengthRequired(),
			Range:            equipment.GetAttackRange(),
			TwoHanded:        equipment.IsTwoHanded(),
			RequiresAmmo:     ammo != references.NoEquipment,
			AmmoID:           ammo.ID(),
			AmmoName:         zStatsEquipmentName(ammo),
			Abilities:        zStatsEquipmentAbilities(equipment),
		})
	}

	return ZStatsCharacter{
		RosterIndex:              rosterIndex,
		SpriteIndex:              character.GetKeySpriteIndex(),
		Name:                     character.GetNameAsString(),
		Gender:                   zStatsGenderName(character.Gender),
		Class:                    character.Class.String(),
		Status:                   zStatsStatusName(character.GetStatus()),
		Strength:                 int(character.Strength),
		Dexterity:                int(character.Dexterity),
		Intelligence:             int(character.Intelligence),
		CurrentHP:                int(character.GetCurrentHp()),
		MaximumHP:                int(character.GetMaxHp()),
		CurrentMP:                int(character.GetCurrentMp()),
		MaximumMP:                int(character.GetMaxMp()),
		Experience:               int(character.Exp),
		Level:                    int(character.Level),
		NextLevelExperience:      nextExperience,
		ExperienceIntoLevel:      intoLevel,
		ExperienceForNextLevel:   forNextLevel,
		ExperienceToNextLevel:    toNextLevel,
		NextLevelProgress:        progress,
		ReadyToAdvance:           ready,
		HighestUsableSpellCircle: highestCircle,
		CircleStudied:            character.HasCircleStudy(),
		CanCast:                  canCast,
		EquipmentAttack:          character.GetTotalAttack(),
		EquipmentDefense:         character.GetTotalDefense(),
		Equipped:                 equipped,
	}
}

func zStatsLevelProgress(character *party_state.PlayerCharacter) (int, int, int, int, float64, bool) {
	level := max(int(character.Level), 1)
	if level >= maximumCharacterLevel {
		return 0, 0, 0, 0, 1, false
	}

	levelStart := minimumExperienceForLevel(level)
	nextLevelExperience := minimumExperienceForLevel(level + 1)
	experienceIntoLevel := max(int(character.Exp)-levelStart, 0)
	experienceForNextLevel := nextLevelExperience - levelStart
	experienceToNextLevel := max(nextLevelExperience-int(character.Exp), 0)
	progress := min(float64(experienceIntoLevel)/float64(experienceForNextLevel), 1)
	ready := CalculateLevelFromExperience(character.Exp) > character.Level

	return nextLevelExperience, experienceIntoLevel, experienceForNextLevel, experienceToNextLevel, progress, ready
}

func zStatsEquipmentName(equipment references.Equipment) string {
	if equipment == references.NoEquipment {
		return "None"
	}

	return equipment.String()
}

func zStatsEquipmentAbilities(equipment references.Equipment) []string {
	abilities := equipment.GetAbilities()
	result := make([]string, 0, len(abilities))
	for _, ability := range abilities {
		result = append(result, ability.String())
	}

	return result
}

func zStatsGenderName(gender party_state.CharacterGender) string {
	if name, ok := gender.FriendlyName(); ok {
		return name
	}

	return "Unknown"
}

func zStatsStatusName(status party_state.CharacterStatus) string {
	if name, ok := status.FriendlyName(); ok {
		return name
	}

	return "Unknown"
}

func (g *GameState) projectZStatsMagic(
	inventoryReferences *references.InventoryItemReferences,
	locker party_state.InventoryLockerSnapshot,
	preparedSpells party_state.SpellInventorySnapshot,
) ZStatsMagicDossier {
	context := g.GetCurrentSpellContext()
	spells := make([]ZStatsSpell, 0, references.TotalSpellCount)
	for spell := references.InLor; spell <= references.AnTym; spell++ {
		knowledge, hidden, visible := g.zStatsSpellKnowledge(spell)
		if !visible {
			continue
		}
		spells = append(spells, g.projectZStatsSpell(
			inventoryReferences,
			preparedSpells,
			spell,
			context,
			knowledge,
			hidden,
		))
	}

	return ZStatsMagicDossier{
		Spells:   spells,
		Reagents: zStatsMagicInventory(inventoryReferences, references.ItemTypeReagent, locker),
		Potions:  zStatsMagicInventory(inventoryReferences, references.ItemTypePotion, locker),
		Scrolls:  zStatsMagicInventory(inventoryReferences, references.ItemTypeScroll, locker),
	}
}

// CurrentSpellCatalog is the shared detached knowledge/inventory view for Cast,
// Mix and Z-stats. Hidden formulas are exposed only through player knowledge.
func (g *GameState) CurrentSpellCatalog() []ZStatsSpell {
	return g.projectZStatsMagic(g.GameReferences.InventoryItemReferences,
		g.PartyState.Inventory.LockerSnapshot(), g.PartyState.Inventory.SpellSnapshot()).Spells
}

func (g *GameState) zStatsSpellKnowledge(spell references.Spell) (KnowledgeEntry, bool, bool) {
	if _, hidden := SpellKnowledgeKey(spell); hidden {
		knowledge, visible := g.SpellKnowledge(spell)

		return knowledge, true, visible
	}

	return KnowledgeEntry{}, false, true
}

func (g *GameState) projectZStatsSpell(
	inventoryReferences *references.InventoryItemReferences,
	preparedSpells party_state.SpellInventorySnapshot,
	spell references.Spell,
	context references.SpellContext,
	knowledge KnowledgeEntry,
	hidden bool,
) ZStatsSpell {
	info := references.GetSpellInfo(spell)
	formulaState := string(SpellFormulaConfirmed)
	reagents := zStatsReagentNames(inventoryReferences, references.GetSpellReagents(spell))
	candidates := make([][]string, 0)
	learningSourceKind, learningSource, learningDate, unread := "", "", "", false
	if hidden {
		formulaState, reagents, candidates = zStatsHiddenSpellFormula(inventoryReferences, knowledge)
		learningSourceKind = string(knowledge.Source.Kind)
		learningSource = knowledge.Source.DisplayName
		if knowledge.DiscoveredOn != nil {
			learningDate = fmt.Sprintf(
				"%d-%d-%d",
				knowledge.DiscoveredOn.Month,
				knowledge.DiscoveredOn.Day,
				knowledge.DiscoveredOn.Year,
			)
		}
		unread = knowledge.Unread
	}

	profile := references.GetSpellTargetingProfile(spell, context)
	inventoryReference := inventoryReferences.GetReferenceByItem(spell)
	var recipeMask uint16
	canMix := 0
	var recipeInventory []SpellReagentStock
	var missing []string
	if formulaState == string(SpellFormulaConfirmed) {
		recipeMask = references.GetSpellReagents(spell)
		canMix = g.MaximumMixQuantity(recipeMask)
		for reagent := references.SulfurAsh; reagent <= references.MandrakeRoot; reagent++ {
			if recipeMask&(1<<uint(reagent)) == 0 {
				continue
			}
			name := zStatsInventoryItemName(inventoryReferences.GetReferenceByItem(reagent))
			quantity := g.PartyState.Inventory.Reagent.Get(reagent)
			recipeInventory = append(recipeInventory, SpellReagentStock{Name: name, Quantity: quantity})
			if quantity == 0 {
				missing = append(missing, name)
			}
		}
	}

	return ZStatsSpell{
		ID:                 spell.ID(),
		SpriteIndex:        inventoryReference.ItemSprite,
		Syllables:          info.Name,
		Name:               info.Name,
		Circle:             info.Circle,
		MPCost:             info.Circle,
		Prepared:           preparedSpells[spell],
		Targeting:          references.SpellTargetLabel(spell, context),
		Unaimed:            profile.Selection == references.TargetAutomatic,
		Scope:              profile.Scope,
		Contexts:           info.Context.PlayerLabel(),
		Description:        info.Description,
		EffectLabel:        references.SpellEffectLabel(spell),
		RecipeMask:         recipeMask,
		CanMix:             canMix,
		RecipeInventory:    recipeInventory,
		MixMissing:         missing,
		FormulaState:       formulaState,
		Reagents:           reagents,
		ReagentCandidates:  candidates,
		Hidden:             hidden,
		LearningSourceKind: learningSourceKind,
		LearningSource:     learningSource,
		LearningDate:       learningDate,
		Unread:             unread,
	}
}

func zStatsHiddenSpellFormula(
	inventoryReferences *references.InventoryItemReferences,
	knowledge KnowledgeEntry,
) (string, []string, [][]string) {
	if knowledge.SpellFormula == nil {
		return unknownSpellFormulaState, make([]string, 0), make([][]string, 0)
	}
	formula := knowledge.SpellFormula
	if formula.State == SpellFormulaConfirmed {
		return string(formula.State), zStatsReagentNames(inventoryReferences, formula.ConfirmedMask), make([][]string, 0)
	}

	candidates := make([][]string, 0, len(formula.CandidateMasks))
	for _, mask := range formula.CandidateMasks {
		candidates = append(candidates, zStatsReagentNames(inventoryReferences, mask))
	}

	return string(formula.State), make([]string, 0), candidates
}

func zStatsReagentNames(inventoryReferences *references.InventoryItemReferences, mask uint16) []string {
	names := make([]string, 0, 8)
	for reagent := references.SulfurAsh; reagent <= references.MandrakeRoot; reagent++ {
		if mask&(1<<reagent) == 0 {
			continue
		}
		names = append(names, zStatsInventoryItemName(inventoryReferences.GetReferenceByItem(reagent)))
	}

	return names
}

func zStatsMagicInventory(
	inventoryReferences *references.InventoryItemReferences,
	itemType references.ItemType,
	locker party_state.InventoryLockerSnapshot,
) []ZStatsInventoryItem {
	items := make([]ZStatsInventoryItem, 0, 8)
	for id := range 8 {
		item, valid := references.ItemFromIdentity(itemType, id)
		if !valid {
			panic("invalid Z-Stats magic inventory identity")
		}
		reference := inventoryReferences.GetReferenceByItem(item)
		var quantity uint16
		switch typed := item.(type) {
		case references.Reagent:
			quantity = locker.Reagents[typed]
		case references.Potion:
			quantity = locker.Potions[typed]
		case references.Scroll:
			quantity = locker.Scrolls[typed]
		default:
			panic("unsupported Z-Stats magic inventory type")
		}
		items = append(items, ZStatsInventoryItem{
			ID:          id,
			SpriteIndex: reference.ItemSprite,
			Name:        zStatsInventoryItemName(reference),
			Quantity:    quantity,
			Description: reference.ItemDescription,
		})
	}

	return items
}

func (g *GameState) projectZStatsOwnedItems(inventoryReferences *references.InventoryItemReferences) []ZStatsOwnedItem {
	items := make([]ZStatsOwnedItem, 0)
	for _, questItem := range references.QuestItemCatalog() {
		quantity := g.PartyState.Inventory.QuestItems.Get(questItem)
		if quantity == 0 {
			continue
		}
		reference := inventoryReferences.GetReferenceByItem(questItem)
		items = append(items, ZStatsOwnedItem{
			ID:          questItem.ID(),
			SpriteIndex: reference.ItemSprite,
			Category:    "Quest Item",
			Name:        zStatsInventoryItemName(reference),
			Quantity:    quantity,
			Description: reference.ItemDescription,
			MoonPhase:   ZStatsMoonPhase{},
		})
	}
	for _, moonstone := range references.MoonstoneCatalog() {
		quantity := g.PartyState.Inventory.Moonstones.Get(moonstone)
		if quantity == 0 {
			continue
		}
		reference := inventoryReferences.GetReferenceByItem(moonstone)
		items = append(items, ZStatsOwnedItem{
			ID:          moonstone.ID(),
			SpriteIndex: reference.ItemSprite,
			Category:    "Moonstone",
			Name:        zStatsInventoryItemName(reference),
			Quantity:    quantity,
			Description: reference.ItemDescription,
			MoonPhase:   ZStatsMoonPhase{Present: true, Phase: datetime.MoonPhase(moonstone.ID())},
		})
	}

	return items
}

func zStatsInventoryItemName(item references.InventoryItem) string {
	return item.FullDisplayName()
}
