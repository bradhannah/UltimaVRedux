package widgets

import (
	"image"
	stdcolor "image/color"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestZStatsDossierDialogMandatoryConfigPanics(t *testing.T) {
	valid := testZStatsDossierConfig(newWidgetTestKeyboard(0))
	tests := []struct {
		name   string
		mutate func(*ZStatsDossierDialogConfig)
		panic  string
	}{
		{
			name:   "keyboard",
			mutate: func(config *ZStatsDossierDialogConfig) { config.Keyboard = nil },
			panic:  "ZStatsDossierDialog: keyboard is required",
		},
		{
			name:   "sprite sheet",
			mutate: func(config *ZStatsDossierDialogConfig) { config.SpriteSheet = nil },
			panic:  "ZStatsDossierDialog: SpriteSheet is required",
		},
		{
			name:   "dismiss callback",
			mutate: func(config *ZStatsDossierDialogConfig) { config.OnDismiss = nil },
			panic:  "ZStatsDossierDialog: OnDismiss is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			require.PanicsWithValue(t, test.panic, func() {
				NewZStatsDossierDialog(testZStatsDossier(), config)
			})
		})
	}
}

func TestZStatsDossierDialogNumberKeysSelectDisplayedMembersRegardlessOfStatus(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dossier := testZStatsDossier()
	dossier.Characters[1].Status = "Dead"
	dossier.Characters[2].Status = "Poisoned"
	dialog := newTestZStatsDossierDialog(dossier, keyboard)

	pressZStatsDossierKey(dialog, keyboard, ebiten.Key2)
	require.Equal(t, 1, dialog.GetSelectedCharacterPosition())
	pressZStatsDossierKey(dialog, keyboard, ebiten.Key3)
	require.Equal(t, 2, dialog.GetSelectedCharacterPosition())
	pressZStatsDossierKey(dialog, keyboard, ebiten.Key6)
	require.Equal(t, 5, dialog.GetSelectedCharacterPosition())
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyRight)
	require.Zero(t, dialog.GetSelectedCharacterPosition())
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyLeft)
	require.Equal(t, 5, dialog.GetSelectedCharacterPosition())

	dialog.switchTab(ZStatsDossierArmoryTab)
	pressZStatsDossierKey(dialog, keyboard, ebiten.Key2)
	require.Equal(t, 5, dialog.GetSelectedCharacterPosition(), "characters are selectable only on Character")
}

func TestZStatsDossierDialogNavigationClampsAtListBounds(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestZStatsDossierDialog(testZStatsDossier(), keyboard)
	dialog.switchTab(ZStatsDossierArmoryTab)

	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyUp)
	require.Zero(t, dialog.GetSelectedListIndex())
	for range len(dialog.dossier.Armory) + 3 {
		pressZStatsDossierKey(dialog, keyboard, ebiten.KeyDown)
	}
	require.Equal(t, len(dialog.dossier.Armory)-1, dialog.GetSelectedListIndex())
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyPageDown)
	require.Equal(t, len(dialog.dossier.Armory)-1, dialog.GetSelectedListIndex())

	for range len(dialog.dossier.Armory) + 3 {
		pressZStatsDossierKey(dialog, keyboard, ebiten.KeyPageUp)
	}
	require.Zero(t, dialog.GetSelectedListIndex())
}

func TestZStatsDossierDialogUnreadSpellSelectionIsPresentationOnly(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dossier := testZStatsDossier()
	dossier.UnreadHiddenSpellCount = 1
	dossier.Magic.Spells = []game_state.ZStatsSpell{
		{ID: 1, Name: "In Lor"},
		{ID: 42, Name: "Vas Wis", Hidden: true, Unread: true},
	}
	dialog := NewZStatsDossierDialog(dossier, testZStatsDossierConfig(keyboard))

	for range 3 {
		pressZStatsDossierKey(dialog, keyboard, ebiten.KeyTab)
	}
	require.Equal(t, "Spells", dialog.tabTitle(ZStatsDossierSpellsTab))

	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyDown)
	require.True(t, dialog.dossier.Magic.Spells[1].Unread)
	require.Equal(t, 1, dialog.dossier.UnreadHiddenSpellCount)
	require.Equal(t, "Spells", dialog.tabTitle(ZStatsDossierSpellsTab))

	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyUp)
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyDown)
	require.True(t, dialog.dossier.Magic.Spells[1].Unread)
}

func TestZStatsDossierDialogPromotesMagicCategoriesToTopTabs(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestZStatsDossierDialog(testZStatsDossier(), keyboard)
	dialog.switchTab(ZStatsDossierSpellsTab)
	require.Equal(t, "Spells", dialog.tabTitle(dialog.GetActiveTab()))

	for _, expected := range []ZStatsDossierTab{
		ZStatsDossierReagentsTab,
		ZStatsDossierPotionsTab,
		ZStatsDossierScrollsTab,
		ZStatsDossierItemsTab,
	} {
		pressZStatsDossierKey(dialog, keyboard, ebiten.KeyTab)
		require.Equal(t, expected, dialog.GetActiveTab())
	}
}

