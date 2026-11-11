package main

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	combatChestRevealDelayMs          int64 = 500
	combatFieldCreationHoldDurationMs int64 = 200
	combatAutoAttackHoldDurationMs    int64 = 1250
	combatAutoAttackWorkflowName            = "combat_auto_attack"
	combatAimCancelLabel                    = "Cancel"
	combatAttackActionLabel                 = "Attack"
)

func (g *GameScene) combatMapInputHandler(key ebiten.Key) {
	if g.handleCombatHUDInput(key) {
		return
	}
	if g.gameState.CombatScene != nil && !g.gameState.CombatScene.CombatTurnsStarted() {
		return
	}

	skipFinishTurn := false
	movementAction := false

	if g.keyboard.IsModifierComboPressed(input.ModifierCtrl, ebiten.KeyM) {
		g.toggleMusic()

		return
	}
	if g.handleBlockedCombatCommand(key) {
		return
	}
	if g.handleControlledMonsterCombatInput(key) {
		return
	}

	switch key { //nolint:exhaustive // Only HUD-specific keys are consumed here.
	case ebiten.KeyEscape:
		g.handleCombatEscape()

		return
	case ebiten.KeySpace:
		if g.gameState.IsSoloMoveMode() {
			g.appendToCurrentRowStr("Pass - monsters act next")
		} else {
			g.appendToCurrentRowStr("Pass")
		}
		g.gameState.FinishCombatAction()

		return
	case ebiten.KeyTab:
		action := g.gameState.PlanCombatAutoAction()
		switch action.Kind {
		case game_state.CombatAutoActionMove:
			g.appendToCurrentRowStr("Auto advance-")
			movementAction = g.handleCombatMovement(action.Direction)
			skipFinishTurn = !movementAction
		case game_state.CombatAutoActionAttack:
			g.appendToCurrentRowStr("Auto attack-")
			g.resolveCombatAutoAttack(action.Target)

			return
		case game_state.CombatAutoActionUnavailable:
			g.appendToCurrentRowStr("Auto")
			g.addRowStr("No reachable enemy.")
			g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")
			g.stopCombatKeyRepeat()

			return
		default:
			return
		}
	case ebiten.KeyUp:
		movementAction = g.handleCombatMovement(references.Up)
		skipFinishTurn = !movementAction
	case ebiten.KeyDown:
		movementAction = g.handleCombatMovement(references.Down)
		skipFinishTurn = !movementAction
	case ebiten.KeyLeft:
		movementAction = g.handleCombatMovement(references.Left)
		skipFinishTurn = !movementAction
	case ebiten.KeyRight:
		movementAction = g.handleCombatMovement(references.Right)
		skipFinishTurn = !movementAction
	case ebiten.KeyG:
		g.appendToCurrentRowStr("Get-")
		g.secondaryKeyState = GetDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyP:
		g.appendToCurrentRowStr("Push-")
		g.secondaryKeyState = PushDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyO:
		g.appendToCurrentRowStr("Open-")
		g.secondaryKeyState = OpenDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyJ:
		g.appendToCurrentRowStr("Jimmy-")
		g.secondaryKeyState = JimmyDoorDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyK:
		g.appendToCurrentRowStr("Klimb-")
		switch g.gameState.BeginCombatKlimb() {
		case game_state.CombatKlimbNeedsDirection:
			g.secondaryKeyState = KlimbDirectionInput
			g.keyboard.SetAllowKeyPressImmediately()
		case game_state.CombatKlimbNeedsLadderDirection:
			g.appendToCurrentRowStr("U/D-")
			g.secondaryKeyState = CombatKlimbLadderDirectionInput
			g.keyboard.SetAllowKeyPressImmediately()
		case game_state.CombatKlimbCompleted:
			g.gameState.FinishCombatAction()
		case game_state.CombatKlimbRejected:
			g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")
		}

		return
	case ebiten.KeyS:
		g.appendToCurrentRowStr("Search-")
		g.secondaryKeyState = SearchDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyA:
		if g.gameState.BeginCombatAttack() {
			g.appendToCurrentRowStr("Attack-")
			availability := g.gameState.SkipUnavailableCombatAttackSources()
			if availability.Outcome == game_state.CombatAttackSequenceComplete {
				g.finishCombatAttackTargetAction(availability)
				skipFinishTurn = true
			} else {
				g.showCurrentCombatAttackPrompt()
				g.secondaryKeyState = CombatAttackTargetInput
				g.keyboard.SetAllowKeyPressImmediately()
			}
		} else {
			skipFinishTurn = true
			g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")
			g.stopCombatKeyRepeat()
		}
	case ebiten.KeyU:
		g.appendToCurrentRowStr("Use...")
		g.StartCombatUseWorkflow()
		skipFinishTurn = true
	case ebiten.KeyY:
		g.appendToCurrentRowStr("Yell ")
		g.startCombatYellWorkflow()
		skipFinishTurn = true
	case ebiten.KeyR:
		g.appendToCurrentRowStr("Ready...")
		switch g.gameState.ActionReadyCombatMap() {
		case game_state.ReadyBeginCompleted:
			g.gameState.FinishCombatAction()
		case game_state.ReadyBeginDialog, game_state.ReadyBeginRejected:
		}
		skipFinishTurn = true
	case ebiten.KeyZ:
		g.appendToCurrentRowStr("Ztats...")
		g.gameState.ActionZtats()
		skipFinishTurn = true
	case ebiten.KeyC:
		g.appendToCurrentRowStr("Cast...")
		if g.gameState.CheckCombatCastInterference() {
			g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")

			return
		}
		g.startSpellCastingWorkflow()
		skipFinishTurn = true
	default:
		g.repromptCombatActor("What?")

		return
	}

	// only process end of turn if the turn is actually done.
	if g.secondaryKeyState == PrimaryInput && !skipFinishTurn {
		if !movementAction {
			g.gameState.FinishCombatAction()
		}
	}
}

