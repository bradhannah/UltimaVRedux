package audio

import "github.com/bradhannah/UltimaVRedux/internal/references"

const lighthouseEmitter = "lighthouse"

func lighthouseAmbientSource(definitions map[AmbientSoundType]AmbientSoundDefinition,
	location references.Location, listener references.Position,
) (AmbientSoundSource, bool) {
	if location != references.Fogsbane && location != references.Stormcrow &&
		location != references.Greyhaven && location != references.Waveguide {
		return AmbientSoundSource{}, false //nolint:exhaustruct // No lighthouse location source.
	}
	if _, exists := definitions[AmbientLighthouse]; !exists {
		panic("lighthouse ambient cue has no definition")
	}

	return AmbientSoundSource{ID: "", Emitter: lighthouseEmitter, Position: listener, SoundType: AmbientLighthouse,
		Background: true, VolumeModifier: 1}, true
}
