package mainscreen

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCharacterRightTextBoundsLeaveScaledBorderInset(t *testing.T) {
	bounds := image.Rect(985, 18, 1267, 346)
	rightText := characterRightTextBounds(bounds)

	assert.Equal(t, 11, bounds.Max.X-rightText.Max.X)
	assert.Equal(t, bounds.Dx()*rightTextWidthPercent/percentDivisor, rightText.Dx())
}

func TestCharacterTextRetainsUnchangedRowsAndRefreshesVisibleValues(t *testing.T) {
	party := summaryBaselineParty()
	summary := &CharacterSummary{}
	summary.ultimaFont, summary.output = newMainScreenOutput(1280, widgets.FontSizeBody, characterOutputMaxLines)
	screen := ebiten.NewImage(1, 1)
	t.Cleanup(screen.Dispose)
	summary.Draw(party, screen, -1, false, -1)
	character := party.GetCharacter(0)
	row := summary.prepareCharacterText(0, character, color.White)
	builds, left, right := row.builds, row.left, row.right
	other := summary.prepareCharacterText(1, party.GetCharacter(1), color.White)
	otherBuilds := other.builds
	for range 60 {
		summary.prepareCharacterText(0, character, color.White)
	}
	require.Equal(t, builds, row.builds)
	require.Same(t, left, row.left)
	require.Same(t, right, row.right)
	for _, test := range []struct {
		name                string
		change              func()
		wantColor           color.Color
		wantLeft, wantRight string
	}{
		{"HP", func() { character.SetHp(50) }, color.White, "50/100HP", "Good"},
		{"max HP", func() { character.SetMaxHp(200) }, color.White, "50/200HP", "Good"},
		{"MP", func() { character.SetMp(17) }, color.White, "50/200HP", "17MP"},
		{"name", func() { copy(character.Name[:], "New") }, color.White, "New", "17MP"},
		{"sleep", func() { character.SetStatus(party_state.Sleep) }, statusColors[party_state.Sleep], "New", "Sleep"},
		{"poison", func() { character.SetStatus(party_state.Poisoned) }, statusColors[party_state.Poisoned], "New", "Pois."},
		{"good again", func() { character.SetStatus(party_state.Good) }, color.White, "New", "Good"},
		{"selection dimming", func() {}, ucolor.From(color.White).Intensity(inactivePlayerIntensity).RGBA(), "New", "Good"},
		{"selection reset", func() {}, color.White, "New", "Good"},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := row.builds
			test.change()
			// No explicit refresh callback: Draw must reflect status transitions.
			summary.Draw(party, screen, -1, false, -1)
			summary.prepareCharacterText(0, character, test.wantColor)
			require.Equal(t, before+1, row.builds)
			require.Contains(t, row.left.GetOutputStr(false), test.wantLeft)
			require.Contains(t, row.right.GetOutputStr(false), test.wantRight)
			_, hasColor := row.left.FirstPanelLineWithColor(0, 2, test.wantColor)
			require.True(t, hasColor, "the rebuilt row must use the new colour")
			require.Same(t, left, row.left)
			require.Same(t, right, row.right)
			summary.prepareCharacterText(0, character, test.wantColor)
			require.Equal(t, before+1, row.builds, "an unchanged follow-up must retain the row")
			require.Equal(t, otherBuilds, other.builds, "changes must not rebuild unrelated party rows")
		})
	}
	// Typography changes must replace row outputs rather than reuse stale faces.
	for _, change := range []func(){
		func() { summary.output.Font.TextFace.Size += 0.5 },
		func() {
			summary.output = text.NewOutput(summary.ultimaFont, summary.output.GetLineSpacing()+1, characterOutputMaxLines, 80)
		},
		func() {
			summary.ultimaFont, summary.output = newMainScreenOutput(1920, widgets.FontSizeBody, characterOutputMaxLines)
		},
	} {
		before, oldLeft, oldRight := row.builds, row.left, row.right
		change()
		summary.prepareCharacterText(0, character, color.White)
		require.Equal(t, before+1, row.builds)
		require.NotSame(t, oldLeft, row.left)
		require.NotSame(t, oldRight, row.right)
		require.Equal(t, otherBuilds, other.builds)
	}
}

