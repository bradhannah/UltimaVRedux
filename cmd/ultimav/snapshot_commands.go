package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
)

const (
	snapshotRepositoryUnavailableError = "snapshot_repository_unavailable"
	snapshotNotFoundError              = "snapshot_not_found"
	snapshotExistsError                = "snapshot_exists"
	snapshotStaleError                 = "stale_snapshot_revision"
	snapshotIneligibleError            = "snapshot_ineligible"
	snapshotReadError                  = "snapshot_read_failed"
	snapshotCaptureError               = "snapshot_capture_failed"
	snapshotRollbackError              = "snapshot_rollback_failed"
	snapshotBuildError                 = "snapshot_build_failed"
	snapshotDeleteError                = "snapshot_delete_failed"
)

type snapshotRepository interface {
	List() ([]save_snapshot.Summary, error)
	Load(reference string) (save_snapshot.Record, error)
	Create(reference string, envelope save_snapshot.Envelope) (save_snapshot.Record, error)
	Delete(reference, expectedRevision string) error
}

type snapshotCapableScene interface {
	playthroughMetadataOwner
	ValidateSaveSnapshotEligibility() error
	CaptureSaveSnapshot() (save_snapshot.SaveSnapshot, error)
}

type snapshotCompatibilityValidator interface {
	ValidateSaveSnapshotCompatibility(snapshot save_snapshot.SaveSnapshot) error
}

func (g *Game) executeSnapshotQueryCommand(request debugging.Request) (debugging.Result, bool) {
	switch request.CommandID {
	case debugging.CommandSaveSnapshotList:
		return g.listSnapshots(request), true
	case debugging.CommandSaveSnapshotGet:
		return g.getSnapshot(request), true
	default:
		return debugging.Result{}, false
	}
}

func (g *Game) executeSnapshotMutationCommand(request debugging.Request) (debugging.Result, bool) {
	switch request.CommandID {
	case debugging.CommandSaveSnapshotCapture:
		return g.captureSnapshot(request), true
	case debugging.CommandSaveSnapshotLoad:
		return g.loadSnapshot(request), true
	case debugging.CommandSaveSnapshotDelete:
		return g.deleteSnapshot(request), true
	default:
		return debugging.Result{}, false
	}
}

func (g *Game) listSnapshots(request debugging.Request) debugging.Result {
	if g.snapshotRepository == nil {
		return debugging.FailureResult(request, snapshotRepositoryUnavailableError, "the save snapshot repository is unavailable")
	}
	records, err := g.snapshotRepository.List()
	if err != nil {
		return snapshotRepositoryFailure(request, snapshotReadError, err)
	}
	summaries := make([]debugging.SaveSnapshotSummary, len(records))
	for index, record := range records {
		loaded, loadErr := g.snapshotRepository.Load(record.Reference.String())
		if loadErr != nil {
			return snapshotRepositoryFailure(request, snapshotReadError, loadErr)
		}
		summaries[index] = g.snapshotSummary(record.Reference, record.Metadata, record.Revision, loaded.Envelope.Snapshot)
	}

	return debugSuccessResult(request, "Save snapshots loaded.", debugging.SaveSnapshotListData{
		Snapshots: summaries, CaptureEligibility: g.snapshotCaptureEligibility(),
	})
}

func (g *Game) getSnapshot(request debugging.Request) debugging.Result {
	if g.snapshotRepository == nil {
		return debugging.FailureResult(request, snapshotRepositoryUnavailableError, "the save snapshot repository is unavailable")
	}
	record, err := g.snapshotRepository.Load(mustDebugStringArgument(request, "reference"))
	if err != nil {
		return snapshotRepositoryFailure(request, snapshotReadError, err)
	}
	detail := snapshotDetail(g.snapshotSummary(record.Reference, record.Envelope.Metadata, record.Revision, record.Envelope.Snapshot), record.Envelope.Snapshot)

	return debugSuccessResult(request, "Save snapshot loaded.", debugging.SaveSnapshotGetData{Snapshot: detail})
}

