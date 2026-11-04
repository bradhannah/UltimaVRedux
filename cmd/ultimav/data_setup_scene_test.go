package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/originaldata"
	"github.com/bradhannah/UltimaVRedux/internal/platform/texteditor"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

type dataSetupEditorStub struct {
	result chan error
}

func (launcher dataSetupEditorStub) Open(string) (<-chan error, error) {
	return launcher.result, nil
}

func TestDataSetupSceneSuppressesBriefSearchProgress(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	scene.searching = true
	screen := newPresentationRaster(1280, 720)
	t.Cleanup(screen.Deallocate)

	scene.searchDisplayFrames = setupSearchDisplayDelay - 1
	scene.Draw(screen)
	assert.Equal(t, ucolor.EGABlack.ToRGBA(), screen.At(screen.Bounds().Dx()/2, screen.Bounds().Dy()/5))

	scene.searchDisplayFrames = setupSearchDisplayDelay
	scene.Draw(screen)
	assert.Contains(t, scene.heading.GetOutputStr(false), "LOCATE ULTIMA V GAME FILES")
	assert.Positive(t, countExactColor(screen, ucolor.EGAWhite), "progress must become visibly rendered after the delay")
}

func TestDataSetupSceneNoCandidatesOffersRecoveryAndFirstRunQuit(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	scene.discover = func(context.Context, originaldata.DiscoveryOptions) originaldata.Report {
		return originaldata.Report{RecommendedDirectory: originaldata.RecommendedDirectory(scene.host)}
	}
	game := NewGame(scene)
	t.Cleanup(scene.Dispose)

	updateDataSetupUntil(t, scene, game, func() bool { return scene.dialogStack.Count() == 1 })
	menu, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)
	assert.Equal(t, []string{
		"Open Settings File",
		"Check Again",
		"Show Search Details",
		"Quit",
	}, menu.ButtonTexts())

	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(game))
	assert.True(t, game.exitRequested)
}

func TestDataSetupSceneSingleCandidateRequiresConfirmation(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	candidate := originaldata.Candidate{
		Path: filepath.Join(t.TempDir(), "UltimaV"), Source: originaldata.SourceGOG, Label: "GOG installation",
	}
	scene.discover = func(context.Context, originaldata.DiscoveryOptions) originaldata.Report {
		return originaldata.Report{Candidates: []originaldata.Candidate{candidate}}
	}
	game := NewGame(scene)
	t.Cleanup(scene.Dispose)

	updateDataSetupUntil(t, scene, game, func() bool { return scene.dialogStack.Count() == 1 })
	menu, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)
	assert.Equal(t, []string{"Use This Installation", "Look Somewhere Else", "Quit"}, menu.ButtonTexts())
	assert.False(t, game.exitRequested)
}

func TestDataSetupSceneMultipleCandidatesPreselectsFirstVerifiedInstallation(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	first := originaldata.Candidate{
		Path: filepath.Join(t.TempDir(), "gog"), Source: originaldata.SourceGOG, Label: "GOG installation",
	}
	scene.discover = func(context.Context, originaldata.DiscoveryOptions) originaldata.Report {
		return originaldata.Report{Candidates: []originaldata.Candidate{
			first,
			{Path: filepath.Join(t.TempDir(), "personal"), Source: originaldata.SourceRecommended, Label: "Personal folder"},
		}}
	}
	game := NewGame(scene)
	t.Cleanup(scene.Dispose)

	updateDataSetupUntil(t, scene, game, func() bool { return scene.dialogStack.Count() == 1 })
	browser, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)
	require.True(t, ok)
	selected, selectedOK := browser.GetSelectedView()
	require.True(t, selectedOK)
	assert.Equal(t, first.Path, selected.ID)
	assert.False(t, selected.Newest)
	assert.Equal(t, first.Path, selected.ReviewValue)
}

