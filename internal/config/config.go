package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/spf13/viper"
)

// Config はアプリケーションの設定を表します
type Config struct {
	API           APIConfig           `mapstructure:"api"`
	Logging       LoggingConfig       `mapstructure:"logging"`
	Artifacts     ArtifactsConfig     `mapstructure:"artifacts"`
	ErrorHandling ErrorHandlingConfig `mapstructure:"error_handling"`
}

// APIConfig はAPI関連の設定を表します
type APIConfig struct {
	URL     string        `mapstructure:"url"`
	Token   string        `mapstructure:"token"`
	Timeout time.Duration `mapstructure:"timeout"`
}

// LoggingConfig はログ関連の設定を表します
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// ArtifactsConfig は成果物関連の設定を表します
type ArtifactsConfig struct {
	MaxSize    int64  `mapstructure:"max_size"`
	Directory  string `mapstructure:"directory"`
	Extensions string `mapstructure:"extensions"`
}

// ErrorHandlingConfig はエラーハンドリング関連の設定を表します
type ErrorHandlingConfig struct {
	AutoFix    bool `mapstructure:"auto_fix"`
	RetryCount int  `mapstructure:"retry_count"`
}

var (
	// GlobalConfig はグローバル設定インスタンスです
	GlobalConfig *Config
)

// Init は設定を初期化します
func Init() error {
	// デフォルト設定
	setDefaults()

	// 環境変数から設定を読み込み
	loadFromEnv()

	// 設定ファイルから読み込み
	if err := loadFromFile(); err != nil {
		return fmt.Errorf("設定ファイルの読み込みに失敗: %w", err)
	}

	// 設定の検証
	if err := validate(); err != nil {
		return fmt.Errorf("設定の検証に失敗: %w", err)
	}

	return nil
}

// setDefaults はデフォルト設定を設定します
func setDefaults() {
	viper.SetDefault("api.timeout", "30s")
	viper.SetDefault("logging.level", "INFO")
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("artifacts.max_size", 100*1024*1024) // 100MB
	viper.SetDefault("artifacts.directory", "/.keruta/doc")
	viper.SetDefault("error_handling.auto_fix", true)
	viper.SetDefault("error_handling.retry_count", 3)
}

// loadFromEnv は環境変数から設定を読み込みます
func loadFromEnv() {
	// API設定
	if url := os.Getenv("KERUTA_API_URL"); url != "" {
		viper.Set("api.url", url)
	}
	if token := os.Getenv("KERUTA_API_TOKEN"); token != "" {
		viper.Set("api.token", token)
	}
	if timeout := os.Getenv("KERUTA_TIMEOUT"); timeout != "" {
		viper.Set("api.timeout", timeout)
	}

	// ログ設定
	if level := os.Getenv("KERUTA_LOG_LEVEL"); level != "" {
		viper.Set("logging.level", level)
	}

	// 成果物設定
	if dir := os.Getenv("KERUTA_ARTIFACTS_DIR"); dir != "" {
		viper.Set("artifacts.directory", dir)
	}
	if maxSize := os.Getenv("KERUTA_MAX_FILE_SIZE"); maxSize != "" {
		if size, err := strconv.ParseInt(maxSize, 10, 64); err == nil {
			viper.Set("artifacts.max_size", size*1024*1024) // MB to bytes
		}
	}

	// エラーハンドリング設定
	if autoFix := os.Getenv("KERUTA_AUTO_FIX_ENABLED"); autoFix != "" {
		if enabled, err := strconv.ParseBool(autoFix); err == nil {
			viper.Set("error_handling.auto_fix", enabled)
		}
	}
	if retryCount := os.Getenv("KERUTA_RETRY_COUNT"); retryCount != "" {
		if count, err := strconv.Atoi(retryCount); err == nil {
			viper.Set("error_handling.retry_count", count)
		}
	}
}

// loadFromFile は設定ファイルから設定を読み込みます
func loadFromFile() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("~/.keruta")
	viper.AddConfigPath("/etc/keruta")

	// 設定ファイルが存在しない場合は無視
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return err
		}
	}

	return nil
}

