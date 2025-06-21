package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

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
	TaskStatusProcessing TaskStatus = "PROCESSING"
	TaskStatusCompleted  TaskStatus = "COMPLETED"
	TaskStatusFailed     TaskStatus = "FAILED"
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

// UpdateTaskStatus はタスクのステータスを更新します
func (c *Client) UpdateTaskStatus(taskID string, status TaskStatus, message string, progress int, errorCode string) error {
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

	logger.WithTaskIDAndComponent("api").WithField("status", status).Info("タスクステータスを更新しました")
	return nil
}

// SendLog はログを送信します
func (c *Client) SendLog(taskID string, level string, message string) error {
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