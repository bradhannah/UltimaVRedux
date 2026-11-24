package game_state

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestDungeonChestLifecyclePreservesCeilingBitAndKeyRules(t *testing.T) {
	t.Run("Open preserves ceiling bit", func(t *testing.T) {
		state, _ := newDungeonInteractionTestState(t, 0x48, 1)

		runDungeonInteraction(t, state, func() { assert.True(t, state.ActionOpenDungeonMap(0)) })

		assert.Equal(t, references.DungeonCell(0x78), currentDungeonInteractionCell(t, state))
	})

	t.Run("Jimmy disarms trap without consuming key", func(t *testing.T) {
		state, _ := newDungeonInteractionTestState(t, 0x4f, 1)
		state.PartyState.GetCharacter(0).Dexterity = 255
		state.PartyState.Inventory.Provisions.Keys.Set(1)

		runDungeonInteraction(t, state, func() { assert.True(t, state.ActionJimmyDungeonMap(0)) })

		assert.Equal(t, references.DungeonCell(0x48), currentDungeonInteractionCell(t, state))
		assert.Equal(t, uint16(1), state.PartyState.Inventory.Provisions.Keys.Get())
	})

	t.Run("Jimmy always breaks key on normal chest", func(t *testing.T) {
		state, _ := newDungeonInteractionTestState(t, 0x48, 1)
		state.PartyState.GetCharacter(0).Dexterity = 255
		state.PartyState.Inventory.Provisions.Keys.Set(1)

		runDungeonInteraction(t, state, func() { assert.False(t, state.ActionJimmyDungeonMap(0)) })

		assert.Equal(t, references.DungeonCell(0x48), currentDungeonInteractionCell(t, state))
		assert.Zero(t, state.PartyState.Inventory.Provisions.Keys.Get())
	})

	t.Run("Get consumes open chest", func(t *testing.T) {
		state, _ := newDungeonInteractionTestState(t, 0x78, 1)

		runDungeonInteraction(t, state, func() { assert.True(t, state.ActionGetDungeonMap()) })

		assert.Equal(t, references.DungeonCell(0x08), currentDungeonInteractionCell(t, state))
	})
}

func TestDungeonOpenTrapTargetsMatchOriginalPayloads(t *testing.T) {
	trapNames := [...]string{"none", "acid", "poison", "bomb", "gas"}
	for _, trapType := range []TrapType{TrapAcid, TrapPoison, TrapBomb, TrapGas} {
		t.Run(trapNames[trapType], func(t *testing.T) {
			state, _ := newDungeonInteractionTestState(t, 0x41, 0)
			require.GreaterOrEqual(t, state.PartyState.GetCharacterCount(), 2)
			for index := range 2 {
				character := state.PartyState.GetCharacter(index)
				character.PartyMap = party_state.PartyMapActive
				character.SetStatus(party_state.Good)
				character.SetMaxHp(100)
				character.SetHp(100)
			}
			seed := findDungeonTrapSeed(t, state, trapType)
			state.SetRandomSeed(seed)

			runDungeonInteraction(t, state, func() { assert.True(t, state.ActionOpenDungeonMap(1)) })

			first := state.PartyState.GetCharacter(0)
			selected := state.PartyState.GetCharacter(1)
			switch trapType {
			case TrapAcid:
				assert.Equal(t, uint16(100), first.GetCurrentHp())
				assert.Less(t, selected.GetCurrentHp(), uint16(100))
			case TrapPoison:
				assert.True(t, first.IsGood())
				assert.True(t, selected.IsPoisoned())
			case TrapBomb:
				assert.Less(t, first.GetCurrentHp(), uint16(100))
				assert.Less(t, selected.GetCurrentHp(), uint16(100))
			case TrapGas:
				assert.True(t, first.IsPoisoned())
				assert.True(t, selected.IsPoisoned())
			case TrapNone:
				t.Fatal("trap seed resolved no trap")
			default:
				t.Fatal("trap seed resolved unknown trap")
			}
			assert.Equal(t, references.DungeonCell(0x70), currentDungeonInteractionCell(t, state))
		})
	}
}

