package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestProductionDungeonPackedSpellWorkflowMatrix(t *testing.T) {
	tests := []struct {
		spell        references.Spell
		keys         []ebiten.Key
		current      references.DungeonCell
		facing       references.DungeonCell
		wantCurrent  references.DungeonCell
		wantFacing   references.DungeonCell
		wantMessages []string
		wantSound    game_state.SoundEffect
	}{
		{
			spell: references.AnSanct, keys: []ebiten.Key{ebiten.KeyA, ebiten.KeyS},
			current: 0x41, facing: 0x00, wantCurrent: 0x70, wantFacing: 0x00,
			wantMessages: []string{"Disarmed!", "Chest opened!"}, wantSound: game_state.SoundUnlock,
		},
		{
			spell: references.InFlamGrav, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyF, ebiten.KeyG},
			current: 0x00, facing: 0x00, wantCurrent: 0x00, wantFacing: 0x82,
			wantSound: game_state.SoundMagic,
		},
		{
			spell: references.InNoxGrav, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyN, ebiten.KeyG},
			current: 0x00, facing: 0x08, wantCurrent: 0x00, wantFacing: 0x89,
			wantSound: game_state.SoundMagic,
		},
		{
			spell: references.InZuGrav, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyZ, ebiten.KeyG},
			current: 0x00, facing: 0x00, wantCurrent: 0x00, wantFacing: 0x80,
			wantSound: game_state.SoundMagic,
		},
		{
			spell: references.InSanctGrav, keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyS, ebiten.KeyG},
			current: 0x00, facing: 0x08, wantCurrent: 0x00, wantFacing: 0x8b,
			wantSound: game_state.SoundMagic,
		},
		{
			spell: references.AnGrav, keys: []ebiten.Key{ebiten.KeyA, ebiten.KeyG},
			current: 0x8a, facing: 0x80, wantCurrent: 0x08, wantFacing: 0x80,
			wantMessages: []string{"Field destroyed!"}, wantSound: game_state.SoundDispel,
		},
	}
	for _, test := range tests {
		t.Run(references.GetSpellName(test.spell), func(t *testing.T) {
			_, state, callbacks := newDungeonInputTestScene(t)
			require.True(t, state.SetActivePlayer(0))
			caster := state.PartyState.GetCharacter(0)
			caster.Class = party_state.Wizard
			caster.Level = 8
			caster.SetMp(8)
			state.PartyState.Inventory.Spells.Set(test.spell, 1)
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
			}
			current := state.MapState.PlayerLocation.Position
			facing := references.Position{X: 2, Y: 1}
			setDungeonInputTestCell(t, state, current, test.current)
			setDungeonInputTestCell(t, state, facing, test.facing)
			beforeTime := state.DateTime
			callbacks.Reset()
			scene := newProductionCastTestScene(state)

			scene.dungeonMapInputHandler(ebiten.KeyC)
			spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			enterSpellSyllables(scene, spellDialog, test.keys)
			require.Equal(t, test.spell, spellDialog.GetSelectedSpell())
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			spellDialog.Update()
			require.True(t, spellDialog.HasPendingCastRequest())
			drainProductionCastWorkflow(t, scene)

			expectedTime := beforeTime
			expectedTime.Advance(1)
			assert.Equal(t, expectedTime, state.DateTime)
			assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
			assert.Equal(t, byte(8-references.GetSpellCircle(test.spell)), caster.GetCurrentMp())
			assert.Zero(t, state.PartyState.Inventory.Spells.Get(test.spell))
			assert.Equal(t, test.wantCurrent, dungeonInputCellAt(t, state, current))
			assert.Equal(t, test.wantFacing, dungeonInputCellAt(t, state, facing))
			assert.Subset(t, callbacks.Messages, test.wantMessages)
			assert.Contains(t, callbacks.SoundEffectsPlayed, test.wantSound)
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.True(t, state.IsAtDungeonCommandBoundary())
		})
	}
}

