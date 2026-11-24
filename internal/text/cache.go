package text

import (
	"fmt"
	"image"
	"image/color"

	beradtext "github.com/bradhannah/berad-sdk/modules/berad-text"
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/profiling"
)

//nolint:gochecknoglobals // Application-owned renderer, serialized on the presentation lane.
var textRenderer = newTextRenderer()

const legacyRasterPadding = 2

func newTextRenderer() *beradtext.Renderer {
	renderer, err := beradtext.NewRenderer(beradtext.DefaultRendererOptions())
	if err != nil {
		panic(fmt.Sprintf("initializing mandatory text renderer: %v", err))
	}

	return renderer
}

// CloseRenderer retires the application-owned SDK raster cache on final shutdown.
// The application must not draw afterward.
func CloseRenderer() {
	textRenderer.Close()
	publishTextAccounting()
}

func newTextLayout(value string, font *beradtext.Font) *beradtext.Layout {
	content, err := beradtext.NewContent([]beradtext.Span{{Text: value, Font: font, Paint: ""}})
	if err != nil {
		panic(fmt.Sprintf("composing game text: %v", err))
	}
	var options beradtext.LayoutOptions // SDK defaults: natural metrics, no wrapping.
	layout, err := beradtext.NewLayout(content, options)
	if err != nil {
		panic(fmt.Sprintf("laying out game text: %v", err))
	}

	return layout
}

// DrawCachedText adapts prompt/placeholder rendering without exposing cached images.
func DrawCachedText(
	target *ebiten.Image, value string, font *UltimaFont, options *ebiten.DrawImageOptions, ink color.Color,
) {
	drawTextLayout(target, newTextLayout(value, font.sdkForScale(1)), options, ink)
}

func drawTextLayout(target *ebiten.Image, layout *beradtext.Layout, options *ebiten.DrawImageOptions, ink color.Color) {
	var palette beradtext.Palette
	if ink != nil {
		palette = beradtext.Palette{"": ink}
	}
	drawPaintedTextLayout(target, layout, options, palette)
}

func drawPaintedTextLayout(
	target *ebiten.Image, layout *beradtext.Layout, options *ebiten.DrawImageOptions, palette beradtext.Palette,
) {
	// Game text images clip overhanging ink at the local origin; keep that
	// presentation policy without narrowing SDK ink geometry.
	bounds := layout.Bounds()
	localClip := image.Rect(0, 0, int(bounds.Width())+legacyRasterPadding, int(bounds.Height())+legacyRasterPadding)
	drawLayoutWithClip(target, layout, options, palette, &localClip)
}

func drawLayoutWithClip(
	target *ebiten.Image, layout *beradtext.Layout, options *ebiten.DrawImageOptions,
	palette beradtext.Palette, localClip *image.Rectangle,
) {
	before := textRenderer.Stats()
	if err := textRenderer.Draw(target, layout, palette, beradtext.DrawOptions{
		GeoM: options.GeoM, Filter: options.Filter, LocalClip: localClip,
	}); err != nil {
		panic(fmt.Sprintf("drawing game text: %v", err))
	}
	after := textRenderer.Stats()
	if after.Hits > before.Hits {
		profiling.Count(profiling.TextHits)
	} else {
		profiling.Count(profiling.TextMisses)
	}
	for count := before.Evictions; count < after.Evictions; count++ {
		profiling.Count(profiling.TextEvictions)
	}
	if after.Bypasses > before.Bypasses {
		profiling.Count(profiling.TextCacheBypasses)
	}
	publishTextAccounting()
}

func publishTextAccounting() {
	stats := textRenderer.Stats()
	if stats.Entries < 0 || stats.Bytes < 0 || stats.PeakBytes < 0 {
		panic("SDK text accounting must be nonnegative")
	}
	profiling.SetGauge(profiling.TextCacheEntries, uint64(stats.Entries))
	profiling.SetGauge(profiling.TextCacheBytes, uint64(stats.Bytes))
	profiling.SetGauge(profiling.TextCachePeakBytes, uint64(stats.PeakBytes))
}
