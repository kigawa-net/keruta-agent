package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"keruta-agent/internal/logger"
)

// WebSocketMessage はWebSocketメッセージを表します
type WebSocketMessage struct {
	Type    string      `json:"type"`
	TaskID  string      `json:"taskId"`
	Status  TaskStatus  `json:"status,omitempty"`
	Message string      `json:"message,omitempty"`
	Input   string      `json:"input,omitempty"`
	Log     *LogMessage `json:"log,omitempty"`
}

// LogMessage はログメッセージを表します
type LogMessage struct {
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Timestamp string                 `json:"timestamp,omitempty"`
	Source    string                 `json:"source,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// WebSocketClient はWebSocketクライアントを表します
type WebSocketClient struct {
	conn           *websocket.Conn
	url            string
	token          string
	taskID         string
	sendChan       chan *WebSocketMessage
	receiveChan    chan *WebSocketMessage
	done           chan struct{}
	reconnectMutex sync.Mutex
	isConnected    bool
	maxRetries     int
	retryInterval  time.Duration
}

// NewWebSocketClient は新しいWebSocketクライアントを作成します
func NewWebSocketClient(baseURL, token, taskID string) *WebSocketClient {
	// デフォルトのWebSocket URL
	wsURL := fmt.Sprintf("ws://%s/ws/agent?token=%s", baseURL, token)

	// URLの形式を確認して適切に処理
	if strings.HasPrefix(baseURL, "http://") || strings.HasPrefix(baseURL, "https://") {
		// URLをパース
		u, err := url.Parse(baseURL)
		if err == nil {
			// u.Host にはホスト名とポート番号（指定されている場合）が含まれる
			host := u.Host
			if u.Scheme == "https" {
				wsURL = fmt.Sprintf("wss://%s/ws/agent?token=%s", host, token)
			} else {
				wsURL = fmt.Sprintf("ws://%s/ws/agent?token=%s", host, token)
			}
		} else {
			logger.WithTaskIDAndComponent("websocket").WithError(err).
				Error("WebSocket URL の作成に失敗しました。baseURL のパースに失敗しました")
		}
	} else if strings.Contains(baseURL, ":") {
		// ホスト:ポートの形式と仮定
		wsURL = fmt.Sprintf("ws://%s/ws/agent?token=%s", baseURL, token)
	} else {
		// ホストのみの形式と仮定
		wsURL = fmt.Sprintf("ws://%s/ws/agent?token=%s", baseURL, token)
	}

	return &WebSocketClient{
		url:           wsURL,
		token:         token,
		taskID:        taskID,
		sendChan:      make(chan *WebSocketMessage, 100),
		receiveChan:   make(chan *WebSocketMessage, 100),
		done:          make(chan struct{}),
		maxRetries:    5,
		retryInterval: 5 * time.Second,
	}
}

// Connect はWebSocketサーバーに接続します
func (wsc *WebSocketClient) Connect() error {
	wsc.reconnectMutex.Lock()
	defer wsc.reconnectMutex.Unlock()

	if wsc.isConnected {
		return nil
	}

	logger.WithTaskIDAndComponent("websocket").Info("WebSocketサーバーに接続中...")

	dialer := websocket.DefaultDialer

	// ヘッダーの設定
	header := http.Header{}
	header.Add("Authorization", "Bearer "+wsc.token)

	// 接続の試行
	logger.WithTaskIDAndComponent("websocket").WithField("url", wsc.url).Debug("WebSocket接続を試行中...")
	conn, resp, err := dialer.Dial(wsc.url, header)

	if err != nil {
		// レスポンスがある場合はエラー詳細をログに記録
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			logger.WithTaskIDAndComponent("websocket").WithFields(logrus.Fields{
				"statusCode": resp.StatusCode,
				"response":   string(body),
				"url":        wsc.url,
			}).Error("WebSocket接続に失敗しました")
			resp.Body.Close()
		}
		return fmt.Errorf("WebSocket接続に失敗: %w", err)
	}

	wsc.conn = conn
	wsc.isConnected = true

	// 送信ゴルーチン
	go wsc.sendLoop()

	// 受信ゴルーチン
	go wsc.receiveLoop()

	logger.WithTaskIDAndComponent("websocket").Info("WebSocketサーバーに接続しました")
	return nil
}

// Close はWebSocket接続を閉じます
func (wsc *WebSocketClient) Close() {
	close(wsc.done)
	if wsc.conn != nil {
		wsc.conn.Close()
	}
	wsc.isConnected = false
}

// SendMessage はメッセージを送信します
func (wsc *WebSocketClient) SendMessage(msg *WebSocketMessage) {
	select {
	case wsc.sendChan <- msg:
		// メッセージをキューに追加
	default:
		logger.WithTaskIDAndComponent("websocket").Warn("送信キューがいっぱいです")
	}
}

// ReceiveMessage はメッセージを受信します
func (wsc *WebSocketClient) ReceiveMessage() (*WebSocketMessage, error) {
	select {
	case msg := <-wsc.receiveChan:
		return msg, nil
	case <-time.After(1 * time.Second):
		return nil, fmt.Errorf("メッセージ受信タイムアウト")
	}
}

// UpdateTaskStatus はタスクステータスをWebSocketで更新します
func (wsc *WebSocketClient) UpdateTaskStatus(status TaskStatus, message string) {
	msg := &WebSocketMessage{
		Type:    "status",
		TaskID:  wsc.taskID,
		Status:  status,
		Message: message,
	}
	wsc.SendMessage(msg)
}

// SendLog はログをWebSocketで送信します
func (wsc *WebSocketClient) SendLog(level, message string) {
	logMsg := &LogMessage{
		Level:     level,
		Message:   message,
		Timestamp: time.Now().Format(time.RFC3339),
		Source:    "agent",
	}

	msg := &WebSocketMessage{
		Type:   "log",
		TaskID: wsc.taskID,
		Log:    logMsg,
	}
	wsc.SendMessage(msg)
}

// WaitForInput は入力待ち状態を通知し、入力を待機します
func (wsc *WebSocketClient) WaitForInput(prompt string) (string, error) {
	// 入力待ち状態を通知
	wsc.UpdateTaskStatus(TaskStatusWaitingForInput, prompt)

	// 入力を待機
	for {
		msg, err := wsc.ReceiveMessage()
		if err != nil {
			return "", err
		}

		if msg.Type == "input" && msg.TaskID == wsc.taskID {
			return msg.Input, nil
		}
	}
}

// sendLoop はメッセージ送信ループを実行します
func (wsc *WebSocketClient) sendLoop() {
	for {
		select {
		case <-wsc.done:
			return
		case msg := <-wsc.sendChan:
			if wsc.conn == nil {
				logger.WithTaskIDAndComponent("websocket").Error("WebSocket接続がありません")
				continue
			}

			err := wsc.conn.WriteJSON(msg)
			if err != nil {
				logger.WithTaskIDAndComponent("websocket").WithError(err).Error("メッセージ送信に失敗しました")
				wsc.reconnect()
			}
		}
	}
}

// receiveLoop はメッセージ受信ループを実行します
func (wsc *WebSocketClient) receiveLoop() {
	for {
		select {
		case <-wsc.done:
			return
		default:
			if wsc.conn == nil {
				time.Sleep(wsc.retryInterval)
				continue
			}

			var msg WebSocketMessage
			err := wsc.conn.ReadJSON(&msg)
			if err != nil {
				logger.WithTaskIDAndComponent("websocket").WithError(err).Error("メッセージ受信に失敗しました")
				wsc.reconnect()
				continue
			}

			// 受信したメッセージをキューに追加
			select {
			case wsc.receiveChan <- &msg:
				// メッセージをキューに追加
			default:
				logger.WithTaskIDAndComponent("websocket").Warn("受信キューがいっぱいです")
			}

			// pingメッセージに対してpongで応答
			if msg.Type == "ping" {
				wsc.SendMessage(&WebSocketMessage{
					Type:   "pong",
					TaskID: wsc.taskID,
				})
			}
		}
	}
}

// reconnect はWebSocket接続を再接続します
func (wsc *WebSocketClient) reconnect() {
	wsc.reconnectMutex.Lock()
	defer wsc.reconnectMutex.Unlock()

	if wsc.conn != nil {
		wsc.conn.Close()
		wsc.conn = nil
	}
	wsc.isConnected = false

	for i := 0; i < wsc.maxRetries; i++ {
		logger.WithTaskIDAndComponent("websocket").Infof("WebSocket再接続を試みています (%d/%d)...", i+1, wsc.maxRetries)

		dialer := websocket.DefaultDialer

		// ヘッダーの設定
		header := http.Header{}
		header.Add("Authorization", "Bearer "+wsc.token)

		// 接続の試行
		logger.WithTaskIDAndComponent("websocket").WithField("url", wsc.url).Debug("WebSocket再接続を試行中...")
		conn, resp, err := dialer.Dial(wsc.url, header)

		if err == nil {
			wsc.conn = conn
			wsc.isConnected = true
			logger.WithTaskIDAndComponent("websocket").Info("WebSocket再接続に成功しました")
			return
		}

		// レスポンスがある場合はエラー詳細をログに記録
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			logger.WithTaskIDAndComponent("websocket").WithFields(logrus.Fields{
				"statusCode": resp.StatusCode,
				"response":   string(body),
				"url":        wsc.url,
			}).Error("WebSocket再接続に失敗しました")
			resp.Body.Close()
		}

		logger.WithTaskIDAndComponent("websocket").WithError(err).Error("WebSocket再接続に失敗しました")
		time.Sleep(wsc.retryInterval)
	}

	logger.WithTaskIDAndComponent("websocket").Error("WebSocket再接続を諦めました")
}
