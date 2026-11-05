package widgets

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func guardResourceTestOptions() []CharacterSelectionOption {
	options := make([]CharacterSelectionOption, 0, 7)
	for index, condition := range []struct {
		name    string
		kind    party_state.CharacterStatus
		state   CharacterSelectionState
		details []string
	}{
		{"Good", party_state.Good, CharacterSelectionAvailable, []string{"Standing guard means no rest HP or MP recovery."}},
		{"Poisoned", party_state.Poisoned, CharacterSelectionWarning, []string{"A poisoned member cannot keep watch.", "Choosing them camps without a watch."}},
		{"Asleep", party_state.Sleep, CharacterSelectionUnavailable, []string{"This character is asleep and cannot act."}},
		{"Dead", party_state.Dead, CharacterSelectionUnavailable, []string{"This character is dead and cannot act."}},
		{"Charmed", party_state.Charmed, CharacterSelectionUnavailable, []string{"This character is charmed and cannot act."}},
		{"Good", party_state.Good, CharacterSelectionAvailable, []string{"Standing guard means no rest HP or MP recovery."}},
	} {
		options = append(options, CharacterSelectionOption{
			CharacterIndex: index, PartyPosition: index + 1, Name: strings.Repeat("W", party_state.NMaxPlayerNameSize),
			Status: condition.name, StatusKind: condition.kind, State: condition.state,
			Resources: &CharacterSelectionResources{CurrentHP: 65535, MaximumHP: 65535, CurrentMP: 255, MaximumMP: 255},
			Details:   condition.details,
		})
	}
	options[3].Resources.CurrentHP = 0
	options[2].Resources.CurrentMP = 0
	options[5].Resources.CurrentMP, options[5].Resources.MaximumMP = 0, 0
	options[0].Active = true
	for index := range options {
		resources := options[index].Resources
		options[index].Facts = []CharacterSelectionFact{
			{Label: "HP", Value: fmt.Sprintf("%d/%d", resources.CurrentHP, resources.MaximumHP)},
			{Label: "MP", Value: fmt.Sprintf("%d/%d", resources.CurrentMP, resources.MaximumMP)},
		}
	}

	return append(options, CharacterSelectionOption{
		CharacterIndex: -1, PartyPosition: 0, Name: "No One", State: CharacterSelectionAvailable,
		Details: []string{"The party camps without a watch.", "Sleeping members may enter an ambush asleep."},
	})
}

func newGuardResourceTestDialog(options []CharacterSelectionOption, onSelect func(int), onCancel func()) *CharacterSelectionDialog {
	return NewCharacterSelectionDialog(CharacterSelectionConfig{
		Title: "Guard: Choose Character", GuardResources: true, Options: options,
		OnSelect: onSelect, OnCancel: onCancel, CancelLabel: "Cancel camping",
		Keyboard: newWidgetTestKeyboard(0), SpriteSheet: &sprites.SpriteSheet{}, Placement: *createMockPlacement(),
	})
}

func guardResourceTestView(dialog *CharacterSelectionDialog) characterRosterDialogView {
	return characterRosterDialogView{
		title: dialog.title, rosterTitle: "PARTY", options: dialog.options,
		selected: dialog.selected, footer: dialog.footer(),
	}
}

func TestGuardResourcesValuesRemainExactAndNoOneBlank(t *testing.T) {
	options := guardResourceTestOptions()
	options[0].Facts = []CharacterSelectionFact{{Label: "HP", Value: "not resource data"}}
	require.Equal(t, [3]string{"65535/65535", "255/255", "Good"}, characterGuardResourceValues(options[0]))
	require.Equal(t, [3]string{"0/65535", "255/255", "Dead"}, characterGuardResourceValues(options[3]))
	require.Equal(t, [3]string{"65535/65535", "0/255", "Asleep"}, characterGuardResourceValues(options[2]))
	require.Equal(t, [3]string{"65535/65535", "0/0", "Good"}, characterGuardResourceValues(options[5]))
	require.Equal(t, [3]string{}, characterGuardResourceValues(options[6]))
}

