package main

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/internal/visual"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const (
	lordBritishFinaleWorkflow = "lord_british_finale"
	finaleRefreshMs           = 56
	finaleWalkDelayMs         = 5 * finaleRefreshMs
	finaleLongWaitMs          = 40 * finaleRefreshMs
	finaleShortWaitMs         = 4 * finaleRefreshMs
	finaleGateDurationMs      = 15 * finaleRefreshMs
	finaleRecoveryDelayMs     = 5000
	finaleReplyPrompt         = "You reply:"
	finaleRecoveryButtonWidth = 0.28
	finaleRecoveryCardWidth   = 0.42
)

type finalePresentationMode uint8

const (
	finaleRescueMap finalePresentationMode = iota
	finaleEndingArt
	finaleSuccessCertificate
	finaleFailureScreen
)

type lordBritishFinalePresentation struct {
	assets            *sprites.EndgameAssets
	review            game_state.LordBritishFinaleReview
	mode              finalePresentationMode
	artFrame          int
	awaitingAdvance   bool
	moongateProgress  float32
	showMoongate      bool
	certificateOutput *text.Output
	dialog            *widgets.InteractionTranscript
	canvas            *ebiten.Image
	departureDissolve int
	recoveryOffered   bool
	recoveryChoice    *widgets.ButtonListModal
	pendingInput      finaleInputAction
	trace             []string
}

type finaleInputAction uint8

const (
	finaleInputNone finaleInputAction = iota
	finaleInputReturnBeforeDoom
	finaleInputReturnToMainMenu
)

type finaleArtFrame struct {
	shapes []introNarrativeShape
	texts  []introNarrativeText
}

var finaleArtFrames = [...]finaleArtFrame{ //nolint:gochecknoglobals // Immutable finale layout table; ref:DNG-0015.
	{
		shapes: []introNarrativeShape{shape("TEXT.16", 0, 216, 0), shape("TEXT.16", 4, 152, 28), shape("END1.16", 0, 0, 0)},
		texts:  []introNarrativeText{storyText(0, 172, 66, 172, 320, 0, 320, 126, 200)},
	},
	{
		shapes: []introNarrativeShape{shape("END1.16", 1, 64, 0)},
		texts:  []introNarrativeText{storyText(1, 0, 92, 0, 320, 0, 320, 126, 200)},
	},
	{
		shapes: []introNarrativeShape{shape("END1.16", 2, 0, 52)},
		texts:  []introNarrativeText{storyText(2, 0, 9, 0, 320, 196, 320, 42, 148)},
	},
	{
		shapes: []introNarrativeShape{shape("TEXT.16", 5, 224, 0), shape("TEXT.16", 0, 176, 0), shape("END2.16", 0, 0, 0)},
		texts:  []introNarrativeText{storyText(3, 179, 38, 179, 320, 0, 320, 100, 200)},
	},
	{
		shapes: []introNarrativeShape{shape("END2.16", 1, 0, 92)},
		texts:  []introNarrativeText{storyText(4, 0, 9, 0, 320, 161, 320, 82, 200)},
	},
	{
		shapes: []introNarrativeShape{shape("END2.16", 2, 160, 0)},
		texts:  []introNarrativeText{storyText(5, 0, 0, 0, 154, 0, 320, 112, 200)},
	},
}

func (g *GameScene) consumeDoomMirrorFinaleHandoff() (bool, error) {
	if g.combatConclusion == nil || g.combatConclusion.Special != game_state.CombatSpecialConclusionDoomMirror {
		return false, nil
	}
	assets, err := sprites.LoadEndgameAssets(g.gameConfig.SavedConfigData.DataFilePath)
	if err != nil {
		return false, fmt.Errorf("loading Lord British finale assets: %w", err)
	}
	review, err := g.gameState.BeginLordBritishFinale()
	if err != nil {
		return false, fmt.Errorf("beginning Lord British finale: %w", err)
	}
	g.combatConclusion = nil
	g.startLordBritishFinale(review, assets)

	return true, nil
}

