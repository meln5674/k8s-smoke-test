.PHONY: build-tools
build-tools: go-build-tools

.PHONY: go-build-tools
go-build-tools: ginkgo kind kubectl helm

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

GINKGO_VERSION ?= $(shell go mod edit -print | grep ginkgo | cut -d ' ' -f2)
GINKGO ?= $(LOCALBIN)/ginkgo
$(GINKGO):
	mkdir -p $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)
ginkgo: $(GINKGO)

KIND_VERSION ?= v0.17.0
KIND ?= $(LOCALBIN)/kind
$(KIND):
	mkdir -p $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/kind@$(KIND_VERSION)
.PHONY: kind
kind: $(KIND)

KUBECTL ?= $(LOCALBIN)/kubectl
KUBECTL_MIRROR ?= https://dl.k8s.io/release
KUBECTL_VERSION ?= v1.25.11
KUBECTL_URL ?= "$(KUBECTL_MIRROR)/$(KUBECTL_VERSION)/bin/$(shell go env GOOS)/$(shell go env GOARCH)/kubectl"
$(KUBECTL):
	mkdir -p $(LOCALBIN)
	curl -vfL $(KUBECTL_URL) > $(KUBECTL)
	chmod +x $(KUBECTL)
	touch $(KUBECTL)
.PHONY: kubectl
kubectl: $(KUBECTL)

HELM_VERSION ?= v3.11.2
HELM ?= $(LOCALBIN)/helm
$(HELM):
	mkdir -p $(LOCALBIN)
	mkdir -p $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install helm.sh/helm/v3/cmd/helm@$(HELM_VERSION)
.PHONY: helm
helm: $(HELM)
