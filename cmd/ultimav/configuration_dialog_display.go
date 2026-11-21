package main

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
)

const (
	configurationDisplayModeRow = iota
	configurationDisplaySizeRow
	configurationDisplayRowCount

	configurationDisplayTransitionTimeout = display.TransitionTimeoutUpdates
	configurationDisplayConfirmationTime  = 720
	configurationTicksPerSecond           = 60
	unknownDisplayLabel                   = "Unknown"
)

type configurationWindowSizeChoice struct {
	config.ScreenResolution
	available   bool
	custom      bool
	pixelWidth  int
	pixelHeight int
}

func (d *ConfigurationDialog) updateDisplayWorkflow() bool {
	switch d.displayPhase {
	case configurationDisplayEditing:
		d.syncDisplayContext()
		d.syncCurrentDraggedWindowSize()

		return false
	case configurationDisplayApplying:
		return d.updateDisplayApplying()
	case configurationDisplayConfirming:
		if d.displayEditScale != d.displayManager.DeviceScaleFactor() || !d.displayStateObserved(
			d.stagedDisplay.Mode, d.stagedDisplay.WindowWidth, d.stagedDisplay.WindowHeight,
		) {
			d.saveError = "Display changed during confirmation. Check the restored settings."
			d.beginDisplayRollback(true)

			return true
		}

		return d.updateDisplayConfirming()
	case configurationDisplayReverting:
		return d.updateDisplayReverting()
	default:
		panic("unsupported Configuration display phase")
	}
}

func (d *ConfigurationDialog) updateDisplayApplying() bool {
	d.displayTransitionTicks++
	if !d.displayManager.IsTransitionPending() {
		if d.displayStateObserved(
			d.stagedDisplay.Mode,
			d.stagedDisplay.WindowWidth,
			d.stagedDisplay.WindowHeight,
		) {
			d.displayPhase = configurationDisplayConfirming
			d.displayConfirmationTicks = configurationDisplayConfirmationTime
			d.keepDisplaySettings = true
			d.keyboard.PrepareForTransition(0)

			return true
		}
		d.saveError = "Display change was not applied. Previous settings restored."
		d.beginDisplayRollback(false)

		return true
	}
	if d.displayTransitionTicks >= configurationDisplayTransitionTimeout {
		d.saveError = "No answer in time. The old display settings are back."
		d.beginDisplayRollback(false)
	}

	return true
}

func (d *ConfigurationDialog) updateDisplayConfirming() bool {
	d.displayConfirmationTicks--
	if d.displayConfirmationTicks <= 0 {
		d.beginDisplayRollback(true)

		return true
	}
	d.updateDisplayConfirmationInput()

	return true
}

func (d *ConfigurationDialog) updateDisplayReverting() bool {
	d.displayTransitionTicks++
	if !d.displayManager.IsTransitionPending() {
		if d.displayStateObserved(d.rollbackMode, d.rollbackWidth, d.rollbackHeight) {
			d.displayPhase = configurationDisplayEditing
			d.displayTransitionTicks = 0
			d.displayRollbackRetry = false
			if d.resetDisplayAfterRollback {
				d.stagedDisplay.Mode = d.rollbackMode
				d.stagedDisplay.WindowWidth = d.rollbackWidth
				d.stagedDisplay.WindowHeight = d.rollbackHeight
				d.displaySizeDirty = false
				d.displayModeDirty = false
			}
			d.keyboard.PrepareForTransition(0)

			return true
		}
		d.displayPhase = configurationDisplayEditing
		d.displayRollbackRetry = true
		d.saveError = "Could not restore the previous display settings."

		return true
	}
	if d.displayTransitionTicks >= configurationDisplayTransitionTimeout {
		d.displayPhase = configurationDisplayEditing
		d.displayRollbackRetry = true
		d.saveError = "Could not restore the previous display settings."
	}

	return true
}

func (d *ConfigurationDialog) displayStateObserved(mode config.Mode, width, height int) bool {
	if d.displayManager.Mode() != mode {
		return false
	}
	if mode != config.ModeWindowed {
		return true
	}
	actualWidth, actualHeight := d.displayManager.LastNormalWindowSize()

	return actualWidth == width && actualHeight == height
}

