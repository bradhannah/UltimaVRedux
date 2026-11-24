package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// GrantSextant commits a live conversation grant. Setup, debug inventory editing
// and hydration use inventory directly and cannot manufacture this event.
func (g *GameState) GrantSextant() {
	items := &g.PartyState.Inventory.QuestItems
	owned := items.Get(references.Sextant) > 0
	items.IncrementByOne(references.Sextant)
	if owned || items.Get(references.Sextant) == 0 || g.MapState.PlayerLocation.Location != references.Greyhaven {
		return
	}
	exterior, valid := g.sextantSettlementExterior(g.CurrentSextantContext())
	if !valid {
		return
	}
	observation := g.currentEarnedAtlasLandmark()
	if observation == nil || observation.Name == "" || observation.NameSource != journal.AtlasSourceEnter ||
		observation.Kind != "lighthouse" || observation.Source != journal.AtlasSourceLook && observation.Source != journal.AtlasSourceEnter ||
		observation.Survey.Measured || observation.Site != moonstoneJournalSite(exterior) {
		return
	}
	before := *observation
	sample := g.sampleDungeonObservation()
	// REDUX DEVIATION: VIEW-0006: original item-use rules restricts sextant Use to outdoors at night. Redux records
	// this one local exterior when the sextant is actually received in Greyhaven,
	// including strict daytime/interior receipt; it never enables an interior fix.
	// See ref:VIEW-0006, ref:ASTRO-0006.
	observation.Source, observation.ObservedAt = journal.AtlasSourceSextantReceipt, sample.at
	observation.Survey = journal.SurveyedLocation{Measured: true, Flexible: sample.sextant.Flexible, Location: observation.Site, RecordedAt: sample.at}
	observation.SurveySource = journal.AtlasSourceSextantReceipt
	g.publishAtlasChange(before, *observation)
}
