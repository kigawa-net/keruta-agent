package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"keruta-agent/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	// テスト用の設定を準備
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:     "http://test-api.example.com",
			Token:   "test-token",
			Timeout: 30 * 1000000000, // 30秒
		},
	}

	client := NewClient()

	assert.NotNil(t, client)
	assert.Equal(t, "http://test-api.example.com", client.baseURL)
	assert.Equal(t, "test-token", client.token)
	assert.NotNil(t, client.httpClient)
}

func TestUpdateTaskStatus(t *testing.T) {
	// テストサーバーを作成
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// リクエストの検証
		assert.Equal(t, "PUT", r.Method)
		assert.Equal(t, "/api/v1/tasks/test-task-123/status", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		// リクエストボディの検証
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req TaskUpdateRequest
		err = json.Unmarshal(body, &req)
		require.NoError(t, err)

		assert.Equal(t, TaskStatusCompleted, req.Status)
		assert.Equal(t, "Task completed successfully", req.Message)
		assert.Equal(t, 100, req.Progress)
		assert.Equal(t, "", req.ErrorCode)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// クライアントを作成
	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	err := client.UpdateTaskStatus("test-task-123", TaskStatusCompleted, "Task completed successfully", 100, "")

	assert.NoError(t, err)
}

func TestUpdateTaskStatusFailure(t *testing.T) {
	// エラーレスポンスを返すサーバーを作成
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	err := client.UpdateTaskStatus("test-task-123", TaskStatusFailed, "Task failed", 0, "ERROR_001")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API呼び出しが失敗しました")
	assert.Contains(t, err.Error(), "500")
}

func TestUpdateTaskStatusNetworkError(t *testing.T) {
	// 存在しないURLでテスト
	client := &Client{
		baseURL: "http://nonexistent-api.example.com",
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 1 * 1000000000, // 1秒
		},
	}

	err := client.UpdateTaskStatus("test-task-123", TaskStatusFailed, "Task failed", 0, "ERROR_001")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API呼び出しに失敗")
}

func TestSendLog(t *testing.T) {
	// テストサーバーを作成
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// リクエストの検証
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/tasks/test-task-123/logs", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		// リクエストボディの検証
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req LogRequest
		err = json.Unmarshal(body, &req)
		require.NoError(t, err)

		assert.Equal(t, "INFO", req.Level)
		assert.Equal(t, "Test log message", req.Message)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	err := client.SendLog("test-task-123", "INFO", "Test log message")

	assert.NoError(t, err)
}

func TestSendLogFailure(t *testing.T) {
	// エラーレスポンスを返すサーバーを作成
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid log format"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	err := client.SendLog("test-task-123", "ERROR", "Test error message")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API呼び出しが失敗しました")
	assert.Contains(t, err.Error(), "400")
}

func TestUploadArtifact(t *testing.T) {
	// テストサーバーを作成
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// リクエストの検証
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/tasks/test-task-123/artifacts", r.URL.Path)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		// マルチパートフォームの解析
		err := r.ParseMultipartForm(32 << 20)
		require.NoError(t, err)

		// ファイルの検証
		file, header, err := r.FormFile("file")
		require.NoError(t, err)
		defer file.Close()

		assert.Equal(t, "test.txt", header.Filename)

		// ファイル内容の検証
		content, err := io.ReadAll(file)
		require.NoError(t, err)
		assert.Equal(t, "test content", string(content))

		// 説明フィールドの検証
		description := r.FormValue("description")
		assert.Equal(t, "Test artifact", description)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// 一時ファイルを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	err = client.UploadArtifact("test-task-123", testFile, "Test artifact")

	assert.NoError(t, err)
}

func TestUploadArtifactFileNotExists(t *testing.T) {
	client := &Client{
		baseURL: "http://test-api.example.com",
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	err := client.UploadArtifact("test-task-123", "/nonexistent/file.txt", "Test artifact")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ファイルのオープンに失敗")
}

func TestUploadArtifactFailure(t *testing.T) {
	// エラーレスポンスを返すサーバーを作成
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.Write([]byte(`{"error": "file too large"}`))
	}))
	defer server.Close()

	// 一時ファイルを作成
	tempDir, err := os.MkdirTemp("", "test-artifacts")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	err = client.UploadArtifact("test-task-123", testFile, "Test artifact")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API呼び出しが失敗しました")
	assert.Contains(t, err.Error(), "413")
}

