# tmiDB CLI Blueprint

## 개요

tmiDB CLI는 tmiDB-Core 시스템의 모든 구성 요소를 관리하고 모니터링하기 위한 명령줄 인터페이스입니다. Unix Domain Socket을 통해 Supervisor와 통신하며, 로그 관리, 프로세스 제어, 시스템 모니터링 등의 기능을 제공합니다.

## 아키텍처

### 통신 구조

```
tmidb-cli <--> IPC Client <--> Unix Socket <--> IPC Server <--> Supervisor
                                                                     |
                                                                     +-- Process Manager
                                                                     +-- Log Manager
                                                                     +-- System Monitor
```

### 주요 컴포넌트

- **IPC Client**: Unix Domain Socket을 통한 Supervisor와의 통신 담당
- **Command Handlers**: 각 명령어의 로직 처리
- **Response Formatters**: 응답 데이터의 사용자 친화적 표시

## 명령어 구조

### 1. 로그 관리 (Logs Management)

#### `tmidb-cli logs [component] [-f|--follow]`

- **설명**: 컴포넌트의 로그를 표시합니다.
- **매개변수**:
  - `component` (선택): 특정 컴포넌트 이름 (기본값: "all")
  - `-f, --follow`: 실시간 로그 스트리밍
- **예시**:
  ```bash
  tmidb-cli logs                    # 모든 컴포넌트의 최근 로그
  tmidb-cli logs api -f             # API 서버의 실시간 로그
  tmidb-cli logs data-manager       # Data Manager의 최근 로그
  ```

#### `tmidb-cli logs enable <component>`

- **설명**: 특정 컴포넌트의 로그 출력을 활성화합니다.
- **매개변수**:
  - `component` (필수): 컴포넌트 이름
- **지원 컴포넌트**: api, data-manager, data-consumer, postgresql, nats, seaweedfs

#### `tmidb-cli logs disable <component>`

- **설명**: 특정 컴포넌트의 로그 출력을 비활성화합니다.
- **매개변수**:
  - `component` (필수): 컴포넌트 이름

#### `tmidb-cli logs status`

- **설명**: 모든 컴포넌트의 로그 활성화 상태를 표시합니다.
- **출력 형식**:
  ```
  📊 Component Log Status:
    api             : 🔊 Enabled
    data-manager    : 🔇 Disabled
    ...
  ```

### 2. 프로세스 제어 (Process Control)

#### `tmidb-cli process list`

- **설명**: 모든 tmiDB 프로세스의 목록과 상태를 표시합니다.
- **출력 형식**:
  ```
  NAME                 STATUS       PID      UPTIME       MEMORY     CPU
  --------------------------------------------------------------------------------
  api                  running      12345    2h30m15s     45.2MB     12.5%
  data-manager         running      12346    2h30m10s     32.1MB     8.3%
  ```

#### `tmidb-cli process status <component>`

- **설명**: 특정 컴포넌트의 상세 상태를 표시합니다.
- **매개변수**:
  - `component` (필수): 컴포넌트 이름

#### `tmidb-cli process start <component>`

- **설명**: 중지된 컴포넌트를 시작합니다.
- **매개변수**:
  - `component` (필수): 컴포넌트 이름

#### `tmidb-cli process stop <component>`

- **설명**: 실행 중인 컴포넌트를 중지합니다.
- **매개변수**:
  - `component` (필수): 컴포넌트 이름

#### `tmidb-cli process restart <component>`

- **설명**: 컴포넌트를 재시작합니다.
- **매개변수**:
  - `component` (필수): 컴포넌트 이름

### 3. 시스템 모니터링 (System Monitoring)

#### `tmidb-cli monitor system`

- **설명**: 실시간 시스템 리소스 사용량을 모니터링합니다.
- **업데이트 주기**: 2초
- **표시 정보**: 프로세스 수, 실행 중, 중지됨, 오류, IPC 연결 수

#### `tmidb-cli monitor services`

- **설명**: 모든 서비스의 헬스 상태를 표시합니다.
- **출력 정보**:
  - 전체 상태 (healthy/degraded/unhealthy)
  - 가동 시간
  - 각 컴포넌트별 상태
  - 오류 목록 (있는 경우)

