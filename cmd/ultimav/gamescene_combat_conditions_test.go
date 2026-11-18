package main

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/mainscreen"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCombatCharacterPanelMatchesExplorationThroughConditionsAndHUDDraw(t *testing.T) {
	// Resolution and all-six selection geometry are independently covered by
	// mainscreen's original-layout oracle. This integration test checks that the
	// combat text overlay changes only the affected text through entry/recovery/exit.
	// One resolution and two distinct selections exercise the shared draw path.
	for _, width := range []int{1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			scene := newCombatConcealmentTestState(t)
			state := scene.gameState
			state.TransitionFromCombatMap()
			for index, status := range []party_state.CharacterStatus{
				party_state.Good, party_state.Poisoned, party_state.Sleep, party_state.Charmed, party_state.Dead, party_state.Good,
			} {
				character := state.PartyState.GetCharacter(index)
				clear(character.Name[:])
				copy(character.Name[:], fmt.Sprintf("PC%d", index+1))
				character.Class = party_state.Avatar
				character.Dexterity = 30
				character.Intelligence = 30
				character.SetMaxHp(100)
				character.SetHp(75)
				character.SetMp(30)
				character.SetStatus(status)
				character.PartyMap = party_state.PartyMapActive
			}
			require.Equal(t, 6, state.PartyState.GetActivePartyCount())
			logicalBounds := image.Rect(0, 0, width, width*9/16)
			panel := calculateRightRailLayout(logicalBounds).characterContent
			screen := ebiten.NewImageWithOptions(panel.Inset(-2), &ebiten.NewImageOptions{Unmanaged: true})
			hudBounds := image.Rect(0, 0, width*3/4, 80)
			hudScreen := ebiten.NewImageWithOptions(hudBounds, &ebiten.NewImageOptions{Unmanaged: true})
			t.Cleanup(screen.Deallocate)
			t.Cleanup(hudScreen.Deallocate)
			scene.characterSummary = mainscreen.NewCharacterSummaryForWidth(scene.spriteSheet, width)
			t.Cleanup(scene.characterSummary.Dispose)
			scene.characterSummary.SetBounds(panel)
			_, scene.combatHUD.turnOutput = widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
				Size: combatHUDFontSize, LineSpacingMultiplier: 1, MaxLines: combatHUDOutputLines, MaxChars: 80,
			}, width)
			var baseline [6][]byte
			var characters [6]party_state.PlayerCharacter
			for index := range characters {
				characters[index] = *state.PartyState.GetCharacter(index)
			}
			selections := [...]int{0, 3}
			for _, index := range selections {
				screen.Clear()
				scene.characterSummary.Draw(&state.PartyState, screen, index, false, -1)
				baseline[index] = combatRosterPixels(screen, panel)
			}
			check := func(phase string, possessed bool) {
				t.Helper()
				if state.IsInCombat() {
					hudScreen.Clear()
					lineHeight := int(math.Ceil(scene.combatHUD.turnOutput.GetLineSpacing()))
					scene.drawCombatCurrentTurn(hudScreen, hudBounds, lineHeight, 2)
				}
				for _, active := range selections {
					for _, solo := range []bool{false, true} {
						screen.Clear()
						scene.characterSummary.SetCombatText(combatRosterText(state))
						scene.characterSummary.Draw(&state.PartyState, screen, active, solo, active)
						got := combatRosterPixels(screen, panel)
						rowBytes := panel.Dy() / 6 * panel.Dx() * 4
						if possessed {
							require.False(t, bytes.Equal(baseline[active][:rowBytes], got[:rowBytes]),
								"possession must immediately change the source text")
							portraitWidth := panel.Dy()/6 - 4
							for y := range panel.Dy() / 6 {
								offset := y * panel.Dx() * 4
								require.Equal(t, baseline[active][offset:offset+portraitWidth*4], got[offset:offset+portraitWidth*4],
									"%s changed the persistent portrait", phase)
							}
							require.Equal(t, baseline[active][rowBytes:], got[rowBytes:], "other roster rows must be unchanged")
						} else {
							require.Equal(t, baseline[active], got,
								"%s changed portraits or restored text; selected=%d solo=%v", phase, active, solo)
						}
					}
				}
				for index := range characters {
					assert.Equal(t, characters[index], *state.PartyState.GetCharacter(index), "draw must not mutate persistent status/resources")
				}
			}
			for range 2 { // Re-entering must not retain layout or font changes.
				require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
				state.BeginCombatTurns(game_state.CombatTurnsImmediate)
				check("combat entry", false)
				combat := state.CombatScene
				source := game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
				combat.PartyPossessed[0], combat.PartyUnderground[0], combat.PartySleeping[0] = true, true, true
				combat.CurrentTurnIdentity = source
				check("possessed, underground and sleeping source", true)
				assert.Contains(t, scene.combatHUD.turnOutput.GetOutputStr(false), "PC1", "the party is named even underground")
				assert.NotContains(t, scene.combatHUD.turnOutput.GetOutputStr(false), combatUnknownEntityLabel)
				require.True(t, state.CastCombatInQuasXen(0, source))
				var clone game_state.CombatTurnIdentity
				combat.ForEachPCClone(func(identity game_state.CombatTurnIdentity, _ references.Position) { clone = identity })
				require.Positive(t, clone.Slot)
				combat.PartyPossessed[0], combat.PartyUnderground[0], combat.PartySleeping[0] = false, false, false
				combat.CurrentTurnIdentity = clone
				check("recovered source with affected clone", false)
				assert.Contains(t, scene.combatHUD.turnOutput.GetOutputStr(false), "CLONE", "a clone is a party entry too")
				assert.NotContains(t, scene.combatHUD.turnOutput.GetOutputStr(false), combatUnknownEntityLabel)
				state.TransitionFromCombatMap()
				check("combat exit", false)
			}
		})
	}
}

