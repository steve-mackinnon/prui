#!/bin/sh
set -eu

case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) echo "prui supports macOS and Linux only." >&2; exit 1 ;;
esac
case "$(uname -m)" in
    arm64|aarch64) arch=arm64 ;;
    x86_64|amd64) arch=amd64 ;;
    *) echo "prui supports ARM64 and x86-64 only." >&2; exit 1 ;;
esac

if ! command -v gh >/dev/null 2>&1; then
    echo "Install the GitHub CLI from https://cli.github.com/ and run gh auth login first." >&2
    exit 1
fi

download_dir=$(mktemp -d)
trap 'rm -rf "$download_dir"' EXIT
trap 'exit 1' HUP INT TERM

gh release download --repo steve-mackinnon/prui \
    --pattern "prui_*_${os}_${arch}.tar.gz" --pattern checksums.txt \
    --dir "$download_dir"

cd "$download_dir"
set -- prui_*_"${os}_${arch}".tar.gz
if [ "$#" -ne 1 ] || [ ! -f "$1" ]; then
    echo "Expected one release archive for ${os}_${arch}." >&2
    exit 1
fi
archive=$1
awk -v archive="$archive" '$2 == archive { print }' checksums.txt > selected-checksum.txt
if [ "$(wc -l < selected-checksum.txt)" -ne 1 ]; then
    echo "Expected one checksum for $archive." >&2
    exit 1
fi
if [ "$os" = darwin ]; then
    shasum -a 256 -c selected-checksum.txt
else
    sha256sum -c selected-checksum.txt
fi
tar -xzf "$archive" prui
mkdir -p "$HOME/.local/bin"
install -m 755 prui "$HOME/.local/bin/prui"
"$HOME/.local/bin/prui" --version
printf '\nInstalled to %s/.local/bin/prui\n' "$HOME"
case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *) printf 'Add this to your shell startup file, then run it in this terminal:\n  export PATH="$HOME/.local/bin:$PATH"\n' ;;
esac