func (g *GameScene) resolveCombatAutoAttack(target references.Position) {
	timing := g.captureCombatTiming()
	defer g.combatTimingScope(timing)()
	if !g.gameState.BeginCombatAutoAttack(target) {
		return
	}
	combat := g.gameState.CombatScene
	targeting := combat.AttackTargeting
	queue := g.releaseAfterCombatAction(g.gameState.EventQueueManager.Workflow(combatAutoAttackWorkflowName).
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput()).
		First(func() {})
	// A bounded source count lets every launch/return finish in the same owner.
	// Later steps review live ammo, obstruction and the exact original victim.
	for range targeting.Sources {
		queue.ThenSequence(func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
			defer g.combatTimingScope(timing)()
			if g.gameState.CombatScene != combat || combat.AttackTargeting != targeting {
				return q
			}
			var result game_state.CombatAttackTargetResult
			_, impacts := g.captureCombatImpacts(func() bool {
				result = g.gameState.ContinueCombatAutoAttack()

				return result.Outcome != game_state.CombatAttackTargetIgnored
			})
			for _, impact := range impacts {
				g.showCombatImpact(impact)
			}
			if result.Launch != nil {
				return q.ThenSequence(g.combatPhysicalProjectileSequence(result, func(game_state.CombatAttackTargetResult) {}))
			}
			if len(impacts) > 0 {
				// Dynamic insertion starts its temporary first node immediately.
				// Anchor the hold to a real step, not that node's ignored leading delay.
				return q.Then(func() {}).Wait(timing.pauseMs(combatAutoAttackHoldDurationMs)).Then(func() {})
			}

			return q
		})
	}
	queue.Then(func() {}).Finally(func() {
		g.gameState.SystemCallbacks.Visual.ClearProjectile()
		g.secondaryKeyState = PrimaryInput
		if g.gameState.CombatScene != combat {
			return
		}
		g.withCombatTiming(timing, func() { g.gameState.ResolvePendingCombatAttack() })
		for combat.AttackTargeting == targeting {
			g.gameState.CancelCombatAttackSource()
		}
		if combat.GetCurrentTurnIdentity() == targeting.Attacker {
			g.gameState.FinishCombatAction()
		}
	}).Start()
}

func (g *GameScene) handleBlockedCombatCommand(key ebiten.Key) bool {
	var feedback string
	switch key { //nolint:exhaustive // Only rejected tactical command keys belong here; ref:COMBAT-0003.
	case ebiten.KeyB:
		feedback = "Board what?"
	case ebiten.KeyD:
		feedback = "D-What?"
	case ebiten.KeyE:
		feedback = "Enter-Not here"
	case ebiten.KeyF:
		feedback = "Fire-Not here"
	case ebiten.KeyH:
		feedback = "Hole up-Not here"
	case ebiten.KeyI:
		feedback = "Ignite torch-Not here"
	case ebiten.KeyL:
		feedback = "Look-Not here"
	case ebiten.KeyM:
		feedback = "Mix-Not here"
	case ebiten.KeyN:
		feedback = "New order-Not here"
	case ebiten.KeyQ:
		feedback = "Quit-Not here"
	case ebiten.KeyT:
		feedback = "Talk-Funny, no response!"
	case ebiten.KeyV:
		feedback = "View-Not here"
	case ebiten.KeyW:
		feedback = "W-What?"
	case ebiten.KeyX:
		feedback = "X-it what?"
	default:
		return false
	}

	g.repromptCombatActor(feedback)

	return true
}

func (g *GameScene) repromptCombatActor(feedback string) {
	g.gameState.SystemCallbacks.Message.AppendToCurrentRowStr(feedback)
	g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")
}

func (g *GameScene) startCombatYellWorkflow() {
	g.gameState.EventQueueManager.Workflow("combat_yell").
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		First(func() {}).
		ThenSequence(g.thenShowInputBoxWithEscapeClear(
			"what?\n:",
			"^[ -~]{0,30}$",
			func(text string, submitted bool) {
				if !submitted {
					text = ""
				}
				g.gameState.ActionYellWithoutEffect(text)
			},
		)).
		Finally(func() {
			g.gameState.FinishCombatAction()
		}).
		Start()
}

