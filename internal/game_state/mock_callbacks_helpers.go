package game_state

import (
	"image/color"
	"slices"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/semantic_input"
)

// MockSystemCallbacks provides comprehensive mock implementations for all SystemCallbacks
// interfaces, with built-in assertion helpers and call tracking.
type MockSystemCallbacks struct {
	t            *testing.T
	gameState    *GameState
	timeBaseline datetime.UltimaDate

	// Message tracking
	Messages          []string
	StyledMessages    []StyledMessage
	CommandPrompts    []string
	CurrentRowContent string
	MessageCallCount  int

	// Audio tracking
	SoundEffectsPlayed []SoundEffect
	PlaybackRequests   []SoundPlaybackRequest
	StopRequests       []SoundStopRequest
	AudioCallCount     int
	MusicUpdateCalls   int

	// Visual tracking
	ScreenFlashes           []ScreenFlashCall
	ScreenFlashColors       []ScreenFlashColorCall  // NEW: Track SetScreenFlashColor calls
	ScreenFlashIntensities  []float64               // NEW: Track SetScreenFlashIntensity calls
	ScreenShakeOffsets      []ScreenShakeOffsetCall // NEW: Track SetScreenShakeOffset calls
	MapFadeLevels           []float64
	ProjectilesStarted      []ProjectileCall   // Track StartProjectile calls
	ProjectileProgress      []float32          // Track UpdateProjectileProgress calls
	ProjectilesClearedCount int                // Track ClearProjectile calls
	CombatImpactCalls       []CombatImpactCall // Track local combat hit feedback
	CombatEvents            []CombatEvent      // Track durable combat event records
	CombatConclusions       []CombatConclusion // Track completed encounter handoffs
	TransientIconCalls      []TransientIconKind
	ExperienceAwards        []DeedExperienceAward
	DungeonInjuryCalls      []DungeonInjuryKind
	VisualCallCount         int

	// Screen tracking
	DisplayUpdates  int // Track calls to UpdateDisplay for search visual feedback
	ScreenCallCount int

	// Flow tracking
	FlowCallCount int

	// Talk tracking
	TalkDialogCalls []TalkDialogCall
	DialogsPushed   []TalkDialog
	TalkCallCount   int

	// Input blocking tracking
	InputBlocked      bool
	BlockInputCalls   int
	UnblockInputCalls int
	ScreenShakeCalls  []ScreenShakeCall

	// Well interaction tracking
	WellInteractionShown    bool
	WellInteractionPosition *references.Position

	// Fountain interaction tracking
	FountainInteractionShown bool

	// Horse refusal tracking
	HorseRefusalShown bool

	// Shadowlord tracking
	ShadowlordSummonedTriggered     []references.Shadowlord   // Tracks TriggerShadowlordSummoned calls (yell summoning)
	ShadowlordPresenceEffectsCalled [][]references.Shadowlord // Tracks ThenShadowlordPresenceEffect calls (each is a slice of lords)

	// View/Crystal Ball tracking
	ShowGemViewInteractionCalled bool // Tracks ShowGemViewInteraction calls

	// Scroll workflow tracking
	StartWindChangeScrollWorkflowCalled   bool // Tracks StartWindChangeScrollWorkflow calls
	StartLightScrollWorkflowCalled        bool // Tracks StartLightScrollWorkflow calls
	StartTimeStopScrollWorkflowCalled     bool // Tracks StartTimeStopScrollWorkflow calls
	StartResurrectionScrollWorkflowCalled bool // Tracks StartResurrectionScrollWorkflow calls
	HMSCapeUseCommit                      *HMSCapeUseCommit

	// Spell expired popup tracking
	SpellExpiredPopupShown bool   // Tracks if ShowSpellExpiredPopup was called
	SpellExpiredName       string // Tracks the spell name passed to ShowSpellExpiredPopup

	// Timed overlay message tracking
	TimedOverlayMessages   []string // Tracks messages passed to ShowTimedOverlayMessage
	SearchFeedbackMessages []string

	// Stonegate trap workflow tracking
	StonegateTrapTriggered bool // Tracks if TriggerStonegateTrap was called

	// Sign dialog tracking
	SignDialogShown bool                   // Tracks if ShowSignDialog was called
	SignDialogSign  *references.SignRecord // The sign passed to ShowSignDialog (nil = default message)
	SignDialogCalls int                    // Number of times ShowSignDialog was called

	// Ready equipment dialog tracking
	ReadyDialogShown      bool                  // Tracks if ShowReadyDialog was called
	ReadyDialogCombatMode bool                  // Tracks the request combat mode
	ReadyDialogRequest    ReadyEquipmentRequest // Tracks the exact Ready target
	ReadyDialogCalls      int                   // Number of times ShowReadyDialog was called
}

