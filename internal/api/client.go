package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
)

// logAPIError はAPI呼び出しのエラーを詳細にログに記録します
func logAPIError(method, url string, reqHeaders map[string]string, reqBody interface{}, resp *http.Response, err error, isWarning bool) {
	logEntry := logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"method": method,
		"url":    url,
	})

	// リクエストヘッダーをログに記録（認証情報は除外）
	headers := make(map[string]string)
	for k, v := range reqHeaders {
		if !strings.Contains(strings.ToLower(k), "auth") {
			headers[k] = v
		} else {
			headers[k] = "[REDACTED]"
		}
	}
	logEntry = logEntry.WithField("requestHeaders", headers)

	// リクエストボディをログに記録
	if reqBody != nil {
		logEntry = logEntry.WithField("requestBody", reqBody)
	}

	if err != nil {
		// HTTP呼び出し自体のエラー
		logEntry = logEntry.WithError(err)
		if isWarning {
			logEntry.Warning("API呼び出しに失敗しましたが、処理を継続します")
		} else {
			logEntry.Error("API呼び出しに失敗しました")
		}
		return
	}

	if resp != nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		// ステータスコードエラー
		var respBody string
		if resp.Body != nil {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			if readErr == nil {
				respBody = string(bodyBytes)
				// レスポンスボディを再設定
				resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			} else {
				respBody = fmt.Sprintf("レスポンスボディの読み取りに失敗: %v", readErr)
			}
		}

		logEntry = logEntry.WithFields(logrus.Fields{
			"statusCode":   resp.StatusCode,
			"responseBody": respBody,
		})

		if isWarning {
			logEntry.Warning("API呼び出しが失敗しましたが、処理を継続します")
		} else {
			logEntry.Error("API呼び出しが失敗しました")
		}
	}
}

// Client はkeruta APIクライアントです
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// TaskStatus はタスクのステータスを表します
type TaskStatus string

const (
	TaskStatusProcessing      TaskStatus = "IN_PROGRESS"
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

// Task はタスク情報を表します
type Task struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Status      TaskStatus             `json:"status"`
	WorkspaceID string                 `json:"workspaceId"`
	Script      string                 `json:"script"`
	Parameters  map[string]interface{} `json:"parameters"`
	CreatedAt   string                 `json:"createdAt"`
	UpdatedAt   string                 `json:"updatedAt"`
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
	// 環境変数でHTTP入力モードを制御
	useHTTP := os.Getenv("KERUTA_USE_HTTP_INPUT") == "true"

	if useHTTP {
		// HTTP APIを使用して入力を待機
		logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
			"taskID": taskID,
			"prompt": prompt,
		}).Info("HTTP APIを通じて入力を待機中...")

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
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			logger.WithTaskIDAndComponent("api").WithError(err).Warning("入力リクエストの送信に失敗しました")
		} else {
			if closeErr := resp.Body.Close(); closeErr != nil {
				logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("レスポンスボディのクローズに失敗しました")
			}
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
			if c.token != "" {
				pollReq.Header.Set("Authorization", "Bearer "+c.token)
			}
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
					if closeErr := pollResp.Body.Close(); closeErr != nil {
						logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("ポーリングレスポンスボディのクローズに失敗しました")
					}
					return "", fmt.Errorf("入力レスポンスのデコードに失敗: %w", err)
				}

				if closeErr := pollResp.Body.Close(); closeErr != nil {
					logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("ポーリングレスポンスボディのクローズに失敗しました")
				}
				logger.WithTaskIDAndComponent("api").Info("入力を受け取りました")
				return inputResp.Input, nil
			}

			if closeErr := pollResp.Body.Close(); closeErr != nil {
				logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("ポーリングレスポンスボディのクローズに失敗しました")
			}
			// 入力がまだ提供されていない場合は待機
			time.Sleep(5 * time.Second)
		}

		return "", fmt.Errorf("入力待ちがタイムアウトしました")
	} else {
		// 標準入力から入力を受け付ける
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
}

// GetScript はタスクのスクリプトを取得します
func (c *Client) GetScript(taskID string) (*Script, error) {
	return getScriptHTTP(c, taskID)
}

// Session はセッション情報を表します
type Session struct {
	ID             string                  `json:"id"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description"`
	Status         string                  `json:"status"`
	WorkspaceID    string                  `json:"workspaceId"`
	Tags           []string                `json:"tags"`
	Metadata       map[string]string       `json:"metadata"`
	TemplateConfig *SessionTemplateConfig  `json:"templateConfig"`
	CreatedAt      interface{}             `json:"createdAt"`
	UpdatedAt      interface{}             `json:"updatedAt"`
}

