package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRoot = modulePath + "/internal/game_state"

func TestForbiddenPackageBoundaries(t *testing.T) {
	for _, path := range []string{
		"github.com/hajimehoshi/ebiten/v2", "github.com/hajimehoshi/ebiten/v2/text/v2",
		"github.com/ebitengine/purego", modulePath + "/internal/display",
		"github.com/bradhannah/berad-sdk/modules/berad-text",
		"github.com/bradhannah/berad-sdk/modules/berad-graphics",
		"github.com/bradhannah/berad-sdk/modules/berad-input",
		modulePath + "/internal/text", modulePath + "/internal/ui/widgets",
		modulePath + "/internal/sprites", modulePath + "/internal/sprites/effects",
		modulePath + "/internal/audio", modulePath + "/pkg/input",
		modulePath + "/cmd/ultimav",
	} {
		assert.True(t, forbiddenPackage(path), path)
		assert.True(t, forbiddenPackage(path+" ["+testRoot+".test]"), path)
	}
	for _, path := range []string{
		modulePath + "/internal/sprites/indexes", modulePath + "/pkg/semantic_input",
		modulePath + "/internal/textual", "image/color", "testing",
		"github.com/bradhannah/berad-sdk/modules/berad-rng",
	} {
		assert.False(t, forbiddenPackage(path), path)
		assert.False(t, forbiddenPackage(path+" ["+testRoot+".test]"), path)
	}
}

func TestGraphRejectsDirectAndTransitiveDependencies(t *testing.T) {
	for _, intermediate := range []string{"", "helper"} {
		t.Run(intermediate, func(t *testing.T) {
			forbidden := modulePath + "/internal/display"
			root := packageMetadata{ImportPath: testRoot, Imports: []string{forbidden}}
			packages := []packageMetadata{root, {ImportPath: forbidden}}
			witness := testRoot + " -> " + forbidden
			if intermediate != "" {
				packages[0].Imports = []string{intermediate}
				packages = append(packages, packageMetadata{ImportPath: intermediate, Imports: []string{forbidden}})
				witness = testRoot + " -> " + intermediate + " -> " + forbidden
			}
			err := checkGraph(packages, []string{testRoot})
			require.ErrorIs(t, err, errDependencyGraph)
			assert.Contains(t, err.Error(), witness)
		})
	}
}

func TestGraphAcceptsPureProductionAndTestClosures(t *testing.T) {
	packages := testGraph(false)
	packages[1].Imports = []string{modulePath + "/internal/sprites/indexes", modulePath + "/pkg/semantic_input"}
	packages = append(packages,
		packageMetadata{ImportPath: modulePath + "/internal/sprites/indexes"},
		packageMetadata{ImportPath: modulePath + "/pkg/semantic_input"},
	)
	require.NoError(t, checkGraph(packages, []string{testRoot}))
}

func TestGraphAllowsCgoPseudoImportAndTerminatesCycles(t *testing.T) {
	packages := []packageMetadata{
		{ImportPath: testRoot, Imports: []string{"C", "helper"}},
		{ImportPath: "helper", Imports: []string{testRoot}},
	}
	require.NoError(t, checkGraph(packages, []string{testRoot}))
}

func TestGraphRejectsTestVariantWithWrongOwner(t *testing.T) {
	packages := testGraph(false)
	packages[1].ForTest = "other"
	err := checkGraph(packages, []string{testRoot})
	require.ErrorContains(t, err, "missing test variant")
}

func TestGraphRejectsInternalAndExternalTestOnlyDependencies(t *testing.T) {
	for _, external := range []bool{false, true} {
		packages := testGraph(external)
		forbidden := modulePath + "/internal/text"
		packages[1].Imports = []string{forbidden}
		packages = append(packages, packageMetadata{ImportPath: forbidden})
		err := checkGraph(packages, []string{testRoot})
		require.Error(t, err)
		assert.Contains(t, err.Error(), packages[1].ImportPath+" -> "+forbidden)
		assert.NotContains(t, err.Error(), testRoot+" -> "+forbidden, "production root remains pure")
	}
}

func TestGraphRejectsTestCodeInProductionClosureOnly(t *testing.T) {
	for _, testOnly := range []string{"testing", modulePath + "/test/helpers"} {
		packages := []packageMetadata{
			{ImportPath: testRoot, Imports: []string{"helper"}},
			{ImportPath: "helper", Imports: []string{testOnly}},
			{ImportPath: testOnly},
		}
		err := checkGraph(packages, []string{testRoot})
		require.ErrorIs(t, err, errDependencyGraph)
		assert.Contains(t, err.Error(), testRoot+" -> helper -> "+testOnly)
	}
	packages := testGraph(false)
	packages[1].Imports = []string{"testing"}
	packages = append(packages, packageMetadata{ImportPath: "testing"})
	require.NoError(t, checkGraph(packages, []string{testRoot}), "test variants may link test code")
}

