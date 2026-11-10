GO ?= go
GOSUMDB ?= sum.golang.org
FUZZTIME ?= 3s

.PHONY: help fraudguard-build fraudguard-test fraudguard-race fraudguard-vet fraudguard-fuzz-smoke fraudguard-chaos fraudguard-demo fraudguard-compose-demo fraudguard-load-test fraudguard-integration fraudguard-sanity fraudguard-verify fraudguard-release-preflight

help:
	@printf '%s\n' \
		'FraudGuard development commands:' \
		'  make fraudguard-build          Compile the Go plugin module' \
		'  make fraudguard-test           Run unit and local-fixture tests' \
		'  make fraudguard-race           Run tests with the race detector' \
		'  make fraudguard-vet            Run go vet' \
		'  make fraudguard-fuzz-smoke     Run bounded local Go fuzz checks' \
		'  make fraudguard-chaos          Run Redis-outage and slow-service fault tests' \
		'  make fraudguard-demo           Run the local synthetic observe-only demo' \
		'  make fraudguard-compose-demo   Run the disposable pinned-Bifrost Docker demo' \
		'  make fraudguard-load-test      Run the localhost-only k6 gateway test' \
		'  make fraudguard-integration    Check against BIFROST_ROOT' \
		'  make fraudguard-sanity         Run the integrated local release sanity test' \
		'  make fraudguard-release-preflight Run local release checks; never deploys' \
		'  make fraudguard-verify         Run build, tests, race, vet, and demo'

fraudguard-build:
	$(MAKE) -C plugins/fraudguard build GO="$(GO)" GOSUMDB="$(GOSUMDB)"

fraudguard-test:
	$(MAKE) -C plugins/fraudguard test GO="$(GO)" GOSUMDB="$(GOSUMDB)"

fraudguard-race:
	$(MAKE) -C plugins/fraudguard race GO="$(GO)" GOSUMDB="$(GOSUMDB)"

fraudguard-vet:
	$(MAKE) -C plugins/fraudguard vet GO="$(GO)" GOSUMDB="$(GOSUMDB)"

fraudguard-fuzz-smoke:
	$(MAKE) -C plugins/fraudguard fuzz-smoke GO="$(GO)" GOSUMDB="$(GOSUMDB)" FUZZTIME="$(FUZZTIME)"

fraudguard-chaos:
	$(MAKE) -C plugins/fraudguard chaos GO="$(GO)" GOSUMDB="$(GOSUMDB)"

fraudguard-demo:
	$(MAKE) -C plugins/fraudguard demo GO="$(GO)" GOSUMDB="$(GOSUMDB)"

fraudguard-compose-demo:
	bash plugins/fraudguard/scripts/demo_walkthrough.sh --compose

fraudguard-load-test:
	@command -v k6 >/dev/null || { echo "k6 is required; install it before running the local load test." >&2; exit 127; }
	k6 run plugins/fraudguard/scripts/load-test.js

fraudguard-integration:
	$(MAKE) -C plugins/fraudguard integration GO="$(GO)" GOSUMDB="$(GOSUMDB)" BIFROST_ROOT="$(BIFROST_ROOT)"

fraudguard-sanity:
	$(MAKE) -C plugins/fraudguard sanity GO="$(GO)" GOSUMDB="$(GOSUMDB)"

fraudguard-verify:
	$(MAKE) -C plugins/fraudguard verify GO="$(GO)" GOSUMDB="$(GOSUMDB)"

fraudguard-release-preflight:
	bash scripts/deploy.sh preflight
