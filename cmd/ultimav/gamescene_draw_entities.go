package main

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// drawAvatarOverlay renders the player avatar as an overlay on top of moongates
// This duplicates the avatar rendering from the map system but renders it post-process
// In camping mode, it renders all party members at their camping positions
// In minimap scene mode, it renders actors from the minimap.Scene (shrine, codex, endgame)
// NOTE: Death scene apparitions are rendered via generic tile dissolve system
// with RenderLayerOverFade (see drawActiveTileDissolves() in gamescene_draw_core.go)
func (g *GameScene) drawAvatarOverlay(screen *ebiten.Image) {
	// Check if we're in minimap scene mode - render actors from minimap.Scene
	if g.miniMapScene != nil {
		logging.Debugf(logging.CategoryUI, "drawAvatarOverlay: In minimap scene mode - rendering minimap actors")
		g.drawMiniMapActors(screen)
		g.drawLordBritishFinaleMoongate(screen)

		return
	}

	// Check if we're in combat mode - render monsters then party members at combat positions
	if g.gameState.IsInCombat() && g.gameState.CombatScene != nil {
		g.drawCombatFieldOverlay(screen)
		g.drawCombatConeOverlay(screen)
		g.drawCombatAdjacentSpellTargetPreview(screen)
		g.drawCombatUtilitySpellEffects(screen)
		g.drawCombatInvocationSiteEffects(screen)
		g.drawCombatFieldDispelEffects(screen)
		g.drawCombatThreatenedSelectionUnderlays(screen)
		g.drawCombatMonsterOverlay(screen)
		g.drawCombatVanishEffects(screen)
		g.drawCombatPartyOverlay(screen)
		g.drawCombatSignatureEffects(screen)
		g.drawCombatStrikeEffects(screen)
		g.drawCombatInvocationActorEffects(screen)
		g.drawCombatDivisionEffects(screen)
		g.drawCombatDirectionIndicators(screen)
		g.drawCombatHealthFrames(screen)
		g.drawCombatActiveSelection(screen)
		g.drawCombatAttackEligibility(screen)
		g.drawCombatAttackCrosshair(screen)

		return
	}

	// Check if we're in camping mode - render party members around campfire
	if g.gameState.IsInCamping() && g.gameState.CampingScene != nil {
		logging.Debugf(logging.CategoryUI, "drawAvatarOverlay: In camping mode - rendering camping party and wanderer")
		g.drawCampingPartyOverlay(screen)
		// Also render wanderer if present during camping
		g.drawWandererOverlay(screen)

		return
	}
	if g.gameState.IsInCamping() {
		logging.Debugf(logging.CategoryUI, "drawAvatarOverlay: IsInCamping=true but CampingScene=nil")
	} else if g.gameState.CampingScene != nil {
		logging.Debugf(logging.CategoryUI, "drawAvatarOverlay: CampingScene!=nil but IsInCamping=false")
	}

	// Normal mode: Avatar is always at the center of the visible screen
	// Calculate center position using same logic as moongate positioning
	// For the player avatar, relX = 0 and relY = 0 since it's centered on itself
	tileX := xTilesVisibleOnGameScreen / 2
	tileY := yTilesVisibleOnGameScreen / 2

	// Calculate exact pixel position on the unscaled map (same as regular tiles)
	// Use -1 offset for 18x18 sprites with 1-pixel border
	screenX, screenY := sprites.ConvertPosition16to18(tileX, tileY)

	// Reuse pre-allocated avatar draw options - reset and setup position
	g.avatarDrawOptions.GeoM.Reset()
	g.avatarDrawOptions.ColorScale.Reset()

	// Apply light source tinting to Avatar (same visual treatment as tiles and NPCs)
	avatarPos := g.gameState.MapState.PlayerLocation.Position
	g.applyLightTinting(avatarPos, g.avatarDrawOptions, g.getElapsedMs())

	g.avatarDrawOptions.GeoM.Translate(screenX, screenY)

	// Apply game screen scaling and positioning (same as map and moongates)
	// Create temp options for the concat operation
	tempOp := &ebiten.DrawImageOptions{}
	sprites.ApplyDrawOptionsFromPercentsForWholeScreen(tempOp, g.mapImage, g.mapScreenPlacement())

	// Apply screen shake offset to avatar if active (same as map drawing)
	if g.screenShakeOffsetX != 0.0 || g.screenShakeOffsetY != 0.0 {
		tempOp.GeoM.Translate(g.screenShakeOffsetX, g.screenShakeOffsetY)
	}

	g.avatarDrawOptions.GeoM.Concat(tempOp.GeoM)

	// Get avatar sprite using GameState API for directional sprite and special states
	// avatarPos already declared above for tinting
	presentation := g.gameState.GetAvatarSpriteAtPosition(avatarPos)

	// Apply deterministic animation timing for avatar movement/idle animations
	// Uses real-time clock for consistent behavior across systems and frame rates
	animationTime := g.getElapsedMs()

	// Avatar uses fixed animation hash (0) for smooth continuous animation
	posHash := int32(0)

	// Get animated sprite - handles directional sprites, movement animation, idle states
	avatarSprite := g.spriteSheet.GetAnimatedSprite(presentation.Sprite, posHash, animationTime)
	poisonMaskSprite := avatarSprite

	// Apply black outline to Avatar for enhanced visibility
	// Skip for background-integrated sprites (sleeping in bed, sitting in chairs, climbing ladders)
	// These sprites include the background baked in, so outlining them looks wrong
	if !presentation.BackgroundIntegrated {
		avatarSprite = g.spriteSheet.GetTileEffectProcessor().ApplyDynamicSimple(avatarSprite, sprites.BlackOutline)
	}

	// Apply teleportation dissolve effect if active
	if g.avatarDissolve.GetLevel() > 0.0 {
		g.drawAvatarOverlayWithDissolveEffect(avatarSprite, g.avatarDrawOptions, screen)
	} else {
		// Draw avatar directly to screen with proper positioning and scaling
		screen.DrawImage(avatarSprite, g.avatarDrawOptions)
		if activePlayerNeedsPoisonOverlay(g.gameState) {
			g.drawPoisonOverlay(screen, poisonMaskSprite, g.avatarDrawOptions, animationTime)
		}
	}
	g.drawLargeMapPartyFrigateHealthBar(screen)
}

