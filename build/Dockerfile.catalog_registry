FROM quay.io/openshift/origin-operator-registry:latest

ARG SRC_BUNDLES

COPY ${SRC_BUNDLES} manifests
RUN initializer --permissive

CMD ["registry-server", "-t", "/tmp/terminate.log"]