// validate は設定を検証します
func validate() error {
	// 必須設定のチェック
	if viper.GetString("api.url") == "" {
		return fmt.Errorf("KERUTA_API_URL が設定されていません")
	}
	
	// デーモンモード以外では KERUTA_TASK_ID が必須
	// デーモンモードではセッションIDまたはワークスペースIDが必要
	if os.Getenv("KERUTA_TASK_ID") == "" {
		sessionID := os.Getenv("KERUTA_SESSION_ID")
		workspaceID := os.Getenv("KERUTA_WORKSPACE_ID")
		coderWorkspaceID := os.Getenv("CODER_WORKSPACE_ID")
		
		// セッションIDまたはワークスペースIDのいずれかが設定されている必要がある
		// ワークスペースIDからセッションIDを動的に取得できる場合もOK
		hasSessionID := sessionID != ""
		hasWorkspaceID := workspaceID != "" || coderWorkspaceID != ""
		
		if !hasSessionID && !hasWorkspaceID {
			return fmt.Errorf("KERUTA_TASK_ID、KERUTA_SESSION_ID、またはKERUTA_WORKSPACE_ID のいずれかが設定されている必要があります")
		}
		
		// ワークスペースIDが設定されているがセッションIDが設定されていない場合、
		// 動的にセッションIDを取得を試みる（API URLが設定されている場合のみ）
		if !hasSessionID && hasWorkspaceID && viper.GetString("api.url") != "" {
			// ここでは検証のみ行い、実際の取得はGetSessionID()で行う
			// API URLが設定されていれば、後でセッションIDを取得できる可能性がある
		}
	}

	// 設定を構造体にマッピング
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return fmt.Errorf("設定のマッピングに失敗: %w", err)
	}

	GlobalConfig = &config
	return nil
}

// GetTaskID はタスクIDを取得します
func GetTaskID() string {
	return os.Getenv("KERUTA_TASK_ID")
}

// GetAPIURL はAPI URLを取得します
func GetAPIURL() string {
	return GlobalConfig.API.URL
}

// GetTimeout はタイムアウトを取得します
func GetTimeout() time.Duration {
	return GlobalConfig.API.Timeout
}

// GetAPIToken はAPIトークンを取得します
func GetAPIToken() string {
	return GlobalConfig.API.Token
}

// GetSessionID はセッションIDを取得します
func GetSessionID() string {
	if sessionID := os.Getenv("KERUTA_SESSION_ID"); sessionID != "" {
		return sessionID
	}
	
	// KERUTA_SESSION_IDが設定されていない場合、ワークスペースIDからセッションIDを取得
	workspaceID := GetWorkspaceID()
	if workspaceID != "" {
		if sessionID := getSessionIDFromWorkspace(workspaceID); sessionID != "" {
			return sessionID
		}
	}
	
	return ""
}

// getSessionIDFromWorkspace はワークスペースIDからセッションIDを取得します
func getSessionIDFromWorkspace(workspaceID string) string {
	apiURL := GetAPIURL()
	if apiURL == "" {
		return ""
	}
	
	url := fmt.Sprintf("%s/api/v1/sessions/by-workspace/%s", apiURL, workspaceID)
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	
	var session struct {
		ID string `json:"id"`
	}
	
	if err := json.Unmarshal(body, &session); err != nil {
		return ""
	}
	
	return session.ID
}

// GetWorkspaceID はワークスペースIDを取得します
func GetWorkspaceID() string {
	if workspaceID := os.Getenv("KERUTA_WORKSPACE_ID"); workspaceID != "" {
		return workspaceID
	}
	// レガシーサポート
	return os.Getenv("CODER_WORKSPACE_ID")
}

// GetPollInterval はポーリング間隔を取得します
func GetPollInterval() time.Duration {
	if interval := os.Getenv("KERUTA_POLL_INTERVAL"); interval != "" {
		if duration, err := time.ParseDuration(interval + "s"); err == nil {
			return duration
		}
	}
	return 5 * time.Second // デフォルト値
}

// GetUseHTTPInput はHTTP入力機能の有効状態を取得します
func GetUseHTTPInput() bool {
	return os.Getenv("KERUTA_USE_HTTP_INPUT") == "true"
}

// GetDaemonPort はデーモンHTTPポートを取得します
func GetDaemonPort() string {
	if port := os.Getenv("KERUTA_DAEMON_PORT"); port != "" {
		return port
	}
	return "8080" // デフォルト値
}