func TestDungeonGetFloorZeroEligibleGoldIsSafeAndDeterministic(t *testing.T) {
	state, callbacks := newDungeonInteractionTestState(t, 0x70, 0)
	seed := findFloorZeroGoldSeed(t, state)
	state.SetRandomSeed(seed)
	goldBefore := state.PartyState.Inventory.Gold.Get()
	callbacks.Reset()

	runDungeonInteraction(t, state, func() { assert.True(t, state.ActionGetDungeonMap()) })

	assert.Contains(t, callbacks.Messages, "0 gold!")
	assert.Equal(t, goldBefore, state.PartyState.Inventory.Gold.Get())
}

func TestDungeonGetAwardsAllLootCategoriesAndSaturates(t *testing.T) {
	state, callbacks := newDungeonInteractionTestState(t, 0x70, 0)
	state.MapState.PlayerLocation.Floor = references.DungeonFloorCount - 1
	seed := findAllDungeonLootSeed(t, state, callbacks)
	clearDungeonLootInventory(state)
	state.SetRandomSeed(seed)
	callbacks.Reset()
	callsBefore := state.GetRNG().Calls()

	state.generateDungeonChestLoot()

	assert.Positive(t, state.PartyState.Inventory.Provisions.Food.Get())
	assert.Positive(t, state.PartyState.Inventory.Gold.Get())
	assert.Positive(t, state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Positive(t, state.PartyState.Inventory.Provisions.Gems.Get())
	assert.Positive(t, state.PartyState.Inventory.Provisions.Torches.Get())
	assert.True(t, dungeonLootHasPotion(state))
	assert.True(t, dungeonLootHasScroll(state))
	assert.Equal(t, uint64(14), state.GetRNG().Calls()-callsBefore, "seven eligibility and seven award rolls")
	assert.True(t, dungeonLootMessagesContain(callbacks.Messages, " potion!"))
	assert.True(t, dungeonLootMessagesContain(callbacks.Messages, "A scroll: "))

	fillDungeonLootInventory(state)
	state.SetRandomSeed(seed)
	state.generateDungeonChestLoot()
	assert.Equal(t, party_state.ItemQuantityLargeMax, state.PartyState.Inventory.Provisions.Food.Get())
	assert.Equal(t, party_state.ItemQuantityLargeMax, state.PartyState.Inventory.Gold.Get())
	assert.Equal(t, party_state.ItemQuantitySmallMax, state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Equal(t, party_state.ItemQuantitySmallMax, state.PartyState.Inventory.Provisions.Gems.Get())
	assert.Equal(t, party_state.ItemQuantitySmallMax, state.PartyState.Inventory.Provisions.Torches.Get())
	for potion := references.Blue; potion < references.NumberOfPotions; potion++ {
		assert.Equal(t, party_state.ItemQuantitySmallMax, state.PartyState.Inventory.Potions.Get(potion))
	}
	for scroll := references.ScrollVasLor; scroll < references.NumberOfScrolls; scroll++ {
		assert.Equal(t, party_state.ItemQuantityLargeMax, state.PartyState.Inventory.Scrolls.Get(scroll))
	}
}

func TestDungeonChestMutationSurvivesSaveSnapshotRoundTrip(t *testing.T) {
	state, _ := newDungeonInteractionTestState(t, 0x48, 1)
	runDungeonInteraction(t, state, func() { assert.True(t, state.ActionOpenDungeonMap(0)) })

	hydrated := roundTripSaveSnapshot(t, state)

	assert.Equal(t, references.DungeonCell(0x78), currentDungeonInteractionCell(t, hydrated))
}

func TestDungeonChestMutationResetsOnFreshVisit(t *testing.T) {
	state, _ := newDungeonInteractionTestState(t, 0x00, 1)
	sourceCells, ok := state.GameReferences.LocationReferences.
		GetLocationReference(references.Deceit).
		CopyDungeonCells()
	require.True(t, ok)
	floor, position := firstDungeonCellOfKind(t, sourceCells, references.DungeonClosedChest)
	state.MapState.PlayerLocation.Floor = floor
	state.MapState.PlayerLocation.Position = position

	runDungeonInteraction(t, state, func() { assert.True(t, state.ActionOpenDungeonMap(0)) })
	assert.Equal(t, references.DungeonOpenChest, currentDungeonInteractionCell(t, state).Kind())

	fresh, err := state.PrepareDungeonState(
		references.Deceit,
		references.DungeonFacingEast,
		references.DungeonArrivedFromAbove,
	)
	require.NoError(t, err)
	state.installDungeonState(fresh)
	resetCell, ok := fresh.Cell(floor, position)
	require.True(t, ok)
	assert.Equal(t, references.DungeonClosedChest, resetCell.Kind())
}

func TestDungeonSearchExactPackedCellBehavior(t *testing.T) {
	t.Run("Search disarms exact floor bomb on high roll", func(t *testing.T) {
		state, callbacks := newDungeonInteractionTestState(t, 0x00, 1)
		state.PartyState.GetCharacter(0).Dexterity = 255
		setDungeonInteractionCell(t, state, references.Position{X: 2, Y: 1}, 0x62)
		callbacks.Reset()

		runDungeonInteraction(t, state, func() {
			assert.True(t, state.ActionSearchDungeonMapWithCharacter(references.Up, 0))
		})

		assert.Equal(t, references.DungeonCell(0x00), dungeonInteractionCellAt(t, state, references.Position{X: 2, Y: 1}))
		assert.Contains(t, callbacks.Messages, "Search-A bomb trap!")
	})

	t.Run("Search does not recognize ceiling-bit floor bomb", func(t *testing.T) {
		state, _ := newDungeonInteractionTestState(t, 0x00, 1)
		state.PartyState.GetCharacter(0).Dexterity = 255
		setDungeonInteractionCell(t, state, references.Position{X: 2, Y: 1}, 0x6a)

		runDungeonInteraction(t, state, func() {
			assert.False(t, state.ActionSearchDungeonMapWithCharacter(references.Up, 0))
		})

		assert.Equal(t, references.DungeonCell(0x6a), dungeonInteractionCellAt(t, state, references.Position{X: 2, Y: 1}))
	})

	t.Run("Search inspects but never disarms chest", func(t *testing.T) {
		state, callbacks := newDungeonInteractionTestState(t, 0x00, 1)
		state.PartyState.GetCharacter(0).Dexterity = 255
		setDungeonInteractionCell(t, state, references.Position{X: 2, Y: 1}, 0x47)
		callbacks.Reset()

		runDungeonInteraction(t, state, func() {
			assert.True(t, state.ActionSearchDungeonMapWithCharacter(references.Up, 0))
		})

		assert.Equal(t, references.DungeonCell(0x47), dungeonInteractionCellAt(t, state, references.Position{X: 2, Y: 1}))
		assert.Contains(t, callbacks.Messages, "Search-A simple trap")
	})

	t.Run("Search crumbles skeleton wall to wall opening", func(t *testing.T) {
		state, _ := newDungeonInteractionTestState(t, 0x00, 1)
		setDungeonInteractionCell(t, state, references.Position{X: 2, Y: 1}, 0xc8)

		runDungeonInteraction(t, state, func() {
			result := state.ResolveDungeonSearchWithCharacter(references.Up, 0)
			assert.True(t, result.Found)
			assert.True(t, result.CrumbledSkeletonWall)
			assert.False(t, result.RevealedHiddenPit)
			assert.Equal(t, references.Position{X: 2, Y: 1}, result.Target)
		})

		assert.Equal(t, references.DungeonCell(0xb8), dungeonInteractionCellAt(t, state, references.Position{X: 2, Y: 1}))
	})
}

func TestDungeonSearchHiddenPitResultAndImmediateMutation(t *testing.T) {
	for _, raw := range []references.DungeonCell{0x61, 0x69} {
		t.Run(fmt.Sprintf("0x%02X", raw.Raw()), func(t *testing.T) {
			state, _ := newDungeonInteractionTestState(t, 0x00, 1)
			target := references.Position{X: 2, Y: 1}
			floor := state.MapState.PlayerLocation.Floor
			setDungeonInteractionCell(t, state, target, raw)
			_, err := state.TransformDungeonCell(floor+1, target, func(references.DungeonCell) references.DungeonCell {
				return 0x42
			})
			require.NoError(t, err)

			for attempt := range 2 {
				runDungeonInteraction(t, state, func() {
					result := state.ResolveDungeonSearchWithCharacter(references.Up, 0)
					revealed := raw == 0x61 && attempt == 0
					assert.Equal(t, revealed, result.Found)
					assert.Equal(t, revealed, result.RevealedHiddenPit)
					assert.False(t, result.CrumbledSkeletonWall)
					assert.Equal(t, target, result.Target)

					// Both mutations must be visible before end-player processing or presentation.
					expectedCell, expectedBelow := raw, references.DungeonCell(0x42)
					if raw == 0x61 {
						expectedCell, expectedBelow = 0x60, 0x4a
					}
					assert.Equal(t, expectedCell, dungeonInteractionCellAt(t, state, target))
					below, ok := state.requireActiveDungeon().Cell(floor+1, target)
					require.True(t, ok)
					assert.Equal(t, expectedBelow, below)
				})
			}
		})
	}
}

func TestDungeonLookHidesSecretPitAndFountainsRemainRepeatable(t *testing.T) {
	state, callbacks := newDungeonInteractionTestState(t, 0x00, 1)
	character := state.PartyState.GetCharacter(0)
	character.SetStatus(party_state.Poisoned)
	target := references.Position{X: 2, Y: 1}
	setDungeonInteractionCell(t, state, target, 0x61)
	callbacks.Reset()

	runDungeonInteraction(t, state, func() {
		_, looked := state.ActionLookDungeonMap(references.Up, 0)
		assert.True(t, looked)
	})
	assert.Contains(t, callbacks.Messages, "Look-You see a passage.")
	assert.Equal(t, references.DungeonCell(0x61), dungeonInteractionCellAt(t, state, target))

	setDungeonInteractionCell(t, state, target, 0x50)
	for range 2 {
		character.SetStatus(party_state.Poisoned)
		var review DungeonLookResult
		runDungeonInteraction(t, state, func() {
			var looked bool
			review, looked = state.ActionLookDungeonMap(references.Up, 0)
			require.True(t, looked)
			require.True(t, review.Fountain)
			assert.True(t, state.ApplyDungeonFountainChoice(0, review, true).Drank)
		})
		assert.True(t, character.IsGood())
		assert.Equal(t, references.DungeonCell(0x50), dungeonInteractionCellAt(t, state, target))
	}
}

func TestDungeonLookUsesCompleteSentencesForTranscriptAndFeedback(t *testing.T) {
	for _, test := range []struct {
		name        string
		cell        references.DungeonCell
		description string
	}{
		{name: "passage", cell: 0x00, description: "a passage."},
		{name: "up ladder", cell: 0x10, description: "an up ladder."},
		{name: "down ladder", cell: 0x20, description: "a down ladder."},
		{name: "both ladders", cell: 0x30, description: "a ladder."},
		{name: "sleep field", cell: 0x80, description: "a sleep field."},
		{name: "poison field", cell: 0x81, description: "a poison gas field."},
		{name: "fire field", cell: 0x82, description: "a wall of fire."},
		{name: "electric field", cell: 0x83, description: "an electric field."},
		{name: "energy field", cell: 0x84, description: "an energy field."},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := newDungeonInteractionTestState(t, 0x00, 1)
			setDungeonInteractionCell(t, state, references.Position{X: 2, Y: 1}, test.cell)
			var feedback string
			state.SystemCallbacks.UI.ShowActionFeedback = func(message string, _ bool) { feedback = message }

			runDungeonInteraction(t, state, func() {
				_, looked := state.ActionLookDungeonMap(references.Up, 0)
				require.True(t, looked)
			})

			assert.Contains(t, callbacks.Messages, "Look-You see "+test.description)
			assert.Equal(t, "You see "+test.description, feedback)
		})
	}
}

