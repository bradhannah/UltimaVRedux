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
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestProductionCastPathCreatesAndOwnsSpellCastingDialog(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	require.True(t, state.SetActivePlayer(0))
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
	}
	beforeTime := state.DateTime
	beforeEffect := state.ActiveDurationEffect
	scene := &GameScene{
		gameState:         state,
		output:            text.NewOutput(nil, 1, 10, 80),
		keyboard:          input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		spriteSheet:       &sprites.SpriteSheet{},
		autoOpenDoorState: &AutoOpenDoorState{},
	}

	scene.smallMapInputHandler(ebiten.KeyC)
	require.True(t, state.EventQueueManager.HasWorkflow("spell_casting_workflow"))

	var spellDialog *widgets.SpellCastingDialog
	for range 10 {
		state.ElapsedMs += DialogPollingRateMs
		state.EventQueueManager.Update()
		if dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SpellCastingDialog); ok {
			spellDialog = dialog
			break
		}
	}
	require.NotNil(t, spellDialog, "normal Cast input must create the production spell dialog")
	assert.Same(t, spellDialog, scene.dialogStack.PeekTopModalDialog())
	assert.Equal(t, 1, scene.dialogStack.Count(), "the workflow must own exactly one modal instance")

	spellDialog.Dismiss()
	for range 10 {
		state.ElapsedMs += DialogPollingRateMs
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow("spell_casting_workflow") {
			break
		}
	}

	assert.False(t, state.EventQueueManager.HasWorkflow("spell_casting_workflow"))
	assert.Equal(t, 0, scene.dialogStack.Count(), "workflow cleanup must remove its owned dialog instance")
	assert.Equal(t, beforeTime, state.DateTime, "canceling Cast is free")
	assert.Equal(t, beforeEffect, state.ActiveDurationEffect)
	assert.False(t, scene.surfaceSpellWorkflowOwnsCycle)
}

func TestProductionKeyboardCastConsumesResourcesAndExactlyOneMinute(t *testing.T) {
	state, mock := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	require.True(t, state.SetActivePlayer(0))
	caster := state.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.SetMp(8)
	state.PartyState.Inventory.Spells.Set(references.InLor, 2)
	beforeTime := state.DateTime
	expectedTime := beforeTime
	expectedTime.Advance(1)
	mock.Reset()
	scene := newProductionCastTestScene(state)

	scene.smallMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	assert.Equal(t, references.InLor, spellDialog.GetSelectedSpell())
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	spellDialog.Update()
	require.True(t, spellDialog.HasPendingCastRequest(), "Enter must publish the dialog's production CastRequest")
	drainProductionCastWorkflow(t, scene)

	assert.Equal(t, expectedTime, state.DateTime, "the keyboard command and workflow must own one minute total")
	assert.Equal(t, []int{1}, mock.TimeAdvanced())
	assert.Equal(t, byte(7), caster.GetCurrentMp())
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.InLor))
}

func TestProductionCombatCastDialogCancellationPreservesAction(t *testing.T) {
	actual, actualMock := newProductionCombatCastState(t)
	expected, _ := newProductionCombatCastState(t)
	casterIdx := actual.CombatScene.GetActiveCombatantIndex()
	caster := actual.PartyState.GetCharacter(casterIdx)
	mpBefore := caster.GetCurrentMp()
	spellsBefore := actual.PartyState.Inventory.Spells.Get(references.InXenMani)
	actualMock.Reset()
	scene := newProductionCastTestScene(actual)

	scene.combatMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	spellDialog.Dismiss()
	drainProductionCastWorkflow(t, scene)

	assertCombatMatchesSingleFinish(t, actual, expected)
	assert.Equal(t, mpBefore, caster.GetCurrentMp())
	assert.Equal(t, spellsBefore, actual.PartyState.Inventory.Spells.Get(references.InXenMani))
	assert.Empty(t, actualMock.CombatEvents)
}

