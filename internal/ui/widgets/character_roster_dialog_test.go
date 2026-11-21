package widgets

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCharacterRosterCompactRowMarkers(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog := newCharacterSelectionTestDialog(characterSelectionTestOptions(), func(int) {})
	shell := dialog.shell
	shell.ensureLayout(1280, 720, 1, "")
	tests := []struct {
		name     string
		state    CharacterSelectionState
		selected bool
		active   bool
		want     string
	}{
		{"available", CharacterSelectionAvailable, false, false, "  1 Brad"},
		{"selected", CharacterSelectionAvailable, true, false, "> 1 Brad"},
		{"warning", CharacterSelectionWarning, false, false, " !1 Brad"},
		{"unavailable", CharacterSelectionUnavailable, false, false, " X1 Brad"},
		{"locked", CharacterSelectionLocked, false, false, " L1 Brad"},
		{"active", CharacterSelectionAvailable, false, true, "  1 Brad*"},
		{"all markers", CharacterSelectionWarning, true, true, ">!1 Brad*"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			option := CharacterSelectionOption{
				PartyPosition: 1, Name: "Brad", State: test.state, Active: test.active,
			}
			shell.appendRosterRow(option, test.selected, shell.layout.rosterBodyRect.Dx())
			assert.Equal(t, test.want, shell.rowOutput.GetOutputStr(false))
		})
	}
}

func TestCharacterRosterNamesFitBesidePoisonedStatus(t *testing.T) {
	for _, size := range [][2]int{{320, 200}, {640, 480}, {1280, 720}, {1920, 1080}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			setResponsiveDialogTestResolution(t, size[0], size[1])
			dialog := newCharacterSelectionTestDialog(characterSelectionTestOptions(), func(int) {})
			shell := dialog.shell
			shell.ensureLayout(size[0], size[1], 1, "")
			row := shell.layout.rowRects[0]
			nameRect, statusRect := shell.rosterRowTextRects(row, "Poisoned")
			statusWidth, _ := shell.bodyFont.MeasureString("Poisoned")
			assert.GreaterOrEqual(t, float64(statusRect.Dx()), statusWidth)
			assert.Equal(t, row.Max.X, statusRect.Max.X)
			assert.Less(t, nameRect.Max.X, statusRect.Min.X)

			option := CharacterSelectionOption{PartyPosition: 1, Name: "Brad", State: CharacterSelectionAvailable}
			shell.appendRosterRow(option, true, nameRect.Dx())
			assert.Equal(t, "> 1 Brad", shell.rowOutput.GetOutputStr(false))

			option.Name = strings.Repeat("LongName", 20)
			option.State = CharacterSelectionWarning
			option.Active = true
			shell.appendRosterRow(option, true, nameRect.Dx())
			got := shell.rowOutput.GetOutputStr(false)
			require.True(t, strings.HasPrefix(got, ">!1 "), got)
			require.True(t, strings.HasSuffix(got, "*"), got)
			assert.Contains(t, got, "...")
			assert.NotContains(t, got, "\n")
			nameWidth, _ := shell.bodyFont.MeasureString(got)
			assert.LessOrEqual(t, nameWidth, float64(nameRect.Dx()))
			assert.Equal(t, 1, shell.rowOutput.GetPanelContentLineCount())
		})
	}
}

func TestHealingRosterColumnsFitAndHealthIsDetached(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {1920, 1080}, {3840, 2160}} {
		setResponsiveDialogTestResolution(t, size[0], size[1])
		options := characterSelectionTestOptions()
		for index := range options {
			options[index].Health = &CharacterSelectionHealth{Current: 400, Maximum: 800, Missing: 400}
			options[index].Status = "Poisoned"
		}
		dialog := newCharacterSelectionTestDialog(options, func(int) {})
		dialog.shell.ensureLayout(size[0], size[1], len(options), dialog.footer())
		columns := dialog.shell.healthColumns(dialog.shell.layout.rowRects[0], dialog.options)
		nameWidth, _ := dialog.shell.bodyFont.MeasureString("> 1 Geoffrey")
		require.GreaterOrEqual(t, float64(columns[0].Dx()), nameWidth)
		for index := 1; index < len(columns); index++ {
			require.Greater(t, columns[index].Min.X, columns[index-1].Max.X)
		}
		options[0].Health.Current = 0
		require.EqualValues(t, 400, dialog.GetOptions()[0].Health.Current)
	}
}