func TestGuardResourcesMeasuredLayoutAllRowsAtEveryResolution(t *testing.T) {
	dialog := newGuardResourceTestDialog(guardResourceTestOptions(), func(int) {}, func() {})
	dialog.HandleNumber(4)
	for _, resolution := range []image.Point{
		{X: 320, Y: 200}, {X: 640, Y: 480}, {X: 800, Y: 600}, {X: 1024, Y: 768},
		{X: 1280, Y: 720}, {X: 1366, Y: 768}, {X: 1920, Y: 1080}, {X: 2560, Y: 1440}, {X: 3840, Y: 2160},
	} {
		t.Run(resolution.String(), func(t *testing.T) {
			setResponsiveDialogTestResolution(t, resolution.X, resolution.Y)
			shell := dialog.shell
			shell.ensureGuardResourceLayout(resolution.X, resolution.Y, guardResourceTestView(dialog))
			layout := shell.layout
			workspace := image.Rect(0, 0, int(float64(resolution.X)*createMockPlacement().EndPercentX), resolution.Y)
			require.True(t, layout.panelRect.In(workspace), "%v outside %v", layout.panelRect, workspace)
			require.Equal(t, 3, dialog.GetCurrentSelection(), "resize preserves inspected unavailable row")
			require.Len(t, layout.rowRects, 7)
			require.Equal(t, resolution.X == 320, shell.guardResourcesCompact,
				"minimum viewport uses two-line rows; larger viewports keep the ordinary columns")
			require.True(t, shell.guardResourceLayoutFits(resolution.X, resolution.Y, guardResourceTestView(dialog)),
				"all complete content must fit, not merely survive clipping")
			require.GreaterOrEqual(t, shell.bodyFont.TextFace.Size, float64(guardResourceMinimumFontPoint))
			titleWidth, _ := shell.titleOutput.Font.MeasureString(dialog.title)
			require.LessOrEqual(t, titleWidth, float64(layout.headerRect.Dx()))
			for index, option := range dialog.options {
				row := layout.rowRects[index]
				require.True(t, row.In(layout.rosterBodyRect))
				require.GreaterOrEqual(t, shell.rowOutput.GetLineCapacity(row.Dy()), 1)
				if index > 0 {
					require.GreaterOrEqual(t, row.Min.Y, layout.rowRects[index-1].Max.Y)
				}
				columns := shell.guardResourceColumns(row, dialog.options)
				nameWidth, _ := shell.bodyFont.MeasureString(">X6 " + option.Name + "*")
				require.GreaterOrEqual(t, columns[0].Dx(), int(math.Ceil(nameWidth)), "complete name and markers")
				shell.appendRosterRow(option, true, columns[0].Dx())
				require.Contains(t, shell.rowOutput.GetOutputStr(false), option.Name,
					"the complete identity must survive rendering preparation")
				require.NotContains(t, shell.rowOutput.GetOutputStr(false), "...", "compact rows do not ellipsize names")
				for column, label := range [4]string{"Name", "HP", "MP", "Condition"} {
					labelWidth, _ := shell.headerOutput.Font.MeasureString(label)
					require.LessOrEqual(t, labelWidth, float64(columns[column].Dx()))
					require.True(t, columns[column].In(row))
					require.GreaterOrEqual(t, shell.rowOutput.GetLineCapacity(columns[column].Dy()), 1)
					for _, other := range columns[column+1:] {
						require.False(t, columns[column].Overlaps(other), "all identity/resource/condition cells are disjoint")
					}
				}
				for field, value := range characterGuardResourceValues(option) {
					rect := columns[field+1]
					width, _ := shell.bodyFont.MeasureString(value)
					require.True(t, rect.In(row))
					require.GreaterOrEqual(t, rect.Dx(), int(math.Ceil(width)), "complete %q", value)
				}
				if shell.guardResourcesCompact {
					require.Less(t, columns[0].Max.X, columns[3].Min.X, "identity and condition share the first line")
					require.Less(t, columns[1].Max.X, columns[2].Min.X, "HP and MP share the second line")
					require.LessOrEqual(t, columns[0].Max.Y, columns[1].Min.Y)
					require.LessOrEqual(t, columns[3].Max.Y, columns[2].Min.Y)
				} else {
					for column := 1; column < len(columns); column++ {
						require.Less(t, columns[column-1].Max.X, columns[column].Min.X)
					}
				}
				shell.appendGuardDetail(option)
				require.LessOrEqual(t, shell.detailOutput.GetPanelContentLineCount(), shell.detailOutput.GetLineCapacity(layout.detailBodyRect.Dy()))
				wantDetail := ""
				for _, fact := range option.Facts {
					wantDetail += fact.Label + ": " + fact.Value + " "
				}
				wantDetail += strings.Join(option.Details, " ")
				require.Equal(t, strings.Fields(wantDetail), strings.Fields(shell.detailOutput.GetOutputStr(false)))
			}
			shell.appendGuardFooter(dialog.footer(), layout.footerRect.Inset(layout.padding).Dx())
			require.Equal(t, guardResourceFooterLines, shell.footerOutput.GetPanelContentLineCount())
			require.Equal(t, strings.Fields(dialog.footer()), strings.Fields(shell.footerOutput.GetOutputStr(false)))
			require.LessOrEqual(t, shell.footerOutput.GetPanelContentLineCount(), shell.footerOutput.GetLineCapacity(layout.footerRect.Inset(layout.padding).Dy()))
			require.LessOrEqual(t, layout.rosterBodyRect.Max.Y, layout.detailRect.Min.Y)
			require.LessOrEqual(t, layout.detailBodyRect.Max.Y, layout.footerRect.Min.Y)
		})
	}
}

