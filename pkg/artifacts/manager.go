package artifacts

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
)

// Manager は成果物管理を担当します
type Manager struct {
	directory string
	maxSize   int64
}

// Artifact は成果物の情報を表します
type Artifact struct {
	Path        string
	Name        string
	Size        int64
	Description string
}

// NewManager は新しい成果物マネージャーを作成します
func NewManager() *Manager {
	return &Manager{
		directory: config.GlobalConfig.Artifacts.Directory,
		maxSize:   config.GlobalConfig.Artifacts.MaxSize,
	}
}

// CollectArtifacts は指定されたディレクトリから成果物を収集します
func (m *Manager) CollectArtifacts() ([]Artifact, error) {
	var artifacts []Artifact

	logger.WithComponent("artifacts").WithField("directory", m.directory).Debug("成果物を収集中")

	// ディレクトリが存在しない場合は空のリストを返す
	if _, err := os.Stat(m.directory); os.IsNotExist(err) {
		logger.WithComponent("artifacts").WithField("directory", m.directory).Info("成果物ディレクトリが存在しません")
		return artifacts, nil
	}

	err := filepath.WalkDir(m.directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// ディレクトリはスキップ
		if d.IsDir() {
			return nil
		}

		// 隠しファイルはスキップ
		if strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}

		// ファイル情報を取得
		info, err := d.Info()
		if err != nil {
			logger.WithComponent("artifacts").WithError(err).WithField("path", path).Warn("ファイル情報の取得に失敗")
			return nil
		}

		// ファイルサイズチェック
		if info.Size() > m.maxSize {
			logger.WithComponent("artifacts").WithFields(logrus.Fields{
				"path":     path,
				"size":     info.Size(),
				"max_size": m.maxSize,
			}).Warn("ファイルサイズが上限を超えています")
			return nil
		}

		// 相対パスを計算
		relPath, err := filepath.Rel(m.directory, path)
		if err != nil {
			logger.WithComponent("artifacts").WithError(err).WithField("path", path).Warn("相対パスの計算に失敗")
			return nil
		}

		artifact := Artifact{
			Path: path,
			Name: relPath,
			Size: info.Size(),
		}

		artifacts = append(artifacts, artifact)

		logger.WithComponent("artifacts").WithFields(logrus.Fields{
			"path": path,
			"name": relPath,
			"size": info.Size(),
		}).Debug("成果物を発見")

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("成果物の収集に失敗: %w", err)
	}

	logger.WithComponent("artifacts").WithField("count", len(artifacts)).Info("成果物の収集が完了しました")
	return artifacts, nil
}

// ValidateArtifact は成果物の妥当性をチェックします
func (m *Manager) ValidateArtifact(path string) error {
	// ファイルの存在確認
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("ファイルが存在しません: %s", path)
	}

	// ファイルサイズの確認
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("ファイル情報の取得に失敗: %w", err)
	}

	if info.Size() > m.maxSize {
		return fmt.Errorf("ファイルサイズが上限を超えています: %d > %d", info.Size(), m.maxSize)
	}

	return nil
}

// GetArtifactDescription は成果物の説明を生成します
func (m *Manager) GetArtifactDescription(artifact Artifact) string {
	ext := strings.ToLower(filepath.Ext(artifact.Name))
	
	switch ext {
	case ".txt", ".md":
		return "テキストファイル"
	case ".pdf":
		return "PDFドキュメント"
	case ".jpg", ".jpeg", ".png", ".gif":
		return "画像ファイル"
	case ".zip", ".tar", ".gz":
		return "アーカイブファイル"
	case ".json", ".xml", ".yaml", ".yml":
		return "データファイル"
	case ".log":
		return "ログファイル"
	default:
		return "成果物ファイル"
	}
}

// CreateArtifactsDirectory は成果物ディレクトリを作成します
func (m *Manager) CreateArtifactsDirectory() error {
	if err := os.MkdirAll(m.directory, 0755); err != nil {
		return fmt.Errorf("成果物ディレクトリの作成に失敗: %w", err)
	}

	logger.WithComponent("artifacts").WithField("directory", m.directory).Info("成果物ディレクトリを作成しました")
	return nil
}

// CleanupArtifacts は成果物ディレクトリをクリーンアップします
func (m *Manager) CleanupArtifacts() error {
	if err := os.RemoveAll(m.directory); err != nil {
		return fmt.Errorf("成果物ディレクトリの削除に失敗: %w", err)
	}

	logger.WithComponent("artifacts").WithField("directory", m.directory).Info("成果物ディレクトリをクリーンアップしました")
	return nil
} 