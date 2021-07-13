IMAGE_REG ?= quay.io
IMAGE_ORG ?= integreatly
IMAGE_NAME ?= cloud-resource-operator
OPERATOR_IMG = $(IMAGE_REG)/$(IMAGE_ORG)/$(IMAGE_NAME):v$(VERSION)
MANIFEST_NAME ?= cloud-resources
NAMESPACE=cloud-resource-operator
PREV_VERSION=0.29.0
VERSION=0.30.0
COMPILE_TARGET=./tmp/_output/bin/$(IMAGE_NAME)
UPGRADE ?= true
CHANNEL ?= rhmi

PREVIOUS_OPERATOR_VERSIONS="0.29.0,0.28.0,0.27.1,0.27.0,0.26.0,0.25.0,0.24.1,0.24.0,0.23.0"

SHELL=/bin/bash

# If the _correct_ version of operator-sdk is on the path, use that (faster);
# otherwise use it through "go run" (slower but will always work and will use correct version)
OPERATOR_SDK_VERSION=1.2.0
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

.PHONY: code/run/service_account
code/run/service_account: setup/service_account
	@oc login --token=$(shell oc serviceaccounts get-token cloud-resource-operator -n ${NAMESPACE}) --server=$(shell sh -c "oc cluster-info | grep -Eo 'https?://[-a-zA-Z0-9\.:]*'") --kubeconfig=TMP_SA_KUBECONFIG --insecure-skip-tls-verify=true
	WATCH_NAMESPACE=$(NAMESPACE) go run ./main.go

.PHONY: code/gen
code/gen: manifests kustomize generate

# Make sure that the previous version and version values are set to correct values.
.PHONY: gen/csv
gen/csv:
	@$(KUSTOMIZE) build config/manifests | operator-sdk generate packagemanifests --kustomize-dir=config/manifests --output-dir packagemanifests/ --version ${VERSION} --default-channel --channel integreatly
	@sed -i "s/Version = \"${PREV_VERSION}\"/Version = \"${VERSION}\"/g" version/version.go
	@yq w -i "packagemanifests/${VERSION}/cloud-resource-operator.clusterserviceversion.yaml" metadata.annotations.containerImage ${OPERATOR_IMG}
	@yq w -i "packagemanifests/${VERSION}/cloud-resource-operator.clusterserviceversion.yaml" metadata.name cloud-resources.v$(VERSION)
	@yq w -i "packagemanifests/${VERSION}/cloud-resource-operator.clusterserviceversion.yaml" spec.install.spec.deployments[0].spec.template.spec.containers[0].image ${OPERATOR_IMG}
	@yq w -i config/manifests/bases/cloud-resource-operator.clusterserviceversion.yaml metadata.name cloud-resources.v${VERSION}
	@yq w -i config/manifests/bases/cloud-resource-operator.clusterserviceversion.yaml metadata.annotations.containerImage ${OPERATOR_IMG}
	@yq w -i config/manifests/bases/cloud-resource-operator.clusterserviceversion.yaml spec.version ${VERSION}

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
	-oc apply -f ./config/samples/cloud_resource_openshift_strategies.yaml -n $(NAMESPACE)
	-oc apply -f ./config/samples/cloud_resources_aws_strategies.yaml -n $(NAMESPACE)
	$(KUSTOMIZE) build config/crd | oc apply -f -

.PHONY: cluster/seed/workshop/blobstorage
cluster/seed/workshop/blobstorage:
	@cat config/samples/integreatly_v1alpha1_blobstorage.yaml | sed "s/type: REPLACE_ME/type: workshop/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/managed/blobstorage
cluster/seed/managed/blobstorage:
	@cat config/samples/integreatly_v1alpha1_blobstorage.yaml | sed "s/type: REPLACE_ME/type: managed/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/workshop/redis
cluster/seed/workshop/redis:
	@cat config/samples/integreatly_v1alpha1_redis.yaml | sed "s/type: REPLACE_ME/type: workshop/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/managed/redis
cluster/seed/managed/redis:
	@cat config/samples/integreatly_v1alpha1_redis.yaml | sed "s/type: REPLACE_ME/type: managed/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/workshop/postgres
cluster/seed/workshop/postgres:
	@cat config/samples/integreatly_v1alpha1_postgres.yaml | sed "s/type: REPLACE_ME/type: workshop/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/seed/managed/postgres
cluster/seed/managed/postgres:
	@cat config/samples/integreatly_v1alpha1_postgres.yaml | sed "s/type: REPLACE_ME/type: managed/g" | oc apply -f - -n $(NAMESPACE)

.PHONY: cluster/clean
cluster/clean:
	@$(KUSTOMIZE) build config/crd | kubectl delete -f -
	@$(KUSTOMIZE) build config/rbac | oc delete --force -f -	
	oc delete project $(NAMESPACE)

.PHONY: test/unit/setup
test/unit/setup:
	@echo Installing gotest
	go get -u github.com/rakyll/gotest

.PHONY: test/unit
test/unit:
	@echo Running tests:
	GO111MODULE=off go get -u github.com/rakyll/gotest
	gotest -v -covermode=count -coverprofile=coverage.out ./pkg/providers/... ./pkg/resources/... ./apis/integreatly/v1alpha1/types/... ./pkg/client/...

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
	echo "build image ${OPERATOR_IMG}"
	docker build . -t ${OPERATOR_IMG}

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
	$(CONTROLLER_GEN) "crd:crdVersions=v1,trivialVersions=false" rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

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
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ;\
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
	go get sigs.k8s.io/kustomize/kustomize/v3@v3.5.4 ;\
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
code/gen: setup/moq apis/integreatly/v1alpha1/zz_generated.deepcopy.go apis/config/v1/zz_generated.deepcopy.go
	$(CONTROLLER_GEN) rbac:roleName=manager-role webhook paths="./..."
	@go generate ./...

.PHONY: setup/moq
setup/moq:
	go get github.com/matryer/moq
	go mod vendor

.PHONY: create/olm/bundle
create/olm/bundle:
	IMAGE_ORG=${IMAGE_ORG} IMAGE_REG=${IMAGE_REG} CHANNEL=${CHANNEL} UPGRADE=${UPGRADE} PREVIOUS_OPERATOR_VERSIONS=${PREVIOUS_OPERATOR_VERSIONS} ./scripts/bundle-rhmi-operators.sh

.PHONY: release/prepare
release/prepare: gen/csv image/push create/olm/bundle