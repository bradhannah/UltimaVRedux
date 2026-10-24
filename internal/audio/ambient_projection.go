package audio

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	berad "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

// AmbientSourcePlan is Redux's neutral presentation intent for one looping voice.
// Gain contains only source overrides/intensity/distance, never cue gain or
// recording calibration. Emitter is game policy metadata (for example bard
// precedence), not an engine role.
type AmbientSourcePlan struct {
	ID           string
	CueID        string
	Gain         float64
	FadeIn       time.Duration
	FadeOut      time.Duration
	Emitter      string
	SourceGain   float64
	DistanceGain float64
	Intensity    float64
}

// AmbientPlanner projects Redux JSON/world sources without constructing players,
// reading game data, or taking ownership of the playback device. Its library and
// definition tables are immutable. Callers supply already-projected coordinates.
type AmbientPlanner struct {
	library     *Library
	tiles       map[indexes.SpriteIndex]AmbientSoundDefinition
	definitions map[AmbientSoundType]AmbientSoundDefinition
}

// NewAmbientPlanner derives the game's tile groups from its supplied library.
func NewAmbientPlanner(library *Library) (*AmbientPlanner, error) {
	if library == nil {
		return nil, fmt.Errorf("%w: ambient planner requires a library", ErrAudioLibrary)
	}
	definitions := ambientSoundDefinitions(library)
	tiles := make(map[indexes.SpriteIndex]AmbientSoundDefinition)
	routing := library.document.Routing
	for sprite, cue := range ambientSpriteMappingsFor(routing) {
		definition := definitions[cue]
		profile, gain := routing.TileAmbientSettings(tileAudioKey(sprite))
		definition.MaxDistance, definition.BaseVolume = profile.RadiusTiles, definition.BaseVolume*gain
		tiles[sprite] = definition
	}

	return &AmbientPlanner{library: library, tiles: tiles, definitions: definitions}, nil
}

// Sources preserves Redux's strongest-static-cue grouping, explicit dynamic IDs,
// cue-owned background beds and lighthouse continuity. Output IDs are sorted;
// projection does not sample a clock, start playback or mutate source metadata.
func (planner *AmbientPlanner) Sources(listener references.Position,
	tiles []TileWithPosition, dynamic []AmbientSoundSource,
) ([]AmbientSourcePlan, error) {
	candidates := collectAmbientSources(planner.tiles, planner.definitions, listener, tiles, dynamic)
	plans := make([]AmbientSourcePlan, 0, len(candidates))
	for sourceID, candidate := range candidates {
		definition := candidate.definition
		cueID := string(definition.SoundType)
		cue, exists := planner.library.document.Cues[cueID]
		if _, ambient := planner.definitions[definition.SoundType]; !exists || !ambient {
			return nil, fmt.Errorf("%w: ambient source %s has no ambient cue", ErrAudioLibrary, sourceID)
		}
		authored := cue.VolumeModifier * planner.library.document.recordingCalibration(cue.RecordingID)
		sourceGain := 0.0
		if authored > 0 {
			sourceGain = definition.BaseVolume / authored
		}
		distanceGain := berad.QuadraticDistanceGain(candidate.distance, definition.MaxDistance)
		gain := sourceGain * candidate.modifier * distanceGain
		if !finiteSourceGain(gain) {
			return nil, fmt.Errorf("%w: ambient source %s gain must be in [0,2]", ErrAudioLibrary, sourceID)
		}
		plans = append(plans, AmbientSourcePlan{ID: string(sourceID), CueID: cueID, Gain: gain,
			FadeIn: definition.FadeInTime, FadeOut: definition.FadeOutTime,
			Emitter: candidate.emitter, SourceGain: sourceGain, DistanceGain: distanceGain, Intensity: candidate.modifier})
	}
	slices.SortFunc(plans, func(left, right AmbientSourcePlan) int { return cmp.Compare(left.ID, right.ID) })

	return plans, nil
}

func finiteSourceGain(gain float64) bool {
	// NaN fails both comparisons; infinity fails the upper/lower bound.
	return gain >= 0 && gain <= 2
}