type SessionTemplateConfig struct {
	TemplateID         string            `json:"templateId"`
	TemplateName       string            `json:"templateName"`
	RepositoryURL      string            `json:"repositoryUrl"`
	RepositoryRef      string            `json:"repositoryRef"`
	TemplatePath       string            `json:"templatePath"`
	PreferredKeywords  []string          `json:"preferredKeywords"`
	Parameters         map[string]string `json:"parameters"`
}

// GetSession はセッション情報を取得します
func (c *Client) GetSession(sessionID string) (*Session, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/%s", c.baseURL, sessionID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("レスポンスボディのクローズに失敗しました")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API呼び出しが失敗しました: %d - %s (URL: %s)", resp.StatusCode, string(body), url)
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("レスポンスのデコードに失敗: %w", err)
	}

	return &session, nil
}

// GetPendingTasksForSession はセッション用の保留中タスクを取得します
func (c *Client) GetPendingTasksForSession(sessionID string) ([]*Task, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/%s/tasks?status=PENDING", c.baseURL, sessionID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("レスポンスボディのクローズに失敗しました")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	var tasks []*Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("レスポンスのデコードに失敗: %w", err)
	}

	return tasks, nil
}

// GetPendingTasksForWorkspace はワークスペース用の保留中タスクを取得します
func (c *Client) GetPendingTasksForWorkspace(workspaceID string) ([]*Task, error) {
	url := fmt.Sprintf("%s/api/v1/workspaces/%s/tasks/pending", c.baseURL, workspaceID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("レスポンスボディのクローズに失敗しました")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	var tasks []*Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("レスポンスのデコードに失敗: %w", err)
	}

	return tasks, nil
}

// StartTask はタスクを開始します
func (c *Client) StartTask(taskID string) error {
	return c.UpdateTaskStatus(taskID, TaskStatusProcessing, "タスクを開始しました", 0, "")
}

// SuccessTask はタスクを成功として完了します
func (c *Client) SuccessTask(taskID string, message string) error {
	return c.UpdateTaskStatus(taskID, TaskStatusCompleted, message, 100, "")
}

// FailTask はタスクを失敗として完了します
func (c *Client) FailTask(taskID string, message string, errorCode string) error {
	return c.UpdateTaskStatus(taskID, TaskStatusFailed, message, 0, errorCode)
}

// GetTaskScript はタスクのスクリプトを取得します
func (c *Client) GetTaskScript(taskID string) (string, error) {
	script, err := c.GetScript(taskID)
	if err != nil {
		return "", err
	}
	return script.Content, nil
}

// SearchSessionByPartialID は部分的なセッションIDで検索し、完全なUUIDを取得します
func (c *Client) SearchSessionByPartialID(partialID string) (*Session, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/search/partial-id?partialId=%s", c.baseURL, partialID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("レスポンスボディのクローズに失敗しました")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API呼び出しが失敗しました: %d - %s (URL: %s)", resp.StatusCode, string(body), url)
	}

	var sessions []Session
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("レスポンスのデコードに失敗: %w", err)
	}

	// 結果が空の場合
	if len(sessions) == 0 {
		return nil, fmt.Errorf("部分的なID '%s' に一致するセッションが見つかりませんでした", partialID)
	}

	// 複数の結果がある場合は最初のものを返す（通常は1つだけのはず）
	if len(sessions) > 1 {
		logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
			"partialId":    partialID,
			"sessionCount": len(sessions),
		}).Warning("部分的なIDに複数のセッションが一致しました。最初のものを使用します")
	}

	return &sessions[0], nil
}

// SearchSessionByName は名前による完全一致でセッションを検索します
func (c *Client) SearchSessionByName(name string) (*Session, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/search?name=%s", c.baseURL, name)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("レスポンスボディのクローズに失敗しました")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API呼び出しが失敗しました: %d - %s (URL: %s)", resp.StatusCode, string(body), url)
	}

	var sessions []Session
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("レスポンスのデコードに失敗: %w", err)
	}

	// 完全一致のセッションを探す
	for _, session := range sessions {
		if session.Name == name {
			return &session, nil
		}
	}

	return nil, fmt.Errorf("名前 '%s' に一致するセッションが見つかりませんでした", name)
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
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"errorMessage": errorMessage,
		"errorCode":    errorCode,
	}).Info("自動修正タスクを作成中")

	// リクエストヘッダーを収集
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	resp, err := c.httpClient.Do(req)

	// API呼び出しエラーの詳細をログに記録（警告レベル）
	logAPIError("POST", url, headers, reqBody, resp, err, true)

	if err != nil {
		return fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("レスポンスボディのクローズに失敗しました")
		}
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	logger.WithTaskIDAndComponent("api").Info("自動修正タスクを作成しました")
	return nil
}
