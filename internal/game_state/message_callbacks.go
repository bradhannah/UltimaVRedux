package game_state

// MessageStyle identifies the semantic presentation of a message segment.
type MessageStyle int

// Styles used by spoken feedback and its quotation marks and emphasis.
const (
	MessageSpoken MessageStyle = iota
	MessageQuotation
	MessageHighlight
)

// MessageSegment is detached text and styling, not a renderer or output handle.
type MessageSegment struct {
	Text  string
	Style MessageStyle
}

// StyledMessage appends one styled row. LeadingBlank adds a spacing row only to
// the primary presentation, not to a secondary plain-text transcript.
type StyledMessage struct {
	Segments     []MessageSegment
	LeadingBlank bool
}

// MessageCallbacks provides dependency injection for UI message output functions.
// This allows game logic to output messages without depending on UI implementation.
// Use NewMessageCallbacks() to create with validation.
type MessageCallbacks struct {
	// AddRowStr adds a new line of text to the message output window
	AddRowStr func(message string)

	// AppendToCurrentRowStr appends text to the current line in the message output window
	AppendToCurrentRowStr func(message string)

	// ShowCommandPrompt displays a command prompt (e.g., "Push-", "Look-")
	ShowCommandPrompt func(command string)

	// AppendStyledMessage presents a complete styled row without exposing its renderer.
	AppendStyledMessage func(message StyledMessage)
}

// NewMessageCallbacks validates a complete named set of message capabilities.
func NewMessageCallbacks(callbacks MessageCallbacks) MessageCallbacks {
	if callbacks.AddRowStr == nil {
		panic("MessageCallbacks.AddRowStr is required - cannot be nil")
	}
	if callbacks.AppendToCurrentRowStr == nil {
		panic("MessageCallbacks.AppendToCurrentRowStr is required - cannot be nil")
	}
	if callbacks.ShowCommandPrompt == nil {
		panic("MessageCallbacks.ShowCommandPrompt is required - cannot be nil")
	}
	if callbacks.AppendStyledMessage == nil {
		panic("MessageCallbacks.AppendStyledMessage is required - cannot be nil")
	}

	return callbacks
}
