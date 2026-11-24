package main

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const dungeonYellPrompt = "What dost thou yell?"

const dungeonUnboundKeyFeedback = "What?"

var dungeonKlimbChoiceKeys = boundKeysGame //nolint:gochecknoglobals // Immutable input binding table.

//nolint:maintidx // Keep accepted-key precharge, token handoffs, and exact-once completion in one auditable dispatcher.
func (g *GameScene) dungeonMapInputHandlerWithGame(game *Game, key ebiten.Key) {
	if g.keyboard.IsModifierComboPressed(input.ModifierCtrl, ebiten.KeyM) {
		g.toggleMusic()

		return
	}
	if key == ebiten.KeyZ {
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Ztats...")
		g.gameState.ActionZtats()

		return
	}

	//nolint:exhaustive // Only dungeon command keys acquire a token; shortcuts above and rejected keys do not precharge.
	switch key {
	case ebiten.KeyUp,
		ebiten.KeyDown,
		ebiten.KeyLeft,
		ebiten.KeyRight,
		ebiten.KeySpace,
		ebiten.KeyEnter,
		ebiten.KeyPeriod,
		ebiten.KeyA,
		ebiten.KeyB,
		ebiten.KeyC,
		ebiten.KeyD,
		ebiten.KeyE,
		ebiten.KeyF,
		ebiten.KeyG,
		ebiten.KeyH,
		ebiten.KeyI,
		ebiten.KeyJ,
		ebiten.KeyK,
		ebiten.KeyL,
		ebiten.KeyM,
		ebiten.KeyN,
		ebiten.KeyO,
		ebiten.KeyP,
		ebiten.KeyQ,
		ebiten.KeyR,
		ebiten.KeyS,
		ebiten.KeyT,
		ebiten.KeyU,
		ebiten.KeyV,
		ebiten.KeyW,
		ebiten.KeyX,
		ebiten.KeyY:
	default:
		g.secondaryKeyState = PrimaryInput
		g.gameState.SystemCallbacks.Message.AddRowStr(dungeonUnboundKeyFeedback)

		return
	}

	var token game_state.DungeonCommandCycle
	if key == ebiten.KeyC || key == ebiten.KeyU {
		token = g.gameState.BeginDeferredDungeonCommandCycle()
	} else {
		token, _ = g.gameState.BeginDungeonCommandCycle()
	}
	var completion game_state.DungeonCommandCompletion

	//nolint:exhaustive // Accepted keys are gated above; default protects the token if the gates diverge.
	switch key {
	case ebiten.KeyD, ebiten.KeyW:
		g.gameState.SystemCallbacks.Message.AddRowStr(key.String() + "-What?")
		completion = game_state.DungeonCommandOnly
	case ebiten.KeySpace:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Pass")
		completion = game_state.DungeonFullEndPlayer
	case ebiten.KeyA:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Attack")
		g.gameState.ActionAttackDungeonMap(references.NoneDirection)
		completion = game_state.DungeonCommandOnly
	case ebiten.KeyB:
		g.gameState.ActionBoard()
		completion = game_state.DungeonFullEndPlayer
	case ebiten.KeyUp:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Advance")
		result := g.gameState.TryDungeonAdvance()
		completion = result.Completion
		if result.TriggersHiddenPit {
			g.gameState.ActionDungeonHiddenPitFall(token, result)

			return
		}
		if result.Outcome != game_state.DungeonMoveMoved &&
			result.Outcome != game_state.DungeonMoveElectricField {
			g.gameState.SystemCallbacks.Message.AddRowStr("Blocked!")
		}
	case ebiten.KeyDown:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Back up")
		result := g.gameState.TryDungeonBackUp()
		completion = result.Completion
		if result.TriggersHiddenPit {
			g.gameState.ActionDungeonHiddenPitFall(token, result)

			return
		}
		if result.Outcome != game_state.DungeonMoveMoved &&
			result.Outcome != game_state.DungeonMoveElectricField {
			g.gameState.SystemCallbacks.Message.AddRowStr("Blocked!")
		}
	case ebiten.KeyLeft:
		result := g.gameState.TurnDungeon(game_state.DungeonTurnLeft)
		completion = result.Completion
		if result.Outcome == game_state.DungeonTurnBlockedInDoorway {
			g.gameState.SystemCallbacks.Message.AddRowStr("Not in doorway!")
		} else {
			g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Turn left")
		}
	case ebiten.KeyRight:
		result := g.gameState.TurnDungeon(game_state.DungeonTurnRight)
		completion = result.Completion
		if result.Outcome == game_state.DungeonTurnBlockedInDoorway {
			g.gameState.SystemCallbacks.Message.AddRowStr("Not in doorway!")
		} else {
			g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Turn right")
		}
	case ebiten.KeyEnter, ebiten.KeyPeriod:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Turn around.")
		result := g.gameState.TurnDungeon(game_state.DungeonTurnAround)
		completion = result.Completion
	case ebiten.KeyC:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Cast...")
		g.startDungeonSpellCastingWorkflow(token)

		return
	case ebiten.KeyE:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Enter what?")
		completion = game_state.DungeonFullEndPlayer
	case ebiten.KeyF:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Fire-")
		g.gameState.SystemCallbacks.Message.AppendToCurrentRowStr("What?")
		completion = game_state.DungeonFullEndPlayer
	case ebiten.KeyI:
		g.gameState.ActionIgnite()
		completion = game_state.DungeonFullEndPlayer
	case ebiten.KeyG:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Get-")
		g.gameState.ActionGetDungeonMap()
		completion = game_state.DungeonFullEndPlayer
	case ebiten.KeyH:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Hole up & camp...")
		g.gameState.ActionHoleUpDungeonMap(token)
		g.startDungeonHoleUpWorkflow(token)

		return
	case ebiten.KeyJ:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Jimmy-")
		g.dungeonJimmyCommandToken = token
		g.startDungeonJimmyWorkflow()

		return
	case ebiten.KeyK:
		options := g.gameState.DungeonKlimbOptionsAtCurrentCell()
		if !options.Up && !options.Down {
			if options.GrappleRequired {
				g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Klimb-")
				g.gameState.SystemCallbacks.Message.AddRowStr("With What?")
			} else {
				g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Klimb-what?")
			}
			g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonCommandOnly)

			return
		}
		if options.Up && options.Down {
			g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Klimb-U/D-")
			g.dungeonKlimbCommandToken = token
			g.secondaryKeyState = KlimbDirectionInput
			g.keyboard.SetAllowKeyPressImmediately()

			return
		}

		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Klimb-")
		direction := game_state.DungeonKlimbDown
		if options.Up {
			direction = game_state.DungeonKlimbUp
		}
		g.gameState.ActionKlimbDungeonMap(token, direction)

		return
	case ebiten.KeyL:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Look...")
		g.dungeonLookCommandToken = token
		g.startDungeonLookWorkflow()

		return
	case ebiten.KeyM:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Mix Reagents")
		g.gameState.ClaimDungeonFullCommand(token)
		if !g.gameState.ActionMixDungeonMap() {
			g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)

			return
		}
		g.startReagentMixingWorkflowWithCompletion(func() {
			g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)
		})

		return
	case ebiten.KeyN:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("New Order...")
		g.gameState.ClaimDungeonFullCommand(token)
		if !g.gameState.ActionNewOrderDungeonMap() {
			g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)

			return
		}
		g.startNewOrderWorkflowWithCompletion(func() {
			g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)
		})

		return
	case ebiten.KeyO:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Open-")
		if !g.gameState.DungeonOpenRequiresCharacter() {
			g.gameState.ActionOpenDungeonMap(-1)
			completion = game_state.DungeonFullEndPlayer

			break
		}
		g.dungeonOpenCommandToken = token
		g.startDungeonOpenWorkflow()

		return
	case ebiten.KeyP:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Push")
		g.gameState.ActionPushDungeonMap()
		completion = game_state.DungeonCommandOnly
	case ebiten.KeyQ:
		if !g.confirmQuickSave() {
			g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonCommandOnly)
			g.saveCurrentPlayerWithConfirmation(game, "Dungeon", false)

			return
		}
		g.showSaveQuestion()
		g.dungeonQuitCommandToken = token
		g.secondaryKeyState = DungeonQuitInput
		g.keyboard.SetAllowKeyPressImmediately()

		return
	case ebiten.KeyR:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Ready...")
		request, outcome := g.gameState.BeginReadyEquipment(false)
		if outcome != game_state.ReadyBeginDialog {
			completion = game_state.DungeonFullEndPlayer

			break
		}
		g.gameState.ClaimDungeonFullCommand(token)
		g.startReadyEquipmentWorkflowWithCompletion(request, func() {
			g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)
		})

		return
	case ebiten.KeyS:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Search...")
		g.dungeonSearchCommandToken = token
		g.startDungeonSearchWorkflow()

		return
	case ebiten.KeyT:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Talk-")
		g.gameState.SystemCallbacks.Message.AppendToCurrentRowStr("Funny, no response!")
		completion = game_state.DungeonFullEndPlayer
	case ebiten.KeyU:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Use")
		g.gameState.ClaimDungeonFullCommand(token)
		g.StartDungeonUseWorkflow(token)

		return
	case ebiten.KeyX:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("X-it ")
		// REDUX DEVIATION: CMD-0002: original Yell has an impossible dungeon guard and can
		// corrupt malformed vehicle state. Redux preserves the canonical
		// on-foot observable result without admitting dungeon vehicles.
		g.gameState.SystemCallbacks.Message.AppendToCurrentRowStr("what?")
		completion = game_state.DungeonFullEndPlayer
	case ebiten.KeyY:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Yell ")
		g.startDungeonYellWorkflow(token)

		return
	case ebiten.KeyV:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("View...")
		model, ok := g.gameState.ActionViewDungeonMap(token)
		if !ok {
			g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)

			return
		}
		g.StartDungeonViewWorkflow(token, model)

		return
	default:
		panic("accepted dungeon key has no command handler")
	}

	g.gameState.CompleteDungeonCommandCycle(token, completion)
}

