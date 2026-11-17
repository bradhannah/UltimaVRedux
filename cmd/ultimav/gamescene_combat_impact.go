package main

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const (
	combatImpactDurationMs         int64 = 1250
	combatImpactLabelFloorMs       int64 = 300
	combatJoltDurationMs                 = 320
	combatLungeDurationMs                = 300
	combatJoltDurationSeconds            = float64(combatJoltDurationMs) / 1000
	combatLungeDurationSeconds           = float64(combatLungeDurationMs) / 1000
	combatJoltDistancePixels             = 3.5
	combatLungeDistancePixels            = 4.0
	combatJoltOscillations               = 1.5
	combatDamageRisePixels               = 18.0
	combatDamageFadeStart                = 0.68
	combatDamageAlphaSteps               = 8.0
	combatDamageEaseExponent             = 2.0
	combatTileCenterPixels               = 8
	combatEntityCenterSourcePixels       = 9
	combatDamageShadowScreenPixels       = 1
	combatDamageMaxChars                 = 12
	combatImpactLanePixels               = 10
	combatFireTintDurationSeconds        = 0.45
	combatFireTintMaximumAlpha           = 0.72
	combatStrikeDurationMs               = 180
	combatStrikeDurationSeconds          = float64(combatStrikeDurationMs) / 1000
	combatStrikeRevealFraction           = 0.55
	combatStrikeRouteStart               = 0.24
	combatStrikeRouteEnd                 = 0.76
	combatStrikeShadowSourcePixels       = 3.0
	combatStrikeFaceSourcePixels         = 1.0
)

type combatStrikeVisual struct {
	Duration               float64
	RevealFraction         float64
	RouteStart, RouteEnd   float64
	ShadowWidth, FaceWidth float32
	ShadowColor, FaceColor ucolor.EGAColor
}

type combatStrikeLine struct {
	FromX, FromY float64
	ToX, ToY     float64
}

// CombatImpactEffect coordinates local sprite motion and floating damage text.
type CombatImpactEffect struct {
	Feedback          game_state.CombatAttackFeedback
	StartedMs         int64
	Elapsed           float64
	Fire              bool
	SpellOutcome      game_state.CombatSpellOutcome
	TargetID          game_state.CombatantID
	FireInjury        *game_state.CombatFireInjury
	fireOwner         game_state.CombatFireInjuryOwner
	fireClockAt       int64
	fireAdmittedAt    int64
	fireMovement      bool
	fireMovementBound bool
	timing            combatPresentationTiming
	motion            combatImpactMotion
}

// impactLabelSeconds is the floating label's reading budget: scaled with the
// admitted combat speed, but never below combatImpactLabelFloorMs.
func (timing combatPresentationTiming) impactLabelSeconds() float64 {
	return float64(max(timing.durationMs(combatImpactDurationMs), combatImpactLabelFloorMs)) / 1000
}

// Motion and the label reading budget both follow the admitted timing.
type combatImpactMotion struct {
	jolt, lunge, strike float64
}

func newCombatImpactMotion(timing combatPresentationTiming) combatImpactMotion {
	return combatImpactMotion{
		jolt: timing.seconds(combatJoltDurationMs), lunge: timing.seconds(combatLungeDurationMs),
		strike: timing.seconds(combatStrikeDurationMs),
	}
}

func (effect CombatImpactEffect) motionDurations() combatImpactMotion {
	if effect.motion.jolt == 0 {
		return newCombatImpactMotion(effect.timing)
	}

	return effect.motion
}

