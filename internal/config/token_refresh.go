package config

import (
	"os"
	"sync"
)

var (
	tokenMutex sync.Mutex
)

// RefreshAPIToken は環境変数から最新のトークンを取得して更新します
func RefreshAPIToken() string {
	tokenMutex.Lock()
	defer tokenMutex.Unlock()

	// 環境変数から最新のトークンを取得
	if token := os.Getenv("KERUTA_API_TOKEN"); token != "" && token != GlobalConfig.API.Token {
		// トークンが更新されていれば、設定を更新
		GlobalConfig.API.Token = token
		// ログ出力は行わない（循環インポートを避けるため）
	}

	return GlobalConfig.API.Token
}
