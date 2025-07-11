package database

import "time"

// PromotedData는 하이브리드 스키마에서 성능 향상을 위해
// 별도 테이블에 저장되는 타입별 데이터를 담는 구조체입니다.
type PromotedData struct {
	Doubles    map[string]float64   `json:"doubles"`
	Integers   map[string]int64     `json:"integers"`
	Keywords   map[string]string    `json:"keywords"`
	Flags      map[string]bool      `json:"flags"`
	Timestamps map[string]time.Time `json:"timestamps"`
	Dates      map[string]string    `json:"dates"` // YYYY-MM-DD 형식의 문자열
}

// Organization represents the structure of an organization in the database.
type Organization struct {
	OrgID     string    `json:"org_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
