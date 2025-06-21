package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
		assert.Equal(t, "/api/tasks/test-task-123/status", r.URL.Path)
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
		assert.Equal(t, "/api/tasks/test-task-123/logs", r.URL.Path)
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
		assert.Equal(t, "/api/tasks/test-task-123/artifacts", r.URL.Path)
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
		assert.Equal(t, "/api/tasks/test-task-123/auto-fix", r.URL.Path)
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
	assert.Equal(t, TaskStatus("PROCESSING"), TaskStatusProcessing)
	assert.Equal(t, TaskStatus("COMPLETED"), TaskStatusCompleted)
	assert.Equal(t, TaskStatus("FAILED"), TaskStatusFailed)
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