func TestProductionCombatAutomaticCastTerminalMatrix(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*game_state.GameState)
		wantMP     byte
		wantSpells uint16
	}{
		{name: "none mixed", mutate: func(state *game_state.GameState) {
			state.PartyState.Inventory.Spells.Set(references.InXenMani, 0)
		}, wantMP: 8, wantSpells: 0},
		{name: "MP too low", mutate: func(state *game_state.GameState) {
			state.PartyState.GetCharacter(state.CombatScene.GetActiveCombatantIndex()).SetMp(1)
		}, wantMP: 1, wantSpells: 1},
		{name: "level too low", mutate: func(state *game_state.GameState) {
			state.PartyState.GetCharacter(state.CombatScene.GetActiveCombatantIndex()).Level = 1
		}, wantMP: 8, wantSpells: 1},
		{name: "negate absorption", mutate: func(state *game_state.GameState) {
			state.ActivateNegateMagicEffect(10)
		}, wantMP: 8, wantSpells: 1},
		{name: "Palace absorption", mutate: func(state *game_state.GameState) {
			state.CombatScene.PreCombatPlayerLocation.Location = references.PalaceOfBlackthorn
		}, wantMP: 8, wantSpells: 1},
		{name: "success", wantMP: 6, wantSpells: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, actualMock := newProductionCombatCastState(t)
			expected, _ := newProductionCombatCastState(t)
			casterIdx := actual.CombatScene.GetActiveCombatantIndex()
			actualMock.Reset()
			scene := newProductionCastTestScene(actual)

			scene.combatMapInputHandler(ebiten.KeyC)
			spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			for _, key := range []ebiten.Key{ebiten.KeyI, ebiten.KeyX, ebiten.KeyM} {
				scene.keyboard.SimulateKeyPress(key)
				spellDialog.Update()
			}
			require.Equal(t, references.InXenMani, spellDialog.GetSelectedSpell())
			if test.mutate != nil {
				test.mutate(actual)
				test.mutate(expected)
			}
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			spellDialog.Update()
			if test.name == "none mixed" || test.name == "MP too low" || test.name == "level too low" {
				for range 10 {
					actual.ElapsedMs += 100
					actual.EventQueueManager.Update()
				}
				require.Same(t, spellDialog, scene.dialogStack.PeekTopModalDialog())
				scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
				spellDialog.Update()
			}
			drainProductionCastWorkflow(t, scene)
			if test.name == "success" {
				expected.FinishCombatAction()
			}

			assertCombatMatchesSingleFinish(t, actual, expected)
			caster := actual.PartyState.GetCharacter(casterIdx)
			assert.Equal(t, test.wantMP, caster.GetCurrentMp())
			assert.Equal(t, test.wantSpells, actual.PartyState.Inventory.Spells.Get(references.InXenMani))
			assert.Empty(t, actualMock.CombatEvents)
		})
	}
}

func TestProductionCombatPreRequestFailurePreservesAction(t *testing.T) {
	tests := []struct {
		name string
		keys []ebiten.Key
		prep func(*game_state.GameState)
	}{
		{
			name: "invalid spell name",
			keys: []ebiten.Key{ebiten.KeyB, ebiten.KeyB, ebiten.KeyB},
		},
		{
			name: "illegal context",
			keys: []ebiten.Key{ebiten.KeyI, ebiten.KeyL},
			prep: func(state *game_state.GameState) {
				state.PartyState.Inventory.Spells.Set(references.InLor, 1)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, actualMock := newProductionCombatCastState(t)
			expected, _ := newProductionCombatCastState(t)
			if test.prep != nil {
				test.prep(actual)
				test.prep(expected)
			}
			casterIdx := actual.CombatScene.GetActiveCombatantIndex()
			caster := actual.PartyState.GetCharacter(casterIdx)
			mpBefore := caster.GetCurrentMp()
			actualMock.Reset()
			scene := newProductionCastTestScene(actual)

			scene.combatMapInputHandler(ebiten.KeyC)
			spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			for _, key := range test.keys {
				scene.keyboard.SimulateKeyPress(key)
				spellDialog.Update()
			}
			if test.name == "invalid spell name" {
				require.Equal(t, references.InvalidSpell, spellDialog.GetSelectedSpell())
			} else {
				require.Equal(t, references.InLor, spellDialog.GetSelectedSpell())
			}
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			spellDialog.Update()
			for range 10 {
				actual.ElapsedMs += 100
				actual.EventQueueManager.Update()
			}
			require.Same(t, spellDialog, scene.dialogStack.PeekTopModalDialog())
			scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
			spellDialog.Update()
			drainProductionCastWorkflow(t, scene)

			assertCombatMatchesSingleFinish(t, actual, expected)
			assert.Equal(t, mpBefore, caster.GetCurrentMp())
			assert.Equal(t, uint16(1), actual.PartyState.Inventory.Spells.Get(references.InXenMani))
			assert.Empty(t, actualMock.CombatEvents)
		})
	}
}

