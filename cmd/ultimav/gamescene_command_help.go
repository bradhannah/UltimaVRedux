package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

const (
	commandHelpMovementKeys = "Arrows / keypad"
	commandHelpPartyKeys    = "0 / 1-6"
	commandKeyLabelSpace    = "Space"
	commandKeyLabelEscape   = "Esc"
	commandHelpJournalKeys  = "Shift+J or F5"
	commandHelpMapKeys      = "Shift+M or F6"
	commandHelpRunesKeys    = "Ctrl+R or F7"
	commandHelpHelpKeys     = "/ or ? or F1"
	commandHelpHistoryKeys  = "PgUp/PgDn/Home/End"
	commandHelpFirstKey     = "[F1] Help"
	commandHelpMarkKey      = "Ctrl+N"
)

func (g *GameScene) handleCommandHelpShortcut(key ebiten.Key) bool {
	if key != ebiten.KeySlash && key != ebiten.KeyF1 || g.secondaryKeyState != PrimaryInput ||
		g.gameState.EventQueueManager.HasActiveWorkflows() {
		return false
	}
	g.showCommandHelp()

	return true
}

// offerHelpHint points a new player at Help in the message window as a game
// begins, until Help has been opened once on this computer.
func (g *GameScene) offerHelpHint() {
	if g.gameConfig.GetHelpHintSeen() {
		return
	}
	widgets.AppendControlHints(g.output.AppendLine(), commandHelpFirstKey).Done()
}

func (g *GameScene) showCommandHelp() {
	if !g.gameConfig.GetHelpHintSeen() {
		if err := g.gameConfig.SetHelpHintSeen(); err != nil {
			logging.Warnf(logging.CategoryUI, "Unable to remember that Help was opened: %v", err)
		}
	}
	var dialog *widgets.CommandHelpDialog
	dialog = widgets.NewCommandHelpDialog(commandHelpEntries(g.gameState.MapState.PlayerLocation.Location.GetMapType()),
		g.currentHelpStatus(), g.keyboard, func() { g.dialogStack.MustRemoveWidgetInstance(dialog) })
	g.dialogStack.PushModalDialog(dialog)
}

