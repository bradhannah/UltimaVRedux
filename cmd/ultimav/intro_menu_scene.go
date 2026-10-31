//nolint:mnd // Verified original layout coordinates, timing, and presentation proportions.
package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"math"
	"path/filepath"
	"strings"

	beradinput "github.com/bradhannah/berad-sdk/modules/berad-input"
	"github.com/hajimehoshi/ebiten/v2"
	textv2 "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/legacy_save"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/originaldata"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/internal/visual"
	"github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	rng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

const (
	introLogicalWidth          = 320
	introLogicalHeight         = 200
	introUpdateInterval        = 16
	introWorkflowName          = "intro_presentation"
	introChoiceJourney         = 0
	introChoiceLoad            = 1
	introChoiceNewGame         = 2
	introChoiceConfigure       = 3
	introChoiceNarrative       = 4
	introChoiceAcknowledgments = 5
	introChoiceReturnView      = 6
	introNewGameCreate         = 0
	introNewGameImport         = 1
	introConfigureDisplay      = 0
	introConfigureAudio        = 1
	introConfigureGameplay     = 2
	introConfigureData         = 3
	introNoticeLineCount       = 2
	introOriginFullY           = 45
	introTitleFizzHeight       = 100
	introFlameX                = 16
	introFlameY                = 70
	introTitleFizzSeed         = 0x554C54494D41
	introFlameFizzSeed         = 0x464C414D4553
	narrativeScreenPadding     = 0.02
)

var errIntroConfigurationRequired = errors.New("intro configuration and saved data are required")

type IntroMenuScene struct {
	navigation          input.ListNavigation
	introSprites        *sprites.IntroSprites
	borderSprites       *sprites.BorderSprites
	ultimaFont          *text.UltimaFont
	menuRenderer        *text.IntroMenuRenderer
	noticeFont          *text.UltimaFont
	noticeOutput        *text.Output
	versionOutput       *text.Output
	keyboard            *input.Keyboard
	config              *config.UltimaVConfiguration
	audioManager        *audio.GameAudioManager
	eventQueueManager   *event_queue.EventQueueManager
	dialogStack         widgets.DialogStack
	presentation        *introPresentationDriver
	narrative           *introNarrativePresentation
	narrativeAssets     *sprites.NarrativeIntroAssets
	returnView          *returnViewPresentation
	returnViewData      *minimap.ReturnViewData
	presentationCanvas  *ebiten.Image
	signatureCanvas     *ebiten.Image
	titleFizzSource     *ebiten.Image
	titleFizzBuffer     *ebiten.Image
	flameFizzBuffer     *ebiten.Image
	titleFizz           *visual.PixelDissolveEffect
	flameFizz           *visual.PixelDissolveEffect
	signaturePointCount int

	elapsedMs               int64
	flameAnimationStartMs   int64
	menuIdleStartedMs       int64
	nCurrentSelection       int
	submenuOpen             bool
	submenuSelection        int
	notice                  string
	showVersion             bool
	journeyOnwardEnabled    bool
	saveAvailabilityKnown   bool
	menuVisible             bool
	active                  bool
	disposed                bool
	startupSkipped          bool
	legacyPrepareResults    chan introLegacyPrepareResult
	legacyPrepareCancel     context.CancelFunc
	legacyPrepareGeneration uint64
	legacyPrepareActive     bool
	legacyPrepareOnFailure  func(error)
	legacyPrepareOnCancel   func()
	prepareLegacySave       func(context.Context, string) (preparedLegacyPlayerSave, error)
}

type introLegacyPrepareResult struct {
	generation uint64
	prepared   preparedLegacyPlayerSave
	err        error
}

// CreateIntroMenuScene creates the title scene from original graphics in the configured data directory.
func CreateIntroMenuScene(gameConfig *config.UltimaVConfiguration) (*IntroMenuScene, error) {
	return createIntroMenuScene(gameConfig, false)
}

// CreateMainMenuScene creates a fresh title scene with its menu immediately available.
func CreateMainMenuScene(gameConfig *config.UltimaVConfiguration) (*IntroMenuScene, error) {
	return createIntroMenuScene(gameConfig, true)
}

func createIntroMenuScene(gameConfig *config.UltimaVConfiguration, startAtMenu bool) (*IntroMenuScene, error) {
	if gameConfig == nil || gameConfig.SavedConfigData == nil {
		return nil, fmt.Errorf("creating intro menu: %w", errIntroConfigurationRequired)
	}
	introSprites, err := sprites.LoadIntroSprites(gameConfig.SavedConfigData.DataFilePath)
	if err != nil {
		return nil, fmt.Errorf("creating intro menu: %w", err)
	}
	font := text.NewUltimaFont(text.GetIntroFontSize())
	intro := newIntroMenuState(gameConfig, introSprites, font)
	intro.presentation = newIntroPresentationDriver(introSprites.BritishPaths)
	intro.titleFizzSource = ebiten.NewImage(introLogicalWidth, introTitleFizzHeight)
	intro.titleFizzSource.DrawImage(introSprites.Ultima16Logo, nil)
	intro.titleFizzBuffer = ebiten.NewImage(introLogicalWidth, introTitleFizzHeight)
	intro.titleFizz = visual.NewPixelDissolveEffect(
		introLogicalWidth, introTitleFizzHeight, introLogicalWidth, introTitleFizzHeight,
		visual.DissolveModeReveal,
		rng.New(introTitleFizzSeed),
	)
	flameSize := introSprites.FlameFizzBase.Bounds().Size()
	intro.flameFizzBuffer = ebiten.NewImage(flameSize.X, flameSize.Y)
	intro.flameFizz = visual.NewPixelDissolveEffect(
		flameSize.X, flameSize.Y, flameSize.X, flameSize.Y,
		visual.DissolveModeReveal,
		rng.New(introFlameFizzSeed),
	)
	intro.eventQueueManager = event_queue.NewEventQueueManager(func() int64 { return intro.elapsedMs }, nil)
	if startAtMenu {
		intro.presentation.Skip()
		intro.showMenu()
	} else {
		startIntroPresentationWorkflow(intro.eventQueueManager, intro.presentation, intro.finishStartupPresentation)
	}

	if !gameConfig.SoundDisabled {
		intro.audioManager, err = audio.NewGameAudioManagerWithConfig(gameConfig.AudioConfiguration)
		if err != nil {
			logging.Warnf(logging.CategoryAudio, "Failed to initialize audio system in intro: %v", err)
			intro.audioManager = nil
		}
	}

	return intro, nil
}

