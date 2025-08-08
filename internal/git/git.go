package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Repository はGitリポジトリの情報を表します
type Repository struct {
	URL            string
	Ref            string
	Path           string
	NewBranchName  string // 作成する新しいブランチ名
	AutoPush       bool   // タスク終了時に自動プッシュするかどうか
	logger         *logrus.Entry
}

// NewRepository は新しいRepositoryインスタンスを作成します
func NewRepository(url, ref, path string, logger *logrus.Entry) *Repository {
	return &Repository{
		URL:    url,
		Ref:    ref,
		Path:   path,
		logger: logger,
	}
}

// NewRepositoryWithBranch は新しいブランチ作成付きのRepositoryインスタンスを作成します
func NewRepositoryWithBranch(url, ref, path, newBranchName string, logger *logrus.Entry) *Repository {
	return &Repository{
		URL:           url,
		Ref:           ref,
		Path:          path,
		NewBranchName: newBranchName,
		AutoPush:      true, // デフォルトで自動プッシュを有効化
		logger:        logger,
	}
}

// NewRepositoryWithBranchAndPush は新しいブランチ作成とプッシュ設定付きのRepositoryインスタンスを作成します
func NewRepositoryWithBranchAndPush(url, ref, path, newBranchName string, autoPush bool, logger *logrus.Entry) *Repository {
	return &Repository{
		URL:           url,
		Ref:           ref,
		Path:          path,
		NewBranchName: newBranchName,
		AutoPush:      autoPush,
		logger:        logger,
	}
}

// CloneOrPull はリポジトリをクローンまたはプルします
func (r *Repository) CloneOrPull() error {
	if r.URL == "" {
		r.logger.Debug("リポジトリURLが設定されていないため、Git操作をスキップします")
		return nil
	}

	// パスが存在するかチェック
	if _, err := os.Stat(r.Path); os.IsNotExist(err) {
		// ディレクトリが存在しない場合、親ディレクトリを作成
		if err := os.MkdirAll(filepath.Dir(r.Path), 0755); err != nil {
			return fmt.Errorf("ディレクトリの作成に失敗: %w", err)
		}
		return r.clone()
	}

	// ディレクトリが存在する場合、Gitリポジトリかどうかチェック
	if r.isGitRepository() {
		return r.pull()
	}

	// Gitリポジトリではない場合、ディレクトリを削除してクローン
	r.logger.Warn("既存のディレクトリがGitリポジトリではないため、削除してクローンします")
	if err := os.RemoveAll(r.Path); err != nil {
		return fmt.Errorf("既存ディレクトリの削除に失敗: %w", err)
	}
	return r.clone()
}

// clone はリポジトリをクローンします
func (r *Repository) clone() error {
	r.logger.WithFields(logrus.Fields{
		"url":  r.URL,
		"ref":  r.Ref,
		"path": r.Path,
	}).Info("🔄 Gitリポジトリをクローンしています...")

	// git clone コマンドを実行
	args := []string{"clone"}

	// 特定のブランチ/タグを指定
	if r.Ref != "" && r.Ref != "main" && r.Ref != "master" {
		args = append(args, "--branch", r.Ref)
	}

	args = append(args, r.URL, r.Path)

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		r.logger.WithError(err).WithField("output", string(output)).Error("Gitクローンに失敗しました")
		return fmt.Errorf("gitクローンに失敗: %w\n出力: %s", err, string(output))
	}

	r.logger.Info("✅ Gitリポジトリのクローンが完了しました")

	// クローン後に指定されたrefにチェックアウト（main/master以外の場合）
	if r.Ref != "" && r.Ref != "main" && r.Ref != "master" {
		if err := r.checkout(); err != nil {
			return err
		}
	}

	// 新しいブランチを作成・チェックアウト
	if r.NewBranchName != "" {
		return r.CreateAndCheckoutBranch()
	}

	return nil
}