// ScreenFlashCall tracks calls to screen flash effect function
type ScreenFlashCall struct {
	R, G, B    float64
	Intensity  float64
	DurationMs int
}

// ScreenShakeCall tracks calls to screen shake function
type ScreenShakeCall struct {
	Intensity  float64
	DurationMs int
}

// ScreenFlashColorCall tracks calls to SetScreenFlashColor (NEW for event queue pattern)
type ScreenFlashColorCall struct {
	R, G, B float64
}

// ScreenShakeOffsetCall tracks calls to SetScreenShakeOffset (NEW for event queue pattern)
type ScreenShakeOffsetCall struct {
	X, Y float64
}

// ProjectileCall tracks calls to StartProjectile function
type ProjectileCall struct {
	FromPos        references.Position
	ToPos          references.Position
	ProjectileType references.MissileType
	DurationMs     uint64 // Duration returned by the mock
}

// CombatImpactCall records local hit feedback requested by combat damage.
type CombatImpactCall struct {
	Feedback CombatAttackFeedback
}

// TalkDialogCall tracks talk dialog creation requests
type TalkDialogCall struct {
	NPC *map_units.NPCFriendly
}

// NewMockSystemCallbacks creates a new mock system callbacks instance
func NewMockSystemCallbacks(t *testing.T) *MockSystemCallbacks {
	t.Helper()

	return &MockSystemCallbacks{
		t:                      t,
		Messages:               make([]string, 0),
		CommandPrompts:         make([]string, 0),
		SoundEffectsPlayed:     make([]SoundEffect, 0),
		PlaybackRequests:       make([]SoundPlaybackRequest, 0),
		StopRequests:           make([]SoundStopRequest, 0),
		TalkDialogCalls:        make([]TalkDialogCall, 0),
		DialogsPushed:          make([]TalkDialog, 0),
		ScreenFlashColors:      make([]ScreenFlashColorCall, 0),  // NEW
		ScreenFlashIntensities: make([]float64, 0),               // NEW
		ScreenShakeOffsets:     make([]ScreenShakeOffsetCall, 0), // NEW
	}
}

