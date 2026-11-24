package main

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// Lady Janell's conversation ends by asking for this screen, where one member at
// a time pays to cast spells of one Circle above their level. ref:PROGRESS-0005

const (
	circleStudyTitle        = "Study of the Circles"
	circleStudyConfirmLabel = "Study"
	circleStudyCancelLabel  = "Leave"
	// circleStudyConfirmPrompt asks before the gold is spent, as two rows of
	// near equal length: price, then member name.
	circleStudyConfirmPrompt = "Each member may study once only.\nPay %d gold for %s to study?"
	// circleStudyConfirmCenterY puts the question just below the study list, as
	// a share of the screen from the top, at every supported resolution.
	circleStudyConfirmCenterY = 0.83
	// noCircleStudyChoice means no member is waiting to be asked.
	noCircleStudyChoice = -1
)

// thenRunCircleStudy keeps the study list open until the player leaves it, so a
// second member can study in the same visit. The list only records a choice; the
// workflow asks the question, so no dialog opens another from its own key handler.
func (g *GameScene) thenRunCircleStudy() func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		var dialog *widgets.CharacterSelectionDialog
		var left bool
		chosen := noCircleStudyChoice

		return q.Then(func() {
			dialog = g.newCircleStudyDialog(
				func(rosterIndex int) { chosen = rosterIndex },
				func() { left = true },
			)
			g.dialogStack.PushModalDialog(dialog)
		}).
			While(func() bool { return !left }, func() {
				if chosen != noCircleStudyChoice {
					g.confirmCircleStudy(dialog, chosen)
					chosen = noCircleStudyChoice
				}
			}, DialogPollingRateMs).
			Then(func() {
				g.dialogStack.RemoveWidgetInstance(dialog)
				g.keyboard.SetForceWaitAnyKey(KeyboardDebounceMs)
			}).
			Wait(KeyboardDebounceMs)
	}
}

func (g *GameScene) newCircleStudyDialog(onSelect func(int), onCancel func()) *widgets.CharacterSelectionDialog {
	return widgets.NewCharacterSelectionDialog(widgets.CharacterSelectionConfig{
		Title:          circleStudyTitle,
		Options:        g.circleStudyOptions(),
		OnSelect:       onSelect,
		OnCancel:       onCancel,
		Keyboard:       g.keyboard,
		SpriteSheet:    g.spriteSheet,
		Placement:      gameScreenPercents,
		ConfirmLabel:   circleStudyConfirmLabel,
		CancelLabel:    circleStudyCancelLabel,
		GuardResources: false,
	})
}

// confirmCircleStudy asks before the gold is spent. Only a yes commits; a no
// returns to the list with nothing changed.
func (g *GameScene) confirmCircleStudy(dialog *widgets.CharacterSelectionDialog, rosterIndex int) {
	name := g.gameState.PartyState.GetCharacter(rosterIndex).GetNameAsString()
	command := widgets.NewYesNoCommand(
		func() { g.answerCircleStudy(dialog, rosterIndex, true) },
		func() { g.answerCircleStudy(dialog, rosterIndex, false) },
		&g.dialogStack,
	)
	prompt := fmt.Sprintf(circleStudyConfirmPrompt, game_state.CircleStudyPrice, name)
	g.dialogStack.PushModalDialog(widgets.NewYesNoInputBoxAt(
		prompt, command, g.keyboard, &g.dialogStack, circleStudyConfirmCenterY))
}

// answerCircleStudy applies the answer to the question once it has closed.
func (g *GameScene) answerCircleStudy(dialog *widgets.CharacterSelectionDialog, rosterIndex int, agreed bool) {
	if agreed {
		g.studyCircle(dialog, rosterIndex)
	}
	g.keyboard.SetForceWaitAnyKey(KeyboardDebounceMs)
}

