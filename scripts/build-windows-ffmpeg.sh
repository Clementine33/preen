#!/bin/sh
# MinGW audio tools plus the corresponding sources, notices and build instructions.
set -eu
jobs=2
host=x86_64-w64-mingw32
prefix=/deps
ffmpeg_version="$(sed -n 's/^ffmpeg = "\([^"]*\)"$/\1/p' /src/mise.toml)"
lame_version="$(sed -n 's/^lame = "\([^"]*\)"$/\1/p' /src/mise.toml)"
opus_version="$(sed -n 's/^opus = "\([^"]*\)"$/\1/p' /src/mise.toml)"
zlib_version="$(sed -n 's/^zlib = "\([^"]*\)"$/\1/p' /src/mise.toml)"
test -n "$ffmpeg_version" && test -n "$lame_version" && test -n "$opus_version" && test -n "$zlib_version"
mkdir -p /out/bin /out/licenses /out/sources/scripts

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
# LAME's optional mpglib decoder is GPL; build only the LGPL encoder.
./configure --host="$host" --prefix="$prefix" --disable-shared --enable-static \
    --disable-frontend --disable-decoder --disable-analyzer-hooks
grep -qx '/\* #undef HAVE_MPGLIB \*/' config.h
make -j"$jobs"
make install
cp COPYING /out/licenses/lame-LGPL-2.0.txt
cp LICENSE /out/licenses/lame-LICENSE.txt
cd ..

cd "opus-${opus_version}"
./configure --host="$host" --prefix="$prefix" --disable-shared --enable-static \
    --disable-doc --disable-extra-programs
make -j"$jobs"
make install
cp COPYING /out/licenses/opus-BSD.txt
cd ..

cd "zlib-${zlib_version}"
CHOST="$host" ./configure --prefix="$prefix" --static
make -j"$jobs"
make install
cp LICENSE /out/licenses/zlib-LICENSE.txt
cd ..

cd "ffmpeg-${ffmpeg_version}"
PKG_CONFIG_LIBDIR="$prefix/lib/pkgconfig" sh /src/scripts/ffmpeg-configure.sh \
    --target-os=mingw32 --arch=x86_64 --enable-cross-compile --cross-prefix="$host-" \
    --pkg-config=pkg-config --pkg-config-flags=--static \
    --extra-cflags="-I$prefix/include" --extra-ldflags="-L$prefix/lib -static" \
    --disable-gpl --disable-nonfree --disable-version3
# These assertions exercise the configured license boundary before linking.
grep -qx '#define CONFIG_GPL 0' config.h
grep -qx '#define CONFIG_NONFREE 0' config.h
grep -qx '#define CONFIG_VERSION3 0' config.h
make -j"$jobs"
make install
cp COPYING.LGPLv2.1 /out/licenses/ffmpeg-LGPL-2.1.txt
cp LICENSE.md /out/licenses/ffmpeg-LICENSE.md
cp CREDITS /out/licenses/ffmpeg-CREDITS.txt
sed -n 's/^#define FFMPEG_CONFIGURATION //p' config.h > /out/licenses/ffmpeg-configure.txt
# Retain all source copyright notices, including those in statically linked components.
cd ..
for component in "ffmpeg-${ffmpeg_version}" "lame-${lame_version}" "opus-${opus_version}" "zlib-${zlib_version}"; do
    grep -rIin 'copyright' "$component" --include='*.c' --include='*.h' --include='*.asm' > "/out/licenses/${component}-copyright.txt"
done

for exe in /out/bin/ffmpeg.exe /out/bin/ffprobe.exe; do
    "$host-objdump" -p "$exe" | sed -n 's/^[[:space:]]*DLL Name: //p' | tr '[:upper:]' '[:lower:]' > imports.txt
    test -s imports.txt
    # Explicit system DLL allowlist: fail closed on third-party DLLs and MinGW runtimes.
    if grep -Evx 'kernel32\.dll|msvcrt\.dll|advapi32\.dll|user32\.dll|shell32\.dll|ole32\.dll|bcrypt\.dll|ws2_32\.dll' imports.txt; then
        echo "Unexpected DLL import in $exe" >&2
        exit 1
    fi
    cp imports.txt "/out/licenses/$(basename "$exe").imports.txt"
done

cp ffmpeg.tar.xz "/out/sources/ffmpeg-${ffmpeg_version}.tar.xz"
cp ffmpeg.tar.xz.asc "/out/sources/ffmpeg-${ffmpeg_version}.tar.xz.asc"
cp ffmpeg-devel.asc ffmpeg-signature.txt /out/sources/
cp lame.tar.gz "/out/sources/lame-${lame_version}.tar.gz"
cp opus.tar.gz "/out/sources/opus-${opus_version}.tar.gz"
cp zlib.tar.gz "/out/sources/zlib-${zlib_version}.tar.gz"
cp /src/scripts/* /out/sources/scripts/
cp /src/mise.toml /src/Dockerfile /out/sources/
cd /out/sources
sha256sum ./*.tar.* > SHA256SUMS
cat > /out/licenses/README.txt <<EOF
Preen Windows audio tools (x86_64)

FFmpeg ${ffmpeg_version}: https://ffmpeg.org/ (LGPL-2.1-or-later)
LAME ${lame_version}: https://lame.sourceforge.io/ (LGPL-2.0-or-later, encoder only)
Opus ${opus_version}: https://opus-codec.org/ (BSD-3-Clause)
zlib ${zlib_version}: https://zlib.net/ (zlib license, PNG cover decoding)

All libraries are statically linked; only Windows system DLLs are imported.
FFmpeg GPL, nonfree and version3 components are disabled. LAME's GPL decoder is disabled.
Each component's full license and copyright notices accompany this file.
MinGW and GCC runtime notices include the GCC Runtime Library Exception.

The corresponding sources artifact distributed with this portable archive contains
the verified upstream tarballs, FFmpeg signature/key, SHA256SUMS, mise.toml,
Dockerfile and scripts/*.sh used to produce these executables. It must accompany
every distribution. FFmpeg's exact configure command is in ffmpeg-configure.txt;
dependency configure commands are in scripts/build-windows-ffmpeg.sh.
Rebuild or modify: place the artifact contents in an empty directory, then run:
  docker build --target desktop-ffmpeg --output type=local,dest=rebuilt .
The Dockerfile fetches the pinned upstream sources and checks signatures/hashes.
Source URLs are recorded in scripts/build-windows-ffmpeg.sh and ffmpeg-source.sh.
No upstream source changes were applied. Rebuilding from modified sources is
permitted under the included licenses; place the rebuilt tools in Preen's bin/.
EOF
"$host-gcc" --version > /out/licenses/compiler.txt
cp /usr/share/doc/mingw-w64-common/copyright /out/licenses/mingw-w64-copyright.txt
cp /usr/share/doc/gcc-mingw-w64-base/copyright /out/licenses/gcc-runtime-copyright.txt
cp /usr/share/common-licenses/GPL-3 /out/licenses/gcc-runtime-GPL-3.txt
# Export only the portable executables, not static libraries or development headers.
rm -rf /out/include /out/lib /out/share
