FROM registry.svc.ci.openshift.org/openshift/release:golang-1.10 AS builder
WORKDIR /go/src/github.com/openshift/must-gather
COPY . .
ENV GO_PACKAGE github.com/openshift/must-gather
RUN go build -ldflags "-X $GO_PACKAGE/pkg/version.versionFromGit=$(git describe --long --tags --abbrev=7 --match 'v[0-9]*')" ./cmd/openshift-must-gather

FROM registry.svc.ci.openshift.org/openshift/origin-v4.0:cli
COPY --from=builder /go/src/github.com/openshift/must-gather/openshift-must-gather /usr/bin/
COPY --from=builder /go/src/github.com/openshift/must-gather/collection-scripts/* /usr/bin/

