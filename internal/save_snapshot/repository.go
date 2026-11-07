package save_snapshot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/bradhannah/UltimaVRedux/internal/repositoryfs"
)

var (
	// ErrSnapshotNotFound indicates that a referenced capture does not exist.
	ErrSnapshotNotFound = errors.New("save snapshot not found")
	// ErrSnapshotExists indicates that an immutable capture already exists.
	ErrSnapshotExists = errors.New("save snapshot already exists")
	// ErrSnapshotStale indicates that content no longer has the expected revision.
	ErrSnapshotStale = errors.New("save snapshot revision is stale")
)

// Record contains a decoded envelope and its canonical SHA-256 revision.
type Record struct {
	Reference Reference
	Envelope  Envelope
	Revision  string
}

// Summary is deterministic list metadata for one capture.
type Summary struct {
	Reference Reference
	Metadata  Metadata
	Revision  string
}

// Repository persists immutable captures under injected source roots.
// Cooperating instances and processes serialize mutation with advisory locks.
type Repository struct {
	roots      map[Source]string
	mutationMu sync.Mutex
}

// ReferenceLockRoots returns detached lock-root metadata for coordination.
func (repository *Repository) ReferenceLockRoots() []repositoryfs.Root {
	return repository.ownRoots()
}

// NewRepository constructs a repository for project, personal, and rollback sources.
func NewRepository(projectRoot, personalRoot, rollbackRoot string) (*Repository, error) {
	if strings.TrimSpace(projectRoot) == "" || strings.TrimSpace(personalRoot) == "" || strings.TrimSpace(rollbackRoot) == "" {
		return nil, errors.New("save snapshot repository requires project, personal, and rollback roots")
	}

	return &Repository{roots: map[Source]string{
		SourceProject: filepath.Clean(projectRoot), SourcePersonal: filepath.Clean(personalRoot), SourceRollback: filepath.Clean(rollbackRoot),
	}}, nil
}

// List returns captures in project, personal, rollback, then ID order.
func (repository *Repository) List() ([]Summary, error) {
	summaries := make([]Summary, 0)
	for _, source := range []Source{SourceProject, SourcePersonal, SourceRollback} {
		entries, err := os.ReadDir(repository.roots[source])
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return nil, fmt.Errorf("list %s save snapshots: %w", source, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			id := strings.TrimSuffix(entry.Name(), ".json")
			reference, err := ParseReference(string(source) + ":" + id)
			if err != nil {
				return nil, fmt.Errorf("list %s save snapshots: %w", source, err)
			}
			record, err := repository.Load(reference.String())
			if err != nil {
				return nil, err
			}
			summaries = append(summaries, Summary{Reference: reference, Metadata: record.Envelope.Metadata, Revision: record.Revision})
		}
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Reference.Source != summaries[j].Reference.Source {
			return sourceOrder(summaries[i].Reference.Source) < sourceOrder(summaries[j].Reference.Source)
		}

		return summaries[i].Reference.ID < summaries[j].Reference.ID
	})

	return summaries, nil
}

// Load reads, strictly validates, and canonicalizes one capture.
func (repository *Repository) Load(qualifiedReference string) (Record, error) {
	reference, err := ParseReference(qualifiedReference)
	if err != nil {
		return Record{}, err
	}
	path := repository.path(reference)
	data, err := repositoryfs.ReadRegularFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Record{}, fmt.Errorf("%w: %s", ErrSnapshotNotFound, reference)
		}
		if errors.Is(err, repositoryfs.ErrNotRegular) {
			return Record{}, fmt.Errorf("%w: capture path must be a regular file", ErrInvalidReference)
		}

		return Record{}, fmt.Errorf("inspect save snapshot %s: %w", reference, err)
	}
	envelope, err := Decode(data)
	if err != nil {
		return Record{}, fmt.Errorf("load save snapshot %s: %w", reference, err)
	}
	revisionData, err := EncodeCanonical(envelope)
	if err != nil {
		return Record{}, err
	}

	return Record{Reference: reference, Envelope: envelope, Revision: repositoryfs.ContentRevision(revisionData)}, nil
}

