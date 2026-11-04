package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

const (
	actionPass   = "pass"
	actionFail   = "fail"
	actionSkip   = "skip"
	actionOutput = "output"
)

//nolint:tagliatelle // These fields use the go list JSON protocol's names.
type packageMetadata struct {
	ImportPath   string   `json:"ImportPath"`
	Name         string   `json:"Name"`
	Dir          string   `json:"Dir"`
	Imports      []string `json:"Imports"`
	Deps         []string `json:"Deps"`
	TestGoFiles  []string `json:"TestGoFiles"`
	XTestGoFiles []string `json:"XTestGoFiles"`
	Incomplete   bool     `json:"Incomplete"`
	Error        *struct {
		Err string `json:"Err"`
	} `json:"Error"`
	DepsErrors []struct {
		Err string `json:"Err"`
	} `json:"DepsErrors"`
}

//nolint:tagliatelle // These fields use the go test -json protocol's names.
type testEvent struct {
	Action     string `json:"Action"`
	Package    string `json:"Package"`
	Test       string `json:"Test"`
	Output     string `json:"Output"`
	ImportPath string `json:"ImportPath"`
}

type packageResult struct {
	hasTests  bool
	terminal  string
	tests     map[string]*testResult
	scope     string
	selected  bool
	directory string
	cohorts   bool
}

type testResult struct {
	terminal string
	output   strings.Builder
}

func readPackages(reader io.Reader) (map[string]*packageResult, error) {
	packages, err := readPackageMetadata(reader)
	if err != nil {
		return nil, err
	}
	expected := make(map[string]*packageResult)
	for name, metadata := range packages {
		expected[name] = &packageResult{
			hasTests:  len(metadata.TestGoFiles)+len(metadata.XTestGoFiles) > 0,
			terminal:  "",
			tests:     make(map[string]*testResult),
			scope:     "",
			selected:  true,
			directory: metadata.Dir,
			cohorts:   metadata.Name == "main" && len(metadata.TestGoFiles) >= nativeCohortFileThreshold,
		}
	}

	return expected, nil
}

func readPackageMetadata(reader io.Reader) (map[string]packageMetadata, error) {
	packages := make(map[string]packageMetadata)
	decoder := json.NewDecoder(reader)
	for {
		var metadata packageMetadata
		if err := decoder.Decode(&metadata); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, fmt.Errorf("decoding go list inventory: %w", err)
		}
		if _, duplicate := packages[metadata.ImportPath]; metadata.ImportPath == "" || duplicate {
			return nil, fmt.Errorf("%w: empty or duplicate package %q in go list", errGate, metadata.ImportPath)
		}
		if metadata.Error != nil {
			return nil, fmt.Errorf("%w: loading %s: %s", errGate, metadata.ImportPath, metadata.Error.Err)
		}
		if len(metadata.DepsErrors) > 0 {
			return nil, fmt.Errorf("%w: loading dependencies of %s: %v", errGate, metadata.ImportPath, metadata.DepsErrors)
		}
		if metadata.Incomplete {
			return nil, fmt.Errorf("%w: incomplete package metadata for %s", errGate, metadata.ImportPath)
		}
		packages[metadata.ImportPath] = metadata
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("%w: empty go list package inventory", errGate)
	}

	return packages, nil
}

func readEvents(reader io.Reader, expected map[string]*packageResult) error {
	decoder := json.NewDecoder(reader)
	count := 0
	var problems []error
	for {
		var event testEvent
		if err := decoder.Decode(&event); err != nil {
			if !errors.Is(err, io.EOF) {
				problems = append(problems, fmt.Errorf("decoding go test JSON: %w", err))
			}

			break
		}
		count++
		if err := recordEvent(event, expected); err != nil {
			problems = append(problems, err)
		}
	}
	if count == 0 {
		problems = append(problems, fmt.Errorf("%w: empty go test event stream", errGate))
	}

	return errors.Join(problems...)
}

