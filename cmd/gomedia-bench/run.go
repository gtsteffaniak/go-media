package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// suites maps a suite name to the packages it runs. "e2e" is the public-API
// benchmark package in bench/; "internal" holds the per-package micro
// benchmarks that already live next to the code they measure.
var suites = map[string][]string{
	"e2e": {"./bench/"},
	"internal": {
		"./internal/thumb/",
		"./internal/isobmff/",
		"./internal/matroska/",
		"./internal/h264/",
		"./internal/h264/cleanroom/decode/",
		"./internal/kernels/h264/",
		"./internal/mpeg4/",
	},
}

// microBenchmarks are the e2e benchmarks that take no fixture. They run as one
// job; every other e2e benchmark is run once per fixture. TestMicroBenchmarksExist
// fails if one of these names no longer exists in bench/.
var microBenchmarks = []string{
	"AllThumbnailFilters",
	"ThumbnailFilterLabel",
	"SIMDActive",
	"OptionsForPreset",
	"ApplyPreset",
	"UnsupportedError",
	"ExternalSubtitleTracks",
}

// fixtureBenchmarks are the e2e operations that run once per fixture. Each
// (operation, fixture) pair is its own job, so a failure stays inside one pair.
// TestBenchmarkCoverage fails if a benchmark in bench/ is in neither list.
var fixtureBenchmarks = []string{
	"Probe",
	"Duration",
	"ReadTags",
	"HasArtwork",
	"EmbeddedArtwork",
	"InspectAudio",
	"LyricsFromReader",
	"HasLyrics",
	"LyricsSidecar",
	"ListSubtitleTracks",
	"ExtractEmbeddedSubtitle",
	"SubtitleToWebVTT",
	"ThumbnailSmall",
	"ThumbnailLarge",
	"ThumbnailWebP",
	"ThumbnailRasterPNG",
	"H264KeyframeYUV420",
	"H264KeyframeTimeSec",
	"MP4KeyframeSamples",
}

// manifestPath is read relative to the repository root.
const manifestPath = "bench/manifest.json"

var labelRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// benchJob is one go test invocation that writes one shard.
type benchJob struct {
	suite  string // "e2e" or "internal"; also the shard directory name
	job    string // shard name: a fixture, "micro", or an internal package
	pkg    string // package to test
	filter string // go test -bench pattern
}

