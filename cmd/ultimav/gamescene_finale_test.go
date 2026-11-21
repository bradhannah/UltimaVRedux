package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	finaleFixtureMapOffset   = 528
	finaleFixtureMapRowBytes = 16
	finaleFixtureMapFileSize = 704
)

func TestLordBritishFinaleSuccessWorkflowReachesRetainedCertificate(t *testing.T) {
	scene := newLordBritishFinaleWorkflowTestScene(t, true)
	presentation := scene.finalePresentation
	require.NotNil(t, presentation)
	require.NotNil(t, scene.miniMapScene)
	lordBritish := scene.miniMapScene.GetActor(minimap.ActorLordBritish)
	require.NotNil(t, lordBritish)
	assert.True(t, lordBritish.Visible)
	assert.Equal(t, references.Position{X: 5, Y: 8}, lordBritish.Position)
	assert.True(t, scene.inputBlocked)

	driveLordBritishFinaleWorkflow(t, scene, ebiten.KeyY)

	assert.Equal(t, game_state.LordBritishFinaleSucceeded, scene.gameState.LordBritishFinaleOutcome())
	assert.Equal(t, finaleSuccessCertificate, presentation.mode)
	assert.NotNil(t, presentation.certificateOutput)
	assert.Equal(t, []string{
		"rescue-map", "lord-british-visible", "greeting", "box-prompt", "box-opened",
		"message-4", "message-5", "message-6", "message-7", "message-8", "box-to-gem",
		"moongate-rise", "ending-art", "art-1", "art-2", "art-3", "art-4", "art-5", "art-6",
		"certificate",
	}, presentation.trace)
	assert.Nil(t, presentation.dialog)
	assert.False(t, scene.dialogStack.HasOpenDialog())
	assert.Nil(t, scene.miniMapScene)
	assert.False(t, scene.gameState.EventQueueManager.HasWorkflow(lordBritishFinaleWorkflow))
	assert.False(t, scene.gameState.EventQueueManager.HasActiveWorkflows())
	for _, stray := range []ebiten.Key{ebiten.KeySpace, ebiten.KeyA, ebiten.KeyEscape, ebiten.KeyUp} {
		scene.keyboard.SimulateKeyPress(stray)
		assert.Equal(t, finaleInputNone, scene.updateLordBritishFinaleInput(),
			"a stray key must not dismiss the record of the game")
	}
	assert.Equal(t, finaleSuccessCertificate, presentation.mode)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	assert.Equal(t, finaleInputReturnToMainMenu, scene.updateLordBritishFinaleInput(), "only Enter leaves it")
	assert.False(t, scene.inputBlocked)
	assert.False(t, scene.gameState.CanAcceptGameplayCommands())
	gateTile, ok := scene.gameState.LordBritishFinaleTile(presentation.review.BoxMoongatePosition)
	require.True(t, ok)
	assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), gateTile)

	scene.Dispose()

	assert.Nil(t, scene.finalePresentation)
	assert.Nil(t, scene.miniMapScene)
	assert.False(t, scene.dialogStack.HasOpenDialog())
}