func (g *GameScene) handleControlledMonsterCombatInput(key ebiten.Key) bool {
	if g.gameState.CombatScene == nil {
		return false
	}
	if monster, _ := g.gameState.CombatScene.GetControlledMonster(); monster == nil {
		return false
	}

	switch key { //nolint:exhaustive // Every unlisted command is deliberately rejected below.
	case ebiten.KeyEscape:
		g.handleCombatEscape()
	case ebiten.KeySpace:
		g.appendToCurrentRowStr("Pass")
		g.gameState.FinishCombatAction()
	case ebiten.KeyUp:
		g.handleCombatMovement(references.Up)
	case ebiten.KeyDown:
		g.handleCombatMovement(references.Down)
	case ebiten.KeyLeft:
		g.handleCombatMovement(references.Left)
	case ebiten.KeyRight:
		g.handleCombatMovement(references.Right)
	case ebiten.KeyA:
		if g.gameState.BeginCombatAttack() {
			g.appendToCurrentRowStr("Attack-")
			availability := g.gameState.SkipUnavailableCombatAttackSources()
			if availability.Outcome == game_state.CombatAttackSequenceComplete {
				g.finishCombatAttackTargetAction(availability)
			} else {
				g.showCurrentCombatAttackPrompt()
				g.secondaryKeyState = CombatAttackTargetInput
				g.keyboard.SetAllowKeyPressImmediately()
			}
		}
	case ebiten.KeyP:
		g.appendToCurrentRowStr("Push-")
		g.secondaryKeyState = PushDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	default:
		g.gameState.SystemCallbacks.Message.AddRowStr("Can't!")
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")
	}

	return true
}

func (g *GameScene) handleCombatEscape() {
	g.gameState.SystemCallbacks.Message.AppendToCurrentRowStr("Escape...")
	if g.gameState.CanEscapeCombatMap() {
		g.startCombatVictoryExitWorkflow()
	}
}

func (g *GameScene) handleCombatHUDInput(key ebiten.Key) bool {
	if g.combatHUD == nil {
		return false
	}

	//nolint:exhaustive // Only HUD-specific keys are consumed here.
	switch key {
	case ebiten.KeyF2:
		g.combatHUD.detail = !g.combatHUD.detail
		g.gameConfig.DebugConfiguration.CombatDetailShown = g.combatHUD.detail
		g.gameConfig.DebugConfiguration.CombatDetailSet = true
		if err := g.gameConfig.SaveDebugConfiguration(); err != nil {
			logging.Warnf(logging.CategoryUI, "Unable to save combat detail setting: %v", err)
		}
	case ebiten.KeyF3:
		g.combatHUD.reviewNextResult()
	case ebiten.KeyPageUp:
		g.combatHUD.historyOffset = min(
			g.combatHUD.historyOffset+1,
			max(g.combatHUD.historyCount-1, 0),
		)
	case ebiten.KeyPageDown:
		g.combatHUD.historyOffset = max(g.combatHUD.historyOffset-1, 0)
		if g.combatHUD.historyOffset == 0 {
			g.combatHUD.unreadEvents = 0
		}
	case ebiten.KeyHome:
		oldestPageCount := g.combatEventCountFromStart(max(g.combatHUD.eventLineBudget, 1))
		g.combatHUD.historyOffset = max(
			g.combatHUD.historyCount-oldestPageCount,
			0,
		)
	case ebiten.KeyEnd:
		g.combatHUD.historyOffset = 0
		g.combatHUD.unreadEvents = 0
	default:
		return false
	}

	return true
}

func (g *GameScene) combatMapHandleSecondaryInput() {
	//nolint:exhaustive // Specialized selectors return before ordinary direction dispatch below.
	switch g.secondaryKeyState {
	case CombatAttackTargetInput:
		g.handleCombatAttackTargetInput()

		return
	case CombatSpellTargetInput:
		g.handleCombatSpellTargetInput()

		return
	case CombatConeTargetInput:
		g.handleCombatConeTargetInput()

		return
	case CombatAdjacentSpellTargetInput:
		g.handleCombatAdjacentSpellTargetInput()

		return
	case CombatKlimbLadderDirectionInput:
		g.handleCombatKlimbLadderDirectionInput()

		return
	}

	if g.cancelCombatDirectionInput() {
		return
	}
	g.appendOrdinaryDirectionHint(references.CombatMapType)
	directionInput := g.ordinaryDirectionInput()
	if directionInput.Outcome == ordinaryDirectionActive {
		return
	}
	if directionInput.Outcome == ordinaryDirectionPassed {
		g.secondaryKeyState = PrimaryInput
		g.gameState.FinishCombatAction()

		return
	}
	direction := directionInput.Direction

	//nolint:exhaustive // Tactical targeting states returned before ordinary direction routing.
	switch g.secondaryKeyState {
	case JimmyDoorDirectionInput:
		g.combatMapJimmySecondary(direction)
		g.secondaryKeyState = PrimaryInput
		g.gameState.FinishCombatAction()

		return
	case OpenDirectionInput:
		deferredAdvance := g.combatMapOpenSecondary(direction)
		g.secondaryKeyState = PrimaryInput
		if !deferredAdvance {
			g.gameState.FinishCombatAction()
		}

		return
	case KlimbDirectionInput:
		ladderAttempt := g.gameState.CombatKlimbTargetKind() == game_state.CombatKlimbLadderTarget
		success := g.combatMapKlimbSecondary(direction)
		switch {
		case success:
			g.secondaryKeyState = PrimaryInput
			g.gameState.FinishCombatAction()
		case ladderAttempt:
			g.secondaryKeyState = PrimaryInput
			g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")
		default:
			g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Klimb-")
		}

		return
	case PushDirectionInput:
		g.combatMapPushSecondary(direction)
		g.secondaryKeyState = PrimaryInput
	case GetDirectionInput:
		g.combatMapGetSecondary(direction)
		g.secondaryKeyState = PrimaryInput
		g.gameState.FinishCombatAction()

		return
	case SearchDirectionInput:
		g.combatMapSearchSecondary(direction)
		g.secondaryKeyState = PrimaryInput
		g.gameState.FinishCombatAction()

		return
	}

	// only process end of turn if the turn is actually done.
	if g.secondaryKeyState == PrimaryInput {
		g.gameState.FinishCombatAction()
	}
}

