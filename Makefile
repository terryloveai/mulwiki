SHELL := /bin/bash

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
HOST_OS ?= $(shell cd server && go env GOOS)
HOST_ARCH ?= $(shell cd server && go env GOARCH)
DIST_DIR ?= dist
BUILD_DIR ?= .build
PACKAGE_NAME ?= mulwiki_$(VERSION)_$(HOST_OS)_$(HOST_ARCH)

CLI_LDFLAGS := -X 'main.version=$(VERSION)' -X 'main.commit=$(COMMIT)' -X 'main.date=$(BUILD_DATE)'

.PHONY: dev build build-go build-web package-current build-all clean test typecheck smoke-e2e

dev:
	@echo "Starting Mulwiki..."
	@echo "Go backend: http://localhost:8080"
	@echo "Web frontend: http://localhost:3000"
	@echo ""
	cd server && go run ./cmd/server &
	cd apps/web && pnpm dev

build: build-go build-web

build-go:
	mkdir -p "$(BUILD_DIR)/bin"
	cd server && go build -o "../$(BUILD_DIR)/bin/mulwiki-server" ./cmd/server
	cd server && go build -ldflags "$(CLI_LDFLAGS)" -o "../$(BUILD_DIR)/bin/mulwiki" ./cmd/mulwiki

build-web:
	pnpm --filter @mulwiki/web build
	mkdir -p apps/web/.next/standalone/apps/web/.next
	cp -R apps/web/.next/static apps/web/.next/standalone/apps/web/.next/static
	cp -R apps/web/public apps/web/.next/standalone/apps/web/public

package-current: build
	rm -rf "$(DIST_DIR)/$(PACKAGE_NAME)"
	mkdir -p "$(DIST_DIR)/$(PACKAGE_NAME)/bin"
	cp "$(BUILD_DIR)/bin/mulwiki-server" "$(DIST_DIR)/$(PACKAGE_NAME)/bin/"
	cp "$(BUILD_DIR)/bin/mulwiki" "$(DIST_DIR)/$(PACKAGE_NAME)/bin/"
	cp -R apps/web/.next/standalone "$(DIST_DIR)/$(PACKAGE_NAME)/web"
	mkdir -p "$(DIST_DIR)/$(PACKAGE_NAME)/server"
	cp -R server/builtin "$(DIST_DIR)/$(PACKAGE_NAME)/server/builtin"
	mkdir -p "$(DIST_DIR)/$(PACKAGE_NAME)/server/pkg"
	cp -R server/pkg/db "$(DIST_DIR)/$(PACKAGE_NAME)/server/pkg/db"
	cp .env.example "$(DIST_DIR)/$(PACKAGE_NAME)/.env.example"
	cp server/.env.example "$(DIST_DIR)/$(PACKAGE_NAME)/server/.env.example"
	cp README.md README.zh-CN.md "$(DIST_DIR)/$(PACKAGE_NAME)/"
	mkdir -p "$(DIST_DIR)/$(PACKAGE_NAME)/docs"
	cp docs/deployment.md "$(DIST_DIR)/$(PACKAGE_NAME)/docs/deployment.md"
	cd "$(DIST_DIR)" && tar -czf "$(PACKAGE_NAME).tar.gz" "$(PACKAGE_NAME)"
	@echo "Package: $(DIST_DIR)/$(PACKAGE_NAME).tar.gz"

build-all:
	@echo "Mulwiki uses go-sqlite3, so cross-compilation requires CGO and a target C compiler."
	@echo "Use package-current on the target OS, or set GOOS/GOARCH/CC explicitly and run:"
	@echo "  cd server && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=x86_64-linux-gnu-gcc go build -o bin/mulwiki-server-linux-amd64 ./cmd/server"
	@echo "  cd server && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=x86_64-linux-gnu-gcc go build -ldflags \"$(CLI_LDFLAGS)\" -o bin/mulwiki-linux-amd64 ./cmd/mulwiki"

test:
	cd server && go test ./...

typecheck:
	pnpm typecheck

smoke-e2e:
	pnpm smoke:e2e

clean:
	rm -rf "$(BUILD_DIR)"
	rm -rf apps/web/.next
	rm -rf "$(DIST_DIR)"
