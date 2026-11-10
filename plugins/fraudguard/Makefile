GO ?= go
GOSUMDB ?= sum.golang.org
VERSION ?= $(shell git describe --tags --match 'v[0-9]*' --abbrev=0 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= unknown
PLUGIN_PACKAGE := github.com/maximhq/bifrost/plugins/fraudguard
BUILD_LDFLAGS := -X $(PLUGIN_PACKAGE).BuildVersion=$(VERSION) -X $(PLUGIN_PACKAGE).BuildCommit=$(COMMIT) -X $(PLUGIN_PACKAGE).BuildTime=$(BUILD_TIME)
ifeq ($(GOSUMDB),off)
$(warning GOSUMDB=off disables public module checksum verification; using sum.golang.org)
override GOSUMDB = sum.golang.org
endif

.PHONY: help setup build test race vet bench demo simulate chaos redis-check integration sanity verify

help:
	@printf '%s\n' \
		'FraudGuard module targets:' \
		'  make setup       Check local tools and run verification' \
		'  make build        Compile all module packages with release metadata' \
		'  make test         Run all unit and local-fixture tests' \
		'  make race         Run tests with the race detector' \
		'  make vet          Run go vet' \
		'  make bench        Run local detector/guard benchmarks' \
		'  make demo         Run synthetic observe-only examples' \
		'  make chaos        Run local Redis/slow-service fault tests' \
		'  make redis-check  Ping FRAUDGUARD_REDIS_URL without printing it' \
		'  make integration Verify against the pinned Bifrost source (set BIFROST_ROOT)' \
		'  make sanity      Run the local release sanity test with miniredis' \
		'  make verify       Run tests, race, vet, and demo'

setup:
	bash scripts/setup_dev.sh verify

test:
	GOSUMDB=$(GOSUMDB) $(GO) test -count=1 ./...

build:
	GOSUMDB=$(GOSUMDB) $(GO) build -ldflags "$(BUILD_LDFLAGS)" ./...

race:
	GOSUMDB=$(GOSUMDB) $(GO) test -count=1 -race ./...

vet:
	GOSUMDB=$(GOSUMDB) $(GO) vet ./...

bench:
	GOSUMDB=$(GOSUMDB) $(GO) test -run '^$$' -bench 'BenchmarkEngine|BenchmarkInspectConversationContext' -benchmem -benchtime=1s -cpu 1,4 ./...

demo:
	GOSUMDB=$(GOSUMDB) $(GO) run ./cmd/demo

simulate: demo

chaos:
	GOSUMDB=$(GOSUMDB) $(GO) test -count=1 -run '^TestChaos' ./...

redis-check:
	GOSUMDB=$(GOSUMDB) $(GO) run ./cmd/redis-check

integration:
	GOSUMDB=$(GOSUMDB) BIFROST_ROOT="$(BIFROST_ROOT)" bash scripts/verify-pinned-bifrost.sh

sanity:
	GOSUMDB=$(GOSUMDB) $(GO) test -count=1 -run '^TestReleaseSanityObserveOnly$$' ./...

verify: build test race vet demo
