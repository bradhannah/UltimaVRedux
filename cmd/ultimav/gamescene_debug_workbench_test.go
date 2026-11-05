package main

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

type debugWorkbenchStateStub struct {
	wind           environment.WindDirection
	setWind        func(environment.WindDirection, bool)
	shrineProgress func(int) game_state.ShrineProgressSnapshot
	alterShrine    func(game_state.ShrineProgressAlteration) (game_state.ShrineProgressAlterationResult, error)
	characters     []game_state.ActiveCharacterSnapshot
	alter          func(game_state.ActiveCharacterAlteration) (game_state.ActiveCharacterAlterationResult, error)
	roster         game_state.PartyRosterSnapshot
	maximize       func(string) (game_state.PartyRosterMutationResult, error)
	setStatuses    func(game_state.PartyStatusMutationRequest) (game_state.PartyRosterMutationResult, error)
	setKarma       func(int) (game_state.PartyKarmaAlteration, error)
}

func (s debugWorkbenchStateStub) GetCurrentWind() environment.WindDirection {
	return s.wind
}

func (s debugWorkbenchStateStub) SetWindDirection(direction environment.WindDirection, immediate bool) {
	if s.setWind == nil {
		panic("unexpected wind alteration")
	}
	s.setWind(direction, immediate)
}

func (s debugWorkbenchStateStub) ShrineProgress(index int) game_state.ShrineProgressSnapshot {
	if s.shrineProgress == nil {
		panic("unexpected shrine query")
	}

	return s.shrineProgress(index)
}

func (s debugWorkbenchStateStub) AlterShrineProgress(
	input game_state.ShrineProgressAlteration,
) (game_state.ShrineProgressAlterationResult, error) {
	if s.alterShrine == nil {
		panic("unexpected shrine alteration")
	}

	return s.alterShrine(input)
}

func (s debugWorkbenchStateStub) ReviewAllShrineQuestState() game_state.AllShrineQuestStateReview {
	shrines := make([]game_state.ShrineProgressSnapshot, 0, game_state.TotalShrines)
	for index := range game_state.TotalShrines {
		shrines = append(shrines, s.ShrineProgress(index))
	}

	return game_state.AllShrineQuestStateReview{Fingerprint: "stub-shrine-review", Shrines: shrines}
}

func (s debugWorkbenchStateStub) ActivePartyCharacters() []game_state.ActiveCharacterSnapshot {
	return s.characters
}

func (s debugWorkbenchStateStub) AlterActiveCharacter(input game_state.ActiveCharacterAlteration) (game_state.ActiveCharacterAlterationResult, error) {
	if s.alter == nil {
		panic("unexpected character alteration")
	}

	return s.alter(input)
}

func (s debugWorkbenchStateStub) CurrentPartyRosterSnapshot() game_state.PartyRosterSnapshot {
	return s.roster
}

func (s debugWorkbenchStateStub) MaximizeActiveParty(fingerprint string) (game_state.PartyRosterMutationResult, error) {
	return s.maximize(fingerprint)
}

func (s debugWorkbenchStateStub) SetPartyRosterStatuses(request game_state.PartyStatusMutationRequest) (game_state.PartyRosterMutationResult, error) {
	return s.setStatuses(request)
}

func (debugWorkbenchStateStub) RestoreActiveParty(string) (game_state.PartyRosterMutationResult, error) {
	panic("debug workbench stub: no test restores the active party")
}

func (debugWorkbenchStateStub) FillActiveParty(string) (game_state.PartyRosterMutationResult, error) {
	panic("debug workbench stub: no test fills the active party")
}

func (debugWorkbenchStateStub) AddInactiveRosterMember(game_state.AddInactiveRosterMemberRequest) (game_state.PartyRosterMutationResult, error) {
	panic("debug workbench stub: no test adds an inactive roster member")
}

func (s debugWorkbenchStateStub) SetPartyKarma(value int) (game_state.PartyKarmaAlteration, error) {
	if s.setKarma == nil {
		panic("unexpected karma alteration")
	}

	return s.setKarma(value)
}