func (g *Game) captureSnapshot(request debugging.Request) debugging.Result {
	if request.ExpectedSceneGeneration == 0 {
		return debugging.FailureResult(request, "stale_scene", "an expected scene generation is required for gameplay capture")
	}
	if g.snapshotRepository == nil {
		return debugging.FailureResult(request, snapshotRepositoryUnavailableError, "the save snapshot repository is unavailable")
	}
	scene, ok := g.currentScene.(snapshotCapableScene)
	if !ok {
		return debugging.FailureResult(request, snapshotIneligibleError, "the active scene cannot be captured")
	}
	snapshot, err := scene.CaptureSaveSnapshot()
	if err != nil {
		return debugging.FailureResult(request, snapshotIneligibleError, err.Error())
	}
	reference := mustDebugStringArgument(request, "source") + ":" + mustDebugStringArgument(request, "id")
	envelope := g.newSnapshotEnvelope(
		mustDebugStringArgument(request, "name"),
		mustDebugStringArgument(request, "description"),
		snapshot,
	)
	record, err := g.snapshotRepository.Create(reference, envelope)
	if err != nil {
		return snapshotRepositoryFailure(request, snapshotCaptureError, err)
	}
	result := debugSuccessResultWithMode(request, debugging.ExecutionAlteration, "Save snapshot captured.", debugging.SaveSnapshotCaptureData{
		Snapshot: g.snapshotSummary(record.Reference, record.Envelope.Metadata, record.Revision, record.Envelope.Snapshot),
	})
	result.Changes = []debugging.StateChange{{Path: "saveSnapshots." + record.Reference.String(), After: mustJSON(record.Revision)}}

	return result
}

func (g *Game) loadSnapshot(request debugging.Request) debugging.Result {
	if request.ExpectedSceneGeneration == 0 {
		return debugging.FailureResult(request, "stale_scene", "an expected scene generation is required for gameplay load")
	}
	if g.snapshotRepository == nil || g.snapshotSceneBuilder == nil {
		return debugging.FailureResult(request, snapshotRepositoryUnavailableError, "save snapshot loading is unavailable")
	}
	reference := mustDebugStringArgument(request, "reference")
	record, err := g.snapshotRepository.Load(reference)
	if err != nil {
		return snapshotRepositoryFailure(request, snapshotReadError, err)
	}
	if record.Revision != mustDebugStringArgument(request, "expectedRevision") {
		return debugging.FailureResult(request, snapshotStaleError, save_snapshot.ErrSnapshotStale.Error())
	}
	active, ok := g.currentScene.(snapshotCapableScene)
	if !ok {
		return debugging.FailureResult(request, snapshotIneligibleError, "the active scene cannot provide an automatic rollback")
	}
	validator, ok := g.currentScene.(snapshotCompatibilityValidator)
	if !ok {
		return debugging.FailureResult(request, snapshotIneligibleError, "the active scene cannot verify runtime snapshot references")
	}
	snapshot, err := g.prepareExcerptMigration(record.Envelope)
	if err != nil {
		return snapshotRepositoryFailure(request, snapshotReadError, err)
	}
	if err = validator.ValidateSaveSnapshotCompatibility(snapshot); err != nil {
		return debugging.FailureResult(request, snapshotIneligibleError, "snapshot is incompatible with the active runtime: "+err.Error())
	}
	rollbackSnapshot, err := active.CaptureSaveSnapshot()
	if err != nil {
		return debugging.FailureResult(request, snapshotIneligibleError, err.Error())
	}
	rollback, err := g.createAutomaticRollback(reference, rollbackSnapshot)
	if err != nil {
		return snapshotRepositoryFailure(request, snapshotRollbackError, err)
	}
	configuration := g.currentScene.GetUltimaConfiguration()
	candidate, err := g.snapshotSceneBuilder(configuration, snapshot)
	if err != nil {
		return snapshotLoadFailureAfterRollback(request, record, rollback, err.Error(), g.sceneGeneration.Load())
	}
	if candidate == nil {
		return snapshotLoadFailureAfterRollback(request, record, rollback, "snapshot scene builder returned no candidate", g.sceneGeneration.Load())
	}
	metadata := playthroughMetadataFromEnvelope(record.Envelope.Metadata)
	if metadata.id == "" {
		metadata, err = freshPlaythroughMetadata(g.playthroughIDProvider)
	}
	if err == nil {
		err = transferScenePlaythrough(candidate, metadata)
	}
	if err != nil {
		if disposer, ok := candidate.(sceneDisposer); ok {
			disposer.Dispose()
		}

		return snapshotLoadFailureAfterRollback(request, record, rollback, err.Error(), g.sceneGeneration.Load())
	}
	beforeGeneration := g.sceneGeneration.Load()
	if err = g.commitSceneReplacement(candidate); err != nil {
		return snapshotLoadFailureAfterRollback(request, record, rollback, err.Error(), g.sceneGeneration.Load())
	}
	result := debugSuccessResultWithMode(request, debugging.ExecutionSceneRestart, "Save snapshot loaded.", debugging.SaveSnapshotLoadData{
		Reference: reference, Revision: record.Revision,
		RollbackReference: rollback.Reference.String(), RollbackRevision: rollback.Revision,
		SceneGeneration: g.sceneGeneration.Load(),
	})
	result.Changes = []debugging.StateChange{
		{Path: "scene.generation", Before: mustJSON(beforeGeneration), After: mustJSON(g.sceneGeneration.Load())},
		{Path: "saveSnapshot.loadedReference", After: mustJSON(reference)},
		{Path: "saveSnapshots." + rollback.Reference.String(), After: mustJSON(rollback.Revision)},
	}

	return result
}

