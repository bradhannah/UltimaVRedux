package extractor

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
)

const (
	opAvatarName    = 0x81
	opEnd           = 0x82
	opPause         = 0x83
	opJoinParty     = 0x84
	opTakeGold      = 0x85
	opGiveItem      = 0x86
	opIncludeNext   = 0x87
	opAskName       = 0x88
	opKarmaUp       = 0x89
	opKarmaDown     = 0x8a
	opCallGuards    = 0x8b
	opIfKnowsName   = 0x8c
	opNewline       = 0x8d
	opRunic         = 0x8e
	opWaitForKey    = 0x8f
	opQuoteBoundary = 0xa2
	opIfKarma       = 0xfe
	opReturnTopics  = 0xff
)

type instructionKind int

const (
	instructionLiteral instructionKind = iota
	instructionWord
	instructionAvatarName
	instructionNewline
	instructionRunic
	instructionQuoteBoundary
	instructionInclude
	instructionAction
	instructionAsk
	instructionIfKnowsName
	instructionIfKarma
)

type actionCode int

const (
	actionEnd actionCode = iota
	actionPause
	actionJoinParty
	actionTakeGold
	actionGiveItem
	actionAskName
	actionKarmaUp
	actionKarmaDown
	actionCallGuards
	actionWaitForKey
	actionReturnTopics
)

type instruction struct {
	kind   instructionKind
	action actionCode
	text   string
	value  int
	target byte
	offset int
	where  sourceLocation
}

