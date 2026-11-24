package player_save

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/repositoryfs"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
)

var (
	errInvalidRestoreRecord = errors.New("player save restore record bytes do not match revision")
	errMissingPlaythroughID = errors.New("metadata playthroughId is required")
)

const (
	rootMode = os.FileMode(0o700)
	fileMode = os.FileMode(0o600)
)

// Repository owns the fixed player save slots under one private root.
type Repository struct {
	root                  string
	validateCompatibility CompatibilityValidator
	mutationMu            sync.Mutex
	syncDirectory         func(string) error
}

// NewRepository constructs a player save repository without creating its root.
func NewRepository(root string, validateCompatibility CompatibilityValidator) (*Repository, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalidRoot
	}

	return &Repository{
		root:                  filepath.Clean(root),
		validateCompatibility: validateCompatibility,
		mutationMu:            sync.Mutex{},
		syncDirectory:         repositoryfs.SyncDirectory,
	}, nil
}

// List independently inspects every fixed slot in slot-number order. One bad
// slot is represented by its status and never prevents healthy entries returning.
func (repository *Repository) List() []SlotSummary {
	slots := fixedSlotIDs()
	summaries := make([]SlotSummary, 0, len(slots))
	for _, slot := range slots {
		record, err := repository.load(slot, true)
		var summary SlotSummary
		summary.Slot = slot
		switch {
		case err == nil:
			summary.Status = SlotReady
			summary.Metadata = summarize(record.Envelope)
			summary.Revision = record.Revision
		case errors.Is(err, ErrSlotNotFound):
			summary.Status = SlotEmpty
		case errors.Is(err, ErrIncompatible):
			summary.Status = SlotIncompatible
			summary.Metadata = summarize(record.Envelope)
			summary.Revision = record.Revision
			summary.Problem = err.Error()
		case errors.Is(err, save_snapshot.ErrInvalidEnvelope), errors.Is(err, save_snapshot.ErrInvalidSnapshot):
			summary.Status = SlotCorrupt
			summary.Revision = record.Revision
			summary.Problem = err.Error()
		default:
			summary.Status = SlotUnavailable
			summary.Problem = err.Error()
		}
		summaries = append(summaries, summary)
	}

	return summaries
}

// Load strictly decodes and compatibility-validates one occupied slot.
// Recognized decode failures return an opaque record for revision-safe replacement
// and exact rollback, alongside the validation error.
func (repository *Repository) Load(slot SlotID) (Record, error) {
	if err := validateSlot(slot); err != nil {
		return Record{}, err
	}

	return repository.load(slot, true)
}

// ValidateCompatibility checks a candidate against the repository's runtime
// references without reading or mutating a slot.
func (repository *Repository) ValidateCompatibility(snapshot save_snapshot.SaveSnapshot) error {
	if repository.validateCompatibility == nil {
		return nil
	}

	return repository.validateCompatibility(snapshot)
}

// Save creates an empty slot or replaces an occupied slot atomically. Creation
// requires an empty expected revision; replacement requires the current revision.
func (repository *Repository) Save(
	slot SlotID,
	envelope save_snapshot.Envelope,
	expectedRevision string,
) (Record, error) {
	if err := validateSlot(slot); err != nil {
		return Record{}, err
	}
	data, err := save_snapshot.EncodeCanonical(envelope)
	if err != nil {
		return Record{}, fmt.Errorf("encode player save %s: %w", slot, err)
	}
	if !save_snapshot.ValidPlaythroughID(envelope.Metadata.PlaythroughID) {
		return Record{}, fmt.Errorf("encode player save %s: %w: %w", slot,
			save_snapshot.ErrInvalidEnvelope, errMissingPlaythroughID)
	}

	repository.mutationMu.Lock()
	defer repository.mutationMu.Unlock()

	unlock, err := repository.lockTransaction()
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	if err = repository.requireExpectedRevision(slot, expectedRevision); err != nil {
		return Record{}, err
	}
	repository.keepOtherVersionCopy(slot)
	if err = repository.replaceData(slot, data); err != nil {
		return Record{}, err
	}

	return Record{
		Slot: slot, Envelope: envelope, Revision: repositoryfs.ContentRevision(data),
		rawData: append([]byte(nil), data...), rawRevision: repositoryfs.ContentRevision(data),
	}, nil
}

// Restore atomically reinstates exact bytes previously returned by Load. It is
// intended only for rolling back a failed higher-level operation.
func (repository *Repository) Restore(record Record, expectedRevision string) error {
	if err := validateSlot(record.Slot); err != nil {
		return err
	}
	if record.rawRevision == "" || repositoryfs.ContentRevision(record.rawData) != record.rawRevision {
		return fmt.Errorf("restore player save %s: %w", record.Slot, errInvalidRestoreRecord)
	}

	repository.mutationMu.Lock()
	defer repository.mutationMu.Unlock()

	unlock, err := repository.lockTransaction()
	if err != nil {
		return err
	}
	defer unlock()
	if err := repository.requireExpectedRevision(record.Slot, expectedRevision); err != nil {
		return err
	}

	return repository.replaceData(record.Slot, record.rawData)
}

