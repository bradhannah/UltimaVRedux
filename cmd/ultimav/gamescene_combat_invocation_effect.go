package main

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const (
	combatSummonInvocationMs    = 120
	combatSummonSealMs          = 140
	combatSummonManifestationMs = 300
	combatSummonAllegianceMs    = 180
	combatDaemonInvocationMs    = 150
	combatDaemonSealMs          = 180
	combatDaemonManifestationMs = 420
	combatDaemonAllegianceMs    = 280
	combatRepelCasterCueMs      = 240
	combatRepelTargetsMs        = 640
	combatManifestationSteps    = 4
	combatRepelTargetPulses     = 3
	combatRepelPulseVisiblePart = 0.65
	combatRepelPulseAlpha       = 0.72
)

type combatSummonEffectPhase int

const (
	combatSummonInvocation combatSummonEffectPhase = iota
	combatSummonSeal
	combatSummonManifestation
	combatSummonAllegiance
)

type combatSummonSpellEffect struct {
	CasterIdx  int
	CasterSlot int
	Caster     references.Position
	Result     game_state.CombatSummonResult
	Phase      combatSummonEffectPhase
	Progress   float32
}

func (effect *combatSummonSpellEffect) hidesMonster(index int) bool {
	shouldHide := effect.Phase == combatSummonInvocation || effect.Phase == combatSummonSeal ||
		(effect.Phase == combatSummonManifestation && combatManifestationStep(effect.Progress)%2 == 0)
	if !shouldHide {
		return false
	}
	for _, created := range effect.Result.Created {
		if created.Index == index {
			return true
		}
	}

	return false
}

type combatRepelEffectPhase int

const (
	combatRepelCasterCue combatRepelEffectPhase = iota
	combatRepelTargets
)

type combatRepelSpellEffect struct {
	CasterIdx  int
	CasterSlot int
	Caster     references.Position
	Result     game_state.CombatAnXenCorpResult
	Phase      combatRepelEffectPhase
	Progress   float32
}

func (g *GameScene) clearCombatInvocationEffects() {
	g.combatSummonEffect = nil
	g.combatRepelEffect = nil
}

func (g *GameScene) thenCastCombatSummon(spell references.Spell, casterIdx int) event_queue.Sequence {
	return g.thenCombatSummon(spell, casterIdx, false)
}

func (g *GameScene) thenUseCombatDaemonScroll(canCommit func() bool, didCommit *bool) event_queue.Sequence {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		casterIdx := g.gameState.CombatScene.GetActiveCombatantIndex()
		owner := g.gameState.CaptureCombatUseOwner()
		origin := g.gameState.MapState.PlayerLocation

		return g.thenCommitCombatScroll(references.ScrollKalXenCorp,
			func() event_queue.Sequence {
				return func(sequence *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
					if !g.gameState.IsCurrentCombatUseOwner(owner) || g.gameState.MapState.PlayerLocation != origin {
						return sequence
					}

					return g.thenCombatSummon(references.KalXenCorp, casterIdx, true)(sequence)
				}
			}, canCommit, didCommit)(q)
	}
}

// combatSummonFailureMessage keeps "no room" for a full fight. A summon that
// found no legal square in its eight random tries says so instead.
func combatSummonFailureMessage(outcome game_state.CombatSummonOutcome) string {
	if outcome == game_state.CombatSummonNoLegalPosition {
		return "Failed! No open ground."
	}

	return "No room to summon!"
}

