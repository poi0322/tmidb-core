# TODO - tmiDB CLI 개선

## 🔥 긴급 (현재) - 2024-12-30 추가 발견 문제

- [ ] **CLI 응답 지연 문제 해결** - CLI 명령어 실행 시 30초 지연 발생 (뮤텍스 데드락 의심)
  - ✅ process manager에서 뮤텍스 사용 최소화 완료 (StartProcess, StopProcess, RestartProcess, GetProcessList, UpdateProcessStats)
  - ✅ IPC client에서 뮤텍스 사용 최소화 완료 (sendMessage)
  - ❌ 여전히 30초 지연 발생 - 추가 조사 필요
- [ ] **프로세스 상태 동기화 문제** - 실제 실행 중인 프로세스를 supervisor가 error로 인식
  - ✅ API 프로세스는 실제로 실행 중 (PID 1123, 3778)
  - ❌ supervisor가 프로세스 상태를 제대로 추적하지 못함
  - ❌ 프로세스 재시작 시 기존 프로세스와 충돌 가능성
- [ ] **메모리 사용량 0B 문제 재발** - 뮤텍스 최적화 후에도 메모리 표시 안됨
  - ❌ 모든 내부 프로세스에서 메모리 0B로 표시
  - ✅ 외부 서비스(PostgreSQL, NATS, SeaweedFS)는 정상 표시

## 🔥 긴급 (이번 주) - 모두 완료! 🎉

- [x] **시스템 모니터링 수정** - monitor system에서 0 0 0 0 출력 문제 해결
  - ✅ supervisor의 handleGetSystemResources에서 실제 CPU/메모리/디스크 사용량 계산 구현
  - ✅ SystemStats 응답에서 프로세스 카운트가 실제 프로세스 수를 반영하도록 수정
  - ✅ CLI 출력 형식을 표 형태로 개선하여 시간, 프로세스, 리소스 정보 표시
- [x] **프로세스 등록 구현** - supervisor 시작 시 기본 프로세스들 자동 등록
  - ✅ api, data-manager, data-consumer, postgresql, nats, seaweedfs 프로세스 등록
  - ✅ process manager의 빈 목록 문제 해결 (이미 구현되어 있음)
- [x] **Config IPC 구현** - supervisor에 config 메시지 타입 추가
  - ✅ handleConfigGet, handleConfigSet, handleConfigList 구현
  - ✅ handleConfigReset, handleConfigImport, handleConfigValidate 구현
  - ✅ 모든 config 명령어 IPC 핸들러 등록 완료
- [x] **메모리 사용량 수정** - 프로세스 메모리 0B 문제 해결
  - ✅ /proc/[pid]/status에서 VmRSS 읽어서 실제 메모리 사용량 계산
  - ✅ 시스템 서비스는 systemctl로 PID 조회 후 메모리 계산
  - ✅ updateProcessStats 함수에서 실시간 통계 업데이트
- [x] **출력 형식 통일** - 모든 명령어에 JSON/YAML 지원
  - ✅ status, monitor health, monitor services 명령어에 --output 플래그 추가
  - ✅ OutputFormatter 사용하여 json, json-pretty, yaml 출력 지원
  - ✅ 구조화된 데이터 형태로 일관된 출력 제공
- [x] **시스템 서비스 연동** - PostgreSQL/NATS/SeaweedFS를 systemctl로 관리
  - ✅ 외부 서비스를 TypeService로 변경하여 systemd 연동
  - ✅ isServiceRunning, startSystemService, getServicePID 함수 구현
  - ✅ Air 재시작 문제 해결 (시스템 서비스는 독립적으로 실행)

## ⚡ 중요 (이번 달)

- [x] **tmiDB API 시스템 강화** - API 명세 기반 완전 개발 가능한 수준으로 향상 ✅ **2024-07-01 완료**
  - ✅ 웹콘솔 경로 단순화 (`/dashboard`, `/categories`, `/listeners`)
  - ✅ PostgreSQL + goja 마이그레이션 시스템 완전 구현 (508줄)
  - ✅ 메모리 캐시 시스템 구현 (468줄, 추가 의존성 없음)
  - ✅ 스마트 페이징 (사용자 지정 페이지 크기 우선순위)
  - ✅ 데이터 API 캐시 통합 및 자동 무효화
  - ✅ 총 1,441줄 코드 추가로 올인원 데이터 서비스 완성

- [x] **컨테이너 시작 시 서비스 자동 실행** - SQL과 코어서비스 컨테이너 시작 시 실행
  - ✅ Dockerfile/Dockerfile.dev에 entrypoint 스크립트 추가
  - ✅ PostgreSQL, NATS, SeaweedFS 백그라운드 자동 시작
  - ✅ PID 파일 저장으로 supervisor가 attach 가능
  - ✅ AttachProcess 메소드 구현으로 기존 프로세스 모니터링
