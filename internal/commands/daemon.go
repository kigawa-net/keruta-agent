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

	// タスク実行前にタスク専用のブランチを作成・チェックアウト
	if err := setupTaskBranch(apiClient, task.SessionID, task.ID, taskLogger); err != nil {
		taskLogger.WithError(err).Warn("タスク専用ブランチのセットアップに失敗しました（処理を継続）")
	}

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

	// スクリプトの実行 - タスク内容に応じてtmux+claude実行またはスクリプト実行を選択
	if isClaudeTask(script) {
		if err := executeTmuxClaudeTask(ctx, apiClient, task.ID, script, taskLogger); err != nil {
			if failErr := apiClient.FailTask(task.ID, fmt.Sprintf("Claude タスクの実行に失敗しました: %v", err), "CLAUDE_EXECUTION_ERROR"); failErr != nil {
				taskLogger.WithError(failErr).Error("タスク失敗の通知に失敗しました")
			}
			return fmt.Errorf("Claude タスクの実行に失敗しました: %w", err)
		}
	} else {
		if err := executeScript(ctx, apiClient, task.ID, script, taskLogger); err != nil {
			if failErr := apiClient.FailTask(task.ID, fmt.Sprintf("スクリプトの実行に失敗しました: %v", err), "SCRIPT_EXECUTION_ERROR"); failErr != nil {
				taskLogger.WithError(failErr).Error("タスク失敗の通知に失敗しました")
			}
			return fmt.Errorf("スクリプトの実行に失敗しました: %w", err)
		}
	}

	// タスク完了後にGit変更をプッシュ
	if err := pushTaskChanges(apiClient, task.SessionID, task.ID, taskLogger); err != nil {
		taskLogger.WithError(err).Warn("変更のプッシュに失敗しました（タスクは完了扱いとします）")
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

	logger.WithField("session", session).Debug("セッション情報取得完了")

	// リポジトリURLがない場合はスキップ
	if session.RepositoryURL == "" {
		logger.Warn("セッションにリポジトリURLが設定されていないため、リポジトリ初期化をスキップします")
		return nil
	}

	// 作業ディレクトリのパスを決定
	gitTemplateConfig := &git.SessionTemplateConfig{
		TemplateID:        "",
		TemplateName:      "",
		TemplatePath:      ".",
		PreferredKeywords: []string{},
		Parameters:        map[string]string{},
	}
	
	// セッションにTemplateConfigがある場合はそのデータも使用
	if session.TemplateConfig != nil {
		gitTemplateConfig.TemplateID = session.TemplateConfig.TemplateID
		gitTemplateConfig.TemplateName = session.TemplateConfig.TemplateName
		gitTemplateConfig.TemplatePath = session.TemplateConfig.TemplatePath
		gitTemplateConfig.PreferredKeywords = session.TemplateConfig.PreferredKeywords
		gitTemplateConfig.Parameters = session.TemplateConfig.Parameters
	}
	
	workDir := git.DetermineWorkingDirectory(sessionID, session.RepositoryURL)

	logger.WithFields(logrus.Fields{
		"repository_url": session.RepositoryURL,
		"repository_ref": session.RepositoryRef,
		"working_dir":    workDir,
	}).Info("📂 Gitリポジトリを初期化しています...")

	// Gitリポジトリを作成
	repo := git.NewRepository(
		session.RepositoryURL,
		session.RepositoryRef,
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

// getWorkspaceName はCoderワークスペース名を取得します
func getWorkspaceName() string {
	// Coder環境変数から取得（最も一般的）
	if workspaceName := os.Getenv("CODER_WORKSPACE_NAME"); workspaceName != "" {
		return workspaceName
	}
	
	// ホスト名から取得（Coderワークスペース内では一般的にワークスペース名がホスト名になる）
	if hostname, err := os.Hostname(); err == nil && hostname != "" && hostname != "localhost" {
		return hostname
	}
	
	// PWDの最後のディレクトリ名から推測
	if pwd := os.Getenv("PWD"); pwd != "" {
		parts := strings.Split(pwd, "/")
		if len(parts) > 0 {
			lastDir := parts[len(parts)-1]
			if lastDir != "" && strings.HasPrefix(lastDir, "session-") {
				return lastDir
			}
		}
	}
	
	return ""
}

// extractSessionIDFromWorkspaceName はワークスペース名からセッションIDを抽出します
func extractSessionIDFromWorkspaceName(workspaceName string) string {
	// パターン1: session-{full-uuid}-{suffix} の形式（最優先）
	// 例: session-29229ea1-8c41-4ca2-b064-7a7a7672dd1a-keruta
	if strings.HasPrefix(workspaceName, "session-") {
		// "session-" を除去
		remaining := workspaceName[8:]
		
		// UUID形式のパターンを探す (8-4-4-4-12の形式)
		if uuid := extractUUIDPattern(remaining); uuid != "" {
			return uuid
		}
		
		// フォールバック: 最初の部分だけを取得（後方互換性のため）
		parts := strings.Split(remaining, "-")
		if len(parts) >= 1 {
			sessionID := parts[0]
			if len(sessionID) >= 8 {
				return sessionID
			}
		}
	}
	
	// パターン2: {full-uuid}-{suffix} の形式
	if uuid := extractUUIDPattern(workspaceName); uuid != "" {
		return uuid
	}
	
	// パターン3: 完全なUUID形式（ハイフンを含む）
	if len(workspaceName) >= 32 && strings.Contains(workspaceName, "-") {
		if isValidUUIDFormat(workspaceName) {
			return workspaceName
		}
	}
	
	// パターン4: {sessionId}-{suffix} の形式（UUIDの最初の部分のみ - 後方互換性）
	parts := strings.Split(workspaceName, "-")
	if len(parts) >= 2 {
		possibleID := parts[0]
		// UUIDの最初の部分らしき文字列（8文字以上の英数字）
		if len(possibleID) >= 8 && isAlphaNumeric(possibleID) {
			return possibleID
		}
	}
	
	return ""
}

// resolveFullSessionID は部分的なセッションIDまたはワークスペース名から完全なUUIDを取得します
func resolveFullSessionID(apiClient *api.Client, partialID string, logger *logrus.Entry) string {
	// 既に完全なUUID形式の場合はそのまま返す
	if isValidUUIDFormat(partialID) {
		return partialID
	}
	
	// 部分的なIDが短すぎる場合はそのまま返す
	if len(partialID) < 4 {
		logger.WithField("partialId", partialID).Debug("部分的なIDが短すぎるため、APIで検索をスキップします")
		return partialID
	}
	
	logger.WithField("partialId", partialID).Info("部分的なセッションIDから完全なUUIDを検索しています...")
	
	// まず、ワークスペース名による完全一致検索を試す
	// ワークスペース名が "session-{uuid}-{suffix}" の形式の場合、ワークスペース名全体で検索
	if strings.HasPrefix(partialID, "session-") || len(partialID) > 20 {
		workspaceName := getWorkspaceName()
		if workspaceName != "" && workspaceName != partialID {
			logger.WithField("workspaceName", workspaceName).Info("ワークスペース名による完全一致検索を試行中...")
			if session, err := apiClient.SearchSessionByName(workspaceName); err == nil {
				logger.WithFields(logrus.Fields{
					"workspaceName": workspaceName,
					"sessionId":     session.ID,
					"sessionName":   session.Name,
				}).Info("ワークスペース名による完全一致でセッションを発見しました")
				return session.ID
			}
		}
	}
	
	// 部分的なIDから完全なUUIDを検索
	session, err := apiClient.SearchSessionByPartialID(partialID)
	if err != nil {
		logger.WithError(err).WithField("partialId", partialID).Warning("部分的なIDでの検索に失敗しました。元のIDを使用します")
		return partialID
	}
	
	logger.WithFields(logrus.Fields{
		"partialId": partialID,
		"fullId":    session.ID,
		"sessionName": session.Name,
	}).Info("完全なセッションUUIDを取得しました")
	
	return session.ID
}

// setupTaskBranch はタスク専用のブランチを作成・チェックアウトします
func setupTaskBranch(apiClient *api.Client, sessionID, taskID string, logger *logrus.Entry) error {
	// 作業ディレクトリが設定されているかチェック
	workDir := os.Getenv("KERUTA_WORKING_DIR")
	if workDir == "" {
		logger.Debug("作業ディレクトリが設定されていないため、ブランチ作成をスキップします")
		return nil
	}

	// ディレクトリがGitリポジトリかチェック
	gitDir := filepath.Join(workDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		logger.Debug("作業ディレクトリがGitリポジトリではないため、ブランチ作成をスキップします")
		return nil
	}

	// セッション情報を取得してリポジトリ設定を確認
	session, err := apiClient.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("セッション情報の取得に失敗: %w", err)
	}

	if session.RepositoryURL == "" {
		logger.Debug("セッションにリポジトリURLが設定されていないため、ブランチ作成をスキップします")
		return nil
	}

	// タスク専用のブランチ名を生成
	branchName := git.GenerateBranchName(sessionID, taskID)
	
	logger.WithFields(logrus.Fields{
		"session_id":  sessionID,
		"task_id":     taskID,
		"branch_name": branchName,
		"working_dir": workDir,
	}).Info("🌿 タスク専用ブランチを作成・チェックアウトしています...")

	// Gitリポジトリインスタンスを作成
	repo := git.NewRepositoryWithBranch(
		session.RepositoryURL,
		session.RepositoryRef,
		workDir,
		branchName,
		logger.WithField("component", "git"),
	)

	// 新しいブランチを作成・チェックアウト
	return repo.CreateAndCheckoutBranch()
}

// pushTaskChanges はタスク完了後に変更をコミット・プッシュします
func pushTaskChanges(apiClient *api.Client, sessionID, taskID string, logger *logrus.Entry) error {
	// 作業ディレクトリが設定されているかチェック
	workDir := os.Getenv("KERUTA_WORKING_DIR")
	if workDir == "" {
		logger.Debug("作業ディレクトリが設定されていないため、プッシュをスキップします")
		return nil
	}

	// ディレクトリがGitリポジトリかチェック
	gitDir := filepath.Join(workDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		logger.Debug("作業ディレクトリがGitリポジトリではないため、プッシュをスキップします")
		return nil
	}

	// セッション情報を取得してリポジトリ設定を確認
	session, err := apiClient.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("セッション情報の取得に失敗: %w", err)
	}

	if session.RepositoryURL == "" {
		logger.Debug("セッションにリポジトリURLが設定されていないため、プッシュをスキップします")
		return nil
	}

	// プッシュが無効化されているかチェック（環境変数）
	if os.Getenv("KERUTA_DISABLE_AUTO_PUSH") == "true" {
		logger.Info("自動プッシュが無効化されています")
		return nil
	}

	logger.WithFields(logrus.Fields{
		"session_id": sessionID,
		"task_id":    taskID,
		"working_dir": workDir,
	}).Info("🚀 タスク完了後の変更をコミット・プッシュしています...")

	// Gitリポジトリインスタンスを作成
	repo := git.NewRepositoryWithBranchAndPush(
		session.RepositoryURL,
		session.RepositoryRef,
		workDir,
		"", // ブランチ名は不要（現在のブランチを使用）
		true, // AutoPush有効
		logger.WithField("component", "git"),
	)

	// コミットメッセージを生成
	branchName := git.GenerateBranchName(sessionID, taskID)
	commitMessage := fmt.Sprintf("Task %s completed\n\nTask executed in branch: %s\nSession: %s", 
		taskID[:8], branchName, sessionID[:8])

	// 変更をコミット・プッシュ
	force := os.Getenv("KERUTA_FORCE_PUSH") == "true"
	return repo.CommitAndPushChanges(commitMessage, force)
}

