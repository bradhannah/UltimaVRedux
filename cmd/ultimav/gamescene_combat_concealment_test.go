package main

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestCombatConcealedMonsterAndSelectionFrameDoNotRender(t *testing.T) {
	for _, terrain := range []indexes.SpriteIndex{indexes.BlackSquare, indexes.BrickWallArchway} {
		t.Run(fmt.Sprintf("terrain-%d", terrain), func(t *testing.T) {
			scene, screen := newCombatConcealmentTestScene(t)
			state := scene.gameState
			position := references.Position{X: 5, Y: 5}
			state.CombatScene.MonsterManager.Clear()
			state.CombatScene.TurnQueue.RemoveAllMonsters()
			enemyRef := &(*state.GameReferences.EnemyReferences)[references.MonsterOrc]
			monster := game_state.NewCombatMonster(references.MonsterOrc, enemyRef, position, state.AsRandomProvider())
			monsterIndex := state.CombatScene.AddMonsterToQueue(monster)
			setCombatConcealmentTerrain(state, position, terrain)

			scene.drawCombatMonsterOverlay(screen)
			assert.Zero(t, opaquePixelCount(screen))
			scene.drawMonsterTargetMonsterSelectionFrame(screen, monsterIndex)
			assert.Zero(t, opaquePixelCount(screen))
		})
	}

	scene, screen := newCombatConcealmentTestScene(t)
	state := scene.gameState
	position := references.Position{X: 5, Y: 5}
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	enemyRef := &(*state.GameReferences.EnemyReferences)[references.MonsterOrc]
	monster := game_state.NewCombatMonster(references.MonsterOrc, enemyRef, position, state.AsRandomProvider())
	monsterIndex := state.CombatScene.AddMonsterToQueue(monster)
	setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
	scene.drawCombatMonsterOverlay(screen)
	assert.Positive(t, opaquePixelCount(screen))
	screen.Clear()
	scene.drawMonsterTargetMonsterSelectionFrame(screen, monsterIndex)
	assert.Positive(t, opaquePixelCount(screen))
}

func TestCombatConcealedPartyMemberAndFieldDoNotRender(t *testing.T) {
	for _, test := range []struct {
		name string
		draw func(*GameScene, *ebiten.Image, references.Position)
	}{
		{
			name: "party member",
			draw: func(scene *GameScene, screen *ebiten.Image, position references.Position) {
				state := scene.gameState
				active := state.CombatScene.GetActiveCombatantIndex()
				for index := range state.CombatScene.PartyActive {
					state.CombatScene.PartyActive[index] = index == active
				}
				state.CombatScene.SetPartyMemberPosition(active, position)
				state.PartyState.GetCharacter(active).SetStatus(party_state.Good)
				scene.drawCombatPartyOverlay(screen)
			},
		},
		{
			name: "field",
			draw: func(scene *GameScene, screen *ebiten.Image, position references.Position) {
				require.True(t, scene.gameState.CombatScene.AddField(
					game_state.NewCombatField(game_state.CombatFieldPoison, position),
				))
				scene.drawCombatFieldOverlay(screen)
			},
		},
	} {
		for _, terrain := range []indexes.SpriteIndex{indexes.BlackSquare, indexes.BrickWallArchway} {
			t.Run(fmt.Sprintf("%s/terrain-%d", test.name, terrain), func(t *testing.T) {
				scene, screen := newCombatConcealmentTestScene(t)
				position := references.Position{X: 5, Y: 5}
				setCombatConcealmentTerrain(scene.gameState, position, terrain)

				test.draw(scene, screen, position)
				assert.Zero(t, opaquePixelCount(screen))
			})
		}
	}
}

func TestCombatConcealedMonsterDoesNotAppearInHUD(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	state := scene.gameState
	position := references.Position{X: 5, Y: 5}
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	enemyRef := &(*state.GameReferences.EnemyReferences)[references.MonsterOrc]
	monster := game_state.NewCombatMonster(references.MonsterOrc, enemyRef, position, state.AsRandomProvider())
	monsterIndex := state.CombatScene.AddMonsterToQueue(monster)
	identity := game_state.CombatTurnIdentity{EntityIndex: monsterIndex}
	snapshot := game_state.CombatTurnForecastEntry{CombatTurnIdentity: identity, Dexterity: 20, Countdown: 10}
	scene.combatHUD.detail = true
	for _, terrain := range []indexes.SpriteIndex{indexes.BlackSquare, indexes.BrickWallArchway} {
		setCombatConcealmentTerrain(state, position, terrain)
		assert.Nil(t, scene.combatTurnSprite(identity))
		assert.Empty(t, scene.combatCurrentTurnDetail(identity, snapshot, true))
	}

	setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
	assert.NotNil(t, scene.combatTurnSprite(identity))
	assert.NotEmpty(t, scene.combatCurrentTurnDetail(identity, snapshot, true))
}

