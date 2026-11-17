package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const (
	combatHealthBarLeftSourcePixel    = 2
	combatHealthBarTopSourcePixel     = 13
	combatHealthBarWidthSourcePixels  = 13
	combatHealthBarHeightSourcePixels = 4
	combatHealthBarInsetSourcePixel   = 1
	combatHealthBarInnerSourcePixels  = combatHealthBarWidthSourcePixels - 2*combatHealthBarInsetSourcePixel
	combatHealthPercentScale          = 100
	combatHealthBarOpacityPercent     = 40
	combatHealthBarAlpha              = ucolor.AlphaOpaque * combatHealthBarOpacityPercent / combatHealthPercentScale
	combatHealthBarBackgroundAlpha    = ucolor.AlphaOpaque
	combatHealthHealthyPercent        = 50
	combatHealthCriticalPercent       = 25
)

func drawCombatHealthBar(
	screen *ebiten.Image,
	drawOptions *ebiten.DrawImageOptions,
	currentHP, maxHP int,
) {
	fillPixels := combatHealthBarFillPixels(currentHP, maxHP)
	left, top := drawOptions.GeoM.Apply(
		combatHealthBarLeftSourcePixel,
		combatHealthBarTopSourcePixel,
	)
	right, bottom := drawOptions.GeoM.Apply(
		combatHealthBarLeftSourcePixel+combatHealthBarWidthSourcePixels,
		combatHealthBarTopSourcePixel+combatHealthBarHeightSourcePixels,
	)
	vector.FillRect(
		screen,
		float32(left),
		float32(top),
		float32(right-left),
		float32(bottom-top),
		ucolor.From(ucolor.EGABlack).Alpha(combatHealthBarBackgroundAlpha).RGBA(),
		false,
	)
	if fillPixels == 0 {
		return
	}

	fillLeft, fillTop := drawOptions.GeoM.Apply(
		combatHealthBarLeftSourcePixel+combatHealthBarInsetSourcePixel,
		combatHealthBarTopSourcePixel+combatHealthBarInsetSourcePixel,
	)
	fillRight, fillBottom := drawOptions.GeoM.Apply(
		float64(combatHealthBarLeftSourcePixel+combatHealthBarInsetSourcePixel+fillPixels),
		combatHealthBarTopSourcePixel+combatHealthBarHeightSourcePixels-combatHealthBarInsetSourcePixel,
	)
	vector.FillRect(
		screen,
		float32(fillLeft),
		float32(fillTop),
		float32(fillRight-fillLeft),
		float32(fillBottom-fillTop),
		combatHealthBarColor(currentHP, maxHP),
		false,
	)
}

func combatHealthBarFillPixels(currentHP, maxHP int) int {
	return combatHealthFill(currentHP, maxHP, combatHealthBarInnerSourcePixels)
}

// combatHealthFill maps health onto width cells; any health left shows one.
func combatHealthFill(currentHP, maxHP, width int) int {
	if currentHP <= 0 || maxHP <= 0 {
		return 0
	}

	currentHP = min(currentHP, maxHP)

	return max(1, (currentHP*width+maxHP-1)/maxHP)
}

func combatHealthBarColor(currentHP, maxHP int) color.Color {
	return ucolor.From(combatHealthTierColor(currentHP, maxHP)).Alpha(combatHealthBarAlpha).RGBA()
}

// combatHealthTierColor is green above half health, yellow down to a quarter
// and red below.
func combatHealthTierColor(currentHP, maxHP int) ucolor.EGAColor {
	if maxHP <= 0 {
		return ucolor.EGALightRed
	}

	scaledHealth := min(max(currentHP, 0), maxHP) * combatHealthPercentScale
	switch {
	case scaledHealth > maxHP*combatHealthHealthyPercent:
		return ucolor.EGALightGreen
	case scaledHealth >= maxHP*combatHealthCriticalPercent:
		return ucolor.EGAYellow
	default:
		return ucolor.EGALightRed
	}
}
