package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
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
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCombatAttackBracketsProductionActorPass(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(1280, 720)
	scene, actor := setupCombatAutoAttackInputTest(t)
	state := scene.gameState
	scene.gameConfig = state.GameConfig
	scene.mapImage = ebiten.NewImage(304, 208)
	t.Cleanup(scene.mapImage.Dispose)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 1, Y: 1})
	character := state.PartyState.GetCharacter(actor.EntityIndex)
	character.SetEquippedItem(references.SlotWeapon, references.MagicAxe)
	character.SetEquippedItem(references.SlotShield, references.NoEquipment)
	monsterRef := (*state.GameReferences.EnemyReferences)[references.MonsterOrc]
	for index := range 9 {
		position := references.Position{X: references.Coordinate(index%3 + 3), Y: references.Coordinate(index/3 + 3)}
		monster := game_state.NewCombatMonster(references.MonsterOrc, &monsterRef, position, state.AsRandomProvider())
		state.CombatScene.AddMonsterToQueue(monster)
	}
	preview := scene.combatAttackPreview()
	require.Len(t, preview.Targets, 9)
	var sample image.Rectangle
	for _, target := range preview.Targets {
		sample = sample.Union(combatFixtureRasterBounds(scene.combatMonsterDrawOptions(target.Position, target.MonsterIndex)))
	}
	require.LessOrEqual(t, sample.Dx()*sample.Dy(), 256*256, "nine nearby actors must use a local raster target: %v", sample)
	screen := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	reference := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	t.Cleanup(screen.Dispose)
	t.Cleanup(reference.Dispose)
	background := ucolor.EGALightCyan.ToRGBA()
	screen.Fill(background)
	// This calls the real outer composition, guarding against moving the
	// eligibility pass back beneath a later actor draw in drawAvatarOverlay.
	scene.drawAvatarOverlay(screen)
	actual := make([]byte, sample.Dx()*sample.Dy()*4)
	wanted := make([]byte, len(actual))
	screen.ReadPixels(actual)
	for _, target := range preview.Targets {
		reference.Fill(background)
		drawCombatAttackBrackets(reference, scene.combatMonsterDrawOptions(target.Position, target.MonsterIndex))
		reference.ReadPixels(wanted)
		markedPixels := 0
		for i := 0; i < len(wanted); i += 4 {
			if wanted[i] == background.R && wanted[i+1] == background.G && wanted[i+2] == background.B {
				continue
			}
			markedPixels++
			if !bytes.Equal(wanted[i:i+4], actual[i:i+4]) {
				t.Fatalf("production actor pass erased target %d marker at (%d,%d): want %v got %v", target.MonsterIndex, sample.Min.X+i/4%sample.Dx(), sample.Min.Y+i/4/sample.Dx(), wanted[i:i+4], actual[i:i+4])
			}
		}
		require.Positive(t, markedPixels)
	}
}