func TestCombatForecastCanOmitConcealedMonsterSlotsWithoutChangingQueue(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	state := scene.gameState
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	enemyRef := &(*state.GameReferences.EnemyReferences)[references.MonsterOrc]
	hiddenPosition := references.Position{X: 5, Y: 5}
	visiblePosition := references.Position{X: 6, Y: 5}
	hiddenIndex := state.CombatScene.AddMonsterToQueue(game_state.NewCombatMonster(
		references.MonsterOrc, enemyRef, hiddenPosition, state.AsRandomProvider(),
	))
	visibleIndex := state.CombatScene.AddMonsterToQueue(game_state.NewCombatMonster(
		references.MonsterOrc, enemyRef, visiblePosition, state.AsRandomProvider(),
	))
	setCombatConcealmentTerrain(state, hiddenPosition, indexes.BlackSquare)
	setCombatConcealmentTerrain(state, visiblePosition, indexes.BrickFloor)
	hidden := game_state.CombatTurnForecastEntry{
		CombatTurnIdentity: game_state.CombatTurnIdentity{EntityIndex: hiddenIndex},
	}
	party := game_state.CombatTurnForecastEntry{
		CombatTurnIdentity: game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0},
	}
	visible := game_state.CombatTurnForecastEntry{
		CombatTurnIdentity: game_state.CombatTurnIdentity{EntityIndex: visibleIndex},
	}
	scene.combatHUD.forecast = [combatForecastSize]game_state.CombatTurnForecastEntry{
		hidden, party, visible, hidden,
	}
	pcsBefore, monstersBefore := state.CombatScene.TurnQueue.CountRemaining()

	scene.gameConfig.GameplayConfiguration.ShowConcealedTurnOrderSlots = true
	assert.Equal(t, combatForecastSize, scene.filterConcealedCombatForecast(combatForecastSize))
	assert.Equal(t, hidden, scene.combatHUD.forecast[0])

	scene.gameConfig.GameplayConfiguration.ShowConcealedTurnOrderSlots = false
	assert.Equal(t, 2, scene.filterConcealedCombatForecast(combatForecastSize))
	assert.Equal(t, party, scene.combatHUD.forecast[0])
	assert.Equal(t, visible, scene.combatHUD.forecast[1])
	pcsAfter, monstersAfter := state.CombatScene.TurnQueue.CountRemaining()
	assert.Equal(t, pcsBefore, pcsAfter)
	assert.Equal(t, monstersBefore, monstersAfter)
}

// The party roster names every member, so the Turn Order panel hides none of them.
func TestCombatTurnPanelAlwaysNamesTheParty(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	state := scene.gameState
	underArch := game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	captive := game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: 1}
	setCombatConcealmentTerrain(state, state.CombatScene.GetCombatantPosition(underArch), indexes.BrickWallArchway)
	state.CombatScene.PartyUnderground[captive.EntityIndex] = true

	for _, identity := range []game_state.CombatTurnIdentity{underArch, captive} {
		_, onBattlefield := state.ProjectCombatActor(identity, 0)
		assert.False(t, onBattlefield, "the battlefield still conceals the body")
		actor, named := state.ProjectCombatTurnActor(identity)
		require.True(t, named)
		assert.Equal(t, state.PartyState.GetCharacter(identity.EntityIndex).GetNameAsString(), actor.Name)
		assert.NotNil(t, scene.combatTurnSprite(identity))
	}

	scene.gameConfig.GameplayConfiguration.ShowConcealedTurnOrderSlots = false
	scene.combatHUD.forecast[0] = game_state.CombatTurnForecastEntry{CombatTurnIdentity: underArch}
	scene.combatHUD.forecast[1] = game_state.CombatTurnForecastEntry{CombatTurnIdentity: captive}
	assert.Equal(t, 2, scene.filterConcealedCombatForecast(2), "Off drops hidden monsters, never the party")
}

func newCombatConcealmentTestScene(t *testing.T) (*GameScene, *ebiten.Image) {
	t.Helper()
	scene := newCombatConcealmentTestState(t)
	screen := newPresentationRaster(1280, 720)
	t.Cleanup(screen.Dispose)
	return scene, screen
}

func newCombatConcealmentTestState(t *testing.T) *GameScene {
	t.Helper()
	retirePresentationFixtureCaches(t)
	scene, _ := newCombatUseTestScene(t)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	scene.mapImage = ebiten.NewImage(304, 208)
	scene.combatHUD = newCombatHUD()
	scene.gameConfig = &config.UltimaVConfiguration{
		GameplayConfiguration: config.DefaultGameplayConfiguration(),
	}
	t.Cleanup(scene.mapImage.Dispose)

	return scene
}

func setCombatConcealmentTerrain(
	state *game_state.GameState,
	position references.Position,
	tile indexes.SpriteIndex,
) {
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, tile)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.NoSprites)
}

func opaquePixelCount(image *ebiten.Image) int {
	pixels := make([]byte, image.Bounds().Dx()*image.Bounds().Dy()*4)
	image.ReadPixels(pixels)
	count := 0
	for index := 3; index < len(pixels); index += 4 {
		if pixels[index] != 0 {
			count++
		}
	}

	return count
}
