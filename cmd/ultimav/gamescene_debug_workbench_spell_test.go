package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func newSpellWorkbenchTestState(t *testing.T) *game_state.GameState {
	t.Helper()
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	state.SystemCallbacks.UI.ShowActionFeedback = func(string, bool) {}

	return state
}

func TestSpellInventoryStatusIncludesAllCastableSpellsAndLiveEffect(t *testing.T) {
	state := newSpellWorkbenchTestState(t)
	state.ActivateLightEffect(240)
	state.ActivateProtectionEffect(20)

	result, handled := executeSpellDebugCommand(debugging.Request{
		SessionID: "test", ID: "status", CommandID: debugging.CommandInventorySpellStatus,
	}, state)
	require.True(t, handled)
	require.True(t, result.Success)
	var status debugging.SpellInventoryStatusData
	require.NoError(t, json.Unmarshal(result.Data, &status))
	require.Len(t, status.Spells, references.TotalSpellCount)
	assert.Equal(t, "In Lor", status.Spells[0].Name)
	assert.Equal(t, "An Tym", status.Spells[47].Name)
	assert.True(t, status.MagicLight.Active)
	assert.Equal(t, 240, status.MagicLight.MinutesRemaining)
	assert.True(t, status.ActiveSpell.Active)
	assert.Equal(t, "P", status.ActiveSpell.EffectType)
}