func TestDataSetupSceneTitleCandidatesPreselectAndMarkActiveInstallation(t *testing.T) {
	scene, configuration := newDataSetupTestScene(t, dataSetupFromTitle)
	first := originaldata.Candidate{Path: filepath.Join(t.TempDir(), "pending"), Label: "GOG installation"}
	active := originaldata.Candidate{Path: filepath.Join(t.TempDir(), "active"), Label: "Portable copy"}
	configuration.SavedConfigData.DataFilePath = active.Path
	require.NoError(t, configuration.PersistDataFilePath(first.Path))

	scene.showCandidateBrowser(&Game{}, []originaldata.Candidate{first, active}, "")
	browser, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)
	require.True(t, ok)
	selected, selectedOK := browser.GetSelectedView()
	require.True(t, selectedOK)
	assert.Equal(t, active.Path, selected.ID)
	assert.True(t, selected.Newest)
	assert.Equal(t, []widgets.SaveSlotDetail{
		{Label: "Folder", Value: active.Path},
		{Label: setupSourceDetailLabel, Value: ""},
		{Label: "Session", Value: "Active this session"},
		{Label: "Next launch", Value: "Not selected"},
	}, selected.Details)
}

func TestDataSetupSceneCandidateValidationFailureReturnsToSameChooser(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	first := originaldata.Candidate{Path: filepath.Join(t.TempDir(), "gog"), Label: "GOG installation"}
	failed := originaldata.Candidate{Path: filepath.Join(t.TempDir(), "portable"), Label: "Portable copy"}
	scene.validate = func(context.Context, string) error { return errors.New("files changed") }
	scene.showCandidateBrowser(&Game{}, []originaldata.Candidate{first, failed}, "")
	scene.candidateSelectionID = failed.Path
	scene.useCandidate(&Game{}, failed)

	updateDataSetupUntil(t, scene, &Game{}, func() bool {
		_, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)

		return ok
	})
	browser, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)
	require.True(t, ok)
	selected, selectedOK := browser.GetSelectedView()
	require.True(t, selectedOK)
	assert.Equal(t, failed.Path, selected.ID)
	assert.Contains(t, scene.status, "no longer usable")
}

func TestDataSetupSceneCancelsSearchWithoutAcceptingLateResult(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	searchStarted := make(chan struct{})
	scene.discover = func(ctx context.Context, _ originaldata.DiscoveryOptions) originaldata.Report {
		close(searchStarted)
		<-ctx.Done()

		return originaldata.Report{
			Candidates: []originaldata.Candidate{{Path: t.TempDir(), Source: originaldata.SourceCommon, Label: "late"}},
		}
	}
	game := NewGame(scene)
	t.Cleanup(scene.Dispose)
	<-searchStarted

	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(game))
	require.False(t, scene.searching)
	menu, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)
	assert.Contains(t, menu.ButtonTexts(), "Check Again")

	// Update on this goroutine: testify's Never polls from other goroutines, which
	// can still be running scene.Update when cleanup disposes the scene.
	for range 10 {
		time.Sleep(5 * time.Millisecond)
		require.NoError(t, scene.Update(game))
		_, changed := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)
		require.False(t, changed, "a cancelled search must not deliver its late result")
	}
}

func TestDataSetupSceneRetrySupersedesCancelledSearch(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	firstStarted := make(chan struct{})
	firstCancelled := make(chan struct{})
	secondPath := filepath.Join(t.TempDir(), "second")
	calls := 0
	scene.discover = func(ctx context.Context, _ originaldata.DiscoveryOptions) originaldata.Report {
		calls++
		if calls == 1 {
			close(firstStarted)
			<-ctx.Done()
			close(firstCancelled)

			return originaldata.Report{}
		}

		return originaldata.Report{Candidates: []originaldata.Candidate{{
			Path: secondPath, Source: originaldata.SourceRecommended, Label: "second search",
		}}}
	}
	game := NewGame(scene)
	t.Cleanup(scene.Dispose)
	<-firstStarted

	scene.startSearch()
	updateDataSetupUntil(t, scene, game, func() bool {
		return len(scene.report.Candidates) == 1 && scene.report.Candidates[0].Label == "second search"
	})

	select {
	case <-firstCancelled:
	case <-time.After(time.Second):
		t.Fatal("superseded search did not receive cancellation")
	}
}

