VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/amor7/gozarDNS/internal/buildinfo.Version=$(VERSION)

.PHONY: all web build release test dev clean

all: web build

web:
	cd web && npm ci && npm run build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/gozar ./cmd/gozar

# Static binaries for the two server architectures, plus checksums.
release: web
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/gozar-linux-amd64 ./cmd/gozar
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/gozar-linux-arm64 ./cmd/gozar
	cd dist && sha256sum gozar-linux-* > checksums.txt

test:
	go vet ./...
	go test ./...
	cd web && npm run typecheck

# A hub and a gate on this machine with demo data. See tools/devstack.
dev:
	go run ./tools/devstack

clean:
	rm -rf bin dist .devstack
