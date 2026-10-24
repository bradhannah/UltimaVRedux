package audio

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	sdkcatalog "github.com/bradhannah/berad-sdk/tools/berad-audio/catalog"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestFountainAmbientAndDrinkingUseSeparateCues(t *testing.T) {
	library := productionLibrary()
	ambient := defaultAmbientSoundDefinitions()[AmbientFountain]
	require.Equal(t, library.recordingPath("fountain_gurgle_01"), ambient.FileName)
	require.NotEmpty(t, ambient.FileName)
	for _, sprite := range []indexes.SpriteIndex{
		indexes.Fountain1, indexes.Fountain2, indexes.Fountain3, indexes.Fountain4,
	} {
		require.Equal(t, AmbientFountain, ambientSpriteMappings()[sprite])
	}
	require.Equal(t, "fountain_drink_01", library.Cues["sfx/SoundFountainDrink"].RecordingID)
	drink, exists := GetSoundEffectMetadata(game_state.SoundFountainDrink)
	require.True(t, exists)
	require.Equal(t, library.Cues["sfx/SoundFountainDrink"].RecordingID, drink.RecordingID)
}

func TestCatalogCoversEveryDeclaredEffectAndProductionAssignment(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../game_state/system_callbacks.go", nil, 0)
	require.NoError(t, err)
	names := catalogEffectNames()
	declared := map[string]bool{}
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}
		for _, spec := range group.Specs {
			values := spec.(*ast.ValueSpec)
			for _, name := range values.Names {
				if strings.HasPrefix(name.Name, "Sound") {
					declared[name.Name] = true
				}
			}
		}
	}
	require.Len(t, names, len(declared), "new SoundEffects must get stable identities even without metadata")
	seen := map[string]bool{}
	for _, name := range names {
		require.True(t, declared[name], "identity for undeclared symbol %s", name)
		require.False(t, seen[name], "duplicate identity %s", name)
		seen[name] = true
	}
	catalog, err := BuildCatalog(false)
	require.NoError(t, err)
	for effect, metadata := range soundEffectMetadataMap() {
		name, exists := names[effect]
		require.True(t, exists, "metadata lacks a stable identity: %v", effect)
		moment := findCatalogMoment(t, catalog, "sfx/"+name)
		require.Equal(t, productionLibrary().recordingPath(metadata.RecordingID), moment.ConfiguredFile)
		require.Equal(t, metadata.RecordingID, moment.AssetID)
		require.Equal(t, metadata.VolumeModifier, *moment.ConfiguredGain)
		if !isPlaceholderSound(effect) {
			require.Equal(t, CatalogPresent, moment.Availability)
			require.NotEmpty(t, moment.AssetID)
		}
	}
	for effect, name := range names {
		if _, exists := soundEffectMetadataMap()[effect]; !exists {
			require.Equal(t, "missing_definition", findCatalogMoment(t, catalog, "sfx/"+name).Availability)
		}
	}
	for id, metadata := range musicTrackMetadataMap() {
		moment := findCatalogMoment(t, catalog, "music/track/"+id)
		require.Equal(t, productionLibrary().recordingPath(metadata.RecordingID), moment.ConfiguredFile)
		require.Equal(t, metadata.VolumeModifier, *moment.ConfiguredGain)
	}
	for kind, definition := range defaultAmbientSoundDefinitions() {
		moment := findCatalogMoment(t, catalog, "ambient/"+catalogAmbientNames()[kind])
		require.Equal(t, definition.FileName, moment.ConfiguredFile)
		require.InDelta(t, definition.BaseVolume,
			*moment.ConfiguredGain*productionLibrary().recordingCalibration(moment.AssetID), 1e-9)
		require.Equal(t, definition.MaxDistance, *moment.MaxDistanceTiles)
	}
	for context, track := range musicContextTracks() {
		moment := findCatalogMoment(t, catalog, "music/context/"+context)
		metadata, exists := musicTrackMetadataMap()[track]
		require.True(t, exists)
		require.Equal(t, productionLibrary().recordingPath(metadata.RecordingID), moment.ConfiguredFile)
	}
}

func TestCatalogUsesAuthoredIntentWithoutAcceptingPlaceholderAssignments(t *testing.T) {
	catalog, err := BuildCatalog(false)
	require.NoError(t, err)
	library := productionLibrary()
	for id := range library.Cues {
		moment := findCatalogMoment(t, catalog, id)
		classification := library.Taxonomy.Profiles[library.Taxonomy.cueProfile(id)]
		require.Equal(t, cueCategory(id), moment.Category, id)
		require.Equal(t, classification.TargetAcousticFamily, moment.Family, id)
		require.Equal(t, classification.Phase, moment.Phase, id)
		require.Equal(t, classification.Outcome, moment.Outcome, id)
		require.Equal(t, classification.MixRole, moment.MixRole, id)
	}
	for _, asset := range catalog.Assets {
		if asset.Registered {
			require.Equal(t, library.Taxonomy.recordingFamily(asset.ID), asset.AcousticFamily)
		}
	}
	push := findCatalogMoment(t, catalog, "sfx/SoundPushObject")
	require.Equal(t, "mechanical_interaction", push.Family)
	require.Equal(t, []string{"mechanical_interaction"}, push.AssignedFamilies)
}

