#!/bin/bash -e

source `dirname $0`/common.sh

usage() { echo "Usage: $0 -o operator_name -c saas-repository-channel" 1>&2; exit 1; }

while getopts "o:c:" option; do
    case "${option}" in
        o)
            operator_name=${OPTARG}
            ;;
        c)
            operator_channel=${OPTARG}
            ;;
        *)
            usage
    esac
done

# Checking parameters
check_mandatory_params operator_channel operator_name

# Parameters for the Dockerfile
SAAS_OPERATOR_DIR="saas-${operator_name}-bundle"
BUNDLE_DIR="${SAAS_OPERATOR_DIR}/${operator_name}"
REGISTRY_IMG="quay.io/app-sre/${operator_name}-registry"
DOCKERFILE_REGISTRY="Dockerfile.olm-registry"
OPERATOR_NEW_VERSION=$(ls "${BUNDLE_DIR}" | sort -t . -k 3 -g | tail -n 1)

# Checking SAAS_OPERATOR_DIR exist
if [ ! -d "${SAAS_OPERATOR_DIR}/.git" ] ; then
    echo "${SAAS_OPERATOR_DIR} should exist and be a git repository"
    exit 1
fi

# Create package yaml
# This must be included in the registry build
# `currentCSV` must reference the latest bundle version included.
# Any version their after `currentCSV` loaded by the initalizer
# will be silently pruned as it's not reachable
PACKAGE_YAML_PATH="${BUNDLE_DIR}/${operator_name}.package.yaml"

cat <<EOF > "${PACKAGE_YAML_PATH}"
packageName: ${operator_name}
channels:
- name: ${operator_channel}
  currentCSV: ${operator_name}.v${OPERATOR_NEW_VERSION}
EOF

# Build registry
cat <<EOF > $DOCKERFILE_REGISTRY
FROM quay.io/openshift/origin-operator-registry:latest
COPY $SAAS_OPERATOR_DIR manifests
RUN initializer --permissive
CMD ["registry-server", "-t", "/tmp/terminate.log"]
EOF

docker build -f $DOCKERFILE_REGISTRY --tag "${REGISTRY_IMG}:${operator_channel}-latest" .

if [ $? -ne 0 ] ; then
    echo "docker build failed, exiting..."
    exit 1
fi

# TODO : Test the image and the version it contains
