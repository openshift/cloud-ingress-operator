# Validate variables in project.mk exist
ifndef IMAGE_REGISTRY
$(error IMAGE_REGISTRY is not set; check project.mk file)
endif
ifndef IMAGE_REPOSITORY
$(error IMAGE_REPOSITORY is not set; check project.mk file)
endif
ifndef IMAGE_NAME
$(error IMAGE_NAME is not set; check project.mk file)
endif
ifndef VERSION_MAJOR
$(error VERSION_MAJOR is not set; check project.mk file)
endif
ifndef VERSION_MINOR
$(error VERSION_MINOR is not set; check project.mk file)
endif

# Generate version and tag information from inputs
COMMIT_NUMBER=$(shell git rev-list `git rev-list --parents HEAD | egrep "^[a-f0-9]{40}$$"`..HEAD --count)
CURRENT_COMMIT=$(shell git rev-parse --short=7 HEAD)
OPERATOR_VERSION=$(VERSION_MAJOR).$(VERSION_MINOR).$(COMMIT_NUMBER)-$(CURRENT_COMMIT)

IMG?=$(IMAGE_REGISTRY)/$(IMAGE_REPOSITORY)/$(IMAGE_NAME):v$(OPERATOR_VERSION)
OPERATOR_IMAGE_URI=${IMG}
OPERATOR_IMAGE_URI_LATEST=$(IMAGE_REGISTRY)/$(IMAGE_REPOSITORY)/$(IMAGE_NAME):latest
OPERATOR_DOCKERFILE ?=build/Dockerfile

BINFILE=build/_output/bin/$(OPERATOR_NAME)
MAINPACKAGE=./cmd/manager
unexport GOFLAGS
GOENV=GOOS=linux GOARCH=amd64 CGO_ENABLED=0
GOBUILDFLAGS=-gcflags="all=-trimpath=${GOPATH}" -asmflags="all=-trimpath=${GOPATH}"

CONTAINER_ENGINE=$(shell which podman 2>/dev/null || which docker 2>/dev/null)

# ex, -v
TESTOPTS := -timeout 1m

ALLOW_DIRTY_CHECKOUT?=false

default: gobuild

.PHONY: clean
clean:
	rm -rf ./build/_output
	rm -rf bundles-staging/ bundles-production/ saas-*-bundle/

.PHONY: isclean
isclean:
	@(test "$(ALLOW_DIRTY_CHECKOUT)" != "false" || test 0 -eq $$(git status --porcelain | wc -l)) || (echo "Local git checkout is not clean, commit changes and try again." >&2 && exit 1)

.PHONY: build
build: isclean envtest
	${CONTAINER_ENGINE} build . -f $(OPERATOR_DOCKERFILE) -t $(OPERATOR_IMAGE_URI)
	${CONTAINER_ENGINE} tag $(OPERATOR_IMAGE_URI) $(OPERATOR_IMAGE_URI_LATEST)

.PHONY: push
push:
	${CONTAINER_ENGINE} push $(OPERATOR_IMAGE_URI)
	${CONTAINER_ENGINE} push $(OPERATOR_IMAGE_URI_LATEST)

.PHONY: skopeo-push
skopeo-push: container-build
	skopeo copy \
		--dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
		"docker-daemon:${OPERATOR_IMAGE_URI_LATEST}" \
		"docker://${OPERATOR_IMAGE_URI_LATEST}"
	skopeo copy \
		--dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
		"docker-daemon:${OPERATOR_IMAGE_URI}" \
		"docker://${OPERATOR_IMAGE_URI}"

.PHONY: build-catalog-image
build-catalog-image:
	$(call create_push_catalog_image,staging,service/saas-$(OPERATOR_NAME)-bundle,$$APP_SRE_BOT_PUSH_TOKEN,false,service/saas-osd-operators,$(OPERATOR_NAME)-services/$(OPERATOR_NAME).yaml,hack/generate-operator-bundle.py,$(CATALOG_REGISTRY_ORGANIZATION))
	$(call create_push_catalog_image,production,service/saas-$(OPERATOR_NAME)-bundle,$$APP_SRE_BOT_PUSH_TOKEN,true,service/saas-osd-operators,$(OPERATOR_NAME)-services/$(OPERATOR_NAME).yaml,hack/generate-operator-bundle.py,$(CATALOG_REGISTRY_ORGANIZATION))

.PHONY: gocheck
gocheck: ## Lint code
	gofmt -s -l $$(go list -f '{{ .Dir }}' ./... ) | grep ".*\.go"; if [ "$$?" = "0" ]; then gofmt -s -d $$(go list -f '{{ .Dir }}' ./... ); exit 1; fi
	go vet ./cmd/... ./pkg/...

.PHONY: gobuild
gobuild: gocheck gotest ## Build binary
	${GOENV} go build ${GOBUILDFLAGS} -o ${BINFILE} ${MAINPACKAGE}

.PHONY: gotest
gotest:
	go test $(TESTOPTS) $$(go list -mod=readonly -e ./...)

.PHONY: envtest
envtest:
	@# test that the env target can be evaluated, required by osd-operators-registry
	@eval $$($(MAKE) env --no-print-directory) || (echo 'Unable to evaulate output of `make env`.  This breaks osd-operators-registry.' >&2 && exit 1)

.PHONY: test
test: envtest gotest

.PHONY: env
.SILENT: env
env: isclean
	echo OPERATOR_NAME=$(OPERATOR_NAME)
	echo OPERATOR_NAMESPACE=$(OPERATOR_NAMESPACE)
	echo OPERATOR_VERSION=$(OPERATOR_VERSION)
	echo OPERATOR_IMAGE_URI=$(OPERATOR_IMAGE_URI)
