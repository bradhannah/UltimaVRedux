package main

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCombatAllegianceTransitionsBaselineDisclosureAndReuse(t *testing.T) {
	scene, mock := newCombatUseTestScene(t)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	state := scene.gameState
	index, monster := signatureMonster(t, scene, references.MonsterStocks, references.Position{X: 5, Y: 5})
	scene.updateCombatAllegiance()
	mock.Messages = nil
	rng, elapsed := state.GetRNG().Calls(), state.ElapsedMs
	monster.SetLoyalty(game_state.CombatMonsterLoyaltyParty)
	scene.updateCombatAllegiance()
	scene.updateCombatAllegiance()
	assert.Equal(t, []string{"Pirate now fights for your side."}, mock.Messages)
	key := combatAllegianceKey{identity: game_state.CombatTurnIdentity{EntityIndex: index}, id: monster.CombatantID()}
	assert.Equal(t, game_state.CombatMonsterLoyaltyParty, scene.combatAllegiance.actors[key].loyalty)
	monster.SetInvisible(true)
	scene.updateCombatAllegiance()
	monster.SetLoyalty(game_state.CombatMonsterLoyaltyMonster)
	scene.updateCombatAllegiance()
	monster.SetInvisible(false)
	scene.updateCombatAllegiance()
	assert.Len(t, mock.Messages, 1, "newly disclosed side is a silent baseline")
	assert.Equal(t, game_state.CombatMonsterLoyaltyMonster, scene.combatAllegiance.actors[key].loyalty)
	assert.Equal(t, rng, state.GetRNG().Calls())
	assert.Equal(t, elapsed, state.ElapsedMs)
	replacement := game_state.NewCombatMonster(references.MonsterStocks, monster.EnemyRef(), monster.Position(), state.AsRandomProvider())
	replacement.SetLoyalty(game_state.CombatMonsterLoyaltyParty)
	state.CombatScene.ReplaceCombatantWithMonster(key.identity, replacement)
	scene.updateCombatAllegiance()
	assert.NotContains(t, scene.combatAllegiance.actors, key)
	assert.Len(t, mock.Messages, 1)
	state.CombatScene = nil
	scene.updateCombatAllegiance()
	assert.Empty(t, scene.combatAllegiance.actors)
}

func TestCombatAllegianceHealthFrameCompositionAndHiddenActors(t *testing.T) {
	scene, _ := newCombatConcealmentTestScene(t)
	state := scene.gameState
	actor := state.CombatScene.GetCurrentTurnIdentity()
	position := state.CombatScene.GetCombatantPosition(actor)
	setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
	state.CombatScene.PartyPossessed[actor.EntityIndex] = true
	state.PartyState.GetCharacter(actor.EntityIndex).SetStatus(party_state.Poisoned)
	frame := ebiten.NewImage(18, 18)
	t.Cleanup(frame.Dispose)
	options := &ebiten.DrawImageOptions{}
	drawCombatPossessedMarker(frame, options)
	drawCombatAttackBrackets(frame, options)
	drawCombatHealthMark(frame, options, scene.combatDisclosedHealthBorder(actor), 50, 100)
	assert.Positive(t, countExactColor(frame, ucolor.CombatPossessed))
	assert.Positive(t, countExactColor(frame, ucolor.CombatAllegianceEnemy))
	assert.Positive(t, countExactColor(frame, ucolor.CombatAttackEligibleFace))
	assertSignatureHP(t, frame, 50, 100)
	for _, hidden := range []bool{false, true} {
		state.CombatScene.PartyUnderground[actor.EntityIndex] = hidden
		assert.Equal(t, !hidden, scene.combatDisclosedHealthBorder(actor) != ucolor.EGABlack)
	}
}

