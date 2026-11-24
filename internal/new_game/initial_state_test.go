package new_game

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/character_creation"
	"github.com/bradhannah/UltimaVRedux/internal/files"
	"github.com/bradhannah/UltimaVRedux/internal/legacy_save"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
)

func TestFreshTemplatePreparedSpellsDoNotInferCompletedLessons(t *testing.T) {
	directory := t.TempDir()
	gam := testInitialGAM()
	const preparedSpellsOffset = 0x24a
	for _, spell := range []references.Spell{references.RelXenBet, references.InXenMani} {
		gam[preparedSpellsOffset+spell.ID()] = 1
	}
	writeInitialTemplates(t, directory, gam, make([]byte, initialObjectsSize))
	prepared, err := PrepareInitialSnapshot(directory, testCreationResult(party_state.Male))
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Snapshot.Redux.PlayerKnowledge) != 0 {
		t.Fatal("fresh template inferred prior lessons")
	}
	for _, spell := range []references.Spell{references.RelXenBet, references.InXenMani} {
		if prepared.Snapshot.Inventory.Spells[spell] != 1 {
			t.Fatal("prepared inventory was removed")
		}
	}
	inferred := save_snapshot.InferLegacyPlayerKnowledge(prepared.Snapshot.Inventory, prepared.Snapshot.Progression)
	if len(inferred) != 2 {
		t.Fatal("progressed-save import must retain inferred knowledge")
	}
}

func TestPrepareInitialSnapshotPreservesTemplateAndAppliesResult(t *testing.T) {
	directory := t.TempDir()
	gam := testInitialGAM()
	initObjects := make([]byte, initialObjectsSize)
	initObjects[8], initObjects[9] = 0x10, 0x10
	initObjects[10], initObjects[11] = 20, 21
	writeInitialTemplates(t, directory, gam, initObjects)

	result := testCreationResult(party_state.Female)
	conversion, err := PrepareInitialSnapshot(directory, result)
	if err != nil {
		t.Fatal(err)
	}
	if issues := save_snapshot.Validate(conversion.Snapshot); len(issues) != 0 {
		t.Fatalf("prepared snapshot validation issues = %#v", issues)
	}

	snapshot := conversion.Snapshot
	avatar := snapshot.Party.Roster[0]
	if avatar.Name != "Avatar" || avatar.Gender != int(party_state.Female) || avatar.Class != int(legacy_save.ClassAvatar) || avatar.Status != int(legacy_save.StatusGood) {
		t.Fatalf("Avatar identity = %#v", avatar)
	}
	if avatar.Strength != 22 || avatar.Dexterity != 19 || avatar.Intelligence != 28 || avatar.CurrentMP != 28 {
		t.Fatalf("Avatar attributes = %#v", avatar)
	}
	if avatar.CurrentHP != 60 || avatar.MaximumHP != 60 || avatar.Experience != 150 || avatar.Level != 2 || avatar.Helmet != 7 || avatar.Weapon != 9 || avatar.Amulet != 12 {
		t.Fatalf("template-derived Avatar fields = %#v", avatar)
	}
	if snapshot.Party.Karma != 75 || snapshot.Inventory.Food != 63 || snapshot.Inventory.Gold != 150 || snapshot.World.Position.Location != 13 || snapshot.World.Position.X != 15 || snapshot.World.Position.Y != 15 {
		t.Fatalf("template-derived state = party %#v inventory %#v world %#v", snapshot.Party, snapshot.Inventory, snapshot.World)
	}
	if snapshot.World.Calendar.Year != 139 || snapshot.World.Calendar.Month != 4 || snapshot.World.Calendar.Day != 5 || snapshot.World.Calendar.Hour != 8 || snapshot.World.Calendar.Minute != 35 {
		t.Fatalf("template-derived calendar = %#v", snapshot.World.Calendar)
	}
	for index, character := range snapshot.Party.Roster {
		if !character.Occupied {
			t.Fatalf("roster[%d] is not occupied", index)
		}
		wantPartyMap := int(legacy_save.PartyMapUnrecruited)
		if index < 3 {
			wantPartyMap = int(legacy_save.PartyMapActive)
		}
		if character.PartyMap != wantPartyMap {
			t.Fatalf("roster[%d].PartyMap = %#x, want %#x", index, character.PartyMap, wantPartyMap)
		}
	}
	if snapshot.Random != (save_snapshot.RandomSnapshot{Seed: 42, SourceCalls: 64}) {
		t.Fatalf("random metadata = %#v", snapshot.Random)
	}
	if !snapshot.NeedsMapUnitReconstruction() || !hasConversionWarning(conversion.Warnings, "mapUnits", "legacy_small_map_npcs_reconstructed") {
		t.Fatalf("OOL reconstruction boundary = redux %#v warnings %#v", snapshot.Redux, conversion.Warnings)
	}
	if snapshot.Continuation.ActivePlayerIndex != -1 || snapshot.Continuation.BeggarKarmaCounter != 0 {
		t.Fatalf("initial continuation = %#v, want no active player and ready beggar counter", snapshot.Continuation)
	}

	gotGAM, err := os.ReadFile(filepath.Join(directory, files.InitGam))
	if err != nil {
		t.Fatal(err)
	}
	gotObjects, err := os.ReadFile(filepath.Join(directory, files.InitOol))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotGAM, gam) || !reflect.DeepEqual(gotObjects, initObjects) {
		t.Fatal("PrepareInitialSnapshot() modified original templates")
	}
}

