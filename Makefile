.PHONY: help setup dev api web check test test-integration lint fmt build docker web-build apk lambda

.DEFAULT_GOAL := help
TEST_DATABASE_URL ?= postgres://splitspends:splitspends@localhost:5432/splitspends_test?sslmode=disable
DART_MEMBERS := packages/ledger packages/client apps/android apps/ios apps/web

help:
	@echo "Targets:"
	@echo "  setup             Copy env files, start Postgres, fetch Go and Dart dependencies"
	@echo "  dev               Run the API (:8080) and the web app (:5173)"
	@echo "  api               Run only the API (apps/api)"
	@echo "  web               Run only the web app (apps/web) against the local API"
	@echo "  check             Format check, vet, unit tests, boundaries, analyze, Dart tests"
	@echo "  test-integration  Database tests (needs TEST_DATABASE_URL)"
	@echo "  build             Build the API and MCP binaries into bin/"
	@echo "  web-build         Build the web app into apps/web/build/web"
	@echo "  apk               Build the Android app into apps/android/build"
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
	dart format $(DART_MEMBERS)

lint:
	@test -z "$$(gofmt -l apps packages)" || (echo "gofmt needed:"; gofmt -l apps packages; exit 1)
	go vet ./...
	dart format --output=none --set-exit-if-changed $(DART_MEMBERS)
	for d in $(DART_MEMBERS); do (cd $$d && flutter analyze) || exit 1; done

test:
	go test -count=1 ./...
	cd packages/ledger && dart test
	cd packages/client && flutter test

test-integration:
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -count=1 ./packages/bootstrap/...

check: lint test

build:
	CGO_ENABLED=0 go build -trimpath -o bin/splitspends ./apps/api
	CGO_ENABLED=0 go build -trimpath -o bin/splitspends-mcp ./apps/mcp

web-build:
	cd apps/web && flutter build web --release --no-web-resources-cdn

apk:
	cd apps/android && flutter build apk --release

docker:
	docker build -t splitspends:latest .

lambda:
	mkdir -p deploy/aws/build/lambda
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o deploy/aws/build/lambda/splitspends ./apps/api
	printf '#!/bin/sh\nexec ./splitspends\n' > deploy/aws/build/lambda/run.sh
	chmod +x deploy/aws/build/lambda/run.sh
