package commands

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"keruta-agent/internal/api"

	"github.com/sirupsen/logrus"
)

// executeTmuxClaudeTask はtmux環境でClaude実行タスクを実行します
func executeTmuxClaudeTask(ctx context.Context, apiClient *api.Client, taskID string, taskContent string, taskLogger *logrus.Entry) error {
	taskLogger.Info("🎯 tmux環境でClaude実行タスクを開始しています...")

	// セッションIDからtmuxセッション名を生成（1セッション = 1tmuxセッション）
	var tmuxSessionName string
	if daemonSessionID != "" {
		tmuxSessionName = fmt.Sprintf("keruta-session-%s", daemonSessionID[:8])
	} else {
		// フォールバック: タスクIDベース（後方互換性）
		tmuxSessionName = fmt.Sprintf("keruta-task-%s", taskID[:8])
	}

	// 既存のtmuxセッションをチェック
	taskLogger.WithFields(logrus.Fields{
		"session_name": tmuxSessionName,
		"session_id":   daemonSessionID,
	}).Debug("🔍 既存のtmuxセッションをチェックしています...")

	_, sessionErr := getTmuxSessionStatus(tmuxSessionName)
	if sessionErr == nil {
		taskLogger.WithFields(logrus.Fields{
			"existing_session": tmuxSessionName,
			"session_id":       daemonSessionID,
		}).Info("✅ 既存のtmuxセッションが見つかりました。再利用します")

		// セッション再利用をAPIにログ送信
		logMessage := fmt.Sprintf("[tmux:%s:reuse] 既存のtmuxセッションを再利用します", tmuxSessionName)
		if sendErr := apiClient.SendLog(taskID, "INFO", logMessage); sendErr != nil {
			taskLogger.WithError(sendErr).Warning("セッション再利用ログの送信に失敗しました")
		}

		// 既存セッションでClaude実行 + ストリーミング
		return executeTmuxCommandInSessionWithStreaming(ctx, apiClient, taskID, taskContent, tmuxSessionName, taskLogger)
	}

	taskLogger.WithFields(logrus.Fields{
		"session_name": tmuxSessionName,
		"error":        sessionErr.Error(),
	}).Debug("❌ 既存のtmuxセッションが見つかりませんでした。新規作成します")

	// ~/keruta ディレクトリの存在を確認・作成
	kerutaDir := os.ExpandEnv("$HOME/keruta")
	if err := ensureDirectory(kerutaDir); err != nil {
		return fmt.Errorf("~/kerutaディレクトリの作成に失敗: %w", err)
	}

	taskLogger.WithFields(logrus.Fields{
		"tmux_session": tmuxSessionName,
		"working_dir":  kerutaDir,
		"task_content": taskContent,
	}).Info("tmuxセッションでClaude実行を開始します")
	if taskContent == "" {
		taskContent = "none"
	}
	// tmuxコマンドを構築 - セッション作成、ディレクトリ移動、Claude実行
	tmuxCmd := exec.CommandContext(ctx, "claude", "-p", "--dangerously-skip-permissions")
	tmuxCmd.Stdin = strings.NewReader(taskContent)
	tmuxCmd.Dir = kerutaDir

	taskLogger.WithFields(logrus.Fields{
		"tmux_session": tmuxSessionName,
		"working_dir":  kerutaDir,
		"command":      tmuxCmd.Args,
	}).Info("🖥️ tmuxコマンドを構築しました")

	// コマンド実行とログ収集
	return executeTmuxCommand(ctx, tmuxCmd, apiClient, taskID, tmuxSessionName, taskLogger)
}

// ensureDirectory はディレクトリの存在を確認し、存在しない場合は作成します
func ensureDirectory(dirPath string) error {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return os.MkdirAll(dirPath, 0755)
	}
	return nil
}

