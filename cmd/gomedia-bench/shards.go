package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// A shard is the raw go test output of one job (one fixture, the micro
// benchmarks, or one internal package). Each job writes its shard exactly once,
// when it finishes, so a shard is never rewritten while a run is in progress.
//
// Shard layout:
//
//	# gomedia-bench shard job=<name> status=<ok|failed> settings=<settings>
//	<go test output>
//
// The report files (e2e.txt, internal.txt) are rebuilt from the ok shards by
// mergeSuite, which runs once at the end of every run.
const shardMagic = "# gomedia-bench shard "

// mergeWait bounds how long a run waits for another run's merge to finish.
const mergeWait = 2 * time.Minute

// settingsKey identifies the measurement settings a shard was produced with.
// Shards with different settings must not be merged into one report.
func settingsKey(count int, benchtime string, cpu int, pgo bool) string {
	return fmt.Sprintf("count=%d;benchtime=%s;cpu=%d;pgo=%v", count, benchtime, cpu, pgo)
}

// shardFileName turns a job name into a file name, e.g. "e2e/fixture" into
// "fixture" under the e2e directory, or an internal package path into a flat
// name.
func shardFileName(job string) string {
	r := strings.NewReplacer("./", "", "/", "_", "\\", "_", " ", "_")
	return strings.Trim(r.Replace(job), "_.") + ".txt"
}

// encodeShard returns the shard file contents for one job.
func encodeShard(job, status, settings string, body []byte) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%sjob=%s status=%s settings=%s\n", shardMagic, job, status, settings)
	b.Write(body)
	return b.Bytes()
}

type shard struct {
	job, status, settings string
	body                  []byte
}

// decodeShard parses a shard file written by encodeShard.
func decodeShard(data []byte) (shard, error) {
	header, body, ok := bytes.Cut(data, []byte("\n"))
	if !ok || !bytes.HasPrefix(header, []byte(shardMagic)) {
		return shard{}, fmt.Errorf("not a gomedia-bench shard")
	}
	var s shard
	for _, field := range strings.Fields(string(header[len(shardMagic):])) {
		k, v, found := strings.Cut(field, "=")
		if !found {
			continue
		}
		switch k {
		case "job":
			s.job = v
		case "status":
			s.status = v
		case "settings":
			s.settings = v
		}
	}
	if s.job == "" || s.status == "" || s.settings == "" {
		return shard{}, fmt.Errorf("shard header is missing job, status or settings")
	}
	s.body = body
	return s, nil
}

// mergeSuite rebuilds <dir>/<suite>.txt from every ok shard in
// <dir>/shards/<suite>/. Failed shards are left out and reported in skipped.
// The write is atomic and runs under the label's merge lock, so concurrent
// runs cannot interleave their rebuilds. It refuses to combine shards that were
// measured with different settings.
func mergeSuite(dir, suite string) (merged int, skipped []string, err error) {
	shardDir := filepath.Join(dir, "shards", suite)
	files, err := filepath.Glob(filepath.Join(shardDir, "*.txt"))
	if err != nil {
		return 0, nil, err
	}
	if len(files) == 0 {
		return 0, nil, nil
	}
	sort.Strings(files)

	release, err := acquireFileLockWait(filepath.Join(dir, ".merge.lock"), mergeWait)
	if err != nil {
		return 0, nil, fmt.Errorf("merge %s: %w", suite, err)
	}
	defer release()

	var out bytes.Buffer
	settings := ""
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return 0, nil, err
		}
		s, err := decodeShard(data)
		if err != nil {
			return 0, nil, fmt.Errorf("%s: %w", f, err)
		}
		if s.status != "ok" {
			skipped = append(skipped, s.job)
			continue
		}
		if settings == "" {
			settings = s.settings
		} else if s.settings != settings {
			return 0, nil, fmt.Errorf("%s was measured with %q but earlier shards used %q; use a new label or remove the older shards", s.job, s.settings, settings)
		}
		out.Write(s.body)
		merged++
	}
	if merged == 0 {
		return 0, skipped, nil
	}
	path := filepath.Join(dir, suite+".txt")
	if err := writeFileAtomic(path, out.Bytes()); err != nil {
		return 0, nil, err
	}
	return merged, skipped, nil
}
