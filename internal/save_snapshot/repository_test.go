package save_snapshot

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/repositoryfs"
)

func TestRepositoryCreateLoadAndDeterministicList(t *testing.T) {
	t.Parallel()
	repository, roots := newTestRepository(t)
	for _, capture := range []struct {
		reference string
		name      string
	}{
		{reference: "rollback:zeta", name: "Rollback Zeta"},
		{reference: "project:zeta", name: "Project Zeta"},
		{reference: "personal:alpha", name: "Personal Alpha"},
		{reference: "project:alpha", name: "Project Alpha"},
	} {
		envelope := validEnvelope(validSnapshot())
		envelope.Metadata.Name = capture.name
		if _, err := repository.Create(capture.reference, envelope); err != nil {
			t.Fatalf("Create(%q) error = %v", capture.reference, err)
		}
	}

	record, err := repository.Load("personal:alpha")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if record.Envelope.Metadata.Name != "Personal Alpha" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(record.Revision) {
		t.Fatalf("Load() = %#v", record)
	}
	summaries, err := repository.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []string{"project:alpha", "project:zeta", "personal:alpha", "rollback:zeta"}
	if len(summaries) != len(want) {
		t.Fatalf("List() count = %d, want %d", len(summaries), len(want))
	}
	for index, summary := range summaries {
		if summary.Reference.String() != want[index] {
			t.Fatalf("List()[%d] = %q, want %q", index, summary.Reference, want[index])
		}
	}
	for _, root := range roots {
		if _, err = os.Stat(filepath.Join(root, repositoryfs.ReferenceLockName)); err != nil {
			t.Fatalf("persistent lock file missing from %q: %v", root, err)
		}
	}
}