func TestProductionDungeonPackedSpellRejectionPreservesResourcesAndCompletesCommand(t *testing.T) {
	_, state, callbacks := newDungeonInputTestScene(t)
	require.True(t, state.SetActivePlayer(0))
	caster := state.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.SetMp(8)
	state.PartyState.Inventory.Spells.Set(references.AnSanct, 1)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
	}
	setDungeonInputTestCell(t, state, state.MapState.PlayerLocation.Position, 0x00)
	setDungeonInputTestCell(t, state, references.Position{X: 2, Y: 1}, 0xb0)
	beforeTime := state.DateTime
	callbacks.Reset()
	scene := newProductionCastTestScene(state)

	scene.dungeonMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	enterSpellSyllables(scene, spellDialog, []ebiten.Key{ebiten.KeyA, ebiten.KeyS})
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	spellDialog.Update()
	failure := waitForTopModal[*widgets.OverlayDialog](t, scene)
	assert.Equal(t, "Failed!", failure.GetMessage())
	failure.Dismiss()
	drainProductionCastWorkflow(t, scene)

	expectedTime := beforeTime
	assert.Equal(t, expectedTime, state.DateTime)
	assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)
	assert.Equal(t, byte(8), caster.GetCurrentMp())
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.AnSanct))
	assert.Contains(t, callbacks.SoundEffectsPlayed, game_state.SoundSpellFailure)
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestProductionDungeonPackedSpellCancellationCompletesCommand(t *testing.T) {
	_, state, _ := newDungeonInputTestScene(t)
	require.True(t, state.SetActivePlayer(0))
	caster := state.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.SetMp(8)
	state.PartyState.Inventory.Spells.Set(references.AnSanct, 1)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
	}
	beforeTime := state.DateTime
	scene := newProductionCastTestScene(state)

	scene.dungeonMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	spellDialog.Dismiss()
	drainProductionCastWorkflow(t, scene)

	expectedTime := beforeTime
	assert.Equal(t, expectedTime, state.DateTime)
	assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)
	assert.Equal(t, byte(8), caster.GetCurrentMp())
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.AnSanct))
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestProductionDungeonFloorSpellWorkflowHandlesInternalAndBoundaryTravel(t *testing.T) {
	tests := []struct {
		name         string
		spell        references.Spell
		keys         []ebiten.Key
		startFloor   references.FloorNumber
		targetCell   references.DungeonCell
		wantLocation references.Location
		wantFloor    references.FloorNumber
		wantDuration int
	}{
		{
			name: "Uus Por internal", spell: references.UusPor, keys: []ebiten.Key{ebiten.KeyU, ebiten.KeyP},
			startFloor: 1, targetCell: 0x08, wantLocation: references.Deceit, wantFloor: 0, wantDuration: 4,
		},
		{
			name: "Des Por internal", spell: references.DesPor, keys: []ebiten.Key{ebiten.KeyD, ebiten.KeyP},
			startFloor: 1, targetCell: 0x00, wantLocation: references.Deceit, wantFloor: 2, wantDuration: 4,
		},
		{
			name: "Uus Por boundary", spell: references.UusPor, keys: []ebiten.Key{ebiten.KeyU, ebiten.KeyP},
			startFloor: 0, wantLocation: references.BritanniaUnderworld, wantFloor: 0, wantDuration: 5,
		},
		{
			name: "Des Por boundary", spell: references.DesPor, keys: []ebiten.Key{ebiten.KeyD, ebiten.KeyP},
			startFloor: 7, wantLocation: references.BritanniaUnderworld, wantFloor: -1, wantDuration: 5,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, state, _ := newDungeonInputTestScene(t)
			require.True(t, state.SetActivePlayer(0))
			caster := state.PartyState.GetCharacter(0)
			caster.Class = party_state.Wizard
			caster.Level = 8
			caster.SetMp(8)
			state.PartyState.Inventory.Spells.Set(test.spell, 1)
			state.MapState.PlayerLocation.Floor = test.startFloor
			position := state.MapState.PlayerLocation.Position
			if test.wantLocation == references.Deceit {
				_, err := state.TransformDungeonCell(test.wantFloor, position,
					func(references.DungeonCell) references.DungeonCell { return test.targetCell })
				require.NoError(t, err)
			}
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
			}
			beforeTime := state.DateTime
			dungeonEntrance := state.GetDungeonEntranceCoordinates()[references.Deceit]
			scene := newProductionCastTestScene(state)

			scene.dungeonMapInputHandler(ebiten.KeyC)
			spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			enterSpellSyllables(scene, spellDialog, test.keys)
			require.Equal(t, test.spell, spellDialog.GetSelectedSpell())
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			spellDialog.Update()
			drainProductionCastWorkflow(t, scene)

			expectedTime := beforeTime
			expectedTime.Advance(1)
			assert.Equal(t, expectedTime, state.DateTime)
			assert.Equal(t, test.wantLocation, state.MapState.PlayerLocation.Location)
			assert.Equal(t, test.wantFloor, state.MapState.PlayerLocation.Floor)
			wantPosition := position
			if test.wantLocation == references.BritanniaUnderworld {
				wantPosition = dungeonEntrance
			}
			assert.Equal(t, wantPosition, state.MapState.PlayerLocation.Position)
			assert.Equal(t, test.wantDuration, state.ActiveDurationEffect.TurnsRemaining)
			assert.Equal(t, byte(4), caster.GetCurrentMp())
			assert.Zero(t, state.PartyState.Inventory.Spells.Get(test.spell))
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.True(t, state.IsAtDungeonCommandBoundary())
		})
	}
}

