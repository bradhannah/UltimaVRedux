package widgets

import (
	"image"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

func TestCharacterSelectionDialogResizePreservesSelection(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog := NewCharacterSelectionDialog(CharacterSelectionConfig{
		Title: "Select character", Options: characterSelectionTestOptions(),
		OnSelect: func(int) {}, OnCancel: func() {}, Keyboard: newWidgetTestKeyboard(0),
		SpriteSheet: &sprites.SpriteSheet{}, Placement: *createMockPlacement(),
	})
	dialog.HandleKeyDown()
	selected := dialog.GetCurrentSelection()
	oldFont := dialog.shell.bodyFont
	dialog.shell.ensureLayout(1280, 720, len(dialog.GetOptions()), dialog.footer())
	oldPanelBounds := dialog.shell.layout.panelRect

	setResponsiveDialogTestResolution(t, 1920, 1080)
	dialog.shell.ensureLayout(1920, 1080, len(dialog.GetOptions()), dialog.footer())

	assert.Equal(t, selected, dialog.GetCurrentSelection())
	assert.NotSame(t, oldFont, dialog.shell.bodyFont)
	assert.NotEqual(t, oldPanelBounds, dialog.shell.layout.panelRect)
	assert.Equal(t, 1920, dialog.shell.lastScreenWidth)
	assert.Equal(t, 1080, dialog.shell.lastScreenHeight)
}

func TestNewOrderDialogResizePreservesSecondSelectionStage(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog := createNewOrderTestDialog(createMockPartyState(), 1, func(int, int) {}, func() {})
	dialog.HandleEnter()
	require.Equal(t, Stage2SelectSecond, dialog.GetStage())
	firstSelection := dialog.GetFirstSelectedIndex()
	currentSelection := dialog.GetCurrentSelectionIndex()
	currentTitle := dialog.GetTitle()
	require.Equal(t, CharacterSelectionLocked, newOrderOptionByIndex(t, dialog.GetOptions(), firstSelection).State)

	setResponsiveDialogTestResolution(t, 1920, 1080)
	dialog.shell.ensureLayout(1920, 1080, len(dialog.GetOptions()), dialog.footer())

	assert.Equal(t, Stage2SelectSecond, dialog.GetStage())
	assert.Equal(t, firstSelection, dialog.GetFirstSelectedIndex())
	assert.Equal(t, currentSelection, dialog.GetCurrentSelectionIndex())
	assert.Equal(t, currentTitle, dialog.GetTitle())
	assert.Equal(t, CharacterSelectionLocked, newOrderOptionByIndex(t, dialog.GetOptions(), firstSelection).State)
}

func TestCharacterRosterDialogUsesLogicalScreenBoundsAtSupportedResolutions(t *testing.T) {
	resolutions := []image.Point{
		{X: 1280, Y: 720},
		{X: 1366, Y: 768},
		{X: 1920, Y: 1080},
		{X: 2560, Y: 1440},
		{X: 3840, Y: 2160},
	}
	placement := *createMockPlacement()
	dialog := newCharacterSelectionTestDialog(characterSelectionTestOptions(), func(int) {})
	for _, resolution := range resolutions {
		dialog.shell.ensureLayout(resolution.X, resolution.Y, len(dialog.GetOptions()), dialog.footer())
		workspace := image.Rect(
			int(float64(resolution.X)*placement.StartPercentX),
			int(float64(resolution.Y)*placement.StartPercentY),
			int(float64(resolution.X)*placement.EndPercentX),
			int(float64(resolution.Y)*placement.EndPercentY),
		)

		assert.True(t, dialog.shell.layout.panelRect.In(workspace))
		assert.Equal(t, resolution.X, dialog.shell.lastScreenWidth)
		assert.Equal(t, resolution.Y, dialog.shell.lastScreenHeight)
	}
}

func TestCharacterRosterDialogUsesCompactTopAlignedLedgerGeometry(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog := newCharacterSelectionTestDialog(characterSelectionTestOptions(), func(int) {})
	dialog.shell.ensureLayout(1280, 720, len(dialog.GetOptions()), dialog.footer())
	layout := dialog.shell.layout
	workspace := image.Rect(0, 0, int(1280*createMockPlacement().EndPercentX), 720)

	assert.LessOrEqual(t, layout.panelRect.Dx(), workspace.Dx()*72/100)
	assert.LessOrEqual(t, layout.panelRect.Dy(), workspace.Dy()*60/100)
	assert.Equal(t, layout.rosterBodyRect.Min.Y, layout.rowRects[0].Min.Y)
	assert.Less(t, layout.rowRects[len(layout.rowRects)-1].Max.Y, layout.rosterBodyRect.Max.Y)
	assert.Less(t, layout.rosterRect.Dx(), layout.panelRect.Dx()/2)
	assert.Equal(t, workspace.Dx()/characterRosterDialogMapColumns, layout.portraitRect.Dx())
	assert.Equal(t, workspace.Dy()/characterRosterDialogMapRows, layout.portraitRect.Dy())
	assert.Equal(
		t,
		layout.padding*characterRosterDialogCenterDivisor,
		layout.detailTextRect.Min.X-layout.portraitRect.Max.X,
	)
	for index, option := range characterSelectionTestOptions() {
		nameRect, statusRect := dialog.shell.rosterRowTextRects(layout.rowRects[index], option.Status)
		requiredNameWidth, _ := dialog.shell.bodyFont.MeasureString(">X * 1 " + option.Name)
		assert.GreaterOrEqual(t, nameRect.Dx(), int(math.Ceil(requiredNameWidth)))
		assert.Less(t, nameRect.Max.X, statusRect.Min.X)
	}
	fontHeight := int(math.Ceil(dialog.shell.rowOutput.GetFontHeight()))
	assert.LessOrEqual(t, layout.rowRects[0].Dy()-fontHeight, fontHeight/2+layout.pixelUnit)
	assert.Equal(t, GetScaledFont(FontSizeCompact).TextFace.Size, dialog.shell.bodyFont.TextFace.Size)
	assert.Equal(t, GetScaledFont(FontSizeBody).TextFace.Size, dialog.shell.footerOutput.Font.TextFace.Size)
}

func TestSpellCastingDialogResizePreservesWorkflowState(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	dialog := NewSpellCastingDialog(
		newWidgetTestKeyboard(0),
		&state.PartyState,
		references.ContextTown,
		state.CurrentSpellCatalog(), state.ValidateSpellTargetingPrerequisites,
	)
	dialog.browser.syllables = "V"
	dialog.browser.filter()
	dialog.browser.selected = 1
	dialog.selectedCasterIdx = 2
	dialog.notice = "Not here!"
	selectedSpellIndex := dialog.browser.selected
	oldOutput := dialog.view.body

	setResponsiveDialogTestResolution(t, 1920, 1080)
	dialog.ensureLayout()

	assert.Equal(t, "V", dialog.browser.syllables)
	assert.Equal(t, selectedSpellIndex, dialog.browser.selected)
	assert.Equal(t, 2, dialog.selectedCasterIdx)
	assert.Equal(t, "Not here!", dialog.notice)
	assert.NotSame(t, oldOutput, dialog.view.body)
}

func TestReagentMixingDialogResizePreservesWorkflowState(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	reagents := []ReagentInfo{
		{Reagent: references.SulfurAsh, Name: "Sulfur Ash", Quantity: 5, SpriteIndex: 0},
		{Reagent: references.Ginseng, Name: "Ginseng", Quantity: 4, SpriteIndex: 0},
		{Reagent: references.Garlic, Name: "Garlic", Quantity: 3, SpriteIndex: 0},
	}
	dialog := NewReagentMixingDialog(
		newWidgetTestKeyboard(0),
		testMixCatalog(),
		reagents,
	)
	dialog.ChooseSpell(references.VasFlam, false)
	dialog.tab, dialog.selection = mixTabExperiment, 1<<references.SulfurAsh
	dialog.quantity, dialog.cursor = 3, 2
	dialog.awaitingConfirmation = true
	dialog.ensureLayout()
	oldOutput, oldWindow := dialog.view.body, dialog.window.body

	setResponsiveDialogTestResolution(t, 1920, 1080)
	dialog.ensureLayout()

	assert.Equal(t, references.VasFlam.ID(), dialog.spell.ID)
	assert.Equal(t, uint16(1<<references.SulfurAsh), dialog.selection)
	assert.Equal(t, mixTabExperiment, dialog.tab)
	assert.Equal(t, 3, dialog.amount())
	assert.Equal(t, 2, dialog.cursor)
	assert.True(t, dialog.awaitingConfirmation)
	assert.NotSame(t, oldOutput, dialog.view.body)
	assert.NotEqual(t, oldWindow, dialog.window.body, "the quantity window is fitted again for the new size")
}

func TestReadyEquipmentDialogResizePreservesWorkflowState(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Equipment.Set(references.Dagger, 1)
	state.PartyState.Inventory.Equipment.Set(references.LongSword, 1)
	state.PartyState.Inventory.Equipment.Set(references.LeatherArmour, 1)
	dialog := NewReadyEquipmentDialog(
		newWidgetTestKeyboard(0),
		&sprites.SpriteSheet{},
		state,
		game_state.ReadyEquipmentRequest{},
	)
	dialog.itemListNav.SetIndex(2)
	dialog.warningMessage = "Keep this warning"
	selectedItemIndex := dialog.itemListNav.GetCurrentIndex()
	oldOutput := dialog.detailOutput

	setResponsiveDialogTestResolution(t, 1920, 1080)
	dialog.ensureLayout()

	assert.Equal(t, selectedItemIndex, dialog.itemListNav.GetCurrentIndex())
	assert.Equal(t, "Keep this warning", dialog.warningMessage)
	assert.Equal(t, 0, dialog.selectedCharIdx)
	assert.NotSame(t, oldOutput, dialog.detailOutput)
}

func setResponsiveDialogTestResolution(t *testing.T, width, height int) {
	t.Helper()
	manager := display.GetManager()
	originalWidth, originalHeight := manager.GetCurrentSize()
	t.Cleanup(func() { manager.PublishLogicalSize(originalWidth, originalHeight) })
	manager.PublishLogicalSize(width, height)
}
