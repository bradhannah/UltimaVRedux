// Package semantic defines the versioned, data-oriented conversation format.
package semantic

const (
	// SchemaVersion identifies the semantic conversation document version.
	SchemaVersion = "ultima5redux.conversation/v1"
	// ProtocolUltima5 selects Ultima V conversation prompts and matching rules.
	ProtocolUltima5 = "ultima5"
)

// KnowledgeID identifies a stable piece of knowledge revealed by a conversation.
type KnowledgeID string

const (
	// KnowledgeSpellAnYlem identifies the An Ylem spell recipe.
	KnowledgeSpellAnYlem KnowledgeID = "spell.an_ylem"
	// KnowledgeSpellInXenMani identifies the In Xen Mani spell recipe.
	KnowledgeSpellInXenMani KnowledgeID = "spell.in_xen_mani"
	// KnowledgeSpellRelXenBet identifies the Rel Xen Bet spell recipe.
	KnowledgeSpellRelXenBet KnowledgeID = "spell.rel_xen_bet"

	// KnowledgeShadowlordFaulineiName identifies knowledge of Faulinei's name.
	KnowledgeShadowlordFaulineiName KnowledgeID = "shadowlord.faulinei.name"
	// KnowledgeShadowlordFaulineiAssociation identifies Faulinei with Falsehood.
	KnowledgeShadowlordFaulineiAssociation KnowledgeID = "shadowlord.faulinei.association"
	// KnowledgeShadowlordAstarothName identifies knowledge of Astaroth's name.
	KnowledgeShadowlordAstarothName KnowledgeID = "shadowlord.astaroth.name"
	// KnowledgeShadowlordAstarothAssociation identifies Astaroth with Hatred.
	KnowledgeShadowlordAstarothAssociation KnowledgeID = "shadowlord.astaroth.association"
	// KnowledgeShadowlordNosfentorName identifies knowledge of Nosfentor's name.
	KnowledgeShadowlordNosfentorName KnowledgeID = "shadowlord.nosfentor.name"
	// KnowledgeShadowlordNosfentorAssociation identifies Nosfentor with Cowardice.
	KnowledgeShadowlordNosfentorAssociation KnowledgeID = "shadowlord.nosfentor.association"

	// KnowledgeShrineHonestyMantra identifies knowledge of the Honesty mantra.
	KnowledgeShrineHonestyMantra KnowledgeID = "shrine.honesty.mantra"
	// KnowledgeShrineCompassionMantra identifies knowledge of the Compassion mantra.
	KnowledgeShrineCompassionMantra KnowledgeID = "shrine.compassion.mantra"
	// KnowledgeShrineValourMantra identifies knowledge of the Valour mantra.
	KnowledgeShrineValourMantra KnowledgeID = "shrine.valour.mantra"
	// KnowledgeShrineJusticeMantra identifies knowledge of the Justice mantra.
	KnowledgeShrineJusticeMantra KnowledgeID = "shrine.justice.mantra"
	// KnowledgeShrineSacrificeMantra identifies knowledge of the Sacrifice mantra.
	KnowledgeShrineSacrificeMantra KnowledgeID = "shrine.sacrifice.mantra"
	// KnowledgeShrineHonorMantra identifies knowledge of the Honor mantra.
	KnowledgeShrineHonorMantra KnowledgeID = "shrine.honor.mantra"
	// KnowledgeShrineSpiritualityMantra identifies knowledge of the Spirituality mantra.
	KnowledgeShrineSpiritualityMantra KnowledgeID = "shrine.spirituality.mantra"
	// KnowledgeShrineHumilityMantra identifies knowledge of the Humility mantra.
	KnowledgeShrineHumilityMantra KnowledgeID = "shrine.humility.mantra"

	// KnowledgeJournalDomainFalsehood identifies a nameless Falsehood disclosure.
	KnowledgeJournalDomainFalsehood KnowledgeID = "journal.domain.falsehood"
	// KnowledgeJournalDomainHatred identifies a nameless Hatred disclosure.
	KnowledgeJournalDomainHatred KnowledgeID = "journal.domain.hatred"
	// KnowledgeJournalDomainCowardice identifies a nameless Cowardice disclosure.
	KnowledgeJournalDomainCowardice KnowledgeID = "journal.domain.cowardice"
	// KnowledgeJournalFlameTruth identifies the Flame of Truth and its keep.
	KnowledgeJournalFlameTruth KnowledgeID = "journal.flame.truth"
	// KnowledgeJournalFlameLove identifies the Flame of Love and its keep.
	KnowledgeJournalFlameLove KnowledgeID = "journal.flame.love"
	// KnowledgeJournalFlameCourage identifies the Flame of Courage and its keep.
	KnowledgeJournalFlameCourage KnowledgeID = "journal.flame.courage"
	// KnowledgeJournalBanishmentMethod identifies Sutek's complete instructions.
	KnowledgeJournalBanishmentMethod KnowledgeID = "journal.banishment.method"
	// KnowledgeJournalResistancePassword identifies the disclosed Resistance password.
	KnowledgeJournalResistancePassword KnowledgeID = "journal.resistance.password"
	// KnowledgeJournalOppressionPassword identifies Flain's disclosed Oppression password.
	KnowledgeJournalOppressionPassword KnowledgeID = "journal.oppression.password"
	// KnowledgeJournalStonesMelody identifies Kenneth's playable harpsichord lesson.
	KnowledgeJournalStonesMelody KnowledgeID = "journal.stones.melody"

	// KnowledgeDungeonDeceitWordOfPower identifies the revealed Deceit word.
	KnowledgeDungeonDeceitWordOfPower KnowledgeID = "dungeon.deceit.word_of_power"
	// KnowledgeDungeonDespiseWordOfPower identifies the revealed Despise word.
	KnowledgeDungeonDespiseWordOfPower KnowledgeID = "dungeon.despise.word_of_power"
	// KnowledgeDungeonDestardWordOfPower identifies Goeth's spoken Destard clue.
	KnowledgeDungeonDestardWordOfPower KnowledgeID = "dungeon.destard.word_of_power"
	// KnowledgeDungeonWrongWordOfPower identifies the revealed Wrong word.
	KnowledgeDungeonWrongWordOfPower KnowledgeID = "dungeon.wrong.word_of_power"
	// KnowledgeDungeonCovetousWordOfPower identifies the revealed Covetous word.
	KnowledgeDungeonCovetousWordOfPower KnowledgeID = "dungeon.covetous.word_of_power"
	// KnowledgeDungeonShameWordOfPower identifies the revealed Shame word.
	KnowledgeDungeonShameWordOfPower KnowledgeID = "dungeon.shame.word_of_power"
	// KnowledgeDungeonHythlothWordOfPower identifies the revealed Hythloth word.
	KnowledgeDungeonHythlothWordOfPower KnowledgeID = "dungeon.hythloth.word_of_power"
	// KnowledgeDungeonDoomWordOfPower identifies the revealed Doom word.
	KnowledgeDungeonDoomWordOfPower KnowledgeID = "dungeon.doom.word_of_power"
)

