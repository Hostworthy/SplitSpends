.PHONY: help setup dev api web check test test-integration lint fmt build docker web-build web-fonts lambda

.DEFAULT_GOAL := help
TEST_DATABASE_URL ?= postgres://splitspends:splitspends@localhost:5432/splitspends_test?sslmode=disable

help:
	@echo "Targets:"
	@echo "  setup             Copy env files, start Postgres, fetch Go and Flutter deps"
	@echo "  dev               Run the API (:8080) and the Flutter web app (:5173)"
	@echo "  api               Run only the API"
	@echo "  web               Run only the Flutter web app against the local API"
	@echo "  check             Format check, vet, unit tests, boundaries, flutter analyze and test"
	@echo "  test-integration  Database tests (needs TEST_DATABASE_URL)"
	@echo "  build             Build server and MCP binaries into bin/"
	@echo "  docker            Build the production image"
	@echo "  web-build         Build the Flutter web app into app/build/web"
	@echo "  web-fonts         Download the web fallback fonts (emoji, other scripts)"
	@echo "  lambda            Build the arm64 Lambda bundle for deploy/aws"

setup:
	cp -n backend/.env.example backend/.env 2>/dev/null || true
	docker compose up -d --wait
	docker compose exec -T postgres psql -U splitspends -tc "SELECT 1 FROM pg_database WHERE datname='splitspends_test'" | grep -q 1 \
		|| docker compose exec -T postgres createdb -U splitspends splitspends_test
	cd backend && go mod download
	cd app && flutter pub get

api:
	cd backend && set -a && . ./.env && set +a && go run ./cmd/server

web: web-fonts
	cd app && flutter run -d web-server --web-port 5173 --dart-define=SPLITSPENDS_URL=http://localhost:8080

dev:
	$(MAKE) -j2 api web

fmt:
	cd backend && gofmt -w .
	cd app && dart format .

lint:
	@test -z "$$(cd backend && gofmt -l .)" || (echo "gofmt needed:"; cd backend && gofmt -l .; exit 1)
	cd backend && go vet ./...
	cd app && dart format --output=none --set-exit-if-changed . && flutter analyze

test:
	cd backend && go test -count=1 ./...
	cd app && flutter test

test-integration:
	cd backend && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -count=1 ./internal/bootstrap/...

check: lint test

build:
	cd backend && CGO_ENABLED=0 go build -trimpath -o ../bin/splitspends ./cmd/server
	cd backend && CGO_ENABLED=0 go build -trimpath -o ../bin/splitspends-mcp ./cmd/mcp

docker:
	docker build -t splitspends:latest .

web-fonts:
	app/tool/fallback_fonts.sh

web-build: web-fonts
	cd app && flutter build web --release --no-web-resources-cdn

lambda:
	mkdir -p deploy/aws/build/lambda
	cd backend && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o ../deploy/aws/build/lambda/splitspends ./cmd/server
	printf '#!/bin/sh\nexec ./splitspends\n' > deploy/aws/build/lambda/run.sh
	chmod +x deploy/aws/build/lambda/run.sh
