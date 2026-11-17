package main

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// The health mark and the feet it covers live in this region of a sprite frame.
// Signature effects split a frame here so the two layers fade independently.
func combatHealthFrameBounds() image.Rectangle { return image.Rect(1, 12, 16, 18) }

func (g *GameScene) showCombatHPBars() bool {
	return g.gameConfig == nil || g.gameConfig.GetShowCombatHPBars()
}

// The combat health mark is a bordered bar 13 source pixels wide whose bottom
// edge sits on source row 17: a border one source pixel thick, a black inset a
// quarter of that, and a health strip one and a half source pixels tall.
const (
	combatHealthMarkLeftSourcePixel    = 2
	combatHealthMarkBottomSourcePixel  = 17
	combatHealthMarkWidthSourcePixels  = 13
	combatHealthMarkStripSourcePixels  = 1.5
	combatHealthMarkInsetSourcePixels  = 0.25
	combatHealthStackGapStripFraction  = 4
	combatHealthMarkInsetsPerDimension = 2
)

type combatHealthMark struct {
	outer, inner, strip image.Rectangle
}

// newCombatHealthMark lays the mark out in whole screen pixels, so every figure
// gets the same border, inset and strip wherever it stands. Below two screen
// pixels per source pixel the inset rounds away and the strip fills the border.
func newCombatHealthMark(options *ebiten.DrawImageOptions) combatHealthMark {
	originX, originY := options.GeoM.Apply(0, 0)
	unitX, unitY := options.GeoM.Apply(1, 1)
	scaleX, scaleY := unitX-originX, unitY-originY
	pixels := func(value float64) int { return int(math.Round(value)) }
	borderX, borderY := max(1, pixels(scaleX)), max(1, pixels(scaleY))
	insetX, insetY := pixels(combatHealthMarkInsetSourcePixels*scaleX), pixels(combatHealthMarkInsetSourcePixels*scaleY)
	height := max(1, pixels(combatHealthMarkStripSourcePixels*scaleY)) + combatHealthMarkInsetsPerDimension*(borderY+insetY)
	left := pixels(originX + combatHealthMarkLeftSourcePixel*scaleX)
	bottom := pixels(originY + combatHealthMarkBottomSourcePixel*scaleY)
	outer := image.Rect(left, bottom-height, left+pixels(combatHealthMarkWidthSourcePixels*scaleX), bottom)
	inner := image.Rect(outer.Min.X+borderX, outer.Min.Y+borderY, outer.Max.X-borderX, outer.Max.Y-borderY)
	strip := image.Rect(inner.Min.X+insetX, inner.Min.Y+insetY, inner.Max.X-insetX, inner.Max.Y-insetY)

	return combatHealthMark{outer: outer, inner: inner, strip: strip}
}

func (mark combatHealthMark) drawFrame(screen *ebiten.Image, border color.Color) {
	fillCombatHealthRect(screen, mark.outer, border)
	fillCombatHealthRect(screen, mark.inner, ucolor.EGABlack)
}

func fillCombatHealthRect(screen *ebiten.Image, rect image.Rectangle, clr color.Color) {
	vector.FillRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), clr, false)
}

// drawCombatHealthMark draws the border in the side's colour and the strip in
// green, yellow or red.
func drawCombatHealthMark(
	screen *ebiten.Image, options *ebiten.DrawImageOptions, border color.Color, currentHP, maxHP int,
) {
	mark := newCombatHealthMark(options)
	mark.drawFrame(screen, border)
	strip := mark.strip
	strip.Max.X = strip.Min.X + combatHealthFill(currentHP, maxHP, strip.Dx())
	fillCombatHealthRect(screen, strip, combatHealthTierColor(currentHP, maxHP))
}

func combatHealthBorderColor(loyalty game_state.CombatMonsterLoyalty) ucolor.EGAColor {
	if loyalty == game_state.CombatMonsterLoyaltyParty {
		return ucolor.CombatHealthFriend
	}

	return ucolor.CombatAllegianceEnemy
}

// The battlefield marks only the injured; Turn Order marks every row.
func (g *GameScene) drawCombatInjuredHealthMark(
	screen *ebiten.Image, options *ebiten.DrawImageOptions, border color.Color, currentHP, maxHP int,
) {
	if g.showCombatHPBars() && currentHP < maxHP {
		drawCombatHealthMark(screen, options, border, currentHP, maxHP)
	}
}

// An actor whose side is not disclosed keeps a black border.
func (g *GameScene) combatDisclosedHealthBorder(identity game_state.CombatTurnIdentity) ucolor.EGAColor {
	if actor, disclosed := g.gameState.ProjectCombatActor(identity, 0); disclosed {
		return combatHealthBorderColor(actor.Loyalty)
	}

	return ucolor.EGABlack
}

