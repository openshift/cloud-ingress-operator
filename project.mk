# Project specific values
OPERATOR_NAME?=$(shell sed -n 's/.*OperatorName .*"\([^"]*\)".*/\1/p' pkg/config/config.go)
OPERATOR_NAMESPACE?=$(shell sed -n 's/.*OperatorNamespace .*"\([^"]*\)".*/\1/p' pkg/config/config.go)

IMAGE_REGISTRY?=quay.io
IMAGE_REPOSITORY?=$(USER)
IMAGE_NAME?=$(OPERATOR_NAME)
CATALOG_REGISTRY_ORGANIZATION?=app-sre

VERSION_MAJOR?=0
VERSION_MINOR?=1

YAML_DIRECTORY?=deploy
SELECTOR_SYNC_SET_TEMPLATE_DIR?=hack/templates/
GIT_ROOT?=$(shell git rev-parse --show-toplevel 2>&1)

# WARNING: REPO_NAME will default to the current directory if there are no remotes
REPO_NAME?=$(shell basename $$((git config --get-regex remote\.*\.url 2>/dev/null | cut -d ' ' -f2 || pwd) | head -n1 | sed 's|.git||g'))

SELECTOR_SYNC_SET_DESTINATION?=${GIT_ROOT}/hack/olm-registry/olm-artifacts-template.yaml

IN_CONTAINER?=false

GEN_SYNCSET=hack/generate_template.py -t ${SELECTOR_SYNC_SET_TEMPLATE_DIR} -y ${YAML_DIRECTORY} -d ${SELECTOR_SYNC_SET_DESTINATION} -r ${REPO_NAME}