func TestProductionDungeonFloorSpellRejectionPreservesResources(t *testing.T) {
	_, state, _ := newDungeonInputTestScene(t)
	require.True(t, state.SetActivePlayer(0))
	caster := state.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.SetMp(8)
	state.PartyState.Inventory.Spells.Set(references.UusPor, 1)
	state.MapState.PlayerLocation.Floor = 1
	_, err := state.TransformDungeonCell(0, state.MapState.PlayerLocation.Position,
		func(references.DungeonCell) references.DungeonCell { return 0xb0 })
	require.NoError(t, err)
	scene := newProductionCastTestScene(state)

	scene.dungeonMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	enterSpellSyllables(scene, spellDialog, []ebiten.Key{ebiten.KeyU, ebiten.KeyP})
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	spellDialog.Update()
	failure := waitForTopModal[*widgets.OverlayDialog](t, scene)
	assert.Equal(t, "Failed!", failure.GetMessage())
	failure.Dismiss()
	drainProductionCastWorkflow(t, scene)

	assert.Equal(t, byte(8), caster.GetCurrentMp())
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.UusPor))
	assert.Equal(t, references.Deceit, state.MapState.PlayerLocation.Location)
	assert.Equal(t, references.FloorNumber(1), state.MapState.PlayerLocation.Floor)
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestProductionDungeonTimeSpellsActivateInDeceitAndDoom(t *testing.T) {
	tests := []struct {
		spell          references.Spell
		keys           []ebiten.Key
		effect         game_state.DurationEffectType
		remainingTurns int
		remainingMP    byte
	}{
		{
			spell: references.RelTym, keys: []ebiten.Key{ebiten.KeyR, ebiten.KeyT},
			effect: game_state.DurationEffectQuickness, remainingTurns: 29, remainingMP: 3,
		},
		{
			spell: references.AnTym, keys: []ebiten.Key{ebiten.KeyA, ebiten.KeyT},
			effect: game_state.DurationEffectTimeStop, remainingTurns: 9, remainingMP: 0,
		},
	}

	for _, location := range []references.Location{references.Deceit, references.Doom} {
		for _, test := range tests {
			t.Run(location.String()+"/"+references.GetSpellName(test.spell), func(t *testing.T) {
				start := dungeonSpellPassagePosition(t, location, 1)
				state, _ := gamefixture.NewIntegrationTestBuilder(t).
					WithLocation(location).
					WithFloor(1).
					WithPlayerAt(int(start.X), int(start.Y)).
					WithSystemCallbacks().
					Build()
				require.True(t, state.SetActivePlayer(0))
				caster := state.PartyState.GetCharacter(0)
				caster.Class = party_state.Wizard
				caster.Level = 8
				caster.SetMp(8)
				state.PartyState.Inventory.Spells.Set(test.spell, 1)
				beforeTime := state.DateTime
				scene := newProductionCastTestScene(state)

				scene.dungeonMapInputHandler(ebiten.KeyC)
				spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
				enterSpellSyllables(scene, spellDialog, test.keys)
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
				spellDialog.Update()
				drainProductionCastWorkflow(t, scene)

				expectedTime := beforeTime
				expectedTime.Advance(1)
				assert.Equal(t, expectedTime, state.DateTime)
				assert.Equal(t, test.effect, state.ActiveDurationEffect.EffectType)
				assert.Equal(t, test.remainingTurns, state.ActiveDurationEffect.TurnsRemaining)
				assert.Equal(t, test.remainingMP, caster.GetCurrentMp())
				assert.Zero(t, state.PartyState.Inventory.Spells.Get(test.spell))
				assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
				assert.True(t, state.IsAtDungeonCommandBoundary())
			})
		}
	}
}

