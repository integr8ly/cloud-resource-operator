NAMESPACE=cloud-resource-operator
VERSION=0.1.0

.PHONY: build
build:
	GO111MODULE=on go build

.PHONY: run
run:
	operator-sdk up local --namespace=$(NAMESPACE)

.PHONY: code/gen
code/gen:
	operator-sdk generate k8s
	operator-sdk generate openapi
	@go generate ./...

.PHONY: code/fix
code/fix:
	@go mod tidy
	@gofmt -w `find . -type f -name '*.go' -not -path "./vendor/*"`

.PHONY: cluster/prepare
cluster/prepare:
	oc new-project $(NAMESPACE)
	oc create -f ./deploy/crds/*_crd.yaml

.PHONY: cluster/clean
cluster/clean:
	oc delete -f ./deploy/crds/*_crd.yaml
	oc delete project $(NAMESPACE)

.PHONY: test/unit
test/unit:
	@echo Running tests:
	go test -v -race -coverprofile=coverage.out ./pkg/...