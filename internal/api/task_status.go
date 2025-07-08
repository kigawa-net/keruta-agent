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

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"url":     url,
		"status":  status,
		"message": message,
	}).Debug("タスクステータスを更新中")

	// リクエストヘッダーを収集
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	resp, err := client.httpClient.Do(req)

	// API呼び出しエラーの詳細をログに記録
	logAPIError("PUT", url, headers, reqBody, resp, err, false)

	if err != nil {
		return fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("レスポンスボディのクローズに失敗しました")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API呼び出しが失敗しました: %d - %s", resp.StatusCode, string(body))
	}

	logger.WithTaskIDAndComponent("api").WithField("status", status).Info("タスクステータスを更新しました")
	return nil
}