func (g *GameScene) handleCombatKlimbLadderDirectionInput() {
	direction, selected := g.combatKlimbLadderDirection()
	if !selected {
		return
	}
	success := g.gameState.ActionKlimbCombatMap(direction)
	g.secondaryKeyState = PrimaryInput
	if success {
		g.gameState.FinishCombatAction()
	} else {
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")
	}
}

func (g *GameScene) combatKlimbLadderDirection() (references.Direction, bool) {
	for _, candidate := range []struct {
		key       ebiten.Key
		direction references.Direction
	}{
		{key: ebiten.KeyU, direction: references.Up},
		{key: ebiten.KeyUp, direction: references.Up},
		{key: ebiten.KeyNumpad8, direction: references.Up},
		{key: ebiten.KeyD, direction: references.Down},
		{key: ebiten.KeyDown, direction: references.Down},
		{key: ebiten.KeyNumpad2, direction: references.Down},
	} {
		if g.keyboard.IsKeyPressedAndRegister(candidate.key) {
			return candidate.direction, true
		}
	}

	return references.Direction(0), false
}

func (g *GameScene) handleCombatAdjacentSpellTargetInput() {
	if g.gameState.CombatScene == nil || g.gameState.CombatScene.AdjacentSpellTargeting == nil {
		g.secondaryKeyState = PrimaryInput

		return
	}
	doorSpell := game_state.IsCombatDoorSpell(g.gameState.CombatScene.AdjacentSpellTargeting.Request.Spell)
	if doorSpell {
		g.showCombatDoorSpellAimPrompt()
	}
	if direction, selected := g.combatConeDirectionInput(); selected {
		g.gameState.SetCombatAdjacentSpellDirection(direction)
		if doorSpell {
			g.showCombatDoorSpellAimPrompt()

			return
		}
		target, valid, _ := g.gameState.CombatAdjacentSpellPreview()
		spell := g.gameState.CombatScene.AdjacentSpellTargeting.Request.Spell
		spellName := references.GetSpellName(spell)
		prompt := spellName + " " + direction.GetDirectionCompassName() + " - invalid target"
		if !g.gameState.CombatTerrainAllowsObjectDisplay(target) || valid {
			prompt = spellName + " " + direction.GetDirectionCompassName() + " - Enter: cast, Esc: cancel"
		}
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt(prompt)

		return
	}
	if g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		g.gameState.CancelCombatAdjacentSpellTargeting()
		g.secondaryKeyState = PrimaryInput
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")

		return
	}
	confirm := g.keyboard.IsKeyPressedAndRegister(ebiten.KeyC) ||
		g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		g.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
	if doorSpell && g.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace) {
		// The door cursor always occupies a neighbour, never the caster square.
		// Space therefore confirms just as it does away from the attack origin.
		confirm = true
	}
	if !confirm {
		return
	}
	defer g.combatTimingScope(g.captureCombatTiming())()
	result := g.gameState.ConfirmCombatAdjacentSpellTarget()
	//nolint:exhaustive // Ignored and cancelled outcomes cannot originate from confirmation.
	switch result.Outcome {
	case game_state.CombatAdjacentSpellTargetAwaitingDirection:
		g.gameState.SystemCallbacks.UI.ShowActionFeedback("Choose an adjacent direction.", true)
	case game_state.CombatAdjacentSpellTargetInvalid:
		g.gameState.SystemCallbacks.UI.ShowActionFeedback("Invalid target.", true)
	case game_state.CombatAdjacentSpellTargetValidationFailed:
		g.secondaryKeyState = PrimaryInput
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")
		g.gameState.SystemCallbacks.UI.ShowActionFeedback(result.Validation.ErrorMsg, true)
	case game_state.CombatAdjacentSpellTargetResolved:
		g.secondaryKeyState = PrimaryInput
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")
		g.startCombatAdjacentSpellEffect(result)
	}
}

