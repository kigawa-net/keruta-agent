package commands

import (
	"fmt"
	"os"

	"keruta-agent/internal/api"
	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"
	"keruta-agent/pkg/artifacts"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	artifactDescription string
)

// artifactCmd は成果物管理コマンドです
var artifactCmd = &cobra.Command{
	Use:   "artifact",
	Short: "成果物の管理",
	Long:  `成果物の追加、一覧表示、削除を行います。`,
}

// artifactAddCmd は成果物追加コマンドです
var artifactAddCmd = &cobra.Command{
	Use:   "add [file]",
	Short: "成果物を手動で追加",
	Long:  `指定されたファイルを成果物として手動で追加します。`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runArtifactAdd(args[0])
	},
}

// artifactListCmd は成果物一覧コマンドです
var artifactListCmd = &cobra.Command{
	Use:   "list",
	Short: "成果物の一覧を表示",
	Long:  `成果物ディレクトリ内のファイル一覧を表示します。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runArtifactList()
	},
}

// artifactRemoveCmd は成果物削除コマンドです
var artifactRemoveCmd = &cobra.Command{
	Use:   "remove [file]",
	Short: "成果物を削除",
	Long:  `指定された成果物を削除します。`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runArtifactRemove(args[0])
	},
}

func init() {
	artifactAddCmd.Flags().StringVar(&artifactDescription, "description", "", "成果物の説明")
	
	artifactCmd.AddCommand(artifactAddCmd)
	artifactCmd.AddCommand(artifactListCmd)
	artifactCmd.AddCommand(artifactRemoveCmd)
}

// runArtifactAdd は成果物追加の実行ロジックです
func runArtifactAdd(filePath string) error {
	taskID := config.GetTaskID()
	if taskID == "" {
		return fmt.Errorf("タスクIDが設定されていません。--task-idフラグまたはKERUTA_TASK_ID環境変数を設定してください")
	}

	logger.WithTaskIDAndComponent("artifact").WithFields(logrus.Fields{
		"file":        filePath,
		"description": artifactDescription,
	}).Info("成果物を追加中")

	// 成果物マネージャーの作成
	manager := artifacts.NewManager()

	// 成果物の妥当性チェック
	if err := manager.ValidateArtifact(filePath); err != nil {
		logger.WithTaskIDAndComponent("artifact").WithError(err).Error("成果物の妥当性チェックに失敗しました")
		return fmt.Errorf("成果物の妥当性チェックに失敗: %w", err)
	}

	// APIクライアントの作成
	client := api.NewClient()

	// 成果物をアップロード
	err := client.UploadArtifact(taskID, filePath, artifactDescription)
	if err != nil {
		logger.WithTaskIDAndComponent("artifact").WithError(err).Error("成果物のアップロードに失敗しました")
		return fmt.Errorf("成果物のアップロードに失敗: %w", err)
	}

	logger.WithTaskIDAndComponent("artifact").WithField("file", filePath).Info("成果物を追加しました")
	return nil
}

// runArtifactList は成果物一覧の実行ロジックです
func runArtifactList() error {
	logger.WithComponent("artifact").Info("成果物一覧を取得中")

	// 成果物マネージャーの作成
	manager := artifacts.NewManager()

	// 成果物の収集
	artifacts, err := manager.CollectArtifacts()
	if err != nil {
		logger.WithComponent("artifact").WithError(err).Error("成果物の収集に失敗しました")
		return fmt.Errorf("成果物の収集に失敗: %w", err)
	}

	if len(artifacts) == 0 {
		fmt.Println("成果物がありません")
		return nil
	}

	// 成果物一覧の表示
	fmt.Printf("成果物一覧 (%d件):\n", len(artifacts))
	fmt.Println("----------------------------------------")
	for i, artifact := range artifacts {
		description := manager.GetArtifactDescription(artifact)
		fmt.Printf("%d. %s (%s, %d bytes)\n", i+1, artifact.Name, description, artifact.Size)
	}

	logger.WithComponent("artifact").WithField("count", len(artifacts)).Info("成果物一覧を表示しました")
	return nil
}

// runArtifactRemove は成果物削除の実行ロジックです
func runArtifactRemove(filePath string) error {
	logger.WithComponent("artifact").WithField("file", filePath).Info("成果物を削除中")

	// 成果物マネージャーの作成
	manager := artifacts.NewManager()

	// ファイルの存在確認
	if err := manager.ValidateArtifact(filePath); err != nil {
		logger.WithComponent("artifact").WithError(err).Error("ファイルの存在確認に失敗しました")
		return fmt.Errorf("ファイルの存在確認に失敗: %w", err)
	}

	// ファイルの削除
	if err := os.Remove(filePath); err != nil {
		logger.WithComponent("artifact").WithError(err).Error("ファイルの削除に失敗しました")
		return fmt.Errorf("ファイルの削除に失敗: %w", err)
	}

	logger.WithComponent("artifact").WithField("file", filePath).Info("成果物を削除しました")
	return nil
} 