.PHONY: build test sanity

build:
	go build ./...

test:
	go test ./...

sanity:
	bash hack/sanity.sh
