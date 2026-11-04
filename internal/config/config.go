// Package config loads and persists player preferences and original-data locations.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/spf13/viper"

	"github.com/bradhannah/UltimaVRedux/internal/files"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
)

// Static errors for consistent error handling
var (
	ErrDebugConfigNil          = errors.New("debug configuration is nil")
	ErrGameplayConfigNil       = errors.New("gameplay configuration is nil")
	ErrAudioConfigNil          = errors.New("audio configuration is nil")
	ErrDisplayConfigNil        = errors.New("display configuration is nil")
	ErrLoggingConfigNil        = errors.New("logging configuration is nil")
	ErrConfigFileUnknown       = errors.New("configuration file path is unknown")
	ErrConfigSourceUnavailable = errors.New("configuration source is unavailable")
	ErrResolutionIndex         = errors.New("invalid window resolution index")
	ErrDisplayConfig           = errors.New("invalid display configuration")
	ErrDataFilePathUnset       = errors.New(
		"DataFilePath is unset; choose an installation or set DataFilePath in the settings file",
	)
)

const (
	maximumVolumeSlider       = 10
	millisecondsPerSecond     = 1000
	defaultAmbientLightRadius = 2
)

// always use 16x9 resolutions

// 1280 x 720 (HD)
// 1366 x 768 (HD)
// 1600 x 900
// 1920 x 1080 (Full HD or 1080p)
// 2560 x 1440 (QHD or 1440p)
// 3840 x 2160 (4K UHD)

// var WindowWidth = 1280
// var WindowHeight = 720

// var WindowWidth = 1920
// var WindowHeight = 1080

// var WindowWidth = 3840
// var WindowHeight = 2160
// var WindowWidth = 2560
// var WindowHeight = 1440

// UltimaVConfigurationFlags holds the legacy display flags and active data path.
type UltimaVConfigurationFlags struct {
	Resolution   int
	FullScreen   bool
	DataFilePath string
}

// AudioConfiguration holds volume settings for different audio channels
// Stores raw slider values (0-10) for simplicity and future tweaking flexibility
type AudioConfiguration struct {
	MusicVolume       int // Music track volume slider (0-10)
	SoundEffectVolume int // Sound effects volume slider (0-10)
	AmbientVolume     int // Ambient sounds volume slider (0-10)
}

// DisplayConfiguration holds desktop display and performance settings.
type DisplayConfiguration struct {
	Mode              Mode
	WindowWidth       int
	WindowHeight      int
	TargetFPS         int  // Target FPS for draw limiting (0 = unlimited, 30, 60, etc.)
	EnableFpsLimiting bool // Enable FPS limiting
}

// GameplayConfiguration holds optional Redux gameplay rules.
type GameplayConfiguration struct {
	// REDUX DEVIATION: Original merchants steal per economic unit; ref:MERCH-0003.
	// BatchMerchantTheft defaults on for one theft per confirmed paid batch,
	// keeping keyboard quantity selection predictable.
	BatchMerchantTheft bool
	// AllowRepeatedMonsterDivision restores original SPLITS behavior when true.
	AllowRepeatedMonsterDivision bool
	// ShowConcealedTurnOrderSlots reserves blank forecast slots for concealed enemies.
	ShowConcealedTurnOrderSlots bool
	// PersistHiddenDungeonDiscoveries retains revealed hidden doors and pits between visits.
	PersistHiddenDungeonDiscoveries bool
	// ShowSubduedUnlitDungeons keeps dungeon geometry faintly visible without real light.
	ShowSubduedUnlitDungeons bool
	// SmoothSurfaceMovement enables optional surface camera scrolling, including boats.
	SmoothSurfaceMovement bool
	// FlexibleSextantUse opts into daytime and settlement sextant navigation.
	FlexibleSextantUse     bool
	CombatSpeedFactor      float64
	ConfirmQuickSave       bool
	ShowCombatHPBars       bool
	MaxVisibleActionPopups int
}

// ConfigurationSettings is the complete player-facing Configuration transaction.
type ConfigurationSettings struct {
	Audio    AudioConfiguration
	Display  DisplayConfiguration
	Gameplay GameplayConfiguration
}

