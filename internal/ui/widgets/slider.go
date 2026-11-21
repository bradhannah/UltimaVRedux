package widgets

import (
	"image"
	stdcolor "image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

// Slider represents a horizontal slider widget with discrete values (1-10 scale)
// Supports keyboard navigation and value change callbacks
type Slider struct {
	// Core properties
	minValue     int
	maxValue     int
	currentValue int
	label        string

	// Display properties
	font      *text.UltimaFont
	output    *text.Output
	placement sprites.PercentBasedPlacement
	bounds    *image.Rectangle

	// Input handling
	keyboard *input.Keyboard

	// Visual customization
	trackColor      stdcolor.Color
	handleColor     stdcolor.Color
	labelColor      stdcolor.Color
	backgroundColor stdcolor.Color

	// State
	hasFocus  bool
	isVisible bool

	// Callback for value changes
	onValueChanged func(oldValue, newValue int)

	// Reusable draw options for safety-first performance
	drawOp *ebiten.DrawImageOptions
}

// SliderConfig provides configuration for creating a slider widget
type SliderConfig struct {
	// Screen placement using percentage-based positioning
	Placement sprites.PercentBasedPlacement

	// Value range and initial state
	MinValue     int    // Minimum value (typically 1)
	MaxValue     int    // Maximum value (typically 10)
	InitialValue int    // Starting value
	Label        string // Display label

	// Input handling
	Keyboard *input.Keyboard // Keyboard handler for value changes

	// Visual customization (optional - safe defaults provided)
	TrackColor      stdcolor.Color // Slider track color
	HandleColor     stdcolor.Color // Slider handle color
	LabelColor      stdcolor.Color // Label text color
	BackgroundColor stdcolor.Color // Background color

	// Callback functions (optional)
	OnValueChanged func(oldValue, newValue int) // Called when value changes

}

const (
	sliderMaxCharsPerLine = 20
	sliderTrackMargin     = 5 // Pixels of empty margin at each end of the track
	sliderFocusThickness  = 2 // Pixel width of the focus border
)

// normalizeSliderConfig validates the config, logging and correcting bad values, and fills in defaults.
func normalizeSliderConfig(config SliderConfig) SliderConfig {
	// Error handling: validate required inputs
	if config.Keyboard == nil {
		logging.Warnf(logging.CategoryUI, "NewSlider called with nil keyboard, slider may not respond to input")
	}

	if config.Label == "" {
		logging.Warnf(logging.CategoryUI, "NewSlider called with empty label, using 'Slider'")
		config.Label = "Slider"
	}

	// Validate value range
	if config.MinValue >= config.MaxValue {
		logging.Warnf(logging.CategoryUI, "NewSlider MinValue %d >= MaxValue %d, using 1-10 range",
			config.MinValue, config.MaxValue)
		config.MinValue = 1
		config.MaxValue = 10
	}

	// Validate initial value
	if config.InitialValue < config.MinValue || config.InitialValue > config.MaxValue {
		logging.Warnf(logging.CategoryUI, "NewSlider InitialValue %d out of range [%d, %d], clamping",
			config.InitialValue, config.MinValue, config.MaxValue)
		if config.InitialValue < config.MinValue {
			config.InitialValue = config.MinValue
		} else {
			config.InitialValue = config.MaxValue
		}
	}

	// Apply default colors if not specified
	if config.TrackColor == nil {
		config.TrackColor = color.SliderTrack.RGBA() // Blue track
	}
	if config.HandleColor == nil {
		config.HandleColor = color.SliderHandle.RGBA() // White handle
	}
	if config.LabelColor == nil {
		config.LabelColor = color.SliderLabel.RGBA() // White text
	}
	if config.BackgroundColor == nil {
		config.BackgroundColor = color.SliderBackground.RGBA() // Dark gray background
	}

	return config
}

// NewSlider creates a new slider widget with safe defaults and validation.
//
// The slider supports discrete values within a specified range, keyboard navigation
// for value changes, and customizable visual styling.
//
// Parameters:
//   - config: Configuration for the slider widget
//
// Returns a configured Slider that implements the Widget interface.
func NewSlider(config SliderConfig) *Slider {
	config = normalizeSliderConfig(config)

	widget := &Slider{
		font:            nil,
		output:          nil,
		bounds:          nil,
		drawOp:          nil,
		minValue:        config.MinValue,
		maxValue:        config.MaxValue,
		currentValue:    config.InitialValue,
		label:           config.Label,
		placement:       config.Placement,
		keyboard:        config.Keyboard,
		trackColor:      config.TrackColor,
		handleColor:     config.HandleColor,
		labelColor:      config.LabelColor,
		backgroundColor: config.BackgroundColor,
		onValueChanged:  config.OnValueChanged,
		hasFocus:        true,
		isVisible:       true,
	}

	// Initialize display components using centralized typography system
	widget.font, widget.output = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		MaxLines:              1, // Single line for label
		MaxChars:              sliderMaxCharsPerLine,
		LineSpacingMultiplier: 0,
	})

	// Initialize reusable draw options for safety-first performance
	widget.drawOp = &ebiten.DrawImageOptions{}

	return widget
}