// Delete removes an occupied slot only when expectedRevision is current.
func (repository *Repository) Delete(slot SlotID, expectedRevision string) error {
	if err := validateSlot(slot); err != nil {
		return err
	}

	repository.mutationMu.Lock()
	defer repository.mutationMu.Unlock()

	unlock, err := repository.lockTransaction()
	if err != nil {
		return err
	}
	defer unlock()

	current, err := repository.load(slot, false)
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return &StaleRevisionError{Slot: slot, Expected: expectedRevision, Actual: current.Revision}
	}
	if err = os.Remove(repository.path(slot)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &NotFoundError{Slot: slot}
		}

		return fmt.Errorf("delete player save %s: %w", slot, err)
	}
	if err = repository.syncDirectory(repository.root); err != nil {
		persistErr := fmt.Errorf("persist deletion of player save %s: %w", slot, err)
		if rollbackErr := repository.rollbackReplacement(slot, current.rawData, true); rollbackErr != nil {
			return errors.Join(persistErr, fmt.Errorf("roll back deletion of player save %s: %w", slot, rollbackErr))
		}

		return persistErr
	}

	return nil
}

// Latest returns the newest ready slot. Equal creation times are resolved by
// the lowest slot number.
func (repository *Repository) Latest() (Record, error) {
	slots := fixedSlotIDs()
	records := make([]Record, 0, len(slots))
	for _, slot := range slots {
		record, err := repository.load(slot, true)
		if err == nil {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		left, right := records[i], records[j]
		if left.Envelope.Metadata.CreatedAt.Equal(right.Envelope.Metadata.CreatedAt) {
			return left.Slot < right.Slot
		}

		return left.Envelope.Metadata.CreatedAt.After(right.Envelope.Metadata.CreatedAt)
	})
	if len(records) > 0 {
		return records[0], nil
	}

	return Record{}, &NotFoundError{Slot: ""}
}

func (repository *Repository) requireExpectedRevision(slot SlotID, expected string) error {
	current, err := repository.load(slot, false)
	if err != nil {
		if !errors.Is(err, ErrSlotNotFound) {
			return fmt.Errorf("inspect player save %s before write: %w", slot, err)
		}
		if expected != "" {
			return &StaleRevisionError{Slot: slot, Expected: expected, Actual: ""}
		}

		return nil
	}
	if current.Revision != expected {
		return &StaleRevisionError{Slot: slot, Expected: expected, Actual: current.Revision}
	}

	return nil
}

func (repository *Repository) load(slot SlotID, checkCompatibility bool) (Record, error) {
	data, err := repositoryfs.ReadRegularFile(repository.path(slot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Record{}, &NotFoundError{Slot: slot}
		}

		return Record{}, fmt.Errorf("read player save %s: %w", slot, err)
	}
	envelope, err := save_snapshot.Decode(data)
	if err != nil {
		return decodeUnsupportedRecord(slot, data, err, checkCompatibility)
	}
	revisionData, err := save_snapshot.EncodeCanonical(envelope)
	if err != nil {
		return Record{}, fmt.Errorf("canonicalize player save %s: %w", slot, err)
	}
	record := Record{
		Slot: slot, Envelope: envelope, Revision: repositoryfs.ContentRevision(revisionData),
		rawData: append([]byte(nil), data...), rawRevision: repositoryfs.ContentRevision(data),
	}
	// Identity is a player eligibility rule, not a prerequisite for revision-safe mutation.
	if checkCompatibility && !save_snapshot.ValidPlaythroughID(envelope.Metadata.PlaythroughID) {
		return record, &IncompatibleError{Slot: slot, Cause: errMissingPlaythroughID}
	}
	if checkCompatibility && repository.validateCompatibility != nil {
		if err = repository.validateCompatibility(envelope.Snapshot); err != nil {
			return record, &IncompatibleError{Slot: slot, Cause: err}
		}
	}

	return record, nil
}

func decodeUnsupportedRecord(slot SlotID, data []byte, decodeErr error, checkCompatibility bool) (Record, error) {
	if errors.Is(decodeErr, save_snapshot.ErrInvalidEnvelope) || errors.Is(decodeErr, save_snapshot.ErrInvalidSnapshot) {
		var envelope save_snapshot.Envelope
		record := Record{
			Slot: slot, Envelope: envelope, Revision: repositoryfs.ContentRevision(data),
			rawData: append([]byte(nil), data...), rawRevision: repositoryfs.ContentRevision(data),
		}
		if checkCompatibility {
			return record, fmt.Errorf("decode player save %s: %w", slot, decodeErr)
		}

		return record, nil
	}
	if !errors.Is(decodeErr, save_snapshot.ErrUnsupportedSnapshotVersion) {
		return Record{}, fmt.Errorf("decode player save %s: %w", slot, decodeErr)
	}
	envelope, err := save_snapshot.DecodeUnsupportedVersion(data)
	if err != nil {
		return Record{}, fmt.Errorf("decode player save %s: %w", slot, decodeErr)
	}
	unsupported := &save_snapshot.UnsupportedVersionError{Version: envelope.Snapshot.Version}
	record := Record{
		Slot: slot, Envelope: envelope, Revision: repositoryfs.ContentRevision(data),
		rawData: append([]byte(nil), data...), rawRevision: repositoryfs.ContentRevision(data),
	}
	if checkCompatibility {
		return record, &IncompatibleError{Slot: slot, Cause: unsupported}
	}

	return record, nil
}