// cmdRun runs benchmark jobs and writes their output as shards under
// <out>/<label>/shards/, then rebuilds <out>/<label>/<suite>.txt from every
// ok shard in the label. Jobs are one fixture each (all operations on it, or
// the single -op), so they can run in parallel. Each shard is written once,
// when its job finishes, and an existing shard for the same job is replaced
// only when that job completes again.
func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	label := fs.String("label", "", "result label, e.g. baseline or opt-h264-idct (required)")
	suite := fs.String("suite", "all", "e2e, internal or all")
	count := fs.Int("count", 5, "go test -count: repetitions per benchmark")
	benchtime := fs.String("benchtime", "1s", "go test -benchtime per repetition (e.g. 1s or 20x)")
	op := fs.String("op", "", "run only this operation, e.g. ThumbnailLarge or Probe (exact name, no Benchmark prefix)")
	fixture := fs.String("fixture", "", "run only these fixtures, comma-separated, e.g. mp4_h264_1920x1080_60s_large")
	pgo := fs.Bool("pgo", true, "build with -pgo=default.pgo (production profile)")
	cpu := fs.Int("cpu", 1, "go test -cpu (GOMAXPROCS); 1 matches the compare harness timing")
	outDir := fs.String("out", "bench/results", "results directory")
	jobs := fs.Int("jobs", defaultJobs, "jobs (go test processes) running at once; 1 for timing comparisons")
	retryFailed := fs.Bool("retry-failed", false, "run only jobs whose shard is missing or failed; finished shards are kept")
	dryRun := fs.Bool("dry-run", false, "print the job list (one per operation and fixture) and exit without running")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !labelRE.MatchString(*label) {
		return fmt.Errorf("-label is required and must match %s", labelRE)
	}
	if *count < 1 {
		return fmt.Errorf("-count must be >= 1")
	}
	if err := validateJobs(*jobs); err != nil {
		return err
	}
	if _, err := buildBenchPattern(*op, ""); err != nil {
		return err
	}
	if *op != "" && len(fixtureOpsFor(*op)) == 0 && !isMicro(*op) {
		return fmt.Errorf("unknown operation %q (see fixtureBenchmarks and microBenchmarks in run.go)", *op)
	}
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("run from the go-media repository root (go.mod not found)")
	}
	var names []string
	switch *suite {
	case "all":
		names = []string{"e2e", "internal"}
	case "e2e", "internal":
		names = []string{*suite}
	default:
		return fmt.Errorf("unknown suite %q", *suite)
	}

	fixtures, err := loadFixtureNames(manifestPath)
	if err != nil {
		return err
	}
	selected, err := selectFixtures(*fixture, fixtures)
	if err != nil {
		return err
	}
	plan := buildPlan(names, selected, *fixture != "", *op)
	if len(plan) == 0 {
		return fmt.Errorf("nothing to run for suite=%s op=%q fixture=%q", *suite, *op, *fixture)
	}

	dir := filepath.Join(*outDir, *label)
	if *dryRun {
		for _, j := range plan {
			fmt.Printf("%s/%s\t%s\n", j.suite, j.job, j.filter)
		}
		fmt.Printf("%d job(s)\n", len(plan))
		return nil
	}
	if *retryFailed {
		before := len(plan)
		plan = keepUnfinished(plan, dir)
		fmt.Printf("retry: %d of %d job(s) need running; finished shards are kept\n", len(plan), before)
		if len(plan) == 0 {
			return nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	settings := settingsKey(*count, *benchtime, *cpu, *pgo)
	selection := fmt.Sprintf("suite=%s op=%s fixture=%s", *suite, orDot(*op), orDot(*fixture))
	meta := hostMeta(*count, *benchtime, selection, *pgo, *cpu, *jobs)

	outs, errs := runJobs(len(plan), *jobs, func(i int) ([]byte, error) {
		return nil, runBenchJob(plan[i], dir, settings, *count, *benchtime, *cpu, *pgo, *outDir)
	})
	_ = outs

	var failed []string
	for i, j := range plan {
		if errs[i] != nil {
			failed = append(failed, fmt.Sprintf("%s/%s: %v", j.suite, j.job, errs[i]))
		}
	}

	// Rebuild the report files once, from every ok shard in this label.
	for _, name := range names {
		merged, skipped, err := mergeSuite(dir, name)
		if err != nil {
			return err
		}
		if len(skipped) > 0 {
			failed = append(failed, fmt.Sprintf("%s: %d shard(s) failed and were left out: %s", name, len(skipped), strings.Join(skipped, ", ")))
		}
		fmt.Printf("%s: merged %d shard(s) into %s\n", name, merged, filepath.Join(dir, name+".txt"))
	}

	meta += fmt.Sprintf("load_end: %s\n", loadAverage())
	if err := writeFileAtomic(filepath.Join(dir, "meta.txt"), []byte(meta)); err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d problem(s); see shards under %s:\n  %s", len(failed), filepath.Join(dir, "shards"), strings.Join(failed, "\n  "))
	}
	fmt.Printf("results written to %s\n", dir)
	return nil
}

// runBenchJob runs one job and writes its shard once. The shard's own lock
// makes a second run on the same fixture fail fast instead of overwriting it.
// Failed jobs still write a shard marked failed, so the problem is visible.
func runBenchJob(j benchJob, dir, settings string, count int, benchtime string, cpu int, pgo bool, outDir string) error {
	shardPath := filepath.Join(dir, "shards", j.suite, shardFileName(j.job))
	if err := os.MkdirAll(filepath.Dir(shardPath), 0o755); err != nil {
		return err
	}
	release, err := acquireFileLock(shardPath + ".lock")
	if err != nil {
		return fmt.Errorf("shard %s: %w", j.job, err)
	}
	defer release()

	argv := []string{"test", "-run", "^$", "-bench", j.filter, "-benchmem",
		"-count", fmt.Sprint(count), "-benchtime", benchtime,
		"-cpu", fmt.Sprint(cpu), "-timeout", "120m"}
	if pgo {
		argv = append(argv, "-pgo=default.pgo")
	}
	argv = append(argv, j.pkg)
	fmt.Printf("== %s/%s: go %s\n", j.suite, j.job, strings.Join(argv, " "))

	var out bytes.Buffer
	cmd := exec.Command("go", argv...)
	cmd.Env = append(internalFixtureEnv(j.suite, dirAbs(outDir)), "CGO_ENABLED=0")
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	start := time.Now()
	runErr := cmd.Run()
	fmt.Printf("   %s/%s finished in %s\n", j.suite, j.job, time.Since(start).Round(time.Second))

	status := "ok"
	if runErr != nil {
		status = "failed"
	}
	if err := writeFileAtomic(shardPath, encodeShard(j.job, status, settings, out.Bytes())); err != nil {
		return err
	}
	return runErr
}

