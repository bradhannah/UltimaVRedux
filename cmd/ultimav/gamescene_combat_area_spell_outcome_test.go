package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func sceneAreaSpellSeed(t *testing.T, accepts func(*beradrng.Source) bool) uint64 {
	t.Helper()
	for seed := uint64(1); seed < 10000; seed++ {
		if accepts(beradrng.New(seed)) {
			return seed
		}
	}
	t.Fatal("no deterministic scene area-spell fixture seed")

	return 0
}

func newAreaSpellOutcomeScene(t *testing.T, spell references.Spell, kind references.MonsterType) (*GameScene, *game_state.MockSystemCallbacks, *game_state.CombatMonster) {
	t.Helper()
	scene, callbacks := newCombatUseTestScene(t)
	state := scene.gameState
	state.BeginCombatTurns(game_state.CombatTurnsPresented)
	combat := state.CombatScene
	combat.TurnQueue.RemoveAllMonsters()
	combat.MonsterManager.Clear()
	raw := &[references.XCombatMapTiles][references.YCombatMapTiles]byte{}
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			raw[x][y] = byte(indexes.BrickFloor)
			setCombatConcealmentTerrain(state, references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}, indexes.BrickFloor)
		}
	}
	combat.CombatMapRef.SetRawData(raw)
	for index, active := range combat.PartyActive {
		if active {
			combat.SetPartyMemberPosition(index, references.Position{X: references.Coordinate(index), Y: 10})
		}
	}
	casterIdx := combat.GetActiveCombatantIndex()
	combat.SetPartyMemberPosition(casterIdx, references.Position{X: 5, Y: 5})
	caster := state.PartyState.GetCharacter(casterIdx)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.Intelligence = 30
	caster.SetMp(30)
	state.PartyState.Inventory.Spells.Set(spell, 2)
	state.ConfirmSpellKnowledge(spell, game_state.ExperimentationKnowledgeSource())
	ref := (*state.GameReferences.EnemyReferences)[kind]
	monster := game_state.NewCombatMonster(kind, &ref, references.Position{X: 5, Y: 9}, beradrng.New(4))
	combat.AddMonsterToQueue(monster)
	publish := state.SystemCallbacks.Combat.PublishEvent
	state.SystemCallbacks.Combat.PublishEvent = func(event game_state.CombatEvent) {
		publish(event)
		scene.recordCombatEvent(event)
	}
	state.SetRandomSeed(23)
	callbacks.Reset()

	return scene, callbacks, monster
}

