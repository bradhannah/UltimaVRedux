package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

// ConfigurationAudioManager defines audio methods needed for settings preview ownership.
type ConfigurationAudioManager interface {
	SetEnabled(enabled bool)
	SetMusicVolume(volume float64) error
	SetSoundEffectVolume(volume float64) error
	SetAmbientVolume(volume float64) error
	// Preview methods for real-time audio feedback
	PlayMusicPreview(track string) error
	StopBackgroundMusic() error
	PlaySoundEffect(effect game_state.SoundEffect) error
	// Ambient sound preview - plays AmbientNatureOutside.ogg at current ambient volume
	PlayAmbientSoundPreview() error
	StopAmbientSoundPreview() error
}

const (
	audioSettingsGameplayTab = iota
	audioSettingsAudioTab
	audioSettingsDisplayTab
)

type configurationDisplayManager interface {
	Mode() config.Mode
	MonitorSize() (int, int)
	DeviceScaleFactor() float64
	OutputSize() (int, int)
	LogicalSize() (int, int)
	LastNormalWindowSize() (int, int)
	PersistableWindowSize() (int, int)
	CommitNormalWindowSize(width, height int)
	RequestMode(mode config.Mode, width, height int) error
	IsTransitionPending() bool
}

type configurationDisplayPhase int

const (
	configurationDisplayEditing configurationDisplayPhase = iota
	configurationDisplayApplying
	configurationDisplayConfirming
	configurationDisplayReverting
)

// ConfigurationDialog provides detached Sound and Gameplay settings with audio preview.
type ConfigurationDialog struct {
	audioReport *configurationAudioReport
	navigation  input.ListNavigation
	// Core dialog properties
	placement *sprites.PercentBasedPlacement
	keyboard  *input.Keyboard
	onDismiss func()
	isVisible bool
	hasFocus  bool

	// Detached configuration transaction
	ultimaConfig             *config.UltimaVConfiguration
	originalAudio            config.AudioConfiguration
	stagedAudio              config.AudioConfiguration
	originalGameplay         config.GameplayConfiguration
	stagedGameplay           config.GameplayConfiguration
	originalDisplay          config.DisplayConfiguration
	stagedDisplay            config.DisplayConfiguration
	audioManager             ConfigurationAudioManager
	displayManager           configurationDisplayManager
	saveError                string
	dismissed                bool
	releaseConfigurationEdit func()

	// UI components
	musicSlider   *widgets.Slider
	sfxSlider     *widgets.Slider
	ambientSlider *widgets.Slider

	// Tab management
	currentTab int
	tabNames   []string

	// Slider focus management (for Audio tab)
	focusedSliderIndex        int // musicSliderIndex, sfxSliderIndex or ambientSliderIndex (Master volume removed)
	focusedGameplayIndex      int
	firstGameplayIndex        int
	focusedDisplayIndex       int
	previewMusicPlaying       bool // Track if preview music is already playing
	previewAmbientPlaying     bool // Track if preview ambient sound is already playing
	displayPhase              configurationDisplayPhase
	displayTransitionTicks    int
	displayConfirmationTicks  int
	keepDisplaySettings       bool
	rollbackMode              config.Mode
	rollbackWidth             int
	rollbackHeight            int
	resetDisplayAfterRollback bool
	displayRollbackRetry      bool
	displaySizeDirty          bool
	displayModeDirty          bool
	displayEditScale          float64

	// Text rendering and responsive layout
	titleOutput    *text.Output
	tabOutput      *text.Output
	gameplayOutput *text.Output
	detailOutput   *text.Output
	labelOutput    *text.Output
	valueOutput    *text.Output
	presetOutput   *text.Output
	footerOutput   *text.Output
	layout         audioSettingsLayout
	lastWidth      int
	lastHeight     int
}

// NewConfigurationDialog creates a detached player-configuration transaction.
func NewConfigurationDialog(
	placement *sprites.PercentBasedPlacement,
	keyboard *input.Keyboard,
	onDismiss func(),
	ultimaConfig *config.UltimaVConfiguration,
	audioManager ConfigurationAudioManager) *ConfigurationDialog {
	return newConfigurationDialogForTab(
		placement,
		keyboard,
		onDismiss,
		ultimaConfig,
		audioManager,
		audioSettingsGameplayTab,
	)
}

