package commands

import (
	"os"
	"testing"

	"keruta-agent/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestSuccessCommand(t *testing.T) {
	// Reset global variables
	successMessage = "タスクが正常に完了しました"
	artifactsDir = ""

	assert.NotNil(t, successCmd)
	assert.Equal(t, "success", successCmd.Use)
	assert.Equal(t, "タスクの成功を報告し、ステータスをCOMPLETEDに更新", successCmd.Short)
	assert.Contains(t, successCmd.Long, "タスクの成功を報告し、ステータスをCOMPLETEDに更新します")
}

func TestSuccessCommandFlags(t *testing.T) {
	// Test flag definitions
	messageFlag := successCmd.Flag("message")
	assert.NotNil(t, messageFlag)
	assert.Equal(t, "タスクが正常に完了しました", messageFlag.DefValue)

	artifactsDirFlag := successCmd.Flag("artifacts-dir")
	assert.NotNil(t, artifactsDirFlag)
	assert.Equal(t, "", artifactsDirFlag.DefValue)
}

func TestSuccessCommandFlagParsing(t *testing.T) {
	// Test flag parsing
	successCmd.SetArgs([]string{"--message", "カスタム成功メッセージ", "--artifacts-dir", "/tmp/artifacts"})
	err := successCmd.ParseFlags([]string{"--message", "カスタム成功メッセージ", "--artifacts-dir", "/tmp/artifacts"})
	assert.NoError(t, err)

	// Verify flags were parsed correctly
	messageFlag := successCmd.Flag("message")
	assert.Equal(t, "カスタム成功メッセージ", messageFlag.Value.String())

	artifactsDirFlag := successCmd.Flag("artifacts-dir")
	assert.Equal(t, "/tmp/artifacts", artifactsDirFlag.Value.String())
}

func TestRunSuccessWithoutTaskID(t *testing.T) {
	// Save original config
	originalConfig := config.GlobalConfig
	defer func() { config.GlobalConfig = originalConfig }()

	// Clear environment variable
	os.Unsetenv("KERUTA_TASK_ID")

	// Set up config without task ID
	config.GlobalConfig = &config.Config{}

	// Run success command should fail without task ID
	err := runSuccess()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "タスクIDが設定されていません")
}

func TestRunSuccessWithTaskID(t *testing.T) {
	// Save original environment
	originalTaskID := os.Getenv("KERUTA_TASK_ID")
	defer func() {
		if originalTaskID != "" {
			os.Setenv("KERUTA_TASK_ID", originalTaskID)
		} else {
			os.Unsetenv("KERUTA_TASK_ID")
		}
	}()

	// Set task ID environment variable
	os.Setenv("KERUTA_TASK_ID", "test-task-456")

	// Save original config
	originalConfig := config.GlobalConfig
	defer func() { config.GlobalConfig = originalConfig }()

	// Set up minimal config
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:   "http://test-api.example.com",
			Token: "test-token",
		},
	}

	// This test will fail due to API call, but we can verify it gets past the task ID check
	err := runSuccess()
	// Should fail on API call, not on task ID validation
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "タスクIDが設定されていません")
}

func TestSuccessCommandBasicValidation(t *testing.T) {
	// Test that the command structure is correct
	assert.Equal(t, "success", successCmd.Use)
	assert.NotEmpty(t, successCmd.Short)
	assert.NotEmpty(t, successCmd.Long)
	assert.NotNil(t, successCmd.RunE)

	// Test flag existence
	flags := successCmd.Flags()
	assert.True(t, flags.HasFlags())

	messageFlag := flags.Lookup("message")
	assert.NotNil(t, messageFlag)
	assert.Equal(t, "string", messageFlag.Value.Type())

	artifactsDirFlag := flags.Lookup("artifacts-dir")
	assert.NotNil(t, artifactsDirFlag)
	assert.Equal(t, "string", artifactsDirFlag.Value.Type())
}

func TestSuccessGlobalVariables(t *testing.T) {
	// Test that global variables can be modified
	originalMessage := successMessage
	originalDir := artifactsDir
	defer func() {
		successMessage = originalMessage
		artifactsDir = originalDir
	}()

	successMessage = "テスト用成功メッセージ"
	artifactsDir = "/test/artifacts"

	assert.Equal(t, "テスト用成功メッセージ", successMessage)
	assert.Equal(t, "/test/artifacts", artifactsDir)
}

func TestSuccessCommandUsage(t *testing.T) {
	// Test command usage string
	assert.Equal(t, "success", successCmd.Use)

	// Test that command has proper help text
	help := successCmd.Long
	assert.Contains(t, help, "タスクの成功を報告")
	assert.Contains(t, help, "ステータスをCOMPLETEDに更新")
	assert.Contains(t, help, "成果物")
}