// Conversation is one complete semantic conversation definition.
type Conversation struct {
	Schema      string
	Protocol    string
	ID          string
	Origin      Origin
	Name        Body
	Description Body
	Greeting    Body
	Job         Body
	Farewell    Body
	Topics      []Topic
	Questions   map[string]Question
}

// OriginKind identifies where a conversation was authored.
type OriginKind string

const (
	// OriginKindOriginal identifies content imported from original game data.
	OriginKindOriginal OriginKind = "original"
	// OriginKindRedux identifies content authored for Ultima 5 Redux.
	OriginKindRedux OriginKind = "redux"
)

// Origin is the closed union of supported conversation origins.
type Origin interface {
	Kind() OriginKind
	isOrigin()
}

// OriginalSource identifies an original Ultima V conversation file family.
type OriginalSource string

const (
	// OriginalSourceCastle identifies CASTLE.TLK content.
	OriginalSourceCastle OriginalSource = "castle"
	// OriginalSourceTowne identifies TOWNE.TLK content.
	OriginalSourceTowne OriginalSource = "towne"
	// OriginalSourceDwelling identifies DWELLING.TLK content.
	OriginalSourceDwelling OriginalSource = "dwelling"
	// OriginalSourceKeep identifies KEEP.TLK content.
	OriginalSourceKeep OriginalSource = "keep"
)

