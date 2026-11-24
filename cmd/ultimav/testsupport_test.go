package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/character_creation"
	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

// IsVisible returns whether the dialog is currently visible.
func (d *ConfigurationDialog) IsVisible() bool {
	return d.isVisible
}

// HasFocus returns whether the dialog currently has input focus.
func (d *ConfigurationDialog) HasFocus() bool {
	return d.hasFocus
}

// GetActiveTab returns the currently active tab index.
func (d *ConfigurationDialog) GetActiveTab() int {
	return d.currentTab
}

// GetMusicVolumeSlider returns the music volume slider for testing.
func (d *ConfigurationDialog) GetMusicVolumeSlider() *widgets.Slider {
	return d.musicSlider
}

// GetSFXVolumeSlider returns the SFX volume slider for testing.
func (d *ConfigurationDialog) GetSFXVolumeSlider() *widgets.Slider {
	return d.sfxSlider
}

// GetAmbientVolumeSlider returns the ambient volume slider for testing.
func (d *ConfigurationDialog) GetAmbientVolumeSlider() *widgets.Slider {
	return d.ambientSlider
}

// Result returns the detached completed result even after scene disposal.
func (scene *CharacterCreationScene) Result() (character_creation.Result, bool) {
	return scene.result, scene.hasResult
}

// StartCarpetBoardingWorkflow initiates the magic carpet boarding workflow with event queue.
// Handles using the magic carpet from inventory to board it at the player's current location.
// Carpet boarding admission and ownership follow ref:VEH-0004.
//
// Flow:
//  1. Remove carpet from inventory (QuestItems)
//  2. Play stub sound
//  3. Spawn carpet at player position and board it
//  4. Show "Board carpet!" confirmation message
//  5. Advance game time
func (g *GameScene) StartCarpetBoardingWorkflow(commit *game_state.CarpetBoardingCommit) {
	g.startCarpetBoardingWorkflow(commit, nil)
}

// StartGemViewWorkflow initiates the gem overhead view display.
func (g *GameScene) StartGemViewWorkflow() {
	g.startGemViewWorkflowWithCompletion(nil)
}

// StartMoonstoneBurialWorkflow initiates the moonstone burial workflow with event queue.
// Handles burying a moonstone at the player's current location to create a permanent moongate.
// Moonstone burial admission and persistence follow ref:MOON-0004.
//
// Flow:
//  1. Remove moonstone from inventory
//  2. Update moonstone system with burial location
//  3. Show "Moonstone buried!" confirmation message
//  4. Advance game time
func (g *GameScene) StartMoonstoneBurialWorkflow(moonstone references.Moonstone) {
	g.startMoonstoneBurialWorkflow(moonstone, nil)
}

// StartSceptreWorkflow initiates surface/town Sceptre use.
// Sceptre context and field-removal rules are documented in ref:QUEST-0003.
func (g *GameScene) StartSceptreWorkflow() {
	g.startSceptreWorkflow(nil)
}

// StartSkullKeyWorkflow initiates the skull key unlock workflow with event queue.
// Skull Keys unlock magic doors; consumption follows ref:PROV-0006.
// REDUX DEVIATION: PROV-0006: original Use pays before unlocking. Redux prompts
// first, then pays for a confirmed direction even when no door is unlocked.
func (g *GameScene) StartSkullKeyWorkflow(commit *game_state.SkullKeyUseCommit) {
	g.startSkullKeyWorkflow(commit, nil)
}

// StartTalkWorkflow initiates a unified talk workflow using an event queue.
// This replaces the old ActionTalkSmallMapExtended with a single event queue
// that handles all talk types:
//   - NoNPC: "No-one to talk to!"
//   - ShadowlordHostile: Damage party with red flash, NPC flees
//   - ShadowlordFlee: "Don't hurt me!" message
//   - Guard: Arrest or extortion dialog
//   - Normal: LinearTalkDialog for full conversation
//   - NonFriendly: "Can't talk to that!"
//
// This fixes the "red screen stays forever" bug by using PartyStateBuilder
// for Shadowlord damage effects, which guarantees cleanup.
func (g *GameScene) StartTalkWorkflow(direction references.Direction) {
	g.startTalkWorkflow(direction, nil, true, false)
}

// StartUseWorkflow initiates the Use command workflow, showing the item selection dialog.
// Shows "No usable items!" if the inventory is empty.
//
// Flow:
//  1. Query usable items from GameState
//  2. Show message if no items available
//  3. Display ItemSelectionDialog for selection
//  4. Execute item use on selection
//  5. Close dialog on cancel (ESC)
func (g *GameScene) StartUseWorkflow() {
	g.startUseWorkflow(game_state.UsageField, false)
}

// StartPotionWorkflow initiates the potion usage workflow with character selection.
// Called from GameState.usePotion via SystemCallbacks.Flow.Items.StartPotionWorkflow.
//
// Flow: Select a character → Consume potion → Roll random effect → Apply it.
//
// Potion effects and substitution are documented in ref:CONSUME-0002.
func (g *GameScene) StartPotionWorkflow(commit *game_state.PotionUseCommit) {
	g.startPotionWorkflow(commit, nil)
}

func (d *LinearTalkDialog) Refresh() {
	d.initializeResizeableVisualElements()
}

func (mvs *MoongateVisualSystem) GetActiveMoongateAt(pos references.Position) *MoongateRenderer {
	if mvs.gameState.MoongateStage() == game_state.MoongateStageHidden {
		return nil
	}
	for _, renderer := range mvs.activeRenderers {
		if renderer.GetPosition() == pos && mvs.isCurrentRenderer(renderer) {
			return renderer
		}
	}

	return nil
}

// SetVisible controls whether the wind display is shown
func (w *WindDisplay) SetVisible(visible bool) {
	w.visible = visible
}

// IsVisible returns whether the wind display is currently visible
func (w *WindDisplay) IsVisible() bool {
	return w.visible
}

// Draw renders the wind display on screen at bottom center of map area
func (w *WindDisplay) Draw(screen *ebiten.Image, windDirection environment.WindDirection) {
	w.DrawLabel(screen, windDirection.GetUIString())
}

// countEqual returns how many values equal want.
func countEqual[T comparable](values []T, want T) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}

	return count
}

// newBritainCombatTestState builds a seeded Britain state and enters the
// Britannia combat territory from the north.
func newBritainCombatTestState(t *testing.T, seed uint64) (*game_state.GameState, *game_state.MockSystemCallbacks) {
	t.Helper()
	gs, mockCallbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(seed).
		Build()
	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, game_state.CombatEntryNorth))

	return gs, mockCallbacks
}