// executeTmuxCommand はtmuxコマンドを実行し、出力を監視します
func executeTmuxCommand(ctx context.Context, cmd *exec.Cmd, apiClient *api.Client, taskID, sessionName string, logger *logrus.Entry) error {
	logger.Info("🚀 tmuxセッションを起動しています...")

	// tmuxセッション開始
	logger.WithFields(logrus.Fields{
		"session": sessionName,
		"command": strings.Join(cmd.Args, " "),
	}).Info("⚡ tmuxセッションを開始します")

	// リアルタイムストリーミング処理でコマンドを実行
	output, err := executeCommandWithStreaming(ctx, cmd, apiClient, taskID, logger)
	if err != nil {
		outputStr := strings.TrimSpace(string(output))

		// セッション名の競合をチェック
		if strings.Contains(outputStr, "duplicate session") || strings.Contains(outputStr, "session already exists") {
			logger.WithFields(logrus.Fields{
				"session": sessionName,
				"output":  outputStr,
			}).Warning("⚠️ tmuxセッション名が競合しています。既存セッションを確認して再利用を試行します")

			// 既存セッションが見つかった場合は再利用
			if _, statusErr := getTmuxSessionStatus(sessionName); statusErr == nil {
				logger.WithField("session", sessionName).Info("🔄 セッション競合が発生しましたが、既存セッションを再利用します")

				// セッション競合による再利用をAPIにログ送信
				logMessage := fmt.Sprintf("[tmux:%s:conflict-reuse] セッション競合により既存セッションを再利用します", sessionName)
				if sendErr := apiClient.SendLog(taskID, "WARNING", logMessage); sendErr != nil {
					logger.WithError(sendErr).Warning("セッション競合ログの送信に失敗しました")
				}

				// 競合が発生した場合は、tmuxセッション作成を中止して正常終了
				// （既存セッションが利用可能であることを示す）
				logger.WithField("session", sessionName).Info("🎯 セッション競合により新規作成を中止し、正常終了します")
				return nil
			}
		}

		logger.WithError(err).WithFields(logrus.Fields{
			"session": sessionName,
			"output":  outputStr,
		}).Error("❌ tmuxセッション開始に失敗")

		// APIにもエラー出力を送信
		if len(output) > 0 {
			logMessage := fmt.Sprintf("[tmux:%s:start-cmd] %s", sessionName, outputStr)
			if sendErr := apiClient.SendLog(taskID, "ERROR", logMessage); sendErr != nil {
				logger.WithError(sendErr).Warning("tmux開始エラーログ送信に失敗しました")
			}
		}

		return fmt.Errorf("tmuxセッション開始に失敗: %w", err)
	}

	// tmux開始コマンドの出力をログ表示
	if len(output) > 0 {
		logger.WithFields(logrus.Fields{
			"session": sessionName,
			"output":  strings.TrimSpace(string(output)),
		}).Info("📋 tmux開始コマンドの出力")

		// APIにもログ送信
		logMessage := fmt.Sprintf("[tmux:%s:start-cmd] %s", sessionName, strings.TrimSpace(string(output)))
		if sendErr := apiClient.SendLog(taskID, "INFO", logMessage); sendErr != nil {
			logger.WithError(sendErr).Warning("tmux開始ログ送信に失敗しました")
		}
	} else {
		logger.WithField("session", sessionName).Info("✅ tmuxセッションが正常に開始されました")

		// セッション開始成功をAPIにログ送信
		logMessage := fmt.Sprintf("[tmux:%s:start-cmd] tmuxセッションが正常に開始されました", sessionName)
		if sendErr := apiClient.SendLog(taskID, "INFO", logMessage); sendErr != nil {
			logger.WithError(sendErr).Warning("tmux開始成功ログの送信に失敗しました")
		}
	}

	// tmuxセッションの出力を監視
	logger.WithField("session", sessionName).Info("👁️ tmux出力監視を開始します")
	go func() {
		ticker := time.NewTicker(1 * time.Second) // より頻繁にキャプチャ
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.WithField("session", sessionName).Debug("コンテキストキャンセルによりtmux監視を停止")
				return
			case <-ticker.C:
				if err := captureTmuxOutput(apiClient, taskID, sessionName, logger); err != nil {
					logger.WithError(err).Debug("tmux出力キャプチャに失敗しました")
				}
			}
		}
	}()

	// tmuxは detached モードで実行されているため、完了を待つ必要はない
	logger.WithField("session", sessionName).Info("🔄 tmuxセッションはバックグラウンドで実行中です")

	// ストリーミング出力監視を開始（バックグラウンド）
	streamCtx, cancelStream := context.WithTimeout(ctx, 5*time.Minute) // 最大5分間ストリーミング
	go func() {
		defer cancelStream()
		if err := streamTmuxOutput(streamCtx, apiClient, taskID, sessionName, logger); err != nil {
			if err != context.Canceled && err != context.DeadlineExceeded {
				logger.WithError(err).Warning("新規セッションのストリーミング処理でエラーが発生しました")
			}
		}
	}()

	// 少し待機してからストリーミングを開始
	time.Sleep(1 * time.Second)

	// 最終的な出力をキャプチャ
	if err := captureTmuxOutput(apiClient, taskID, sessionName, logger); err != nil {
		logger.WithError(err).Warning("最終出力キャプチャに失敗しました")
	}

	// tmuxセッションはクリーンアップしない（再利用のため保持）
	logger.WithField("session", sessionName).Info("tmuxセッションを保持します（再利用のため）")

	logger.Info("✅ tmux Claude実行タスクが完了しました")
	return nil
}

