package main

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// Experience is light magenta wherever it is shown, and a level that is earned
// but not yet granted is yellow. Both say what they mean in words as well.

// experienceShareText names an amount paid to everyone, or to a lone Avatar.
func experienceShareText(each, recipients int) string {
	if recipients == 1 {
		return fmt.Sprintf("+%d", each)
	}

	return fmt.Sprintf("+%d each", each)
}

// experienceAwardText is the message-window line for a deed award.
func experienceAwardText(award game_state.DeedExperienceAward) string {
	text := fmt.Sprintf("+%d experience", award.Each)
	if award.Recipients != 1 {
		text += " each"
	}
	if award.BackPay {
		text += " for past deeds"
	}

	return text
}

// experienceDialogText is the same award phrased like a dialog's reward lines.
func experienceDialogText(award game_state.DeedExperienceAward) string {
	return "Experience " + experienceShareText(award.Each, award.Recipients)
}

func experienceReadyText(ready game_state.ExperienceLevelReady) string {
	return fmt.Sprintf("%s is ready for level %d.", ready.Name, ready.Level)
}

// showExperienceAward writes a deed award to the message window. A deed that
// has no dialog of its own also raises the usual action popup.
func (g *GameScene) showExperienceAward(award game_state.DeedExperienceAward, popup bool) {
	if !award.Paid() {
		return
	}
	g.output.AppendLine().Text(experienceAwardText(award), ucolor.ExperienceGain).Done()
	for _, ready := range award.Ready {
		g.output.AppendLine().Text(experienceReadyText(ready), ucolor.ExperienceLevelReady).Done()
	}
	if popup {
		g.ShowActionFeedback(experienceAwardText(award), false)
	}
}

// experienceDialogLines are the lines an award adds to a dialog that already
// reports the deed. No award adds none.
func experienceDialogLines(award game_state.DeedExperienceAward) []widgets.StyledDialogLine {
	if !award.Paid() {
		return nil
	}
	lines := []widgets.StyledDialogLine{{Text: experienceDialogText(award), Color: ucolor.ExperienceGain}}
	for _, ready := range award.Ready {
		lines = append(lines, widgets.StyledDialogLine{Text: experienceReadyText(ready), Color: ucolor.ExperienceLevelReady})
	}

	return lines
}

// settlePastDeedExperience pays deeds finished before the setting could pay for
// them: once when a game begins or loads, and again when the setting changes.
func (g *GameScene) settlePastDeedExperience() {
	tier := g.gameConfig.GetDeedExperience()
	if g.deedExperienceSettled && tier == g.deedExperienceTier {
		return
	}
	g.deedExperienceSettled, g.deedExperienceTier = true, tier
	g.showExperienceAward(g.gameState.SettlePastDeedExperience(), true)
}