func newIntroMenuState(
	gameConfig *config.UltimaVConfiguration,
	introSprites *sprites.IntroSprites,
	font *text.UltimaFont,
) *IntroMenuScene {
	var navigation input.ListNavigation
	noticeFont := text.NewUltimaFont(text.GetIntroNoticeFontSize())
	noticeOutput := text.NewOutput(
		noticeFont, noticeFont.TextFace.Metrics().HAscent*1.2, introNoticeLineCount, 80,
	)

	return &IntroMenuScene{
		navigation:              navigation,
		introSprites:            introSprites,
		borderSprites:           sprites.NewBorderSprites(),
		ultimaFont:              font,
		menuRenderer:            text.NewIntroMenuRenderer(font),
		noticeFont:              noticeFont,
		noticeOutput:            noticeOutput,
		versionOutput:           newIntroVersionOutput(),
		keyboard:                input.NewKeyboard(250),
		config:                  gameConfig,
		audioManager:            nil,
		eventQueueManager:       nil,
		dialogStack:             widgets.DialogStack{},
		presentation:            nil,
		narrative:               nil,
		narrativeAssets:         nil,
		returnView:              nil,
		returnViewData:          nil,
		presentationCanvas:      ebiten.NewImage(introLogicalWidth, introLogicalHeight),
		signatureCanvas:         ebiten.NewImage(introLogicalWidth, introLogicalHeight),
		titleFizzSource:         nil,
		titleFizzBuffer:         nil,
		flameFizzBuffer:         nil,
		titleFizz:               nil,
		flameFizz:               nil,
		signaturePointCount:     0,
		elapsedMs:               0,
		flameAnimationStartMs:   0,
		menuIdleStartedMs:       0,
		nCurrentSelection:       introChoiceLoad,
		submenuOpen:             false,
		submenuSelection:        0,
		notice:                  "",
		showVersion:             false,
		journeyOnwardEnabled:    false,
		saveAvailabilityKnown:   false,
		menuVisible:             false,
		active:                  false,
		disposed:                false,
		startupSkipped:          false,
		legacyPrepareResults:    make(chan introLegacyPrepareResult, 1),
		legacyPrepareCancel:     nil,
		legacyPrepareGeneration: 0,
		legacyPrepareActive:     false,
		legacyPrepareOnFailure:  nil,
		legacyPrepareOnCancel:   nil,
		prepareLegacySave:       nil,
	}
}

func (m *IntroMenuScene) finishStartupPresentation() {
	if m.startupSkipped {
		m.showMenu()

		return
	}
	m.startReturnToView()
}

func (m *IntroMenuScene) Activate() {
	if m.disposed {
		panic("IntroMenuScene.Activate: disposed scene cannot be activated")
	}
	if m.active {
		return
	}
	m.active = true
	if m.audioManager != nil {
		if err := m.audioManager.PlayMusicForContext("title"); err != nil {
			logging.Warnf(logging.CategoryAudio, "Failed to start title music: %v", err)
		}
	}
}

func (m *IntroMenuScene) PrepareForSceneReplacement() {
	m.stopLegacySavePreparation()
	if m.audioManager == nil {
		return
	}
	if err := m.audioManager.StopBackgroundMusic(); err != nil {
		logging.Warnf(logging.CategoryAudio, "Failed to stop intro music: %v", err)
	}
}

func (m *IntroMenuScene) Dispose() {
	if m.disposed {
		return
	}
	m.disposed = true
	m.active = false
	m.stopLegacySavePreparation()
	if m.audioManager == nil {
		return
	}
	if err := m.audioManager.Close(); err != nil {
		logging.Warnf(logging.CategoryAudio, "Failed to close intro audio: %v", err)
	}
	m.audioManager = nil
}

func (m *IntroMenuScene) InvalidateResolution() {
	m.ultimaFont = text.NewUltimaFont(text.GetIntroFontSize())
	m.menuRenderer = text.NewIntroMenuRenderer(m.ultimaFont)
	m.noticeFont = text.NewUltimaFont(text.GetIntroNoticeFontSize())
	m.noticeOutput.SetFont(m.noticeFont, m.noticeFont.TextFace.Metrics().HAscent*1.2)
	versionFont := newIntroVersionFont()
	m.versionOutput.SetFont(versionFont, versionFont.TextFace.Metrics().HAscent*1.2)
}

func (m *IntroMenuScene) GetUltimaConfiguration() *config.UltimaVConfiguration {
	return m.config
}

func (m *IntroMenuScene) PrepareInputForSceneActivation() {
	m.keyboard.PrepareForTransition(0)
}

//nolint:gochecknoglobals // Fixed input map.
var boundKeysIntro = beradinput.NewBindings(
	ebiten.KeyDown, ebiten.KeyUp, ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeyEscape, ebiten.KeyV,
)

//nolint:gochecknoglobals // All-key presentations use the keyboard adapter; Caps Lock is not actionable input.
var boundKeysAny = func() []ebiten.Key {
	keys := make([]ebiten.Key, 0, int(ebiten.KeyMax-ebiten.KeyA)+1)
	for key := ebiten.KeyA; key <= ebiten.KeyMax; key++ {
		keys = append(keys, key)
	}

	return keys
}()

func (m *IntroMenuScene) Update(game *Game) error {
	if m.audioManager != nil {
		m.audioManager.Update()
	}
	m.elapsedMs += introUpdateInterval
	m.keyboard.ClearFrameConsumption()
	if m.legacyPrepareActive {
		m.updateLegacySavePreparation(game)

		return nil
	}
	if modal := m.dialogStack.PeekTopModalDialog(); modal != nil {
		m.navigation.BlockUntilRelease(m.keyboard.IsKeyHeld)
		m.menuIdleStartedMs = m.elapsedMs
		modal.Update()

		return nil
	}
	if !m.menuVisible {
		m.navigation.BlockUntilRelease(m.keyboard.IsKeyHeld)
		m.updatePresentationInput()
		m.eventQueueManager.Update()

		return nil
	}

	if m.keyboard.GetBoundKeyPressed(&boundKeysAny) != nil {
		m.menuIdleStartedMs = m.elapsedMs
	}
	if m.updateMenuNavigation() {
		return nil
	}
	pressedKey := m.keyboard.GetRegisteredKeyPressed(boundKeysIntro)
	if pressedKey == nil {
		m.keyboard.SetAllowKeyPressImmediately()
		if m.elapsedMs-m.menuIdleStartedMs >= 10_000 {
			m.startReturnToView()
		}

		return nil
	}
	m.menuIdleStartedMs = m.elapsedMs
	if m.keyboard.TryToRegisterKeyPress(*pressedKey) {
		m.handleMenuKey(game, *pressedKey)
	}

	return nil
}

// handleMenuKey runs one registered title-menu key. Escape backs out of an
// open sub-menu before it offers to exit the game.
func (m *IntroMenuScene) handleMenuKey(game *Game, key ebiten.Key) {
	switch key { //nolint:exhaustive // Only the bound title keys reach this switch.
	case ebiten.KeyEscape:
		if m.submenuOpen {
			m.closeSubmenu()

			return
		}
		m.showExitConfirmation(game)
	case ebiten.KeyV:
		m.showVersion = !m.showVersion
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		m.activateSelection(game)
	}
}