func TestProductionCombatPartyTargetNoEffectAndSuccessConsumeResourcesAndOneAction(t *testing.T) {
	for _, validTarget := range []bool{false, true} {
		name := "full health legal target"
		if validTarget {
			name = "success"
		}
		t.Run(name, func(t *testing.T) {
			actual, actualMock := newProductionCombatCastState(t)
			expected, _ := newProductionCombatCastState(t)
			casterIdx := actual.CombatScene.GetActiveCombatantIndex()
			caster := actual.PartyState.GetCharacter(casterIdx)
			actual.PartyState.Inventory.Spells.Set(references.Mani, 1)
			mpBefore := caster.GetCurrentMp()
			actualMock.Reset()
			scene := newProductionCastTestScene(actual)

			scene.combatMapInputHandler(ebiten.KeyC)
			spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			scene.keyboard.SimulateKeyPress(ebiten.KeyM)
			spellDialog.Update()
			require.Equal(t, references.Mani, spellDialog.GetSelectedSpell())
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			spellDialog.Update()
			waitForProductionPartySpellTargeting(t, scene)
			targetIdx := actual.CombatScene.SpellTargeting.Recipient.EntityIndex
			target := actual.PartyState.GetCharacter(targetIdx)
			target.SetHp(target.GetMaxHp())
			if validTarget {
				target.SubtractHp(30)
			}
			hpBefore := target.GetCurrentHp()
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			scene.handleCombatSpellTargetInput()
			drainProductionCastWorkflow(t, scene)
			expected.FinishCombatAction()

			assertCombatMatchesSingleFinish(t, actual, expected)
			assert.Equal(t, mpBefore-1, caster.GetCurrentMp())
			assert.Zero(t, actual.PartyState.Inventory.Spells.Get(references.Mani))
			if validTarget {
				assert.Greater(t, target.GetCurrentHp(), hpBefore)
			} else {
				assert.Equal(t, hpBefore, target.GetCurrentHp())
			}
			assert.Empty(t, actualMock.CombatEvents)
		})
	}
}

func TestProductionCombatPartyTargetCancellationPreservesResourcesAndAction(t *testing.T) {
	actual, _ := newProductionCombatCastState(t)
	expected, _ := newProductionCombatCastState(t)
	casterIdx := actual.CombatScene.GetActiveCombatantIndex()
	caster := actual.PartyState.GetCharacter(casterIdx)
	actual.PartyState.Inventory.Spells.Set(references.Mani, 1)
	mpBefore := caster.GetCurrentMp()
	scene := newProductionCastTestScene(actual)

	scene.combatMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	scene.keyboard.SimulateKeyPress(ebiten.KeyM)
	spellDialog.Update()
	require.Equal(t, references.Mani, spellDialog.GetSelectedSpell())
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	spellDialog.Update()
	waitForProductionPartySpellTargeting(t, scene)
	assert.Equal(t, mpBefore, caster.GetCurrentMp())
	assert.Equal(t, uint16(1), actual.PartyState.Inventory.Spells.Get(references.Mani))
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	scene.handleCombatSpellTargetInput()
	drainProductionCastWorkflow(t, scene)

	assertCombatMatchesSingleFinish(t, actual, expected)
	assert.Equal(t, mpBefore, caster.GetCurrentMp())
	assert.Equal(t, uint16(1), actual.PartyState.Inventory.Spells.Get(references.Mani))
}

