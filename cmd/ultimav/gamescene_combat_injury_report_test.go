package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func setCombatHPBarsForTest(scene *GameScene, visible bool) {
	if scene.gameConfig == nil {
		scene.gameConfig = &config.UltimaVConfiguration{}
	}
	if scene.gameConfig.GameplayConfiguration == nil {
		scene.gameConfig.GameplayConfiguration = config.DefaultGameplayConfiguration()
	}
	scene.gameConfig.GameplayConfiguration.ShowCombatHPBars = visible
}

func injuryReportFixture(t *testing.T, scene *GameScene) (game_state.CombatEvent, *game_state.CombatMonster) {
	t.Helper()
	scene.gameState.CombatScene.MonsterManager.Clear()
	scene.gameState.CombatScene.TurnQueue.RemoveAllMonsters()
	index, monster := signatureMonster(t, scene, references.MonsterOrc, references.Position{X: 5, Y: 3})
	actor := scene.gameState.CombatScene.GetCurrentTurnIdentity()
	return game_state.CombatEvent{
		Kind: game_state.CombatEventAttack, Result: game_state.CombatFeedbackDamage, HPLost: 12,
		Attacker: game_state.CombatFeedbackEntity{IsPartyMember: true, Index: actor.EntityIndex, Slot: actor.Slot},
		Target:   game_state.CombatFeedbackEntity{Index: index, Position: monster.Position()},
		MonsterInjury: game_state.CombatMonsterInjury{
			Severity: game_state.CombatInjuryHeavilyWounded, Disclosed: true,
			MonsterType: references.MonsterOrc, CombatantID: monster.CombatantID(),
			Loyalty: game_state.CombatMonsterLoyaltyMonster,
		},
	}, monster
}

func TestCombatInjuryReportHistorySurvivesHealingDeathReuseAndToggles(t *testing.T) {
	scene := newCombatReportTestScene(t)
	setCombatHPBarsForTest(scene, true)
	event, monster := injuryReportFixture(t, scene)
	scene.recordCombatEvent(event)
	slot := (scene.combatHUD.historyStart + scene.combatHUD.historyCount - 1) % combatEventHistoryCapacity
	want := ""
	scene.withCombatHistoryEvent(slot, func(record game_state.CombatEvent) {
		want = scene.combatEventPlainText(record)
		assert.Contains(t, want, "for 12")
		assert.NotContains(t, want, "wounded")
	})
	for _, mutate := range []func(){
		func() { monster.TakeDamage(1) },
		func() { monster.Heal(100) },
		func() { monster.SetCurrentHP(0) },
		func() {
			scene.gameState.CombatScene.MonsterManager.Clear()
			scene.gameState.CombatScene.TurnQueue.RemoveAllMonsters()
			index, _ := signatureMonster(t, scene, references.MonsterTroll, event.Target.Position)
			require.Equal(t, event.Target.Index, index)
		},
	} {
		mutate()
		for _, showBars := range []bool{false, true, false} {
			setCombatHPBarsForTest(scene, showBars)
			scene.withCombatHistoryEvent(slot, func(record game_state.CombatEvent) {
				expected := want
				if !showBars {
					expected += ". Orc is heavily wounded!"
				}
				assert.Equal(t, expected, scene.combatEventPlainText(record))
				assert.Equal(t, game_state.CombatInjuryHeavilyWounded, record.MonsterInjury.Severity)
			})
		}
	}
}

func TestCombatInjuryReportFrozenDisclosureAndExclusions(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*game_state.CombatEvent)
	}{
		{"hidden at resolution", func(event *game_state.CombatEvent) { event.MonsterInjury.Disclosed = false }},
		{"no injury", func(event *game_state.CombatEvent) { event.MonsterInjury.Severity = game_state.CombatInjuryNone }},
		{"zero loss", func(event *game_state.CombatEvent) { event.HPLost = 0 }},
		{"fatal", func(event *game_state.CombatEvent) { event.Fatal = true }},
		{"miss", func(event *game_state.CombatEvent) { event.Result = game_state.CombatFeedbackMiss }},
		{"sleep", func(event *game_state.CombatEvent) { event.Result = game_state.CombatFeedbackSleep }},
		{"poison status", func(event *game_state.CombatEvent) { event.Result = game_state.CombatFeedbackPoison }},
		{"blocked", func(event *game_state.CombatEvent) { event.Diagnostics.ProjectileBlocked = true }},
		{"PC", func(event *game_state.CombatEvent) { event.Target.IsPartyMember = true; event.Target.Index = 0 }},
		{"PC clone", func(event *game_state.CombatEvent) {
			event.Target.IsPartyMember = true
			event.Target.Index = 0
			event.Target.Slot = 1
		}},
		{"hidden area", func(event *game_state.CombatEvent) {
			event.Kind = game_state.CombatEventSpell
			event.Diagnostics.Spell = references.InFlamHur
			event.SpellTarget = &game_state.CombatAreaSpellTargetResult{Disclosed: false, Outcome: game_state.CombatSpellDamaged}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene := newCombatReportTestScene(t)
			setCombatHPBarsForTest(scene, false)
			event, _ := injuryReportFixture(t, scene)
			test.change(&event)
			// Admission identities come from the resolution snapshot even if the
			// live target is now visible. No impact execution is needed here.
			slot := scene.combatHUD.addEvent(event)
			scene.combatHUD.identities[slot] = scene.captureCombatReportIdentities(event)
			scene.withCombatHistoryEvent(slot, func(record game_state.CombatEvent) {
				assert.Empty(t, scene.combatInjurySuffix(record))
				assert.NotContains(t, scene.combatEventPlainText(record), "wounded")
			})
		})
	}
}