// captureTmuxOutput はtmuxセッションの出力をキャプチャしてAPIに送信します
func captureTmuxOutput(apiClient *api.Client, taskID, sessionName string, logger *logrus.Entry) error {
	logger.WithField("session", sessionName).Debug("🔍 tmuxセッション出力キャプチャを開始")

	// まずtmuxセッションが存在するかチェック
	if _, err := getTmuxSessionStatus(sessionName); err != nil {
		logger.WithError(err).WithField("session", sessionName).Debug("tmuxセッションが存在しないため出力キャプチャをスキップ")
		return nil // セッションが存在しない場合はエラーにしない
	}

	// tmux capture-pane で出力を取得（履歴も含む）
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-S", "-3000")

	logger.WithFields(logrus.Fields{
		"session": sessionName,
		"command": strings.Join(cmd.Args, " "),
	}).Debug("📸 tmux capture-paneコマンドを実行します")

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.WithError(err).WithFields(logrus.Fields{
			"session": sessionName,
			"output":  string(output),
		}).Debug("tmux出力キャプチャに失敗（セッションが存在しない可能性）")

		// キャプチャエラーの詳細をAPIに送信
		if len(output) > 0 {
			logMessage := fmt.Sprintf("[tmux:%s:capture-error] %s", sessionName, strings.TrimSpace(string(output)))
			if sendErr := apiClient.SendLog(taskID, "DEBUG", logMessage); sendErr != nil {
				logger.WithError(sendErr).Debug("キャプチャエラーログ送信に失敗しました")
			}
		}

		return nil // セッション出力キャプチャの失敗は致命的エラーにしない
	}

	// capture-paneコマンドの実行ログ
	logger.WithFields(logrus.Fields{
		"session":    sessionName,
		"bytes_read": len(output),
	}).Debug("✅ tmux capture-paneが正常に実行されました")

	// 出力が空でない場合のみログ送信
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" {
		logger.WithFields(logrus.Fields{
			"session":     sessionName,
			"lines_count": len(strings.Split(outputStr, "\n")),
		}).Debug("📄 tmux出力をキャプチャしました")

		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				// ログにプレフィックスを追加してtmux出力であることを明示
				logMessage := fmt.Sprintf("[tmux:%s] %s", sessionName, line)
				logger.Info(logMessage)
				// APIにログを送信
				if sendErr := apiClient.SendLog(taskID, "INFO", logMessage); sendErr != nil {
					logger.WithError(sendErr).Warning("ログ送信に失敗しました")
				}
			}
		}
	} else {
		logger.WithField("session", sessionName).Debug("tmux出力は空でした")
	}

	return nil
}

