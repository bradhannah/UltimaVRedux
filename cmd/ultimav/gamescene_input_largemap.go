package main

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/pkg/semantic_input"
)

const (
	automaticSailingWorkflowName    = "automatic_sailing"
	automaticSailingPhaseIntervalMs = environment.WindTickIntervalMs
	shipBadlyDamagedHullThreshold   = 10
)

//nolint:maintidx // Keep outdoor command routing and each command's completion ownership together.
func (g *GameScene) largeMapInputHandlerWithGame(game *Game, key ebiten.Key) {
	g.syncAutomaticSailingInterruption()
	if g.sailingMovementHeldBeforeInterruption(key) {
		return
	}
	if g.interruptAutomaticSailingForCommand(key) {
		return
	}
	g.clearSurfaceCameraForInput(key)
	if g.keyboard.IsModifierComboPressed(input.ModifierCtrl, ebiten.KeyM) {
		g.toggleMusic()

		return
	}
	if key == ebiten.KeyQ {
		g.surfaceQuickSaveInput(game)

		return
	}
	var doomCandidate save_snapshot.SaveSnapshot
	var doomCandidateErr error
	if game != nil && key == ebiten.KeyE && g.gameState.IsDoomEnterAttempt() {
		doomCandidate, doomCandidateErr = g.CaptureSaveSnapshot()
	}

	g.gameState.BeginSurfaceCommandRouting()
	defer g.gameState.EndSurfaceCommandRouting()
	originMapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()
	result := newSurfaceCommandResult(surfaceCommandStillActive, originMapType)

	//nolint:exhaustive // Only outdoor command keys are admitted; unbound keys return without completing a command.
	switch key {
	case ebiten.KeyD, ebiten.KeyW:
		g.gameState.SystemCallbacks.Message.AddRowStr(key.String() + "-What?")
		result = newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
		result.ConsumesNormalCycle = false
	case ebiten.KeySpace:
		result = surfaceResultFromSuccess(g.gameState.ActionPass(), originMapType)
	case ebiten.KeyEnter:
		g.gameState.SystemCallbacks.Message.AddRowStr("What?")
		result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
	case ebiten.KeyPeriod:
		g.gameState.SystemCallbacks.Message.AddRowStr("What?")
		result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		result.ConsumesNormalCycle = false
	case ebiten.KeyUp:
		// If already sailing, ignore arrow keys (event queue is handling movement)
		if g.gameState.IsSailing() {
			return
		}
		// Check if automatic sailing should start (frigate with raised sails)
		if g.startAutomaticSailing(references.Up) {
			return // Sailing started - don't process normal movement
		}
		g.handleSurfaceCameraMovement(ebiten.KeyUp)

		return
	case ebiten.KeyDown:
		// If already sailing, ignore arrow keys
		if g.gameState.IsSailing() {
			return
		}
		// Check if automatic sailing should start
		if g.startAutomaticSailing(references.Down) {
			return // Sailing started - don't process normal movement
		}
		g.handleSurfaceCameraMovement(ebiten.KeyDown)

		return
	case ebiten.KeyLeft:
		// If already sailing, ignore arrow keys
		if g.gameState.IsSailing() {
			return
		}
		// Check if automatic sailing should start
		if g.startAutomaticSailing(references.Left) {
			return // Sailing started - don't process normal movement
		}
		g.handleSurfaceCameraMovement(ebiten.KeyLeft)

		return
	case ebiten.KeyRight:
		// If already sailing, ignore arrow keys
		if g.gameState.IsSailing() {
			return
		}
		// Check if automatic sailing should start
		if g.startAutomaticSailing(references.Right) {
			return // Sailing started - don't process normal movement
		}
		g.handleSurfaceCameraMovement(ebiten.KeyRight)

		return
	case ebiten.KeyB:
		result = surfaceResultFromSuccess(g.gameState.ActionBoard(), originMapType)
	case ebiten.KeyK:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Klimb-")
		g.secondaryKeyState = KlimbDirectionInput
	case ebiten.KeyG:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt(g.getPrompt())
		g.secondaryKeyState = GetDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyX:
		result = surfaceResultFromSuccess(g.gameState.ActionExit(), originMapType)
	case ebiten.KeyP:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Push-")
		g.secondaryKeyState = PushDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyL:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Look-")
		g.secondaryKeyState = LookDirectionInput
	case ebiten.KeyE:
		outcome := g.gameState.ResolveEnterLargeMap()
		if outcome == game_state.EnterLargeMapDoomTransition && game != nil {
			game.acceptDoomEntry(g, doomCandidate, doomCandidateErr)
		}
		if outcome.Accepted() && g.gameState.EventQueueManager.HasActiveWorkflows() {
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
			result.MapTransitioned = true
			g.deferSurfaceCommandCompletion(result)
		} else {
			result = surfaceResultFromSuccess(outcome.Accepted(), originMapType)
		}
	case ebiten.KeyO:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Open-")
		g.secondaryKeyState = OpenDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
	case ebiten.KeyJ:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Jimmy-")
		if !g.gameState.PartyState.Inventory.Provisions.Keys.HasSome() && !g.gameState.HasAdjacentKnownSafeSurfaceChest() {
			g.gameState.ActionJimmyLargeMapWithCharacter(references.NoneDirection, nil)
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
		success := g.gameState.ActionYellLargeMap()
		if g.gameState.EventQueueManager.HasWorkflow("word_of_power_entry") {
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
			g.deferSurfaceCommandCompletion(result)
		} else {
			result = surfaceResultFromSuccess(success, originMapType)
		}
	case ebiten.KeyF:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Fire-")
		g.secondaryKeyState = FireDirectionInput
		g.keyboard.SetAllowKeyPressImmediately()
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
		if g.gameState.ActionMixLargeMap() {
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

		if g.gameState.ActionNewOrderLargeMap() {
			g.startNewOrderWorkflowWithCompletion(func() {
				g.completeSurfaceCommand(newSurfaceCommandResult(surfaceCommandCompleted, originMapType))
			})
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
		} else {
			result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		}
	case ebiten.KeyH:
		if g.gameState.ActionHoleUpLargeMap() {
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
			// The camping workflow owns completion, including free setup cancellation.
			if !g.gameState.EventQueueManager.HasWorkflow("camping_workflow") {
				// Ship repair has its own workflow and retains ordinary completion.
				g.deferSurfaceCommandCompletion(result)
			}
		} else {
			result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		}
	default:
		return
	}

	g.completeSurfaceCommand(result)
}

func (g *GameScene) largeMapHandleSecondaryInput() {
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
		commandState := g.secondaryKeyState
		g.secondaryKeyState = PrimaryInput
		result = newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
		if commandState == AttackDirectionInput {
			result.ConsumesNormalCycle = false
		}
		g.completeSurfaceCommand(result)

		return
	}
	direction := directionInput.Direction

	//nolint:exhaustive // Only outdoor direction prompts dispatch here; other input owners reset to primary input.
	switch g.secondaryKeyState {
	case JimmyDoorDirectionInput:
		state := g.largeMapJimmySecondaryWithDialog(direction, func(state surfaceCommandState) {
			g.completeSurfaceCommand(newSurfaceCommandResult(state, originMapType))
		})
		result = newSurfaceCommandResult(state, originMapType)
		g.secondaryKeyState = PrimaryInput
	case KlimbDirectionInput:
		result = surfaceResultFromSuccess(g.gameState.ActionKlimbLargeMap(direction), originMapType)
		// A fall's damage lands in its workflow; finish the turn after it, as town Klimb does.
		if result.State == surfaceCommandCompleted && g.gameState.EventQueueManager.HasActiveWorkflows() {
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
			g.deferSurfaceCommandCompletion(result)
		}
		g.secondaryKeyState = PrimaryInput
	case LookDirectionInput:
		result = surfaceResultFromSuccess(g.gameState.ActionLookLargeMap(direction), originMapType)
		g.secondaryKeyState = PrimaryInput
	case GetDirectionInput:
		get := g.gameState.ActionGetLargeMap
		if g.getAll {
			get = g.gameState.ActionGetAllLargeMap
		}
		result = surfaceResultFromSuccess(get(direction), originMapType)
		g.secondaryKeyState = PrimaryInput
	case PushDirectionInput:
		result = surfaceResultFromSuccess(g.gameState.ActionPushLargeMap(direction), originMapType)
		g.secondaryKeyState = PrimaryInput
	case OpenDirectionInput:
		state := g.surfaceOpenSecondaryWithDialog(direction, func(state surfaceCommandState) {
			g.completeSurfaceCommand(newSurfaceCommandResult(state, originMapType))
		})
		result = newSurfaceCommandResult(state, originMapType)
		g.secondaryKeyState = PrimaryInput
	case TalkDirectionInput:
		result = surfaceResultFromSuccess(g.gameState.ActionTalkLargeMap(), originMapType)
		g.secondaryKeyState = PrimaryInput
	case SearchDirectionInput:
		state := g.surfaceSearchSecondaryWithDialog(direction, func(state surfaceCommandState) {
			g.completeSurfaceCommand(newSurfaceCommandResult(state, originMapType))
		})
		result = newSurfaceCommandResult(state, originMapType)
		g.secondaryKeyState = PrimaryInput
	case AttackDirectionInput:
		result = surfaceResultFromSuccess(g.gameState.ActionAttackLargeMap(direction), originMapType)
		g.secondaryKeyState = PrimaryInput
		g.secondaryKeyState = PrimaryInput
	// YellDirectionInput removed - now handled by YellDialog
	// WindChangeScrollDirectionInput removed - now handled by DirectionPrompt dialog
	case FireDirectionInput:
		if g.gameState.ActionFireLargeMap(direction) {
			result = newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
			g.deferSurfaceCommandCompletion(result)
		} else {
			result = newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		}
		g.secondaryKeyState = PrimaryInput
	// CastDirectionInput removed - now handled by spell casting event queue
	default:
		// better safe than sorry
		g.secondaryKeyState = PrimaryInput
	}
	g.completeSurfaceCommand(result)
}

// startAutomaticSailing begins automatic sailing mode for frigates with raised sails
// Uses a deterministic input-first phase loop to match original Ultima V sailing behavior:
// - Same arrow key: continues the current cadence
// - Different arrow key: changes direction (continues sailing)
// - Space bar: "Sheets in irons!" (stops sailing, key consumed)
// - Any other key: stops sailing AND processes key normally
// Returns true if sailing started, false if requirements not met
//
//nolint:maintidx // Sailing cadence and caller-owned outcomes form one production workflow.
func (g *GameScene) startAutomaticSailing(direction references.Direction) bool {
	// The sailing flag is cleared in Finally before the queue name is released.
	// Consume movement input while that workflow is still finalizing instead of
	// attempting to reserve the same programming-error-protected name again.
	if g.gameState.EventQueueManager.HasWorkflow(automaticSailingWorkflowName) {
		return true
	}
	// Validate sailing requirements (frigate with raised sails)
	if !g.gameState.StartAutomaticSailing(direction) {
		return false
	}

	// Show user feedback
	g.output.AddText("Sailing!")

	var waitCounter int
	var stopped bool
	var interruptedByInput bool
	var consumeStopInput bool
	var hadCollision bool
	var hadEnemyContact bool

	// Start event queue for automatic sailing with key-aware callback
	logging.Debugf(logging.CategoryGameState, "SAILING: Starting automatic_sailing queue")
	g.gameState.EventQueueManager.Workflow(automaticSailingWorkflowName).
		Priority(event_queue.PriorityNormal).
		Sequential().
		First(func() {}).
		While(
			func() bool { return !stopped && g.gameState.ContinueAutomaticSailing() },
			func() {
				g.surfaceCamera = surfaceCameraProjection{}
				key := g.GetPressedKey()
				if key != nil {
					pressedDirection := automaticSailingDirection(*key)
					if pressedDirection == references.NoneDirection {
						stopped = true
						interruptedByInput = true
						if *key == semantic_input.KeySpace {
							consumeStopInput = true
							g.gameState.SystemCallbacks.Message.AddRowStr("Sheets in irons!")
						}

						return
					}

					vehicle := g.gameState.PartyVehicle.GetVehicleDetails()
					if pressedDirection != vehicle.CurrentDirection() {
						vehicle.SetPartyVehicleDirection(pressedDirection)
						waitCounter = 0

						return
					}
				}

				currentDirection := g.gameState.PartyVehicle.GetVehicleDetails().CurrentDirection()
				requiredWaits, canMove := g.gameState.AutomaticSailingWaitPhases(currentDirection)
				if !canMove || waitCounter < requiredWaits {
					g.gameState.CompleteAutomaticSailingWait()
					waitCounter++
					if g.gameState.HasPendingOverworldContact() {
						hadEnemyContact = true
						g.gameState.StopAutomaticSailingForCombat()
						stopped = true
					}

					return
				}

				waitCounter = 0
				origin := g.gameState.MapState.PlayerLocation
				originLayer := g.gameState.GetLayeredMapByCurrentLocation()
				expected := origin
				expected.Position = *currentDirection.GetNewPositionInDirection(&origin.Position).
					GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
				result := g.handleAutomaticSailingMovement(currentDirection)
				g.completeSurfaceMovement(result)
				if g.gameState.HasPendingOverworldContact() {
					hadEnemyContact = true
					g.gameState.StopAutomaticSailingForCombat()
					stopped = true

					return
				}
				if result.State == surfaceCommandRejected || result.State == surfaceCommandCancelled {
					hadCollision = true
					g.gameState.StopAutomaticSailing()
					stopped = true

					return
				}
				if result.MovementOccurred && g.gameState.IsPlayerCurrentlyOnWhirlpool() {
					vehicle := g.gameState.PartyVehicle.GetVehicleDetails()
					if vehicle.VehicleType == references.FrigateVehicle {
						vehicle.SetSailState(map_units.SailFurled)
					}
					g.gameState.StopAutomaticSailing()
					stopped = true
				}
				if result.State == surfaceCommandCompleted && result.MovementOccurred && !result.MapTransitioned {
					g.installAutomaticSailingCamera(origin, expected, originLayer)
				}
			},
			automaticSailingPhaseIntervalMs,
		).If(

		func() bool { return hadEnemyContact }, func(q *event_queue.SequentialEventBuilder) {
			q.Then(func() {
				g.gameState.StopAutomaticSailingForCombat()
			})
		}).If(

		func() bool { return hadCollision }, func(q *event_queue.SequentialEventBuilder) {
			direction := g.gameState.PartyVehicle.GetVehicleDetails().CurrentDirection()
			targetPos := direction.GetNewPositionInDirection(&g.gameState.MapState.PlayerLocation.Position).
				GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
			collisionKind := g.gameState.ClassifySailingCollision(*targetPos)

			if collisionKind == game_state.SailingCollisionPier {
				q.Then(func() {
					result := g.gameState.ResolveSailingCollision(*targetPos)
					if result.Kind != game_state.SailingCollisionPier || result.HasHit {
						panic("reviewed pier collision changed before resolution")
					}
					g.output.AddText("Docked!")
				})

				return
			}
			if collisionKind == game_state.SailingCollisionNone {
				return
			}

			collisionMsg := "COLLISION!"
			isShoalCollision := collisionKind == game_state.SailingCollisionShoal
			if isShoalCollision {
				collisionMsg = "BREAKING UP!"
			}
			var hitResult game_state.SourceHitResult

			q.Then(func() {
				result := g.gameState.ResolveSailingCollision(*targetPos)
				if result.Kind != collisionKind || !result.HasHit {
					panic("reviewed sailing collision changed before resolution")
				}
				hitResult = result.Hit
				g.output.AddText(collisionMsg)

				if isShoalCollision {
					g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundShoalScrape)
				}
			}).ThenSequence(

				game_state.NewPartyStateBuilder(g.gameState).
					FlashWater().
					WithSound(game_state.SoundGlassBreak).
					Build()).
				Wait(1000).
				Then(func() {
					g.output.AddText(fmt.Sprintf("Hull took %d damage!", hitResult.HullDamage))
					switch hitResult.Outcome {
					case game_state.SourceHitFrigateDamaged:
						g.output.AddText(fmt.Sprintf("Hull: %d -> %d HP", hitResult.HullBefore, hitResult.HullAfter))
					case game_state.SourceHitEscapedBySkiff:
						g.output.AddText("Ship sunk! Abandon ship!")
						g.output.AddText("You escape in a carried skiff!")
					case game_state.SourceHitEscapedByCarpet:
						g.output.AddText("Ship sunk! Abandon ship!")
						g.output.AddText("You escape on the magic carpet!")
					case game_state.SourceHitDrowned:
						g.output.AddText("Ship sunk! DROWNING!!!")
					case game_state.SourceHitPartyDamaged:
						panic("sailing collision resolved as non-frigate party damage")
					}
				}).
				Wait(500).
				Then(func() {
					if hitResult.Outcome == game_state.SourceHitFrigateDamaged &&
						hitResult.HullAfter < shipBadlyDamagedHullThreshold {
						g.output.AddText("DANGER: SHIP BADLY DAMAGED!")
					}
				}).
				Then(func() {
					if hitResult.PartyWiped {
						g.gameState.SystemCallbacks.Flow.Consequences.TriggerDeathSequence()
					}
				})
		}).Finally(

		func() {
			g.surfaceCamera = surfaceCameraProjection{}
			logging.Debugf(logging.CategoryGameState, "SAILING: Finally() executing - automatic sailing ended")
			if interruptedByInput {
				g.gameState.CompleteAutomaticSailingInterruption()
			} else {
				g.gameState.StopAutomaticSailing()
				if hadCollision {
					g.gameState.ProcessPendingProvisionHours()
				}
			}
			if consumeStopInput {
				g.keyboard.PrepareForTransition(0)
			}
			g.syncAutomaticSailingInterruption()
		}).
		Start()

	return true
}

func automaticSailingDirection(key semantic_input.Key) references.Direction {
	switch key {
	case semantic_input.KeyUp:
		return references.Up
	case semantic_input.KeyDown:
		return references.Down
	case semantic_input.KeyLeft:
		return references.Left
	case semantic_input.KeyRight:
		return references.Right
	case semantic_input.KeyOther, semantic_input.KeySpace, semantic_input.KeyEnter:
		return references.NoneDirection
	}

	panic("unknown semantic sailing key")
}

func automaticSailingDirectionKey(direction references.Direction) ebiten.Key {
	switch direction {
	case references.Up:
		return ebiten.KeyUp
	case references.Down:
		return ebiten.KeyDown
	case references.Left:
		return ebiten.KeyLeft
	case references.Right:
		return ebiten.KeyRight
	case references.NoneDirection:
		panic("automatic sailing requires a cardinal direction")
	default:
		panic("unknown sailing direction")
	}
}
