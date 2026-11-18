package game_state

import (
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/journal"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func (g *GameState) projectChronicleEvent(event QuestJournalEvent) QuestJournalChronicleEntry {
	entry := QuestJournalChronicleEntry{Sequence: event.Sequence, Source: event.SourceName}
	switch event.Kind {
	case QuestJournalEventItemAcquired:
		entry.Title = journal.ItemMilestoneTitle(string(event.Subject)) + " acquired"
		entry.Summary = "Recovered " + journal.ItemMilestoneTitle(string(event.Subject)) + "."
	case QuestJournalEventShadowlordDefeated:
		lord, supported := shadowlordForQuestJournalSubject(event.Subject)
		if !supported {
			panic("unsupported Shadowlord defeat subject")
		}
		entry.DisplayName = g.questJournalShadowlordDisplayName(lord)
		entry.Title = entry.DisplayName + " defeated"
		entry.Summary = entry.DisplayName + " has been permanently defeated."
	case QuestJournalEventCodexAnswerLearned:
		for index := range TotalShrines {
			key, _ := ShrineMantraKnowledgeKey(index)
			if string(event.Subject) == strings.TrimSuffix(key.ID, ".mantra") {
				entry.Title = allShrines[index].VirtueName + ": Codex answer"
				entry.Summary = allShrines[index].AnswerText()
			}
		}
	case QuestJournalEventKnowledgeLearned:
		entry.Title, entry.Summary = g.projectKnowledgeNote(string(event.Subject))
	default:
		panic("unsupported authoritative quest journal event")
	}
	if entry.Title == "" {
		panic("unsupported authoritative quest journal subject")
	}

	return entry
}

func (g *GameState) projectKnowledgeNote(subject string) (string, string) {
	if subject == "journal.stones.melody" {
		return "Stones: harpsichord melody", "Stand immediately north of the harpsichord. Press the top-row number keys or numpad keys one at a time: 6 7 8 9 8 7 8 7 6 7 6 5 3."
	}
	if subject == "journal.resistance.password" {
		return "Resistance password", "The Resistance password is DAWN."
	}
	if subject == "journal.oppression.password" {
		return "Oppression password", "The Oppression password is IMPERA."
	}
	for _, candidate := range canonicalDungeonWords {
		if subject != "dungeon."+strings.ToLower(candidate.dungeon.String())+".word_of_power" {
			continue
		}
		if candidate.dungeon == references.Destard {
			// The player identifies Destard in question_01/drat before Goeth's reply.
			// Preserve his reversed runes rather than solving the word itself.
			return "Word of Power: Destard", "Goeth spoke the runes AIPONI for Destard."
		}
		if candidate.dungeon == references.Shame {
			// Sindar names SHAME in a separate response, not in this word discovery.
			return "Word of Power: INFAMA", "Sindar spoke the word INFAMA."
		}
		if candidate.dungeon == references.Doom {
			if g.CodexRevelationComplete {
				// The clues speak the word; a short title fits the Chronicle list.
				return "Codex revelation", codexRevelationText
			}

			return "Word of Power: VERAMOCOR", codexWordSummary
		}

		return "Word of Power: " + candidate.dungeon.String(), "The Word of Power for " + candidate.dungeon.String() + " is " + candidate.word + "."
	}
	for lord := references.Faulinei; lord < references.ShadowlordCount; lord++ {
		name, _ := ShadowlordKnowledgeKey(lord, ShadowlordKnowledgeName)
		association, _ := ShadowlordKnowledgeKey(lord, ShadowlordKnowledgeAssociation)
		if subject == "shadowlord."+name.ID {
			return lord.String(), "The name " + lord.String() + " has been learned."
		}
		if subject == "shadowlord."+association.ID {
			name := g.questJournalShadowlordDisplayName(lord)

			return name + ": " + lord.GetAntiVirtue(), name + " embodies " + lord.GetAntiVirtue() + "."
		}
		if subject == "journal.domain."+strings.ToLower(lord.GetAntiVirtue()) {
			title := "Shadowlord of " + lord.GetAntiVirtue()

			return title, "A " + title + " exists."
		}
		flame := references.FlameOfPrinciple(lord)
		if subject == "journal.flame."+strings.ToLower(flame.String()) {
			return flame.GetPrinciple(), "The " + flame.GetPrinciple() + " is at " + questJournalKeepName(references.FlameLocations[flame].Location) + "."
		}
	}
	for index := range TotalShrines {
		key, _ := ShrineMantraKnowledgeKey(index)
		if subject == "shrine."+key.ID {
			return allShrines[index].VirtueName + ": mantra", "The mantra of " + allShrines[index].VirtueName + " is " + strings.ToUpper(allShrines[index].Mantra) + "."
		}
	}
	if subject == "journal.banishment.method" {
		return "Sutek's instructions", "Cast the shards into the Eternal Flames of Truth, Love, and Courage while each respective Shadowlord stands near the flame of the Principle it opposes."
	}
	panic("unsupported journal knowledge subject")
}

func questJournalKeepName(location references.Location) string {
	//nolint:exhaustive // Journal Flame discoveries admit only these three keeps.
	switch location {
	case references.Lycaeum:
		return "Lycaeum"
	case references.EmpathAbbey:
		return "Empath Abbey"
	case references.SerpentsHold:
		return "Serpent's Hold"
	default:
		panic("journal Flame discovery requires a keep")
	}
}