// showExitConfirmation gives keyboard-only players a way out of the title menu.
func (m *IntroMenuScene) showExitConfirmation(game *Game) {
	var menu *widgets.ButtonListModal
	cancel := func() { m.dialogStack.MustRemoveWidgetInstance(menu) }
	placement := sprites.PercentBasedPlacement{StartPercentX: 0, EndPercentX: 1, StartPercentY: 0, EndPercentY: 1}
	menu = widgets.NewButtonListModal("Exit Ultima V Redux?", cancel, m.keyboard, &placement)
	menu.AddButton("Exit", game.requestExit)
	menu.AddButton("Cancel", cancel)
	m.dialogStack.PushModalDialog(menu)
}

func (m *IntroMenuScene) updateMenuNavigation() bool {
	key, owned := m.navigation.Poll(m.keyboard, ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyLeft, ebiten.KeyRight)
	if owned && key != nil {
		switch *key { //nolint:exhaustive // The compact intro menu has no paging action.
		case ebiten.KeyUp:
			// REDUX DEVIATION: INTRO-0001: original highlight wraps; Redux clamps by product decision.
			m.moveIntroSelection(-1)
			m.notice = ""
		case ebiten.KeyDown:
			m.moveIntroSelection(1)
			m.notice = ""
		case ebiten.KeyRight:
			// Right opens the selected sub-menu, mirroring its chevron.
			if !m.submenuOpen && m.selectedChoiceHasChildren() {
				m.openSubmenu()
			}
		case ebiten.KeyLeft:
			if m.submenuOpen {
				m.closeSubmenu()
			}
		}
	}

	return owned
}

func (m *IntroMenuScene) selectedChoiceHasChildren() bool {
	return m.introChoiceEnabled(m.nCurrentSelection) && len(text.IntroChoices[m.nCurrentSelection].Children) > 0
}

// openSubmenu moves the cursor into the selected choice's children; the
// parent stays lit so the player can see which door they walked through.
func (m *IntroMenuScene) openSubmenu() {
	m.submenuOpen = true
	m.submenuSelection = 0
	m.notice = ""
}

func (m *IntroMenuScene) closeSubmenu() {
	m.submenuOpen = false
	m.submenuSelection = 0
	m.notice = ""
}

func (m *IntroMenuScene) introMenuState() text.IntroMenuState {
	enabledChoices := make([]bool, len(text.IntroChoices))
	for choice := range enabledChoices {
		enabledChoices[choice] = m.introChoiceEnabled(choice)
	}

	return text.IntroMenuState{
		Selection:        m.nCurrentSelection,
		Enabled:          enabledChoices,
		SubmenuOpen:      m.submenuOpen,
		SubmenuSelection: m.submenuSelection,
	}
}

func (m *IntroMenuScene) updatePresentationInput() {
	if m.returnView != nil {
		m.updateReturnViewInput()

		return
	}
	if m.narrative != nil {
		m.updateNarrativeInput()

		return
	}
	pressedKey := m.keyboard.GetRegisteredKeyPressed(boundKeysIntro)
	if pressedKey == nil || *pressedKey != ebiten.KeyEnter {
		return
	}
	if m.keyboard.TryToRegisterKeyPress(*pressedKey) {
		// REDUX DEVIATION: INTRO-0003: original presentation accepts any key to interrupt.
		// Redux requires Enter so incidental keys do not skip the startup sequence.
		m.startupSkipped = true
		m.presentation.Skip()
	}
}

func (m *IntroMenuScene) updateReturnViewInput() {
	pressedKey := m.keyboard.GetBoundKeyPressed(&boundKeysAny)
	if pressedKey == nil || !m.keyboard.TryToRegisterKeyPress(*pressedKey) {
		return
	}
	m.returnView.executor.Interrupt()
}

func (m *IntroMenuScene) updateNarrativeInput() {
	m.narrative.syncFrame(m.elapsedMs)
	if !m.narrative.executor.IsWaitingForAdvance() {
		return
	}
	pressedKey := m.keyboard.GetBoundKeyPressed(&boundKeysAny)
	if pressedKey == nil {
		return
	}
	if !m.narrative.canAdvance(m.elapsedMs) {
		// The original clears input collected while the frame-2 fizz runs.
		m.keyboard.TryToRegisterKeyPress(*pressedKey)

		return
	}
	if !m.keyboard.TryToRegisterKeyPress(*pressedKey) {
		return
	}
	m.narrative.executor.Advance()
	m.narrative.syncFrame(m.elapsedMs)
}

func (m *IntroMenuScene) activateSelection(game *Game) {
	if !m.introChoiceEnabled(m.nCurrentSelection) {
		return
	}
	if m.submenuOpen {
		m.activateSubmenuSelection(game)

		return
	}
	if m.selectedChoiceHasChildren() {
		m.openSubmenu()

		return
	}
	switch m.nCurrentSelection {
	case introChoiceJourney:
		m.continueLatestPlayerSave(game)
	case introChoiceLoad:
		m.openPlayerSaveBrowser(game, playerSaveBrowserLoad, nil)
	case introChoiceNarrative:
		m.startNarrativeIntroduction()
	case introChoiceAcknowledgments:
		m.openAcknowledgments()
	case introChoiceReturnView:
		m.startReturnToView()
	default:
		m.notice = text.IntroChoices[m.nCurrentSelection].Label + " is not yet available."
	}
}

// activateSubmenuSelection dispatches the child under the cursor. The
// sub-menu stays open afterwards so a returning player lands where they left.
func (m *IntroMenuScene) activateSubmenuSelection(game *Game) {
	switch {
	case m.nCurrentSelection == introChoiceNewGame && m.submenuSelection == introNewGameCreate:
		m.startCharacterCreation(game)
	case m.nCurrentSelection == introChoiceNewGame && m.submenuSelection == introNewGameImport:
		m.startActiveLegacySaveImport(game)
	case m.nCurrentSelection == introChoiceConfigure && m.submenuSelection == introConfigureDisplay:
		m.openConfigurationDialogForTab(audioSettingsDisplayTab)
	case m.nCurrentSelection == introChoiceConfigure && m.submenuSelection == introConfigureAudio:
		m.openConfigurationDialogForTab(audioSettingsAudioTab)
	case m.nCurrentSelection == introChoiceConfigure && m.submenuSelection == introConfigureGameplay:
		m.openConfigurationDialogForTab(audioSettingsGameplayTab)
	case m.nCurrentSelection == introChoiceConfigure && m.submenuSelection == introConfigureData:
		m.startDataDirectorySetup(game)
	default:
		children := text.IntroChoices[m.nCurrentSelection].Children
		m.notice = children[m.submenuSelection] + " is not yet available."
	}
}

