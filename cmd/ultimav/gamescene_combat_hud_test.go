package main

import (
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestCombatPartySpriteIndexUsesOriginalInvisiblePlayerTile(t *testing.T) {
	character := &party_state.PlayerCharacter{}

	assert.Equal(t, indexes.SpriteIndex(indexes.InvisiblePlayer), combatPartySpriteIndex(character, true, false))
	assert.Equal(t, indexes.RatKeyIndex, combatPartySpriteIndex(character, false, true))
}

func TestCombatConditionPlainTextExplainsQuicknessSuppression(t *testing.T) {
	assert.Equal(t,
		"Orc loses its action to quickness",
		combatConditionPlainText(game_state.CombatConditionQuicknessSuppressed, "Orc"),
	)
}

func TestCombatEventAudioRequestsOneResultCueAndSkipsAreaEvents(t *testing.T) {
	gs, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).WithSystemCallbacks().Build()
	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
	scene := &GameScene{gameState: gs}
	cases := []struct {
		kind   game_state.CombatEventKind
		result game_state.CombatFeedbackResult
		want   *game_state.SoundEffect
	}{
		{game_state.CombatEventAttack, game_state.CombatFeedbackMiss, soundEffectPointer(game_state.SoundCombatMiss)},
		{game_state.CombatEventAttack, game_state.CombatFeedbackGraze, soundEffectPointer(game_state.SoundCombatGraze)},
		{game_state.CombatEventAttack, game_state.CombatFeedbackDamage, soundEffectPointer(game_state.SoundCombatHit)},
		{game_state.CombatEventRangedAttack, game_state.CombatFeedbackDamage, soundEffectPointer(game_state.SoundCombatHit)},
		{game_state.CombatEventEquipmentShattered, 0, soundEffectPointer(game_state.SoundCombatShatter)},
		{game_state.CombatEventFled, 0, soundEffectPointer(game_state.SoundCombatFlee)},
		{game_state.CombatEventVictory, 0, soundEffectPointer(game_state.SoundCombatVictory)},
		{game_state.CombatEventFieldTriggered, 0, nil},
		{game_state.CombatEventSpell, 0, nil},
	}
	for _, test := range cases {
		before := len(callbacks.SoundEffectsPlayed)
		scene.recordCombatEvent(game_state.CombatEvent{
			Kind: test.kind, Result: test.result,
		})
		if test.want == nil {
			require.Len(t, callbacks.SoundEffectsPlayed, before, "kind %d", test.kind)
			continue
		}
		require.Equal(t, []game_state.SoundEffect{*test.want}, callbacks.SoundEffectsPlayed[before:], "kind %d", test.kind)
	}
}

func soundEffectPointer(effect game_state.SoundEffect) *game_state.SoundEffect { return &effect }

func TestCombatUtilitySpellPlainTextDescribesPersistentMutation(t *testing.T) {
	event := game_state.CombatEvent{
		Kind:          game_state.CombatEventUtilitySpell,
		UtilityResult: game_state.CombatUtilityDoorMagicLocked,
		Diagnostics:   game_state.CombatDiagnostics{Spell: references.AnExPor},
	}

	assert.Equal(t, "Avatar casts An Ex Por and seals the door", combatUtilitySpellPlainText(event, "Avatar"))
}

func TestCombatHUDFormatsMonsterVisibilityAndTeleportEvents(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)
	gs.CombatScene.MonsterManager.Clear()
	monsterRef := &(*gs.GameReferences.EnemyReferences)[references.MonsterShadowlord]
	monster := game_state.NewCombatMonster(
		references.MonsterShadowlord, monsterRef, references.Position{X: 2, Y: 2}, gs.AsRandomProvider(),
	)
	monsterIndex := gs.CombatScene.AddMonsterToQueue(monster)
	scene := &GameScene{gameState: gs}
	event := game_state.CombatEvent{
		Kind:       game_state.CombatEventVisibility,
		Attacker:   game_state.CombatFeedbackEntity{Index: monsterIndex},
		Visibility: game_state.CombatVisibilityHidden,
	}

	assert.Equal(t, "Shadowlord disappears", scene.combatEventPlainText(event))
	event.Visibility = game_state.CombatVisibilityRevealed
	assert.Equal(t, "Shadowlord reappears", scene.combatEventPlainText(event))
	event.Kind = game_state.CombatEventTeleport
	event.From = references.Position{X: 2, Y: 2}
	event.To = references.Position{X: 8, Y: 7}
	assert.Equal(t, "Shadowlord teleports", scene.combatEventPlainText(event))
	assert.Equal(t, "From (2,2) to (8,7)", scene.combatEventDetailText(event))
	event.Kind = game_state.CombatEventVanished
	event.Vanished = game_state.CombatVanishedMonster{
		CombatantID: monster.CombatantID(), Index: monsterIndex,
		MonsterType: references.MonsterShadowlord, Position: references.Position{X: 2, Y: 2},
	}
	assert.Equal(t, "Shadowlord vanishes", scene.combatEventPlainText(event))
	assert.Equal(t, "Vanished at (2,2)", scene.combatEventDetailText(event))
}

