package main

import (
	"image"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

const (
	transientFeedbackLimit             = 3 // Default for scenes without configuration (focused fixtures).
	transientFeedbackMoveDurationMs    = int64(180)
	transientFeedbackGapPixelUnits     = 2
	transientFeedbackPaddingPixelUnits = 2
	transientFeedbackNormalCenter      = 0.70
	transientFeedbackDungeonCenter     = 0.85
	transientFeedbackMapCenter         = 0.37
	transientFeedbackLifetimeMs        = widgets.ActionFeedbackDisplayMs + widgets.ActionFeedbackFadeOutMs
)

type transientFeedbackLayoutMode uint8

const (
	transientFeedbackSurface transientFeedbackLayoutMode = iota
	transientFeedbackCombat
	transientFeedbackDungeon
)

type transientFeedbackLayoutContext struct {
	screen          image.Rectangle
	available       image.Rectangle
	preferredCenter image.Point
	gap             int
	mode            transientFeedbackLayoutMode
}

type transientFeedbackMotion struct {
	fromX, fromY float64
	toX, toY     float64
	startedAt    int64
	endsAt       int64
}

func (m transientFeedbackMotion) activeAt(now int64) bool {
	return m.endsAt > m.startedAt && now < m.endsAt && (m.fromX != m.toX || m.fromY != m.toY)
}

func (m transientFeedbackMotion) sample(now int64) (float64, float64) {
	if m.endsAt <= m.startedAt || now >= m.endsAt {
		return m.toX, m.toY
	}
	if now <= m.startedAt {
		return m.fromX, m.fromY
	}
	progress := float64(now-m.startedAt) / float64(m.endsAt-m.startedAt)
	remaining := 1 - progress
	eased := 1 - remaining*remaining*remaining

	return m.fromX + (m.toX-m.fromX)*eased, m.fromY + (m.toY-m.fromY)*eased
}

// solveTransientFeedbackTops places newest-first cards downward, then applies
// one common bottom clamp. The caller evicts until all but an oversized
// singleton fit.
func solveTransientFeedbackTops(available image.Rectangle, preferredCenterY, gap int, heights []int) []int {
	if len(heights) == 0 {
		return nil
	}
	if available.Empty() || gap < 0 {
		panic("solveTransientFeedbackTops: invalid layout bounds")
	}
	groupHeight := gap * (len(heights) - 1)
	for _, height := range heights {
		if height <= 0 {
			panic("solveTransientFeedbackTops: card height must be positive")
		}
		groupHeight += height
	}

	top := preferredCenterY - heights[0]/2
	if groupHeight > available.Dy() {
		if len(heights) != 1 {
			panic("solveTransientFeedbackTops: oversized groups must be evicted first")
		}
		top = available.Min.Y
	} else {
		top = max(available.Min.Y, min(top, available.Max.Y-groupHeight))
	}

	tops := make([]int, len(heights))
	for index, height := range heights {
		tops[index] = top
		top += height + gap
	}

	return tops
}

func (g *GameScene) admitTransientActionFeedback(dialog *widgets.MessageDialog, now int64) {
	if dialog == nil || g.gameState == nil {
		panic("GameScene.admitTransientActionFeedback: dialog and game state are required")
	}
	g.reconcileTransientActionFeedback(now, false)
	g.retireExternallyRemovedActionFeedback()
	g.retireStaleCombatChestFeedback()
	if g.actionFeedbackLimit() == 0 {
		g.clearTransientActionFeedback()
		dialog.HideWithoutAnimation()

		return
	}

	feedback := &transientActionFeedback{
		dialog: dialog, startedAt: now, phase: transientFeedbackFadingIn,
	}
	dialog.UseAnimationClock(func() int64 { return feedback.animationMs })
	dialog.Show()
	g.dialogStack.PushNonBlockingDialog(dialog)
	g.transientActionFeedback = append(g.transientActionFeedback, feedback)
	g.layoutTransientActionFeedback(now, g.currentTransientFeedbackLayout())
}

// actionFeedbackLimit reads only the successfully applied preference. An explicit
// zero is Off; only an absent configuration uses the historical default.
func (g *GameScene) actionFeedbackLimit() int {
	if g.gameConfig == nil {
		return transientFeedbackLimit
	}

	return g.gameConfig.GetMaxVisibleActionPopups()
}

// reconcileTransientFeedbackCapacity runs independently of admission, geometry
// changes and modal-paused lifetimes. The slice is in oldest-first arrival order.
func (g *GameScene) reconcileTransientFeedbackCapacity() bool {
	limit := g.actionFeedbackLimit()
	removed := false
	for len(g.transientActionFeedback) > limit {
		g.evictTransientActionFeedback(0)
		removed = true
	}

	return removed
}

func (g *GameScene) updateTransientActionFeedback() {
	if g.gameState == nil {
		return
	}
	now := g.gameState.ElapsedMs
	g.reconcileTransientActionFeedback(now, true)
	removed := g.retireExternallyRemovedActionFeedback()
	removed = g.retireStaleCombatChestFeedback() || removed
	removed = g.reconcileTransientFeedbackCapacity() || removed
	if len(g.transientActionFeedback) == 0 {
		g.clearTransientActionFeedback()

		return
	}
	context := g.currentTransientFeedbackLayout()
	if removed || !g.transientActionFeedbackLayoutSet || context != g.transientActionFeedbackLayout {
		g.layoutTransientActionFeedback(now, context)
	}
}

// reconcileTransientActionFeedback advances finite motion/lifecycle boundaries
// chronologically so a large elapsed jump matches subdivided updates.
func (g *GameScene) reconcileTransientActionFeedback(now int64, layoutAfterRemoval bool) {
	if !g.transientActionFeedbackClockSet {
		g.transientActionFeedbackAt = now
		g.transientActionFeedbackClockSet = true
	}
	if now < g.transientActionFeedbackAt {
		panic("GameScene.reconcileTransientActionFeedback: elapsed time moved backwards")
	}
	for _, feedback := range g.transientActionFeedback {
		validateTransientActionFeedback(feedback, now)
	}

	cursor := g.transientActionFeedbackAt
	for {
		if g.applyTransientFeedbackLifecycle() && layoutAfterRemoval && len(g.transientActionFeedback) > 0 &&
			g.transientActionFeedbackLayoutSet {
			g.layoutTransientActionFeedback(cursor, g.transientActionFeedbackLayout)
		}
		if cursor == now {
			break
		}

		next := now
		for _, feedback := range g.transientActionFeedback {
			if g.actionFeedbackBelowModal(feedback.dialog) {
				continue
			}
			if feedback.protectedUntil > cursor {
				next = min(next, feedback.protectedUntil)

				continue
			}
			boundary := transientFeedbackPhaseBoundary(feedback.phase)
			if feedback.consumedMs < boundary {
				next = min(next, cursor+boundary-feedback.consumedMs)
			}
		}
		if next <= cursor {
			panic("GameScene.reconcileTransientActionFeedback: non-advancing event boundary")
		}
		g.advanceTransientFeedbackInterval(cursor, next)
		cursor = next
	}
	g.transientActionFeedbackAt = now
}

func (g *GameScene) advanceTransientFeedbackInterval(from, to int64) {
	for _, feedback := range g.transientActionFeedback {
		if g.actionFeedbackBelowModal(feedback.dialog) {
			continue
		}
		feedback.samplePosition(to)
		if from >= feedback.protectedUntil {
			feedback.consumedMs += to - from
		}
		feedback.animationMs = feedback.consumedMs
		feedback.dialog.SetPresentationPosition(
			feedback.x, feedback.y, g.transientActionFeedbackLayout.available,
		)
	}
}

func (g *GameScene) actionFeedbackBelowModal(dialog *widgets.MessageDialog) bool {
	modal := g.dialogStack.PeekTopModalDialog()
	if modal == nil {
		return false
	}
	for _, widget := range g.dialogStack.Dialogs() {
		if widget == modal {
			return false
		}
		if widget == dialog {
			return true
		}
	}

	return false
}

func (g *GameScene) applyTransientFeedbackLifecycle() bool {
	removed := false
	writeIndex := 0
	for _, feedback := range g.transientActionFeedback {
		for {
			switch {
			case feedback.phase == transientFeedbackFadingIn &&
				feedback.consumedMs >= widgets.ActionFeedbackFadeInMs:
				feedback.phase = transientFeedbackHolding
			case feedback.phase == transientFeedbackHolding &&
				feedback.consumedMs >= widgets.ActionFeedbackDisplayMs:
				feedback.animationMs = widgets.ActionFeedbackDisplayMs
				feedback.dialog.Hide()
				feedback.phase = transientFeedbackFadingOut
			default:
				goto transitioned
			}
		}
	transitioned:
		if feedback.phase == transientFeedbackFadingOut && feedback.consumedMs >= transientFeedbackLifetimeMs {
			delete(g.combatChestFeedback, feedback)
			g.dialogStack.RemoveWidgetInstance(feedback.dialog)
			feedback.dialog.HideWithoutAnimation()
			removed = true

			continue
		}
		g.transientActionFeedback[writeIndex] = feedback
		writeIndex++
	}
	clear(g.transientActionFeedback[writeIndex:])
	g.transientActionFeedback = g.transientActionFeedback[:writeIndex]

	return removed
}

func transientFeedbackPhaseBoundary(phase transientFeedbackPhase) int64 {
	switch phase {
	case transientFeedbackFadingIn:
		return widgets.ActionFeedbackFadeInMs
	case transientFeedbackHolding:
		return widgets.ActionFeedbackDisplayMs
	case transientFeedbackFadingOut:
		return transientFeedbackLifetimeMs
	default:
		panic("transientFeedbackPhaseBoundary: invalid lifecycle phase")
	}
}

func validateTransientActionFeedback(feedback *transientActionFeedback, now int64) {
	if feedback == nil || feedback.dialog == nil {
		panic("GameScene.reconcileTransientActionFeedback: invalid owned feedback")
	}
	if now < feedback.startedAt || feedback.consumedMs < 0 || feedback.consumedMs > transientFeedbackLifetimeMs {
		panic("GameScene.reconcileTransientActionFeedback: invalid feedback time")
	}
	if feedback.phase < transientFeedbackFadingIn || feedback.phase > transientFeedbackFadingOut {
		panic("GameScene.reconcileTransientActionFeedback: invalid lifecycle phase")
	}
}

func (feedback *transientActionFeedback) samplePosition(now int64) {
	if !feedback.positioned || !feedback.motion.activeAt(now) {
		if feedback.positioned && feedback.motion.endsAt > 0 && now >= feedback.motion.endsAt {
			feedback.x, feedback.y = feedback.motion.toX, feedback.motion.toY
			feedback.motion = transientFeedbackMotion{}
		}

		return
	}
	feedback.x, feedback.y = feedback.motion.sample(now)
}

func (g *GameScene) layoutTransientActionFeedback(now int64, context transientFeedbackLayoutContext) {
	g.reconcileTransientFeedbackCapacity()
	for _, feedback := range g.transientActionFeedback {
		feedback.samplePosition(now)
		bounds := feedback.dialog.MeasureCompleteCard(context.screen, context.available, context.preferredCenter.X)
		feedback.cardWidth = bounds.Dx()
		feedback.cardHeight = bounds.Dy()
	}
	for len(g.transientActionFeedback) > 1 &&
		transientFeedbackGroupHeight(g.transientActionFeedback, context.gap) > context.available.Dy() {
		g.evictTransientActionFeedback(0)
	}
	if len(g.transientActionFeedback) == 0 {
		g.transientActionFeedbackLayout = transientFeedbackLayoutContext{}
		g.transientActionFeedbackLayoutSet = false

		return
	}

	heights := make([]int, len(g.transientActionFeedback))
	for index := range g.transientActionFeedback {
		heights[index] = g.transientActionFeedback[len(g.transientActionFeedback)-1-index].cardHeight
	}
	tops := solveTransientFeedbackTops(context.available, context.preferredCenter.Y, context.gap, heights)
	for index, feedback := range g.transientActionFeedback {
		targetTop := tops[len(tops)-1-index]
		bounds := feedback.dialog.PresentationBounds()
		targetLeft := bounds.Min.X
		if !feedback.positioned {
			feedback.x = float64(targetLeft)
			feedback.y = float64(preferredSingletonTop(context.available, context.preferredCenter.Y, feedback.cardHeight))
			feedback.positioned = true
		} else {
			feedback.x = clampFeedbackCoordinate(feedback.x, feedback.cardWidth, context.available.Min.X, context.available.Max.X)
			feedback.y = clampFeedbackCoordinate(feedback.y, feedback.cardHeight, context.available.Min.Y, context.available.Max.Y)
		}
		feedback.retarget(now, float64(targetLeft), float64(targetTop))
		feedback.dialog.SetPresentationPosition(feedback.x, feedback.y, context.available)
	}
	g.protectRearrangementObscuration(now)
	g.transientActionFeedbackLayout = context
	g.transientActionFeedbackLayoutSet = true
}

func (feedback *transientActionFeedback) retarget(now int64, targetX, targetY float64) {
	if feedback.motion.activeAt(now) && feedback.motion.toX == targetX && feedback.motion.toY == targetY {
		return
	}
	if math.Abs(feedback.x-targetX) < 0.5 && math.Abs(feedback.y-targetY) < 0.5 {
		feedback.x, feedback.y = targetX, targetY
		feedback.motion = transientFeedbackMotion{}

		return
	}
	feedback.motion = transientFeedbackMotion{
		fromX: feedback.x, fromY: feedback.y, toX: targetX, toY: targetY,
		startedAt: now, endsAt: now + transientFeedbackMoveDurationMs,
	}
	feedback.protectedUntil = max(feedback.protectedUntil, feedback.motion.endsAt)
}

func (g *GameScene) protectRearrangementObscuration(now int64) {
	for olderIndex, older := range g.transientActionFeedback {
		olderSweep := older.sweptBounds(now)
		for newerIndex := olderIndex + 1; newerIndex < len(g.transientActionFeedback); newerIndex++ {
			newer := g.transientActionFeedback[newerIndex]
			end := max(older.motion.endsAt, newer.motion.endsAt)
			if end > now && olderSweep.Overlaps(newer.sweptBounds(now)) {
				older.protectedUntil = max(older.protectedUntil, end)
			}
		}
	}
}

func (feedback *transientActionFeedback) sweptBounds(now int64) image.Rectangle {
	current := image.Rect(
		int(math.Floor(feedback.x)), int(math.Floor(feedback.y)),
		int(math.Ceil(feedback.x))+feedback.cardWidth, int(math.Ceil(feedback.y))+feedback.cardHeight,
	)
	if !feedback.motion.activeAt(now) {
		return current
	}
	target := image.Rect(
		int(math.Floor(feedback.motion.toX)), int(math.Floor(feedback.motion.toY)),
		int(math.Ceil(feedback.motion.toX))+feedback.cardWidth,
		int(math.Ceil(feedback.motion.toY))+feedback.cardHeight,
	)

	return current.Union(target)
}

func transientFeedbackGroupHeight(feedback []*transientActionFeedback, gap int) int {
	height := gap * max(0, len(feedback)-1)
	for _, item := range feedback {
		height += item.cardHeight
	}

	return height
}

func preferredSingletonTop(available image.Rectangle, centerY, height int) int {
	if height > available.Dy() {
		return available.Min.Y
	}

	return max(available.Min.Y, min(centerY-height/2, available.Max.Y-height))
}

func clampFeedbackCoordinate(value float64, size, minimum, maximum int) float64 {
	if size > maximum-minimum {
		return float64(minimum)
	}

	return max(float64(minimum), min(value, float64(maximum-size)))
}

func (g *GameScene) evictTransientActionFeedback(index int) {
	feedback := g.transientActionFeedback[index]
	delete(g.combatChestFeedback, feedback)
	g.dialogStack.RemoveWidgetInstance(feedback.dialog)
	feedback.dialog.HideWithoutAnimation()
	copy(g.transientActionFeedback[index:], g.transientActionFeedback[index+1:])
	g.transientActionFeedback[len(g.transientActionFeedback)-1] = nil
	g.transientActionFeedback = g.transientActionFeedback[:len(g.transientActionFeedback)-1]
}

func (g *GameScene) retireExternallyRemovedActionFeedback() bool {
	writeIndex := 0
	removed := false
	for _, feedback := range g.transientActionFeedback {
		if !g.dialogStack.ContainsWidgetInstance(feedback.dialog) {
			delete(g.combatChestFeedback, feedback)
			feedback.dialog.HideWithoutAnimation()
			removed = true

			continue
		}
		g.transientActionFeedback[writeIndex] = feedback
		writeIndex++
	}
	clear(g.transientActionFeedback[writeIndex:])
	g.transientActionFeedback = g.transientActionFeedback[:writeIndex]

	return removed
}

func (g *GameScene) clearTransientActionFeedback() {
	clear(g.combatChestFeedback)
	for _, feedback := range g.transientActionFeedback {
		g.dialogStack.RemoveWidgetInstance(feedback.dialog)
		feedback.dialog.HideWithoutAnimation()
	}
	clear(g.transientActionFeedback)
	g.transientActionFeedback = nil
	g.transientActionFeedbackAt = 0
	g.transientActionFeedbackClockSet = false
	g.transientActionFeedbackLayout = transientFeedbackLayoutContext{}
	g.transientActionFeedbackLayoutSet = false
}

func (g *GameScene) currentTransientFeedbackLayout() transientFeedbackLayoutContext {
	width, height := display.GetManager().GetCurrentSize()
	screen := image.Rect(0, 0, width, height)
	mode := transientFeedbackSurface
	available := g.surfaceFeedbackBounds(screen)
	centerPercent := transientFeedbackNormalCenter
	centerX := screen.Min.X + int(math.Round(float64(screen.Dx())*transientFeedbackMapCenter))
	if g.gameState != nil && g.gameState.IsInCombat() {
		mode = transientFeedbackCombat
		available = g.combatLayout().battlefieldRectangle(screen)
		centerX = (available.Min.X + available.Max.X) / 2
	} else if g.gameState != nil &&
		g.gameState.MapState.PlayerLocation.Location.GetMapType() == references.DungeonMapType {
		mode = transientFeedbackDungeon
		available = g.dungeonFeedbackBounds(screen)
		centerPercent = transientFeedbackDungeonCenter
	}
	pixelUnit := widgets.ClassicUIPixelUnit(width, height)
	padding := transientFeedbackPaddingPixelUnits * pixelUnit
	available = available.Inset(padding)
	if available.Empty() {
		panic("GameScene.currentTransientFeedbackLayout: feedback bounds are empty")
	}

	return transientFeedbackLayoutContext{
		screen: screen, available: available,
		preferredCenter: image.Pt(centerX, screen.Min.Y+int(math.Round(float64(screen.Dy())*centerPercent))),
		gap:             transientFeedbackGapPixelUnits * pixelUnit, mode: mode,
	}
}

func (g *GameScene) surfaceFeedbackBounds(screen image.Rectangle) image.Rectangle {
	aperture := g.borders.surfaceAperture
	if aperture.Empty() {
		aperture = image.Rect(
			screen.Min.X+int(math.Round(float64(screen.Dx())*gameScreenPercents.StartPercentX)),
			screen.Min.Y+int(math.Round(float64(screen.Dy())*gameScreenPercents.StartPercentY)),
			screen.Min.X+int(math.Round(float64(screen.Dx())*gameScreenPercents.EndPercentX)),
			screen.Min.Y+int(math.Round(float64(screen.Dy())*gameScreenPercents.EndPercentY)),
		)
	}
	safeRight := screen.Min.X + int(math.Floor(float64(screen.Dx())*dungeonHUDRightRailStart))

	return aperture.Intersect(image.Rect(screen.Min.X, screen.Min.Y, safeRight, screen.Max.Y))
}

func (g *GameScene) dungeonFeedbackBounds(screen image.Rectangle) image.Rectangle {
	levelHeight, directionHeight, debugHeight := 0, 0, 0
	if g.dungeonHUD.level != nil {
		levelHeight = int(math.Ceil(g.dungeonHUD.level.GetTotalHeightForLines(1)))
	}
	if g.dungeonHUD.direction != nil {
		directionHeight = int(math.Ceil(g.dungeonHUD.direction.GetTotalHeightForLines(1)))
	}
	if g.dungeonHUD.debug != nil {
		debugHeight = int(math.Ceil(g.dungeonHUD.debug.GetTotalHeightForLines(1)))
	}
	layout := calculateDungeonHUDLayout(screen, levelHeight, directionHeight, debugHeight)
	available := g.surfaceFeedbackBounds(screen).Intersect(layout.safePaneBounds)
	available.Min.Y = max(available.Min.Y, layout.frameBounds.Min.Y+layout.borderBandHeight)
	available.Max.Y = min(available.Max.Y, layout.frameBounds.Max.Y-layout.borderBandHeight)

	return available
}
