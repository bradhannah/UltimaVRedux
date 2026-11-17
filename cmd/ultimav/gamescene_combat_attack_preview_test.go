package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCombatAttackBracketsFullOutlinedPixelGeometry(t *testing.T) {
	bluePixel := func(x, y int) bool {
		return (x == 5 || x == 11) && y >= 9 && y <= 10 ||
			y == 10 && (x >= 5 && x <= 6 || x >= 10 && x <= 11)
	}
	assert.Equal(t, ucolor.EGALightBlue, ucolor.CombatAttackEligibleFace)
	assert.Equal(t, ucolor.EGABlack, ucolor.CombatAttackEligibleBorder)
	for _, scale := range []int{1, 2, 4} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			screen := ebiten.NewImage(24*scale, 24*scale)
			t.Cleanup(screen.Dispose)
			background := ucolor.EGALightGreen.ToRGBA()
			screen.Fill(background)
			options := &ebiten.DrawImageOptions{}
			options.GeoM.Scale(float64(scale), float64(scale))
			options.GeoM.Translate(float64(3*scale), float64(2*scale))
			drawCombatAttackBrackets(screen, options)
			pixels := make([]byte, 24*scale*24*scale*4)
			screen.ReadPixels(pixels)
			for y := range 24 * scale {
				for x := range 24 * scale {
					sx, sy := x/scale-3, y/scale-2
					want := background
					for dy := -1; dy <= 1; dy++ {
						for dx := -1; dx <= 1; dx++ {
							if bluePixel(sx+dx, sy+dy) {
								want = ucolor.EGABlack.ToRGBA()
							}
						}
					}
					if bluePixel(sx, sy) {
						want = ucolor.EGALightBlue.ToRGBA()
					}
					i := (y*24*scale + x) * 4
					got := color.RGBA{R: pixels[i], G: pixels[i+1], B: pixels[i+2], A: pixels[i+3]}
					require.Equal(t, want, got, "pixel (%d,%d), scale %d", sx, sy, scale)
				}
			}
		})
	}
}

func TestCombatAttackBracketsAimedTargetSuppressionAndExit(t *testing.T) {
	scene, _ := setupCombatAutoAttackInputTest(t)
	scene.mapImage = ebiten.NewImage(304, 208)
	screen := newPresentationRaster(1280, 720)
	t.Cleanup(screen.Dispose)
	t.Cleanup(scene.mapImage.Dispose)
	pixels := make([]byte, 1280*720*4)
	facePixels := func() int {
		screen.Fill(ucolor.EGABlack)
		scene.drawCombatAttackEligibility(screen)
		screen.ReadPixels(pixels)
		face := ucolor.CombatAttackEligibleFace.ToRGBA()
		count := 0
		for i := 0; i < len(pixels); i += 4 {
			if pixels[i] == face.R && pixels[i+1] == face.G && pixels[i+2] == face.B && pixels[i+3] == face.A {
				count++
			}
		}
		return count
	}
	before := facePixels()
	require.Positive(t, before)
	scene.gameState.ElapsedMs += 500
	assert.Equal(t, before, facePixels(), "marker does not pulse with game time")
	require.True(t, scene.gameState.BeginCombatAttack())
	scene.secondaryKeyState = CombatAttackTargetInput
	scene.gameState.CombatScene.AttackTargeting.Cursor = scene.gameState.CombatScene.MonsterManager.GetMonster(0).Position()
	assert.Zero(t, facePixels(), "selected reticle owns the aimed target's square")
	scene.finishCombatAttackTargetAction(scene.gameState.CancelInitialCombatAttack())
	assert.Equal(t, before, facePixels(), "cancelling Aim restores normal any-source eligibility")
	scene.gameState.CombatScene = nil
	assert.Zero(t, facePixels(), "combat exit has no cached markers")
}