// executeCommandWithStreaming はコマンドをリアルタイムストリーミングで実行します
func executeCommandWithStreaming(ctx context.Context, cmd *exec.Cmd, apiClient *api.Client, taskID string, logger *logrus.Entry) ([]byte, error) {
	logger.Debug("📡 コマンドのリアルタイムストリーミング実行を開始")

	// 標準出力と標準エラーのパイプを作成
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("標準出力パイプの作成に失敗: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("標準エラーパイプの作成に失敗: %w", err)
	}

	// 出力を蓄積するバッファ
	var outputBuffer []byte
	var outputMutex sync.Mutex

	// コマンドを開始
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("コマンドの開始に失敗: %w", err)
	}

	// WaitGroupで並行処理の完了を待機
	var wg sync.WaitGroup
	wg.Add(2)

	// 標準出力をストリーミング
	go func() {
		defer wg.Done()
		streamOutput(ctx, stdout, "stdout", apiClient, taskID, logger, &outputBuffer, &outputMutex)
	}()

	// 標準エラーをストリーミング
	go func() {
		defer wg.Done()
		streamOutput(ctx, stderr, "stderr", apiClient, taskID, logger, &outputBuffer, &outputMutex)
	}()

	// コマンドの完了を待機
	cmdErr := cmd.Wait()

	// ストリーミングの完了を待機
	wg.Wait()

	// 最終的な出力を返す
	outputMutex.Lock()
	finalOutput := make([]byte, len(outputBuffer))
	copy(finalOutput, outputBuffer)
	outputMutex.Unlock()

	return finalOutput, cmdErr
}

// streamOutput は指定されたReaderからの出力をリアルタイムでストリーミングします
func streamOutput(ctx context.Context, reader io.Reader, streamType string, apiClient *api.Client, taskID string, logger *logrus.Entry, outputBuffer *[]byte, outputMutex *sync.Mutex) {
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			logger.Debug("コンテキストキャンセルによりストリーミングを停止")
			return
		default:
			line := scanner.Text()

			// 出力バッファに追加
			outputMutex.Lock()
			*outputBuffer = append(*outputBuffer, []byte(line+"\n")...)
			outputMutex.Unlock()

			// 空行はスキップ
			if strings.TrimSpace(line) == "" {
				continue
			}

			// ログメッセージを作成
			logMessage := fmt.Sprintf("[claude:%s] %s", streamType, line)
			logger.Info(logMessage)

			// APIにリアルタイムでログを送信
			if err := apiClient.SendLog(taskID, "INFO", logMessage); err != nil {
				logger.WithError(err).Warning("リアルタイムログ送信に失敗しました")
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logger.WithError(err).WithField("stream_type", streamType).Warning("ストリーム読み取りでエラーが発生しました")
	}
}

// executeTmuxCommandInSessionWithStreaming は既存のtmuxセッション内でコマンドを実行し、リアルタイムストリーミングも行います
func executeTmuxCommandInSessionWithStreaming(ctx context.Context, apiClient *api.Client, taskID, taskContent, sessionName string, logger *logrus.Entry) error {
	logger.WithField("session", sessionName).Info("🔄 既存のtmuxセッション内でClaude実行タスク（ストリーミング付き）を実行します")

	// ストリーミング用のコンテキスト
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	// ストリーミングをバックグラウンドで開始
	go func() {
		if err := streamTmuxOutput(streamCtx, apiClient, taskID, sessionName, logger); err != nil {
			if err != context.Canceled {
				logger.WithError(err).Warning("ストリーミング処理でエラーが発生しました")
			}
		}
	}()

	// 既存の処理を実行
	err := executeTmuxCommandInSession(ctx, apiClient, taskID, taskContent, sessionName, logger)

	// ストリーミング停止前に少し待機してタスク完了ログを取得
	time.Sleep(3 * time.Second)
	cancelStream()

	return err
}