func (g *GameScene) startLordBritishFinale(review game_state.LordBritishFinaleReview, assets *sprites.EndgameAssets) {
	if len(assets.Messages) != 11 || len(assets.ArtText) != len(finaleArtFrames) {
		panic("startLordBritishFinale: validated ending records are unavailable")
	}
	presentation := &lordBritishFinalePresentation{
		assets: assets, review: review, mode: finaleRescueMap, artFrame: 0,
		awaitingAdvance: false, moongateProgress: 0, showMoongate: false,
		certificateOutput: nil, dialog: nil, canvas: ebiten.NewImage(320, 200), departureDissolve: 0,
		recoveryOffered: false, recoveryChoice: nil, pendingInput: finaleInputNone,
		trace: make([]string, 0, 32),
	}
	g.finalePresentation = presentation
	g.miniMapScene = minimap.NewMiniMapScene().
		AddActor(minimap.ActorLordBritish, review.LordBritishStart, indexes.LordBritish1)
	presentation.record("rescue-map")

	transcriptPlacement := cinematicTranscriptPlacement()
	dialog := widgets.NewInteractionTranscript(g.keyboard, widgets.InteractionTranscriptConfig{
		MaxLines: 14, MaxInputLength: 1,
		PanelPlacement: &transcriptPlacement,
		WidthSamples: []string{
			"Lord British says: Well met, " + review.AvatarName + "!",
			assets.Messages[1], assets.Messages[2], assets.Messages[10],
		},
	})
	presentation.dialog = dialog
	removeDialog := func() {
		if dialog != nil {
			g.dialogStack.RemoveWidgetInstance(dialog)
			dialog = nil
			presentation.dialog = nil
		}
	}

	workflow := g.gameState.EventQueueManager.Workflow(lordBritishFinaleWorkflow).
		Priority(event_queue.PriorityCritical).
		Sequential().BlockInput().
		First(func() {
			presentation.record("lord-british-visible")
		}).
		Wait(finaleLongWaitMs).
		ThenSequence(g.thenRunMiniMapScript(
			minimap.NewMiniMapScript("finale_lord_british_arrival").
				WalkTo(minimap.ActorLordBritish, review.LordBritishEnd, finaleWalkDelayMs).
				WithWalkingSounds().Build(),
		))

	for _, actor := range review.Party {
		actorID := finalePartyActorID(actor.PartyIndex)
		if actor.Revived {
			workflow.Then(func() {
				name := g.gameState.PartyState.GetCharacter(actor.PartyIndex).GetNameAsString()
				g.gameState.SystemCallbacks.Message.AddRowStr(name + " lives!")
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundResurrect)
				g.gameState.SystemCallbacks.Visual.SetScreenFlashColor(1, 1, 1)
				presentation.record("revive-" + name)
			}).Animate(400, func(progress float32) {
				g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(math.Sin(float64(progress)*math.Pi) * 0.8)
			}).Then(func() {
				g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(0)
			})
		}
		workflow.Then(func() {
			g.miniMapScene.AddActor(actorID, actor.Start, actor.Sprite)
		}).ThenSequence(g.thenRunMiniMapScript(
			minimap.NewMiniMapScript(fmt.Sprintf("finale_party_%d_arrival", actor.PartyIndex)).
				WalkTo(actorID, actor.Destination, finaleWalkDelayMs).
				WithWalkingSounds().Build(),
		))
	}

	var answerResult game_state.LordBritishFinaleAnswerResult
	workflow.Wait(finaleLongWaitMs).Then(func() {
		dialog.AppendLine(finaleGreetingText(assets.Messages[0], review.AvatarName))
		g.dialogStack.PushModalDialog(dialog)
		presentation.record("greeting")
	}).ThenSequence(g.thenWaitForInteractionContinuation(dialog)).Then(func() {
		dialog.AppendLine(finaleQuestionText(assets.Messages[1]))
		presentation.record("box-prompt")
	}).ThenSequence(g.thenPromptInteractionUppercaseYesNo(
		dialog,
		finaleReplyPrompt,
		func(q *event_queue.SequentialEventBuilder) {
			q.Then(func() { answerResult = g.gameState.SubmitLordBritishFinaleAnswer('Y') })
		},
		func(q *event_queue.SequentialEventBuilder) {
			q.Then(func() {
				answerResult = g.gameState.SubmitLordBritishFinaleAnswer('N')
				dialog.AppendLine(finaleQuestionText(assets.Messages[2]))
				presentation.record("sandalwood-prompt")
			}).ThenSequence(g.thenPromptInteractionUppercaseYesNo(
				dialog,
				finaleReplyPrompt,
				func(second *event_queue.SequentialEventBuilder) {
					second.Then(func() { answerResult = g.gameState.SubmitLordBritishFinaleAnswer('Y') })
				},
				func(second *event_queue.SequentialEventBuilder) {
					second.Then(func() { answerResult = g.gameState.SubmitLordBritishFinaleAnswer('N') })
				},
			))
		},
	)).If(
		func() bool { return answerResult.Outcome == game_state.LordBritishFinaleSucceeded },
		func(q *event_queue.SequentialEventBuilder) {
			g.addLordBritishSuccessFinale(q, dialog, removeDialog, presentation)
		},
	).If(
		func() bool { return answerResult.Outcome == game_state.LordBritishFinaleFailed },
		func(q *event_queue.SequentialEventBuilder) {
			g.addLordBritishFailureFinale(q, dialog, presentation, answerResult.MissingBox)
		},
	).Finally(func() {
		if presentation.mode != finaleFailureScreen {
			removeDialog()
		}
		g.clearLordBritishFinalePresentation(true)
		g.gameState.EventQueueManager.Shutdown()
	}).Start()
}

