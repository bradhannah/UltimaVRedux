package main

import (
	"fmt"
	"strings"

	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/conversation"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ultimacolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

var _ widgets.Widget = (*LinearTalkDialog)(nil)
var _ game_state.TalkDialog = (*LinearTalkDialog)(nil)

const (
	// Talk uses the standard body size with extra line spacing for readability.
	talkFontSize              = widgets.FontSizeBody
	talkLineSpacingMultiplier = 1.3
	talkContinuousMaxLines    = 14
	talkMaxCharsInput         = 45
	talkHistoryMaxEntries     = 256
	talkHistoryMaxBytes       = 256 * 1024
	talkFooterLines           = 2
)

const (
	talkPercentOffEdge          = 0.02
	talkCardGapPercentY         = 0.015
	percentTextIndentFromBorder = 0.017
)

const (
	borderStartPercentX = talkPercentOffEdge
	borderEndPercentX   = .75 + .01 - talkPercentOffEdge
	// Start below the avatar and the tile beneath it, so the speaker stays visible.
	borderStartPercentY = float64(yCenter+2)/yTilesVisibleOnGameScreen + talkCardGapPercentY
	// End just inside the map frame, whose inner edge is about 98% down the screen.
	borderEndPercentY = 0.98 - talkCardGapPercentY
)

const (
	textRectStartPercentX = talkPercentOffEdge + percentTextIndentFromBorder
	textRectEndPercentX   = .75 + .01 - talkPercentOffEdge
)
const (
	textInputStartPercentX = textRectStartPercentX
	textInputEndPercentX   = textRectEndPercentX
	textInputStartPercentY = 0.88   // Maintains 3.5% spacing from input border top
	textInputEndPercentY   = 0.9225 // 15% smaller height (0.0425 vs 0.05)
)

const (
	maxPauseSplits = 2 // Split at first pause marker only (before + after)
	// UI text constants
	interestPlaceholder = " name, job, bye" // Topic suggestions; controls belong to the styled footer.
)

// LinearTalkDialog presents one stateful conversation engine.
type LinearTalkDialog struct {
	historyNavigation input.ListNavigation
	font              *text.UltimaFont
	Output            *text.Output

	TextInput *widgets.TextInput

	gameScene *GameScene

	npcReference references.NPCReference
	actorID      int
	engine       conversation.Engine
	callbacks    *GameActionCallbacks

	// State management for the linear system
	waitingForInput            bool
	waitingForKeypress         bool // Passive flag copied from response.WaitingForKeypress
	waitingForKeypressContinue bool // Active UI state: blocked for KeyWait (0x8F), waiting for any key to continue
	waitingForDismissal        bool // True when conversation complete, showing "Press any key to close"
	conversationCompleted      bool
	currentResponse            *conversation.ConversationResponse

	// Pause animation state (Phase 3)
	isPausing            bool                       // Currently animating a pause
	pauseStartMs         int64                      // When current pause animation started
	animatedText         string                     // Text displayed before current pause marker
	pendingOutput        string                     // Output waiting after current pause
	pendingSegments      []conversation.TextSegment // Segments waiting after current pause (for runic support)
	pendingContinues     bool                       // Pending pause output continues the current insertion row
	currentPauseIndex    int                        // Which pause marker we're on (for multiple sequential pauses)
	lastScreenWidth      int
	lastScreenHeight     int
	historyPanel         *text.Output
	footerOutput         *text.Output
	historyRow           int
	historyStartedMs     int64
	presentedSourceBytes int
	mounted              bool
	closed               bool
	conversationID       string
	exchangeNumber       int
	exchange             journal.Exchange
	excerptNotice        string
	excerptNoticeUntilMs int64
	conversationStarted  bool
	historyExchange      int
	knownSpeaker         string
}

func NewLinearTalkDialog(gameScene *GameScene, npc *map_units.NPCFriendly) *LinearTalkDialog {
	if gameScene == nil || gameScene.gameState == nil || npc == nil ||
		gameScene.gameState.GameReferences == nil ||
		gameScene.gameState.GameReferences.ConversationCatalog == nil {
		return nil
	}
	dialog := newSemanticTalkDialog(gameScene, npc)
	if dialog == nil {
		return nil
	}

	// Initialize UI elements (borrowed from original TalkDialog) with error handling
	defer func() {
		if r := recover(); r != nil {
			// If UI initialization fails, return nil instead of crashing
			dialog = nil
		}
	}()

	dialog.initializeResizeableVisualElements()

	// Start the conversation with error handling
	defer func() {
		if r := recover(); r != nil {
			// If conversation startup fails, return nil instead of crashing
			dialog = nil
		}
	}()

	dialog.startConversation()

	return dialog
}

func newSemanticTalkDialog(gameScene *GameScene, npc *map_units.NPCFriendly) *LinearTalkDialog {
	npcRef := npc.NPCReference
	dialogNumber := references.DialogNumber(npcRef.DialogNumber)
	if dialogNumber == references.NoResponse || dialogNumber.IsSpecial() {
		return nil
	}
	currentLocation := gameScene.gameState.MapState.PlayerLocation.Location
	source, exists := semanticSourceForMapType(references.GetMapMasterFromLocation(currentLocation))
	if !exists {
		return nil
	}
	program, exists := gameScene.gameState.GameReferences.ConversationCatalog.Original(source, int(npcRef.DialogNumber))
	if !exists {
		return nil
	}
	definition, err := program.Definition()
	if err != nil {
		panic(fmt.Sprintf("loading validated conversation %q: %v", program.ID(), err))
	}
	npcRef.Position = npc.Pos()
	callbacks := NewGameActionCallbacks(gameScene, npcRef, npc)
	engine, err := conversation.NewDeferredSemanticConversationEngine(definition, callbacks, false)
	if err != nil {
		panic(fmt.Sprintf("creating semantic conversation engine %q: %v", program.ID(), err))
	}
	callbacks.SetNPCName(engine.ExtractNPCName())

	return &LinearTalkDialog{ //nolint:exhaustruct_v5 // UI state is initialized immediately after construction.
		gameScene: gameScene, npcReference: npcRef, actorID: npc.MapUnitDetails().NPCNum,
		engine: engine, callbacks: callbacks,
	}
}

func semanticSourceForMapType(mapType references.SmallMapMasterTypes) (semantic.OriginalSource, bool) {
	switch mapType {
	case references.Castle:
		return semantic.OriginalSourceCastle, true
	case references.Towne:
		return semantic.OriginalSourceTowne, true
	case references.Dwelling:
		return semantic.OriginalSourceDwelling, true
	case references.Keep:
		return semantic.OriginalSourceKeep, true
	case references.Dungeon, references.CutOrIntroScene, references.None:
		return "", false
	}

	return "", false
}

// initializeResizeableVisualElements rebuilds resolution-dependent text resources.
func (d *LinearTalkDialog) initializeResizeableVisualElements() {
	d.lastScreenWidth, d.lastScreenHeight = display.GetManager().GetCurrentSize()
	d.reflowTranscriptOutput()

	// Share the transcript font and spacing so controls never outsize speech.
	d.footerOutput = text.NewOutput(d.font, d.Output.GetLineSpacing(), talkFooterLines, talkMaxCharsInput)
	d.Output.SetDebugOptions(&d.gameScene.gameState.DebugOptions)
	if d.callbacks != nil {
		d.callbacks.SetDialogOutput(d.Output)
	}
	d.initializeConversationTextInput()
}

func (d *LinearTalkDialog) reflowTranscriptOutput() {
	maxLines := talkContinuousMaxLines
	if d.Output == nil {
		d.font, d.Output = widgets.GetScaledFontWithOutput(widgets.FontConfig{
			Size:                  talkFontSize,
			LineSpacing:           0,
			LineSpacingMultiplier: talkLineSpacingMultiplier,
			MaxLines:              maxLines,
			MaxChars:              talkMaxCharsInput,
		})
		d.Output.RetainLogicalHistory(talkHistoryMaxEntries, talkHistoryMaxBytes)
	} else {
		font, output := widgets.GetScaledFontWithOutput(widgets.FontConfig{
			Size:                  talkFontSize,
			LineSpacing:           0,
			LineSpacingMultiplier: talkLineSpacingMultiplier,
			MaxLines:              maxLines,
			MaxChars:              talkMaxCharsInput,
		})
		d.font = font
		d.Output.ReflowRetained(font, output.GetLineSpacing(), talkMaxCharsInput)
		d.reflowConversationHistory()
	}
}

func (d *LinearTalkDialog) reflowConversationHistory() {
	if d.historyPanel == nil {
		return
	}
	row := d.historyRow
	if len(d.gameScene.lastConversation.Exchanges) > 0 {
		d.showJournalHistoryExchange()
	} else {
		d.historyPanel = d.Output.RetainedHistoryPanel()
	}
	d.historyRow = min(row, max(0, d.historyPanel.GetPanelContentLineCount()-1))
}

func (d *LinearTalkDialog) initializeConversationTextInput() {
	if d.TextInput == nil {
		d.TextInput = widgets.NewTextInput(
			sprites.PercentBasedPlacement{
				StartPercentX: textInputStartPercentX,
				EndPercentX:   textInputEndPercentX,
				StartPercentY: textInputStartPercentY,
				EndPercentY:   textInputEndPercentY,
			},
			widgets.GetScaledLineHeight(talkFontSize),
			talkMaxCharsInput,
			nil,
			widgets.TextInputCallbacks{
				AmbiguousAutoComplete: func(message string) {
					d.Output.AddTextRaw(message)
				},
				OnEnter: d.onEnter, OnCommandExecuted: nil, ShowCommandList: nil,
			},
			d.gameScene.keyboard)
		d.TextInput.SetFreeTextMode()
		d.TextInput.SetInputColors(widgets.TextInputColors{
			DefaultColor:          ultimacolor.TextInputColor,
			NoMatchesColor:        ultimacolor.TextInputColor,
			OneMatchColor:         ultimacolor.TextInputColor,
			MoreThanOneMatchColor: ultimacolor.TextInputColor,
		})
	} else {
		d.TextInput.SetFontPoint(widgets.GetScaledLineHeight(talkFontSize))
	}
}

// startConversation initiates the conversation and displays initial output
func (d *LinearTalkDialog) startConversation() {
	d.beginJournalExchange("")
	d.currentResponse = d.engine.Start(d.actorID)
	d.processCurrentResponse()
}

// processCurrentResponse handles the current response from the engine
func (d *LinearTalkDialog) processCurrentResponse() {
	if d.closed || d.currentResponse == nil {
		return
	}

	// Display any output using segment-based or string-based rendering
	d.presentedSourceBytes = 0
	d.displayResponseOutput()
	d.commitPresentedKnowledge()
	if d.isPausing {
		return
	}
	d.finishResponsePresentation()
}

func (d *LinearTalkDialog) finishResponsePresentation() {
	if d.currentResponse.RevealedName != "" {
		d.knownSpeaker = d.currentResponse.RevealedName
		if d.callbacks != nil {
			d.callbacks.knownSpeaker = d.knownSpeaker
		}
		d.exchange.Speaker = d.knownSpeaker
		d.publishPresentedExchange()
		if err := d.gameScene.attributeKeptConversation(d.conversationID, d.knownSpeaker); err != nil {
			d.setExcerptNotice("Unable to update kept excerpts.", false)
		}
	}
	// Handle errors
	if d.currentResponse.Error != nil {
		// we will not print the error - because it is not really an error
		// d.Output.AddTextRaw(fmt.Sprintf(errorOutputFormat, d.currentResponse.Error))
		return
	}

	// Check if conversation is complete but awaiting dismissal (e.g., after JoinParty or BYE)
	if d.currentResponse.IsCompleteAwaitingDismissal {
		d.conversationCompleted = true
		d.enterDismissalWaitState()

		return
	}

	// Check if conversation is complete - remove self from dialog stack (explicit reference)
	if d.currentResponse.IsComplete {
		d.conversationCompleted = true
		d.Close()

		return
	}

	// Update state flags
	d.waitingForInput = d.currentResponse.NeedsInput
	d.waitingForKeypress = d.currentResponse.WaitingForKeypress

	// Check if engine is waiting for keypress (KeyWait command 0x8F)
	// This blocks input until player presses any key, then resumes conversation
	if d.waitingForKeypress {
		d.enterKeyWaitState()

		return
	}

	// Set input prompt prefix if needed (but not during pause animation)
	if !d.isPausing {
		d.setInputPromptFromResponse()
	}
}

// ConversationCompleted reports whether the semantic engine reached a terminal
// response. Dialog removal before that point is an aborted conversation.
func (d *LinearTalkDialog) ConversationCompleted() bool {
	return d != nil && d.conversationCompleted
}

// displayResponseOutput renders the response output, handling runic segments and pause markers.
// Uses segment-based rendering for mixed normal/runic text, falling back to string-based for plain text.
func (d *LinearTalkDialog) displayResponseOutput() {
	if d.currentResponse.Output == "" {
		return
	}

	segments := d.currentResponse.Segments
	useSegments := len(segments) > 0 && hasRunicSegments(segments)

	if useSegments {
		// Segment-based rendering for runic text support
		if beforePause, afterPause, hasPause := splitSegmentsAtPause(segments); hasPause {
			// Start pause animation with segment support
			d.outputSegmentsToDialog(beforePause, d.currentResponse.ContinuesPreviousOutput)
			d.pendingSegments = afterPause
			d.pendingOutput = "" // Clear string-based pending output
			d.pendingContinues = outputContinuesAfterSegments(beforePause, d.currentResponse.ContinuesPreviousOutput)

			d.beginPauseAnimation()

			logging.Debugf(logging.CategoryUI,
				"Started segment-based pause animation, pending: %d segments", len(d.pendingSegments))
		} else {
			// No pause marker - render all segments
			d.outputSegmentsToDialog(segments, d.currentResponse.ContinuesPreviousOutput)
		}

		return
	}
	// Preserve the engine's authored whitespace and first-marker boundary.
	if beforePause, afterPause, hasPause := splitAtFirstPauseMarker(d.currentResponse.Output); hasPause {
		d.animatedText = beforePause
		d.outputTextToDialog(d.animatedText, d.currentResponse.ContinuesPreviousOutput)
		d.pendingOutput = afterPause
		d.pendingSegments = nil
		d.pendingContinues = outputContinuesAfterText(beforePause, d.currentResponse.ContinuesPreviousOutput)
		d.beginPauseAnimation()
		logging.Debugf(logging.CategoryUI, "Started pause animation, pending: %d chars", len(d.pendingOutput))
	} else {
		d.outputTextToDialog(d.currentResponse.Output, d.currentResponse.ContinuesPreviousOutput)
	}
}

// setInputPromptFromResponse sets the input prompt based on current response state.
// Outputs prompt as green text on a new line, ready for user input to be appended.
// Sets placeholder only for "Your interest?" prompt, clears it otherwise.
func (d *LinearTalkDialog) setInputPromptFromResponse() {
	d.TextInput.SetCursorVisible(d.currentResponse.NeedsInput)
	// Always clear TextInput prompt since we're outputting to text area
	d.TextInput.ClearPrompt()

	// Always clear placeholder first
	d.TextInput.ClearPlaceholder()

	if d.currentResponse.NeedsInput {
		if d.currentResponse.InputPrompt != "" {
			d.addInputPrompt(d.currentResponse.InputPrompt)

			// Set placeholder only for normal conversation (not label questions)
			// Use IsLabelQuestion flag instead of string comparison for semantic clarity
			if !d.currentResponse.IsLabelQuestion {
				d.TextInput.SetPlaceholder(interestPlaceholder, ultimacolor.TextPlaceHolderColor)
			}
		}
	}
}

func (d *LinearTalkDialog) addInputPrompt(prompt string) {
	d.Output.AddStyledText([]text.ColorSegment{
		{
			Text: prompt, Color: ultimacolor.EGALightGreen,
			FontType: text.FontTypeNormal, FontScale: text.FontScaleNormal,
		},
	}, etext.AlignStart)
	// REDUX DEVIATION: TALK-0004: original conversation input echoes after a plain colon.
	// Redux uses the classic command marker to distinguish the live response row.
	d.Output.AddStyledPrompt([]text.ColorSegment{
		{
			Text: ">", Color: ultimacolor.EGALightGreen, FontType: text.FontTypeCommandPrompt,
			FontScale: text.FontScaleNormal,
		},
		{
			Text: " ", Color: ultimacolor.EGALightGreen, FontType: text.FontTypeNormal,
			FontScale: text.FontScaleNormal,
		},
	}, etext.AlignStart)
}

// onEnter commits user input to the active prompt or response-marker row.
func (d *LinearTalkDialog) onEnter() {
	str := d.TextInput.GetText()

	// The joined style appends to its prompt row. Continuous styles append to the
	// marker row already underneath the prompt, preserving the live-input position.
	d.Output.AppendText(str)
	// NOTE: Spacing after user input is now handled by the engine, not the dialog UI
	// Removed: d.Output.AddText("\n") - this was causing extra newlines in AskName flow
	// where the engine resets its buffer and produces fresh output

	// Process input through linear engine
	if d.waitingForInput {
		d.beginJournalExchange(str)
		d.currentResponse = d.engine.ProcessInput(str)
		d.processCurrentResponse()
	}

	d.TextInput.ClearText()
}

// splitAtFirstPauseMarker splits text at the first pause marker occurrence.
// Returns the text before the marker, the text after the marker, and whether a marker was found.
func splitAtFirstPauseMarker(value string) (string, string, bool) {
	return strings.Cut(value, conversation.PauseMarker)
}

// hasRunicSegments checks if any segment in the slice requires runic font rendering.
func hasRunicSegments(segments []conversation.TextSegment) bool {
	for _, seg := range segments {
		if seg.IsRunic {
			return true
		}
	}

	return false
}

// splitSegmentsAtPause splits segments at the first pause marker, preserving runic info.
// Returns segments before the pause, segments after the pause, and whether a pause was found.
func splitSegmentsAtPause(
	segments []conversation.TextSegment,
) ([]conversation.TextSegment, []conversation.TextSegment, bool) {
	for index, seg := range segments {
		if strings.Contains(seg.Text, conversation.PauseMarker) {
			// Found pause marker in this segment
			parts := strings.SplitN(seg.Text, conversation.PauseMarker, maxPauseSplits)

			// Build "before" segments: all previous segments + text before pause in current segment
			before := make([]conversation.TextSegment, 0, index+1)
			before = append(before, segments[:index]...)
			if parts[0] != "" {
				before = append(before, conversation.TextSegment{Text: parts[0], IsRunic: seg.IsRunic})
			}

			// Build "after" segments: text after pause in current segment + all following segments
			after := make([]conversation.TextSegment, 0, len(segments)-index)
			if len(parts) > 1 && parts[1] != "" {
				after = append(after, conversation.TextSegment{Text: parts[1], IsRunic: seg.IsRunic})
			}
			after = append(after, segments[index+1:]...)

			return before, after, true
		}
	}
	// No pause marker found
	return segments, nil, false
}

// outputSegmentsToDialog renders text segments to the dialog output using LineBuilder.
// This enables mixed normal/runic text rendering in conversations.
func (d *LinearTalkDialog) outputSegmentsToDialog(segments []conversation.TextSegment, continues bool) {
	if len(segments) == 0 {
		return
	}
	d.retainPresentedExchange(segments, continues)
	defer func() {
		for _, segment := range segments {
			d.presentedSourceBytes += len(segment.Text)
		}
		d.commitPresentedKnowledge()
	}()

	styled := make([]text.ColorSegment, 0, len(segments))
	for _, seg := range segments {
		fontType := text.FontTypeNormal
		if seg.IsRunic {
			fontType = text.FontTypeRunic
		}
		styled = append(styled, text.ColorSegment{
			Text: seg.Text, Color: ultimacolor.EGAWhite, FontType: fontType, FontScale: text.FontScaleNormal,
		})
	}
	if continues {
		d.Output.AppendStyledText(styled)

		return
	}
	d.Output.AddStyledText(styled, etext.AlignStart)
}

func (d *LinearTalkDialog) outputTextToDialog(output string, continues bool) {
	if output == "" {
		return
	}
	d.retainPresentedExchange([]conversation.TextSegment{{Text: output, IsRunic: false}}, continues)
	defer func() {
		d.presentedSourceBytes += len(output)
		d.commitPresentedKnowledge()
	}()
	if continues {
		d.Output.AppendText(output)

		return
	}
	d.Output.AddTextRaw(output)
}

func outputContinuesAfterText(output string, inherited bool) bool {
	if output == "" {
		return inherited
	}

	return !strings.HasSuffix(output, "\n")
}

func outputContinuesAfterSegments(segments []conversation.TextSegment, inherited bool) bool {
	if len(segments) == 0 {
		return inherited
	}

	return outputContinuesAfterText(segments[len(segments)-1].Text, inherited)
}

// AcceptsText reports whether the player is typing a keyword.
func (d *LinearTalkDialog) AcceptsText() bool {
	return d.historyPanel == nil && !d.waitingForKeypressContinue && !d.waitingForDismissal
}
