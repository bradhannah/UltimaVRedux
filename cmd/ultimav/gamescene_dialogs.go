package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func (g *GameScene) startReagentMixingWorkflowWithCompletion(complete func()) {
	session := &spellMixSession{}
	g.gameState.EventQueueManager.Workflow("reagent_mixing_workflow").Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().First(func() {}).ThenSequence(g.thenSpellMixSession(session)).
		Then(func() {
			if complete != nil {
				complete()
			}
		}).
		Finally(func() { g.cleanupSpellMixSession(session) }).Start()
}

// spellCastingWorkflowState holds all state for the spell casting workflow.
// It tracks the multi-stage spell casting process: caster selection → spell selection → targeting → execution.
type spellCastingWorkflowState struct {
	// Dialogs - UI components active during workflow stages
	casterDialog      *widgets.CharacterSelectionDialog // "Who will cast?" dialog
	spellDialog       *widgets.SpellCastingDialog       // Main spell selection dialog
	targetingDialog   *widgets.CharacterSelectionDialog // "On who?" dialog for character targets
	gemViewDialog     *widgets.GemViewDialog
	dungeonViewDialog *widgets.DungeonViewDialog
	phaseInputDialog  *widgets.InputBox

	// Selection results
	selectedCasterIdx  int                              // Index of character casting the spell
	targetCharacterIdx int                              // Index selected by TargetPartyMember.
	pendingRequest     game_state.CastRequest           // The spell cast request from dialog
	targetingProfile   references.SpellTargetingProfile // Context-resolved selection and scope.
	validationResult   game_state.SpellValidationResult // Result of spell validation

	// Status flags
	casterSelected                bool // true when caster has been chosen (or auto-selected)
	casterCancelled               bool // true if user cancelled caster selection
	noOneCanCast                  bool // true if no spell-capable characters exist
	hasRequest                    bool // true when pendingRequest is populated
	targetSelected                bool // true when target selection is complete
	restoreXRay                   bool
	previousXRay                  bool
	moonPhaseSelected             bool
	moongateDestination           game_state.MoongateDestination
	teleportEffectsOwned          bool
	dungeonViewModel              game_state.DungeonViewModel
	dungeonCommandToken           game_state.DungeonCommandCycle
	ownsDungeonCommand            bool
	mixSession                    *spellMixSession
	surfaceOriginMapType          references.GeneralMapType
	surfaceOriginLocation         references.Location
	ownsSurfaceCommand            bool
	ownsCombatAction              bool
	combatActionTransferred       bool
	processProvisionHoursOnFinish bool
	commitSequence                uint64
	castOrigin                    references.PlayerLocation
	combatCaster                  game_state.CombatTurnIdentity
	combatTiming                  combatPresentationTiming
}

const castFailureDialogDurationMs = 2000

// castCancelLabel ends every prompt that follows Cast's list. It says at the one
// moment it matters that backing out costs no MP, mixture or turn.
const (
	castCancelLabel = "Cancel, nothing spent"
	castActionLabel = "Cast"
)

// startSpellCastingWorkflow initiates the spell casting workflow.
// Flow: "Who will cast?" → SpellCastingDialog → Targeting → Spell Effect
//
// The workflow begins by showing a caster selection dialog ("Who will cast?") filtered to
// spell-capable characters (alive with MP capacity). If no one can cast, shows error and aborts.
//
// When the player presses Enter with a spell selected, the dialog creates a CastRequest which
// this workflow consumes and routes to appropriate targeting based on spell's TargetingType:
//   - TargetAutomatic: Validate, execute via event queue builder
//   - TargetPartyMember: Show "On who?" CharacterSelectionDialog, validate, execute via builder
//   - TargetDirection: Show direction prompt, validate, execute via builder
//   - TargetAdjacentTile: Combat adjacent selector
//   - TargetCombatantCrosshair/TargetTileCrosshair: Combat crosshair
//
// All spell effects use event queue builders (thenCast*) for proper staging with
// PartyStateBuilder visual effects, input blocking, and guaranteed cleanup.
func (g *GameScene) startSpellCastingWorkflow() {
	g.startSpellCastingWorkflowWithDungeonCommand(nil)
}

func (g *GameScene) startDungeonSpellCastingWorkflow(token game_state.DungeonCommandCycle) {
	g.startSpellCastingWorkflowWithDungeonCommand(&token)
}