func TestGuardResourcesInspectionAndExplicitConfirmation(t *testing.T) {
	selected, cancelled := 99, false
	dialog := newGuardResourceTestDialog(guardResourceTestOptions(), func(index int) { selected = index }, func() { cancelled = true })
	dialog.keyboard.SetForceWaitAnyKey(0)
	for range len(dialog.options) {
		before := dialog.GetCurrentSelection()
		dialog.HandleKeyDown()
		dialog.HandleKeyUp()
		require.Equal(t, before, dialog.GetCurrentSelection(), "both directions include unavailable rows")
		dialog.HandleKeyDown()
	}
	require.Equal(t, 0, dialog.GetCurrentSelection(), "traversal wraps through No One")
	for position := 0; position <= 6; position++ {
		dialog.keyboard.SimulateKeyPress(ebiten.Key(int(ebiten.Key0) + position))
		dialog.Update()
		want := position - 1
		require.Equal(t, want, dialog.GetCurrentSelection())
		require.Equal(t, 99, selected, "inspection must not confirm")
	}
	dialog.HandleNumber(4)
	dialog.HandleEnter()
	require.Equal(t, 99, selected)
	dialog.HandleNumber(2)
	dialog.HandleEnter()
	require.Equal(t, 1, selected, "warning remains confirmable; scene owns cancellation")
	dialog.HandleNumber(0)
	dialog.HandleEnter()
	require.Equal(t, -1, selected)
	dialog.HandleEscape()
	require.True(t, cancelled)
}

func TestGuardResourcesCompactTypographyUsesMeasuredHeight(t *testing.T) {
	dialog := newGuardResourceTestDialog(guardResourceTestOptions(), func(int) {}, func() {})
	dialog.HandleNumber(0)
	for _, fixture := range []struct {
		size        image.Point
		compact     bool
		fontPoint   float64
		footerLines int
	}{
		{image.Pt(320, 200), true, 6, 2},
		{image.Pt(320, 480), true, 8, 3},
		{image.Pt(640, 480), false, 8, 2},
		{image.Pt(320, 200), true, 6, 2},
	} {
		t.Run(fixture.size.String(), func(t *testing.T) {
			shell := dialog.shell
			view := guardResourceTestView(dialog)
			shell.ensureGuardResourceLayout(fixture.size.X, fixture.size.Y, view)
			require.Equal(t, fixture.compact, shell.guardResourcesCompact)
			require.Equal(t, fixture.fontPoint, shell.bodyFont.TextFace.Size,
				"use the largest fitting guard typography, rather than shrinking all narrow viewports")
			require.True(t, shell.guardResourceLayoutFits(fixture.size.X, fixture.size.Y, view))
			require.Equal(t, -1, dialog.GetCurrentSelection(), "resizing does not change No One inspection")
			shell.appendGuardFooter(view.footer, shell.layout.footerRect.Inset(shell.layout.padding).Dx())
			require.Equal(t, fixture.footerLines, shell.footerOutput.GetPanelContentLineCount())
			require.Equal(t, strings.Fields(view.footer),
				strings.Fields(shell.footerOutput.GetOutputStr(false)), "wrapping cannot overwrite a hint")
		})
	}
}