func TestProductionDungeonAnTymAtMoongateBoundaryCompletesWithoutDeadlock(t *testing.T) {
	start := dungeonSpellPassagePosition(t, references.Deceit, 1)
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Deceit).
		WithFloor(1).
		WithPlayerAt(int(start.X), int(start.Y)).
		WithSystemCallbacks().
		Build()
	require.True(t, state.SetActivePlayer(0))
	caster := state.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.SetMp(8)
	state.PartyState.Inventory.Spells.Set(references.AnTym, 1)
	state.DateTime.Hour, state.DateTime.Minute = 19, 59
	scene := newProductionCastTestScene(state)

	scene.dungeonMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	enterSpellSyllables(scene, spellDialog, []ebiten.Key{ebiten.KeyA, ebiten.KeyT})
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	spellDialog.Update()
	for range 30 {
		state.ElapsedMs += 1000
		state.UpdateMoongateTransition()
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow("spell_casting_workflow") {
			break
		}
	}

	assert.False(t, state.EventQueueManager.HasWorkflow("spell_casting_workflow"))
	assert.False(t, state.IsMoongateTransitionActive())
	assert.Equal(t, byte(20), state.DateTime.Hour)
	assert.Equal(t, byte(0), state.DateTime.Minute)
	assert.Equal(t, game_state.DurationEffectTimeStop, state.ActiveDurationEffect.EffectType)
	assert.Equal(t, 9, state.ActiveDurationEffect.TurnsRemaining)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestProductionDungeonVasRelPorCompletesTokenBeforeExactTravel(t *testing.T) {
	for _, location := range []references.Location{references.Deceit, references.Doom} {
		t.Run(location.String(), func(t *testing.T) {
			start := dungeonSpellPassagePosition(t, location, 1)
			state, _ := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(location).
				WithFloor(1).
				WithPlayerAt(int(start.X), int(start.Y)).
				WithSystemCallbacks().
				Build()
			require.True(t, state.SetActivePlayer(0))
			caster := state.PartyState.GetCharacter(0)
			caster.Class = party_state.Wizard
			caster.Level = 8
			caster.SetMp(8)
			state.PartyState.Inventory.Spells.Set(references.VasRelPor, 1)
			stone := state.GetMoonstoneSystem().GetMoonstoneByIndex(int(datetime.MoonPhaseFirstQuarter))
			stone.X, stone.Y = 14, 15
			stone.Map = uint8(references.Britain)
			stone.Level = 1
			stone.State = game_state.MoonstoneStateBuried
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
			}
			state.DateTime.Hour, state.DateTime.Minute = 5, 59
			beforeTime := state.DateTime
			scene := newProductionCastTestScene(state)

			scene.dungeonMapInputHandler(ebiten.KeyC)
			spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			enterSpellSyllables(scene, spellDialog, []ebiten.Key{ebiten.KeyV, ebiten.KeyR, ebiten.KeyP})
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			spellDialog.Update()
			phaseDialog := waitForTopModal[*widgets.InputBox](t, scene)
			scene.keyboard.SimulateKeyPress(ebiten.Key3)
			phaseDialog.Update()
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			phaseDialog.Update()
			drainProductionCastWorkflow(t, scene)

			expectedTime := beforeTime
			expectedTime.Advance(1)
			assert.Equal(t, expectedTime, state.DateTime, "dungeon precharge is the only elapsed minute")
			assert.Equal(t, references.Britain, state.MapState.PlayerLocation.Location)
			assert.Equal(t, references.FloorNumber(1), state.MapState.PlayerLocation.Floor)
			assert.Equal(t, references.Position{X: 14, Y: 15}, state.MapState.PlayerLocation.Position)
			assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining,
				"successful travel must suppress dungeon end-player processing")
			assert.Zero(t, caster.GetCurrentMp())
			assert.Zero(t, state.PartyState.Inventory.Spells.Get(references.VasRelPor))
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.True(t, state.IsAtDungeonCommandBoundary())
			entry := state.CurrentQuestJournalDossier().Moonstones[datetime.MoonPhaseFirstQuarter]
			assert.True(t, entry.Known && entry.Placed)
			assert.Equal(t, "Reached through Vas Rel Por", entry.FirstSource.String())
			assert.False(t, entry.LastObservedSurvey.Measured, "town arrival cannot be surveyed")
			assert.Equal(t, int(expectedTime.Hour), entry.ObservedAt.Time.Hour)
			assert.Equal(t, int(expectedTime.Minute), entry.ObservedAt.Time.Minute, "dungeon precharge precedes observation")
		})
	}
}

