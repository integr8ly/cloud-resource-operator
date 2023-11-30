IMAGE_REG ?= quay.io
IMAGE_ORG ?= integreatly
IMAGE_NAME ?= cloud-resource-operator
OPERATOR_IMG = $(IMAGE_REG)/$(IMAGE_ORG)/$(IMAGE_NAME):v$(VERSION)
CONTAINER_PLATFORM ?= linux/amd64
MANIFEST_NAME ?= cloud-resources
NAMESPACE=cloud-resource-operator
PREV_VERSION=1.1.1
VERSION=1.1.2
COMPILE_TARGET=./tmp/_output/bin/$(IMAGE_NAME)
UPGRADE ?= true
CHANNEL ?= rhmi
REDIS_NODE_SIZE ?= ""
REDIS_NAME ?= example-redis
# openshift/aws/gcp
PROVIDER ?= openshift
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
GOLANGCI_LINT_VERSION=v1.50.0

SHELL=/bin/bash

# If the _correct_ version of operator-sdk is on the path, use that (faster);
# otherwise use it through "go run" (slower but will always work and will use correct version)
OPERATOR_SDK_VERSION=1.14.0
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

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: build
build: code/gen
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o=$(COMPILE_TARGET) ./main.go

.PHONY: run
run:
	RECTIME=30 WATCH_NAMESPACE=$(NAMESPACE) go run ./main.go

.PHONY: setup/service_account
setup/service_account: kustomize
	@-oc new-project $(NAMESPACE)
	@oc project $(NAMESPACE)
	@-oc create -f config/rbac/service_account.yaml -n $(NAMESPACE)
	@$(KUSTOMIZE) build config/rbac | oc replace --force -f -	

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT)
$(GOLANGCI_LINT): $(LOCALBIN)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(LOCALBIN) $(GOLANGCI_LINT_VERSION)

.PHONY: code/run/service_account
code/run/service_account: setup/service_account
	@oc login --token=$(shell oc create token cloud-resource-operator -n ${NAMESPACE} --duration=24h) --server=$(shell sh -c "oc cluster-info | grep -Eo 'https?://[-a-zA-Z0-9\.:]*'") --kubeconfig=TMP_SA_KUBECONFIG --insecure-skip-tls-verify=true
	WATCH_NAMESPACE=$(NAMESPACE) go run ./main.go

.PHONY: code/gen
code/gen: manifests kustomize generate

# Make sure that the previous version and version values are set to correct values.
.PHONY: gen/csv
gen/csv: kustomize
	@SEMVER=$(VERSION) PREV_VERSION=$(PREV_VERSION) ./scripts/gen-csv.sh

.PHONY: code/fix
code/fix:
	@go mod tidy
	@gofmt -w `find . -type f -name '*.go' -not -path "./vendor/*"`

.PHONY: code/check
code/check:
	@diff -u <(echo -n) <(gofmt -d `find . -type f -name '*.go' -not -path "./vendor/*"`)

.PHONY: vendor/fix
vendor/fix:
	go mod tidy
	go mod vendor

.PHONY: vendor/check
vendor/check: vendor/fix
	git diff --exit-code vendor/

.PHONY: cluster/prepare
cluster/prepare: kustomize setup/service_account
	-oc new-project $(NAMESPACE) || true
	-oc label namespace $(NAMESPACE) monitoring-key=middleware
	-oc apply -f ./config/samples/cloud_resource_config.yaml -n $(NAMESPACE)
	-oc apply -f ./config/samples/cloud_resources_$(PROVIDER)_strategies.yaml -n $(NAMESPACE)
	$(KUSTOMIZE) build config/crd | oc apply -f -

.PHONY: cluster/seed/blobstorage
cluster/seed/blobstorage:
	@cat config/samples/integreatly_v1alpha1_blobstorage.yaml | sed "s/type: REPLACE_ME/type: $(PROVIDER)/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/redis
cluster/seed/redis:
	@cat config/samples/integreatly_v1alpha1_redis.yaml | sed "s/name: REPLACE_ME/name: $(REDIS_NAME)/g" | sed "s/type: REPLACE_ME/type: $(PROVIDER)/g" | sed "s/size: REPLACE_ME/size: '$(REDIS_NODE_SIZE)'/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/redissnapshot
cluster/seed/redissnapshot:
	@cat config/samples/integreatly_v1alpha1_redissnapshot.yaml | sed "s/resourceName: REPLACE_ME/resourceName: example-redis/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/postgres
