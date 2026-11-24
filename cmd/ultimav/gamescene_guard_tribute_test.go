package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestGuardTributePaymentUsesSharedTalkLifecycle(t *testing.T) {
	tests := []struct {
		name            string
		location        references.Location
		gold            uint16
		activeMembers   int
		npcInitiated    bool
		expectedGold    uint16
		expectedMinutes []int
		expectedDemand  string
		expectedPrompt  string
	}{
		{
			name:            "manual generic tribute",
			location:        references.Britain,
			gold:            100,
			activeMembers:   2,
			expectedGold:    80,
			expectedMinutes: []int{1},
			expectedDemand:  "A guard demands a 20 gp tribute to Blackthorn!",
			expectedPrompt:  "Dost thou pay?",
		},
		{
			name:            "NPC initiated Minoc charity",
			location:        references.Minoc,
			gold:            101,
			activeMembers:   1,
			npcInitiated:    true,
			expectedGold:    50,
			expectedMinutes: []int{},
			expectedDemand:  "\"Thou wilt give half thy gold to charity!\"",
			expectedPrompt:  "Dost thou pay?",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGuardTributeFixture(t, test.location, test.activeMembers)
			fixture.state.PartyState.Inventory.Gold.Set(test.gold)

			if test.npcInitiated {
				fixture.scene.startNPCTalkWorkflow(fixture.guard)
			} else {
				fixture.scene.StartTalkWorkflow(references.Right)
			}
			require.True(t, fixture.state.EventQueueManager.HasWorkflow("talk_workflow"))
			dialog := waitForGuardYesNo(t, fixture)
			require.Equal(t, test.expectedPrompt, dialog.ActivePrompt())

			answerGuardYesNo(t, fixture, ebiten.KeyY)
			pumpGuardTalkWithoutInputUntilDone(t, fixture)

			transcriptText := strings.Join(strings.Fields(dialog.TranscriptText()), " ")
			require.Contains(t, transcriptText, test.expectedDemand)
			require.Contains(t, transcriptText, test.expectedPrompt+" > Yes")
			require.NotContains(t, dialog.TranscriptText(), "The guard nods and walks away.")
			require.Equal(t, test.expectedGold, fixture.state.PartyState.Inventory.Gold.Get())
			if len(test.expectedMinutes) == 0 {
				require.Empty(t, fixture.callbacks.TimeAdvanced())
			} else {
				require.Equal(t, test.expectedMinutes, fixture.callbacks.TimeAdvanced())
			}
			require.Equal(t, 1, fixture.callbacks.BlockInputCalls)
			require.Equal(t, 1, fixture.callbacks.UnblockInputCalls)
			require.False(t, fixture.state.EventQueueManager.HasActiveWorkflows())
			require.False(t, fixture.scene.dialogStack.HasOpenDialog())
		})
	}
}

func TestHostileGuardPendingInteractionRoutesDirectlyToArrest(t *testing.T) {
	for _, dialogNumber := range []byte{byte(references.DialogGuard), byte(references.NoResponse)} {
		t.Run(fmt.Sprintf("dialog 0x%02x", dialogNumber), func(t *testing.T) {
			fixture := newGuardTributeFixture(t, references.Yew, 1)
			fixture.guard.NPCReference.DialogNumber = dialogNumber
			fixture.guard.NPCReference.Schedule.Ai[0] = byte(references.Talk3)
			fixture.guard.MapUnitDetails().SetLawAIOverride(references.ExtortOrAttackOrFollow)
			fixture.guard.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionGuardArrest
			fixture.state.PartyState.Inventory.Gold.Set(100)
			before := fixture.state.DateTime

			require.True(t, fixture.scene.processNPCAutoTalk())
			require.Equal(t, map_units.PendingNPCInteractionNone, fixture.guard.MapUnitDetails().PendingInteraction)
			require.True(t, fixture.state.EventQueueManager.HasWorkflow("guard_arrest_workflow"))
			require.False(t, fixture.state.EventQueueManager.HasWorkflow("talk_workflow"))

			dialog := waitForGuardYesNo(t, fixture)
			require.Equal(t, "Wilt thou come quietly?", dialog.ActivePrompt())
			require.Contains(t, dialog.TranscriptText(), "Thou art under arrest!")
			require.NotContains(t, dialog.TranscriptText(), "Dost thou pay?")
			require.Equal(t, references.AiType(references.Talk3), references.AiType(fixture.guard.NPCReference.Schedule.Ai[0]))
			_, consumed := fixture.guard.MapUnitDetails().ConsumedTalk3ScheduleSlot()
			require.False(t, consumed)

			answerGuardYesNo(t, fixture, ebiten.KeyN)
			pumpGuardTalkUntilDone(t, fixture)
			require.Equal(t, uint16(100), fixture.state.PartyState.Inventory.Gold.Get())
			require.Equal(t, before, fixture.state.DateTime)
			require.Empty(t, fixture.callbacks.TimeAdvanced())
			require.True(t, fixture.state.HasPendingOverworldContact())
		})
	}
}