func TestCharacterSummaryRenderingMatchesOriginalPortraitAndTextBaseline(t *testing.T) {
	// Positions captured from b19e37af's ordinary six-row layout, with text one
	// pixel higher for the 23-size body typography. These do not call the
	// production layout or typography helpers to compute expectations.
	for _, fixture := range []struct {
		width, rowPitch, portraitX, portraitY, portraitSize int
		textX, textY, leftWidth, rightX, rightWidth         int
	}{
		{960, 41, 739, 15, 37, 780, 19, 85, 869, 73},
		{1280, 54, 985, 20, 50, 1039, 25, 115, 1158, 98},
		{1920, 81, 1478, 29, 77, 1559, 38, 174, 1737, 147},
	} {
		t.Run(fmt.Sprint(fixture.width), func(t *testing.T) {
			party := summaryBaselineParty()
			sheet := sprites.NewSpriteSheet()
			summary := NewCharacterSummaryForWidth(sheet, fixture.width)
			actual := ebiten.NewImage(fixture.width, fixture.width*9/16)
			expected := ebiten.NewImage(fixture.width, fixture.width*9/16)
			t.Cleanup(actual.Dispose)
			t.Cleanup(expected.Dispose)
			size := 23 * float64(fixture.width) / 1920
			font := text.NewUltimaFont(size)
			reference := text.NewOutput(font, size*1.59, 10, 16)
			fontBefore := *summary.ultimaFont.TextFace
			metricsBefore := summary.ultimaFont.TextFace.Metrics()
			glyphWidth, glyphHeight := summary.ultimaFont.MeasureString("Pois. 75/100HP 30MP")
			_, otherHUD := newMainScreenOutput(fixture.width, widgets.FontSizeHUD, 10)
			require.NotSame(t, summary.ultimaFont, otherHUD.Font)
			otherHUD.SetMaxCharsPerLine(80)
			otherTarget := ebiten.NewImage(fixture.width, fixture.width*9/16)
			t.Cleanup(otherTarget.Dispose)

			for active := range 6 {
				expected.Clear()
				var portraits []image.Rectangle
				for index, label := range []string{"Good", "Pois.", "Sleep", "Charmed", "Dead", "Good"} {
					character := party.GetCharacter(index)
					statusColor := []color.Color{nil, ucolor.EGAGreen, ucolor.EGAMagenta, ucolor.EGABlue, ucolor.EGARed, nil}[index]
					portrait := sheet.GetSprite(character.GetKeySpriteIndex())
					if statusColor != nil {
						portrait = sheet.GetTileEffectProcessor().ApplyDynamicOutlineRGBA(portrait, statusColor, 0.7)
					}
					options := &ebiten.DrawImageOptions{}
					options.GeoM.Scale(float64(fixture.portraitSize)/float64(portrait.Bounds().Dx()),
						float64(fixture.portraitSize)/float64(portrait.Bounds().Dy()))
					options.GeoM.Translate(float64(fixture.portraitX), float64(fixture.portraitY+index*fixture.rowPitch))
					expected.DrawImage(portrait, options)
					portraits = append(portraits, image.Rect(fixture.portraitX, fixture.portraitY+index*fixture.rowPitch,
						fixture.portraitX+fixture.portraitSize, fixture.portraitY+index*fixture.rowPitch+fixture.portraitSize))
					textColor := statusColor
					if textColor == nil {
						textColor = ucolor.White
					}
					if index != active {
						textColor = ucolor.From(textColor).Intensity(0.6).RGBA()
					}
					reference.Reset()
					reference.AppendLine().Text(character.GetNameAsString(), textColor).Done()
					reference.AppendLine().Text("75/100HP", textColor).Done()
					reference.DrawPanelContent(expected, image.Pt(fixture.textX, fixture.textY+index*fixture.rowPitch), fixture.leftWidth, etext.AlignStart)
					reference.Reset()
					reference.AppendLine().Text(label, textColor).Done()
					reference.AppendLine().Text("30MP", textColor).Done()
					reference.DrawPanelContent(expected, image.Pt(fixture.rightX, fixture.textY+index*fixture.rowPitch), fixture.rightWidth, etext.AlignEnd)
				}
				for _, conditions := range []CombatConditions{{}, {Possessed: true, Underground: true, Sleeping: true}, {}} {
					otherHUD.Reset()
					AppendCombatConditions(otherHUD.AppendLine(), conditions).Done()
					otherHUD.DrawPanelContent(otherTarget, image.Pt(0, 0), fixture.width, etext.AlignStart)
					actual.Clear()
					summary.Draw(party, actual, active, false, -1)
					requireSummaryPixels(t, expected, actual, portraits)
					assert.Equal(t, fontBefore, *summary.ultimaFont.TextFace)
					assert.Equal(t, metricsBefore, summary.ultimaFont.TextFace.Metrics())
					width, height := summary.ultimaFont.MeasureString("Pois. 75/100HP 30MP")
					assert.Equal(t, glyphWidth, width)
					assert.Equal(t, glyphHeight, height)
					assert.InDelta(t, size*1.59, summary.output.GetLineSpacing(), 1e-12)
				}
			}
		})
	}
}

