//nolint:mnd // Panel indices, logical coordinates, and stripe counts are verified original presentation facts.
package main

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	gametext "github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const (
	dungeonCanvasWidth       = 160
	dungeonCanvasHeight      = 164
	dungeonViewDepth         = 4
	dungeonMaxRenderOps      = 72
	dungeonFieldFrameMs      = 100
	dungeonFieldColorMask    = 0x03
	dungeonNormalCenterBase  = 8
	dungeonCenterPanelBase   = 12
	dungeonSpecialSideBase   = 20
	dungeonSpecialCenterBase = 24
	dungeonClipMinX          = 24
	dungeonClipMinY          = 28
	dungeonClipMaxX          = 137
	dungeonClipMaxY          = 164
	// dungeonSubduedUnlitBrightness is a fixed cosmetic multiplier and never affects gameplay light.
	dungeonSubduedUnlitBrightness = float32(0.17)
)

func dungeonDoorwayClipBounds() image.Rectangle {
	// ref:DNG-0009: original doorway contents clip to viewport (40,42)..(152,183). These
	// coordinates are normalized to the Redux canvas, with exclusive maxima.
	return image.Rect(dungeonClipMinX, dungeonClipMinY, dungeonClipMaxX, dungeonClipMaxY)
}

func compositeDungeonDoorwayClip(canvas, clipLayer *ebiten.Image) {
	bounds := dungeonDoorwayClipBounds()
	vertices := [...]ebiten.Vertex{
		dungeonDoorwayClipVertex(bounds.Min.X, bounds.Min.Y),
		dungeonDoorwayClipVertex(bounds.Max.X, bounds.Min.Y),
		dungeonDoorwayClipVertex(bounds.Min.X, bounds.Max.Y),
		dungeonDoorwayClipVertex(bounds.Max.X, bounds.Max.Y),
	}
	indices := [...]uint16{0, 1, 2, 1, 2, 3}
	options := ebiten.DrawTrianglesOptions{ //nolint:exhaustruct_v5 // Zero blending and nearest filtering are required.
		Address: ebiten.AddressClampToZero,
	}
	canvas.DrawTriangles(vertices[:], indices[:], clipLayer, &options)
}

func dungeonDoorwayClipVertex(x, y int) ebiten.Vertex {
	coordinateX := float32(x)
	coordinateY := float32(y)

	return ebiten.Vertex{
		DstX: coordinateX, DstY: coordinateY,
		SrcX: coordinateX, SrcY: coordinateY,
		ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1,
		Custom0: 0, Custom1: 0, Custom2: 0, Custom3: 0,
	}
}

type dungeonRenderOpKind byte

const (
	dungeonRenderPanel dungeonRenderOpKind = iota
	dungeonRenderItem
	dungeonRenderMonster
	dungeonRenderField
	dungeonRenderGraffiti
	dungeonRenderSkeletonEyes
	dungeonRenderStalactite
	dungeonRenderSkeletonCrumble
	dungeonRenderFieldDispel
)

type dungeonRenderRole byte

const (
	dungeonRenderCenter dungeonRenderRole = iota
	dungeonRenderLeft
	dungeonRenderRight
	dungeonRenderContents
)

type dungeonRenderOp struct {
	kind        dungeonRenderOpKind
	role        dungeonRenderRole
	position    references.Position
	depth       int
	spriteIndex int
	x           int
	y           int
	mirror      bool
	top         bool
	fieldColor  byte
	phase       byte
	dissolve    *DissolveEffectState
	fieldTimeMs int64
}

type dungeonRenderPlan struct {
	wallSet         int
	location        references.Location
	floor           references.FloorNumber
	atmosphereFrame uint64
	trueLight       bool
	dark            bool
	brightness      float32
	clipEnabled     bool
	clipFrom        int
	count           int
	ops             [dungeonMaxRenderOps]dungeonRenderOp
}

