package logger

import (
	"os"
	"strings"

	"keruta-agent/internal/config"

	"github.com/sirupsen/logrus"
)

// LogSender はログを送信するためのインターフェースです
type LogSender interface {
	SendLog(taskID string, level string, message string) error
}

// APILogHook はAPIにログを送信するためのHookです
type APILogHook struct {
	client LogSender
}

// NewAPILogHook は新しいAPILogHookを作成します
func NewAPILogHook(client LogSender) *APILogHook {
	return &APILogHook{client: client}
}

// Levels はHookが処理するログレベルを返します
func (hook *APILogHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.InfoLevel,
		logrus.WarnLevel,
		logrus.ErrorLevel,
		logrus.FatalLevel,
		logrus.PanicLevel,
	}
}

// Fire はログエントリーをAPIに送信します
func (hook *APILogHook) Fire(entry *logrus.Entry) error {
	if hook.client == nil {
		return nil
	}

	// API関連のログは送信しない（無限ループ防止）
	if component, ok := entry.Data["component"]; ok && component == "api" {
		return nil
	}

	// タスクIDを取得
	taskID := config.GetTaskID()
	if taskID == "" {
		return nil // タスクIDが設定されていない場合は送信しない
	}

	// ログレベルを文字列に変換
	level := strings.ToUpper(entry.Level.String())

	// APIにログを送信（エラーは無視）
	go func() {
		_ = hook.client.SendLog(taskID, level, entry.Message)
	}()

	return nil
}

var apiLogHook *APILogHook

// SetAPIClient はAPIクライアントを設定してログのAPI送信を有効化します
func SetAPIClient(client LogSender) {
	if apiLogHook != nil {
		logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	}
	
	apiLogHook = NewAPILogHook(client)
	logrus.AddHook(apiLogHook)
}

// Init はロガーを初期化します
func Init() error {
	// ログレベルの設定
	level, err := logrus.ParseLevel(config.GlobalConfig.Logging.Level)
	if err != nil {
		return err
	}
	logrus.SetLevel(level)

	// ログフォーマットの設定
	if config.GlobalConfig.Logging.Format == "json" {
		logrus.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
	}

	// 出力先の設定
	logrus.SetOutput(os.Stdout)

	return nil
}

// WithTaskID はタスクID付きのログエントリを作成します
func WithTaskID() *logrus.Entry {
	return logrus.WithField("task_id", config.GetTaskID())
}

// WithComponent はコンポーネント名付きのログエントリを作成します
func WithComponent(component string) *logrus.Entry {
	return logrus.WithField("component", component)
}

// WithTaskIDAndComponent はタスクIDとコンポーネント名付きのログエントリを作成します
func WithTaskIDAndComponent(component string) *logrus.Entry {
	return logrus.WithFields(logrus.Fields{
		"task_id":   config.GetTaskID(),
		"component": component,
	})
} 