func TestSpellInventorySetIsStaleSafeAndPresetExcludesNox(t *testing.T) {
	state := newSpellWorkbenchTestState(t)
	state.PartyState.Inventory.Spells.Set(references.InLor, 0)
	stale, _ := executeSpellDebugCommand(debugging.Request{
		SessionID: "test", ID: "stale", CommandID: debugging.CommandInventorySpellSet,
		Arguments: map[string]json.RawMessage{
			"spell": json.RawMessage(`"0"`), "targetQuantity": json.RawMessage(`7`),
			"expectedQuantity": json.RawMessage(`1`),
		},
	}, state)
	assert.False(t, stale.Success)
	assert.Equal(t, "stale_spell_inventory", stale.ErrorCode)

	updated, _ := executeSpellDebugCommand(debugging.Request{
		SessionID: "test", ID: "set", CommandID: debugging.CommandInventorySpellSet,
		Arguments: map[string]json.RawMessage{
			"spell": json.RawMessage(`"0"`), "targetQuantity": json.RawMessage(`7`),
			"expectedQuantity": json.RawMessage(`0`),
		},
	}, state)
	require.True(t, updated.Success)
	require.Len(t, updated.Changes, 1)
	assert.Equal(t, "inventory.spells.0.quantity", updated.Changes[0].Path)

	status := newSpellInventoryStatusResult(state)
	state.PartyState.Inventory.Spells.Set(references.Nox, 3)
	preset, _ := executeSpellDebugCommand(debugging.Request{
		SessionID: "test", ID: "preset", CommandID: debugging.CommandInventorySpellPresetMax,
		Arguments: map[string]json.RawMessage{"expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint)},
	}, state)
	require.True(t, preset.Success)
	assert.Equal(t, uint16(3), state.PartyState.Inventory.Spells.Get(references.Nox))
	for spell := references.InLor; spell <= references.AnTym; spell++ {
		assert.Equal(t, uint16(99), state.PartyState.Inventory.Spells.Get(spell))
	}
}

func TestSpellEffectDurationCommandUsesReviewedState(t *testing.T) {
	state := newSpellWorkbenchTestState(t)
	state.ActivateProtectionEffect(20)
	result, handled := executeSpellDebugCommand(debugging.Request{
		SessionID: "test", ID: "duration", CommandID: debugging.CommandSpellEffectSetRemaining,
		Arguments: map[string]json.RawMessage{
			"effectKind": json.RawMessage(`"active_spell"`), "expectedEffect": json.RawMessage(`"P"`),
			"expectedTurns": json.RawMessage(`20`), "expectedLightMinutes": json.RawMessage(`0`),
			"remaining": json.RawMessage(`1`),
		},
	}, state)
	require.True(t, handled)
	require.True(t, result.Success)
	assert.Equal(t, 1, state.ActiveDurationEffect.TurnsRemaining)
}

func TestSpellEffectActivationIsExplicitAndIdempotent(t *testing.T) {
	state := newSpellWorkbenchTestState(t)
	request := debugging.Request{
		SessionID: "test", ID: "activate", CommandID: debugging.CommandSpellEffectActivate,
		Arguments: map[string]json.RawMessage{
			"effect": json.RawMessage(`"light"`), "expectedEffect": json.RawMessage(`""`),
			"expectedTurns": json.RawMessage(`0`), "expectedLightMinutes": json.RawMessage(`0`),
		},
	}
	first, handled := executeSpellDebugCommand(request, state)
	require.True(t, handled)
	require.True(t, first.Success)
	assert.Equal(t, game_state.DurationEffectNone, state.ActiveDurationEffect.EffectType)
	assert.Equal(t, 240, state.MagicLightMinutesRemaining)

	state.MagicLightMinutesRemaining = 5
	request.Arguments["expectedLightMinutes"] = json.RawMessage(`5`)
	second, _ := executeSpellDebugCommand(request, state)
	require.True(t, second.Success)
	assert.Equal(t, game_state.DurationEffectNone, state.ActiveDurationEffect.EffectType)
	assert.Equal(t, 240, state.MagicLightMinutesRemaining)
}

func TestSpellLightDurationCommandIsIndependentAndStaleSafe(t *testing.T) {
	state := newSpellWorkbenchTestState(t)
	state.ActivateProtectionEffect(20)
	state.ActivateLightEffect(30)

	stale, _ := executeSpellDebugCommand(debugging.Request{
		SessionID: "test", ID: "stale-light", CommandID: debugging.CommandSpellEffectSetRemaining,
		Arguments: map[string]json.RawMessage{
			"effectKind": json.RawMessage(`"magic_light"`), "expectedEffect": json.RawMessage(`"P"`),
			"expectedTurns": json.RawMessage(`20`), "expectedLightMinutes": json.RawMessage(`29`),
			"remaining": json.RawMessage(`5`),
		},
	}, state)
	assert.False(t, stale.Success)
	assert.Equal(t, "stale_spell_effects", stale.ErrorCode)
	assert.Equal(t, 30, state.MagicLightMinutesRemaining)
	assert.Equal(t, 20, state.ActiveDurationEffect.TurnsRemaining)

	updated, _ := executeSpellDebugCommand(debugging.Request{
		SessionID: "test", ID: "set-light", CommandID: debugging.CommandSpellEffectSetRemaining,
		Arguments: map[string]json.RawMessage{
			"effectKind": json.RawMessage(`"magic_light"`), "expectedEffect": json.RawMessage(`"P"`),
			"expectedTurns": json.RawMessage(`20`), "expectedLightMinutes": json.RawMessage(`30`),
			"remaining": json.RawMessage(`5`),
		},
	}, state)
	require.True(t, updated.Success)
	assert.Equal(t, 5, state.MagicLightMinutesRemaining)
	assert.Equal(t, game_state.DurationEffectProtection, state.ActiveDurationEffect.EffectType)
	require.Len(t, updated.Changes, 1)
	assert.Equal(t, "effects.magicLight.minutesRemaining", updated.Changes[0].Path)
}

func TestSpellEffectActivationRejectsStaleIndependentState(t *testing.T) {
	state := newSpellWorkbenchTestState(t)
	state.ActivateProtectionEffect(20)
	result, _ := executeSpellDebugCommand(debugging.Request{
		SessionID: "test", ID: "stale-activate", CommandID: debugging.CommandSpellEffectActivate,
		Arguments: map[string]json.RawMessage{
			"effect": json.RawMessage(`"time_stop"`), "expectedEffect": json.RawMessage(`"P"`),
			"expectedTurns": json.RawMessage(`19`), "expectedLightMinutes": json.RawMessage(`0`),
		},
	}, state)
	assert.False(t, result.Success)
	assert.Equal(t, "stale_spell_effects", result.ErrorCode)
	assert.Equal(t, game_state.DurationEffectProtection, state.ActiveDurationEffect.EffectType)
}

func TestSpellEffectDurationRejectsStaleReview(t *testing.T) {
	state := newSpellWorkbenchTestState(t)
	state.ActivateProtectionEffect(20)
	result, _ := executeSpellDebugCommand(debugging.Request{
		SessionID: "test", ID: "stale-duration", CommandID: debugging.CommandSpellEffectSetRemaining,
		Arguments: map[string]json.RawMessage{
			"effectKind": json.RawMessage(`"active_spell"`), "expectedEffect": json.RawMessage(`"P"`),
			"expectedTurns": json.RawMessage(`19`), "expectedLightMinutes": json.RawMessage(`0`),
			"remaining": json.RawMessage(`1`),
		},
	}, state)
	assert.False(t, result.Success)
	assert.Equal(t, "stale_spell_effects", result.ErrorCode)
	assert.Equal(t, 20, state.ActiveDurationEffect.TurnsRemaining)
}
