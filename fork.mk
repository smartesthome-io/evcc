# fork-only targets, included from Makefile

FLAVOR ?= claude-simple
BASE_IMAGE ?= evcc:latest
CONTAINER ?= podman

# build a UI flavor from ui/<FLAVOR> into ui/<FLAVOR>/dist
ui-flavor:
	cd ui/$(FLAVOR) && npm ci && npm run build

# thin image: base evcc image plus the flavor served via EVCC_UI_DIR
container-flavor:
	$(CONTAINER) build -f Dockerfile.flavor --build-arg FLAVOR=$(FLAVOR) --build-arg BASE_IMAGE=$(BASE_IMAGE) -t evcc:$(FLAVOR) .

REGISTRY ?= docker.io/smartesthomeio/evcc
TAG ?= dev
PLATFORMS ?= amd64 arm64

# multi-arch base (<TAG>) and flavor (<TAG>-<FLAVOR>) images pushed to REGISTRY
container-push:
	set -e; for a in $(PLATFORMS); do \
		$(CONTAINER) build --format docker --platform linux/$$a -t localhost/evcc-base:$$a .; \
		$(CONTAINER) build --format docker --platform linux/$$a -f Dockerfile.flavor --build-arg FLAVOR=$(FLAVOR) --build-arg BASE_IMAGE=localhost/evcc-base:$$a -t localhost/evcc-flavor:$$a .; \
	done; \
	for pair in "$(TAG):evcc-base" "$(TAG)-$(FLAVOR):evcc-flavor"; do \
		tag=$${pair%%:*}; img=$${pair##*:}; \
		$(CONTAINER) manifest rm localhost/list-$$tag >/dev/null 2>&1 || true; \
		$(CONTAINER) manifest create localhost/list-$$tag >/dev/null; \
		for a in $(PLATFORMS); do $(CONTAINER) manifest add localhost/list-$$tag containers-storage:localhost/$$img:$$a >/dev/null; done; \
		$(CONTAINER) manifest push --all --format docker localhost/list-$$tag docker://$(REGISTRY):$$tag; \
	done

.PHONY: ui-flavor container-flavor container-push
