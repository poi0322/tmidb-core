package cache

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// CacheItem은 캐시 항목을 나타냅니다
type CacheItem struct {
	Value     interface{} `json:"value"`
	ExpiresAt time.Time   `json:"expires_at"`
	CreatedAt time.Time   `json:"created_at"`
	AccessAt  time.Time   `json:"access_at"`
	HitCount  int         `json:"hit_count"`
}

// isExpired는 캐시 항목이 만료되었는지 확인합니다
func (item *CacheItem) isExpired() bool {
	return !item.ExpiresAt.IsZero() && time.Now().After(item.ExpiresAt)
}

// MemoryCache는 메모리 기반 캐시를 나타냅니다
type MemoryCache struct {
	items       map[string]*CacheItem
	mutex       sync.RWMutex
	maxSize     int
	defaultTTL  time.Duration
	cleanupTick time.Duration
	stats       CacheStats
	stopCleanup chan bool
}

// CacheStats는 캐시 통계를 나타냅니다
type CacheStats struct {
	Hits        int64     `json:"hits"`
	Misses      int64     `json:"misses"`
	Sets        int64     `json:"sets"`
	Deletes     int64     `json:"deletes"`
	Evictions   int64     `json:"evictions"`
	Size        int       `json:"size"`
	MaxSize     int       `json:"max_size"`
	HitRate     float64   `json:"hit_rate"`
	LastCleanup time.Time `json:"last_cleanup"`
}

// NewMemoryCache는 새로운 메모리 캐시를 생성합니다
func NewMemoryCache(maxSize int, defaultTTL time.Duration) *MemoryCache {
	cache := &MemoryCache{
		items:       make(map[string]*CacheItem),
		maxSize:     maxSize,
		defaultTTL:  defaultTTL,
		cleanupTick: 5 * time.Minute, // 5분마다 정리
		stopCleanup: make(chan bool),
		stats: CacheStats{
			MaxSize: maxSize,
		},
	}

	// 백그라운드 정리 고루틴 시작
	go cache.cleanupExpired()

	return cache
}

// Set은 캐시에 값을 저장합니다
func (c *MemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	var expiresAt time.Time

	if ttl > 0 {
		expiresAt = now.Add(ttl)
	} else if c.defaultTTL > 0 {
		expiresAt = now.Add(c.defaultTTL)
	}

	// 메모리 제한 확인
	if len(c.items) >= c.maxSize {
		c.evictOldest()
	}

	c.items[key] = &CacheItem{
		Value:     value,
		ExpiresAt: expiresAt,
		CreatedAt: now,
		AccessAt:  now,
		HitCount:  0,
	}

	c.stats.Sets++
	c.stats.Size = len(c.items)

	log.Printf("캐시 설정: %s (TTL: %v)", key, ttl)
}

// Get은 캐시에서 값을 조회합니다
func (c *MemoryCache) Get(key string) (interface{}, bool) {
	c.mutex.RLock()
	item, exists := c.items[key]
	c.mutex.RUnlock()

	if !exists {
		c.mutex.Lock()
		c.stats.Misses++
		c.mutex.Unlock()
		return nil, false
	}

	if item.isExpired() {
		c.Delete(key)
		c.mutex.Lock()
		c.stats.Misses++
		c.mutex.Unlock()
		return nil, false
	}

	c.mutex.Lock()
	item.AccessAt = time.Now()
	item.HitCount++
	c.stats.Hits++
	c.calculateHitRate()
	c.mutex.Unlock()

	return item.Value, true
}

// GetString은 문자열 값을 조회합니다
func (c *MemoryCache) GetString(key string) (string, bool) {
	value, exists := c.Get(key)
	if !exists {
		return "", false
	}

	if str, ok := value.(string); ok {
		return str, true
	}

	return "", false
}

// GetJSON은 JSON 문자열을 파싱하여 반환합니다
func (c *MemoryCache) GetJSON(key string, dest interface{}) bool {
	value, exists := c.Get(key)
	if !exists {
		return false
	}

	if jsonStr, ok := value.(string); ok {
		err := json.Unmarshal([]byte(jsonStr), dest)
		return err == nil
	}

	return false
}

// SetJSON은 값을 JSON 문자열로 저장합니다
func (c *MemoryCache) SetJSON(key string, value interface{}, ttl time.Duration) error {
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("JSON 마샬링 실패: %v", err)
	}

	c.Set(key, string(jsonBytes), ttl)
	return nil
}

// Delete는 캐시에서 키를 삭제합니다
func (c *MemoryCache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, exists := c.items[key]; exists {
		delete(c.items, key)
		c.stats.Deletes++
		c.stats.Size = len(c.items)
		log.Printf("캐시 삭제: %s", key)
	}
}

// DeletePattern은 패턴에 맞는 모든 키를 삭제합니다
func (c *MemoryCache) DeletePattern(pattern string) int {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var keysToDelete []string
	for key := range c.items {
		// 간단한 패턴 매칭 (와일드카드 지원)
		if c.matchPattern(key, pattern) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(c.items, key)
		c.stats.Deletes++
	}

	c.stats.Size = len(c.items)
	
	if len(keysToDelete) > 0 {
		log.Printf("패턴 캐시 삭제: %s (%d개)", pattern, len(keysToDelete))
	}

	return len(keysToDelete)
}

