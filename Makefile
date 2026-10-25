# go-media developer targets. Comparison uses pinned ffmpeg 9.0.2 (see scripts/install-ffmpeg.sh).
FFMPEG_VERSION ?= 9.0.2
FFMPEG_DIR ?= $(HOME)/ffmpeg-$(FFMPEG_VERSION)
export PATH := $(FFMPEG_DIR):$(PATH)

.PHONY: test fixtures ffmpeg compare compare-gate ci vet

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

ci: test compare-gate
