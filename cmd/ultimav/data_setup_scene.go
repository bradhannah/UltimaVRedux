//nolint:mnd // Asset-independent setup layout uses stable panel proportions and buffer capacities.
package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/originaldata"
	"github.com/bradhannah/UltimaVRedux/internal/platform/texteditor"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

type dataSetupMode int

const (
	dataSetupFirstRun dataSetupMode = iota
	dataSetupFromTitle
	setupBackLabel          = "Back"
	setupReturnToMenuLabel  = "Return to Menu"
	setupDescriptionRows    = 12
	setupSourceDetailLabel  = "Source"
	setupFolderGuidance     = "Choose the folder containing DATA.OVL, not the DATA.OVL file itself."
	setupSearchDisplayDelay = 12
)

type dataSetupSearchResult struct {
	generation uint64
	report     originaldata.Report
}

type dataSetupValidationKind int

const (
	dataSetupValidateCandidate dataSetupValidationKind = iota
	dataSetupCheckSettings
	dataSetupLoadSelected
)

type dataSetupValidationResult struct {
	generation uint64
	kind       dataSetupValidationKind
	candidate  originaldata.Candidate
	playable   *config.UltimaVConfiguration
	err        error
}

type dataSetupScene struct {
	configuration      *config.UltimaVConfiguration
	configurationError error
	originalDataError  error
	host               originaldata.HostProfile
	mode               dataSetupMode
	soundDisabled      bool
	keyboard           *input.Keyboard
	dialogStack        widgets.DialogStack
	editor             texteditor.Launcher
	editorResult       <-chan error
	discover           func(context.Context, originaldata.DiscoveryOptions) originaldata.Report
	validate           originaldata.ValidateFunc
	loadPlayable       func() (*config.UltimaVConfiguration, error)
	loadCandidate      func(string) (*config.UltimaVConfiguration, error)
	onReady            func(*Game, *config.UltimaVConfiguration) error
	onReturn           func(*Game) error

	cancelSearch         context.CancelFunc
	searchResults        chan dataSetupSearchResult
	searchGeneration     uint64
	searching            bool
	searchDisplayFrames  int
	cancelValidation     context.CancelFunc
	validationResults    chan dataSetupValidationResult
	validationGeneration uint64
	validationKind       dataSetupValidationKind
	validationCandidate  originaldata.Candidate
	validationActivity   string
	validating           bool
	report               originaldata.Report
	status               string
	candidateChoices     []originaldata.Candidate
	candidateSelectionID string
	candidateFromBrowser bool
	active               bool
	disposed             bool
	settingsVisible      bool
	settingsBack         func()

	font        *text.UltimaFont
	heading     *text.Output
	body        *text.Output
	footer      *text.Output
	pendingGame *Game
}

//nolint:funlen // Explicit field initialization keeps the setup lifecycle contract auditable.
func newDataSetupScene(
	configuration *config.UltimaVConfiguration,
	configurationError error,
	originalDataError error,
	host originaldata.HostProfile,
	mode dataSetupMode,
	soundDisabled bool,
	onReady func(*Game, *config.UltimaVConfiguration) error,
	onReturn func(*Game) error,
) *dataSetupScene {
	if configuration == nil {
		panic("newDataSetupScene: recovery configuration is required")
	}
	if mode == dataSetupFirstRun && onReady == nil {
		panic("newDataSetupScene: first-run completion callback is required")
	}
	if mode == dataSetupFromTitle && onReturn == nil {
		panic("newDataSetupScene: title return callback is required")
	}
	configurationPath, configurationPathErr := configuration.ConfigurationFilePath()
	scene := &dataSetupScene{
		configuration:      configuration,
		configurationError: configurationError,
		originalDataError:  originalDataError,
		host:               host,
		mode:               mode,
		soundDisabled:      soundDisabled,
		keyboard:           input.NewKeyboard(250),
		dialogStack:        widgets.DialogStack{},
		editor:             texteditor.SystemLauncher{},
		editorResult:       nil,
		discover:           originaldata.Discover,
		validate:           originaldata.ValidateInstallation,
		loadPlayable: func() (*config.UltimaVConfiguration, error) {
			if configurationPathErr != nil {
				return nil, fmt.Errorf("locating saved settings for setup: %w", configurationPathErr)
			}

			return config.LoadUltimaVConfiguration(configurationPath, "")
		},
		loadCandidate: func(candidate string) (*config.UltimaVConfiguration, error) {
			if configurationPathErr != nil {
				return nil, fmt.Errorf("locating candidate settings for setup: %w", configurationPathErr)
			}

			return config.LoadUltimaVConfiguration(configurationPath, candidate)
		},
		onReady:              onReady,
		onReturn:             onReturn,
		cancelSearch:         nil,
		searchResults:        make(chan dataSetupSearchResult, 1),
		searchGeneration:     0,
		searching:            false,
		searchDisplayFrames:  0,
		cancelValidation:     nil,
		validationResults:    make(chan dataSetupValidationResult, 1),
		validationGeneration: 0,
		validationKind:       dataSetupValidateCandidate,
		validationCandidate:  originaldata.Candidate{Path: "", Source: "", Label: ""},
		validationActivity:   "",
		validating:           false,
		report: originaldata.Report{
			Candidates: nil, Details: nil, CheckedRoots: nil, RecommendedDirectory: "", Cancelled: false,
		},
		status:               "",
		candidateChoices:     nil,
		candidateSelectionID: "",
		candidateFromBrowser: false,
		active:               false,
		disposed:             false,
		settingsVisible:      false,
		settingsBack:         nil,
		font:                 nil,
		heading:              nil,
		body:                 nil,
		footer:               nil,
		pendingGame:          nil,
	}
	scene.initializeText()

	return scene
}

