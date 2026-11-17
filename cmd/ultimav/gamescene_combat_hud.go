package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const (
	combatEventHistoryCapacity  = 64
	combatForecastSize          = 12
	combatLadderSpriteScale     = 0.5
	combatUnknownTurnLabel      = "???"
	combatAsleepLabel           = "asleep"
	combatHUDFontSize           = defaultOutputFontSize // Single visual-review switch for both combat panes.
	combatHUDOutputLines        = 48
	combatHUDOutputChars        = 40
	combatHUDTurnHeightPercent  = 0.5
	combatHUDCenterDivisor      = 2
	combatHUDLineGapDivisor     = 3
	combatHUDContentStartInsetX = 0.019
	combatHUDContentEndInsetX   = 0.02
	combatHUDContentInsetY      = 0.03
	combatHUDBorderOverlapY     = 0.015
	combatHUDDiagnosticParts    = 8
	combatAutoPassOpacity       = 0.5
	combatPoisonLabel           = "POISON"
	combatGenericEventLabel     = "EVENT"
	combatUnknownEntityLabel    = "Unknown"
)

// CombatHUD owns non-modal combat forecast and event-tape presentation state.
type CombatHUD struct {
	activeCombat         *game_state.CombatState
	detail               bool
	history              [combatEventHistoryCapacity]game_state.CombatEvent
	historyStart         int
	historyCount         int
	historyOffset        int
	unreadEvents         int
	eventLineBudget      int
	eventMaxChars        int
	forecast             [combatForecastSize]game_state.CombatTurnForecastEntry
	forecastCount        int
	turnOutput           *text.Output
	eventOutput          *text.Output
	turnBorder           *widgets.Border
	eventBorder          *widgets.Border
	borderBounds         image.Rectangle
	borderLayout         combatLayout
	notices              [combatEventHistoryCapacity]combatReportNotice
	attentionAt          int64
	identities           [combatEventHistoryCapacity]combatReportIdentities
	presentingIdentities *combatReportIdentities
	toMessageWindow      bool
	readout              []combatReadoutRow
}

func newCombatHUD() *CombatHUD {
	return &CombatHUD{
		activeCombat:    nil,
		detail:          false,
		history:         [combatEventHistoryCapacity]game_state.CombatEvent{},
		historyStart:    0,
		historyCount:    0,
		historyOffset:   0,
		unreadEvents:    0,
		eventLineBudget: 0,
		eventMaxChars:   combatHUDOutputChars,
		forecast:        [combatForecastSize]game_state.CombatTurnForecastEntry{},
		forecastCount:   0,
		turnOutput:      nil,
		eventOutput:     nil,
		turnBorder:      nil,
		eventBorder:     nil,
		borderBounds:    image.Rect(0, 0, 0, 0),
	}
}

func (hud *CombatHUD) setOutputs(turnOutput, eventOutput *text.Output) {
	hud.turnOutput = turnOutput
	hud.eventOutput = eventOutput
}

func (hud *CombatHUD) sync(combat *game_state.CombatState) {
	if combat == hud.activeCombat {
		return
	}
	hud.activeCombat = combat
	hud.historyStart = 0
	hud.historyCount = 0
	hud.historyOffset = 0
	hud.unreadEvents = 0
	hud.forecastCount = 0
	hud.notices = [combatEventHistoryCapacity]combatReportNotice{}
	hud.identities = [combatEventHistoryCapacity]combatReportIdentities{}
	hud.presentingIdentities = nil
	hud.attentionAt = 0
	if combat != nil {
		hud.addEvent(game_state.CombatEvent{
			Kind: game_state.CombatEventBattleBegun,
		})
	}
}

func (hud *CombatHUD) addEvent(event game_state.CombatEvent) int {
	// Routine steps remain authoritative events, but never occupy player report
	// capacity. Filtering at admission also protects loot from hidden evictions.
	if event.Kind == game_state.CombatEventMove {
		return -1
	}
	wasBrowsing := hud.historyOffset > 0
	index := (hud.historyStart + hud.historyCount) % len(hud.history)
	if hud.historyCount < len(hud.history) {
		hud.history[index] = event
		hud.historyCount++
	} else {
		hud.history[hud.historyStart] = event
		hud.historyStart = (hud.historyStart + 1) % len(hud.history)
	}
	hud.notices[index] = combatReportNotice{}
	hud.identities[index] = combatReportIdentities{}
	if combatReportNoteworthy(event) {
		hud.notices[index] = combatReportNotice{remainingMs: combatReportEmphasisMs, pending: true}
	}
	if wasBrowsing {
		hud.historyOffset = min(hud.historyOffset+1, max(hud.historyCount-1, 0))
		hud.unreadEvents++
	}

	return index
}

func (g *GameScene) recordCombatEvent(event game_state.CombatEvent) {
	if event.SpellTarget != nil {
		result := *event.SpellTarget
		event.SpellTarget = &result
	}
	if event.FireInjury != nil {
		injury := *event.FireInjury
		event.FireInjury = &injury
	}
	if event.Kind == game_state.CombatEventMove {
		g.closeCombatFireMovementAdmission()
	}
	g.observeCombatPossessionAllegiance(event)
	if event.Kind == game_state.CombatEventPickup && g.provisionChanges.state == g.gameState &&
		event.Pickup.Result.ItemType == references.ItemTypeProvision {
		g.provisionChanges.changes.AbsorbReportedPickup(
			references.Provision(event.Pickup.Result.ItemID), event.Pickup.Result.Acquired,
		)
	}
	// Authoritative results own one impact cue per attack. Direct poison retains
	// its former attack cue under its single field owner; other fields/cones use
	// their own presentation paths, since they may publish once per affected actor.
	switch event.Kind { //nolint:exhaustive // Only impact owners and final outcomes request these sounds.
	case game_state.CombatEventAttack, game_state.CombatEventRangedAttack:
		switch event.Result {
		case game_state.CombatFeedbackMiss:
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundCombatMiss)
		case game_state.CombatFeedbackGraze:
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundCombatGraze)
		case game_state.CombatFeedbackDamage:
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundCombatHit)
		case game_state.CombatFeedbackPoison, game_state.CombatFeedbackSleep, game_state.CombatFeedbackTheft:
			// Status effects have separate presentation/feedback owners.
		}
	case game_state.CombatEventEquipmentShattered:
		g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundCombatShatter)
	case game_state.CombatEventFieldTriggered:
		if event.FieldKind == game_state.CombatFieldPoison && event.Diagnostics.Spell == references.InNoxGrav && event.Diagnostics.MagicDamage {
			// Direct poison's single field report retains the hit cue previously
			// requested by its duplicate generic attack event.
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundCombatHit)
		}
	case game_state.CombatEventVictory:
		g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundCombatVictory)
	case game_state.CombatEventFled:
		g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundCombatFlee)
	case game_state.CombatEventInterruption:
		// REDUX DEVIATION: COMBAT-0012: original interference emits text only.
		// A dedicated cue makes the rejected action understandable alongside the report.
		//nolint:godox // TODO(audio): Assign combat_interrupted_01; once per rejected source/action, never on report redraw.
		g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundCombatInterrupted)
	default:
	}
	if g.combatHUD == nil {
		g.combatHUD = newCombatHUD()
	}
	g.combatHUD.sync(g.gameState.CombatScene)
	g.updateCombatReportAttention()
	disclosure := g.captureCombatReportIdentities(event)
	if event.SpellTarget != nil && !event.SpellTarget.Disclosed {
		disclosure.Target.Name = combatUnknownEntityLabel
		disclosure.Target.Disclosed = false
		disclosure.DetailsDisclosed = false
	}
	if event.FireInjury != nil && !event.FireInjury.Disclosed {
		disclosure.Target.Name = combatUnknownEntityLabel
		disclosure.Target.Disclosed = false
		disclosure.DetailsDisclosed = false
	}
	if slot := g.combatHUD.addEvent(event); slot >= 0 {
		g.combatHUD.identities[slot] = disclosure
		if g.combatLayout().panels != config.CombatPanelsOrderAndReport {
			g.writeCombatEventToMessageWindow(slot)
		}
	}
	g.showCombatSignatureEffect(event)
	if event.SpellTarget != nil {
		g.showCombatAreaSpellImpact(event)
	}
	if event.Kind == game_state.CombatEventDivision {
		g.showCombatDivisionEffect(event)
	}
	if event.Kind == game_state.CombatEventVanished && disclosure.VanishedDisclosed {
		g.showCombatVanishEffect(event)
	}
	if event.Kind == game_state.CombatEventFieldDispelled {
		g.showCombatFieldDispelEffect(event)
	}
	if event.FireInjury != nil && event.SpellTarget == nil {
		g.showCombatFireImpact(event)
	}
}

