package widgets

import (
	"fmt"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

var _ Widget = (*NewOrderDialog)(nil)

// NewOrderStage represents the current stage of the two-stage selection.
type NewOrderStage int

// New Order stages: pick the first companion, then the one to swap it with.
const (
	Stage1SelectFirst NewOrderStage = iota
	Stage2SelectSecond

	newOrderStatusGood     = "Good"
	newOrderStatusPoisoned = "Poisoned"
	newOrderStatusCharmed  = "Charmed"
)

// NewOrderDialog owns the two-stage New Order state machine.
type NewOrderDialog struct {
	navigation
	options        []CharacterSelectionOption
	keyboard       *input.Keyboard
	shell          *characterRosterDialogShell
	swapCallback   func(first, second int)
	cancelCallback func()
	activeSlot     int

	stage              NewOrderStage
	selected           int
	firstSelectedIndex int
	isComplete         bool
}

// NewNewOrderDialog creates a two-stage character reorder dialog.
func NewNewOrderDialog(
	partyState *party_state.PartyState,
	activeSlot int,
	swapCallback func(first, second int),
	cancelCallback func(),
	keyboard *input.Keyboard,
	spriteSheet *sprites.SpriteSheet,
	placement sprites.PercentBasedPlacement,
) *NewOrderDialog {
	if partyState == nil {
		panic("NewOrderDialog: PartyState is required")
	}
	if swapCallback == nil {
		panic("NewOrderDialog: swapCallback is required")
	}
	if cancelCallback == nil {
		panic("NewOrderDialog: cancelCallback is required")
	}
	if keyboard == nil {
		panic("NewOrderDialog: Keyboard is required")
	}
	options := newOrderOptions(partyState)
	if len(options) == 0 {
		panic("NewOrderDialog: active party is required")
	}
	dialog := &NewOrderDialog{
		navigation:         newNavigation(keyboard.IsKeyHeld),
		options:            options,
		keyboard:           keyboard,
		shell:              newCharacterRosterDialogShell(placement, spriteSheet),
		swapCallback:       swapCallback,
		cancelCallback:     cancelCallback,
		activeSlot:         activeSlot,
		stage:              Stage1SelectFirst,
		selected:           firstMovableNewOrderOption(options),
		firstSelectedIndex: -1,
		isComplete:         false,
	}
	dialog.keyboard.SetForceWaitAnyKey(DialogDismissalDelayMs)

	return dialog
}

// Draw renders the current New Order stage.
func (d *NewOrderDialog) Draw(screen *ebiten.Image) {
	d.shell.draw(screen, characterRosterDialogView{
		title:       d.title(),
		rosterTitle: "CURRENT ORDER",
		options:     d.viewOptions(),
		selected:    d.selected,
		footer:      d.footer(),
	})
}

// Update handles keyboard input for the current New Order stage.
func (d *NewOrderDialog) Update() {
	if d.isComplete {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.HandleEscape()

		return
	}
	if d.handleDirectionalKey() {
		return
	}
	if d.handleNumberKey() {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace) {
		d.HandleEnter()
	}
}

func (d *NewOrderDialog) handleDirectionalKey() bool {
	key, owned := d.Poll(d.keyboard,
		ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyLeft, ebiten.KeyRight, ebiten.KeyPageUp, ebiten.KeyPageDown)
	if key != nil {
		switch *key { //nolint:exhaustive // Compact party selectors have no paging action.
		case ebiten.KeyDown, ebiten.KeyRight:
			d.HandleKeyDown()
		case ebiten.KeyUp, ebiten.KeyLeft:
			d.HandleKeyUp()
		}
	}

	return owned
}

// HandleKeyDown inspects the next visible row, including Avatar and the lock.
func (d *NewOrderDialog) HandleKeyDown() {
	d.selected = (d.selected + 1) % len(d.options)
}

// HandleKeyUp inspects the previous visible row, including Avatar and the lock.
func (d *NewOrderDialog) HandleKeyUp() {
	d.selected--
	if d.selected < 0 {
		d.selected = len(d.options) - 1
	}
}

// HandleNumber inspects one party position without confirming it.
func (d *NewOrderDialog) HandleNumber(position int) {
	for index, option := range d.options {
		if option.PartyPosition == position {
			d.selected = index

			return
		}
	}
}

// HandleEnter locks the first movable companion or confirms the second.
func (d *NewOrderDialog) HandleEnter() {
	option := d.options[d.selected]
	if option.CharacterIndex == party_state.AvatarCharacterIndex ||
		(d.stage == Stage2SelectSecond && option.CharacterIndex == d.firstSelectedIndex) {
		return
	}
	switch d.stage {
	case Stage1SelectFirst:
		d.firstSelectedIndex = option.CharacterIndex
		d.stage = Stage2SelectSecond
		d.moveToNextSwapOption()
	case Stage2SelectSecond:
		d.isComplete = true
		d.swapCallback(d.firstSelectedIndex, option.CharacterIndex)
	default:
		panic("unknown New Order stage")
	}
}

// HandleEscape cancels stage 1 or returns stage 2 to the first choice.
func (d *NewOrderDialog) HandleEscape() {
	if d.stage == Stage2SelectSecond {
		d.stage = Stage1SelectFirst
		d.firstSelectedIndex = -1

		return
	}
	d.isComplete = true
	d.cancelCallback()
}

func (d *NewOrderDialog) handleNumberKey() bool {
	for position := 1; position <= 6; position++ {
		regular := ebiten.Key(int(ebiten.Key1) + position - 1)
		digit := ebiten.Key(int(ebiten.KeyDigit1) + position - 1)
		numpad := ebiten.Key(int(ebiten.KeyNumpad1) + position - 1)
		if d.keyboard.IsKeyPressedAndRegister(regular) ||
			d.keyboard.IsKeyPressedAndRegister(digit) ||
			d.keyboard.IsKeyPressedAndRegister(numpad) {
			d.HandleNumber(position)

			return true
		}
	}

	return false
}

func (d *NewOrderDialog) moveToNextSwapOption() {
	for offset := 1; offset < len(d.options); offset++ {
		candidate := (d.selected + offset) % len(d.options)
		index := d.options[candidate].CharacterIndex
		if index != party_state.AvatarCharacterIndex && index != d.firstSelectedIndex {
			d.selected = candidate

			return
		}
	}
}

func (d *NewOrderDialog) viewOptions() []CharacterSelectionOption {
	options := cloneCharacterSelectionOptions(d.options)
	for index := range options {
		option := &options[index]
		option.Facts = []CharacterSelectionFact{{Label: "Position", Value: strconv.Itoa(option.PartyPosition)}}
		if option.CharacterIndex == d.activeSlot {
			option.Active = true
			option.Facts = append(option.Facts, CharacterSelectionFact{Label: "Active slot", Value: "Yes"})
		}
		if option.CharacterIndex == party_state.AvatarCharacterIndex {
			option.State = CharacterSelectionUnavailable
			option.Details = []string{"The Avatar must lead the party."}

			continue
		}
		if d.stage == Stage1SelectFirst {
			option.State = CharacterSelectionAvailable
			option.Details = []string{"Choose the first companion to move."}

			continue
		}
		if option.CharacterIndex == d.firstSelectedIndex {
			option.State = CharacterSelectionLocked
			option.Details = []string{"Swap " + option.Name + " with whom?", "Choose another companion to complete the swap."}

			continue
		}
		option.State = CharacterSelectionAvailable
		first := d.optionByCharacterIndex(d.firstSelectedIndex)
		option.Facts = []CharacterSelectionFact{
			{Label: first.Name, Value: fmt.Sprintf("%d -> %d", first.PartyPosition, option.PartyPosition)},
			{Label: option.Name, Value: fmt.Sprintf("%d -> %d", option.PartyPosition, first.PartyPosition)},
		}
		option.Details = []string{d.activeSwapConsequence(first, *option)}
	}

	return options
}

func (d *NewOrderDialog) activeSwapConsequence(first, second CharacterSelectionOption) string {
	switch d.activeSlot {
	case first.CharacterIndex:
		return second.Name + " is now the active player."
	case second.CharacterIndex:
		return first.Name + " is now the active player."
	default:
		return "The active player is unchanged."
	}
}

func (d *NewOrderDialog) optionByCharacterIndex(characterIndex int) CharacterSelectionOption {
	for _, option := range d.options {
		if option.CharacterIndex == characterIndex {
			return option
		}
	}
	panic("New Order character is not visible")
}

func (d *NewOrderDialog) title() string {
	if d.stage == Stage1SelectFirst {
		return "New Order"
	}

	return "Swap " + d.optionByCharacterIndex(d.firstSelectedIndex).Name + " With..."
}

func (d *NewOrderDialog) footer() string {
	if d.stage == Stage1SelectFirst {
		return "[Arrows/1-6] Inspect   [Enter/Space] Lock   [Esc] Cancel"
	}

	return "[Arrows/1-6] Inspect   [Enter/Space] Swap   [Esc] Choose First"
}

// IsComplete reports whether the dialog has swapped or cancelled.
func (d *NewOrderDialog) IsComplete() bool { return d.isComplete }

func newOrderOptions(partyState *party_state.PartyState) []CharacterSelectionOption {
	options := make([]CharacterSelectionOption, 0, party_state.MaxActiveParty)
	for index := range party_state.MaxActiveParty {
		character := partyState.GetCharacter(index)
		if character == nil || character.PartyMap != party_state.PartyMapActive || character.GetNameAsString() == "" {
			continue
		}
		state := CharacterSelectionAvailable
		details := []string{"Choose the first companion to move."}
		if index == party_state.AvatarCharacterIndex {
			state = CharacterSelectionUnavailable
			details = []string{"The Avatar must lead the party."}
		}
		options = append(options, CharacterSelectionOption{
			CharacterIndex: index,
			PartyPosition:  index + 1,
			Name:           character.GetNameAsString(),
			Status:         newOrderStatus(character.GetStatus()),
			StatusKind:     character.GetStatus(),
			PortraitSprite: character.GetKeySpriteIndex(),
			State:          state,
			Active:         false,
			Facts:          nil,
			Details:        details,
			Health:         nil,
			Resources:      nil,
		})
	}

	return options
}

func firstMovableNewOrderOption(options []CharacterSelectionOption) int {
	for index, option := range options {
		if option.CharacterIndex != party_state.AvatarCharacterIndex {
			return index
		}
	}

	return 0
}

func newOrderStatus(status party_state.CharacterStatus) string {
	switch status {
	case party_state.Good:
		return newOrderStatusGood
	case party_state.Poisoned:
		return newOrderStatusPoisoned
	case party_state.Sleep:
		return "Asleep"
	case party_state.Dead:
		return "Dead"
	case party_state.Charmed:
		return newOrderStatusCharmed
	default:
		return "Unknown"
	}
}
