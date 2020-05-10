IMAGE_REG=quay.io
IMAGE_ORG=integreatly
IMAGE_NAME=cloud-resource-operator
MANIFEST_NAME=cloud-resources
NAMESPACE=cloud-resource-operator
PREV_VERSION=0.15.1
VERSION=0.15.2
COMPILE_TARGET=./tmp/_output/bin/$(IMAGE_NAME)

# If the _correct_ version of operator-sdk is on the path, use that (faster);
# otherwise use it through "go run" (slower but will always work and will use correct version)
OPERATOR_SDK_VERSION=0.12.0
ifeq ($(shell operator-sdk version 2> /dev/null | sed -e 's/", .*/"/' -e 's/.* //'), "v$(OPERATOR_SDK_VERSION)")
	OPERATOR_SDK ?= operator-sdk
else
	OPERATOR_SDK ?= go run github.com/operator-framework/operator-sdk/cmd/operator-sdk
endif

AUTH_TOKEN=$(shell curl -sH "Content-Type: application/json" -XPOST https://quay.io/cnr/api/v1/users/login -d '{"user": {"username": "$(QUAY_USERNAME)", "password": "${QUAY_PASSWORD}"}}' | jq -r '.token')

OS := $(shell uname)
ifeq ($(OS),Darwin)
	OPERATOR_SDK_OS := apple-darwin
else
	OPERATOR_SDK_OS := linux-gnu
endif

.PHONY: build
build:
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o=$(COMPILE_TARGET) cmd/manager/main.go

.PHONY: run
run:
	RECTIME=30 $(OPERATOR_SDK) up local --namespace=""

.PHONY: setup/service_account
setup/service_account:
	@oc replace --force -f deploy/role.yaml -n $(NAMESPACE)
	@oc replace --force -f deploy/cluster_role.yaml -n $(NAMESPACE)
	@oc replace --force -f deploy/service_account.yaml -n $(NAMESPACE)
	@oc replace --force -f deploy/role_binding.yaml -n $(NAMESPACE)
	@cat deploy/role_binding.yaml | sed "s/namespace: cloud-resource/namespace: $(NAMESPACE)/g" | oc replace --force -f -

.PHONY: code/run/service_account
code/run/service_account: setup/service_account
	@oc login --token=$(shell oc serviceaccounts get-token cloud-resource-operator -n ${NAMESPACE})
	@$(OPERATOR_SDK) up local --namespace=$(NAMESPACE)

operator-sdk-v0.10:
	@echo $(OPERATOR_SDK_OS)
	@curl -Lo operator-sdk-v0.10 https://github.com/operator-framework/operator-sdk/releases/download/v0.10.1/operator-sdk-v0.10.1-x86_64-$(OPERATOR_SDK_OS) && chmod +x operator-sdk-v0.10

.PHONY: code/gen
code/gen: operator-sdk-v0.10
	GOROOT=$(shell go env GOROOT) GO111MODULE="on" ./operator-sdk-v0.10 generate k8s
	GOROOT=$(shell go env GOROOT) GO111MODULE="on" ./operator-sdk-v0.10 generate openapi
	@go generate ./...

.PHONY: gen/csv
gen/csv:
	sed -i.bak 's/image:.*/image: quay\.io\/integreatly\/cloud-resource-operator:v$(VERSION)/g' deploy/operator.yaml && rm deploy/operator.yaml.bak
	@$(OPERATOR_SDK) olm-catalog gen-csv --operator-name=cloud-resources --csv-version $(VERSION) --from-version $(PREV_VERSION) --update-crds --csv-channel=integreatly --default-channel
	@sed -i.bak 's/$(PREV_VERSION)/$(VERSION)/g' deploy/olm-catalog/cloud-resources/cloud-resources.package.yaml && rm deploy/olm-catalog/cloud-resources/cloud-resources.package.yaml.bak
	@sed -i.bak s/cloud-resource-operator:v$(PREV_VERSION)/cloud-resource-operator:v$(VERSION)/g deploy/olm-catalog/cloud-resources/$(VERSION)/cloud-resources.v$(VERSION).clusterserviceversion.yaml && rm deploy/olm-catalog/cloud-resources/$(VERSION)/cloud-resources.v$(VERSION).clusterserviceversion.yaml.bak
.PHONY: code/fix
code/fix:
	@go mod tidy
	@gofmt -w `find . -type f -name '*.go' -not -path "./vendor/*"`

.PHONY: code/check
code/check:
	go fmt `go list ./... | grep -v /vendor/`
	golint ./pkg/...

.PHONY: code/audit
code/audit:
	gosec ./...

.PHONY: cluster/prepare
cluster/prepare:
	-oc new-project $(NAMESPACE) || true
	-oc label namespace $(NAMESPACE) monitoring-key=middleware
	oc apply -f ./deploy/crds/integreatly_v1alpha1_blobstorage_crd.yaml -n $(NAMESPACE)
	oc apply -f ./deploy/crds/integreatly_v1alpha1_redis_crd.yaml -n $(NAMESPACE)
	oc apply -f ./deploy/crds/integreatly_v1alpha1_postgres_crd.yaml -n $(NAMESPACE)
	oc apply -f ./deploy/crds/integreatly_v1alpha1_redissnapshot_crd.yaml -n $(NAMESPACE)
	oc apply -f ./deploy/crds/integreatly_v1alpha1_postgressnapshot_crd.yaml -n $(NAMESPACE)
	oc apply -f ./deploy/service_account.yaml -n $(NAMESPACE)
	oc apply -f ./deploy/role.yaml -n $(NAMESPACE)
	oc apply -f ./deploy/role_binding.yaml -n $(NAMESPACE)
	oc apply -f ./deploy/examples/ -n $(NAMESPACE)

.PHONY: cluster/seed/workshop/blobstorage
cluster/seed/workshop/blobstorage:
	@cat deploy/crds/integreatly_v1alpha1_blobstorage_cr.yaml | sed "s/type: REPLACE_ME/type: workshop/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/managed/blobstorage
cluster/seed/managed/blobstorage:
	@cat deploy/crds/integreatly_v1alpha1_blobstorage_cr.yaml | sed "s/type: REPLACE_ME/type: managed/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/workshop/redis
cluster/seed/workshop/redis:
	@cat deploy/crds/integreatly_v1alpha1_redis_cr.yaml | sed "s/type: REPLACE_ME/type: workshop/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/managed/redis
cluster/seed/managed/redis:
	@cat deploy/crds/integreatly_v1alpha1_redis_cr.yaml | sed "s/type: REPLACE_ME/type: managed/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/workshop/postgres
cluster/seed/workshop/postgres:
	@cat deploy/crds/integreatly_v1alpha1_postgres_cr.yaml | sed "s/type: REPLACE_ME/type: workshop/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/managed/postgres
cluster/seed/managed/postgres:
	@cat deploy/crds/integreatly_v1alpha1_postgres_cr.yaml | sed "s/type: REPLACE_ME/type: managed/g" | oc apply -f - -n $(NAMESPACE)


.PHONY: cluster/clean
cluster/clean:
	oc delete -f ./deploy/crds/integreatly_v1alpha1_blobstorage_crd.yaml -n $(NAMESPACE)
	oc delete -f ./deploy/crds/integreatly_v1alpha1_redis_crd.yaml -n $(NAMESPACE)
	oc delete -f ./deploy/crds/integreatly_v1alpha1_postgres_crd.yaml -n $(NAMESPACE)
	oc delete -f ./deploy/crds/integreatly_v1alpha1_redissnapshot_crd.yaml -n $(NAMESPACE)
	oc delete -f ./deploy/crds/integreatly_v1alpha1_postgressnapshot_crd.yaml -n $(NAMESPACE)
	oc delete -f ./deploy/service_account.yaml -n $(NAMESPACE)
	oc delete -f ./deploy/role.yaml -n $(NAMESPACE)
	oc delete -f ./deploy/role_binding.yaml -n $(NAMESPACE)
	oc delete -f ./deploy/examples/ -n $(NAMESPACE)
	oc delete project $(NAMESPACE)

.PHONY: test/unit/setup
test/unit/setup:
	@echo Installing gotest
	go get -u github.com/rakyll/gotest

.PHONY: test/e2e/prow
test/e2e/prow: cluster/prepare
	@echo Running e2e tests:
	GO111MODULE=on $(OPERATOR_SDK) test local ./test/e2e --up-local --namespace $(NAMESPACE) --go-test-flags "-timeout=60m -v"
	oc delete project $(NAMESPACE)

.PHONY: test/e2e/local
test/e2e/local: cluster/prepare
	@echo Running e2e tests:
	$(OPERATOR_SDK) test local ./test/e2e --up-local --namespace $(NAMESPACE) --go-test-flags "-timeout=60m -v"
	oc delete project $(NAMESPACE)

.PHONY: test/e2e/image
test/e2e/image:
	@echo Running e2e tests:
	$(OPERATOR_SDK) test local ./test/e2e --go-test-flags "-timeout=60m -v -parallel=2" --image $(IMAGE_REG)/$(IMAGE_ORG)/$(IMAGE_NAME):$(VERSION)

.PHONY: test/unit
test/unit:
	@echo Running tests:
	GO111MODULE=off go get -u github.com/rakyll/gotest
	gotest -v -covermode=count -coverprofile=coverage.out ./pkg/controller/... ./pkg/providers/... ./pkg/resources/... ./pkg/apis/integreatly/v1alpha1/types/...

.PHONY: test/unit/coverage
test/unit/coverage:
	@echo Running the coverage cli and html
	go tool cover -html=coverage.out
	go tool cover -func=coverage.out

.PHONY: test/unit/ci
test/unit/ci: test/unit
	@echo Removing mock file coverage
	sed -i.bak '/_moq.go/d' coverage.out

.PHONY: image/build
image/build: build
	$(OPERATOR_SDK) build $(IMAGE_REG)/$(IMAGE_ORG)/$(IMAGE_NAME):$(VERSION)

.PHONY: image/push
image/push: image/build
	docker push $(IMAGE_REG)/$(IMAGE_ORG)/$(IMAGE_NAME):$(VERSION)

.PHONY: vendor/check
vendor/check: vendor/fix
	git diff --exit-code vendor/

.PHONY: vendor/fix
vendor/fix:
	go mod tidy
	go mod vendor
