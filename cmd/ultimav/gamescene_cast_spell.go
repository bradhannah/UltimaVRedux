package main

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// Spell visual effect constants
const (
	authenticTickDurationMs = 55
	wisAnYlemTicks          = 20
	wisAnYlemDurationMs     = authenticTickDurationMs * wisAnYlemTicks
)

// =============================================================================
// Spell Helper Functions
// =============================================================================

// thenShowSpellNoEffect shows "No effect!" with the spell failure sound.
// Used when a spell is cast but has no effect on the target (dead, already healthy, etc.).
//
// Returns a modifier function for use with ThenSequence in event queues.
func (g *GameScene) thenShowSpellNoEffect() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenShowSpellFeedback("No effect!", game_state.SoundSpellFailure)
}

// thenShowSpellFeedback shows a feedback message with an optional sound effect.
// Used for spell success/failure feedback within event queue sequences.
//
// Returns a modifier function for use with ThenSequence in event queues.
func (g *GameScene) thenShowSpellFeedback(message string, sound game_state.SoundEffect) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		overlay := widgets.NewOverlayDialog(message).
			AsConfirmation().
			WithUserDismissed(g.keyboard).
			AutoPopFromStack(&g.dialogStack).
			Build()

		return q.Then(func() {
			if sound != game_state.SoundNoEffect {
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(sound)
			}
		}).ThenSequence(

			g.showOverlayAndWait(overlay))
	}
}

// thenShowSpellSuccess reports a spell that simply worked; its sound has already
// played. Outside battle the report is a row in the message window, where every
// other command reports, so there is no box to close. Battle keeps the box, as
// does anything to read and any cast that failed or did nothing.
func (g *GameScene) thenShowSpellSuccess(message string) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if g.gameState.IsInCombat() {
			return g.thenShowSpellFeedback(message, game_state.SoundNoEffect)(q)
		}

		return q.Then(func() { g.gameState.SystemCallbacks.Message.AddRowStr(message) })
	}
}

func (g *GameScene) thenShowRunicSpellSuccess(message string) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		overlay := widgets.NewOverlayDialog("").
			AsConfirmation().
			WithUserDismissed(g.keyboard).
			AutoPopFromStack(&g.dialogStack).
			Build()
		overlay.AppendLineBuilder().White("").Runic(message).Done()

		return q.ThenSequence(g.showOverlayAndWait(overlay))
	}
}

// =============================================================================
// Self-Target Spells
// =============================================================================

// thenCastInLor applies the In Lor (Light) spell effect.
// Creates magical light for InLorLightDuration gameplay minutes.
// Circle 1 spell, TargetAutomatic, ContextNonCombat.
func (g *GameScene) thenCastInLor() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		// Apply game effect
		g.gameState.ActivateLightEffect(game_state.InLorLightDuration)

		// Visual effects via PartyStateBuilder
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				FlashInverted().
				WithSound(game_state.SoundMagic).
				Build()).ThenSequence(

			g.thenShowSpellSuccess("Light!"))
	}
}

// thenCastVasLor applies the Vas Lor (Great Light) spell effect.
// Creates magical light for VasLorLightDuration gameplay minutes.
// Circle 3 spell, TargetAutomatic, ContextNonCombat.
func (g *GameScene) thenCastVasLor() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		// Apply game effect
		g.gameState.ActivateLightEffect(game_state.VasLorLightDuration)

		// Visual effects via PartyStateBuilder
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				FlashInverted().
				WithSound(game_state.SoundMagic).
				Build()).ThenSequence(

			g.thenShowSpellSuccess("Great light!"))
	}
}

// thenCastInXenMani applies the In Xen Mani (Create Food) spell effect.
// Creates 1-CreateFoodDiceSize units of food for the party.
// Circle 2 spell, TargetAutomatic (affects party), ContextAll.
func (g *GameScene) thenCastInXenMani() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	timing := g.capturePresentationTiming(g.gameState.IsInCombat())

	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		foodAmount := g.gameState.CreateFood()

		// Visual effects via PartyStateBuilder
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				FlashInverted().
				WithSound(game_state.SoundCreateFood).
				BuildWithPresentationTiming(timing.duration)).ThenSequence(

			g.thenShowSpellSuccess(fmt.Sprintf("Created %d food!", foodAmount)))
	}
}