func (s debugWorkbenchStateStub) CurrentPartyKarma() int {
	return 50
}

func TestActiveCharacterOptionSetUsesOpaqueSlotAndOccupantToken(t *testing.T) {
	optionSet := activeCharacterOptionSet([]game_state.ActiveCharacterSnapshot{{
		Slot:       1,
		Name:       "Shamino",
		Class:      party_state.Fighter,
		Experience: 750,
	}}, 50, "")

	assert.Equal(t, debugging.OptionProviderActivePartyCharacters, optionSet.Provider)
	require.Len(t, optionSet.Choices, 1)
	slot, name, err := parseActiveCharacterToken(optionSet.Choices[0].Value)
	require.NoError(t, err)
	assert.Equal(t, 1, slot)
	assert.Equal(t, "Shamino", name)
	assert.Contains(t, optionSet.Choices[0].Label, "Position 2")
	assert.NotEmpty(t, optionSet.Choices[0].Metadata)
	var metadata activeCharacterOptionMetadata
	require.NoError(t, json.Unmarshal(optionSet.Choices[0].Metadata, &metadata))
	assert.Equal(t, uint16(750), metadata.Experience)
	assert.Equal(t, 50, metadata.Karma)
	positions := activePartyPositionOptionSet([]game_state.ActiveCharacterSnapshot{{Slot: 1, Name: "Shamino"}}, 50, "")
	assert.Equal(t, debugging.OptionProviderActivePartyPositions, positions.Provider)
	assert.Equal(t, "2", positions.Choices[0].Value)
	loadoutTargets := equipmentLoadoutTargetOptionSet([]game_state.ActiveCharacterSnapshot{{Slot: 1, Name: "Shamino"}}, 50, "")
	assert.Equal(t, debugging.OptionProviderEquipmentLoadoutTargets, loadoutTargets.Provider)
	require.Len(t, loadoutTargets.Choices, 2)
	assert.Equal(t, debugging.Choice{Value: "all", Label: "All active party members"}, loadoutTargets.Choices[0])
}

func mustUnmarshalDebugString(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value string
	require.NoError(t, json.Unmarshal(raw, &value))

	return value
}

func TestExecuteDebugCommandPartyCharacterInspect(t *testing.T) {
	state := debugWorkbenchStateStub{roster: game_state.PartyRosterSnapshot{Fingerprint: "reviewed", Characters: []game_state.PartyRosterCharacterSnapshot{{RosterIndex: 0, Name: "Avatar", PartyMap: party_state.PartyMapActive}}}, characters: []game_state.ActiveCharacterSnapshot{{
		Slot:         0,
		Name:         "Avatar",
		Class:        party_state.Avatar,
		Status:       party_state.Good,
		Strength:     20,
		Dexterity:    21,
		Intelligence: 22,
		CurrentMP:    18,
		CurrentHP:    90,
		MaximumHP:    100,
		Experience:   500,
		Level:        5,
	}}}

	result := executeDebugCommand(state, debugging.Request{
		SessionID: "test-session",
		ID:        "party-inspect",
		CommandID: debugging.CommandPartyCharacterInspect,
	})

	require.True(t, result.Success)
	var data activePartyResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	require.Len(t, data.Characters, 1)
	assert.Equal(t, "Avatar", data.Characters[0].Name)
	assert.Equal(t, 1, data.Characters[0].PartyPosition)
	assert.Equal(t, "reviewed", data.Fingerprint)
	assert.Equal(t, 50, data.Karma)
	require.Len(t, data.Roster, 1)
	assert.Equal(t, 0, data.Roster[0].RosterIndex)
}

