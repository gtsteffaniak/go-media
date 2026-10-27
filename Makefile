# go-media developer targets. CI/compare-gate use pinned ffmpeg 9.0.2 (scripts/install-ffmpeg.sh).
# Local compare uses ffmpeg on PATH when present (see scripts/ffmpeg-path.sh).
FFMPEG_VERSION ?= 9.0.2
FFMPEG_DIR ?= $(HOME)/ffmpeg-$(FFMPEG_VERSION)
COMPARE_REPORT_SITE ?= test/compare/report_site
COMPARE_SERVER_PORT ?= 8765
export FFMPEG_VERSION

.PHONY: test fixtures ffmpeg compare compare-gate ci vet server pgo check-pgo profile bench-thumb bench-micro bench-perf-gate ceiling-profile

vet:
	CGO_ENABLED=0 go vet ./...

test: vet
	CGO_ENABLED=0 go test ./...

fixtures:
	./scripts/generate-fixtures.sh

ffmpeg:
	bash scripts/install-ffmpeg.sh $(FFMPEG_VERSION)

compare:
	bash scripts/run-comparison.sh

compare-gate: ffmpeg
	FFMPEG_DIR="$(FFMPEG_DIR)" CGO_ENABLED=0 COMPARE_GATE=1 bash scripts/run-comparison.sh

server:
	python3 -m http.server $(COMPARE_SERVER_PORT) -d $(COMPARE_REPORT_SITE)

pgo:
	./scripts/regenerate-pgo.sh

check-pgo:
	bash scripts/check-pgo-stale.sh

profile:
	GOMAXPROCS=1 CGO_ENABLED=0 go test -run='^$' -bench='BenchmarkMJPEG|BenchmarkMP4H264' -benchtime=20x -cpuprofile=/tmp/gomedia-thumb.prof ./internal/thumb/
	go tool pprof -top -cum /tmp/gomedia-thumb.prof

bench-thumb:
	GOMAXPROCS=1 CGO_ENABLED=0 go test -run='^$' -bench='BenchmarkMJPEG|BenchmarkMP4H264' -benchtime=10x ./internal/thumb/

bench-micro:
	GOMAXPROCS=1 CGO_ENABLED=0 go test -run='^$$' -bench='BenchmarkIDCT|BenchmarkBaseline|BenchmarkResidual|BenchmarkReadLevels' -benchtime=20x ./internal/kernels/h264/ ./internal/h264/ ./internal/h264/cleanroom/decode/

benchstat-micro:
	bash scripts/perf-micro-benchstat.sh

bench-perf-gate:
	CGO_ENABLED=0 go test ./internal/kernels/h264/ -run TestLayer2 -count=1

ceiling-profile:
	GOMAXPROCS=1 CGO_ENABLED=0 go test -run='^$$' -bench=BenchmarkBaselineThumbnailDecode -benchtime=10x -cpuprofile=/tmp/ceiling.prof ./internal/h264/
	go tool pprof -top -cum /tmp/ceiling.prof

ci: test compare-gate