func TestZStatsDossierDialogFiltersVisibleSpellsWithCastSyllableInput(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dossier := testZStatsDossier()
	dossier.Magic.Spells = []game_state.ZStatsSpell{
		{ID: references.InLor.ID(), Syllables: "In Lor", Name: "In Lor", Circle: 1},
		{ID: references.InWis.ID(), Syllables: "In Wis", Name: "In Wis", Circle: 2},
		{ID: references.AnZu.ID(), Syllables: "An Zu", Name: "An Zu", Circle: 1},
		{ID: references.InXenMani.ID(), Syllables: "In Xen Mani", Name: "In Xen Mani", Circle: 2, Hidden: true},
	}
	dialog := newTestZStatsDossierDialog(dossier, keyboard)
	dialog.switchTab(ZStatsDossierSpellsTab)

	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyI)
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyL)
	require.Equal(t, "IL", dialog.spellBrowser.syllables)
	require.Len(t, dialog.spellBrowser.filtered, 1)
	require.Equal(t, "In Lor", dialog.selectedSpell().Name)

	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyBackspace)
	require.Equal(t, "I", dialog.spellBrowser.syllables)
	require.Len(t, dialog.spellBrowser.filtered, 3)

	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyBackspace)
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyA)
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyZ)
	require.Equal(t, "AZ", dialog.spellBrowser.syllables)
	require.False(t, dialog.IsDismissed())
	require.Equal(t, "An Zu", dialog.selectedSpell().Name)

	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyBackspace)
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyBackspace)
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyQ)
	require.Empty(t, dialog.spellBrowser.filtered)
	require.Equal(t, -1, dialog.GetSelectedListIndex())
}

func TestZStatsDossierDialogDismissalIsIdempotent(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dismissCalls := 0
	config := testZStatsDossierConfig(keyboard)
	config.OnDismiss = func() { dismissCalls++ }
	dialog := NewZStatsDossierDialog(testZStatsDossier(), config)

	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyZ)
	require.False(t, dialog.IsDismissed())
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyEscape)
	require.True(t, dialog.IsDismissed())
	dialog.Dismiss()
	require.Equal(t, 1, dismissCalls)
}

func TestZStatsDossierDialogDeepClonesAllNestedModelSlices(t *testing.T) {
	dossier := testZStatsDossier()
	dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))

	dossier.Characters[0].Name = "Changed"
	dossier.Characters[0].Equipped[0].Name = "Changed"
	dossier.Characters[0].Equipped[0].Abilities[0] = "Changed"
	dossier.Provisions[0].Name = "Changed"
	dossier.Armory[0].Name = "Changed"
	dossier.Armory[0].Abilities[0] = "Changed"
	dossier.Magic.Spells[0].Name = "Changed"
	dossier.Magic.Spells[0].Reagents[0] = "Changed"
	dossier.Magic.Spells[0].ReagentCandidates[0][0] = "Changed"
	dossier.Magic.Reagents[0].Name = "Changed"
	dossier.Magic.Potions[0].Name = "Changed"
	dossier.Magic.Scrolls[0].Name = "Changed"
	dossier.Items[0].Name = "Changed"

	require.Equal(t, "Avatar", dialog.dossier.Characters[0].Name)
	require.Equal(t, "Leather Helm", dialog.dossier.Characters[0].Equipped[0].Name)
	require.Equal(t, "Protection", dialog.dossier.Characters[0].Equipped[0].Abilities[0])
	require.Equal(t, "Food", dialog.dossier.Provisions[0].Name)
	require.Equal(t, "Long Sword", dialog.dossier.Armory[0].Name)
	require.Equal(t, "Slashing", dialog.dossier.Armory[0].Abilities[0])
	require.Equal(t, "In Lor", dialog.dossier.Magic.Spells[0].Name)
	require.Equal(t, "Sulfurous Ash", dialog.dossier.Magic.Spells[0].Reagents[0])
	require.Equal(t, "Ginseng", dialog.dossier.Magic.Spells[0].ReagentCandidates[0][0])
	require.Equal(t, "Garlic", dialog.dossier.Magic.Reagents[0].Name)
	require.Equal(t, "Red Potion", dialog.dossier.Magic.Potions[0].Name)
	require.Equal(t, "In Lor Scroll", dialog.dossier.Magic.Scrolls[0].Name)
	require.Equal(t, "Crown", dialog.dossier.Items[0].Name)

	dialog.dossier.Magic.Spells[0].ReagentCandidates[0][0] = "Widget owned"
	require.Equal(t, "Changed", dossier.Magic.Spells[0].ReagentCandidates[0][0])
}

func TestZStatsDossierDialogCompactDetailsPageEveryVariablePane(t *testing.T) {
	setZStatsTestResolution(t, 320, 200)
	dossier := testZStatsDossier()
	dossier.Characters[0].Name = "Avatar With A Deliberately Long Name"
	dossier.Characters[0].Equipped[0].Abilities = []string{strings.Repeat("Protection and unusual defensive magic ", 12)}
	dossier.Provisions[0].Description = strings.Repeat("Food supports the traveling party. ", 40)
	dossier.Armory[0].Abilities = []string{strings.Repeat("A lengthy authentic equipment ability record ", 20)}
	dossier.Magic.Spells[0].Description = strings.Repeat("The spell description remains fully inspectable. ", 40)
	dossier.Magic.Reagents[0].Description = strings.Repeat("The reagent description remains fully inspectable. ", 40)
	dossier.Magic.Potions[0].Description = strings.Repeat("The potion description remains fully inspectable. ", 40)
	dossier.Magic.Scrolls[0].Description = strings.Repeat("The scroll description remains fully inspectable. ", 40)
	dossier.Items[0].Description = strings.Repeat("The possession description remains fully inspectable. ", 40)
	dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
	prepare := func() {
		dialog.ensureLayout()
		dialog.detailPages = 1
		rect := dialog.layout.globalDetailRect
		switch dialog.activeTab {
		case ZStatsDossierCharacterTab:
			dialog.prepareCharacterStats()
			dialog.prepareCharacterItemDetail()
		case ZStatsDossierProvisionsTab:
			dialog.prepareProvisionDetail(rect)
		case ZStatsDossierArmoryTab:
			dialog.prepareArmoryDetail(rect)
		case ZStatsDossierSpellsTab:
			dialog.prepareSpellDetail(rect)
		case ZStatsDossierReagentsTab, ZStatsDossierPotionsTab, ZStatsDossierScrollsTab:
			dialog.prepareMagicInventoryDetail(rect)
		case ZStatsDossierItemsTab:
			dialog.prepareItemDetail(rect)
		}
		dialog.detailPage = min(dialog.detailPage, dialog.detailPageCount()-1)
		dialog.prepareFooter()
	}

	for tab := ZStatsDossierCharacterTab; tab < zStatsDossierTabCount; tab++ {
		dialog.switchTab(tab)
		require.NotPanics(t, prepare, dialog.tabTitle(tab))
		pageCount := dialog.detailPageCount()
		for page := range pageCount {
			dialog.detailPage = page
			require.NotPanics(t, prepare, dialog.tabTitle(tab))
			require.Equal(t, page, dialog.detailPage, "every computed page remains accessible")
		}
		require.True(t, dialog.layout.bodyRect.In(dialog.layout.panelRect))
		require.LessOrEqual(t, dialog.layout.bodyRect.Max.Y, dialog.layout.footerRect.Max.Y)
	}
}

