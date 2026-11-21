package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/mainscreen"
)

func TestCombatPossessionReportTypedTextAndNoHiddenControllerDisclosure(t *testing.T) {
	scene, _ := newCombatUseTestScene(t)
	state := scene.gameState
	actor := state.CombatScene.GetCurrentTurnIdentity()
	name := state.PartyState.GetCharacter(actor.EntityIndex).GetNameAsString()
	scene.combatHUD = newCombatHUD()
	scene.combatHUD.eventOutput = text.NewOutput(nil, 1, 20, 120)
	for _, test := range []struct {
		kind   game_state.CombatEventKind
		turns  int
		reason game_state.CombatPossessionReleaseReason
		suffix string
	}{
		{game_state.CombatEventPossessionResistance, 3, game_state.CombatPossessionReleaseNatural, " struggles against the enchantment."},
		{game_state.CombatEventPossessionResistance, 7, game_state.CombatPossessionReleaseNatural, "'s resistance grows stronger."},
		{game_state.CombatEventPossessionResistance, 10, game_state.CombatPossessionReleaseNatural, " is close to breaking free!"},
		{game_state.CombatEventPossessionRelease, 12, game_state.CombatPossessionReleaseNatural, " breaks free of the enchantment!"},
		{game_state.CombatEventPossessionRelease, 8, game_state.CombatPossessionReleaseControllerDefeated, " regains control as the possessor is defeated!"},
		{game_state.CombatEventPossessionRelease, 8, game_state.CombatPossessionReleaseControllerEscaped, " regains control as the possessor flees!"},
		{game_state.CombatEventPossessionRelease, 8, game_state.CombatPossessionReleaseControllerReplaced, " regains control as the transformed possessor's hold breaks!"},
	} {
		event := game_state.CombatEvent{Kind: test.kind, Target: game_state.CombatFeedbackEntity{IsPartyMember: true, Index: actor.EntityIndex}, PossessionEpisodeID: 17, PossessionTurns: test.turns, PossessionReleaseReason: test.reason}
		before := snapshotBlockedCombatState(t, state)
		assert.Equal(t, name+test.suffix, scene.combatEventPlainText(event))
		scene.combatHUD.eventOutput.Reset()
		scene.appendCombatEventSentence(event)
		assert.Contains(t, scene.combatHUD.eventOutput.GetOutputStr(false), name+test.suffix)
		assert.Equal(t, fmt.Sprintf("%d/12 own scheduled turns completed", test.turns), scene.combatEventDetailText(event))
		assert.NotContains(t, scene.combatEventDetailText(event), "17", "episode IDs are internal, not controller clues")
		assert.Equal(t, before, snapshotBlockedCombatState(t, state))
	}
	cloneA := game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: actor.EntityIndex, Slot: 1}
	cloneB := game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: actor.EntityIndex, Slot: 2}
	assert.NotEqual(t, scene.combatEntityEventName(cloneA), scene.combatEntityEventName(cloneB))
}

func TestCombatPossessionProgressAndHelpAreDetached(t *testing.T) {
	output := text.NewOutput(nil, 1, 8, 100)
	mainscreen.AppendCombatConditions(output.AppendLine(), mainscreen.CombatConditions{Possessed: true, TimedPossession: true, PossessionTurns: 8}).Done()
	assert.Equal(t, "Possessed - 8/12 turns", output.GetOutputStr(false))
	output.Reset()
	mainscreen.AppendCombatConditions(output.AppendLine(), mainscreen.CombatConditions{Possessed: true}).Done()
	assert.Equal(t, "Possessed", output.GetOutputStr(false), "raw/Chaos control never advertises timed resistance")
	scene, _ := newCombatUseTestScene(t)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	before := snapshotBlockedCombatState(t, scene.gameState)
	view := scene.currentHelpStatus()
	found := false
	for _, entry := range view.Effects {
		if entry.Name == "Possession" {
			found = true
			for _, fragment := range []string{"breaks free in time", "An Xen Ex", "possessor is slain", "turned into a rat"} {
				assert.Contains(t, entry.Description, fragment)
			}
		}
	}
	assert.True(t, found)
	assert.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState))
}

