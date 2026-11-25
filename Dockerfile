# syntax=docker/dockerfile:1.7
# One image serves the API, the MCP endpoint and the Flutter web app.

# Base images are pinned by digest; Dependabot (docker ecosystem) proposes
# new digests, so the pins are refreshed through review instead of going stale.
FROM debian:bookworm-slim@sha256:e67eeead736ff7bbe258ef088bff683635c561b1fcfefad0806c1df4ab49ce0e AS web
# The official Flutter release named in .flutter-version, which CI also uses.
# FLUTTER_SHA256 is that archive's "sha256" in Flutter's releases_linux.json;
# update both together.
ARG FLUTTER_SHA256=4ad17fc8462fbcef2ba19ff4a78c0a16b7147fde1add8578e267ea62cdb92678
COPY .flutter-version /tmp/flutter-version
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git unzip xz-utils \
 && rm -rf /var/lib/apt/lists/* \
 && curl -fsSL -o /tmp/flutter.tar.xz "https://storage.googleapis.com/flutter_infra_release/releases/stable/linux/flutter_linux_$(cat /tmp/flutter-version)-stable.tar.xz" \
 && { echo "${FLUTTER_SHA256}  /tmp/flutter.tar.xz" | sha256sum -c - \
      || { echo "The Flutter archive does not match FLUTTER_SHA256. Set it to the hash for $(cat /tmp/flutter-version) in releases_linux.json." >&2; exit 1; }; } \
 && tar -xJf /tmp/flutter.tar.xz -C /opt \
 && rm /tmp/flutter.tar.xz \
 && git config --global --add safe.directory /opt/flutter
ENV PATH="/opt/flutter/bin:${PATH}"
RUN flutter config --no-analytics --enable-web && flutter precache --web
WORKDIR /src
# Workspace manifests first so dependency downloads are cached.
COPY pubspec.yaml pubspec.lock ./
COPY packages/ledger/pubspec.yaml packages/ledger/
COPY packages/client/pubspec.yaml packages/client/
COPY apps/apk/pubspec.yaml apps/apk/
COPY apps/ios/pubspec.yaml apps/ios/
COPY apps/web/pubspec.yaml apps/web/
RUN flutter pub get --enforce-lockfile
COPY packages/ledger packages/ledger
COPY packages/client packages/client
COPY apps/web apps/web
WORKDIR /src/apps/web
RUN flutter build web --release --no-web-resources-cdn

# The Go release go.mod names, so the image and the release binaries match.
FROM golang:1.26.11-alpine@sha256:ecb67f987aa1a57794c91d19ade7ae0209f2f0f5496aaef9e6ae85ddf92d02ab AS api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY packages packages
COPY apps/api apps/api
COPY apps/mcp apps/mcp
COPY scripts/release/notices.sh scripts/release/
RUN mkdir -p /notices && sh scripts/release/notices.sh /notices/THIRD_PARTY_NOTICES.txt
ARG VERSION=dev
# The same flags as LDFLAGS in the Makefile.
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/splitspends ./apps/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/splitspends-mcp ./apps/mcp \
 && mkdir -p /data

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=api /out/ /usr/local/bin/
COPY --from=api /notices/ /usr/share/doc/splitspends/
# /data must belong to the runtime user: Docker seeds new volumes from it.
COPY --from=api --chown=nonroot:nonroot /data /data
COPY --from=web /src/apps/web/build/web /srv/web
ENV ADDR=:8080 WEB_DIR=/srv/web DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/splitspends"]