func (p *dungeonRenderPlan) append(op dungeonRenderOp) {
	if p.count >= len(p.ops) {
		panic("dungeon render plan capacity exceeded")
	}
	p.ops[p.count] = op
	p.count++
}

func (p dungeonRenderPlan) clipsOperation(index int) bool {
	return p.clipEnabled && index >= p.clipFrom
}

type dungeonSpriteSource interface {
	Panel(wallSet, panel int) (*ebiten.Image, bool)
	SpecialWallPanelDelta(wallSet, specialPanel int) (*ebiten.Image, bool)
	Item(item int) (*ebiten.Image, bool)
	Monster(monster, halfFrame int) (*ebiten.Image, bool)
}

type dungeonViewport struct {
	x      float64
	y      float64
	width  float64
	height float64
}

type dungeonAspectFit struct {
	x     float64
	y     float64
	scale float64
}

func dungeonWallSetForLocation(location references.Location) (int, bool) {
	//nolint:exhaustive // The default intentionally rejects every non-dungeon location.
	switch location {
	case references.Despise, references.Destard, references.Doom:
		return 0, true
	case references.Shame, references.Hythloth:
		return 1, true
	case references.Deceit, references.Wrong, references.Covetous:
		return 2, true
	default:
		return 0, false
	}
}

type dungeonRenderPresentation struct {
	environmentTimeMs int64
	creatureTimeMs    int64
	state             *game_state.GameState
	fieldDispels      []dungeonFieldDispelEffect
}

func buildDungeonRenderPlanWithPresentation(
	snapshot *game_state.DungeonStateSnapshot,
	player references.PlayerLocation,
	lit bool,
	showSubduedUnlit bool,
	presentation dungeonRenderPresentation,
) dungeonRenderPlan {
	wallSet := validateDungeonRenderSource(snapshot, player.Location)
	plan := dungeonRenderPlan{ //nolint:exhaustruct_v5 // Count and operations begin at their required zero values.
		wallSet:         wallSet,
		location:        player.Location,
		floor:           player.Floor,
		atmosphereFrame: uint64(max(presentation.environmentTimeMs, 0) / dungeonAtmosphereFrameMs),
		trueLight:       lit,
		dark:            !lit && !showSubduedUnlit,
		brightness:      1,
	}
	if plan.dark {
		return plan
	}
	if !lit {
		// REDUX DEVIATION: DNG-0009: original darkness is black when torch and
		// magical light are both absent. Redux can show the same geometry at fixed 17% brightness.
		plan.brightness = dungeonSubduedUnlitBrightness
	}
	validateLitDungeonRenderState(snapshot, player)

	var contentPositions [dungeonViewDepth]references.Position
	var contentCells [dungeonViewDepth]references.DungeonCell
	contentCount := 0
	position := player.Position
	for depth := range dungeonViewDepth {
		cell := dungeonCellForRender(snapshot, player.Floor, position)
		if !appendDungeonCenter(&plan, cell, position, depth) {
			break
		}

		appendDungeonSides(&plan, snapshot, player.Floor, cell, position, depth)
		contentPositions[contentCount] = position
		contentCells[contentCount] = cell
		contentCount++
		position = dungeonRelativePosition(position, snapshot.Facing, 1, 0)
	}

	for index := contentCount - 1; index >= 0; index-- {
		appendDungeonFieldDispels(&plan, presentation.state, presentation.fieldDispels,
			contentPositions[index], index, presentation.environmentTimeMs)
		appendDungeonContents(&plan, contentCells[index], contentPositions[index], index)
		appendDungeonWanderer(
			&plan,
			snapshot.Wanderer,
			contentPositions[index],
			index,
			presentation.creatureTimeMs,
		)
	}

	return plan
}

func validateDungeonRenderSource(
	snapshot *game_state.DungeonStateSnapshot,
	location references.Location,
) int {
	if snapshot == nil {
		panic("dungeon render snapshot is nil")
	}
	wallSet, ok := dungeonWallSetForLocation(location)
	if !ok {
		panic(fmt.Sprintf("location %s has no dungeon presentation", location))
	}

	return wallSet
}

