package save_snapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPlaythroughID = "0123456789abcdef0123456789abcdef"

func TestEnvelopeTwoMetadataCombinationsRoundTripAndMatchSchema(t *testing.T) {
	t.Parallel()
	require.Equal(t, 2, CurrentEnvelopeVersion)
	require.Equal(t, 26, CurrentVersion)
	require.Equal(t, 80, MaxUserDescriptionRunes)
	compiled, document := compileCurrentNativeSaveSchema(t)
	walkJSONSchema(t, document, func(reference string) {
		assert.True(t, strings.HasPrefix(reference, "#/$defs/") || strings.HasPrefix(reference, "native-save-e2-s16.schema.json#/$defs/") ||
			strings.HasPrefix(reference, "native-save-e2-s17.schema.json#/$defs/") ||
			strings.HasPrefix(reference, "native-save-e2-s18.schema.json#/$defs/") ||
			strings.HasPrefix(reference, "native-save-e2-s19.schema.json#/$defs/") ||
			strings.HasPrefix(reference, "native-save-e2-s20.schema.json#/$defs/") ||
			strings.HasPrefix(reference, "native-save-e2-s20.schema.json#/properties/") ||
			strings.HasPrefix(reference, "native-save-e2-s21.schema.json#/$defs/") ||
			strings.HasPrefix(reference, "native-save-e2-s21.schema.json#/properties/") ||
			strings.HasPrefix(reference, "native-save-e2-s22.schema.json#/$defs/") ||
			strings.HasPrefix(reference, "native-save-e2-s22.schema.json#/properties/") ||
			strings.HasPrefix(reference, "native-save-e2-s23.schema.json#/$defs/") ||
			strings.HasPrefix(reference, "native-save-e2-s23.schema.json#/properties/") ||
			strings.HasPrefix(reference, "native-save-e2-s24.schema.json#/$defs/") ||
			strings.HasPrefix(reference, "native-save-e2-s24.schema.json#/properties/") ||
			strings.HasPrefix(reference, "native-save-e2-s25.schema.json#/$defs/") ||
			strings.HasPrefix(reference, "native-save-e2-s25.schema.json#/properties/"), "unapproved reference %q", reference)
	})
	for _, identity := range []string{"", testPlaythroughID} {
		for _, note := range []string{"", "  About to go to Doom!  ", strings.Repeat("\u754c", 80), strings.Repeat("\U0001f409", 80), "e\u0301"} {
			for _, doom := range []string{"", "independent-doom-association"} {
				envelope := validEnvelope(validSnapshot())
				envelope.Metadata.PlaythroughID = identity
				envelope.Metadata.UserDescription = note
				envelope.Metadata.DoomRunID = doom
				envelope.Metadata.Description = "System/developer text remains independent.\n" + strings.Repeat("x", 100)
				data, err := EncodeCanonical(envelope)
				require.NoError(t, err)
				require.NoError(t, compiled.Validate(decodeJSONSchemaInstance(t, data)))
				decoded, err := Decode(data)
				require.NoError(t, err)
				assert.Equal(t, envelope, decoded, "metadata must not be trimmed, merged, or backfilled")
				reencoded, err := EncodeCanonical(decoded)
				require.NoError(t, err)
				assert.Equal(t, data, reencoded)
				for member, value := range map[string]string{"playthroughId": identity, "userDescription": note, "doomRunId": doom} {
					assert.Equal(t, value != "", bytes.Contains(data, []byte(`"`+member+`":`)), member)
				}
			}
		}
	}
}