func TestGuardTributeWorkflowUsesSelectorInsteadOfActorShape(t *testing.T) {
	fixture := newGuardTributeFixture(t, references.Britain, 1)
	fixture.guard.NPCReference.SetKeyIndex(indexes.BardKeyIndex)
	fixture.state.PartyState.Inventory.Gold.Set(100)

	fixture.scene.StartTalkWorkflow(references.Right)
	answerGuardYesNo(t, fixture, ebiten.KeyY)
	pumpGuardTalkUntilDone(t, fixture)

	require.Equal(t, uint16(90), fixture.state.PartyState.Inventory.Gold.Get())
	require.False(t, fixture.state.EventQueueManager.HasActiveWorkflows())
	require.False(t, fixture.scene.dialogStack.HasOpenDialog())
}

func TestGuardTributeInsufficientGoldHandsOffToInlineJail(t *testing.T) {
	fixture := newGuardTributeFixture(t, references.Britain, 1)
	fixture.state.PartyState.Inventory.Gold.Set(0)
	fixture.state.PartyState.Inventory.Provisions.Keys.Set(7)
	fixture.state.DateTime.Hour = references.JailReleaseHour
	fixture.state.DateTime.Minute = 59
	expectedRelease := fixture.state.DateTime
	expectedRelease.Advance(1)
	for expectedRelease.Hour != references.JailReleaseHour {
		expectedRelease.Advance(references.JailSentenceStepMinutes)
	}

	fixture.scene.StartTalkWorkflow(references.Right)
	transcript := answerGuardYesNo(t, fixture, ebiten.KeyY)
	require.True(t, fixture.state.EventQueueManager.HasWorkflow("talk_workflow"))
	require.Same(t, transcript, waitForGuardYesNo(t, fixture))
	require.Equal(t, "Wilt thou come quietly?", transcript.ActivePrompt())
	require.Contains(t, transcript.TranscriptText(), "Thou hast not enough gold. The tribute is unpaid.")
	require.Contains(t, transcript.TranscriptText(), "Dost thou pay?  > Yes")
	require.Equal(t, references.Britain, fixture.state.MapState.PlayerLocation.Location)
	require.Equal(t, uint16(7), fixture.state.PartyState.Inventory.Provisions.Keys.Get())
	require.Zero(t, fixture.state.PartyState.Inventory.Gold.Get())
	require.False(t, fixture.state.HasPendingOverworldContact())
	require.Same(t, transcript, answerGuardYesNo(t, fixture, ebiten.KeyY))
	pumpGuardTalkUntilDone(t, fixture)

	transcriptText := strings.Join(strings.Fields(transcript.TranscriptText()), " ")
	require.Contains(t, transcriptText, "Wilt thou come quietly? > Yes")
	require.Contains(t, transcriptText, "Thou dost awaken to...")
	require.Equal(t, references.Yew, fixture.state.MapState.PlayerLocation.Location)
	require.Equal(t, references.Position{X: references.JailPositionX, Y: references.JailPositionY},
		fixture.state.MapState.PlayerLocation.Position)
	require.Equal(t, references.JailFloorNumber, fixture.state.MapState.PlayerLocation.Floor)
	require.Equal(t, expectedRelease, fixture.state.DateTime,
		"the manual Talk minute must precede the sentence boundary check")
	require.Zero(t, fixture.state.PartyState.Inventory.Provisions.Keys.Get())
	require.False(t, fixture.state.IsCriminal())
	require.Equal(t, 1, fixture.callbacks.BlockInputCalls)
	require.Equal(t, 1, fixture.callbacks.UnblockInputCalls)
	require.False(t, fixture.state.EventQueueManager.HasActiveWorkflows())
	require.False(t, fixture.scene.dialogStack.HasOpenDialog())
}

