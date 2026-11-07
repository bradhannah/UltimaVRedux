package save_snapshot

import (
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	maxSnapshotFrigateSkiffs = references.MaxFrigateSkiffs
	maxSnapshotSmashed       = 255
	shadowlordTownMax        = 8

	experimentationKnowledgeDisplayName = "Experimentation"
)

// Issue describes one snapshot validation failure.
type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Validate performs pure validation without mutating the snapshot.
func Validate(snapshot SaveSnapshot) []Issue {
	return validateSnapshot(snapshot, CurrentVersion)
}

//nolint:maintidx // Snapshot validation intentionally aggregates all independent sections.
func validateSnapshot(snapshot SaveSnapshot, expectedVersion int) []Issue {
	issues := make([]Issue, 0)
	add := func(path, code, message string) {
		issues = append(issues, Issue{Path: path, Code: code, Message: message})
	}
	if snapshot.Version != expectedVersion {
		add("version", "unsupported_version", fmt.Sprintf("version must be %d", expectedVersion))
	}
	if err := snapshot.LastConversation.Validate(); err != nil {
		add("lastConversation", "invalid", err.Error())
	}
	if expectedVersion >= 25 {
		if err := snapshot.Excerpts.Validate(); err != nil {
			add("excerpts", "invalid", err.Error())
		}
	}
	if expectedVersion >= 26 {
		if err := journal.ValidateNotes(snapshot.Notes); err != nil {
			add("notes", "invalid", err.Error())
		}
	}
	validateWorldJournal(snapshot, add)
	validateChestAssessments(snapshot, add)
	validateOpenDoor(snapshot, add)
	validatePersonalStateV24(snapshot, add)
	if err := journal.ValidatePresentation(snapshot.Presentation); err != nil {
		add("presentation", "invalid", err.Error())
	}
	if len(snapshot.Presentation.Visits.Changes) > journal.MaxChronicleEntries+11 {
		add("presentation", "out_of_range", "change count exceeds the version's finite catalog")
	}
	for _, change := range snapshot.Presentation.Visits.Changes {
		if change.Tab == journal.ChronicleTab && change.Entry > journal.MaxChronicleEntries {
			add("presentation", "out_of_range", "Chronicle key exceeds the version's finite catalog")
		}
	}
	if snapshot.Continuation.ActivePlayerIndex < -1 || snapshot.Continuation.ActivePlayerIndex > 5 {
		add("continuation.activePlayerIndex", "out_of_range", "active player index must be -1 or a party index from 0 through 5")
	}
	if snapshot.Continuation.BeggarKarmaCounter < 0 || snapshot.Continuation.BeggarKarmaCounter > 0xff {
		add("continuation.beggarKarmaCounter", "out_of_range", "beggar karma counter must fit one byte")
	}
	if snapshot.Continuation.CombatActionTimeCounter < 0 || snapshot.Continuation.CombatActionTimeCounter >= 10 {
		add("continuation.combatActionTimeCounter", "out_of_range", "combat action time counter must be from 0 through 9")
	}
	validateDungeonDiscoveries(snapshot.Redux.DungeonDiscoveries, add)
	validatePlayerKnowledge(snapshot.Redux.PlayerKnowledge, add)
	validateQuestJournal(snapshot.Redux.QuestJournal, snapshot.Progression.Shadowlords.Alive, add)
	validateParty(snapshot.Party, add)
	validatePosition("world.position", snapshot.World.Position, add)
	validatePosition("world.lastLargeMapPosition", snapshot.World.LastLargeMapPosition, add)
	validateDungeonSnapshot(snapshot, add)
	c := snapshot.World.Calendar
	if c.Year < 1 || c.Year > 65535 || c.Month < 1 || c.Month > datetime.MonthsPerYear || c.Day < 1 || c.Day > datetime.DaysInMonth || c.Hour < 0 || c.Hour >= datetime.HoursPerDay || c.Minute < 0 || c.Minute >= datetime.MinutesPerHour {
		add("world.calendar", "out_of_range", "calendar fields are outside authentic Ultima V ranges")
	}
	if snapshot.World.ElapsedMs < 0 || snapshot.World.LastCamped < 0 {
		add("world", "negative", "elapsed time and counters cannot be negative")
	}
	if snapshot.World.LastProcessedHour < 0 || snapshot.World.LastProcessedHour >= datetime.HoursPerDay || snapshot.World.LastWandererMonth < 0 || snapshot.World.LastWandererMonth > datetime.MonthsPerYear {
		add("world", "out_of_range", "processed hour or wanderer month is outside runtime ranges")
	}
	if snapshot.World.IsSailing {
		add("world.isSailing", "unsupported", "automatic sailing is workflow-owned and cannot be restored")
	}
	if snapshot.Party.Karma < 0 || snapshot.Party.Karma > 99 {
		add("party.karma", "out_of_range", "karma must be between 0 and 99")
	}
	for index, stone := range snapshot.Progression.Moonstones {
		if stone.State != 0 && stone.State != 1 && stone.State != 0xff {
			add(fmt.Sprintf("progression.moonstones[%d].state", index), "unsupported", "moonstone state is unsupported")
		}
	}
	for index, recipe := range snapshot.Progression.Recipes {
		if recipe.Mask > 0xff {
			add(fmt.Sprintf("progression.recipes[%d].mask", index), "out_of_range", "recipe mask exceeds eight reagents")
		}
	}
	shadowlords := snapshot.Progression.Shadowlords
	if shadowlords.LastCalled < -1 || shadowlords.LastCalled >= ShadowlordCount || shadowlords.AutomaticPresence < -1 || shadowlords.AutomaticPresence >= ShadowlordCount {
		add("progression.shadowlords", "out_of_range", "shadowlord identities are outside runtime ranges")
	}
	assignedTowns := make(map[int]struct{}, ShadowlordCount)
	for index, town := range shadowlords.TownAssignments {
		path := fmt.Sprintf("progression.shadowlords.townAssignments[%d]", index)
		if town < 0 || town > shadowlordTownMax {
			add(path, "out_of_range", "Shadowlord town assignment must be unassigned or town 1 through 8")

			continue
		}
		if !shadowlords.Alive[index] && town != 0 {
			add(path, "inconsistent", "defeated Shadowlords cannot retain a town assignment")
		}
		if town == 0 {
			continue
		}
		if _, exists := assignedTowns[town]; exists {
			add(path, "duplicate", "living Shadowlords must have unique town assignments")
		}
		assignedTowns[town] = struct{}{}
	}
	validateInventory(snapshot.Inventory, add)
	validateEffects(snapshot.Effects, add)
	if snapshot.Wind.Direction < 0 || snapshot.Wind.Direction > 4 || snapshot.Wind.UpdateCounter < 0 || snapshot.Wind.NextTickMs < 0 {
		add("wind", "out_of_range", "wind state is outside runtime ranges")
	}
	validateVehicle("partyVehicle", snapshot.PartyVehicle, add)
	pendingDeliveryCount := 0
	if snapshot.PartyVehicle != nil && snapshot.PartyVehicle.PendingDelivery {
		pendingDeliveryCount++
		add("partyVehicle.pendingDelivery", "inconsistent", "the boarded party vehicle cannot be pending delivery")
	}
	unitKeys := make(map[[3]int]struct{}, len(snapshot.MapUnits))
	controllerOrders := make(map[[2]int]map[int]struct{})
	for index, unit := range snapshot.MapUnits {
		path := fmt.Sprintf("mapUnits[%d]", index)
		if unit.Kind != MapUnitKindEnemy && unit.Kind != MapUnitKindNPC &&
			unit.Kind != MapUnitKindVehicle && unit.Kind != MapUnitKindChest {
			add(path+".kind", "unsupported", "map unit kind is unsupported")
		}
		if unit.World < -1 || unit.World > 0 || unit.ControllerOrder < 0 || unit.NPCNum < 0 || unit.NPCNum >= 32 || unit.ReferenceID < 0 {
			add(path, "out_of_range", "map unit controller identity is outside runtime ranges")
		}
		if unit.ArrivedScheduleSlot != nil && !references.NPCScheduleSlot(*unit.ArrivedScheduleSlot).IsValid() {
			add(path+".arrivedScheduleSlot", "out_of_range", "arrived schedule slot must identify one of the three NPC destinations")
		}
		if unit.ConsumedTalk3ScheduleSlot != nil {
			if unit.Kind != MapUnitKindNPC {
				add(path+".consumedTalk3ScheduleSlot", "inconsistent", "consumed TALK3 schedule slot is valid only for NPC units")
			}
			if !references.NPCScheduleSlot(*unit.ConsumedTalk3ScheduleSlot).IsValid() {
				add(path+".consumedTalk3ScheduleSlot", "out_of_range", "consumed TALK3 schedule slot must identify one of the three NPC destinations")
			}
		}
		if unit.SurfaceChestNumber != nil {
			if *unit.SurfaceChestNumber < 0 || *unit.SurfaceChestNumber > 0xff {
				add(path+".surfaceChestNumber", "out_of_range", "surface chest number must fit one byte")
			}
			if unit.Kind != MapUnitKindNPC && unit.Kind != MapUnitKindChest {
				add(path+".surfaceChestNumber", "inconsistent", "surface chest authority is valid only for NPC or chest units")
			}
		}
		if unit.Kind == MapUnitKindChest && unit.SurfaceChestNumber == nil {
			add(path+".surfaceChestNumber", "required", "chest units require surface chest state")
		}
		validateAIOverrideProvenance(path, unit, snapshot.World.Position.Location, add)
		validatePosition(path+".position", unit.Position, add)
		key := [3]int{unit.Position.Location, unit.World, unit.NPCNum}
		if _, exists := unitKeys[key]; exists {
			add(path, "duplicate", "map unit controller identity must be unique")
		}
		unitKeys[key] = struct{}{}
		controllerKey := [2]int{unit.Position.Location, unit.World}
		if controllerOrders[controllerKey] == nil {
			controllerOrders[controllerKey] = make(map[int]struct{})
		}
		if _, exists := controllerOrders[controllerKey][unit.ControllerOrder]; exists {
			add(path+".controllerOrder", "duplicate", "controller processing order must be unique")
		}
		controllerOrders[controllerKey][unit.ControllerOrder] = struct{}{}
		for pathIndex, position := range unit.CurrentPath {
			validatePosition(fmt.Sprintf("%s.currentPath[%d]", path, pathIndex), position, add)
		}
		validateVehicle(path+".vehicle", unit.Vehicle, add)
		if unit.Kind == MapUnitKindVehicle && unit.Vehicle == nil {
			add(path+".vehicle", "required", "vehicle details are required")
		}
		if unit.Vehicle != nil && unit.Vehicle.PendingDelivery {
			pendingDeliveryCount++
			pendingPath := path + ".vehicle.pendingDelivery"
			if pendingDeliveryCount > 1 {
				add(pendingPath, "duplicate", "only one vehicle may be pending delivery")
			}
			validatePendingDeliveryMapUnit(pendingPath, snapshot.World.Position, unit, add)
		}
		validateMapUnitEnemyState(path, unit, add)
	}
	for controller, orders := range controllerOrders {
		for order := range len(orders) {
			if _, exists := orders[order]; !exists {
				add("mapUnits", "noncontiguous_order", fmt.Sprintf("controller %d/%d processing order must be contiguous", controller[0], controller[1]))

				break
			}
		}
	}
	stackKeys := make(map[[4]int]struct{}, len(snapshot.ItemStacks))
	for index, stack := range snapshot.ItemStacks {
		path := fmt.Sprintf("itemStacks[%d]", index)
		validatePosition(path+".position", stack.Position, add)
		stackKey := [4]int{stack.Position.Location, stack.Position.X, stack.Position.Y, stack.Position.Floor}
		if _, exists := stackKeys[stackKey]; exists {
			add(path+".position", "duplicate", "only one item stack may occupy a map position")
		}
		stackKeys[stackKey] = struct{}{}
		if len(stack.Items) == 0 {
			add(path+".items", "required", "item stack cannot be empty")
		}
		for itemIndex, item := range stack.Items {
			if !supportedItemType(item.Type) || item.ID < 0 || item.Quantity == 0 {
				add(fmt.Sprintf("%s.items[%d]", path, itemIndex), "out_of_range", "item identity and quantity must be positive runtime values")
			}
		}
	}
	mutationKeys := make(map[string]struct{}, len(snapshot.MapMutations))
	for index, mutation := range snapshot.MapMutations {
		path := fmt.Sprintf("mapMutations[%d]", index)
		validatePosition(path+".position", mutation.Position, add)
		if mutation.Kind != MapMutationSetTile && mutation.Kind != MapMutationSetObject {
			add(path+".kind", "unsupported", "map mutation kind is unsupported")
		}
		if mutation.Value < -1 {
			add(path+".value", "out_of_range", "map mutation value must be a sprite index or the no-sprite sentinel")
		}
		mutationKey := fmt.Sprintf("%d/%d/%d/%d/%s", mutation.Position.Location, mutation.Position.X, mutation.Position.Y, mutation.Position.Floor, mutation.Kind)
		if _, exists := mutationKeys[mutationKey]; exists {
			add(path, "duplicate", "map mutation layer and position must be unique")
		}
		mutationKeys[mutationKey] = struct{}{}
	}

	return issues
}