func TestPlaythroughIdentityValidationMatchesCodecAndSchema(t *testing.T) {
	t.Parallel()
	compiled, _ := compileCurrentNativeSaveSchema(t)
	for _, test := range []struct {
		value string
		valid bool
	}{
		{value: "", valid: false},
		{value: testPlaythroughID, valid: true},
		{value: strings.Repeat("0", 32), valid: true},
		{value: strings.Repeat("f", 32), valid: true},
		{value: strings.Repeat("a", 31)},
		{value: strings.Repeat("a", 33)},
		{value: strings.ToUpper(testPlaythroughID)},
		{value: strings.Repeat("g", 32)},
		{value: " " + testPlaythroughID},
		{value: testPlaythroughID + "\n"},
		{value: "\n"},
		{value: strings.Repeat("a", 31) + "\n"},
		{value: strings.Repeat("\uff10", 32)},
		{value: "01234567-89ab-cdef-0123-456789abcdef"},
	} {
		t.Run(fmt.Sprintf("%q", test.value), func(t *testing.T) {
			assert.Equal(t, test.valid, ValidPlaythroughID(test.value))
			envelope := validEnvelope(validSnapshot())
			envelope.Metadata.PlaythroughID = test.value
			_, encodeErr := EncodeCanonical(envelope)
			instance := currentEnvelopeDocument(t, validSnapshot())
			instance["metadata"].(map[string]any)["playthroughId"] = test.value
			data, err := json.Marshal(instance)
			require.NoError(t, err)
			_, decodeErr := Decode(data)
			schemaErr := compiled.Validate(decodeJSONSchemaInstance(t, data))
			if test.valid || test.value == "" {
				assert.NoError(t, encodeErr)
				assert.NoError(t, decodeErr)
				assert.NoError(t, schemaErr)
			} else {
				assert.ErrorIs(t, encodeErr, ErrInvalidEnvelope)
				assert.ErrorIs(t, decodeErr, ErrInvalidEnvelope)
				assert.Error(t, schemaErr)
			}
		})
	}
}

func TestUserDescriptionValidationMatchesCodecAndSchema(t *testing.T) {
	t.Parallel()
	compiled, _ := compileCurrentNativeSaveSchema(t)
	for _, note := range []string{"", "  Case, punctuation & interior  spaces!  ", "\u00a0padded\u00a0", strings.Repeat("x", 80), strings.Repeat("\U0001f409", 80)} {
		assert.NoError(t, ValidateUserDescription(note))
	}
	invalid := []string{strings.Repeat("x", 81), strings.Repeat("\u754c", 81), strings.Repeat("\U0001f409", 81), "first\nsecond", "\u2028", "\u2029"}
	for character := rune(0); character <= 0x9f; character++ {
		if character < 0x20 || character >= 0x7f {
			invalid = append(invalid, "before"+string(character)+"after")
		}
	}
	for _, note := range invalid {
		t.Run(fmt.Sprintf("%q", note), func(t *testing.T) {
			assert.Error(t, ValidateUserDescription(note))
			envelope := validEnvelope(validSnapshot())
			envelope.Metadata.UserDescription = note
			_, err := EncodeCanonical(envelope)
			assert.ErrorIs(t, err, ErrInvalidEnvelope)
			data, err := json.Marshal(envelope)
			require.NoError(t, err)
			_, err = Decode(data)
			assert.ErrorIs(t, err, ErrInvalidEnvelope)
			assert.Error(t, compiled.Validate(decodeJSONSchemaInstance(t, data)))
		})
	}
	for _, invalidUTF8 := range []string{"\xff", "\xc3", "\xc0\x80", "\xed\xa0\x80"} {
		assert.Error(t, ValidateUserDescription(invalidUTF8))
		envelope := validEnvelope(validSnapshot())
		envelope.Metadata.UserDescription = invalidUTF8
		_, err := EncodeCanonical(envelope)
		assert.ErrorIs(t, err, ErrInvalidEnvelope)
		envelope.Metadata.UserDescription = "placeholder"
		data, err := EncodeCanonical(envelope)
		require.NoError(t, err)
		data = bytes.Replace(data, []byte("placeholder"), []byte(invalidUTF8), 1)
		_, err = Decode(data)
		assert.ErrorIs(t, err, ErrInvalidEnvelope, "invalid UTF-8 must not be replaced during JSON decoding")
	}
}