func TestZStatsDossierDialogEmptyDossierSurvivesAllInputResizeAndClose(t *testing.T) {
	setZStatsTestResolution(t, 320, 200)
	keyboard := newWidgetTestKeyboard(0)
	dismissCalls := 0
	config := testZStatsDossierConfig(keyboard)
	config.OnDismiss = func() { dismissCalls++ }
	dialog := NewZStatsDossierDialog(game_state.ZStatsDossier{}, config)

	for tab := ZStatsDossierCharacterTab; tab < zStatsDossierTabCount; tab++ {
		dialog.switchTab(tab)
		require.NotPanics(t, func() {
			pressZStatsDossierKey(dialog, keyboard, ebiten.KeyDown)
			pressZStatsDossierKey(dialog, keyboard, ebiten.KeyPageDown)
			pressZStatsDossierKey(dialog, keyboard, ebiten.KeySpace)
		})
		require.Equal(t, -1, dialog.GetSelectedListIndex())
	}

	setZStatsTestResolution(t, 1280, 720)
	dialog.ensureLayout()
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyEscape)
	require.True(t, dialog.IsDismissed())
	require.Equal(t, 1, dismissCalls)
}

func TestZStatsDossierDialogResizeRebuildsWideLayoutAndPreservesState(t *testing.T) {
	setZStatsTestResolution(t, 320, 200)

	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestZStatsDossierDialog(testZStatsDossier(), keyboard)
	dialog.switchTab(ZStatsDossierArmoryTab)
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyDown)
	dialog.ensureLayout()
	smallFontSize := dialog.bodyFont.TextFace.Size
	require.True(t, dialog.outerBorder.GetBounds().In(image.Rect(0, 0, 320, 200)))
	requireZStatsWideLayout(t, dialog.layout)

	setZStatsTestResolution(t, 1280, 720)
	dialog.ensureLayout()
	require.Equal(t, ZStatsDossierArmoryTab, dialog.GetActiveTab())
	require.Equal(t, 1, dialog.GetSelectedListIndex())
	require.Greater(t, dialog.bodyFont.TextFace.Size, smallFontSize)
	largeBounds := dialog.outerBorder.GetBounds()
	require.True(t, largeBounds.In(image.Rect(0, 0, 1280, 720)))
	require.Less(t, largeBounds.Min.Y, 72)
	require.Greater(t, largeBounds.Max.Y, 648)
	overlap := dialog.pixelUnit()
	require.Equal(t, dialog.layout.headerRect.Max.Y-overlap, dialog.layout.bodyRect.Min.Y)
	require.Equal(t, dialog.layout.bodyRect.Max.Y-overlap, dialog.layout.footerRect.Min.Y)
	largeScreen := ebiten.NewImageWithOptions(largeBounds, nil)
	t.Cleanup(largeScreen.Dispose)
	for _, rect := range dialog.frameRects() {
		FillClassicPixelRoundedRect(largeScreen, rect, dialog.pixelUnit(), pkgcolor.DialogBackground.RGBA())
	}
	dialog.drawFrames(largeScreen)
	panePerimeterPoint := image.Pt(
		dialog.layout.headerRect.Min.X+dialog.layout.headerRect.Dx()/2,
		dialog.layout.headerRect.Min.Y,
	)
	requireColorRGBA(t, DialogRuleColor, largeScreen.At(panePerimeterPoint.X, panePerimeterPoint.Y))
	paneCorner := dialog.layout.headerRect.Min
	requireColorNotRGBA(t, pkgcolor.DialogBackground.RGBA(), largeScreen.At(paneCorner.X, paneCorner.Y))
	legacyOuterPoint := image.Pt(largeBounds.Min.X+largeBounds.Dx()/2, largeBounds.Min.Y)
	requireColorNotRGBA(t, DialogRuleColor, largeScreen.At(legacyOuterPoint.X, legacyOuterPoint.Y))
	requireZStatsWideLayout(t, dialog.layout)
}

