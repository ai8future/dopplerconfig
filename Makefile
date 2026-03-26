VERSION := $(shell cat VERSION)
BINARY := bin/dopplerconfig
LDFLAGS := -ldflags="-w -s -X main.version=$(VERSION)"

.DEFAULT_GOAL := build

.PHONY: build build-linux build-darwin build-all test clean lint deps run

build:
	@rm -f $(BINARY)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY) ./cmd/dopplerconfig

build-linux:
	@rm -f $(BINARY)-linux-amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-linux-amd64 ./cmd/dopplerconfig

build-darwin:
	@rm -f $(BINARY)-darwin-arm64
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-darwin-arm64 ./cmd/dopplerconfig

build-all: build-linux build-darwin
	cp scripts/launcher.sh $(BINARY)
	chmod +x $(BINARY)

test:
	go test -v -race ./...

clean:
	rm -rf bin/

lint:
	golangci-lint run ./...

deps:
	go mod download
	go mod tidy

run: build
	$(BINARY)