func validateAIOverrideProvenance(
	path string,
	unit MapUnitSnapshot,
	activeLocation int,
	add func(string, string, string),
) {
	provenance := references.AIOverrideProvenance(unit.AIOverrideProvenance)
	if !provenance.IsValid() {
		add(path+".aiOverrideProvenance", "unsupported", "AI override provenance is unsupported")

		return
	}
	if unit.Kind != MapUnitKindNPC && provenance != references.AIOverrideProvenanceNone {
		add(path+".aiOverrideProvenance", "inconsistent", "AI override provenance is valid only for NPC units")

		return
	}

	aiType := references.AiType(unit.OverriddenAI)
	// Action 5 is supported in authored schedules, not in this frozen native
	// override enumeration. Loading the authored schedule requires no wire change.
	if !aiType.IsValid() || aiType == references.TalkAlways {
		add(path+".overriddenAi", "unsupported", "overridden AI type is unsupported")

		return
	}
	// Action 7 is produced by setattack; retain action 6 for shipped snapshots.
	switch provenance {
	case references.AIOverrideProvenanceNone:
		return
	case references.AIOverrideProvenanceAutomaticAstaroth:
		if aiType != references.ExtortOrAttackOrFollow && aiType != references.DrudgeWorthThing && aiType != references.FleeFromAvatar {
			add(path+".aiOverrideProvenance", "inconsistent", "automatic Astaroth provenance requires hostile or resulting flee AI")
		}
		if unit.Position.Location != activeLocation {
			add(path+".aiOverrideProvenance", "inconsistent", "automatic Astaroth provenance requires the active location")
		}
	case references.AIOverrideProvenanceAutomaticNosfentor:
		if aiType != references.FleeFromAvatar && aiType != references.ExtortOrAttackOrFollow && aiType != references.DrudgeWorthThing {
			add(path+".aiOverrideProvenance", "inconsistent", "automatic Nosfentor provenance requires flee or resulting hostile AI")
		}
		if unit.Position.Location != activeLocation {
			add(path+".aiOverrideProvenance", "inconsistent", "automatic Nosfentor provenance requires the active location")
		}
	case references.AIOverrideProvenanceGuardHostility:
		if aiType != references.ExtortOrAttackOrFollow && aiType != references.DrudgeWorthThing {
			add(path+".aiOverrideProvenance", "inconsistent", "guard hostility provenance requires hostile AI")
		}
	case references.AIOverrideProvenanceGenericFlee:
		if aiType != references.FleeFromAvatar {
			add(path+".aiOverrideProvenance", "inconsistent", "generic flee provenance requires flee AI")
		}
	}
}

