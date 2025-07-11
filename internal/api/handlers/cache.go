package handlers

import (
	"log"
	"time"

	"github.com/tmidb/tmidb-core/internal/cache"
)

var dataCache *cache.MemoryCache

// InitDataCache는 데이터 캐시를 초기화합니다.
func InitDataCache() {
	// 캐시 설정: 최대 1000개 항목, 30분 TTL
	dataCache = cache.NewMemoryCache(1000, 30*time.Minute)

	log.Println("Data cache initialized successfully")
}

// GetDataCache는 전역 데이터 캐시 인스턴스를 반환합니다.
func GetDataCache() *cache.MemoryCache {
	return dataCache
}

// ClearDataCache는 데이터 캐시를 전체 삭제합니다.
func ClearDataCache() error {
	if dataCache == nil {
		return nil
	}

	dataCache.Clear()
	log.Println("Data cache cleared")
	return nil
}
