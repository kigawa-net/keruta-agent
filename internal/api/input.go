package api

import (
	"bufio"
	"fmt"
	"os"

	"keruta-agent/internal/logger"
)

// waitForInputWebSocket はWebSocketを使用して入力を待機します
func waitForInputWebSocket(client *Client, taskID string, prompt string) (string, error) {
	// WebSocketクライアントの取得
	wsClient, err := client.GetWebSocketClient(taskID)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("WebSocketクライアントの取得に失敗しました")
		return "", err
	}

	// WebSocketで入力待ち状態を通知し、入力を待機
	input, err := wsClient.WaitForInput(prompt)
	if err != nil {
		logger.WithTaskIDAndComponent("api").WithError(err).Error("WebSocketでの入力待機に失敗しました")
		return "", err
	}

	return input, nil
}

// waitForInputStdin は標準入力から入力を待機します
func waitForInputStdin() (string, error) {
	logger.WithTaskIDAndComponent("api").Info("標準入力からの入力を待機中...")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("標準入力の読み取りに失敗: %w", err)
	}
	return input, nil
}