func (g *Game) deleteSnapshot(request debugging.Request) debugging.Result {
	if g.snapshotRepository == nil {
		return debugging.FailureResult(request, snapshotRepositoryUnavailableError, "the save snapshot repository is unavailable")
	}
	reference := mustDebugStringArgument(request, "reference")
	revision := mustDebugStringArgument(request, "expectedRevision")
	if err := g.snapshotRepository.Delete(reference, revision); err != nil {
		return snapshotRepositoryFailure(request, snapshotDeleteError, err)
	}
	result := debugSuccessResultWithMode(request, debugging.ExecutionAlteration, "Save snapshot deleted.", debugging.SaveSnapshotDeleteData{Reference: reference, Revision: revision})
	result.Changes = []debugging.StateChange{{Path: "saveSnapshots." + reference, Before: mustJSON(revision)}}

	return result
}

func (g *Game) createAutomaticRollback(loadedReference string, snapshot save_snapshot.SaveSnapshot) (save_snapshot.Record, error) {
	now := g.snapshotNow().UTC()
	envelope := g.newSnapshotEnvelope("Automatic rollback", "Captured before loading "+loadedReference+".", snapshot)
	baseID := "rollback-" + now.Format("20060102t150405000000000")
	for attempt := range 1000 {
		reference := fmt.Sprintf("rollback:%s-%03d", baseID, attempt)
		record, err := g.snapshotRepository.Create(reference, envelope)
		if !errors.Is(err, save_snapshot.ErrSnapshotExists) {
			return record, err
		}
	}

	return save_snapshot.Record{}, errors.New("automatic rollback namespace exhausted")
}

func (g *Game) newSnapshotEnvelope(name, description string, snapshot save_snapshot.SaveSnapshot) save_snapshot.Envelope {
	owner, ok := g.currentScene.(playthroughMetadataOwner)
	if !ok {
		panic("Game.newSnapshotEnvelope: capture scene must own checkpoint metadata")
	}
	metadata := owner.playthroughMetadata()

	return save_snapshot.Envelope{
		Format: save_snapshot.EnvelopeFormat, EnvelopeVersion: save_snapshot.CurrentEnvelopeVersion,
		Metadata: save_snapshot.Metadata{Name: name, Description: description, CreatedAt: g.snapshotNow().UTC(), GameVersion: g.snapshotGameVersion,
			PlaythroughID: metadata.id, UserDescription: metadata.userDescription},
		Snapshot: snapshot,
	}
}