func (g *GameScene) dungeonMapHandleSecondaryInput(game *Game) {
	//nolint:exhaustive // Only dungeon-owned secondary states are actionable here.
	switch g.secondaryKeyState {
	case LookDirectionInput:
		g.dungeonLookSecondaryInput()
	case SearchDirectionInput:
		g.dungeonSearchSecondaryInput()
	case KlimbDirectionInput:
		g.dungeonKlimbSecondaryInput()
	case DungeonQuitInput:
		g.dungeonQuitSecondaryInput(game)
	default:
		g.secondaryKeyState = PrimaryInput
	}
}

func (g *GameScene) dungeonQuitSecondaryInput(game *Game) {
	if g.gameState.IsAtDungeonCommandBoundary() {
		g.dungeonQuitCommandToken = game_state.DungeonCommandCycle{}
		g.secondaryKeyState = PrimaryInput
		g.clearSaveQuestion()

		return
	}
	key := g.keyboard.GetBoundKeyPressed(&boundKeysGame)
	if key == nil || !g.keyboard.TryToRegisterKeyPress(*key) {
		return
	}
	if *key != ebiten.KeyY && *key != ebiten.KeyN && *key != ebiten.KeyEscape {
		return
	}

	token := g.dungeonQuitCommandToken
	g.dungeonQuitCommandToken = game_state.DungeonCommandCycle{}
	g.secondaryKeyState = PrimaryInput
	g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonCommandOnly)
	g.clearSaveQuestion()
	if *key != ebiten.KeyY {
		g.gameState.SystemCallbacks.Message.AddRowStr("Save game? No")

		return
	}

	g.gameState.SystemCallbacks.Message.AddRowStr("Save game? Yes")
	g.saveCurrentPlayer(game, "Dungeon")
}