// pull はリポジトリをプルします
func (r *Repository) pull() error {
	r.logger.WithFields(logrus.Fields{
		"url":  r.URL,
		"ref":  r.Ref,
		"path": r.Path,
	}).Info("🔄 Gitリポジトリをプルしています...")

	// リポジトリディレクトリに移動してプル
	oldDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("現在のディレクトリの取得に失敗: %w", err)
	}
	defer func() {
		if chErr := os.Chdir(oldDir); chErr != nil {
			r.logger.WithError(chErr).Error("元のディレクトリに戻るのに失敗しました")
		}
	}()

	if err := os.Chdir(r.Path); err != nil {
		return fmt.Errorf("リポジトリディレクトリへの移動に失敗: %w", err)
	}

	// リモートの情報を取得
	if err := r.fetch(); err != nil {
		return err
	}

	// 指定されたrefにチェックアウト
	if r.Ref != "" {
		if err := r.checkout(); err != nil {
			return err
		}
	}

	// プル実行
	cmd := exec.Command("git", "pull")
	output, err := cmd.CombinedOutput()

	if err != nil {
		r.logger.WithError(err).WithField("output", string(output)).Error("Gitプルに失敗しました")
		return fmt.Errorf("gitプルに失敗: %w\n出力: %s", err, string(output))
	}

	r.logger.Info("✅ Gitリポジトリのプルが完了しました")

	// 新しいブランチを作成・チェックアウト
	if r.NewBranchName != "" {
		return r.CreateAndCheckoutBranch()
	}

	return nil
}

// fetch はリモートの情報を取得します
func (r *Repository) fetch() error {
	r.logger.Debug("リモートの情報を取得しています...")

	cmd := exec.Command("git", "fetch", "--all")
	output, err := cmd.CombinedOutput()

	if err != nil {
		r.logger.WithError(err).WithField("output", string(output)).Error("Git fetchに失敗しました")
		return fmt.Errorf("git fetchに失敗: %w\n出力: %s", err, string(output))
	}

	return nil
}

// checkout は指定されたrefにチェックアウトします
func (r *Repository) checkout() error {
	if r.Ref == "" {
		return nil
	}

	r.logger.WithField("ref", r.Ref).Debug("指定されたrefにチェックアウトしています...")

	cmd := exec.Command("git", "checkout", r.Ref)
	output, err := cmd.CombinedOutput()

	if err != nil {
		r.logger.WithError(err).WithFields(logrus.Fields{
			"ref":    r.Ref,
			"output": string(output),
		}).Error("Git checkoutに失敗しました")
		return fmt.Errorf("git checkout %s に失敗: %w\n出力: %s", r.Ref, err, string(output))
	}

	r.logger.WithField("ref", r.Ref).Info("指定されたrefにチェックアウトしました")
	return nil
}

// CreateAndCheckoutBranch は新しいブランチを作成してチェックアウトします
func (r *Repository) CreateAndCheckoutBranch() error {
	if r.NewBranchName == "" {
		return nil
	}

	r.logger.WithField("branch_name", r.NewBranchName).Info("🌿 新しいブランチを作成・チェックアウトしています...")

	// 現在のディレクトリを保存
	oldDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("現在のディレクトリの取得に失敗: %w", err)
	}
	defer func() {
		if chErr := os.Chdir(oldDir); chErr != nil {
			r.logger.WithError(chErr).Error("元のディレクトリに戻るのに失敗しました")
		}
	}()

	// リポジトリディレクトリに移動
	if err := os.Chdir(r.Path); err != nil {
		return fmt.Errorf("リポジトリディレクトリへの移動に失敗: %w", err)
	}

	// ブランチが既に存在するかチェック
	if r.branchExists(r.NewBranchName) {
		r.logger.WithField("branch_name", r.NewBranchName).Info("ブランチが既に存在するためチェックアウトします")
		return r.checkoutExistingBranch(r.NewBranchName)
	}

	// 新しいブランチを作成してチェックアウト
	cmd := exec.Command("git", "checkout", "-b", r.NewBranchName)
	output, err := cmd.CombinedOutput()

	if err != nil {
		r.logger.WithError(err).WithFields(logrus.Fields{
			"branch_name": r.NewBranchName,
			"output":      string(output),
		}).Error("新しいブランチの作成・チェックアウトに失敗しました")
		return fmt.Errorf("git checkout -b %s に失敗: %w\n出力: %s", r.NewBranchName, err, string(output))
	}

	r.logger.WithField("branch_name", r.NewBranchName).Info("✅ 新しいブランチを作成・チェックアウトしました")
	return nil
}

