package display

import (
	"errors"
	"fmt"
	"image"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/config"
)

const (
	// TransitionTimeoutUpdates bounds an OS display request that is never observed.
	TransitionTimeoutUpdates = 180
	aspectWidth              = 16
	aspectHeight             = 9
)

var (
	// ErrWindowSizeBelowMinimum identifies unsupported normal window dimensions.
	ErrWindowSizeBelowMinimum = errors.New("window size is below the supported minimum")
)

//nolint:interfacebloat // This cohesive boundary mirrors the desktop-window API used by the manager.
type backend interface {
	WindowSize() (int, int)
	MonitorSize() (int, int)
	DeviceScaleFactor() float64
	WindowPosition() (int, int)
	SetWindowPosition(x, y int)
	SetWindowSize(width, height int)
	SetWindowResizingEnabled()
	SetWindowSizeLimits(minWidth, minHeight, maxWidth, maxHeight int)
	SetFullscreen(fullscreen bool)
	IsFullscreen() bool
	MaximizeWindow()
	IsWindowMaximized() bool
	RestoreWindow()
}

type ebitenBackend struct{}

func (ebitenBackend) WindowSize() (int, int)  { return ebiten.WindowSize() }
func (ebitenBackend) MonitorSize() (int, int) { return ebiten.Monitor().Size() }
func (ebitenBackend) DeviceScaleFactor() float64 {
	if monitor := ebiten.Monitor(); monitor != nil {
		return monitor.DeviceScaleFactor()
	}

	return 1
}
func (ebitenBackend) WindowPosition() (int, int) { return ebiten.WindowPosition() }
func (ebitenBackend) SetWindowPosition(x, y int) { ebiten.SetWindowPosition(x, y) }
func (ebitenBackend) SetWindowSize(w, h int)     { ebiten.SetWindowSize(w, h) }
func (ebitenBackend) SetWindowResizingEnabled() {
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
}
func (ebitenBackend) SetWindowSizeLimits(a, b, c, d int) {
	ebiten.SetWindowSizeLimits(a, b, c, d)
}
func (ebitenBackend) SetFullscreen(fullscreen bool) { ebiten.SetFullscreen(fullscreen) }
func (ebitenBackend) IsFullscreen() bool            { return ebiten.IsFullscreen() }
func (ebitenBackend) MaximizeWindow()               { ebiten.MaximizeWindow() }
func (ebitenBackend) IsWindowMaximized() bool       { return ebiten.IsWindowMaximized() }
func (ebitenBackend) RestoreWindow()                { ebiten.RestoreWindow() }

// DisplayManager owns desktop display requests and observes their actual result.
//
//nolint:revive // Preserve the established display boundary's public name.
type DisplayManager struct {
	backend backend

	mode          config.Mode
	requestedMode config.Mode
	pending       bool
	requestActive bool
	requestTicks  int

	windowWidth     int
	windowHeight    int
	monitorWidth    int
	monitorHeight   int
	logicalWidth    int
	logicalHeight   int
	logicalDirty    bool
	deviceScale     float64
	containerWidth  int
	containerHeight int
	outputWidth     int
	outputHeight    int

	requestedWindowWidth  int
	requestedWindowHeight int
	lastNormalWidth       int
	lastNormalHeight      int
	persistableWidth      int
	persistableHeight     int
	maximizeIssued        bool

	mapPopupViewportProvider func(image.Rectangle) image.Rectangle
}

var (
	//nolint:gochecknoglobals // One process-wide display identity is the accepted application boundary.
	globalManager *DisplayManager
	//nolint:gochecknoglobals // Synchronizes construction of that process-wide identity.
	once sync.Once
)

// GetManager returns the canonical process display manager.
func GetManager() *DisplayManager {
	once.Do(func() {
		globalManager = newDisplayManager()
	})

	return globalManager
}