func (g *GameScene) thenCombatSummon(spell references.Spell, casterIdx int, fromScroll bool) event_queue.Sequence {
	timing := g.captureCombatTiming()
	owner := g.gameState.CaptureCombatUseOwner()
	origin := g.gameState.MapState.PlayerLocation

	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var result game_state.CombatSummonResult
		invocationMs, sealMs, manifestationMs, allegianceMs := combatSummonDurations(spell)

		return q.Then(g.combatTimingStep(timing, func() {
			if fromScroll && (!g.gameState.IsCurrentCombatUseOwner(owner) ||
				g.gameState.MapState.PlayerLocation != origin) {
				result.Outcome = game_state.CombatSummonInvalidCaster

				return
			}
			// Resolve once before exposing any presentation state. The result owns all later VFX decisions.
			//nolint:exhaustive // Dispatch limits this sequence to the three summoning spells.
			switch spell {
			case references.KalXen:
				result = g.gameState.CastCombatKalXen()
			case references.InBetXen:
				result = g.gameState.CastCombatInBetXen()
			case references.KalXenCorp:
				if fromScroll {
					result = g.gameState.UseCombatKalXenCorpScroll(casterIdx)
				} else {
					result = g.gameState.CastCombatKalXenCorp(casterIdx)
				}
			default:
				panic("unsupported combat summon spell")
			}
			if result.Outcome != game_state.CombatSummonSucceeded {
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundSpellFailure)
				g.gameState.SystemCallbacks.UI.ShowActionFeedback(combatSummonFailureMessage(result.Outcome), true)

				return
			}
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
			caster := g.activeCombatCasterIdentity(casterIdx)
			g.combatSummonEffect = &combatSummonSpellEffect{
				CasterIdx:  casterIdx,
				CasterSlot: caster.Slot,
				Caster:     g.gameState.CombatScene.GetCombatantPosition(caster),
				Result:     result,
				Phase:      combatSummonInvocation,
			}
		})).If(func() bool { return result.Outcome == game_state.CombatSummonSucceeded }, func(effectQ *event_queue.SequentialEventBuilder) {
			effectQ.Animate(timing.duration(invocationMs), g.updateCombatSummonProgress).
				Then(func() { g.setCombatSummonPhase(combatSummonSeal) }).
				Animate(timing.duration(sealMs), g.updateCombatSummonProgress).
				Then(func() { g.setCombatSummonPhase(combatSummonManifestation) }).
				Animate(timing.duration(manifestationMs), g.updateCombatSummonProgress).
				Then(func() {
					g.setCombatSummonPhase(combatSummonAllegiance)
					if result.Binding == game_state.CombatSummonBindingFailed {
						g.gameState.SystemCallbacks.UI.ShowActionFeedback("Oops...", true)
					}
				}).Animate(timing.duration(allegianceMs), g.updateCombatSummonProgress).
				Then(func() { g.combatSummonEffect = nil })
		})
	}
}

func combatSummonDurations(spell references.Spell) (uint64, uint64, uint64, uint64) {
	if spell == references.KalXenCorp {
		return combatDaemonInvocationMs, combatDaemonSealMs, combatDaemonManifestationMs, combatDaemonAllegianceMs
	}

	return combatSummonInvocationMs, combatSummonSealMs, combatSummonManifestationMs, combatSummonAllegianceMs
}

func (g *GameScene) updateCombatSummonProgress(progress float32) {
	if g.combatSummonEffect != nil {
		g.combatSummonEffect.Progress = progress
	}
}

func (g *GameScene) setCombatSummonPhase(phase combatSummonEffectPhase) {
	g.combatSummonEffect.Phase = phase
	g.combatSummonEffect.Progress = 0
}

// thenCastCombatRepel casts Repel Undead or Fear, which share the caster cue and
// target pulses; cast applies the spell's rule and feedback describes its result.
func (g *GameScene) thenCastCombatRepel(
	casterIdx int,
	cast func(int) game_state.CombatAnXenCorpResult,
	feedback func(game_state.CombatAnXenCorpResult) string,
) event_queue.Sequence {
	timing := g.captureCombatTiming()

	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var result game_state.CombatAnXenCorpResult

		return q.Then(g.combatTimingStep(timing, func() {
			result = cast(casterIdx)
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
			g.gameState.SystemCallbacks.Message.AddRowStr(feedback(result))
			caster := g.activeCombatCasterIdentity(casterIdx)
			g.combatRepelEffect = &combatRepelSpellEffect{
				CasterIdx:  casterIdx,
				CasterSlot: caster.Slot,
				Caster:     g.gameState.CombatScene.GetCombatantPosition(caster),
				Result:     result,
				Phase:      combatRepelCasterCue,
			}
		})).Animate(timing.duration(combatRepelCasterCueMs), func(progress float32) {
			g.combatRepelEffect.Progress = progress
		}).Then(func() {
			g.combatRepelEffect.Phase = combatRepelTargets
			g.combatRepelEffect.Progress = 0
		}).If(func() bool { return combatRepelHasTargets(result) }, func(effectQ *event_queue.SequentialEventBuilder) {
			effectQ.Animate(timing.duration(combatRepelTargetsMs), func(progress float32) {
				g.combatRepelEffect.Progress = progress
			})
		}).Then(func() {
			// The cast workflow's Finally clears this on cancellation; a Finally here
			// would replace that workflow's cleanup.
			g.combatRepelEffect = nil
		})
	}
}

