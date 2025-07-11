#!/bin/bash
# tmiDB 환경변수 설정 스크립트

set -e

echo "🌍 Setting up tmiDB environment variables..."

# Go 환경변수 설정
export GOROOT=/usr/local/go
export GOPATH=/go
export GOPROXY=https://proxy.golang.org,direct
export GOSUMDB=sum.golang.org
export CGO_ENABLED=1

# tmiDB 환경변수 설정
export TMIDB_HOME=/app
export TMIDB_BIN_DIR=/app/bin
export TMIDB_TMP_DIR=/app/bin
export TMIDB_DATA_DIR=/data
export TMIDB_LOG_DIR=/app/logs

# PATH 설정 (우선순위: tmp -> bin -> go -> system)
export PATH="/app/bin:/app/bin:/usr/local/go/bin:/go/bin:${PATH}"

# 개발 환경 설정
if [ "${TMIDB_DEV_MODE}" = "true" ]; then
    export TMIDB_LOG_LEVEL=debug
    export TMIDB_HOT_RELOAD=true
    echo "🔧 Development mode enabled"
fi

echo "✅ Environment setup complete"
echo "📍 GOROOT: ${GOROOT}"
echo "📍 GOPATH: ${GOPATH}"
echo "📍 PATH: ${PATH}"
echo "📍 tmiDB Home: ${TMIDB_HOME}" 