// OriginalOrigin identifies a one-based dialog in an original conversation file.
type OriginalOrigin struct {
	Source OriginalSource
	Dialog int
}

// Kind returns OriginKindOriginal.
func (*OriginalOrigin) Kind() OriginKind { return OriginKindOriginal }
func (*OriginalOrigin) isOrigin()        {}

// ReduxOrigin identifies a conversation authored for Ultima 5 Redux.
type ReduxOrigin struct{}

// Kind returns OriginKindRedux.
func (*ReduxOrigin) Kind() OriginKind { return OriginKindRedux }
func (*ReduxOrigin) isOrigin()        {}

// Topic is an ordered set of equivalent keywords and its response.
type Topic struct {
	Keywords []string
	Response Body
}

// Question is a named prompt with ordered answers and a fallback response.
type Question struct {
	Prompt Body
	// Answers may be empty when only protocol-level matching and Default apply.
	Answers []Answer
	Default Body
}

// Answer is an ordered set of equivalent response keywords and its body.
type Answer struct {
	Keywords []string
	Response Body
}

// Body is an ordered sequence of semantic actions.
type Body []Action

// ActionKind identifies a semantic conversation action.
type ActionKind string

const (
	// ActionKindSay identifies NPC speech.
	ActionKindSay ActionKind = "say"
	// ActionKindNarrate identifies narration.
	ActionKindNarrate ActionKind = "narrate"
	// ActionKindAsk identifies a named question transfer.
	ActionKindAsk ActionKind = "ask"
	// ActionKindBranch identifies a conditional body.
	ActionKindBranch ActionKind = "branch"
	// ActionKindTakeGold identifies a gold transaction.
	ActionKindTakeGold ActionKind = "take_gold"
	// ActionKindGiveItem identifies an item grant.
	ActionKindGiveItem ActionKind = "give_item"
	// ActionKindAdjustKarma identifies a karma adjustment.
	ActionKindAdjustKarma ActionKind = "adjust_karma"
	// ActionKindDiscoverKnowledge identifies a persistent knowledge discovery.
	ActionKindDiscoverKnowledge ActionKind = "discover_knowledge"
	// ActionKindPause identifies the protocol pause behavior.
	ActionKindPause ActionKind = "pause"
	// ActionKindWaitForKey identifies a key acknowledgement wait.
	ActionKindWaitForKey ActionKind = "wait_for_key"
	// ActionKindAskName identifies the protocol party-name prompt.
	ActionKindAskName ActionKind = "ask_name"
	// ActionKindJoinParty identifies a party-join attempt.
	ActionKindJoinParty ActionKind = "join_party"
	// ActionKindCallGuards identifies the standard guard outcome.
	ActionKindCallGuards ActionKind = "call_guards"
	// ActionKindGoToJail identifies the standard jail outcome.
	ActionKindGoToJail ActionKind = "go_to_jail"
	// ActionKindBeginCircleStudy ends the talk so the host can open circle study.
	ActionKindBeginCircleStudy ActionKind = "begin_circle_study"
	// ActionKindReturnToTopics identifies a return to the topic prompt.
	ActionKindReturnToTopics ActionKind = "return_to_topics"
	// ActionKindEnd identifies the end of a conversation.
	ActionKindEnd ActionKind = "end"
)