func TestProductionCastShutdownRemovesOpenSpellDialog(t *testing.T) {
	state, _ := newProductionCombatCastState(t)
	scene := newProductionCastTestScene(state)

	scene.combatMapInputHandler(ebiten.KeyC)
	waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	require.NotZero(t, scene.dialogStack.Count())

	require.NotPanics(t, state.EventQueueManager.Shutdown)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestProductionCombatVasManiRejectsBeforeTargetWithoutCost(t *testing.T) {
	actual, actualMock := newProductionCombatCastState(t)
	expected, _ := newProductionCombatCastState(t)
	actual.CombatScene.Victory = false
	expected.CombatScene.Victory = false
	casterIdx := actual.CombatScene.GetActiveCombatantIndex()
	caster := actual.PartyState.GetCharacter(casterIdx)
	caster.SetHp(caster.GetMaxHp())
	caster.SubtractHp(1)
	actual.PartyState.Inventory.Spells.Set(references.VasMani, 1)
	mpBefore := caster.GetCurrentMp()
	actualMock.Reset()
	scene := newProductionCastTestScene(actual)

	scene.combatMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	for _, key := range []ebiten.Key{ebiten.KeyV, ebiten.KeyM} {
		scene.keyboard.SimulateKeyPress(key)
		spellDialog.Update()
	}
	require.Equal(t, references.VasMani, spellDialog.GetSelectedSpell())
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	spellDialog.Update()
	for range 10 {
		actual.ElapsedMs += 100
		actual.EventQueueManager.Update()
	}
	require.Same(t, spellDialog, scene.dialogStack.PeekTopModalDialog())
	require.False(t, spellDialog.HasPendingCastRequest())
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	spellDialog.Update()
	drainProductionCastWorkflow(t, scene)

	assertCombatMatchesSingleFinish(t, actual, expected)
	assert.Equal(t, mpBefore, caster.GetCurrentMp())
	assert.Equal(t, uint16(1), actual.PartyState.Inventory.Spells.Get(references.VasMani))
	for _, event := range actualMock.CombatEvents {
		assert.NotEqual(t, game_state.CombatEventSpell, event.Kind)
	}
}

func TestProductionVasRelPorPhaseWorkflowCommitsOnlyValidSelection(t *testing.T) {
	tests := []struct {
		name             string
		phaseKey         ebiten.Key
		moonstoneState   game_state.MoonstoneState
		moonstoneMap     uint8
		wantMP           byte
		wantSpells       uint16
		wantTimeAdvanced []int
		wantLocation     references.Location
		wantPosition     references.Position
	}{
		{
			name:             "cancel",
			phaseKey:         ebiten.KeyEscape,
			moonstoneState:   game_state.MoonstoneStateBuried,
			moonstoneMap:     uint8(references.Britain),
			wantMP:           12,
			wantSpells:       2,
			wantLocation:     references.BritanniaUnderworld,
			wantPosition:     references.Position{X: 10, Y: 10},
			wantTimeAdvanced: nil,
		},
		{
			name:             "unavailable phase",
			phaseKey:         ebiten.Key3,
			moonstoneState:   game_state.MoonstoneStateInInventory,
			moonstoneMap:     0xFF,
			wantMP:           12,
			wantSpells:       2,
			wantLocation:     references.BritanniaUnderworld,
			wantPosition:     references.Position{X: 10, Y: 10},
			wantTimeAdvanced: nil,
		},
		{
			name:             "valid phase",
			phaseKey:         ebiten.Key3,
			moonstoneState:   game_state.MoonstoneStateBuried,
			moonstoneMap:     uint8(references.Britain),
			wantMP:           4,
			wantSpells:       1,
			wantLocation:     references.Britain,
			wantPosition:     references.Position{X: 14, Y: 15},
			wantTimeAdvanced: []int{game_state.DefaultLargeMapMinutesPerTurn},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, mock := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithPlayerAt(10, 10).
				WithSystemCallbacks().
				Build()
			require.True(t, state.SetActivePlayer(0))
			caster := state.PartyState.GetCharacter(0)
			caster.Class = party_state.Wizard
			caster.Level = 8
			caster.SetMp(12)
			state.PartyState.Inventory.Spells.Set(references.VasRelPor, 2)
			stone := state.GetMoonstoneSystem().GetMoonstoneByIndex(int(datetime.MoonPhaseFirstQuarter))
			stone.X, stone.Y = 14, 15
			stone.Map = test.moonstoneMap
			stone.Level = 1
			stone.State = test.moonstoneState
			beforeTime := state.DateTime
			mock.Reset()
			scene := newProductionCastTestScene(state)

			scene.smallMapInputHandler(ebiten.KeyC)
			spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			for _, key := range []ebiten.Key{ebiten.KeyV, ebiten.KeyR, ebiten.KeyP} {
				scene.keyboard.SimulateKeyPress(key)
				spellDialog.Update()
			}
			require.Equal(t, references.VasRelPor, spellDialog.GetSelectedSpell())
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			spellDialog.Update()
			phaseDialog := waitForTopModal[*widgets.InputBox](t, scene)
			scene.keyboard.SimulateKeyPress(test.phaseKey)
			phaseDialog.Update()
			if test.phaseKey != ebiten.KeyEscape {
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
				phaseDialog.Update()
			}
			drainProductionCastWorkflow(t, scene)

			assert.Equal(t, test.wantMP, caster.GetCurrentMp())
			assert.Equal(t, test.wantSpells, state.PartyState.Inventory.Spells.Get(references.VasRelPor))
			if len(test.wantTimeAdvanced) == 0 {
				assert.Empty(t, mock.TimeAdvanced())
			} else {
				assert.Equal(t, test.wantTimeAdvanced, mock.TimeAdvanced())
			}
			assert.Equal(t, test.wantLocation, state.MapState.PlayerLocation.Location)
			assert.Equal(t, test.wantPosition, state.MapState.PlayerLocation.Position)
			if len(test.wantTimeAdvanced) == 0 {
				assert.Equal(t, beforeTime, state.DateTime)
			} else {
				expectedTime := beforeTime
				expectedTime.Advance(game_state.DefaultLargeMapMinutesPerTurn)
				assert.Equal(t, expectedTime, state.DateTime)
			}
			assert.False(t, scene.surfaceSpellWorkflowOwnsCycle)
		})
	}
}

