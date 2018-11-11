BINARY=vultr-cloudinit
PACKAGES=$(shell glide novendor)
SOURCE_FILES=$(shell find . -name '*.go' -not -path '*vendor*')
VERSION=dev

.PHONY: all build check clean coverage fmt help lint test vet

all: check build test ## run fmt, vet, lint, build the binaries and run the tests

check: fmt vet lint ## run fmt, vet, lint

vet: ## run go vet
	@echo "Running $@"
	@test -z "$$(go vet ${PACKAGES} 2>&1 | tee /dev/stderr)"

fmt: ## run go fmt
	@echo "Running $@"
	@gofmt -s -l -w ${SOURCE_FILES}

build: ## build the go packages
	@echo "Running $@"
	@go build -ldflags "-X main.Version=${VERSION}" -o bin/${BINARY} .
	@mv bin/${BINARY} ${GOPATH}/bin/${BINARY}-dev

build-linux: ## build the go packages for Linux
	@echo "Running $@"
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.Version=${VERSION}" -o bin/${BINARY}_linux_amd64 .

build-osx: ## build the go packages for OSX
	@echo "Running $@"
	@GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.Version=${VERSION}" -o bin/${BINARY}_darwin_amd64 .

build-windows: ## build the go packages for Windows
	@echo "Running $@"
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.Version=${VERSION}" -o bin/${BINARY}_windows_amd64.exe .


test: ## run test
	@echo "Running $@"
	@go test ${PACKAGES}

coverage: ## run tests with coverage metrics
	@echo "Running $@"
	@go test -cover ${PACKAGES}

clean: ## clean up binaries
	@echo "Running $@"
	@rm -rf bin

help: ## this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST) | sort