//nolint:cyclop,funlen // Keep protocol action validation and state transitions in one auditable switch.
func recordEvent(event testEvent, expected map[string]*packageResult) error {
	if event.Action == "build-output" {
		return nil
	}
	if event.Action == "build-fail" {
		return fmt.Errorf("%w: build failed for %s", errGate, event.ImportPath)
	}
	result := expected[event.Package]
	if result == nil {
		return fmt.Errorf("%w: event for unexpected package %q", errGate, event.Package)
	}
	if event.Test == "" {
		switch event.Action {
		case "start", "output":
			return nil
		case actionPass, actionFail, actionSkip:
			if result.terminal != "" {
				return fmt.Errorf("%w: duplicate package terminal for %s", errGate, event.Package)
			}
			result.terminal = event.Action

			return nil
		default:
			return fmt.Errorf("%w: invalid package action %q for %s", errGate, event.Action, event.Package)
		}
	}
	if result.terminal != "" {
		return fmt.Errorf("%w: test event after package terminal: %s/%s", errGate, event.Package, event.Test)
	}
	test := result.tests[event.Test]
	if event.Action == "run" {
		if test != nil {
			return fmt.Errorf("%w: duplicate test run for %s/%s", errGate, event.Package, event.Test)
		}
		result.tests[event.Test] = &testResult{} //nolint:exhaustruct_v5 // Test starts with no terminal or output.

		return nil
	}
	if test == nil {
		return fmt.Errorf("%w: test event without run: %s/%s", errGate, event.Package, event.Test)
	}
	switch event.Action {
	case "output":
		if test.terminal == "" {
			test.output.WriteString(event.Output)
		}
	case "pause", "cont":
		// Parallel tests may suspend and resume before their terminal event.
	case actionPass, actionFail, actionSkip:
		if test.terminal != "" {
			return fmt.Errorf("%w: duplicate test terminal for %s/%s", errGate, event.Package, event.Test)
		}
		test.terminal = event.Action
		if event.Action != actionSkip {
			test.output.Reset()
		}
	default:
		return fmt.Errorf("%w: unknown test action %q", errGate, event.Action)
	}

	return nil
}

func validateResults(expected map[string]*packageResult, strict bool) error {
	var problems []error
	for _, name := range slices.Sorted(maps.Keys(expected)) {
		result := expected[name]
		if result.terminal == "" {
			problems = append(problems, fmt.Errorf("%w: missing package terminal for %s (build or test interrupted)",
				errGate, name))
		} else if result.terminal == actionFail || (result.terminal == actionSkip && result.hasTests) {
			problems = append(problems, fmt.Errorf("%w: package %s ended with %s", errGate, name, result.terminal))
		}
		if result.hasTests && len(result.tests) == 0 {
			problems = append(problems, fmt.Errorf("%w: package %s has test files but emitted no test events", errGate, name))
		}
		for _, testName := range slices.Sorted(maps.Keys(result.tests)) {
			test := result.tests[testName]
			if test.terminal == "" || test.terminal == actionFail || (strict && test.terminal == actionSkip) {
				problems = append(problems, fmt.Errorf("%w: test %s/%s terminal=%q", errGate, name, testName, test.terminal))
			}
		}
	}

	return errors.Join(problems...)
}

func writeSummary(writer io.Writer, expected map[string]*packageResult) error {
	var summary strings.Builder
	fmt.Fprintf(&summary, "\nTest gate package inventory (%d expected):\n", len(expected))
	skips, tests, completed, selected := 0, 0, 0, 0
	for _, name := range slices.Sorted(maps.Keys(expected)) {
		result := expected[name]
		if !result.selected {
			fmt.Fprintf(&summary, "  %s: ASSIGNED %s (other required scope; not executed)\n", name, result.scope)

			continue
		}
		selected++
		terminal := result.terminal
		if terminal == "" {
			terminal = "MISSING"
		} else {
			completed++
		}
		fmt.Fprintf(&summary, "  %s: %s (test events: %d) [selected scope: %s]\n",
			name, terminal, len(result.tests), result.scope)
		tests += len(result.tests)
		for _, testName := range slices.Sorted(maps.Keys(result.tests)) {
			test := result.tests[testName]
			if test.terminal == actionSkip {
				skips++
				reason := strings.TrimSpace(test.output.String())
				if reason == "" {
					reason = "(no skip reason emitted by go test)"
				}
				fmt.Fprintf(&summary, "  SKIP %s/%s\n%s\n", name, testName, reason)
			}
		}
	}
	fmt.Fprintf(&summary, "Test gate totals: packages=%d/%d tests/subtests=%d skipped tests=%d\n",
		completed, selected, tests, skips)
	fmt.Fprintf(&summary, "Required scopes: logic and render; selected packages=%d inventory packages=%d "+
		"assigned other scope=%d\n",
		selected, len(expected), len(expected)-selected)
	if _, err := io.WriteString(writer, summary.String()); err != nil {
		return fmt.Errorf("writing test gate summary: %w", err)
	}

	return nil
}