func TestLordBritishFinaleNoBoxFailureRetainsAndDisposesTerminalTableau(t *testing.T) {
	scene := newLordBritishFinaleWorkflowTestScene(t, false)
	presentation := scene.finalePresentation
	require.NotNil(t, presentation)

	driveLordBritishFinaleWorkflow(t, scene, ebiten.KeyY)

	assert.Equal(t, game_state.LordBritishFinaleFailed, scene.gameState.LordBritishFinaleOutcome())
	assert.Equal(t, finaleFailureScreen, presentation.mode)
	assert.Equal(t, []string{
		"rescue-map", "lord-british-visible", "greeting", "box-prompt",
		"failure-i-see", "failure-message", "failure-terminal", "recovery-choice",
	}, presentation.trace)
	require.NotNil(t, presentation.dialog)
	terminalTranscript := strings.Join(strings.Fields(presentation.dialog.TranscriptText()), " ")
	assert.Contains(t, terminalTranscript, "I see")
	assert.Contains(t, terminalTranscript, "remain here a while")
	assert.Contains(t, terminalTranscript, finaleReplyPrompt)
	assert.NotContains(t, terminalTranscript, "Yes or No?")
	require.NotNil(t, presentation.recoveryChoice)
	assert.Equal(t, []string{"Return Before Doom", "Return to Main Menu"}, presentation.recoveryChoice.ButtonTexts())
	assert.Same(t, presentation.recoveryChoice, scene.dialogStack.PeekTopModalDialog())
	assert.True(t, scene.dialogStack.HasOpenDialog())
	require.NotNil(t, scene.miniMapScene)
	lordBritish := scene.miniMapScene.GetActor(minimap.ActorLordBritish)
	require.NotNil(t, lordBritish)
	assert.True(t, lordBritish.Visible)
	assert.Equal(t, references.Position{X: 4, Y: 1}, lordBritish.Position)
	avatar := scene.miniMapScene.GetActor(minimap.ActorAvatar)
	require.NotNil(t, avatar)
	assert.Equal(t, references.Position{X: 8, Y: 4}, avatar.Position)
	assert.Equal(t, indexes.SpriteIndex(indexes.SleepingInBed), scene.miniMapActorSpriteForRender(lordBritish))
	assert.Equal(t, indexes.SpriteIndex(indexes.SitChairDown), scene.miniMapActorSpriteForRender(avatar))
	partyAtTable := scene.miniMapScene.AddActor(
		finalePartyActorID(2), references.Position{X: 8, Y: 6}, indexes.Fighter1,
	).GetActor(finalePartyActorID(2))
	require.NotNil(t, partyAtTable)
	assert.Equal(t, indexes.SpriteIndex(indexes.SitChairUpEat1), scene.miniMapActorSpriteForRender(partyAtTable))
	assert.False(t, scene.gameState.EventQueueManager.HasWorkflow(lordBritishFinaleWorkflow))
	assert.False(t, scene.gameState.EventQueueManager.HasActiveWorkflows())
	assert.False(t, scene.inputBlocked)
	assert.False(t, scene.gameState.CanAcceptGameplayCommands())

	scene.Dispose()

	assert.Nil(t, scene.finalePresentation)
	assert.Nil(t, scene.miniMapScene)
	assert.False(t, scene.dialogStack.HasOpenDialog())
}

func TestLordBritishFinaleRecoveryChoiceWaitsAdditionalDeterministicFiveSeconds(t *testing.T) {
	scene := newLordBritishFinaleWorkflowTestScene(t, false)
	driveLordBritishFinaleUntilTrace(t, scene, "failure-terminal", ebiten.KeyY)
	presentation := scene.finalePresentation

	assert.Nil(t, presentation.recoveryChoice)
	scene.gameState.ElapsedMs += finaleRecoveryDelayMs - 1
	require.NoError(t, scene.Update(&Game{currentScene: scene}))
	assert.Nil(t, presentation.recoveryChoice)
	scene.gameState.ElapsedMs++
	require.NoError(t, scene.Update(&Game{currentScene: scene}))
	require.NotNil(t, presentation.recoveryChoice)
	assert.Same(t, presentation.recoveryChoice, scene.dialogStack.PeekTopModalDialog())
}

func TestLordBritishFinaleNonMissingBoxFailureDoesNotOfferRecovery(t *testing.T) {
	scene := newLordBritishFinaleWorkflowTestScene(t, true)
	presentation := scene.finalePresentation
	presentation.mode = finaleFailureScreen
	presentation.recoveryOffered = false
	presentation.dialog.SetControlHint("[Any key] Return to Main Menu")
	scene.keyboard = input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)

	assert.Equal(t, finaleFailureScreen, presentation.mode)
	assert.False(t, presentation.recoveryOffered)
	assert.Nil(t, presentation.recoveryChoice)
	assert.NotContains(t, presentation.dialog.TranscriptText(), "Return Before Doom")
	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	assert.Equal(t, finaleInputReturnToMainMenu, scene.updateLordBritishFinaleInput())
}

