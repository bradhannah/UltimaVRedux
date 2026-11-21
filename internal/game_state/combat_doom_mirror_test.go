package game_state

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestDoomMirrorAbsorbsOnlyActingPCInVerifiedFinalRoom(t *testing.T) {
	state, callbacks := newDoomMirrorCombatTestState(t, 2, true)
	actor := state.CombatScene.GetCurrentTurnIdentity()
	state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 5, Y: 2})
	statusBefore := state.PartyState.GetCharacter(actor.EntityIndex).GetStatus()

	state.finishCombatAction()

	assert.False(t, state.CombatScene.IsPartyMemberActive(actor.EntityIndex))
	entry := state.CombatScene.TurnQueue.GetEntryByIdentity(actor)
	require.NotNil(t, entry)
	assert.Equal(t, turnQueueAbsorbed, entry.status)
	assert.Equal(t, statusBefore, state.PartyState.GetCharacter(actor.EntityIndex).GetStatus())
	assert.True(t, state.PartyState.GetCharacter(actor.EntityIndex).IsAlive())
	assert.True(t, state.CombatScene.IsPartyMemberActive(1))
	assert.Equal(t, CombatExitDoomMirror, state.CombatScene.ExitRoute)
	assert.Contains(t, callbacks.Messages, "Avatar is absorbed!")
	assert.True(t, callbacks.SoundEffectWasPlayed(SoundAbsorb), "being taken by the mirror is heard as well as read")
	assert.Empty(t, callbacks.CombatConclusions)
}

func TestDoomMirrorRequiresVerifiedRoomAndGeometry(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CombatState)
	}{
		{
			name: "other dungeon",
			mutate: func(combat *CombatState) {
				combat.DungeonRoomOrigin.Location = references.Deceit
			},
		},
		{
			name: "other room",
			mutate: func(combat *CombatState) {
				combat.DungeonRoomOrigin.Room = doomFinalRoomID - 1
			},
		},
		{
			name: "cleared room",
			mutate: func(combat *CombatState) {
				combat.DungeonRoomOrigin.Live = false
			},
		},
		{
			name: "wrong dungeon coordinate",
			mutate: func(combat *CombatState) {
				combat.DungeonRoomOrigin.Position.X--
			},
		},
		{
			name: "actor off absorption row",
			mutate: func(combat *CombatState) {
				identity := combat.GetCurrentTurnIdentity()
				combat.SetPartyMemberPosition(identity.EntityIndex, references.Position{X: 5, Y: 3})
			},
		},
		{
			name: "mirror not directly north",
			mutate: func(combat *CombatState) {
				combat.CombatMapRef.SetTileNumber(
					references.Position{X: 5, Y: doomMirrorTerrainY},
					indexes.BrickFloor,
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := newDoomMirrorCombatTestState(t, 1, true)
			actor := state.CombatScene.GetCurrentTurnIdentity()
			state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 5, Y: 2})
			test.mutate(state.CombatScene)

			state.finishCombatAction()

			assert.True(t, state.CombatScene.IsPartyMemberActive(actor.EntityIndex))
			entry := state.CombatScene.TurnQueue.GetEntryByIdentity(actor)
			require.NotNil(t, entry)
			assert.Equal(t, turnQueueActive, entry.status)
			assert.Equal(t, CombatExitNone, state.CombatScene.ExitRoute)
			assert.False(t, state.HasPendingLordBritishFinale())
			assert.False(t, callbacks.SoundEffectWasPlayed(SoundAbsorb))
			assert.Empty(t, callbacks.CombatConclusions)
		})
	}
}

func TestDoomMirrorNumericSelectionDoesNotAbsorb(t *testing.T) {
	state, _ := newDoomMirrorCombatTestState(t, 2, true)
	actor := state.CombatScene.GetCurrentTurnIdentity()
	state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 5, Y: 2})

	state.FinishCombatSelection()

	require.NotNil(t, state.CombatScene)
	assert.True(t, state.CombatScene.IsPartyMemberActive(actor.EntityIndex))
	entry := state.CombatScene.TurnQueue.GetEntryByIdentity(actor)
	require.NotNil(t, entry)
	assert.Equal(t, turnQueueActive, entry.status)
	assert.Equal(t, CombatExitNone, state.CombatScene.ExitRoute)
}

func TestDoomMirrorAbsorptionFollowsFieldExpiration(t *testing.T) {
	state, callbacks := newDoomMirrorCombatTestState(t, 1, true)
	actor := state.CombatScene.GetCurrentTurnIdentity()
	state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 5, Y: 2})
	field := NewCombatField(CombatFieldProtection, references.Position{X: 1, Y: 1})
	field.createdThisAction = true
	require.True(t, state.CombatScene.AddField(field))
	seed := findCombatWakeSeed(t, state, 255, func(roll int) bool { return roll < 16 })
	state.SetRandomSeed(seed)
	callbacks.Reset()

	state.finishCombatAction()

	assert.Empty(t, state.CombatScene.Fields())
	entry := state.CombatScene.TurnQueue.GetEntryByIdentity(actor)
	require.NotNil(t, entry)
	assert.Equal(t, turnQueueAbsorbed, entry.status)
	require.Len(t, callbacks.CombatEvents, 1)
	assert.Equal(t, CombatEventFieldExpired, callbacks.CombatEvents[0].Kind)
}