func validateQuestJournal(
	events []QuestJournalEventSnapshot,
	shadowlordsAlive [ShadowlordCount]bool,
	add func(string, string, string),
) {
	if events == nil {
		add("redux.questJournal", "required", "quest journal must be a non-null array")

		return
	}
	seen := make(map[[2]string]struct{}, len(events))
	if len(events) > journal.MaxChronicleEntries {
		add("redux.questJournal", "out_of_range", "Chronicle exceeds the version's finite catalog")
		add("redux.questJournal", "out_of_range", fmt.Sprintf("quest journal contains at most %d first discoveries", journal.MaxChronicleEntries))
	}
	for index, event := range events {
		path := fmt.Sprintf("redux.questJournal[%d]", index)
		if event.Sequence != uint64(index+1) {
			add(path+".sequence", "not_contiguous", "quest journal sequence must match one-based insertion order")
		}
		key := [2]string{event.Kind, event.Subject}
		if _, exists := seen[key]; exists {
			add(path, "duplicate", "quest journal kind and subject must be unique")
		}
		seen[key] = struct{}{}
		if event.Kind == "item-acquired" {
			if journal.ItemMilestoneTitle(event.Subject) == "" || event.Source != QuestJournalSourceGameplay ||
				strings.TrimSpace(event.SourceName) == "" || event.OccurredOn == nil {
				add(path, "invalid", "invalid acquisition milestone")
			} else {
				date := event.OccurredOn
				if date.Year < 1 || date.Year > 65535 || date.Month < 1 || date.Month > datetime.MonthsPerYear || date.Day < 1 || date.Day > datetime.DaysInMonth {
					add(path, "invalid", "invalid acquisition date")
				}
			}

			continue
		}
		if event.Kind == QuestJournalEventKnowledgeLearned || event.Kind == QuestJournalEventCodexAnswerLearned {
			validateDiscoveryEvent(path, event, add)

			continue
		}
		if event.SourceName != "" {
			add(path+".sourceName", "inconsistent", "historical defeat sources have no name")
		}
		if event.Kind != QuestJournalEventShadowlordDefeated {
			add(path+".kind", "unsupported", "quest journal event kind is unsupported")
		}
		shadowlordIndex := -1
		switch event.Subject {
		case QuestJournalSubjectFaulinei:
			shadowlordIndex = int(references.Faulinei)
		case QuestJournalSubjectAstaroth:
			shadowlordIndex = int(references.Astaroth)
		case QuestJournalSubjectNosfentor:
			shadowlordIndex = int(references.Nosfentor)
		default:
			add(path+".subject", "unsupported", "quest journal subject is unsupported")
		}
		if event.Kind == QuestJournalEventShadowlordDefeated &&
			shadowlordIndex >= 0 && shadowlordsAlive[shadowlordIndex] {
			add(path, "inconsistent", "quest journal defeat event requires the Shadowlord to be defeated")
		}
		if event.Source != QuestJournalSourceGameplay && event.Source != QuestJournalSourceMigration {
			add(path+".source", "unsupported", "quest journal source is unsupported")
		}
		if event.Source == QuestJournalSourceGameplay && event.OccurredOn == nil {
			add(path+".occurredOn", "required", "gameplay quest journal events require a date")
		}
		if event.OccurredOn != nil {
			date := event.OccurredOn
			if date.Year < 1 || date.Year > 65535 || date.Month < 1 || date.Month > datetime.MonthsPerYear || date.Day < 1 || date.Day > datetime.DaysInMonth {
				add(path+".occurredOn", "out_of_range", "quest journal date is outside authentic Ultima V ranges")
			}
		}
		if event.Source == QuestJournalSourceMigration && (event.OccurredOn != nil || !event.Acknowledged) {
			add(path, "inconsistent", "migrated quest journal events require an unknown date and acknowledged state")
		}
	}
}

