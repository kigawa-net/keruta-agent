package logger

import (
	"os"

	"keruta-agent/internal/config"

	"github.com/sirupsen/logrus"
)

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