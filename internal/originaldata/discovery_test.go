package originaldata

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecommendedDirectoryUsesResolvedHomeAcrossPlatforms(t *testing.T) {
	for _, goos := range []string{"windows", "darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			home := t.TempDir()
			assert.Equal(t, filepath.Join(home, "UltimaV"), RecommendedDirectory(HostProfile{
				GOOS: goos, HomeDirectory: home,
			}))
		})
	}
}

func TestDiscoverUsesHeroicMetadataAndDeduplicatesCanonicalPaths(t *testing.T) {
	home := t.TempDir()
	install := filepath.Join(home, "Games", "Ultima V")
	require.NoError(t, os.MkdirAll(install, 0o700))
	metadataPath := filepath.Join(home, ".config", "heroic", "gog_store", "installed.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o700))
	metadata, err := json.Marshal(map[string]any{
		"installed": []map[string]string{{"appName": GOGGameID, "install_path": install}},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(metadataPath, metadata, 0o600))
	alias := filepath.Join(home, "alias")
	require.NoError(t, os.Symlink(install, alias))

	report := Discover(context.Background(), DiscoveryOptions{
		Host: HostProfile{
			GOOS: "linux", HomeDirectory: home,
			Environment: map[string]string{"XDG_CONFIG_HOME": filepath.Join(home, ".config")},
		},
		ConfiguredDirectory: alias,
		Validate: func(_ context.Context, path string) error {
			if canonicalIdentity(path, "linux") == canonicalIdentity(install, "linux") {
				return nil
			}

			return errors.New("not a fixture installation")
		},
	})

	require.Len(t, report.Candidates, 1)
	assert.Equal(t, canonicalIdentity(install, "linux"), canonicalIdentity(report.Candidates[0].Path, "linux"))
	assert.Equal(t, filepath.Join(home, "UltimaV"), report.RecommendedDirectory)
}

func TestDiscoverResolvesCurrentAndLegacyMacBundleLayouts(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "Applications", "Ultima V™.app", "Contents", "Resources", "game")
	legacy := filepath.Join(
		root, "Legacy.app", "Contents", "Resources", "game", "Ultima 5.app", "Contents", "Resources",
		"Ultima 5.boxer", "C ULTIMA5.harddisk",
	)
	for _, directory := range []string{current, legacy} {
		require.NoError(t, os.MkdirAll(directory, 0o700))
	}

	report := Discover(context.Background(), DiscoveryOptions{
		Host: HostProfile{GOOS: "darwin", HomeDirectory: root},
		AdditionalSeeds: []Seed{
			{Path: filepath.Join(root, "Applications", "Ultima V™.app"), Source: SourceGOG},
			{Path: filepath.Join(root, "Legacy.app"), Source: SourceGOG},
		},
		Validate: func(_ context.Context, path string) error {
			if path == current || path == legacy {
				return nil
			}

			return errors.New("not game data")
		},
	})

	assert.ElementsMatch(t, []string{current, legacy}, StableCandidatePaths(report))
}

func TestDiscoverCancellationReturnsNoLateCandidates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report := Discover(ctx, DiscoveryOptions{
		Host:     HostProfile{GOOS: "linux", HomeDirectory: t.TempDir()},
		Validate: func(context.Context, string) error { return nil },
	})

	assert.True(t, report.Cancelled)
	assert.Empty(t, report.Candidates)
}

func TestDiscoverOnlyUsesHomeFallbackAndExplicitOldPath(t *testing.T) {
	home := t.TempDir()
	t.Chdir(home)
	for _, directory := range []string{"OLD", "UltimaV", "Documents/UltimaV"} {
		require.NoError(t, os.MkdirAll(directory, 0o700))
	}
	options := DiscoveryOptions{
		Host:     HostProfile{GOOS: "linux", HomeDirectory: home},
		Validate: func(context.Context, string) error { return nil },
	}
	report := Discover(context.Background(), options)
	assert.Equal(t, []string{filepath.Join(home, "UltimaV")}, StableCandidatePaths(report))
	options.ConfiguredDirectory = "./OLD"
	report = Discover(context.Background(), options)
	assert.ElementsMatch(t, []string{filepath.Join(home, "OLD"), filepath.Join(home, "UltimaV")}, StableCandidatePaths(report))
	assert.Equal(t, SourceConfigured, report.Candidates[0].Source)
}

