# `tmiDB-Core` 기능 청사진

이 문서는 `tmiDB-Core` 프로젝트의 목적, 구성 요소, 핵심 기능 및 데이터 흐름을 정의하는 기술 청사진입니다.

## 1. 개요

`tmiDB-Core`는 **멀티테넌트 아키텍처**를 기반으로 한 프로세스 관리 기반의 올인원 데이터베이스 시스템입니다. 각 organization은 독립적인 PostgreSQL 데이터베이스를 가지며, 코어 데이터베이스에서 계정 정보와 조직 메타데이터를 관리합니다. 단일 supervisor가 모든 구성요소를 자식 프로세스로 관리하며, CLI를 통해 각 구성요소의 로그를 통합 제어할 수 있습니다.

## 2. 핵심 아키텍처

### 2.1. 프로세스 관리 구조

```
tmidb-supervisor (메인 프로세스)
├── PostgreSQL + TimescaleDB (자식 프로세스)
│   ├── _core_tmidb (코어 데이터베이스)
│   ├── tmidb_org1 (조직별 데이터베이스)
│   ├── tmidb_org2 (조직별 데이터베이스)
│   └── ...
├── NATS Server (자식 프로세스)
├── SeaweedFS (자식 프로세스)
├── api (자식 프로세스)
├── data-manager (자식 프로세스)
└── data-consumer (자식 프로세스)
```

### 2.2. 멀티테넌트 데이터베이스 구조

#### 코어 데이터베이스 (\_core_tmidb)

- **사용자 관리**: 계정 정보, 비밀번호 해시, 역할
- **토큰 관리**: API 토큰, 권한 설정
- **조직 메타데이터**: organization 정보, 데이터베이스 매핑
- **시스템 설정**: 전역 설정, 조직별 설정
- **통계 캐싱**: 시스템 및 조직별 통계
- **활동 로그**: 사용자 활동, 감사 로그

#### 조직별 데이터베이스 (tmidb\_{org_name})

- **데이터 저장**: 카테고리별 실제 데이터
- **스키마 관리**: 카테고리 스키마 정의
- **리스너 설정**: 데이터 구독 설정
- **파일 메타데이터**: 첨부 파일 정보
- **시계열 데이터**: TimescaleDB 하이퍼테이블

### 2.3. 로그 관리 시스템

- **통합 로그**: 모든 구성요소의 로그가 supervisor를 통해 통합 출력
- **CLI 제어**: `tmidb-cli`를 통해 각 구성요소별 로그 on/off 제어
- **실시간 모니터링**: 각 프로세스의 상태와 로그를 실시간으로 관리

## 3. 핵심 구성 요소

### 3.1. tmidb-supervisor

**역할**: 전체 시스템의 프로세스 관리자

**기능**:

- 모든 외부 서비스(DB, NATS, SeaweedFS) 자식 프로세스로 시작/관리
- 핵심 구성요소(api, data-manager, data-consumer) 프로세스 관리
- 통합 로그 수집 및 출력
- 프로세스 헬스체크 및 자동 재시작
- CLI 명령 수신 및 처리

### 3.2. tmidb-cli

**역할**: 시스템 제어 CLI 도구

**기능**:

- 각 구성요소별 로그 켜기/끄기 제어
- 프로세스 상태 확인
- 시스템 설정 변경
- 실시간 로그 모니터링

**사용 예시**:

```bash
tmidb-cli logs enable api
tmidb-cli logs disable postgresql
tmidb-cli logs show data-manager
tmidb-cli status
```

### 3.3. api

**역할**: REST API 서버 및 웹 관리 콘솔

**기능**:

- 웹 관리 콘솔 제공 (Go html/template 기반)
- REST API 엔드포인트 노출
- 사용자 인증 및 인가 (세션 + Bearer 토큰)
- 데이터베이스 직접 연동

### 3.4. data-manager

**역할**: 데이터 수집 및 관리

**기능**:

- 외부 데이터 소스로부터 데이터 수집
- 데이터 검증 및 변환
- NATS를 통한 데이터 발행
- 카테고리 스키마 기반 데이터 처리
- 실시간 데이터 스트리밍

### 3.5. data-consumer

**역할**: 데이터 소비 및 처리

**기능**:

- NATS 메시지 구독 및 처리
- 데이터베이스 쓰기 작업
- 배치 처리 및 집계
- 데이터 정합성 검증
- 실시간 알림 처리