func TestExecuteDebugCommandPartyRosterBatchMutationsUseReviewedStableIndices(t *testing.T) {
	before := game_state.PartyRosterSnapshot{Fingerprint: "before", Characters: []game_state.PartyRosterCharacterSnapshot{{RosterIndex: 0, Name: "Avatar", PartyMap: party_state.PartyMapActive, Status: party_state.Good, CurrentHP: 20, MaximumHP: 90, Level: 4}}}
	after := game_state.PartyRosterSnapshot{Fingerprint: "after", Characters: []game_state.PartyRosterCharacterSnapshot{{RosterIndex: 0, Name: "Avatar", PartyMap: party_state.PartyMapActive, Status: party_state.Dead, CurrentHP: 0, MaximumHP: 90, Level: 4}}}
	state := debugWorkbenchStateStub{setStatuses: func(request game_state.PartyStatusMutationRequest) (game_state.PartyRosterMutationResult, error) {
		assert.Equal(t, "before", request.ExpectedFingerprint)
		assert.Equal(t, []int{0}, request.RosterIndices)
		assert.Equal(t, party_state.Dead, request.Status)
		return game_state.PartyRosterMutationResult{Before: before, After: after}, nil
	}}
	result := executeDebugCommand(state, debugging.Request{
		SessionID: "test", ID: "status", CommandID: debugging.CommandPartyStatusSet,
		Arguments: map[string]json.RawMessage{
			"expectedFingerprint": json.RawMessage(`"before"`),
			"rosterIndices":       json.RawMessage(`[0]`),
			"status":              json.RawMessage(`"dead"`),
		},
	})

	require.True(t, result.Success)
	assert.Equal(t, debugging.ExecutionSynchronized, result.EffectiveExecutionMode)
	assert.Contains(t, stateChangePaths(result.Changes), "party.roster.0.status")
	assert.Contains(t, stateChangePaths(result.Changes), "party.roster.0.currentHp")
	var data partyRosterMutationResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.Equal(t, "before", data.Before.Fingerprint)
	assert.Equal(t, "after", data.After.Fingerprint)
}

func TestExecuteDebugCommandPartyRosterMutationRejectsStaleReview(t *testing.T) {
	state := debugWorkbenchStateStub{maximize: func(string) (game_state.PartyRosterMutationResult, error) {
		return game_state.PartyRosterMutationResult{}, game_state.ErrStalePartyRoster
	}}
	result := executeDebugCommand(state, debugging.Request{
		SessionID: "test", ID: "maximize", CommandID: debugging.CommandPartyMaximizeLevels,
		Arguments: map[string]json.RawMessage{"expectedFingerprint": json.RawMessage(`"stale"`)},
	})

	require.False(t, result.Success)
	assert.Equal(t, "stale_party_roster", result.ErrorCode)
	assert.Empty(t, result.Changes)
}