// extractUUIDPattern はUUID形式のパターンを抽出します
func extractUUIDPattern(text string) string {
	// UUID形式: 8-4-4-4-12 (例: 29229ea1-8c41-4ca2-b064-7a7a7672dd1a)
	parts := strings.Split(text, "-")
	if len(parts) >= 5 {
		// 最初の5つの部分がUUID形式かチェック
		if len(parts[0]) == 8 && len(parts[1]) == 4 && len(parts[2]) == 4 && 
		   len(parts[3]) == 4 && len(parts[4]) == 12 {
			// 各部分が16進数かチェック
			uuid := strings.Join(parts[0:5], "-")
			if isValidUUIDFormat(uuid) {
				return uuid
			}
		}
	}
	return ""
}

// isValidUUIDFormat はUUID形式として有効かをチェックします
func isValidUUIDFormat(uuid string) bool {
	// 基本的な長さチェック (36文字: 32文字 + 4つのハイフン)
	if len(uuid) != 36 {
		return false
	}
	
	// ハイフンの位置チェック
	if uuid[8] != '-' || uuid[13] != '-' || uuid[18] != '-' || uuid[23] != '-' {
		return false
	}
	
	// 各部分が16進数かチェック
	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		return false
	}
	
	for _, part := range parts {
		for _, r := range part {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	
	return true
}

// isAlphaNumeric は文字列が英数字のみかチェックします
func isAlphaNumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// isClaudeTask はタスクがClaude実行タスクかどうかを判定します
func isClaudeTask(script string) bool {
	// タスク内容に特定のキーワードが含まれている場合にClaude実行とみなす
	return strings.Contains(script, "claude") ||
		   strings.Contains(script, "CLAUDE") ||
		   strings.Contains(script, "Claude")
}

// executeTmuxClaudeTask はtmux環境でClaude実行タスクを実行します
func executeTmuxClaudeTask(ctx context.Context, apiClient *api.Client, taskID string, taskContent string, taskLogger *logrus.Entry) error {
	taskLogger.Info("🎯 tmux環境でClaude実行タスクを開始しています...")

	// ~/keruta ディレクトリの存在を確認・作成
	kerutaDir := os.ExpandEnv("$HOME/keruta")
	if err := ensureDirectory(kerutaDir); err != nil {
		return fmt.Errorf("~/kerutaディレクトリの作成に失敗: %w", err)
	}

	// tmuxセッション名を生成（タスクIDベース）
	tmuxSessionName := fmt.Sprintf("keruta-task-%s", taskID[:8])
	
	taskLogger.WithFields(logrus.Fields{
		"tmux_session": tmuxSessionName,
		"working_dir":  kerutaDir,
		"task_content": taskContent,
	}).Info("tmuxセッションでClaude実行を開始します")

	// Claude実行コマンドを構築
	claudeCmd := fmt.Sprintf(`claude -p "%s" --dangerously-skip-permissions`, strings.ReplaceAll(taskContent, `"`, `\"`))
	
	// tmuxコマンドを構築 - セッション作成、ディレクトリ移動、Claude実行
	tmuxCmd := exec.CommandContext(ctx, "tmux", 
		"new-session", "-d", "-s", tmuxSessionName, 
		"-c", kerutaDir,
		claudeCmd)

	// コマンド実行とログ収集
	return executeTmuxCommand(ctx, tmuxCmd, apiClient, taskID, tmuxSessionName, taskLogger)
}

// ensureDirectory はディレクトリの存在を確認し、存在しない場合は作成します
func ensureDirectory(dirPath string) error {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return os.MkdirAll(dirPath, 0755)
	}
	return nil
}