func (s *dataSetupScene) Activate() {
	if s.disposed {
		panic("dataSetupScene.Activate: disposed scene cannot be activated")
	}
	if s.active {
		return
	}
	s.active = true
	s.startSearch()
}

func (s *dataSetupScene) PrepareInputForSceneActivation() {
	s.keyboard.PrepareForTransition(0)
}

func (s *dataSetupScene) PrepareForSceneReplacement() {
	s.stopSearch()
	s.stopValidation()
}

func (s *dataSetupScene) Dispose() {
	if s.disposed {
		return
	}
	s.stopSearch()
	s.stopValidation()
	s.clearDialogs()
	s.active = false
	s.disposed = true
}

func (s *dataSetupScene) GetUltimaConfiguration() *config.UltimaVConfiguration {
	return s.configuration
}

func (s *dataSetupScene) InvalidateResolution() {
	s.initializeText()
}

func (s *dataSetupScene) Update(game *Game) error {
	s.pendingGame = game
	s.keyboard.ClearFrameConsumption()
	s.updateEditorResult(game)
	if modal := s.dialogStack.PeekTopModalDialog(); modal != nil {
		modal.Update()

		return nil
	}
	if s.searching && s.updateSearch(game) {
		return nil
	}
	if s.validating {
		if s.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
			s.cancelValidationFlow(game)

			return nil
		}
		select {
		case result := <-s.validationResults:
			if result.generation == s.validationGeneration {
				s.validating = false
				if s.cancelValidation != nil {
					s.cancelValidation()
				}
				s.cancelValidation = nil
				s.handleValidationResult(game, result)

				return nil
			}
		default:
		}
	}

	return nil
}

// updateSearch reports cancellation so Update cannot accept other work in that frame.
func (s *dataSetupScene) updateSearch(game *Game) bool {
	if s.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		s.stopSearch()
		if s.mode == dataSetupFromTitle {
			s.returnToTitle(game)
		} else {
			s.status = "Search cancelled. No files or settings were changed."
			s.showRecoveryActions(game)
		}

		return true
	}
	s.searchDisplayFrames++
	select {
	case result := <-s.searchResults:
		if result.generation == s.searchGeneration {
			s.stopSearch()
			s.report = result.report
			s.showSearchResult(game)
		}
	default:
	}

	return false
}

func (s *dataSetupScene) Draw(screen *ebiten.Image) {
	screen.Fill(ucolor.EGABlack)
	if s.searching && s.searchDisplayFrames >= setupSearchDisplayDelay {
		s.drawSearch(screen)
	}
	if s.validating {
		s.drawValidation(screen)
	}
	s.dialogStack.Draw(screen)
}

func (s *dataSetupScene) drawValidation(screen *ebiten.Image) {
	width, height := screen.Bounds().Dx(), screen.Bounds().Dy()
	s.heading.Reset()
	s.heading.AppendLine().WithAlignment(etext.AlignCenter).White("CHECK ULTIMA V GAME FILES").Done()
	s.heading.DrawPanelContent(screen, image.Pt(width/10, height/5), width*8/10, etext.AlignCenter)
	s.body.Reset()
	s.body.AppendLine().WithAlignment(etext.AlignCenter).White(s.validationActivity).WordWrap().Done()
	s.body.AppendLine().WithAlignment(etext.AlignCenter).
		White("Your original files will not be changed.").Done()
	s.body.DrawPanelContent(screen, image.Pt(width/8, height*2/5), width*3/4, etext.AlignCenter)
	s.footer.Reset()
	widgets.AppendControlHints(s.footer.AppendLine().WithAlignment(etext.AlignCenter), "[Esc] Cancel").Done()
	s.footer.DrawPanelContent(screen, image.Pt(width/10, height*4/5), width*8/10, etext.AlignCenter)
}

func (s *dataSetupScene) initializeText() {
	s.font = text.NewUltimaFont(text.GetIntroFontSize())
	lineSpacing := s.font.TextFace.Metrics().HAscent * 1.4
	s.heading = text.NewOutput(s.font, lineSpacing, 2, 80)
	s.body = text.NewOutput(s.font, lineSpacing, 10, 120)
	s.footer = text.NewOutput(s.font, lineSpacing, 2, 100)
}

