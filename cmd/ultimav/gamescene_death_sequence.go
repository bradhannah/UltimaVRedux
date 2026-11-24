package main

import (
	"fmt"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/internal/visual"
)

// Death sequence presentation timing; recovery authority is ref:RECOVERY-0002.
// Original game used ticks (~55ms each), converted to milliseconds
const (
	deathResurrectionWorkflowName = "death_resurrection_workflow"
	deathFadeToBlackMs            = 2000 // Fade to black duration
	deathFadeFromBlackMs          = 2000 // Fade from black duration
	deathMessageDurationMs        = 2500 // Duration for each death message
	deathMessageDelayMs           = 770  // Delay between messages (14 ticks)
	deathLongDelayMs              = 1540 // Long delay between messages (28 ticks)

	// Pulse sounds
	deathPulseCount        = 6   // Number of initial pulse sounds
	deathPulseSoundDelayMs = 400 // Delay between pulse sounds

	// Guardian appearances
	guardianAppearDelayMs = 220  // Delay between guardian appearances (4 ticks)
	guardianFizzInMs      = 1500 // Duration of fizz-in effect

	// Earthquake
	deathEarthquakeDurationMs = 1000 // Duration of each earthquake
	deathEarthquakeGapMs      = 200  // Gap between double earthquakes
	deathEarthquakeCycles     = 15.0

	// Resurrection pulses
	resurrectionPulseDelayMs   = 1000 // Delay between individual resurrection pulses
	resurrectionPulseFlashMs   = 200  // Duration of each resurrection pulse flash
	resurrectionPulseIntensity = 0.4  // Peak intensity of resurrection pulse

)

// Death sequence messages accompanying full-party recovery (ref:RECOVERY-0002).
const (
	deathDarknessMsg  = "An unending darkness\nengulfs thee..."
	deathRefugeMsg    = "Thou hast found refuge."
	deathPeaceMsg     = "No evil lives here,\nonly peace and darkness."
	deathDisturbedMsg = "But thy slumber\nis disturbed!"
)

// Final messages
const (
	fortisFortunaMsg      = "Someone shouts\n\n\"FORTIS FORTUNA\nAVENTARI\""
	thunderMsg            = "There is a peal\nof thunder!"
	strangeWordsMsg       = "Strange words\nare intoned."
	vertigoMsg            = "Vertigo..."
	guardiansResurrectMsg = "The Guardians have\nresurrected thee!"
)

// Death scene apparition positions on the 11x11 grid (from death_scene.go)
// These are relative to the centered 11x11 grid, not world coordinates
const (
	guardianLeftX  = 2
	guardianLeftY  = 6
	guardianRightX = 8
	guardianRightY = 6
	wandererX      = 5
	wandererY      = 2
	avatarX        = 5 // Center of 11x11 grid (resurrection position)
	avatarY        = 5
)

// Death scene dissolve options helpers - reduces verbosity and ensures consistency
func guardianLeftDissolveOpts(mode visual.DissolveMode) TileDissolveOptions {
	return TileDissolveOptions{
		Position:     references.Position{X: guardianLeftX, Y: guardianLeftY},
		SpriteIndex:  indexes.CodexAngelLeft,
		Mode:         mode,
		OutlineColor: sprites.BlackOutline,
		PositionMode: PositionModeFixedGrid,
		RenderLayer:  RenderLayerOverFade,
	}
}

func guardianRightDissolveOpts(mode visual.DissolveMode) TileDissolveOptions {
	return TileDissolveOptions{
		Position:     references.Position{X: guardianRightX, Y: guardianRightY},
		SpriteIndex:  indexes.CodexAngelRight,
		Mode:         mode,
		OutlineColor: sprites.BlackOutline,
		PositionMode: PositionModeFixedGrid,
		RenderLayer:  RenderLayerOverFade,
	}
}

func wandererDissolveOpts(mode visual.DissolveMode) TileDissolveOptions {
	return TileDissolveOptions{
		Position:     references.Position{X: wandererX, Y: wandererY},
		SpriteIndex:  indexes.Apparation1,
		Mode:         mode,
		OutlineColor: sprites.BlackOutline,
		PositionMode: PositionModeFixedGrid,
		RenderLayer:  RenderLayerOverFade,
	}
}

func avatarDissolveOpts(mode visual.DissolveMode) TileDissolveOptions {
	return TileDissolveOptions{
		Position:     references.Position{X: avatarX, Y: avatarY},
		SpriteIndex:  indexes.AvatarKeyIndex,
		Mode:         mode,
		OutlineColor: sprites.BlackOutline,
		PositionMode: PositionModeFixedGrid,
		RenderLayer:  RenderLayerOverFade,
	}
}