func TestCatalogIsDeterministicAndPreservesIndependentGaps(t *testing.T) {
	first, err := BuildCatalog(false)
	require.NoError(t, err)
	second, err := BuildCatalog(false)
	require.NoError(t, err)
	require.Equal(t, first, second)
	encoded, err := json.Marshal(first)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"calibrationGainDb":null`, "unknown calibration is not invented approval")
	push := findCatalogMoment(t, first, "sfx/SoundPushObject")
	require.Equal(t, CatalogPresent, push.Availability)
	require.Equal(t, CatalogUnreviewed, push.AssignmentReview)
	require.Equal(t, CatalogUnreviewed, push.ListeningReview)
	require.Equal(t, catalogNotScanned, push.TriggerCoverage)
	failure := findCatalogMoment(t, first, "sfx/SoundSpellFailure")
	require.Equal(t, CatalogPresent, failure.Availability)
	gap := findCatalogMoment(t, first, "sfx/SoundRingVanish")
	require.Equal(t, CatalogMissing, gap.Availability)
	require.Equal(t, CatalogPlaceholder, gap.AssignmentReview)
	unlock := findCatalogMoment(t, first, "sfx/SoundMagicUnlock")
	require.Equal(t, CatalogPresent, unlock.Availability)
	require.NotEmpty(t, unlock.AssetID)
	require.NotNil(t, unlock.ConfiguredGain)
	for _, asset := range first.Assets {
		if asset.Availability == CatalogPresent {
			require.Len(t, asset.SHA256, 64)
			require.Positive(t, asset.DurationMs)
		} else {
			require.Empty(t, asset.Path)
			require.Empty(t, asset.SHA256)
		}
		require.Equal(t, productionLibrary().Recordings[asset.ID].CalibrationGainDB, asset.CalibrationGainDB)
		require.Equal(t, CatalogUnreviewed, asset.ListeningReview)
	}
	metal := findCatalogMoment(t, first, "sfx/SoundWalkMetal1")
	require.Equal(t, productionLibrary().Recordings[metal.AssetID].PlaybackWindow, metal.Window)
	index := slices.IndexFunc(first.Assets, func(asset sdkcatalog.CatalogAsset) bool { return asset.ID == metal.AssetID })
	require.NotEqual(t, -1, index)
	if metal.Window != nil {
		require.GreaterOrEqual(t, first.Assets[index].DurationMs, metal.Window.EndMs, "excerpt must fit its recording")
	}
}

func TestCatalogAnnotationsDoNotMisdiagnoseReplacementRecordings(t *testing.T) {
	moment := newCatalogMoment("sfx/SoundPushObject", "Push", catalogSFX)
	moment.AssetID = "object_push_wood_01"
	catalog := &Catalog{Moments: []CatalogMoment{moment}}
	applyCatalogNotes(catalog)
	require.Equal(t, CatalogUnreviewed, catalog.Moments[0].AssignmentReview)
	require.NotContains(t, catalog.Moments[0].Note, "healing")
}

func TestCatalogGraphRejectsUnresolvedAndStaleLinks(t *testing.T) {
	catalog, err := BuildCatalog(false)
	require.NoError(t, err)
	catalog.Moments[0].AssetID = "asset/not-found"
	require.ErrorIs(t, catalog.Validate(), ErrAudioCatalog)
	catalog, err = BuildCatalog(false)
	require.NoError(t, err)
	catalog.Assets[0].MomentIDs = append(catalog.Assets[0].MomentIDs, "sfx/Unknown")
	require.ErrorIs(t, catalog.Validate(), ErrAudioCatalog)
}

func TestCatalogFindsReferencesWithoutCountingCommentsStringsOrTests(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"cmd/ultimav", "internal/audio", "internal/game_state"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o700))
	}
	source := `package main
// SoundSpellFailure is a comment, not playback.
var description = "SoundSpellFailure"
func actual() { callbacks.PlaySoundEffect(game_state.SoundPushObject) }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "cmd/ultimav/example.go"), []byte(source), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cmd/ultimav/example_test.go"),
		[]byte("package main\nfunc testOnly() { callbacks.PlaySoundEffect(game_state.SoundSpellFailure) }"), 0o600))
	catalog, err := BuildCatalog(false)
	require.NoError(t, err)
	require.NoError(t, catalog.AttachSourceEvidence(root))
	push := findCatalogMoment(t, catalog, "sfx/SoundPushObject")
	require.Equal(t, "playback_request_found", push.TriggerCoverage)
	require.Equal(t, CatalogUnreviewed, push.ListeningReview)
	require.Len(t, push.Evidence, 1)
	require.Equal(t, "actual", push.Evidence[0].Function)
	require.Equal(t, "no_static_reference_found", findCatalogMoment(t, catalog, "sfx/SoundSpellFailure").TriggerCoverage)
	fingerprint := catalog.SourceTreeSHA256
	require.Len(t, fingerprint, 64)
	require.NoError(t, catalog.AttachSourceEvidence(root))
	require.Equal(t, fingerprint, catalog.SourceTreeSHA256)
	require.NoError(t, os.WriteFile(filepath.Join(root, "cmd/ultimav/example.go"), []byte(source+"\n"), 0o600))
	require.NoError(t, catalog.AttachSourceEvidence(root))
	require.NotEqual(t, fingerprint, catalog.SourceTreeSHA256)
	require.Len(t, findCatalogMoment(t, catalog, "sfx/SoundPushObject").Evidence, 1, "rescanning replaces evidence")
}

func findCatalogMoment(t *testing.T, catalog *Catalog, id string) CatalogMoment {
	t.Helper()
	for _, moment := range catalog.Moments {
		if moment.ID == id {
			return moment
		}
	}
	t.Fatalf("missing moment %s", id)
	return CatalogMoment{}
}
