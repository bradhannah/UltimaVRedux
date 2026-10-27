package text

import (
	"image"
	"math"
	"testing"
)

func TestIntroMenuChoicePositionsFitPanel(t *testing.T) {
	const (
		screenHeight = 720
		fontHeight   = 22.0
	)
	choiceCount := len(IntroChoices)

	positions := introMenuChoicePositions(screenHeight, fontHeight, choiceCount)
	if len(positions) != choiceCount {
		t.Fatalf("position count = %d, want %d", len(positions), choiceCount)
	}
	menuTop := screenHeight * introMenuTextStartY
	menuBottom := screenHeight * introMenuTextEndY
	if positions[0] < menuTop {
		t.Fatalf("first choice top = %f, want at least %f", positions[0], menuTop)
	}
	lastBottom := positions[len(positions)-1] + fontHeight
	if lastBottom > menuBottom+1e-9 {
		t.Fatalf("last choice bottom = %f, want at most %f", lastBottom, menuBottom)
	}
	for index := 1; index < len(positions); index++ {
		if positions[index] <= positions[index-1] {
			t.Fatalf("choice %d top = %f, want greater than previous top %f", index, positions[index], positions[index-1])
		}
	}
	regularStep := positions[1] - positions[0]
	for index := 1; index < len(positions); index++ {
		step := positions[index] - positions[index-1]
		if math.Abs(step-regularStep) > 1e-9 {
			t.Fatalf("choice %d step = %f, want uniform step %f", index, step, regularStep)
		}
	}
	safeTop := screenHeight * (introMenuFrameStartY + introMenuFrameSafeInsetY)
	safeBottom := screenHeight * (introMenuFrameEndY - introMenuFrameSafeInsetY)
	if positions[0] < safeTop-1e-9 {
		t.Fatalf("first row top = %f, want at least safe frame top %f", positions[0], safeTop)
	}
	if lastBottom > safeBottom+1e-9 {
		t.Fatalf("last row bottom = %f, want at most safe frame bottom %f", lastBottom, safeBottom)
	}
}

func TestIntroMenuTreeKeepsSevenTopLevelRows(t *testing.T) {
	const topLevelRows = 7
	if len(IntroChoices) != topLevelRows {
		t.Fatalf("top-level row count = %d, want %d", len(IntroChoices), topLevelRows)
	}
	wantChildren := map[string][]string{
		"New Game":  {"Create New Character", "Import from Legacy"},
		"Configure": {"Display", "Audio", "Set Data Directory"},
	}
	for _, choice := range IntroChoices {
		want, isParent := wantChildren[choice.Label]
		if !isParent {
			if len(choice.Children) != 0 {
				t.Fatalf("%s has children %v, want none", choice.Label, choice.Children)
			}

			continue
		}
		if len(choice.Children) != len(want) {
			t.Fatalf("%s children = %v, want %v", choice.Label, choice.Children, want)
		}
		for index, label := range want {
			if choice.Children[index] != label {
				t.Fatalf("%s child %d = %q, want %q", choice.Label, index, choice.Children[index], label)
			}
		}
		delete(wantChildren, choice.Label)
	}
	if len(wantChildren) != 0 {
		t.Fatalf("missing parent rows: %v", wantChildren)
	}
}

func TestIntroMenuSevenRowsUseTheirOwnRowHeight(t *testing.T) {
	sevenRowHeight := introMenuFontHeightPercent(len(IntroChoices))
	want := introFontReferenceHeight * introTextCoverage / float64(len(IntroChoices))
	if math.Abs(sevenRowHeight-want) > 1e-12 {
		t.Fatalf("seven-row font height = %f, want %f", sevenRowHeight, want)
	}
	if sevenRowHeight <= introMenuFontHeightPercent(introMenuMaxChoices) {
		t.Fatalf("seven rows must draw larger than the nine-row layout, got %f", sevenRowHeight)
	}
	if sevenRowHeight > introBaseFontHeight {
		t.Fatalf("seven-row font height = %f exceeds base height %f", sevenRowHeight, introBaseFontHeight)
	}
}

func TestIntroMenuRejectsMoreThanNineChoices(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected layout-capacity panic")
		}
	}()

	introMenuFontHeightPercent(introMenuMaxChoices + 1)
}

func TestIntroMarkerBandsMirrorSymmetrically(t *testing.T) {
	origin := image.Pt(10, 20)
	bands := introMarkerBands(introMarkerRows[:], origin, 2, false)
	if len(bands) != introMarkerHeightUnits {
		t.Fatalf("band count = %d, want %d", len(bands), introMarkerHeightUnits)
	}
	if bands[0] != image.Rect(10, 20, 14, 22) {
		t.Fatalf("top band = %v, want a two-unit tip at the origin", bands[0])
	}
	if bands[3] != image.Rect(10, 26, 22, 28) {
		t.Fatalf("middle band = %v, want the full six-unit base", bands[3])
	}
	mirrored := introMarkerBands(introMarkerRows[:], origin, 2, true)
	if mirrored[0] != image.Rect(18, 20, 22, 22) {
		t.Fatalf("mirrored top band = %v, want the tip hugging the right edge", mirrored[0])
	}
	if mirrored[3] != bands[3] {
		t.Fatalf("mirrored middle band = %v, want %v", mirrored[3], bands[3])
	}
	chevron := introMarkerBands(introChevronRows[:], origin, 1, false)
	if chevron[3] != image.Rect(10, 23, 14, 24) {
		t.Fatalf("chevron base = %v, want four units wide", chevron[3])
	}
}

func TestIntroSubmenuChildrenFollowTheOpenParent(t *testing.T) {
	closed := IntroMenuState{Selection: 3, Enabled: nil, SubmenuOpen: false, SubmenuSelection: 0}
	if children := introSubmenuChildren(closed); children != nil {
		t.Fatalf("closed menu children = %v, want none", children)
	}
	configure := IntroMenuState{Selection: 3, Enabled: nil, SubmenuOpen: true, SubmenuSelection: 1}
	if children := introSubmenuChildren(configure); len(children) != 3 || children[1] != "Audio" {
		t.Fatalf("Configure children = %v, want Display, Audio, Set Data Directory", children)
	}
	leaf := IntroMenuState{Selection: 0, Enabled: nil, SubmenuOpen: true, SubmenuSelection: 0}
	if children := introSubmenuChildren(leaf); len(children) != 0 {
		t.Fatalf("Journey Onward children = %v, want none", children)
	}
}
