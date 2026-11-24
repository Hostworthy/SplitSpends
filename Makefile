.PHONY: help setup dev api web check test test-integration lint vuln fmt hooks brand build docker web-build apk lambda

.DEFAULT_GOAL := help
TEST_DATABASE_URL ?= postgres://splitspends:splitspends@localhost:5432/splitspends_test?sslmode=disable
DART_MEMBERS := packages/ledger packages/client apps/apk apps/ios apps/web
# `go run pkg@version` picks the toolchain from the tool's own go.mod, and a
# tool built with an older Go refuses this module, so pin go.mod's version.
# CI builds the same golangci-lint version (.github/workflows/ci.yml).
GO_VERSION := $(shell sed -n 's/^go //p' go.mod)
# Stamped into the binaries (`splitspends version`, the MCP server info).
# The release workflow passes the tag; the Dockerfile uses the same flags.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -s -w -X main.version=$(VERSION)
# Output directory and executable suffix (.exe when GOOS=windows).
BIN ?= bin
EXE ?= $(shell go env GOEXE)
GOLANGCI ?= GOTOOLCHAIN=go$(GO_VERSION) go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0
GOVULNCHECK ?= GOTOOLCHAIN=go$(GO_VERSION) go run golang.org/x/vuln/cmd/govulncheck@v1.10.0

help:
	@echo "Targets:"
	@echo "  setup             Copy env files, start Postgres, fetch Go and Dart dependencies"
	@echo "  dev               Run the API (:8080) and the web app (:5173)"
	@echo "  api               Run only the API (apps/api)"
	@echo "  web               Run only the web app (apps/web) against the local API"
	@echo "  check             lint, vuln and test"
	@echo "  lint              gofmt, go vet, golangci-lint, dart format, flutter analyze, fonts"
	@echo "  vuln              Known vulnerabilities in Go code and the standard library"
	@echo "  fmt               Format Go and Dart and apply Dart fixes"
	@echo "  hooks             Run quick format checks before every commit"
	@echo "  brand             Render icons, splash images and banners"
	@echo "  test-integration  Database tests (needs TEST_DATABASE_URL)"
	@echo "  build             Build the API and MCP binaries into bin/"
	@echo "  web-build         Build the web app into apps/web/build/web"
	@echo "  apk               Build the Android app into apps/apk/build"
	@echo "  docker            Build the production image"
	@echo "  lambda            Build the arm64 Lambda bundle for deploy/aws"

setup:
	cp -n apps/api/.env.example apps/api/.env 2>/dev/null || true
	docker compose up -d --wait
	docker compose exec -T postgres psql -U splitspends -tc "SELECT 1 FROM pg_database WHERE datname='splitspends_test'" | grep -q 1 \
		|| docker compose exec -T postgres createdb -U splitspends splitspends_test
	go mod download
	flutter pub get

api:
	set -a && . apps/api/.env && set +a && go run ./apps/api

web:
	cd apps/web && flutter run -d web-server --web-port 5173 --dart-define=SPLITSPENDS_URL=http://localhost:8080

dev:
	$(MAKE) -j2 api web

fmt:
	gofmt -w apps packages
	for d in $(DART_MEMBERS); do (cd $$d && dart fix --apply >/dev/null) || exit 1; done
	dart format $(DART_MEMBERS)

hooks:
	git config core.hooksPath scripts/hooks
	@echo "Pre-commit checks enabled. Disable with: git config --unset core.hooksPath"

brand:
	python3 scripts/brand/render.py

lint:
	@test -z "$$(gofmt -l apps packages)" || (echo "gofmt needed:"; gofmt -l apps packages; exit 1)
	go vet ./...
	$(GOLANGCI) run ./...
	dart format --output=none --set-exit-if-changed $(DART_MEMBERS)
	for d in $(DART_MEMBERS); do (cd $$d && flutter analyze) || exit 1; done
	scripts/fonts/fallback.sh check

vuln:
	$(GOVULNCHECK) ./...

test:
	go test -count=1 ./...
	cd packages/ledger && dart test
	cd packages/client && flutter test

test-integration:
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -count=1 ./...

check: lint vuln test

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o "$(BIN)/splitspends$(EXE)" ./apps/api
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o "$(BIN)/splitspends-mcp$(EXE)" ./apps/mcp

web-build:
	cd apps/web && flutter build web --release --no-web-resources-cdn

apk:
	cd apps/apk && flutter build apk --release

docker:
	docker build --build-arg VERSION="$(VERSION)" -t splitspends:latest .

lambda:
	mkdir -p deploy/aws/build/lambda
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o deploy/aws/build/lambda/splitspends ./apps/api
	printf '#!/bin/sh\nexec ./splitspends\n' > deploy/aws/build/lambda/run.sh
	chmod +x deploy/aws/build/lambda/run.sh
