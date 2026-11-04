package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Large application packages contain multiple independently owned scenes. Fixed
// coarse contexts release native process-wide resources between features, without
// a subprocess per test or a caller-controlled selection that could hide tests.
const nativeCohortFileThreshold = 128

// nativeSelectionLimit bounds one cohort process's -test.run pattern. Windows
// rejects command lines over 32,767 characters, so a large cohort runs as several
// consecutive processes, each still a fixed, complete slice of the cohort.
const nativeSelectionLimit = 16_000

func nativeCohort(name string) string {
	switch {
	case containsTerm(name, "Combat", "Projectile", "Corpser", "FleeChevron", "AttackBrackets", "Ettin"):
		return "combat"
	case containsTerm(name, "Dungeon", "Doom"):
		return "dungeon"
	case containsTerm(name, "Terrain", "SurfaceCamera", "Daylight"):
		return "surface"
	case containsTerm(name, "Moongate", "Sky", "Astronomy"):
		return "astronomy"
	case containsTerm(name, "Intro", "CharacterCreation", "CompletedCharacter", "ReturnView",
		"MainMenu", "WindDisplay", "MapBorder"):
		return "title"
	case containsTerm(name, "Talk", "Sign", "Runic", "Journal", "ZStats", "Ready", "Provision",
		"Feedback", "Configuration", "Spell", "Input", "ItemSelection"):
		return "dialogs"
	default:
		return "ui"
	}
}

func containsTerm(name string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(name, term) {
			return true
		}
	}

	return false
}

func nativeBinaryArguments(settings options, directory, binary, name, selection, list string) []string {
	return []string{"-C", directory, "tool", "test2json", "-t", "-p", name, binary,
		"-test.v=test2json", "-test.parallel=1", "-test.count=1", "-test.skip=^$", "-test.bench=", "-test.fuzz=",
		"-test.short=" + strconv.FormatBool(settings.short), "-test.timeout=180s",
		"-test.run=" + selection, "-test.list=" + list}
}

func nativeWorkloads(reader io.Reader, name string) ([]string, error) {
	decoder := json.NewDecoder(reader)
	names := make(map[string]bool)
	terminal := ""
	for {
		var event testEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, fmt.Errorf("decoding native discovery: %w", err)
		}
		if event.Package != name || event.Test != "" {
			return nil, fmt.Errorf("%w: unexpected assertion/package in native discovery", errGate)
		}
		if event.Action == actionPass || event.Action == actionFail || event.Action == actionSkip {
			if terminal != "" {
				return nil, fmt.Errorf("%w: duplicate native discovery terminal", errGate)
			}
			terminal = event.Action
		}
		if event.Action != actionOutput {
			continue
		}
		if err := collectNativeNames(event.Output, names); err != nil {
			return nil, err
		}
	}
	if terminal != actionPass || len(names) == 0 {
		return nil, fmt.Errorf("%w: incomplete or empty native discovery for %s", errGate, name)
	}
	result := make([]string, 0, len(names))
	for workload := range names {
		result = append(result, workload)
	}
	sort.Strings(result)

	return result, nil
}

func collectNativeNames(output string, names map[string]bool) error {
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "Test") && !strings.HasPrefix(line, "Example") && !strings.HasPrefix(line, "Fuzz") {
			continue
		}
		if strings.ContainsAny(line, " /\t\r") {
			continue
		}
		if names[line] {
			return fmt.Errorf("%w: duplicate discovered native workload %s", errGate, line)
		}
		names[line] = true
	}

	return nil
}

func nativeGCEnvironment(profile string, environment []string) []string {
	if profile != profileConstrained {
		return environment
	}
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if key == "GOGC" || (runtime.GOOS == osWindows && strings.EqualFold(key, "GOGC")) {
			return environment
		}
	}

	return append(slices.Clone(environment), "GOGC=50")
}

