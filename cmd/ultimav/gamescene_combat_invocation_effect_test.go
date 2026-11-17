package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCombatSummonPhasesUseContractingCasterAndBindingColors(t *testing.T) {
	assert.Less(t, combatSummonCasterInset(0), combatSummonCasterInset(1),
		"caster corners must contract inward")
	assert.Greater(t,
		combatSummonAllegianceInset(game_state.CombatSummonBoundToParty, 0),
		combatSummonAllegianceInset(game_state.CombatSummonBoundToParty, 1),
		"loyal brackets settle outward",
	)
	assert.Greater(t,
		combatSummonAllegianceInset(game_state.CombatSummonBindingFailed, 0),
		combatSummonAllegianceInset(game_state.CombatSummonBindingFailed, 1),
		"failed binding ruptures outward",
	)
	assert.Equal(t, ucolor.EGALightCyan, combatSummonAllegianceColor(game_state.CombatSummonBoundToParty))
	assert.Equal(t, ucolor.EGALightRed, combatSummonAllegianceColor(game_state.CombatSummonBindingFailed))
}

func TestCombatSummonManifestationUsesStableSteppedPhases(t *testing.T) {
	assert.Equal(t, 0, combatManifestationStep(0))
	assert.Equal(t, 1, combatManifestationStep(0.25))
	assert.Equal(t, 2, combatManifestationStep(0.5))
	assert.Equal(t, 3, combatManifestationStep(0.75))
	assert.Equal(t, 3, combatManifestationStep(1))

	regular := [4]uint64{}
	regular[0], regular[1], regular[2], regular[3] = combatSummonDurations(references.KalXen)
	daemon := [4]uint64{}
	daemon[0], daemon[1], daemon[2], daemon[3] = combatSummonDurations(references.KalXenCorp)
	assert.Equal(t, [4]uint64{120, 140, 300, 180}, regular)
	assert.Equal(t, [4]uint64{150, 180, 420, 280}, daemon)
}

func TestCombatRepelTargetGeometryDistinguishesAffectedAndResisted(t *testing.T) {
	assert.Greater(t, combatRepelTargetInset(true, 0), combatRepelTargetInset(true, 1),
		"affected undead brackets expand outward")
	assert.Less(t, combatRepelTargetInset(false, 0), combatRepelTargetInset(false, 1),
		"resisted and immune undead brackets contract inward")
	assert.Equal(t, ucolor.EGAWhite, combatRepelCasterColor(0))
	assert.Equal(t, ucolor.EGAYellow, combatRepelCasterColor(1))
	assert.Equal(t, ucolor.EGAYellow, combatRepelTargetColor(true))
	assert.Equal(t, ucolor.EGADarkGray, combatRepelTargetColor(false))
}

func TestCombatRepelAffectedPulseFlashesThreeTimesAndEndsCleanly(t *testing.T) {
	for pulse := range combatRepelTargetPulses {
		start := float32(pulse) / combatRepelTargetPulses
		middle := start + combatRepelPulseVisiblePart/(2*combatRepelTargetPulses)
		end := start + combatRepelPulseVisiblePart/combatRepelTargetPulses

		clr, visible := combatRepelPulseColor(start)
		assert.True(t, visible, "pulse %d must begin visible", pulse)
		assert.Equal(t, ucolor.EGAWhite, clr)

		clr, visible = combatRepelPulseColor(middle)
		assert.True(t, visible, "pulse %d must transition to yellow", pulse)
		assert.Equal(t, ucolor.EGAYellow, clr)

		_, visible = combatRepelPulseColor(end)
		assert.False(t, visible, "pulse %d must include a visible break", pulse)
	}
	_, visible := combatRepelPulseColor(1)
	assert.False(t, visible, "completed repel effects must leave no silhouette")
}

func TestCombatRepelOnlyAnimatesTargetsThatWereConsidered(t *testing.T) {
	assert.False(t, combatRepelHasTargets(game_state.CombatAnXenCorpResult{}),
		"a cast with no undead must not wait through an empty target animation")
	assert.True(t, combatRepelHasTargets(game_state.CombatAnXenCorpResult{
		Resisted: []game_state.CombatSpellMonster{{}},
	}))
}