func TestCombatUtilityEffectGeometryDistinguishesBindingAndRelease(t *testing.T) {
	assert.Less(t,
		combatUtilityEffectInset(references.AnExPor, combatUtilityApproach, 0),
		combatUtilityEffectInset(references.AnExPor, combatUtilityApproach, 1),
		"magical lock corners contract inward",
	)
	assert.Greater(t,
		combatUtilityEffectInset(references.InExPor, combatUtilityApproach, 0),
		combatUtilityEffectInset(references.InExPor, combatUtilityApproach, 1),
		"magical unlock corners expand outward",
	)
	assert.Equal(t, ucolor.EGAMagenta, combatUtilityEffectColor(references.AnExPor, combatUtilityApproach))
	assert.Equal(t, ucolor.EGALightCyan, combatUtilityEffectColor(references.InExPor, combatUtilityApproach))
}

// TestCombatPanelsMirrorThePartyColumn keeps the left panel column exactly as
// wide as the party column, with the battlefield centred between them.
func TestCombatPanelsMirrorThePartyColumn(t *testing.T) {
	screenBounds := image.Rect(0, 0, 1280, 720)
	battlefield := combatLayout{}.battlefieldRectangle(screenBounds)
	panels := combatLayout{}.panelRectangle(screenBounds)
	party := calculateRightRailLayout(screenBounds).characterBorder

	assert.Equal(t, image.Rect(358, 55, 922, 665), battlefield)
	assert.Equal(t, image.Rect(0, 0, 319, 720), panels)
	assert.Equal(t, party.Dx(), panels.Dx(), "both columns are the same width")
	assert.Equal(t, party.Min.X-battlefield.Max.X, battlefield.Min.X-panels.Max.X, "the battlefield is centred")
	turns, events := combatLayout{}.turnAndReportRectangles(screenBounds)
	assert.Equal(t, party.Max.Y, turns.Max.Y, "Turn Order ends level with the party roster")
	assert.Equal(t, panels.Max.Y, events.Max.Y)
	assert.Equal(t, 11, turns.Max.Y-events.Min.Y, "panes use the right-rail border overlap")
	assert.Equal(t, image.Rect(24, 22, 293, 338), combatHUDContentBounds(screenBounds, turns))
	assert.Equal(t, image.Rect(24, 371, 293, 698), combatHUDContentBounds(screenBounds, events))
}

// Turn Order only gives the ladder the whole column; Off removes the column and
// centres the battlefield.
func TestCombatPanelsChoiceMovesOnlyWhatItNames(t *testing.T) {
	screenBounds := image.Rect(0, 0, 1280, 720)
	rail := combatLayout{panels: config.CombatPanelsTurnOrderOnly}
	turns, events := rail.turnAndReportRectangles(screenBounds)
	assert.Equal(t, combatLayout{}.panelRectangle(screenBounds), turns)
	assert.True(t, events.Empty())
	assert.Equal(t, combatLayout{}.battlefieldRectangle(screenBounds), rail.battlefieldRectangle(screenBounds))

	off := combatLayout{panels: config.CombatPanelsOff}
	assert.True(t, off.panelRectangle(screenBounds).Empty())
	battlefield := off.battlefieldRectangle(screenBounds)
	viewportRight := combatViewportSourceX(screenBounds, combatViewportWidthSourcePixels)
	assert.InDelta(t, battlefield.Min.X, viewportRight-battlefield.Max.X, 1, "the battlefield is centred")
}

