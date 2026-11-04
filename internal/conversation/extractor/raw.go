package extractor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	tlkHeaderPrefixSize = 2
	tlkHeaderEntrySize  = 4
	questionMarker      = 0x90
	firstQuestionID     = 0x91
	lastQuestionID      = 0x9f
	recordEndByte       = 0xc0
	fixedFieldCount     = 5
	maxOriginalInputLen = 15
)

type sourceLocation struct {
	filename       string
	conversationID int
}

//nolint:err113 // Source-specific binary errors require dynamic file, conversation, and byte context.
func (location sourceLocation) errorAt(offset int, format string, arguments ...any) error {
	message := fmt.Sprintf(format, arguments...)

	return fmt.Errorf(
		"%s conversation %d offset 0x%04x: %s",
		location.filename,
		location.conversationID,
		offset,
		message,
	)
}

type rawField struct {
	data   []byte
	offset int
	where  sourceLocation
}

type rawPair struct {
	key                      rawField
	response                 rawField
	unrenderableOutputMarker *rawField
}

type rawQuestion struct {
	id                      byte
	prompt                  rawField
	fallback                rawField
	answers                 []rawPair
	unreachableOrphanAnswer *rawField
	hasInput                bool
}

type rawConversation struct {
	id        int
	fixed     [fixedFieldCount]rawField
	topics    []rawPair
	questions []rawQuestion
}

type tlkEntry struct {
	id     int
	offset int
}

//nolint:cyclop,err113,funlen // Header validation reports each exact binary-contract failure.
func parseTLK(filename string, data []byte, expectedCount int) ([]rawConversation, error) {
	if len(data) < tlkHeaderPrefixSize {
		return nil, fmt.Errorf("%s offset 0x0000: TLK file is shorter than its entry count", filename)
	}
	entryCount := int(binary.LittleEndian.Uint16(data[:tlkHeaderPrefixSize]))
	if entryCount != expectedCount {
		return nil, fmt.Errorf("%s offset 0x0000: entry count is %d, want %d", filename, entryCount, expectedCount)
	}
	tableEnd := tlkHeaderPrefixSize + entryCount*tlkHeaderEntrySize
	if tableEnd > len(data) {
		return nil, fmt.Errorf("%s offset 0x0000: truncated TLK header needs %d bytes, has %d", filename, tableEnd, len(data))
	}

	entries := make([]tlkEntry, 0, entryCount)
	seenIDs := make(map[int]struct{}, entryCount)
	for index := range entryCount {
		base := tlkHeaderPrefixSize + index*tlkHeaderEntrySize
		conversationID := int(binary.LittleEndian.Uint16(data[base : base+2]))
		offset := int(binary.LittleEndian.Uint16(data[base+2 : base+4]))
		if conversationID == 0 {
			return nil, fmt.Errorf("%s header entry %d offset 0x%04x: conversation ID must be one-based", filename, index, base)
		}
		if _, exists := seenIDs[conversationID]; exists {
			return nil, fmt.Errorf(
				"%s header entry %d offset 0x%04x: duplicate conversation ID %d",
				filename,
				index,
				base,
				conversationID,
			)
		}
		seenIDs[conversationID] = struct{}{}
		if index > 0 && conversationID <= entries[index-1].id {
			return nil, fmt.Errorf(
				"%s header entry %d offset 0x%04x: conversation ID %d is not strictly ordered",
				filename,
				index,
				base,
				conversationID,
			)
		}
		if conversationID != index+1 {
			return nil, fmt.Errorf(
				"%s header entry %d offset 0x%04x: conversation ID is %d, want %d",
				filename,
				index,
				base,
				conversationID,
				index+1,
			)
		}
		if offset < tableEnd || offset >= len(data) {
			return nil, fmt.Errorf(
				"%s header entry %d offset 0x%04x: record offset 0x%04x is outside [%04x,%04x)",
				filename,
				index,
				base,
				offset,
				tableEnd,
				len(data),
			)
		}
		if index == 0 && offset != tableEnd {
			return nil, fmt.Errorf(
				"%s header entry 0 offset 0x%04x: first record starts at 0x%04x, "+
					"leaving unaccounted header padding",
				filename,
				base,
				offset,
			)
		}
		if index > 0 && offset <= entries[index-1].offset {
			return nil, fmt.Errorf(
				"%s header entry %d offset 0x%04x: record offset 0x%04x is not strictly ordered",
				filename,
				index,
				base,
				offset,
			)
		}
		entries = append(entries, tlkEntry{id: conversationID, offset: offset})
	}

	conversations := make([]rawConversation, 0, entryCount)
	for index, entry := range entries {
		end := len(data)
		if index+1 < len(entries) {
			end = entries[index+1].offset
		}
		conversation, err := parseRecord(filename, entry.id, entry.offset, data[entry.offset:end])
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}

	return conversations, nil
}

