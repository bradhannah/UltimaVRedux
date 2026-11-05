//nolint:mnd // ref:INTRO-0003: original 320x200 composition, margins, and narrative offsets.
package main

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/visual"
	"github.com/bradhannah/UltimaVRedux/pkg/color"
	rng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

const (
	introNarrativeWorkflowName = "ultima_v_introduction"
	// REDUX DEVIATION: INTRO-0003: original Codex reveal delegates to a graphics-driver dissolve with no verified
	// duration. Redux uses a deterministic 1500 ms dissolve so progression remains reproducible and readable.
	introNarrativeFizzDuration = 1500
	introNarrativeFizzSeed     = 0x434F444558
)

type introNarrativeShape struct {
	archive string
	index   int
	x       int
	y       int
}

type introNarrativeText struct {
	storyBlock int
	literal    string
	x          int
	y          int
	outerLeft  int
	outerRight int
	innerLeft  int
	innerRight int
	upperY     int
	lowerY     int
	spaceWidth int
}

type introNarrativeFrame struct {
	id     minimap.FrameID
	shapes []introNarrativeShape
	texts  []introNarrativeText
	fizz   *introNarrativeShape
}

type introNarrativePresentation struct {
	assets         *sprites.NarrativeIntroAssets
	scene          *minimap.Scene
	executor       *minimap.ScriptExecutor
	currentFrame   minimap.FrameID
	frameStartedMs int64
	fizz           *visual.PixelDissolveEffect
	fizzBuffer     *ebiten.Image
}

