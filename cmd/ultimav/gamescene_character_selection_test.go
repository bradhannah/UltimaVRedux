package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestCharacterSelectionCasterProfileExplainsEveryActor(t *testing.T) {
	scene := newCharacterSelectionProfileScene(t)
	avatar := scene.gameState.PartyState.GetCharacter(0)
	avatar.Class = party_state.Wizard
	avatar.SetMp(8)
	iolo := scene.gameState.PartyState.GetCharacter(1)
	iolo.Class = party_state.Fighter
	shamino := scene.gameState.PartyState.GetCharacter(2)
	shamino.Class = party_state.Wizard
	shamino.SetStatus(party_state.Sleep)

	options := scene.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionCaster})

	require.Len(t, options, 3)
	assert.Equal(t, widgets.CharacterSelectionAvailable, selectionOption(t, options, 0).State)
	assert.Equal(t, widgets.CharacterSelectionUnavailable, selectionOption(t, options, 1).State)
	assert.Contains(t, selectionOption(t, options, 1).Details[0], "no magical ability")
	assert.Equal(t, widgets.CharacterSelectionUnavailable, selectionOption(t, options, 2).State)
	assert.Equal(t, "Asleep", selectionOption(t, options, 2).Status)
}

func TestHealingChoicesExposeCurrentHealthWithoutChangingEligibility(t *testing.T) {
	scene := newCharacterSelectionProfileScene(t)
	character := scene.gameState.PartyState.GetAvatar()
	character.SetHp(character.GetMaxHp())
	character.SubtractHp(17)
	options := scene.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionSpellTarget, Spell: references.Mani})
	health := selectionOption(t, options, 0).Health
	require.NotNil(t, health)
	require.EqualValues(t, 17, health.Missing)
	require.Equal(t, character.GetCurrentHp(), health.Current)
	character.AddHp(17)
	require.EqualValues(t, 17, health.Missing, "display snapshot must remain detached")
	caster := scene.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionCaster})
	require.Nil(t, selectionOption(t, caster, 0).Health)
}

func TestCharacterSelectionSpellProfilesKeepIneffectiveTargetsConfirmable(t *testing.T) {
	scene := newCharacterSelectionProfileScene(t)
	scene.gameState.PartyState.GetCharacter(0).SetStatus(party_state.Poisoned)
	scene.gameState.PartyState.GetCharacter(1).SetStatus(party_state.Good)
	scene.gameState.PartyState.GetCharacter(2).SetStatus(party_state.Dead)

	options := scene.characterSelectionOptions(characterSelectionContext{
		Profile: characterSelectionSpellTarget,
		Spell:   references.AnNox,
	})

	assert.Equal(t, widgets.CharacterSelectionAvailable, selectionOption(t, options, 0).State)
	assert.Equal(t, widgets.CharacterSelectionWarning, selectionOption(t, options, 1).State)
	assert.Equal(t, widgets.CharacterSelectionWarning, selectionOption(t, options, 2).State)
	assert.Contains(t, selectionOption(t, options, 1).Details[1], "Confirmation consumes")
}

func TestCharacterSelectionStatusProjectionPreservesSpellEligibility(t *testing.T) {
	scene := newCharacterSelectionProfileScene(t)
	character := scene.gameState.PartyState.GetCharacter(1)
	for _, status := range []party_state.CharacterStatus{
		party_state.Good, party_state.Poisoned, party_state.Sleep, party_state.Charmed, party_state.Dead,
	} {
		for _, spell := range []references.Spell{references.AnNox, references.AnZu} {
			character.SetStatus(status)
			options := scene.characterSelectionOptions(characterSelectionContext{
				Profile: characterSelectionSpellTarget, Spell: spell,
			})
			option := selectionOption(t, options, 1)
			assert.Equal(t, status, option.StatusKind)
			assert.Equal(t, characterSelectionStatus(status), option.Status)
			wantState := widgets.CharacterSelectionWarning
			if (spell == references.AnNox && status == party_state.Poisoned) ||
				(spell == references.AnZu && status == party_state.Sleep) {
				wantState = widgets.CharacterSelectionAvailable
			}
			assert.Equal(t, wantState, option.State)
			character.SetStatus(party_state.Good)
			assert.Equal(t, status, option.StatusKind, "presentation is detached from subsequent status changes")
		}
	}
	assert.Empty(t, characterSelectionNoOneOption().Status)
}

