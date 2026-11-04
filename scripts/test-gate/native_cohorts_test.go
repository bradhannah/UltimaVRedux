package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func nativeDiscoveryFixture(t *testing.T, names ...string) string {
	t.Helper()
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	for _, name := range names {
		require.NoError(t, encoder.Encode(testEvent{Action: "output", Package: "app", Output: name + "\n"}))
	}
	require.NoError(t, encoder.Encode(testEvent{Action: actionPass, Package: "app"}))
	return buffer.String()
}

func nativeAssertionFixture(t *testing.T, name, terminal string) string {
	t.Helper()
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	for _, event := range []testEvent{
		{Action: "start", Package: "app"},
		{Action: "run", Package: "app", Test: name},
		{Action: "run", Package: "app", Test: name + "/child"},
		{Action: terminal, Package: "app", Test: name + "/child"},
		{Action: actionPass, Package: "app", Test: name},
		{Action: actionPass, Package: "app"},
	} {
		require.NoError(t, encoder.Encode(event))
	}
	return buffer.String()
}

func TestNativeCohortsBuildOnceAndExecuteDiscoveredWorkloadsExactlyOnce(t *testing.T) {
	settings := options{profile: profileHost, race: true, short: true, strict: true}
	result := &packageResult{hasTests: true, tests: make(map[string]*testResult), directory: t.TempDir()}
	var stdout, stderr bytes.Buffer
	builds, listings := 0, 0
	seen := make(map[string]int)
	runner := func(_ context.Context, arguments, environment []string, output, _ io.Writer) error {
		require.Empty(t, environment, "host native cohorts must not inject heap/GC settings")
		if arguments[0] == "test" {
			builds++
			require.Contains(t, arguments, "-c")
			require.Contains(t, arguments, "-p=1")
			require.Contains(t, arguments, "-race=true")
			return nil
		}
		require.Equal(t, []string{"-C", result.directory, "tool", "test2json", "-t", "-p", "app"}, arguments[:7])
		for _, flag := range []string{"-test.count=1", "-test.parallel=1", "-test.skip=^$", "-test.bench=", "-test.fuzz=", "-test.short=true"} {
			require.Contains(t, arguments, flag)
		}
		if arguments[len(arguments)-1] == "-test.list=." {
			listings++
			_, err := io.WriteString(output, nativeDiscoveryFixture(t, "TestCombatPaint", "ExampleTerrain", "FuzzPlain"))
			return err
		}
		pattern := arguments[len(arguments)-2]
		for _, name := range []string{"TestCombatPaint", "ExampleTerrain", "FuzzPlain"} {
			if strings.Contains(pattern, name) {
				seen[name]++
				_, err := io.WriteString(output, nativeAssertionFixture(t, name, actionPass))
				return err
			}
		}
		return errors.New("unknown selection")
	}
	require.NoError(t, runNativeCohorts(t.Context(), settings, nil, "app", result, &stdout, &stderr, runner))
	require.Equal(t, 1, builds)
	require.Equal(t, 1, listings)
	require.Equal(t, map[string]int{"TestCombatPaint": 1, "ExampleTerrain": 1, "FuzzPlain": 1}, seen)
	require.Len(t, result.tests, 6)
	require.Equal(t, actionPass, result.terminal)
}

func TestNativeSelectionPartsStayUnderLimitAndCoverEveryWorkloadOnce(t *testing.T) {
	selected := make([]string, 0, 800)
	for index := range 800 {
		selected = append(selected, fmt.Sprintf("TestUserInterfaceWorkloadNumber%04dKeepsItsLongDescriptiveName", index))
	}
	parts := nativeSelectionParts(selected, nativeSelectionLimit)
	require.Greater(t, len(parts), 1, "an oversized cohort must split")
	var rejoined []string
	for _, part := range parts {
		require.NotEmpty(t, part)
		require.LessOrEqual(t, len(nativeSelection(part)), nativeSelectionLimit)
		rejoined = append(rejoined, part...)
	}
	require.Equal(t, selected, rejoined, "parts keep order and cover every workload exactly once")

	require.Equal(t, [][]string{{"TestA", "TestB"}}, nativeSelectionParts([]string{"TestA", "TestB"}, 64))
	long := "Test" + strings.Repeat("X", 100)
	require.Equal(t, [][]string{{"TestA"}, {long}, {"TestB"}},
		nativeSelectionParts([]string{"TestA", long, "TestB"}, 64), "an over-long name runs alone")
}

func TestNativeGCOnlyDefaultsForConstrainedProfile(t *testing.T) {
	for _, profile := range []string{profileHost, profileConstrained, profileThroughput} {
		environment := []string{"GOGC=125", "GOMEMLIMIT=off"}
		require.Equal(t, environment, nativeGCEnvironment(profile, environment))
		if profile == profileConstrained {
			require.Equal(t, []string{"GOGC=50"}, nativeGCEnvironment(profile, nil))
		} else {
			require.Empty(t, nativeGCEnvironment(profile, nil))
		}
	}
}

func TestNativeDiscoveryRejectsEmptyDuplicateFailedAndUnexpectedStreams(t *testing.T) {
	for _, fixture := range []string{
		"", nativeDiscoveryFixture(t), nativeDiscoveryFixture(t, "TestOne", "TestOne"),
		strings.ReplaceAll(nativeDiscoveryFixture(t, "TestOne"), `"pass"`, `"skip"`),
		strings.ReplaceAll(nativeDiscoveryFixture(t, "TestOne"), `"app"`, `"other"`),
		`{"Action":"run","Package":"app","Test":"TestOne"}`,
	} {
		_, err := nativeWorkloads(strings.NewReader(fixture), "app")
		require.Error(t, err)
	}
}

func TestNativeCohortMergePreservesFailuresAndRejectsOmissionsAndDuplicates(t *testing.T) {
	result := &packageResult{tests: make(map[string]*testResult)}
	local := &packageResult{tests: map[string]*testResult{
		"TestOne": {terminal: actionPass}, "TestOne/child": {terminal: actionSkip},
	}}
	require.ErrorContains(t, mergeNativeCohort("app", []string{"TestOne", "TestMissing"}, result, local), "omitted")
	require.Equal(t, actionSkip, result.tests["TestOne/child"].terminal)
	require.ErrorContains(t, validateResults(map[string]*packageResult{"app": local}, true), "skip")
	require.ErrorContains(t, mergeNativeCohort("app", []string{"TestOne"}, result, local), "repeated")
	other := &packageResult{tests: map[string]*testResult{"TestOther": {terminal: actionPass}}}
	require.ErrorContains(t, mergeNativeCohort("app", []string{"TestOne"}, result, other), "unexpected")
}
