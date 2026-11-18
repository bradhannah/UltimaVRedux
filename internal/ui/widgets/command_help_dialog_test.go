package widgets

import (
	"image"
	"image/color"
	"strings"
	"testing"

	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestCommandHelpDialogSpacePagingAndNavigation(t *testing.T) {
	setGuidanceTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	closed := 0
	entries := make([]CommandHelpEntry, 100)
	for index := range entries {
		entries[index] = CommandHelpEntry{Key: "A", Purpose: "Attack", NextInput: "Choose direction."}
	}
	dialog := NewCommandHelpDialog(entries, HelpStatusView{}, keyboard, func() { closed++ })
	require.Equal(t, helpPageNow, dialog.page, "Help opens on the keys that work right now")
	dialog.showPage(helpPageCommands)
	press := func(key ebiten.Key) {
		keyboard.SimulateKeyPress(key)
		dialog.Update()
	}
	require.Zero(t, dialog.row)
	require.Greater(t, dialog.lastRow(), dialog.pageSize())
	require.Contains(t, dialog.footer.GetOutputStr(false), "[Space] Next")
	require.Contains(t, dialog.footer.GetOutputStr(false), "COMMAND HELP")
	require.Contains(t, dialog.footer.GetOutputStr(false), "[Esc] Close")
	press(ebiten.KeySpace)
	require.Equal(t, dialog.pageSize(), dialog.row)
	press(ebiten.KeyHome)
	press(ebiten.KeyPageDown)
	require.Equal(t, dialog.pageSize(), dialog.row)
	for range dialog.lastRow()/dialog.pageSize() + 2 {
		press(ebiten.KeySpace)
	}
	require.Equal(t, dialog.lastRow(), dialog.row, "Space must clamp, never wrap")
	press(ebiten.KeyPageUp)
	require.Equal(t, dialog.lastRow()-dialog.pageSize(), dialog.row)
	press(ebiten.KeyUp)
	require.Equal(t, dialog.lastRow()-dialog.pageSize()-1, dialog.row)
	press(ebiten.KeyDown)
	require.Equal(t, dialog.lastRow()-dialog.pageSize(), dialog.row)
	press(ebiten.KeyHome)
	press(ebiten.KeyPageUp)
	require.Zero(t, dialog.row)
	press(ebiten.KeyEnd)
	require.Equal(t, dialog.lastRow(), dialog.row)
	press(ebiten.KeyEscape)
	require.Equal(t, 1, closed)
}

func TestCommandHelpDialogDrawsOpaqueClassicFrame(t *testing.T) {
	setGuidanceTestResolution(t, 1280, 720)
	dialog := NewCommandHelpDialog(nil, HelpStatusView{}, newWidgetTestKeyboard(0), func() {})
	screen := ebiten.NewImage(dialog.width, dialog.height)
	screen.Fill(ucolor.EGAMagenta)
	bounds := dialog.border.GetBounds()
	interior := dialog.border.GetInteriorBounds()
	dialog.Draw(screen)
	require.Equal(t, bounds, dialog.border.GetBounds())
	require.Equal(t, interior, dialog.border.GetInteriorBounds())
	require.Equal(t, color.RGBA{R: 255, G: 255, B: 255, A: 255},
		color.RGBAModel.Convert(screen.At(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y)))
	require.Equal(t, color.RGBA{A: 255},
		color.RGBAModel.Convert(screen.At(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2)))
	require.Equal(t, color.RGBAModel.Convert(ucolor.EGAMagenta),
		color.RGBAModel.Convert(screen.At(bounds.Min.X, bounds.Min.Y)), "stepped corner stays outside the card")
}

func TestHelpStatusIsDetachedAndPreservesCommandReadingPosition(t *testing.T) {
	setGuidanceTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(-1)
	entries := make([]CommandHelpEntry, 60)
	for index := range entries {
		entries[index] = CommandHelpEntry{Key: "A", Purpose: "Attack", NextInput: "Choose direction."}
	}
	icon := ebiten.NewImage(16, 16)
	t.Cleanup(icon.Deallocate)
	icon.Fill(ucolor.EGAWhite)
	status := HelpStatusView{Food: 96, Effects: []StatusHelpEntry{{Name: "Torch", Description: "Lights nearby ground.",
		Icon: icon, Tint: [3]float32{1, 1, 1}}}}
	dialog := NewCommandHelpDialog(entries, status, keyboard, func() {})
	dialog.showPage(helpPageCommands)
	status.Effects[0].Name = "Changed externally"
	keyboard.SimulateKeyPress(ebiten.KeyPageDown)
	dialog.Update()
	commandRow := dialog.row
	keyboard.SimulateKeyPress(ebiten.KeyTab)
	dialog.Update()
	require.Equal(t, helpPageStatus, dialog.page)
	require.Contains(t, dialog.output.GetOutputStr(true), "Torch")
	require.Contains(t, dialog.output.GetOutputStr(true), "None worn.")
	require.Contains(t, dialog.output.GetOutputStr(true), "Food remaining: 96")
	require.NotContains(t, dialog.output.GetOutputStr(true), "Changed externally")
	require.True(t, dialog.contentBounds().In(dialog.border.GetInteriorBounds()))
	keyboard.SimulateKeyPress(ebiten.KeyTab)
	dialog.Update()
	require.Equal(t, helpPageNow, dialog.page, "Tab wraps from Status to Right now")
	keyboard.SimulateKeyPress(ebiten.KeyTab)
	dialog.Update()
	require.Equal(t, commandRow, dialog.row)
}

// Help opens on the keys that work now, grouped, and names the rest.
func TestCommandHelpOpensOnKeysThatWorkRightNow(t *testing.T) {
	setGuidanceTestResolution(t, 1280, 720)
	dialog := NewCommandHelpDialog([]CommandHelpEntry{
		{Key: "A", Purpose: "Attack", Group: "MOVE AND ACT"},
		{Key: "T", Purpose: "Talk", Group: "SURROUNDINGS", Unavailable: "Not available during combat."},
		{Key: "F2", Purpose: "Combat details", Group: "BATTLE PANELS"},
	}, HelpStatusView{}, newWidgetTestKeyboard(0), func() {})
	page := strings.Join(strings.Fields(dialog.output.GetOutputStr(false)), " ")
	require.Contains(t, page, "MOVE AND ACT [A] Attack BATTLE PANELS [F2] Combat details")
	require.Contains(t, page, "Not available here: T.")
	require.NotContains(t, page, "SURROUNDINGS", "a group with no usable key is not headed")
	require.Equal(t, "Right now    Commands    Status", dialog.header.GetOutputStr(false))
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	dialog.Draw(screen)
	// The open page is marked by the shared plate, which lies under its own title only.
	bounds := dialog.border.GetInteriorBounds()
	plate := image.Rectangle{Min: image.Pt(bounds.Max.X, bounds.Max.Y), Max: bounds.Min}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if color.RGBAModel.Convert(screen.At(x, y)) == color.RGBAModel.Convert(DialogActiveTabBackground) {
				plate = plate.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	require.Less(t, plate.Max.X, bounds.Min.X+bounds.Dx()/2, "Right now is the first title on the centred line")
	require.Greater(t, plate.Dx(), plate.Dy())
}
