package widgets

import (
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

var _ Widget = (*CharacterSelectionDialog)(nil)

// CharacterSelectionConfig configures one detached ordinary character selector.
type CharacterSelectionConfig struct {
	Title        string
	Options      []CharacterSelectionOption
	OnSelect     func(int)
	OnCancel     func()
	Keyboard     *input.Keyboard
	SpriteSheet  *sprites.SpriteSheet
	Placement    sprites.PercentBasedPlacement
	ConfirmLabel string
	CancelLabel  string

	// GuardResources explicitly selects the camping watch's HP/MP comparison layout.
	GuardResources bool
}

// CharacterSelectionDialog owns ordinary character inspection and confirmation.
type CharacterSelectionDialog struct {
	navigation
	title        string
	options      []CharacterSelectionOption
	onSelect     func(int)
	onCancel     func()
	keyboard     *input.Keyboard
	shell        *characterRosterDialogShell
	selected     int
	confirmLabel string
	cancelLabel  string
}

// NewCharacterSelectionDialog creates a keyboard-only character selector.
func NewCharacterSelectionDialog(config CharacterSelectionConfig) *CharacterSelectionDialog {
	if config.Keyboard == nil {
		panic("CharacterSelectionDialog: Keyboard is required")
	}
	if config.SpriteSheet == nil {
		panic("CharacterSelectionDialog: SpriteSheet is required")
	}
	if config.OnSelect == nil {
		panic("CharacterSelectionDialog: OnSelect is required")
	}
	if config.OnCancel == nil {
		panic("CharacterSelectionDialog: OnCancel is required")
	}
	if len(config.Options) == 0 {
		panic("CharacterSelectionDialog: at least one option is required")
	}
	confirmLabel := config.ConfirmLabel
	if confirmLabel == "" {
		confirmLabel = "Select"
	}
	cancelLabel := config.CancelLabel
	if cancelLabel == "" {
		cancelLabel = defaultCancelActionLabel
	}
	dialog := &CharacterSelectionDialog{
		navigation:   newNavigation(config.Keyboard.IsKeyHeld),
		title:        config.Title,
		options:      cloneCharacterSelectionOptions(config.Options),
		onSelect:     config.OnSelect,
		onCancel:     config.OnCancel,
		keyboard:     config.Keyboard,
		shell:        newCharacterRosterDialogShell(config.Placement, config.SpriteSheet),
		selected:     initialCharacterSelection(config.Options),
		confirmLabel: confirmLabel,
		cancelLabel:  cancelLabel,
	}
	dialog.keyboard.SetForceWaitAnyKey(DialogDismissalDelayMs)

	dialog.shell.healthRoster = !config.GuardResources && config.Options[0].Health != nil
	dialog.shell.guardResources = config.GuardResources

	return dialog
}

// SetOptions replaces the roster after its owner changed game state, keeping
// the inspected character selected.
func (d *CharacterSelectionDialog) SetOptions(options []CharacterSelectionOption) {
	if len(options) == 0 {
		panic("CharacterSelectionDialog: at least one option is required")
	}
	inspected := d.options[d.selected].CharacterIndex
	d.options = cloneCharacterSelectionOptions(options)
	d.selected = 0
	for index, option := range d.options {
		if option.CharacterIndex == inspected {
			d.selected = index

			break
		}
	}
}

// Draw renders the detached roster and the currently inspected option.
func (d *CharacterSelectionDialog) Draw(screen *ebiten.Image) {
	d.shell.draw(screen, characterRosterDialogView{
		title:       d.title,
		rosterTitle: "PARTY",
		options:     d.options,
		selected:    d.selected,
		footer:      d.footer(),
	})
}

// Update routes keyboard inspection, confirmation, and cancellation to the dialog owner.
func (d *CharacterSelectionDialog) Update() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.HandleEscape()

		return
	}
	if key, owned := d.Poll(d.keyboard,
		ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyLeft, ebiten.KeyRight, ebiten.KeyPageUp, ebiten.KeyPageDown); owned {
		if key != nil {
			switch *key { //nolint:exhaustive // Compact party selectors have no paging action.
			case ebiten.KeyDown, ebiten.KeyRight:
				d.HandleKeyDown()
			case ebiten.KeyUp, ebiten.KeyLeft:
				d.HandleKeyUp()
			}
		}

		return
	}
	if d.handleNumberKey() {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		d.HandleEnter()
	}
}

// HandleEscape cancels ordinary selection.
func (d *CharacterSelectionDialog) HandleEscape() {
	d.onCancel()
}

// HandleKeyDown inspects the next visible row, including unavailable rows.
func (d *CharacterSelectionDialog) HandleKeyDown() {
	d.selected = (d.selected + 1) % len(d.options)
}

// HandleKeyUp inspects the previous visible row, including unavailable rows.
func (d *CharacterSelectionDialog) HandleKeyUp() {
	d.selected--
	if d.selected < 0 {
		d.selected = len(d.options) - 1
	}
}

// HandleEnter confirms available and warning rows. Unavailable rows stay open.
func (d *CharacterSelectionDialog) HandleEnter() {
	option := d.options[d.selected]
	if !option.confirmable() {
		return
	}
	d.onSelect(option.CharacterIndex)
}

// HandleNumber inspects one party position without confirming it.
func (d *CharacterSelectionDialog) HandleNumber(position int) {
	for index, option := range d.options {
		if option.PartyPosition == position {
			d.selected = index

			return
		}
	}
}

func (d *CharacterSelectionDialog) handleNumberKey() bool {
	for position := range 7 {
		regular := ebiten.Key(int(ebiten.Key0) + position)
		digit := ebiten.Key(int(ebiten.KeyDigit0) + position)
		numpad := ebiten.Key(int(ebiten.KeyNumpad0) + position)
		if d.keyboard.IsKeyPressedAndRegister(regular) ||
			d.keyboard.IsKeyPressedAndRegister(digit) ||
			d.keyboard.IsKeyPressedAndRegister(numpad) {
			d.HandleNumber(position)

			return true
		}
	}

	return false
}

func (d *CharacterSelectionDialog) footer() string {
	numberRange := "1-6"
	for _, option := range d.options {
		if option.PartyPosition == 0 {
			numberRange = "0-6"

			break
		}
	}

	return "[Arrows/" + numberRange + "] Inspect   [Enter] " + d.confirmLabel + "   [Esc] " + d.cancelLabel
}

func initialCharacterSelection(options []CharacterSelectionOption) int {
	for index, option := range options {
		if option.confirmable() {
			return index
		}
	}

	return 0
}

func cloneCharacterSelectionOptions(options []CharacterSelectionOption) []CharacterSelectionOption {
	cloned := make([]CharacterSelectionOption, len(options))
	for index, option := range options {
		cloned[index] = cloneCharacterSelectionOption(option)
	}

	return cloned
}

func cloneCharacterSelectionOption(option CharacterSelectionOption) CharacterSelectionOption {
	if option.Resources != nil {
		resources := *option.Resources
		option.Resources = &resources
	}
	if option.Health != nil {
		health := *option.Health
		option.Health = &health
	}
	option.Facts = slices.Clone(option.Facts)
	option.Details = slices.Clone(option.Details)

	return option
}