func validateLitDungeonRenderState(
	snapshot *game_state.DungeonStateSnapshot,
	player references.PlayerLocation,
) {
	if !references.IsDungeonCoordinate(player.Floor, player.Position) {
		panic("dungeon player location is outside the active 8x8x8 map")
	}
	if !snapshot.Facing.IsValid() {
		panic("dungeon render snapshot has invalid facing")
	}
}

func appendDungeonSides(
	plan *dungeonRenderPlan,
	snapshot *game_state.DungeonStateSnapshot,
	floor references.FloorNumber,
	centerCell references.DungeonCell,
	position references.Position,
	depth int,
) {
	// A doorway occupied at depth zero already supplies both visible jambs.
	if depth == 0 && centerCell.Raw() == 0xe0 {
		return
	}

	left := dungeonRelativePosition(position, snapshot.Facing, 0, 1)
	right := dungeonRelativePosition(position, snapshot.Facing, 0, -1)
	appendDungeonSide(plan, dungeonCellForRender(snapshot, floor, left), left, depth, false)
	appendDungeonSide(plan, dungeonCellForRender(snapshot, floor, right), right, depth, true)
}

func dungeonCellForRender(
	snapshot *game_state.DungeonStateSnapshot,
	floor references.FloorNumber,
	position references.Position,
) references.DungeonCell {
	cell, ok := snapshot.Cells.Cell(floor, position)
	if !ok {
		panic("dungeon renderer sampled outside the wrapped map")
	}

	return cell
}

func dungeonRelativePosition(
	position references.Position,
	facing references.DungeonFacing,
	forward, left int,
) references.Position {
	var forwardX, forwardY, leftX, leftY int
	switch facing {
	case references.DungeonFacingNorth:
		forwardY, leftX = -1, -1
	case references.DungeonFacingEast:
		forwardX, leftY = 1, -1
	case references.DungeonFacingSouth:
		forwardY, leftX = 1, 1
	case references.DungeonFacingWest:
		forwardX, leftY = -1, 1
	default:
		panic("invalid dungeon facing")
	}

	return references.Position{
		X: references.WrapDungeonCoordinate(int(position.X) + forward*forwardX + left*leftX),
		Y: references.WrapDungeonCoordinate(int(position.Y) + forward*forwardY + left*leftY),
	}
}

func appendDungeonCenter(
	plan *dungeonRenderPlan,
	cell references.DungeonCell,
	position references.Position,
	depth int,
) bool {
	if cell.Kind() < references.DungeonClearedRoom {
		return true
	}

	panel := dungeonCenterPanelIndex(cell.Kind(), depth)
	appendDungeonPanel(plan, panel, dungeonRenderCenter, position, depth, false)
	appendDungeonPanel(plan, panel, dungeonRenderCenter, position, depth, true)
	appendDungeonCenterAtmospherics(plan, cell, position, depth)

	currentDoorway := depth == 0 && cell.Kind() == references.DungeonRevealedDoor
	if currentDoorway {
		plan.clipEnabled = true
		plan.clipFrom = plan.count
	}

	return currentDoorway
}

func dungeonCenterPanelIndex(kind references.DungeonCellKind, depth int) int {
	if depth == 0 {
		return dungeonCenterPanelBase
	}

	var base int
	//nolint:exhaustive // appendDungeonCenter guards this helper to structural classes 0xA through 0xF.
	switch kind {
	case references.DungeonWall, references.DungeonHiddenDoor:
		base = 8
	case references.DungeonSpecialWall:
		base = dungeonSpecialCenterBase
	case references.DungeonClearedRoom, references.DungeonRevealedDoor, references.DungeonLiveRoom:
		base = 12
	default:
		panic("center panel requested for a non-structural dungeon cell")
	}

	return base + depth
}