// thenCastInWis applies the In Wis (Locate) spell effect.
// Displays the party's current coordinates on the overworld.
// Circle 2 spell, TargetAutomatic, ContextOverworld.
func (g *GameScene) thenCastInWis() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		position := g.gameState.MapState.PlayerLocation.Position

		// Visual effects (informational - no flash, just sound)
		return q.Then(func() {
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundLocate)
		}).ThenSequence(

			g.thenShowRunicSpellSuccess(references.FormatBritannianCoordinates(
				position.X, position.Y)))
	}
}

// thenCastWisAnYlem applies the Wis An Ylem (View Gem) spell effect.
// The original restores normal rendering after 20 fixed 18.2 Hz ticks.
// Circle 6 spell, TargetAutomatic, ContextOverworld | ContextTown.
func (g *GameScene) thenCastWisAnYlem(state *spellCastingWorkflowState) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.Then(func() {
			state.previousXRay = g.xRayVisionActive
			state.restoreXRay = true
			g.xRayVisionActive = true
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundXRay)
		}).Wait(wisAnYlemDurationMs).
			Then(func() {
				g.restoreSpellXRay(state)
			})
	}
}

// thenCastInQuasWis applies the In Quas Wis (View) spell effect.
// Surface maps reuse GemViewDialog; dungeons use the source-backed symbolic view.
// Circle 7 spell, TargetAutomatic, ContextNonCombat.
func (g *GameScene) thenCastInQuasWis(state *spellCastingWorkflowState) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if g.gameState.GetCurrentSpellContext() == references.ContextDungeon {
			return q.Then(func() {
				model, err := g.gameState.CurrentDungeonViewModel()
				if err != nil {
					panic(err)
				}
				state.dungeonViewModel = model
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundXRay)
			}).ThenSequence(g.thenShowDungeonViewDialog(&state.dungeonViewModel, &state.dungeonViewDialog))
		}

		return q.Then(func() {
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundXRay)
		}).ThenSequence(g.thenShowGemViewDialog(&state.gemViewDialog))
	}
}

// thenCastVasRelPor travels to the captured moonstone destination. The owning
// spell workflow commits the one-minute Redux casting cost after transition.
func (g *GameScene) thenCastVasRelPor(
	destination game_state.MoongateDestination,
	state *spellCastingWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		dungeonOrigin := state.ownsDungeonCommand

		return q.Then(func() {
			state.teleportEffectsOwned = true
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
			if dungeonOrigin {
				g.gameState.CompleteDungeonCommandCycle(
					state.dungeonCommandToken,
					game_state.DungeonCommandOnly,
				)
				state.ownsDungeonCommand = false
			}
		}).ThenSequence(
			g.gameState.BuildMoonstoneSpellTeleportationSequence(destination),
		).Then(func() {
			if !dungeonOrigin && !g.surfaceSpellWorkflowOwnsCycle {
				g.gameState.AdvanceGameplayMinutes(1)
				state.processProvisionHoursOnFinish = true
			}
			state.teleportEffectsOwned = false
		})
	}
}

// =============================================================================
// Character-Target Spells
// =============================================================================

// thenCastMani applies the Mani (Heal) spell effect.
// Heals the target with the original transformed d30 (ref:RNG-0001).
// Circle 1 spell, TargetPartyMember, ContextAll.
func (g *GameScene) thenCastMani(targetIdx int) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		result := g.gameState.CastMani(targetIdx)
		if !result.Applied {
			return g.thenShowSpellNoEffect()(q)
		}

		// Visual effects via PartyStateBuilder
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				ToCharacter(targetIdx).
				FlashInverted().
				WithSound(game_state.SoundHeal).
				Build()).ThenSequence(

			g.thenShowSpellSuccess(fmt.Sprintf("Healed %d!", result.Amount)))
	}
}