func newDisplayManager(backends ...backend) *DisplayManager {
	displayBackend := backend(ebitenBackend{})
	if len(backends) > 0 {
		displayBackend = backends[0]
	}
	windowWidth, windowHeight := displayBackend.WindowSize()
	monitorWidth, monitorHeight := displayBackend.MonitorSize()
	mode := observedMode(displayBackend)
	lastWidth, lastHeight := validNormalSize(windowWidth, windowHeight)

	//nolint:exhaustruct_v5 // Transition counters and unpublished frame metrics intentionally start at zero.
	return &DisplayManager{
		backend:               displayBackend,
		mode:                  mode,
		requestedMode:         mode,
		windowWidth:           windowWidth,
		windowHeight:          windowHeight,
		monitorWidth:          monitorWidth,
		monitorHeight:         monitorHeight,
		deviceScale:           ValidScale(displayBackend.DeviceScaleFactor()),
		requestedWindowWidth:  lastWidth,
		requestedWindowHeight: lastHeight,
		lastNormalWidth:       lastWidth,
		lastNormalHeight:      lastHeight,
		persistableWidth:      lastWidth,
		persistableHeight:     lastHeight,
	}
}

func observedMode(b backend) config.Mode {
	if b.IsFullscreen() {
		return config.ModeFullscreen
	}
	if b.IsWindowMaximized() {
		return config.ModeMaximized
	}

	return config.ModeWindowed
}

func validNormalSize(width, height int) (int, int) {
	if width < config.MinimumWindowUnitsWidth || height < config.MinimumWindowUnitsHeight {
		return config.MinimumWindowWidth, config.MinimumWindowHeight
	}

	return width, height
}

// ConfigureResizable enables decorated window resizing with the supported minimum size.
func (dm *DisplayManager) ConfigureResizable() {
	dm.backend.SetWindowResizingEnabled()
	width, height := WindowMinimum(dm.monitorWidth, dm.monitorHeight, dm.deviceScale)
	dm.backend.SetWindowSizeLimits(width, height, -1, -1)
}

// PublishLogicalSize publishes the authoritative draw-surface dimensions.
func (dm *DisplayManager) PublishLogicalSize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if width == dm.logicalWidth && height == dm.logicalHeight {
		return
	}

	dm.logicalWidth = width
	dm.logicalHeight = height
	dm.logicalDirty = true
}

// Update observes backend state and reports whether display state changed.
// It should be called once per frame from the game loop.
//
//nolint:cyclop // Observation, delayed maximize, completion, and notification form one transition.
func (dm *DisplayManager) Update() bool {
	windowWidth, windowHeight := dm.backend.WindowSize()
	monitorWidth, monitorHeight := dm.backend.MonitorSize()
	scale := ValidScale(dm.backend.DeviceScaleFactor())
	scaleChanged := scale != dm.deviceScale
	monitorChanged := monitorWidth != dm.monitorWidth || monitorHeight != dm.monitorHeight
	mode := observedMode(dm.backend)

	changed := mode != dm.mode ||
		windowWidth != dm.windowWidth || windowHeight != dm.windowHeight ||
		monitorWidth != dm.monitorWidth || monitorHeight != dm.monitorHeight ||
		dm.logicalDirty || scaleChanged

	dm.observeNormalWindow(mode, windowWidth, windowHeight)

	dm.mode = mode
	dm.windowWidth = windowWidth
	dm.windowHeight = windowHeight
	dm.monitorWidth = monitorWidth
	dm.monitorHeight = monitorHeight
	dm.deviceScale = scale
	if scaleChanged || monitorChanged {
		dm.ConfigureResizable()
	}
	dm.logicalDirty = false

	if dm.requestActive && dm.requestedMode == config.ModeMaximized &&
		mode == config.ModeWindowed && !dm.maximizeIssued {
		dm.ConfigureResizable()
		dm.backend.MaximizeWindow()
		dm.maximizeIssued = true
	}

	if dm.requestActive {
		dm.requestTicks++
		dm.pending = !dm.requestObserved()
		if !dm.pending {
			dm.requestActive = false
		} else if dm.requestTicks >= TransitionTimeoutUpdates {
			dm.cancelUnobservedRequest()
		}
	} else {
		dm.pending = false
	}

	return changed
}