func (g *GameScene) startDungeonYellWorkflow(token game_state.DungeonCommandCycle) {
	g.gameState.ClaimDungeonFullCommand(token)
	g.gameState.EventQueueManager.Workflow("dungeon_yell").
		Priority(event_queue.PriorityMedium).
		Sequential().BlockInput().
		First(func() {}).
		ThenSequence(g.thenShowInputBoxWithEscapeClear(
			dungeonYellPrompt,
			"^[ -~]{0,30}$",
			func(text string, submitted bool) {
				if !submitted {
					text = ""
				}
				g.gameState.ActionYellWithoutEffect(text)
			},
		)).
		Finally(func() {
			// REDUX DEVIATION: CMD-0002: original Yell returns without a value for empty
			// input. Redux full-completes deterministically after "Nothing".
			g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)
		}).
		Start()
}

func (g *GameScene) dungeonSearchSecondaryInput() {
	if g.gameState.IsAtDungeonCommandBoundary() {
		g.dungeonSearchCommandToken = game_state.DungeonCommandCycle{}
		g.secondaryKeyState = PrimaryInput

		return
	}
	key := g.keyboard.GetBoundKeyPressed(&boundKeysGame)
	if key == nil || !g.keyboard.TryToRegisterKeyPress(*key) {
		return
	}

	var direction references.Direction
	//nolint:exhaustive // Only original Ahead/Here/Left/Right/Pass choices resolve Search.
	switch *key {
	case ebiten.KeyUp, ebiten.KeyNumpad8:
		direction = references.Up
	case ebiten.KeyDown, ebiten.KeyNumpad2:
		direction = references.Down
	case ebiten.KeyLeft, ebiten.KeyNumpad4:
		direction = references.Left
	case ebiten.KeyRight, ebiten.KeyNumpad6:
		direction = references.Right
	case ebiten.KeySpace, ebiten.KeyEscape:
		token := g.dungeonSearchCommandToken
		g.dungeonSearchCommandToken = game_state.DungeonCommandCycle{}
		g.secondaryKeyState = PrimaryInput
		g.gameState.SystemCallbacks.Message.AddRowStr("Pass")
		g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)

		return
	default:
		return
	}

	token := g.dungeonSearchCommandToken
	g.dungeonSearchCommandToken = game_state.DungeonCommandCycle{}
	g.secondaryKeyState = PrimaryInput
	result := g.gameState.ResolveDungeonSearchWithCharacter(direction, g.dungeonSearchCharacterIndex)
	if result.CrumbledSkeletonWall {
		g.startDungeonSkeletonCrumble(result.Target)
	}
	if result.RevealedHiddenPit {
		g.startDungeonPitReveal(result.Target)
	}
	g.gameState.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)
}