func TestCombatAttackPreviewPresentationRefreshAndCleanup(t *testing.T) {
	scene, actor := setupCombatAutoAttackInputTest(t)
	state := scene.gameState
	character := state.PartyState.GetCharacter(actor.EntityIndex)
	character.SetEquippedItem(references.SlotWeapon, references.Bow)
	character.SetEquippedItem(references.SlotShield, references.NoEquipment)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 2)
	rngCalls := state.GetRNG().Calls()
	preview := scene.combatAttackPreview()
	require.Len(t, preview.Targets, 1)
	assert.Nil(t, preview.Aim)
	require.True(t, state.BeginCombatAttack())
	scene.secondaryKeyState = CombatAttackTargetInput
	state.CombatScene.AttackTargeting.Cursor = preview.Targets[0].Position
	assert.NotNil(t, scene.combatAttackPreview().Aim)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 0)
	assert.Empty(t, scene.combatAttackPreview().Targets)
	assert.Equal(t, game_state.CombatShotNoAmmo, scene.combatAttackPreview().Aim.Status)
	scene.finishCombatAttackTargetAction(state.CancelInitialCombatAttack())
	assert.Nil(t, scene.combatAttackPreview().Aim)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 2)
	assert.Len(t, scene.combatAttackPreview().Targets, 1)
	scene.inputBlocked = true
	assert.Empty(t, scene.combatAttackPreview())
	scene.inputBlocked = false
	scene.secondaryKeyState = CombatSpellTargetInput
	assert.Empty(t, scene.combatAttackPreview())
	scene.secondaryKeyState = PrimaryInput
	state.CombatScene.MonsterManager.GetMonster(0).SetInvisible(true)
	assert.Empty(t, scene.combatAttackPreview().Targets)
	state.CombatScene = nil
	assert.Empty(t, scene.combatAttackPreview())
	assert.Equal(t, rngCalls, state.GetRNG().Calls())
}

func TestCombatAttackReadoutNamesRangeAndUnavailableSquare(t *testing.T) {
	rows := appendCombatAttackReadoutRows(nil, game_state.CombatAttackPreview{
		Sources: []game_state.CombatAttackSource{
			{Equipment: references.SpikedHelm, MaxRange: 1},
			{Equipment: references.TwoHHammer, MaxRange: 1},
		},
	})
	assert.Equal(t, []combatReadoutRow{{text: "Spiked Helm", value: "R1"}, {text: "2H Hammer", value: "R1"}}, rows)

	rows = appendCombatAttackReadoutRows(nil, game_state.CombatAttackPreview{
		Sources: []game_state.CombatAttackSource{{Equipment: references.NoEquipment, MaxRange: 1}},
		Aim:     &game_state.CombatAttackTargetPreview{Distance: 3, Status: game_state.CombatShotOutOfRange},
	})
	assert.Equal(t, []combatReadoutRow{
		{text: "Bare Hands", value: "R1"},
		{text: "No one seen", value: "D3"}, // An unnamed square never reveals its occupant.
		{label: "Attack: ", text: "Too far"},
	}, rows)

	rows = appendCombatAttackReadoutRows(nil, game_state.CombatAttackPreview{
		Aim: &game_state.CombatAttackTargetPreview{Name: "Troll", Distance: 6, Status: game_state.CombatShotClear,
			Actor: &game_state.CombatActorPresentation{Loyalty: game_state.CombatMonsterLoyaltyMonster}},
	})
	assert.Equal(t, []combatReadoutRow{{text: "Troll", value: "D6"}, {label: "Enemy: ", text: "Clear"}}, rows)
	assert.Equal(t, "Bypass", combatShotStatusText(game_state.CombatShotTerrainBypass))
	assert.Equal(t, "Bypasses terrain on hit", string(game_state.CombatShotTerrainBypass),
		"presentation wording must not change the authoritative status")
}

// Every weapon, target and verdict takes exactly one row of the Turn Order
// column. A name that is too long is cut plainly.
func TestCombatAttackReadoutRowsFitOneRowAtSupportedResolutions(t *testing.T) {
	for _, resolution := range []image.Point{{1280, 720}, {1366, 768}, {1920, 1080}, {2560, 1440}, {3840, 2160}} {
		t.Run(fmt.Sprintf("%dx%d", resolution.X, resolution.Y), func(t *testing.T) {
			screen := image.Rectangle{Max: resolution}
			turns, _ := combatLayout{}.turnAndReportRectangles(screen)
			width := combatHUDContentBounds(screen, turns).Dx()
			font := text.NewUltimaFont(float64(combatHUDFontSize) * float64(resolution.X) / 1920)
			var rows []combatReadoutRow
			for item := references.LeatherHelm; item <= references.MysticSword; item++ {
				if item.GetAttackValue() > 0 {
					rows = append(rows, combatReadoutRow{text: combatReadoutEquipmentName(item), value: "R15"})
				}
			}
			for monster := references.MonsterType(0); monster < references.TotalMonsterTypes; monster++ {
				rows = append(rows, combatReadoutRow{text: monster.Name(), value: "D15"})
			}
			for _, status := range []game_state.CombatShotStatus{
				game_state.CombatShotClear, game_state.CombatShotTerrainBypass, game_state.CombatShotOutOfRange,
				game_state.CombatShotBlocked, game_state.CombatShotNoAmmo, game_state.CombatShotInterference,
				game_state.CombatShotUnready, game_state.CombatShotChooseTarget, game_state.CombatShotFriendly,
			} {
				for _, label := range []string{"Attack: ", "Enemy: ", "Ally: "} {
					rows = append(rows, combatReadoutRow{label: label, text: combatShotStatusText(status)})
				}
			}
			for _, row := range rows {
				fitted := combatReadoutRowText(font, row, width)
				whole := row.label + fitted
				if row.value != "" {
					whole += " " + row.value
				}
				assert.LessOrEqual(t, measuredTextWidth(font, whole), float64(width), whole)
				assert.True(t, strings.HasPrefix(row.text, fitted), "%q is a plain cut", fitted)
				if row.label != "" {
					assert.Equal(t, row.text, fitted, "every verdict is shown whole")
				}
			}
			for _, whole := range []string{"Morning Star", "2H Hammer"} {
				assert.Equal(t, whole, combatReadoutRowText(font, combatReadoutRow{text: whole, value: "R2"}, width))
			}
		})
	}
}

