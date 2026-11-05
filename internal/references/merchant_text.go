package references

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var (
	errUnknownMerchantText            = errors.New("unknown merchant text ID")
	errInvalidMerchantTextValue       = errors.New("invalid merchant text value")
	errMerchantWordDictionaryRequired = errors.New("compressed-word dictionary is required")
	errCorruptMerchantText            = errors.New("corrupt SHOPPE.DAT")
)

// MerchantTextID is the semantic SHOPPE.TXT label for one SHOPPE.DAT record.
type MerchantTextID string

// MerchantTimeOfDay is the original @ placeholder value.
type MerchantTimeOfDay string

// Merchant time constants are the only valid original @ substitutions.
const (
	MerchantMorning   MerchantTimeOfDay = "morning"
	MerchantAfternoon MerchantTimeOfDay = "afternoon"
	MerchantEvening   MerchantTimeOfDay = "evening"
)

// MerchantTextValues supplies the typed values understood by SHOPPE templates.
// An ampersand means ItemName except in SENS records, where it means InformantName.
type MerchantTextValues struct {
	OwnerName     string
	ShopName      string
	ItemName      string
	Price         int
	Quantity      int
	InformantName string
	LocationName  string
	TimeOfDay     MerchantTimeOfDay
}

// NewMerchantReferences loads required SHOPPE.DAT prose and indexes the active
// merchant table independently of the semantic NPC conversation catalog.
func NewMerchantReferences(shoppePath string, wordDict *WordDict) (*MerchantReferences, error) {
	data, err := os.ReadFile(shoppePath)
	if err != nil {
		return nil, fmt.Errorf("reading required SHOPPE.DAT: %w", err)
	}

	return newMerchantReferences(data, wordDict, originalMerchantDefinitions)
}

func newMerchantReferences(
	data []byte,
	wordDict *WordDict,
	definitions []MerchantDefinition,
) (*MerchantReferences, error) {
	texts, err := parseMerchantTexts(data, wordDict)
	if err != nil {
		return nil, fmt.Errorf("parsing SHOPPE.DAT: %w", err)
	}

	refs, err := newMerchantReferenceIndex(definitions, texts)
	if err != nil {
		return nil, fmt.Errorf("indexing merchant definitions: %w", err)
	}

	return refs, nil
}

// Text returns an expanded SHOPPE template with typed placeholders intact.
func (r *MerchantReferences) Text(id MerchantTextID) (string, bool) {
	text, ok := r.texts[id]

	return text, ok
}

// FormatText replaces the seven merchant-text placeholders with typed values.
func (r *MerchantReferences) FormatText(id MerchantTextID, values MerchantTextValues) (string, error) {
	template, ok := r.texts[id]
	if !ok {
		return "", fmt.Errorf("%w %q", errUnknownMerchantText, id)
	}

	var formatted strings.Builder
	formatted.Grow(len(template))

	for i := range len(template) {
		placeholder := template[i]
		var value string

		switch placeholder {
		case '#':
			value = values.ShopName
		case '$':
			value = values.OwnerName
		case '@':
			if values.TimeOfDay != MerchantMorning &&
				values.TimeOfDay != MerchantAfternoon &&
				values.TimeOfDay != MerchantEvening {
				return "", fmt.Errorf(
					"%w for merchant text %q: missing or invalid time of day",
					errInvalidMerchantTextValue, id,
				)
			}
			value = string(values.TimeOfDay)
		case '%':
			value = strconv.Itoa(values.Price)
		case '^':
			value = strconv.Itoa(values.Quantity)
		case '&':
			if strings.HasPrefix(string(id), "SENS_") {
				value = values.InformantName
			} else {
				value = values.ItemName
			}
		case '*':
			value = values.LocationName
		default:
			formatted.WriteByte(placeholder)

			continue
		}

		if value == "" {
			return "", fmt.Errorf(
				"%w for merchant text %q: no value for placeholder %q",
				errInvalidMerchantTextValue, id, placeholder,
			)
		}
		formatted.WriteString(value)
	}

	return formatted.String(), nil
}

func parseMerchantTexts(data []byte, wordDict *WordDict) (map[MerchantTextID]string, error) {
	if wordDict == nil {
		return nil, errMerchantWordDictionaryRequired
	}

	texts := make(map[MerchantTextID]string, len(merchantTextRecords))
	for index, record := range merchantTextRecords {
		if record.offset >= len(data) {
			return nil, fmt.Errorf(
				"%w: record %q offset %d is outside %d-byte file",
				errCorruptMerchantText,
				record.id, record.offset, len(data),
			)
		}

		relativeEnd := bytes.IndexByte(data[record.offset:], 0)
		if relativeEnd < 0 {
			return nil, fmt.Errorf("%w: record %q has no null terminator", errCorruptMerchantText, record.id)
		}
		end := record.offset + relativeEnd
		if end == record.offset {
			return nil, fmt.Errorf("%w: record %q is empty", errCorruptMerchantText, record.id)
		}

		expectedNext := len(data)
		if index+1 < len(merchantTextRecords) {
			expectedNext = merchantTextRecords[index+1].offset
		}
		validBoundary := end+1 == expectedNext
		// The shipped file has one extra null after the final generated record.
		if index == len(merchantTextRecords)-1 && end+2 == len(data) && data[end+1] == 0 {
			validBoundary = true
		}
		if !validBoundary {
			return nil, fmt.Errorf(
				"%w: record %q ends at %d; next record must start at %d",
				errCorruptMerchantText,
				record.id, end+1, expectedNext,
			)
		}

		expanded, err := wordDict.ReplaceMerchantString(string(data[record.offset:end]))
		if err != nil {
			return nil, fmt.Errorf("expanding record %q: %w", record.id, err)
		}
		texts[record.id] = expanded
	}

	return texts, nil
}

