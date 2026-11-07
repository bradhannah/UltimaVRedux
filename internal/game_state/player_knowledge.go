package game_state

import (
	"sort"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// KnowledgeCategory identifies one durable family of player knowledge.
type KnowledgeCategory string

const (
	// KnowledgeCategorySpell contains the three discoveries omitted from the Book of Lore.
	KnowledgeCategorySpell KnowledgeCategory = "spell"
	// KnowledgeCategoryShadowlord contains learned Shadowlord names and associations.
	KnowledgeCategoryShadowlord KnowledgeCategory = "shadowlord"
	// KnowledgeCategoryShrine contains learned shrine mantras.
	KnowledgeCategoryShrine KnowledgeCategory = "shrine"
	// KnowledgeCategoryJournal contains independent domain, Flame, and method discoveries.
	KnowledgeCategoryJournal KnowledgeCategory = "journal"
	// KnowledgeCategoryDungeon contains the eight learned Words of Power.
	KnowledgeCategoryDungeon KnowledgeCategory = "dungeon"
)

const (
	spellKnowledgeAnYlem    = "an-ylem"
	spellKnowledgeInXenMani = "in-xen-mani"
	spellKnowledgeRelXenBet = "rel-xen-bet"
)

// ShadowlordKnowledgeKind identifies one independently discoverable fact.
type ShadowlordKnowledgeKind string

const (
	// ShadowlordKnowledgeName identifies knowledge of a Shadowlord's name.
	ShadowlordKnowledgeName ShadowlordKnowledgeKind = "name"
	// ShadowlordKnowledgeAssociation identifies knowledge of a Shadowlord's anti-virtue.
	ShadowlordKnowledgeAssociation ShadowlordKnowledgeKind = "association"
)

// KnowledgeKey is a stable category and semantic identity pair.
type KnowledgeKey struct {
	Category KnowledgeCategory
	ID       string
}

// KnowledgeSourceKind identifies how the player learned or confirmed an entry.
type KnowledgeSourceKind string

const (
	// KnowledgeSourceDialogue identifies knowledge learned from a named speaker.
	KnowledgeSourceDialogue KnowledgeSourceKind = "dialogue"
	// KnowledgeSourceExperimentation identifies knowledge learned by successful mixing.
	KnowledgeSourceExperimentation KnowledgeSourceKind = "experimentation"
	// KnowledgeSourceMigration identifies knowledge inferred from an older save.
	KnowledgeSourceMigration KnowledgeSourceKind = "migration"
	// KnowledgeSourceObservation identifies knowledge learned by observing the world.
	KnowledgeSourceObservation KnowledgeSourceKind = "observation"
	// KnowledgeSourceRevelation identifies knowledge explicitly revealed by an event.
	KnowledgeSourceRevelation KnowledgeSourceKind = "revelation"
)

const (
	experimentationKnowledgeDisplayName = "Experimentation"
	migratedKnowledgeDisplayName        = "Previously learned; date unknown"
)

// KnowledgeSource records the provenance kind and player-facing source name.
type KnowledgeSource struct {
	Kind        KnowledgeSourceKind
	DisplayName string
}

// DialogueKnowledgeSource constructs provenance for a named dialogue speaker.
func DialogueKnowledgeSource(displayName string) KnowledgeSource {
	if strings.TrimSpace(displayName) == "" {
		panic("dialogue knowledge source requires a display name")
	}

	return KnowledgeSource{Kind: KnowledgeSourceDialogue, DisplayName: displayName}
}

// ObservationKnowledgeSource constructs provenance for an observed place or object.
func ObservationKnowledgeSource(name string) KnowledgeSource {
	source := KnowledgeSource{Kind: KnowledgeSourceObservation, DisplayName: name}
	validateLoreKnowledgeSource(source)

	return source
}

// RevelationKnowledgeSource constructs provenance for an explicit revelation.
func RevelationKnowledgeSource(name string) KnowledgeSource {
	source := KnowledgeSource{Kind: KnowledgeSourceRevelation, DisplayName: name}
	validateLoreKnowledgeSource(source)

	return source
}

// ExperimentationKnowledgeSource constructs provenance for successful mixing.
func ExperimentationKnowledgeSource() KnowledgeSource {
	return KnowledgeSource{Kind: KnowledgeSourceExperimentation, DisplayName: experimentationKnowledgeDisplayName}
}

// SpellFormulaState identifies whether a stored formula is tentative or canonical.
type SpellFormulaState string

const (
	// SpellFormulaUncertain records multiple candidates without selecting one.
	SpellFormulaUncertain SpellFormulaState = "uncertain"
	// SpellFormulaConfirmed records the canonical reagent mask.
	SpellFormulaConfirmed SpellFormulaState = "confirmed"
)

// SpellFormulaKnowledge is the optional formula detail attached to spell knowledge.
type SpellFormulaKnowledge struct {
	State          SpellFormulaState
	ConfirmedMask  uint16
	CandidateMasks []uint16
}

// KnowledgeEntry is a detached durable player-knowledge value.
type KnowledgeEntry struct {
	Key          KnowledgeKey
	Source       KnowledgeSource
	DiscoveredOn *datetime.BritannianDate
	Unread       bool
	SpellFormula *SpellFormulaKnowledge
}

type playerKnowledge struct {
	entries map[KnowledgeKey]KnowledgeEntry
}

func newPlayerKnowledge() *playerKnowledge {
	return &playerKnowledge{entries: make(map[KnowledgeKey]KnowledgeEntry)}
}

// SpellKnowledgeKey returns the stable knowledge key for a hidden spell.
func SpellKnowledgeKey(spell references.Spell) (KnowledgeKey, bool) {
	if spell == references.AnYlem {
		return KnowledgeKey{Category: KnowledgeCategorySpell, ID: spellKnowledgeAnYlem}, true
	}
	if spell == references.InXenMani {
		return KnowledgeKey{Category: KnowledgeCategorySpell, ID: spellKnowledgeInXenMani}, true
	}
	if spell == references.RelXenBet {
		return KnowledgeKey{Category: KnowledgeCategorySpell, ID: spellKnowledgeRelXenBet}, true
	}

	return KnowledgeKey{}, false
}

// ShadowlordKnowledgeKey returns the stable key for one Shadowlord fact.
func ShadowlordKnowledgeKey(lord references.Shadowlord, kind ShadowlordKnowledgeKind) (KnowledgeKey, bool) {
	var identity string
	switch lord {
	case references.Faulinei:
		identity = "faulinei"
	case references.Astaroth:
		identity = "astaroth"
	case references.Nosfentor:
		identity = "nosfentor"
	case references.NoShadowlord:
		return KnowledgeKey{}, false
	default:
		return KnowledgeKey{}, false
	}
	if kind != ShadowlordKnowledgeName && kind != ShadowlordKnowledgeAssociation {
		return KnowledgeKey{}, false
	}

	return KnowledgeKey{Category: KnowledgeCategoryShadowlord, ID: identity + "." + string(kind)}, true
}

// ShrineMantraKnowledgeKey returns the stable key for one shrine's mantra.
func ShrineMantraKnowledgeKey(shrineIndex int) (KnowledgeKey, bool) {
	ids := [...]string{
		"honesty.mantra",
		"compassion.mantra",
		"valour.mantra",
		"justice.mantra",
		"sacrifice.mantra",
		"honor.mantra",
		"spirituality.mantra",
		"humility.mantra",
	}
	if shrineIndex < 0 || shrineIndex >= len(ids) {
		return KnowledgeKey{}, false
	}

	return KnowledgeKey{Category: KnowledgeCategoryShrine, ID: ids[shrineIndex]}, true
}

// DiscoverSpellKnowledge records one of the three spells omitted from the Book
// of Lore. It returns true only when durable knowledge changed.
func (g *GameState) DiscoverSpellKnowledge(spell references.Spell, source KnowledgeSource) bool {
	g.requirePlayerKnowledge()
	validateKnowledgeSource(source, false)
	key, supported := SpellKnowledgeKey(spell)
	if !supported {
		return false
	}

	formula := discoveredSpellFormula(spell)
	if existing, found := g.playerKnowledge.entries[key]; found {
		if existing.SpellFormula != nil {
			return false
		}
		existing.Source = source
		existing.DiscoveredOn = currentKnowledgeDate(g.DateTime)
		existing.Unread = true
		existing.SpellFormula = formula
		g.playerKnowledge.entries[key] = existing

		return true
	}

	g.playerKnowledge.entries[key] = KnowledgeEntry{
		Key: key, Source: source, DiscoveredOn: currentKnowledgeDate(g.DateTime), Unread: true,
		SpellFormula: formula,
	}

	return true
}

// ConfirmSpellKnowledge records the canonical formula for a hidden spell. It
// may create the discovery when successful experimentation is the first source.
func (g *GameState) ConfirmSpellKnowledge(spell references.Spell, source KnowledgeSource) bool {
	g.requirePlayerKnowledge()
	validateKnowledgeSource(source, false)
	key, supported := SpellKnowledgeKey(spell)
	if !supported {
		return false
	}
	canonical := references.GetSpellReagents(spell)
	if existing, found := g.playerKnowledge.entries[key]; found {
		if existing.SpellFormula != nil &&
			existing.SpellFormula.State == SpellFormulaConfirmed &&
			existing.SpellFormula.ConfirmedMask == canonical {
			return false
		}
		existing.Source = source
		existing.DiscoveredOn = currentKnowledgeDate(g.DateTime)
		existing.Unread = true
		existing.SpellFormula = confirmedSpellFormula(spell)
		g.playerKnowledge.entries[key] = existing

		return true
	}

	g.playerKnowledge.entries[key] = KnowledgeEntry{
		Key: key, Source: source, DiscoveredOn: currentKnowledgeDate(g.DateTime), Unread: true,
		SpellFormula: confirmedSpellFormula(spell),
	}

	return true
}

// DiscoverShadowlordKnowledge records one independently learned Shadowlord fact.
// Repeated discoveries preserve the original provenance and date.
func (g *GameState) DiscoverShadowlordKnowledge(
	lord references.Shadowlord,
	kind ShadowlordKnowledgeKind,
	source KnowledgeSource,
) bool {
	g.requirePlayerKnowledge()
	validateLoreKnowledgeSource(source)
	key, supported := ShadowlordKnowledgeKey(lord, kind)
	if !supported {
		return false
	}

	return g.discoverLoreKnowledge(key, source)
}

// DiscoverShrineMantraKnowledge records one learned shrine mantra. Repeated
// discoveries preserve the original provenance and date.
func (g *GameState) DiscoverShrineMantraKnowledge(shrineIndex int, source KnowledgeSource) bool {
	g.requirePlayerKnowledge()
	validateLoreKnowledgeSource(source)
	key, supported := ShrineMantraKnowledgeKey(shrineIndex)
	if !supported {
		return false
	}

	return g.discoverLoreKnowledge(key, source)
}

// WordOfPowerKnowledgeKey accepts only lowercase canonical dungeon names.
func WordOfPowerKnowledgeKey(dungeonName string) (KnowledgeKey, bool) {
	for _, candidate := range canonicalDungeonWords {
		if dungeonName == strings.ToLower(candidate.dungeon.String()) {
			return KnowledgeKey{Category: KnowledgeCategoryDungeon, ID: dungeonName + ".word_of_power"}, true
		}
	}

	return KnowledgeKey{}, false
}

// DiscoverWordOfPowerKnowledge records an actually revealed Word of Power once.
// Knowing a word does not unseal its dungeon or change any shrine progress.
func (g *GameState) DiscoverWordOfPowerKnowledge(dungeonName string, source KnowledgeSource) bool {
	g.requirePlayerKnowledge()
	validateLoreKnowledgeSource(source)
	key, supported := WordOfPowerKnowledgeKey(dungeonName)
	if !supported {
		return false
	}

	return g.discoverLoreKnowledge(key, source)
}

// WordOfPowerKnowledge returns detached provenance for a learned dungeon word.
func (g *GameState) WordOfPowerKnowledge(dungeonName string) (KnowledgeEntry, bool) {
	g.requirePlayerKnowledge()
	key, supported := WordOfPowerKnowledgeKey(dungeonName)
	if !supported {
		return KnowledgeEntry{}, false
	}

	return g.knowledge(key)
}

// JournalKnowledgeKey accepts only the finite, category-stripped Journal keys.
func JournalKnowledgeKey(id string) (KnowledgeKey, bool) {
	switch id {
	case "domain.falsehood", "domain.hatred", "domain.cowardice",
		"flame.truth", "flame.love", "flame.courage", "banishment.method",
		"resistance.password", "oppression.password", "stones.melody":
		return KnowledgeKey{Category: KnowledgeCategoryJournal, ID: id}, true
	default:
		return KnowledgeKey{}, false
	}
}

// DiscoverJournalKnowledge records an independent clue, never an inferred name binding.
func (g *GameState) DiscoverJournalKnowledge(id string, source KnowledgeSource) bool {
	g.requirePlayerKnowledge()
	validateLoreKnowledgeSource(source)
	key, supported := JournalKnowledgeKey(id)
	if !supported {
		return false
	}
	if _, known := g.JournalKnowledge(id); known {
		return false
	}

	return g.discoverLoreKnowledge(key, source)
}

// JournalKnowledge returns an independent clue or an explicitly known domain.
// Association-only saves remain valid; implication preserves the association's provenance.
func (g *GameState) JournalKnowledge(id string) (KnowledgeEntry, bool) {
	g.requirePlayerKnowledge()
	key, supported := JournalKnowledgeKey(id)
	if !supported {
		return KnowledgeEntry{}, false
	}
	if entry, found := g.knowledge(key); found {
		return entry, true
	}
	for lord := references.Faulinei; lord < references.ShadowlordCount; lord++ {
		if id == "domain."+strings.ToLower(lord.GetAntiVirtue()) {
			entry, found := g.ShadowlordKnowledge(lord, ShadowlordKnowledgeAssociation)
			entry.Key = key

			return entry, found
		}
	}

	return KnowledgeEntry{}, false
}

// KnowledgeSnapshot returns stable-sorted detached copies of all knowledge.
func (g *GameState) KnowledgeSnapshot() []KnowledgeEntry {
	g.requirePlayerKnowledge()
	entries := make([]KnowledgeEntry, 0, len(g.playerKnowledge.entries))
	for _, entry := range g.playerKnowledge.entries {
		entries = append(entries, cloneKnowledgeEntry(entry))
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Key.Category != entries[j].Key.Category {
			return entries[i].Key.Category < entries[j].Key.Category
		}

		return entries[i].Key.ID < entries[j].Key.ID
	})

	return entries
}

