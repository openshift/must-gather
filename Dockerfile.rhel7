FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.18-openshift-4.12 AS builder
WORKDIR /go/src/github.com/openshift/must-gather
COPY . .
ENV GO_PACKAGE github.com/openshift/must-gather

FROM registry.ci.openshift.org/ocp/4.12:cli
COPY --from=builder /go/src/github.com/openshift/must-gather/collection-scripts/* /usr/bin/
RUN yum install --setopt=tsflags=nodocs -y jq && yum clean all && rm -rf /var/cache/yum/*
