#!/bin/bash

# API 서버 빌드 스크립트
echo "🔨 Building tmiDB API Server..."

# 스크립트가 실행되는 디렉터리 확인
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

cd "$SCRIPT_DIR"

# bin 디렉터리가 없으면 생성
mkdir -p ../../bin

go build -o ../../bin/api .

if [ $? -eq 0 ]; then
    echo "✅ API 서버 빌드 완료: bin/api"
else
    echo "❌ API 서버 빌드 실패"
    exit 1
fi