type recordReader struct {
	data  []byte
	base  int
	index int
	where sourceLocation
}

//nolint:gocognit,cyclop,exhaustruct_v5,funlen // The state machine mirrors the closed TLK field grammar.
func parseRecord(filename string, id, base int, data []byte) (rawConversation, error) {
	reader := recordReader{data: data, base: base, index: 0, where: sourceLocation{filename: filename, conversationID: id}}
	conversation := rawConversation{id: id, topics: nil, questions: nil}
	for index := range fixedFieldCount {
		field, err := reader.field(fmt.Sprintf("fixed field %d", index+1))
		if err != nil {
			return rawConversation{}, err
		}
		conversation.fixed[index] = field
	}

	for !reader.atQuestionMarker() {
		pair, err := reader.topicPair()
		if err != nil {
			return rawConversation{}, err
		}
		conversation.topics = append(conversation.topics, pair)
	}

	seenQuestions := make(map[byte]struct{})
	lastID := byte(0)
	for {
		markerOffset := reader.absoluteOffset()
		if reader.atTerminalMarker() {
			reader.index += 3
			if reader.remaining() != 0 {
				return rawConversation{}, reader.where.errorAt(reader.absoluteOffset(), "unaccounted bytes follow terminal marker")
			}

			return conversation, nil
		}
		if err := reader.consume(questionMarker); err != nil {
			return rawConversation{}, err
		}
		if reader.remaining() == 0 {
			return rawConversation{}, reader.where.errorAt(markerOffset, "truncated question marker")
		}
		questionID := reader.data[reader.index]
		reader.index++
		if questionID < firstQuestionID || questionID > lastQuestionID {
			return rawConversation{}, reader.where.errorAt(
				markerOffset+1,
				"question ID 0x%02x is outside 0x91..0x9f",
				questionID,
			)
		}
		if _, exists := seenQuestions[questionID]; exists {
			return rawConversation{}, reader.where.errorAt(markerOffset+1, "duplicate question ID 0x%02x", questionID)
		}
		if lastID != 0 && questionID <= lastID {
			return rawConversation{}, reader.where.errorAt(
				markerOffset+1,
				"question ID 0x%02x is not strictly ordered",
				questionID,
			)
		}
		seenQuestions[questionID] = struct{}{}
		lastID = questionID

		prompt, err := reader.field("question prompt")
		if err != nil {
			return rawConversation{}, err
		}
		question := rawQuestion{
			id: questionID, prompt: prompt, fallback: rawField{}, answers: nil,
			unreachableOrphanAnswer: nil, hasInput: false,
		}
		if reader.atQuestionMarker() {
			conversation.questions = append(conversation.questions, question)

			continue
		}
		fallback, err := reader.field("question fallback")
		if err != nil {
			return rawConversation{}, err
		}
		question.fallback = fallback
		question.hasInput = true
		for !reader.atQuestionMarker() && !reader.atTerminalMarker() {
			answer, orphan, pairErr := reader.questionAnswer()
			if pairErr != nil {
				return rawConversation{}, pairErr
			}
			if orphan != nil {
				question.unreachableOrphanAnswer = orphan

				break
			}
			question.answers = append(question.answers, answer)
		}
		conversation.questions = append(conversation.questions, question)
	}
}

func (reader *recordReader) questionAnswer() (rawPair, *rawField, error) {
	key, err := reader.field("question answer key")
	if err != nil {
		return rawPair{}, nil, err
	}
	if len(key.data) == 0 {
		return rawPair{}, nil, reader.where.errorAt(key.offset, "question answer key is empty")
	}
	if reader.atTerminalMarker() {
		if isOriginalInputReachable(key.data) {
			return rawPair{}, nil, reader.where.errorAt(
				key.offset,
				"reachable question answer key has no response before terminal marker",
			)
		}

		var noPair rawPair

		return noPair, &key, nil
	}
	response, err := reader.field("question answer response")
	if err != nil {
		return rawPair{}, nil, err
	}

	return rawPair{key: key, response: response, unrenderableOutputMarker: nil}, nil, nil
}

