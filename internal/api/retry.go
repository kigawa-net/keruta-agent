package api

import (
	"fmt"
	"strings"
	"time"

	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
)

// RetryableFunc は再試行可能な関数の型です
type RetryableFunc func() error

// RetryWithBackoff は指定された関数を指数バックオフで再試行します
func RetryWithBackoff(operation string, fn RetryableFunc) error {
	retryCount := config.GlobalConfig.ErrorHandling.RetryCount
	var err error

	for i := 0; i < retryCount; i++ {
		err = fn()
		if err == nil {
			return nil
		}

		// 最後の試行ではエラーを返す
		if i == retryCount-1 {
			return err
		}

		// 接続エラーの場合のみ再試行
		if isConnectionError(err) {
			// 指数バックオフ: 1秒、2秒、4秒...
			waitTime := time.Duration(1<<uint(i)) * time.Second
			logger.WithTaskIDAndComponent("api").WithFields(logrus.Fields{
				"operation": operation,
				"attempt":   i + 1,
				"maxRetry":  retryCount,
				"waitTime":  waitTime,
				"error":     err.Error(),
			}).Warning("API呼び出しに失敗しました。再試行します")
			time.Sleep(waitTime)
		} else {
			// 接続エラー以外はすぐに失敗
			return err
		}
	}

	return err
}

// isConnectionError は与えられたエラーが接続エラーかどうかを判定します
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	// 接続エラーに関連する文字列をチェック
	connectionErrors := []string{
		"connection refused",
		"connect: connection refused",
		"i/o timeout",
		"no such host",
		"network is unreachable",
		"connection reset by peer",
		"dial tcp",
	}

	for _, msg := range connectionErrors {
		if contains(errMsg, msg) {
			return true
		}
	}
	return false
}

// contains は文字列に部分文字列が含まれているかどうかを判定します
func contains(s, substr string) bool {
	return fmt.Sprintf("%s", s) != "" && strings.Contains(s, substr)
}