// thenCastVasMani applies the Vas Mani (Great Heal) spell effect.
// Heals target to maximum HP (full restore).
// ref:SPELL-0002: restore the selected living character's HP to its maximum.
// Circle 5 spell, TargetPartyMember, ContextAll.
func (g *GameScene) thenCastVasMani(targetIdx int) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		result := g.gameState.CastVasMani(targetIdx)
		if !result.Applied {
			return g.thenShowSpellNoEffect()(q)
		}

		// Visual effects via PartyStateBuilder
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				ToCharacter(targetIdx).
				FlashInverted().
				WithSound(game_state.SoundGreatHeal).
				Build()).ThenSequence(

			g.thenShowSpellSuccess(fmt.Sprintf("Healed %d!", result.Amount)))
	}
}

// thenCastAnNox applies the An Nox (Cure Poison) spell effect.
// Removes poison status from target character.
// Circle 1 spell, TargetPartyMember, ContextAll.
func (g *GameScene) thenCastAnNox(targetIdx int) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if !g.gameState.CastAnNox(targetIdx).Applied {
			return g.thenShowSpellNoEffect()(q)
		}

		// Visual effects via PartyStateBuilder
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				ToCharacter(targetIdx).
				FlashInverted().
				WithSound(game_state.SoundCure).
				Build()).ThenSequence(

			g.thenShowSpellSuccess("Cured!"))
	}
}

// thenCastAnZu applies the An Zu (Awaken) spell effect.
// Wakes a sleeping character.
// Circle 1 spell, TargetPartyMember, ContextAll.
func (g *GameScene) thenCastAnZu(targetIdx int) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		target := g.gameState.PartyState.GetCharacter(targetIdx)

		// Can't awaken dead characters
		if target.IsDead() {
			return g.thenShowSpellNoEffect()(q)
		}

		// Check if actually asleep - spell works but no effect
		if !target.IsAsleep() {
			return q.Then(func() {
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
			}).ThenSequence(

				g.thenShowSpellFeedback("Not asleep!", game_state.SoundNoEffect))
		}

		// Wake the character
		g.gameState.AwakenCharacter(targetIdx)

		// Visual effects via PartyStateBuilder
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				ToCharacter(targetIdx).
				FlashWhite().
				WithSound(game_state.SoundAwakenSpell).
				Build()).ThenSequence(

			g.thenShowSpellSuccess("Awakened!"))
	}
}

// thenCastInManiCorp applies the In Mani Corp (Resurrect) spell effect.
// Resurrects a dead character and recalculates progression from current karma.
// Circle 8 spell, TargetPartyMember, ContextNonCombat.
func (g *GameScene) thenCastInManiCorp(targetIdx int) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		target := g.gameState.PartyState.GetCharacter(targetIdx)

		// Can only resurrect dead characters
		if !target.IsDead() {
			return q.Then(func() {
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
			}).ThenSequence(

				g.thenShowSpellFeedback("Not dead!", game_state.SoundNoEffect))
		}

		// Resurrect the character
		g.gameState.ResurrectCharacter(targetIdx)

		// Visual effects via PartyStateBuilder (longer flash for major effect)
		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				ToCharacter(targetIdx).
				FlashWhite().
				WithSound(game_state.SoundResurrect).
				Build()).ThenSequence(

			g.thenShowSpellSuccess("Resurrected!"))
	}
}

// =============================================================================
// Direction-Target Spells
// =============================================================================

// thenCastAnYlem applies the An Ylem matter-destruction spell.
func (g *GameScene) thenCastAnYlem(direction references.Direction) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		targetPos := direction.GetNewPositionInDirection(&g.gameState.MapState.PlayerLocation.Position)
		if g.gameState.CastAnYlem(targetPos) != game_state.AnYlemSuccess {
			return g.thenShowSpellNoEffect()(q)
		}

		return q.ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				FlashInverted().
				WithSound(game_state.SoundMagic).
				Build()).ThenSequence(
			g.thenShowSpellSuccess("POOF!"))
	}
}

