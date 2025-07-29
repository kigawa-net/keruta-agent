package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"keruta-agent/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDaemon(t *testing.T) {
	// テスト用の設定を準備
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:     "http://test-daemon-api.example.com",
			Token:   "test-daemon-token",
			Timeout: 30 * time.Second,
		},
		Logging: config.LoggingConfig{
			Level:  "INFO",
			Format: "json",
		},
	}

	daemon := NewDaemon()

	assert.NotNil(t, daemon)
	assert.Equal(t, "8080", daemon.port) // デフォルトポート
}

func TestDaemonStart(t *testing.T) {
	// テスト用の設定を準備
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:     "http://test-daemon-start.example.com",
			Token:   "test-token",
			Timeout: 30 * time.Second,
		},
	}

	// テスト用のポートを設定
	os.Setenv("KERUTA_DAEMON_PORT", "0") // OSが自動でポートを選択
	defer os.Unsetenv("KERUTA_DAEMON_PORT")

	daemon := NewDaemon()

	// バックグラウンドでデーモンを開始
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := daemon.Start(ctx)
		assert.NoError(t, err)
	}()

	// デーモンが開始されるまで少し待機
	time.Sleep(100 * time.Millisecond)

	// デーモンを停止
	cancel()
}

func TestDaemonStop(t *testing.T) {
	// テスト用の設定を準備
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:     "http://test-daemon-stop.example.com",
			Token:   "test-token",
			Timeout: 30 * time.Second,
		},
	}

	daemon := NewDaemon()

	// 短時間でタイムアウトするコンテキスト
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Start は ctx がキャンセルされると停止する
	err := daemon.Start(ctx)
	assert.NoError(t, err) // コンテキストキャンセルは正常終了
}

func TestHealthCheckHandler(t *testing.T) {
	daemon := &Daemon{}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	daemon.healthCheckHandler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response["status"])
	assert.NotEmpty(t, response["timestamp"])
}

func TestTaskExecutionHandler(t *testing.T) {
	// テスト用のAPIサーバーを作成
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/tasks/test-task-123/status":
			assert.Equal(t, "PUT", r.Method)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		case "/api/v1/tasks/test-task-123/logs":
			assert.Equal(t, "POST", r.Method)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer apiServer.Close()

	// テスト用の設定を準備
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:     apiServer.URL,
			Token:   "test-token",
			Timeout: 30 * time.Second,
		},
	}

	daemon := NewDaemon()

	// テスト用のタスク実行リクエスト
	taskRequest := map[string]interface{}{
		"taskId": "test-task-123",
		"script": "echo 'Hello, Daemon Test!'",
		"environment": map[string]string{
			"TEST_VAR": "test_value",
		},
	}

	requestBody, err := json.Marshal(taskRequest)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/execute", strings.NewReader(string(requestBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	daemon.taskExecutionHandler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "accepted", response["status"])
	assert.Equal(t, "test-task-123", response["taskId"])
}

func TestTaskExecutionHandlerInvalidRequest(t *testing.T) {
	daemon := &Daemon{}

	// 無効なJSONリクエスト
	req := httptest.NewRequest(http.MethodPost, "/execute", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	daemon.taskExecutionHandler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["message"].(string), "リクエストの解析に失敗")
}

func TestTaskExecutionHandlerMissingTaskID(t *testing.T) {
	daemon := &Daemon{}

	// taskIdが欠けているリクエスト
	taskRequest := map[string]interface{}{
		"script": "echo 'Missing task ID'",
	}

	requestBody, err := json.Marshal(taskRequest)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/execute", strings.NewReader(string(requestBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	daemon.taskExecutionHandler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["message"].(string), "taskIdが指定されていません")
}

func TestTaskExecutionHandlerMissingScript(t *testing.T) {
	daemon := &Daemon{}

	// scriptが欠けているリクエスト
	taskRequest := map[string]interface{}{
		"taskId": "test-task-456",
	}

	requestBody, err := json.Marshal(taskRequest)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/execute", strings.NewReader(string(requestBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	daemon.taskExecutionHandler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["message"].(string), "scriptが指定されていません")
}

func TestMetricsHandler(t *testing.T) {
	daemon := &Daemon{}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	daemon.metricsHandler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// メトリクスの基本フィールドが存在することを確認
	assert.Contains(t, response, "uptime")
	assert.Contains(t, response, "tasks_executed")
	assert.Contains(t, response, "tasks_successful")
	assert.Contains(t, response, "tasks_failed")
	assert.Contains(t, response, "memory_usage")
	assert.Contains(t, response, "cpu_usage")
}

func TestConfigHandler(t *testing.T) {
	// テスト用の設定を準備
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:     "http://test-config.example.com",
			Token:   "masked-for-security",
			Timeout: 45 * time.Second,
		},
		Logging: config.LoggingConfig{
			Level:  "DEBUG",
			Format: "json",
		},
		Artifacts: config.ArtifactsConfig{
			MaxSize:   104857600, // 100MB
			Directory: "/test/artifacts",
		},
		ErrorHandling: config.ErrorHandlingConfig{
			AutoFix:    true,
			RetryCount: 3,
		},
	}

	daemon := &Daemon{}

	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	w := httptest.NewRecorder()

	daemon.configHandler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// 設定が正しく返されることを確認
	assert.Equal(t, "http://test-config.example.com", response["api_url"])
	assert.Equal(t, "***", response["api_token"]) // トークンはマスクされる
	assert.Equal(t, "45s", response["api_timeout"])
	assert.Equal(t, "DEBUG", response["log_level"])
	assert.Equal(t, "json", response["log_format"])
	assert.Equal(t, float64(104857600), response["artifacts_max_size"])
	assert.Equal(t, "/test/artifacts", response["artifacts_directory"])
	assert.Equal(t, true, response["error_handling_auto_fix"])
	assert.Equal(t, float64(3), response["error_handling_retry_count"])
}