func TestGuardResourcesConditionColoursSurviveSelection(t *testing.T) {
	options := guardResourceTestOptions()
	dialog := newGuardResourceTestDialog(options, func(int) {}, func() {})
	shell := dialog.shell
	shell.ensureGuardResourceLayout(1280, 720, guardResourceTestView(dialog))
	for index, expected := range []pkgcolor.EGAColor{pkgcolor.EGAGreen, pkgcolor.EGAMagenta, pkgcolor.EGARed, pkgcolor.EGALightBlue} {
		option := options[index+1]
		for _, selected := range []bool{false, true} {
			shell.appendGuardResourceCell(option, selected, guardResourceConditionColumn, option.Status)
			_, found := shell.rowOutput.FirstPanelLineWithColor(0, 1, expected)
			require.True(t, found, "condition colour survives availability and focus")
			require.Equal(t, []string{option.Status}, strings.Fields(shell.rowOutput.GetOutputStr(false)))
			shell.appendGuardResourceCell(option, selected, 0, characterGuardResourceValues(option)[0])
			_, found = shell.rowOutput.FirstPanelLineWithColor(0, 1, characterRosterRowColor(option.State, selected))
			require.True(t, found, "resource values retain readable row contrast")
		}
	}
}

func TestGuardResourcesLayoutRequiresExplicitOptIn(t *testing.T) {
	options := guardResourceTestOptions()
	ordinary := newCharacterSelectionTestDialog(options, func(int) {})
	require.False(t, ordinary.shell.guardResources)
	require.False(t, ordinary.shell.healthRoster)
	options[0].Health = &CharacterSelectionHealth{Current: 1, Maximum: 2, Missing: 1}
	healing := newCharacterSelectionTestDialog(options, func(int) {})
	require.False(t, healing.shell.guardResources)
	require.True(t, healing.shell.healthRoster)
	guard := newGuardResourceTestDialog(options, func(int) {}, func() {})
	require.True(t, guard.shell.guardResources)
	require.False(t, guard.shell.healthRoster, "explicit guard layout takes precedence over optional healing data")
	reorder := createNewOrderTestDialog(createMockPartyState(), 1, func(int, int) {}, func() {})
	require.False(t, reorder.shell.guardResources)
}

func TestGuardResourcesNoOneFirstOrAloneKeepsExplicitLayout(t *testing.T) {
	setResponsiveDialogTestResolution(t, 320, 200)
	options := guardResourceTestOptions()
	noOne := options[len(options)-1]
	for _, rows := range [][]CharacterSelectionOption{
		append([]CharacterSelectionOption{noOne}, options[:len(options)-1]...),
		{noOne},
	} {
		selected := 99
		dialog := newGuardResourceTestDialog(rows, func(index int) { selected = index }, func() {})
		require.Equal(t, -1, dialog.GetCurrentSelection())
		require.True(t, dialog.shell.guardResources)
		require.False(t, dialog.shell.healthRoster)
		screen := ebiten.NewImage(320, 200)
		dialog.Draw(screen)
		screen.Deallocate()
		require.Len(t, dialog.shell.layout.rowRects, len(rows))
		require.Equal(t, [3]string{}, characterGuardResourceValues(dialog.GetCurrentOption()))
		dialog.HandleEnter()
		require.Equal(t, -1, selected)
	}
}

func TestGuardResourcesNativeRenderingKeepsCellsAndHUDContained(t *testing.T) {
	for _, resolution := range []image.Point{{X: 320, Y: 200}, {X: 1280, Y: 720}, {X: 3440, Y: 1440}} {
		t.Run(resolution.String(), func(t *testing.T) {
			setResponsiveDialogTestResolution(t, resolution.X, resolution.Y)
			dialog := newGuardResourceTestDialog(guardResourceTestOptions(), func(int) {}, func() {})
			dialog.HandleNumber(0)
			screen := ebiten.NewImage(resolution.X, resolution.Y)
			defer screen.Deallocate()
			dialog.Draw(screen)
			shell := dialog.shell
			require.True(t, renderedNontransparentBounds(screen, screen.Bounds()).In(shell.layout.panelRect),
				"rendering must not spill into the HUD or beyond the card")
			for index, option := range dialog.options {
				columns := shell.guardResourceColumns(shell.layout.rowRects[index], dialog.options)
				for field, rect := range columns {
					pixels := make([]byte, rect.Dx()*rect.Dy()*4)
					screen.SubImage(rect).(*ebiten.Image).ReadPixels(pixels)
					ink := 0
					for pixel := 0; pixel < len(pixels); pixel += 4 {
						if pixels[pixel] != 0 || pixels[pixel+1] != 0 || pixels[pixel+2] != 0 {
							ink++
						}
					}
					if option.Resources == nil && field > 0 {
						require.Zero(t, ink, "No One has blank resource and condition cells")
					} else {
						require.Positive(t, ink, "row %d column %d must render while No One is inspected", index, field)
					}
				}
			}
		})
	}
}
