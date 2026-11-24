package main

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestDetermineTalkTypeRequiresMatchingAutomaticOverrideProvenance(t *testing.T) {
	tests := []struct {
		name       string
		presence   references.Shadowlord
		aiType     references.AiType
		provenance references.AIOverrideProvenance
		want       TalkType
	}{
		{
			name:       "Astaroth hostility",
			presence:   references.Astaroth,
			aiType:     references.ExtortOrAttackOrFollow,
			provenance: references.AIOverrideProvenanceAutomaticAstaroth,
			want:       TalkTypeShadowlordHostile,
		},
		{
			name:       "Astaroth remains hostile after flee transition",
			presence:   references.Astaroth,
			aiType:     references.FleeFromAvatar,
			provenance: references.AIOverrideProvenanceAutomaticAstaroth,
			want:       TalkTypeShadowlordHostile,
		},
		{
			name:       "Nosfentor cowardice",
			presence:   references.Nosfentor,
			aiType:     references.FleeFromAvatar,
			provenance: references.AIOverrideProvenanceAutomaticNosfentor,
			want:       TalkTypeShadowlordFlee,
		},
		{
			name:       "guard hostility is not Astaroth",
			presence:   references.Astaroth,
			aiType:     references.ExtortOrAttackOrFollow,
			provenance: references.AIOverrideProvenanceGuardHostility,
			want:       TalkTypeGuard,
		},
		{
			name:       "generic flee is not Nosfentor",
			presence:   references.Nosfentor,
			aiType:     references.FleeFromAvatar,
			provenance: references.AIOverrideProvenanceGenericFlee,
			want:       TalkTypeGuard,
		},
		{
			name:       "Yell summon does not activate automatic response",
			presence:   references.Shadowlord(-1),
			aiType:     references.ExtortOrAttackOrFollow,
			provenance: references.AIOverrideProvenanceAutomaticAstaroth,
			want:       TalkTypeGuard,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := new(game_state.GameState)
			state.ShadowlordState = references.NewShadowlordState()
			state.ShadowlordState.AutomaticPresence = test.presence
			state.SetCriminalStatus(true)
			scene := &GameScene{gameState: state}
			npc := shadowlordTalkTestGuard(test.aiType, test.provenance)

			assert.Equal(t, test.want, scene.determineTalkType(npc, false))
		})
	}
}

func TestAstarothFleeTransitionRetainsAutomaticProvenance(t *testing.T) {
	npc := shadowlordTalkTestGuard(references.ExtortOrAttackOrFollow, references.AIOverrideProvenanceAutomaticAstaroth)

	npc.MapUnitDetails().SetAIOverride(references.FleeFromAvatar, references.AIOverrideProvenanceAutomaticAstaroth)

	assert.Equal(t, references.AiType(references.FleeFromAvatar), npc.MapUnitDetails().GetOverriddenAiType())
	assert.Equal(t, references.AIOverrideProvenanceAutomaticAstaroth, npc.MapUnitDetails().AIOverrideProvenance())
}

func TestAstarothTalkShowsResponseThenDamagesOnceAndRemainsAstaroth(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	fixture.state.ShadowlordState.AutomaticPresence = references.Astaroth
	fixture.npc.MapUnitDetails().SetAIOverride(
		references.ExtortOrAttackOrFollow,
		references.AIOverrideProvenanceAutomaticAstaroth,
	)
	avatar := fixture.state.PartyState.GetCharacter(0)
	avatar.SetHp(100)

	for attempt := range 2 {
		hpBefore := avatar.GetCurrentHp()
		rngCallsBefore := fixture.state.GetRNG().Calls()
		fixture.scene.StartTalkWorkflow(fixture.direction)
		overlay := waitForMerchantOverlay(t, fixture)

		assert.Equal(t, `"Begone, vermin!"`, overlay.GetMessage())
		assert.Equal(t, hpBefore, avatar.GetCurrentHp(), "attempt %d damaged before response dismissal", attempt+1)
		assert.Equal(t, rngCallsBefore, fixture.state.GetRNG().Calls())

		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})

		assert.Less(t, avatar.GetCurrentHp(), hpBefore)
		assert.GreaterOrEqual(t, avatar.GetCurrentHp(), hpBefore-8)
		assert.Equal(t, rngCallsBefore+1, fixture.state.GetRNG().Calls())
		assert.Equal(t, references.AiType(references.FleeFromAvatar), fixture.npc.MapUnitDetails().GetOverriddenAiType())
		assert.Equal(t, references.AIOverrideProvenanceAutomaticAstaroth, fixture.npc.MapUnitDetails().AIOverrideProvenance())
	}
}