func TestPatchAvatarTemplateChangesOnlyVerifiedFields(t *testing.T) {
	gam := testInitialGAM()
	before := append([]byte(nil), gam...)
	result := character_creation.Result{
		Name:    "NewName",
		Gender:  party_state.Female,
		Bonuses: character_creation.AttributeBonuses{Strength: 4, Dexterity: 5, Intelligence: 6},
	}

	if err := patchAvatarTemplate(gam, result); err != nil {
		t.Fatal(err)
	}

	allowed := map[int]bool{
		avatarGenderOffset:       true,
		avatarStrengthOffset:     true,
		avatarDexterityOffset:    true,
		avatarIntelligenceOffset: true,
		avatarMagicPointsOffset:  true,
	}
	for index := range avatarNameSize {
		allowed[avatarRecordOffset+index] = true
	}
	for index := range gam {
		if !allowed[index] && gam[index] != before[index] {
			t.Fatalf("unapproved byte %#x changed from %#x to %#x", index, before[index], gam[index])
		}
	}
	if got := string(gam[avatarRecordOffset : avatarRecordOffset+len(result.Name)]); got != result.Name {
		t.Fatalf("name = %q, want %q", got, result.Name)
	}
	if gam[avatarRecordOffset+len(result.Name)] != 0 || gam[avatarStrengthOffset] != 22 || gam[avatarDexterityOffset] != 24 || gam[avatarIntelligenceOffset] != 26 || gam[avatarMagicPointsOffset] != 26 {
		t.Fatalf("patched Avatar bytes = %#v", gam[avatarRecordOffset:avatarRecordOffset+32])
	}
}

func TestBuildInitialSavedObjectsUsesOriginalHalfLayout(t *testing.T) {
	initObjects := make([]byte, initialObjectsSize)
	for index := range initObjects {
		initObjects[index] = byte(index + 1)
	}

	savedObjects, err := buildInitialSavedObjects(initObjects)
	if err != nil {
		t.Fatal(err)
	}
	if len(savedObjects) != legacy_save.SavedObjectsSize {
		t.Fatalf("saved object length = %d", len(savedObjects))
	}
	if !reflect.DeepEqual(savedObjects[:initialObjectsSize], make([]byte, initialObjectsSize)) || !reflect.DeepEqual(savedObjects[initialObjectsSize:], initObjects) {
		t.Fatal("saved object halves do not match original new_char layout")
	}
}