//nolint:gochecknoglobals // ref:INTRO-0003: immutable illustrated narrative frame layout.
var introNarrativeFrames = [...]introNarrativeFrame{
	introFrame(1,
		[]introNarrativeShape{shape("TEXT.16", 0, 224, 30), shape("TEXT.16", 1, 168, 58), shape("STORY1.16", 0, 0, 0)},
		storyText(0, 180, 128, 180, 320, 0, 320, 180, 200)),
	introFrameWithFizz(2,
		[]introNarrativeShape{shape("STORY1.16", 1, 0, 74)},
		storyText(1, 0, 0, 0, 320, 172, 320, 70, 200),
		shape("STORY1.16", 2, 40, 86)),
	introFrame(3, []introNarrativeShape{shape("STORY2.16", 0, 136, 0)}, storyText(2, 0, 40, 0, 132, 0, 320, 131, 200)),
	introFrame(4, []introNarrativeShape{shape("STORY2.16", 1, 0, 38)}, storyText(3, 0, 0, 0, 320, 210, 320, 32, 160)),
	introFrame(5, []introNarrativeShape{shape("STORY2.16", 2, 152, 76)}, storyText(4, 0, 9, 0, 320, 0, 148, 70, 200)),
	introFrame(6, []introNarrativeShape{shape("STORY2.16", 2, 0, 0)}, storyText(5, 176, 0, 176, 320, 0, 320, 133, 200)),
	{
		id:   "intro-07",
		fizz: nil,
		shapes: []introNarrativeShape{
			shape("STORY2.16", 2, 72, 38), shape("STORY2.16", 3, 96, 39),
		},
		texts: []introNarrativeText{
			literalText("Instantly, a shimmering blue door springs up!", 32, 9),
			literalText("With heart beating rapidly, you step into it.", 32, 180),
		},
	},
	introFrame(8,
		[]introNarrativeShape{shape("TEXT.16", 0, 232, 26), shape("TEXT.16", 2, 200, 54), shape("STORY3.16", 0, 0, 0)},
		storyText(6, 188, 136, 188, 320, 0, 320, 168, 200)),
	introFrame(9, []introNarrativeShape{shape("STORY3.16", 1, 0, 82)}, storyText(7, 0, 0, 0, 320, 0, 320, 200, 200)),
	introFrame(10, []introNarrativeShape{shape("STORY4.16", 0, 0, 82)}, storyText(8, 0, 0, 0, 320, 0, 320, 200, 200)),
	introFrame(11, []introNarrativeShape{shape("STORY4.16", 1, 0, 82)}, storyText(9, 0, 0, 0, 320, 0, 320, 200, 200)),
	introFrame(12, []introNarrativeShape{shape("STORY5.16", 0, 0, 82)}, storyText(10, 0, 0, 0, 320, 0, 320, 200, 200)),
	introFrame(13, []introNarrativeShape{shape("STORY5.16", 1, 0, 82)}, storyText(11, 0, 0, 0, 320, 0, 320, 200, 200)),
	introFrame(14, []introNarrativeShape{shape("STORY6.16", 0, 176, 0)}, storyText(12, 0, 0, 0, 170, 0, 320, 114, 200)),
	introFrame(15,
		[]introNarrativeShape{shape("TEXT.16", 0, 184, 0), shape("TEXT.16", 3, 248, 0), shape("STORY6.16", 1, 0, 0)},
		storyText(13, 184, 32, 184, 320, 0, 320, 114, 200)),
	introFrame(16,
		[]introNarrativeShape{shape("STORY6.16", 2, 176, 0), shape("STORY6.16", 3, 176, 55)},
		storyText(14, 0, 0, 0, 170, 0, 320, 96, 200)),
	introFrame(17,
		[]introNarrativeShape{shape("STORY6.16", 6, 0, 46), shape("STORY6.16", 5, 0, 101)},
		storyText(15, 0, 0, 0, 320, 148, 320, 33, 137)),
	introFrame(18,
		[]introNarrativeShape{shape("STORY6.16", 4, 176, 78), shape("STORY6.16", 7, 176, 133)},
		storyText(16, 0, 0, 0, 320, 0, 170, 70, 200)),
	introFrame(19,
		[]introNarrativeShape{shape("STORY6.16", 2, 0, 0), shape("STORY6.16", 5, 0, 55)},
		storyText(17, 174, 0, 148, 320, 0, 320, 96, 200)),
	introFrame(20,
		[]introNarrativeShape{shape("STORY6.16", 6, 176, 55), shape("STORY6.16", 7, 176, 110)},
		storyText(18, 0, 9, 0, 320, 0, 170, 51, 146)),
	introFrame(21,
		[]introNarrativeShape{shape("STORY6.16", 4, 0, 87), shape("STORY6.16", 3, 0, 142)},
		storyText(19, 0, 0, 0, 320, 156, 320, 79, 200)),
}

func newIntroNarrativePresentation(assets *sprites.NarrativeIntroAssets) *introNarrativePresentation {
	scene := minimap.NewMiniMapScene()
	builder := minimap.NewMiniMapScript(introNarrativeWorkflowName)
	for _, frame := range introNarrativeFrames {
		builder.SetFrame(frame.id).AwaitAdvance()
	}
	script := builder.Build()
	executor := minimap.NewScriptExecutor(scene, script)
	executor.Update(0)

	return &introNarrativePresentation{
		assets: assets, scene: scene, executor: executor,
		currentFrame: "", frameStartedMs: 0, fizz: nil, fizzBuffer: nil,
	}
}

func (p *introNarrativePresentation) syncFrame(nowMs int64) {
	frameID := p.scene.GetCurrentFrame()
	if frameID == "" || frameID == p.currentFrame {
		return
	}
	p.currentFrame = frameID
	p.frameStartedMs = nowMs
	p.fizz = nil
	p.fizzBuffer = nil
	frame := p.frame()
	if frame.fizz == nil {
		return
	}
	img := p.shapeImage(*frame.fizz)
	size := img.Bounds().Size()
	p.fizz = visual.NewPixelDissolveEffect(
		size.X, size.Y, size.X, size.Y, visual.DissolveModeReveal,
		rng.New(introNarrativeFizzSeed),
	)
	p.fizzBuffer = ebiten.NewImage(size.X, size.Y)
}