const (
	gameplayBatchMerchantTheftKey              = "Gameplay.Merchants.BatchTheft"
	gameplayShowConcealedTurnOrderSlotsKey     = "Gameplay.Combat.ShowConcealedTurnOrderSlots"
	gameplayPersistHiddenDungeonDiscoveriesKey = "Gameplay.Dungeons.PersistHiddenDiscoveries"
	gameplayShowSubduedUnlitDungeonsKey        = "Gameplay.Dungeons.ShowSubduedUnlitDungeons"
	gameplaySmoothSurfaceMovementKey           = "Gameplay.Presentation.SmoothSurfaceMovement"
	gameplayFlexibleSextantUseKey              = "Gameplay.Navigation.FlexibleSextantUse"
)

// LoggingConfiguration holds logging system settings
type LoggingConfiguration struct {
	Enabled        bool            // Global logging enabled/disabled
	StdoutEnabled  bool            // Output to stdout enabled/disabled
	CategoryStates map[string]bool // Per-category enabled/disabled state
	MinLevel       string          // Minimum log level ("DEBUG", "INFO", "WARN", "ERROR", "FATAL")
}

// DebugConfiguration holds debug settings.
type DebugConfiguration struct {
	// Small-map presentation prototype; read from config at startup, not player settings.
	SurfaceMovementDurationMs int
	SurfaceMovementCurve      string
	CombatDetailShown         bool    // Show authentic combat rolls and initiative detail
	CombatDetailSet           bool    // True after loading or explicitly changing the setting
	CombatMovementSpeedFactor float64 // Deprecated legacy snapshot; runtime speed belongs to Gameplay.
}

// Combat movement speed factors define the supported persisted debug range and default.
const (
	MinCombatMovementSpeedFactor = 0.25
	MaxCombatMovementSpeedFactor = 4.0
)

func validCombatMovementSpeedFactor(factor float64) bool {
	return !math.IsNaN(factor) && !math.IsInf(factor, 0) &&
		factor >= MinCombatMovementSpeedFactor && factor <= MaxCombatMovementSpeedFactor
}

// clampSliderValue ensures slider value is in valid 0-10 range
func clampSliderValue(value int) int {
	if value < 0 {
		return 0
	}
	if value > maximumVolumeSlider {
		return maximumVolumeSlider
	}

	return value
}

// SetMusicVolume sets the music volume slider with validation (0-10)
func (a *AudioConfiguration) SetMusicVolume(sliderValue int) {
	a.MusicVolume = clampSliderValue(sliderValue)
}

// SetSoundEffectVolume sets the sound effect volume slider with validation (0-10)
func (a *AudioConfiguration) SetSoundEffectVolume(sliderValue int) {
	a.SoundEffectVolume = clampSliderValue(sliderValue)
}

// SetAmbientVolume sets the ambient volume slider with validation (0-10)
func (a *AudioConfiguration) SetAmbientVolume(sliderValue int) {
	a.AmbientVolume = clampSliderValue(sliderValue)
}

// GetFrameInterval returns the minimum time between frames in milliseconds
func (d *DisplayConfiguration) GetFrameInterval() int64 {
	if !d.EnableFpsLimiting || d.TargetFPS <= 0 {
		return 0 // No limiting
	}

	return millisecondsPerSecond / int64(d.TargetFPS)
}

// UltimaVConfiguration owns active preferences, original data, and the selected persistence source.
type UltimaVConfiguration struct {
	// DataFilePath     string
	RawDataOvl       []byte
	allWindowConfigs []ScreenResolution

	SavedConfigData *UltimaVConfigurationFlags

	// Each configuration owns its selected source, including unknown settings.
	source *viper.Viper
	// Track preferences last loaded or deliberately saved by this session so an
	// exit-time save cannot undo external edits that take effect next launch.
	runtimeBaseline          *viper.Viper
	configurationEditors     int
	configurationDiagnostics []string

	// Audio configuration for volume settings
	AudioConfiguration *AudioConfiguration

	// SoundDisabled disables runtime audio initialization without changing persisted volume settings.
	SoundDisabled bool

	// Display configuration for performance settings
	DisplayConfiguration *DisplayConfiguration

	// Gameplay configuration for deliberate Redux rules.
	GameplayConfiguration *GameplayConfiguration

	// Logging configuration for debug output
	LoggingConfiguration *LoggingConfiguration

	// Debug configuration for console history
	DebugConfiguration *DebugConfiguration

	// AmbientLightRadius is the visibility radius when avatar has no torch/magic at night.
	// Range: 1-5. Default: 2 tiles. Provides minimal "night vision" in darkness.
	AmbientLightRadius int
}