#### `tmidb-cli monitor health`

- **설명**: 시스템의 전반적인 헬스 체크를 수행합니다.
- **체크 항목**:
  - Supervisor 응답성
  - 각 컴포넌트 실행 상태
  - 전체 시스템 헬스 점수

### 4. 상태 확인 (Status Check)

#### `tmidb-cli status`

- **설명**: 모든 tmiDB 컴포넌트의 상태를 요약해서 표시합니다.
- **표시 정보**: 상태, PID, 가동 시간, 메모리 사용량, CPU 사용률

## 오류 처리

### 연결 오류

- Supervisor와 연결할 수 없는 경우:
  ```
  ❌ Failed to connect to supervisor: [error details]
  💡 Make sure tmidb-supervisor is running
  ```

### 명령 실행 오류

- 각 명령의 실패 시 구체적인 오류 메시지 표시
- 복구 가능한 오류의 경우 해결 방법 제안

## 개선 필요 사항

### 1. 현재 구현 상태

- ✅ 기본 명령어 구조
- ✅ IPC 통신 프레임워크
- ⚠️ 일부 명령어의 실제 동작 미구현
- ❌ 테스트 코드 부재

### 2. 구현 필요 기능

1. **로그 필터링**

   - 로그 레벨별 필터링 (debug, info, warn, error)
   - 시간 범위 지정
   - 정규식 패턴 매칭

2. **프로세스 그룹 제어**

   - 여러 컴포넌트 동시 제어
   - 의존성 기반 시작/중지 순서

3. **설정 관리**

   - `tmidb-cli config get/set` 명령어
   - 런타임 설정 변경

4. **백업/복구**

   - 데이터베이스 백업 명령어
   - 설정 백업/복구

5. **진단 도구**
   - 성능 프로파일링
   - 디버그 정보 수집
   - 문제 진단 마법사

### 3. 사용성 개선

1. **자동 완성**

   - Bash/Zsh 자동 완성 스크립트
   - 컴포넌트 이름 자동 완성

2. **대화형 모드**

   - `tmidb-cli interactive` 명령어
   - 실시간 대시보드 뷰

3. **출력 형식**
   - JSON 출력 옵션 (`--json`)
   - 테이블 형식 개선
   - 색상 코딩 개선

## 테스트 계획

### 단위 테스트

- 각 명령어 핸들러 테스트
- IPC 통신 테스트
- 응답 포맷터 테스트

### 통합 테스트

- Supervisor와의 실제 통신 테스트
- 명령어 체인 테스트
- 오류 시나리오 테스트

### 성능 테스트

- 대량 로그 스트리밍 성능
- 동시 연결 처리
- 응답 시간 측정

## 보안 고려사항

1. **권한 관리**

   - Unix 소켓 파일 권한 (0666)
   - 민감한 작업에 대한 확인 프롬프트

2. **입력 검증**

   - 컴포넌트 이름 유효성 검사
   - 명령어 인젝션 방지

3. **감사 로깅**
   - 모든 CLI 명령 실행 기록
   - 사용자 식별 정보 포함

## 구현 로드맵

### Phase 1: 핵심 기능 완성 ✅ 완료

- [x] 기본 명령어 구조
- [x] IPC 통신 구현
- [x] 모든 명령어 실제 동작 구현
- [x] 기본 테스트 작성

### Phase 2: 기능 확장 ✅ 완료

- [x] 로그 필터링 기능 (레벨, 시간, 패턴)
- [x] 프로세스 그룹 제어
- [x] 설정 관리 명령어 (get/set/export/import/validate)

### Phase 3: 사용성 개선 ✅ 부분 완료

- [x] JSON 출력 형식 옵션 (--output json/json-pretty)
- [x] 색상 코딩 (로그 레벨별, 상태별)
- [ ] 자동 완성 스크립트 (향후 개발)
- [ ] 대화형 모드 (향후 개발)

### Phase 4: 고급 기능 ✅ 완료

- [x] 백업/복구 기능 (create/restore/list/delete/verify)
- [x] 진단 도구 (all/component/connectivity/performance/logs/fix)
- [x] 성능 분석 및 모니터링