// Update processes keyboard input for value changes.
func (s *Slider) Update() {
	if !s.isVisible || !s.hasFocus || s.keyboard == nil {
		return
	}

	if s.keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft) {
		s.decreaseValue()
	}
	if s.keyboard.IsKeyPressedAndRegister(ebiten.KeyRight) {
		s.increaseValue()
	}
}

// Draw renders the slider to the screen
func (s *Slider) Draw(screen *ebiten.Image) {
	// Error handling: validate screen parameter
	if screen == nil {
		logging.Warnf(logging.CategoryUI, "Slider.Draw called with nil screen, skipping draw")

		return
	}

	if !s.isVisible {
		return
	}

	// Calculate slider areas
	sliderRect := s.getBounds()

	// Draw slider track and handle only (no label - drawn separately)
	s.drawSliderTrack(screen, sliderRect)
	s.drawSliderHandle(screen, sliderRect)

	// Draw focus indicator if focused
	if s.hasFocus {
		s.drawFocusIndicator(screen)
	}
}

// drawSliderTrack draws the horizontal track
func (s *Slider) drawSliderTrack(screen *ebiten.Image, sliderRect image.Rectangle) {
	// Calculate track position (horizontal line in middle)
	trackY := sliderRect.Min.Y + sliderRect.Dy()/halfDivisor
	trackHeight := 8                                    // Thicker track for better visibility
	trackStartX := sliderRect.Min.X + sliderTrackMargin // Small left margin only
	trackEndX := sliderRect.Max.X - sliderTrackMargin   // Small right margin only

	// Draw track as solid blue rectangle
	for y := trackY - trackHeight/halfDivisor; y < trackY+trackHeight/halfDivisor; y++ {
		for x := trackStartX; x < trackEndX; x++ {
			screen.Set(x, y, s.trackColor)
		}
	}
}

// drawSliderHandle draws the slider handle at current value position
func (s *Slider) drawSliderHandle(screen *ebiten.Image, sliderRect image.Rectangle) {
	// Calculate handle position
	trackStartX := sliderRect.Min.X + sliderTrackMargin // Match track start position
	trackEndX := sliderRect.Max.X - sliderTrackMargin   // Match track end position
	trackWidth := trackEndX - trackStartX

	// Calculate handle position based on current value
	valueRange := s.maxValue - s.minValue
	if valueRange <= 0 {
		return
	}

	valueOffset := float64(s.currentValue-s.minValue) / float64(valueRange)
	handleX := trackStartX + int(float64(trackWidth)*valueOffset)

	handleY := sliderRect.Min.Y + sliderRect.Dy()/halfDivisor
	handleSize := 12 // Larger handle for better visibility

	// Draw handle as white square
	for y := handleY - handleSize/halfDivisor; y < handleY+handleSize/halfDivisor; y++ {
		for x := handleX - handleSize/halfDivisor; x < handleX+handleSize/halfDivisor; x++ {
			if x >= sliderRect.Min.X && x < sliderRect.Max.X &&
				y >= sliderRect.Min.Y && y < sliderRect.Max.Y {
				screen.Set(x, y, s.handleColor)
			}
		}
	}
}

