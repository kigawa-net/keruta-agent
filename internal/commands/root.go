package commands

import (
	"os"

	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	// グローバルフラグ
	verbose bool
	taskID  string
)

// rootCmd はルートコマンドです
var rootCmd = &cobra.Command{
	Use:   "keruta",
	Short: "keruta-agent - Kubernetes Pod内でタスクを実行するCLIツール",
	Long: `keruta-agentは、kerutaシステムによってKubernetes Jobとして実行されるPod内で動作するCLIツールです。
タスクの実行状況をkeruta APIサーバーに報告し、成果物の保存、ログの収集、エラーハンドリングなどの機能を提供します。`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// ログレベルの設定
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}

		// タスクIDの設定
		if taskID != "" {
			os.Setenv("KERUTA_TASK_ID", taskID)
		}

		logger.WithTaskID().Debug("keruta-agentを開始しました")
	},
}

// Execute はルートコマンドを実行します
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// グローバルフラグの設定
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "詳細ログを出力")
	rootCmd.PersistentFlags().StringVar(&taskID, "task-id", "", "タスクID（環境変数KERUTA_TASK_IDから自動取得）")

	// サブコマンドの追加
	rootCmd.AddCommand(executeCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(successCmd)
	rootCmd.AddCommand(failCmd)
	rootCmd.AddCommand(progressCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(artifactCmd)
	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(configCmd)

	// ヘルプテンプレートの設定
	rootCmd.SetHelpTemplate(`{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`)

	// 使用例の設定
	rootCmd.Example = `  # スクリプトの実行
  keruta execute --task-id task123

  # タスクの開始
  keruta start

  # 進捗の報告
  keruta progress 50 --message "データ処理中..."

  # タスクの成功
  keruta success --message "処理が完了しました"

  # タスクの失敗
  keruta fail --message "エラーが発生しました" --error-code DB_ERROR

  # ログの送信
  keruta log INFO "処理を開始します"

  # ヘルスチェック
  keruta health`
}
