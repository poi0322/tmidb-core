package main

import (
	"testing"
	"time"

	"github.com/tmidb/tmidb-core/internal/ipc"
)

// 테스트를 위한 Mock IPC 서버
type mockIPCServer struct {
	handlers map[ipc.MessageType]ipc.HandlerFunc
}

func newMockIPCServer() *mockIPCServer {
	return &mockIPCServer{
		handlers: make(map[ipc.MessageType]ipc.HandlerFunc),
	}
}

func (m *mockIPCServer) RegisterHandler(msgType ipc.MessageType, handler ipc.HandlerFunc) {
	m.handlers[msgType] = handler
}

// 유틸리티 함수 테스트
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "zero duration",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "seconds only",
			duration: 45 * time.Second,
			expected: "45s",
		},
		{
			name:     "minutes and seconds",
			duration: 2*time.Minute + 30*time.Second,
			expected: "2m30s",
		},
		{
			name:     "hours, minutes and seconds",
			duration: 3*time.Hour + 15*time.Minute + 20*time.Second,
			expected: "3h15m20s",
		},
		{
			name:     "days and hours",
			duration: 48*time.Hour + 30*time.Minute,
			expected: "2d0h30m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %v, want %v", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{
			name:     "zero bytes",
			bytes:    0,
			expected: "0B",
		},
		{
			name:     "bytes",
			bytes:    512,
			expected: "512B",
		},
		{
			name:     "kilobytes",
			bytes:    2048,
			expected: "2.0KB",
		},
		{
			name:     "megabytes",
			bytes:    5 * 1024 * 1024,
			expected: "5.0MB",
		},
		{
			name:     "gigabytes",
			bytes:    10 * 1024 * 1024 * 1024,
			expected: "10.0GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatBytes(%v) = %v, want %v", tt.bytes, result, tt.expected)
			}
		})
	}
}

// ProcessInfo 정렬 테스트를 위한 헬퍼 함수
func createProcessInfo(name string, status string, cpu float64) ipc.ProcessInfo {
	return ipc.ProcessInfo{
		Name:      name,
		Status:    status,
		PID:       12345,
		Uptime:    time.Hour,
		Memory:    1024 * 1024,
		CPU:       cpu,
		Enabled:   true,
		StartTime: time.Now(),
	}
}

// 로그 엔트리 생성 헬퍼 함수
func createLogEntry(process, level, message string) ipc.LogEntry {
	return ipc.LogEntry{
		Process:   process,
		Level:     level,
		Message:   message,
		Timestamp: time.Now(),
	}
}