func (g *GameScene) drawCombatFieldOverlay(screen *ebiten.Image) {
	combatScene := g.gameState.CombatScene
	for _, field := range combatScene.Fields() {
		if !g.gameState.CombatFieldIsDisplayed(field) ||
			(g.combatLootPreview != nil && g.combatLootPreview.position == field.Position()) {
			continue
		}
		drawOptions := g.combatSquareMarkerDrawOptions(field.Position())
		position := field.Position()
		posHash := int32(position.X)*int32(references.YCombatMapTiles) + int32(position.Y)
		sprite := g.spriteSheet.GetAnimatedSprite(field.Sprite(), posHash, g.getElapsedMs())
		screen.DrawImage(sprite, drawOptions)
	}
}

func (g *GameScene) drawLargeMapPartyFrigateHealthBar(screen *ebiten.Image) {
	if g.gameState.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType {
		return
	}

	vehicle := g.gameState.PartyVehicle.GetVehicleDetails()
	if vehicle.VehicleType != references.FrigateVehicle {
		return
	}

	drawCombatHealthBar(screen, g.avatarDrawOptions, vehicle.GetHullHP(), map_units.MaxHullHP)
}

const (
	combatSourceTilePixels   = 16
	combatSourceTileInset    = 1
	crosshairStrokePixels    = 2
	crosshairShadowPixels    = 1
	crosshairPlusArmPixels   = 3
	crosshairCornerFraction  = 0.32
	crosshairCenterDivisor   = 2
	combatMarkerBorderPixels = 1
)

// drawCombatAttackCrosshair keeps the familiar full-size cursor uniformly white.
// Eligibility and preview ownership are documented in ref:COMBAT-0012.
func (g *GameScene) drawCombatAttackCrosshair(screen *ebiten.Image) {
	var cursor references.Position
	switch {
	case g.gameState.CombatScene.SpellTargeting != nil:
		cursor = g.gameState.CombatScene.SpellTargeting.Cursor
	case g.gameState.CombatScene.AttackTargeting != nil:
		cursor = g.gameState.CombatScene.AttackTargeting.Cursor
	default:
		var active bool
		cursor, active = g.combatDoorSpellCursor()
		if !active {
			return
		}
	}
	if g.gameState.IsCombatPartySpellTargeting() {
		g.drawCombatPartySpellWakeMarker(screen)
	}

	drawCombatReticle(screen, g.combatSquareMarkerDrawOptions(cursor))
}

// Square visuals have no actor identity. Fields, footprints, utility cues and
// aim/wake markers share battlefield placement and shake, never actor motion.
func (g *GameScene) combatSquareMarkerDrawOptions(position references.Position) *ebiten.DrawImageOptions {
	x, y := g.combatLayout().positionToSourcePixels(position)
	options := &ebiten.DrawImageOptions{}
	options.GeoM.Translate(x, y)
	mapOptions := &ebiten.DrawImageOptions{}
	sprites.ApplyDrawOptionsFromPercentsForWholeScreen(mapOptions, g.mapImage, g.mapScreenPlacement())
	mapOptions.GeoM.Translate(g.screenShakeOffsetX, g.screenShakeOffsetY)
	options.GeoM.Concat(mapOptions.GeoM)

	return options
}

func combatWakeReticleFaces() [2]image.Rectangle {
	return [2]image.Rectangle{image.Rect(8, 6, 11, 7), image.Rect(9, 5, 10, 8)}
}

// Restore the original cursor's face geometry: inset 16-pixel cell, 32% corner
// arms, centred six-pixel cross and two-pixel strokes. Preserve its minimum-axis
// stroke/centre scaling and offset black shadow rather than resizing for HP.
func drawCombatReticle(screen *ebiten.Image, options *ebiten.DrawImageOptions) {
	leftPos, topPos := options.GeoM.Apply(combatSourceTileInset, combatSourceTileInset)
	rightPos, bottomPos := options.GeoM.Apply(combatSourceTileInset+combatSourceTilePixels, combatSourceTileInset+combatSourceTilePixels)
	width, height := float32(rightPos-leftPos), float32(bottomPos-topPos)
	sourcePixelScale := min(width, height) / combatSourceTilePixels
	strokeWidth := float32(crosshairStrokePixels) * sourcePixelScale
	shadowOffset := float32(crosshairShadowPixels) * sourcePixelScale
	plusArmLength := float32(crosshairPlusArmPixels) * sourcePixelScale
	cornerX, cornerY := width*crosshairCornerFraction, height*crosshairCornerFraction
	left, top, right, bottom := float32(leftPos), float32(topPos), float32(rightPos), float32(bottomPos)
	centerX, centerY := (left+right)/crosshairCenterDivisor, (top+bottom)/crosshairCenterDivisor
	drawCrosshairCorners(screen, left+shadowOffset, top+shadowOffset, right+shadowOffset, bottom+shadowOffset,
		cornerX, cornerY, strokeWidth, ucolor.CombatAimFrameBorder)
	drawCrosshairPlus(screen, centerX+shadowOffset, centerY+shadowOffset, plusArmLength, strokeWidth, ucolor.CombatAimFrameBorder)
	drawCrosshairCorners(screen, left, top, right, bottom, cornerX, cornerY, strokeWidth, ucolor.CombatAimFrameFace)
	drawCrosshairPlus(screen, centerX, centerY, plusArmLength, strokeWidth, ucolor.CombatAimFrameFace)
}

func drawCrosshairCorners(
	screen *ebiten.Image,
	left, top, right, bottom, cornerX, cornerY, strokeWidth float32,
	clr color.Color,
) {
	vector.StrokeLine(screen, left, top, left+cornerX, top, strokeWidth, clr, false)
	vector.StrokeLine(screen, left, top, left, top+cornerY, strokeWidth, clr, false)
	vector.StrokeLine(screen, right, top, right-cornerX, top, strokeWidth, clr, false)
	vector.StrokeLine(screen, right, top, right, top+cornerY, strokeWidth, clr, false)
	vector.StrokeLine(screen, left, bottom, left+cornerX, bottom, strokeWidth, clr, false)
	vector.StrokeLine(screen, left, bottom, left, bottom-cornerY, strokeWidth, clr, false)
	vector.StrokeLine(screen, right, bottom, right-cornerX, bottom, strokeWidth, clr, false)
	vector.StrokeLine(screen, right, bottom, right, bottom-cornerY, strokeWidth, clr, false)
}