// NewUltimaVConfiguration loads the user's configuration. Only a missing default
// file is created; reading an existing file never requires write access.
func NewUltimaVConfiguration() (*UltimaVConfiguration, error) {
	source, err := loadUserConfigurationSource()
	if err != nil {
		return nil, err
	}

	return newUltimaVConfiguration(source, "", true)
}

func loadUserConfigurationSource() (*viper.Viper, error) {
	home, err := configurationHome()
	if err != nil {
		return nil, fmt.Errorf("locating configuration home: %w", err)
	}
	source := viper.New()
	source.SetConfigName("config")
	source.SetConfigType("yaml")
	directory := filepath.Join(home, ".ultima_v_redux")
	source.AddConfigPath(directory)
	if err := source.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return source, fmt.Errorf("reading configuration: %w", err)
		}
		if err := os.MkdirAll(directory, 0o700); err != nil { //nolint:mnd // User configuration is private to its owner.
			return nil, fmt.Errorf("creating configuration directory: %w", err)
		}
		source.Set("DataFilePath", "")
		if err := source.SafeWriteConfig(); err != nil {
			return nil, fmt.Errorf("writing default configuration: %w", err)
		}
		source.SetConfigFile(filepath.Join(directory, "config.yaml"))
	}

	return source, nil
}

// NewUltimaVConfigurationWithPath uses default preferences and explicit game data.
// It never reads HOME and has no persistence destination.
func NewUltimaVConfigurationWithPath(dataFilePath string) (*UltimaVConfiguration, error) {
	return LoadUltimaVConfiguration("", dataFilePath)
}

// PlayableWithDataFilePath loads original data from an explicit directory while
// preserving this configuration's in-memory preferences. It performs no writes.
func (uc *UltimaVConfiguration) PlayableWithDataFilePath(dataFilePath string) (*UltimaVConfiguration, error) {
	if uc == nil || uc.source == nil {
		return nil, fmt.Errorf("loading playable configuration: %w", ErrConfigSourceUnavailable)
	}

	return newUltimaVConfiguration(uc.source, dataFilePath, true)
}

// LoadUltimaVConfiguration reads exactly the selected YAML file, without search
// paths or writes. An empty configFilePath uses in-memory defaults and disables
// persistence. A nonempty dataFilePath overrides the loaded game-data location.
func LoadUltimaVConfiguration(configFilePath, dataFilePath string) (*UltimaVConfiguration, error) {
	source := viper.New()
	source.SetConfigType("yaml")
	if configFilePath != "" {
		absolutePath, err := filepath.Abs(configFilePath)
		if err != nil {
			return nil, fmt.Errorf("resolving configuration %s: %w", configFilePath, err)
		}
		source.SetConfigFile(absolutePath)
		if err := source.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("reading configuration %s: %w", configFilePath, err)
		}
	}

	return newUltimaVConfiguration(source, dataFilePath, true)
}

// newUltimaVConfiguration is the internal constructor used by both public constructors
//

func newUltimaVConfiguration(
	source *viper.Viper,
	explicitDataPath string,
	loadOriginalData bool,
) (*UltimaVConfiguration, error) {
	if err := validateConfigurationTypes(source); err != nil {
		return nil, err
	}
	configuration := UltimaVConfiguration{source: source} //nolint:exhaustruct_v5 // Sections are loaded below.
	if err := configuration.loadConfigurationSource(explicitDataPath, loadOriginalData); err != nil {
		return nil, err
	}

	configuration.allWindowConfigs = SupportedWindowResolutions()

	configuration.loadAudioConfiguration()
	configuration.loadDisplayConfiguration()

	configuration.loadGameplayPreferences()

	configuration.loadLoggingConfiguration()
	configuration.loadDebugConfiguration()

	// Initialize ambient light radius with fallback to default
	if source.IsSet("Lighting.AmbientLightRadius") {
		configuration.AmbientLightRadius = source.GetInt("Lighting.AmbientLightRadius")
	} else {
		configuration.AmbientLightRadius = defaultAmbientLightRadius
	}
	// Clamp to valid range (1-5)
	if configuration.AmbientLightRadius < 1 || configuration.AmbientLightRadius > 5 {
		configuration.AmbientLightRadius = defaultAmbientLightRadius
	}

	configuration.runtimeBaseline = viper.New()
	setInitializedConfiguration(configuration.runtimeBaseline, &configuration)

	return &configuration, nil
}

