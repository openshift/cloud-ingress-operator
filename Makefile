FIPS_ENABLED=true
include boilerplate/generated-includes.mk

SHELL := /usr/bin/env bash

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update

.PHONY: generate-mocks
generate-mocks:
	go install github.com/golang/mock/mockgen@v1.6.0
	mockgen -source=pkg/cloudclient/cloudclient.go -destination=pkg/cloudclient/mock_cloudclient/mock_cloudclient.go
	mockgen -source=pkg/cloudclient/gcp/gcp.go -destination=pkg/cloudclient/mock_cloudclient/gcp/mock_gcp.go