func TestGuardTributeInsufficientGoldRetainsArrestCombatAuthority(t *testing.T) {
	for _, npcInitiated := range []bool{false, true} {
		t.Run(fmt.Sprintf("NPC initiated %t", npcInitiated), func(t *testing.T) {
			fixture := newGuardTributeFixture(t, references.Britain, 1)
			fixture.state.PartyState.Inventory.Gold.Set(0)
			expectedTime := fixture.state.DateTime
			if npcInitiated {
				fixture.scene.startNPCTalkWorkflow(fixture.guard)
			} else {
				fixture.scene.StartTalkWorkflow(references.Right)
				expectedTime.Advance(1)
			}
			answerGuardYesNo(t, fixture, ebiten.KeyY)
			dialog := waitForGuardYesNo(t, fixture)
			require.Equal(t, "Wilt thou come quietly?", dialog.ActivePrompt())
			require.Contains(t, dialog.TranscriptText(), "Thou hast not enough gold. The tribute is unpaid.")
			require.False(t, fixture.state.HasPendingOverworldContact())
			answerGuardYesNo(t, fixture, ebiten.KeyN)
			pumpGuardTalkUntilDone(t, fixture)

			require.Equal(t, expectedTime, fixture.state.DateTime)
			if npcInitiated {
				require.Empty(t, fixture.callbacks.TimeAdvanced())
			} else {
				require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
			}
			require.Zero(t, fixture.state.PartyState.Inventory.Gold.Get())
			require.Equal(t, 1, fixture.callbacks.BlockInputCalls)
			require.Equal(t, 1, fixture.callbacks.UnblockInputCalls)
			require.False(t, fixture.scene.dialogStack.HasOpenDialog())
			require.True(t, fixture.state.HasPendingOverworldContact())
			require.NoError(t, fixture.state.ResolvePendingOverworldContact())
			require.NotNil(t, fixture.state.CombatScene)
			require.False(t, fixture.state.HasPendingOverworldContact())
			fixture.state.EndCombat(game_state.CombatDefeat)
			require.Equal(t, expectedTime, fixture.state.DateTime)
		})
	}
}

func TestGuardTributeRefusalDefersCombatUntilTalkCleanup(t *testing.T) {
	fixture := newGuardTributeFixture(t, references.Britain, 1)
	fixture.state.PartyState.Inventory.Gold.Set(100)
	beforeTalk := fixture.state.DateTime

	fixture.scene.StartTalkWorkflow(references.Right)
	answerGuardYesNo(t, fixture, ebiten.KeyN)
	answerGuardYesNo(t, fixture, ebiten.KeyN)
	pumpGuardTalkUntilDone(t, fixture)

	require.True(t, fixture.state.HasPendingOverworldContact())
	require.Nil(t, fixture.state.CombatScene)
	expectedAfterTalk := beforeTalk
	expectedAfterTalk.Advance(1)
	require.Equal(t, expectedAfterTalk, fixture.state.DateTime)
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced(),
		"manual Talk time must complete before arrest combat is queued")
	require.False(t, fixture.scene.dialogStack.HasOpenDialog())
	require.NoError(t, fixture.state.ResolvePendingOverworldContact())
	require.NotNil(t, fixture.state.CombatScene)
	require.True(t, fixture.state.IsCriminal())
	beforeCombatReturn := fixture.state.DateTime
	fixture.state.EndCombat(game_state.CombatDefeat)
	require.Equal(t, beforeCombatReturn, fixture.state.DateTime,
		"combat return must not complete the already-owned Talk turn again")
}

func TestNPCInitiatedGuardRefusalDoesNotChargeSecondMinute(t *testing.T) {
	fixture := newGuardTributeFixture(t, references.Britain, 1)
	fixture.state.PartyState.Inventory.Gold.Set(100)
	beforeTalk := fixture.state.DateTime

	fixture.scene.startNPCTalkWorkflow(fixture.guard)
	answerGuardYesNo(t, fixture, ebiten.KeyN)
	answerGuardYesNo(t, fixture, ebiten.KeyN)
	pumpGuardTalkUntilDone(t, fixture)

	require.Equal(t, beforeTalk, fixture.state.DateTime)
	require.Empty(t, fixture.callbacks.TimeAdvanced())
	require.True(t, fixture.state.HasPendingOverworldContact())
	require.NoError(t, fixture.state.ResolvePendingOverworldContact())
	require.NotNil(t, fixture.state.CombatScene)
	fixture.state.EndCombat(game_state.CombatDefeat)
	require.Equal(t, beforeTalk, fixture.state.DateTime)
}