// branchExists はブランチが存在するかどうかを確認します
func (r *Repository) branchExists(branchName string) bool {
	// ローカルブランチの存在確認
	cmd := exec.Command("git", "branch", "--list", branchName)
	output, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		return true
	}

	// リモートブランチの存在確認
	cmd = exec.Command("git", "branch", "-r", "--list", "origin/"+branchName)
	output, err = cmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		return true
	}

	return false
}

// checkoutExistingBranch は既存のブランチにチェックアウトします
func (r *Repository) checkoutExistingBranch(branchName string) error {
	cmd := exec.Command("git", "checkout", branchName)
	output, err := cmd.CombinedOutput()

	if err != nil {
		r.logger.WithError(err).WithFields(logrus.Fields{
			"branch_name": branchName,
			"output":      string(output),
		}).Error("既存ブランチへのチェックアウトに失敗しました")
		return fmt.Errorf("git checkout %s に失敗: %w\n出力: %s", branchName, err, string(output))
	}

	r.logger.WithField("branch_name", branchName).Info("既存のブランチにチェックアウトしました")
	return nil
}

// PushBranch は指定されたブランチをリモートにプッシュします
func (r *Repository) PushBranch(branchName string, force bool) error {
	if branchName == "" {
		return fmt.Errorf("ブランチ名が指定されていません")
	}

	r.logger.WithField("branch_name", branchName).Info("🚀 ブランチをリモートにプッシュしています...")

	// 現在のディレクトリを保存
	oldDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("現在のディレクトリの取得に失敗: %w", err)
	}
	defer func() {
		if chErr := os.Chdir(oldDir); chErr != nil {
			r.logger.WithError(chErr).Error("元のディレクトリに戻るのに失敗しました")
		}
	}()

	// リポジトリディレクトリに移動
	if err := os.Chdir(r.Path); err != nil {
		return fmt.Errorf("リポジトリディレクトリへの移動に失敗: %w", err)
	}

	// プッシュコマンドの構築
	args := []string{"push", "-u", "origin", branchName}
	if force {
		args = append(args, "--force-with-lease")
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		r.logger.WithError(err).WithFields(logrus.Fields{
			"branch_name": branchName,
			"output":      string(output),
			"force":       force,
		}).Error("ブランチのプッシュに失敗しました")
		return fmt.Errorf("git push origin %s に失敗: %w\n出力: %s", branchName, err, string(output))
	}

	r.logger.WithField("branch_name", branchName).Info("✅ ブランチのプッシュが完了しました")
	return nil
}

// PushCurrentBranch は現在のブランチをリモートにプッシュします
func (r *Repository) PushCurrentBranch(force bool) error {
	// 現在のブランチ名を取得
	currentBranch, err := r.getCurrentBranchName()
	if err != nil {
		return fmt.Errorf("現在のブランチ名の取得に失敗: %w", err)
	}

	return r.PushBranch(currentBranch, force)
}