func appendDungeonSide(
	plan *dungeonRenderPlan,
	cell references.DungeonCell,
	position references.Position,
	depth int,
	right bool,
) {
	panel := 16 + depth
	if cell.Kind() >= references.DungeonClearedRoom {
		//nolint:exhaustive // The default is the shared solid-side treatment for wall and hidden-door classes.
		switch cell.Kind() {
		case references.DungeonSpecialWall:
			panel = dungeonSpecialSideBase + depth
		case references.DungeonClearedRoom, references.DungeonRevealedDoor, references.DungeonLiveRoom:
			panel = 4 + depth
		default:
			panel = depth
		}
	}
	role := dungeonRenderLeft
	if right {
		role = dungeonRenderRight
	}
	appendDungeonPanel(plan, panel, role, position, depth, right)
	appendDungeonSideAtmospherics(plan, cell, position, depth, right)
}

func appendDungeonPanel(
	plan *dungeonRenderPlan,
	panel int,
	role dungeonRenderRole,
	position references.Position,
	depth int,
	right bool,
) {
	xPosition := dungeonLeftPanelX(depth)
	if right {
		xPosition = dungeonRightPanelX(depth)
		if panel >= 8 && panel <= 15 || panel >= 24 && panel <= 27 {
			xPosition = 80
		}
	}
	plan.append(dungeonRenderOp{
		kind:        dungeonRenderPanel,
		role:        role,
		position:    position,
		depth:       depth,
		spriteIndex: panel,
		x:           xPosition,
		y:           0,
		mirror:      right,
		top:         false,
		fieldColor:  0,
		phase:       0,
		dissolve:    nil,
		fieldTimeMs: 0,
	})
}

func dungeonLeftPanelX(depth int) int {
	switch depth {
	case 0:
		return 0
	case 1:
		return 24
	case 2:
		return 56
	case 3:
		return 72
	default:
		panic("invalid dungeon panel depth")
	}
}

func dungeonRightPanelX(depth int) int {
	switch depth {
	case 0:
		return 136
	case 1:
		return 104
	case 2:
		return 88
	case 3:
		return 80
	default:
		panic("invalid dungeon panel depth")
	}
}

func appendDungeonContents(
	plan *dungeonRenderPlan,
	cell references.DungeonCell,
	position references.Position,
	depth int,
) {
	kind := cell.Kind()
	switch {
	case kind >= references.DungeonLadderUp && kind <= references.DungeonLadderBoth:
		appendDungeonLadders(plan, kind, position, depth)
	case kind >= references.DungeonClosedChest && kind <= references.DungeonOpenChest:
		appendDungeonFloorObject(plan, cell, position, depth)
	case kind == references.DungeonEnergyField:
		plan.append(dungeonRenderOp{
			kind:        dungeonRenderField,
			role:        dungeonRenderContents,
			position:    position,
			depth:       depth,
			spriteIndex: 0,
			x:           0,
			y:           0,
			mirror:      false,
			top:         false,
			fieldColor:  cell.Payload() & dungeonFieldColorMask,
			phase:       0,
			dissolve:    nil,
			fieldTimeMs: 0,
		})
	}

	if kind < references.DungeonReserved && cell.HasBit08() {
		appendDungeonItemPair(plan, 8+depth, true, position, depth)
	}
}

func appendDungeonLadders(
	plan *dungeonRenderPlan,
	kind references.DungeonCellKind,
	position references.Position,
	depth int,
) {
	if kind == references.DungeonLadderUp || kind == references.DungeonLadderBoth {
		appendDungeonItemPair(plan, depth, true, position, depth)
	}
	if kind == references.DungeonLadderDown || kind == references.DungeonLadderBoth {
		appendDungeonItemPair(plan, depth, false, position, depth)
		if kind == references.DungeonLadderBoth {
			// At every depth the flipped upper artwork ends at exclusive row 81.
			// Join the lower pair there instead of row 82; standalone ladders
			// retain their original placement and shared masked artwork.
			plan.ops[plan.count-2].y--
			plan.ops[plan.count-1].y--
		}
	}
}

