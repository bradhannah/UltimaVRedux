package save_snapshot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bradhannah/UltimaVRedux/internal/repositoryfs"
)

const (
	// EnvelopeFormat identifies native Ultima 5 Redux snapshot JSON.
	EnvelopeFormat = "ultima5redux-native-snapshot"
	// CurrentEnvelopeVersion is the only supported envelope version.
	CurrentEnvelopeVersion = 2
	// MaxUserDescriptionRunes bounds the optional single-line player checkpoint note.
	MaxUserDescriptionRunes = 80
)

var (
	// ErrInvalidEnvelope indicates malformed, unsupported, or non-canonicalizable data.
	ErrInvalidEnvelope = errors.New("invalid save snapshot envelope")
	// ErrInvalidSnapshot indicates semantic snapshot validation failures.
	ErrInvalidSnapshot = errors.New("invalid save snapshot")
	// ErrUnsupportedSnapshotVersion indicates a well-formed native record whose
	// snapshot schema is deliberately incompatible with this build.
	ErrUnsupportedSnapshotVersion = errors.New("unsupported save snapshot version")
)

// UnsupportedVersionError identifies the rejected native snapshot schema.
type UnsupportedVersionError struct {
	Version int
}

// Error implements error.
func (unsupported *UnsupportedVersionError) Error() string {
	return fmt.Sprintf("%v: got %d, supported versions are %d through %d", ErrUnsupportedSnapshotVersion, unsupported.Version, MinimumVersion, CurrentVersion)
}

// Unwrap supports errors.Is(err, ErrUnsupportedSnapshotVersion).
func (unsupported *UnsupportedVersionError) Unwrap() error {
	return ErrUnsupportedSnapshotVersion
}

// Metadata describes a capture without affecting gameplay state.
type Metadata struct {
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	CreatedAt       time.Time `json:"createdAt"`
	GameVersion     string    `json:"gameVersion"`
	DoomRunID       string    `json:"doomRunId,omitempty"`
	PlaythroughID   string    `json:"playthroughId,omitempty"`
	UserDescription string    `json:"userDescription,omitempty"`
}

// Envelope is the strict native JSON transport for one snapshot.
type Envelope struct {
	Format          string       `json:"format"`
	EnvelopeVersion int          `json:"envelopeVersion"`
	Metadata        Metadata     `json:"metadata"`
	Snapshot        SaveSnapshot `json:"snapshot"`
	legacyExcerpts  bool
}

// NeedsExcerptMigration reports decode-only old-format provenance. It cannot
// be requested by JSON or inferred from an empty current collection.
func (envelope Envelope) NeedsExcerptMigration() bool { return envelope.legacyExcerpts }

// UnmarshalJSON enforces required fields on the strict current snapshot object.
func (snapshot *SaveSnapshot) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, exists := fields["redux"]; !exists {
		return errors.New("snapshot redux is required")
	}
	continuation, exists := fields["continuation"]
	if !exists || bytes.Equal(bytes.TrimSpace(continuation), []byte("null")) {
		return errors.New("snapshot continuation is required")
	}
	presentation, exists := fields["presentation"]
	if !exists || bytes.Equal(bytes.TrimSpace(presentation), []byte("null")) {
		return errors.New("snapshot presentation is required")
	}
	conversation, exists := fields["lastConversation"]
	if !exists || bytes.Equal(bytes.TrimSpace(conversation), []byte("null")) {
		return errors.New("snapshot lastConversation is required")
	}
	if err := validateConversationJSONMembers(conversation); err != nil {
		return err
	}
	if _, exists := fields["dungeonJournal"]; !exists {
		return errors.New("snapshot dungeonJournal is required")
	}
	if _, exists := fields["atlasLandmarks"]; !exists {
		return errors.New("snapshot atlasLandmarks is required")
	}
	if _, exists := fields["openDoor"]; !exists {
		return errors.New("snapshot openDoor is required")
	}

	type decodedSnapshot SaveSnapshot
	var decoded decodedSnapshot
	if err := decodeRequiredJSON(data, &decoded); err != nil {
		return fmt.Errorf("decode save snapshot: %w", err)
	}

	*snapshot = SaveSnapshot(decoded)

	return nil
}