func TestCombatAttackBracketsDenseComposition(t *testing.T) {
	sheet := sharedPresentationTestSpriteSheet()
	for _, face := range combatAttackBracketFaces() {
		assert.True(t, face.Inset(-1).In(image.Rect(1, 1, 17, 17)), "full backing must fit the 16-pixel cell")
		assert.False(t, face.Inset(-1).Overlaps(combatHealthFrameBounds()), "reserve the entire exterior health frame, not just its colored fill")
	}
	for scaleIndex, scale := range []struct{ x, y float64 }{{1, 1}, {1280 * 0.76 / 304, 720.0 / 208}, {1366 * 0.76 / 304, 768.0 / 208}} {
		for originIndex, origin := range []struct{ x, y float64 }{{30.35, 30.65}, {30.65, 30.35}, {30.5, 30.5}} {
			for layoutIndex, layout := range []string{"all marked", "center only", "checkerboard"} {
				for _, aimCenter := range []bool{false, true} {
					// Check registered geometry throughout the full 54-case matrix.
					// Rasterize one origin per layout/aim role, distributing samples
					// over native and both fractional scales instead of their product.
					for _, face := range combatAttackBracketFaces() {
						backing := face.Inset(-1)
						left, top := origin.x+float64(backing.Min.X)*scale.x, origin.y+float64(backing.Min.Y)*scale.y
						right, bottom := origin.x+float64(backing.Max.X)*scale.x, origin.y+float64(backing.Max.Y)*scale.y
						assert.GreaterOrEqual(t, left, origin.x+scale.x)
						assert.GreaterOrEqual(t, top, origin.y+scale.y)
						assert.LessOrEqual(t, right, origin.x+17*scale.x)
						assert.LessOrEqual(t, bottom, origin.y+12*scale.y, "all scales reserve the exterior health frame")
					}
					rasterOrigin := layoutIndex
					if aimCenter {
						rasterOrigin = (rasterOrigin + 1) % 3
					}
					if scaleIndex != layoutIndex || originIndex != rasterOrigin {
						continue
					}
					t.Run(fmt.Sprintf("%.3fx%.3f/origin%.2f,%.2f/%s/aim%t", scale.x, scale.y, origin.x, origin.y, layout, aimCenter), func(t *testing.T) {
						const width = 256
						screen := ebiten.NewImage(width, width)
						protected := ebiten.NewImage(width, width)
						reference := ebiten.NewImage(width, width)
						t.Cleanup(screen.Dispose)
						t.Cleanup(protected.Dispose)
						t.Cleanup(reference.Dispose)
						background := ucolor.EGALightCyan.ToRGBA()
						faceColor := ucolor.CombatAttackEligibleFace.ToRGBA()
						screen.Fill(background)
						var options [9]*ebiten.DrawImageOptions
						var marked [9]bool
						for index := range 9 {
							x, y := index%3, index/3
							options[index] = &ebiten.DrawImageOptions{}
							options[index].GeoM.Scale(scale.x, scale.y)
							options[index].GeoM.Translate(origin.x+float64(x*16)*scale.x, origin.y+float64(y*16)*scale.y)
							marked[index] = layout == "all marked" || layout == "center only" && index == 4 || layout == "checkerboard" && (x+y)%2 == 0
							monster := sheet.GetTileEffectProcessor().ApplyDynamicSimple(sheet.GetSprite(indexes.OrcKeyIndex+indexes.SpriteIndex(index%4)), sprites.BlackOutline)
							screen.DrawImage(monster, options[index])
							// Include real status and HP renderers for marked AND unmarked
							// neighbors. Their complete raster footprint is protected.
							for _, target := range []*ebiten.Image{screen, protected} {
								drawCombatFleeChevrons(target, options[index])
								loyalty := game_state.CombatMonsterLoyaltyMonster
								if !marked[index] {
									loyalty = game_state.CombatMonsterLoyaltyParty
								}
								if index == 4 {
									drawCombatPossessedMarker(target, options[index])
								}
								drawCombatHealthMark(target, options[index], combatHealthBorderColor(loyalty), 25+index%3*25, 100)
							}
						}
						before := make([]byte, width*width*4)
						after := make([]byte, len(before))
						wanted := make([]byte, len(before))
						protectedPixels := make([]byte, len(before))
						allowed := make([]bool, width*width)
						screen.ReadPixels(before)
						protected.ReadPixels(protectedPixels)
						// Production order: all actor artwork/HP/status, foreground
						// eligibility, then the dominant aiming reticle.
						for index := range 9 {
							if marked[index] && !(aimCenter && index == 4) {
								drawCombatAttackBrackets(screen, options[index])
							}
						}
						screen.ReadPixels(after)
						for i := 0; i < len(before); i += 4 {
							if protectedPixels[i+3] != 0 && !bytes.Equal(before[i:i+4], after[i:i+4]) {
								t.Fatalf("marker altered HP/status at (%d,%d): %v -> %v", i/4%width, i/4/width, before[i:i+4], after[i:i+4])
							}
						}
						for index := range 9 {
							reference.Fill(background)
							drawCombatAttackBrackets(reference, options[index])
							reference.ReadPixels(wanted)
							middle, _ := options[index].GeoM.Apply(9, 0)
							var faces, borders [2]int
							visible := marked[index] && !(aimCenter && index == 4)
							for i := 0; i < len(wanted); i += 4 {
								if wanted[i] == background.R && wanted[i+1] == background.G && wanted[i+2] == background.B {
									continue
								}
								if !visible {
									if !bytes.Equal(before[i:i+4], after[i:i+4]) {
										t.Fatalf("unmarked/aimed creature %d gained marker pixels", index)
									}
									continue
								}
								allowed[i/4] = true
								if !bytes.Equal(wanted[i:i+4], after[i:i+4]) {
									t.Fatalf("creature (%d,%d) lost marker contrast at (%d,%d): want %v got %v", index%3, index/3, i/4%width, i/4/width, wanted[i:i+4], after[i:i+4])
								}
								side := 0
								if float64(i/4%width) >= middle {
									side = 1
								}
								if wanted[i] == faceColor.R && wanted[i+1] == faceColor.G && wanted[i+2] == faceColor.B {
									faces[side]++
								} else {
									require.Equal(t, []byte{0, 0, 0}, wanted[i:i+3])
									borders[side]++
								}
								require.Equal(t, byte(255), wanted[i+3])
							}
							if visible {
								for side := range 2 {
									require.Positive(t, faces[side], "creature %d side %d blue arm", index, side)
									require.Positive(t, borders[side], "creature %d side %d full black border", index, side)
								}
							}
						}
						for i := 0; i < len(before); i += 4 {
							if !allowed[i/4] && !bytes.Equal(before[i:i+4], after[i:i+4]) {
								t.Fatalf("marker changed unrelated pixels at (%d,%d)", i/4%width, i/4/width)
							}
						}
						if aimCenter {
							drawCombatReticle(screen, options[4])
							assertOriginalReticlePixels(t, screen, options[4], nil)
						}
					})
				}
			}
		}
	}
}

