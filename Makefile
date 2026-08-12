# Modified by PastureStack contributors for independent maintenance and rebranding.
TARGETS := $(shell ls scripts)

DAPPER_IMAGE ?= pasturestack-external-dns-sync-dapper:ubuntu26
DAPPER_HOST_ARCH ?= amd64
DOCKER_BUILD_NETWORK ?= host

.dapper-image: Dockerfile.dapper ubuntu-apt.lock
	docker build \
		--network $(DOCKER_BUILD_NETWORK) \
		--build-arg DAPPER_HOST_ARCH=$(DAPPER_HOST_ARCH) \
		-t $(DAPPER_IMAGE) \
		-f Dockerfile.dapper .

$(TARGETS): .dapper-image
	docker run --rm \
		-v $(CURDIR):/go/src/github.com/PastureStack/external-dns-sync \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-e DAPPER_UID=$$(id -u) \
		-e DAPPER_GID=$$(id -g) \
		-e ARCH=$(DAPPER_HOST_ARCH) \
		-e REPO \
		-e IMAGE_NAMESPACE \
		-e TAG \
		-e VERSION_OVERRIDE \
		-e REVISION_OVERRIDE \
		-e DOCKER_BUILD_NETWORK=$(DOCKER_BUILD_NETWORK) \
		$(DAPPER_IMAGE) $@

trash:
	@echo "Dependencies are vendored; no legacy trash dependency sync is required."

trash-keep: trash

deps: trash

.DEFAULT_GOAL := ci

.PHONY: $(TARGETS) deps trash trash-keep