// ToSystemCallbacks converts the mock to actual SystemCallbacks interface
//
//nolint:maintidx // Keep the complete required callback wiring in one auditable test adapter.
func (m *MockSystemCallbacks) ToSystemCallbacks() *SystemCallbacks {
	messageCallbacks := NewMessageCallbacks(MessageCallbacks{
		AddRowStr:             m.addRowStr,
		AppendToCurrentRowStr: m.appendToCurrentRowStr,
		ShowCommandPrompt:     m.showCommandPrompt,
		AppendStyledMessage:   m.appendStyledMessage,
	})

	visualCallbacks := NewVisualCallbacks(VisualCallbacks{
		SetWandererDissolveLevel:                  func(float64) {},
		InitializeWandererDissolveEffect:          func() {},
		CleanupWandererDissolveEffect:             func() {},
		SetAvatarDissolveLevel:                    func(float64) {},
		InitializeAvatarDissolveEffect:            func() {},
		CleanupAvatarDissolveEffect:               func() {},
		SetMapFadeLevel:                           m.setMapFadeLevel,
		SetScreenFlashColor:                       m.setScreenFlashColor,
		SetScreenFlashIntensity:                   m.setScreenFlashIntensity,
		SetScreenShakeOffset:                      m.setScreenShakeOffset,
		SetScreenInvert:                           func(float64, int) {},
		BlockAllInput:                             m.blockAllInput,
		IsAnyKeyPressed:                           func() bool { return false },
		GetPressedKey:                             func() *semantic_input.Key { return nil },
		ResetTeleportationSourceRenderer:          func() {},
		StabilizeTeleportationDestinationRenderer: func(MoongateDestination) {},
		StartProjectile:                           m.startProjectile,
		UpdateProjectileProgress:                  m.updateProjectileProgress,
		ClearProjectile:                           m.clearProjectile,
		PlayKapowAtPosition: func(pos references.Position) {
			m.t.Logf("💥 PlayKapowAtPosition at (%d, %d)", pos.X, pos.Y)
		},
		ShowCombatImpact: func(feedback CombatAttackFeedback) {
			m.CombatImpactCalls = append(m.CombatImpactCalls, CombatImpactCall{Feedback: feedback})
		},
		ShowDungeonInjury: func(kind DungeonInjuryKind) {
			m.DungeonInjuryCalls = append(m.DungeonInjuryCalls, kind)
			m.VisualCallCount++
		},
	})
	combatCallbacks := NewCombatCallbacks(
		func(event CombatEvent) { m.CombatEvents = append(m.CombatEvents, event) },
		func(conclusion CombatConclusion) { m.CombatConclusions = append(m.CombatConclusions, conclusion) },
	)

	audioCallbacks := NewAudioCallbacks(
		m.requestSoundPlayback,
		m.requestSoundStop,
		m.updateLocationMusic,
		func() {}, // PauseBackgroundMusic no-op for mock
		func() {}, // ResumeBackgroundMusic no-op for mock
		m.requestSoundVoice,
	)

	screenCallbacks := NewScreenCallbacks(ScreenCallbacks{
		UpdateDisplay:   m.updateDisplay,
		HideAllOverlays: func() {},
		ShowAllOverlays: func() {},
		CreateAndPushMessageDialog: func() {
			m.t.Logf("💬 CreateAndPushMessageDialog called")
		},
		CreateAndPushCampingResultDialog: func(message string) { m.t.Logf("Camping result dialog: %s", message) },
		PopMessageDialog:                 func() { m.t.Logf("💬 PopMessageDialog called") },
		HideMessageDialog:                func() { m.t.Logf("💬 HideMessageDialog called") },
		ClearMessageDialog:               func() { m.t.Logf("💬 ClearMessageDialog called") },
		SetMessagePrompt: func(prompt string) {
			m.t.Logf("💬 SetMessagePrompt: %s", prompt)
		},
		SetMessagePromptColor: func(prompt string, c color.Color) {
			m.t.Logf("💬 SetMessagePromptColor: %s (color: %v)", prompt, c)
		},
		ClearMessagePrompt: func() { m.t.Logf("💬 ClearMessagePrompt called") },
		SetMessageDialogFadeDurations: func(fadeInMs, fadeOutMs int) {
			m.t.Logf("💬 SetMessageDialogFadeDurations: in=%dms, out=%dms", fadeInMs, fadeOutMs)
		},
		GetMessageDialogFadeInDuration:  func() int { return 300 },
		GetMessageDialogFadeOutDuration: func() int { return 200 },
		ShowPressAnyKeyPrompt:           func() { m.t.Logf("💬 ShowPressAnyKeyPrompt called") },
	})

	flowCallbacks := NewFlowCallbacks(FlowCallbacks{
		Consequences: FlowConsequenceCallbacks{
			TriggerDeathSequence: func() {},
			TriggerStonegateTrap: func(StonegateTrapOutcome) {
				m.t.Logf("🌋 TriggerStonegateTrap called")
				m.StonegateTrapTriggered = true
			},
		},
		Rest: FlowRestCallbacks{
			RequestSleepDuration: func() {},
			RequestCampDuration:  func() {},
		},
		WorldInteraction: FlowWorldInteractionCallbacks{
			ShowWellInteraction:     m.showWellInteraction,
			ShowFountainInteraction: m.showFountainInteraction,
			ShowHorseRefusalDialog:  m.showHorseRefusalDialog,
		},
		Observation: FlowObservationCallbacks{
			ShowTelescopeInteraction: func() { m.t.Logf("🔭 ShowTelescopeInteraction called") },
			ShowCrystalBallInteraction: func() {
				m.t.Logf("🔮 ShowCrystalBallInteraction called")
			},
			ShowGemViewInteraction: func(complete func()) {
				m.t.Logf("💎 ShowGemViewInteraction called")
				m.ShowGemViewInteractionCalled = true
				if complete != nil {
					complete()
				}
			},
			ShowSpyglassInteraction: func(complete func()) {
				m.t.Logf("🔭 ShowSpyglassInteraction called")
				if complete != nil {
					complete()
				}
			},
			ShowPocketWatchTime: func(timeString string, complete func()) {
				m.t.Logf("⏰ ShowPocketWatchTime called: %s", timeString)
				if complete != nil {
					complete()
				}
			},
		},
		Shadowlord: FlowShadowlordCallbacks{
			TriggerShadowlordSummoned: func(lord references.Shadowlord) {
				m.t.Logf("👻 TriggerShadowlordSummoned: %s", lord.String())
				m.ShadowlordSummonedTriggered = append(m.ShadowlordSummonedTriggered, lord)
			},
			ThenShadowlordPresenceEffect: func(lords []references.Shadowlord) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
				m.ShadowlordPresenceEffectsCalled = append(m.ShadowlordPresenceEffectsCalled, lords)

				return func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder { return q }
			},
			ShowShardUsageWorkflow: func(shard references.QuestItem, complete func()) {
				m.t.Logf("Shard usage workflow called for item %d", shard)
				if complete != nil {
					complete()
				}
			},
		},
		Items: FlowItemCallbacks{
			StartPotionWorkflow: func(commit *PotionUseCommit, complete func()) {
				m.t.Logf("Potion workflow called: potion %d", commit.review.Potion)
				if complete != nil {
					complete()
				}
			},
			StartWindChangeScrollWorkflow: func(complete func()) {
				m.StartWindChangeScrollWorkflowCalled = true
				if complete != nil {
					complete()
				}
			},
			StartLightScrollWorkflow: func(complete func()) {
				m.StartLightScrollWorkflowCalled = true
				if complete != nil {
					complete()
				}
			},
			StartTimeStopScrollWorkflow: func(complete func()) {
				m.StartTimeStopScrollWorkflowCalled = true
				if complete != nil {
					complete()
				}
			},
			StartResurrectionScrollWorkflow: func(complete func()) {
				m.StartResurrectionScrollWorkflowCalled = true
				if complete != nil {
					complete()
				}
			},
			TriggerToggleItemFlash: func(ToggleItemType) {},
			StartMoonstoneBurialWorkflow: func(moonstone references.Moonstone, complete func()) {
				m.t.Logf("🌙 StartMoonstoneBurialWorkflow called: moonstone %d", moonstone)
				if complete != nil {
					complete()
				}
			},
			StartCarpetBoardingWorkflow: func(_ *CarpetBoardingCommit, complete func()) {
				if complete != nil {
					complete()
				}
			},
			StartSkullKeyWorkflow: func(_ *SkullKeyUseCommit, complete func()) {
				if complete != nil {
					complete()
				}
			},
			StartHMSCapeWorkflow: func(commit *HMSCapeUseCommit, complete func()) {
				m.HMSCapeUseCommit = commit
				if complete != nil {
					complete()
				}
			},
			StartSceptreWorkflow: func(complete func()) {
				if complete != nil {
					complete()
				}
			},
			ShowReadyDialog: func(request ReadyEquipmentRequest) {
				m.ReadyDialogShown = true
				m.ReadyDialogCombatMode = request.CombatMode
				m.ReadyDialogRequest = request
				m.ReadyDialogCalls++
			},
		},
		SacredSites: FlowSacredSiteCallbacks{
			StartShrineWorkflow: func(shrineIndex int) {
				m.t.Logf("🛕 StartShrineWorkflow called for shrine %d", shrineIndex)
			},
			StartCodexWorkflow: func() { m.t.Logf("📖 StartCodexWorkflow called") },
		},
	})

	talkCallbacks := NewTalkCallbacks(
		m.createTalkDialog,
		m.pushDialog,
	)

	uiCallbacks := UICallbacks{
		ShowZStatsDossier: func(ZStatsDossier) {
			m.t.Logf("ShowZStatsDossier called")
		},
		JournalChanged: func(int, int) {},
		AtlasUpdated:   func(journal.AtlasChange) {},
		ShowQuestJournalDossier: func(QuestJournalDossier) {
			m.t.Logf("ShowQuestJournalDossier called")
		},
		ShowYellDialogLargeMap: func() {
			m.t.Logf("💬 ShowYellDialogLargeMap called (mock no-op)")
		},
		ShowTimedOverlayMessage: func(message string, durationMs int, centerHorizontally bool, alsoAddToOutput bool) {
			m.t.Logf("💬 ShowTimedOverlayMessage: %q for %dms, centered=%v, alsoAddToOutput=%v", message, durationMs, centerHorizontally, alsoAddToOutput)
			m.TimedOverlayMessages = append(m.TimedOverlayMessages, message)
			if alsoAddToOutput {
				m.Messages = append(m.Messages, message)
			}
		},
		ShowActionFeedback: func(message string, alsoAddToOutput bool) {
			m.t.Logf("🔔 ShowActionFeedback called: %q, alsoAddToOutput=%v", message, alsoAddToOutput)
			if alsoAddToOutput {
				m.Messages = append(m.Messages, message)
			}
		},
		ShowSearchFeedback: func(message string) {
			m.SearchFeedbackMessages = append(m.SearchFeedbackMessages, message)
		},
		ShowSpellExpiredPopup: func(spellName string) {
			m.t.Logf("✨ ShowSpellExpiredPopup: %s wore off!", spellName)
			m.SpellExpiredPopupShown = true
			m.SpellExpiredName = spellName
		},
		ShowTransientIcon: func(kind TransientIconKind) {
			m.t.Logf("ShowTransientIcon: kind=%d", kind)
			m.TransientIconCalls = append(m.TransientIconCalls, kind)
		},
		ShowExperienceAward: func(award DeedExperienceAward) {
			m.ExperienceAwards = append(m.ExperienceAwards, award)
		},
		ShowSignDialog: func(sign *references.SignRecord) {
			m.SignDialogShown = true
			m.SignDialogSign = sign
			m.SignDialogCalls++
			if sign != nil {
				m.t.Logf("🪧 ShowSignDialog called: location=%d at (%d, %d)", sign.Location, sign.X, sign.Y)
			} else {
				m.t.Logf("🪧 ShowSignDialog called: (default message)")
			}
		},
	}

	systemCallbacks, err := NewSystemCallbacks(
		messageCallbacks,
		visualCallbacks,
		audioCallbacks,
		screenCallbacks,
		flowCallbacks,
		talkCallbacks,
		uiCallbacks,
		combatCallbacks,
	)

	if err != nil {
		m.t.Fatalf("Failed to create SystemCallbacks: %v", err)
	}

	return systemCallbacks
}

