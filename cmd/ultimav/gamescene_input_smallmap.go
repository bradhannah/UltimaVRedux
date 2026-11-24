package main

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

//nolint:maintidx // Keep pre-cycle shortcuts, town command routing, and each command's completion ownership together.
func (g *GameScene) smallMapInputHandlerWithGame(game *Game, key ebiten.Key) {
	g.clearSurfaceCameraForInput(key)
	if key == ebiten.KeyM && g.keyboard.IsModifierComboPressed(input.ModifierCtrl, ebiten.KeyM) {
		g.toggleMusic()

		return
	}
	if key == ebiten.KeyQ {
		g.surfaceQuickSaveInput(game)

		return
	}
	g.gameState.BeginSurfaceCommandRouting()
	defer g.gameState.EndSurfaceCommandRouting()
	originMapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()
	result := newSurfaceCommandResult(surfaceCommandStillActive, originMapType)

	//nolint:exhaustive // Only town command keys belong here; Q is handled before routing and unbound keys return without completion.
	switch key {
	case ebiten.KeyD, ebiten.KeyW:
		g.gameState.SystemCallbacks.Message.AddRowStr(key.String() + "-What?")
		result = newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
		result.ConsumesNormalCycle = false
	case ebiten.KeySpace:
		result = surfaceResultFromSuccess(g.gameState.ActionPass(), originMapType)
		result.Pass = true
	case ebiten.KeyEnter:
		g.gameState.SystemCallbacks.Message.AddRowStr("What?")
		result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		result.ConsumesNormalCycle = false
	case ebiten.KeyPeriod:
		g.gameState.SystemCallbacks.Message.AddRowStr("What?")
		result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		result.ConsumesNormalCycle = false
	case ebiten.KeyUp:
		g.handleSurfaceCameraMovement(ebiten.KeyUp)

		return
	case ebiten.KeyDown:
		g.handleSurfaceCameraMovement(ebiten.KeyDown)

		return
	case ebiten.KeyLeft:
		g.handleSurfaceCameraMovement(ebiten.KeyLeft)

		return
	case ebiten.KeyRight:
		g.handleSurfaceCameraMovement(ebiten.KeyRight)

		return
	case ebiten.KeyB:
		result = surfaceResultFromSuccess(g.gameState.ActionBoard(), originMapType)
	case ebiten.KeyK:
		switch g.smallMapKlimb() {
		case game_state.SmallMapKlimbCompleted:
			if g.gameState.EventQueueManager.HasActiveWorkflows() {
				result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
				g.deferSurfaceCommandCompletion(result)
			} else {
				result = newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
			}
		case game_state.SmallMapKlimbRejected:
			result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
			result.ConsumesNormalCycle = false
		case game_state.SmallMapKlimbNeedsDirection:
		}
	case ebiten.KeyL:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Look-")
		g.secondaryKeyState = LookDirectionInput
	case ebiten.KeyX:
		result = surfaceResultFromSuccess(g.gameState.ActionExit(), originMapType)
	case ebiten.KeyG:
		// get the thing - direction
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Get-")
		g.secondaryKeyState = GetDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyE:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Enter what?")
		result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
	case ebiten.KeyP:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Push-")
		g.secondaryKeyState = PushDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyO:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Open-")
		g.secondaryKeyState = OpenDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyJ:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Jimmy-")
		if !g.gameState.PartyState.Inventory.Provisions.Keys.HasSome() && !g.gameState.HasAdjacentKnownSafeSurfaceChest() {
			g.gameState.ActionJimmySmallMapWithCharacter(references.NoneDirection, nil)
			result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)

			break
		}
		g.secondaryKeyState = JimmyDoorDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyI:
		result = surfaceResultFromSuccess(g.gameState.ActionIgnite(), originMapType)
	case ebiten.KeyT:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Talk-")
		g.secondaryKeyState = TalkDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyS:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Search-")
		g.secondaryKeyState = SearchDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyA:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Attack-")
		g.secondaryKeyState = AttackDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyU:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Use")
		g.startSurfaceUseWorkflow()
		if g.gameState.EventQueueManager.HasWorkflow("use_item_workflow") {
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
		} else {
			result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
			result.ConsumesNormalCycle = false
		}
	case ebiten.KeyY:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Yell")
		g.ShowYellDialogSmallMap()
		result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
	case ebiten.KeyR:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Ready...")
		request, outcome := g.gameState.BeginReadyEquipment(false)
		if outcome == game_state.ReadyBeginDialog {
			g.startReadyEquipmentWorkflowWithCompletion(request, func() {
				g.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandCompleted, originMapType))
			})
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
		} else {
			result = newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
		}
	case ebiten.KeyV:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("View...")
		if g.gameState.ActionViewSurface() {
			g.startGemViewWorkflowWithCompletion(func() {
				g.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandCompleted, originMapType))
			})
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
		} else {
			result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		}
	case ebiten.KeyZ:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Ztats...")
		g.gameState.ActionZtats()
		result = newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
		result.ConsumesNormalCycle = false
	case ebiten.KeyM:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Mix Reagents")
		if g.gameState.ActionMixSmallMap() {
			g.startReagentMixingWorkflowWithCompletion(func() {
				g.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandCompleted, originMapType))
			})
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
		} else {
			result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		}
	case ebiten.KeyC:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Cast...")
		g.startSpellCastingWorkflow()
		result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
	case ebiten.KeyN:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("New Order...")

		if g.gameState.ActionNewOrderSmallMap() {
			g.startNewOrderWorkflowWithCompletion(func() {
				g.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandCompleted, originMapType))
			})
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
		} else {
			result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		}
	case ebiten.KeyF:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Fire-")
		// Small map Fire auto-detects adjacent cannon - no direction input needed
		// ref:SHIP-0005: original cannon aiming checks 4 cardinal directions
		if g.gameState.ActionFireSmallMap() {
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
			g.deferSurfaceCommandCompletion(result)
		} else {
			result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		}
	case ebiten.KeyH:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Hole up & camp...")
		if g.gameState.ActionHoleUpSmallMap() {
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
			g.deferSurfaceCommandCompletion(result)
		} else {
			result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		}
	default:
		return
	}

	g.completeSurfaceCommand(result)
}

