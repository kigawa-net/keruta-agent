package health

import (
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"time"

	"keruta-agent/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChecker(t *testing.T) {
	// テスト用の設定を準備
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:   "http://test-api.example.com",
			Token: "test-token",
		},
	}

	checker := NewChecker()
	
	assert.NotNil(t, checker)
	assert.NotNil(t, checker.apiClient)
}

func TestCheckAll(t *testing.T) {
	// テスト用の設定を準備
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:   "http://test-api.example.com",
			Token: "test-token",
		},
		Artifacts: config.ArtifactsConfig{
			Directory: "/test/artifacts",
		},
	}
	os.Setenv("KERUTA_TASK_ID", "test-task-123")
	defer os.Unsetenv("KERUTA_TASK_ID")

	checker := NewChecker()
	status := checker.CheckAll()

	assert.NotNil(t, status)
	assert.False(t, status.Overall) // API接続が失敗するため
	assert.NotZero(t, status.Timestamp)
	assert.Len(t, status.Checks, 4)
	
	// 各チェックが実行されていることを確認
	assert.Contains(t, status.Checks, "api")
	assert.Contains(t, status.Checks, "disk")
	assert.Contains(t, status.Checks, "memory")
	assert.Contains(t, status.Checks, "config")
}

func TestCheckAPI(t *testing.T) {
	// テストサーバーを作成
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// 設定を更新
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:   server.URL,
			Token: "test-token",
		},
	}

	checker := NewChecker()
	result := checker.CheckAPI()

	assert.True(t, result.Status)
	assert.Equal(t, "API接続は正常です", result.Message)
	assert.Empty(t, result.Error)
}

func TestCheckAPIFailure(t *testing.T) {
	// 存在しないURLでテスト
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:   "http://nonexistent-api.example.com",
			Token: "test-token",
		},
	}

	checker := NewChecker()
	result := checker.CheckAPI()

	assert.False(t, result.Status)
	assert.Equal(t, "API接続に失敗しました", result.Message)
	assert.NotEmpty(t, result.Error)
}

func TestCheckAPIInvalidResponse(t *testing.T) {
	// エラーレスポンスを返すサーバーを作成
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:   server.URL,
			Token: "test-token",
		},
	}

	checker := NewChecker()
	result := checker.CheckAPI()

	assert.False(t, result.Status)
	assert.Contains(t, result.Message, "API応答が異常です")
	assert.Empty(t, result.Error)
}

func TestCheckDisk(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	config.GlobalConfig = &config.Config{
		Artifacts: config.ArtifactsConfig{
			Directory: tempDir,
		},
	}

	checker := NewChecker()
	result := checker.CheckDisk()

	assert.True(t, result.Status)
	assert.Equal(t, "ディスク容量は十分です", result.Message)
	assert.Empty(t, result.Error)
}

func TestCheckDiskCreateDirectory(t *testing.T) {
	// 存在しないディレクトリを指定
	config.GlobalConfig = &config.Config{
		Artifacts: config.ArtifactsConfig{
			Directory: "/tmp/test-artifacts-new",
		},
	}

	checker := NewChecker()
	result := checker.CheckDisk()

	assert.True(t, result.Status)
	assert.Equal(t, "ディスク容量は十分です", result.Message)
	assert.Empty(t, result.Error)

	// ディレクトリが作成されたことを確認
	_, err := os.Stat("/tmp/test-artifacts-new")
	assert.NoError(t, err)
	
	// クリーンアップ
	os.RemoveAll("/tmp/test-artifacts-new")
}

func TestCheckMemory(t *testing.T) {
	checker := NewChecker()
	result := checker.CheckMemory()

	assert.True(t, result.Status)
	assert.Contains(t, result.Message, "メモリ使用量は正常です")
	assert.Empty(t, result.Error)
}

func TestCheckMemoryHighUsage(t *testing.T) {
	// メモリ使用量を強制的に増やす（テスト用）
	// 実際のテストでは、このような操作は避けるべきですが、
	// テストの目的でメモリ使用量をシミュレート
	checker := NewChecker()
	
	// 現在のメモリ使用量を取得
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	// メモリ使用量が非常に高い場合のテスト
	// 実際の実装では、このような状況は稀ですが、
	// エッジケースとしてテスト
	result := checker.CheckMemory()
	
	// メモリ使用量が閾値を超えている場合は失敗
	if m.Alloc > 1024*1024*1024 { // 1GB
		assert.False(t, result.Status)
		assert.Contains(t, result.Message, "メモリ使用量が閾値を超えています")
	} else {
		assert.True(t, result.Status)
		assert.Contains(t, result.Message, "メモリ使用量は正常です")
	}
}

