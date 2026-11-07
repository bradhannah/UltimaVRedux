package player_save

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/repositoryfs"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
)

func TestSlotIDsAreStableAndTraversalIsImpossible(t *testing.T) {
	t.Parallel()
	want := []SlotID{"slot-01", "slot-02", "slot-03", "slot-04", "slot-05", "slot-06", "slot-07", "slot-08"}
	if got := AllSlotIDs(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("AllSlotIDs() = %v, want %v", got, want)
	}
	for _, slot := range want {
		parsed, err := ParseSlotID(string(slot))
		if err != nil || parsed != slot || !slot.Valid() {
			t.Fatalf("ParseSlotID(%q) = %q, %v", slot, parsed, err)
		}
	}

	root := t.TempDir()
	repository := newTestRepository(t, filepath.Join(root, "slots"), nil)
	outside := filepath.Join(root, "escape.json")
	for _, invalid := range []SlotID{"", "slot-00", "slot-09", "../escape", "slot-01/../../escape", "/escape", "SLOT-01"} {
		if _, err := ParseSlotID(string(invalid)); !errors.Is(err, ErrInvalidSlotID) {
			t.Fatalf("ParseSlotID(%q) error = %v, want ErrInvalidSlotID", invalid, err)
		}
		if _, err := repository.Save(invalid, testEnvelope(testTime(1)), ""); !errors.Is(err, ErrInvalidSlotID) {
			t.Fatalf("Save(%q) error = %v, want ErrInvalidSlotID", invalid, err)
		}
		if _, err := repository.Load(invalid); !errors.Is(err, ErrInvalidSlotID) {
			t.Fatalf("Load(%q) error = %v, want ErrInvalidSlotID", invalid, err)
		}
		if err := repository.Delete(invalid, "revision"); !errors.Is(err, ErrInvalidSlotID) {
			t.Fatalf("Delete(%q) error = %v, want ErrInvalidSlotID", invalid, err)
		}
	}
	if _, err := os.Stat(outside); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("traversal created %q: %v", outside, err)
	}
}

func TestRepositoryCreateLoadListMetadataCanonicalRevisionAndPermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "player-saves")
	repository := newTestRepository(t, root, nil)
	envelope := testEnvelope(testTime(1))
	envelope.Snapshot.Party.Roster[1] = testCharacter("Iolo")
	envelope.Snapshot.Party.Roster[2] = testCharacter("Shamino")
	envelope.Snapshot.World.Position = save_snapshot.PositionSnapshot{Location: 7, X: 12, Y: 34, Floor: -2}
	envelope.Snapshot.World.Calendar = save_snapshot.CalendarSnapshot{Year: 142, Month: 8, Day: 17, Hour: 21, Minute: 5, Turns: 99}

	record, err := repository.Save(Slot03, envelope, "")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	path := filepath.Join(root, "slot-03.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	canonical, err := save_snapshot.EncodeCanonical(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, canonical) {
		t.Fatal("slot file does not contain canonical envelope bytes")
	}
	wantRevision := fmt.Sprintf("%x", sha256.Sum256(canonical))
	if record.Revision != wantRevision {
		t.Fatalf("Save() revision = %q, want %q", record.Revision, wantRevision)
	}
	loaded, err := repository.Load(Slot03)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Revision != record.Revision || loaded.Envelope.Metadata.Name != envelope.Metadata.Name {
		t.Fatalf("Load() = %#v, want revision %q and name %q", loaded, record.Revision, envelope.Metadata.Name)
	}

	summaries := repository.List()
	if len(summaries) != 8 {
		t.Fatalf("List() returned %d slots, want 8", len(summaries))
	}
	for index, summary := range summaries {
		if summary.Slot != AllSlotIDs()[index] {
			t.Fatalf("List()[%d].Slot = %q", index, summary.Slot)
		}
		if summary.Slot != Slot03 && summary.Status != SlotEmpty {
			t.Fatalf("List()[%d].Status = %q, want empty", index, summary.Status)
		}
	}
	summary := summaries[2]
	if summary.Status != SlotReady || summary.Revision != record.Revision {
		t.Fatalf("slot-03 summary = %#v", summary)
	}
	metadata := summary.Metadata
	if !metadata.CreatedAt.Equal(envelope.Metadata.CreatedAt) || metadata.SnapshotVersion != save_snapshot.CurrentVersion || metadata.AvatarName != "Avatar" || metadata.Location != 7 || metadata.Floor != -2 || metadata.Calendar != envelope.Snapshot.World.Calendar || metadata.ActivePartyCount != 3 {
		t.Fatalf("derived metadata = %#v", metadata)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entryNames(entries), []string{repositoryfs.ReferenceLockName, "slot-03.json"}) {
		t.Fatalf("save root entries = %v", entryNames(entries))
	}
	if runtime.GOOS != "windows" {
		assertMode(t, root, 0o700)
		assertMode(t, path, 0o600)
	}
}