// Action is the closed union of semantic conversation actions.
type Action interface {
	Kind() ActionKind
	isAction()
}

// SayAction emits one paragraph of NPC speech.
type SayAction struct{ Content RichText }

// Kind returns ActionKindSay.
func (*SayAction) Kind() ActionKind { return ActionKindSay }
func (*SayAction) isAction()        {}

// NarrateAction emits one paragraph of narration.
type NarrateAction struct{ Content RichText }

// Kind returns ActionKindNarrate.
func (*NarrateAction) Kind() ActionKind { return ActionKindNarrate }
func (*NarrateAction) isAction()        {}

// AskAction transfers control to a named question.
type AskAction struct{ Question string }

// Kind returns ActionKindAsk.
func (*AskAction) Kind() ActionKind { return ActionKindAsk }
func (*AskAction) isAction()        {}

// BranchAction selects a body from a semantic condition.
type BranchAction struct {
	Condition Condition
	Then      Body
	Otherwise *Body
}

// Kind returns ActionKindBranch.
func (*BranchAction) Kind() ActionKind { return ActionKindBranch }
func (*BranchAction) isAction()        {}

// TakeGoldAction takes a positive amount of gold using protocol outcomes.
type TakeGoldAction struct{ Amount int }

// Kind returns ActionKindTakeGold.
func (*TakeGoldAction) Kind() ActionKind { return ActionKindTakeGold }
func (*TakeGoldAction) isAction()        {}

// GiveItemAction grants a semantic item quantity using protocol outcomes.
type GiveItemAction struct {
	Item     string
	Quantity int
}

// Kind returns ActionKindGiveItem.
func (*GiveItemAction) Kind() ActionKind { return ActionKindGiveItem }
func (*GiveItemAction) isAction()        {}

// AdjustKarmaAction applies a non-zero karma delta.
type AdjustKarmaAction struct{ Delta int }

// Kind returns ActionKindAdjustKarma.
func (*AdjustKarmaAction) Kind() ActionKind { return ActionKindAdjustKarma }
func (*AdjustKarmaAction) isAction()        {}

// DiscoverKnowledgeAction records a stable knowledge discovery without emitting output.
type DiscoverKnowledgeAction struct{ Knowledge KnowledgeID }

// Kind returns ActionKindDiscoverKnowledge.
func (*DiscoverKnowledgeAction) Kind() ActionKind { return ActionKindDiscoverKnowledge }
func (*DiscoverKnowledgeAction) isAction()        {}

// PauseAction requests the protocol pause behavior.
type PauseAction struct{}

// Kind returns ActionKindPause.
func (*PauseAction) Kind() ActionKind { return ActionKindPause }
func (*PauseAction) isAction()        {}

// WaitForKeyAction waits for protocol key acknowledgement.
type WaitForKeyAction struct{}

// Kind returns ActionKindWaitForKey.
func (*WaitForKeyAction) Kind() ActionKind { return ActionKindWaitForKey }
func (*WaitForKeyAction) isAction()        {}

// AskNameAction asks for the party name using the protocol prompt.
type AskNameAction struct{}

// Kind returns ActionKindAskName.
func (*AskNameAction) Kind() ActionKind { return ActionKindAskName }
func (*AskNameAction) isAction()        {}

// JoinPartyAction attempts to join the party using protocol outcomes.
type JoinPartyAction struct{}

// Kind returns ActionKindJoinParty.
func (*JoinPartyAction) Kind() ActionKind { return ActionKindJoinParty }
func (*JoinPartyAction) isAction()        {}

// CallGuardsAction invokes the standard guard outcome.
type CallGuardsAction struct{}

// Kind returns ActionKindCallGuards.
func (*CallGuardsAction) Kind() ActionKind { return ActionKindCallGuards }
func (*CallGuardsAction) isAction()        {}