//nolint:gocognit,gocyclo,cyclop,mnd,exhaustruct_v5,lll,funlen // Closed opcode dispatch mirrors source order.
func parseExecutable(field rawField, dictionary map[byte]string, allowInclude bool) ([]instruction, error) {
	instructions := make([]instruction, 0, len(field.data))
	for index := 0; index < len(field.data); index++ {
		value := field.data[index]
		currentOffset := field.offset + index
		add := func(kind instructionKind) {
			instructions = append(instructions, instruction{kind: kind, offset: currentOffset, where: field.where})
		}
		switch value {
		case opAvatarName:
			add(instructionAvatarName)
		case opEnd:
			instructions = append(instructions, actionInstruction(actionEnd, currentOffset, field.where))
		case opPause:
			instructions = append(instructions, actionInstruction(actionPause, currentOffset, field.where))
		case opJoinParty:
			instructions = append(instructions, actionInstruction(actionJoinParty, currentOffset, field.where))
		case opTakeGold:
			if index+3 >= len(field.data) {
				return nil, field.where.errorAt(currentOffset, "take_gold opcode requires exactly three amount bytes")
			}
			amount := 0
			for payloadIndex := 1; payloadIndex <= 3; payloadIndex++ {
				digit := field.data[index+payloadIndex] & 0x7f
				if digit < '0' || digit > '9' {
					return nil, field.where.errorAt(currentOffset+payloadIndex, "take_gold payload byte 0x%02x is not an ASCII digit", field.data[index+payloadIndex])
				}
				amount = amount*10 + int(digit-'0')
			}
			if amount == 0 {
				return nil, field.where.errorAt(currentOffset, "take_gold amount 000 cannot be represented by semantic v1")
			}
			entry := actionInstruction(actionTakeGold, currentOffset, field.where)
			entry.value = amount
			instructions = append(instructions, entry)
			index += 3
		case opGiveItem:
			if index+1 >= len(field.data) {
				return nil, field.where.errorAt(currentOffset, "give_item opcode requires one item byte")
			}
			item, err := semanticItem(field.data[index+1] & 0x7f)
			if err != nil {
				return nil, field.where.errorAt(currentOffset+1, "%v", err)
			}
			entry := actionInstruction(actionGiveItem, currentOffset, field.where)
			entry.text = item
			instructions = append(instructions, entry)
			index++
		case opIncludeNext:
			if !allowInclude {
				return nil, field.where.errorAt(currentOffset, "include-next-response opcode is only supported inside a keyword response")
			}
			add(instructionInclude)
		case opAskName:
			instructions = append(instructions, actionInstruction(actionAskName, currentOffset, field.where))
		case opKarmaUp:
			instructions = append(instructions, actionInstruction(actionKarmaUp, currentOffset, field.where))
		case opKarmaDown:
			instructions = append(instructions, actionInstruction(actionKarmaDown, currentOffset, field.where))
		case opCallGuards:
			instructions = append(instructions, actionInstruction(actionCallGuards, currentOffset, field.where))
		case opIfKnowsName:
			if index+1 >= len(field.data) {
				return nil, field.where.errorAt(currentOffset, "npc_knows_party_name branch requires one target byte")
			}
			target := field.data[index+1]
			if target != opReturnTopics && (target < firstQuestionID || target > lastQuestionID) {
				return nil, field.where.errorAt(currentOffset+1, "npc_knows_party_name target 0x%02x is neither a question nor 0xff", target)
			}
			instructions = append(instructions, instruction{
				kind: instructionIfKnowsName, target: target, offset: currentOffset, where: field.where,
			})
			index++
		case opNewline:
			add(instructionNewline)
		case opRunic:
			add(instructionRunic)
		case opWaitForKey:
			instructions = append(instructions, actionInstruction(actionWaitForKey, currentOffset, field.where))
		case opQuoteBoundary:
			add(instructionQuoteBoundary)
		case opIfKarma:
			if index+2 >= len(field.data) {
				return nil, field.where.errorAt(currentOffset, "karma_at_least branch requires threshold and target bytes")
			}
			minimum := int(field.data[index+1] & 0x7f)
			target := field.data[index+2]
			if minimum > 99 {
				return nil, field.where.errorAt(currentOffset+1, "karma threshold %d exceeds semantic v1 maximum 99", minimum)
			}
			if target < firstQuestionID || target > lastQuestionID {
				return nil, field.where.errorAt(currentOffset+2, "karma_at_least target 0x%02x is not a question", target)
			}
			instructions = append(instructions, instruction{
				kind: instructionIfKarma, value: minimum, target: target, offset: currentOffset, where: field.where,
			})
			index += 2
		case opReturnTopics:
			instructions = append(instructions, actionInstruction(actionReturnTopics, currentOffset, field.where))
		default:
			if value >= firstQuestionID && value <= lastQuestionID {
				instructions = append(instructions, instruction{
					kind: instructionAsk, target: value, offset: currentOffset, where: field.where,
				})

				continue
			}
			if word, exists := dictionary[value]; exists {
				instructions = append(instructions, instruction{
					kind: instructionWord, text: word, offset: currentOffset, where: field.where,
				})

				continue
			}
			if value >= 0xa0 && value <= 0xfd {
				character := value & 0x7f
				if character == '{' || character == '}' {
					return nil, field.where.errorAt(currentOffset, "literal %q cannot be represented without changing semantic v1 substitution meaning", character)
				}
				instructions = append(instructions, instruction{
					kind: instructionLiteral, text: string([]byte{character}), offset: currentOffset, where: field.where,
				})

				continue
			}

			return nil, field.where.errorAt(currentOffset, "unsupported executable byte 0x%02x", value)
		}
	}

	return instructions, nil
}

//nolint:exhaustruct_v5 // Non-action instruction fields intentionally remain zero.
func actionInstruction(action actionCode, offset int, where sourceLocation) instruction {
	return instruction{kind: instructionAction, action: action, offset: offset, where: where}
}

//nolint:mnd // 0x7f is the original low-seven-bit text mask and maximum accepted literal byte; ref:TALK-0002.
func decodeMatchKey(field rawField) (string, bool, error) {
	if !isPrintableMatchKey(field.data) {
		return "", false, nil
	}
	var builder strings.Builder
	for _, value := range field.data {
		character := value & 0x7f
		builder.WriteByte(character)
	}
	if strings.TrimSpace(builder.String()) == "" {
		return "", true, field.where.errorAt(
			field.offset,
			"reachable match key is empty after semantic normalization",
		)
	}

	return builder.String(), true, nil
}