func measuredTextWidth(font *text.UltimaFont, value string) float64 {
	width, _ := font.MeasureString(value)

	return width
}

func TestCombatCurrentNameAndLadderRowAreFull(t *testing.T) {
	scene, actor := setupCombatAutoAttackInputTest(t)
	character := scene.gameState.PartyState.GetCharacter(actor.EntityIndex)
	clear(character.Name[:])
	copy(character.Name[:], "Shamino")
	scene.spriteSheet = &sprites.SpriteSheet{SpriteImage: ebiten.NewImage(512, 256)}
	scene.mapImage = ebiten.NewImage(304, 208)
	scene.combatHUD = newCombatHUD()
	font := text.NewUltimaFont(float64(combatHUDFontSize) * 1280 / 1920)
	scene.combatHUD.turnOutput = text.NewOutput(font, float64(combatHUDFontSize)*1280/1920, combatHUDOutputLines, 80)
	screen := newPresentationRaster(1280, 720)
	t.Cleanup(screen.Dispose)
	for _, clone := range []bool{false, true} {
		identity := actor
		want := "SHAMINO"
		if clone {
			scene.gameState.CombatScene.CurrentTurnIdentity = actor
			require.True(t, scene.gameState.CastCombatInQuasXen(actor.EntityIndex, actor))
			scene.gameState.CombatScene.ForEachPCClone(func(exact game_state.CombatTurnIdentity, _ references.Position) { identity = exact })
			want += " CLONE"
		}
		scene.gameState.CombatScene.CurrentTurnIdentity = identity
		for _, detail := range []bool{false, true} {
			scene.combatHUD.detail = detail
			for _, autoPass := range []bool{false, true} {
				scene.gameState.CombatScene.SoloMoveMode = autoPass
				scene.gameState.CombatScene.SoloMoveCharacterIndex = (actor.EntityIndex + 1) % 6
				lineHeight := int(math.Ceil(scene.combatHUD.turnOutput.GetLineSpacing()))
				bottom := scene.drawCombatCurrentTurn(screen, image.Rect(622, 22, 947, 338), lineHeight, lineHeight/3)
				content := scene.combatHUD.turnOutput.GetOutputStr(false)
				assert.Contains(t, strings.ReplaceAll(content, "\n", " "), want)
				assert.GreaterOrEqual(t, bottom, 22+lineHeight+lineHeight/3+int(math.Ceil(scene.combatHUD.turnOutput.GetTotalHeightForLines(scene.combatHUD.turnOutput.GetPanelContentLineCount()))))
				scene.drawCombatLadderRow(screen, image.Rect(622, 200, 947, 240),
					game_state.CombatTurnForecastEntry{CombatTurnIdentity: identity, CanTakeTurn: true}, lineHeight)
				wantNext := "Shamino"
				if clone {
					wantNext = "Shamino Clone 1"
				}
				row := strings.TrimSpace(scene.combatHUD.turnOutput.GetOutputStr(false))
				if autoPass && clone {
					// The reason a turn is lost shares the row; the longest name then shortens.
					assert.True(t, strings.HasPrefix(row, "Shamino C"), row)
				} else {
					assert.Equal(t, wantNext, row, "a ladder row has room for the whole name")
				}
			}
		}
	}
}
