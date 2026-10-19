FROM ghcr.io/jdx/mise:2026.10.4-debian AS tools
RUN apt-get update && apt-get install -y --no-install-recommends libatomic1 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY mise.toml ./
RUN mise trust && mise install node pnpm go ffmpeg

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

FROM tools AS media
# mise's FFmpeg distribution is relocatable, but dynamically linked.
RUN mkdir -p /out/bin /out/lib \
    && cp "$(mise where ffmpeg)/bin/ffmpeg" "$(mise where ffmpeg)/bin/ffprobe" /out/bin/ \
    && cd "$(mise where ffmpeg)/lib" \
    && find . -name '*.so*' -exec cp -a --parents {} /out/lib/ \;

FROM debian:13-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /data /app/web && chown 1000:1000 /data
COPY --from=backend /out/preen /usr/local/bin/preen
COPY --from=web /src/frontend/web/dist /app/web
COPY --from=media /out /opt/ffmpeg
ENV PATH="/opt/ffmpeg/bin:$PATH" PREEN_DATA_DIR=/data
USER 1000:1000
EXPOSE 8765
CMD ["preen", "--allow-lan", "--port=8765", "--web-dir=/app/web"]
