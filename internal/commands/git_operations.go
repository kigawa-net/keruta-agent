package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"keruta-agent/internal/api"
	"keruta-agent/internal/git"

	"github.com/sirupsen/logrus"
)

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
		"session_id":  sessionID,
		"task_id":     taskID,
		"working_dir": workDir,
	}).Info("🚀 タスク完了後の変更をコミット・プッシュしています...")

	// Gitリポジトリインスタンスを作成
	repo := git.NewRepositoryWithBranchAndPush(
		session.RepositoryURL,
		session.RepositoryRef,
		workDir,
		"",   // ブランチ名は不要（現在のブランチを使用）
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
