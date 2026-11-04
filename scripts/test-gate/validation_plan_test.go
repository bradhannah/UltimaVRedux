package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidationPlanRunsGoAndPythonChecksOnce(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	command := exec.Command("make", "--no-print-directory", "-n", "validate", "GO=go")
	command.Dir = root
	command.Env = cleanMakeEnvironment()
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	plan := strings.Join(strings.Fields(string(output)), " ")
	for _, step := range []string{
		"go run ./scripts/test-gate -profile=host", "go vet ./...", "go build ./...",
		"go run ./scripts/check-lint-config", "go run ./scripts/check-documentation",
		"./scripts/pre-commit-checks.sh --policy-only", "-B scripts/check-gamefacts.py",
		"-I -B -m unittest discover -s scripts",
		"scripts/python_quality.py format-check", "scripts/python_quality.py lint",
		"scripts/python_quality.py ready",
	} {
		require.Equal(t, 1, strings.Count(plan, step), "validation must schedule %q exactly once", step)
	}
	require.NotContains(t, plan, "go test -short")
	require.NotContains(t, plan, "opencode")
	require.NotContains(t, plan, "go generate")
	require.NotContains(t, plan, "npm ")
	require.Contains(t, plan, ".cache/python-quality/")
	require.NotContains(t, plan, "scripts/python_quality.py provision")
	require.NotContains(t, plan, "scripts/python_quality.py staged-check")
	require.Contains(t, plan, "-suite=logic")
	require.NotContains(t, plan, "-suite=render")
	require.NotContains(t, plan, "-suite=all")

	command = exec.Command("make", "--no-print-directory", "-n", "pre-commit")
	command.Dir = root
	command.Env = cleanMakeEnvironment()
	output, err = command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Equal(t, "./scripts/pre-commit-checks.sh", strings.TrimSpace(string(output)),
		"standalone pre-commit must retain its complete check path")
}

func TestBuildUsesShippedAssets(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	command := exec.CommandContext(t.Context(), "make", "--no-print-directory", "-n", "build", "GO=go")
	command.Dir = root
	command.Env = cleanMakeEnvironment()
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	plan := string(output)
	require.Contains(t, plan, "go build -o bin/ultimav ./cmd/ultimav")
	for _, generation := range []string{"go generate", "npm", "web-build", "python", "opencode"} {
		require.NotContains(t, plan, generation)
	}
}

func TestQualityGateSchedulesRequiredScopesWithoutAggregateRepeat(t *testing.T) {
	contents, err := os.ReadFile("../quality-gate.sh")
	require.NoError(t, err)
	// Windows checkouts may convert line endings to CRLF.
	plan := strings.ReplaceAll(string(contents), "\r\n", "\n")
	for _, step := range []string{"make validate\n", "make web-build\n", "make web-test-browser\n"} {
		require.Equal(t, 1, strings.Count(plan, step), "quality gate must schedule %q once", step)
	}
	// Each platform branch executes one native pass and one debug race pass.
	require.Equal(t, 1, strings.Count(plan, "xvfb-run -a make test-render\n"))
	require.Equal(t, 1, strings.Count(plan, "    run_stage native make test-render\n"))
	require.Equal(t, 2, strings.Count(plan, "-parallel=1 -race ./internal/debugging/..."))
	require.Less(t, strings.Index(plan, "make validate"), strings.Index(plan, "make test-render"))
	require.Less(t, strings.Index(plan, "make test-render"), strings.Index(plan, "make web-build"))
	for _, duplicate := range []string{"make test-all", "make test-provisioned", "go test ./...", "make check"} {
		require.NotContains(t, plan, duplicate)
	}
}