func appendDungeonFloorObject(
	plan *dungeonRenderPlan,
	cell references.DungeonCell,
	position references.Position,
	depth int,
) {
	//nolint:exhaustive // appendDungeonContents guards this helper to classes 0x4 through 0x7.
	switch cell.Kind() {
	case references.DungeonClosedChest:
		appendDungeonItemPair(plan, 12+depth, false, position, depth)
	case references.DungeonFountain:
		appendDungeonItemPair(plan, 4+depth, false, position, depth)
	case references.DungeonPitOrTrap:
		if cell.Payload()&0x07 == 0 {
			appendDungeonItemPair(plan, 8+depth, false, position, depth)
		}
	case references.DungeonOpenChest:
		appendDungeonItemPair(plan, 16+depth, false, position, depth)
	default:
		panic("dungeon floor-object helper received a non-object cell")
	}
}

func appendDungeonItemPair(
	plan *dungeonRenderPlan,
	item int,
	top bool,
	position references.Position,
	depth int,
) {
	xPosition, yPosition := dungeonItemPosition(item, depth, top, false)
	plan.append(dungeonRenderOp{
		kind:        dungeonRenderItem,
		role:        dungeonRenderContents,
		position:    position,
		depth:       depth,
		spriteIndex: item,
		x:           xPosition,
		y:           yPosition,
		mirror:      false,
		top:         top,
		fieldColor:  0,
		phase:       0,
		dissolve:    nil,
		fieldTimeMs: 0,
	})
	xPosition, yPosition = dungeonItemPosition(item, depth, top, true)
	plan.append(dungeonRenderOp{
		kind:        dungeonRenderItem,
		role:        dungeonRenderContents,
		position:    position,
		depth:       depth,
		spriteIndex: item,
		x:           xPosition,
		y:           yPosition,
		mirror:      true,
		top:         top,
		fieldColor:  0,
		phase:       0,
		dissolve:    nil,
		fieldTimeMs: 0,
	})
}

func dungeonItemPosition(item, depth int, top, right bool) (int, int) {
	if depth < 0 || depth >= dungeonViewDepth {
		panic("invalid dungeon item depth")
	}
	leftPositions := [dungeonViewDepth]int{40, 56, 64, 72}
	topPositions := [dungeonViewDepth]int{1, 25, 57, 73}
	bottomPositions := [dungeonViewDepth]int{138, 106, 90, 82}
	xPosition := leftPositions[depth]
	if right {
		xPosition = 80
	}

	if item < 8 && !top {
		return xPosition, 82
	}
	if top {
		return xPosition, topPositions[depth]
	}

	return xPosition, bottomPositions[depth]
}

func drawDungeonRenderPlan(
	canvas *ebiten.Image,
	clipLayer *ebiten.Image,
	source dungeonSpriteSource,
	graffitiOutput *gametext.Output,
	plan dungeonRenderPlan,
	presentationTimeMs int64,
) {
	validateDungeonRenderTargets(canvas, clipLayer, source, graffitiOutput, plan)
	canvas.Fill(color.Black)
	if plan.dark {
		return
	}
	if plan.clipEnabled {
		clipLayer.Clear()
	}

	for index := range plan.count {
		target := canvas
		if plan.clipsOperation(index) {
			target = clipLayer
		}
		drawDungeonRenderOp(
			target,
			source,
			graffitiOutput,
			plan.wallSet,
			plan.ops[index],
			presentationTimeMs,
			plan.brightness,
		)
	}
	if plan.clipEnabled {
		compositeDungeonDoorwayClip(canvas, clipLayer)
	}
}

