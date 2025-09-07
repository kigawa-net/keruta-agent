package commands

import (
	"context"
	"fmt"
	"io"
	"keruta-agent/internal/api"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

func executeClaudeTask(ctx context.Context, apiClient *api.Client, taskID string, taskContent *io.PipeReader, taskLogger *logrus.Entry) error {
	taskLogger.Info("🎯 環境でClaude実行タスクを開始しています...")

	// ~/keruta ディレクトリの存在を確認・作成
	kerutaDir := os.ExpandEnv("$HOME/keruta")
	if err := ensureDirectory(kerutaDir); err != nil {
		return fmt.Errorf("~/kerutaディレクトリの作成に失敗: %w", err)
	}

	taskLogger.WithFields(logrus.Fields{
		"working_dir": kerutaDir,
	}).Info("セッションでClaude実行を開始します")

	// コマンドを構築 - セッション作成、ディレクトリ移動、Claude実行
	Cmd := exec.CommandContext(ctx, "claude", "--dangerously-skip-permissions")
	Cmd.Stdin = taskContent
	Cmd.Dir = kerutaDir

	taskLogger.WithFields(logrus.Fields{
		"working_dir": kerutaDir,
		"command":     Cmd.Args,
	}).Info("🖥️ コマンドを構築しました")

	// コマンド実行とログ収集
	return executeCommand(Cmd, apiClient, taskID, taskLogger)
}

// ensureDirectory はディレクトリの存在を確認し、存在しない場合は作成します
func ensureDirectory(dirPath string) error {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return os.MkdirAll(dirPath, 0755)
	}
	return nil
}

func executeCommand(cmd *exec.Cmd, apiClient *api.Client, taskID string, logger *logrus.Entry) error {
	logger.Info("🚀セッションを起動しています...")

	// セッション開始
	logger.WithFields(logrus.Fields{
		"command": strings.Join(cmd.Args, " "),
	}).Info("⚡ セッションを開始します")
	// コマンドの標準出力・標準エラーをキャプチャ
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := strings.TrimSpace(string(output))

		// APIにもエラー出力を送信
		if len(output) > 0 {
			logMessage := fmt.Sprintf("[:start-cmd] %s", outputStr)
			if sendErr := apiClient.SendLog(taskID, "ERROR", logMessage); sendErr != nil {
				logger.WithError(sendErr).Warning("開始エラーログ送信に失敗しました")
			}
		}

		return fmt.Errorf("セッション開始に失敗: %w", err)
	}

	if len(output) > 0 {
		logger.WithFields(logrus.Fields{
			"output": strings.TrimSpace(string(output)),
		}).Info("📋 ")

		// APIにもログ送信
		logMessage := fmt.Sprintf("[:start-cmd] %s", strings.TrimSpace(string(output)))
		if sendErr := apiClient.SendLog(taskID, "INFO", logMessage); sendErr != nil {
			logger.WithError(sendErr).Warning("ログ送信に失敗しました")
		}
	} else {

		// セッション開始成功をAPIにログ送信
		logMessage := "[:start-cmd] コマンド実行完了"
		if sendErr := apiClient.SendLog(taskID, "INFO", logMessage); sendErr != nil {
			logger.WithError(sendErr).Warning("ログ送信に失敗しました")
		}
	}
	logger.Info("✅ Claude実行タスクが完了しました")
	return nil
}
