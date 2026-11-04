package main

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
)

type debugDispatchTestScene struct {
	executedBeforeUpdate bool
	executed             bool
	mutateArguments      bool
	updates              int
	optionSets           []debugging.OptionSet
	optionReads          int
}

func (s *debugDispatchTestScene) Update(_ *Game) error {
	s.executedBeforeUpdate = s.executed
	s.updates++

	return nil
}

func (s *debugDispatchTestScene) Draw(_ *ebiten.Image) {}

func (s *debugDispatchTestScene) GetUltimaConfiguration() *config.UltimaVConfiguration {
	return nil
}

func (s *debugDispatchTestScene) InvalidateResolution() {}

func (s *debugDispatchTestScene) ExecuteDebugCommand(request debugging.Request) debugging.Result {
	s.executed = true
	if s.mutateArguments {
		request.Arguments["direction"][1] = 'X'
		request.Arguments["executorOnly"] = json.RawMessage(`true`)
	}

	return debugging.SuccessResult(request, debugging.ExecutionQuery, "executed", nil)
}

func (s *debugDispatchTestScene) DebugOptionSets() []debugging.OptionSet {
	s.optionReads++
	return s.optionSets
}

type noDebugDispatchTestScene struct {
	updates int
}

func (s *noDebugDispatchTestScene) Update(_ *Game) error {
	s.updates++

	return nil
}

func (s *noDebugDispatchTestScene) Draw(_ *ebiten.Image) {}

func (s *noDebugDispatchTestScene) GetUltimaConfiguration() *config.UltimaVConfiguration {
	return nil
}

func (s *noDebugDispatchTestScene) InvalidateResolution() {}

type lifecycleTestScene struct {
	name            string
	events          *[]string
	preflightErr    error
	activationPanic any
	debugDispatchTestScene
}

func (s *lifecycleTestScene) PreflightActivation() error { return s.preflightErr }

func (s *lifecycleTestScene) PrepareInputForSceneActivation() {
	*s.events = append(*s.events, s.name+".prepare-input")
}

func (s *lifecycleTestScene) Activate() {
	*s.events = append(*s.events, s.name+".activate")
	if s.activationPanic != nil {
		panic(s.activationPanic)
	}
}

func (s *lifecycleTestScene) PrepareForSceneReplacement() {
	*s.events = append(*s.events, s.name+".prepare")
}

func (s *lifecycleTestScene) Dispose() {
	*s.events = append(*s.events, s.name+".dispose")
}

func TestGameProcessesDebugRequestBeforeSceneUpdate(t *testing.T) {
	scene := &debugDispatchTestScene{}
	game := NewGame(scene)
	require.NoError(t, game.SubmitDebugRequest(debugging.Request{
		SessionID: "test-session",
		ID:        "request-1",
		CommandID: debugging.CommandEnvironmentWindStatus,
	}))

	err := game.Update()

	require.NoError(t, err)
	assert.True(t, scene.executedBeforeUpdate)
	assert.Equal(t, 1, scene.updates)
	result, ok := game.NextDebugResult()
	require.True(t, ok)
	assert.True(t, result.Success)
	assert.Equal(t, game.DebugSceneGeneration(), result.SceneGeneration)
	require.NotNil(t, result.Invocation)
	assert.Equal(t, debugging.CommandEnvironmentWindStatus, result.Invocation.CommandID)
	assert.Empty(t, result.Invocation.Arguments)
}

func TestGameRejectsDebugRequestWhenSceneHasNoExecutor(t *testing.T) {
	scene := &noDebugDispatchTestScene{}
	game := NewGame(scene)
	require.NoError(t, game.SubmitDebugRequest(debugging.Request{
		SessionID: "test-session",
		ID:        "request-1",
		CommandID: debugging.CommandEnvironmentWindStatus,
	}))

	err := game.Update()

	require.NoError(t, err)
	result, ok := game.NextDebugResult()
	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, "gameplay_unavailable", result.ErrorCode)
	require.NotNil(t, result.Invocation)
	assert.Equal(t, debugging.CommandEnvironmentWindStatus, result.Invocation.CommandID)
	assert.Equal(t, 1, scene.updates)
}

func TestGameRejectsUnknownDebugCommandBeforeSceneDispatch(t *testing.T) {
	scene := &debugDispatchTestScene{}
	game := NewGame(scene)
	require.NoError(t, game.SubmitDebugRequest(debugging.Request{
		SessionID: "test-session",
		ID:        "request-1",
		CommandID: "unknown.command",
	}))

	err := game.Update()

	require.NoError(t, err)
	assert.False(t, scene.executed)
	result, ok := game.NextDebugResult()
	require.True(t, ok)
	assert.Equal(t, "invalid_request", result.ErrorCode)
	assert.Nil(t, result.Invocation)
}

