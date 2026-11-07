package main

import (
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
)

func (g *GameScene) handleCommandHelpShortcut(key ebiten.Key) bool {
	if key != ebiten.KeySlash && key != ebiten.KeyF1 || g.secondaryKeyState != PrimaryInput ||
		g.gameState.EventQueueManager.HasActiveWorkflows() {
		return false
	}
	g.showCommandHelp()

	return true
}

func (g *GameScene) showCommandHelp() {
	var dialog *widgets.CommandHelpDialog
	dialog = widgets.NewCommandHelpDialog(commandHelpEntries(g.gameState.MapState.PlayerLocation.Location.GetMapType()),
		g.currentHelpStatus(), g.keyboard, func() { g.dialogStack.MustRemoveWidgetInstance(dialog) })
	g.dialogStack.PushModalDialog(dialog)
}

func commandHelpEntries(context references.GeneralMapType) []widgets.CommandHelpEntry {
	direction := "Arrow/keypad direction; Space: Pass (not Esc)."
	if context == references.SmallMapType || context == references.LargeMapType {
		direction = "Arrow/keypad direction; Esc: Cancel without spending a turn. Space: Pass with the normal command cost."
	}
	entries := []widgets.CommandHelpEntry{
		{Key: commandHelpMovementKeys, Purpose: "Move", NextInput: "Move one step; hold to repeat."},
		{Key: commandKeyLabelSpace, Purpose: "Pass", NextInput: "Wait once immediately; hold to repeat in exploration. Release after a dialog or context change."},
		{Key: "A", Purpose: "Attack", NextInput: direction},
		{Key: "B", Purpose: "Board a vehicle", NextInput: "Stand on a boardable vehicle."},
		{Key: "C", Purpose: "Cast a prepared spell", NextInput: "Choose caster/spell, then the displayed target interaction. " +
			"Final confirmation spends MP and one mixture. Cancelling beforehand costs no MP, mixture, or gameplay time."},
		{Key: "E", Purpose: "Enter a location", NextInput: "Stand on a town, dungeon or other entrance."},
		{Key: "F", Purpose: "Fire a cannon", NextInput: direction},
		{Key: "G", Purpose: "Get an item", NextInput: direction},
		{Key: "H", Purpose: "Hole up and camp", NextInput: "Follow the camp prompts. During sleeping, press Space once to speed up the remaining rest. Each new camp starts normally."},
		{Key: "I", Purpose: "Ignite a torch", NextInput: "Requires a carried torch; consumes it immediately."},
		{Key: "J", Purpose: "Jimmy a lock", NextInput: direction + " Choose a character when asked."},
		{Key: "K", Purpose: "Klimb", NextInput: "Use a ladder or climbable terrain; choose direction when asked."},
		{Key: "L", Purpose: "Look", NextInput: direction},
		{Key: "M", Purpose: "Mix reagents", NextInput: "Choose a spell, then ingredients and quantity. Ctrl+F sets the maximum that fits for a confirmed recipe; Enter in preparation mixes."},
		{Key: "N", Purpose: "New party order", NextInput: "Select members to exchange; Avatar remains first."},
		{Key: "O", Purpose: "Open", NextInput: direction},
		{Key: "P", Purpose: "Push", NextInput: direction},
		{Key: "Q", Purpose: "Quick save", NextInput: "Save to the active player slot. Confirm Quick Save defaults On: answer Y/N, then acknowledge the result. Off saves immediately with a message; errors still need acknowledgement. Save or load a slot first."},
		{Key: "Ctrl+Q", Purpose: "Quick load", NextInput: "Reload the active player slot immediately, without confirmation or saving the current game. Works at normal player input, including controlled-monster combat turns. Unavailable chords are discarded; release before retrying. No Shift/Alt/Meta; Caps Lock is neutral."},
		{Key: "R", Purpose: "Ready equipment", NextInput: "Choose character/item; Enter applies the displayed Equip/Unready operation immediately. Space pages longer details."},
		{Key: "S", Purpose: commandLabelSearch, NextInput: direction},
		{Key: "T", Purpose: "Talk", NextInput: direction + " Type topics or phrases; Enter submits. NAME/JOB/BYE; PgUp reviews this conversation only."},
		{Key: "U", Purpose: "Use an item", NextInput: "Choose item/user, then follow its target prompt."},
		{Key: "V", Purpose: "View with a gem", NextInput: "Requires a gem; acknowledge the map view."},
		{Key: "X", Purpose: "Exit a vehicle", NextInput: "Must currently be riding or aboard."},
		{Key: "Y", Purpose: "Yell", NextInput: "Type words, then Enter."},
		{Key: "Z", Purpose: "Inspect party statistics", NextInput: "Navigate the dossier; Esc closes."},
		{Key: commandHelpPartyKeys, Purpose: "Active party member", NextInput: "0 selects all; 1-6 selects a party position."},
		{Key: commandHelpJournalKeys, Purpose: "Quest journal", NextInput: "Review discovered information; Esc closes."},
		{Key: commandHelpMapKeys, Purpose: "Traveller's map", NextInput: "Browse places, notes and your position; Esc closes. Time stops while it is open."},
		{Key: "Ctrl+N", Purpose: "Mark current position", NextInput: "On the surface overworld, open the map note editor at your current sextant fix. Requires a usable sextant; Esc returns to the map."},
		{Key: "F4", Purpose: "Last battle summary", NextInput: "Review collected totals from the last completed encounter in this session; Enter/Esc closes. No turn is spent."},
		{Key: "Ctrl+M", Purpose: "Toggle music", NextInput: "Toggle music from primary input."},
		{Key: commandHelpRunesKeys, Purpose: "Runic alphabet reference", NextInput: "Toggle the right-side chart during exploration, sign reading or conversation. Translate the runes yourself; no time is spent."},
		{Key: commandHelpHelpKeys, Purpose: "Context command help", NextInput: "PgUp/PgDn to read; Space: Next page; Esc closes. Available only at primary input."},
		{Key: commandKeyLabelEscape, Purpose: "Game menu", NextInput: "Choose Help, Save, Load or Configure; Esc returns."},
		{Key: "D / W", Purpose: "Unassigned command keys", Unavailable: "No player action is assigned to these keys."},
	}
	for index := range entries {
		entry := &entries[index]
		if entry.Key == "Ctrl+N" && context != references.LargeMapType {
			entry.Unavailable = "Available on the surface overworld."
		}
		switch context {
		case references.CombatMapType:
			switch entry.Key {
			case "B", "E", "F", "H", "I", "L", "M", "N", "Q", "T", "V", "X", commandHelpMapKeys, commandHelpRunesKeys:
				entry.Unavailable = "Not available during combat."
			}
			switch entry.Key {
			case commandKeyLabelSpace:
				entry.NextInput = "Pass one action per fresh press; holding does not repeat in combat."
			case "G", "J", "O", "P", "S":
				entry.NextInput = "Arrow/keypad direction; Esc: Cancel for free. Space: Pass spends the action. " +
					"In solo mode during enemy turns, silently queue one G/O/J/P/S verb; another replaces it, Esc clears. " +
					"Early directions must be released and pressed afresh when ready."
			case "K":
				entry.NextInput = "Ordinary direction: Esc cancels for free; Space passes the action. Underfoot ladders and the U/D ladder selector use their displayed route."
			case "A":
				entry.NextInput = "Arrows/keypad aim; Enter or A attacks. Escape cancels before the first attack, " +
					"then skips the remaining weapon. Space skips at your own square and attacks elsewhere."
			case "C":
				entry.NextInput = "Choose a spell for the acting caster, then its displayed target interaction. " +
					"Final confirmation spends MP and one mixture. " +
					"Cancelling beforehand costs no MP, mixture, or gameplay time and preserves the combat action."
			case "R":
				entry.NextInput = "Choose equipment for the acting character; Enter applies the displayed Equip/Unready operation immediately."
			case commandHelpPartyKeys:
				entry.NextInput = "Top-row 0 restores all-party control even during enemy or skipped turns and clears any queued verb. " +
					"The current action finishes normally; subsequent eligible turns follow DEX order. " +
					"At a ready turn, 1-6 selects another PC and passes the current actor until that PC can act."
			case commandKeyLabelEscape:
				entry.Purpose, entry.NextInput = "Leave the battlefield", "Exit when permitted, normally after victory. Dungeon rooms require a chosen exit route."
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
				entry.NextInput = "Attacks a wanderer directly ahead using the current facing."
			case "B", "E", "F", "T", "P", "X":
				entry.Unavailable = "No corresponding interaction in dungeon corridors."
			}
		case references.SmallMapType:
			if entry.Key == "K" {
				entry.NextInput += " Esc cancels an uncommitted direction without spending a turn."
			}
			if entry.Key == commandHelpPartyKeys {
				entry.NextInput = "0 selects all; 1-6 selects a party position. At a harpsichord, number keys play notes instead."
			}
			if entry.Key == "E" {
				entry.Unavailable = "Already inside a location; leave through its boundary."
			}
			if entry.Key == "F" {
				entry.NextInput = "Automatically finds an adjacent cannon."
			}
		case references.LargeMapType:
			if entry.Key == "A" {
				entry.NextInput = "Arrow/keypad direction; Esc: Cancel without spending a turn. Space also ends this prompt without spending a turn."
			}
			if entry.Key == "K" {
				entry.NextInput += " Esc cancels an uncommitted direction without spending a turn."
			}
			if entry.Key == commandHelpMovementKeys {
				entry.NextInput = "Move; hold to repeat. A frigate with raised sails continues automatically; Space stops sailing."
			}
			if entry.Key == "Y" {
				entry.NextInput = "On a frigate, toggles sails immediately. Otherwise type words and press Enter."
			}
		case references.CampingMapType:
			entry.Unavailable = "Camping owns input; follow the active camp prompt."
		}
	}
	if context == references.CombatMapType {
		entries = append(entries,
			widgets.CommandHelpEntry{Key: "Tab", Purpose: "Auto advance / attack", NextInput: "Acts toward a reachable enemy."},
			widgets.CommandHelpEntry{Key: "F2", Purpose: "Combat details", NextInput: "Toggle the combat detail panel."},
			widgets.CommandHelpEntry{Key: "F3", Purpose: "Noteworthy combat results", NextInput: "Jump to the oldest pending pickup, discovery, or trap; otherwise review the latest result. No turn is spent."},
			widgets.CommandHelpEntry{Key: "PgUp/PgDn/Home/End", Purpose: "Combat message history", NextInput: "Review older/newer events; End returns live."},
		)
	}

	return entries
}