func TestExecuteDebugCommandPartyCharacterUpdateReturnsChanges(t *testing.T) {
	token := activeCharacterToken(1, "Shamino")
	state := debugWorkbenchStateStub{alter: func(input game_state.ActiveCharacterAlteration) (game_state.ActiveCharacterAlterationResult, error) {
		require.Equal(t, 1, input.Slot)
		require.Equal(t, "Shamino", input.ExpectedOccupant)
		require.NotNil(t, input.Strength)
		require.Equal(t, 24, *input.Strength)
		require.NotNil(t, input.CurrentHP)
		require.Equal(t, 90, *input.CurrentHP)
		require.NotNil(t, input.CurrentMP)
		require.Equal(t, 15, *input.CurrentMP)
		require.NotNil(t, input.Experience)
		require.Equal(t, 9000, *input.Experience)
		require.NotNil(t, input.Status)
		require.Equal(t, party_state.Poisoned, *input.Status)

		return game_state.ActiveCharacterAlterationResult{
			CharacterBefore: game_state.ActiveCharacterSnapshot{Slot: 1, Name: "Shamino", Strength: 20, CurrentHP: 80, CurrentMP: 10, Experience: 500, Status: party_state.Good},
			CharacterAfter:  game_state.ActiveCharacterSnapshot{Slot: 2, Name: "Shamino", Strength: 24, CurrentHP: 90, CurrentMP: 15, Experience: 9000, Status: party_state.Poisoned},
			OrderBefore:     []game_state.ActivePartySlotSnapshot{{Slot: 1, Name: "Shamino"}, {Slot: 2, Name: "Iolo"}},
			OrderAfter:      []game_state.ActivePartySlotSnapshot{{Slot: 1, Name: "Iolo"}, {Slot: 2, Name: "Shamino"}},
		}, nil
	}}
	request := debugging.Request{
		SessionID: "test-session",
		ID:        "party-update",
		CommandID: debugging.CommandPartyCharacterUpdate,
		Arguments: map[string]json.RawMessage{
			"character":     json.RawMessage(strconv.Quote(token)),
			"strength":      json.RawMessage(`24`),
			"currentHp":     json.RawMessage(`90`),
			"currentMp":     json.RawMessage(`15`),
			"experience":    json.RawMessage(`9000`),
			"status":        json.RawMessage(`"poisoned"`),
			"partyPosition": json.RawMessage(`"3"`),
		},
	}

	result := executeDebugCommand(state, request)

	require.True(t, result.Success)
	assert.Equal(t, debugging.ExecutionLiveSafe, result.EffectiveExecutionMode)
	assert.NotEmpty(t, result.Changes)
	assert.Equal(t, "party.character.Shamino.strength", result.Changes[0].Path)
	assert.Contains(t, stateChangePaths(result.Changes), "party.character.Shamino.currentHp")
	assert.Contains(t, stateChangePaths(result.Changes), "party.character.Shamino.currentMp")
	assert.Contains(t, stateChangePaths(result.Changes), "party.character.Shamino.experience")
	var orderChange debugging.StateChange
	for _, change := range result.Changes {
		if change.Path == "party.order" {
			orderChange = change
			break
		}
	}
	require.NotEmpty(t, orderChange.Path)
	var orderAfter []partyOrderSlotResult
	require.NoError(t, json.Unmarshal(orderChange.After, &orderAfter))
	assert.Equal(t, []partyOrderSlotResult{{PartyPosition: 2, Name: "Iolo"}, {PartyPosition: 3, Name: "Shamino"}}, orderAfter)
}

func TestExecuteDebugCommandSetsExactPartyKarma(t *testing.T) {
	state := debugWorkbenchStateStub{setKarma: func(value int) (game_state.PartyKarmaAlteration, error) {
		assert.Equal(t, 88, value)
		return game_state.PartyKarmaAlteration{Before: 50, After: value}, nil
	}}
	result := executeDebugCommand(state, debugging.Request{
		SessionID: "test", ID: "karma", CommandID: debugging.CommandPartyKarmaSet,
		Arguments: map[string]json.RawMessage{"value": json.RawMessage(`88`)},
	})

	require.True(t, result.Success)
	require.Len(t, result.Changes, 1)
	assert.Equal(t, "party.karma", result.Changes[0].Path)
}

func stateChangePaths(changes []debugging.StateChange) []string {
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		paths = append(paths, change.Path)
	}

	return paths
}

func TestExecuteDebugCommandWindStatus(t *testing.T) {
	request := debugging.Request{
		SessionID: "test-session",
		ID:        "request-1",
		CommandID: debugging.CommandEnvironmentWindStatus,
	}

	result := executeDebugCommand(debugWorkbenchStateStub{wind: environment.North}, request)

	require.True(t, result.Success)
	var data windStatusResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.Equal(t, environment.North.String(), data.Direction)
}

func TestExecuteDebugCommandWindSetReturnsSemanticChange(t *testing.T) {
	state := debugWorkbenchStateStub{wind: environment.North}
	state.setWind = func(direction environment.WindDirection, immediate bool) {
		assert.Equal(t, environment.West, direction)
		assert.True(t, immediate)
		state.wind = direction
	}

	result := executeDebugCommand(&state, debugging.Request{
		SessionID: "test-session",
		ID:        "wind-set",
		CommandID: debugging.CommandEnvironmentWindSet,
		Arguments: map[string]json.RawMessage{"direction": json.RawMessage(`"west"`)},
	})

	require.True(t, result.Success)
	assert.Equal(t, debugging.ExecutionLiveSafe, result.EffectiveExecutionMode)
	require.Len(t, result.Changes, 1)
	assert.Equal(t, "environment.wind.direction", result.Changes[0].Path)
}