func (g *GameScene) showCombatAreaSpellImpact(event game_state.CombatEvent) {
	result := event.SpellTarget
	if result == nil || !result.Disclosed {
		return
	}
	effect := CombatImpactEffect{
		StartedMs: g.gameState.ElapsedMs,
		timing:    g.captureCombatTiming(),
		motion:    newCombatImpactMotion(g.captureCombatTiming()),
		Feedback: game_state.CombatAttackFeedback{
			Attacker: event.Attacker, Target: result.Target, Result: event.Result,
			HPLost: result.HPLost, Fatal: result.Fatal,
			Presentation: game_state.CombatAttackPresentation{
				Delivery: game_state.CombatAttackDeliverySpell, Source: references.NoEquipment,
			},
		},
		Fire:         event.Diagnostics.Spell == references.InFlamHur && result.HPLost > 0,
		SpellOutcome: result.Outcome,
		TargetID:     result.CombatantID,
	}
	if effect.Fire {
		injury := g.gameState.CombatSpellFireInjury(*result)
		g.admitCombatFireImpact(effect, injury)
	} else {
		g.combatImpactEffects = append(g.combatImpactEffects, effect)
	}
	// Retain the cone's successful arrival effect, owned by the same result as
	// its label/tint. A processed zero-damage square is not a successful impact.
	if event.Diagnostics.Spell != references.InVasPorYlem &&
		(result.Outcome == game_state.CombatSpellDamaged || result.Outcome == game_state.CombatSpellSlept ||
			result.Outcome == game_state.CombatSpellPoisoned) {
		g.gameState.SystemCallbacks.Visual.PlayKapowAtPosition(result.Target.Position)
	}
}

func (g *GameScene) combatAreaImpactStillDisclosed(effect CombatImpactEffect) bool {
	if effect.SpellOutcome == game_state.CombatSpellUnspecified {
		return true
	}
	target := effect.Feedback.Target
	if !g.gameState.CombatTerrainAllowsObjectDisplay(target.Position) {
		return false
	}
	if target.IsPartyMember {
		return !g.gameState.CombatScene.IsCombatPCUnderground(combatTurnIdentityForFeedback(target))
	}
	monster := g.gameState.CombatScene.MonsterManager.GetMonster(target.Index)

	return monster != nil && monster.CombatantID() == effect.TargetID && !monster.IsInvisible() && !monster.IsCorpserHidden()
}

func (g *GameScene) showCombatImpact(feedback game_state.CombatAttackFeedback) {
	if g.combatImpactCapture != nil {
		g.combatImpactCapture = append(g.combatImpactCapture, feedback)

		return
	}
	g.combatImpactEffects = append(g.combatImpactEffects, CombatImpactEffect{
		StartedMs: g.gameState.ElapsedMs,
		timing:    g.captureCombatTiming(),
		motion:    newCombatImpactMotion(g.captureCombatTiming()),
		Feedback:  feedback,
		Elapsed:   0,
		Fire:      false,
	})
}

func (g *GameScene) captureCombatImpacts(resolve func() bool) (bool, []game_state.CombatAttackFeedback) {
	if g.combatImpactCapture != nil {
		panic("combat impact capture is already active")
	}
	g.combatImpactCapture = make([]game_state.CombatAttackFeedback, 0, 3)
	defer func() {
		g.combatImpactCapture = nil
	}()

	accepted := resolve()

	return accepted, g.combatImpactCapture
}

func (g *GameScene) showCombatFireImpact(event game_state.CombatEvent) {
	if event.FireInjury == nil {
		return
	}
	injury := *event.FireInjury
	g.admitCombatFireImpact(CombatImpactEffect{
		Feedback: game_state.CombatAttackFeedback{
			Attacker: injury.Target,
			Target:   injury.Target,
			Result:   game_state.CombatFeedbackDamage,
			HPLost:   injury.HPLost,
			Fatal:    injury.Fatal,
			Presentation: game_state.CombatAttackPresentation{
				Delivery: game_state.CombatAttackDeliveryEnvironmental,
				Source:   references.NoEquipment,
			},
		},
		Elapsed: 0,
		Fire:    true,
	}, injury)
}

func compareCoordinate(target, attacker references.Coordinate) int {
	switch {
	case target > attacker:
		return 1
	case target < attacker:
		return -1
	default:
		return 0
	}
}