type merchantTextRecord struct {
	id     MerchantTextID
	offset int
}

// Merchant text records are addressed by exact SHOPPE.DAT byte offsets. Keeping only
// labels and offsets permits strict parsing without embedding copyrighted prose.
//
//nolint:gochecknoglobals // Semantic SHOPPE.DAT merchant-text offsets are immutable after initialization; ref:MERCH-0001.
var merchantTextRecords = [...]merchantTextRecord{
	{"RUDE_ARMS_0", 0},
	{"RUDE_ARMS_1", 17},
	{"RUDE_ARMS_2", 56},
	{"RUDE_ARMS_3", 67},
	{"POLITE_ARMS_0", 87},
	{"POLITE_ARMS_1", 100},
	{"POLITE_ARMS_2", 118},
	{"POLITE_ARMS_3", 131},
	{"ARMS_PITCH_0", 149},
	{"ARMS_PITCH_1", 208},
	{"ARMS_PITCH_2", 287},
	{"ARMS_PITCH_3", 338},
	{"ARMS_PITCH_4", 402},
	{"ARMS_PITCH_5", 459},
	{"ARMS_PITCH_6", 521},
	{"ARMS_PITCH_7", 589},
	{"ARMS_PITCH_9", 690},
	{"ARMS_PITCH_10", 778},
	{"ARMS_PITCH_11", 834},
	{"ARMS_PITCH_12", 886},
	{"ARMS_PITCH_13", 966},
	{"ARMS_PITCH_14", 1040},
	{"ARMS_PITCH_16", 1125},
	{"ARMS_PITCH_17", 1185},
	{"ARMS_PITCH_18", 1246},
	{"ARMS_PITCH_19", 1313},
	{"ARMS_PITCH_20", 1376},
	{"ARMS_PITCH_21", 1451},
	{"ARMS_PITCH_22", 1526},
	{"ARMS_PITCH_23", 1592},
	{"ARMS_PITCH_24", 1646},
	{"ARMS_PITCH_25", 1712},
	{"ARMS_PITCH_26", 1774},
	{"ARMS_PITCH_27", 1825},
	{"ARMS_PITCH_28", 1858},
	{"ARMS_PITCH_29", 1916},
	{"ARMS_PITCH_30", 1956},
	{"ARMS_PITCH_31", 2023},
	{"ARMS_PITCH_32", 2095},
	{"ARMS_PITCH_33", 2162},
	{"ARMS_PITCH_34", 2235},
	{"ARMS_PITCH_36", 2318},
	{"ARMS_PITCH_37", 2393},
	{"ARMS_PITCH_38", 2464},
	{"ARMS_PITCH_42", 2551},
	{"ARMS_PITCH_43", 2656},
	{"ARMS_PITCH_44", 2754},
	{"ARMS_PITCH_45", 2841},
	{"ARMS_PITCH_46", 2936},
	{"SCUZZ_0", 3008},
	{"SCUZZ_1", 3071},
	{"SCUZZ_2", 3118},
	{"SCUZZ_3", 3157},
	{"SCUZZ_4", 3233},
	{"SCUZZ_5", 3304},
	{"SCUZZ_6", 3346},
	{"SCUZZ_7", 3389},
	{"BAR_TXT_0", 3436},
	{"BAR_TXT_1", 3494},
	{"BAR_TXT_2", 3545},
	{"BAR_TXT_3", 3604},
	{"RUDE_BAR_0", 3652},
	{"RUDE_BAR_1", 3689},
	{"RUDE_BAR_2", 3717},
	{"RUDE_BAR_3", 3745},
	{"POLITE_BAR_0", 3772},
	{"POLITE_BAR_1", 3790},
	{"POLITE_BAR_2", 3811},
	{"POLITE_BAR_3", 3828},
	{"BAR_PITCH_0", 3849},
	{"BAR_PITCH_1", 3930},
	{"BAR_PITCH_2", 3998},
	{"BAR_PITCH_3", 4063},
	{"GAB_0", 4162},
	{"GAB_1", 4250},
	{"GAB_2", 4330},
	{"GAB_3", 4422},
	{"PRO_PITCH_0", 4504},
	{"PRO_PITCH_1", 4567},
	{"PRO_PITCH_2", 4639},
	{"PRO_PITCH_3", 4721},
	{"PRO_PITCH_4", 4761},
	{"PRO_PITCH_5", 4835},
	{"PRO_PITCH_6", 4876},
	{"AMNESIA", 4942},
	{"SENS_0", 5026},
	{"SENS_1", 5038},
	{"SENS_2", 5081},
	{"SENS_3", 5107},
	{"BAR_MISC_0", 5139},
	{"BAR_MISC_1", 5183},
	{"BAR_MISC_2", 5226},
	{"HORSE_TXT_0", 5265},
	{"HORSE_TXT_1", 5345},
	{"HORSE_TXT_2", 5395},
	{"HORSE_TXT_3", 5463},
	{"RUDE_HORSE_0", 5530},
	{"RUDE_HORSE_1", 5556},
	{"RUDE_HORSE_2", 5566},
	{"RUDE_HORSE_3", 5577},
	{"POLITE_HORSE_0", 5610},
	{"POLITE_HORSE_1", 5643},
	{"POLITE_HORSE_2", 5659},
	{"POLITE_HORSE_3", 5687},
	{"HORSE_PITCH", 5699},
	{"SHIP_TXT_0", 5783},
	{"SHIP_TXT_1", 5848},
	{"SHIP_TXT_2", 5904},
	{"SHIP_TXT_3", 5965},
	{"RUDE_SHIP_0", 6024},
	{"RUDE_SHIP_1", 6043},
	{"RUDE_SHIP_2", 6087},
	{"RUDE_SHIP_3", 6098},
	{"POLITE_SHIP_0", 6118},
	{"POLITE_SHIP_1", 6151},
	{"POLITE_SHIP_2", 6173},
	{"POLITE_SHIP_3", 6194},
	{"SHIP_PITCH_0", 6206},
	{"SHIP_PITCH_1", 6284},
	{"SHIP_MSG_0", 6379},
	{"SHIP_MSG_1", 6459},
	{"SHIP_MSG_2", 6495},
	{"SHIP_MSG_3", 6542},
	{"SHIP_MSG_4", 6571},
	{"SHIP_MSG_5", 6618},
	{"SHIP_MSG_6", 6642},
	{"SHIP_MSG_7", 6736},
	{"REAG_TXT_0", 6759},
	{"REAG_TXT_1", 6826},
	{"REAG_TXT_2", 6885},
	{"REAG_TXT_3", 6949},
	{"RUDE_REAG_0", 7009},
	{"RUDE_REAG_1", 7027},
	{"RUDE_REAG_2", 7056},
	{"RUDE_REAG_3", 7080},
	{"POLITE_REAG_0", 7103},
	{"POLITE_REAG_1", 7130},
	{"POLITE_REAG_2", 7157},
	{"POLITE_REAG_3", 7183},
	{"REAG_PITCH_0", 7196},
	{"REAG_PITCH_1", 7292},
	{"REAG_PITCH_2", 7386},
	{"REAG_PITCH_3", 7483},
	{"REAG_PITCH_4", 7565},
	{"REAG_PITCH_5", 7662},
	{"REAG_PITCH_6", 7758},
	{"REAG_PITCH_7", 7869},
	{"REAG_MISC_0", 7979},
	{"GUILD_TXT_0", 8034},
	{"GUILD_TXT_1", 8082},
	{"GUILD_TXT_2", 8145},
	{"GUILD_TXT_3", 8230},
	{"RUDE_GUILD_0", 8287},
	{"RUDE_GUILD_1", 8312},
	{"RUDE_GUILD_2", 8326},
	{"RUDE_GUILD_3", 8347},
	{"POLITE_GUILD_0", 8379},
	{"POLITE_GUILD_1", 8410},
	{"POLITE_GUILD_2", 8430},
	{"POLITE_GUILD_3", 8462},
	{"GUILD_PITCH_0", 8493},
	{"GUILD_PITCH_1", 8556},
	{"GUILD_PITCH_2", 8633},
	{"GUILD_MISC_0", 8678},
	{"GUILD_MISC_1", 8725},
	{"HEAL_TXT_0", 8764},
	{"HEAL_TXT_1", 8819},
	{"HEAL_TXT_2", 8875},
	{"HEAL_TXT_3", 8927},
	{"POLITE_HEAL_0", 8990},
	{"POLITE_HEAL_1", 9020},
	{"POLITE_HEAL_2", 9054},
	{"POLITE_HEAL_3", 9089},
	{"HEAL_MISC_0", 9131},
	{"INN_TXT_0", 9188},
	{"INN_TXT_1", 9242},
	{"INN_TXT_2", 9308},
	{"INN_TXT_3", 9365},
	{"RUDE_INN_0", 9458},
	{"RUDE_INN_1", 9484},
	{"RUDE_INN_2", 9504},
	{"RUDE_INN_3", 9529},
	{"POLITE_INN_0", 9577},
	{"POLITE_INN_1", 9604},
	{"POLITE_INN_2", 9634},
	{"POLITE_INN_3", 9662},
	{"INN_PITCH_0", 9680},
	{"INN_PITCH_1", 9728},
	{"INN_PITCH_2", 9793},
	{"INN_PITCH_4", 9831},
	{"INN_PITCH_5", 9895},
	{"INN_MISC_0", 9960},
	{"INN_MISC_1", 10019},
	{"INN_MISC_2", 10074},
}
