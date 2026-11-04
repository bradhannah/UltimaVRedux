package main

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
)

type snapshotCommandTestRepository struct {
	records   map[string]save_snapshot.Record
	events    *[]string
	listErr   error
	createErr error
}

func (r *snapshotCommandTestRepository) List() ([]save_snapshot.Summary, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	summaries := make([]save_snapshot.Summary, 0, len(r.records))
	for _, record := range r.records {
		summaries = append(summaries, save_snapshot.Summary{Reference: record.Reference, Metadata: record.Envelope.Metadata, Revision: record.Revision})
	}
	return summaries, nil
}

func (r *snapshotCommandTestRepository) Load(reference string) (save_snapshot.Record, error) {
	record, ok := r.records[reference]
	if !ok {
		return save_snapshot.Record{}, save_snapshot.ErrSnapshotNotFound
	}
	return record, nil
}

func (r *snapshotCommandTestRepository) Create(reference string, envelope save_snapshot.Envelope) (save_snapshot.Record, error) {
	if r.events != nil {
		*r.events = append(*r.events, "repository.create:"+reference)
	}
	if r.createErr != nil {
		return save_snapshot.Record{}, r.createErr
	}
	if _, exists := r.records[reference]; exists {
		return save_snapshot.Record{}, save_snapshot.ErrSnapshotExists
	}
	parsed, err := save_snapshot.ParseReference(reference)
	if err != nil {
		return save_snapshot.Record{}, err
	}
	record := save_snapshot.Record{Reference: parsed, Envelope: envelope, Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	r.records[reference] = record
	return record, nil
}

func (r *snapshotCommandTestRepository) Delete(reference, expectedRevision string) error {
	return r.DeleteIfUnreferenced(reference, expectedRevision, nil)
}
func (r *snapshotCommandTestRepository) DeleteIfUnreferenced(reference, expectedRevision string, check func() error) error {
	record, ok := r.records[reference]
	if !ok {
		return save_snapshot.ErrSnapshotNotFound
	}
	if record.Revision != expectedRevision {
		return save_snapshot.ErrSnapshotStale
	}
	if check != nil {
		if err := check(); err != nil {
			return err
		}
	}
	delete(r.records, reference)
	return nil
}

type snapshotCommandTestScene struct {
	playthrough   playthroughMetadata
	name          string
	events        *[]string
	snapshot      save_snapshot.SaveSnapshot
	compatibility error
	captures      int
	updates       int
}

func (s *snapshotCommandTestScene) setPlaythroughMetadata(metadata playthroughMetadata) {
	s.playthrough = metadata
}
func (s *snapshotCommandTestScene) playthroughMetadata() playthroughMetadata { return s.playthrough }

func (s *snapshotCommandTestScene) Update(_ *Game) error { s.updates++; return nil }
func (s *snapshotCommandTestScene) Draw(_ *ebiten.Image) {}
func (s *snapshotCommandTestScene) GetUltimaConfiguration() *config.UltimaVConfiguration {
	return nil
}
func (s *snapshotCommandTestScene) InvalidateResolution() {}
func (s *snapshotCommandTestScene) ValidateSaveSnapshotEligibility() error {
	return nil
}
func (s *snapshotCommandTestScene) ValidateSaveSnapshotCompatibility(_ save_snapshot.SaveSnapshot) error {
	return s.compatibility
}
func (s *snapshotCommandTestScene) CaptureSaveSnapshot() (save_snapshot.SaveSnapshot, error) {
	s.captures++
	return s.snapshot, nil
}
func (s *snapshotCommandTestScene) Activate() {
	if s.events != nil {
		*s.events = append(*s.events, s.name+".activate")
	}
}
func (s *snapshotCommandTestScene) PrepareForSceneReplacement() {
	if s.events != nil {
		*s.events = append(*s.events, s.name+".prepare")
	}
}
func (s *snapshotCommandTestScene) Dispose() {
	if s.events != nil {
		*s.events = append(*s.events, s.name+".dispose")
	}
}

func TestSnapshotCommandsCaptureListGetAndDeleteLifecycle(t *testing.T) {
	snapshot := validCommandTestSnapshot()
	snapshot.Redux.PlayerKnowledge = []save_snapshot.PlayerKnowledgeSnapshot{validCommandTestLore()}
	scene := &snapshotCommandTestScene{snapshot: snapshot}
	repository := &snapshotCommandTestRepository{records: make(map[string]save_snapshot.Record)}
	game := NewGame(scene)
	game.ConfigureSnapshotRepository(repository, nil)
	game.snapshotNow = func() time.Time { return time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC) }

	capture := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotCapture, map[string]any{
		"source": "project", "id": "stable-point", "name": "Stable Point", "description": "Before testing.",
	})
	require.True(t, capture.Success, capture.ErrorMessage)
	assert.Equal(t, debugging.ExecutionAlteration, capture.EffectiveExecutionMode)
	assert.Equal(t, 1, scene.captures)
	require.Contains(t, repository.records, "project:stable-point")
	assert.Equal(t, snapshot.Redux.PlayerKnowledge, repository.records["project:stable-point"].Envelope.Snapshot.Redux.PlayerKnowledge)

	list := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotList, nil)
	require.True(t, list.Success, list.ErrorMessage)
	var listData debugging.SaveSnapshotListData
	require.NoError(t, json.Unmarshal(list.Data, &listData))
	require.Len(t, listData.Snapshots, 1)
	assert.True(t, listData.CaptureEligibility.CanCapture)
	assert.True(t, listData.Snapshots[0].Compatibility.Compatible)
	assert.True(t, listData.Snapshots[0].Eligibility.CanCapture)
	assert.True(t, listData.Snapshots[0].Eligibility.CanDelete)
	assert.NotContains(t, string(list.Data), `"party"`)

	get := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotGet, map[string]any{"reference": "project:stable-point"})
	require.True(t, get.Success, get.ErrorMessage)
	assert.NotContains(t, string(get.Data), `"inventory"`)
	assert.NotContains(t, string(get.Data), `"playerKnowledge"`)
	var getData debugging.SaveSnapshotGetData
	require.NoError(t, json.Unmarshal(get.Data, &getData))
	assert.Equal(t, references.Britain.String(), getData.Snapshot.LocationName)

	revision := repository.records["project:stable-point"].Revision
	stale := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotDelete, map[string]any{"reference": "project:stable-point", "expectedRevision": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	assert.False(t, stale.Success)
	assert.Equal(t, snapshotStaleError, stale.ErrorCode)
	require.Contains(t, repository.records, "project:stable-point")
	deleted := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotDelete, map[string]any{"reference": "project:stable-point", "expectedRevision": revision})
	assert.True(t, deleted.Success, deleted.ErrorMessage)
	assert.NotContains(t, repository.records, "project:stable-point")
}