// SpellKnowledge returns a detached hidden-spell entry when present.
func (g *GameState) SpellKnowledge(spell references.Spell) (KnowledgeEntry, bool) {
	g.requirePlayerKnowledge()
	key, supported := SpellKnowledgeKey(spell)
	if !supported {
		return KnowledgeEntry{}, false
	}
	entry, found := g.playerKnowledge.entries[key]
	if !found {
		return KnowledgeEntry{}, false
	}

	return cloneKnowledgeEntry(entry), true
}

// ShadowlordKnowledge returns a detached learned Shadowlord fact when present.
func (g *GameState) ShadowlordKnowledge(
	lord references.Shadowlord,
	kind ShadowlordKnowledgeKind,
) (KnowledgeEntry, bool) {
	g.requirePlayerKnowledge()
	key, supported := ShadowlordKnowledgeKey(lord, kind)
	if !supported {
		return KnowledgeEntry{}, false
	}

	return g.knowledge(key)
}

// ShrineMantraKnowledge returns detached learned mantra knowledge when present.
func (g *GameState) ShrineMantraKnowledge(shrineIndex int) (KnowledgeEntry, bool) {
	g.requirePlayerKnowledge()
	key, supported := ShrineMantraKnowledgeKey(shrineIndex)
	if !supported {
		return KnowledgeEntry{}, false
	}

	return g.knowledge(key)
}

