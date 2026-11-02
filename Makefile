BINARY  := ps4pkgq
PKG     := ./cmd/ps4pkgq
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build build-windows test vet fmt lint tidy clean

all: vet test build

build:
	go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY) $(PKG)

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY).exe $(PKG)

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

clean:
	rm -rf dist
