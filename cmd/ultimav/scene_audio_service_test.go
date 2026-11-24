package main

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	eaudio "github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestReadersServiceAudioThroughOpeningReadingAndDismissal(t *testing.T) {
	for _, reader := range []string{"atlas", "journal"} {
		t.Run(reader, func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
				WithSystemCallbacks().WithRandomSeed(42).Build()
			now := time.Unix(100, 0)
			config, err := audio.NewAudioConfiguration()
			require.NoError(t, err)
			context := eaudio.CurrentContext()
			if context == nil {
				context = eaudio.NewContext(44100)
			}
			manager, err := audio.NewGameAudioManagerWithRuntime(config, context, func() time.Time { return now })
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, manager.Close()); state.EventQueueManager.Shutdown() })
			keys := map[ebiten.Key]bool{ebiten.KeyShiftLeft: true}
			if reader == "atlas" {
				keys[ebiten.KeyM] = true
			} else {
				keys[ebiten.KeyJ] = true
			}
			scene := &GameScene{
				gameState: state, gameConfig: state.GameConfig,
				keyboard: input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return keys[key] }),
				output:   text.NewOutput(nil, 1, 64, 80), spriteSheet: sharedPresentationTestSpriteSheet(),
				now: func() time.Time { return now }, startTime: now, audioManager: manager,
			}
			state.SystemCallbacks.UI.ShowQuestJournalDossier = scene.ShowQuestJournalDossier
			state.ElapsedMs = 321
			beforeDate, beforeRNG := state.DateTime, state.GetRNG().Calls()
			request := game_state.SoundPlaybackRequest{Effect: game_state.SoundEarthquake, Mode: game_state.SoundPlaybackLoop, Volume: 1}
			assertService := func() {
				t.Helper()
				for range 8 {
					require.NoError(t, manager.PlaySound(request))
				}
				require.Error(t, manager.PlaySound(request), "all eight owned loops occupy the budget")
				manager.StopSound(game_state.SoundStopRequest{Effect: request.Effect, FadeOutMs: 500})
				now = now.Add(time.Second)
				require.NoError(t, scene.Update(nil))
				require.NoError(t, manager.PlaySound(request), "the reader frame must retire released voices")
				manager.StopSound(game_state.SoundStopRequest{Effect: request.Effect})
			}
			assertService() // Opening frame, including the atlas early return.
			if reader == "atlas" {
				require.NotNil(t, scene.atlasSession)
			} else {
				require.NotNil(t, scene.journalSession)
			}
			fired := 0
			state.EventQueueManager.Workflow("reader-audio-service").Priority(event_queue.PriorityNormal).Sequential().
				First(func() { fired++ }).Start()
			clear(keys)
			assertService() // Reading frame.
			keys[ebiten.KeyEscape] = true
			assertService() // Dismissal frame remains no-time.
			require.Zero(t, fired)
			require.EqualValues(t, 321, state.ElapsedMs)
			require.Equal(t, beforeDate, state.DateTime)
			require.Equal(t, beforeRNG, state.GetRNG().Calls())
		})
	}
}
