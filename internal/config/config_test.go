package config

import (
	"github.com/spf13/viper"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	// 通常モード（非デーモンモード）での動作をテスト
	// os.Argsを通常モードに設定
	originalArgs := os.Args
	os.Args = []string{"keruta-agent", "execute"}
	defer func() { os.Args = originalArgs }()

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

func TestGetCoderWorkspaceName(t *testing.T) {
	// 環境変数を設定
	os.Setenv("CODER_WORKSPACE_NAME", "test-workspace")
	defer os.Unsetenv("CODER_WORKSPACE_NAME")

	workspaceName := GetCoderWorkspaceName()

	assert.Equal(t, "test-workspace", workspaceName)
}

func TestGetCoderWorkspaceNameEmpty(t *testing.T) {
	// 環境変数を設定しない
	os.Unsetenv("CODER_WORKSPACE_NAME")

	workspaceName := GetCoderWorkspaceName()

	assert.Equal(t, "", workspaceName)
}

func TestGetWorkspaceID(t *testing.T) {
	// KERUTA_WORKSPACE_IDが設定されている場合
	os.Setenv("KERUTA_WORKSPACE_ID", "keruta-workspace-123")
	defer os.Unsetenv("KERUTA_WORKSPACE_ID")

	workspaceID := GetWorkspaceID()
	assert.Equal(t, "keruta-workspace-123", workspaceID)
}

func TestGetWorkspaceIDLegacy(t *testing.T) {
	// KERUTA_WORKSPACE_IDが設定されていない場合、CODER_WORKSPACE_IDを使用
	os.Unsetenv("KERUTA_WORKSPACE_ID")
	os.Setenv("CODER_WORKSPACE_ID", "legacy-workspace-456")
	defer os.Unsetenv("CODER_WORKSPACE_ID")

	workspaceID := GetWorkspaceID()
	assert.Equal(t, "legacy-workspace-456", workspaceID)
}

func TestGetWorkspaceIDEmpty(t *testing.T) {
	// 両方とも設定されていない場合
	os.Unsetenv("KERUTA_WORKSPACE_ID")
	os.Unsetenv("CODER_WORKSPACE_ID")

	workspaceID := GetWorkspaceID()
	assert.Equal(t, "", workspaceID)
}

func TestGetSessionID(t *testing.T) {
	// KERUTA_SESSION_IDが設定されている場合
	os.Setenv("KERUTA_SESSION_ID", "test-session-789")
	defer os.Unsetenv("KERUTA_SESSION_ID")

	sessionID := GetSessionID()
	assert.Equal(t, "test-session-789", sessionID)
}

func TestGetSessionIDFromWorkspace(t *testing.T) {
	// SESSION_IDが設定されていない場合、WORKSPACE_IDから取得を試みる
	os.Unsetenv("KERUTA_SESSION_ID")
	os.Setenv("KERUTA_WORKSPACE_ID", "test-workspace-456")

	// GlobalConfigを設定
	viper.Reset()
	setDefaults()
	viper.Set("api.url", "http://test-api.example.com")
	var config Config
	viper.Unmarshal(&config)
	GlobalConfig = &config

	defer func() {
		os.Unsetenv("KERUTA_WORKSPACE_ID")
		GlobalConfig = nil
	}()

	// API呼び出しが発生するが、実際のAPIサーバーがないため空文字列が返される
	sessionID := GetSessionID()
	assert.Equal(t, "", sessionID) // APIサーバーがないため空文字列
}

func TestGetSessionIDEmpty(t *testing.T) {
	// SESSION_IDもWORKSPACE_IDも設定されていない場合
	os.Unsetenv("KERUTA_SESSION_ID")
	os.Unsetenv("KERUTA_WORKSPACE_ID")
	os.Unsetenv("CODER_WORKSPACE_ID")

	sessionID := GetSessionID()
	assert.Equal(t, "", sessionID)
}

func TestGetPollInterval(t *testing.T) {
	// デフォルト値のテスト
	os.Unsetenv("KERUTA_POLL_INTERVAL")
	interval := GetPollInterval()
	assert.Equal(t, 5*time.Second, interval)
}

func TestGetPollIntervalCustom(t *testing.T) {
	// カスタム値のテスト
	os.Setenv("KERUTA_POLL_INTERVAL", "10")
	defer os.Unsetenv("KERUTA_POLL_INTERVAL")
	interval := GetPollInterval()
	assert.Equal(t, 10*time.Second, interval)
}

func TestGetPollIntervalInvalid(t *testing.T) {
	// 無効な値のテスト（デフォルト値が返される）
	os.Setenv("KERUTA_POLL_INTERVAL", "invalid")
	defer os.Unsetenv("KERUTA_POLL_INTERVAL")
	interval := GetPollInterval()
	assert.Equal(t, 5*time.Second, interval)
}

func TestGetUseHTTPInput(t *testing.T) {
	// デフォルト値のテスト（false）
	os.Unsetenv("KERUTA_USE_HTTP_INPUT")
	useHTTP := GetUseHTTPInput()
	assert.False(t, useHTTP)
}

func TestGetUseHTTPInputEnabled(t *testing.T) {
	// 有効にした場合のテスト
	os.Setenv("KERUTA_USE_HTTP_INPUT", "true")
	defer os.Unsetenv("KERUTA_USE_HTTP_INPUT")
	useHTTP := GetUseHTTPInput()
	assert.True(t, useHTTP)
}

func TestGetUseHTTPInputDisabled(t *testing.T) {
	// 明示的に無効にした場合のテスト
	os.Setenv("KERUTA_USE_HTTP_INPUT", "false")
	defer os.Unsetenv("KERUTA_USE_HTTP_INPUT")
	useHTTP := GetUseHTTPInput()
	assert.False(t, useHTTP)
}

func TestGetDaemonPort(t *testing.T) {
	// デフォルト値のテスト
	os.Unsetenv("KERUTA_DAEMON_PORT")
	port := GetDaemonPort()
	assert.Equal(t, "8080", port)
}

func TestGetDaemonPortCustom(t *testing.T) {
	// カスタム値のテスト
	os.Setenv("KERUTA_DAEMON_PORT", "9090")
	defer os.Unsetenv("KERUTA_DAEMON_PORT")
	port := GetDaemonPort()
	assert.Equal(t, "9090", port)
}

func TestGetAPIToken(t *testing.T) {
	// GlobalConfigを設定
	viper.Reset()
	setDefaults()
	viper.Set("api.token", "test-get-api-token")
	var config Config
	viper.Unmarshal(&config)
	GlobalConfig = &config

	defer func() {
		GlobalConfig = nil
	}()

	apiToken := GetAPIToken()
	assert.Equal(t, "test-get-api-token", apiToken)
}

func TestValidateWithSessionID(t *testing.T) {
	viper.Reset()
	setDefaults()

	// API URLを設定
	viper.Set("api.url", "http://test-api.example.com")

	// SESSION_IDを設定（TASK_IDなし）
	// デーモンモードをシミュレート
	originalArgs := os.Args
	os.Args = []string{"keruta-agent", "daemon"}
	os.Setenv("KERUTA_SESSION_ID", "test-session-123")
	os.Unsetenv("KERUTA_TASK_ID")

	defer func() {
		os.Args = originalArgs
		os.Unsetenv("KERUTA_SESSION_ID")
		viper.Reset()
		GlobalConfig = nil
	}()

	err := validate()
	assert.NoError(t, err)
	assert.NotNil(t, GlobalConfig)
}

func TestValidateWithWorkspaceID(t *testing.T) {
	viper.Reset()
	setDefaults()

	// API URLを設定
	viper.Set("api.url", "http://test-api.example.com")

	// WORKSPACE_IDを設定（TASK_ID、SESSION_IDなし）
	// デーモンモードをシミュレート
	originalArgs := os.Args
	os.Args = []string{"keruta-agent", "daemon"}
	os.Setenv("KERUTA_WORKSPACE_ID", "test-workspace-123")
	os.Unsetenv("KERUTA_TASK_ID")
	os.Unsetenv("KERUTA_SESSION_ID")

	defer func() {
		os.Args = originalArgs
		os.Unsetenv("KERUTA_WORKSPACE_ID")
		viper.Reset()
		GlobalConfig = nil
	}()

	err := validate()
	assert.NoError(t, err)
	assert.NotNil(t, GlobalConfig)
}

func TestValidateWithCoderWorkspaceID(t *testing.T) {
	viper.Reset()
	setDefaults()

	// API URLを設定
	viper.Set("api.url", "http://test-api.example.com")

	// CODER_WORKSPACE_IDを設定
	// デーモンモードをシミュレート
	originalArgs := os.Args
	os.Args = []string{"keruta-agent", "daemon"}
	os.Setenv("CODER_WORKSPACE_ID", "coder-workspace-123")
	os.Unsetenv("KERUTA_TASK_ID")
	os.Unsetenv("KERUTA_SESSION_ID")
	os.Unsetenv("KERUTA_WORKSPACE_ID")

	defer func() {
		os.Args = originalArgs
		os.Unsetenv("CODER_WORKSPACE_ID")
		viper.Reset()
		GlobalConfig = nil
	}()

	err := validate()
	assert.NoError(t, err)
	assert.NotNil(t, GlobalConfig)
}

func TestValidateWithCoderWorkspaceName(t *testing.T) {
	viper.Reset()
	setDefaults()

	// API URLを設定
	viper.Set("api.url", "http://test-api.example.com")

	// CODER_WORKSPACE_NAMEを設定
	// デーモンモードをシミュレート
	originalArgs := os.Args
	os.Args = []string{"keruta-agent", "daemon"}
	os.Setenv("CODER_WORKSPACE_NAME", "my-workspace")
	os.Unsetenv("KERUTA_TASK_ID")
	os.Unsetenv("KERUTA_SESSION_ID")
	os.Unsetenv("KERUTA_WORKSPACE_ID")
	os.Unsetenv("CODER_WORKSPACE_ID")

	defer func() {
		os.Args = originalArgs
		os.Unsetenv("CODER_WORKSPACE_NAME")
		viper.Reset()
		GlobalConfig = nil
	}()

	err := validate()
	assert.NoError(t, err)
	assert.NotNil(t, GlobalConfig)
}

// デーモンモード用のテストケース
func TestValidateDaemonModeWithoutIDsButWithAPIURL(t *testing.T) {
	viper.Reset()
	setDefaults()

	// API URLを設定
	viper.Set("api.url", "http://test-api.example.com")

	// デーモンモードをシミュレート、IDなし
	originalArgs := os.Args
	os.Args = []string{"keruta-agent", "daemon"}
	os.Unsetenv("KERUTA_TASK_ID")
	os.Unsetenv("KERUTA_SESSION_ID")
	os.Unsetenv("KERUTA_WORKSPACE_ID")
	os.Unsetenv("CODER_WORKSPACE_ID")
	os.Unsetenv("CODER_WORKSPACE_NAME")

	defer func() {
		os.Args = originalArgs
		viper.Reset()
		GlobalConfig = nil
	}()

	// API URLが設定されていれば、後でワークスペース名から取得を試みることができるため、エラーにならない
	err := validate()
	assert.NoError(t, err)
	assert.NotNil(t, GlobalConfig)
}

func TestConfigStructInitialization(t *testing.T) {
	// 設定構造体が正しく初期化されることをテスト
	viper.Reset()
	setDefaults()
	viper.Set("api.url", "http://test.example.com")
	viper.Set("api.token", "test-token")
	viper.Set("api.timeout", "45s")
	viper.Set("logging.level", "DEBUG")
	viper.Set("logging.format", "text")
	viper.Set("artifacts.max_size", int64(50*1024*1024))
	viper.Set("artifacts.directory", "/test/artifacts")
	viper.Set("artifacts.extensions", ".txt,.log")
	viper.Set("error_handling.auto_fix", false)
	viper.Set("error_handling.retry_count", 5)

	os.Setenv("KERUTA_TASK_ID", "test-task-123")
	defer os.Unsetenv("KERUTA_TASK_ID")

	err := validate()
	require.NoError(t, err)

	assert.Equal(t, "http://test.example.com", GlobalConfig.API.URL)
	assert.Equal(t, "test-token", GlobalConfig.API.Token)
	assert.Equal(t, 45*time.Second, GlobalConfig.API.Timeout)
	assert.Equal(t, "DEBUG", GlobalConfig.Logging.Level)
	assert.Equal(t, "text", GlobalConfig.Logging.Format)
	assert.Equal(t, int64(50*1024*1024), GlobalConfig.Artifacts.MaxSize)
	assert.Equal(t, "/test/artifacts", GlobalConfig.Artifacts.Directory)
	assert.Equal(t, ".txt,.log", GlobalConfig.Artifacts.Extensions)
	assert.Equal(t, false, GlobalConfig.ErrorHandling.AutoFix)
	assert.Equal(t, 5, GlobalConfig.ErrorHandling.RetryCount)

	GlobalConfig = nil
}