//nolint:err113,mnd // Recursive include expansion keeps cycle/range state together.
func flattenResponses(
	pairs []rawPair,
	dictionary map[byte]string,
	reachable []bool,
) ([][]instruction, error) {
	parsed := make([][]instruction, len(pairs))
	parsedSet := make([]bool, len(pairs))
	resolved := make([][]instruction, len(pairs))
	state := make([]byte, len(pairs))
	var resolve func(int) ([]instruction, error)
	resolve = func(index int) ([]instruction, error) {
		if index < 0 || index >= len(parsed) {
			return nil, errors.New("include-next-response target is outside the response list")
		}
		switch state[index] {
		case 1:
			return nil, pairs[index].response.where.errorAt(pairs[index].response.offset, "include-next-response cycle detected")
		case 2:
			return resolved[index], nil
		}
		if !parsedSet[index] {
			instructions, err := parseExecutable(pairs[index].response, dictionary, true)
			if err != nil {
				return nil, err
			}
			parsed[index] = instructions
			parsedSet[index] = true
		}
		state[index] = 1
		output := make([]instruction, 0, len(parsed[index]))
		for _, entry := range parsed[index] {
			if entry.kind != instructionInclude {
				output = append(output, entry)

				continue
			}
			if index+1 >= len(parsed) {
				return nil, entry.where.errorAt(entry.offset, "include-next-response has no following response")
			}
			next, err := resolve(index + 1)
			if err != nil {
				return nil, err
			}
			output = append(output, next...)
		}
		state[index] = 2
		resolved[index] = output

		return output, nil
	}
	for index, isReachable := range reachable {
		if !isReachable {
			continue
		}
		if _, err := resolve(index); err != nil {
			return nil, err
		}
	}

	return resolved, nil
}

func isPureAlias(field rawField) bool {
	return len(field.data) == 1 && field.data[0] == opIncludeNext
}

type responseGroup struct {
	keywords []string
	body     semantic.Body
}

func convertResponseGroups(
	pairs []rawPair,
	dictionary map[byte]string,
	resolveTarget func(instruction) (semantic.Body, error),
) ([]responseGroup, []AuditEntry, error) {
	keys, reachable, auditEntries, err := classifyResponseKeys(pairs)
	if err != nil {
		return nil, nil, err
	}
	resolved, err := flattenResponses(pairs, dictionary, reachable)
	if err != nil {
		return nil, nil, err
	}
	groups := make([]responseGroup, 0, len(pairs))
	for index := 0; index < len(pairs); index++ {
		if !reachable[index] {
			continue
		}
		keywords := []string{keys[index]}
		end := index
		for isPureAlias(pairs[end].response) {
			if end+1 >= len(pairs) {
				return nil, nil, pairs[end].response.where.errorAt(
					pairs[end].response.offset,
					"pure alias has no following response",
				)
			}
			end++
			if reachable[end] {
				keywords = append(keywords, keys[end])
			}
		}
		body, bodyErr := instructionsToBody(resolved[index], semantic.ActionKindSay, resolveTarget)
		if bodyErr != nil {
			return nil, nil, bodyErr
		}
		groups = append(groups, responseGroup{keywords: keywords, body: body})
		index = end
	}

	return groups, auditEntries, nil
}

func classifyResponseKeys(pairs []rawPair) ([]string, []bool, []AuditEntry, error) {
	keys := make([]string, len(pairs))
	reachable := make([]bool, len(pairs))
	auditEntries := make([]AuditEntry, 0)
	for index, pair := range pairs {
		key, isReachable, err := decodeMatchKey(pair.key)
		if err != nil {
			return nil, nil, nil, err
		}
		keys[index] = key
		reachable[index] = isReachable
		if !isReachable {
			auditEntries = append(auditEntries, newUnreachableMatchKeyAudit(pair))
		}
		if pair.unrenderableOutputMarker != nil {
			auditEntries = append(
				auditEntries,
				newUnrenderableControlOutputAudit(*pair.unrenderableOutputMarker),
			)
		}
	}

	return keys, reachable, auditEntries, nil
}

func newUnreachableMatchKeyAudit(pair rawPair) AuditEntry {
	keyDigest := sha256.Sum256(pair.key.data)
	responseDigest := sha256.Sum256(pair.response.data)

	return AuditEntry{
		SourceFile: pair.key.where.filename, ConversationID: pair.key.where.conversationID,
		KeyOffset: pair.key.offset, ResponseOffset: pair.response.offset,
		KeyLength: len(pair.key.data), ResponseLength: len(pair.response.data),
		KeySHA256: hex.EncodeToString(keyDigest[:]), ResponseSHA256: hex.EncodeToString(responseDigest[:]),
		KeyPrefixHex: auditKeyPrefix(pair.key.data), Reason: AuditReasonUnreachableMatchKey,
		Explanation: unreachableMatchKeyExplanation,
	}
}

