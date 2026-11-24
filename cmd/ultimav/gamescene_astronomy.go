package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// ShowTelescopeInteraction is the SystemCallbacks wrapper that initiates telescope viewing.
// Called from GameState when player looks at a telescope tile.
func (g *GameScene) ShowTelescopeInteraction() {
	g.startTelescopeWorkflow(false)
}

// ShowSpyglassInteraction is the SystemCallbacks wrapper that initiates spyglass viewing.
// Called from GameState when player uses spyglass from the Use menu.
func (g *GameScene) ShowSpyglassInteraction(complete func()) {
	g.startSpyglassWorkflow(complete)
}

// StartTelescopeWorkflow initiates the telescope viewing sequence.
// Branches based on time of day: night shows astronomy, day causes sun damage.
func (g *GameScene) StartTelescopeWorkflow() {
	g.startTelescopeWorkflow(true)
}

func (g *GameScene) startTelescopeWorkflow(advanceTime bool) {
	var astronomyDialog *widgets.AstronomyDialog
	g.gameState.EventQueueManager.Workflow("telescope_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			g.output.AddText("Look-")
		}).ThenSequence(

		func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
			if !isTelescopeDaytime(g.gameState.DateTime.Hour) {
				q.Then(func() {
					g.gameState.SystemCallbacks.UI.ShowActionFeedback("Thou dost see the night sky!", true)
				}).ThenSequence(

					g.thenShowAstronomyDialog(&astronomyDialog))
			} else {
				q.Then(func() {
					g.gameState.SystemCallbacks.UI.ShowActionFeedback("Thou dost see the sun!", true)
				}).ThenSequence(

					g.thenApplySunDamage())
			}

			return q
		}).
		Finally(

			func() {
				if astronomyDialog != nil {
					g.dialogStack.RemoveWidgetInstance(astronomyDialog)
					astronomyDialog = nil
				}
				g.keyboard.ResetReleaseDebouncing()
				if advanceTime {
					g.gameState.AdvanceGameplayMinutesAndProcessProvisions(1)
				}
			}).
		Start()
}

func isTelescopeDaytime(hour uint8) bool {
	return hour >= 6 && hour <= 17
}

// thenShowAstronomyDialog returns an event queue modifier that shows the astronomy dialog.
func (g *GameScene) thenShowAstronomyDialog(
	dialog **widgets.AstronomyDialog,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		return q.
			Then(func() {
				// Get Shadowlord planet indices
				shadowlordPlanets := g.gameState.GetShadowlordPlanetIndices()

				// REDUX DEVIATION: ASTRO-0004: original uses fixed star bounds and
				// interleaved planet/comet drawing. Redux retains responsive modern
				// dialog geometry and layering for readable presentation.
				*dialog = widgets.NewAstronomyDialog(widgets.AstronomyDialogConfig{
					Placement:               centeredGameDialogPercents,
					Keyboard:                g.keyboard,
					SpriteSheet:             g.spriteSheet,
					ShadowlordPlanetIndices: shadowlordPlanets,
					GameYear:                g.gameState.DateTime.Year,
					GameMonth:               g.gameState.DateTime.Month,
					GameDay:                 g.gameState.DateTime.Day,
					RandomIntInRange:        g.gameState.RandomIntInRange,
				})

				g.dialogStack.PushModalDialog(*dialog)
			}).
			While(
				func() bool { return !(*dialog).IsDismissed() },
				func() {}, // Empty action - just polling
				16,        // ~60fps poll rate
			).
			Then(func() {
				g.keyboard.ResetReleaseDebouncing()
			})
	}
}

// thenApplySunDamage returns an event queue modifier that applies sun damage.
func (g *GameScene) thenApplySunDamage() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	targetIndex := g.gameState.TelescopeSunDamageTarget()

	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if targetIndex < 0 {
			return q
		}

		return q.ThenSequence(

			game_state.NewPartyStateBuilder(g.gameState).
					WithFixedDamage(game_state.TelescopeSunDamage).
					ToCharacter(targetIndex).
					WithFlash(200, 1.0, 1.0, 0.8, 0.7).
					WithSound(game_state.SoundWhiteNoiseDamage).
					Build()).
			Wait(500) // Brief pause after damage
	}
}

// StartSpyglassWorkflow initiates the spyglass viewing sequence.
// Requirements:
//   - Spyglass must be in inventory
//   - Must be on Britannia or an above-ground small map
//   - Daytime: "No stars!" (no sun damage unlike telescope)
//   - Nighttime: Shows astronomy dialog
func (g *GameScene) StartSpyglassWorkflow() {
	g.startSpyglassWorkflow(nil)
}

func (g *GameScene) startSpyglassWorkflow(complete func()) {
	var astronomyDialog *widgets.AstronomyDialog
	// Check inventory
	spyglassQuantity := g.gameState.PartyState.Inventory.QuestItems.GetQuantity(references.Spyglass)
	if spyglassQuantity == nil || !(*spyglassQuantity).HasSome() {
		g.gameState.SystemCallbacks.UI.ShowActionFeedback("No spyglass!", true)
		if complete != nil {
			complete()
		}

		return
	}

	if !g.gameState.IsSpyglassLocationValid() {
		g.gameState.SystemCallbacks.UI.ShowActionFeedback("Not here!", true)
		if complete != nil {
			complete()
		}

		return
	}

	// Branch day/night (no sun damage for spyglass)
	g.gameState.EventQueueManager.Workflow("spyglass_workflow").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {
			g.output.AddText("Look-")
		}).ThenSequence(

		func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
			if g.gameState.IsUseAstronomyNight() {
				q.Then(func() {
					g.gameState.SystemCallbacks.UI.ShowActionFeedback("Thou dost see the night sky!", true)
				}).ThenSequence(

					g.thenShowAstronomyDialog(&astronomyDialog))
			} else {
				q.Then(func() {
					g.gameState.SystemCallbacks.UI.ShowActionFeedback("No stars!", true)
				})
			}

			return q
		}).
		Finally(

			func() {
				if astronomyDialog != nil {
					g.dialogStack.RemoveWidgetInstance(astronomyDialog)
					astronomyDialog = nil
				}
				g.keyboard.ResetReleaseDebouncing()
				if complete == nil {
					g.gameState.AdvanceGameplayMinutesAndProcessProvisions(1)
				} else {
					complete()
				}
			}).
		Start()
}