// Keep the advisory lock through revision checking, publication, fsync and
// rollback. Never unlink its file: contenders must keep locking the same inode.
func (repository *Repository) lockTransaction() (func(), error) {
	unlock, err := repositoryfs.AcquireReferenceLocks([]repositoryfs.Root{{
		Path: repository.root, DirMode: rootMode, FileMode: fileMode,
	}})
	if err != nil {
		return nil, fmt.Errorf("lock player save transaction: %w", err)
	}

	return unlock, nil
}

func (repository *Repository) path(slot SlotID) string {
	return filepath.Join(repository.root, string(slot)+".json")
}

// keepOtherVersionCopy copies a save that another version of the game wrote
// before this version replaces it. The first save after loading an older file
// rewrites it in the current format, and a newer build's file cannot be read
// here at all, so the copy is the only way back to either. It is best effort:
// a save must never fail because its safety copy could not be written.
func (repository *Repository) keepOtherVersionCopy(slot SlotID) {
	previous, occupied, err := repository.readCurrentData(slot)
	if err != nil || !occupied {
		return
	}
	version, err := save_snapshot.StoredVersion(previous)
	if err != nil || version == save_snapshot.CurrentVersion {
		return
	}
	copyPath := filepath.Join(repository.root, fmt.Sprintf("%s.v%d.bak", slot, version))
	if _, statErr := os.Lstat(copyPath); statErr == nil {
		return // The first copy is the original; keep it.
	}
	err = repositoryfs.PublishTemporaryFile(repository.root, ".player-save-*.tmp", previous, fileMode,
		func(temporaryPath string) error { return os.Rename(temporaryPath, copyPath) })
	if err != nil {
		logging.Warnf(logging.CategoryGameState, "Could not keep a copy of %s (version %d): %v", slot, version, err)
	}
}

func (repository *Repository) replaceData(slot SlotID, data []byte) error {
	previous, occupied, err := repository.readCurrentData(slot)
	if err != nil {
		return err
	}
	if err = repository.writeAndRename(slot, data); err != nil {
		return err
	}
	if err = repository.syncDirectory(repository.root); err == nil {
		return nil
	}

	persistErr := fmt.Errorf("persist player save %s: %w", slot, err)
	if rollbackErr := repository.rollbackReplacement(slot, previous, occupied); rollbackErr != nil {
		return errors.Join(persistErr, fmt.Errorf("roll back player save %s: %w", slot, rollbackErr))
	}

	return persistErr
}

func (repository *Repository) readCurrentData(slot SlotID) ([]byte, bool, error) {
	data, err := repositoryfs.ReadRegularFile(repository.path(slot))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read current player save %s: %w", slot, err)
	}

	return data, true, nil
}

func (repository *Repository) writeAndRename(slot SlotID, data []byte) error {
	if err := repositoryfs.PublishTemporaryFile(
		repository.root,
		".player-save-*.tmp",
		data,
		fileMode,
		func(temporaryPath string) error {
			if renameErr := os.Rename(temporaryPath, repository.path(slot)); renameErr != nil {
				return fmt.Errorf("replace player save %s: %w", slot, renameErr)
			}

			return nil
		},
	); err != nil {
		return fmt.Errorf("write player save %s: %w", slot, err)
	}

	return nil
}

func (repository *Repository) rollbackReplacement(slot SlotID, previous []byte, occupied bool) error {
	if occupied {
		if err := repository.writeAndRename(slot, previous); err != nil {
			return err
		}
	} else if err := os.Remove(repository.path(slot)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove newly written player save: %w", err)
	}

	return repository.syncDirectory(repository.root)
}

func validateSlot(slot SlotID) error {
	if !slot.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidSlotID, slot)
	}

	return nil
}

func summarize(envelope save_snapshot.Envelope) MetadataSummary {
	snapshot := envelope.Snapshot
	activePartyCount := 0
	for _, character := range snapshot.Party.Roster {
		if character.Occupied && character.PartyMap == 0 {
			activePartyCount++
		}
	}

	return MetadataSummary{
		CreatedAt:        envelope.Metadata.CreatedAt,
		PlaythroughID:    envelope.Metadata.PlaythroughID,
		UserDescription:  envelope.Metadata.UserDescription,
		SnapshotVersion:  snapshot.Version,
		AvatarName:       snapshot.Party.Roster[0].Name,
		Location:         snapshot.World.Position.Location,
		Floor:            snapshot.World.Position.Floor,
		Calendar:         snapshot.World.Calendar,
		ActivePartyCount: activePartyCount,
	}
}
