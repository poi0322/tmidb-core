package busconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/tmidb/tmidb-core/internal/database"
)

// DataPoint 수집되는 데이터 포인트 구조체
type DataPoint struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Source    string         `json:"source"`
	Category  string         `json:"category"`
	Data      map[string]any `json:"data"`
}

// BaseConsumer는 NATS 메시지 소비자의 공통 로직을 포함합니다.
type BaseConsumer struct {
	NatsConn *nats.Conn
	DB       database.DBTX
	Subs     []*nats.Subscription
	Ctx      context.Context
	Cancel   context.CancelFunc
}

// NewBaseConsumer는 새로운 BaseConsumer 인스턴스를 생성합니다.
func NewBaseConsumer(ctx context.Context, db database.DBTX) (*BaseConsumer, error) {
	childCtx, cancel := context.WithCancel(ctx)
	consumer := &BaseConsumer{
		DB:     db,
		Ctx:    childCtx,
		Cancel: cancel,
	}
	if err := consumer.connectNATS(); err != nil {
		cancel()
		return nil, err
	}
	return consumer, nil
}

// ConnectNATS NATS 서버에 연결합니다.
func (bc *BaseConsumer) connectNATS() error {
	var err error
	for i := 0; i < 10; i++ {
		bc.NatsConn, err = nats.Connect(getNatsURL())
		if err == nil {
			log.Println("✅ BaseConsumer connected to NATS server")
			return nil
		}
		log.Printf("⏳ BaseConsumer waiting for NATS server... (attempt %d/10)", i+1)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("failed to connect to NATS after 10 attempts: %w", err)
}

// StartSubscriptions 데이터 구독을 시작합니다
func (bc *BaseConsumer) StartSubscriptions(dataHandler nats.MsgHandler, metricsHandler nats.MsgHandler) error {
	if dataHandler != nil {
		sub1, err := bc.NatsConn.Subscribe("tmidb.data.>", dataHandler)
		if err != nil {
			return fmt.Errorf("failed to subscribe to data stream: %w", err)
		}
		bc.Subs = append(bc.Subs, sub1)
	}

	if metricsHandler != nil {
		sub2, err := bc.NatsConn.Subscribe("tmidb.data.system.>", metricsHandler)
		if err != nil {
			return fmt.Errorf("failed to subscribe to system metrics: %w", err)
		}
		bc.Subs = append(bc.Subs, sub2)
	}

	log.Println("📡 BaseConsumer started NATS subscriptions")
	return nil
}

// SaveToDatabase 데이터를 데이터베이스에 저장합니다
func (bc *BaseConsumer) SaveToDatabase(dataPoint DataPoint) error {
	if bc.DB == nil {
		return fmt.Errorf("database connection not available")
	}

	dataJSON, err := json.Marshal(dataPoint.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal data JSON: %w", err)
	}

	query := `
		INSERT INTO ts_obs (target_id, category_name, ts, payload) 
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (target_id, category_name, ts) DO UPDATE SET
			payload = EXCLUDED.payload
	`

	_, err = bc.DB.Exec(query, dataPoint.ID, dataPoint.Category, dataPoint.Timestamp, string(dataJSON))
	if err != nil {
		return fmt.Errorf("failed to insert data into database: %w", err)
	}

	return nil
}

// StartBatchProcessor 배치 처리를 시작합니다
func (bc *BaseConsumer) StartBatchProcessor() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	log.Println("🔄 BaseConsumer starting batch processor...")

	for {
		select {
		case <-ticker.C:
			bc.processBatch()
		case <-bc.Ctx.Done():
			log.Println("🛑 BaseConsumer stopping batch processor...")
			return
		}
	}
}

// processBatch 배치 처리를 수행합니다
func (bc *BaseConsumer) processBatch() {
	log.Println("🔄 BaseConsumer running batch processing...")

	if err := bc.aggregateData(); err != nil {
		log.Printf("❌ Failed to aggregate data: %v", err)
	}

	if err := bc.cleanupOldData(); err != nil {
		log.Printf("❌ Failed to cleanup old data: %v", err)
	}

	log.Println("✅ BaseConsumer batch processing completed")
}

func (bc *BaseConsumer) aggregateData() error {
	// This function is a placeholder for data aggregation logic.
	// In a real application, this would perform tasks like calculating hourly averages, etc.
	log.Println("📊 Data aggregation task running...")
	return nil
}

func (bc *BaseConsumer) cleanupOldData() error {
	if bc.DB == nil {
		return fmt.Errorf("database connection not available")
	}

	query := `DELETE FROM ts_obs WHERE ts < NOW() - INTERVAL '30 days'`
	result, err := bc.DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to cleanup old data: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("🧹 Cleaned up %d old records", rowsAffected)
	}
	return nil
}

// Cleanup 리소스를 정리합니다
func (bc *BaseConsumer) Cleanup() {
	log.Println("🧹 Cleaning up BaseConsumer...")
	for _, sub := range bc.Subs {
		if sub != nil {
			if err := sub.Unsubscribe(); err != nil {
					log.Printf("⚠️ Failed to unsubscribe from NATS: %v", err)
				}
		}
	}
	if bc.NatsConn != nil {
		bc.NatsConn.Close()
	}
	bc.Cancel()
	log.Println("✅ BaseConsumer cleanup completed")
}

// NATS URL을 환경 변수 또는 기본값에서 가져옵니다.
func getNatsURL() string {
	if url := os.Getenv("NATS_URL"); url != "" {
		return url
	}
	return nats.DefaultURL
}