func (g *GameScene) activeCombatCasterIdentity(casterIdx int) game_state.CombatTurnIdentity {
	identity := g.gameState.CombatScene.GetCurrentTurnIdentity()
	if identity.IsPartyMember && identity.EntityIndex == casterIdx {
		return identity
	}

	return game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: casterIdx}
}

func (g *GameScene) drawCombatInvocationSiteEffects(screen *ebiten.Image) {
	effect := g.combatSummonEffect
	if effect == nil || len(effect.Result.Created) == 0 || effect.Phase == combatSummonInvocation ||
		effect.Phase == combatSummonAllegiance {
		return
	}
	position := effect.Result.Created[0].Position
	if !g.gameState.CombatTerrainAllowsObjectDisplay(position) {
		return
	}
	drawOptions := g.combatEntityDrawOptions(position, false, effect.Result.Created[0].Index, 0)
	if effect.Phase == combatSummonSeal {
		drawCombatSeal(screen, drawOptions, effect.Progress, effect.Result.Spell == references.KalXenCorp)
	}

	step := combatManifestationStep(effect.Progress)
	spriteIndex := indexes.MagicField
	alpha := float32(0.22)
	if effect.Phase == combatSummonManifestation && step%2 == 1 {
		spriteIndex = indexes.Explosion
		alpha = 0.3
	}
	options := *drawOptions
	options.ColorScale.Scale(1, 0.25, 1, alpha)
	screen.DrawImage(g.spriteSheet.GetSprite(indexes.SpriteIndex(spriteIndex)), &options)
}

func combatManifestationStep(progress float32) int {
	return min(int(progress*combatManifestationSteps), combatManifestationSteps-1)
}

func drawCombatSeal(screen *ebiten.Image, options *ebiten.DrawImageOptions, progress float32, nested bool) {
	inset := float64(3 - 2*progress)
	drawCombatSealRectangle(screen, options, inset, ucolor.EGALightMagenta)
	if nested {
		drawCombatSealRectangle(screen, options, inset+3, ucolor.EGAMagenta)
	}
}

func drawCombatSealRectangle(screen *ebiten.Image, options *ebiten.DrawImageOptions, inset float64, clr color.Color) {
	left, top := options.GeoM.Apply(inset, inset)
	right, bottom := options.GeoM.Apply(sprites.TileSize-inset, sprites.TileSize-inset)
	width := combatActivePCSourcePixelScale(options)
	vector.StrokeRect(screen, float32(left), float32(top), float32(right-left), float32(bottom-top), width, ucolor.EGABlack, false)
	vector.StrokeRect(screen, float32(left), float32(top), float32(right-left), float32(bottom-top), width/2, clr, false)
}