## 4. 데이터 흐름

### 4.1. 데이터 수집 흐름

```
외부 데이터 소스 → data-manager → NATS → data-consumer → 조직별 PostgreSQL/TimescaleDB
```

### 4.2. API 요청 흐름

```
클라이언트 → api → 코어 DB (인증) → 조직별 DB (데이터)
```

### 4.3. 파일 저장 흐름

```
클라이언트 → api → SeaweedFS
```

### 4.4. Setup 및 Organization 생성 흐름

```
Setup 요청 → 코어 DB 생성 → Organization DB 생성 → 스키마 초기화 → 기본 리스너 생성 → 관리자 계정 생성
```

## 5. 기술 스택

### 5.1. 외부 서비스

- **PostgreSQL 15 + TimescaleDB**: 시계열 데이터 저장
- **NATS JetStream**: 메시지 큐 및 스트리밍
- **SeaweedFS**: 분산 파일 저장

### 5.2. Go 구성요소

- **Fiber**: 웹 프레임워크 (api)
- **html/template**: 웹 템플릿 엔진
- **NATS Go Client**: 메시지 처리
- **pgx**: PostgreSQL 드라이버

## 6. 개발 환경

### 6.1. 컨테이너 구성

- **프로덕션**: 올인원 컨테이너 (모든 서비스 포함)
- **개발**: 라이브 리로딩 지원 (air 사용)
- **볼륨 마운트**: 소스 코드 실시간 반영

### 6.2. 로그 관리

- **통합 로그**: supervisor를 통한 중앙집중식 로그 관리
- **선택적 출력**: CLI를 통한 구성요소별 로그 제어
- **구조화된 로그**: JSON 형태의 구조화된 로그 출력

## 7. 구현 우선순위

### Phase 1: 기본 프로세스 관리 ✅ **완료**

1. tmidb-supervisor 구현
2. 외부 서비스 프로세스 관리
3. 기본 로그 통합

### Phase 2: 핵심 구성요소 ✅ **완료**

1. api 서버 구현
2. data-manager 구현
3. data-consumer 구현

### Phase 3: CLI 및 고급 기능 ✅ **완료**

1. tmidb-cli 구현
2. 로그 제어 시스템
3. 프로세스 모니터링

### Phase 4: 웹 관리 콘솔 ✅ **완료**

1. 사용자 관리
2. 데이터 탐색기
3. 시스템 모니터링 대시보드

### Phase 5: 멀티테넌트 아키텍처 ✅ **완료**

1. 코어 데이터베이스 구조 설계
2. Organization별 데이터베이스 생성
3. Setup 과정 구현
4. 동적 데이터베이스 연결
5. 권한 체계 구현

## 8. 향후 확장성

- **마이크로서비스 분리**: 필요시 각 구성요소를 독립 서비스로 분리 가능
- **수평 확장**: data-manager, data-consumer의 다중 인스턴스 지원
- **플러그인 시스템**: 외부 데이터 소스 연동을 위한 플러그인 아키텍처
- **클러스터링**: 다중 노드 환경에서의 분산 처리 지원
- **조직별 독립 배포**: 각 organization을 별도 서버에 분산 배포 가능
- **데이터베이스 샤딩**: 대용량 organization의 데이터베이스 분할 지원

## 9. Setup 및 Organization 관리

### 9.1. 초기 Setup 과정

#### 1단계: 코어 데이터베이스 초기화

```bash
# 코어 데이터베이스 생성 및 스키마 초기화
docker exec tmidb-tmidb-core-1 /app/bin/api --init-db
```

#### 2단계: Organization 생성

```bash
# Organization 생성 API 호출
curl -X POST "http://localhost:8020/api/setup/organization" \
  -H "Content-Type: application/json" \
  -d '{
    "organization_name": "Hospital ABC",
    "admin_username": "admin",
    "admin_password": "secure_password"
  }'
```

#### 3단계: 자동 생성되는 구성요소

- **PostgreSQL 데이터베이스**: `tmidb_hospital_abc`
- **스키마 및 테이블**: 모든 데이터 테이블 자동 생성
- **기본 리스너**: metrics, logs, events 리스너
- **관리자 계정**: 지정된 사용자명/비밀번호로 생성
- **API 토큰**: 초기 관리자용 토큰 자동 생성

### 9.2. Organization 전환

#### 웹콘솔에서 전환