func (m *IntroMenuScene) startDataDirectorySetup(game *Game) {
	host, err := originaldata.CurrentHostProfile()
	if err != nil {
		m.notice = "Data-directory setup could not inspect this computer."
		logging.Warnf(logging.CategoryUI, "Data-directory host setup failed: %v", err)

		return
	}
	activeConfiguration := m.config
	setup := newDataSetupScene(
		activeConfiguration,
		nil,
		nil,
		host,
		dataSetupFromTitle,
		activeConfiguration.SoundDisabled,
		nil,
		func(game *Game) error {
			return game.ReplaceScene(func() (Scene, error) {
				return CreateMainMenuScene(activeConfiguration)
			})
		},
	)
	if err := game.ReplaceScene(func() (Scene, error) { return setup, nil }); err != nil {
		m.notice = "Data-directory setup could not be started."
		logging.Warnf(logging.CategoryUI, "Data-directory scene replacement failed: %v", err)
	}
}

func (m *IntroMenuScene) startCharacterCreation(game *Game) {
	if err := game.startCharacterCreation(); err != nil {
		m.notice = "Character creation could not be started."
		logging.Warnf(logging.CategoryUI, "Character creation startup failed: %v", err)
	}
}

// openConfigurationDialogForTab opens Configuration on the chosen tab while the
// title hands its audio ownership to the dialog.
func (m *IntroMenuScene) openConfigurationDialogForTab(tab int) {
	m.notice = ""
	if m.audioManager != nil {
		m.audioManager.GetMusicManager().SetSuppressed(audio.MusicMenu, true)
		m.audioManager.SetEnabled(false)
	}
	var configurationAudioManager ConfigurationAudioManager
	if m.audioManager != nil {
		configurationAudioManager = m.audioManager
	}
	placement := &sprites.PercentBasedPlacement{
		StartPercentX: 0,
		EndPercentX:   1,
		StartPercentY: 0,
		EndPercentY:   1,
	}
	var configurationDialog *ConfigurationDialog
	onDismiss := func() {
		m.dialogStack.MustRemoveWidgetInstance(configurationDialog)
		m.keyboard.PrepareForTransition(0)
		if m.audioManager != nil {
			m.audioManager.SetEnabled(true)
			m.audioManager.GetMusicManager().SetSuppressed(audio.MusicMenu, false)
			m.resumeTitleMusic("Configuration")
		}
	}
	configurationDialog = newConfigurationDialogForTab(
		placement,
		m.keyboard,
		onDismiss,
		m.config,
		configurationAudioManager,
		tab,
	)
	m.dialogStack.PushModalDialog(configurationDialog)
}

func (m *IntroMenuScene) characterCreationIdentityPresentation() *characterCreationIdentityPresentation {
	return newCharacterCreationIdentityPresentation(m.introSprites, m.elapsedMs-m.flameAnimationStartMs)
}

func (m *IntroMenuScene) showMenu() {
	m.menuVisible = true
	m.flameAnimationStartMs = m.elapsedMs
	m.menuIdleStartedMs = m.elapsedMs
	m.keyboard.PrepareForTransition(0)
}

func (m *IntroMenuScene) startReturnToView() {
	if m.returnViewData == nil {
		data, err := minimap.LoadReturnViewData(m.config.SavedConfigData.DataFilePath)
		if err != nil {
			m.showMenu()
			m.notice = "Return to the View could not be loaded."
			logging.Warnf(logging.CategoryUI, "Return to the View loading failed: %v", err)

			return
		}
		m.returnViewData = data
	}
	presentation, err := newReturnViewPresentation(m.returnViewData)
	if err != nil {
		m.showMenu()
		m.notice = "Return to the View could not be started."
		logging.Warnf(logging.CategoryUI, "Return to the View setup failed: %v", err)

		return
	}
	m.notice = ""
	m.menuVisible = false
	m.returnView = presentation
	m.keyboard.PrepareForTransition(0)
	m.pauseTitleMusic()
	m.eventQueueManager.Workflow(returnViewWorkflowName).
		Priority(event_queue.PriorityLow).
		Sequential().
		First(func() {}).
		RunScript(m.returnView.executor).
		Finally(func() {
			m.returnView = nil
			m.showMenu()
			m.resumeTitleMusic("Return to the View")
		}).
		Start()
}

func (m *IntroMenuScene) pauseTitleMusic() {
	if m.audioManager == nil {
		return
	}
	m.audioManager.GetMusicManager().SetSuppressed(audio.MusicCinematic, true)
}

func (m *IntroMenuScene) resumeTitleMusic(context string) {
	if m.audioManager == nil {
		return
	}
	m.audioManager.GetMusicManager().SetSuppressed(audio.MusicCinematic, false)
	if err := m.audioManager.PlayMusicForContext("title"); err != nil {
		logging.Warnf(logging.CategoryAudio, "Failed to resume title music after %s: %v", context, err)
	}
}

func (m *IntroMenuScene) startNarrativeIntroduction() {
	if m.narrativeAssets == nil {
		assets, err := sprites.LoadNarrativeIntroAssets(m.config.SavedConfigData.DataFilePath)
		if err != nil {
			m.notice = "Introduction could not be loaded."
			logging.Warnf(logging.CategoryUI, "Narrative Introduction loading failed: %v", err)

			return
		}
		m.narrativeAssets = assets
	}
	m.notice = ""
	m.menuVisible = false
	m.narrative = newIntroNarrativePresentation(m.narrativeAssets)
	m.keyboard.PrepareForTransition(0)
	m.pauseTitleMusic()
	m.eventQueueManager.Workflow(introNarrativeWorkflowName).
		Priority(event_queue.PriorityLow).
		Sequential().
		First(func() {}).
		RunScript(m.narrative.executor).
		Finally(func() {
			m.narrative = nil
			m.showMenu()
			m.resumeTitleMusic("Introduction")
		}).
		Start()
}

func (m *IntroMenuScene) applyPlayerSaveCatalog(summaries []player_save.SlotSummary) {
	wasKnown := m.saveAvailabilityKnown
	m.saveAvailabilityKnown = true
	m.journeyOnwardEnabled = false
	for _, summary := range summaries {
		if summary.Status == player_save.SlotReady {
			m.journeyOnwardEnabled = true

			break
		}
	}
	if !m.journeyOnwardEnabled && m.nCurrentSelection == introChoiceJourney {
		m.nCurrentSelection = introChoiceLoad

		return
	}
	if !wasKnown && m.journeyOnwardEnabled && m.nCurrentSelection == introChoiceLoad {
		m.nCurrentSelection = introChoiceJourney
	}
}

func (m *IntroMenuScene) introChoiceEnabled(choice int) bool {
	return choice != introChoiceJourney || m.journeyOnwardEnabled
}