func drawCrosshairPlus(screen *ebiten.Image, centerX, centerY, armLength, strokeWidth float32, clr color.Color) {
	vector.StrokeLine(screen, centerX-armLength, centerY, centerX+armLength, centerY, strokeWidth, clr, false)
	vector.StrokeLine(screen, centerX, centerY-armLength, centerX, centerY+armLength, strokeWidth, clr, false)
}

// The waking position is secondary: a coloured, outlined centre without the
// selected cursor's four white corners. This remains distinct in grayscale.
func drawCombatWakeReticle(screen *ebiten.Image, options *ebiten.DrawImageOptions) {
	for _, border := range []bool{true, false} {
		for _, face := range combatWakeReticleFaces() {
			clr := ucolor.CombatWakeMarkerFace
			if border {
				face, clr = face.Inset(-combatMarkerBorderPixels), ucolor.CombatAimFrameBorder
			}
			drawCombatMarkerRect(screen, options, face, clr)
		}
	}
}

// Transform source rectangles independently on both axes. Rounded edges (rather
// than a minimum-axis line width) keep unequal fractional scales and shake crisp.
func drawCombatMarkerRect(screen *ebiten.Image, options *ebiten.DrawImageOptions, rect image.Rectangle, clr color.Color) {
	left, top := options.GeoM.Apply(float64(rect.Min.X), float64(rect.Min.Y))
	right, bottom := options.GeoM.Apply(float64(rect.Max.X), float64(rect.Max.Y))
	left, top = math.Round(left), math.Round(top)
	right, bottom = math.Round(right), math.Round(bottom)
	vector.FillRect(screen, float32(left), float32(top), float32(right-left), float32(bottom-top), clr, false)
}

func activePCOutlinePulse(elapsedMs int64) float32 {
	return quantizedSinePulse(
		elapsedMs,
		sprites.ActivePCOutlinePulsePeriodMs,
		sprites.ActivePCOutlinePulseLow,
		sprites.ActivePCOutlinePulseHigh,
		sprites.ActivePCOutlinePulseSteps,
	)
}

func activeMonsterOutlinePulse(elapsedMs int64) float32 {
	return quantizedSinePulse(
		elapsedMs,
		sprites.ActiveMonsterOutlinePulsePeriodMs,
		sprites.ActiveMonsterOutlinePulseLow,
		sprites.ActiveMonsterOutlinePulseHigh,
		sprites.ActiveMonsterOutlinePulseSteps,
	)
}

func quantizedSinePulse(elapsedMs, periodMs int64, low, high float32, stepCount int) float32 {
	pulse := sinePulse(elapsedMs, periodMs)
	steps := float32(stepCount - 1)
	quantized := float32(math.Round(float64(pulse*steps))) / steps

	return low + (high-low)*quantized
}

func sinePulse(elapsedMs int64, periodMs int64) float32 {
	const (
		fullCircleRadians = 2 * math.Pi
		pulseNormalizer   = 2
	)
	phase := float64(elapsedMs%periodMs) / float64(periodMs)

	return float32((math.Sin(phase*fullCircleRadians-math.Pi/pulseNormalizer) + 1) / pulseNormalizer)
}

// drawCombatPartyOverlay renders all party members at their combat positions on the 11x11 map.
// Body sprites retain black outlines; the active selection draws separately above HP.
// The camera stays FIXED at the center (5,5) - actors move relative to this.
func (g *GameScene) drawCombatPartyOverlay(screen *ebiten.Image) {
	combatScene := g.gameState.CombatScene
	if combatScene == nil {
		return
	}

	animationTime := g.getElapsedMs()

	activeIdentity := combatScene.GetCurrentTurnIdentity()
	hasActivePC := combatScene.ActiveMonsterIndex == game_state.NoActiveMonster &&
		activeIdentity.IsPartyMember &&
		!combatScene.IsCombatPCPossessed(activeIdentity)
	targetPCIdx := combatScene.ActiveTargetPCIndex

	for memberIdx := range g.gameState.PartyState.GetCharacterCount() {
		if !combatScene.IsPartyMemberActive(memberIdx) {
			continue
		}
		identity := game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: memberIdx}
		if combatScene.IsCombatPCUnderground(identity) || g.combatSignatureHidesActor(identity) {
			continue
		}
		if g.combatTeleportEffect != nil && g.combatTeleportEffect.Caster == identity &&
			g.combatTeleportEffect.Phase == combatTeleportHidden {
			continue
		}

		character := g.gameState.PartyState.GetCharacter(memberIdx)
		if character.IsDead() || !character.IsInParty() {
			continue
		}

		memberPos := combatScene.GetPartyMemberPosition(memberIdx)

		g.drawCombatCharacterSprite(
			screen, character, memberPos,
			identity,
			activeIdentity, hasActivePC, animationTime,
			targetPCIdx,
		)
	}
	combatScene.ForEachPCClone(func(identity game_state.CombatTurnIdentity, position references.Position) {
		if combatScene.IsCombatPCUnderground(identity) || g.combatSignatureHidesActor(identity) {
			return
		}
		if g.combatTeleportEffect != nil && g.combatTeleportEffect.Caster == identity &&
			g.combatTeleportEffect.Phase == combatTeleportHidden {
			return
		}
		character := g.gameState.PartyState.GetCharacter(identity.EntityIndex)
		if character.IsDead() || !character.IsInParty() {
			return
		}
		g.drawCombatCharacterSprite(
			screen, character, position,
			identity, activeIdentity, hasActivePC, animationTime,
			targetPCIdx,
		)
	})
}