func (g *GameScene) addLordBritishSuccessFinale(
	q *event_queue.SequentialEventBuilder,
	dialog *widgets.InteractionTranscript,
	removeDialog func(),
	presentation *lordBritishFinalePresentation,
) {
	avatarID := finalePartyActorID(0)
	boxID := minimap.ActorID("sandalwood-box")
	gate := presentation.review.BoxMoongatePosition
	q.Wait(8 * finaleRefreshMs).
		ThenSequence(g.thenRunMiniMapScript(minimap.NewMiniMapScript("finale_avatar_handoff").
			WalkTo(avatarID, gate, finaleWalkDelayMs).WithWalkingSounds().
			WalkTo(avatarID, gate.Down(), finaleWalkDelayMs).WithWalkingSounds().Build())).
		Wait(finaleShortWaitMs).
		Then(func() {
			g.miniMapScene.AddActor(boxID, gate, indexes.ItemSandalwoodBox)
			dialog.AppendLine(presentation.assets.Messages[3])
			presentation.record("box-opened")
		}).Wait(finaleLongWaitMs).Then(func() {
		dialog.AppendLine("He says:")
	}).ThenSequence(g.thenWaitForInteractionContinuation(dialog))

	for message := 4; message <= 8; message++ {
		q.Then(func() {
			dialog.AppendLine(presentation.assets.Messages[message])
			presentation.record(fmt.Sprintf("message-%d", message))
		}).ThenSequence(g.thenWaitForInteractionContinuation(dialog))
	}
	q.Then(func() {
		dialog.AppendLine(presentation.assets.Messages[9])
		g.miniMapScene.SetActorSprite(boxID, indexes.ItemGem)
		presentation.record("box-to-gem")
	}).ThenSequence(g.thenWaitForInteractionContinuation(dialog)).Then(func() {
		g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundAbsorb)
		g.miniMapScene.SetActorVisible(boxID, false)
		presentation.showMoongate = true
		presentation.moongateProgress = 0
		g.gameState.SetLordBritishFinaleTile(gate, indexes.BrickFloor)
		presentation.record("moongate-rise")
	}).Animate(finaleGateDurationMs, func(progress float32) {
		presentation.moongateProgress = progress
	}).Then(func() {
		presentation.moongateProgress = 1
	}).Wait(finaleShortWaitMs).
		ThenSequence(g.thenFinaleActorDeparture(minimap.ActorLordBritish, gate, presentation))

	for index := range presentation.review.Party {
		q.ThenSequence(g.thenFinaleActorDeparture(finalePartyActorID(index), gate, presentation))
	}
	q.Animate(finaleGateDurationMs, func(progress float32) {
		presentation.moongateProgress = 1 - progress
	}).Then(func() {
		presentation.showMoongate = false
		g.gameState.SetLordBritishFinaleTile(gate, indexes.BrickFloor)
		removeDialog()
		presentation.mode = finaleEndingArt
		presentation.record("ending-art")
	})
	for frame := range finaleArtFrames {
		q.Then(func() {
			presentation.artFrame = frame
			presentation.awaitingAdvance = true
			g.keyboard.PrepareForTransition(0)
			presentation.record(fmt.Sprintf("art-%d", frame+1))
		}).While(func() bool { return presentation.awaitingAdvance }, func() {}, DialogPollingRateMs)
	}
	q.Then(func() {
		presentation.mode = finaleSuccessCertificate
		presentation.certificateOutput = g.newFinaleCertificateOutput()
		g.keyboard.PrepareForTransition(0)
		presentation.record("certificate")
	})
}

