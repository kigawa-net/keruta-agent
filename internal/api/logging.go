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
	req.Header.Set("Authorization", "Bearer "+client.token)

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"level":   level,
		"message": message,
	}).Debug("ログを送信中")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Warning("API呼び出しに失敗しましたが、処理を継続します")
		return fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
			"statusCode": resp.StatusCode,
			"response":   string(body),
		}).Warning("API呼び出しが失敗しましたが、処理を継続します")
		return fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

// sendLogWebSocket はWebSocketを使用してログを送信します
func sendLogWebSocket(client *Client, level string, message string) {
	if client.wsInitialized && client.wsClient != nil {
		client.wsClient.SendLog(level, message)
		logger.WithTaskIDAndComponent("api").Debug("WebSocketでログを送信しました")
	}
}