func TestDoomMirrorMultipleAbsorptionsWithEnemiesEndInDefeat(t *testing.T) {
	state, callbacks := newDoomMirrorCombatTestState(t, 2, true)
	for partyIndex := range 2 {
		identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: partyIndex}
		state.CombatScene.CurrentTurnIdentity = identity
		state.CombatScene.CurrentTurnIndex = partyIndex
		state.CombatScene.SetPartyMemberPosition(partyIndex, references.Position{X: 5, Y: 2})
		state.finishCombatAction()
		assert.Equal(t, turnQueueAbsorbed, state.CombatScene.TurnQueue.GetEntryByIdentity(identity).status)
	}

	reason := state.checkCombatEndConditions()

	assert.Equal(t, CombatDefeat, reason)
	assert.Nil(t, state.CombatScene)
	require.Len(t, callbacks.CombatConclusions, 1)
	assert.Equal(t, CombatConclusion{
		Reason: CombatDefeat, Special: CombatSpecialConclusionNone,
	}, callbacks.CombatConclusions[0])
}

func TestDoomMirrorSourceAbsorptionPreservesActivePCClone(t *testing.T) {
	state, callbacks := newDoomMirrorCombatTestState(t, 1, true)
	source := state.CombatScene.GetCurrentTurnIdentity()
	clone := state.CombatScene.addPCCloneFromSource(
		source,
		source.EntityIndex,
		references.Position{X: 4, Y: 4},
		NewCombatPCProvider(state.PartyState.GetCharacter(source.EntityIndex)),
	)
	require.NotZero(t, clone.Slot)
	state.CombatScene.SetPartyMemberPosition(source.EntityIndex, references.Position{X: 5, Y: 2})

	state.finishCombatAction()

	assert.Equal(t, CombatNotEnded, state.checkCombatEndConditions())
	require.NotNil(t, state.CombatScene)
	cloneEntry := state.CombatScene.TurnQueue.GetEntryByIdentity(clone)
	require.NotNil(t, cloneEntry)
	assert.Equal(t, turnQueueActive, cloneEntry.status)
	assert.Empty(t, callbacks.CombatConclusions)

	state.CombatScene.CurrentTurnIdentity = clone
	state.CombatScene.CurrentTurnIndex = clone.EntityIndex
	state.CombatScene.setCombatantPosition(clone, references.Position{X: 5, Y: 2})
	state.finishCombatAction()

	assert.Equal(t, CombatDefeat, state.checkCombatEndConditions())
	require.Len(t, callbacks.CombatConclusions, 1)
	assert.Equal(t, CombatSpecialConclusionNone, callbacks.CombatConclusions[0].Special)
}

func TestDoomMirrorFinalAbsorptionPublishesOneFinaleHandoff(t *testing.T) {
	state, callbacks := newDoomMirrorCombatTestState(t, 1, false)
	actor := state.CombatScene.GetCurrentTurnIdentity()
	state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 5, Y: 2})
	state.CombatScene.DungeonRoomOrigin.finishEndPlayerOnReturn = true
	state.activeDungeon.roomContinuation.finishEndPlayer = true

	state.FinishCombatAction()

	assert.Nil(t, state.CombatScene)
	require.Len(t, callbacks.CombatConclusions, 1)
	assert.Equal(t, CombatConclusion{
		Reason: CombatVictory, Special: CombatSpecialConclusionDoomMirror,
	}, callbacks.CombatConclusions[0])
	assert.Equal(t, references.Doom, state.MapState.PlayerLocation.Location)
	cell, ok := state.activeDungeon.Cell(doomFinalRoomFloor, references.Position{X: doomFinalRoomX, Y: doomFinalRoomY})
	require.True(t, ok)
	assert.Equal(t, references.DungeonLiveRoom, cell.Kind(), "finale handoff precedes ordinary room clearing")
	assert.True(t, state.IsAtDungeonCommandBoundary(), "finale handoff must clear the suspended room continuation")
	assert.True(t, state.HasPendingLordBritishFinale())
}