func (reader *recordReader) topicPair() (rawPair, error) {
	key, err := reader.field("topic key")
	if err != nil {
		return rawPair{}, err
	}
	if len(key.data) == 0 {
		return rawPair{}, reader.where.errorAt(key.offset, "topic key is empty")
	}
	if !reader.atQuestionMarker() {
		response, responseErr := reader.field("topic response")
		if responseErr != nil {
			return rawPair{}, responseErr
		}

		return rawPair{key: key, response: response, unrenderableOutputMarker: nil}, nil
	}
	if reader.atTerminalMarker() || reader.remaining() < 2 ||
		!isPrintableMatchKey(key.data) || !isQuestionID(reader.data[reader.index+1]) {
		return rawPair{}, reader.where.errorAt(
			reader.absoluteOffset(),
			"topic key has no NUL-terminated response before question marker",
		)
	}
	marker := rawField{
		data: reader.data[reader.index : reader.index+1], offset: reader.absoluteOffset(), where: reader.where,
	}
	response := rawField{
		data: reader.data[reader.index+1 : reader.index+2], offset: reader.absoluteOffset() + 1, where: reader.where,
	}

	return rawPair{key: key, response: response, unrenderableOutputMarker: &marker}, nil
}

func (reader *recordReader) field(kind string) (rawField, error) {
	if reader.remaining() == 0 {
		return rawField{}, reader.where.errorAt(reader.absoluteOffset(), "missing NUL-terminated %s", kind)
	}
	relativeEnd := bytes.IndexByte(reader.data[reader.index:], 0)
	if relativeEnd < 0 {
		return rawField{}, reader.where.errorAt(reader.absoluteOffset(), "unterminated %s", kind)
	}
	start := reader.index
	end := start + relativeEnd
	reader.index = end + 1

	return rawField{data: reader.data[start:end], offset: reader.base + start, where: reader.where}, nil
}

func (reader *recordReader) consume(want byte) error {
	if reader.remaining() == 0 {
		return reader.where.errorAt(reader.absoluteOffset(), "expected byte 0x%02x at end of record", want)
	}
	if got := reader.data[reader.index]; got != want {
		return reader.where.errorAt(reader.absoluteOffset(), "got byte 0x%02x, want 0x%02x", got, want)
	}
	reader.index++

	return nil
}

func (reader *recordReader) atQuestionMarker() bool {
	return reader.remaining() > 0 && reader.data[reader.index] == questionMarker
}

func (reader *recordReader) atTerminalMarker() bool {
	return reader.remaining() >= 3 && bytes.Equal(
		reader.data[reader.index:reader.index+3],
		[]byte{questionMarker, lastQuestionID, recordEndByte},
	)
}

func isQuestionID(value byte) bool {
	return value >= firstQuestionID && value <= lastQuestionID
}

//nolint:mnd // 0x7f is the original low-seven-bit text mask and maximum accepted literal byte; ref:TALK-0002.
func isPrintableMatchKey(data []byte) bool {
	for _, value := range data {
		character := value & 0x7f
		if character < 0x20 || character > 0x7f {
			return false
		}
	}

	return true
}

func isOriginalInputReachable(data []byte) bool {
	return len(data) <= maxOriginalInputLen && isPrintableMatchKey(data)
}

func (reader *recordReader) remaining() int { return len(reader.data) - reader.index }
func (reader *recordReader) absoluteOffset() int {
	return reader.base + reader.index
}

//nolint:err113,mnd // Dictionary token gaps require exact diagnostics; ref:TALK-0002.
func parseWordTable(data []byte) (map[byte]string, error) {
	if len(data) == 0 || data[len(data)-1] != 0 {
		return nil, errors.New("compressed-word table is not NUL terminated")
	}
	parts := bytes.Split(data, []byte{0})
	if len(parts) != 119 || len(parts[len(parts)-1]) != 0 {
		return nil, fmt.Errorf("compressed-word table contains %d words, want 118", len(parts)-1)
	}
	words := parts[:len(parts)-1]
	for index, word := range words {
		if len(word) == 0 {
			return nil, fmt.Errorf("compressed-word table word %d is empty", index)
		}
		for _, character := range word {
			if character < 0x20 || character > 0x7e {
				return nil, fmt.Errorf("compressed-word table word %d contains non-ASCII byte 0x%02x", index, character)
			}
		}
	}

	tokens := make([]byte, 0, len(words))
	appendRange := func(start, end byte) {
		for token := start; token <= end; token++ {
			tokens = append(tokens, token)
		}
	}
	appendRange(0x01, 0x07)
	appendRange(0x09, 0x1b)
	appendRange(0x1d, 0x31)
	appendRange(0x33, 0x40)
	appendRange(0x42, 0x42)
	appendRange(0x44, 0x45)
	appendRange(0x47, 0x47)
	appendRange(0x4c, 0x80)
	if len(tokens) != len(words) {
		return nil, fmt.Errorf("internal compressed-token map has %d entries, want %d", len(tokens), len(words))
	}
	dictionary := make(map[byte]string, len(words))
	for index, token := range tokens {
		dictionary[token] = string(words[index])
	}

	return dictionary, nil
}