func TestCreateAutoFixTask(t *testing.T) {
	// テストサーバーを作成
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// リクエストの検証
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/tasks/test-task-123/auto-fix", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		// リクエストボディの検証
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]string
		err = json.Unmarshal(body, &req)
		require.NoError(t, err)

		assert.Equal(t, "Test error message", req["errorMessage"])
		assert.Equal(t, "ERROR_001", req["errorCode"])

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	err := client.CreateAutoFixTask("test-task-123", "Test error message", "ERROR_001")

	assert.NoError(t, err)
}

func TestCreateAutoFixTaskFailure(t *testing.T) {
	// エラーレスポンスを返すサーバーを作成
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "auto-fix creation failed"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	err := client.CreateAutoFixTask("test-task-123", "Test error message", "ERROR_001")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API呼び出しが失敗しました")
	assert.Contains(t, err.Error(), "500")
}

func TestTaskStatusConstants(t *testing.T) {
	assert.Equal(t, TaskStatus("IN_PROGRESS"), TaskStatusProcessing)
	assert.Equal(t, TaskStatus("COMPLETED"), TaskStatusCompleted)
	assert.Equal(t, TaskStatus("FAILED"), TaskStatusFailed)
}

func TestClientWithTimeout(t *testing.T) {
	// タイムアウトが短いクライアントを作成
	config.GlobalConfig = &config.Config{
		API: config.APIConfig{
			URL:     "http://timeout-test.example.com",
			Token:   "timeout-token",
			Timeout: 1 * 1000000, // 1マイクロ秒（非常に短い）
		},
	}

	client := NewClient()
	assert.NotNil(t, client)

	// タイムアウトが発生することを確認（実際のサーバーに接続しないため、即座にエラー）
	err := client.UpdateTaskStatus("timeout-task", TaskStatusCompleted, "Test", 100, "")
	assert.Error(t, err)
}

func TestUpdateTaskStatusWithRetry(t *testing.T) {
	retryCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		retryCount++
		if retryCount < 3 {
			// 最初の2回は失敗
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// 3回目で成功
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "retry-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	// リトライ機能付きでタスクステータス更新を実行
	err := client.UpdateTaskStatusWithRetry("retry-task", TaskStatusCompleted, "Retry test", 100, "", 3)
	assert.NoError(t, err)
	assert.Equal(t, 3, retryCount) // 3回試行されたことを確認
}

func TestUpdateTaskStatusWithRetryAllFailed(t *testing.T) {
	retryCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		retryCount++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "persistent error"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "retry-fail-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	// 全てのリトライが失敗することを確認
	err := client.UpdateTaskStatusWithRetryAllFailed("retry-fail-task", TaskStatusFailed, "All retries failed", 0, "ERROR_RETRY", 3)
	assert.Error(t, err)
	assert.Equal(t, 3, retryCount) // 3回試行されたことを確認
	assert.Contains(t, err.Error(), "最大リトライ回数に達しました")
}

func TestSendLogBatch(t *testing.T) {
	var receivedLogs []LogRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/tasks/batch-task/logs/batch", r.URL.Path)

		var batchRequest struct {
			Logs []LogRequest `json:"logs"`
		}
		err := json.NewDecoder(r.Body).Decode(&batchRequest)
		require.NoError(t, err)

		receivedLogs = batchRequest.Logs
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "batch-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	logs := []LogRequest{
		{Level: "INFO", Message: "Log 1"},
		{Level: "ERROR", Message: "Log 2"},
		{Level: "DEBUG", Message: "Log 3"},
	}

	err := client.SendLogBatch("batch-task", logs)
	assert.NoError(t, err)
	assert.Len(t, receivedLogs, 3)
	assert.Equal(t, "INFO", receivedLogs[0].Level)
	assert.Equal(t, "Log 1", receivedLogs[0].Message)
}

