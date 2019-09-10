NAMESPACE=cloud-resource-operator
VERSION=0.1.0

.PHONY: build
build:
	go build cmd/manager/main.go

.PHONY: run
run:
	operator-sdk up local --namespace=""

.PHONY: code/gen
code/gen:
	operator-sdk generate k8s
	operator-sdk generate openapi
	@go generate ./...

.PHONY: code/fix
code/fix:
	@go mod tidy
	@gofmt -w `find . -type f -name '*.go' -not -path "./vendor/*"`

.PHONY: code/check
code/check:
	bash -c "diff -u <(echo -n) <(gofmt -d ./)"

.PHONY: cluster/prepare
cluster/prepare:
	oc new-project $(NAMESPACE) || true
	oc apply -f ./deploy/crds/integreatly_v1alpha1_blobstorage_crd.yaml
	oc apply -f ./deploy/crds/integreatly_v1alpha1_redis_crd.yaml
	oc apply -f ./deploy/examples/
	oc apply -f ./deploy/crds/*cr.yaml -n $(NAMESPACE)

.PHONY: cluster/clean
cluster/clean:
	oc delete -f ./deploy/crds/*_crd.yaml
	oc delete project $(NAMESPACE)

.PHONY: test/unit
test/unit:
	@echo Running tests:
	go test -v -covermode=count -coverprofile=coverage.out ./pkg/...