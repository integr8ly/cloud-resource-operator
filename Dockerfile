# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.25.9 AS builder

# this is required for podman
USER root

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY pkg/ pkg/
COPY internal/ internal/
COPY version/ version/
COPY test/ test/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -mod=vendor -a -tags netgo -ldflags="-w -s -extldflags '-static'" -o cloud-resource-operator ./cmd/main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/cloud-resource-operator \
USER_UID=1001 \
USER_NAME=cloud-resource-operator

COPY --from=builder /workspace/cloud-resource-operator /usr/local/bin/cloud-resource-operator

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
