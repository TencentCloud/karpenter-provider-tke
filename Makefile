TAG?=dev
REGISTRY?=ccr.ccs.tencentyun.com/test
IMAGE=$(REGISTRY)/karpenter-tke-controller
E2E_IMAGE=$(REGISTRY)/karpenter-tke-e2e
E2E_SECRET?=e2e-env
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
CONTROLLER_GEN = go run ${PROJECT_DIR}/vendor/sigs.k8s.io/controller-tools/cmd/controller-gen

all: help

.PHONY: build
build: karpenter-tke-controller ## build all binaries

.PHONY: help
help: ## Display help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: gen-objects
gen-objects: ## generate the controller-gen related objects
	$(CONTROLLER_GEN) object paths="./..."

.PHONY: generate
generate: gen-objects manifests ## generate all controller-gen files

karpenter-tke-controller: ## build the main karpenter controller
	CGO_ENABLED=0 go build -ldflags "-X sigs.k8s.io/karpenter/pkg/operator.Version=$(TAG)" -o bin/karpenter-tke-controller cmd/controller/main.go

.PHONY: manifests
manifests: ## generate the controller-gen kubernetes manifests
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd paths="./pkg/apis/v1beta1" output:crd:artifacts:config=pkg/apis/crds
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd paths="./vendor/sigs.k8s.io/karpenter/..." output:crd:artifacts:config=pkg/apis/crds

.PHONY: vendor
vendor: ## update modules and populate local vendor directory
	go mod tidy
	go mod vendor
	go mod verify

.PHONY: image
image: karpenter-tke-controller ## build and push the docker image
	docker build -t $(IMAGE):$(TAG) .
	docker push $(IMAGE):$(TAG)

.PHONY: release
release: build image ## release

.PHONY: integration-test
integration-test: ## run integration tests
	@bash test/integration/run-test.sh

TEST_SUITE?=integration
FOCUS?=

.PHONY: e2etests
e2etests: ## run Go e2e tests (TEST_SUITE=integration|lifecycle, FOCUS=regex)
	go test -p 1 -count 1 -timeout 2h -v \
		./test/suites/$(shell echo $(TEST_SUITE) | tr A-Z a-z)/... \
		--ginkgo.focus="$(FOCUS)" \
		--ginkgo.timeout=1.5h \
		--ginkgo.grace-period=3m \
		--ginkgo.vv

.PHONY: test
test: ## run unit tests with coverage report
	go test -coverprofile=coverage.out -covermode=atomic ./pkg/... || true
	@[ -f coverage.out ] && go tool cover -func=coverage.out || true

.PHONY: e2e-image
e2e-image: ## build and push e2e test image
	docker build -f Dockerfile.e2e -t $(E2E_IMAGE):$(TAG) .
	docker push $(E2E_IMAGE):$(TAG)

.PHONY: e2e-deploy
e2e-deploy: ## deploy e2e test job via helm (E2E_SECRET=<secret-name>)
	helm upgrade --install karpenter-e2e charts/karpenter-e2e \
		--namespace karpenter \
		--set image.repository=$(E2E_IMAGE) \
		--set image.tag=$(TAG) \
		--set envSecret.name=$(E2E_SECRET)

.PHONY: e2e-logs
e2e-logs: ## stream logs from running e2e job
	kubectl logs -n karpenter -l app.kubernetes.io/name=karpenter-e2e --follow

.PHONY: e2e-report
e2e-report: ## print markdown e2e test report from ConfigMap
	@kubectl get configmap -n karpenter e2e-test-report \
		-o jsonpath='{.data.report\.md}' 2>/dev/null \
		|| echo "(report not ready yet)"

