package artifacts

import (
	"os"
	"path/filepath"
	"testing"

	"keruta-agent/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	// テスト用の設定を準備
	config.GlobalConfig = &config.Config{
		Artifacts: config.ArtifactsConfig{
			Directory: "/test/artifacts",
			MaxSize:   100 * 1024 * 1024,
		},
	}

	manager := NewManager()

	assert.NotNil(t, manager)
	assert.Equal(t, "/test/artifacts", manager.directory)
	assert.Equal(t, int64(100*1024*1024), manager.maxSize)
}

func TestCollectArtifactsEmptyDirectory(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := &Manager{
		directory: tempDir,
		maxSize:   100 * 1024 * 1024,
	}

	artifacts, err := manager.CollectArtifacts()

	assert.NoError(t, err)
	assert.Empty(t, artifacts)
}

func TestCollectArtifactsWithFiles(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// テストファイルを作成
	testFile1 := filepath.Join(tempDir, "test1.txt")
	testFile2 := filepath.Join(tempDir, "test2.json")
	
	err = os.WriteFile(testFile1, []byte("test content 1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(testFile2, []byte(`{"test": "content"}`), 0644)
	require.NoError(t, err)

	manager := &Manager{
		directory: tempDir,
		maxSize:   100 * 1024 * 1024,
	}

	artifacts, err := manager.CollectArtifacts()

	assert.NoError(t, err)
	assert.Len(t, artifacts, 2)

	// ファイル名でソートして検証
	names := []string{artifacts[0].Name, artifacts[1].Name}
	assert.Contains(t, names, "test1.txt")
	assert.Contains(t, names, "test2.json")
}

func TestCollectArtifactsWithHiddenFiles(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 隠しファイルを作成
	hiddenFile := filepath.Join(tempDir, ".hidden.txt")
	err = os.WriteFile(hiddenFile, []byte("hidden content"), 0644)
	require.NoError(t, err)

	// 通常ファイルを作成
	normalFile := filepath.Join(tempDir, "normal.txt")
	err = os.WriteFile(normalFile, []byte("normal content"), 0644)
	require.NoError(t, err)

	manager := &Manager{
		directory: tempDir,
		maxSize:   100 * 1024 * 1024,
	}

	artifacts, err := manager.CollectArtifacts()

	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "normal.txt", artifacts[0].Name)
}

func TestCollectArtifactsWithLargeFile(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 大きなファイルを作成（1MB）
	largeFile := filepath.Join(tempDir, "large.txt")
	largeContent := make([]byte, 1024*1024)
	err = os.WriteFile(largeFile, largeContent, 0644)
	require.NoError(t, err)

	// 小さなファイルを作成
	smallFile := filepath.Join(tempDir, "small.txt")
	err = os.WriteFile(smallFile, []byte("small content"), 0644)
	require.NoError(t, err)

	manager := &Manager{
		directory: tempDir,
		maxSize:   512 * 1024, // 512KB制限
	}

	artifacts, err := manager.CollectArtifacts()

	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "small.txt", artifacts[0].Name)
}

func TestValidateArtifactSuccess(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// テストファイルを作成
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	manager := &Manager{
		directory: tempDir,
		maxSize:   100 * 1024 * 1024,
	}

	err = manager.ValidateArtifact(testFile)
	assert.NoError(t, err)
}

func TestValidateArtifactFileNotExists(t *testing.T) {
	manager := &Manager{
		directory: "/test/artifacts",
		maxSize:   100 * 1024 * 1024,
	}

	err := manager.ValidateArtifact("/nonexistent/file.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ファイルが存在しません")
}

func TestValidateArtifactTooLarge(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 大きなファイルを作成
	largeFile := filepath.Join(tempDir, "large.txt")
	largeContent := make([]byte, 1024*1024) // 1MB
	err = os.WriteFile(largeFile, largeContent, 0644)
	require.NoError(t, err)

	manager := &Manager{
		directory: tempDir,
		maxSize:   512 * 1024, // 512KB制限
	}

	err = manager.ValidateArtifact(largeFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ファイルサイズが上限を超えています")
}

func TestGetArtifactDescription(t *testing.T) {
	manager := &Manager{
		directory: "/test/artifacts",
		maxSize:   100 * 1024 * 1024,
	}

	tests := []struct {
		name     string
		expected string
	}{
		{"test.txt", "テキストファイル"},
		{"README.md", "テキストファイル"},
		{"document.pdf", "PDFドキュメント"},
		{"image.jpg", "画像ファイル"},
		{"image.png", "画像ファイル"},
		{"archive.zip", "アーカイブファイル"},
		{"data.json", "データファイル"},
		{"config.yaml", "データファイル"},
		{"app.log", "ログファイル"},
		{"unknown.xyz", "成果物ファイル"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := Artifact{Name: tt.name}
			description := manager.GetArtifactDescription(artifact)
			assert.Equal(t, tt.expected, description)
		})
	}
}

func TestCreateArtifactsDirectory(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	newDir := filepath.Join(tempDir, "new-artifacts")
	manager := &Manager{
		directory: newDir,
		maxSize:   100 * 1024 * 1024,
	}

	err = manager.CreateArtifactsDirectory()
	assert.NoError(t, err)

	// ディレクトリが作成されたことを確認
	_, err = os.Stat(newDir)
	assert.NoError(t, err)
}

func TestCreateArtifactsDirectoryAlreadyExists(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager := &Manager{
		directory: tempDir,
		maxSize:   100 * 1024 * 1024,
	}

	// 既に存在するディレクトリに対して実行
	err = manager.CreateArtifactsDirectory()
	assert.NoError(t, err)
}

func TestCleanupArtifacts(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)

	// テストファイルを作成
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	manager := &Manager{
		directory: tempDir,
		maxSize:   100 * 1024 * 1024,
	}

	err = manager.CleanupArtifacts()
	assert.NoError(t, err)

	// ディレクトリが削除されたことを確認
	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

func TestCollectArtifactsWithSubdirectories(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// サブディレクトリを作成
	subDir := filepath.Join(tempDir, "subdir")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	// サブディレクトリ内にファイルを作成
	subFile := filepath.Join(subDir, "subfile.txt")
	err = os.WriteFile(subFile, []byte("sub content"), 0644)
	require.NoError(t, err)

	// ルートディレクトリにファイルを作成
	rootFile := filepath.Join(tempDir, "rootfile.txt")
	err = os.WriteFile(rootFile, []byte("root content"), 0644)
	require.NoError(t, err)

	manager := &Manager{
		directory: tempDir,
		maxSize:   100 * 1024 * 1024,
	}

	artifacts, err := manager.CollectArtifacts()

	assert.NoError(t, err)
	assert.Len(t, artifacts, 2)

	// ファイル名でソートして検証
	names := []string{artifacts[0].Name, artifacts[1].Name}
	assert.Contains(t, names, "rootfile.txt")
	assert.Contains(t, names, "subdir/subfile.txt")
} 