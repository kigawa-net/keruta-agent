# keruta-agent

> **概要**: kerutaによって実行されるjobで利用できるkerutaコマンドを実装するサブプロジェクトです。Coderワークスペース内でデーモンとして動作し、タスクの実行状況をkeruta APIサーバーに報告するCLIツールです。

## 目次
- [概要](#概要)
- [アーキテクチャ](#アーキテクチャ)
- [機能仕様](#機能仕様)
- [コマンド仕様](#コマンド仕様)
- [環境変数](#環境変数)
- [セットアップ](#セットアップ)
- [使用方法](#使用方法)
- [技術スタック](#技術スタック)
- [プロジェクト構造](#プロジェクト構造)
- [関連リンク](#関連リンク)

## 概要
`keruta-agent`は、kerutaシステムによってCoderワークスペース内で動作するCLIツールです。各ワークスペースに対応するセッションのタスクを一つずつ順次実行し、タスクの実行状況をkeruta APIサーバーに報告します。デーモンモードでバックグラウンド動作し、成果物の保存、ログの収集、エラーハンドリングなどの機能を提供します。

### 主な機能
- **セッション連携** - ワークスペースに対応するセッションのタスクを順次実行
- **Gitリポジトリ管理** - セッションのテンプレート設定に基づくGitリポジトリの自動クローン/プル（`~/keruta`ディレクトリ配下）
- **デーモンモード実行** - Coderワークスペース内でバックグラウンド実行
- **タスクキュー処理** - セッション内のタスクを一つずつ順次処理
- **タスクステータス管理** - タスクステータスの更新（PENDING → PROCESSING → COMPLETED/FAILED）
- **成果物管理** - 成果物の保存（`/.keruta/doc`ディレクトリ配下のファイル）
- **ログ収集** - 実行ログの収集と送信
- **エラーハンドリング** - エラー発生時の自動修正タスク作成
- **監視機能** - タスク実行時間の計測、ヘルスチェック機能
- **入力処理** - HTTP APIを通じた動的入力処理

## アーキテクチャ
```
┌─────────────────────────────────────────────────────────┐
│               Coder Workspace                           │
│                                                         │
│  ┌─────────────────┐    ┌─────────────────────────────┐ │
│  │   Task Queue    │    │    keruta-agent (daemon)   │ │
│  │                 │    │                             │ │
│  │ Task 1: PENDING │───▶│  • セッション監視           │ │
│  │ Task 2: PENDING │    │  • タスクキュー処理         │ │
│  │ Task 3: PENDING │    │  • タスク順次実行           │ │
│  │                 │    │  • ステータス更新           │ │
│  └─────────────────┘    │  • 成果物・ログ収集         │ │
│                         └─────────────────────────────┘ │
│                                                         │
└─────────────────────────────────────────────────────────┘
                                │
                                ▼
                    ┌─────────────────────────┐
                    │     keruta API          │
                    │   (Spring Boot)         │
                    │                         │
                    │  • セッション管理       │
                    │  • タスク状態更新       │
                    │  • ドキュメント保存     │
                    │  • ログ保存             │
                    └─────────────────────────┘
```

### セッション・ワークスペース・タスクの関係
```
Session (1) ←→ (1) Workspace ←→ (1) keruta-agent
    │
    └── Tasks (1..n)
        ├── Task 1 (PENDING → PROCESSING → COMPLETED)
        ├── Task 2 (PENDING → PROCESSING → COMPLETED)
        └── Task n (PENDING → PROCESSING → COMPLETED)
```

## 機能仕様

### 1. セッション・タスク管理
- **セッション監視** - ワークスペースに対応するセッションの状態を監視
- **タスクキューイング** - セッション内のタスクを取得しキューに追加
- **順次実行** - タスクを一つずつ順番に実行（並列実行なし）
- **状態同期** - タスクの実行状態をリアルタイムでAPIに報告

### 2. タスクライフサイクル管理
- **取得**: セッションからPENDINGタスクを取得
- **開始**: `keruta start` - タスクをPROCESSING状態に更新
- **実行**: タスクスクリプトの実行
- **成功**: `keruta success` - タスクをCOMPLETED状態に更新
- **失敗**: `keruta fail` - タスクをFAILED状態に更新
- **進捗**: `keruta progress <percentage>` - 進捗率を更新

### 3. 成果物管理
- `/.keruta/doc`ディレクトリ配下のファイルを自動収集
- ファイルサイズ制限: 100MB（設定可能）
- サポート形式: テキスト、画像、PDF、ZIP等
- メタデータ付きでkeruta APIに送信

### 4. ログ管理
- 標準出力・標準エラー出力の自動キャプチャ
- 構造化ログ（JSON形式）のサポート
- ログレベル制御（DEBUG, INFO, WARN, ERROR）
- ログローテーション機能

### 5. エラーハンドリング
- 予期しないエラー発生時の自動検出
- エラー詳細の自動収集
- 自動修正タスクの作成（設定可能）
- リトライ機能（設定可能）

### 6. 監視・メトリクス
- 実行時間の計測
- リソース使用量の監視
- ヘルスチェック機能
- メトリクスのkeruta APIへの送信

### 7. 入力処理とデーモンモード
- **デーモンモード** - `keruta daemon` コマンドでバックグラウンド実行
- **HTTP API** - 外部からの入力受付（環境変数 `KERUTA_USE_HTTP_INPUT=true` で有効化）
- **入力待機** - タスク実行中の動的入力処理
- **自動検出** - 入力待ち状態の自動検出と通知

## タスク実行フロー

### 1. セッション監視とタスク取得
```
1. セッション状態の確認 (GET /api/v1/sessions/{sessionId})
2. PENDINGタスクの取得 (GET /api/v1/sessions/{sessionId}/tasks?status=PENDING)
3. タスクの優先度順ソート
4. 次に実行するタスクの決定
```

### 2. タスク実行プロセス
```
1. タスクステータスをPROCESSINGに更新
2. タスクスクリプトの実行開始
3. 進捗とログのリアルタイム送信
4. 成果物の自動収集
5. 完了時のステータス更新 (COMPLETED/FAILED)
6. 次のタスクへ移行
```

### 3. エラー処理とリトライ
```
1. エラー発生の検出
2. エラー詳細の収集とログ送信
3. タスクステータスをFAILEDに更新
4. 自動修正タスクの作成（オプション）
5. 次のタスクへ継続（エラー時も停止しない）
```

## コマンド仕様

### 基本コマンド

#### `keruta start`
タスクの実行を開始し、ステータスをPROCESSINGに更新します。

```bash
keruta start [options]
```

**オプション:**
- `--task-id <id>`: タスクIDを指定（環境変数から自動取得がデフォルト）
- `--api-url <url>`: keruta APIのURL（環境変数から自動取得がデフォルト）
- `--log-level <level>`: ログレベル（DEBUG, INFO, WARN, ERROR）

**例:**
```bash
#!/bin/bash
keruta start --log-level INFO
# タスク処理を実行
```

#### `keruta success`
タスクの成功を報告し、ステータスをCOMPLETEDに更新します。

```bash
keruta success [options]
```

**オプション:**
- `--message <message>`: 成功メッセージ
- `--artifacts-dir <path>`: 成果物ディレクトリ（デフォルト: `/.keruta/doc`）

**例:**
```bash
# 処理完了後
keruta success --message "データ処理が正常に完了しました"
```

#### `keruta fail`
タスクの失敗を報告し、ステータスをFAILEDに更新します。

```bash
keruta fail [options]
```

**オプション:**
- `--message <message>`: エラーメッセージ
- `--error-code <code>`: エラーコード
- `--auto-fix`: 自動修正タスクを作成するかどうか

**例:**
```bash
# エラー発生時
keruta fail --message "データベース接続に失敗しました" --error-code DB_CONNECTION_ERROR
```

#### `keruta progress`
タスクの進捗率を更新します。

```bash
keruta progress <percentage> [options]
```

**引数:**
- `percentage`: 進捗率（0-100）

**例:**
```bash
keruta progress 50 --message "データ処理中..."
```

#### `keruta daemon`
デーモンモードでkeruta-agentを起動し、セッションのタスクを自動実行します。

```bash
keruta daemon [options]
```

**オプション:**
- `--session-id <id>`: 監視するセッションID（環境変数から自動取得がデフォルト）
- `--port <port>`: HTTP APIのポート番号（デフォルト: 8080）
- `--host <host>`: HTTPサーバーのホスト（デフォルト: localhost）
- `--pid-file <file>`: PIDファイルのパス
- `--poll-interval <seconds>`: タスクポーリング間隔（デフォルト: 5秒）

**例:**
```bash
# セッションのタスクを自動実行
keruta daemon --session-id session-123

# カスタムポーリング間隔でデーモン起動
keruta daemon --poll-interval 10 --port 8080

# バックグラウンドで実行
nohup keruta daemon > /dev/null 2>&1 &
```

#### `keruta execute`
スクリプトやコマンドを実行します。

```bash
keruta execute <script> [options]
```

**引数:**
- `script`: 実行するスクリプトまたはコマンド

**例:**
```bash
keruta execute ./my-script.sh
keruta execute "python data_processing.py"
```

### ユーティリティコマンド

#### `keruta log`
構造化ログを送信します。

```bash
keruta log <level> <message> [options]
```

**引数:**
- `level`: ログレベル（DEBUG, INFO, WARN, ERROR）
- `message`: ログメッセージ

**例:**
```bash
keruta log INFO "データベースクエリを実行中..."
```

#### `keruta artifact`
成果物を手動で追加します。

```bash
keruta artifact add <file> [options]
keruta artifact list
keruta artifact remove <file>
```

**例:**
```bash
keruta artifact add ./output/report.pdf --description "月次レポート"
```

#### `keruta health`
ヘルスチェックを実行します。

```bash
keruta health [options]
```

**オプション:**
- `--check-api`: keruta APIとの接続確認
- `--check-disk`: ディスク容量確認
- `--check-memory`: メモリ使用量確認

#### `keruta config`
設定を表示・更新します。

```bash
keruta config show
keruta config set <key> <value>
```

## 環境変数

### 必須環境変数

| 変数名 | 説明 | 例 |
|--------|------|-----|
| `KERUTA_SESSION_ID` | 監視するセッションの一意識別子 | `session-123e4567-e89b-12d3-a456` |
| `KERUTA_WORKSPACE_ID` | ワークスペースの一意識別子 | `workspace-456` |
| `KERUTA_API_URL` | keruta APIのURL | `http://keruta-api:8080` |
| `KERUTA_API_TOKEN` | API認証トークン | `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...` |

### オプション環境変数

| 変数名 | 説明 | デフォルト値 |
|--------|------|-------------|
| `KERUTA_LOG_LEVEL` | ログレベル | `INFO` |
| `KERUTA_ARTIFACTS_DIR` | 成果物ディレクトリ | `/.keruta/doc` |
| `KERUTA_MAX_FILE_SIZE` | 最大ファイルサイズ（MB） | `100` |
| `KERUTA_AUTO_FIX_ENABLED` | 自動修正タスク作成 | `true` |
| `KERUTA_RETRY_COUNT` | リトライ回数 | `3` |
| `KERUTA_TIMEOUT` | API呼び出しタイムアウト（秒） | `30` |
| `KERUTA_USE_HTTP_INPUT` | HTTP入力機能の有効化 | `false` |
| `KERUTA_DAEMON_PORT` | デーモンHTTPポート | `8080` |
| `KERUTA_POLL_INTERVAL` | タスクポーリング間隔（秒） | `5` |
| `KERUTA_MAX_CONCURRENT_TASKS` | 最大同時実行タスク数（常に1） | `1` |
| `KERUTA_WORKING_DIR` | タスク実行時の作業ディレクトリ | 自動設定 |
| `KERUTA_BASE_DIR` | ベースディレクトリ | `$HOME/.keruta` または `/tmp/keruta` |
| `CODER_WORKSPACE_ID` | Coderワークスペース自動検出用 | 自動設定 |

## セットアップ

### 1. ビルド
```bash
# 依存関係のインストール
go mod download

# バイナリのビルド（基本）
go build -o keruta-agent ./cmd/keruta-agent

# ビルドスクリプトの使用（推奨）
./scripts/build.sh

# Dockerイメージのビルド
docker build -t keruta-agent:latest .
```

### 2. インストール
```bash
# システム全体にインストール
sudo cp keruta-agent /usr/local/bin/
sudo chmod +x /usr/local/bin/keruta-agent

# または、PATHに追加
export PATH=$PATH:/path/to/keruta-agent
```

### 3. 設定
```bash
# 設定ファイルの作成（オプション）
mkdir -p ~/.keruta
cat > ~/.keruta/config.yaml << EOF
api:
  url: http://keruta-api:8080
  timeout: 30s
logging:
  level: INFO
  format: json
artifacts:
  max_size: 100MB
  directory: /.keruta/doc
error_handling:
  auto_fix: true
  retry_count: 3
EOF
```

### 4. 実行確認
```bash
# ヘルプメッセージの表示
./keruta-agent --help

# バージョン情報の表示
./keruta-agent version
```

## 使用方法

### ワークスペース内でのデーモン起動例
```bash
#!/bin/bash
set -e

# 環境変数の設定
export KERUTA_SESSION_ID="session-123"
export KERUTA_WORKSPACE_ID="workspace-456"
export KERUTA_API_URL="http://keruta-api:8080"
export KERUTA_API_TOKEN="your-api-token"

# 1. デーモンとしてkeruta-agentを起動（セッションのタスクを自動実行）
keruta daemon --session-id $KERUTA_SESSION_ID --port 8080 &
DAEMON_PID=$!

# 2. デーモンが自動的にセッションのタスクを順次実行
# （手動でタスクを指定する必要なし）

# 3. セッション完了まで待機
wait $DAEMON_PID
```

### 手動タスク実行例
```bash
#!/bin/bash
set -e

# 1. デーモンとしてkeruta-agentを起動
keruta daemon --port 8080 &
DAEMON_PID=$!

# 2. 特定のタスクを手動実行
keruta execute "./data-processing-script.sh"

# 3. デーモンの停止
kill $DAEMON_PID
```

### 基本的な使用例
```bash
#!/bin/bash
set -e

# タスク開始
keruta start

# 進捗報告
keruta progress 25 --message "データの読み込み中..."

# 処理実行
python process_data.py

# 進捗報告
keruta progress 75 --message "データの処理中..."

# 成果物の作成
mkdir -p /.keruta/doc
echo "処理結果" > /.keruta/doc/result.txt

# タスク成功
keruta success --message "データ処理が完了しました"
```

### エラーハンドリング例
```bash
#!/bin/bash
set -e

keruta start

# エラーハンドリング
trap 'keruta fail --message "予期しないエラーが発生しました: $?"' ERR

# 処理実行
python risky_operation.py

keruta success
```

### 構造化ログの使用例
```bash
#!/bin/bash
keruta start

keruta log INFO "処理を開始します"
keruta log DEBUG "設定ファイルを読み込み中..."

# 処理実行
python main.py

keruta log INFO "処理が完了しました"
keruta success
```

## 技術スタック
- **言語**: Go 1.22+
- **フレームワーク**: Cobra（CLIフレームワーク）
- **HTTP クライアント**: net/http（標準ライブラリ）
- **設定管理**: Viper
- **ログ**: logrus
- **テスト**: testify

## 開発・デバッグ
### ローカル開発
```bash
# 開発用の実行
go run ./cmd/keruta-agent --help

# テストの実行
go test ./...

# テストカバレッジの確認
go test -cover ./...

# 静的解析
go vet ./...
go fmt ./...
```

### デバッグ
```bash
# デバッグモードでの実行
KERUTA_LOG_LEVEL=DEBUG ./keruta-agent --verbose <command>

# 設定の確認
./keruta-agent config show
```

## プロジェクト構造
```
keruta-agent/
├── cmd/
│   └── keruta-agent/          # メインエントリーポイント
│       └── main.go
├── internal/
│   ├── api/                   # keruta APIクライアント
│   │   ├── artifacts.go       # 成果物API
│   │   ├── client.go          # APIクライアント
│   │   ├── http_client.go     # HTTPクライアント
│   │   ├── input.go           # 入力API
│   │   ├── logging.go         # ログAPI
│   │   ├── retry.go           # リトライ機能
│   │   ├── script.go          # スクリプトAPI
│   │   └── task_status.go     # タスクステータスAPI
│   ├── commands/              # CLIコマンド実装
│   │   ├── artifact.go        # artifactコマンド
│   │   ├── config.go          # configコマンド
│   │   ├── daemon.go          # daemonコマンド
│   │   ├── execute.go         # executeコマンド
│   │   ├── fail.go            # failコマンド
│   │   ├── health.go          # healthコマンド
│   │   ├── log.go             # logコマンド
│   │   ├── progress.go        # progressコマンド
│   │   ├── root.go            # rootコマンド
│   │   ├── start.go           # startコマンド
│   │   └── success.go         # successコマンド
│   ├── config/                # 設定管理
│   │   └── config.go
│   └── logger/                # ログ機能
│       └── logger.go
├── pkg/
│   ├── artifacts/             # 成果物管理
│   │   └── manager.go
│   └── health/                # ヘルスチェック
│       └── checker.go
├── scripts/                   # ビルド・デプロイスクリプト
│   └── build.sh               # ビルドスクリプト
├── Dockerfile                 # Dockerイメージ定義
├── go.mod                     # Goモジュール定義
├── go.sum                     # 依存関係チェックサム
└── README.md                  # このファイル
```

## 関連リンク
- [keruta API仕様](../keruta-api/README.md) - メインAPI仕様
- [keruta Executor](../keruta-executor/README.md) - ワークスペース管理
- [プロジェクト全体のドキュメント](../CLAUDE.md) - 開発ガイド 
