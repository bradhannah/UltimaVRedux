package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/conversation"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

var errUnsupportedConversationKnowledge = errors.New("unsupported conversation knowledge")

const selflessDonationKarma = 3

// GameActionCallbacks implements the ActionCallbacks interface to integrate
// the semantic conversation engine with the game's state and UI systems
type GameActionCallbacks struct {
	gameScene        *GameScene
	npcRef           references.NPCReference
	npc              *map_units.NPCFriendly
	npcName          string
	knownSpeaker     string // Journal attribution label, independent of NAME dialogue progress.
	output           strings.Builder
	dialogOutput     *text.Output // Reference to dialog's output for Crout() calls
	pendingKnowledge []pendingConversationKnowledge
	notifiedResponse uint64
	// circleStudyRequested is set when the conversation ends by offering circle study.
	circleStudyRequested bool
}

type pendingConversationKnowledge struct {
	knowledge semantic.KnowledgeID
	boundary  conversation.DiscoveryBoundary
	source    game_state.KnowledgeSource
}

var _ conversation.DeferredKnowledgeCallbacks = (*GameActionCallbacks)(nil)

// QueueKnowledge captures provenance at execution, without granting unseen lore.
func (g *GameActionCallbacks) QueueKnowledge(
	knowledge semantic.KnowledgeID, boundary conversation.DiscoveryBoundary,
) error {
	speaker := g.knownSpeaker
	if speaker == "" {
		speaker = journal.UnknownSpeaker
	}
	g.pendingKnowledge = append(g.pendingKnowledge, pendingConversationKnowledge{
		knowledge: knowledge, boundary: boundary, source: game_state.DialogueKnowledgeSource(speaker),
	})

	return nil
}

func (g *GameActionCallbacks) commitPresentedKnowledge(boundary conversation.DiscoveryBoundary) {
	pending := g.pendingKnowledge
	g.pendingKnowledge = nil
	for _, discovery := range pending {
		if discovery.boundary.Response != boundary.Response || discovery.boundary.SourceOffset > boundary.SourceOffset {
			g.pendingKnowledge = append(g.pendingKnowledge, discovery)

			continue
		}
		before := g.gameScene.journalVisits.Revision
		g.gameScene.journalNoticeSuppressed = g.notifiedResponse == boundary.Response
		err := g.discoverKnowledge(discovery.knowledge, discovery.source)
		g.gameScene.journalNoticeSuppressed = false
		if err != nil {
			panic(fmt.Sprintf("committing validated conversation discovery: %v", err))
		}
		if g.gameScene.journalVisits.Revision != before {
			g.notifiedResponse = boundary.Response
		}
	}
}

// NewGameActionCallbacks creates a new callback implementation for the given game scene and NPC
func NewGameActionCallbacks(
	gameScene *GameScene,
	npcRef references.NPCReference,
	npc *map_units.NPCFriendly,
) *GameActionCallbacks {
	return &GameActionCallbacks{
		gameScene: gameScene, npcRef: npcRef, npc: npc, npcName: "NPC",
		knownSpeaker: "", output: strings.Builder{}, dialogOutput: nil,
		pendingKnowledge: nil, notifiedResponse: 0, circleStudyRequested: false,
	}
}

// SetNPCName supplies the validated conversation name used by gameplay actions.
func (g *GameActionCallbacks) SetNPCName(name string) {
	if strings.TrimSpace(name) != "" {
		g.npcName = strings.TrimSpace(name)
	}
}

// GetAccumulatedOutput returns all text that has been accumulated via ShowOutput callback.
// This text buffer captures NPC dialogue and system messages during conversation processing.
func (g *GameActionCallbacks) GetAccumulatedOutput() string {
	return g.output.String()
}

// ClearAccumulatedOutput resets the accumulated output buffer.
// Called at the start of each conversation processing cycle to clear previous messages.
func (g *GameActionCallbacks) ClearAccumulatedOutput() {
	g.output.Reset()
}

