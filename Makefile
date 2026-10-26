# go-media developer targets. CI/compare-gate use pinned ffmpeg 9.0.2 (scripts/install-ffmpeg.sh).
# Local compare uses ffmpeg on PATH when present (see scripts/ffmpeg-path.sh).
FFMPEG_VERSION ?= 9.0.2
FFMPEG_DIR ?= $(HOME)/ffmpeg-$(FFMPEG_VERSION)
COMPARE_REPORT_SITE ?= test/compare/report_site
COMPARE_SERVER_PORT ?= 8765
export FFMPEG_DIR FFMPEG_VERSION

.PHONY: test fixtures ffmpeg compare compare-gate compare-docs ci vet server pgo check-pgo profile bench-thumb

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
	CGO_ENABLED=0 COMPARE_GATE=1 bash scripts/run-comparison.sh

server:
	python3 -m http.server $(COMPARE_SERVER_PORT) -d $(COMPARE_REPORT_SITE)

compare-docs: compare
	python3 scripts/capture-comparison-docs.py

pgo:
	./scripts/regenerate-pgo.sh

check-pgo:
	bash scripts/check-pgo-stale.sh

profile:
	GOMAXPROCS=1 CGO_ENABLED=0 go test -run='^$' -bench='BenchmarkMJPEG|BenchmarkMP4H264' -benchtime=20x -cpuprofile=/tmp/gomedia-thumb.prof ./internal/thumb/
	go tool pprof -top -cum /tmp/gomedia-thumb.prof

bench-thumb:
	GOMAXPROCS=1 CGO_ENABLED=0 go test -run='^$' -bench='BenchmarkMJPEG|BenchmarkMP4H264' -benchtime=10x ./internal/thumb/

ci: test compare-gate
