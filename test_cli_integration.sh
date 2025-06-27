#!/bin/bash

# tmiDB CLI 통합 테스트 스크립트

set -e

echo "🔨 Building tmiDB CLI..."
go build -o ./bin/tmidb-cli ./cmd/cli

echo "✅ CLI Build complete!"
echo ""

# Mock Supervisor를 백그라운드에서 실행하기 위한 Go 프로그램 생성
cat > mock_supervisor_runner.go << 'EOF'
// +build ignore

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	
	"github.com/tmidb/tmidb-core/internal/ipc"
)

// MockSupervisor 구조체와 메서드들을 여기에 복사
// (실제 구현에서는 import하거나 별도 패키지로 만들어야 함)

func main() {
	socketPath := "/tmp/tmidb-test-supervisor.sock"
	
	// 기존 소켓 파일 제거
	os.Remove(socketPath)
	
	// Mock Supervisor 시작
	server := ipc.NewServer(socketPath)
	
	// 간단한 핸들러 등록
	server.RegisterHandler(ipc.MessageTypeSystemHealth, func(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
		return ipc.NewResponse(msg.ID, true, map[string]interface{}{
			"status": "healthy",
			"uptime": "2h30m",
		}, "")
	})
	
	server.RegisterHandler(ipc.MessageTypeProcessList, func(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
		processes := []map[string]interface{}{
			{
				"name":      "api",
				"status":    "running",
				"pid":       12345,
				"uptime":    7200000000000, // 2 hours in nanoseconds
				"memory":    52428800,       // 50MB
				"cpu":       12.5,
				"enabled":   true,
				"start_time": time.Now().Add(-2 * time.Hour),
			},
			{
				"name":      "data-manager",
				"status":    "running",
				"pid":       12346,
				"uptime":    7200000000000,
				"memory":    31457280, // 30MB
				"cpu":       8.3,
				"enabled":   true,
				"start_time": time.Now().Add(-2 * time.Hour),
			},
		}
		return ipc.NewResponse(msg.ID, true, processes, "")
	})
	
	server.RegisterHandler(ipc.MessageTypeLogStatus, func(conn *ipc.Connection, msg *ipc.Message) *ipc.Response {
		status := map[string]interface{}{
			"api":           true,
			"data-manager":  true,
			"data-consumer": false,
			"postgresql":    true,
			"nats":          true,
			"seaweedfs":     true,
		}
		return ipc.NewResponse(msg.ID, true, status, "")
	})
	
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	
	fmt.Println("✅ Mock supervisor started on", socketPath)
	
	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	server.Stop()
	fmt.Println("✅ Mock supervisor stopped")
}
EOF

# Mock Supervisor 빌드 및 실행
echo "🔨 Building mock supervisor..."
go build -o ./bin/mock-supervisor mock_supervisor_runner.go

echo "🚀 Starting mock supervisor..."
./bin/mock-supervisor &
MOCK_PID=$!

# Mock Supervisor가 시작될 시간을 줌
sleep 2

# 정리 함수
cleanup() {
    echo ""
    echo "🧹 Cleaning up..."
    kill $MOCK_PID 2>/dev/null || true
    wait $MOCK_PID 2>/dev/null || true
    rm -f /tmp/tmidb-test-supervisor.sock
    rm -f mock_supervisor_runner.go
    rm -f ./bin/mock-supervisor
    echo "✅ Cleanup complete"
}

# 스크립트 종료 시 정리
trap cleanup EXIT

# CLI 명령어 테스트
export TMIDB_SOCKET_PATH="/tmp/tmidb-test-supervisor.sock"

echo ""
echo "📋 Running CLI tests..."
echo "======================"

# 테스트 함수
test_command() {
    local description="$1"
    local command="$2"
    
    echo ""
    echo "🧪 Test: $description"
    echo "📝 Command: $command"
    echo "---"
    
    # 명령 실행
    set +e
    eval "$command"
    local result=$?
    set -e
    
    if [ $result -eq 0 ]; then
        echo "✅ Success"
    else
        echo "❌ Failed with exit code: $result"
    fi
    echo "---"
}

# 기본 명령어 테스트
test_command "Status command" "./bin/tmidb-cli status"
test_command "Process list" "./bin/tmidb-cli process list"
test_command "Log status" "./bin/tmidb-cli logs status"
test_command "Health check" "./bin/tmidb-cli monitor health"

echo ""
echo "✅ All tests completed!" 