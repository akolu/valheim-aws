.PHONY: build test vet clean install

BINARY := bonfire
ALIAS  := bf
BUILD_DIR := .
GOBIN  := $(shell go env GOPATH)/bin

build:
	go build -o $(BUILD_DIR)/$(BINARY) .
	ln -sf $(BINARY) $(BUILD_DIR)/$(ALIAS)

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BUILD_DIR)/$(BINARY) $(BUILD_DIR)/$(ALIAS)

install:
	go build -o $(GOBIN)/$(BINARY) .
	ln -sf $(GOBIN)/$(BINARY) $(GOBIN)/$(ALIAS)