func combatPartyTurnIndex(activeMonsterIndex, currentPartyIndex int, pendingAdvance bool) int {
	if activeMonsterIndex != game_state.NoActiveMonster || pendingAdvance {
		return game_state.NoTargetPC
	}

	return currentPartyIndex
}

// drawCombatMonsterOverlay renders all active monsters at their combat positions on the 11x11 map.
// Invisible monsters are skipped. Selection frames identify active monsters;
// sprite silhouettes retain black outlines for terrain contrast.
// The camera stays FIXED at the center (5,5) - same as party rendering.
func (g *GameScene) drawCombatMonsterOverlay(screen *ebiten.Image) {
	combatScene := g.gameState.CombatScene
	if combatScene == nil {
		return
	}

	animationTime := g.getElapsedMs()

	combatScene.MonsterManager.ForEachActive(func(idx int, monster *game_state.CombatMonster) {
		if monster.IsInvisible() || monster.IsDead() {
			return
		}
		if g.combatSignatureHidesActor(game_state.CombatTurnIdentity{EntityIndex: idx}) {
			return
		}
		if g.combatSummonEffect != nil && g.combatSummonEffect.hidesMonster(idx) {
			return
		}

		enemyRef := monster.EnemyRef()
		if enemyRef == nil || enemyRef.KeyFrameTile == nil {
			return
		}

		monsterPos := monster.Position()
		if !g.gameState.CombatTerrainAllowsObjectDisplay(monsterPos) {
			return
		}
		stack := g.combatPartyInsectStack(monsterPos)
		if len(stack) > 0 && stack[0] != idx {
			return
		}

		// Convert combat position to the left-aligned 11x11 battlefield.
		tileX, tileY := combatPositionToViewportTile(monsterPos)

		if tileX < 0 || tileX >= xTilesVisibleOnGameScreen || tileY < 0 || tileY >= yTilesVisibleOnGameScreen {
			return
		}

		monsterDrawOptions := g.combatMonsterDrawOptions(monsterPos, idx)

		bodyTime := g.combatActorAnimationTime(game_state.CombatTurnIdentity{EntityIndex: idx}, monster.CombatantID())
		monsterSprite := g.combatMonsterSprite(monster, idx, bodyTime)
		sleepMaskSprite := monsterSprite

		if divisionOutlinePhase, isNewDuplicate := g.combatDivisionChildOutlinePhase(idx); isNewDuplicate {
			monsterSprite = g.spriteSheet.GetTileEffectProcessor().ApplyDynamicMarchingOutline(
				monsterSprite,
				divisionOutlinePhase,
			)
		} else if idx == combatScene.ActiveMonsterIndex && activeCombatPCIndicatorStyle != combatActivePCIndicatorSelectionTile {
			monsterSprite = g.spriteSheet.GetTileEffectProcessor().ApplyDynamicSimpleWithIntensity(
				monsterSprite,
				sprites.CombatActiveMonsterOutline,
				activeMonsterOutlinePulse(animationTime),
			)
		} else {
			monsterSprite = g.spriteSheet.GetTileEffectProcessor().ApplyDynamicSimple(
				monsterSprite,
				sprites.BlackOutline,
			)
		}

		screen.DrawImage(monsterSprite, monsterDrawOptions)
		g.drawCombatMonsterConditions(screen, monsterSprite, sleepMaskSprite, monsterDrawOptions, monster, animationTime)
		g.drawCombatFireImpactOverlay(screen, sleepMaskSprite, monsterDrawOptions, false, idx, 0)
	})
}

const (
	corpserExposedFrameDurationMs = 175
	corpserExposedFrameCount      = 3
)

func (g *GameScene) combatMonsterSprite(
	monster *game_state.CombatMonster,
	monsterIndex int,
	animationTime int64,
) *ebiten.Image {
	base := monster.EnemyRef().KeyFrameTile.Index
	if monster.IsAsleep() || monster.IsCorpserHidden() {
		return g.spriteSheet.GetSprite(base)
	}
	if monster.IsCorpserExposed() {
		return g.spriteSheet.GetSprite(corpserExposedSpriteIndex(monsterIndex, animationTime))
	}
	posHash := int32(monsterIndex) //nolint:gosec // Monster slots are bounded by the combat queue.

	return g.spriteSheet.GetAnimatedSprite(base, posHash, animationTime)
}

func corpserExposedSpriteIndex(monsterIndex int, animationTime int64) indexes.SpriteIndex {
	frame := int(animationTime/int64(corpserExposedFrameDurationMs)) + monsterIndex

	return indexes.Corpser2 + indexes.SpriteIndex(frame%corpserExposedFrameCount)
}

// drawCombatCharacterSprite renders a single combat party member sprite at the
// correct screen position relative to the fixed camera center.
// Body and condition pixels retain terrain admission; active arch hints are a
// separate foreground pass, and threatened intent stays in its underlay.
func (g *GameScene) drawCombatCharacterSprite(
	screen *ebiten.Image,
	character *party_state.PlayerCharacter,
	memberPos references.Position,
	identity, activeIdentity game_state.CombatTurnIdentity,
	hasActivePC bool,
	animationTime int64,
	targetPCIndex int,
) {
	if !g.gameState.CombatTerrainAllowsObjectDisplay(memberPos) {
		return
	}
	memberIndex := identity.EntityIndex
	// Convert combat position to the left-aligned 11x11 battlefield.
	tileX, tileY := combatPositionToViewportTile(memberPos)

	if tileX < 0 || tileX >= xTilesVisibleOnGameScreen || tileY < 0 || tileY >= yTilesVisibleOnGameScreen {
		return
	}

	partyDrawOptions := g.combatPartyIdentityDrawOptions(memberPos, identity)

	characterSprite := g.combatPartyBodySprite(character, identity)
	poisonMaskSprite := characterSprite
	isActive := hasActivePC && identity == activeIdentity

	// Target intent is carried by the red under-tile frame, leaving sprites unobscured.
	switch {
	case memberIndex == targetPCIndex:
		characterSprite = g.spriteSheet.GetTileEffectProcessor().ApplyDynamicSimple(
			characterSprite,
			sprites.CombatInactivePCOutline,
		)
	case isActive:
		characterSprite = g.applyCombatActivePCIndicator(characterSprite, animationTime)
	default:
		characterSprite = g.spriteSheet.GetTileEffectProcessor().ApplyDynamicSimple(
			characterSprite,
			sprites.CombatInactivePCOutline,
		)
	}

	screen.DrawImage(characterSprite, partyDrawOptions)
	g.drawCombatPartyConditions(screen, poisonMaskSprite, partyDrawOptions, identity, animationTime)
	g.drawCombatFireImpactOverlay(screen, poisonMaskSprite, partyDrawOptions, true, memberIndex, identity.Slot)
	if isActive {
		drawCombatActivePCIndicatorOverlay(screen, partyDrawOptions, animationTime)
	}
	g.drawCombatSceptreActorEffects(screen, identity, partyDrawOptions)
}

