package api

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
)

// uploadArtifactHTTP はHTTP APIを使用して成果物をアップロードします
func uploadArtifactHTTP(client *Client, taskID string, filePath string, description string) error {
	url := fmt.Sprintf("%s/api/v1/tasks/%s/artifacts", client.baseURL, taskID)

	file, err := os.Open(filePath)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("ファイルのオープンに失敗しました")
		return fmt.Errorf("ファイルのオープンに失敗: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.WithTaskIDAndComponent("api").WithError(closeErr).Warning("ファイルのクローズに失敗しました")
		}
	}()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// ファイルフィールド
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("フォームファイルの作成に失敗しました")
		return fmt.Errorf("フォームファイルの作成に失敗: %w", err)
	}

	_, err = io.Copy(part, file)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("ファイルのコピーに失敗しました")
		return fmt.Errorf("ファイルのコピーに失敗: %w", err)
	}

	// 説明フィールド
	if description != "" {
		err = writer.WriteField("description", description)
		if err != nil {
			logger.WithTaskIDAndComponent("api").WithError(err).Error("説明フィールドの書き込みに失敗しました")
			return fmt.Errorf("説明フィールドの書き込みに失敗: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("マルチパートライターのクローズに失敗しました")
		return fmt.Errorf("マルチパートライターのクローズに失敗: %w", err)
	}

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("リクエストの作成に失敗しました")
		return fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"file":        filePath,
		"description": description,
	}).Debug("成果物をアップロード中")

	// リクエストヘッダーを収集
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	resp, err := client.httpClient.Do(req)

	// API呼び出しエラーの詳細をログに記録（警告レベル）
	logAPIError("POST", url, headers, map[string]string{"file": filePath, "description": description}, resp, err, true)

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

	logger.WithTaskIDAndComponent("api").WithField("file", filePath).Info("成果物をアップロードしました")
	return nil
}