func validateDungeonRenderTargets(
	canvas *ebiten.Image,
	clipLayer *ebiten.Image,
	source dungeonSpriteSource,
	graffitiOutput *gametext.Output,
	plan dungeonRenderPlan,
) {
	if canvas == nil || canvas.Bounds().Size() != image.Pt(dungeonCanvasWidth, dungeonCanvasHeight) {
		panic("dungeon renderer requires a 160x164 canvas")
	}
	if source == nil {
		panic("dungeon sprite source is nil")
	}
	if graffitiOutput == nil {
		panic("dungeon graffiti output is nil")
	}
	if clipLayer == nil || clipLayer.Bounds().Size() != image.Pt(dungeonCanvasWidth, dungeonCanvasHeight) {
		panic("dungeon renderer requires a 160x164 clip layer")
	}
	if canvas == clipLayer {
		panic("dungeon renderer canvas and clip layer must be distinct")
	}
	if plan.clipEnabled && (plan.clipFrom < 0 || plan.clipFrom > plan.count) {
		panic("dungeon render plan has an invalid clip boundary")
	}
}

func drawDungeonRenderOp(
	canvas *ebiten.Image,
	source dungeonSpriteSource,
	graffitiOutput *gametext.Output,
	wallSet int,
	command dungeonRenderOp,
	presentationTimeMs int64,
	brightness float32,
) {
	switch command.kind {
	case dungeonRenderPanel:
		panel, ok := source.Panel(wallSet, command.spriteIndex)
		if !ok {
			panic(fmt.Sprintf("dungeon wall set %d panel %d is unavailable", wallSet, command.spriteIndex))
		}
		drawDungeonSprite(canvas, panel, command, brightness)
	case dungeonRenderItem:
		drawDungeonItem(canvas, source, command, brightness)
	case dungeonRenderMonster:
		monster, ok := source.Monster(command.spriteIndex, int(command.phase))
		if !ok {
			panic(fmt.Sprintf(
				"dungeon monster set %d half-frame %d is unavailable",
				command.spriteIndex,
				command.phase,
			))
		}
		drawDungeonSprite(canvas, monster, command, brightness)
	case dungeonRenderField:
		drawDungeonField(canvas, command, presentationTimeMs, brightness)
	case dungeonRenderFieldDispel:
		drawDungeonFieldDispel(canvas, command, brightness)
	case dungeonRenderGraffiti:
		drawDungeonGraffiti(canvas, graffitiOutput, command)
	case dungeonRenderSkeletonEyes:
		drawDungeonSkeletonEyes(canvas, command)
	case dungeonRenderStalactite:
		drawDungeonStalactite(canvas, command)
	case dungeonRenderSkeletonCrumble:
		delta, ok := source.SpecialWallPanelDelta(wallSet, command.spriteIndex)
		if !ok {
			panic(fmt.Sprintf("dungeon wall set %d crumble delta %d is unavailable", wallSet, command.spriteIndex))
		}
		drawDungeonSkeletonCrumble(canvas, delta, command, brightness)
	default:
		panic("unknown dungeon render operation")
	}
}

func drawDungeonItem(canvas *ebiten.Image, source dungeonSpriteSource, command dungeonRenderOp, brightness float32) {
	item, ok := source.Item(command.spriteIndex)
	if !ok {
		panic(fmt.Sprintf("dungeon item %d is unavailable", command.spriteIndex))
	}
	if command.dissolve != nil {
		drawDungeonPitReveal(canvas, item, command, brightness)
	} else {
		drawDungeonSprite(canvas, item, command, brightness)
	}
}

func drawDungeonSprite(canvas, sprite *ebiten.Image, command dungeonRenderOp, brightness float32) {
	var options ebiten.DrawImageOptions
	scaleX := 1.0
	scaleY := 1.0
	translateX := float64(command.x)
	translateY := float64(command.y)
	if command.mirror {
		scaleX = -1
		translateX += float64(sprite.Bounds().Dx())
	}
	if command.top {
		scaleY = -1
		translateY += float64(sprite.Bounds().Dy())
	}
	options.GeoM.Scale(scaleX, scaleY)
	options.GeoM.Translate(translateX, translateY)
	options.ColorScale.Scale(brightness, brightness, brightness, 1)
	canvas.DrawImage(sprite, &options)
}

