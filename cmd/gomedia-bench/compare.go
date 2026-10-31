package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// sample holds the ns/op repetitions of one benchmark in one result label.
type sample struct {
	ns []float64
}

// stats summarises repetitions in milliseconds.
type stats struct {
	n      int
	mean   float64 // ms
	sd     float64 // ms, sample standard deviation
	cv     float64 // percent
	values []float64
}

// row is one benchmark compared between two labels.
type row struct {
	name       string
	op, format string
	base, new  stats
	ok         bool
	delta      float64 // fraction, new/base - 1
	t          float64
	verdict    string
}

// knownFormats are fixture-name prefixes used to group results by format.
var knownFormats = []string{"mp4", "mkv", "webm", "mp3", "flac", "ogg", "wav", "m4a", "srt", "ass", "vtt"}

var msKeys = map[string]bool{"ns/op": true}

// cmdCompare compares two result label directories. For every benchmark
// present in both it prints mean ± sample stddev in ms, the coefficient of
// variation, % change, Welch's t, and a verdict. A change counts only when it
// is at least 5% and statistically distinct (|t| above the 95% critical
// value); otherwise it is reported as unchanged. The summary gives per-op and
// per-format geometric means and the cumulative improvement.
func cmdCompare(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	format := fs.String("format", "md", "output format: md or text")
	threshold := fs.Float64("threshold", 5, "minimum %% change to count as improved/regressed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("usage: compare [-format md|text] BASE_DIR NEW_DIR")
	}
	base, err := readLabel(fs.Arg(0))
	if err != nil {
		return err
	}
	cur, err := readLabel(fs.Arg(1))
	if err != nil {
		return err
	}

	var rows []row
	for name, b := range base {
		n, ok := cur[name]
		if !ok {
			continue
		}
		r := row{name: name, base: summarize(b.ns), new: summarize(n.ns), ok: true}
		r.op, r.format = splitName(name)
		r.delta = r.new.mean/r.base.mean - 1
		r.t = welchT(r.base, r.new)
		r.verdict = verdict(r, *threshold)
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	missingNew, missingBase := diffNames(base, cur), diffNames(cur, base)
	if len(rows) == 0 {
		return fmt.Errorf("no benchmarks in common between %s and %s", fs.Arg(0), fs.Arg(1))
	}

	if *format == "text" {
		printText(rows, fs.Arg(0), fs.Arg(1), missingNew, missingBase)
	} else {
		printMarkdown(rows, fs.Arg(0), fs.Arg(1), missingNew, missingBase)
	}
	return nil
}

// readLabel parses every suite file (*.txt, excluding meta.txt) in dir.
func readLabel(dir string) (map[string]*sample, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no result files in %s", dir)
	}
	out := map[string]*sample{}
	for _, f := range files {
		if filepath.Base(f) == "meta.txt" {
			continue
		}
		fh, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			name, ns, ok := parseBenchLine(sc.Text())
			if !ok {
				continue
			}
			s := out[name]
			if s == nil {
				s = &sample{}
				out[name] = s
			}
			s.ns = append(s.ns, ns)
		}
		fh.Close()
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
	}
	return out, nil
}

// parseBenchLine parses "BenchmarkName[/sub]  N  123 ns/op ...".
func parseBenchLine(line string) (string, float64, bool) {
	if !strings.HasPrefix(line, "Benchmark") {
		return "", 0, false
	}
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return "", 0, false
	}
	for i := 2; i+1 < len(fields); i++ {
		if msKeys[fields[i+1]] {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				return "", 0, false
			}
			return strings.TrimPrefix(fields[0], "Benchmark"), v, true
		}
	}
	return "", 0, false
}

func summarize(ns []float64) stats {
	st := stats{n: len(ns)}
	if len(ns) == 0 {
		return st
	}
	ms := make([]float64, len(ns))
	for i, v := range ns {
		ms[i] = v / 1e6
	}
	st.values = ms
	var sum float64
	for _, v := range ms {
		sum += v
	}
	st.mean = sum / float64(len(ms))
	if len(ms) > 1 {
		var ss float64
		for _, v := range ms {
			d := v - st.mean
			ss += d * d
		}
		st.sd = math.Sqrt(ss / float64(len(ms)-1))
	}
	if st.mean > 0 {
		st.cv = st.sd / st.mean * 100
	}
	return st
}

// welchT returns Welch's t statistic for new minus base.
func welchT(b, n stats) float64 {
	if b.n < 2 || n.n < 2 {
		return 0
	}
	se := math.Sqrt(b.sd*b.sd/float64(b.n) + n.sd*n.sd/float64(n.n))
	if se == 0 {
		return math.Inf(sign(n.mean - b.mean))
	}
	return (n.mean - b.mean) / se
}

func sign(v float64) int {
	if v < 0 {
		return -1
	}
	return 1
}

// tCritical95 returns the two-sided 95% Student t critical value for df.
func tCritical95(df float64) float64 {
	table := []float64{12.71, 4.30, 3.18, 2.78, 2.57, 2.45, 2.36, 2.31, 2.26, 2.23, 2.20, 2.18, 2.16, 2.14, 2.13}
	d := int(math.Floor(df))
	switch {
	case d < 1:
		return 12.71
	case d <= len(table):
		return table[d-1]
	case d < 30:
		return 2.10
	default:
		return 1.96
	}
}

