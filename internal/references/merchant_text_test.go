package references

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerchantTextLoaderExpandsWordsAndIndexesSemanticIDs(t *testing.T) {
	t.Parallel()

	data := syntheticShoppeData(t, map[MerchantTextID][]byte{
		"RUDE_ARMS_0": {'H', 'a', 'i', 'l', ' ', 0x80, '!'},
		"RUDE_ARMS_1": {0x81, ' ', 'f', 'r', 'i', 'e', 'n', 'd'},
	})
	shoppePath := filepath.Join(t.TempDir(), "SHOPPE.DAT")
	require.NoError(t, os.WriteFile(shoppePath, data, 0o600))
	refs, err := NewMerchantReferences(shoppePath, syntheticMerchantWordDict())
	require.NoError(t, err)

	text, ok := refs.Text("RUDE_ARMS_0")
	require.True(t, ok)
	assert.Equal(t, "Hail the !", strings.TrimSpace(text))

	text, ok = refs.Text("RUDE_ARMS_1")
	require.True(t, ok)
	assert.Equal(t, "thou friend", strings.TrimSpace(text))

	_, ok = refs.Text("NOT_A_SHOPPE_LABEL")
	assert.False(t, ok)
}

func TestMerchantTextFormattingUsesTypedPlaceholders(t *testing.T) {
	t.Parallel()

	data := syntheticShoppeData(t, map[MerchantTextID][]byte{
		"BAR_TXT_0": {'#', '|', '$', '|', '@', '|', '%', '|', '^', '|', '&', '|', '*'},
		"SENS_0":    {'&', '|', '*'},
	})
	refs, err := newMerchantReferences(data, syntheticMerchantWordDict(), originalMerchantDefinitions)
	require.NoError(t, err)

	values := MerchantTextValues{
		OwnerName:     "Owner",
		ShopName:      "Shop",
		ItemName:      "Item",
		Price:         42,
		Quantity:      7,
		InformantName: "Informant",
		LocationName:  "Location",
		TimeOfDay:     MerchantAfternoon,
	}
	formatted, err := refs.FormatText("BAR_TXT_0", values)
	require.NoError(t, err)
	assert.Equal(t, "Shop|Owner|afternoon|42|7|Item|Location", strings.TrimSpace(formatted))

	formatted, err = refs.FormatText("SENS_0", values)
	require.NoError(t, err)
	assert.Equal(t, "Informant|Location", strings.TrimSpace(formatted))
}

func TestMerchantTextFormattingRejectsMissingTypedValues(t *testing.T) {
	t.Parallel()

	data := syntheticShoppeData(t, map[MerchantTextID][]byte{
		"BAR_TXT_0": {'#'},
		"BAR_TXT_1": {'@'},
	})
	refs, err := newMerchantReferences(data, syntheticMerchantWordDict(), originalMerchantDefinitions)
	require.NoError(t, err)

	_, err = refs.FormatText("BAR_TXT_0", MerchantTextValues{})
	assert.ErrorContains(t, err, "no value for placeholder")

	_, err = refs.FormatText("BAR_TXT_1", MerchantTextValues{})
	assert.ErrorContains(t, err, "missing or invalid time of day")

	_, err = refs.FormatText("UNKNOWN", MerchantTextValues{})
	assert.ErrorContains(t, err, "unknown merchant text ID")
}

func TestMerchantTextLoaderRejectsCorruption(t *testing.T) {
	t.Parallel()

	t.Run("truncated", func(t *testing.T) {
		data := syntheticShoppeData(t, nil)
		_, err := newMerchantReferences(data[:100], syntheticMerchantWordDict(), originalMerchantDefinitions)
		assert.ErrorContains(t, err, "outside")
	})

	t.Run("shifted record boundary", func(t *testing.T) {
		data := syntheticShoppeData(t, nil)
		data[16] = 'x'
		_, err := newMerchantReferences(data, syntheticMerchantWordDict(), originalMerchantDefinitions)
		assert.ErrorContains(t, err, "next record must start")
	})

	t.Run("invalid compressed token", func(t *testing.T) {
		data := syntheticShoppeData(t, map[MerchantTextID][]byte{
			"RUDE_ARMS_0": {0xc2},
		})
		_, err := newMerchantReferences(data, syntheticMerchantWordDict(), originalMerchantDefinitions)
		assert.ErrorContains(t, err, "expanding record")
	})

	t.Run("blank record", func(t *testing.T) {
		data := syntheticShoppeData(t, map[MerchantTextID][]byte{
			"RUDE_ARMS_0": []byte(" "),
		})
		_, err := newMerchantReferences(data, syntheticMerchantWordDict(), originalMerchantDefinitions)
		assert.ErrorContains(t, err, `record "RUDE_ARMS_0" is blank`)
	})

	t.Run("missing dictionary", func(t *testing.T) {
		data := syntheticShoppeData(t, nil)
		_, err := newMerchantReferences(data, nil, originalMerchantDefinitions)
		assert.ErrorContains(t, err, "dictionary is required")
	})
}

func TestNewMerchantReferencesRequiresShoppeData(t *testing.T) {
	t.Parallel()

	_, err := NewMerchantReferences(
		filepath.Join(t.TempDir(), "SHOPPE.DAT"),
		syntheticMerchantWordDict(),
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "reading required SHOPPE.DAT")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func syntheticMerchantWordDict() *WordDict {
	words := make([]string, 118)
	words[0] = "the"
	words[1] = "thou"
	return NewWordDict(words)
}

func syntheticShoppeData(t *testing.T, overrides map[MerchantTextID][]byte) []byte {
	t.Helper()

	const syntheticLastRecordLength = 64
	data := make([]byte, merchantTextRecords[len(merchantTextRecords)-1].offset+syntheticLastRecordLength)
	for index, record := range merchantTextRecords {
		end := len(data)
		if index+1 < len(merchantTextRecords) {
			end = merchantTextRecords[index+1].offset
		}
		for position := record.offset; position < end-1; position++ {
			data[position] = 'x'
		}
		data[end-1] = 0
	}

	for id, replacement := range overrides {
		recordIndex := -1
		for index, record := range merchantTextRecords {
			if record.id == id {
				recordIndex = index
				break
			}
		}
		require.NotEqual(t, -1, recordIndex, "unknown synthetic text ID %q", id)

		start := merchantTextRecords[recordIndex].offset
		end := len(data) - 1
		if recordIndex+1 < len(merchantTextRecords) {
			end = merchantTextRecords[recordIndex+1].offset - 1
		}
		require.LessOrEqual(t, len(replacement), end-start)
		for position := start; position < end; position++ {
			data[position] = ' '
		}
		copy(data[start:end], replacement)
	}

	return data
}