func TestSnapshotCompatibilityDisablesImpossibleLoads(t *testing.T) {
	target := snapshotCommandTestRecord(t, "project:incompatible", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	repository := &snapshotCommandTestRepository{records: map[string]save_snapshot.Record{target.Reference.String(): target}}
	scene := &snapshotCommandTestScene{snapshot: validCommandTestSnapshot(), compatibility: errors.New("monster reference 999 is unavailable")}
	game := NewGame(scene)
	game.ConfigureSnapshotRepository(repository, func(_ *config.UltimaVConfiguration, _ save_snapshot.SaveSnapshot) (Scene, error) {
		return &snapshotCommandTestScene{}, nil
	})

	list := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotList, nil)
	require.True(t, list.Success, list.ErrorMessage)
	var data debugging.SaveSnapshotListData
	require.NoError(t, json.Unmarshal(list.Data, &data))
	require.Len(t, data.Snapshots, 1)
	assert.False(t, data.Snapshots[0].Compatibility.Compatible)
	assert.Contains(t, data.Snapshots[0].Compatibility.Reason, "monster reference")
	assert.False(t, data.Snapshots[0].Eligibility.CanLoad)

	load := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{
		"reference": target.Reference.String(), "expectedRevision": target.Revision,
	})
	assert.Equal(t, snapshotIneligibleError, load.ErrorCode)
	assert.Zero(t, scene.captures, "incompatible load must fail before rollback capture")
}