func TestLordBritishRecoveryChoiceOwnsUpdateInputUntilReleasedAndCleansExactly(t *testing.T) {
	scene := newLordBritishFinaleWorkflowTestScene(t, false)
	driveLordBritishFinaleUntilTrace(t, scene, "failure-terminal", ebiten.KeyY)
	held := map[ebiten.Key]bool{ebiten.KeyEnter: true}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
	scene.gameState.ElapsedMs += finaleRecoveryDelayMs
	scene.gameState.EventQueueManager.Update()
	presentation := scene.finalePresentation
	choice := presentation.recoveryChoice
	require.NotNil(t, choice)

	menu := &playerSaveTestScene{configuration: scene.gameConfig}
	game := NewGame(scene)
	game.mainMenuSceneBuilder = func(*config.UltimaVConfiguration) (Scene, error) { return menu, nil }

	require.NoError(t, scene.Update(game))
	assert.Equal(t, finaleInputNone, presentation.pendingInput)
	assert.Same(t, scene, game.currentScene)

	held[ebiten.KeyEnter] = false
	require.NoError(t, scene.Update(game))
	assert.Equal(t, finaleInputNone, presentation.pendingInput, "release frame must not choose")

	decoyCount := 0
	decoy := widgets.NewButtonListModal("Other Modal", func() {}, scene.keyboard, &gameScreenPercents)
	decoy.AddButton("Only Decoy", func() { decoyCount++ })
	scene.dialogStack.PushModalDialog(decoy)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	require.NoError(t, scene.Update(game))
	assert.Equal(t, 1, decoyCount)
	assert.Equal(t, finaleInputNone, presentation.pendingInput)
	assert.Same(t, decoy, scene.dialogStack.PeekTopModalDialog())
	scene.dialogStack.MustRemoveWidgetInstance(decoy)
	assert.Same(t, choice, scene.dialogStack.PeekTopModalDialog())

	held[ebiten.KeyDown] = true
	require.NoError(t, scene.Update(game))
	held[ebiten.KeyDown] = false
	require.NoError(t, scene.Update(game))
	held[ebiten.KeyEnter] = true
	require.NoError(t, scene.Update(game))
	assert.Equal(t, finaleInputReturnToMainMenu, presentation.pendingInput)
	assert.Same(t, scene, game.currentScene, "choice action is consumed on the next scene update")
	held[ebiten.KeyEnter] = false
	require.NoError(t, scene.Update(game))

	assert.Same(t, menu, game.currentScene)
	assert.True(t, scene.disposed)
	assert.Zero(t, scene.dialogStack.Count())
	assert.Nil(t, scene.finalePresentation)
}

func TestLordBritishRecoveryFailureReplacesExactChoiceAndRetainsMenuRoute(t *testing.T) {
	scene := newLordBritishFinaleWorkflowTestScene(t, false)
	driveLordBritishFinaleWorkflow(t, scene, ebiten.KeyY)
	presentation := scene.finalePresentation
	oldChoice := presentation.recoveryChoice
	require.NotNil(t, oldChoice)
	game := NewGame(scene)

	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	require.NoError(t, scene.Update(game))
	assert.Equal(t, finaleInputReturnBeforeDoom, presentation.pendingInput)
	require.NoError(t, scene.Update(game))

	assert.Same(t, scene, game.currentScene)
	require.NotNil(t, presentation.recoveryChoice)
	assert.NotSame(t, oldChoice, presentation.recoveryChoice)
	assert.NotContains(t, scene.dialogStack.Dialogs(), oldChoice)
	assert.Same(t, presentation.recoveryChoice, scene.dialogStack.PeekTopModalDialog())
	assert.Equal(t, []string{"Return Before Doom", "Return to Main Menu"}, presentation.recoveryChoice.ButtonTexts())
	assert.Contains(t, presentation.trace, "recovery-unavailable:This Doom run has no matching recovery checkpoint")

	scene.Dispose()
	assert.Zero(t, scene.dialogStack.Count())
}