func (g *GameScene) drawCombatHUD(screen *ebiten.Image) {
	if g.gameState.CombatScene == nil || g.combatHUD == nil ||
		g.combatHUD.turnOutput == nil || g.combatHUD.eventOutput == nil {
		return
	}

	g.combatHUD.sync(g.gameState.CombatScene)
	layout := g.combatLayout()
	turnBounds, eventBounds := layout.turnAndReportRectangles(screen.Bounds())
	if turnBounds.Empty() {
		return
	}
	g.combatHUD.ensureBorders(screen.Bounds(), layout)
	if g.combatHUD.eventBorder != nil {
		g.combatHUD.eventBorder.Draw(screen)
	}
	g.combatHUD.turnBorder.Draw(screen)
	turnContent := combatHUDContentBounds(screen.Bounds(), turnBounds)
	eventContent := image.Rectangle{}
	if !eventBounds.Empty() {
		eventContent = combatHUDContentBounds(screen.Bounds(), eventBounds)
	}
	if g.gameState.IsCombatPartySpellTargeting() {
		g.drawCombatPartySpellHUD(screen, turnContent, eventContent)

		return
	}
	g.drawCombatTurnForecast(screen, turnContent)
	if !eventContent.Empty() {
		g.drawCombatEventTape(screen, eventContent)
	}
}

func combatHUDStackedRectangles(screenBounds, bounds image.Rectangle) (image.Rectangle, image.Rectangle) {
	turnBottom := bounds.Min.Y + int(float64(bounds.Dy())*combatHUDTurnHeightPercent)
	overlap := int(math.Round(float64(screenBounds.Dy()) * combatHUDBorderOverlapY))
	turn := image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, turnBottom)
	events := image.Rect(bounds.Min.X, turnBottom-overlap, bounds.Max.X, bounds.Max.Y)

	return turn, events
}

func combatHUDContentBounds(screenBounds, panelBounds image.Rectangle) image.Rectangle {
	return image.Rect(
		panelBounds.Min.X+int(math.Round(float64(screenBounds.Dx())*combatHUDContentStartInsetX)),
		panelBounds.Min.Y+int(math.Round(float64(screenBounds.Dy())*combatHUDContentInsetY)),
		panelBounds.Max.X-int(math.Round(float64(screenBounds.Dx())*combatHUDContentEndInsetX)),
		panelBounds.Max.Y-int(math.Round(float64(screenBounds.Dy())*combatHUDContentInsetY)),
	)
}

func (hud *CombatHUD) ensureBorders(screenBounds image.Rectangle, layout combatLayout) {
	if hud.turnBorder != nil && hud.borderBounds == screenBounds && hud.borderLayout == layout {
		return
	}
	turnBounds, eventBounds := layout.turnAndReportRectangles(screenBounds)
	hud.turnBorder = widgets.NewOpaqueBorder(
		combatHUDPlacement(screenBounds, turnBounds),
		borderWidthScaling,
		color.Black,
	)
	hud.eventBorder = nil
	if !eventBounds.Empty() {
		hud.eventBorder = widgets.NewOpaqueBorder(
			combatHUDPlacement(screenBounds, eventBounds),
			borderWidthScaling,
			color.Black,
		)
	}
	hud.borderBounds, hud.borderLayout = screenBounds, layout
}

func combatHUDPlacement(screenBounds, bounds image.Rectangle) sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: float64(bounds.Min.X-screenBounds.Min.X) / float64(screenBounds.Dx()),
		EndPercentX:   float64(bounds.Max.X-screenBounds.Min.X) / float64(screenBounds.Dx()),
		StartPercentY: float64(bounds.Min.Y-screenBounds.Min.Y) / float64(screenBounds.Dy()),
		EndPercentY:   float64(bounds.Max.Y-screenBounds.Min.Y) / float64(screenBounds.Dy()),
	}
}

func (g *GameScene) drawCombatTurnForecast(screen *ebiten.Image, bounds image.Rectangle) {
	hud := g.combatHUD
	lineHeight := int(math.Ceil(hud.turnOutput.GetLineSpacing()))
	lineGap := lineHeight / combatHUDLineGapDivisor
	hud.turnOutput.Reset()
	hud.turnOutput.SetMaxCharsPerLine(max(1, hud.turnOutput.Font.CharsForWidth(bounds.Dx())))
	hud.turnOutput.AppendLine().Yellow("TURN ORDER").LightCyan(combatDetailHeaderSuffix(hud.detail)).Done()
	hud.turnOutput.DrawPanelContent(screen, bounds.Min, bounds.Dx(), etext.AlignStart)

	currentBottom := g.drawCombatCurrentTurn(screen, bounds, lineHeight, lineGap)
	if height := g.drawCombatAttackReadout(screen, image.Pt(bounds.Min.X, currentBottom+lineGap), bounds.Dx()); height > 0 {
		currentBottom += lineGap + height
	}
	ruleTop := currentBottom + lineGap
	unit := widgets.ClassicUIPixelUnit(screen.Bounds().Dx(), screen.Bounds().Dy())
	vector.FillRect(screen, float32(bounds.Min.X), float32(ruleTop), float32(bounds.Dx()), float32(unit),
		widgets.DialogRuleLightColor, false)
	g.drawCombatTurnLadder(screen, image.Rect(bounds.Min.X, ruleTop+unit+lineGap, bounds.Max.X, bounds.Max.Y), lineHeight)
}

func (g *GameScene) drawCombatCurrentTurn(
	screen *ebiten.Image,
	bounds image.Rectangle,
	lineHeight, lineGap int,
) int {
	hud := g.combatHUD
	current, currentValid := currentCombatTurnIdentity(g.gameState.CombatScene)
	currentSpriteTop := bounds.Min.Y + lineHeight + lineGap
	currentSpriteBottom := currentSpriteTop
	if currentValid {
		sprite := g.combatTurnSprite(current)
		autoPass := combatTurnIsAutoPass(g.gameState.CombatScene, current)
		opacity := float32(1)
		if autoPass {
			opacity = combatAutoPassOpacity
		}
		spriteBounds := g.drawCombatHUDSprite(screen, sprite, bounds.Min.X, currentSpriteTop, opacity, 1)
		currentSpriteBottom = spriteBounds.Max.Y
		snapshot, ok := g.gameState.CombatScene.TurnQueue.GetEntrySnapshot(current)
		detail := g.combatCurrentTurnDetail(current, snapshot, ok)
		textX := spriteBounds.Max.X + lineGap
		hud.turnOutput.Reset()
		hud.turnOutput.SetMaxCharsPerLine(max(1, hud.turnOutput.Font.CharsForWidth(bounds.Max.X-textX)))
		if actor, disclosed := g.gameState.ProjectCombatActor(current, 0); disclosed {
			name := strings.ToUpper(actor.Name)
			if autoPass {
				hud.turnOutput.AppendLine().LightGray(name).Done()
			} else {
				hud.turnOutput.AppendLine().White(name).Done()
			}
			g.appendCombatCurrentAllegiance(actor, detail)
			if actor.Charmed || actor.Summoned {
				cause := "Summoned"
				if actor.Charmed {
					cause = "Charmed"
					if actor.Summoned {
						cause += "/Summoned"
					}
				}
				hud.turnOutput.AppendLine().LightGray(cause).Done()
			}
		}
		if g.combatTurnIdentityConcealed(current) {
			hud.turnOutput.AppendLine().LightGray(combatUnknownEntityLabel).Done()
		}
		if !g.combatTurnIdentityConcealed(current) {
			g.appendCombatTurnConditions(current)
		}
		hud.turnOutput.DrawPanelContent(
			screen,
			image.Pt(textX, currentSpriteTop),
			bounds.Max.X-textX,
			etext.AlignStart,
		)
		currentSpriteBottom = max(currentSpriteBottom, currentSpriteTop+
			int(math.Ceil(hud.turnOutput.GetTotalHeightForLines(hud.turnOutput.GetPanelContentLineCount()))))
	} else {
		hud.turnOutput.Reset()
		hud.turnOutput.AppendLine().LightGray("WAIT").Done()
		hud.turnOutput.DrawPanelContent(
			screen,
			image.Pt(bounds.Min.X, currentSpriteTop),
			bounds.Dx(),
			etext.AlignStart,
		)
		currentSpriteBottom += lineHeight
	}

	return currentSpriteBottom
}