func combatRosterPixels(screen *ebiten.Image, panel image.Rectangle) []byte {
	pixels := make([]byte, panel.Dx()*panel.Dy()*4)
	screen.SubImage(panel).(*ebiten.Image).ReadPixels(pixels)
	// Only normalize exact nearest-neighbour texel ties in independently
	// allocated portrait outlines. Names/status/resources remain byte-exact.
	// The independent mainscreen baseline test pins these original bounds.
	rowPitch := panel.Dy() / 6
	portraitSize := rowPitch - 4
	for row := range 6 {
		for y := range portraitSize {
			for x := range portraitSize {
				sx, sy := (float64(x)+0.5)*18/float64(portraitSize), (float64(y)+0.5)*18/float64(portraitSize)
				if math.Abs(sx-math.Round(sx)) < 1e-6 || math.Abs(sy-math.Round(sy)) < 1e-6 {
					offset := ((row*rowPitch+2+y)*panel.Dx() + x) * 4
					clear(pixels[offset : offset+4])
				}
			}
		}
	}

	return pixels
}

func TestCombatConditionsDescribeExactCloneAndSourceWithoutRosterMutation(t *testing.T) {
	scene, _ := newCombatUseTestScene(t)
	state := scene.gameState
	combat := state.CombatScene
	source := combat.GetCurrentTurnIdentity()
	character := state.PartyState.GetCharacter(source.EntityIndex)
	character.SetStatus(party_state.Poisoned)
	combat.PartyPossessed[source.EntityIndex] = true
	combat.PartySleeping[source.EntityIndex] = true
	combat.PartyUnderground[source.EntityIndex] = true
	require.True(t, state.CastCombatInQuasXen(source.EntityIndex, source))
	var clone game_state.CombatTurnIdentity
	combat.ForEachPCClone(func(identity game_state.CombatTurnIdentity, _ references.Position) { clone = identity })
	require.Positive(t, clone.Slot)
	combat.PartyPossessed[source.EntityIndex] = false
	combat.PartySleeping[source.EntityIndex] = false
	combat.PartyUnderground[source.EntityIndex] = false
	before := snapshotBlockedCombatState(t, state)
	assert.Equal(t, mainscreen.CombatConditions{}, combatConditions(combat, source))
	assert.Equal(t, mainscreen.CombatConditions{Possessed: true, Sleeping: true, Underground: true}, combatConditions(combat, clone))
	scene.combatHUD = newCombatHUD()
	scene.combatHUD.turnOutput = text.NewOutput(nil, 1, 10, 80)
	scene.appendCombatTurnConditions(source)
	assert.Empty(t, scene.combatHUD.turnOutput.GetOutputStr(false))
	scene.appendCombatTurnConditions(clone)
	assert.Equal(t, "Possessed | Underground | Sleep", scene.combatHUD.turnOutput.GetOutputStr(false))
	assert.Equal(t, party_state.Poisoned, character.GetStatus())
	assert.Equal(t, before, snapshotBlockedCombatState(t, state))
	state.TransitionFromCombatMap()
	assert.Empty(t, combatConditions(state.CombatScene, source))
	assert.Equal(t, party_state.Poisoned, character.GetStatus())
}

