package widgets

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	hoursPromptQuestion    = "Rest how many hours?"
	hoursPromptLabel       = "Hours    " // Pads the label so the arrows stand clear of it.
	hoursPromptKeys        = "Left/Right/1-9"
	hoursPromptLines       = 5
	hoursPromptChars       = 28
	hoursPromptLineSpacing = 1.2
	hoursPromptUsualRest   = 8
	restActionLabel        = "Rest"
)

var _ Widget = (*HoursPrompt)(nil)

// HoursPromptConfig configures the small box that asks how long to rest.
type HoursPromptConfig struct {
	Keyboard *input.Keyboard
	Most     int // The longest rest that can be asked for.
	// CenterX and CenterY place the box, as shares of the screen.
	CenterX, CenterY float64
	// WakeTime words the time the party would wake after the hours given.
	WakeTime func(hours int) string
	// ZeroCancels makes 0 and Space cancel too, as the original's dungeon prompt does.
	ZeroCancels bool
	// OnResult receives the hours chosen, or zero when the prompt is cancelled.
	OnResult func(hours int)
}

// HoursPrompt asks how many hours to rest: Left and Right step the hours, a digit
// types them, and one line says about when the party would wake.
type HoursPrompt struct {
	quantityInput
	config HoursPromptConfig
	hours  int
	done   bool

	output     *text.Output
	panel      image.Rectangle
	content    image.Rectangle
	pixelUnit  int
	lastWidth  int
	lastHeight int
}

// NewHoursPrompt creates the prompt at the usual night's rest, or the most allowed.
func NewHoursPrompt(config HoursPromptConfig) *HoursPrompt {
	if config.Keyboard == nil || config.Most < 1 || config.WakeTime == nil || config.OnResult == nil {
		panic("HoursPrompt: keyboard, a longest rest, a waking time and a result callback are required")
	}
	var empty image.Rectangle
	prompt := &HoursPrompt{
		quantityInput: newQuantityInput(config.Keyboard, config.Keyboard.IsKeyHeld, ebiten.KeyUp, ebiten.KeyDown),
		config:        config,
		hours:         min(hoursPromptUsualRest, config.Most),
		done:          false,
		output:        nil,
		panel:         empty,
		content:       empty,
		pixelUnit:     0,
		lastWidth:     0,
		lastHeight:    0,
	}
	prompt.ensureLayout()

	return prompt
}

// GetBounds returns the box's outer bounds.
func (p *HoursPrompt) GetBounds() image.Rectangle {
	p.ensureLayout()

	return p.panel
}

// Update steps or types the hours, and confirms or cancels.
func (p *HoursPrompt) Update() {
	if p.done {
		return
	}
	keyboard := p.config.Keyboard
	cancelled := keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape)
	if p.config.ZeroCancels {
		cancelled = cancelled || keyboard.IsKeyPressedAndRegister(ebiten.Key0) ||
			keyboard.IsKeyPressedAndRegister(ebiten.KeySpace)
	}
	if cancelled {
		p.finish(0)

		return
	}
	hours, owned := p.pollAmount(p.hours, p.config.Most)
	if hours != p.hours {
		p.hours = hours
		p.populate()
	}
	if owned {
		return
	}
	if keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) || keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		p.finish(p.hours)
	}
}

func (p *HoursPrompt) finish(hours int) {
	p.done = true
	p.onDialogFocusChanged(false)
	p.config.OnResult(hours)
}

// Draw renders the box over the map.
func (p *HoursPrompt) Draw(screen *ebiten.Image) {
	if screen == nil {
		return
	}
	p.ensureLayout()
	FillClassicPixelRoundedRect(screen, p.panel, p.pixelUnit, ucolor.DialogBackground.RGBA())
	p.output.DrawPanelContent(screen, p.content.Min, p.content.Dx(), etext.AlignStart)
	DrawClassicPixelRoundedFrame(screen, p.panel, p.pixelUnit, DialogRuleColor)
}

func (p *HoursPrompt) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	if width <= 0 || height <= 0 || width == p.lastWidth && height == p.lastHeight {
		return
	}
	p.lastWidth, p.lastHeight = width, height
	p.pixelUnit = getClassicUIPixelUnit(width, height)
	padding := max(inputPromptMinimumPadding, p.pixelUnit*inputPromptPaddingUnits)
	var font *text.UltimaFont
	font, p.output = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacingMultiplier: hoursPromptLineSpacing,
		MaxLines: hoursPromptLines, MaxChars: hoursPromptChars,
	})
	letter, _ := font.MeasureString("M")
	textWidth := int(math.Ceil(letter*hoursPromptChars)) + 1
	textHeight := int(math.Ceil(p.output.GetTotalHeightForLines(hoursPromptLines)))
	panelWidth, panelHeight := textWidth+padding*inputPromptCenterDivisor, textHeight+padding*inputPromptCenterDivisor
	centerX := int(math.Round(p.config.CenterX * float64(width)))
	centerY := int(math.Round(p.config.CenterY * float64(height)))
	minX := centeredPromptRectMinX(centerX, panelWidth, width, padding)
	minY := min(max(padding, centerY-panelHeight/inputPromptCenterDivisor), height-padding-panelHeight)
	p.panel = image.Rect(minX, minY, minX+panelWidth, minY+panelHeight)
	p.content = insetRectangle(p.panel, padding)
	p.populate()
}

func (p *HoursPrompt) populate() {
	p.output.Reset()
	p.output.AppendLine().Text(hoursPromptQuestion, DialogValueColor).Done()
	appendAmountStepper(p.output.AppendLine().Text(hoursPromptLabel, DialogValueColor), p.hours, p.config.Most).Done()
	p.output.AppendLine().Text("Wake about "+p.config.WakeTime(p.hours), DialogControlHintColor).Done()
	onlyOne := func(hint ControlHint) bool { return p.config.Most <= 1 && hint.Key == hoursPromptKeys }
	PackControlHintsDimming(p.output, []ControlHint{{Key: hoursPromptKeys, Action: "Hours"}}, p.content.Dx(),
		etext.AlignStart, onlyOne)
	PackControlHintsDimming(p.output, []ControlHint{
		{Key: hintKeyEnter, Action: restActionLabel}, {Key: hintKeyEsc, Action: defaultCancelActionLabel},
	}, p.content.Dx(), etext.AlignStart, nil)
}
