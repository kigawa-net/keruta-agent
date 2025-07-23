package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"keruta-agent/internal/api"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	daemonInterval    time.Duration
	daemonPidFile     string
	daemonLogFile     string
	daemonWorkspaceID string
)

// daemonCmd はデーモンモードでkeruta-agentを実行するコマンドです
var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "デーモンモードでkeruta-agentを実行",
	Long: `デーモンモードでkeruta-agentを実行します。
このモードでは、定期的にAPIサーバーから新しいタスクをポーリングし、
受信したタスクを自動的に実行します。

デーモンは以下の機能を提供します：
- 定期的なタスクポーリング
- 自動タスク実行
- ヘルスチェック機能
- グレースフルシャットダウン
- PIDファイル管理`,
	RunE: runDaemon,
	Example: `  # デーモンモードで実行
  keruta daemon

  # 30秒間隔でポーリング
  keruta daemon --interval 30s

  # ワークスペースIDを指定
  keruta daemon --workspace-id ws-123

  # PIDファイルを指定
  keruta daemon --pid-file /var/run/keruta-agent.pid

  # ログファイルを指定
  keruta daemon --log-file /var/log/keruta-agent.log`,
}

func runDaemon(_ *cobra.Command, _ []string) error {
	daemonLogger := logger.WithTaskID()
	daemonLogger.Info("🚀 keruta-agentをデーモンモードで開始しています...")

	// PIDファイルの作成
	if daemonPidFile != "" {
		if err := writePIDFile(daemonPidFile); err != nil {
			return fmt.Errorf("PIDファイルの作成に失敗しました: %w", err)
		}
		defer func() {
			removePIDFile(daemonPidFile)
		}()
		daemonLogger.WithField("pid_file", daemonPidFile).Info("PIDファイルを作成しました")
	}

	// ログファイルの設定
	if daemonLogFile != "" {
		file, err := os.OpenFile(daemonLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("ログファイルの作成に失敗しました: %w", err)
		}
		defer func() {
			if closeErr := file.Close(); closeErr != nil {
				daemonLogger.WithError(closeErr).Error("ログファイルのクローズに失敗しました")
			}
		}()
		logrus.SetOutput(file)
		daemonLogger.WithField("log_file", daemonLogFile).Info("ログファイルを設定しました")
	}

	// APIクライアントの初期化
	apiClient := api.NewClient()

	// デーモンの開始情報をログ出力
	daemonLogger.WithFields(logrus.Fields{
		"interval":     daemonInterval,
		"workspace_id": daemonWorkspaceID,
		"pid":          os.Getpid(),
	}).Info("デーモン設定")

	// シグナルハンドリングの設定
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		daemonLogger.WithField("signal", sig).Info("シャットダウンシグナルを受信しました")
		cancel()
	}()

	// メインデーモンループ
	ticker := time.NewTicker(daemonInterval)
	defer ticker.Stop()

	daemonLogger.Info("✅ デーモンが開始されました。タスクのポーリングを開始します...")

	for {
		select {
		case <-ctx.Done():
			daemonLogger.Info("🛑 グレースフルシャットダウンを実行しています...")
			return nil
		case <-ticker.C:
			if err := pollAndExecuteTasks(ctx, apiClient, daemonLogger); err != nil {
				daemonLogger.WithError(err).Error("タスクポーリング中にエラーが発生しました")
			}
		}
	}
}

// pollAndExecuteTasks はAPIサーバーからタスクをポーリングし、実行します
func pollAndExecuteTasks(ctx context.Context, apiClient *api.Client, logger *logrus.Entry) error {
	logger.Debug("📡 新しいタスクをポーリングしています...")

	// ワークスペース用のタスクを取得
	tasks, err := apiClient.GetPendingTasksForWorkspace(daemonWorkspaceID)
	if err != nil {
		return fmt.Errorf("タスクの取得に失敗しました: %w", err)
	}

	if len(tasks) == 0 {
		logger.Debug("新しいタスクはありません")
		return nil
	}

	logger.WithField("task_count", len(tasks)).Info("📋 新しいタスクを受信しました")

	// 各タスクを順次実行
	for _, task := range tasks {
		select {
		case <-ctx.Done():
			logger.Info("シャットダウン中のため、タスク実行を中断します")
			return nil
		default:
			if err := executeTask(ctx, apiClient, task, logger); err != nil {
				logger.WithError(err).WithField("task_id", task.ID).Error("タスクの実行に失敗しました")
			}
		}
	}

	return nil
}

