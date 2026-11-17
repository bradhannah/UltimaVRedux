package main

import (
	"bytes"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// A pinned active-indicator reference, independent of scene identity resolution,
// production selection geometry, and production corner rectangles. Together with
// the recorded aim reference it specifies exact allowed foreground pixels, not
// an exemption for an entire selected actor or a tolerance for changed HP.
func drawActiveSelectionReference(screen *ebiten.Image, options *ebiten.DrawImageOptions, elapsed int64, face ucolor.EGAColor) {
	low, high, offset := float64(1), float64(17), float64(0)
	stencil := activeCornerRaster
	if elapsed%900 >= 450 {
		low, high, offset = 0, 18, -1
		stencil = expandedActiveCornerRaster
	}
	left, top := options.GeoM.Apply(low, low)
	right, bottom := options.GeoM.Apply(high, high)
	x0, y0 := options.GeoM.Apply(0, 0)
	x1, y1 := options.GeoM.Apply(1, 1)
	scale := float32(math.Min(x1-x0, y1-y0))
	vector.StrokeRect(screen, float32(left), float32(top), float32(right-left), float32(bottom-top), 2*scale, ucolor.EGABlack, false)
	vector.StrokeRect(screen, float32(left), float32(top), float32(right-left), float32(bottom-top), scale, face, false)
	for y, row := range strings.Split(stencil, "\n") {
		for x, pixel := range row {
			if pixel == '.' {
				continue
			}
			clr := ucolor.EGABlack
			if pixel == 'W' {
				clr = face
			}
			left, top := options.GeoM.Apply(float64(x)+offset, float64(y)+offset)
			right, bottom := options.GeoM.Apply(float64(x+1)+offset, float64(y+1)+offset)
			left, top, right, bottom = math.Round(left), math.Round(top), math.Round(right), math.Round(bottom)
			vector.FillRect(screen, float32(left), float32(top), float32(right-left), float32(bottom-top), clr, false)
		}
	}
}

func assertCombatHealthSelectionComposition(
	t *testing.T, screen, health, foreground *ebiten.Image, actors []*ebiten.DrawImageOptions,
) {
	t.Helper()
	bounds := health.Bounds()
	require.Equal(t, bounds, screen.Bounds())
	require.Equal(t, bounds, foreground.Bounds())
	wantedHP, markers, actual := signaturePixels(health), signaturePixels(foreground), signaturePixels(screen)
	unchangedHP, overlaidHP, markerPixels := 0, 0, 0
	for pixel := 0; pixel < len(actual); pixel += 4 {
		want := wantedHP[pixel : pixel+4]
		if markers[pixel+3] != 0 {
			want = markers[pixel : pixel+4]
			markerPixels++
			if wantedHP[pixel+3] != 0 && !bytes.Equal(want, wantedHP[pixel:pixel+4]) {
				overlaidHP++
			}
		} else if wantedHP[pixel+3] != 0 {
			unchangedHP++
		} else {
			continue
		}
		if !bytes.Equal(want, actual[pixel:pixel+4]) {
			t.Fatalf("health/selection composition at (%d,%d), foreground=%t: want %v got %v",
				bounds.Min.X+pixel/4%bounds.Dx(), bounds.Min.Y+pixel/4/bounds.Dx(), markers[pixel+3] != 0, want, actual[pixel:pixel+4])
		}
	}
	require.Positive(t, markerPixels, "exercise the actual foreground indicator")
	require.Positive(t, overlaidHP, "exercise selection taking priority over HP, not just a nearby marker")
	require.Positive(t, unchangedHP, "uncovered health pixels must still be compared exactly")
	// These fixture layouts leave the mark's interior and a side of its border
	// uncovered, even on selected actors. Pin their readability for every injured
	// neighbor, including 1-HP bars, rather than just counting colours somewhere
	// in the whole battlefield. An uninjured actor shows no mark and is skipped.
	marked := 0
	for actor, options := range actors {
		middleX, middleY := options.GeoM.Apply(8.5, 15)
		middle := image.Pt(int(middleX), int(middleY))
		if !middle.In(bounds) || wantedHP[((middle.Y-bounds.Min.Y)*bounds.Dx()+middle.X-bounds.Min.X)*4+3] == 0 {
			continue
		}
		marked++
		for _, source := range []image.Rectangle{image.Rect(3, 14, 14, 16), image.Rect(14, 14, 15, 16)} {
			left, top := options.GeoM.Apply(float64(source.Min.X), float64(source.Min.Y))
			right, bottom := options.GeoM.Apply(float64(source.Max.X), float64(source.Max.Y))
			interior := image.Rect(int(math.Ceil(left)), int(math.Ceil(top)), int(math.Floor(right)), int(math.Floor(bottom))).Intersect(bounds)
			require.False(t, interior.Empty(), "actor %d region %v must be sampled", actor, source)
			for y := interior.Min.Y; y < interior.Max.Y; y++ {
				for x := interior.Min.X; x < interior.Max.X; x++ {
					pixel := ((y-bounds.Min.Y)*bounds.Dx() + x - bounds.Min.X) * 4
					require.Zero(t, markers[pixel+3], "selection invaded actor %d protected fill/allegiance at %d,%d", actor, x, y)
					require.Equal(t, byte(255), wantedHP[pixel+3], "fixture lost actor %d health at %d,%d", actor, x, y)
					require.Equal(t, wantedHP[pixel:pixel+4], actual[pixel:pixel+4], "actor %d unreadable fill/allegiance at %d,%d", actor, x, y)
				}
			}
		}
	}
	require.Positive(t, marked, "the fixture must injure someone")
}

func TestCombatHealthMarkLayoutIsWholePixelsWhereverItStands(t *testing.T) {
	native := newCombatHealthMark(&ebiten.DrawImageOptions{})
	assert.Equal(t, image.Rect(2, 13, 15, 17), native.outer)
	assert.Equal(t, image.Rect(3, 14, 14, 16), native.inner)
	assert.Equal(t, native.inner, native.strip, "one screen pixel per source pixel leaves no room for an inset")
	assert.True(t, native.outer.In(combatHealthFrameBounds()), "signature effects split frames on this region")
	for _, face := range combatAttackBracketFaces() {
		assert.False(t, face.Inset(-1).Overlaps(combatHealthFrameBounds()))
	}
	// 1280x720 draws a source pixel 3.2 screen pixels wide and about 3.47 tall.
	for _, offset := range []float64{0, 0.35, 0.5, 0.65} {
		options := &ebiten.DrawImageOptions{}
		options.GeoM.Scale(3.2, 610.0/176)
		options.GeoM.Translate(100+offset, 200+offset)
		mark := newCombatHealthMark(options)
		assert.Equal(t, image.Pt(42, 13), mark.outer.Size(), "offset %g", offset)
		assert.Equal(t, mark.outer.Inset(3), mark.inner, "a border one source pixel thick")
		assert.Equal(t, mark.inner.Inset(1), mark.strip, "a black inset one screen pixel thick")
		assert.Equal(t, 5, mark.strip.Dy(), "a health strip one and a half source pixels tall")
	}
}

func TestCombatHealthMarkDrawsSideBorderBlackInsetAndHealthStrip(t *testing.T) {
	options := &ebiten.DrawImageOptions{}
	options.GeoM.Scale(3.2, 610.0/176)
	frame := ebiten.NewImage(64, 64)
	t.Cleanup(frame.Dispose)
	const border, interior, stripWidth, stripHeight = 42*13 - 36*7, 36 * 7, 34, 5
	for _, test := range []struct {
		loyalty game_state.CombatMonsterLoyalty
		side    ucolor.EGAColor
		hp      int
		health  ucolor.EGAColor
		filled  int
	}{
		{game_state.CombatMonsterLoyaltyParty, ucolor.CombatHealthFriend, 100, ucolor.EGALightGreen, stripWidth},
		{game_state.CombatMonsterLoyaltyMonster, ucolor.CombatAllegianceEnemy, 51, ucolor.EGALightGreen, 18},
		{game_state.CombatMonsterLoyaltyMonster, ucolor.CombatAllegianceEnemy, 50, ucolor.EGAYellow, 17},
		{game_state.CombatMonsterLoyaltyParty, ucolor.CombatHealthFriend, 25, ucolor.EGAYellow, 9},
		{game_state.CombatMonsterLoyaltyParty, ucolor.CombatHealthFriend, 24, ucolor.EGALightRed, 9},
		{game_state.CombatMonsterLoyaltyParty, ucolor.CombatHealthFriend, 1, ucolor.EGALightRed, 1},
		{game_state.CombatMonsterLoyaltyParty, ucolor.CombatHealthFriend, 0, ucolor.EGALightRed, 0},
	} {
		frame.Clear()
		drawCombatHealthMark(frame, options, combatHealthBorderColor(test.loyalty), test.hp, 100)
		assert.Equal(t, border, countExactColor(frame, test.side), "hp %d", test.hp)
		assert.Equal(t, test.filled*stripHeight, countExactColor(frame, test.health), "hp %d", test.hp)
		assert.Equal(t, interior-test.filled*stripHeight, countExactColor(frame, ucolor.EGABlack), "hp %d", test.hp)
		assert.Equal(t, border+interior, opaquePixelCount(frame), "hp %d: nothing outside the mark", test.hp)
		assert.Zero(t, countExactColor(frame, ucolor.CombatAllegianceAlly), "no cyan on a health mark")
	}
}

func TestCombatHealthFrameInsectAggregatePreservesSegmentsAndDisclosure(t *testing.T) {
	scene := newCombatConcealmentTestState(t)
	state := scene.gameState
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	position := references.Position{X: 5, Y: 5}
	setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
	ref := (*state.GameReferences.EnemyReferences)[references.MonsterInsect]
	ref.HitPoints = 100
	var indices []int
	for _, hp := range []int{1, 25, 50, 100} {
		monster := game_state.NewCombatMonster(references.MonsterInsect, &ref, position, state.AsRandomProvider())
		monster.SetLoyalty(game_state.CombatMonsterLoyaltyParty)
		monster.SetCurrentHP(hp)
		indices = append(indices, state.CombatScene.AddMonsterToQueue(monster))
	}
	scene.beginCombatRenderFrame()
	t.Cleanup(scene.endCombatRenderFrame)
	require.Equal(t, indices, scene.combatPartyInsectStack(position))
	frame, segments := ebiten.NewImage(18, 18), ebiten.NewImage(18, 18)
	t.Cleanup(frame.Dispose)
	t.Cleanup(segments.Dispose)
	options := &ebiten.DrawImageOptions{}
	scene.drawCombatInsectStackHealth(segments, newCombatHealthMark(options).strip, indices)
	for slot, index := range indices {
		frame.Clear()
		actor, ok := state.ProjectCombatActor(game_state.CombatTurnIdentity{EntityIndex: index}, 0)
		require.True(t, ok)
		scene.drawCombatActorHealthFrame(frame, options, actor)
		if slot != 0 {
			assert.Zero(t, opaquePixelCount(frame), "only the representative may emit the aggregate")
			continue
		}
		want, got := signaturePixels(segments), signaturePixels(frame)
		for pixel := 0; pixel < len(want); pixel += 4 {
			if want[pixel+3] != 0 {
				require.Equal(t, want[pixel:pixel+4], got[pixel:pixel+4], "segment attribution/fill changed at %d", pixel/4)
			}
		}
		assert.Equal(t, 13*4-11*2, countExactColor(frame, ucolor.CombatHealthFriend))
		for member := range indices {
			x, y := combatInsectStackPipPosition(member, len(indices))
			assert.GreaterOrEqual(t, y, 2.0, "northern health frame ends before the pip row")
			assert.Equal(t, ucolor.EGACyan.ToRGBA(), frame.At(int(x), int(y)))
		}
	}
	scene.endCombatRenderFrame()
	state.CombatScene.MonsterManager.GetMonster(indices[0]).SetInvisible(true)
	require.Equal(t, indices[1:], scene.combatPartyInsectStack(position), "invisible member must not add a pip or health segment")
	setCombatConcealmentTerrain(state, position, indexes.BlackSquare)
	assert.Empty(t, scene.combatPartyInsectStack(position))
}

func TestCombatHealthFrameSignatureOpacityAndHiddenIntervals(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	// Use source-pixel scale to isolate opacity and hidden-interval composition.
	// Fractional health geometry is independently pinned above; comparing two
	// separately packed source textures at fractional nearest ties is not an
	// independent opacity oracle and can select opposite boundary texels.
	manager.PublishLogicalSize(400, 208)
	scene := newCombatConcealmentTestState(t)
	state := scene.gameState
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	for i := range state.CombatScene.PartyActive {
		state.CombatScene.PartyActive[i] = false
	}
	from, to := references.Position{X: 2, Y: 2}, references.Position{X: 8, Y: 7}
	setCombatConcealmentTerrain(state, from, indexes.BrickFloor)
	index, monster := signatureMonster(t, scene, references.MonsterShadowlord, to)
	monster.SetCurrentHP(1)
	scene.recordCombatEvent(game_state.CombatEvent{Kind: game_state.CombatEventTeleport, From: from, To: to,
		Attacker: game_state.CombatFeedbackEntity{Index: index, Position: to}})
	require.Len(t, scene.combatSignatureEffects, 1)
	effect := scene.combatSignatureEffects[0]
	sample := combatFixtureRasterBounds(scene.combatEntityDrawOptions(from, false, index, 0)).Union(
		combatFixtureRasterBounds(scene.combatEntityDrawOptions(to, false, index, 0)))
	actual := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	expected := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	mask := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	t.Cleanup(actual.Dispose)
	t.Cleanup(expected.Dispose)
	t.Cleanup(mask.Dispose)
	for _, age := range []int64{90, 180, 259, 360} {
		state.ElapsedMs = effect.StartedMs + age
		scene.updateCombatSignatureEffects()
		actual.Clear()
		expected.Clear()
		mask.Clear()
		scene.drawCombatSignatureEffects(actual)
		scene.drawCombatHealthFrames(actual)
		if age == 180 || age == 259 {
			assert.Zero(t, opaquePixelCount(actual), "hidden phase must expose neither destination nor health")
			continue
		}
		require.Equal(t, signaturePixels(effect.Frame.SubImage(combatHealthFrameBounds()).(*ebiten.Image)),
			signaturePixels(effect.HealthFrame.SubImage(combatHealthFrameBounds()).(*ebiten.Image)), "foreground split must preserve every source health pixel")
		position, clr := from, ucolor.CombatMaterialization
		if age == 360 {
			position, clr = to, ucolor.CombatTeleportArrival
		}
		options := scene.combatEntityDrawOptions(position, false, index, 0)
		drawCombatCornerCue(expected, options, 2.5, clr)
		drawCombatHealthMark(mask, options, ucolor.CombatAllegianceEnemy, 1, monster.MaxHP())
		options.ColorScale.ScaleAlpha(0.5)
		// Independent full-frame reference: one application of original fade,
		// rather than an opaque HP pass or two half-opacity blends.
		expected.DrawImage(effect.Frame, options)
		want, got, region := signaturePixels(expected), signaturePixels(actual), signaturePixels(mask)
		for pixel := 0; pixel < len(want); pixel += 4 {
			if region[pixel+3] != 0 {
				require.True(t, bytes.Equal(want[pixel:pixel+4], got[pixel:pixel+4]), "age%d altered faded health at (%d,%d): %v -> %v", age, pixel/4%actual.Bounds().Dx(), pixel/4/actual.Bounds().Dx(), want[pixel:pixel+4], got[pixel:pixel+4])
			}
		}
		assertSignatureHP(t, effect.Frame, 1, monster.MaxHP())
	}
	scene.clearCombatSignatureEffects()
}

func TestCombatHealthFramesSummonSuppressionAndPossessionSwitch(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(400, 208)
	scene := newCombatConcealmentTestState(t)
	screen := ebiten.NewImage(400, 208)
	t.Cleanup(screen.Dispose)
	state := scene.gameState
	actor := state.CombatScene.GetCurrentTurnIdentity()
	for index := range state.CombatScene.PartyActive {
		state.CombatScene.PartyActive[index] = index == actor.EntityIndex
	}
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	setCombatConcealmentTerrain(state, state.CombatScene.GetCombatantPosition(actor), indexes.BrickFloor)
	character := state.PartyState.GetCharacter(actor.EntityIndex)
	character.SetHp(character.GetMaxHp())
	scene.drawCombatHealthFrames(screen)
	assert.Zero(t, opaquePixelCount(screen), "the battlefield marks only the injured")
	character.SetHp(character.GetMaxHp() * 3 / 4) // Green, so the strip is neither side's colour.
	for _, possessed := range []bool{false, true, false} {
		state.CombatScene.PartyPossessed[actor.EntityIndex] = possessed
		screen.Clear()
		scene.drawCombatHealthFrames(screen)
		assert.Equal(t, possessed, countExactColor(screen, ucolor.CombatAllegianceEnemy) > 0)
		assert.Equal(t, !possessed, countExactColor(screen, ucolor.CombatHealthFriend) > 0)
	}
	state.CombatScene.PartyActive[actor.EntityIndex] = false
	index, monster := signatureMonster(t, scene, references.MonsterRat, references.Position{X: 5, Y: 5})
	monster.SetLoyalty(game_state.CombatMonsterLoyaltyParty)
	monster.SetCurrentHP(monster.MaxHP() - 1)
	scene.combatSummonEffect = &combatSummonSpellEffect{Result: game_state.CombatSummonResult{Created: []game_state.CombatSummonedMonster{{Index: index, CombatantID: monster.CombatantID(), Position: monster.Position()}}}}
	for _, phase := range []combatSummonEffectPhase{combatSummonInvocation, combatSummonSeal, combatSummonManifestation, combatSummonAllegiance} {
		scene.combatSummonEffect.Phase = phase
		screen.Clear()
		scene.drawCombatHealthFrames(screen)
		assert.Equal(t, phase == combatSummonAllegiance, opaquePixelCount(screen) > 0)
	}
	monster.SetInvisible(true)
	screen.Clear()
	scene.drawCombatHealthFrames(screen)
	assert.Zero(t, opaquePixelCount(screen))
}

func TestCombatHealthFrameCorpserMasksWholeOutlineAndHP(t *testing.T) {
	for _, condition := range []game_state.CombatConditionResult{game_state.CombatConditionDraggedUnder, game_state.CombatConditionRegurgitated} {
		scene := newCombatConcealmentTestState(t)
		state := scene.gameState
		actor := state.CombatScene.GetCurrentTurnIdentity()
		position := state.CombatScene.GetCombatantPosition(actor)
		setCombatConcealmentTerrain(state, position, indexes.BrickFloor)
		state.CombatScene.PartyUnderground[actor.EntityIndex] = condition == game_state.CombatConditionDraggedUnder
		scene.recordCombatEvent(game_state.CombatEvent{Kind: game_state.CombatEventCondition, Condition: condition,
			Target: game_state.CombatFeedbackEntity{IsPartyMember: true, Index: actor.EntityIndex, Slot: actor.Slot, Position: position}})
		require.Len(t, scene.combatSignatureEffects, 1)
		effect := scene.combatSignatureEffects[0]
		state.ElapsedMs += effect.DurationMs / 2
		scene.updateCombatSignatureEffects()
		scene.prepareCombatSignatureLayers(effect)
		pixels, full, mask := signaturePixels(effect.HealthFrame), signaturePixels(effect.Frame), signaturePixels(effect.Dissolve.effect.GetMask())
		width := effect.Frame.Bounds().Dx()
		visible, hidden := 0, 0
		for pixel := 0; pixel < len(pixels); pixel += 4 {
			point := image.Pt(pixel/4%width, pixel/4/width)
			if !point.In(combatHealthFrameBounds()) || mask[pixel+3] == 0 {
				require.Equal(t, []byte{0, 0, 0, 0}, pixels[pixel:pixel+4], "health or allegiance escaped its mask at %v", point)
				if point.In(combatHealthFrameBounds()) {
					hidden++
				}
			} else {
				require.Equal(t, full[pixel:pixel+4], pixels[pixel:pixel+4])
				visible++
			}
		}
		assert.Positive(t, visible)
		assert.Positive(t, hidden)
		scene.clearCombatSignatureEffects()
		assert.Nil(t, effect.BodyFrame)
		assert.Nil(t, effect.HealthFrame)
	}
}