func (g *GameScene) dungeonKlimbSecondaryInput() {
	if g.gameState.IsAtDungeonCommandBoundary() {
		g.dungeonKlimbCommandToken = game_state.DungeonCommandCycle{}
		g.secondaryKeyState = PrimaryInput

		return
	}
	key := g.keyboard.GetBoundKeyPressed(&dungeonKlimbChoiceKeys)
	if key == nil || !g.keyboard.TryToRegisterKeyPress(*key) {
		return
	}

	var direction game_state.DungeonKlimbDirection
	//nolint:exhaustive // Only original Up/Down/Pass choices resolve this retained command.
	switch *key {
	case ebiten.KeyUp, ebiten.KeyU, ebiten.KeyNumpad8:
		direction = game_state.DungeonKlimbUp
	case ebiten.KeyDown, ebiten.KeyD, ebiten.KeyNumpad2:
		direction = game_state.DungeonKlimbDown
	case ebiten.KeySpace, ebiten.KeyEscape:
		token := g.dungeonKlimbCommandToken
		g.dungeonKlimbCommandToken = game_state.DungeonCommandCycle{}
		g.secondaryKeyState = PrimaryInput
		g.gameState.CompleteDungeonKlimbPass(token)

		return
	default:
		return
	}

	token := g.dungeonKlimbCommandToken
	g.dungeonKlimbCommandToken = game_state.DungeonCommandCycle{}
	g.secondaryKeyState = PrimaryInput
	g.gameState.ActionKlimbDungeonMap(token, direction)
}