// drawFocusIndicator draws a focus indicator around the slider
func (s *Slider) drawFocusIndicator(screen *ebiten.Image) {
	sliderRect := s.getBounds()

	// Draw a bright white border around the slider when focused
	focusColor := color.SliderFocus.RGBA() // Bright white

	// Draw top border
	for x := sliderRect.Min.X; x < sliderRect.Max.X; x++ {
		screen.Set(x, sliderRect.Min.Y, focusColor)
		screen.Set(x, sliderRect.Min.Y+1, focusColor)
	}

	// Draw bottom border
	for x := sliderRect.Min.X; x < sliderRect.Max.X; x++ {
		screen.Set(x, sliderRect.Max.Y-1, focusColor)
		screen.Set(x, sliderRect.Max.Y-sliderFocusThickness, focusColor)
	}

	// Draw left border
	for y := sliderRect.Min.Y; y < sliderRect.Max.Y; y++ {
		screen.Set(sliderRect.Min.X, y, focusColor)
		screen.Set(sliderRect.Min.X+1, y, focusColor)
	}

	// Draw right border
	for y := sliderRect.Min.Y; y < sliderRect.Max.Y; y++ {
		screen.Set(sliderRect.Max.X-1, y, focusColor)
		screen.Set(sliderRect.Max.X-sliderFocusThickness, y, focusColor)
	}
}

// increaseValue increases the current value by 1 (with bounds checking)
func (s *Slider) increaseValue() {
	oldValue := s.currentValue
	if s.currentValue < s.maxValue {
		s.currentValue++
		s.notifyValueChanged(oldValue, s.currentValue)
	}
}

// decreaseValue decreases the current value by 1 (with bounds checking)
func (s *Slider) decreaseValue() {
	oldValue := s.currentValue
	if s.currentValue > s.minValue {
		s.currentValue--
		s.notifyValueChanged(oldValue, s.currentValue)
	}
}

// notifyValueChanged calls the value change callback if set
func (s *Slider) notifyValueChanged(oldValue, newValue int) {
	if s.onValueChanged != nil && oldValue != newValue {
		s.onValueChanged(oldValue, newValue)
	}
}

// GetValue returns the current slider value
func (s *Slider) GetValue() int {
	return s.currentValue
}

// SetValue sets the slider value programmatically (with bounds checking and callback)
func (s *Slider) SetValue(value int) {
	oldValue := s.currentValue

	// Clamp value to valid range
	if value < s.minValue {
		value = s.minValue
	}
	if value > s.maxValue {
		value = s.maxValue
	}

	s.currentValue = value
	s.notifyValueChanged(oldValue, s.currentValue)
}

// HasFocus returns whether the slider has keyboard focus
func (s *Slider) HasFocus() bool {
	return s.hasFocus
}

// SetFocus sets whether the slider has keyboard focus
func (s *Slider) SetFocus(focused bool) {
	s.hasFocus = focused
}

// SetBounds updates the slider's placement in logical screen pixels.
func (s *Slider) SetBounds(bounds image.Rectangle) {
	s.bounds = &bounds
}

// GetBounds returns the slider's current logical screen bounds.
func (s *Slider) GetBounds() image.Rectangle {
	return s.getBounds()
}

func (s *Slider) getBounds() image.Rectangle {
	if s.bounds != nil {
		return *s.bounds
	}

	return *sprites.GetRectangleFromPercents(s.placement)
}