func (g *GameScene) showCombatDoorSpellAimPrompt() {
	targeting := g.gameState.CombatScene.AdjacentSpellTargeting
	// Deliberately no lock/trap/validity description: aiming must not inspect
	// concealed terrain or disclose whether an unassessed chest is trapped.
	g.gameState.SystemCallbacks.Message.ShowCommandPrompt(
		references.GetSpellName(targeting.Request.Spell) +
			" - Arrows/keypad: adjacent target; Enter/C/Space: cast; Esc: cancel",
	)
}

func (g *GameScene) startCombatAdjacentSpellEffect(result game_state.CombatAdjacentSpellTargetResult) {
	timing := g.captureCombatTiming()
	if game_state.IsCombatDoorSpell(result.Request.Spell) {
		g.startCombatUtilitySpellEffect(result)

		return
	}
	g.gameState.EventQueueManager.Workflow("combat_adjacent_spell_effect").
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		First(g.combatTimingStep(timing, func() {
			//nolint:exhaustive // Adjacent targeting admits only implemented spells.
			switch result.Request.Spell {
			case references.AnGrav:
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundDispel)
				if !g.gameState.CastCombatAnGrav(result.Request.CasterIdx, result.Request.Direction) {
					panic("combat An Grav resolved without a valid target")
				}
			case references.AnYlem:
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
				if !g.gameState.CastCombatAnYlem(result.Request.CasterIdx, result.Request.Direction) {
					panic("combat An Ylem resolved without a valid target")
				}
			default:
				panic("unsupported adjacent combat spell")
			}
			g.gameState.SystemCallbacks.Visual.PlayKapowAtPosition(result.Target)
		})).Finally(func() {
		g.gameState.FinishCombatAction()
	}).Start()
}

func (g *GameScene) handleCombatConeTargetInput() {
	if g.gameState.CombatScene == nil || g.gameState.CombatScene.ConeTargeting == nil {
		g.secondaryKeyState = PrimaryInput

		return
	}
	if direction, selected := g.combatConeDirectionInput(); selected {
		g.gameState.SetCombatConeDirection(direction)
		spell := g.gameState.CombatScene.ConeTargeting.Request.Spell
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt(
			references.GetSpellName(spell) + " " + direction.GetDirectionCompassName() + " - Enter: cast, Esc: cancel",
		)

		return
	}
	if g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		g.gameState.CancelCombatConeTargeting()
		g.secondaryKeyState = PrimaryInput
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")

		return
	}
	confirm := g.keyboard.IsKeyPressedAndRegister(ebiten.KeyC) ||
		g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		g.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
	if !confirm {
		return
	}
	defer g.combatTimingScope(g.captureCombatTiming())()
	result := g.gameState.ConfirmCombatConeTarget()
	//nolint:exhaustive // Ignored and cancelled outcomes cannot originate from confirmation.
	switch result.Outcome {
	case game_state.CombatConeTargetAwaitingDirection:
		g.gameState.SystemCallbacks.UI.ShowActionFeedback("Choose a direction.", true)
	case game_state.CombatConeTargetValidationFailed:
		g.secondaryKeyState = PrimaryInput
		g.gameState.SystemCallbacks.UI.ShowActionFeedback(result.Validation.ErrorMsg, true)
	case game_state.CombatConeTargetResolved:
		g.secondaryKeyState = PrimaryInput
		g.startCombatConeSpellEffect(result)
	}
}

func (g *GameScene) combatConeDirectionInput() (references.Direction, bool) {
	inputs := []struct {
		key       ebiten.Key
		direction references.Direction
	}{
		{ebiten.KeyUp, references.Up},
		{ebiten.KeyNumpad8, references.Up},
		{ebiten.KeyDown, references.Down},
		{ebiten.KeyNumpad2, references.Down},
		{ebiten.KeyLeft, references.Left},
		{ebiten.KeyNumpad4, references.Left},
		{ebiten.KeyRight, references.Right},
		{ebiten.KeyNumpad6, references.Right},
	}
	for _, input := range inputs {
		if g.keyboard.IsKeyPressedAndRegisterRepeatable(input.key) {
			return input.direction, true
		}
	}

	return references.NoneDirection, false
}

func (g *GameScene) handleCombatAttackTargetInput() {
	targeting := g.gameState.CombatScene.AttackTargeting
	if targeting == nil {
		g.secondaryKeyState = PrimaryInput

		return
	}

	if dx, dy, ok := g.getCombatTargetMovement(); ok {
		g.gameState.MoveCombatAttackCursor(dx, dy)
		g.showCombatAttackTargetContext()

		return
	}

	if g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		if targeting.SourceIndex == 0 {
			g.finishCombatAttackTargetAction(g.gameState.CancelInitialCombatAttack())
		} else {
			g.captureCombatAttackTiming()
			g.finishCombatAttackTargetAction(g.gameState.CancelCombatAttackSource())
		}

		return
	}
	if g.repeatCombatAttackConfirmation(targeting) {
		return
	}

	g.handleCombatAttackConfirmation(targeting)
}

