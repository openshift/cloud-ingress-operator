#!/bin/bash

set -e

cd $(dirname $0)/..

if [[ -z $IMAGE_REPOSITORY ]]; then
  IMAGE_REPOSITORY=app-sre
fi

# Build & push operator image and catalogsource image
make IMAGE_REPOSITORY=$IMAGE_REPOSITORY build skopeo-push build-catalog-image