func (g *GameScene) addLordBritishFailureFinale(
	q *event_queue.SequentialEventBuilder,
	dialog *widgets.InteractionTranscript,
	presentation *lordBritishFinalePresentation,
	recoveryOffered bool,
) {
	presentation.recoveryOffered = recoveryOffered
	q.Then(func() {
		dialog.AppendLine("\"I see...")
		presentation.record("failure-i-see")
	}).Wait(finaleLongWaitMs).Then(func() {
		dialog.AppendLine(presentation.assets.Messages[10])
		presentation.record("failure-message")
		avatar := g.miniMapScene.GetActor(finalePartyActorID(0))
		avatar.Position = avatar.Position.Up()
		g.PlayWalkingSoundAt(avatar.Position)
	}).Wait(finaleWalkDelayMs).ThenSequence(
		g.thenRunParallelFinaleWalks([]finaleWalkTarget{
			{id: finalePartyActorID(2), target: references.Position{X: 8, Y: 6}},
			{id: minimap.ActorLordBritish, target: references.Position{X: 4, Y: 1}},
			{id: finalePartyActorID(0), target: references.Position{X: 8, Y: 4}},
		}),
	).Then(func() {
		// REDUX DEVIATION: DNG-0015: original finale randomly wanders four companions forever
		// after the failure tableau. Redux freezes that tableau so it consumes no more
		// gameplay RNG and, for a genuine missing-box failure, later offers recovery.
		presentation.mode = finaleFailureScreen
		g.keyboard.PrepareForTransition(0)
		presentation.record("failure-terminal")
	})
	if !recoveryOffered {
		q.Then(func() {
			dialog.SetControlHint("[Any key] Return to Main Menu")
		})

		return
	}
	q.Wait(finaleRecoveryDelayMs).Then(func() {
		g.showDoomRecoveryChoice("The Sandalwood Box is missing. Choose how to continue.")
		presentation.record("recovery-choice")
	})
}

func (g *GameScene) showDoomRecoveryChoice(description string) {
	presentation := g.finalePresentation
	if presentation == nil || presentation.mode != finaleFailureScreen || !presentation.recoveryOffered {
		panic("GameScene.showDoomRecoveryChoice: missing-box finale is required")
	}
	if presentation.recoveryChoice != nil {
		g.dialogStack.MustRemoveWidgetInstance(presentation.recoveryChoice)
		presentation.recoveryChoice = nil
	}

	var choice *widgets.ButtonListModal
	choose := func(action finaleInputAction) {
		presentation.pendingInput = action
	}
	// The ending has the conversation on the left and the map on the right: the
	// choice sits inside the map, as a small window does in ordinary play, and
	// leaves the king's words readable.
	mapPlacement := g.finaleMapViewportPlacement()
	choice = widgets.NewButtonListModal(
		"Doom Recovery",
		func() { choose(finaleInputReturnToMainMenu) },
		g.keyboard,
		&mapPlacement,
	)
	choice.SetButtonWidthPercent(finaleRecoveryButtonWidth)
	choice.SetBorderWidthPercent(finaleRecoveryCardWidth)
	choice.SetDescription(description)
	choice.SetEscapeLabel("Main menu")
	choice.AddButton("Return Before Doom", func() { choose(finaleInputReturnBeforeDoom) })
	choice.AddButton("Return to Main Menu", func() { choose(finaleInputReturnToMainMenu) })
	presentation.recoveryChoice = choice
	g.dialogStack.PushModalDialog(choice)
}

// finaleMapViewportPlacement is the framed map of the ending's split screen, as
// shares of the screen.
func (g *GameScene) finaleMapViewportPlacement() sprites.PercentBasedPlacement {
	width, height := display.GetManager().GetCurrentSize()
	viewport := cinematicMapLayoutForScreen(width, height).viewport

	return sprites.PercentBasedPlacement{
		StartPercentX: float64(viewport.Min.X) / float64(width),
		EndPercentX:   float64(viewport.Max.X) / float64(width),
		StartPercentY: float64(viewport.Min.Y) / float64(height),
		EndPercentY:   float64(viewport.Max.Y) / float64(height),
	}
}

type finaleWalkTarget struct {
	id     minimap.ActorID
	target references.Position
}

