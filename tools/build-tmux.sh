#!/usr/bin/env bash
# Build tmux for one macOS architecture with libevent linked in statically, for the Mac app to carry beside handlined and hl:
# tools/build-tmux.sh <arm64|amd64> <outdir>. Writes <outdir>/tmux and <outdir>/licenses/{tmux,libevent}.txt (both are under permissive
# licences that ask for their notice to travel with the binary). The two sources are pinned by checksum.
arch="$1"
out="$2"
if { [ "$arch" != arm64 ] && [ "$arch" != amd64 ]; } || [ -z "$out" ] || [ $# -ne 2 ]; then
    echo "usage: tools/build-tmux.sh <arm64|amd64> <outdir>" >&2
    exit 2
fi
TMUX_VERSION=3.8
TMUX_SHA256=e79c699c7e949dccd0a4a125e17b8d1261e311b16979dbc8eb34542f3966d82e
EVENT_VERSION=2.1.12
EVENT_SHA256=92e6de1be9ec176428fd2367677e61ceffc2ee1cb119035037a27d346b0403bb
cc_arch=arm64; host=aarch64-apple-darwin
if [ "$arch" = amd64 ]; then cc_arch=x86_64; host=x86_64-apple-darwin; fi
export MACOSX_DEPLOYMENT_TARGET=11.0
export CC="clang -arch $cc_arch"
work="$(mktemp -d "${TMPDIR:-/tmp}/build-tmux.XXXXXX")" || exit 1
trap 'rm -rf "$work"' EXIT
cd "$work" || exit 1
fetch() { # url sha256 file
    curl -fsSL -o "$3" "$1" || exit 1
    [ "$(shasum -a 256 "$3" | cut -d' ' -f1)" = "$2" ] || { echo "tools/build-tmux.sh: $3 is not the file pinned" >&2; exit 1; }
}
fetch "https://github.com/libevent/libevent/releases/download/release-$EVENT_VERSION-stable/libevent-$EVENT_VERSION-stable.tar.gz" "$EVENT_SHA256" libevent.tgz
fetch "https://github.com/tmux/tmux/releases/download/$TMUX_VERSION/tmux-$TMUX_VERSION.tar.gz" "$TMUX_SHA256" tmux.tgz
tar xzf libevent.tgz && tar xzf tmux.tgz || exit 1
prefix="$work/prefix"
(cd "libevent-$EVENT_VERSION-stable" &&
    ./configure --host="$host" --prefix="$prefix" --disable-shared --enable-static --disable-openssl --disable-samples --disable-libevent-regress >/dev/null &&
    make -j"$(sysctl -n hw.ncpu)" >/dev/null && make install >/dev/null) || exit 1
(cd "tmux-$TMUX_VERSION" &&
    PKG_CONFIG_LIBDIR=/nonexistent LIBEVENT_CFLAGS="-I$prefix/include" LIBEVENT_LIBS="$prefix/lib/libevent.a" \
        ./configure --host="$host" --disable-utf8proc --disable-jemalloc >/dev/null &&
    make -j"$(sysctl -n hw.ncpu)" >/dev/null) || exit 1
mkdir -p "$out/licenses" || exit 1
cp "tmux-$TMUX_VERSION/tmux" "$out/tmux" && strip -x "$out/tmux" || exit 1
cp "tmux-$TMUX_VERSION/COPYING" "$out/licenses/tmux.txt" && cp "libevent-$EVENT_VERSION-stable/LICENSE" "$out/licenses/libevent.txt" || exit 1
# Only the system's libraries may be left to the Mac: libevent is inside.
if otool -L "$out/tmux" | tail -n +2 | grep -v -e '^[[:space:]]*/usr/lib/' -e '^[[:space:]]*/System/'; then
    echo "tools/build-tmux.sh: tmux links something that is not the system's" >&2
    exit 1
fi
echo "$out/tmux: tmux $TMUX_VERSION with libevent $EVENT_VERSION ($arch)"