func TestSnapshotLoadPersistsRollbackBeforeBuildingAndAtomicallyReplacesScene(t *testing.T) {
	events := make([]string, 0)
	oldSnapshot := validCommandTestSnapshot()
	oldSnapshot.Redux.PlayerKnowledge = []save_snapshot.PlayerKnowledgeSnapshot{validCommandTestLore()}
	oldScene := &snapshotCommandTestScene{name: "old", events: &events, snapshot: oldSnapshot}
	target := snapshotCommandTestRecord(t, "personal:target", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	targetLore := validCommandTestLore()
	targetLore.Key = save_snapshot.ShrineKnowledgeHonestyMantra
	targetLore.Category = save_snapshot.KnowledgeCategoryShrine
	target.Envelope.Snapshot.Redux.PlayerKnowledge = []save_snapshot.PlayerKnowledgeSnapshot{targetLore}
	repository := &snapshotCommandTestRepository{records: map[string]save_snapshot.Record{target.Reference.String(): target}, events: &events}
	game := NewGame(oldScene)
	events = events[:0]
	newScene := &snapshotCommandTestScene{name: "new", events: &events, snapshot: target.Envelope.Snapshot}
	var builtSnapshot save_snapshot.SaveSnapshot
	game.ConfigureSnapshotRepository(repository, func(_ *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
		events = append(events, "candidate.build")
		builtSnapshot = snapshot
		return newScene, nil
	})
	game.snapshotNow = func() time.Time { return time.Date(2026, 8, 6, 12, 30, 0, 0, time.UTC) }
	beforeGeneration := game.DebugSceneGeneration()

	result := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{"reference": target.Reference.String(), "expectedRevision": target.Revision})

	require.True(t, result.Success, result.ErrorMessage)
	assert.Same(t, newScene, game.currentScene)
	assert.Equal(t, beforeGeneration+1, game.DebugSceneGeneration())
	require.GreaterOrEqual(t, len(events), 5)
	assert.Contains(t, events[0], "repository.create:rollback:")
	assert.Equal(t, "candidate.build", events[1])
	assert.Equal(t, []string{"new.activate", "old.prepare", "old.dispose"}, events[2:])
	assert.Equal(t, target.Envelope.Snapshot.Redux.PlayerKnowledge, builtSnapshot.Redux.PlayerKnowledge)
	var data debugging.SaveSnapshotLoadData
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.Contains(t, data.RollbackReference, "rollback:")
	assert.Equal(t, game.DebugSceneGeneration(), data.SceneGeneration)
	assert.Equal(t, oldSnapshot.Redux.PlayerKnowledge, repository.records[data.RollbackReference].Envelope.Snapshot.Redux.PlayerKnowledge)
}

