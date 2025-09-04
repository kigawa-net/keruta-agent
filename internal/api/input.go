package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
)

// waitForInputStdin は標準入力から入力を待機します
func waitForInputStdin(prompt string) (string, error) {
	logger.WithTaskIDAndComponent("api").WithField("prompt", prompt).Info("標準入力からの入力を待機中...")

	// プロンプトを表示
	fmt.Printf("%s ", prompt)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("標準入力の読み取りに失敗: %w", err)
	}
	return input, nil
}

// waitForInputHTTP はHTTP APIを使用して入力を待機します
func waitForInputHTTP(client *Client, taskID string, prompt string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/tasks/%s/input", client.baseURL, taskID)

	// 入力待ち状態をログに記録
	logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
		"taskID": taskID,
		"prompt": prompt,
	}).Info("HTTP APIを通じて入力を待機中...")

	// 入力が提供されるまでポーリング
	maxRetries := 12 * 60 * 24 * 7
	for i := 0; i < maxRetries; i++ {
		// 入力をチェック
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return "", fmt.Errorf("入力リクエストの作成に失敗: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := client.httpClient.Do(req)
		if err != nil {
			logger.WithTaskIDAndComponent("api").WithError(err).Warning("入力のチェックに失敗しました、再試行します")
			time.Sleep(5 * time.Second)
			continue
		}

		defer resp.Body.Close()

		// 入力が提供された場合
		if resp.StatusCode == http.StatusOK {
			var inputResp struct {
				Input string `json:"input"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&inputResp); err != nil {
				return "", fmt.Errorf("入力レスポンスのデコードに失敗: %w", err)
			}

			logger.WithTaskIDAndComponent("api").Info("入力を受け取りました")
			return inputResp.Input, nil
		}

		// 入力がまだ提供されていない場合は待機
		time.Sleep(5 * time.Second)
	}

	return "", fmt.Errorf("入力待ちがタイムアウトしました")
}