func TestCombatAreaSpellFullWorkflowPaysResolvesAndCompletesOnce(t *testing.T) {
	for _, test := range []struct {
		name    string
		spell   references.Spell
		keys    []ebiten.Key
		damage  bool
		outcome game_state.CombatSpellOutcome
	}{
		{name: "Python avoids earthquake", spell: references.InVasPorYlem, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyV, ebiten.KeyP, ebiten.KeyY}},
		{name: "Python takes earthquake damage", spell: references.InVasPorYlem, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyV, ebiten.KeyP, ebiten.KeyY}, damage: true},
		{name: "Sleep no targets", spell: references.InZu, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyZ}},
		{name: "Flame Wind no targets", spell: references.InFlamHur, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyF, ebiten.KeyH}},
		{name: "Poison Wind no targets", spell: references.InNoxHur, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyN, ebiten.KeyH}},
		{name: "Energy Vortex no targets", spell: references.InVasGravCorp, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyV, ebiten.KeyG, ebiten.KeyC}},
		{name: "Sleep resisted", spell: references.InZu, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyZ}, outcome: game_state.CombatSpellResisted},
		{name: "Sleep immune", spell: references.InZu, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyZ}, outcome: game_state.CombatSpellImmune},
		{name: "Sleep succeeds", spell: references.InZu, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyZ}, outcome: game_state.CombatSpellSlept},
		{name: "Vortex resisted", spell: references.InVasGravCorp, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyV, ebiten.KeyG, ebiten.KeyC}, outcome: game_state.CombatSpellResisted},
		{name: "Vortex immune", spell: references.InVasGravCorp, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyV, ebiten.KeyG, ebiten.KeyC}, outcome: game_state.CombatSpellImmune},
		{name: "Flame zero damage", spell: references.InFlamHur, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyF, ebiten.KeyH}, outcome: game_state.CombatSpellZeroDamage},
		{name: "Poison zero damage", spell: references.InNoxHur, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyN, ebiten.KeyH}, outcome: game_state.CombatSpellZeroDamage},
	} {
		t.Run(test.name, func(t *testing.T) {
			kind := references.MonsterSnake
			if test.outcome == game_state.CombatSpellImmune {
				kind = references.MonsterBlackthorn
			}
			scene, callbacks, python := newAreaSpellOutcomeScene(t, test.spell, kind)
			baseline, _, baselinePython := newAreaSpellOutcomeScene(t, test.spell, kind)
			state, expected := scene.gameState, baseline.gameState
			casterIdx := state.CombatScene.GetActiveCombatantIndex()
			caster := state.PartyState.GetCharacter(casterIdx)
			quake := test.spell == references.InVasPorYlem
			targetedCone := test.outcome != game_state.CombatSpellUnspecified
			var effectSeed, committedCalls uint64
			if targetedCone {
				python.SetPosition(references.Position{X: 5, Y: 3})
				baselinePython.SetPosition(python.Position())
				if test.outcome == game_state.CombatSpellImmune || test.outcome == game_state.CombatSpellSlept {
					caster.Intelligence = 255
					expected.PartyState.GetCharacter(casterIdx).Intelligence = 255
				}
				effectSeed = sceneAreaSpellSeed(t, func(rng *beradrng.Source) bool {
					if test.spell == references.InFlamHur {
						return rng.Intn(31) == 0
					}
					roll := max(1, rng.Intn(61)/2)
					if test.spell == references.InNoxHur {
						return roll >= python.CurrentDex() && rng.Intn(21) == 0
					}
					return test.outcome != game_state.CombatSpellResisted || roll < (30+python.EnemyRef().Intelligence-int(caster.Intelligence))/2
				})
			}
			if quake {
				seed := sceneAreaSpellSeed(t, func(rng *beradrng.Source) bool {
					hit := max(1, rng.Intn(61)/2) >= python.CurrentDex()
					return hit == test.damage && (!hit || rng.Intn(20)+1 < python.CurrentHP())
				})
				state.SetRandomSeed(seed)
				expected.SetRandomSeed(seed)
			}
			request := game_state.CastRequest{Spell: test.spell, CasterIdx: casterIdx, Direction: references.Up}
			var expectedResult game_state.CombatAreaSpellResult
			if quake {
				require.True(t, expected.ValidateAndConsumeSpellResources(request).Valid)
				expectedResult = expected.CastCombatEarthquakeResult(casterIdx)
			} else {
				require.True(t, expected.BeginCombatConeTargeting(request).Valid)
				expected.SetCombatConeDirection(references.Up)
				committed := expected.ConfirmCombatConeTarget()
				committedCalls = expected.GetRNG().Calls()
				if targetedCone {
					expected.SetRandomSeed(effectSeed)
				}
				expectedResult = expected.ResolveCommittedCombatCone(committed)
				if targetedCone {
					require.Len(t, expectedResult.Targets, 1)
					require.Equal(t, test.outcome, expectedResult.Targets[0].Outcome)
				}
			}
			expected.FinishCombatAction()
			// Exercise the production selector, payment, effect dispatch, and
			// cleanup; never call a spell effect separately on the actual scene.
			scene.startSpellCastingWorkflow()
			dialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			enterSpellSyllables(scene, dialog, test.keys)
			require.Equal(t, test.spell, dialog.GetSelectedSpell())
			assert.Equal(t, byte(30), caster.GetCurrentMp())
			assert.Equal(t, uint16(2), state.PartyState.Inventory.Spells.Get(test.spell))
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			dialog.Update()
			if !quake {
				for range 30 {
					state.ElapsedMs += 100
					state.EventQueueManager.Update()
					if state.CombatScene.ConeTargeting != nil && !state.EventQueueManager.HasWorkflow("spell_casting_workflow") {
						break
					}
				}
				require.NotNil(t, state.CombatScene.ConeTargeting)
				assert.Equal(t, byte(30), caster.GetCurrentMp())
				scene.keyboard.SimulateKeyPress(ebiten.KeyUp)
				scene.handleCombatConeTargetInput()
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
				scene.handleCombatConeTargetInput()
				require.True(t, state.EventQueueManager.HasWorkflow("combat_cone_spell_effect"))
				assert.Equal(t, committedCalls, state.GetRNG().Calls(), "confirmation retains the same randomized collection")
				if targetedCone {
					// Fix the resolution stream after paid geometry collection in
					// both fixtures, before any animated impact can resolve it.
					state.SetRandomSeed(effectSeed)
				}
			}
			sawCone, sawConeProgress := false, false
			for range 100 {
				state.ElapsedMs += 125
				state.EventQueueManager.Update()
				if scene.combatConeCast != nil {
					sawCone = true
					sawConeProgress = sawConeProgress || scene.combatConeCast.progress > 0
				}
				if !state.EventQueueManager.HasActiveWorkflows() {
					break
				}
			}
			require.False(t, state.EventQueueManager.HasActiveWorkflows())
			assert.False(t, callbacks.InputBlocked)
			assert.Nil(t, scene.combatConeCast)
			assert.Zero(t, scene.dialogStack.Count())
			assert.Equal(t, byte(30-int(test.spell)/6-1), caster.GetCurrentMp())
			assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(test.spell))
			assert.Equal(t, uint64(1), state.CastUseCommitSequence())
			assert.Equal(t, baselinePython.CurrentHP(), python.CurrentHP())
			assertCombatMatchesSingleFinish(t, state, expected)
			assert.Equal(t, expected.CombatScene.HasScheduledCombatStep(), state.CombatScene.HasScheduledCombatStep())
			assert.Equal(t, expected.GetRNG().Calls(), state.GetRNG().Calls())
			assert.Equal(t, expected.GetRNG().Intn(1000000), state.GetRNG().Intn(1000000))
			sound := game_state.SoundWhiteNoiseDamage
			if quake {
				sound = game_state.SoundEarthquake
				spellEvents := combatAreaResultEvents(callbacks.CombatEvents)
				require.Len(t, spellEvents, 1)
				assert.Equal(t, expectedResult.Targets[0], *spellEvents[0].SpellTarget)
				require.NotEmpty(t, callbacks.ScreenShakeOffsets)
				assert.Equal(t, game_state.ScreenShakeOffsetCall{}, callbacks.ScreenShakeOffsets[len(callbacks.ScreenShakeOffsets)-1])
			} else {
				assert.True(t, sawCone)
				assert.True(t, sawConeProgress, "empty casts retain their propagation animation")
				if targetedCone {
					spellEvents := combatAreaResultEvents(callbacks.CombatEvents)
					require.Len(t, spellEvents, 1)
					assert.Equal(t, expectedResult.Targets[0], *spellEvents[0].SpellTarget)
					assert.NotContains(t, callbacks.Messages, "No targets reached.")
					if summary := combatAreaSpellSummary(expectedResult); summary != "" {
						assert.Contains(t, callbacks.Messages, summary)
					}
				} else {
					assert.Equal(t, game_state.CombatAreaSpellNoTargets, expectedResult.Outcome())
					assert.Empty(t, combatAreaResultEvents(callbacks.CombatEvents))
					assert.Contains(t, callbacks.Messages, "No targets reached.")
				}
			}
			castSounds := 0
			for _, played := range callbacks.SoundEffectsPlayed {
				if played == sound {
					castSounds++
				}
			}
			assert.Equal(t, 1, castSounds)
			assert.NotContains(t, callbacks.SoundEffectsPlayed, game_state.SoundSpellFailure)
			if quake && !test.damage {
				assert.Contains(t, callbacks.Messages, "No effect: 1 avoided.")
			}
			// Extra queue ticks cannot pay, resolve, or finish the completed action again.
			calls, events := state.GetRNG().Calls(), len(callbacks.CombatEvents)
			for range 5 {
				state.ElapsedMs += 1000
				state.EventQueueManager.Update()
			}
			assert.Equal(t, calls, state.GetRNG().Calls())
			assert.Len(t, callbacks.CombatEvents, events)
			assertCombatMatchesSingleFinish(t, state, expected)
			assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(test.spell))
		})
	}
}