// SetDialogOutput sets the reference to the dialog's output system for direct text output.
// MUST be called during dialog initialization. Missing dialogOutput will cause panics
// in methods that write to it (Crout, IncreaseKarma, DecreaseKarma, DeductGold).
func (g *GameActionCallbacks) SetDialogOutput(output *text.Output) {
	g.dialogOutput = output
}

// === Core Action Callbacks ===

// JoinParty attempts to add the NPC to the player's party.
// Returns a capacity refusal without an error; unexpected recruitment failures return both.
// On success, displays a personalized join message.
//
// Process:
// 1. Check party room availability
// 2. Extract NPC name from conversation engine
// 3. Atomically recruit through GameState (roster, source absence, live removal)
// 4. Display personalized success message
func (g *GameActionCallbacks) JoinParty() (conversation.JoinPartyOutcome, error) {
	npcName := g.npcName
	if err := g.gameScene.gameState.RecruitNPC(g.npc, npcName); err != nil {
		if errors.Is(err, game_state.ErrPartyCapacity) {
			g.ShowOutput("\nThou hast no room for me in thy party! Seek me again if one of thy members doth leave thee.\n")

			return conversation.JoinPartyCapacityRefused, nil
		}

		g.ShowOutput(fmt.Sprintf("%v\n", err))
		wrappedErr := fmt.Errorf("recruiting NPC: %w", err)

		return conversation.JoinPartyUnexpectedError, wrappedErr
	}

	// Display personalized success message with actual NPC name in yellow
	g.dialogOutput.AddTextWithColor(npcName+" has joined thee!", ucolor.EGAYellow)

	return conversation.JoinPartySuccess, nil
}

// CallGuards activates guards and triggers civilian flee behavior.
// Guard activation follows ref:CRIME-0002:
// - Guards/ShadowLords/Devils become hostile
// - Other NPCs have 50% chance to flee
// - Player marked as criminal (first guard to reach will arrest)
func (g *GameActionCallbacks) CallGuards() {
	state := g.gameScene.gameState
	state.SetCriminalStatus(true)
	if _, err := state.ActivateGuards(); err != nil {
		logging.Warnf(logging.CategoryGameState, "Unable to activate guards: %v", err)
	}
}

// IncreaseKarma improves the party's karma by 1 point.
// Displays yellow text feedback via dialog output system.
// Plays pleasant chime sound effect for audio feedback.
func (g *GameActionCallbacks) IncreaseKarma() {
	amount := 1
	g.gameScene.gameState.AdjustPartyKarma(amount)

	// Display yellow text feedback for karma increase
	message := fmt.Sprintf("Karma improved by %d!", amount)
	g.dialogOutput.AddTextWithColor(message, ucolor.EGAYellow)

	// Audio feedback - pleasant chime
	g.gameScene.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundKarmaUp)
}

// DecreaseKarma worsens the party's karma by 1 point.
// Displays yellow text feedback via dialog output system.
// Plays warning tone sound effect for audio feedback.
func (g *GameActionCallbacks) DecreaseKarma() {
	amount := 1
	g.gameScene.gameState.AdjustPartyKarma(-amount)

	// Display yellow text feedback for karma decrease
	message := fmt.Sprintf("Karma worsened by %d!", amount)
	g.dialogOutput.AddTextWithColor(message, ucolor.EGAYellow)

	// Audio feedback - warning tone
	g.gameScene.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundKarmaDown)
}

// GoToJail delegates the Redux conversation action to the canonical Yew sentence.
func (g *GameActionCallbacks) GoToJail() {
	g.dialogOutput.AddTextWithColor("Guards grab thee and throw thee in jail!", ucolor.EGAYellow)
	g.gameScene.gameState.ExecuteJailSentence()
}

// BeginCircleStudy records that circle study opens once this talk has closed.
// The talk workflow owns the screen; ref:PROGRESS-0005.
func (g *GameActionCallbacks) BeginCircleStudy() {
	g.circleStudyRequested = true
}