func (s *dataSetupScene) drawSearch(screen *ebiten.Image) {
	width, height := screen.Bounds().Dx(), screen.Bounds().Dy()
	s.heading.Reset()
	s.heading.AppendLine().WithAlignment(etext.AlignCenter).White("LOCATE ULTIMA V GAME FILES").Done()
	s.heading.DrawPanelContent(screen, image.Pt(width/10, height/5), width*8/10, etext.AlignCenter)
	s.body.Reset()
	s.body.AppendLine().WithAlignment(etext.AlignCenter).White("Looking in the usual places...").Done()
	s.body.AppendLine().WithAlignment(etext.AlignCenter).
		White("Checking configured folders, GOG and Heroic installations, and your home UltimaV folder").WordWrap().Done()
	s.body.AppendLine().WithAlignment(etext.AlignCenter).
		White("Recommended fallback: " + originaldata.RecommendedDirectory(s.host)).WordWrap().Done()
	s.body.DrawPanelContent(screen, image.Pt(width/8, height*2/5), width*3/4, etext.AlignCenter)
	s.footer.Reset()
	widgets.AppendControlHints(s.footer.AppendLine().WithAlignment(etext.AlignCenter), "[Esc] Cancel").Done()
	s.footer.DrawPanelContent(screen, image.Pt(width/10, height*4/5), width*8/10, etext.AlignCenter)
}

func (s *dataSetupScene) startSearch() {
	s.stopValidation()
	s.settingsVisible = false
	s.clearDialogs()
	s.stopSearch()
	s.searchGeneration++
	generation := s.searchGeneration
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelSearch = cancel
	s.searching = true
	s.searchDisplayFrames = 0
	s.status = ""
	configured := ""
	if s.configuration.SavedConfigData != nil {
		configured = s.configuration.SavedConfigData.DataFilePath
	}
	previous := s.configuration.PersistedDataFilePath()
	options := originaldata.DiscoveryOptions{
		Host: s.host, ConfiguredDirectory: configured,
		PreviousDataDirectory: previous,
		AdditionalSeeds:       nil,
		Validate:              s.validate,
	}
	results := make(chan dataSetupSearchResult, 1)
	s.searchResults = results
	discover := s.discover
	go func() {
		report := discover(ctx, options)
		select {
		case results <- dataSetupSearchResult{generation: generation, report: report}:
		case <-ctx.Done():
		}
	}()
}

func (s *dataSetupScene) stopSearch() {
	s.searchGeneration++
	if s.cancelSearch != nil {
		s.cancelSearch()
		s.cancelSearch = nil
	}
	s.searching = false
	s.searchDisplayFrames = 0
}

func (s *dataSetupScene) beginValidation(
	kind dataSetupValidationKind,
	activity string,
	work func(context.Context) dataSetupValidationResult,
) {
	s.stopSearch()
	s.settingsVisible = false
	s.editorResult = nil
	s.clearDialogs()
	s.stopValidation()
	s.validationGeneration++
	generation := s.validationGeneration
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelValidation = cancel
	s.validationKind = kind
	s.validationActivity = activity
	s.validating = true
	results := make(chan dataSetupValidationResult, 1)
	s.validationResults = results
	go func() {
		result := work(ctx)
		result.generation = generation
		result.kind = kind
		select {
		case results <- result:
		case <-ctx.Done():
		}
	}()
}

func (s *dataSetupScene) stopValidation() {
	if s.validating || s.cancelValidation != nil {
		s.validationGeneration++
	}
	if s.cancelValidation != nil {
		s.cancelValidation()
		s.cancelValidation = nil
	}
	s.validating = false
}

// folderProblemStatus words a failed folder check for the player and keeps the
// technical error in the log.
func folderProblemStatus(err error) string {
	logging.Warnf(logging.CategorySystem, "Game folder check failed: %v", err)
	if reason := originaldata.PlainReason(err); reason != "" {
		return "That folder no longer works: " + reason
	}

	return "That folder no longer works. The game log has the details."
}

// setupProblemStatus words a settings or start-up failure for the player and
// keeps the technical error in the log.
func setupProblemStatus(sentence string, err error) string {
	logging.Warnf(logging.CategorySystem, "%s %v", sentence, err)
	switch {
	case errors.Is(err, fs.ErrPermission):
		return sentence + " Redux is not allowed to use that file or folder."
	case errors.Is(err, fs.ErrNotExist):
		return sentence + " The file or folder is missing."
	default:
		return sentence + " The game log has the details."
	}
}

