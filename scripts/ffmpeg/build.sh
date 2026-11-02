#!/usr/bin/env bash
# 在一次性 ubuntu:24.04 容器内交叉编译接收端内嵌的精简 LGPL ffmpeg.exe。
# 由 scripts/build-ffmpeg.ps1 调用：/src 为已校验哈希的源码包，/out 为输出目录。
# 只启用视频模式需要的组件：ddagrab 桌面复制、GPU/CPU 缩放、NVENC / AMF / Media Foundation
# H.264 编码与 FLV 输出；不启用任何 GPL / nonfree 组件，静态链接、无额外 DLL。
set -euo pipefail

FFMPEG_VERSION="$1"
NVCODEC_VERSION="$2"
AMF_VERSION="$3"

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq --no-install-recommends mingw-w64 nasm make pkg-config xz-utils ca-certificates >/dev/null

work=/tmp/build
deps=/opt/deps
mkdir -p "$work" "$deps/include"
cd "$work"
tar -xJf "/src/ffmpeg-${FFMPEG_VERSION}.tar.xz"
tar -xzf "/src/nv-codec-headers-n${NVCODEC_VERSION}.tar.gz"
tar -xzf "/src/AMF-headers-v${AMF_VERSION}.tar.gz"

make -C "nv-codec-headers-n${NVCODEC_VERSION}" PREFIX="$deps" install >/dev/null
cp -r "amf-headers-v${AMF_VERSION}/AMF" "$deps/include/"

export PKG_CONFIG_PATH="$deps/lib/pkgconfig"
cd "ffmpeg-${FFMPEG_VERSION}"
configure_flags=(
  --target-os=mingw32 --arch=x86_64 --cross-prefix=x86_64-w64-mingw32-
  --pkg-config=pkg-config --pkg-config-flags=--static
  --enable-static --disable-shared --disable-debug --disable-doc
  --disable-autodetect --disable-everything --disable-network
  --disable-ffplay --disable-ffprobe --enable-w32threads
  --enable-d3d11va --enable-ffnvcodec --enable-nvenc --enable-amf --enable-mediafoundation
  --enable-indev=lavfi
  --enable-filter=ddagrab,hwdownload,hwmap,hwupload,scale,scale_d3d11,format,null
  --enable-encoder=h264_nvenc,h264_amf,h264_mf,wrapped_avframe
  --enable-bsf=h264_metadata,extract_extradata
  --enable-parser=h264
  --enable-muxer=flv,null
  --enable-protocol=pipe,file
  --enable-swscale
  --extra-cflags="-I$deps/include -O2"
  --extra-ldflags="-static -static-libgcc"
)
./configure "${configure_flags[@]}" >/tmp/configure.log || { tail -40 /tmp/configure.log; cat ffbuild/config.log | tail -60; exit 1; }
# 构建前确认 GPL / nonfree 均未启用，许可证为 LGPL。
grep -q '^#define CONFIG_GPL 0' config.h
grep -q '^#define CONFIG_NONFREE 0' config.h
grep -q '^#define CONFIG_DDAGRAB_FILTER 1' config_components.h
grep -q '^#define CONFIG_H264_NVENC_ENCODER 1' config_components.h
grep -q '^#define CONFIG_H264_AMF_ENCODER 1' config_components.h
grep -q '^#define CONFIG_H264_MF_ENCODER 1' config_components.h
make -j"$(nproc)" >/tmp/make.log 2>&1 || { tail -60 /tmp/make.log; exit 1; }
x86_64-w64-mingw32-strip -o /out/ffmpeg.exe ffmpeg.exe
# 记录真实使用的 configure 参数，写入清单供 LGPL 源码对应说明。
printf '%s ' "${configure_flags[@]}" >/out/configure.txt
cp COPYING.LGPLv2.1 /out/COPYING.LGPLv2.1
x86_64-w64-mingw32-objdump -p /out/ffmpeg.exe | grep 'DLL Name' | sort -u >/out/imports.txt
