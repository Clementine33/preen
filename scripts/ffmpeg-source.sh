#!/bin/sh
# Run from the source download directory; verify the release signer before extracting.
set -eu
ffmpeg_version="$(sed -n 's/^ffmpeg = "\([^"]*\)"$/\1/p' /src/mise.toml)"
test -n "$ffmpeg_version"
get() { curl -fsSL --http1.1 --connect-timeout 15 --retry 3 --retry-all-errors "$@"; }
for suffix in tar.xz tar.xz.asc; do
    file="ffmpeg-${ffmpeg_version}.${suffix}"
    # Fallback mirror: Gentoo shards by the first byte of the filename's BLAKE2b-512.
    shard="$(printf %s "$file" | b2sum | cut -c1-2)"
    get "https://ffmpeg.org/releases/$file" -o "ffmpeg.$suffix" ||
        get "https://distfiles.gentoo.org/distfiles/$shard/$file" -o "ffmpeg.$suffix"
done
get https://ffmpeg.org/ffmpeg-devel.asc -o ffmpeg-devel.asc ||
    get https://raw.githubusercontent.com/FFmpeg/web/master/htdocs/ffmpeg-devel.asc -o ffmpeg-devel.asc
export GNUPGHOME="$(mktemp -d)"
gpg --batch --quiet --import ffmpeg-devel.asc
gpg --batch --status-fd 1 --verify ffmpeg.tar.xz.asc ffmpeg.tar.xz > ffmpeg-signature.txt
# FFmpeg release key: https://ffmpeg.org/download.html#releases
grep -Eq '^\[GNUPG:\] VALIDSIG .* FCF986EA15E6E293A5644F10B4322F04D67658D8$' ffmpeg-signature.txt
tar -xJf ffmpeg.tar.xz
