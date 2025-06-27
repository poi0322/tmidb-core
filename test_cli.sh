#!/bin/bash

# tmiDB CLI 테스트 스크립트

set -e

echo "🔨 Building tmiDB CLI..."
go build -o ./bin/tmidb-cli ./cmd/cli

echo "✅ Build complete!"
echo ""

# CLI 경로 설정
CLI="./bin/tmidb-cli"

echo "📋 Testing CLI commands..."
echo "=========================="

# 테스트 함수
test_command() {
    local description="$1"
    local command="$2"
    
    echo ""
    echo "🧪 Test: $description"
    echo "📝 Command: $command"
    echo "---"
    
    # 명령 실행 (오류가 있어도 계속)
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

# Supervisor가 실행 중이 아닐 때의 동작 테스트
echo "🔍 Testing without Supervisor running..."
test_command "Status command (no supervisor)" "$CLI status"

# Supervisor 시작
echo ""
echo "🚀 Starting Supervisor for testing..."
./bin/tmidb-supervisor &
SUPERVISOR_PID=$!

# Supervisor가 시작될 시간을 줌
sleep 3

# 정리 함수
cleanup() {
    echo ""
    echo "🧹 Cleaning up..."
    kill $SUPERVISOR_PID 2>/dev/null || true
    wait $SUPERVISOR_PID 2>/dev/null || true
    rm -f /tmp/tmidb-supervisor.sock
    echo "✅ Cleanup complete"
}

# 스크립트 종료 시 정리
trap cleanup EXIT

# 기본 명령어 테스트
test_command "Status command" "$CLI status"
test_command "Process list" "$CLI process list"
test_command "Log status" "$CLI logs status"

# 로그 관련 명령어 테스트
test_command "Enable logs for API" "$CLI logs enable api"
test_command "Disable logs for data-manager" "$CLI logs disable data-manager"
test_command "Check log status again" "$CLI logs status"
test_command "Show recent logs" "$CLI logs"
test_command "Show API logs" "$CLI logs api"

# 프로세스 관련 명령어 테스트
test_command "Process status for API" "$CLI process status api"
test_command "Stop API process" "$CLI process stop api"
test_command "Start API process" "$CLI process start api"
test_command "Restart data-manager" "$CLI process restart data-manager"

# 모니터링 명령어 테스트
test_command "System health check" "$CLI monitor health"
test_command "Service health status" "$CLI monitor services"

# 잘못된 명령어 테스트
test_command "Invalid component name" "$CLI process status invalid-component"
test_command "Invalid command" "$CLI invalid-command"

echo ""
echo "📊 Test Summary"
echo "==============="
echo "All tests completed. Please review the output above for any failures." 