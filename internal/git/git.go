package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// Repository はGitリポジトリの情報を表します
type Repository struct {
	URL    string
	Ref    string
	Path   string
	logger *logrus.Entry
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
		"url": r.URL,
		"ref": r.Ref,
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
		return r.checkout()
	}
	
	return nil
}

// pull はリポジトリをプルします
func (r *Repository) pull() error {
	r.logger.WithFields(logrus.Fields{
		"url": r.URL,
		"ref": r.Ref,
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
			"ref": r.Ref,
			"output": string(output),
		}).Error("Git checkoutに失敗しました")
		return fmt.Errorf("git checkout %s に失敗: %w\n出力: %s", r.Ref, err, string(output))
	}
	
	r.logger.WithField("ref", r.Ref).Info("指定されたrefにチェックアウトしました")
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