func TestDataSetupSceneFirstRunLoadsCandidateBeforePersistingAndContinuing(t *testing.T) {
	scene, configuration := newDataSetupTestScene(t, dataSetupFirstRun)
	selected := filepath.Join(t.TempDir(), "UltimaV")
	playable := new(config.UltimaVConfiguration)
	scene.loadCandidate = func(string) (*config.UltimaVConfiguration, error) { return playable, nil }
	var received *config.UltimaVConfiguration
	scene.onReady = func(_ *Game, configuration *config.UltimaVConfiguration) error {
		received = configuration

		return nil
	}
	game := &Game{}

	scene.useCandidate(game, originaldata.Candidate{Path: selected})
	updateDataSetupUntil(t, scene, game, func() bool { return received != nil })

	assert.Equal(t, selected, configuration.PersistedDataFilePath())
	assert.Same(t, playable, received)
	assert.Equal(t, selected, received.PersistedDataFilePath())
}

func TestDataSetupSceneRevalidatesCandidateBeforePersisting(t *testing.T) {
	scene, configuration := newDataSetupTestScene(t, dataSetupFromTitle)
	previous := configuration.PersistedDataFilePath()
	scene.validate = func(context.Context, string) error { return errors.New("files changed") }
	game := &Game{}

	scene.useCandidate(game, originaldata.Candidate{Path: filepath.Join(t.TempDir(), "UltimaV")})
	updateDataSetupUntil(t, scene, game, func() bool { return strings.Contains(scene.status, "no longer usable") })

	assert.Equal(t, previous, configuration.PersistedDataFilePath())
	assert.Contains(t, scene.status, "no longer usable")
}

func TestDataSetupSceneCandidateValidationIsAsyncAndCancelable(t *testing.T) {
	scene, configuration := newDataSetupTestScene(t, dataSetupFirstRun)
	previous := configuration.PersistedDataFilePath()
	started := make(chan struct{})
	scene.validate = func(ctx context.Context, _ string) error {
		close(started)
		<-ctx.Done()

		return ctx.Err()
	}
	game := &Game{}
	candidate := originaldata.Candidate{Path: filepath.Join(t.TempDir(), "UltimaV"), Label: "Selected folder"}

	scene.useCandidate(game, candidate)
	<-started
	require.True(t, scene.validating)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(game))

	assert.False(t, scene.validating)
	assert.Equal(t, previous, configuration.PersistedDataFilePath())
	assert.Contains(t, scene.status, "cancelled")
	menu, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)
	assert.Contains(t, menu.ButtonTexts(), "Use This Installation")
}

func TestDataSetupSceneTitleSelectionChangesOnlyNextLaunchPath(t *testing.T) {
	scene, configuration := newDataSetupTestScene(t, dataSetupFromTitle)
	activePath := configuration.SavedConfigData.DataFilePath
	selected := filepath.Join(t.TempDir(), "UltimaV")
	game := &Game{}

	scene.useCandidate(game, originaldata.Candidate{Path: selected})
	updateDataSetupUntil(t, scene, game, func() bool { return scene.dialogStack.Count() == 1 })

	assert.Equal(t, activePath, configuration.SavedConfigData.DataFilePath)
	assert.Equal(t, selected, configuration.PersistedDataFilePath())
	menu, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)
	assert.Equal(t, []string{"Quit Now", "Return to Menu"}, menu.ButtonTexts())
}

func TestDataSetupSceneTitleSelectionDoesNotRewriteAlreadyActivePersistedPath(t *testing.T) {
	scene, configuration := newDataSetupTestScene(t, dataSetupFromTitle)
	path, err := configuration.ConfigurationFilePath()
	require.NoError(t, err)
	active := filepath.Join(t.TempDir(), "active")
	configuration.SavedConfigData.DataFilePath = active
	contents := []byte("# preserve formatting\nDataFilePath: '" + active + "'\n")
	require.NoError(t, os.WriteFile(path, contents, 0o600))
	game := &Game{}
	candidate := originaldata.Candidate{Path: configuration.SavedConfigData.DataFilePath}

	scene.useCandidate(game, candidate)
	updateDataSetupUntil(t, scene, game, func() bool { return scene.dialogStack.Count() == 1 })

	assert.NotContains(t, scene.status, "could not be saved")
	menu, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)
	assert.Equal(t, []string{"Return to Menu"}, menu.ButtonTexts())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, contents, after)
}