func (g *GameScene) thenRunParallelFinaleWalks(
	targets []finaleWalkTarget,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	var executors []*minimap.ScriptExecutor

	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.Then(func() {
			executors = make([]*minimap.ScriptExecutor, 0, len(targets))
			for _, target := range targets {
				if g.miniMapScene.GetActor(target.id) == nil {
					continue
				}
				script := minimap.NewMiniMapScript("finale_failure_walk").
					WalkTo(target.id, target.target, finaleWalkDelayMs).WithWalkingSounds().Build()
				executors = append(executors, minimap.NewScriptExecutorWithSounds(g.miniMapScene, script, g))
			}
		}).While(func() bool {
			for _, executor := range executors {
				if !executor.IsComplete() {
					return true
				}
			}

			return false
		}, func() {
			for _, executor := range executors {
				executor.Update(16)
			}
		}, 16)
	}
}

func (g *GameScene) thenFinaleActorDeparture(
	actorID minimap.ActorID,
	gate references.Position,
	presentation *lordBritishFinalePresentation,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.ThenSequence(g.thenRunMiniMapScript(minimap.NewMiniMapScript("finale_gate_departure").
			WalkTo(actorID, gate, finaleWalkDelayMs).WithWalkingSounds().Build())).Then(func() {
			actor := g.miniMapScene.GetActor(actorID)
			if actor == nil {
				panic("thenFinaleActorDeparture: actor is unavailable")
			}
			g.miniMapScene.SetActorVisible(actorID, false)
			// REDUX DEVIATION: DNG-0015: original finale clears each actor immediately at the gate. Redux
			// gives each departure the familiar moongate fizzle requested for visual continuity.
			presentation.departureDissolve = g.StartTileDissolveWithOptions(TileDissolveOptions{
				Position: gate, SpriteIndex: actor.Sprite, Mode: visual.DissolveModeHide,
				OutlineColor: sprites.NoOutline, PositionMode: PositionModeFixedGrid,
				RenderLayer: RenderLayerDefault,
			})
		}).Animate(finaleGateDurationMs, func(progress float32) {
			g.UpdateTileDissolveProgress(presentation.departureDissolve, float64(progress))
		}).Then(func() {
			g.StopTileDissolve(presentation.departureDissolve)
			presentation.departureDissolve = 0
		}).Wait(finaleRefreshMs)
	}
}

func finalePartyActorID(index int) minimap.ActorID {
	if index == 0 {
		return minimap.ActorAvatar
	}

	return minimap.ActorID(fmt.Sprintf("party-%d", index))
}

func finaleQuestionText(message string) string {
	question, found := strings.CutSuffix(strings.TrimSpace(message), finaleReplyPrompt)
	if !found {
		panic("finaleQuestionText: source reply prompt is unavailable")
	}

	return strings.TrimSpace(question)
}

func finaleGreetingText(message, avatarName string) string {
	return strings.TrimSpace(message) + " " + avatarName + "!\""
}

func (p *lordBritishFinalePresentation) record(step string) {
	p.trace = append(p.trace, step)
}

func (g *GameScene) updateLordBritishFinaleInput() finaleInputAction {
	presentation := g.finalePresentation
	if presentation == nil {
		return finaleInputNone
	}
	if presentation.pendingInput != finaleInputNone {
		action := presentation.pendingInput
		presentation.pendingInput = finaleInputNone

		return action
	}
	if presentation.mode == finaleSuccessCertificate {
		// The certificate is the record of the whole game, so a stray key must not
		// dismiss it: only Enter does.
		if !g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) &&
			!g.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
			return finaleInputNone
		}
		// REDUX DEVIATION: DNG-0015: original final screen loops forever. Redux returns to
		// a fresh main menu after the player acknowledges the retained certificate.
		return finaleInputReturnToMainMenu
	}
	if presentation.mode == finaleFailureScreen {
		if presentation.recoveryOffered {
			return finaleInputNone
		}
		pressed := g.keyboard.GetBoundKeyPressed(&boundKeysAny)
		if pressed == nil || !g.keyboard.TryToRegisterKeyPress(*pressed) {
			return finaleInputNone
		}
		// REDUX DEVIATION: DNG-0015: original finale never polls input in its infinite failure loop.
		// Redux returns to a fresh main menu after a new key press so the application remains usable.
		return finaleInputReturnToMainMenu
	}
	if !presentation.awaitingAdvance {
		return finaleInputNone
	}
	pressed := g.keyboard.GetBoundKeyPressed(&boundKeysAny)
	if pressed == nil || !g.keyboard.TryToRegisterKeyPress(*pressed) {
		return finaleInputNone
	}
	presentation.awaitingAdvance = false
	g.keyboard.SetForceWaitAnyKey(KeyboardDebounceMs)

	return finaleInputNone
}