- 사이드바의 조직 드롭다운에서 선택
- URL 파라미터로 organization ID 전달
- 세션에 organization 정보 저장

#### API를 통한 전환

```bash
curl -X POST "http://localhost:8020/api/manage/session/organization" \
  -H "Authorization: Bearer {token}" \
  -H "Content-Type: application/json" \
  -d '{"organization_id": "org_uuid"}'
```

### 9.3. 권한 체계

#### 사용자 역할

- **관리자 (admin)**: 모든 organization 접근 및 관리
- **편집자 (editor)**: 해당 organization 내 데이터 읽기/쓰기
- **뷰어 (viewer)**: 해당 organization 내 데이터 읽기만

#### 토큰 권한

- **카테고리별 접근 제어**: 특정 카테고리만 읽기/쓰기 허용
- **전체 권한**: 모든 카테고리 접근 허용
- **관리자 권한**: 시스템 전체 관리 권한

### 9.4. 데이터베이스 연결 관리

#### 동적 연결

- Organization ID로 해당 데이터베이스 자동 연결
- 연결 풀 관리로 성능 최적화
- 연결 실패 시 자동 재시도

#### 메타데이터 관리

- 코어 DB에 organization별 데이터베이스 정보 저장
- 데이터베이스 생성/삭제 이력 추적
- 백업 및 복원 정보 관리

## 10. 조직별 데이터베이스 관리 함수

### 10.1. 데이터베이스 생성 함수

```go
// CreateOrganizationDatabase 함수
func CreateOrganizationDatabase(orgName string) error {
    // 1. 코어 DB에서 organization 정보 조회
    // 2. PostgreSQL에 새 데이터베이스 생성
    // 3. 기본 스키마 및 테이블 생성
    // 4. 기본 리스너 설정 생성
    // 5. 메타데이터 업데이트
}

// DropOrganizationDatabase 함수
func DropOrganizationDatabase(orgName string) error {
    // 1. 연결된 모든 세션 종료
    // 2. 데이터베이스 삭제
    // 3. 코어 DB에서 메타데이터 삭제
}
```

### 10.2. 데이터베이스 연결 관리 함수

```go
// GetOrganizationConnection 함수
func GetOrganizationConnection(orgID string) (*sql.DB, error) {
    // 1. 코어 DB에서 organization 정보 조회
    // 2. 연결 풀에서 기존 연결 확인
    // 3. 없으면 새 연결 생성
    // 4. 연결 풀에 등록
}

// CloseOrganizationConnection 함수
func CloseOrganizationConnection(orgID string) error {
    // 1. 연결 풀에서 연결 제거
    // 2. 실제 DB 연결 종료
}
```

### 10.3. 스키마 관리 함수

```go
// InitializeOrganizationSchema 함수
func InitializeOrganizationSchema(orgName string) error {
    // 1. 기본 테이블 생성 (categories, listeners, files 등)
    // 2. TimescaleDB 하이퍼테이블 생성
    // 3. 인덱스 생성
    // 4. 기본 리스너 설정
}

// MigrateOrganizationSchema 함수
func MigrateOrganizationSchema(orgName string, version int) error {
    // 1. 현재 스키마 버전 확인
    // 2. 마이그레이션 스크립트 실행
    // 3. 스키마 버전 업데이트
}
```

## 11. 파일 첨부 시스템

### 11.1. 파일 첨부 방식

#### 옵션 1: URL 전용 방식 (기본)

카테고리 데이터 조회 시 파일은 URL로만 제공됩니다.

```json
{
  "target_id": "patient_123",
  "category": "vital",
  "data": {
    "bp": 120,
    "hr": 72,
    "files": [
      {
        "file_id": "file_uuid_abc123",
        "filename": "blood_test.pdf",
        "file_url": "http://seaweed:8080/1,abc123.pdf",
        "thumbnail_url": "http://seaweed:8080/1,abc123_thumb.jpg",
        "file_size": 2048576,
        "file_type": "application/pdf",
        "created_at": "2024-12-01T09:30:00Z"
      }
    ]
  }
}
```

#### 옵션 2: /attach 엔드포인트 방식 (파일 blob 요청)

요청 시 `/attach`를 붙이면 파일을 blob으로 직접 제공합니다.

```http
# 카테고리의 모든 파일을 blob으로 요청
GET /api/v1/targets/patient_123/categories/vital/attach

# 특정 파일만 blob으로 요청
GET /api/v1/targets/patient_123/categories/vital/files/file_uuid_abc123/attach
```

