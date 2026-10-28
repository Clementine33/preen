#!/bin/sh
# MinGW audio tools for Windows portable desktop.
set -eu
jobs=2
host=x86_64-w64-mingw32
prefix=/deps
ffmpeg_version="$(sed -n 's/^ffmpeg = "\([^"]*\)"$/\1/p' /src/mise.toml)"
lame_version="$(sed -n 's/^lame = "\([^"]*\)"$/\1/p' /src/mise.toml)"
opus_version="$(sed -n 's/^opus = "\([^"]*\)"$/\1/p' /src/mise.toml)"
zlib_version="$(sed -n 's/^zlib = "\([^"]*\)"$/\1/p' /src/mise.toml)"
test -n "$ffmpeg_version" && test -n "$lame_version" && test -n "$opus_version" && test -n "$zlib_version"
mkdir -p /out/bin

sh /src/scripts/ffmpeg-source.sh
curl -fsSL "https://downloads.sourceforge.net/project/lame/lame/${lame_version}/lame-${lame_version}.tar.gz" -o lame.tar.gz
curl -fsSL "https://downloads.xiph.org/releases/opus/opus-${opus_version}.tar.gz" -o opus.tar.gz
curl -fsSL "https://zlib.net/fossils/zlib-${zlib_version}.tar.gz" -o zlib.tar.gz
# LAME: https://github.com/spack/spack-packages/blob/develop/repos/spack_repo/builtin/packages/lame/package.py
# Opus: https://opus-codec.org/news/ ; zlib: https://github.com/madler/zlib/releases
sha256sum -c <<'HASHES'
ddfe36cab873794038ae2c1210557ad34857a4b6bdc515785d1da9e175b1da1e  lame.tar.gz
6ffcb593207be92584df15b32466ed64bbec99109f007c82205f0194572411a1  opus.tar.gz
bb329a0a2cd0274d05519d61c667c062e06990d72e125ee2dfa8de64f0119d16  zlib.tar.gz
HASHES
tar -xzf lame.tar.gz
tar -xzf opus.tar.gz
tar -xzf zlib.tar.gz

cd "lame-${lame_version}"
./configure --host="$host" --prefix="$prefix" --disable-shared --enable-static \
    --disable-frontend --disable-decoder --disable-analyzer-hooks
make -j"$jobs"
make install
cd ..

cd "opus-${opus_version}"
./configure --host="$host" --prefix="$prefix" --disable-shared --enable-static \
    --disable-doc --disable-extra-programs
make -j"$jobs"
make install
cd ..

cd "zlib-${zlib_version}"
CHOST="$host" ./configure --prefix="$prefix" --static
make -j"$jobs"
make install
cd ..

cd "ffmpeg-${ffmpeg_version}"
PKG_CONFIG_LIBDIR="$prefix/lib/pkgconfig" sh /src/scripts/ffmpeg-configure.sh \
    --target-os=mingw32 --arch=x86_64 --enable-cross-compile --cross-prefix="$host-" \
    --pkg-config=pkg-config --pkg-config-flags=--static \
    --extra-cflags="-I$prefix/include" --extra-ldflags="-L$prefix/lib -static"
make -j"$jobs"
make install
cd ..

for exe in /out/bin/ffmpeg.exe /out/bin/ffprobe.exe; do
    "$host-objdump" -p "$exe" | sed -n 's/^[[:space:]]*DLL Name: //p' | tr '[:upper:]' '[:lower:]' > imports.txt
    test -s imports.txt
    # Explicit system DLL allowlist: fail closed on third-party DLLs and MinGW runtimes.
    if grep -Evx 'kernel32\.dll|msvcrt\.dll|advapi32\.dll|user32\.dll|shell32\.dll|ole32\.dll|bcrypt\.dll|ws2_32\.dll' imports.txt; then
        echo "Unexpected DLL import in $exe" >&2
        exit 1
    fi
done

cat > /out/THIRD_PARTY_NOTICES.txt <<EOF
Preen Windows third-party audio tools notices

Bundled in bin/ffmpeg.exe and bin/ffprobe.exe:
- FFmpeg ${ffmpeg_version}: https://ffmpeg.org/
- LAME ${lame_version}: https://lame.sourceforge.io/
- Opus ${opus_version}: https://opus-codec.org/
- zlib ${zlib_version}: https://zlib.net/
- MinGW-w64 and GCC runtime libraries
EOF
for section in \
    "FFmpeg:${ffmpeg_version}:ffmpeg-${ffmpeg_version}/LICENSE.md" \
    "FFmpeg LGPL-2.1:${ffmpeg_version}:ffmpeg-${ffmpeg_version}/COPYING.LGPLv2.1" \
    "LAME:${lame_version}:lame-${lame_version}/LICENSE" \
    "LAME LGPL-2.0:${lame_version}:lame-${lame_version}/COPYING" \
    "Opus:${opus_version}:opus-${opus_version}/COPYING" \
    "zlib:${zlib_version}:zlib-${zlib_version}/LICENSE" \
    "MinGW-w64::/usr/share/doc/mingw-w64-common/copyright" \
    "GCC Runtime::/usr/share/doc/gcc-mingw-w64-base/copyright"; do
    title="${section%%:*}"
    file="${section##*:}"
    printf '\n================================================================================\n%s\n================================================================================\n\n' "$title" >> /out/THIRD_PARTY_NOTICES.txt
    cat "$file" >> /out/THIRD_PARTY_NOTICES.txt
done

# Export only the portable executables and merged notices, not static libraries or headers.
rm -rf /out/include /out/lib /out/share