func TestCharacterSelectionPotionProfileAllowsSleepingBlueTargetAndDefersConsumption(t *testing.T) {
	scene := newCharacterSelectionProfileScene(t)
	scene.gameState.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	scene.gameState.PartyState.GetCharacter(1).SetStatus(party_state.Sleep)
	scene.gameState.PartyState.GetCharacter(2).SetStatus(party_state.Dead)

	options := scene.characterSelectionOptions(characterSelectionContext{
		Profile: characterSelectionPotionTarget,
		Potion:  references.Blue,
	})

	assert.Equal(t, widgets.CharacterSelectionWarning, selectionOption(t, options, 0).State)
	assert.Equal(t, widgets.CharacterSelectionAvailable, selectionOption(t, options, 1).State)
	assert.Equal(t, widgets.CharacterSelectionWarning, selectionOption(t, options, 2).State)
	assert.Contains(t, selectionOption(t, options, 1).Details[1], "confirmed")
}

func TestCharacterSelectionPotionProfileWarnsAboutCombatOnlyFieldEffects(t *testing.T) {
	scene := newCharacterSelectionProfileScene(t)

	for _, potion := range []references.Potion{references.Purple, references.Black} {
		options := scene.characterSelectionOptions(characterSelectionContext{
			Profile: characterSelectionPotionTarget,
			Potion:  potion,
		})

		assert.Equal(t, widgets.CharacterSelectionWarning, selectionOption(t, options, 0).State)
		assert.Contains(t, selectionOption(t, options, 0).Details[0], "no noticeable effect outside combat")
	}
}

func TestCharacterSelectionGuardAndCrystalBallProfilesExposeConsequences(t *testing.T) {
	scene := newCharacterSelectionProfileScene(t)
	scene.gameState.PartyState.GetCharacter(0).SetStatus(party_state.Poisoned)
	scene.gameState.PartyState.GetCharacter(1).SetStatus(party_state.Sleep)
	viewer := scene.gameState.PartyState.GetCharacter(2)
	viewer.SetHp(1)

	guard := scene.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionGuard})
	assert.Equal(t, widgets.CharacterSelectionWarning, selectionOption(t, guard, 0).State)
	assert.Equal(t, []string{
		"A poisoned member cannot keep watch.", "Choosing them camps without a watch.",
	}, selectionOption(t, guard, 0).Details)
	assert.Equal(t, widgets.CharacterSelectionUnavailable, selectionOption(t, guard, 1).State)

	crystal := scene.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionCrystalBall})
	assert.Equal(t, widgets.CharacterSelectionWarning, selectionOption(t, crystal, 2).State)
	assert.Contains(t, selectionOption(t, crystal, 2).Details[1], "lethal")
	assert.Equal(t, "Intelligence", selectionOption(t, crystal, 2).Facts[0].Label)
}

func TestCharacterSelectionOpenJimmyAndSearchProfilesShowOnlyGoverningFacts(t *testing.T) {
	scene := newCharacterSelectionProfileScene(t)
	character := scene.gameState.PartyState.GetCharacter(0)
	character.Dexterity = 23

	open := selectionOption(t, scene.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionOpen}), 0)
	jimmy := selectionOption(t, scene.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionJimmy}), 0)
	search := selectionOption(t, scene.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionSearch}), 0)

	require.Len(t, open.Facts, 1)
	assert.Equal(t, "HP", open.Facts[0].Label)
	assert.Equal(t, "Dexterity", jimmy.Facts[0].Label)
	assert.Equal(t, "23", jimmy.Facts[0].Value)
	assert.Equal(t, "Intelligence", search.Facts[0].Label)
	assert.NotContains(t, open.Details[0], "Dexterity")
}