// Draw HP and allegiance together after every actor/status/effect silhouette.
// Neighboring 18-pixel sprites overlap 16-pixel cells and must not erase either
// the HP fill or its exterior. This path is combat-only; world/vehicle consumers
// of drawCombatHealthBar retain their original rendering.
func (g *GameScene) drawCombatHealthFrames(screen *ebiten.Image) {
	g.gameState.ForEachDisclosedCombatActor(func(actor game_state.CombatActorPresentation) {
		if !g.combatAllegiancePresented(actor) || g.combatSignatureHidesActor(actor.Identity) {
			return
		}
		options := g.combatEntityDrawOptions(actor.Position, actor.Identity.IsPartyMember, actor.Identity.EntityIndex, actor.Identity.Slot)
		g.drawCombatActorHealthFrame(screen, options, actor)
	})
	for _, effect := range g.combatSignatureEffects {
		options, visible := g.combatSignatureHealthOptions(effect)
		if !visible {
			continue
		}
		// The initial signature pass excludes this region. Apply the same mask
		// and opacity here exactly once, not over an already blended HP underlay.
		g.prepareCombatSignatureLayers(effect)
		screen.DrawImage(effect.HealthFrame, options)
	}
}

func (g *GameScene) drawCombatActorHealthFrame(screen *ebiten.Image, options *ebiten.DrawImageOptions, actor game_state.CombatActorPresentation) {
	border := combatHealthBorderColor(actor.Loyalty)
	if actor.Identity.IsPartyMember {
		character := g.gameState.PartyState.GetCharacter(actor.Identity.EntityIndex)
		g.drawCombatInjuredHealthMark(screen, options, border, int(character.GetCurrentHp()), int(character.GetMaxHp()))

		return
	}
	stack := g.combatPartyInsectStack(actor.Position)
	if len(stack) > 0 {
		if stack[0] != actor.Identity.EntityIndex {
			return
		}
		if g.showCombatHPBars() && g.combatInsectStackInjured(stack) {
			mark := newCombatHealthMark(options)
			mark.drawFrame(screen, border)
			g.drawCombatInsectStackHealth(screen, mark.strip, stack)
		}
		g.drawCombatInsectStackIndicators(screen, options, stack)

		return
	}
	monster := g.gameState.CombatScene.MonsterManager.GetMonster(actor.Identity.EntityIndex)
	g.drawCombatInjuredHealthMark(screen, options, border, monster.CurrentHP(), monster.MaxHP())
}

func (g *GameScene) combatSignatureHealthOptions(effect *combatSignatureEffect) (*ebiten.DrawImageOptions, bool) {
	if !g.combatSignatureEffectCurrent(effect) || effect.Kind == game_state.CombatEventPossession ||
		(effect.Kind == game_state.CombatEventGating && effect.GatePhase == combatGateArrival) {
		return nil, false
	}
	position, opacity := effect.To, float32(1)
	if effect.Kind == game_state.CombatEventTeleport {
		phase, progress := effect.blink(max(g.gameState.ElapsedMs-effect.StartedMs, 0))
		if phase == combatTeleportHidden {
			return nil, false
		}
		if phase == combatTeleportDeparture {
			position, opacity = effect.From, 1-progress
		} else {
			opacity = progress
		}
	}
	if !g.gameState.CombatTerrainAllowsObjectDisplay(position) {
		return nil, false
	}
	options := g.combatEntityDrawOptions(position, effect.Actor.IsPartyMember, effect.Actor.EntityIndex, effect.Actor.Slot)
	options.ColorScale.ScaleAlpha(opacity)

	return options, true
}

func (g *GameScene) combatSignatureImage(effect *combatSignatureEffect) *ebiten.Image {
	frame := g.composeCombatSignatureFrame(effect)
	if effect.Kind != game_state.CombatEventGating {
		return spriteWithDissolveMask(frame, &effect.Dissolve)
	}

	return frame
}

func (g *GameScene) prepareCombatSignatureLayers(effect *combatSignatureEffect) {
	frame := g.combatSignatureImage(effect)
	// Even with bars hidden this region contains sprite feet/status pixels.
	// Always partition and draw both layers; only HP artwork is conditional.
	// Keep both layers at the full source dimensions and use the same transform.
	// Cropped-source quads can choose a different nearest texel at noninteger
	// scaling ties. Partition at integral source pixels before scaling instead.
	effect.BodyFrame.Clear()
	effect.BodyFrame.DrawImage(frame, &ebiten.DrawImageOptions{})
	combatHealthFrameRegion(effect.BodyFrame).Clear()
	effect.HealthFrame.Clear()
	combatHealthFrameRegion(effect.HealthFrame).DrawImage(frame, &ebiten.DrawImageOptions{})
}

func combatHealthFrameRegion(frame *ebiten.Image) *ebiten.Image {
	region, ok := frame.SubImage(combatHealthFrameBounds()).(*ebiten.Image)
	if !ok {
		panic("combat health frame region is not an Ebitengine image")
	}

	return region
}
