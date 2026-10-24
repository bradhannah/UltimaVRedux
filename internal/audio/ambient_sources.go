package audio

import (
	"fmt"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

type ambientCandidate struct {
	emitter            string
	definition         AmbientSoundDefinition
	distance, modifier float64
}

// collectAmbientSources is Redux's world-to-soundscape policy: the strongest tile
// per cue, then the game's code-owned sources with their stable identities.
func collectAmbientSources(tileDefinitions map[indexes.SpriteIndex]AmbientSoundDefinition,
	definitions map[AmbientSoundType]AmbientSoundDefinition, position references.Position,
	tiles []TileWithPosition, dynamic []AmbientSoundSource,
) map[AmbientSoundType]ambientCandidate {
	sources := make(map[AmbientSoundType]ambientCandidate)
	for _, tile := range tiles {
		definition, exists := tileDefinitions[tile.Tile.Index]
		if !exists {
			continue
		}
		distance := ambientSourceDistance(position, tile.Position)
		previous, exists := sources[definition.SoundType]
		if strongerAmbientSource(definition, distance, previous.definition, previous.distance, exists) {
			sources[definition.SoundType] = ambientCandidate{
				emitter: "", definition: definition, distance: distance, modifier: 1,
			}
		}
	}
	for _, source := range dynamic {
		collectDynamicAmbientSource(sources, definitions, position, source)
	}

	return sources
}

func collectDynamicAmbientSource(
	sources map[AmbientSoundType]ambientCandidate, definitions map[AmbientSoundType]AmbientSoundDefinition,
	listener references.Position, source AmbientSoundSource,
) {
	definition := definitions[source.SoundType]
	definition.SoundType = source.SoundType // An unknown cue keeps its name for the planner's error.
	if source.Background {
		definition.MaxDistance = 0
	}
	distance := ambientSourceDistance(listener, source.Position)
	if definition.MaxDistance == 0 {
		distance = 0 // A background source is non-spatial, not tied to the last listener snapshot.
	}
	if distance > definition.MaxDistance {
		return
	}
	key := AmbientSoundType(fmt.Sprintf("source/%s/%s/%d/%d",
		source.Emitter, source.SoundType, source.Position.X, source.Position.Y))
	if source.ID != "" {
		key = AmbientSoundType("source/" + source.ID)
	}
	if definition.MaxDistance == 0 {
		key = AmbientSoundType("background/" + string(source.SoundType))
	}
	if source.Emitter == lighthouseEmitter {
		key = source.SoundType // Preserve the coastal/interior lighthouse voice.
	}
	previous, exists := sources[key]
	if !exists || distance < previous.distance || source.Emitter == lighthouseEmitter {
		sources[key] = ambientCandidate{
			emitter: source.Emitter, definition: definition, distance: distance, modifier: source.VolumeModifier,
		}
	}
}

func ambientSourceDistance(left, right references.Position) float64 {
	dx, dy := float64(left.X-right.X), float64(left.Y-right.Y)

	return math.Sqrt(dx*dx + dy*dy)
}