func TestZStatsDossierDialogTopTabsHaveRenderedSeparationAtSupportedResolutions(t *testing.T) {
	firstResolution := config.SupportedWindowResolutions()[0]
	setZStatsTestResolution(t, firstResolution.X, firstResolution.Y)
	dossier := testZStatsDossier()
	dossier.UnreadHiddenSpellCount = references.TotalSpellCount
	dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))

	for _, resolution := range config.SupportedWindowResolutions() {
		setZStatsTestResolution(t, resolution.X, resolution.Y)
		dialog.ensureLayout()
		require.Equal(t, dialog.bodyFont.TextFace.Size, dialog.tabOutput.Font.TextFace.Size)

		var previousRight int
		for tab := ZStatsDossierCharacterTab; tab < zStatsDossierTabCount; tab++ {
			dialog.switchTab(tab)
			rect := dialog.layout.tabRects[tab]
			require.True(t, rect.In(dialog.layout.tabsRect))
			width, _ := dialog.tabOutput.Font.MeasureString(dialog.tabTitle(tab))
			require.LessOrEqual(t, width, float64(rect.Dx()))
			if tab >= zStatsDossierTabColumns {
				above := dialog.layout.tabRects[tab-zStatsDossierTabColumns]
				require.Equal(t, above.Min.X, rect.Min.X)
				require.Equal(t, above.Max.X, rect.Max.X)
				require.GreaterOrEqual(t, rect.Min.Y, above.Max.Y)
			}
			if resolution.X != 1366 && resolution.X != 2560 {
				continue
			}
			screen := ebiten.NewImageWithOptions(rect, nil)
			dialog.drawTabTitle(screen, tab, rect)
			bounds := renderedContentBounds(t, screen)
			screen.Dispose()
			require.Equal(t, dialog.tabTitle(tab), dialog.tabOutput.GetOutputStr(false))
			require.True(
				t,
				bounds.In(rect),
				"%s at %dx%d: rendered %v outside %v",
				dialog.tabTitle(tab),
				resolution.X,
				resolution.Y,
				bounds,
				rect,
			)
			if tab%zStatsDossierTabColumns != 0 {
				require.GreaterOrEqual(
					t,
					bounds.Min.X-previousRight,
					dialog.pixelUnit()*4,
					"%s at %dx%d: previous right %d, left %d, rect %v",
					dialog.tabTitle(tab),
					resolution.X,
					resolution.Y,
					previousRight,
					bounds.Min.X,
					rect,
				)
			}
			previousRight = bounds.Max.X
		}
	}
}

func renderedContentBounds(t *testing.T, screen *ebiten.Image) image.Rectangle {
	t.Helper()
	search := screen.Bounds()
	pixels := make([]byte, search.Dx()*search.Dy()*4)
	screen.ReadPixels(pixels)
	minimumX, minimumY := search.Max.X, search.Max.Y
	maximumX, maximumY := search.Min.X, search.Min.Y
	found := false
	for y := search.Min.Y; y < search.Max.Y; y++ {
		for x := search.Min.X; x < search.Max.X; x++ {
			if pixels[((y-search.Min.Y)*search.Dx()+x-search.Min.X)*4+3] == 0 {
				continue
			}
			found = true
			minimumX = min(minimumX, x)
			minimumY = min(minimumY, y)
			maximumX = max(maximumX, x+1)
			maximumY = max(maximumY, y+1)
		}
	}
	require.True(t, found, "text was not rendered")

	return image.Rect(minimumX, minimumY, maximumX, maximumY)
}

func TestZStatsDossierDialogDetailsUseCatalogPortraitsAndExcludeReadiness(t *testing.T) {
	dialog := newTestZStatsDossierDialog(testZStatsDossier(), newWidgetTestKeyboard(0))
	dialog.prepareCharacterStats()
	characterText := dialog.detailOutput.GetOutputStr(false)
	require.Contains(t, characterText, "Avatar")
	require.Contains(t, characterText, "READY TO ADVANCE")
	require.Contains(t, characterText, "HP:")
	require.Contains(t, characterText, "MP:")
	require.Contains(t, characterText, "STR:")
	require.Contains(t, characterText, "DEX:")
	require.Contains(t, characterText, "INT:")
	require.Contains(t, characterText, "Attack:")
	require.Contains(t, characterText, "Defense:")
	require.NotContains(t, characterText, "Equipment STR:")
	require.NotContains(t, characterText, "STR / DEX / INT")
	require.NotContains(t, characterText, "Gear ATK / DEF")
	require.NotContains(t, characterText, "Highest spell")
	dialog.preparePanelHeader(dialog.layout.characterPortraitRect, dialog.selectedCharacterModel().Name)
	require.Contains(t, dialog.titleOutput.GetOutputStr(false), "Avatar")
	dialog.prepareCharacterItemDetail()
	require.Contains(t, dialog.summaryOutput.GetOutputStr(false), "Leather Helm")
	characterItemBody := dialog.detailOutput.GetOutputStr(false)
	require.Contains(t, characterItemBody, "Strength required")
	require.Contains(t, characterItemBody, "Hands required: One")

	dialog.switchTab(ZStatsDossierArmoryTab)
	dialog.prepareArmoryDetail(dialog.layout.globalDetailRect)
	armorySummary := dialog.summaryOutput.GetOutputStr(false)
	require.Contains(t, armorySummary, "Available / Readied")
	require.NotContains(t, armorySummary, "Selected character")
	armoryBody := dialog.detailOutput.GetOutputStr(false)
	require.Contains(t, armoryBody, "Attack:")
	require.Contains(t, armoryBody, "Defense:")
	require.Contains(t, armoryBody, "Strength required:")
	require.Contains(t, armoryBody, "Range:")
	require.Contains(t, armoryBody, "Hands required: One")

	dialog.switchTab(ZStatsDossierSpellsTab)
	dialog.prepareSpellDetail(dialog.layout.globalDetailRect)
	magicSummary := dialog.summaryOutput.GetOutputStr(false)
	require.Contains(t, magicSummary, "Circle:")
	require.Contains(t, magicSummary, "MP cost:")
	require.Contains(t, magicSummary, "Can mix:")
	magicBody := dialog.detailOutput.GetOutputStr(false)
	require.Contains(t, magicBody, "Untested 1")
	require.Contains(t, magicBody, "Learned from")
	require.NotContains(t, magicBody, "Current context")
	require.NotContains(t, magicBody, "Allowed / Supported")
	require.NotContains(t, magicBody, "Readiness")
}

