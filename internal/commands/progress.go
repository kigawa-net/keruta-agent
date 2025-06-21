package commands

import (
	"fmt"
	"strconv"

	"keruta-agent/internal/api"
	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	progressMessage string
)

// progressCmd は進捗報告コマンドです
var progressCmd = &cobra.Command{
	Use:   "progress [percentage]",
	Short: "タスクの進捗率を更新",
	Long: `タスクの進捗率を更新します。
進捗率は0から100の間の整数で指定してください。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProgress(args[0])
	},
}

func init() {
	progressCmd.Flags().StringVar(&progressMessage, "message", "", "進捗メッセージ")
}

// runProgress はprogressコマンドの実行ロジックです
func runProgress(percentageStr string) error {
	taskID := config.GetTaskID()
	if taskID == "" {
		return fmt.Errorf("タスクIDが設定されていません。--task-idフラグまたはKERUTA_TASK_ID環境変数を設定してください")
	}

	// 進捗率の解析
	percentage, err := strconv.Atoi(percentageStr)
	if err != nil {
		return fmt.Errorf("進捗率は整数で指定してください: %s", percentageStr)
	}

	// 進捗率の範囲チェック
	if percentage < 0 || percentage > 100 {
		return fmt.Errorf("進捗率は0から100の間で指定してください: %d", percentage)
	}

	logger.WithTaskIDAndComponent("progress").WithFields(logrus.Fields{
		"percentage": percentage,
		"message":    progressMessage,
	}).Info("進捗を更新します")

	// APIクライアントの作成
	client := api.NewClient()

	// タスクステータスを更新（進捗率のみ）
	err = client.UpdateTaskStatus(
		taskID,
		api.TaskStatusProcessing, // 進捗更新時はPROCESSINGのまま
		progressMessage,
		percentage,
		"",
	)
	if err != nil {
		logger.WithTaskIDAndComponent("progress").WithError(err).Error("進捗の更新に失敗しました")
		return fmt.Errorf("進捗の更新に失敗: %w", err)
	}

	// 進捗ログを送信
	if progressMessage != "" {
		err = client.SendLog(taskID, "INFO", fmt.Sprintf("進捗: %d%% - %s", percentage, progressMessage))
	} else {
		err = client.SendLog(taskID, "INFO", fmt.Sprintf("進捗: %d%%", percentage))
	}
	if err != nil {
		logger.WithTaskIDAndComponent("progress").WithError(err).Warn("進捗ログの送信に失敗しました")
		// ログ送信の失敗は致命的ではないので、エラーを返さない
	}

	logger.WithTaskIDAndComponent("progress").WithField("percentage", percentage).Info("進捗を更新しました")
	return nil
} 