func TestGameExecutesApplicationQueryWithoutGameplayExecutor(t *testing.T) {
	scene := &noDebugDispatchTestScene{}
	game := NewGame(scene)
	require.NoError(t, game.SubmitDebugRequest(debugging.Request{
		SessionID: "test-session",
		ID:        "request-1",
		CommandID: debugging.CommandLoggingStatus,
	}))

	require.NoError(t, game.Update())
	result, ok := game.NextDebugResult()

	require.True(t, ok)
	assert.True(t, result.Success)
	assert.Equal(t, debugging.ExecutionQuery, result.EffectiveExecutionMode)
}

func TestGameRejectsRequestForReplacedScene(t *testing.T) {
	game := NewGame(&debugDispatchTestScene{})
	staleGeneration := game.DebugSceneGeneration()
	game.replaceScene(&debugDispatchTestScene{})
	require.NoError(t, game.SubmitDebugRequest(debugging.Request{
		SessionID:               "test-session",
		ID:                      "request-1",
		CommandID:               debugging.CommandEnvironmentWindStatus,
		ExpectedSceneGeneration: staleGeneration,
	}))

	require.NoError(t, game.Update())
	result, ok := game.NextDebugResult()

	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, "stale_scene", result.ErrorCode)
	assert.Equal(t, game.DebugSceneGeneration(), result.SceneGeneration)
}

func TestGameReplacementConstructionFailureLeavesActiveSceneUntouched(t *testing.T) {
	events := make([]string, 0)
	oldScene := &lifecycleTestScene{name: "old", events: &events}
	game := NewGame(oldScene)
	events = events[:0]
	generation := game.DebugSceneGeneration()

	wantErr := errors.New("candidate failed")
	err := game.ReplaceScene(func() (Scene, error) {
		events = append(events, "candidate.construct")
		return nil, wantErr
	})

	require.ErrorIs(t, err, wantErr)
	assert.Same(t, oldScene, game.currentScene)
	assert.Equal(t, generation, game.DebugSceneGeneration())
	assert.Equal(t, []string{"candidate.construct"}, events)
}

func TestGameReplacementActivatesCandidateBeforeDisposingOldScene(t *testing.T) {
	events := make([]string, 0)
	oldScene := &lifecycleTestScene{name: "old", events: &events}
	newScene := &lifecycleTestScene{name: "new", events: &events}
	game := NewGame(oldScene)
	events = events[:0]

	require.NoError(t, game.ReplaceScene(func() (Scene, error) {
		events = append(events, "new.construct")
		return newScene, nil
	}))

	assert.Same(t, newScene, game.currentScene)
	assert.Equal(t, []string{
		"new.construct",
		"new.prepare-input",
		"new.activate",
		"old.prepare",
		"old.dispose",
	}, events)
}

func TestGameReplacementActivationFailureLeavesOldSceneUnprepared(t *testing.T) {
	for _, test := range []struct {
		name      string
		candidate *lifecycleTestScene
	}{
		{name: "preflight error", candidate: &lifecycleTestScene{preflightErr: errors.New("not ready")}},
		{name: "activation panic", candidate: &lifecycleTestScene{activationPanic: "activation failed"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			events := make([]string, 0)
			oldScene := &lifecycleTestScene{name: "old", events: &events}
			test.candidate.name, test.candidate.events = "new", &events
			game := NewGame(oldScene)
			events = events[:0]
			generation := game.DebugSceneGeneration()

			err := game.ReplaceScene(func() (Scene, error) { return test.candidate, nil })

			require.Error(t, err)
			assert.Same(t, oldScene, game.currentScene)
			assert.Equal(t, generation, game.DebugSceneGeneration())
			assert.Equal(t, 1, countEqual(events, "new.dispose"))
			assert.NotContains(t, events, "old.prepare")
			assert.NotContains(t, events, "old.dispose")
		})
	}
}

