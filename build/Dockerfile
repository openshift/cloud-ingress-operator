FROM registry.svc.ci.openshift.org/openshift/release:golang-1.13 AS builder

RUN mkdir -p /workdir
WORKDIR /workdir
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make gobuild

####
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV USER_UID=1001 \
    USER_NAME=cloud-ingress-operator

COPY --from=builder /workdir/build/_output/bin/* /usr/local/bin/

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}

LABEL io.openshift.managed.name="cloud-ingress-operator" \
      io.openshift.managed.description="Operator to manage cloud ingress."