// buildPlan lists every job for the selected suites. e2e has one job per
// selected fixture, plus one micro job when no fixture was chosen. internal
// has one job per package and is skipped when fixtures were chosen, since
// those benchmarks take no fixture.
func buildPlan(names, fixtures []string, fixtureMode bool, op string) []benchJob {
	var plan []benchJob
	for _, name := range names {
		switch name {
		case "e2e":
			for _, f := range fixtures {
				for _, o := range fixtureOpsFor(op) {
					pattern, err := buildBenchPattern(o, f)
					if err != nil {
						continue // names were validated by selectFixtures and -op
					}
					plan = append(plan, benchJob{suite: "e2e", job: o + "__" + f, pkg: "./bench/", filter: pattern})
				}
			}
			if !fixtureMode {
				if op == "" {
					plan = append(plan, benchJob{suite: "e2e", job: "micro", pkg: "./bench/", filter: microPattern()})
				} else if isMicro(op) {
					plan = append(plan, benchJob{suite: "e2e", job: "micro", pkg: "./bench/", filter: "^Benchmark" + regexp.QuoteMeta(op) + "$"})
				}
			}
		case "internal":
			if fixtureMode {
				continue
			}
			filter := "."
			if op != "" {
				filter = "^Benchmark" + regexp.QuoteMeta(op) + "$"
			}
			for _, pkg := range suites["internal"] {
				plan = append(plan, benchJob{suite: "internal", job: strings.TrimPrefix(pkg, "./internal/"), pkg: pkg, filter: filter})
			}
		}
	}
	return plan
}