func (g *GameScene) updateCombatImpactEffects(_ float64) {
	if !g.gameState.IsInCombat() {
		clear(g.combatImpactEffects)
		g.combatImpactEffects = g.combatImpactEffects[:0]

		return
	}

	writeIndex := 0
	for i := range g.combatImpactEffects {
		effect := &g.combatImpactEffects[i]
		if effect.FireInjury != nil {
			if _, disclosed := g.gameState.ProjectCombatFireInjury(*effect.FireInjury, effect.fireOwner); !disclosed {
				continue
			}
			effect.Elapsed = float64(max(g.gameState.ElapsedMs-effect.fireClockAt, 0)) / 1000
			if effect.fireMovement {
				effect.Elapsed = 0
			}
		} else {
			// Queues can admit impacts earlier in this same update. Only time
			// after admission belongs to their motion and reading clock.
			effect.Elapsed = float64(max(g.gameState.ElapsedMs-effect.StartedMs, 0)) / 1000
			if !g.combatAreaImpactStillDisclosed(*effect) {
				continue
			}
		}
		if effect.Elapsed >= effect.timing.impactLabelSeconds() {
			continue
		}
		g.combatImpactEffects[writeIndex] = *effect
		writeIndex++
	}
	clear(g.combatImpactEffects[writeIndex:])
	g.combatImpactEffects = g.combatImpactEffects[:writeIndex]
}

func (g *GameScene) combatEntityOffset(isPartyMember bool, index, slot int) (float64, float64) {
	movementX, movementY := g.combatMovementOffset(isPartyMember, index, slot)
	impactX, impactY := g.combatImpactEntityOffset(isPartyMember, index, slot)

	return movementX + impactX, movementY + impactY
}

func (g *GameScene) combatImpactEntityOffset(isPartyMember bool, index, slot int) (float64, float64) {
	for i := len(g.combatImpactEffects) - 1; i >= 0; i-- {
		effect := g.combatImpactEffects[i]
		feedback := effect.Feedback
		if effect.Fire {
			continue
		}
		if feedback.Target.IsPartyMember == isPartyMember && feedback.Target.Index == index &&
			(!isPartyMember || feedback.Target.Slot == slot) &&
			feedback.Result == game_state.CombatFeedbackDamage && effect.Elapsed < effect.motionDurations().jolt {
			return combatTargetJoltOffset(effect)
		}
		if !effect.Fire && feedback.Presentation.Delivery == game_state.CombatAttackDeliveryMelee &&
			feedback.Attacker.IsPartyMember == isPartyMember && feedback.Attacker.Index == index &&
			(!isPartyMember || feedback.Attacker.Slot == slot) &&
			effect.Elapsed < effect.motionDurations().lunge {
			return combatAttackerLungeOffset(effect)
		}
	}

	return 0, 0
}

func (g *GameScene) drawCombatFireImpactOverlay(
	screen, sprite *ebiten.Image,
	baseOptions *ebiten.DrawImageOptions,
	isPartyMember bool,
	index, slot int,
) {
	alpha := g.combatFireImpactAlpha(isPartyMember, index, slot)
	if alpha == 0 {
		return
	}
	mask := sprites.GetSpriteMask(sprite)
	if mask == nil {
		return
	}
	overlayOptions := *baseOptions
	overlayOptions.ColorScale.Reset()
	overlayOptions.ColorScale.ScaleWithColor(ucolor.EGALightRed.ToRGBA())
	overlayOptions.ColorScale.ScaleAlpha(float32(alpha))
	screen.DrawImage(mask, &overlayOptions)
}

func (g *GameScene) combatFireImpactAlpha(isPartyMember bool, index, slot int) float64 {
	for effectIndex := len(g.combatImpactEffects) - 1; effectIndex >= 0; effectIndex-- {
		effect := g.combatImpactEffects[effectIndex]
		target := effect.Feedback.Target
		if effect.Fire && effect.Elapsed < combatFireTintDurationSeconds &&
			g.combatFireEffectVisible(effect) &&
			target.IsPartyMember == isPartyMember && target.Index == index &&
			(!isPartyMember || target.Slot == slot) {
			return combatFireTintMaximumAlpha * (1 - effect.Elapsed/combatFireTintDurationSeconds)
		}
	}

	return 0
}

