package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/tmidb/tmidb-core/internal/config"
)

// Target 데이터 구조체
type TargetData struct {
	TargetID     string                 `json:"target_id"`
	Name         string                 `json:"name"`
	CategoryName string                 `json:"category_name"`
	CategoryData map[string]interface{} `json:"category_data"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// CreateTargetData는 새로운 타겟 데이터를 생성합니다.
func (conn *OrgDBConnection) CreateTargetData(targetName, categoryName string, categoryData map[string]interface{}) (*TargetData, error) {
	tx, err := conn.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// 1. 타겟 생성 또는 조회
	var targetID string
	var createdAt, updatedAt time.Time

	err = tx.QueryRow(`
		INSERT INTO target (name) 
		VALUES ($1) 
		ON CONFLICT (name) DO UPDATE SET updated_at = now()
		RETURNING target_id, created_at, updated_at
	`, targetName).Scan(&targetID, &createdAt, &updatedAt)

	if err != nil {
		// 이미 존재하는 타겟인 경우 조회
		err = tx.QueryRow(`
			SELECT target_id, created_at, updated_at 
			FROM target 
			WHERE name = $1
		`, targetName).Scan(&targetID, &createdAt, &updatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to create or get target: %v", err)
		}
	}

	// 2. 카테고리 데이터 JSON 변환
	categoryDataJSON, err := json.Marshal(categoryData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal category data: %v", err)
	}

	// 3. 타겟-카테고리 매핑 생성/업데이트
	_, err = tx.Exec(`
		INSERT INTO target_categories (target_id, org_id, category_name, category_data)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (target_id, category_name) DO UPDATE SET
			category_data = EXCLUDED.category_data,
			updated_at = now()
	`, targetID, conn.OrgID, categoryName, string(categoryDataJSON))

	if err != nil {
		return nil, fmt.Errorf("failed to create target category: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &TargetData{
		TargetID:     targetID,
		Name:         targetName,
		CategoryName: categoryName,
		CategoryData: categoryData,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}, nil
}

// GetTargetData는 타겟 데이터를 조회합니다.
func (conn *OrgDBConnection) GetTargetData(targetID, categoryName string) (*TargetData, error) {
	var target TargetData
	var categoryDataJSON string

	err := conn.DB.QueryRow(`
		SELECT t.target_id, t.name, tc.category_name, tc.category_data, t.created_at, t.updated_at
		FROM target t
		JOIN target_categories tc ON t.target_id = tc.target_id
		WHERE t.target_id = $1 AND tc.category_name = $2 AND tc.org_id = $3
	`, targetID, categoryName, conn.OrgID).Scan(
		&target.TargetID, &target.Name, &target.CategoryName,
		&categoryDataJSON, &target.CreatedAt, &target.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to get target data: %v", err)
	}

	// JSON 파싱
	if err := json.Unmarshal([]byte(categoryDataJSON), &target.CategoryData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal category data: %v", err)
	}

	return &target, nil
}

// UpdateTargetData는 타겟 데이터를 업데이트합니다.
func (conn *OrgDBConnection) UpdateTargetData(targetID, categoryName string, categoryData map[string]interface{}) error {
	categoryDataJSON, err := json.Marshal(categoryData)
	if err != nil {
		return fmt.Errorf("failed to marshal category data: %v", err)
	}

	_, err = conn.DB.Exec(`
		UPDATE target_categories 
		SET category_data = $1, updated_at = now()
		WHERE target_id = $2 AND category_name = $3 AND org_id = $4
	`, string(categoryDataJSON), targetID, categoryName, conn.OrgID)

	if err != nil {
		return fmt.Errorf("failed to update target data: %v", err)
	}

	return nil
}

// DeleteTargetData는 타겟 데이터를 삭제합니다.
func (conn *OrgDBConnection) DeleteTargetData(targetID, categoryName string) error {
	_, err := conn.DB.Exec(`
		DELETE FROM target_categories 
		WHERE target_id = $1 AND category_name = $2 AND org_id = $3
	`, targetID, categoryName, conn.OrgID)

	if err != nil {
		return fmt.Errorf("failed to delete target data: %v", err)
	}

	return nil
}

// TimeSeriesData 구조체
type TimeSeriesData struct {
	TargetID     string                 `json:"target_id"`
	CategoryName string                 `json:"category_name"`
	Timestamp    time.Time              `json:"timestamp"`
	Payload      map[string]interface{} `json:"payload"`
}

// InsertTimeSeriesData는 시계열 데이터를 삽입합니다.
func (conn *OrgDBConnection) InsertTimeSeriesData(targetID, categoryName string, payload map[string]interface{}, timestamp *time.Time) error {
	ts := time.Now()
	if timestamp != nil {
		ts = *timestamp
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	_, err = conn.DB.Exec(`
		INSERT INTO ts_obs (target_id, category_name, ts, payload)
		VALUES ($1, $2, $3, $4)
	`, targetID, categoryName, ts, string(payloadJSON))

	if err != nil {
		return fmt.Errorf("failed to insert time series data: %v", err)
	}

	return nil
}

// GetTimeSeriesData는 시계열 데이터를 조회합니다.
func (conn *OrgDBConnection) GetTimeSeriesData(targetID, categoryName string, startTime, endTime *time.Time, limit int) ([]TimeSeriesData, error) {
	var query string
	var args []interface{}

	// 기본 쿼리
	query = `
		SELECT target_id, category_name, ts, payload
		FROM ts_obs
		WHERE target_id = $1 AND category_name = $2
	`
	args = []interface{}{targetID, categoryName}

	// 시간 범위 조건 추가
	if startTime != nil {
		query += " AND ts >= $" + fmt.Sprintf("%d", len(args)+1)
		args = append(args, *startTime)
	}
	if endTime != nil {
		query += " AND ts <= $" + fmt.Sprintf("%d", len(args)+1)
		args = append(args, *endTime)
	}

	// 정렬 및 제한
	query += " ORDER BY ts DESC"
	if limit > 0 {
		query += " LIMIT $" + fmt.Sprintf("%d", len(args)+1)
		args = append(args, limit)
	}

	rows, err := conn.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query time series data: %v", err)
	}
	defer rows.Close()

	var results []TimeSeriesData
	for rows.Next() {
		var data TimeSeriesData
		var payloadJSON string

		if err := rows.Scan(&data.TargetID, &data.CategoryName, &data.Timestamp, &payloadJSON); err != nil {
			return nil, fmt.Errorf("failed to scan time series data: %v", err)
		}

		// JSON 파싱
		if err := json.Unmarshal([]byte(payloadJSON), &data.Payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %v", err)
		}

		results = append(results, data)
	}

	return results, nil
}

// SearchTargets는 타겟을 검색합니다.
func (conn *OrgDBConnection) SearchTargets(query, categoryName string, limit int) ([]TargetData, error) {
	var sqlQuery string
	var args []interface{}

	// 기본 쿼리
	sqlQuery = `
		SELECT DISTINCT t.target_id, t.name, tc.category_name, tc.category_data, t.created_at, t.updated_at
		FROM target t
		JOIN target_categories tc ON t.target_id = tc.target_id
		WHERE tc.org_id = $1
	`
	args = []interface{}{conn.OrgID}

	// 검색 조건 추가
	if query != "" {
		sqlQuery += " AND (t.name ILIKE $" + fmt.Sprintf("%d", len(args)+1) + " OR t.target_id::text ILIKE $" + fmt.Sprintf("%d", len(args)+1) + ")"
		args = append(args, "%"+query+"%")
	}

	if categoryName != "" {
		sqlQuery += " AND tc.category_name = $" + fmt.Sprintf("%d", len(args)+1)
		args = append(args, categoryName)
	}

	// 정렬 및 제한
	sqlQuery += " ORDER BY t.created_at DESC"
	if limit > 0 {
		sqlQuery += " LIMIT $" + fmt.Sprintf("%d", len(args)+1)
		args = append(args, limit)
	}

	rows, err := conn.DB.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search targets: %v", err)
	}
	defer rows.Close()

	var results []TargetData
	for rows.Next() {
		var target TargetData
		var categoryDataJSON string

		if err := rows.Scan(&target.TargetID, &target.Name, &target.CategoryName, &categoryDataJSON, &target.CreatedAt, &target.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan target data: %v", err)
		}

		// JSON 파싱
		if err := json.Unmarshal([]byte(categoryDataJSON), &target.CategoryData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal category data: %v", err)
		}

		results = append(results, target)
	}

	return results, nil
}

// TargetSearchResult는 타겟 검색 결과를 담는 구조체입니다.
type TargetSearchResult struct {
	TargetID string `json:"target_id"`
	Name     string `json:"name"`
}

// SearchTargets는 이름 또는 ID로 타겟을 검색합니다.
func SearchTargets(cfg *config.Config, orgID, query string) ([]TargetSearchResult, error) {
	db, err := ConnectToOrganizationDatabase(orgID, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to organization database for org %s: %w", orgID, err)
	}
	defer db.Close()

	// ILIKE를 사용하여 대소문자 구분 없이 검색
	searchQuery := `
		SELECT target_id, target_name
		FROM public.targets
		WHERE org_id = $1 AND (target_name ILIKE $2 OR target_id::text ILIKE $2)
		ORDER BY target_name
		LIMIT 10;
	`
	searchPattern := "%" + query + "%"
	rows, err := db.Query(searchQuery, orgID, searchPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search targets for org %s: %w", orgID, err)
	}
	defer rows.Close()

	var results []TargetSearchResult
	for rows.Next() {
		var t TargetSearchResult
		if err := rows.Scan(&t.TargetID, &t.Name); err != nil {
			log.Printf("Warning: failed to scan target search result for org %s: %v", orgID, err)
			continue
		}
		results = append(results, t)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration for target search, org %s: %w", orgID, err)
	}

	return results, nil
}

// GetDataExplorerData는 데이터 탐색기를 위한 데이터를 조회합니다.
func (conn *OrgDBConnection) GetDataExplorerData(filters map[string]interface{}, limit int) ([]map[string]interface{}, error) {
	var query string
	var args []interface{}

	// 기본 쿼리 - 최근 데이터와 타겟 정보 조인
	query = `
		SELECT 
			t.target_id,
			t.name as target_name,
			tc.category_name,
			tc.category_data,
			ts.ts as timestamp,
			ts.payload
		FROM target t
		JOIN target_categories tc ON t.target_id = tc.target_id
		LEFT JOIN ts_obs ts ON t.target_id = ts.target_id AND tc.category_name = ts.category_name
		WHERE tc.org_id = $1
	`
	args = []interface{}{conn.OrgID}

	// 필터 조건 추가
	if categoryName, ok := filters["category"]; ok {
		query += " AND tc.category_name = $" + fmt.Sprintf("%d", len(args)+1)
		args = append(args, categoryName)
	}

	if targetID, ok := filters["target_id"]; ok {
		query += " AND t.target_id = $" + fmt.Sprintf("%d", len(args)+1)
		args = append(args, targetID)
	}

	// 정렬 및 제한
	query += " ORDER BY ts.ts DESC NULLS LAST"
	if limit > 0 {
		query += " LIMIT $" + fmt.Sprintf("%d", len(args)+1)
		args = append(args, limit)
	}

	rows, err := conn.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query data explorer data: %v", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var targetID, targetName, categoryName, categoryDataJSON string
		var timestamp sql.NullTime
		var payloadJSON sql.NullString

		if err := rows.Scan(&targetID, &targetName, &categoryName, &categoryDataJSON, &timestamp, &payloadJSON); err != nil {
			return nil, fmt.Errorf("failed to scan data explorer data: %v", err)
		}

		// 기본 데이터 구성
		record := map[string]interface{}{
			"target_id":     targetID,
			"target_name":   targetName,
			"category_name": categoryName,
		}

		// 카테고리 데이터 파싱
		var categoryData map[string]interface{}
		if err := json.Unmarshal([]byte(categoryDataJSON), &categoryData); err == nil {
			record["category_data"] = categoryData
		}

		// 시계열 데이터 파싱 (있는 경우)
		if timestamp.Valid {
			record["timestamp"] = timestamp.Time
			if payloadJSON.Valid {
				var payload map[string]interface{}
				if err := json.Unmarshal([]byte(payloadJSON.String), &payload); err == nil {
					record["payload"] = payload
				}
			}
		}

		results = append(results, record)
	}

	return results, nil
}

// SchemaDefinition은 카테고리 스키마의 구조를 정의합니다.
type SchemaDefinition struct {
	Fields       map[string]SchemaField `json:"fields"`
	IsTimeseries bool                   `json:"is_timeseries"`
}

// SchemaField는 스키마 내의 개별 필드 속성을 정의합니다.
type SchemaField struct {
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// ValidationResult는 데이터 유효성 검증 결과를 담습니다.
type ValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

// getLatestCategorySchema는 특정 카테고리의 가장 최신 활성 스키마를 조회합니다.
func getLatestCategorySchema(db *sql.DB, orgID, categoryName string) (*SchemaDefinition, int, error) {
	query := `
		SELECT schema_definition, version
		FROM public.category_schemas
		WHERE org_id = $1 AND category_name = $2 AND is_active = true
		ORDER BY version DESC
		LIMIT 1;
	`
	var schemaDefRaw []byte
	var version int
	err := db.QueryRow(query, orgID, categoryName).Scan(&schemaDefRaw, &version)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, 0, fmt.Errorf("no active schema found for category '%s'", categoryName)
		}
		return nil, 0, fmt.Errorf("failed to query schema for category '%s': %w", categoryName, err)
	}

	var schemaDef SchemaDefinition
	if err := json.Unmarshal(schemaDefRaw, &schemaDef); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal schema for category '%s': %w", categoryName, err)
	}
	return &schemaDef, version, nil
}

// ValidateDataAgainstSchema는 주어진 데이터가 카테고리 스키마에 맞는지 검증합니다.
func ValidateDataAgainstSchema(cfg *config.Config, orgID, categoryName string, dataToValidate []byte) (*ValidationResult, error) {
	db, err := ConnectToOrganizationDatabase(orgID, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to organization database: %w", err)
	}
	defer db.Close()

	schema, _, err := getLatestCategorySchema(db, orgID, categoryName)
	if err != nil {
		return nil, err
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(dataToValidate, &dataMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data to validate: %w", err)
	}

	var errors []string
	for fieldName, fieldSchema := range schema.Fields {
		value, exists := dataMap[fieldName]

		if fieldSchema.Required && !exists {
			errors = append(errors, fmt.Sprintf("필수 필드 '%s'가 누락되었습니다.", fieldName))
			continue
		}

		if !exists {
			continue // 필수 아닌 필드가 누락된 경우는 검증 통과
		}

		// 타입 검증 (간단한 버전)
		valueType := reflect.TypeOf(value).Kind()

		switch fieldSchema.Type {
		case "string":
			if valueType != reflect.String {
				errors = append(errors, fmt.Sprintf("필드 '%s'의 타입이 문자열(string)이 아닙니다.", fieldName))
			}
		case "number":
			if valueType != reflect.Float64 { // JSON 숫자는 float64로 언마샬됨
				errors = append(errors, fmt.Sprintf("필드 '%s'의 타입이 숫자(number)가 아닙니다.", fieldName))
			}
		case "boolean":
			if valueType != reflect.Bool {
				errors = append(errors, fmt.Sprintf("필드 '%s'의 타입이 불리언(boolean)이 아닙니다.", fieldName))
			}
		case "object":
			if valueType != reflect.Map {
				errors = append(errors, fmt.Sprintf("필드 '%s'의 타입이 객체(object)가 아닙니다.", fieldName))
			}
		case "array":
			if valueType != reflect.Slice {
				errors = append(errors, fmt.Sprintf("필드 '%s'의 타입이 배열(array)이 아닙니다.", fieldName))
			}
		}
	}

	return &ValidationResult{
		Valid:  len(errors) == 0,
		Errors: errors,
	}, nil
}

// SaveExplorerData는 데이터 탐색기에서 입력된 데이터를 저장/수정합니다.
func SaveExplorerData(cfg *config.Config, orgID, targetID, categoryName string, jsonData []byte) error {
	db, err := ConnectToOrganizationDatabase(orgID, cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to organization database: %w", err)
	}
	defer db.Close()

	// 최신 스키마 버전 조회
	_, schemaVersion, err := getLatestCategorySchema(db, orgID, categoryName)
	if err != nil {
		// 스키마가 없으면 저장 불가
		return fmt.Errorf("cannot save data without a valid schema for category '%s': %w", categoryName, err)
	}

	// UPSERT 쿼리
	query := `
		INSERT INTO public.target_categories (target_id, org_id, category_name, category_data, schema_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (target_id, category_name) DO UPDATE SET
			category_data = EXCLUDED.category_data,
			schema_version = EXCLUDED.schema_version,
			updated_at = NOW();
	`

	_, err = db.Exec(query, targetID, orgID, categoryName, jsonData, schemaVersion)
	if err != nil {
		return fmt.Errorf("failed to upsert data for target %s, category %s: %w", targetID, categoryName, err)
	}

	return nil
}

// UpsertHybridData는 하이브리드 스키마 데이터를 처리합니다.
// 트랜잭션 내에서 target_categories와 promoted_* 테이블들을 모두 처리합니다.
func (db *OrgDBConnection) UpsertHybridData(targetID, targetName, category string, schemaVersion int, jsonData map[string]interface{}, promotedData PromotedData) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // 롤백은 기본적으로 호출, 커밋 성공 시 무시됨

	// 1. target_categories 테이블에 JSON 데이터 UPSERT
	jsonDataBytes, err := json.Marshal(jsonData)
	if err != nil {
		return fmt.Errorf("failed to marshal json data: %w", err)
	}

	upsertTargetSQL := `
		INSERT INTO targets (target_id, org_id, target_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (target_id) DO UPDATE SET target_name = EXCLUDED.target_name;
	`
	// org_id를 db.OrgID에서 가져와야 합니다.
	if _, err := tx.Exec(upsertTargetSQL, targetID, db.OrgID, targetName); err != nil {
		return fmt.Errorf("failed to upsert into targets: %w", err)
	}

	upsertCategorySQL := `
		INSERT INTO target_categories (target_id, org_id, category_name, schema_version, category_data)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (target_id, category_name) DO UPDATE
		SET schema_version = $4, category_data = $5, updated_at = now();
	`
	if _, err := tx.Exec(upsertCategorySQL, targetID, db.OrgID, category, schemaVersion, jsonDataBytes); err != nil {
		return fmt.Errorf("failed to upsert into target_categories: %w", err)
	}

	// 2. 프로모션된 Double 데이터 UPSERT
	if len(promotedData.Doubles) > 0 {
		upsertDoubleSQL := `
			INSERT INTO promoted_doubles (target_id, category_name, field_name, value)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (target_id, category_name, field_name) DO UPDATE
			SET value = $4;
		`
		for field, value := range promotedData.Doubles {
			if _, err := tx.Exec(upsertDoubleSQL, targetID, category, field, value); err != nil {
				return fmt.Errorf("failed to upsert double '%s': %w", field, err)
			}
		}
	}

	// 3. 프로모션된 Integer 데이터 UPSERT
	if len(promotedData.Integers) > 0 {
		upsertIntegerSQL := `
			INSERT INTO promoted_integers (target_id, category_name, field_name, value)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (target_id, category_name, field_name) DO UPDATE
			SET value = $4;
		`
		for field, value := range promotedData.Integers {
			if _, err := tx.Exec(upsertIntegerSQL, targetID, category, field, value); err != nil {
				return fmt.Errorf("failed to upsert integer '%s': %w", field, err)
			}
		}
	}

	// 4. 프로모션된 Keyword 데이터 UPSERT
	if len(promotedData.Keywords) > 0 {
		upsertKeywordSQL := `
			INSERT INTO promoted_keywords (target_id, category_name, field_name, value)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (target_id, category_name, field_name) DO UPDATE
			SET value = $4;
		`
		for field, value := range promotedData.Keywords {
			if _, err := tx.Exec(upsertKeywordSQL, targetID, category, field, value); err != nil {
				return fmt.Errorf("failed to upsert keyword '%s': %w", field, err)
			}
		}
	}

	// 5. 프로모션된 Flag 데이터 UPSERT
	if len(promotedData.Flags) > 0 {
		upsertFlagSQL := `
			INSERT INTO promoted_flags (target_id, category_name, field_name, value)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (target_id, category_name, field_name) DO UPDATE
			SET value = $4;
		`
		for field, value := range promotedData.Flags {
			if _, err := tx.Exec(upsertFlagSQL, targetID, category, field, value); err != nil {
				return fmt.Errorf("failed to upsert flag '%s': %w", field, err)
			}
		}
	}

	// 6. 프로모션된 Timestamp 데이터 UPSERT
	if len(promotedData.Timestamps) > 0 {
		upsertTimestampSQL := `
			INSERT INTO promoted_timestamps (target_id, category_name, field_name, value)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (target_id, category_name, field_name) DO UPDATE
			SET value = $4;
		`
		for field, value := range promotedData.Timestamps {
			if _, err := tx.Exec(upsertTimestampSQL, targetID, category, field, value); err != nil {
				return fmt.Errorf("failed to upsert timestamp '%s': %w", field, err)
			}
		}
	}

	// 7. 프로모션된 Date 데이터 UPSERT
	if len(promotedData.Dates) > 0 {
		upsertDateSQL := `
			INSERT INTO promoted_dates (target_id, category_name, field_name, value)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (target_id, category_name, field_name) DO UPDATE
			SET value = $4;
		`
		for field, value := range promotedData.Dates {
			if _, err := tx.Exec(upsertDateSQL, targetID, category, field, value); err != nil {
				return fmt.Errorf("failed to upsert date '%s': %w", field, err)
			}
		}
	}

	// 8. 모든 작업이 성공했으므로 트랜잭션 커밋
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// CategoryInfoForExplorer는 데이터 탐색기에서 사용할 카테고리 정보를 담는 구조체입니다.
type CategoryInfoForExplorer struct {
	CategoryName     string          `json:"category_name"`
	Version          int             `json:"version"`
	SchemaDefinition json.RawMessage `json:"schema"` // Raw JSON으로 전달
	IsTimeseries     bool            `json:"is_timeseries"`
	IsActive         bool            `json:"is_active"`
	SchemaID         string          `json:"schema_id"`
}

// GetCategoriesForExplorer는 특정 조직의 모든 활성 카테고리(최신 버전) 목록을 조회합니다.
func GetCategoriesForExplorer(cfg *config.Config, orgID string) ([]CategoryInfoForExplorer, error) {
	db, err := ConnectToOrganizationDatabase(orgID, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to organization database for org %s: %w", orgID, err)
	}
	defer db.Close()

	// 각 카테고리의 가장 최신 활성 버전 스키마를 가져오는 쿼리
	query := `
		SELECT DISTINCT ON (category_name)
			schema_id, category_name, version, schema_definition, is_active
		FROM
			public.category_schemas
		WHERE
			org_id = $1 AND is_active = true
		ORDER BY
			category_name, version DESC;
	`

	rows, err := db.Query(query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query categories for org %s: %w", orgID, err)
	}
	defer rows.Close()

	var categories []CategoryInfoForExplorer

	for rows.Next() {
		var cat CategoryInfoForExplorer
		var schemaDefRaw []byte

		if err := rows.Scan(&cat.SchemaID, &cat.CategoryName, &cat.Version, &schemaDefRaw, &cat.IsActive); err != nil {
			log.Printf("Warning: failed to scan category row for org %s: %v", orgID, err)
			continue
		}

		cat.SchemaDefinition = json.RawMessage(schemaDefRaw)

		// 스키마 정의에서 is_timeseries 추출
		var schemaMap map[string]interface{}
		if err := json.Unmarshal(schemaDefRaw, &schemaMap); err == nil {
			if isTs, ok := schemaMap["is_timeseries"].(bool); ok {
				cat.IsTimeseries = isTs
			}
		}

		categories = append(categories, cat)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration for org %s: %w", orgID, err)
	}

	return categories, nil
}

// ExplorerDataFilter는 데이터 탐색기 필터 조건을 담는 구조체입니다.
type ExplorerDataFilter struct {
	Category  string
	StartDate string
	EndDate   string
}

// ExplorerDataRow는 데이터 탐색기 목록의 한 행을 나타냅니다.
type ExplorerDataRow struct {
	RecordID  string          `json:"record_id"`
	TargetID  string          `json:"target_id"`
	Category  string          `json:"category"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// GetExplorerData는 필터 조건에 맞는 데이터를 조회합니다.
func GetExplorerData(cfg *config.Config, orgID string, filter ExplorerDataFilter) ([]ExplorerDataRow, error) {
	db, err := ConnectToOrganizationDatabase(orgID, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to organization database for org %s: %w", orgID, err)
	}
	defer db.Close()

	query := `
		SELECT
			(target_id::text || '_' || category_name) as record_id,
			target_id,
			category_name,
			updated_at,
			category_data
		FROM
			public.target_categories
		WHERE org_id = $1
	`
	args := []interface{}{orgID}
	argCount := 1

	if filter.Category != "" {
		argCount++
		query += fmt.Sprintf(" AND category_name = $%d", argCount)
		args = append(args, filter.Category)
	}
	if filter.StartDate != "" {
		argCount++
		query += fmt.Sprintf(" AND updated_at >= $%d", argCount)
		args = append(args, filter.StartDate)
	}
	if filter.EndDate != "" {
		argCount++
		endDate, _ := time.Parse("2006-01-02", filter.EndDate)
		endDate = endDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		query += fmt.Sprintf(" AND updated_at <= $%d", argCount)
		args = append(args, endDate)
	}

	query += " ORDER BY updated_at DESC LIMIT 200" // 페이지네이션 전 임시 제한

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query explorer data for org %s: %w", orgID, err)
	}
	defer rows.Close()

	var results []ExplorerDataRow
	for rows.Next() {
		var r ExplorerDataRow
		if err := rows.Scan(&r.RecordID, &r.TargetID, &r.Category, &r.Timestamp, &r.Data); err != nil {
			log.Printf("Warning: failed to scan explorer data row for org %s: %v", orgID, err)
			continue
		}
		results = append(results, r)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration for explorer data, org %s: %w", orgID, err)
	}

	return results, nil
}