// getCurrentBranchName は現在のブランチ名を取得します
func (r *Repository) getCurrentBranchName() (string, error) {
	// 現在のディレクトリを保存
	oldDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("現在のディレクトリの取得に失敗: %w", err)
	}
	defer func() {
		if chErr := os.Chdir(oldDir); chErr != nil {
			r.logger.WithError(chErr).Error("元のディレクトリに戻るのに失敗しました")
		}
	}()

	// リポジトリディレクトリに移動
	if err := os.Chdir(r.Path); err != nil {
		return "", fmt.Errorf("リポジトリディレクトリへの移動に失敗: %w", err)
	}

	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("現在のブランチ名の取得に失敗: %w", err)
	}

	branchName := strings.TrimSpace(string(output))
	if branchName == "" {
		return "", fmt.Errorf("ブランチ名が空です")
	}

	return branchName, nil
}

// CommitAllChanges は全ての変更をコミットします
func (r *Repository) CommitAllChanges(message string) error {
	if message == "" {
		message = "Auto-commit by keruta-agent"
	}

	r.logger.WithField("message", message).Info("📝 変更をコミットしています...")

	// 現在のディレクトリを保存
	oldDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("現在のディレクトリの取得に失敗: %w", err)
	}
	defer func() {
		if chErr := os.Chdir(oldDir); chErr != nil {
			r.logger.WithError(chErr).Error("元のディレクトリに戻るのに失敗しました")
		}
	}()

	// リポジトリディレクトリに移動
	if err := os.Chdir(r.Path); err != nil {
		return fmt.Errorf("リポジトリディレクトリへの移動に失敗: %w", err)
	}

	// 変更があるかチェック
	hasChanges, err := r.hasUncommittedChanges()
	if err != nil {
		return fmt.Errorf("変更状態の確認に失敗: %w", err)
	}

	if !hasChanges {
		r.logger.Info("コミットする変更がありません")
		return nil
	}

	// git add -A
	addCmd := exec.Command("git", "add", "-A")
	addOutput, err := addCmd.CombinedOutput()
	if err != nil {
		r.logger.WithError(err).WithField("output", string(addOutput)).Error("git add に失敗しました")
		return fmt.Errorf("git add に失敗: %w\n出力: %s", err, string(addOutput))
	}

	// git commit
	commitCmd := exec.Command("git", "commit", "-m", message)
	commitOutput, err := commitCmd.CombinedOutput()
	if err != nil {
		r.logger.WithError(err).WithField("output", string(commitOutput)).Error("git commit に失敗しました")
		return fmt.Errorf("git commit に失敗: %w\n出力: %s", err, string(commitOutput))
	}

	r.logger.Info("✅ 変更のコミットが完了しました")
	return nil
}

// hasUncommittedChanges は未コミットの変更があるかチェックします
func (r *Repository) hasUncommittedChanges() (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status の実行に失敗: %w", err)
	}

	// 出力が空でない場合は変更がある
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// CommitAndPushChanges は変更をコミットしてプッシュします
func (r *Repository) CommitAndPushChanges(commitMessage string, force bool) error {
	// 変更をコミット
	if err := r.CommitAllChanges(commitMessage); err != nil {
		return fmt.Errorf("コミットに失敗: %w", err)
	}

	// 現在のブランチをプッシュ
	if err := r.PushCurrentBranch(force); err != nil {
		return fmt.Errorf("プッシュに失敗: %w", err)
	}

	return nil
}

// isGitRepository はディレクトリがGitリポジトリかどうかを判定します
func (r *Repository) isGitRepository() bool {
	gitDir := filepath.Join(r.Path, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return false
	}

	// git statusコマンドでリポジトリの有効性を確認
	oldDir, err := os.Getwd()
	if err != nil {
		return false
	}
	defer func() {
		if chErr := os.Chdir(oldDir); chErr != nil {
			r.logger.WithError(chErr).Error("元のディレクトリに戻るのに失敗しました")
		}
	}()

	if err := os.Chdir(r.Path); err != nil {
		return false
	}

	cmd := exec.Command("git", "status", "--porcelain")
	if err := cmd.Run(); err != nil {
		return false
	}

	return true
}