func commandHelpEntries(context references.GeneralMapType) []widgets.CommandHelpEntry {
	direction := "Arrows or keypad choose the direction. Space passes the turn."
	if context == references.SmallMapType || context == references.LargeMapType {
		direction = "Arrows or keypad choose the direction. Esc cancels and costs no turn; Space passes the turn."
	}
	entries := []widgets.CommandHelpEntry{
		{Key: commandHelpMovementKeys, Purpose: "Move", NextInput: "Move one step; hold to repeat."},
		{Key: commandKeyLabelSpace, Purpose: "Pass", NextInput: "Wait one turn; hold to keep waiting."},
		{Key: "A", Purpose: "Attack", NextInput: direction},
		{Key: "B", Purpose: "Board a vehicle", NextInput: "Stand on a boardable vehicle."},
		{Key: "C", Purpose: "Cast a prepared spell", NextInput: "Choose a caster and a spell. Enter names the next step: " +
			"a spell with no aim is cast at once; any other asks who or where first. " +
			"Casting spends MP and one mixture. Cancelling costs nothing."},
		{Key: "E", Purpose: "Enter a location", NextInput: "Stand on a town, dungeon or other entrance."},
		{Key: "F", Purpose: "Fire a cannon", NextInput: direction},
		{Key: "G", Purpose: "Get an item", NextInput: direction},
		{Key: "H", Purpose: "Hole up and camp", NextInput: "Follow the camp prompts. While the party sleeps, Space hurries the rest along."},
		{Key: "I", Purpose: "Ignite a torch", NextInput: "Lights one of your torches."},
		{Key: "J", Purpose: "Jimmy a lock", NextInput: direction + " Choose a character when asked."},
		{Key: "K", Purpose: "Klimb", NextInput: "Use a ladder or climbable terrain; choose direction when asked."},
		{Key: "L", Purpose: "Look", NextInput: direction},
		{Key: "M", Purpose: "Mix reagents", NextInput: "Choose a spell, then how many: Left/Right or digits set the amount, M the most that fits, Enter mixes. Tab opens Experiment to try other ingredients."},
		{Key: "N", Purpose: "New party order", NextInput: "Swap two members. The Avatar must lead."},
		{Key: "O", Purpose: "Open", NextInput: direction},
		{Key: "P", Purpose: "Push", NextInput: direction},
		{Key: "Q", Purpose: "Quick save", NextInput: "Saves to the slot you last saved or loaded. Asks first unless Confirm Quick Save is off. Save or load a slot once before using it."},
		{Key: "Ctrl+Q", Purpose: "Quick load", NextInput: "Reloads the slot you last saved or loaded, without saving first. Asks first unless Confirm Quick Save is off."},
		{Key: "R", Purpose: "Ready equipment", NextInput: "Choose a member and an item; Enter readies or unreadies it. Space shows more detail."},
		{Key: "S", Purpose: commandLabelSearch, NextInput: direction},
		{Key: "T", Purpose: "Talk", NextInput: direction + " Type a word and press Enter. Try NAME, JOB and BYE. PgUp rereads this conversation."},
		{Key: "U", Purpose: "Use an item", NextInput: "Choose an item and who uses it, then answer what it asks."},
		{Key: "V", Purpose: "View with a gem", NextInput: "Uses one gem. Any key closes the view."},
		{Key: "X", Purpose: "Exit a vehicle", NextInput: "Step off your horse, carpet or ship."},
		{Key: "Y", Purpose: "Yell", NextInput: "Type words, then Enter."},
		{Key: "Z", Purpose: "Inspect party statistics", NextInput: "Look over each member; Esc closes."},
		{Key: commandHelpPartyKeys, Purpose: "Active party member", NextInput: "0 selects all; 1-6 selects a party position."},
		{Key: commandHelpJournalKeys, Purpose: "Journal", NextInput: "Read what you have learned and keep your own notes under My Notes; N writes a new one. Esc closes."},
		{Key: commandHelpMapKeys, Purpose: "Traveller's map", NextInput: "Browse places, notes and your position; Esc closes. Time stops while it is open."},
		{Key: commandHelpMarkKey, Purpose: "Mark current position", NextInput: "Writes a Map note where you stand. Needs a sextant reading, outdoors on the surface. Esc goes back."},
		{Key: "F4", Purpose: "Last battle summary", NextInput: "Shows what the last battle gave you. Enter or Esc closes. Costs no turn."},
		{Key: "Ctrl+M", Purpose: "Music on or off", NextInput: "Turns the music on or off."},
		{Key: commandHelpRunesKeys, Purpose: "Runic alphabet reference", NextInput: "Shows or hides the rune chart on the right, while exploring, reading a sign or talking. The reading is up to you. No time passes."},
		{Key: commandHelpHelpKeys, Purpose: "Context command help", NextInput: "PgUp and PgDn scroll; Space turns the page; Esc closes."},
		{Key: commandKeyLabelEscape, Purpose: "Game menu", NextInput: "Choose Help, Save, Load or Configure; Esc returns."},
		{Key: "D / W", Purpose: "Unused keys", Unavailable: "D and W do nothing."},
	}
	for index := range entries {
		entry := &entries[index]
		if entry.Key == commandHelpMarkKey && context != references.LargeMapType {
			entry.Unavailable = "Available on the surface overworld."
		}
		switch context {
		case references.CombatMapType:
			switch entry.Key {
			case "B", "E", "F", "H", "I", "L", "M", "N", "Q", "T", "V", "X", commandHelpMapKeys, commandHelpRunesKeys:
				entry.Unavailable = "Not available during combat."
			}
			switch entry.Key {
			case commandHelpMovementKeys:
				entry.NextInput = "Move one step. Hold to repeat, if Combat Key Repeat is on in Configure."
			case commandKeyLabelSpace:
				entry.NextInput = "Passes once per press. Holding the key does not repeat in battle."
			case "G", "J", "O", "P", "S":
				entry.NextInput = "Arrows or keypad choose the direction. Esc cancels and costs no turn; Space passes the action. " +
					"In solo mode during enemy turns, silently queue one G/O/J/P/S verb; another replaces it, Esc clears. " +
					"Early directions must be released and pressed afresh when ready."
			case "K":
				entry.NextInput = "Ordinary direction: Esc cancels and costs no turn; Space passes the action. Underfoot ladders and the U/D ladder selector use their displayed route."
			case "A":
				entry.NextInput = "Arrows/keypad aim; Enter or A attacks. Esc cancels before the first attack, " +
					"then skips the remaining weapon. Space skips at your own square and attacks elsewhere. " +
					"Combat Key Repeat set to Fast repeats A while held."
			case "C":
				entry.NextInput = "Choose a spell for the acting caster. Enter names the next step: " +
					"a spell with no aim is cast at once; any other is aimed first: " +
					"arrows turn or move the aim, and the shaded squares show who it reaches. " +
					"Casting spends MP and one mixture. " +
					"Cancelling costs nothing and preserves the combat action."
			case "R":
				entry.NextInput = "Choose equipment for the acting character; Enter applies the displayed Equip/Unready operation immediately."
			case commandHelpPartyKeys:
				entry.NextInput = "1-6 lets that member act alone while the others pass. 0 on the top row gives the whole party its turns back, at any time."
			case commandKeyLabelEscape:
				entry.Purpose, entry.NextInput = "Leave the battlefield", "Leave once the battle is won. In a dungeon room, choose the way out."
			}
		case references.DungeonMapType:
			switch entry.Key {
			case commandHelpMovementKeys:
				entry.NextInput = "Up advances, Down backs up, Left/Right turn. Enter/period turns around."
			case "G", "J", "O":
				entry.NextInput = "Acts on the current dungeon cell; choose a character when prompted."
				if entry.Key == "J" {
					entry.Purpose = "Disarm the current dungeon chest"
				}
			case "L", "S":
				entry.NextInput = "Choose a character, then Up: Ahead, Down: Here, Left/Right. Space or Esc: Pass."
			case "A":
				entry.NextInput = "Attacks a creature straight ahead."
			case "B", "E", "F", "T", "P", "X":
				entry.Unavailable = "Not used in dungeon corridors."
			}
		case references.SmallMapType:
			if entry.Key == "K" {
				entry.NextInput += " Esc cancels an uncommitted direction and costs no turn."
			}
			if entry.Key == commandHelpPartyKeys {
				entry.NextInput = "0 selects all; 1-6 selects a party position. At a harpsichord, number keys play notes instead."
			}
			if entry.Key == "E" {
				entry.Unavailable = "You are already inside. Walk off the edge of the map to leave."
			}
			if entry.Key == "F" {
				entry.NextInput = "Automatically finds an adjacent cannon."
			}
		case references.LargeMapType:
			if entry.Key == "A" {
				entry.NextInput = "Arrows or keypad choose the direction. Esc or Space cancels and costs no turn."
			}
			if entry.Key == "K" {
				entry.NextInput += " Esc cancels an uncommitted direction and costs no turn."
			}
			if entry.Key == commandHelpMovementKeys {
				entry.NextInput = "Move; hold to repeat. A frigate with raised sails continues automatically; Space stops sailing."
			}
			if entry.Key == "Y" {
				entry.NextInput = "On a frigate, raises or lowers the sails. Elsewhere, type the words and press Enter."
			}
		case references.CampingMapType:
			entry.Unavailable = "Answer the camp prompt first."
		}
	}
	if context == references.CombatMapType {
		entries = append(entries,
			widgets.CommandHelpEntry{Key: "Tab", Purpose: "Auto advance / attack", NextInput: "Moves toward an enemy, or attacks one in reach. Hold to repeat, if Combat Key Repeat is on."},
			widgets.CommandHelpEntry{Key: "F2", Purpose: "Combat details", NextInput: "Shows or hides the combat details."},
			widgets.CommandHelpEntry{Key: "F3", Purpose: "Noteworthy combat results", NextInput: "Jumps to the oldest find, pickup or trap you have not looked at; otherwise shows the latest. Costs no turn."},
			widgets.CommandHelpEntry{Key: commandHelpHistoryKeys, Purpose: "Combat message history", NextInput: "Scrolls back through the battle; End returns to now."},
		)
	}
	for index := range entries {
		entries[index].Group = commandHelpGroup(entries[index].Key)
	}

	return entries
}

// commandHelpGroup names the heading a key is listed under on Help's Right now page.
func commandHelpGroup(key string) string {
	switch key {
	case commandHelpMovementKeys, commandKeyLabelSpace, "A", "C", "Tab":
		return "MOVE AND ACT"
	case "G", "I", "M", "N", "R", "U", "Z", commandHelpPartyKeys:
		return "ITEMS AND PARTY"
	case "F2", "F3", commandHelpHistoryKeys:
		return "BATTLE PANELS"
	case commandHelpJournalKeys, commandHelpMapKeys, commandHelpRunesKeys, commandHelpHelpKeys, commandKeyLabelEscape,
		commandHelpMarkKey, "Ctrl+M", "Ctrl+Q", "Q", "F4":
		return "BOOKS AND SYSTEM"
	default:
		return "SURROUNDINGS"
	}
}