// streamTmuxOutput はtmuxセッションの出力をリアルタイムでストリーミング処理します
func streamTmuxOutput(ctx context.Context, apiClient *api.Client, taskID, sessionName string, logger *logrus.Entry) error {
	logger.WithField("session", sessionName).Info("🔄 tmuxセッション出力ストリーミングを開始")

	// まずtmuxセッションが存在するかチェック
	if _, err := getTmuxSessionStatus(sessionName); err != nil {
		logger.WithError(err).WithField("session", sessionName).Debug("tmuxセッションが存在しないためストリーミングをスキップ")
		return nil
	}

	var lastLineCount int
	ticker := time.NewTicker(1 * time.Second) // 1秒間隔でポーリング（よりリアルタイム）
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.WithField("session", sessionName).Info("✋ コンテキストキャンセルによりストリーミングを停止")
			return ctx.Err()
		case <-ticker.C:
			// tmux出力を取得
			cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-S", "-3000")
			output, err := cmd.CombinedOutput()
			if err != nil {
				// セッションが終了している可能性
				if _, sessionErr := getTmuxSessionStatus(sessionName); sessionErr != nil {
					logger.WithField("session", sessionName).Info("✅ tmuxセッションが終了しているためストリーミングを停止")
					return nil
				}
				logger.WithError(err).Debug("tmux capture-paneでエラーが発生しましたが継続します")
				continue
			}

			outputStr := strings.TrimSpace(string(output))
			if outputStr == "" {
				continue
			}

			lines := strings.Split(outputStr, "\n")
			currentLineCount := len(lines)

			// 新しい行のみを処理
			if currentLineCount > lastLineCount {
				newLines := lines[lastLineCount:]
				for _, line := range newLines {
					if strings.TrimSpace(line) != "" {
						logMessage := fmt.Sprintf("[tmux:%s:stream] %s", sessionName, line)
						logger.Info(logMessage)

						// APIにログを送信
						if sendErr := apiClient.SendLog(taskID, "INFO", logMessage); sendErr != nil {
							logger.WithError(sendErr).Warning("ストリーミングログ送信に失敗しました")
						}
					}
				}
				lastLineCount = currentLineCount
			}
		}
	}
}

// killTmuxSession はtmuxセッションを終了します
func killTmuxSession(sessionName string, logger *logrus.Entry) error {
	cmd := exec.Command("tmux", "kill-session", "-t", sessionName)

	logger.WithFields(logrus.Fields{
		"session": sessionName,
		"command": strings.Join(cmd.Args, " "),
	}).Info("💀 tmuxセッションを終了します")

	// kill-sessionコマンドの出力をキャプチャ
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.WithError(err).WithFields(logrus.Fields{
			"session": sessionName,
			"output":  string(output),
		}).Error("❌ tmuxセッション終了に失敗")

		if len(output) > 0 {
			return fmt.Errorf("tmuxセッション終了に失敗 (出力: %s): %w", strings.TrimSpace(string(output)), err)
		}
		return fmt.Errorf("tmuxセッション終了に失敗: %w", err)
	}

	// 終了コマンドの出力をログ表示
	if len(output) > 0 {
		logger.WithFields(logrus.Fields{
			"session": sessionName,
			"output":  strings.TrimSpace(string(output)),
		}).Info("📋 tmux終了コマンドの出力")
	} else {
		logger.WithField("session", sessionName).Info("✅ tmuxセッションが正常に終了しました")
	}

	return nil
}