func drawDungeonField(
	canvas *ebiten.Image,
	command dungeonRenderOp,
	presentationTimeMs int64,
	brightness float32,
) {
	drawDungeonFieldAt(canvas, command, presentationTimeMs, brightness, 0, 0)
}

// drawDungeonFieldAt keeps the ordinary seed and inclusive stripe geometry;
// offsets allow the identical field to be reconstructed in a cropped buffer.
func drawDungeonFieldAt(
	canvas *ebiten.Image,
	command dungeonRenderOp,
	presentationTimeMs int64,
	brightness float32,
	offsetX, offsetY int,
) {
	minimumX, maximumX, minimumY, maximumY, count, lineLength := dungeonFieldGeometry(command.depth)
	fieldColor := dungeonFieldColor(command.fieldColor)
	fieldColor.R = uint8(float32(fieldColor.R) * brightness)
	fieldColor.G = uint8(float32(fieldColor.G) * brightness)
	fieldColor.B = uint8(float32(fieldColor.B) * brightness)
	frame := uint64(max(presentationTimeMs, 0) / dungeonFieldFrameMs)
	positionX := uint64(command.position.X) //nolint:gosec // Coordinates are validated as 0..7.
	positionY := uint64(command.position.Y) //nolint:gosec // Coordinates are validated as 0..7.
	depth := uint64(command.depth)          //nolint:gosec // Depth is validated as 0..3.
	seed := positionX | positionY<<4 | depth<<8 | frame<<12
	xSpan := uint64(maximumX - minimumX - lineLength + 1) //nolint:gosec // Verified positive span.
	ySpan := uint64(maximumY - minimumY + 1)              //nolint:gosec // Verified positive span.

	// REDUX DEVIATION: DNG-0009: original field animation randomly redraws 300/100/50/15 horizontal field
	// segments each frame. Redux derives stripe placement from presentation time and cell/depth so
	// rendering never consumes gameplay RNG.
	for range count {
		seed = seed*1664525 + 1013904223
		xPosition := minimumX + int(seed%xSpan) //nolint:gosec // Modulo result is bounded by the field canvas.
		seed = seed*1664525 + 1013904223
		yPosition := minimumY + int(seed%ySpan) //nolint:gosec // Modulo result is bounded by the field canvas.
		vector.FillRect(
			canvas,
			float32(xPosition+offsetX),
			float32(yPosition+offsetY),
			float32(lineLength+1),
			1,
			fieldColor,
			false,
		)
	}
}

func dungeonFieldGeometry(depth int) (int, int, int, int, int, int) {
	switch depth {
	case 0:
		return 0, 151, 2, 153, 300, 7
	case 1:
		return 40, 119, 42, 121, 100, 7
	case 2:
		return 64, 95, 66, 97, 50, 5
	case 3:
		return 76, 83, 78, 85, 15, 2
	default:
		panic("invalid dungeon field depth")
	}
}

func dungeonFieldColor(fieldColor byte) color.RGBA {
	switch fieldColor {
	case 0:
		return ucolor.EGALightMagenta.ToRGBA()
	case 1:
		return ucolor.EGALightGreen.ToRGBA()
	case 2:
		return ucolor.EGALightRed.ToRGBA()
	case 3:
		return ucolor.EGALightBlue.ToRGBA()
	default:
		panic("invalid dungeon field color")
	}
}

func dungeonMapViewport(bounds image.Rectangle) dungeonViewport {
	return dungeonViewport{
		x:      float64(bounds.Min.X) + float64(bounds.Dx())*gameScreenPercents.StartPercentX,
		y:      float64(bounds.Min.Y) + float64(bounds.Dy())*gameScreenPercents.StartPercentY,
		width:  float64(bounds.Dx()) * (gameScreenPercents.EndPercentX - gameScreenPercents.StartPercentX),
		height: float64(bounds.Dy()) * (gameScreenPercents.EndPercentY - gameScreenPercents.StartPercentY),
	}
}

