// Package tooling supplies Redux project policy to the standalone SDK audio tools.
// Keeping this adapter outside package audio prevents gameplay from importing TUI.
package tooling

import (
	"fmt"
	"path/filepath"
	"strings"

	gameaudio "github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	editor "github.com/bradhannah/berad-sdk/tools/berad-audio/editor"
)

const categoryMusic = "music"
const categoryAmbient = "ambient"
const categorySFX = "sfx"
const groupNaval = "naval"
const familyAmbience = "ambience"
const newAmbientGain = 0.3

// OpenAuthoringEditor supplies Redux policy to the shared SDK authoring engine.
// No embedded assets are selected: the editable manifest is explicitly disk-backed.
func OpenAuthoringEditor(path string) (*authoring.LibraryEditor, error) {
	options := authoring.DefaultEditorOptions(ebitengine.Decode)
	options.Validate = gameaudio.ReduxLibraryValidator
	options.Calibration = func(family string) authoring.CalibrationOptions {
		value := authoring.DefaultCalibrationOptions()
		value.TargetDB = gameaudio.DefaultCalibrationTarget(family)
		if family == "score" || family == "instrument" || family == familyAmbience || family == "nature" || family == "fire" {
			value.Method = authoring.GatedLoudness
		}

		return value
	}
	options.AcceptExistingPath = func(path string) bool {
		return strings.HasPrefix(path, "sounds/") || strings.HasPrefix(path, "music/")
	}
	options.ImportDestination = func(
		identity string, previous authoring.Recording, source, fingerprint string,
	) (string, error) {
		directory := "sounds/imported"
		if previous.File != nil && strings.HasPrefix(*previous.File, "music/") {
			directory = "music/imported"
		}

		return directory + "/" + identity + "_" + fingerprint[:12] + strings.ToLower(filepath.Ext(source)), nil
	}
	options.Targets = authoring.AuthoringTargets{Tiles: gameaudio.TileTargets(), Families: gameaudio.AcousticFamilies(),
		Groups: map[string]authoring.AssignmentGroup{
			categoryMusic: {Label: "Music", Targets: gameaudio.MusicTargets(),
				DefaultTarget: "default", Category: categoryMusic},
			"dungeon": {Label: "Dungeon backgrounds", Targets: gameaudio.DungeonTargets(),
				DefaultTarget: "default", Category: categoryAmbient},
			groupNaval: {Label: "Naval situations", Targets: gameaudio.NavalTargets(),
				DefaultTarget: "", Category: categoryAmbient},
		}, CueCategories: make(map[string]string)}
	// Derive explicit categories from the selected document, not compiled cue IDs.
	// The validator still enforces the game's known prefix/completeness policy.
	doc, err := authoring.OpenLibraryProject(path, options)
	if err != nil {
		return nil, fmt.Errorf("open SDK authoring project: %w", err)
	}
	options.Targets.CueCategories = cueCategories(doc.CueIDs())
	doc, err = authoring.OpenLibraryProject(path, options)
	if err != nil {
		return nil, fmt.Errorf("open categorized SDK project: %w", err)
	}

	return doc, nil
}

func cueCategories(ids []string) map[string]string {
	result := make(map[string]string, len(ids))
	for _, cue := range ids {
		category := categorySFX
		if strings.HasPrefix(cue, "music/") {
			category = categoryMusic
		}
		if strings.HasPrefix(cue, "ambient/") {
			category = categoryAmbient
		}
		result[cue] = category
	}

	return result
}

// AudioEditorOptions contains presentation defaults for Redux's supported editor.
func AudioEditorOptions() editor.Options {
	volumes := config.DefaultAudioConfiguration()

	return editor.Options{SliderGain: gameaudio.ConvertSliderToVolume, MixLabel: "GAME MIX",
		InitialSliders: &editor.SliderLevels{Music: volumes.MusicVolume, SFX: volumes.SoundEffectVolume,
			Ambience: volumes.AmbientVolume},
		SavedNotice: "Saved library.json. Rebuild the game to use these changes.",
		Ambient: &editor.AmbientPreset{CuePrefix: "ambient/", Profile: "ambient_bed",
			Family: familyAmbience, Gain: newAmbientGain,
			HowToHear: "Assign this ambient cue to a tile family or dungeon background; supply its recording to hear it."},
		RecordingCategory: func(_ string, recording authoring.Recording, family string) string {
			if recording.File != nil && strings.HasPrefix(*recording.File, "music/") {
				return categoryMusic
			}
			if family == familyAmbience {
				return categoryAmbient
			}

			return categorySFX
		},
		TargetPolicy: targetPolicy}
}

func targetPolicy(group, target string) editor.TargetPolicy {
	var policy editor.TargetPolicy
	policy.Category = categoryAmbient
	switch {
	case group == categoryMusic:
		policy.Category, policy.Title = categoryMusic, "CHOOSE MUSIC"
		policy.Help = "Preview plays immediately. Gameplay retains scheduling and interruption rules."
	case group == groupNaval && target == "sailing":
		policy.Category, policy.Title = categoryMusic, "CHOOSE SAILING MUSIC"
		policy.Help = "Plays once when sails are hoisted; fades on furling. Combat music takes precedence."
	case group == groupNaval:
		policy.Background, policy.Title = true, "CHOOSE ONBOARD AMBIENCE"
		policy.Help = "Non-spatial ambience loops while aboard, including when sails are down."
	case group == "dungeon":
		policy.Background, policy.Title = true, "CHOOSE DUNGEON BACKGROUND"
		policy.Help = "Background continues across this dungeon's corridors and rooms; fades on departure."
	default:
		policy.Spatial, policy.Title = true, "CHOOSE AMBIENT SOUND"
	}

	return policy
}