// getTmuxSessionStatus は既存のtmuxセッションの状態を確認します
func getTmuxSessionStatus(sessionName string) (string, error) {
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)

	// has-sessionコマンドの出力をキャプチャ
	output, err := cmd.CombinedOutput()
	if err != nil {
		// セッションが存在しない場合の詳細出力
		if len(output) > 0 {
			return "", fmt.Errorf("tmuxセッションが存在しません (出力: %s): %w", strings.TrimSpace(string(output)), err)
		}
		return "", fmt.Errorf("tmuxセッションが存在しません: %w", err)
	}

	// セッション存在確認成功（通常は出力なし）
	return sessionName, nil
}

// executeTmuxCommandInSession は既存のtmuxセッション内でコマンドを実行します
func executeTmuxCommandInSession(ctx context.Context, apiClient *api.Client, taskID, taskContent, sessionName string, logger *logrus.Entry) error {
	logger.WithField("session", sessionName).Info("🔄 既存のtmuxセッション内でClaude実行タスクを実行します")

	// Claudeコマンドを構築
	claudeCmd := fmt.Sprintf(`claude -p "%s" --dangerously-skip-permissions`, strings.ReplaceAll(taskContent, `"`, `\"`))

	// コマンド送信前の状態をログ出力
	logger.WithFields(logrus.Fields{
		"session": sessionName,
		"command": claudeCmd,
	}).Info("📤 tmuxセッションにコマンドを送信します")

	// 既存のtmuxセッション内でClaude実行
	sendCmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", sessionName, claudeCmd, "Enter")

	// コマンドの標準出力・標準エラーをキャプチャ
	output, err := sendCmd.CombinedOutput()
	if err != nil {
		logger.WithError(err).WithFields(logrus.Fields{
			"session": sessionName,
			"output":  string(output),
		}).Error("❌ tmuxセッション内でのコマンド実行に失敗")
		return fmt.Errorf("tmuxセッション内でのコマンド実行に失敗しました: %w", err)
	}

	// sendCmdの出力をログ表示
	if len(output) > 0 {
		logger.WithFields(logrus.Fields{
			"session": sessionName,
			"output":  strings.TrimSpace(string(output)),
		}).Info("📋 tmux send-keysコマンドの出力")

		// APIにもログ送信
		logMessage := fmt.Sprintf("[tmux:%s:send-cmd] %s", sessionName, strings.TrimSpace(string(output)))
		if sendErr := apiClient.SendLog(taskID, "INFO", logMessage); sendErr != nil {
			logger.WithError(sendErr).Warning("send-keysログ送信に失敗しました")
		}
	} else {
		logger.WithField("session", sessionName).Info("✅ コマンドが正常に送信されました")

		// コマンド送信成功をAPIにログ送信
		logMessage := fmt.Sprintf("[tmux:%s:send-cmd] コマンドが正常に送信されました: %s", sessionName, claudeCmd)
		if sendErr := apiClient.SendLog(taskID, "INFO", logMessage); sendErr != nil {
			logger.WithError(sendErr).Warning("コマンド送信ログの送信に失敗しました")
		}
	}

	// 出力を監視
	logger.WithField("session", sessionName).Info("👁️ tmux出力監視を開始します")
	go func() {
		ticker := time.NewTicker(1 * time.Second) // より頻繁に監視
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.WithField("session", sessionName).Debug("コンテキストキャンセルによりtmux監視を停止")
				return
			case <-ticker.C:
				if err := captureTmuxOutput(apiClient, taskID, sessionName, logger); err != nil {
					logger.WithError(err).Debug("tmux出力キャプチャに失敗しました")
				}
			}
		}
	}()

	// 少し待機してから最終出力をキャプチャ
	time.Sleep(3 * time.Second)
	if err := captureTmuxOutput(apiClient, taskID, sessionName, logger); err != nil {
		logger.WithError(err).Warning("最終出力キャプチャに失敗しました")
	}

	logger.WithField("session", sessionName).Info("✅ 既存セッション内でのClaude実行タスクが完了しました")
	return nil
}