// Clear는 모든 캐시를 삭제합니다
func (c *MemoryCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	itemCount := len(c.items)
	c.items = make(map[string]*CacheItem)
	c.stats.Deletes += int64(itemCount)
	c.stats.Size = 0

	log.Printf("전체 캐시 삭제: %d개", itemCount)
}

// Exists는 키가 존재하는지 확인합니다
func (c *MemoryCache) Exists(key string) bool {
	c.mutex.RLock()
	item, exists := c.items[key]
	c.mutex.RUnlock()

	if !exists {
		return false
	}

	return !item.isExpired()
}

// Keys는 모든 키를 반환합니다
func (c *MemoryCache) Keys() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	var keys []string
	for key, item := range c.items {
		if !item.isExpired() {
			keys = append(keys, key)
		}
	}

	return keys
}

// Size는 현재 캐시 크기를 반환합니다
func (c *MemoryCache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.items)
}

// Stats는 캐시 통계를 반환합니다
func (c *MemoryCache) Stats() CacheStats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	stats := c.stats
	stats.Size = len(c.items)
	return stats
}

// InvalidateCategory는 카테고리 관련 캐시를 무효화합니다
func (c *MemoryCache) InvalidateCategory(category string) {
	patterns := []string{
		fmt.Sprintf("category:%s:*", category),
		fmt.Sprintf("schema:%s", category),
		fmt.Sprintf("targets:%s:*", category),
		fmt.Sprintf("timeseries:%s:*", category),
	}

	total := 0
	for _, pattern := range patterns {
		total += c.DeletePattern(pattern)
	}

	log.Printf("카테고리 캐시 무효화: %s (%d개)", category, total)
}

// InvalidateTarget은 타겟 관련 캐시를 무효화합니다
func (c *MemoryCache) InvalidateTarget(targetID string) {
	patterns := []string{
		fmt.Sprintf("target:%s:*", targetID),
		fmt.Sprintf("targets:*:%s", targetID),
		fmt.Sprintf("timeseries:*:%s:*", targetID),
	}

	total := 0
	for _, pattern := range patterns {
		total += c.DeletePattern(pattern)
	}

	log.Printf("타겟 캐시 무효화: %s (%d개)", targetID, total)
}

// 내부 메서드들

// cleanupExpired는 만료된 항목들을 정리합니다
func (c *MemoryCache) cleanupExpired() {
	ticker := time.NewTicker(c.cleanupTick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.performCleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// performCleanup은 실제 정리 작업을 수행합니다
func (c *MemoryCache) performCleanup() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	var expiredKeys []string

	for key, item := range c.items {
		if item.isExpired() {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(c.items, key)
		c.stats.Evictions++
	}

	c.stats.Size = len(c.items)
	c.stats.LastCleanup = now

	if len(expiredKeys) > 0 {
		log.Printf("만료된 캐시 정리: %d개", len(expiredKeys))
	}
}

// evictOldest는 가장 오래된 항목을 제거합니다
func (c *MemoryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, item := range c.items {
		if oldestKey == "" || item.AccessAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = item.AccessAt
		}
	}

	if oldestKey != "" {
		delete(c.items, oldestKey)
		c.stats.Evictions++
		log.Printf("캐시 용량 초과로 제거: %s", oldestKey)
	}
}

// calculateHitRate는 히트율을 계산합니다
func (c *MemoryCache) calculateHitRate() {
	total := c.stats.Hits + c.stats.Misses
	if total > 0 {
		c.stats.HitRate = float64(c.stats.Hits) / float64(total) * 100
	}
}

// matchPattern은 간단한 패턴 매칭을 수행합니다
func (c *MemoryCache) matchPattern(str, pattern string) bool {
	// 간단한 와일드카드 매칭 (*만 지원)
	if pattern == "*" {
		return true
	}

	// * 가 없으면 정확히 일치해야 함
	if !contains(pattern, "*") {
		return str == pattern
	}

	// * 기준으로 분리
	parts := split(pattern, "*")
	if len(parts) == 0 {
		return true
	}

	// 첫 번째 부분으로 시작해야 함
	if parts[0] != "" && !hasPrefix(str, parts[0]) {
		return false
	}

	// 마지막 부분으로 끝나야 함
	if len(parts) > 1 && parts[len(parts)-1] != "" && !hasSuffix(str, parts[len(parts)-1]) {
		return false
	}

	// 중간 부분들이 순서대로 포함되어야 함
	currentPos := len(parts[0])
	for i := 1; i < len(parts)-1; i++ {
		part := parts[i]
		if part == "" {
			continue
		}

		pos := indexOf(str[currentPos:], part)
		if pos == -1 {
			return false
		}
		currentPos += pos + len(part)
	}

	return true
}

// 문자열 유틸리티 함수들
func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func split(s, sep string) []string {
	if sep == "" {
		return []string{s}
	}

	var parts []string
	start := 0

	for {
		pos := indexOf(s[start:], sep)
		if pos == -1 {
			parts = append(parts, s[start:])
			break
		}

		parts = append(parts, s[start:start+pos])
		start += pos + len(sep)
	}

	return parts
}

// Close는 캐시를 종료합니다
func (c *MemoryCache) Close() {
	close(c.stopCleanup)
	c.Clear()
	log.Println("메모리 캐시 종료됨")
} 