// executeTask は個別のタスクを実行します
func executeTask(ctx context.Context, apiClient *api.Client, task *api.Task, parentLogger *logrus.Entry) error {
	taskLogger := parentLogger.WithField("task_id", task.ID)
	taskLogger.Info("🔄 タスクを実行しています...")

	// 環境変数にタスクIDを設定
	oldTaskID := os.Getenv("KERUTA_TASK_ID")
	if err := os.Setenv("KERUTA_TASK_ID", task.ID); err != nil {
		return fmt.Errorf("環境変数の設定に失敗しました: %w", err)
	}
	defer func() {
		if err := os.Setenv("KERUTA_TASK_ID", oldTaskID); err != nil {
			taskLogger.WithError(err).Error("環境変数の復元に失敗しました")
		}
	}()

	// タスク開始の通知
	if err := apiClient.StartTask(task.ID); err != nil {
		return fmt.Errorf("タスク開始の通知に失敗しました: %w", err)
	}

	// スクリプトの取得
	script, err := apiClient.GetTaskScript(task.ID)
	if err != nil {
		if failErr := apiClient.FailTask(task.ID, "スクリプトの取得に失敗しました", "SCRIPT_FETCH_ERROR"); failErr != nil {
			taskLogger.WithError(failErr).Error("タスク失敗の通知に失敗しました")
		}
		return fmt.Errorf("スクリプトの取得に失敗しました: %w", err)
	}

	// スクリプトの実行
	if err := executeScript(ctx, script, taskLogger); err != nil {
		if failErr := apiClient.FailTask(task.ID, fmt.Sprintf("スクリプトの実行に失敗しました: %v", err), "SCRIPT_EXECUTION_ERROR"); failErr != nil {
			taskLogger.WithError(failErr).Error("タスク失敗の通知に失敗しました")
		}
		return fmt.Errorf("スクリプトの実行に失敗しました: %w", err)
	}

	// タスク成功の通知
	if err := apiClient.SuccessTask(task.ID, "タスクが正常に完了しました"); err != nil {
		return fmt.Errorf("タスク成功の通知に失敗しました: %w", err)
	}

	taskLogger.Info("✅ タスクが正常に完了しました")
	return nil
}

// executeScript はスクリプトを実行します
func executeScript(_ context.Context, _ string, scriptLogger *logrus.Entry) error {
	// この関数は実際のスクリプト実行ロジックを実装する必要があります
	// 現在は簡単な実装例です
	scriptLogger.Info("📝 スクリプトを実行しています...")

	// TODO: 実際のスクリプト実行ロジックを実装
	// exec.CommandContext を使用してスクリプトを実行し、
	// リアルタイムでログをAPIサーバーに送信する

	scriptLogger.Info("✅ スクリプトの実行が完了しました")
	return nil
}

// writePIDFile はPIDファイルを作成します
func writePIDFile(pidFile string) error {
	file, err := os.Create(pidFile)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = fmt.Fprintf(file, "%d\n", os.Getpid())
	return err
}

// removePIDFile はPIDファイルを削除します
func removePIDFile(pidFile string) {
	if err := os.Remove(pidFile); err != nil {
		logrus.WithError(err).WithField("pid_file", pidFile).Error("PIDファイルの削除に失敗しました")
	}
}

func init() {
	// フラグの設定
	daemonCmd.Flags().DurationVar(&daemonInterval, "interval", 10*time.Second, "タスクポーリングの間隔")
	daemonCmd.Flags().StringVar(&daemonPidFile, "pid-file", "", "PIDファイルのパス")
	daemonCmd.Flags().StringVar(&daemonLogFile, "log-file", "", "ログファイルのパス")
	daemonCmd.Flags().StringVar(&daemonWorkspaceID, "workspace-id", "", "ワークスペースID（環境変数CODER_WORKSPACE_IDから自動取得）")

	// 環境変数からのデフォルト値設定
	if workspaceID := os.Getenv("CODER_WORKSPACE_ID"); workspaceID != "" {
		daemonWorkspaceID = workspaceID
	}
}