func TestExecuteDebugCommandShrineStatusCombinesReferenceAndProgress(t *testing.T) {
	state := debugWorkbenchStateStub{shrineProgress: func(index int) game_state.ShrineProgressSnapshot {
		progress := game_state.ShrineProgressSnapshot{Index: index, QuestState: game_state.ShrineProgressNotAssigned}
		if index == 2 {
			progress.QuestState = game_state.ShrineProgressReadyToComplete
			progress.Completed = true
			progress.Active = true
		}

		return progress
	}}

	result := executeDebugCommand(state, debugging.Request{
		SessionID: "test-session",
		ID:        "shrine-status",
		CommandID: debugging.CommandShrineStatus,
	})

	require.True(t, result.Success)
	var data shrineStatusResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	require.Len(t, data.Shrines, game_state.TotalShrines)
	assert.Equal(t, "stub-shrine-review", data.Fingerprint)
	assert.Equal(t, "Honesty", data.Shrines[0].VirtueName)
	assert.Equal(t, 2, data.FirstActiveIndex)
	assert.False(t, data.AllCompleted)
}

func TestExecuteDebugCommandShrineProgressUpdateDoesNotReplayRewards(t *testing.T) {
	state := debugWorkbenchStateStub{alterShrine: func(input game_state.ShrineProgressAlteration) (game_state.ShrineProgressAlterationResult, error) {
		require.Equal(t, 7, input.Index)
		require.NotNil(t, input.QuestState)
		assert.Equal(t, game_state.ShrineProgressOffering, *input.QuestState)
		require.NotNil(t, input.Ruined)
		assert.False(t, *input.Ruined)

		return game_state.ShrineProgressAlterationResult{
			Before: game_state.ShrineProgressSnapshot{
				Index: 7, QuestState: game_state.ShrineProgressReadyToComplete, Completed: true, Active: true, Ruined: true,
			},
			After: game_state.ShrineProgressSnapshot{
				Index: 7, QuestState: game_state.ShrineProgressOffering, Completed: true, Active: false, Ruined: false,
			},
		}, nil
	}}
	result := executeDebugCommand(state, debugging.Request{
		SessionID: "test-session",
		ID:        "shrine-update",
		CommandID: debugging.CommandShrineProgressUpdate,
		Arguments: map[string]json.RawMessage{
			"index":      json.RawMessage(`"7"`),
			"questState": json.RawMessage(`"offering"`),
			"ruined":     json.RawMessage(`false`),
		},
	})

	require.True(t, result.Success)
	assert.Equal(t, debugging.ExecutionLiveSafe, result.EffectiveExecutionMode)
	assert.NotEmpty(t, result.Changes)
}