func (s *dataSetupScene) handleValidationResult(game *Game, result dataSetupValidationResult) {
	switch result.kind {
	case dataSetupValidateCandidate:
		if result.err != nil {
			s.status = folderProblemStatus(result.err)
			if s.candidateFromBrowser {
				s.showCandidateBrowser(game, s.candidateChoices, s.status)

				return
			}
			s.showSingleCandidate(game, result.candidate)

			return
		}
		s.status = ""
		s.commitCandidate(game, result.candidate)
	case dataSetupCheckSettings:
		if result.err != nil {
			s.status = setupProblemStatus("The saved settings cannot be used yet.", result.err)
			if originaldata.PlainReason(result.err) != "" {
				s.status = folderProblemStatus(result.err)
			}
			s.showSettingsRecoveryWithBack(game, false, s.settingsBack)

			return
		}
		s.completeSavedSettings(game, result.playable)
	case dataSetupLoadSelected:
		if result.err != nil {
			s.status = folderProblemStatus(result.err)
			s.showRecoveryActions(game)

			return
		}
		if err := s.persistCandidate(result.candidate); err != nil {
			s.status = setupProblemStatus("The folder is good, but the choice could not be saved.", err)
			s.showRecoveryActions(game)

			return
		}
		result.playable.AdoptPersistedConfiguration(s.configuration)
		result.playable.SoundDisabled = s.soundDisabled
		if err := s.onReady(game, result.playable); err != nil {
			s.status = setupProblemStatus("Redux could not start with these files.", err)
			s.showRecoveryActions(game)
		}
	}
}

func (s *dataSetupScene) cancelValidationFlow(game *Game) {
	kind := s.validationKind
	candidate := s.validationCandidate
	s.stopValidation()
	switch kind {
	case dataSetupValidateCandidate:
		s.status = "Folder validation was cancelled. No settings were changed."
		if s.candidateFromBrowser {
			s.showCandidateBrowser(game, s.candidateChoices, s.status)

			return
		}
		s.showSingleCandidate(game, candidate)
	case dataSetupCheckSettings:
		s.status = "Saved-settings validation was cancelled."
		s.showSettingsRecoveryWithBack(game, false, s.settingsBack)
	case dataSetupLoadSelected:
		s.status = "Loading was cancelled. No settings were changed."
		s.showRecoveryActions(game)
	}
}

func (s *dataSetupScene) showSearchResult(game *Game) {
	switch len(s.report.Candidates) {
	case 0:
		s.status = "No complete Ultima V installation was found. Put your Ultima V files in " +
			originaldata.RecommendedDirectory(s.host) + " and choose Check Again."
		s.showRecoveryActions(game)
	case 1:
		s.showSingleCandidate(game, s.report.Candidates[0])
	default:
		s.showCandidateBrowser(game, s.report.Candidates, "")
	}
}

func (s *dataSetupScene) showSingleCandidate(game *Game, candidate originaldata.Candidate) {
	s.candidateChoices = []originaldata.Candidate{candidate}
	s.candidateSelectionID = candidate.Path
	s.candidateFromBrowser = false
	description := fmt.Sprintf(
		"Redux found Ultima V.\nSource: %s\nFolder: %s\nYour original files will not be changed.",
		candidate.Label, candidate.Path,
	)
	if s.status != "" {
		description = s.status + "\n" + description
	}
	s.showButtonMenu("Ultima V Found", description, []setupButton{
		{label: "Use This Installation", action: func() { s.useCandidate(game, candidate) }},
		{label: "Look Somewhere Else", action: func() { s.showAlternativeActions(game) }},
		{label: s.cancelLabel(), action: func() { s.cancelSetup(game) }},
	})
}

func (s *dataSetupScene) showCandidateBrowser(
	game *Game,
	candidates []originaldata.Candidate,
	notice string,
) {
	s.settingsVisible = false
	s.clearDialogs()
	const alternativeLocationID = "look-somewhere-else"
	s.candidateChoices = append([]originaldata.Candidate(nil), candidates...)
	initialSelectionID := s.preferredCandidateSelectionID(candidates)
	newestLegend := s.currentInstallationLegend(candidates)
	if s.candidateSelectionID != "" && s.containsCandidate(candidates, s.candidateSelectionID) {
		initialSelectionID = s.candidateSelectionID
	}
	s.candidateSelectionID = initialSelectionID
	s.candidateFromBrowser = true
	views := make([]widgets.SaveSlotView, 0, len(candidates)+1)
	for _, candidate := range candidates {
		views = append(views, s.dataCandidateView(candidate))
	}
	views = append(views, widgets.SaveSlotView{
		ID: alternativeLocationID, Label: "Look Somewhere Else", Status: "Edit the settings file",
		RowSummary: "", ReviewValue: "Set DataFilePath in the saved settings file.", Description: "",
		Newest: false, HasSave: true, NavigationDisabled: false,
		Details: nil, Problem: "", PrimaryEnabled: true,
	})
	var browser *widgets.SaveSlotBrowser
	browser = widgets.NewSaveSlotBrowser(views, widgets.SaveSlotBrowserConfig{
		Title: "Choose Game Files", PrimaryActionLabel: "Use Selected", CancelActionLabel: s.cancelLabel(),
		InitialSelectionID: initialSelectionID, NewestLegend: newestLegend,
		ReviewLabel: "Selected Folder", RequireExplicitSelection: false, Notice: notice, Keyboard: s.keyboard,
		Placement: sprites.PercentBasedPlacement{
			StartPercentX: 0, EndPercentX: 0, StartPercentY: 0, EndPercentY: 0,
		},
		OnPrimary: func(view widgets.SaveSlotView) {
			s.dialogStack.MustRemoveWidgetInstance(browser)
			if view.ID == alternativeLocationID {
				s.candidateFromBrowser = false
				s.showAlternativeActions(game)

				return
			}
			for _, candidate := range candidates {
				if candidate.Path == view.ID {
					s.candidateSelectionID = candidate.Path
					s.useCandidate(game, candidate)

					return
				}
			}
		},
		OnCancel: func() {
			s.dialogStack.MustRemoveWidgetInstance(browser)
			s.cancelSetup(game)
		},
		IsKeyPressed: nil,
	})
	s.dialogStack.PushModalDialog(browser)
}

