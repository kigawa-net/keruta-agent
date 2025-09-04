package main

import (
	"fmt"
	"os"

	"keruta-agent/internal/commands"
	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
)

func main() {
	// 設定の初期化
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "設定の初期化に失敗しました: %v\n", err)
		os.Exit(1)
	}

	// ロガーの初期化
	if err := logger.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "ロガーの初期化に失敗しました: %v\n", err)
		os.Exit(1)
	}

	// ルートコマンドの実行
	if err := commands.Execute(); err != nil {
		logrus.WithError(err).Error("コマンドの実行に失敗しました")
		os.Exit(1)
	}
} 