func TestCombatAreaSpellResultHasOneAccurateLocalImpact(t *testing.T) {
	for _, test := range []struct {
		name          string
		spell         references.Spell
		kind          references.MonsterType
		outcome       game_state.CombatSpellOutcome
		label, report string
	}{
		{name: "Sleep save", spell: references.InZu, kind: references.MonsterSnake, outcome: game_state.CombatSpellResisted, label: "RESIST", report: "resist"},
		{name: "Sleep immunity", spell: references.InZu, kind: references.MonsterBlackthorn, outcome: game_state.CombatSpellImmune, label: "IMMUNE", report: "immune"},
		{name: "Sleep success", spell: references.InZu, kind: references.MonsterSnake, outcome: game_state.CombatSpellSlept, label: "SLEEP", report: "sleep"},
		{name: "Vortex save", spell: references.InVasGravCorp, kind: references.MonsterSnake, outcome: game_state.CombatSpellResisted, label: "RESIST", report: "resist"},
		{name: "Vortex immunity", spell: references.InVasGravCorp, kind: references.MonsterBlackthorn, outcome: game_state.CombatSpellImmune, label: "IMMUNE", report: "immune"},
		{name: "Flame zero", spell: references.InFlamHur, kind: references.MonsterSnake, outcome: game_state.CombatSpellZeroDamage, label: "NO DAMAGE", report: "no damage"},
		{name: "Flame damage", spell: references.InFlamHur, kind: references.MonsterSnake, outcome: game_state.CombatSpellDamaged, label: "-1", report: "for 1"},
		{name: "Poison zero", spell: references.InNoxHur, kind: references.MonsterSnake, outcome: game_state.CombatSpellZeroDamage, label: "NO DAMAGE", report: "no damage"},
		{name: "Poison avoid", spell: references.InNoxHur, kind: references.MonsterSnake, outcome: game_state.CombatSpellAvoided, label: "AVOIDED", report: "avoid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene, callbacks, monster := newAreaSpellOutcomeScene(t, test.spell, test.kind)
			state := scene.gameState
			monster.SetPosition(references.Position{X: 5, Y: 3})
			casterIdx := state.CombatScene.GetActiveCombatantIndex()
			caster := state.PartyState.GetCharacter(casterIdx)
			caster.Intelligence = 255
			if test.outcome == game_state.CombatSpellResisted {
				caster.Intelligence = 0
			}
			require.True(t, state.BeginCombatConeTargeting(game_state.CastRequest{Spell: test.spell, CasterIdx: casterIdx}).Valid)
			state.SetCombatConeDirection(references.Up)
			committed := state.ConfirmCombatConeTarget()
			seed := sceneAreaSpellSeed(t, func(rng *beradrng.Source) bool {
				if test.spell == references.InFlamHur {
					if test.outcome == game_state.CombatSpellDamaged {
						return rng.Intn(31) == 1
					}

					return rng.Intn(31) == 0
				}
				roll := max(1, rng.Intn(61)/2)
				if test.spell == references.InNoxHur {
					if test.outcome == game_state.CombatSpellAvoided {
						return roll < monster.CurrentDex()
					}
					return roll >= monster.CurrentDex() && rng.Intn(21) == 0
				}
				if test.outcome == game_state.CombatSpellResisted {
					return roll < (30+monster.EnemyRef().Intelligence)/2
				}
				return true
			})
			state.SetRandomSeed(seed)
			hp := monster.CurrentHP()
			kapows := 0
			state.SystemCallbacks.Visual.PlayKapowAtPosition = func(position references.Position) {
				assert.Equal(t, monster.Position(), position)
				kapows++
			}
			result := state.ResolveCommittedCombatCone(committed)
			require.Len(t, result.Targets, 1)
			assert.Equal(t, test.outcome, result.Targets[0].Outcome)
			assert.Equal(t, hp-result.Targets[0].HPLost, monster.CurrentHP())
			assert.Equal(t, test.outcome == game_state.CombatSpellSlept, monster.IsAsleep())
			require.Len(t, callbacks.CombatEvents, 1)
			require.Len(t, scene.combatImpactEffects, 1, "one event owns one local effect")
			assert.Empty(t, callbacks.CombatImpactCalls, "no duplicate callback owner")
			label, _ := combatImpactText(scene.combatImpactEffects[0])
			assert.Equal(t, test.label, label)
			assert.Equal(t, test.spell == references.InFlamHur && test.outcome == game_state.CombatSpellDamaged, scene.combatImpactEffects[0].Fire)
			if test.outcome == game_state.CombatSpellSlept || test.outcome == game_state.CombatSpellDamaged {
				assert.Equal(t, 1, kapows, "successful cone arrivals keep their existing effect")
			} else {
				assert.Zero(t, kapows, "unsuccessful results must not look like a damaging impact")
			}
			assert.Contains(t, scene.combatEventPlainText(callbacks.CombatEvents[0]), test.report)
			calls := uint64(1)
			if test.spell == references.InNoxHur && test.outcome == game_state.CombatSpellZeroDamage {
				calls++
			}
			assert.Equal(t, calls, state.GetRNG().Calls())
			scene.presentCombatAreaSpellSummary(result)
			if test.outcome == game_state.CombatSpellSlept {
				assert.Empty(t, callbacks.Messages, "no ineffective-cast summary for successful sleep")
			}
			assert.Equal(t, calls, state.GetRNG().Calls(), "presentation never rerolls outcomes")
		})
	}
}