func TestSnapshotLoadStaleRevisionAndBuildFailureLeaveActiveScene(t *testing.T) {
	target := snapshotCommandTestRecord(t, "project:target", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	for _, test := range []struct {
		name             string
		revision         string
		buildErr         error
		wantCode         string
		wantRollback     bool
		wantBuilderCalls int
	}{
		{name: "stale revision", revision: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", wantCode: snapshotStaleError},
		{name: "candidate failure", revision: target.Revision, buildErr: errors.New("candidate failed"), wantCode: snapshotBuildError, wantRollback: true, wantBuilderCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			oldScene := &snapshotCommandTestScene{snapshot: validCommandTestSnapshot()}
			repository := &snapshotCommandTestRepository{records: map[string]save_snapshot.Record{target.Reference.String(): target}}
			game := NewGame(oldScene)
			builderCalls := 0
			game.ConfigureSnapshotRepository(repository, func(_ *config.UltimaVConfiguration, _ save_snapshot.SaveSnapshot) (Scene, error) {
				builderCalls++
				return nil, test.buildErr
			})
			generation := game.DebugSceneGeneration()

			result := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{"reference": target.Reference.String(), "expectedRevision": test.revision})

			assert.False(t, result.Success)
			assert.Equal(t, test.wantCode, result.ErrorCode)
			assert.Same(t, oldScene, game.currentScene)
			assert.Equal(t, generation, game.DebugSceneGeneration())
			assert.Equal(t, test.wantBuilderCalls, builderCalls)
			rollbackCount := 0
			for reference := range repository.records {
				if len(reference) > len("rollback:") && reference[:len("rollback:")] == "rollback:" {
					rollbackCount++
				}
			}
			if test.wantRollback {
				assert.Equal(t, 1, rollbackCount)
				require.Len(t, result.Changes, 1)
				assert.Contains(t, result.Changes[0].Path, "saveSnapshots.rollback:")
				var data debugging.SaveSnapshotLoadData
				require.NoError(t, json.Unmarshal(result.Data, &data))
				assert.NotEmpty(t, data.RollbackReference)
			} else {
				assert.Zero(t, rollbackCount)
			}
		})
	}
}

func TestSnapshotLoadRollbackFailureDoesNotBuildOrReplaceScene(t *testing.T) {
	target := snapshotCommandTestRecord(t, "project:target", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	oldScene := &snapshotCommandTestScene{snapshot: validCommandTestSnapshot()}
	repository := &snapshotCommandTestRepository{
		records:   map[string]save_snapshot.Record{target.Reference.String(): target},
		createErr: errors.New("rollback disk full"),
	}
	game := NewGame(oldScene)
	built := false
	game.ConfigureSnapshotRepository(repository, func(_ *config.UltimaVConfiguration, _ save_snapshot.SaveSnapshot) (Scene, error) {
		built = true
		return &snapshotCommandTestScene{}, nil
	})
	generation := game.DebugSceneGeneration()

	result := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{"reference": target.Reference.String(), "expectedRevision": target.Revision})

	assert.Equal(t, snapshotRollbackError, result.ErrorCode)
	assert.False(t, built)
	assert.Same(t, oldScene, game.currentScene)
	assert.Equal(t, generation, game.DebugSceneGeneration())
}

func TestSnapshotCaptureAndLoadRequireExpectedSceneGeneration(t *testing.T) {
	target := snapshotCommandTestRecord(t, "project:target", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	scene := &snapshotCommandTestScene{snapshot: validCommandTestSnapshot()}
	repository := &snapshotCommandTestRepository{records: map[string]save_snapshot.Record{target.Reference.String(): target}}
	game := NewGame(scene)
	game.ConfigureSnapshotRepository(repository, func(_ *config.UltimaVConfiguration, _ save_snapshot.SaveSnapshot) (Scene, error) { return scene, nil })

	for _, request := range []debugging.Request{
		{SessionID: "test", ID: "capture", CommandID: debugging.CommandSaveSnapshotCapture, Arguments: rawSnapshotTestArguments(t, map[string]any{"source": "personal", "id": "missing-generation", "name": "Missing"})},
		{SessionID: "test", ID: "load", CommandID: debugging.CommandSaveSnapshotLoad, Arguments: rawSnapshotTestArguments(t, map[string]any{"reference": target.Reference.String(), "expectedRevision": target.Revision})},
	} {
		result := game.ExecuteLocalDebugCommand(request)
		assert.Equal(t, "stale_scene", result.ErrorCode)
	}
	assert.Zero(t, scene.captures)
}

func TestSnapshotCommandsMapRepositoryErrorsAndDispatchOnUpdateThread(t *testing.T) {
	scene := &snapshotCommandTestScene{snapshot: validCommandTestSnapshot()}
	repository := &snapshotCommandTestRepository{records: make(map[string]save_snapshot.Record), listErr: errors.New("disk unavailable")}
	game := NewGame(scene)
	game.ConfigureSnapshotRepository(repository, nil)
	require.NoError(t, game.SubmitDebugRequest(debugging.Request{SessionID: "test", ID: "list", CommandID: debugging.CommandSaveSnapshotList}))

	require.NoError(t, game.Update())
	result, ok := game.NextDebugResult()
	require.True(t, ok)
	assert.Equal(t, snapshotReadError, result.ErrorCode)
	assert.Equal(t, 1, scene.updates)

	repository.listErr = nil
	repository.createErr = errors.New("read-only filesystem")
	failedCapture := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotCapture, map[string]any{"source": "personal", "id": "failure", "name": "Failure"})
	assert.Equal(t, snapshotCaptureError, failedCapture.ErrorCode)
}