//nolint:maintidx // Existing staged Cast orchestrator; token ownership is one additional workflow concern.
func (g *GameScene) startSpellCastingWorkflowWithDungeonCommand(token *game_state.DungeonCommandCycle) {
	state := &spellCastingWorkflowState{
		commitSequence:        g.gameState.CastUseCommitSequence(),
		castOrigin:            g.gameState.MapState.PlayerLocation,
		surfaceOriginMapType:  g.gameState.MapState.PlayerLocation.Location.GetMapType(),
		surfaceOriginLocation: g.gameState.MapState.PlayerLocation.Location,
		ownsCombatAction:      g.gameState.GetCurrentSpellContext() == references.ContextCombat,
	}
	if state.ownsCombatAction {
		state.combatCaster = g.gameState.CombatScene.GetCurrentTurnIdentity()
	}
	if token != nil {
		state.dungeonCommandToken = *token
		state.ownsDungeonCommand = true
	}
	context := g.gameState.GetCurrentSpellContext()
	if context == references.ContextOverworld || context == references.ContextTown ||
		context == references.ContextNonCombat {
		state.ownsSurfaceCommand = true
		g.surfaceSpellWorkflowOwnsCycle = true
	}

	g.gameState.EventQueueManager.Workflow("spell_casting_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			if g.gameState.GetCurrentSpellContext() == references.ContextCombat {
				activeIdx := g.gameState.CombatScene.GetActiveCombatantIndex()
				activeCaster := g.gameState.PartyState.GetCharacter(activeIdx)
				if activeCaster == nil || !activeCaster.IsAlive() {
					state.noOneCanCast = true
					g.showTemporaryCastFailure("Thou canst not cast now!")

					return
				}
				if !activeCaster.CanCastSpells() {
					state.noOneCanCast = true
					g.showTemporaryCastFailure(activeCaster.GetNameAsString() + " has no magical ability!")

					return
				}
				state.selectedCasterIdx = activeIdx
				state.casterSelected = true

				return
			}

			spellCapableCount := g.countSpellCapableCharacters()
			if spellCapableCount == 0 {
				state.noOneCanCast = true
				g.showTemporaryCastFailure("No one in the party has magical ability!")

				return
			}

			if g.gameState.HasActivePlayer() {
				activePlayer := g.gameState.GetActivePlayer()
				if activePlayer != nil && isSpellCapable(activePlayer) {
					state.selectedCasterIdx = g.gameState.GetActivePlayerIndex()
					state.casterSelected = true

					return
				}
			}

			state.casterDialog = widgets.NewCharacterSelectionDialog(widgets.CharacterSelectionConfig{
				Title:   "Cast: Choose Caster",
				Options: g.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionCaster}),
				OnSelect: func(idx int) {
					state.selectedCasterIdx = idx
					state.casterSelected = true
					g.dialogStack.RemoveWidgetInstance(state.casterDialog)
				},
				OnCancel: func() {
					state.casterCancelled = true
					state.casterSelected = true
					g.dialogStack.RemoveWidgetInstance(state.casterDialog)
				},
				Keyboard:    g.keyboard,
				SpriteSheet: g.spriteSheet,
				Placement:   gameScreenPercents,
			})
			g.dialogStack.PushModalDialog(state.casterDialog)
		}).
		While(func() bool {
			if state.noOneCanCast || state.casterSelected {
				return false
			}

			return true
		}, func() {

		}, DialogPollingRateMs).Then(func() {
		if state.noOneCanCast || state.casterCancelled || !state.casterSelected {
			return
		}
		caster := g.gameState.PartyState.GetCharacter(state.selectedCasterIdx)
		unlimitedResources := g.gameState.DebugOptions.UnlimitedCastingResources && caster.GetMaxMp() > 0
		if !unlimitedResources && caster.GetCurrentMp() == 0 {
			state.noOneCanCast = true
			g.showTemporaryCastFailure(caster.GetNameAsString() + " has no MP remaining!")
		}
	}).If(

		func() bool { return !state.noOneCanCast && !state.casterCancelled },
		func(subQ *event_queue.SequentialEventBuilder) {
			subQ.ThenSequence(g.thenChooseSpell(state))
		}).If(

		func() bool { return state.hasRequest && state.targetingProfile.Selection == references.TargetMoonPhase },
		func(subQ *event_queue.SequentialEventBuilder) {
			subQ.Then(func() {
				state.validationResult = g.gameState.ValidateVasRelPorPrerequisites(state.pendingRequest)
			}).If(
				func() bool { return !state.validationResult.Valid },
				func(errQ *event_queue.SequentialEventBuilder) {
					g.thenShowSpellValidationError(state.validationResult)(errQ)
				},
			).If(
				func() bool { return state.validationResult.Valid },
				func(phaseQ *event_queue.SequentialEventBuilder) {
					phaseQ.ThenSequence(g.thenShowInputBox(
						"To which phase, 1 to 8?",
						"^[1-8]$",
						func(text string, submitted bool) {
							phase, selected := parseMoonPhaseInput(text, submitted)
							state.pendingRequest.MoonPhase = phase
							state.moonPhaseSelected = selected
						},
						&state.phaseInputDialog,
					)).If(
						func() bool { return state.moonPhaseSelected },
						func(destinationQ *event_queue.SequentialEventBuilder) {
							var destinationErr error
							destinationQ.Then(func() {
								state.moongateDestination, destinationErr = g.gameState.GetMoongateDestinationForPhase(
									state.pendingRequest.MoonPhase,
								)
							}).If(
								func() bool { return destinationErr != nil },
								func(unavailableQ *event_queue.SequentialEventBuilder) {
									g.thenShowSpellNoEffect()(unavailableQ)
								},
							).If(
								func() bool { return destinationErr == nil },
								func(castQ *event_queue.SequentialEventBuilder) {
									castQ.Then(func() {
										state.validationResult = g.commitWorkflowSpell(state)
									}).If(
										func() bool { return !state.validationResult.Valid },
										func(errQ *event_queue.SequentialEventBuilder) {
											g.thenShowSpellValidationError(state.validationResult)(errQ)
										},
									).If(
										func() bool { return state.validationResult.Valid },
										func(effectQ *event_queue.SequentialEventBuilder) {
											g.dispatchSpellEffect(effectQ, state.pendingRequest, state)
										},
									)
								},
							)
						},
					)
				},
			)
		}).If(

		func() bool { return state.hasRequest && state.targetingProfile.Selection == references.TargetAutomatic },
		func(subQ *event_queue.SequentialEventBuilder) {
			subQ.Then(func() {
				state.pendingRequest.TargetIdx = state.pendingRequest.CasterIdx
				state.validationResult = g.commitWorkflowSpell(state)
			}).If(

				func() bool { return !state.validationResult.Valid },
				func(errQ *event_queue.SequentialEventBuilder) {
					g.thenShowSpellValidationError(state.validationResult)(errQ)
				}).If(

				func() bool { return state.validationResult.Valid },
				func(castQ *event_queue.SequentialEventBuilder) {
					g.dispatchSpellEffect(castQ, state.pendingRequest, state)
				})
		}).If(

		func() bool {
			return state.hasRequest && state.targetingProfile.Selection == references.TargetPartyMember
		},
		func(subQ *event_queue.SequentialEventBuilder) {
			subQ.Then(func() {
				state.validationResult = g.gameState.ValidateSpellTargetingPrerequisites(state.pendingRequest)
			}).If(
				func() bool { return !state.validationResult.Valid },
				func(errQ *event_queue.SequentialEventBuilder) {
					g.thenShowSpellValidationError(state.validationResult)(errQ)
				},
			)
		}).If(

		func() bool {
			return state.hasRequest && state.targetingProfile.Selection == references.TargetPartyMember &&
				state.validationResult.Valid
		},
		func(subQ *event_queue.SequentialEventBuilder) {
			subQ.Then(func() {
				state.targetingDialog = widgets.NewCharacterSelectionDialog(widgets.CharacterSelectionConfig{
					Title:        references.GetSpellName(state.pendingRequest.Spell) + ": who is it for?",
					ConfirmLabel: castActionLabel,
					CancelLabel:  castCancelLabel,
					Options: g.characterSelectionOptions(characterSelectionContext{
						Profile: characterSelectionSpellTarget,
						Spell:   state.pendingRequest.Spell,
					}),
					OnSelect: func(charIndex int) {
						g.dialogStack.RemoveWidgetInstance(state.targetingDialog)
						state.targetCharacterIdx = charIndex
						state.targetSelected = true
					},
					OnCancel: func() {
						g.dialogStack.RemoveWidgetInstance(state.targetingDialog)
						state.targetCharacterIdx = -1
						state.targetSelected = true
					},
					Keyboard:    g.keyboard,
					SpriteSheet: g.spriteSheet,
					Placement:   gameScreenPercents,
				})
				g.dialogStack.PushModalDialog(state.targetingDialog)
			}).
				While(
					func() bool { return !state.targetSelected },
					func() {},
					DialogPollingRateMs,
				).If(

				func() bool { return state.targetCharacterIdx >= 0 },
				func(castQ *event_queue.SequentialEventBuilder) {
					castQ.Then(func() {
						state.pendingRequest.TargetIdx = state.targetCharacterIdx
						state.validationResult = g.commitWorkflowSpell(state)
					}).If(func() bool { return state.validationResult.Valid }, func(effectQ *event_queue.SequentialEventBuilder) {
						g.dispatchSpellEffect(effectQ, state.pendingRequest, state)
					}).If(func() bool { return !state.validationResult.Valid }, func(errQ *event_queue.SequentialEventBuilder) {
						g.thenShowSpellValidationError(state.validationResult)(errQ)
					})
				})
		}).If(

		func() bool {
			return state.hasRequest && (state.targetingProfile.Selection == references.TargetDirection ||
				state.targetingProfile.Selection == references.TargetAdjacentTile)
		},
		func(subQ *event_queue.SequentialEventBuilder) {
			if g.gameState.GetCurrentSpellContext() == references.ContextCombat {
				if state.targetingProfile.Selection == references.TargetAdjacentTile {
					subQ.Then(func() {
						state.validationResult = g.gameState.BeginCombatAdjacentSpellTargeting(state.pendingRequest)
						if !state.validationResult.Valid {
							g.gameState.SystemCallbacks.UI.ShowActionFeedback(state.validationResult.ErrorMsg, true)

							return
						}
						state.combatActionTransferred = true
						g.showCombatAimControls(castActionLabel, castCancelLabel)
						g.secondaryKeyState = CombatAdjacentSpellTargetInput
						g.keyboard.SetAllowKeyPressImmediately()
					})

					return
				}
				if state.targetingProfile.Scope == references.ScopeCone {
					subQ.Then(func() {
						state.validationResult = g.gameState.BeginCombatConeTargeting(state.pendingRequest)
						if !state.validationResult.Valid {
							g.gameState.SystemCallbacks.UI.ShowActionFeedback(state.validationResult.ErrorMsg, true)

							return
						}
						state.combatActionTransferred = true
						g.showCombatAimControls(castActionLabel, castCancelLabel)
						g.secondaryKeyState = CombatConeTargetInput
						g.keyboard.SetAllowKeyPressImmediately()
					})

					return
				}
			}
			var directionSelected bool
			var selectedDirection references.Direction

			subQ.ThenSequence(g.thenShowCastDirectionPrompt(
				state.pendingRequest.Spell,
				func(dir references.Direction) {
					selectedDirection = dir
					directionSelected = true
				},
				func() {
					directionSelected = false
				},
			)).If(

				func() bool { return directionSelected },
				func(castQ *event_queue.SequentialEventBuilder) {
					castQ.Then(func() {
						state.pendingRequest.Direction = selectedDirection
						state.validationResult = g.commitWorkflowSpell(state)
					}).If(

						func() bool { return !state.validationResult.Valid },
						func(errQ *event_queue.SequentialEventBuilder) {
							g.thenShowSpellValidationError(state.validationResult)(errQ)
						}).If(

						func() bool { return state.validationResult.Valid },
						func(effectQ *event_queue.SequentialEventBuilder) {
							g.dispatchSpellEffect(effectQ, state.pendingRequest, state)
						})
				})
		}).If(

		func() bool {
			return state.hasRequest && (state.targetingProfile.Selection == references.TargetCombatantCrosshair ||
				state.targetingProfile.Selection == references.TargetTileCrosshair || state.targetingProfile.Selection == references.TargetCombatPartyMember)
		},
		func(subQ *event_queue.SequentialEventBuilder) {
			subQ.Then(func() {
				if state.targetingProfile.Selection == references.TargetCombatPartyMember {
					state.validationResult = g.gameState.BeginCombatPartySpellTargeting(state.pendingRequest)
				} else {
					state.validationResult = g.gameState.BeginCombatSpellTargeting(state.pendingRequest)
				}
				if !state.validationResult.Valid {
					g.gameState.SystemCallbacks.UI.ShowActionFeedback(state.validationResult.ErrorMsg, true)

					return
				}
				state.combatActionTransferred = true
				g.showCombatAimControls(castActionLabel, castCancelLabel)
				g.secondaryKeyState = CombatSpellTargetInput
				g.keyboard.PrepareForTransition(0)
			})
		}).Finally(

		func() {
			g.finishSpellCastingWorkflow(state)
		}).
		Start()
}

