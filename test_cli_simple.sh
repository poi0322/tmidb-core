#!/bin/bash

# 간단한 tmiDB CLI 테스트 스크립트
# 외부 서비스 없이 기본 기능만 테스트

set -e

echo "🔨 Building tmiDB CLI..."
go build -o ./bin/tmidb-cli ./cmd/cli

echo "✅ CLI Build complete!"
echo ""

# CLI 경로 설정
CLI="./bin/tmidb-cli"

echo "📋 Testing basic CLI functionality..."
echo "===================================="

# 기본 명령어 테스트 (Supervisor 없이)
echo ""
echo "1️⃣ Testing help commands..."
$CLI --help

echo ""
echo "2️⃣ Testing subcommand help..."
$CLI logs --help

echo ""
echo "3️⃣ Testing process subcommand help..."
$CLI process --help

echo ""
echo "4️⃣ Testing monitor subcommand help..."
$CLI monitor --help

echo ""
echo "5️⃣ Testing error handling (no supervisor)..."
set +e
$CLI status
EXIT_CODE=$?
set -e

if [ $EXIT_CODE -eq 1 ]; then
    echo "✅ Correctly handled missing supervisor"
else
    echo "❌ Unexpected exit code: $EXIT_CODE"
fi

echo ""
echo "6️⃣ Testing invalid command..."
set +e
$CLI invalid-command 2>&1
EXIT_CODE=$?
set -e

if [ $EXIT_CODE -eq 1 ]; then
    echo "✅ Correctly handled invalid command"
else
    echo "❌ Unexpected exit code: $EXIT_CODE"
fi

echo ""
echo "✅ Basic CLI tests completed!" 