// Keep production transforms in native screen coordinates while allocating only
// the actor footprints plus four source pixels of guard for outlines and cues.
func combatFixtureRasterBounds(options *ebiten.DrawImageOptions) image.Rectangle {
	var bounds image.Rectangle
	for _, point := range []image.Point{{-4, -4}, {22, -4}, {-4, 22}, {22, 22}} {
		x, y := options.GeoM.Apply(float64(point.X), float64(point.Y))
		bounds = bounds.Union(image.Rect(int(math.Floor(x)), int(math.Floor(y)), int(math.Ceil(x))+1, int(math.Ceil(y))+1))
	}
	return bounds
}

func newCombatMarkerTestScene(t *testing.T) (*GameScene, game_state.CombatTurnIdentity) {
	t.Helper()
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(1366, 768)
	scene, actor := setupCombatAutoAttackInputTest(t)
	// Keep the fixture's sustaining enemy out of the marker sample. Dedicated
	// dense-neighbor cases cover intentional HP/marker overlap separately.
	scene.gameState.CombatScene.MonsterManager.ForEachActive(func(_ int, monster *game_state.CombatMonster) {
		monster.SetPosition(references.Position{X: 10, Y: 0})
	})
	scene.gameConfig = scene.gameState.GameConfig
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	scene.mapImage = ebiten.NewImage(304, 208)
	t.Cleanup(scene.mapImage.Dispose)

	return scene, actor
}