func (g *GameScene) commitWorkflowSpell(state *spellCastingWorkflowState) game_state.SpellValidationResult {
	if g.gameState.MapState.PlayerLocation != state.castOrigin ||
		(state.ownsCombatAction && (g.gameState.CombatScene == nil ||
			g.gameState.CombatScene.GetCurrentTurnIdentity() != state.combatCaster)) {
		return game_state.SpellValidationResult{ErrorMsg: "Casting context changed!"}
	}

	state.combatTiming = g.capturePresentationTiming(state.ownsCombatAction)
	defer g.combatTimingScope(state.combatTiming)()

	return g.gameState.ValidateAndConsumeSpellResources(state.pendingRequest)
}

func (g *GameScene) finishSpellCastingWorkflow(state *spellCastingWorkflowState) {
	if state.ownsCombatAction {
		g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(0)
		g.gameState.SystemCallbacks.Visual.SetScreenInvert(0, 0)
		g.gameState.SystemCallbacks.Visual.SetScreenShakeOffset(0, 0)
	}
	if state.mixSession != nil {
		g.cleanupSpellMixSession(state.mixSession)
	}
	g.clearCastUseDirectionPrompt()
	committed := (state.ownsSurfaceCommand || state.ownsCombatAction) &&
		g.gameState.CastUseCommitSequence() != state.commitSequence
	if state.casterDialog != nil {
		g.dialogStack.RemoveWidgetInstance(state.casterDialog)
		state.casterDialog = nil
	}
	if state.spellDialog != nil {
		g.dialogStack.RemoveWidgetInstance(state.spellDialog)
		state.spellDialog = nil
	}
	if state.targetingDialog != nil {
		g.dialogStack.RemoveWidgetInstance(state.targetingDialog)
		state.targetingDialog = nil
	}
	if state.phaseInputDialog != nil {
		g.dialogStack.RemoveWidgetInstance(state.phaseInputDialog)
		state.phaseInputDialog = nil
	}
	if state.gemViewDialog != nil {
		g.dialogStack.RemoveWidgetInstance(state.gemViewDialog)
		state.gemViewDialog = nil
	}
	if state.dungeonViewDialog != nil {
		state.dungeonViewDialog.Dismiss()
		g.dialogStack.RemoveWidgetInstance(state.dungeonViewDialog)
		state.dungeonViewDialog = nil
	}
	g.restoreSpellXRay(state)
	g.clearCombatInvocationEffects()
	g.clearCombatTeleportEffect()
	if state.teleportEffectsOwned {
		g.gameState.SystemCallbacks.Visual.SetAvatarDissolveLevel(0)
		g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(0)
		g.gameState.SystemCallbacks.Visual.CleanupAvatarDissolveEffect()
		state.teleportEffectsOwned = false
	}
	if state.ownsDungeonCommand {
		state.ownsDungeonCommand = false
		g.gameState.CompleteDungeonCommandCycle(state.dungeonCommandToken, game_state.DungeonFullEndPlayer)
	} else {
		g.keyboard.ResetReleaseDebouncing()
		if state.ownsSurfaceCommand {
			state.ownsSurfaceCommand = false
			g.surfaceSpellWorkflowOwnsCycle = false
			completionState := surfaceCommandCancelled
			if committed {
				completionState = surfaceCommandCompleted
			} else if state.hasRequest {
				completionState = surfaceCommandRejected
			}
			result := newSurfaceCommandResult(completionState, state.surfaceOriginMapType)
			result.ConsumesNormalCycle = committed
			result.MapTransitioned = g.gameState.MapState.PlayerLocation.Location != state.surfaceOriginLocation
			if committed && result.MapTransitioned {
				minutes := game_state.DefaultSmallMapMinutesPerTurn
				if state.surfaceOriginMapType == references.LargeMapType {
					minutes = game_state.DefaultLargeMapMinutesPerTurn
				}
				g.gameState.AdvanceGameplayMinutesAndProcessProvisions(minutes)
			}
			g.completeSurfaceCommand(result)
		}
	}
	if state.ownsCombatAction && !state.combatActionTransferred {
		state.ownsCombatAction = false
		if committed {
			g.gameState.FinishCombatAction()
		}
	}
	if state.processProvisionHoursOnFinish {
		g.gameState.ProcessPendingProvisionHours()
	}
}

