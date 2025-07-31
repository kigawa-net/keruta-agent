package commands

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"keruta-agent/pkg/health"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCommand(t *testing.T) {
	// Reset global variables
	healthCheckType = ""
	healthFormat = "text"

	assert.NotNil(t, healthCmd)
	assert.Equal(t, "health", healthCmd.Use)
	assert.Equal(t, "ヘルスチェックを実行", healthCmd.Short)
	assert.Contains(t, healthCmd.Long, "システムのヘルスチェックを実行します")
}

func TestHealthCommandFlags(t *testing.T) {
	// Test flag definitions
	checkFlag := healthCmd.Flag("check")
	assert.NotNil(t, checkFlag)
	assert.Equal(t, "", checkFlag.DefValue)

	formatFlag := healthCmd.Flag("format")
	assert.NotNil(t, formatFlag)
	assert.Equal(t, "text", formatFlag.DefValue)
}

func TestOutputHealthStatusText(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	status := &health.HealthStatus{
		Overall:   true,
		Timestamp: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		Checks: map[string]health.CheckResult{
			"api": {
				Status:  true,
				Message: "API connection OK",
				Error:   "",
			},
			"disk": {
				Status:  false,
				Message: "Low disk space",
				Error:   "disk usage over 90%",
			},
		},
	}

	err := outputHealthStatusText(status)
	require.NoError(t, err)

	// Close writer and read output
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = oldStdout

	output := buf.String()
	assert.Contains(t, output, "ヘルスチェック結果")
	assert.Contains(t, output, "2023-01-01 12:00:00")
	assert.Contains(t, output, "全体ステータス: OK")
	assert.Contains(t, output, "[OK] api: API connection OK")
	assert.Contains(t, output, "[NG] disk: Low disk space")
	assert.Contains(t, output, "エラー: disk usage over 90%")
}

func TestOutputHealthStatusJSON(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	status := &health.HealthStatus{
		Overall:   false,
		Timestamp: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		Checks: map[string]health.CheckResult{
			"memory": {
				Status:  false,
				Message: "High memory usage",
				Error:   "memory usage over 80%",
			},
		},
	}

	err := outputHealthStatusJSON(status)
	require.NoError(t, err)

	// Close writer and read output
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = oldStdout

	output := buf.String()
	assert.Contains(t, output, `"overall": false`)
	assert.Contains(t, output, `"memory"`)
	assert.Contains(t, output, `"status": false`)
	assert.Contains(t, output, `"message": "High memory usage"`)
	assert.Contains(t, output, `"error": "memory usage over 80%"`)
}

func TestOutputHealthStatus(t *testing.T) {
	status := &health.HealthStatus{
		Overall:   true,
		Timestamp: time.Now(),
		Checks: map[string]health.CheckResult{
			"test": {Status: true, Message: "Test OK", Error: ""},
		},
	}

	tests := []struct {
		name    string
		format  string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "text format",
			format:  "text",
			wantErr: false,
		},
		{
			name:    "json format",
			format:  "json",
			wantErr: false,
		},
		{
			name:    "invalid format",
			format:  "xml",
			wantErr: true,
			errMsg:  "無効な出力形式です: xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout to avoid polluting test output
			oldStdout := os.Stdout
			_, w, _ := os.Pipe()
			os.Stdout = w

			healthFormat = tt.format
			err := outputHealthStatus(status)

			w.Close()
			os.Stdout = oldStdout

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHealthCommandExecution(t *testing.T) {
	// This test verifies the command can be created and flags work
	// We can't easily test runHealth() without mocking the health package

	// Test flag parsing
	healthCmd.SetArgs([]string{"--check", "api", "--format", "json"})
	err := healthCmd.ParseFlags([]string{"--check", "api", "--format", "json"})
	assert.NoError(t, err)

	// Verify flags were parsed correctly
	checkFlag := healthCmd.Flag("check")
	assert.Equal(t, "api", checkFlag.Value.String())

	formatFlag := healthCmd.Flag("format")
	assert.Equal(t, "json", formatFlag.Value.String())
}

func TestHealthOutputFormatValidation(t *testing.T) {
	status := &health.HealthStatus{
		Overall:   true,
		Timestamp: time.Now(),
		Checks:    make(map[string]health.CheckResult),
	}

	// Test with invalid format
	healthFormat = "invalid"
	err := outputHealthStatus(status)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "無効な出力形式です: invalid")

	// Reset to default
	healthFormat = "text"
}