func TestRepositoryCapturesAreImmutableAndDeleteIsRevisionSafe(t *testing.T) {
	t.Parallel()
	repository, roots := newTestRepository(t)
	envelope := validEnvelope(validSnapshot())
	record, err := repository.Create("project:named", envelope)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	replacement := envelope
	replacement.Metadata.Name = "Replacement"
	if _, err = repository.Create("project:named", replacement); !errors.Is(err, ErrSnapshotExists) {
		t.Fatalf("Create(existing) error = %v, want ErrSnapshotExists", err)
	}
	entries, err := os.ReadDir(roots[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != repositoryfs.ReferenceLockName || entries[1].Name() != "named.json" {
		t.Fatalf("create collision left files behind: %v", entries)
	}
	if err = repository.Delete("project:named", "stale"); !errors.Is(err, ErrSnapshotStale) {
		t.Fatalf("Delete(stale) error = %v, want ErrSnapshotStale", err)
	}
	loaded, err := repository.Load("project:named")
	if err != nil || loaded.Envelope.Metadata.Name != envelope.Metadata.Name {
		t.Fatalf("stale delete or create collision changed capture: %#v, %v", loaded, err)
	}
	if err = repository.Delete("project:named", record.Revision); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err = repository.Load("project:named"); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("Load(deleted) error = %v, want ErrSnapshotNotFound", err)
	}
}

func TestRepositoryRevisionUsesCanonicalContent(t *testing.T) {
	t.Parallel()
	repository, roots := newTestRepository(t)
	record, err := repository.Create("project:canonical", validEnvelope(validSnapshot()))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	path := filepath.Join(roots[0], "canonical.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	compact := &bytes.Buffer{}
	if err = json.Compact(compact, data); err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if err = os.WriteFile(path, compact.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile(compact) error = %v", err)
	}
	loaded, err := repository.Load("project:canonical")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Revision != record.Revision {
		t.Fatalf("canonical revision changed with whitespace: got %q, want %q", loaded.Revision, record.Revision)
	}
}

func TestRepositoryPromotesMinimumVersionWithCanonicalRevision(t *testing.T) {
	t.Parallel()
	repository, roots := newTestRepository(t)
	document := currentEnvelopeDocument(t, validSnapshot())
	body := document["snapshot"].(map[string]any)
	body["version"] = MinimumVersion
	delete(body, "excerpts")
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(roots[0], "promoted-v24.json")
	if err = os.MkdirAll(roots[0], 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	record, err := repository.Load("project:promoted-v24")
	if err != nil {
		t.Fatalf("Load(version %d) error = %v", MinimumVersion, err)
	}
	if record.Envelope.Snapshot.Version != CurrentVersion || !record.Envelope.NeedsExcerptMigration() {
		t.Fatalf("Load(version %d) snapshot = %#v", MinimumVersion, record.Envelope.Snapshot)
	}
	canonical, err := EncodeCanonical(record.Envelope)
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision != repositoryfs.ContentRevision(canonical) {
		t.Fatalf("Load(version %d) revision = %q, want canonical %q", MinimumVersion, record.Revision, repositoryfs.ContentRevision(canonical))
	}
	retained, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retained, data) {
		t.Fatalf("Load(version %d) rewrote the source record", MinimumVersion)
	}
}

func TestRepositoryRejectsUnsupportedVersionWithoutRewritingSource(t *testing.T) {
	t.Parallel()
	repository, roots := newTestRepository(t)
	document := currentEnvelopeDocument(t, validSnapshot())
	document["snapshot"].(map[string]any)["version"] = MinimumVersion - 1
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(roots[0], "unsupported-v23.json")
	if err = os.MkdirAll(roots[0], 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = repository.Load("project:unsupported-v23")
	var unsupported *UnsupportedVersionError
	if !errors.As(err, &unsupported) || unsupported.Version != MinimumVersion-1 {
		t.Fatalf("Load(unsupported version) error = %v, want typed UnsupportedVersionError", err)
	}
	retained, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retained, data) {
		t.Fatal("Load(unsupported version) rewrote the source record")
	}
}

func TestRepositoryCreatesActiveDungeonCapture(t *testing.T) {
	t.Parallel()
	repository, _ := newTestRepository(t)
	envelope := validEnvelope(validSnapshot())
	envelope.Snapshot.World.Position = PositionSnapshot{Location: 33, X: 2, Y: 3, Floor: 4}
	envelope.Snapshot.Dungeon = &DungeonSnapshot{
		Location: 33, Cells: make([]int, DungeonCellCount), Facing: 1, LastDirection: 5,
	}

	record, err := repository.Create("rollback:dungeon", envelope)
	if err != nil {
		t.Fatalf("Create(active dungeon) error = %v", err)
	}
	loaded, err := repository.Load(record.Reference.String())
	if err != nil {
		t.Fatalf("Load(active dungeon) error = %v", err)
	}
	if !reflect.DeepEqual(loaded.Envelope.Snapshot.Dungeon, envelope.Snapshot.Dungeon) {
		t.Fatal("active dungeon changed through named repository")
	}
}

func TestRepositoryConcurrentCreateHasSingleWinner(t *testing.T) {
	t.Parallel()
	repository, roots := newTestRepository(t)
	other, err := NewRepository(roots[0], roots[1], roots[2])
	if err != nil {
		t.Fatalf("NewRepository(second) error = %v", err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, writer := range []*Repository{repository, other} {
		go func() {
			<-start
			_, createErr := writer.Create("rollback:racing", validEnvelope(validSnapshot()))
			results <- createErr
		}()
	}
	close(start)
	successes, exists := 0, 0
	for range 2 {
		err = <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrSnapshotExists):
			exists++
		default:
			t.Fatalf("Create() error = %v", err)
		}
	}
	if successes != 1 || exists != 1 {
		t.Fatalf("concurrent results: successes=%d exists=%d", successes, exists)
	}
}

func TestRepositoryRejectsInvalidPathsAndSymlinks(t *testing.T) {
	t.Parallel()
	repository, roots := newTestRepository(t)
	if _, err := repository.Create("project:../escape", validEnvelope(validSnapshot())); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("Create(traversal) error = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(roots[0], "linked.json")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := repository.Load("project:linked"); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("Load(symlink) error = %v, want ErrInvalidReference", err)
	}
	if _, err := repository.Create("project:linked", validEnvelope(validSnapshot())); !errors.Is(err, ErrSnapshotExists) {
		t.Fatalf("Create(symlink) error = %v, want ErrSnapshotExists", err)
	}
}

func TestRepositoryRejectsCorruptFilesAndMissingRoots(t *testing.T) {
	t.Parallel()
	if _, err := NewRepository("", t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("NewRepository() accepted an empty root")
	}
	repository, roots := newTestRepository(t)
	if err := os.MkdirAll(roots[1], 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(roots[1], "corrupt.json"), []byte(`{"bad":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := repository.Load("personal:corrupt"); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("Load(corrupt) error = %v, want ErrInvalidEnvelope", err)
	}
}

func TestRepositoryUsesPrivateModesForPersonalAndRollback(t *testing.T) {
	repository, roots := newTestRepository(t)
	for index, reference := range []string{"personal:private", "rollback:recovery"} {
		if _, err := repository.Create(reference, validEnvelope(validSnapshot())); err != nil {
			t.Fatalf("Create(%q) error = %v", reference, err)
		}
		root := roots[index+1]
		rootInfo, err := os.Stat(root)
		if err != nil {
			t.Fatal(err)
		}
		parsed, _ := ParseReference(reference)
		fileInfo, err := os.Stat(filepath.Join(root, parsed.ID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		// Windows has no Unix permission bits; repositoryfs skips chmod there.
		if runtime.GOOS == "windows" {
			continue
		}
		if got := rootInfo.Mode().Perm(); got != 0o700 {
			t.Fatalf("%s root mode = %o, want 700", reference, got)
		}
		if got := fileInfo.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s file mode = %o, want 600", reference, got)
		}
	}
}

func newTestRepository(t *testing.T) (*Repository, []string) {
	t.Helper()
	root := t.TempDir()
	roots := []string{filepath.Join(root, "project"), filepath.Join(root, "personal"), filepath.Join(root, "rollback")}
	repository, err := NewRepository(roots[0], roots[1], roots[2])
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	return repository, roots
}