func TestCombatAttackSideReadoutAndActualActorLayout(t *testing.T) {
	for _, resolution := range []image.Point{{1280, 720}, {1366, 768}, {1920, 1080}, {2560, 1440}, {3840, 2160}} {
		t.Run(fmt.Sprint(resolution), func(t *testing.T) {
			bounds := image.Rectangle{Max: resolution}
			turn, _ := combatLayout{}.turnAndReportRectangles(bounds)
			content := combatHUDContentBounds(bounds, turn)
			size := float64(combatHUDFontSize) * float64(resolution.X) / 1920
			font := text.NewUltimaFont(size)
			output := text.NewOutput(font, size, combatHUDOutputLines, combatHUDOutputChars)
			// Exercise real current-actor text, including full clone name, known
			// cause and detail; the fixed pane and font must leave space for NEXT.
			scene, actor := setupCombatAutoAttackInputTest(t)
			scene.spriteSheet = sharedPresentationTestSpriteSheet()
			scene.mapImage = ebiten.NewImage(304, 208)
			t.Cleanup(scene.mapImage.Dispose)
			screen := newPresentationRaster(resolution.X, resolution.Y)
			t.Cleanup(screen.Dispose)
			character := scene.gameState.PartyState.GetCharacter(actor.EntityIndex)
			clear(character.Name[:])
			copy(character.Name[:], "Blackthorn")
			scene.gameState.CombatScene.PartyPossessed[actor.EntityIndex] = true
			scene.gameState.CombatScene.PartySleeping[actor.EntityIndex] = true
			require.True(t, scene.gameState.CastCombatInQuasXen(actor.EntityIndex, actor))
			scene.gameState.CombatScene.PartyPossessed[actor.EntityIndex] = false
			scene.gameState.CombatScene.PartySleeping[actor.EntityIndex] = false
			scene.gameState.CombatScene.ForEachPCClone(func(identity game_state.CombatTurnIdentity, _ references.Position) { actor = identity })
			scene.gameState.CombatScene.CurrentTurnIdentity = actor
			scene.combatHUD = newCombatHUD()
			scene.combatHUD.detail = true
			scene.combatHUD.turnOutput = output
			line := int(math.Ceil(output.GetLineSpacing()))
			bottom := scene.drawCombatCurrentTurn(screen, content, line, line/3)
			spriteHeight := int(math.Round(18 * float64(resolution.Y) / 208))
			readoutHeight := int(math.Ceil(output.GetTotalHeightForLines(3)))
			requiredBottom := bottom + line/3 + readoutHeight + 2*line + spriteHeight + int(math.Ceil(output.GetTotalHeightForLines(1)))
			assert.LessOrEqual(t, requiredBottom, content.Max.Y)
		})
	}
}

func TestCombatGateHitchesRetainEachPhaseAndOneAudioOnset(t *testing.T) {
	scene, mock := newCombatUseTestScene(t)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	state := scene.gameState
	step := nextNonActionablePCPresentationStep(t, state)
	index, monster := signatureMonster(t, scene, references.MonsterDevil, references.Position{X: 5, Y: 5})
	scene.recordCombatEvent(signatureGateEvent(index, monster))
	effect := scene.combatSignatureEffects[0]
	scene.startScheduledCombatAutoPass(step)
	state.EventQueueManager.Update()
	for _, test := range []struct {
		delta int64
		phase combatGatePhase
	}{
		{5000, combatGateMaterialize}, {599, combatGateMaterialize}, {5000, combatGateSettle}, {124, combatGateSettle},
	} {
		state.ElapsedMs += test.delta
		state.EventQueueManager.Update() // Production order: workflow before effect update.
		require.True(t, scene.hasCombatSignatureEffects())
		require.True(t, state.EventQueueManager.HasWorkflow("combat_pc_auto_pass"))
		scene.updateCombatSignatureEffects()
		assert.Equal(t, test.phase, effect.GatePhase)
		scene.updateCombatSignatureEffects()
	}
	gateSounds := 0
	for _, sound := range mock.SoundEffectsPlayed {
		if sound == game_state.SoundMonsterGate {
			gateSounds++
		}
	}
	assert.Equal(t, 1, gateSounds)
	state.ElapsedMs++
	scene.updateCombatSignatureEffects()
	assert.Empty(t, scene.combatSignatureEffects)
	assert.Nil(t, effect.Frame)
	assert.Nil(t, effect.Highlight)
	assert.False(t, effect.Dissolve.IsActive())
	state.EventQueueManager.Shutdown()
}

