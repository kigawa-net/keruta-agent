package commands

import (
	"os"
	"strings"

	"keruta-agent/internal/api"

	"github.com/sirupsen/logrus"
)

// getWorkspaceName はCoderワークスペース名を取得します
func getWorkspaceName() string {
	// Coder環境変数から取得（最も一般的）
	if workspaceName := os.Getenv("CODER_WORKSPACE_NAME"); workspaceName != "" {
		return workspaceName
	}

	// ホスト名から取得（Coderワークスペース内では一般的にワークスペース名がホスト名になる）
	if hostname, err := os.Hostname(); err == nil && hostname != "" && hostname != "localhost" {
		return hostname
	}

	// PWDの最後のディレクトリ名から推測
	if pwd := os.Getenv("PWD"); pwd != "" {
		parts := strings.Split(pwd, "/")
		if len(parts) > 0 {
			lastDir := parts[len(parts)-1]
			if lastDir != "" && strings.HasPrefix(lastDir, "session-") {
				return lastDir
			}
		}
	}

	return ""
}

// extractSessionIDFromWorkspaceName はワークスペース名からセッションIDを抽出します
func extractSessionIDFromWorkspaceName(workspaceName string) string {
	// パターン1: ws-{sessionId8}-{name10}-{time4} の形式（新規則、最優先）
	// 例: ws-0fcfba18-session0fc-7973 または ws-0fcfba18-ws0fcfba18-7973
	if strings.HasPrefix(workspaceName, "ws-") {
		// "ws-" を除去
		remaining := workspaceName[3:]
		parts := strings.Split(remaining, "-")

		// 最低3つの部分が必要: {sessionId8}-{name10}-{time4}
		if len(parts) >= 3 {
			sessionIdPart := parts[0]
			// セッションIDは8文字の英数字
			if len(sessionIdPart) == 8 && isAlphaNumeric(sessionIdPart) {
				return sessionIdPart
			}
		}

		// 特別なケース: ws-{sessionId8}-ws{sessionId8}-{time4} の形式
		// 例: ws-0fcfba18-ws0fcfba18-7973
		if len(parts) >= 3 {
			sessionIdPart := parts[0]
			secondPart := parts[1]
			// 第2部分が "ws" + セッションID の形式かチェック
			if len(sessionIdPart) == 8 && isAlphaNumeric(sessionIdPart) &&
				strings.HasPrefix(secondPart, "ws") && len(secondPart) == 10 &&
				secondPart[2:] == sessionIdPart {
				return sessionIdPart
			}
		}
	}

	// パターン2: session-{full-uuid}-{suffix} の形式（旧規則、後方互換性）
	// 例: session-29229ea1-8c41-4ca2-b064-7a7a7672dd1a-keruta
	if strings.HasPrefix(workspaceName, "session-") {
		// "session-" を除去
		remaining := workspaceName[8:]

		// UUID形式のパターンを探す (8-4-4-4-12の形式)
		if uuid := extractUUIDPattern(remaining); uuid != "" {
			return uuid
		}

		// フォールバック: 最初の部分だけを取得（後方互換性のため）
		parts := strings.Split(remaining, "-")
		if len(parts) >= 1 {
			sessionID := parts[0]
			if len(sessionID) >= 8 {
				return sessionID
			}
		}
	}

	// パターン3: {full-uuid}-{suffix} の形式
	if uuid := extractUUIDPattern(workspaceName); uuid != "" {
		return uuid
	}

	// パターン4: 完全なUUID形式（ハイフンを含む）
	if len(workspaceName) >= 32 && strings.Contains(workspaceName, "-") {
		if isValidUUIDFormat(workspaceName) {
			return workspaceName
		}
	}

	// パターン5: {sessionId}-{suffix} の形式（UUIDの最初の部分のみ - 後方互換性）
	parts := strings.Split(workspaceName, "-")
	if len(parts) >= 2 {
		possibleID := parts[0]
		// UUIDの最初の部分らしき文字列（8文字以上の英数字）
		if len(possibleID) >= 8 && isAlphaNumeric(possibleID) {
			return possibleID
		}
	}

	return ""
}

// resolveFullSessionID は部分的なセッションIDまたはワークスペース名から完全なUUIDを取得します
func resolveFullSessionID(apiClient *api.Client, partialID string, logger *logrus.Entry) string {
	// 既に完全なUUID形式の場合はそのまま返す
	if isValidUUIDFormat(partialID) {
		return partialID
	}

	// 部分的なIDが短すぎる場合はそのまま返す
	if len(partialID) < 4 {
		logger.WithField("partialId", partialID).Debug("部分的なIDが短すぎるため、APIで検索をスキップします")
		return partialID
	}

	logger.WithField("partialId", partialID).Info("部分的なセッションIDから完全なUUIDを検索しています...")

	// まず、ワークスペース名による完全一致検索を試す
	// ワークスペース名が "session-{uuid}-{suffix}" の形式の場合、ワークスペース名全体で検索
	if strings.HasPrefix(partialID, "session-") || len(partialID) > 20 {
		workspaceName := getWorkspaceName()
		if workspaceName != "" && workspaceName != partialID {
			logger.WithField("workspaceName", workspaceName).Info("ワークスペース名による完全一致検索を試行中...")
			if session, err := apiClient.SearchSessionByName(workspaceName); err == nil {
				logger.WithFields(logrus.Fields{
					"workspaceName": workspaceName,
					"sessionId":     session.ID,
					"sessionName":   session.Name,
				}).Info("ワークスペース名による完全一致でセッションを発見しました")
				return session.ID
			}
		}
	}

	// 部分的なIDから完全なUUIDを検索
	session, err := apiClient.SearchSessionByPartialID(partialID)
	if err != nil {
		logger.WithError(err).WithField("partialId", partialID).Warning("部分的なIDでの検索に失敗しました。元のIDを使用します")
		return partialID
	}

	logger.WithFields(logrus.Fields{
		"partialId":   partialID,
		"fullId":      session.ID,
		"sessionName": session.Name,
	}).Info("完全なセッションUUIDを取得しました")

	return session.ID
}

// extractUUIDPattern はUUID形式のパターンを抽出します
func extractUUIDPattern(text string) string {
	// UUID形式: 8-4-4-4-12 (例: 29229ea1-8c41-4ca2-b064-7a7a7672dd1a)
	parts := strings.Split(text, "-")
	if len(parts) >= 5 {
		// 最初の5つの部分がUUID形式かチェック
		if len(parts[0]) == 8 && len(parts[1]) == 4 && len(parts[2]) == 4 &&
			len(parts[3]) == 4 && len(parts[4]) == 12 {
			// 各部分が16進数かチェック
			uuid := strings.Join(parts[0:5], "-")
			if isValidUUIDFormat(uuid) {
				return uuid
			}
		}
	}
	return ""
}

// isValidUUIDFormat はUUID形式として有効かをチェックします
func isValidUUIDFormat(uuid string) bool {
	// 基本的な長さチェック (36文字: 32文字 + 4つのハイフン)
	if len(uuid) != 36 {
		return false
	}

	// ハイフンの位置チェック
	if uuid[8] != '-' || uuid[13] != '-' || uuid[18] != '-' || uuid[23] != '-' {
		return false
	}

	// 各部分が16進数かチェック
	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		return false
	}

	for _, part := range parts {
		for _, r := range part {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}

	return true
}

// isAlphaNumeric は文字列が英数字のみかチェックします
func isAlphaNumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