func TestNPCInitiatedGuardSurrenderDoesNotChargeSecondMinute(t *testing.T) {
	fixture := newGuardTributeFixture(t, references.Britain, 1)
	fixture.state.SetCriminalStatus(true)
	fixture.state.DateTime.Hour = 7
	fixture.state.DateTime.Minute = 55

	fixture.scene.startNPCTalkWorkflow(fixture.guard)
	answerGuardYesNo(t, fixture, ebiten.KeyY)
	pumpGuardTalkUntilDone(t, fixture)

	require.Equal(t, references.Yew, fixture.state.MapState.PlayerLocation.Location)
	require.Equal(t, uint8(references.JailReleaseHour), fixture.state.DateTime.Hour)
	require.Equal(t, uint8(15), fixture.state.DateTime.Minute)
	require.Empty(t, fixture.callbacks.TimeAdvanced())
	require.False(t, fixture.state.EventQueueManager.HasActiveWorkflows())
	require.False(t, fixture.scene.dialogStack.HasOpenDialog())
}

func TestPalaceGuardRequiresBadgeBeforePassword(t *testing.T) {
	fixture := newGuardTributeFixture(t, references.PalaceOfBlackthorn, 1)

	fixture.scene.StartTalkWorkflow(references.Right)
	dialog := waitForGuardYesNo(t, fixture)
	require.Equal(t, "Wilt thou come quietly?", dialog.ActivePrompt())
	answerGuardYesNo(t, fixture, ebiten.KeyN)
	pumpGuardTalkUntilDone(t, fixture)

	require.True(t, fixture.state.HasPendingOverworldContact())
	require.NotContains(t, dialog.TranscriptText(), "Dost thou pay?")
}

func TestPalaceGuardAcceptsBadgePasswordWithoutChangingBadge(t *testing.T) {
	fixture := newGuardTributeFixture(t, references.PalaceOfBlackthorn, 1)
	fixture.state.BadgeState = game_state.ToggleItemState{
		IsActive: true, TurnsRemaining: game_state.ToggleItemDuration,
	}

	fixture.scene.StartTalkWorkflow(references.Right)
	dialog := waitForGuardYesNo(t, fixture)
	require.Equal(t, "Your response?", dialog.ActivePrompt())
	submitGuardText(t, fixture, dialog, []ebiten.Key{ebiten.KeyI, ebiten.KeyM, ebiten.KeyP, ebiten.KeyE})
	pumpGuardTalkUntilDone(t, fixture)

	require.Contains(t, dialog.TranscriptText(), "Pass, friend!")
	require.False(t, fixture.state.HasPendingOverworldContact())
	require.True(t, fixture.state.IsBadgeActive())
	require.Equal(t, game_state.ToggleItemDuration, fixture.state.BadgeState.TurnsRemaining)
	require.Equal(t, references.PalaceOfBlackthorn, fixture.state.MapState.PlayerLocation.Location)
}

func TestPalaceGuardRejectsWrongBadgePassword(t *testing.T) {
	fixture := newGuardTributeFixture(t, references.PalaceOfBlackthorn, 1)
	fixture.state.BadgeState = game_state.ToggleItemState{
		IsActive: true, TurnsRemaining: game_state.ToggleItemDuration,
	}

	fixture.scene.StartTalkWorkflow(references.Right)
	dialog := waitForGuardYesNo(t, fixture)
	submitGuardText(t, fixture, dialog, []ebiten.Key{ebiten.KeyN, ebiten.KeyO, ebiten.KeyP, ebiten.KeyE})
	dialog = waitForGuardYesNo(t, fixture)
	require.Equal(t, "Wilt thou come quietly?", dialog.ActivePrompt())
	answerGuardYesNo(t, fixture, ebiten.KeyN)
	pumpGuardTalkUntilDone(t, fixture)

	require.True(t, fixture.state.HasPendingOverworldContact())
}

type guardTributeFixture struct {
	state     *game_state.GameState
	callbacks *game_state.MockSystemCallbacks
	scene     *GameScene
	guard     *map_units.NPCFriendly
}

