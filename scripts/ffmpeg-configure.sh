#!/bin/sh
# Shared audio/cover whitelist for the container and Windows portable build.
set -eu
exec ./configure \
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
    "$@"
