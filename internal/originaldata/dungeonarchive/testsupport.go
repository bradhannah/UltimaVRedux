package dungeonarchive

import "image"

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support.

// ItemSizes returns the verified dimensions used by dungeon rendering.
func ItemSizes() [ItemCount]image.Point { return itemSizes }

// MonsterHalfSizes returns the verified dimensions used by dungeon rendering.
func MonsterHalfSizes() [MonsterHalfCount]image.Point { return monsterHalfSizes }

// PanelSizes returns the verified dimensions used by dungeon rendering.
func PanelSizes() [PanelCount]image.Point { return panelSizes }
