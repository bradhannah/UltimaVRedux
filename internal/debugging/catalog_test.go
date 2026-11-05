package debugging

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestInitialCatalogPublishesSupportedCommands(t *testing.T) {
	catalog := NewInitialCatalog()
	seen := make(map[string]bool)
	for _, definition := range catalog.Definitions() {
		assert.False(t, seen[definition.ID], definition.ID)
		seen[definition.ID] = true
	}

	for _, id := range []string{
		CommandEnvironmentWindStatus, CommandLoggingStatus, CommandShrineStatus, CommandShrineProgressUpdate,
		CommandPartyCharacterInspect, CommandPartyCharacterUpdate, CommandCalendarStatus, CommandSearchStatus,
		CommandQuestItemStatus, CommandWorldPositionStatus, CommandWorldPositionSet, CommandMovementLandmarkTravel,
		CommandMoongateStatus, CommandCombatEncounterOptions, CommandCombatEncounterStart, CommandInventoryLockerStatus,
		CommandInventorySpellStatus, CommandCombatStatus, CommandCombatPresentationSpeedSet, CommandDebugOptionsStatus,
		CommandMovementFreeMoveSet, CommandTestSetupStatus, CommandTestHoleUp, CommandDeathKillParty,
		CommandDeathTestSequence, CommandTestTelescope, CommandSaveSnapshotList, CommandSaveSnapshotLoad,
		CommandMerchantTesterCatalog, CommandMerchantTesterPreviewOpen, CommandMerchantTesterLiveOpen,
		CommandCodexQuestStateSetAll, CommandPartyEquipmentLoadoutUpdate,
	} {
		assert.True(t, seen[id], id)
	}

	for _, id := range []string{
		"scenario.status", "scenario.definition.list", "scenario.definition.get", "scenario.definition.validate",
		"scenario.definition.create", "scenario.definition.update", "scenario.definition.delete",
		"scenario.definition.duplicate", "scenario.run.start", "scenario.run.status", "scenario.run.cancel",
		"combat.map-catalog.all", "shrine.reference.inspect",
		"display.status", "display.mode.set", "display.resolution.set", "display.fullscreen.set",
		"movement.teleport", "movement.enter-location", "movement.floor.set",
		"redux.modifications.status", "redux.combat.repeated-monster-division.set",
		"redux.dungeons.persist-hidden-discoveries.set", "redux.dungeons.subdued-unlit-visibility.set",
		"redux.presentation.smooth-surface-movement.set", "redux.navigation.flexible_sextant_use.set",
		"redux.saves.confirm-quick-save.set", "redux.combat.health-bars.set", "redux.presentation.action-popups.set",
	} {
		assert.False(t, seen[id], id)
	}

	landmark, ok := catalog.Get(CommandMovementLandmarkTravel)
	require.True(t, ok)
	assert.Contains(t, landmark.Parameters[0].Choices, Choice{
		Value: "lord_british_secret_room", Label: "Lord British's Secret Room",
	})
	speed, ok := catalog.Get(CommandCombatPresentationSpeedSet)
	require.True(t, ok)
	assert.Equal(t, categoryCombat, speed.Category)
	assert.Equal(t, []ExecutionMode{ExecutionPresentation}, speed.ExecutionModes)
	for _, id := range []string{CommandDeathKillParty, CommandDeathTestSequence, CommandSaveSnapshotLoad} {
		definition, found := catalog.Get(id)
		require.True(t, found)
		assert.True(t, definition.Destructive, id)
	}
}

func TestMerchantTesterCatalogDefinitionsAndTypedParameters(t *testing.T) {
	catalog := NewInitialCatalog()

	status, ok := catalog.Get(CommandMerchantTesterCatalog)
	require.True(t, ok)
	assert.Empty(t, status.Parameters)
	assert.Equal(t, []ExecutionMode{ExecutionQuery}, status.ExecutionModes)

	preview, ok := catalog.Get(CommandMerchantTesterPreviewOpen)
	require.True(t, ok)
	require.Len(t, preview.Parameters, 1)
	assert.Equal(t, "merchantId", preview.Parameters[0].ID)
	assert.Equal(t, ParameterString, preview.Parameters[0].Kind)
	assert.True(t, preview.Parameters[0].Required)
	assert.Equal(t, OptionProviderMerchantTesterMerchants, preview.Parameters[0].OptionsProvider)
	assert.Equal(t, []ExecutionMode{ExecutionWorkflow}, preview.ExecutionModes)

	live, ok := catalog.Get(CommandMerchantTesterLiveOpen)
	require.True(t, ok)
	require.Len(t, live.Parameters, 6)
	assert.Equal(t, []string{
		"merchantId", "expectedLocationId", "expectedFloor", "expectedX", "expectedY", "expectedSourceFingerprint",
	}, []string{
		live.Parameters[0].ID, live.Parameters[1].ID, live.Parameters[2].ID,
		live.Parameters[3].ID, live.Parameters[4].ID, live.Parameters[5].ID,
	})
	assert.Equal(t, []ExecutionMode{ExecutionWorkflow, ExecutionAlteration}, live.ExecutionModes)
	assert.True(t, live.Destructive)

	require.NoError(t, catalog.ValidateRequest(Request{
		SessionID: "test", ID: "preview", CommandID: CommandMerchantTesterPreviewOpen,
		Arguments: map[string]json.RawMessage{"merchantId": json.RawMessage(`"guild.paws"`)},
	}))
	require.NoError(t, catalog.ValidateRequest(Request{
		SessionID: "test", ID: "live", CommandID: CommandMerchantTesterLiveOpen,
		Arguments: map[string]json.RawMessage{
			"merchantId":         json.RawMessage(`"unknown.merchant"`),
			"expectedLocationId": json.RawMessage(`2`), "expectedFloor": json.RawMessage(`0`),
			"expectedX": json.RawMessage(`10`), "expectedY": json.RawMessage(`11`),
			"expectedSourceFingerprint": json.RawMessage(`"reviewed"`),
		},
	}), "unknown identities must reach update-thread merchant_unknown handling")
}