// GiveItem gives the specified item to the player's inventory.
// Maps original Ultima V item IDs to current inventory system with proper limits
// (ref:TALK-0003; the mapping lives in GameState.GiveConversationItem).
// A gift at its carrying limit grants nothing, and the receipt says so.
func (g *GameActionCallbacks) GiveItem(itemID int) {
	name, outcome := g.gameScene.gameState.GiveConversationItem(itemID)
	switch outcome {
	case game_state.GiftUnknown:
		logging.Warnf(logging.CategoryConversation, "GiveItem: Unknown itemID %d", itemID)
		g.dialogOutput.AddTextWithColor(fmt.Sprintf("Received: Item #%d", itemID), ucolor.EGAYellow)
	case game_state.GiftAtLimit:
		g.dialogOutput.AddTextWithColor("Thou canst carry no more "+name+"!", ucolor.EGAYellow)
	case game_state.GiftGranted:
		logging.Infof(logging.CategoryConversation, "GiveItem: %s (itemID %d)", name, itemID)
		g.dialogOutput.AddTextWithColor("Received: "+name, ucolor.EGAYellow)
	}
}

// DiscoverKnowledge records player knowledge revealed by semantic conversation actions.
func (g *GameActionCallbacks) DiscoverKnowledge(knowledge semantic.KnowledgeID) error {
	return g.discoverKnowledge(knowledge, game_state.DialogueKnowledgeSource(g.npcName))
}

func (g *GameActionCallbacks) discoverKnowledge(
	knowledge semantic.KnowledgeID, source game_state.KnowledgeSource,
) error {
	if id, ok := strings.CutPrefix(string(knowledge), "journal."); ok {
		g.gameScene.gameState.DiscoverJournalKnowledge(id, source)

		return nil
	}
	if spell, supported := conversationSpellKnowledge(knowledge); supported {
		g.gameScene.gameState.DiscoverSpellKnowledge(spell, source)

		return nil
	}
	if lord, kind, supported := conversationShadowlordKnowledge(knowledge); supported {
		g.gameScene.gameState.DiscoverShadowlordKnowledge(lord, kind, source)

		return nil
	}
	if shrineIndex, supported := conversationShrineMantraKnowledge(knowledge); supported {
		g.gameScene.gameState.DiscoverShrineMantraKnowledge(shrineIndex, source)

		return nil
	}
	if dungeon, supported := conversationWordOfPowerKnowledge(knowledge); supported {
		g.gameScene.gameState.DiscoverWordOfPowerKnowledge(dungeon, source)

		return nil
	}

	return fmt.Errorf("%w %q", errUnsupportedConversationKnowledge, knowledge)
}

func conversationSpellKnowledge(knowledge semantic.KnowledgeID) (references.Spell, bool) {
	definitions := [...]struct {
		knowledge semantic.KnowledgeID
		spell     references.Spell
	}{
		{knowledge: semantic.KnowledgeSpellAnYlem, spell: references.AnYlem},
		{knowledge: semantic.KnowledgeSpellInXenMani, spell: references.InXenMani},
		{knowledge: semantic.KnowledgeSpellRelXenBet, spell: references.RelXenBet},
	}
	for _, definition := range definitions {
		if definition.knowledge == knowledge {
			return definition.spell, true
		}
	}

	return 0, false
}