func TestCombatHUDDetailDefaultsOff(t *testing.T) {
	assert.False(t, newCombatHUD().detail)
}

func TestCurrentCombatTurnIdentityIsHiddenBeforeTurnsStart(t *testing.T) {
	combat := &game_state.CombatState{}

	_, valid := currentCombatTurnIdentity(combat)

	assert.False(t, valid)
}

func TestCurrentCombatTurnIdentityIsVisibleAfterGameStateStartsTurns(t *testing.T) {
	state, _ := newBritainCombatTestState(t, 42)
	state.BeginCombatTurns(game_state.CombatTurnsImmediate)

	identity, valid := currentCombatTurnIdentity(state.CombatScene)

	require.True(t, valid)
	assert.Equal(t, state.CombatScene.GetCurrentTurnIdentity(), identity)
}

func TestCombatDetailHeaderSuffixHiddenWhenDisabled(t *testing.T) {
	assert.Empty(t, combatDetailHeaderSuffix(false))
	assert.Equal(t, " DTL:ON", combatDetailHeaderSuffix(true))
}

func TestCombatTurnContentFitsSupportedResolutions(t *testing.T) {
	resolutions := []image.Point{
		{X: 1280, Y: 720},
		{X: 1366, Y: 768},
		{X: 1920, Y: 1080},
		{X: 2560, Y: 1440},
		{X: 3840, Y: 2160},
	}
	for _, resolution := range resolutions {
		t.Run(fmt.Sprintf("%dx%d", resolution.X, resolution.Y), func(t *testing.T) {
			screenBounds := image.Rectangle{Max: resolution}
			hudBounds := combatLayout{}.panelRectangle(screenBounds)
			turnBounds, eventBounds := combatHUDStackedRectangles(screenBounds, hudBounds)
			contentBounds := combatHUDContentBounds(screenBounds, turnBounds)
			lineHeight := int(math.Ceil(float64(combatHUDFontSize) * float64(resolution.X) / 1920))
			lineGap := lineHeight / combatHUDLineGapDivisor
			spriteHeight := int(math.Round(18 * float64(resolution.Y) / 208))
			ladderRow := max(spriteHeight/2, lineHeight) + lineGap
			requiredHeight := lineHeight + lineGap + spriteHeight + 2*lineHeight + 2*lineGap + 4*ladderRow

			assert.Equal(t, 0, hudBounds.Min.Y)
			assert.Equal(t, resolution.Y, hudBounds.Max.Y)
			assert.Equal(t, int(math.Round(float64(resolution.Y)*combatHUDBorderOverlapY)),
				turnBounds.Max.Y-eventBounds.Min.Y)
			assert.LessOrEqual(t, requiredHeight, contentBounds.Dy(),
				"turn title, current actor, two weapon rows and four ladder rows must clear both borders")
		})
	}
}

func TestCombatEventLineCapacityFitsCompleteGlyphHeight(t *testing.T) {
	_, output := widgets.GetScaledFontWithOutput(widgets.FontConfig{
		Size:                  combatHUDFontSize,
		LineSpacingMultiplier: 1,
		MaxLines:              combatHUDOutputLines,
		MaxChars:              combatHUDOutputChars,
	})
	screenBounds := image.Rect(0, 0, 1280, 720)
	_, eventBounds := combatLayout{}.turnAndReportRectangles(screenBounds)
	contentBounds := combatHUDContentBounds(screenBounds, eventBounds)
	eventHeight := contentBounds.Dy()
	capacity := output.GetLineCapacity(eventHeight)
	maxChars := output.Font.CharsForWidth(contentBounds.Dx())
	assert.LessOrEqual(t, output.GetTotalHeightForLines(capacity), float64(eventHeight))
	assert.Greater(t, output.GetTotalHeightForLines(capacity+1), float64(eventHeight))
	assert.Greater(t, maxChars, maxCharsPerLine, "combat event pane is intentionally wider than the original message pane")
	t.Logf("combat event pane fits approximately %d conservative characters", maxChars)
}

func TestCombatHUDStartsEachEncounterWithBattleMessage(t *testing.T) {
	hud := newCombatHUD()
	combat := &game_state.CombatState{}

	hud.sync(combat)
	require.Equal(t, 1, hud.historyCount)
	assert.Equal(t, game_state.CombatEventBattleBegun, hud.history[0].Kind)
	hud.sync(combat)
	assert.Equal(t, 1, hud.historyCount, "repeated sync must not duplicate the opening message")
}

