#!/bin/sh
# Run from the source download directory; verify the release signer before extracting.
set -eu
ffmpeg_version="$(sed -n 's/^ffmpeg = "\([^"]*\)"$/\1/p' /src/mise.toml)"
test -n "$ffmpeg_version"
curl -fsSL "https://ffmpeg.org/releases/ffmpeg-${ffmpeg_version}.tar.xz" -o ffmpeg.tar.xz
curl -fsSL "https://ffmpeg.org/releases/ffmpeg-${ffmpeg_version}.tar.xz.asc" -o ffmpeg.tar.xz.asc
curl -fsSL https://ffmpeg.org/ffmpeg-devel.asc -o ffmpeg-devel.asc
export GNUPGHOME="$(mktemp -d)"
gpg --batch --quiet --import ffmpeg-devel.asc
gpg --batch --status-fd 1 --verify ffmpeg.tar.xz.asc ffmpeg.tar.xz > ffmpeg-signature.txt
# FFmpeg release key: https://ffmpeg.org/download.html#releases
grep -Eq '^\[GNUPG:\] VALIDSIG .* FCF986EA15E6E293A5644F10B4322F04D67658D8$' ffmpeg-signature.txt
tar -xJf ffmpeg.tar.xz
