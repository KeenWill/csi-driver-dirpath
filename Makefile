.PHONY: build test sanity e2e helm-lint manifests

HELM ?= helm

build:
	go build ./...

test:
	go test ./...

sanity:
	bash hack/sanity.sh

e2e:
	bash hack/e2e.sh

helm-lint:
	$(HELM) lint charts/csi-driver-dirpath

manifests:
	HELM=$(HELM) bash hack/render-manifests.sh
