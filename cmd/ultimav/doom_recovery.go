package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/doom_recovery"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
)

const (
	doomRecoveryName        = "Pre-Doom Recovery"
	doomRecoveryDescription = "Saved automatically just before you entered Doom."
	doomRunIDBytes          = 16
)

var (
	ErrDoomRecoveryUnavailable       = errors.New("pre-Doom recovery services are unavailable")
	ErrDoomRecoveryUnassociated      = errors.New("current Doom run has no recovery association")
	ErrDoomRecoveryAssociation       = errors.New("pre-Doom recovery association does not match the current run")
	ErrDoomRecoveryOrigin            = errors.New("pre-Doom recovery checkpoint has an invalid origin")
	ErrDoomRecoverySceneIneligible   = errors.New("recovery scene does not support canonical validation")
	ErrDoomRecoveryHydrationMismatch = errors.New("recovery scene did not reproduce the canonical checkpoint")
)

type doomRecoveryRepository interface {
	Load() (doom_recovery.Record, error)
	Replace(envelope save_snapshot.Envelope) (doom_recovery.Record, error)
}

func newDoomRunID() (string, error) {
	bytes := make([]byte, doomRunIDBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate Doom run ID: %w", err)
	}

	return hex.EncodeToString(bytes), nil
}

func (g *GameScene) setDoomRunAssociation(runID string) {
	g.activeDoomRunID = runID
}

func (g *GameScene) doomRunAssociation() string {
	return g.activeDoomRunID
}

func (g *GameScene) doomRunAssociationForPlayerSave() string {
	if g == nil || g.gameState == nil || g.gameState.MapState.PlayerLocation.Location != references.Doom {
		return ""
	}

	return g.activeDoomRunID
}

func (g *GameScene) clearDoomRunAssociationOutsideRun() {
	if g.activeDoomRunID == "" || g.gameState == nil {
		return
	}
	location := g.gameState.MapState.PlayerLocation.Location
	if location == references.Doom || location == references.CombatLocation || location == references.FinaleLocation {
		return
	}
	if location == references.BritanniaUnderworld &&
		g.gameState.EventQueueManager.HasWorkflow("enter_location") {
		return
	}

	g.activeDoomRunID = ""
}

func (g *Game) acceptDoomEntry(
	scene *GameScene,
	candidate save_snapshot.SaveSnapshot,
	captureErr error,
) {
	runID, err := g.doomRunID()
	if err != nil {
		scene.setDoomRunAssociation("")
		scene.reportDoomRecoveryPreparationFailure(err)

		return
	}
	scene.setDoomRunAssociation(runID)
	if captureErr != nil {
		scene.reportDoomRecoveryPreparationFailure(captureErr)

		return
	}
	if g.doomRecoveryRepository == nil || g.doomRecoveryNow == nil {
		scene.reportDoomRecoveryPreparationFailure(ErrDoomRecoveryUnavailable)

		return
	}
	metadata := scene.playthroughMetadata()
	if err = metadata.validatePlayerSave(); err != nil {
		scene.reportDoomRecoveryPreparationFailure(err)

		return
	}
	envelope := save_snapshot.Envelope{
		Format: save_snapshot.EnvelopeFormat, EnvelopeVersion: save_snapshot.CurrentEnvelopeVersion,
		Metadata: save_snapshot.Metadata{
			Name: doomRecoveryName, Description: doomRecoveryDescription,
			CreatedAt: g.doomRecoveryNow().UTC(), GameVersion: g.playerSaveGameVersion, DoomRunID: runID,
			PlaythroughID: metadata.id, UserDescription: metadata.userDescription,
		},
		Snapshot: candidate,
	}
	if _, err = g.doomRecoveryRepository.Replace(envelope); err != nil {
		scene.reportDoomRecoveryPreparationFailure(err)
	}
}

func (g *GameScene) reportDoomRecoveryPreparationFailure(err error) {
	logging.Warnf(logging.CategoryGameState, "Unable to prepare pre-Doom recovery checkpoint: %v", err)
	g.gameState.SystemCallbacks.Message.AddRowStr("Pre-Doom recovery is unavailable.")
}

