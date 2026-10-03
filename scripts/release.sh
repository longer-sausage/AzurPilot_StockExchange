#!/bin/bash
# 在开发机/CI 构建，低性能服务器只需解包运行。CGO=0，无 Node/Go 运行时依赖。
set -euo pipefail
ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
npm ci --prefix frontend --no-audit --no-fund
npm run build --prefix frontend
go test ./...
mkdir -p bin releases
for arch in amd64 arm64; do CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "bin/exchange-linux-$arch" ./cmd/exchange; done
PACKAGE_DIR="$(mktemp -d /tmp/mmex-package.XXXXXX)"
mkdir -p "$PACKAGE_DIR/bin" "$PACKAGE_DIR/frontend/dist"
cp bin/exchange-linux-* "$PACKAGE_DIR/bin/"
cp -R frontend/dist/. "$PACKAGE_DIR/frontend/dist/"
(cd "$PACKAGE_DIR" && find bin frontend/dist -type f -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS && tar -czf "$ROOT_DIR/releases/mingmiao-exchange.tar.gz" bin frontend/dist SHA256SUMS)
echo "$ROOT_DIR/releases/mingmiao-exchange.tar.gz"
