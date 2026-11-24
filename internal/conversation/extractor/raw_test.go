package extractor

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWordTableMapsOriginalTokenRanges(t *testing.T) {
	t.Parallel()

	dictionary, err := parseWordTable(syntheticWordTable())
	require.NoError(t, err)
	require.Len(t, dictionary, 118)
	assert.Equal(t, "w000", dictionary[0x01])
	assert.Equal(t, "w006", dictionary[0x07])
	assert.Equal(t, "w007", dictionary[0x09])
	assert.Equal(t, "w117", dictionary[0x80])
	_, exists := dictionary[0x08]
	assert.False(t, exists)
}

func TestParseWordTableRejectsMalformedTables(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		data    []byte
		message string
	}{
		{name: "unterminated", data: []byte("word"), message: "not NUL terminated"},
		{name: "wrong count", data: []byte("word\x00"), message: "contains 1 words, want 118"},
		{name: "empty word", data: append([]byte{0}, syntheticWordTable()[5:]...), message: "word 0 is empty"},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseWordTable(testCase.data)
			require.ErrorContains(t, err, testCase.message)
		})
	}
}

func TestParseTLKPreservesOrderedOneBasedRecords(t *testing.T) {
	t.Parallel()

	first := minimalRecord()
	second := minimalRecord()
	data := buildTLK([]int{1, 2}, first, second)
	conversations, err := parseTLK("TEST.TLK", data, 2)
	require.NoError(t, err)
	require.Len(t, conversations, 2)
	assert.Equal(t, 1, conversations[0].id)
	assert.Equal(t, 2, conversations[1].id)
	for _, conversation := range conversations {
		assert.Len(t, conversation.fixed, fixedFieldCount)
	}
}

func TestParseRecordSupportsFallbackOnlyAndTerminalPromptQuestions(t *testing.T) {
	t.Parallel()

	record := append(fixedFields("Name", "Description", "Greeting", "Job", "Bye"),
		questionMarker, firstQuestionID)
	record = appendField(record, upper("Question?"))
	record = appendField(record, upper("Fallback."))
	record = append(record, questionMarker, firstQuestionID+1)
	record = appendField(record, append(upper("Transfer."), opReturnTopics))
	record = append(record, questionMarker, lastQuestionID, recordEndByte)

	conversation, err := parseRecord("TEST.TLK", 1, 0x20, record)
	require.NoError(t, err)
	require.Len(t, conversation.questions, 2)
	assert.True(t, conversation.questions[0].hasInput)
	assert.Empty(t, conversation.questions[0].answers)
	assert.False(t, conversation.questions[1].hasInput)
}

func TestParseRecordConsumesTerminalMarkerAfterFinalQuestionAnswer(t *testing.T) {
	t.Parallel()

	record := append(fixedFields("Name", "Description", "Greeting", "Job", "Bye"),
		questionMarker, firstQuestionID)
	record = appendField(record, upper("Question?"))
	record = appendField(record, upper("Fallback."))
	record = appendField(record, upper("y"))
	record = appendField(record, upper("Yes."))
	terminalOffset := 0x3470 + len(record)
	record = append(record, questionMarker, lastQuestionID, recordEndByte)

	conversation, err := parseRecord("TOWNE.TLK", 23, 0x3470, record)
	require.NoError(t, err)
	require.Len(t, conversation.questions, 1)
	question := conversation.questions[0]
	assert.Equal(t, byte(firstQuestionID), question.id)
	assert.True(t, question.hasInput)
	require.Len(t, question.answers, 1)
	assert.Equal(t, terminalOffset, question.answers[0].response.offset+len(question.answers[0].response.data)+1)
}

