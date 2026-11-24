package main

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestCapsLockReaderPhysicalShiftMatrix(t *testing.T) {
	for _, caps := range []bool{false, true} {
		for _, shift := range []ebiten.Key{ebiten.KeyShiftLeft, ebiten.KeyShiftRight} {
			for _, key := range []ebiten.Key{ebiten.KeyM, ebiten.KeyJ} {
				t.Run(fmt.Sprintf("caps=%t/%s/%s", caps, shift, key), func(t *testing.T) {
					scene, held, clock, update := newReaderInputClockScene(t)
					held[ebiten.KeyCapsLock] = caps
					for range 2 {
						held[shift], held[key] = true, true
						*clock += keyPressDelay + 1
						update()
						if key == ebiten.KeyM {
							require.NotNil(t, scene.atlasSession)
						} else {
							require.NotNil(t, scene.journalSession)
						}
						delete(held, key)
						update()
						require.True(t, scene.keyboard.IsWaitingForRelease(), "Shift is a real held control")
						delete(held, shift)
						update()
						require.False(t, scene.keyboard.IsWaitingForRelease())
						held[shift], held[key] = true, true
						*clock += keyPressDelay + 1
						update()
						require.Zero(t, scene.dialogStack.Count())
						delete(held, shift)
						delete(held, key)
						update()
						require.Nil(t, scene.atlasSession)
						require.Nil(t, scene.journalSession)
					}
				})
			}
		}
	}
}

func TestCapsLockPlainLettersRetainCommandRouting(t *testing.T) {
	for _, caps := range []bool{false, true} {
		for _, key := range []ebiten.Key{ebiten.KeyM, ebiten.KeyJ} {
			t.Run(fmt.Sprintf("caps=%t/%s", caps, key), func(t *testing.T) {
				scene, held, _, update := newReaderInputClockScene(t)
				var prompts []string
				show := scene.gameState.SystemCallbacks.Message.ShowCommandPrompt
				scene.gameState.SystemCallbacks.Message.ShowCommandPrompt = func(prompt string) {
					prompts = append(prompts, prompt)
					show(prompt)
				}
				held[ebiten.KeyCapsLock], held[key] = caps, true
				update()
				require.Nil(t, scene.atlasSession)
				require.Nil(t, scene.journalSession)
				command := "Jimmy-"
				if key == ebiten.KeyM {
					command = "Mix Reagents"
				}
				require.Contains(t, prompts, command, "the plain letter still runs its command")
			})
		}
	}
}