func (g *Game) snapshotSummary(reference save_snapshot.Reference, metadata save_snapshot.Metadata, revision string, snapshot save_snapshot.SaveSnapshot) debugging.SaveSnapshotSummary {
	namedSource := reference.Source != save_snapshot.SourceRollback
	eligibility := g.snapshotCaptureEligibility()
	eligibility.CanCapture = eligibility.CanCapture && namedSource
	eligibility.CanDelete = namedSource
	compatibility := debugging.SaveSnapshotCompatibility{Reason: "active scene cannot verify runtime snapshot references"}
	if scene, ok := g.currentScene.(snapshotCompatibilityValidator); ok {
		if err := scene.ValidateSaveSnapshotCompatibility(snapshot); err != nil {
			compatibility.Reason = err.Error()
		} else {
			compatibility = debugging.SaveSnapshotCompatibility{Compatible: true}
		}
	}
	if !compatibility.Compatible {
		eligibility.CanLoad = false
		if eligibility.Reason == "" {
			eligibility.Reason = compatibility.Reason
		}
	}

	return debugging.SaveSnapshotSummary{
		Reference: reference.String(), Source: string(reference.Source), ID: reference.ID,
		Name: metadata.Name, Description: metadata.Description, CreatedAt: metadata.CreatedAt.Format(time.RFC3339Nano),
		GameVersion: metadata.GameVersion, Revision: revision,
		Compatibility: compatibility, Eligibility: eligibility,
	}
}

func (g *Game) snapshotCaptureEligibility() debugging.SaveSnapshotEligibility {
	eligibility := debugging.SaveSnapshotEligibility{CanCapture: true, CanLoad: true}
	scene, ok := g.currentScene.(snapshotCapableScene)
	if !ok {
		eligibility.CanCapture = false
		eligibility.CanLoad = false
		eligibility.Reason = "active scene cannot provide snapshot or rollback state"

		return eligibility
	}
	if err := scene.ValidateSaveSnapshotEligibility(); err != nil {
		eligibility.CanCapture = false
		eligibility.CanLoad = false
		eligibility.Reason = err.Error()
	}

	return eligibility
}

func snapshotLoadFailureAfterRollback(
	request debugging.Request,
	record save_snapshot.Record,
	rollback save_snapshot.Record,
	message string,
	sceneGeneration uint64,
) debugging.Result {
	result := debugging.FailureResult(request, snapshotBuildError, message)
	data := debugging.SaveSnapshotLoadData{
		Reference: record.Reference.String(), Revision: record.Revision,
		RollbackReference: rollback.Reference.String(), RollbackRevision: rollback.Revision,
		SceneGeneration: sceneGeneration,
	}
	result.Data = mustJSON(data)
	result.Changes = []debugging.StateChange{{Path: "saveSnapshots." + rollback.Reference.String(), After: mustJSON(rollback.Revision)}}

	return result
}

func snapshotDetail(summary debugging.SaveSnapshotSummary, snapshot save_snapshot.SaveSnapshot) debugging.SaveSnapshotDetail {
	partyMembers := 0
	for _, character := range snapshot.Party.Roster {
		if character.Occupied && character.PartyMap == int(party_state.PartyMapActive) {
			partyMembers++
		}
	}
	position, calendar := snapshot.World.Position, snapshot.World.Calendar
	date := datetime.UltimaDate{Year: uint16(calendar.Year), Month: byte(calendar.Month), Day: byte(calendar.Day), Hour: byte(calendar.Hour), Minute: byte(calendar.Minute)}
	moonPhase := date.GetCurrentMoonPhase()

	return debugging.SaveSnapshotDetail{
		Summary: summary, SnapshotVersion: snapshot.Version,
		Location: position.Location, LocationName: references.Location(position.Location).String(),
		X: position.X, Y: position.Y, Floor: position.Floor,
		Year: calendar.Year, Month: calendar.Month, Day: calendar.Day, Hour: calendar.Hour, Minute: calendar.Minute,
		PartyMembers: partyMembers,
		MoonPhase:    moonPhase.String(), MoonPhaseIndex: int(moonPhase),
	}
}

func snapshotRepositoryFailure(request debugging.Request, fallback string, err error) debugging.Result {
	switch {
	case errors.Is(err, save_snapshot.ErrSnapshotNotFound):
		return debugging.FailureResult(request, snapshotNotFoundError, err.Error())
	case errors.Is(err, save_snapshot.ErrSnapshotExists):
		return debugging.FailureResult(request, snapshotExistsError, err.Error())
	case errors.Is(err, save_snapshot.ErrSnapshotStale):
		return debugging.FailureResult(request, snapshotStaleError, err.Error())
	default:
		return debugging.FailureResult(request, fallback, err.Error())
	}
}

func mustJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic("marshal debug result value: " + err.Error())
	}

	return data
}
