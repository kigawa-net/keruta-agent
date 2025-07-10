package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
)

// HTTPClient はHTTP APIクライアントを表します
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPClient は新しいHTTP APIクライアントを作成します
func NewHTTPClient(baseURL string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// UpdateTaskStatus はタスクのステータスを更新します
func (c *HTTPClient) UpdateTaskStatus(taskID string, status TaskStatus, message string, progress int, errorCode string) error {
	url := fmt.Sprintf("%s/api/v1/tasks/%s/status", c.baseURL, taskID)

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

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"url":     url,
		"status":  status,
		"message": message,
	}).Debug("タスクステータスを更新中")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("API呼び出しに失敗しました")
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
func (c *HTTPClient) SendLog(taskID string, level string, message string) error {
	url := fmt.Sprintf("%s/api/v1/tasks/%s/logs", c.baseURL, taskID)

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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Warning("ログの送信に失敗しました")
		return nil // ログ送信の失敗は無視
	}
	defer resp.Body.Close()

	return nil
}

// GetScript はタスクのスクリプトを取得します
func (c *HTTPClient) GetScript(taskID string) (*Script, error) {
	url := fmt.Sprintf("%s/api/v1/tasks/%s/script", c.baseURL, taskID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("リクエストの作成に失敗しました")
		return nil, fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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
		logger.WithTaskIDAndComponent("api").WithError(err).Error("レスポンスのデコードに失敗しました")
		return nil, fmt.Errorf("レスポンスのデコードに失敗: %w", err)
	}

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"taskID":   taskID,
		"language": scriptResp.Script.Language,
		"filename": scriptResp.Script.Filename,
	}).Info("スクリプトを取得しました")

	return &scriptResp.Script, nil
}

// WaitForInput は入力待ち状態を通知し、入力を待機します
// HTTP APIを使用して入力を待機します
func (c *HTTPClient) WaitForInput(taskID string, prompt string) (string, error) {
	// 入力待ち状態をAPIに通知
	url := fmt.Sprintf("%s/api/v1/tasks/%s/input-request", c.baseURL, taskID)
	reqBody := map[string]string{
		"prompt": prompt,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("リクエストボディのマーシャルに失敗: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"taskID": taskID,
		"prompt": prompt,
	}).Info("HTTP APIを通じて入力を待機中...")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Warning("入力リクエストの送信に失敗しました")
	} else {
		resp.Body.Close()
	}

	// 入力が提供されるまでポーリング
	pollURL := fmt.Sprintf("%s/api/v1/tasks/%s/input", c.baseURL, taskID)
	maxRetries := 12 * 60 * 24 * 7
	for i := 0; i < maxRetries; i++ {
		pollReq, err := http.NewRequest("GET", pollURL, nil)
		if err != nil {
			return "", fmt.Errorf("入力ポーリングリクエストの作成に失敗: %w", err)
		}

		pollReq.Header.Set("Content-Type", "application/json")
		pollResp, err := c.httpClient.Do(pollReq)
		if err != nil {
			logger.WithTaskIDAndComponent("api").WithError(err).Warning("入力のポーリングに失敗しました、再試行します")
			time.Sleep(5 * time.Second)
			continue
		}

		// 入力が提供された場合
		if pollResp.StatusCode == http.StatusOK {
			var inputResp struct {
				Input string `json:"input"`
			}

			if err := json.NewDecoder(pollResp.Body).Decode(&inputResp); err != nil {
				pollResp.Body.Close()
				return "", fmt.Errorf("入力レスポンスのデコードに失敗: %w", err)
			}

			pollResp.Body.Close()
			logger.WithTaskIDAndComponent("api").Info("入力を受け取りました")
			return inputResp.Input, nil
		}

		pollResp.Body.Close()
		// 入力がまだ提供されていない場合は待機
		time.Sleep(5 * time.Second)
	}

	return "", fmt.Errorf("入力待ちがタイムアウトしました")
}

// UploadArtifact は成果物をアップロードします
func (c *HTTPClient) UploadArtifact(taskID string, filePath string, description string) error {
	// 既存のuploadArtifactHTTP関数と同様の実装
	// 必要に応じて実装
	return fmt.Errorf("未実装")
}
