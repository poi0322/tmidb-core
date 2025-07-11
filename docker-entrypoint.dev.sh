#!/bin/bash
set -e

echo "🔧 tmiDB Development Environment Starting..."

# 환경변수 설정
echo "🌍 Setting up environment variables..."
source /app/scripts/setup-env.sh save

# PATH 업데이트 및 심볼릭 링크 설정
echo "🔗 Setting up PATH and CLI tools..."
export PATH="/app/bin:/app/bin:/usr/local/go/bin:/go/bin:$PATH"

# CLI 도구들의 심볼릭 링크 생성 (개발 환경에서는 /app/bin 우선)
if [ -f "/app/bin/tmidb-cli" ]; then
    ln -sf /app/bin/tmidb-cli /usr/local/bin/tmidb-cli
    echo "✅ tmidb-cli linked from /app/bin"
elif [ -f "/app/bin/tmidb-cli" ]; then
    ln -sf /app/bin/tmidb-cli /usr/local/bin/tmidb-cli
    echo "✅ tmidb-cli linked from /app/bin"
fi

# Go가 PATH에 있는지 확인
if ! command -v go >/dev/null 2>&1; then
    echo "⚠️ Go not found in PATH, creating symlink..."
    ln -sf /usr/local/go/bin/go /usr/local/bin/go
fi

echo "📍 Current PATH: $PATH"
echo "🔍 Go location: $(which go 2>/dev/null || echo 'not found')"
echo "🔍 CLI location: $(which tmidb-cli 2>/dev/null || echo 'not found')"

# Go 모듈 정리 (개발 환경에서 변경사항 반영)
echo "📦 Updating Go modules..."
cd /app && go mod tidy

# Initialize PostgreSQL if needed
echo "🗄️ Initializing PostgreSQL..."
/usr/local/bin/init-postgres.sh

# --- Start: Ensure PostgreSQL external access for development ---
echo "🔧 Configuring PostgreSQL for external access..."
PG_CONF="/data/postgresql/postgresql.conf"
PG_HBA_CONF="/data/postgresql/pg_hba.conf"

# Check and set listen_addresses
if ! grep -q "^listen_addresses\s*=\s*'\*'" "$PG_CONF"; then
    echo "listen_addresses = '*'" >> "$PG_CONF"
    echo "   - Enabled listening on all addresses"
else
    echo "   - 'listen_addresses' already configured"
fi

# Check and set host-based authentication
if ! grep -q "host    all             all             0.0.0.0/0               scram-sha-256" "$PG_HBA_CONF"; then
    echo "host    all             all             0.0.0.0/0               scram-sha-256" >> "$PG_HBA_CONF"
    echo "   - Added rule to allow all remote connections"
else
    echo "   - Host-based authentication already configured"
fi
# --- End: Ensure PostgreSQL external access for development ---

# Start PostgreSQL in background
echo "🚀 Starting PostgreSQL..."
runuser -u postgres -- postgres -D /data/postgresql -k /var/run/postgresql &
POSTGRES_PID=$!
echo "PostgreSQL started with PID $POSTGRES_PID"

# Start NATS in background
echo "🚀 Starting NATS..."
runuser -u natsuser -- nats-server -sd /data/nats &
NATS_PID=$!
echo "NATS started with PID $NATS_PID"

# Start SeaweedFS in background
echo "🚀 Starting SeaweedFS..."
runuser -u seaweeduser -- weed master -mdir=/data/seaweedfs/master &
SEAWEED_PID=$!
echo "SeaweedFS started with PID $SEAWEED_PID"

# Save PIDs for supervisor to attach
echo "$POSTGRES_PID" > /var/run/postgresql.pid
echo "$NATS_PID" > /var/run/nats.pid
echo "$SEAWEED_PID" > /var/run/seaweedfs.pid

# Wait a moment for services to start
sleep 3
echo "✅ All external services started successfully"

# --- Start: Pre-create extensions as superuser ---
echo "🔧 Pre-creating extensions as superuser..."
if PGPASSWORD=postgres psql -U postgres -h localhost -lqt | cut -d \| -f 1 | grep -qw _core_tmidb; then
    echo "   - Database '_core_tmidb' exists. Creating extensions..."
    PGPASSWORD=postgres psql -U postgres -h localhost -d _core_tmidb -c "CREATE EXTENSION IF NOT EXISTS pg_uuidv7;"
    echo "   - pg_uuidv7 extension ensured."
else
    echo "   - Database '_core_tmidb' does not exist yet. Skipping extension creation."
fi
# --- End: Pre-create extensions as superuser ---

# 초기 빌드 (최신 변경사항 반영)
echo "🔨 Building components..."
/app/scripts/build.sh all

echo ""
echo "🔥 Starting Air for hot-reloading..."
echo "   - Air will monitor code changes and rebuild supervisor automatically"
echo "   - Supervisor will manage all internal services (API, data-manager, data-consumer)"
echo "   - Use 'docker compose logs -f' to watch logs"
echo ""

# The CMD of the Dockerfile will be executed by exec "$@"
# In development, this is 'air -c .air.toml', which will handle
# hot-reloading by rebuilding and restarting the supervisor.
exec "$@" 