package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestDebugShardUseRejectsStaleStateAndQueuesSelectedShardWithoutMutation(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Lycaeum).
		WithFloor(2).
		WithPlayerAt(15, 9).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.QuestItems.Set(references.ShardOfFalsehood, 1)
	state.PartyState.Inventory.QuestItems.Set(references.ShardOfHatred, 1)
	state.ShadowlordState.Summon(references.Faulinei)
	scene := &GameScene{gameState: state, gameReferences: state.GameReferences}
	review := newShardUseReviewResult(state, references.ShardOfHatred)
	request := shardUseRequest(review)
	request.Arguments["expectedX"] = json.RawMessage(`14`)

	stale := scene.ExecuteDebugCommand(request)
	assert.False(t, stale.Success)
	assert.Equal(t, "stale_shard_location", stale.ErrorCode)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.ShardOfHatred))

	request = shardUseRequest(review)
	request.Arguments["expectedFlame"] = json.RawMessage(`"none"`)
	staleFlame := scene.ExecuteDebugCommand(request)
	assert.Equal(t, "stale_shard_flame", staleFlame.ErrorCode)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())

	request = shardUseRequest(review)
	request.Arguments["expectedShadowlord"] = json.RawMessage(`"astaroth"`)
	staleShadowlord := scene.ExecuteDebugCommand(request)
	assert.Equal(t, "stale_shadowlord_state", staleShadowlord.ErrorCode)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())

	request = shardUseRequest(review)
	queued := scene.ExecuteDebugCommand(request)
	require.True(t, queued.Success, queued.ErrorMessage)
	assert.Equal(t, debugging.ExecutionWorkflow, queued.EffectiveExecutionMode)
	assert.True(t, state.EventQueueManager.HasWorkflow("shard_workflow"))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.ShardOfHatred), "queueing must not consume the shard")
	var telemetry shardUseQueuedResult
	require.NoError(t, json.Unmarshal(queued.Data, &telemetry))
	assert.True(t, telemetry.Queued)
	assert.Equal(t, "shard_hatred", telemetry.Review.Shard)
	assert.Equal(t, "truth", telemetry.Review.Flame)
	assert.Equal(t, "no_effect", telemetry.Review.ExpectedOutcome)
}

func TestDebugShardUseRejectsStaleSelectedOwnership(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).WithPlayerAt(100, 100).WithSystemCallbacks().Build()
	state.PartyState.Inventory.QuestItems.Set(references.ShardOfCowardice, 1)
	scene := &GameScene{gameState: state, gameReferences: state.GameReferences}
	review := newShardUseReviewResult(state, references.ShardOfCowardice)
	state.PartyState.Inventory.QuestItems.Set(references.ShardOfCowardice, 0)

	result := scene.ExecuteDebugCommand(shardUseRequest(review))
	assert.False(t, result.Success)
	assert.Equal(t, "stale_shard_ownership", result.ErrorCode)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
}

func TestDebugUsableInventoryPresetReturnsExactSemanticChangesAndRejectsStaleReview(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).WithPlayerAt(100, 100).WithSystemCallbacks().Build()
	scene := &GameScene{gameState: state}
	review, err := state.ReviewUsableInventoryPreset()
	require.NoError(t, err)
	request := debugging.Request{CommandID: debugging.CommandQuestInventoryPresetUsable, Arguments: map[string]json.RawMessage{
		"expectedFingerprint": mustDebugJSON(review.Fingerprint),
	}}
	state.PartyState.Inventory.QuestItems.Set(references.ShardOfFalsehood, 1)

	stale := scene.ExecuteDebugCommand(request)
	assert.False(t, stale.Success)
	assert.Equal(t, "stale_inventory", stale.ErrorCode)

	review, err = state.ReviewUsableInventoryPreset()
	require.NoError(t, err)
	request.Arguments["expectedFingerprint"] = mustDebugJSON(review.Fingerprint)
	result := scene.ExecuteDebugCommand(request)
	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, debugging.ExecutionAlteration, result.EffectiveExecutionMode)
	assert.NotEmpty(t, result.Changes)
	assert.Contains(t, result.Changes[0].Path, "inventory.")
	var telemetry usableInventoryPresetTelemetry
	require.NoError(t, json.Unmarshal(result.Data, &telemetry))
	assert.NotEmpty(t, telemetry.ReviewFingerprint)
	assert.NotEmpty(t, telemetry.Changes)
	assert.Contains(t, []string{"quest_items", "moonstones", "potions", "scrolls"}, telemetry.Changes[0].Category)
}

func TestDebugRitualStateMutationsAreBlockedDuringGameplayWorkflow(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
	scene := &GameScene{gameState: state}
	state.EventQueueManager.Workflow("active_gameplay").
		Priority(event_queue.PriorityNormal).
		Sequential().First(func() {}).
		Start()

	for _, commandID := range []string{
		debugging.CommandQuestItemEnsure,
		debugging.CommandQuestItemRemove,
	} {
		result := scene.ExecuteDebugCommand(debugging.Request{CommandID: commandID})

		assert.False(t, result.Success)
		assert.Equal(t, "gameplay_workflow_active", result.ErrorCode)
	}
}

func shardUseRequest(review shardUseReviewResult) debugging.Request {
	return debugging.Request{CommandID: debugging.CommandQuestShardUse, Arguments: map[string]json.RawMessage{
		"shard": mustDebugJSON(review.Shard), "expectedQuantity": mustDebugJSON(review.Quantity),
		"expectedLocationId": mustDebugJSON(review.LocationID), "expectedX": mustDebugJSON(review.X),
		"expectedY": mustDebugJSON(review.Y), "expectedFloor": mustDebugJSON(review.Floor),
		"expectedFlame": mustDebugJSON(review.Flame), "expectedShadowlordSummoned": mustDebugJSON(review.ShadowlordSummoned),
		"expectedShadowlord": mustDebugJSON(review.Shadowlord), "expectedShadowlordAlive": mustDebugJSON(review.ShadowlordAlive),
	}}
}
