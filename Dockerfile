FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.15-openshift-4.8 AS builder
WORKDIR /go/src/github.com/openshift/must-gather
COPY . .
ENV GO_PACKAGE github.com/openshift/must-gather

FROM registry.ci.openshift.org/ocp/4.8:cli
COPY --from=builder /go/src/github.com/openshift/must-gather/collection-scripts/* /usr/bin/

