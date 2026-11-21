package widgets

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func newTestHoursPrompt(keyboard *input.Keyboard, zeroCancels bool, result *int) *HoursPrompt {
	return NewHoursPrompt(HoursPromptConfig{
		Keyboard: keyboard, Most: 9, CenterX: DialogHorizontalMapOnly.ToPercentage(), CenterY: 0.7,
		WakeTime:    func(hours int) string { return fmt.Sprintf("%d:03AM", hours) },
		ZeroCancels: zeroCancels,
		OnResult:    func(hours int) { *result = hours },
	})
}

func pressHoursPromptKey(prompt *HoursPrompt, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	prompt.Update()
}

func TestHoursPromptStepsTypesAndSaysWhenThePartyWakes(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	result := -1
	prompt := newTestHoursPrompt(keyboard, false, &result)
	shown := func() string { return strings.Join(strings.Fields(outputText(prompt.output)), " ") }

	require.Equal(t, "Rest how many hours? Hours < 8 > of 9 Wake about 8:03AM "+
		"[Left/Right/1-9] Hours [Enter] Rest [Esc] Cancel", shown())
	pressHoursPromptKey(prompt, keyboard, ebiten.KeyRight)
	pressHoursPromptKey(prompt, keyboard, ebiten.KeyRight)
	require.Equal(t, 9, prompt.GetHours(), "the hours stop at the most")
	pressHoursPromptKey(prompt, keyboard, ebiten.KeyLeft)
	require.Equal(t, 8, prompt.GetHours())
	pressHoursPromptKey(prompt, keyboard, ebiten.Key3)
	require.Contains(t, shown(), "Hours < 3 > of 9 Wake about 3:03AM")
	pressHoursPromptKey(prompt, keyboard, ebiten.Key0)
	pressHoursPromptKey(prompt, keyboard, ebiten.KeySpace)
	require.Equal(t, 3, prompt.GetHours(), "on the surface 0 and Space ask for nothing")
	require.Equal(t, -1, result)

	pressHoursPromptKey(prompt, keyboard, ebiten.KeyEnter)
	require.Equal(t, 3, result)
	pressHoursPromptKey(prompt, keyboard, ebiten.KeyEnter)
	require.Equal(t, 3, result, "an answered prompt answers once")
}

func TestHoursPromptCancelsWithEscAndInADungeonWithZeroOrSpace(t *testing.T) {
	for name, test := range map[string]struct {
		key         ebiten.Key
		zeroCancels bool
	}{
		"Esc":             {key: ebiten.KeyEscape},
		"dungeon zero":    {key: ebiten.Key0, zeroCancels: true},
		"dungeon Space":   {key: ebiten.KeySpace, zeroCancels: true},
		"dungeon Esc too": {key: ebiten.KeyEscape, zeroCancels: true},
	} {
		t.Run(name, func(t *testing.T) {
			keyboard := newWidgetTestKeyboard(0)
			result := -1
			prompt := newTestHoursPrompt(keyboard, test.zeroCancels, &result)
			pressHoursPromptKey(prompt, keyboard, test.key)
			require.Zero(t, result, "a cancelled rest is zero hours")
		})
	}
}

func TestHoursPromptHoldsItsTextOverTheMapAtEverySupportedSize(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		setMerchantShellTestResolution(t, size.X, size.Y)
		result := -1
		prompt := newTestHoursPrompt(newWidgetTestKeyboard(0), true, &result)
		at := fmt.Sprintf("%dx%d", size.X, size.Y)
		bounds := prompt.GetBounds()
		require.True(t, bounds.In(image.Rect(0, 0, size.X, size.Y)), at)
		require.InDelta(t, DialogHorizontalMapOnly.ToPercentage()*float64(size.X), bounds.Min.X+bounds.Dx()/2, 2, at)
		require.Equal(t, hoursPromptLines, prompt.output.GetPanelContentLineCount(), "%s: a line wrapped", at)
		require.GreaterOrEqual(t, prompt.output.Font.CharsForWidth(prompt.content.Dx()), hoursPromptChars, at)
		for _, line := range strings.Split(outputText(prompt.output), "\n") {
			require.LessOrEqual(t, utf8.RuneCountInString(strings.TrimRight(line, " ")), hoursPromptChars, "%s: %q", at, line)
		}
		screen := ebiten.NewImage(size.X, size.Y)
		prompt.Draw(screen)
		requireImageRegionContainsColor(t, screen, prompt.content, DialogLabelColor)
		screen.Deallocate()
	}
}