func TestCombatPossessionRecoveryClearsOnsetAndLabelImmediately(t *testing.T) {
	scene, _ := newCombatUseTestScene(t)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	state := scene.gameState
	actor := state.CombatScene.GetCurrentTurnIdentity()
	position := state.CombatScene.GetCombatantPosition(actor)
	state.CombatScene.PartyPossessed[actor.EntityIndex] = true
	scene.recordCombatEvent(game_state.CombatEvent{Kind: game_state.CombatEventPossession,
		Target: game_state.CombatFeedbackEntity{IsPartyMember: true, Index: actor.EntityIndex, Position: position},
		From:   references.Position{X: 5, Y: 5}, To: position})
	require.True(t, scene.hasCombatSignatureEffects())
	state.CombatScene.PartyPossessed[actor.EntityIndex] = false
	state.CombatScene.PartySleeping[actor.EntityIndex] = true
	assert.False(t, scene.hasCombatSignatureEffects(), "recovery must not wait for an animation tick")
	assert.Equal(t, mainscreen.CombatConditions{Sleeping: true}, combatConditions(state.CombatScene, actor))
	scene.updateCombatSignatureEffects()
	assert.Empty(t, scene.combatSignatureEffects)
}

func TestCombatPossessedMarkerExactCloneSelectedUnselectedAndHP(t *testing.T) {
	scene, screen := newCombatConcealmentTestScene(t)
	state := scene.gameState
	combat := state.CombatScene
	source := combat.GetCurrentTurnIdentity()
	character := state.PartyState.GetCharacter(source.EntityIndex)
	character.SetStatus(party_state.Poisoned)
	combat.PartyPossessed[source.EntityIndex] = true
	require.True(t, state.CastCombatInQuasXen(source.EntityIndex, source))
	var clone game_state.CombatTurnIdentity
	combat.ForEachPCClone(func(identity game_state.CombatTurnIdentity, _ references.Position) { clone = identity })
	combat.PartyPossessed[source.EntityIndex] = false
	for _, identity := range []game_state.CombatTurnIdentity{source, clone} {
		position := combat.GetCombatantPosition(identity)
		setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
		for _, selected := range []bool{false, true} {
			screen.Clear()
			scene.drawCombatCharacterSprite(screen, character, position, identity, identity, selected, 0, game_state.NoTargetPC)
			options := scene.combatPartyIdentityDrawOptions(position, identity)
			left, top := options.GeoM.Apply(10, 2)
			right, bottom := options.GeoM.Apply(14, 4)
			pixels := screen.SubImage(image.Rect(int(left), int(top), int(right), int(bottom))).(*ebiten.Image)
			count := countExactColor(pixels, ucolor.CombatPossessed)
			if identity == clone {
				assert.Positive(t, count)
			} else {
				assert.Zero(t, count, "source must not inherit its clone's marker")
			}
		}
	}
	assert.Equal(t, ucolor.CombatActivePCOutline, combatActiveSelectionFaceColor(game_state.NoActiveMonster))
	assert.Equal(t, ucolor.EGAYellow, combatActiveSelectionFaceColor(0))
	assert.NotEqual(t, ucolor.CombatPossessed, combatHealthBarColor(int(character.GetCurrentHp()), int(character.GetMaxHp())))
}

