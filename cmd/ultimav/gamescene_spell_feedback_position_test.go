package main

import (
	"fmt"
	"image"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSpellFeedbackRenderedPositionAndExactDismissal(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })

	for _, combat := range []bool{false, true} {
		t.Run(fmt.Sprintf("combat=%t", combat), func(t *testing.T) {
			var state *game_state.GameState
			if combat {
				state, _ = newProductionCombatCastState(t)
			} else {
				state, _ = gamefixture.NewIntegrationTestBuilder(t).
					WithLocation(references.Britain).
					WithSystemCallbacks().
					Build()
			}
			scene := newProductionCastTestScene(state)
			previous := manager.SetMapPopupViewportProvider(scene.mapPopupViewport)
			defer manager.SetMapPopupViewportProvider(previous)
			longRunicMessage := strings.Repeat("AN NOX VAS FLAM ", 5)
			cases := []struct {
				name     string
				message  string
				feedback event_queue.Sequence
			}{
				{"cured", "Cured!", scene.thenShowSpellFeedback("Cured!", game_state.SoundNoEffect)},
				{"awakened", "Awakened!", scene.thenShowSpellFeedback("Awakened!", game_state.SoundNoEffect)},
				{"no effect", "No effect!", scene.thenShowSpellFeedback("No effect!", game_state.SoundNoEffect)},
				{"magic locked", "Already magic locked!", scene.thenShowSpellFeedback("Already magic locked!", game_state.SoundNoEffect)},
				{"runic", "LAT A'A LON B'B", scene.thenShowRunicSpellSuccess("LAT A'A LON B'B")},
				{"long runic", longRunicMessage, scene.thenShowRunicSpellSuccess(longRunicMessage)},
				{"potion", "No noticeable effect now!", scene.thenPresentPotionEffect(game_state.PotionEffectNoNotice, 0)},
				{"generic", "Moonstone buried!", scene.thenShowConfirmationDialog("Moonstone buried!")},
			}
			for _, size := range []image.Point{{X: 1280, Y: 720}, {X: 1920, Y: 1080}, {X: 2560, Y: 1440}} {
				for _, tt := range cases {
					t.Run(fmt.Sprintf("%dx%d/%s", size.X, size.Y, tt.name), func(t *testing.T) {
						manager.PublishLogicalSize(size.X, size.Y)
						const workflow = "spell_feedback_position_test"
						state.EventQueueManager.Workflow(workflow).
							Priority(event_queue.PriorityNormal).
							Sequential().
							ThenSequence(tt.feedback).
							Start()
						state.EventQueueManager.Update()
						overlay, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
						require.True(t, ok)
						require.True(t, overlay.IsVisible())
						assert.True(t, overlay.IsUserDismissed())
						assert.Equal(t, strings.Fields(tt.message), strings.Fields(overlay.GetContentText()))

						bounds := spellFeedbackPresentationBounds(overlay)
						// Layout and dismissal cover the complete matrix. Rasterize each
						// distinct layout family at one size, including wrapped runic text.
						if size.X == 1280 && (tt.name == "cured" || tt.name == "runic" || tt.name == "long runic") {
							bounds = renderedSpellFeedbackBounds(t, overlay, size)
						}
						centerX := float64(bounds.Min.X+bounds.Max.X) / 2
						centerY := float64(bounds.Min.Y+bounds.Max.Y) / 2
						wantCenterX := 0.37 * float64(size.X)
						if combat {
							battlefield := combatLayout{}.battlefieldRectangle(image.Rectangle{Max: size})
							wantCenterX = float64(battlefield.Min.X+battlefield.Max.X) / 2
							assert.GreaterOrEqual(t, bounds.Min.X, battlefield.Min.X-1)
							assert.LessOrEqual(t, bounds.Max.X, battlefield.Max.X+1)
							assert.Greater(t, centerX-0.37*float64(size.X), 10.0,
								"combat feedback must exclude the Turn Order panel, not center on the full map pane")
						}
						assert.InDelta(t, wantCenterX, centerX, 1.5, "rendered horizontal center")
						assert.InDelta(t, 0.70*float64(size.Y), centerY, 1.5, "confirmation vertical anchor must remain unchanged")

						state.ElapsedMs += 1000
						state.EventQueueManager.Update()
						assert.True(t, state.EventQueueManager.HasWorkflow(workflow), "feedback must wait for dismissal")
						other := &countingDialog{}
						scene.dialogStack.PushModalDialog(other)
						overlay.Dismiss()
						assert.True(t, overlay.IsDismissed())
						assert.Equal(t, 1, scene.dialogStack.Count())
						assert.Same(t, other, scene.dialogStack.PeekTopModalDialog())
						for range 3 {
							state.ElapsedMs += DialogPollingRateMs
							state.EventQueueManager.Update()
						}
						assert.False(t, state.EventQueueManager.HasWorkflow(workflow))
						assert.Same(t, other, scene.dialogStack.PeekTopModalDialog())
						scene.dialogStack.MustRemoveWidgetInstance(other)
					})
				}
			}
		})
	}
}

func spellFeedbackPresentationBounds(overlay *widgets.OverlayDialog) image.Rectangle {
	// Use the existing underlying presentation measurement without expanding
	// the production widget API just for this matrix.
	field := reflect.ValueOf(overlay).Elem().FieldByName("dialog")
	dialog := reflect.NewAt(field.Type(), field.Addr().UnsafePointer()).Elem().Interface().(*widgets.MessageDialog)
	return dialog.PresentationBounds()
}

func renderedSpellFeedbackBounds(t *testing.T, widget widgets.Widget, size image.Point) image.Rectangle {
	t.Helper()
	screen := newPresentationRaster(size.X, size.Y)
	defer screen.Deallocate()
	// Drawing only the popup onto transparency measures its actual rendered bounds.
	widget.Draw(screen)
	pixels := make([]byte, 4*size.X*size.Y)
	screen.ReadPixels(pixels)
	bounds := image.Rectangle{Min: size}
	for y := range size.Y {
		for x := range size.X {
			if pixels[4*(y*size.X+x)+3] == 0 {
				continue
			}
			bounds.Min.X = min(bounds.Min.X, x)
			bounds.Min.Y = min(bounds.Min.Y, y)
			bounds.Max.X = max(bounds.Max.X, x+1)
			bounds.Max.Y = max(bounds.Max.Y, y+1)
		}
	}
	require.False(t, bounds.Empty(), "visible feedback must render pixels")

	return bounds
}