- [x] **CLI 서비스 권한 관리** - 서비스 시작/정지/재시작 및 로그 접근 권한 관리
  - ✅ service 명령어 그룹 추가 (list, control, logs)
  - ✅ 서비스별 권한 표시 및 제어 기능
  - ✅ 실시간 로그 스트리밍 및 히스토리 조회
  - ✅ JSON/YAML 출력 지원으로 권한 정보 구조화
- [x] **실제 프로세스 통계 구현** - process manager의 updateProcessStats에서 실제 CPU/메모리 사용량 계산
  - ✅ /proc/[pid]/stat 파일 읽기 또는 시스템 API 사용
  - ✅ 실시간 리소스 모니터링 구현
- [x] **뮤텍스 최적화** - CLI 응답 지연 문제 해결을 위한 뮤텍스 사용 최소화
  - ✅ process manager의 모든 주요 함수에서 뮤텍스 보유 시간 최소화
  - ✅ IPC client에서 뮤텍스 사용 패턴 개선
  - ✅ 데이터 복사를 통한 뮤텍스 외부 처리 구현
- [ ] **프로세스 재시작 기능 완전 수정** - 현재 restart는 작동하지만 프로세스 상태 추적 문제
  - ✅ restart 명령어 자체는 정상 작동 확인
  - ❌ 프로세스 상태가 error로 잘못 표시되는 문제 해결 필요
- [x] **Backup 기능 검증** - create/restore/verify 전체 테스트
- [ ] **Diagnose 기능 완성** - performance/connectivity/fix 구현
- [ ] **로그 필터링** - level/pattern/time 기반 필터 구현
- [ ] **에러 메시지 개선** - 사용자 친화적 메시지로 변경

## 🔥 긴급 (현재 진행) - 웹콘솔 기능 구현

- [x] **웹콘솔 기본 구조 구축** ✅ **2025-01-01 완료**
  - ✅ 반응형 레이아웃 시스템 구현 (435줄) - Bootstrap + 모바일 친화적
  - ✅ 대시보드 페이지 완성 (624줄) - 실시간 모니터링, Chart.js 통계
  - ✅ 카테고리 관리 페이지 (570줄) - CRUD, 스키마 편집, 실시간 검색
  - ✅ 대시보드 API 6개 엔드포인트 구현 (169줄 추가)
- [x] **웹콘솔 Tailwind CSS 변환** ✅ **2025-01-01 완료**
  - ✅ 레이아웃 시스템 완전 재작성 (Bootstrap → Tailwind + Alpine.js)
  - ✅ 다크모드 지원 및 모바일 친화적 사이드바 구현
  - ✅ 대시보드 페이지 Tailwind 변환 (Alpine.js 상태 관리)
  - ✅ 카테고리 관리 페이지 Tailwind 변환 (함수형 컴포넌트)
  - ✅ 온프레미스 친화적 (외부 CDN 의존성 제거, 로컬 라이브러리)
- [x] **파일 관리 시스템 완성** ✅ **2025-01-01 완료**
  - ✅ 드래그앤드롭 업로드 지원 (멀티파일)
  - ✅ 그리드/리스트 뷰 전환 및 파일 검색/정렬
  - ✅ 파일 다운로드/삭제 기능 완비
  - ✅ 파일 타입별 아이콘 및 업로드 진행률 표시
  - ✅ 완전한 Tailwind + Alpine.js 기반 UI
- [ ] **로그 수집 및 감사 시스템** - DB 기반으로 며칠치 보관
  - [ ] 수정/삭제 로그 테이블 생성
  - [ ] 자동 정리 정책 구현
  - [ ] 감사 대시보드 구현
  - [ ] 로그 필터링 및 검색

## 📋 일반 (언젠가)

- [ ] **성능 최적화** - 하나로 합쳐진 시스템 최적화
  - [ ] 쿼리 최적화 (인덱스, 실행계획)
  - [ ] 캐시 효율성 개선
  - [ ] 메모리 관리 최적화
  - [ ] 응답 속도 향상
- [ ] **CLI 도움말 개선** - 각 명령어별 상세 사용법 추가
- [ ] **자동 완성 지원** - bash/zsh completion 스크립트 생성
- [ ] **설정 파일 지원** - YAML/JSON 설정 파일 읽기
- [ ] **플러그인 시스템** - 확장 가능한 명령어 구조
- [ ] **국제화 지원** - 다국어 메시지 지원

## 🧪 테스트 필요

- [ ] **통합 테스트** - supervisor 연동 전체 검증
- [ ] **성능 테스트** - 대용량 데이터 처리
- [ ] **보안 테스트** - 권한/암호화 검증

---

## 📊 진행 상황

- **완료**: 13/15 긴급+중요 항목 (87%)
- **전체**: 14/25 항목 (56%)
- **웹콘솔**: 완전 구현 완료 (2,000줄+ Tailwind + Alpine.js)
- **현재 긴급**: 로그 시스템 구현
- **기존 긴급 문제**: 3개 (CLI 지연, 프로세스 상태 동기화, 메모리 표시)

---

**업데이트**: 2025-01-01 (웹콘솔 Tailwind 변환 + 파일관리 완성! 현대적 UI 완료)
