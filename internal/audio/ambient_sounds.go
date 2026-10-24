// Package audio owns Redux audio catalogs, policy and authoring adapters.
package audio

import (
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	berad "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

// AmbientSoundType is a game-owned cue identity, not an engine enum.
type AmbientSoundType string

// AmbientLighthouse is the only ambient cue game code names; the library's routing
// supplies every other one.
const AmbientLighthouse AmbientSoundType = "ambient/AmbientLighthouse"

// AmbientSoundDefinition is derived game routing data. BaseVolume includes cue
// gain/calibration; AmbientPlanner separates those factors before module playback.
type AmbientSoundDefinition struct {
	SoundType   AmbientSoundType
	FileName    string
	MaxDistance float64
	BaseVolume  float64
	FadeInTime  time.Duration
	FadeOutTime time.Duration
}

// TileWithPosition supplies already-projected game terrain to source discovery.
type TileWithPosition struct {
	Tile     *references.Tile
	Position references.Position
}

// AmbientSoundSource is game world intent: which cue sounds, where and how
// strongly. The planner takes the cue's recording, radius, level and fades from
// the library. A background source has no position or distance falloff.
type AmbientSoundSource struct {
	ID             string
	Emitter        string
	Position       references.Position
	SoundType      AmbientSoundType
	Background     bool
	VolumeModifier float64
}

func defaultAmbientSoundDefinitions() map[AmbientSoundType]AmbientSoundDefinition {
	library := productionLibrary()

	return buildAmbientDefinitions(library, library.recordingPath)
}

func ambientSoundDefinitions(project *Library) map[AmbientSoundType]AmbientSoundDefinition {
	return buildAmbientDefinitions(project.document, project.recordingPath)
}

func buildAmbientDefinitions(library *audioLibrary,
	recordingPath func(string) string,
) map[AmbientSoundType]AmbientSoundDefinition {
	definitions := make(map[AmbientSoundType]AmbientSoundDefinition)
	for id, profile := range library.Routing.Ambient {
		cue := library.Cues[id]
		kind := AmbientSoundType(id)
		definitions[kind] = AmbientSoundDefinition{SoundType: kind, FileName: recordingPath(cue.RecordingID),
			MaxDistance: profile.RadiusTiles, BaseVolume: cue.VolumeModifier * library.recordingCalibration(cue.RecordingID),
			FadeInTime:  time.Duration(profile.FadeInMs) * time.Millisecond,
			FadeOutTime: time.Duration(profile.FadeOutMs) * time.Millisecond}
	}

	return definitions
}

func ambientSpriteMappings() map[indexes.SpriteIndex]AmbientSoundType {
	return ambientSpriteMappingsFor(productionLibrary().Routing)
}

func ambientSpriteMappingsFor(routing Routing) map[indexes.SpriteIndex]AmbientSoundType {
	mappings := make(map[indexes.SpriteIndex]AmbientSoundType)
	for sprite := range *references.NewTileReferences() {
		assignment := routing.Tiles[tileAudioKey(sprite)]
		if assignment.AmbientCue != "" {
			mappings[sprite] = AmbientSoundType(assignment.AmbientCue)
		}
	}

	return mappings
}

func strongerAmbientSource(candidate AmbientSoundDefinition, distance float64,
	previous AmbientSoundDefinition, previousDistance float64, existing bool,
) bool {
	volume := candidate.BaseVolume * berad.QuadraticDistanceGain(distance, candidate.MaxDistance)
	if volume <= 0 {
		return false
	}
	priorVolume := previous.BaseVolume * berad.QuadraticDistanceGain(previousDistance, previous.MaxDistance)

	return !existing || volume > priorVolume || (volume == priorVolume && distance < previousDistance)
}