func TestCombatPositionMapsToViewportTiles(t *testing.T) {
	tileX, tileY := combatPositionToViewportTile(references.Position{X: 0, Y: 0})
	assert.Equal(t, 0, tileX)
	assert.Equal(t, 1, tileY)
	tileX, tileY = combatPositionToViewportTile(references.Position{X: 10, Y: 10})
	assert.Equal(t, 10, tileX)
	assert.Equal(t, 11, tileY)
}

func TestCombatPositionLeavesSourcePixelGutter(t *testing.T) {
	leftX, _ := combatLayout{}.positionToSourcePixels(references.Position{X: 0, Y: 0})
	rightX, _ := combatLayout{}.positionToSourcePixels(references.Position{X: 10, Y: 0})

	assert.Equal(t, float64(combatLayout{}.battlefieldLeft()-1), leftX)
	assert.Equal(t, float64(combatLayout{}.battlefieldLeft()+10*sprites.TileSize-1), rightX)
}

func TestCombatMoveDirectionFormatsCardinalMovement(t *testing.T) {
	event := game_state.CombatEvent{
		From: references.Position{X: 5, Y: 5},
		To:   references.Position{X: 4, Y: 5},
	}
	assert.Equal(t, "WEST", combatMoveDirection(event))
	event.To = references.Position{X: 5, Y: 6}
	assert.Equal(t, "SOUTH", combatMoveDirection(event))
}

func TestCombatHUDSpriteSizeUsesMapScale(t *testing.T) {
	screen := newPresentationRaster(1280, 720)
	scene := &GameScene{mapImage: ebiten.NewImage(304, 208)}
	sprite := ebiten.NewImage(18, 18)

	width, height := scene.combatHUDSpriteSize(screen, sprite)
	scaleX, scaleY := scene.combatHUDSpriteScale(screen)

	assert.Equal(t, 58, width)
	assert.Equal(t, 62, height)
	assert.InDelta(t, 1280*0.76/304, scaleX, 0.0001)
	assert.InDelta(t, 720.0/208, scaleY, 0.0001)
}

func TestCombatHUDEventHistoryRetainsNewestEvents(t *testing.T) {
	hud := newCombatHUD()

	for index := 0; index < combatEventHistoryCapacity+3; index++ {
		hud.addEvent(game_state.CombatEvent{HPLost: index})
	}

	require.Equal(t, combatEventHistoryCapacity, hud.historyCount)
	assert.Equal(t, 3, hud.history[hud.historyStart].HPLost)
	newestIndex := (hud.historyStart + hud.historyCount - 1) % len(hud.history)
	assert.Equal(t, combatEventHistoryCapacity+2, hud.history[newestIndex].HPLost)
}

