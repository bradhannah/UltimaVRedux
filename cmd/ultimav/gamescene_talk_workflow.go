package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func (g *GameScene) startSurfaceTalkWorkflow(direction references.Direction) {
	g.startTalkWorkflow(direction, nil, true, true)
}

// startNPCTalkWorkflow routes NPC-initiated Talk through the same lifecycle as
// manual Talk without charging a second minute for the preceding command.
func (g *GameScene) startNPCTalkWorkflow(npc *map_units.NPCFriendly) {
	g.startTalkWorkflow(references.NoneDirection, npc, false, false)
}

// startNPCGuardArrestWorkflow handles hostile Guard contact without entering
// Talk dispatch or consuming a TALK3 schedule action.
func (g *GameScene) startNPCGuardArrestWorkflow(source *map_units.NPCFriendly) {
	if g.gameState.MapState.PlayerLocation.Location == references.PalaceOfBlackthorn {
		if g.gameState.PartyState.GetLivingPartyCount() > 0 {
			g.startBlackthornInterrogationWorkflow()
		}

		return
	}
	workflowState := new(merchantTalkWorkflowState)
	dialog := widgets.NewInteractionTranscript(g.keyboard, widgets.InteractionTranscriptConfig{
		MaxLines:       6,
		MaxInputLength: 14,
		WidthSamples: []string{
			"Thou art under arrest!",
			"Wilt thou come quietly?  > Yes",
			"The guard strikes thee unconscious!",
			"Thou dost awaken to...",
		},
	})
	workflowState.interactionTranscript = dialog

	g.gameState.EventQueueManager.Workflow("guard_arrest_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			g.dialogStack.PushModalDialog(dialog)
		}).
		ThenSequence(g.thenShowArrestDialog(source, workflowState, dialog)).
		Finally(func() {
			g.finishMerchantWorkflow(workflowState)
		}).
		Start()
}

func (g *GameScene) startTalkWorkflow(
	direction references.Direction,
	initiatingNPC *map_units.NPCFriendly,
	advanceTime bool,
	completeSurfaceCycle bool,
) {
	originMapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()
	originLocation := g.gameState.MapState.PlayerLocation.Location
	// Variables captured in closure for runtime evaluation
	var npc map_units.MapUnit
	var npcFriendly *map_units.NPCFriendly
	var talkType TalkType
	var normalConversationCompleted bool
	var normalDialog *LinearTalkDialog
	var theftResult game_state.FaulineiConversationTheftResult
	merchantState := &merchantTalkWorkflowState{advanceTime: advanceTime && !completeSurfaceCycle}

	g.gameState.EventQueueManager.Workflow("talk_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			if initiatingNPC != nil {
				npc = initiatingNPC
			} else {
				const maxTalkDistance = 2
				_, npc = g.gameState.FindNPCInDirection(
					g.gameState.MapState.PlayerLocation.Position,
					direction,
					maxTalkDistance,
				)
			}

			if npc == nil {
				talkType = TalkTypeNoNPC

				return
			}

			var ok bool
			npcFriendly, ok = npc.(*map_units.NPCFriendly)
			if ok && npcFriendly != nil {
				npcFriendly.ApplyTalk3OneShot()
				talkType = g.determineTalkType(npcFriendly, initiatingNPC != nil)
			} else {
				talkType = TalkTypeNonFriendly
			}
		}).If(

		func() bool { return talkType == TalkTypeNoNPC },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(g.thenShowConfirmationDialog("No-one to talk to!"))
		}).If(

		func() bool { return talkType == TalkTypeShadowlordHostile },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(g.thenShowConfirmationDialog("\"Begone, vermin!\"")).
				ThenSequence(g.thenApplyShadowlordDamage()).
				Then(func() {
					npcFriendly.MapUnitDetails().SetAIOverride(
						references.FleeFromAvatar,
						references.AIOverrideProvenanceAutomaticAstaroth,
					)
				})
		}).If(

		func() bool { return talkType == TalkTypeShadowlordFlee },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(g.thenShowConfirmationDialog("\"Don't hurt me! Please go away!\""))
		}).If(

		func() bool { return talkType == TalkTypeGuard },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(g.thenHandleGuardTalk(npcFriendly, merchantState))
		}).If(

		func() bool { return talkType == TalkTypeSleeping },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(g.thenShowConfirmationDialog("\"Zzzzzz...\""))
		}).If(

		func() bool { return talkType == TalkTypeMerchant },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(g.thenHandleMerchantTalk(npcFriendly, merchantState))
		}).If(

		func() bool { return talkType == TalkTypeNormal },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(g.thenShowLinearTalkDialog(npcFriendly, &normalConversationCompleted, &normalDialog)).
				Then(func() {
					if normalConversationCompleted {
						theftResult = g.gameState.ApplyFaulineiConversationTheft()
					}
				}).If(
				// This closure must read the result after the preceding event mutates it.
				//nolint:gocritic // A method value would capture the current zero-value receiver.
				func() bool { return theftResult.Occurred() },
				func(theftQ *event_queue.SequentialEventBuilder) {
					theftQ.ThenSequence(g.thenShowConfirmationDialog("Something was stolen!"))
				}).If(
				// Read the dialog after the talk event has assigned it. ref:PROGRESS-0005
				func() bool { return normalConversationCompleted && normalDialog.CircleStudyRequested() },
				func(studyQ *event_queue.SequentialEventBuilder) {
					studyQ.ThenSequence(g.thenRunCircleStudy())
				})
		}).If(

		func() bool { return talkType == TalkTypeNonFriendly },
		func(q *event_queue.SequentialEventBuilder) {
			q.ThenSequence(g.thenShowConfirmationDialog("Can't talk to that!"))
		}).Finally(

		func() {
			if normalDialog != nil {
				normalDialog.Close()
			}
			g.finishMerchantWorkflow(merchantState)
			if completeSurfaceCycle {
				state := surfaceCommandCompleted
				if talkType == TalkTypeNoNPC || talkType == TalkTypeNonFriendly {
					state = surfaceCommandRejected
				}
				result := newSurfaceCommandResult(state, originMapType)
				result.MapTransitioned = g.gameState.MapState.PlayerLocation.Location != originLocation
				g.completeSurfaceCommand(result)
			}
		}).
		Start()
}