func (p *introNarrativePresentation) canAdvance(nowMs int64) bool {
	return p.fizz == nil || nowMs-p.frameStartedMs >= introNarrativeFizzDuration
}

func (p *introNarrativePresentation) draw(canvas *ebiten.Image, nowMs int64) {
	p.syncFrame(nowMs)
	canvas.Fill(color.Black)
	frame := p.frame()
	for _, placement := range frame.shapes {
		drawIntroImage(canvas, p.shapeImage(placement), placement.x, placement.y)
	}
	for _, placement := range frame.texts {
		text := placement.literal
		if placement.storyBlock >= 0 {
			text = p.assets.StoryBlocks[placement.storyBlock]
		}
		drawIntroNarrativeText(canvas, p.assets, text, placement)
	}
	if frame.fizz == nil {
		return
	}
	progress := min(float64(nowMs-p.frameStartedMs)/introNarrativeFizzDuration, 1)
	p.fizz.UpdateProgress(progress)
	drawIntroDissolve(
		canvas, p.shapeImage(*frame.fizz), p.fizz, p.fizzBuffer,
		introImageDrawOptions(frame.fizz.x, frame.fizz.y),
	)
}

func (p *introNarrativePresentation) frame() introNarrativeFrame {
	for _, frame := range introNarrativeFrames {
		if frame.id == p.currentFrame {
			return frame
		}
	}
	panic(fmt.Sprintf("intro narrative selected unknown frame %q", p.currentFrame))
}

func (p *introNarrativePresentation) shapeImage(placement introNarrativeShape) *ebiten.Image {
	archive, ok := p.assets.Shapes[placement.archive]
	if !ok || placement.index < 0 || placement.index >= len(archive) {
		panic(fmt.Sprintf("intro narrative shape %s[%d] is unavailable", placement.archive, placement.index))
	}

	return archive[placement.index]
}

func introFrame(number int, shapes []introNarrativeShape, text introNarrativeText) introNarrativeFrame {
	return introNarrativeFrame{
		id: minimap.FrameID(fmt.Sprintf("intro-%02d", number)), shapes: shapes,
		texts: []introNarrativeText{text}, fizz: nil,
	}
}

func introFrameWithFizz(
	number int,
	shapes []introNarrativeShape,
	text introNarrativeText,
	fizz introNarrativeShape,
) introNarrativeFrame {
	frame := introFrame(number, shapes, text)
	frame.fizz = &fizz

	return frame
}

func shape(archive string, index, x, y int) introNarrativeShape {
	return introNarrativeShape{archive: archive, index: index, x: x, y: y}
}

func storyText(block, x, y, outerLeft, outerRight, innerLeft, innerRight, upperY, lowerY int) introNarrativeText {
	return introNarrativeText{
		storyBlock: block, literal: "", x: x, y: y,
		outerLeft: outerLeft, outerRight: outerRight, innerLeft: innerLeft, innerRight: innerRight,
		upperY: upperY, lowerY: lowerY, spaceWidth: 5,
	}
}

func literalText(text string, x, y int) introNarrativeText {
	result := storyText(-1, x, y, 0, introLogicalWidth, 0, introLogicalWidth, introLogicalHeight, introLogicalHeight)
	result.literal = text

	return result
}

func drawIntroNarrativeText(
	canvas *ebiten.Image,
	assets *sprites.NarrativeIntroAssets,
	text string,
	layout introNarrativeText,
) {
	const lineHeight = 9
	drawX, drawY, start := layout.x, layout.y, 0
	for start < len(text) {
		left, right := layout.outerLeft, layout.outerRight
		if drawY > layout.upperY && drawY < layout.lowerY {
			left, right = layout.innerLeft, layout.innerRight
		}
		if start > 0 {
			drawX = left
		}
		line := introNarrativeLine(text, start, drawX-left, right-left, assets.GlyphWidths, layout.spaceWidth)
		drawIntroNarrativeLine(canvas, assets, text[start:line.end], drawX, drawY, line, layout.spaceWidth)
		start = line.next
		drawY += lineHeight
	}
}

