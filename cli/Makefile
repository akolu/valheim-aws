.PHONY: build test vet clean install

BINARY := bonfire
ALIAS  := bf
BUILD_DIR := .
GOBIN  := $(shell go env GOPATH)/bin
VERSION := $(shell git rev-parse HEAD 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X github.com/bonfire/cli/cmd.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) .
	ln -sf $(BINARY) $(BUILD_DIR)/$(ALIAS)

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BUILD_DIR)/$(BINARY) $(BUILD_DIR)/$(ALIAS)

install:
	go build $(LDFLAGS) -o $(GOBIN)/$(BINARY) .
	ln -sf $(GOBIN)/$(BINARY) $(GOBIN)/$(ALIAS)