// thenCastAnSanct applies the An Sanct unlock/disarm spell.
func (g *GameScene) thenCastAnSanct(direction references.Direction) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		targetPos := direction.GetNewPositionInDirection(&g.gameState.MapState.PlayerLocation.Position)
		if g.gameState.GetCurrentSpellContext() == references.ContextOverworld {
			targetPos.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
		}

		switch g.gameState.CastAnSanct(targetPos) {
		case game_state.AnSanctDoorUnlocked:
			return q.ThenSequence(
				game_state.NewPartyStateBuilder(g.gameState).
					FlashInverted().
					WithSound(game_state.SoundUnlock).
					Build()).ThenSequence(
				g.thenShowSpellSuccess("Unlocked!"))
		case game_state.AnSanctChestDisarmed:
			return q.ThenSequence(
				game_state.NewPartyStateBuilder(g.gameState).
					FlashInverted().
					WithSound(game_state.SoundUnlock).
					Build()).ThenSequence(
				g.thenShowSpellSuccess("Disarmed!"))
		case game_state.AnSanctNoEffect:
			return g.thenShowSpellNoEffect()(q)
		default:
			panic("thenCastAnSanct: unknown result")
		}
	}
}

func (g *GameScene) thenCastDungeonPackedSpell(
	spell references.Spell,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.Then(func() {
			result := g.gameState.CastDungeonPackedSpell(spell)
			if !result.Applied {
				panic("validated dungeon packed spell resolved without a target")
			}

			//nolint:exhaustive // Every packed-cell dungeon spell has explicit feedback.
			switch spell {
			case references.AnSanct:
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundUnlock)
				if result.Disarmed {
					g.gameState.SystemCallbacks.Message.AddRowStr("Disarmed!")
				}
				g.gameState.SystemCallbacks.Message.AddRowStr("Chest opened!")
			case references.AnGrav:
				g.presentDungeonFieldDispel(result.Change)
				g.gameState.SystemCallbacks.Message.AddRowStr("Field destroyed!")
			case references.InFlamGrav,
				references.InNoxGrav,
				references.InZuGrav,
				references.InSanctGrav:
				// REDUX DEVIATION: DNG-0008: original dungeon field casting uses sound variant 4
				// for In Sanct Grav and sound variant 3 for the other fields.
				// Redux uses the shared semantic magic sound for field creation.
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
			default:
				panic("unknown dungeon packed spell feedback")
			}
			g.gameState.SystemCallbacks.Screen.UpdateDisplay()
		})
	}
}

// thenCastAnExPor applies the An Ex Por (Magic Lock) spell effect.
// Magically locks a door in the specified direction.
// Circle 5 spell, TargetDirection, ContextTown | ContextCombat.
func (g *GameScene) thenCastAnExPor(direction references.Direction) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenCastMagicDoorSpell(direction, game_state.SoundMagicLock, g.castMagicLockDoor)
}

// thenCastInExPor applies the In Ex Por (Magic Unlock) spell effect.
// Unlocks a magically locked door in the specified direction.
// Circle 5 spell, TargetDirection, ContextTown | ContextCombat.
func (g *GameScene) thenCastInExPor(direction references.Direction) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.thenCastMagicDoorSpell(direction, game_state.SoundMagicUnlock, g.castMagicUnlockDoor)
}

// Resolve the target and mutate the door only when the owning sequence reaches
// this step. Successful casts share the inverted flash followed by confirmation.
func (g *GameScene) thenCastMagicDoorSpell(
	direction references.Direction,
	sound game_state.SoundEffect,
	cast func(*references.Position) (bool, string),
) event_queue.Sequence {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		targetPos := direction.GetNewPositionInDirection(&g.gameState.MapState.PlayerLocation.Position)
		success, message := cast(targetPos)
		if success {
			return q.ThenSequence(
				game_state.NewPartyStateBuilder(g.gameState).
					FlashInverted().
					WithSound(sound).
					Build()).ThenSequence(
				g.thenShowSpellSuccess(message))
		}
		if message == "" {
			return g.thenShowSpellNoEffect()(q)
		}

		return g.thenShowSpellFeedback(message, game_state.SoundSpellFailure)(q)
	}
}

func (g *GameScene) castMagicLockDoor(targetPos *references.Position) (bool, string) {
	switch g.gameState.MagicLockDoor(targetPos) {
	case game_state.MagicLockSuccess:
		return true, "Locked!"
	case game_state.MagicLockAlreadyMagic:
		return false, "Already magic locked!"
	case game_state.MagicLockNotADoor:
		return false, "Not a door!"
	default:
		return false, ""
	}
}