func (g *GameScene) restoreSpellXRay(state *spellCastingWorkflowState) {
	if !state.restoreXRay {
		return
	}
	g.xRayVisionActive = state.previousXRay
	state.restoreXRay = false
}

func (g *GameScene) showTemporaryCastFailure(message string) {
	g.ShowTimedOverlayMessage(message, castFailureDialogDurationMs, true, true)
}

// thenShowSpellValidationError shows a validation error message with appropriate sound.
func (g *GameScene) thenShowSpellValidationError(result game_state.SpellValidationResult) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		sound := game_state.SoundSpellFailure
		if result.IsAbsorb {
			sound = game_state.SoundAbsorb
		}

		return g.thenShowSpellFeedback(result.ErrorMsg, sound)(q)
	}
}

// dispatchSpellEffect routes the spell to its event queue builder based on spell type.
// Chains the appropriate thenCast*() function into the provided queue.
// For unimplemented spells, shows a generic feedback message.
func (g *GameScene) dispatchSpellEffect(
	q *event_queue.SequentialEventBuilder,
	request game_state.CastRequest,
	state *spellCastingWorkflowState,
) {
	defer g.combatTimingScope(state.combatTiming)()
	//nolint:exhaustive // Context validation routes combat-only spells elsewhere; unsupported consumed spells panic.
	switch request.Spell {
	// Self-target spells
	case references.InLor:
		q.ThenSequence(g.thenCastInLor())
	case references.VasLor:
		q.ThenSequence(g.thenCastVasLor())
	case references.InXenMani:
		q.ThenSequence(g.thenCastInXenMani())
	case references.InFlamGrav, references.InNoxGrav, references.InZuGrav, references.InSanctGrav:
		q.ThenSequence(g.thenCastDungeonPackedSpell(request.Spell))
	case references.InWis:
		q.ThenSequence(g.thenCastInWis())
	case references.WisAnYlem:
		q.ThenSequence(g.thenCastWisAnYlem(state))
	case references.InQuasWis:
		q.ThenSequence(g.thenCastInQuasWis(state))
	case references.VasRelPor:
		q.ThenSequence(g.thenCastVasRelPor(state.moongateDestination, state))
	case references.UusPor:
		q.ThenSequence(g.thenCastDungeonFloorSpell(request.Spell, state))
	case references.DesPor:
		q.ThenSequence(g.thenCastDungeonFloorSpell(request.Spell, state))
	case references.InZu:
		q.Then(func() { g.gameState.CastCombatInZu(request.CasterIdx, request.Direction) })
	case references.WisQuas:
		q.ThenSequence(g.thenCastCombatWisQuas())
	case references.InVasPorYlem:
		q.ThenSequence(g.thenCastCombatInVasPorYlem(request.CasterIdx))
	case references.RelTym:
		q.ThenSequence(g.thenCastCombatRelTym())
	case references.SanctLor:
		q.ThenSequence(g.thenCastCombatSanctLor(request.CasterIdx))
	case references.KalXen, references.InBetXen, references.KalXenCorp:
		q.ThenSequence(g.thenCastCombatSummon(request.Spell, request.CasterIdx))
	case references.AnXenCorp:
		q.ThenSequence(g.thenCastCombatRepel(request.CasterIdx, g.gameState.CastCombatAnXenCorp, combatRepelFeedbackText))
	case references.InSanct:
		q.ThenSequence(g.thenCastInSanct())
	case references.QuasAnWis:
		q.ThenSequence(g.thenCastCombatQuasAnWis())
	case references.InAn:
		if g.gameState.GetCurrentSpellContext() == references.ContextCombat {
			q.ThenSequence(g.thenCastCombatInAn())
		} else {
			q.Then(func() {
				g.gameState.CastCombatInAn()
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
			})
		}
	case references.AnTym:
		q.ThenSequence(g.thenCastAnTym())
	case references.InQuasCorp:
		q.ThenSequence(g.thenCastCombatRepel(request.CasterIdx, g.gameState.CastCombatInQuasCorp, combatFearFeedbackText))

	// Character-target spells
	case references.Mani:
		q.ThenSequence(g.thenCastMani(request.TargetIdx))
	case references.VasMani:
		q.ThenSequence(g.thenCastVasMani(request.TargetIdx))
	case references.AnNox:
		q.ThenSequence(g.thenCastAnNox(request.TargetIdx))
	case references.AnZu:
		q.ThenSequence(g.thenCastAnZu(request.TargetIdx))
	case references.InManiCorp:
		q.ThenSequence(g.thenCastInManiCorp(request.TargetIdx))

	// Direction-target spells
	case references.AnYlem:
		q.ThenSequence(g.thenCastAnYlem(request.Direction))
	case references.AnSanct:
		if g.gameState.GetCurrentSpellContext() == references.ContextDungeon {
			q.ThenSequence(g.thenCastDungeonPackedSpell(request.Spell))
		} else {
			q.ThenSequence(g.thenCastAnSanct(request.Direction))
		}
	case references.AnExPor:
		q.ThenSequence(g.thenCastAnExPor(request.Direction))
	case references.InExPor:
		q.ThenSequence(g.thenCastInExPor(request.Direction))
	case references.AnGrav:
		//nolint:exhaustive // Non-dungeon/non-combat contexts share the surface route.
		switch g.gameState.GetCurrentSpellContext() {
		case references.ContextDungeon:
			q.ThenSequence(g.thenCastDungeonPackedSpell(request.Spell))
		case references.ContextCombat:
			q.Then(func() {
				if !g.gameState.CastCombatAnGrav(request.CasterIdx, request.Direction) {
					g.gameState.SystemCallbacks.Message.AddRowStr("No effect!")
				}
			})
		default:
			q.ThenSequence(g.thenCastAnGrav(request.Direction))
		}
	case references.InPor:
		if g.gameState.GetCurrentSpellContext() == references.ContextCombat {
			q.ThenSequence(g.thenCastCombatInPor(request.CasterIdx))
		} else {
			q.ThenSequence(g.thenCastInPor(request.Direction))
		}
	case references.RelHur:
		q.ThenSequence(g.thenCastRelHur(request.Direction))

	default:
		// Reaching this default after resource validation is a programming error.
		panic("dispatchSpellEffect: consumed spell has no effect dispatch")
	}
}

