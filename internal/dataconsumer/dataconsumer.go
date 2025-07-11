package dataconsumer

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

// DataConsumer 데이터 소비 및 처리를 담당하는 구조체
type DataConsumer struct {
	*busconsumer.BaseConsumer
}

// DataPoint 처리할 데이터 포인트 구조체
type DataPoint struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Source    string         `json:"source"`
	Category  string         `json:"category"`
	Data      map[string]any `json:"data"`
}

// New DataConsumer 인스턴스를 생성합니다
func New() *DataConsumer {
	dc := &DataConsumer{}

	// Go 1.24 기능: 자동 정리를 위한 cleanup 등록
	runtime.SetFinalizer(dc, func(consumer *DataConsumer) {
		if consumer.BaseConsumer != nil {
			consumer.Cleanup()
		}
	})

	return dc
}

// Start DataConsumer를 시작합니다
func (dc *DataConsumer) Start(ctx context.Context) error {
	log.Println("🔄 Initializing Data Consumer...")

	// 데이터베이스 연결
	if err := dc.connectDatabase(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// 기본 소비자 생성
	base, err := busconsumer.NewBaseConsumer(ctx, database.CoreDB)
	if err != nil {
		return fmt.Errorf("failed to create base consumer: %w", err)
	}
	dc.BaseConsumer = base

	// 데이터 구독 시작
	if err := dc.StartSubscriptions(dc.handleDataMessage, dc.handleSystemMetrics); err != nil {
		return fmt.Errorf("failed to start subscriptions: %w", err)
	}

	// 배치 처리 시작
	go dc.StartBatchProcessor()

	log.Println("✅ Data Consumer started successfully")

	// 컨텍스트 완료까지 대기
	<-dc.Ctx.Done()

	// 정리 작업은 finalizer 또는 명시적 호출에 의해 수행됩니다.

	return nil
}

// connectDatabase 데이터베이스에 연결합니다
func (dc *DataConsumer) connectDatabase() error {
	for i := 0; i < 15; i++ {
		// 전역 DB 변수 확인
		if database.CoreDB == nil {
			log.Printf("⏳ Data Consumer: database.CoreDB is nil (attempt %d/15)", i+1)
		} else {
			// DB 연결 상태 확인
			if err := database.CheckDatabaseHealth(); err != nil {
				log.Printf("⏳ Data Consumer: database health check failed - %v (attempt %d/15)", err, i+1)
			} else {
				log.Println("✅ Data Consumer connected to database")
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
func (dc *DataConsumer) handleDataMessage(msg *nats.Msg) {
	var dataPoint busconsumer.DataPoint
	if err := json.Unmarshal(msg.Data, &dataPoint); err != nil {
		log.Printf("❌ DataConsumer: Failed to unmarshal data message: %v", err)
		return
	}

	log.Printf("📨 DataConsumer received data: %s from %s.%s", dataPoint.ID, dataPoint.Source, dataPoint.Category)

	// 데이터베이스에 저장
	if err := dc.SaveToDatabase(dataPoint); err != nil {
		log.Printf("❌ DataConsumer: Failed to save data to database: %v", err)
		return
	}

	log.Printf("💾 DataConsumer saved data: %s", dataPoint.ID)
}

// handleSystemMetrics 시스템 메트릭을 처리합니다
func (dc *DataConsumer) handleSystemMetrics(msg *nats.Msg) {
	var dataPoint busconsumer.DataPoint
	if err := json.Unmarshal(msg.Data, &dataPoint); err != nil {
		log.Printf("❌ DataConsumer: Failed to unmarshal system metrics: %v", err)
		return
	}

	log.Printf("📊 DataConsumer processing system metrics: %s", dataPoint.ID)

	// 시스템 메트릭 특별 처리
	if err := dc.processSystemMetrics(dataPoint); err != nil {
		log.Printf("❌ DataConsumer: Failed to process system metrics: %v", err)
		return
	}

	// 데이터베이스에 저장
	if err := dc.SaveToDatabase(dataPoint); err != nil {
		log.Printf("❌ DataConsumer: Failed to save system metrics: %v", err)
		return
	}

	log.Printf("📈 DataConsumer processed and saved system metrics: %s", dataPoint.ID)
}

// processSystemMetrics 시스템 메트릭을 특별 처리합니다
func (dc *DataConsumer) processSystemMetrics(dataPoint busconsumer.DataPoint) error {
	// CPU 사용률이 90% 이상인 경우 알림
	if cpuUsage, ok := dataPoint.Data["cpu_usage"].(float64); ok && cpuUsage > 90.0 {
		log.Printf("⚠️ HIGH CPU USAGE ALERT: %.1f%%", cpuUsage)
		// 여기서 알림 시스템으로 메시지를 보낼 수 있습니다
	}

	// 메모리 사용률이 85% 이상인 경우 알림
	if memUsage, ok := dataPoint.Data["memory_usage"].(float64); ok && memUsage > 85.0 {
		log.Printf("⚠️ HIGH MEMORY USAGE ALERT: %.1f%%", memUsage)
	}

	return nil
}