func (g *GameScene) clearLordBritishFinalePresentation(keepTerminal bool) {
	presentation := g.finalePresentation
	if presentation == nil {
		return
	}
	if !keepTerminal && presentation.recoveryChoice != nil {
		g.dialogStack.MustRemoveWidgetInstance(presentation.recoveryChoice)
		presentation.recoveryChoice = nil
	}
	if !keepTerminal && presentation.dialog != nil {
		g.dialogStack.RemoveWidgetInstance(presentation.dialog)
		presentation.dialog = nil
	}
	if presentation.departureDissolve != 0 {
		g.StopTileDissolve(presentation.departureDissolve)
		presentation.departureDissolve = 0
	}
	terminal := keepTerminal && (presentation.mode == finaleSuccessCertificate || presentation.mode == finaleFailureScreen)
	if !terminal || presentation.mode != finaleFailureScreen {
		g.miniMapScene = nil
	}
	g.screenFlashIntensity = 0
	g.mapFadeLevel = 0
	for _, sound := range []game_state.SoundEffect{
		game_state.SoundResurrect, game_state.SoundAbsorb,
	} {
		g.gameState.SystemCallbacks.Audio.Stop(sound).Now()
	}
	if !terminal {
		g.finalePresentation = nil
	}
}

func (g *GameScene) drawLordBritishFinaleFullScreen(screen *ebiten.Image) bool {
	presentation := g.finalePresentation
	if presentation == nil || (presentation.mode != finaleEndingArt &&
		presentation.mode != finaleSuccessCertificate) {
		return false
	}
	canvas := presentation.canvas
	canvas.Fill(image.Black)
	switch presentation.mode {
	case finaleEndingArt:
		frame := finaleArtFrames[presentation.artFrame]
		for _, placement := range frame.shapes {
			drawIntroImage(canvas, presentation.assets.Shapes[placement.archive][placement.index], placement.x, placement.y)
		}
		for _, placement := range frame.texts {
			drawIntroNarrativeText(canvas, presentation.assets.Narrative, presentation.assets.ArtText[placement.storyBlock], placement)
		}
	case finaleSuccessCertificate:
		drawIntroImage(canvas, presentation.assets.Shapes["ENDSC.16"][0], 40, 0)
		if presentation.certificateOutput != nil {
			presentation.certificateOutput.DrawPanelContent(
				canvas, image.Pt(0, text.BitmapCharHeight), canvas.Bounds().Dx(), etext.AlignCenter,
			)
		}
	case finaleRescueMap, finaleFailureScreen:
		panic("drawLordBritishFinaleFullScreen: non-fullscreen finale mode")
	}
	op := &ebiten.DrawImageOptions{}
	storyPlacement := narrativeScreenPlacement()
	sprites.ApplyDrawOptionsFromPercentsForWholeScreen(op, canvas, storyPlacement)
	screen.DrawImage(canvas, op)

	return true
}

func (g *GameScene) drawLordBritishFinaleMoongate(screen *ebiten.Image) {
	presentation := g.finalePresentation
	if presentation == nil || !presentation.showMoongate || presentation.moongateProgress <= 0 {
		return
	}
	sprite := g.spriteSheet.GetRawSprite(indexes.Moongate)
	rows := max(1, int(math.Ceil(float64(presentation.moongateProgress)*float64(sprite.Bounds().Dy()))))
	visibleRect := image.Rect(0, sprite.Bounds().Dy()-rows, sprite.Bounds().Dx(), sprite.Bounds().Dy())
	visible, ok := sprite.SubImage(
		visibleRect,
	).(*ebiten.Image)
	if !ok {
		panic("drawLordBritishFinaleMoongate: Ebitengine subimage has unexpected type")
	}
	gate := presentation.review.BoxMoongatePosition
	screenTileX, screenTileY := minimap.GridToScreenTile(gate, g.miniMapScene.GetCameraCenter(), 11)
	tileX := screenTileX + (xTilesVisibleOnGameScreen-11)/2
	tileY := screenTileY + (yTilesVisibleOnGameScreen-11)/2
	x, y := sprites.ConvertPosition16to18(tileX, tileY)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(x, y+float64(sprite.Bounds().Dy()-rows))
	mapOp := &ebiten.DrawImageOptions{}
	sprites.ApplyDrawOptionsFromPercentsForWholeScreen(mapOp, g.mapImage, g.mapScreenPlacement())
	op.GeoM.Concat(mapOp.GeoM)
	// REDUX DEVIATION: DNG-0015: original finale draws the standard blue moongate tile. Redux swaps
	// its blue palette entries to red to distinguish this singular finale portal.
	screen.DrawRectShader(visible.Bounds().Dx(), visible.Bounds().Dy(), getFinaleMoongateShader(),
		&ebiten.DrawRectShaderOptions{
			GeoM:   op.GeoM,
			Images: [4]*ebiten.Image{visible, nil, nil, nil},
		})
}

