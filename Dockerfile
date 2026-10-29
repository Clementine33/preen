FROM ghcr.io/jdx/mise:2026.10.4-debian AS tools
RUN apt-get update && apt-get install -y --no-install-recommends libatomic1 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY mise.toml ./
RUN mise trust && mise install node pnpm go

FROM tools AS web-build
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/web/package.json frontend/web/package.json
RUN mise exec node pnpm -- pnpm install --frozen-lockfile
COPY frontend/web frontend/web
RUN mise exec node pnpm -- pnpm -C frontend/web build

# Release overrides this stage with its frontend artifact as a named build context.
FROM scratch AS web
COPY --from=web-build /src/frontend/web/dist /

FROM tools AS backend
ARG VERSION
COPY scripts/release-version.mjs /src/scripts/release-version.mjs
COPY backend/go backend/go
WORKDIR /src/backend/go
RUN VERSION="$VERSION" mise exec node -- node --input-type=module -e \
    'import { releaseVersion } from "/src/scripts/release-version.mjs"; releaseVersion(process.env.VERSION)' \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 mise exec go -- go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/preen ./cmd/preen

FROM alpine:3.24 AS media
# Link LAME and zlib from their static archives; Opus stays a runtime package.
RUN apk add --no-cache build-base pkgconf nasm curl xz gnupg lame-dev opus-dev zlib-dev zlib-static \
    && rm /usr/lib/libmp3lame.so /usr/lib/libz.so
WORKDIR /build
COPY mise.toml /src/mise.toml
COPY scripts/ffmpeg-source.sh scripts/ffmpeg-configure.sh /src/scripts/
# The final check allows only musl and libopus as runtime libraries.
RUN sh /src/scripts/ffmpeg-source.sh \
    && ffmpeg_version="$(sed -n 's/^ffmpeg = "\([^"]*\)"$/\1/p' /src/mise.toml)" \
    && cd "ffmpeg-${ffmpeg_version}" \
    && sh /src/scripts/ffmpeg-configure.sh \
    && make -j"$(getconf _NPROCESSORS_ONLN)" \
    && make install \
    && ! readelf -d /out/bin/ffmpeg /out/bin/ffprobe | sed -n 's/.*Shared library: \[\(.*\)\]/\1/p' \
        | grep -Evx 'libc\.musl-x86_64\.so\.1|libopus\.so\.0'

FROM debian:13-slim AS media-windows
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential mingw-w64 pkg-config nasm curl ca-certificates xz-utils gnupg \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY mise.toml /src/mise.toml
COPY scripts/ffmpeg-source.sh scripts/ffmpeg-configure.sh scripts/build-windows-ffmpeg.sh /src/scripts/
RUN sh /src/scripts/build-windows-ffmpeg.sh

FROM scratch AS desktop-ffmpeg
COPY --from=media-windows /out/ /

FROM alpine:3.24
RUN apk add --no-cache ca-certificates opus \
    && mkdir -p /data /app/web && chown 1000:1000 /data
COPY --from=backend /out/preen /usr/local/bin/preen
COPY --from=web / /app/web
COPY --from=media /out/bin /opt/ffmpeg/bin
ENV PATH="/opt/ffmpeg/bin:$PATH" PREEN_DATA_DIR=/data
USER 1000:1000
EXPOSE 8765
CMD ["preen", "--allow-lan", "--port=8765", "--web-dir=/app/web"]