func (uc *UltimaVConfiguration) loadConfigurationSource(explicitDataPath string, loadOriginalData bool) error {
	uc.SavedConfigData = &UltimaVConfigurationFlags{Resolution: 0, FullScreen: false, DataFilePath: ""}
	if err := uc.source.Unmarshal(uc.SavedConfigData); err != nil {
		return fmt.Errorf("decoding configuration: %w", err)
	}

	uc.SavedConfigData.DataFilePath = explicitDataPath
	if explicitDataPath == "" {
		uc.SavedConfigData.DataFilePath = uc.source.GetString("DataFilePath")
	}
	if strings.TrimSpace(uc.SavedConfigData.DataFilePath) == "" {
		uc.SavedConfigData.DataFilePath = ""
	}

	if loadOriginalData {
		if uc.SavedConfigData.DataFilePath == "" {
			return ErrDataFilePathUnset
		}
		var err error
		uc.RawDataOvl, err = os.ReadFile(files.Resolve(uc.SavedConfigData.DataFilePath, files.DataOvl))
		if err != nil {
			return fmt.Errorf("reading DATA.OVL from %s: %w", uc.SavedConfigData.DataFilePath, err)
		}
	}

	return nil
}

//nolint:mnd // Slider defaults are intentional persisted configuration values.
func (uc *UltimaVConfiguration) loadAudioConfiguration() {
	uc.AudioConfiguration = &AudioConfiguration{MusicVolume: 5, SoundEffectVolume: 6, AmbientVolume: 4}
	if uc.source.IsSet("Audio.MusicVolume") {
		uc.AudioConfiguration.MusicVolume = uc.source.GetInt("Audio.MusicVolume")
	}
	if uc.source.IsSet("Audio.SoundEffectVolume") {
		uc.AudioConfiguration.SoundEffectVolume = uc.source.GetInt("Audio.SoundEffectVolume")
	}
	if uc.source.IsSet("Audio.AmbientVolume") {
		uc.AudioConfiguration.AmbientVolume = uc.source.GetInt("Audio.AmbientVolume")
	}
}

func (uc *UltimaVConfiguration) loadDisplayConfiguration() {
	uc.DisplayConfiguration = defaultDisplayConfiguration()
	if uc.source.IsSet("Display.TargetFPS") {
		uc.DisplayConfiguration.TargetFPS = uc.source.GetInt("Display.TargetFPS")
	}
	if uc.source.IsSet("Display.EnableFpsLimiting") {
		uc.DisplayConfiguration.EnableFpsLimiting = uc.source.GetBool("Display.EnableFpsLimiting")
	}

	modeSet := uc.source.IsSet("Display.Mode")
	widthSet := uc.source.IsSet("Display.WindowWidth")
	heightSet := uc.source.IsSet("Display.WindowHeight")
	if modeSet || widthSet || heightSet {
		if modeSet && widthSet && heightSet {
			uc.loadExplicitDisplayDimensions()
		}
		uc.syncLegacyDisplayModel()

		return
	}

	uc.loadLegacyDisplayDimensions()
	uc.syncLegacyDisplayModel()
}

func (uc *UltimaVConfiguration) loadExplicitDisplayDimensions() {
	modeValue, modeOK := uc.source.Get("Display.Mode").(string)
	mode, modeErr := ParseMode(modeValue)
	width, widthOK := strictConfigurationInt(uc.source.Get("Display.WindowWidth"))
	height, heightOK := strictConfigurationInt(uc.source.Get("Display.WindowHeight"))
	if modeOK && modeErr == nil && widthOK && heightOK && validDisplayDimensions(width, height) {
		uc.DisplayConfiguration.Mode = mode
		uc.DisplayConfiguration.WindowWidth = width
		uc.DisplayConfiguration.WindowHeight = height
	}
}

func (uc *UltimaVConfiguration) loadLegacyDisplayDimensions() {
	resolution, resolutionOK := strictConfigurationInt(uc.source.Get("Resolution"))
	fullscreen, fullscreenOK := uc.source.Get("FullScreen").(bool)
	resolutions := uc.windowResolutions()
	if uc.source.IsSet("Resolution") && uc.source.IsSet("FullScreen") &&
		resolutionOK && fullscreenOK && resolution >= 0 && resolution < len(resolutions) {
		uc.DisplayConfiguration.WindowWidth = resolutions[resolution].X
		uc.DisplayConfiguration.WindowHeight = resolutions[resolution].Y
		if fullscreen {
			uc.DisplayConfiguration.Mode = ModeFullscreen
		}
	}
}

