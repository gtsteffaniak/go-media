package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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

var labelRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// cmdRun runs the selected suites with go test -bench and stores the raw
// output in <out>/<label>/<suite>.txt, plus a meta.txt describing the host
// and source revision so results can be compared later.
func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	label := fs.String("label", "", "result label, e.g. baseline or opt-h264-idct (required)")
	suite := fs.String("suite", "all", "e2e, internal or all")
	count := fs.Int("count", 5, "go test -count: repetitions per benchmark")
	benchtime := fs.String("benchtime", "1s", "go test -benchtime per repetition (e.g. 1s or 20x)")
	filter := fs.String("bench", ".", "go test -bench regexp")
	pgo := fs.Bool("pgo", true, "build with -pgo=default.pgo (production profile)")
	cpu := fs.Int("cpu", 1, "go test -cpu (GOMAXPROCS); 1 matches the compare harness timing")
	outDir := fs.String("out", "bench/results", "results directory")
	jobs := fs.Int("jobs", defaultJobs, "max benchmark jobs (go test processes) running at once; 1 for timing comparisons")
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

	dir := filepath.Join(*outDir, *label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	meta := hostMeta(*count, *benchtime, *filter, *pgo, *cpu, *jobs)
	if err := os.WriteFile(filepath.Join(dir, "meta.txt"), []byte(meta), 0o644); err != nil {
		return err
	}

	// Each package is one go test job. runJobs keeps at most -jobs of them
	// running at once; outputs are reassembled per suite below.
	type benchJob struct{ suite, pkg string }
	var plan []benchJob
	for _, name := range names {
		for _, pkg := range suites[name] {
			plan = append(plan, benchJob{suite: name, pkg: pkg})
		}
	}
	outs, errs := runJobs(len(plan), *jobs, func(i int) ([]byte, error) {
		j := plan[i]
		argv := []string{"test", "-run", "^$", "-bench", *filter, "-benchmem",
			"-count", fmt.Sprint(*count), "-benchtime", *benchtime,
			"-cpu", fmt.Sprint(*cpu), "-timeout", "120m"}
		if *pgo {
			argv = append(argv, "-pgo=default.pgo")
		}
		argv = append(argv, j.pkg)
		fmt.Printf("== suite %s %s: go %s\n", j.suite, j.pkg, strings.Join(argv, " "))
		var out bytes.Buffer
		cmd := exec.Command("go", argv...)
		cmd.Env = append(internalFixtureEnv(j.suite, dirAbs(*outDir)), "CGO_ENABLED=0")
		cmd.Stdout = &out
		cmd.Stderr = os.Stderr
		start := time.Now()
		runErr := cmd.Run()
		fmt.Printf("   %s %s finished in %s\n", j.suite, j.pkg, time.Since(start).Round(time.Second))
		return out.Bytes(), runErr
	})

	var failed []string
	for _, name := range names {
		var suiteOut bytes.Buffer
		for i, j := range plan {
			if j.suite != name {
				continue
			}
			suiteOut.Write(outs[i])
			if errs[i] != nil {
				failed = append(failed, fmt.Sprintf("%s %s: %v", name, j.pkg, errs[i]))
			}
		}
		path := filepath.Join(dir, name+".txt")
		if err := os.WriteFile(path, suiteOut.Bytes(), 0o644); err != nil {
			return err
		}
	}
	if f, err := os.OpenFile(filepath.Join(dir, "meta.txt"), os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(f, "load_end: %s\n", loadAverage())
		f.Close()
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d job(s) failed; partial output kept in %s:\n  %s", len(failed), dir, strings.Join(failed, "\n  "))
	}
	fmt.Printf("results written to %s\n", dir)
	return nil
}

// hostMeta records what produced a result set.
func hostMeta(count int, benchtime, filter string, pgo bool, cpu, jobs int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "date: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "go: %s\n", runtime.Version())
	fmt.Fprintf(&b, "goos/goarch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "cpu: %s\n", cpuModel())
	fmt.Fprintf(&b, "logical_cpus: %d\n", runtime.NumCPU())
	fmt.Fprintf(&b, "git: %s\n", gitRev())
	fmt.Fprintf(&b, "load_start: %s\n", loadAverage())
	fmt.Fprintf(&b, "count: %d\nbenchtime: %s\nbench: %s\npgo: %v\ncpu: %d\njobs: %d\n", count, benchtime, filter, pgo, cpu, jobs)
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
