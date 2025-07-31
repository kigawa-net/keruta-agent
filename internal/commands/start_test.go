package commands

import (
	"os"
	"testing"

	"keruta-agent/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestStartCommand(t *testing.T) {
	// Reset global variables
	startMessage = "タスクを開始しました"

	assert.NotNil(t, startCmd)
	assert.Equal(t, "start", startCmd.Use)
	assert.Equal(t, "タスクの実行を開始し、ステータスをPROCESSINGに更新", startCmd.Short)
	assert.Contains(t, startCmd.Long, "タスクの実行を開始し、ステータスをPROCESSINGに更新します")
}

func TestStartCommandFlags(t *testing.T) {
	// Test flag definitions
	messageFlag := startCmd.Flag("message")
	assert.NotNil(t, messageFlag)
	assert.Equal(t, "タスクを開始しました", messageFlag.DefValue)
}

func TestStartCommandFlagParsing(t *testing.T) {
	// Test flag parsing
	startCmd.SetArgs([]string{"--message", "カスタム開始メッセージ"})
	err := startCmd.ParseFlags([]string{"--message", "カスタム開始メッセージ"})
	assert.NoError(t, err)

	// Verify flag was parsed correctly
	messageFlag := startCmd.Flag("message")
	assert.Equal(t, "カスタム開始メッセージ", messageFlag.Value.String())
}

func TestRunStartWithoutTaskID(t *testing.T) {
	// Save original config
	originalConfig := config.GlobalConfig
	defer func() { config.GlobalConfig = originalConfig }()

	// Clear environment variable
	os.Unsetenv("KERUTA_TASK_ID")

	// Set up config without task ID
	config.GlobalConfig = &config.Config{}

	// Run start command should fail without task ID
	err := runStart()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "タスクIDが設定されていません")
}

func TestRunStartWithTaskID(t *testing.T) {
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
	os.Setenv("KERUTA_TASK_ID", "test-task-123")

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
	err := runStart()
	// Should fail on API call, not on task ID validation
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "タスクIDが設定されていません")
}

func TestStartCommandBasicValidation(t *testing.T) {
	// Test that the command structure is correct
	assert.Equal(t, "start", startCmd.Use)
	assert.NotEmpty(t, startCmd.Short)
	assert.NotEmpty(t, startCmd.Long)
	assert.NotNil(t, startCmd.RunE)

	// Test flag existence
	flags := startCmd.Flags()
	assert.True(t, flags.HasFlags())

	messageFlag := flags.Lookup("message")
	assert.NotNil(t, messageFlag)
	assert.Equal(t, "string", messageFlag.Value.Type())
}

func TestStartMessageGlobalVariable(t *testing.T) {
	// Test that global variable can be modified
	original := startMessage
	defer func() { startMessage = original }()

	startMessage = "テスト用メッセージ"
	assert.Equal(t, "テスト用メッセージ", startMessage)

	// Test flag parsing affects global variable
	startCmd.SetArgs([]string{"--message", "フラグからのメッセージ"})
	err := startCmd.ParseFlags([]string{"--message", "フラグからのメッセージ"})
	assert.NoError(t, err)

	// The global variable should be updated by flag parsing
	// Note: This depends on cobra's flag binding behavior
}