func TestCombatInjuryReportsStyledPlainAndPagingUseOnePolicy(t *testing.T) {
	for _, kind := range []game_state.CombatEventKind{game_state.CombatEventAttack, game_state.CombatEventRangedAttack,
		game_state.CombatEventSpell, game_state.CombatEventFieldTriggered} {
		for _, area := range []bool{false, true} {
			if area && kind != game_state.CombatEventSpell && kind != game_state.CombatEventFieldTriggered {
				continue
			}
			t.Run(fmt.Sprintf("kind%d/area%t", kind, area), func(t *testing.T) {
				scene := newCombatReportTestScene(t)
				event, _ := injuryReportFixture(t, scene)
				event.Kind, event.FieldKind = kind, game_state.CombatFieldFire
				event.Diagnostics.Spell = references.InFlamHur
				if kind == game_state.CombatEventFieldTriggered && area {
					event.FieldKind = game_state.CombatFieldPoison
				}
				if kind == game_state.CombatEventSpell && area {
					event.SpellTarget = &game_state.CombatAreaSpellTargetResult{
						Target: event.Target, Disclosed: true, Outcome: game_state.CombatSpellDamaged,
						HPLost: event.HPLost, MonsterInjury: event.MonsterInjury,
					}
				}
				hud := scene.combatHUD
				hud.historyCount = 0
				for range 8 {
					slot := hud.addEvent(event)
					hud.identities[slot] = scene.captureCombatReportIdentities(event)
				}
				for _, showBars := range []bool{true, false, true} {
					setCombatHPBarsForTest(scene, showBars)
					for _, width := range []int{24, 40, 80} {
						hud.eventMaxChars = width
						hud.eventOutput.SetMaxCharsPerLine(width)
						hud.eventOutput.Reset()
						scene.withCombatHistoryEvent(0, func(record game_state.CombatEvent) {
							plain := scene.combatEventPlainText(record)
							scene.appendCombatEvent(record)
							styled := strings.Join(strings.Fields(hud.eventOutput.GetOutputStr(false)), " ")
							assert.Contains(t, styled, strings.Join(strings.Fields(plain), " "))
							assert.Contains(t, plain, "12")
							assert.Equal(t, !showBars, strings.Contains(plain, "Orc is heavily wounded!"))
							assert.Equal(t, scene.combatEventLineCount(record), hud.eventOutput.GetPanelContentLineCount())
						})
						lines := scene.combatHistoryEventLineCount(0)
						start, count := scene.visibleCombatEventRange(2 * lines)
						assert.Equal(t, 6, start)
						assert.Equal(t, 2, count)
						hud.historyOffset = 3
						start, count = scene.visibleCombatEventRange(2 * lines)
						assert.Equal(t, 3, start)
						assert.Equal(t, 2, count)
						hud.historyOffset = 0
					}
				}
			})
		}
	}
}

func TestCombatInjuryReportUsesResolutionIdentityBeforeAdmission(t *testing.T) {
	scene := newCombatReportTestScene(t)
	setCombatHPBarsForTest(scene, false)
	event, _ := injuryReportFixture(t, scene)
	state := scene.gameState
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	index, replacement := signatureMonster(t, scene, references.MonsterTroll, event.Target.Position)
	require.Equal(t, event.Target.Index, index)
	require.NotEqual(t, event.MonsterInjury.CombatantID, replacement.CombatantID())
	scene.recordCombatEvent(event)
	slot := (scene.combatHUD.historyStart + scene.combatHUD.historyCount - 1) % combatEventHistoryCapacity
	scene.withCombatHistoryEvent(slot, func(record game_state.CombatEvent) {
		report := scene.combatEventPlainText(record)
		assert.Contains(t, report, "Orc is heavily wounded!")
		assert.NotContains(t, report, "Troll")
	})
}