func (g *GameScene) drawCombatInvocationActorEffects(screen *ebiten.Image) {
	if effect := g.combatSummonEffect; effect != nil {
		if effect.Phase == combatSummonInvocation && g.gameState.CombatTerrainAllowsObjectDisplay(effect.Caster) {
			drawCombatCornerCue(screen, g.combatEntityDrawOptions(effect.Caster, true, effect.CasterIdx, effect.CasterSlot),
				combatSummonCasterInset(effect.Progress), ucolor.EGALightMagenta)
		}
		if effect.Phase == combatSummonAllegiance {
			for _, created := range effect.Result.Created {
				if !g.gameState.CombatTerrainAllowsObjectDisplay(created.Position) {
					continue
				}
				drawCombatCornerCue(screen, g.combatEntityDrawOptions(created.Position, false, created.Index, 0),
					combatSummonAllegianceInset(effect.Result.Binding, effect.Progress),
					combatSummonAllegianceColor(effect.Result.Binding))
			}
		}
	}
	if effect := g.combatRepelEffect; effect != nil {
		g.drawCombatRepelActorEffects(screen, effect)
	}
}

func combatSummonCasterInset(progress float32) float32 { return progress * 5 }

func combatSummonAllegianceInset(binding game_state.CombatSummonBindingOutcome, progress float32) float32 {
	if binding == game_state.CombatSummonBindingFailed {
		return -progress * 4
	}

	return (1 - progress) * 3
}

func combatSummonAllegianceColor(binding game_state.CombatSummonBindingOutcome) ucolor.EGAColor {
	if binding == game_state.CombatSummonBindingFailed {
		return ucolor.EGALightRed
	}

	return ucolor.EGALightCyan
}

func (g *GameScene) drawCombatRepelActorEffects(screen *ebiten.Image, effect *combatRepelSpellEffect) {
	if effect.Phase == combatRepelCasterCue {
		if !g.gameState.CombatTerrainAllowsObjectDisplay(effect.Caster) {
			return
		}
		drawCombatCornerCue(screen, g.combatEntityDrawOptions(effect.Caster, true, effect.CasterIdx, effect.CasterSlot),
			(1-effect.Progress)*3, combatRepelCasterColor(effect.Progress))

		return
	}
	for _, target := range effect.Result.Affected {
		if !g.gameState.CombatTerrainAllowsObjectDisplay(target.Position) {
			continue
		}
		g.drawCombatRepelAffectedPulse(screen, target, effect.Progress)
		drawCombatCornerCue(screen, g.combatEntityDrawOptions(target.Position, false, target.Index, 0),
			combatRepelTargetInset(true, effect.Progress), combatRepelTargetColor(true))
	}
	for _, targets := range [][]game_state.CombatSpellMonster{effect.Result.Resisted, effect.Result.Immune} {
		for _, target := range targets {
			if !g.gameState.CombatTerrainAllowsObjectDisplay(target.Position) {
				continue
			}
			drawCombatCornerCue(screen, g.combatEntityDrawOptions(target.Position, false, target.Index, 0),
				combatRepelTargetInset(false, effect.Progress), combatRepelTargetColor(false))
		}
	}
}

func (g *GameScene) drawCombatRepelAffectedPulse(
	screen *ebiten.Image,
	target game_state.CombatSpellMonster,
	progress float32,
) {
	clr, visible := combatRepelPulseColor(progress)
	if !visible {
		return
	}
	monster := g.gameState.CombatScene.MonsterManager.GetMonster(target.Index)
	if monster == nil || monster.EnemyRef() == nil || monster.EnemyRef().KeyFrameTile == nil {
		return
	}
	mask := sprites.GetSpriteMask(g.spriteSheet.GetSprite(monster.EnemyRef().KeyFrameTile.Index))
	if mask == nil {
		return
	}
	options := *g.combatEntityDrawOptions(target.Position, false, target.Index, 0)
	options.ColorScale.Reset()
	options.ColorScale.ScaleWithColor(clr.ToRGBA())
	options.ColorScale.ScaleAlpha(combatRepelPulseAlpha)
	screen.DrawImage(mask, &options)
}

func combatRepelPulseColor(progress float32) (ucolor.EGAColor, bool) {
	if progress < 0 || progress >= 1 {
		return ucolor.EGAWhite, false
	}
	phase := progress * combatRepelTargetPulses
	pulseProgress := phase - float32(int(phase))
	if pulseProgress >= combatRepelPulseVisiblePart {
		return ucolor.EGAWhite, false
	}
	if pulseProgress < combatRepelPulseVisiblePart/2 {
		return ucolor.EGAWhite, true
	}

	return ucolor.EGAYellow, true
}

