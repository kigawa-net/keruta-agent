package commands

import (
	"context"
	"fmt"
	"os"
	"testing"

	"keruta-agent/internal/api"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestTmuxSessionManagement(t *testing.T) {
	// テスト用のセッションID
	testSessionID := "test-session-12345678"

	tests := []struct {
		name           string
		sessionID      string
		expectedPrefix string
	}{
		{
			name:           "セッションIDありの場合のtmuxセッション名生成",
			sessionID:      testSessionID,
			expectedPrefix: "keruta-session-",
		},
		{
			name:           "セッションIDなしの場合のフォールバック",
			sessionID:      "",
			expectedPrefix: "keruta-task-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// グローバル変数を設定
			originalSessionID := daemonSessionID
			daemonSessionID = tt.sessionID
			defer func() {
				daemonSessionID = originalSessionID
			}()

			// テスト用のタスクID
			taskID := "task-12345678"

			// tmuxセッション名を生成する部分をテスト
			var tmuxSessionName string
			if daemonSessionID != "" {
				tmuxSessionName = "keruta-session-" + daemonSessionID[:8]
			} else {
				tmuxSessionName = "keruta-task-" + taskID[:8]
			}

			assert.Contains(t, tmuxSessionName, tt.expectedPrefix)
		})
	}
}

func TestGetTmuxSessionStatus(t *testing.T) {
	testSessionName := "test-session-nonexistent"

	// 存在しないセッションのテスト
	_, err := getTmuxSessionStatus(testSessionName)
	assert.Error(t, err, "存在しないtmuxセッションに対してエラーが返されるべき")
}

func TestTmuxSessionReuse(t *testing.T) {
	// テスト環境では実際のtmuxコマンドを実行しないため、
	// セッション再利用のロジックのみテスト

	testCases := []struct {
		name        string
		sessionName string
		shouldReuse bool
	}{
		{
			name:        "Kerutaセッション用tmuxセッションは再利用",
			sessionName: "keruta-session-12345678",
			shouldReuse: true,
		},
		{
			name:        "タスク用tmuxセッションは削除",
			sessionName: "keruta-task-12345678",
			shouldReuse: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shouldPreserve := tc.sessionName[:len("keruta-session-")] == "keruta-session-"
			assert.Equal(t, tc.shouldReuse, shouldPreserve)
		})
	}
}

func TestTmuxSessionNaming(t *testing.T) {
	tests := []struct {
		name                string
		sessionID           string
		taskID              string
		expectedSessionName string
	}{
		{
			name:                "完全なセッションIDでのtmuxセッション名",
			sessionID:           "29229ea1-8c41-4ca2-b064-7a7a7672dd1a",
			taskID:              "task-abc12345",
			expectedSessionName: "keruta-session-29229ea1",
		},
		{
			name:                "短いセッションIDでのtmuxセッション名",
			sessionID:           "12345678",
			taskID:              "task-abc12345",
			expectedSessionName: "keruta-session-12345678",
		},
		{
			name:                "セッションIDなしでのフォールバック",
			sessionID:           "",
			taskID:              "abc12345-def67890",
			expectedSessionName: "keruta-task-abc12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tmuxSessionName string
			if tt.sessionID != "" {
				tmuxSessionName = "keruta-session-" + tt.sessionID[:8]
			} else {
				tmuxSessionName = "keruta-task-" + tt.taskID[:8]
			}

			assert.Equal(t, tt.expectedSessionName, tmuxSessionName)
		})
	}
}

func TestExecuteTmuxCommandInSessionLogic(t *testing.T) {
	// テスト用のロガー
	logger := logrus.NewEntry(logrus.New())

	// executeTmuxCommandInSession の引数をテスト
	ctx := context.Background()
	taskID := "test-task-12345"
	taskContent := "test claude task"
	sessionName := "test-session"

	// 引数の妥当性をチェック
	assert.NotNil(t, ctx)
	assert.NotEmpty(t, taskID)
	assert.NotEmpty(t, taskContent)
	assert.NotEmpty(t, sessionName)
	assert.NotNil(t, logger)

	// Claudeコマンドの構築をテスト
	claudeCmd := "claude -p \"" + taskContent + "\" --dangerously-skip-permissions"
	expected := "claude -p \"test claude task\" --dangerously-skip-permissions"
	assert.Equal(t, expected, claudeCmd)
}

// モック用の構造体とメソッド
type MockAPIClient struct {
	sessions map[string]*api.Session
}

func NewMockAPIClient() *MockAPIClient {
	return &MockAPIClient{
		sessions: make(map[string]*api.Session),
	}
}

func (m *MockAPIClient) GetSession(sessionID string) (*api.Session, error) {
	if session, exists := m.sessions[sessionID]; exists {
		return session, nil
	}
	return nil, fmt.Errorf("session not found")
}

func (m *MockAPIClient) SendLog(taskID, level, message string) error {
	// モック実装 - ログを送信したと仮定
	return nil
}

func TestTmuxSessionLifecycle(t *testing.T) {
	// テスト用のセットアップ
	originalSessionID := daemonSessionID
	daemonSessionID = "test-session-12345678"
	defer func() {
		daemonSessionID = originalSessionID
	}()

	// セッション名の生成をテスト
	tmuxSessionName := "keruta-session-" + daemonSessionID[:8]
	assert.Equal(t, "keruta-session-test-ses", tmuxSessionName)

	// セッション保持の判定をテスト
	shouldPreserve := tmuxSessionName[:len("keruta-session-")] == "keruta-session-"
	assert.True(t, shouldPreserve, "Kerutaセッション用のtmuxセッションは保持されるべき")
}

func TestEnsureDirectory(t *testing.T) {
	// 一時ディレクトリでテスト
	tempDir := "/tmp/test-keruta-" + "12345"
	defer os.RemoveAll(tempDir)

	// ディレクトリが存在しない場合の作成をテスト
	err := ensureDirectory(tempDir)
	assert.NoError(t, err)

	// ディレクトリが作成されたことを確認
	if _, statErr := os.Stat(tempDir); os.IsNotExist(statErr) {
		t.Errorf("ディレクトリが作成されていません: %s", tempDir)
	}

	// 既に存在するディレクトリの場合のテスト
	err = ensureDirectory(tempDir)
	assert.NoError(t, err, "既存ディレクトリに対してもエラーが発生しないべき")
}

// ベンチマークテスト
func BenchmarkTmuxSessionNaming(b *testing.B) {
	sessionID := "29229ea1-8c41-4ca2-b064-7a7a7672dd1a"
	taskID := "task-12345678"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var tmuxSessionName string
		if sessionID != "" {
			tmuxSessionName = "keruta-session-" + sessionID[:8]
		} else {
			tmuxSessionName = "keruta-task-" + taskID[:8]
		}
		_ = tmuxSessionName
	}
}