func TestCombatGateProductionCompositionShowsFragmentsWithoutOpaqueUnderlay(t *testing.T) {
	scene, screen := newCombatConcealmentTestScene(t)
	state := scene.gameState
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	index, monster := signatureMonster(t, scene, references.MonsterDevil, references.Position{X: 5, Y: 5})
	monster.SetCurrentHP(monster.MaxHP() - 1) // The battlefield marks only the injured.
	scene.recordCombatEvent(signatureGateEvent(index, monster))
	effect := scene.combatSignatureEffects[0]
	counts := make([]int, 0, 3)
	for _, age := range []int64{0, 125, 185, 425, 725} {
		advanceSignatureForTest(scene, effect.StartedMs+age)
		screen.Clear()
		scene.drawCombatMonsterOverlay(screen)
		assert.Zero(t, opaquePixelCount(screen), "normal actor is suppressed at age %d", age)
		scene.drawCombatSignatureEffects(screen)
		assert.Positive(t, opaquePixelCount(screen), "arrival must be visible with sound muted")
		if age == 0 {
			assert.Zero(t, opaquePixelCount(effect.Frame), "prelude shows disturbance only")
			continue
		}
		assertSignatureHP(t, effect.Frame, monster.CurrentHP(), monster.MaxHP())
		assert.Positive(t, countExactColor(effect.Frame, ucolor.CombatAllegianceEnemy))
		if age == 185 || age == 425 {
			assert.Positive(t, opaquePixelCount(effect.Highlight), "bright leading fragments at age %d", age)
		}
		if age >= 185 {
			// Exclude the semantic health frame from silhouette count.
			counts = append(counts, opaquePixelCount(effect.Frame.SubImage(image.Rect(2, 2, 16, 12)).(*ebiten.Image)))
		}
	}
	require.Len(t, counts, 3)
	assert.Less(t, counts[0], counts[1], "sparse fragments -> partial silhouette")
	assert.Less(t, counts[1], counts[2], "partial silhouette -> complete daemon")
	assert.Zero(t, opaquePixelCount(effect.Highlight), "settling has normal colours")
	scene.clearCombatSignatureEffects()
}

func TestCombatAttackPromptSequenceAndOriginSpaceSkip(t *testing.T) {
	scene, actor := setupCombatAutoAttackInputTest(t)
	state := scene.gameState
	character := state.PartyState.GetCharacter(actor.EntityIndex)
	character.SetEquippedItem(references.SlotHelm, references.SpikedHelm)
	character.SetEquippedItem(references.SlotWeapon, references.MorningStar)
	character.SetEquippedItem(references.SlotShield, references.NoEquipment)
	scene.combatMapInputHandler(ebiten.KeyA)
	first := scene.output.GetOutputStr(false)
	assert.Contains(t, first, "Attack 1/2: Spiked Helm")
	assert.Contains(t, first, "Skip at origin")
	assert.Contains(t, first, "Distance")
	assert.Contains(t, first, "Range")
	assert.Contains(t, first, "Cancel")
	rng := state.GetRNG().Calls()
	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	scene.handleCombatAttackTargetInput()
	require.NotNil(t, state.CombatScene.AttackTargeting)
	assert.Equal(t, 1, state.CombatScene.AttackTargeting.SourceIndex)
	assert.Equal(t, references.MorningStar, scene.combatAttackPreview().Sources[0].Equipment)
	assert.Equal(t, 2, scene.combatAttackPreview().Sources[0].MaxRange)
	assert.Equal(t, rng, state.GetRNG().Calls())
	second := scene.output.GetOutputStr(false)
	assert.Contains(t, second, "Attack 2/2: Morning Star")
	assert.Contains(t, second, "Skip item")
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	scene.handleCombatAttackTargetInput()
	assert.Nil(t, state.CombatScene.AttackTargeting)
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
}

