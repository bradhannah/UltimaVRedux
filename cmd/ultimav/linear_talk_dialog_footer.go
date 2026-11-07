package main

import (
	"image"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	talkFooterPaddingUnits  = 2
	excerptNoticeDurationMs = 3000
)

// Take the footer from the bottom of the card, beneath the continuous transcript.
// The card stays inside the map frame, so the transcript yields the footer's rows.
func (d *LinearTalkDialog) layoutWithFooter(bounds image.Rectangle) linearTalkDialogLayout {
	layout := calculateLinearTalkDialogLayout(bounds)
	padding := layout.pixelUnit * talkFooterPaddingUnits
	height := int(math.Ceil(d.footerOutput.GetTotalHeightForLines(d.footerLines()))) + padding*2 + layout.pixelUnit
	transcriptBottom := layout.frameRect.Max.Y - height
	layout.continuousTranscriptRect.Max.Y = transcriptBottom - layout.pixelUnit*linearTalkContentVerticalPaddingUnits
	layout.footerDividerY = transcriptBottom - layout.pixelUnit
	layout.footerRect = image.Rect(layout.continuousTranscriptRect.Min.X, transcriptBottom+padding,
		layout.continuousTranscriptRect.Max.X, layout.frameRect.Max.Y-padding-layout.pixelUnit)

	return layout
}

// footerLines keeps one row across input, key waits and capture confirmation,
// so inline typing never jumps when a hint changes. History review and capture
// failures need a second row and take it from the transcript.
func (d *LinearTalkDialog) footerLines() int {
	if d.historyPanel != nil || d.excerptFailed() {
		return talkFooterLines
	}

	return 1
}

// excerptFailed reports a retained capture failure, which offers Retry.
func (d *LinearTalkDialog) excerptFailed() bool {
	return d.excerptNotice != "" && d.excerptNotice != "Excerpt kept"
}

func (d *LinearTalkDialog) conversationFooterGuidance(width int) string {
	acknowledge := ""
	if d.historyPanel == nil && d.waitingForKeypressContinue {
		acknowledge = "[Space] Continue   "
	} else if d.historyPanel == nil && d.waitingForDismissal {
		acknowledge = "[Space] Close   "
	}
	if d.excerptFailed() {
		// Keep the failure's prefix readable without allowing an OS path or other
		// long diagnostic to overflow the frame. Retry retains the interaction.
		notice := widgets.FitDialogLine(d.excerptNotice, d.footerOutput.Font, width)

		return notice + "\n[Tab] Retry"
	}
	keep := "[Tab] Keep excerpt   "
	if d.excerptNotice == "Excerpt kept" {
		keep = "[Tab] Excerpt kept   "
	}
	if d.gameScene.gameConfig.GetSimpleNotesMode() {
		keep = "" // Excerpts is hidden in Simple Notes Mode.
	}
	if d.historyPanel != nil {
		return "[Up/Down] Exchange   [Shift+Up/Down/PgUp/PgDn] Read   " + keep + "[Esc] Back"
	}

	// History is left out so these hints fit one row; command help lists PgUp.
	return acknowledge + keep + "[Shift+J] Journal"
}

func (d *LinearTalkDialog) drawConversationFooter(screen *ebiten.Image, layout linearTalkDialogLayout) {
	vector.FillRect(screen, float32(layout.frameRect.Min.X), float32(layout.footerDividerY),
		float32(layout.frameRect.Dx()), float32(layout.pixelUnit), widgets.DialogRuleColor, false)
	widgets.DrawKeyboardFooter(screen, layout.footerRect, d.conversationFooterGuidance(layout.footerRect.Dx()),
		d.footerOutput)
}

func (d *LinearTalkDialog) setExcerptNotice(message string, brief bool) {
	d.excerptNotice = message
	d.excerptNoticeUntilMs = 0
	if brief {
		d.excerptNoticeUntilMs = d.gameScene.gameState.ElapsedMs + excerptNoticeDurationMs
	}
}

func (d *LinearTalkDialog) expireExcerptNotice() {
	if d.excerptNoticeUntilMs > 0 && d.gameScene.gameState.ElapsedMs >= d.excerptNoticeUntilMs {
		d.setExcerptNotice("", false)
	}
}