// Recorded Redux cursor baseline (db440b0): axis-aligned, butt-ended strokes
// around (1,1)..(17,17), 32% corner arms, a centred six-pixel plus, and a
// two-pixel stroke scaled by the smaller axis. This independent rectangle
// raster does not call the production stroke/geometry helpers or their constants.
func assertOriginalReticlePixels(t *testing.T, screen *ebiten.Image, options *ebiten.DrawImageOptions, before []byte) {
	t.Helper()
	bounds := screen.Bounds()
	expected := ebiten.NewImageWithOptions(bounds, &ebiten.NewImageOptions{Unmanaged: true})
	defer expected.Dispose()
	if before != nil {
		require.Len(t, before, bounds.Dx()*bounds.Dy()*4)
		expected.WritePixels(before)
	}
	drawOriginalReticleReference(expected, options)
	wanted, actual := signaturePixels(expected), signaturePixels(screen)
	checked := 0
	for pixel := 0; pixel < len(wanted); pixel += 4 {
		if before == nil && wanted[pixel+3] == 0 {
			continue
		}
		checked++
		if !bytes.Equal(wanted[pixel:pixel+4], actual[pixel:pixel+4]) {
			t.Fatalf("original-size cursor at (%d,%d): want %v got %v", bounds.Min.X+pixel/4%bounds.Dx(),
				bounds.Min.Y+pixel/4/bounds.Dx(), wanted[pixel:pixel+4], actual[pixel:pixel+4])
		}
	}
	require.Positive(t, checked)
}

func drawOriginalReticleReference(expected *ebiten.Image, options *ebiten.DrawImageOptions) {
	x1, y1 := options.GeoM.Apply(1, 1)
	x2, y2 := options.GeoM.Apply(17, 17)
	l, top, r, bottom := float32(x1), float32(y1), float32(x2), float32(y2)
	w, h := float32(x2-x1), float32(y2-y1)
	half := min(w, h) / 16
	ax, ay := w*0.32, h*0.32
	cx, cy := (l+r)/2, (top+bottom)/2
	faces := [10][4]float32{
		{l, top - half, l + ax, top + half}, {l - half, top, l + half, top + ay},
		{r - ax, top - half, r, top + half}, {r - half, top, r + half, top + ay},
		{l, bottom - half, l + ax, bottom + half}, {l - half, bottom - ay, l + half, bottom},
		{r - ax, bottom - half, r, bottom + half}, {r - half, bottom - ay, r + half, bottom},
		{cx - 3*half, cy - half, cx + 3*half, cy + half}, {cx - half, cy - 3*half, cx + half, cy + 3*half},
	}
	for _, shadow := range []bool{true, false} {
		offset, clr := float32(0), ucolor.EGAWhite
		if shadow {
			offset, clr = half, ucolor.EGABlack
		}
		for _, rect := range faces {
			vector.FillRect(expected, rect[0]+offset, rect[1]+offset, rect[2]-rect[0], rect[3]-rect[1], clr, false)
		}
	}
}

const wakeReticleRaster = `..................
..................
..................
..................
........BBB.......
.......BBCBB......
.......BCCCB......
.......BBCBB......
........BBB.......
..................
..................
..................
..................
..................
..................
..................
..................
..................`