func TestQualityGateReportsStagesAndStopsOnFailureOrCancellation(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	directory := t.TempDir()
	tool := `#!/bin/sh
printf 'CALLED %s %s\n' "${0##*/}" "$*"
if [ "$1" = "$CANCEL_TARGET" ]; then kill -TERM "$PPID"; exit 0; fi
if [ "$1" = "$FAIL_TARGET" ]; then exit 37; fi
`
	for name, body := range map[string]string{
		"make": tool, "go": tool,
		"uname":    "#!/bin/sh\nprintf '%s\\n' \"$FAKE_OS\"\n",
		"xvfb-run": "#!/bin/sh\nshift\nexec \"$@\"\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(body), 0o700))
	}
	stages := []string{"validate", "native", "frontend", "browser", "race"}
	targets := []string{"validate", "test-render", "web-build", "web-test-browser", "test"}
	for _, test := range []struct {
		name, goos, fail, cancel string
		last, status             int
	}{
		{name: "native success", goos: "Darwin", last: 4},
		{name: "Xvfb success", goos: "Linux", last: 4},
		{name: "validate failure", goos: "Darwin", fail: "validate", last: 0, status: 37},
		{name: "native failure", goos: "Linux", fail: "test-render", last: 1, status: 37},
		{name: "frontend failure", goos: "Darwin", fail: "web-build", last: 2, status: 37},
		{name: "browser failure", goos: "Darwin", fail: "web-test-browser", last: 3, status: 37},
		{name: "race failure", goos: "Linux", fail: "test", last: 4, status: 37},
		{name: "cancel", goos: "Darwin", cancel: "test-render", last: 1, status: 143},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.CommandContext(t.Context(), "bash", "scripts/quality-gate.sh")
			command.Dir = root
			command.Env = append(cleanMakeEnvironment(), "PATH="+directory+string(os.PathListSeparator)+os.Getenv("PATH"),
				"FAKE_OS="+test.goos, "FAIL_TARGET="+test.fail, "CANCEL_TARGET="+test.cancel)
			output, err := command.CombinedOutput()
			if test.status == 0 {
				require.NoError(t, err, "%s", output)
			} else {
				var exitError *exec.ExitError
				require.True(t, errors.As(err, &exitError), "%v: %s", err, output)
				require.Equal(t, test.status, exitError.ExitCode(), "%s", output)
			}
			for index, stage := range stages {
				if index > test.last {
					require.NotContains(t, string(output), "STAGE START "+stage)
					continue
				}
				status := 0
				if index == test.last {
					status = test.status
				}
				require.Equal(t, 1, strings.Count(string(output), "STAGE START "+stage+"\n"))
				require.Regexp(t, "STAGE END "+stage+" status="+strconv.Itoa(status)+` DURATION=\d+s`, string(output))
				toolName := "make"
				if stage == "race" {
					toolName = "go"
				}
				require.Equal(t, 1, strings.Count(string(output), "CALLED "+toolName+" "+targets[index]))
			}
			require.Equal(t, test.last+1, strings.Count(string(output), "CALLED "), "no repeated or extra workloads")
		})
	}
}

func TestFormatCheckPreservesToolFailure(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	probe := filepath.Join(t.TempDir(), "python-probe.mk")
	// Python tool failures have real-tool unittest fixtures. This regression checks
	// the Go formatter exit without repeating Python validation inside logic tests.
	require.NoError(t, os.WriteFile(probe, []byte("python-env-check python-format-check:\n\t@:\n"), 0o600))
	command := exec.CommandContext(t.Context(), "make", "--no-print-directory", "-f", "Makefile", "-f", probe,
		"format-check", "GOIMPORTS=false")
	command.Dir = root
	command.Env = cleanMakeEnvironment()
	output, err := command.CombinedOutput()
	require.Error(t, err, "a failed formatting tool must fail validation: %s", output)
}

func cleanMakeEnvironment() []string {
	var environment []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "MAKEFLAGS", "MFLAGS", "MAKEOVERRIDES", "GOFLAGS", "GOENV", "TEST_PROFILE", "TEST_GO_BUILD_FLAGS",
			"GOMAXPROCS", "GOMEMLIMIT", "GOGC", "ULTIMA5_REQUIRE_DATA":
			continue
		}
		environment = append(environment, entry)
	}
	return environment
}

