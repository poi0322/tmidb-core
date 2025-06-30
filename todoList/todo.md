# TODO - tmiDB CLI 개선

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
- [ ] **Backup 기능 검증** - create/restore/verify 전체 테스트
- [ ] **Diagnose 기능 완성** - performance/connectivity/fix 구현
- [ ] **로그 필터링** - level/pattern/time 기반 필터 구현
- [ ] **에러 메시지 개선** - 사용자 친화적 메시지로 변경

## 📋 일반 (언젠가)
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
- **완료**: 8/8 긴급+중요 항목 (100%) 🎉
- **전체**: 9/16 항목 (56%)
- **주요 성과**: Air 재시작 문제 완전 해결, 서비스 권한 관리 시스템 구축

---
**업데이트**: 2024-12-30 (3개 긴급 항목 완료) 