cluster/seed/postgres:
	@cat config/samples/integreatly_v1alpha1_postgres.yaml | sed "s/type: REPLACE_ME/type: $(PROVIDER)/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/postgressnapshot
cluster/seed/postgressnapshot:
	@cat config/samples/integreatly_v1alpha1_postgressnapshot.yaml | sed "s/resourceName: REPLACE_ME/resourceName: example-postgres/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/clean
cluster/clean:
	@$(KUSTOMIZE) build config/crd | oc delete -f -
	@$(KUSTOMIZE) build config/rbac | oc delete --force -f -	
	oc delete project $(NAMESPACE)

.PHONY: test/unit
test/unit:
	@echo Running tests:
	go install github.com/rakyll/gotest@v0.0.6
	gotest -v -covermode=count -coverprofile=coverage.out ./pkg/providers/... ./pkg/resources/... ./apis/integreatly/v1alpha1/types/... ./pkg/client/...

.PHONY: image/build
image/build: build
	echo "build image ${OPERATOR_IMG}"
	docker build --platform=$(CONTAINER_PLATFORM) -t ${OPERATOR_IMG} .

.PHONY: image/push
image/push: image/build
	docker push ${OPERATOR_IMG}

.PHONY: test/e2e/prow
test/e2e/prow: export component := cloud-resource-operator
test/e2e/prow: export OPERATOR_IMAGE := ${IMAGE_FORMAT}
test/e2e/prow: cluster/prepare cluster/deploy
	@echo Running e2e tests:
	go clean -testcache && go test -v ./test/e2e -timeout=120m -ginkgo.v
	oc delete project $(NAMESPACE)

.PHONY: test/e2e/local 
test/e2e/local: cluster/prepare
	@echo Running e2e tests:
	go clean -testcache && go test -v ./test/e2e -timeout=120m -ginkgo.v
	oc delete project $(NAMESPACE)

.PHONY: test/lint
test/lint: golangci-lint
	@$(GOLANGCI_LINT) run

.PHONY: cluster/deploy
cluster/deploy: kustomize
	@echo Deploying operator with image: ${OPERATOR_IMAGE}
	@ - cd config/manager && $(KUSTOMIZE) edit set image controller=${OPERATOR_IMAGE}
	@ - $(KUSTOMIZE) build config/cloud-resource-operator | oc apply -f -

.PHONY: test/e2e/image
test/e2e/image:
	@echo Running e2e tests:
	$(OPERATOR_SDK) test local ./test/e2e --go-test-flags "-timeout=60m -v -parallel=2" --image $(IMAGE_REG)/$(IMAGE_ORG)/$(IMAGE_NAME):$(VERSION)

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) "crd:crdVersions=v1" rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.9.2 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

kustomize:
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go install sigs.k8s.io/kustomize/kustomize/v4@v4.5.5 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

.PHONY: code/audit
code/audit:
	gosec ./...

.PHONY: code/gen
code/gen: setup/moq vendor/fix apis/integreatly/v1alpha1/zz_generated.deepcopy.go
	$(CONTROLLER_GEN) rbac:roleName=manager-role webhook paths="./..."
	@go generate ./...

.PHONY: setup/moq
setup/moq:
	go install github.com/matryer/moq@v0.3.0

.PHONY: create/olm/bundle
create/olm/bundle:
	@PREV_VERSION=$(PREV_VERSION) ./scripts/create-olm-bundle.sh

.PHONY: release/prepare
release/prepare: gen/csv image/push create/olm/bundle

# Commands added to support cro release pipeline, please remove once we move to CPaaS CRO builds
.PHONY: image/build/pipelines
image/build/pipelines: build
	echo "build image ${OPERATOR_IMG}"
	sudo podman build --platform=$(CONTAINER_PLATFORM) --ulimit nofile=65535:65535 . -t ${OPERATOR_IMG}
	sudo podman save ${OPERATOR_IMG} | sudo -u jenkins podman load

.PHONY: image/push/pipelines
image/push/pipelines: image/build/pipelines
	echo "pushing image ${OPERATOR_IMG}"
	podman push ${OPERATOR_IMG}

.PHONY: verify/release/exist
verify/release/exist:
	IMAGE_TO_SCAN=${OPERATOR_IMG} ./scripts/imageExists.sh

.PHONY: coverage
coverage:
	hack/codecov.sh

.PHONY: setup/sts
setup/sts:
	NAMESPACE=$(NAMESPACE) ./scripts/sts/create-rhoam-policy.sh

.PHONY: gosec
gosec:
	gosec -exclude-dir=hack/redis ./...