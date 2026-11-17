package main

import "image"

func (g *GameScene) mapPopupViewport(screenBounds image.Rectangle) image.Rectangle {
	if g.gameState != nil && g.gameState.IsInCombat() {
		return g.combatLayout().battlefieldRectangle(screenBounds)
	}

	return image.Rectangle{}
}
