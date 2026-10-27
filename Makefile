GO ?= go
GO_MIN ?= go1.22.12
GOBIN ?= $(shell $(GO) env GOPATH)/bin
GOLANGCI_LINT_VERSION ?= v2.15.0

.PHONY: all
all: test build

.PHONY: build
build:
	$(GO) build ./...

.PHONY: test
test:
	gotestsum --junitfile test-results.xml --format testname -- -v -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'

# Runs the tests with the oldest supported Go version (downloaded on first use).
.PHONY: test-min
test-min:
	GOTOOLCHAIN=$(GO_MIN) $(GO) test -v ./...

.PHONY: lint
lint:
	$(GOBIN)/golangci-lint run ./...

.PHONY: fmt
fmt:
	$(GOBIN)/golangci-lint fmt ./...
	$(GOBIN)/golangci-lint run --fix ./...

.PHONY: install-dev
install-dev:
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(GOBIN) $(GOLANGCI_LINT_VERSION)
	$(GOBIN)/golangci-lint --version

.PHONY: clean
clean:
	$(GO) clean
	rm -f test-results.xml coverage.out
