# Build the manager binary	
FROM golang:1.16.2 as builder	
		
WORKDIR /workspace	
# Copy the Go Modules manifests	
COPY go.mod go.mod	
COPY go.sum go.sum	
# cache deps before building and copying source so that we don't need to re-download as much	
# and so that source changes don't invalidate our downloaded layer	
RUN go mod download	
		
# Copy the go source	
COPY main.go main.go	
COPY apis/ apis/		
COPY controllers/ controllers/	
COPY pkg/ pkg/
COPY internal/ internal/	
COPY version/ version/		
COPY test/ test/
		
# Build	
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o cloud-resource-operator main.go

FROM registry.access.redhat.com/ubi7/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/cloud-resource-operator \
USER_UID=1001 \
USER_NAME=cloud-resource-operator

COPY --from=builder /workspace/cloud-resource-operator /usr/local/bin/cloud-resource-operator

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
