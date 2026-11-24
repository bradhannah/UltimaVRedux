package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

type surfacePrimaryRouteContext struct {
	name     string
	location references.Location
	floor    references.FloorNumber
	handle   func(*GameScene, ebiten.Key)
	minutes  int
}

var auditedSurfaceRouteContexts = []surfacePrimaryRouteContext{
	{
		name: "Britannia", location: references.BritanniaUnderworld,
		floor: references.FloorNumber(references.OVERWORLD), handle: (*GameScene).largeMapInputHandler,
		minutes: game_state.DefaultLargeMapMinutesPerTurn,
	},
	{
		name: "Underworld", location: references.BritanniaUnderworld,
		floor: references.FloorNumber(references.UNDERWORLD), handle: (*GameScene).largeMapInputHandler,
		minutes: game_state.DefaultLargeMapMinutesPerTurn,
	},
	{
		name: "town", location: references.Britain,
		handle: (*GameScene).smallMapInputHandler, minutes: game_state.DefaultSmallMapMinutesPerTurn,
	},
}

func TestSurfacePrimaryDirectionalCommandRoutingByContext(t *testing.T) {
	tests := []struct {
		contextIndexes []int
		name           string
		key            ebiten.Key
		prompt         string
		state          InputState
	}{
		{contextIndexes: []int{0, 1, 2}, name: "Get", key: ebiten.KeyG, prompt: "Get-", state: GetDirectionInput},
		{contextIndexes: []int{0, 1, 2}, name: "Klimb", key: ebiten.KeyK, prompt: "Klimb-", state: KlimbDirectionInput},
		{contextIndexes: []int{0, 1}, name: "Talk", key: ebiten.KeyT, prompt: "Talk-", state: TalkDirectionInput},
		{contextIndexes: []int{2}, name: "Push", key: ebiten.KeyP, prompt: "Push-", state: PushDirectionInput},
	}

	for _, test := range tests {
		for _, contextIndex := range test.contextIndexes {
			context := auditedSurfaceRouteContexts[contextIndex]
			t.Run(context.name+"/"+test.name, func(t *testing.T) {
				scene, callbacks := newAuditedSurfaceRouteScene(t, context)

				context.handle(scene, test.key)

				assert.Equal(t, test.state, scene.secondaryKeyState)
				assert.Equal(t, []string{test.prompt}, callbacks.CommandPrompts)
				assert.Empty(t, callbacks.TimeAdvanced())
			})
		}
	}
}

func TestSurfacePrimaryImmediateCommandCompletionByContext(t *testing.T) {
	tests := []struct {
		contextIndexes []int
		name           string
		key            ebiten.Key
		prompt         string
		message        string
		currentRow     string
	}{
		{contextIndexes: []int{0, 1}, name: "Board", key: ebiten.KeyB, prompt: "Board ", currentRow: "What?"},
		{contextIndexes: []int{2}, name: "Enter", key: ebiten.KeyE, prompt: "Enter what?"},
		{
			contextIndexes: []int{2}, name: "Fire without cannon", key: ebiten.KeyF,
			prompt: "Fire-", currentRow: "What?",
		},
		{
			contextIndexes: []int{2}, name: "Hole up without bed", key: ebiten.KeyH,
			prompt: "Hole up & camp...", message: "Find a bed first!",
		},
		{
			contextIndexes: []int{0, 1, 2}, name: "Ignite without torch", key: ebiten.KeyI,
			prompt: "Ignite torch!", message: "None owned!",
		},
		{contextIndexes: []int{0, 1, 2}, name: "X-it on foot", key: ebiten.KeyX, prompt: "X-it ", currentRow: "what?"},
	}

	for _, test := range tests {
		for _, contextIndex := range test.contextIndexes {
			context := auditedSurfaceRouteContexts[contextIndex]
			t.Run(context.name+"/"+test.name, func(t *testing.T) {
				scene, callbacks := newAuditedSurfaceRouteScene(t, context)

				context.handle(scene, test.key)

				assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
				assert.Equal(t, []string{test.prompt}, callbacks.CommandPrompts)
				if test.message != "" {
					assert.Equal(t, []string{test.message}, callbacks.Messages)
				}
				if test.currentRow != "" {
					assert.Equal(t, test.currentRow, callbacks.CurrentRowContent)
				}
				assert.Equal(t, []int{context.minutes}, callbacks.TimeAdvanced())
			})
		}
	}
}

