package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	daemonInterval    time.Duration
	daemonPidFile     string
	daemonLogFile     string
	daemonWorkspaceID string
	daemonSessionID   string
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

	// Gitコマンドの利用可能性を確認
	if err := git.ValidateGitCommand(); err != nil {
		daemonLogger.WithError(err).Warn("Gitコマンドが利用できません。リポジトリ機能は無効になります")
	}

	// セッションの情報を取得してGitリポジトリを初期化
	if daemonSessionID != "" {
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
		session, err := apiClient.GetSession(daemonSessionID)
		if err != nil {
			return fmt.Errorf("セッション情報の取得に失敗しました: %w", err)
		}

		// セッションが完了している場合はタスクポーリングをスキップ
		if session.Status == "COMPLETED" || session.Status == "TERMINATED" {
			logger.WithField("session_status", session.Status).Debug("セッションが完了しているため、タスクポーリングをスキップします")
			return nil
		}

		// セッション用のPENDINGタスクを取得
		tasks, err := apiClient.GetPendingTasksForSession(daemonSessionID)
		if err != nil {
			return fmt.Errorf("セッションタスクの取得に失敗しました: %w", err)
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
			return fmt.Errorf("ワークスペースタスクの取得に失敗しました: %w", err)
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
		return fmt.Errorf("セッションIDまたはワークスペースIDが設定されていません")
	}

	return nil
}

// pollAndExecuteTasks はAPIサーバーからタスクをポーリングし、実行します（レガシー関数）
func pollAndExecuteTasks(ctx context.Context, apiClient *api.Client, logger *logrus.Entry) error {
	return pollAndExecuteSessionTasks(ctx, apiClient, logger)
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
	if err := executeScript(ctx, apiClient, task.ID, script, taskLogger); err != nil {
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
func executeScript(ctx context.Context, apiClient *api.Client, taskID string, script string, scriptLogger *logrus.Entry) error {
	scriptLogger.Info("📝 スクリプトを実行しています...")

	// 一時的なスクリプトファイルを作成
	tmpFile, err := os.CreateTemp("", "keruta-script-*.sh")
	if err != nil {
		return fmt.Errorf("一時スクリプトファイルの作成に失敗: %w", err)
	}
	defer func() {
		if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
			scriptLogger.WithError(removeErr).Warning("一時スクリプトファイルの削除に失敗しました")
		}
	}()

	// スクリプト内容を書き込み
	if _, err := tmpFile.WriteString(script); err != nil {
		return fmt.Errorf("スクリプトファイルへの書き込みに失敗: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("スクリプトファイルのクローズに失敗: %w", err)
	}

	// 実行権限を付与
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		return fmt.Errorf("スクリプトファイルの実行権限設定に失敗: %w", err)
	}

	// スクリプトを実行
	cmd := exec.CommandContext(ctx, "/bin/bash", tmpFile.Name())
	
	// 作業ディレクトリを設定（環境変数KERUTA_WORKING_DIRが設定されている場合）
	if workDir := os.Getenv("KERUTA_WORKING_DIR"); workDir != "" {
		if _, err := os.Stat(workDir); err == nil {
			cmd.Dir = workDir
			scriptLogger.WithField("working_dir", workDir).Debug("作業ディレクトリを設定しました")
		} else {
			scriptLogger.WithField("working_dir", workDir).Warn("作業ディレクトリが存在しません")
		}
	}
	
	// 標準出力・標準エラーのパイプを作成
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("標準出力パイプの作成に失敗: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("標準エラーパイプの作成に失敗: %w", err)
	}

	// コマンドを開始
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("スクリプトの開始に失敗: %w", err)
	}

	// 標準出力をリアルタイムで読み取りログ送信
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) != "" {
				scriptLogger.Info(line)
				// リアルタイムでログを送信
				if sendErr := apiClient.SendLog(taskID, "INFO", line); sendErr != nil {
					scriptLogger.WithError(sendErr).Warning("標準出力ログ送信に失敗しました")
				}
			}
		}
		if err := scanner.Err(); err != nil {
			scriptLogger.WithError(err).Error("標準出力の読み取りに失敗しました")
		}
	}()

	// 標準エラーをリアルタイムで読み取りログ送信
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) != "" {
				scriptLogger.Error(line)
				// リアルタイムでログを送信
				if sendErr := apiClient.SendLog(taskID, "ERROR", line); sendErr != nil {
					scriptLogger.WithError(sendErr).Warning("標準エラーログ送信に失敗しました")
				}
			}
		}
		if err := scanner.Err(); err != nil {
			scriptLogger.WithError(err).Error("標準エラーの読み取りに失敗しました")
		}
	}()

	// コマンドの完了を待機
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("スクリプトの実行に失敗: %w", err)
	}

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