// Body and arch contour select the same unoutlined frame using exact-entry
// conditions and the settled body clock (including Time Stop).
func (g *GameScene) combatPartyBodySprite(
	character *party_state.PlayerCharacter,
	identity game_state.CombatTurnIdentity,
) *ebiten.Image {
	combat := g.gameState.CombatScene
	if combat.IsCombatPCSleeping(identity) {
		return g.spriteSheet.GetSprite(indexes.DeadBody)
	}
	spriteIndex := combatPartySpriteIndex(character, combat.IsCombatPCInvisible(identity), combat.IsCombatPCRat(identity))
	// Signature composition owns a static current-condition frame while its
	// fade/mask is active. The arch contour must use that same silhouette.
	if spriteIndex == indexes.InvisiblePlayer || g.combatSignatureHidesActor(identity) {
		return g.spriteSheet.GetSprite(spriteIndex)
	}
	posHash := int32(identity.EntityIndex) //nolint:gosec // Party roster indices are bounded to 0-5.

	return g.spriteSheet.GetAnimatedSprite(spriteIndex, posHash, g.combatActorAnimationTime(identity, 0))
}

func combatPartySpriteIndex(character *party_state.PlayerCharacter, invisible, rat bool) indexes.SpriteIndex {
	if invisible {
		return indexes.InvisiblePlayer
	}
	if rat {
		return indexes.RatKeyIndex
	}

	return character.GetKeySpriteIndex()
}

func (g *GameScene) combatPartyDrawOptions(
	memberPosition references.Position,
	memberIndex int,
) *ebiten.DrawImageOptions {
	return g.combatEntityDrawOptions(memberPosition, true, memberIndex, 0)
}

func (g *GameScene) combatPartyIdentityDrawOptions(
	memberPosition references.Position,
	identity game_state.CombatTurnIdentity,
) *ebiten.DrawImageOptions {
	return g.combatEntityDrawOptions(memberPosition, true, identity.EntityIndex, identity.Slot)
}

func (g *GameScene) combatMonsterDrawOptions(
	monsterPosition references.Position,
	monsterIndex int,
) *ebiten.DrawImageOptions {
	return g.combatEntityDrawOptions(monsterPosition, false, monsterIndex, 0)
}

func (g *GameScene) combatEntityDrawOptions(
	position references.Position,
	isPartyMember bool,
	entityIndex, slot int,
) *ebiten.DrawImageOptions {
	screenX, screenY := g.combatLayout().positionToSourcePixels(position)
	drawOptions := &ebiten.DrawImageOptions{}
	drawOptions.GeoM.Translate(screenX, screenY)
	impactX, impactY := g.combatEntityOffset(isPartyMember, entityIndex, slot)
	drawOptions.GeoM.Translate(impactX, impactY)

	tempOptions := &ebiten.DrawImageOptions{}
	sprites.ApplyDrawOptionsFromPercentsForWholeScreen(tempOptions, g.mapImage, g.mapScreenPlacement())
	if g.screenShakeOffsetX != 0.0 || g.screenShakeOffsetY != 0.0 {
		tempOptions.GeoM.Translate(g.screenShakeOffsetX, g.screenShakeOffsetY)
	}
	drawOptions.GeoM.Concat(tempOptions.GeoM)

	return drawOptions
}

// drawCampingPartyOverlay renders all party members at their camping positions around the campfire
func (g *GameScene) drawCampingPartyOverlay(screen *ebiten.Image) {
	// Get party state and camping scene
	campingScene := g.gameState.CampingScene
	if campingScene == nil {
		return
	}

	// Animation timing for sprites
	animationTime := g.getElapsedMs()

	// Render each active party member at their camping position
	for i := range g.gameState.PartyState.GetCharacterCount() {
		character := g.gameState.PartyState.GetCharacter(i)
		// Skip dead characters
		if character.IsDead() || !character.IsInParty() {
			continue
		}

		// Get the party member's camping position
		campingPos := campingScene.GetPartyMemberPosition(i)

		// Convert camping position (0-10) to screen tile position
		// Camping map is 11x11 centered, so we need to offset properly
		centerOffset := 5 // Center of 11x11 map
		tileX := int(campingPos.X) - centerOffset + (xTilesVisibleOnGameScreen / 2)
		tileY := int(campingPos.Y) - centerOffset + (yTilesVisibleOnGameScreen / 2)

		// Skip if position is off-screen
		if tileX < 0 || tileX >= xTilesVisibleOnGameScreen || tileY < 0 || tileY >= yTilesVisibleOnGameScreen {
			continue
		}

		// Calculate screen pixel position
		screenX, screenY := sprites.ConvertPosition16to18(tileX, tileY)

		// Setup draw options
		partyDrawOptions := &ebiten.DrawImageOptions{}
		partyDrawOptions.GeoM.Translate(screenX, screenY)

		// Apply game screen scaling and positioning
		tempOp := &ebiten.DrawImageOptions{}
		sprites.ApplyDrawOptionsFromPercentsForWholeScreen(tempOp, g.mapImage, g.mapScreenPlacement())

		// Apply screen shake offset if active
		if g.screenShakeOffsetX != 0.0 || g.screenShakeOffsetY != 0.0 {
			tempOp.GeoM.Translate(g.screenShakeOffsetX, g.screenShakeOffsetY)
		}

		partyDrawOptions.GeoM.Concat(tempOp.GeoM)

		// Determine character sprite based on overrides first, then guard status and sleep state
		var characterSprite *ebiten.Image
		var spriteIndex indexes.SpriteIndex

		// Check for sprite overrides first (priority system)
		entityID := fmt.Sprintf("party_%d", i)
		if campingScene.GuardIndex == i {
			entityID = "guard" // Special identifier for the current guard
		}

		if override, exists := g.gameState.SpriteOverrides.Get("camping", entityID); exists {
			// Use override sprite
			spriteIndex = override
			posHash := int32(i) // Unique hash per character
			characterSprite = g.spriteSheet.GetAnimatedSprite(spriteIndex, posHash, animationTime)
		} else if campingScene.GuardIndex == i {
			// This character is the guard - show standing alert (fallback)
			spriteIndex = g.gameState.GetGuardSpriteForCamping(character, i)
			posHash := int32(i) // Unique hash per character
			characterSprite = g.spriteSheet.GetAnimatedSprite(spriteIndex, posHash, animationTime)
		} else {
			// This character is sleeping - show sleeping sprite (fallback)
			if character.IsAsleep() {
				spriteIndex = g.gameState.GetCharacterSpriteForCamping()
			} else {
				spriteIndex = character.GetKeySpriteIndex()
			}
			posHash := int32(i) // Unique hash per character
			characterSprite = g.spriteSheet.GetAnimatedSprite(spriteIndex, posHash, animationTime)
		}

		poisonMaskSprite := characterSprite

		// Both guards and sleepers use the same outline; retain the original
		// sprite above as the poison mask before applying the outline.
		characterSprite = g.spriteSheet.GetTileEffectProcessor().ApplyDynamicSimple(characterSprite, sprites.BlackOutline)

		// Draw the character sprite
		screen.DrawImage(characterSprite, partyDrawOptions)
		if character.IsPoisoned() {
			g.drawPoisonOverlay(screen, poisonMaskSprite, partyDrawOptions, animationTime)
		}
	}
}