func smallMapMovementKey(direction references.Direction) ebiten.Key {
	switch direction {
	case references.Up:
		return ebiten.KeyUp
	case references.Down:
		return ebiten.KeyDown
	case references.Right:
		return ebiten.KeyRight
	case references.Left:
		return ebiten.KeyLeft
	case references.NoneDirection:
		panic("smashed command interception returned no direction")
	default:
		panic("smashed command interception returned an invalid direction")
	}
}

func (g *GameScene) smallMapHandleSecondaryInput() {
	if g.cancelSurfaceDirectionInput() {
		return
	}
	g.gameState.BeginSurfaceCommandRouting()
	defer g.gameState.EndSurfaceCommandRouting()

	originMapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()
	result := newSurfaceCommandResult(surfaceCommandStillActive, originMapType)
	directionInput := g.ordinaryDirectionInput()
	if directionInput.Outcome == ordinaryDirectionActive {
		g.completeSurfaceCommand(result)

		return
	}
	if directionInput.Outcome == ordinaryDirectionPassed {
		g.secondaryKeyState = PrimaryInput
		g.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandCompleted, originMapType))

		return
	}
	direction := directionInput.Direction

	//nolint:exhaustive // Only town direction states are actionable here; other owners route separately or recover via default.
	switch g.secondaryKeyState {
	case JimmyDoorDirectionInput:
		state := g.smallMapJimmySecondary(direction, func(state surfaceCommandState) {
			g.completeSurfaceCommand(newSurfaceCommandResult(state, originMapType))
		})
		g.secondaryKeyState = PrimaryInput
		result = newSurfaceCommandResult(state, originMapType)
	case OpenDirectionInput:
		state := g.surfaceOpenSecondaryWithDialog(direction, func(state surfaceCommandState) {
			g.completeSurfaceCommand(newSurfaceCommandResult(state, originMapType))
		})
		result = newSurfaceCommandResult(state, originMapType)
		g.secondaryKeyState = PrimaryInput
	case KlimbDirectionInput:
		g.smallMapKlimbSecondary(direction)
		g.secondaryKeyState = PrimaryInput
		if g.gameState.EventQueueManager.HasActiveWorkflows() {
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
			g.deferSurfaceCommandCompletion(result)
		} else {
			result = newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
		}
	case PushDirectionInput:
		g.smallMapPushSecondary(direction)
		g.secondaryKeyState = PrimaryInput
		result = newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
	case GetDirectionInput:
		g.smallMapGetSecondary(direction)
		g.secondaryKeyState = PrimaryInput
		result = newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
	case LookDirectionInput:
		success := g.gameState.ActionLookSmallMap(direction)
		if g.gameState.EventQueueManager.HasWorkflow(wishingWellWorkflowName) ||
			g.gameState.EventQueueManager.HasWorkflow("telescope_workflow") {
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
			g.deferSurfaceCommandCompletion(result)
		} else {
			result = surfaceResultFromSuccess(success, originMapType)
		}
		g.secondaryKeyState = PrimaryInput
	case TalkDirectionInput:
		// Delegate to GameState which handles all messages via SystemCallbacks (BUG25 fix)
		g.smallMapTalkSecondary(direction)
		g.secondaryKeyState = PrimaryInput
		result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
	case SearchDirectionInput:
		state := g.surfaceSearchSecondaryWithDialog(direction, func(state surfaceCommandState) {
			g.completeSurfaceCommand(newSurfaceCommandResult(state, originMapType))
		})
		g.secondaryKeyState = PrimaryInput
		result = newSurfaceCommandResult(state, originMapType)
	case AttackDirectionInput:
		success := g.smallMapAttackSecondary(direction)
		g.secondaryKeyState = PrimaryInput
		result = surfaceResultFromSuccess(success, originMapType)
		if g.gameState.IsInCombat() {
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
		}
	// CastDirectionInput removed - now handled by spell casting event queue
	// WindChangeScrollDirectionInput removed - now handled by DirectionPrompt dialog
	// YellDirectionInput removed - now handled by YellDialog
	// FireDirectionInput removed for small maps - Fire auto-detects adjacent cannon
	// See ActionFireSmallMap() in action_fire.go
	default:
		// UI state recovery: unexpected input state, reset to primary for graceful degradation
		logging.Warnf(logging.CategoryUI, "unexpected input state %v, resetting to primary", g.secondaryKeyState)
		g.secondaryKeyState = PrimaryInput
	}

	g.completeSurfaceCommand(result)
}