// executeTmuxCommand はtmuxコマンドを実行し、出力を監視します
func executeTmuxCommand(ctx context.Context, cmd *exec.Cmd, apiClient *api.Client, taskID, sessionName string, logger *logrus.Entry) error {
	logger.Info("🚀 tmuxセッションを起動しています...")

	// tmuxセッション開始
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("tmuxセッション開始に失敗: %w", err)
	}

	// tmuxセッションの出力を監視
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := captureTmuxOutput(apiClient, taskID, sessionName, logger); err != nil {
					logger.WithError(err).Warning("tmux出力キャプチャに失敗しました")
				}
			}
		}
	}()

	// tmuxセッションの完了を待機
	if err := cmd.Wait(); err != nil {
		// tmuxセッションを明示的に終了
		_ = killTmuxSession(sessionName, logger)
		return fmt.Errorf("tmuxセッション実行に失敗: %w", err)
	}

	// 最終的な出力をキャプチャ
	if err := captureTmuxOutput(apiClient, taskID, sessionName, logger); err != nil {
		logger.WithError(err).Warning("最終出力キャプチャに失敗しました")
	}

	// tmuxセッションをクリーンアップ
	if err := killTmuxSession(sessionName, logger); err != nil {
		logger.WithError(err).Warning("tmuxセッションのクリーンアップに失敗しました")
	}

	logger.Info("✅ tmux Claude実行タスクが完了しました")
	return nil
}

// captureTmuxOutput はtmuxセッションの出力をキャプチャしてAPIに送信します
func captureTmuxOutput(apiClient *api.Client, taskID, sessionName string, logger *logrus.Entry) error {
	// tmux capture-pane で出力を取得
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("tmux出力キャプチャに失敗: %w", err)
	}

	// 出力が空でない場合のみログ送信
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" {
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				logger.Info(line)
				// APIにログを送信
				if sendErr := apiClient.SendLog(taskID, "INFO", line); sendErr != nil {
					logger.WithError(sendErr).Warning("ログ送信に失敗しました")
				}
			}
		}
	}

	return nil
}

// killTmuxSession はtmuxセッションを終了します
func killTmuxSession(sessionName string, logger *logrus.Entry) error {
	cmd := exec.Command("tmux", "kill-session", "-t", sessionName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmuxセッション終了に失敗: %w", err)
	}
	
	logger.WithField("session", sessionName).Info("tmuxセッションを終了しました")
	return nil
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