// drawWandererOverlay renders the wanderer sprite during camping if present
func (g *GameScene) drawWandererOverlay(screen *ebiten.Image) {
	// Only render during camping mode
	campingScene := g.gameState.CampingScene
	if campingScene == nil || !campingScene.WandererPresent {
		logging.Debugf(logging.CategoryUI, "drawWandererOverlay: Skipping - campingScene=%v, wandererPresent=%v",
			campingScene != nil, campingScene != nil && campingScene.WandererPresent)

		return
	}

	logging.Debugf(logging.CategoryUI, "drawWandererOverlay: Rendering wanderer with dissolveLevel=%.2f", g.wandererDissolve.GetLevel())

	// Get wanderer position on camping map
	wandererPos := campingScene.GetWandererPosition()
	logging.Debugf(logging.CategoryUI, "drawWandererOverlay: Wanderer position (%d,%d)", wandererPos.X, wandererPos.Y)

	// Convert camping position to screen tile position
	centerOffset := 5 // Center of 11x11 camping map
	tileX := int(wandererPos.X) - centerOffset + (xTilesVisibleOnGameScreen / 2)
	tileY := int(wandererPos.Y) - centerOffset + (yTilesVisibleOnGameScreen / 2)

	logging.Debugf(logging.CategoryUI, "drawWandererOverlay: Screen tile position (%d,%d), visible area (%dx%d)",
		tileX, tileY, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)

	// Skip if position is off-screen
	if tileX < 0 || tileX >= xTilesVisibleOnGameScreen || tileY < 0 || tileY >= yTilesVisibleOnGameScreen {
		logging.Debugf(logging.CategoryUI, "drawWandererOverlay: Position off-screen, skipping")

		return
	}

	// Calculate screen pixel position
	screenX, screenY := sprites.ConvertPosition16to18(tileX, tileY)
	logging.Debugf(logging.CategoryUI, "drawWandererOverlay: Screen pixel position (%.1f,%.1f)", screenX, screenY)

	// Setup draw options
	wandererDrawOptions := &ebiten.DrawImageOptions{}
	wandererDrawOptions.GeoM.Translate(screenX, screenY)

	// Apply game screen scaling and positioning
	tempOp := &ebiten.DrawImageOptions{}
	sprites.ApplyDrawOptionsFromPercentsForWholeScreen(tempOp, g.mapImage, g.mapScreenPlacement())

	// Apply screen shake offset if active
	if g.screenShakeOffsetX != 0.0 || g.screenShakeOffsetY != 0.0 {
		tempOp.GeoM.Translate(g.screenShakeOffsetX, g.screenShakeOffsetY)
	}

	wandererDrawOptions.GeoM.Concat(tempOp.GeoM)

	// Get wanderer sprite (using Wizard1 sprite)
	animationTime := g.getElapsedMs()
	posHash := int32(999) // Unique hash for wanderer
	wandererSprite := g.spriteSheet.GetAnimatedSprite(indexes.Apparation1, posHash, animationTime)

	if wandererSprite == nil {
		logging.Debugf(logging.CategoryUI, "drawWandererOverlay: Failed to load Wizard1 sprite")

		return
	}

	logging.Debugf(logging.CategoryUI, "drawWandererOverlay: Loaded Wizard1 sprite successfully")

	// Apply magical blue outline for mystical appearance
	wandererSprite = g.spriteSheet.GetTileEffectProcessor().ApplyDynamicSimple(wandererSprite, sprites.BlueOutline)
	logging.Debugf(logging.CategoryUI, "drawWandererOverlay: Applied blue outline")

	// Apply dissolve effect if active
	if g.wandererDissolve.GetLevel() > 0.0 {
		logging.Debugf(logging.CategoryUI, "drawWandererOverlay: Applying dissolve effect level %.2f", g.wandererDissolve.GetLevel())
		g.drawWandererWithDissolveEffect(wandererSprite, wandererDrawOptions, screen)
	} else {
		logging.Debugf(logging.CategoryUI, "drawWandererOverlay: Drawing wanderer directly (no dissolve)")
		// Draw wanderer directly to screen
		screen.DrawImage(wandererSprite, wandererDrawOptions)
	}
}