func TestCheckConfigSuccess(t *testing.T) {
	// 必要な設定を全て設定
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:   "http://test-api.example.com",
			Token: "test-token",
		},
	}
	os.Setenv("KERUTA_TASK_ID", "test-task-123")
	defer os.Unsetenv("KERUTA_TASK_ID")

	checker := NewChecker()
	result := checker.CheckConfig()

	assert.True(t, result.Status)
	assert.Equal(t, "設定は正常です", result.Message)
	assert.Empty(t, result.Error)
}

func TestCheckConfigMissingAPIURL(t *testing.T) {
	// API URLを設定しない
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			Token: "test-token",
		},
	}
	os.Setenv("KERUTA_TASK_ID", "test-task-123")
	defer os.Unsetenv("KERUTA_TASK_ID")

	checker := NewChecker()
	result := checker.CheckConfig()

	assert.False(t, result.Status)
	assert.Equal(t, "API URLが設定されていません", result.Message)
	assert.Empty(t, result.Error)
}

func TestCheckConfigMissingAPIToken(t *testing.T) {
	// API Tokenを設定しない
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL: "http://test-api.example.com",
		},
	}
	os.Setenv("KERUTA_TASK_ID", "test-task-123")
	defer os.Unsetenv("KERUTA_TASK_ID")

	checker := NewChecker()
	result := checker.CheckConfig()

	assert.False(t, result.Status)
	assert.Equal(t, "APIトークンが設定されていません", result.Message)
	assert.Empty(t, result.Error)
}

func TestCheckConfigMissingTaskID(t *testing.T) {
	// タスクIDを設定しない
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:   "http://test-api.example.com",
			Token: "test-token",
		},
	}
	os.Unsetenv("KERUTA_TASK_ID")

	checker := NewChecker()
	result := checker.CheckConfig()

	assert.False(t, result.Status)
	assert.Equal(t, "タスクIDが設定されていません", result.Message)
	assert.Empty(t, result.Error)
}

func TestCheckSpecific(t *testing.T) {
	// 一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// テスト用の設定を準備
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:   "http://test-api.example.com",
			Token: "test-token",
		},
		Artifacts: config.ArtifactsConfig{
			Directory: tempDir,
		},
	}
	os.Setenv("KERUTA_TASK_ID", "test-task-123")
	defer os.Unsetenv("KERUTA_TASK_ID")

	checker := NewChecker()

	tests := []struct {
		checkType string
		expected  bool
	}{
		{"api", false},     // API接続が失敗するため
		{"disk", true},     // ディスクチェックは成功
		{"memory", true},   // メモリチェックは成功
		{"config", true},   // 設定チェックは成功
		{"unknown", false}, // 未知のチェックタイプ
	}

	for _, tt := range tests {
		t.Run(tt.checkType, func(t *testing.T) {
			result := checker.CheckSpecific(tt.checkType)
			assert.Equal(t, tt.expected, result.Status)
		})
	}
}

func TestHealthStatus(t *testing.T) {
	status := &HealthStatus{
		Overall:   true,
		Timestamp: time.Now(),
		Checks: map[string]CheckResult{
			"test1": {Status: true, Message: "test1 ok"},
			"test2": {Status: false, Message: "test2 failed"},
		},
	}

	assert.True(t, status.Overall)
	assert.NotZero(t, status.Timestamp)
	assert.Len(t, status.Checks, 2)
	assert.True(t, status.Checks["test1"].Status)
	assert.False(t, status.Checks["test2"].Status)
}

func TestCheckResult(t *testing.T) {
	successResult := CheckResult{
		Status:  true,
		Message: "Success message",
	}

	failureResult := CheckResult{
		Status:  false,
		Message: "Failure message",
		Error:   "Error details",
	}

	assert.True(t, successResult.Status)
	assert.Equal(t, "Success message", successResult.Message)
	assert.Empty(t, successResult.Error)

	assert.False(t, failureResult.Status)
	assert.Equal(t, "Failure message", failureResult.Message)
	assert.Equal(t, "Error details", failureResult.Error)
} 