func TestFinalePaletteRemapsOnlyOriginalDriverTileSet(t *testing.T) {
	require.NotNil(t, getFinalePaletteShader())
	for _, sprite := range []indexes.SpriteIndex{
		indexes.BrickFloor, indexes.BookcaseLeft, indexes.BookcaseRight,
		indexes.ChairBackForward, indexes.ChairBackBack,
		indexes.TableLeft, indexes.TableRight, indexes.TableFoodBottom,
		indexes.LeftBed, indexes.RightBed, indexes.Box,
		indexes.RightSconce, indexes.LeftSconce, indexes.CookStove,
		indexes.Moongate, indexes.ItemGem, indexes.ItemSandalwoodBox,
		indexes.SleepingInBed,
		indexes.SitChairUpEat1, indexes.SitChairUpEat2,
		indexes.SitChairUpEat3, indexes.SitChairUpEat4,
	} {
		assert.True(t, finalePaletteRemapsSprite(sprite), "sprite %d", sprite)
	}
	for _, sprite := range []indexes.SpriteIndex{
		indexes.StoneBrickWall, indexes.LordBritish1, indexes.AvatarKeyIndex, indexes.SitChairDown,
	} {
		assert.False(t, finalePaletteRemapsSprite(sprite), "sprite %d", sprite)
	}
}

func TestFinaleMoongateShaderSwapsOnlyBluePaletteEntries(t *testing.T) {
	source := ebiten.NewImage(4, 1)
	source.Set(0, 0, ucolor.EGABlue)
	source.Set(1, 0, ucolor.EGALightBlue)
	source.Set(2, 0, ucolor.EGAWhite)
	source.Set(3, 0, ucolor.EGATransparent)
	target := ebiten.NewImage(4, 1)
	target.DrawRectShader(4, 1, getFinaleMoongateShader(), &ebiten.DrawRectShaderOptions{
		Images: [4]*ebiten.Image{source, nil, nil, nil},
	})

	assert.Equal(t, ucolor.EGARed.ToRGBA(), target.At(0, 0))
	assert.Equal(t, ucolor.EGALightRed.ToRGBA(), target.At(1, 0))
	assert.Equal(t, ucolor.EGAWhite.ToRGBA(), target.At(2, 0))
	assert.Equal(t, ucolor.EGATransparent.ToRGBA(), target.At(3, 0))
}

func TestFinaleArtPlanMatchesEndgameSourceOrder(t *testing.T) {
	require.Len(t, finaleArtFrames, 6)
	assert.Equal(t, []introNarrativeShape{
		shape("TEXT.16", 0, 216, 0), shape("TEXT.16", 4, 152, 28), shape("END1.16", 0, 0, 0),
	}, finaleArtFrames[0].shapes)
	assert.Equal(t, shape("END1.16", 1, 64, 0), finaleArtFrames[1].shapes[0])
	assert.Equal(t, shape("END1.16", 2, 0, 52), finaleArtFrames[2].shapes[0])
	assert.Equal(t, []introNarrativeShape{
		shape("TEXT.16", 5, 224, 0), shape("TEXT.16", 0, 176, 0), shape("END2.16", 0, 0, 0),
	}, finaleArtFrames[3].shapes)
	assert.Equal(t, shape("END2.16", 1, 0, 92), finaleArtFrames[4].shapes[0])
	assert.Equal(t, shape("END2.16", 2, 160, 0), finaleArtFrames[5].shapes[0])
	assert.Equal(t, []int{0, 1, 2, 3, 4, 5}, []int{
		finaleArtFrames[0].texts[0].storyBlock, finaleArtFrames[1].texts[0].storyBlock,
		finaleArtFrames[2].texts[0].storyBlock, finaleArtFrames[3].texts[0].storyBlock,
		finaleArtFrames[4].texts[0].storyBlock, finaleArtFrames[5].texts[0].storyBlock,
	})
}

func TestFinaleCertificateSourceFormatting(t *testing.T) {
	assert.Equal(t, "First", finaleOrdinal(1))
	assert.Equal(t, "Thirteenth", finaleOrdinal(13))
	assert.Equal(t, "Twentieth", finaleOrdinal(20))
	assert.Equal(t, "Twenty-Eighth", finaleOrdinal(28))
	assert.Equal(t, "One", finaleCardinal(1))
	assert.Equal(t, "Ninety-Nine", finaleCardinal(99))
	assert.Equal(t, "2 years, 3 months, 6 days", finaleDurationText(2, 3, 6))
}