func TestPrepareInitialSnapshotRejectsInvalidInputsWithoutPartialResult(t *testing.T) {
	validResult := testCreationResult(party_state.Male)
	directory := t.TempDir()
	writeInitialTemplates(t, directory, testInitialGAM(), make([]byte, initialObjectsSize))

	tests := []struct {
		name          string
		dataDirectory string
		result        character_creation.Result
		wantError     error
	}{
		{name: "blank data path", dataDirectory: " ", result: validResult, wantError: ErrInvalidInitialState},
		{name: "invalid name", dataDirectory: directory, result: func() character_creation.Result {
			result := validResult
			result.Name = ""
			return result
		}(), wantError: character_creation.ErrInvalidName},
		{name: "invalid gender", dataDirectory: directory, result: func() character_creation.Result {
			result := validResult
			result.Gender = 0
			return result
		}(), wantError: character_creation.ErrInvalidGender},
		{name: "negative bonus", dataDirectory: directory, result: func() character_creation.Result {
			result := validResult
			result.Bonuses.Strength = -1
			return result
		}(), wantError: character_creation.ErrInvalidBonuses},
		{name: "impossible bonus", dataDirectory: directory, result: func() character_creation.Result {
			result := validResult
			result.Bonuses.Intelligence = 11
			return result
		}(), wantError: character_creation.ErrInvalidBonuses},
		{name: "inconsistent result", dataDirectory: directory, result: func() character_creation.Result {
			result := validResult
			result.Bonuses = character_creation.AttributeBonuses{}
			return result
		}(), wantError: character_creation.ErrInconsistentResult},
		{name: "incomplete result", dataDirectory: directory, result: func() character_creation.Result {
			result := validResult
			result.RandomSourceCalls = 0
			return result
		}(), wantError: character_creation.ErrIncompleteResult},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := PrepareInitialSnapshot(test.dataDirectory, test.result)
			if !errors.Is(err, test.wantError) || !reflect.DeepEqual(got, legacy_save.ConversionResult{}) {
				t.Fatalf("PrepareInitialSnapshot() = %#v, %v; want zero result wrapping %v", got, err, test.wantError)
			}
		})
	}
}

func TestPrepareInitialSnapshotRejectsTemplateFailures(t *testing.T) {
	result := testCreationResult(party_state.Male)
	tests := []struct {
		name      string
		gam       []byte
		ool       []byte
		writeGAM  bool
		writeOOL  bool
		wantError error
	}{
		{name: "missing GAM", writeOOL: true, ool: make([]byte, initialObjectsSize), wantError: os.ErrNotExist},
		{name: "short GAM", writeGAM: true, gam: make([]byte, legacy_save.SavedGameSize-1), writeOOL: true, ool: make([]byte, initialObjectsSize), wantError: ErrInvalidInitialTemplate},
		{name: "missing OOL", writeGAM: true, gam: testInitialGAM(), wantError: os.ErrNotExist},
		{name: "long OOL", writeGAM: true, gam: testInitialGAM(), writeOOL: true, ool: make([]byte, initialObjectsSize+1), wantError: ErrInvalidInitialTemplate},
		{name: "malformed GAM", writeGAM: true, gam: make([]byte, legacy_save.SavedGameSize), writeOOL: true, ool: make([]byte, initialObjectsSize), wantError: legacy_save.ErrInvalidBundle},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if test.writeGAM {
				writeTestFile(t, filepath.Join(directory, files.InitGam), test.gam)
			}
			if test.writeOOL {
				writeTestFile(t, filepath.Join(directory, files.InitOol), test.ool)
			}

			got, err := PrepareInitialSnapshot(directory, result)
			if !errors.Is(err, test.wantError) || !reflect.DeepEqual(got, legacy_save.ConversionResult{}) {
				t.Fatalf("PrepareInitialSnapshot() = %#v, %v; want zero result wrapping %v", got, err, test.wantError)
			}
		})
	}
}