func validatePlayerKnowledge(entries []PlayerKnowledgeSnapshot, add func(string, string, string)) {
	if entries == nil {
		add("redux.playerKnowledge", "required", "player knowledge must be a non-null array")

		return
	}
	seen := make(map[[2]string]struct{}, len(entries))
	for index, entry := range entries {
		path := fmt.Sprintf("redux.playerKnowledge[%d]", index)
		key := [2]string{entry.Category, entry.Key}
		if _, exists := seen[key]; exists {
			add(path, "duplicate", "player knowledge category and key must be unique")
		}
		seen[key] = struct{}{}
		if index > 0 && !playerKnowledgeLess(entries[index-1], entry) {
			add(path, "not_sorted", "player knowledge must be sorted by category and key")
		}
		switch entry.Category {
		case KnowledgeCategorySpell:
			validateSpellKnowledge(path, entry, add)
		case KnowledgeCategoryShadowlord, KnowledgeCategoryShrine, KnowledgeCategoryJournal, KnowledgeCategoryDungeon:
			validateLoreKnowledge(path, entry, add)
		default:
			add(path+".category", "unsupported", "player knowledge category is unsupported")
		}
	}
}

func validateSpellKnowledge(path string, entry PlayerKnowledgeSnapshot, add func(string, string, string)) {
	definition, supported := hiddenSpellKnowledgeDefinitionFor(entry.Key)
	if !supported {
		add(path+".key", "unsupported", "spell knowledge key is not a hidden spell")
	}
	validateKnowledgeSource(path, entry, add)
	validateKnowledgeDate(path, entry.DiscoveredOn, add)
	if entry.SpellFormula != nil {
		validateSpellFormula(path+".spellFormula", entry.SpellFormula, definition, supported, add)
	}
}

func validateLoreKnowledge(path string, entry PlayerKnowledgeSnapshot, add func(string, string, string)) {
	if !supportedLoreKnowledgeKey(entry.Category, entry.Key) {
		add(path+".key", "unsupported", "lore knowledge key is unsupported for its category")
	}
	if entry.Source.Kind == KnowledgeSourceObservation || entry.Source.Kind == KnowledgeSourceRevelation {
		if strings.TrimSpace(entry.Source.DisplayName) == "" {
			add(path+".source.displayName", "required", "lore source requires a display name")
		}
	} else {
		validateKnowledgeSource(path, entry, add)
	}
	if entry.Source.Kind == KnowledgeSourceExperimentation || entry.Source.Kind == KnowledgeSourceMigration {
		add(path+".source.kind", "inconsistent", "lore knowledge must be learned through dialogue")
	}
	if entry.DiscoveredOn == nil {
		add(path+".discoveredOn", "required", "lore knowledge requires a discovery date")
	} else {
		validateKnowledgeDate(path, entry.DiscoveredOn, add)
	}
	if entry.Unread {
		add(path+".unread", "inconsistent", "lore knowledge cannot be unread")
	}
	if entry.SpellFormula != nil {
		add(path+".spellFormula", "inconsistent", "lore knowledge cannot contain a spell formula")
	}
}

func supportedLoreKnowledgeKey(category, key string) bool {
	switch category {
	case KnowledgeCategoryShadowlord:
		switch key {
		case ShadowlordKnowledgeFaulineiName, ShadowlordKnowledgeFaulineiAssociation,
			ShadowlordKnowledgeAstarothName, ShadowlordKnowledgeAstarothAssociation,
			ShadowlordKnowledgeNosfentorName, ShadowlordKnowledgeNosfentorAssociation:
			return true
		}
	case KnowledgeCategoryShrine:
		switch key {
		case ShrineKnowledgeHonestyMantra, ShrineKnowledgeCompassionMantra,
			ShrineKnowledgeValourMantra, ShrineKnowledgeJusticeMantra,
			ShrineKnowledgeSacrificeMantra, ShrineKnowledgeHonorMantra,
			ShrineKnowledgeSpiritualityMantra, ShrineKnowledgeHumilityMantra:
			return true
		}
	case KnowledgeCategoryJournal:
		switch key {
		case JournalKnowledgeDomainFalsehood, JournalKnowledgeDomainHatred, JournalKnowledgeDomainCowardice,
			JournalKnowledgeFlameTruth, JournalKnowledgeFlameLove, JournalKnowledgeFlameCourage, JournalKnowledgeBanishmentMethod,
			JournalKnowledgeResistancePassword, JournalKnowledgeStonesMelody, JournalKnowledgeOppressionPassword:
			return true
		}
	case KnowledgeCategoryDungeon:
		switch key {
		case DungeonKnowledgeDeceitWordOfPower, DungeonKnowledgeDespiseWordOfPower,
			DungeonKnowledgeDestardWordOfPower, DungeonKnowledgeWrongWordOfPower,
			DungeonKnowledgeCovetousWordOfPower, DungeonKnowledgeShameWordOfPower,
			DungeonKnowledgeHythlothWordOfPower, DungeonKnowledgeDoomWordOfPower:
			return true
		}
	}

	return false
}