// UnmarshalJSON enforces the required fields in the strict Redux extension.
func (snapshot *ReduxSnapshot) UnmarshalJSON(data []byte) error {
	var encoded struct {
		DungeonDiscoveries     *[]DungeonDiscoverySnapshot  `json:"dungeonDiscoveries"`
		PlayerKnowledge        *[]PlayerKnowledgeSnapshot   `json:"playerKnowledge"`
		QuestJournal           *[]QuestJournalEventSnapshot `json:"questJournal"`
		LordBritishPassageOpen *bool                        `json:"lordBritishPassageOpen"`
	}
	if err := repositoryfs.DecodeStrictJSON(data, &encoded); err != nil {
		return fmt.Errorf("decode redux snapshot: %w", err)
	}
	if encoded.DungeonDiscoveries == nil {
		return errors.New("redux dungeonDiscoveries is required")
	}
	if encoded.PlayerKnowledge == nil {
		return errors.New("redux playerKnowledge is required")
	}
	if encoded.QuestJournal == nil {
		return errors.New("redux questJournal is required")
	}
	if encoded.LordBritishPassageOpen == nil {
		return errors.New("redux lordBritishPassageOpen is required")
	}
	snapshot.DungeonDiscoveries = *encoded.DungeonDiscoveries
	snapshot.PlayerKnowledge = *encoded.PlayerKnowledge
	snapshot.QuestJournal = *encoded.QuestJournal
	snapshot.LordBritishPassageOpen = *encoded.LordBritishPassageOpen

	return nil
}

// ValidationError exposes every semantic snapshot issue.
type ValidationError struct {
	Issues []Issue
}

// Error implements error.
func (validationError *ValidationError) Error() string {
	if len(validationError.Issues) > 0 {
		first := validationError.Issues[0]

		return fmt.Sprintf("%v: %d issue(s); first: %s [%s] %s", ErrInvalidSnapshot, len(validationError.Issues), first.Path, first.Code, first.Message)
	}

	return fmt.Sprintf("%v: %d issue(s)", ErrInvalidSnapshot, len(validationError.Issues))
}

// Unwrap supports errors.Is(err, ErrInvalidSnapshot).
func (validationError *ValidationError) Unwrap() error {
	return ErrInvalidSnapshot
}

// EncodeCanonical validates and deterministically encodes an envelope.
func EncodeCanonical(envelope Envelope) ([]byte, error) {
	if envelope.Snapshot.NeedsMapUnitReconstruction() {
		return nil, fmt.Errorf("%w: transient map-unit reconstruction must be consumed before encoding", ErrInvalidEnvelope)
	}
	if err := validateEnvelope(envelope); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("%w: encode JSON: %w", ErrInvalidEnvelope, err)
	}

	return append(data, '\n'), nil
}

// Decode strictly decodes and validates one native envelope.
func Decode(data []byte) (Envelope, error) {
	encoded, snapshotFields, version, err := decodeEnvelopeTransport(data, "decode JSON")
	if err != nil {
		return Envelope{}, err
	}
	if version < MinimumVersion || version > CurrentVersion {
		return Envelope{}, &UnsupportedVersionError{Version: version}
	}
	if err = validateSnapshotJSONMembers(snapshotFields, "dungeonDiscoveries", "playerKnowledge", "questJournal"); err != nil {
		return Envelope{}, err
	}
	if err = validateSurfaceChestJSONMembers(snapshotFields); err != nil {
		return Envelope{}, err
	}
	if err = validateConversationJSONMembers(snapshotFields["lastConversation"]); err != nil {
		return Envelope{}, err
	}

	var snapshot SaveSnapshot
	if version == CurrentVersion {
		if err = decodeRequiredJSON(encoded.Snapshot, &snapshot); err != nil {
			return Envelope{}, fmt.Errorf("%w: decode JSON: %w", ErrInvalidEnvelope, err)
		}
	} else {
		if snapshot, err = decodePreviousSnapshot(encoded.Snapshot, version); err != nil {
			return Envelope{}, err
		}
		if issues := validateSnapshot(snapshot, version); len(issues) != 0 {
			return Envelope{}, &ValidationError{Issues: issues}
		}
		snapshot.Version = CurrentVersion
	}

	envelope := Envelope{
		Format:          encoded.Format,
		EnvelopeVersion: encoded.EnvelopeVersion,
		Metadata:        encoded.Metadata,
		Snapshot:        snapshot,
		legacyExcerpts:  version < 25,
	}
	if err = validateEnvelope(envelope); err != nil {
		return Envelope{}, err
	}

	return envelope, nil
}

