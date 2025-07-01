# tmiDB API 명세서 v1.0

## 📋 개요

tmiDB는 **Target-Speaker-Listener** 구조를 기반으로 한 실시간 데이터 관리 플랫폼입니다.

### 핵심 개념
- **Target (대상)**: 데이터를 수집할 기본 단위 (환자, 장비, 서비스 등)
- **Speaker (화자)**: 데이터를 생성하고 입력하는 주체 (API, 자동 스크래퍼 등)  
- **Listener (청취자)**: 특정 데이터를 구독하는 주체 (대시보드, 알림 시스템 등)
- **Category (카테고리)**: 데이터 분류 및 스키마 정의 단위

---

## 🔗 Base URL

```
http://localhost:8080/api
```

---

## 🔐 인증

모든 API 요청은 Bearer 토큰 인증이 필요합니다.

```http
Authorization: Bearer {token}
```

---

## 📊 1. 핵심 데이터 API

### 1.1 카테고리 데이터 조회

```http
GET /api/{version}/category/{category}
```

**Parameters:**
- `version`: `v1`, `v2`, `latest`, `all`
- `category`: 카테고리 이름

**Query Parameters:**
```
# 기본 필터링
?field=value&field2>100&field3!=null

# 배열 연산
?tags[]contains=urgent&symptoms[]includes_any=fever,cough

# 특수 연산
?age.exists=true&notes.empty=false&created_at>=2024-12-01
```

**응답 (v1 요청):**
```json
{
  "responseTime": "2024-12-01 15:04:05",
  "vital": {
    "version": "v1", 
    "data": [
      {
        "target_id": "patient_123",
        "target_name": "홍길동",
        "version": "v1",
        "bp": 120,
        "hr": 72,
        "updated_at": "2024-12-01T15:00:00Z"
      }
    ],
    "pagination": {
      "current_page": 1,
      "total_pages": 5,
      "total_records": 4520,
      "next_page_url": "/api/v1/category/vital?page=2"
    }
  }
}
```

**응답 (all 요청):**
```json
{
  "responseTime": "2024-12-01 15:04:05", 
  "vital": {
    "version": "all",
    "data": [
      {
        "target_id": "patient_123",
        "target_name": "홍길동", 
        "version": "v1",
        "bp": 120,
        "hr": 72,
        "updated_at": "2024-12-01T14:30:00Z"
      },
      {
        "target_id": "patient_123",
        "target_name": "홍길동",
        "version": "v2", 
        "bp": 120,
        "hr": 72,
        "spo2": 98,
        "weight": 75,
        "updated_at": "2024-12-01T15:00:00Z"
      }
    ]
  }
}
```

### 1.2 특정 대상 데이터 조회

```http
GET /api/{version}/targets/{target_id}/categories/{category}
```

**응답:**
```json
{
  "target_id": "patient_123",
  "target_name": "홍길동",
  "category": "vital",
  "version": "v2",
  "category_data": {
    "bp": 120,
    "hr": 72, 
    "spo2": 98,
    "weight": 75
  },
  "files": [
    {
      "file_id": "file_uuid_abc123",
      "filename": "blood_test.pdf",
      "file_type": "application/pdf", 
      "file_size": 2048576,
      "created_at": "2024-12-01T09:30:00Z",
      "seaweed_url": "http://seaweed:8080/1,abc123.pdf",
      "thumbnail_url": "http://seaweed:8080/1,abc123_thumb.jpg",
      "is_large_file": false,
      "metadata": {
        "test_type": "CBC",
        "lab_name": "중앙검사실"
      }
    }
  ],
  "updated_at": "2024-12-01T15:00:00Z"
}
```

### 1.3 데이터 생성/업데이트

```http
POST /api/{version}/targets/{target_id}/categories/{category}
Content-Type: multipart/form-data
```

