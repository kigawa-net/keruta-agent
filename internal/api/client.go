package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
)

// Client はkeruta APIクライアントです
type Client struct {
	baseURL       string
	token         string
	httpClient    *http.Client
	wsClient      *WebSocketClient
	wsInitialized bool
}

// TaskStatus はタスクのステータスを表します
type TaskStatus string

const (
	TaskStatusProcessing      TaskStatus = "PROCESSING"
	TaskStatusCompleted       TaskStatus = "COMPLETED"
	TaskStatusFailed          TaskStatus = "FAILED"
	TaskStatusWaitingForInput TaskStatus = "WAITING_FOR_INPUT"
)

// TaskUpdateRequest はタスク更新リクエストを表します
type TaskUpdateRequest struct {
	Status    TaskStatus `json:"status"`
	Message   string     `json:"message,omitempty"`
	Progress  int        `json:"progress,omitempty"`
	ErrorCode string     `json:"errorCode,omitempty"`
}

// LogRequest はログ送信リクエストを表します
type LogRequest struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Script はスクリプト情報を表します
type Script struct {
	Content    string                 `json:"content"`
	Language   string                 `json:"language"`
	Filename   string                 `json:"filename"`
	Parameters map[string]interface{} `json:"parameters"`
}

// ScriptResponse はスクリプト取得レスポンスを表します
type ScriptResponse struct {
	Success bool   `json:"success"`
	TaskID  string `json:"taskId"`
	Script  Script `json:"script"`
}

// NewClient は新しいAPIクライアントを作成します
func NewClient() *Client {
	return &Client{
		baseURL: config.GetAPIURL(),
		token:   config.GetAPIToken(),
		httpClient: &http.Client{
			Timeout: config.GetTimeout(),
		},
		wsInitialized: false,
	}
}

// CloseWebSocketClient はWebSocketクライアントを閉じます
func (c *Client) CloseWebSocketClient() {
	if c.wsInitialized && c.wsClient != nil {
		c.wsClient.Close()
		c.wsInitialized = false
		c.wsClient = nil
		logger.WithTaskIDAndComponent("api").Info("WebSocketクライアントを閉じました")
	}
}

// GetWebSocketClient はWebSocketクライアントを取得します
// taskIDが指定されていない場合は、config.GetTaskID()から取得します
func (c *Client) GetWebSocketClient(taskID string) (*WebSocketClient, error) {
	if taskID == "" {
		taskID = config.GetTaskID()
		if taskID == "" {
			return nil, fmt.Errorf("タスクIDが設定されていません")
		}
	}

	if !c.wsInitialized || c.wsClient == nil || c.wsClient.taskID != taskID {
		c.wsClient = NewWebSocketClient(c.baseURL, c.token, taskID)
		c.wsInitialized = true
	}

	if err := c.wsClient.Connect(); err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("WebSocket接続に失敗しました")
		return nil, err
	}

	return c.wsClient, nil
}

// UpdateTaskStatus はタスクのステータスを更新します
func (c *Client) UpdateTaskStatus(taskID string, status TaskStatus, message string, progress int, errorCode string) error {
	// HTTP APIでステータス更新
	url := fmt.Sprintf("%s/api/tasks/%s/status", c.baseURL, taskID)

	reqBody := TaskUpdateRequest{
		Status:    status,
		Message:   message,
		Progress:  progress,
		ErrorCode: errorCode,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("リクエストボディのマーシャルに失敗: %w", err)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"url":     url,
		"status":  status,
		"message": message,
	}).Debug("タスクステータスを更新中")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	// WebSocketでもステータス更新（WebSocketクライアントが初期化されている場合）
	if c.wsInitialized && c.wsClient != nil {
		c.wsClient.UpdateTaskStatus(status, message)
	}

	logger.WithTaskIDAndComponent("api").WithField("status", status).Info("タスクステータスを更新しました")
	return nil
}