func combatTargetJoltOffset(effect CombatImpactEffect) (float64, float64) {
	progress := effect.Elapsed / effect.motionDurations().jolt
	damping := 1 - progress
	wave := math.Sin(progress * math.Pi * 2 * combatJoltOscillations)
	distance := combatJoltDistancePixels * damping * wave
	directionX, directionY := combatEffectDirection(effect)

	return directionX * distance, directionY * distance
}

func combatAttackerLungeOffset(effect CombatImpactEffect) (float64, float64) {
	progress := effect.Elapsed / effect.motionDurations().lunge
	distance := combatLungeDistancePixels * math.Sin(progress*math.Pi)
	directionX, directionY := combatEffectDirection(effect)

	return directionX * distance, directionY * distance
}

func combatEffectDirection(effect CombatImpactEffect) (float64, float64) {
	attackerPos := effect.Feedback.Attacker.Position
	targetPos := effect.Feedback.Target.Position

	return float64(compareCoordinate(targetPos.X, attackerPos.X)),
		float64(compareCoordinate(targetPos.Y, attackerPos.Y))
}

func combatStrikeVisualFor(presentation game_state.CombatAttackPresentation) (combatStrikeVisual, bool) {
	if presentation.Delivery != game_state.CombatAttackDeliveryMelee {
		return combatStrikeVisual{}, false
	}

	// Original U5 had no traveling melee strike. Redux adds a
	// short presentation-only streak to make the occupied target spatially clear.
	// Source remains part of the selection contract so weapon-family presets can
	// be introduced here without changing authoritative combat callbacks.
	return combatStrikeVisual{
		Duration:       combatStrikeDurationSeconds,
		RevealFraction: combatStrikeRevealFraction,
		RouteStart:     combatStrikeRouteStart,
		RouteEnd:       combatStrikeRouteEnd,
		ShadowWidth:    combatStrikeShadowSourcePixels,
		FaceWidth:      combatStrikeFaceSourcePixels,
		ShadowColor:    ucolor.EGABlack,
		FaceColor:      ucolor.EGAWhite,
	}, true
}

func combatStrikeLineAtProgress(
	fromX, fromY, toX, toY float64,
	elapsed float64,
	visual combatStrikeVisual,
) (combatStrikeLine, bool) {
	if elapsed < 0 || elapsed >= visual.Duration || visual.Duration <= 0 || visual.RevealFraction <= 0 ||
		visual.RevealFraction >= 1 || visual.RouteStart >= visual.RouteEnd {
		return combatStrikeLine{}, false
	}
	progress := elapsed / visual.Duration
	head := visual.RouteEnd
	tail := visual.RouteStart
	if progress < visual.RevealFraction {
		head = visual.RouteStart + (visual.RouteEnd-visual.RouteStart)*(progress/visual.RevealFraction)
	} else {
		tail = visual.RouteStart + (visual.RouteEnd-visual.RouteStart)*
			((progress-visual.RevealFraction)/(1-visual.RevealFraction))
	}
	pointAt := func(fraction float64) (float64, float64) {
		return fromX + (toX-fromX)*fraction, fromY + (toY-fromY)*fraction
	}
	line := combatStrikeLine{}
	line.FromX, line.FromY = pointAt(tail)
	line.ToX, line.ToY = pointAt(head)

	return line, true
}

