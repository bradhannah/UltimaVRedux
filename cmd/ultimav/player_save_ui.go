package main

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"
	"unicode"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

type playerSaveBrowserMode uint8

const (
	playerSaveBrowserLoad playerSaveBrowserMode = iota
	playerSaveBrowserWrite
	playerSaveHoursPerHalfDay = 12
	playerSavePartyLabel      = "Party"
)

type playerSaveBrowserModel struct {
	views                    []widgets.SaveSlotView
	summaries                map[player_save.SlotID]player_save.SlotSummary
	initialSelectionID       string
	newestLegend             string
	requireExplicitSelection bool
}

func buildPlayerSaveBrowserModel(
	summaries []player_save.SlotSummary,
	mode playerSaveBrowserMode,
	playthroughID string,
) playerSaveBrowserModel {
	model := playerSaveBrowserModel{
		views:                    make([]widgets.SaveSlotView, 0, len(summaries)),
		summaries:                make(map[player_save.SlotID]player_save.SlotSummary, len(summaries)),
		initialSelectionID:       "",
		newestLegend:             "",
		requireExplicitSelection: false,
	}
	newestID := newestReadyPlayerSaveSlot(summaries, mode, playthroughID)
	for _, summary := range summaries {
		model.summaries[summary.Slot] = summary
		view := playerSaveSlotView(summary, mode)
		view.Newest = view.ID == newestID
		model.views = append(model.views, view)
	}
	model.initialSelectionID = newestID
	model.newestLegend = "* Newest save for this playthrough"
	if mode == playerSaveBrowserLoad {
		model.newestLegend = "* Newest save overall"
		if model.initialSelectionID == "" && len(model.views) > 0 {
			model.initialSelectionID = model.views[0].ID
		}
	} else if newestID == "" {
		for _, slot := range player_save.AllSlotIDs() {
			if summary, found := model.summaries[slot]; found && summary.Status == player_save.SlotEmpty {
				model.initialSelectionID = string(slot)

				break
			}
		}
		model.requireExplicitSelection = model.initialSelectionID == ""
	}

	return model
}

func (model playerSaveBrowserModel) summary(view widgets.SaveSlotView) (player_save.SlotSummary, bool) {
	slot, err := player_save.ParseSlotID(view.ID)
	if err != nil {
		var summary player_save.SlotSummary

		return summary, false
	}
	summary, ok := model.summaries[slot]

	return summary, ok
}

func playerSaveSlotView(summary player_save.SlotSummary, mode playerSaveBrowserMode) widgets.SaveSlotView {
	primaryEnabled := summary.Status == player_save.SlotReady
	if mode == playerSaveBrowserWrite {
		primaryEnabled = summary.Status == player_save.SlotEmpty ||
			summary.Status == player_save.SlotReady ||
			summary.Status == player_save.SlotIncompatible ||
			(summary.Status == player_save.SlotCorrupt && summary.Revision != "")
	}

	if summary.Problem != "" {
		logging.Warnf(logging.CategoryGameState, "Inspect player save %s: %s", summary.Slot, summary.Problem)
	}
	var problem string
	switch summary.Status {
	case player_save.SlotCorrupt:
		if summary.Revision != "" {
			problem = "This save cannot be loaded. You can deliberately overwrite this slot when saving or importing. " +
				"Its existing data will be permanently replaced."

			break
		}
		problem = fmt.Sprintf(
			"This save is damaged and cannot be loaded or overwritten. Choose another slot. "+
				"To reclaim this slot, close the game, back up %s.json from the configured save directory, "+
				"then move that file out of the directory. Reopen the game to use the empty slot.",
			summary.Slot,
		)
	case player_save.SlotIncompatible:
		problem = "This save is not compatible with this game version or game data. " +
			"Use a compatible version to load it, or choose another slot."
	case player_save.SlotUnavailable:
		problem = "This save could not be read. Check access to the configured save directory, then reopen this screen."
	case player_save.SlotEmpty, player_save.SlotReady:
	}

	return widgets.SaveSlotView{
		ID:          string(summary.Slot),
		Label:       playerSaveSlotName(summary.Slot),
		Status:      playerSaveStatusLabel(summary.Status),
		RowSummary:  playerSaveRowSummary(summary),
		Description: summary.Metadata.UserDescription,
		HasSave:     summary.Status != player_save.SlotEmpty,
		ReviewValue: "",
		Newest:      false,
		NavigationDisabled: mode == playerSaveBrowserLoad &&
			summary.Status == player_save.SlotEmpty,
		Details:        playerSaveDetails(summary),
		Problem:        problem,
		PrimaryEnabled: primaryEnabled,
	}
}

