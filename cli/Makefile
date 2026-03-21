.PHONY: build test vet clean install

BINARY := bonfire
BUILD_DIR := .

build:
	go build -o $(BUILD_DIR)/$(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BUILD_DIR)/$(BINARY)

install:
	go install .
