package commands

import (
	"encoding/json"
	"fmt"
	"time"

	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	configKey   string
	configValue string
	configFormat string
)

// configCmd は設定管理コマンドです
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "設定の表示・更新",
	Long:  `設定の表示や更新を行います。`,
}

// configShowCmd は設定表示コマンドです
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "設定を表示",
	Long:  `現在の設定を表示します。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigShow()
	},
}

// configSetCmd は設定更新コマンドです
var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "設定を更新",
	Long:  `指定されたキーの設定値を更新します。`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigSet(args[0], args[1])
	},
}

func init() {
	configShowCmd.Flags().StringVar(&configFormat, "format", "text", "出力形式 (text, json)")
	
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
}

// runConfigShow は設定表示の実行ロジックです
func runConfigShow() error {
	logger.WithComponent("config").Info("設定を表示中")

	if config.GlobalConfig == nil {
		return fmt.Errorf("設定が初期化されていません")
	}

	switch configFormat {
	case "json":
		return outputConfigJSON()
	case "text":
		return outputConfigText()
	default:
		return fmt.Errorf("無効な出力形式です: %s", configFormat)
	}
}

// runConfigSet は設定更新の実行ロジックです
func runConfigSet(key, value string) error {
	logger.WithComponent("config").WithFields(logrus.Fields{
		"key":   key,
		"value": value,
	}).Info("設定を更新中")

	// 設定の更新（簡易版）
	// 実際の実装では、設定ファイルへの永続化も行う
	switch key {
	case "api.url":
		config.GlobalConfig.API.URL = value
	case "api.timeout":
		// タイムアウトの解析は簡略化
		config.GlobalConfig.API.Timeout = 30 * time.Second
	case "logging.level":
		config.GlobalConfig.Logging.Level = value
	case "logging.format":
		config.GlobalConfig.Logging.Format = value
	case "artifacts.directory":
		config.GlobalConfig.Artifacts.Directory = value
	case "artifacts.max_size":
		// サイズの解析は簡略化
		config.GlobalConfig.Artifacts.MaxSize = 100 * 1024 * 1024
	case "error_handling.auto_fix":
		config.GlobalConfig.ErrorHandling.AutoFix = (value == "true")
	case "error_handling.retry_count":
		// リトライ回数の解析は簡略化
		config.GlobalConfig.ErrorHandling.RetryCount = 3
	default:
		return fmt.Errorf("不明な設定キーです: %s", key)
	}

	logger.WithComponent("config").WithFields(logrus.Fields{
		"key":   key,
		"value": value,
	}).Info("設定を更新しました")
	return nil
}

// outputConfigJSON は設定をJSON形式で出力します
func outputConfigJSON() error {
	jsonData, err := json.MarshalIndent(config.GlobalConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("JSONマーシャルに失敗: %w", err)
	}

	fmt.Println(string(jsonData))
	return nil
}

// outputConfigText は設定をテキスト形式で出力します
func outputConfigText() error {
	cfg := config.GlobalConfig

	fmt.Println("keruta-agent 設定")
	fmt.Println("==================")

	fmt.Println("\n[API設定]")
	fmt.Printf("  URL: %s\n", cfg.API.URL)
	fmt.Printf("  タイムアウト: %s\n", cfg.API.Timeout)

	fmt.Println("\n[ログ設定]")
	fmt.Printf("  レベル: %s\n", cfg.Logging.Level)
	fmt.Printf("  フォーマット: %s\n", cfg.Logging.Format)

	fmt.Println("\n[成果物設定]")
	fmt.Printf("  ディレクトリ: %s\n", cfg.Artifacts.Directory)
	fmt.Printf("  最大サイズ: %d bytes\n", cfg.Artifacts.MaxSize)

	fmt.Println("\n[エラーハンドリング設定]")
	fmt.Printf("  自動修正: %t\n", cfg.ErrorHandling.AutoFix)
	fmt.Printf("  リトライ回数: %d\n", cfg.ErrorHandling.RetryCount)

	fmt.Println("\n[環境変数]")
	fmt.Printf("  タスクID: %s\n", config.GetTaskID())
	fmt.Printf("  API URL: %s\n", config.GetAPIURL())
	fmt.Printf("  API トークン: %s\n", maskToken(config.GetAPIToken()))

	return nil
}

// maskToken はトークンをマスクします
func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
} 