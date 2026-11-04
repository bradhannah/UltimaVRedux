package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
)

func TestDebugCommandRegistryExactlyCoversCatalog(t *testing.T) {
	catalog := debugging.NewInitialCatalog()
	registry, err := newDebugCommandRegistry(catalog)
	require.NoError(t, err)

	definitions := catalog.Definitions()
	require.Len(t, registry.registrations, len(definitions))
	for _, definition := range definitions {
		registration, ok := registry.lookup(definition.ID)
		assert.True(t, ok, "catalog command %q has no application registration", definition.ID)
		if !ok {
			continue
		}
		assert.NotEmpty(t, registration.owner, definition.ID)
		assert.NotNil(t, registration.handler, definition.ID)
	}
}

func TestDebugCommandRegistryRejectsDuplicateOwner(t *testing.T) {
	registry := newEmptyDebugCommandRegistry()
	handler := func(debugCommandContext, debugging.Request) debugging.Result { return debugging.Result{} }
	require.NoError(t, registry.register(debugCommandRegistration{
		commandID: debugging.CommandEnvironmentWindStatus,
		owner:     "environment",
		handler:   handler,
	}))

	err := registry.register(debugCommandRegistration{
		commandID: debugging.CommandEnvironmentWindStatus,
		owner:     "shadowing-owner",
		handler:   handler,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), debugging.CommandEnvironmentWindStatus)
}

func TestDebugCommandRegistryCoLocatesGuards(t *testing.T) {
	catalog := debugging.NewInitialCatalog()
	registry, err := newDebugCommandRegistry(catalog)
	require.NoError(t, err)

	guarded := []string{
		debugging.CommandQuestItemEnsure,
		debugging.CommandQuestItemRemove,
		debugging.CommandInventoryEquipmentClear,
		debugging.CommandPartyEquipmentLoadoutUpdate,
		debugging.CommandCombatMonsterSleepSet,
		debugging.CommandCombatExitDebug,
		debugging.CommandCombatMonsterSpawn,
		debugging.CommandCombatDungeonRoomClear,
	}
	for _, commandID := range guarded {
		registration, ok := registry.lookup(commandID)
		require.True(t, ok, commandID)
		assert.NotNil(t, registration.guard, commandID)
	}
}

func TestDebugCommandRegistryLookupSelectsOneOwnerWithoutRoutingOrder(t *testing.T) {
	registry, err := newDebugCommandRegistry(debugging.NewInitialCatalog())
	require.NoError(t, err)

	registration, ok := registry.lookup(debugging.CommandCombatStatus)
	require.True(t, ok)
	assert.Equal(t, "active combat", registration.owner)
	assert.Equal(t, debugCommandScene, registration.scope)
}