func parseMoonPhaseInput(text string, submitted bool) (datetime.MoonPhase, bool) {
	if !submitted {
		return datetime.MoonPhaseNew, false
	}
	phase, err := strconv.Atoi(text)
	if err != nil || phase < 1 || phase > 8 {
		return datetime.MoonPhaseNew, false
	}

	return datetime.MoonPhase(phase - 1), true
}

// buildReagentInventory creates the reagent inventory list from party state.
func (g *GameScene) buildReagentInventory() []widgets.ReagentInfo {
	// Define reagent display info
	type reagentDef struct {
		reagent references.Reagent
		name    string
	}

	reagentDefs := []reagentDef{
		{references.SulfurAsh, "Sulfurous Ash"},
		{references.Ginseng, "Ginseng"},
		{references.Garlic, "Garlic"},
		{references.SpiderSilk, "Spider Silk"},
		{references.BloodMoss, "Blood Moss"},
		{references.BlackPearl, "Black Pearl"},
		{references.NightShade, "Nightshade"},
		{references.MandrakeRoot, "Mandrake Root"},
	}

	inventory := make([]widgets.ReagentInfo, len(reagentDefs))
	for i, def := range reagentDefs {
		qty := g.gameState.PartyState.Inventory.Reagent.Get(def.reagent)
		inventory[i] = widgets.ReagentInfo{
			Reagent:     def.reagent,
			Name:        def.name,
			Quantity:    int(qty),
			SpriteIndex: i, // Sprite index matches reagent order
		}
	}

	return inventory
}