// A party member fighting for the party needs no side label. Anyone else does:
// an enemy, an ally monster, or a possessed party member.
func (g *GameScene) appendCombatCurrentAllegiance(actor game_state.CombatActorPresentation, detail string) {
	ownParty := actor.Identity.IsPartyMember && actor.Loyalty == game_state.CombatMonsterLoyaltyParty
	if ownParty && detail == "" {
		return
	}
	line := g.combatHUD.turnOutput.AppendLine()
	if !ownParty {
		clr := ucolor.CombatAllegianceEnemy
		if actor.Loyalty == game_state.CombatMonsterLoyaltyParty {
			clr = ucolor.CombatAllegianceAlly
		}
		line.Text(combatAllegianceText(actor), clr)
		if detail != "" {
			line.LightGray(" ")
		}
	}
	line.LightGray(detail).Done()
}

// The ladder lists coming turns downward, one actor per row, for as many rows
// as the pane has room: sprite, full name, then health or the reason a turn is lost.
func (g *GameScene) drawCombatTurnLadder(screen *ebiten.Image, bounds image.Rectangle, lineHeight int) {
	hud := g.combatHUD
	hud.forecastCount = g.gameState.CombatScene.TurnQueue.FillForecast(hud.forecast[:])
	hud.forecastCount = g.filterConcealedCombatForecast(hud.forecastCount)
	_, spriteHeight := g.combatLadderSpriteSize(screen)
	rowHeight := max(spriteHeight, lineHeight) + lineHeight/combatHUDLineGapDivisor
	for index := range hud.forecastCount {
		row := image.Rect(bounds.Min.X, bounds.Min.Y+index*rowHeight, bounds.Max.X, bounds.Min.Y+(index+1)*rowHeight)
		if row.Max.Y > bounds.Max.Y {
			break
		}
		g.drawCombatLadderRow(screen, row, hud.forecast[index], lineHeight)
	}
}

func (g *GameScene) drawCombatLadderRow(
	screen *ebiten.Image, row image.Rectangle, forecast game_state.CombatTurnForecastEntry, lineHeight int,
) {
	hud := g.combatHUD
	identity := forecast.CombatTurnIdentity
	// Quickness outcomes are deliberately unknown until the authoritative
	// monster-loyal boundary draws its independent gate.
	skipped := combatTurnIsAutoPass(g.gameState.CombatScene, identity) ||
		(!forecast.CanTakeTurn && !forecast.ActionUncertain)
	opacity := float32(1)
	if skipped {
		opacity = combatAutoPassOpacity
	}
	gap := lineHeight / combatHUDLineGapDivisor
	spriteWidth, _ := g.combatLadderSpriteSize(screen)
	g.drawCombatHUDSprite(screen, g.combatTurnSprite(identity), row.Min.X, row.Min.Y, opacity, combatLadderSpriteScale)
	textLeft := row.Min.X + spriteWidth + gap
	textTop := row.Min.Y + (row.Dy()-gap-lineHeight)/combatHUDCenterDivisor

	actor, disclosed := g.gameState.ProjectCombatActor(identity, 0)
	name, suffix, nameColor := combatUnknownTurnLabel, "", color.Color(ucolor.EGALightGray)
	if disclosed {
		name, nameColor = actor.Name, combatLadderNameColor(actor, skipped)
	}
	trailing := ""
	switch {
	case !disclosed:
		trailing = "unseen"
	case skipped:
		trailing = g.combatLadderSkipReason(identity)
	}
	trailingWidth := 0
	if trailing != "" {
		width, _ := hud.turnOutput.Font.MeasureString(trailing)
		trailingWidth = int(math.Ceil(width))
		combatSingleLine(hud.turnOutput, trailing, ucolor.EGALightGray)
		hud.turnOutput.DrawPanelContent(screen, image.Pt(textLeft, textTop), row.Max.X-textLeft, etext.AlignEnd)
	} else if g.showCombatHPBars() {
		trailingWidth = g.drawCombatLadderHealth(screen, row, actor, gap)
	}
	if forecast.ActionUncertain && disclosed {
		suffix = "?"
	}
	name = widgets.CutDialogLine(hud.turnOutput.Font, name, suffix, row.Max.X-textLeft-trailingWidth-gap)
	combatSingleLine(hud.turnOutput, name, nameColor)
	hud.turnOutput.DrawPanelContent(screen, image.Pt(textLeft, textTop), row.Max.X-textLeft, etext.AlignStart)
}

// combatLadderSpriteSize is a ladder portrait's size: a battlefield sprite, reduced.
func (g *GameScene) combatLadderSpriteSize(screen *ebiten.Image) (int, int) {
	scaleX, scaleY := g.combatHUDSpriteScale(screen)

	return int(math.Round(sprites.ExpandedTileSize * scaleX * combatLadderSpriteScale)),
		int(math.Round(sprites.ExpandedTileSize * scaleY * combatLadderSpriteScale))
}

// A measured single line must not be rewrapped by an unrelated M-width budget.
func combatSingleLine(output *text.Output, value string, clr color.Color) {
	output.Reset()
	output.SetMaxCharsPerLine(max(1, len(value)))
	output.AppendLine().Text(value, clr).Done()
}

func combatLadderNameColor(actor game_state.CombatActorPresentation, skipped bool) color.Color {
	switch {
	case skipped:
		return ucolor.EGALightGray
	case actor.Loyalty != game_state.CombatMonsterLoyaltyParty:
		return ucolor.CombatAllegianceEnemy
	case actor.Identity.IsPartyMember:
		return ucolor.EGAWhite
	default:
		return ucolor.CombatAllegianceAlly
	}
}

// combatLadderSkipReason says in a word why a coming turn will be lost.
func (g *GameScene) combatLadderSkipReason(identity game_state.CombatTurnIdentity) string {
	combat := g.gameState.CombatScene
	if identity.IsPartyMember {
		if combat.IsCombatPCSleeping(identity) {
			return combatAsleepLabel
		}

		return "skips"
	}
	if monster := combat.MonsterManager.GetMonster(identity.EntityIndex); monster != nil && monster.IsAsleep() {
		return combatAsleepLabel
	}

	return "skips"
}

// drawCombatLadderHealth draws the battlefield's own health mark, at the same
// size, at the row's right edge and returns the width it used. Every row has one.
func (g *GameScene) drawCombatLadderHealth(
	screen *ebiten.Image, row image.Rectangle, actor game_state.CombatActorPresentation, gap int,
) int {
	var current, maximum int
	if actor.Identity.IsPartyMember {
		character := g.gameState.PartyState.GetCharacter(actor.Identity.EntityIndex)
		current, maximum = int(character.GetCurrentHp()), int(character.GetMaxHp())
	} else {
		monster := g.gameState.CombatScene.MonsterManager.GetMonster(actor.Identity.EntityIndex)
		current, maximum = monster.CurrentHP(), monster.MaxHP()
	}
	scaleX, scaleY := g.combatHUDSpriteScale(screen)
	options := &ebiten.DrawImageOptions{}
	options.GeoM.Scale(scaleX, scaleY)
	// Whole-pixel moves keep the mark's layout, so place it by its own bounds.
	bounds := newCombatHealthMark(options).outer
	top := row.Min.Y + (row.Dy()-gap-bounds.Dy())/combatHUDCenterDivisor
	options.GeoM.Translate(float64(row.Max.X-bounds.Max.X), float64(top-bounds.Min.Y))
	drawCombatHealthMark(screen, options, combatHealthBorderColor(actor.Loyalty), current, maximum)

	return bounds.Dx()
}

func (g *GameScene) filterConcealedCombatForecast(count int) int {
	if g.gameConfig.GameplayConfiguration.ShowConcealedTurnOrderSlots {
		return count
	}

	visibleCount := 0
	for index := range count {
		forecast := g.combatHUD.forecast[index]
		if g.combatTurnIdentityConcealed(forecast.CombatTurnIdentity) {
			continue
		}
		g.combatHUD.forecast[visibleCount] = forecast
		visibleCount++
	}

	return visibleCount
}

func (g *GameScene) combatTurnIdentityConcealed(identity game_state.CombatTurnIdentity) bool {
	_, disclosed := g.gameState.ProjectCombatActor(identity, 0)

	return !disclosed
}

func currentCombatTurnIdentity(combat *game_state.CombatState) (game_state.CombatTurnIdentity, bool) {
	if combat == nil || !combat.CombatTurnsStarted() {
		return game_state.CombatTurnIdentity{IsPartyMember: false, EntityIndex: 0}, false
	}

	return combat.GetCurrentTurnIdentity(), true
}

