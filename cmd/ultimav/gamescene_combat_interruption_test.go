package main

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func newCombatInterruptionTestScene(t *testing.T) (*GameScene, *game_state.MockSystemCallbacks, game_state.CombatTurnIdentity) {
	t.Helper()
	scene, callbacks := newCombatUseTestScene(t)
	state := scene.gameState
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	state.ActiveDurationEffect = game_state.DurationEffect{}
	for index, active := range state.CombatScene.PartyActive {
		if !active {
			continue
		}
		identity := game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: index}
		character := state.PartyState.GetCharacter(index)
		character.SetStatus(party_state.Good)
		character.Dexterity = 10
		if index == 0 {
			character.Dexterity = 35
		}
		character.SetEquippedItem(references.SlotHelm, references.NoEquipment)
		character.SetEquippedItem(references.SlotWeapon, references.Bow)
		character.SetEquippedItem(references.SlotShield, references.MainGauche)
		state.CombatScene.PartySleeping[index] = false
		state.CombatScene.SetPartyMemberPosition(index, references.Position{X: references.Coordinate(index), Y: 10})
		entry := state.CombatScene.TurnQueue.GetEntryByIdentity(identity)
		state.CombatScene.TurnQueue.ReplaceCombatant(identity, identity, entry.Combatant)
	}
	origin := references.Position{X: 5, Y: 5}
	state.CombatScene.SetPartyMemberPosition(0, origin)
	setCombatConcealmentTerrain(state, origin, indexes.BrickFloor)
	engagerPosition := references.Position{X: 6, Y: 5}
	setCombatConcealmentTerrain(state, engagerPosition, indexes.BrickFloor)
	reference := (*state.GameReferences.EnemyReferences)[references.MonsterOrc]
	reference.Dexterity, reference.Strength, reference.HitPoints = 1, 1, 100
	monster := game_state.NewCombatMonster(references.MonsterOrc, &reference, engagerPosition, state.AsRandomProvider())
	index := state.CombatScene.AddMonsterToQueue(monster)
	step := nextSignatureMonsterStep(t, scene, index)
	callbacks.Reset()
	_, err := state.ResolveScheduledCombatStep(step.Token)
	require.NoError(t, err)
	require.NotEmpty(t, callbacks.CombatEvents, "production monster turn must remember its melee victim")
	attack := callbacks.CombatEvents[len(callbacks.CombatEvents)-1]
	require.Equal(t, game_state.CombatEventAttack, attack.Kind)
	require.True(t, attack.Target.IsPartyMember)
	require.Equal(t, 0, attack.Target.Index)
	require.NoError(t, state.CompleteScheduledCombatStep(step.Token))
	require.False(t, state.CombatScene.HasScheduledCombatStep())
	require.True(t, state.SelectCombatPlayer(0))
	actor := state.CombatScene.GetCurrentTurnIdentity()
	require.Equal(t, 0, actor.EntityIndex)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 2)
	scene.combatHUD = newCombatHUD()
	_, scene.combatHUD.eventOutput = widgets.GetScaledFontWithOutput(widgets.FontConfig{
		Size: combatHUDFontSize, LineSpacingMultiplier: 1, MaxLines: combatHUDOutputLines, MaxChars: combatHUDOutputChars,
	})
	scene.combatHUD.sync(state.CombatScene)
	publish := state.SystemCallbacks.Combat.PublishEvent
	state.SystemCallbacks.Combat = game_state.NewCombatCallbacks(func(event game_state.CombatEvent) {
		publish(event)
		scene.recordCombatEvent(event)
	}, scene.recordCombatConclusion)
	callbacks.Reset()
	return scene, callbacks, actor
}

