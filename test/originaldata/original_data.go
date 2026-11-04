// Package originaldata locates optional copyrighted game data for integration tests.
package originaldata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/files"
)

// ErrUnavailable means no optional installation was selected or found. Errors
// from an explicitly selected or present installation never wrap this sentinel.
var ErrUnavailable = errors.New("optional original game data unavailable")

// Required enables the provisioned-data test contract. Other test skips (such as
// platform exclusions) remain the responsibility of the gate's skip inventory.
func Required() bool {
	return os.Getenv("ULTIMA5_REQUIRE_DATA") == "1"
}

// ResolveDirectory selects PATH before DIR, then repository OLD. It never reads
// HOME or falls back after an explicit selection, even when that path is broken.
// Successful paths are absolute and symlink-resolved for reference-cache keys.
func ResolveDirectory() (string, error) {
	for _, variable := range []string{"ULTIMA5_DATA_PATH", "ULTIMA5_DATA_DIR"} {
		if directory := os.Getenv(variable); directory != "" {
			return canonicalDirectory(directory)
		}
	}
	root := repositoryRoot()
	if root == "" {
		return "", ErrUnavailable
	}
	directory := filepath.Join(root, "OLD")
	if _, err := os.Stat(directory); errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: %s; set ULTIMA5_DATA_PATH or ULTIMA5_DATA_DIR", ErrUnavailable, directory)
	}

	return canonicalDirectory(directory)
}

func canonicalDirectory(directory string) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolving original data directory %q: %w", directory, err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolving original data directory %q: %w", directory, err)
	}
	// The developer's own data path, already made absolute and symlink-free.
	info, err := os.Stat(canonical) //nolint:gosec // G703: trusted local test configuration.
	if err != nil {
		return "", fmt.Errorf("reading original data directory %q: %w", canonical, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("original data directory %q: %w", canonical, os.ErrInvalid)
	}

	return canonical, nil
}

// Check fails broken selected data, preserving the loader's diagnostic. Only
// genuinely absent optional data skips, and strict provisioned mode fails that too.
func Check(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if errors.Is(err, ErrUnavailable) && !Required() {
		t.Skipf("%v", err)
	}
	t.Fatalf("original game data: %v (ULTIMA5_REQUIRE_DATA=%q)", err, os.Getenv("ULTIMA5_REQUIRE_DATA"))
}

// talkFiles are the original conversation files, one per small-map location type.
func talkFiles() []string {
	return []string{files.CastleTlk, files.TowneTlk, files.DwellingTlk, files.KeepTlk}
}

// Directory selects original data and checks required files. By default it
// checks conversation inputs; callers with other needs can supply filenames.
// Once a source is selected, missing files are failures, not optional skips.
func Directory(t *testing.T, required ...string) string {
	t.Helper()
	directory, err := ResolveDirectory()
	Check(t, err)
	if len(required) == 0 {
		required = append([]string{files.DataOvl}, talkFiles()...)
	}
	RequireFiles(t, directory, required...)

	return directory
}

// RequireFiles checks additional inputs in an already-selected installation.
func RequireFiles(t *testing.T, directory string, filenames ...string) {
	t.Helper()
	for _, filename := range filenames {
		candidate := filepath.Join(directory, filename)
		info, err := os.Stat(candidate)
		if err != nil {
			Check(t, fmt.Errorf("reading selected original data file %q: %w", candidate, err))
		}
		if !info.Mode().IsRegular() {
			Check(t, fmt.Errorf("selected original data file %q: %w", candidate, os.ErrInvalid))
		}
	}
}

func repositoryRoot() string {
	directory, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return ""
		}
		directory = parent
	}
}
