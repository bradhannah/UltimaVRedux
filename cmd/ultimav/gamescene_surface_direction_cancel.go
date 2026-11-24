package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/hajimehoshi/ebiten/v2"
)

func (g *GameScene) hasUncommittedSurfaceDirection(mapType references.GeneralMapType) bool {
	if mapType != references.SmallMapType && mapType != references.LargeMapType {
		return false
	}
	// Keep item and spell direction readers on their own commitment rules.
	//nolint:exhaustive // Only reachable ordinary, precommit surface prompts belong here.
	switch g.secondaryKeyState {
	case GetDirectionInput, PushDirectionInput, OpenDirectionInput, JimmyDoorDirectionInput,
		KlimbDirectionInput, LookDirectionInput, TalkDirectionInput, SearchDirectionInput, AttackDirectionInput:
		return true
	case FireDirectionInput:
		return mapType == references.LargeMapType
	default:
		return false
	}
}

func (g *GameScene) cancelSurfaceDirectionInput() bool {
	if !g.hasUncommittedSurfaceDirection(g.gameState.MapState.PlayerLocation.Location.GetMapType()) {
		return false
	}
	if !g.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		return false
	}
	// REDUX DEVIATION: CMD-0003: original direction input excludes Escape.
	// Redux lets an accidental, uncommitted surface command be abandoned for free.
	// Talk's direction prompt uses this too; see ref:TALK-0005.
	g.appendToCurrentRowStr("Cancelled")
	g.secondaryKeyState = PrimaryInput
	// Registration consumes Escape through release without delaying the next key.
	g.keyboard.SetAllowKeyPressImmediately()

	return true
}