func TestExecuteDebugCommandLoggingStatus(t *testing.T) {
	logger := logging.GetGlobalLogger()
	beforeEnabled, beforeStdout := logger.IsEnabled(), logger.IsStdoutEnabled()
	beforeLevel, beforeCategories := logger.GetMinLevel(), logger.GetCategoryStates()
	t.Cleanup(func() {
		logger.SetEnabled(beforeEnabled)
		logger.SetStdoutEnabled(beforeStdout)
		logger.SetMinLevel(beforeLevel)
		logger.SetCategoryStates(beforeCategories)
	})
	request := debugging.Request{
		SessionID: "test-session",
		ID:        "request-1",
		CommandID: debugging.CommandLoggingStatus,
	}

	result := executeDebugCommand(nil, request)

	require.True(t, result.Success)
	var data loggingStatusResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.NotEmpty(t, data.MinimumLevel)
	assert.Len(t, data.CategoryStates, len(logging.GetAllCategories()))

	enabled := executeDebugCommand(nil, debugging.Request{SessionID: "test", ID: "enabled", CommandID: debugging.CommandLoggingEnabledSet,
		Arguments: map[string]json.RawMessage{"enabled": json.RawMessage(`true`)}})
	require.True(t, enabled.Success)
	assert.Equal(t, debugging.ExecutionLiveSafe, enabled.EffectiveExecutionMode)
	assert.True(t, logger.IsEnabled())
	assert.Equal(t, "logging.enabled", enabled.Changes[0].Path)

	category := executeDebugCommand(nil, debugging.Request{SessionID: "test", ID: "category", CommandID: debugging.CommandLoggingCategorySet,
		Arguments: map[string]json.RawMessage{"category": json.RawMessage(`"combat"`), "enabled": json.RawMessage(`true`)}})
	require.True(t, category.Success)
	assert.True(t, logger.GetCategoryStates()[logging.CategoryCombat])

	level := executeDebugCommand(nil, debugging.Request{SessionID: "test", ID: "level", CommandID: debugging.CommandLoggingLevelSet,
		Arguments: map[string]json.RawMessage{"level": json.RawMessage(`"WARN"`)}})
	require.True(t, level.Success)
	assert.Equal(t, logging.WARN, logger.GetMinLevel())
}

func TestExecuteApprovedCalendarCommands(t *testing.T) {
	state := newApprovedToolTestState()
	state.DateTime = datetime.UltimaDate{Year: 1, Month: 2, Day: 3, Hour: 4, Minute: 5, Turn: 6}

	setTime := executeDebugCommand(state, debugging.Request{
		SessionID: "test", ID: "time", CommandID: debugging.CommandCalendarSetTime,
		Arguments: map[string]json.RawMessage{"hour": json.RawMessage(`23`), "minute": json.RawMessage(`59`)},
	})
	require.True(t, setTime.Success)
	assert.Equal(t, debugging.ExecutionLiveSafe, setTime.EffectiveExecutionMode)
	assert.Equal(t, byte(23), state.DateTime.Hour)
	assert.Equal(t, byte(59), state.DateTime.Minute)
	assert.Len(t, setTime.Changes, 2)

	setDay := executeDebugCommand(state, debugging.Request{
		SessionID: "test", ID: "day", CommandID: debugging.CommandCalendarSetDay,
		Arguments: map[string]json.RawMessage{"day": json.RawMessage(`28`)},
	})
	require.True(t, setDay.Success)
	assert.Equal(t, byte(28), state.DateTime.Day)
	require.Len(t, setDay.Changes, 1)
	assert.Equal(t, "calendar.day", setDay.Changes[0].Path)

	status := executeDebugCommand(state, debugging.Request{SessionID: "test", ID: "status", CommandID: debugging.CommandCalendarStatus})
	require.True(t, status.Success)
	var calendar calendarStatusResult
	require.NoError(t, json.Unmarshal(status.Data, &calendar))
	assert.Equal(t, byte(28), calendar.Day)
	assert.Equal(t, "Crescent Waning", calendar.MoonPhase)
	assert.Equal(t, 7, calendar.MoonPhaseIndex)
}

