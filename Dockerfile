FROM ghcr.io/jdx/mise:2026.10.4-debian AS tools
RUN apt-get update && apt-get install -y --no-install-recommends libatomic1 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY mise.toml ./
RUN mise trust && mise install node pnpm go

FROM tools AS web
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/web/package.json frontend/web/package.json
RUN mise exec node pnpm -- pnpm install --frozen-lockfile
COPY frontend/web frontend/web
RUN mise exec node pnpm -- pnpm -C frontend/web build

FROM tools AS backend
ARG VERSION
COPY backend/go backend/go
WORKDIR /src/backend/go
RUN printf '%s\n' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' \
    && case "$VERSION" in *-dev) exit 1;; esac \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 mise exec go -- go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/preen ./cmd/preen

FROM alpine:3.24 AS media
# Link LAME and zlib from their static archives; Opus stays a runtime package.
RUN apk add --no-cache build-base pkgconf nasm curl xz gnupg lame-dev opus-dev zlib-dev zlib-static \
    && rm /usr/lib/libmp3lame.so /usr/lib/libz.so
WORKDIR /build
COPY mise.toml ./
# FFmpeg release signing key: https://ffmpeg.org/download.html#releases
ARG FFMPEG_SIGNING_KEY=FCF986EA15E6E293A5644F10B4322F04D67658D8
# Keep this audio whitelist aligned with the encoder adapter; no video pipeline.
# The final check allows only musl and libopus as runtime libraries.
RUN ffmpeg_version="$(sed -n 's/^ffmpeg = "\([^"]*\)"$/\1/p' mise.toml)" \
    && test -n "$ffmpeg_version" \
    && curl -fsSL "https://ffmpeg.org/releases/ffmpeg-${ffmpeg_version}.tar.xz" -o ffmpeg.tar.xz \
    && curl -fsSL "https://ffmpeg.org/releases/ffmpeg-${ffmpeg_version}.tar.xz.asc" -o ffmpeg.tar.xz.asc \
    && curl -fsSL https://ffmpeg.org/ffmpeg-devel.asc -o ffmpeg-devel.asc \
    && export GNUPGHOME="$(mktemp -d)" \
    && gpg --batch --quiet --import ffmpeg-devel.asc \
    && gpg --batch --status-fd 1 --verify ffmpeg.tar.xz.asc ffmpeg.tar.xz \
        | grep -Eq "^\[GNUPG:\] VALIDSIG .* ${FFMPEG_SIGNING_KEY}\$" \
    && tar -xJf ffmpeg.tar.xz \
    && cd "ffmpeg-${ffmpeg_version}" \
    && ./configure \
        --prefix=/out --disable-everything --disable-autodetect \
        --disable-shared --enable-static \
        --disable-doc --disable-debug --disable-network --disable-avdevice --disable-swscale \
        --enable-small --enable-libmp3lame --enable-libopus --enable-zlib \
        --enable-protocol=file,pipe,fd \
        --enable-demuxer=wav,flac,mp3,aac,mov,ogg \
        --enable-muxer=wav,flac,mp3,ipod,opus,null \
        --enable-decoder='pcm_*,flac,mp3,mp3float,aac,alac,opus,png,mjpeg' \
        --enable-encoder=aac,flac,pcm_s16le,pcm_s24le,pcm_s32le,pcm_f32le,pcm_f64le,libmp3lame,libopus \
        --enable-parser=aac,aac_latm,flac,mpegaudio,opus,png,mjpeg \
        --enable-filter=aresample,aformat,anull \
    && make -j"$(getconf _NPROCESSORS_ONLN)" \
    && make install \
    && ! readelf -d /out/bin/ffmpeg /out/bin/ffprobe | sed -n 's/.*Shared library: \[\(.*\)\]/\1/p' \
        | grep -Evx 'libc\.musl-x86_64\.so\.1|libopus\.so\.0'

FROM alpine:3.24
RUN apk add --no-cache ca-certificates opus \
    && mkdir -p /data /app/web && chown 1000:1000 /data
COPY --from=backend /out/preen /usr/local/bin/preen
COPY --from=web /src/frontend/web/dist /app/web
COPY --from=media /out/bin /opt/ffmpeg/bin
ENV PATH="/opt/ffmpeg/bin:$PATH" PREEN_DATA_DIR=/data
USER 1000:1000
EXPOSE 8765
CMD ["preen", "--allow-lan", "--port=8765", "--web-dir=/app/web"]