func (dm *DisplayManager) observeNormalWindow(mode config.Mode, width, height int) {
	if mode != config.ModeWindowed || width < config.MinimumWindowUnitsWidth || height < config.MinimumWindowUnitsHeight {
		return
	}
	if !dm.requestActive || dm.requestedMode == config.ModeWindowed {
		dm.lastNormalWidth, dm.lastNormalHeight = width, height
	}
	if !dm.requestActive && (width != dm.windowWidth || height != dm.windowHeight) {
		dm.persistableWidth, dm.persistableHeight = width, height
	}
}

func (dm *DisplayManager) requestObserved() bool {
	if dm.mode != dm.requestedMode {
		return false
	}
	if dm.mode == config.ModeWindowed {
		return dm.windowWidth == dm.requestedWindowWidth &&
			dm.windowHeight == dm.requestedWindowHeight
	}

	return true
}

// GetCurrentSize returns logical dimensions, falling back to an aspect-safe
// positive fit of the outer container until Layout publishes a logical size.
func (dm *DisplayManager) GetCurrentSize() (int, int) {
	if dm.logicalWidth > 0 && dm.logicalHeight > 0 {
		return dm.logicalWidth, dm.logicalHeight
	}
	width, height := dm.backend.WindowSize()
	if dm.backend.IsFullscreen() {
		width, height = dm.backend.MonitorSize()
	}

	return aspectFit16By9(width, height)
}

// WindowSize returns the last observed decorated/restored window size.
func (dm *DisplayManager) WindowSize() (int, int) {
	return dm.windowWidth, dm.windowHeight
}

// MonitorSize returns the last observed current-monitor size.
func (dm *DisplayManager) MonitorSize() (int, int) {
	return dm.monitorWidth, dm.monitorHeight
}

// ContainerSize returns the current outer container dimensions.
func (dm *DisplayManager) ContainerSize() (int, int) {
	if dm.containerWidth > 0 && dm.containerHeight > 0 {
		return dm.containerWidth, dm.containerHeight
	}
	if dm.mode == config.ModeFullscreen {
		return dm.monitorWidth, dm.monitorHeight
	}

	return dm.windowWidth, dm.windowHeight
}

// LogicalSize returns the published logical draw size, or zeroes before publication.
func (dm *DisplayManager) LogicalSize() (int, int) {
	return dm.logicalWidth, dm.logicalHeight
}

// Mode returns the last observed desktop mode.
func (dm *DisplayManager) Mode() config.Mode {
	return dm.mode
}

// RequestedMode returns the mode most recently requested by the application.
func (dm *DisplayManager) RequestedMode() config.Mode {
	return dm.requestedMode
}

// IsTransitionPending reports whether the requested mode and size remain unobserved.
func (dm *DisplayManager) IsTransitionPending() bool {
	return dm.pending
}

// LastNormalWindowSize returns the most recently observed normal window size.
func (dm *DisplayManager) LastNormalWindowSize() (int, int) {
	return dm.lastNormalWidth, dm.lastNormalHeight
}

// PersistableWindowSize returns the latest player-dragged or explicitly
// committed normal size. Runtime-only requests do not change it.
func (dm *DisplayManager) PersistableWindowSize() (int, int) {
	return dm.persistableWidth, dm.persistableHeight
}

// CommitNormalWindowSize marks validated normal dimensions as player settings.
func (dm *DisplayManager) CommitNormalWindowSize(width, height int) {
	if width < config.MinimumWindowUnitsWidth || height < config.MinimumWindowUnitsHeight {
		panic("display: cannot commit an undersized normal window")
	}
	dm.persistableWidth = width
	dm.persistableHeight = height
}