func TestExecuteSearchStatusAndItemStackAdd(t *testing.T) {
	state := newApprovedToolTestState()
	state.MapState.PlayerLocation = references.PlayerLocation{
		Location: references.Britain, Position: references.Position{X: 2, Y: 2}, Floor: 0,
	}
	require.NoError(t, state.ItemStacksMap.Push(&references.Position{X: 2, Y: 2}, &references.ItemStack{Items: references.ItemStacks{
		{Item: references.Food, Quantity: 3}, {Item: references.Key, Quantity: 2},
	}}))

	statusResult := executeDebugCommand(state, debugging.Request{SessionID: "test", ID: "status", CommandID: debugging.CommandSearchStatus})
	require.True(t, statusResult.Success)
	var status searchStatusResult
	require.NoError(t, json.Unmarshal(statusResult.Data, &status))
	require.Len(t, status.Cells, 5)
	assert.Len(t, status.Cells[0].ItemStack, 2)
	require.Len(t, status.Cells[1].AuthenticLocations, 1)
	assert.Equal(t, 29, status.Cells[1].AuthenticLocations[0].Index)
	assert.Greater(t, status.Progress.OneTimeLocations, 0)
	state.MapState.PlayerLocation.Location = references.BritanniaUnderworld

	addResult := executeDebugCommand(state, debugging.Request{
		SessionID: "test", ID: "add", CommandID: debugging.CommandSearchItemStackAdd,
		Arguments: map[string]json.RawMessage{"x": json.RawMessage(`2`), "y": json.RawMessage(`2`), "item": json.RawMessage(`"food"`), "quantity": json.RawMessage(`4`)},
	})
	require.True(t, addResult.Success)
	require.Len(t, addResult.Changes, 1)
	assert.Equal(t, "search.itemStacks.2.2.food", addResult.Changes[0].Path)
	stack, exists := state.ItemStacksMap.Snapshot(&references.Position{X: 2, Y: 2})
	require.True(t, exists)
	assert.Equal(t, uint16(7), stack.Items[0].Quantity)
}

func TestExecuteSearchProgressResetUsesAccurateScope(t *testing.T) {
	state := newApprovedToolTestState()
	found := state.SearchManager.GetAuthenticLocationManager().SearchAtPosition(2, 1, 0, references.Britain)
	require.NotNil(t, found)
	result := executeDebugCommand(state, debugging.Request{
		SessionID: "test", ID: "reset", CommandID: debugging.CommandSearchProgressReset,
		Arguments: map[string]json.RawMessage{"scope": json.RawMessage(`"one_time_locations"`)},
	})

	require.True(t, result.Success)
	assert.Equal(t, debugging.ExecutionAlteration, result.EffectiveExecutionMode)
	require.Len(t, result.Changes, 1)
	assert.Equal(t, "search.progress.one_time_locations.found", result.Changes[0].Path)
	var progress searchProgressResult
	require.NoError(t, json.Unmarshal(result.Data, &progress))
	assert.Zero(t, progress.FoundOneTimeLocations)
}

func TestExecuteQuestItemStatusEnsureAndRemove(t *testing.T) {
	state := newApprovedToolTestState()
	statusResult := executeDebugCommand(state, debugging.Request{SessionID: "test", ID: "status", CommandID: debugging.CommandQuestItemStatus})
	require.True(t, statusResult.Success)
	var status questItemStatusResult
	require.NoError(t, json.Unmarshal(statusResult.Data, &status))
	require.Len(t, status.Items, references.NumberOfQuestItems)
	assert.Equal(t, "amulet", status.Items[0].Key)
	assert.Equal(t, "artifact", status.Items[0].Category)
	require.NotNil(t, status.Items[0].CanonicalPlacement)
	assert.Nil(t, status.Items[references.Sextant].CanonicalPlacement)

	ensure := executeDebugCommand(state, debugging.Request{
		SessionID: "test", ID: "ensure", CommandID: debugging.CommandQuestItemEnsure,
		Arguments: map[string]json.RawMessage{"item": json.RawMessage(`"sextant"`)},
	})
	require.True(t, ensure.Success)
	assert.Equal(t, debugging.ExecutionAlteration, ensure.EffectiveExecutionMode)
	require.Len(t, ensure.Changes, 1)
	assert.Equal(t, "quest.items.sextant.quantity", ensure.Changes[0].Path)
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.Sextant))

	remove := executeDebugCommand(state, debugging.Request{
		SessionID: "test", ID: "remove", CommandID: debugging.CommandQuestItemRemove,
		Arguments: map[string]json.RawMessage{"item": json.RawMessage(`"sextant"`)},
	})
	require.True(t, remove.Success)
	assert.Equal(t, uint16(0), state.PartyState.Inventory.QuestItems.Get(references.Sextant))
}