func validateDiscoveryEvent(path string, event QuestJournalEventSnapshot, add func(string, string, string)) {
	supported := false
	if event.Kind == QuestJournalEventKnowledgeLearned {
		category, key, _ := strings.Cut(event.Subject, ".")
		supported = supportedLoreKnowledgeKey(category, key)
	} else {
		switch event.Subject {
		case "honesty", "compassion", "valour", "justice", "sacrifice", "honor", "spirituality", "humility":
			supported = true
		}
	}
	if !supported {
		add(path+".subject", "unsupported", "discovery subject is outside the finite journal catalog")
	}
	if event.Source != QuestJournalSourceDialogue && event.Source != QuestJournalSourceObservation && event.Source != QuestJournalSourceRevelation {
		add(path+".source", "unsupported", "discovery events require dialogue, observation, or revelation provenance")
	}
	if strings.TrimSpace(event.SourceName) == "" {
		add(path+".sourceName", "required", "discovery events require a source name")
	}
	if event.OccurredOn == nil {
		add(path+".occurredOn", "required", "discovery events require a date")
	} else if date := event.OccurredOn; date.Year < 1 || date.Year > 65535 || date.Month < 1 || date.Month > datetime.MonthsPerYear || date.Day < 1 || date.Day > datetime.DaysInMonth {
		add(path+".occurredOn", "out_of_range", "discovery date is outside authentic Ultima V ranges")
	}
}

func validateKnowledgeDate(path string, date *BritannianDateSnapshot, add func(string, string, string)) {
	if date != nil && (date.Year < 1 || date.Year > 65535 || date.Month < 1 || date.Month > datetime.MonthsPerYear || date.Day < 1 || date.Day > datetime.DaysInMonth) {
		add(path+".discoveredOn", "out_of_range", "knowledge date is outside authentic Ultima V ranges")
	}
}

func playerKnowledgeLess(left, right PlayerKnowledgeSnapshot) bool {
	if left.Category != right.Category {
		return left.Category < right.Category
	}

	return left.Key < right.Key
}

func hiddenSpellKnowledgeDefinitionFor(key string) (hiddenSpellKnowledgeDefinition, bool) {
	for _, definition := range hiddenSpellKnowledgeDefinitions() {
		if definition.key == key {
			return definition, true
		}
	}

	return hiddenSpellKnowledgeDefinition{}, false
}

func validateKnowledgeSource(path string, entry PlayerKnowledgeSnapshot, add func(string, string, string)) {
	source := entry.Source
	switch source.Kind {
	case KnowledgeSourceDialogue:
		if strings.TrimSpace(source.DisplayName) == "" {
			add(path+".source.displayName", "required", "dialogue knowledge source requires a display name")
		}
	case KnowledgeSourceExperimentation:
		if source.DisplayName != experimentationKnowledgeDisplayName {
			add(path+".source.displayName", "inconsistent", "experimentation knowledge source has an invalid display name")
		}
	case KnowledgeSourceMigration:
		if source.DisplayName != migratedKnowledgeDisplayName {
			add(path+".source.displayName", "inconsistent", "migrated knowledge source has an invalid display name")
		}
	default:
		add(path+".source.kind", "unsupported", "player knowledge source kind is unsupported")
	}
}

func validateSpellFormula(
	path string,
	formula *SpellFormulaKnowledgeSnapshot,
	definition hiddenSpellKnowledgeDefinition,
	supportedKey bool,
	add func(string, string, string),
) {
	if formula.CandidateMasks == nil {
		add(path+".candidateMasks", "required", "formula candidates must be a non-null array")
	}
	seen := make(map[uint16]struct{}, len(formula.CandidateMasks))
	for index, candidate := range formula.CandidateMasks {
		candidatePath := fmt.Sprintf("%s.candidateMasks[%d]", path, index)
		if candidate == 0 || candidate > 0xff {
			add(candidatePath, "out_of_range", "formula candidate must use one to eight reagent bits")
		}
		if _, exists := seen[candidate]; exists {
			add(candidatePath, "duplicate", "formula candidates must be unique")
		}
		seen[candidate] = struct{}{}
	}
	switch formula.State {
	case SpellFormulaUncertain:
		if formula.ConfirmedMask != 0 || len(formula.CandidateMasks) < 2 {
			add(path, "inconsistent", "uncertain formula requires at least two candidates and no confirmed mask")
		}
	case SpellFormulaConfirmed:
		if len(formula.CandidateMasks) != 0 {
			add(path+".candidateMasks", "inconsistent", "confirmed formula cannot retain candidates")
		}
		if formula.ConfirmedMask == 0 || formula.ConfirmedMask > 0xff {
			add(path+".confirmedMask", "out_of_range", "confirmed formula must use one to eight reagent bits")
		} else if supportedKey && formula.ConfirmedMask != references.GetSpellReagents(definition.spell) {
			add(path+".confirmedMask", "inconsistent", "confirmed formula must match the canonical hidden-spell recipe")
		}
	default:
		add(path+".state", "unsupported", "spell formula state is unsupported")
	}
}

func validateDungeonDiscoveries(
	discoveries []DungeonDiscoverySnapshot,
	add func(string, string, string),
) {
	if discoveries == nil {
		add("redux.dungeonDiscoveries", "required", "dungeon discoveries must be a non-null array")

		return
	}
	seen := make(map[[4]int]struct{}, len(discoveries))
	for index, discovery := range discoveries {
		path := fmt.Sprintf("redux.dungeonDiscoveries[%d]", index)
		if discovery.Location < int(references.Deceit) || discovery.Location > int(references.Doom) ||
			discovery.Floor < 0 || discovery.Floor >= references.DungeonFloorCount ||
			discovery.X < 0 || discovery.X >= int(references.XDungeonMapTiles) ||
			discovery.Y < 0 || discovery.Y >= int(references.YDungeonMapTiles) {
			add(path, "out_of_range", "dungeon discovery must address one dungeon cell")
		}
		if discovery.Kind != DungeonDiscoveryHiddenDoor && discovery.Kind != DungeonDiscoveryHiddenPit {
			add(path+".kind", "unsupported", "dungeon discovery kind is unsupported")
		}
		key := [4]int{discovery.Location, discovery.Floor, discovery.Y, discovery.X}
		if _, exists := seen[key]; exists {
			add(path, "duplicate", "dungeon discovery coordinates must be unique")
		}
		seen[key] = struct{}{}
		if index > 0 && !dungeonDiscoveryLess(discoveries[index-1], discovery) {
			add(path, "not_sorted", "dungeon discoveries must be sorted by location, floor, y, x, and kind")
		}
	}
}

