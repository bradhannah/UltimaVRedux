package widgets

import (
	"image"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestControlHintSegmentsSeparateKeysFromActionsWithoutChangingCopy(t *testing.T) {
	output := text.NewOutput(nil, 1, 4, 80)
	line := AppendControlHints(output.AppendLine(), "Y/N: Choose   Enter: Confirm   Esc: Cancel")
	for _, segment := range line.GetSegments() {
		if strings.Contains(segment.Text, "Choose") || strings.Contains(segment.Text, "Confirm") || strings.Contains(segment.Text, "Cancel") {
			requireColorEqual(t, DialogControlHintColor, segment.Color)
		} else if strings.TrimSpace(segment.Text) != "" {
			requireColorEqual(t, DialogControlKeyColor, segment.Color)
		}
	}
	line.Done()
	require.Equal(t, "Y/N: Choose   Enter: Confirm   Esc: Cancel", output.GetOutputStr(false))
}

func TestReadyFooterMatchesAuthoritativeSelection(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	character := state.PartyState.GetAvatar()
	character.Strength = 30
	for slot := references.SlotHelm; slot <= references.SlotAmulet; slot++ {
		character.SetEquippedItem(slot, references.NoEquipment)
	}
	character.SetEquippedItem(references.SlotHelm, references.LeatherHelm)
	for _, gear := range []references.Equipment{references.Dagger, references.Arrows, references.ChainCoif} {
		state.PartyState.Inventory.Equipment.Set(gear, 1)
	}
	dialog := NewReadyEquipmentDialog(newWidgetTestKeyboard(-1), &sprites.SpriteSheet{}, state, game_state.ReadyEquipmentRequest{})
	for _, test := range []struct {
		gear           references.Equipment
		action, status string
	}{
		{references.Dagger, "Ready", "Ready to equip"},
		{references.LeatherHelm, "Unready", "Enter removes it"},
		{references.Arrows, "No change", "No equipment change"},
		{references.ChainCoif, "Review replacement", "Replacing removes other items"},
	} {
		found := false
		for index, item := range dialog.availableItems {
			if item.Equipment != test.gear {
				continue
			}
			found = true
			dialog.itemListNav.SetIndex(index)
			status, action, _ := dialog.selectionGuidance()
			require.Equal(t, test.status, status)
			require.Equal(t, test.action, action)
			dialog.populateStatusOutput(dialog.statusBorder.GetInteriorBounds().Dx() - (itemDialogTextPadding+4)*2)
			require.Contains(t, dialog.statusOutput.GetOutputStr(true), test.action)
		}
		require.True(t, found)
	}
}

func TestCastGuidanceUsesResolvedTargetAndScope(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	state.PartyState.GetAvatar().Class = party_state.Wizard
	for _, test := range []struct {
		spell   references.Spell
		context references.SpellContext
		choice  references.TargetingType
		scope   references.SpellEffectScope
		enter   string // What the Enter hint promises for this spell here.
	}{
		{references.InPor, references.ContextCombat, references.TargetAutomatic, references.ScopeCaster, "Cast now"},
		{references.InPor, references.ContextOverworld, references.TargetDirection, references.ScopeWorldTransition, "Choose direction"},
		{references.Mani, references.ContextTown, references.TargetPartyMember, references.ScopeSingleCharacter, "Choose who"},
		{references.Mani, references.ContextCombat, references.TargetCombatPartyMember, references.ScopeSingleCharacter, "Choose who"},
		{references.AnYlem, references.ContextCombat, references.TargetAdjacentTile, references.ScopeSingleTile, "Aim"},
		{references.InFlamGrav, references.ContextCombat, references.TargetTileCrosshair, references.ScopeSingleTile, "Aim"},
		{references.InFlamHur, references.ContextCombat, references.TargetDirection, references.ScopeCone, "Aim"},
		{references.InFlamGrav, references.ContextDungeon, references.TargetAutomatic, references.ScopeSingleTile, "Cast now"},
		{references.VasRelPor, references.ContextTown, references.TargetMoonPhase, references.ScopeWorldTransition, "Name the phase"},
	} {
		for _, size := range []image.Point{{1280, 720}, {3840, 2160}} {
			setGuidanceTestResolution(t, size.X, size.Y)
			catalog := testMixCatalog()
			entry := &catalog[test.spell]
			profile := references.GetSpellTargetingProfile(test.spell, test.context)
			entry.Targeting, entry.Scope = references.SpellTargetLabel(test.spell, test.context), profile.Scope
			entry.Unaimed = profile.Selection == references.TargetAutomatic
			entry.Contexts = references.GetSpellInfo(test.spell).Context.PlayerLabel()
			dialog := NewSpellCastingDialog(newWidgetTestKeyboard(-1), &state.PartyState, test.context, catalog, state.ValidateSpellTargetingPrerequisites)
			dialog.browser.selectID(test.spell.ID())
			dialog.ensureLayout()
			content := strings.Join(strings.Fields(dialog.view.body.GetOutputStr(true)), " ")
			require.Contains(t, content, "Circle:")
			require.Contains(t, content, "Costs:")
			require.NotContains(t, content, "Recipe", "recipes belong to Mix and Z-Stats")
			require.Contains(t, content, "Target: "+entry.Targeting)
			if entry.Unaimed {
				require.NotContains(t, content, "Affects:", "the Target line already says what an unaimed spell falls on")
			} else {
				require.Contains(t, content, "Target: "+test.choice.PlayerLabel())
				require.Contains(t, content, "Affects: "+spellScopeLabel(test.scope))
			}
			require.Contains(t, content, strings.Join(strings.Fields(entry.Description), " "))
			require.Contains(t, content, "Where: "+entry.Contexts)
			require.Positive(t, dialog.browser.pages, "overflow is accessible through detail paging")
			require.Contains(t, dialog.view.footer.GetOutputStr(false), "[Enter] "+test.enter)
		}
	}
}

func TestCommandHelpKeyboardPagingResizeAndClose(t *testing.T) {
	setGuidanceTestResolution(t, 1280, 720)
	held := make(map[ebiten.Key]bool)
	keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	closed := 0
	entries := make([]CommandHelpEntry, 40)
	for index := range entries {
		entries[index] = CommandHelpEntry{Key: "A", Purpose: "Attack", NextInput: "Arrow direction; Space Pass"}
	}
	dialog := NewCommandHelpDialog(entries, HelpStatusView{}, keyboard, func() { closed++ })
	dialog.showPage(helpPageCommands)
	entries[0].Purpose = "changed externally"
	require.NotContains(t, dialog.output.GetOutputStr(true), "changed externally")
	press := func(key ebiten.Key) {
		held[key] = true
		dialog.Update()
		delete(held, key)
		keyboard.ClearFrameConsumption()
		dialog.Update()
	}
	press(ebiten.KeyPageDown)
	require.Greater(t, dialog.row, 0)
	press(ebiten.KeyEnd)
	require.Equal(t, dialog.lastRow(), dialog.row)
	display.GetManager().PublishLogicalSize(3840, 2160)
	dialog.Update()
	require.Greater(t, dialog.row, 0)
	press(ebiten.KeyHome)
	require.Zero(t, dialog.row)
	press(ebiten.KeyEscape)
	require.Equal(t, 1, closed)
}

func setGuidanceTestResolution(t *testing.T, width, height int) {
	t.Helper()
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(width, height)
}