// decodePreviousSnapshot decodes a frozen older format into the current shape.
// Members the older format lacked keep their zero value.
func decodePreviousSnapshot(data json.RawMessage, version int) (SaveSnapshot, error) {
	switch version {
	case 24:
		var previous saveSnapshotV24
		if err := decodeRequiredJSON(data, &previous); err != nil {
			return SaveSnapshot{}, fmt.Errorf("%w: decode version %d snapshot: %w", ErrInvalidEnvelope, version, err)
		}

		return previous.validationView(), nil
	case 25:
		var previous saveSnapshotV25
		if err := decodeRequiredJSON(data, &previous); err != nil {
			return SaveSnapshot{}, fmt.Errorf("%w: decode version %d snapshot: %w", ErrInvalidEnvelope, version, err)
		}

		return previous.validationView(), nil
	default:
		return SaveSnapshot{}, &UnsupportedVersionError{Version: version}
	}
}

func validateSnapshotJSONMembers(snapshotFields map[string]json.RawMessage, reduxArrayMembers ...string) error {
	if _, present := snapshotFields["dungeon"]; !present {
		return fmt.Errorf("%w: snapshot dungeon member is required", ErrInvalidEnvelope)
	}
	var reduxFields map[string]json.RawMessage
	if err := json.Unmarshal(snapshotFields["redux"], &reduxFields); err != nil || reduxFields == nil {
		return fmt.Errorf("%w: snapshot redux member must be an object", ErrInvalidEnvelope)
	}
	for _, member := range reduxArrayMembers {
		value, present := reduxFields[member]
		if !present || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("%w: snapshot redux.%s member must be a non-null array", ErrInvalidEnvelope, member)
		}
	}

	return nil
}

func validateSurfaceChestJSONMembers(snapshotFields map[string]json.RawMessage) error {
	var units []map[string]json.RawMessage
	if err := json.Unmarshal(snapshotFields["mapUnits"], &units); err != nil {
		return fmt.Errorf("%w: snapshot mapUnits member must be an array: %w", ErrInvalidEnvelope, err)
	}
	for index, unit := range units {
		value, present := unit["surfaceChestNumber"]
		if !present {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("%w: snapshot mapUnits[%d].surfaceChestNumber must be a non-null integer", ErrInvalidEnvelope, index)
		}
		var number int
		if err := json.Unmarshal(value, &number); err != nil {
			return fmt.Errorf("%w: snapshot mapUnits[%d].surfaceChestNumber must be an integer: %w", ErrInvalidEnvelope, index, err)
		}
	}

	return nil
}

// DecodeUnsupportedVersion validates the stable envelope portions of one
// record whose snapshot schema is unsupported. The snapshot body is otherwise
// opaque so future schema fields do not make a valid record appear corrupt.
func DecodeUnsupportedVersion(data []byte) (Envelope, error) {
	encoded, _, version, err := decodeEnvelopeTransport(data, "decode unsupported envelope")
	if err != nil {
		return Envelope{}, err
	}
	if version >= MinimumVersion && version <= CurrentVersion {
		return Envelope{}, fmt.Errorf("%w: snapshot version %d is supported", ErrInvalidEnvelope, version)
	}

	return Envelope{
		Format:          encoded.Format,
		EnvelopeVersion: encoded.EnvelopeVersion,
		Metadata:        encoded.Metadata,
		Snapshot:        SaveSnapshot{Version: version},
	}, nil
}

type encodedEnvelope struct {
	Format          string          `json:"format"`
	EnvelopeVersion int             `json:"envelopeVersion"`
	Metadata        Metadata        `json:"metadata"`
	Snapshot        json.RawMessage `json:"snapshot"`
}

