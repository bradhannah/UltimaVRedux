package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func spellBrowserTestKeys(scene *GameScene) func(...ebiten.Key) {
	held := map[ebiten.Key]bool{}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	return func(keys ...ebiten.Key) {
		for _, key := range keys {
			held[key] = true
		}
		scene.updateDialogs()
		clear(held)
		scene.keyboard.ClearFrameConsumption()
		scene.updateDialogs()
	}
}

func waitForSpellBrowserState(t *testing.T, scene *GameScene, ready func() bool) {
	t.Helper()
	for range 100 {
		scene.gameState.ElapsedMs += 100
		scene.gameState.EventQueueManager.Update()
		if ready() {
			return
		}
	}
	t.Fatal("spell browser transition did not complete")
}

func TestCastMixShortcutReturnsToSameSelectorAndChargesOnlyMixOnCancel(t *testing.T) {
	for _, location := range []references.Location{references.Britain, references.Deceit} {
		t.Run(location.String(), func(t *testing.T) {
			state, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(location).WithSystemCallbacks().WithRandomSeed(42).Build()
			scene := newProductionCastTestScene(state)
			press := spellBrowserTestKeys(scene)
			require.True(t, state.SetActivePlayer(0))
			caster := state.PartyState.GetAvatar()
			caster.Class, caster.Level = party_state.Wizard, 8
			caster.SetMp(8)
			state.PartyState.Inventory.Reagent.Set(references.SulfurAsh, 6)
			state.PartyState.Inventory.Spells.Set(references.InLor, 0)
			if location == references.Deceit {
				scene.dungeonMapInputHandler(ebiten.KeyC)
			} else {
				scene.smallMapInputHandler(ebiten.KeyC)
			}
			selector := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			press(ebiten.KeyI)
			press(ebiten.KeyL)
			before := state.DateTime
			press(ebiten.KeyControl, ebiten.KeyM)
			waitForSpellBrowserState(t, scene, func() bool {
				_, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ReagentMixingDialog)
				return ok
			})
			press(ebiten.Key6)
			press(ebiten.Key5)
			press(ebiten.KeyEnter)
			waitForSpellBrowserState(t, scene, func() bool {
				receipt, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
				if ok {
					receipt.Dismiss()
				}
				return ok
			})
			// Allow the receipt, normal Mix cycle and Cast resumption to finish.
			for range 30 {
				state.ElapsedMs += 100
				state.EventQueueManager.Update()
			}
			require.Same(t, selector, scene.dialogStack.PeekTopModalDialog())
			require.Equal(t, references.InLor, selector.GetSelectedSpell())
			require.Equal(t, uint16(5), state.PartyState.Inventory.Spells.Get(references.InLor))
			require.Equal(t, uint16(1), state.PartyState.Inventory.Reagent.Get(references.SulfurAsh))
			require.Equal(t, byte(8), caster.GetCurrentMp())
			require.NotEqual(t, before, state.DateTime, "Mix pays its ordinary command cycle")
			afterMix := state.DateTime
			press(ebiten.KeyEscape)
			drainProductionCastWorkflow(t, scene)
			require.Equal(t, afterMix, state.DateTime, "cancelling resumed Cast adds no charge")
			require.Zero(t, scene.dialogStack.Count())
			if location == references.Deceit {
				require.True(t, state.IsAtDungeonCommandBoundary())
			}
		})
	}
}

func TestCastPrerequisiteRefusalRetainsBrowserWithoutSpendingResources(t *testing.T) {
	for _, location := range []references.Location{references.Britain, references.Deceit} {
		t.Run(location.String(), func(t *testing.T) {
			state, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(location).
				WithSystemCallbacks().WithRandomSeed(42).Build()
			scene := newProductionCastTestScene(state)
			press := spellBrowserTestKeys(scene)
			require.True(t, state.SetActivePlayer(0))
			caster := state.PartyState.GetAvatar()
			caster.Class, caster.Level = party_state.Wizard, 8
			caster.SetMp(8)
			state.PartyState.Inventory.Spells.Set(references.InLor, 0)
			if location == references.Deceit {
				scene.dungeonMapInputHandler(ebiten.KeyC)
			} else {
				scene.smallMapInputHandler(ebiten.KeyC)
			}
			selector := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			press(ebiten.KeyI)
			press(ebiten.KeyL)
			before := state.DateTime
			press(ebiten.KeyNumpadEnter)
			for range 10 {
				state.ElapsedMs += 100
				state.EventQueueManager.Update()
			}
			require.Same(t, selector, scene.dialogStack.PeekTopModalDialog())
			require.Equal(t, references.InLor, selector.GetSelectedSpell())
			press(ebiten.KeyEscape)
			drainProductionCastWorkflow(t, scene)
			require.Equal(t, before, state.DateTime)
			require.Equal(t, byte(8), caster.GetCurrentMp())
			require.Zero(t, state.PartyState.Inventory.Spells.Get(references.InLor))
			require.Zero(t, scene.dialogStack.Count())
		})
	}
}