// determineTalkType determines the talk type for a friendly NPC based on their current state.
// This evaluates Shadowlord effects, guard status, and dialog availability.
func (g *GameScene) determineTalkType(npc *map_units.NPCFriendly, _ bool) TalkType {
	// Shadowlord responses require both persisted per-unit cause and matching
	// active automatic presence. Generic law/flee states and Yell summons cannot
	// select these branches even though they reuse the same AI values.
	provenance := npc.MapUnitDetails().AIOverrideProvenance()
	switch {
	case provenance == references.AIOverrideProvenanceAutomaticAstaroth &&
		g.gameState.ShadowlordState != nil &&
		g.gameState.ShadowlordState.AutomaticPresence == references.Astaroth:
		return TalkTypeShadowlordHostile
	case provenance == references.AIOverrideProvenanceAutomaticNosfentor &&
		g.gameState.ShadowlordState != nil &&
		g.gameState.ShadowlordState.AutomaticPresence == references.Nosfentor:
		return TalkTypeShadowlordFlee
	}

	dialogNum := references.DialogNumber(npc.NPCReference.DialogNumber)
	if g.gameState.IsCriminal() && npc.NPCReference.GetNPCType() == references.Guard {
		return TalkTypeGuard
	}
	// ref:CRIME-0001: conversation selector 0xff selects procedural tribute;
	// actor shape is not part of the dispatch rule.
	if dialogNum == references.DialogGuard {
		return TalkTypeGuard
	}

	// Check if NPC is sleeping (on a bed tile)
	// ref:TALK-0005: NPCs on bed tiles say "Zzzzzz...".
	// Note: Use GetTileTopMapOnlyTile to get the underlying map tile, not the NPC sprite
	npcPos := npc.Pos()
	npcMap := g.gameState.MapState.LayeredMaps.GetLayeredMap(
		references.SmallMapType,
		g.gameState.MapState.PlayerLocation.Floor,
	)
	if npcMap != nil {
		npcTile := npcMap.GetTileTopMapOnlyTile(&npcPos)
		if npcTile != nil && npcTile.Index.IsBed() {
			return TalkTypeSleeping
		}
	}

	if isMerchantTalkDialog(dialogNum) {
		return TalkTypeMerchant
	}

	// Default to normal conversation
	return TalkTypeNormal
}

func isMerchantTalkDialog(dialogNumber references.DialogNumber) bool {
	_, err := references.MerchantFamilyFromDialog(dialogNumber)

	return err == nil
}