func TestGraphFollowsImportMapWithoutCollapsingTestVariants(t *testing.T) {
	packages := testGraph(false)
	alias := "helper [" + testRoot + ".test]"
	forbidden := "github.com/hajimehoshi/ebiten/v2"
	packages[1].Imports = []string{"helper"}
	packages[1].ImportMap = map[string]string{"helper": alias}
	packages = append(packages,
		packageMetadata{ImportPath: "helper"},
		packageMetadata{ImportPath: alias, ForTest: testRoot, Imports: []string{forbidden}},
		packageMetadata{ImportPath: forbidden},
	)
	err := checkGraph(packages, []string{testRoot})
	require.Error(t, err)
	assert.Contains(t, err.Error(), packages[1].ImportPath+" -> "+alias+" -> "+forbidden)
}

func TestGraphWitnessIsShortestAndDeterministic(t *testing.T) {
	forbidden := modulePath + "/pkg/input"
	packages := []packageMetadata{
		{ImportPath: testRoot, Imports: []string{"z", "b", "a"}},
		{ImportPath: "a", Imports: []string{forbidden}},
		{ImportPath: "b", Imports: []string{forbidden}},
		{ImportPath: "z", Imports: []string{"b"}},
		{ImportPath: forbidden},
	}
	err := checkGraph(packages, []string{testRoot})
	require.Error(t, err)
	assert.Equal(t, errDependencyGraph.Error()+": "+testRoot+" -> a -> "+forbidden, err.Error())
	slices.Reverse(packages)
	for index := range packages {
		slices.Reverse(packages[index].Imports)
	}
	again := checkGraph(packages, []string{testRoot})
	require.Error(t, again)
	assert.Equal(t, err.Error(), again.Error())
}

func TestGraphFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name     string
		packages []packageMetadata
		roots    []string
		want     string
	}{
		{name: "missing root", roots: []string{testRoot}, want: "missing root"},
		{name: "no roots", packages: []packageMetadata{{ImportPath: testRoot}}, want: "no roots"},
		{name: "missing identity", packages: []packageMetadata{{}}, roots: []string{testRoot}, want: "no import path"},
		{name: "duplicate", packages: []packageMetadata{{ImportPath: testRoot}, {ImportPath: testRoot}}, roots: []string{testRoot}, want: "duplicate package"},
		{name: "incomplete", packages: []packageMetadata{{ImportPath: testRoot, Incomplete: true}}, roots: []string{testRoot}, want: "incomplete package"},
		{name: "load error", packages: []packageMetadata{{ImportPath: testRoot, Error: &packageError{Err: "syntax failure"}}}, roots: []string{testRoot}, want: "syntax failure"},
		{name: "dependency load error", packages: []packageMetadata{{ImportPath: testRoot, DepsErrors: []packageError{{Err: "broken dependency"}}}}, roots: []string{testRoot}, want: "broken dependency"},
		{name: "missing edge", packages: []packageMetadata{{ImportPath: testRoot, Imports: []string{"missing"}}}, roots: []string{testRoot}, want: testRoot + " -> missing"},
		{name: "missing mapped edge", packages: []packageMetadata{{ImportPath: testRoot, Imports: []string{"helper"}, ImportMap: map[string]string{"helper": "missing variant"}}, {ImportPath: "helper"}}, roots: []string{testRoot}, want: "missing variant"},
		{name: "missing internal test", packages: []packageMetadata{{ImportPath: testRoot, TestGoFiles: []string{"core_test.go"}}}, roots: []string{testRoot}, want: "missing test variant"},
		{name: "missing external test", packages: []packageMetadata{{ImportPath: testRoot, XTestGoFiles: []string{"core_test.go"}}}, roots: []string{testRoot}, want: "missing test variant"},
		{name: "missing test binary", packages: testGraph(false)[:2], roots: []string{testRoot}, want: "missing test variant"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := checkGraph(test.packages, test.roots)
			require.ErrorIs(t, err, errDependencyGraph)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestDecodePackagesRejectsEmptyMalformedAndTruncatedMetadata(t *testing.T) {
	for _, input := range []string{"", " ", "not JSON", `{"ImportPath":"a"`, `{"ImportPath":"a"} {`} {
		_, err := decodePackages(strings.NewReader(input))
		require.Error(t, err, input)
	}
	packages, err := decodePackages(strings.NewReader(`{"ImportPath":"a"} {"ImportPath":"b"}`))
	require.NoError(t, err)
	require.Len(t, packages, 2)
	assert.Equal(t, "b", packages[1].ImportPath)
}

func TestRunReportsGoListFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := run(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorContains(t, err, "loading core production/test metadata")
}

func testGraph(external bool) []packageMetadata {
	root := packageMetadata{ImportPath: testRoot}
	variant := testRoot + " [" + testRoot + ".test]"
	if external {
		root.XTestGoFiles = []string{"core_test.go"}
		variant = testRoot + "_test [" + testRoot + ".test]"
	} else {
		root.TestGoFiles = []string{"core_test.go"}
	}

	return []packageMetadata{
		root,
		{ImportPath: variant, ForTest: testRoot},
		{ImportPath: testRoot + ".test", Imports: []string{variant}},
	}
}
