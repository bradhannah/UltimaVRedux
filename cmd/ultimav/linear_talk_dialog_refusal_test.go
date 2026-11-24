package main

import (
	"fmt"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/conversation"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestTalkProductionFreeTextAndKeyWait(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.SerpentsHold).WithRandomSeed(42).WithSystemCallbacks().Build()
	var actor *map_units.NPCFriendly
	for _, unit := range *state.CurrentNPCAIController.GetNpcs() {
		if npc, ok := unit.(*map_units.NPCFriendly); ok && npc.MapUnitDetails().NPCNum == 17 {
			actor = npc
			break
		}
	}
	require.NotNil(t, actor)
	held := make(map[ebiten.Key]bool)
	keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	scene := &GameScene{gameState: state, keyboard: keyboard}
	dialog := NewLinearTalkDialog(scene, actor)
	require.NotNil(t, dialog)
	scene.PushDialog(dialog)
	press := func(key ebiten.Key) {
		held[key] = true
		dialog.Update()
		delete(held, key)
		keyboard.ClearFrameConsumption()
		dialog.Update()
	}
	typeWords := func(words string) {
		for _, letter := range words {
			if letter == ' ' {
				press(ebiten.KeySpace)
			} else {
				press(ebiten.KeyA + ebiten.Key(letter-'A'))
			}
		}
		press(ebiten.KeyEnter)
	}
	press(ebiten.KeyTab)
	typeWords("YOUR NAME")
	require.Contains(t, dialog.currentResponse.Output, "My name is Kristi")
	typeWords("SKULL")
	require.True(t, dialog.waitingForKeypressContinue)
	for steps := 0; dialog.waitingForKeypressContinue && steps < 10; steps++ {
		press(ebiten.KeySpace)
	}
	require.False(t, dialog.waitingForKeypressContinue)
	require.Empty(t, dialog.TextInput.GetText(), "continue keys must not become input")
	require.True(t, dialog.currentResponse.IsLabelQuestion)
	typeWords("NO")
	require.False(t, dialog.currentResponse.IsLabelQuestion)
	typeWords("")
	require.True(t, dialog.waitingForDismissal)
}

func TestTalkProductionAskNameUsesActivePartyAndActorFlag(t *testing.T) {
	for _, value := range []string{"Iolo", "A"} {
		t.Run(value, func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.SerpentsHold).WithRandomSeed(42).WithSystemCallbacks().Build()
			var actor *map_units.NPCFriendly
			for _, unit := range *state.CurrentNPCAIController.GetNpcs() {
				if npc, ok := unit.(*map_units.NPCFriendly); ok && npc.MapUnitDetails().NPCNum == 17 {
					actor = npc
					break
				}
			}
			require.NotNil(t, actor)
			met, dead := state.PartyState.NPCFlagsSnapshot()
			met[references.SerpentsHold][17] = false
			met[references.SerpentsHold][24] = false
			state.PartyState.RestoreNPCFlags(met, dead)
			scene := &GameScene{gameState: state, keyboard: newPresentationTestKeyboard()}
			dialog := NewLinearTalkDialog(scene, actor)
			require.NotNil(t, dialog)
			scene.PushDialog(dialog)
			require.Contains(t, dialog.callbacks.GetActivePartyNames(), "Iolo")
			program, found := state.GameReferences.ConversationCatalog.Original(semantic.OriginalSourceKeep, 24)
			require.True(t, found)
			definition, err := program.Definition()
			require.NoError(t, err)
			definition.Description = semantic.Body{&semantic.AskNameAction{}}
			engine, err := conversation.NewSemanticConversationEngine(definition, dialog.callbacks, false)
			require.NoError(t, err)
			dialog.engine = engine
			dialog.startConversation()
			dialog.TextInput.SetText(value)
			dialog.onEnter()
			require.Equal(t, value == "Iolo", state.PartyState.HasMetNPC(references.SerpentsHold, 17))
			require.False(t, state.PartyState.HasMetNPC(references.SerpentsHold, 24), "dialog number is not actor identity")
			require.Equal(t, state.PartyState.AvatarName(), dialog.callbacks.GetAvatarName())
		})
	}
}

func TestKristiProductionPaymentRefusal(t *testing.T) {
	for _, gold := range []uint16{0, 99, 100} {
		for _, next := range []string{"NO", "NAME", "BYE", "JOB"} {
			t.Run(fmt.Sprintf("%d/%s", gold, next), func(t *testing.T) {
				state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.SerpentsHold).
					WithRandomSeed(42).WithSystemCallbacks().Build()
				state.PartyState.Inventory.Gold.Set(gold)
				state.PartyState.Inventory.Provisions.SkullKeys.Set(0)
				var actor *map_units.NPCFriendly
				for _, unit := range *state.CurrentNPCAIController.GetNpcs() {
					if npc, ok := unit.(*map_units.NPCFriendly); ok && npc.MapUnitDetails().NPCNum == 17 {
						actor = npc
						break
					}
				}
				require.NotNil(t, actor)
				require.EqualValues(t, 24, actor.NPCReference.DialogNumber)
				scene := &GameScene{gameState: state, keyboard: newPresentationTestKeyboard()}
				dialog := NewLinearTalkDialog(scene, actor)
				require.NotNil(t, dialog)
				scene.PushDialog(dialog)
				require.Equal(t, "Kristi", dialog.engine.ExtractNPCName())
				enter := func(value string) { dialog.TextInput.SetText(value); dialog.onEnter() }
				enter("SKULL")
				for steps := 0; dialog.currentResponse.WaitingForKeypress && steps < 10; steps++ {
					dialog.waitingForKeypressContinue = false
					dialog.currentResponse = dialog.engine.ProcessInput("")
					dialog.processCurrentResponse()
				}
				require.True(t, dialog.waitingForInput)
				require.Contains(t, dialog.currentResponse.Output, "100 gold")
				knowledge, karma := state.KnowledgeSnapshot(), state.PartyState.Karma.Value
				enter("YES")
				require.NoError(t, dialog.currentResponse.Error)
				require.True(t, dialog.waitingForInput)
				require.False(t, dialog.currentResponse.IsLabelQuestion)
				enter(next)
				require.NoError(t, dialog.currentResponse.Error)
				if gold < 100 {
					require.Zero(t, state.PartyState.Inventory.Provisions.SkullKeys.Get())
					require.Equal(t, gold, state.PartyState.Inventory.Gold.Get())
					require.Equal(t, knowledge, state.KnowledgeSnapshot())
					require.Equal(t, karma, state.PartyState.Karma.Value)
				} else {
					require.EqualValues(t, 5, state.PartyState.Inventory.Provisions.SkullKeys.Get())
					require.Zero(t, state.PartyState.Inventory.Gold.Get())
				}
			})
		}
	}
}
