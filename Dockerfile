# tmidb-core/Dockerfile - All-in-One Production Build
# 이 파일은 Go 애플리케이션과 모든 종속 서비스(PostgreSQL, NATS, SeaweedFS)를
# 포함하는 단일 프로덕션 이미지를 빌드합니다.

# 1. Builder Stage: Go 애플리케이션 컴파일
FROM golang:1.24-bullseye AS builder

LABEL maintainer="poi"
LABEL version="0.1-all-in-one"

# Set build arguments for target architecture
ARG TARGETOS=linux
ARG TARGETARCH=amd64

ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
ENV CGO_ENABLED=0

WORKDIR /app

# 의존성 파일을 먼저 복사하고 다운로드하여 Docker 캐시를 활용합니다.
COPY go.mod go.sum ./
RUN go mod download

# 전체 소스 코드를 복사합니다.
COPY . .

# 필요한 모든 바이너리를 빌드합니다.
RUN go build -ldflags="-w -s" -o /app/bin/tmidb-supervisor ./cmd/supervisor && \
    go build -ldflags="-w -s" -o /app/bin/tmidb-api ./cmd/api && \
    go build -ldflags="-w -s" -o /app/bin/tmidb-data-manager ./cmd/data-manager && \
    go build -ldflags="-w -s" -o /app/bin/tmidb-data-consumer ./cmd/data-consumer && \
    go build -ldflags="-w -s" -o /app/bin/tmidb-cli ./cmd/cli


# 2. Final Stage: 모든 서비스가 포함된 프로덕션 이미지 생성
FROM debian:bullseye-slim

# 필요한 패키지 설치: postgresql, curl(다운로드) 등
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gnupg \
    locales \
    lsb-release \
    util-linux \
    && rm -rf /var/lib/apt/lists/*

# 로케일 설정 (영어 및 한글 지원)
RUN echo "en_US.UTF-8 UTF-8" > /etc/locale.gen && \
    echo "ko_KR.UTF-8 UTF-8" >> /etc/locale.gen && \
    locale-gen en_US.UTF-8 ko_KR.UTF-8 && \
    update-locale LANG=en_US.UTF-8
ENV LANG=en_US.UTF-8
ENV LANGUAGE=en_US:en
ENV LC_ALL=en_US.UTF-8

# PostgreSQL 15 리포지토리 추가 및 설치
RUN curl -s https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor | tee /etc/apt/trusted.gpg.d/postgresql.gpg >/dev/null && \
    echo "deb http://apt.postgresql.org/pub/repos/apt/ $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends postgresql-15 && \
    rm -rf /var/lib/apt/lists/* && \
    ln -s /usr/lib/postgresql/15/bin/postgres /usr/local/bin/postgres && \
    ln -s /usr/lib/postgresql/15/bin/initdb /usr/local/bin/initdb

# TimescaleDB 설치
RUN mkdir -p /etc/apt/keyrings && \
    curl -fsSL https://packagecloud.io/timescale/timescaledb/gpgkey | gpg --dearmor -o /etc/apt/keyrings/timescaledb.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/timescaledb.gpg] https://packagecloud.io/timescale/timescaledb/debian/ $(lsb_release -cs) main" > /etc/apt/sources.list.d/timescaledb.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends timescaledb-2-postgresql-15 && \
    rm -rf /var/lib/apt/lists/*

# NATS 서버 설치
RUN curl -L https://github.com/nats-io/nats-server/releases/download/v2.10.21/nats-server-v2.10.21-linux-amd64.tar.gz | tar -xz && \
    mv nats-server-v2.10.21-linux-amd64/nats-server /usr/local/bin/

# SeaweedFS 설치
RUN curl -L https://github.com/seaweedfs/seaweedfs/releases/download/3.68/linux_amd64.tar.gz | tar -xz && \
    mv weed /usr/local/bin/

# 보안을 위해 서비스별 사용자 생성
RUN adduser --system --group natsuser && \
    adduser --system --group seaweeduser
# postgres 유저는 postgresql-15 패키지 설치 시 자동으로 생성됩니다.

# 디렉토리 생성 및 권한 설정
RUN mkdir -p /app/bin /data/postgresql /data/seaweedfs /data/nats /var/run/postgresql && \
    chown -R postgres:postgres /data/postgresql /var/run/postgresql && \
    chown -R natsuser:natsuser /data/nats && \
    chown -R seaweeduser:seaweeduser /data/seaweedfs && \
    chmod 775 /var/run/postgresql && \
    chmod g+s /var/run/postgresql

# PostgreSQL 데이터 디렉토리를 postgres 사용자로 초기화
USER postgres
RUN initdb -D /data/postgresql --encoding=UTF8 --locale=en_US.UTF-8
USER root

WORKDIR /app

# Go 애플리케이션 및 관련 파일 복사
COPY --from=builder /app/bin /app/bin
COPY cmd/api/views ./views
COPY cmd/api/static ./static

# 바이너리를 시스템 PATH에 추가
ENV PATH="/app/bin:${PATH}"

# Create entrypoint script that starts all services before supervisor
RUN echo '#!/bin/bash' > /usr/local/bin/docker-entrypoint.sh && \
    echo 'set -e' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '# Start PostgreSQL in background' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'echo "🚀 Starting PostgreSQL..."' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'su - postgres -c "postgres -D /data/postgresql -k /var/run/postgresql" &' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'POSTGRES_PID=$!' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'echo "PostgreSQL started with PID $POSTGRES_PID"' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '# Start NATS in background' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'echo "🚀 Starting NATS..."' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'runuser -u natsuser -- nats-server -sd /data/nats &' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'NATS_PID=$!' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'echo "NATS started with PID $NATS_PID"' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '# Start SeaweedFS in background' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'echo "🚀 Starting SeaweedFS..."' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'runuser -u seaweeduser -- weed master -mdir=/data/seaweedfs/master &' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'SEAWEED_PID=$!' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'echo "SeaweedFS started with PID $SEAWEED_PID"' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '# Save PIDs for supervisor to attach' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'echo "$POSTGRES_PID" > /var/run/postgresql.pid' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'echo "$NATS_PID" > /var/run/nats.pid' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'echo "$SEAWEED_PID" > /var/run/seaweedfs.pid' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '# Wait a moment for services to start' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'sleep 3' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'echo "✅ All services started successfully"' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '' >> /usr/local/bin/docker-entrypoint.sh && \
    echo '# Start supervisor to manage services' >> /usr/local/bin/docker-entrypoint.sh && \
    echo 'exec tmidb-supervisor' >> /usr/local/bin/docker-entrypoint.sh && \
    chmod +x /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

# 컨테이너 시작 시 실행될 Supervisor
# Supervisor는 내부적으로 DB, NATS, SeaweedFS 및 Go 애플리케이션들을 관리합니다.
CMD ["tmidb-supervisor"]