func TestDungeonLookDarknessUsesCompleteSentence(t *testing.T) {
	state, callbacks := newDungeonInteractionTestState(t, 0x00, 1)
	state.MapState.Lighting.RestoreTorchMinutes(0)
	state.MapState.Lighting.SetMagicLight(false)
	var feedback string
	state.SystemCallbacks.UI.ShowActionFeedback = func(message string, _ bool) { feedback = message }

	assert.False(t, state.CanLookDungeonMap())
	assert.Equal(t, []string{"You see darkness."}, callbacks.Messages)
	assert.Equal(t, "You see darkness.", feedback)
}

func TestDungeonFountainFamiliesApplyExactRepeatableEffects(t *testing.T) {
	for cell := references.DungeonCell(0x50); cell <= 0x5f; cell++ {
		t.Run(fmt.Sprintf("0x%02X", cell.Raw()), func(t *testing.T) {
			state, callbacks := newDungeonInteractionTestState(t, 0x00, uint64(cell))
			character := state.PartyState.GetCharacter(0)
			target := references.Position{X: 2, Y: 1}
			setDungeonInteractionCell(t, state, target, cell)
			character.SetStatus(party_state.Good)
			character.SetHp(50)
			if cell == 0x50 {
				character.SetStatus(party_state.Poisoned)
			}
			if cell >= 0x53 {
				state.SetRandomSeed(findDungeonBadTasteSeed(t, state, cell == 0x53))
			}

			var review DungeonLookResult
			runDungeonInteraction(t, state, func() {
				var looked bool
				review, looked = state.ActionLookDungeonMap(references.Up, 0)
				require.True(t, looked)
				assert.True(t, state.ApplyDungeonFountainChoice(0, review, true).Drank)
			})

			switch cell {
			case 0x50:
				assert.True(t, character.IsGood())
				assert.Equal(t, uint16(50), character.GetCurrentHp())
				assert.Equal(t, []TransientIconKind{TransientIconPoisonCure}, callbacks.TransientIconCalls)
			case 0x51:
				assert.True(t, character.IsGood())
				assert.Equal(t, character.GetMaxHp(), character.GetCurrentHp())
				assert.Equal(t, []TransientIconKind{TransientIconHealing}, callbacks.TransientIconCalls)
			case 0x52:
				assert.True(t, character.IsPoisoned())
				assert.Equal(t, uint16(49), character.GetCurrentHp(),
					"dungeon end-player processing applies the same-cycle poison tick")
				assert.Equal(t, []TransientIconKind{TransientIconPoisonOnset}, callbacks.TransientIconCalls)
			default:
				assert.Empty(t, callbacks.TransientIconCalls)
				assert.True(t, character.IsGood())
				assert.GreaterOrEqual(t, character.GetCurrentHp(), uint16(43))
				assert.LessOrEqual(t, character.GetCurrentHp(), uint16(50))
				if cell == 0x53 {
					assert.Equal(t, uint16(50), character.GetCurrentHp(), "zero damage remains valid")
				}
			}
			assert.Equal(t, cell, dungeonInteractionCellAt(t, state, target))
		})
	}
}