func summaryBaselineParty() *party_state.PartyState {
	party := party_state.NewBlankPartyState()
	for index, status := range []party_state.CharacterStatus{
		party_state.Good, party_state.Poisoned, party_state.Sleep, party_state.Charmed, party_state.Dead, party_state.Good,
	} {
		character := party.GetCharacter(index)
		copy(character.Name[:], fmt.Sprintf("PC%d", index+1))
		character.Class = party_state.Avatar
		character.Intelligence = 30
		character.SetMaxHp(100)
		character.SetHp(75)
		character.SetMp(30)
		character.SetStatus(status)
		character.PartyMap = party_state.PartyMapActive
	}

	return party
}

func summaryPixels(screen *ebiten.Image, bounds image.Rectangle) []byte {
	pixels := make([]byte, bounds.Dx()*bounds.Dy()*4)
	screen.SubImage(bounds).(*ebiten.Image).ReadPixels(pixels)

	return pixels
}

func requireSummaryPixels(t *testing.T, expected, actual *ebiten.Image, portraits []image.Rectangle) {
	t.Helper()
	want, got := summaryPixels(expected, expected.Bounds()), summaryPixels(actual, actual.Bounds())
	for offset := 0; offset < len(want); offset += 4 {
		if bytes.Equal(want[offset:offset+4], got[offset:offset+4]) {
			continue
		}
		point := image.Pt((offset/4)%actual.Bounds().Dx(), (offset/4)/actual.Bounds().Dx())
		// Independently allocated outline textures can round either way exactly
		// on a nearest-neighbour source-texel boundary. Ignore only those ties;
		// all other portrait pixels, all text, and all surrounding space are exact.
		tie := false
		for _, portrait := range portraits {
			if !point.In(portrait) {
				continue
			}
			x := (float64(point.X-portrait.Min.X) + 0.5) * 18 / float64(portrait.Dx())
			y := (float64(point.Y-portrait.Min.Y) + 0.5) * 18 / float64(portrait.Dy())
			tie = math.Abs(x-math.Round(x)) < 1e-6 || math.Abs(y-math.Round(y)) < 1e-6
		}
		require.True(t, tie, "baseline pixel mismatch at %v: expected %v, actual %v", point, want[offset:offset+4], got[offset:offset+4])
	}
}