// loadFixtureNames returns the fixture names in the manifest, sorted.
func loadFixtureNames(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m struct {
		Fixtures []struct {
			Name string `json:"name"`
		} `json:"fixtures"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	names := make([]string, 0, len(m.Fixtures))
	for _, f := range m.Fixtures {
		if f.Name != "" {
			names = append(names, f.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// selectFixtures returns every fixture when spec is empty, otherwise the
// fixtures named in the comma-separated spec. Unknown names are an error.
func selectFixtures(spec string, all []string) ([]string, error) {
	if spec == "" {
		return all, nil
	}
	known := map[string]bool{}
	for _, f := range all {
		known[f] = true
	}
	var out []string
	var unknown []string
	seen := map[string]bool{}
	for _, raw := range strings.Split(spec, ",") {
		f := strings.TrimSpace(raw)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		if !known[f] {
			unknown = append(unknown, f)
			continue
		}
		out = append(out, f)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown fixture(s): %s (see %s)", strings.Join(unknown, ", "), manifestPath)
	}
	return out, nil
}

// fixtureOpsFor returns the per-fixture operations to run for -op: every one
// when op is empty, just op when it is a per-fixture operation, and none
// otherwise (a micro operation or an unknown name).
func fixtureOpsFor(op string) []string {
	if op == "" {
		return fixtureBenchmarks
	}
	for _, f := range fixtureBenchmarks {
		if f == op {
			return []string{op}
		}
	}
	return nil
}

// keepUnfinished drops every job whose shard already finished with status ok,
// so a retry reruns only the missing or failed ones.
func keepUnfinished(plan []benchJob, dir string) []benchJob {
	var keep []benchJob
	for _, j := range plan {
		data, err := os.ReadFile(filepath.Join(dir, "shards", j.suite, shardFileName(j.job)))
		if err == nil {
			if s, derr := decodeShard(data); derr == nil && s.status == "ok" {
				continue
			}
		}
		keep = append(keep, j)
	}
	return keep
}

func isMicro(op string) bool {
	for _, m := range microBenchmarks {
		if m == op {
			return true
		}
	}
	return false
}

// microPattern selects exactly the micro benchmarks, anchored so that no
// other benchmark's name can match.
func microPattern() string {
	quoted := make([]string, len(microBenchmarks))
	for i, m := range microBenchmarks {
		quoted[i] = regexp.QuoteMeta(m)
	}
	return "^Benchmark(" + strings.Join(quoted, "|") + ")$"
}

func orDot(s string) string {
	if s == "" {
		return "."
	}
	return s
}

// nameRE limits -op and -fixture values to plain benchmark/fixture names so
// they can never inject a "/" level separator or regex syntax.
var nameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// buildBenchPattern turns an operation and/or fixture name into a go test
// -bench pattern. Sub-benchmarks are named Benchmark<Op>/<fixture>, and go test
// matches each "/"-separated level against that level's name, so:
//   - op only:      ^BenchmarkOp$            (all fixtures for that operation)
//   - fixture only: ./^fixture$              (that fixture for every operation)
//   - both:         ^BenchmarkOp$/^fixture$  (one operation on one fixture)
//   - neither:      .                        (everything)
//
// Anchors prevent substring matches such as ThumbnailLarge matching
// ThumbnailLargeWebM, or one fixture name matching a longer one.
func buildBenchPattern(op, fixture string) (string, error) {
	if op != "" && !nameRE.MatchString(op) {
		return "", fmt.Errorf("-op %q must contain only letters, digits, _, . or -", op)
	}
	if fixture != "" && !nameRE.MatchString(fixture) {
		return "", fmt.Errorf("-fixture %q must contain only letters, digits, _, . or -", fixture)
	}
	top := "."
	if op != "" {
		top = "^Benchmark" + regexp.QuoteMeta(op) + "$"
	}
	if fixture == "" {
		return top, nil
	}
	return top + "/^" + regexp.QuoteMeta(fixture) + "$", nil
}

// hostMeta records what produced a result set.
func hostMeta(count int, benchtime, selection string, pgo bool, cpu, jobs int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "date: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "go: %s\n", runtime.Version())
	fmt.Fprintf(&b, "goos/goarch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "cpu: %s\n", cpuModel())
	fmt.Fprintf(&b, "logical_cpus: %d\n", runtime.NumCPU())
	fmt.Fprintf(&b, "git: %s\n", gitRev())
	fmt.Fprintf(&b, "load_start: %s\n", loadAverage())
	fmt.Fprintf(&b, "count: %d\nbenchtime: %s\nselection: %s\npgo: %v\ncpu: %d\njobs: %d\n", count, benchtime, selection, pgo, cpu, jobs)
	return b.String()
}

func cpuModel() string {
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	if runtime.GOOS == "linux" {
		if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "model name" {
					return strings.TrimSpace(v)
				}
			}
		}
	}
	return "unknown"
}

func gitRev() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	dirty := ""
	if st, err := exec.Command("git", "status", "--porcelain").Output(); err == nil && len(bytes.TrimSpace(st)) > 0 {
		dirty = "+dirty"
	}
	return strings.TrimSpace(string(out)) + dirty
}

// loadAverage returns the 1/5/15-minute load average, so noisy runs can be
// identified later (a run started while other benchmarks ran is marked by
// high load).
func loadAverage() string {
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil {
			return strings.Trim(strings.TrimSpace(string(out)), "{} ")
		}
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		return strings.Join(strings.Fields(string(b))[:3], " ")
	}
	// Fallback: parse the "load averages: a b c" tail of uptime(1).
	if out, err := exec.Command("uptime").Output(); err == nil {
		if _, tail, ok := strings.Cut(string(out), "averages:"); ok {
			return strings.Join(strings.Fields(strings.ReplaceAll(tail, ",", " ")), " ")
		}
		if _, tail, ok := strings.Cut(string(out), "average:"); ok {
			return strings.Join(strings.Fields(strings.ReplaceAll(tail, ",", " ")), " ")
		}
	}
	return "unknown"
}

// internalFixtureEnv points the internal benchmarks at the compare harness
// assets (cmd/gomedia-compare/.assets, gitignored) when the caller has not
// set the variables. Without them those benchmarks skip silently, which
// would make two runs cover different cases.
func internalFixtureEnv(suite, _ string) []string {
	env := os.Environ()
	if suite != "internal" {
		return env
	}
	assets, err := filepath.Abs("cmd/gomedia-compare/.assets/video")
	if err != nil {
		return env
	}
	defaults := map[string]string{
		"GOMEDIA_BENCH_MJPEG":     "mjpeg.mp4",
		"GOMEDIA_BENCH_WEBM":      "webm_vp8.webm",
		"GOMEDIA_BENCH_MP4_SHORT": "mp4_h264.mp4",
		"GOMEDIA_BENCH_MP4_LONG":  "mp4_h264_main_1080.mp4",
	}
	have := map[string]bool{}
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok {
			have[k] = true
		}
	}
	for k, f := range defaults {
		p := filepath.Join(assets, f)
		if !have[k] {
			if _, err := os.Stat(p); err == nil {
				env = append(env, k+"="+p)
			}
		}
	}
	return env
}

func dirAbs(p string) string { a, _ := filepath.Abs(p); return a }
