package commands

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"keruta-agent/internal/api"
	"keruta-agent/internal/config"
	"keruta-agent/internal/logger"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	// executeCmd flags
	apiURL          string
	workDir         string
	logLevel        string
	autoDetectInput bool
	timeout         int
)

// executeCmd はタスク実行コマンドです
var executeCmd = &cobra.Command{
	Use:   "execute",
	Short: "指定されたタスクIDのスクリプトを実行",
	Long: `指定されたタスクIDのスクリプトをkeruta APIから取得し、サブプロセスとして実行します。
WebSocket通信により、タスク状態や標準入力、ログなどをリアルタイムで連携します。
入力待ち状態を自動検出します。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runExecute()
	},
	Example: `  # 基本的な実行
  keruta execute --task-id task123

  # カスタム設定での実行
  keruta execute \
      --task-id task123 \
      --api-url http://keruta-api:8080 \
      --work-dir /work \
      --log-level DEBUG

  # タイムアウト付きで実行
  keruta execute \
      --timeout 300 \
      --task-id task123`,
}

func init() {
	// フラグの設定
	executeCmd.Flags().StringVar(&apiURL, "api-url", "", "keruta APIのURL（環境変数KERUTA_API_URLから自動取得）")
	executeCmd.Flags().StringVar(&workDir, "work-dir", "/work", "作業ディレクトリ")
	executeCmd.Flags().StringVar(&logLevel, "log-level", "INFO", "ログレベル（DEBUG, INFO, WARN, ERROR）")
	executeCmd.Flags().BoolVar(&autoDetectInput, "auto-detect-input", true, "入力待ち状態の自動検出")
	executeCmd.Flags().IntVar(&timeout, "timeout", 0, "サブプロセスのタイムアウト時間（秒）、0は無制限")
}

// runExecute はexecuteコマンドの実行ロジックです
func runExecute() error {
	taskID := config.GetTaskID()
	if taskID == "" {
		return fmt.Errorf("タスクIDが設定されていません。--task-idフラグまたはKERUTA_TASK_ID環境変数を設定してください")
	}

	logger.WithTaskIDAndComponent("execute").Info("タスク実行を開始します")

	// APIクライアントの作成
	if apiURL != "" {
		os.Setenv("KERUTA_API_URL", apiURL)
	}
	client := api.NewClient()

	// WebSocket機能は削除されました
	logger.WithTaskIDAndComponent("execute").Info("WebSocket機能は削除されました")

	// ログレベルの設定
	setLogLevel(logLevel)

	// 作業ディレクトリの作成
	if err := createWorkDir(workDir); err != nil {
		return err
	}

	// タスクステータスをPROCESSINGに更新
	if err := updateTaskStatusProcessing(client, taskID); err != nil {
		return err
	}

	// 1. APIからスクリプトを取得
	script, err := retrieveScript(client, taskID)
	if err != nil {
		return err
	}

	// 2. スクリプトをファイルに保存
	scriptPath, err := saveScriptToFile(script, workDir)
	if err != nil {
		updateTaskStatusFailed(client, taskID, "スクリプトファイルの作成に失敗しました", "SCRIPT_WRITE_ERROR")
		return err
	}

	// 3. サブプロセスとして実行
	cmd := setupCommand(scriptPath, workDir, script.Parameters)

	// 4. 標準出力・標準エラー出力をキャプチャ
	stdout, stderr, stdin, err := setupIOPipes(cmd)
	if err != nil {
		updateTaskStatusFailed(client, taskID, err.Error(), "PIPE_ERROR")
		return err
	}

	// 5. サブプロセスの開始
	if err := startCommand(cmd); err != nil {
		updateTaskStatusFailed(client, taskID, "スクリプトの実行開始に失敗しました", "EXECUTION_ERROR")
		return err
	}

	// 6. 標準出力・標準エラー出力の処理
	processOutputs(client, taskID, stdout, stderr, stdin)

	// 7. サブプロセスの終了を待機
	exitErr := waitForCommand(cmd, timeout)

	// 8. サブプロセスの終了状態に応じてタスク状態を更新
	if exitErr != nil {
		logger.WithTaskIDAndComponent("execute").WithError(exitErr).Error("スクリプトの実行に失敗しました")
		updateTaskStatusFailed(client, taskID, "スクリプトの実行に失敗しました", "EXECUTION_ERROR")
		return fmt.Errorf("スクリプトの実行に失敗: %w", exitErr)
	}

	// 実行完了後、タスクステータスをCOMPLETEDに更新
	if err := updateTaskStatusCompleted(client, taskID); err != nil {
		return err
	}

	logger.WithTaskIDAndComponent("execute").Info("タスク実行が完了しました")
	return nil
}

// createWorkDir は作業ディレクトリを作成します
func createWorkDir(workDir string) error {
	if err := os.MkdirAll(workDir, 0755); err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Error("作業ディレクトリの作成に失敗しました")
		return fmt.Errorf("作業ディレクトリの作成に失敗: %w", err)
	}
	return nil
}

// updateTaskStatusProcessing はタスクステータスをPROCESSINGに更新します
func updateTaskStatusProcessing(client *api.Client, taskID string) error {
	err := client.UpdateTaskStatus(
		taskID,
		api.TaskStatusProcessing,
		"タスクを実行中",
		0,
		"",
	)
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Warning("タスクステータスの更新に失敗しましたが、処理を継続します")
		// エラーを返さずに処理を継続
		return nil
	}
	return nil
}

// updateTaskStatusWaitingForInput はタスクステータスをWAITING_FOR_INPUTに更新します
func updateTaskStatusWaitingForInput(client *api.Client, taskID string) error {
	err := client.UpdateTaskStatus(
		taskID,
		api.TaskStatusWaitingForInput,
		"ユーザー入力を待機中",
		50,
		"",
	)
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Warning("入力待ち状態への更新に失敗しましたが、処理を継続します")
		// エラーを返さずに処理を継続
		return nil
	}
	return nil
}

// updateTaskStatusResumeProcessing はタスクステータスを入力受付後にPROCESSINGに戻します
func updateTaskStatusResumeProcessing(client *api.Client, taskID string) error {
	err := client.UpdateTaskStatus(
		taskID,
		api.TaskStatusProcessing,
		"処理を再開しました",
		60,
		"",
	)
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Warning("処理中状態への更新に失敗しましたが、処理を継続します")
		// エラーを返さずに処理を継続
		return nil
	}
	return nil
}

// updateTaskStatusCompleted はタスクステータスをCOMPLETEDに更新します
func updateTaskStatusCompleted(client *api.Client, taskID string) error {
	err := client.UpdateTaskStatus(
		taskID,
		api.TaskStatusCompleted,
		"タスクが正常に完了しました",
		100,
		"",
	)
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Warning("タスクステータスの更新に失敗しましたが、処理を継続します")
		// エラーを返さずに処理を継続
		return nil
	}
	return nil
}

// updateTaskStatusFailed はタスクステータスを失敗に更新するヘルパー関数です
func updateTaskStatusFailed(client *api.Client, taskID, message, errorCode string) {
	err := client.UpdateTaskStatus(
		taskID,
		api.TaskStatusFailed,
		message,
		0,
		errorCode,
	)
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Warning("失敗ステータスの更新に失敗しましたが、処理を継続します")
	}
}

// setLogLevel はログレベルを設定します
func setLogLevel(level string) {
	switch level {
	case "DEBUG":
		logrus.SetLevel(logrus.DebugLevel)
	case "INFO":
		logrus.SetLevel(logrus.InfoLevel)
	case "WARN":
		logrus.SetLevel(logrus.WarnLevel)
	case "ERROR":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}
}

// retrieveScript はAPIからスクリプトを取得します
func retrieveScript(client *api.Client, taskID string) (*api.Script, error) {
	logger.WithTaskIDAndComponent("execute").Info("スクリプトを取得中...")
	script, err := client.GetScript(taskID)
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Error("スクリプトの取得に失敗しました")
		updateTaskStatusFailed(client, taskID, "スクリプトの取得に失敗しました", "SCRIPT_FETCH_ERROR")
		return nil, fmt.Errorf("スクリプトの取得に失敗: %w", err)
	}
	return script, nil
}

// saveScriptToFile はスクリプトをファイルに保存します
func saveScriptToFile(script *api.Script, workDir string) (string, error) {
	scriptPath := filepath.Join(workDir, script.Filename)
	err := os.WriteFile(scriptPath, []byte(script.Content), 0755)
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Error("スクリプトファイルの作成に失敗しました")
		return "", fmt.Errorf("スクリプトファイルの作成に失敗: %w", err)
	}
	logger.WithTaskIDAndComponent("execute").WithField("path", scriptPath).Info("スクリプトファイルを作成しました")
	return scriptPath, nil
}

// setupCommand はサブプロセスのコマンドを設定します
func setupCommand(scriptPath, workDir string, parameters map[string]interface{}) *exec.Cmd {
	cmd := exec.Command(scriptPath)
	cmd.Dir = workDir

	// 環境変数の設定
	cmd.Env = os.Environ()
	if parameters != nil {
		if env, ok := parameters["env"].(map[string]interface{}); ok {
			for k, v := range env {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}
	return cmd
}

// setupIOPipes はコマンドの標準入出力パイプを設定します
func setupIOPipes(cmd *exec.Cmd) (io.ReadCloser, io.ReadCloser, io.WriteCloser, error) {
	// 標準出力パイプの作成
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Error("標準出力パイプの作成に失敗しました")
		return nil, nil, nil, fmt.Errorf("標準出力パイプの作成に失敗: %w", err)
	}

	// 標準エラー出力パイプの作成
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Error("標準エラー出力パイプの作成に失敗しました")
		return nil, nil, nil, fmt.Errorf("標準エラー出力パイプの作成に失敗: %w", err)
	}

	// 標準入力パイプの作成
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Error("標準入力パイプの作成に失敗しました")
		return nil, nil, nil, fmt.Errorf("標準入力パイプの作成に失敗: %w", err)
	}

	return stdoutPipe, stderrPipe, stdinPipe, nil
}

// startCommand はコマンドを開始します
func startCommand(cmd *exec.Cmd) error {
	logger.WithTaskIDAndComponent("execute").Info("スクリプトの実行を開始します")
	if err := cmd.Start(); err != nil {
		logger.WithTaskIDAndComponent("execute").WithError(err).Error("スクリプトの実行開始に失敗しました")
		return fmt.Errorf("スクリプトの実行開始に失敗: %w", err)
	}
	return nil
}

// isInputWaiting は行が入力待ち状態かどうかを判定します
func isInputWaiting(line string) bool {
	return autoDetectInput && len(line) > 0 && (line[len(line)-1:] == ":" || line[len(line)-1:] == ">" || line[len(line)-1:] == "?")
}

// processStdout は標準出力を処理します
func processStdout(client *api.Client, taskID string, stdout io.ReadCloser, stdin io.WriteCloser) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		logger.WithTaskIDAndComponent("script").Info(line)
		client.SendLog(taskID, "INFO", line)

		// 入力待ち状態の検出
		if isInputWaiting(line) {
			logger.WithTaskIDAndComponent("execute").Info("入力待ち状態を検出しました")

			// タスクステータスを入力待ち状態に更新
			updateTaskStatusWaitingForInput(client, taskID)

			// WebSocketを通じて入力待ち状態を通知し、入力を待機
			go func() {
				// 入力を待機
				input, err := client.WaitForInput(taskID, line)
				if err != nil {
					logger.WithTaskIDAndComponent("execute").WithError(err).Error("入力の待機に失敗しました")
					return
				}

				logger.WithTaskIDAndComponent("execute").Info("入力を受け付けました")

				// タスクステータスを処理中に戻す
				updateTaskStatusResumeProcessing(client, taskID)

				// 入力をサブプロセスに送信
				_, err = stdin.Write([]byte(input))
				if err != nil {
					logger.WithTaskIDAndComponent("execute").WithError(err).Error("標準入力の書き込みに失敗しました")
				}
			}()
		}
	}
}

// processStderr は標準エラー出力を処理します
func processStderr(client *api.Client, taskID string, stderr io.ReadCloser) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		logger.WithTaskIDAndComponent("script").Error(line)
		client.SendLog(taskID, "ERROR", line)
	}
}

// processOutputs は標準出力と標準エラー出力を処理します
func processOutputs(client *api.Client, taskID string, stdout, stderr io.ReadCloser, stdin io.WriteCloser) {
	// 標準出力の処理
	go processStdout(client, taskID, stdout, stdin)

	// 標準エラー出力の処理
	go processStderr(client, taskID, stderr)
}

// waitForCommand はコマンドの終了を待機します
func waitForCommand(cmd *exec.Cmd, timeoutSec int) error {
	// タイムアウト処理
	var timeoutCh <-chan time.Time
	if timeoutSec > 0 {
		timeoutCh = time.After(time.Duration(timeoutSec) * time.Second)
	}

	// 完了チャネル
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	// サブプロセスの終了を待機
	select {
	case <-timeoutCh:
		// タイムアウト発生
		logger.WithTaskIDAndComponent("execute").Warn("スクリプト実行がタイムアウトしました")
		if cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGTERM)
			time.Sleep(2 * time.Second)
			cmd.Process.Kill()
		}
		return fmt.Errorf("スクリプト実行がタイムアウトしました")
	case err := <-doneCh:
		return err
	}
}