func TestFinaleCertificateUsesOriginalStaticRows(t *testing.T) {
	scene := newLordBritishFinaleWorkflowTestScene(t, true)
	driveLordBritishFinaleWorkflow(t, scene, ebiten.KeyY)

	rows := strings.Split(scene.finalePresentation.certificateOutput.GetOutputStr(true), " \n")
	require.Len(t, rows, 23)
	assert.Equal(t, "Be it known that on", rows[0])
	assert.Equal(t, "THE QUEST OF THE AVATAR", rows[15])
	assert.Equal(t, "IS FOREVER", rows[16])
	assert.Empty(t, rows[17])
	assert.Empty(t, rows[18])
	assert.Empty(t, rows[19])
	assert.Equal(t, "Report now, thy Quest compleat in", rows[20])
	assert.Equal(t, "to Lord British at Origin Systems!", rows[22])
}

func TestFinaleGreetingNormalizesSourceLineBreakBeforeAvatarName(t *testing.T) {
	assert.Equal(t,
		"Lord British says:\n\n\"Well met, Brad!\"",
		finaleGreetingText("Lord British says:\n\n\"Well met,\n", "Brad"),
	)
}

func TestFinaleQuestionTextSeparatesSourceReplyPrompt(t *testing.T) {
	assert.Equal(t, "Didst thou bring my box?", finaleQuestionText(
		"Didst thou bring my box?\n\nYou reply: ",
	))
	assert.Panics(t, func() { finaleQuestionText("Didst thou bring my box?") })
}

func TestFinaleActorDepartureFizzlesAtMoongate(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	gate := references.Position{X: 5, Y: 4}
	presentation := new(lordBritishFinalePresentation)
	scene := &GameScene{
		gameState:   state,
		spriteSheet: sharedPresentationTestSpriteSheet(),
		miniMapScene: minimap.NewMiniMapScene().AddActor(
			minimap.ActorLordBritish,
			gate,
			indexes.LordBritish1,
		),
		finalePresentation: presentation,
	}
	state.EventQueueManager.Workflow("test_finale_departure").
		Priority(event_queue.PriorityCritical).
		Sequential().
		First(func() {}).
		ThenSequence(scene.thenFinaleActorDeparture(minimap.ActorLordBritish, gate, presentation)).
		Start()

	sawFizzle := false
	maxProgress := 0.0
	for range 100 {
		state.ElapsedMs += finaleRefreshMs
		state.EventQueueManager.Update()
		if len(scene.activeTileDissolves) > 0 {
			sawFizzle = true
			dissolve := scene.activeTileDissolves[0]
			assert.Equal(t, gate, dissolve.Position)
			assert.Equal(t, indexes.SpriteIndex(indexes.LordBritish1), dissolve.SpriteIndex)
			assert.False(t, scene.miniMapScene.GetActor(minimap.ActorLordBritish).Visible)
			maxProgress = max(maxProgress, dissolve.Effect.GetLevel())
		}
		if !state.EventQueueManager.HasActiveWorkflows() {
			break
		}
	}

	assert.True(t, sawFizzle)
	assert.Greater(t, maxProgress, 0.0)
	assert.Empty(t, scene.activeTileDissolves)
	assert.Zero(t, presentation.departureDissolve)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
}

func TestFinaleCertificateAcknowledgementReturnsToMainMenu(t *testing.T) {
	scene := newLordBritishFinaleWorkflowTestScene(t, true)
	driveLordBritishFinaleWorkflow(t, scene, ebiten.KeyY)
	scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(ebiten.Key) bool { return false })
	replacement := &playerSaveTestScene{configuration: scene.gameConfig}
	game := NewGame(scene)
	game.mainMenuSceneBuilder = func(configuration *config.UltimaVConfiguration) (Scene, error) {
		assert.Same(t, scene.gameConfig, configuration)

		return replacement, nil
	}

	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	require.NoError(t, scene.Update(game))
	assert.Same(t, scene, game.currentScene, "a stray key leaves the certificate up")
	assert.False(t, scene.disposed)

	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	require.NoError(t, scene.Update(game))

	assert.Same(t, replacement, game.currentScene)
	assert.True(t, scene.disposed)
}