// showDeathDialog displays a timed overlay message for death sequence using the OverlayDialog builder.
// Uses AsOverlayFeedback() template for consistent map-center-lower positioning with auto-sizing.
// Messages are mirrored to the output pane for authentic Ultima V record.
func (g *GameScene) showDeathDialog(message string) {
	// Create dialog using OverlayDialog builder - instant show matches original behavior
	overlay := widgets.NewOverlayDialog(message).
		AsOverlayFeedback(). // Map center lower, auto-size, centered text, no effects
		WithExternallyManagedDuration(deathMessageDurationMs).
		MirrorToOutput(g.output). // Mirror to output pane for authentic Ultima V record
		Build()

	g.installTimedOverlay(overlay)
}

// StartDeathResurrectionWorkflow initiates the complete death and resurrection sequence.
// This is triggered when all party members die (IsAllPartyDead() returns true).
// Full-party recovery presentation follows ref:RECOVERY-0002.
//
// Uses the generic tile dissolve system for sprite rendering:
// - Avatar: AvatarKeyIndex (appears first at center, resurrection position)
// - Guardians: CodexAngelLeft (94), CodexAngelRight (95)
// - Wanderer: Apparation1 (372)
func (g *GameScene) StartDeathResurrectionWorkflow() {
	// Prevent double-triggering
	if g.gameState.EventQueueManager.HasWorkflow(deathResurrectionWorkflowName) ||
		g.gameState.EventQueueManager.HasWorkflow("stonegate_trap_workflow") {
		return
	}
	g.startDeathResurrectionWorkflow()
}

func (g *GameScene) startDeathResurrectionWorkflow() {
	q := g.gameState.EventQueueManager.Workflow(deathResurrectionWorkflowName).
		Priority(event_queue.PriorityCritical).
		Sequential()
	g.buildDeathResurrectionWorkflow(q)
	q.Start()
}

// buildDeathResurrectionWorkflow configures the death resurrection event chain.
// Used by HandoffTo when chaining from other workflows (e.g., Stonegate trap).
// The builder receives a queue with Priority and Sequential already configured.
func (g *GameScene) buildDeathResurrectionWorkflow(q *event_queue.SequentialEventBuilder) {
	review, reviewErr := g.gameState.ReviewDeathResurrection()
	if reviewErr != nil {
		q.BlockInput().First(func() {
			g.gameState.SystemCallbacks.Message.AddRowStr("The Guardians cannot reach thee.")
		}).Finally(func() {
			g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(0)
			g.ShowNonAvatarEntities()
		})

		return
	}
	karmaReport, err := g.gameState.GameReferences.KarmaReports.Select(
		review.KarmaAtDeath,
		references.KarmaReportDeath,
	)
	if err != nil {
		panic(fmt.Sprintf("selecting death karma report: %v", err))
	}

	var outcome game_state.DeathResurrectionOutcome
	var committed bool
	var ambientWasHidden bool

	// Dissolve IDs for generic tile dissolve system
	var avatarID, guardianLeftID, guardianRightID, wandererID int

	q.BlockInput().
		First(func() {
			ambientWasHidden = g.hideAmbientSounds
			g.HideAmbientSounds()
			g.gameState.SystemCallbacks.Audio.PauseBackgroundMusic()
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundDeath)
		}).
		Animate(deathFadeToBlackMs, func(progress float32) {
			g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(float64(progress))
		}).
		Wait(500).
		Then(func() {
			avatarID = g.StartTileDissolveWithOptions(avatarDissolveOpts(visual.DissolveModeReveal))
		}).
		Animate(uint64(guardianFizzInMs), func(progress float32) {
			g.UpdateTileDissolveProgress(avatarID, float64(progress))
		}).
		Wait(300).
		Then(func() {
			g.showDeathDialog(deathDarknessMsg)
		}).
		Wait(deathMessageDurationMs+deathMessageDelayMs).
		Then(func() {
			g.showDeathDialog(deathRefugeMsg)
		}).
		Wait(deathMessageDurationMs+deathLongDelayMs).
		Then(func() {
			g.showDeathDialog(deathPeaceMsg)
		}).
		Wait(deathMessageDurationMs+deathLongDelayMs).
		Then(func() {
			g.showDeathDialog(deathDisturbedMsg)
		}).
		Wait(deathMessageDurationMs).
		RepeatWithDelay(deathPulseCount, deathPulseSoundDelayMs, func(_ int) bool {
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)

			return true
		}).
		Wait(500).
		Then(func() {
			g.showDeathDialog(fortisFortunaMsg)
		}).
		Wait(deathMessageDurationMs).
		Then(func() {
			guardianLeftID = g.StartTileDissolveWithOptions(guardianLeftDissolveOpts(visual.DissolveModeReveal))
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundApparitionAppears)
		}).
		Animate(uint64(guardianFizzInMs), func(progress float32) {
			g.UpdateTileDissolveProgress(guardianLeftID, float64(progress))
		}).
		Wait(guardianAppearDelayMs).
		Then(func() {
			guardianRightID = g.StartTileDissolveWithOptions(guardianRightDissolveOpts(visual.DissolveModeReveal))
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundApparitionAppears)
		}).
		Animate(uint64(guardianFizzInMs), func(progress float32) {
			g.UpdateTileDissolveProgress(guardianRightID, float64(progress))
		}).
		Wait(guardianAppearDelayMs).
		Then(func() {
			g.showDeathDialog(thunderMsg)
		}).
		Wait(deathMessageDurationMs).ThenSequence(

		g.thenThunderLightningEffect()).
		Wait(300).ThenSequence(

		g.thenDeathEarthquake()).
		Wait(deathEarthquakeGapMs).ThenSequence(

		g.thenDeathEarthquake()).
		Wait(500).
		Then(func() {
			wandererID = g.StartTileDissolveWithOptions(wandererDissolveOpts(visual.DissolveModeReveal))
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundApparitionAppears)
		}).
		Animate(uint64(guardianFizzInMs), func(progress float32) {
			g.UpdateTileDissolveProgress(wandererID, float64(progress))
		}).
		Then(func() {
		}).
		Wait(500).
		Then(func() {
			var err error
			outcome, err = g.gameState.CommitDeathResurrection(review.Fingerprint)
			committed = err == nil
			if !committed {
				g.gameState.SystemCallbacks.Message.AddRowStr("The Guardians cannot reach thee.")

				return
			}
		}).If(func() bool { return committed }, func(resurrectionQ *event_queue.SequentialEventBuilder) {
		g.buildCommittedDeathResurrectionPresentation(
			resurrectionQ,
			outcome,
			karmaReport,
			&avatarID,
			&guardianLeftID,
			&guardianRightID,
			&wandererID,
		)
	}).Finally(

		func() {
			g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(0)
			g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(0)
			g.gameState.SystemCallbacks.Visual.SetScreenShakeOffset(0, 0)
			g.ShowNonAvatarEntities()

			g.StopAllTileDissolves()

			g.gameState.SystemCallbacks.Audio.ResumeBackgroundMusic()
			if !ambientWasHidden {
				g.ShowAmbientSounds()
			}
		})

	// Note: Caller is responsible for calling .Start() - either StartDeathResurrectionWorkflow()
	// or HandoffTo() which handles it automatically
}