func (s *dataSetupScene) dataCandidateView(candidate originaldata.Candidate) widgets.SaveSlotView {
	active := s.candidateIsActive(candidate)
	pending := s.candidateIsPending(candidate)
	details := []widgets.SaveSlotDetail{
		{Label: "Folder", Value: candidate.Path},
		{Label: setupSourceDetailLabel, Value: string(candidate.Source)},
	}
	if s.mode == dataSetupFromTitle {
		sessionStatus := "Not active this session"
		if active {
			sessionStatus = "Active this session"
		}
		nextLaunchStatus := "Not selected"
		if pending {
			nextLaunchStatus = "Selected for next launch"
		}
		details = append(details,
			widgets.SaveSlotDetail{Label: "Session", Value: sessionStatus},
			widgets.SaveSlotDetail{Label: "Next launch", Value: nextLaunchStatus},
		)
	}

	return widgets.SaveSlotView{
		ID: candidate.Path, Label: candidate.Label, Status: "All game files found", RowSummary: "",
		ReviewValue: candidate.Path, Description: "", Newest: active, HasSave: true, NavigationDisabled: false,
		Details: details,
		Problem: "", PrimaryEnabled: true,
	}
}

func (s *dataSetupScene) preferredCandidateSelectionID(candidates []originaldata.Candidate) string {
	for _, candidate := range candidates {
		if s.candidateIsActive(candidate) {
			return candidate.Path
		}
	}
	for _, candidate := range candidates {
		if s.candidateIsPending(candidate) {
			return candidate.Path
		}
	}
	if len(candidates) > 0 {
		return candidates[0].Path
	}

	return ""
}

func (s *dataSetupScene) containsCandidate(candidates []originaldata.Candidate, path string) bool {
	for _, candidate := range candidates {
		if candidate.Path == path {
			return true
		}
	}

	return false
}

func (s *dataSetupScene) candidateIsActive(candidate originaldata.Candidate) bool {
	return s.mode == dataSetupFromTitle && s.configuration.SavedConfigData != nil &&
		candidate.Path != "" && filepathEqual(candidate.Path, s.configuration.SavedConfigData.DataFilePath)
}

func (s *dataSetupScene) candidateIsPending(candidate originaldata.Candidate) bool {
	return candidate.Path != "" && s.configuration.PersistedDataFilePath() != "" &&
		filepathEqual(candidate.Path, s.configuration.PersistedDataFilePath())
}

func (s *dataSetupScene) currentInstallationLegend(candidates []originaldata.Candidate) string {
	if s.mode != dataSetupFromTitle {
		return ""
	}
	for _, candidate := range candidates {
		if s.candidateIsActive(candidate) {
			return "* Active this session"
		}
	}

	return ""
}

func (s *dataSetupScene) showRecoveryActions(game *Game) {
	description := s.status
	if description != "" {
		description += "\n"
	}
	description += "Recommended personal folder: " + originaldata.RecommendedDirectory(s.host)
	if s.mode == dataSetupFromTitle {
		description += "\nYour current game-file location remains active."
	}
	s.showButtonMenu("Game File Setup", description, []setupButton{
		{label: "Open Settings File", action: func() { s.showSettingsRecovery(game, true) }},
		{label: "Check Again", action: func() { s.checkAgain(game) }},
		{label: "Show Search Details", action: func() { s.showSearchDetails(game) }},
		{label: s.cancelLabel(), action: func() { s.cancelSetup(game) }},
	})
}

func (s *dataSetupScene) showAlternativeActions(game *Game) {
	s.showButtonMenuWithCancel("Choose Another Location",
		"Set DataFilePath in the saved settings file to use another folder.", []setupButton{
			{label: "Open Settings File", action: func() {
				s.showSettingsRecoveryWithBack(game, true, func() { s.showAlternativeActions(game) })
			}},
			{label: setupBackLabel, action: func() { s.showSearchResult(game) }},
		}, setupBackLabel, func() { s.showSearchResult(game) })
}

