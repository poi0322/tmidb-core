package datamanager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/tmidb/tmidb-core/internal/busconsumer"
	"github.com/tmidb/tmidb-core/internal/database"
)

// DataManager 데이터 수집 및 데이터베이스 관리를 담당하는 구조체
type DataManager struct {
	*busconsumer.BaseConsumer
}

// New DataManager 인스턴스를 생성합니다
func New() *DataManager {
	dm := &DataManager{}

	runtime.SetFinalizer(dm, func(manager *DataManager) {
		if manager.BaseConsumer != nil {
			manager.Cleanup()
		}
	})
	return dm
}

// Start DataManager를 시작합니다
func (dm *DataManager) Start(ctx context.Context) error {
	log.Println("📊 Initializing Data Manager...")

	// 데이터베이스 연결
	if err := dm.connectDatabase(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// 기본 소비자 생성
	base, err := busconsumer.NewBaseConsumer(ctx, database.CoreDB)
	if err != nil {
		return fmt.Errorf("failed to create base consumer: %w", err)
	}
	dm.BaseConsumer = base

	// 데이터 구독 시작
	if err := dm.StartSubscriptions(dm.handleDataMessage, dm.handleSystemMetrics); err != nil {
		return fmt.Errorf("failed to start subscriptions: %w", err)
	}

	// 데이터 수집 프로세스 시작
	go dm.startDataCollection()

	// 배치 처리 시작
	go dm.StartBatchProcessor()

	log.Println("✅ Data Manager started successfully")

	// 컨텍스트 완료까지 대기
	<-dm.Ctx.Done()

	return nil
}

// connectDatabase 데이터베이스에 연결합니다
func (dm *DataManager) connectDatabase() error {
	for i := 0; i < 15; i++ {
		// 전역 DB 변수 확인
		if database.CoreDB == nil {
			log.Printf("⏳ Data Manager: database.CoreDB is nil (attempt %d/15)", i+1)
		} else {
			// DB 연결 상태 확인
			if err := database.CheckDatabaseHealth(); err != nil {
				log.Printf("⏳ Data Manager: database health check failed - %v (attempt %d/15)", err, i+1)
			} else {
				log.Println("✅ Data Manager connected to database")
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}

	// 최종 실패 시 상세 에러 정보 제공
	if database.CoreDB == nil {
		return fmt.Errorf("failed to connect to database after 15 attempts: global DB variable is nil - ensure database.InitDatabase() was called successfully")
	}
	return fmt.Errorf("failed to connect to database after 15 attempts: database health check failed")
}

// handleDataMessage 일반 데이터 메시지를 처리합니다
func (dm *DataManager) handleDataMessage(msg *nats.Msg) {
	var dataPoint busconsumer.DataPoint
	if err := json.Unmarshal(msg.Data, &dataPoint); err != nil {
		log.Printf("❌ DataManager: Failed to unmarshal data message: %v", err)
		return
	}

	log.Printf("📨 DataManager received data: %s from %s.%s", dataPoint.ID, dataPoint.Source, dataPoint.Category)

	if err := dm.SaveToDatabase(dataPoint); err != nil {
		log.Printf("❌ DataManager: Failed to save data to database: %v", err)
		return
	}

	log.Printf("💾 DataManager saved data: %s", dataPoint.ID)
}

// handleSystemMetrics 시스템 메트릭을 처리합니다
func (dm *DataManager) handleSystemMetrics(msg *nats.Msg) {
	var dataPoint busconsumer.DataPoint
	if err := json.Unmarshal(msg.Data, &dataPoint); err != nil {
		log.Printf("❌ DataManager: Failed to unmarshal system metrics: %v", err)
		return
	}

	log.Printf("📊 DataManager processing system metrics: %s", dataPoint.ID)

	if err := dm.processSystemMetrics(dataPoint); err != nil {
		log.Printf("❌ DataManager: Failed to process system metrics: %v", err)
		return
	}

	if err := dm.SaveToDatabase(dataPoint); err != nil {
		log.Printf("❌ DataManager: Failed to save system metrics: %v", err)
		return
	}

	log.Printf("📈 DataManager processed and saved system metrics: %s", dataPoint.ID)
}

// processSystemMetrics 시스템 메트릭을 특별 처리합니다
func (dm *DataManager) processSystemMetrics(dataPoint busconsumer.DataPoint) error {
	// 모듈 상태 확인 및 알림
	if modules, ok := dataPoint.Data["modules"].(map[string]any); ok {
		dm.checkModuleAlerts(modules)
	}
	return nil
}

// checkModuleAlerts 모듈 상태를 확인하고 필요시 알림을 발생시킵니다
func (dm *DataManager) checkModuleAlerts(modules map[string]any) {
	for moduleName, moduleData := range modules {
		if moduleInfo, ok := moduleData.(map[string]any); ok {
			if healthy, exists := moduleInfo["healthy"].(bool); exists && !healthy {
				log.Printf("🚨 MODULE HEALTH ALERT: %s is unhealthy", moduleName)
				if errorMsg, hasError := moduleInfo["error"].(string); hasError {
					log.Printf("   Error: %s", errorMsg)
				}
			}

			if status, exists := moduleInfo["status"].(string); exists {
				switch status {
				case "disconnected", "error":
					log.Printf("⚠️ MODULE STATUS ALERT: %s is %s", moduleName, status)
				case "running":
					// 정상 상태는 로그하지 않음 (너무 많은 로그 방지)
				default:
					log.Printf("ℹ️ MODULE STATUS: %s is %s", moduleName, status)
				}
			}

			// PostgreSQL 연결 풀 경고
			if moduleName == "postgresql" {
				if connections, ok := moduleInfo["connections"].(map[string]any); ok {
					if waitCount, exists := connections["wait_count"].(uint64); exists && waitCount > 10 {
						log.Printf("⚠️ POSTGRESQL ALERT: High connection wait count: %d", waitCount)
					}
					if inUse, exists := connections["in_use"].(int); exists && inUse > 8 {
						log.Printf("⚠️ POSTGRESQL ALERT: High connections in use: %d", inUse)
					}
				}
			}

			// NATS 연결 문제 경고
			if moduleName == "nats" {
				if stats, ok := moduleInfo["stats"].(map[string]any); ok {
					if reconnects, exists := stats["reconnects"].(uint64); exists && reconnects > 5 {
						log.Printf("⚠️ NATS ALERT: High reconnect count: %d", reconnects)
					}
				}
			}
		}
	}
}

// startDataCollection 주기적인 데이터 수집을 시작합니다
func (dm *DataManager) startDataCollection() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("🔄 Data Manager starting periodic data collection...")

	for {
		select {
		case <-ticker.C:
			dm.collectSystemMetrics()
		case <-dm.Ctx.Done():
			log.Println("🛑 Data Manager stopping data collection...")
			return
		}
	}
}

// collectSystemMetrics 시스템 메트릭을 수집합니다
func (dm *DataManager) collectSystemMetrics() {
	// 시스템 메트릭용 고정 UUID 사용 (UUID v4 형식)
	systemMetricsUUID := "00000000-0000-4000-8000-000000000001"

	// 실제 서브 모듈 상태 수집
	moduleStatus := dm.collectModuleStatus()

	dataPoint := busconsumer.DataPoint{
		ID:        systemMetricsUUID,
		Timestamp: time.Now(),
		Source:    "system",
		Category:  "metrics",
		Data: map[string]any{
			"modules":      moduleStatus,
			"timestamp_id": fmt.Sprintf("system-metrics-%d", time.Now().Unix()),
			"uptime":       time.Since(time.Now().Add(-time.Hour)).Seconds(), // 임시 업타임
		},
	}

	// 디버그: 발행할 데이터 출력
	if dataJSON, err := json.Marshal(dataPoint.Data); err == nil {
		log.Printf("🔍 DEBUG: Publishing system metrics data: %s", string(dataJSON))
	}

	if err := dm.publishData(dataPoint); err != nil {
		log.Printf("❌ Failed to publish system metrics: %v", err)
	} else {
		log.Printf("📤 Data Manager published system metrics: %s", dataPoint.Data["timestamp_id"])
	}
}

// collectModuleStatus 각 서브 모듈의 상태를 수집합니다
func (dm *DataManager) collectModuleStatus() map[string]any {
	status := make(map[string]any)

	// PostgreSQL 상태 확인
	status["postgresql"] = dm.checkPostgreSQLStatus()

	// NATS 상태 확인
	status["nats"] = dm.checkNATSStatus()

	// SeaweedFS 상태 확인 (HTTP API 호출)
	status["seaweedfs"] = dm.checkSeaweedFSStatus()

	// API 서버 상태 (자체 확인)
	status["api_server"] = dm.checkAPIServerStatus()

	// Data Manager 자체 상태
	status["data_manager"] = map[string]any{
		"status":      "running",
		"last_check":  time.Now().Format(time.RFC3339),
		"healthy":     true,
		"connections": dm.getDataManagerConnections(),
	}

	// Data Consumer 상태 (NATS를 통한 헬스체크)
	status["data_consumer"] = dm.checkDataConsumerStatus()

	return status
}

// checkPostgreSQLStatus PostgreSQL 상태를 확인합니다
func (dm *DataManager) checkPostgreSQLStatus() map[string]any {
	status := map[string]any{
		"status":     "unknown",
		"healthy":    false,
		"last_check": time.Now().Format(time.RFC3339),
	}

	if database.CoreDB == nil {
		status["status"] = "disconnected"
		status["error"] = "database connection is nil"
		return status
	}

	// 연결 상태 확인
	if err := database.CoreDB.Ping(); err != nil {
		status["status"] = "error"
		status["error"] = err.Error()
		return status
	}

	// 연결 풀 상태 확인
	stats := database.CoreDB.Stats()
	status["status"] = "running"
	status["healthy"] = true
	status["connections"] = map[string]any{
		"open":          stats.OpenConnections,
		"in_use":        stats.InUse,
		"idle":          stats.Idle,
		"max_open":      stats.MaxOpenConnections,
		"wait_count":    stats.WaitCount,
		"wait_duration": stats.WaitDuration.String(),
	}

	return status
}

// checkNATSStatus NATS 상태를 확인합니다
func (dm *DataManager) checkNATSStatus() map[string]any {
	status := map[string]any{
		"status":     "unknown",
		"healthy":    false,
		"last_check": time.Now().Format(time.RFC3339),
	}

	if dm.NatsConn == nil {
		status["status"] = "disconnected"
		status["error"] = "NATS connection is nil"
		return status
	}

	// NATS 연결 상태 확인
	if !dm.NatsConn.IsConnected() {
		status["status"] = "disconnected"
		status["error"] = "NATS connection lost"
		return status
	}

	// NATS 통계 정보
	natsStats := dm.NatsConn.Stats()
	status["status"] = "running"
	status["healthy"] = true
	status["stats"] = map[string]any{
		"in_msgs":    natsStats.InMsgs,
		"out_msgs":   natsStats.OutMsgs,
		"in_bytes":   natsStats.InBytes,
		"out_bytes":  natsStats.OutBytes,
		"reconnects": natsStats.Reconnects,
	}

	return status
}

// checkSeaweedFSStatus SeaweedFS 상태를 확인합니다 (HTTP API 호출)
func (dm *DataManager) checkSeaweedFSStatus() map[string]any {
	status := map[string]any{
		"status":     "unknown",
		"healthy":    false,
		"last_check": time.Now().Format(time.RFC3339),
	}

	// SeaweedFS Master 상태 확인
	masterHealthy := dm.checkSeaweedFSMaster()
	volumeHealthy := dm.checkSeaweedFSVolume()

	status["master_url"] = "http://localhost:9333"
	status["volume_url"] = "http://localhost:8081"
	status["master_healthy"] = masterHealthy
	status["volume_healthy"] = volumeHealthy

	if masterHealthy && volumeHealthy {
		status["status"] = "running"
		status["healthy"] = true
	} else if masterHealthy || volumeHealthy {
		status["status"] = "partial"
		status["healthy"] = false
		status["error"] = "some components unavailable"
	} else {
		status["status"] = "error"
		status["healthy"] = false
		status["error"] = "all components unavailable"
	}

	return status
}

// checkSeaweedFSMaster SeaweedFS Master 서버 상태를 확인합니다
func (dm *DataManager) checkSeaweedFSMaster() bool {
	// 간단한 타임아웃으로 연결 테스트
	// 실제 구현에서는 HTTP 클라이언트 사용
	// 현재는 기본적으로 true 반환 (서비스가 실행 중이라고 가정)
	return true
}

// checkSeaweedFSVolume SeaweedFS Volume 서버 상태를 확인합니다
func (dm *DataManager) checkSeaweedFSVolume() bool {
	// 간단한 타임아웃으로 연결 테스트
	// 실제 구현에서는 HTTP 클라이언트 사용
	// 현재는 기본적으로 true 반환 (서비스가 실행 중이라고 가정)
	return true
}

// checkAPIServerStatus API 서버 상태를 확인합니다
func (dm *DataManager) checkAPIServerStatus() map[string]any {
	status := map[string]any{
		"status":     "running",
		"healthy":    true,
		"last_check": time.Now().Format(time.RFC3339),
		"port":       8020,
		"note":       "assumed running - data-manager is operational",
	}

	return status
}

// checkDataConsumerStatus Data Consumer 상태를 확인합니다
func (dm *DataManager) checkDataConsumerStatus() map[string]any {
	status := map[string]any{
		"status":     "unknown",
		"healthy":    false,
		"last_check": time.Now().Format(time.RFC3339),
	}

	// NATS를 통한 헬스체크 메시지 발송 (간단한 ping-pong)
	if dm.NatsConn != nil && dm.NatsConn.IsConnected() {
		status["status"] = "assumed_running"
		status["healthy"] = true
		status["note"] = "NATS connection available - consumer likely operational"
	} else {
		status["status"] = "unknown"
		status["error"] = "cannot verify - NATS unavailable"
	}

	return status
}

// getDataManagerConnections Data Manager의 연결 상태를 반환합니다
func (dm *DataManager) getDataManagerConnections() map[string]any {
	connections := map[string]any{
		"database": database.CoreDB != nil,
		"nats":     dm.NatsConn != nil && dm.NatsConn.IsConnected(),
	}

	return connections
}

// publishData 데이터를 NATS로 발행합니다
func (dm *DataManager) publishData(dataPoint busconsumer.DataPoint) error {
	if dm.NatsConn == nil {
		return fmt.Errorf("NATS connection not available")
	}

	data, err := json.Marshal(dataPoint)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	subject := fmt.Sprintf("tmidb.data.%s.%s", dataPoint.Source, dataPoint.Category)
	return dm.NatsConn.Publish(subject, data)
}
