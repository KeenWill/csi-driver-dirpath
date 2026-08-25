.PHONY: build test sanity e2e

build:
	go build ./...

test:
	go test ./...

sanity:
	bash hack/sanity.sh

e2e:
	bash hack/e2e.sh
