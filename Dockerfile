# syntax=docker/dockerfile:1.7
# One image serves the API, the MCP endpoint and the Flutter web app.

FROM ghcr.io/cirruslabs/flutter:3.50.1 AS web
WORKDIR /src/app
COPY app/pubspec.yaml app/pubspec.lock* ./
RUN flutter pub get
# Fallback fonts (emoji, non-Latin scripts) served by this image instead of
# Google Fonts. Its own layer, so it is cached until the lock file changes.
# --build-arg FALLBACK_FONTS=notocoloremoji keeps only emoji (about 2 MB).
ARG FALLBACK_FONTS=all
COPY app/tool/fallback_fonts.sh app/tool/fallback_fonts.lock ./tool/
RUN FALLBACK_FONTS="$FALLBACK_FONTS" tool/fallback_fonts.sh
COPY app/ ./
RUN flutter build web --release --no-web-resources-cdn

FROM golang:1.26-alpine AS api
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/splitspends ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/splitspends-mcp ./cmd/mcp

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=api /out/ /usr/local/bin/
COPY --from=web /src/app/build/web /srv/web
ENV ADDR=:8080 WEB_DIR=/srv/web DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/splitspends"]