func combatAreaResultEvents(events []game_state.CombatEvent) []game_state.CombatEvent {
	var results []game_state.CombatEvent
	for _, event := range events {
		if event.SpellTarget != nil {
			results = append(results, event)
		}
	}

	return results
}

func TestCombatAreaSpellSummaryRespectsDisclosureAndStatusSuccess(t *testing.T) {
	result := game_state.CombatAreaSpellResult{Targets: []game_state.CombatAreaSpellTargetResult{
		{Disclosed: true, Outcome: game_state.CombatSpellAvoided},
		{Disclosed: true, Outcome: game_state.CombatSpellResisted},
		{Disclosed: true, Outcome: game_state.CombatSpellImmune},
		{Disclosed: true, Outcome: game_state.CombatSpellZeroDamage},
	}}
	assert.Equal(t, "No effect: 1 avoided; 1 resisted; 1 immune; 1 took no damage.", combatAreaSpellSummary(result))
	result.Targets[0].Disclosed = false
	assert.Equal(t, "No visible effect.", combatAreaSpellSummary(result))
	for _, success := range []game_state.CombatSpellOutcome{game_state.CombatSpellDamaged, game_state.CombatSpellSlept, game_state.CombatSpellPoisoned} {
		result.Targets[1].Outcome = success
		assert.Empty(t, combatAreaSpellSummary(result))
	}
}

