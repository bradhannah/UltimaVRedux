package main

import (
	"slices"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func (m *IntroMenuScene) openAcknowledgments() {
	if m.dialogStack.PeekTopModalDialog() != nil {
		return
	}
	m.notice = ""
	// REDUX DEVIATION: INTRO-0002: original acknowledgements show the static STARTSC
	// credit panel until a keypress. Redux rolls a readable combined document
	// to include original, project and recording attributions with keyboard control.
	var dialog *widgets.CreditsDialog
	dialog = widgets.NewCreditsDialog(buildAcknowledgments(audio.RecordingCredits()), m.keyboard,
		func() int64 { return m.elapsedMs }, func() {
			m.dialogStack.MustRemoveWidgetInstance(dialog)
			m.keyboard.PrepareForTransition(0)
		})
	m.dialogStack.PushModalDialog(dialog)
}

//nolint:exhaustruct_v5 // Declarative paragraphs select a single heading/prose role; blank paragraphs provide spacing.
func buildAcknowledgments(recordings []audio.RecordingCredit) []text.CreditParagraph {
	document := text.OriginalAndReduxCredits()
	document = append(document, text.CreditParagraph{Heading: "Music & Sound Attribution"},
		text.CreditParagraph{Value: "These credits describe the supplied recordings. " +
			"Conversion and editing are credited separately."},
		text.CreditParagraph{})
	for _, music := range []bool{true, false} {
		heading := "Sound Effects"
		if music {
			heading = "Music"
		}
		document = append(document, text.CreditParagraph{Heading: heading})
		for _, group := range groupRecordingCredits(recordings, music) {
			document = appendRecordingAttribution(document, group)
		}
	}

	return append(document, text.SpecialThanksCredits()...)
}

type recordingCreditGroup struct {
	names  []string
	source audio.RecordingProvenance
}

func groupRecordingCredits(recordings []audio.RecordingCredit, music bool) []recordingCreditGroup {
	// Sort a detached copy: caller order and map enumeration cannot reorder credits.
	ordered := slices.Clone(recordings)
	slices.SortFunc(ordered, func(a, b audio.RecordingCredit) int { return strings.Compare(a.Name, b.Name) })
	var groups []recordingCreditGroup
	for _, recording := range ordered {
		if recording.Music != music {
			continue
		}
		index := slices.IndexFunc(groups, func(group recordingCreditGroup) bool {
			a, b := group.source, recording.Provenance

			return a.SourceType == b.SourceType && a.SourceCreator == b.SourceCreator &&
				a.SourceURL == b.SourceURL && a.License == b.License && slices.Equal(a.Processing, b.Processing)
		})
		if index < 0 {
			groups = append(groups, recordingCreditGroup{source: recording.Provenance, names: nil})
			index = len(groups) - 1
		}
		if !slices.Contains(groups[index].names, recording.Name) {
			groups[index].names = append(groups[index].names, recording.Name)
		}
	}

	return groups
}

//nolint:exhaustruct_v5 // Attribution paragraphs use prose or label/value roles, with no section heading.
func appendRecordingAttribution(document []text.CreditParagraph, group recordingCreditGroup) []text.CreditParagraph {
	const unknownCredit = "unknown"

	document = append(document, text.CreditParagraph{Label: "Recordings", Value: strings.Join(group.names, "; ")})
	source := group.source
	known := func(value string) bool { return value != "" && value != unknownCredit }
	if known(source.SourceCreator) {
		document = append(document, text.CreditParagraph{Label: "Source creator", Value: source.SourceCreator})
	}
	if known(source.SourceURL) {
		document = append(document, text.CreditParagraph{Label: "Source", Value: source.SourceURL})
	}
	switch {
	case known(source.License):
		// Supplied credit strings sometimes contain the only creator/work/URL.
		// Preserve them verbatim rather than parsing them into guessed fields.
		document = append(document, text.CreditParagraph{Label: "Supplied attribution / license", Value: source.License})
	case known(source.SourceCreator) || known(source.SourceURL):
		document = append(document, text.CreditParagraph{Label: "License", Value: "Not recorded"})
	default:
		document = append(document, text.CreditParagraph{Value: "Source attribution not recorded."})
	}
	for _, step := range source.Processing {
		document = append(document, text.CreditParagraph{Label: "Processing", Value: step.Description},
			text.CreditParagraph{Label: "Tool", Value: step.Tool},
			text.CreditParagraph{Label: "Processed by", Value: step.PerformedBy})
	}

	return append(document, text.CreditParagraph{})
}
