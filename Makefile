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

.PHONY: code/audit
code/audit:
	gosec ./...

.PHONY: cluster/prepare
cluster/prepare:
	oc new-project $(NAMESPACE) || true
	oc apply -f ./deploy/crds/integreatly_v1alpha1_blobstorage_crd.yaml
	oc apply -f ./deploy/crds/integreatly_v1alpha1_smtpcredentialset_crd.yaml
	oc apply -f ./deploy/examples/

.PHONY: cluster/seed/smtp
cluster/seed/smtp:
	oc apply -f ./deploy/crds/integreatly_v1alpha1_smtpcredentialset_cr.yaml -n $(NAMESPACE)

.PHONY: cluster/seed/blobstorage
cluster/seed/blobstorage:
	oc apply -f ./deploy/crds/integreatly_v1alpha1_blobstorage_cr.yaml -n $(NAMESPACE)

.PHONY: cluster/clean
cluster/clean:
	oc project $(NAMESPACE)
	oc delete -f ./deploy/crds/integreatly_v1alpha1_blobstorage_crd.yaml
	oc delete -f ./deploy/crds/integreatly_v1alpha1_smtpcredentialset_crd.yaml
	oc delete -f ./deploy/crds/integreatly_v1alpha1_redis_crd.yaml
	oc delete project $(NAMESPACE)

.PHONY: test/unit
test/unit:
	@echo Running tests:
	go test -v -covermode=count -coverprofile=coverage.out ./pkg/...

.PHONY: test/unit/ci
test/unit/ci: test/unit
	@echo Removing mock file coverage
	sed -i.bak '/_moq.go/d' coverage.out