// GoToJailAction invokes the standard jail outcome.
type GoToJailAction struct{}

// Kind returns ActionKindGoToJail.
func (*GoToJailAction) Kind() ActionKind { return ActionKindGoToJail }
func (*GoToJailAction) isAction()        {}

// BeginCircleStudyAction ends the conversation and asks the host to open circle
// study once the talk has closed. REDUX DEVIATION: PROGRESS-0005.
type BeginCircleStudyAction struct{}

// Kind returns ActionKindBeginCircleStudy.
func (*BeginCircleStudyAction) Kind() ActionKind { return ActionKindBeginCircleStudy }
func (*BeginCircleStudyAction) isAction()        {}

// ReturnToTopicsAction returns to the protocol topic prompt.
type ReturnToTopicsAction struct{}

// Kind returns ActionKindReturnToTopics.
func (*ReturnToTopicsAction) Kind() ActionKind { return ActionKindReturnToTopics }
func (*ReturnToTopicsAction) isAction()        {}

// EndAction ends the conversation.
type EndAction struct{}

// Kind returns ActionKindEnd.
func (*EndAction) Kind() ActionKind { return ActionKindEnd }
func (*EndAction) isAction()        {}

// ConditionKind identifies a supported branch condition.
type ConditionKind string

const (
	// ConditionKindNPCKnowsPartyName identifies the party-name knowledge test.
	ConditionKindNPCKnowsPartyName ConditionKind = "npc_knows_party_name"
	// ConditionKindKarmaAtLeast identifies the karma threshold test.
	ConditionKindKarmaAtLeast ConditionKind = "karma_at_least"
)

// Condition is the closed union of v1 branch conditions.
type Condition interface {
	Kind() ConditionKind
	isCondition()
}

// NPCKnowsPartyNameCondition tests whether the NPC knows the party name.
type NPCKnowsPartyNameCondition struct{}

// Kind returns ConditionKindNPCKnowsPartyName.
func (*NPCKnowsPartyNameCondition) Kind() ConditionKind {
	return ConditionKindNPCKnowsPartyName
}
func (*NPCKnowsPartyNameCondition) isCondition() {}

// KarmaAtLeastCondition tests the party's karma against a threshold.
type KarmaAtLeastCondition struct{ Minimum int }

// Kind returns ConditionKindKarmaAtLeast.
func (*KarmaAtLeastCondition) Kind() ConditionKind { return ConditionKindKarmaAtLeast }
func (*KarmaAtLeastCondition) isCondition()        {}

// RichText is an ordered sequence of ordinary, runic, and line-break parts.
type RichText []InlinePart

// InlineKind identifies a rich-text part.
type InlineKind string

const (
	// InlineKindText identifies ordinary text.
	InlineKindText InlineKind = "text"
	// InlineKindRunic identifies runic text.
	InlineKindRunic InlineKind = "runic"
	// InlineKindLineBreak identifies an inline line break.
	InlineKindLineBreak InlineKind = "line_break"
)

// InlinePart is the closed union of rich-text parts.
type InlinePart interface {
	Kind() InlineKind
	isInlinePart()
}

// TextPart contains ordinary conversation text.
type TextPart struct{ Text string }

// Kind returns InlineKindText.
func (*TextPart) Kind() InlineKind { return InlineKindText }
func (*TextPart) isInlinePart()    {}

// RunicPart contains text rendered in runic script.
type RunicPart struct{ Text string }

// Kind returns InlineKindRunic.
func (*RunicPart) Kind() InlineKind { return InlineKindRunic }
func (*RunicPart) isInlinePart()    {}

// LineBreakPart inserts an inline line break without creating another action.
type LineBreakPart struct{}

// Kind returns InlineKindLineBreak.
func (*LineBreakPart) Kind() InlineKind { return InlineKindLineBreak }
func (*LineBreakPart) isInlinePart()    {}
