FROM quay.io/app-sre/boilerplate:image-v3.0.6 AS builder

RUN mkdir -p /workdir
WORKDIR /workdir
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make go-build

####
FROM registry.access.redhat.com/ubi8/ubi-minimal:8.8-1037

ENV USER_UID=1001 \
    USER_NAME=cloud-ingress-operator

COPY --from=builder /workdir/build/_output/bin/* /usr/local/bin/

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}

LABEL io.openshift.managed.name="cloud-ingress-operator" \
      io.openshift.managed.description="Operator to manage cloud ingress."