func TestRecommendedDirectoryDoesNotFallBackToWorkingDirectory(t *testing.T) {
	assert.Empty(t, RecommendedDirectory(HostProfile{}))
	assert.Empty(t, RecommendedDirectory(HostProfile{HomeDirectory: "relative"}))
	report := Discover(context.Background(), DiscoveryOptions{Host: HostProfile{GOOS: "linux"}})
	assert.Empty(t, report.Candidates)
	assert.Empty(t, report.CheckedRoots)
	require.Len(t, report.Details, 1)
	assert.Contains(t, report.Details[0].Reason, "absolute home folder")
}

func TestDiscoverEnforcesSeedAndHeroicMetadataBounds(t *testing.T) {
	home := t.TempDir()
	metadataPath := filepath.Join(home, ".config", "heroic", "gog_store", "installed.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o700))
	require.NoError(t, os.WriteFile(metadataPath, make([]byte, heroicMetadataMaximumBytes+1), 0o600))
	additional := make([]Seed, discoverySeedLimit+1)
	for index := range additional {
		additional[index] = Seed{Path: filepath.Join(home, "seed", strconv.Itoa(index)), Source: SourceCommon}
	}

	report := Discover(context.Background(), DiscoveryOptions{
		Host: HostProfile{
			GOOS: "linux", HomeDirectory: home,
			Environment: map[string]string{"XDG_CONFIG_HOME": filepath.Join(home, ".config")},
		},
		AdditionalSeeds: additional,
		Validate:        func(context.Context, string) error { return nil },
	})

	assert.Len(t, report.CheckedRoots, discoverySeedLimit)
	assert.Condition(t, func() bool {
		return slices.ContainsFunc(report.Details, func(detail Detail) bool {
			return strings.Contains(detail.Reason, "metadata exceeds")
		})
	})
	assert.Condition(t, func() bool {
		return slices.ContainsFunc(report.Details, func(detail Detail) bool {
			return strings.Contains(detail.Reason, "bounded search stopped")
		})
	})
}

func TestWindowsGOGSeedsAcceptCustomRecordsAndRejectOtherPublishers(t *testing.T) {
	seeds := WindowsGOGSeeds([]WindowsGOGRegistryRecord{
		{GamePath: `D:\Games\Ultima V`, UninstallPath: "", UninstallPublisher: ""},
		{GamePath: "", UninstallPath: `E:\GOG\Ultima 5`, UninstallPublisher: "GOG.com"},
		{GamePath: "", UninstallPath: `C:\Unrelated`, UninstallPublisher: "Other"},
	})

	require.Len(t, seeds, 2)
	assert.Equal(t, `D:\Games\Ultima V`, seeds[0].Path)
	assert.Equal(t, "GOG installation record", seeds[0].Label)
	assert.Equal(t, `E:\GOG\Ultima 5`, seeds[1].Path)
	assert.Equal(t, "GOG uninstall record", seeds[1].Label)
}

func TestCommonSeedsUseGOGRootsAndProgramFilesFallback(t *testing.T) {
	// A temporary directory is absolute on every host; `\Program Files` lacks a
	// drive letter and so is not absolute when the test itself runs on Windows.
	programFiles := filepath.Join(t.TempDir(), "Program Files")
	seeds := commonSeeds(HostProfile{
		GOOS: "windows", HomeDirectory: filepath.Join(string(filepath.Separator), "Users", "Avatar"),
		Environment: map[string]string{"ProgramFiles": programFiles, "SystemDrive": "C:"},
	})

	paths := make([]string, 0, len(seeds))
	for _, seed := range seeds {
		paths = append(paths, seed.Path)
	}
	gogRoot := filepath.Join(programFiles, "GOG Galaxy", "Games")
	assert.Contains(t, paths, gogRoot)
	assert.Contains(t, boundedLayouts(gogRoot, "windows"),
		filepath.Join(gogRoot, "Ultima 4-5-6", "Ultima 5"))
}

// StableCandidatePaths returns sorted paths for diagnostics and tests.
func StableCandidatePaths(report Report) []string {
	paths := make([]string, len(report.Candidates))
	for index, candidate := range report.Candidates {
		paths[index] = candidate.Path
	}
	sort.Strings(paths)

	return paths
}