// UnreadKnowledgeCount returns the number of unread entries in one category.
func (g *GameState) UnreadKnowledgeCount(category KnowledgeCategory) int {
	g.requirePlayerKnowledge()
	count := 0
	for key, entry := range g.playerKnowledge.entries {
		if key.Category == category && entry.Unread {
			count++
		}
	}

	return count
}

func (g *GameState) requirePlayerKnowledge() {
	if g.playerKnowledge == nil {
		panic("GameState player knowledge is not initialized")
	}
}

func (g *GameState) discoverLoreKnowledge(key KnowledgeKey, source KnowledgeSource) bool {
	if _, found := g.playerKnowledge.entries[key]; found {
		return false
	}
	g.playerKnowledge.entries[key] = KnowledgeEntry{
		Key: key, Source: source, DiscoveredOn: currentKnowledgeDate(g.DateTime), Unread: false,
	}
	g.recordJournalKnowledge(g.playerKnowledge.entries[key])

	return true
}

func (g *GameState) knowledge(key KnowledgeKey) (KnowledgeEntry, bool) {
	entry, found := g.playerKnowledge.entries[key]
	if !found {
		return KnowledgeEntry{}, false
	}

	return cloneKnowledgeEntry(entry), true
}

func validateKnowledgeSource(source KnowledgeSource, allowMigration bool) {
	valid := source.Kind == KnowledgeSourceDialogue && strings.TrimSpace(source.DisplayName) != "" ||
		source.Kind == KnowledgeSourceExperimentation && source.DisplayName == experimentationKnowledgeDisplayName ||
		allowMigration && source.Kind == KnowledgeSourceMigration && source.DisplayName == migratedKnowledgeDisplayName
	if !valid {
		panic("invalid player knowledge source")
	}
}