func (d *ConfigurationDialog) beginDisplayApply() {
	if d.stagedDisplay.Mode == config.ModeWindowed && d.displaySizeDirty {
		found := false
		for _, choice := range d.windowSizeChoices() {
			found = found || (choice.available && choice.X == d.stagedDisplay.WindowWidth &&
				choice.Y == d.stagedDisplay.WindowHeight)
		}
		if !found {
			d.saveError = "Window size no longer fits this display. Choose another size or Maximize."

			return
		}
	}
	d.displayEditScale = d.displayManager.DeviceScaleFactor()
	d.displayRollbackRetry = false
	d.rollbackMode = d.displayManager.Mode()
	d.rollbackWidth, d.rollbackHeight = d.displayManager.LastNormalWindowSize()
	if err := d.displayManager.RequestMode(
		d.stagedDisplay.Mode,
		d.stagedDisplay.WindowWidth,
		d.stagedDisplay.WindowHeight,
	); err != nil {
		d.saveError = fmt.Sprintf("Could not apply display settings: %v", err)

		return
	}
	d.saveError = ""
	d.displayPhase = configurationDisplayApplying
	d.displayTransitionTicks = 0
	d.keyboard.PrepareForTransition(0)
}

func (d *ConfigurationDialog) beginDisplayRollback(resetStaged bool) {
	monitorWidth, monitorHeight := d.displayManager.MonitorSize()
	d.rollbackWidth, d.rollbackHeight = display.RestoreWindowSize(
		d.rollbackWidth, d.rollbackHeight, monitorWidth, monitorHeight, d.displayManager.DeviceScaleFactor(),
	)
	d.resetDisplayAfterRollback = resetStaged
	if err := d.displayManager.RequestMode(d.rollbackMode, d.rollbackWidth, d.rollbackHeight); err != nil {
		d.displayPhase = configurationDisplayEditing
		d.displayRollbackRetry = true
		d.saveError = fmt.Sprintf("Could not restore display settings: %v", err)

		return
	}
	d.displayPhase = configurationDisplayReverting
	d.displayTransitionTicks = 0
	d.keyboard.PrepareForTransition(0)
}

func (d *ConfigurationDialog) updateDisplayConfirmationInput() {
	boundKeys := []ebiten.Key{ebiten.KeyEscape, ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeyLeft, ebiten.KeyRight}
	boundKey := d.keyboard.GetBoundKeyPressed(&boundKeys)
	if boundKey == nil || !d.keyboard.TryToRegisterKeyPress(*boundKey) {
		return
	}

	//nolint:exhaustive // The lookup above restricts input to confirmation controls.
	switch *boundKey {
	case ebiten.KeyEscape:
		d.beginDisplayRollback(true)
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		if !d.keepDisplaySettings {
			d.beginDisplayRollback(true)

			return
		}
		if !d.persistConfiguration() {
			d.beginDisplayRollback(false)
		}
	case ebiten.KeyLeft, ebiten.KeyRight:
		d.keepDisplaySettings = !d.keepDisplaySettings
	default:
		return
	}
}

func (d *ConfigurationDialog) updateDisplayTab() {
	boundKeys := []ebiten.Key{
		ebiten.KeyEscape,
		ebiten.KeyEnter,
		ebiten.KeyNumpadEnter,
		ebiten.KeyLeft,
		ebiten.KeyRight,
		ebiten.KeyTab,
	}
	boundKey := d.keyboard.GetBoundKeyPressed(&boundKeys)
	if boundKey == nil || !d.keyboard.TryToRegisterKeyPress(*boundKey) {
		return
	}

	//nolint:exhaustive // The lookup above restricts input to Display controls.
	switch *boundKey {
	case ebiten.KeyEscape:
		d.onCancelClicked()
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		d.onOKClicked()
	case ebiten.KeyTab:
		d.cycleConfigurationTab()
	case ebiten.KeyLeft:
		d.adjustFocusedDisplaySetting(-1)
	case ebiten.KeyRight:
		d.adjustFocusedDisplaySetting(1)
	default:
		return
	}
}

func (d *ConfigurationDialog) adjustFocusedDisplaySetting(delta int) {
	d.displayRollbackRetry = false
	switch d.focusedDisplayIndex {
	case configurationDisplayModeRow:
		modes := [...]config.Mode{config.ModeWindowed, config.ModeMaximized, config.ModeFullscreen}
		current := 0
		for index, mode := range modes {
			if mode == d.stagedDisplay.Mode {
				current = index

				break
			}
		}
		current = (current + delta + len(modes)) % len(modes)
		d.stagedDisplay.Mode = modes[current]
		d.displayModeDirty = true
		if d.stagedDisplay.Mode != config.ModeWindowed {
			d.focusedDisplayIndex = configurationDisplayModeRow
		}
	case configurationDisplaySizeRow:
		if d.stagedDisplay.Mode != config.ModeWindowed {
			return
		}
		choices := d.availableWindowSizes()
		if len(choices) == 0 {
			return
		}
		current := 0
		for index, choice := range choices {
			if choice.X == d.stagedDisplay.WindowWidth && choice.Y == d.stagedDisplay.WindowHeight {
				current = index

				break
			}
		}
		current = (current + delta + len(choices)) % len(choices)
		d.stagedDisplay.WindowWidth = choices[current].X
		d.stagedDisplay.WindowHeight = choices[current].Y
		d.displaySizeDirty = true
	default:
		panic("unsupported Display setting index")
	}
	d.saveError = ""
}

