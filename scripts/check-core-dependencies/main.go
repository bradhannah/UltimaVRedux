// Command check-core-dependencies checks the actual production and test import
// closures of the gameplay roots: neither may reach presentation code, and the
// production closure may not reach test code. Run from the repository root.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

const (
	modulePath  = "github.com/bradhannah/UltimaVRedux"
	listTimeout = 2 * time.Minute
)

var errDependencyGraph = errors.New("core dependency boundary failed")

// These fields retain go list's variant identities and import remapping. Do not
// collapse "package [root.test]" into "package": their imports can differ.
//
//nolint:tagliatelle // Field names are the go list JSON protocol.
type packageMetadata struct {
	ImportPath   string            `json:"ImportPath"`
	ForTest      string            `json:"ForTest"`
	Imports      []string          `json:"Imports"`
	ImportMap    map[string]string `json:"ImportMap"`
	TestGoFiles  []string          `json:"TestGoFiles"`
	XTestGoFiles []string          `json:"XTestGoFiles"`
	Incomplete   bool              `json:"Incomplete"`
	Error        *packageError     `json:"Error"`
	DepsErrors   []packageError    `json:"DepsErrors"`
}

//nolint:tagliatelle // Field names are the go list JSON protocol.
type packageError struct {
	Err string `json:"Err"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
	err := run(ctx)
	cancel()
	if err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	roots := []string{
		modulePath + "/internal/game_state",
		modulePath + "/internal/party_state",
		modulePath + "/internal/map_state",
		modulePath + "/internal/ai",
		modulePath + "/internal/references",
		modulePath + "/internal/config",
	}
	args := append([]string{"list", "-mod=readonly", "-deps", "-test", "-e", "-json"}, roots...)
	//nolint:gosec // Executable, flags and package roots are fixed above.
	command := exec.CommandContext(ctx, "go", args...)
	command.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("loading core production/test metadata: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	packages, err := decodePackages(bytes.NewReader(output))
	if err != nil {
		return err
	}

	return checkGraph(packages, roots)
}

func decodePackages(reader io.Reader) ([]packageMetadata, error) {
	decoder := json.NewDecoder(reader)
	var packages []packageMetadata
	for {
		var metadata packageMetadata
		if err := decoder.Decode(&metadata); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, fmt.Errorf("decoding go list metadata: %w", err)
		}
		packages = append(packages, metadata)
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("%w: empty go list metadata", errDependencyGraph)
	}

	return packages, nil
}

func checkGraph(packages []packageMetadata, roots []string) error {
	graph := make(map[string]packageMetadata, len(packages))
	packages = slices.Clone(packages)
	slices.SortFunc(packages, func(left, right packageMetadata) int {
		return strings.Compare(left.ImportPath, right.ImportPath)
	})
	for _, metadata := range packages {
		if metadata.ImportPath == "" {
			return fmt.Errorf("%w: package has no import path", errDependencyGraph)
		}
		if _, exists := graph[metadata.ImportPath]; exists {
			return fmt.Errorf("%w: duplicate package %s", errDependencyGraph, metadata.ImportPath)
		}
		if err := validateLoadedPackage(metadata); err != nil {
			return err
		}
		graph[metadata.ImportPath] = metadata
	}
	for _, metadata := range packages {
		for _, dependency := range resolvedImports(metadata) {
			if _, exists := graph[dependency]; !exists {
				return fmt.Errorf("%w: missing package on path %s -> %s", errDependencyGraph, metadata.ImportPath, dependency)
			}
		}
	}
	starts, err := rootVariants(graph, roots)
	if err != nil {
		return err
	}
	var violations []error
	for _, start := range starts {
		if witness := forbiddenWitness(graph, start, forbiddenPackage); len(witness) != 0 {
			violations = append(violations, fmt.Errorf("%w: %s", errDependencyGraph, strings.Join(witness, " -> ")))
		}
	}
	// Only the production roots are held to this rule; their test variants
	// reach test code by design.
	for _, root := range roots {
		if witness := forbiddenWitness(graph, root, testOnlyPackage); len(witness) != 0 {
			violations = append(violations, fmt.Errorf("%w: %s", errDependencyGraph, strings.Join(witness, " -> ")))
		}
	}

	return errors.Join(violations...)
}

func validateLoadedPackage(metadata packageMetadata) error {
	if metadata.Error != nil {
		return fmt.Errorf("%w: %s: %s", errDependencyGraph, metadata.ImportPath, metadata.Error.Err)
	}
	if len(metadata.DepsErrors) > 0 {
		messages := make([]string, len(metadata.DepsErrors))
		for index, loadError := range metadata.DepsErrors {
			messages[index] = loadError.Err
		}
		slices.Sort(messages)

		return fmt.Errorf("%w: %s: %s", errDependencyGraph, metadata.ImportPath, strings.Join(messages, "; "))
	}
	if metadata.Incomplete {
		return fmt.Errorf("%w: incomplete package %s", errDependencyGraph, metadata.ImportPath)
	}

	return nil
}

func rootVariants(graph map[string]packageMetadata, roots []string) ([]string, error) {
	if len(roots) == 0 {
		return nil, fmt.Errorf("%w: no roots selected", errDependencyGraph)
	}
	starts := slices.Clone(roots)
	slices.Sort(starts)
	for _, root := range slices.Clone(starts) {
		metadata, exists := graph[root]
		if !exists {
			return nil, fmt.Errorf("%w: missing root %s", errDependencyGraph, root)
		}
		var variants []string
		if len(metadata.TestGoFiles) > 0 {
			variants = append(variants, root+" ["+root+".test]")
		}
		if len(metadata.XTestGoFiles) > 0 {
			variants = append(variants, root+"_test ["+root+".test]")
		}
		if len(variants) > 0 {
			variants = append(variants, root+".test")
		}
		for _, variant := range variants {
			variantMetadata, found := graph[variant]
			if !found || (variant != root+".test" && variantMetadata.ForTest != root) {
				return nil, fmt.Errorf("%w: missing test variant %s", errDependencyGraph, variant)
			}
			starts = append(starts, variant)
		}
	}
	slices.Sort(starts)

	return slices.Compact(starts), nil
}

func resolvedImports(metadata packageMetadata) []string {
	imports := make([]string, 0, len(metadata.Imports))
	for _, dependency := range metadata.Imports {
		if mapped, found := metadata.ImportMap[dependency]; found {
			dependency = mapped
		}
		if dependency != "C" { // C is a cgo pseudo-import, not a Go package.
			imports = append(imports, dependency)
		}
	}
	slices.Sort(imports)

	return slices.Compact(imports)
}

// Breadth-first traversal with sorted edges yields a deterministic shortest
// witness, including test-only and remapped package identities.
func forbiddenWitness(graph map[string]packageMetadata, root string, forbidden func(string) bool) []string {
	queue := []string{root}
	parent := map[string]string{root: ""}
	for index := 0; index < len(queue); index++ {
		current := queue[index]
		if forbidden(current) {
			var witness []string
			for node := current; node != ""; node = parent[node] {
				witness = append(witness, node)
			}
			slices.Reverse(witness)

			return witness
		}
		for _, dependency := range resolvedImports(graph[current]) {
			if _, visited := parent[dependency]; !visited {
				parent[dependency] = current
				queue = append(queue, dependency)
			}
		}
	}

	return nil
}

func forbiddenPackage(identity string) bool {
	path, _, _ := strings.Cut(identity, " [")
	if path == modulePath+"/internal/sprites/indexes" {
		return false
	}
	for _, boundary := range []string{
		"github.com/hajimehoshi/ebiten",
		"github.com/ebitengine",
		"github.com/bradhannah/berad-sdk/modules/berad-text",
		"github.com/bradhannah/berad-sdk/modules/berad-graphics",
		"github.com/bradhannah/berad-sdk/modules/berad-input",
		modulePath + "/internal/display",
		modulePath + "/internal/text",
		modulePath + "/internal/ui",
		modulePath + "/internal/sprites",
		modulePath + "/internal/audio",
		modulePath + "/pkg/input",
		modulePath + "/cmd",
	} {
		if path == boundary || strings.HasPrefix(path, boundary+"/") {
			return true
		}
	}

	return false
}

// testOnlyPackage reports code that only tests may link: the standard testing
// package and this module's test support. A gameplay root that imports either
// puts test code into the game.
func testOnlyPackage(identity string) bool {
	path, _, _ := strings.Cut(identity, " [")

	return path == "testing" || path == modulePath+"/test" || strings.HasPrefix(path, modulePath+"/test/")
}
