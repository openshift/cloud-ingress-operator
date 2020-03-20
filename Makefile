SHELL := /usr/bin/env bash

# Include shared Makefiles
include project.mk
include standard.mk
include functions.mk

default: generate-syncset gobuild

# Extend Makefile after here
CONTAINER_ENGINE?=docker

# Build the docker image
.PHONY: container-build
container-build:
	$(MAKE) build

# Push the docker image
.PHONY: container-push
container-push:
	$(MAKE) push

.PHONY: operator-sdk-generate
operator-sdk-generate:
	operator-sdk generate crds
	operator-sdk generate k8s

.PHONY: generate-syncset
generate-syncset:
	if [ "${IN_CONTAINER}" == "true" ]; then \
		$(CONTAINER_ENGINE) run --rm -v `pwd -P`:`pwd -P` python:2.7.15 /bin/sh -c "cd `pwd`; pip install oyaml; `pwd`/${GEN_SYNCSET}"; \
	else \
		${GEN_SYNCSET}; \
	fi