func TestFormatCombatDiagnosticsShowsOnlyConsumedRolls(t *testing.T) {
	miss := game_state.CombatDiagnostics{
		Hit: game_state.CombatHitResult{
			Outcome:   game_state.CombatHitMiss,
			Threshold: 14,
			D30Roll:   7,
			Rolled:    true,
		},
	}
	assert.Equal(t, "Roll 7 < 14", formatCombatDiagnostics(miss))

	hit := game_state.CombatDiagnostics{
		Hit: game_state.CombatHitResult{
			Outcome:   game_state.CombatHitNormal,
			Threshold: 12,
			D30Roll:   18,
			Rolled:    true,
		},
		Damage: game_state.CombatDamageResult{
			AttackRoll:      9,
			AttackRolled:    true,
			ArmorRoll:       2,
			ArmorRolled:     true,
			PostArmorDamage: 7,
		},
		FinalDamage: 7,
	}
	assert.Equal(t, "Roll 18 >= 12; Damage 9 - armor 2 = 7", formatCombatDiagnostics(hit))

	overriddenDex := hit
	overriddenDex.TargetBaseDex = 20
	overriddenDex.Hit.DefenderEffectiveDex = 1
	assert.Equal(t,
		"Roll 18 >= 12; Target DEX 20 -> 1; Damage 9 - armor 2 = 7",
		formatCombatDiagnostics(overriddenDex),
	)

	poison := game_state.CombatDiagnostics{
		PoisonRoll:           1,
		PoisonRollMaximum:    3,
		PoisonRolled:         true,
		PoisonFallbackDamage: 19,
		PoisonFallbackRolled: true,
	}
	assert.Equal(t, "Poison roll 1 of 3; Poison fallback damage 19", formatCombatDiagnostics(poison))

	theft := game_state.CombatDiagnostics{
		TheftRoll:        3,
		TheftRollMaximum: 3,
		TheftRolled:      true,
		FoodStolen:       5,
	}
	assert.Equal(t, "Theft roll 3 of 3; Food stolen 5", formatCombatDiagnostics(theft))

	bareHands := game_state.CombatDiagnostics{
		Damage: game_state.CombatDamageResult{
			AttackRoll:      1,
			PreArmorDamage:  1,
			PostArmorDamage: 1,
		},
		FinalDamage: 1,
	}
	assert.Equal(t, "Damage 1", formatCombatDiagnostics(bareHands))

	failedPoisonThenHit := hit
	failedPoisonThenHit.PoisonRolled = true
	failedPoisonThenHit.PoisonRoll = 0
	failedPoisonThenHit.PoisonRollMaximum = 3
	assert.Equal(t,
		"Roll 18 >= 12; Damage 9 - armor 2 = 7; Poison roll 0 of 3",
		formatCombatDiagnostics(failedPoisonThenHit),
	)
}

func TestCombatAttackPlainTextUsesPlainWords(t *testing.T) {
	miss := game_state.CombatEvent{Result: game_state.CombatFeedbackMiss}
	assert.Equal(t, "Squid misses Shamino", combatAttackPlainText(miss, "Squid", "Shamino"))

	graze := game_state.CombatEvent{Result: game_state.CombatFeedbackGraze}
	assert.Equal(t, "Shamino grazes Squid", combatAttackPlainText(graze, "Shamino", "Squid"))

	poison := game_state.CombatEvent{Result: game_state.CombatFeedbackPoison}
	assert.Equal(t, "Squid poisons Shamino", combatAttackPlainText(poison, "Squid", "Shamino"))

	sleep := game_state.CombatEvent{Result: game_state.CombatFeedbackSleep}
	assert.Equal(t, "Gazer hits Shamino: asleep", combatAttackPlainText(sleep, "Gazer", "Shamino"))

	theft := game_state.CombatEvent{
		Result:      game_state.CombatFeedbackTheft,
		Diagnostics: game_state.CombatDiagnostics{FoodStolen: 5},
	}
	assert.Equal(t, "Gremlin steals 5 food", combatAttackPlainText(theft, "Gremlin", "Shamino"))

	damage := game_state.CombatEvent{Result: game_state.CombatFeedbackDamage, HPLost: 7}
	assert.Equal(t, "Avatar hits Orc for 7", combatAttackPlainText(damage, "Avatar", "Orc"))

	slain := game_state.CombatEvent{Result: game_state.CombatFeedbackDamage, HPLost: 12, Fatal: true}
	assert.Equal(t, "Avatar slays Orc", combatAttackPlainText(slain, "Avatar", "Orc"))

	fallen := slain
	fallen.Target.IsPartyMember = true
	assert.Equal(t, "Orc hits Julia for 12. Julia falls!", combatAttackPlainText(fallen, "Orc", "Julia"))
}

func TestCombatConditionPlainTextDescribesSkippedTurn(t *testing.T) {
	assert.Equal(t, "Avatar sleeps through the turn",
		combatConditionPlainText(game_state.CombatConditionSleeping, "Avatar"))
	assert.Equal(t, "Avatar wakes up",
		combatConditionPlainText(game_state.CombatConditionWoke, "Avatar"))
}

func TestCombatDivisionPlainTextReportsSplit(t *testing.T) {
	assert.Equal(t, "Slime divides", combatDivisionPlainText("Slime"))
}