func validateLoreKnowledgeSource(source KnowledgeSource) {
	if strings.TrimSpace(source.DisplayName) == "" ||
		(source.Kind != KnowledgeSourceDialogue && source.Kind != KnowledgeSourceObservation && source.Kind != KnowledgeSourceRevelation) {
		panic("lore knowledge source must be dialogue, observation, or revelation")
	}
}

func currentKnowledgeDate(date datetime.UltimaDate) *datetime.BritannianDate {
	value := date.DateOnly()
	if !value.IsValid() {
		panic("player knowledge requires a valid Britannian date")
	}

	return &value
}

func discoveredSpellFormula(spell references.Spell) *SpellFormulaKnowledge {
	if spell != references.AnYlem {
		return confirmedSpellFormula(spell)
	}

	return &SpellFormulaKnowledge{
		State: SpellFormulaUncertain,
		CandidateMasks: []uint16{
			(1 << references.SulfurAsh) | (1 << references.MandrakeRoot),
			(1 << references.Garlic) | (1 << references.BloodMoss),
		},
	}
}

func confirmedSpellFormula(spell references.Spell) *SpellFormulaKnowledge {
	return &SpellFormulaKnowledge{
		State: SpellFormulaConfirmed, ConfirmedMask: references.GetSpellReagents(spell),
		CandidateMasks: make([]uint16, 0),
	}
}

func cloneKnowledgeEntry(entry KnowledgeEntry) KnowledgeEntry {
	cloned := entry
	if entry.DiscoveredOn != nil {
		date := *entry.DiscoveredOn
		cloned.DiscoveredOn = &date
	}
	if entry.SpellFormula != nil {
		formula := *entry.SpellFormula
		formula.CandidateMasks = append([]uint16(nil), entry.SpellFormula.CandidateMasks...)
		cloned.SpellFormula = &formula
	}

	return cloned
}