func TestCombatInsectStackPipsAreStable(t *testing.T) {
	previousX := -1.0
	for member := range 4 {
		x, y := combatInsectStackPipPosition(member, 4)
		assert.Greater(t, x, previousX)
		assert.Equal(t, 2.0, y)
		previousX = x
	}
}

func TestCombatSummonSequenceMutatesOnceAndCleansPresentation(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 1)
	scene := &GameScene{gameState: gs}
	casterIdx := gs.CombatScene.GetActiveCombatantIndex()
	before := gs.CombatScene.MonsterManager.Count()
	finished := false

	gs.EventQueueManager.Workflow("test_combat_summon_presentation").
		Priority(event_queue.PriorityHigh).
		Sequential().
		First(func() {}).
		ThenSequence(scene.thenCastCombatSummon(references.KalXen, casterIdx)).
		Then(func() { finished = true }).
		Finally(scene.clearCombatInvocationEffects).
		Start()
	for range 4 {
		gs.EventQueueManager.Update()
	}
	require.NotNil(t, scene.combatSummonEffect)
	assert.Equal(t, before+1, gs.CombatScene.MonsterManager.Count(), "summon mutation occurs before presentation")

	for range 20 {
		gs.ElapsedMs += 100
		gs.EventQueueManager.Update()
	}
	assert.True(t, finished)
	assert.Nil(t, scene.combatSummonEffect)
	assert.Equal(t, before+1, gs.CombatScene.MonsterManager.Count(), "presentation must not repeat the core summon")
}

func TestCombatInvocationCleanupClearsEveryTemporaryChannel(t *testing.T) {
	scene := &GameScene{
		combatSummonEffect: &combatSummonSpellEffect{},
		combatRepelEffect:  &combatRepelSpellEffect{},
	}

	scene.clearCombatInvocationEffects()

	assert.Nil(t, scene.combatSummonEffect)
	assert.Nil(t, scene.combatRepelEffect)
}

func TestCombatRepelAndFearCastsFinishTheCombatActionOnce(t *testing.T) {
	for _, test := range []struct {
		name  string
		spell references.Spell
		keys  []ebiten.Key
		cast  func(*game_state.GameState, int) game_state.CombatAnXenCorpResult
	}{
		{
			name: "Repel Undead", spell: references.AnXenCorp,
			keys: []ebiten.Key{ebiten.KeyA, ebiten.KeyX, ebiten.KeyC},
			cast: (*game_state.GameState).CastCombatAnXenCorp,
		},
		{
			name: "Fear", spell: references.InQuasCorp,
			keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyQ, ebiten.KeyC},
			cast: (*game_state.GameState).CastCombatInQuasCorp,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene, callbacks, _ := newAreaSpellOutcomeScene(t, test.spell, references.MonsterSkeleton)
			baseline, _, _ := newAreaSpellOutcomeScene(t, test.spell, references.MonsterSkeleton)
			state, expected := scene.gameState, baseline.gameState
			casterIdx := state.CombatScene.GetActiveCombatantIndex()
			request := game_state.CastRequest{Spell: test.spell, CasterIdx: casterIdx}
			require.True(t, expected.ValidateAndConsumeSpellResources(request).Valid)
			test.cast(expected, casterIdx)
			expected.FinishCombatAction()

			scene.startSpellCastingWorkflow()
			dialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			enterSpellSyllables(scene, dialog, test.keys)
			require.Equal(t, test.spell, dialog.GetSelectedSpell())
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			dialog.Update()
			for range 100 {
				state.ElapsedMs += 125
				state.EventQueueManager.Update()
				if !state.EventQueueManager.HasActiveWorkflows() {
					break
				}
			}

			require.False(t, state.EventQueueManager.HasActiveWorkflows())
			assert.False(t, callbacks.InputBlocked)
			assert.Nil(t, scene.combatRepelEffect)
			assert.Zero(t, scene.dialogStack.Count())
			assertCombatMatchesSingleFinish(t, state, expected)
		})
	}
}
