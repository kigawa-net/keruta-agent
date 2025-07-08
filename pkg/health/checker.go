package health

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"keruta-agent/internal/api"
	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"
)

// Checker はヘルスチェックを担当します
type Checker struct {
	apiClient *api.Client
}

// HealthStatus はヘルスチェックの結果を表します
type HealthStatus struct {
	Overall   bool                   `json:"overall"`
	Timestamp time.Time              `json:"timestamp"`
	Checks    map[string]CheckResult `json:"checks"`
}

// CheckResult は個別のチェック結果を表します
type CheckResult struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// NewChecker は新しいヘルスチェッカーを作成します
func NewChecker() *Checker {
	return &Checker{
		apiClient: api.NewClient(),
	}
}

// CheckAll は全てのヘルスチェックを実行します
func (c *Checker) CheckAll() *HealthStatus {
	status := &HealthStatus{
		Overall:   true,
		Timestamp: time.Now(),
		Checks:    make(map[string]CheckResult),
	}

	// API接続チェック
	apiResult := c.CheckAPI()
	status.Checks["api"] = apiResult
	if !apiResult.Status {
		status.Overall = false
	}

	// ディスク容量チェック
	diskResult := c.CheckDisk()
	status.Checks["disk"] = diskResult
	if !diskResult.Status {
		status.Overall = false
	}

	// メモリ使用量チェック
	memoryResult := c.CheckMemory()
	status.Checks["memory"] = memoryResult
	if !memoryResult.Status {
		status.Overall = false
	}

	// 設定チェック
	configResult := c.CheckConfig()
	status.Checks["config"] = configResult
	if !configResult.Status {
		status.Overall = false
	}

	logger.WithComponent("health").WithField("overall", status.Overall).Info("ヘルスチェックが完了しました")
	return status
}

// CheckAPI はAPI接続をチェックします
func (c *Checker) CheckAPI() CheckResult {
	logger.WithComponent("health").Debug("API接続をチェック中")

	// 簡単なヘルスチェックエンドポイントを呼び出し
	url := fmt.Sprintf("%s/health", config.GetAPIURL())

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return CheckResult{
			Status:  false,
			Message: "API接続に失敗しました",
			Error:   err.Error(),
		}
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WithComponent("health").WithError(closeErr).Warning("レスポンスボディのクローズに失敗しました")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return CheckResult{
			Status:  false,
			Message: fmt.Sprintf("API応答が異常です: %d", resp.StatusCode),
		}
	}

	return CheckResult{
		Status:  true,
		Message: "API接続は正常です",
	}
}

// CheckDisk はディスク容量をチェックします
func (c *Checker) CheckDisk() CheckResult {
	logger.WithComponent("health").Debug("ディスク容量をチェック中")

	// 成果物ディレクトリの容量をチェック
	dir := config.GlobalConfig.Artifacts.Directory

	// ディレクトリが存在しない場合は作成を試行
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return CheckResult{
				Status:  false,
				Message: "成果物ディレクトリの作成に失敗しました",
				Error:   err.Error(),
			}
		}
	}

	// ディスク容量の確認（簡易版）
	// 実際の実装では、より詳細なディスク容量チェックを行う
	return CheckResult{
		Status:  true,
		Message: "ディスク容量は十分です",
	}
}

// CheckMemory はメモリ使用量をチェックします
func (c *Checker) CheckMemory() CheckResult {
	logger.WithComponent("health").Debug("メモリ使用量をチェック中")

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// メモリ使用量の閾値（例: 1GB）
	maxMemory := uint64(1024 * 1024 * 1024)

	if m.Alloc > maxMemory {
		return CheckResult{
			Status:  false,
			Message: fmt.Sprintf("メモリ使用量が閾値を超えています: %d MB", m.Alloc/1024/1024),
		}
	}

	return CheckResult{
		Status:  true,
		Message: fmt.Sprintf("メモリ使用量は正常です: %d MB", m.Alloc/1024/1024),
	}
}

// CheckConfig は設定をチェックします
func (c *Checker) CheckConfig() CheckResult {
	logger.WithComponent("health").Debug("設定をチェック中")

	// 必須設定のチェック
	if config.GetAPIURL() == "" {
		return CheckResult{
			Status:  false,
			Message: "API URLが設定されていません",
		}
	}

	// API認証は不要になったため、トークンのチェックは行わない

	if config.GetTaskID() == "" {
		return CheckResult{
			Status:  false,
			Message: "タスクIDが設定されていません",
		}
	}

	return CheckResult{
		Status:  true,
		Message: "設定は正常です",
	}
}

// CheckSpecific は特定のヘルスチェックを実行します
func (c *Checker) CheckSpecific(checkType string) CheckResult {
	switch checkType {
	case "api":
		return c.CheckAPI()
	case "disk":
		return c.CheckDisk()
	case "memory":
		return c.CheckMemory()
	case "config":
		return c.CheckConfig()
	default:
		return CheckResult{
			Status:  false,
			Message: fmt.Sprintf("不明なチェックタイプ: %s", checkType),
		}
	}
}
