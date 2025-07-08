package api

import (
	"bufio"
	"fmt"
	"os"

	"keruta-agent/internal/logger"
)


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