func countExactColor(screen *ebiten.Image, color ucolor.EGAColor) int {
	pixels := make([]byte, screen.Bounds().Dx()*screen.Bounds().Dy()*4)
	screen.ReadPixels(pixels)
	count := 0
	for index := 0; index < len(pixels); index += 4 {
		if pixels[index] == color.R && pixels[index+1] == color.G && pixels[index+2] == color.B && pixels[index+3] != 0 {
			count++
		}
	}

	return count
}

func TestCombatInvisibleTurnHUDDoesNotDiscloseNewIdentity(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	index, monster := signatureMonster(t, scene, references.MonsterShadowlord, references.Position{X: 5, Y: 5})
	monster.SetInvisible(true)
	identity := game_state.CombatTurnIdentity{EntityIndex: index}
	scene.combatHUD.detail = true
	assert.True(t, scene.combatTurnIdentityConcealed(identity))
	assert.Nil(t, scene.combatTurnSprite(identity))
	assert.Equal(t, combatUnknownEntityLabel, scene.combatEntityEventName(identity))
	assert.Empty(t, scene.combatCurrentTurnDetail(identity, game_state.CombatTurnForecastEntry{Dexterity: 30}, true))
	scene.combatHUD.forecast[0] = game_state.CombatTurnForecastEntry{CombatTurnIdentity: identity}
	scene.gameConfig.GameplayConfiguration.ShowConcealedTurnOrderSlots = false
	assert.Zero(t, scene.filterConcealedCombatForecast(1))
	scene.gameConfig.GameplayConfiguration.ShowConcealedTurnOrderSlots = true
	assert.Equal(t, 1, scene.filterConcealedCombatForecast(1))
	assert.Nil(t, scene.combatTurnSprite(identity), "optional slot disclosure must still not reveal the sprite")
}

func TestCombatReportIdentitySurvivesSlotReuseAndDoesNotLeakInvisibleArrival(t *testing.T) {
	scene := newCombatReportTestScene(t)
	index, monster := signatureMonster(t, scene, references.MonsterDevil, references.Position{X: 5, Y: 5})
	event := game_state.CombatEvent{Kind: game_state.CombatEventRetired,
		Attacker: game_state.CombatFeedbackEntity{Index: index, Position: monster.Position()}}
	scene.gameState.SystemCallbacks.Combat.PublishEvent(event)
	slot := (scene.combatHUD.historyStart + scene.combatHUD.historyCount - 1) % combatEventHistoryCapacity
	replacement := game_state.NewCombatMonster(references.MonsterRat, &(*scene.gameState.GameReferences.EnemyReferences)[references.MonsterRat],
		monster.Position(), scene.gameState.AsRandomProvider())
	scene.gameState.CombatScene.ReplaceCombatantWithMonster(game_state.CombatTurnIdentity{EntityIndex: index}, replacement)
	// Compact the retired final slot, as the manager's removal API permits.
	scene.gameState.CombatScene.MonsterManager.RemoveMonster(index)
	scene.combatHUD.eventOutput.Reset()
	scene.withCombatHistoryEvent(slot, scene.appendCombatEvent)
	// The narrow report column may wrap the sentence.
	assert.Contains(t, strings.Join(strings.Fields(scene.combatHUD.eventOutput.GetOutputStr(false)), " "),
		references.MonsterDevil.Name()+" disappears")
	assert.NotContains(t, scene.combatHUD.eventOutput.GetOutputStr(false), "Rat")
	replacement.SetInvisible(true)
	scene.gameState.SystemCallbacks.Combat.PublishEvent(event)
	last := (scene.combatHUD.historyStart + scene.combatHUD.historyCount - 1) % combatEventHistoryCapacity
	assert.Equal(t, combatUnknownEntityLabel, scene.combatHUD.identities[last].Attacker.Name)
}