func combatRepelHasTargets(result game_state.CombatAnXenCorpResult) bool {
	return len(result.Affected)+len(result.Resisted)+len(result.Immune) > 0
}

func combatRepelCasterColor(progress float32) ucolor.EGAColor {
	if progress < 0.5 {
		return ucolor.EGAWhite
	}

	return ucolor.EGAYellow
}

func combatRepelTargetColor(affected bool) ucolor.EGAColor {
	if affected {
		return ucolor.EGAYellow
	}

	return ucolor.EGADarkGray
}

func combatRepelTargetInset(affected bool, progress float32) float32 {
	if affected {
		return -4 * progress
	}

	return 5 * progress
}

func (g *GameScene) combatPartyInsectStack(position references.Position) []int {
	frame := &g.combatRenderFrame
	var stacks map[references.Position][]int
	if frame.active {
		if !frame.insectStacksReady {
			frame.insectStacks = g.buildCombatPartyInsectStacks()
			frame.insectStacksReady = true
		}
		stacks = frame.insectStacks
	} else {
		stacks = g.buildCombatPartyInsectStacks()
	}
	stack := stacks[position]
	if len(stack) < 2 || len(stack) > 4 {
		return nil
	}

	return stack
}

func (g *GameScene) drawCombatMonsterStatusMarks(
	screen *ebiten.Image,
	options *ebiten.DrawImageOptions,
	monster *game_state.CombatMonster,
) {
	if monster.IsFleeing() {
		drawCombatFleeChevrons(screen, options)
	}
}

func (g *GameScene) drawCombatInsectStackIndicators(
	screen *ebiten.Image,
	options *ebiten.DrawImageOptions,
	indices []int,
) {
	activeIndex := game_state.NoActiveMonster
	if _, index := g.gameState.CombatScene.GetControlledMonster(); index >= 0 {
		activeIndex = index
	}
	for member, index := range indices {
		x, y := combatInsectStackPipPosition(member, len(indices))
		left, top := options.GeoM.Apply(x, y)
		right, bottom := options.GeoM.Apply(x+1, y+1)
		clr := ucolor.EGACyan
		if index == activeIndex {
			clr = ucolor.EGAWhite
		}
		vector.FillRect(screen, float32(left), float32(top), float32(right-left), float32(bottom-top), clr, false)
	}
}

func combatInsectStackPipPosition(member, count int) (float64, float64) {
	spacing := float64(6) / float64(max(count-1, 1))

	// The northern actor's exterior health frame reaches source row 1 of this
	// cell. Row 2 keeps every stack member's pip independently readable.
	return 9 + float64(member)*spacing, 2
}

// One upright segment per swarm member shares the mark's strip, each filling
// from the bottom, with a black gap between neighbours.
func (g *GameScene) drawCombatInsectStackHealth(screen *ebiten.Image, strip image.Rectangle, indices []int) {
	gap := max(1, strip.Dy()/combatHealthStackGapStripFraction)
	for member, index := range indices {
		monster := g.gameState.CombatScene.MonsterManager.GetMonster(index)
		segment := image.Rect(strip.Min.X+member*strip.Dx()/len(indices), strip.Min.Y,
			strip.Min.X+(member+1)*strip.Dx()/len(indices), strip.Max.Y)
		if member < len(indices)-1 {
			segment.Max.X -= gap
		}
		segment.Min.Y = segment.Max.Y - combatHealthFill(monster.CurrentHP(), monster.MaxHP(), strip.Dy())
		fillCombatHealthRect(screen, segment, combatHealthTierColor(monster.CurrentHP(), monster.MaxHP()))
	}
}

func (g *GameScene) combatInsectStackInjured(indices []int) bool {
	for _, index := range indices {
		if monster := g.gameState.CombatScene.MonsterManager.GetMonster(index); monster.CurrentHP() < monster.MaxHP() {
			return true
		}
	}

	return false
}