func TestFinaleCancellationCleansPresentationOwnedState(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{
		gameState: state,
		miniMapScene: minimap.NewMiniMapScene().AddActor(
			minimap.ActorLordBritish,
			references.Position{X: 5, Y: 8},
			0,
		),
		finalePresentation: &lordBritishFinalePresentation{
			mode: finaleRescueMap,
		},
		screenFlashIntensity: 1,
		mapFadeLevel:         1,
	}
	state.EventQueueManager.Workflow(lordBritishFinaleWorkflow).
		Priority(event_queue.PriorityCritical).
		Sequential().
		First(func() {}).
		While(func() bool { return true }, func() {}, 1).
		Finally(func() { scene.clearLordBritishFinalePresentation(false) }).
		Start()

	scene.Dispose()

	assert.Nil(t, scene.finalePresentation)
	assert.Nil(t, scene.miniMapScene)
	assert.Zero(t, scene.screenFlashIntensity)
	assert.Zero(t, scene.mapFadeLevel)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
}

func newLordBritishFinaleWorkflowTestScene(t *testing.T, hasBox bool) *GameScene {
	t.Helper()
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Doom).
		WithFloor(7).
		WithPlayerAt(5, 7).
		WithSystemCallbacks().
		WithRandomSeed(1).
		Build()
	for index := 1; index < party_state.MaxActiveParty; index++ {
		state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
	}
	avatar := state.PartyState.GetAvatar()
	avatar.PartyMap = party_state.PartyMapActive
	avatar.SetStatus(party_state.Good)
	avatar.SetHp(avatar.GetMaxHp())
	state.PartyState.Inventory.QuestItems.Set(references.WoodenBox, 0)
	if hasBox {
		state.PartyState.Inventory.QuestItems.Set(references.WoodenBox, 1)
	}
	font, _ := newMainOutput(1280)
	scene := &GameScene{
		gameConfig: state.GameConfig,
		gameState:  state,
		// Simulated prompt input must not mix with keys held on the host desktop.
		keyboard:             input.NewKeyboardWithKeyStateSource(0, func(ebiten.Key) bool { return false }),
		ultimaFont:           font,
		spriteSheet:          sharedPresentationTestSpriteSheet(),
		activeEffectsOverlay: NewActiveEffectsOverlay(nil),
		moongateVisualSystem: NewMoongateVisualSystem(state),
		windDisplay:          NewWindDisplay(),
	}
	systemCallbacks := callbacks.ToSystemCallbacks()
	systemCallbacks.Visual.BlockAllInput = scene.blockAllInput
	systemCallbacks.Combat.PublishConclusion = scene.recordCombatConclusion
	state.SystemCallbacks = systemCallbacks
	state.SetupEnvironmentalSystemCallbacks()
	installVerifiedDoomMirrorCombat(t, state)

	state.FinishCombatAction()

	require.Nil(t, state.CombatScene)
	require.True(t, state.HasPendingLordBritishFinale())
	require.NotNil(t, scene.combatConclusion)
	require.Equal(t, game_state.CombatSpecialConclusionDoomMirror, scene.combatConclusion.Special)
	state.GameConfig = finaleMapFixtureConfig(t)
	review, err := state.BeginLordBritishFinale()
	require.NoError(t, err)
	scene.combatConclusion = nil
	scene.startLordBritishFinale(review, finaleAssetFixture())

	return scene
}

func installVerifiedDoomMirrorCombat(t *testing.T, state *game_state.GameState) {
	t.Helper()
	mapIndex := references.CalculateDungeonCombatMapIndex(references.DoomDungeon, 15)
	combatMap := references.NewCombatMapReference(mapIndex, references.DungeonCombatTerritory)
	var rawMap [references.XCombatMapTiles][references.YCombatMapTiles]byte
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			rawMap[x][y] = byte(indexes.BrickFloor)
		}
	}
	rawMap[5][1] = byte(indexes.Mirror)
	combatMap.SetRawData(&rawMap)
	combat := game_state.NewCombatState(combatMap, game_state.CombatEntryNorth, &state.PartyState)
	combat.PreCombatPlayerLocation = references.PlayerLocation{
		Location: references.Doom, Floor: 7, Position: references.Position{X: 5, Y: 7},
	}
	combat.ExitContext = game_state.CombatExitDungeonRoom
	combat.DungeonRoomOrigin = &game_state.DungeonRoomEncounterOrigin{
		Location: references.Doom, Floor: 7, Position: references.Position{X: 5, Y: 7},
		DungeonIndex: references.DoomDungeon, Room: 15, Live: true,
	}
	combat.SetPartyMemberPosition(0, references.Position{X: 5, Y: 2})
	state.CombatScene = combat
	state.MapState.PlayerLocation = references.PlayerLocation{
		Location: references.CombatLocation, Position: references.Position{X: 5, Y: 5},
	}
	state.MapState.LayeredMaps.ResetAndCreateCombatMap(
		combatMap, state.GameReferences.TileReferences, 19, 13,
	)
}