func TestCombatInjuryDirectPoisonKeepsOneNumericReportAndHitCue(t *testing.T) {
	scene := newCombatReportTestScene(t)
	event, _ := injuryReportFixture(t, scene)
	event.Kind = game_state.CombatEventFieldTriggered
	event.FieldKind = game_state.CombatFieldPoison
	event.Diagnostics = game_state.CombatDiagnostics{Spell: references.InNoxGrav, MagicDamage: true}
	var sounds []game_state.SoundEffect
	scene.gameState.SystemCallbacks.Audio.PlaySoundEffect = func(sound game_state.SoundEffect) { sounds = append(sounds, sound) }
	scene.recordCombatEvent(event)
	require.Equal(t, []game_state.SoundEffect{game_state.SoundCombatHit}, sounds)
	slot := (scene.combatHUD.historyStart + scene.combatHUD.historyCount - 1) % combatEventHistoryCapacity
	for _, visible := range []bool{true, false, true} {
		setCombatHPBarsForTest(scene, visible)
		scene.withCombatHistoryEvent(slot, func(record game_state.CombatEvent) {
			plain := scene.combatEventPlainText(record)
			assert.Equal(t, 1, strings.Count(plain, "takes 12 from"))
			assert.Equal(t, !visible, strings.Contains(plain, "heavily wounded!"))
			scene.combatHUD.eventOutput.Reset()
			scene.appendCombatEvent(record)
		})
	}
	assert.Len(t, sounds, 1, "history formatting and toggles cannot replay the hit cue")
}

func TestCombatFieldReportSlotZeroUsesVictimDisclosureAndFrozenHistory(t *testing.T) {
	for _, targetVisible := range []bool{false, true} {
		t.Run(fmt.Sprintf("targetVisible%t", targetVisible), func(t *testing.T) {
			scene := newCombatReportTestScene(t)
			state := scene.gameState
			state.CombatScene.MonsterManager.Clear()
			state.CombatScene.TurnQueue.RemoveAllMonsters()
			position := references.Position{X: 5, Y: 3}
			reference := (*state.GameReferences.EnemyReferences)[references.MonsterOrc]
			reference.HitPoints = 20
			reference.EnemyAbilities = nil
			monster := game_state.NewCombatMonster(references.MonsterOrc, &reference, position, state.AsRandomProvider())
			monster.SetCurrentHP(12)
			monster.SetLoyalty(game_state.CombatMonsterLoyaltyParty)
			index := state.CombatScene.AddMonsterToQueue(monster)
			require.Zero(t, index, "exercise the collision with the absent attacker's zero identity")
			identity := game_state.CombatTurnIdentity{EntityIndex: index}
			state.CombatScene.CurrentTurnIdentity = identity
			state.CombatScene.ActiveMonsterIndex = index
			require.Equal(t, identity, state.CombatScene.GetCurrentTurnIdentity())
			targetTile, dummyTile := indexes.SpriteIndex(indexes.BrickWallArchway), indexes.SpriteIndex(indexes.BrickFloor)
			if targetVisible {
				targetTile, dummyTile = dummyTile, targetTile
			}
			state.CombatScene.CombatMapRef.SetTileNumber(position, targetTile)
			setCombatConcealmentTerrain(state, position, targetTile)
			setCombatConcealmentTerrain(state, references.Position{}, dummyTile)
			require.Equal(t, targetVisible, state.CombatTerrainAllowsObjectDisplay(position))
			require.Equal(t, !targetVisible, state.CombatTerrainAllowsObjectDisplay(references.Position{}))
			require.True(t, state.CombatScene.AddField(game_state.NewCombatField(game_state.CombatFieldFire, position)))
			state.SetRandomSeed(sceneAreaSpellSeed(t, func(rng *beradrng.Source) bool {
				return rng.Intn(256) >= 16 && rng.Intn(11) == 7
			}))
			var triggers []game_state.CombatEvent
			state.SystemCallbacks.Combat.PublishEvent = func(event game_state.CombatEvent) {
				if event.Kind == game_state.CombatEventFieldTriggered {
					triggers = append(triggers, event)
				}
				scene.recordCombatEvent(event)
			}
			state.FinishCombatActionDeferred()
			require.Len(t, triggers, 1, "use the actual standing-field producer")
			event := triggers[0]
			require.Equal(t, game_state.CombatFeedbackEntity{}, event.Attacker)
			require.Equal(t, index, event.Target.Index)
			require.Equal(t, 7, event.HPLost)
			require.Equal(t, game_state.CombatInjuryHeavilyWounded, event.MonsterInjury.Severity)
			require.Equal(t, targetVisible, event.MonsterInjury.Disclosed)
			slot := (scene.combatHUD.historyStart + scene.combatHUD.historyCount - 1) % combatEventHistoryCapacity
			require.Equal(t, game_state.CombatEventFieldTriggered, scene.combatHUD.history[slot].Kind)
			assert.Empty(t, scene.combatHUD.identities[slot].Attacker.Name, "absent attacker cannot disclose a live monster")
			checkHistory := func() {
				for _, bars := range []bool{true, false} {
					setCombatHPBarsForTest(scene, bars)
					scene.withCombatHistoryEvent(slot, func(record game_state.CombatEvent) {
						want := "Unknown is burned for 7"
						if targetVisible {
							want = "Orc is burned for 7"
							if !bars {
								want += ". Orc is heavily wounded!"
							}
						}
						assert.Equal(t, want, scene.combatEventPlainText(record))
						for _, width := range []int{24, 80} {
							scene.combatHUD.eventMaxChars = width
							scene.combatHUD.eventOutput.SetMaxCharsPerLine(width)
							scene.combatHUD.eventOutput.Reset()
							scene.appendCombatEvent(record)
							styled := strings.Join(strings.Fields(scene.combatHUD.eventOutput.GetOutputStr(false)), " ")
							assert.Contains(t, styled, want)
							assert.NotContains(t, styled, "Troll")
							if !targetVisible {
								assert.NotContains(t, styled, "Orc")
								assert.NotContains(t, styled, "wounded")
							}
							assert.Equal(t, scene.combatHistoryEventLineCount(slot), scene.combatHUD.eventOutput.GetPanelContentLineCount())
						}
					})
				}
			}
			checkHistory()
			state.CombatScene.MonsterManager.Clear()
			state.CombatScene.TurnQueue.RemoveAllMonsters()
			replacementIndex, replacement := signatureMonster(t, scene, references.MonsterTroll, position)
			require.Equal(t, index, replacementIndex)
			require.NotEqual(t, event.MonsterInjury.CombatantID, replacement.CombatantID())
			setCombatConcealmentTerrain(state, position, dummyTile)
			setCombatConcealmentTerrain(state, references.Position{}, targetTile)
			checkHistory()
		})
	}
}