func TestCombatAreaSpellConcealedTargetNeverAcquiresLocalFeedback(t *testing.T) {
	scene, callbacks, monster := newAreaSpellOutcomeScene(t, references.InZu, references.MonsterSnake)
	state := scene.gameState
	monster.SetPosition(references.Position{X: 5, Y: 3})
	casterIdx := state.CombatScene.GetActiveCombatantIndex()
	state.PartyState.GetCharacter(casterIdx).Intelligence = 255
	require.True(t, state.BeginCombatConeTargeting(game_state.CastRequest{Spell: references.InZu, CasterIdx: casterIdx}).Valid)
	state.SetCombatConeDirection(references.Up)
	committed := state.ConfirmCombatConeTarget()
	monster.SetInvisible(true)
	result := state.ResolveCommittedCombatCone(committed)
	require.Len(t, result.Targets, 1)
	assert.True(t, monster.IsAsleep())
	assert.False(t, result.Targets[0].Disclosed)
	assert.Empty(t, scene.combatImpactEffects)
	assert.Equal(t, "No visible effect.", combatAreaSpellSummary(result))
	require.Len(t, callbacks.CombatEvents, 1)
	assert.Contains(t, scene.combatEventPlainText(callbacks.CombatEvents[0]), "unseen target")
	assert.NotContains(t, scene.combatEventPlainText(callbacks.CombatEvents[0]), "Python")
	assert.Empty(t, scene.combatEventDetailText(callbacks.CombatEvents[0]))
}

func TestCombatAreaSpellImpactRejectsRecycledMonsterIdentity(t *testing.T) {
	scene, _, monster := newAreaSpellOutcomeScene(t, references.InVasPorYlem, references.MonsterSnake)
	state := scene.gameState
	casterIdx := state.CombatScene.GetActiveCombatantIndex()
	seed := sceneAreaSpellSeed(t, func(rng *beradrng.Source) bool {
		return max(1, rng.Intn(61)/2) < monster.CurrentDex()
	})
	state.SetRandomSeed(seed)
	state.CastCombatEarthquakeResult(casterIdx)
	require.Len(t, scene.combatImpactEffects, 1)
	effect := scene.combatImpactEffects[0]
	assert.True(t, scene.combatAreaImpactStillDisclosed(effect))
	monster.SetInvisible(true)
	assert.False(t, scene.combatAreaImpactStillDisclosed(effect))
	monster.SetInvisible(false)
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	state.CombatScene.MonsterManager.Clear()
	ref := *monster.EnemyRef()
	replacement := game_state.NewCombatMonster(references.MonsterSnake, &ref, monster.Position(), beradrng.New(4))
	state.CombatScene.AddMonsterToQueue(replacement)
	require.NotEqual(t, monster.CombatantID(), replacement.CombatantID())
	assert.False(t, scene.combatAreaImpactStillDisclosed(effect))
	scene.updateCombatImpactEffects(0)
	assert.Empty(t, scene.combatImpactEffects)
}