func TestDataSetupSceneTitleSelectingActivePathClearsPendingChange(t *testing.T) {
	scene, configuration := newDataSetupTestScene(t, dataSetupFromTitle)
	active := filepath.Join(t.TempDir(), "active")
	pending := filepath.Join(t.TempDir(), "pending")
	configuration.SavedConfigData.DataFilePath = active
	require.NoError(t, configuration.PersistDataFilePath(pending))
	game := &Game{}

	scene.useCandidate(game, originaldata.Candidate{Path: active})
	updateDataSetupUntil(t, scene, game, func() bool { return scene.dialogStack.Count() == 1 })

	assert.Equal(t, active, configuration.PersistedDataFilePath())
	menu, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)
	assert.Equal(t, []string{setupReturnToMenuLabel}, menu.ButtonTexts())
}

func TestDataSetupScenePreservesExplicitOldWithoutAutomaticPortableSeeds(t *testing.T) {
	scene, configuration := newDataSetupTestScene(t, dataSetupFirstRun)
	path, err := configuration.ConfigurationFilePath()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("DataFilePath: ./OLD\n"), 0o600))
	require.NoError(t, configuration.RefreshPersistedConfiguration())
	configuration.SavedConfigData.DataFilePath = "./OLD"
	options := make(chan originaldata.DiscoveryOptions, 1)
	scene.discover = func(_ context.Context, discovered originaldata.DiscoveryOptions) originaldata.Report {
		options <- discovered

		return originaldata.Report{}
	}
	game := NewGame(scene)
	t.Cleanup(scene.Dispose)

	select {
	case discovered := <-options:
		assert.Equal(t, "./OLD", discovered.ConfiguredDirectory)
		assert.Equal(t, "./OLD", discovered.PreviousDataDirectory)
		assert.Empty(t, discovered.AdditionalSeeds)
	case <-time.After(time.Second):
		t.Fatal("data discovery did not start")
	}
	assert.NotNil(t, game.currentScene)
}

func TestDataSetupSceneFirstRunReloadFailureReturnsToRecovery(t *testing.T) {
	scene, configuration := newDataSetupTestScene(t, dataSetupFirstRun)
	previous := configuration.PersistedDataFilePath()
	scene.loadCandidate = func(string) (*config.UltimaVConfiguration, error) {
		return nil, errors.New("reload failed")
	}
	game := &Game{}

	scene.useCandidate(game, originaldata.Candidate{Path: filepath.Join(t.TempDir(), "UltimaV")})
	updateDataSetupUntil(t, scene, game, func() bool { return strings.Contains(scene.status, "reload failed") })

	assert.Contains(t, scene.status, "reload failed")
	assert.Equal(t, previous, configuration.PersistedDataFilePath())
	_, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	assert.True(t, ok)
}

func TestDataSetupSceneReportsAsynchronousEditorFailure(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	result := make(chan error, 1)
	scene.editor = dataSetupEditorStub{result: result}
	game := &Game{}
	scene.showSettingsRecovery(game, true)
	result <- errors.New("helper exited")
	close(result)

	require.NoError(t, scene.Update(game))

	assert.Contains(t, scene.status, "helper exited")
	assert.True(t, scene.settingsVisible)
	assert.Equal(t, 1, scene.dialogStack.Count())
}

func TestDataSetupSceneValidationDropsLateEditorFailure(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	result := make(chan error, 1)
	scene.editorResult = result
	started := make(chan struct{})
	scene.beginValidation(dataSetupCheckSettings, "Checking settings...", func(ctx context.Context) dataSetupValidationResult {
		close(started)
		<-ctx.Done()

		return dataSetupValidationResult{err: ctx.Err()}
	})
	<-started
	result <- errors.New("late editor failure")

	require.NoError(t, scene.Update(&Game{}))

	assert.True(t, scene.validating)
	assert.False(t, scene.settingsVisible)
	assert.Zero(t, scene.dialogStack.Count())
	assert.NotContains(t, scene.status, "late editor failure")
	scene.stopValidation()
}

func TestDataSetupSceneAlternativeActionsHaveTruthfulBackNavigation(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	game := &Game{}
	scene.report.Candidates = []originaldata.Candidate{{Path: "/games/ultima-v", Label: "GOG installation"}}
	scene.showAlternativeActions(game)

	menu, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)
	assert.Equal(t, []string{"Open Settings File", "Back"}, menu.ButtonTexts())
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(game))

	menu, ok = scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)
	assert.Contains(t, menu.ButtonTexts(), "Use This Installation")
	assert.False(t, game.exitRequested)
}