// buildMixConfirmationMessage creates the confirmation message for mix outcome.
func (g *GameScene) buildMixConfirmationMessage(outcome game_state.MixOutcome) string {
	if !outcome.Success {
		return outcome.Message
	}

	// REDUX DEVIATION: MIX-0004: original success feedback is brief; Redux adds a receipt to make capped stock gains explicit.
	message := fmt.Sprintf("%s\n\nAdded %d %s. Stock: %d.", outcome.Message, outcome.Added, outcome.SpellName, outcome.Stock)
	if excess := outcome.Quantity - outcome.Added; excess > 0 {
		message += fmt.Sprintf("\n%d excess mixtures discarded.", excess)
	}
	if outcome.IsNewRecipe {
		message += "\n\nNew recipe memorized!"
	}

	return message
}

// applyMixTrapEffects applies trap effects after a failed mix attempt.
// Called from startReagentMixingWorkflow after the dialog is dismissed.
// Uses PartyStateBuilder to apply damage/poison with synchronized VFX.
func (g *GameScene) applyMixTrapEffects(q *event_queue.SequentialEventBuilder, trap *game_state.TrapResult) *event_queue.SequentialEventBuilder {
	// Preserve the trap's own failure message.
	var effectDesc string
	switch trap.TrapType {
	case game_state.TrapNone:
		// No additional injury description.
	case game_state.TrapAcid:
		effectDesc = g.gameState.PartyState.GetCharacter(trap.TargetIndex).GetNameAsString() + " burned!"
	case game_state.TrapPoison:
		effectDesc = g.gameState.PartyState.GetCharacter(trap.TargetIndex).GetNameAsString() + " poisoned!"
	case game_state.TrapBomb:
		effectDesc = "Party damaged!"
	case game_state.TrapGas:
		effectDesc = "Party poisoned!"
	}
	feedbackMsg := fmt.Sprintf("%s\n%s", trap.Message, effectDesc)

	// Show non-modal action feedback
	q = q.Then(func() {
		g.gameState.SystemCallbacks.UI.ShowActionFeedback(feedbackMsg, true)
	})

	// Apply trap effects via PartyStateBuilder
	switch trap.TrapType {
	case game_state.TrapNone:
		return q
	case game_state.TrapAcid:
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				WithOriginalD30Damage().
				ToCharacters(trap.TargetIndices).
				FlashOrange().ShakeModerate().
				WithSound(game_state.SoundSpellFailure).
				Build())

	case game_state.TrapPoison:
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				WithPoison().
				ToCharacters(trap.TargetIndices).
				FlashGreen().
				WithSound(game_state.SoundPoison).
				Build())

	case game_state.TrapBomb:
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				WithRandomDamage(trap.DamageMin, trap.DamageMax).
				ToCharacters(trap.TargetIndices).
				FlashOrange().ShakeStrong().
				WithSound(game_state.SoundSpellFailure).
				Build())

	case game_state.TrapGas:
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				WithPoison().
				ToCharacters(trap.TargetIndices).
				FlashGreen().
				WithSound(game_state.SoundPoison).
				Build())
	}

	return q
}

// toggleMusic toggles background music on/off
func (g *GameScene) toggleMusic() {
	if g.audioManager == nil {
		g.addRowStr("No music available")

		return
	}

	// Get current music manager
	musicManager := g.audioManager.GetMusicManager()
	if musicManager == nil {
		g.addRowStr("No music available")

		return
	}

	// A deliberate quiet interval is still enabled music.
	if musicManager.IsEnabled() {
		musicManager.SetEnabled(false)
		g.addRowStr("Music Off")
	} else {
		musicManager.SetEnabled(true)
		// Start music for current location
		currentLocation := g.gameState.MapState.PlayerLocation.Location
		err := musicManager.PlayMusicForLocation(currentLocation)
		if err != nil {
			g.addRowStr("The music would not start")
		} else {
			g.addRowStr("Music On")
		}
	}
}

// TalkCallbacks interface implementation
//
//nolint:ireturn // The core callback contract must not depend on concrete presentation widgets.
func (g *GameScene) CreateTalkDialog(npc *map_units.NPCFriendly) game_state.TalkDialog {
	// Create the dialog
	dialog := NewLinearTalkDialog(g, npc)

	// Handle the nil pointer to interface conversion properly
	if dialog == nil {
		return nil // This returns a proper nil interface
	}

	return dialog
}

func (g *GameScene) PushDialog(dialog game_state.TalkDialog) {
	// Handle nil dialog gracefully to prevent crashes
	if dialog == nil {
		// Nil dialog is handled by caller (ActionTalkSmallMap) with appropriate user feedback
		return
	}

	if linearDialog, ok := dialog.(*LinearTalkDialog); ok {
		g.dialogStack.PushModalDialog(linearDialog)
		linearDialog.mount()
	} else {
		// The current presentation adapter supports only semantic linear Talk.
		g.output.AddText("Dialog system error.")
	}
}

