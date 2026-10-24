package audio

import (
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	berad "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

// AmbientScanRadius is a game world-query bound derived from its authored routing.
func (manager *GameAudioManager) AmbientScanRadius() int {
	var radius float64
	for _, profile := range manager.library.document.Routing.Ambient {
		radius = max(radius, profile.RadiusTiles)
	}
	for _, tile := range manager.library.document.Routing.Tiles {
		if tile.RadiusTiles != nil {
			radius = max(radius, *tile.RadiusTiles)
		}
	}

	return int(math.Ceil(radius))
}

// EmitterAmbientSource places a code-owned game emitter (bard, moongate) at a
// position; the library chooses its cue, which supplies everything else.
func (manager *GameAudioManager) EmitterAmbientSource(
	emitter string, position references.Position, volume float64,
) AmbientSoundSource {
	cue, exists := manager.library.document.Routing.Emitters[emitter]
	if !exists {
		panic("unknown game ambient emitter: " + emitter)
	}

	return AmbientSoundSource{ID: "", Emitter: emitter, Position: position, SoundType: AmbientSoundType(cue),
		Background: false, VolumeModifier: volume}
}

// DungeonAmbientSource retains a non-spatial cue-owned dungeon-visit background.
func (manager *GameAudioManager) DungeonAmbientSource(
	location references.Location, listener references.Position,
) (AmbientSoundSource, bool) {
	return dungeonAmbientSource(manager.library, manager.planner.definitions, location, listener)
}

// NavalAmbientSource uses game vessel eligibility and JSON assignments.
func (manager *GameAudioManager) NavalAmbientSource(
	vessel string, listener references.Position,
) (AmbientSoundSource, bool) {
	return navalAmbientSource(manager.library.document.Routing, manager.planner.definitions, vessel, listener)
}

// LighthouseAmbientSource keeps the shared coastal/interior voice identity in Redux.
func (manager *GameAudioManager) LighthouseAmbientSource(
	location references.Location, listener references.Position,
) (AmbientSoundSource, bool) {
	return lighthouseAmbientSource(manager.planner.definitions, location, listener)
}

// UpdateAmbientSoundsWithDynamicSources projects game intent, preserving grouping
// and game emitter roles outside the generic controller. Omitted calls retain state.
func (manager *GameAudioManager) UpdateAmbientSoundsWithDynamicSources(listener references.Position,
	tiles []TileWithPosition, dynamic []AmbientSoundSource,
) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.closed || !manager.enabled || manager.ambientSuppressed {
		return
	}
	plans, err := manager.planner.Sources(listener, tiles, dynamic)
	if err != nil {
		manager.report(err)

		return
	}
	sources := make([]berad.AmbientSource, 0, len(plans))
	desired := make(map[berad.SourceID]AmbientSourcePlan, len(plans))
	for _, plan := range plans {
		sourceID := berad.SourceID(plan.ID)
		sources = append(sources, berad.AmbientSource{ID: sourceID, Cue: berad.CueID(plan.CueID), Gain: plan.Gain,
			Loop: true, FadeIn: plan.FadeIn, FadeOut: plan.FadeOut})
		desired[sourceID] = plan
	}
	manager.report(manager.ambience.Reconcile(sources))
	for _, source := range manager.ambience.Snapshot().Sources {
		if source.Desired && source.Voice.Cue == source.Cue {
			manager.roles[source.ID] = desired[source.ID].Emitter
			manager.sourceMix[source.ID] = desired[source.ID]
		}
	}
}