func (m *IntroMenuScene) moveIntroSelection(direction int) {
	if m.submenuOpen {
		children := text.IntroChoices[m.nCurrentSelection].Children
		m.submenuSelection = min(max(m.submenuSelection+direction, 0), len(children)-1)

		return
	}
	for next := m.nCurrentSelection + direction; next >= 0 && next < len(text.IntroChoices); next += direction {
		if m.introChoiceEnabled(next) {
			m.nCurrentSelection = next

			return
		}
	}
}

func (m *IntroMenuScene) continueLatestPlayerSave(game *Game) {
	err := game.loadLatestPlayerSave()
	if errors.Is(err, player_save.ErrSlotNotFound) {
		m.openPlayerSaveBrowser(game, playerSaveBrowserLoad, nil)

		return
	}
	if err != nil {
		m.reportPlayerSaveError("Load", err)
	}
}

func (m *IntroMenuScene) startActiveLegacySaveImport(game *Game) {
	m.stopLegacySavePreparation()
	directory := m.config.SavedConfigData.DataFilePath
	if strings.TrimSpace(directory) == "" {
		m.showLegacySaveFailure(game, "(unset)", config.ErrDataFilePathUnset)

		return
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		m.showLegacySaveFailure(game, directory, err)

		return
	}
	m.prepareActiveLegacySave(game, absolute)
}

func (m *IntroMenuScene) prepareActiveLegacySave(game *Game, directory string) {
	m.startLegacySavePreparation(game, directory,
		func(err error) { m.showLegacySaveFailure(game, directory, err) },
		func() {
			m.notice = "Original-save check cancelled. No files or settings were changed."
			m.menuIdleStartedMs = m.elapsedMs
		})
}

func (m *IntroMenuScene) showLegacySaveFailure(game *Game, directory string, err error) {
	m.notice = ""
	var menu *widgets.ButtonListModal
	back := func() {
		m.dialogStack.MustRemoveWidgetInstance(menu)
		m.stopLegacySavePreparation()
		m.menuIdleStartedMs = m.elapsedMs
	}
	placement := sprites.PercentBasedPlacement{StartPercentX: 0, EndPercentX: 1, StartPercentY: 0, EndPercentY: 1}
	menu = widgets.NewButtonListModal("Original Save Unavailable", back, m.keyboard, &placement)
	menu.SetDescriptionRowCapacity(setupDescriptionRows)
	menu.SetDescription("Active game folder: " + directory + "\n" + legacySavePreparationMessage(err) + "\n" + err.Error())
	menu.SetEscapeLabel(setupBackLabel)
	menu.AddButton("Check Again", func() {
		back()
		m.startActiveLegacySaveImport(game)
	})
	menu.AddButton(setupBackLabel, back)
	m.dialogStack.PushModalDialog(menu)
}

func (m *IntroMenuScene) startLegacySavePreparation(
	game *Game,
	selectedPath string,
	onFailure func(error),
	onCancel func(),
) {
	m.stopLegacySavePreparation()
	m.legacyPrepareGeneration++
	generation := m.legacyPrepareGeneration
	ctx, cancel := context.WithCancel(context.Background())
	m.legacyPrepareCancel = cancel
	m.legacyPrepareActive = true
	m.legacyPrepareOnFailure = onFailure
	m.legacyPrepareOnCancel = onCancel
	m.notice = "Checking the original save in the active game folder..."
	results := make(chan introLegacyPrepareResult, 1)
	m.legacyPrepareResults = results
	prepare := m.prepareLegacySave
	if prepare == nil {
		prepare = game.prepareLegacyPlayerSaveContext
	}
	go func() {
		resolved, err := legacy_save.ResolveSavedGamePathContext(ctx, selectedPath, "")
		var prepared preparedLegacyPlayerSave
		if err == nil {
			prepared, err = prepare(ctx, resolved)
			prepared.sourceDirectory = selectedPath
		}
		select {
		case results <- introLegacyPrepareResult{generation: generation, prepared: prepared, err: err}:
		case <-ctx.Done():
		}
	}()
}

func (m *IntroMenuScene) updateLegacySavePreparation(game *Game) {
	if m.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		onCancel := m.legacyPrepareOnCancel
		m.stopLegacySavePreparation()
		onCancel()

		return
	}
	select {
	case result := <-m.legacyPrepareResults:
		if result.generation == m.legacyPrepareGeneration {
			onFailure := m.legacyPrepareOnFailure
			m.stopLegacySavePreparation()
			if result.err != nil {
				onFailure(result.err)
			} else {
				m.openPlayerSaveBrowser(game, playerSaveBrowserWrite, &result.prepared)
			}
		}
	default:
	}
}

func (m *IntroMenuScene) stopLegacySavePreparation() {
	m.legacyPrepareGeneration++
	m.menuIdleStartedMs = m.elapsedMs
	if m.legacyPrepareCancel != nil {
		m.legacyPrepareCancel()
		m.legacyPrepareCancel = nil
	}
	m.legacyPrepareActive = false
	m.legacyPrepareOnFailure = nil
	m.legacyPrepareOnCancel = nil
}

func legacySavePreparationMessage(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "SAVED.GAM could not be found."
	case errors.Is(err, fs.ErrPermission):
		return "SAVED.GAM or SAVED.OOL could not be read due to file permissions."
	}

	var sourceValidation *legacy_save.ValidationError
	if errors.As(err, &sourceValidation) && len(sourceValidation.Issues) > 0 {
		return legacySaveSourceValidationMessage(sourceValidation.Issues[0])
	}

	var snapshotValidation *save_snapshot.ValidationError
	if errors.As(err, &snapshotValidation) && len(snapshotValidation.Issues) > 0 {
		return "Legacy save state is unsupported: " + snapshotValidation.Issues[0].Message
	}

	return "That legacy save could not be read."
}

func legacySaveSourceValidationMessage(issue legacy_save.ValidationIssue) string {
	switch issue.Code {
	case "invalid_filename":
		return "The active game location must be a folder containing SAVED.GAM and SAVED.OOL."
	case "missing_sibling":
		return "SAVED.OOL is missing beside SAVED.GAM in the active game folder."
	case "missing_file":
		return "SAVED.GAM was not found in that folder."
	case "ambiguous_sibling":
		if issue.Path == "gam" {
			return "Multiple SAVED.GAM files were found in that folder."
		}

		return "Multiple SAVED.OOL files were found beside SAVED.GAM."
	case "invalid_length":
		return issue.Message
	default:
		return "Legacy save data is invalid: " + issue.Message
	}
}

func (m *IntroMenuScene) openPlayerSaveBrowser(
	game *Game,
	mode playerSaveBrowserMode,
	legacySave *preparedLegacyPlayerSave,
) {
	m.openPlayerSaveBrowserWithState(game, mode, legacySave, "", "")
}