func TestCombatSummonAndRepelAggregateTextIsConcise(t *testing.T) {
	summon := game_state.CombatSummonEvent{
		Spell:   references.InBetXen,
		Created: []game_state.CombatSummonedMonster{{MonsterType: references.MonsterInsect}},
		Count:   4,
		Binding: game_state.CombatSummonBoundToParty,
	}
	assert.Equal(t, "4 loyal allies", combatSummonAggregateText(summon))

	repel := game_state.CombatAnXenCorpResult{
		Affected: make([]game_state.CombatSpellMonster, 2),
		Resisted: make([]game_state.CombatSpellMonster, 1),
		Immune:   make([]game_state.CombatSpellMonster, 3),
	}
	assert.Equal(t, "repels 2 undead; 1 resists; 3 are immune", combatRepelAggregateText(repel))
}

func TestCombatFearNarrationDoesNotClaimUndeadRepulsion(t *testing.T) {
	event := game_state.CombatEvent{Kind: game_state.CombatEventRepelUndead,
		Diagnostics: game_state.CombatDiagnostics{Spell: references.InQuasCorp},
		AnXenCorp: game_state.CombatAnXenCorpEvent{Result: game_state.CombatAnXenCorpResult{
			Affected: []game_state.CombatSpellMonster{{MonsterType: references.MonsterOrc}},
		}}}
	spell, result := combatFearOrRepelText(event)
	assert.Equal(t, "In Quas Corp", spell)
	assert.Equal(t, "1 creature flees.", result)
	event.Diagnostics.Spell = references.AnXenCorp
	spell, result = combatFearOrRepelText(event)
	assert.Equal(t, "An Xen Corp", spell)
	assert.Equal(t, "repels 1 undead", result)
}

func TestCombatRepelFeedbackExplainsEveryOutcome(t *testing.T) {
	tests := []struct {
		name     string
		result   game_state.CombatAnXenCorpResult
		feedback string
		event    string
	}{
		{
			name:     "no undead",
			result:   game_state.CombatAnXenCorpResult{},
			feedback: "No undead are affected.",
			event:    "affects no undead",
		},
		{
			name: "one repelled",
			result: game_state.CombatAnXenCorpResult{
				Affected: []game_state.CombatSpellMonster{{}},
			},
			feedback: "1 undead is repelled.",
			event:    "repels 1 undead",
		},
		{
			name: "all resisted",
			result: game_state.CombatAnXenCorpResult{
				Resisted: make([]game_state.CombatSpellMonster, 2),
			},
			feedback: "No undead are repelled; 2 resist.",
			event:    "repels no undead; 2 resist",
		},
		{
			name: "mixed",
			result: game_state.CombatAnXenCorpResult{
				Affected: make([]game_state.CombatSpellMonster, 3),
				Resisted: make([]game_state.CombatSpellMonster, 2),
				Immune:   []game_state.CombatSpellMonster{{}},
			},
			feedback: "3 undead are repelled; 2 resist; 1 is immune.",
			event:    "repels 3 undead; 2 resist; 1 is immune",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.feedback, combatRepelFeedbackText(tt.result))
			assert.Equal(t, tt.event, combatRepelAggregateText(tt.result))
		})
	}
}

func TestCombatRepelDetailIdentifiesTargets(t *testing.T) {
	result := game_state.CombatAnXenCorpResult{
		Affected: []game_state.CombatSpellMonster{{
			Index:       4,
			MonsterType: references.MonsterSkeleton,
			Position:    references.Position{X: 3, Y: 7},
		}},
	}

	assert.Equal(t, "Affected: Skeleton #4 at (3,7)", combatRepelDetailText(result))
}

func TestCombatEventLineUsesDistinctMarker(t *testing.T) {
	hud := newCombatHUD()
	_, hud.eventOutput = widgets.GetScaledFontWithOutput(widgets.FontConfig{
		Size:                  combatHUDFontSize,
		LineSpacingMultiplier: 1,
		MaxLines:              combatHUDOutputLines,
		MaxChars:              combatHUDOutputChars,
	})
	scene := &GameScene{combatHUD: hud}

	segments := scene.newCombatEventLine().GetSegments()

	require.Len(t, segments, 1)
	assert.Equal(t, text.FontTypeCommandPrompt, segments[0].FontType)
	assert.Equal(t, ucolor.EGALightBlue, segments[0].Color)
}

