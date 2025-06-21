package commands

import (
	"fmt"

	"keruta-agent/internal/api"
	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	failMessage string
	errorCode   string
	autoFix     bool
)

// failCmd はタスク失敗コマンドです
var failCmd = &cobra.Command{
	Use:   "fail",
	Short: "タスクの失敗を報告し、ステータスをFAILEDに更新",
	Long: `タスクの失敗を報告し、ステータスをFAILEDに更新します。
オプションで自動修正タスクの作成も行います。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFail()
	},
}

func init() {
	failCmd.Flags().StringVar(&failMessage, "message", "タスクが失敗しました", "エラーメッセージ")
	failCmd.Flags().StringVar(&errorCode, "error-code", "", "エラーコード")
	failCmd.Flags().BoolVar(&autoFix, "auto-fix", false, "自動修正タスクを作成するかどうか")
}

// runFail はfailコマンドの実行ロジックです
func runFail() error {
	taskID := config.GetTaskID()
	if taskID == "" {
		return fmt.Errorf("タスクIDが設定されていません。--task-idフラグまたはKERUTA_TASK_ID環境変数を設定してください")
	}

	logger.WithTaskIDAndComponent("fail").WithFields(logrus.Fields{
		"message":   failMessage,
		"errorCode": errorCode,
		"autoFix":   autoFix,
	}).Info("タスクを失敗として完了します")

	// APIクライアントの作成
	client := api.NewClient()

	// 自動修正タスクの作成
	if autoFix && config.GlobalConfig.ErrorHandling.AutoFix {
		if err := createAutoFixTask(client, taskID, failMessage, errorCode); err != nil {
			logger.WithTaskIDAndComponent("fail").WithError(err).Warn("自動修正タスクの作成に失敗しました")
			// 自動修正タスク作成の失敗は致命的ではないので、処理を続行
		}
	}

	// タスクステータスをFAILEDに更新
	err := client.UpdateTaskStatus(
		taskID,
		api.TaskStatusFailed,
		failMessage,
		0,
		errorCode,
	)
	if err != nil {
		logger.WithTaskIDAndComponent("fail").WithError(err).Error("タスクステータスの更新に失敗しました")
		return fmt.Errorf("タスクステータスの更新に失敗: %w", err)
	}

	// 失敗ログを送信
	err = client.SendLog(taskID, "ERROR", fmt.Sprintf("タスクが失敗しました: %s", failMessage))
	if err != nil {
		logger.WithTaskIDAndComponent("fail").WithError(err).Warn("失敗ログの送信に失敗しました")
		// ログ送信の失敗は致命的ではないので、エラーを返さない
	}

	logger.WithTaskIDAndComponent("fail").Info("タスクを失敗として完了しました")
	return nil
}

// createAutoFixTask は自動修正タスクを作成します
func createAutoFixTask(client *api.Client, taskID string, errorMessage string, errorCode string) error {
	logger.WithTaskIDAndComponent("fail").WithFields(logrus.Fields{
		"errorMessage": errorMessage,
		"errorCode":    errorCode,
	}).Info("自動修正タスクを作成中")

	err := client.CreateAutoFixTask(taskID, errorMessage, errorCode)
	if err != nil {
		return fmt.Errorf("自動修正タスクの作成に失敗: %w", err)
	}

	logger.WithTaskIDAndComponent("fail").Info("自動修正タスクを作成しました")
	return nil
} 