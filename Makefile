TAG?=dev
REGISTRY?=ccr.ccs.tencentyun.com/test
IMAGE=$(REGISTRY)/karpenter-tke-controller
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
image: ## build and push the docker image
	docker build -t $(IMAGE):$(TAG) .
	docker push $(IMAGE):$(TAG)

.PHONY: release
release: build image ## release