func decodeEnvelopeTransport(data []byte, operation string) (encodedEnvelope, map[string]json.RawMessage, int, error) {
	if !utf8.Valid(data) {
		return encodedEnvelope{}, nil, 0, fmt.Errorf("%w: JSON must be valid UTF-8", ErrInvalidEnvelope)
	}
	if err := validateUniqueJSONMembers(data); err != nil {
		return encodedEnvelope{}, nil, 0, fmt.Errorf("%w: %s: %w", ErrInvalidEnvelope, operation, err)
	}
	var encoded encodedEnvelope
	if err := decodeRequiredJSON(data, &encoded); err != nil {
		return encodedEnvelope{}, nil, 0, fmt.Errorf("%w: %s: %w", ErrInvalidEnvelope, operation, err)
	}
	if encoded.Format != EnvelopeFormat {
		return encodedEnvelope{}, nil, 0, fmt.Errorf("%w: unsupported format %q", ErrInvalidEnvelope, encoded.Format)
	}
	if encoded.EnvelopeVersion != CurrentEnvelopeVersion {
		return encodedEnvelope{}, nil, 0, fmt.Errorf("%w: unsupported envelope version %d", ErrInvalidEnvelope, encoded.EnvelopeVersion)
	}
	if err := validateMetadata(encoded.Metadata); err != nil {
		return encodedEnvelope{}, nil, 0, err
	}

	var snapshotFields map[string]json.RawMessage
	if err := json.Unmarshal(encoded.Snapshot, &snapshotFields); err != nil || snapshotFields == nil {
		return encodedEnvelope{}, nil, 0, fmt.Errorf("%w: snapshot must be an object", ErrInvalidEnvelope)
	}
	versionData, ok := snapshotFields["version"]
	if !ok {
		return encodedEnvelope{}, nil, 0, fmt.Errorf("%w: snapshot version is required", ErrInvalidEnvelope)
	}
	var version *int
	if err := json.Unmarshal(versionData, &version); err != nil {
		return encodedEnvelope{}, nil, 0, fmt.Errorf("%w: snapshot version must be an integer: %w", ErrInvalidEnvelope, err)
	}
	if version == nil {
		return encodedEnvelope{}, nil, 0, fmt.Errorf("%w: snapshot version must be an integer", ErrInvalidEnvelope)
	}

	return encoded, snapshotFields, *version, nil
}

