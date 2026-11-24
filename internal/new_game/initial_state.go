// Package new_game prepares detached native state for new-game workflows.
package new_game

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/character_creation"
	"github.com/bradhannah/UltimaVRedux/internal/files"
	"github.com/bradhannah/UltimaVRedux/internal/legacy_save"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
)

const (
	initialObjectsSize       = legacy_save.SavedObjectsSize / 2
	avatarRecordOffset       = 0x0002
	avatarNameSize           = 9
	avatarGenderOffset       = avatarRecordOffset + 0x09
	avatarStrengthOffset     = avatarRecordOffset + 0x0c
	avatarDexterityOffset    = avatarRecordOffset + 0x0d
	avatarIntelligenceOffset = avatarRecordOffset + 0x0e
	avatarMagicPointsOffset  = avatarRecordOffset + 0x0f
	minimumAvatarStrength    = 20
	maximumOriginalAttribute = 255
)

var (
	// ErrInvalidInitialState indicates that preparation inputs cannot produce original save bytes.
	ErrInvalidInitialState = errors.New("invalid initial character state")
	// ErrInvalidInitialTemplate indicates that a configured INIT file has the wrong exact size.
	ErrInvalidInitialTemplate = errors.New("invalid original initial template")
)

// PrepareInitialSnapshot applies a completed questionnaire result to detached
// original INIT templates and converts them into a validated native snapshot.
func PrepareInitialSnapshot(
	originalDataDirectory string,
	result character_creation.Result,
) (legacy_save.ConversionResult, error) {
	if strings.TrimSpace(originalDataDirectory) == "" {
		return legacy_save.ConversionResult{}, fmt.Errorf(
			"%w: configured original-data directory is required",
			ErrInvalidInitialState,
		)
	}
	if err := validateInitialResult(result); err != nil {
		return legacy_save.ConversionResult{}, err
	}

	gam, err := readInitialTemplate(originalDataDirectory, files.InitGam, legacy_save.SavedGameSize)
	if err != nil {
		return legacy_save.ConversionResult{}, err
	}

	initObjects, err := readInitialTemplate(originalDataDirectory, files.InitOol, initialObjectsSize)
	if err != nil {
		return legacy_save.ConversionResult{}, err
	}
	savedObjects, err := buildInitialSavedObjects(initObjects)
	if err != nil {
		return legacy_save.ConversionResult{}, err
	}

	patchedGAM := append([]byte(nil), gam...)
	if err := patchAvatarTemplate(patchedGAM, result); err != nil {
		return legacy_save.ConversionResult{}, fmt.Errorf("patching Avatar template: %w", err)
	}
	bundle, err := legacy_save.DecodeBundle(patchedGAM, savedObjects)
	if err != nil {
		return legacy_save.ConversionResult{}, fmt.Errorf("decoding initial templates: %w", err)
	}
	conversion, err := legacy_save.Convert(bundle)
	if err != nil {
		return legacy_save.ConversionResult{}, fmt.Errorf("converting initial templates: %w", err)
	}
	conversion.Snapshot.Random = save_snapshot.RandomSnapshot{
		Seed:        result.RandomSeed,
		SourceCalls: result.RandomSourceCalls,
	}
	// INIT templates describe a fresh journey, not a progressed imported save.
	// Prepared mixtures are inventory, not evidence of a completed lesson here.
	// Preserve normal legacy-import inference and all template inventory amounts.
	conversion.Snapshot.Redux.PlayerKnowledge = []save_snapshot.PlayerKnowledgeSnapshot{}
	if issues := save_snapshot.Validate(conversion.Snapshot); len(issues) != 0 {
		return legacy_save.ConversionResult{}, fmt.Errorf(
			"validating prepared initial snapshot: %w",
			&save_snapshot.ValidationError{Issues: issues},
		)
	}

	return conversion, nil
}

func validateInitialResult(result character_creation.Result) error {
	if err := character_creation.ValidateResult(result); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInitialState, err)
	}

	return nil
}

func readInitialTemplate(directory, filename string, expectedSize int) ([]byte, error) {
	templatePath := files.Resolve(directory, filename)
	//nolint:gosec // The caller supplies the configured original-data directory; filenames are constants.
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("reading configured %s: %w", filename, err)
	}
	if len(data) != expectedSize {
		return nil, fmt.Errorf(
			"%w: %s must be exactly %d bytes; got %d",
			ErrInvalidInitialTemplate,
			filename,
			expectedSize,
			len(data),
		)
	}

	return data, nil
}

func patchAvatarTemplate(gam []byte, result character_creation.Result) error {
	strength := max(minimumAvatarStrength, int(gam[avatarStrengthOffset])+result.Bonuses.Strength)
	dexterity := int(gam[avatarDexterityOffset]) + result.Bonuses.Dexterity
	intelligence := int(gam[avatarIntelligenceOffset]) + result.Bonuses.Intelligence
	if strength > maximumOriginalAttribute || dexterity > maximumOriginalAttribute ||
		intelligence > maximumOriginalAttribute {
		return fmt.Errorf("%w: final Avatar attributes must not exceed %d", ErrInvalidInitialState, maximumOriginalAttribute)
	}

	clear(gam[avatarRecordOffset : avatarRecordOffset+avatarNameSize])
	copy(gam[avatarRecordOffset:avatarRecordOffset+avatarNameSize], result.Name)
	gam[avatarGenderOffset] = byte(result.Gender)
	gam[avatarStrengthOffset] = byte(strength)
	gam[avatarDexterityOffset] = byte(dexterity)       //nolint:gosec // Bounded above before narrowing.
	gam[avatarIntelligenceOffset] = byte(intelligence) //nolint:gosec // Bounded above before narrowing.
	gam[avatarMagicPointsOffset] = byte(intelligence)  //nolint:gosec // Bounded above before narrowing.

	return nil
}

func buildInitialSavedObjects(initObjects []byte) ([]byte, error) {
	if len(initObjects) != initialObjectsSize {
		return nil, fmt.Errorf(
			"%w: %s must be exactly %d bytes; got %d",
			ErrInvalidInitialTemplate,
			files.InitOol,
			initialObjectsSize,
			len(initObjects),
		)
	}

	savedObjects := make([]byte, legacy_save.SavedObjectsSize)
	copy(savedObjects[initialObjectsSize:], initObjects)

	return savedObjects, nil
}