func dungeonDiscoveryLess(left, right DungeonDiscoverySnapshot) bool {
	if left.Location != right.Location {
		return left.Location < right.Location
	}
	if left.Floor != right.Floor {
		return left.Floor < right.Floor
	}
	if left.Y != right.Y {
		return left.Y < right.Y
	}
	if left.X != right.X {
		return left.X < right.X
	}

	return left.Kind < right.Kind
}

func validateDungeonSnapshot(snapshot SaveSnapshot, add func(string, string, string)) {
	position := snapshot.World.Position
	isDungeon := references.Location(position.Location).GetMapType() == references.DungeonMapType
	if isDungeon && snapshot.Dungeon == nil {
		add("dungeon", "required", "active dungeon state is required at a dungeon location")

		return
	}
	if !isDungeon && snapshot.Dungeon != nil {
		add("dungeon", "inconsistent", "active dungeon state is forbidden outside a dungeon location")

		return
	}
	if snapshot.Dungeon == nil {
		return
	}
	if snapshot.PartyVehicle != nil {
		add("partyVehicle", "inconsistent", "the party cannot occupy a vehicle in a dungeon")
	}

	dungeon := snapshot.Dungeon
	if dungeon.Location != position.Location {
		add("dungeon.location", "inconsistent", "dungeon identity must match world position")
	}
	if dungeon.Location < int(references.Deceit) || dungeon.Location > int(references.Doom) {
		add("dungeon.location", "unsupported", "location is not a dungeon")
	}
	if position.Floor < 0 || position.Floor >= references.DungeonFloorCount ||
		position.X < 0 || position.X >= int(references.XDungeonMapTiles) ||
		position.Y < 0 || position.Y >= int(references.YDungeonMapTiles) {
		add("world.position", "out_of_range", "active dungeon position must address the 8x8x8 dungeon map")
	}
	if len(dungeon.Cells) != DungeonCellCount {
		add("dungeon.cells", "wrong_length", fmt.Sprintf("dungeon cells must contain exactly %d raw bytes", DungeonCellCount))
	}
	for index, cell := range dungeon.Cells {
		if cell < 0 || cell > 255 {
			add(fmt.Sprintf("dungeon.cells[%d]", index), "out_of_range", "raw dungeon cell must be a byte")
		}
	}
	if dungeon.Facing < int(references.DungeonFacingNorth) || dungeon.Facing > int(references.DungeonFacingWest) {
		add("dungeon.facing", "out_of_range", "dungeon facing must be one of the four cardinal values")
	}
	if dungeon.LastDirection < int(references.DungeonLastNorth) || dungeon.LastDirection > int(references.DungeonArrivedFromAbove) {
		add("dungeon.lastDirection", "out_of_range", "dungeon last direction must be cardinal or vertical")
	}
	if dungeon.Wanderer == nil {
		return
	}

	wanderer := dungeon.Wanderer
	monster := references.MonsterType(wanderer.Monster)
	if !monster.IsValid() {
		add("dungeon.wanderer.monster", "out_of_range", "dungeon wanderer monster is invalid")
	} else if !references.IsDungeonWanderingMonster(monster) {
		add("dungeon.wanderer.monster", "unsupported", "monster is not in the dungeon wandering pool")
	}
	if wanderer.OnCeiling && monster != references.MonsterSpider && monster != references.MonsterSlime {
		add("dungeon.wanderer.onCeiling", "inconsistent", "only spiders and slimes can occupy the ceiling")
	}
	if wanderer.Floor < 0 || wanderer.Floor >= references.DungeonFloorCount ||
		wanderer.X < 0 || wanderer.X >= int(references.XDungeonMapTiles) ||
		wanderer.Y < 0 || wanderer.Y >= int(references.YDungeonMapTiles) {
		add("dungeon.wanderer", "out_of_range", "dungeon wanderer floor or current position is invalid")
	}
	if wanderer.Floor < 0 || wanderer.Floor >= references.DungeonFloorCount ||
		wanderer.PreviousX < 0 || wanderer.PreviousX >= int(references.XDungeonMapTiles) ||
		wanderer.PreviousY < 0 || wanderer.PreviousY >= int(references.YDungeonMapTiles) {
		add("dungeon.wanderer", "out_of_range", "dungeon wanderer floor or previous position is invalid")
	}
	if wanderer.Floor == position.Floor && wanderer.X == position.X && wanderer.Y == position.Y {
		add("dungeon.wanderer", "occupied", "dungeon wanderer cannot occupy the party position")
	}
}