func (s *dataSetupScene) showSearchDetails(game *Game) {
	s.settingsVisible = false
	s.clearDialogs()
	views := s.searchDetailViews()
	var browser *widgets.SaveSlotBrowser
	browser = widgets.NewSaveSlotBrowser(views, widgets.SaveSlotBrowserConfig{
		Title: "Search Details", PrimaryActionLabel: setupBackLabel, CancelActionLabel: setupBackLabel,
		InitialSelectionID: "", NewestLegend: "", ReviewLabel: "",
		RequireExplicitSelection: false,
		Notice:                   "Only the usual places were searched, not the whole computer.",
		Keyboard:                 s.keyboard,
		Placement: sprites.PercentBasedPlacement{
			StartPercentX: 0, EndPercentX: 0, StartPercentY: 0, EndPercentY: 0,
		},
		OnPrimary: func(widgets.SaveSlotView) {
			s.dialogStack.MustRemoveWidgetInstance(browser)
			s.showRecoveryActions(game)
		},
		OnCancel: func() {
			s.dialogStack.MustRemoveWidgetInstance(browser)
			s.showRecoveryActions(game)
		},
		IsKeyPressed: nil,
	})
	s.dialogStack.PushModalDialog(browser)
}

func (s *dataSetupScene) searchDetailViews() []widgets.SaveSlotView {
	views := make([]widgets.SaveSlotView, 0, len(s.report.CheckedRoots)+len(s.report.Details)+2)
	appendError := func(label string, err error) {
		if err == nil {
			return
		}
		views = append(views, widgets.SaveSlotView{
			ID: fmt.Sprintf("startup-%d", len(views)), Label: label, Status: "Needs attention",
			RowSummary: "Startup", ReviewValue: "", Description: err.Error(), Newest: false, HasSave: false,
			NavigationDisabled: false, Details: []widgets.SaveSlotDetail{{Label: "Reason", Value: err.Error()}},
			Problem: err.Error(), PrimaryEnabled: true,
		})
	}
	appendError("Settings", s.configurationError)
	appendError("Configured game files", s.originalDataError)
	for index, root := range s.report.CheckedRoots {
		label := root.Label
		if strings.TrimSpace(label) == "" {
			label = string(root.Source)
		}
		views = append(views, widgets.SaveSlotView{
			ID: fmt.Sprintf("root-%d", index), Label: label, Status: "Searched here",
			RowSummary: string(root.Source), ReviewValue: "", Description: "", Newest: false, HasSave: false,
			NavigationDisabled: false,
			Details: []widgets.SaveSlotDetail{
				{Label: "Root", Value: root.Path},
				{Label: setupSourceDetailLabel, Value: string(root.Source)},
			},
			Problem: "", PrimaryEnabled: true,
		})
	}
	for index, detail := range s.report.Details {
		views = append(views, widgets.SaveSlotView{
			ID: fmt.Sprintf("detail-%d", index), Label: string(detail.Source), Status: "Incomplete or skipped",
			RowSummary: "Review details", ReviewValue: "", Description: detail.Reason, Newest: false, HasSave: false,
			NavigationDisabled: false,
			Details: []widgets.SaveSlotDetail{
				{Label: "Path", Value: detail.Path},
				{Label: "Reason", Value: detail.Reason},
			},
			Problem: detail.Reason, PrimaryEnabled: true,
		})
	}
	if len(views) == 0 {
		views = append(views, widgets.SaveSlotView{
			ID: "no-search-locations", Label: "No recorded locations", Status: "Nothing was searched",
			RowSummary: "", ReviewValue: "", Description: "", Newest: false, HasSave: false, NavigationDisabled: false,
			Details: []widgets.SaveSlotDetail{{Label: "Recommended", Value: originaldata.RecommendedDirectory(s.host)}},
			Problem: "", PrimaryEnabled: true,
		})
	}

	return views
}

func (s *dataSetupScene) checkAgain(game *Game) {
	if err := s.configuration.RefreshPersistedConfiguration(); err != nil {
		s.configurationError = err
		s.status = setupProblemStatus("The saved settings could not be read again.", err)
		s.showSettingsRecovery(game, false)

		return
	}
	s.configurationError = nil
	s.startSearch()
}

func (s *dataSetupScene) useCandidate(game *Game, candidate originaldata.Candidate) {
	if err := s.configuration.RefreshPersistedConfiguration(); err != nil {
		s.configurationError = err
		s.status = setupProblemStatus("Repair the saved settings before choosing a folder.", err)
		s.showSettingsRecovery(game, false)

		return
	}
	s.configurationError = nil
	s.validationCandidate = candidate
	validate := s.validate
	s.beginValidation(dataSetupValidateCandidate, "Checking the folder before saving it...",
		func(ctx context.Context) dataSetupValidationResult {
			return dataSetupValidationResult{
				generation: 0, kind: dataSetupValidateCandidate, candidate: candidate,
				playable: nil, err: validate(ctx, candidate.Path),
			}
		})
}