func TestDungeonAutomaticBombClearsAndDamagesEveryNonDeadMember(t *testing.T) {
	state, callbacks := newDungeonInteractionTestState(t, 0x6a, 15)
	beforeHP := make(map[int]uint16)
	for index := 0; index < state.PartyState.GetCharacterCount(); index++ {
		character := state.PartyState.GetCharacter(index)
		if character.PartyMap != party_state.PartyMapActive {
			continue
		}
		character.SetMaxHp(100)
		character.SetHp(100)
		character.SetStatus(party_state.Good)
		beforeHP[index] = character.GetCurrentHp()
	}
	require.NotEmpty(t, beforeHP)
	callbacks.Reset()

	token, _ := state.BeginDungeonCommandCycle()
	state.CompleteDungeonCommandCycle(token, DungeonFullEndPlayer)

	assert.Equal(t, references.DungeonCell(0x08), currentDungeonInteractionCell(t, state))
	assert.Contains(t, callbacks.Messages, "Bomb Trap!")
	assert.Contains(t, callbacks.Messages, "KABOOM!!")
	for index, hp := range beforeHP {
		assert.Less(t, state.PartyState.GetCharacter(index).GetCurrentHp(), hp)
	}
}

func newDungeonInteractionTestState(
	t *testing.T,
	cell references.DungeonCell,
	seed uint64,
) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Deceit).
		WithFloor(0).
		WithPlayerAt(1, 1).
		WithRandomSeed(seed).
		WithSystemCallbacks().
		Build()
	require.NotNil(t, state)
	state.MapState.Lighting.RestoreTorchMinutes(10)
	character := state.PartyState.GetCharacter(0)
	character.PartyMap = party_state.PartyMapActive
	character.SetStatus(party_state.Good)
	character.SetMaxHp(100)
	character.SetHp(100)
	character.Dexterity = 30
	setDungeonInteractionCell(t, state, state.MapState.PlayerLocation.Position, cell)
	callbacks.Reset()

	return state, callbacks
}

