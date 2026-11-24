# syntax=docker/dockerfile:1.7
# One image serves the API, the MCP endpoint and the Flutter web app.

FROM debian:bookworm-slim AS web
# The official Flutter release, pinned to the version CI and local builds use.
ARG FLUTTER_VERSION=3.50.1
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git unzip xz-utils \
 && rm -rf /var/lib/apt/lists/* \
 && curl -fsSL "https://storage.googleapis.com/flutter_infra_release/releases/stable/linux/flutter_linux_${FLUTTER_VERSION}-stable.tar.xz" \
    | tar -xJ -C /opt \
 && git config --global --add safe.directory /opt/flutter
ENV PATH="/opt/flutter/bin:${PATH}"
RUN flutter config --no-analytics --enable-web && flutter precache --web
WORKDIR /src
# Workspace manifests first so dependency downloads are cached.
COPY pubspec.yaml pubspec.lock ./
COPY packages/ledger/pubspec.yaml packages/ledger/
COPY packages/client/pubspec.yaml packages/client/
COPY apps/android/pubspec.yaml apps/android/
COPY apps/ios/pubspec.yaml apps/ios/
COPY apps/web/pubspec.yaml apps/web/
RUN flutter pub get
COPY packages/ledger packages/ledger
COPY packages/client packages/client
COPY apps/web apps/web
WORKDIR /src/apps/web
RUN flutter build web --release --no-web-resources-cdn

FROM golang:1.26-alpine AS api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY packages packages
COPY apps/api apps/api
COPY apps/mcp apps/mcp
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/splitspends ./apps/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/splitspends-mcp ./apps/mcp \
 && mkdir -p /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=api /out/ /usr/local/bin/
# /data must belong to the runtime user: Docker seeds new volumes from it.
COPY --from=api --chown=nonroot:nonroot /data /data
COPY --from=web /src/apps/web/build/web /srv/web
ENV ADDR=:8080 WEB_DIR=/srv/web DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/splitspends"]
