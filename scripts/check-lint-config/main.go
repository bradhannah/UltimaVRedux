// Command check-lint-config verifies the repository config and exercises errcheck.
// Run from the repository root with a preprovisioned v2.15.0 golangci-lint binary:
// go run ./scripts/check-lint-config /absolute/path/to/golangci-lint
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	pinnedVersion = "2.15.0"
	argumentCount = 2
	checkTimeout  = 10 * time.Minute
	fixtureMode   = 0o600
	settingCount  = 2
)

//go:embed testdata/fixture.go.txt
var fixture string

var errRegression = errors.New("lint config regression")

func main() {
	if err := run(); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != argumentCount {
		return fmt.Errorf("%w: pass the preprovisioned v%s golangci-lint binary path", errRegression, pinnedVersion)
	}
	binary, err := exec.LookPath(os.Args[1])
	if err != nil {
		return fmt.Errorf("locating golangci-lint: %w", err)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return fmt.Errorf("resolving golangci-lint: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	versionJSON, err := invoke(ctx, binary, "", "version", "--json")
	if err != nil {
		return err
	}
	var version struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(versionJSON, &version); err != nil {
		return fmt.Errorf("decoding linter version: %w", err)
	}
	if version.Version != pinnedVersion {
		return fmt.Errorf("%w: need v%s, got %q", errRegression, pinnedVersion, version.Version)
	}
	if _, err := invoke(ctx, binary, "", "config", "verify", "--config", ".golangci.yml"); err != nil {
		return err
	}
	config, err := os.ReadFile(".golangci.yml")
	if err != nil {
		return fmt.Errorf("reading repository config: %w", err)
	}
	if err := requireWholeTreeLint(config); err != nil {
		return err
	}
	directory, err := os.MkdirTemp("", "lint-config-")
	if err != nil {
		return fmt.Errorf("creating fixture module: %w", err)
	}
	// Keep cleanup errors observable without hiding the check's original failure.
	checkErr := checkFixture(ctx, binary, directory, config)
	if err := os.RemoveAll(directory); err != nil {
		return errors.Join(checkErr, fmt.Errorf("removing fixture module: %w", err))
	}

	return checkErr
}

// requireWholeTreeLint keeps the gate enforcing every file: a revision cutoff
// would let inherited findings return unnoticed.
func requireWholeTreeLint(source []byte) error {
	var config map[string]any
	if err := yaml.Unmarshal(source, &config); err != nil {
		return fmt.Errorf("parsing repository config: %w", err)
	}
	issues, ok := config["issues"].(map[string]any)
	if !ok {
		return fmt.Errorf("%w: expected issues mapping", errRegression)
	}
	for _, key := range []string{"new", "new-from-rev", "new-from-patch", "new-from-merge-base"} {
		if _, exists := issues[key]; exists {
			return fmt.Errorf("%w: issues.%s would stop linting the whole tree", errRegression, key)
		}
	}

	return nil
}

func checkFixture(ctx context.Context, binary, directory string, config []byte) error {
	for name, content := range map[string][]byte{
		"go.mod":        []byte("module lintconfigfixture\n\ngo 1.24.0\n"),
		"fixture.go":    []byte(fixture),
		".golangci.yml": config,
	} {
		if err := os.WriteFile(filepath.Join(directory, name), content, fixtureMode); err != nil {
			return fmt.Errorf("writing fixture %s: %w", name, err)
		}
	}
	// A successful process is required even with findings. Infrastructure failures
	// must not count as proof that errcheck found the intentionally ignored errors.
	output, err := invoke(ctx, binary, directory, "run", "--config", ".golangci.yml",
		"--issues-exit-code=0", "--show-stats=false", "--output.text.path=", "--output.json.path=stdout", "./...")
	if err != nil {
		return err
	}
	//nolint:tagliatelle // Field names are the golangci-lint JSON protocol.
	var report struct {
		Issues []struct {
			FromLinter string `json:"FromLinter"`
			Pos        struct {
				Filename string `json:"Filename"`
				Line     int    `json:"Line"`
			} `json:"Pos"`
		} `json:"Issues"`
	}
	if err := json.Unmarshal(output, &report); err != nil {
		return fmt.Errorf("decoding fixture diagnostics: %w", err)
	}
	want := make(map[int]bool)
	for index, line := range strings.Split(fixture, "\n") {
		if strings.Contains(line, "want: errcheck") {
			want[index+1] = false
		}
	}
	for _, issue := range report.Issues {
		if issue.FromLinter != "errcheck" {
			continue
		}
		if _, exists := want[issue.Pos.Line]; !exists || filepath.Base(issue.Pos.Filename) != "fixture.go" {
			return fmt.Errorf("%w: unexpected errcheck finding at %+v", errRegression, issue.Pos)
		}
		want[issue.Pos.Line] = true
	}
	for line, found := range want {
		if !found {
			return fmt.Errorf("%w: missing errcheck finding on fixture.go:%d", errRegression, line)
		}
	}
	if len(want) != settingCount {
		return fmt.Errorf("%w: fixture must exercise both errcheck settings", errRegression)
	}
	const success = "lint config: schema valid; 2/2 errcheck regressions detected; checked controls clean"
	if _, err := fmt.Fprintln(os.Stdout, success); err != nil {
		return fmt.Errorf("reporting check result: %w", err)
	}

	return nil
}

func invoke(ctx context.Context, binary, directory string, args ...string) ([]byte, error) {
	//nolint:gosec // Explicit operator-supplied local linter binary, checked for the repository pin before use.
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOWORK=off")
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("golangci-lint %v: %w: %s%s", args, err, stderr.String(), output)
	}

	return output, nil
}
