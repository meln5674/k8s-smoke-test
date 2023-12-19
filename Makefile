include build-env.Makefile

.PHONY: e2e
e2e: $(GINKGO) $(KIND) $(KUBECTL) $(HELM) vet
	LOCALBIN=$(LOCALBIN) $(GINKGO) run -vv .

mods:
	go mod download

.PHONY: e2e
vet:
	go vet ./...