func TestMakeForwardsProfilesAndStrictRaceFlags(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	for _, profile := range []string{profileHost, profileConstrained, profileThroughput} {
		for _, target := range []string{"test", "test-render", "test-all", "test-race", "test-provisioned", "test-race-provisioned"} {
			t.Run(profile+"/"+target, func(t *testing.T) {
				command := exec.CommandContext(t.Context(), "make", "--no-print-directory", "-n", target, "GO=go", "TEST_PROFILE="+profile)
				command.Dir = root
				command.Env = append(cleanMakeEnvironment(), "MAKEFLAGS=-j8", "ULTIMA5_REQUIRE_DATA=1")
				output, err := command.CombinedOutput()
				require.NoError(t, err, "%s", output)
				plan := strings.Join(strings.Fields(string(output)), " ")
				bootstrap := "go run -p=1 ./scripts/test-gate"
				if profile == profileHost {
					bootstrap = "go run ./scripts/test-gate"
				}
				require.Equal(t, 1, strings.Count(plan, bootstrap))
				require.Contains(t, plan, bootstrap+" -profile="+profile+" -strict")
				suite := "all"
				if target == "test" {
					suite = "logic"
				} else if target == "test-render" {
					suite = "render"
				}
				require.Contains(t, plan, "-suite="+suite)
				if strings.Contains(target, "race") {
					require.Contains(t, plan, " -race")
				} else {
					require.NotContains(t, plan, " -race")
				}
			})
		}
	}
}

func TestMakeProfileEnvironmentAndPreflight(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	probe := filepath.Join(t.TempDir(), "probe.mk")
	// Replace only the test recipe with a tiny shell probe. No Go command runs.
	require.NoError(t, os.WriteFile(probe, []byte(`test:
	@printf 'PROFILE=%s CPU=%s MEMORY=%s GC=%s\nGOFLAGS=%s\nBUILD_FLAGS=%s\n' "$$TEST_PROFILE" "$$GOMAXPROCS" "$$GOMEMLIMIT" "$$GOGC" "$$GOFLAGS" "$(TEST_GO_BUILD_FLAGS)"
`), 0o600))
	const callerFlags = "-tags='one two' -mod=readonly -p=9 -parallel=9"
	for _, test := range []struct {
		name        string
		arguments   []string
		environment []string
		want        string
		fail        bool
		bounded     bool
	}{
		{name: "default", want: "PROFILE=host CPU= MEMORY= GC="},
		{name: "host explicit environment", environment: []string{"GOMAXPROCS=7", "GOMEMLIMIT=768MiB", "GOGC=150"}, want: "PROFILE=host CPU=7 MEMORY=768MiB GC=150"},
		{name: "host explicit command-line", arguments: []string{"GOMAXPROCS=6", "GOMEMLIMIT=off", "GOGC=125"}, want: "PROFILE=host CPU=6 MEMORY=off GC=125"},
		{name: "constrained", arguments: []string{"TEST_PROFILE=constrained"}, want: "PROFILE=constrained CPU=2 MEMORY=256MiB GC=50", bounded: true},
		{name: "throughput", arguments: []string{"TEST_PROFILE=throughput"}, want: "PROFILE=throughput CPU=4 MEMORY=2GiB GC=50", bounded: true},
		{name: "exported selection", environment: []string{"TEST_PROFILE=throughput"}, want: "PROFILE=throughput CPU=4 MEMORY=2GiB GC=50", bounded: true},
		{name: "command-line build flags", arguments: []string{"GOFLAGS=" + callerFlags}, want: "PROFILE=host CPU= MEMORY= GC="},
		{name: "matching explicit", arguments: []string{"TEST_PROFILE=constrained"}, environment: []string{"GOMAXPROCS=2", "GOMEMLIMIT=256MiB", "GOGC=125"}, want: "PROFILE=constrained CPU=2 MEMORY=256MiB GC=125", bounded: true},
		{name: "lower CPU", arguments: []string{"TEST_PROFILE=constrained"}, environment: []string{"GOMAXPROCS=1"}, want: "requires GOMAXPROCS=2", fail: true},
		{name: "higher memory", arguments: []string{"TEST_PROFILE=constrained"}, environment: []string{"GOMEMLIMIT=512MiB"}, want: "requires GOMEMLIMIT=256MiB", fail: true},
		{name: "higher CPU", arguments: []string{"TEST_PROFILE=constrained", "GOMAXPROCS=8"}, want: "requires GOMAXPROCS=2", fail: true},
		{name: "empty memory", arguments: []string{"TEST_PROFILE=constrained"}, environment: []string{"GOMEMLIMIT="}, want: "requires GOMEMLIMIT=256MiB", fail: true},
		{name: "Windows lowercase memory", arguments: []string{"TEST_PROFILE=constrained"}, environment: []string{"OS=Windows_NT", "gomemlimit=512MiB"}, want: "Windows resource variables must use uppercase names", fail: true},
		{name: "Windows mixed CPU", arguments: []string{"TEST_PROFILE=constrained"}, environment: []string{"OS=Windows_NT", "GoMaxProcs=1"}, want: "Windows resource variables must use uppercase names", fail: true},
		{name: "wrong profile", arguments: []string{"TEST_PROFILE=typo"}, want: "Unknown TEST_PROFILE", fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			arguments := append([]string{"--no-print-directory", "-f", "Makefile", "-f", probe, "test"}, test.arguments...)
			command := exec.CommandContext(t.Context(), "make", arguments...)
			command.Dir = root
			command.Env = append(cleanMakeEnvironment(), "MAKEFLAGS=-j8", "GOFLAGS="+callerFlags)
			command.Env = append(command.Env, test.environment...)
			output, err := command.CombinedOutput()
			if test.fail {
				require.Error(t, err)
				require.NotContains(t, string(output), " CPU=", "the workload must not start before preflight succeeds")
			} else {
				require.NoError(t, err, "%s", output)
				buildFlags := ""
				if test.bounded {
					buildFlags = "-p=1"
				}
				require.Contains(t, string(output), "GOFLAGS="+callerFlags+"\nBUILD_FLAGS="+buildFlags+"\n")
			}
			require.Contains(t, string(output), test.want)
		})
	}
}

