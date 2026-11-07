package legacy_save

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadBundleFindsCaseInsensitiveExactSiblingWithoutWriting(t *testing.T) {
	directory := t.TempDir()
	gam, ool := validBundleBytes()
	gamPath := filepath.Join(directory, "saved.gam")
	oolPath := filepath.Join(directory, "saved.ool")
	if err := os.WriteFile(gamPath, gam, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oolPath, ool, 0o444); err != nil {
		t.Fatal(err)
	}

	bundle, err := ReadBundleContext(t.Context(), gamPath)
	if err != nil {
		t.Fatalf("ReadBundleContext() error = %v", err)
	}
	if bundle.Game.Characters[0].Name != "C00" {
		t.Fatalf("ReadBundleContext() character name = %q, want C00", bundle.Game.Characters[0].Name)
	}

	gamAfter, err := os.ReadFile(gamPath)
	if err != nil {
		t.Fatal(err)
	}
	oolAfter, err := os.ReadFile(oolPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gamAfter, gam) || !bytes.Equal(oolAfter, ool) {
		t.Fatal("ReadBundleContext() modified a source file")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("ReadBundleContext() changed directory contents: %#v", entries)
	}
}

func TestReadBundleRequiresExactSiblingName(t *testing.T) {
	directory := t.TempDir()
	gam, ool := validBundleBytes()
	gamPath := filepath.Join(directory, "SAVED.GAM")
	if err := os.WriteFile(gamPath, gam, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SAVED.OOL.bak"), ool, 0o600); err != nil {
		t.Fatal(err)
	}

	bundle, err := ReadBundleContext(t.Context(), gamPath)
	if bundle != (Bundle{}) {
		t.Fatal("missing sibling returned a partial bundle")
	}
	assertValidationIssue(t, err, "ool", "missing_sibling", -1)
}

func TestReadBundleRejectsWrongSelectedFilename(t *testing.T) {
	directory := t.TempDir()
	gam, ool := validBundleBytes()
	gamPath := filepath.Join(directory, "GAME.GAM")
	if err := os.WriteFile(gamPath, gam, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SAVED.OOL"), ool, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ReadBundleContext(t.Context(), gamPath)
	assertValidationIssue(t, err, "gam", "invalid_filename", -1)
}

func TestReadBundlePropagatesExactFileLengthValidation(t *testing.T) {
	tests := []struct {
		name       string
		gamLength  int
		oollLength int
		path       string
	}{
		{name: "short SAVED.GAM", gamLength: SavedGameSize - 1, oollLength: SavedObjectsSize, path: "gam"},
		{name: "long SAVED.GAM", gamLength: SavedGameSize + 1, oollLength: SavedObjectsSize, path: "gam"},
		{name: "short SAVED.OOL", gamLength: SavedGameSize, oollLength: SavedObjectsSize - 1, path: "ool"},
		{name: "long SAVED.OOL", gamLength: SavedGameSize, oollLength: SavedObjectsSize + 1, path: "ool"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			gamPath := filepath.Join(directory, "SAVED.GAM")
			if err := os.WriteFile(gamPath, make([]byte, test.gamLength), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "SAVED.OOL"), make([]byte, test.oollLength), 0o600); err != nil {
				t.Fatal(err)
			}

			bundle, err := ReadBundleContext(t.Context(), gamPath)
			if bundle != (Bundle{}) {
				t.Fatal("invalid source length returned a partial bundle")
			}
			assertValidationIssue(t, err, test.path, "invalid_length", -1)
		})
	}
}

func TestReadBundleRejectsHugeFileBeforeAllocation(t *testing.T) {
	directory := t.TempDir()
	gamPath := filepath.Join(directory, "SAVED.GAM")
	file, err := os.Create(gamPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(1 << 30); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = ReadBundleContext(t.Context(), gamPath)
	assertValidationIssue(t, err, "gam", "invalid_length", -1)
}

func TestReadBundleContextHonorsPreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ReadBundleContext(ctx, filepath.Join(t.TempDir(), "SAVED.GAM"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadBundleContext() error = %v, want context.Canceled", err)
	}
}

func TestResolveSavedGamePathBoundsDirectoryEnumeration(t *testing.T) {
	directory := t.TempDir()
	for index := 0; index <= saveDirectoryLimit; index++ {
		if err := os.Mkdir(filepath.Join(directory, fmt.Sprintf("entry-%04d", index)), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	_, err := ResolveSavedGamePathContext(context.Background(), directory, directory)
	assertValidationIssue(t, err, "gam", "directory_too_large", -1)
}

func assertValidationIssue(t *testing.T, err error, path, code string, offset int) {
	t.Helper()
	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("error = %T %v, want *ValidationError", err, err)
	}
	if len(validationError.Issues) != 1 {
		t.Fatalf("issues = %#v, want one", validationError.Issues)
	}
	issue := validationError.Issues[0]
	if issue.Path != path || issue.Code != code || issue.Offset != offset || issue.Message == "" {
		t.Fatalf("issue = %#v, want path=%q code=%q offset=%d and a message", issue, path, code, offset)
	}
}

func TestResolveSavedGamePathAcceptsDirectoryFileCaseAndHomeShorthand(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "Original Saves")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	gamPath := filepath.Join(directory, "saved.gam")
	if err := os.WriteFile(gamPath, []byte("gam"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SaVeD.OoL"), []byte("ool"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, input := range []string{
		directory,
		filepath.Join(directory, "SAVED.GAM"),
		filepath.Join("~", "Original Saves"),
	} {
		resolved, err := ResolveSavedGamePathContext(context.Background(), input, home)
		if err != nil {
			t.Fatalf("ResolveSavedGamePathContext(%q) error = %v", input, err)
		}
		if resolved != gamPath {
			t.Fatalf("ResolveSavedGamePathContext(%q) = %q, want %q", input, resolved, gamPath)
		}
	}
}

func TestResolveSavedGamePathRejectsMissingAndAmbiguousPairs(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "SAVED.GAM"), []byte("gam"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveSavedGamePathContext(context.Background(), directory, directory)
	assertValidationIssue(t, err, "ool", "missing_sibling", -1)

	if err := os.WriteFile(filepath.Join(directory, "SAVED.OOL"), []byte("ool"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "saved.gam"), []byte("gam"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, readErr := os.ReadDir(directory)
	if readErr != nil {
		t.Fatal(readErr)
	}
	gamMatches := 0
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), "SAVED.GAM") {
			gamMatches++
		}
	}
	if gamMatches < 2 {
		return // The host filesystem does not permit case-distinct sibling names.
	}
	_, err = ResolveSavedGamePathContext(context.Background(), directory, directory)
	assertValidationIssue(t, err, "gam", "ambiguous_sibling", -1)
}
