package main

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func (g *GameScene) handleCombatPartySpellInput() {
	if dx, dy, moved := g.getCombatTargetMovement(); moved {
		g.gameState.MoveCombatSpellCursor(dx, dy)

		return
	}
	if g.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab) {
		g.gameState.CycleCombatPartySpellRecipient()

		return
	}
	if g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		g.gameState.CancelCombatSpellTargeting()
		g.secondaryKeyState = PrimaryInput
		g.keyboard.PrepareForTransition(0)

		return
	}
	if !g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) && !g.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		return
	}
	defer g.combatTimingScope(g.captureCombatTiming())()
	result := g.gameState.ConfirmCombatPartySpellRecipient()
	switch result.Outcome {
	case game_state.CombatSpellTargetInvalid:
		g.ShowActionFeedback("Choose a visible party member.", true)
	case game_state.CombatSpellTargetValidationFailed:
		g.secondaryKeyState = PrimaryInput
		g.ShowActionFeedback(result.Validation.ErrorMsg, true)
		g.keyboard.PrepareForTransition(0)
	case game_state.CombatSpellTargetResolved:
		g.secondaryKeyState = PrimaryInput
		g.keyboard.PrepareForTransition(0)
		g.presentCombatPartySpell(result)
	case game_state.CombatSpellTargetIgnored, game_state.CombatSpellTargetCancelled:
	}
}

func (g *GameScene) presentCombatPartySpell(result game_state.CombatPartySpellResult) {
	timing := g.captureCombatTiming()
	message, sound := "No effect!", game_state.SoundMagic
	if result.Applied {
		switch result.Spell { //nolint:exhaustive // Validated party-spell selector.
		case references.Mani, references.VasMani:
			message, sound = fmt.Sprintf("Healed %d!", result.Healed), game_state.SoundHeal
			if result.Spell == references.VasMani {
				sound = game_state.SoundGreatHeal
			}
		case references.AnNox:
			message, sound = "Cured!", game_state.SoundCure
		case references.AnZu:
			message, sound = "Awakened!", game_state.SoundAwakenSpell
		}
	}
	combat := g.gameState.CombatScene
	g.gameState.EventQueueManager.Workflow("combat_party_spell_effect").Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().First(func() {
		g.ShowActionFeedback(message, true)
		if !result.Applied {
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(sound)
		}
	}).ThenSequence(func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if !result.Applied {
			return q
		}
		feedback := game_state.NewPartyStateBuilder(g.gameState).ToCharacter(result.Target.EntityIndex).WithSound(sound)
		if result.Spell == references.AnZu {
			feedback.FlashWhite()
		} else {
			feedback.FlashInverted()
		}

		return q.ThenSequence(feedback.BuildWithPresentationTiming(timing.duration))
	}).
		Finally(func() {
			g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(0)
			g.gameState.SystemCallbacks.Visual.SetScreenInvert(0, 0)
			if g.gameState.CombatScene == combat {
				g.gameState.FinishCombatAction()
			}
		}).Start()
}

func (g *GameScene) drawCombatPartySpellHUD(screen *ebiten.Image, turnBounds, eventBounds image.Rectangle) {
	targeting := g.gameState.CombatScene.SpellTargeting
	preview := g.gameState.ReviewCombatPartySpellRecipient()
	top, bottom := g.combatHUD.turnOutput, g.combatHUD.eventOutput
	top.Reset()
	bottom.Reset()
	setCombatHUDOutputCapacity(top, turnBounds)
	if eventBounds.Empty() {
		// One pane: the recipient's details and the keys follow the heading.
		bottom = top
	} else {
		setCombatHUDOutputCapacity(bottom, eventBounds)
	}
	top.AppendLine().Text("CAST: ", widgets.DialogLabelColor).Text(references.GetSpellName(targeting.Request.Spell), widgets.DialogValueColor).WordWrap().Done()
	top.AppendLine().Text("Caster: ", widgets.DialogLabelColor).Text(g.gameState.PartyState.GetCharacter(targeting.Caster.EntityIndex).GetNameAsString(), widgets.DialogValueColor).WordWrap().Done()
	if !preview.Valid {
		top.AppendLine().Text("No one is there!", widgets.DialogStatusWarningColor).WordWrap().Done()
		bottom.AppendLine().Text("Choose a visible party member.", widgets.DialogValueColor).WordWrap().Done()
	} else {
		top.AppendLine().Text("> ", widgets.DialogControlKeyColor).Text(preview.Name, widgets.DialogValueColor).WordWrap().Done()
		top.AppendLine().Text("HP: ", widgets.DialogLabelColor).Textf("%d / %d", widgets.DialogValueColor, preview.HP, preview.MaxHP).Done()
		statusColor := widgets.DialogValueColor
		if preview.Status != "Good" {
			statusColor = widgets.DialogStatusWarningColor
		}
		top.AppendLine().Text("Status: ", widgets.DialogLabelColor).Text(preview.Status, statusColor).WordWrap().Done()
		if preview.Identity.Slot != 0 {
			bottom.AppendLine().Text("HP shared with roster character.", widgets.DialogLabelColor).WordWrap().Done()
		}
		if preview.Occupants > 1 {
			bottom.AppendLine().Textf("Occupant %d of %d", widgets.DialogValueColor, preview.Occupant, preview.Occupants).Done()
		}
		if preview.WakingName != "" {
			bottom.AppendLine().Text("Will awaken: ", widgets.DialogLabelColor).Text(preview.WakingName, widgets.DialogValueColor).WordWrap().Done()
		}
		bottom.AppendLine().Text(preview.Consequence, widgets.DialogValueColor).WordWrap().Done()
	}
	bottom.AppendLine().Text("Cost: ", widgets.DialogLabelColor).Textf("%d MP + 1 mixture", widgets.DialogValueColor, references.GetSpellCircle(targeting.Request.Spell)).WordWrap().Done()
	widgets.AppendControlHints(bottom.AppendLine(), "[Arrows/keypad] Aim   [Tab] Next member").WordWrap().Done()
	widgets.AppendControlHints(bottom.AppendLine(), "[Enter] Cast   [Esc] Cancel").WordWrap().Done()
	top.DrawPanelContent(screen, turnBounds.Min, turnBounds.Dx(), etext.AlignStart)
	if bottom != top {
		bottom.DrawPanelContent(screen, eventBounds.Min, eventBounds.Dx(), etext.AlignStart)
	}
}

func setCombatHUDOutputCapacity(output *text.Output, bounds image.Rectangle) {
	width, _ := output.Font.MeasureString("M")
	output.SetMaxCharsPerLine(max(1, int(float64(bounds.Dx())/width)))
}

func (g *GameScene) drawCombatPartySpellWakeMarker(screen *ebiten.Image) {
	if !g.gameState.IsCombatPartySpellTargeting() {
		return
	}
	preview := g.gameState.ReviewCombatPartySpellRecipient()
	if !preview.Valid || !preview.ShowWakingPosition || preview.WakingPosition == g.gameState.CombatScene.SpellTargeting.Cursor {
		return
	}
	drawCombatWakeReticle(screen, g.combatSquareMarkerDrawOptions(preview.WakingPosition))
}