func (g *GameScene) combatTurnSprite(
	identity game_state.CombatTurnIdentity,
) *ebiten.Image {
	if g.combatTurnIdentityConcealed(identity) {
		return nil
	}
	if identity.IsPartyMember {
		character := g.gameState.PartyState.GetCharacter(identity.EntityIndex)

		return g.spriteSheet.GetSprite(combatPartySpriteIndex(
			character,
			g.gameState.CombatScene.IsCombatPCInvisible(identity),
			g.gameState.CombatScene.IsCombatPCRat(identity),
		))
	}
	monster := g.gameState.CombatScene.MonsterManager.GetMonster(identity.EntityIndex)
	if monster == nil || monster.EnemyRef() == nil || monster.EnemyRef().KeyFrameTile == nil {
		return nil
	}
	if g.combatTurnIdentityConcealed(identity) {
		return nil
	}

	return g.spriteSheet.GetSprite(monster.EnemyRef().KeyFrameTile.Index)
}

func (g *GameScene) combatHUDSpriteSize(
	screen *ebiten.Image,
	sprite *ebiten.Image,
) (int, int) {
	if sprite == nil {
		return 0, 0
	}
	scaleX, scaleY := g.combatHUDSpriteScale(screen)

	return int(math.Round(float64(sprite.Bounds().Dx()) * scaleX)),
		int(math.Round(float64(sprite.Bounds().Dy()) * scaleY))
}

func (g *GameScene) combatHUDSpriteScale(screen *ebiten.Image) (float64, float64) {
	if g.mapImage == nil {
		return 0, 0
	}
	targetWidth := float64(screen.Bounds().Dx()) *
		(gameScreenPercents.EndPercentX - gameScreenPercents.StartPercentX)
	targetHeight := float64(screen.Bounds().Dy()) *
		(gameScreenPercents.EndPercentY - gameScreenPercents.StartPercentY)

	return targetWidth / float64(g.mapImage.Bounds().Dx()),
		targetHeight / float64(g.mapImage.Bounds().Dy())
}

func (g *GameScene) combatCurrentTurnDetail(
	identity game_state.CombatTurnIdentity,
	snapshot game_state.CombatTurnForecastEntry,
	snapshotValid bool,
) string {
	if !g.combatHUD.detail || !snapshotValid {
		return ""
	}
	if !identity.IsPartyMember {
		if g.combatTurnIdentityConcealed(identity) {
			return ""
		}
	}
	modifier := 0
	if !identity.IsPartyMember {
		monster := g.gameState.CombatScene.MonsterManager.GetMonster(identity.EntityIndex)
		if monster != nil {
			modifier = monster.DexterityModifier()
		}
	}
	if modifier != 0 {
		return fmt.Sprintf("D%02d%+d C%02d", snapshot.Dexterity, modifier, snapshot.Countdown)
	}

	return fmt.Sprintf("D%02d C%02d", snapshot.Dexterity, snapshot.Countdown)
}

func (g *GameScene) drawCombatHUDSprite(
	screen *ebiten.Image,
	sprite *ebiten.Image,
	left, top int,
	opacity float32,
	scale float64,
) image.Rectangle {
	if sprite == nil {
		return image.Rect(left, top, left, top)
	}
	width, height := g.combatHUDSpriteSize(screen, sprite)
	width, height = int(math.Round(float64(width)*scale)), int(math.Round(float64(height)*scale))
	scaleX, scaleY := g.combatHUDSpriteScale(screen)
	options := &ebiten.DrawImageOptions{}
	options.GeoM.Scale(scaleX*scale, scaleY*scale)
	options.GeoM.Translate(float64(left), float64(top))
	options.ColorScale.ScaleAlpha(opacity)
	screen.DrawImage(sprite, options)

	return image.Rect(left, top, left+width, top+height)
}

func combatTurnIsAutoPass(combat *game_state.CombatState, identity game_state.CombatTurnIdentity) bool {
	return combat != nil && identity.IsPartyMember && combat.IsPartyTurnAutoPass(identity.EntityIndex)
}

func (g *GameScene) drawCombatEventTape(screen *ebiten.Image, bounds image.Rectangle) {
	spans := g.prepareCombatEventTape(bounds)
	g.drawCombatReportEmphasis(screen, bounds, spans)
	g.combatHUD.eventOutput.DrawPanelContent(screen, bounds.Min, bounds.Dx(), etext.AlignStart)
}

func (g *GameScene) prepareCombatEventTape(bounds image.Rectangle) []combatReportSpan {
	hud := g.combatHUD
	hud.eventMaxChars = hud.eventOutput.Font.CharsForWidth(bounds.Dx())
	hud.eventOutput.SetMaxCharsPerLine(hud.eventMaxChars)
	lineCapacity := hud.eventOutput.GetLineCapacity(bounds.Dy())
	g.appendCombatReportHeader()
	headerLines := hud.eventOutput.GetPanelContentLineCount()
	hud.eventLineBudget = max(lineCapacity-headerLines, 1)
	start, count := g.visibleCombatEventRange(hud.eventLineBudget)
	var spans []combatReportSpan
	for {
		g.appendCombatReportHeader()
		spans = spans[:0]
		for index := start; index < start+count; index++ {
			slot := (hud.historyStart + index) % len(hud.history)
			firstLine := hud.eventOutput.GetPanelContentLineCount()
			g.withCombatHistoryEvent(slot, g.appendCombatEvent)
			spans = append(spans, combatReportSpan{
				slot: slot, firstLine: firstLine,
				lines: hud.eventOutput.GetPanelContentLineCount() - firstLine,
			})
		}
		if hud.eventOutput.GetPanelContentLineCount() <= lineCapacity || count <= 1 {
			break
		}
		start++
		count--
	}

	return spans
}

func combatDetailHeaderSuffix(detail bool) string {
	if detail {
		return " DTL:ON"
	}

	return ""
}

func (g *GameScene) visibleCombatEventRange(lineBudget int) (int, int) {
	hud := g.combatHUD
	if hud.historyCount == 0 {
		return 0, 0
	}
	end := hud.historyCount - hud.historyOffset
	end = max(end, 0)
	start := end
	usedLines := 0
	for start > 0 {
		eventLines := g.combatHistoryEventLineCount((hud.historyStart + start - 1) % len(hud.history))
		if usedLines > 0 && usedLines+eventLines > lineBudget {
			break
		}
		usedLines += eventLines
		start--
	}

	return start, end - start
}

func (g *GameScene) combatEventLineCount(event game_state.CombatEvent) int {
	hud := g.combatHUD
	payloadWidth := max(hud.eventMaxChars-1, 1)
	measurement := text.MeasureText(
		g.combatEventPlainText(event),
		hud.eventOutput.Font,
		hud.eventOutput.GetLineSpacing(),
		payloadWidth,
	)
	lines := measurement.Lines
	if detail := g.combatEventDetailText(event); detail != "" && hud.detail {
		lines += text.MeasureText(
			detail,
			hud.eventOutput.Font,
			hud.eventOutput.GetLineSpacing(),
			payloadWidth,
		).Lines
	}

	return max(lines, 1)
}

func (g *GameScene) combatEventCountFromStart(lineBudget int) int {
	hud := g.combatHUD
	usedLines := 0
	count := 0
	for count < hud.historyCount {
		eventLines := g.combatHistoryEventLineCount((hud.historyStart + count) % len(hud.history))
		if count > 0 && usedLines+eventLines > lineBudget {
			break
		}
		usedLines += eventLines
		count++
	}

	return count
}

func (g *GameScene) appendCombatEvent(event game_state.CombatEvent) {
	g.appendCombatEventSentence(event)
	if detail := g.combatEventDetailText(event); detail != "" && g.combatHUD.detail {
		output := g.combatHUD.eventOutput
		if g.combatHUD.toMessageWindow {
			output = g.output
		}
		output.AppendLine().
			WithContinuationIndent(1).
			White(" ").LightGray(detail).Done()
	}
}

// Battle lines go to the Battle Report, or to the message window when Combat
// Panels shows no report. The message window keeps its arrow for commands.
func (g *GameScene) newCombatEventLine() *text.LineBuilder {
	if g.combatHUD.toMessageWindow {
		return g.output.AppendLine().WithContinuationIndent(1)
	}

	return g.combatHUD.eventOutput.AppendLine().
		CommandPromptMarker(ucolor.EGALightBlue).
		WithContinuationIndent(1)
}

func (g *GameScene) writeCombatEventToMessageWindow(slot int) {
	g.combatHUD.toMessageWindow = true
	g.withCombatHistoryEvent(slot, g.appendCombatEvent)
	g.combatHUD.toMessageWindow = false
}