func newConfigurationDialogForTab(
	placement *sprites.PercentBasedPlacement,
	keyboard *input.Keyboard,
	onDismiss func(),
	ultimaConfig *config.UltimaVConfiguration,
	audioManager ConfigurationAudioManager,
	initialTab int,
) *ConfigurationDialog {
	validateConfigurationDialogInputs(placement, keyboard, onDismiss, ultimaConfig, initialTab)

	dialog := &ConfigurationDialog{ //nolint:exhaustruct_v5 // Other fields are presentation state with valid zeros.
		placement:        placement,
		keyboard:         keyboard,
		onDismiss:        onDismiss,
		ultimaConfig:     ultimaConfig,
		originalAudio:    *ultimaConfig.AudioConfiguration,
		stagedAudio:      *ultimaConfig.AudioConfiguration,
		originalGameplay: *ultimaConfig.GameplayConfiguration,
		stagedGameplay:   *ultimaConfig.GameplayConfiguration,
		originalDisplay:  *ultimaConfig.DisplayConfiguration,
		stagedDisplay:    *ultimaConfig.DisplayConfiguration,
		audioManager:     audioManager,
		displayManager:   display.GetManager(),
		isVisible:        true,
		hasFocus:         true,
		currentTab:       initialTab,
		tabNames:         []string{"Gameplay", "Sound", "Display"},
	}
	windowWidth, windowHeight := dialog.displayManager.PersistableWindowSize()
	if windowWidth >= config.MinimumWindowUnitsWidth && windowHeight >= config.MinimumWindowUnitsHeight {
		dialog.stagedDisplay.WindowWidth = windowWidth
		dialog.stagedDisplay.WindowHeight = windowHeight
	}
	dialog.stagedDisplay.Mode = dialog.displayManager.Mode()
	dialog.displayEditScale = dialog.displayManager.DeviceScaleFactor()

	dialog.initializeComponents()
	dialog.releaseConfigurationEdit = ultimaConfig.BeginConfigurationEditing()
	width, height := display.GetManager().GetCurrentSize()
	dialog.ensureLayout(width, height)
	dialog.musicSlider.SetFocus(false)
	dialog.sfxSlider.SetFocus(false)
	dialog.ambientSlider.SetFocus(false)

	if initialTab == audioSettingsAudioTab {
		dialog.enablePreviewAudio()
		dialog.updateSliderFocus()
	}

	return dialog
}

func validateConfigurationDialogInputs(
	placement *sprites.PercentBasedPlacement,
	keyboard *input.Keyboard,
	onDismiss func(),
	ultimaConfig *config.UltimaVConfiguration,
	initialTab int,
) {
	if placement == nil || keyboard == nil || onDismiss == nil || ultimaConfig == nil ||
		ultimaConfig.AudioConfiguration == nil || ultimaConfig.DisplayConfiguration == nil ||
		ultimaConfig.GameplayConfiguration == nil {
		panic("NewConfigurationDialog: all parameters and configuration sections are required")
	}
	if initialTab < audioSettingsGameplayTab || initialTab > audioSettingsDisplayTab {
		panic("NewConfigurationDialog: unsupported initial tab")
	}
}

// initializeComponents sets up all UI components for the dialog.
func (d *ConfigurationDialog) initializeComponents() {
	const maximumSlider = 10
	emptyPlacement := sprites.PercentBasedPlacement{
		StartPercentX: 0,
		EndPercentX:   0,
		StartPercentY: 0,
		EndPercentY:   0,
	}
	d.musicSlider = widgets.NewSlider(widgets.SliderConfig{
		TrackColor: nil, HandleColor: nil, LabelColor: nil, BackgroundColor: nil,
		Placement:    emptyPlacement,
		Keyboard:     d.keyboard,
		InitialValue: d.stagedAudio.MusicVolume,
		MinValue:     0,
		MaxValue:     maximumSlider,
		Label:        "Music Volume",
		OnValueChanged: func(_, newValue int) {
			d.stagedAudio.SetMusicVolume(newValue)
			d.saveError = ""
			d.adjustMusicPreviewVolume()
		},
	})

	d.sfxSlider = widgets.NewSlider(widgets.SliderConfig{
		TrackColor: nil, HandleColor: nil, LabelColor: nil, BackgroundColor: nil,
		Placement:    emptyPlacement,
		Keyboard:     d.keyboard,
		InitialValue: d.stagedAudio.SoundEffectVolume,
		MinValue:     0,
		MaxValue:     maximumSlider,
		Label:        "SFX Volume",
		OnValueChanged: func(_, newValue int) {
			d.stagedAudio.SetSoundEffectVolume(newValue)
			d.saveError = ""
			d.previewSoundEffectVolume()
		},
	})

	d.ambientSlider = widgets.NewSlider(widgets.SliderConfig{
		TrackColor: nil, HandleColor: nil, LabelColor: nil, BackgroundColor: nil,
		Placement:    emptyPlacement,
		Keyboard:     d.keyboard,
		InitialValue: d.stagedAudio.AmbientVolume,
		MinValue:     0,
		MaxValue:     maximumSlider,
		Label:        "Ambient Volume",
		OnValueChanged: func(_, newValue int) {
			d.stagedAudio.SetAmbientVolume(newValue)
			d.saveError = ""
			d.adjustAmbientPreviewVolume()
		},
	})
}

