package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// Sign dialog layout constants
const (
	signDialogStartX = 0.10 // Left edge as percentage of game area
	signDialogEndX   = 0.65 // Right edge as percentage of game area
	signDialogStartY = 0.20 // Top edge as percentage of game area
	signDialogEndY   = 0.80 // Bottom edge as percentage of game area
	// Original sign fallback, including its data-defined frame (ref:SIGN-0004).
	defaultSignText = "abbbbbbbbbbbbbbc\n" +
		"g              g\n" + //nolint:dupword // Frame glyph codes, not prose.
		"g  LIVE@BY@[E  g\n" +
		"g  EIGHT@LAWS  g\n" +
		"g              g\n" + //nolint:dupword // Frame glyph codes, not prose.
		"deeeeeeeeeeeeeef\n"
)

// ShowSignDialog displays a sign using runic/normal bitmap fonts.
// This is the SystemCallbacks.UI.ShowSignDialog implementation.
func (g *GameScene) ShowSignDialog(sign *references.SignRecord) {
	logging.Debugf(logging.CategoryUI, "ShowSignDialog: called")
	g.startSignDialogWorkflow(sign)
}

// startSignDialogWorkflow initiates the sign display workflow using event queue.
func (g *GameScene) startSignDialogWorkflow(sign *references.SignRecord) {
	logging.Debugf(logging.CategoryUI, "startSignDialogWorkflow: starting event queue")
	var signDialog *widgets.SignDialog

	g.gameState.EventQueueManager.Workflow("sign_dialog_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			logging.Debugf(logging.CategoryUI, "sign_dialog_workflow.First: creating dialog")

			var rawText []byte
			if sign != nil && len(sign.RawText) > 0 {
				rawText = sign.RawText
				logging.Debugf(logging.CategoryUI, "sign_dialog_workflow.First: using sign text (%d bytes)", len(rawText))
			} else {
				rawText = []byte(defaultSignText)
				logging.Debugf(logging.CategoryUI, "sign_dialog_workflow.First: using default text")
			}

			signDialog = widgets.NewSignDialog(widgets.SignDialogConfig{
				Placement: sprites.PercentBasedPlacement{
					StartPercentX: signDialogStartX,
					EndPercentX:   signDialogEndX,
					StartPercentY: signDialogStartY,
					EndPercentY:   signDialogEndY,
				},
				Keyboard: g.keyboard,
				RawText:  rawText,
			})

			logging.Debugf(logging.CategoryUI, "sign_dialog_workflow.First: pushing dialog to stack")
			g.dialogStack.PushModalDialog(signDialog)
		}).
		While(
			func() bool { return !signDialog.IsDismissed() && g.dialogStack.ContainsWidgetInstance(signDialog) },
			func() {

			},
			DialogPollingRateMs,
		).Finally(

		func() {
			logging.Debugf(logging.CategoryUI, "sign_dialog_workflow.Last: cleanup")

			if signDialog != nil {
				signDialog.SetTranslated(false)
			}
			if g.runicReference.owner == signDialog {
				g.closeRunicReference()
			}
			g.dialogStack.RemoveWidgetInstance(signDialog)
			g.keyboard.PrepareForTransition(widgets.KeyboardTransitionDelayMs)
		}).
		Start()

	logging.Debugf(logging.CategoryUI, "startSignDialogWorkflow: event queue started")
}