// InitializeMode configures the initial desktop mode before Ebitengine's main
// loop starts. It avoids runtime-only reads such as WindowPosition.
func (dm *DisplayManager) InitializeMode(mode config.Mode, width, height int) error {
	if err := dm.validateModeRequest(mode, width, height); err != nil {
		return err
	}

	dm.ConfigureResizable()
	dm.backend.SetWindowSize(width, height)
	dm.beginRequest(mode, width, height)
	dm.lastNormalWidth = width
	dm.lastNormalHeight = height
	dm.CommitNormalWindowSize(width, height)

	switch mode {
	case config.ModeWindowed:
		dm.backend.SetFullscreen(false)
	case config.ModeMaximized:
		dm.backend.SetFullscreen(false)
		dm.backend.MaximizeWindow()
		dm.maximizeIssued = true
	case config.ModeFullscreen:
		dm.backend.SetFullscreen(true)
	}

	return nil
}

// RequestMode requests a validated mode transition and normal window size.
func (dm *DisplayManager) RequestMode(mode config.Mode, width, height int) error {
	if err := dm.validateModeRequest(mode, width, height); err != nil {
		return err
	}

	actualMode := observedMode(dm.backend)
	actualWidth, actualHeight := dm.backend.WindowSize()
	dm.beginRequest(mode, width, height)
	dm.lastNormalWidth = width
	dm.lastNormalHeight = height

	switch mode {
	case config.ModeWindowed:
		wasNormal := actualMode == config.ModeWindowed
		oldX, oldY := 0, 0
		if wasNormal {
			oldX, oldY = dm.backend.WindowPosition()
		}
		dm.backend.SetFullscreen(false)
		if dm.backend.IsWindowMaximized() {
			dm.backend.RestoreWindow()
		}
		dm.backend.SetWindowSize(width, height)
		if wasNormal && (actualWidth != width || actualHeight != height) {
			dm.backend.SetWindowPosition(
				oldX+(actualWidth-width)/2,
				oldY+(actualHeight-height)/2,
			)
		}
	case config.ModeMaximized:
		dm.ConfigureResizable()
		dm.backend.SetFullscreen(false)
		if !dm.backend.IsFullscreen() && !dm.backend.IsWindowMaximized() {
			dm.backend.MaximizeWindow()
			dm.maximizeIssued = true
		}
	case config.ModeFullscreen:
		dm.backend.SetFullscreen(true)
	}

	return nil
}

func (dm *DisplayManager) validateModeRequest(mode config.Mode, width, height int) error {
	if !mode.IsValid() {
		return fmt.Errorf("%w: %q", config.ErrInvalidMode, mode)
	}
	minWidth, minHeight := WindowMinimum(dm.monitorWidth, dm.monitorHeight, dm.deviceScale)
	if width < minWidth || height < minHeight {
		return fmt.Errorf(
			"%w: got %dx%d, minimum is %dx%d",
			ErrWindowSizeBelowMinimum,
			width,
			height,
			minWidth,
			minHeight,
		)
	}

	return nil
}

func (dm *DisplayManager) beginRequest(mode config.Mode, width, height int) {
	dm.requestedMode = mode
	dm.requestedWindowWidth = width
	dm.requestedWindowHeight = height
	dm.pending = true
	dm.requestActive = true
	dm.requestTicks = 0
	dm.maximizeIssued = false
}

func (dm *DisplayManager) cancelUnobservedRequest() {
	if dm.mode == config.ModeWindowed &&
		dm.windowWidth >= config.MinimumWindowUnitsWidth && dm.windowHeight >= config.MinimumWindowUnitsHeight {
		dm.lastNormalWidth = dm.windowWidth
		dm.lastNormalHeight = dm.windowHeight
	}
	dm.requestedMode = dm.mode
	dm.requestedWindowWidth = dm.lastNormalWidth
	dm.requestedWindowHeight = dm.lastNormalHeight
	dm.pending = false
	dm.requestActive = false
}

func aspectFit16By9(width, height int) (int, int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if width*aspectHeight <= height*aspectWidth {
		fittedHeight := width * aspectHeight / aspectWidth
		if fittedHeight < 1 {
			fittedHeight = 1
		}

		return width, fittedHeight
	}
	fittedWidth := height * aspectWidth / aspectHeight
	if fittedWidth < 1 {
		fittedWidth = 1
	}

	return fittedWidth, height
}
