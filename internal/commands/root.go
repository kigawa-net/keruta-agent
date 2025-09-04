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
			if err := os.Setenv("KERUTA_TASK_ID", taskID); err != nil {
				logger.WithTaskID().WithError(err).Error("環境変数KERUTA_TASK_IDの設定に失敗しました")
			}
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
	rootCmd.AddCommand(daemonCmd)

	// ヘルプテンプレートの設定
	rootCmd.SetHelpTemplate(`{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`)

	// 使用例の設定
	rootCmd.Example = `  # デーモンモードで起動
  keruta daemon

  # デーモンモードでポート指定
  keruta daemon --port 8080`
}
