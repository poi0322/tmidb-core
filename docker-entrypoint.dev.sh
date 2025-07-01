#!/bin/bash
set -e

# Initialize PostgreSQL if needed
/usr/local/bin/init-postgres.sh

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
echo ""

# The CMD of the Dockerfile will be executed by exec "$@"
# In development, this is typically 'air -c .air.toml', which will handle
# hot-reloading by rebuilding and restarting the supervisor.
echo " HANDING OVER TO CMD"
exec "$@" 