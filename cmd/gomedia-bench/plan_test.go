package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The package directory is cmd/gomedia-bench, two levels below the repository root.
const benchDirFromPkg = "../../bench"

// benchSource concatenates the e2e benchmark sources.
func benchSource(t *testing.T) string {
	t.Helper()
	src, err := filepath.Glob(filepath.Join(benchDirFromPkg, "*_test.go"))
	if err != nil || len(src) == 0 {
		t.Fatalf("no bench sources found: %v", err)
	}
	var all strings.Builder
	for _, f := range src {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	return all.String()
}

var benchFuncRE = regexp.MustCompile(`func (Benchmark\w+)\(b \*testing\.B\)`)

func TestOperationListsMatchSource(t *testing.T) {
	src := benchSource(t)
	for _, name := range append(append([]string{}, microBenchmarks...), fixtureBenchmarks...) {
		if !strings.Contains(src, "func Benchmark"+name+"(") {
			t.Errorf("%s is listed in run.go but not defined in bench/*_test.go", name)
		}
	}
}

// TestBenchmarkCoverage fails when a new benchmark is added to bench/ but not
// to either list, which would otherwise be silently skipped by the harness.
func TestBenchmarkCoverage(t *testing.T) {
	listed := map[string]bool{}
	for _, n := range microBenchmarks {
		listed["Benchmark"+n] = true
	}
	for _, n := range fixtureBenchmarks {
		listed["Benchmark"+n] = true
	}
	var missing []string
	for _, m := range benchFuncRE.FindAllStringSubmatch(benchSource(t), -1) {
		if !listed[m[1]] {
			missing = append(missing, m[1])
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("benchmarks not in microBenchmarks or fixtureBenchmarks (run.go): %s", strings.Join(missing, ", "))
	}
}

func TestLoadFixtureNamesFromManifest(t *testing.T) {
	names, err := loadFixtureNames(filepath.Join(benchDirFromPkg, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 30 {
		t.Fatalf("expected the full fixture corpus, got %d names", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatal("fixture names must be sorted")
		}
	}
}

func TestSelectFixtures(t *testing.T) {
	all := []string{"a", "b", "c"}

	got, err := selectFixtures("", all)
	if err != nil || len(got) != 3 {
		t.Fatalf("empty spec should select all: %v %v", got, err)
	}
	got, err = selectFixtures("c, a,a", all)
	if err != nil || strings.Join(got, ",") != "c,a" {
		t.Fatalf("spec with spaces and duplicates: %v %v", got, err)
	}
	if _, err := selectFixtures("a,zzz", all); err == nil || !strings.Contains(err.Error(), "zzz") {
		t.Fatalf("unknown fixture should be reported by name, got %v", err)
	}
}

func TestBuildPlanOneJobPerOperationAndFixture(t *testing.T) {
	fixtures := []string{"f1", "f2"}

	// All operations: one job per (operation, fixture), plus the micro job.
	plan := buildPlan([]string{"e2e"}, fixtures, false, "")
	want := len(fixtureBenchmarks)*len(fixtures) + 1
	if len(plan) != want {
		t.Fatalf("expected %d jobs, got %d", want, len(plan))
	}
	if plan[0].job != "Probe__f1" || plan[0].filter != "^BenchmarkProbe$/^f1$" {
		t.Fatalf("first job = %+v", plan[0])
	}
	last := plan[len(plan)-1]
	if last.job != "micro" || !strings.Contains(last.filter, "SIMDActive") {
		t.Fatalf("micro job = %+v", last)
	}

	// Chosen fixtures only: no micro job.
	plan = buildPlan([]string{"e2e"}, fixtures[:1], true, "")
	if len(plan) != len(fixtureBenchmarks) {
		t.Fatalf("fixture mode should give one job per operation for the chosen fixture, got %d", len(plan))
	}

	// One operation across fixtures.
	plan = buildPlan([]string{"e2e"}, fixtures, false, "ThumbnailLarge")
	if len(plan) != len(fixtures) || plan[0].job != "ThumbnailLarge__f1" {
		t.Fatalf("op plan = %+v", plan)
	}

	// A micro operation runs only as the micro job, never per fixture.
	plan = buildPlan([]string{"e2e"}, fixtures, false, "SIMDActive")
	if len(plan) != 1 || plan[0].job != "micro" || plan[0].filter != "^BenchmarkSIMDActive$" {
		t.Fatalf("micro op plan = %+v", plan)
	}
}

func TestBuildPlanInternalSkippedForFixtureMode(t *testing.T) {
	plan := buildPlan([]string{"internal"}, []string{"f1"}, true, "")
	if len(plan) != 0 {
		t.Fatalf("internal jobs must not run when fixtures are chosen, got %d", len(plan))
	}
	plan = buildPlan([]string{"internal"}, []string{"f1"}, false, "")
	if len(plan) != len(suites["internal"]) {
		t.Fatalf("expected one internal job per package, got %d", len(plan))
	}
}

func TestFixtureOpsFor(t *testing.T) {
	if got := fixtureOpsFor("Probe"); len(got) != 1 || got[0] != "Probe" {
		t.Fatalf("per-fixture op: %v", got)
	}
	if got := fixtureOpsFor("SIMDActive"); got != nil {
		t.Fatalf("micro op should have no per-fixture jobs: %v", got)
	}
	if got := fixtureOpsFor(""); len(got) != len(fixtureBenchmarks) {
		t.Fatalf("empty op should list all per-fixture ops")
	}
}

func TestKeepUnfinishedReturnsOnlyMissingOrFailed(t *testing.T) {
	dir := t.TempDir()
	const settings = "count=1;benchtime=1x;cpu=1;pgo=true"
	plan := []benchJob{
		{suite: "e2e", job: "ok__f"},
		{suite: "e2e", job: "bad__f"},
		{suite: "e2e", job: "absent__f"},
	}
	writeShardFile(t, dir, "e2e", shardFileName("ok__f"), encodeShard("ok__f", "ok", settings, []byte("x\n")))
	writeShardFile(t, dir, "e2e", shardFileName("bad__f"), encodeShard("bad__f", "failed", settings, nil))

	got := keepUnfinished(plan, dir)
	if len(got) != 2 || got[0].job != "bad__f" || got[1].job != "absent__f" {
		t.Fatalf("retry plan = %+v", got)
	}
}

func TestCheckShardSettingsRejectsMismatch(t *testing.T) {
	dir := t.TempDir()
	plan := []benchJob{{suite: "e2e", job: "ok__f"}, {suite: "e2e", job: "bad__f"}}
	writeShardFile(t, dir, "e2e", shardFileName("ok__f"), encodeShard("ok__f", "ok", "count=1;benchtime=1x;cpu=1;pgo=true", []byte("x\n")))
	writeShardFile(t, dir, "e2e", shardFileName("bad__f"), encodeShard("bad__f", "failed", "count=9;benchtime=1x;cpu=1;pgo=true", nil))
	if err := checkShardSettings(plan, dir, "count=1;benchtime=1x;cpu=1;pgo=true"); err != nil {
		t.Fatalf("matching settings rejected: %v", err)
	}
	if err := checkShardSettings(plan, dir, "count=5;benchtime=1x;cpu=1;pgo=true"); err == nil {
		t.Fatal("mismatched ok shard accepted")
	}
}
