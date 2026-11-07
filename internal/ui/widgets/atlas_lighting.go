package widgets

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// AtlasLighting is the presentation-only light the map terrain is drawn under.
// Markers, labels, grid and panels stay at full brightness, so the map remains
// usable. It never reads or changes gameplay time.
type AtlasLighting struct {
	// Terrain multiplies the terrain colour; {1, 1, 1} draws the map as charted.
	Terrain [3]float32
	// EdgeShade (0-1) darkens the border of a centred pool of carried light
	// toward EdgeColour. Zero spreads the light evenly across the whole map.
	EdgeShade  float32
	EdgeColour [3]float32
	// PoolCore (0-1) is the fraction of the pool's radius that stays fully lit.
	PoolCore float32
	// Flicker is the fractional brightness wobble of fire light; zero is steady.
	Flicker float64
}

const (
	// Two unrelated periods read as firelight rather than a steady pulse.
	atlasFlickerSlowSeconds = 1.9
	atlasFlickerFastSeconds = 0.53
	atlasFlickerSlowWeight  = 0.6
	atlasFlickerFastWeight  = 0.4
	// A small mask is enough: linear filtering smooths it across the map pane.
	atlasLightPoolSize   = 64
	atlasLightPoolRadius = atlasLightPoolSize / 2
	// Smoothstep is 3t² - 2t³: a soft start and end to the pool's falloff.
	atlasSmoothstepSquare = 3
	atlasSmoothstepCube   = 2
)

// AtlasDaylight draws the map exactly as charted.
func AtlasDaylight() AtlasLighting {
	return AtlasLighting{Terrain: [3]float32{1, 1, 1}, EdgeShade: 0, EdgeColour: [3]float32{}, PoolCore: 0, Flicker: 0}
}

// SetLighting chooses the light the terrain is drawn under for this opening.
func (d *AtlasDialog) SetLighting(lighting AtlasLighting) { d.lighting = lighting }

// lightFlicker is the current firelight multiplier, driven by the dialog's own
// frame count so the frozen game clock and RNG are never consulted.
func (d *AtlasDialog) lightFlicker() float32 {
	if d.lighting.Flicker == 0 {
		return 1
	}
	seconds := float64(d.lightFrames) / float64(ebiten.TPS())
	wave := atlasFlickerSlowWeight*math.Sin(2*math.Pi*seconds/atlasFlickerSlowSeconds) +
		atlasFlickerFastWeight*math.Sin(2*math.Pi*seconds/atlasFlickerFastSeconds)

	return float32(1 + d.lighting.Flicker*wave)
}

func (d *AtlasDialog) applyTerrainLight(op *ebiten.DrawImageOptions, flicker float32) {
	terrain := d.lighting.Terrain
	op.ColorScale.Scale(terrain[0]*flicker, terrain[1]*flicker, terrain[2]*flicker, 1)
}

// drawLightPool darkens the terrain toward the map's border, leaving the centre
// lit. The shade eases slightly against the flicker so the pool's reach breathes.
func (d *AtlasDialog) drawLightPool(screen *ebiten.Image, flicker float32) {
	if d.lighting.EdgeShade <= 0 {
		return
	}
	if d.lightPool == nil || d.lightPoolCore != d.lighting.PoolCore {
		d.lightPool = newAtlasLightPool(d.lighting.PoolCore)
		d.lightPoolCore = d.lighting.PoolCore
	}
	shade := min(1, max(0, d.lighting.EdgeShade*(1+(1-flicker))))
	edge := d.lighting.EdgeColour
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(d.mapRect.Dx())/atlasLightPoolSize, float64(d.mapRect.Dy())/atlasLightPoolSize)
	op.GeoM.Translate(float64(d.mapRect.Min.X), float64(d.mapRect.Min.Y))
	op.Filter = ebiten.FilterLinear
	op.ColorScale.Scale(edge[0]*shade, edge[1]*shade, edge[2]*shade, shade)
	screen.DrawImage(d.lightPool, op)
}

// newAtlasLightPool is a white mask whose opacity rises from the lit core to
// the pane's border (and corners) along a smoothstep curve.
func newAtlasLightPool(core float32) *ebiten.Image {
	pixels := image.NewRGBA(image.Rect(0, 0, atlasLightPoolSize, atlasLightPoolSize))
	half := float64(atlasLightPoolRadius)
	inner := float64(core)
	for y := range atlasLightPoolSize {
		for x := range atlasLightPoolSize {
			dx, dy := (float64(x)+0.5-half)/half, (float64(y)+0.5-half)/half
			t := min(1, max(0, (math.Hypot(dx, dy)-inner)/(1-inner)))
			alpha := uint8(math.Round(t * t * (atlasSmoothstepSquare - atlasSmoothstepCube*t) * math.MaxUint8))
			pixels.SetRGBA(x, y, color.RGBA{R: alpha, G: alpha, B: alpha, A: alpha})
		}
	}

	return ebiten.NewImageFromImage(pixels)
}