func TestDoomMirrorProvisionDeathSuppressesFinaleHandoff(t *testing.T) {
	state, callbacks := newDoomMirrorCombatTestState(t, 1, false)
	actor := state.CombatScene.GetCurrentTurnIdentity()
	state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 5, Y: 2})
	state.PartyState.GetCharacter(actor.EntityIndex).SetHp(1)
	state.PartyState.Inventory.Provisions.Food.Set(0)
	state.pendingProvisionHours = []int{6}
	state.DateTime.Hour = 6
	state.lastProcessedHour = 5
	deathSequences := 0
	state.SystemCallbacks.Flow.Consequences.TriggerDeathSequence = func() { deathSequences++ }

	state.FinishCombatAction()

	assert.Nil(t, state.CombatScene)
	require.Len(t, callbacks.CombatConclusions, 1)
	assert.Equal(t, CombatSpecialConclusionNone, callbacks.CombatConclusions[0].Special)
	assert.False(t, state.HasPendingLordBritishFinale())
	assert.Equal(t, 1, deathSequences)
}

func newDoomMirrorCombatTestState(
	t *testing.T,
	partyCount int,
	withMonster bool,
) (*GameState, *MockSystemCallbacks) {
	t.Helper()

	state := &GameState{}
	state.SetRandomSeed(1)
	state.EventQueueManager = event_queue.NewEventQueueManager(func() int64 { return state.ElapsedMs }, nil)
	var roster [party_state.NRosterSlots]*party_state.PlayerCharacter
	for index := range roster {
		roster[index] = &party_state.PlayerCharacter{PartyMap: party_state.PartyMapUnrecruited}
	}
	for index := range partyCount {
		character := roster[index]
		character.PartyMap = party_state.PartyMapActive
		character.Dexterity = 20
		character.SetMaxHp(100)
		character.SetHp(100)
		name := fmt.Sprintf("PC%d", index+1)
		if index == 0 {
			name = "Avatar"
		}
		copy(character.Name[:], name)
	}
	state.PartyState = *party_state.NewPartyStateFromRoster(roster)
	callbacks := NewMockSystemCallbacks(t)
	state.SystemCallbacks = callbacks.ToSystemCallbacks()
	state.environmentalHazards = environment.NewEnvironmentalHazards(
		state.rng,
		&messageCallbacksAdapter{state.SystemCallbacks},
	)

	roomPosition := references.Position{X: doomFinalRoomX, Y: doomFinalRoomY}
	var cells references.DungeonCells
	require.True(t, cells.SetCell(doomFinalRoomFloor, roomPosition, references.DungeonCell(0xf0|doomFinalRoomID)))
	dungeon, err := newDungeonState(
		references.Doom,
		cells,
		references.DungeonFacingNorth,
		references.DungeonLastNorth,
	)
	require.NoError(t, err)
	state.activeDungeon = dungeon
	state.MapState.PlayerLocation = references.PlayerLocation{
		Location: references.CombatLocation,
		Floor:    0,
		Position: references.Position{X: combatMapCenterCoord, Y: combatMapCenterCoord},
	}
	state.MapState.XTilesVisibleOnGameScreen = 19
	state.MapState.YTilesVisibleOnGameScreen = 13
	state.MapState.LayeredMaps = *map_state.NewLayeredMaps(
		nil,
		&references.LargeMapReference{},
		&references.LargeMapReference{},
		19,
		13,
	)
	tileReferences := references.NewTileReferences()
	state.MapState.LayeredMaps.ResetAndCreateDungeonMap(cells, tileReferences, 19, 13)

	combatMap := references.NewCombatMapReference(
		references.CalculateDungeonCombatMapIndex(references.DoomDungeon, doomFinalRoomID),
		references.DungeonCombatTerritory,
	)
	var rawMap [references.XCombatMapTiles][references.YCombatMapTiles]byte
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			rawMap[x][y] = byte(indexes.BrickFloor)
		}
	}
	rawMap[5][doomMirrorTerrainY] = byte(indexes.Mirror)
	combatMap.SetRawData(&rawMap)
	combat := NewCombatState(combatMap, CombatEntryNorth, &state.PartyState)
	combat.PreCombatPlayerLocation = references.PlayerLocation{
		Location: references.Doom,
		Floor:    doomFinalRoomFloor,
		Position: roomPosition,
	}
	combat.ExitContext = CombatExitDungeonRoom
	combat.DungeonRoomOrigin = &DungeonRoomEncounterOrigin{
		Location:     references.Doom,
		Floor:        doomFinalRoomFloor,
		Position:     roomPosition,
		DungeonIndex: references.DoomDungeon,
		Room:         doomFinalRoomID,
		Live:         true,
	}
	state.CombatScene = combat
	state.MapState.LayeredMaps.ResetAndCreateCombatMap(combatMap, tileReferences, 19, 13)
	if withMonster {
		reference := references.EnemyReference{
			MonsterType: references.MonsterOrc,
			Name:        "Orc",
			Dexterity:   10,
			HitPoints:   20,
		}
		combat.AddMonsterToQueue(NewCombatMonster(
			references.MonsterOrc,
			&reference,
			references.Position{X: 8, Y: 8},
			newFixedRNG(1),
		))
	}
	callbacks.Reset()

	return state, callbacks
}