func TestCombatEventLineCountReservesMarkerColumn(t *testing.T) {
	hud := newCombatHUD()
	_, hud.eventOutput = widgets.GetScaledFontWithOutput(widgets.FontConfig{
		Size:                  combatHUDFontSize,
		LineSpacingMultiplier: 1,
		MaxLines:              combatHUDOutputLines,
		MaxChars:              combatHUDOutputChars,
	})
	hud.eventMaxChars = len("Victory!")
	scene := &GameScene{combatHUD: hud}

	lines := scene.combatEventLineCount(game_state.CombatEvent{Kind: game_state.CombatEventVictory})

	assert.Equal(t, 2, lines, "event payload must wrap within the width remaining after its marker")
}

func TestCombatEventRangeUsesAvailableLineBudget(t *testing.T) {
	hud := newCombatHUD()
	_, output := widgets.GetScaledFontWithOutput(widgets.FontConfig{
		Size:                  combatHUDFontSize,
		LineSpacingMultiplier: 1,
		MaxLines:              combatHUDOutputLines,
		MaxChars:              combatHUDOutputChars,
	})
	hud.eventOutput = output
	for range 10 {
		hud.addEvent(game_state.CombatEvent{Kind: game_state.CombatEventBattleBegun})
	}
	scene := &GameScene{combatHUD: hud}

	start, count := scene.visibleCombatEventRange(8)

	assert.Equal(t, 2, start)
	assert.Equal(t, 8, count)
}

func TestFormatCombatDiagnosticsSeparatesUndeadModifier(t *testing.T) {
	diagnostics := game_state.CombatDiagnostics{
		Damage: game_state.CombatDamageResult{
			AttackRoll:      9,
			AttackRolled:    true,
			ArmorRoll:       2,
			ArmorRolled:     true,
			PostArmorDamage: 7,
		},
		FinalDamage:  3,
		TargetUndead: true,
	}

	assert.Equal(t,
		"Damage 9 - armor 2 = 7; Undead resistance halves damage to 3",
		formatCombatDiagnostics(diagnostics),
	)

	diagnostics.Damage.ArmorRolled = false
	diagnostics.Damage.ArmorRoll = 0
	diagnostics.Damage.PostArmorDamage = 9
	diagnostics.FinalDamage = 4
	assert.Equal(t, "Damage 9; Undead resistance halves damage to 4", formatCombatDiagnostics(diagnostics))
}

func TestFormatCombatDiagnosticsReportsMagicUndeadBypass(t *testing.T) {
	diagnostics := game_state.CombatDiagnostics{
		Damage: game_state.CombatDamageResult{
			AttackRoll:      20,
			AttackRolled:    true,
			PostArmorDamage: 20,
		},
		FinalDamage:  20,
		TargetUndead: true,
		MagicDamage:  true,
	}

	assert.Equal(t,
		"Damage 20; Magic bypasses undead resistance",
		formatCombatDiagnostics(diagnostics),
	)
}

func TestCombatHUDHistoryKeysDoNotTouchCombatState(t *testing.T) {
	hud := newCombatHUD()
	_, output := widgets.GetScaledFontWithOutput(widgets.FontConfig{
		Size:                  combatHUDFontSize,
		LineSpacingMultiplier: 1,
		MaxLines:              combatHUDOutputLines,
		MaxChars:              combatHUDOutputChars,
	})
	hud.eventOutput = output
	hud.historyCount = 10
	hud.eventLineBudget = 6
	scene := &GameScene{combatHUD: hud}

	assert.True(t, scene.handleCombatHUDInput(ebiten.KeyPageUp))
	assert.Equal(t, 1, hud.historyOffset)
	assert.True(t, scene.handleCombatHUDInput(ebiten.KeyHome))
	assert.Equal(t, 4, hud.historyOffset)
	assert.True(t, scene.handleCombatHUDInput(ebiten.KeyPageDown))
	assert.Equal(t, 3, hud.historyOffset)
	assert.True(t, scene.handleCombatHUDInput(ebiten.KeyEnd))
	assert.Zero(t, hud.historyOffset)
}

func TestCombatHUDNewEventPreservesHistoryViewport(t *testing.T) {
	hud := newCombatHUD()
	for index := range 10 {
		hud.addEvent(game_state.CombatEvent{HPLost: index})
	}
	hud.historyOffset = 4

	hud.addEvent(game_state.CombatEvent{HPLost: 10})

	assert.Equal(t, 5, hud.historyOffset)
	assert.Equal(t, 1, hud.unreadEvents)
}