func validateParty(party PartySnapshot, add func(string, string, string)) {
	activeCount := 0
	seenInactive := false
	if !party.Roster[0].Occupied {
		add("party.roster[0].occupied", "required", "roster slot zero must contain the Avatar")
	}
	for index, character := range party.Roster {
		path := fmt.Sprintf("party.roster[%d]", index)
		if !character.Occupied {
			if character != (CharacterSnapshot{}) {
				add(path, "nonempty_vacant_slot", "vacant roster slots must contain zero values")
			}
			seenInactive = true

			continue
		}
		if character.Name == "" || len(character.Name) > 9 || character.Level < 1 || character.Level > 255 || character.Experience < 0 || character.Experience > 65535 ||
			character.Strength < 0 || character.Strength > 255 || character.Dexterity < 0 || character.Dexterity > 255 || character.Intelligence < 0 || character.Intelligence > 255 ||
			character.CurrentHP < 0 || character.MaximumHP < character.CurrentHP || character.MaximumHP > 65535 || character.CurrentMP < 0 || character.CurrentMP > character.Intelligence ||
			character.MonthsAtInn < 0 || character.MonthsAtInn > 255 || character.ArmorClass < 0 || character.ArmorClass > 255 ||
			character.Helmet < 0 || character.Helmet > 255 || character.Armor < 0 || character.Armor > 255 || character.Weapon < 0 || character.Weapon > 255 || character.Shield < 0 || character.Shield > 255 || character.Ring < 0 || character.Ring > 255 || character.Amulet < 0 || character.Amulet > 255 {
			add(path, "out_of_range", "occupied character has invalid identity, level, or vitals")
		}
		validateCharacterIdentity(path, character, add)
		if index == 0 {
			if validCharacterClass(character.Class) && character.Class != int(party_state.Avatar) {
				add(path+".class", "inconsistent", "roster slot zero must contain the Avatar class")
			}
			if validPartyMap(character.PartyMap) && character.PartyMap != int(party_state.PartyMapActive) {
				add(path+".partyMap", "inconsistent", "the Avatar in roster slot zero must be active")
			}
		}
		active := character.PartyMap == int(party_state.PartyMapActive)
		if active && (seenInactive || activeCount == party_state.MaxCharactersInParty) {
			add(path+".partyMap", "noncontiguous_active_party", "active characters must occupy a packed prefix of at most six slots")
		}
		if active {
			activeCount++
		} else {
			seenInactive = true
		}
	}
	if activeCount == 0 {
		add("party.roster", "missing_active_party", "the Avatar must lead a non-empty active roster")
	}
	seen := make(map[[2]int]struct{})
	for _, group := range []struct {
		path  string
		flags []NPCFlagSnapshot
	}{{"party.metNpcs", party.MetNPCs}, {"party.deadNpcs", party.DeadNPCs}} {
		clear(seen)
		for index, flag := range group.flags {
			key := [2]int{flag.Location, flag.NPC}
			if flag.Location < 0 || flag.Location >= int(references.SerpentsHold-references.Moonglow+1) ||
				flag.NPC < 0 || flag.NPC >= 32 {
				add(fmt.Sprintf("%s[%d]", group.path, index), "out_of_range", "NPC flag is outside native v5 ranges")
			}
			if _, exists := seen[key]; exists {
				add(fmt.Sprintf("%s[%d]", group.path, index), "duplicate", "NPC flag must be unique")
			}
			seen[key] = struct{}{}
		}
	}
}

func validateCharacterIdentity(path string, character CharacterSnapshot, add func(string, string, string)) {
	if !validCharacterGender(character.Gender) {
		add(path+".gender", "unsupported", "character gender must be male or female")
	}
	if !validCharacterClass(character.Class) {
		add(path+".class", "unsupported", "character class must be Avatar, Fighter, Bard, or Mage")
	}
	if !validCharacterStatus(character.Status) {
		add(path+".status", "unsupported", "character status must be good, poisoned, charmed, sleeping, or dead")
	}
	if !validPartyMap(character.PartyMap) {
		add(path+".partyMap", "unsupported", "party map must be active, an Inn map ID, martyr, or unrecruited")
	}
}

func validCharacterGender(gender int) bool {
	return gender == int(party_state.Male) || gender == int(party_state.Female)
}

func validCharacterClass(class int) bool {
	switch class {
	case int(party_state.Avatar), int(party_state.Fighter), int(party_state.Bard), int(party_state.Wizard):
		return true
	default:
		return false
	}
}

func validCharacterStatus(status int) bool {
	switch status {
	case int(party_state.Good), int(party_state.Poisoned), int(party_state.Charmed), int(party_state.Sleep), int(party_state.Dead):
		return true
	default:
		return false
	}
}

func validateInventory(inventory InventorySnapshot, add func(string, string, string)) {
	if inventory.Food > 9999 || inventory.Gold > 9999 || inventory.Gems > 99 || inventory.Torches > 99 || inventory.Keys > 99 || inventory.SkullKeys > 99 {
		add("inventory", "out_of_range", "provision quantity exceeds runtime capacity")
	}
	for index, quantity := range inventory.Equipment {
		if quantity > 99 {
			add(fmt.Sprintf("inventory.equipment[%d]", index), "out_of_range", "equipment quantity exceeds runtime capacity")
		}
	}
	for index, quantity := range inventory.Spells {
		if quantity > 99 {
			add(fmt.Sprintf("inventory.spells[%d]", index), "out_of_range", "spell quantity exceeds runtime capacity")
		}
	}
	for index, quantity := range inventory.Scrolls {
		if quantity > 9999 {
			add(fmt.Sprintf("inventory.scrolls[%d]", index), "out_of_range", "scroll quantity exceeds runtime capacity")
		}
	}
	groups := []struct {
		name       string
		quantities []uint16
	}{{"potions", inventory.Potions[:]}, {"reagents", inventory.Reagents[:]}, {"questItems", inventory.QuestItems[:]}, {"moonstones", inventory.Moonstones[:]}}
	for _, group := range groups {
		name, quantities := group.name, group.quantities
		for index, quantity := range quantities {
			if quantity > 99 {
				add(fmt.Sprintf("inventory.%s[%d]", name, index), "out_of_range", "quantity exceeds runtime capacity")
			}
		}
	}
}

func validateEffects(effects EffectsSnapshot, add func(string, string, string)) {
	if effects.Drunk < 0 {
		add("effects.drunk", "out_of_range", "drunk cannot be negative")
	}
	if effects.Smashed < 0 || effects.Smashed > maxSnapshotSmashed {
		add("effects.smashed", "out_of_range", fmt.Sprintf("smashed must be between 0 and %d", maxSnapshotSmashed))
	}
	if effects.MagicLightMinutesRemaining < 0 {
		add("effects.magicLightMinutesRemaining", "negative", "magical-light minutes cannot be negative")
	}
	if effects.CarriedTorchMinutesRemaining < 0 {
		add("effects.carriedTorchMinutesRemaining", "negative", "carried-torch minutes cannot be negative")
	}
	switch effects.DurationType {
	case 0:
		if effects.DurationTurns != 0 || effects.DurationToggleOnly {
			add("effects.durationType", "inconsistent", "an empty active-spell slot must have zero turns and cannot be toggle-only")
		}
	case 'P', 'N', 'T', 'Q', 'C':
		if effects.DurationTurns <= 0 || effects.DurationToggleOnly {
			add("effects.durationType", "inconsistent", "an active spell requires positive turns and cannot be toggle-only")
		}
	default:
		add("effects.durationType", "unsupported", "active spell code is unsupported")
	}
	for _, entry := range []struct {
		name   string
		toggle ToggleSnapshot
	}{{"amulet", effects.Amulet}, {"crown", effects.Crown}, {"badge", effects.Badge}} {
		name, toggle := entry.name, entry.toggle
		if toggle.Turns < 0 || (!toggle.Active && toggle.Turns != 0) {
			add("effects."+name, "inconsistent", "inactive toggles must have zero non-negative turns")
		}
	}
}