// studyCircle commits one member's study and shows the list as it now stands.
func (g *GameScene) studyCircle(dialog *widgets.CharacterSelectionDialog, rosterIndex int) {
	err := g.gameState.StudyCircle(rosterIndex)
	switch {
	case err == nil:
		member := g.gameState.PartyState.GetCharacter(rosterIndex)
		g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMagic)
		g.gameState.SystemCallbacks.Message.AddRowStr(fmt.Sprintf(
			"%s may now cast spells of Circle %d.", member.GetNameAsString(), member.SpellCircleLimit()))
	case errors.Is(err, game_state.ErrCircleStudyUnavailable), errors.Is(err, game_state.ErrCircleStudyInsufficientGold):
		// The list was built from the same state, so a refusal only means it is stale.
	default:
		panic("circle study failed unexpectedly: " + err.Error())
	}
	dialog.SetOptions(g.circleStudyOptions())
}

// circleStudyOptions projects each active member with the reason they may or
// may not study, in words as well as by state. The wording is kept short: at
// 1080p a roster row holds about thirteen letters of name and status together,
// and the detail box about fifteen letters a row.
func (g *GameScene) circleStudyOptions() []widgets.CharacterSelectionOption {
	state := g.gameState.GetCircleStudyState()
	options := make([]widgets.CharacterSelectionOption, 0, len(state.Candidates))
	for _, candidate := range state.Candidates {
		character := g.gameState.PartyState.GetCharacter(candidate.RosterIndex)
		option := widgets.CharacterSelectionOption{
			CharacterIndex: candidate.RosterIndex,
			PartyPosition:  candidate.RosterIndex + 1,
			Name:           candidate.Name,
			StatusKind:     character.GetStatus(),
			PortraitSprite: character.GetKeySpriteIndex(),
			State:          widgets.CharacterSelectionUnavailable,
		}
		populateCircleStudyOption(&option, candidate, state)
		options = append(options, option)
	}

	return options
}

func populateCircleStudyOption(
	option *widgets.CharacterSelectionOption,
	candidate game_state.CircleStudyCandidate,
	state game_state.CircleStudyState,
) {
	fact := func(label, value string) widgets.CharacterSelectionFact {
		return widgets.CharacterSelectionFact{Label: label, Value: value}
	}
	circle := fact("Circle", strconv.Itoa(candidate.Circle))
	switch candidate.Eligibility {
	case game_state.CircleStudyEligible:
		option.Facts = []widgets.CharacterSelectionFact{
			fact("Circle", fmt.Sprintf("%d to %d", candidate.Circle, candidate.Circle+1)),
			fact("Cost", strconv.Itoa(state.Price)+" gold"),
			fact("Gold", strconv.Itoa(state.Gold)),
		}
		if state.Gold < state.Price {
			option.Status = "Short"
			option.Details = []string{"Thou hast not enough gold to study."}

			return
		}
		option.State = widgets.CharacterSelectionAvailable
		option.Status = "Ready"
		option.Facts = append(option.Facts, fact("After", strconv.Itoa(state.Gold-state.Price)))
		option.Details = []string{"One Circle above level, for good."}
	case game_state.CircleStudyNoMagic:
		option.Status = "Can't"
		option.Facts = []widgets.CharacterSelectionFact{fact("Class", candidate.Class)}
		option.Details = []string{"A " + candidate.Class + " casts no spells."}
	case game_state.CircleStudyDead:
		option.Status = "Dead"
		option.Details = []string{"The dead cannot study."}
	case game_state.CircleStudyAlreadyStudied:
		option.Status = "Done"
		option.Facts = []widgets.CharacterSelectionFact{circle}
		option.Details = []string{"Casts one Circle above level."}
	case game_state.CircleStudyNothingToLearn:
		option.Status = "Adept"
		option.Facts = []widgets.CharacterSelectionFact{circle}
		option.Details = []string{"Already casts the eighth Circle."}
	default:
		panic("unknown circle study eligibility")
	}
}
