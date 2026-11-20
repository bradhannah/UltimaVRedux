package conversation

import (
	"fmt"
	"strings"
	"testing"

	conversationpatchdata "github.com/bradhannah/UltimaVRedux/data/conversation-patches"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/catalog"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/test/originaldata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These exercise the production, patched semantic programs rather than the
// retired compatibility interpreter. Actor slots deliberately differ from dialogs.
func TestOriginalSemanticAvaOfferingRecognitionAndDonation(t *testing.T) {
	originalCatalog, err := catalog.LoadOriginal(originaldata.Directory(t), conversationpatchdata.Files)
	require.NoError(t, err)
	definition := originalCatalogDefinition(t, originalCatalog, semantic.OriginalSourceCastle, 31)
	t.Run("unrecognized name does not mark the actor met", func(t *testing.T) {
		callbacks := &semanticTestCallbacks{avatarName: "Avatar"}
		engine, engineErr := NewSemanticConversationEngine(definition, callbacks, false)
		require.NoError(t, engineErr)
		engine.Start(73)
		engine.ProcessInput("virt")
		require.Contains(t, engine.ProcessInput("y").Output, promptName)
		response := engine.ProcessInput("Stranger")
		require.NoError(t, response.Error)
		assert.Contains(t, response.Output, responseIfYouSay)
		assert.Empty(t, callbacks.metActorIDs)
		assert.False(t, callbacks.hasMet)
	})
	for _, known := range []bool{false, true} {
		for _, accept := range []bool{false, true} {
			t.Run(fmt.Sprintf("known=%t/accept=%t", known, accept), func(t *testing.T) {
				callbacks := &semanticTestCallbacks{avatarName: "Avatar", hasMet: known}
				engine, engineErr := NewSemanticConversationEngine(definition, callbacks, false)
				require.NoError(t, engineErr)
				start := engine.Start(73)
				require.NoError(t, start.Error)
				assert.Contains(t, engine.ProcessInput("virt").Output, "offering")
				question := engine.ProcessInput("y")
				if !known {
					require.Contains(t, question.Output, promptName)
					question = engine.ProcessInput("Avatar")
					assert.Equal(t, []int{73}, callbacks.metActorIDs)
				}
				require.Contains(t, question.Output, "5 gold crowns")
				assert.NotContains(t, question.Output, promptName)
				answer := "n"
				if accept {
					answer = "y"
				}
				response := engine.ProcessInput(answer)
				require.NoError(t, response.Error)
				assert.NotContains(t, response.Output, responseIfYouSay)
				assert.NotContains(t, response.Output, "005")
				if accept {
					assert.Equal(t, []int{5}, callbacks.gold)
					// The imported first-meeting donation branch grants five karma;
					// the recognized-party branch does not repeat that grant.
					if known {
						assert.Zero(t, callbacks.karmaUp)
					} else {
						assert.Equal(t, 5, callbacks.karmaUp)
					}
				} else {
					assert.Empty(t, callbacks.gold)
					assert.Zero(t, callbacks.karmaUp)
				}
				// A new session retains recognition through the authoritative callback.
				engine.Stop()
				next, nextErr := NewSemanticConversationEngine(definition, callbacks, false)
				require.NoError(t, nextErr)
				assert.Contains(t, next.Start(73).Output, "Avatar")
			})
		}
	}
}

func TestOriginalSemanticChucklesSequentialWaitsAndRoutine(t *testing.T) {
	originalCatalog, err := catalog.LoadOriginal(originaldata.Directory(t), conversationpatchdata.Files)
	require.NoError(t, err)
	callbacks := &semanticTestCallbacks{avatarName: "Avatar", hasMet: true}
	engine, err := NewSemanticConversationEngine(originalCatalogDefinition(t, originalCatalog, semantic.OriginalSourceCastle, 9), callbacks, false)
	require.NoError(t, err)
	engine.Start(72)
	response := engine.ProcessInput("welc")
	stages := []string{"Welcome, welcome, welcome", "Lord Cantabrigian British", "trustworthy", "Chuckles the Bumble", "That's ME!"}
	for index, stage := range stages {
		require.NoError(t, response.Error)
		assertSemanticResponseMirror(t, response)
		assert.Contains(t, response.Output, stage)
		if index < len(stages)-1 {
			require.True(t, response.WaitingForKeypress)
			assert.NotContains(t, response.Output, stages[index+1])
			response = engine.ProcessInput("")
		} else {
			assert.False(t, response.WaitingForKeypress)
			assert.True(t, response.NeedsInput)
		}
	}
	routine := engine.ProcessInput("ente")
	require.NoError(t, routine.Error)
	assert.Equal(t, 3, strings.Count(routine.Output, "Ho eyo he hum!"))
	assert.Contains(t, routine.Output, "Bounce, bounce, bounce, bounce!")
	assert.Equal(t, 4, callbacks.pauses)
	require.True(t, routine.IsLabelQuestion)
	retry := engine.ProcessInput("n")
	require.NoError(t, retry.Error)
	assert.Contains(t, retry.Output, "better this time")
	assert.Contains(t, retry.Output, "Didst thou enjoy that?")
	accepted := engine.ProcessInput("y")
	require.NoError(t, accepted.Error)
	assert.Contains(t, accepted.Output, "I thought thou might")
	assert.False(t, accepted.IsLabelQuestion)
}

func TestOriginalSemanticRecruitmentAndRefusal(t *testing.T) {
	originalCatalog, err := catalog.LoadOriginal(originaldata.Directory(t), conversationpatchdata.Files)
	require.NoError(t, err)
	for _, npc := range []struct {
		name   string
		source semantic.OriginalSource
		dialog int
	}{
		{name: "Saduj", source: semantic.OriginalSourceCastle, dialog: 7},
		{name: "Jaana", source: semantic.OriginalSourceTowne, dialog: 35},
	} {
		for _, outcome := range []JoinPartyOutcome{JoinPartySuccess, JoinPartyCapacityRefused} {
			t.Run(fmt.Sprintf("%s/outcome=%d", npc.name, outcome), func(t *testing.T) {
				callbacks := &semanticTestCallbacks{avatarName: "Avatar", hasMet: true, joinOutcome: outcome}
				engine, engineErr := NewSemanticConversationEngine(originalCatalogDefinition(t, originalCatalog, npc.source, npc.dialog), callbacks, false)
				require.NoError(t, engineErr)
				engine.Start(74)
				response := engine.ProcessInput("join")
				for step := 0; step < 10 && callbacks.joined == 0; step++ {
					require.NoError(t, response.Error)
					if response.WaitingForKeypress {
						response = engine.ProcessInput("")
					} else {
						require.True(t, response.IsLabelQuestion)
						response = engine.ProcessInput("y")
					}
				}
				require.Equal(t, 1, callbacks.joined)
				require.NoError(t, response.Error)
				if outcome == JoinPartySuccess {
					assert.True(t, response.IsCompleteAwaitingDismissal)
					assert.False(t, engine.IsActive())
				} else {
					require.True(t, response.WaitingForKeypress)
					assert.True(t, engine.IsActive())
					assert.True(t, engine.ProcessInput("").NeedsInput)
				}
			})
		}
	}
}

func TestOriginalSemanticRecognitionAndEmptyGreeting(t *testing.T) {
	originalCatalog, err := catalog.LoadOriginal(originaldata.Directory(t), conversationpatchdata.Files)
	require.NoError(t, err)
	for _, dialog := range []int{30, 36} { // Delwyn and Greymarch.
		for _, known := range []bool{false, true} {
			t.Run(fmt.Sprintf("dialog=%d/known=%t", dialog, known), func(t *testing.T) {
				engine, engineErr := NewSemanticConversationEngine(originalCatalogDefinition(t, originalCatalog, semantic.OriginalSourceTowne, dialog), &semanticTestCallbacks{avatarName: "Avatar", hasMet: known}, false)
				require.NoError(t, engineErr)
				response := engine.Start(75)
				require.NoError(t, response.Error)
				assert.NotEmpty(t, response.Output)
				assert.NotContains(t, response.Output, `""`)
				assertSemanticResponseMirror(t, response)
			})
		}
	}
}

// Lady Janell plants each keyword in her own speech before the player can use
// it, and only a yes at her question asks the host for circle study.
// ref:PROGRESS-0005
func TestOriginalSemanticLadyJanellLeadsFromHerOwnWordsToCircleStudy(t *testing.T) {
	originalCatalog, err := catalog.LoadOriginal(originaldata.Directory(t), conversationpatchdata.Files)
	require.NoError(t, err)
	definition := originalCatalogDefinition(t, originalCatalog, semantic.OriginalSourceKeep, 11)
	for _, answer := range []string{"y", "n"} {
		t.Run("answer="+answer, func(t *testing.T) {
			callbacks := &semanticTestCallbacks{avatarName: "Avatar"}
			engine, engineErr := NewSemanticConversationEngine(definition, callbacks, false)
			require.NoError(t, engineErr)
			require.NoError(t, engine.Start(40).Error)
			assert.Contains(t, engine.ProcessInput("study").Output, responseNoHelp, "a bare keyword she never spoke does nothing")

			evilTimes := engine.ProcessInput("queen")
			assert.Contains(t, evilTimes.Output, "Evil times are before us")
			require.True(t, evilTimes.WaitingForKeypress)
			assert.Contains(t, engine.ProcessInput("").Output, "The Great Council is broken")

			council := engine.ProcessInput("council")
			assert.Contains(t, council.Output, "They sealed the eight dungeons")
			assert.Contains(t, council.Output, "defenders stronger")

			truth := engine.ProcessInput("stronger")
			assert.Contains(t, truth.Output, "precepts of Truth")
			require.True(t, truth.WaitingForKeypress)
			price := engine.ProcessInput("")
			assert.Contains(t, price.Output, "3000 gold crowns")
			assert.Contains(t, price.Output, "but once")
			require.True(t, price.WaitingForKeypress)
			question := engine.ProcessInput("")
			assert.Contains(t, question.Output, "Dost thou wish one of thy party to learn it now?")
			require.True(t, question.NeedsInput)
			require.Zero(t, callbacks.studies, "nothing is asked of the host before the answer")

			response := engine.ProcessInput(answer)
			require.NoError(t, response.Error)
			if answer == "y" {
				assert.Contains(t, response.Output, "Then let the one who would learn step forward.",
					"she speaks before the talk ends, so no empty quotation is shown")
				assert.True(t, response.IsCompleteAwaitingDismissal, "the talk ends so the study can open")
				assert.Equal(t, 1, callbacks.studies)

				return
			}
			assert.Contains(t, response.Output, "I see.")
			assert.False(t, response.IsCompleteAwaitingDismissal, "declining returns to her topics")
			assert.Zero(t, callbacks.studies)
			assert.Contains(t, engine.ProcessInput("truth").Output, "two sisters", "her own quest chain is unchanged")
		})
	}
}
