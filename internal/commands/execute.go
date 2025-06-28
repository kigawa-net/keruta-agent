package commands

import (
	"fmt"
	"os"

	"keruta-agent/internal/api"
	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	// executeCmd flags
	apiURL          string
	workDir         string
	logLevel        string
	autoDetectInput bool
	timeout         int
)

// executeCmd はタスク実行コマンドです
var executeCmd = &cobra.Command{
	Use:   "execute",
	Short: "指定されたタスクIDのスクリプトを実行",
	Long: `指定されたタスクIDのスクリプトをkeruta APIから取得し、サブプロセスとして実行します。
WebSocket通信により、タスク状態や標準入力、ログなどをリアルタイムで連携します。
入力待ち状態を自動検出します。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runExecute()
	},
	Example: `  # 基本的な実行
  keruta execute --task-id task123

  # カスタム設定での実行
  keruta execute \
      --task-id task123 \
      --api-url http://keruta-api:8080 \
      --work-dir /work \
      --log-level DEBUG

  # タイムアウト付きで実行
  keruta execute \
      --timeout 300 \
      --task-id task123`,
}

func init() {
	// フラグの設定
	executeCmd.Flags().StringVar(&apiURL, "api-url", "", "keruta APIのURL（環境変数KERUTA_API_URLから自動取得）")
	executeCmd.Flags().StringVar(&workDir, "work-dir", "/work", "作業ディレクトリ")
	executeCmd.Flags().StringVar(&logLevel, "log-level", "INFO", "ログレベル（DEBUG, INFO, WARN, ERROR）")
	executeCmd.Flags().BoolVar(&autoDetectInput, "auto-detect-input", true, "入力待ち状態の自動検出")
	executeCmd.Flags().IntVar(&timeout, "timeout", 0, "サブプロセスのタイムアウト時間（秒）、0は無制限")
}

// runExecute はexecuteコマンドの実行ロジックです
func runExecute() error {
	taskID := config.GetTaskID()
	if taskID == "" {
		return fmt.Errorf("タスクIDが設定されていません。--task-idフラグまたはKERUTA_TASK_ID環境変数を設定してください")
	}

	logger.WithTaskIDAndComponent("execute").Info("タスク実行を開始します")

	// APIクライアントの作成
	if apiURL != "" {
		os.Setenv("KERUTA_API_URL", apiURL)
	}
	client := api.NewClient()

	// ログレベルの設定
	setLogLevel(logLevel)

	// 作業ディレクトリの作成
	if err := os.MkdirAll(workDir, 0755); err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Error("作業ディレクトリの作成に失敗しました")
		return fmt.Errorf("作業ディレクトリの作成に失敗: %w", err)
	}

	// タスクステータスをPROCESSINGに更新
	err := client.UpdateTaskStatus(
		taskID,
		api.TaskStatusProcessing,
		"タスクを実行中",
		0,
		"",
	)
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Error("タスクステータスの更新に失敗しました")
		return fmt.Errorf("タスクステータスの更新に失敗: %w", err)
	}

	// スクリプトの取得とサブプロセスの実行は実際の実装では以下のようなステップが必要
	// 1. APIからスクリプトを取得
	// 2. WebSocketで接続
	// 3. スクリプトをファイルに保存
	// 4. サブプロセスとして実行
	// 5. 標準出力・標準エラー出力をキャプチャしてWebSocketで送信
	// 6. 入力待ち状態を検出してタスク状態を更新
	// 7. WebSocketから標準入力を受信してサブプロセスに送信
	// 8. サブプロセスの終了状態に応じてタスク状態を更新

	// この実装ではプレースホルダーとして簡易的な処理を行います
	logger.WithTaskIDAndComponent("execute").Info("スクリプトの実行を開始します")

	// 実行完了後、タスクステータスをCOMPLETEDに更新
	err = client.UpdateTaskStatus(
		taskID,
		api.TaskStatusCompleted,
		"タスクが正常に完了しました",
		100,
		"",
	)
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Error("タスクステータスの更新に失敗しました")
		return fmt.Errorf("タスクステータスの更新に失敗: %w", err)
	}

	logger.WithTaskIDAndComponent("execute").Info("タスク実行が完了しました")
	return nil
}

// setLogLevel はログレベルを設定します
func setLogLevel(level string) {
	switch level {
	case "DEBUG":
		logrus.SetLevel(logrus.DebugLevel)
	case "INFO":
		logrus.SetLevel(logrus.InfoLevel)
	case "WARN":
		logrus.SetLevel(logrus.WarnLevel)
	case "ERROR":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}
}