// Message callback implementations
func (m *MockSystemCallbacks) addRowStr(str string) {
	m.Messages = append(m.Messages, str)
	m.MessageCallCount++
	m.t.Logf("📝 Message: %s", str)
}

func (m *MockSystemCallbacks) appendToCurrentRowStr(str string) {
	m.CurrentRowContent += str
	m.MessageCallCount++
	m.t.Logf("📝 Append: %s", str)
}

func (m *MockSystemCallbacks) showCommandPrompt(command string) {
	m.CommandPrompts = append(m.CommandPrompts, command)
	m.MessageCallCount++
	m.t.Logf("Command prompt: %s", command)
}

func (m *MockSystemCallbacks) appendStyledMessage(message StyledMessage) {
	message.Segments = slices.Clone(message.Segments)
	m.StyledMessages = append(m.StyledMessages, message)
	var plain strings.Builder
	for _, segment := range message.Segments {
		plain.WriteString(segment.Text)
	}
	m.addRowStr(plain.String())
}

// Visual callback implementations

func (m *MockSystemCallbacks) setMapFadeLevel(level float64) {
	m.MapFadeLevels = append(m.MapFadeLevels, level)
	m.VisualCallCount++
}

func (m *MockSystemCallbacks) setScreenFlashColor(r, g, b float64) {
	m.ScreenFlashColors = append(m.ScreenFlashColors, ScreenFlashColorCall{
		R: r, G: g, B: b,
	})
	m.VisualCallCount++
	m.t.Logf("🎨 Screen flash color set: RGB(%.2f,%.2f,%.2f)", r, g, b)
}