func TestGetTaskInfo(t *testing.T) {
	taskInfo := map[string]interface{}{
		"id":          "info-task-123",
		"status":      "RUNNING",
		"progress":    75,
		"description": "Test task information",
		"created_at":  "2023-07-29T10:00:00Z",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/v1/tasks/info-task-123", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(taskInfo)
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "info-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	result, err := client.GetTaskInfo("info-task-123")
	assert.NoError(t, err)
	assert.Equal(t, "info-task-123", result["id"])
	assert.Equal(t, "RUNNING", result["status"])
	assert.Equal(t, float64(75), result["progress"])
}

func TestGetTaskInfoNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "task not found"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "info-not-found-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	_, err := client.GetTaskInfo("non-existent-task")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestValidateTaskStatus(t *testing.T) {
	// 有効なステータス
	assert.True(t, ValidateTaskStatus(TaskStatusProcessing))
	assert.True(t, ValidateTaskStatus(TaskStatusCompleted))
	assert.True(t, ValidateTaskStatus(TaskStatusFailed))

	// 無効なステータス
	assert.False(t, ValidateTaskStatus(TaskStatus("INVALID_STATUS")))
	assert.False(t, ValidateTaskStatus(TaskStatus("")))
}

func TestClientHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/health", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "version": "1.0.0"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "health-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	health, err := client.HealthCheck()
	assert.NoError(t, err)
	assert.Equal(t, "ok", health["status"])
	assert.Equal(t, "1.0.0", health["version"])
}

func TestClientHealthCheckFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status": "error", "message": "service unavailable"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "health-fail-token",
		httpClient: &http.Client{
			Timeout: 30 * 1000000000,
		},
	}

	_, err := client.HealthCheck()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestTaskUpdateRequest(t *testing.T) {
	req := TaskUpdateRequest{
		Status:    TaskStatusCompleted,
		Message:   "Task completed",
		Progress:  100,
		ErrorCode: "",
	}

	jsonData, err := json.Marshal(req)
	require.NoError(t, err)

	var unmarshaled TaskUpdateRequest
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, req.Status, unmarshaled.Status)
	assert.Equal(t, req.Message, unmarshaled.Message)
	assert.Equal(t, req.Progress, unmarshaled.Progress)
	assert.Equal(t, req.ErrorCode, unmarshaled.ErrorCode)
}

func TestLogRequest(t *testing.T) {
	req := LogRequest{
		Level:   "INFO",
		Message: "Test log message",
	}

	jsonData, err := json.Marshal(req)
	require.NoError(t, err)

	var unmarshaled LogRequest
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, req.Level, unmarshaled.Level)
	assert.Equal(t, req.Message, unmarshaled.Message)
}

// 追加のヘルパーメソッドの実装（実際のAPIクライアントにも実装が必要）

func (c *Client) UpdateTaskStatusWithRetry(taskID string, status TaskStatus, message string, progress int, errorCode string, maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		err := c.UpdateTaskStatus(taskID, status, message, progress, errorCode)
		if err == nil {
			return nil
		}
		if i == maxRetries-1 {
			return fmt.Errorf("最大リトライ回数に達しました: %w", err)
		}
		// 少し待機してからリトライ
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	return nil
}

func (c *Client) UpdateTaskStatusWithRetryAllFailed(taskID string, status TaskStatus, message string, progress int, errorCode string, maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		err := c.UpdateTaskStatus(taskID, status, message, progress, errorCode)
		if err == nil {
			return nil
		}
		// 常に失敗するバージョン
	}
	return fmt.Errorf("最大リトライ回数に達しました")
}

func (c *Client) SendLogBatch(taskID string, logs []LogRequest) error {
	batchRequest := struct {
		Logs []LogRequest `json:"logs"`
	}{
		Logs: logs,
	}

	jsonData, err := json.Marshal(batchRequest)
	if err != nil {
		return fmt.Errorf("リクエストのシリアライズに失敗: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/v1/tasks/"+taskID+"/logs/batch", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API呼び出しが失敗しました: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) GetTaskInfo(taskID string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/tasks/"+taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API呼び出しが失敗しました: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("レスポンスの解析に失敗: %w", err)
	}

	return result, nil
}

func ValidateTaskStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusProcessing, TaskStatusCompleted, TaskStatusFailed:
		return true
	default:
		return false
	}
}

func (c *Client) HealthCheck() (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/health", nil)
	if err != nil {
		return nil, fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ヘルスチェックが失敗しました: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("レスポンスの解析に失敗: %w", err)
	}

	return result, nil
}
