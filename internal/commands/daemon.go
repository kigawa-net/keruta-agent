package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"keruta-agent/internal/api"
	"keruta-agent/internal/git"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	daemonInterval     time.Duration
	daemonPidFile      string
	daemonLogFile      string
	daemonWorkspaceID  string
	daemonSessionID    string
	daemonPollInterval time.Duration
)

// daemonCmd はデーモンモードでkeruta-agentを実行するコマンドです
var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "デーモンモードでkeruta-agentを実行",
	Long: `デーモンモードでkeruta-agentを実行します。
このモードでは、セッションに対応するタスクを定期的にポーリングし、
受信したタスクを一つずつ順次実行します。

デーモンは以下の機能を提供します：
- セッション監視とタスクポーリング
- タスクの順次実行（並列実行なし）
- 自動エラーハンドリング
- ヘルスチェック機能
- グレースフルシャットダウン
- PIDファイル管理`,
	RunE: runDaemon,
	Example: `  # セッションのタスクを自動実行
  keruta daemon --session-id session-123

  # 30秒間隔でポーリング
  keruta daemon --poll-interval 30s

  # ワークスペースIDとセッションIDを指定
  keruta daemon --session-id session-123 --workspace-id ws-123

  # PIDファイルを指定
  keruta daemon --pid-file /var/run/keruta-agent.pid

  # ログファイルを指定
  keruta daemon --log-file /var/log/keruta-agent.log

  # Coderワークスペース内で自動実行（ワークスペース名から自動でセッションIDを検出）
  keruta daemon  # CODER_WORKSPACE_NAME環境変数またはホスト名から自動取得`,
}