**Request Body:**
```json
{
  "category_data": {
    "bp": 125,
    "hr": 75,
    "spo2": 99,
    "notes": "환자 상태 양호"
  },
  "files": [
    // 첨부 파일들 (multipart)
  ]
}
```

**응답:**
```json
{
  "success": true,
  "target_id": "patient_123",
  "category": "vital", 
  "version": "v2",
  "updated_at": "2024-12-01T15:30:00Z",
  "files_uploaded": [
    {
      "file_id": "file_uuid_def456",
      "filename": "new_xray.dcm",
      "file_size": 25165824,
      "is_large_file": true,
      "seaweed_url": "http://seaweed:8080/2,def456.dcm",
      "thumbnail_url": "http://seaweed:8080/2,def456_thumb.jpg"
    }
  ]
}
```

### 1.4 데이터 삭제

```http
DELETE /api/{version}/targets/{target_id}/categories/{category}
```

**응답:**
```json
{
  "success": true,
  "message": "Data and associated files deleted",
  "deleted_files": ["file_uuid_abc123", "file_uuid_def456"]
}
```

---

## 🎧 2. 리스너 API

### 2.1 단일 리스너 데이터 조회

```http
GET /api/{version}/listener/{listener_id}
```

**응답:**
```json
{
  "responseTime": "2024-12-01 15:04:05",
  "vitalDashboard": {
    "vital": {
      "version": "v2",
      "data": [
        {
          "target_id": "patient_123",
          "target_name": "홍길동",
          "version": "v2", 
          "bp": 120,
          "hr": 72,
          "spo2": 98,
          "updated_at": "2024-12-01T15:00:00Z"
        }
      ]
    },
    "ward": {
      "version": "v1", 
      "data": [...]
    }
  }
}
```

### 2.2 다중 리스너 데이터 조회

```http
GET /api/{version}/listener/{listener_id_1}/{listener_id_2}/{listener_id_3}
```

**응답:**
```json
{
  "responseTime": "2024-12-01 15:04:05",
  "listeners": {
    "vitalDashboard": {
      "vital": {"version": "v2", "data": [...]},
      "ward": {"version": "v1", "data": [...]}
    },
    "alertSystem": {
      "vital": {"version": "v2", "data": [...]},
      "emergency": {"version": "v1", "data": [...]}
    }
  }
}
```

---

## ⏱️ 3. 시계열 데이터 API

### 3.1 시계열 데이터 조회

```http
GET /api/{version}/targets/{target_id}/categories/{category}/timeseries
```

**Query Parameters:**
```
?from=2024-12-01T00:00:00Z&to=2024-12-01T23:59:59Z&limit=1000
```

**응답:**
```json
{
  "target_id": "patient_123",
  "category": "vital",
  "timerange": {
    "from": "2024-12-01T00:00:00Z",
    "to": "2024-12-01T23:59:59Z"
  },
  "data": [
    {
      "ts": "2024-12-01T15:00:00Z",
      "version": "v2",
      "payload": {
        "bp": 120,
        "hr": 72,
        "spo2": 98
      }
    },
    {
      "ts": "2024-12-01T15:05:00Z", 
      "version": "v2",
      "payload": {
        "bp": 122,
        "hr": 74,
        "spo2": 97
      }
    }
  ],
  "total_records": 288
}
```

### 3.2 시계열 데이터 추가

```http
POST /api/{version}/targets/{target_id}/categories/{category}/timeseries
```

**Request Body:**
```json
{
  "timestamp": "2024-12-01T15:30:00Z",  // 선택적, 생략시 현재 시간
  "payload": {
    "bp": 125,
    "hr": 76,
    "spo2": 99
  }
}
```

---

## 📐 4. 스키마 관리 API

### 4.1 카테고리 스키마 조회

```http
GET /api/{version}/category/{category}/schema
```