// ShowYellDialogLargeMap displays the yell dialog for entering Words of Power on large map.
// Handles two cases:
//  1. Shrine restoration - Word of Power yelled adjacent to ruined shrine
//  2. Dungeon entrance - Word of Power yelled near dungeon entrance
//
// Uses event queue workflow with conditional earthquake animation based on validation.
// Outdoor Yell checks for ruined shrines first (ref:SHRINE-0005).
func (g *GameScene) ShowYellDialogLargeMap() {
	var wordProcessed bool
	var shrineData *game_state.ShrineData

	// Variables for shrine restoration mantra validation
	var virtueValid bool
	var restorationCancelled bool
	mantraAllValid := true
	var transcript *widgets.InteractionTranscript
	removeTranscript := func() {
		if transcript != nil {
			g.dialogStack.RemoveWidgetInstance(transcript)
			transcript = nil
		}
	}

	g.gameState.EventQueueManager.Workflow("word_of_power_entry").
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		ThenSequence(

			g.thenShowInputBox(
				"What dost thou yell?",
				"^[[:print:]]{0,30}$",
				func(text string, submitted bool) {
					// REDUX DEVIATION: SHRINE-0005: original Yell has no immediate Escape cancellation.
					// Redux cancels safely, while every submitted response gets explicit resolution.
					if !submitted {
						return
					}

					shrineData = g.gameState.GetAdjacentRuinedShrine(text)
					if shrineData != nil {
						return
					}

					wordProcessed = g.gameState.ProcessWordOfPower(text)
				},
			)).If(

		func() bool { return shrineData != nil },
		func(shrineQ *event_queue.SequentialEventBuilder) {
			transcript = widgets.NewInteractionTranscript(g.keyboard, widgets.InteractionTranscriptConfig{
				MaxLines:       7,
				MaxInputLength: 40,
				WidthSamples: []string{
					"Upon what virtue dost thou meditate?",
					"Virtue?  > " + strings.Repeat("M", 20),
					"Mantra:  > " + strings.Repeat("M", 20),
				},
			})
			shrineQ.ThenSequence(g.thenShowShrineRestorationEarthquake()).Then(func() {
				transcript.AppendLine("Upon what virtue dost thou meditate?")
				g.dialogStack.PushModalDialog(transcript)
			}).ThenSequence(

				g.thenPromptInteractionText(
					transcript,
					"Virtue?",
					func(text string, submitted bool) {
						restorationCancelled = !submitted
						if !submitted || text == "" {
							virtueValid = false

							return
						}
						virtueValid = game_state.ValidateRestorationVirtue(text, shrineData.VirtueName)
					},
				)).If(

				func() bool { return !restorationCancelled && !virtueValid },
				func(failQ *event_queue.SequentialEventBuilder) {
					// REDUX DEVIATION: SHRINE-0005: original restoration asks all three mantras and
					// prints only a newline after failure. Redux stops early and gives feedback.
					failQ.Then(func() { removeTranscript() }).
						ThenSequence(g.thenShowConfirmationDialog("Thy thoughts are unfocused."))
				}).If(

				func() bool { return virtueValid },
				func(virtueQ *event_queue.SequentialEventBuilder) {
					// REDUX DEVIATION: SHRINE-0005: original restoration always asks three
					// mantras. Redux's advertised Cancel stops the remaining prompts.
					for range game_state.MantraRepetitions {
						virtueQ.If(func() bool { return !restorationCancelled }, func(q *event_queue.SequentialEventBuilder) {
							q.ThenSequence(g.thenPromptInteractionText(transcript, "Mantra:", func(text string, submitted bool) {
								restorationCancelled = !submitted
								if !submitted || !game_state.ValidateMantra(text, shrineData.Mantra) {
									mantraAllValid = false
								}
							}))
						})
					}
					virtueQ.If(

						func() bool { return !restorationCancelled && !mantraAllValid },
						func(mantraFailQ *event_queue.SequentialEventBuilder) {
							mantraFailQ.Then(func() { removeTranscript() }).
								ThenSequence(g.thenShowConfirmationDialog("Thy thoughts are unfocused."))
						}).If(

						func() bool { return !restorationCancelled && mantraAllValid },
						func(successQ *event_queue.SequentialEventBuilder) {
							successQ.ThenSequence(func(restoreQ *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
								removeTranscript()
								g.gameState.SetShrineRuined(shrineData.Index, false)

								g.thenShowShrineRestoredEarthquake()(restoreQ)

								return restoreQ
							})
						})
				})
		}).If(

		func() bool { return wordProcessed },
		func(dungeonQ *event_queue.SequentialEventBuilder) {
			dungeonQ.ThenSequence(g.thenShowDungeonEarthquake())
		}).Finally(func() { removeTranscript() }).
		Start()
}

// ShowYellDialogSmallMap owns the submitted word and its ordinary-turn outcome.
func (g *GameScene) ShowYellDialogSmallMap() {
	originMapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()
	completion := newSurfaceCommandResult(surfaceCommandCancelled, originMapType)

	g.gameState.EventQueueManager.Workflow("yell_small_map").
		Priority(event_queue.PriorityLow).
		Sequential().ThenSequence(

		g.thenShowInputBox(
			"What dost thou yell?",
			"^[a-zA-Z]{0,10}$",
			func(text string, submitted bool) {
				if submitted {
					completion.State = surfaceCommandCompleted
					if g.gameState.ActionYellSmallMap(text) {
						// Successful summoning deliberately returns without an ordinary
						// town cycle. Appearance feedback owns no actor step. ref:SHADOW-0004.
						completion.ConsumesNormalCycle = false
					}
				}
			},
		)).Finally(func() {
		g.completeSurfaceCommand(completion)
	}).
		Start()
}

// ShowTimedOverlayMessage displays a non-interactive overlay message that auto-dismisses.
// The message appears immediately and runs concurrently with other effects.
// It ignores all keyboard input - purely time-based dismissal.
//
// This is used for brief status messages during effects (e.g., "Destroyed!" during cannon hit)
// where the message should be visible while shake/kapow/sounds play underneath.
//
// Parameters:
//   - message: the text to display
//   - durationMs: how long to show the message before auto-hiding (milliseconds)
//   - centerHorizontally: if true, centers on whole screen (50% X); if false, centers on map area only (37% X)
//   - alsoAddToOutput: if true, also adds message to main output pane
//
// NOTE: This method does NOT use event queues because it's often called from inside
// event queue callbacks. Instead, it uses a timer-based approach with dismissal checked
// in updateTimedOverlay() during the main update loop.
func (g *GameScene) ShowTimedOverlayMessage(message string, durationMs int, centerHorizontally bool, alsoAddToOutput bool) {
	// Optionally add to main output pane for authentic Ultima V record
	if alsoAddToOutput {
		g.output.AddText(message)
	}

	builder := widgets.NewOverlayDialog(message).
		AsOverlayFeedback().
		WithExternallyManagedDuration(durationMs)
	if !centerHorizontally {
		builder = builder.WithTextAlignment(widgets.TextAlignLeft)
	}
	g.installTimedOverlay(builder.Build())
}