func TestGameReplacementPreservesBridgeAndRejectsQueuedStaleRequest(t *testing.T) {
	events := make([]string, 0)
	oldScene := &lifecycleTestScene{name: "old", events: &events}
	newScene := &lifecycleTestScene{name: "new", events: &events}
	game := NewGame(oldScene)
	staleGeneration := game.DebugSceneGeneration()
	require.NoError(t, game.SubmitDebugRequest(debugging.Request{
		SessionID:               "test-session",
		ID:                      "queued-before-replacement",
		CommandID:               debugging.CommandEnvironmentWindStatus,
		ExpectedSceneGeneration: staleGeneration,
	}))

	require.NoError(t, game.ReplaceScene(func() (Scene, error) { return newScene, nil }))
	require.NoError(t, game.Update())
	result, ok := game.NextDebugResult()

	require.True(t, ok)
	assert.Equal(t, "stale_scene", result.ErrorCode)
	assert.False(t, oldScene.executed)
	assert.False(t, newScene.executed)
	assert.Equal(t, 1, newScene.updates)
	assert.Equal(t, 1, countEqual(events, "old.dispose"))
}

func TestGameApplicationQueryIgnoresSceneGeneration(t *testing.T) {
	game := NewGame(&noDebugDispatchTestScene{})
	staleGeneration := game.DebugSceneGeneration()
	game.replaceScene(&noDebugDispatchTestScene{})
	require.NoError(t, game.SubmitDebugRequest(debugging.Request{
		SessionID:               "test-session",
		ID:                      "request-1",
		CommandID:               debugging.CommandLoggingStatus,
		ExpectedSceneGeneration: staleGeneration,
	}))

	require.NoError(t, game.Update())
	result, ok := game.NextDebugResult()

	require.True(t, ok)
	assert.True(t, result.Success)
}

func TestGameSnapshotsNormalizedInvocationBeforeExecution(t *testing.T) {
	scene := &debugDispatchTestScene{mutateArguments: true}
	game := NewGame(scene)
	require.NoError(t, game.SubmitDebugRequest(debugging.Request{
		SessionID: "test-session",
		ID:        "wind-set",
		CommandID: debugging.CommandEnvironmentWindSet,
		Arguments: map[string]json.RawMessage{"direction": json.RawMessage(`"north"`)},
	}))

	require.NoError(t, game.Update())
	result, ok := game.NextDebugResult()

	require.True(t, ok)
	require.NotNil(t, result.Invocation)
	assert.Equal(t, debugging.CommandEnvironmentWindSet, result.Invocation.CommandID)
	assert.JSONEq(t, `"north"`, string(result.Invocation.Arguments["direction"]))
	assert.NotContains(t, result.Invocation.Arguments, "executorOnly")
}

func TestGameBridgeDispatchesToReplacementScene(t *testing.T) {
	oldScene := &debugDispatchTestScene{}
	newScene := &debugDispatchTestScene{}
	game := NewGame(oldScene)
	require.NoError(t, game.SubmitDebugRequest(debugging.Request{
		SessionID: "test-session",
		ID:        "request-1",
		CommandID: debugging.CommandEnvironmentWindStatus,
	}))
	game.replaceScene(newScene)

	require.NoError(t, game.Update())

	assert.False(t, oldScene.executed)
	assert.True(t, newScene.executed)
}

func TestGamePublishesDetachedDynamicOptionsFromUpdate(t *testing.T) {
	scene := &debugDispatchTestScene{optionSets: []debugging.OptionSet{{
		Provider:          debugging.OptionProviderActivePartyCharacters,
		Choices:           []debugging.Choice{{Value: "0.QXZhdGFy", Label: "Position 1 · Avatar", Metadata: nil}},
		UnavailableReason: "",
	}}}
	game := NewGame(scene)
	game.debugDiscoveryPolling.Store(true)
	require.Empty(t, game.DebugDiscoverySnapshot().OptionSets)

	require.NoError(t, game.Update())
	discovery := game.DebugDiscoverySnapshot()

	require.Len(t, discovery.OptionSets, 1)
	assert.Equal(t, "Position 1 · Avatar", discovery.OptionSets[0].Choices[0].Label)
	discovery.OptionSets[0].Choices[0].Label = "mutated"
	assert.Equal(t, "Position 1 · Avatar", game.DebugDiscoverySnapshot().OptionSets[0].Choices[0].Label)
	publication := game.debugDiscovery.Load()
	require.NoError(t, game.Update())
	assert.Same(t, publication, game.debugDiscovery.Load(), "unchanged choices retain immutable publication")
	scene.optionSets[0].Choices[0].Label = "provider changed"
	assert.Equal(t, "Position 1 · Avatar", publication.OptionSets[0].Choices[0].Label)
}

