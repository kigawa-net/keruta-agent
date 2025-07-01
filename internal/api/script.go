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

// getScriptHTTP はHTTP APIを使用してスクリプトを取得します
func getScriptHTTP(client *Client, taskID string) (*Script, error) {
	url := fmt.Sprintf("%s/api/v1/tasks/%s/script", client.baseURL, taskID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("リクエストの作成に失敗しました")
		return nil, fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+client.token)

	logger.WithTaskIDAndComponent("api").WithField("taskID", taskID).Debug("スクリプトを取得中")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("API呼び出しに失敗しました")
		return nil, fmt.Errorf("API呼び出しに失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
			"statusCode": resp.StatusCode,
			"response":   string(body),
		}).Error("API呼び出しが失敗しました")
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

// createAutoFixTaskHTTP はHTTP APIを使用して自動修正タスクを作成します
func createAutoFixTaskHTTP(client *Client, taskID string, errorMessage string, errorCode string) error {
	url := fmt.Sprintf("%s/api/v1/tasks/%s/auto-fix", client.baseURL, taskID)

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
	req.Header.Set("Authorization", "Bearer "+client.token)

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"errorMessage": errorMessage,
		"errorCode":    errorCode,
	}).Info("自動修正タスクを作成中")

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

	logger.WithTaskIDAndComponent("api").Info("自動修正タスクを作成しました")
	return nil
}