func (g *GameScene) drawCombatStrikeEffects(screen *ebiten.Image) {
	for index := range g.combatImpactEffects {
		effect := g.combatImpactEffects[index]
		visual, ok := combatStrikeVisualFor(effect.Feedback.Presentation)
		visual.Duration = effect.motionDurations().strike
		if !ok {
			continue
		}
		attacker := effect.Feedback.Attacker
		target := effect.Feedback.Target
		attackerOptions := g.combatEntityDrawOptions(
			attacker.Position, attacker.IsPartyMember, attacker.Index, attacker.Slot,
		)
		targetOptions := g.combatEntityDrawOptions(
			target.Position, target.IsPartyMember, target.Index, target.Slot,
		)
		attackerX, attackerY := attackerOptions.GeoM.Apply(
			combatEntityCenterSourcePixels, combatEntityCenterSourcePixels,
		)
		targetX, targetY := targetOptions.GeoM.Apply(
			combatEntityCenterSourcePixels, combatEntityCenterSourcePixels,
		)
		line, visible := combatStrikeLineAtProgress(
			attackerX, attackerY, targetX, targetY, effect.Elapsed, visual,
		)
		if !visible {
			continue
		}
		scale := combatActivePCSourcePixelScale(attackerOptions)
		vector.StrokeLine(
			screen,
			float32(line.FromX), float32(line.FromY), float32(line.ToX), float32(line.ToY),
			visual.ShadowWidth*scale,
			visual.ShadowColor,
			false,
		)
		vector.StrokeLine(
			screen,
			float32(line.FromX), float32(line.FromY), float32(line.ToX), float32(line.ToY),
			visual.FaceWidth*scale,
			visual.FaceColor,
			false,
		)
	}
}

func (g *GameScene) drawCombatImpactEffects(screen *ebiten.Image) {
	if g.combatDamageOutput == nil || !g.gameState.IsInCombat() {
		return
	}

	for effectIndex := range g.combatImpactEffects {
		if !g.combatFireEffectVisible(g.combatImpactEffects[effectIndex]) {
			continue
		}
		lane := combatImpactLane(g.combatImpactEffects, effectIndex)
		g.drawCombatDamageIndicator(screen, &g.combatImpactEffects[effectIndex], lane)
	}
}

func combatImpactLane(effects []CombatImpactEffect, effectIndex int) int {
	lane := 0
	target := effects[effectIndex].Feedback.Target
	for priorIndex := range effectIndex {
		priorTarget := effects[priorIndex].Feedback.Target
		if priorTarget.IsPartyMember == target.IsPartyMember && priorTarget.Index == target.Index &&
			(!target.IsPartyMember || priorTarget.Slot == target.Slot) {
			lane++
		}
	}

	return lane
}

func combatImpactLaneOffset(lane int) float64 {
	if lane == 0 {
		return 0
	}
	direction := 1
	if lane%2 == 1 {
		direction = -1
	}

	return float64((lane + 1) / 2 * combatImpactLanePixels * direction)
}

func combatImpactLanePosition(baseX float64, lane int, minimumX, maximumX float64) float64 {
	baseX = math.Min(math.Max(baseX, minimumX), maximumX)
	if baseX-minimumX < combatImpactLanePixels {
		return math.Min(math.Max(baseX+float64(lane*combatImpactLanePixels), minimumX), maximumX)
	}
	if maximumX-baseX < combatImpactLanePixels {
		return math.Min(math.Max(baseX-float64(lane*combatImpactLanePixels), minimumX), maximumX)
	}

	offset := combatImpactLaneOffset(lane)
	position := baseX + offset
	if position < minimumX || position > maximumX {
		if baseX-minimumX <= maximumX-baseX {
			position = baseX + float64(lane*combatImpactLanePixels)
		} else {
			position = baseX - float64(lane*combatImpactLanePixels)
		}
	}

	return math.Min(math.Max(position, minimumX), maximumX)
}

func (g *GameScene) drawCombatDamageIndicator(
	screen *ebiten.Image,
	effect *CombatImpactEffect,
	lane int,
) {
	progress := effect.Elapsed / effect.timing.impactLabelSeconds()
	easeOut := 1 - math.Pow(1-progress, combatDamageEaseExponent)
	rise := easeOut * combatDamageRisePixels

	screenX, screenY := g.combatImpactLabelPosition(*effect)

	mapOptions := &ebiten.DrawImageOptions{}
	sprites.ApplyDrawOptionsFromPercentsForWholeScreen(mapOptions, g.mapImage, g.mapScreenPlacement())
	mapOptions.GeoM.Translate(g.screenShakeOffsetX, g.screenShakeOffsetY)
	textX, textY := mapOptions.GeoM.Apply(screenX+combatTileCenterPixels, screenY-rise)

	label, textBaseColor := combatImpactText(*effect)
	textWidth, textHeight := g.combatDamageOutput.Font.MeasureString(label)
	battlefield := g.combatLayout().battlefieldRectangle(screen.Bounds())
	textX = combatImpactLanePosition(
		textX,
		lane,
		float64(battlefield.Min.X)+textWidth/2,
		float64(battlefield.Max.X)-textWidth/2,
	)
	textY = math.Min(
		math.Max(textY, float64(battlefield.Min.Y)),
		float64(battlefield.Max.Y)-textHeight,
	)
	shadowX, shadowY := combatImpactShadowPosition(textX, textY)
	drawWidth := max(int(math.Ceil(textWidth)), 1)
	alpha := combatDamageAlpha(progress)

	g.drawCombatDamageText(screen, label, shadowX, shadowY, textWidth, drawWidth,
		combatDamageColor(ucolor.CombatDamageShadow, alpha))
	g.drawCombatDamageText(screen, label, textX, textY, textWidth, drawWidth,
		combatDamageColor(textBaseColor, alpha))
}

