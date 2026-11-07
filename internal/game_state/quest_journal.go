package game_state

import (
	"cmp"
	"slices"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// QuestJournalEventKind identifies one semantic Chronicle event family.
type QuestJournalEventKind string

const (
	// QuestJournalEventShadowlordDefeated records one permanent Shadowlord defeat.
	QuestJournalEventShadowlordDefeated QuestJournalEventKind = "shadowlord-defeated"
	// QuestJournalEventKnowledgeLearned records a first finite lore discovery.
	QuestJournalEventKnowledgeLearned QuestJournalEventKind = "knowledge-learned"
	// QuestJournalEventCodexAnswerLearned records the actual Codex answer commit.
	QuestJournalEventCodexAnswerLearned QuestJournalEventKind = "codex-answer-learned"
	// QuestJournalEventItemAcquired records a significant successful pickup.
	QuestJournalEventItemAcquired QuestJournalEventKind = "item-acquired"
)

// QuestJournalSubject is a stable semantic event subject, not display prose.
type QuestJournalSubject string

const (
	// QuestJournalSubjectFaulinei identifies Faulinei.
	QuestJournalSubjectFaulinei QuestJournalSubject = "faulinei"
	// QuestJournalSubjectAstaroth identifies Astaroth.
	QuestJournalSubjectAstaroth QuestJournalSubject = "astaroth"
	// QuestJournalSubjectNosfentor identifies Nosfentor.
	QuestJournalSubjectNosfentor QuestJournalSubject = "nosfentor"
)

// QuestJournalEventSource identifies how a Chronicle fact entered the journal.
type QuestJournalEventSource string

const (
	// QuestJournalSourceGameplay identifies an event committed by live gameplay.
	QuestJournalSourceGameplay QuestJournalEventSource = "gameplay"
	// QuestJournalSourceMigration identifies an event inferred from older authority.
	QuestJournalSourceMigration QuestJournalEventSource = "migration"
)

// QuestJournalEvent is one detached semantic Chronicle event.
type QuestJournalEvent struct {
	Sequence     uint64
	Kind         QuestJournalEventKind
	Subject      QuestJournalSubject
	OccurredOn   *datetime.BritannianDate
	Source       QuestJournalEventSource
	SourceName   string
	Acknowledged bool
}

type questJournal struct {
	events []QuestJournalEvent
}

func newQuestJournal() *questJournal {
	return &questJournal{events: make([]QuestJournalEvent, 0)}
}

// RecordShadowlordDefeatEvent appends a date-stamped event exactly once. Repeated
// calls leave the original sequence, date, source, and acknowledgment unchanged.
func (g *GameState) RecordShadowlordDefeatEvent(lord references.Shadowlord) bool {
	g.requireQuestJournal()
	subject, supported := questJournalSubjectForShadowlord(lord)
	if !supported {
		return false
	}
	date := g.DateTime.DateOnly()
	if !date.IsValid() {
		panic("quest journal requires a valid Britannian date")
	}

	return g.appendLiveJournalEvent(QuestJournalEvent{
		Kind:       QuestJournalEventShadowlordDefeated,
		Subject:    subject,
		OccurredOn: &date,
		Source:     QuestJournalSourceGameplay,
	})
}

func (g *GameState) appendLiveJournalEvent(event QuestJournalEvent) bool {
	g.requireQuestJournal()
	if !g.questJournal.append(event) {
		return false
	}
	g.SystemCallbacks.UI.JournalChanged(journal.ChronicleTab, len(g.questJournal.events))

	return true
}

func (g *GameState) recordJournalKnowledge(entry KnowledgeEntry) {
	g.appendLiveJournalEvent(QuestJournalEvent{
		Kind:       QuestJournalEventKnowledgeLearned,
		Subject:    QuestJournalSubject(string(entry.Key.Category) + "." + entry.Key.ID),
		OccurredOn: entry.DiscoveredOn, Source: QuestJournalEventSource(entry.Source.Kind),
		SourceName: entry.Source.DisplayName,
	})
	for index := range TotalShrines {
		key, _ := ShrineMantraKnowledgeKey(index)
		if entry.Key == key {
			g.SystemCallbacks.UI.JournalChanged(journal.VirtuesTab, index)
		}
	}
	for lord := references.Faulinei; lord < references.ShadowlordCount; lord++ {
		name, _ := ShadowlordKnowledgeKey(lord, ShadowlordKnowledgeName)
		association, _ := ShadowlordKnowledgeKey(lord, ShadowlordKnowledgeAssociation)
		_, associated := g.ShadowlordKnowledge(lord, ShadowlordKnowledgeAssociation)
		flameID := "flame." + strings.ToLower(references.FlameOfPrinciple(lord).String())
		if entry.Key == name || entry.Key == association ||
			(entry.Key.Category == KnowledgeCategoryJournal && entry.Key.ID == flameID && associated) {
			g.SystemCallbacks.UI.JournalChanged(journal.ShadowlordsTab, int(lord))
		}
	}
}

// QuestJournalEvents returns insertion-ordered detached Chronicle events.
func (g *GameState) QuestJournalEvents() []QuestJournalEvent {
	g.requireQuestJournal()
	events := make([]QuestJournalEvent, 0, len(g.questJournal.events))
	for _, event := range g.questJournal.events {
		events = append(events, cloneQuestJournalEvent(event))
	}

	return events
}

func (journal *questJournal) append(event QuestJournalEvent) bool {
	for _, existing := range journal.events {
		if existing.Kind == event.Kind && existing.Subject == event.Subject {
			return false
		}
	}
	event.Sequence = uint64(len(journal.events) + 1)
	journal.events = append(journal.events, cloneQuestJournalEvent(event))

	return true
}

func (g *GameState) requireQuestJournal() {
	if g.questJournal == nil {
		panic("GameState quest journal is not initialized")
	}
}

func questJournalSubjectForShadowlord(lord references.Shadowlord) (QuestJournalSubject, bool) {
	switch lord {
	case references.Faulinei:
		return QuestJournalSubjectFaulinei, true
	case references.Astaroth:
		return QuestJournalSubjectAstaroth, true
	case references.Nosfentor:
		return QuestJournalSubjectNosfentor, true
	case references.NoShadowlord:
		return "", false
	default:
		return "", false
	}
}

func cloneQuestJournalEvent(event QuestJournalEvent) QuestJournalEvent {
	cloned := event
	if event.OccurredOn != nil {
		date := *event.OccurredOn
		cloned.OccurredOn = &date
	}

	return cloned
}

// QuestJournalDossier is the detached read-only projection for all journal tabs.
type QuestJournalDossier struct {
	Chronicle   []QuestJournalChronicleEntry
	Virtues     []QuestJournalVirtue
	Shadowlords []QuestJournalShadowlord
	Dungeons    []journal.DungeonEntry
	Moonstones  [8]journal.MoonstoneEntry
}

// QuestJournalChronicleEntry is a display-safe Chronicle projection that does
// not expose the stable subject used by authoritative persistence.
type QuestJournalChronicleEntry struct {
	Sequence    uint64
	Title       string
	Summary     string
	Source      string
	DisplayName string
	OccurredOn  *datetime.BritannianDate
}

// QuestJournalVirtue projects one shrine's authoritative quest flags.
type QuestJournalVirtue struct {
	Index              int
	Name               string
	QuestState         ShrineProgressState
	Ruined             bool
	MantraLearned      bool
	Mantra             string
	CodexAnswerLearned bool
	CodexAnswer        string
}

// QuestJournalShadowlordState is the authoritative living/defeated projection.
type QuestJournalShadowlordState string

const (
	// QuestJournalShadowlordLiving indicates an identity remains alive.
	QuestJournalShadowlordLiving QuestJournalShadowlordState = "living"
	// QuestJournalShadowlordDefeated indicates an identity is permanently dead.
	QuestJournalShadowlordDefeated QuestJournalShadowlordState = "defeated"
)

// QuestJournalShadowlord is a display-safe projection of one fixed row.
type QuestJournalShadowlord struct {
	DisplayName string
	Association string
	Opposes     string
	Flame       string
	Location    string
	State       QuestJournalShadowlordState
}

// CurrentQuestJournalDossier returns Chronicle metadata and current progression
// authority without allowing either representation to mutate the other.
func (g *GameState) CurrentQuestJournalDossier() QuestJournalDossier {
	g.requirePlayerKnowledge()
	virtues := make([]QuestJournalVirtue, 0, TotalShrines)
	for index := range TotalShrines {
		progress := g.ShrineProgress(index)
		_, mantraLearned := g.ShrineMantraKnowledge(index)
		virtue := QuestJournalVirtue{
			Index:              index,
			Name:               allShrines[index].VirtueName,
			QuestState:         progress.QuestState,
			Ruined:             progress.Ruined,
			MantraLearned:      mantraLearned,
			CodexAnswerLearned: progress.Completed,
		}
		if mantraLearned {
			virtue.Mantra = strings.ToUpper(allShrines[index].Mantra)
		}
		if progress.Completed {
			virtue.CodexAnswer = allShrines[index].AnswerText()
		}
		virtues = append(virtues, virtue)
	}

	if g.ShadowlordState == nil {
		panic("GameState Shadowlord state is not initialized")
	}
	shadowlords := make([]QuestJournalShadowlord, 0, references.ShadowlordCount)
	for lord := references.Faulinei; lord < references.ShadowlordCount; lord++ {
		state := QuestJournalShadowlordDefeated
		if g.ShadowlordState.IsAlive(lord) {
			state = QuestJournalShadowlordLiving
		}
		shadowlords = append(shadowlords, g.projectQuestJournalShadowlord(lord, state))
	}

	return QuestJournalDossier{
		Dungeons:   g.journalDungeons(),
		Moonstones: g.journalMoonstones(),
		Chronicle:  g.projectQuestJournalChronicle(), Virtues: virtues, Shadowlords: shadowlords,
	}
}

func (g *GameState) projectQuestJournalChronicle() []QuestJournalChronicleEntry {
	events := g.QuestJournalEvents()
	chronicle := make([]QuestJournalChronicleEntry, 0, len(events))
	for _, event := range events {
		entry := g.projectChronicleEvent(event)
		if event.OccurredOn != nil {
			date := *event.OccurredOn
			entry.OccurredOn = &date
		}
		chronicle = append(chronicle, entry)
	}
	// Spell discoveries already persist their provenance in player knowledge.
	// Project them rather than duplicating that authority in the event wire format.
	// Reserve distinct presentation keys above the finite Chronicle event catalog;
	// these are not persisted event sequences or a second discovery record.
	const spellNoteKeyBase = 1 << 16
	for _, spell := range []references.Spell{references.RelXenBet, references.InXenMani} {
		knowledge, found := g.SpellKnowledge(spell)
		if !found {
			continue
		}
		name := references.GetSpellInfo(spell).Name
		entry := QuestJournalChronicleEntry{
			Sequence: spellNoteKeyBase + uint64(spell),
			Title:    name + " discovered", Summary: "Learned of " + name + ". See Magic for the known formula.",
			Source: knowledge.Source.DisplayName,
		}
		if knowledge.DiscoveredOn != nil {
			date := *knowledge.DiscoveredOn
			entry.OccurredOn = &date
		}
		chronicle = append(chronicle, entry)
	}
	slices.SortStableFunc(chronicle, compareChronicleDates)

	return chronicle
}

func compareChronicleDates(left, right QuestJournalChronicleEntry) int {
	if left.OccurredOn == nil && right.OccurredOn == nil {
		return 0
	}
	if left.OccurredOn == nil {
		return -1
	}
	if right.OccurredOn == nil {
		return 1
	}
	if year := cmp.Compare(left.OccurredOn.Year, right.OccurredOn.Year); year != 0 {
		return year
	}
	if month := cmp.Compare(left.OccurredOn.Month, right.OccurredOn.Month); month != 0 {
		return month
	}

	return cmp.Compare(left.OccurredOn.Day, right.OccurredOn.Day)
}

func (g *GameState) projectQuestJournalShadowlord(
	lord references.Shadowlord,
	state QuestJournalShadowlordState,
) QuestJournalShadowlord {
	projected := QuestJournalShadowlord{
		DisplayName: g.questJournalShadowlordDisplayName(lord),
		State:       state,
	}
	if _, learned := g.ShadowlordKnowledge(lord, ShadowlordKnowledgeAssociation); learned {
		projected.Association = lord.GetAntiVirtue()
		flame := references.FlameOfPrinciple(lord)
		projected.Opposes = flame.String()
		if _, known := g.JournalKnowledge("flame." + strings.ToLower(flame.String())); known {
			projected.Flame = flame.GetPrinciple()
			projected.Location = questJournalKeepName(references.FlameLocations[flame].Location)
		}
	}

	return projected
}

func (g *GameState) questJournalShadowlordDisplayName(lord references.Shadowlord) string {
	if _, learned := g.ShadowlordKnowledge(lord, ShadowlordKnowledgeName); learned {
		return lord.String()
	}
	switch lord {
	case references.Faulinei:
		return "Shadowlord I"
	case references.Astaroth:
		return "Shadowlord II"
	case references.Nosfentor:
		return "Shadowlord III"
	case references.NoShadowlord:
		panic("cannot project absent Shadowlord")
	default:
		panic("cannot project unknown Shadowlord")
	}
}

func shadowlordForQuestJournalSubject(subject QuestJournalSubject) (references.Shadowlord, bool) {
	switch subject {
	case QuestJournalSubjectFaulinei:
		return references.Faulinei, true
	case QuestJournalSubjectAstaroth:
		return references.Astaroth, true
	case QuestJournalSubjectNosfentor:
		return references.Nosfentor, true
	default:
		return references.NoShadowlord, false
	}
}