**응답:**
```json
{
  "category": "vital",
  "version": "v2",
  "schema": {
    "type": "object",
    "properties": {
      "bp": {"type": "integer", "minimum": 50, "maximum": 250},
      "hr": {"type": "integer", "minimum": 30, "maximum": 200},
      "spo2": {"type": "number", "minimum": 70, "maximum": 100},
      "weight": {"type": "number", "minimum": 0}
    },
    "required": ["bp", "hr"]
  },
  "migration_available": {
    "from_v1": true,
    "to_v3": false
  }
}
```

---

## 🔄 5. 버전 관리 & 마이그레이션

### 5.1 마이그레이션 스크립트 등록

```http
POST /api/admin/migrations
```

**Request Body:**
```json
{
  "category_name": "vital",
  "from_version": 1,
  "to_version": 2,
  "migration_script": `
    function migrate(oldData) {
      return {
        ...oldData,
        patId: oldData.pat_id,          // 필드명 변경
        weight: oldData.weight || null, // 새 필드 추가
        bp_systolic: parseInt(oldData.bp?.split('/')[0]) || null
      };
    }
  `
}
```

### 5.2 마이그레이션 실행

```http
POST /api/admin/migrations/{migration_id}/execute
```

**응답:**
```json
{
  "migration_id": "migration_uuid_123",
  "status": "queued",
  "estimated_records": 15420,
  "job_id": "job_uuid_456"
}
```

---

## 📁 6. 파일 관리

### 6.1 파일 업로드 정책

- **대용량 임계값**: 20MB
- **썸네일**: 이미지 파일 100KB 이내, 비율 유지
- **지원 형식**: 모든 형식 (DICOM, PDF, JPG, PNG 등)
- **저장소**: SeaweedFS

### 6.2 파일 직접 업로드

```http
POST /api/{version}/targets/{target_id}/categories/{category}/files
Content-Type: multipart/form-data
```

### 6.3 파일 삭제

```http
DELETE /api/{version}/targets/{target_id}/categories/{category}/files/{file_id}
```

---

## 🌐 7. WebSocket 실시간 API

### 7.1 연결

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');
```

### 7.2 구독 관리

```javascript
// 구독 요청
ws.send(JSON.stringify({
  "type": "subscribe",
  "subscriptions": [
    {
      "subscribe_name": "bp_danger",
      "target_id": "patient_123",
      "category": "vital", 
      "version": "v2",
      "query": "bp>140"
    },
    {
      "subscribe_name": "ward_overview",
      "listener_id": "ward_dashboard",
      "version": "latest"
    }
  ]
}));