func newUnrenderableControlOutputAudit(marker rawField) AuditEntry {
	markerDigest := sha256.Sum256(marker.data)
	emptyDigest := sha256.Sum256(nil)

	return AuditEntry{
		SourceFile: marker.where.filename, ConversationID: marker.where.conversationID,
		KeyOffset: marker.offset, ResponseOffset: marker.offset + len(marker.data),
		KeyLength: len(marker.data), ResponseLength: 0,
		KeySHA256: hex.EncodeToString(markerDigest[:]), ResponseSHA256: hex.EncodeToString(emptyDigest[:]),
		KeyPrefixHex: auditKeyPrefix(marker.data), Reason: AuditReasonUnrenderableControlOutput,
		Explanation: unrenderableControlOutputExplanation,
	}
}

func newUnreachableOrphanAnswerKeyAudit(key rawField) AuditEntry {
	keyDigest := sha256.Sum256(key.data)
	emptyDigest := sha256.Sum256(nil)

	return AuditEntry{
		SourceFile: key.where.filename, ConversationID: key.where.conversationID,
		KeyOffset: key.offset, ResponseOffset: key.offset + len(key.data) + 1,
		KeyLength: len(key.data), ResponseLength: 0,
		KeySHA256: hex.EncodeToString(keyDigest[:]), ResponseSHA256: hex.EncodeToString(emptyDigest[:]),
		KeyPrefixHex: auditKeyPrefix(key.data), Reason: AuditReasonUnreachableOrphanAnswerKey,
		Explanation: unreachableOrphanAnswerKeyExplanation,
	}
}

type bodyBuilder struct {
	body         semantic.Body
	content      semantic.RichText
	role         semantic.ActionKind
	runic        bool
	pendingSpace bool
	terminated   bool
}

//nolint:cyclop,err113,exhaustruct_v5,funlen // Ordered conversion is a closed instruction state machine.
func instructionsToBody(
	instructions []instruction,
	initialRole semantic.ActionKind,
	resolveTarget func(instruction) (semantic.Body, error),
) (semantic.Body, error) {
	builder := bodyBuilder{body: make(semantic.Body, 0), content: nil, role: initialRole}
	for index := 0; index < len(instructions); index++ {
		entry := instructions[index]
		if builder.terminated {
			return nil, entry.where.errorAt(entry.offset, "bytes follow a terminal semantic action")
		}
		switch entry.kind {
		case instructionLiteral:
			builder.literal(entry.text)
		case instructionWord:
			builder.word(entry.text)
		case instructionAvatarName:
			builder.literal("{avatar_name}")
		case instructionNewline:
			count := 1
			for index+count < len(instructions) && instructions[index+count].kind == instructionNewline {
				count++
			}
			builder.consumePendingSpace()
			for range count {
				builder.content = append(builder.content, &semantic.LineBreakPart{})
			}
			index += count - 1
		case instructionRunic:
			builder.runic = !builder.runic
		case instructionQuoteBoundary:
			builder.consumePendingSpace()
			builder.flush()
			if builder.role == semantic.ActionKindSay {
				builder.role = semantic.ActionKindNarrate
			} else {
				builder.role = semantic.ActionKindSay
			}
		case instructionAction:
			builder.flush()
			action, terminal := semanticAction(entry)
			builder.body = append(builder.body, action)
			builder.terminated = terminal
		case instructionAsk:
			builder.flush()
			targetBody, targetErr := resolveTarget(entry)
			if targetErr != nil {
				return nil, targetErr
			}
			builder.body = append(builder.body, targetBody...)
			builder.terminated = true
		case instructionIfKnowsName:
			builder.flush()
			thenBody, branchErr := resolveTarget(entry)
			if branchErr != nil {
				return nil, branchErr
			}
			builder.body = append(builder.body, &semantic.BranchAction{
				Condition: &semantic.NPCKnowsPartyNameCondition{}, Then: thenBody, Otherwise: nil,
			})
		case instructionIfKarma:
			builder.flush()
			thenBody, branchErr := resolveTarget(entry)
			if branchErr != nil {
				return nil, branchErr
			}
			builder.body = append(builder.body, &semantic.BranchAction{
				Condition: &semantic.KarmaAtLeastCondition{Minimum: entry.value}, Then: thenBody, Otherwise: nil,
			})
		case instructionInclude:
			return nil, entry.where.errorAt(entry.offset, "unresolved include-next-response instruction")
		}
	}
	if builder.runic {
		return nil, errors.New("executable field ends with an unbalanced runic toggle")
	}
	builder.flush()

	return builder.body, nil
}