func TestCombatInterruptionProductionKeyboardRoutesOnceToBothReportsAndAudio(t *testing.T) {
	for _, key := range []ebiten.Key{ebiten.KeyA, ebiten.KeyTab, ebiten.KeyC} {
		t.Run(fmt.Sprint(key), func(t *testing.T) {
			scene, callbacks, actor := newCombatInterruptionTestScene(t)
			state := scene.gameState
			date, calls := state.DateTime, state.GetRNG().Calls()
			scene.combatMapInputHandler(key)
			if key == ebiten.KeyTab {
				state.EventQueueManager.Update()
			}
			require.NotEmpty(t, callbacks.CombatEvents)
			event := callbacks.CombatEvents[0]
			require.Equal(t, game_state.CombatEventInterruption, event.Kind)
			assert.Equal(t, actor.EntityIndex, event.Target.Index)
			assert.Equal(t, 1, countEqual(callbacks.SoundEffectsPlayed, game_state.SoundCombatInterrupted))
			assert.Equal(t, 1, countEqual(callbacks.Messages, "Orc interferes!"))
			assert.Equal(t, uint16(2), state.PartyState.Inventory.Equipment.Get(references.Arrows))
			assert.Equal(t, actor, state.CombatScene.GetCurrentTurnIdentity(), "interruption does not complete the remaining source/action")
			wantAction := "bow attack"
			if key == ebiten.KeyA {
				require.NotNil(t, state.CombatScene.AttackTargeting)
				assert.Equal(t, references.MainGauche, state.CombatScene.AttackTargeting.CurrentSource().Equipment)
				assert.Equal(t, CombatAttackTargetInput, scene.secondaryKeyState)
			}
			if key == ebiten.KeyC {
				wantAction = "casting"
				assert.Equal(t, game_state.CombatInterruptedCast, event.InterruptedAction)
				assert.Zero(t, scene.dialogStack.Count())
				assert.Equal(t, calls, state.GetRNG().Calls())
				assert.Equal(t, date, state.DateTime)
			}
			want := "Orc interrupts Avatar's " + wantAction
			assert.Equal(t, want, scene.combatEventPlainText(event))
			scene.combatHUD.eventOutput.SetMaxCharsPerLine(120)
			scene.combatHUD.eventOutput.Reset()
			scene.appendCombatEventSentence(event)
			assert.Contains(t, scene.combatHUD.eventOutput.GetOutputStr(false), want)
			assert.NotEmpty(t, scene.combatEventDetailText(event))
			// Report redraw, wrapping and history navigation are presentation-only.
			beforeSounds, beforeEvents := len(callbacks.SoundEffectsPlayed), len(callbacks.CombatEvents)
			for _, size := range []image.Point{{1280, 720}, {1920, 1080}} {
				screen := newPresentationRaster(size.X, size.Y)
				_, report := combatLayout{}.turnAndReportRectangles(screen.Bounds())
				scene.combatHUD.detail = true
				scene.drawCombatEventTape(screen, combatHUDContentBounds(screen.Bounds(), report))
				scene.handleCombatHUDInput(ebiten.KeyPageUp)
				scene.handleCombatHUDInput(ebiten.KeyEnd)
				screen.Dispose()
			}
			assert.Len(t, callbacks.SoundEffectsPlayed, beforeSounds)
			assert.Len(t, callbacks.CombatEvents, beforeEvents)
			if key == ebiten.KeyTab {
				for range 20 {
					state.ElapsedMs += 1000
					state.EventQueueManager.Update()
					if !state.EventQueueManager.HasWorkflow(combatAutoAttackWorkflowName) {
						break
					}
				}
				assert.False(t, state.EventQueueManager.HasWorkflow(combatAutoAttackWorkflowName))
				assert.False(t, callbacks.InputBlocked)
				assert.Equal(t, 1, countEqual(callbacks.SoundEffectsPlayed, game_state.SoundCombatInterrupted))
				assert.Equal(t, 1, countEqual(callbacks.Messages, "Orc interferes!"))
			}
			state.EventQueueManager.Shutdown()
		})
	}
}

func TestCombatInterruptionReportFreezesIdentityAndConcealment(t *testing.T) {
	for _, concealed := range []bool{false, true} {
		scene := newCombatReportTestScene(t)
		position := references.Position{X: 6, Y: 5}
		index, monster := signatureMonster(t, scene, references.MonsterOrc, position)
		monster.SetInvisible(concealed)
		actor := scene.gameState.CombatScene.GetCurrentTurnIdentity()
		event := game_state.CombatEvent{Kind: game_state.CombatEventInterruption,
			Attacker: game_state.CombatFeedbackEntity{Index: index, Position: position},
			Target: game_state.CombatFeedbackEntity{IsPartyMember: true, Index: actor.EntityIndex, Slot: actor.Slot,
				Position: scene.gameState.CombatScene.GetCombatantPosition(actor)},
			Diagnostics: game_state.CombatDiagnostics{Source: references.Bow}}
		scene.recordCombatEvent(event)
		slot := scene.combatHUD.historyCount - 1
		monster.SetInvisible(false)
		scene.gameState.CombatScene.MonsterManager.RemoveMonster(index)
		replacement := game_state.NewCombatMonster(references.MonsterRat,
			&(*scene.gameState.GameReferences.EnemyReferences)[references.MonsterRat], position, scene.gameState.AsRandomProvider())
		scene.gameState.CombatScene.MonsterManager.AddMonster(replacement)
		scene.withCombatHistoryEvent(slot, func(historical game_state.CombatEvent) {
			message := scene.combatEventPlainText(historical)
			assert.Contains(t, message, "bow attack")
			assert.NotContains(t, message, "Rat")
			if concealed {
				assert.True(t, strings.HasPrefix(message, "Unknown interrupts "))
				assert.Empty(t, scene.combatEventDetailText(historical))
			} else {
				assert.True(t, strings.HasPrefix(message, "Orc interrupts "))
				assert.Contains(t, scene.combatEventDetailText(historical), "(6,5)")
			}
		})
	}
}
