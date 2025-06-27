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
	base, err := busconsumer.NewBaseConsumer(ctx, database.DB)
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
		if err := database.CheckDatabaseHealth(); err == nil {
			log.Println("✅ Data Manager connected to database")
			return nil
		}
		log.Printf("⏳ Data Manager waiting for database... (attempt %d/15)", i+1)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("failed to connect to database after 15 attempts")
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
	if cpuUsage, ok := dataPoint.Data["cpu_usage"].(float64); ok && cpuUsage > 90.0 {
		log.Printf("⚠️ HIGH CPU USAGE ALERT: %.1f%%", cpuUsage)
	}
	if memUsage, ok := dataPoint.Data["memory_usage"].(float64); ok && memUsage > 85.0 {
		log.Printf("⚠️ HIGH MEMORY USAGE ALERT: %.1f%%", memUsage)
	}
	return nil
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
	dataPoint := busconsumer.DataPoint{
		ID:        fmt.Sprintf("system-metrics-%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Source:    "system",
		Category:  "metrics",
		Data: map[string]interface{}{
			"cpu_usage":    85.5,
			"memory_usage": 67.2,
			"disk_usage":   45.8,
			"network_io":   1024.0,
		},
	}

	if err := dm.publishData(dataPoint); err != nil {
		log.Printf("❌ Failed to publish system metrics: %v", err)
	} else {
		log.Printf("📤 Data Manager published system metrics: %s", dataPoint.ID)
	}
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