func TestRepositoryListDerivesActivePartyCountFromFullRoster(t *testing.T) {
	repository := newTestRepository(t, filepath.Join(t.TempDir(), "player-saves"), nil)
	envelope := testEnvelope(testTime(1))
	names := [...]string{
		"Avatar", "Shamino", "Iolo", "Mariah", "Geoffrey", "Jaana", "Julia", "Dupre",
		"Katrina", "Sentri", "Gwenno", "Johne", "Gorn", "Maxwell", "Toshi", "Saduj",
	}
	for index, name := range names {
		envelope.Snapshot.Party.Roster[index] = testCharacter(name)
	}
	for index := 3; index < len(envelope.Snapshot.Party.Roster); index++ {
		envelope.Snapshot.Party.Roster[index].PartyMap = 2
	}
	envelope.Snapshot.Party.Roster[4].PartyMap = 0x7f
	envelope.Snapshot.Party.Roster[5].PartyMap = 0xff

	if _, err := repository.Save(Slot01, envelope, ""); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	summary := repository.List()[0]
	if summary.Status != SlotReady || summary.Metadata.ActivePartyCount != 3 {
		t.Fatalf("List()[0] = %#v, want ready slot with three active party members", summary)
	}
}

func TestRepositoryLoadsMinimumVersionWithCanonicalCurrentRevision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "player-saves")
	repository := newTestRepository(t, root, nil)
	if err := repository.ensureRoot(); err != nil {
		t.Fatal(err)
	}
	data := minimumVersionPlayerSaveData(t, testEnvelope(testTime(1)))
	if err := os.WriteFile(filepath.Join(root, "slot-01.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	record, err := repository.Load(Slot01)
	if err != nil {
		t.Fatalf("Load(minimum version) error = %v", err)
	}
	if record.Envelope.Snapshot.Version != save_snapshot.CurrentVersion || record.Envelope.Snapshot.Dungeon != nil {
		t.Fatalf("Load(minimum version) snapshot = %#v", record.Envelope.Snapshot)
	}
	canonical, err := save_snapshot.EncodeCanonical(record.Envelope)
	if err != nil {
		t.Fatal(err)
	}
	wantRevision := fmt.Sprintf("%x", sha256.Sum256(canonical))
	rawRevision := fmt.Sprintf("%x", sha256.Sum256(data))
	if record.Revision != wantRevision || record.Revision == rawRevision {
		t.Fatalf("Load(minimum version) revision = %q, want canonical v%d %q and not raw historical %q", record.Revision, save_snapshot.CurrentVersion, wantRevision, rawRevision)
	}
	if raw, readErr := os.ReadFile(filepath.Join(root, "slot-01.json")); readErr != nil || !bytes.Equal(raw, data) {
		t.Fatalf("Load(minimum version) rewrote source bytes: error=%v", readErr)
	}
}

func TestRepositorySavesActiveDungeonSlot(t *testing.T) {
	repository := newTestRepository(t, filepath.Join(t.TempDir(), "player-saves"), nil)
	envelope := testEnvelope(testTime(1))
	envelope.Snapshot.World.Position = save_snapshot.PositionSnapshot{Location: 33, X: 2, Y: 3, Floor: 4}
	envelope.Snapshot.Dungeon = &save_snapshot.DungeonSnapshot{
		Location: 33, Cells: make([]int, save_snapshot.DungeonCellCount), Facing: 1, LastDirection: 5,
	}

	record, err := repository.Save(Slot01, envelope, "")
	if err != nil {
		t.Fatalf("Save(active dungeon) error = %v", err)
	}
	loaded, err := repository.Load(Slot01)
	if err != nil {
		t.Fatalf("Load(active dungeon) error = %v", err)
	}
	if !reflect.DeepEqual(loaded.Envelope.Snapshot.Dungeon, envelope.Snapshot.Dungeon) || loaded.Revision != record.Revision {
		t.Fatal("active dungeon changed through player slot repository")
	}
}

func TestRepositorySaveRollsBackCreateWhenDirectorySyncFails(t *testing.T) {
	repository := newTestRepository(t, filepath.Join(t.TempDir(), "slots"), nil)
	syncCalls := 0
	repository.syncDirectory = func(string) error {
		syncCalls++
		if syncCalls == 1 {
			return errors.New("injected sync failure")
		}

		return nil
	}

	if _, err := repository.Save(Slot01, testEnvelope(testTime(1)), ""); err == nil {
		t.Fatal("Save() error = nil, want sync failure")
	}
	if _, err := repository.Load(Slot01); !errors.Is(err, ErrSlotNotFound) {
		t.Fatalf("Load() error = %v, want ErrSlotNotFound after rollback", err)
	}
}

func TestRepositorySaveRollsBackOverwriteWhenDirectorySyncFails(t *testing.T) {
	repository := newTestRepository(t, filepath.Join(t.TempDir(), "slots"), nil)
	original, err := repository.Save(Slot01, testEnvelope(testTime(1)), "")
	if err != nil {
		t.Fatal(err)
	}
	originalData, err := os.ReadFile(repository.path(Slot01))
	if err != nil {
		t.Fatal(err)
	}

	syncCalls := 0
	repository.syncDirectory = func(string) error {
		syncCalls++
		if syncCalls == 1 {
			return errors.New("injected sync failure")
		}

		return nil
	}
	if _, err = repository.Save(Slot01, testEnvelope(testTime(2)), original.Revision); err == nil {
		t.Fatal("Save() error = nil, want sync failure")
	}

	got, err := os.ReadFile(repository.path(Slot01))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, originalData) {
		t.Fatal("failed overwrite did not restore original slot bytes")
	}
}

