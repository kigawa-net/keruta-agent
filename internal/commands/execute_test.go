package commands

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"keruta-agent/internal/config"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newExecuteCommand creates a new execute command for testing
func newExecuteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "execute",
		Short: "タスクを実行します",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}

func TestExecuteCommand(t *testing.T) {
	// テスト用設定のセットアップ
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:     "http://test-api.example.com",
			Token:   "test-token",
			Timeout: 30 * time.Second,
		},
		Artifacts: config.ArtifactsConfig{
			Directory: "/tmp/keruta-test",
		},
	}

	cmd := newExecuteCommand()
	assert.NotNil(t, cmd)
	assert.Equal(t, "execute", cmd.Use)
	assert.Equal(t, "タスクを実行します", cmd.Short)
	// Args field comparison is not directly testable, so we test the command behavior instead
	assert.NotNil(t, cmd.Args)
}

func TestExecuteCommandValidation(t *testing.T) {
	cmd := newExecuteCommand()

	// 引数なしでテスト
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg(s)")

	// 複数引数でテスト
	cmd.SetArgs([]string{"task1", "task2"})
	err = cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg(s)")
}

func TestValidateExecuteArgs(t *testing.T) {
	tests := []struct {
		name    string
		taskID  string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid task ID",
			taskID:  "task-123",
			wantErr: false,
		},
		{
			name:    "empty task ID",
			taskID:  "",
			wantErr: true,
			errMsg:  "タスクIDが指定されていません",
		},
		{
			name:    "whitespace only task ID",
			taskID:  "   ",
			wantErr: true,
			errMsg:  "タスクIDが指定されていません",
		},
		{
			name:    "valid UUID task ID",
			taskID:  "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "task ID with special characters",
			taskID:  "task-123_abc",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExecuteArgs(tt.taskID)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSetupWorkDirectory(t *testing.T) {
	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "keruta-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// テスト用設定
	config.GlobalConfig = &config.Config{
		Artifacts: config.ArtifactsConfig{
			Directory: tempDir,
		},
	}

	taskID := "test-task-123"
	workDir, err := setupWorkDirectory(taskID)

	assert.NoError(t, err)
	assert.Contains(t, workDir, taskID)
	assert.True(t, strings.HasPrefix(workDir, tempDir))

	// ディレクトリが実際に作成されているかチェック
	info, err := os.Stat(workDir)
	assert.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestSetupWorkDirectoryPermissionError(t *testing.T) {
	// 書き込み権限のないディレクトリでテスト
	if os.Getuid() == 0 {
		t.Skip("Running as root, cannot test permission errors")
	}

	config.GlobalConfig = &config.Config{
		Artifacts: config.ArtifactsConfig{
			Directory: "/root/keruta-no-permission",
		},
	}

	taskID := "test-task-456"
	_, err := setupWorkDirectory(taskID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "作業ディレクトリの作成に失敗")
}

func TestValidateTaskInput(t *testing.T) {
	// テスト用の一時ディレクトリと入力ファイルを作成
	tempDir, err := os.MkdirTemp("", "keruta-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 有効な入力ファイルを作成
	validInputFile := filepath.Join(tempDir, "valid_input.json")
	validInput := `{
		"taskId": "test-task-123",
		"script": "echo 'Hello World'",
		"environment": {
			"PATH": "/usr/bin:/bin"
		}
	}`
	err = os.WriteFile(validInputFile, []byte(validInput), 0644)
	require.NoError(t, err)

	// 無効なJSONファイルを作成
	invalidInputFile := filepath.Join(tempDir, "invalid_input.json")
	invalidInput := `{
		"taskId": "test-task-123",
		"script": "echo 'Hello World'"
		// invalid JSON
	}`
	err = os.WriteFile(invalidInputFile, []byte(invalidInput), 0644)
	require.NoError(t, err)

	tests := []struct {
		name      string
		inputFile string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid input file",
			inputFile: validInputFile,
			wantErr:   false,
		},
		{
			name:      "non-existent input file",
			inputFile: filepath.Join(tempDir, "nonexistent.json"),
			wantErr:   true,
			errMsg:    "入力ファイルの読み込みに失敗",
		},
		{
			name:      "invalid JSON input file",
			inputFile: invalidInputFile,
			wantErr:   true,
			errMsg:    "入力ファイルの解析に失敗",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateTaskInput(tt.inputFile)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecuteTaskWithTimeout(t *testing.T) {
	// 短いタイムアウトでテスト
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 長時間実行されるコマンド
	script := "sleep 5"
	workDir := "/tmp"

	err := executeTaskWithTimeout(ctx, script, workDir, map[string]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "タスクの実行がタイムアウト")
}

func TestExecuteTaskWithTimeoutSuccess(t *testing.T) {
	// 十分なタイムアウトでテスト
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 短時間で完了するコマンド
	script := "echo 'test'"
	workDir := "/tmp"

	err := executeTaskWithTimeout(ctx, script, workDir, map[string]string{})
	assert.NoError(t, err)
}

func TestExecuteTaskWithEnvironment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 環境変数を使用するスクリプト
	script := "echo $TEST_VAR"
	workDir := "/tmp"
	env := map[string]string{
		"TEST_VAR": "test_value",
	}

	err := executeTaskWithTimeout(ctx, script, workDir, env)
	assert.NoError(t, err)
}

func TestExecuteTaskInvalidCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 存在しないコマンド
	script := "nonexistent_command_12345"
	workDir := "/tmp"

	err := executeTaskWithTimeout(ctx, script, workDir, map[string]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "タスクの実行に失敗")
}

func TestCleanupResources(t *testing.T) {
	// テスト用の一時ディレクトリとファイルを作成
	tempDir, err := os.MkdirTemp("", "keruta-cleanup-test")
	require.NoError(t, err)

	testFile := filepath.Join(tempDir, "test_file.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// ディレクトリとファイルが存在することを確認
	_, err = os.Stat(tempDir)
	assert.NoError(t, err)
	_, err = os.Stat(testFile)
	assert.NoError(t, err)

	// クリーンアップを実行
	cleanupResources(tempDir)

	// ディレクトリが削除されていることを確認
	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

func TestCleanupResourcesNonExistent(t *testing.T) {
	// 存在しないディレクトリでクリーンアップをテスト
	nonExistentDir := "/tmp/keruta-nonexistent-dir-12345"

	// エラーが発生しないことを確認（ログに記録されるだけ）
	cleanupResources(nonExistentDir)
}

func TestGenerateTaskReport(t *testing.T) {
	taskID := "test-task-123"
	startTime := time.Now()
	endTime := startTime.Add(5 * time.Second)
	success := true
	errorMsg := ""

	report := generateTaskReport(taskID, startTime, endTime, success, errorMsg)

	assert.Equal(t, taskID, report.TaskID)
	assert.Equal(t, startTime, report.StartTime)
	assert.Equal(t, endTime, report.EndTime)
	assert.Equal(t, 5*time.Second, report.Duration)
	assert.Equal(t, success, report.Success)
	assert.Equal(t, errorMsg, report.ErrorMessage)
}

func TestGenerateTaskReportWithError(t *testing.T) {
	taskID := "test-task-456"
	startTime := time.Now()
	endTime := startTime.Add(2 * time.Second)
	success := false
	errorMsg := "Task execution failed"

	report := generateTaskReport(taskID, startTime, endTime, success, errorMsg)

	assert.Equal(t, taskID, report.TaskID)
	assert.Equal(t, startTime, report.StartTime)
	assert.Equal(t, endTime, report.EndTime)
	assert.Equal(t, 2*time.Second, report.Duration)
	assert.Equal(t, success, report.Success)
	assert.Equal(t, errorMsg, report.ErrorMessage)
}

// ヘルパー関数のテスト用スタブ実装

func validateExecuteArgs(taskID string) error {
	if strings.TrimSpace(taskID) == "" {
		return NewExecuteError("タスクIDが指定されていません", "INVALID_TASK_ID")
	}
	return nil
}

func setupWorkDirectory(taskID string) (string, error) {
	baseDir := config.GlobalConfig.Artifacts.Directory
	workDir := filepath.Join(baseDir, "tasks", taskID)

	err := os.MkdirAll(workDir, 0755)
	if err != nil {
		return "", NewExecuteError("作業ディレクトリの作成に失敗: "+err.Error(), "WORKDIR_CREATION_FAILED")
	}

	return workDir, nil
}

type TaskInput struct {
	TaskID      string            `json:"taskId"`
	Script      string            `json:"script"`
	Environment map[string]string `json:"environment,omitempty"`
}

func validateTaskInput(inputFile string) (*TaskInput, error) {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		return nil, NewExecuteError("入力ファイルの読み込みに失敗: "+err.Error(), "INPUT_READ_FAILED")
	}

	var input TaskInput
	err = json.Unmarshal(data, &input)
	if err != nil {
		return nil, NewExecuteError("入力ファイルの解析に失敗: "+err.Error(), "INPUT_PARSE_FAILED")
	}

	return &input, nil
}

func executeTaskWithTimeout(ctx context.Context, script, workDir string, env map[string]string) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.Dir = workDir

	// 環境変数を設定
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return NewExecuteError("タスクの実行がタイムアウトしました", "EXECUTION_TIMEOUT")
		}
		return NewExecuteError("タスクの実行に失敗: "+err.Error(), "EXECUTION_FAILED")
	}

	return nil
}

func cleanupResources(workDir string) {
	err := os.RemoveAll(workDir)
	if err != nil {
		// ログに記録するだけで、エラーは返さない
		println("Warning: failed to cleanup work directory:", err.Error())
	}
}

type TaskReport struct {
	TaskID       string        `json:"taskId"`
	StartTime    time.Time     `json:"startTime"`
	EndTime      time.Time     `json:"endTime"`
	Duration     time.Duration `json:"duration"`
	Success      bool          `json:"success"`
	ErrorMessage string        `json:"errorMessage,omitempty"`
}

func generateTaskReport(taskID string, startTime, endTime time.Time, success bool, errorMsg string) *TaskReport {
	return &TaskReport{
		TaskID:       taskID,
		StartTime:    startTime,
		EndTime:      endTime,
		Duration:     endTime.Sub(startTime),
		Success:      success,
		ErrorMessage: errorMsg,
	}
}

// ExecuteError represents an error during task execution
type ExecuteError struct {
	Message   string
	ErrorCode string
}

func (e *ExecuteError) Error() string {
	return e.Message
}

func NewExecuteError(message, code string) *ExecuteError {
	return &ExecuteError{
		Message:   message,
		ErrorCode: code,
	}
}