func (m *MockSystemCallbacks) setScreenFlashIntensity(intensity float64) {
	m.ScreenFlashIntensities = append(m.ScreenFlashIntensities, intensity)
	m.VisualCallCount++
	m.t.Logf("💡 Screen flash intensity set: %.2f", intensity)
}

func (m *MockSystemCallbacks) setScreenShakeOffset(x, y float64) {
	m.ScreenShakeOffsets = append(m.ScreenShakeOffsets, ScreenShakeOffsetCall{
		X: x, Y: y,
	})
	m.VisualCallCount++
	m.t.Logf("📐 Screen shake offset set: (%.2f, %.2f)", x, y)
}

// Projectile system mock methods

func (m *MockSystemCallbacks) startProjectile(fromPos, toPos references.Position, projectileType references.MissileType) uint64 {
	// Calculate mock duration based on tile distance (using simplified formula)
	dx := float64(toPos.X) - float64(fromPos.X)
	dy := float64(toPos.Y) - float64(fromPos.Y)
	distance := dx*dx + dy*dy // Squared distance for simplicity
	if distance < 1 {
		distance = 1
	}
	durationMs := uint64(100 + distance*10) // Simple formula for mock duration

	m.ProjectilesStarted = append(m.ProjectilesStarted, ProjectileCall{
		FromPos:        fromPos,
		ToPos:          toPos,
		ProjectileType: projectileType,
		DurationMs:     durationMs,
	})
	m.VisualCallCount++
	m.t.Logf("🎯 Projectile started: from(%d,%d) to(%d,%d) type=%v duration=%dms",
		fromPos.X, fromPos.Y, toPos.X, toPos.Y, projectileType, durationMs)

	return durationMs
}

