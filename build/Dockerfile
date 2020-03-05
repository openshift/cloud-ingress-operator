FROM registry.svc.ci.openshift.org/openshift/release:golang-1.13 AS builder

# Allow specifying a GOPROXY cache during build to speed up dependency resolution
ARG GOPROXY=https://proxy.golang.org

ENV OPERATOR_PATH=/go/src/github.com/openshift/cloud-ingress-operator \
    GO111MODULE=on \
    GOPROXY=$GOPROXY

RUN mkdir -p ${OPERATOR_PATH}
WORKDIR ${OPERATOR_PATH}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make gobuild

####
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV OPERATOR_PATH=/go/src/github.com/openshift/cloud-ingress-operator \
    OPERATOR_BIN=cloud-ingress-operator \
    USER_UID=1001 \
    USER_NAME=cloud-ingress-operator

# install operator binary
COPY --from=builder ${OPERATOR_PATH}/build/_output/bin/${OPERATOR_BIN} /usr/local/bin/${OPERATOR_BIN}


COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}

LABEL io.openshift.managed.name="cloud-ingress-operator" \
      io.openshift.managed.description="Operator to manage cloud ingress."