func (m *IntroMenuScene) openPlayerSaveBrowserWithState(
	game *Game,
	mode playerSaveBrowserMode,
	legacySave *preparedLegacyPlayerSave,
	selectionID string,
	notice string,
) {
	summaries, err := game.listPlayerSaveSlots()
	if err != nil {
		m.reportPlayerSaveError("List saves", err)

		return
	}

	model := buildIntroSaveBrowserModel(summaries, mode, legacySave, selectionID)
	title, action := "Select Save Game", "Load"
	if mode == playerSaveBrowserWrite {
		title, action = "Import Legacy Save", "Import"
	}
	var browser *widgets.SaveSlotBrowser
	browser = widgets.NewSaveSlotBrowser(model.views, widgets.SaveSlotBrowserConfig{
		Title:                    title,
		PrimaryActionLabel:       action,
		CancelActionLabel:        "",
		ReviewLabel:              "",
		InitialSelectionID:       model.initialSelectionID,
		NewestLegend:             model.newestLegend,
		RequireExplicitSelection: model.requireExplicitSelection,
		Notice:                   notice,
		Keyboard:                 m.keyboard,
		Placement: sprites.PercentBasedPlacement{
			StartPercentX: 0,
			EndPercentX:   0,
			StartPercentY: 0,
			EndPercentY:   0,
		},
		OnPrimary: func(view widgets.SaveSlotView) {
			summary, ok := model.summary(view)
			if !ok {
				panic("IntroMenuScene: save browser returned an unknown slot")
			}
			if mode == playerSaveBrowserLoad {
				m.loadReviewedPlayerSave(game, browser, mode, legacySave, summary)

				return
			}
			if legacySave == nil {
				panic("IntroMenuScene: import browser requires a prepared legacy save")
			}
			m.selectLegacyImportDestination(game, browser, summary, legacySave)
		},
		OnCancel: func() {
			m.dialogStack.MustRemoveWidgetInstance(browser)
			m.menuIdleStartedMs = m.elapsedMs
		},
		IsKeyPressed: nil,
	})
	m.notice = ""
	m.dialogStack.PushModalDialog(browser)
}

func buildIntroSaveBrowserModel(
	summaries []player_save.SlotSummary,
	mode playerSaveBrowserMode,
	legacySave *preparedLegacyPlayerSave,
	selectionID string,
) playerSaveBrowserModel {
	playthroughID := ""
	if legacySave != nil {
		playthroughID = legacySave.playthrough.id
	}
	model := buildPlayerSaveBrowserModel(summaries, mode, playthroughID)
	if legacySave != nil {
		for index := range model.views {
			model.views[index].Details = append([]widgets.SaveSlotDetail{
				{Label: "Source folder", Value: legacySave.sourceDirectory},
			}, model.views[index].Details...)
		}
	}
	if selectionID != "" {
		model.initialSelectionID = selectionID
		model.requireExplicitSelection = false
	}

	return model
}

func (m *IntroMenuScene) loadReviewedPlayerSave(
	game *Game,
	browser *widgets.SaveSlotBrowser,
	mode playerSaveBrowserMode,
	legacySave *preparedLegacyPlayerSave,
	summary player_save.SlotSummary,
) {
	if err := game.loadPlayerSaveSlot(summary.Slot, summary.Revision); err != nil {
		m.refreshPlayerSaveBrowserAfterFailure(
			game,
			browser,
			mode,
			legacySave,
			summary.Slot,
			"Load",
			err,
		)

		return
	}
	m.dialogStack.MustRemoveWidgetInstance(browser)
}

func (m *IntroMenuScene) selectLegacyImportDestination(
	game *Game,
	browser *widgets.SaveSlotBrowser,
	summary player_save.SlotSummary,
	legacySave *preparedLegacyPlayerSave,
) {
	switch summary.Status {
	case player_save.SlotEmpty, player_save.SlotReady, player_save.SlotIncompatible, player_save.SlotCorrupt:
		showPlayerSaveDescriptionEditor(&m.dialogStack, m.keyboard, "Import To", summary,
			legacySave.playthrough.id, &legacySave.draft, func(note string) {
				legacySave.playthrough.userDescription = note
				m.importLegacySave(game, browser, summary, legacySave)
			})
	case player_save.SlotUnavailable:
		// These rows are disabled by the reviewed browser model.
	default:
		panic("IntroMenuScene: unsupported player save slot status")
	}
}

func (m *IntroMenuScene) importLegacySave(
	game *Game,
	browser *widgets.SaveSlotBrowser,
	summary player_save.SlotSummary,
	legacySave *preparedLegacyPlayerSave,
) {
	m.dialogStack.MustRemoveWidgetInstance(browser)
	err := game.importPreparedLegacyPlayerSave(summary.Slot, summary.Revision, *legacySave)
	if err != nil {
		m.openPlayerSaveBrowserWithState(game, playerSaveBrowserWrite, legacySave,
			string(summary.Slot), playerSaveFailureMessage("Import", err))
		dialog := widgets.NewOverlayDialog(fmt.Sprintf("Import to %s failed.\n\n%v\n\nReturn to the save browser.",
			playerSaveSlotName(summary.Slot), err)).
			AsConfirmation().
			WithPosition(widgets.PositionScreenCenter).
			WithUserDismissed(m.keyboard).
			AutoPopFromStack(&m.dialogStack).
			Build()
		m.keyboard.PrepareForTransition(0)
		dialog.ShowWithoutAnimation()
		m.dialogStack.PushModalDialog(dialog)

		return
	}
	if len(legacySave.warnings) > 0 {
		logging.Warnf(logging.CategoryUI,
			"Legacy save import completed with %d conversion warnings", len(legacySave.warnings))
	}
}

func (m *IntroMenuScene) refreshPlayerSaveBrowserAfterFailure(
	game *Game,
	browser *widgets.SaveSlotBrowser,
	mode playerSaveBrowserMode,
	legacySave *preparedLegacyPlayerSave,
	slot player_save.SlotID,
	action string,
	err error,
) {
	m.dialogStack.MustRemoveWidgetInstance(browser)
	m.openPlayerSaveBrowserWithState(
		game,
		mode,
		legacySave,
		string(slot),
		playerSaveFailureMessage(action, err),
	)
}

func (m *IntroMenuScene) reportPlayerSaveError(action string, err error) {
	m.notice = playerSaveFailureMessage(action, err)
}

func (m *IntroMenuScene) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)
	if !m.menuVisible {
		if m.returnView != nil {
			m.drawTitleBackdrop(screen)
			m.returnView.draw(m.presentationCanvas, m.elapsedMs)
			drawIntroCanvas(screen, m.presentationCanvas)
			m.drawReturnViewCaption(screen)

			return
		}
		if m.narrative != nil {
			m.narrative.draw(m.presentationCanvas, m.elapsedMs)
			drawNarrativeCanvas(screen, m.presentationCanvas)

			return
		}
		m.drawPresentation(screen)

		return
	}
	m.drawMenu(screen)
	m.dialogStack.Draw(screen)
}

