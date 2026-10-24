// Package audio owns Redux audio catalogs, policy and authoring adapters.
package audio

import (
	"math"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
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
	LoopSound   bool
	FadeInTime  time.Duration
	FadeOutTime time.Duration
}

// TileWithPosition supplies already-projected game terrain to source discovery.
type TileWithPosition struct {
	Tile     *references.Tile
	Position references.Position
}

// AmbientSoundSource is game world intent; the planner handles roles, grouping
// and projection before passing neutral source requests to Berad Audio.
type AmbientSoundSource struct {
	ID             string
	Emitter        string
	Position       references.Position
	SoundType      AmbientSoundType
	IsActive       bool
	IsDynamic      bool
	SoundFile      string
	MaxDistance    float64
	baseVolume     float64
	VolumeModifier float64
	FadeInTime     time.Duration
	FadeOutTime    time.Duration
}

// GetBaseVolume exposes the derived authored coefficient without mutable access.
func (source *AmbientSoundSource) GetBaseVolume() float64 { return source.baseVolume }

// SetBaseVolume bounds the calibrated game source coefficient.
func (source *AmbientSoundSource) SetBaseVolume(volume float64) {
	const maximum = 4.0
	if math.IsNaN(volume) || math.IsInf(volume, 0) {
		logging.Warnf(logging.CategoryAudio, "Invalid non-finite ambient base volume")
		volume = 0
	} else if volume < 0 || volume > maximum {
		logging.Warnf(logging.CategoryAudio, "Attempted to set invalid base volume %.2f, clamping to [0, 4]", volume)
		volume = math.Max(0, math.Min(maximum, volume))
	}
	source.baseVolume = volume
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
			LoopSound: true, FadeInTime: time.Duration(profile.FadeInMs) * time.Millisecond,
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