//nolint:cyclop,funlen // Recovery validation is one ordered candidate-first transaction.
func (g *Game) returnBeforeDoom(source *GameScene) error {
	if source == nil || g.currentScene != source {
		return ErrDoomRecoverySceneIneligible
	}
	runID := source.doomRunAssociation()
	if runID == "" {
		return ErrDoomRecoveryUnassociated
	}
	if g.doomRecoveryRepository == nil || g.doomRecoverySceneBuilder == nil {
		return ErrDoomRecoveryUnavailable
	}
	record, err := g.doomRecoveryRepository.Load()
	if err != nil {
		return fmt.Errorf("load pre-Doom recovery checkpoint: %w", err)
	}
	if record.Envelope.Metadata.DoomRunID == "" || record.Envelope.Metadata.DoomRunID != runID {
		return ErrDoomRecoveryAssociation
	}
	metadata := playthroughMetadataFromEnvelope(record.Envelope.Metadata)
	if !save_snapshot.ValidPlaythroughID(metadata.id) || metadata.id != source.activePlaythrough.id {
		return ErrDoomRecoveryAssociation
	}
	if !source.isValidPreDoomRecoveryOrigin(record.Envelope.Snapshot) {
		return ErrDoomRecoveryOrigin
	}

	snapshot, err := g.prepareExcerptMigration(record.Envelope)
	if err != nil {
		return fmt.Errorf("prepare pre-Doom recovery excerpts: %w", err)
	}
	candidate, err := g.doomRecoverySceneBuilder(source.gameConfig, snapshot)
	if err != nil {
		return fmt.Errorf("construct pre-Doom recovery scene: %w", err)
	}
	if candidate == nil {
		return ErrDoomRecoverySceneIneligible
	}
	candidateOwned := true
	defer func() {
		if candidateOwned {
			if disposer, ok := candidate.(sceneDisposer); ok {
				disposer.Dispose()
			}
		}
	}()
	if err = transferScenePlaythrough(candidate, metadata); err != nil {
		return fmt.Errorf("restore pre-Doom playthrough: %w", err)
	}

	capture, capturesSnapshots := candidate.(playerSaveCaptureScene)
	if !capturesSnapshots {
		return ErrDoomRecoverySceneIneligible
	}
	if _, _, hasProvenance := capture.playerSaveProvenance(); hasProvenance {
		return ErrDoomRecoverySceneIneligible
	}
	association, ok := candidate.(doomRunAssociationOwner)
	if !ok {
		return ErrDoomRecoverySceneIneligible
	}
	association.setDoomRunAssociation("")
	if preflight, ok := candidate.(sceneActivationPreflighter); ok {
		if err = preflight.PreflightActivation(); err != nil {
			return fmt.Errorf("preflight pre-Doom recovery scene: %w", err)
		}
	}
	recaptured, err := capture.CaptureSaveSnapshot()
	if err != nil {
		return fmt.Errorf("validate pre-Doom recovery scene: %w", err)
	}
	if !reflect.DeepEqual(recaptured, snapshot) {
		return ErrDoomRecoveryHydrationMismatch
	}

	candidateOwned = false
	if err = g.commitSceneReplacement(candidate); err != nil {
		return fmt.Errorf("activate pre-Doom recovery scene: %w", err)
	}

	return nil
}

func (g *GameScene) isValidPreDoomRecoveryOrigin(snapshot save_snapshot.SaveSnapshot) bool {
	if snapshot.World.Position.Location != int(references.BritanniaUnderworld) ||
		snapshot.World.Position.Floor != int(references.UNDERWORLD) || snapshot.Dungeon != nil {
		return false
	}
	entrance := g.gameState.GetDungeonEntranceCoordinates()[references.Doom]

	return snapshot.World.Position.X == int(entrance.X) && snapshot.World.Position.Y == int(entrance.Y)
}

func (g *GameScene) showDoomRecoveryFailure(err error) {
	presentation := g.finalePresentation
	if presentation == nil {
		return
	}
	message := "The pre-Doom checkpoint could not be restored."
	switch {
	case errors.Is(err, doom_recovery.ErrCheckpointNotFound):
		message = "No pre-Doom checkpoint is available."
	case errors.Is(err, save_snapshot.ErrInvalidEnvelope), errors.Is(err, save_snapshot.ErrInvalidSnapshot):
		message = "The pre-Doom checkpoint is damaged."
	case errors.Is(err, doom_recovery.ErrCheckpointIncompatible):
		message = "The pre-Doom checkpoint is incompatible with this build."
	case errors.Is(err, ErrDoomRecoveryUnassociated), errors.Is(err, ErrDoomRecoveryAssociation):
		message = "This Doom run has no matching recovery checkpoint."
	case errors.Is(err, ErrDoomRecoveryOrigin):
		message = "The recovery checkpoint is not from before this Doom run."
	}
	g.showDoomRecoveryChoice(message)
	presentation.record("recovery-unavailable:" + strings.TrimSuffix(message, "."))
}