func TestGameRejectsDynamicCommandPreparedAgainstStaleOptions(t *testing.T) {
	scene := &debugDispatchTestScene{optionSets: []debugging.OptionSet{{
		Provider:          debugging.OptionProviderActivePartyCharacters,
		Choices:           []debugging.Choice{{Value: "0.QXZhdGFy", Label: "Position 1 · Avatar", Metadata: nil}},
		UnavailableReason: "",
	}}}
	game := NewGame(scene)
	game.refreshDebugDiscoverySnapshot()
	require.NoError(t, game.Update())
	staleRevision := game.DebugDiscoverySnapshot().OptionsRevision
	scene.optionSets[0].Choices[0].Label = "Position 1 · Changed"
	require.NoError(t, game.SubmitDebugRequest(debugging.Request{
		SessionID:               "test-session",
		ID:                      "party-update",
		CommandID:               debugging.CommandPartyCharacterUpdate,
		ExpectedSceneGeneration: game.DebugSceneGeneration(),
		ExpectedOptionsRevision: staleRevision,
		Arguments: map[string]json.RawMessage{
			"character": json.RawMessage(`"0.QXZhdGFy"`),
			"strength":  json.RawMessage(`20`),
		},
	}))

	require.NoError(t, game.Update())
	result, ok := game.NextDebugResult()

	require.True(t, ok)
	assert.False(t, result.Success)
	assert.Equal(t, "stale_options", result.ErrorCode)
	require.NotNil(t, result.Invocation)
	assert.Equal(t, debugging.CommandPartyCharacterUpdate, result.Invocation.CommandID)
	assert.JSONEq(t, `20`, string(result.Invocation.Arguments["strength"]))
	assert.False(t, scene.executed)
}

func TestNewGameRequiresInitialScene(t *testing.T) {
	assert.Panics(t, func() {
		NewGame(nil)
	})
}

func TestGameDiscoveryIsDormantWithoutWorkbench(t *testing.T) {
	scene := &debugDispatchTestScene{}
	game := NewGame(scene)
	publication := game.debugDiscovery.Load()
	for range 60 {
		require.NoError(t, game.Update())
	}
	assert.Zero(t, scene.optionReads, "idle frames must not build developer options")
	assert.Same(t, publication, game.debugDiscovery.Load())
	request := debugging.Request{SessionID: "local", ID: "query", CommandID: debugging.CommandEnvironmentWindStatus}
	require.NoError(t, game.SubmitDebugRequest(request))
	require.NoError(t, game.Update())
	result, ok := game.NextDebugResult()
	require.True(t, ok)
	assert.True(t, result.Success, "bridge commands still work without a server")
	assert.Positive(t, scene.optionReads)
}

func TestGameDiscoverySharesStaticDefinitionsAndRetiresDormantReviews(t *testing.T) {
	scene := &debugDispatchTestScene{optionSets: []debugging.OptionSet{{
		Provider: debugging.OptionProviderActivePartyCharacters,
		Choices:  []debugging.Choice{{Value: "0.QXZhdGFy", Label: "Avatar", Metadata: json.RawMessage(`{"hp":100}`)}},
	}}}
	game := NewGame(scene)
	game.debugDiscoveryPolling.Store(true)
	require.NoError(t, game.Update())
	first := game.debugDiscovery.Load()
	assert.Same(t, &game.debugDefinitions[0], &first.Commands[0])
	detached := game.DebugDiscoverySnapshot()
	detached.Commands[0].Name = "mutated"
	detached.OptionSets[0].Choices[0].Metadata[1] = 'X'
	assert.NotEqual(t, "mutated", game.DebugDiscoverySnapshot().Commands[0].Name)
	assert.JSONEq(t, `{"hp":100}`, string(first.OptionSets[0].Choices[0].Metadata))
	scene.optionSets[0].Choices[0].Metadata = json.RawMessage(`{"hp":90}`)
	require.NoError(t, game.Update())
	second := game.debugDiscovery.Load()
	assert.Greater(t, second.OptionsRevision, first.OptionsRevision)
	assert.Same(t, &first.Commands[0], &second.Commands[0])
	assert.JSONEq(t, `{"hp":100}`, string(first.OptionSets[0].Choices[0].Metadata))
	game.stopDebugDiscoveryPolling()
	reads := scene.optionReads
	require.NoError(t, game.Update())
	assert.Equal(t, reads, scene.optionReads)
	result := game.ExecuteLocalDebugCommand(debugging.Request{
		SessionID: "local", ID: "retired-review", CommandID: debugging.CommandPartyCharacterUpdate,
		ExpectedOptionsRevision: second.OptionsRevision,
		Arguments:               map[string]json.RawMessage{"character": json.RawMessage(`"0.QXZhdGFy"`), "strength": json.RawMessage(`20`)},
	})
	assert.Equal(t, "stale_options", result.ErrorCode, "reviews cannot cross a suspended publication interval")
	assert.False(t, scene.executed)
}