func playerSaveFailureMessage(action string, err error) string {
	logging.Warnf(logging.CategoryGameState, "Player save %s failed: %v", action, err)
	if errors.Is(err, player_save.ErrStaleRevision) {
		return playerSaveErrorDetail(err)
	}

	return action + " failed. " + playerSaveErrorDetail(err)
}

func playerSaveErrorDetail(err error) string {
	switch {
	case errors.Is(err, player_save.ErrStaleRevision):
		return "This save changed since you opened the screen. Review the slot and try again."
	case errors.Is(err, ErrNoActivePlayerSaveSlot):
		return "Save or load a player slot first."
	case errors.Is(err, game_state.ErrSaveSnapshotNotRepresentable),
		errors.Is(err, ErrGameSceneSnapshotNotRepresentable):
		return "Saving is not available right now. Finish the current action and try again."
	case errors.Is(err, ErrPlayerSaveSceneIneligible):
		return "The game could not complete this operation. Check the game log for details."
	case errors.Is(err, player_save.ErrSlotNotFound), errors.Is(err, fs.ErrNotExist):
		return "The save could not be found. Reopen the save screen and choose another slot."
	case errors.Is(err, player_save.ErrIncompatible), errors.Is(err, save_snapshot.ErrUnsupportedSnapshotVersion):
		return "This save needs a compatible game version or game data. Choose another save."
	case errors.Is(err, save_snapshot.ErrInvalidEnvelope), errors.Is(err, save_snapshot.ErrInvalidSnapshot):
		return "The save data could not be validated. Check the game log for details."
	case errors.Is(err, fs.ErrPermission):
		return "Check file permissions for the configured save directory and try again."
	case errors.Is(err, ErrPlayerSaveUnavailable):
		return "Save storage is unavailable. Check the configured save directory and try again."
	default:
		return "Please try again. If the problem continues, check the game log for details."
	}
}

func playerQuickLoadErrorDetail(err error) string {
	switch {
	case errors.Is(err, ErrNoActivePlayerSaveSlot):
		return "Save or load a player slot first."
	case errors.Is(err, player_save.ErrStaleRevision):
		return "The active save changed. Use Load to review the slot before loading it."
	case errors.Is(err, player_save.ErrSlotNotFound), errors.Is(err, fs.ErrNotExist):
		return "The active save could not be found. Use Load to review your slots."
	case errors.Is(err, ErrPlayerSaveUnavailable):
		return "Load services are unavailable. Check the configured save directory and try again."
	case errors.Is(err, player_save.ErrIncompatible), errors.Is(err, save_snapshot.ErrUnsupportedSnapshotVersion):
		return "The active save needs compatible game data or a compatible game version. Use Load to review your slots."
	case errors.Is(err, save_snapshot.ErrInvalidEnvelope), errors.Is(err, save_snapshot.ErrInvalidSnapshot):
		return "The active save could not be validated. Use Load to review your slots."
	default:
		return playerSaveErrorDetail(err)
	}
}

func playerSaveRowSummary(summary player_save.SlotSummary) string {
	if summary.Status == player_save.SlotEmpty {
		return ""
	}
	if summary.Status == player_save.SlotCorrupt || summary.Status == player_save.SlotUnavailable {
		return playerSaveStatusLabel(summary.Status)
	}

	if avatarName := strings.TrimSpace(summary.Metadata.AvatarName); avatarName != "" {
		return avatarName
	}

	return playerSaveStatusLabel(summary.Status)
}