func (d *ConfigurationDialog) syncCurrentDraggedWindowSize() {
	if d.displaySizeDirty || d.displayManager.Mode() != config.ModeWindowed ||
		d.displayManager.IsTransitionPending() {
		return
	}
	width, height := d.displayManager.PersistableWindowSize()
	if width < config.MinimumWindowUnitsWidth || height < config.MinimumWindowUnitsHeight {
		return
	}
	d.stagedDisplay.WindowWidth = width
	d.stagedDisplay.WindowHeight = height
}

func (d *ConfigurationDialog) availableWindowSizes() []config.ScreenResolution {
	allChoices := d.windowSizeChoices()
	choices := make([]config.ScreenResolution, 0, len(allChoices))
	for _, choice := range allChoices {
		if choice.available {
			choices = append(choices, choice.ScreenResolution)
		}
	}

	return choices
}

func (d *ConfigurationDialog) windowSizeChoices() []configurationWindowSizeChoice {
	monitorWidth, monitorHeight := d.displayManager.MonitorSize()
	scale := d.displayManager.DeviceScaleFactor()
	choices := make([]configurationWindowSizeChoice, 0, len(config.WindowPixelPresets())+1)
	custom := config.ScreenResolution{X: d.stagedDisplay.WindowWidth, Y: d.stagedDisplay.WindowHeight}
	customIsPreset := false
	for _, pixels := range config.WindowPixelPresets() {
		width, height := display.WindowUnits(pixels.X, pixels.Y, scale)
		resolution := config.ScreenResolution{X: width, Y: height}
		if resolution == custom {
			customIsPreset = true
		}
		choices = append(choices, configurationWindowSizeChoice{
			ScreenResolution: resolution,
			available: float64(width) <= float64(monitorWidth)*display.WindowFitFraction &&
				float64(height) <= float64(monitorHeight)*display.WindowFitFraction &&
				width >= config.MinimumWindowUnitsWidth && height >= config.MinimumWindowUnitsHeight,
			custom:     false,
			pixelWidth: pixels.X, pixelHeight: pixels.Y,
		})
	}
	if !customIsPreset && custom.X >= config.MinimumWindowUnitsWidth && custom.Y >= config.MinimumWindowUnitsHeight {
		pixelWidth, pixelHeight := display.WindowPixels(custom.X, custom.Y, scale)
		choices = append(choices, configurationWindowSizeChoice{
			ScreenResolution: custom,
			available:        custom.X <= monitorWidth && custom.Y <= monitorHeight,
			custom:           true,
			pixelWidth:       pixelWidth, pixelHeight: pixelHeight,
		})
	}

	return choices
}

func displayModeLabel(mode config.Mode) string {
	switch mode {
	case config.ModeWindowed:
		return "Windowed"
	case config.ModeMaximized:
		return "Maximized Window"
	case config.ModeFullscreen:
		return "Borderless Fullscreen"
	default:
		return unknownDisplayLabel
	}
}

func (d *ConfigurationDialog) displaySizeLabel() string {
	if d.stagedDisplay.Mode == config.ModeMaximized || d.stagedDisplay.Mode == config.ModeFullscreen {
		if d.stagedDisplay.Mode != d.displayManager.Mode() {
			return "Automatic after Apply"
		}
		width, height := d.displayManager.OutputSize()
		if width <= 0 || height <= 0 {
			return "Automatic (awaiting frame)"
		}

		return fmt.Sprintf("%d x %d pixels (Auto)", width, height)
	}
	width, height := display.WindowPixels(
		d.stagedDisplay.WindowWidth, d.stagedDisplay.WindowHeight, d.displayManager.DeviceScaleFactor(),
	)
	label := fmt.Sprintf("%d x %d pixels", width, height)
	for _, choice := range d.windowSizeChoices() {
		if !choice.custom && choice.X == d.stagedDisplay.WindowWidth && choice.Y == d.stagedDisplay.WindowHeight {
			if width != choice.pixelWidth || height != choice.pixelHeight {
				return "About " + label
			}

			return label
		}
	}

	return "Custom: " + label
}

// syncDisplayContext follows native mode changes only until a player explicitly
// stages a mode. A scale change invalidates staged pixel-size intent, not history.
func (d *ConfigurationDialog) syncDisplayContext() {
	if !d.displayModeDirty {
		d.stagedDisplay.Mode = d.displayManager.Mode()
	}
	if scale := d.displayManager.DeviceScaleFactor(); scale != d.displayEditScale {
		d.displayEditScale = scale
		d.displaySizeDirty = false
		d.saveError = "Display scale changed. Check the window size before applying."
	}
}