// initializeRepositoryForSession はセッションのGitリポジトリを初期化します
func initializeRepositoryForSession(apiClient *api.Client, sessionID string, logger *logrus.Entry) error {
	logger.Info("🔧 セッションのリポジトリ情報を取得しています...")

	// セッション情報を取得
	session, err := apiClient.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("セッション情報の取得に失敗: %w", err)
	}

	// テンプレート設定がない場合はスキップ
	if session.TemplateConfig == nil {
		logger.Debug("セッションにテンプレート設定がないため、リポジトリ初期化をスキップします")
		return nil
	}

	templateConfig := session.TemplateConfig

	// リポジトリURLがない場合はスキップ
	if templateConfig.RepositoryURL == "" {
		logger.Debug("リポジトリURLが設定されていないため、リポジトリ初期化をスキップします")
		return nil
	}

	// 作業ディレクトリのパスを決定
	workDir := determineWorkingDirectory(sessionID, templateConfig)
	
	logger.WithFields(logrus.Fields{
		"repository_url": templateConfig.RepositoryURL,
		"repository_ref": templateConfig.RepositoryRef,
		"working_dir":    workDir,
	}).Info("📂 Gitリポジトリを初期化しています...")

	// Gitリポジトリを作成
	repo := git.NewRepository(
		templateConfig.RepositoryURL,
		templateConfig.RepositoryRef,
		workDir,
		logger.WithField("component", "git"),
	)

	// クローンまたはプル実行
	if err := repo.CloneOrPull(); err != nil {
		return fmt.Errorf("リポジトリのクローン/プルに失敗: %w", err)
	}

	// 環境変数にワーキングディレクトリを設定
	if err := os.Setenv("KERUTA_WORKING_DIR", workDir); err != nil {
		logger.WithError(err).Warn("環境変数KERUTA_WORKING_DIRの設定に失敗しました")
	}

	logger.WithField("working_dir", workDir).Info("✅ リポジトリの初期化が完了しました")
	return nil
}

// determineWorkingDirectory は作業ディレクトリのパスを決定します
func determineWorkingDirectory(sessionID string, templateConfig *api.SessionTemplateConfig) string {
	// 環境変数で作業ディレクトリが指定されている場合はそれを使用
	if workDir := os.Getenv("KERUTA_WORKING_DIR"); workDir != "" {
		return workDir
	}

	// デフォルトのベースディレクトリを決定
	baseDir := os.Getenv("KERUTA_BASE_DIR")
	if baseDir == "" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			baseDir = filepath.Join(homeDir, ".keruta")
		} else {
			baseDir = "/tmp/keruta"
		}
	}

	// セッションごとのディレクトリを作成
	sessionDir := filepath.Join(baseDir, "sessions", sessionID)
	
	// リポジトリ名を抽出（URLの最後の部分）
	repoName := "repository"
	if templateConfig.RepositoryURL != "" {
		parts := strings.Split(strings.TrimSuffix(templateConfig.RepositoryURL, ".git"), "/")
		if len(parts) > 0 {
			repoName = parts[len(parts)-1]
		}
	}

	return filepath.Join(sessionDir, repoName)
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

	// poll-intervalが設定されていない場合、intervalを使用（後方互換性）
	daemonCmd.PreRun = func(cmd *cobra.Command, args []string) {
		if !cmd.Flags().Changed("poll-interval") && cmd.Flags().Changed("interval") {
			daemonPollInterval = daemonInterval
		}
	}
}