func executeSnapshotTestCommand(t *testing.T, game *Game, commandID string, arguments map[string]any) debugging.Result {
	t.Helper()
	rawArguments := rawSnapshotTestArguments(t, arguments)
	return game.ExecuteLocalDebugCommand(debugging.Request{
		SessionID: "test", ID: commandID, CommandID: commandID,
		ExpectedSceneGeneration: game.DebugSceneGeneration(), Arguments: rawArguments,
	})
}

func rawSnapshotTestArguments(t *testing.T, arguments map[string]any) map[string]json.RawMessage {
	t.Helper()
	rawArguments := make(map[string]json.RawMessage, len(arguments))
	for key, value := range arguments {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		rawArguments[key] = encoded
	}
	return rawArguments
}

func snapshotCommandTestRecord(t *testing.T, reference, revision string) save_snapshot.Record {
	t.Helper()
	parsed, err := save_snapshot.ParseReference(reference)
	require.NoError(t, err)
	return save_snapshot.Record{
		Reference: parsed,
		Envelope: save_snapshot.Envelope{
			Format: save_snapshot.EnvelopeFormat, EnvelopeVersion: save_snapshot.CurrentEnvelopeVersion,
			Metadata: save_snapshot.Metadata{Name: "Target", CreatedAt: time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC), GameVersion: "test"},
			Snapshot: validCommandTestSnapshot(),
		},
		Revision: revision,
	}
}

func validCommandTestSnapshot() save_snapshot.SaveSnapshot {
	snapshot := save_snapshot.SaveSnapshot{
		Version:      save_snapshot.CurrentVersion,
		Presentation: journal.EmptyPresentation(),
		Continuation: save_snapshot.ContinuationSnapshot{ActivePlayerIndex: -1},
		Redux: save_snapshot.ReduxSnapshot{
			DungeonDiscoveries: make([]save_snapshot.DungeonDiscoverySnapshot, 0),
			PlayerKnowledge:    make([]save_snapshot.PlayerKnowledgeSnapshot, 0),
			QuestJournal:       make([]save_snapshot.QuestJournalEventSnapshot, 0),
		},
	}
	snapshot.Party.Roster[0] = save_snapshot.CharacterSnapshot{
		Occupied: true, Name: "Avatar", Gender: int(party_state.Male), Class: int(party_state.Avatar),
		Status: int(party_state.Good), Level: 1, MaximumHP: 30,
	}
	snapshot.World.Position = save_snapshot.PositionSnapshot{Location: int(references.Britain), X: 12, Y: 17}
	snapshot.World.Calendar = save_snapshot.CalendarSnapshot{Year: 1, Month: 1, Day: 1}
	snapshot.Progression.Shadowlords.LastCalled = -1
	snapshot.Progression.Shadowlords.AutomaticPresence = -1
	return snapshot
}

func validCommandTestLore() save_snapshot.PlayerKnowledgeSnapshot {
	return save_snapshot.PlayerKnowledgeSnapshot{
		Category: save_snapshot.KnowledgeCategoryShadowlord,
		Key:      save_snapshot.ShadowlordKnowledgeAstarothName,
		Source: save_snapshot.KnowledgeSourceSnapshot{
			Kind: save_snapshot.KnowledgeSourceDialogue, DisplayName: "Savant",
		},
		DiscoveredOn: &save_snapshot.BritannianDateSnapshot{Year: 1, Month: 1, Day: 1},
	}
}