func (m *MockSystemCallbacks) updateProjectileProgress(progress float32) {
	m.ProjectileProgress = append(m.ProjectileProgress, progress)
	m.VisualCallCount++
	// Only log significant progress updates to reduce noise
	if progress == 0 || progress >= 0.99 || len(m.ProjectileProgress)%10 == 0 {
		m.t.Logf("🎯 Projectile progress: %.2f", progress)
	}
}

func (m *MockSystemCallbacks) clearProjectile() {
	m.ProjectilesClearedCount++
	m.VisualCallCount++
	m.t.Logf("🎯 Projectile cleared (total: %d)", m.ProjectilesClearedCount)
}

func (m *MockSystemCallbacks) blockAllInput(blocked bool) {
	m.InputBlocked = blocked
	if blocked {
		m.BlockInputCalls++
		m.t.Logf("🔒 Input blocked")
	} else {
		m.UnblockInputCalls++
		m.t.Logf("🔓 Input unblocked")
	}
	m.VisualCallCount++
}

// Audio callback implementations
//
//nolint:ireturn // Implements the opaque presentation callback contract for gameplay fixtures.
func (m *MockSystemCallbacks) requestSoundVoice(request SoundPlaybackRequest) SoundVoice {
	m.requestSoundPlayback(request)

	return &mockSoundVoice{callbacks: m, effect: request.Effect, stopped: false}
}

type mockSoundVoice struct {
	callbacks *MockSystemCallbacks
	effect    SoundEffect
	stopped   bool
}

func (voice *mockSoundVoice) Stop(fadeOutMs int64) {
	if !voice.stopped {
		voice.stopped = true
		voice.callbacks.requestSoundStop(SoundStopRequest{Effect: voice.effect, FadeOutMs: fadeOutMs})
	}
}

func (m *MockSystemCallbacks) requestSoundPlayback(request SoundPlaybackRequest) {
	m.SoundEffectsPlayed = append(m.SoundEffectsPlayed, request.Effect)
	m.PlaybackRequests = append(m.PlaybackRequests, request)
	m.AudioCallCount++
	m.t.Logf("🔊 Sound: %v", request.Effect)
}

func (m *MockSystemCallbacks) requestSoundStop(request SoundStopRequest) {
	m.StopRequests = append(m.StopRequests, request)
	m.t.Logf("🔇 Stop sound: %v (fade-out: %dms)", request.Effect, request.FadeOutMs)
}

func (m *MockSystemCallbacks) updateLocationMusic() {
	m.MusicUpdateCalls++
	m.AudioCallCount++
	m.t.Logf("🎵 Music: Location updated")
}