func (s *dataSetupScene) commitCandidate(game *Game, candidate originaldata.Candidate) {
	if s.configurationError != nil {
		s.status = setupProblemStatus("Repair the saved settings before continuing.", s.configurationError)
		s.showSettingsRecovery(game, false)

		return
	}
	if s.mode == dataSetupFromTitle {
		active := s.candidateIsActive(candidate)
		pending := s.candidateIsPending(candidate)
		if err := s.persistCandidate(candidate); err != nil {
			s.status = setupProblemStatus("The choice could not be saved.", err)
			s.showRecoveryActions(game)

			return
		}
		if active && pending {
			s.showButtonMenu("Game-File Location Confirmed",
				"This folder is already active and saved for the next launch.", []setupButton{
					{label: setupReturnToMenuLabel, action: func() { s.returnToTitle(game) }},
				})

			return
		}
		if active {
			s.showButtonMenu("Game-File Location Confirmed",
				"The new folder choice was cleared. This session keeps the current files.", []setupButton{
					{label: setupReturnToMenuLabel, action: func() { s.returnToTitle(game) }},
				})

			return
		}
		s.showButtonMenu("Game-File Location Saved",
			"The new location takes effect next launch. This session will keep using the current files.",
			[]setupButton{
				{label: "Quit Now", action: game.requestExit},
				{label: setupReturnToMenuLabel, action: func() { s.returnToTitle(game) }},
			})

		return
	}

	loadCandidate := s.loadCandidate
	s.beginValidation(dataSetupLoadSelected, "Loading the game files...",
		func(ctx context.Context) dataSetupValidationResult {
			playable, err := loadCandidate(candidate.Path)
			if err == nil {
				err = ctx.Err()
			}

			return dataSetupValidationResult{
				generation: 0, kind: dataSetupLoadSelected,
				candidate: candidate,
				playable:  playable, err: err,
			}
		})
}

func (s *dataSetupScene) persistCandidate(candidate originaldata.Candidate) error {
	if err := s.configuration.RefreshPersistedConfiguration(); err != nil {
		s.configurationError = err

		return fmt.Errorf("rereading settings before accepting installation: %w", err)
	}
	if s.candidateIsPending(candidate) {
		return nil
	}

	if err := s.configuration.PersistDataFilePath(candidate.Path); err != nil {
		return fmt.Errorf("persisting accepted installation: %w", err)
	}

	return nil
}

func (s *dataSetupScene) showSettingsRecovery(game *Game, launch bool) {
	s.showSettingsRecoveryWithBack(game, launch, func() { s.showRecoveryActions(game) })
}

func (s *dataSetupScene) showSettingsRecoveryWithBack(game *Game, launch bool, onBack func()) {
	if onBack == nil {
		onBack = func() { s.showRecoveryActions(game) }
	}
	s.settingsBack = onBack
	path, err := s.configuration.ConfigurationFilePath()
	if err != nil {
		s.status = setupProblemStatus("The settings file could not be found.", err)
		onBack()

		return
	}
	if launch {
		result, launchErr := s.editor.Open(path)
		if launchErr != nil {
			s.status = "The editor could not be opened. Edit this file manually: " + path
		} else {
			s.editorResult = result
			s.status = "The settings file was requested in your text editor."
		}
	}
	description := fmt.Sprintf(
		"Settings file: %s\n"+
			"1. Find DataFilePath.\n"+
			"2. Replace its value with your Ultima V folder.\n"+
			"3. Save the file, then return here.\n"+
			"Example: DataFilePath: '%s'\n"+
			setupFolderGuidance,
		path, yamlSingleQuotedValue(originaldata.RecommendedDirectory(s.host)),
	)
	if s.status != "" {
		description = s.status + "\n" + description
	}
	s.showButtonMenuWithCancel("Set the Game-File Location", description, []setupButton{
		{label: "Check Saved Settings", action: func() { s.checkSavedSettings(game) }},
		{label: "Open Settings File Again", action: func() { s.showSettingsRecoveryWithBack(game, true, onBack) }},
		{label: setupBackLabel, action: onBack},
	}, setupBackLabel, onBack)
	s.settingsVisible = true
}

func (s *dataSetupScene) updateEditorResult(game *Game) {
	if s.editorResult == nil {
		return
	}
	select {
	case err, open := <-s.editorResult:
		s.editorResult = nil
		if open && err != nil {
			s.status = setupProblemStatus("The text editor reported a problem. Edit the settings file by hand.", err)
			if s.settingsVisible {
				s.showSettingsRecoveryWithBack(game, false, s.settingsBack)
			}
		}
	default:
	}
}

