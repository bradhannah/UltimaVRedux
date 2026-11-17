package main

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// Ordinary actors and their signature frames share the same exact-entry status
// composition. Blink/capture apply dissolve/alpha to the complete frame. Gate
// arrival masks the highlighted silhouette and conditions before adding the
// steady health frame, so neither highlights nor opaque status masks defeat semantics.
func (g *GameScene) drawCombatPartyConditions(
	screen, sprite *ebiten.Image, options *ebiten.DrawImageOptions,
	identity game_state.CombatTurnIdentity, elapsedMs int64,
) {
	character := g.gameState.PartyState.GetCharacter(identity.EntityIndex)
	if character.IsPoisoned() {
		g.drawPoisonOverlay(screen, sprite, options, elapsedMs)
	}
	if g.gameState.CombatScene.IsCombatPCSleeping(identity) {
		g.drawSleepOverlay(screen, sprite, options, elapsedMs)
	}
	if g.gameState.CombatScene.IsCombatPCPossessed(identity) {
		drawCombatPossessedMarker(screen, options)
	}
}

func (g *GameScene) drawCombatMonsterConditions(
	screen, outlinedSprite, sprite *ebiten.Image, options *ebiten.DrawImageOptions,
	monster *game_state.CombatMonster, elapsedMs int64,
) {
	if monster.IsPoisoned() {
		g.drawPoisonOverlay(screen, outlinedSprite, options, elapsedMs)
	}
	if monster.IsAsleep() {
		g.drawSleepOverlay(screen, sprite, options, elapsedMs)
	}
	g.drawCombatMonsterStatusMarks(screen, options, monster)
}

func (g *GameScene) composeCombatSignatureFrame(effect *combatSignatureEffect) *ebiten.Image {
	frame := effect.Frame
	frame.Clear()
	options := &ebiten.DrawImageOptions{}
	elapsedMs := g.gameState.ElapsedMs
	var sprite *ebiten.Image
	if effect.Actor.IsPartyMember {
		character := g.gameState.PartyState.GetCharacter(effect.Actor.EntityIndex)
		index := combatPartySpriteIndex(character, effect.Encounter.IsCombatPCInvisible(effect.Actor),
			effect.Encounter.IsCombatPCRat(effect.Actor))
		if effect.Encounter.IsCombatPCSleeping(effect.Actor) {
			index = indexes.DeadBody
		}
		sprite = g.spriteSheet.GetSprite(index)
	} else {
		monster := effect.Encounter.MonsterManager.GetMonster(effect.Actor.EntityIndex)
		sprite = g.combatMonsterSprite(monster, effect.Actor.EntityIndex, elapsedMs)
	}
	outlined := g.spriteSheet.GetTileEffectProcessor().ApplyDynamicSimple(sprite, sprites.BlackOutline)
	frame.DrawImage(outlined, options)
	if effect.Kind == game_state.CombatEventGating {
		// Highlights are clipped to actual sprite alpha, never HP/status/health frames.
		effect.Highlight.Clear()
		effect.Dissolve.effect.VisitLeadingPixels(0.14, func(rect image.Rectangle) {
			vector.FillRect(effect.Highlight, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), ucolor.CombatMaterializationHighlight, false)
		})
		effect.Highlight.DrawImage(sprite, &ebiten.DrawImageOptions{Blend: ebiten.BlendDestinationIn})
		frame.DrawImage(effect.Highlight, options)
	}
	if effect.Actor.IsPartyMember {
		g.drawCombatPartyConditions(frame, sprite, options, effect.Actor, elapsedMs)
		character := g.gameState.PartyState.GetCharacter(effect.Actor.EntityIndex)
		g.drawCombatInjuredHealthMark(frame, options, g.combatDisclosedHealthBorder(effect.Actor),
			int(character.GetCurrentHp()), int(character.GetMaxHp()))
	} else {
		monster := effect.Encounter.MonsterManager.GetMonster(effect.Actor.EntityIndex)
		g.drawCombatMonsterConditions(frame, outlined, sprite, options, monster, elapsedMs)
		if effect.Kind == game_state.CombatEventGating {
			// Status tints share the revealed silhouette; never restore an opaque
			// daemon underneath sparse fragments. Health-frame semantics follow.
			frame.DrawImage(effect.Dissolve.effect.GetMask(), &ebiten.DrawImageOptions{Blend: ebiten.BlendDestinationIn})
		}
		g.drawCombatInjuredHealthMark(frame, options, g.combatDisclosedHealthBorder(effect.Actor),
			monster.CurrentHP(), monster.MaxHP())
	}

	return frame
}

// Selection tracks the visible signature actor; targeting and direction renderers
// retain their own layers. Identity/position validity is checked before projecting.
func (g *GameScene) combatSelectionPresentation(
	identity game_state.CombatTurnIdentity, position references.Position,
) (references.Position, bool) {
	for _, effect := range g.combatSignatureEffects {
		if effect.Actor != identity || effect.Kind == game_state.CombatEventPossession || !g.combatSignatureEffectCurrent(effect) {
			continue
		}
		if effect.Kind == game_state.CombatEventTeleport {
			phase, _ := effect.blink(max(g.gameState.ElapsedMs-effect.StartedMs, 0))
			if phase == combatTeleportHidden {
				return effect.To, false
			}
			if phase == combatTeleportDeparture {
				return effect.From, true
			}
		}
		if effect.Kind == game_state.CombatEventGating && effect.GatePhase == combatGateArrival {
			return effect.To, false
		}

		return effect.To, true
	}

	return position, !identity.IsPartyMember || !g.gameState.CombatScene.IsCombatPCUnderground(identity)
}