func TestMakeProfileDoesNotChangeGameEnvironment(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	probe := filepath.Join(t.TempDir(), "probe.mk")
	require.NoError(t, os.WriteFile(probe, []byte(`run:
	@printf 'CPU=%s MEMORY=%s\nGOFLAGS=%s\nBUILD_FLAGS=%s\n' "$$GOMAXPROCS" "$$GOMEMLIMIT" "$$GOFLAGS" "$(TEST_GO_BUILD_FLAGS)"
`), 0o600))
	const callerFlags = "-tags='one two' -mod=readonly -p=9 -parallel=9"
	command := exec.CommandContext(t.Context(), "make", "--no-print-directory", "-f", "Makefile", "-f", probe,
		"run", "TEST_PROFILE=throughput", "GOFLAGS="+callerFlags)
	command.Dir = root
	command.Env = append(cleanMakeEnvironment(), "MAKEFLAGS=-j8", "GOMAXPROCS=1", "GOMEMLIMIT=512MiB")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Contains(t, string(output), "CPU=1 MEMORY=512MiB\nGOFLAGS="+callerFlags+"\nBUILD_FLAGS=\n")
}

func TestMakeDoesNotShadowPersistedGoFlags(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	directory := t.TempDir()
	goenv := filepath.Join(directory, "goenv")
	require.NoError(t, os.WriteFile(goenv, []byte("GOFLAGS=-tags=profilefixture\n"), 0o600))
	probe := filepath.Join(directory, "probe.mk")
	require.NoError(t, os.WriteFile(probe, []byte(`test:
	@printf 'GOENV=%s\nGOFLAGS_SET=%s\nBUILD_FLAGS=%s\n' "$$GOENV" "$${GOFLAGS+x}" "$(TEST_GO_BUILD_FLAGS)"
`), 0o600))
	command := exec.CommandContext(t.Context(), "make", "--no-print-directory", "-f", "Makefile", "-f", probe, "test")
	command.Dir = root
	command.Env = append(cleanMakeEnvironment(), "MAKEFLAGS=-j8", "GOENV="+goenv)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Contains(t, string(output), "GOENV="+goenv+"\nGOFLAGS_SET=\nBUILD_FLAGS=\n",
		"Make must not synthesize GOFLAGS and hide the selected Go environment file")
}

func TestMakeSerializesPrerequisitesDespiteInheritedJobs(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	probe := filepath.Join(t.TempDir(), "probe.mk")
	require.NoError(t, os.WriteFile(probe, []byte(`.PHONY: profile-order profile-first profile-second
profile-order: profile-first profile-second
profile-first:
	@printf 'first-start\n'; sleep 0.02; printf 'first-end\n'
profile-second:
	@printf 'second-start\n'; sleep 0.02; printf 'second-end\n'
`), 0o600))
	command := exec.CommandContext(t.Context(), "make", "--no-print-directory", "-f", "Makefile", "-f", probe, "profile-order")
	command.Dir = root
	command.Env = append(cleanMakeEnvironment(), "MAKEFLAGS=-j8")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Equal(t, "first-start\nfirst-end\nsecond-start\nsecond-end\n", string(output))
}