func conversationShadowlordKnowledge(
	knowledge semantic.KnowledgeID,
) (references.Shadowlord, game_state.ShadowlordKnowledgeKind, bool) {
	definitions := [...]struct {
		knowledge semantic.KnowledgeID
		lord      references.Shadowlord
		kind      game_state.ShadowlordKnowledgeKind
	}{
		{semantic.KnowledgeShadowlordFaulineiName, references.Faulinei, game_state.ShadowlordKnowledgeName},
		{semantic.KnowledgeShadowlordFaulineiAssociation, references.Faulinei, game_state.ShadowlordKnowledgeAssociation},
		{semantic.KnowledgeShadowlordAstarothName, references.Astaroth, game_state.ShadowlordKnowledgeName},
		{semantic.KnowledgeShadowlordAstarothAssociation, references.Astaroth, game_state.ShadowlordKnowledgeAssociation},
		{semantic.KnowledgeShadowlordNosfentorName, references.Nosfentor, game_state.ShadowlordKnowledgeName},
		{semantic.KnowledgeShadowlordNosfentorAssociation, references.Nosfentor, game_state.ShadowlordKnowledgeAssociation},
	}
	for _, definition := range definitions {
		if definition.knowledge == knowledge {
			return definition.lord, definition.kind, true
		}
	}

	return references.NoShadowlord, "", false
}

func conversationShrineMantraKnowledge(knowledge semantic.KnowledgeID) (int, bool) {
	ids := [...]semantic.KnowledgeID{
		semantic.KnowledgeShrineHonestyMantra,
		semantic.KnowledgeShrineCompassionMantra,
		semantic.KnowledgeShrineValourMantra,
		semantic.KnowledgeShrineJusticeMantra,
		semantic.KnowledgeShrineSacrificeMantra,
		semantic.KnowledgeShrineHonorMantra,
		semantic.KnowledgeShrineSpiritualityMantra,
		semantic.KnowledgeShrineHumilityMantra,
	}
	for shrineIndex, candidate := range ids {
		if candidate == knowledge {
			return shrineIndex, true
		}
	}

	return 0, false
}

func conversationWordOfPowerKnowledge(knowledge semantic.KnowledgeID) (string, bool) {
	definitions := [...]struct {
		knowledge semantic.KnowledgeID
		dungeon   string
	}{
		{semantic.KnowledgeDungeonDeceitWordOfPower, "deceit"},
		{semantic.KnowledgeDungeonDespiseWordOfPower, "despise"},
		{semantic.KnowledgeDungeonDestardWordOfPower, "destard"},
		{semantic.KnowledgeDungeonWrongWordOfPower, "wrong"},
		{semantic.KnowledgeDungeonCovetousWordOfPower, "covetous"},
		{semantic.KnowledgeDungeonShameWordOfPower, "shame"},
		{semantic.KnowledgeDungeonHythlothWordOfPower, "hythloth"},
		{semantic.KnowledgeDungeonDoomWordOfPower, "doom"},
	}
	for _, definition := range definitions {
		if definition.knowledge == knowledge {
			return definition.dungeon, true
		}
	}

	return "", false
}

// DeductGoldSimple removes gold from party inventory with yellow text feedback only.
// Used for regular conversation gold requests (GoldPrompt 0x85), NOT guard extortion.
// Returns error if insufficient gold.
//
// Beggar Karma Bonus (ref:KARMA-0011):
//   - If NPC is a beggar and the 100-update cooldown is ready: +1 karma
//   - If giving ALL gold to beggar: +2 additional karma (+3 total)
//
// Parameters:
//   - amount: Gold pieces to deduct
//   - npcName: Name of NPC receiving the gold (for logging)
func (g *GameActionCallbacks) DeductGoldSimple(amount int, npcName string) error {
	currentGold := int(g.gameScene.gameState.PartyState.Inventory.Gold.Get())
	if currentGold < amount {
		g.dialogOutput.AddTextWithColor("Thou hast not enough gold!", ucolor.EGAYellow)

		return fmt.Errorf("%w: have %d gold, need %d", conversation.ErrInsufficientPayment, currentGold, amount)
	}

	outcome, err := g.gameScene.gameState.CommitScriptedGoldPayment(amount, g.npcRef.IsBeggar())
	if err != nil {
		return fmt.Errorf("giving %d gold pieces to %s: %w", amount, npcName, err)
	}

	// Simple yellow text feedback in conversation output (no modal dialog)
	g.dialogOutput.AddTextWithColor(fmt.Sprintf("You give %d gold pieces to %s.", amount, npcName),
		ucolor.EGAYellow)

	remainingGold := int(outcome.GoldAfter)
	karmaBonus := outcome.KarmaAward

	if karmaBonus > 0 {
		g.gameScene.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundKarmaUp)

		if karmaBonus >= selflessDonationKarma {
			g.dialogOutput.AddTextWithColor("Selfless sacrifice! Karma greatly improved!", ucolor.EGAYellow)
		} else {
			g.dialogOutput.AddTextWithColor("Generous donation! Karma improved!", ucolor.EGAYellow)
		}

		logging.Infof(logging.CategoryConversation,
			"DeductGoldSimple: Beggar karma bonus applied - gave %d gp, remaining=%d, karma+%d",
			amount, remainingGold, karmaBonus)
	}

	logging.Infof(logging.CategoryConversation, "DeductGoldSimple: Gave %d gp to %s, remaining=%d",
		amount, npcName, g.gameScene.gameState.PartyState.Inventory.Gold.Get())

	return nil
}