func TestParseRecordPreservesPrintableTopicBeforeQuestionMarker(t *testing.T) {
	t.Parallel()

	base := 0x300
	record := fixedFields("Name", "Description", "Greeting", "Job", "Bye")
	record = appendField(record, upper("ask"))
	markerOffset := base + len(record)
	record = append(record, questionMarker, firstQuestionID)
	record = appendField(record, upper("Question?"))
	record = appendField(record, upper("Fallback."))
	record = append(record, questionMarker, lastQuestionID, recordEndByte)

	conversation, err := parseRecord("TOWNE.TLK", 23, base, record)
	require.NoError(t, err)
	require.Len(t, conversation.topics, 1)
	require.Len(t, conversation.questions, 1)
	pair := conversation.topics[0]
	assert.Equal(t, []byte{firstQuestionID}, pair.response.data)
	assert.Equal(t, markerOffset+1, pair.response.offset)
	require.NotNil(t, pair.unrenderableOutputMarker)
	assert.Equal(t, []byte{questionMarker}, pair.unrenderableOutputMarker.data)
	assert.Equal(t, markerOffset, pair.unrenderableOutputMarker.offset)
	assert.Equal(t, byte(firstQuestionID), conversation.questions[0].id)
}

func TestParseRecordRejectsOtherTopicResponsesOrphanedByQuestionMarker(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		key    []byte
		marker []byte
	}{
		{name: "unreachable key", key: []byte{opIncludeNext}, marker: []byte{questionMarker, firstQuestionID}},
		{name: "terminal marker", key: upper("ask"), marker: []byte{questionMarker, lastQuestionID, recordEndByte}},
		{name: "invalid question ID", key: upper("ask"), marker: []byte{questionMarker, 0x80}},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			record := fixedFields("", "", "", "", "")
			record = appendField(record, testCase.key)
			record = append(record, testCase.marker...)
			_, err := parseRecord("TOWNE.TLK", 23, 0x300, record)
			require.ErrorContains(t, err, "topic key has no NUL-terminated response before question marker")
		})
	}
}

func TestParseRecordPreservesUnreachableQuestionAnswerKeyBeforeTerminalMarker(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		orphan []byte
	}{
		{name: "overlength printable", orphan: upper("sixteen byte key!")},
		{name: "masked control", orphan: append(upper("short"), opAskName)},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			record := append(fixedFields("Name", "Description", "Greeting", "Job", "Bye"),
				questionMarker, firstQuestionID)
			record = appendField(record, upper("Question?"))
			record = appendField(record, upper("Fallback."))
			orphanOffset := 0x400 + len(record)
			record = appendField(record, testCase.orphan)
			terminalOffset := 0x400 + len(record)
			record = append(record, questionMarker, lastQuestionID, recordEndByte)

			conversation, err := parseRecord("DWELLING.TLK", 15, 0x400, record)
			require.NoError(t, err)
			require.Len(t, conversation.questions, 1)
			question := conversation.questions[0]
			assert.Empty(t, question.answers)
			require.NotNil(t, question.unreachableOrphanAnswer)
			assert.Equal(t, testCase.orphan, question.unreachableOrphanAnswer.data)
			assert.Equal(t, orphanOffset, question.unreachableOrphanAnswer.offset)
			assert.Equal(t, terminalOffset, orphanOffset+len(testCase.orphan)+1)
		})
	}
}

func TestParseRecordRejectsReachableQuestionAnswerKeyWithoutResponse(t *testing.T) {
	t.Parallel()

	record := append(fixedFields("Name", "Description", "Greeting", "Job", "Bye"),
		questionMarker, firstQuestionID)
	record = appendField(record, upper("Question?"))
	record = appendField(record, upper("Fallback."))
	orphanOffset := 0x400 + len(record)
	record = appendField(record, upper("reachable"))
	record = append(record, questionMarker, lastQuestionID, recordEndByte)

	_, err := parseRecord("DWELLING.TLK", 15, 0x400, record)
	require.ErrorContains(t, err, "reachable question answer key has no response before terminal marker")
	assert.Contains(t, err.Error(), fmt.Sprintf("offset 0x%04x", orphanOffset))
}