**응답:**

```http
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="blood_test.pdf"

[파일 바이너리 데이터]
```

#### 옵션 3: 파일 업로드 (POST)

파일을 업로드할 때는 별도 엔드포인트를 사용합니다.

```http
POST /api/v1/targets/patient_123/categories/vital/files
Content-Type: multipart/form-data
```

### 11.2. 파일 관리 API

```go
// GetFileBlob 함수
func GetFileBlob(targetID, category, fileID string) ([]byte, error) {
    // 1. 파일 메타데이터 조회
    // 2. SeaweedFS에서 파일 다운로드
    // 3. blob 데이터 반환
}

// GetCategoryFilesBlob 함수
func GetCategoryFilesBlob(targetID, category string) ([]byte, error) {
    // 1. 카테고리의 모든 파일 메타데이터 조회
    // 2. 모든 파일을 zip으로 압축
    // 3. 압축된 blob 데이터 반환
}

// UploadFile 함수
func UploadFile(targetID, category string, file multipart.File) error {
    // 1. 파일 검증 (크기, 형식 등)
    // 2. SeaweedFS에 업로드
    // 3. 썸네일 생성 (이미지인 경우)
    // 4. 메타데이터 저장
}

// DeleteFile 함수
func DeleteFile(targetID, category, fileID string) error {
    // 1. 파일 메타데이터 조회
    // 2. SeaweedFS에서 파일 삭제
    // 3. 메타데이터 삭제
}
```

## 12. 리스너 응답 구조 개선

### 12.1. 리스너 쿼리 정보 포함

```json
{
  "responseTime": "2024-12-01 15:04:05",
  "vitalDashboard": {
    "listener_info": {
      "listener_id": "vitalDashboard",
      "query": "bp>120 OR hr>100",
      "categories": ["vital", "ward"],
      "version": "v2"
    },
    "vital": {
      "version": "v2",
      "data": [
        {
          "target_id": "patient_123",
          "target_name": "홍길동",
          "data": {
            "bp": 120,
            "hr": 72,
            "spo2": 98
          },
          "version": "v2",
          "updated_at": "2024-12-01T15:00:00Z"
        }
      ]
    },
    "ward": {
      "version": "v1",
      "data": [
        {
          "target_id": "patient_123",
          "target_name": "홍길동",
          "data": {
            "ward_number": "ICU-101",
            "bed_number": "A1"
          },
          "version": "v1",
          "updated_at": "2024-12-01T15:00:00Z"
        }
      ]
    }
  }
}
```

### 12.2. 리스너 관리 함수

```go
// CreateListener 함수
func CreateListener(orgID, listenerID string, config ListenerConfig) error {
    // 1. 리스너 설정 검증
    // 2. 쿼리 파싱 및 최적화
    // 3. 데이터베이스에 저장
}

// GetListenerData 함수
func GetListenerData(orgID, listenerID string) (*ListenerResponse, error) {
    // 1. 리스너 설정 조회
    // 2. 쿼리 실행
    // 3. 데이터 조합
    // 4. 응답 구조 생성
}
```

## 13. 구현 계획

### Phase 6: 파일 시스템 개선 ⏳ **진행 예정**

1. **파일 첨부 방식 결정**: URL 전용 vs /attach 엔드포인트
2. **SeaweedFS 연동**: 파일 업로드/다운로드 구현
3. **썸네일 생성**: 이미지 파일 자동 썸네일 생성
4. **파일 메타데이터 관리**: 데이터베이스 연동

### Phase 7: 리스너 시스템 개선 ⏳ **진행 예정**

1. **리스너 쿼리 정보**: 응답에 쿼리 정보 포함
2. **데이터 구조 개선**: data 객체 내부에 실제 데이터 배치
3. **쿼리 최적화**: 성능 향상을 위한 쿼리 최적화
4. **실시간 구독**: WebSocket 기반 실시간 데이터 스트리밍

### Phase 8: 데이터베이스 관리 고도화 ⏳ **진행 예정**

1. **동적 데이터베이스 생성**: Organization 생성 시 자동 DB 생성
2. **연결 풀 최적화**: 성능 향상을 위한 연결 관리
3. **백업 및 복원**: Organization별 데이터 백업 시스템
4. **모니터링**: 데이터베이스 성능 및 상태 모니터링