func verdict(r row, thresholdPct float64) string {
	if !r.ok || r.base.n < 2 || r.new.n < 2 {
		return "n<2"
	}
	// Welch-Satterthwaite degrees of freedom.
	vb := r.base.sd * r.base.sd / float64(r.base.n)
	vn := r.new.sd * r.new.sd / float64(r.new.n)
	df := 1e9
	if vb+vn > 0 {
		df = (vb + vn) * (vb + vn) / (vb*vb/float64(r.base.n-1) + vn*vn/float64(r.new.n-1))
	}
	sig := math.Abs(r.t) >= tCritical95(df)
	pct := r.delta * 100
	var v string
	switch {
	case sig && pct <= -thresholdPct:
		v = "improved"
	case sig && pct >= thresholdPct:
		v = "REGRESSED"
	case sig:
		v = "changed <threshold"
	default:
		v = "unchanged"
	}
	if r.base.cv > 5 || r.new.cv > 5 {
		v += " (noisy)"
	}
	return v
}

// splitName derives the operation and format from a benchmark name such as
// "Probe/mp4_h264_1080p" -> ("Probe", "mp4").
func splitName(name string) (op, format string) {
	op, sub, _ := strings.Cut(name, "/")
	format = "-"
	for _, f := range knownFormats {
		if strings.HasPrefix(sub, f+"_") || sub == f {
			format = f
			break
		}
	}
	return op, format
}

func diffNames(a, b map[string]*sample) []string {
	var out []string
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// geomeanSummary returns the geometric mean of new/base ratios over rows.
func geomeanSummary(rows []row) (ratio float64, n int) {
	sum := 0.0
	for _, r := range rows {
		if r.base.mean <= 0 || r.new.mean <= 0 {
			continue
		}
		sum += math.Log(r.new.mean / r.base.mean)
		n++
	}
	if n == 0 {
		return 1, 0
	}
	return math.Exp(sum / float64(n)), n
}

func fmtMS(v float64) string {
	switch {
	case v >= 100:
		return fmt.Sprintf("%.1f", v)
	case v >= 1:
		return fmt.Sprintf("%.3f", v)
	default:
		return fmt.Sprintf("%.4f", v)
	}
}

func fmtSD(s stats) string {
	return fmt.Sprintf("%s ± %s", fmtMS(s.mean), fmtMS(s.sd))
}

func printMarkdown(rows []row, baseDir, newDir string, onlyBase, onlyNew []string) {
	fmt.Printf("Compare `%s` (base) → `%s` (new). Times are ms/op, mean ± sample stddev over the -count repetitions.\n\n", baseDir, newDir)
	fmt.Println("| Benchmark | Base ms | New ms | CV% base/new | Δ% | t | Verdict |")
	fmt.Println("|---|---|---|---|---|---|---|")
	for _, r := range rows {
		fmt.Printf("| `%s` | %s | %s | %.1f / %.1f | %+.1f%% | %.1f | %s |\n",
			r.name, fmtSD(r.base), fmtSD(r.new), r.base.cv, r.new.cv, r.delta*100, r.t, r.verdict)
	}
	fmt.Println()
	printSummaryMD(rows)
	printNames(onlyBase, "only in base")
	printNames(onlyNew, "only in new")
}

func printText(rows []row, baseDir, newDir string, onlyBase, onlyNew []string) {
	fmt.Printf("base=%s new=%s\n", baseDir, newDir)
	for _, r := range rows {
		fmt.Printf("%-60s base=%-22s new=%-22s d=%+7.2f%% t=%6.1f %s\n",
			r.name, fmtSD(r.base), fmtSD(r.new), r.delta*100, r.t, r.verdict)
	}
	printSummaryMD(rows)
	printNames(onlyBase, "only in base")
	printNames(onlyNew, "only in new")
}

func printSummaryMD(rows []row) {
	ratio, n := geomeanSummary(rows)
	fmt.Printf("**Overall:** %d benchmarks, geometric-mean time ratio %.4f → cumulative change **%+.1f%%** (negative is faster).\n\n", n, ratio, (ratio-1)*100)

	count := map[string]int{}
	for _, r := range rows {
		key := strings.Fields(r.verdict + " ")[0]
		count[key]++
	}
	fmt.Printf("Verdicts: improved=%d, REGRESSED=%d, changed<threshold=%d, unchanged=%d, n<2=%d\n\n",
		count["improved"], count["REGRESSED"], count["changed"], count["unchanged"], count["n<2"])

	group := func(title string, key func(row) string) {
		groups := map[string][]row{}
		var keys []string
		for _, r := range rows {
			k := key(r)
			if _, ok := groups[k]; !ok {
				keys = append(keys, k)
			}
			groups[k] = append(groups[k], r)
		}
		sort.Strings(keys)
		fmt.Printf("| %s | benchmarks | geomean Δ%% |\n|---|---|---|\n", title)
		for _, k := range keys {
			g, n := geomeanSummary(groups[k])
			fmt.Printf("| %s | %d | %+.1f%% |\n", k, n, (g-1)*100)
		}
		fmt.Println()
	}
	group("Operation", func(r row) string { return r.op })
	group("Format", func(r row) string { return r.format })
}

func printNames(names []string, title string) {
	if len(names) == 0 {
		return
	}
	fmt.Printf("%s: %s\n\n", title, strings.Join(names, ", "))
}
