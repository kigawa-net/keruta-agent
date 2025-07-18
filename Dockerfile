# ビルドステージ
FROM golang:1.22-alpine AS builder

# 必要なパッケージをインストール
RUN apk add --no-cache git ca-certificates tzdata

# 作業ディレクトリを設定
WORKDIR /app

# Goモジュールファイルをコピー
COPY go.mod go.sum ./

# 依存関係をダウンロード
RUN go mod download

# ソースコードをコピー
COPY . .

# バイナリをビルド
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o keruta-agent ./cmd/keruta-agent

# 実行ステージ
FROM alpine:latest

# 必要なパッケージをインストール
RUN apk --no-cache add ca-certificates tzdata

# 作業ディレクトリを設定
WORKDIR /root

# ビルドステージからバイナリをコピー
COPY --from=builder /app/keruta-agent .

# 成果物ディレクトリを作成
RUN mkdir -p /.keruta/doc

# 実行権限を付与
RUN chmod +x keruta-agent

# 環境変数を設定
ENV KERUTA_ARTIFACTS_DIR=/.keruta/doc

# エントリーポイントを設定
ENTRYPOINT ["./keruta-agent"]