func TestAstarothAttackContactPrecedesCombatAndArrestWithoutAnotherTurn(t *testing.T) {
	for _, sprite := range []indexes.SpriteIndex{indexes.TownsPersonKeyIndex, indexes.GuardKeyIndex} {
		for _, action := range []references.AiType{references.ExtortOrAttackOrFollow, references.DrudgeWorthThing} {
			t.Run(fmt.Sprintf("shape%d/action%d", sprite, action), func(t *testing.T) {
				fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
				fixture.npc.NPCReference.SetKeyIndex(sprite)
				fixture.npc.NPCReference.DialogNumber = 1
				fixture.state.ShadowlordState.AutomaticPresence = references.Astaroth
				fixture.state.CurrentNPCAIController.GetNpcs().ReplaceWith(map_units.MapUnits{fixture.npc})
				if action == references.DrudgeWorthThing {
					fixture.npc.NPCReference.Schedule.Time[3] = 1
					fixture.state.SetRandomSeed(12345)
					fixture.state.ApplyShadowlordEffectsToNPCs(references.Astaroth)
					require.Equal(t, action, fixture.npc.MapUnitDetails().GetOverriddenAiType())
				} else {
					fixture.npc.MapUnitDetails().SetAIOverride(action, references.AIOverrideProvenanceAutomaticAstaroth)
				}
				fixture.state.SetCriminalStatus(true)
				avatar := fixture.state.PartyState.GetCharacter(0)
				avatar.SetHp(100)
				hpBefore := avatar.GetCurrentHp()
				fixture.callbacks.Reset()

				fixture.state.CurrentNPCAIController.AdvanceNextTurnCalcAndMoveNPCs()
				require.Equal(t, map_units.PendingNPCInteractionTalk, fixture.npc.MapUnitDetails().PendingInteraction)
				rngCallsBefore := fixture.state.GetRNG().Calls()
				require.True(t, fixture.scene.processNPCAutoTalk())
				assert.Equal(t, map_units.PendingNPCInteractionNone, fixture.npc.MapUnitDetails().PendingInteraction)
				overlay := waitForMerchantOverlay(t, fixture)
				assert.Equal(t, `"Begone, vermin!"`, overlay.GetMessage())
				assert.Equal(t, hpBefore, avatar.GetCurrentHp())
				assert.Equal(t, rngCallsBefore, fixture.state.GetRNG().Calls())
				assert.False(t, fixture.state.HasPendingOverworldContact())
				assert.Nil(t, fixture.state.CombatScene)

				pressMerchantDialogKey(fixture, ebiten.KeyEnter)
				pumpMerchantWorkflowUntil(t, fixture, func() bool {
					return !fixture.state.EventQueueManager.HasActiveWorkflows()
				})

				assert.Less(t, avatar.GetCurrentHp(), hpBefore)
				assert.GreaterOrEqual(t, avatar.GetCurrentHp(), hpBefore-8)
				assert.Equal(t, rngCallsBefore+1, fixture.state.GetRNG().Calls())
				assert.Equal(t, references.AiType(references.FleeFromAvatar), fixture.npc.MapUnitDetails().GetOverriddenAiType())
				assert.Equal(t, references.AIOverrideProvenanceAutomaticAstaroth, fixture.npc.MapUnitDetails().AIOverrideProvenance())
				assert.Empty(t, fixture.callbacks.TimeAdvanced())
				assert.False(t, fixture.state.HasPendingOverworldContact())
				assert.Nil(t, fixture.state.CombatScene)
				assert.False(t, fixture.scene.processNPCAutoTalk())
			})
		}
	}
}