func finaleMapFixtureConfig(t *testing.T) *config.UltimaVConfiguration {
	t.Helper()
	directory := t.TempDir()
	data := make([]byte, finaleFixtureMapFileSize)
	for row := range minimap.MiscMapSize {
		for column := range minimap.MiscMapSize {
			data[finaleFixtureMapOffset+row*finaleFixtureMapRowBytes+column] = byte(indexes.BrickFloor)
		}
	}
	data[finaleFixtureMapOffset+1*finaleFixtureMapRowBytes+4] = byte(indexes.LeftBed)
	data[finaleFixtureMapOffset+4*finaleFixtureMapRowBytes+8] = byte(indexes.ChairBackBack)
	data[finaleFixtureMapOffset+5*finaleFixtureMapRowBytes+8] = byte(indexes.TableFoodBottom)
	data[finaleFixtureMapOffset+6*finaleFixtureMapRowBytes+8] = byte(indexes.ChairBackForward)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "MISCMAPS.DAT"), data, 0o600))

	return &config.UltimaVConfiguration{
		SavedConfigData: &config.UltimaVConfigurationFlags{DataFilePath: directory},
	}
}

func finaleAssetFixture() *sprites.EndgameAssets {
	messages := []string{
		"Lord British says: \"Well met, ",
		"Didst thou bring my box?\n\nYou reply: ",
		"The sandalwood box?\n\nDidst thou bring it?\n\nYou reply: ",
		"Lord British carefully opens the box.",
		"Success message four.",
		"Success message five.",
		"Success message six.",
		"Success message seven.",
		"Success message eight.",
		"The gem is revealed.",
		"We must remain here a while.",
	}

	return &sprites.EndgameAssets{
		Shapes: make(map[string][]*ebiten.Image), Narrative: nil,
		ArtText:  []string{"art-1", "art-2", "art-3", "art-4", "art-5", "art-6"},
		Messages: messages,
	}
}

func driveLordBritishFinaleWorkflow(t *testing.T, scene *GameScene, answer ebiten.Key) {
	t.Helper()
	const maximumUpdates = 1000
	for range maximumUpdates {
		if !scene.gameState.EventQueueManager.HasActiveWorkflows() {
			return
		}
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		presentation := scene.finalePresentation
		if presentation.dialog != nil {
			if presentation.dialog.ActivePrompt() != "" {
				require.Equal(t, finaleReplyPrompt, presentation.dialog.ActivePrompt())
				scene.keyboard.SimulateKeyPress(answer)
				presentation.dialog.Update()
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
				presentation.dialog.Update()
			} else if !presentation.dialog.InputComplete() {
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
				presentation.dialog.Update()
			}
		}
		if presentation.awaitingAdvance {
			scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
			scene.updateLordBritishFinaleInput()
		}
	}
	t.Fatalf("Lord British finale workflow did not complete; trace=%v", scene.finalePresentation.trace)
}

func driveLordBritishFinaleUntilTrace(t *testing.T, scene *GameScene, target string, answer ebiten.Key) {
	t.Helper()
	for range 1000 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		presentation := scene.finalePresentation
		if presentation.dialog != nil {
			if presentation.dialog.ActivePrompt() != "" {
				scene.keyboard.SimulateKeyPress(answer)
				presentation.dialog.Update()
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
				presentation.dialog.Update()
			} else if !presentation.dialog.InputComplete() {
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
				presentation.dialog.Update()
			}
		}
		for _, step := range presentation.trace {
			if step == target {
				return
			}
		}
	}
	t.Fatalf("Lord British finale did not reach %q; trace=%v", target, scene.finalePresentation.trace)
}