// Screen callback implementations
func (m *MockSystemCallbacks) updateDisplay() {
	m.DisplayUpdates++
	m.ScreenCallCount++
	m.t.Logf("🖥️ Display updated (search visual feedback)")
}

func (m *MockSystemCallbacks) showWellInteraction(pos *references.Position) {
	m.WellInteractionShown = true
	m.WellInteractionPosition = pos
	m.FlowCallCount++
	if pos != nil {
		m.t.Logf("🪣 Show well interaction at position (%d, %d)", pos.X, pos.Y)
	} else {
		m.t.Logf("🪣 Show well interaction at nil position")
	}
}

func (m *MockSystemCallbacks) showFountainInteraction() {
	m.FountainInteractionShown = true
	m.FlowCallCount++
	m.t.Logf("⛲ Show fountain interaction (cosmetic drinking)")
}

func (m *MockSystemCallbacks) showHorseRefusalDialog() {
	m.HorseRefusalShown = true
	m.FlowCallCount++
	m.t.Logf("🐴 Horse refusal dialog shown (\"Nay!\")")
}

// Talk callback implementations
//
//nolint:ireturn // Implements the required TalkDialog callback boundary for test fixtures.
func (m *MockSystemCallbacks) createTalkDialog(npc *map_units.NPCFriendly) TalkDialog {
	m.TalkDialogCalls = append(m.TalkDialogCalls, TalkDialogCall{NPC: npc})
	m.TalkCallCount++
	m.t.Logf("💭 Create talk dialog for NPC")
	// Return nil for now - can be extended with mock dialog if needed
	return nil
}

func (m *MockSystemCallbacks) pushDialog(dialog TalkDialog) {
	m.DialogsPushed = append(m.DialogsPushed, dialog)
	m.TalkCallCount++
	m.t.Logf("📋 Dialog pushed")
}

// Test assertion helpers

// MessagesContain returns true if any recorded message contains the expected substring.
func (m *MockSystemCallbacks) MessagesContain(expected string) bool {
	for _, msg := range m.Messages {
		if strings.Contains(msg, expected) {
			return true
		}
	}

	return false
}

// SoundEffectWasPlayed returns true if the specified sound effect was played.
func (m *MockSystemCallbacks) SoundEffectWasPlayed(expected SoundEffect) bool {
	for _, effect := range m.SoundEffectsPlayed {
		if effect == expected {
			return true
		}
	}

	return false
}

// PlaybackRequestCount returns the number of matching typed playback requests.
func (m *MockSystemCallbacks) PlaybackRequestCount(effect SoundEffect, mode SoundPlaybackMode) int {
	count := 0
	for _, request := range m.PlaybackRequests {
		if request.Effect == effect && request.Mode == mode {
			count++
		}
	}

	return count
}

// StopRequestCount returns the number of stop requests for one effect.
func (m *MockSystemCallbacks) StopRequestCount(effect SoundEffect) int {
	count := 0
	for _, request := range m.StopRequests {
		if request.Effect == effect {
			count++
		}
	}

	return count
}

// AssertMessageContains checks whether recorded output includes the expected text.
func (m *MockSystemCallbacks) AssertMessageContains(expected string) {
	m.t.Helper()
	for _, msg := range m.Messages {
		if strings.Contains(msg, expected) {
			return
		}
	}
	m.t.Errorf("Expected message containing '%s', but not found in: %v", expected, m.Messages)
}

// AssertTimeAdvanced checks authoritative elapsed calendar minutes.
func (m *MockSystemCallbacks) AssertTimeAdvanced(expectedMinutes int) {
	m.t.Helper()
	totalAdvanced := 0
	for _, minutes := range m.TimeAdvanced() {
		totalAdvanced += minutes
	}
	if totalAdvanced != expectedMinutes {
		m.t.Errorf("Expected %d minutes advanced, got %d", expectedMinutes, totalAdvanced)
	}
}

// BindGameState makes time assertions derive from authoritative GameState calendar state.
func (m *MockSystemCallbacks) BindGameState(state *GameState) {
	m.gameState = state
	m.timeBaseline = state.DateTime
}