func validPartyMap(partyMap int) bool {
	return partyMap == int(party_state.PartyMapActive) ||
		partyMap == int(party_state.PartyMapMartyr) ||
		partyMap == int(party_state.PartyMapUnrecruited) ||
		(partyMap >= int(references.Moonglow) && partyMap <= int(references.Doom))
}

func validateMapUnitEnemyState(path string, unit MapUnitSnapshot, add func(string, string, string)) {
	if unit.Kind != MapUnitKindEnemy {
		if unit.EnemyVariant != "" || unit.EnemyCurrentHP != 0 || unit.EnemyFacing != 0 || unit.Shadowlord != nil {
			add(path, "inconsistent", "non-enemy units cannot contain enemy instance state")
		}

		return
	}
	if unit.EnemyCurrentHP < 0 || unit.EnemyCurrentHP > 65535 || unit.EnemyFacing < 0 || unit.EnemyFacing > 4 {
		add(path, "out_of_range", "enemy hit points or facing are outside runtime ranges")
	}
	switch unit.EnemyVariant {
	case "standard", "whirlpool":
		if unit.EnemyFacing != 0 || unit.Shadowlord != nil {
			add(path, "inconsistent", "this enemy variant cannot have facing or Shadowlord identity")
		}
	case "pirate":
		if unit.EnemyFacing == 0 || unit.Shadowlord != nil {
			add(path, "inconsistent", "pirates require a cardinal facing and no Shadowlord identity")
		}
	case "shadowlord":
		if unit.EnemyFacing != 0 || unit.Shadowlord == nil || *unit.Shadowlord < 0 || *unit.Shadowlord >= ShadowlordCount {
			add(path, "inconsistent", "Shadowlord enemies require an exact identity and no facing")
		}
	default:
		add(path+".enemyVariant", "unsupported", "enemy variant is unsupported")
	}
}

func validateVehicle(path string, vehicle *VehicleSnapshot, add func(string, string, string)) {
	if vehicle == nil {
		return
	}
	if vehicle.Type < 1 || vehicle.Type > 4 || vehicle.CurrentDirection < 0 || vehicle.CurrentDirection > 4 || vehicle.PreviousDirection < 0 || vehicle.PreviousDirection > 4 || vehicle.Skiffs < 0 || vehicle.Skiffs > maxSnapshotFrigateSkiffs || vehicle.Hull < 0 || vehicle.Hull > 99 || vehicle.SailState < 0 || vehicle.SailState > 1 {
		add(path, "out_of_range", "vehicle fields are outside runtime ranges")
	}
	if vehicle.Type != int(references.FrigateVehicle) && vehicle.Skiffs != 0 {
		add(path+".skiffs", "inconsistent", "only frigates can carry skiffs")
	}
	if vehicle.Type == int(references.FrigateVehicle) && vehicle.Hull == 0 {
		add(path+".hull", "inconsistent", "an active frigate must have positive hull")
	}
}

func validatePendingDeliveryMapUnit(
	path string,
	activePosition PositionSnapshot,
	unit MapUnitSnapshot,
	add func(string, string, string),
) {
	vehicleType := references.VehicleType(unit.Vehicle.Type)
	if unit.Kind != MapUnitKindVehicle || (vehicleType != references.FrigateVehicle && vehicleType != references.SkiffVehicle) {
		add(path, "inconsistent", "pending delivery is valid only for a map-unit frigate or skiff")
	}
	if unit.Position.Location != int(references.BritanniaUnderworld) ||
		unit.World != int(references.OVERWORLD) || unit.Position.Floor != int(references.OVERWORLD) {
		add(path, "inconsistent", "pending delivery must be on Britannia's overworld main floor")
	}
	if !unit.Visible || unit.Vehicle.OwnedByNPC {
		add(path, "inconsistent", "a pending-delivery vehicle must be visible and unowned")
	}
	vehicle := unit.Vehicle
	if vehicle.Hull != 99 || vehicle.SailState != 0 ||
		vehicle.CurrentDirection != int(references.Right) ||
		vehicle.PreviousDirection != int(references.Right) || vehicle.HasHMSCape {
		add(path, "inconsistent", "a pending-delivery vehicle must retain default hull, sail, direction, and HMS Cape state")
	}
	if vehicleType == references.FrigateVehicle &&
		(vehicle.Skiffs < 2 || vehicle.Skiffs > maxSnapshotFrigateSkiffs) {
		add(path, "inconsistent", fmt.Sprintf(
			"a pending-delivery frigate must carry between 2 and %d skiffs",
			maxSnapshotFrigateSkiffs,
		))
	}
	if vehicleType == references.SkiffVehicle && vehicle.Skiffs != 0 {
		add(path, "inconsistent", "a pending-delivery standalone skiff cannot carry skiffs")
	}
	if activePosition.Location == int(references.BritanniaUnderworld) &&
		activePosition.Floor == int(references.OVERWORLD) {
		add(path, "inconsistent", "pending delivery cannot persist at a stable active-overworld boundary")
	}
}

func validatePosition(path string, position PositionSnapshot, add func(string, string, string)) {
	if position.Location < 0 || position.Location > 255 || position.X < 0 || position.X > 255 || position.Y < 0 || position.Y > 255 || position.Floor < -128 || position.Floor > 127 {
		add(path, "out_of_range", "location, coordinates, or floor exceed runtime storage")
	}
}

func supportedItemType(itemType int) bool {
	switch itemType {
	case 0, 1, 2, 4, 5, 7, 8, 9:
		return true
	default:
		return false
	}
}