func TestFaulineiConversationTheftRequiresCompletedNormalConversation(t *testing.T) {
	t.Run("completed conversation steals once and reports once", func(t *testing.T) {
		fixture := newFaulineiNormalTalkFixture(t)
		fixture.scene.StartTalkWorkflow(fixture.direction)
		dialog := waitForLinearTalkDialog(t, fixture)
		dialog.conversationCompleted = true
		fixture.scene.dialogStack.RemoveWidgetInstance(dialog)

		overlay := waitForMerchantOverlay(t, fixture)
		assert.Equal(t, "Something was stolen!", overlay.GetMessage())
		assert.Zero(t, fixture.state.PartyState.Inventory.Provisions.Keys.Get())
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
		assert.Zero(t, fixture.scene.dialogStack.Count())
	})

	t.Run("removed dialog is cancellation", func(t *testing.T) {
		fixture := newFaulineiNormalTalkFixture(t)
		rngCallsBefore := fixture.state.GetRNG().Calls()
		fixture.scene.StartTalkWorkflow(fixture.direction)
		dialog := waitForLinearTalkDialog(t, fixture)
		fixture.scene.dialogStack.RemoveWidgetInstance(dialog)

		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})

		assert.Equal(t, uint16(1), fixture.state.PartyState.Inventory.Provisions.Keys.Get())
		assert.Equal(t, rngCallsBefore, fixture.state.GetRNG().Calls())
		assert.Zero(t, fixture.scene.dialogStack.Count())
	})
}

func TestFaulineiConversationTheftExcludesMerchantHandler(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	fixture.state.ShadowlordState.AutomaticPresence = references.Faulinei
	fixture.state.PartyState.Inventory.Provisions.Keys.Set(1)
	rngCallsBefore := fixture.state.GetRNG().Calls()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	assert.Equal(t, uint16(1), fixture.state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Equal(t, rngCallsBefore, fixture.state.GetRNG().Calls())
}

func newFaulineiNormalTalkFixture(t *testing.T) *merchantTalkFixture {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Stonegate).
		WithSystemCallbacks().
		WithRandomSeed(1).
		Build()
	reference := references.NPCReference{
		Location: references.Stonegate, DialogNumber: 10, NpcType: references.NoStatedNpc,
		Schedule: references.NPCSchedule{X: [3]byte{5, 5, 5}, Y: [3]byte{5, 5, 5}},
	}
	reference.SetKeyIndex(indexes.TownsPersonKeyIndex)
	npc := map_units.NewNPCFriendly(reference, 1)
	npc.SetPos(references.Position{X: 5, Y: 5})
	npc.SetFloor(state.MapState.PlayerLocation.Floor)
	npc.SetVisible(true)
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(map_units.MapUnits{npc})
	state.ShadowlordState.AutomaticPresence = references.Faulinei
	state.PartyState.Inventory.Provisions.Keys.Set(1)
	state.PartyState.Inventory.Provisions.Gems.Set(0)
	state.PartyState.Inventory.Provisions.Torches.Set(0)
	direction := placePlayerNextToMerchant(state, npc)
	scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}

	return &merchantTalkFixture{
		state: state, callbacks: callbacks, scene: scene, npc: npc, direction: direction,
	}
}

func waitForLinearTalkDialog(t *testing.T, fixture *merchantTalkFixture) *LinearTalkDialog {
	t.Helper()
	var dialog *LinearTalkDialog
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		for _, widget := range fixture.scene.dialogStack.Dialogs() {
			if candidate, ok := widget.(*LinearTalkDialog); ok {
				dialog = candidate

				return true
			}
		}

		return false
	})
	require.NotNil(t, dialog)

	return dialog
}

func shadowlordTalkTestGuard(
	aiType references.AiType,
	provenance references.AIOverrideProvenance,
) *map_units.NPCFriendly {
	reference := references.NPCReference{DialogNumber: byte(references.DialogGuard), NpcType: references.Guard}
	reference.SetKeyIndex(indexes.GuardKeyIndex)
	npc := map_units.NewNPCFriendly(reference, 1)
	npc.MapUnitDetails().SetAIOverride(aiType, provenance)

	return npc
}