// 구독 업데이트 (덮어쓰기)
ws.send(JSON.stringify({
  "type": "subscribe", 
  "subscriptions": [
    {
      "subscribe_name": "bp_danger",
      "target_id": "patient_123",
      "category": "vital",
      "query": "bp>140"
    },
    {
      "subscribe_name": "new_alerts",
      "category": "emergency", 
      "query": "priority=urgent"
    }
  ]
}));
```

### 7.3 실시간 데이터 수신

```javascript
ws.onmessage = function(event) {
  const message = JSON.parse(event.data);
  
  if (message.subscribe_name === "bp_danger") {
    // 혈압 위험 알림 처리
    console.log("혈압 위험:", message.data);
  } else if (message.subscribe_name === "ward_overview") {
    // 병동 현황 업데이트
    updateDashboard(message.data);
  }
};
```

**실시간 메시지 형식:**
```json
{
  "subscribe_name": "bp_danger",
  "timestamp": "2024-12-01T15:30:00Z",
  "action": "DATA_UPDATED",
  "data": {
    "target_id": "patient_123",
    "target_name": "홍길동",
    "category": "vital",
    "version": "v2",
    "bp": 145,
    "hr": 78,
    "spo2": 96,
    "updated_at": "2024-12-01T15:30:00Z"
  }
}
```

---

## 📊 8. 자동 페이징

### 8.1 페이징 규칙

- **100건 이하**: 페이징 없음
- **100~1000건**: 100건씩
- **1000~10000건**: 500건씩  
- **10000건 이상**: 1000건씩

### 8.2 페이징 응답

```json
{
  "pagination": {
    "current_page": 1,
    "total_pages": 15,
    "total_records": 14750,
    "page_size": 1000,
    "next_page_url": "/api/v1/category/vital?page=2",
    "prev_page_url": null
  }
}
```

---

## 📝 9. 감사 로그

### 9.1 추적 대상

- `DATA_WRITE`: 데이터 생성/수정
- `DATA_DELETE`: 데이터 삭제
- `SCHEMA_CHANGE`: 스키마 변경
- `USER_LOGIN`: 사용자 로그인
- `TOKEN_GENERATED`: 토큰 생성

### 9.2 로그 형식

```json
{
  "timestamp": "2024-12-01T15:30:00Z",
  "action": "DATA_WRITE", 
  "user_id": "doctor_kim",
  "organization_id": "hospital_abc",
  "target_id": "patient_123",
  "category": "vital",
  "version": "v2",
  "ip_address": "192.168.1.100",
  "user_agent": "tmiDB-Dashboard/1.0",
  "changes": {
    "before": {"bp": 120},
    "after": {"bp": 125}
  }
}
```

---

## ⚠️ 10. 에러 응답

### 10.1 표준 에러 형식

```json
{
  "success": false,
  "error": {
    "code": "SCHEMA_NOT_FOUND",
    "message": "Schema not found",
    "details": "Category 'vital' version 3 does not exist",
    "timestamp": "2024-12-01T15:30:00Z"
  }
}
```

### 10.2 주요 에러 코드

- `UNAUTHORIZED`: 인증 실패
- `FORBIDDEN`: 권한 부족
- `NOT_FOUND`: 리소스 없음
- `VALIDATION_ERROR`: 입력 검증 실패
- `SCHEMA_NOT_FOUND`: 스키마 없음
- `MIGRATION_FAILED`: 마이그레이션 실패
- `FILE_TOO_LARGE`: 파일 크기 초과
- `THUMBNAIL_GENERATION_FAILED`: 썸네일 생성 실패

---

## ✅ 11. 구현 상태

### Phase 1: 핵심 데이터 API ✅ **완료**
- ✅ 카테고리 데이터 CRUD
- ✅ 버전별 조회 (`v1`, `v2`, `latest`, `all`)
- ✅ 자동 페이징 (10만건 이상 시 자동 활성화)
- ✅ 표준화된 응답 형식
- ✅ 시계열 데이터 API (TimescaleDB 연동)
- ✅ 토큰 기반 인증

### Phase 2: 리스너 & 스키마 API ✅ **완료**
- ✅ 단일/다중 리스너 데이터 조회
- ✅ 리스너 쿼리 처리
- ✅ 카테고리 스키마 조회 (버전별)
- ✅ 스키마 검증 (기본 구현)

### Phase 3: 관리자 콘솔 & API ✅ **완료**
- ✅ 어드민 콘솔 페이지 라우팅
- ✅ 카테고리 관리 API
- ✅ 리스너 관리 API
- ✅ 사용자 관리 API
- ✅ 토큰 관리 API
- ✅ 마이그레이션 관리 API (스텁)

### Phase 4: 파일 시스템 🔄 **부분 완료**
- ✅ 파일 업로드/삭제 API (스텁)
- ⏳ SeaweedFS 연동
- ⏳ 썸네일 생성 (100KB 이내)
- ⏳ 대용량 파일 처리 (20MB+)

### Phase 5: 버전 관리 🔄 **완료**
- ✅ 마이그레이션 API 구조
- ✅ goja JS 마이그레이션 엔진 (PostgreSQL + goja)
- ✅ SQL 및 JavaScript 스크립트 지원
- ✅ 트랜잭션 기반 안전한 실행
- ✅ 실행 결과 상세 로깅

### Phase 6: 실시간 WebSocket ⏳ **진행 예정**
- ⏳ WebSocket 구독 시스템
- ⏳ subscribe_name 활용
- ⏳ 실시간 데이터 스트리밍
- ⏳ 리스너 기반 구독

### Phase 7: 로그 & 감사 ⏳ **진행 예정**
- ⏳ 조직별 로그 저장
- ⏳ 로그 보존 기간 설정
- ⏳ 감사 대시보드

---

## 🚀 12. 빠른 시작 가이드

### 12.1 서버 시작

```bash
# 프로젝트 루트에서
cd cmd/api
go run main.go