func TestProductionDungeonVasRelPorInstallsCrossDungeonDestination(t *testing.T) {
	start := dungeonSpellPassagePosition(t, references.Deceit, 1)
	destination := dungeonSpellPassagePosition(t, references.Doom, 1)
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Deceit).
		WithFloor(1).
		WithPlayerAt(int(start.X), int(start.Y)).
		WithSystemCallbacks().
		Build()
	require.True(t, state.SetActivePlayer(0))
	caster := state.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.SetMp(8)
	state.PartyState.Inventory.Spells.Set(references.VasRelPor, 1)
	stone := state.GetMoonstoneSystem().GetMoonstoneByIndex(int(datetime.MoonPhaseFirstQuarter))
	stone.X, stone.Y = uint8(destination.X), uint8(destination.Y)
	stone.Map = uint8(references.Doom)
	stone.Level = 1
	stone.State = game_state.MoonstoneStateBuried
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
	}
	beforeTime := state.DateTime
	scene := newProductionCastTestScene(state)

	scene.dungeonMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	enterSpellSyllables(scene, spellDialog, []ebiten.Key{ebiten.KeyV, ebiten.KeyR, ebiten.KeyP})
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	spellDialog.Update()
	phaseDialog := waitForTopModal[*widgets.InputBox](t, scene)
	scene.keyboard.SimulateKeyPress(ebiten.Key3)
	phaseDialog.Update()
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	phaseDialog.Update()
	drainProductionCastWorkflow(t, scene)

	expectedTime := beforeTime
	expectedTime.Advance(1)
	assert.Equal(t, expectedTime, state.DateTime)
	assert.Equal(t, references.Doom, state.MapState.PlayerLocation.Location)
	assert.Equal(t, references.FloorNumber(1), state.MapState.PlayerLocation.Floor)
	assert.Equal(t, destination, state.MapState.PlayerLocation.Position)
	snapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Equal(t, references.Doom, snapshot.Location)
	assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)
	assert.Zero(t, caster.GetCurrentMp())
	assert.Zero(t, state.PartyState.Inventory.Spells.Get(references.VasRelPor))
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestProductionDungeonVasRelPorUnavailableAndCancelledPathsRemainAtomic(t *testing.T) {
	tests := []struct {
		name       string
		cancel     bool
		stoneState game_state.MoonstoneState
		stoneMap   uint8
	}{
		{name: "cancel", cancel: true, stoneState: game_state.MoonstoneStateBuried, stoneMap: uint8(references.Britain)},
		{name: "inventory stone", stoneState: game_state.MoonstoneStateInInventory, stoneMap: 0xff},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.Deceit).
				WithFloor(1).
				WithPlayerAt(1, 1).
				WithSystemCallbacks().
				Build()
			require.True(t, state.SetActivePlayer(0))
			caster := state.PartyState.GetCharacter(0)
			caster.Class = party_state.Wizard
			caster.Level = 8
			caster.SetMp(8)
			state.PartyState.Inventory.Spells.Set(references.VasRelPor, 1)
			stone := state.GetMoonstoneSystem().GetMoonstoneByIndex(int(datetime.MoonPhaseFirstQuarter))
			stone.Map = test.stoneMap
			stone.State = test.stoneState
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
			}
			scene := newProductionCastTestScene(state)

			scene.dungeonMapInputHandler(ebiten.KeyC)
			spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			enterSpellSyllables(scene, spellDialog, []ebiten.Key{ebiten.KeyV, ebiten.KeyR, ebiten.KeyP})
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			spellDialog.Update()
			phaseDialog := waitForTopModal[*widgets.InputBox](t, scene)
			if test.cancel {
				scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
				phaseDialog.Update()
			} else {
				scene.keyboard.SimulateKeyPress(ebiten.Key3)
				phaseDialog.Update()
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
				phaseDialog.Update()
				failure := waitForTopModal[*widgets.OverlayDialog](t, scene)
				assert.Equal(t, "No effect!", failure.GetMessage())
				failure.Dismiss()
			}
			drainProductionCastWorkflow(t, scene)

			assert.Equal(t, byte(8), caster.GetCurrentMp())
			assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.VasRelPor))
			assert.Equal(t, references.Deceit, state.MapState.PlayerLocation.Location)
			assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.True(t, state.IsAtDungeonCommandBoundary())
		})
	}
}

func enterSpellSyllables(scene *GameScene, dialog *widgets.SpellCastingDialog, keys []ebiten.Key) {
	for _, key := range keys {
		scene.keyboard.SimulateKeyPress(key)
		dialog.Update()
	}
}

func dungeonSpellPassagePosition(
	t *testing.T,
	location references.Location,
	floor references.FloorNumber,
) references.Position {
	t.Helper()
	probe, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(location).
		WithFloor(floor).
		WithPlayerAt(1, 1).
		WithSystemCallbacks().
		Build()
	snapshot, active := probe.DungeonStateSnapshot()
	require.True(t, active)
	for y := references.Coordinate(0); y < references.YDungeonMapTiles; y++ {
		for x := references.Coordinate(0); x < references.XDungeonMapTiles; x++ {
			position := references.Position{X: x, Y: y}
			cell, ok := snapshot.Cells.Cell(floor, position)
			require.True(t, ok)
			if cell.Raw()&0xf0 == 0 {
				return position
			}
		}
	}
	t.Fatal("dungeon floor has no passage cell")

	return references.Position{}
}