func runDungeonInteraction(t *testing.T, state *GameState, action func()) {
	t.Helper()
	token, _ := state.BeginDungeonCommandCycle()
	action()
	state.CompleteDungeonCommandCycle(token, DungeonFullEndPlayer)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func setDungeonInteractionCell(
	t *testing.T,
	state *GameState,
	position references.Position,
	cell references.DungeonCell,
) {
	t.Helper()
	_, err := state.TransformDungeonCell(
		state.MapState.PlayerLocation.Floor,
		position,
		func(references.DungeonCell) references.DungeonCell { return cell },
	)
	require.NoError(t, err)
}

func currentDungeonInteractionCell(t *testing.T, state *GameState) references.DungeonCell {
	t.Helper()

	return dungeonInteractionCellAt(t, state, state.MapState.PlayerLocation.Position)
}

func dungeonInteractionCellAt(
	t *testing.T,
	state *GameState,
	position references.Position,
) references.DungeonCell {
	t.Helper()
	snapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	cell, ok := snapshot.Cells.Cell(state.MapState.PlayerLocation.Floor, position)
	require.True(t, ok)

	return cell
}

func findFloorZeroGoldSeed(t *testing.T, state *GameState) uint64 {
	t.Helper()
	for seed := uint64(0); seed < 1000; seed++ {
		state.SetRandomSeed(seed)
		foodRoll := state.RandomIntInRange(1, 4)
		if foodRoll >= 2 {
			state.RandomIntInRange(1, 31)
		}
		if state.RandomIntInRange(1, 4) >= 4 {
			return seed
		}
	}
	t.Fatal("no deterministic floor-zero eligible gold seed found")

	return 0
}

func findDungeonTrapSeed(t *testing.T, state *GameState, wanted TrapType) uint64 {
	t.Helper()
	for seed := uint64(0); seed < 1000; seed++ {
		state.SetRandomSeed(seed)
		result := state.ExecuteBoomForTarget(1)
		if result != nil && result.TrapType == wanted {
			return seed
		}
	}
	t.Fatalf("no deterministic seed found for trap %v", wanted)

	return 0
}

func findAllDungeonLootSeed(t *testing.T, state *GameState, callbacks *MockSystemCallbacks) uint64 {
	t.Helper()
	for seed := uint64(0); seed < 10_000; seed++ {
		clearDungeonLootInventory(state)
		state.SetRandomSeed(seed)
		callbacks.Reset()
		state.generateDungeonChestLoot()
		if state.PartyState.Inventory.Provisions.Food.HasSome() &&
			state.PartyState.Inventory.Gold.HasSome() &&
			state.PartyState.Inventory.Provisions.Keys.HasSome() &&
			state.PartyState.Inventory.Provisions.Gems.HasSome() &&
			state.PartyState.Inventory.Provisions.Torches.HasSome() &&
			dungeonLootHasPotion(state) && dungeonLootHasScroll(state) {
			return seed
		}
	}
	t.Fatal("no deterministic seed found that awards all dungeon loot categories")

	return 0
}

func clearDungeonLootInventory(state *GameState) {
	state.PartyState.Inventory.Provisions.Food.Set(0)
	state.PartyState.Inventory.Gold.Set(0)
	state.PartyState.Inventory.Provisions.Keys.Set(0)
	state.PartyState.Inventory.Provisions.Gems.Set(0)
	state.PartyState.Inventory.Provisions.Torches.Set(0)
	for potion := references.Blue; potion < references.NumberOfPotions; potion++ {
		state.PartyState.Inventory.Potions.Set(potion, 0)
	}
	for scroll := references.ScrollVasLor; scroll < references.NumberOfScrolls; scroll++ {
		state.PartyState.Inventory.Scrolls.Set(scroll, 0)
	}
}

func fillDungeonLootInventory(state *GameState) {
	state.PartyState.Inventory.Provisions.Food.Set(party_state.ItemQuantityLargeMax)
	state.PartyState.Inventory.Gold.Set(party_state.ItemQuantityLargeMax)
	state.PartyState.Inventory.Provisions.Keys.Set(party_state.ItemQuantitySmallMax)
	state.PartyState.Inventory.Provisions.Gems.Set(party_state.ItemQuantitySmallMax)
	state.PartyState.Inventory.Provisions.Torches.Set(party_state.ItemQuantitySmallMax)
	for potion := references.Blue; potion < references.NumberOfPotions; potion++ {
		state.PartyState.Inventory.Potions.Set(potion, party_state.ItemQuantitySmallMax)
	}
	for scroll := references.ScrollVasLor; scroll < references.NumberOfScrolls; scroll++ {
		state.PartyState.Inventory.Scrolls.Set(scroll, party_state.ItemQuantityLargeMax)
	}
}

func dungeonLootHasPotion(state *GameState) bool {
	for potion := references.Blue; potion < references.NumberOfPotions; potion++ {
		if state.PartyState.Inventory.Potions.Get(potion) > 0 {
			return true
		}
	}

	return false
}

func dungeonLootHasScroll(state *GameState) bool {
	for scroll := references.ScrollVasLor; scroll < references.NumberOfScrolls; scroll++ {
		if state.PartyState.Inventory.Scrolls.Get(scroll) > 0 {
			return true
		}
	}

	return false
}

func dungeonLootMessagesContain(messages []string, text string) bool {
	for _, message := range messages {
		if strings.Contains(message, text) {
			return true
		}
	}

	return false
}

func firstDungeonCellOfKind(
	t *testing.T,
	cells references.DungeonCells,
	wanted references.DungeonCellKind,
) (references.FloorNumber, references.Position) {
	t.Helper()
	for floor := references.FloorNumber(0); floor < references.DungeonFloorCount; floor++ {
		for y := range references.YDungeonMapTiles {
			for x := range references.XDungeonMapTiles {
				position := references.Position{X: x, Y: y}
				cell, ok := cells.Cell(floor, position)
				if ok && cell.Kind() == wanted {
					return floor, position
				}
			}
		}
	}
	t.Fatalf("dungeon source has no cell kind %v", wanted)

	return 0, references.Position{}
}

func findDungeonBadTasteSeed(t *testing.T, state *GameState, zeroDamage bool) uint64 {
	t.Helper()
	for seed := uint64(0); seed < 1000; seed++ {
		state.SetRandomSeed(seed)
		damage := state.RandomIntInRange(0, 7)
		if (damage == 0) == zeroDamage {
			return seed
		}
	}
	t.Fatal("no deterministic bad-taste fountain seed found")

	return 0
}