func TestProductionCastAbsorptionUsesValidationFeedbackWithoutConsumption(t *testing.T) {
	state, mock := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Stonegate).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	require.True(t, state.SetActivePlayer(0))
	caster := state.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.SetMp(8)
	state.PartyState.Inventory.Spells.Set(references.InLor, 1)
	beforeTime := state.DateTime
	mock.Reset()
	scene := newProductionCastTestScene(state)

	scene.smallMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	spellDialog.Update()
	drainProductionCastWorkflow(t, scene)

	assert.Equal(t, beforeTime, state.DateTime)
	assert.Empty(t, mock.TimeAdvanced())
	assert.Equal(t, byte(8), caster.GetCurrentMp())
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.InLor))
	assert.Contains(t, mock.SoundEffectsPlayed, game_state.SoundAbsorb)
}

func TestSpellCapabilityDistinguishesClassFromCurrentMP(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	character := state.PartyState.GetCharacter(0)
	character.SetMp(0)

	character.Class = party_state.Fighter
	assert.False(t, isSpellCapable(character), "fighters never gain spellcasting ability")

	character.Class = party_state.Wizard
	assert.True(t, isSpellCapable(character), "a depleted wizard remains spell-capable")
	character.SetStatus(party_state.Sleep)
	assert.False(t, isSpellCapable(character), "a sleeping wizard cannot be selected as an actor")
	character.SetStatus(party_state.Poisoned)
	assert.True(t, isSpellCapable(character), "a poisoned wizard remains able to cast")
}