func TestRepositoryOverwriteRequiresCanonicalCurrentRevision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository := newTestRepository(t, root, nil)
	first := testEnvelope(testTime(1))
	record, err := repository.Save(Slot01, first, "")
	if err != nil {
		t.Fatal(err)
	}
	replacement := testEnvelope(testTime(2))
	replacement.Metadata.Name = "Replacement"

	_, err = repository.Save(Slot01, replacement, "")
	var stale *StaleRevisionError
	if !errors.Is(err, ErrStaleRevision) || !errors.As(err, &stale) || stale.Actual != record.Revision || stale.Expected != "" {
		t.Fatalf("Save(existing without revision) error = %#v", err)
	}
	_, err = repository.Save(Slot01, replacement, "stale")
	if !errors.As(err, &stale) || stale.Actual != record.Revision || stale.Expected != "stale" {
		t.Fatalf("Save(stale) error = %#v", err)
	}
	loaded, err := repository.Load(Slot01)
	if err != nil || loaded.Envelope.Metadata.Name != first.Metadata.Name {
		t.Fatalf("stale overwrite changed slot: %#v, %v", loaded, err)
	}

	canonical, err := save_snapshot.EncodeCanonical(first)
	if err != nil {
		t.Fatal(err)
	}
	compact := bytes.NewBuffer(nil)
	if err = json.Compact(compact, canonical); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "slot-01.json"), compact.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	overwritten, err := repository.Save(Slot01, replacement, record.Revision)
	if err != nil {
		t.Fatalf("Save(canonical revision) error = %v", err)
	}
	if overwritten.Revision == record.Revision {
		t.Fatal("replacement revision did not change")
	}
}

func TestRepositoryConcurrentSavesAllowOneExpectedRevisionWinner(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t, t.TempDir(), nil)
	start := make(chan struct{})
	results := make(chan error, 2)
	for day := 1; day <= 2; day++ {
		envelope := testEnvelope(testTime(day))
		contender := newTestRepository(t, repository.root, nil)
		go func() {
			<-start
			_, saveErr := contender.Save(Slot04, envelope, "")
			results <- saveErr
		}()
	}
	close(start)

	successes, stale := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrStaleRevision):
			stale++
		default:
			t.Fatalf("Save() error = %v", err)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent Save() results: successes=%d stale=%d", successes, stale)
	}
	if _, err := repository.Load(Slot04); err != nil {
		t.Fatalf("Load() after concurrent saves: %v", err)
	}
}

