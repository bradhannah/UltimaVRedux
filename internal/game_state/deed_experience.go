package game_state

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// REDUX DEVIATION: PROGRESS-0004: the original awards experience only for kills,
// so a party that follows the quest reaches Doom several levels short. Redux
// pays every member of the active party a fixed amount for each quest deed, once.
// The Deed Experience setting chooses None, Lean (the default) or Fuller, which
// doubles Lean. A deed done while the setting pays nothing is paid later, to the
// party as it then stands, when the setting is turned on.

// Lean experience for each kind of deed. ref:PROGRESS-0004
const (
	deedCodexAnswerExperience   = 20
	deedShrineReturnExperience  = 20
	deedRevelationExperience    = 200
	deedDungeonOpenedExperience = 25
	deedQuestItemExperience     = 150
	deedShadowlordExperience    = 300
	deedFullerMultiplier        = 2
)

// deed is one once-only payment: its saved key, Lean amount and completion test.
type deed struct {
	key        string
	experience int
	done       func(*GameState) bool
}

// ExperienceLevelReady names a living member whose experience has just earned
// a level that Lord British's spirit has not yet granted.
type ExperienceLevelReady struct {
	Name  string
	Level int
}

// DeedExperienceAward reports one payment of deed experience to the party.
type DeedExperienceAward struct {
	// Each is the experience paid to every recipient.
	Each int
	// Recipients counts the active party members paid, the dead included.
	Recipients int
	// Deeds counts the payments settled together.
	Deeds int
	// BackPay marks deeds finished before the setting could pay for them.
	BackPay bool
	Ready   []ExperienceLevelReady
}

// Paid reports whether anything was awarded.
func (award DeedExperienceAward) Paid() bool {
	return award.Each > 0
}

// deedCatalog lists every payment in a stable order. Keys are saved; never
// rename one.
func deedCatalog() []deed {
	deeds := make([]deed, 0, TotalShrines*2+1+len(deedDungeons())+len(deedQuestItems())+references.ShadowlordCount)
	for index := range TotalShrines {
		virtue := strings.ToLower(GetShrineData(index).VirtueName)
		deeds = append(deeds,
			deed{"codex." + virtue, deedCodexAnswerExperience, func(g *GameState) bool {
				return g.shrineQuestsCompleted[index]
			}},
			deed{"shrine." + virtue, deedShrineReturnExperience, func(g *GameState) bool {
				return g.shrineQuestsCompleted[index] && !g.shrineQuestsActive[index]
			}})
	}
	deeds = append(deeds, deed{"codex.revelation", deedRevelationExperience, func(g *GameState) bool {
		return g.AreAllQuestsComplete() && g.knowsDoomWordOfPower()
	}})
	for _, dungeon := range deedDungeons() {
		deeds = append(deeds, deed{"dungeon." + strings.ToLower(dungeon.String()), deedDungeonOpenedExperience,
			func(g *GameState) bool { return g.dungeonEntranceWasOpened(dungeon) }})
	}
	for _, item := range deedQuestItems() {
		deeds = append(deeds, deed{"item." + item.key, deedQuestItemExperience, func(g *GameState) bool {
			return g.questItemWasAcquired(item)
		}})
	}
	for lord := references.Faulinei; lord < references.ShadowlordCount; lord++ {
		deeds = append(deeds, deed{"shadowlord." + strings.ToLower(lord.String()), deedShadowlordExperience,
			func(g *GameState) bool { return g.ShadowlordState != nil && !g.ShadowlordState.IsAlive(lord) }})
	}

	return deeds
}

func deedDungeons() []references.Location {
	return []references.Location{
		references.Deceit, references.Despise, references.Destard, references.Wrong,
		references.Covetous, references.Shame, references.Hythloth, references.Doom,
	}
}

// deedQuestItem pairs a quest item with its Chronicle key. A shard is consumed
// by its ritual, so a destroyed Shadowlord also proves the shard was recovered.
type deedQuestItem struct {
	item      references.QuestItem
	key       string
	destroyed references.Shadowlord
	isShard   bool
}

func deedQuestItems() []deedQuestItem {
	return []deedQuestItem{
		{references.Crown, "crown", 0, false},
		{references.Sceptre, "sceptre", 0, false},
		{references.Amulet, "amulet", 0, false},
		{references.WoodenBox, "sandalwood-box", 0, false},
		{references.ShardOfFalsehood, "shard.falsehood", references.Faulinei, true},
		{references.ShardOfHatred, "shard.hatred", references.Astaroth, true},
		{references.ShardOfCowardice, "shard.cowardice", references.Nosfentor, true},
	}
}

func (g *GameState) knowsDoomWordOfPower() bool {
	if g.playerKnowledge == nil {
		return false
	}
	_, known := g.WordOfPowerKnowledge("doom")

	return known
}