func (g *GameScene) appendCombatEventSentence(event game_state.CombatEvent) {
	switch event.Kind {
	case game_state.CombatEventInterruption:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		line.Text(" interrupts ", ucolor.CombatReportWarning)
		g.appendCombatEntitySegment(line, event.Target)
		line.White("'s ").Text(combatInterruptedActionText(event), widgets.DialogValueColor).Done()
	case game_state.CombatEventPickup:
		g.appendCombatPickup(event.Pickup)
	case game_state.CombatEventLootRevealed:
		g.newCombatEventLine().Text("FOUND: ", widgets.DialogLabelColor).
			Text(combatLootRevealText(event.LootReveal), widgets.DialogValueColor).Done()
	case game_state.CombatEventChestTrap:
		g.newCombatEventLine().Text("TRAP: ", ucolor.CombatReportWarning).
			Text(combatChestTrapText(event.ChestTrap), widgets.DialogValueColor).Done()
	case game_state.CombatEventBattleBegun:
		g.newCombatEventLine().LightGreen("Battle has begun!").Done()
	case game_state.CombatEventVictory:
		g.newCombatEventLine().LightGreen("Victory!").Done()
	case game_state.CombatEventDefeat:
		g.newCombatEventLine().LightRed("Defeat").Done()
	case game_state.CombatEventMove:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		line.White(" moves ").LightCyan(strings.ToLower(combatMoveDirection(event))).Done()
	case game_state.CombatEventFled:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		line.White(" flees the battle").Done()
	case game_state.CombatEventEquipmentShattered:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		line.White("'s ").White(strings.ToLower(event.Diagnostics.Source.String())).LightRed(" shatters").Done()
	case game_state.CombatEventAttack, game_state.CombatEventPoison, game_state.CombatEventRangedAttack:
		g.appendCombatAttackSentence(event)
	case game_state.CombatEventPossession:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		line.Text(" possesses ", ucolor.CombatPossessed)
		g.appendCombatEntitySegment(line, event.Target)
		line.Done()
	case game_state.CombatEventGating:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		line.Magenta(g.combatGateReportResult(event)).Done()
	case game_state.CombatEventRetired:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		line.Magenta(" disappears").Done()
	case game_state.CombatEventVanished:
		g.newCombatEventLine().Text(g.combatVanishedReportName(event), ucolor.CombatAllegianceEnemy).Magenta(" vanishes").Done()
	case game_state.CombatEventVisibility:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		if event.Visibility == game_state.CombatVisibilityHidden {
			line.Magenta(" disappears").Done()
		} else {
			line.LightCyan(" reappears").Done()
		}
	case game_state.CombatEventTeleport:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		line.LightCyan(" teleports").Done()
	case game_state.CombatEventRecovery:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Target)
		line.Magenta(" is freed and passes out").Done()
	case game_state.CombatEventPossessionResistance, game_state.CombatEventPossessionRelease:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Target)
		line.LightCyan(combatPossessionEventSuffix(event)).Done()
	case game_state.CombatEventSummon:
		line := g.newCombatEventLine().LightCyan(references.GetSpellName(event.Summon.Spell)).White(" summons ")
		line.White(g.combatSummonReportResult(event)).Done()
	case game_state.CombatEventRepelUndead:
		spell, result := g.combatRepelReportResult(event)
		line := g.newCombatEventLine().LightCyan(spell).White(" ")
		line.White(result).Done()
	case game_state.CombatEventSpell:
		g.appendCombatSpellSentence(event)
	case game_state.CombatEventCondition:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Target)
		switch event.Condition {
		case game_state.CombatConditionQuicknessSuppressed:
			line.LightCyan(" loses its action to quickness").Done()
		case game_state.CombatConditionWoke:
			line.LightGreen(" wakes up").Done()
		case game_state.CombatConditionSleeping:
			line.LightGray(" sleeps through the turn").Done()
		case game_state.CombatConditionDraggedUnder:
			line.Magenta(" is dragged under").Done()
		case game_state.CombatConditionUnderground:
			line.Magenta(" struggles underground").Done()
		case game_state.CombatConditionRegurgitated:
			line.LightGreen(" is regurgitated").Done()
		}
	case game_state.CombatEventDivision:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		line.LightGreen(" divides").Done()
	case game_state.CombatEventFieldCreated:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		line.LightCyan(" creates a ").LightGreen(combatFieldName(event.FieldKind)).White(" field").Done()
	case game_state.CombatEventFieldTriggered:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Target)
		if event.FieldKind == game_state.CombatFieldSleep {
			line.Magenta(" falls asleep").Done()

			return
		}
		if event.FieldKind == game_state.CombatFieldFire {
			line.Textf(" is burned for %d", ucolor.CombatDamageText, event.HPLost)
			g.appendCombatInjurySuffix(line, event).Done()

			return
		}
		if event.HPLost > 0 || (event.FieldKind == game_state.CombatFieldPoison && event.Diagnostics.MagicDamage) {
			line.White(" takes ").Textf("%d", ucolor.CombatDamageText, event.HPLost).White(" from ").LightGreen(combatFieldName(event.FieldKind))
			g.appendCombatInjurySuffix(line, event).Done()

			return
		}
		line.White(" is affected by ").LightGreen(combatFieldName(event.FieldKind)).Done()
	case game_state.CombatEventFieldDispelled:
		line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
		line.White(" dispels a ").LightGreen(combatFieldName(event.FieldKind)).White(" field").Done()
	case game_state.CombatEventFieldExpired:
		if event.FieldFizzled {
			g.newCombatEventLine().Magenta(combatFieldName(event.FieldKind) + " field fizzles out").Done()

			return
		}
		g.newCombatEventLine().LightGray(combatFieldName(event.FieldKind) + " field dissipates").Done()
	case game_state.CombatEventUtilitySpell:
		g.appendCombatUtilitySpellSentence(event)
	}
}

func (g *GameScene) appendCombatUtilitySpellSentence(event game_state.CombatEvent) {
	line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
	line.White(" casts ").LightCyan(references.GetSpellName(event.Diagnostics.Spell)).White(" and ")
	switch event.UtilityResult {
	case game_state.CombatUtilityDoorUnlocked:
		line.LightGreen("unlocks the door").Done()
	case game_state.CombatUtilityChestDisarmed:
		line.LightGreen("disarms the chest").Done()
	case game_state.CombatUtilityDoorMagicLocked:
		line.Magenta("seals the door").Done()
	case game_state.CombatUtilityMagicLockRemoved:
		line.LightCyan("breaks the magical lock").Done()
	case game_state.CombatUtilityTeleported:
		line.LightCyan("blinks across the battlefield").Done()
	}
}

func (g *GameScene) appendCombatSpellSentence(event game_state.CombatEvent) {
	line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
	line.White(" casts ").LightCyan(references.GetSpellInfo(event.Diagnostics.Spell).Name)
	if event.SpellTarget != nil {
		if !event.SpellTarget.Disclosed {
			line.LightGray(" toward an unseen target").Done()

			return
		}
		line.White(" on ")
		g.appendCombatEntitySegment(line, event.Target)
		suffix, clr := combatAreaSpellTargetSuffix(*event.SpellTarget)
		line.Text(suffix, clr)
		g.appendCombatInjurySuffix(line, event).Done()

		return
	}
	if event.Diagnostics.ProjectileBlocked {
		line.White(" but the missile is ").LightGray("blocked").Done()

		return
	}
	if event.Result == game_state.CombatFeedbackPoison {
		line.White(" on ")
		g.appendCombatEntitySegment(line, event.Target).White(" and ").LightGreen("poisons").White(" them").Done()

		return
	}
	if event.Result == game_state.CombatFeedbackMiss {
		line.White(" on ")
		g.appendCombatEntitySegment(line, event.Target).White(" but they ").LightGray("resist").Done()

		return
	}
	line.White(" on ")
	g.appendCombatEntitySegment(line, event.Target)
	switch {
	case event.Fatal:
		line.Textf(" for %d, slain", ucolor.CombatDamageText, event.HPLost).Done()
	case event.HPLost == 0:
		line.LightGray(" but causes no harm").Done()
	default:
		line.White(" for ").Textf("%d", ucolor.CombatDamageText, event.HPLost)
		g.appendCombatInjurySuffix(line, event).Done()
	}
}

// Attacks are reported in plain words, with the word that says what happened
// in that effect's colour. The wound wording follows under its own policy: only
// while health bars are hidden.
func (g *GameScene) appendCombatAttackSentence(event game_state.CombatEvent) {
	parts := combatAttackParts(event)
	if len(parts) == 0 {
		return
	}
	line := g.appendCombatEntitySegment(g.newCombatEventLine(), event.Attacker)
	for _, part := range parts {
		if part.target {
			name, clr := g.combatEntitySegment(event.Target)
			line.Text(" "+name+part.text, clr)

			continue
		}
		line.Text(part.text, part.color)
	}
	g.appendCombatInjurySuffix(line, event).Done()
}

