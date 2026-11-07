package display

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBackend struct {
	windowWidth, windowHeight   int
	monitorWidth, monitorHeight int
	windowX, windowY            int
	fullscreen                  bool
	maximized                   bool
	requestedFullscreen         bool
	requestedWidth              int
	requestedHeight             int
	positionReads               int
	positionCalls               int
	restoreCalls                int
	operations                  []string
	scale                       float64
	limits                      [4]int
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		windowWidth:   1280,
		windowHeight:  720,
		monitorWidth:  1920,
		monitorHeight: 1080,
	}
}

func (f *fakeBackend) WindowSize() (int, int)     { return f.windowWidth, f.windowHeight }
func (f *fakeBackend) MonitorSize() (int, int)    { return f.monitorWidth, f.monitorHeight }
func (f *fakeBackend) DeviceScaleFactor() float64 { return ValidScale(f.scale) }
func (f *fakeBackend) WindowPosition() (int, int) {
	f.positionReads++

	return f.windowX, f.windowY
}
func (f *fakeBackend) SetWindowPosition(x, y int) {
	f.windowX, f.windowY = x, y
	f.positionCalls++
}
func (f *fakeBackend) SetWindowSize(width, height int) {
	f.requestedWidth, f.requestedHeight = width, height
	f.operations = append(f.operations, "size")
}
func (f *fakeBackend) SetWindowResizingEnabled() {
	f.operations = append(f.operations, "resizable")
}
func (f *fakeBackend) SetWindowSizeLimits(minw, minh, maxw, maxh int) {
	f.limits = [4]int{minw, minh, maxw, maxh}
	f.operations = append(f.operations, "limits")
}
func (f *fakeBackend) SetFullscreen(fullscreen bool) {
	f.requestedFullscreen = fullscreen
	f.operations = append(f.operations, "fullscreen")
}
func (f *fakeBackend) IsFullscreen() bool { return f.fullscreen }
func (f *fakeBackend) MaximizeWindow() {
	f.operations = append(f.operations, "maximize")
}
func (f *fakeBackend) IsWindowMaximized() bool { return f.maximized }
func (f *fakeBackend) RestoreWindow() {
	f.restoreCalls++
	f.operations = append(f.operations, "restore")
}

func TestModeParsingPreservesValidatedIdentity(t *testing.T) {
	for _, mode := range []config.Mode{config.ModeWindowed, config.ModeMaximized, config.ModeFullscreen} {
		parsed, err := config.ParseMode(mode.String())
		require.NoError(t, err)
		assert.Equal(t, mode, parsed)
		assert.True(t, parsed.IsValid())
	}

	for _, value := range []string{"", "Windowed", "borderless", " windowed"} {
		_, err := config.ParseMode(value)
		require.ErrorIs(t, err, config.ErrInvalidMode)
	}
}

func TestGetManagerHasCanonicalIdentity(t *testing.T) {
	manager := GetManager()
	secondLookup := GetManager()
	assert.Same(t, manager, secondLookup)
}

func TestConfigureResizableSetsEnabledAndMinimumLimits(t *testing.T) {
	backend := newFakeBackend()
	manager := newDisplayManager(backend)

	manager.ConfigureResizable()

	assert.Equal(t, []string{"resizable", "limits"}, backend.operations)
}

func TestLegacyWindowSizeReportsRejectedRequest(t *testing.T) {
	logger := logging.GetGlobalLogger()
	enabled, level, categories := logger.IsEnabled(), logger.GetMinLevel(), logger.GetCategoryStates()
	t.Cleanup(func() {
		logger.SetEnabled(enabled)
		logger.SetMinLevel(level)
		logger.SetCategoryStates(categories)
	})
	logger.SetEnabled(true)
	logger.SetMinLevel(logging.WARN)
	logger.SetCategoryEnabled(logging.CategoryUI, true)
	cursor := logger.EntriesAfter(0, 1).NewestSequence
	backend := newFakeBackend()
	manager := newDisplayManager(backend)

	manager.SetWindowSize(config.MinimumWindowWidth-1, config.MinimumWindowHeight)

	assert.Empty(t, backend.operations)
	assert.False(t, manager.pending)
	entries := logger.EntriesAfter(cursor, 1).Entries
	require.Len(t, entries, 1)
	assert.Equal(t, "WARN", entries[0].Level)
	assert.Equal(t, logging.CategoryUI, entries[0].Category)
	assert.Contains(t, entries[0].Message, ErrWindowSizeBelowMinimum.Error())
}