//nolint:mnd // Thirty FPS is the intentional default frame limit.
func defaultDisplayConfiguration() *DisplayConfiguration {
	return &DisplayConfiguration{
		Mode:              ModeWindowed,
		WindowWidth:       MinimumWindowWidth,
		WindowHeight:      MinimumWindowHeight,
		TargetFPS:         30,
		EnableFpsLimiting: true,
	}
}

func validDisplayDimensions(width, height int) bool {
	return width >= MinimumWindowUnitsWidth && height >= MinimumWindowUnitsHeight
}

func validDisplayConfiguration(configuration DisplayConfiguration) bool {
	return configuration.Mode.IsValid() &&
		validDisplayDimensions(configuration.WindowWidth, configuration.WindowHeight)
}

func strictConfigurationInt(value any) (int, bool) {
	number, ok := value.(int)

	return number, ok
}

func (uc *UltimaVConfiguration) loadLoggingConfiguration() {
	uc.LoggingConfiguration = &LoggingConfiguration{
		Enabled:        false,
		StdoutEnabled:  false,
		CategoryStates: make(map[string]bool),
		MinLevel:       "DEBUG",
	}
	uc.LoggingConfiguration.Enabled = uc.source.GetBool("Logging.Enabled")
	uc.LoggingConfiguration.StdoutEnabled = uc.source.GetBool("Logging.StdoutEnabled")
	if uc.source.IsSet("Logging.MinLevel") {
		uc.LoggingConfiguration.MinLevel = uc.source.GetString("Logging.MinLevel")
	}
	for category, enabled := range uc.source.GetStringMap("Logging.CategoryStates") {
		if boolValue, ok := enabled.(bool); ok {
			uc.LoggingConfiguration.CategoryStates[category] = boolValue
		}
	}
}

func (uc *UltimaVConfiguration) loadDebugConfiguration() {
	uc.DebugConfiguration = &DebugConfiguration{ //nolint:exhaustruct_v5 // Remaining fields intentionally use defaults.
		SurfaceMovementDurationMs: uc.source.GetInt("Debug.SurfaceMovementDurationMs"),
		SurfaceMovementCurve:      uc.source.GetString("Debug.SurfaceMovementCurve"),
		CombatMovementSpeedFactor: uc.GetCombatSpeedFactor(),
	}
	if uc.source.IsSet("Debug.CombatDetailShown") {
		uc.DebugConfiguration.CombatDetailShown = uc.source.GetBool("Debug.CombatDetailShown")
		uc.DebugConfiguration.CombatDetailSet = true
	}
}

// GetLookDataFilePath returns the look-description file in the active data folder.
func (uc *UltimaVConfiguration) GetLookDataFilePath() string {
	return files.Resolve(uc.SavedConfigData.DataFilePath, files.Look2Dat)
}

// GetSignsDataFilePath returns the signs file in the active data folder.
func (uc *UltimaVConfiguration) GetSignsDataFilePath() string {
	return files.Resolve(uc.SavedConfigData.DataFilePath, files.SignsDat)
}

func (uc *UltimaVConfiguration) windowResolutions() []ScreenResolution {
	if len(uc.allWindowConfigs) != 0 {
		return uc.allWindowConfigs
	}

	return SupportedWindowResolutions()
}

func (uc *UltimaVConfiguration) syncLegacyDisplayModel() {
	if uc.DisplayConfiguration == nil || uc.SavedConfigData == nil {
		return
	}
	uc.SavedConfigData.FullScreen = uc.DisplayConfiguration.Mode == ModeFullscreen
	uc.SavedConfigData.Resolution = 0
	for index, resolution := range uc.windowResolutions() {
		if resolution.X == uc.DisplayConfiguration.WindowWidth &&
			resolution.Y == uc.DisplayConfiguration.WindowHeight {
			uc.SavedConfigData.Resolution = index

			return
		}
	}
}

// UpdateSaveFile persists changed runtime preferences and initializes absent
// defaults without overwriting externally edited next-launch preferences.
func (uc *UltimaVConfiguration) UpdateSaveFile() error {
	current := viper.New()
	setInitializedConfiguration(current, uc)
	if err := uc.saveConfiguration(func(candidate *viper.Viper) {
		for _, key := range current.AllKeys() {
			if !candidate.IsSet(key) || uc.runtimeBaseline == nil ||
				!reflect.DeepEqual(current.Get(key), uc.runtimeBaseline.Get(key)) {
				candidate.Set(key, current.Get(key))
			}
		}
	}); err != nil {
		return fmt.Errorf("saving configuration: %w", err)
	}

	return nil
}