// combatAttackPart is one run of an attack line after the attacker's name:
// either words in a colour or the target's name. Rows wrap between runs, so
// each run opens with its own space and punctuation rides on the run before it.
type combatAttackPart struct {
	text   string
	color  ucolor.EGAColor
	target bool
}

func combatAttackParts(event game_state.CombatEvent) []combatAttackPart {
	target := combatAttackPart{target: true}
	words := func(text string, clr ucolor.EGAColor) combatAttackPart {
		return combatAttackPart{text: text, color: clr}
	}
	hits, damage := words(" hits", ucolor.EGAWhite), " "+strconv.Itoa(event.HPLost)
	hit := func(after string) []combatAttackPart {
		return []combatAttackPart{hits, target, words(" for", ucolor.EGAWhite), words(damage+after, ucolor.CombatDamageText)}
	}
	switch {
	case event.Fatal && event.Target.IsPartyMember:
		return append(hit("."), target, words(" falls!", ucolor.CombatDamageText))
	case event.Fatal:
		return []combatAttackPart{words(" slays", ucolor.CombatDamageText), target}
	case event.Result == game_state.CombatFeedbackMiss:
		return []combatAttackPart{words(" misses", ucolor.EGALightGray), target}
	case event.Result == game_state.CombatFeedbackGraze:
		return []combatAttackPart{words(" grazes", ucolor.EGALightGray), target}
	case event.Result == game_state.CombatFeedbackPoison:
		return []combatAttackPart{words(" poisons", ucolor.EGALightGreen), target}
	case event.Result == game_state.CombatFeedbackSleep:
		return []combatAttackPart{
			hits, {text: ":", target: true}, words(" "+combatAsleepLabel, ucolor.EGALightMagenta),
		}
	case event.Result == game_state.CombatFeedbackTheft:
		return []combatAttackPart{words(fmt.Sprintf(" steals %d food", event.Diagnostics.FoodStolen), ucolor.EGAYellow)}
	case event.Result == game_state.CombatFeedbackDamage:
		return hit("")
	default:
		return nil
	}
}

func (g *GameScene) appendCombatEntitySegment(
	line *text.LineBuilder,
	entity game_state.CombatFeedbackEntity,
) *text.LineBuilder {
	name, clr := g.combatEntitySegment(entity)

	return line.Text(name, clr)
}

func (g *GameScene) combatEntitySegment(entity game_state.CombatFeedbackEntity) (string, ucolor.EGAColor) {
	if g.combatHUD != nil {
		if snapshot, ok := g.combatHUD.reportIdentity(combatTurnIdentityForFeedback(entity)); ok {
			return snapshot.Name, snapshot.Color
		}
	}
	name := g.combatEntityEventName(combatTurnIdentityForFeedback(entity))
	if entity.IsPartyMember {
		return name, ucolor.EGAYellow
	}
	if g.gameState.CombatScene != nil {
		monster := g.gameState.CombatScene.MonsterManager.GetMonster(entity.Index)
		if monster != nil && monster.IsPartyLoyal() {
			return name, ucolor.EGALightCyan
		}
	}

	return name, ucolor.CombatAllegianceEnemy
}

func (g *GameScene) combatEventPlainText(event game_state.CombatEvent) string {
	return g.combatEventBasePlainText(event) + g.combatInjurySuffix(event)
}

func (g *GameScene) combatEventBasePlainText(event game_state.CombatEvent) string {
	switch event.Kind {
	case game_state.CombatEventInterruption:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))
		target := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Target))

		return actor + " interrupts " + target + "'s " + combatInterruptedActionText(event)
	case game_state.CombatEventPickup:
		return combatPickupText(event.Pickup)
	case game_state.CombatEventLootRevealed:
		return "FOUND: " + combatLootRevealText(event.LootReveal)
	case game_state.CombatEventChestTrap:
		return "TRAP: " + combatChestTrapText(event.ChestTrap)
	case game_state.CombatEventBattleBegun:
		return "Battle has begun!"
	case game_state.CombatEventVictory:
		return "Victory!"
	case game_state.CombatEventDefeat:
		return "Defeat"
	case game_state.CombatEventMove:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))

		return fmt.Sprintf("%s moves %s", actor, strings.ToLower(combatMoveDirection(event)))
	case game_state.CombatEventFled:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))

		return actor + " flees the battle"
	case game_state.CombatEventEquipmentShattered:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))

		return fmt.Sprintf("%s's %s shatters", actor, strings.ToLower(event.Diagnostics.Source.String()))
	case game_state.CombatEventAttack, game_state.CombatEventPoison, game_state.CombatEventRangedAttack:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))
		target := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Target))

		return combatAttackPlainText(event, actor, target)
	case game_state.CombatEventPossession:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))
		target := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Target))

		return fmt.Sprintf("%s possesses %s", actor, target)
	case game_state.CombatEventGating:
		return g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker)) + g.combatGateReportResult(event)
	case game_state.CombatEventRetired:
		return g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker)) + " disappears"
	case game_state.CombatEventVanished:
		return g.combatVanishedReportName(event) + " vanishes"
	case game_state.CombatEventVisibility:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))
		if event.Visibility == game_state.CombatVisibilityHidden {
			return actor + " disappears"
		}

		return actor + " reappears"
	case game_state.CombatEventTeleport:
		return g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker)) + " teleports"
	case game_state.CombatEventRecovery:
		return g.combatEntityEventName(combatTurnIdentityForFeedback(event.Target)) + " is freed and passes out"
	case game_state.CombatEventPossessionResistance, game_state.CombatEventPossessionRelease:
		return g.combatEntityEventName(combatTurnIdentityForFeedback(event.Target)) + combatPossessionEventSuffix(event)
	case game_state.CombatEventSummon:
		return references.GetSpellName(event.Summon.Spell) + " summons " + g.combatSummonReportResult(event)
	case game_state.CombatEventRepelUndead:
		spell, result := g.combatRepelReportResult(event)

		return spell + " " + result
	case game_state.CombatEventSpell:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))
		target := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Target))
		spellInfo := references.GetSpellInfo(event.Diagnostics.Spell)
		if event.SpellTarget != nil {
			if !event.SpellTarget.Disclosed {
				return fmt.Sprintf("%s casts %s toward an unseen target", actor, spellInfo.Name)
			}
			suffix, _ := combatAreaSpellTargetSuffix(*event.SpellTarget)

			return fmt.Sprintf("%s casts %s on %s%s", actor, spellInfo.Name, target, suffix)
		}
		if event.Diagnostics.ProjectileBlocked {
			return fmt.Sprintf("%s casts %s but the missile is blocked", actor, spellInfo.Name)
		}
		if event.Result == game_state.CombatFeedbackPoison {
			return fmt.Sprintf("%s casts %s on %s and poisons them", actor, spellInfo.Name, target)
		}
		if event.Result == game_state.CombatFeedbackMiss {
			return fmt.Sprintf("%s casts %s on %s but they resist", actor, spellInfo.Name, target)
		}
		if event.Fatal {
			return fmt.Sprintf("%s casts %s on %s for %d, slain", actor, spellInfo.Name, target, event.HPLost)
		}
		if event.HPLost == 0 {
			return fmt.Sprintf("%s casts %s on %s but causes no harm", actor, spellInfo.Name, target)
		}

		return fmt.Sprintf("%s casts %s on %s for %d", actor, spellInfo.Name, target, event.HPLost)
	case game_state.CombatEventCondition:
		target := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Target))

		return combatConditionPlainText(event.Condition, target)
	case game_state.CombatEventDivision:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))

		return combatDivisionPlainText(actor)
	case game_state.CombatEventFieldCreated:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))

		return actor + " creates a " + combatFieldName(event.FieldKind) + " field"
	case game_state.CombatEventFieldTriggered:
		target := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Target))
		if event.FieldKind == game_state.CombatFieldSleep {
			return target + " falls asleep"
		}
		if event.FieldKind == game_state.CombatFieldFire {
			return fmt.Sprintf("%s is burned for %d", target, event.HPLost)
		}
		if event.HPLost > 0 || (event.FieldKind == game_state.CombatFieldPoison && event.Diagnostics.MagicDamage) {
			return fmt.Sprintf("%s takes %d from %s", target, event.HPLost, combatFieldName(event.FieldKind))
		}

		return target + " is affected by " + combatFieldName(event.FieldKind)
	case game_state.CombatEventFieldDispelled:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))

		return actor + " dispels a " + combatFieldName(event.FieldKind) + " field"
	case game_state.CombatEventFieldExpired:
		if event.FieldFizzled {
			return combatFieldName(event.FieldKind) + " field fizzles out"
		}

		return combatFieldName(event.FieldKind) + " field dissipates"
	case game_state.CombatEventUtilitySpell:
		actor := g.combatEntityEventName(combatTurnIdentityForFeedback(event.Attacker))

		return combatUtilitySpellPlainText(event, actor)
	}

	return combatGenericEventLabel
}