func (s *dataSetupScene) checkSavedSettings(_ *Game) {
	loadPlayable, validate := s.loadPlayable, s.validate
	s.beginValidation(dataSetupCheckSettings, "Checking the saved game folder...",
		func(ctx context.Context) dataSetupValidationResult {
			playable, err := loadPlayable()
			if err == nil {
				err = validate(ctx, playable.SavedConfigData.DataFilePath)
			}

			return dataSetupValidationResult{
				generation: 0, kind: dataSetupCheckSettings,
				candidate: originaldata.Candidate{Path: "", Source: "", Label: ""},
				playable:  playable, err: err,
			}
		})
}

func (s *dataSetupScene) completeSavedSettings(game *Game, playable *config.UltimaVConfiguration) {
	if err := s.configuration.RefreshPersistedConfiguration(); err != nil {
		s.configurationError = err
		s.status = setupProblemStatus("The saved settings cannot be used yet.", err)
		s.showSettingsRecoveryWithBack(game, false, s.settingsBack)

		return
	}
	s.configurationError = nil
	if !filepathEqual(playable.SavedConfigData.DataFilePath, s.configuration.PersistedDataFilePath()) {
		s.status = "The settings changed while they were being checked. Check them again."
		s.showSettingsRecoveryWithBack(game, false, s.settingsBack)

		return
	}
	playable.SoundDisabled = s.soundDisabled
	if s.mode == dataSetupFromTitle {
		if filepathEqual(playable.SavedConfigData.DataFilePath, s.configuration.SavedConfigData.DataFilePath) {
			s.showButtonMenu("Game-File Location Confirmed",
				"The saved folder is already active. No restart is needed.", []setupButton{
					{label: setupReturnToMenuLabel, action: func() { s.returnToTitle(game) }},
				})

			return
		}
		s.showButtonMenu("Game-File Location Saved",
			"The saved location was verified and will take effect next launch.", []setupButton{
				{label: "Quit Now", action: game.requestExit},
				{label: setupReturnToMenuLabel, action: func() { s.returnToTitle(game) }},
			})

		return
	}
	s.status = "The saved folder works. Confirm it to continue."
	s.showSingleCandidate(game, originaldata.Candidate{
		Path: playable.SavedConfigData.DataFilePath, Source: originaldata.SourceConfigured, Label: "Saved settings",
	})
}

func yamlSingleQuotedValue(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func (s *dataSetupScene) cancelSetup(game *Game) {
	s.stopSearch()
	s.stopValidation()
	if s.mode == dataSetupFromTitle {
		s.returnToTitle(game)

		return
	}
	game.requestExit()
}

func (s *dataSetupScene) returnToTitle(game *Game) {
	if err := s.onReturn(game); err != nil {
		s.status = setupProblemStatus("The title menu could not be restored.", err)
		s.showRecoveryActions(game)
	}
}

func (s *dataSetupScene) cancelLabel() string {
	if s.mode == dataSetupFromTitle {
		return setupBackLabel
	}

	return "Quit"
}

type setupButton struct {
	label  string
	action func()
}

func (s *dataSetupScene) showButtonMenu(title, description string, buttons []setupButton) {
	s.showButtonMenuWithCancel(title, description, buttons, s.cancelLabel(), func() {
		s.cancelSetup(s.pendingGame)
	})
}

func (s *dataSetupScene) showButtonMenuWithCancel(
	title string,
	description string,
	buttons []setupButton,
	escapeLabel string,
	onEscape func(),
) {
	s.settingsVisible = false
	s.clearDialogs()
	placement := sprites.PercentBasedPlacement{StartPercentX: 0, EndPercentX: 1, StartPercentY: 0, EndPercentY: 1}
	var menu *widgets.ButtonListModal
	menu = widgets.NewButtonListModal(title, func() {
		s.dialogStack.MustRemoveWidgetInstance(menu)
		onEscape()
	}, s.keyboard, &placement)
	menu.SetButtonWidthPercent(0.5)
	menu.SetBorderWidthPercent(0.82)
	menu.SetDescriptionRowCapacity(setupDescriptionRows)
	menu.SetEscapeLabel(escapeLabel)
	menu.SetDescription(description)
	for _, button := range buttons {
		action := button.action
		menu.AddButton(button.label, func() {
			s.dialogStack.MustRemoveWidgetInstance(menu)
			if action != nil {
				action()
			}
		})
	}
	s.dialogStack.PushModalDialog(menu)
}

func (s *dataSetupScene) clearDialogs() {
	for _, dialog := range s.dialogStack.Dialogs() {
		s.dialogStack.MustRemoveWidgetInstance(dialog)
	}
}

func filepathEqual(left, right string) bool {
	return filepathEqualForOS(left, right, runtime.GOOS)
}

func filepathEqualForOS(left, right, goos string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	leftPath, leftErr := filepath.Abs(left)
	rightPath, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		leftPath = left
		rightPath = right
	}
	if goos == "windows" {
		return strings.EqualFold(leftPath, rightPath)
	}

	return leftPath == rightPath
}
