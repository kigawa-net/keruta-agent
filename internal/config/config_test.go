package config

import (
	"github.com/spf13/viper"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSetDefaults(t *testing.T) {
	// テスト用にviperをリセット
	viper.Reset()

	setDefaults()

	assert.Equal(t, "30s", viper.GetString("api.timeout"))
	assert.Equal(t, "INFO", viper.GetString("logging.level"))
	assert.Equal(t, "json", viper.GetString("logging.format"))
	assert.Equal(t, int64(100*1024*1024), viper.GetInt64("artifacts.max_size"))
	assert.Equal(t, "/.keruta/doc", viper.GetString("artifacts.directory"))
	assert.Equal(t, true, viper.GetBool("error_handling.auto_fix"))
	assert.Equal(t, 3, viper.GetInt("error_handling.retry_count"))
}

func TestLoadFromEnv(t *testing.T) {
	// テスト用にviperをリセット
	viper.Reset()
	setDefaults()

	// 環境変数を設定
	os.Setenv("KERUTA_API_URL", "http://test-api.example.com")
	os.Setenv("KERUTA_API_TOKEN", "test-token")
	os.Setenv("KERUTA_TIMEOUT", "60s")
	os.Setenv("KERUTA_LOG_LEVEL", "DEBUG")
	os.Setenv("KERUTA_ARTIFACTS_DIR", "/test/artifacts")
	os.Setenv("KERUTA_MAX_FILE_SIZE", "50")
	os.Setenv("KERUTA_AUTO_FIX_ENABLED", "false")
	os.Setenv("KERUTA_RETRY_COUNT", "5")

	defer func() {
		os.Unsetenv("KERUTA_API_URL")
		os.Unsetenv("KERUTA_API_TOKEN")
		os.Unsetenv("KERUTA_TIMEOUT")
		os.Unsetenv("KERUTA_LOG_LEVEL")
		os.Unsetenv("KERUTA_ARTIFACTS_DIR")
		os.Unsetenv("KERUTA_MAX_FILE_SIZE")
		os.Unsetenv("KERUTA_AUTO_FIX_ENABLED")
		os.Unsetenv("KERUTA_RETRY_COUNT")
	}()

	loadFromEnv()

	assert.Equal(t, "http://test-api.example.com", viper.GetString("api.url"))
	assert.Equal(t, "test-token", viper.GetString("api.token"))
	assert.Equal(t, "60s", viper.GetString("api.timeout"))
	assert.Equal(t, "DEBUG", viper.GetString("logging.level"))
	assert.Equal(t, "/test/artifacts", viper.GetString("artifacts.directory"))
	assert.Equal(t, int64(50*1024*1024), viper.GetInt64("artifacts.max_size"))
	assert.Equal(t, false, viper.GetBool("error_handling.auto_fix"))
	assert.Equal(t, 5, viper.GetInt("error_handling.retry_count"))
}

func TestLoadFromEnvInvalidValues(t *testing.T) {
	// テスト用にviperをリセット
	viper.Reset()
	setDefaults()

	// 無効な値を設定
	os.Setenv("KERUTA_MAX_FILE_SIZE", "invalid")
	os.Setenv("KERUTA_AUTO_FIX_ENABLED", "invalid")
	os.Setenv("KERUTA_RETRY_COUNT", "invalid")

	defer func() {
		os.Unsetenv("KERUTA_MAX_FILE_SIZE")
		os.Unsetenv("KERUTA_AUTO_FIX_ENABLED")
		os.Unsetenv("KERUTA_RETRY_COUNT")
	}()

	loadFromEnv()

	// デフォルト値が保持されることを確認
	assert.Equal(t, int64(100*1024*1024), viper.GetInt64("artifacts.max_size"))
	assert.Equal(t, true, viper.GetBool("error_handling.auto_fix"))
	assert.Equal(t, 3, viper.GetInt("error_handling.retry_count"))
}

func TestValidateSuccess(t *testing.T) {
	// テスト用にviperをリセット
	viper.Reset()
	setDefaults()

	// 必須設定を設定
	viper.Set("api.url", "http://test-api.example.com")
	viper.Set("api.token", "test-token")

	// 必須環境変数を設定
	os.Setenv("KERUTA_TASK_ID", "test-task-123")
	defer os.Unsetenv("KERUTA_TASK_ID")

	err := validate()

	assert.NoError(t, err)
	assert.NotNil(t, GlobalConfig)
	assert.Equal(t, "http://test-api.example.com", GlobalConfig.API.URL)
}

func TestValidateMissingAPIURL(t *testing.T) {
	// テスト用にviperをリセット
	viper.Reset()
	setDefaults()

	// API URLを設定しない
	viper.Set("api.token", "test-token")

	// 必須環境変数を設定
	os.Setenv("KERUTA_TASK_ID", "test-task-123")
	defer os.Unsetenv("KERUTA_TASK_ID")

	err := validate()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "KERUTA_API_URL が設定されていません")
}

func TestValidateMissingTaskID(t *testing.T) {
	// テスト用にviperをリセット
	viper.Reset()
	setDefaults()

	// API URLとTokenを設定
	viper.Set("api.url", "http://test-api.example.com")
	viper.Set("api.token", "test-token")

	// KERUTA_TASK_IDを設定しない

	err := validate()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "KERUTA_TASK_ID が設定されていません")
}

func TestGetTaskID(t *testing.T) {
	// 環境変数を設定
	os.Setenv("KERUTA_TASK_ID", "test-task-123")
	defer os.Unsetenv("KERUTA_TASK_ID")

	taskID := GetTaskID()
	assert.Equal(t, "test-task-123", taskID)
}

func TestGetTaskIDEmpty(t *testing.T) {
	// 環境変数をクリア
	os.Unsetenv("KERUTA_TASK_ID")

	taskID := GetTaskID()
	assert.Equal(t, "", taskID)
}

func TestGetAPIURL(t *testing.T) {
	// テスト用にviperをリセット
	viper.Reset()
	setDefaults()
	viper.Set("api.url", "http://test-api.example.com")

	// GlobalConfigを設定
	var config Config
	viper.Unmarshal(&config)
	GlobalConfig = &config

	url := GetAPIURL()
	assert.Equal(t, "http://test-api.example.com", url)
}

func TestGetTimeout(t *testing.T) {
	// テスト用にviperをリセット
	viper.Reset()
	setDefaults()
	viper.Set("api.timeout", "45s")

	// GlobalConfigを設定
	var config Config
	viper.Unmarshal(&config)
	GlobalConfig = &config

	timeout := GetTimeout()
	assert.Equal(t, 45*time.Second, timeout)
}

func TestInitSuccess(t *testing.T) {
	// テスト用にviperをリセット
	viper.Reset()

	// 環境変数を設定
	os.Setenv("KERUTA_API_URL", "http://test-api.example.com")
	os.Setenv("KERUTA_API_TOKEN", "test-token")
	os.Setenv("KERUTA_TASK_ID", "test-task-123")

	defer func() {
		os.Unsetenv("KERUTA_API_URL")
		os.Unsetenv("KERUTA_API_TOKEN")
		os.Unsetenv("KERUTA_TASK_ID")
	}()

	err := Init()

	assert.NoError(t, err)
	assert.NotNil(t, GlobalConfig)
	assert.Equal(t, "http://test-api.example.com", GlobalConfig.API.URL)
}

func TestInitFailure(t *testing.T) {
	// テスト用にviperをリセット
	viper.Reset()

	// 必須環境変数を設定しない

	err := Init()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "KERUTA_API_URL が設定されていません")
}