func TestInitializeModeAvoidsRuntimeWindowPosition(t *testing.T) {
	tests := []struct {
		mode       config.Mode
		operations []string
	}{
		{mode: config.ModeWindowed, operations: []string{"resizable", "limits", "size", "fullscreen"}},
		{mode: config.ModeMaximized, operations: []string{"resizable", "limits", "size", "fullscreen", "maximize"}},
		{mode: config.ModeFullscreen, operations: []string{"resizable", "limits", "size", "fullscreen"}},
	}
	for _, test := range tests {
		t.Run(test.mode.String(), func(t *testing.T) {
			backend := newFakeBackend()
			backend.windowWidth = 640
			backend.windowHeight = 480
			manager := newDisplayManager(backend)

			require.NoError(t, manager.InitializeMode(test.mode, 1600, 900))

			assert.Equal(t, test.operations, backend.operations)
			assert.Zero(t, backend.positionReads)
			assert.Zero(t, backend.positionCalls)
			assert.Equal(t, test.mode, manager.requestedMode)
			assert.Equal(t, []int{1600, 900}, pair(manager.LastNormalWindowSize()))
			assert.Equal(t, []int{1600, 900}, pair(manager.PersistableWindowSize()))
			assert.True(t, manager.IsTransitionPending())
		})
	}
}

func TestRequestModeWaitsForDelayedBackendObservation(t *testing.T) {
	backend := newFakeBackend()
	manager := newDisplayManager(backend)

	require.NoError(t, manager.RequestMode(config.ModeFullscreen, 1280, 720))
	assert.Equal(t, config.ModeWindowed, manager.Mode())
	assert.Equal(t, config.ModeFullscreen, manager.requestedMode)
	assert.True(t, manager.IsTransitionPending())
	assert.False(t, manager.Update())
	assert.True(t, manager.IsTransitionPending())

	backend.fullscreen = true
	assert.True(t, manager.Update())
	assert.Equal(t, config.ModeFullscreen, manager.Mode())
	assert.False(t, manager.IsTransitionPending())
}

func TestUnobservedRequestTimesOutAndCanBeRetried(t *testing.T) {
	backend := newFakeBackend()
	manager := newDisplayManager(backend)

	require.NoError(t, manager.RequestMode(config.ModeFullscreen, 1280, 720))
	for range TransitionTimeoutUpdates {
		manager.Update()
	}

	assert.False(t, manager.IsTransitionPending())
	assert.Equal(t, config.ModeWindowed, manager.requestedMode)
	require.NoError(t, manager.RequestMode(config.ModeMaximized, 1280, 720))
	assert.True(t, manager.IsTransitionPending())
}

func TestEveryModeTransitionUsesObservedBackendState(t *testing.T) {
	backend := newFakeBackend()
	manager := newDisplayManager(backend)

	require.NoError(t, manager.RequestMode(config.ModeMaximized, 1280, 720))
	assert.Equal(t, []string{"resizable", "limits", "fullscreen", "maximize"}, backend.operations)
	assert.True(t, manager.IsTransitionPending())
	backend.maximized = true
	manager.Update()
	assert.Equal(t, config.ModeMaximized, manager.Mode())
	assert.False(t, manager.IsTransitionPending())

	require.NoError(t, manager.RequestMode(config.ModeWindowed, 1600, 900))
	assert.Equal(t, 1, backend.restoreCalls)
	backend.maximized = false
	backend.windowWidth, backend.windowHeight = 1600, 900
	manager.Update()
	assert.Equal(t, config.ModeWindowed, manager.Mode())
	assert.False(t, manager.IsTransitionPending())

	require.NoError(t, manager.RequestMode(config.ModeFullscreen, 1600, 900))
	backend.fullscreen = true
	manager.Update()
	assert.Equal(t, config.ModeFullscreen, manager.Mode())
}

func TestDelayedNonWindowedTransitionRetainsRequestedNormalSize(t *testing.T) {
	backend := newFakeBackend()
	manager := newDisplayManager(backend)

	require.NoError(t, manager.RequestMode(config.ModeMaximized, 1600, 900))
	manager.Update()
	assert.Equal(t, []int{1600, 900}, pair(manager.LastNormalWindowSize()))

	backend.maximized = true
	manager.Update()
	assert.Equal(t, []int{1600, 900}, pair(manager.LastNormalWindowSize()))
}

func TestMaximizeWaitsForFullscreenExitAndEnablesResizingFirst(t *testing.T) {
	backend := newFakeBackend()
	backend.fullscreen = true
	manager := newDisplayManager(backend)

	require.NoError(t, manager.RequestMode(config.ModeMaximized, 1280, 720))
	assert.Equal(t, []string{"resizable", "limits", "fullscreen"}, backend.operations)

	backend.fullscreen = false
	manager.Update()
	assert.Equal(t, []string{
		"resizable", "limits", "fullscreen", "resizable", "limits", "maximize",
	}, backend.operations)
	assert.True(t, manager.IsTransitionPending())

	backend.maximized = true
	manager.Update()
	assert.False(t, manager.IsTransitionPending())
}

