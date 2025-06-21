#!/bin/bash

# keruta-agent ビルドスクリプト

set -e

# スクリプトのディレクトリを取得
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# 色付き出力の関数
print_info() {
    echo -e "\033[1;34m[INFO]\033[0m $1"
}

print_success() {
    echo -e "\033[1;32m[SUCCESS]\033[0m $1"
}

print_error() {
    echo -e "\033[1;31m[ERROR]\033[0m $1"
}

print_warning() {
    echo -e "\033[1;33m[WARNING]\033[0m $1"
}

# プロジェクトルートに移動
cd "$PROJECT_ROOT"

print_info "keruta-agent のビルドを開始します"

# Goのバージョンチェック
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
print_info "Go バージョン: $GO_VERSION"

# 必要なGoバージョン（1.21以上）
REQUIRED_VERSION="1.21"
if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
    print_error "Go 1.21以上が必要です。現在のバージョン: $GO_VERSION"
    exit 1
fi

# 依存関係のダウンロード
print_info "依存関係をダウンロード中..."
go mod download

# テストの実行
print_info "テストを実行中..."
go test ./...

# バイナリのビルド
print_info "バイナリをビルド中..."
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S_UTC')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# ビルドフラグ
LDFLAGS="-X main.BuildTime=$BUILD_TIME -X main.GitCommit=$GIT_COMMIT"

# バイナリをビルド
go build -ldflags "$LDFLAGS" -o keruta-agent ./cmd/keruta-agent

# ビルド結果の確認
if [ -f "keruta-agent" ]; then
    print_success "バイナリのビルドが完了しました"
    print_info "バイナリサイズ: $(du -h keruta-agent | cut -f1)"
    print_info "ビルド時刻: $BUILD_TIME"
    print_info "Gitコミット: $GIT_COMMIT"
else
    print_error "バイナリのビルドに失敗しました"
    exit 1
fi

# 実行権限を付与
chmod +x keruta-agent

print_success "keruta-agent のビルドが完了しました！"
print_info "使用方法: ./keruta-agent --help" 