func TestZStatsDossierDialogMagicUsesRedUnknownAndMeaningfulProvenance(t *testing.T) {
	dossier := testZStatsDossier()
	spell := &dossier.Magic.Spells[0]
	spell.Reagents = nil
	spell.ReagentCandidates = nil
	spell.Hidden = false
	spell.LearningSourceKind = ""
	spell.LearningSource = ""
	spell.LearningDate = ""
	dialog := newRenderedTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
	dialog.switchTab(ZStatsDossierSpellsTab)
	screen := ebiten.NewImage(1280, 720)

	dialog.drawSpellDetail(screen, dialog.layout.globalDetailRect)
	body := dialog.detailOutput.GetOutputStr(false)
	require.Contains(t, body, "Recipe: Unknown")
	require.NotContains(t, body, "Learned from")
	require.NotContains(t, body, "Learned on")
	requireImageRegionContainsColor(t, screen, dialog.layout.globalDetailRect, pkgcolor.EGARed)

	spell = &dialog.dossier.Magic.Spells[0]
	spell.Hidden = true
	spell.LearningSourceKind = string(game_state.KnowledgeSourceMigration)
	spell.LearningSource = "Previously learned; date unknown"
	dialog.drawSpellDetail(screen, dialog.layout.globalDetailRect)
	body = dialog.detailOutput.GetOutputStr(false)
	require.Contains(t, body, "Learned from: Previously learned")
	require.NotContains(t, body, "Learned on")
	require.Equal(t, "Mariah", zStatsLearningSource(string(game_state.KnowledgeSourceDialogue), "Mariah"))
	require.Equal(t, "Experimentation", zStatsLearningSource(
		string(game_state.KnowledgeSourceExperimentation),
		"Experimentation",
	))
}

func TestZStatsDossierDialogSpaceLoopsCapacityBoundDetailPagesAndSelectionResets(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dossier := testZStatsDossier()
	dossier.Items[0].Description = strings.Repeat("The shard hums with ancient and dangerous power. ", 80)
	dossier.Items[1].Description = strings.Repeat("The moonstone answers the phases of the moons. ", 80)
	dialog := newTestZStatsDossierDialog(dossier, keyboard)
	dialog.switchTab(ZStatsDossierItemsTab)

	dialog.prepareItemDetail(dialog.layout.globalDetailRect)
	pages := dialog.detailPageCount()
	require.Greater(t, pages, 1)
	_, _, bodyRect := dialog.catalogDetailRects(dialog.layout.globalDetailRect)
	capacity := max(1, dialog.detailOutput.GetLineCapacity(bodyRect.Dy()))
	require.Greater(t, dialog.detailOutput.GetPanelContentLineCount(), capacity)
	dialog.prepareFooter()
	dialog.prepareFooter()
	footer := dialog.footerOutput.GetOutputStr(false)
	require.LessOrEqual(t, dialog.footerOutput.GetPanelContentLineCount(), zStatsDossierFooterMaxLines)
	require.Contains(t, footer, "Tab")
	require.Contains(t, footer, "[Up/Down]")
	require.Contains(t, footer, "Space")
	require.Contains(t, footer, "[Esc]")

	pressZStatsDossierKey(dialog, keyboard, ebiten.KeySpace)
	require.Equal(t, 1, dialog.detailPage)
	for range pages - 1 {
		pressZStatsDossierKey(dialog, keyboard, ebiten.KeySpace)
	}
	require.Zero(t, dialog.detailPage)

	pressZStatsDossierKey(dialog, keyboard, ebiten.KeySpace)
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeyDown)
	require.Equal(t, 1, dialog.GetSelectedListIndex())
	require.Zero(t, dialog.detailPage)
}

func TestZStatsDossierDialogArmoryNamesAndSpellCounts(t *testing.T) {
	setZStatsTestResolution(t, 1280, 720)
	dialog := newTestZStatsDossierDialog(testZStatsDossier(), newWidgetTestKeyboard(0))
	rowRect := image.Rect(100, 100, 900, 140)

	dialog.switchTab(ZStatsDossierArmoryTab)
	dialog.prepareListRow(rowRect, 0, false)
	armoryRow := dialog.rowOutput.GetOutputStr(false)
	require.Contains(t, armoryRow, references.LongSword.String())
	require.NotContains(t, armoryRow, "(")

	dialog.switchTab(ZStatsDossierSpellsTab)
	dialog.prepareListRow(rowRect, 0, false)
	spellRow := dialog.rowOutput.GetOutputStr(false)
	require.Contains(t, spellRow, "In Lor")
	require.Contains(t, spellRow, "\n"+rowDetailIndent+"Light", "the meaning sits indented under the incantation")
	require.Equal(t, "2", dialog.spellView.count.GetOutputStr(false))
	require.NotContains(t, spellRow, "Prepared")
	require.NotContains(t, spellRow, "NEW")

	dialog.prepareFooter()
	footer := dialog.footerOutput.GetOutputStr(false)
	require.Contains(t, footer, "Tab")
	require.Contains(t, footer, "A-Z/Backspace")
	require.Contains(t, footer, "[Up/Down]")
	require.NotContains(t, footer, "F1")
	require.Contains(t, footer, "[Esc]")

	dialog.switchTab(ZStatsDossierCharacterTab)
	dialog.prepareFooter()
	characterFooter := dialog.footerOutput.GetOutputStr(false)
	require.Contains(t, characterFooter, "Left/Right/1-6")
}