func TestParseTLKRejectsMalformedHeadersAndRecords(t *testing.T) {
	t.Parallel()

	valid := buildTLK([]int{1}, minimalRecord())
	testCases := []struct {
		name    string
		mutate  func([]byte) []byte
		message string
	}{
		{
			name: "unexpected count", mutate: func(data []byte) []byte { return data },
			message: "entry count is 1, want 2",
		},
		{
			name: "zero ID", mutate: func(data []byte) []byte {
				binary.LittleEndian.PutUint16(data[2:4], 0)

				return data
			}, message: "conversation ID must be one-based",
		},
		{
			name: "offset in table", mutate: func(data []byte) []byte {
				binary.LittleEndian.PutUint16(data[4:6], 2)

				return data
			}, message: "record offset",
		},
		{
			name: "trailing bytes", mutate: func(data []byte) []byte {
				return append(data, 0xaa)
			}, message: "unaccounted bytes follow terminal marker",
		},
		{
			name: "unterminated field", mutate: func(data []byte) []byte {
				return data[:len(data)-3]
			}, message: "missing NUL-terminated topic key",
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data := testCase.mutate(append([]byte(nil), valid...))
			expectedCount := 1
			if testCase.name == "unexpected count" {
				expectedCount = 2
			}
			_, err := parseTLK("TEST.TLK", data, expectedCount)
			require.ErrorContains(t, err, testCase.message)
		})
	}
}

func TestParseRecordRejectsQuestionOrderingAndBrokenPairs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		record  []byte
		message string
	}{
		{
			name: "duplicate question",
			record: questionRecord(
				[]byte{questionMarker, firstQuestionID}, upper("One?"), upper("No."),
				[]byte{questionMarker, firstQuestionID}, upper("Again?"), upper("No."),
			),
			message: "duplicate question ID",
		},
		{
			name: "descending question",
			record: questionRecord(
				[]byte{questionMarker, firstQuestionID + 1}, upper("Two?"), upper("No."),
				[]byte{questionMarker, firstQuestionID}, upper("One?"), upper("No."),
			),
			message: "not strictly ordered",
		},
		{
			name: "empty topic key",
			record: append(append(fixedFields("", "", "", "", ""), 0),
				questionMarker, lastQuestionID, recordEndByte),
			message: "topic key is empty",
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseRecord("TEST.TLK", 3, 0x80, testCase.record)
			require.ErrorContains(t, err, testCase.message)
			assert.Contains(t, err.Error(), "TEST.TLK conversation 3 offset")
		})
	}
}

func syntheticWordTable() []byte {
	data := make([]byte, 0, compressedWordsLength)
	for index := range 118 {
		data = append(data, fmt.Sprintf("w%03d", index)...)
		data = append(data, 0)
	}

	return data
}

func buildTLK(ids []int, records ...[]byte) []byte {
	headerSize := tlkHeaderPrefixSize + len(ids)*tlkHeaderEntrySize
	data := make([]byte, headerSize)
	binary.LittleEndian.PutUint16(data[:2], uint16(len(ids)))
	offset := headerSize
	for index, id := range ids {
		base := tlkHeaderPrefixSize + index*tlkHeaderEntrySize
		binary.LittleEndian.PutUint16(data[base:base+2], uint16(id))
		binary.LittleEndian.PutUint16(data[base+2:base+4], uint16(offset))
		data = append(data, records[index]...)
		offset += len(records[index])
	}

	return data
}

func minimalRecord() []byte {
	record := fixedFields("", "", "", "", "")

	return append(record, questionMarker, lastQuestionID, recordEndByte)
}

func fixedFields(values ...string) []byte {
	data := make([]byte, 0)
	for _, value := range values {
		data = appendField(data, upper(value))
	}

	return data
}

func appendField(data, field []byte) []byte {
	data = append(data, field...)

	return append(data, 0)
}

func upper(value string) []byte {
	data := []byte(value)
	for index := range data {
		data[index] |= 0x80
	}

	return data
}

func questionRecord(parts ...[]byte) []byte {
	record := fixedFields("", "", "", "", "")
	for index, part := range parts {
		if index%3 == 0 {
			record = append(record, part...)
		} else {
			record = appendField(record, part)
		}
	}

	return append(record, questionMarker, lastQuestionID, recordEndByte)
}

// A lowercase copy of the original must work on case-sensitive file systems.
func TestReadInputsAcceptsLowercaseFileNames(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	for _, filename := range sourceFilenames() {
		require.NoError(t, os.WriteFile(filepath.Join(directory, strings.ToLower(filename)), []byte{1}, 0o600))
	}

	inputs, _, err := readInputs(directory)
	require.NoError(t, err)
	assert.Len(t, inputs, len(sourceFilenames()))
}
