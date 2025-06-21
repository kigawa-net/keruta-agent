package commands

import (
	"encoding/json"
	"fmt"
	"time"

	"keruta-agent/internal/logger"
	"keruta-agent/pkg/health"

	"github.com/spf13/cobra"
)

var (
	healthCheckType string
	healthFormat    string
)

// healthCmd はヘルスチェックコマンドです
var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "ヘルスチェックを実行",
	Long: `システムのヘルスチェックを実行します。
API接続、ディスク容量、メモリ使用量、設定などをチェックします。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHealth()
	},
}

func init() {
	healthCmd.Flags().StringVar(&healthCheckType, "check", "", "特定のチェックを実行 (api, disk, memory, config)")
	healthCmd.Flags().StringVar(&healthFormat, "format", "text", "出力形式 (text, json)")
}

// runHealth はhealthコマンドの実行ロジックです
func runHealth() error {
	logger.WithComponent("health").Info("ヘルスチェックを開始します")

	// ヘルスチェッカーの作成
	checker := health.NewChecker()

	var status *health.HealthStatus

	// 特定のチェックタイプが指定されている場合
	if healthCheckType != "" {
		result := checker.CheckSpecific(healthCheckType)
		status = &health.HealthStatus{
			Overall:   result.Status,
			Timestamp: time.Now(),
			Checks: map[string]health.CheckResult{
				healthCheckType: result,
			},
		}
	} else {
		// 全てのチェックを実行
		status = checker.CheckAll()
	}

	// 結果の出力
	if err := outputHealthStatus(status); err != nil {
		logger.WithComponent("health").WithError(err).Error("ヘルスチェック結果の出力に失敗しました")
		return fmt.Errorf("ヘルスチェック結果の出力に失敗: %w", err)
	}

	// 終了コードの設定
	if !status.Overall {
		logger.WithComponent("health").Error("ヘルスチェックが失敗しました")
		return fmt.Errorf("ヘルスチェックが失敗しました")
	}

	logger.WithComponent("health").Info("ヘルスチェックが完了しました")
	return nil
}

// outputHealthStatus はヘルスチェック結果を出力します
func outputHealthStatus(status *health.HealthStatus) error {
	switch healthFormat {
	case "json":
		return outputHealthStatusJSON(status)
	case "text":
		return outputHealthStatusText(status)
	default:
		return fmt.Errorf("無効な出力形式です: %s", healthFormat)
	}
}

// outputHealthStatusJSON はヘルスチェック結果をJSON形式で出力します
func outputHealthStatusJSON(status *health.HealthStatus) error {
	jsonData, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("JSONマーシャルに失敗: %w", err)
	}

	fmt.Println(string(jsonData))
	return nil
}

// outputHealthStatusText はヘルスチェック結果をテキスト形式で出力します
func outputHealthStatusText(status *health.HealthStatus) error {
	fmt.Printf("ヘルスチェック結果 (%s)\n", status.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Println("==========================================")

	overallStatus := "OK"
	if !status.Overall {
		overallStatus = "NG"
	}
	fmt.Printf("全体ステータス: %s\n\n", overallStatus)

	for checkType, result := range status.Checks {
		statusText := "OK"
		if !result.Status {
			statusText = "NG"
		}

		fmt.Printf("[%s] %s: %s\n", statusText, checkType, result.Message)
		if result.Error != "" {
			fmt.Printf("    エラー: %s\n", result.Error)
		}
	}

	fmt.Println()
	return nil
} 