func requireZStatsWideLayout(t *testing.T, layout zStatsDossierLayout) {
	t.Helper()
	require.Positive(t, layout.tabsRect.Dx())
	require.Positive(t, layout.characterPortraitRect.Dx())
	require.Positive(t, layout.characterListRect.Dx())
	require.Positive(t, layout.characterStatsRect.Dx())
	require.Positive(t, layout.characterItemRect.Dx())
	require.Positive(t, layout.globalListRect.Dx())
	require.Positive(t, layout.globalDetailRect.Dx())
	require.Positive(t, layout.footerRect.Dy())
	require.Equal(t, layout.characterPortraitRect.Min.Y, layout.characterListRect.Min.Y)
	require.LessOrEqual(t, layout.characterListRect.Min.X, layout.characterPortraitRect.Max.X)
	require.LessOrEqual(t, layout.characterStatsRect.Min.X, layout.characterListRect.Max.X)
}

func newTestZStatsDossierDialog(dossier game_state.ZStatsDossier, keyboard *input.Keyboard) *ZStatsDossierDialog {
	return NewZStatsDossierDialog(dossier, testZStatsDossierConfig(keyboard))
}

func newRenderedTestZStatsDossierDialog(dossier game_state.ZStatsDossier, keyboard *input.Keyboard) *ZStatsDossierDialog {
	configuration := testZStatsDossierConfig(keyboard)
	configuration.SpriteSheet = testZStatsSpriteSheet()
	return NewZStatsDossierDialog(dossier, configuration)
}

func testZStatsDossierConfig(keyboard *input.Keyboard) ZStatsDossierDialogConfig {
	return ZStatsDossierDialogConfig{
		Keyboard:    keyboard,
		SpriteSheet: &sprites.SpriteSheet{},
		OnDismiss:   func() {},
	}
}

var zStatsTestSpriteSheet *sprites.SpriteSheet

func testZStatsSpriteSheet() *sprites.SpriteSheet {
	// Render fixtures borrow this read-only sheet; state/layout fixtures need no bitmap.
	if zStatsTestSpriteSheet == nil {
		zStatsTestSpriteSheet = &sprites.SpriteSheet{SpriteImage: ebiten.NewImage(512, 256)}
	}
	return zStatsTestSpriteSheet
}