func TestExecuteWorldPositionStatus(t *testing.T) {
	state := newApprovedToolTestState()
	state.MapState.PlayerLocation = references.PlayerLocation{
		Location: references.Britain, Position: references.Position{X: 4, Y: 5}, Floor: 1,
	}

	statusResult := executeDebugCommand(state, debugging.Request{SessionID: "test", ID: "status", CommandID: debugging.CommandWorldPositionStatus})
	require.True(t, statusResult.Success)
	var status debugging.WorldPositionStatusData
	require.NoError(t, json.Unmarshal(statusResult.Data, &status))
	assert.Equal(t, "small", status.MapType)
	assert.Equal(t, 31, status.Bounds.MaximumX)
	assert.Equal(t, 1, status.Floor)
	require.Len(t, status.Destinations, int(references.Doom)+1)

}

func TestExecuteMoongateStatusReturnsDetachedMoonstonesAndRoute(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(224, 133).
		WithSystemCallbacks().
		Build()
	state.DateTime = datetime.UltimaDate{Year: 1, Month: 1, Day: 1, Hour: 22}
	state.GetMoonstoneSystem().MarkMoonstoneFoundAtPosition(0)
	stabilizeMoongateStage(t, state)

	result := executeDebugCommand(state, debugging.Request{SessionID: "test", ID: "status", CommandID: debugging.CommandMoongateStatus})
	require.True(t, result.Success)
	var status moongateStatusResult
	require.NoError(t, json.Unmarshal(result.Data, &status))
	require.Len(t, status.Moonstones, 8)
	assert.True(t, status.Visible)
	assert.False(t, status.TeleportationActive)
	require.NotNil(t, status.Route)
	assert.Equal(t, "moonstone", status.Route.Kind)
	require.NotNil(t, status.Route.Destination)
	assert.Equal(t, "found_not_collected", status.Moonstones[0].State)
	status.Moonstones[0].X = 0
	assert.NotEqual(t, status.Moonstones[0].X, state.GetMoonstones()[0].X)
}

func TestExecuteMoongateStatusRepresentsSpiritualityWithoutCoordinates(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	state.DateTime = datetime.UltimaDate{Year: 1, Month: 1, Day: 1, Hour: 0, Minute: 9}

	result := executeDebugCommand(state, debugging.Request{
		SessionID: "test", ID: "status", CommandID: debugging.CommandMoongateStatus,
	})

	require.True(t, result.Success)
	var status moongateStatusResult
	require.NoError(t, json.Unmarshal(result.Data, &status))
	require.NotNil(t, status.Route)
	assert.Equal(t, "spirituality", status.Route.Kind)
	assert.Nil(t, status.Route.Destination)
	require.NotNil(t, status.Route.ShrineIndex)
	assert.Equal(t, game_state.SpiritualityShrineIndex, *status.Route.ShrineIndex)
	assert.Equal(t, "Spirituality", status.Route.ShrineName)
}

func newApprovedToolTestState() *game_state.GameState {
	state := &game_state.GameState{
		DateTime:        datetime.UltimaDate{Year: 1, Month: 1, Day: 1},
		ItemStacksMap:   *references.NewItemStacksMap(),
		GameReferences:  &references.GameReferences{InventoryItemReferences: references.NewInventoryItemsReferences()},
		SystemCallbacks: &game_state.SystemCallbacks{},
	}
	state.PartyState.Inventory = *party_state.NewInventory()
	state.SearchManager = game_state.NewSearchSystemManager(state)
	return state
}

func TestExecuteDebugCommandRejectsUnimplementedCommand(t *testing.T) {
	request := debugging.Request{SessionID: "test-session", ID: "request-1", CommandID: "unimplemented.command"}

	result := executeDebugCommand(nil, request)

	assert.False(t, result.Success)
	assert.Equal(t, "command_not_implemented", result.ErrorCode)
}