func TestWindowedPresetChangePreservesCenter(t *testing.T) {
	backend := newFakeBackend()
	backend.windowX, backend.windowY = 100, 200
	manager := newDisplayManager(backend)

	require.NoError(t, manager.RequestMode(config.ModeWindowed, 1600, 900))

	assert.Equal(t, -60, backend.windowX)
	assert.Equal(t, 110, backend.windowY)
	assert.Equal(t, 1, backend.positionCalls)
}

func TestOrdinaryWindowDragDoesNotRecenter(t *testing.T) {
	backend := newFakeBackend()
	manager := newDisplayManager(backend)

	backend.windowWidth, backend.windowHeight = 1500, 800
	assert.True(t, manager.Update())

	assert.Zero(t, backend.positionCalls)
	assert.Equal(t, 1500, manager.lastNormalWidth)
	assert.Equal(t, 800, manager.lastNormalHeight)
	assert.False(t, manager.IsTransitionPending())
}

func TestRequestedNormalSizeIsRetainedWhenLeavingWindowed(t *testing.T) {
	backend := newFakeBackend()
	backend.windowWidth, backend.windowHeight = 1500, 800
	manager := newDisplayManager(backend)

	require.NoError(t, manager.RequestMode(config.ModeFullscreen, 1600, 900))

	width, height := manager.LastNormalWindowSize()
	assert.Equal(t, 1600, width)
	assert.Equal(t, 900, height)
}

func TestRuntimeRequestDoesNotChangePersistableWindowSize(t *testing.T) {
	backend := newFakeBackend()
	manager := newDisplayManager(backend)

	require.NoError(t, manager.RequestMode(config.ModeWindowed, 1600, 900))
	backend.windowWidth, backend.windowHeight = 1600, 900
	manager.Update()
	assert.Equal(t, []int{1280, 720}, pair(manager.PersistableWindowSize()))

	backend.windowWidth, backend.windowHeight = 1700, 950
	manager.Update()
	assert.Equal(t, []int{1700, 950}, pair(manager.PersistableWindowSize()))

	manager.CommitNormalWindowSize(1600, 900)
	assert.Equal(t, []int{1600, 900}, pair(manager.PersistableWindowSize()))
}

func TestRestoreIsGuardedByObservedMaximizedState(t *testing.T) {
	backend := newFakeBackend()
	manager := newDisplayManager(backend)

	require.NoError(t, manager.RequestMode(config.ModeWindowed, 1280, 720))
	assert.Zero(t, backend.restoreCalls)

	backend.maximized = true
	manager.Update()
	require.NoError(t, manager.RequestMode(config.ModeWindowed, 1280, 720))
	assert.Equal(t, 1, backend.restoreCalls)
}

func TestLogicalWindowMonitorAndContainerSizesRemainDistinct(t *testing.T) {
	backend := newFakeBackend()
	backend.windowWidth, backend.windowHeight = 1600, 1000
	backend.monitorWidth, backend.monitorHeight = 3440, 1440
	backend.fullscreen = true
	manager := newDisplayManager(backend)

	assert.Equal(t, []int{1600, 1000}, []int{manager.windowWidth, manager.windowHeight})
	assert.Equal(t, []int{3440, 1440}, pair(manager.MonitorSize()))
	assert.Equal(t, []int{2560, 1440}, pair(manager.GetCurrentSize()))
	assert.Equal(t, []int{0, 0}, pair(manager.LogicalSize()))

	manager.PublishLogicalSize(2000, 1125)
	assert.Equal(t, []int{2000, 1125}, pair(manager.LogicalSize()))
	assert.Equal(t, []int{2000, 1125}, pair(manager.GetCurrentSize()))
}

func TestCurrentSizeFallbackAndTinyPublicationStayPositive(t *testing.T) {
	backend := newFakeBackend()
	backend.windowWidth, backend.windowHeight = 0, 0
	backend.monitorWidth, backend.monitorHeight = 0, 0
	manager := newDisplayManager(backend)

	assert.Equal(t, []int{1, 1}, pair(manager.GetCurrentSize()))
	assert.Equal(t, []int{config.MinimumWindowWidth, config.MinimumWindowHeight}, pair(manager.LastNormalWindowSize()))

	manager.PublishLogicalSize(0, -10)
	assert.Equal(t, []int{1, 1}, pair(manager.GetCurrentSize()))
}

func TestRequestModeRejectsInvalidModeAndUndersizedWindow(t *testing.T) {
	manager := newDisplayManager(newFakeBackend())

	require.ErrorIs(t, manager.RequestMode(config.Mode("invalid"), 1280, 720), config.ErrInvalidMode)
	require.Error(t, manager.RequestMode(config.ModeWindowed, config.MinimumWindowWidth-1, config.MinimumWindowHeight))
	require.Error(t, manager.RequestMode(config.ModeWindowed, config.MinimumWindowWidth, config.MinimumWindowHeight-1))
}

func pair(first, second int) []int {
	return []int{first, second}
}