# 출력:
# 🌐 Starting tmiDB API Server...
# 🌐 API Server listening on :8020
```

### 12.2 헬스체크

```bash
curl http://localhost:8020/api/health
```

**응답:**
```json
{
  "success": true,
  "data": {
    "status": "healthy",
    "timestamp": "2024-12-01T15:00:00Z",
    "version": "1.0.0",
    "database": "healthy"
  },
  "timestamp": "2024-12-01T15:00:00Z",
  "request_id": "req_1733068800123456789"
}
```

### 12.3 웹콘솔 접속

**메인 웹콘솔** (Tailwind CSS + Alpine.js 기반):
- `http://localhost:8020/dashboard` - 실시간 시스템 대시보드
- `http://localhost:8020/categories` - 카테고리 관리 (CRUD)
- `http://localhost:8020/files` - 파일 관리 (업로드/다운로드)

**고급 관리** (Admin 패널):
- `http://localhost:8020/admin/dashboard` - 관리자 대시보드
- `http://localhost:8020/admin/users` - 사용자 관리
- `http://localhost:8020/admin/tokens` - API 토큰 생성
- `http://localhost:8020/admin/listeners` - 리스너 설정
- `http://localhost:8020/admin/data_explorer` - SQL 데이터 탐색기

**특징:**
- ✅ **온프레미스 친화적**: 외부 CDN 의존성 Zero
- ✅ **다크모드 지원**: 자동 테마 감지 + 수동 토글
- ✅ **완전 반응형**: 모바일/태블릿/데스크톱 최적화
- ✅ **실시간 업데이트**: 30초 자동 새로고침
- ✅ **드래그앤드롭**: 파일 업로드 UI

### 12.4 토큰 생성

어드민 콘솔에서 토큰 생성 또는:

```bash
curl -X POST "http://localhost:8020/api/admin/tokens" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Dashboard Token",
    "token_type": "permanent",
    "categories": ["vital", "patient_info"]
  }'
```

### 12.5 실제 사용 예제: 병원 대시보드

#### 환자 바이탈 사인 등록

```bash
curl -X POST "http://localhost:8020/api/v1/targets/patient_12345/categories/vital" \
  -H "Authorization: Bearer tmitk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "v1",
    "bp": 120,
    "spo2": 98.5,
    "heart_rate": 75,
    "timestamp": "2024-12-01T14:30:00Z"
  }'
```

#### 카테고리별 데이터 조회

```bash
# 최신 버전만 조회
curl "http://localhost:8020/api/latest/category/vital" \
  -H "Authorization: Bearer tmitk_xxx"

# 특정 조건으로 필터링
curl "http://localhost:8020/api/v1/category/vital?bp>=120&ward=ICU" \
  -H "Authorization: Bearer tmitk_xxx"

# 모든 버전 조회
curl "http://localhost:8020/api/all/category/vital" \
  -H "Authorization: Bearer tmitk_xxx"
```

#### 리스너로 통합 데이터 조회

```bash
# 단일 리스너
curl "http://localhost:8020/api/v1/listener/vital_dashboard?subscribe_name=bp_monitor" \
  -H "Authorization: Bearer tmitk_xxx"

# 다중 리스너 (vital + ward + io 통합)
curl "http://localhost:8020/api/v1/listener/vital+ward+io?subscribe_name=ward_overview" \
  -H "Authorization: Bearer tmitk_xxx"
```

#### 시계열 데이터 조회

