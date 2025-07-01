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
	defer file.Close()

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

	writer.Close()

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("リクエストの作成に失敗しました")
		return fmt.Errorf("リクエストの作成に失敗: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+client.token)

	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"file":        filePath,
		"description": description,
	}).Debug("成果物をアップロード中")

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

	logger.WithTaskIDAndComponent("api").WithField("file", filePath).Info("成果物をアップロードしました")
	return nil
}
