package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
)

// sendLogHTTP はHTTP APIを使用してログを送信します
func sendLogHTTP(client *Client, taskID string, level string, message string) error {
	url := fmt.Sprintf("%s/api/v1/tasks/%s/logs", client.baseURL, taskID)

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
	if client.token != "" {
		req.Header.Set("Authorization", "Bearer "+client.token)
	}

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"level":   level,
		"message": message,
	}).Debug("ログを送信中")

	// リクエストヘッダーを収集
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	resp, err := client.httpClient.Do(req)

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

	return nil
}