// A nil before buffer checks only marker pixels in a production composition.
// Supplying the pre-draw raster also checks every unmarked pixel, including
// viewport guards, open sprite interior, HP and neighboring cells.
func assertCombatMarkerPixels(
	t *testing.T, screen *ebiten.Image, options *ebiten.DrawImageOptions,
	stencil string, frameColor, centerColor color.Color, before []byte,
) {
	t.Helper()
	bounds := screen.Bounds()
	actual := make([]byte, bounds.Dx()*bounds.Dy()*4)
	screen.ReadPixels(actual)
	wanted := make([]byte, len(actual))
	if before != nil {
		require.Len(t, before, len(actual))
		copy(wanted, before)
	}
	marked := make([]bool, len(actual)/4)
	for y, row := range strings.Split(stencil, "\n") {
		for x, pixel := range row {
			if pixel == '.' {
				continue
			}
			clr := color.Color(ucolor.EGABlack)
			switch pixel {
			case 'W':
				clr = frameColor
			case 'C':
				clr = centerColor
			case 'B':
			default:
				t.Fatalf("unknown stencil pixel %q", pixel)
			}
			r, g, b, a := ucolor.ExtractRGBA(clr)
			left, top := options.GeoM.Apply(float64(x), float64(y))
			right, bottom := options.GeoM.Apply(float64(x+1), float64(y+1))
			for sy := max(int(math.Round(top)), bounds.Min.Y); sy < min(int(math.Round(bottom)), bounds.Max.Y); sy++ {
				for sx := max(int(math.Round(left)), bounds.Min.X); sx < min(int(math.Round(right)), bounds.Max.X); sx++ {
					offset := (sy-bounds.Min.Y)*bounds.Dx() + sx - bounds.Min.X
					marked[offset] = true
					copy(wanted[offset*4:offset*4+4], []byte{r, g, b, a})
				}
			}
		}
	}
	count := 0
	for pixel, isMarked := range marked {
		if isMarked {
			count++
		}
		if (isMarked || before != nil) && !bytes.Equal(wanted[pixel*4:pixel*4+4], actual[pixel*4:pixel*4+4]) {
			t.Fatalf("marker raster at (%d,%d), marked=%t: want %v got %v",
				bounds.Min.X+pixel%bounds.Dx(), bounds.Min.Y+pixel/bounds.Dx(), isMarked,
				wanted[pixel*4:pixel*4+4], actual[pixel*4:pixel*4+4])
		}
	}
	require.Positive(t, count, "fixture must sample the marker")
}

func TestCombatReticleOriginalFootprintAndClippedFractionalGeometry(t *testing.T) {
	for _, scale := range []struct{ x, y float64 }{{1, 1}, {2, 3}, {1366 * 0.76 / 304, 768.0 / 208}} {
		for _, background := range []string{"light", "dark", "brick"} {
			for _, clipped := range []bool{false, true} {
				t.Run(fmt.Sprintf("%gx%g/%s/clip%t", scale.x, scale.y, background, clipped), func(t *testing.T) {
					options := &ebiten.DrawImageOptions{}
					options.GeoM.Scale(scale.x, scale.y)
					options.GeoM.Translate(6.35, 6.65)
					bounds := image.Rect(0, 0, 80, 80)
					if clipped {
						bounds = image.Rect(12, 12, 48, 40)
					}
					screen := ebiten.NewImageWithOptions(bounds, &ebiten.NewImageOptions{Unmanaged: true})
					defer screen.Dispose()
					pixels := make([]byte, bounds.Dx()*bounds.Dy()*4)
					for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
						for x := bounds.Min.X; x < bounds.Max.X; x++ {
							clr := ucolor.EGALightGray
							if background == "dark" {
								clr = ucolor.EGABlue
							} else if background == "brick" {
								clr = ucolor.EGABrown
								if y%7 == 0 || (x+(y/7%2)*5)%11 == 0 {
									clr = ucolor.EGARed
								}
							}
							offset := ((y-bounds.Min.Y)*bounds.Dx() + x - bounds.Min.X) * 4
							copy(pixels[offset:offset+4], []byte{clr.R, clr.G, clr.B, clr.A})
						}
					}
					screen.WritePixels(pixels)
					drawCombatReticle(screen, options)
					assertOriginalReticlePixels(t, screen, options, pixels)
				})
			}
		}
	}
}