func dungeonCanvasAspectFit(viewport dungeonViewport) dungeonAspectFit {
	scale := math.Min(viewport.width/dungeonCanvasWidth, viewport.height/dungeonCanvasHeight)

	return dungeonAspectFit{
		x:     viewport.x + (viewport.width-dungeonCanvasWidth*scale)/2,
		y:     viewport.y + (viewport.height-dungeonCanvasHeight*scale)/2,
		scale: scale,
	}
}

func (g *GameScene) drawDungeonViewport(screen *ebiten.Image) dungeonHUDStatus {
	snapshot, player, lit := g.currentDungeonRenderState()
	showSubduedUnlit := g.gameState.GameConfig != nil &&
		g.gameState.GameConfig.GameplayConfiguration != nil &&
		g.gameState.GameConfig.GameplayConfiguration.ShowSubduedUnlitDungeons
	plan := buildDungeonRenderPlanWithPresentation(
		&snapshot,
		player,
		lit,
		showSubduedUnlit,
		dungeonRenderPresentation{
			environmentTimeMs: g.gameState.ElapsedMs, creatureTimeMs: g.dungeonActorAnimationTime(),
			state: g.gameState, fieldDispels: g.dungeonFieldDispels,
		},
	)
	for index := range g.dungeonSkeletonCrumbles {
		appendDungeonSkeletonCrumble(&plan, &g.dungeonSkeletonCrumbles[index], g.gameState.ElapsedMs)
	}
	for index := range g.dungeonPitReveals {
		applyDungeonPitReveal(&plan, &g.dungeonPitReveals[index], g.gameState.ElapsedMs)
	}
	if g.dungeonCanvas == nil {
		g.dungeonCanvas = ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	}
	if g.dungeonClipLayer == nil {
		g.dungeonClipLayer = ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	}
	drawDungeonRenderPlan(
		g.dungeonCanvas,
		g.dungeonClipLayer,
		g.dungeonSprites,
		g.dungeonGraffitiOutput,
		plan,
		g.gameState.ElapsedMs,
	)

	viewport := dungeonMapViewport(screen.Bounds())
	vector.FillRect(
		screen,
		float32(viewport.x),
		float32(viewport.y),
		float32(viewport.width),
		float32(viewport.height),
		color.Black,
		false,
	)
	fit := dungeonCanvasAspectFit(viewport)
	if g.dungeonDrawOptions == nil {
		g.dungeonDrawOptions = &ebiten.DrawImageOptions{}
	}
	g.dungeonDrawOptions.GeoM.Reset()
	g.dungeonDrawOptions.GeoM.Scale(fit.scale, fit.scale)
	g.dungeonDrawOptions.GeoM.Translate(fit.x+g.screenShakeOffsetX, fit.y+g.screenShakeOffsetY)
	screen.DrawImage(g.dungeonCanvas, g.dungeonDrawOptions)

	return newDungeonHUDStatus(player, snapshot.Facing)
}

func (g *GameScene) currentDungeonRenderState() (
	game_state.DungeonStateSnapshot,
	references.PlayerLocation,
	bool,
) {
	if g.gameState == nil {
		panic("dungeon rendering requires game state")
	}
	player := g.gameState.MapState.PlayerLocation
	if player.Location.GetMapType() != references.DungeonMapType {
		panic("dungeon renderer invoked outside a dungeon")
	}
	snapshot, active := g.gameState.DungeonStateSnapshot()
	if !active {
		panic("dungeon map has no matching active dungeon snapshot")
	}
	if snapshot.Location != player.Location {
		panic("dungeon map has no matching active dungeon snapshot")
	}

	return snapshot, player, g.gameState.MapState.Lighting.HasAnyLight()
}

var _ dungeonSpriteSource = (*sprites.DungeonSprites)(nil)
