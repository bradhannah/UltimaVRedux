package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	conversationpatchdata "github.com/bradhannah/UltimaVRedux/data/conversation-patches"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/catalog"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/originaldata"
)

const ladyJanellDialogNumber = 11

// newCircleStudyTalkFixture stands the party beside Lady Janell with an Avatar,
// a Wizard and a Fighter at level 7 and enough gold for two of them.
func newCircleStudyTalkFixture(t *testing.T) *merchantTalkFixture {
	t.Helper()
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Lycaeum).
		WithSystemCallbacks().
		WithRandomSeed(1).
		Build()
	reference := references.NPCReference{
		Location: references.Lycaeum, DialogNumber: ladyJanellDialogNumber, NpcType: references.NoStatedNpc,
		Schedule: references.NPCSchedule{X: [3]byte{5, 5, 5}, Y: [3]byte{5, 5, 5}},
	}
	reference.SetKeyIndex(indexes.TownsPersonKeyIndex)
	npc := map_units.NewNPCFriendly(reference, 1)
	npc.SetPos(references.Position{X: 5, Y: 5})
	npc.SetFloor(state.MapState.PlayerLocation.Floor)
	npc.SetVisible(true)
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(map_units.MapUnits{npc})
	for index, class := range []party_state.CharacterClass{party_state.Avatar, party_state.Wizard, party_state.Fighter} {
		member := state.PartyState.GetCharacter(index)
		member.Class, member.Level = class, 7
		member.SetStatus(party_state.Good)
	}
	state.PartyState.Inventory.Gold.Set(6819)
	scene := &GameScene{
		gameState:   state,
		keyboard:    input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		spriteSheet: &sprites.SpriteSheet{SpriteImage: ebiten.NewImage(512, 256)},
	}
	callbacks.BindGameState(state)

	return &merchantTalkFixture{
		state: state, callbacks: callbacks, scene: scene, npc: npc, direction: placePlayerNextToMerchant(state, npc),
	}
}

func waitForCircleStudyDialog(t *testing.T, fixture *merchantTalkFixture) *widgets.CharacterSelectionDialog {
	t.Helper()
	var dialog *widgets.CharacterSelectionDialog
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		candidate, ok := fixture.scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
		if ok {
			dialog = candidate
		}

		return ok
	})

	return dialog
}

func TestCircleStudyOpensAfterTheOfferAndTeachesOneMemberAtATime(t *testing.T) {
	fixture := newCircleStudyTalkFixture(t)
	party := &fixture.state.PartyState
	fixture.scene.StartTalkWorkflow(fixture.direction)
	talk := waitForLinearTalkDialog(t, fixture)
	talk.callbacks.BeginCircleStudy() // What a yes at her question does.
	talk.conversationCompleted = true
	fixture.scene.dialogStack.RemoveWidgetInstance(talk)

	study := waitForCircleStudyDialog(t, fixture)
	require.NotNil(t, study, "the study list opens once the talk has closed")
	settle := func() {
		pumpMerchantWorkflowUntil(t, fixture, func() bool { return !fixture.scene.keyboard.IsWaitingForRelease() })
	}
	// choose presses Enter on the inspected row and answers the question it
	// raises. The question box's own typing is covered by its widget tests, so
	// the answer is given the way its Yes/No command gives it.
	choose := func(rosterIndex int, agreed bool) {
		settle()
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		var prompt *widgets.InputBox
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			candidate, asked := fixture.scene.dialogStack.PeekTopModalDialog().(*widgets.InputBox)
			prompt = candidate

			return asked
		})
		assert.Regexp(t, `^Each member may study once only\.\nPay 3000 gold for \w+ to study\?$`, prompt.Question)
		prompt.InvokeOnDismiss()
		fixture.scene.answerCircleStudy(study, rosterIndex, agreed)
		assert.Same(t, study, fixture.scene.dialogStack.PeekTopModalDialog(), "the list stays open for another member")
		settle()
	}
	refused := func() {
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		for range 5 {
			fixture.state.ElapsedMs += merchantDialogPollingRateMs
			fixture.state.EventQueueManager.Update()
		}
		assert.Same(t, study, fixture.scene.dialogStack.PeekTopModalDialog(), "a member who cannot study is not asked")
	}

	choose(0, false)
	assert.False(t, party.GetCharacter(0).HasCircleStudy(), "a no changes nothing")
	assert.Equal(t, uint16(6819), party.Inventory.Gold.Get())
	choose(0, true)
	assert.True(t, party.GetCharacter(0).HasCircleStudy())
	assert.Equal(t, uint16(3819), party.Inventory.Gold.Get())
	refused()
	assert.Equal(t, uint16(3819), party.Inventory.Gold.Get(), "a member who has studied cannot pay twice")

	pressMerchantDialogKey(fixture, ebiten.KeyDown)
	choose(1, true)
	assert.True(t, party.GetCharacter(1).HasCircleStudy())
	assert.Equal(t, uint16(819), party.Inventory.Gold.Get())
	pressMerchantDialogKey(fixture, ebiten.KeyDown)
	refused()
	assert.False(t, party.GetCharacter(2).HasCircleStudy(), "a Fighter is refused")
	assert.Equal(t, uint16(819), party.Inventory.Gold.Get())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool { return !fixture.state.EventQueueManager.HasActiveWorkflows() })
	assert.Zero(t, fixture.scene.dialogStack.Count())
}

