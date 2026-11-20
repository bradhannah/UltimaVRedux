package conversation

// TextSegment represents ordinary or runic conversation text.
type TextSegment struct {
	Text    string
	IsRunic bool
}

const (
	quoteOpen  = "\""
	quoteClose = "\""

	// PauseMarker marks a cosmetic timed pause in the presented response.
	PauseMarker = "⏸"

	promptInterest = "Your interest?"
	// REDUX DEVIATION: Original questions print "You respond-\n:" (ref:TALK-0004). Redux
	// keeps the prompt on one line because input lives in a separate panel.
	promptRespond = "You respond:"
	promptName    = "What is thy name?"

	responseNamePrefix = "My name is "
	responseNoHelp     = "I cannot help thee with that."
	responsePleasure   = "A pleasure!"
	responseIfYouSay   = "If you say so..."
)

// JoinPartyOutcome reports the runtime result of a recruitment attempt.
type JoinPartyOutcome uint8

const (
	// JoinPartySuccess indicates that the NPC joined and the conversation ends.
	JoinPartySuccess JoinPartyOutcome = iota
	// JoinPartyCapacityRefused indicates an ordinary full-party refusal.
	JoinPartyCapacityRefused
	// JoinPartyUnexpectedError indicates a failed recruitment without a returned error.
	JoinPartyUnexpectedError
)

// ActionCallbacks connects conversation actions to authoritative gameplay.
//
//nolint:interfacebloat // Actions and actor-state queries form one conversation host contract.
type ActionCallbacks interface {
	JoinParty() (JoinPartyOutcome, error)
	CallGuards()
	IncreaseKarma()
	DecreaseKarma()
	GoToJail()
	// BeginCircleStudy records that the host should open circle study once the
	// talk has closed; ref:PROGRESS-0005.
	BeginCircleStudy()
	GiveItem(itemID int)
	DeductGoldSimple(amount int, npcName string) error
	TimedPause()
	HasMet(actorID int) bool
	GetAvatarName() string
	GetKarmaLevel() int
	SetMet(actorID int)
}

// ConversationResponse is one semantic step consumed by the Talk presentation.
//
//nolint:revive // Retain the established response type shared with the production Talk presentation.
type ConversationResponse struct {
	// RevealedName is populated only by an explicit NAME response. Presentation
	// waits for the complete response before using it as attribution.
	RevealedName                string
	Serial                      uint64
	Output                      string
	Segments                    []TextSegment
	NeedsInput                  bool
	InputPrompt                 string
	IsComplete                  bool
	IsCompleteAwaitingDismissal bool
	Error                       error
	WaitingForKeypress          bool
	IsLabelQuestion             bool
	ContinuesPreviousOutput     bool
}
