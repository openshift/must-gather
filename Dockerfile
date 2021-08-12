FROM registry.ci.openshift.org/openshift/release:golang-1.16 AS builder
WORKDIR /go/src/github.com/openshift/must-gather
COPY . .
ENV GO_PACKAGE github.com/openshift/must-gather

FROM quay.io/openshift/origin-cli:4.8
COPY --from=builder /go/src/github.com/openshift/must-gather/collection-scripts/* /usr/bin/

