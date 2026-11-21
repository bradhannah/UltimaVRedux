package widgets

import (
	"image"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestMixAll48KeyboardRequests(t *testing.T) {
	// Independent identity table: neither display labels nor the widget lookup
	// generate the expected requests. In particular Mani != Vas Mani.
	cases := []struct {
		keys  string
		spell references.Spell
	}{
		{"IL", references.InLor}, {"GP", references.GravPor}, {"AZ", references.AnZu},
		{"AN", references.AnNox}, {"M", references.Mani}, {"AY", references.AnYlem},
		{"AS", references.AnSanct}, {"AXC", references.AnXenCorp}, {"RH", references.RelHur},
		{"IW", references.InWis}, {"KX", references.KalXen}, {"IXM", references.InXenMani},
		{"VL", references.VasLor}, {"VF", references.VasFlam}, {"IFG", references.InFlamGrav},
		{"ING", references.InNoxGrav}, {"IZG", references.InZuGrav}, {"IP", references.InPor},
		{"AG", references.AnGrav}, {"IS", references.InSanct}, {"ISG", references.InSanctGrav},
		{"UP", references.UusPor}, {"DP", references.DesPor}, {"WQ", references.WisQuas},
		{"IBX", references.InBetXen}, {"AEP", references.AnExPor}, {"IEP", references.InExPor},
		{"VM", references.VasMani}, {"IZ", references.InZu}, {"RT", references.RelTym},
		{"IVPY", references.InVasPorYlem}, {"QAW", references.QuasAnWis}, {"IA", references.InAn},
		{"WAY", references.WisAnYlem}, {"AXE", references.AnXenEx}, {"RXB", references.RelXenBet},
		{"SL", references.SanctLor}, {"XC", references.XenCorp}, {"IQX", references.InQuasXen},
		{"IQW", references.InQuasWis}, {"INH", references.InNoxHur}, {"IQC", references.InQuasCorp},
		{"IMC", references.InManiCorp}, {"KXC", references.KalXenCorp}, {"IVGC", references.InVasGravCorp},
		{"IFH", references.InFlamHur}, {"VRP", references.VasRelPor}, {"AT", references.AnTym},
	}
	require.Len(t, cases, 48)
	catalog := testMixCatalog()
	for _, test := range cases {
		t.Run(test.keys, func(t *testing.T) {
			held := make(map[ebiten.Key]bool)
			keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
			reagents := make([]ReagentInfo, 8)
			for index := range reagents {
				reagents[index] = ReagentInfo{Reagent: references.Reagent(index), Quantity: 99}
			}
			dialog := NewReagentMixingDialog(keyboard, catalog, reagents)
			press := func(key ebiten.Key) {
				held[key] = true
				dialog.Update()
				delete(held, key)
				keyboard.ClearFrameConsumption()
				dialog.Update()
			}
			for _, letter := range test.keys {
				press(ebiten.KeyA + ebiten.Key(letter-'A'))
			}
			require.False(t, dialog.HasPendingMixRequest(), "typing cannot commit")
			press(ebiten.KeyEnter)
			require.False(t, dialog.HasPendingMixRequest(), "choosing a spell cannot commit")
			press(ebiten.KeyEnter)
			require.True(t, dialog.HasPendingMixRequest())
			request := dialog.ConsumePendingMixRequest()
			require.Equal(t, test.spell, request.Spell)
			require.Equal(t, 1, request.Quantity)
			require.Equal(t, references.GetSpellReagents(test.spell), request.ReagentMask)
			for _, reagent := range dialog.reagentInventory {
				require.Equal(t, 99, reagent.Quantity)
			}
		})
	}
}

// mixTestKeys presses real keys on a held-key source, one press per call.
func mixTestKeys(dialog **ReagentMixingDialog, catalog []game_state.ZStatsSpell, reagents []ReagentInfo,
) func(...ebiten.Key) {
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	*dialog = NewReagentMixingDialog(keyboard, catalog, reagents)
	return func(keys ...ebiten.Key) {
		for _, key := range keys {
			held[key] = true
			(*dialog).Update()
			delete(held, key)
			keyboard.ClearFrameConsumption()
			(*dialog).Update()
		}
	}
}

func TestMixKeyboardQuantityClearAndSpellEdit(t *testing.T) {
	var dialog *ReagentMixingDialog
	press := mixTestKeys(&dialog, testMixCatalog(), []ReagentInfo{{Reagent: references.SulfurAsh, Quantity: 99}})
	press(ebiten.KeyI, ebiten.KeyL, ebiten.KeyEnter, ebiten.Key2, ebiten.Key3)
	require.Equal(t, 23, dialog.amount())
	press(ebiten.KeyDelete)
	require.Equal(t, 1, dialog.amount())
	require.Equal(t, references.InLor.ID(), dialog.spell.ID)
	press(ebiten.Key5, ebiten.KeyBackspace)
	require.Equal(t, 1, dialog.amount())
	require.Equal(t, references.InLor.ID(), dialog.spell.ID)
	require.False(t, dialog.HasPendingMixRequest())
}

func TestMixArrowsStepTheAmountWithinWhatIngredientsAllow(t *testing.T) {
	var dialog *ReagentMixingDialog
	press := mixTestKeys(&dialog, testMixCatalog(), []ReagentInfo{{Reagent: references.SulfurAsh, Quantity: 3}})
	dialog.ChooseSpell(references.InLor, true)
	press(ebiten.KeyLeft)
	require.Equal(t, 1, dialog.amount(), "Left stops at one")
	press(ebiten.KeyRight, ebiten.KeyRight, ebiten.KeyRight, ebiten.KeyRight)
	require.Equal(t, 3, dialog.amount(), "Right stops at what the ingredients allow")
	press(ebiten.KeyLeft, ebiten.KeyEnter)
	require.Equal(t, 2, dialog.ConsumePendingMixRequest().Quantity)
}

func TestMixMostSetsTheLargestAmountThatFits(t *testing.T) {
	for _, test := range []struct {
		prepared          uint16
		ingredients, want int
		topLine           string
	}{
		{92, 18, 7, "Ready to mix."}, {92, 3, 3, "Ready to mix."}, {0, 99, 99, "Ready to mix."},
		{99, 18, 1, "Already 99 prepared. None would fit."},
		{0, 0, 0, "Cannot mix: no Sulfurous Ash."},
	} {
		catalog := testMixCatalog()
		catalog[references.InLor].Prepared = test.prepared
		var dialog *ReagentMixingDialog
		press := mixTestKeys(&dialog, catalog,
			[]ReagentInfo{{Reagent: references.SulfurAsh, Name: "Sulfurous Ash", Quantity: test.ingredients}})
		dialog.ChooseSpell(references.InLor, true)
		press(ebiten.KeyM)
		require.Equal(t, test.want, dialog.amount())
		require.False(t, dialog.HasPendingMixRequest(), "M never mixes")
		sentence, _ := dialog.topLine()
		require.Equal(t, test.topLine, sentence)
		press(ebiten.KeyEnter)
		require.Equal(t, test.ingredients > 0, dialog.HasPendingMixRequest(), "a wasteful mix stays allowed")
		if test.ingredients > 0 {
			require.Equal(t, test.want, dialog.ConsumePendingMixRequest().Quantity)
		}
	}
}

func testMixCatalog() []game_state.ZStatsSpell {
	catalog := make([]game_state.ZStatsSpell, 0, references.TotalSpellCount)
	for spell := range references.Spell(references.TotalSpellCount) {
		info := references.GetSpellInfo(spell)
		catalog = append(catalog, game_state.ZStatsSpell{ID: spell.ID(), Name: info.Name, Syllables: info.Name,
			Description: info.Description, EffectLabel: references.SpellEffectLabel(spell), RecipeMask: references.GetSpellReagents(spell),
			FormulaState: string(game_state.SpellFormulaConfirmed)})
	}
	return catalog
}

func TestMixSingleDigitReplacementAndQuantityBoundaries(t *testing.T) {
	for _, maximum := range []int{0, 6, 12, 99} {
		dialog := NewReagentMixingDialog(newWidgetTestKeyboard(0), testMixCatalog(), []ReagentInfo{{Reagent: references.SulfurAsh, Quantity: maximum}})
		dialog.ChooseSpell(references.InLor, true)
		dialog.handleQuantityInput(6)
		dialog.handleQuantityInput(5)
		sentence, sentenceColor := dialog.topLine()
		switch maximum {
		case 0:
			require.Equal(t, 0, dialog.amount())
		case 6:
			require.Equal(t, 5, dialog.amount())
		case 12:
			require.Equal(t, 6, dialog.amount(), "65 is rejected, not silently clamped")
			require.Equal(t, "Maximum is 12; digit not accepted.", sentence)
			require.Equal(t, DialogWarningColor, sentenceColor, "a refusal is written in the readable red")
		case 99:
			require.Equal(t, 65, dialog.amount())
		}
	}
}

func TestMixRapidRepeatedAndKeypadDigits(t *testing.T) {
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(165, func(key ebiten.Key) bool { return held[key] })
	dialog := NewReagentMixingDialog(keyboard, testMixCatalog(), []ReagentInfo{{Reagent: references.SulfurAsh, Quantity: 99}})
	dialog.ChooseSpell(references.InLor, true)
	press := func(key ebiten.Key) {
		held[key] = true
		dialog.Update()
		dialog.Update()
		clear(held)
		keyboard.ClearFrameConsumption()
	}
	press(ebiten.Key1)
	press(ebiten.Key1)
	require.Equal(t, 11, dialog.amount(), "separate presses register even inside the movement delay")
	press(ebiten.KeyNumpad9)
	press(ebiten.KeyNumpad9)
	require.Equal(t, 99, dialog.amount())
	press(ebiten.KeyBackspace)
	require.Equal(t, 1, dialog.amount())
	press(ebiten.Key0)
	require.Equal(t, 1, dialog.amount())
	press(ebiten.KeyNumpad1)
	press(ebiten.KeyNumpad0)
	press(ebiten.KeyNumpadEnter)
	require.Equal(t, 10, dialog.ConsumePendingMixRequest().Quantity)
}

// The Recipe tab can only mix the recorded recipe. Trying another mixture is
// a deliberate Tab away, where every ingredient has a tick box.
func TestMixRecipeTabCannotChangeTheMixtureAndExperimentCan(t *testing.T) {
	var dialog *ReagentMixingDialog
	press := mixTestKeys(&dialog, testMixCatalog(), fullTestReagentInventory())
	dialog.ChooseSpell(references.VasFlam, true)
	recipe := references.GetSpellReagents(references.VasFlam)
	require.Equal(t, mixTabRecipe, dialog.tab)
	press(ebiten.KeyDown, ebiten.KeySpace)
	require.Equal(t, recipe, dialog.mask(), "Space ticks nothing on the Recipe tab")
	sentence, sentenceColor := dialog.topLine()
	require.Equal(t, "Ready to mix.", sentence)
	require.Equal(t, DialogLabelColor, sentenceColor)

	press(ebiten.KeyTab)
	require.Equal(t, mixTabExperiment, dialog.tab)
	require.Equal(t, recipe, dialog.mask(), "the recipe starts ticked")
	press(ebiten.KeyDown, ebiten.KeySpace)
	ginseng := uint16(1) << uint(references.Ginseng)
	require.Equal(t, recipe|ginseng, dialog.mask())
	sentence, sentenceColor = dialog.topLine()
	require.Equal(t, "Untested: it may be wasted and may hurt.", sentence)
	require.Equal(t, DialogWarningColor, sentenceColor)

	press(ebiten.KeyTab)
	require.Equal(t, recipe, dialog.mask(), "Recipe still mixes the recorded recipe")
	press(ebiten.KeyTab, ebiten.Key3, ebiten.KeyEnter)
	request := dialog.ConsumePendingMixRequest()
	require.Equal(t, recipe|ginseng, request.ReagentMask, "the experiment is kept across tabs and requested as ticked")
	require.Equal(t, 3, request.Quantity)
}

func TestMixSpellWithoutRecordedRecipeOpensOnExperimentAlone(t *testing.T) {
	catalog := testMixCatalog()
	catalog[references.VasFlam].RecipeMask, catalog[references.VasFlam].FormulaState = 0, "uncertain"
	catalog[references.VasFlam].ReagentCandidates = [][]string{{"Sulfurous Ash", "Black Pearl"}}
	var dialog *ReagentMixingDialog
	press := mixTestKeys(&dialog, catalog, fullTestReagentInventory())
	dialog.ChooseSpell(references.VasFlam, true)
	require.Equal(t, []mixTab{mixTabExperiment}, dialog.tabs())
	sentence, _ := dialog.topLine()
	require.Equal(t, "No recipe recorded. Try ingredients.", sentence)
	press(ebiten.KeyTab, ebiten.KeyEnter)
	require.Equal(t, mixTabExperiment, dialog.tab, "there is no Recipe tab to open")
	require.False(t, dialog.HasPendingMixRequest(), "nothing ticked, nothing to mix")
	require.True(t, dialog.NeedsResultBox(), "a recipe learned by experiment keeps its result box")
	press(ebiten.KeySpace, ebiten.KeyEnter)
	require.Equal(t, uint16(1)<<uint(references.SulfurAsh), dialog.ConsumePendingMixRequest().ReagentMask)
}

func TestMixTopLineNamesWhatIsMissing(t *testing.T) {
	reagents := fullTestReagentInventory()
	reagents[references.MandrakeRoot].Quantity = 0
	dialog := NewReagentMixingDialog(newWidgetTestKeyboard(0), testMixCatalog(), reagents)
	dialog.ChooseSpell(references.KalXen, true)
	sentence, sentenceColor := dialog.topLine()
	require.Equal(t, "Cannot mix: no Mandrake Root.", sentence)
	require.Equal(t, DialogWarningColor, sentenceColor)
	require.Equal(t, DialogWarningColor, dialog.view.warning, "the list's details name a shortage in the same red")
	require.Zero(t, dialog.amount())

	reagents[references.SpiderSilk].Quantity = 0
	dialog = NewReagentMixingDialog(newWidgetTestKeyboard(0), testMixCatalog(), reagents)
	dialog.ChooseSpell(references.KalXen, true)
	sentence, _ = dialog.topLine()
	require.Equal(t, "Cannot mix: lacks 2 ingredients.", sentence)
}

func TestMixTopLineStatesTheLossBeforeEnter(t *testing.T) {
	catalog := testMixCatalog()
	catalog[references.InLor].Prepared = 97
	dialog := NewReagentMixingDialog(newWidgetTestKeyboard(0), catalog, fullTestReagentInventory())
	dialog.ChooseSpell(references.InLor, true)
	dialog.handleQuantityInput(5)
	sentence, sentenceColor := dialog.topLine()
	require.Equal(t, "Only 2 fit. 3 would be thrown away.", sentence)
	require.Equal(t, DialogWarningColor, sentenceColor)
	dialog.setMost()
	require.Equal(t, 2, dialog.amount())
	sentence, _ = dialog.topLine()
	require.Equal(t, "Ready to mix.", sentence)
}

// The Mix list opens on what can be mixed now. That is a convenience, not a
// wall: initials that match nothing there find the spell among every known one.
func TestMixListOpensOnMixableAndWidensForTypedInitials(t *testing.T) {
	catalog := testMixCatalog()
	catalog[references.InLor].CanMix, catalog[references.Mani].CanMix = 4, 2
	var dialog *ReagentMixingDialog
	press := mixTestKeys(&dialog, catalog, fullTestReagentInventory())
	require.Equal(t, spellFilterMixable, dialog.browser.filterMode)
	require.Len(t, dialog.browser.filtered, 2)
	press(ebiten.KeyK, ebiten.KeyX)
	require.Equal(t, spellFilterAll, dialog.browser.filterMode, "Kal Xen cannot be mixed, so the list widens to find it")
	require.Equal(t, references.KalXen.ID(), dialog.browser.current().ID)

	nothingMixable := NewReagentMixingDialog(newWidgetTestKeyboard(0), testMixCatalog(), nil)
	require.Equal(t, spellFilterAll, nothingMixable.browser.filterMode, "an empty list would say less than every known spell")
	require.Len(t, nothingMixable.browser.filtered, references.TotalSpellCount)
}

// A plain success returns to the list with nothing to close and nothing to
// clear: the result is on the header, the typed letters are gone, and the spell
// keeps its row even when the mix used the last of an ingredient.
func TestMixSuccessReturnsToListWithResultAndClearedLetters(t *testing.T) {
	catalog := testMixCatalog()
	catalog[references.VasFlam].CanMix, catalog[references.Mani].CanMix = 4, 2
	var dialog *ReagentMixingDialog
	press := mixTestKeys(&dialog, catalog, fullTestReagentInventory())
	press(ebiten.KeyV, ebiten.KeyF, ebiten.KeyEnter, ebiten.Key4, ebiten.KeyEnter)
	require.False(t, dialog.NeedsResultBox())
	request := dialog.ConsumePendingMixRequest()
	catalog[references.VasFlam].CanMix, catalog[references.VasFlam].Prepared = 0, 4
	dialog.Acknowledge(game_state.MixOutcome{Success: true, SpellName: "Vas Flam", Quantity: request.Quantity,
		Added: 4, Stock: 4}, catalog, fullTestReagentInventory())

	require.False(t, dialog.preparing)
	require.Equal(t, "Mixed 4 Vas Flam. You now have 4.", dialog.result)
	require.False(t, dialog.resultLoss)
	require.Empty(t, dialog.browser.syllables, "the letters typed for the last spell are cleared")
	require.Equal(t, spellFilterMixable, dialog.browser.filterMode)
	require.Equal(t, references.VasFlam.ID(), dialog.browser.current().ID, "the mixed spell keeps its row")
	require.Equal(t, uint16(4), dialog.browser.current().Prepared)

	press(ebiten.KeyM)
	require.Empty(t, dialog.result, "the result gives way once the list is asked for something else")
	require.Equal(t, references.Mani.ID(), dialog.browser.current().ID, "one letter picks the next spell")
	require.Len(t, dialog.browser.filtered, 1, "Vas Flam, no longer mixable, has left the Mixable list")
}

// A failed mix with no trap leaves the window open. An earlier success line
// would then describe stock that the failure has since spent.
func TestMixFailureClearsAnEarlierResult(t *testing.T) {
	catalog := testMixCatalog()
	catalog[references.VasFlam].CanMix = 4
	var dialog *ReagentMixingDialog
	press := mixTestKeys(&dialog, catalog, fullTestReagentInventory())
	press(ebiten.KeyV, ebiten.KeyF, ebiten.KeyEnter, ebiten.KeyEnter)
	dialog.ConsumePendingMixRequest()
	dialog.Acknowledge(game_state.MixOutcome{Success: true, SpellName: "Vas Flam", Quantity: 1, Added: 1, Stock: 1},
		catalog, fullTestReagentInventory())
	require.NotEmpty(t, dialog.result)

	press(ebiten.KeyEnter, ebiten.KeyTab, ebiten.KeyDown, ebiten.KeySpace, ebiten.KeyEnter)
	dialog.ConsumePendingMixRequest()
	dialog.Acknowledge(game_state.MixOutcome{Message: "Reagents wasted!"}, catalog, fullTestReagentInventory())
	require.True(t, dialog.preparing, "with no trap the window stays open")
	sentence, sentenceColor := dialog.topLine()
	require.Equal(t, "Reagents wasted!", sentence)
	require.Equal(t, DialogWarningColor, sentenceColor)
	require.Empty(t, dialog.result)
	require.Equal(t, -1, dialog.browser.keepID)
}

func TestMixResultLineSaysWhatWasLost(t *testing.T) {
	for _, test := range []struct {
		added int
		want  string
		loss  bool
	}{
		{5, "Mixed 5 In Lor. You now have 99.", false},
		{2, "Only 2 of 5 In Lor fit. You now have 99.", true},
		{0, "No room for 5 In Lor. You still have 99.", true},
	} {
		line, loss := mixResultLine(game_state.MixOutcome{Success: true, SpellName: "In Lor", Quantity: 5,
			Added: test.added, Stock: 99})
		require.Equal(t, test.want, line)
		require.Equal(t, test.loss, loss)
	}
}

// Every sentence, table row and hint row of the quantity window is written for
// a window fifty letters wide. This holds that promise at every supported size,
// with the longest spell names and the largest numbers, so nothing is ever cut.
func TestMixWindowAndResultTextFitEverySupportedSize(t *testing.T) {
	catalog := testMixCatalog()
	for index := range catalog {
		catalog[index].Prepared = 98
	}
	empty := fullTestReagentInventory()
	for index := range empty {
		empty[index].Quantity = 0
	}
	for _, size := range config.SupportedWindowResolutions() {
		setGuidanceTestResolution(t, size.X, size.Y)
		for spell := range references.Spell(references.TotalSpellCount) {
			dialog := NewReagentMixingDialog(newWidgetTestKeyboard(0), catalog, fullTestReagentInventory())
			dialog.ChooseSpell(spell, false)
			dialog.ensureLayout()
			view, window := &dialog.view, dialog.window
			require.True(t, window.header.Union(window.footer).In(image.Rect(0, 0, size.X, size.Y)),
				"%dx%d: the window is on the screen", size.X, size.Y)
			letters := view.font.CharsForWidth(window.body.Inset(view.padding).Dx())
			require.GreaterOrEqual(t, letters, mixWindowColumns, "%dx%d", size.X, size.Y)
			header := window.header.Inset(view.padding).Dx()
			label := dialog.spell.Name + " - " + dialog.spell.EffectLabel
			require.Equal(t, label, FitDialogLine(label, view.title.Font, header), "%dx%d: %s is shown whole", size.X, size.Y, label)

			sentences := make([]string, 0, 8)
			collect := func() {
				sentence, _ := dialog.topLine()
				sentences = append(sentences, sentence)
			}
			collect()
			dialog.handleQuantityInput(8)
			collect()
			dialog.tab, dialog.selection = mixTabExperiment, 0xff
			collect()
			dialog.notice = "Maximum is 99; digit not accepted."
			collect()
			missing := NewReagentMixingDialog(newWidgetTestKeyboard(0), catalog, empty)
			missing.ChooseSpell(spell, false)
			sentence, _ := missing.topLine()
			sentences = append(sentences, sentence)
			for _, sentence := range sentences {
				require.LessOrEqual(t, len(sentence), mixWindowColumns, sentence)
			}

			listHeader := view.headerRect.Inset(view.padding).Dx()
			for _, added := range []int{99, 98, 0} {
				line, _ := mixResultLine(game_state.MixOutcome{Success: true, SpellName: dialog.spell.Name,
					Quantity: 99, Added: added, Stock: 99})
				width, _ := view.title.Font.MeasureString(line)
				require.LessOrEqual(t, int(width), listHeader, "%dx%d: %s", size.X, size.Y, line)
			}
		}
		list := NewReagentMixingDialog(newWidgetTestKeyboard(0), catalog, fullTestReagentInventory())
		prompt, _ := list.view.font.MeasureString("Spell: _  " + spellInitialsPrompt)
		require.LessOrEqual(t, int(prompt), list.view.searchRect.Inset(list.view.padding).Dx(), "%dx%d", size.X, size.Y)
	}
}

// The amount window is a window on top of the spell list: the list stays drawn
// behind it, shaded, and the window itself keeps its full brightness.
func TestMixAmountWindowSitsOnItsShadedList(t *testing.T) {
	setGuidanceTestResolution(t, 1280, 720)
	dialog := NewReagentMixingDialog(newWidgetTestKeyboard(0), testMixCatalog(), fullTestReagentInventory())
	dialog.ChooseSpell(references.InLor, true)
	require.True(t, dialog.preparing)
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	dialog.Draw(screen)

	window := dialog.window.header.Union(dialog.window.footer)
	list := dialog.view.headerRect
	require.False(t, list.In(window), "the list is wider than the window on it")
	behind := image.Rect(list.Min.X, list.Min.Y, window.Min.X, list.Max.Y)
	brightest := uint32(0)
	for y := behind.Min.Y; y < behind.Max.Y; y++ {
		for x := behind.Min.X; x < behind.Max.X; x++ {
			red, _, _, _ := screen.At(x, y).RGBA()
			brightest = max(brightest, red>>8)
		}
	}
	require.Positive(t, brightest, "the list is still drawn behind the window")
	require.LessOrEqual(t, brightest, uint32(255-backdropShadeAlpha), "and it is shaded")
	requireImageRegionContainsColor(t, screen, dialog.window.header, DialogHeaderColor)
}