func (g *GameScene) smallMapKlimb() game_state.SmallMapKlimbStartOutcome {
	outcome := g.gameState.BeginSmallMapKlimb()
	switch outcome {
	case game_state.SmallMapKlimbNeedsDirection:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Klimb-")
		g.secondaryKeyState = KlimbDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case game_state.SmallMapKlimbRejected, game_state.SmallMapKlimbCompleted:
		g.secondaryKeyState = PrimaryInput
	}

	return outcome
}

func (g *GameScene) smallMapKlimbSecondary(direction references.Direction) {
	// Delegate all logic to GameState - it handles all feedback via SystemCallbacks
	g.gameState.ActionKlimbSmallMap(direction)
}

func (g *GameScene) smallMapPushSecondary(direction references.Direction) {
	pushThingPos := direction.GetNewPositionInDirection(&g.gameState.MapState.PlayerLocation.Position)
	pushThingTile := g.gameState.GetLayeredMapByCurrentLocation().GetTopTile(pushThingPos)

	// Early validation - avoid GameState call if obviously invalid
	if !pushThingTile.IsPushable() {
		g.output.AddText("Won't budge!") // Direct UI - no game logic

		return
	}

	// Delegate everything else to GameState - it handles all feedback via SystemCallbacks
	g.gameState.ActionPushSmallMap(direction)
}

func (g *GameScene) smallMapJimmySecondary(
	direction references.Direction,
	onComplete func(surfaceCommandState),
) surfaceCommandState {
	return g.smallMapJimmySecondaryWithDialog(direction, onComplete)
}

func (g *GameScene) smallMapGetSecondary(direction references.Direction) {
	// Delegate all logic to GameState - it handles all feedback via SystemCallbacks
	g.gameState.ActionGetSmallMap(direction)
}

func (g *GameScene) smallMapTalkSecondary(direction references.Direction) {
	// Use unified talk workflow with event queue
	// This fixes the "red screen stays forever" bug when talking to Shadowlord-affected NPCs
	g.startSurfaceTalkWorkflow(direction)
}

func (g *GameScene) smallMapAttackSecondary(direction references.Direction) bool {
	// The ActionAttackSmallMap already handles all messages via SystemCallbacks
	return g.gameState.ActionAttackSmallMap(direction)
}

// smallMapYellSecondary removed - now handled by YellDialog

// smallMapFireSecondary removed - Fire now auto-detects adjacent cannon without direction input
// See ActionFireSmallMap() in action_fire.go