func runNativeCohorts(ctx context.Context, settings options, environment []string, name string,
	result *packageResult, stdout, stderr io.Writer, runner goRunner,
) (returnErr error) {
	// The opt-in constrained profile collects transient Go owners sooner so
	// native cleanup can run between draws. Host retains ordinary Go defaults;
	// every profile preserves an explicit caller's GC setting.
	environment = nativeGCEnvironment(settings.profile, environment)
	if result.directory == "" {
		return fmt.Errorf("%w: native application lacks its package directory", errGate)
	}
	temporary, err := os.MkdirTemp("", "native-test-contexts-")
	if err != nil {
		return fmt.Errorf("creating native test build directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(temporary); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("removing native test build: %w", err))
		}
	}()
	binary := filepath.Join(temporary, "application.test")
	if runtime.GOOS == osWindows {
		binary += ".exe"
	}
	build := []string{"test", "-c", goSerialFlag, "-race=" + strconv.FormatBool(settings.race), "-o", binary, name}
	if err := runner(ctx, build, environment, stdout, stderr); err != nil {
		return fmt.Errorf("building native application tests: %w", err)
	}
	var listing bytes.Buffer
	discover := nativeBinaryArguments(settings, result.directory, binary, name, ".", ".")
	if err := runner(ctx, discover, environment, &listing, stderr); err != nil {
		return fmt.Errorf("discovering native application tests: %w", err)
	}
	workloads, err := nativeWorkloads(&listing, name)
	if err != nil {
		return err
	}
	groups := make(map[string][]string)
	for _, workload := range workloads {
		group := nativeCohort(workload)
		groups[group] = append(groups[group], workload)
	}
	for _, group := range []string{"combat", "dungeon", "surface", "astronomy", "title", "dialogs", "ui"} {
		selected := groups[group]
		if len(selected) == 0 {
			continue
		}
		parts := nativeSelectionParts(selected, nativeSelectionLimit)
		_, err := fmt.Fprintf(stderr, "Native cohort %s: %s (%d top-level workloads, %d process(es))\n",
			name, group, len(selected), len(parts))
		if err != nil {
			return fmt.Errorf("reporting native cohort: %w", err)
		}
		for _, part := range parts {
			err = runNativeCohort(ctx, settings, environment, binary, name, part, result, stdout, stderr, runner)
			if err != nil {
				return err
			}
		}
	}
	result.terminal = actionPass

	return nil
}

// nativeSelection anchors and escapes workload names as one -test.run pattern.
func nativeSelection(selected []string) string {
	patterns := make([]string, len(selected))
	for index, workload := range selected {
		patterns[index] = regexp.QuoteMeta(workload)
	}

	return "^(" + strings.Join(patterns, "|") + ")$"
}

// nativeSelectionParts splits selected, in order, into the fewest consecutive
// parts whose selection patterns fit within limit. A single over-long name still
// forms its own part rather than being dropped.
func nativeSelectionParts(selected []string, limit int) [][]string {
	const frame = len("^(") + len(")$")
	var parts [][]string
	start, length := 0, frame
	for index, workload := range selected {
		size := len(regexp.QuoteMeta(workload))
		if index > start {
			if length+1+size > limit {
				parts = append(parts, selected[start:index])
				start, length = index, frame
			} else {
				size++ // The "|" separator before this name.
			}
		}
		length += size
	}
	if start < len(selected) {
		parts = append(parts, selected[start:])
	}

	return parts
}

func runNativeCohort(ctx context.Context, settings options, environment []string, binary, name string,
	selected []string, result *packageResult, stdout, stderr io.Writer, runner goRunner,
) error {
	local := &packageResult{hasTests: true, tests: make(map[string]*testResult), terminal: "",
		scope: result.scope, selected: true, directory: result.directory, cohorts: false}
	expected := map[string]*packageResult{name: local}
	arguments := nativeBinaryArguments(settings, result.directory, binary, name, nativeSelection(selected), "")
	runErr := runTestPackages(ctx, arguments, environment, expected, stdout, stderr, runner)
	mergeErr := mergeNativeCohort(name, selected, result, local)

	return errors.Join(runErr, mergeErr, validateResults(expected, settings.strict))
}

func mergeNativeCohort(name string, selected []string, result, local *packageResult) error {
	allowed := make(map[string]bool, len(selected))
	var problems []error
	for _, workload := range selected {
		allowed[workload] = true
		if test := local.tests[workload]; test == nil || test.terminal == "" {
			problems = append(problems, fmt.Errorf("%w: native workload omitted: %s/%s", errGate, name, workload))
		}
	}
	for testName, test := range local.tests {
		top, _, _ := strings.Cut(testName, "/")
		if !allowed[top] || result.tests[testName] != nil {
			problems = append(problems,
				fmt.Errorf("%w: unexpected or repeated native assertion: %s/%s", errGate, name, testName))

			continue
		}
		result.tests[testName] = test
	}

	return errors.Join(problems...)
}