func (g *GameScene) handleCombatAttackConfirmation(targeting *game_state.CombatAttackTargeting) {
	confirm := g.keyboard.IsKeyPressedAndRegister(ebiten.KeyA) ||
		g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		g.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
	if g.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace) {
		if targeting.Cursor == targeting.Origin {
			g.captureCombatAttackTiming()
			g.finishCombatAttackTargetAction(g.gameState.CancelCombatAttackSource())

			return
		}
		confirm = true
	}
	if confirm {
		g.confirmCombatAttack(targeting)
	}
}

// confirmCombatAttack fires the current source at the aimed square, as A, Enter
// or Space does.
func (g *GameScene) confirmCombatAttack(targeting *game_state.CombatAttackTargeting) {
	if targeting.Cursor == targeting.Origin {
		g.finishCombatAttackTargetAction(g.gameState.ConfirmCombatAttackTarget())

		return
	}
	timing := g.captureCombatAttackTiming()
	var result game_state.CombatAttackTargetResult
	g.withCombatTiming(timing, func() { result = g.gameState.ConfirmCombatAttackTarget() })
	if result.Launch != nil {
		g.withCombatTiming(timing, func() { g.startCombatProjectileAttack(result) })

		return
	}
	g.finishCombatAttackTargetAction(result)
}

func (g *GameScene) startCombatProjectileAttack(result game_state.CombatAttackTargetResult) {
	timing := g.captureCombatTiming()
	resolution := game_state.CombatAttackTargetResult{Outcome: game_state.CombatAttackTargetIgnored}
	resolved := false

	g.releaseAfterCombatAction(g.gameState.EventQueueManager.Workflow("combat_physical_projectile_attack").
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput()).
		First(func() {}).
		ThenSequence(g.combatPhysicalProjectileSequence(result, func(impact game_state.CombatAttackTargetResult) {
			resolution = impact
			resolved = true
		})).Finally(func() {
		g.gameState.SystemCallbacks.Visual.ClearProjectile()
		if !resolved {
			g.withCombatTiming(timing, func() { resolution = g.gameState.ResolvePendingCombatAttack() })
			resolved = resolution.Outcome != game_state.CombatAttackTargetIgnored
		}
		if resolved {
			g.finishCombatAttackTargetAction(resolution)
		}
	}).Start()
}

// Both manual aiming and Tab await impact, return travel and cleanup in their
// owning workflow. Neither continuation can spend the next source before this ends.
func (g *GameScene) combatPhysicalProjectileSequence(
	result game_state.CombatAttackTargetResult,
	onResolved func(game_state.CombatAttackTargetResult),
) event_queue.Sequence {
	launch := *result.Launch
	timing := g.captureCombatTiming()

	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.ThenSequence(g.combatProjectileAnimationSequenceWithTiming(timing, launch, func() {
			resolution := g.gameState.ResolvePendingCombatAttack()
			if resolution.Outcome == game_state.CombatAttackTargetIgnored {
				panic("combat projectile completed without a pending physical attack")
			}
			if resolution.ShowImpact {
				g.gameState.SystemCallbacks.Visual.PlayKapowAtPosition(launch.Impact)
			}
			onResolved(resolution)
		})).ThenSequence(func(sequence *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
			if result.ReturnLaunch == nil {
				return sequence
			}

			return sequence.ThenSequence(g.combatProjectileAnimationSequenceWithTiming(timing, *result.ReturnLaunch, func() {}))
		}).Then(g.gameState.SystemCallbacks.Visual.ClearProjectile)
	}
}

func (g *GameScene) getCombatTargetMovement() (int, int, bool) {
	movements := []struct {
		key    ebiten.Key
		dx, dy int
	}{
		{ebiten.KeyNumpad7, -1, -1},
		{ebiten.KeyNumpad9, 1, -1},
		{ebiten.KeyNumpad1, -1, 1},
		{ebiten.KeyNumpad3, 1, 1},
		{ebiten.KeyUp, 0, -1},
		{ebiten.KeyNumpad8, 0, -1},
		{ebiten.KeyDown, 0, 1},
		{ebiten.KeyNumpad2, 0, 1},
		{ebiten.KeyLeft, -1, 0},
		{ebiten.KeyNumpad4, -1, 0},
		{ebiten.KeyRight, 1, 0},
		{ebiten.KeyNumpad6, 1, 0},
	}
	for _, movement := range movements {
		if g.keyboard.IsKeyPressedAndRegisterRepeatable(movement.key) {
			return movement.dx, movement.dy, true
		}
	}

	return 0, 0, false
}

func (g *GameScene) finishCombatAttackTargetAction(result game_state.CombatAttackTargetResult) {
	if result.Outcome == game_state.CombatAttackTargetIgnored {
		return
	}
	if result.Outcome == game_state.CombatAttackSequenceComplete {
		g.combatAttackTiming = combatAttackTimingSnapshot{}
		g.secondaryKeyState = PrimaryInput
		g.gameState.FinishCombatAction()

		return
	}
	if result.Outcome == game_state.CombatAttackSequenceCancelled {
		g.combatAttackTiming = combatAttackTimingSnapshot{}
		g.secondaryKeyState = PrimaryInput
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")

		return
	}
	availability := g.gameState.SkipUnavailableCombatAttackSources()
	if availability.Outcome != game_state.CombatAttackTargetIgnored {
		g.finishCombatAttackTargetAction(availability)

		return
	}

	g.showCurrentCombatAttackPrompt()
}