func combatImpactShadowPosition(textX, textY float64) (float64, float64) {
	return textX + combatDamageShadowScreenPixels, textY + combatDamageShadowScreenPixels
}

func combatFeedbackText(feedback game_state.CombatAttackFeedback) (string, ucolor.EGAColor) {
	if feedback.Fatal {
		if feedback.Target.IsPartyMember {
			return "FALLEN", ucolor.EGALightRed
		}

		return "SLAIN", ucolor.EGALightRed
	}
	switch feedback.Result {
	case game_state.CombatFeedbackMiss:
		return "MISS", ucolor.CombatMissText
	case game_state.CombatFeedbackGraze:
		return "GRAZE", ucolor.CombatGrazeText
	case game_state.CombatFeedbackDamage:
		return fmt.Sprintf("-%d", feedback.HPLost), ucolor.CombatDamageText
	case game_state.CombatFeedbackPoison:
		return combatPoisonLabel, ucolor.EGALightGreen
	case game_state.CombatFeedbackSleep:
		return "SLEEP", ucolor.EGALightMagenta
	case game_state.CombatFeedbackTheft:
		return "STOLEN", ucolor.EGALightCyan
	default:
		panic("unsupported combat feedback result")
	}
}

func combatImpactText(effect CombatImpactEffect) (string, ucolor.EGAColor) {
	switch effect.SpellOutcome {
	case game_state.CombatSpellAvoided:
		return "AVOIDED", ucolor.CombatMissText
	case game_state.CombatSpellResisted:
		return "RESIST", ucolor.CombatMissText
	case game_state.CombatSpellImmune:
		return "IMMUNE", ucolor.CombatMissText
	case game_state.CombatSpellZeroDamage:
		return "NO DAMAGE", ucolor.CombatGrazeText
	case game_state.CombatSpellUnspecified, game_state.CombatSpellDamaged, game_state.CombatSpellSlept, game_state.CombatSpellPoisoned:
		return combatFeedbackText(effect.Feedback)
	default:
		panic("unknown combat spell impact outcome")
	}
}

func (g *GameScene) drawCombatDamageText(
	screen *ebiten.Image,
	label string,
	indicatorX, indicatorY, textWidth float64,
	drawWidth int,
	textColor color.RGBA,
) {
	g.combatDamageOutput.Reset()
	g.combatDamageOutput.AddTextWithColor(label, textColor)
	g.combatDamageOutput.DrawPanelContent(
		screen,
		image.Pt(int(indicatorX-textWidth/2), int(indicatorY)),
		drawWidth,
		etext.AlignStart,
	)
}

func combatDamageAlpha(progress float64) float64 {
	if progress <= combatDamageFadeStart {
		return 1
	}

	alpha := 1 - (progress-combatDamageFadeStart)/(1-combatDamageFadeStart)

	return math.Round(alpha*combatDamageAlphaSteps) / combatDamageAlphaSteps
}

func combatDamageColor(base ucolor.EGAColor, alpha float64) color.RGBA {
	result := base.ToRGBA()
	result.A = uint8(math.Round(alpha * ucolor.AlphaOpaque))

	return result
}
