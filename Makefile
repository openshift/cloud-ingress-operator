export KONFLUX_BUILDS=true
FIPS_ENABLED=true
include boilerplate/generated-includes.mk

SHELL := /usr/bin/env bash

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update