type introNarrativeLineLayout struct {
	end        int
	next       int
	width      int
	maxWidth   int
	spaces     int
	justify    bool
	drawHyphen bool
}

func introNarrativeLine(
	text string,
	start, initialWidth, maxWidth int,
	widths []int,
	spaceWidth int,
) introNarrativeLineLayout {
	// Like the original wrapper, measure forward until the line is full, then back
	// up to the nearest space or '_' soft hyphen whose '-' still fits. A '_' inside
	// the last word therefore wins over the space before that word.
	width, spaces, index := initialWidth, 0, start
	for ; index < len(text) && width < maxWidth && text[index] != '\n'; index++ {
		if text[index] <= ' ' {
			spaces++
		}
		width += introNarrativeCharacterWidth(text[index], widths, spaceWidth)
	}
	line := introNarrativeLineLayout{
		end: index, next: index, width: width, maxWidth: maxWidth, spaces: spaces,
		justify: false, drawHyphen: false,
	}
	if index < len(text) && text[index] == '\n' {
		line.next++

		return line
	}
	if width < maxWidth {
		return line
	}
	overflow := index - 1
	hyphenWidth := introNarrativeCharacterWidth('-', widths, spaceWidth)
	line.justify = true
	for index = overflow; index > start; index-- {
		char := text[index]
		switch {
		case char == ' ':
			line.end, line.next, line.width, line.spaces = index, index+1, width-spaceWidth, spaces-1

			return line
		case char == '_' && width+hyphenWidth < maxWidth:
			line.end, line.next, line.width, line.drawHyphen = index, index+1, width+hyphenWidth, true

			return line
		case char != '_' && char != '{':
			width -= introNarrativeCharacterWidth(char, widths, spaceWidth)
		}
	}
	// No break point fits. Shipped text never gets here, and the original would
	// drop a character, so split the word before the overflowing glyph instead.
	line.end = max(overflow, start+1)
	line.next = line.end
	if line.end == overflow {
		line.width -= introNarrativeCharacterWidth(text[overflow], widths, spaceWidth)
	}

	return line
}

func drawIntroNarrativeLine(
	canvas *ebiten.Image,
	assets *sprites.NarrativeIntroAssets,
	text string,
	drawX, drawY int,
	layout introNarrativeLineLayout,
	spaceWidth int,
) {
	extra, spaces := 0, layout.spaces
	if layout.justify {
		extra = max(0, layout.maxWidth-layout.width)
	}
	for index := range len(text) {
		char := text[index]
		switch {
		case char <= ' ':
			drawX += spaceWidth
			if spaces > 0 && layout.justify {
				addition := extra / spaces
				drawX += addition
				extra -= addition
				spaces--
			}
		case char == '{':
			drawX += 15
		case char == '_':
		default:
			drawX += drawIntroNarrativeGlyph(canvas, assets, char, drawX, drawY)
		}
	}
	if layout.drawHyphen {
		drawIntroNarrativeGlyph(canvas, assets, '-', drawX, drawY)
	}
}

func drawIntroNarrativeGlyph(
	canvas *ebiten.Image,
	assets *sprites.NarrativeIntroAssets,
	char byte,
	drawX, drawY int,
) int {
	index := int(char - ' ')
	if index < 0 || index >= len(assets.Glyphs) {
		return 0
	}
	if drawY < 192 && assets.Glyphs[index] != nil {
		drawIntroImage(canvas, assets.Glyphs[index], drawX, drawY)
	}

	return assets.GlyphWidths[index] + 1
}

func introNarrativeCharacterWidth(char byte, widths []int, spaceWidth int) int {
	switch {
	case char <= ' ':
		return spaceWidth
	case char == '{':
		return 15
	case char == '_':
		return 0
	}
	index := int(char - ' ')
	if index < 0 || index >= len(widths) {
		return 0
	}

	return widths[index] + 1
}