func (g *GameScene) handleCombatSpellTargetInput() {
	if g.gameState.CombatScene == nil {
		g.secondaryKeyState = PrimaryInput

		return
	}
	if g.gameState.IsCombatPartySpellTargeting() {
		g.handleCombatPartySpellInput()

		return
	}
	targeting := g.gameState.CombatScene.SpellTargeting
	if targeting == nil {
		g.secondaryKeyState = PrimaryInput

		return
	}

	if dx, dy, ok := g.getCombatTargetMovement(); ok {
		g.gameState.MoveCombatSpellCursor(dx, dy)

		return
	}
	if g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		g.finishCombatSpellTargetAction(game_state.CombatSpellTargetResult{
			Outcome: g.gameState.CancelCombatSpellTargeting(),
		})

		return
	}

	confirm := g.keyboard.IsKeyPressedAndRegister(ebiten.KeyC) ||
		g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		g.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) ||
		g.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace)
	if confirm {
		defer g.combatTimingScope(g.captureCombatTiming())()
		g.finishCombatSpellTargetAction(g.gameState.ConfirmCombatSpellTarget())
	}
}

func (g *GameScene) finishCombatSpellTargetAction(result game_state.CombatSpellTargetResult) {
	switch result.Outcome {
	case game_state.CombatSpellTargetIgnored:
		return
	case game_state.CombatSpellTargetInvalid:
		g.gameState.SystemCallbacks.UI.ShowActionFeedback("Choose a target square.", true)
	case game_state.CombatSpellTargetCancelled:
		g.secondaryKeyState = PrimaryInput
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")
	case game_state.CombatSpellTargetValidationFailed:
		g.secondaryKeyState = PrimaryInput
		g.gameState.SystemCallbacks.UI.ShowActionFeedback(result.Validation.ErrorMsg, true)
	case game_state.CombatSpellTargetResolved:
		g.secondaryKeyState = PrimaryInput
		g.startCombatSpellEffect(result)
	}
}

func (g *GameScene) startCombatSpellEffect(result game_state.CombatSpellTargetResult) {
	timing := g.captureCombatTiming()
	launch := result.Launch
	launch.Missile = combatSpellMissile(result.Spell)

	g.gameState.EventQueueManager.Workflow("combat_spell_effect").
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		First(func() {
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
		}).ThenSequence(g.combatProjectileAnimationSequenceWithTiming(timing, launch, func() {
		if !g.gameState.ResolvePendingCombatSpell() {
			panic("combat spell effect completed without a pending spell")
		}
		if !launch.Blocked {
			g.gameState.SystemCallbacks.Visual.PlayKapowAtPosition(launch.Impact)
			if !game_state.IsCombatFieldSpell(result.Spell) {
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundSpellImpact)
			}
		}
	})).ThenSequence(func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if game_state.IsCombatFieldSpell(result.Spell) {
			// This legacy leading-only dynamic delay is ineffective. Preserve the
			// reviewed 1x behavior rather than silently add a new field-spell hold.
			return q.Wait(timing.durationMs(combatFieldCreationHoldDurationMs))
		}

		return q
	}).Finally(func() {
		g.gameState.SystemCallbacks.Visual.ClearProjectile()
		g.withCombatTiming(timing, func() { g.gameState.ResolvePendingCombatSpell() })
		g.gameState.FinishCombatAction()
	}).Start()
}

func combatSpellMissile(spell references.Spell) references.MissileType {
	//nolint:exhaustive // Red is the established default spell missile.
	switch spell {
	case references.GravPor, references.XenCorp:
		return references.MissileBlue
	case references.InNoxGrav:
		return references.MissileGreen
	case references.InZuGrav:
		return references.MissileViolet
	case references.InSanctGrav:
		return references.MissileBlue
	default:
		return references.MissileRed
	}
}

func (g *GameScene) showCurrentCombatAttackPrompt() {
	targeting := g.gameState.CombatScene.AttackTargeting
	if targeting == nil {
		return
	}
	if targeting.IsMonsterAttacker() {
		g.appendToCurrentRowStr("Aim!")
		g.showCombatAimControls(combatAttackActionLabel, combatAimCancelLabel)

		return
	}
	source := targeting.CurrentSource()
	name := source.Equipment.String()
	if source.Equipment == references.NoEquipment {
		name = "Bare Hands"
	}
	g.addRowStr(fmt.Sprintf("Attack %d/%d: %s", targeting.SourceIndex+1, len(targeting.Sources), name))
	g.appendLineBuilderDualOutput().Text("R: ", widgets.DialogLabelColor).Text("Range", widgets.DialogValueColor).
		Text("  D: ", widgets.DialogLabelColor).Text("Distance", widgets.DialogValueColor).WordWrap().Done()
	escape := combatAimCancelLabel
	if targeting.SourceIndex > 0 {
		escape = "Skip item"
	}
	g.showCombatAimControls(combatAttackActionLabel, escape)
	widgets.AppendControlHints(g.appendLineBuilderDualOutput(), "[Space] Skip at origin").WordWrap().Done()
	g.showCombatAttackTargetContext()
}