func TestCombatPossessionReleaseSuppressesOnlyExactAllegianceNotice(t *testing.T) {
	scene, mock := newCombatUseTestScene(t)
	state := scene.gameState
	actor := state.CombatScene.GetCurrentTurnIdentity()
	position := state.CombatScene.GetCombatantPosition(actor)
	setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
	state.CombatScene.PartyPossessed[actor.EntityIndex] = true
	scene.updateCombatAllegiance()
	mock.Reset()
	state.CombatScene.PartyPossessed[actor.EntityIndex] = false
	scene.recordCombatEvent(game_state.CombatEvent{Kind: game_state.CombatEventPossessionRelease,
		Target:              game_state.CombatFeedbackEntity{IsPartyMember: true, Index: actor.EntityIndex, Position: position},
		PossessionEpisodeID: 1, PossessionTurns: 12})
	scene.updateCombatAllegiance()
	assert.Empty(t, mock.Messages, "typed release already describes this exact transition")
	// First admission synchronizes the encounter and records BattleBegun before
	// the single release; it is not a duplicate recovery/allegiance announcement.
	require.Equal(t, 2, scene.combatHUD.historyCount)
	assert.Equal(t, game_state.CombatEventBattleBegun, scene.combatHUD.history[0].Kind)
	assert.Equal(t, game_state.CombatEventPossessionRelease, scene.combatHUD.history[1].Kind)
	assert.Equal(t, uint64(1), scene.combatHUD.history[1].PossessionEpisodeID)
}

func TestCombatPossessionRealEpisodeProjectionAndEffectGuard(t *testing.T) {
	scene, _ := newCombatConcealmentTestScene(t)
	state := scene.gameState
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	to := references.Position{X: 1, Y: 8}
	state.CombatScene.SetPartyMemberPosition(0, to)
	setCombatConcealmentTerrain(state, to, indexes.BrickFloor)
	state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	state.PartyState.GetCharacter(0).Intelligence = 1
	state.CombatScene.PartySleeping[0] = false
	index, monster := signatureMonster(t, scene, references.MonsterShadowlord, references.Position{X: 8, Y: 1})
	monster.SetInvisible(true)
	state.SystemCallbacks.Combat = game_state.NewCombatCallbacks(scene.recordCombatEvent, scene.recordCombatConclusion)
	step := nextSignatureMonsterStep(t, scene, index)
	state.SetRandomSeed(possessionPresentationSeed(t, monster.EnemyRef().Intelligence))
	_, err := state.ResolveScheduledCombatStep(step.Token)
	require.NoError(t, err)
	actor := game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	id, turns, active := state.CombatScene.CombatPossessionProgress(actor)
	require.True(t, active)
	require.NotZero(t, id)
	assert.Zero(t, turns)
	conditions := combatConditions(state.CombatScene, actor)
	assert.True(t, conditions.TimedPossession)
	assert.Zero(t, conditions.PossessionTurns)
	projection, ok := state.ProjectCombatActor(actor, 0)
	require.True(t, ok)
	assert.Equal(t, id, projection.PossessionEpisodeID)
	require.Len(t, scene.combatSignatureEffects, 1)
	effect := scene.combatSignatureEffects[0]
	assert.Equal(t, id, effect.PossessionEpisodeID)
	assert.True(t, scene.combatSignatureEffectCurrent(effect))
	stale := *effect
	stale.PossessionEpisodeID++
	assert.False(t, scene.combatSignatureEffectCurrent(&stale), "raw possessed=true cannot validate a different episode")
	state.PartyState.GetCharacter(1).Intelligence = 255
	require.True(t, state.CastCombatAnXenEx(1, actor))
	assert.False(t, scene.combatSignatureEffectCurrent(effect))
	assert.False(t, combatConditions(state.CombatScene, actor).TimedPossession)
}