// Optional actual-size production-render captures; never writes licensed pixels
// into the repository and never substitutes for live/joint visual acceptance.
func captureCombatPolishFrame(t *testing.T, scene *GameScene, name string, age int64) {
	t.Helper()
	directory := os.Getenv("ULTIMA5_REVIEW_IMAGES")
	if directory == "" {
		return
	}
	manager := display.GetManager()
	savedManager := *manager
	manager.PublishLogicalSize(1280, 720)
	defer func() { *manager = savedManager }()
	if scene.mapImage == nil {
		scene.mapImage = ebiten.NewImage(304, 208)
		t.Cleanup(scene.mapImage.Dispose)
	}
	screen := newPresentationRaster(1280, 720)
	defer screen.Dispose()
	screen.Fill(ucolor.EGABlack)
	for y := range references.YCombatMapTiles {
		for x := range references.XCombatMapTiles {
			position := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			tile := indexes.SpriteIndex(indexes.BrickFloor)
			if strings.Contains(name, "grass") {
				tile = indexes.Grass
			}
			if strings.Contains(name, "water") {
				tile = indexes.Water1
			}
			screen.DrawImage(scene.spriteSheet.GetSprite(tile), scene.combatEntityDrawOptions(position, false, -1, 0))
		}
	}
	scene.drawAvatarOverlay(screen)
	font := text.NewUltimaFont(16)
	output := text.NewOutput(font, 16, 10, 40)
	output.AppendLine().White(name).Done()
	output.AppendLine().Whitef("Presentation age %d ms", age).Done()
	output.AppendLine().LightCyan("Cyan HP frame: Ally").Done()
	output.AppendLine().LightRed("Red HP frame: Enemy").Done()
	output.AppendLine().White("1280x720 composition fixture").Done()
	if strings.Contains(name, "health-frames") {
		output.AppendLine().White("Pairs: full HP, quarter HP, 1 HP").Done()
	}
	output.DrawPanelContent(screen, image.Pt(640, 50), 620, etext.AlignStart)
	file, err := os.Create(filepath.Join(directory, fmt.Sprintf("combat-%s-%03d.png", name, age)))
	require.NoError(t, err)
	require.NoError(t, png.Encode(file, screen))
	require.NoError(t, file.Close())
}