// previewAudioChanges applies current slider values to the audio system for real-time preview.
func (d *ConfigurationDialog) previewAudioChanges() {
	if d.audioManager == nil {
		return
	}

	// Convert raw slider values (0-10) to optimized volume levels (0.0-1.0) for audio system
	musicVolume := audio.ConvertSliderToVolume(d.stagedAudio.MusicVolume)
	sfxVolume := audio.ConvertSliderToVolume(d.stagedAudio.SoundEffectVolume)
	ambientVolume := audio.ConvertSliderToVolume(d.stagedAudio.AmbientVolume)

	// Apply volume changes to the audio system in real-time
	if err := d.audioManager.SetMusicVolume(musicVolume); err != nil {
		logging.Warnf(logging.CategoryAudio, "Failed to set music volume: %v", err)
	}

	if err := d.audioManager.SetSoundEffectVolume(sfxVolume); err != nil {
		logging.Warnf(logging.CategoryAudio, "Failed to set sound effect volume: %v", err)
	}

	if err := d.audioManager.SetAmbientVolume(ambientVolume); err != nil {
		logging.Warnf(logging.CategoryAudio, "Failed to set ambient volume: %v", err)
	}
}

// revertAudioSettings reverts audio system to original settings
func (d *ConfigurationDialog) revertAudioSettings() {
	if d.audioManager == nil {
		return
	}

	// Convert original slider values to volume levels
	musicVolume := audio.ConvertSliderToVolume(d.originalAudio.MusicVolume)
	sfxVolume := audio.ConvertSliderToVolume(d.originalAudio.SoundEffectVolume)
	ambientVolume := audio.ConvertSliderToVolume(d.originalAudio.AmbientVolume)

	// Apply original settings
	if err := d.audioManager.SetMusicVolume(musicVolume); err != nil {
		logging.Warnf(logging.CategoryAudio, "Failed to revert music volume: %v", err)
	}

	if err := d.audioManager.SetSoundEffectVolume(sfxVolume); err != nil {
		logging.Warnf(logging.CategoryAudio, "Failed to revert sound effect volume: %v", err)
	}

	if err := d.audioManager.SetAmbientVolume(ambientVolume); err != nil {
		logging.Warnf(logging.CategoryAudio, "Failed to revert ambient volume: %v", err)
	}
}

// onOKClicked applies the current settings and closes the dialog.
func (d *ConfigurationDialog) onOKClicked() {
	if d.displayRollbackRetry {
		d.beginDisplayRollback(d.resetDisplayAfterRollback)

		return
	}
	if d.displayChangeNeedsConfirmation() {
		d.beginDisplayApply()

		return
	}
	d.persistConfiguration()
}

func (d *ConfigurationDialog) displayChangeNeedsConfirmation() bool {
	if d.stagedDisplay.Mode != d.displayManager.Mode() {
		return true
	}
	if d.stagedDisplay.Mode != config.ModeWindowed {
		return false
	}
	windowWidth, windowHeight := d.displayManager.LastNormalWindowSize()

	return d.stagedDisplay.WindowWidth != windowWidth || d.stagedDisplay.WindowHeight != windowHeight
}

func (d *ConfigurationDialog) persistConfiguration() bool {
	if err := d.ultimaConfig.SaveConfigurationSettings(config.ConfigurationSettings{
		Audio:    d.stagedAudio,
		Display:  d.stagedDisplay,
		Gameplay: d.stagedGameplay,
	}); err != nil {
		d.saveError = "Could not save configuration."
		logging.Warnf(logging.CategoryConfig, "Failed to save player configuration: %v", err)

		return false
	}

	d.displayManager.CommitNormalWindowSize(d.stagedDisplay.WindowWidth, d.stagedDisplay.WindowHeight)
	d.stopPreviewAudio()
	d.dismiss()

	return true
}

// onCancelClicked reverts changes and closes the dialog.
func (d *ConfigurationDialog) onCancelClicked() {
	// Stop any preview music
	d.stopPreviewAudio()

	// Revert audio system to original settings
	d.revertAudioSettings()

	d.dismiss()
}

// dismiss closes the dialog and calls the onDismiss callback.
func (d *ConfigurationDialog) dismiss() {
	if d.dismissed {
		return
	}
	d.dismissed = true
	d.releaseConfigurationEdit()
	d.isVisible = false
	d.hasFocus = false

	d.restorePausedAudio()

	d.onDismiss()
}

// enablePreviewAudio temporarily enables the audio manager while the Escape menu is open.
func (d *ConfigurationDialog) enablePreviewAudio() {
	if d.audioManager != nil {
		d.audioManager.SetEnabled(true)
	}
}

// restorePausedAudio returns control to the still-open Escape menu.
func (d *ConfigurationDialog) restorePausedAudio() {
	if d.audioManager != nil {
		d.audioManager.SetEnabled(false)
	}
}

// Tab management methods for testing
