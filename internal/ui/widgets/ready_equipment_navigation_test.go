package widgets

import (
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

func pressReadyNavigation(dialog *ReadyEquipmentDialog, held map[ebiten.Key]bool, key ebiten.Key, shift bool) {
	dialog.keyboard.ClearFrameConsumption()
	held[ebiten.KeyShift] = shift
	dialog.keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func TestReadyTabCyclesEligibleMembersAndResetsComparison(t *testing.T) {
	state, dialog, held := newReadyReviewFixture(t)
	for index := range party_state.MaxActiveParty {
		character := state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapActive
		character.SetStatus(party_state.Good)
		character.SetHp(100)
	}
	state.PartyState.GetCharacter(1).SetStatus(party_state.Dead)
	state.PartyState.GetCharacter(3).PartyMap = party_state.PartyMapUnrecruited
	state.PartyState.GetCharacter(4).SetHp(0)
	before := state.PartyState.Inventory.LockerSnapshot()
	avatar := *state.PartyState.GetAvatar()
	for _, step := range []struct {
		key   ebiten.Key
		shift bool
		want  int
	}{
		{ebiten.KeyTab, false, 2}, {ebiten.KeyTab, false, 5}, {ebiten.KeyTab, false, 0},
		{ebiten.KeyTab, true, 5}, {ebiten.KeyTab, true, 2}, {ebiten.KeyTab, true, 0},
		{ebiten.KeyRight, false, 2}, {ebiten.KeyLeft, false, 0},
		{ebiten.Key6, false, 5}, {ebiten.Key2, false, 5}, {ebiten.Key1, false, 0},
	} {
		dialog.targetHand = references.SlotShield
		dialog.detailPage = 2
		dialog.warningMessage = "old warning"
		previous := dialog.selectedCharIdx
		pressReadyNavigation(dialog, held, step.key, step.shift)
		require.Equal(t, step.want, dialog.selectedCharIdx)
		if previous != step.want {
			require.Zero(t, dialog.detailPage)
			require.Empty(t, dialog.warningMessage)
			require.Equal(t, references.SlotNone, dialog.targetHand)
			require.Nil(t, dialog.review)
		}
		_, submitted := dialog.TakeTransaction()
		require.False(t, submitted)
	}
	require.Equal(t, before, state.PartyState.Inventory.LockerSnapshot())
	require.Equal(t, avatar, *state.PartyState.GetAvatar())
}

func TestReadyTabSingleMemberSkipsAbsentRosterSlots(t *testing.T) {
	state, dialog, held := newReadyReviewFixture(t)
	var roster [party_state.NRosterSlots]*party_state.PlayerCharacter
	roster[0] = state.PartyState.GetAvatar()
	state.PartyState = *party_state.NewPartyStateFromRoster(roster)
	for _, shift := range []bool{false, true} {
		pressReadyNavigation(dialog, held, ebiten.KeyTab, shift)
		require.Zero(t, dialog.selectedCharIdx)
		require.Equal(t, "CHAR 1/1", dialog.characterPositionLabel())
	}
}

func TestReadyCharacterChangeRefreshesActualSpriteAndSummary(t *testing.T) {
	state, dialog, held := newReadyReviewFixture(t)
	for index := 2; index < party_state.MaxActiveParty; index++ {
		state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
	}
	second := state.PartyState.GetCharacter(1)
	second.PartyMap = party_state.PartyMapActive
	second.SetStatus(party_state.Good)
	second.SetHp(100)
	second.Class = party_state.Wizard
	second.Strength = 30
	dialog.spriteSheet = &sprites.SpriteSheet{SpriteImage: sprites.NewPngSprite(sprites.TileAtlasPNG())}
	screen := ebiten.NewImageWithOptions(dialog.charBorder.GetBounds(), nil)
	t.Cleanup(screen.Dispose)
	selectReadyReviewItem(t, dialog, references.Halberd)
	for index, label := range []string{"CHAR 1/2", "CHAR 2/2"} {
		if index > 0 {
			pressReadyNavigation(dialog, held, ebiten.KeyTab, false)
		}
		dialog.drawCharacterPanel(screen)
		character := state.PartyState.GetCharacter(index)
		require.Equal(t, character.GetNameAsString(), dialog.titleOutput.GetOutputStr(false))
		require.Equal(t, label, dialog.indicatorOutput.GetOutputStr(false))
		require.Equal(t, int(character.Strength), dialog.getSelectedItem().Preview.Strength)
		raw := dialog.spriteSheet.GetRawSprite(character.GetKeySpriteIndex())
		source := make([]byte, sprites.TileSize*sprites.TileSize*4)
		raw.ReadPixels(source)
		rect := dialog.characterHeaderLayout().portrait
		portrait, ok := screen.SubImage(rect).(*ebiten.Image)
		require.True(t, ok)
		actual := make([]byte, rect.Dx()*rect.Dy()*4)
		portrait.ReadPixels(actual)
		expected := make([]byte, len(actual))
		scale := rect.Dx() / sprites.TileSize
		for y := range rect.Dy() {
			for x := range rect.Dx() {
				from := ((y/scale)*sprites.TileSize + x/scale) * 4
				to := (y*rect.Dx() + x) * 4
				copy(expected[to:to+4], source[from:from+4])
			}
		}
		// Render onto transparency: exact expanded source pixels establish both
		// identity and nearest-neighbour integer scaling, without a snapshot file.
		require.Equal(t, expected, actual)
		screen.Clear()
	}
}

func TestReadyNavigationRespectsCombatAndModalOwnership(t *testing.T) {
	for _, owner := range []string{"combat", "confirmation", "result"} {
		t.Run(owner, func(t *testing.T) {
			_, dialog, held := newReadyReviewFixture(t)
			switch owner {
			case "combat":
				dialog.request.LockCharacter = true
			case "confirmation":
				selectReadyReviewItem(t, dialog, references.Halberd)
				dialog.toggleEquipSelected()
				require.NotNil(t, dialog.confirmation)
			case "result":
				dialog.waitingForResult = true
			}
			for _, shift := range []bool{false, true} {
				for _, key := range []ebiten.Key{ebiten.KeyTab, ebiten.KeyLeft, ebiten.KeyRight, ebiten.Key2} {
					pressReadyNavigation(dialog, held, key, shift)
					require.Zero(t, dialog.selectedCharIdx)
				}
			}
			if owner != "combat" {
				for _, key := range []ebiten.Key{ebiten.KeyBracketLeft, ebiten.KeyBracketRight} {
					pressReadyNavigation(dialog, held, key, false)
					require.Zero(t, dialog.detailPage)
				}
			}
		})
	}
}

func TestReadySpaceAndBracketsPageAllDetailsWithoutChangingCharacterOrEquipping(t *testing.T) {
	_, dialog, held := newReadyReviewFixture(t)
	selectReadyReviewItem(t, dialog, references.Halberd)
	// An actual long refusal exercises wrapping and pagination through Update.
	item := dialog.getSelectedItem()
	item.CanEquip = false
	item.EquipError = strings.Repeat("Equipment is unavailable. ", 30)
	dialog.populateDetailOutput()
	pages := dialog.detailPageCount()
	require.Greater(t, pages, 1)
	dialog.request.LockCharacter = true
	for page := 1; page <= pages; page++ {
		next := ebiten.KeySpace // Space pages; ] remains an alias.
		if page%2 == 0 {
			next = ebiten.KeyBracketRight
		}
		pressReadyNavigation(dialog, held, next, false)
		require.Equal(t, page%pages, dialog.detailPage)
		require.Zero(t, dialog.selectedCharIdx)
		require.Nil(t, dialog.confirmation)
		_, submitted := dialog.TakeTransaction()
		require.False(t, submitted)
	}
	pressReadyNavigation(dialog, held, ebiten.KeyBracketLeft, false)
	require.Equal(t, pages-1, dialog.detailPage)
	pressReadyNavigation(dialog, held, ebiten.KeyTab, false)
	require.Equal(t, pages-1, dialog.detailPage, "locked Tab cannot page details")
	dialog.prepareDetailPage()
	require.Contains(t, dialog.indicatorOutput.GetOutputStr(true), "/")
}

func TestReadyHeaderAndHintsFitSupportedResolutions(t *testing.T) {
	state, dialog, _ := newReadyReviewFixture(t)
	// The count includes dead active members; the selected position is party order.
	state.PartyState.GetCharacter(1).SetStatus(party_state.Dead)
	for _, size := range []image.Point{{1280, 720}, {1366, 768}, {1920, 1080}, {3840, 2160}} {
		setResponsiveDialogTestResolution(t, size.X, size.Y)
		dialog.ensureLayout()
		selectReadyReviewItem(t, dialog, references.Halberd)
		dialog.getSelectedItem().EquipError = strings.Repeat("Long refusal ", 80)
		dialog.getSelectedItem().CanEquip = false
		dialog.populateDetailOutput()
		layout := dialog.characterHeaderLayout()
		dialog.populateCharacterOutput(layout)
		for _, rect := range []image.Rectangle{layout.portrait, layout.name, layout.position, layout.strength} {
			require.True(t, rect.In(dialog.charBorder.GetInteriorBounds()), "%s: %v", size, rect)
		}
		require.False(t, layout.name.Overlaps(layout.position))
		require.False(t, layout.portrait.Overlaps(layout.strength))
		require.Zero(t, layout.portrait.Dx()%16)
		strengthWidth, _ := dialog.font.MeasureString(strings.TrimSpace(dialog.charOutput.GetOutputStr(false)))
		require.LessOrEqual(t, strengthWidth, float64(layout.strength.Dx()))
		for _, locked := range []bool{false, true} {
			dialog.request.LockCharacter = locked
			dialog.populateStatusOutput(dialog.statusBorder.GetInteriorBounds().Dx() - 2*(itemDialogTextPadding+4))
			footer := dialog.statusOutput.GetOutputStr(true)
			require.Contains(t, footer, "[Esc] Close", "keys and their action descriptions must wrap together")
			require.Equal(t, dialog.detailPageCount() > 1, strings.Contains(footer, "[Space] Details"))
			contentWidth := dialog.statusBorder.GetInteriorBounds().Dx() - 2*(itemDialogTextPadding+4)
			for _, line := range strings.Split(footer, "\n") {
				width, _ := dialog.font.MeasureString(strings.TrimSpace(line))
				require.LessOrEqual(t, width, float64(contentWidth), size.String())
			}
			if locked {
				require.NotContains(t, footer, "Tab")
			} else {
				require.Contains(t, footer, "[Tab] Next character")
				require.NotContains(t, footer, "Shift+Tab", "the repeated Previous hint leaves the row")
			}
			require.LessOrEqual(t, dialog.statusOutput.GetTotalHeightForLines(dialog.statusOutput.GetPanelContentLineCount()),
				float64(dialog.statusBorder.GetInteriorBounds().Dy()), size.String())
		}
	}
}