func combatAreaSpellTargetSuffix(result game_state.CombatAreaSpellTargetResult) (string, ucolor.EGAColor) {
	switch result.Outcome {
	case game_state.CombatSpellAvoided:
		return " but they avoid it", ucolor.CombatMissText
	case game_state.CombatSpellResisted:
		return " but they resist", ucolor.CombatMissText
	case game_state.CombatSpellImmune:
		return " but they are immune", ucolor.CombatMissText
	case game_state.CombatSpellZeroDamage:
		return " but causes no damage", ucolor.CombatGrazeText
	case game_state.CombatSpellSlept:
		return " and puts them to sleep", ucolor.EGALightMagenta
	case game_state.CombatSpellPoisoned:
		return " and poisons them", ucolor.EGALightGreen
	case game_state.CombatSpellDamaged:
		if result.Fatal {
			return fmt.Sprintf(" for %d, slain", result.HPLost), ucolor.CombatDamageText
		}

		return fmt.Sprintf(" for %d", result.HPLost), ucolor.CombatDamageText
	case game_state.CombatSpellUnspecified:
		panic("area spell report requires a resolved outcome")
	default:
		panic("unknown area spell report outcome")
	}
}

func combatInterruptedActionText(event game_state.CombatEvent) string {
	switch event.InterruptedAction {
	case game_state.CombatInterruptedAttack:
		return strings.ToLower(event.Diagnostics.Source.String()) + " attack"
	case game_state.CombatInterruptedCast:
		return "casting"
	default:
		panic("unknown interrupted combat action")
	}
}

func combatUtilitySpellPlainText(event game_state.CombatEvent, actor string) string {
	spell := references.GetSpellName(event.Diagnostics.Spell)
	switch event.UtilityResult {
	case game_state.CombatUtilityDoorUnlocked:
		return actor + " casts " + spell + " and unlocks the door"
	case game_state.CombatUtilityChestDisarmed:
		return actor + " casts " + spell + " and disarms the chest"
	case game_state.CombatUtilityDoorMagicLocked:
		return actor + " casts " + spell + " and seals the door"
	case game_state.CombatUtilityMagicLockRemoved:
		return actor + " casts " + spell + " and breaks the magical lock"
	case game_state.CombatUtilityTeleported:
		return actor + " casts " + spell + " and blinks across the battlefield"
	default:
		return combatGenericEventLabel
	}
}

func combatConditionPlainText(condition game_state.CombatConditionResult, target string) string {
	switch condition {
	case game_state.CombatConditionQuicknessSuppressed:
		return target + " loses its action to quickness"
	case game_state.CombatConditionWoke:
		return target + " wakes up"
	case game_state.CombatConditionSleeping:
		return target + " sleeps through the turn"
	case game_state.CombatConditionDraggedUnder:
		return target + " is dragged under"
	case game_state.CombatConditionUnderground:
		return target + " struggles underground"
	case game_state.CombatConditionRegurgitated:
		return target + " is regurgitated"
	default:
		return target + " sleeps through the turn"
	}
}

func combatDivisionPlainText(actor string) string {
	return actor + " divides"
}

func combatAttackPlainText(event game_state.CombatEvent, actor, target string) string {
	parts := combatAttackParts(event)
	if len(parts) == 0 {
		return combatGenericEventLabel
	}
	var sentence strings.Builder
	sentence.WriteString(actor)
	for _, part := range parts {
		if part.target {
			sentence.WriteString(" " + target)
		}
		sentence.WriteString(part.text)
	}

	return sentence.String()
}

func (g *GameScene) combatEventDetailText(event game_state.CombatEvent) string {
	if (event.SpellTarget != nil && !event.SpellTarget.Disclosed) || !g.combatReportDisclosure(event).DetailsDisclosed {
		return ""
	}
	switch event.Kind {
	case game_state.CombatEventInterruption:
		return fmt.Sprintf("Interference from (%d,%d); action at (%d,%d)",
			event.Attacker.Position.X, event.Attacker.Position.Y, event.Target.Position.X, event.Target.Position.Y)
	case game_state.CombatEventMove, game_state.CombatEventTeleport:
		return fmt.Sprintf("From (%d,%d) to (%d,%d)", event.From.X, event.From.Y, event.To.X, event.To.Y)
	case game_state.CombatEventDivision:
		return fmt.Sprintf("Clone created at (%d,%d)", event.To.X, event.To.Y)
	case game_state.CombatEventFieldCreated, game_state.CombatEventFieldDispelled, game_state.CombatEventFieldExpired:
		return fmt.Sprintf("At (%d,%d)", event.To.X, event.To.Y)
	case game_state.CombatEventUtilitySpell:
		if event.UtilityResult == game_state.CombatUtilityTeleported {
			return fmt.Sprintf("From (%d,%d) to (%d,%d)", event.From.X, event.From.Y, event.To.X, event.To.Y)
		}

		return fmt.Sprintf("At (%d,%d)", event.To.X, event.To.Y)
	case game_state.CombatEventAttack, game_state.CombatEventPoison, game_state.CombatEventSpell,
		game_state.CombatEventRangedAttack, game_state.CombatEventPossession:
		return formatCombatDiagnostics(event.Diagnostics)
	case game_state.CombatEventSummon:
		return combatSummonDetailText(event.Summon)
	case game_state.CombatEventRepelUndead:
		return combatRepelDetailText(event.AnXenCorp.Result)
	case game_state.CombatEventGating:
		return fmt.Sprintf("Created %s #%d at (%d,%d)", event.Created.MonsterType.Name(), event.Created.Index,
			event.Created.Position.X, event.Created.Position.Y)
	case game_state.CombatEventRetired:
		return fmt.Sprintf("Retired at (%d,%d)", event.From.X, event.From.Y)
	case game_state.CombatEventVanished:
		return fmt.Sprintf("Vanished at (%d,%d)", event.Vanished.Position.X, event.Vanished.Position.Y)
	case game_state.CombatEventRecovery:
		return fmt.Sprintf("Recovered at (%d,%d)", event.Target.Position.X, event.Target.Position.Y)
	case game_state.CombatEventPossessionResistance, game_state.CombatEventPossessionRelease:
		return fmt.Sprintf("%d/12 own scheduled turns completed", event.PossessionTurns)
	case game_state.CombatEventFled:
		return fmt.Sprintf("Left from (%d,%d)", event.Attacker.Position.X, event.Attacker.Position.Y)
	case game_state.CombatEventBattleBegun,
		game_state.CombatEventPickup,
		game_state.CombatEventLootRevealed,
		game_state.CombatEventChestTrap,
		game_state.CombatEventCondition,
		game_state.CombatEventVisibility,
		game_state.CombatEventEquipmentShattered,
		game_state.CombatEventVictory,
		game_state.CombatEventDefeat,
		game_state.CombatEventFieldTriggered:
		return ""
	}

	return ""
}

func combatSummonAggregateText(event game_state.CombatSummonEvent) string {
	noun := "allies"
	if event.Count == 1 {
		noun = event.Created[0].MonsterType.Name()
	}
	if event.Binding == game_state.CombatSummonBindingFailed {
		return fmt.Sprintf("%d hostile %s", event.Count, noun)
	}

	return fmt.Sprintf("%d loyal %s", event.Count, noun)
}

func combatFearOrRepelText(event game_state.CombatEvent) (string, string) {
	if event.Diagnostics.Spell == references.InQuasCorp {
		return "In Quas Corp", combatFearFeedbackText(event.AnXenCorp.Result)
	}

	return "An Xen Corp", combatRepelAggregateText(event.AnXenCorp.Result)
}

func combatRepelAggregateText(result game_state.CombatAnXenCorpResult) string {
	if !combatRepelHasTargets(result) {
		return "affects no undead"
	}
	parts := make([]string, 0, 3)
	if len(result.Affected) == 0 {
		parts = append(parts, "repels no undead")
	} else {
		parts = append(parts, fmt.Sprintf("repels %d undead", len(result.Affected)))
	}
	if len(result.Resisted) == 1 {
		parts = append(parts, "1 resists")
	} else if len(result.Resisted) > 1 {
		parts = append(parts, fmt.Sprintf("%d resist", len(result.Resisted)))
	}
	if len(result.Immune) == 1 {
		parts = append(parts, "1 is immune")
	} else if len(result.Immune) > 1 {
		parts = append(parts, fmt.Sprintf("%d are immune", len(result.Immune)))
	}

	return strings.Join(parts, "; ")
}