func (g *GameScene) newFinaleCertificateOutput() *text.Output {
	date, ok := g.gameState.LordBritishFinaleCompletionDate()
	if !ok {
		panic("newFinaleCertificateOutput: completion date is unavailable")
	}
	avatar := g.gameState.PartyState.GetAvatar().GetNameAsString()
	years, months, days := int(date.Year)-139, int(date.Month)-4, int(date.Day)-5
	if days < 0 {
		days += 28
		months--
	}
	if months < 0 {
		months += 13
		years--
	}
	duration := finaleDurationText(years, months, days)
	lines := []string{
		"Be it known that on", "the " + finaleOrdinal(int(date.Day)) + " Day of",
		"the " + finaleOrdinal(int(date.Month)) + " Month", "of the Year",
		finaleCardinal(int(date.Year)/100) + " Hundred", finaleCardinal(int(date.Year) % 100), "",
		avatar + " the Avatar", "", "saved the life", "of our sovereign",
		"Lord British, thereby", "saving our people", "and our land.", "",
		"THE QUEST OF THE AVATAR", "IS FOREVER", "", "", "", "Report now, thy Quest compleat in",
		duration, "to Lord British at Origin Systems!",
	}
	output := text.NewOutput(
		text.NewUltimaFont(text.BitmapCharHeight),
		text.BitmapCharHeight,
		len(lines),
		320/text.BitmapCharWidth,
	)
	for index, line := range lines {
		builder := output.AppendLine()
		switch {
		case index >= 15 && index <= 19:
			builder.RunicWithColor(line, ucolor.EGABlack)
		case index <= 19:
			builder.IBMWithColor(line, ucolor.EGABlack)
		default:
			builder.IBMWithColor(line, ucolor.EGAWhite)
		}
		builder.Done()
	}

	return output
}

func finaleDurationText(years, months, days int) string {
	parts := make([]string, 0, 3)
	for _, value := range []struct {
		n    int
		unit string
	}{{years, "year"}, {months, "month"}, {days, "day"}} {
		if value.n == 0 {
			continue
		}
		unit := value.unit
		if value.n > 1 {
			unit += "s"
		}
		parts = append(parts, fmt.Sprintf("%d %s", value.n, unit))
	}

	return strings.Join(parts, ", ")
}

func finaleCardinal(number int) string {
	ones := []string{"", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten", "Eleven", "Twelve", "Thirteen", "Fourteen", "Fifteen", "Sixteen", "Seventeen", "Eighteen", "Nineteen"}
	tens := []string{"", "", "Twenty", "Thirty", "Forty", "Fifty", "Sixty", "Seventy", "Eighty", "Ninety"}
	if number >= 0 && number < len(ones) {
		return ones[number]
	}
	if number >= 20 && number < 100 {
		if number%10 == 0 {
			return tens[number/10]
		}

		return tens[number/10] + "-" + ones[number%10]
	}

	return strconv.Itoa(number)
}

func finaleOrdinal(number int) string {
	ordinals := []string{"", "First", "Second", "Third", "Fourth", "Fifth", "Sixth", "Seventh", "Eighth", "Ninth", "Tenth", "Eleventh", "Twelfth"}
	if number > 0 && number < len(ordinals) {
		return ordinals[number]
	}
	if number < 20 {
		return finaleCardinal(number) + "th"
	}
	if number == 20 {
		return "Twentieth"
	}

	return "Twenty-" + ordinals[number-20]
}