func (g *GameScene) buildCommittedDeathResurrectionPresentation(
	q *event_queue.SequentialEventBuilder,
	outcome game_state.DeathResurrectionOutcome,
	karmaReport string,
	avatarID, guardianLeftID, guardianRightID, wandererID *int,
) {
	q.ThenSequence(g.thenShowDeathKarmaReport(karmaReport)).
		Then(func() {
			g.showDeathDialog(strangeWordsMsg)
		}).
		Wait(deathMessageDurationMs).ThenSequence(
		func(pulseQ *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
			for i := range outcome.Members {
				if i > 0 {
					pulseQ.Wait(resurrectionPulseDelayMs - resurrectionPulseFlashMs)
				}
				pulseQ.ThenSequence(g.thenResurrectionPulseFlash())
			}

			return pulseQ
		}).
		Wait(500).
		Then(func() {
			g.showDeathDialog(vertigoMsg)
		}).
		Then(func() {
			g.StopTileDissolve(*avatarID)
			g.StopTileDissolve(*guardianLeftID)
			g.StopTileDissolve(*guardianRightID)
			g.StopTileDissolve(*wandererID)

			*avatarID = g.StartTileDissolveWithOptions(avatarDissolveOpts(visual.DissolveModeHide))
			*guardianLeftID = g.StartTileDissolveWithOptions(guardianLeftDissolveOpts(visual.DissolveModeHide))
			*guardianRightID = g.StartTileDissolveWithOptions(guardianRightDissolveOpts(visual.DissolveModeHide))
			*wandererID = g.StartTileDissolveWithOptions(wandererDissolveOpts(visual.DissolveModeHide))
		}).
		Animate(uint64(guardianFizzInMs), func(progress float32) {
			g.UpdateTileDissolveProgress(*avatarID, float64(progress))
			g.UpdateTileDissolveProgress(*guardianLeftID, float64(progress))
			g.UpdateTileDissolveProgress(*guardianRightID, float64(progress))
			g.UpdateTileDissolveProgress(*wandererID, float64(progress))
		}).
		Then(func() {
			g.StopTileDissolve(*avatarID)
			g.StopTileDissolve(*guardianLeftID)
			g.StopTileDissolve(*guardianRightID)
			g.StopTileDissolve(*wandererID)
		}).
		Wait(deathMessageDurationMs).
		Then(func() {
			g.showDeathDialog(guardiansResurrectMsg)
		}).
		Animate(deathFadeFromBlackMs, func(progress float32) {
			g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(1.0 - float64(progress))
		}).
		Wait(deathMessageDurationMs)
}