func playerSaveStatusLabel(status player_save.SlotStatus) string {
	label := strings.ReplaceAll(string(status), "_", " ")
	if label == "" {
		return "Unknown"
	}

	return strings.ToUpper(label[:1]) + label[1:]
}

func playerSaveDetails(summary player_save.SlotSummary) []widgets.SaveSlotDetail {
	if summary.Status == player_save.SlotEmpty {
		return []widgets.SaveSlotDetail{{Label: "", Value: "No save data."}}
	}
	if summary.Status == player_save.SlotCorrupt || summary.Status == player_save.SlotUnavailable {
		return []widgets.SaveSlotDetail{{Label: "", Value: "No readable save metadata."}}
	}

	metadata := summary.Metadata
	var details []widgets.SaveSlotDetail
	if metadata.AvatarName != "" {
		details = append(details, widgets.SaveSlotDetail{Label: "Avatar", Value: metadata.AvatarName})
	}
	details = append(details,
		widgets.SaveSlotDetail{Label: "Location", Value: playerSaveLocationLabel(metadata.Location, metadata.Floor)},
		widgets.SaveSlotDetail{Label: playerSavePartyLabel, Value: fmt.Sprintf("%d members", metadata.ActivePartyCount)},
	)
	calendar := metadata.Calendar
	if calendar.Year > 0 && calendar.Month > 0 && calendar.Day > 0 {
		details = append(details, widgets.SaveSlotDetail{
			Label: "Britannian",
			Value: playerSaveCalendarLabel(
				calendar.Year,
				calendar.Month,
				calendar.Day,
				calendar.Hour,
				calendar.Minute,
			),
		})
	}
	if !metadata.CreatedAt.IsZero() {
		details = append(details, widgets.SaveSlotDetail{
			Label: "Saved",
			Value: playerSaveLocalTime(metadata.CreatedAt),
		})
	}

	return details
}

func playerSaveCalendarLabel(year, month, day, hour, minute int) string {
	suffix := "AM"
	displayHour := hour
	if displayHour >= playerSaveHoursPerHalfDay {
		displayHour -= playerSaveHoursPerHalfDay
		suffix = "PM"
	}
	if displayHour == 0 {
		displayHour = 12
	}

	return fmt.Sprintf("%d-%d-%d, %d:%02d%s", month, day, year, displayHour, minute, suffix)
}

func newestReadyPlayerSaveSlot(
	summaries []player_save.SlotSummary, mode playerSaveBrowserMode, playthroughID string,
) string {
	var newest *player_save.SlotSummary
	for index := range summaries {
		summary := &summaries[index]
		if summary.Status != player_save.SlotReady || !save_snapshot.ValidPlaythroughID(summary.Metadata.PlaythroughID) ||
			(mode == playerSaveBrowserWrite && summary.Metadata.PlaythroughID != playthroughID) {
			continue
		}
		if newest == nil || summary.Metadata.CreatedAt.After(newest.Metadata.CreatedAt) ||
			(summary.Metadata.CreatedAt.Equal(newest.Metadata.CreatedAt) && summary.Slot < newest.Slot) {
			newest = summary
		}
	}
	if newest == nil {
		return ""
	}

	return string(newest.Slot)
}

//nolint:mnd // Decimal ordinal suffix rules use literal final digits and the 11-13 exception.
func playerSaveLocationLabel(locationID, floor int) string {
	if locationID == int(references.BritanniaUnderworld) {
		if floor == int(references.UNDERWORLD) {
			return "Underworld"
		}

		return "Britannia"
	}
	if locationID < 0 || locationID > 255 {
		return fmt.Sprintf("Location %d, floor %d", locationID, floor)
	}

	name := humanizePlayerSaveLocation(references.Location(locationID).String())
	if references.Location(locationID).GetMapType() == references.DungeonMapType {
		return fmt.Sprintf("%s, level %d", name, floor+1)
	}
	var label string
	switch {
	case floor < -1:
		label = fmt.Sprintf("basement %d", -floor)
	case floor == -1:
		label = "basement"
	case floor == 0:
		label = "main"
	default:
		suffix := "th"
		if floor%100 < 11 || floor%100 > 13 {
			switch floor % 10 {
			case 1:
				suffix = "st"
			case 2:
				suffix = "nd"
			case 3:
				suffix = "rd"
			}
		}
		label = fmt.Sprintf("%d%s", floor, suffix)
	}

	return name + ", " + label
}

