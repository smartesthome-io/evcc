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

.PHONY: ui-flavor container-flavor
