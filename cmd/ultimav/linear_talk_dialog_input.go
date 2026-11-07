package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/hajimehoshi/ebiten/v2"
)

// talkPauseDurationMs is how long a conversation pause marker holds the output (half of the original 3 seconds).
const talkPauseDurationMs = 1500

// Update method (simplified from original since no channel handling needed)
func (d *LinearTalkDialog) Update() {
	if d.closed || (d.callbacks != nil && !d.mounted) {
		return
	}
	if d.mounted && !d.isMounted() {
		d.discardPendingKnowledge()

		return
	}
	d.ensureTalkLayout()
	d.expireExcerptNotice()
	// Simple Notes Mode hides Excerpts, so Tab does not keep passages there.
	if !d.gameScene.gameConfig.GetSimpleNotesMode() && d.gameScene.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab) {
		d.keepPresentedExchange()
		d.gameScene.keyboard.PrepareForTransition(0)

		return
	}
	if d.updateHistoryInput() {
		return
	}
	// KeyWait (0x8F) is processed before dismissal: both block input, but KeyWait continues the conversation.
	if d.waitingForKeypressContinue {
		d.updateKeyWait()

		return
	}
	if d.waitingForDismissal {
		d.updateDismissal()

		return
	}

	// Block input during pause animation
	if !d.isPausing {
		d.TextInput.Update()
	}

	// Handle pause animation timing (no visual feedback during pause)
	if d.isPausing {
		elapsedMs := d.gameScene.gameState.ElapsedMs - d.pauseStartMs

		if elapsedMs >= talkPauseDurationMs {
			// Pause complete - finish it and process next marker if any
			d.finishCurrentPause()

			return // Exit early to avoid processing accumulated output during pause
		}
	}

	// Check if engine has any pending output to display (only when not pausing)
	if !d.isPausing {
		d.flushAccumulatedOutput()
	}
}

// flushAccumulatedOutput appends any output the engine callbacks have accumulated.
func (d *LinearTalkDialog) flushAccumulatedOutput() {
	if d.callbacks == nil {
		return
	}
	if accumulatedOutput := d.callbacks.GetAccumulatedOutput(); accumulatedOutput != "" {
		d.Output.AppendText(accumulatedOutput)
		d.callbacks.ClearAccumulatedOutput()
	}
}

// freshKeypressWhileBlocked reports whether a new key was pressed while input is blocked. It first waits
// for all initially-pressed keys to be released.
func (d *LinearTalkDialog) freshKeypressWhileBlocked(releasedMessage string) bool {
	keyboard := d.gameScene.keyboard
	if keyboard.IsWaitingForRelease() {
		keyboard.TryAcceptKey(ebiten.KeySpace) // Triggers the release state update
		if keyboard.IsWaitingForRelease() {
			return false
		}
		logging.Debugf(logging.CategoryUI, "%s", releasedMessage)
	}

	return keyboard.IsAnyKeyPressed()
}

// updateKeyWait blocks input until any key is pressed, then resumes the conversation.
func (d *LinearTalkDialog) updateKeyWait() {
	if !d.freshKeypressWhileBlocked("All keys released, ready for KeyWait keypress") {
		return
	}
	logging.Debugf(logging.CategoryUI, "New keypress detected in KeyWait state, continuing conversation")

	// Mark pressed keys as consumed so they do not bleed into underlying systems; release-based tracking cleans up.
	d.consumeCurrentlyPressedKeys()
	d.waitingForKeypressContinue = false

	// The semantic engine resumes its suspended action frames.
	d.currentResponse = d.engine.ProcessInput("")
	d.processCurrentResponse()
}

// updateDismissal closes the dialog on a fresh keypress once the dismissal wait is active.
func (d *LinearTalkDialog) updateDismissal() {
	if !d.freshKeypressWhileBlocked("All keys released, ready for new keypress") {
		return
	}
	logging.Debugf(logging.CategoryUI, "New keypress detected in dismissal state, closing dialog")

	// Mark pressed keys as consumed so the dismissal key does not bleed into underlying game input.
	d.consumeCurrentlyPressedKeys()
	d.Close()
}