func TestCombatReportRealSlotZeroAttackerAndSelfTargetRemainPresent(t *testing.T) {
	for _, test := range []struct{ selfTarget, sourceVisible bool }{{false, false}, {false, true}, {true, false}, {true, true}} {
		t.Run(fmt.Sprintf("selfTarget%t/sourceVisible%t", test.selfTarget, test.sourceVisible), func(t *testing.T) {
			scene := newCombatReportTestScene(t)
			event, monster := injuryReportFixture(t, scene)
			require.Zero(t, event.Target.Index)
			sourceTile, dummyTile := indexes.SpriteIndex(indexes.BrickWallArchway), indexes.SpriteIndex(indexes.BrickFloor)
			if test.sourceVisible {
				sourceTile, dummyTile = dummyTile, sourceTile
			}
			setCombatConcealmentTerrain(scene.gameState, references.Position{}, dummyTile)
			setCombatConcealmentTerrain(scene.gameState, monster.Position(), sourceTile)
			event.Attacker = event.Target
			if !test.selfTarget {
				index, target := signatureMonster(t, scene, references.MonsterTroll, references.Position{X: 7, Y: 3})
				event.Target = game_state.CombatFeedbackEntity{Index: index, Position: target.Position()}
			}
			event.MonsterInjury = game_state.CombatMonsterInjury{}
			scene.recordCombatEvent(event)
			slot := (scene.combatHUD.historyStart + scene.combatHUD.historyCount - 1) % combatEventHistoryCapacity
			sourceName := combatUnknownEntityLabel
			if test.sourceVisible {
				sourceName = "Orc"
			}
			require.Equal(t, sourceName, scene.combatHUD.identities[slot].Attacker.Name)
			require.Equal(t, test.sourceVisible, scene.combatHUD.identities[slot].Attacker.Disclosed)
			require.Equal(t, test.sourceVisible, scene.combatHUD.identities[slot].DetailsDisclosed)
			targetName := "Troll"
			if test.selfTarget {
				targetName = sourceName
			}
			want := sourceName + " hits " + targetName + " for 12"
			scene.withCombatHistoryEvent(slot, func(record game_state.CombatEvent) {
				assert.Equal(t, want, scene.combatEventPlainText(record))
				scene.combatHUD.eventOutput.Reset()
				scene.combatHUD.eventOutput.SetMaxCharsPerLine(80)
				scene.appendCombatEvent(record)
				assert.Contains(t, strings.Join(strings.Fields(scene.combatHUD.eventOutput.GetOutputStr(false)), " "), want)
			})
		})
	}
}
