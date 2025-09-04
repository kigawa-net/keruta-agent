package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
)

// getScriptHTTP はHTTP APIを使用してスクリプトを取得します
func getScriptHTTP(client *Client, taskID string) (*Script, error) {
	url := fmt.Sprintf("%s/api/v1/tasks/%s/script", client.baseURL, taskID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("リクエストの作成に失敗しました")
		return nil, fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if client.token != "" {
		req.Header.Set("Authorization", "Bearer "+client.token)
	}

	logger.WithTaskIDAndComponent("api").WithField("taskID", taskID).Debug("スクリプトを取得中")

	// リクエストヘッダーを収集
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	resp, err := client.httpClient.Do(req)

	// API呼び出しエラーの詳細をログに記録
	logAPIError("GET", url, headers, nil, resp, err, false)

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

	var scriptResp ScriptResponse
	if err := json.NewDecoder(resp.Body).Decode(&scriptResp); err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("レスポンスのデコードに失敗しました")
		return nil, fmt.Errorf("レスポンスのデコードに失敗: %w, %s/api/v1/tasks/%s/script", err, client.baseURL, taskID)
	}

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"taskID":   taskID,
		"language": scriptResp.Script.Language,
		"filename": scriptResp.Script.Filename,
	}).Info("スクリプトを取得しました")

	return &scriptResp.Script, nil
}