// Note: drawDeathSceneApparitions() and drawDeathApparition() removed
// Death scene apparitions now rendered via generic tile dissolve system
// See gamescene_death_sequence.go: StartTileDissolveWithOptions() with PositionModeFixedGrid + RenderLayerOverFade

// drawSpriteWithDissolveEffect applies GPU-accelerated dissolve effect to ANY sprite.
// This is a generic function used by avatar, wanderer, and any future dissolving entities.
// Zero per-frame allocations - reuses persistent buffers.
func (g *GameScene) drawSpriteWithDissolveEffect(
	sprite *ebiten.Image,
	dissolveState *DissolveEffectState,
	drawOptions *ebiten.DrawImageOptions,
	screen *ebiten.Image,
) {
	screen.DrawImage(spriteWithDissolveMask(sprite, dissolveState), drawOptions)
}

// Share the existing persistent mask buffer with split signature rendering.
func spriteWithDissolveMask(sprite *ebiten.Image, dissolveState *DissolveEffectState) *ebiten.Image {
	// If dissolve effect is not active, draw sprite normally
	if !dissolveState.IsActive() {
		return sprite
	}

	// Reuse persistent masked buffer (eliminates per-frame allocations)
	// Check if buffer needs recreation due to sprite size change
	spriteWidth, spriteHeight := sprite.Bounds().Dx(), sprite.Bounds().Dy()
	if dissolveState.buffer == nil ||
		dissolveState.buffer.Bounds().Dx() != spriteWidth ||
		dissolveState.buffer.Bounds().Dy() != spriteHeight {
		dissolveState.buffer = ebiten.NewImage(spriteWidth, spriteHeight)
	}

	// Clear buffer for fresh rendering
	dissolveState.buffer.Clear()

	// Copy the original sprite to masked buffer
	dissolveState.buffer.DrawImage(sprite, &ebiten.DrawImageOptions{})

	// Apply GPU-accelerated mask using BlendDestinationIn
	// This preserves sprite RGB values while using mask alpha for visibility (dissolve effect)
	// Formula: sprite.color × mask.alpha = correct pixel colors with progressive reveal/hide
	maskOp := &ebiten.DrawImageOptions{}
	maskOp.Blend = ebiten.BlendDestinationIn
	dissolveState.buffer.DrawImage(dissolveState.effect.GetMask(), maskOp)

	return dissolveState.buffer
}

// drawWandererWithDissolveEffect renders the wanderer with GPU-accelerated dissolve effect.
// Uses the generic drawSpriteWithDissolveEffect() function.
func (g *GameScene) drawWandererWithDissolveEffect(wandererSprite *ebiten.Image, drawOptions *ebiten.DrawImageOptions, screen *ebiten.Image) {
	// Ensure dissolve effect is initialized
	if !g.wandererDissolve.IsActive() {
		logging.Warnf(logging.CategoryUI, "drawWandererWithDissolveEffect: Dissolve effect not initialized, initializing now")
		g.initializeWandererDissolveEffect()
		if !g.wandererDissolve.IsActive() {
			// Still not active - initialization failed, draw without dissolve
			logging.Warnf(logging.CategoryUI, "drawWandererWithDissolveEffect: Failed to initialize dissolve effect, drawing without dissolve")
			screen.DrawImage(wandererSprite, drawOptions)

			return
		}
	}

	// Use generic dissolve rendering function
	g.drawSpriteWithDissolveEffect(wandererSprite, &g.wandererDissolve, drawOptions, screen)
}

// drawAvatarOverlayWithDissolveEffect renders the avatar with GPU-accelerated dissolve effect.
// Uses the generic drawSpriteWithDissolveEffect() function.
func (g *GameScene) drawAvatarOverlayWithDissolveEffect(avatarSprite *ebiten.Image, drawOptions *ebiten.DrawImageOptions, screen *ebiten.Image) {
	// Ensure dissolve effect is initialized
	if !g.avatarDissolve.IsActive() {
		logging.Warnf(logging.CategoryUI, "drawAvatarOverlayWithDissolveEffect: Dissolve effect not initialized, drawing without dissolve")
		screen.DrawImage(avatarSprite, drawOptions)

		return
	}

	// Use generic dissolve rendering function
	g.drawSpriteWithDissolveEffect(avatarSprite, &g.avatarDissolve, drawOptions, screen)
}

