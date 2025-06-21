package commands

import (
	"fmt"
	"time"

	"keruta-agent/internal/api"
	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	startMessage string
)

// startCmd はタスク開始コマンドです
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "タスクの実行を開始し、ステータスをPROCESSINGに更新",
	Long: `タスクの実行を開始し、ステータスをPROCESSINGに更新します。
このコマンドは、タスク処理の最初に呼び出される必要があります。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStart()
	},
}

func init() {
	startCmd.Flags().StringVar(&startMessage, "message", "タスクを開始しました", "開始メッセージ")
}

// runStart はstartコマンドの実行ロジックです
func runStart() error {
	taskID := config.GetTaskID()
	if taskID == "" {
		return fmt.Errorf("タスクIDが設定されていません。--task-idフラグまたはKERUTA_TASK_ID環境変数を設定してください")
	}

	logger.WithTaskIDAndComponent("start").WithField("message", startMessage).Info("タスクを開始します")

	// APIクライアントの作成
	client := api.NewClient()

	// タスクステータスをPROCESSINGに更新
	err := client.UpdateTaskStatus(
		taskID,
		api.TaskStatusProcessing,
		startMessage,
		0,
		"",
	)
	if err != nil {
		logger.WithTaskIDAndComponent("start").WithError(err).Error("タスクステータスの更新に失敗しました")
		return fmt.Errorf("タスクステータスの更新に失敗: %w", err)
	}

	// 開始ログを送信
	err = client.SendLog(taskID, "INFO", fmt.Sprintf("タスクを開始しました: %s", startMessage))
	if err != nil {
		logger.WithTaskIDAndComponent("start").WithError(err).Warn("開始ログの送信に失敗しました")
		// ログ送信の失敗は致命的ではないので、エラーを返さない
	}

	logger.WithTaskIDAndComponent("start").Info("タスクを開始しました")
	return nil
} 