func TestRepositoryFailedWriteLeavesCurrentSlotAndNoTemporaryFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository := newTestRepository(t, root, nil)
	record, err := repository.Save(Slot05, testEnvelope(testTime(1)), "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "slot-05.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	invalid := testEnvelope(testTime(2))
	invalid.Snapshot.Version = save_snapshot.CurrentVersion + 1
	if _, err = repository.Save(Slot05, invalid, record.Revision); !errors.Is(err, save_snapshot.ErrInvalidSnapshot) {
		t.Fatalf("Save(invalid) error = %v, want ErrInvalidSnapshot", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed replacement changed the current slot")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entryNames(entries), []string{repositoryfs.ReferenceLockName, "slot-05.json"}) {
		t.Fatalf("failed replacement left files behind: %v", entryNames(entries))
	}
}

func TestRepositoryListIsolatesCorruptAndUnavailableSlots(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository := newTestRepository(t, root, nil)
	if _, err := repository.Save(Slot01, testEnvelope(testTime(1)), ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "slot-02.json"), []byte(`{"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "slot-03.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	summaries := repository.List()
	want := []SlotStatus{SlotReady, SlotCorrupt, SlotUnavailable, SlotEmpty, SlotEmpty, SlotEmpty, SlotEmpty, SlotEmpty}
	for index, status := range want {
		if summaries[index].Status != status {
			t.Fatalf("List()[%d] = %#v, want status %q", index, summaries[index], status)
		}
	}
	if summaries[1].Problem == "" || summaries[2].Problem == "" {
		t.Fatalf("bad slots lack diagnostic reasons: %#v %#v", summaries[1], summaries[2])
	}

	if runtime.GOOS == "windows" {
		// Windows reports a path beneath a regular file as not found rather than
		// "not a directory", so List shows these slots as empty there.
		return
	}
	unavailableRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(unavailableRoot, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	unavailable := newTestRepository(t, unavailableRoot, nil).List()
	if len(unavailable) != 8 {
		t.Fatalf("unavailable List() count = %d", len(unavailable))
	}
	for index, summary := range unavailable {
		if summary.Status != SlotUnavailable {
			t.Fatalf("unavailable List()[%d] = %#v", index, summary)
		}
	}
}

func TestRepositoryCompatibilityValidationOnlyReceivesDecodedSnapshots(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writer := newTestRepository(t, root, nil)
	compatible := testEnvelope(testTime(1))
	incompatible := testEnvelope(testTime(2))
	incompatible.Snapshot.World.Position.Location = 99
	if _, err := writer.Save(Slot01, compatible, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Save(Slot02, incompatible, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "slot-03.json"), []byte("not JSON"), 0o600); err != nil {
		t.Fatal(err)
	}

	validated := 0
	validatorError := errors.New("location is unavailable in current game data")
	repository := newTestRepository(t, root, func(snapshot save_snapshot.SaveSnapshot) error {
		validated++
		if snapshot.World.Position.Location == 99 {
			return validatorError
		}

		return nil
	})
	summaries := repository.List()
	if validated != 2 {
		t.Fatalf("compatibility validator called %d times, want 2 valid decoded snapshots", validated)
	}
	if summaries[0].Status != SlotReady || summaries[1].Status != SlotIncompatible || summaries[2].Status != SlotCorrupt {
		t.Fatalf("compatibility statuses = %q, %q, %q", summaries[0].Status, summaries[1].Status, summaries[2].Status)
	}
	if summaries[1].Metadata.Location != 99 || summaries[1].Revision == "" || !strings.Contains(summaries[1].Problem, validatorError.Error()) {
		t.Fatalf("incompatible summary = %#v", summaries[1])
	}
	_, err := repository.Load(Slot02)
	var incompatibleError *IncompatibleError
	if !errors.Is(err, ErrIncompatible) || !errors.Is(err, validatorError) || !errors.As(err, &incompatibleError) || incompatibleError.Slot != Slot02 {
		t.Fatalf("Load(incompatible) error = %#v", err)
	}
}

func TestRepositoryListsUnsupportedSnapshotVersionAsReclaimableIncompatible(t *testing.T) {
	t.Parallel()
	// Older native saves below the compatibility floor are reclaimable just like future ones.
	for _, version := range []int{save_snapshot.MinimumVersion - 1, 99} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			envelope := testEnvelope(testTime(1))
			data, err := save_snapshot.EncodeCanonical(envelope)
			if err != nil {
				t.Fatal(err)
			}
			data = bytes.Replace(
				data,
				[]byte(fmt.Sprintf(`"version": %d`, save_snapshot.CurrentVersion)),
				[]byte(fmt.Sprintf(`"version": %d`, version)),
				1,
			)
			if err = os.WriteFile(filepath.Join(root, "slot-02.json"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			repository := newTestRepository(t, root, nil)

			summary := repository.List()[1]
			if summary.Status != SlotIncompatible || summary.Revision == "" || summary.Metadata.SnapshotVersion != version {
				t.Fatalf("unsupported summary = %#v", summary)
			}
			var unsupported *save_snapshot.UnsupportedVersionError
			_, loadErr := repository.Load(Slot02)
			if !errors.As(loadErr, &unsupported) {
				t.Fatal("unsupported slot load did not preserve typed version error")
			}
			replacement := testEnvelope(testTime(2))
			if _, err = repository.Save(Slot02, replacement, summary.Revision); err != nil {
				t.Fatalf("Save(reclaim unsupported slot) error = %v", err)
			}
			if loaded, loadErr := repository.Load(Slot02); loadErr != nil ||
				loaded.Envelope.Metadata.Name != replacement.Metadata.Name ||
				loaded.Envelope.Snapshot.Version != replacement.Snapshot.Version {
				t.Fatalf("Load(reclaimed slot) = %#v, %v", loaded, loadErr)
			}
		})
	}
}

func TestRepositoryRejectsInvalidUnsupportedSnapshotEnvelopesAsCorrupt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	data, err := save_snapshot.EncodeCanonical(testEnvelope(testTime(1)))
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(
		data,
		[]byte(fmt.Sprintf(`"version": %d`, save_snapshot.CurrentVersion)),
		[]byte(`"version": 99`),
		1,
	)
	var base map[string]any
	if err = json.Unmarshal(data, &base); err != nil {
		t.Fatal(err)
	}
	encode := func(mutate func(map[string]any)) []byte {
		document := clonePlayerSaveJSON(t, base)
		mutate(document)
		encoded, marshalErr := json.Marshal(document)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}

		return encoded
	}
	unknownSnapshot := encode(func(document map[string]any) {
		document["snapshot"].(map[string]any)["futureField"] = true
	})
	cases := []struct {
		data   []byte
		status SlotStatus
	}{
		{data: data, status: SlotIncompatible},
		{data: unknownSnapshot, status: SlotIncompatible},
		{data: encode(func(document map[string]any) { document["future"] = true }), status: SlotCorrupt},
		{data: encode(func(document map[string]any) { document["format"] = "other" }), status: SlotCorrupt},
		{data: encode(func(document map[string]any) { delete(document, "metadata") }), status: SlotCorrupt},
		{data: append(append([]byte(nil), data...), []byte(` {}`)...), status: SlotCorrupt},
		{data: encode(func(document map[string]any) { document["snapshot"].(map[string]any)["version"] = "99" }), status: SlotCorrupt},
		{data: []byte(`{"format":`), status: SlotCorrupt},
	}
	for index, test := range cases {
		slot := AllSlotIDs()[index]
		if err = os.WriteFile(filepath.Join(root, string(slot)+".json"), test.data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	summaries := newTestRepository(t, root, nil).List()
	for index, test := range cases {
		if summaries[index].Status != test.status {
			t.Fatalf("List()[%d] = %#v, want status %q", index, summaries[index], test.status)
		}
		if test.status == SlotCorrupt && summaries[index].Revision != repositoryfs.ContentRevision(test.data) {
			t.Fatalf("corrupt List()[%d] lacks raw reclaim revision", index)
		}
	}
}

func TestRepositoryRestorePreservesUnsupportedSnapshotBytes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	envelope := testEnvelope(testTime(1))
	data, err := save_snapshot.EncodeCanonical(envelope)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(
		data,
		[]byte(fmt.Sprintf(`"version": %d`, save_snapshot.CurrentVersion)),
		[]byte(`"version": 99`),
		1,
	)
	path := filepath.Join(root, "slot-03.json")
	if err = os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	repository := newTestRepository(t, root, nil)
	previous, loadErr := repository.Load(Slot03)
	if !errors.Is(loadErr, ErrIncompatible) {
		t.Fatalf("Load(unsupported) error = %v", loadErr)
	}
	replacement, err := repository.Save(Slot03, testEnvelope(testTime(2)), previous.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.Restore(previous, replacement.Revision); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, data) {
		t.Fatal("Restore() did not preserve unsupported snapshot bytes")
	}
}

func TestRepositoryLatestUsesCreationTimeThenLowestSlotNumber(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writer := newTestRepository(t, root, nil)
	for _, entry := range []struct {
		slot     SlotID
		created  time.Time
		location int
	}{
		{slot: Slot03, created: testTime(1), location: 3},
		{slot: Slot08, created: testTime(3), location: 8},
		{slot: Slot02, created: testTime(3), location: 2},
		{slot: Slot01, created: testTime(4), location: 99},
	} {
		envelope := testEnvelope(entry.created)
		envelope.Snapshot.World.Position.Location = entry.location
		if _, err := writer.Save(entry.slot, envelope, ""); err != nil {
			t.Fatal(err)
		}
	}
	repository := newTestRepository(t, root, func(snapshot save_snapshot.SaveSnapshot) error {
		if snapshot.World.Position.Location == 99 {
			return errors.New("unsupported location")
		}

		return nil
	})

	latest, err := repository.Latest()
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if latest.Slot != Slot02 {
		t.Fatalf("Latest().Slot = %q, want %q", latest.Slot, Slot02)
	}

	empty := newTestRepository(t, filepath.Join(t.TempDir(), "empty"), nil)
	_, err = empty.Latest()
	var notFound *NotFoundError
	if !errors.Is(err, ErrSlotNotFound) || !errors.As(err, &notFound) {
		t.Fatalf("Latest(empty) error = %#v", err)
	}
}

func TestRepositoryDeleteRequiresCurrentRevision(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t, t.TempDir(), nil)
	if err := repository.Delete(Slot06, "missing"); err == nil {
		t.Fatal("Delete(empty) succeeded")
	} else {
		var notFound *NotFoundError
		if !errors.Is(err, ErrSlotNotFound) || !errors.As(err, &notFound) || notFound.Slot != Slot06 {
			t.Fatalf("Delete(empty) error = %#v", err)
		}
	}
	record, err := repository.Save(Slot06, testEnvelope(testTime(1)), "")
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.Delete(Slot06, "stale"); err == nil {
		t.Fatal("Delete(stale) succeeded")
	} else {
		var stale *StaleRevisionError
		if !errors.Is(err, ErrStaleRevision) || !errors.As(err, &stale) || stale.Actual != record.Revision {
			t.Fatalf("Delete(stale) error = %#v", err)
		}
	}
	if _, err = repository.Load(Slot06); err != nil {
		t.Fatalf("stale delete removed slot: %v", err)
	}
	if err = repository.Delete(Slot06, record.Revision); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err = repository.Load(Slot06); !errors.Is(err, ErrSlotNotFound) {
		t.Fatalf("Load(deleted) error = %v, want ErrSlotNotFound", err)
	}
	if summary := repository.List()[5]; summary.Status != SlotEmpty {
		t.Fatalf("deleted slot summary = %#v", summary)
	}
}

func TestRepositoryExpectedRevisionPreventsRecreateAfterDeletion(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t, t.TempDir(), nil)
	record, err := repository.Save(Slot07, testEnvelope(testTime(1)), "")
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.Delete(Slot07, record.Revision); err != nil {
		t.Fatal(err)
	}
	_, err = repository.Save(Slot07, testEnvelope(testTime(2)), record.Revision)
	var stale *StaleRevisionError
	if !errors.As(err, &stale) || stale.Expected != record.Revision || stale.Actual != "" {
		t.Fatalf("Save(deleted expected revision) error = %#v", err)
	}
	if _, err = repository.Load(Slot07); !errors.Is(err, ErrSlotNotFound) {
		t.Fatalf("stale recreate wrote slot: %v", err)
	}
}

func newTestRepository(t *testing.T, root string, validator CompatibilityValidator) *Repository {
	t.Helper()
	repository, err := NewRepository(root, validator)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	return repository
}

func testEnvelope(createdAt time.Time) save_snapshot.Envelope {
	snapshot := save_snapshot.SaveSnapshot{
		Version:      save_snapshot.CurrentVersion,
		Presentation: journal.EmptyPresentation(),
		Redux: save_snapshot.ReduxSnapshot{
			DungeonDiscoveries: make([]save_snapshot.DungeonDiscoverySnapshot, 0),
			PlayerKnowledge:    make([]save_snapshot.PlayerKnowledgeSnapshot, 0),
			QuestJournal:       make([]save_snapshot.QuestJournalEventSnapshot, 0),
		},
		World: save_snapshot.WorldSnapshot{
			Calendar: save_snapshot.CalendarSnapshot{Year: 100, Month: 1, Day: 1},
		},
	}
	snapshot.Party.Roster[0] = testCharacter("Avatar")

	return save_snapshot.Envelope{
		Format:          save_snapshot.EnvelopeFormat,
		EnvelopeVersion: save_snapshot.CurrentEnvelopeVersion,
		Metadata: save_snapshot.Metadata{
			Name:          "Player save",
			Description:   "Synthetic player save test fixture.",
			CreatedAt:     createdAt,
			GameVersion:   "test",
			PlaythroughID: "0123456789abcdef0123456789abcdef",
		},
		Snapshot: snapshot,
	}
}

func testCharacter(name string) save_snapshot.CharacterSnapshot {
	class := party_state.Fighter
	if name == "Avatar" {
		class = party_state.Avatar
	}

	return save_snapshot.CharacterSnapshot{
		Occupied: true, Name: name, Gender: int(party_state.Male), Class: int(class),
		Status: int(party_state.Good), Level: 1,
	}
}

func testTime(day int) time.Time {
	return time.Date(2026, time.August, day, 12, 0, 0, 0, time.UTC)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}

	return names
}

func clonePlayerSaveJSON(t *testing.T, source map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err = json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}

	return clone
}

// minimumVersionPlayerSaveData encodes the oldest supported native shape.
func minimumVersionPlayerSaveData(t *testing.T, envelope save_snapshot.Envelope) []byte {
	t.Helper()
	data, err := save_snapshot.EncodeCanonical(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err = json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	snapshot := document["snapshot"].(map[string]any)
	snapshot["version"] = float64(save_snapshot.MinimumVersion)
	delete(snapshot, "excerpts")
	delete(snapshot, "notes")
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestMinimumVersionPlayerSavePreservesVehiclesAndChest(t *testing.T) {
	source := testEnvelope(testTime(1))
	source.Snapshot.AtlasFleet.LastID = 2
	source.Snapshot.PartyVehicle = &save_snapshot.VehicleSnapshot{
		Type: int(references.FrigateVehicle), VesselID: 1, Hull: 87, Skiffs: 2,
	}
	number := 0x87
	source.Snapshot.MapUnits = []save_snapshot.MapUnitSnapshot{
		{Kind: save_snapshot.MapUnitKindVehicle, NPCNum: 1, Position: source.Snapshot.World.Position,
			AIOverrideProvenance: string(references.AIOverrideProvenanceNone),
			Vehicle:              &save_snapshot.VehicleSnapshot{Type: int(references.SkiffVehicle), VesselID: 2, Hull: 99}},
		{Kind: save_snapshot.MapUnitKindChest, NPCNum: 2, ControllerOrder: 1, Position: source.Snapshot.World.Position,
			AIOverrideProvenance: string(references.AIOverrideProvenanceNone), SurfaceChestNumber: &number},
	}
	data := minimumVersionPlayerSaveData(t, source)
	root := filepath.Join(t.TempDir(), "player-saves")
	repository := newTestRepository(t, root, nil)
	if err := repository.ensureRoot(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "slot-01.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	record, err := repository.Load(Slot01)
	if err != nil {
		t.Fatalf("Load(minimum version vehicles/chest) error = %v", err)
	}
	if !reflect.DeepEqual(source.Snapshot, record.Envelope.Snapshot) || source.Metadata != record.Envelope.Metadata ||
		!record.Envelope.NeedsExcerptMigration() {
		t.Fatal("minimum-version fixture changed authoritative vehicle/chest state")
	}
}
