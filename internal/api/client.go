package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
)

// Client はkeruta APIクライアントです
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
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
	}
}

// GetWebSocketClient はWebSocketクライアントを取得します
// WebSocket機能は削除されました
func (c *Client) GetWebSocketClient(taskID string) (interface{}, error) {
	logger.WithTaskIDAndComponent("api").WithField("taskID", taskID).Info("WebSocket機能は削除されました")
	return nil, fmt.Errorf("WebSocket機能は削除されました (taskID: %s)", taskID)
}

// UpdateTaskStatus はタスクのステータスを更新します
func (c *Client) UpdateTaskStatus(taskID string, status TaskStatus, message string, progress int, errorCode string) error {
	// HTTP APIでステータス更新
	err := updateTaskStatusHTTP(c, taskID, status, message, progress, errorCode)
	if err != nil {
		return err
	}
	return nil
}

// SendLog はログを送信します
func (c *Client) SendLog(taskID string, level string, message string) error {
	// HTTP APIでログ送信
	err := sendLogHTTP(c, taskID, level, message)
	if err != nil {
		return err
	}
	return nil
}

// UploadArtifact は成果物をアップロードします
func (c *Client) UploadArtifact(taskID string, filePath string, description string) error {
	return uploadArtifactHTTP(c, taskID, filePath, description)
}

// WaitForInput は入力待ち状態を通知し、入力を待機します
func (c *Client) WaitForInput(taskID string, prompt string) (string, error) {
	// WebSocket機能は削除されたため、標準入力から入力を受け付ける
	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"taskID": taskID,
		"prompt": prompt,
	}).Info("標準入力からの入力を待機中...")

	// プロンプトを表示
	fmt.Printf("%s ", prompt)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("標準入力の読み取りに失敗 (taskID: %s, prompt: %s): %w", taskID, prompt, err)
	}
	return input, nil
}

// GetScript はタスクのスクリプトを取得します
func (c *Client) GetScript(taskID string) (*Script, error) {
	return getScriptHTTP(c, taskID)
}

// CreateAutoFixTask は自動修正タスクを作成します
func (c *Client) CreateAutoFixTask(taskID string, errorMessage string, errorCode string) error {
	url := fmt.Sprintf("%s/api/v1/tasks/%s/auto-fix", c.baseURL, taskID)

	reqBody := map[string]string{
		"errorMessage": errorMessage,
		"errorCode":    errorCode,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("リクエストボディのマーシャルに失敗しました")
		return fmt.Errorf("リクエストボディのマーシャルに失敗: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("リクエストの作成に失敗しました")
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
		logger.WithTaskIDAndComponent("api").WithError(err).Warning("API呼び出しに失敗しましたが、処理を継続します")
		return fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("レスポンスボディのクローズに失敗しました")
		}
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
			"statusCode": resp.StatusCode,
			"response":   string(body),
		}).Warning("API呼び出しが失敗しましたが、処理を継続します")
		return fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	logger.WithTaskIDAndComponent("api").Info("自動修正タスクを作成しました")
	return nil
}
