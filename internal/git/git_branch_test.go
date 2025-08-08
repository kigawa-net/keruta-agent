package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		taskID    string
		expected  string
	}{
		{
			name:      "両方のIDが提供された場合",
			sessionID: "29229ea1-8c41-4ca2-b064-7a7a7672dd1a",
			taskID:    "12345678-1234-1234-1234-123456789abc",
			expected:  "keruta-task-29229ea1-12345678",
		},
		{
			name:      "セッションIDのみの場合",
			sessionID: "29229ea1-8c41-4ca2-b064-7a7a7672dd1a",
			taskID:    "",
			expected:  "keruta-session-29229ea1",
		},
		{
			name:      "タスクIDのみの場合",
			sessionID: "",
			taskID:    "12345678-1234-1234-1234-123456789abc",
			expected:  "keruta-task-12345678",
		},
		{
			name:      "短いIDの場合",
			sessionID: "short",
			taskID:    "",
			expected:  "keruta-branch-",
		},
		{
			name:      "両方とも空の場合",
			sessionID: "",
			taskID:    "",
			expected:  "",
		},
		{
			name:      "ハイフンなしのUUIDの場合",
			sessionID: "29229ea18c414ca2b0647a7a7672dd1a",
			taskID:    "",
			expected:  "keruta-session-29229ea1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateBranchName(tt.sessionID, tt.taskID)
			if tt.expected == "keruta-branch-" {
				// タイムスタンプベースの場合は接頭辞のみチェック
				assert.Contains(t, result, "keruta-branch-")
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestNewRepositoryWithBranch(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	
	repo := NewRepositoryWithBranch(
		"https://github.com/test/repo.git",
		"main",
		"/tmp/test",
		"test-branch",
		logger,
	)
	
	assert.Equal(t, "https://github.com/test/repo.git", repo.URL)
	assert.Equal(t, "main", repo.Ref)
	assert.Equal(t, "/tmp/test", repo.Path)
	assert.Equal(t, "test-branch", repo.NewBranchName)
}

func TestCreateAndCheckoutBranch(t *testing.T) {
	if !isGitAvailable() {
		t.Skip("Git command not available")
	}

	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "keruta-git-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.ErrorLevel) // テスト中のログ出力を抑制

	t.Run("新しいブランチ名が空の場合は何もしない", func(t *testing.T) {
		repo := NewRepositoryWithBranch("", "", tempDir, "", logger)
		err := repo.CreateAndCheckoutBranch()
		assert.NoError(t, err)
	})

	t.Run("Gitリポジトリではないディレクトリでエラー", func(t *testing.T) {
		testDir := filepath.Join(tempDir, "not-git-repo")
		err := os.MkdirAll(testDir, 0755)
		require.NoError(t, err)

		repo := NewRepositoryWithBranch("", "", testDir, "test-branch", logger)
		err = repo.CreateAndCheckoutBranch()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a git repository")
	})
}

func TestBranchExists(t *testing.T) {
	if !isGitAvailable() {
		t.Skip("Git command not available")
	}

	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "keruta-git-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 一時的なGitリポジトリを作成
	gitDir := filepath.Join(tempDir, "test-repo")
	err = os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Git リポジトリを初期化
	err = runGitCommand(gitDir, "init")
	require.NoError(t, err)

	// 初期コミットを作成
	err = runGitCommand(gitDir, "config", "user.name", "Test User")
	require.NoError(t, err)
	err = runGitCommand(gitDir, "config", "user.email", "test@example.com")
	require.NoError(t, err)

	// 初期ファイルを作成
	testFile := filepath.Join(gitDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	err = runGitCommand(gitDir, "add", "test.txt")
	require.NoError(t, err)
	err = runGitCommand(gitDir, "commit", "-m", "Initial commit")
	require.NoError(t, err)

	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.ErrorLevel)

	repo := NewRepositoryWithBranch("", "", gitDir, "", logger)

	t.Run("存在しないブランチ", func(t *testing.T) {
		exists := repo.branchExists("non-existent-branch")
		assert.False(t, exists)
	})

	t.Run("mainブランチは存在する", func(t *testing.T) {
		// main または master ブランチの確認
		existsMain := repo.branchExists("main")
		existsMaster := repo.branchExists("master")
		// どちらか一方は存在するはず
		assert.True(t, existsMain || existsMaster)
	})
}

// isGitAvailable は git コマンドが利用可能かチェックします
func isGitAvailable() bool {
	return ValidateGitCommand() == nil
}

// runGitCommand は指定されたディレクトリでGitコマンドを実行します
func runGitCommand(dir string, args ...string) error {
	oldDir, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(oldDir)

	if err := os.Chdir(dir); err != nil {
		return err
	}

	cmd := exec.Command("git", args...)
	return cmd.Run()
}

// runGitCommandWithOutput は指定されたディレクトリでGitコマンドを実行して出力を返します
func runGitCommandWithOutput(dir string, args ...string) ([]byte, error) {
	oldDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	defer os.Chdir(oldDir)

	if err := os.Chdir(dir); err != nil {
		return nil, err
	}

	cmd := exec.Command("git", args...)
	return cmd.Output()
}

func TestPushBranch(t *testing.T) {
	if !isGitAvailable() {
		t.Skip("Git command not available")
	}

	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "keruta-git-push-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 一時的なGitリポジトリを作成
	gitDir := filepath.Join(tempDir, "test-repo")
	err = os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Git リポジトリを初期化
	err = runGitCommand(gitDir, "init")
	require.NoError(t, err)

	// Git設定
	err = runGitCommand(gitDir, "config", "user.name", "Test User")
	require.NoError(t, err)
	err = runGitCommand(gitDir, "config", "user.email", "test@example.com")
	require.NoError(t, err)

	// 初期ファイルを作成してコミット
	testFile := filepath.Join(gitDir, "test.txt")
	err = os.WriteFile(testFile, []byte("initial content"), 0644)
	require.NoError(t, err)

	err = runGitCommand(gitDir, "add", "test.txt")
	require.NoError(t, err)
	err = runGitCommand(gitDir, "commit", "-m", "Initial commit")
	require.NoError(t, err)

	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.ErrorLevel)

	repo := NewRepositoryWithBranchAndPush("", "", gitDir, "test-branch", true, logger)

	t.Run("リモートリポジトリが設定されていない場合エラー", func(t *testing.T) {
		// 現在のブランチ名を取得して使用
		currentBranch, err := repo.getCurrentBranchName()
		require.NoError(t, err)
		
		err = repo.PushBranch(currentBranch, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fatal:")
	})
}

func TestCommitAllChanges(t *testing.T) {
	if !isGitAvailable() {
		t.Skip("Git command not available")
	}

	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "keruta-git-commit-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 一時的なGitリポジトリを作成
	gitDir := filepath.Join(tempDir, "test-repo")
	err = os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Git リポジトリを初期化
	err = runGitCommand(gitDir, "init")
	require.NoError(t, err)

	// Git設定
	err = runGitCommand(gitDir, "config", "user.name", "Test User")
	require.NoError(t, err)
	err = runGitCommand(gitDir, "config", "user.email", "test@example.com")
	require.NoError(t, err)

	// 初期ファイルを作成してコミット
	testFile := filepath.Join(gitDir, "test.txt")
	err = os.WriteFile(testFile, []byte("initial content"), 0644)
	require.NoError(t, err)

	err = runGitCommand(gitDir, "add", "test.txt")
	require.NoError(t, err)
	err = runGitCommand(gitDir, "commit", "-m", "Initial commit")
	require.NoError(t, err)

	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.ErrorLevel)

	repo := NewRepositoryWithBranchAndPush("", "", gitDir, "", false, logger)

	t.Run("変更がない場合は何もしない", func(t *testing.T) {
		err := repo.CommitAllChanges("No changes")
		assert.NoError(t, err)
	})

	t.Run("変更がある場合はコミットする", func(t *testing.T) {
		// ファイルを変更
		err = os.WriteFile(testFile, []byte("modified content"), 0644)
		require.NoError(t, err)

		// 新しいファイルを追加
		newFile := filepath.Join(gitDir, "new.txt")
		err = os.WriteFile(newFile, []byte("new file content"), 0644)
		require.NoError(t, err)

		err := repo.CommitAllChanges("Test commit message")
		assert.NoError(t, err)

		// コミットが作成されたことを確認
		output, err := runGitCommandWithOutput(gitDir, "log", "--oneline", "-1")
		require.NoError(t, err)
		assert.Contains(t, string(output), "Test commit message")
	})
}

func TestGetCurrentBranchName(t *testing.T) {
	if !isGitAvailable() {
		t.Skip("Git command not available")
	}

	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "keruta-git-branch-name-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 一時的なGitリポジトリを作成
	gitDir := filepath.Join(tempDir, "test-repo")
	err = os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Git リポジトリを初期化
	err = runGitCommand(gitDir, "init")
	require.NoError(t, err)

	// Git設定
	err = runGitCommand(gitDir, "config", "user.name", "Test User")
	require.NoError(t, err)
	err = runGitCommand(gitDir, "config", "user.email", "test@example.com")
	require.NoError(t, err)

	// 初期ファイルを作成してコミット
	testFile := filepath.Join(gitDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	err = runGitCommand(gitDir, "add", "test.txt")
	require.NoError(t, err)
	err = runGitCommand(gitDir, "commit", "-m", "Initial commit")
	require.NoError(t, err)

	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.ErrorLevel)

	repo := NewRepositoryWithBranchAndPush("", "", gitDir, "", false, logger)

	t.Run("現在のブランチ名を取得", func(t *testing.T) {
		branchName, err := repo.getCurrentBranchName()
		assert.NoError(t, err)
		// main または master のいずれかであるはず
		assert.True(t, branchName == "main" || branchName == "master")
	})
}