func TestSurfaceSearchUsesEligibleActiveActorWithoutSelection(t *testing.T) {
	scene, callbacks, direction, seed := newSurfaceSearchActorScene(t)
	activateSurfaceSearchActor(scene, 0, 0)
	activateSurfaceSearchActor(scene, 1, 255)
	require.True(t, scene.gameState.SetActivePlayer(1))
	scene.gameState.SetRandomSeed(seed)

	state := scene.surfaceSearchSecondaryWithDialog(direction, func(surfaceCommandState) {})

	assert.Equal(t, surfaceCommandCompleted, state)
	assert.Nil(t, scene.dialogStack.PeekTopModalDialog())
	assert.Equal(t, []string{"Thou dost find a complex trap!"}, callbacks.Messages)
	assert.Equal(t, []int{1}, callbacks.TimeAdvanced())
}

func TestSurfaceSearchWithoutActiveActorUsesSelectedDialogActor(t *testing.T) {
	scene, callbacks, direction, seed := newSurfaceSearchActorScene(t)
	activateSurfaceSearchActor(scene, 0, 0)
	activateSurfaceSearchActor(scene, 1, 255)
	scene.gameState.ClearActivePlayer()
	scene.gameState.SetRandomSeed(seed)
	completionCalls := 0
	completedState := surfaceCommandCancelled

	state := scene.surfaceSearchSecondaryWithDialog(direction, func(state surfaceCommandState) {
		completionCalls++
		completedState = state
		scene.completeSurfaceCommand(newSurfaceCommandResult(state, references.SmallMapType))
	})

	assert.Equal(t, surfaceCommandDeferred, state)
	assert.Empty(t, callbacks.Messages)
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	assert.Equal(t, "Search: Choose Character", dialog.GetTitle())
	dialog.HandleKeyDown()
	assert.Equal(t, 1, dialog.GetCurrentSelection())
	dialog.HandleEnter()

	assert.Equal(t, 1, completionCalls)
	assert.Equal(t, surfaceCommandCompleted, completedState)
	assert.Equal(t, []string{"Thou dost find a complex trap!"}, callbacks.Messages)
	assert.Equal(t, []int{1}, callbacks.TimeAdvanced())
	assert.Zero(t, scene.dialogStack.Count())
}

func TestSurfaceSearchClearsNewlyIneligibleActorAndUsesFallbackSelection(t *testing.T) {
	scene, callbacks, direction, seed := newSurfaceSearchActorScene(t)
	activateSurfaceSearchActor(scene, 0, 255)
	activateSurfaceSearchActor(scene, 1, 0)
	activateSurfaceSearchActor(scene, 2, 0)
	require.True(t, scene.gameState.SetActivePlayer(1))
	scene.gameState.PartyState.GetCharacter(1).SetStatus(party_state.Sleep)
	scene.gameState.SetRandomSeed(seed)
	completedState := surfaceCommandCancelled

	state := scene.surfaceSearchSecondaryWithDialog(direction, func(state surfaceCommandState) {
		completedState = state
		scene.completeSurfaceCommand(newSurfaceCommandResult(state, references.SmallMapType))
	})

	assert.Equal(t, surfaceCommandDeferred, state)
	assert.Equal(t, -1, scene.gameState.GetActivePlayerIndex())
	assert.Empty(t, callbacks.Messages)
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	assert.Equal(t, widgets.CharacterSelectionUnavailable, selectionOption(t, dialog.GetOptions(), 1).State)
	assert.Equal(t, 0, dialog.GetCurrentSelection())
	dialog.HandleEnter()

	assert.Equal(t, surfaceCommandCompleted, completedState)
	assert.Equal(t, []string{"Thou dost find a complex trap!"}, callbacks.Messages)
	assert.Equal(t, []int{1}, callbacks.TimeAdvanced())
	assert.Zero(t, scene.dialogStack.Count())
}