// drawActiveTileDissolves renders all active tile dissolve effects for a specific render layer.
// Used for generic tile materialization/dematerialization (death scene, camping wanderer, item spawning).
//
// Parameters:
//   - screen: Target screen to render to
//   - layer: Which render layer to draw (RenderLayerDefault = before fade, RenderLayerOverFade = after fade)
func (g *GameScene) drawActiveTileDissolves(screen *ebiten.Image, layer TileDissolveRenderLayer) {
	if len(g.activeTileDissolves) == 0 {
		return
	}

	playerPos := g.gameState.MapState.PlayerLocation.Position
	animationTime := g.getElapsedMs()

	for i := range g.activeTileDissolves {
		state := &g.activeTileDissolves[i]

		// Filter by render layer
		if state.RenderLayer != layer {
			continue
		}

		if !state.Effect.IsActive() {
			continue
		}

		logging.Debugf(logging.CategoryUI, "drawActiveTileDissolves: ID=%d layer=%d pos=(%d,%d) posMode=%d",
			state.ID, state.RenderLayer, state.Position.X, state.Position.Y, state.PositionMode)

		var tileX, tileY int

		switch state.PositionMode {
		case PositionModeWorld:
			// World coordinates: position relative to player
			relX := int(state.Position.X - playerPos.X)
			relY := int(state.Position.Y - playerPos.Y)
			if g.terrainDither.enabled {
				relative := g.ditherRelativePosition(state.Position)
				relX, relY = int(relative.X), int(relative.Y)
			}
			tileX = (xTilesVisibleOnGameScreen / 2) + relX
			tileY = (yTilesVisibleOnGameScreen / 2) + relY

		case PositionModeFixedGrid:
			// Fixed 11x11 grid: position is absolute on centered grid (death scene, camping)
			// Position (0,0) to (10,10) maps to screen tiles centered on visible area
			// The 11x11 grid should be centered, so tile (5,5) is at screen center
			const gridSize = 11
			centerOffset := gridSize / 2 // 5
			tileX = int(state.Position.X) - centerOffset + (xTilesVisibleOnGameScreen / 2)
			tileY = int(state.Position.Y) - centerOffset + (yTilesVisibleOnGameScreen / 2)
		}

		worldPos := references.Position{
			X: playerPos.X + references.Coordinate(tileX) - xCenter,
			Y: playerPos.Y + references.Coordinate(tileY) - yCenter,
		}
		if !g.ditherAllowsWorldEffect(worldPos) {
			continue
		}

		// Skip if position is off-screen
		if tileX < 0 || tileX >= xTilesVisibleOnGameScreen || tileY < 0 || tileY >= yTilesVisibleOnGameScreen {
			continue
		}

		// Calculate screen pixel position
		screenX, screenY := sprites.ConvertPosition16to18(tileX, tileY)

		// Setup draw options
		drawOptions := &ebiten.DrawImageOptions{}
		drawOptions.GeoM.Translate(screenX, screenY)

		// Apply game screen scaling and positioning
		tempOp := &ebiten.DrawImageOptions{}
		sprites.ApplyDrawOptionsFromPercentsForWholeScreen(tempOp, g.mapImage, g.mapScreenPlacement())

		// Apply screen shake offset if active
		if g.terrainDither.enabled {
			tempOp = g.worldDrawOptions()
		} else if g.screenShakeOffsetX != 0.0 || g.screenShakeOffsetY != 0.0 {
			tempOp.GeoM.Translate(g.screenShakeOffsetX, g.screenShakeOffsetY)
		}

		drawOptions.GeoM.Concat(tempOp.GeoM)

		// Get animated sprite for this tile
		// Animation variation intentionally wraps the ID product to its low 32 bits.
		posHash := int32(state.ID * 12345) //nolint:gosec // Cosmetic hash truncation preserves its established animation phase.
		tileSprite := g.spriteSheet.GetAnimatedSprite(state.SpriteIndex, posHash, animationTime)

		if tileSprite == nil {
			logging.Warnf(logging.CategoryUI, "drawActiveTileDissolves: Failed to get sprite %d", state.SpriteIndex)

			continue
		}

		// Apply outline if configured (uses OutlineColorType for flexibility)
		if state.OutlineColor != sprites.NoOutline {
			tileSprite = g.spriteSheet.GetTileEffectProcessor().ApplyDynamicSimple(tileSprite, state.OutlineColor)
		}

		// Apply dissolve effect
		g.drawSpriteWithDissolveEffect(tileSprite, &state.Effect, drawOptions, screen)
	}
}

// Note: drawDeathSceneSpritesOverFade() removed - death scene apparitions now rendered
// via generic tile dissolve system with RenderLayerOverFade in drawActiveTileDissolves()

// drawMiniMapActors renders actors from the minimap.Scene onto the screen
// This is used for scripted cutscenes (shrine, codex, endgame) where actors
// move according to predefined scripts with the camera staying fixed.
// The scene's camera center determines which part of the grid is visible.
func (g *GameScene) drawMiniMapActors(screen *ebiten.Image) {
	if g.miniMapScene == nil {
		return
	}

	// Get visible actors and camera center
	actors := g.miniMapScene.GetVisibleActors()
	cameraCenter := g.miniMapScene.GetCameraCenter()

	// Animation timing for sprites
	animationTime := g.getElapsedMs()

	// 11x11 viewport size (same as shrine/camping maps)
	const viewportSize = 11

	for _, actor := range actors {
		// Convert actor grid position to screen tile position relative to camera
		screenTileX, screenTileY := minimap.GridToScreenTile(actor.Position, cameraCenter, viewportSize)

		// Check if actor is visible on the 11x11 viewport
		if !minimap.IsOnScreen(actor.Position, cameraCenter, viewportSize) {
			continue
		}

		// Convert 11x11 viewport tile to actual screen tile (center on screen)
		// The 11x11 minimap viewport centers in the larger visible game screen
		centerOffsetX := (xTilesVisibleOnGameScreen - viewportSize) / 2
		centerOffsetY := (yTilesVisibleOnGameScreen - viewportSize) / 2
		tileX := screenTileX + centerOffsetX
		tileY := screenTileY + centerOffsetY

		// Calculate screen pixel position
		screenX, screenY := sprites.ConvertPosition16to18(tileX, tileY)

		// Setup draw options
		actorDrawOptions := &ebiten.DrawImageOptions{}
		actorDrawOptions.GeoM.Translate(screenX, screenY)

		// Apply game screen scaling and positioning
		tempOp := &ebiten.DrawImageOptions{}
		sprites.ApplyDrawOptionsFromPercentsForWholeScreen(tempOp, g.mapImage, g.mapScreenPlacement())

		// Apply screen shake offset if active
		if g.screenShakeOffsetX != 0.0 || g.screenShakeOffsetY != 0.0 {
			tempOp.GeoM.Translate(g.screenShakeOffsetX, g.screenShakeOffsetY)
		}

		actorDrawOptions.GeoM.Concat(tempOp.GeoM)

		// Finale furniture is rendered as the original terrain-integrated actor composite.
		actorSpriteIndex := g.miniMapActorSpriteForRender(actor)

		// Get animated sprite for actor
		posHash := actor.Position.AnimationHash()
		actorSprite := g.spriteSheet.GetAnimatedSprite(actorSpriteIndex, posHash, animationTime)

		// Draw actor to screen
		g.drawTilePaletteSprite(screen, actorSprite, actorDrawOptions, actorSpriteIndex)
	}
}

func (g *GameScene) miniMapActorSpriteForRender(actor *minimap.Actor) indexes.SpriteIndex {
	if finaleTile, ok := g.gameState.LordBritishFinaleTile(actor.Position); ok {
		return g.gameState.GetNPCSpriteReplacement(finaleTile, actor.Sprite, actor.Position).Sprite
	}

	return actor.Sprite
}