func decodeRequiredJSON(data []byte, target any) error {
	if err := repositoryfs.DecodeStrictJSON(data, target); err != nil {
		return err
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	if err := validateRequiredJSONMembers(document, reflect.TypeOf(target), "$"); err != nil {
		return err
	}

	return nil
}

func validateRequiredJSONMembers(document any, targetType reflect.Type, path string) error {
	if document == nil && targetType.Kind() != reflect.Pointer && targetType.Kind() != reflect.Slice {
		return fmt.Errorf("required member %s cannot be null", path)
	}
	for targetType.Kind() == reflect.Pointer {
		if document == nil {
			return nil
		}
		targetType = targetType.Elem()
	}

	//nolint:exhaustive // Only JSON containers can contain additional required members.
	switch targetType.Kind() {
	case reflect.Struct:
		object, ok := document.(map[string]any)
		if !ok {
			return nil
		}
		knownMembers := make(map[string]struct{}, targetType.NumField())
		for index := range targetType.NumField() {
			field := targetType.Field(index)
			if !field.IsExported() {
				continue
			}
			name := field.Name
			optional := false
			if tag, ok := field.Tag.Lookup("json"); ok {
				var options string
				name, options, _ = strings.Cut(tag, ",")
				optional = strings.Contains(","+options+",", ",omitempty,")
				if name == "-" {
					continue
				}
				if name == "" {
					name = field.Name
				}
			}
			knownMembers[name] = struct{}{}
			value, present := object[name]
			fieldPath := path + "." + name
			if !present {
				if optional {
					continue
				}

				return fmt.Errorf("required member %s is missing", fieldPath)
			}
			if err := validateRequiredJSONMembers(value, field.Type, fieldPath); err != nil {
				return err
			}
		}
		// encoding/json also accepts case-insensitive aliases, which the wire schema does not.
		for name := range object {
			if _, known := knownMembers[name]; !known {
				return fmt.Errorf("unknown member %s.%s", path, name)
			}
		}
	case reflect.Array:
		array, ok := document.([]any)
		if !ok {
			return nil
		}
		if len(array) != targetType.Len() {
			return fmt.Errorf("required array %s must contain exactly %d items", path, targetType.Len())
		}
		for index, value := range array {
			if err := validateRequiredJSONMembers(value, targetType.Elem(), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case reflect.Slice:
		array, ok := document.([]any)
		if !ok {
			return nil
		}
		for index, value := range array {
			if err := validateRequiredJSONMembers(value, targetType.Elem(), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	default:
		return nil
	}

	return nil
}

func validateUniqueJSONMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}

	if err = validateUniqueJSONValue(decoder, token); err != nil {
		return err
	}

	return validateJSONSurrogatePairs(data)
}

// Check raw escapes before constructing the envelope: encoding/json silently
// replaces unpaired surrogates with U+FFFD. Other JSON syntax is checked above.
func validateJSONSurrogatePairs(data []byte) error {
	for index := 0; index < len(data); index++ {
		if data[index] != '\\' {
			continue
		}
		index++
		if index >= len(data) || data[index] != 'u' {
			continue
		}
		if index+4 >= len(data) {
			return errors.New("truncated JSON Unicode escape")
		}
		value, err := strconv.ParseUint(string(data[index+1:index+5]), 16, 16)
		if err != nil {
			return fmt.Errorf("invalid JSON Unicode escape: %w", err)
		}
		index += 4
		if value < 0xd800 || value > 0xdfff {
			continue
		}
		if value > 0xdbff || index+6 >= len(data) || data[index+1] != '\\' || data[index+2] != 'u' {
			return errors.New("unpaired JSON UTF-16 surrogate escape")
		}
		low, err := strconv.ParseUint(string(data[index+3:index+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return errors.New("unpaired JSON UTF-16 surrogate escape")
		}
		index += 6
	}

	return nil
}

func validateUniqueJSONValue(decoder *json.Decoder, token json.Token) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delimiter {
	case '{':
		members := make(map[string]struct{})
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := nameToken.(string)
			if !ok {
				return errors.New("object member name must be a string")
			}
			if _, duplicate := members[name]; duplicate {
				return fmt.Errorf("duplicate object member %q", name)
			}
			members[name] = struct{}{}

			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := validateUniqueJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := validateUniqueJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}

	_, err := decoder.Token()

	return err
}

func validateEnvelope(envelope Envelope) error {
	if envelope.Format != EnvelopeFormat {
		return fmt.Errorf("%w: unsupported format %q", ErrInvalidEnvelope, envelope.Format)
	}
	if envelope.EnvelopeVersion != CurrentEnvelopeVersion {
		return fmt.Errorf("%w: unsupported envelope version %d", ErrInvalidEnvelope, envelope.EnvelopeVersion)
	}
	if err := validateMetadata(envelope.Metadata); err != nil {
		return err
	}
	if issues := Validate(envelope.Snapshot); len(issues) != 0 {
		return &ValidationError{Issues: issues}
	}

	return nil
}

func validateMetadata(metadata Metadata) error {
	if strings.TrimSpace(metadata.Name) == "" {
		return fmt.Errorf("%w: metadata name is required", ErrInvalidEnvelope)
	}
	if metadata.CreatedAt.IsZero() {
		return fmt.Errorf("%w: metadata creation time is required", ErrInvalidEnvelope)
	}
	if metadata.CreatedAt.Location() != time.UTC {
		return fmt.Errorf("%w: metadata creation time must be UTC", ErrInvalidEnvelope)
	}
	if metadata.PlaythroughID != "" && !ValidPlaythroughID(metadata.PlaythroughID) {
		return fmt.Errorf("%w: metadata playthroughId must contain 32 lowercase hexadecimal characters", ErrInvalidEnvelope)
	}
	if err := ValidateUserDescription(metadata.UserDescription); err != nil {
		return fmt.Errorf("%w: metadata userDescription: %w", ErrInvalidEnvelope, err)
	}

	return nil
}

// ValidPlaythroughID reports whether an identity is exactly 32 lowercase hex characters.
// Empty identities are permitted in generic metadata, but are not valid player identities.
func ValidPlaythroughID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}

	return true
}

// ValidateUserDescription validates an optional player note without trimming or coercing it.
func ValidateUserDescription(value string) error {
	if !utf8.ValidString(value) {
		return errors.New("must be valid UTF-8")
	}
	if utf8.RuneCountInString(value) > MaxUserDescriptionRunes {
		return fmt.Errorf("must contain at most %d Unicode runes", MaxUserDescriptionRunes)
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return errors.New("must be a single line without control characters")
		}
	}

	return nil
}