// Stage changes independently so a failed write cannot leak Viper overrides.
func (uc *UltimaVConfiguration) saveConfiguration(update func(*viper.Viper)) error {
	if uc.GameplayConfiguration != nil {
		if err := ValidateGameplayConfiguration(*uc.GameplayConfiguration); err != nil {
			return err
		}
	}
	if uc.source == nil || uc.source.ConfigFileUsed() == "" {
		return ErrConfigFileUnknown
	}
	configurationPath := uc.source.ConfigFileUsed()
	candidate := viper.New()
	candidate.SetConfigType("yaml")
	candidate.SetConfigFile(configurationPath)
	if err := candidate.ReadInConfig(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("rereading configuration %s: %w", configurationPath, err)
		}
		if err := candidate.MergeConfigMap(uc.source.AllSettings()); err != nil {
			return fmt.Errorf("recreating configuration %s: %w", configurationPath, err)
		}
	}
	if err := validateConfigurationTypes(candidate); err != nil {
		return err
	}
	update(candidate)
	if !candidate.IsSet(gameplayCombatSpeedKey) && uc.GameplayConfiguration != nil {
		candidate.Set(gameplayCombatSpeedKey, uc.GetCombatSpeedFactor())
	}
	if err := validateConfigurationTypes(candidate); err != nil {
		return err
	}
	if err := writeViperConfigAtomically(candidate, configurationPath); err != nil {
		return err
	}
	uc.source = candidate
	if uc.runtimeBaseline != nil {
		update(uc.runtimeBaseline)
	}

	return nil
}

func setInitializedConfiguration(configuration *viper.Viper, model *UltimaVConfiguration) {
	// DataFilePath is intentionally omitted. Ordinary preference saves must not
	// persist an explicit runtime data override or replace a pending next-launch selection.
	if model.SavedConfigData != nil {
		configuration.Set("Resolution", model.SavedConfigData.Resolution)
		configuration.Set("FullScreen", model.SavedConfigData.FullScreen)
	}
	if model.AudioConfiguration != nil {
		setAudioConfiguration(configuration, *model.AudioConfiguration)
	}
	if model.DisplayConfiguration != nil {
		setDisplayConfiguration(configuration, *model.DisplayConfiguration)
	}
	if model.GameplayConfiguration != nil {
		setGameplayConfiguration(configuration, *model.GameplayConfiguration)
	}
	if model.LoggingConfiguration != nil {
		setLoggingConfiguration(configuration, *model.LoggingConfiguration)
	}
	if model.DebugConfiguration != nil {
		setDebugConfiguration(configuration, *model.DebugConfiguration)
	}
}

// SaveConfigurationSettings atomically persists the player-facing settings.
// Existing configuration pointers remain stable for runtime consumers.
func (uc *UltimaVConfiguration) SaveConfigurationSettings(settings ConfigurationSettings) error {
	if uc.AudioConfiguration == nil {
		return ErrAudioConfigNil
	}
	if uc.DisplayConfiguration == nil {
		return ErrDisplayConfigNil
	}
	if uc.GameplayConfiguration == nil {
		return ErrGameplayConfigNil
	}
	if err := ValidateGameplayConfiguration(settings.Gameplay); err != nil {
		return err
	}
	if isZeroDisplayConfiguration(settings.Display) {
		settings.Display = *uc.DisplayConfiguration
	}
	if !validDisplayConfiguration(settings.Display) {
		return fmt.Errorf(
			"%w: %s %dx%d",
			ErrDisplayConfig,
			settings.Display.Mode,
			settings.Display.WindowWidth,
			settings.Display.WindowHeight,
		)
	}

	if err := uc.saveConfiguration(func(candidate *viper.Viper) {
		setConfigurationSettings(candidate, settings)
	}); err != nil {
		return fmt.Errorf("saving configuration settings: %w", err)
	}

	*uc.AudioConfiguration = settings.Audio
	*uc.DisplayConfiguration = settings.Display
	*uc.GameplayConfiguration = settings.Gameplay
	uc.syncLegacyDisplayModel()

	return nil
}