func pressZStatsDossierKey(dialog *ZStatsDossierDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func setZStatsTestResolution(t *testing.T, width, height int) {
	t.Helper()
	setResponsiveDialogTestResolution(t, width, height)
}

func testZStatsDossier() game_state.ZStatsDossier {
	characters := make([]game_state.ZStatsCharacter, 6)
	for index := range characters {
		characters[index] = game_state.ZStatsCharacter{
			RosterIndex:              index * 2,
			SpriteIndex:              indexes.AvatarKeyIndex,
			Name:                     []string{"Avatar", "Shamino", "Iolo", "Mariah", "Geoffrey", "Jaana"}[index],
			Gender:                   "Male",
			Class:                    "Avatar",
			Status:                   "Good",
			Strength:                 20,
			Dexterity:                18,
			Intelligence:             17,
			CurrentHP:                80,
			MaximumHP:                100,
			CurrentMP:                20,
			MaximumMP:                30,
			Experience:               900,
			Level:                    5,
			ExperienceIntoLevel:      400,
			ExperienceForNextLevel:   500,
			ExperienceToNextLevel:    100,
			NextLevelProgress:        0.8,
			ReadyToAdvance:           index == 0,
			HighestUsableSpellCircle: 5,
			CanCast:                  true,
			EquipmentAttack:          12,
			EquipmentDefense:         9,
			Equipped:                 testZStatsEquipment(),
		}
	}

	armory := make([]game_state.ZStatsArmoryItem, 12)
	for index := range armory {
		armory[index] = game_state.ZStatsArmoryItem{
			ID:               30 + index,
			SpriteIndex:      indexes.SpriteIndex(265),
			Name:             "Armory Item",
			Category:         "Weapon",
			Available:        uint16(index % 2),
			Readied:          1,
			Attack:           index,
			StrengthRequired: 2,
		}
	}
	armory[0] = game_state.ZStatsArmoryItem{
		ID: references.LongSword.ID(), SpriteIndex: indexes.SpriteIndex(265),
		Name: "Long Sword", Category: "Weapon", Available: 2, Readied: 1,
		Attack: 8, StrengthRequired: 5, Abilities: []string{"Slashing"},
	}

	return game_state.ZStatsDossier{
		InitialCharacterIndex: 0,
		Characters:            characters,
		Provisions: []game_state.ZStatsProvision{
			{ID: references.Food.ID(), SpriteIndex: indexes.ItemFood, Name: "Food", Quantity: 123, Description: "Food for the party."},
			{ID: references.Gold.ID(), SpriteIndex: indexes.ItemMoney, Name: "Gold", Quantity: 456, Description: "Coin of the realm."},
			{ID: references.Key.ID(), SpriteIndex: indexes.ItemKey, Name: "Keys", Quantity: 7, Description: "Keys open locks."},
			{ID: references.Gem.ID(), SpriteIndex: indexes.ItemGem, Name: "Gems", Quantity: 8, Description: "Gems reveal the nearby world."},
			{ID: references.Torches.ID(), SpriteIndex: indexes.ItemTorch, Name: "Torches", Quantity: 9, Description: "Torches light dungeons."},
			{ID: references.SkullKeys.ID(), SpriteIndex: indexes.AvatarKeyIndex, Name: "Skull Keys", Quantity: 2, Description: "Odd magical keys."},
		},
		Armory: armory,
		Magic: game_state.ZStatsMagicDossier{
			Spells: []game_state.ZStatsSpell{{
				ID: 0, SpriteIndex: indexes.SpriteIndex(260), Name: "In Lor", Syllables: "In Lor", Circle: 1, MPCost: 1, Prepared: 2,
				Targeting: "None", Contexts: "All", FormulaState: "candidate",
				Reagents: []string{"Sulfurous Ash"}, ReagentCandidates: [][]string{{"Ginseng", "Garlic"}},
				Hidden: true, LearningSourceKind: string(game_state.KnowledgeSourceDialogue),
				LearningSource: "Mariah", LearningDate: "8-15-142",
			}},
			Reagents: []game_state.ZStatsInventoryItem{{ID: 0, SpriteIndex: indexes.SpriteIndex(1), Name: "Garlic", Quantity: 0, Description: "A reagent"}},
			Potions:  []game_state.ZStatsInventoryItem{{ID: 0, SpriteIndex: indexes.SpriteIndex(259), Name: "Red Potion", Quantity: 0, Description: "A potion"}},
			Scrolls:  []game_state.ZStatsInventoryItem{{ID: 0, SpriteIndex: indexes.SpriteIndex(260), Name: "In Lor Scroll", Quantity: 0, Description: "A scroll"}},
		},
		Items: []game_state.ZStatsOwnedItem{
			{ID: 1, SpriteIndex: indexes.SpriteIndex(437), Category: "Quest Item", Name: "Crown", Quantity: 1, Description: "An artifact"},
			{ID: 2, SpriteIndex: indexes.SpriteIndex(281), Category: "Moonstone", Name: "Blue Moonstone", Quantity: 1, Description: "A moonstone",
				MoonPhase: game_state.ZStatsMoonPhase{Present: true, Phase: datetime.MoonPhaseFirstQuarter}},
			{ID: 3, Category: "Quest Item", Name: "Unowned", Quantity: 0},
			{ID: 4, Category: "Other", Name: "Ordinary", Quantity: 1},
		},
	}
}

func testZStatsEquipment() []game_state.ZStatsEquippedItem {
	return []game_state.ZStatsEquippedItem{
		{Slot: "Helm", ItemID: references.LeatherHelm.ID(), Name: "Leather Helm", Category: "Helm", StrengthRequired: 1, Abilities: []string{"Protection"}},
		{Slot: "Armour", ItemID: references.ClothArmour.ID(), Name: "Cloth Armour", Category: "Armour", StrengthRequired: 1},
		{Slot: "Weapon", ItemID: references.LongSword.ID(), Name: "Long Sword", Category: "Weapon", StrengthRequired: 2},
		{Slot: "Shield", ItemID: references.SmallShield.ID(), Name: "Small Shield", Category: "Shield", StrengthRequired: 2},
		{Slot: "Ring", ItemID: references.RingProtection.ID(), Name: "Ring of Protection", Category: "Ring"},
		{Slot: "Amulet", ItemID: references.AmuletOfTurning.ID(), Name: "Amulet of Turning", Category: "Amulet"},
	}
}

func TestZStatsMoonstonePixelsUseIntegerPhaseArtAndSelectionContrast(t *testing.T) {
	for _, size := range []image.Point{{320, 200}, {1280, 720}, {2560, 1440}} {
		t.Run(size.String(), func(t *testing.T) {
			setZStatsTestResolution(t, size.X, size.Y)
			dossier := testZStatsDossier()
			dossier.Items = dossier.Items[1:2]
			dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
			t.Cleanup(dialog.outerBorder.Dispose)
			dialog.switchTab(ZStatsDossierItemsTab)
			row := dialog.panelBodyRect(dialog.layout.globalListRect)
			row.Max.Y = row.Min.Y + dialog.listRowHeight(dialog.layout.globalListRect)
			for _, selected := range []bool{false, true} {
				foreground := DialogItemNameColor
				var background stdcolor.Color = pkgcolor.DialogBackground.RGBA()
				if selected {
					foreground, background = DialogSelectedText, DialogSelectedBackground
				}
				counts := make([]int, 8)
				for phase := datetime.MoonPhaseNew; phase <= datetime.MoonPhaseCrescentWaning; phase++ {
					dialog.dossier.Items[0].MoonPhase.Phase = phase
					screen := ebiten.NewImageWithOptions(row, nil)
					screen.Fill(background)
					dialog.drawListRow(screen, row, 0, selected)
					left := row.Min.X + dialog.layout.padding
					glyph := image.Rect(left, row.Min.Y, left+dialog.rowIconWidth, row.Min.Y+dialog.rowIconWidth)
					counts[phase] = requireZStatsIntegerPhasePixels(t, screen, glyph, phase, foreground, background)
					screen.Dispose()
				}
				require.Positive(t, counts[0], "New Moon outline is visible, selected=%v", selected)
				require.Greater(t, counts[4], counts[0], "Full Moon is filled, selected=%v", selected)
			}
			for phase := datetime.MoonPhaseNew; phase <= datetime.MoonPhaseCrescentWaning; phase++ {
				dialog.dossier.Items[0].MoonPhase.Phase = phase
				prepared := dialog.prepareItemDetail(dialog.layout.globalDetailRect)
				screen := ebiten.NewImageWithOptions(dialog.layout.globalDetailRect, nil)
				screen.Fill(pkgcolor.DialogBackground.RGBA())
				dialog.drawItemDetail(screen, dialog.layout.globalDetailRect)
				glyph := prepared.phaseIcon.Inset(dialog.pixelUnit() * zStatsDossierPortraitInsetUnits)
				requireZStatsIntegerPhasePixels(t, screen, glyph, phase, DialogHeaderColor, pkgcolor.DialogBackground.RGBA())
				screen.Dispose()
			}
		})
	}
}

// Compare the actual list/detail renderer against a native 8x8 phase cell. Each
// source pixel must expand into an identical whole-number square, including the
// background holes that make New Moon's outline distinct from Full Moon.
func requireZStatsIntegerPhasePixels(
	t *testing.T, screen *ebiten.Image, glyph image.Rectangle, phase datetime.MoonPhase,
	foreground, background stdcolor.Color,
) int {
	t.Helper()
	require.Equal(t, glyph.Dx(), glyph.Dy())
	require.Zero(t, glyph.Dx()%text.BitmapCharWidth)
	scale := glyph.Dx() / text.BitmapCharWidth
	require.Positive(t, scale)
	baseline := ebiten.NewImage(text.BitmapCharWidth, text.BitmapCharHeight)
	defer baseline.Dispose()
	baseline.Fill(background)
	output := text.NewOutput(text.NewUltimaFont(8), 8, 1, 1)
	output.AppendLine().RunicWithColor(string(rune(0x30+phase)), foreground).Done()
	output.DrawPanelContent(baseline, image.Point{}, 8, etext.AlignStart)
	count := 0
	for y := range text.BitmapCharHeight {
		for x := range text.BitmapCharWidth {
			expected := baseline.At(x, y)
			r, g, b, a := expected.RGBA()
			fr, fg, fb, fa := foreground.RGBA()
			if r == fr && g == fg && b == fb && a == fa {
				count++
			}
			for dy := range scale {
				for dx := range scale {
					requireColorRGBA(t, expected, screen.At(glyph.Min.X+x*scale+dx, glyph.Min.Y+y*scale+dy))
				}
			}
		}
	}
	return count
}

func TestZStatsMoonstoneLorePagingKeepsFullCompactHeading(t *testing.T) {
	setZStatsTestResolution(t, 1280, 720)
	dossier := testZStatsDossier()
	dossier.Items = dossier.Items[1:2]
	dossier.Items[0].Name = references.NewInventoryItemsReferences().GetReferenceByItem(references.CrescentWaning).FullDisplayName()
	dossier.Items[0].MoonPhase.Phase = datetime.MoonPhaseCrescentWaning
	dossier.Items[0].Description = strings.Repeat("Preserved moonstone lore. ", 100)
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestZStatsDossierDialog(dossier, keyboard)
	t.Cleanup(dialog.outerBorder.Dispose)
	dialog.switchTab(ZStatsDossierItemsTab)
	prepared := dialog.prepareItemDetail(dialog.layout.globalDetailRect)
	summary := dialog.summaryOutput.GetOutputStr(false)
	require.Contains(t, summary, dossier.Items[0].Name)
	require.Greater(t, dialog.detailPageCount(), 1)
	screen := ebiten.NewImageWithOptions(dialog.layout.globalDetailRect, nil)
	t.Cleanup(screen.Dispose)
	dialog.drawItemDetail(screen, dialog.layout.globalDetailRect)
	header := prepared.phaseIcon.Union(prepared.summary)
	before := make([]byte, header.Dx()*header.Dy()*4)
	screen.SubImage(header).(*ebiten.Image).ReadPixels(before)
	pages := dialog.detailPageCount()
	for page := 1; page < pages; page++ {
		pressZStatsDossierKey(dialog, keyboard, ebiten.KeySpace)
		screen.Clear()
		dialog.drawItemDetail(screen, dialog.layout.globalDetailRect)
		require.Equal(t, page, dialog.detailPage)
		require.Equal(t, summary, dialog.summaryOutput.GetOutputStr(false))
		after := make([]byte, len(before))
		screen.SubImage(header).(*ebiten.Image).ReadPixels(after)
		require.Equal(t, before, after, "paging lore must retain the compact phase header")
	}
	pressZStatsDossierKey(dialog, keyboard, ebiten.KeySpace)
	require.Zero(t, dialog.detailPage)
}

func TestZStatsMoonstoneBypassesSpriteWhileQuestPortraitStillDraws(t *testing.T) {
	setZStatsTestResolution(t, 1280, 720)
	configuration := testZStatsDossierConfig(newWidgetTestKeyboard(0))
	sheet := ebiten.NewImage(512, 256)
	t.Cleanup(sheet.Dispose)
	sheet.Fill(pkgcolor.EGALightGreen)
	configuration.SpriteSheet = &sprites.SpriteSheet{SpriteImage: sheet}
	dialog := NewZStatsDossierDialog(testZStatsDossier(), configuration)
	t.Cleanup(dialog.outerBorder.Dispose)
	dialog.switchTab(ZStatsDossierItemsTab)
	screen := ebiten.NewImageWithOptions(dialog.layout.globalDetailRect, nil)
	t.Cleanup(screen.Dispose)
	dialog.drawItemDetail(screen, dialog.layout.globalDetailRect)
	prepared := dialog.prepareItemDetail(dialog.layout.globalDetailRect)
	requireImageRegionContainsColor(t, screen, prepared.portrait, pkgcolor.EGALightGreen)
	dialog.setCurrentSelection(1)
	screen.Clear()
	dialog.drawItemDetail(screen, dialog.layout.globalDetailRect)
	prepared = dialog.prepareItemDetail(dialog.layout.globalDetailRect)
	require.True(t, prepared.portrait.Empty())
	for y := prepared.phaseIcon.Min.Y; y < prepared.phaseIcon.Max.Y; y++ {
		for x := prepared.phaseIcon.Min.X; x < prepared.phaseIcon.Max.X; x++ {
			requireColorNotRGBA(t, pkgcolor.EGALightGreen, screen.At(x, y))
		}
	}
}
