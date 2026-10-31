package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShardRoundTrip(t *testing.T) {
	data := encodeShard("mp4_h264_640x360_20s", "ok", "count=3;benchtime=1s;cpu=1;pgo=true", []byte("BenchmarkX 1 5 ns/op\n"))
	s, err := decodeShard(data)
	if err != nil {
		t.Fatal(err)
	}
	if s.job != "mp4_h264_640x360_20s" || s.status != "ok" || s.settings != "count=3;benchtime=1s;cpu=1;pgo=true" {
		t.Fatalf("header not preserved: %+v", s)
	}
	if string(s.body) != "BenchmarkX 1 5 ns/op\n" {
		t.Fatalf("body not preserved: %q", s.body)
	}
}

func TestDecodeShardRejectsForeignFiles(t *testing.T) {
	for _, bad := range []string{"", "BenchmarkX 1 5 ns/op\n", "# gomedia-bench shard job=x status=ok\n"} {
		if _, err := decodeShard([]byte(bad)); err == nil {
			t.Errorf("decodeShard(%q): expected error", bad)
		}
	}
}

func TestShardFileNameIsFlatAndSafe(t *testing.T) {
	cases := map[string]string{
		"mp4_h264_640x360_20s":           "mp4_h264_640x360_20s.txt",
		"micro":                          "micro.txt",
		"internal/h264/cleanroom/decode": "internal_h264_cleanroom_decode.txt",
	}
	for in, want := range cases {
		if got := shardFileName(in); got != want {
			t.Errorf("shardFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func writeShardFile(t *testing.T, dir, suite, name string, data []byte) {
	t.Helper()
	p := filepath.Join(dir, "shards", suite, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSuiteUsesOnlyOkShards(t *testing.T) {
	dir := t.TempDir()
	const settings = "count=3;benchtime=1s;cpu=1;pgo=true"
	writeShardFile(t, dir, "e2e", "a.txt", encodeShard("a", "ok", settings, []byte("A-result\n")))
	writeShardFile(t, dir, "e2e", "b.txt", encodeShard("b", "failed", settings, []byte("B-partial\n")))
	writeShardFile(t, dir, "e2e", "c.txt", encodeShard("c", "ok", settings, []byte("C-result\n")))

	merged, skipped, err := mergeSuite(dir, "e2e")
	if err != nil {
		t.Fatal(err)
	}
	if merged != 2 || len(skipped) != 1 || skipped[0] != "b" {
		t.Fatalf("merged=%d skipped=%v", merged, skipped)
	}
	got, err := os.ReadFile(filepath.Join(dir, "e2e.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "A-result\nC-result\n" {
		t.Fatalf("report = %q", got)
	}
}

func TestMergeSuiteRefusesMixedSettings(t *testing.T) {
	dir := t.TempDir()
	writeShardFile(t, dir, "e2e", "a.txt", encodeShard("a", "ok", "count=3;benchtime=1s;cpu=1;pgo=true", []byte("A\n")))
	writeShardFile(t, dir, "e2e", "b.txt", encodeShard("b", "ok", "count=5;benchtime=1s;cpu=1;pgo=true", []byte("B\n")))

	if _, _, err := mergeSuite(dir, "e2e"); err == nil || !strings.Contains(err.Error(), "measured with") {
		t.Fatalf("expected mixed-settings error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "e2e.txt")); !os.IsNotExist(err) {
		t.Fatal("report must not be written when settings are mixed")
	}
}

func TestMergeSuiteWithNoShardsWritesNothing(t *testing.T) {
	dir := t.TempDir()
	merged, _, err := mergeSuite(dir, "e2e")
	if err != nil || merged != 0 {
		t.Fatalf("merged=%d err=%v", merged, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "e2e.txt")); !os.IsNotExist(err) {
		t.Fatal("report written with no shards")
	}
}