func (m *IntroMenuScene) drawReturnViewCaption(screen *ebiten.Image) {
	caption := m.returnView.caption()
	if caption == "" {
		return
	}
	leftCell, rightCell := returnViewCaptionCells(caption)
	scale, offsetX, offsetY := introCanvasTransform(screen)
	m.noticeOutput.Reset()
	m.noticeOutput.AppendLine().White(caption).WithAlignment(textv2.AlignCenter).Done()
	m.noticeOutput.DrawPanelContent(
		screen,
		image.Pt(
			int(offsetX+float64((leftCell+1)*8)*scale),
			int(offsetY+returnViewCaptionY*scale),
		),
		int(float64((rightCell-leftCell-2)*8)*scale),
		textv2.AlignCenter,
	)
}

func (m *IntroMenuScene) drawPresentation(screen *ebiten.Image) {
	if m.presentation.phase == introTitleFizz || m.presentation.phase == introFlameFizz {
		m.drawTitleFizz(screen)

		return
	}

	canvas := m.presentationCanvas
	canvas.Fill(color.Black)
	switch m.presentation.phase {
	case introOrigin:
		m.drawOriginFrame(canvas)
	case introPresents:
		drawCenteredIntroImage(canvas, m.introSprites.OriginFrames[6], 45)
		drawIntroImage(canvas, m.introSprites.Presents, 108, 140)
	case introArticle:
		drawIntroImage(canvas, m.introSprites.ArticleA, 152, 0)
	case introSignature:
		drawIntroImage(canvas, m.introSprites.ArticleA, 152, 0)
		for _, point := range m.presentation.signaturePoints[m.signaturePointCount:] {
			m.signatureCanvas.Set(point.x, point.y, color.White)
		}
		m.signaturePointCount = len(m.presentation.signaturePoints)
		canvas.DrawImage(m.signatureCanvas, nil)
	case introProduction, introPresentationComplete:
		drawIntroImage(canvas, m.introSprites.ArticleA, 152, 0)
		drawIntroImage(canvas, m.introSprites.BritishSignature, 24, 66)
		drawIntroImage(canvas, m.introSprites.Production, 104, 160)
	case introTitleFizz, introFlameFizz:
		return
	}

	drawIntroCanvas(screen, canvas)
}

func (m *IntroMenuScene) drawOriginFrame(canvas *ebiten.Image) {
	frame := m.presentation.currentOriginFrame()
	img := m.introSprites.OriginFrames[frame.imageIndex]
	canvas.DrawImage(img, originFrameDrawOptions(img.Bounds().Size(), frame.flipped))
}

func originFramePosition(size image.Point) image.Point {
	const fullHeight = 61

	return image.Pt(
		(introLogicalWidth-size.X)/2,
		introOriginFullY+(fullHeight-size.Y)/2,
	)
}

func originFrameDrawOptions(size image.Point, flipped bool) *ebiten.DrawImageOptions {
	position := originFramePosition(size)
	drawOptions := &ebiten.DrawImageOptions{}
	if flipped {
		drawOptions.GeoM.Scale(1, -1)
		drawOptions.GeoM.Translate(float64(position.X), float64(position.Y+size.Y))
	} else {
		drawOptions.GeoM.Translate(float64(position.X), float64(position.Y))
	}

	return drawOptions
}

func (m *IntroMenuScene) drawTitleFizz(screen *ebiten.Image) {
	canvas := m.presentationCanvas
	canvas.Fill(color.Black)
	if m.presentation.phase == introTitleFizz {
		m.titleFizz.UpdateProgress(m.presentation.phaseProgress(introTitleFizzDuration))
		drawIntroDissolve(canvas, m.titleFizzSource, m.titleFizz, m.titleFizzBuffer, nil)
		drawIntroCanvas(screen, canvas)

		return
	}

	drawIntroImage(canvas, m.introSprites.Ultima16Logo, 0, 0)
	drawIntroImage(canvas, m.introSprites.FlameFizzBase, introFlameX, introFlameY)
	m.flameFizz.UpdateProgress(m.presentation.phaseProgress(introFlameFizzDuration))
	drawIntroDissolve(
		canvas,
		m.introSprites.FlameAnimation.GetCurrentImageAtTime(0),
		m.flameFizz,
		m.flameFizzBuffer,
		introImageDrawOptions(introFlameX, introFlameY),
	)
	drawIntroCanvas(screen, canvas)
}

func drawIntroDissolve(
	screen, source *ebiten.Image,
	effect *visual.PixelDissolveEffect,
	buffer *ebiten.Image,
	drawOptions *ebiten.DrawImageOptions,
) {
	buffer.Clear()
	buffer.DrawImage(source, nil)
	maskOptions := &ebiten.DrawImageOptions{}
	maskOptions.Blend = ebiten.BlendDestinationIn
	buffer.DrawImage(effect.GetMask(), maskOptions)
	if drawOptions == nil {
		drawOptions = &ebiten.DrawImageOptions{}
	}
	screen.DrawImage(buffer, drawOptions)
}

func drawCenteredIntroImage(canvas, img *ebiten.Image, y int) {
	drawIntroImage(canvas, img, (introLogicalWidth-img.Bounds().Dx())/2, y)
}

func drawIntroImage(canvas, img *ebiten.Image, x, y int) {
	canvas.DrawImage(img, introImageDrawOptions(x, y))
}

func introImageDrawOptions(x, y int) *ebiten.DrawImageOptions {
	drawOptions := &ebiten.DrawImageOptions{}
	drawOptions.GeoM.Translate(float64(x), float64(y))

	return drawOptions
}

func drawIntroCanvas(screen, canvas *ebiten.Image) {
	scale, offsetX, offsetY := introCanvasTransform(screen)
	drawScaledIntroCanvas(screen, canvas, scale, offsetX, offsetY)
}

func drawNarrativeCanvas(screen, canvas *ebiten.Image) {
	scale, offsetX, offsetY := narrativeCanvasTransform(screen.Bounds())
	drawScaledIntroCanvas(screen, canvas, scale, offsetX, offsetY)
}

func drawScaledIntroCanvas(screen, canvas *ebiten.Image, scale, offsetX, offsetY float64) {
	drawOptions := &ebiten.DrawImageOptions{}
	drawOptions.Filter = ebiten.FilterNearest
	drawOptions.GeoM.Scale(scale, scale)
	drawOptions.GeoM.Translate(offsetX, offsetY)
	screen.DrawImage(canvas, drawOptions)
}

func introCanvasTransform(screen *ebiten.Image) (float64, float64, float64) {
	scale := min(float64(screen.Bounds().Dx())/introLogicalWidth, float64(screen.Bounds().Dy())/introLogicalHeight)
	offsetX := (float64(screen.Bounds().Dx()) - introLogicalWidth*scale) / 2
	offsetY := (float64(screen.Bounds().Dy()) - introLogicalHeight*scale) / 2

	return scale, offsetX, offsetY
}