func TestDataSetupSceneSearchDetailsIncludeStartupErrorsRootsAndLimits(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	scene.configurationError = errors.New("invalid settings")
	scene.originalDataError = errors.New("missing DATA.OVL")
	scene.report.CheckedRoots = []originaldata.Seed{{Path: "/games/ultima-v", Source: originaldata.SourceGOG, Label: "GOG record"}}
	scene.report.Details = []originaldata.Detail{{
		Path: "Discovery catalogue", Source: originaldata.SourceCommon, Reason: "bounded search stopped",
	}}

	views := scene.searchDetailViews()

	require.Len(t, views, 4)
	assert.Equal(t, "Settings", views[0].Label)
	assert.Equal(t, "/games/ultima-v", views[2].Details[0].Value)
	assert.Contains(t, views[3].Problem, "bounded search stopped")
}

func TestDataSetupSceneCheckAgainRefreshesExternallyEditedPath(t *testing.T) {
	scene, configuration := newDataSetupTestScene(t, dataSetupFirstRun)
	path, err := configuration.ConfigurationFilePath()
	require.NoError(t, err)
	selected := filepath.Join(t.TempDir(), "external edit")
	require.NoError(t, os.WriteFile(path, []byte("DataFilePath: '"+selected+"'\n"), 0o600))
	options := make(chan originaldata.DiscoveryOptions, 1)
	scene.discover = func(_ context.Context, discovered originaldata.DiscoveryOptions) originaldata.Report {
		options <- discovered

		return originaldata.Report{}
	}

	scene.checkAgain(&Game{})

	assert.Equal(t, selected, (<-options).PreviousDataDirectory)
	scene.stopSearch()
}

func TestDataSetupSceneNestedEscapeReturnsToRecoveryActions(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFirstRun)
	game := &Game{}
	scene.showSearchDetails(game)

	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(game))

	menu, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)
	assert.Contains(t, menu.ButtonTexts(), "Check Again")
	assert.False(t, game.exitRequested)
}

func TestDataSetupSceneTitleSearchEscapeReturnsToMenu(t *testing.T) {
	scene, _ := newDataSetupTestScene(t, dataSetupFromTitle)
	returned := 0
	scene.onReturn = func(*Game) error {
		returned++

		return nil
	}
	scene.searching = true
	scene.cancelSearch = func() {}
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)

	require.NoError(t, scene.Update(&Game{}))

	assert.Equal(t, 1, returned)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestFilepathEqualForOSIgnoresWindowsCase(t *testing.T) {
	assert.True(t, filepathEqualForOS(filepath.Join("Games", "UltimaV"), filepath.Join("games", "ultimav"), "windows"))
	assert.False(t, filepathEqualForOS(filepath.Join("Games", "UltimaV"), filepath.Join("games", "ultimav"), "linux"))
}

func newDataSetupTestScene(
	t *testing.T,
	mode dataSetupMode,
) (*dataSetupScene, *config.UltimaVConfiguration) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	startup := config.LoadStartupConfiguration()
	require.NotNil(t, startup.Configuration)
	require.NoError(t, startup.ConfigurationError)
	host := originaldata.HostProfile{
		GOOS: "linux", HomeDirectory: home, Environment: map[string]string{},
	}
	ready := func(*Game, *config.UltimaVConfiguration) error { return nil }
	back := func(*Game) error { return nil }
	scene := newDataSetupScene(startup.Configuration, nil, startup.OriginalDataError, host, mode, false, ready, back)
	scene.keyboard = newPresentationTestKeyboard()
	scene.validate = func(context.Context, string) error { return nil }

	return scene, startup.Configuration
}

func updateDataSetupUntil(
	t *testing.T,
	scene *dataSetupScene,
	game *Game,
	condition func() bool,
) {
	t.Helper()
	require.Eventually(t, func() bool {
		require.NoError(t, scene.Update(game))

		return condition()
	}, time.Second, time.Millisecond)
}

var _ texteditor.Launcher = dataSetupEditorStub{}