func (builder *bodyBuilder) literal(text string) {
	builder.consumePendingSpace()
	builder.appendText(text)
}

func (builder *bodyBuilder) word(word string) {
	if builder.hasVisibleContent() && !builder.endsInSpace() {
		builder.appendText(" ")
	}
	builder.appendText(word)
	builder.pendingSpace = true
}

func (builder *bodyBuilder) hasVisibleContent() bool {
	for _, part := range builder.content {
		switch value := part.(type) {
		case *semantic.TextPart:
			if value.Text != "" {
				return true
			}
		case *semantic.RunicPart:
			if value.Text != "" {
				return true
			}
		}
	}

	return false
}

func (builder *bodyBuilder) endsInSpace() bool {
	if len(builder.content) == 0 {
		return false
	}
	var text string
	switch value := builder.content[len(builder.content)-1].(type) {
	case *semantic.TextPart:
		text = value.Text
	case *semantic.RunicPart:
		text = value.Text
	default:
		return false
	}

	return strings.HasSuffix(text, " ")
}

func (builder *bodyBuilder) consumePendingSpace() {
	if builder.pendingSpace {
		builder.appendText(" ")
		builder.pendingSpace = false
	}
}

func (builder *bodyBuilder) appendText(text string) {
	if text == "" {
		return
	}
	if len(builder.content) > 0 {
		switch previous := builder.content[len(builder.content)-1].(type) {
		case *semantic.TextPart:
			if !builder.runic {
				previous.Text += text

				return
			}
		case *semantic.RunicPart:
			if builder.runic {
				previous.Text += text

				return
			}
		}
	}
	if builder.runic {
		builder.content = append(builder.content, &semantic.RunicPart{Text: text})
	} else {
		builder.content = append(builder.content, &semantic.TextPart{Text: text})
	}
}

func (builder *bodyBuilder) flush() {
	if len(builder.content) == 0 {
		return
	}
	if builder.role == semantic.ActionKindNarrate {
		builder.body = append(builder.body, &semantic.NarrateAction{Content: builder.content})
	} else {
		builder.body = append(builder.body, &semantic.SayAction{Content: builder.content})
	}
	builder.content = nil
}

//nolint:ireturn // The switch exhaustively maps the closed extractor action union.
func semanticAction(entry instruction) (semantic.Action, bool) {
	switch entry.action {
	case actionEnd:
		return &semantic.EndAction{}, true
	case actionPause:
		return &semantic.PauseAction{}, false
	case actionJoinParty:
		return &semantic.JoinPartyAction{}, true
	case actionTakeGold:
		return &semantic.TakeGoldAction{Amount: entry.value}, false
	case actionGiveItem:
		return &semantic.GiveItemAction{Item: entry.text, Quantity: 1}, false
	case actionAskName:
		return &semantic.AskNameAction{}, false
	case actionKarmaUp:
		return &semantic.AdjustKarmaAction{Delta: 1}, false
	case actionKarmaDown:
		return &semantic.AdjustKarmaAction{Delta: -1}, false
	case actionCallGuards:
		return &semantic.CallGuardsAction{}, false
	case actionWaitForKey:
		return &semantic.WaitForKeyAction{}, false
	case actionReturnTopics:
		return &semantic.ReturnToTopicsAction{}, true
	}
	panic("unhandled extractor action")
}

//nolint:err113 // Unknown source item IDs require their exact payload in the error.
func semanticItem(itemID byte) (string, error) {
	if item, exists := semantic.OriginalItemName(int(itemID)); exists {
		return item, nil
	}

	return "", fmt.Errorf("give_item payload 0x%02x has no verified semantic item mapping", itemID)
}

func questionName(id byte) string {
	return "question_" + fmt.Sprintf("%02d", int(id-firstQuestionID)+1)
}