func isZeroDisplayConfiguration(configuration DisplayConfiguration) bool {
	return configuration.Mode == "" && configuration.WindowWidth == 0 &&
		configuration.WindowHeight == 0 && configuration.TargetFPS == 0 &&
		!configuration.EnableFpsLimiting
}

func setConfigurationSettings(configuration *viper.Viper, settings ConfigurationSettings) {
	setAudioConfiguration(configuration, settings.Audio)
	setDisplayConfiguration(configuration, settings.Display)
	setGameplayConfiguration(configuration, settings.Gameplay)
}

func setAudioConfiguration(configuration *viper.Viper, audio AudioConfiguration) {
	configuration.Set("Audio.MusicVolume", audio.MusicVolume)
	configuration.Set("Audio.SoundEffectVolume", audio.SoundEffectVolume)
	configuration.Set("Audio.AmbientVolume", audio.AmbientVolume)
}

func setDisplayConfiguration(configuration *viper.Viper, displayConfiguration DisplayConfiguration) {
	configuration.Set("Display.Mode", displayConfiguration.Mode.String())
	configuration.Set("Display.WindowWidth", displayConfiguration.WindowWidth)
	configuration.Set("Display.WindowHeight", displayConfiguration.WindowHeight)
	configuration.Set("Display.TargetFPS", displayConfiguration.TargetFPS)
	configuration.Set("Display.EnableFpsLimiting", displayConfiguration.EnableFpsLimiting)
}

func setGameplayConfiguration(configuration *viper.Viper, gameplay GameplayConfiguration) {
	configuration.Set(gameplayCombatSpeedKey, gameplay.CombatSpeedFactor)
	configuration.Set(gameplayConfirmQuickSaveKey, gameplay.ConfirmQuickSave)
	configuration.Set(gameplayShowCombatHPBarsKey, gameplay.ShowCombatHPBars)
	configuration.Set(gameplayMaxActionPopupsKey, gameplay.MaxVisibleActionPopups)
	configuration.Set(gameplayBatchMerchantTheftKey, gameplay.BatchMerchantTheft)
	configuration.Set(
		"Gameplay.Combat.AllowRepeatedMonsterDivision",
		gameplay.AllowRepeatedMonsterDivision,
	)
	configuration.Set(gameplayShowConcealedTurnOrderSlotsKey, gameplay.ShowConcealedTurnOrderSlots)
	configuration.Set(
		gameplayPersistHiddenDungeonDiscoveriesKey,
		gameplay.PersistHiddenDungeonDiscoveries,
	)
	configuration.Set(gameplayShowSubduedUnlitDungeonsKey, gameplay.ShowSubduedUnlitDungeons)
	configuration.Set(gameplaySmoothSurfaceMovementKey, gameplay.SmoothSurfaceMovement)
	configuration.Set(gameplayFlexibleSextantUseKey, gameplay.FlexibleSextantUse)
}

func writeViperConfigAtomically(configuration *viper.Viper, configPath string) error {
	if configPath == "" {
		return ErrConfigFileUnknown
	}

	directory := filepath.Dir(configPath)
	temporary, err := os.CreateTemp(
		directory,
		"."+filepath.Base(configPath)+"-*.yaml",
	)
	if err != nil {
		return fmt.Errorf("creating temporary configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		if cleanupErr := os.Remove(temporaryPath); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			logging.Warnf(logging.CategoryConfig, "Removing temporary configuration %s: %v", temporaryPath, cleanupErr)
		}
	}()
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("closing temporary configuration: %w", err)
	}
	if err := configuration.WriteConfigAs(temporaryPath); err != nil {
		return fmt.Errorf("writing temporary configuration: %w", err)
	}
	if fileInfo, statErr := os.Stat(configPath); statErr == nil {
		if err := os.Chmod(temporaryPath, fileInfo.Mode().Perm()); err != nil {
			return fmt.Errorf("preserving configuration permissions: %w", err)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("reading configuration permissions: %w", statErr)
	}
	if err := os.Rename(temporaryPath, configPath); err != nil {
		return fmt.Errorf("replacing configuration: %w", err)
	}

	return nil
}

// SaveLoggingConfiguration immediately persists current logging settings to config file
func (uc *UltimaVConfiguration) SaveLoggingConfiguration() error {
	if uc.LoggingConfiguration == nil {
		return ErrLoggingConfigNil
	}

	if err := uc.saveConfiguration(func(candidate *viper.Viper) {
		setLoggingConfiguration(candidate, *uc.LoggingConfiguration)
	}); err != nil {
		return fmt.Errorf("saving logging configuration: %w", err)
	}

	return nil
}