func TestCombatActiveFrameDenseProductionAboveHealthAndBelowSelectedAim(t *testing.T) {
	scene, party := newCombatMarkerTestScene(t)
	state, combat := scene.gameState, scene.gameState.CombatScene
	combat.SetPartyMemberPosition(party.EntityIndex, references.Position{X: 1, Y: 1})
	combat.MonsterManager.Clear()
	combat.TurnQueue.RemoveAllMonsters()
	var options [9]*ebiten.DrawImageOptions
	var sample image.Rectangle
	var centreIndex int
	for index := range 9 {
		position := references.Position{X: references.Coordinate(index%3 + 3), Y: references.Coordinate(index/3 + 3)}
		monsterIndex, monster := signatureMonster(t, scene, references.MonsterOrc, position)
		monster.SetCurrentHP(monster.MaxHP() - 1) // The battlefield marks only the injured.
		if index%2 == 0 {
			monster.SetLoyalty(game_state.CombatMonsterLoyaltyParty)
		}
		if index == 4 {
			centreIndex = monsterIndex
		}
		options[index] = scene.combatMonsterDrawOptions(position, monsterIndex)
		sample = sample.Union(combatFixtureRasterBounds(options[index]))
	}
	combat.ActiveMonsterIndex = centreIndex
	combat.CurrentTurnIdentity = game_state.CombatTurnIdentity{EntityIndex: centreIndex}
	state.ElapsedMs = 450 // The expanded corners overlap the northern actor's HP rim.
	require.LessOrEqual(t, sample.Dx()*sample.Dy(), 256*256)
	screen := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	protected := ebiten.NewImageWithOptions(sample, &ebiten.NewImageOptions{Unmanaged: true})
	defer screen.Dispose()
	defer protected.Dispose()
	scene.drawCombatHealthFrames(protected)
	wantedHP := signaturePixels(protected)
	before := snapshotBlockedCombatState(t, state)
	scene.drawAvatarOverlay(screen)
	assert.Equal(t, before, snapshotBlockedCombatState(t, state), "drawing selection must not advance gameplay")
	actual := signaturePixels(screen)
	overlaidHPPixels := 0
	for pixel := 0; pixel < len(actual); pixel += 4 {
		if wantedHP[pixel+3] != 0 && !bytes.Equal(wantedHP[pixel:pixel+4], actual[pixel:pixel+4]) {
			overlaidHPPixels++
		}
	}
	require.Positive(t, overlaidHPPixels, "the fixture must exercise actual active-marker/HP overlap")
	// The entire upper accents now win even where the northern HP rim overlaps.
	assertActiveCornerPixels(t, screen, options[4], state.ElapsedMs, ucolor.CombatActiveMonsterOutline, nil)
	assertActiveFrameEdges(t, screen, options[4], state.ElapsedMs, ucolor.CombatActiveMonsterOutline)
	combat.AttackTargeting = &game_state.CombatAttackTargeting{Cursor: references.Position{X: 4, Y: 4}}
	screen.Clear()
	scene.drawAvatarOverlay(screen)
	assertOriginalReticlePixels(t, screen, options[4], nil)
}

func TestCombatReticleRecordedNativeFaceExtents(t *testing.T) {
	screen := ebiten.NewImage(22, 22)
	defer screen.Dispose()
	options := &ebiten.DrawImageOptions{}
	options.GeoM.Translate(2, 2)
	drawCombatReticle(screen, options)
	// Recorded occupied native pixels at the original arm tips, outer edges and
	// centre cross. The former compact/HP-reserved design cannot satisfy these.
	for _, point := range []image.Point{{1, 0}, {0, 1}, {5, 1}, {1, 5}, {12, 1}, {16, 5},
		{1, 12}, {5, 17}, {12, 17}, {17, 12}, {6, 8}, {11, 9}, {8, 6}, {9, 11}} {
		r, g, b, a := ucolor.ExtractRGBA(screen.At(point.X+2, point.Y+2))
		assert.Equal(t, []uint8{255, 255, 255, 255}, []uint8{r, g, b, a}, "original face at %v", point)
	}
	for _, point := range []image.Point{{0, 0}, {6, 1}, {1, 6}, {9, 5}, {17, 17}, {9, 13}} {
		r, g, b, _ := ucolor.ExtractRGBA(screen.At(point.X+2, point.Y+2))
		assert.NotEqual(t, []uint8{255, 255, 255}, []uint8{r, g, b}, "original open interior/arm end at %v", point)
	}
}