// SendLog はログを送信します
func (c *Client) SendLog(taskID string, level string, message string) error {
	// HTTP APIでログ送信
	url := fmt.Sprintf("%s/api/tasks/%s/logs", c.baseURL, taskID)

	reqBody := LogRequest{
		Level:   level,
		Message: message,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("リクエストボディのマーシャルに失敗: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"level":   level,
		"message": message,
	}).Debug("ログを送信中")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	// WebSocketでもログ送信（WebSocketクライアントが初期化されている場合）
	if c.wsInitialized && c.wsClient != nil {
		c.wsClient.SendLog(level, message)
	}

	return nil
}

// UploadArtifact は成果物をアップロードします
func (c *Client) UploadArtifact(taskID string, filePath string, description string) error {
	url := fmt.Sprintf("%s/api/tasks/%s/artifacts", c.baseURL, taskID)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("ファイルのオープンに失敗: %w", err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// ファイルフィールド
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("フォームファイルの作成に失敗: %w", err)
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return fmt.Errorf("ファイルのコピーに失敗: %w", err)
	}

	// 説明フィールド
	if description != "" {
		err = writer.WriteField("description", description)
		if err != nil {
			return fmt.Errorf("説明フィールドの書き込みに失敗: %w", err)
		}
	}

	writer.Close()

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"file":        filePath,
		"description": description,
	}).Debug("成果物をアップロード中")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	logger.WithTaskIDAndComponent("api").WithField("file", filePath).Info("成果物をアップロードしました")
	return nil
}

// WaitForInput は入力待ち状態を通知し、入力を待機します
func (c *Client) WaitForInput(taskID string, prompt string) (string, error) {
	// WebSocketクライアントの取得
	wsClient, err := c.GetWebSocketClient(taskID)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("WebSocketクライアントの取得に失敗しました")
		// WebSocketが使えない場合は標準入力から入力を受け付ける
		logger.WithTaskIDAndComponent("api").Info("標準入力からの入力を待機中...")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("標準入力の読み取りに失敗: %w", err)
		}
		return input, nil
	}

	// WebSocketで入力待ち状態を通知し、入力を待機
	input, err := wsClient.WaitForInput(prompt)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("WebSocketでの入力待機に失敗しました")
		// エラーが発生した場合は標準入力から入力を受け付ける
		logger.WithTaskIDAndComponent("api").Info("標準入力からの入力を待機中...")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("標準入力の読み取りに失敗: %w", err)
		}
		return input, nil
	}

	return input, nil
}

// GetScript はタスクのスクリプトを取得します
func (c *Client) GetScript(taskID string) (*Script, error) {
	url := fmt.Sprintf("%s/api/tasks/%s/script", c.baseURL, taskID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	logger.WithTaskIDAndComponent("api").WithField("taskID", taskID).Debug("スクリプトを取得中")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	var scriptResp ScriptResponse
	if err := json.NewDecoder(resp.Body).Decode(&scriptResp); err != nil {
		return nil, fmt.Errorf("レスポンスのデコードに失敗: %w, %s/api/tasks/%s/script", err, c.baseURL, taskID)
	}

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"taskID":   taskID,
		"language": scriptResp.Script.Language,
		"filename": scriptResp.Script.Filename,
	}).Info("スクリプトを取得しました")

	return &scriptResp.Script, nil
}

// CreateAutoFixTask は自動修正タスクを作成します
func (c *Client) CreateAutoFixTask(taskID string, errorMessage string, errorCode string) error {
	url := fmt.Sprintf("%s/api/tasks/%s/auto-fix", c.baseURL, taskID)

	reqBody := map[string]string{
		"errorMessage": errorMessage,
		"errorCode":    errorCode,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("リクエストボディのマーシャルに失敗: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"errorMessage": errorMessage,
		"errorCode":    errorCode,
	}).Info("自動修正タスクを作成中")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	logger.WithTaskIDAndComponent("api").Info("自動修正タスクを作成しました")
	return nil
}