func TestWorldNavigatorCatalogDefinitionsAndNormalization(t *testing.T) {
	catalog := NewInitialCatalog()

	world, ok := catalog.Get(CommandWorldPositionSet)
	require.True(t, ok)
	assert.Equal(t, "World", world.Category)
	assert.Contains(t, world.SearchTerms, "navigator")
	require.Len(t, world.Parameters, 12)
	assert.Equal(t, ParameterEnum, world.Parameters[0].Kind)
	assert.Equal(t, []Choice{
		{Value: WorldPlacementExact, Label: "Arbitrary exact position"},
		{Value: WorldPlacementDungeonOutside, Label: "Outside dungeon entrance"},
		{Value: WorldPlacementDungeonInsideStart, Label: "Inside dungeon start"},
		{Value: WorldPlacementDungeonOutsideRoom, Label: "Outside a room"},
		{Value: WorldPlacementDungeonUnderworldExit, Label: "Dungeon Underworld-exit ladder"},
	}, world.Parameters[0].Choices)
	assert.Equal(t, ParameterEnum, world.Parameters[1].Kind)
	assert.Equal(t, []Choice{
		{Value: WorldRouteBritannia, Label: "Britannia"},
		{Value: WorldRouteUnderworld, Label: "Underworld"},
	}, world.Parameters[1].Choices)
	assert.Equal(t, ParameterEnum, world.Parameters[2].Kind)
	require.Len(t, world.Parameters[2].Choices, int(references.Doom)+1)
	assert.Equal(t, Choice{Value: "0", Label: "Britannia / Underworld"}, world.Parameters[2].Choices[0])
	assert.Equal(t, Choice{Value: "40", Label: "Doom"}, world.Parameters[2].Choices[40])
	assert.Equal(t, ParameterEnum, world.Parameters[3].Kind)
	require.Len(t, world.Parameters[3].Choices, references.DungeonFloorCount+1)
	assert.Equal(t, "-1", world.Parameters[3].Choices[0].Value)
	assert.Equal(t, "7", world.Parameters[3].Choices[8].Value)
	assert.Equal(t, []string{"expectedLocationId", "expectedFloor", "expectedX", "expectedY", "expectedSourceFingerprint"}, []string{
		world.Parameters[7].ID,
		world.Parameters[8].ID,
		world.Parameters[9].ID,
		world.Parameters[10].ID,
		world.Parameters[11].ID,
	})
	assert.Equal(t, "room", world.Parameters[4].ID)
	assert.False(t, world.Parameters[4].Required, "only the outside-a-room placement needs a room")
	require.NoError(t, catalog.ValidateRequest(Request{
		SessionID: "test", ID: "world", CommandID: CommandWorldPositionSet,
		Arguments: map[string]json.RawMessage{
			"placement": json.RawMessage(`"exact"`), "route": json.RawMessage(`"underworld"`),
			"location": json.RawMessage(`"40"`), "floor": json.RawMessage(`"7"`),
			"x": json.RawMessage(`7`), "y": json.RawMessage(`0`),
			"expectedLocationId": json.RawMessage(`2`), "expectedFloor": json.RawMessage(`0`),
			"expectedX": json.RawMessage(`10`), "expectedY": json.RawMessage(`11`),
			"expectedSourceFingerprint": json.RawMessage(`"reviewed-world"`),
		},
	}))
	routeRequest, err := catalog.NormalizeRequest(Request{
		SessionID: "test", ID: "world-route", CommandID: CommandWorldPositionSet,
		Arguments: map[string]json.RawMessage{
			"placement": json.RawMessage(`"dungeon_underworld_exit"`), "route": json.RawMessage(`"underworld"`),
			"location":           json.RawMessage(`"33"`),
			"expectedLocationId": json.RawMessage(`2`), "expectedFloor": json.RawMessage(`0`),
			"expectedX": json.RawMessage(`10`), "expectedY": json.RawMessage(`11`),
			"expectedSourceFingerprint": json.RawMessage(`"reviewed-world"`),
		},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `"dungeon_underworld_exit"`, string(routeRequest.Arguments["placement"]))
	assert.JSONEq(t, `0`, string(routeRequest.Arguments["x"]))

}

func TestPartyEquipmentLoadoutCatalogUsesSlotSpecificAtomicChoices(t *testing.T) {
	definition, ok := NewInitialCatalog().Get(CommandPartyEquipmentLoadoutUpdate)
	require.True(t, ok)
	require.Len(t, definition.Parameters, 8)
	assert.Equal(t, OptionProviderActivePartyCharacters, definition.Parameters[0].OptionsProvider)
	assert.Equal(t, "helm", definition.Parameters[1].ID)
	assert.Equal(t, "amulet", definition.Parameters[6].ID)
	assert.Equal(t, inventoryExpectedSnapshotID, definition.Parameters[7].ID)
	assert.Contains(t, definition.Parameters[3].Choices, choice("inventory:30", "Equip Long Sword from inventory"))
	assert.Contains(t, definition.Parameters[3].Choices, choice("manufacture:39", "Manufacture and equip Glass Sword"))
	assert.NotContains(t, definition.Parameters[3].Choices, choice("inventory:27", "Equip Arrows from inventory"))
}

func TestInitialCatalogValidatesSaveSnapshotOperations(t *testing.T) {
	catalog := NewInitialCatalog()
	revision := json.RawMessage(`"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`)

	require.NoError(t, catalog.ValidateRequest(Request{SessionID: "test", ID: "list", CommandID: CommandSaveSnapshotList}))
	require.NoError(t, catalog.ValidateRequest(Request{SessionID: "test", ID: "get", CommandID: CommandSaveSnapshotGet, Arguments: map[string]json.RawMessage{"reference": json.RawMessage(`"rollback:automatic-1"`)}}))
	capture, err := catalog.NormalizeRequest(Request{SessionID: "test", ID: "capture", CommandID: CommandSaveSnapshotCapture, Arguments: map[string]json.RawMessage{
		"id": json.RawMessage(`"before-test"`), "name": json.RawMessage(`"Before test"`),
	}})
	require.NoError(t, err)
	assert.JSONEq(t, `"personal"`, string(capture.Arguments["source"]))
	assert.JSONEq(t, `""`, string(capture.Arguments["description"]))

	load := Request{SessionID: "test", ID: "load", CommandID: CommandSaveSnapshotLoad, Arguments: map[string]json.RawMessage{"reference": json.RawMessage(`"project:before-test"`), "expectedRevision": revision}}
	require.NoError(t, catalog.ValidateRequest(load))
	remove := Request{SessionID: "test", ID: "delete", CommandID: CommandSaveSnapshotDelete, Arguments: map[string]json.RawMessage{"reference": json.RawMessage(`"personal:before-test"`), "expectedRevision": revision}}
	require.NoError(t, catalog.ValidateRequest(remove))
	remove.Arguments["reference"] = json.RawMessage(`"rollback:automatic-1"`)
	require.ErrorIs(t, catalog.ValidateRequest(remove), ErrInvalidRequest)
	load.Arguments["expectedRevision"] = json.RawMessage(`"stale"`)
	require.ErrorIs(t, catalog.ValidateRequest(load), ErrInvalidRequest)
}

func TestInitialCatalogValidatesEighthBatchCombatArguments(t *testing.T) {
	catalog := NewInitialCatalog()
	base := map[string]json.RawMessage{
		"encounterId": json.RawMessage(`"4"`), "monsterId": json.RawMessage(`"7"`),
		"expectedCurrentHp": json.RawMessage(`20`), "expectedSnapshot": json.RawMessage(`"reviewed"`),
	}
	damage := Request{SessionID: "test", ID: "damage", CommandID: CommandCombatMonsterDamageRaw, Arguments: base}
	damage.Arguments["damage"] = json.RawMessage(`999`)
	require.NoError(t, catalog.ValidateRequest(damage))
	damage.Arguments["damage"] = json.RawMessage(`1000`)
	require.ErrorIs(t, catalog.ValidateRequest(damage), ErrInvalidRequest)

	for _, commandID := range []string{CommandCombatMonsterKillAll, CommandCombatMonsterClear} {
		require.NoError(t, catalog.ValidateRequest(Request{
			SessionID: "test", ID: commandID, CommandID: commandID,
			Arguments: map[string]json.RawMessage{"encounterId": json.RawMessage(`"4"`), "expectedSnapshot": json.RawMessage(`"reviewed"`)},
		}))
	}
}

func TestInitialCatalogValidatesSeventhBatchReviewedShrineState(t *testing.T) {
	catalog := NewInitialCatalog()
	request := Request{
		SessionID: "test", ID: "set-all-shrines", CommandID: CommandCodexQuestStateSetAll,
		Arguments: map[string]json.RawMessage{
			"targetState":         json.RawMessage(`"completed"`),
			"expectedFingerprint": json.RawMessage(`"reviewed-shrines"`),
		},
	}

	_, err := catalog.NormalizeRequest(request)
	require.NoError(t, err)

	request.Arguments["targetState"] = json.RawMessage(`"assigned"`)
	_, err = catalog.NormalizeRequest(request)
	assert.Error(t, err)

	request.Arguments["targetState"] = json.RawMessage(`"not_assigned"`)
	delete(request.Arguments, "expectedFingerprint")
	_, err = catalog.NormalizeRequest(request)
	assert.Error(t, err)
}

func TestInitialCatalogValidatesSixthBatchReviewedFingerprint(t *testing.T) {
	catalog := NewInitialCatalog()
	for _, commandID := range []string{CommandDeathKillParty, CommandDeathTestSequence} {
		request := Request{SessionID: "test", ID: commandID, CommandID: commandID, Arguments: map[string]json.RawMessage{
			"expectedFingerprint": json.RawMessage(`"reviewed-party"`),
		}}
		_, err := catalog.NormalizeRequest(request)
		require.NoError(t, err)

		delete(request.Arguments, "expectedFingerprint")
		_, err = catalog.NormalizeRequest(request)
		assert.Error(t, err)
	}
}

func TestInitialCatalogMatchesBrowserContractFixture(t *testing.T) {
	want, err := os.ReadFile("webui/client/src/initial_catalog.fixture.json")
	require.NoError(t, err)
	got, err := json.Marshal(NewInitialCatalog().Definitions())
	require.NoError(t, err)

	assert.JSONEq(t, string(want), string(got))
}

func TestInitialCatalogValidatesIndependentSpellEffectReviewedState(t *testing.T) {
	catalog := NewInitialCatalog()
	setRequest := Request{SessionID: "test", ID: "set-effect", CommandID: CommandSpellEffectSetRemaining, Arguments: map[string]json.RawMessage{
		"effectKind": json.RawMessage(`"magic_light"`), "expectedEffect": json.RawMessage(`"P"`),
		"expectedTurns": json.RawMessage(`20`), "expectedLightMinutes": json.RawMessage(`30`), "remaining": json.RawMessage(`5`),
	}}
	require.NoError(t, catalog.ValidateRequest(setRequest))
	delete(setRequest.Arguments, "expectedLightMinutes")
	require.Error(t, catalog.ValidateRequest(setRequest))

	activateRequest := Request{SessionID: "test", ID: "activate", CommandID: CommandSpellEffectActivate, Arguments: map[string]json.RawMessage{
		"effect": json.RawMessage(`"light"`), "expectedEffect": json.RawMessage(`""`),
		"expectedTurns": json.RawMessage(`0`), "expectedLightMinutes": json.RawMessage(`0`),
	}}
	require.NoError(t, catalog.ValidateRequest(activateRequest))
}

func TestInitialCatalogValidatesThirdBatchParameters(t *testing.T) {
	catalog := NewInitialCatalog()
	holeUp := Request{SessionID: "test", ID: "hole-up", CommandID: CommandTestHoleUp, Arguments: map[string]json.RawMessage{
		"hours": json.RawMessage(`1`), "expectedPartyFingerprint": json.RawMessage(`"party"`),
	}}
	normalized, err := catalog.NormalizeRequest(holeUp)
	require.NoError(t, err)
	assert.JSONEq(t, `false`, string(normalized.Arguments["forceWanderer"]))
	assert.JSONEq(t, `false`, string(normalized.Arguments["forceAmbush"]))
	holeUp.Arguments["hours"] = json.RawMessage(`25`)
	require.Error(t, catalog.ValidateRequest(holeUp))

	search := Request{SessionID: "test", ID: "search", CommandID: CommandTestSearch, Arguments: map[string]json.RawMessage{"direction": json.RawMessage(`"up"`)}}
	require.NoError(t, catalog.ValidateRequest(search))
	search.Arguments["direction"] = json.RawMessage(`"north"`)
	require.Error(t, catalog.ValidateRequest(search))
}

func TestInitialCatalogValidatesFourthBatchReviewedParameters(t *testing.T) {
	catalog := NewInitialCatalog()
	shard := Request{SessionID: "test", ID: "shard", CommandID: CommandQuestShardUse, Arguments: map[string]json.RawMessage{
		"shard": json.RawMessage(`"shard_hatred"`), "expectedQuantity": json.RawMessage(`1`),
		"expectedLocationId": json.RawMessage(`31`), "expectedX": json.RawMessage(`15`), "expectedY": json.RawMessage(`3`),
		"expectedFloor": json.RawMessage(`1`), "expectedFlame": json.RawMessage(`"love"`),
		"expectedShadowlordSummoned": json.RawMessage(`true`), "expectedShadowlord": json.RawMessage(`"astaroth"`),
		"expectedShadowlordAlive": json.RawMessage(`true`),
	}}
	require.NoError(t, catalog.ValidateRequest(shard))
	shard.Arguments["shard"] = json.RawMessage(`"amulet"`)
	require.Error(t, catalog.ValidateRequest(shard))

	require.NoError(t, catalog.ValidateRequest(Request{SessionID: "test", ID: "preset", CommandID: CommandQuestInventoryPresetUsable, Arguments: map[string]json.RawMessage{
		"expectedFingerprint": json.RawMessage(`"reviewed"`),
	}}))
}

func TestInitialCatalogValidatesApprovedToolBoundaries(t *testing.T) {
	catalog := NewInitialCatalog()
	valid := []Request{
		{SessionID: "test", ID: "time", CommandID: CommandCalendarSetTime, Arguments: map[string]json.RawMessage{"hour": json.RawMessage(`23`), "minute": json.RawMessage(`59`)}},
		{SessionID: "test", ID: "day", CommandID: CommandCalendarSetDay, Arguments: map[string]json.RawMessage{"day": json.RawMessage(`28`)}},
		{SessionID: "test", ID: "stack", CommandID: CommandSearchItemStackAdd, Arguments: map[string]json.RawMessage{"x": json.RawMessage(`255`), "y": json.RawMessage(`0`), "item": json.RawMessage(`"gold"`), "quantity": json.RawMessage(`999`)}},
		{SessionID: "test", ID: "quest", CommandID: CommandQuestItemEnsure, Arguments: map[string]json.RawMessage{"item": json.RawMessage(`"sextant"`)}},
	}
	for _, request := range valid {
		require.NoError(t, catalog.ValidateRequest(request), request.ID)
	}

	invalid := []Request{
		{SessionID: "test", ID: "time", CommandID: CommandCalendarSetTime, Arguments: map[string]json.RawMessage{"hour": json.RawMessage(`24`), "minute": json.RawMessage(`0`)}},
		{SessionID: "test", ID: "day", CommandID: CommandCalendarSetDay, Arguments: map[string]json.RawMessage{"day": json.RawMessage(`0`)}},
		{SessionID: "test", ID: "stack", CommandID: CommandSearchItemStackAdd, Arguments: map[string]json.RawMessage{"x": json.RawMessage(`0`), "y": json.RawMessage(`0`), "item": json.RawMessage(`"gems"`), "quantity": json.RawMessage(`1`)}},
		{SessionID: "test", ID: "quest", CommandID: CommandQuestItemRemove, Arguments: map[string]json.RawMessage{"item": json.RawMessage(`"skull_key"`)}},
	}
	for _, request := range invalid {
		require.ErrorIs(t, catalog.ValidateRequest(request), ErrInvalidRequest, request.ID)
	}
}

func TestInitialCatalogNormalizesSearchResetScope(t *testing.T) {
	normalized, err := NewInitialCatalog().NormalizeRequest(Request{SessionID: "test", ID: "reset", CommandID: CommandSearchProgressReset})
	require.NoError(t, err)
	assert.JSONEq(t, `"one_time_locations"`, string(normalized.Arguments["scope"]))
}

func TestInitialCatalogNormalizesSecondBatchDefaults(t *testing.T) {
	catalog := NewInitialCatalog()

	encounter, err := catalog.NormalizeRequest(Request{
		SessionID: "test", ID: "encounter", CommandID: CommandCombatEncounterStart,
		Arguments: map[string]json.RawMessage{
			"territory": json.RawMessage(`"britannia"`), "britanniaMap": json.RawMessage(`2`),
			"entryDirection": json.RawMessage(`"north"`), "monsterType": json.RawMessage(`"32"`),
			"countMode": json.RawMessage(`"automatic"`),
		},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `true`, string(encounter.Arguments["friends"]))
}

func TestInitialCatalogValidatesSecondBatchBounds(t *testing.T) {
	catalog := NewInitialCatalog()
	valid := []Request{
		{SessionID: "test", ID: "generation", CommandID: CommandMonsterGenerationSet, Arguments: map[string]json.RawMessage{"enabled": json.RawMessage(`true`), "odds": json.RawMessage(`1000`)}},
		{SessionID: "test", ID: "damage", CommandID: CommandShipDamageRaw, Arguments: map[string]json.RawMessage{"amount": json.RawMessage(`98`)}},
		{SessionID: "test", ID: "spawn", CommandID: CommandVehicleSpawn, Arguments: map[string]json.RawMessage{"type": json.RawMessage(`"frigate"`), "placement": json.RawMessage(`"dock"`), "dock": json.RawMessage(`"east_britanny"`)}},
		{SessionID: "test", ID: "board", CommandID: CommandVehicleForceBoard, Arguments: map[string]json.RawMessage{"type": json.RawMessage(`"carpet"`)}},
	}
	for _, request := range valid {
		require.NoError(t, catalog.ValidateRequest(request), request.ID)
	}

	invalid := []Request{
		{SessionID: "test", ID: "generation", CommandID: CommandMonsterGenerationSet, Arguments: map[string]json.RawMessage{"enabled": json.RawMessage(`true`), "odds": json.RawMessage(`0`)}},
		{SessionID: "test", ID: "damage", CommandID: CommandShipDamageRaw, Arguments: map[string]json.RawMessage{"amount": json.RawMessage(`99`)}},
		{SessionID: "test", ID: "spawn", CommandID: CommandVehicleSpawn, Arguments: map[string]json.RawMessage{"type": json.RawMessage(`"horse"`), "placement": json.RawMessage(`"avatar"`)}},
		{SessionID: "test", ID: "board", CommandID: CommandVehicleForceBoard, Arguments: map[string]json.RawMessage{"type": json.RawMessage(`"none"`)}},
	}
	for _, request := range invalid {
		require.ErrorIs(t, catalog.ValidateRequest(request), ErrInvalidRequest, request.ID)
	}
}

func TestInitialCatalogValidatesInventoryLockerArguments(t *testing.T) {
	catalog := NewInitialCatalog()
	valid := []Request{
		{SessionID: "test", ID: "set", CommandID: CommandInventoryProvisionSet, Arguments: map[string]json.RawMessage{"provision": json.RawMessage(`"food"`), "targetQuantity": json.RawMessage(`0`), "expectedQuantity": json.RawMessage(`9999`)}},
		{SessionID: "test", ID: "reagent", CommandID: CommandInventoryReagentSet, Arguments: map[string]json.RawMessage{"reagent": json.RawMessage(`"7"`), "targetQuantity": json.RawMessage(`99`), "expectedQuantity": json.RawMessage(`0`)}},
		{SessionID: "test", ID: "equipment", CommandID: CommandInventoryEquipmentSet, Arguments: map[string]json.RawMessage{"equipment": json.RawMessage(`"47"`), "targetQuantity": json.RawMessage(`0`), "expectedQuantity": json.RawMessage(`99`)}},
		{SessionID: "test", ID: "potion", CommandID: CommandInventoryPotionAdd, Arguments: map[string]json.RawMessage{"potion": json.RawMessage(`"7"`), "amount": json.RawMessage(`1`)}},
		{SessionID: "test", ID: "preset", CommandID: CommandInventoryReagentPreset, Arguments: map[string]json.RawMessage{"preset": json.RawMessage(`"reagents_all99"`), "expectedSnapshot": json.RawMessage(`"snapshot"`)}},
		{SessionID: "test", ID: "grant", CommandID: CommandInventoryEquipmentGrantPreset, Arguments: map[string]json.RawMessage{"preset": json.RawMessage(`"weapons5"`), "expectedSnapshot": json.RawMessage(`"snapshot"`)}},
		{SessionID: "test", ID: "clear", CommandID: CommandInventoryEquipmentClear, Arguments: map[string]json.RawMessage{"expectedSnapshot": json.RawMessage(`"snapshot"`)}},
		{SessionID: "test", ID: "spells", CommandID: CommandInventorySpellPresetMax, Arguments: map[string]json.RawMessage{"expectedSnapshot": json.RawMessage(`"snapshot"`)}},
		{SessionID: "test", ID: "spell-set", CommandID: CommandInventorySpellSet, Arguments: map[string]json.RawMessage{"spell": json.RawMessage(`"47"`), "targetQuantity": json.RawMessage(`99`), "expectedQuantity": json.RawMessage(`0`)}},
	}
	for _, request := range valid {
		require.NoError(t, catalog.ValidateRequest(request), request.ID)
	}

	invalid := []Request{
		{SessionID: "test", ID: "set", CommandID: CommandInventoryProvisionSet, Arguments: map[string]json.RawMessage{"provision": json.RawMessage(`"food"`), "targetQuantity": json.RawMessage(`10000`), "expectedQuantity": json.RawMessage(`0`)}},
		{SessionID: "test", ID: "potion", CommandID: CommandInventoryPotionAdd, Arguments: map[string]json.RawMessage{"potion": json.RawMessage(`"8"`), "amount": json.RawMessage(`0`)}},
		{SessionID: "test", ID: "preset", CommandID: CommandInventoryScrollPreset, Arguments: map[string]json.RawMessage{"preset": json.RawMessage(`"scrolls_all1"`)}},
		{SessionID: "test", ID: "reagent", CommandID: CommandInventoryReagentSet, Arguments: map[string]json.RawMessage{"reagent": json.RawMessage(`"8"`), "targetQuantity": json.RawMessage(`0`), "expectedQuantity": json.RawMessage(`0`)}},
		{SessionID: "test", ID: "equipment", CommandID: CommandInventoryEquipmentSet, Arguments: map[string]json.RawMessage{"equipment": json.RawMessage(`"0"`), "targetQuantity": json.RawMessage(`100`), "expectedQuantity": json.RawMessage(`0`)}},
		{SessionID: "test", ID: "clear", CommandID: CommandInventoryEquipmentClear},
		{SessionID: "test", ID: "spells", CommandID: CommandInventorySpellPresetMax},
		{SessionID: "test", ID: "spell-set", CommandID: CommandInventorySpellSet, Arguments: map[string]json.RawMessage{"spell": json.RawMessage(`"48"`), "targetQuantity": json.RawMessage(`100`), "expectedQuantity": json.RawMessage(`0`)}},
	}
	for _, request := range invalid {
		require.ErrorIs(t, catalog.ValidateRequest(request), ErrInvalidRequest, request.ID)
	}
}

func TestInventoryEquipmentSetCatalogHasAllAuthoritativeEquipment(t *testing.T) {
	definition, ok := NewInitialCatalog().Get(CommandInventoryEquipmentSet)
	require.True(t, ok)
	require.Len(t, definition.Parameters, 3)
	require.Len(t, definition.Parameters[0].Choices, 48)

	for id, choice := range definition.Parameters[0].Choices {
		assert.Equal(t, strconv.Itoa(id), choice.Value)
		assert.Equal(t, references.Equipment(id).String(), choice.Label)
	}
}

func TestInitialCatalogValidatesActiveCombatConditionArguments(t *testing.T) {
	catalog := NewInitialCatalog()
	valid := Request{
		SessionID: "test", ID: "sleep", CommandID: CommandCombatMonsterSleepSet,
		Arguments: map[string]json.RawMessage{
			"encounterId":      json.RawMessage(`"4"`),
			"monsterId":        json.RawMessage(`"2"`),
			"asleep":           json.RawMessage(`true`),
			"expectedAsleep":   json.RawMessage(`false`),
			"expectedSnapshot": json.RawMessage(`"snapshot"`),
		},
	}
	require.NoError(t, catalog.ValidateRequest(valid))

	delete(valid.Arguments, "expectedSnapshot")
	require.ErrorIs(t, catalog.ValidateRequest(valid), ErrInvalidRequest)
}

func TestInitialCatalogValidatesActiveCombatExitArguments(t *testing.T) {
	catalog := NewInitialCatalog()
	valid := Request{
		SessionID: "test", ID: "exit", CommandID: CommandCombatExitDebug,
		Arguments: map[string]json.RawMessage{
			"encounterId":      json.RawMessage(`"4"`),
			"expectedSnapshot": json.RawMessage(`"snapshot"`),
		},
	}
	require.NoError(t, catalog.ValidateRequest(valid))

	valid.Arguments["encounterId"] = json.RawMessage(`"0"`)
	require.ErrorIs(t, catalog.ValidateRequest(valid), ErrInvalidRequest)
	valid.Arguments["encounterId"] = json.RawMessage(`"4"`)
	delete(valid.Arguments, "expectedSnapshot")
	require.ErrorIs(t, catalog.ValidateRequest(valid), ErrInvalidRequest)
}

func TestInitialCatalogValidatesActiveCombatSpawnArguments(t *testing.T) {
	catalog := NewInitialCatalog()
	valid := Request{
		SessionID: "test", ID: "spawn", CommandID: CommandCombatMonsterSpawn,
		Arguments: map[string]json.RawMessage{
			"encounterId": json.RawMessage(`"4"`), "expectedSnapshot": json.RawMessage(`"snapshot"`),
			"monsterType": json.RawMessage(`32`), "countMode": json.RawMessage(`"exact"`),
			"exactCount": json.RawMessage(`2`), "friends": json.RawMessage(`true`), "seed": json.RawMessage(`0`),
		},
	}
	require.NoError(t, catalog.ValidateRequest(valid))

	valid.Arguments["monsterType"] = json.RawMessage(`48`)
	require.ErrorIs(t, catalog.ValidateRequest(valid), ErrInvalidRequest)
	valid.Arguments["monsterType"] = json.RawMessage(`32`)
	valid.Arguments["exactCount"] = json.RawMessage(`17`)
	require.ErrorIs(t, catalog.ValidateRequest(valid), ErrInvalidRequest)
}

func TestInitialCatalogValidatesLoggingAndPresentationArguments(t *testing.T) {
	catalog := NewInitialCatalog()
	for _, request := range []Request{
		{SessionID: "test", ID: "category", CommandID: CommandLoggingCategorySet, Arguments: map[string]json.RawMessage{"category": json.RawMessage(`"combat"`), "enabled": json.RawMessage(`true`)}},
		{SessionID: "test", ID: "level", CommandID: CommandLoggingLevelSet, Arguments: map[string]json.RawMessage{"level": json.RawMessage(`"WARN"`)}},
		{SessionID: "test", ID: "overlay", CommandID: CommandSearchOverlaySet, Arguments: map[string]json.RawMessage{"enabled": json.RawMessage(`true`)}},
		{SessionID: "test", ID: "speed", CommandID: CommandCombatPresentationSpeedSet, Arguments: map[string]json.RawMessage{"percent": json.RawMessage(`2000`)}},
	} {
		require.NoError(t, catalog.ValidateRequest(request))
	}

	invalid := Request{SessionID: "test", ID: "invalid", CommandID: CommandLoggingCategorySet, Arguments: map[string]json.RawMessage{"category": json.RawMessage(`"unknown"`), "enabled": json.RawMessage(`true`)}}
	require.ErrorIs(t, catalog.ValidateRequest(invalid), ErrInvalidRequest)
	invalid = Request{SessionID: "test", ID: "invalid", CommandID: CommandCombatPresentationSpeedSet, Arguments: map[string]json.RawMessage{"percent": json.RawMessage(`2001`)}}
	require.ErrorIs(t, catalog.ValidateRequest(invalid), ErrInvalidRequest)
}

func TestCatalogRejectsDuplicateCommandID(t *testing.T) {
	definition := validTestDefinition()

	_, err := NewCatalog([]CommandDefinition{definition, definition})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate command ID")
}

func TestCatalogValidatesTypedArguments(t *testing.T) {
	minimum := 1
	maximum := 3
	definition := validTestDefinition()
	definition.Parameters = []ParameterDefinition{
		{
			ID:       "count",
			Name:     "Count",
			Kind:     ParameterInteger,
			Required: true,
			Minimum:  &minimum,
			Maximum:  &maximum,
		},
		{
			ID:       "mode",
			Name:     "Mode",
			Kind:     ParameterEnum,
			Required: true,
			Choices: []Choice{
				{Value: "one", Label: "One", Metadata: nil},
				{Value: "two", Label: "Two", Metadata: nil},
			},
		},
	}
	catalog, err := NewCatalog([]CommandDefinition{definition})
	require.NoError(t, err)

	err = catalog.ValidateRequest(Request{
		SessionID: "test-session",
		ID:        "request-1",
		CommandID: definition.ID,
		Arguments: map[string]json.RawMessage{
			"count": json.RawMessage(`2`),
			"mode":  json.RawMessage(`"two"`),
		},
	})
	require.NoError(t, err)

	err = catalog.ValidateRequest(Request{
		SessionID: "test-session",
		ID:        "request-2",
		CommandID: definition.ID,
		Arguments: map[string]json.RawMessage{
			"count": json.RawMessage(`4`),
			"mode":  json.RawMessage(`"three"`),
		},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestCatalogRejectsUnknownCommand(t *testing.T) {
	catalog := NewInitialCatalog()

	err := catalog.ValidateRequest(Request{SessionID: "test-session", ID: "request-1", CommandID: "unknown.command"})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCommandNotFound))
}

func TestCatalogReturnsDefensiveCopies(t *testing.T) {
	minimum := 1
	definitionWithBounds := validTestDefinition()
	definitionWithBounds.Parameters = []ParameterDefinition{{
		ID:      "count",
		Name:    "Count",
		Kind:    ParameterInteger,
		Minimum: &minimum,
	}}
	catalog, err := NewCatalog([]CommandDefinition{definitionWithBounds})
	require.NoError(t, err)
	minimum = 10
	definitions := catalog.Definitions()
	*definitions[0].Parameters[0].Minimum = 20

	definition, ok := catalog.Get(definitionWithBounds.ID)

	require.True(t, ok)
	assert.Equal(t, 1, *definition.Parameters[0].Minimum)
}

func TestCatalogNormalizesDefaultsWithoutMutatingRequest(t *testing.T) {
	definition := validTestDefinition()
	definition.Parameters = []ParameterDefinition{{
		ID:      "count",
		Name:    "Count",
		Kind:    ParameterInteger,
		Default: json.RawMessage(`2`),
	}}
	catalog, err := NewCatalog([]CommandDefinition{definition})
	require.NoError(t, err)
	request := Request{SessionID: "test-session", ID: "request-1", CommandID: definition.ID}

	normalized, err := catalog.NormalizeRequest(request)

	require.NoError(t, err)
	assert.Nil(t, request.Arguments)
	assert.JSONEq(t, `2`, string(normalized.Arguments["count"]))
}

func TestNewNormalizedInvocationDefensivelyCopiesTypedArguments(t *testing.T) {
	argument := json.RawMessage(`{"count":2,"enabled":true}`)
	request := Request{
		CommandID: "test.command",
		Arguments: map[string]json.RawMessage{"settings": argument},
	}

	invocation := NewNormalizedInvocation(request)
	argument[9] = '9'
	delete(request.Arguments, "settings")

	assert.Equal(t, "test.command", invocation.CommandID)
	assert.JSONEq(t, `{"count":2,"enabled":true}`, string(invocation.Arguments["settings"]))
	empty := NewNormalizedInvocation(Request{CommandID: "test.empty"})
	assert.NotNil(t, empty.Arguments)
	assert.Empty(t, empty.Arguments)
}

func TestCatalogRejectsNullAndInvalidDefaults(t *testing.T) {
	definition := validTestDefinition()
	definition.Parameters = []ParameterDefinition{{
		ID:      "count",
		Name:    "Count",
		Kind:    ParameterInteger,
		Default: json.RawMessage(`null`),
	}}

	_, err := NewCatalog([]CommandDefinition{definition})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCatalog)
}

func TestCloneDiscoverySnapshotDetachesDynamicChoiceMetadata(t *testing.T) {
	original := DiscoverySnapshot{
		SceneGeneration: 2,
		OptionsRevision: 3,
		Commands:        NewInitialCatalog().Definitions(),
		OptionSets: []OptionSet{{
			Provider: OptionProviderActivePartyCharacters,
			Choices: []Choice{{
				Value:    "0.QXZhdGFy",
				Label:    "Position 1 · Avatar",
				Metadata: json.RawMessage(`{"strength":20}`),
			}},
			UnavailableReason: "outside combat only",
		}},
	}

	cloned := CloneDiscoverySnapshot(original)
	cloned.OptionSets[0].Choices[0].Metadata[0] = '['
	cloned.OptionSets[0].Choices[0].Label = "changed"
	cloned.OptionSets[0].UnavailableReason = "changed"

	assert.JSONEq(t, `{"strength":20}`, string(original.OptionSets[0].Choices[0].Metadata))
	assert.Equal(t, "Position 1 · Avatar", original.OptionSets[0].Choices[0].Label)
	assert.Equal(t, "outside combat only", original.OptionSets[0].UnavailableReason)
}

func validTestDefinition() CommandDefinition {
	return CommandDefinition{
		ID:             "test.command",
		Category:       "Test",
		Name:           "Test Command",
		Description:    "A test command.",
		ExecutionModes: []ExecutionMode{ExecutionQuery},
	}
}