func TestCharacterRosterStatusColorIgnoresSelectionAndEligibility(t *testing.T) {
	tests := []struct {
		status party_state.CharacterStatus
		label  string
		color  color.Color
	}{
		{party_state.Poisoned, "Poisoned", pkgcolor.EGAGreen},
		{party_state.Sleep, "Asleep", pkgcolor.EGAMagenta},
		{party_state.Charmed, "Charmed", pkgcolor.EGALightBlue},
		{party_state.Dead, "Dead", pkgcolor.EGALightRed},
		{party_state.Good, "Good", nil},
		{0, "Unknown", nil},
		{0, "", nil},
	}
	for _, test := range tests {
		for _, state := range []CharacterSelectionState{
			CharacterSelectionAvailable, CharacterSelectionWarning, CharacterSelectionUnavailable, CharacterSelectionLocked,
		} {
			for _, selected := range []bool{false, true} {
				option := CharacterSelectionOption{Status: test.label, StatusKind: test.status, State: state}
				fallback := characterRosterRowColor(state, selected)
				want := test.color
				if want == nil {
					want = fallback
				}
				requireColorRGBA(t, want, option.statusColor(fallback))
				option.Status = "A display label must not determine its hue"
				requireColorRGBA(t, want, option.statusColor(fallback))
				assert.Equal(t, state == CharacterSelectionAvailable || state == CharacterSelectionWarning,
					option.confirmable())
			}
		}
	}
}

func TestCharacterRosterDrawPreservesStatusColorInBothPanes(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	tests := []struct {
		status party_state.CharacterStatus
		label  string
		color  color.Color
	}{
		{party_state.Poisoned, "Poisoned", pkgcolor.EGAGreen},
		{party_state.Sleep, "Asleep", pkgcolor.EGAMagenta},
		{party_state.Charmed, "Charmed", pkgcolor.EGALightBlue},
		{party_state.Dead, "Dead", pkgcolor.EGALightRed},
	}
	for _, test := range tests {
		for _, state := range []CharacterSelectionState{
			CharacterSelectionWarning, CharacterSelectionUnavailable,
		} {
			t.Run(fmt.Sprintf("%s/state%d", test.label, state), func(t *testing.T) {
				options := characterSelectionTestOptions()[:2]
				options[1].Status = test.label
				options[1].StatusKind = test.status
				options[1].State = state
				options[1].PortraitSprite = indexes.NoSprites
				dialog := newCharacterSelectionTestDialog(options, func(int) {})
				screen := ebiten.NewImage(1280, 720)
				defer screen.Deallocate()
				for _, position := range []int{1, 2} {
					dialog.HandleNumber(position)
					dialog.Draw(screen)
					layout := dialog.shell.layout
					nameRect, statusRect := dialog.shell.rosterRowTextRects(layout.rowRects[1], test.label)
					requireImageRegionContainsColor(t, screen, statusRect, test.color)
					requireImageRegionContainsColor(t, screen, nameRect, characterRosterRowColor(state, position == 2))
					if position == 2 {
						requireImageRegionContainsColor(t, screen, nameRect, pkgcolor.FocusIndicator.RGBA())
						requireImageRegionContainsColor(t, screen, layout.detailBodyRect, test.color)
						assert.Contains(t, dialog.shell.detailOutput.GetOutputStr(false), "Status: "+test.label)
					}
				}
			})
		}
	}
}

func TestCharacterRosterFooterShowsEveryHintAtSupportedSizes(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			setResponsiveDialogTestResolution(t, size.X, size.Y)
			dialog := newCharacterSelectionTestDialog(characterSelectionTestOptions(), func(int) {})
			screen := ebiten.NewImage(size.X, size.Y)
			defer screen.Deallocate()
			dialog.Draw(screen)
			footer := dialog.shell.footerOutput
			for _, hint := range []string{"Inspect", "Select", "Cancel"} {
				require.Contains(t, footer.GetOutputStr(false), hint)
			}
			content := insetCharacterRosterRect(dialog.shell.layout.footerRect, dialog.shell.layout.padding)
			require.LessOrEqual(t, footer.GetTotalHeightForLines(footer.GetPanelContentLineCount()), float64(content.Dy()))
		})
	}
}