func newGuardTributeFixture(
	t *testing.T,
	location references.Location,
	activeMembers int,
) *guardTributeFixture {
	t.Helper()

	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(location).
		WithSystemCallbacks().
		WithRandomSeed(1).
		Build()
	for index := range party_state.MaxActiveParty {
		character := state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapUnrecruited
		character.SetStatus(party_state.Dead)
	}
	for index := range activeMembers {
		character := state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapActive
		character.SetStatus(party_state.Good)
		character.SetMaxHp(100)
		character.SetHp(100)
	}

	floor := references.FloorNumber(0)
	playerPosition := references.Position{X: 5, Y: 5}
	guardPosition := references.Position{X: 6, Y: 5}
	state.MapState.PlayerLocation.Position = playerPosition
	state.MapState.PlayerLocation.Floor = floor
	guardReference := references.NPCReference{
		DialogNumber: byte(references.DialogGuard),
		Location:     location,
		Schedule:     references.CreateNPCScheduledFixedOneLocation(guardPosition, floor),
	}
	guardReference.SetKeyIndex(indexes.GuardKeyIndex)
	guard := map_units.NewNPCFriendly(guardReference, 0)
	guard.PlaceAtScheduleSlot(0)
	state.CurrentNPCAIController.GetNpcs().ReplaceWith([]map_units.MapUnit{guard})

	scene := &GameScene{
		gameState: state,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
	}

	return &guardTributeFixture{state: state, callbacks: callbacks, scene: scene, guard: guard}
}

func answerGuardYesNo(
	t *testing.T,
	fixture *guardTributeFixture,
	choice ebiten.Key,
) *widgets.InteractionTranscript {
	t.Helper()

	dialog := waitForGuardYesNo(t, fixture)
	fixture.scene.keyboard.ClearFrameConsumption()
	fixture.scene.keyboard.SetForceWaitAnyKey(-1000)
	fixture.scene.keyboard.SimulateKeyPress(choice)
	dialog.Update()
	fixture.scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	require.NotEmpty(t, dialog.Response(), "the simulated Guard choice must reach the transcript")
	require.Same(t, dialog, fixture.scene.dialogStack.PeekTopModalDialog(),
		"the submitted Guard choice must retain its transcript")

	return dialog
}

func submitGuardText(
	t *testing.T,
	fixture *guardTributeFixture,
	dialog *widgets.InteractionTranscript,
	keys []ebiten.Key,
) {
	t.Helper()
	for _, key := range keys {
		fixture.scene.keyboard.ClearFrameConsumption()
		fixture.scene.keyboard.SetForceWaitAnyKey(-1000)
		fixture.scene.keyboard.SimulateKeyPress(key)
		dialog.Update()
	}
	fixture.scene.keyboard.ClearFrameConsumption()
	fixture.scene.keyboard.SetForceWaitAnyKey(-1000)
	fixture.scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	require.True(t, dialog.InputComplete())
}

func waitForGuardYesNo(t *testing.T, fixture *guardTributeFixture) *widgets.InteractionTranscript {
	t.Helper()

	for range 100 {
		if dialog, ok := fixture.scene.dialogStack.PeekTopModalDialog().(*widgets.InteractionTranscript); ok &&
			dialog.ActivePrompt() != "" && !dialog.InputComplete() {
			return dialog
		}
		fixture.state.ElapsedMs += DialogPollingRateMs
		fixture.state.EventQueueManager.Update()
	}
	t.Fatal("guard Talk workflow did not show a Yes/No dialog")

	return nil
}

func pumpGuardTalkUntilDone(t *testing.T, fixture *guardTributeFixture) {
	t.Helper()

	for range 500 {
		if overlay, ok := fixture.scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog); ok {
			overlay.Dismiss()
		}
		if dialog, ok := fixture.scene.dialogStack.PeekTopModalDialog().(*widgets.InteractionTranscript); ok &&
			dialog.ActivePrompt() == "" && !dialog.InputComplete() {
			fixture.scene.keyboard.ClearFrameConsumption()
			fixture.scene.keyboard.SetForceWaitAnyKey(-1000)
			fixture.scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			dialog.Update()
		}
		fixture.state.ElapsedMs += 1000
		fixture.state.EventQueueManager.Update()
		if !fixture.state.EventQueueManager.HasActiveWorkflows() {
			return
		}
	}
	t.Fatalf("guard Talk workflow did not complete: top dialog %T, dialog count %d",
		fixture.scene.dialogStack.PeekTopModalDialog(), fixture.scene.dialogStack.Count())
}

func pumpGuardTalkWithoutInputUntilDone(t *testing.T, fixture *guardTributeFixture) {
	t.Helper()

	for range 100 {
		fixture.state.ElapsedMs += 1000
		fixture.state.EventQueueManager.Update()
		if !fixture.state.EventQueueManager.HasActiveWorkflows() {
			return
		}
	}
	t.Fatal("guard Talk workflow required unsupported acknowledgement input")
}