func TestSurfaceSearchClearsCharmedActiveActorAndUsesFallbackSelection(t *testing.T) {
	scene, _, direction, seed := newSurfaceSearchActorScene(t)
	activateSurfaceSearchActor(scene, 0, 255)
	activateSurfaceSearchActor(scene, 1, 0)
	activateSurfaceSearchActor(scene, 2, 0)
	require.True(t, scene.gameState.SetActivePlayer(1))
	scene.gameState.PartyState.GetCharacter(1).SetStatus(party_state.Charmed)
	scene.gameState.SetRandomSeed(seed)

	state := scene.surfaceSearchSecondaryWithDialog(direction, func(surfaceCommandState) {})

	assert.Equal(t, surfaceCommandDeferred, state)
	assert.Equal(t, -1, scene.gameState.GetActivePlayerIndex())
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	assert.Equal(t, widgets.CharacterSelectionUnavailable, selectionOption(t, dialog.GetOptions(), 1).State)
}

func newSurfaceSearchActorScene(
	t *testing.T,
) (*GameScene, *game_state.MockSystemCallbacks, references.Direction, uint64) {
	t.Helper()
	seed := surfaceSearchLowIntelligenceSeed(t)
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithFloor(references.MainFloor).
		WithRandomSeed(seed).
		WithSystemCallbacks().
		Build()
	for index := range party_state.NRosterSlots {
		character := state.PartyState.GetCharacter(index)
		if character != nil {
			character.PartyMap = party_state.PartyMapUnrecruited
		}
	}
	for _, unit := range *state.CurrentNPCAIController.GetNpcs() {
		chest, ok := unit.(*map_units.NPCFriendly)
		if !ok || !chest.IsSurfaceChest() {
			continue
		}
		chest.RestoreSurfaceChestNumber(0x80 | 30)
		state.MapState.PlayerLocation.Floor = chest.Floor()
		if chest.Pos().X > 0 {
			state.MapState.PlayerLocation.Position = references.Position{X: chest.Pos().X - 1, Y: chest.Pos().Y}
			callbacks.Reset()

			return newSurfaceCompletionTestScene(state), callbacks, references.Right, seed
		}
		state.MapState.PlayerLocation.Position = references.Position{X: chest.Pos().X + 1, Y: chest.Pos().Y}
		callbacks.Reset()

		return newSurfaceCompletionTestScene(state), callbacks, references.Left, seed
	}
	t.Fatal("Lord British's Castle fixture has no searchable surface chest")

	return nil, nil, references.NoneDirection, 0
}

func activateSurfaceSearchActor(scene *GameScene, index int, intelligence byte) {
	character := scene.gameState.PartyState.GetCharacter(index)
	character.PartyMap = party_state.PartyMapActive
	character.SetStatus(party_state.Good)
	character.Intelligence = intelligence
}

func surfaceSearchLowIntelligenceSeed(t *testing.T) uint64 {
	t.Helper()
	for seed := uint64(1); seed < 1000; seed++ {
		if beradrng.New(seed).Intn(32)+1 < 31 {
			return seed
		}
	}
	t.Fatal("no deterministic low-intelligence Search seed found")

	return 0
}

func newCharacterSelectionProfileScene(t *testing.T) *GameScene {
	t.Helper()
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	for index := 0; index < 3; index++ {
		state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapActive
		state.PartyState.GetCharacter(index).SetStatus(party_state.Good)
	}
	for index := 3; index < party_state.MaxActiveParty; index++ {
		state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
	}

	return &GameScene{gameState: state}
}

func selectionOption(
	t *testing.T,
	options []widgets.CharacterSelectionOption,
	characterIndex int,
) widgets.CharacterSelectionOption {
	t.Helper()
	for _, option := range options {
		if option.CharacterIndex == characterIndex {
			return option
		}
	}
	t.Fatalf("character %d not found", characterIndex)

	return widgets.CharacterSelectionOption{}
}