func TestCastEligibilityFailuresUseTemporaryOverlay(t *testing.T) {
	t.Run("no magical ability", func(t *testing.T) {
		scene := newCastFailureTestScene(t)
		scene.gameState.PartyState.ForEachCharacter(func(_ int, character *party_state.PlayerCharacter) {
			if character.IsInParty() {
				character.Class = party_state.Fighter
			}
		})

		scene.smallMapInputHandler(ebiten.KeyC)
		overlay := waitForTimedOverlay(t, scene)
		assert.Equal(t, "No one in the party has magical ability!", overlay.GetMessage())
		assertTemporaryCastOverlayDismisses(t, scene)
		drainProductionCastWorkflow(t, scene)
		assert.False(t, scene.surfaceSpellWorkflowOwnsCycle)
	})

	t.Run("depleted magic points", func(t *testing.T) {
		scene := newCastFailureTestScene(t)
		avatar := scene.gameState.PartyState.GetAvatar()
		avatar.Class = party_state.Wizard
		avatar.SetMp(0)
		require.True(t, scene.gameState.SetActivePlayer(0))

		scene.smallMapInputHandler(ebiten.KeyC)
		overlay := waitForTimedOverlay(t, scene)
		assert.Equal(t, avatar.GetNameAsString()+" has no MP remaining!", overlay.GetMessage())
		assertTemporaryCastOverlayDismisses(t, scene)
		drainProductionCastWorkflow(t, scene)
		assert.False(t, scene.surfaceSpellWorkflowOwnsCycle)
	})
}

func newCastFailureTestScene(t *testing.T) *GameScene {
	t.Helper()
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()

	return &GameScene{
		gameState:         state,
		output:            text.NewOutput(nil, 1, 10, 80),
		keyboard:          input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		spriteSheet:       &sprites.SpriteSheet{},
		autoOpenDoorState: &AutoOpenDoorState{},
	}
}

func newProductionCastTestScene(state *game_state.GameState) *GameScene {
	return &GameScene{
		gameState: state,
		output:    text.NewOutput(nil, 1, 10, 80),
		// SimulateKeyPress supplies the command, but modifiers/navigation are
		// still polled. Host-held Ctrl must not turn fixture syllables into chords.
		keyboard:          input.NewKeyboardWithKeyStateSource(0, func(ebiten.Key) bool { return false }),
		spriteSheet:       &sprites.SpriteSheet{},
		autoOpenDoorState: &AutoOpenDoorState{},
	}
}

func newProductionCombatCastState(t *testing.T) (*game_state.GameState, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, mock := newBritainCombatTestState(t, 42)
	state.BeginCombatTurns(game_state.CombatTurnsImmediate)
	state.PartyState.ForEachCharacter(func(_ int, caster *party_state.PlayerCharacter) {
		if !caster.IsInParty() {
			return
		}
		caster.Class = party_state.Wizard
		caster.Level = 8
		caster.SetMp(8)
	})
	state.PartyState.Inventory.Spells.Set(references.InXenMani, 1)
	state.ConfirmSpellKnowledge(references.InXenMani, game_state.ExperimentationKnowledgeSource())

	return state, mock
}

func assertCombatMatchesSingleFinish(t *testing.T, actual, expected *game_state.GameState) {
	t.Helper()
	assert.Equal(t, expected.CombatScene.GetCurrentTurnIdentity(), actual.CombatScene.GetCurrentTurnIdentity())
	actualForecast := make([]game_state.CombatTurnForecastEntry, 8)
	expectedForecast := make([]game_state.CombatTurnForecastEntry, 8)
	assert.Equal(t,
		expected.CombatScene.TurnQueue.FillForecast(expectedForecast),
		actual.CombatScene.TurnQueue.FillForecast(actualForecast),
	)
	assert.Equal(t, expectedForecast, actualForecast, "Cast must finish the accepted action exactly once")
}

// waitForTopModal advances workflows until the top modal dialog is a D and
// returns it.
func waitForTopModal[D any](t *testing.T, scene *GameScene) D {
	t.Helper()

	return waitForTopModalWithin[D](t, scene, DialogPollingRateMs, 30)
}