func TestEnvelopeTwoStrictMetadataFields(t *testing.T) {
	t.Parallel()
	compiled, _ := compileCurrentNativeSaveSchema(t)
	for _, member := range []string{"playthroughId", "userDescription", "doomRunId", "description"} {
		for _, value := range []any{nil, true, 32, []any{}, map[string]any{}} {
			instance := currentEnvelopeDocument(t, validSnapshot())
			instance["metadata"].(map[string]any)[member] = value
			data, err := json.Marshal(instance)
			require.NoError(t, err)
			_, err = Decode(data)
			assert.ErrorIs(t, err, ErrInvalidEnvelope, "%s=%#v", member, value)
			assert.Error(t, compiled.Validate(decodeJSONSchemaInstance(t, data)), "%s=%#v", member, value)
		}
	}
	for _, member := range []string{"unknown", "PlaythroughId", "UserDescription", "DoomRunId"} {
		instance := currentEnvelopeDocument(t, validSnapshot())
		instance["metadata"].(map[string]any)[member] = ""
		data, err := json.Marshal(instance)
		require.NoError(t, err)
		_, err = Decode(data)
		assert.ErrorIs(t, err, ErrInvalidEnvelope, member)
		assert.Error(t, compiled.Validate(decodeJSONSchemaInstance(t, data)), member)
	}
	envelope := validEnvelope(validSnapshot())
	envelope.Metadata.PlaythroughID = testPlaythroughID
	envelope.Metadata.UserDescription = "Before Doom"
	envelope.Metadata.DoomRunID = "doom"
	data, err := EncodeCanonical(envelope)
	require.NoError(t, err)
	for _, member := range []string{"playthroughId", "userDescription", "doomRunId", "description"} {
		key := []byte(`"` + member + `":`)
		duplicate := bytes.Replace(data, key, append([]byte(`"`+member+`":"",`), key...), 1)
		_, err = Decode(duplicate)
		assert.ErrorIs(t, err, ErrInvalidEnvelope, member)
	}
	_, err = Decode(append(data, []byte(` {}`)...))
	assert.ErrorIs(t, err, ErrInvalidEnvelope)
}

func TestEnvelopeTwoRejectsUnpairedSurrogatesWithoutReplacingValidUnicode(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		raw  string
		want string
		bad  bool
	}{
		{name: "isolated high", raw: `"Before \ud800 Doom"`, bad: true},
		{name: "isolated low", raw: `"Before \udfff Doom"`, bad: true},
		{name: "high at end", raw: `"Before \udbff"`, bad: true},
		{name: "two high", raw: `"\ud800\udbff"`, bad: true},
		{name: "two low", raw: `"\udc00\udfff"`, bad: true},
		{name: "reversed pair", raw: `"\udc00\ud800"`, bad: true},
		{name: "separated pair", raw: `"\ud800 \udc00"`, bad: true},
		{name: "non-surrogate second", raw: `"\ud800\u0041"`, bad: true},
		{name: "escaped second", raw: `"\ud800\\udc00"`, bad: true},
		{name: "replacement second", raw: `"\ud800\ufffd"`, bad: true},
		{name: "valid pair then high", raw: `"\ud83d\udc09\ud800"`, bad: true},
		{name: "valid pair then low", raw: `"\ud83d\udc09\udc00"`, bad: true},
		{name: "emoji pair", raw: `"Before \ud83d\udc09 Doom"`, want: "Before \U0001f409 Doom"},
		{name: "uppercase hex", raw: `"\uD83D\uDC09"`, want: "\U0001f409"},
		{name: "adjacent pairs", raw: `"\ud800\udc00\udbff\udfff"`, want: "\U00010000\U0010ffff"},
		{name: "escaped backslashes", raw: `"Before \\ud800 \\udc00 Doom"`, want: `Before \ud800 \udc00 Doom`},
		{name: "escaped backslash before pair", raw: `"\\\ud83d\udc09"`, want: "\\\U0001f409"},
		{name: "literal replacement", raw: "\"Before \ufffd Doom\"", want: "Before \ufffd Doom"},
		{name: "escaped replacement", raw: `"Before \ufffd Doom"`, want: "Before \ufffd Doom"},
		{name: "adjacent BMP", raw: `"\ud7ff\ue000"`, want: "\ud7ff\ue000"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, version := range []int{CurrentVersion, 99} {
				envelope := validEnvelope(validSnapshot())
				envelope.Metadata.UserDescription = "placeholder"
				envelope.Snapshot.Version = version
				data, err := json.Marshal(envelope)
				require.NoError(t, err)
				data = bytes.Replace(data, []byte(`"placeholder"`), []byte(test.raw), 1)
				decode := Decode
				if version == 99 {
					decode = DecodeUnsupportedVersion
				}
				decoded, err := decode(data)
				if test.bad {
					require.ErrorIs(t, err, ErrInvalidEnvelope)
					assert.ErrorContains(t, err, "unpaired JSON UTF-16 surrogate escape")
					assert.Equal(t, Envelope{}, decoded, "malformed records cannot acquire canonical revisions")
					continue
				}
				require.NoError(t, err)
				assert.Equal(t, test.want, decoded.Metadata.UserDescription)
				if version == CurrentVersion {
					canonical, encodeErr := EncodeCanonical(decoded)
					require.NoError(t, encodeErr)
					reloaded, decodeErr := Decode(canonical)
					require.NoError(t, decodeErr)
					assert.Equal(t, test.want, reloaded.Metadata.UserDescription)
				}
			}
		})
	}
}