func testInitialGAM() []byte {
	const (
		characterRecordSize = 32
		partyCountOffset    = 0x02b5
		yearOffset          = 0x02ce
		activePlayerOffset  = 0x02d5
		monthOffset         = 0x02d7
		dayOffset           = 0x02d8
		hourOffset          = 0x02d9
		minuteOffset        = 0x02db
		karmaOffset         = 0x02e2
		locationOffset      = 0x02ed
		floorOffset         = 0x02ef
		partyXOffset        = 0x02f0
		partyYOffset        = 0x02f1
		foodOffset          = 0x0202
		goldOffset          = 0x0204
	)
	gam := make([]byte, legacy_save.SavedGameSize)
	for index := range save_snapshot.RosterSlotCount {
		base := avatarRecordOffset + index*characterRecordSize
		if index != 0 {
			copy(gam[base:base+avatarNameSize], fmt.Sprintf("C%02d", index))
		}
		gam[base+0x09] = byte(legacy_save.GenderMale)
		gam[base+0x0a] = byte(legacy_save.ClassFighter)
		gam[base+0x0b] = byte(legacy_save.StatusGood)
		gam[base+0x0c], gam[base+0x0d], gam[base+0x0e], gam[base+0x0f] = 18, 19, 20, 10
		binary.LittleEndian.PutUint16(gam[base+0x10:], 60)
		binary.LittleEndian.PutUint16(gam[base+0x12:], 60)
		binary.LittleEndian.PutUint16(gam[base+0x14:], uint16(150+index))
		gam[base+0x16] = 2
		gam[base+0x1f] = byte(legacy_save.PartyMapUnrecruited)
	}
	gam[avatarRecordOffset+0x0a] = byte(legacy_save.ClassAvatar)
	gam[avatarRecordOffset+0x19] = 7
	gam[avatarRecordOffset+0x1a] = 8
	gam[avatarRecordOffset+0x1b] = 9
	gam[avatarRecordOffset+0x1c] = 10
	gam[avatarRecordOffset+0x1d] = 11
	gam[avatarRecordOffset+0x1e] = 12
	gam[avatarRecordOffset+characterRecordSize+0x0a] = byte(legacy_save.ClassFighter)
	gam[avatarRecordOffset+2*characterRecordSize+0x0a] = byte(legacy_save.ClassBard)
	for index := 0; index < 3; index++ {
		gam[avatarRecordOffset+index*characterRecordSize+0x1f] = byte(legacy_save.PartyMapActive)
	}
	gam[partyCountOffset] = 3
	gam[activePlayerOffset] = 0xff
	binary.LittleEndian.PutUint16(gam[yearOffset:], 139)
	gam[monthOffset], gam[dayOffset], gam[hourOffset], gam[minuteOffset] = 4, 5, 8, 35
	gam[karmaOffset] = 75
	gam[locationOffset], gam[floorOffset], gam[partyXOffset], gam[partyYOffset] = 13, 0, 15, 15
	binary.LittleEndian.PutUint16(gam[foodOffset:], 63)
	binary.LittleEndian.PutUint16(gam[goldOffset:], 150)

	return gam
}

func testCreationResult(gender party_state.CharacterGender) character_creation.Result {
	return character_creation.Result{
		Name:              "Avatar",
		Gender:            gender,
		Bonuses:           character_creation.AttributeBonuses{Strength: 4, Intelligence: 8},
		RandomSeed:        42,
		RandomSourceCalls: 64,
	}
}

func writeInitialTemplates(t *testing.T, directory string, gam, ool []byte) {
	t.Helper()
	writeTestFile(t, filepath.Join(directory, files.InitGam), gam)
	writeTestFile(t, filepath.Join(directory, files.InitOol), ool)
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasConversionWarning(warnings []legacy_save.ConversionWarning, path, code string) bool {
	for _, warning := range warnings {
		if warning.Path == path && warning.Code == code {
			return true
		}
	}

	return false
}

// A lowercase copy of the original must work on case-sensitive file systems.
func TestReadInitialTemplateAcceptsLowercaseFileNames(t *testing.T) {
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, strings.ToLower(files.InitOol)), make([]byte, initialObjectsSize))

	if _, err := readInitialTemplate(directory, files.InitOol, initialObjectsSize); err != nil {
		t.Fatal(err)
	}
}