// dungeonEntranceWasOpened also accepts the Journal's record of the word being
// used, because the same word seals an entrance again.
func (g *GameState) dungeonEntranceWasOpened(dungeon references.Location) bool {
	index, _ := dungeonIndex(dungeon)

	return g.DungeonStates[index] || g.dungeonJournal[index].WordUsed
}

func (g *GameState) questItemWasAcquired(item deedQuestItem) bool {
	if g.PartyState.Inventory.QuestItems.Get(item.item) > 0 {
		return true
	}
	if item.isShard && g.ShadowlordState != nil && !g.ShadowlordState.IsAlive(item.destroyed) {
		return true
	}
	if g.questJournal == nil {
		return false
	}

	return slices.ContainsFunc(g.questJournal.events, func(event QuestJournalEvent) bool {
		return event.Kind == QuestJournalEventItemAcquired && string(event.Subject) == item.key
	})
}

func (g *GameState) deedExperienceMultiplier() int {
	switch g.GameConfig.GetDeedExperience() {
	case config.DeedExperienceLean:
		return 1
	case config.DeedExperienceFuller:
		return deedFullerMultiplier
	default:
		return 0
	}
}

// SettlePastDeedExperience pays deeds finished before the setting could pay for
// them: on an older save, or after the setting is turned on.
func (g *GameState) SettlePastDeedExperience() DeedExperienceAward {
	return g.settleDeedExperience(true)
}

// SettleDeedExperience pays any deed the party has just finished.
func (g *GameState) SettleDeedExperience() DeedExperienceAward {
	return g.settleDeedExperience(false)
}

// settleDeedExperience pays every finished, unpaid deed once. The caller
// presents the award beside the deed's own message.
func (g *GameState) settleDeedExperience(backPay bool) DeedExperienceAward {
	multiplier := g.deedExperienceMultiplier()
	if multiplier == 0 {
		return DeedExperienceAward{}
	}
	total, settled := 0, 0
	for _, candidate := range deedCatalog() {
		if g.deedExperiencePaid[candidate.key] || !candidate.done(g) {
			continue
		}
		if g.deedExperiencePaid == nil {
			g.deedExperiencePaid = make(map[string]bool)
		}
		g.deedExperiencePaid[candidate.key] = true
		total += candidate.experience * multiplier
		settled++
	}
	if settled == 0 {
		return DeedExperienceAward{}
	}
	award := g.payPartyExperience(total)
	award.Deeds, award.BackPay = settled, backPay

	return award
}

// payPartyExperience gives every active member the same amount. The dead are
// paid too, so nobody has to be raised before a deed.
func (g *GameState) payPartyExperience(amount int) DeedExperienceAward {
	award := DeedExperienceAward{Each: amount}
	for index := range party_state.MaxActiveParty {
		member := g.PartyState.GetCharacter(index)
		if member == nil || member.PartyMap != party_state.PartyMapActive {
			continue
		}
		before := member.Exp
		member.Exp = uint16(min(int(before)+amount, MaxExperienceValue))
		award.Recipients++
		if ready, level := experienceLevelReady(member, before); ready {
			award.Ready = append(award.Ready, ExperienceLevelReady{Name: member.GetNameAsString(), Level: level})
		}
	}

	return award
}

// experienceLevelReady reports a living member whose experience crossed a level
// line that their granted level has not reached. ref:PROGRESS-0003
func experienceLevelReady(member *party_state.PlayerCharacter, before uint16) (bool, int) {
	earned := CalculateLevelFromExperience(member.Exp)
	if member.IsDead() || earned <= CalculateLevelFromExperience(before) || earned <= member.Level {
		return false, 0
	}

	return true, int(earned)
}

// deedExperiencePaidKeys returns the saved, sorted record of paid deeds. No
// payments is the zero value, so an untouched record survives a round trip.
func (g *GameState) deedExperiencePaidKeys() []string {
	var keys []string
	for key, paid := range g.deedExperiencePaid {
		if paid {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)

	return keys
}

// ErrUnknownDeed rejects a saved payment for a deed this build does not know.
var ErrUnknownDeed = errors.New("unknown quest deed")

func (g *GameState) restoreDeedExperiencePaid(keys []string) error {
	catalog := deedCatalog()
	g.deedExperiencePaid = nil
	for _, key := range keys {
		if !slices.ContainsFunc(catalog, func(known deed) bool { return known.key == key }) {
			return fmt.Errorf("%w: %q", ErrUnknownDeed, key)
		}
		if g.deedExperiencePaid == nil {
			g.deedExperiencePaid = make(map[string]bool, len(keys))
		}
		g.deedExperiencePaid[key] = true
	}

	return nil
}
