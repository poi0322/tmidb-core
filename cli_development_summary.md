# tmiDB CLI 개발 요약

## 개요

tmiDB CLI는 tmiDB-Core 시스템의 모든 구성 요소를 관리하고 모니터링하기 위한 명령줄 도구입니다. Unix Domain Socket을 통한 IPC 통신으로 Supervisor와 상호작용합니다.

## 구현 완료 기능

### Phase 1: 핵심 기능 ✅

#### 1. 로그 관리

- ✅ `logs` - 컴포넌트 로그 표시
- ✅ `logs -f` - 실시간 로그 스트리밍
- ✅ `logs enable <component>` - 로그 활성화
- ✅ `logs disable <component>` - 로그 비활성화
- ✅ `logs status` - 로그 상태 확인
- ✅ `logs follow <component>` - 특정 컴포넌트 로그 추적

#### 2. 프로세스 제어

- ✅ `process list` - 모든 프로세스 목록
- ✅ `process status <component>` - 프로세스 상태 확인
- ✅ `process start <component>` - 프로세스 시작
- ✅ `process stop <component>` - 프로세스 중지
- ✅ `process restart <component>` - 프로세스 재시작

#### 3. 시스템 모니터링

- ✅ `monitor system` - 실시간 시스템 리소스 모니터링
- ✅ `monitor services` - 서비스 헬스 상태
- ✅ `monitor health` - 전체 시스템 헬스 체크

#### 4. 상태 확인

- ✅ `status` - 모든 컴포넌트 상태 요약

### Phase 2: 기능 확장 ✅

#### 1. 로그 필터링

- ✅ `logs filter` - 고급 로그 필터링 (레벨, 시간, 패턴)
- ✅ `logs search <pattern>` - 정규식 패턴으로 로그 검색
- ✅ `--level` - 로그 레벨별 필터링
- ✅ `--since/--until` - 시간 범위 지정
- ✅ `--pattern` - 정규식 패턴 매칭

#### 2. 프로세스 그룹 제어

- ✅ `process group list` - 프로세스 그룹 목록
- ✅ `process group start/stop/restart <group>` - 그룹 단위 제어
- ✅ `process group status <group>` - 그룹 상태 확인
- ✅ `process batch start/stop` - 여러 프로세스 일괄 제어
- ✅ 의존성 기반 시작/중지 순서

#### 3. 설정 관리

- ✅ `config get/set` - 설정 조회 및 변경
- ✅ `config list` - 모든 설정 키 목록
- ✅ `config reset` - 기본값으로 초기화
- ✅ `config export/import` - 설정 백업 및 복원
- ✅ `config validate` - 설정 유효성 검증

### Phase 3: 사용성 개선 ✅

#### 1. JSON 출력 옵션

- ✅ `--output json` - JSON 형식 출력
- ✅ `--output json-pretty` - 들여쓰기된 JSON 출력
- ✅ 모든 명령어에 전역 플래그로 지원
- ✅ 구조화된 데이터 포맷터

### Phase 4: 고급 기능 ✅

#### 1. 백업/복구

- ✅ `backup create` - 백업 생성 (자동/수동 이름)
- ✅ `backup restore` - 백업에서 복구
- ✅ `backup list` - 백업 목록 조회
- ✅ `backup delete` - 백업 삭제
- ✅ `backup verify` - 백업 무결성 검증
- ✅ 진행 상황 모니터링
- ✅ 선택적 컴포넌트 백업/복구

#### 2. 진단 도구

- ✅ `diagnose all` - 전체 시스템 진단
- ✅ `diagnose component <name>` - 특정 컴포넌트 진단
- ✅ `diagnose connectivity` - 연결성 테스트
- ✅ `diagnose performance` - 성능 분석
- ✅ `diagnose logs` - 로그 분석
- ✅ `diagnose fix` - 자동 문제 해결 (dry-run 지원)

## 테스트 결과

### 1. 단위 테스트

- ✅ 유틸리티 함수 테스트 (`formatDuration`, `formatBytes`)
- ✅ Mock IPC 서버 구현

### 2. 통합 테스트

- ✅ Mock Supervisor를 사용한 CLI 명령어 테스트
- ✅ IPC 통신 정상 동작 확인
- ✅ 에러 처리 검증

### 3. 테스트 스크립트

- `test_cli_simple.sh` - 기본 CLI 기능 테스트
- `test_cli_integration.sh` - Mock Supervisor와 통합 테스트
- `test_cli.sh` - 실제 Supervisor와 전체 테스트