// GetWorkingDirectory は作業ディレクトリのパスを返します
func (r *Repository) GetWorkingDirectory() string {
	return r.Path
}

// ValidateGitCommand はgitコマンドが使用可能かどうかを確認します
func ValidateGitCommand() error {
	cmd := exec.Command("git", "--version")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("gitコマンドが見つかりません。Gitがインストールされていることを確認してください: %w\n出力: %s", err, string(output))
	}

	version := strings.TrimSpace(string(output))
	logrus.WithField("version", version).Debug("Gitコマンドが利用可能です")
	return nil
}

// SessionTemplateConfig はセッションテンプレートの設定を表します
type SessionTemplateConfig struct {
	TemplateID        string            `json:"templateId"`
	TemplateName      string            `json:"templateName"`
	TemplatePath      string            `json:"templatePath"`
	PreferredKeywords []string          `json:"preferredKeywords"`
	Parameters        map[string]string `json:"parameters"`
}

// DetermineWorkingDirectory は作業ディレクトリのパスを決定します
func DetermineWorkingDirectory(sessionID string, repositoryURL string) string {
	// 環境変数で作業ディレクトリが指定されている場合はそれを使用
	if workDir := os.Getenv("KERUTA_WORKING_DIR"); workDir != "" {
		return workDir
	}

	// デフォルトのベースディレクトリを決定（~/kerutaに変更）
	baseDir := os.Getenv("KERUTA_BASE_DIR")
	if baseDir == "" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			baseDir = filepath.Join(homeDir, "keruta")
		} else {
			baseDir = "/tmp/keruta"
		}
	}

	// リポジトリ名を抽出（URLの最後の部分）
	repoName := "repository"
	if repositoryURL != "" {
		parts := strings.Split(strings.TrimSuffix(repositoryURL, ".git"), "/")
		if len(parts) > 0 {
			repoName = parts[len(parts)-1]
		}
	}

	return filepath.Join(baseDir, repoName)
}

// GenerateBranchName はセッションIDやタスクIDに基づいてブランチ名を生成します
func GenerateBranchName(sessionID, taskID string) string {
	if sessionID == "" && taskID == "" {
		return ""
	}

	// セッションベースのブランチ名
	if sessionID != "" {
		// UUIDの場合は最初の8文字を使用
		if len(sessionID) >= 8 {
			sessionPrefix := sessionID
			if strings.Contains(sessionID, "-") {
				parts := strings.Split(sessionID, "-")
				if len(parts) > 0 {
					sessionPrefix = parts[0]
				}
			} else if len(sessionID) > 8 {
				sessionPrefix = sessionID[:8]
			}
			
			// タスクIDがある場合は追加
			if taskID != "" && len(taskID) >= 8 {
				taskPrefix := taskID
				if strings.Contains(taskID, "-") {
					parts := strings.Split(taskID, "-")
					if len(parts) > 0 {
						taskPrefix = parts[0]
					}
				} else if len(taskID) > 8 {
					taskPrefix = taskID[:8]
				}
				return fmt.Sprintf("keruta-task-%s-%s", sessionPrefix, taskPrefix)
			}
			
			return fmt.Sprintf("keruta-session-%s", sessionPrefix)
		}
	}

	// タスクベースのブランチ名（セッションIDがない場合）
	if taskID != "" && len(taskID) >= 8 {
		taskPrefix := taskID
		if strings.Contains(taskID, "-") {
			parts := strings.Split(taskID, "-")
			if len(parts) > 0 {
				taskPrefix = parts[0]
			}
		} else if len(taskID) > 8 {
			taskPrefix = taskID[:8]
		}
		return fmt.Sprintf("keruta-task-%s", taskPrefix)
	}

	// フォールバック: タイムスタンプベースのブランチ名
	return fmt.Sprintf("keruta-branch-%d", time.Now().Unix())
}
