package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRepository(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	repo := NewRepository("https://github.com/example/repo.git", "main", "/tmp/test", logger)

	assert.Equal(t, "https://github.com/example/repo.git", repo.URL)
	assert.Equal(t, "main", repo.Ref)
	assert.Equal(t, "/tmp/test", repo.Path)
	assert.NotNil(t, repo.logger)
}

func TestGetWorkingDirectory(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	repo := NewRepository("", "", "/tmp/test", logger)

	assert.Equal(t, "/tmp/test", repo.GetWorkingDirectory())
}

func TestValidateGitCommand(t *testing.T) {
	// この時点でgitがシステムにインストールされていることを前提とする
	// CI環境ではgitが利用可能であることが期待される
	err := ValidateGitCommand()

	// gitが利用可能な場合はエラーなし、利用できない場合はエラーあり
	// 両方のケースを許容する
	if err != nil {
		t.Logf("Git command not available: %v", err)
		assert.Contains(t, err.Error(), "gitコマンドが見つかりません")
	} else {
		t.Log("Git command is available")
	}
}

func TestCloneOrPullEmptyURL(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	repo := NewRepository("", "", "/tmp/test", logger)

	// URLが空の場合はスキップされるべき
	err := repo.CloneOrPull()
	assert.NoError(t, err)
}

func TestIsGitRepositoryNonExistentPath(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	repo := NewRepository("", "", "/non/existent/path", logger)

	// 存在しないパスはGitリポジトリではない
	isGit := repo.isGitRepository()
	assert.False(t, isGit)
}

func TestIsGitRepositoryEmptyDirectory(t *testing.T) {
	// 一時ディレクトリを作成
	tmpDir, err := os.MkdirTemp("", "git-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger := logrus.NewEntry(logrus.New())
	repo := NewRepository("", "", tmpDir, logger)

	// 空のディレクトリはGitリポジトリではない
	isGit := repo.isGitRepository()
	assert.False(t, isGit)
}

func TestDetermineWorkingDirectoryWithEnvVar(t *testing.T) {
	// 環境変数をテスト用に設定
	oldWorkDir := os.Getenv("KERUTA_WORKING_DIR")
	defer func() {
		if oldWorkDir != "" {
			os.Setenv("KERUTA_WORKING_DIR", oldWorkDir)
		} else {
			os.Unsetenv("KERUTA_WORKING_DIR")
		}
	}()

	testWorkDir := "/test/working/dir"
	os.Setenv("KERUTA_WORKING_DIR", testWorkDir)

	// テスト用のセッション設定
	templateConfig := &SessionTemplateConfig{
		RepositoryURL: "https://github.com/example/repo.git",
	}

	workDir := DetermineWorkingDirectory("test-session", templateConfig)
	assert.Equal(t, testWorkDir, workDir)
}

func TestDetermineWorkingDirectoryDefault(t *testing.T) {
	// 環境変数をクリア
	oldWorkDir := os.Getenv("KERUTA_WORKING_DIR")
	oldBaseDir := os.Getenv("KERUTA_BASE_DIR")
	defer func() {
		if oldWorkDir != "" {
			os.Setenv("KERUTA_WORKING_DIR", oldWorkDir)
		} else {
			os.Unsetenv("KERUTA_WORKING_DIR")
		}
		if oldBaseDir != "" {
			os.Setenv("KERUTA_BASE_DIR", oldBaseDir)
		} else {
			os.Unsetenv("KERUTA_BASE_DIR")
		}
	}()

	os.Unsetenv("KERUTA_WORKING_DIR")
	os.Unsetenv("KERUTA_BASE_DIR")

	// テスト用のセッション設定
	templateConfig := &SessionTemplateConfig{
		RepositoryURL: "https://github.com/example/repo.git",
	}

	workDir := DetermineWorkingDirectory("test-session-123", templateConfig)

	// ホームディレクトリまたは/tmp/kerutaベースのパスが生成される
	assert.True(t,
		strings.Contains(workDir, "test-session-123") &&
			strings.Contains(workDir, "repo"),
		"Working directory should contain session ID and repo name: %s", workDir)
}

func TestDetermineWorkingDirectoryWithCustomBaseDir(t *testing.T) {
	// 環境変数をテスト用に設定
	oldWorkDir := os.Getenv("KERUTA_WORKING_DIR")
	oldBaseDir := os.Getenv("KERUTA_BASE_DIR")
	defer func() {
		if oldWorkDir != "" {
			os.Setenv("KERUTA_WORKING_DIR", oldWorkDir)
		} else {
			os.Unsetenv("KERUTA_WORKING_DIR")
		}
		if oldBaseDir != "" {
			os.Setenv("KERUTA_BASE_DIR", oldBaseDir)
		} else {
			os.Unsetenv("KERUTA_BASE_DIR")
		}
	}()

	os.Unsetenv("KERUTA_WORKING_DIR")
	testBaseDir := "/custom/base/dir"
	os.Setenv("KERUTA_BASE_DIR", testBaseDir)

	// テスト用のセッション設定
	templateConfig := &SessionTemplateConfig{
		RepositoryURL: "https://github.com/example/my-project.git",
	}

	workDir := DetermineWorkingDirectory("session-456", templateConfig)
	expectedPath := filepath.Join(testBaseDir, "sessions", "session-456", "my-project")
	assert.Equal(t, expectedPath, workDir)
}

func TestDetermineWorkingDirectoryRepoNameExtraction(t *testing.T) {
	tests := []struct {
		name          string
		repositoryURL string
		expectedRepo  string
	}{
		{
			name:          "GitHub HTTPS URL",
			repositoryURL: "https://github.com/user/my-repo.git",
			expectedRepo:  "my-repo",
		},
		{
			name:          "GitHub HTTPS URL without .git",
			repositoryURL: "https://github.com/user/my-repo",
			expectedRepo:  "my-repo",
		},
		{
			name:          "GitLab SSH URL",
			repositoryURL: "git@gitlab.com:user/project-name.git",
			expectedRepo:  "project-name",
		},
		{
			name:          "Empty URL",
			repositoryURL: "",
			expectedRepo:  "repository",
		},
		{
			name:          "Complex project name",
			repositoryURL: "https://github.com/org/complex-project-name.git",
			expectedRepo:  "complex-project-name",
		},
	}

	// 環境変数をクリア
	oldWorkDir := os.Getenv("KERUTA_WORKING_DIR")
	oldBaseDir := os.Getenv("KERUTA_BASE_DIR")
	defer func() {
		if oldWorkDir != "" {
			os.Setenv("KERUTA_WORKING_DIR", oldWorkDir)
		} else {
			os.Unsetenv("KERUTA_WORKING_DIR")
		}
		if oldBaseDir != "" {
			os.Setenv("KERUTA_BASE_DIR", oldBaseDir)
		} else {
			os.Unsetenv("KERUTA_BASE_DIR")
		}
	}()

	os.Unsetenv("KERUTA_WORKING_DIR")
	os.Setenv("KERUTA_BASE_DIR", "/test/base")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			templateConfig := &SessionTemplateConfig{
				RepositoryURL: tt.repositoryURL,
			}

			workDir := DetermineWorkingDirectory("test-session", templateConfig)
			expectedPath := filepath.Join("/test/base", "sessions", "test-session", tt.expectedRepo)
			assert.Equal(t, expectedPath, workDir)
		})
	}
}
