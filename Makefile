# go-media developer targets. Comparison uses pinned ffmpeg 9.0.2 (see scripts/install-ffmpeg.sh).
FFMPEG_VERSION ?= 9.0.2
FFMPEG_DIR ?= $(HOME)/ffmpeg-$(FFMPEG_VERSION)
COMPARE_REPORT_SITE ?= test/compare/report_site
COMPARE_SERVER_PORT ?= 8765
export PATH := $(FFMPEG_DIR):$(PATH)

.PHONY: test fixtures ffmpeg compare compare-gate compare-docs ci vet server

vet:
	CGO_ENABLED=0 go vet ./...

test: vet
	CGO_ENABLED=0 go test ./...

fixtures:
	./scripts/generate-fixtures.sh

ffmpeg:
	bash scripts/install-ffmpeg.sh $(FFMPEG_VERSION)

compare: ffmpeg
	bash scripts/run-comparison.sh

compare-gate: ffmpeg
	COMPARE_GATE=1 bash scripts/run-comparison.sh

server:
	python3 -m http.server $(COMPARE_SERVER_PORT) -d $(COMPARE_REPORT_SITE)

compare-docs: compare
	python3 scripts/capture-comparison-docs.py

ci: test compare-gate