// installTimedOverlay replaces the scene-owned overlay and starts its sole deadline.
func (g *GameScene) installTimedOverlay(overlay *widgets.OverlayDialog) {
	if overlay == nil {
		panic("GameScene.installTimedOverlay: overlay is required")
	}
	if !overlay.IsDurationExternallyManaged() {
		panic("GameScene.installTimedOverlay: overlay duration must be externally managed")
	}
	if g.gameState == nil {
		panic("GameScene.installTimedOverlay: game state is required")
	}

	g.clearTimedOverlay()
	overlay.ShowWithoutAnimation()
	g.dialogStack.PushModalDialog(overlay)
	g.timedOverlayDialog = overlay
	g.timedOverlayDismissMs = g.gameState.ElapsedMs + int64(overlay.GetDurationMs())
}

// clearTimedOverlay removes only the exact scene-owned overlay. It is idempotent
// so expiry and scene teardown can safely converge on the same cleanup path.
func (g *GameScene) clearTimedOverlay() {
	if g.timedOverlayDialog == nil {
		g.timedOverlayDismissMs = 0

		return
	}

	overlay := g.timedOverlayDialog
	g.dialogStack.RemoveWidgetInstance(overlay)
	overlay.HideWithoutAnimation()
	g.timedOverlayDialog = nil
	g.timedOverlayDismissMs = 0
}

// updateTimedOverlay checks if a timed overlay should be dismissed.
// Called from the main update loop to handle auto-dismissal without event queues.
func (g *GameScene) updateTimedOverlay() {
	if g.timedOverlayDialog == nil {
		return
	}

	if g.gameState.ElapsedMs >= g.timedOverlayDismissMs {
		g.clearTimedOverlay()
	}
}

// ShowActionFeedback displays a popup for action command responses (e.g., "Unlocked!", "Key broke!").
// Its lifecycle is owned by GameScene and advances from GameState.ElapsedMs.
//
// Features:
//   - 400ms fade-in, 1600ms hold, and 300ms fade-out
//   - The applied 0–3 popup limit bounds a shared newest-first action/Search stack
//   - Never blocks input: game continues normally while popup displays
//
// Parameters:
//   - message: short feedback text to display
//   - alsoAddToOutput: if true, also adds message to main output pane (consolidates dual call pattern)
//
// This is the primary visual feedback mechanism for player commands, providing
// prominent feedback without requiring the player to watch the text scroll.
func (g *GameScene) ShowActionFeedback(message string, alsoAddToOutput bool) {
	logging.Infof(logging.CategoryUI,
		"ShowActionFeedback called with message: %q, alsoAddToOutput: %v", message, alsoAddToOutput)

	// Optionally add to main output pane for authentic Ultima V record
	if alsoAddToOutput {
		g.output.AddText(message)
	}

	if g.actionFeedbackLimit() == 0 {
		g.clearTransientActionFeedback()

		return
	}
	g.showTransientActionFeedback(widgets.NewActionFeedbackDialog(message, g.actionFeedbackPosition()))
}

// ShowSearchFeedback displays the complete Search result without duplicating output-pane text.
func (g *GameScene) ShowSearchFeedback(message string) {
	if g.actionFeedbackLimit() == 0 {
		g.clearTransientActionFeedback()

		return
	}
	g.showTransientActionFeedback(widgets.NewSearchFeedbackDialog(message, g.actionFeedbackPosition()))
}

func (g *GameScene) showTransientActionFeedback(dialog *widgets.MessageDialog) {
	g.admitTransientActionFeedback(dialog, g.gameState.ElapsedMs)
}

func (g *GameScene) actionFeedbackPosition() widgets.Position {
	if g.gameState != nil &&
		g.gameState.MapState.PlayerLocation.Location.GetMapType() == references.DungeonMapType {
		return widgets.PositionMapCenterBottom
	}

	return widgets.PositionMapCenterLower
}

// countSpellCapableCharacters returns count of alive party members with MP capacity.
// Used to determine if "Who will cast?" dialog should be shown.
func (g *GameScene) countSpellCapableCharacters() int {
	count := 0
	g.gameState.PartyState.ForEachCharacter(func(_ int, c *party_state.PlayerCharacter) {
		if c.PartyMap == party_state.PartyMapActive && isSpellCapable(c) {
			count++
		}
	})

	return count
}

// isSpellCapable returns true if the living character's class can cast spells.
// Used as CustomFilter for CharacterSelectionDialog.
func isSpellCapable(c *party_state.PlayerCharacter) bool {
	return c.CanCastSpells() && isCharacterActorEligible(c)
}

// startReadyEquipmentWorkflow initiates the Ready Equipment dialog workflow.
// Called when player presses 'R' key to equip/unequip weapons and armor.
//
// The workflow creates a ReadyEquipmentDialog, pushes it to the dialog stack,
// and polls until the dialog is dismissed via ESC key.
func (g *GameScene) startReadyEquipmentWorkflow(request game_state.ReadyEquipmentRequest) {
	g.startReadyEquipmentWorkflowWithCompletion(request, nil)
}

func (g *GameScene) startReadyEquipmentWorkflowWithCompletion(
	request game_state.ReadyEquipmentRequest,
	complete func(),
) {
	var dialog *widgets.ReadyEquipmentDialog

	g.gameState.EventQueueManager.Workflow("ready_equipment_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			dialog = widgets.NewReadyEquipmentDialog(
				g.keyboard,
				g.spriteSheet,
				g.gameState,
				request,
			)
			g.dialogStack.PushModalDialog(dialog)
		}).
		While(func() bool {
			if transaction, submitted := dialog.TakeTransaction(); submitted {
				result, err := g.gameState.ApplyReadyEquipment(transaction)
				dialog.ApplyTransactionResult(result, err)
			}

			return !dialog.IsDismissed()
		}, func() {

		}, DialogPollingRateMs).
		Finally(func() {
			if dialog != nil {
				dialog.Dismiss()
				g.dialogStack.RemoveWidgetInstance(dialog)
			}
			g.keyboard.ResetReleaseDebouncing()
			if complete != nil {
				complete()
			} else if request.CombatMode {
				token := g.gameState.FinishCombatActionDeferredFor(request.CombatIdentity)
				g.completeScheduledCombatStep("ready equipment", token)
			}
		}).
		Start()
}