// waitForTopModalWithin advances workflows by stepMs for at most updates
// updates until the top modal dialog is a D, and returns it.
func waitForTopModalWithin[D any](t *testing.T, scene *GameScene, stepMs int64, updates int) D {
	t.Helper()
	for range updates {
		scene.gameState.ElapsedMs += stepMs
		scene.gameState.EventQueueManager.Update()
		if dialog, ok := scene.dialogStack.PeekTopModalDialog().(D); ok {
			return dialog
		}
	}
	var none D
	t.Fatalf("%T was not shown", none)

	return none
}

// drainSceneWorkflow advances workflows by stepMs until the named workflow ends.
func drainSceneWorkflow(t *testing.T, scene *GameScene, name string, stepMs int64) {
	t.Helper()
	for range 500 {
		scene.gameState.ElapsedMs += stepMs
		scene.gameState.EventQueueManager.Update()
		if !scene.gameState.EventQueueManager.HasWorkflow(name) {
			return
		}
	}
	t.Fatalf("workflow %q did not complete", name)
}

// acknowledgeTranscript presses Enter on an interaction transcript and requires
// it to accept the acknowledgement.
func acknowledgeTranscript(t *testing.T, scene *GameScene, transcript *widgets.InteractionTranscript) {
	t.Helper()
	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetForceWaitAnyKey(-1000)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	transcript.Update()
	require.True(t, transcript.InputComplete())
}

func drainProductionCastWorkflow(t *testing.T, scene *GameScene) {
	t.Helper()
	for range 30 {
		if overlay, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog); ok {
			overlay.Dismiss()
		}
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		scene.updateTransientActionFeedback()
		if !scene.gameState.EventQueueManager.HasWorkflow("spell_casting_workflow") &&
			!scene.gameState.EventQueueManager.HasWorkflow("combat_party_spell_effect") && len(scene.transientActionFeedback) == 0 {
			return
		}
	}
	t.Fatal("production spell casting workflow did not complete")
}

func waitForProductionPartySpellTargeting(t *testing.T, scene *GameScene) {
	t.Helper()
	for range 20 {
		scene.gameState.ElapsedMs += DialogPollingRateMs
		scene.gameState.EventQueueManager.Update()
		if scene.gameState.IsCombatPartySpellTargeting() && !scene.gameState.EventQueueManager.HasWorkflow("spell_casting_workflow") {
			require.Equal(t, CombatSpellTargetInput, scene.secondaryKeyState)
			require.Zero(t, scene.dialogStack.Count(), "battlefield selection never falls back to roster dialog")
			return
		}
	}
	t.Fatal("combat party spell did not transfer to battlefield targeting")
}

func assertTemporaryCastOverlayDismisses(t *testing.T, scene *GameScene) {
	t.Helper()
	scene.gameState.ElapsedMs += castFailureDialogDurationMs
	scene.updateTimedOverlay()
	assert.Nil(t, scene.timedOverlayDialog)
	assert.Zero(t, scene.dialogStack.Count())
}

// drainDismissingOverlays drains the named workflow in one-second steps,
// dismissing any feedback overlay that would otherwise wait for a key.
func drainDismissingOverlays(t *testing.T, scene *GameScene, name string) {
	t.Helper()
	for range 50 {
		if overlay, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog); ok {
			overlay.Dismiss()
		}
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if !scene.gameState.EventQueueManager.HasWorkflow(name) {
			return
		}
	}
	t.Fatalf("workflow %q did not complete", name)
}

// waitForTimedOverlay advances workflows until the scene shows its timed overlay.
func waitForTimedOverlay(t *testing.T, scene *GameScene) *widgets.OverlayDialog {
	t.Helper()
	for range 20 {
		scene.gameState.ElapsedMs += DialogPollingRateMs
		scene.gameState.EventQueueManager.Update()
		if overlay, ok := scene.timedOverlayDialog.(*widgets.OverlayDialog); ok {
			return overlay
		}
	}
	t.Fatal("timed overlay was not shown")

	return nil
}