func TestEnvelopeTwoRejectsOtherEnvelopeVersionsWithoutSnapshotMigration(t *testing.T) {
	t.Parallel()
	compiled, _ := compileCurrentNativeSaveSchema(t)
	for _, version := range []int{0, 1, 3} {
		envelope := validEnvelope(validSnapshot())
		envelope.EnvelopeVersion = version
		_, err := EncodeCanonical(envelope)
		assert.ErrorIs(t, err, ErrInvalidEnvelope)
		data, err := json.Marshal(envelope)
		require.NoError(t, err)
		_, err = Decode(data)
		assert.ErrorIs(t, err, ErrInvalidEnvelope)
		assert.ErrorContains(t, err, "unsupported envelope version")
		_, err = DecodeUnsupportedVersion(data)
		assert.ErrorIs(t, err, ErrInvalidEnvelope)
		assert.Error(t, compiled.Validate(decodeJSONSchemaInstance(t, data)))
	}
}

func TestGenericRepositoryPreservesEnvelopeTwoMetadata(t *testing.T) {
	t.Parallel()
	repository, roots := newTestRepository(t)
	for _, identity := range []string{"", testPlaythroughID} {
		envelope := validEnvelope(validSnapshot())
		envelope.Metadata.PlaythroughID = identity
		envelope.Metadata.UserDescription = "  Independent player note  "
		envelope.Metadata.DoomRunID = "independent-doom-run"
		reference := "project:capture"
		if identity == "" {
			reference = "rollback:generic"
		}
		record, err := repository.Create(reference, envelope)
		require.NoError(t, err)
		loaded, err := repository.Load(reference)
		require.NoError(t, err)
		assert.Equal(t, envelope, loaded.Envelope)
		assert.Equal(t, record.Revision, loaded.Revision)
	}
	summaries, err := repository.List()
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	assert.Equal(t, testPlaythroughID, summaries[0].Metadata.PlaythroughID)
	assert.Empty(t, summaries[1].Metadata.PlaythroughID)
	assert.Equal(t, summaries[0].Metadata.UserDescription, summaries[1].Metadata.UserDescription)

	old := validEnvelope(validSnapshot())
	old.EnvelopeVersion = 1
	data, err := json.Marshal(old)
	require.NoError(t, err)
	path := filepath.Join(roots[0], "old.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	_, err = repository.Load("project:old")
	require.ErrorIs(t, err, ErrInvalidEnvelope)
	assert.ErrorContains(t, err, "unsupported envelope version 1")
	_, err = repository.List()
	require.ErrorIs(t, err, ErrInvalidEnvelope, "old developer captures also remain unsupported")
	retained, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, retained)
}