func runDaemon(_ *cobra.Command, _ []string) error {
	daemonLogger := logger.WithTaskID()
	daemonLogger.Info("🚀 keruta-agentをデーモンモードで開始しています...")

	// PIDファイルの作成
	if daemonPidFile != "" {
		if err := writePIDFile(daemonPidFile); err != nil {
			return fmt.Errorf("PID file creation failed: %w", err)
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
			return fmt.Errorf("log file creation failed: %w", err)
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

	// ログのAPI送信を有効化
	logger.SetAPIClient(apiClient)

	// Gitコマンドの利用可能性を確認
	if err := git.ValidateGitCommand(); err != nil {
		daemonLogger.WithError(err).Warn("Gitコマンドが利用できません。リポジトリ機能は無効になります")
	}

	// セッションの情報を取得してGitリポジトリを初期化
	if daemonSessionID != "" {
		// 部分的なIDから完全なUUIDを取得
		fullSessionID := resolveFullSessionID(apiClient, daemonSessionID, daemonLogger)
		if fullSessionID != daemonSessionID {
			// 完全なUUIDが取得できた場合、更新する
			daemonSessionID = fullSessionID
		}

		if err := initializeRepositoryForSession(apiClient, daemonSessionID, daemonLogger); err != nil {
			daemonLogger.WithError(err).Error("リポジトリの初期化に失敗しました")
		}
	}

	// デーモンの開始情報をログ出力
	daemonLogger.WithFields(logrus.Fields{
		"poll_interval": daemonPollInterval,
		"session_id":    daemonSessionID,
		"workspace_id":  daemonWorkspaceID,
		"pid":           os.Getpid(),
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
	ticker := time.NewTicker(daemonPollInterval)
	defer ticker.Stop()

	daemonLogger.Info("✅ デーモンが開始されました。セッションのタスクポーリングを開始します...")

	for {
		select {
		case <-ctx.Done():
			daemonLogger.Info("🛑 グレースフルシャットダウンを実行しています...")
			return nil
		case <-ticker.C:
			if err := pollAndExecuteSessionTasks(ctx, apiClient, daemonLogger); err != nil {
				daemonLogger.WithError(err).Error("セッションタスクポーリング中にエラーが発生しました")
			}
		}
	}
}

// pollAndExecuteSessionTasks はセッションからタスクをポーリングし、順次実行します
func pollAndExecuteSessionTasks(ctx context.Context, apiClient *api.Client, logger *logrus.Entry) error {
	logger.Debug("📡 セッションから新しいタスクをポーリングしています...")

	// セッション状態の確認
	if daemonSessionID != "" {
		// 部分的なIDから完全なUUIDを取得
		fullSessionID := resolveFullSessionID(apiClient, daemonSessionID, logger)
		if fullSessionID != daemonSessionID {
			// 完全なUUIDが取得できた場合、更新する
			daemonSessionID = fullSessionID
		}

		session, err := apiClient.GetSession(daemonSessionID)
		if err != nil {
			return fmt.Errorf("session info retrieval failed: %w", err)
		}

		// セッションが完了している場合はタスクポーリングをスキップ
		if session.Status == "COMPLETED" || session.Status == "TERMINATED" {
			logger.WithField("session_status", session.Status).Debug("セッションが完了しているため、タスクポーリングをスキップします")
			return nil
		}

		// セッション用のPENDINGタスクを取得
		tasks, err := apiClient.GetPendingTasksForSession(daemonSessionID)
		if err != nil {
			return fmt.Errorf("session task retrieval failed: %w", err)
		}

		if len(tasks) == 0 {
			logger.Debug("新しいタスクはありません")
			return nil
		}

		logger.WithFields(logrus.Fields{
			"task_count": len(tasks),
			"session_id": daemonSessionID,
		}).Info("📋 セッションから新しいタスクを受信しました")

		// 各タスクを順次実行（並列実行なし）
		for _, task := range tasks {
			select {
			case <-ctx.Done():
				logger.Info("シャットダウン中のため、タスク実行を中断します")
				return nil
			default:
				if err := executeTask(ctx, apiClient, task, logger); err != nil {
					logger.WithError(err).WithField("task_id", task.ID).Error("タスクの実行に失敗しました")
					// エラーが発生しても次のタスクへ継続
				}
			}
		}
	} else if daemonWorkspaceID != "" {
		// レガシーサポート: ワークスペース用のタスクを取得
		tasks, err := apiClient.GetPendingTasksForWorkspace(daemonWorkspaceID)
		if err != nil {
			return fmt.Errorf("workspace task retrieval failed: %w", err)
		}

		if len(tasks) == 0 {
			logger.Debug("新しいタスクはありません")
			return nil
		}

		logger.WithField("task_count", len(tasks)).Info("📋 ワークスペースから新しいタスクを受信しました")

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
	} else {
		return fmt.Errorf("session ID or workspace ID not configured")
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
		return fmt.Errorf("environment variable setup failed: %w", err)
	}
	defer func() {
		if err := os.Setenv("KERUTA_TASK_ID", oldTaskID); err != nil {
			taskLogger.WithError(err).Error("環境変数の復元に失敗しました")
		}
	}()

	// タスク開始の通知
	if err := apiClient.StartTask(task.ID); err != nil {
		return fmt.Errorf("task start notification failed: %w", err)
	}

	// スクリプトの取得
	script, err := apiClient.GetTaskScript(task.ID)
	if err != nil {
		if failErr := apiClient.FailTask(task.ID, "スクリプトの取得に失敗しました", "SCRIPT_FETCH_ERROR"); failErr != nil {
			taskLogger.WithError(failErr).Error("タスク失敗の通知に失敗しました")
		}
		return fmt.Errorf("script retrieval failed: %w", err)
	}

	// スクリプト内容を表示
	taskLogger.Info("📋 実行するスクリプトの内容:")
	taskLogger.Info("=" + strings.Repeat("=", 50))
	for i, line := range strings.Split(script, "\n") {
		taskLogger.Infof("%3d | %s", i+1, line)
	}
	taskLogger.Info("=" + strings.Repeat("=", 50))

	// スクリプトの実行 - 常にclaudeコマンドを使用
	if err := executeTmuxClaudeTask(ctx, apiClient, task.ID, script, taskLogger); err != nil {
		if failErr := apiClient.FailTask(task.ID, fmt.Sprintf("Claude タスクの実行に失敗しました: %v", err), "CLAUDE_EXECUTION_ERROR"); failErr != nil {
			taskLogger.WithError(failErr).Error("タスク失敗の通知に失敗しました")
		}
		return fmt.Errorf("claude task execution failed: %w", err)
	}

	// タスク完了後にGit変更をプッシュ
	if err := pushTaskChanges(apiClient, task.SessionID, task.ID, taskLogger); err != nil {
		taskLogger.WithError(err).Warn("変更のプッシュに失敗しました（タスクは完了扱いとします）")
	}

	// タスク成功の通知
	if err := apiClient.SuccessTask(task.ID, "タスクが正常に完了しました"); err != nil {
		return fmt.Errorf("task success notification failed: %w", err)
	}

	taskLogger.Info("✅ タスクが正常に完了しました")
	return nil
}

// writePIDFile はPIDファイルを作成します
func writePIDFile(pidFile string) error {
	file, err := os.Create(pidFile)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			// ログ出力は行わない（ファイルクローズエラーは通常クリティカルではない）
		}
	}()

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
	daemonCmd.Flags().DurationVar(&daemonInterval, "interval", 10*time.Second, "タスクポーリングの間隔（非推奨、--poll-intervalを使用）")
	daemonCmd.Flags().DurationVar(&daemonPollInterval, "poll-interval", 5*time.Second, "タスクポーリングの間隔")
	daemonCmd.Flags().StringVar(&daemonSessionID, "session-id", "", "監視するセッションID（環境変数KERUTA_SESSION_IDから自動取得）")
	daemonCmd.Flags().StringVar(&daemonWorkspaceID, "workspace-id", "", "ワークスペースID（環境変数KERUTA_WORKSPACE_IDから自動取得）")
	daemonCmd.Flags().StringVar(&daemonPidFile, "pid-file", "", "PIDファイルのパス")
	daemonCmd.Flags().StringVar(&daemonLogFile, "log-file", "", "ログファイルのパス")

	// 環境変数からのデフォルト値設定
	if sessionID := os.Getenv("KERUTA_SESSION_ID"); sessionID != "" {
		daemonSessionID = sessionID
	}
	if workspaceID := os.Getenv("KERUTA_WORKSPACE_ID"); workspaceID != "" {
		daemonWorkspaceID = workspaceID
	}
	// レガシーサポート
	if workspaceID := os.Getenv("CODER_WORKSPACE_ID"); workspaceID != "" && daemonWorkspaceID == "" {
		daemonWorkspaceID = workspaceID
	}

	// ワークスペース名からセッションIDを自動取得
	if daemonSessionID == "" && daemonWorkspaceID == "" {
		if workspaceName := getWorkspaceName(); workspaceName != "" {
			logrus.WithField("workspace_name", workspaceName).Info("ワークスペース名からセッションIDを取得しています...")
			if partialSessionID := extractSessionIDFromWorkspaceName(workspaceName); partialSessionID != "" {
				// 部分的なIDから完全なUUIDを取得する処理を追加
				// ここではまだAPIクライアントが利用できないため、部分的なIDを一時的に設定
				daemonSessionID = partialSessionID
				logrus.WithField("session_id", partialSessionID).Info("ワークスペース名からセッションIDを自動取得しました")
			} else {
				logrus.WithField("workspace_name", workspaceName).Warn("ワークスペース名からセッションIDを抽出できませんでした")
			}
		} else {
			logrus.Warn("ワークスペース名を取得できませんでした。環境変数CODER_WORKSPACE_NAME、ホスト名、またはディレクトリ名を確認してください")
		}
	}

	// poll-intervalが設定されていない場合、intervalを使用（後方互換性）
	daemonCmd.PreRun = func(cmd *cobra.Command, args []string) {
		if !cmd.Flags().Changed("poll-interval") && cmd.Flags().Changed("interval") {
			daemonPollInterval = daemonInterval
		}
	}
}