func (g *GameScene) castMagicUnlockDoor(targetPos *references.Position) (bool, string) {
	switch g.gameState.MagicUnlockDoor(targetPos) {
	case game_state.MagicUnlockSuccess:
		return true, "Unlocked!"
	case game_state.MagicUnlockNotMagicDoor:
		return false, "Not a magic door!"
	case game_state.MagicUnlockNotADoor:
		return false, "Not a door!"
	default:
		return false, ""
	}
}

// thenCastAnGrav applies the An Grav (Dispel Field) spell effect.
// Dispels a magic field in the specified direction.
// Circle 4 spell, TargetDirection, ContextDungeon | ContextCombat.
func (g *GameScene) thenCastAnGrav(direction references.Direction) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		// Get target position
		playerPos := &g.gameState.MapState.PlayerLocation.Position
		targetPos := direction.GetNewPositionInDirection(playerPos)

		// Check if target is a magic field that can be dispelled
		result := g.gameState.DispelField(targetPos)

		switch result {
		case game_state.DispelFieldSuccess:
			return q.ThenSequence(
				game_state.NewPartyStateBuilder(g.gameState).
					FlashInverted().
					WithSound(game_state.SoundDispel).
					Build()).ThenSequence(

				g.thenShowSpellSuccess("Dispelled!"))

		case game_state.DispelFieldNotAField:
			return g.thenShowSpellFeedback("Nothing to dispel!", game_state.SoundSpellFailure)(q)
		default:
			return g.thenShowSpellNoEffect()(q)
		}
	}
}

// thenCastInPor applies the In Por (Blink) spell effect.
// Teleports the player to the farthest eligible grass in the selected direction.
// Circle 3 spell, TargetDirection, ContextOverworld | ContextCombat.
// Combat uses its separate automatic random-destination effect.
func (g *GameScene) thenCastInPor(direction references.Direction) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		result := g.gameState.BlinkTeleport(direction)

		switch result {
		case game_state.BlinkSuccess:
			return q.ThenSequence(
				game_state.NewPartyStateBuilder(g.gameState).
					FlashInverted().
					WithSound(game_state.SoundMagic).
					Build()).ThenSequence(

				g.thenShowSpellSuccess("Teleport!"))

		case game_state.BlinkBlocked:
			return g.thenShowSpellFeedback("Failed!", game_state.SoundSpellFailure)(q)
		case game_state.BlinkCombatNotSupported:
			// Combat dispatch must use the automatic effect instead of this direction route.
			return g.thenShowSpellFeedback("Not in combat!", game_state.SoundSpellFailure)(q)
		default:
			return g.thenShowSpellNoEffect()(q)
		}
	}
}

// thenCastRelHur applies the Rel Hur (Wind Change) spell effect.
// Changes the wind direction to the specified direction.
// Circle 2 spell, TargetDirection, ContextOverworld.
func (g *GameScene) thenCastRelHur(direction references.Direction) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if !g.gameState.SetWindByDirection(direction, true) {
			return g.thenShowSpellNoEffect()(q)
		}

		// Show success with direction name
		dirName := direction.GetDirectionCompassName()

		return q.Then(func() {
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
		}).ThenSequence(g.thenShowSpellSuccess("Wind now blows " + dirName + "!"))
	}
}

// =============================================================================
// Dungeon Floor Spells
// =============================================================================

// thenCastDungeonFloorSpell applies Uus Por or Des Por after atomic validation.
func (g *GameScene) thenCastDungeonFloorSpell(
	spell references.Spell,
	state *spellCastingWorkflowState,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		message := "Up!"
		if spell == references.DesPor {
			message = "Down!"
		}

		return q.Then(func() {
			result := g.gameState.CastDungeonFloorSpell(state.dungeonCommandToken, spell)
			if result.Outcome == game_state.DungeonFloorSpellFailed {
				panic("validated dungeon floor spell failed during dispatch")
			}
			if result.CommandCompleted {
				state.ownsDungeonCommand = false
				if spell == references.UusPor {
					g.gameState.SystemCallbacks.Message.AddRowStr("Exit to Britannia!")
				} else {
					g.gameState.SystemCallbacks.Message.AddRowStr("Exit to Underworld!")
				}
			}
		}).ThenSequence(
			game_state.NewPartyStateBuilder(g.gameState).
				FlashInverted().
				WithSound(game_state.SoundMagic).
				Build()).ThenSequence(
			g.thenShowSpellSuccess(message))
	}
}
