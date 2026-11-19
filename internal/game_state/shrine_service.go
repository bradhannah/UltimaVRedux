package game_state

import (
	"errors"
	"fmt"
	"hash"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

var (
	// ErrShrineServiceInvalid means the requested shrine progression is unavailable.
	ErrShrineServiceInvalid = errors.New("invalid shrine service request")
	// ErrShrineServiceStale means shrine, party, or resource state changed after review.
	ErrShrineServiceStale = errors.New("shrine service state changed after review")
	// ErrCodexServiceInvalid means the requested Codex progression is unavailable.
	ErrCodexServiceInvalid = errors.New("invalid Codex service request")
	// ErrCodexServiceStale means quest progression changed after Codex review.
	ErrCodexServiceStale = errors.New("codex quest state changed after review")
)

// ShrineMeditationReview is a detached progression snapshot for one shrine.
type ShrineMeditationReview struct {
	Fingerprint string
	ShrineIndex int
	QuestState  int
	VirtueName  string
	QuestText   string
	Gold        uint16
}

// ShrineQuestOutcome describes an assigned or completed shrine quest for presentation.
type ShrineQuestOutcome struct {
	ShrineIndex     int
	VirtueName      string
	QuestText       string
	Assigned        bool
	Completed       bool
	StatBonuses     []string
	KarmaAdded      int
	PrimaryMessage  string
	FollowupMessage string
	// Experience is the deed experience this completion paid the party.
	Experience DeedExperienceAward
}

// ShrineOfferingReview is a detached quote for a submitted offering digit.
type ShrineOfferingReview struct {
	Fingerprint string
	ShrineIndex int
	Digit       int
	Cost        uint16
	Gold        uint16
	Affordable  bool
	Message     string
}

// ShrineOfferingOutcome describes a committed offering for presentation.
type ShrineOfferingOutcome struct {
	ShrineIndex int
	Digit       int
	Cost        uint16
	GoldBefore  uint16
	GoldAfter   uint16
	KarmaAdded  int
	Message     string
}

// CodexQuestReview is a detached view of the next Codex progression step.
type CodexQuestReview struct {
	Fingerprint       string
	AllQuestsComplete bool
	ActiveQuest       int
	AnswerText        string
	EmptyMessage      string
}

// CodexQuestOutcome reports the committed answer and resulting completion state.
type CodexQuestOutcome struct {
	QuestIndex        int
	AnswerText        string
	AllQuestsComplete bool
	Heading           string
	// Experience is the deed experience this answer paid the party.
	Experience DeedExperienceAward
}

// CodexMemorial is the roster-ordered source presentation for Blackthorn's victims.
type CodexMemorial struct {
	Names        []string
	UrnPositions []references.Position
	Text         string
}

// ReviewCodexMemorial scans original roster slots 1 through 15 and lays out
// the source-defined urn actors.
func (g *GameState) ReviewCodexMemorial() CodexMemorial {
	names := g.PartyState.MartyrNames()
	result := CodexMemorial{Names: make([]string, len(names))}
	for index, name := range names {
		result.Names[index] = strings.ToUpper(name)
	}
	if len(names) == 0 {
		return result
	}

	visibleUrns := min(len(names), 8)
	result.UrnPositions = codexUrnPositions(visibleUrns)
	heading := "urns marked:"
	if len(names) == 1 {
		heading = "an urn marked:"
	}
	result.Text = "Thou dost see\n" + heading + "\n\n" + strings.Join(result.Names, "\n")

	return result
}

func codexUrnPositions(count int) []references.Position {
	positions := [8][]references.Position{
		{{X: 5, Y: 1}},
		{{X: 4, Y: 1}, {X: 6, Y: 1}},
		{{X: 4, Y: 1}, {X: 5, Y: 1}, {X: 6, Y: 1}},
		{{X: 3, Y: 1}, {X: 4, Y: 1}, {X: 6, Y: 1}, {X: 7, Y: 1}},
		{{X: 3, Y: 1}, {X: 4, Y: 1}, {X: 5, Y: 1}, {X: 6, Y: 1}, {X: 7, Y: 1}},
		{{X: 1, Y: 2}, {X: 1, Y: 3}, {X: 1, Y: 4}, {X: 9, Y: 2}, {X: 9, Y: 3}, {X: 9, Y: 4}},
		{{X: 1, Y: 2}, {X: 1, Y: 3}, {X: 1, Y: 4}, {X: 9, Y: 2}, {X: 9, Y: 3}, {X: 9, Y: 4}, {X: 5, Y: 1}},
		{{X: 1, Y: 2}, {X: 1, Y: 3}, {X: 1, Y: 4}, {X: 1, Y: 5}, {X: 9, Y: 2}, {X: 9, Y: 3}, {X: 9, Y: 4}, {X: 9, Y: 5}},
	}
	if count < 1 || count > len(positions) {
		return nil
	}

	return append([]references.Position(nil), positions[count-1]...)
}

// ShrineVisitOutcome reports authoritative map restoration and elapsed time.
type ShrineVisitOutcome struct {
	LocationBefore references.PlayerLocation
	LocationAfter  references.PlayerLocation
	DateTimeBefore datetime.UltimaDate
	DateTimeAfter  datetime.UltimaDate
	ElapsedMinutes int
}

// ReviewShrineMeditation captures the exact progression and reward inputs for one shrine.
func (g *GameState) ReviewShrineMeditation(shrineIndex int) (ShrineMeditationReview, error) {
	data := GetShrineData(shrineIndex)
	if data == nil {
		return ShrineMeditationReview{}, fmt.Errorf("%w: shrine index %d", ErrShrineServiceInvalid, shrineIndex)
	}
	if !g.IsInShrine() || g.ShrineScene == nil || g.ShrineScene.ShrineIndex != shrineIndex {
		return ShrineMeditationReview{}, fmt.Errorf("%w: shrine meditation is not active", ErrShrineServiceInvalid)
	}
	if g.PartyState.GetAvatar() == nil {
		return ShrineMeditationReview{}, fmt.Errorf("%w: Avatar is unavailable", ErrShrineServiceInvalid)
	}

	review := ShrineMeditationReview{
		ShrineIndex: shrineIndex,
		QuestState:  g.GetShrineQuestState(shrineIndex),
		VirtueName:  data.VirtueName,
		QuestText:   data.QuestText(),
		Gold:        g.PartyState.Inventory.Gold.Get(),
	}
	review.Fingerprint = g.shrineMeditationFingerprint(review)

	return review, nil
}

// CommitShrineQuestAssignment revalidates and activates the reviewed shrine quest.
func (g *GameState) CommitShrineQuestAssignment(
	shrineIndex int,
	expectedFingerprint string,
) (ShrineQuestOutcome, error) {
	review, err := g.validateShrineMeditationReview(shrineIndex, expectedFingerprint)
	if err != nil {
		return ShrineQuestOutcome{}, err
	}
	if review.QuestState != QuestStateNotAssigned {
		return ShrineQuestOutcome{}, fmt.Errorf("%w: quest is not assignable", ErrShrineServiceInvalid)
	}

	g.SetShrineQuestActive(shrineIndex, true)

	return ShrineQuestOutcome{
		ShrineIndex:     shrineIndex,
		VirtueName:      review.VirtueName,
		QuestText:       review.QuestText,
		Assigned:        true,
		Completed:       false,
		StatBonuses:     nil,
		KarmaAdded:      0,
		PrimaryMessage:  "The Altar speaks and a Quest is ordained!",
		FollowupMessage: "Return again when thy Quest is done!",
	}, nil
}

// CommitShrineQuestCompletion atomically applies quest flags, Avatar rewards, and karma.
func (g *GameState) CommitShrineQuestCompletion(
	shrineIndex int,
	expectedFingerprint string,
) (ShrineQuestOutcome, error) {
	review, err := g.validateShrineMeditationReview(shrineIndex, expectedFingerprint)
	if err != nil {
		return ShrineQuestOutcome{}, err
	}
	if review.QuestState != QuestStateReadyToComplete {
		return ShrineQuestOutcome{}, fmt.Errorf("%w: quest is not ready to complete", ErrShrineServiceInvalid)
	}

	data := GetShrineData(shrineIndex)
	avatar := g.PartyState.GetAvatar()
	bonuses := make([]string, 0, 3)
	// REDUX DEVIATION: KARMA-0009: original completion interleaves karma and
	// stats. Redux commits stats then one combined karma award, preserving
	// final values and a single coherent reward transaction.
	g.SetShrineQuestActive(shrineIndex, false)
	if data.StrBonus > 0 {
		avatar.Strength = byte(min(int(avatar.Strength)+int(data.StrBonus), MaxStatValue))
		bonuses = append(bonuses, "Strength +1")
	}
	if data.DexBonus > 0 {
		avatar.Dexterity = byte(min(int(avatar.Dexterity)+int(data.DexBonus), MaxStatValue))
		bonuses = append(bonuses, "Dexterity +1")
	}
	if data.IntBonus > 0 {
		avatar.Intelligence = byte(min(int(avatar.Intelligence)+int(data.IntBonus), MaxStatValue))
		bonuses = append(bonuses, "Intelligence +1")
	}
	karma := g.AdjustPartyKarma(data.KarmaBonus())

	return ShrineQuestOutcome{
		ShrineIndex:     shrineIndex,
		VirtueName:      review.VirtueName,
		QuestText:       review.QuestText,
		Assigned:        false,
		Completed:       true,
		StatBonuses:     bonuses,
		KarmaAdded:      karma.After - karma.Before,
		PrimaryMessage:  "WELL DONE!",
		FollowupMessage: "",
		Experience:      g.settleDeedExperience(false),
	}, nil
}

// ReviewShrineOffering quotes one original digit-times-100 offering.
func (g *GameState) ReviewShrineOffering(shrineIndex, digit int) (ShrineOfferingReview, error) {
	if digit < 1 || digit > 9 {
		return ShrineOfferingReview{}, fmt.Errorf("%w: offering digit must be 1 through 9", ErrShrineServiceInvalid)
	}
	meditation, err := g.ReviewShrineMeditation(shrineIndex)
	if err != nil {
		return ShrineOfferingReview{}, err
	}
	if meditation.QuestState != QuestStateOffering {
		return ShrineOfferingReview{}, fmt.Errorf("%w: shrine is not accepting offerings", ErrShrineServiceInvalid)
	}

	cost := uint16(digit * 100)
	review := ShrineOfferingReview{
		Fingerprint: meditation.Fingerprint,
		ShrineIndex: shrineIndex,
		Digit:       digit,
		Cost:        cost,
		Gold:        meditation.Gold,
		Affordable:  meditation.Gold >= cost,
	}
	if !review.Affordable {
		review.Message = "Thou hast not that much gold!"
	}

	return review, nil
}

// CommitShrineOffering revalidates and atomically applies payment and karma.
func (g *GameState) CommitShrineOffering(
	review ShrineOfferingReview,
) (ShrineOfferingOutcome, error) {
	current, err := g.ReviewShrineOffering(review.ShrineIndex, review.Digit)
	if err != nil {
		return ShrineOfferingOutcome{}, err
	}
	if review.Fingerprint == "" || review.Fingerprint != current.Fingerprint ||
		review.Cost != current.Cost || review.Gold != current.Gold {
		return ShrineOfferingOutcome{}, ErrShrineServiceStale
	}
	if !current.Affordable {
		return ShrineOfferingOutcome{Message: current.Message}, fmt.Errorf(
			"%w: offering is unaffordable",
			ErrShrineServiceInvalid,
		)
	}

	g.PartyState.Inventory.Gold.DecrementBy(current.Cost)
	karma := g.AdjustPartyKarma(current.Digit)

	return ShrineOfferingOutcome{
		ShrineIndex: current.ShrineIndex,
		Digit:       current.Digit,
		Cost:        current.Cost,
		GoldBefore:  current.Gold,
		GoldAfter:   g.PartyState.Inventory.Gold.Get(),
		KarmaAdded:  karma.After - karma.Before,
		Message:     "ALAKAZAM!",
	}, nil
}

// ReviewCodexQuest captures all quest flags and the first active answer.
func (g *GameState) ReviewCodexQuest() (CodexQuestReview, error) {
	if !g.IsInCodex() || g.ShrineScene == nil {
		return CodexQuestReview{}, fmt.Errorf("%w: Codex reading is not active", ErrCodexServiceInvalid)
	}

	review := CodexQuestReview{
		AllQuestsComplete: g.AreAllQuestsComplete(),
		ActiveQuest:       g.GetFirstActiveQuest(),
		EmptyMessage:      "The pages are blank!",
	}
	if review.ActiveQuest >= 0 {
		review.AnswerText = GetShrineData(review.ActiveQuest).AnswerText()
	}
	review.Fingerprint = g.codexQuestFingerprint()

	return review, nil
}

// CommitCodexQuestAnswer revalidates and marks the reviewed active quest complete.
func (g *GameState) CommitCodexQuestAnswer(review CodexQuestReview) (CodexQuestOutcome, error) {
	current, err := g.ReviewCodexQuest()
	if err != nil {
		return CodexQuestOutcome{}, err
	}
	if review.Fingerprint == "" || review.Fingerprint != current.Fingerprint ||
		review.ActiveQuest != current.ActiveQuest || review.AnswerText != current.AnswerText {
		return CodexQuestOutcome{}, ErrCodexServiceStale
	}
	if current.AllQuestsComplete || current.ActiveQuest < 0 {
		return CodexQuestOutcome{}, fmt.Errorf("%w: no active quest answer is available", ErrCodexServiceInvalid)
	}

	g.SetShrineQuestCompleted(current.ActiveQuest, true)
	key, _ := ShrineMantraKnowledgeKey(current.ActiveQuest)
	g.appendLiveJournalEvent(QuestJournalEvent{
		Kind:       QuestJournalEventCodexAnswerLearned,
		Subject:    QuestJournalSubject(strings.TrimSuffix(key.ID, ".mantra")),
		OccurredOn: currentKnowledgeDate(g.DateTime),
		Source:     QuestJournalEventSource(KnowledgeSourceRevelation), SourceName: "Codex",
	})

	return CodexQuestOutcome{
		QuestIndex:        current.ActiveQuest,
		AnswerText:        current.AnswerText,
		AllQuestsComplete: g.AreAllQuestsComplete(),
		Heading:           "Thou dost read:",
		Experience:        g.settleDeedExperience(false),
	}, nil
}

// FinishShrineVisit restores the world location and applies the visit's time cost.
func (g *GameState) FinishShrineVisit() (ShrineVisitOutcome, error) {
	outcome := ShrineVisitOutcome{
		LocationBefore: g.MapState.PlayerLocation,
		LocationAfter:  g.MapState.PlayerLocation,
		DateTimeBefore: g.DateTime,
		DateTimeAfter:  g.DateTime,
		ElapsedMinutes: 0,
	}
	if !g.IsInMiniMap() || g.ShrineScene == nil {
		return outcome, fmt.Errorf("%w: shrine or Codex visit is not active", ErrShrineServiceInvalid)
	}

	g.TransitionFromShrineMap()
	outcome.ElapsedMinutes = g.AdvanceGameplayMinutesAndProcessProvisions(ShrineTimeCostMinutes)
	outcome.LocationAfter = g.MapState.PlayerLocation
	outcome.DateTimeAfter = g.DateTime

	return outcome, nil
}

func (g *GameState) validateShrineMeditationReview(
	shrineIndex int,
	expectedFingerprint string,
) (ShrineMeditationReview, error) {
	review, err := g.ReviewShrineMeditation(shrineIndex)
	if err != nil {
		return ShrineMeditationReview{}, err
	}
	if expectedFingerprint == "" || expectedFingerprint != review.Fingerprint {
		return ShrineMeditationReview{}, ErrShrineServiceStale
	}

	return review, nil
}

func (g *GameState) shrineMeditationFingerprint(review ShrineMeditationReview) string {
	progress := g.ShrineProgress(review.ShrineIndex)
	avatar := g.PartyState.GetAvatar()

	return reviewedMutationFingerprint(func(digest hash.Hash) {
		writeFingerprintf(
			digest,
			"%d:%t:%t:%d:%d:%d:%d:%d",
			review.ShrineIndex,
			progress.Completed,
			progress.Active,
			avatar.Strength,
			avatar.Dexterity,
			avatar.Intelligence,
			g.PartyState.Karma.Value,
			review.Gold,
		)
	})
}

func (g *GameState) codexQuestFingerprint() string {
	return reviewedMutationFingerprint(func(digest hash.Hash) {
		for index := range TotalShrines {
			progress := g.ShrineProgress(index)
			writeFingerprintf(digest, "%d:%t:%t;", index, progress.Completed, progress.Active)
		}
	})
}
