package audio

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

//nolint:godox // The owner intentionally deferred recordings for these wired slots.
func dungeonAmbientSource(library *Library, definitions map[AmbientSoundType]AmbientSoundDefinition,
	location references.Location, listener references.Position,
) (AmbientSoundSource, bool) {
	// TODO(audio): Supply cave/brick recordings; loop through floors/combat, fade on exit.
	cue := AmbientSoundType(dungeonCue(library.document.Routing, location))
	if cue == "" {
		return AmbientSoundSource{}, false //nolint:exhaustruct_v5 // Explicit or inherited silence.
	}
	if _, exists := definitions[cue]; !exists {
		panic("validated dungeon background has no definition")
	}

	return AmbientSoundSource{ID: "", Emitter: "", Position: listener, SoundType: cue,
		Background: true, VolumeModifier: 1}, true
}

func navalAmbientSource(
	routing Routing, definitions map[AmbientSoundType]AmbientSoundDefinition,
	vessel string, listener references.Position,
) (AmbientSoundSource, bool) {
	if vessel != NavalFrigate && vessel != NavalSkiff {
		return AmbientSoundSource{}, false //nolint:exhaustruct_v5 // No supported boarded vessel.
	}
	cue := routing.Groups[routingGroupNaval][vessel]
	if cue == nil {
		return AmbientSoundSource{}, false //nolint:exhaustruct_v5 // Authored silence.
	}
	if _, exists := definitions[AmbientSoundType(*cue)]; !exists {
		panic("validated naval background has no definition")
	}

	return AmbientSoundSource{ID: "", Emitter: vessel, Position: listener, SoundType: AmbientSoundType(*cue),
		Background: true, VolumeModifier: 1}, true
}