// SaveDebugConfiguration immediately persists current debug settings to config file
func (uc *UltimaVConfiguration) SaveDebugConfiguration() error {
	if uc.DebugConfiguration == nil {
		return ErrDebugConfigNil
	}

	if err := uc.saveConfiguration(func(candidate *viper.Viper) {
		setDebugConfiguration(candidate, *uc.DebugConfiguration)
	}); err != nil {
		return fmt.Errorf("saving debug configuration: %w", err)
	}

	return nil
}

func setLoggingConfiguration(configuration *viper.Viper, logging LoggingConfiguration) {
	// Keep the detached candidate in the same mapping shape as decoded YAML.
	// Viper preserves a map[string]bool passed to Set rather than normalizing it.
	categoryStates := make(map[string]any, len(logging.CategoryStates))
	for category, enabled := range logging.CategoryStates {
		categoryStates[category] = enabled
	}
	configuration.Set("Logging.Enabled", logging.Enabled)
	configuration.Set("Logging.StdoutEnabled", logging.StdoutEnabled)
	configuration.Set("Logging.MinLevel", logging.MinLevel)
	configuration.Set("Logging.CategoryStates", categoryStates)
}

func setDebugConfiguration(configuration *viper.Viper, debug DebugConfiguration) {
	if debug.CombatDetailSet {
		configuration.Set("Debug.CombatDetailShown", debug.CombatDetailShown)
	}
}

// SetAllowRepeatedMonsterDivision updates and persists the repeated-division rule atomically.
func (uc *UltimaVConfiguration) SetAllowRepeatedMonsterDivision(enabled bool) error {
	return uc.saveGameplayPreference(func(settings *GameplayConfiguration) {
		settings.AllowRepeatedMonsterDivision = enabled
	})
}

// SetPersistHiddenDungeonDiscoveries updates and persists hidden-discovery retention atomically.
func (uc *UltimaVConfiguration) SetPersistHiddenDungeonDiscoveries(enabled bool) error {
	return uc.saveGameplayPreference(func(settings *GameplayConfiguration) {
		settings.PersistHiddenDungeonDiscoveries = enabled
	})
}

// SetShowSubduedUnlitDungeons updates and persists subdued unlit rendering atomically.
func (uc *UltimaVConfiguration) SetShowSubduedUnlitDungeons(enabled bool) error {
	return uc.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.ShowSubduedUnlitDungeons = enabled })
}

// SetSmoothSurfaceMovement updates and persists optional surface scrolling atomically.
func (uc *UltimaVConfiguration) SetSmoothSurfaceMovement(enabled bool) error {
	return uc.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.SmoothSurfaceMovement = enabled })
}

// SetFlexibleSextantUse updates and persists optional sextant navigation atomically.
func (uc *UltimaVConfiguration) SetFlexibleSextantUse(enabled bool) error {
	return uc.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.FlexibleSextantUse = enabled })
}

// configuredBoolean replaces value with the configured setting, if one is set.
func (uc *UltimaVConfiguration) configuredBoolean(value *bool, key string) {
	if uc.source.IsSet(key) {
		*value = uc.source.GetBool(key)
	}
}

// GetFileWithFullPath joins a filename to the active original-data folder.
func (uc *UltimaVConfiguration) GetFileWithFullPath(fileName string) string {
	return files.Resolve(uc.SavedConfigData.DataFilePath, fileName)
}

// GetAllNpcFilePaths returns the four small-map NPC data file paths.
func (uc *UltimaVConfiguration) GetAllNpcFilePaths() []string {
	return []string{
		uc.GetFileWithFullPath(files.TowneNpc),
		uc.GetFileWithFullPath(files.DwellingNpc),
		uc.GetFileWithFullPath(files.CastleNpc),
		uc.GetFileWithFullPath(files.KeepNpc),
	}
}

// GetAmbientLightRadius returns the ambient visibility radius with fallback to default.
// This is the radius for PlayerLightAmbient - minimal night vision when no torch/magic.
func (uc *UltimaVConfiguration) GetAmbientLightRadius() int {
	if uc.AmbientLightRadius < 1 {
		return defaultAmbientLightRadius
	}

	return uc.AmbientLightRadius
}
