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

// updateTaskStatusHTTP はHTTP APIを使用してタスクのステータスを更新します
func updateTaskStatusHTTP(client *Client, taskID string, status TaskStatus, message string, progress int, errorCode string) error {
	url := fmt.Sprintf("%s/api/v1/tasks/%s/status", client.baseURL, taskID)

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
	req.Header.Set("Authorization", "Bearer "+client.token)

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"url":     url,
		"status":  status,
		"message": message,
	}).Debug("タスクステータスを更新中")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("API呼び出しに失敗しました")
		return fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
			"statusCode": resp.StatusCode,
			"response":   string(body),
		}).Error("API呼び出しが失敗しました")
		return fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	logger.WithTaskIDAndComponent("api").WithField("status", status).Info("タスクステータスを更新しました")
	return nil
}

// updateTaskStatusWebSocket はWebSocketを使用してタスクのステータスを更新します
func updateTaskStatusWebSocket(client *Client, status TaskStatus, message string) {
	if client.wsInitialized && client.wsClient != nil {
		client.wsClient.UpdateTaskStatus(status, message)
		logger.WithTaskIDAndComponent("api").Info("WebSocketでタスクステータスを更新しました")
	}
}