//nolint:gosmopolitan // Player save timestamps deliberately use the machine's local timezone for display only.
func playerSaveLocalTime(saved time.Time) string {
	return saved.Local().Format("Jan 2, 2006, 3:04 PM MST")
}

type playerSaveDescriptionDraft struct {
	slot player_save.SlotID
	text string
	set  bool
}

func playerSaveDescriptionInitial(
	summary player_save.SlotSummary, playthroughID string, draft *playerSaveDescriptionDraft,
) string {
	if draft.set && draft.slot == summary.Slot {
		return draft.text
	}
	if playthroughID != "" && summary.Metadata.PlaythroughID == playthroughID {
		// Older saves allowed longer notes. Offer the part that fits rather than an
		// empty box, which Enter would save over the note.
		runes := []rune(summary.Metadata.UserDescription)

		return strings.TrimSpace(string(runes[:min(len(runes), widgets.SaveSlotRowDescriptionMaxRunes)]))
	}

	return ""
}

func playerSaveDescriptionNotice(summary player_save.SlotSummary, playthroughID string) string {
	if summary.Status == player_save.SlotCorrupt {
		return "Replaces unreadable save data in " + playerSaveSlotName(summary.Slot) +
			". Its existing data will be permanently replaced."
	}
	if summary.Status == player_save.SlotEmpty {
		return "Save this checkpoint to " + playerSaveSlotName(summary.Slot) + "."
	}
	notice := "Replaces " + summary.Metadata.AvatarName + "'s save"
	if !summary.Metadata.CreatedAt.IsZero() {
		notice += " from " + playerSaveLocalTime(summary.Metadata.CreatedAt)
	}
	notice += "."
	if summary.Metadata.PlaythroughID != playthroughID || playthroughID == "" {
		notice += " This slot belongs to another playthrough."
	}

	return notice
}

func validatePlayerSaveDescriptionInput(value string) string {
	// Reject control input before trimming, including a pasted terminal newline.
	if err := save_snapshot.ValidateUserDescription(value); err != nil {
		return err.Error()
	}

	return ""
}

func showPlayerSaveDescriptionEditor(
	stack *widgets.DialogStack, keyboard *input.Keyboard, title string,
	summary player_save.SlotSummary, playthroughID string, draft *playerSaveDescriptionDraft,
	onSubmit func(string),
) {
	var editor *widgets.SaveDescriptionEntryModal
	editor = widgets.NewSaveDescriptionEntryModal(widgets.SaveDescriptionEntryModalConfig{
		Title:       title + " " + playerSaveSlotName(summary.Slot),
		Prompt:      "",
		Notice:      playerSaveDescriptionNotice(summary, playthroughID),
		InitialText: playerSaveDescriptionInitial(summary, playthroughID, draft),
		Keyboard:    keyboard, Validate: validatePlayerSaveDescriptionInput,
		MaxRunes:     widgets.SaveSlotRowDescriptionMaxRunes,
		IsKeyPressed: nil,
		OnResult: func(value string, submitted bool) {
			stack.MustRemoveWidgetInstance(editor)
			if !submitted {
				return
			}
			value = strings.TrimSpace(value)
			*draft = playerSaveDescriptionDraft{slot: summary.Slot, text: value, set: true}
			onSubmit(value)
		},
	})
	stack.PushModalDialog(editor)
}

func humanizePlayerSaveLocation(value string) string {
	var result strings.Builder
	for index, current := range value {
		if index > 0 && unicode.IsUpper(current) {
			result.WriteByte(' ')
		}
		result.WriteRune(current)
	}

	return strings.NewReplacer(
		"Britishs", "British's",
		"Iolos", "Iolo's",
		"Suteks", "Sutek's",
		"Vraals", "Vraal's",
		"Grendels", "Grendel's",
		"Buccaneers", "Buccaneer's",
		"Serpents", "Serpent's",
	).Replace(result.String())
}