func TestCombatHealthFramesActualSizeLookalikes(t *testing.T) {
	manager := display.GetManager()
	savedManager := *manager
	t.Cleanup(func() { *manager = savedManager })
	manager.PublishLogicalSize(1280, 720)
	scene, screen := newCombatConcealmentTestScene(t)
	scene.gameState.CombatScene.MonsterManager.Clear()
	scene.gameState.CombatScene.TurnQueue.RemoveAllMonsters()
	actor := scene.gameState.CombatScene.GetCurrentTurnIdentity()
	scene.gameState.PartyState.GetCharacter(actor.EntityIndex).Intelligence = 255
	for index, active := range scene.gameState.CombatScene.PartyActive {
		if active {
			character := scene.gameState.PartyState.GetCharacter(index)
			character.SetHp(character.GetMaxHp() - 1) // The battlefield marks only the injured.
			position := references.Position{X: references.Coordinate(index + 1), Y: 9}
			scene.gameState.CombatScene.SetPartyMemberPosition(index, position)
			setCombatConcealmentTerrain(scene.gameState, position, indexes.BrickFloor)
		}
	}
	ref := (*scene.gameState.GameReferences.EnemyReferences)[references.MonsterBard]
	ref.HitPoints = 100
	for i := range 6 {
		position := references.Position{X: references.Coordinate(2 + i), Y: 5}
		monster := game_state.NewCombatMonster(references.MonsterBard, &ref, position, scene.gameState.AsRandomProvider())
		index := scene.gameState.CombatScene.AddMonsterToQueue(monster)
		setCombatConcealmentTerrain(scene.gameState, position, indexes.BrickFloor)
		switch {
		case i >= 4:
			monster.SetCurrentHP(1)
		case i >= 2:
			monster.SetCurrentHP(max(1, monster.MaxHP()/4))
		default:
			monster.SetCurrentHP(monster.MaxHP() - 1)
		}
		if i%2 == 1 {
			scene.gameState.SetRandomSeed(42)
			require.True(t, scene.gameState.CastCombatAnXenEx(actor.EntityIndex, game_state.CombatTurnIdentity{EntityIndex: index}))
		}
	}
	scene.gameState.CombatScene.PartyPossessed[actor.EntityIndex] = true
	scene.updateCombatAllegiance()
	scene.drawAvatarOverlay(screen)
	assert.Positive(t, countExactColor(screen, ucolor.CombatHealthFriend))
	assert.Positive(t, countExactColor(screen, ucolor.CombatAllegianceEnemy))
	var sample image.Rectangle
	var healthOptions []*ebiten.DrawImageOptions
	scene.gameState.ForEachDisclosedCombatActor(func(view game_state.CombatActorPresentation) {
		options := scene.combatEntityDrawOptions(view.Position, view.Identity.IsPartyMember, view.Identity.EntityIndex, view.Identity.Slot)
		healthOptions = append(healthOptions, options)
		sample = sample.Union(combatFixtureRasterBounds(options))
	})
	sample = sample.Intersect(screen.Bounds())
	reference := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	foreground := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	t.Cleanup(reference.Dispose)
	t.Cleanup(foreground.Dispose)
	scene.gameState.ForEachDisclosedCombatActor(func(view game_state.CombatActorPresentation) {
		scene.drawCombatActorHealthFrame(reference, scene.combatEntityDrawOptions(view.Position, view.Identity.IsPartyMember, view.Identity.EntityIndex, view.Identity.Slot), view)
	})
	// The possessed PC's yellow full frame intentionally overlays its red
	// allegiance rim. Every uncovered health pixel, the complete 1/25/99-HP
	// fills and a readable side of every actor's allegiance border remain exact.
	require.Equal(t, actor, scene.gameState.CombatScene.GetCurrentTurnIdentity())
	activeOptions := scene.combatPartyIdentityDrawOptions(scene.gameState.CombatScene.GetCombatantPosition(actor), actor)
	drawActiveSelectionReference(foreground, activeOptions, scene.gameState.ElapsedMs, ucolor.CombatActiveMonsterOutline)
	assertCombatHealthSelectionComposition(t, screen.SubImage(sample).(*ebiten.Image), reference, foreground, healthOptions)
	captureCombatPolishFrame(t, scene, "health-frames-grass-low-hp", 0)
	captureCombatPolishFrame(t, scene, "health-frames-water-low-hp", 0)
}

func TestCombatCapturePreservesDisplayState(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	manager := display.GetManager()
	logicalWidth, logicalHeight := manager.LogicalSize()
	resolvedWidth, resolvedHeight := manager.GetCurrentSize()
	t.Setenv("ULTIMA5_REVIEW_IMAGES", t.TempDir())
	captureCombatPolishFrame(t, scene, "display-isolation", 0)
	width, height := manager.LogicalSize()
	assert.Equal(t, image.Pt(logicalWidth, logicalHeight), image.Pt(width, height))
	width, height = manager.GetCurrentSize()
	assert.Equal(t, image.Pt(resolvedWidth, resolvedHeight), image.Pt(width, height))
}