// === Player Interaction Callbacks ===

// ShowOutput accumulates text in the output buffer for display.
// Text is captured during conversation processing and retrieved via GetAccumulatedOutput().
func (g *GameActionCallbacks) ShowOutput(text string) {
	g.output.WriteString(text)
}

// TimedPause is called when the conversation engine encounters a Pause command (0x83).
// The pause marker is written directly to the engine's output buffer for proper text
// interspersion. This callback is for logging/tracking only - the actual pause animation
// is handled by the UI layer.
func (g *GameActionCallbacks) TimedPause() {
	// Callback for logging/tracking pause events
	// Note: The engine writes the pause marker directly to its output buffer
	// so it's interspersed correctly with conversation text
	logging.Debugf(logging.CategoryConversation, "TimedPause: callback invoked (marker written by engine)")
}

// === Game State Query Callbacks ===

// HasMet returns true if the player has previously met this NPC.
// Used by conversation engine to determine greeting behavior and dialogue branches.
func (g *GameActionCallbacks) HasMet(npcID int) bool {
	return g.gameScene.gameState.PartyState.HasMetNPC(g.npcRef.Location, npcID)
}

// GetAvatarName returns the player's character name.
// Used in conversation text substitution when NPCs address the player by name.
func (g *GameActionCallbacks) GetAvatarName() string {
	return g.gameScene.gameState.PartyState.AvatarName()
}

// GetActivePartyNames returns only current party identities, in source slot order.
func (g *GameActionCallbacks) GetActivePartyNames() []string {
	names := make([]string, 0, party_state.MaxActiveParty)
	g.gameScene.gameState.PartyState.ForEachCharacter(func(index int, character *party_state.PlayerCharacter) {
		if index < party_state.MaxActiveParty && character.PartyMap == party_state.PartyMapActive {
			names = append(names, character.GetNameAsString())
		}
	})

	return names
}

// GetKarmaLevel returns the current karma value for the party.
// Used by conversation engine for karma-based dialogue branching.
func (g *GameActionCallbacks) GetKarmaLevel() int {
	// Return current karma value
	return g.gameScene.gameState.PartyState.Karma.Value
}

// SetMet marks this NPC as having been met by the player.
// Called after successful name exchange via AskName command in conversations.
// Affects future greeting behavior and dialogue branches.
func (g *GameActionCallbacks) SetMet(npcID int) {
	if npcID < 0 || npcID >= 32 {
		return
	}
	g.gameScene.gameState.PartyState.SetMet(g.npcRef.Location, npcID)
}

// Ensure GameActionCallbacks implements the ActionCallbacks interface
var _ conversation.ActionCallbacks = (*GameActionCallbacks)(nil)
var _ conversation.SemanticActionCallbacks = (*GameActionCallbacks)(nil)