func (g *GameScene) showCombatAttackTargetContext() {
	preview := g.gameState.PreviewCombatAttack()
	if preview.Aim == nil {
		return
	}
	message := "Target: no one visible"
	if preview.Aim.Status == game_state.CombatShotChooseTarget {
		message = "Choose target"
	} else if preview.Aim.Actor != nil {
		message = "Target: " + preview.Aim.Name + " - " + combatActorContext(*preview.Aim.Actor)
	}
	g.gameState.SystemCallbacks.Message.ShowCommandPrompt(message)
}

func (g *GameScene) showCombatAimControls(action, escape string) {
	widgets.AppendControlHints(g.appendLineBuilderDualOutput(), "[Arrows/keypad] Aim").WordWrap().Done()
	widgets.AppendControlHints(g.appendLineBuilderDualOutput(), "[Enter] "+action+"   [Esc] "+escape).WordWrap().Done()
}

// Combat Map Secondary Action Handlers

func (g *GameScene) combatMapKlimbSecondary(direction references.Direction) bool {
	return g.gameState.ActionKlimbCombatMap(direction)
}

func (g *GameScene) combatMapPushSecondary(direction references.Direction) {
	g.gameState.ActionPushCombatMap(direction)
}

func (g *GameScene) combatMapOpenSecondary(direction references.Direction) bool {
	defer g.combatTimingScope(g.captureCombatTiming())()
	result := g.gameState.ActionOpenCombatMap(direction)
	if !result.Handled {
		g.addRowStr("Nothing to open!")

		return false
	}
	if len(result.RevealLines) == 0 {
		if result.OpenedChest {
			g.recordCombatEvent(game_state.CombatEvent{
				Kind:       game_state.CombatEventLootRevealed,
				LootReveal: game_state.CombatLootReveal{},
			})
		}

		return false
	}

	token := g.gameState.FinishCombatActionDeferred()
	g.startCombatChestRevealWorkflow(result, token)

	return true
}

func (g *GameScene) combatMapJimmySecondary(direction references.Direction) {
	handled := g.captureCombatChestJimmyFeedback(func() (bool, *game_state.CombatChestIdentity) {
		return g.gameState.ActionJimmyCombatMapWithDisarm(direction)
	})
	if !handled {
		g.addRowStr("Nothing to jimmy!")
	}
}

func (g *GameScene) combatMapGetSecondary(direction references.Direction) {
	result := g.gameState.GetCombatItem(direction)
	if !result.Handled {
		g.addRowStr(result.Message)
	}
}

func (g *GameScene) combatMapSearchSecondary(direction references.Direction) {
	// Search system handles all user feedback via SystemCallbacks
	g.gameState.ActionSearchCombatMap(direction)
}

// handleCombatMovement moves the active combat character in the given direction.
// Returns true when the caller should finish the turn. Flee finalizes internally
// before advancing, so it returns false to prevent a second or post-combat finish.
func (g *GameScene) handleCombatMovement(direction references.Direction) bool {
	defer g.combatTimingScope(g.captureCombatTiming())()
	if g.gameState.CombatScene == nil {
		return false
	}

	g.appendToCurrentRowStr(direction.GetDirectionCompassName())

	identity := g.gameState.CombatScene.GetCurrentTurnIdentity()
	turnIdx := identity.EntityIndex
	from := g.gameState.CombatScene.GetCombatantPosition(identity)
	if !identity.IsPartyMember {
		monster := g.gameState.CombatScene.MonsterManager.GetMonster(turnIdx)
		if monster == nil {
			return false
		}
		from = monster.Position()
	}
	result := g.gameState.MoveCombatCharacter(direction)

	switch result {
	case game_state.CombatMoveMoved:
		to := g.gameState.CombatScene.GetCombatantPosition(identity)
		token := g.gameState.FinishCombatTurnDeferred()
		if identity.IsPartyMember {
			g.startPlayerCombatMovementWorkflow(identity, from, to, token)
		} else {
			monster := g.gameState.CombatScene.MonsterManager.GetMonster(turnIdx)
			if monster == nil {
				return false
			}
			to = monster.Position()
			g.startControlledMonsterCombatMovementWorkflow(turnIdx, from, to, token)
		}

		return true
	case game_state.CombatMoveFled:
		// Flee messages are already published; consume this turn before selecting
		// the next actor so a final departure cannot advance the restored world.
		g.gameState.FinishTurn()
		g.gameState.AdvanceCombatTurn()

		return false
	case game_state.CombatMoveBlocked:
		g.addRowStr("Blocked!")
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("")
		g.stopCombatKeyRepeat()

		return false
	default:
		return false
	}
}