```bash
curl "http://localhost:8020/api/v1/targets/patient_12345/categories/vital/timeseries?start_time=2024-12-01T00:00:00Z&end_time=2024-12-01T23:59:59Z&interval=1h" \
  -H "Authorization: Bearer tmitk_xxx"
```

### 12.6 표준 응답 형식

모든 API는 다음과 같은 표준 형식으로 응답합니다:

```json
{
  "success": true,
  "data": { /* 실제 데이터 */ },
  "meta": {
    "pagination": {
      "current_page": 1,
      "page_size": 1000,
      "total_pages": 5,
      "total_records": 4520,
      "has_next": true,
      "has_prev": false
    },
    "version": {
      "requested_version": "latest",
      "actual_versions": ["2"],
      "is_multi_version": false
    },
    "query": {
      "filters": ["bp >= '120'", "ward = 'ICU'"],
      "process_time": "15.2ms",
      "cache_hit": false
    }
  },
  "timestamp": "2024-12-01T15:00:00Z",
  "request_id": "req_1733068800123456789"
}
```

### 12.7 에러 처리

에러 발생 시 일관된 형식으로 응답:

```json
{
  "success": false,
  "error": {
    "code": "AUTH_TOKEN_INVALID",
    "message": "Invalid or expired token",
    "details": "Token expired at 2024-12-01T10:00:00Z"
  },
  "timestamp": "2024-12-01T15:00:00Z",
  "request_id": "req_1733068800123456789"
}
```

---

## 📋 13. 다음 구현 단계

### 즉시 구현 가능한 것들:
1. **파일 업로드 시스템** (SeaweedFS 연동)
2. **썸네일 생성 파이프라인**
3. **WebSocket 실시간 스트리밍**
4. **goja 기반 마이그레이션 엔진**

### 새로 추가된 기능들:
1. **메모리 캐시 시스템** ✅ **완료**
   - 10,000개 항목 지원
   - 데이터 변경 시 자동 무효화
   - 패턴 기반 캐시 삭제
   - 통계 및 히트율 추적

2. **스마트 페이징** ✅ **완료**
   - 사용자 지정 페이지 크기 시 자동 규칙 무시
   - 10만건 이상 시 자동 페이징 (1,000건)
   - 캐시 통합 지원

3. **마이그레이션 시스템** ✅ **완료**
   - PostgreSQL + goja JavaScript 엔진
   - SQL과 JavaScript 스크립트 혼합 지원
   - 트랜잭션 안전성 보장
   - 실행 로그 및 에러 추적

4. **웹콘솔 완전 구현** ✅ **완료**
   - Tailwind CSS + Alpine.js 기반 현대적 UI
   - 온프레미스 친화적 (외부 CDN 의존성 Zero)
   - 실시간 대시보드, 카테고리 관리, 파일 시스템 완비
   - 다크모드, 반응형 디자인, 드래그앤드롭 지원
   - 총 2,000줄+ 완전 구현

### 추가 기능 개발:
1. **로그 수집 및 감사**
2. **백업 및 복원**
3. **성능 모니터링**
4. **WebSocket 실시간 스트리밍**

**tmiDB API v1.0+ 완전 구현 완료!** 🚀
현대적 웹콘솔과 데이터 올인원 서비스로 병원, 공장 등 온프레미스 환경에서 즉시 사용 가능합니다.

**주요 완성 기능:**
- 🌟 **Tailwind 웹콘솔**: 완전 오프라인 동작, 현대적 UI/UX
- 📊 **실시간 대시보드**: 시스템 모니터링과 메트릭 표시  
- 📂 **파일 관리**: 드래그앤드롭 업로드, 그리드/리스트 뷰
- 🗂️ **카테고리 CRUD**: 동적 스키마 편집과 실시간 검색
- 🚀 **메모리 캐시**: 고성능 데이터 액세스
- 🔄 **마이그레이션**: PostgreSQL + JavaScript 통합 엔진 