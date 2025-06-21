package commands

import (
	"fmt"
	"strings"

	"keruta-agent/internal/api"
	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// logCmd はログ送信コマンドです
var logCmd = &cobra.Command{
	Use:   "log [level] [message]",
	Short: "構造化ログを送信",
	Long: `構造化ログをkeruta APIに送信します。
ログレベルは DEBUG, INFO, WARN, ERROR のいずれかを指定してください。`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLog(args[0], args[1])
	},
}

// runLog はlogコマンドの実行ロジックです
func runLog(level string, message string) error {
	taskID := config.GetTaskID()
	if taskID == "" {
		return fmt.Errorf("タスクIDが設定されていません。--task-idフラグまたはKERUTA_TASK_ID環境変数を設定してください")
	}

	// ログレベルの正規化
	level = strings.ToUpper(level)

	// ログレベルの検証
	validLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
	}

	if !validLevels[level] {
		return fmt.Errorf("無効なログレベルです: %s (有効な値: DEBUG, INFO, WARN, ERROR)", level)
	}

	logger.WithTaskIDAndComponent("log").WithFields(logrus.Fields{
		"level":   level,
		"message": message,
	}).Debug("ログを送信中")

	// APIクライアントの作成
	client := api.NewClient()

	// ログを送信
	err := client.SendLog(taskID, level, message)
	if err != nil {
		logger.WithTaskIDAndComponent("log").WithError(err).Error("ログの送信に失敗しました")
		return fmt.Errorf("ログの送信に失敗: %w", err)
	}

	logger.WithTaskIDAndComponent("log").WithFields(logrus.Fields{
		"level":   level,
		"message": message,
	}).Info("ログを送信しました")
	return nil
} 