// TimeAdvanced returns state-derived elapsed minutes since the last bind or reset.
// Turn boundaries preserve the existing test helper's per-operation shape without a gameplay callback.
func (m *MockSystemCallbacks) TimeAdvanced() []int {
	if m.gameState == nil {
		return nil
	}
	elapsed := ultimaDateAbsoluteMinute(m.gameState.DateTime) - ultimaDateAbsoluteMinute(m.timeBaseline)
	if elapsed <= 0 {
		return nil
	}
	turns := int64(m.gameState.DateTime.Turn - m.timeBaseline.Turn)
	if turns <= 1 {
		return []int{int(elapsed)}
	}
	advances := make([]int, turns)
	for i := range len(advances) - 1 {
		advances[i] = 1
	}
	advances[len(advances)-1] = int(elapsed - (turns - 1))

	return advances
}

func ultimaDateAbsoluteMinute(date datetime.UltimaDate) int64 {
	days := (int64(date.Year)-1)*datetime.MonthsPerYear*datetime.DaysInMonth +
		(int64(date.Month)-1)*datetime.DaysInMonth + int64(date.Day) - 1

	return days*datetime.HoursPerDay*datetime.MinutesPerHour +
		int64(date.Hour)*datetime.MinutesPerHour + int64(date.Minute)
}

// Reset clears all recorded calls for test reuse
func (m *MockSystemCallbacks) Reset() {
	m.Messages = m.Messages[:0]
	m.StyledMessages = m.StyledMessages[:0]
	m.SearchFeedbackMessages = m.SearchFeedbackMessages[:0]
	m.CommandPrompts = m.CommandPrompts[:0]
	m.CurrentRowContent = ""
	m.MessageCallCount = 0

	m.SoundEffectsPlayed = m.SoundEffectsPlayed[:0]
	m.PlaybackRequests = m.PlaybackRequests[:0]
	m.StopRequests = m.StopRequests[:0]
	m.AudioCallCount = 0
	m.MusicUpdateCalls = 0

	m.ScreenShakeCalls = m.ScreenShakeCalls[:0]
	m.MapFadeLevels = m.MapFadeLevels[:0]
	m.CombatImpactCalls = m.CombatImpactCalls[:0]
	m.CombatEvents = m.CombatEvents[:0]
	m.CombatConclusions = m.CombatConclusions[:0]
	m.TransientIconCalls = m.TransientIconCalls[:0]
	m.ExperienceAwards = m.ExperienceAwards[:0]
	m.DungeonInjuryCalls = m.DungeonInjuryCalls[:0]
	m.InputBlocked = false
	m.BlockInputCalls = 0
	m.UnblockInputCalls = 0
	m.VisualCallCount = 0

	m.DisplayUpdates = 0
	m.ScreenCallCount = 0

	m.FlowCallCount = 0
	m.HorseRefusalShown = false

	m.TalkDialogCalls = m.TalkDialogCalls[:0]
	m.DialogsPushed = m.DialogsPushed[:0]
	m.TalkCallCount = 0

	m.ShadowlordSummonedTriggered = m.ShadowlordSummonedTriggered[:0]
	m.ShadowlordPresenceEffectsCalled = m.ShadowlordPresenceEffectsCalled[:0]

	// View/Crystal Ball tracking
	m.ShowGemViewInteractionCalled = false

	// Scroll workflow tracking
	m.StartWindChangeScrollWorkflowCalled = false
	m.StartLightScrollWorkflowCalled = false
	m.StartTimeStopScrollWorkflowCalled = false
	m.StartResurrectionScrollWorkflowCalled = false
	m.HMSCapeUseCommit = nil

	// Spell expired popup tracking
	m.SpellExpiredPopupShown = false
	m.SpellExpiredName = ""

	// Stonegate trap workflow tracking
	m.StonegateTrapTriggered = false

	// Sign dialog tracking
	m.SignDialogShown = false
	m.SignDialogSign = nil
	m.SignDialogCalls = 0

	// Ready equipment dialog tracking
	m.ReadyDialogShown = false
	m.ReadyDialogCombatMode = false
	m.ReadyDialogRequest = ReadyEquipmentRequest{}
	m.ReadyDialogCalls = 0
	if m.gameState != nil {
		m.timeBaseline = m.gameState.DateTime
	}
}