// Create atomically stores a new immutable named capture.
func (repository *Repository) Create(qualifiedReference string, envelope Envelope) (Record, error) {
	repository.mutationMu.Lock()
	defer repository.mutationMu.Unlock()
	unlock, err := repository.lockMutations()
	if err != nil {
		return Record{}, err
	}
	defer unlock()

	reference, err := ParseReference(qualifiedReference)
	if err != nil {
		return Record{}, err
	}
	data, err := EncodeCanonical(envelope)
	if err != nil {
		return Record{}, err
	}
	root := repository.roots[reference.Source]
	rootPolicy := repository.rootPolicy(reference.Source)
	if err = repositoryfs.EnsureRoot(rootPolicy); err != nil {
		return Record{}, fmt.Errorf("create save snapshot root: %w", err)
	}
	if err = repositoryfs.PublishTemporaryFile(root, ".save-snapshot-*.tmp", data, rootPolicy.FileMode, func(temporaryPath string) error {
		if linkErr := os.Link(temporaryPath, repository.path(reference)); linkErr != nil {
			if errors.Is(linkErr, os.ErrExist) {
				return fmt.Errorf("%w: %s", ErrSnapshotExists, reference)
			}

			return fmt.Errorf("create save snapshot %s: %w", reference, linkErr)
		}

		return nil
	}); err != nil {
		return Record{}, fmt.Errorf("write save snapshot %s: %w", reference, err)
	}
	if err = syncDirectory(root); err != nil {
		return Record{}, fmt.Errorf("persist save snapshot %s: %w", reference, err)
	}

	return Record{Reference: reference, Envelope: envelope, Revision: repositoryfs.ContentRevision(data)}, nil
}

// Delete removes a capture only when expectedRevision matches canonical content.
func (repository *Repository) Delete(qualifiedReference, expectedRevision string) error {
	return repository.DeleteIfUnreferenced(qualifiedReference, expectedRevision, nil)
}

// DeleteIfUnreferenced checks scenario ownership and removes a snapshot while
// holding the same cross-process locks as scenario reference mutations.
func (repository *Repository) DeleteIfUnreferenced(qualifiedReference, expectedRevision string, checkReferences func() error) error {
	repository.mutationMu.Lock()
	defer repository.mutationMu.Unlock()
	unlock, err := repository.lockMutations()
	if err != nil {
		return err
	}
	defer unlock()

	reference, err := ParseReference(qualifiedReference)
	if err != nil {
		return err
	}
	if _, err = repository.requireRevision(reference, expectedRevision); err != nil {
		return err
	}
	if checkReferences != nil {
		if err = checkReferences(); err != nil {
			return err
		}
	}
	if err = os.Remove(repository.path(reference)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrSnapshotNotFound, reference)
		}

		return fmt.Errorf("delete save snapshot %s: %w", reference, err)
	}

	if err = syncDirectory(repository.roots[reference.Source]); err != nil {
		return fmt.Errorf("persist deletion of save snapshot %s: %w", reference, err)
	}

	return nil
}

func (repository *Repository) requireRevision(reference Reference, expected string) (Record, error) {
	record, err := repository.Load(reference.String())
	if err != nil {
		return Record{}, err
	}
	if record.Revision != expected {
		return Record{}, fmt.Errorf("%w: %s", ErrSnapshotStale, reference)
	}

	return record, nil
}

func (repository *Repository) path(reference Reference) string {
	return filepath.Join(repository.roots[reference.Source], reference.ID+".json")
}

func (repository *Repository) lockMutations() (func(), error) {
	return repositoryfs.AcquireReferenceLocks(repository.ReferenceLockRoots())
}

func syncDirectory(root string) error {
	return repositoryfs.SyncDirectory(root)
}

func sourceOrder(source Source) int {
	switch source {
	case SourceProject:
		return 0
	case SourcePersonal:
		return 1
	case SourceRollback:
		return 2
	default:
		return 3
	}
}

func (repository *Repository) ownRoots() []repositoryfs.Root {
	return []repositoryfs.Root{
		repository.rootPolicy(SourceProject), repository.rootPolicy(SourcePersonal), repository.rootPolicy(SourceRollback),
	}
}

func (repository *Repository) rootPolicy(source Source) repositoryfs.Root {
	mode := os.FileMode(0o700)
	fileMode := os.FileMode(0o600)
	if source == SourceProject {
		mode, fileMode = 0o755, 0o644
	}

	return repositoryfs.Root{Path: repository.roots[source], DirMode: mode, FileMode: fileMode}
}
