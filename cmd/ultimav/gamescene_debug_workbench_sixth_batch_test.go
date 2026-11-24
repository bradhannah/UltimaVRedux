package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestDebugKillPartyReturnsExactChangesAndSupportsNoOp(t *testing.T) {
	scene := newDeathDebugTestScene(t)
	review := scene.gameState.ReviewActivePartyWipe()

	result := scene.ExecuteDebugCommand(deathDebugRequest(debugging.CommandDeathKillParty, review.Fingerprint))

	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, debugging.ExecutionSynchronized, result.EffectiveExecutionMode)
	assert.False(t, scene.gameState.EventQueueManager.HasActiveWorkflows())
	var data activePartyWipeDebugResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	require.Len(t, data.Changes, len(review.Members))
	assert.False(t, data.Queued)
	assert.Len(t, result.Changes, len(review.Members)*2)
	assert.Equal(t, "party.roster.0.currentHp", result.Changes[0].Path)
	assert.Equal(t, "party.roster.0.status", result.Changes[1].Path)
	for index, change := range data.Changes {
		assert.Equal(t, review.Members[index].Slot, change.Before.Slot)
		assert.Zero(t, change.After.HP)
		assert.Equal(t, "Dead", change.After.Status)
	}

	noOpReview := scene.gameState.ReviewActivePartyWipe()
	noOp := scene.ExecuteDebugCommand(deathDebugRequest(debugging.CommandDeathKillParty, noOpReview.Fingerprint))
	require.True(t, noOp.Success, noOp.ErrorMessage)
	require.NoError(t, json.Unmarshal(noOp.Data, &data))
	assert.Empty(t, data.Changes)
	assert.Empty(t, noOp.Changes)
}

func TestDebugDeathSequenceWipesBeforeQueueingProductionWorkflow(t *testing.T) {
	scene := newDeathDebugTestScene(t)
	review := scene.gameState.ReviewActivePartyWipe()

	result := scene.ExecuteDebugCommand(deathDebugRequest(debugging.CommandDeathTestSequence, review.Fingerprint))

	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, debugging.ExecutionWorkflow, result.EffectiveExecutionMode)
	assert.True(t, scene.gameState.PartyState.IsAllPartyDead())
	assert.True(t, scene.gameState.EventQueueManager.HasWorkflow(deathResurrectionWorkflowName))
	var data activePartyWipeDebugResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.True(t, data.Queued)
	assert.Equal(t, deathResurrectionWorkflowName, data.Workflow)
	assert.Len(t, data.Changes, len(review.Members))
}

func TestDebugDeathCommandsRejectStaleAndWorkflowStartupFailuresWithoutPartialWipe(t *testing.T) {
	t.Run("stale review", func(t *testing.T) {
		scene := newDeathDebugTestScene(t)
		review := scene.gameState.ReviewActivePartyWipe()
		character := scene.gameState.PartyState.GetCharacter(review.Members[0].Slot)
		character.SetHp(character.GetCurrentHp() - 1)
		before := character.GetCurrentHp()

		result := scene.ExecuteDebugCommand(deathDebugRequest(debugging.CommandDeathKillParty, review.Fingerprint))

		assert.Equal(t, "stale_party_wipe", result.ErrorCode)
		assert.Equal(t, before, character.GetCurrentHp())
		assert.Equal(t, party_state.Good, character.GetStatus())
	})

	t.Run("scene resources", func(t *testing.T) {
		scene := newDeathDebugTestScene(t)
		scene.spriteSheet = nil
		assertDeathSequenceRejectedWithoutWipe(t, scene, "scene_resources_unavailable")
	})

	t.Run("dialog", func(t *testing.T) {
		scene := newDeathDebugTestScene(t)
		scene.dialogStack.PushModalDialog(&blockingDebugWorkflowDialog{})
		assertDeathSequenceRejectedWithoutWipe(t, scene, "dialog_active")
	})

	t.Run("combat", func(t *testing.T) {
		scene := newDeathDebugTestScene(t)
		scene.gameState.MapState.PlayerLocation.Location = references.CombatLocation
		assertDeathSequenceRejectedWithoutWipe(t, scene, "combat_active")
	})

	t.Run("scripted scene", func(t *testing.T) {
		scene := newDeathDebugTestScene(t)
		scene.miniMapScene = minimap.NewMiniMapScene()
		assertDeathSequenceRejectedWithoutWipe(t, scene, "scripted_scene_active")
	})

	t.Run("workflow", func(t *testing.T) {
		scene := newDeathDebugTestScene(t)
		scene.gameState.EventQueueManager.Workflow("other").Priority(event_queue.PriorityNormal).Sequential().First(func() {}).Finally(func() {}).Start()
		assertDeathSequenceRejectedWithoutWipe(t, scene, "gameplay_workflow_active")
	})
}

func assertDeathSequenceRejectedWithoutWipe(t *testing.T, scene *GameScene, errorCode string) {
	t.Helper()
	review := scene.gameState.ReviewActivePartyWipe()
	result := scene.ExecuteDebugCommand(deathDebugRequest(debugging.CommandDeathTestSequence, review.Fingerprint))
	assert.Equal(t, errorCode, result.ErrorCode)
	assert.False(t, scene.gameState.PartyState.IsAllPartyDead())
	assert.False(t, scene.gameState.EventQueueManager.HasWorkflow(deathResurrectionWorkflowName))
	for _, member := range review.Members {
		character := scene.gameState.PartyState.GetCharacter(member.Slot)
		assert.Equal(t, member.HP, character.GetCurrentHp())
		assert.Equal(t, member.Status, character.GetStatus())
	}
}

func deathDebugRequest(commandID, fingerprint string) debugging.Request {
	return debugging.Request{CommandID: commandID, Arguments: map[string]json.RawMessage{
		"expectedFingerprint": json.RawMessage(`"` + fingerprint + `"`),
	}}
}

func newDeathDebugTestScene(t *testing.T) *GameScene {
	t.Helper()
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithFloor(0).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{
		gameState: state, output: text.NewOutput(nil, 1, 10, 80), spriteSheet: &sprites.SpriteSheet{},
	}
	return scene
}
