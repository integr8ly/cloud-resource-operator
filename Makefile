IMAGE_REG=quay.io
IMAGE_ORG=integreatly
IMAGE_NAME=cloud-resource-operator
MANIFEST_NAME=cloud-resources
NAMESPACE=cloud-resource-operator
VERSION=0.1.0
COMPILE_TARGET=./tmp/_output/bin/$(IMAGE_NAME)

AUTH_TOKEN=$(shell curl -sH "Content-Type: application/json" -XPOST https://quay.io/cnr/api/v1/users/login -d '{"user": {"username": "$(QUAY_USERNAME)", "password": "${QUAY_PASSWORD}"}}' | jq -r '.token')

.PHONY: build
build:
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o=$(COMPILE_TARGET) cmd/manager/main.go

.PHONY: run
run:
	RECTIME=30 operator-sdk up local --namespace=""

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
	oc apply -f ./deploy/crds/integreatly_v1alpha1_redis_crd.yaml
	oc apply -f ./deploy/crds/integreatly_v1alpha1_postgres_crd.yaml
	oc apply -f ./deploy/examples/

.PHONY: cluster/seed/smtp
cluster/seed/smtp:
	oc apply -f ./deploy/crds/integreatly_v1alpha1_smtpcredentialset_cr.yaml -n $(NAMESPACE)

.PHONY: cluster/seed/blobstorage
cluster/seed/blobstorage:
	oc apply -f ./deploy/crds/integreatly_v1alpha1_blobstorage_cr.yaml -n $(NAMESPACE)

.PHONY: cluster/seed/redis
cluster/seed/redis:
	oc apply -f ./deploy/crds/integreatly_v1alpha1_redis_cr.yaml -n $(NAMESPACE)

.PHONY: cluster/seed/postgres
cluster/seed/postgres:
	oc apply -f ./deploy/crds/integreatly_v1alpha1_postgres_cr.yaml -n $(NAMESPACE)

.PHONY: cluster/seed/all
cluster/seed/all:
	oc apply -f ./deploy/crds/integreatly_v1alpha1_smtpcredentialset_cr.yaml -n $(NAMESPACE)
	oc apply -f ./deploy/crds/integreatly_v1alpha1_blobstorage_cr.yaml -n $(NAMESPACE)
	oc apply -f ./deploy/crds/integreatly_v1alpha1_redis_cr.yaml -n $(NAMESPACE)
	oc apply -f ./deploy/crds/integreatly_v1alpha1_postgres_cr.yaml -n $(NAMESPACE)

.PHONY: cluster/clean
cluster/clean:
	oc project $(NAMESPACE)
	oc delete -f ./deploy/crds/integreatly_v1alpha1_blobstorage_crd.yaml
	oc delete -f ./deploy/crds/integreatly_v1alpha1_smtpcredentialset_crd.yaml
	oc delete -f ./deploy/crds/integreatly_v1alpha1_redis_crd.yaml
	oc delete project $(NAMESPACE)

.PHONY: test/unit/setup
test/unit/setup:
	@echo Installing gotest
	go get -u github.com/rakyll/gotest

.PHONY: test/e2e
test/e2e:
	@echo Running e2e tests:
	operator-sdk test local ./test/e2e --go-test-flags "-v"


.PHONY: test/unit
test/unit:
	@echo Running tests:
	gotest -v -covermode=count -coverprofile=coverage.out ./pkg/...

.PHONY: test/unit/ci
test/unit/ci: test/unit
	@echo Removing mock file coverage
	sed -i.bak '/_moq.go/d' coverage.out

.PHONY: image/build
image/build: build
	operator-sdk build $(IMAGE_REG)/$(IMAGE_ORG)/$(IMAGE_NAME):$(VERSION)

.PHONY: image/push
image/push: image/build
	docker push $(IMAGE_REG)/$(IMAGE_ORG)/$(IMAGE_NAME):$(VERSION)

.PHONY: manifest/push
manifest/push:
	@operator-courier --verbose push deploy/olm-catalog/cloud-resources/ $(IMAGE_ORG) $(MANIFEST_NAME) $(VERSION) "$(AUTH_TOKEN)"

.PHONY: setup/travis
setup/travis:
	@curl -Lo operator-sdk https://github.com/operator-framework/operator-sdk/releases/download/v0.10.0/operator-sdk-v0.10.0-x86_64-linux-gnu && chmod +x operator-sdk && sudo mv operator-sdk /usr/local/bin/
	pip3 install operator-courier==2.1.7