func TestSurfacePrimaryRetainedWorkflowRoutingByContext(t *testing.T) {
	tests := []struct {
		contextIndexes []int
		name           string
		key            ebiten.Key
		prompt         string
		workflow       string
		prepare        func(*game_state.GameState)
	}{
		{
			contextIndexes: []int{0, 1}, name: "Hole up", key: ebiten.KeyH,
			prompt: "Hole up & camp...", workflow: "surface_command_completion",
		},
		{
			contextIndexes: []int{0, 1, 2}, name: "Mix", key: ebiten.KeyM,
			prompt: "Mix Reagents", workflow: "reagent_mixing_workflow",
			prepare: func(state *game_state.GameState) {
				state.PartyState.Inventory.Reagent.Set(references.SulfurAsh, 1)
			},
		},
		{
			contextIndexes: []int{0, 1, 2}, name: "New Order", key: ebiten.KeyN,
			prompt: "New Order...", workflow: "new_order_workflow",
			prepare: func(state *game_state.GameState) {
				for index := range party_state.MaxActiveParty {
					state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
				}
				for index := range 3 {
					state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapActive
				}
			},
		},
	}

	for _, test := range tests {
		for _, contextIndex := range test.contextIndexes {
			context := auditedSurfaceRouteContexts[contextIndex]
			t.Run(context.name+"/"+test.name, func(t *testing.T) {
				scene, callbacks := newAuditedSurfaceRouteScene(t, context)
				if test.prepare != nil {
					test.prepare(scene.gameState)
					callbacks.Reset()
				}

				context.handle(scene, test.key)

				assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
				assert.Equal(t, []string{test.prompt}, callbacks.CommandPrompts)
				assert.True(t, scene.gameState.EventQueueManager.HasWorkflow(test.workflow))
				assert.Empty(t, callbacks.TimeAdvanced())
			})
		}
	}
}

func TestTownHoleUpQueuesBedRestAndOrdinaryCompletion(t *testing.T) {
	context := auditedSurfaceRouteContexts[2]
	scene, callbacks := newAuditedSurfaceRouteScene(t, context)
	scene.gameState.SystemCallbacks.Flow.Rest.RequestSleepDuration = scene.RequestSleepDuration
	position := scene.gameState.MapState.PlayerLocation.Position
	scene.gameState.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapLayer, &position, indexes.LeftBed)
	callbacks.Reset()

	context.handle(scene, ebiten.KeyH)

	assert.True(t, scene.gameState.EventQueueManager.HasWorkflow("bed_rest_workflow"))
	assert.True(t, scene.gameState.EventQueueManager.HasWorkflow("surface_command_completion"))
	assert.Empty(t, callbacks.TimeAdvanced(), "ordinary completion waits for bed-rest finalization")
}

func TestCombatPrimaryDirectionalCommandRouting(t *testing.T) {
	tests := []struct {
		name   string
		key    ebiten.Key
		prompt string
		state  InputState
	}{
		{name: "Get", key: ebiten.KeyG, prompt: "Get-", state: GetDirectionInput},
		{name: "Jimmy", key: ebiten.KeyJ, prompt: "Jimmy-", state: JimmyDoorDirectionInput},
		{name: "Open", key: ebiten.KeyO, prompt: "Open-", state: OpenDirectionInput},
		{name: "Search", key: ebiten.KeyS, prompt: "Search-", state: SearchDirectionInput},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, callbacks := newCombatUseTestScene(t)
			scene.gameState.BeginCombatTurns(game_state.CombatTurnsPresented)
			callbacks.Reset()
			before := scene.gameState.CombatScene.GetCurrentTurnIdentity()

			scene.combatMapInputHandler(test.key)

			assert.Equal(t, test.state, scene.secondaryKeyState)
			assert.Contains(t, scene.output.GetOutputStr(true), test.prompt)
			assert.Equal(t, before, scene.gameState.CombatScene.GetCurrentTurnIdentity())
			assert.Empty(t, callbacks.TimeAdvanced())
		})
	}
}

func TestCombatPrimaryPassCompletesAction(t *testing.T) {
	scene, callbacks := newCombatUseTestScene(t)
	expected, _ := newCombatUseTestScene(t)
	scene.gameState.BeginCombatTurns(game_state.CombatTurnsPresented)
	expected.gameState.BeginCombatTurns(game_state.CombatTurnsPresented)
	callbacks.Reset()

	expected.gameState.FinishCombatAction()
	scene.combatMapInputHandler(ebiten.KeySpace)

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Contains(t, scene.output.GetOutputStr(true), "Pass")
	assert.Equal(
		t,
		expected.gameState.CombatScene.GetCurrentTurnIdentity(),
		scene.gameState.CombatScene.GetCurrentTurnIdentity(),
	)
	assert.Empty(t, callbacks.TimeAdvanced())
}

func newAuditedSurfaceRouteScene(
	t *testing.T,
	context surfacePrimaryRouteContext,
) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	builder := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(context.location).
		WithPlayerAt(15, 15).
		WithRandomSeed(467).
		WithSystemCallbacks()
	if context.location == references.BritanniaUnderworld {
		builder.WithFloor(context.floor).WithPlayerAt(50, 50)
	}
	state, callbacks := builder.Build()
	state.DebugOptions.MonsterGen = false
	state.CurrentNPCAIController.GetNpcs().ClearAllMapUnits()
	state.PartyState.Inventory.Provisions.Torches.Set(0)
	position := state.MapState.PlayerLocation.Position
	state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.Grass)
	for _, direction := range []references.Direction{references.Up, references.Right, references.Down, references.Left} {
		adjacent := direction.GetNewPositionInDirection(&position)
		state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapOverrideLayer, adjacent, indexes.Grass)
	}
	callbacks.Reset()

	return newSurfaceCompletionTestScene(state), callbacks
}