func (g *GameScene) thenShowDeathKarmaReport(
	report string,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	overlay := widgets.NewOverlayDialog(fmt.Sprintf("\"%s\"", report)).
		AsOverlayFeedback().
		WithUserDismissed(g.keyboard).
		AutoPopFromStack(&g.dialogStack).
		MirrorToOutput(g.output).
		Build()

	return g.showOverlayAndWait(overlay)
}

// thenDeathEarthquake returns a modifier function for death sequence earthquake effect
func (g *GameScene) thenDeathEarthquake() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.
			Then(func() {
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundEarthquake)
			}).
			Animate(deathEarthquakeDurationMs, func(progress float32) {
				// Sinusoidal intensity for natural shake feel
				intensity := math.Sin(float64(progress)*math.Pi) * 0.8
				phase := float64(progress) * deathEarthquakeCycles * 2 * math.Pi
				offsetX := math.Sin(phase) * 6 * intensity
				offsetY := math.Cos(phase) * 6 * intensity
				g.gameState.SystemCallbacks.Visual.SetScreenShakeOffset(offsetX, offsetY)
			}).
			Then(func() {
				g.gameState.SystemCallbacks.Visual.SetScreenShakeOffset(0, 0)
			})
	}
}

// thenSmoothApparitionFlash creates a smooth triangle wave flash for guardian/wanderer appearances
// The flash smoothly rises to peak intensity and then fades back to 0
func (g *GameScene) thenSmoothApparitionFlash(r, gr, b, maxIntensity float64, durationMs uint64) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.
			Then(func() {
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundApparitionAppears)
				g.gameState.SystemCallbacks.Visual.SetScreenFlashColor(r, gr, b)
			}).
			Animate(durationMs, func(progress float32) {
				// Triangle wave: 0 → maxIntensity → 0
				var intensity float64
				if progress < 0.5 {
					intensity = maxIntensity * float64(progress*2.0)
				} else {
					intensity = maxIntensity * float64(2.0-progress*2.0)
				}
				g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(intensity)
			}).
			Then(func() {
				g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(0)
			})
	}
}

// thenResurrectionPulseFlash creates a smooth flash for individual resurrection pulses
func (g *GameScene) thenResurrectionPulseFlash() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.
			Then(func() {
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
				g.gameState.SystemCallbacks.Visual.SetScreenFlashColor(1.0, 1.0, 1.0)
			}).
			Animate(resurrectionPulseFlashMs, func(progress float32) {
				// Triangle wave: 0 → peak → 0
				var intensity float64
				if progress < 0.5 {
					intensity = resurrectionPulseIntensity * float64(progress*2.0)
				} else {
					intensity = resurrectionPulseIntensity * float64(2.0-progress*2.0)
				}
				g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(intensity)
			}).
			Then(func() {
				g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(0)
			})
	}
}

// thenThunderLightningEffect creates multiple lightning flashes with thunder sounds
// Simulates a thunder storm with 4 flashes of varying intensity and timing
func (g *GameScene) thenThunderLightningEffect() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.ThenSequence(

			g.thenSingleLightningFlash(300, 0.8)).
			Wait(150).ThenSequence(

			g.thenSingleLightningFlash(200, 0.6)).
			Wait(350).ThenSequence(

			g.thenSingleLightningFlash(400, 0.9)).
			Wait(100).ThenSequence(

			g.thenSingleLightningFlash(250, 0.7))
	}
}

// thenSingleLightningFlash creates a single lightning flash with asymmetric timing
// Quick rise to peak (20% of duration), slow fade out (80% of duration)
func (g *GameScene) thenSingleLightningFlash(durationMs uint64, maxIntensity float64) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.
			Then(func() {
				// Use SoundEarthquake as placeholder for thunder (no SoundThunder exists yet)
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundEarthquake)
				g.gameState.SystemCallbacks.Visual.SetScreenFlashColor(1.0, 1.0, 1.0)
			}).
			Animate(durationMs, func(progress float32) {
				// Asymmetric flash: quick rise (first 20%), slow fade (remaining 80%)
				var intensity float64
				if progress < 0.2 {
					// Fast rise to peak
					intensity = maxIntensity * float64(progress/0.2)
				} else {
					// Slow fade out
					intensity = maxIntensity * float64(1.0-(float64(progress)-0.2)/0.8)
				}
				g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(intensity)
			}).
			Then(func() {
				g.gameState.SystemCallbacks.Visual.SetScreenFlashIntensity(0)
			})
	}
}