func TestZStatsMixShortcutRefreshesSameSection(t *testing.T) {
	for _, location := range []references.Location{references.Britain, references.Deceit} {
		t.Run(location.String(), func(t *testing.T) {
			state, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(location).WithSystemCallbacks().Build()
			scene := newProductionCastTestScene(state)
			press := spellBrowserTestKeys(scene)
			state.PartyState.Inventory.Reagent.Set(references.SulfurAsh, 6)
			state.PartyState.Inventory.Spells.Set(references.InLor, 0)
			scene.ShowZStatsDossier(state.CurrentZStatsDossier())
			dossier := scene.dialogStack.PeekTopModalDialog().(*widgets.ZStatsDossierDialog)
			for range 3 {
				press(ebiten.KeyTab)
			}
			press(ebiten.KeyI)
			press(ebiten.KeyL)
			press(ebiten.KeyControl, ebiten.KeyM)
			waitForSpellBrowserState(t, scene, func() bool {
				_, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ReagentMixingDialog)
				return ok
			})
			press(ebiten.Key5)
			press(ebiten.KeyEnter)
			waitForSpellBrowserState(t, scene, func() bool {
				receipt, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
				if ok {
					receipt.Dismiss()
				}
				return ok
			})
			waitForSpellBrowserState(t, scene, func() bool { return !state.EventQueueManager.HasWorkflow("reagent_mixing_workflow") })
			require.Same(t, dossier, scene.dialogStack.PeekTopModalDialog())
			require.Equal(t, widgets.ZStatsDossierSpellsTab, dossier.GetActiveTab())
			require.Equal(t, uint16(5), state.PartyState.Inventory.Spells.Get(references.InLor))
			press(ebiten.KeyEscape)
			require.Zero(t, scene.dialogStack.Count())
			if location == references.Deceit {
				require.True(t, state.IsAtDungeonCommandBoundary())
			}
		})
	}
}

func TestZStatsNestedMixShutdownClosesSuspendedDossier(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	scene := newProductionCastTestScene(state)
	press := spellBrowserTestKeys(scene)
	scene.ShowZStatsDossier(state.CurrentZStatsDossier())
	for range 3 {
		press(ebiten.KeyTab)
	}
	press(ebiten.KeyControl, ebiten.KeyM)
	waitForSpellBrowserState(t, scene, func() bool { return scene.dialogStack.Count() == 2 })
	state.EventQueueManager.Shutdown()
	require.Zero(t, scene.dialogStack.Count())
}

func TestCastNestedMixShutdownCleansBothModals(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	scene := newProductionCastTestScene(state)
	press := spellBrowserTestKeys(scene)
	require.True(t, state.SetActivePlayer(0))
	state.PartyState.GetAvatar().Class = party_state.Wizard
	state.PartyState.GetAvatar().SetMp(8)
	scene.smallMapInputHandler(ebiten.KeyC)
	waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	press(ebiten.KeyControl, ebiten.KeyM)
	waitForSpellBrowserState(t, scene, func() bool { return scene.dialogStack.Count() == 2 })
	state.EventQueueManager.Shutdown()
	require.Zero(t, scene.dialogStack.Count())
	require.False(t, state.EventQueueManager.HasActiveWorkflows())
}

func TestCastMixCancellationAndFailureResumeParent(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "failed recipe"}[failed], func(t *testing.T) {
			state, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
				WithSystemCallbacks().WithRandomSeed(42).Build()
			scene := newProductionCastTestScene(state)
			press := spellBrowserTestKeys(scene)
			require.True(t, state.SetActivePlayer(0))
			state.PartyState.GetAvatar().Class = party_state.Wizard
			state.PartyState.GetAvatar().SetMp(8)
			state.PartyState.Inventory.Reagent.Set(references.SulfurAsh, 6)
			state.PartyState.Inventory.Reagent.Set(references.Ginseng, 6)
			state.PartyState.Inventory.Spells.Set(references.InLor, 0)
			scene.smallMapInputHandler(ebiten.KeyC)
			selector := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
			press(ebiten.KeyI)
			press(ebiten.KeyL)
			press(ebiten.KeyControl, ebiten.KeyM)
			waitForSpellBrowserState(t, scene, func() bool {
				_, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ReagentMixingDialog)
				return ok
			})
			if failed {
				press(ebiten.KeyTab) // The Experiment tab: swap Sulfurous Ash for Ginseng.
				press(ebiten.KeySpace)
				press(ebiten.KeyDown)
				press(ebiten.KeySpace)
				press(ebiten.KeyEnter)
			} else {
				press(ebiten.KeyEscape)
			}
			for range 100 {
				state.ElapsedMs += 100
				state.EventQueueManager.Update()
			}
			require.Same(t, selector, scene.dialogStack.PeekTopModalDialog())
			require.Equal(t, references.InLor, selector.GetSelectedSpell())
			require.Zero(t, state.PartyState.Inventory.Spells.Get(references.InLor))
			if failed {
				require.Equal(t, uint16(5), state.PartyState.Inventory.Reagent.Get(references.Ginseng))
			}
			press(ebiten.KeyEscape)
			drainProductionCastWorkflow(t, scene)
			require.Zero(t, scene.dialogStack.Count())
		})
	}
}

func TestZStatsCombatMixRefusalLeavesDossierUsable(t *testing.T) {
	state, _ := newProductionCombatCastState(t)
	scene := newProductionCastTestScene(state)
	press := spellBrowserTestKeys(scene)
	scene.ShowZStatsDossier(state.CurrentZStatsDossier())
	dossier := scene.dialogStack.PeekTopModalDialog().(*widgets.ZStatsDossierDialog)
	for range 3 {
		press(ebiten.KeyTab)
	}
	before, actor := state.DateTime, state.CombatScene.GetCurrentTurnIdentity()
	press(ebiten.KeyControl, ebiten.KeyM)
	require.Same(t, dossier, scene.dialogStack.PeekTopModalDialog())
	require.False(t, state.EventQueueManager.HasWorkflow("reagent_mixing_workflow"))
	require.Equal(t, before, state.DateTime)
	require.Equal(t, actor, state.CombatScene.GetCurrentTurnIdentity())
	press(ebiten.KeyEscape)
	require.Zero(t, scene.dialogStack.Count())
}