## 사용 방법

### 빌드

```bash
cd tmidb-core
go build -o ./bin/tmidb-cli ./cmd/cli
```

### 기본 사용법

```bash
# 상태 확인
tmidb-cli status
tmidb-cli status --output json

# 프로세스 제어
tmidb-cli process list
tmidb-cli process restart api
tmidb-cli process group start all
tmidb-cli process batch stop api data-manager

# 로그 관리
tmidb-cli logs
tmidb-cli logs api -f  # API 로그 실시간 추적
tmidb-cli logs filter --level=error --since=1h
tmidb-cli logs search "connection failed"

# 설정 관리
tmidb-cli config get api.port
tmidb-cli config set log.level debug
tmidb-cli config export config-backup.yaml

# 백업/복구
tmidb-cli backup create
tmidb-cli backup restore backup-20240101-120000
tmidb-cli backup list

# 진단
tmidb-cli diagnose all
tmidb-cli diagnose performance --duration=60s
tmidb-cli diagnose fix --dry-run
```

### 환경 변수

- `TMIDB_SOCKET_PATH` - Unix 소켓 경로 지정 (기본값: `/tmp/tmidb-supervisor.sock`)

## 아키텍처

```
┌─────────────┐     IPC      ┌─────────────┐
│  tmidb-cli  │ ◄──────────► │ Supervisor  │
└─────────────┘  Unix Socket └─────────────┘
      │                            │
      │                            ├── Process Manager
      └── Commands                 ├── Log Manager
          ├── logs                 └── System Monitor
          ├── process
          ├── monitor
          └── status
```

## 주요 코드 구조

### 명령어 구조

- `rootCmd` - 최상위 명령어
- `logsCmd` - 로그 관련 서브커맨드
- `processCmd` - 프로세스 관련 서브커맨드
- `monitorCmd` - 모니터링 관련 서브커맨드
- `statusCmd` - 상태 확인 명령어

### IPC 통신

- `ipc.Client` - Unix Socket 클라이언트
- 메시지 타입별 핸들러
- 비동기 응답 처리

## 추가 개선 가능 사항

### 자동 완성

- [ ] Bash 자동 완성 스크립트
- [ ] Zsh 자동 완성 스크립트
- [ ] Fish 자동 완성 스크립트

### 대화형 모드

- [ ] 대화형 셸 모드
- [ ] 실시간 대시보드
- [ ] TUI (Terminal UI) 인터페이스

### 성능 최적화

- [ ] 응답 시간 프로파일링
- [ ] 메모리 사용량 최적화
- [ ] 배치 작업 최적화

### 플러그인 시스템

- [ ] 사용자 정의 명령어 지원
- [ ] 확장 가능한 아키텍처
- [ ] 스크립트 통합

## 문제 해결

### Supervisor 연결 실패

```
❌ Failed to connect to supervisor: dial unix /tmp/tmidb-supervisor.sock: connect: no such file or directory
💡 Make sure tmidb-supervisor is running
```

**해결**: Supervisor가 실행 중인지 확인하고, 소켓 경로가 올바른지 확인

### 권한 오류

```
❌ Failed to connect to supervisor: dial unix /tmp/tmidb-supervisor.sock: connect: permission denied
```

**해결**: 소켓 파일의 권한 확인 (`ls -l /tmp/tmidb-supervisor.sock`)

## 결론

tmiDB CLI는 Phase 1-4의 모든 계획된 기능이 성공적으로 구현되었습니다.

**구현 완료된 주요 기능:**

- ✅ **로그 관리**: 기본 조회부터 고급 필터링, 실시간 스트리밍까지 완벽 지원
- ✅ **프로세스 제어**: 개별 제어부터 그룹 관리, 의존성 기반 제어까지 구현
- ✅ **설정 관리**: 런타임 설정 변경, 백업/복원, 검증 기능 제공
- ✅ **시스템 모니터링**: 실시간 리소스 모니터링 및 헬스 체크
- ✅ **백업/복구**: 전체/부분 백업, 무결성 검증, 진행 상황 모니터링
- ✅ **진단 도구**: 종합 진단, 성능 분석, 자동 문제 해결
- ✅ **JSON 출력**: 모든 명령어에서 구조화된 데이터 출력 지원

이제 tmiDB는 도커 컨테이너 환경에서 완벽하게 운영 가능한 CLI 도구를 갖추게 되었습니다.