func narrativeCanvasTransform(bounds image.Rectangle) (float64, float64, float64) {
	placement := narrativeScreenPlacement()
	availableWidth := float64(bounds.Dx()) * (placement.EndPercentX - placement.StartPercentX)
	availableHeight := float64(bounds.Dy()) * (placement.EndPercentY - placement.StartPercentY)
	scale := min(availableWidth/introLogicalWidth, availableHeight/introLogicalHeight)
	offsetX := (float64(bounds.Dx()) - introLogicalWidth*scale) / 2
	offsetY := (float64(bounds.Dy()) - introLogicalHeight*scale) / 2

	return scale, offsetX, offsetY
}

func narrativeScreenPlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: narrativeScreenPadding,
		EndPercentX:   1 - narrativeScreenPadding,
		StartPercentY: narrativeScreenPadding,
		EndPercentY:   1 - narrativeScreenPadding,
	}
}

func (m *IntroMenuScene) drawTitleBackdrop(screen *ebiten.Image) {
	canvas := m.presentationCanvas
	canvas.Clear()
	drawIntroImage(canvas, m.introSprites.Ultima16Logo, 0, 0)
	flameElapsedMs := m.elapsedMs - m.flameAnimationStartMs
	drawIntroImage(
		canvas,
		m.introSprites.FlameAnimation.GetCurrentImageAtTime(flameElapsedMs),
		introFlameX,
		introFlameY,
	)
	drawIntroCanvas(screen, canvas)

	const reduxStartX = 0.3
	opRedux := sprites.GetXSpriteWithPercents(m.introSprites.ReduxLogo.Bounds(), sprites.PercentXBasedPlacement{
		StartPercentX: reduxStartX,
		EndPercentX:   1 - reduxStartX,
		StartPercentY: 0.28,
	})
	screen.DrawImage(m.introSprites.ReduxLogo, opRedux)
}

func (m *IntroMenuScene) drawMenu(screen *ebiten.Image) {
	m.drawTitleBackdrop(screen)
	menuBorder, menuBorderOp := m.borderSprites.VeryPixelatedRoundedBlueWhite.CreateSizedAndScaledBorderSprite(
		introMenuBorderIdealWidth,
		text.IntroMenuFramePlacement(),
	)
	screen.DrawImage(menuBorder, menuBorderOp)
	if m.menuRenderer == nil {
		m.menuRenderer = text.NewIntroMenuRenderer(m.ultimaFont)
	}
	state := m.introMenuState()
	m.menuRenderer.DrawChoices(screen, state)
	if state.SubmenuOpen {
		panelBorder, panelBorderOp := m.borderSprites.VeryPixelatedRoundedBlueWhite.CreateSizedAndScaledBorderSprite(
			text.IntroSubmenuBorderIdealWidth,
			m.menuRenderer.SubmenuPanelPlacement(screen.Bounds(), state),
		)
		screen.DrawImage(panelBorder, panelBorderOp)
		m.menuRenderer.DrawSubmenu(screen, state)
	}
	m.drawNotice(screen)
	if m.showVersion {
		m.drawVersion(screen)
	}
}

const (
	// introMenuBorderIdealWidth is the screen width at which the menu border
	// draws its reference artwork at 1:1; wider screens scale it up.
	introMenuBorderIdealWidth = 400
	// introMenuBorderBandReferencePixels is the blue band's thickness in the
	// 20x20 reference border, and introMenuBorderCornerReferencePixels clears
	// its rounded corner.
	introMenuBorderBandReferencePixels   = 6
	introMenuBorderCornerReferencePixels = 8
	// introVersionFontBandFraction sizes the version font to sit inside the band.
	introVersionFontBandFraction = 0.8
)

// introMenuBorderScale is the menu border's reference-to-screen pixel scale.
func introMenuBorderScale(screenWidth int) float64 {
	return float64(screenWidth) / introMenuBorderIdealWidth
}

// drawVersion writes the build version on the menu frame's lower blue band,
// right-aligned clear of the rounded corner. V toggles it.
func (m *IntroMenuScene) drawVersion(screen *ebiten.Image) {
	bounds := screen.Bounds()
	placement := text.IntroMenuFramePlacement()
	scale := introMenuBorderScale(bounds.Dx())
	bandHeight := introMenuBorderBandReferencePixels * scale
	bandTop := float64(bounds.Dy())*placement.EndPercentY - bandHeight
	left := float64(bounds.Dx()) * placement.StartPercentX
	right := float64(bounds.Dx())*placement.EndPercentX - introMenuBorderCornerReferencePixels*scale
	// Centering the line box centers the glyphs, descenders included.
	top := bandTop + (bandHeight-m.versionOutput.GetLineSpacing())/2
	m.versionOutput.DrawPanelContent(
		screen,
		image.Pt(int(math.Round(left)), int(math.Round(top))),
		int(right-left),
		textv2.AlignEnd,
	)
}

// newIntroVersionFont sizes the version line to the menu border's blue band.
func newIntroVersionFont() *text.UltimaFont {
	screenWidth, _ := display.GetManager().GetCurrentSize()
	bandHeight := introMenuBorderBandReferencePixels * introMenuBorderScale(screenWidth)

	return text.NewUltimaFont(bandHeight * introVersionFontBandFraction)
}

// newIntroVersionOutput holds the title screen's version line, so bug reports
// can quote it; the content never changes after construction.
func newIntroVersionOutput() *text.Output {
	font := newIntroVersionFont()
	output := text.NewOutput(font, font.TextFace.Metrics().HAscent*1.2, 1, 80)
	output.AppendLine().White(runtimeGameVersion()).WithAlignment(textv2.AlignEnd).Done()

	return output
}

func (m *IntroMenuScene) drawNotice(screen *ebiten.Image) {
	if m.notice == "" {
		return
	}
	m.prepareNotice(screen.Bounds())
	m.noticeOutput.DrawPanelContent(
		screen,
		image.Pt(screen.Bounds().Dx()/10, int(float64(screen.Bounds().Dy())*0.575)),
		screen.Bounds().Dx()*8/10,
		textv2.AlignCenter,
	)
}

func (m *IntroMenuScene) prepareNotice(bounds image.Rectangle) {
	m.noticeOutput.Reset()
	m.noticeOutput.SetMaxCharsPerLine(max(1, m.noticeFont.CharsForWidth(bounds.Dx()*8/10)))
	if m.notice != "" {
		line := m.noticeOutput.AppendLine().WithAlignment(textv2.AlignCenter).Yellow(m.notice)
		if m.legacyPrepareActive {
			widgets.AppendControlHints(line.Text("  ", widgets.DialogControlHintColor), "[Esc] Cancel")
		}
		line.WordWrap().Done()
	}
}