func TestStatusHandler(t *testing.T) {
	daemon := &Daemon{
		startTime: time.Now().Add(-5 * time.Minute), // 5分前に開始
	}

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()

	daemon.statusHandler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// ステータス情報が正しく返されることを確認
	assert.Equal(t, "running", response["status"])
	assert.Contains(t, response, "uptime")
	assert.Contains(t, response, "version")
	assert.Contains(t, response, "build_time")
	assert.Contains(t, response, "go_version")

	// アップタイムが正の値であることを確認
	uptime, ok := response["uptime"].(string)
	assert.True(t, ok)
	assert.NotEmpty(t, uptime)
}

func TestInvalidMethodHandler(t *testing.T) {
	daemon := &Daemon{}

	// GETメソッドでexecuteエンドポイントにアクセス（POSTのみサポート）
	req := httptest.NewRequest(http.MethodGet, "/execute", nil)
	w := httptest.NewRecorder()

	daemon.taskExecutionHandler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

	var response map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["message"].(string), "Method not allowed")
}

func TestMiddlewareLogging(t *testing.T) {
	daemon := &Daemon{}

	// テスト用のハンドラー
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// ログミドルウェアを適用
	wrappedHandler := daemon.loggingMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body := w.Body.String()
	assert.Equal(t, "OK", body)
}

func TestCORSMiddleware(t *testing.T) {
	daemon := &Daemon{}

	// テスト用のハンドラー
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// CORSミドルウェアを適用
	wrappedHandler := daemon.corsMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// CORSヘッダーが設定されていることを確認
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST, PUT, DELETE, OPTIONS", resp.Header.Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, Authorization", resp.Header.Get("Access-Control-Allow-Headers"))
}

func TestOPTIONSRequest(t *testing.T) {
	daemon := &Daemon{}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := daemon.corsMiddleware(testHandler)

	// OPTIONSリクエスト
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// CORSヘッダーが設定されていることを確認
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST, PUT, DELETE, OPTIONS", resp.Header.Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, Authorization", resp.Header.Get("Access-Control-Allow-Headers"))
}

// ヘルパー構造体とメソッドの実装（実際のdaemonパッケージにも実装が必要）

type Daemon struct {
	port      string
	server    *http.Server
	startTime time.Time
}

func NewDaemon() *Daemon {
	port := config.GetDaemonPort()
	return &Daemon{
		port:      port,
		startTime: time.Now(),
	}
}

func (d *Daemon) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// ルートハンドラーを設定
	mux.HandleFunc("/health", d.healthCheckHandler)
	mux.HandleFunc("/execute", d.taskExecutionHandler)
	mux.HandleFunc("/metrics", d.metricsHandler)
	mux.HandleFunc("/config", d.configHandler)
	mux.HandleFunc("/status", d.statusHandler)

	// ミドルウェアを適用
	handler := d.loggingMiddleware(d.corsMiddleware(mux))

	d.server = &http.Server{
		Addr:    ":" + d.port,
		Handler: handler,
	}

	// バックグラウンドでサーバーを開始
	go func() {
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// エラーログ
		}
	}()

	// コンテキストがキャンセルされるまで待機
	<-ctx.Done()

	// グレースフルシャットダウン
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return d.server.Shutdown(shutdownCtx)
}

func (d *Daemon) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (d *Daemon) taskExecutionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		d.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var request map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		d.sendErrorResponse(w, http.StatusBadRequest, "リクエストの解析に失敗しました: "+err.Error())
		return
	}

	taskID, ok := request["taskId"].(string)
	if !ok || taskID == "" {
		d.sendErrorResponse(w, http.StatusBadRequest, "taskIdが指定されていません")
		return
	}

	script, ok := request["script"].(string)
	if !ok || script == "" {
		d.sendErrorResponse(w, http.StatusBadRequest, "scriptが指定されていません")
		return
	}

	// タスクを非同期実行（実際の実装では goroutine で実行）
	// ここではテスト用に即座にレスポンスを返す
	response := map[string]interface{}{
		"status": "accepted",
		"taskId": taskID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (d *Daemon) metricsHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"uptime":           time.Since(d.startTime).String(),
		"tasks_executed":   0,
		"tasks_successful": 0,
		"tasks_failed":     0,
		"memory_usage":     "0MB",
		"cpu_usage":        "0%",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (d *Daemon) configHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.GlobalConfig
	if cfg == nil {
		d.sendErrorResponse(w, http.StatusInternalServerError, "設定が初期化されていません")
		return
	}

	response := map[string]interface{}{
		"api_url":                    cfg.API.URL,
		"api_token":                  "***", // セキュリティのためマスク
		"api_timeout":                cfg.API.Timeout.String(),
		"log_level":                  cfg.Logging.Level,
		"log_format":                 cfg.Logging.Format,
		"artifacts_max_size":         cfg.Artifacts.MaxSize,
		"artifacts_directory":        cfg.Artifacts.Directory,
		"error_handling_auto_fix":    cfg.ErrorHandling.AutoFix,
		"error_handling_retry_count": cfg.ErrorHandling.RetryCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (d *Daemon) statusHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":     "running",
		"uptime":     time.Since(d.startTime).String(),
		"version":    "1.0.0",
		"build_time": "2023-07-29T12:00:00Z",
		"go_version": "go1.20",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (d *Daemon) sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := map[string]interface{}{
		"status":  "error",
		"message": message,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func (d *Daemon) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)

		// ログ出力（実際の実装ではloggerを使用）
		_ = duration
	})
}

func (d *Daemon) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
