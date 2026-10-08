#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

for tool in go git python3; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "Verification requires $tool on PATH." >&2
        exit 1
    fi
done

case "$(go env GOOS)" in
    darwin|linux) ;;
    *) echo "Terminal verification supports macOS and Linux." >&2; exit 1 ;;
esac

unformatted=$(find cmd internal -name '*.go' -type f -exec gofmt -l {} +)
if [ -n "$unformatted" ]; then
    echo "Run gofmt on these files:" >&2
    echo "$unformatted" >&2
    exit 1
fi

sh -n scripts/install.sh
python3 scripts/install_test.py

go vet ./...
go test -race -count=1 -timeout=5m ./...
go build ./...
