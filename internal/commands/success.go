package commands

import (
	"fmt"

	"keruta-agent/internal/api"
	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"
	"keruta-agent/pkg/artifacts"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	successMessage string
	artifactsDir   string
)

// successCmd はタスク成功コマンドです
var successCmd = &cobra.Command{
	Use:   "success",
	Short: "タスクの成功を報告し、ステータスをCOMPLETEDに更新",
	Long: `タスクの成功を報告し、ステータスをCOMPLETEDに更新します。
成果物ディレクトリから成果物を自動収集してアップロードします。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSuccess()
	},
}

func init() {
	successCmd.Flags().StringVar(&successMessage, "message", "タスクが正常に完了しました", "成功メッセージ")
	successCmd.Flags().StringVar(&artifactsDir, "artifacts-dir", "", "成果物ディレクトリ（デフォルト: 設定ファイルの値）")
}

// runSuccess はsuccessコマンドの実行ロジックです
func runSuccess() error {
	taskID := config.GetTaskID()
	if taskID == "" {
		return fmt.Errorf("タスクIDが設定されていません。--task-idフラグまたはKERUTA_TASK_ID環境変数を設定してください")
	}

	logger.WithTaskIDAndComponent("success").WithField("message", successMessage).Info("タスクを成功として完了します")

	// APIクライアントの作成
	client := api.NewClient()

	// 成果物の収集とアップロード
	if err := uploadArtifacts(client, taskID, artifactsDir); err != nil {
		logger.WithTaskIDAndComponent("success").WithError(err).Warn("成果物のアップロードに失敗しました")
		// 成果物アップロードの失敗は致命的ではないので、処理を続行
	}

	// タスクステータスをCOMPLETEDに更新
	err := client.UpdateTaskStatus(
		taskID,
		api.TaskStatusCompleted,
		successMessage,
		100,
		"",
	)
	if err != nil {
		logger.WithTaskIDAndComponent("success").WithError(err).Error("タスクステータスの更新に失敗しました")
		return fmt.Errorf("タスクステータスの更新に失敗: %w", err)
	}

	// 完了ログを送信
	err = client.SendLog(taskID, "INFO", fmt.Sprintf("タスクが正常に完了しました: %s", successMessage))
	if err != nil {
		logger.WithTaskIDAndComponent("success").WithError(err).Warn("完了ログの送信に失敗しました")
		// ログ送信の失敗は致命的ではないので、エラーを返さない
	}

	logger.WithTaskIDAndComponent("success").Info("タスクを正常に完了しました")
	return nil
}

// uploadArtifacts は成果物を収集してアップロードします
func uploadArtifacts(client *api.Client, taskID string, artifactsDir string) error {
	// 成果物マネージャーの作成
	manager := artifacts.NewManager()

	// 成果物ディレクトリの設定
	if artifactsDir != "" {
		manager.Directory = artifactsDir
	}

	// 成果物の収集
	artifacts, err := manager.CollectArtifacts()
	if err != nil {
		return fmt.Errorf("成果物の収集に失敗: %w", err)
	}

	if len(artifacts) == 0 {
		logger.WithTaskIDAndComponent("success").Info("アップロードする成果物がありません")
		return nil
	}

	// 成果物のアップロード
	for _, artifact := range artifacts {
		description := manager.GetArtifactDescription(artifact)
		
		err := client.UploadArtifact(taskID, artifact.Path, description)
		if err != nil {
			logger.WithTaskIDAndComponent("success").WithError(err).WithField("file", artifact.Path).Warn("成果物のアップロードに失敗しました")
			continue
		}

		logger.WithTaskIDAndComponent("success").WithField("file", artifact.Path).Debug("成果物をアップロードしました")
	}

	logger.WithTaskIDAndComponent("success").WithField("count", len(artifacts)).Info("成果物のアップロードが完了しました")
	return nil
} 