func TestCircleStudyStaysShutWithoutACompletedOffer(t *testing.T) {
	for name, offered := range map[string]bool{"talk ended without the offer": false, "talk removed after the offer": true} {
		t.Run(name, func(t *testing.T) {
			fixture := newCircleStudyTalkFixture(t)
			fixture.scene.StartTalkWorkflow(fixture.direction)
			talk := waitForLinearTalkDialog(t, fixture)
			if offered {
				talk.callbacks.BeginCircleStudy()
			} else {
				talk.conversationCompleted = true
			}
			fixture.scene.dialogStack.RemoveWidgetInstance(talk)

			pumpMerchantWorkflowUntil(t, fixture, func() bool { return !fixture.state.EventQueueManager.HasActiveWorkflows() })

			assert.Zero(t, fixture.scene.dialogStack.Count())
			assert.Equal(t, uint16(6819), fixture.state.PartyState.Inventory.Gold.Get())
		})
	}
}

// Each row says in words why a member may or may not study, and what it costs.
func TestCircleStudyOptionsExplainEveryRow(t *testing.T) {
	fixture := newCircleStudyTalkFixture(t)
	party := &fixture.state.PartyState
	party.GetCharacter(1).SetCircleStudy(true)
	describe := func(option widgets.CharacterSelectionOption) string {
		parts := []string{option.Status}
		for _, fact := range option.Facts {
			parts = append(parts, fact.Label+" "+fact.Value)
		}

		return strings.Join(append(parts, option.Details...), " | ")
	}

	options := fixture.scene.circleStudyOptions()

	require.Len(t, options, party.GetActivePartyCount())
	assert.Equal(t, widgets.CharacterSelectionAvailable, options[0].State)
	assert.Equal(t, "Ready | Circle 7 to 8 | Cost 3000 gold | Gold 6819 | After 3819 | "+
		"One Circle above level, for good.", describe(options[0]))
	assert.Equal(t, widgets.CharacterSelectionUnavailable, options[1].State)
	assert.Equal(t, "Done | Circle 8 | Casts one Circle above level.", describe(options[1]))
	assert.Equal(t, widgets.CharacterSelectionUnavailable, options[2].State)
	assert.Equal(t, "Can't | Class Fighter | A Fighter casts no spells.", describe(options[2]))

	party.Inventory.Gold.Set(game_state.CircleStudyPrice - 1)
	poor := fixture.scene.circleStudyOptions()[0]
	assert.Equal(t, widgets.CharacterSelectionUnavailable, poor.State)
	assert.Equal(t, "Short | Circle 7 to 8 | Cost 3000 gold | Gold 2999 | Not enough gold to study.", describe(poor))
}

// A playtest at 1440p showed a title cut to its last word and names cut with
// dots, which a 720p check had missed. Every state is checked with the longest
// names at every supported window size.
func TestCircleStudyListShowsEverythingWholeAtEverySupportedSize(t *testing.T) {
	fixture := newCircleStudyTalkFixture(t)
	party := &fixture.state.PartyState
	require.GreaterOrEqual(t, party.GetActivePartyCount(), 5)
	rename := func(index int, name string) {
		member := party.GetCharacter(index)
		member.Name = [party_state.NMaxPlayerNameSize]byte{}
		copy(member.Name[:], name)
	}
	rename(0, "Beradius") // An Avatar may carry an eight-letter name.
	rename(2, "Geoffrey")
	party.GetCharacter(1).SetCircleStudy(true)
	fallen, adept := party.GetCharacter(3), party.GetCharacter(4)
	fallen.Class = party_state.Bard
	fallen.SetStatus(party_state.Dead)
	adept.Class, adept.Level = party_state.Wizard, 8
	adept.SetStatus(party_state.Good)
	manager := display.GetManager()
	width, height := manager.GetCurrentSize()
	t.Cleanup(func() { manager.PublishLogicalSize(width, height) })

	for _, gold := range []uint16{6819, 0} {
		party.Inventory.Gold.Set(gold)
		for _, size := range config.SupportedWindowResolutions() {
			manager.PublishLogicalSize(size.X, size.Y)
			dialog := fixture.scene.newCircleStudyDialog(func(int) {}, func() {})
			assert.Empty(t, dialog.LayoutProblemsAt(size.X, size.Y), "gold %d at %dx%d", gold, size.X, size.Y)
		}
	}
}

// Her spoken price is authored text, so it is checked against the real one.
func TestLadyJanellSpeaksTheRealCircleStudyPrice(t *testing.T) {
	originalCatalog, err := catalog.LoadOriginal(originaldata.Directory(t), conversationpatchdata.Files)
	require.NoError(t, err)
	program, exists := originalCatalog.Original(semantic.OriginalSourceKeep, ladyJanellDialogNumber)
	require.True(t, exists)
	definition, err := program.Definition()
	require.NoError(t, err)
	document, err := semantic.Marshal(definition)
	require.NoError(t, err)

	assert.Contains(t, string(document), fmt.Sprintf("For %d gold crowns", game_state.CircleStudyPrice))
}