func combatRepelFeedbackText(result game_state.CombatAnXenCorpResult) string {
	if !combatRepelHasTargets(result) {
		return "No undead are affected."
	}
	parts := make([]string, 0, 3)
	switch len(result.Affected) {
	case 0:
		parts = append(parts, "No undead are repelled")
	case 1:
		parts = append(parts, "1 undead is repelled")
	default:
		parts = append(parts, fmt.Sprintf("%d undead are repelled", len(result.Affected)))
	}
	if len(result.Resisted) == 1 {
		parts = append(parts, "1 resists")
	} else if len(result.Resisted) > 1 {
		parts = append(parts, fmt.Sprintf("%d resist", len(result.Resisted)))
	}
	if len(result.Immune) == 1 {
		parts = append(parts, "1 is immune")
	} else if len(result.Immune) > 1 {
		parts = append(parts, fmt.Sprintf("%d are immune", len(result.Immune)))
	}

	return strings.Join(parts, "; ") + "."
}

func combatFearFeedbackText(result game_state.CombatAnXenCorpResult) string {
	if !combatRepelHasTargets(result) {
		return "No creatures are affected."
	}
	parts := make([]string, 0, 3)
	if len(result.Affected) == 1 {
		parts = append(parts, "1 creature flees")
	} else if len(result.Affected) > 1 {
		parts = append(parts, fmt.Sprintf("%d creatures flee", len(result.Affected)))
	}
	if len(result.Resisted) == 1 {
		parts = append(parts, "1 resists")
	} else if len(result.Resisted) > 1 {
		parts = append(parts, fmt.Sprintf("%d resist", len(result.Resisted)))
	}
	if len(result.Immune) == 1 {
		parts = append(parts, "1 is immune")
	} else if len(result.Immune) > 1 {
		parts = append(parts, fmt.Sprintf("%d are immune", len(result.Immune)))
	}

	return strings.Join(parts, "; ") + "."
}

func combatSummonDetailText(event game_state.CombatSummonEvent) string {
	parts := make([]string, 0, len(event.Created))
	for _, created := range event.Created {
		parts = append(parts, fmt.Sprintf("%s #%d at (%d,%d)", created.MonsterType.Name(), created.Index,
			created.Position.X, created.Position.Y))
	}

	return strings.Join(parts, "; ")
}

func combatRepelDetailText(result game_state.CombatAnXenCorpResult) string {
	parts := make([]string, 0, 3)
	if len(result.Affected) > 0 {
		parts = append(parts, combatRepelTargetDetail("Affected", result.Affected))
	}
	if len(result.Resisted) > 0 {
		parts = append(parts, combatRepelTargetDetail("Resisted", result.Resisted))
	}
	if len(result.Immune) > 0 {
		parts = append(parts, combatRepelTargetDetail("Immune", result.Immune))
	}

	return strings.Join(parts, "; ")
}

func combatRepelTargetDetail(outcome string, targets []game_state.CombatSpellMonster) string {
	entries := make([]string, 0, len(targets))
	for _, target := range targets {
		entries = append(entries, fmt.Sprintf("%s #%d at (%d,%d)", target.MonsterType.Name(), target.Index,
			target.Position.X, target.Position.Y))
	}

	return outcome + ": " + strings.Join(entries, ", ")
}

func combatFieldName(kind game_state.CombatFieldKind) string {
	switch kind {
	case game_state.CombatFieldPoison:
		return "poison"
	case game_state.CombatFieldSleep:
		return "sleep"
	case game_state.CombatFieldFire:
		return "fire"
	case game_state.CombatFieldProtection:
		return "protection"
	default:
		return "magic"
	}
}

func combatMoveDirection(event game_state.CombatEvent) string {
	switch {
	case event.To.X < event.From.X:
		return "WEST"
	case event.To.X > event.From.X:
		return "EAST"
	case event.To.Y < event.From.Y:
		return "NORTH"
	case event.To.Y > event.From.Y:
		return "SOUTH"
	default:
		return "STILL"
	}
}

func formatCombatDiagnostics(diagnostics game_state.CombatDiagnostics) string {
	parts := make([]string, 0, combatHUDDiagnosticParts)
	if diagnostics.Hit.AutomaticHit {
		parts = append(parts, "Automatic hit")
	} else if diagnostics.Hit.Rolled {
		operator := ">="
		if diagnostics.Hit.Outcome == game_state.CombatHitMiss {
			operator = "<"
		}
		parts = append(parts, fmt.Sprintf("Roll %d %s %d", diagnostics.Hit.D30Roll,
			operator, diagnostics.Hit.Threshold))
	}
	if diagnostics.TargetBaseDex > 0 && diagnostics.Hit.DefenderEffectiveDex > 0 &&
		diagnostics.TargetBaseDex != diagnostics.Hit.DefenderEffectiveDex {
		parts = append(parts, fmt.Sprintf(
			"Target DEX %d -> %d",
			diagnostics.TargetBaseDex,
			diagnostics.Hit.DefenderEffectiveDex,
		))
	}
	if damage := formatCombatDamageDiagnostics(diagnostics); damage != "" {
		parts = append(parts, damage)
	}
	switch {
	case diagnostics.TargetImmortal:
		parts = append(parts, fmt.Sprintf("Immortal target: %d damage", diagnostics.FinalDamage))
	case diagnostics.TargetUndead && diagnostics.MagicDamage:
		parts = append(parts, "Magic bypasses undead resistance")
	case diagnostics.TargetUndead && !diagnostics.MagicWeapon:
		parts = append(parts, fmt.Sprintf("Undead resistance halves damage to %d", diagnostics.FinalDamage))
	case diagnostics.MagicWeapon && diagnostics.TargetUndead:
		parts = append(parts, "Magic bypasses undead resistance")
	}
	if diagnostics.PoisonRolled {
		parts = append(parts, fmt.Sprintf("Poison roll %d of %d", diagnostics.PoisonRoll,
			diagnostics.PoisonRollMaximum))
	}
	if diagnostics.PoisonFallbackRolled {
		parts = append(parts, fmt.Sprintf("Poison fallback damage %d", diagnostics.PoisonFallbackDamage))
	}
	if diagnostics.TheftRolled {
		parts = append(parts, fmt.Sprintf("Theft roll %d of %d", diagnostics.TheftRoll,
			diagnostics.TheftRollMaximum))
	}
	if diagnostics.FoodStolen > 0 {
		parts = append(parts, fmt.Sprintf("Food stolen %d", diagnostics.FoodStolen))
	}
	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "; ")
}

func formatCombatDamageDiagnostics(diagnostics game_state.CombatDiagnostics) string {
	damage := diagnostics.Damage
	if !damage.AttackRolled && damage.AttackRoll == 0 && !damage.ArmorRolled {
		return ""
	}

	result := fmt.Sprintf("Damage %d", damage.AttackRoll)
	if damage.ArmorRolled {
		result += fmt.Sprintf(" - armor %d = %d", damage.ArmorRoll, damage.PostArmorDamage)
	} else if diagnostics.FinalDamage != damage.PostArmorDamage &&
		!diagnostics.TargetImmortal &&
		(!diagnostics.TargetUndead || diagnostics.MagicWeapon) {
		result += fmt.Sprintf(" = %d", diagnostics.FinalDamage)
	}

	return result
}

func (g *GameScene) combatEntityEventName(identity game_state.CombatTurnIdentity) string {
	if g.combatHUD != nil {
		if snapshot, ok := g.combatHUD.reportIdentity(identity); ok {
			return snapshot.Name
		}
	}
	if identity.IsPartyMember {
		name := g.gameState.PartyState.GetCharacter(identity.EntityIndex).GetNameAsString()
		if identity.Slot != 0 {
			return fmt.Sprintf("%s clone %d", name, identity.Slot)
		}

		return name
	}
	monster := g.gameState.CombatScene.MonsterManager.GetMonster(identity.EntityIndex)
	if monster == nil {
		return combatUnknownEntityLabel
	}
	if monster.IsInvisible() || !g.gameState.CombatTerrainAllowsObjectDisplay(monster.Position()) {
		return combatUnknownEntityLabel
	}

	return monster.MonsterType().Name()
}

func combatTurnIdentityForFeedback(entity game_state.CombatFeedbackEntity) game_state.CombatTurnIdentity {
	return game_state.CombatTurnIdentity{
		IsPartyMember: entity.IsPartyMember,
		EntityIndex:   entity.Index,
		Slot:          entity.Slot,
	}
}
